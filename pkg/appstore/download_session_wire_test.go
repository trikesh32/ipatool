package appstore

import (
	"context"
	"io"
	stdhttp "net/http"
	"strings"
	"testing"

	"github.com/majd/ipatool/v2/internal/sap/assets"
	"github.com/majd/ipatool/v2/internal/store/emulator"
	"github.com/majd/ipatool/v2/internal/store/profile"
	apphttp "github.com/majd/ipatool/v2/pkg/http"
)

func TestDownloadSessionReachesFrameworkWire(t *testing.T) {
	ctx := context.Background()

	bundle, err := assets.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}

	p, err := profile.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}

	for _, endpoint := range []downloadProductEndpoint{
		(&appstore{}).volumeStoreEndpoint(Account{Pod: "42"}),
		{baseURL: "https://downloaddispatch.itunes.apple.com/r/redownload", versionKey: downloadVersionKeyRedownload},
	} {
		t.Run(endpoint.baseURL, func(t *testing.T) {
			guest, err := emulator.Open(ctx, bundle.AppleMediaServices, p)
			if err != nil {
				t.Fatal(err)
			}

			defer guest.Close()

			account := Account{DirectoryServicesID: "123456", PasswordToken: "fixture-session-token", StoreFront: "143441-1,29"}

			request := (&appstore{}).downloadProductRequest(endpoint, account, App{ID: 1234}, "001122334455", "")
			if !request.NoRedirects {
				t.Error("credential-bearing request permits automatic redirects")
			}

			headers := stdhttp.Header{}
			for k, v := range request.Headers {
				headers.Set(k, v)
			}

			calls := 0

			response, err := guest.Execute(emulator.Request{Operation: "request", URL: request.URL, Method: request.Method, Headers: headers,
				HardwareID: []byte{0, 17, 34, 51, 68, 85}, Parameters: request.Payload.(*apphttp.XMLPayload).Content, NoRedirects: request.NoRedirects,
				Transport: func(wire *stdhttp.Request) (*stdhttp.Response, error) {
					calls++
					for k, want := range map[string]string{"X-Token": account.PasswordToken, "X-Dsid": account.DirectoryServicesID, "X-Apple-Store-Front": account.StoreFront} {
						if wire.Header.Get(k) != want {
							t.Errorf("%s did not reach the wire", k)
						}
					}

					return &stdhttp.Response{StatusCode: 200, Header: stdhttp.Header{"Content-Type": []string{"application/x-apple-plist"}}, Body: io.NopCloser(strings.NewReader(`<plist><dict><key>songList</key><array/></dict></plist>`))}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			defer response.Body.Close()

			if calls != 1 {
				t.Fatal("unexpected request count")
			}
		})
	}
}
