package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dustin/go-humanize"
	"github.com/muesli/reflow/wordwrap"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"namespacelabs.dev/breakpoint/pkg/config"
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

	configPath := cmd.Flags().String("config", "", "Path to the configuration file.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *configPath == "" {
			return errors.New("--config is required")
		}

		ctx := cmd.Context()

		cfg, err := config.LoadConfig(ctx, *configPath)
		if err != nil {
			return err
		}

		mopts := waiter.ManagerOpts{
			InitialDur: cfg.ParsedDuration,
			Webhooks:   cfg.Webhooks,
		}

		if cfg.SlackBot != nil {
			mopts.SlackBots = append(mopts.SlackBots, *cfg.SlackBot)
		}

		mgr, ctx := waiter.NewManager(ctx, mopts)

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
			WriteNotify: func() {
				if cfg.ParsedDurationAutoExtend > 0 {
					mgr.ExtendWait(cfg.ParsedDurationAutoExtend, false)
				}
			},
		})
		if err != nil {
			return err
		}

		mgr.SetConnectionCountCallback(sshd.NumConnections)

		eg, ctx := errgroup.WithContext(ctx)

		pl := passthrough.NewListener(ctx, dummyAddr{})

		eg.Go(func() error {
			return sshd.Server.Serve(pl)
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

func cancelIsOK(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "internal" }
func (dummyAddr) String() string  { return "quic-revproxy" }
