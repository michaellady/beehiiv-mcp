package main

import (
	"bytes"
	"strings"
	"testing"
)

func newAuthDeps(store credStore, stdin string) (*authDeps, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &authDeps{
		Store:    store,
		In:       strings.NewReader(stdin),
		Out:      out,
		Getenv:   func(k string) string { return "" },
		Platform: "darwin",
	}, out
}

func TestRunAuth_SetStoresBothSecretsAndReportsSuccess(t *testing.T) {
	store := newFakeCredStore()
	deps, out := newAuthDeps(store, "secret-key\npub-123\n")

	if err := runAuth(deps, "set"); err != nil {
		t.Fatalf("runAuth set: %v", err)
	}

	got, err := store.Get(keychainService, apiKeyAccount)
	if err != nil {
		t.Errorf("api key not stored: %v", err)
	} else if string(got) != "secret-key" {
		t.Errorf("api key = %q, want secret-key", string(got))
	}
	gotPub, err := store.Get(keychainService, publicationIDAccount)
	if err != nil {
		t.Errorf("publication id not stored: %v", err)
	} else if string(gotPub) != "pub-123" {
		t.Errorf("publication id = %q, want pub-123", string(gotPub))
	}

	if !containsAll(out.String(), "Saved", "beehiiv-mcp") {
		t.Errorf("output missing success message: %q", out.String())
	}
}

func TestRunAuth_SetOnNonDarwinErrors(t *testing.T) {
	store := newFakeCredStore()
	deps, _ := newAuthDeps(store, "x\ny\n")
	deps.Platform = "linux"

	err := runAuth(deps, "set")
	if err == nil {
		t.Fatal("runAuth set should error on non-darwin")
	}
	if !containsAll(err.Error(), "macOS", "BEEHIIV_API_KEY") {
		t.Errorf("error = %q, want to mention macOS + env-var fallback", err.Error())
	}
}

func TestRunAuth_CheckReportsKeychainSourceWithMaskedKey(t *testing.T) {
	store := newFakeCredStore()
	_ = store.Set(keychainService, apiKeyAccount, []byte("sk_abcdefghij_1234"))
	_ = store.Set(keychainService, publicationIDAccount, []byte("pub_123"))

	deps, out := newAuthDeps(store, "")
	if err := runAuth(deps, "check"); err != nil {
		t.Fatalf("runAuth check: %v", err)
	}

	s := out.String()
	if !containsAll(s, "keychain", "...1234", "pub_123") {
		t.Errorf("check output missing expected fields: %q", s)
	}
	if strings.Contains(s, "sk_abcdefghij") {
		t.Errorf("check output should not leak the full key: %q", s)
	}
}

func TestRunAuth_CheckReportsEnvSourceWhenKeychainEmpty(t *testing.T) {
	store := newFakeCredStore()
	deps, out := newAuthDeps(store, "")
	deps.Getenv = func(k string) string {
		switch k {
		case envAPIKey:
			return "env-api-1234"
		case envPublicationID:
			return "env-pub-1"
		}
		return ""
	}

	if err := runAuth(deps, "check"); err != nil {
		t.Fatalf("runAuth check: %v", err)
	}
	s := out.String()
	if !containsAll(s, "env", "...1234") {
		t.Errorf("check output missing env source: %q", s)
	}
}

func TestRunAuth_CheckReportsMissingWhenNothingConfigured(t *testing.T) {
	store := newFakeCredStore()
	deps, out := newAuthDeps(store, "")

	err := runAuth(deps, "check")
	if err == nil {
		t.Fatal("check with no credentials should return an error for scripting")
	}
	s := out.String()
	if !containsAll(s, "No beehiiv credentials found") {
		t.Errorf("check output should report missing; got %q", s)
	}
	if !containsAll(err.Error(), "beehiiv-mcp auth set") {
		t.Errorf("error should point to setup command; got %q", err.Error())
	}
}

func TestRunAuth_DeleteRemovesBothSecrets(t *testing.T) {
	store := newFakeCredStore()
	_ = store.Set(keychainService, apiKeyAccount, []byte("k"))
	_ = store.Set(keychainService, publicationIDAccount, []byte("p"))

	deps, out := newAuthDeps(store, "")
	if err := runAuth(deps, "delete"); err != nil {
		t.Fatalf("runAuth delete: %v", err)
	}

	if _, err := store.Get(keychainService, apiKeyAccount); err == nil {
		t.Errorf("api key still present after delete")
	}
	if _, err := store.Get(keychainService, publicationIDAccount); err == nil {
		t.Errorf("publication id still present after delete")
	}
	if !contains(out.String(), "Removed") {
		t.Errorf("output missing delete confirmation: %q", out.String())
	}
}

func TestRunAuth_DeleteIsIdempotent(t *testing.T) {
	store := newFakeCredStore()
	deps, _ := newAuthDeps(store, "")
	if err := runAuth(deps, "delete"); err != nil {
		t.Fatalf("runAuth delete on empty store: %v", err)
	}
}

func TestRunAuth_UnknownVerbErrors(t *testing.T) {
	store := newFakeCredStore()
	deps, _ := newAuthDeps(store, "")
	err := runAuth(deps, "rotate")
	if err == nil {
		t.Fatal("unknown verb should error")
	}
	if !containsAll(err.Error(), "set", "check", "delete") {
		t.Errorf("error should list valid verbs; got %q", err.Error())
	}
}

func TestRunAuth_SetRejectsEmptyInput(t *testing.T) {
	store := newFakeCredStore()
	deps, _ := newAuthDeps(store, "\n\n")
	err := runAuth(deps, "set")
	if err == nil {
		t.Fatal("empty values should error")
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "***"},
		{"abcd", "****"},
		{"abcde", "...bcde"},
		{"sk_abcdefghij_1234", "...1234"},
	}
	for _, tc := range tests {
		if got := maskSecret(tc.in); got != tc.want {
			t.Errorf("maskSecret(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
