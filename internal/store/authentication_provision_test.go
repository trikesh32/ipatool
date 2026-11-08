package store

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func challengeResponse(status int, challenge string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"X-Apple-Amd-Action": []string{"SP"}, "X-Apple-Amd-Data": []string{challenge}}, Body: io.NopCloser(strings.NewReader("fixture challenge"))}
}

func TestStoreProvisioningPreservesSignedRequestAnd2FA(t *testing.T) {
	const body = `<plist>fixture-password-and-2fa</plist>`
	req, _ := http.NewRequest(http.MethodPost, "https://auth.itunes.apple.com/auth/v1/native?Pod=42", strings.NewReader(body))
	req.Header.Set("X-Apple-ActionSignature", "fixture-signature")
	req.Header.Set("Cookie", "session=fixture")

	count := 0
	send := func(r *http.Request) (*http.Response, error) {
		count++
		data, _ := io.ReadAll(r.Body)
		r.Body.Close()

		if string(data) != body || r.URL.String() != req.URL.String() || r.Header.Get("X-Apple-ActionSignature") != "fixture-signature" || r.Header.Get("Cookie") != "session=fixture" {
			t.Fatal("signed login context changed")
		}

		if count == 1 {
			return challengeResponse(200, "YWJj"), nil
		}

		if count != 2 || r.Header.Get("X-Apple-AMD") != "ZGVm" || r.Header.Get("X-Apple-AMD-M") != "Z2hp" {
			t.Fatal("retry proof missing or unbounded retry")
		}

		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("success"))}, nil
	}
	provision := func(_ context.Context, challenge, guid string) (map[string]string, error) {
		if challenge != "YWJj" || guid != "021122334455" {
			t.Fatal("helper input contains unexpected data")
		}

		return map[string]string{"X-Apple-AMD": "ZGVm", "X-Apple-AMD-M": "Z2hp"}, nil
	}
	res, err := sendWithStoreProvisioning(req, "021122334455", send, provision)

	if err != nil {
		t.Fatal(err)
	}

	res.Body.Close()

	if count != 2 {
		t.Fatal("request was not retried once")
	}
}

func TestStoreProvisioningRejectsForeignChallengesAndRedirects(t *testing.T) {
	for _, tc := range []struct {
		endpoint, method string
		status           int
	}{
		{"https://evil.invalid/auth/v1/native", http.MethodPost, 200},
		{"http://auth.itunes.apple.com/auth/v1/native", http.MethodPost, 200},
		{"https://auth.itunes.apple.com/auth/v1/native", http.MethodGet, 200},
		{"https://auth.itunes.apple.com/auth/v1/native", http.MethodPost, 302},
	} {
		req, _ := http.NewRequest(tc.method, tc.endpoint, strings.NewReader("fixture"))
		res, err := sendWithStoreProvisioning(req, "021122334455", func(*http.Request) (*http.Response, error) { return challengeResponse(tc.status, "YWJj"), nil }, func(context.Context, string, string) (map[string]string, error) {
			t.Fatal("untrusted challenge reached helper")

			return nil, nil
		})

		if err != nil {
			t.Fatal(err)
		}

		res.Body.Close()
	}
}

func TestStoreProvisioningIsBoundedAndPropagatesFailure(t *testing.T) {
	for _, challenge := range []string{"", "%%%", strings.Repeat("a", (16<<10)+1)} {
		req, _ := http.NewRequest(http.MethodPost, "https://auth.itunes.apple.com/auth/v1/native", strings.NewReader("fixture"))
		_, err := sendWithStoreProvisioning(req, "021122334455", func(*http.Request) (*http.Response, error) { return challengeResponse(200, challenge), nil }, func(context.Context, string, string) (map[string]string, error) {
			t.Fatal("invalid challenge reached helper")

			return nil, nil
		})

		if err == nil {
			t.Fatal("accepted invalid challenge")
		}
	}

	req, _ := http.NewRequest(http.MethodPost, "https://auth.itunes.apple.com/auth/v1/native", strings.NewReader("fixture"))
	count := 0

	send := func(*http.Request) (*http.Response, error) {
		count++

		return challengeResponse(http.StatusOK, "YWJj"), nil
	}
	_, err := sendWithStoreProvisioning(req, "021122334455", send, func(context.Context, string, string) (map[string]string, error) {
		return map[string]string{"X-Apple-AMD": "YWJj", "X-Apple-AMD-M": "ZGVm"}, nil
	})

	if err == nil || count != 2 {
		t.Fatal("repeated SP was not stopped")
	}

	failure := errors.New("helper failure")
	_, err = sendWithStoreProvisioning(req, "021122334455", send, func(context.Context, string, string) (map[string]string, error) { return nil, failure })

	if !errors.Is(err, failure) {
		t.Fatal("helper failure lost")
	}
}
