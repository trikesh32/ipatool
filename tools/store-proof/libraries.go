// SPDX-License-Identifier: LGPL-2.0-only
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const apkURL = "https://apps.mzstatic.com/content/android-apple-music-apk/applemusic.apk"
const maxAPKSize = 512 << 20

var libraryHashes = map[string]map[string]string{
	"amd64": {
		"libCoreADI.so":           "ece4ffa6bbe4239e8f0d4714da57e62ce5e3c581d19553a83d78ac897783b905",
		"libstoreservicescore.so": "d8be230df34c0044758d43e33a8dabd3236b9a4258c4d48cb366911224d52f68",
	},
	"arm64": {
		"libCoreADI.so":           "9d5b557cf3faf88c34c394ab2cd7fc1a3b4acf3aed51f28cd0bc093858107c22",
		"libstoreservicescore.so": "bd335e6963ba5ae2f5babca613435ad56d2660a9aee37df35f876c458d105ce4",
	},
}

func verifyLibraries(dir string) error {
	hashes, ok := libraryHashes[runtime.GOARCH]
	if !ok {
		return errors.New("Store proof supports Linux amd64 and arm64")
	}
	for name, want := range hashes {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(f, (16<<20)+1))
		f.Close()
		if err != nil {
			return err
		}
		if n > 16<<20 || hex.EncodeToString(h.Sum(nil)) != want {
			return fmt.Errorf("Store library %s differs from the verified build", name)
		}
	}
	return nil
}

func installLibraries(dir string) error {
	if _, ok := libraryHashes[runtime.GOARCH]; !ok {
		return errors.New("unsupported Store proof architecture")
	}
	if verifyLibraries(dir) == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".applemusic-*.apk")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	fmt.Fprintln(os.Stderr, "Downloading Apple Music ADI libraries from Apple...")
	client := &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || len(via) > 5 {
			return errors.New("invalid APK redirect")
		}
		return nil
	}}
	res, err := client.Get(apkURL)
	if err != nil {
		return errors.New("Apple Music APK download failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Apple Music APK returned HTTP %d", res.StatusCode)
	}
	n, err := io.Copy(f, io.LimitReader(res.Body, maxAPKSize+1))
	if err != nil {
		return err
	}
	if n > maxAPKSize {
		return errors.New("Apple Music APK exceeds size limit")
	}
	z, err := zip.NewReader(f, n)
	if err != nil {
		return errors.New("Apple Music APK is not a ZIP archive")
	}
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64-v8a"}[runtime.GOARCH]
	data := map[string][]byte{}
	for name, want := range libraryHashes[runtime.GOARCH] {
		entry, err := z.Open("lib/" + arch + "/" + name)
		if err != nil {
			return fmt.Errorf("APK lacks %s", name)
		}
		b, err := io.ReadAll(io.LimitReader(entry, (16<<20)+1))
		entry.Close()
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		if len(b) > 16<<20 || hex.EncodeToString(h[:]) != want {
			return fmt.Errorf("Apple changed %s; update reviewed library hashes before using this APK", name)
		}
		data[name] = b
	}
	for name, b := range data {
		if err = writePrivateFile(filepath.Join(dir, name), b); err != nil {
			return err
		}
	}
	return verifyLibraries(dir)
}
