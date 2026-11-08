// SPDX-License-Identifier: LGPL-2.0-only
// Linux Store machine proof helper. See NOTICE for dependency provenance.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/trikesh32/ipatool-store-proof/internal/adi"
	"howett.net/plist"
)

const storeScope int64 = -1
const storeUserAgent = "Configurator/2.21 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6"
const finishURL = "https://auth.itunes.apple.com/auth/v1/otp/1.5/finish"

type proofResponse struct {
	Provisioned bool              `json:"provisioned"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type provisionInput struct {
	Challenge string `json:"challenge"`
	GUID      string `json:"guid"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "--setup" && os.Args[1] != "--headers" && os.Args[1] != "--provision") {
		return errors.New("usage: ipatool-store-proof --setup | --headers | --provision")
	}
	dir := os.Getenv("IPATOOL_STORE_PROOF_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(base, "ipatool", "store-proof")
	}
	if !filepath.IsAbs(dir) {
		return errors.New("IPATOOL_STORE_PROOF_DIR must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) // process-local library state and adi.pb are serialized
	libDir := filepath.Join(dir, "lib")
	if os.Args[1] == "--setup" {
		if err = installLibraries(libDir); err != nil {
			return err
		}
	} else if err = verifyLibraries(libDir); err != nil {
		return errors.New("Store ADI libraries unavailable or changed; run ipatool-store-proof --setup")
	}
	identifier, err := machineIdentifier(filepath.Join(dir, "device.json"))
	if err != nil {
		return err
	}
	a, err := adi.New(libDir)
	if err != nil {
		return err
	}
	defer a.Close()
	if err = a.SetProvisioningPath(dir); err != nil {
		return err
	}
	if err = a.SetIdentifier(identifier); err != nil {
		return err
	}
	if os.Args[1] == "--provision" {
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, (32<<10)+1))
		if err != nil || len(raw) > 32<<10 {
			return errors.New("invalid provisioning input")
		}
		input, err := decodeProvisionInput(raw)
		if err != nil {
			return err
		}
		if err = provision(a, input, newHTTPClient()); err != nil {
			return err
		}
	}
	ready, err := a.IsMachineProvisioned(storeScope)
	if err != nil {
		return err
	}
	result := proofResponse{Provisioned: ready}
	if ready && os.Args[1] != "--setup" {
		otp, mid, err := a.RequestOTP(storeScope)
		if err != nil {
			return err
		}
		result.Headers = map[string]string{"X-Apple-AMD": base64.StdEncoding.EncodeToString(otp), "X-Apple-AMD-M": base64.StdEncoding.EncodeToString(mid)}
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func machineIdentifier(path string) (string, error) {
	var state struct {
		Identifier string `json:"identifier"`
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if json.Unmarshal(data, &state) != nil {
			return "", errors.New("invalid Store identity; refusing to replace it")
		}
		if b, e := hex.DecodeString(state.Identifier); e != nil || len(b) != 8 {
			return "", errors.New("invalid Store identity identifier")
		}
		return state.Identifier, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		return "", err
	}
	state.Identifier = hex.EncodeToString(random[:])
	data, err = json.Marshal(state)
	if err != nil {
		return "", err
	}
	if err = writePrivateFile(path, data); err != nil {
		return "", err
	}
	return state.Identifier, nil
}

func decodeProvisionInput(raw []byte) (provisionInput, error) {
	var input provisionInput
	if json.Unmarshal(raw, &input) != nil {
		return input, errors.New("invalid provisioning JSON")
	}
	guid, err := hex.DecodeString(input.GUID)
	if err != nil || len(guid) != 6 {
		return input, errors.New("invalid Store machine GUID")
	}
	if len(input.Challenge) == 0 || len(input.Challenge) > 16<<10 {
		return input, errors.New("invalid Store challenge length")
	}
	if data, err := base64.StdEncoding.Strict().DecodeString(input.Challenge); err != nil || len(data) == 0 {
		return input, errors.New("invalid Store challenge encoding")
	}
	return input, nil
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type adiProvisioner interface {
	StartProvisioning(int64, []byte) ([]byte, uint32, error)
	EndProvisioning(uint32, []byte, []byte) error
	DestroyProvisioning(uint32) error
}

func provision(a adiProvisioner, input provisionInput, client *http.Client) error {
	challenge, err := base64.StdEncoding.Strict().DecodeString(input.Challenge)
	if err != nil {
		return errors.New("invalid challenge")
	}
	cpim, session, err := a.StartProvisioning(storeScope, challenge)
	if err != nil {
		return fmt.Errorf("start Store ADI provisioning: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = a.DestroyProvisioning(session)
		}
	}()
	body, err := plist.Marshal(map[string]string{"clientData": base64.StdEncoding.EncodeToString(cpim), "guid": input.GUID}, plist.XMLFormat)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, finishURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", storeUserAgent)
	req.Header.Set("Content-Type", "application/x-apple-plist")
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Store provisioning network request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Store provisioning returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return errors.New("invalid Store provisioning response length")
	}
	var values struct {
		SettingInfo  string `plist:"settingInfo"`
		TransportKey string `plist:"transportKey"`
	}
	if _, err = plist.Unmarshal(raw, &values); err != nil {
		return errors.New("Store provisioning returned invalid plist")
	}
	ptm, e1 := base64.StdEncoding.Strict().DecodeString(values.SettingInfo)
	tk, e2 := base64.StdEncoding.Strict().DecodeString(values.TransportKey)
	if e1 != nil || e2 != nil || len(ptm) == 0 || len(tk) == 0 {
		return errors.New("Store provisioning returned no valid settingInfo/transportKey")
	}
	if err = a.EndProvisioning(session, ptm, tk); err != nil {
		return fmt.Errorf("finish Store ADI provisioning: %w", err)
	}
	complete = true
	return nil
}

func writePrivateFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".store-proof-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
