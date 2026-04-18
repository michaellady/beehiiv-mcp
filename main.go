package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) >= 2 && os.Args[1] == "auth" {
		verb := ""
		if len(os.Args) >= 3 {
			verb = os.Args[2]
		}
		deps := &authDeps{
			Store:    newMacOSKeychain(),
			In:       os.Stdin,
			Out:      os.Stdout,
			Getenv:   os.Getenv,
			Platform: runtime.GOOS,
		}
		if err := runAuth(deps, verb); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}

	// Default: run the MCP server on stdio.
	if err := serveStdio(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serveStdio() error {
	creds, err := resolveCredentials(newMacOSKeychain())
	if err != nil {
		return err
	}

	client := newClient(clientOpts{APIKey: creds.APIKey})
	snapshots := newSnapshotStore(defaultSnapshotDir())

	mcp := buildServer(serverDeps{
		Stats:       client,
		Automations: client,
		Segments:    client,
		Webhooks:    client,
		Snapshots:   snapshots,
		PubID:       creds.PublicationID,
	})
	return server.ServeStdio(mcp)
}

// defaultSnapshotDir returns $XDG_DATA_HOME/beehiiv-mcp/snapshots, falling back
// to ~/.local/share/beehiiv-mcp/snapshots or the current directory.
func defaultSnapshotDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "beehiiv-mcp", "snapshots")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "snapshots"
	}
	return filepath.Join(home, ".local", "share", "beehiiv-mcp", "snapshots")
}
