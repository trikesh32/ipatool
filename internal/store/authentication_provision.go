package store

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
)

type storeProofProvisioner func(context.Context, string, string) (map[string]string, error)

// Retry the identical SAP-signed request once after an origin-bound SP challenge.
// The helper receives only the machine challenge and GUID, never account data.
func sendWithStoreProvisioning(request *http.Request, guid string, send func(*http.Request) (*http.Response, error), provision storeProofProvisioner) (*http.Response, error) {
	res, err := send(request)
	if err != nil || !needsStoreProof(request) || res.StatusCode != http.StatusOK || res.Header.Get("X-Apple-AMD-Action") != "SP" {
		return res, err
	}

	defer res.Body.Close()
	challenge := res.Header.Get("X-Apple-AMD-Data")

	if len(challenge) == 0 || len(challenge) > 16<<10 {
		return nil, errors.New("invalid Store provisioning challenge from Apple")
	}

	if b, e := base64.StdEncoding.Strict().DecodeString(challenge); e != nil || len(b) == 0 {
		return nil, errors.New("invalid Store provisioning challenge from Apple")
	}

	if request.Body != nil && request.GetBody == nil {
		return nil, errors.New("cannot replay Store request after provisioning")
	}

	headers, err := provision(request.Context(), challenge, guid)

	if err != nil {
		return nil, fmt.Errorf("provision Store machine proof: %w", err)
	}

	if headers["X-Apple-AMD"] == "" || headers["X-Apple-AMD-M"] == "" {
		return nil, errors.New("store provisioning returned incomplete device proof")
	}

	retry := request.Clone(request.Context())

	if request.GetBody != nil {
		retry.Body, err = request.GetBody()
		if err != nil {
			return nil, fmt.Errorf("replay Store request: %w", err)
		}
	}

	for _, key := range []string{"X-Apple-AMD", "X-Apple-AMD-M"} {
		retry.Header.Set(key, headers[key])
	}

	res, err = send(retry)

	if err == nil && res.Header.Get("X-Apple-AMD-Action") == "SP" {
		res.Body.Close()

		return nil, errors.New("provisioned Store device proof rejected by Apple; automatic provisioning was limited to one attempt")
	}

	return res, err
}
