package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	toolStats       = "beehiiv_stats"
	toolAutomations = "beehiiv_automations"
	toolSegments    = "beehiiv_segments"
	toolWebhooks    = "beehiiv_webhooks"
)

// serverDeps bundles the external dependencies the tool handlers need.
// Tests construct it with fakes; main() wires the real client + snapshot store.
type serverDeps struct {
	Stats       statsAPI
	Automations automationsAPI
	Segments    segmentsAPI
	Webhooks    webhooksAPI
	Snapshots   *snapshotStore
	PubID       string
	Now         func() time.Time
	SnapKeep    int
}

func (d *serverDeps) applyDefaults() {
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	if d.SnapKeep <= 0 {
		d.SnapKeep = 30
	}
}

// --- handler functions (unit-testable without going through MCPServer) ---

func handleStats(ctx context.Context, deps *serverDeps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in := statsInput{
		WindowDays: req.GetInt("window_days", defaultWindowDays),
		PostLimit:  req.GetInt("post_limit", defaultPostLimit),
	}
	out, err := runStats(ctx, deps.Stats, deps.Snapshots, deps.PubID, in, deps.Now())
	if err != nil {
		return mcp.NewToolResultErrorFromErr("stats failed", err), nil
	}
	if deps.Snapshots != nil {
		_, _ = deps.Snapshots.Prune(deps.SnapKeep)
	}
	return jsonResult(out)
}

func handleAutomations(ctx context.Context, deps *serverDeps, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	in := automationsInput{IncludeEmails: req.GetBool("include_emails", false)}
	out, err := runAutomations(ctx, deps.Automations, deps.PubID, in)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("automations failed", err), nil
	}
	return jsonResult(out)
}

func handleSegments(ctx context.Context, deps *serverDeps, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := runSegments(ctx, deps.Segments, deps.PubID, deps.Now())
	if err != nil {
		return mcp.NewToolResultErrorFromErr("segments failed", err), nil
	}
	return jsonResult(out)
}

func handleWebhooks(ctx context.Context, deps *serverDeps, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out, err := runWebhooks(ctx, deps.Webhooks, deps.PubID, deps.Now())
	if err != nil {
		return mcp.NewToolResultErrorFromErr("webhooks failed", err), nil
	}
	return jsonResult(out)
}

// --- wiring ---

// buildServer returns an MCPServer with all four tools registered against deps.
func buildServer(deps serverDeps) *server.MCPServer {
	deps.applyDefaults()

	s := server.NewMCPServer(
		"beehiiv-mcp",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(
		mcp.NewTool(toolStats,
			mcp.WithDescription(
				"Get beehiiv newsletter dashboard data — current subscriber count, "+
					"growth over a window (vs. the closest-older local snapshot), "+
					"engagement rates, and stats for recent posts.",
			),
			mcp.WithNumber("window_days",
				mcp.Description("Growth comparison window in days (default 30)."),
				mcp.DefaultNumber(defaultWindowDays),
			),
			mcp.WithNumber("post_limit",
				mcp.Description("Number of recent posts to include (default 5, max 20)."),
				mcp.DefaultNumber(defaultPostLimit),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleStats(ctx, &deps, req)
		},
	)

	s.AddTool(
		mcp.NewTool(toolAutomations,
			mcp.WithDescription(
				"List the publication's automations with current status and "+
					"journey-level metrics (active / completed / exited counts).",
			),
			mcp.WithBoolean("include_emails",
				mcp.Description("Include per-email details for each automation (slower; extra API calls)."),
				mcp.DefaultBool(false),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleAutomations(ctx, &deps, req)
		},
	)

	s.AddTool(
		mcp.NewTool(toolSegments,
			mcp.WithDescription(
				"List segments (saved subscriber filters) with current member counts "+
					"and freshness. Dynamic segments unrecalculated for >24h are flagged stale.",
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleSegments(ctx, &deps, req)
		},
	)

	s.AddTool(
		mcp.NewTool(toolWebhooks,
			mcp.WithDescription(
				"List registered webhooks with subscribed events and recent delivery health.",
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return handleWebhooks(ctx, &deps, req)
		},
	)

	return s
}

// jsonResult marshals v to indented JSON and wraps it in a text content block.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("marshal result", err), nil
	}
	return mcp.NewToolResultText(string(buf)), nil
}

// listToolNames returns the registered tool names in a stable order — used
// by tests to assert the full tool set without touching MCPServer internals.
func listToolNames() []string {
	return []string{toolStats, toolAutomations, toolSegments, toolWebhooks}
}
