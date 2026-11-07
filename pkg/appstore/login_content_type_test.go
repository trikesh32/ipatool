package appstore

import (
	"testing"

	"github.com/majd/ipatool/v2/pkg/http"
)

func TestLoginDeclaresPlistPayload(t *testing.T) {
	req := (&appstore{}).loginRequest("test@example.invalid", "test-password", "", "001122334455", testAuthEndpoint, 1, nil)
	if !req.NoRedirects {
		t.Fatal("authentication redirects must use the App Store endpoint allowlist")
	}
	if _, ok := req.Payload.(*http.XMLPayload); !ok {
		t.Fatal("authentication must submit a plist payload")
	}
	if got := req.Headers["Content-Type"]; got != "application/x-apple-plist" {
		t.Fatalf("plist authentication payload declared as %q", got)
	}
}

func TestNativeAuthenticationRedirect(t *testing.T) {
	const base = "https://auth.itunes.apple.com/auth/v1/native"
	got, err := resolveAuthenticationRedirect(base, "/auth/v1/native/?Pod=7")
	if err != nil || got != "https://auth.itunes.apple.com/auth/v1/native/?Pod=7" {
		t.Fatalf("native redirect: got %q, error %v", got, err)
	}
	for _, target := range []string{"https://auth.itunes.apple.com.example.invalid/auth/v1/native", "http://auth.itunes.apple.com/auth/v1/native", "/auth/v1/native/other"} {
		if _, err := resolveAuthenticationRedirect(base, target); err == nil {
			t.Fatal("untrusted native redirect accepted")
		}
	}
}
