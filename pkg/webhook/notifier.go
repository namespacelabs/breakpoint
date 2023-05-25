package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"namespacelabs.dev/breakpoint/pkg/httperrors"
)

const (
	userAgent = "Breakpoint/1.0"
)

func Notify(ctx context.Context, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	return httperrors.MaybeError(resp)
}
