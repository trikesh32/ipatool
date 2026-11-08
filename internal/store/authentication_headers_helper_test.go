package store

import (
	"io"
	"strings"
	"testing"
)

func TestStoreProofHelperRejectsUnexpectedOutput(t *testing.T) {
	for _, data := range []string{
		`{}`, `null`, `{"X-Apple-I-MD":"YWJj","X-Apple-I-MD-M":"ZGVm"}`,
		`{"X-Apple-AMD":"YWJj","X-Apple-AMD-M":"ZGVm","Authorization":"secret"}`,
		`{"X-Apple-AMD":"YWJj\r\nInjected: yes","X-Apple-AMD-M":"ZGVm"}`,
		`{"X-Apple-AMD":"","X-Apple-AMD-M":"ZGVm"}`,
		`{"X-Apple-AMD":"invalid","X-Apple-AMD-M":"ZGVm"}`,
		`{"X-Apple-AMD":"YWJj","X-Apple-AMD-M":123}`,
		`{"X-Apple-AMD":"YWJj","X-Apple-AMD-M":"ZGVm"} trailing`,
	} {
		if _, err := decodeStoreProof([]byte(data)); err == nil {
			t.Fatal("accepted invalid proof output")
		}
	}

	headers, err := decodeStoreProof([]byte(`{"X-Apple-AMD":"YWJj","X-Apple-AMD-M":"ZGVm"}`))

	if err != nil || len(headers) != 2 {
		t.Fatalf("valid proof rejected: %v", err)
	}
}

func TestStoreProofHelperOutputIsBounded(t *testing.T) {
	var out boundedProofOutput
	if _, err := out.Write([]byte(strings.Repeat("x", 32<<10))); err != nil {
		t.Fatal(err)
	}

	if _, err := out.Write([]byte("x")); err == nil {
		t.Fatal("accepted oversized helper output")
	}

	if out.Len() != 32<<10 {
		t.Fatal("retained oversized output")
	}

	var streamed boundedProofOutput
	// Exercise io.Copy's optional interfaces, as os/exec does for child stdout.
	if _, err := io.Copy(&streamed, strings.NewReader(strings.Repeat("x", (32<<10)+1))); err == nil {
		t.Fatal("streamed helper output bypassed the bound")
	}
}

func TestStoreProofHelperProvisioningState(t *testing.T) {
	if h, e := decodeHelperResponse([]byte(`{"provisioned":false}`)); e != nil || h != nil {
		t.Fatal("initial machine state rejected")
	}

	for _, data := range []string{`{}`, `{"headers":{}}`, `{"provisioned":false,"headers":{"X-Apple-AMD":"YWJj"}}`, `{"provisioned":true}`, `{"provisioned":true,"headers":{"X-Apple-I-MD":"YWJj"}}`} {
		if _, e := decodeHelperResponse([]byte(data)); e == nil {
			t.Fatal("invalid helper state accepted")
		}
	}

	if h, e := decodeHelperResponse([]byte(`{"provisioned":true,"headers":{"X-Apple-AMD":"YWJj","X-Apple-AMD-M":"ZGVm"}}`)); e != nil || len(h) != 2 {
		t.Fatal("valid provisioned state rejected")
	}
}

func TestStoreProofHelperIsOptInAndRequiresAbsolutePath(t *testing.T) {
	t.Setenv("IPATOOL_STORE_PROOF_HELPER", "")

	if headers, err := externalStoreProof(); err != nil || headers != nil {
		t.Fatal("unconfigured helper changed behavior")
	}

	t.Setenv("IPATOOL_STORE_PROOF_HELPER", "helper-from-path")

	if _, err := externalStoreProof(); err == nil {
		t.Fatal("accepted PATH lookup for helper")
	}
}
