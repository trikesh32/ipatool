package emulator

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/majd/ipatool/v2/internal/sap/assets"
	"github.com/majd/ipatool/v2/internal/store/profile"
)

func queryTestRuntime(t *testing.T) *Runtime {
	t.Helper()

	ctx := context.Background()

	bundle, err := assets.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}

	prepared, err := profile.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}

	guest, err := open(ctx, bundle.AppleMediaServices, prepared, true)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = guest.Close() })

	return guest
}

// Expectations were compared with native Foundation NSURLComponents.
func TestURLComponentsSetQuery(t *testing.T) {
	r := queryTestRuntime(t)

	for _, tc := range []struct {
		name, input, encoded string
		nilValue             bool
	}{
		{name: "remove", nilValue: true},
		{name: "empty"},
		{name: "reserved", input: "a=b+c&x=/?:@!$'()*+,;=[]#%2F", encoded: "a=b+c&x=/?:@!$'()*+,;=%5B%5D%23%252F"},
		{name: "unicode", input: "q=тест &x=hello world\n", encoded: "q=%D1%82%D0%B5%D1%81%D1%82%20&x=hello%20world%0A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := r.Invoke(r.ClassAddress("NSURLComponents"), "componentsWithURL:resolvingAgainstBaseURL:", r.New("NSURL", "https://example.invalid/path?old=1#anchor"), 0)
			if err != nil {
				t.Fatal(err)
			}

			var value uint64
			if !tc.nilValue {
				value = r.New("NSString", tc.input)
			}

			if _, err = r.Invoke(c, "setQuery:", value); err != nil {
				t.Fatal(err)
			}

			for selector, want := range map[string]string{"query": tc.input, "percentEncodedQuery": tc.encoded} {
				got, err := r.Invoke(c, selector)
				if err != nil {
					t.Fatal(err)
				}

				if (got == 0) != tc.nilValue || r.String(got) != want {
					t.Fatalf("%s mismatch, nil=%t value=%q", selector, got == 0, r.String(got))
				}
			}

			got, err := r.Invoke(c, "URL")
			if err != nil {
				t.Fatal(err)
			}

			want := "https://example.invalid/path"
			if !tc.nilValue {
				want += "?" + tc.encoded
			}

			want += "#anchor"
			if r.String(got) != want {
				t.Fatalf("URL = %q, want %q", r.String(got), want)
			}
		})
	}
}

func TestRedownloadHTTPFailureDoesNotCrashBridge(t *testing.T) {
	for _, status := range []int{403, 404, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			r := queryTestRuntime(t)
			calls := 0

			response, err := r.Execute(Request{Operation: "request", Method: http.MethodPost,
				URL:        "https://downloaddispatch.itunes.apple.com/r/redownload?guid=001122334455",
				HardwareID: []byte{0, 17, 34, 51, 68, 85}, Headers: http.Header{"Content-Type": []string{"application/x-apple-plist"}},
				Parameters: map[string]any{"salableAdamId": "1234", "guid": "001122334455"},
				Transport: func(req *http.Request) (*http.Response, error) {
					calls++

					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("<html>fixture failure</html>"))}, nil
				},
			})
			if response != nil {
				defer response.Body.Close()
			}

			if calls != 1 {
				t.Fatalf("transport calls=%d", calls)
			}

			if err != nil {
				t.Fatal(err)
			}

			if err == nil && (response == nil || response.StatusCode != status) {
				t.Fatal("HTTP failure lost")
			}
		})
	}
}
