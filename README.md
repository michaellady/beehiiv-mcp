# beehiiv-mcp

Read-only MCP server that gives Claude visibility into a beehiiv newsletter: subscriber count + growth, recent post stats, automations, segments, and webhooks.

## Tools

- **`beehiiv_stats`** — current subscriber count, growth over a window (diffed against the closest-older local snapshot), engagement rates, and stats for the most recent posts. Writes a timestamped snapshot on every call so next week's "+87 subs" report doesn't need an extra API round-trip.
- **`beehiiv_automations`** — lists the publication's automations with current status and journey-level metrics (active / completed / exited counts). Pass `include_emails: true` to also fetch each automation's email sequence.
- **`beehiiv_segments`** — lists saved subscriber filters with current member counts. Flags dynamic segments as stale when the last recalculation is >24h old.
- **`beehiiv_webhooks`** — lists registered webhooks with subscribed events and recent delivery health.

Read-only: no posts are created, no subscribers are edited, no state is mutated.

## Setup

### 1. Build the binary

```bash
cd ~/dev/beehiiv-mcp
go build -o beehiiv-mcp .
```

### 2. Store credentials in macOS Keychain

Create an API key at https://app.beehiiv.com/settings/integrations/api, then:

```bash
./beehiiv-mcp auth set
# Enter your beehiiv API key: …
# Enter your beehiiv publication ID (starts with "pub_"): …
```

The first keychain write triggers a one-time macOS prompt: **"Allow beehiiv-mcp to access Keychain?"** — pick **Always Allow** so Claude Code doesn't see a dialog on every tool call.

Verify the credentials are readable:

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

### 3. Register the MCP server with Claude Code

Add an entry to your Claude Code MCP config (usually `~/.claude.json`):

```json
{
  "mcpServers": {
    "beehiiv": {
      "command": "/Users/mikelady/dev/beehiiv-mcp/beehiiv-mcp"
    }
  }
}
```

Restart Claude Code. The tools will appear as `mcp__beehiiv__beehiiv_stats`, `mcp__beehiiv__beehiiv_automations`, `mcp__beehiiv__beehiiv_segments`, `mcp__beehiiv__beehiiv_webhooks`.

### 4. Try it

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
# Unit tests (no network, no keychain access)
go test ./...

# Coverage
go test -cover ./...

# Integration tests — opt-in; hits the live API with whatever credentials are configured
go test -tags integration ./...

# Build + static analysis
go build ./... && go vet ./...
```

The codebase is structured for testability via narrow interfaces:

- `credStore` (`credentials.go`) — Keychain abstraction; real impl in `keychain_darwin.go`, tests use `fakeCredStore`.
- `statsAPI`, `automationsAPI`, `segmentsAPI`, `webhooksAPI` (in each handler file) — the slice of the beehiiv API that each tool needs. Real implementation methods live on `*client` in `api.go`; tests inject focused fakes.

Unit tests run in <1s and never touch the network or Keychain.

## Limitations

- **macOS only** for native Keychain access. On other platforms the binary still runs, but `auth set/delete` errors out and you must use env vars.
- **Read-only.** No tools mutate state. If writes are added in the future they'll be gated on explicit user confirmation.
- **Ad opportunities tool is deferred** until the core stats tools are proven in practice.

## Rate limits

beehiiv enforces 180 requests per minute per organization. Each tool call makes 3-6 HTTP requests, so normal use doesn't approach the limit. The HTTP client retries 429/5xx responses with exponential backoff (base 500ms, max 3 retries).

## License

MIT
