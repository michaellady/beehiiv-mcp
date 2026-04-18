package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// credStore abstracts credential storage. Real implementation wraps
// keybase/go-keychain on darwin; tests pass an in-memory fake.
type credStore interface {
	Get(service, account string) ([]byte, error)
	Set(service, account string, data []byte) error
	Delete(service, account string) error
}

// Keychain service + account identifiers.
const (
	keychainService      = "beehiiv-mcp"
	apiKeyAccount        = "api_key"
	publicationIDAccount = "publication_id"
)

// Env-var names used as fallback when Keychain lookup fails.
const (
	envAPIKey        = "BEEHIIV_API_KEY"
	envPublicationID = "BEEHIIV_PUBLICATION_ID"
)

// Source labels reported back in the resolved Credentials.
const (
	sourceKeychain = "keychain"
	sourceEnv      = "env"
	sourceMixed    = "mixed"
)

// Sentinel errors. errNotFound is returned by credStore implementations when
// an item is absent; callers must treat it as a lookup miss, not a hard error.
var (
	errNotFound           = errors.New("credential not found")
	errMissingCredentials = errors.New("missing beehiiv credentials")
)

// Credentials holds resolved API access values and the source they came from.
type Credentials struct {
	APIKey        string
	PublicationID string
	Source        string // "keychain", "env", or "mixed"
}

// resolveCredentials looks up API key and publication ID, preferring Keychain
// over env vars for each field independently. Returns errMissingCredentials
// (wrapped with a setup-instructing message) if either field is unresolved.
func resolveCredentials(store credStore) (Credentials, error) {
	apiKey, apiSource := lookup(store, apiKeyAccount, envAPIKey)
	pubID, pubSource := lookup(store, publicationIDAccount, envPublicationID)

	var missing []string
	if apiKey == "" {
		missing = append(missing, "API key (keychain or $"+envAPIKey+")")
	}
	if pubID == "" {
		missing = append(missing, "publication ID (keychain or $"+envPublicationID+")")
	}
	if len(missing) > 0 {
		return Credentials{}, fmt.Errorf(
			"%w: %s. Run `beehiiv-mcp auth set` to store credentials in macOS Keychain, or export $%s and $%s for dev use",
			errMissingCredentials,
			strings.Join(missing, ", "),
			envAPIKey,
			envPublicationID,
		)
	}

	return Credentials{
		APIKey:        apiKey,
		PublicationID: pubID,
		Source:        combineSources(apiSource, pubSource),
	}, nil
}

// lookup returns (value, source). Source is "" when neither keychain nor env yields a value.
func lookup(store credStore, account, envName string) (string, string) {
	if store != nil {
		if v, err := store.Get(keychainService, account); err == nil && len(v) > 0 {
			return string(v), sourceKeychain
		}
	}
	if v := os.Getenv(envName); v != "" {
		return v, sourceEnv
	}
	return "", ""
}

func combineSources(a, b string) string {
	switch {
	case a == sourceKeychain && b == sourceKeychain:
		return sourceKeychain
	case a == sourceEnv && b == sourceEnv:
		return sourceEnv
	default:
		return sourceMixed
	}
}
