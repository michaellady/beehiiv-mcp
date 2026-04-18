//go:build !darwin

package main

import "fmt"

// macOSKeychain on non-darwin platforms always errors on writes/reads. The
// caller is expected to fall back to environment variables via the lookup
// chain in credentials.go.
type macOSKeychain struct{}

func newMacOSKeychain() credStore { return macOSKeychain{} }

func (macOSKeychain) Get(service, account string) ([]byte, error) {
	return nil, errNotFound
}

func (macOSKeychain) Set(service, account string, data []byte) error {
	return fmt.Errorf("macOS Keychain not available on this platform; use env vars %s and %s instead", envAPIKey, envPublicationID)
}

func (macOSKeychain) Delete(service, account string) error {
	return nil
}
