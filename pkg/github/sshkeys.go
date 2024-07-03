package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

func ResolveSSHKeys(ctx context.Context, usernames []string) (map[string][]string, error) {
	// Fetch in sequence to minimize how many requests in parallel we issue to GitHub.

	m := map[string][]string{}
	for _, username := range usernames {
		t := time.Now()

		keys, err := fetchKeys(username)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch SSH keys for GitHub user %q: %w", username, err)
		}

		if len(keys) == 0 {
			zerolog.Ctx(ctx).Warn().Str("username", username).Dur("took", time.Since(t)).Msg("No keys found")
			continue
		}

		m[username] = keys

		zerolog.Ctx(ctx).Info().Str("username", username).Dur("took", time.Since(t)).Msg("Resolved keys")
	}

	return m, nil
}

func fetchKeys(username string) ([]string, error) {
	resp, err := http.Get(fmt.Sprintf("https://github.com/%s.keys", username))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var keys []string
	for _, line := range strings.FieldsFunc(strings.TrimSpace(string(contents)), func(r rune) bool { return r == '\n' }) {
		keys = append(keys, strings.TrimSpace(line))
	}

	return keys, nil
}
