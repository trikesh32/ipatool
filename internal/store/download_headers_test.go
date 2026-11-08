package store

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDownloadAnisetteTransportRouting(t *testing.T) {
	for _, target := range []string{
		"https://buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct",
		"https://buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/buyProduct",
		"https://p42-buy.itunes.apple.com/WebObjects/MZBuy.woa/wa/buyProduct",
		"https://p42-buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct?guid=fixture",
		"https://downloaddispatch.itunes.apple.com/WebObjects/DownloadDispatch.woa/wa/ent/download",
		"https://downloaddispatch.itunes.apple.com/r/redownload",
		"https://downloaddispatch.itunes.apple.com/up/updateProduct",
	} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			provider := func(string) (map[string]string, error) {
				calls++

				return map[string]string{"X-Apple-AMD": "fixture-store-otp", "X-Apple-AMD-M": "fixture-store-machine"}, nil
			}
			r, _ := http.NewRequest(http.MethodPost, target, strings.NewReader("fixture-payload"))
			r.Header.Set("X-Dsid", "fixture-dsid")

			if err := addNativeAuthenticationHeaders(r, provider); err != nil {
				t.Fatal(err)
			}

			body, _ := io.ReadAll(r.Body)
			if calls != 1 || r.Header.Get("X-Apple-AMD") != "fixture-store-otp" || string(body) != "fixture-payload" || r.Header.Get("X-Dsid") != "fixture-dsid" {
				t.Fatal("download proof missing or request changed")
			}
		})
	}

	for _, target := range []string{
		"https://cdn.example.invalid/Overcast.ipa",
		"https://itunes.apple.com/lookup?id=fixture",
		"http://buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct",
		"https://p42-buy.itunes.apple.com.evil.invalid/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct",
		"https://evil-buy.itunes.apple.com/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct",
		"https://auth.itunes.apple.com/WebObjects/MZFinance.woa/wa/volumeStoreDownloadProduct",
		"https://user@downloaddispatch.itunes.apple.com/r/redownload",
		"https://downloaddispatch.itunes.apple.com:444/r/redownload",
		"https://downloaddispatch.itunes.apple.com/r/%72edownload",
		"https://downloaddispatch.itunes.apple.com/r/redownload#fragment",
		"https://downloaddispatch.itunes.apple.com/other",
	} {
		t.Run(target, func(t *testing.T) {
			u, _ := url.Parse(target)
			if isNativeDownloadURL(u) {
				t.Fatal("unexpected trusted download endpoint")
			}

			r, _ := http.NewRequest(http.MethodPost, target, nil)
			if err := addNativeAuthenticationHeaders(r, func(string) (map[string]string, error) {
				t.Fatal("provider called for non-commerce request")

				return nil, nil
			}); err != nil {
				t.Fatal(err)
			}

			if len(r.Header) != 0 {
				t.Fatal("proof leaked to unrelated request")
			}
		})
	}
}
