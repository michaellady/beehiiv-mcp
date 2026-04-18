package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// authDeps wires the side-effectful pieces of the auth subcommand so tests can
// swap them for fakes.
type authDeps struct {
	Store    credStore
	In       io.Reader
	Out      io.Writer
	Getenv   func(string) string
	Platform string // "darwin" gates macOS Keychain mutations
}

func runAuth(deps *authDeps, verb string) error {
	switch verb {
	case "set":
		return runAuthSet(deps)
	case "check":
		return runAuthCheck(deps)
	case "delete":
		return runAuthDelete(deps)
	default:
		return fmt.Errorf("unknown auth verb %q (want set|check|delete)", verb)
	}
}

func runAuthSet(deps *authDeps) error {
	if deps.Platform != "darwin" {
		return fmt.Errorf(
			"auth set requires macOS Keychain. On this platform, export $%s and $%s instead",
			envAPIKey, envPublicationID,
		)
	}

	reader := bufio.NewReader(deps.In)
	fmt.Fprintln(deps.Out, "Enter your beehiiv API key (create one at https://app.beehiiv.com/settings/integrations/api):")
	apiKey, err := readLine(reader)
	if err != nil {
		return fmt.Errorf("read api key: %w", err)
	}
	if apiKey == "" {
		return fmt.Errorf("api key cannot be empty")
	}

	fmt.Fprintln(deps.Out, "Enter your beehiiv publication ID (starts with 'pub_'):")
	pubID, err := readLine(reader)
	if err != nil {
		return fmt.Errorf("read publication id: %w", err)
	}
	if pubID == "" {
		return fmt.Errorf("publication id cannot be empty")
	}

	if err := deps.Store.Set(keychainService, apiKeyAccount, []byte(apiKey)); err != nil {
		return fmt.Errorf("save api key to keychain: %w", err)
	}
	if err := deps.Store.Set(keychainService, publicationIDAccount, []byte(pubID)); err != nil {
		return fmt.Errorf("save publication id to keychain: %w", err)
	}

	fmt.Fprintln(deps.Out, "Saved beehiiv-mcp credentials to the macOS Keychain.")
	fmt.Fprintln(deps.Out, "On first use macOS may prompt to Allow access; choose \"Always Allow\" to avoid future prompts.")
	return nil
}

func runAuthCheck(deps *authDeps) error {
	var getenv func(string) string = deps.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	apiKey, apiSrc := readCredential(deps.Store, apiKeyAccount, envAPIKey, getenv)
	pubID, pubSrc := readCredential(deps.Store, publicationIDAccount, envPublicationID, getenv)

	if apiKey == "" && pubID == "" {
		fmt.Fprintln(deps.Out, "No beehiiv credentials found (checked macOS Keychain and env vars).")
		return fmt.Errorf("no credentials configured; run `beehiiv-mcp auth set` to store them in the Keychain")
	}

	if apiKey != "" {
		fmt.Fprintf(deps.Out, "API key:        %s  (source: %s)\n", maskSecret(apiKey), apiSrc)
	} else {
		fmt.Fprintln(deps.Out, "API key:        (missing)")
	}
	if pubID != "" {
		fmt.Fprintf(deps.Out, "Publication ID: %s  (source: %s)\n", pubID, pubSrc)
	} else {
		fmt.Fprintln(deps.Out, "Publication ID: (missing)")
	}
	return nil
}

func runAuthDelete(deps *authDeps) error {
	if deps.Platform != "darwin" {
		return fmt.Errorf("auth delete requires macOS Keychain; nothing to do on this platform")
	}
	_ = deps.Store.Delete(keychainService, apiKeyAccount)
	_ = deps.Store.Delete(keychainService, publicationIDAccount)
	fmt.Fprintln(deps.Out, "Removed beehiiv-mcp credentials from the macOS Keychain (no-op if absent).")
	return nil
}

// readCredential mirrors lookup() in credentials.go but also accepts a getenv
// override so tests can simulate env-var configurations without mutating the
// real environment.
func readCredential(store credStore, account, envName string, getenv func(string) string) (value, source string) {
	if store != nil {
		if v, err := store.Get(keychainService, account); err == nil && len(v) > 0 {
			return string(v), sourceKeychain
		}
	}
	if v := getenv(envName); v != "" {
		return v, sourceEnv
	}
	return "", ""
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// maskSecret hides all but the last 4 characters of s. Strings of length <= 4
// are fully masked so we never reveal most of the key for short inputs.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return "..." + s[len(s)-4:]
}
