//go:build probe

// Run with: go test -tags probe -v -run Probe ./...
//
// Hits the live beehiiv API through the same credential path as the MCP
// server and dumps raw JSON bodies so we can verify wire-struct field names.
// Guarded by a build tag so normal tests never touch the network.

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func probeClient(t *testing.T) (*client, string) {
	t.Helper()
	creds, err := resolveCredentials(newMacOSKeychain())
	if err != nil {
		t.Skipf("probe: credentials unavailable: %v", err)
	}
	return newClient(clientOpts{APIKey: creds.APIKey}), creds.PublicationID
}

func probeDump(t *testing.T, label string, c *client, path string, query map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var raw json.RawMessage
	if err := c.do(ctx, "GET", path, query, &raw); err != nil {
		t.Logf("%s: ERROR %v", label, err)
		return
	}
	// Pretty-print for easier reading.
	var pretty interface{}
	_ = json.Unmarshal(raw, &pretty)
	pp, _ := json.MarshalIndent(pretty, "", "  ")
	t.Logf("\n=== %s (%s) ===\n%s", label, path, string(pp))
}

func TestProbe_SubscriptionsCount(t *testing.T) {
	c, pubID := probeClient(t)
	probeDump(t, "subscriptions-limit-1", c, "/publications/"+pubID+"/subscriptions", map[string]string{"limit": "1"})
}

func TestProbe_PublicationExpandStats(t *testing.T) {
	c, pubID := probeClient(t)
	probeDump(t, "publication-expand-stats", c, "/publications/"+pubID, map[string]string{"expand[]": "stats"})
	probeDump(t, "publication-plain", c, "/publications/"+pubID, nil)
}

func TestProbe_PostWithStats(t *testing.T) {
	c, pubID := probeClient(t)
	probeDump(t, "posts-expand-stats", c, "/publications/"+pubID+"/posts", map[string]string{
		"expand[]":  "stats",
		"limit":     "1",
		"status":    "confirmed",
		"order_by":  "publish_date",
		"direction": "desc",
	})
}

func TestProbe_Engagements(t *testing.T) {
	c, pubID := probeClient(t)
	// Try common endpoint shapes; whichever returns data wins.
	probeDump(t, "engagement-singular", c, "/publications/"+pubID+"/engagement", nil)
	probeDump(t, "engagements-plural", c, "/publications/"+pubID+"/engagements", nil)
}
