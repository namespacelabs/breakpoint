package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/muesli/reflow/wordwrap"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/metadata"
	internalv1 "namespacelabs.dev/breakpoint/api/private/v1"
	v1 "namespacelabs.dev/breakpoint/api/public/v1"
	"namespacelabs.dev/breakpoint/pkg/github"
	"namespacelabs.dev/breakpoint/pkg/githuboidc"
	"namespacelabs.dev/breakpoint/pkg/internalserver"
	"namespacelabs.dev/breakpoint/pkg/passthrough"
	"namespacelabs.dev/breakpoint/pkg/quicproxyclient"
	"namespacelabs.dev/breakpoint/pkg/sshd"
	"namespacelabs.dev/breakpoint/pkg/waiter"
)

func init() {
	rootCmd.AddCommand(newWaitCmd())
}

func newWaitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Blocks for the duration of the breakpoint",
	}

	config := cmd.Flags().String("config", "", "Path to the configuration file.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *config == "" {
			return errors.New("--config is required")
		}

		ctx := cmd.Context()

		cfg, err := loadConfig(ctx, *config)
		if err != nil {
			return err
		}

		mgr, ctx := waiter.NewManager(ctx, waiter.ManagerOpts{
			InitialDur: cfg.ParsedDuration,
			Webhooks:   cfg.Webhooks,
		})

		sshd, err := sshd.MakeServer(ctx, sshd.SSHServerOpts{
			Shell:          cfg.Shell,
			AuthorizedKeys: cfg.AllKeys,
			AllowedUsers:   cfg.AllowedSSHUsers,
			Env:            os.Environ(),
			InteractiveMOTD: func(w io.Writer) {
				ww := wordwrap.NewWriter(80)

				fmt.Fprintln(ww)
				fmt.Fprintf(ww, "Welcome to a breakpoint-provided remote shell.\n")
				fmt.Fprintln(ww)
				fmt.Fprintf(ww, "This breakpoint will expire %s.\n", humanize.Time(mgr.Expiration()))
				fmt.Fprintln(ww)
				fmt.Fprintf(ww, "The following additional commands are available:\n\n")
				fmt.Fprintf(ww, " - `breakpoint extend` to extend the breakpoint duration.\n")
				fmt.Fprintf(ww, " - `breakpoint resume` to resume immediately.\n")

				_ = ww.Close()

				_, _ = w.Write(ww.Bytes())
			},
		})
		if err != nil {
			return err
		}

		eg, ctx := errgroup.WithContext(ctx)

		pl := passthrough.NewListener(ctx, dummyAddr{})

		eg.Go(func() error {
			return sshd.Serve(pl)
		})

		eg.Go(func() error {
			defer pl.Close()

			return quicproxyclient.Serve(ctx, cfg.Endpoint, cfg.RegisterMetadata, quicproxyclient.Handlers{
				OnAllocation: func(endpoint string) {
					mgr.SetEndpoint(endpoint)
				},
				Proxy: pl.Offer,
			})
		})

		eg.Go(func() error {
			return internalserver.ListenAndServe(ctx, mgr)
		})

		eg.Go(func() error {
			return mgr.Wait()
		})

		return cancelIsOK(eg.Wait())
	}

	return cmd
}

func loadConfig(ctx context.Context, file string) (ParsedConfig, error) {
	var cfg ParsedConfig
	if err := loadJSON(file, &cfg.WaitConfig); err != nil {
		return cfg, err
	}

	for _, wh := range cfg.Webhooks {
		if wh.URL == "" {
			return cfg, errors.New("webhook is missing url")
		}
	}

	if len(cfg.Shell) == 0 {
		cfg.Shell = []string{"/bin/sh"}
	}

	requireGitHubOIDC := false
	for _, feature := range cfg.Enable {
		switch feature {
		case "github/oidc":
			// Force enable.
			requireGitHubOIDC = false

		default:
			return cfg, fmt.Errorf("unknown feature %q", feature)
		}
	}

	cfg.RegisterMetadata = metadata.MD{}
	if githuboidc.OIDCAvailable() || requireGitHubOIDC {
		token, err := githuboidc.JWT(ctx, v1.GitHubOIDCAudience)
		if err != nil {
			if requireGitHubOIDC {
				return cfg, err
			}

			zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to obtain GitHUB OIDC token")
		} else {
			cfg.RegisterMetadata[v1.GitHubOIDCTokenHeader] = []string{token.Value}
		}
	}

	dur, err := time.ParseDuration(cfg.Duration)
	if err != nil {
		return cfg, err
	}

	cfg.ParsedDuration = dur

	keyMap, err := github.ResolveSSHKeys(ctx, cfg.AuthorizedGithubUsers)
	if err != nil {
		return cfg, err
	}

	revIndex := map[string]string{}

	for _, key := range cfg.AuthorizedKeys {
		revIndex[key] = key
	}

	for user, keys := range keyMap {
		for _, key := range keys {
			revIndex[key] = user
		}
	}

	cfg.AllKeys = revIndex
	return cfg, nil
}

type ParsedConfig struct {
	internalv1.WaitConfig

	AllKeys          map[string]string // Key ID -> Owned name
	ParsedDuration   time.Duration
	RegisterMetadata metadata.MD
}

func cancelIsOK(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

func loadJSON(filename string, target any) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}

	return json.NewDecoder(f).Decode(target)
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "internal" }
func (dummyAddr) String() string  { return "quic-revproxy" }
