package store

import (
	"fmt"
	"net/http"
	"strings"
)

// Device proofs belong only to the exact native authentication origin.
func addNativeAuthenticationHeaders(request *http.Request, provider func(string) (map[string]string, error)) error {
	u := request.URL
	if u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "auth.itunes.apple.com") ||
		u.User != nil || u.Fragment != "" || u.RawPath != "" || (u.Port() != "" && u.Port() != "443") {
		return nil
	}
	switch u.Path {
	case "/auth/v1/native", "/auth/v1/native/", "/auth/v1/native/fast", "/auth/v1/native/fast/":
	default:
		return nil
	}
	headers, err := provider(u.String())
	if err != nil {
		return fmt.Errorf("create native Store device proof: %w", err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	return nil
}
