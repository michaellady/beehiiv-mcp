package main

import (
	"errors"
	"testing"
)

type fakeCredStore struct {
	items map[string][]byte
}

func newFakeCredStore() *fakeCredStore {
	return &fakeCredStore{items: map[string][]byte{}}
}

func (f *fakeCredStore) key(service, account string) string {
	return service + "\x00" + account
}

func (f *fakeCredStore) Get(service, account string) ([]byte, error) {
	v, ok := f.items[f.key(service, account)]
	if !ok {
		return nil, errNotFound
	}
	return v, nil
}

func (f *fakeCredStore) Set(service, account string, data []byte) error {
	f.items[f.key(service, account)] = data
	return nil
}

func (f *fakeCredStore) Delete(service, account string) error {
	delete(f.items, f.key(service, account))
	return nil
}

func TestResolveCredentials_KeychainTakesPrecedence(t *testing.T) {
	store := newFakeCredStore()
	_ = store.Set(keychainService, apiKeyAccount, []byte("kc-api-key"))
	_ = store.Set(keychainService, publicationIDAccount, []byte("kc-pub-id"))

	t.Setenv("BEEHIIV_API_KEY", "env-api-key")
	t.Setenv("BEEHIIV_PUBLICATION_ID", "env-pub-id")

	creds, err := resolveCredentials(store)
	if err != nil {
		t.Fatalf("resolveCredentials: %v", err)
	}
	if creds.APIKey != "kc-api-key" {
		t.Errorf("APIKey = %q, want kc-api-key", creds.APIKey)
	}
	if creds.PublicationID != "kc-pub-id" {
		t.Errorf("PublicationID = %q, want kc-pub-id", creds.PublicationID)
	}
	if creds.Source != sourceKeychain {
		t.Errorf("Source = %q, want %q", creds.Source, sourceKeychain)
	}
}

func TestResolveCredentials_FallsBackToEnvVars(t *testing.T) {
	store := newFakeCredStore() // empty

	t.Setenv("BEEHIIV_API_KEY", "env-api-key")
	t.Setenv("BEEHIIV_PUBLICATION_ID", "env-pub-id")

	creds, err := resolveCredentials(store)
	if err != nil {
		t.Fatalf("resolveCredentials: %v", err)
	}
	if creds.APIKey != "env-api-key" {
		t.Errorf("APIKey = %q, want env-api-key", creds.APIKey)
	}
	if creds.PublicationID != "env-pub-id" {
		t.Errorf("PublicationID = %q, want env-pub-id", creds.PublicationID)
	}
	if creds.Source != sourceEnv {
		t.Errorf("Source = %q, want %q", creds.Source, sourceEnv)
	}
}

func TestResolveCredentials_MixedSourcesPreferKeychainPerField(t *testing.T) {
	// Only API key is in keychain; publication ID is only in env. Each field resolves independently.
	store := newFakeCredStore()
	_ = store.Set(keychainService, apiKeyAccount, []byte("kc-api-key"))

	t.Setenv("BEEHIIV_API_KEY", "env-api-key")
	t.Setenv("BEEHIIV_PUBLICATION_ID", "env-pub-id")

	creds, err := resolveCredentials(store)
	if err != nil {
		t.Fatalf("resolveCredentials: %v", err)
	}
	if creds.APIKey != "kc-api-key" {
		t.Errorf("APIKey = %q, want kc-api-key", creds.APIKey)
	}
	if creds.PublicationID != "env-pub-id" {
		t.Errorf("PublicationID = %q, want env-pub-id", creds.PublicationID)
	}
	if creds.Source != sourceMixed {
		t.Errorf("Source = %q, want %q", creds.Source, sourceMixed)
	}
}

func TestResolveCredentials_BothMissingReturnsSetupError(t *testing.T) {
	store := newFakeCredStore() // empty

	t.Setenv("BEEHIIV_API_KEY", "")
	t.Setenv("BEEHIIV_PUBLICATION_ID", "")

	_, err := resolveCredentials(store)
	if err == nil {
		t.Fatal("resolveCredentials should error when no credentials are available")
	}
	if !errors.Is(err, errMissingCredentials) {
		t.Errorf("error = %v, want wraps errMissingCredentials", err)
	}
	msg := err.Error()
	if msg == "" || !containsAll(msg, "beehiiv-mcp auth set", "BEEHIIV_API_KEY") {
		t.Errorf("error message should reference setup command and env var fallback; got: %s", msg)
	}
}

func TestResolveCredentials_PartialMissingReportsWhichFieldIsMissing(t *testing.T) {
	// API key present via env, publication ID missing entirely.
	store := newFakeCredStore()

	t.Setenv("BEEHIIV_API_KEY", "env-api-key")
	t.Setenv("BEEHIIV_PUBLICATION_ID", "")

	_, err := resolveCredentials(store)
	if err == nil {
		t.Fatal("resolveCredentials should error when publication ID is missing")
	}
	if !errors.Is(err, errMissingCredentials) {
		t.Errorf("error = %v, want wraps errMissingCredentials", err)
	}
	if !containsAll(err.Error(), "publication") {
		t.Errorf("error should mention missing publication ID; got: %s", err.Error())
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
