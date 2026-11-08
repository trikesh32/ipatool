//go:build !darwin

package http

import "os"

var DefaultUserAgent = storeProofUserAgent("Configurator/2.17 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6")

var CommerceUserAgent = storeProofUserAgent("Configurator/2.18 (Macintosh; OS X 15.3.2; 24D81) AppleWebKit/0620.2.4.11.6")

func storeProofUserAgent(legacy string) string {
	if os.Getenv("IPATOOL_STORE_PROOF_HELPER") != "" {
		return "Configurator/2.21 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6"
	}
	return legacy
}
