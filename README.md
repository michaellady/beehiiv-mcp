# beehiiv-mcp

Read-only MCP server that gives Claude visibility into a beehiiv newsletter: subscriber count + growth, recent post stats, automations, segments, and webhooks.

## Tools

- **`beehiiv_stats`** — current subscriber count, growth over a window (diffed against the closest-older local snapshot), engagement rates, and stats for the most recent posts. Writes a timestamped snapshot on every call so next week's "+87 subs" report doesn't need an extra API round-trip.
- **`beehiiv_automations`** — lists the publication's automations with current status and journey-level metrics (active / completed / exited counts). Pass `include_emails: true` to also fetch each automation's email sequence.
- **`beehiiv_segments`** — lists saved subscriber filters with current member counts. Flags dynamic segments as stale when the last recalculation is >24h old.
- **`beehiiv_webhooks`** — lists registered webhooks with subscribed events and recent delivery health.

Read-only: no posts are created, no subscribers are edited, no state is mutated.

## Install

**Option A — Download a pre-built binary (fastest):**

Every code-change push to `main` produces a new patch release — grab the latest from [github.com/michaellady/beehiiv-mcp/releases/latest](https://github.com/michaellady/beehiiv-mcp/releases/latest). If you want a specific earlier version, all releases are listed under `/releases`.

Pick `darwin_arm64` (Apple Silicon) or `darwin_amd64` (Intel, if available — Intel builds are best-effort on the free-tier GitHub runner), verify the SHA256, then extract:

```bash
tar xzf beehiiv-mcp_v0.0.1_darwin_arm64.tar.gz
cd beehiiv-mcp_v0.0.1_darwin_arm64
./beehiiv-mcp auth set
```

Pre-built binaries are **ad-hoc signed**, which means macOS shows a Keychain "Allow" prompt the first time you run `auth set` AND again after each upgrade (because each release has a fresh cdhash-based signature). If that's annoying, use Option B for stable signatures across upgrades.

**Option B — Build from source (zero-prompt across upgrades):**

Follow the Setup section below. Signing with a stable Apple Developer identity keeps your Keychain ACL entries valid across `make install` runs, so prompts appear exactly once.

## Setup (build from source)

### 1. Pick a code-signing identity

Signing the binary with a stable identity is what makes your Keychain ACL entries survive rebuilds — macOS would otherwise treat every `go build` as a different app and re-prompt.

List identities currently in your keychain:

```bash
security find-identity -v -p codesigning
```

If you see an `Apple Development` or `Developer ID Application` identity from your Apple Developer account, use that — it's already trusted, no setup needed.

If you don't have one, create a self-signed cert via **Keychain Access → Certificate Assistant → Create a Certificate…** (Name: `beehiiv-mcp-dev`, Identity Type: Self-Signed Root, Certificate Type: Code Signing), then trust it for code signing:

```bash
security find-certificate -c beehiiv-mcp-dev -p > /tmp/cert.pem
security add-trusted-cert -d -r trustRoot -p codeSign \
  -k ~/Library/Keychains/login.keychain-db /tmp/cert.pem
rm /tmp/cert.pem
```

### 2. Clone + build + sign the binary

```bash
git clone https://github.com/michaellady/beehiiv-mcp.git
cd beehiiv-mcp
make install
```

`make install` runs `go build` then `codesign`, defaulting to the **first valid codesigning identity** in your keychain. Output includes the absolute path to paste into your Claude Code MCP config.

To pin a specific identity, override `CODESIGN_IDENTITY` with either its SHA1 or full name:

```bash
make install CODESIGN_IDENTITY="C2CC96BE88BCE19D66511036A9BC9EBB6DF3F424"
make install CODESIGN_IDENTITY="Apple Development: you@example.com (TEAMID)"
```

### 3. Store credentials in macOS Keychain

Create an API key at https://app.beehiiv.com/settings/integrations/api, then:

```bash
./beehiiv-mcp auth set
# Enter your beehiiv API key: …
# Enter your beehiiv publication ID (starts with "pub_"): …
```

The first write to Keychain may trigger one macOS "Allow" prompt. Accept it. Because the item is created with an ACL that pre-trusts this signed binary, **subsequent reads by `beehiiv-mcp` happen silently** — no prompt when Claude Code launches the MCP server, and no prompt after rebuilds (as long as you keep signing with the same identity).

Verify the credentials are readable (should show no prompt):

```bash
./beehiiv-mcp auth check
# API key:        ...ab12  (source: keychain)
# Publication ID: pub_…    (source: keychain)
```

If you prefer environment variables (dev / CI), export them instead — the binary prefers Keychain and falls back to env vars per field:

```bash
export BEEHIIV_API_KEY=…
export BEEHIIV_PUBLICATION_ID=pub_…
```

### 4. Register the MCP server with Claude Code

Add an entry to your Claude Code MCP config (usually `~/.claude.json`):

```json
{
  "mcpServers": {
    "beehiiv": {
      "command": "/absolute/path/to/beehiiv-mcp/beehiiv-mcp"
    }
  }
}
```

Restart Claude Code. The tools will appear as `mcp__beehiiv__beehiiv_stats`, `mcp__beehiiv__beehiiv_automations`, `mcp__beehiiv__beehiiv_segments`, `mcp__beehiiv__beehiiv_webhooks`.

### 5. Try it

Ask Claude:

> How's my newsletter doing? Growth vs last month?

Claude will call `beehiiv_stats` and return subscriber count + 30-day delta. The first call has no prior snapshot, so growth will show `history_sufficient: false` — subsequent calls populate the history.

## Subcommands

| Command                    | What it does                                                    |
|---------------------------|------------------------------------------------------------------|
| `beehiiv-mcp` (no args)   | Runs the MCP server on stdio (what Claude Code launches).       |
| `beehiiv-mcp auth set`    | Prompt for and store API key + publication ID in the Keychain.  |
| `beehiiv-mcp auth check`  | Show which credentials are configured (masks all but last 4).   |
| `beehiiv-mcp auth delete` | Remove stored credentials from the Keychain.                    |

## Snapshots

Each `beehiiv_stats` call writes `stats-<UTC timestamp>.json` to:

- `$XDG_DATA_HOME/beehiiv-mcp/snapshots/` (if set), or
- `~/.local/share/beehiiv-mcp/snapshots/`

The 30 most recent snapshots are kept; older ones are pruned automatically. These enable the growth-over-window calculation without re-querying beehiiv for historical subscriber counts.

## Development

```bash
make test               # unit tests (no network, no keychain access)
make cover              # coverage
make test-integration   # opt-in; hits the live API with configured credentials
make vet                # go vet ./...
make install            # go build + codesign (use after any source change)
make clean              # remove binary + coverage
```

The codebase is structured for testability via narrow interfaces:

- `credStore` (`credentials.go`) — Keychain abstraction; real impl in `keychain_darwin.go`, tests use `fakeCredStore`.
- `statsAPI`, `automationsAPI`, `segmentsAPI`, `webhooksAPI` (in each handler file) — the slice of the beehiiv API that each tool needs. Real implementation methods live on `*client` in `api.go`; tests inject focused fakes.

Unit tests run in <1s and never touch the network or Keychain.

### Keychain implementation notes

`keychain_darwin.go` talks to Security.framework directly via cgo — no third-party keychain dependency. At `auth set` time it constructs a `SecAccess` whose trusted-applications list contains exactly the current running binary (`SecTrustedApplicationCreateFromPath(NULL, …)`). Reads from that same binary thereafter skip the macOS authorization prompt.

The Designated Requirement embedded in the ACL is derived from the binary's code signature. This is why signing with a stable identity matters — rebuilds signed with the same identity match the same DR, so the ACL entry stays valid. Rebuilding without codesign (or with a different cert) changes the DR and macOS will re-prompt.

The APIs used (`SecKeychainItemCreateFromContent`, `SecAccessCreate`, `SecTrustedApplicationCreateFromPath`) are deprecated in macOS 10.15 in favor of `SecItemAdd` + `kSecUseDataProtectionKeychain`. The modern APIs do not expose per-item trusted-app ACLs on the file-based login keychain, so we use the legacy ones deliberately and silence the deprecation warnings.

## Limitations

- **macOS only** for native Keychain access. On other platforms the binary still runs, but `auth set/delete` errors out and you must use env vars.
- **Read-only.** No tools mutate state. If writes are added in the future they'll be gated on explicit user confirmation.
- **Ad opportunities tool is deferred** until the core stats tools are proven in practice.

## Rate limits

beehiiv enforces 180 requests per minute per organization. Each tool call makes 3-6 HTTP requests, so normal use doesn't approach the limit. The HTTP client retries 429/5xx responses with exponential backoff (base 500ms, max 3 retries).

## License

MIT
