package store

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
)

func TestNativeAuthenticationHeadersAreOriginBound(t *testing.T) {
	for _, endpoint := range []string{
		"http://auth.itunes.apple.com/auth/v1/native",
		"https://auth.itunes.apple.com.example.invalid/auth/v1/native",
		"https://example.invalid/auth/v1/native",
		"https://user@auth.itunes.apple.com/auth/v1/native",
		"https://auth.itunes.apple.com:8443/auth/v1/native",
		"https://auth.itunes.apple.com/auth/v1/native%2ffast",
		"https://auth.itunes.apple.com/other",
		"https://auth.itunes.apple.com/auth/v1/native#fragment",
	} {
		t.Run(endpoint, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, endpoint, nil)
			if err != nil {
				t.Fatal(err)
			}

			err = addNativeAuthenticationHeaders(req, func(string) (map[string]string, error) {
				t.Fatal("device proof requested for an unrelated origin")

				return nil, nil
			})
			if err != nil || len(req.Header) != 0 {
				t.Fatal("unrelated request changed")
			}
		})
	}
}

func TestNativeAuthenticationHeadersRefreshAndErrors(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "https://auth.itunes.apple.com/auth/v1/native", nil)
	calls := 0

	provider := func(endpoint string) (map[string]string, error) {
		if endpoint != req.URL.String() {
			t.Fatal("provider received different URL")
		}

		calls++

		return map[string]string{"X-Apple-AMD": strconv.Itoa(calls), "X-Apple-AMD-M": "synthetic-machine"}, nil
	}
	for i := 1; i <= 2; i++ {
		if err := addNativeAuthenticationHeaders(req, provider); err != nil {
			t.Fatal(err)
		}

		if req.Header.Get("X-Apple-AMD") != strconv.Itoa(i) || req.Header.Get("X-Apple-AMD-M") != "synthetic-machine" {
			t.Fatal("missing or stale device proof")
		}
	}

	failure := errors.New("native unavailable")
	if err := addNativeAuthenticationHeaders(req, func(string) (map[string]string, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatal("device proof failure was lost")
	}
}
