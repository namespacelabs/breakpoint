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
		for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
			keys = append(keys, strings.TrimSpace(line))
		}

		m[username] = keys

		zerolog.Ctx(ctx).Info().Str("username", username).Dur("took", time.Since(t)).Msg("Resolved keys")
	}

	return m, nil
}
