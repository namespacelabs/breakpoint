package githuboidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"namespacelabs.dev/breakpoint/pkg/httperrors"
)

var ErrMissingIdTokenWrite = errors.New("please add `id-token: write` to your workflow permissions")

const (
	userAgent = "actions/oidc-client"
)

type Token struct {
	Value string `json:"value"`
}

func OIDCAvailable() bool {
	x, y := oidcConf()
	return x != "" && y != ""
}

func JWT(ctx context.Context, audience string) (*Token, error) {
	idTokenURL, idToken := oidcConf()
	if idTokenURL == "" || idToken == "" {
		return nil, ErrMissingIdTokenWrite
	}

	if audience != "" {
		idTokenURL += fmt.Sprintf("&audience=%s", url.QueryEscape(audience))
	}

	req, err := http.NewRequestWithContext(ctx, "GET", idTokenURL, nil)
	if err != nil {
		return nil, fmt.Errorf("github/oidc: failed to create HTTP request: %w", err)
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("User-Agent", userAgent)
	req.Header.Add("Authorization", "Bearer "+idToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github/oidc: failed to request github JWT: %w", err)
	}

	defer resp.Body.Close()

	if err := httperrors.MaybeError(resp); err != nil {
		return nil, fmt.Errorf("github/oidc: failed to obtain token: %v", err)
	}

	var token Token
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, fmt.Errorf("github/oidc: bad response: %w", err)
	}

	return &token, nil
}

func oidcConf() (string, string) {
	idTokenURL := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	idToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")

	return idTokenURL, idToken
}
