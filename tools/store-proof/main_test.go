// SPDX-License-Identifier: LGPL-2.0-only
package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"howett.net/plist"
)

type fixtureADI struct{ started, finished, destroyed bool }

func (f *fixtureADI) StartProvisioning(scope int64, b []byte) ([]byte, uint32, error) {
	if scope != -1 || string(b) != "server-challenge" {
		return nil, 0, errors.New("wrong Store scope or challenge")
	}
	f.started = true
	return []byte("client-data"), 7, nil
}
func (f *fixtureADI) EndProvisioning(session uint32, ptm, tk []byte) error {
	if session != 7 || string(ptm) != "setting-info" || string(tk) != "transport-key" {
		return errors.New("wrong finish inputs")
	}
	f.finished = true
	return nil
}
func (f *fixtureADI) DestroyProvisioning(session uint32) error {
	f.destroyed = session == 7
	return nil
}

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStoreProvisioningProtocol(t *testing.T) {
	a := &fixtureADI{}
	client := newHTTPClient()
	client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != finishURL || r.Method != "POST" {
			t.Fatal("wrong provisioning endpoint")
		}
		if r.Header.Get("Content-Type") != "application/x-apple-plist" || r.Header.Get("User-Agent") != storeUserAgent {
			t.Fatal("wrong Store request format")
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("account credentials in provisioning request")
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]string
		if _, err := plist.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 2 || body["guid"] != "021122334455" || body["clientData"] != base64.StdEncoding.EncodeToString([]byte("client-data")) {
			t.Fatal("wrong Store provisioning fields")
		}
		data, _ := plist.Marshal(map[string]string{"settingInfo": base64.StdEncoding.EncodeToString([]byte("setting-info")), "transportKey": base64.StdEncoding.EncodeToString([]byte("transport-key"))}, plist.XMLFormat)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})
	err := provision(a, provisionInput{GUID: "021122334455", Challenge: base64.StdEncoding.EncodeToString([]byte("server-challenge"))}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !a.started || !a.finished || a.destroyed {
		t.Fatal("wrong ADI lifecycle")
	}
}

func TestStoreProvisioningDoesNotFollowRedirectOrKeepFailedSession(t *testing.T) {
	a := &fixtureADI{}
	calls := 0
	client := newHTTPClient()
	client.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://example.invalid/collect"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	err := provision(a, provisionInput{GUID: "021122334455", Challenge: base64.StdEncoding.EncodeToString([]byte("server-challenge"))}, client)
	if err == nil || calls != 1 || a.finished || !a.destroyed {
		t.Fatal("redirect followed or failed session not destroyed")
	}
}

func TestProvisioningInputValidation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"guid":"../../","challenge":"YWJj"}`, `{"guid":"021122334455","challenge":"%%%"}`, `{"guid":"021122334455","challenge":""}`} {
		if _, err := decodeProvisionInput([]byte(raw)); err == nil {
			t.Fatal("accepted invalid provisioning input")
		}
	}
	if _, err := decodeProvisionInput([]byte(`{"guid":"021122334455","challenge":"YWJj"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestIdentityPersistsAndCorruptionIsNotSilentlyReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	first, err := machineIdentifier(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := machineIdentifier(path)
	if err != nil || first != second {
		t.Fatal("machine identity changed")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("identity file is not private")
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := machineIdentifier(path); err == nil {
		t.Fatal("corrupt identity replaced")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "corrupt" {
		t.Fatal("corrupt identity modified")
	}
}
