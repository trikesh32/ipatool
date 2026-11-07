//go:build !darwin

package store

func platformAuthenticationHeaders(string) (map[string]string, error) { return nil, nil }
