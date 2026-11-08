package store

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Device proofs belong only to native authentication and known commerce API origins.
func addNativeAuthenticationHeaders(request *http.Request, provider func(string) (map[string]string, error)) error {
	u := request.URL

	if request.Method != http.MethodPost {
		return nil
	}

	if isNativeDownloadURL(u) || isNativePurchaseURL(u) {
		return applyNativeHeaders(request, provider)
	}

	if u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "auth.itunes.apple.com") ||
		u.User != nil || u.Fragment != "" || u.RawPath != "" || (u.Port() != "" && u.Port() != "443") {
		return nil
	}

	switch u.Path {
	case "/auth/v1/native", "/auth/v1/native/", "/auth/v1/native/fast", "/auth/v1/native/fast/":
	default:
		return nil
	}

	return applyNativeHeaders(request, provider)
}

func isNativePurchaseURL(value *url.URL) bool {
	if value == nil || (value.Path != "/WebObjects/MZFinance.woa/wa/buyProduct" && value.Path != "/WebObjects/MZBuy.woa/wa/buyProduct") {
		return false
	}

	probe := *value
	probe.Path = "/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct"

	return isNativeDownloadURL(&probe)
}

func applyNativeHeaders(request *http.Request, provider func(string) (map[string]string, error)) error {
	headers, err := provider(request.URL.String())
	if err != nil {
		return fmt.Errorf("create native Store device proof: %w", err)
	}

	for key, value := range headers {
		request.Header.Set(key, value)
	}

	return nil
}

// Only commerce metadata endpoints need device proof; CDN package downloads,
// artwork, catalog lookups and unrelated requests must not receive it.
func isNativeDownloadURL(value *url.URL) bool {
	if value == nil || value.Scheme != "https" || value.User != nil || value.Opaque != "" ||
		value.Fragment != "" || value.RawPath != "" || (value.Port() != "" && value.Port() != "443") {
		return false
	}

	if strings.EqualFold(value.Hostname(), "downloaddispatch.itunes.apple.com") {
		switch value.Path {
		case "/WebObjects/DownloadDispatch.woa/wa/ent/download", "/r/redownload", "/up/updateProduct":
			return true
		}

		return false
	}

	if value.Path != "/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct" {
		return false
	}
	// Accept only buy/pNN-buy hosts, without broad suffix checks.
	host := strings.ToLower(value.Hostname())
	if host == "buy.itunes.apple.com" {
		return true
	}

	pod, ok := strings.CutSuffix(host, "-buy.itunes.apple.com")
	if !ok || len(pod) < 2 || pod[0] != 'p' {
		return false
	}

	for _, c := range pod[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}
