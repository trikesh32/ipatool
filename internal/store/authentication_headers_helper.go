package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// The opt-in helper supplies machine proof only. No Apple ID, password, cookies,
// request body or session token is passed to it. Provisioning uses Store's
// machine scope (-1), not AuthKit's scope (-2).
func externalStoreProof() (map[string]string, error) {
	return externalStoreProofContext(context.Background())
}

func externalStoreProofEnabled() bool {
	return runtime.GOOS != "darwin" && os.Getenv("IPATOOL_STORE_PROOF_HELPER") != ""
}

func externalStoreProofContext(ctx context.Context) (map[string]string, error) {
	return runStoreProofHelper(ctx, "--headers", nil)
}

func provisionExternalStoreProof(ctx context.Context, challenge, guid string) (map[string]string, error) {
	data, err := json.Marshal(map[string]string{"challenge": challenge, "guid": guid})
	if err != nil {
		return nil, fmt.Errorf("encode Store provisioning request: %w", err)
	}

	headers, err := runStoreProofHelper(ctx, "--provision", data)

	if err == nil && len(headers) == 0 {
		return nil, errors.New("store proof helper did not complete provisioning")
	}

	return headers, err
}

func runStoreProofHelper(parent context.Context, action string, input []byte) (map[string]string, error) {
	path := os.Getenv("IPATOOL_STORE_PROOF_HELPER")
	if path == "" {
		return nil, nil
	}

	if !filepath.IsAbs(path) {
		return nil, errors.New("IPATOOL_STORE_PROOF_HELPER must be an absolute executable path")
	}

	timeout := 15 * time.Second

	if action == "--provision" {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(parent, timeout)

	defer cancel()

	cmd := exec.CommandContext(ctx, path, action)
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(input)

	var output boundedProofOutput
	cmd.Stdout = &output

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("store proof helper timed out")
		}

		return nil, fmt.Errorf("store proof helper failed: %w", err)
	}

	return decodeHelperResponse(output.Bytes())
}

func decodeHelperResponse(data []byte) (map[string]string, error) {
	var result struct {
		Provisioned *bool           `json:"provisioned"`
		Headers     json.RawMessage `json:"headers"`
	}

	if json.Unmarshal(data, &result) != nil || result.Provisioned == nil {
		return nil, errors.New("store proof helper returned an invalid response")
	}

	if !*result.Provisioned {
		if len(result.Headers) != 0 {
			return nil, errors.New("unprovisioned Store helper returned unexpected headers")
		}

		return nil, nil
	}

	return decodeStoreProof(result.Headers)
}

// Do not embed bytes.Buffer: its ReaderFrom would bypass Write's size bound.
type boundedProofOutput struct{ buffer bytes.Buffer }

func (b *boundedProofOutput) Len() int      { return b.buffer.Len() }
func (b *boundedProofOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedProofOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 32<<10 {
		return 0, errors.New("store proof helper output exceeds limit")
	}

	n, err := b.buffer.Write(p)
	if err != nil {
		return n, fmt.Errorf("buffer Store proof helper output: %w", err)
	}

	return n, nil
}

func decodeStoreProof(data []byte) (map[string]string, error) {
	var headers map[string]string
	if err := json.Unmarshal(data, &headers); err != nil {
		return nil, errors.New("store proof helper returned invalid JSON")
	}

	if len(headers) != 2 {
		return nil, errors.New("store proof helper must return exactly X-Apple-AMD and X-Apple-AMD-M")
	}

	for _, key := range []string{"X-Apple-AMD", "X-Apple-AMD-M"} {
		value := headers[key]
		if value == "" || len(value) > 8192 || bytes.ContainsAny([]byte(value), "\r\n") {
			return nil, errors.New("store proof helper returned invalid headers")
		}

		if _, err := base64.StdEncoding.Strict().DecodeString(value); err != nil {
			return nil, errors.New("store proof helper returned invalid proof encoding")
		}
	}

	return headers, nil
}
