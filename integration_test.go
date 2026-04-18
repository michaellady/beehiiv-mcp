//go:build integration

package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// Integration tests are opt-in. Run with:
//   go test -tags integration ./...
//
// Requires a live beehiiv API key + publication ID. These may be stored in
// the macOS Keychain (via `beehiiv-mcp auth set`) OR exported as env vars:
//   BEEHIIV_API_KEY=...
//   BEEHIIV_PUBLICATION_ID=...
//
// Tests are read-only — no state is mutated on the beehiiv side.

func newIntegrationClient(t *testing.T) (*client, string) {
	t.Helper()
	creds, err := resolveCredentials(newMacOSKeychain())
	if err != nil {
		t.Skipf("integration: credentials unavailable (%v) — run `beehiiv-mcp auth set` or export BEEHIIV_API_KEY / BEEHIIV_PUBLICATION_ID", err)
	}
	return newClient(clientOpts{APIKey: creds.APIKey}), creds.PublicationID
}

func TestIntegration_GetPublication(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pub, err := c.GetPublication(ctx, pubID)
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	if pub.ID == "" || pub.Name == "" {
		t.Errorf("publication fields empty: %+v", pub)
	}
	t.Logf("publication: %s (%s)", pub.Name, pub.ID)
}

func TestIntegration_CountSubscribers(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	n, err := c.CountSubscribers(ctx, pubID)
	if err != nil {
		t.Fatalf("CountSubscribers: %v", err)
	}
	if n < 0 {
		t.Errorf("subscriber count negative: %d", n)
	}
	t.Logf("subscribers: %d", n)
}

func TestIntegration_ListPostsWithStats(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	posts, err := c.ListPostsWithStats(ctx, pubID, 3)
	if err != nil {
		t.Fatalf("ListPostsWithStats: %v", err)
	}
	if len(posts) > 3 {
		t.Errorf("got %d posts, expected ≤3 (limit)", len(posts))
	}
	for _, p := range posts {
		t.Logf("post %s  opens=%d  rate=%.3f  title=%q", p.PublishDate, p.Opens, p.OpenRate, p.Title)
	}
}

func TestIntegration_ListAutomations(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	autos, err := c.ListAutomations(ctx, pubID)
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	for _, a := range autos {
		t.Logf("automation: %s (%s) status=%s trigger=%s emails=%d", a.Name, a.ID, a.Status, a.Trigger, a.EmailCount)
	}
}

func TestIntegration_ListSegments(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	segs, err := c.ListSegments(ctx, pubID)
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	for _, s := range segs {
		t.Logf("segment: %s type=%s members=%d last_calc=%s", s.Name, s.Type, s.MemberCount, s.LastCalculatedAt)
	}
}

func TestIntegration_ListWebhooks(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hooks, err := c.ListWebhooks(ctx, pubID)
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	for _, h := range hooks {
		t.Logf("webhook: %s status=%s last_delivery=%s events=%v", h.URL, h.Status, h.LastDeliveryStatus, h.Events)
	}
}

func TestIntegration_RunStatsFullPath(t *testing.T) {
	c, pubID := newIntegrationClient(t)
	store := newSnapshotStore(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	out, err := runStats(ctx, c, store, pubID, statsInput{WindowDays: 30, PostLimit: 3}, time.Now().UTC())
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if out.SnapshotSavedTo == "" {
		t.Errorf("SnapshotSavedTo should be set")
	}
	if _, err := os.Stat(out.SnapshotSavedTo); err != nil {
		t.Errorf("snapshot file missing: %v", err)
	}
	t.Logf("stats: subs=%d open_rate=%.3f recent_posts=%d", out.Subscribers.Current, out.Engagement.OpenRate, len(out.RecentPosts))
}
