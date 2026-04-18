package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	if len(os.Args) >= 3 && os.Args[1] == "auth" {
		deps := &authDeps{
			Store:    newMacOSKeychain(),
			In:       os.Stdin,
			Out:      os.Stdout,
			Getenv:   os.Getenv,
			Platform: runtime.GOOS,
		}
		if err := runAuth(deps, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	// MCP server wiring lands in step 9 (see plan).
	fmt.Fprintln(os.Stderr, "beehiiv-mcp: MCP server mode not yet implemented; run `beehiiv-mcp auth <set|check|delete>` for credential management.")
	os.Exit(2)
}
