package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
	internalv1 "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/api/public/v1"
	"namespacelabs.dev/breakpoint/pkg/github"
	"namespacelabs.dev/breakpoint/pkg/githuboidc"
	"namespacelabs.dev/breakpoint/pkg/jsonfile"
)

func LoadConfig(ctx context.Context, file string) (ParsedConfig, error) {
	var cfg ParsedConfig
	if err := jsonfile.Load(file, &cfg.WaitConfig); err != nil {
		return cfg, err
	}

	for _, wh := range cfg.Webhooks {
		if wh.URL == "" {
			return cfg, errors.New("webhook is missing url")
		}
	}

	if len(cfg.Shell) == 0 {
		if sh, ok := os.LookupEnv("SHELL"); ok {
			cfg.Shell = []string{sh}
		} else {
			cfg.Shell = []string{"/bin/sh"}
		}
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
