package httperrors

import (
	"fmt"
	"io"
	"net/http"
)

type HttpError struct {
	StatusCode  int
	ServerError string
}

func (he HttpError) Error() string {
	if len(he.ServerError) > 0 {
		return fmt.Sprintf("request failed with %s, got from the server:\n%s", http.StatusText(he.StatusCode), he.ServerError)
	}

	return fmt.Sprintf("request failed with %s", http.StatusText(he.StatusCode))
}

func MaybeError(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return &HttpError{StatusCode: resp.StatusCode, ServerError: string(bodyBytes)}
	}

	return nil
}
