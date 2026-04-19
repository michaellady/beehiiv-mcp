package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func makeDeps(t *testing.T) *serverDeps {
	t.Helper()
	return &serverDeps{
		Stats: &fakeStatsAPI{
			pub:      Publication{ID: "pub_1", Name: "EVC"},
			pubStats: PublicationStats{ActiveSubscriptions: 1000, AverageOpenRate: 40, AverageClickRate: 5},
			posts:    []Post{{ID: "p1", Title: "hi", PublishDate: "2026-04-14"}},
		},
		Automations: &fakeAutomationsAPI{
			list:     []Automation{{ID: "a1", Name: "Welcome", Status: "active"}},
			journeys: map[string]JourneyStats{"a1": {Active: 10, Completed: 5}},
		},
		Segments: &fakeSegmentsAPI{segs: []Segment{
			{ID: "s1", Type: "static", MemberCount: 20},
		}},
		Webhooks: &fakeWebhooksAPI{hooks: []Webhook{
			{ID: "w1", Status: "active", LastDeliveryStatus: "success"},
		}},
		Snapshots: newSnapshotStore(t.TempDir()),
		PubID:     "pub_1",
		Now:       func() time.Time { return time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC) },
		SnapKeep:  30,
	}
}

func newReq(args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

func extractText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if r == nil {
		t.Fatal("nil CallToolResult")
	}
	var b strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestBuildServer_RegistersAllFourTools(t *testing.T) {
	s := buildServer(*makeDeps(t))
	if s == nil {
		t.Fatal("buildServer returned nil")
	}
	want := map[string]bool{
		toolStats: true, toolAutomations: true, toolSegments: true, toolWebhooks: true,
	}
	for _, n := range listToolNames() {
		if !want[n] {
			t.Errorf("unexpected tool %q", n)
		}
		delete(want, n)
	}
	if len(want) != 0 {
		t.Errorf("missing tools: %v", want)
	}
}

func TestBuildServer_DefaultsApplied(t *testing.T) {
	deps := *makeDeps(t)
	deps.Now = nil
	deps.SnapKeep = 0
	s := buildServer(deps)
	if s == nil {
		t.Fatal("buildServer nil")
	}
}

func TestHandleStats_ReturnsJSONWithSubscriberCount(t *testing.T) {
	deps := makeDeps(t)
	res, err := handleStats(context.Background(), deps, newReq(map[string]any{
		"window_days": float64(30), "post_limit": float64(3),
	}))
	if err != nil {
		t.Fatalf("handleStats: %v", err)
	}
	txt := extractText(t, res)

	var got map[string]any
	if jErr := json.Unmarshal([]byte(txt), &got); jErr != nil {
		t.Fatalf("non-JSON response: %v\n%s", jErr, txt)
	}
	pub, _ := got["publication"].(map[string]any)
	if pub == nil || pub["id"] != "pub_1" {
		t.Errorf("publication.id wrong: %v", got["publication"])
	}
	subs, _ := got["subscribers"].(map[string]any)
	if subs == nil || subs["current"].(float64) != 1000 {
		t.Errorf("subscribers.current wrong: %v", got["subscribers"])
	}
	if res.IsError {
		t.Errorf("result should not be marked IsError; text: %s", txt)
	}
}

func TestHandleStats_ErrorFromAPIBecomesToolError(t *testing.T) {
	deps := makeDeps(t)
	f := deps.Stats.(*fakeStatsAPI)
	f.pubErr = errors.New("upstream 500")

	res, err := handleStats(context.Background(), deps, newReq(nil))
	if err != nil {
		t.Fatalf("handleStats: %v", err)
	}
	if !res.IsError {
		t.Errorf("result should be IsError; text: %s", extractText(t, res))
	}
}

func TestHandleAutomations_IncludeEmailsPropagatesToAPI(t *testing.T) {
	deps := makeDeps(t)
	f := deps.Automations.(*fakeAutomationsAPI)
	f.emails = map[string][]AutomationEmail{"a1": {{ID: "e1", Subject: "Welcome"}}}

	res, err := handleAutomations(context.Background(), deps, newReq(map[string]any{"include_emails": true}))
	if err != nil {
		t.Fatalf("handleAutomations: %v", err)
	}
	txt := extractText(t, res)
	if !strings.Contains(txt, `"subject": "Welcome"`) {
		t.Errorf("expected emails in output; got:\n%s", txt)
	}
	if f.emailCalls != 1 {
		t.Errorf("emailCalls = %d, want 1", f.emailCalls)
	}
}

func TestHandleAutomations_ErrorPath(t *testing.T) {
	deps := makeDeps(t)
	deps.Automations.(*fakeAutomationsAPI).listErr = errors.New("boom")
	res, err := handleAutomations(context.Background(), deps, newReq(nil))
	if err != nil {
		t.Fatalf("handleAutomations: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError on API failure")
	}
}

func TestHandleSegments_ReturnsTotalCount(t *testing.T) {
	deps := makeDeps(t)
	res, _ := handleSegments(context.Background(), deps, newReq(nil))
	var got map[string]any
	_ = json.Unmarshal([]byte(extractText(t, res)), &got)
	if int(got["total_segment_count"].(float64)) != 1 {
		t.Errorf("total_segment_count wrong: %v", got["total_segment_count"])
	}
}

func TestHandleSegments_ErrorPath(t *testing.T) {
	deps := makeDeps(t)
	deps.Segments.(*fakeSegmentsAPI).err = errors.New("nope")
	res, _ := handleSegments(context.Background(), deps, newReq(nil))
	if !res.IsError {
		t.Errorf("expected IsError")
	}
}

func TestHandleWebhooks_ReturnsActiveCount(t *testing.T) {
	deps := makeDeps(t)
	res, _ := handleWebhooks(context.Background(), deps, newReq(nil))
	var got map[string]any
	_ = json.Unmarshal([]byte(extractText(t, res)), &got)
	if int(got["active_count"].(float64)) != 1 {
		t.Errorf("active_count wrong: %v", got["active_count"])
	}
}

func TestHandleWebhooks_ErrorPath(t *testing.T) {
	deps := makeDeps(t)
	deps.Webhooks.(*fakeWebhooksAPI).err = errors.New("nope")
	res, _ := handleWebhooks(context.Background(), deps, newReq(nil))
	if !res.IsError {
		t.Errorf("expected IsError")
	}
}

func TestJSONResult_MarshalsIndentedJSON(t *testing.T) {
	res, err := jsonResult(map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("jsonResult: %v", err)
	}
	txt := extractText(t, res)
	if !strings.Contains(txt, "\n  \"k\": \"v\"") {
		t.Errorf("expected indented JSON, got: %s", txt)
	}
}
