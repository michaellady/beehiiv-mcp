package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withAPI runs fn against a fake beehiiv API whose handlers are described by
// the routes map (method+path → response body). Returns the client wired to
// the test server plus a teardown.
func withAPI(t *testing.T, routes map[string]string) *client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		body, ok := routes[key]
		if !ok {
			http.Error(w, "unmapped route "+key, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})
}

func TestAPI_GetPublication(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1": `{"data":{"id":"pub_1","name":"EVC"}}`,
	})
	pub, err := c.GetPublication(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("GetPublication: %v", err)
	}
	if pub.ID != "pub_1" || pub.Name != "EVC" {
		t.Errorf("pub = %+v", pub)
	}
}

func TestAPI_CountSubscribers(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/subscriptions": `{"total_results":1234,"data":[]}`,
	})
	n, err := c.CountSubscribers(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("CountSubscribers: %v", err)
	}
	if n != 1234 {
		t.Errorf("n = %d, want 1234", n)
	}
}

func TestAPI_ListPostsWithStats_MapsFields(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/posts": `{
			"data":[
				{"id":"p1","title":"Hello","publish_date":1744669200,
				 "stats":{"email":{"opens":100,"open_rate":0.4,"clicks":10,"click_rate":0.04,"subscribers_gained":3}}}
			],
			"page":1,"total_pages":1
		}`,
	})
	posts, err := c.ListPostsWithStats(context.Background(), "pub_1", 5)
	if err != nil {
		t.Fatalf("ListPostsWithStats: %v", err)
	}
	if len(posts) != 1 || posts[0].Title != "Hello" {
		t.Fatalf("posts = %+v", posts)
	}
	p := posts[0]
	if p.Opens != 100 || p.OpenRate != 0.4 || p.Clicks != 10 || p.ClickRate != 0.04 || p.SubscriberGained != 3 {
		t.Errorf("mapped fields wrong: %+v", p)
	}
	if p.PublishDate == "" {
		t.Errorf("PublishDate should be formatted; got empty")
	}
}

func TestAPI_ListPostsWithStats_CapsToLimit(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/posts": `{
			"data":[{"id":"a"},{"id":"b"},{"id":"c"}],
			"page":1,"total_pages":1
		}`,
	})
	posts, err := c.ListPostsWithStats(context.Background(), "pub_1", 2)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(posts) != 2 {
		t.Errorf("posts = %d, want 2 (capped)", len(posts))
	}
}

func TestAPI_GetEngagements_DegradesOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()
	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})

	eng, err := c.GetEngagements(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("GetEngagements should not propagate 404, got %v", err)
	}
	if eng.OpenRate != 0 {
		t.Errorf("eng = %+v, want zero on 404", eng)
	}
}

func TestAPI_ListAutomations(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/automations": `{
			"data":[
				{"id":"a1","name":"Welcome","status":"active","trigger":"subscription_created","email_count":3}
			],
			"page":1,"total_pages":1
		}`,
	})
	autos, err := c.ListAutomations(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("ListAutomations: %v", err)
	}
	if len(autos) != 1 || autos[0].Name != "Welcome" {
		t.Errorf("autos = %+v", autos)
	}
}

func TestAPI_GetJourneyStats(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/automations/a1/journeys": `{"data":{"active_subscriptions":10,"completed_subscriptions":5,"exited_subscriptions":1}}`,
	})
	js, err := c.GetJourneyStats(context.Background(), "pub_1", "a1")
	if err != nil {
		t.Fatalf("GetJourneyStats: %v", err)
	}
	if js.Active != 10 || js.Completed != 5 || js.Exited != 1 {
		t.Errorf("js = %+v", js)
	}
}

func TestAPI_ListAutomationEmails(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/automations/a1/emails": `{
			"data":[{"id":"e1","subject":"Welcome!","position":1,"delay_days":0,"opens":100,"open_rate":0.5,"clicks":20,"click_rate":0.1}],
			"page":1,"total_pages":1
		}`,
	})
	emails, err := c.ListAutomationEmails(context.Background(), "pub_1", "a1")
	if err != nil {
		t.Fatalf("ListAutomationEmails: %v", err)
	}
	if len(emails) != 1 || emails[0].Subject != "Welcome!" {
		t.Errorf("emails = %+v", emails)
	}
}

func TestAPI_ListSegments_ParsesTypeAndTimestamp(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/segments": `{
			"data":[
				{"id":"s1","name":"Paid","type":"static","member_count":42,"last_calculated_at":0},
				{"id":"s2","name":"Recent","type":"dynamic","member_count":300,"last_calculated_at":1744641600}
			],
			"page":1,"total_pages":1
		}`,
	})
	segs, err := c.ListSegments(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("segs = %d, want 2", len(segs))
	}
	if !segs[0].LastCalculatedAt.IsZero() {
		t.Errorf("static segment should have zero LastCalculatedAt")
	}
	if segs[1].LastCalculatedAt.IsZero() {
		t.Errorf("dynamic segment should have non-zero LastCalculatedAt")
	}
}

func TestAPI_ListWebhooks_FillsUnknownStatus(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/webhooks": `{
			"data":[
				{"id":"w1","url":"https://x","status":"active","events":["subscription.created"],"last_delivery_status":""}
			],
			"page":1,"total_pages":1
		}`,
	})
	hooks, err := c.ListWebhooks(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(hooks) != 1 {
		t.Fatalf("hooks = %d, want 1", len(hooks))
	}
	if hooks[0].LastDeliveryStatus != "unknown" {
		t.Errorf("expected empty last_delivery_status to be mapped to 'unknown'; got %q", hooks[0].LastDeliveryStatus)
	}
}

func TestAPI_Helpers(t *testing.T) {
	if got := unixToDate(0); got != "" {
		t.Errorf("unixToDate(0) = %q, want empty", got)
	}
	if got := unixToTime(0); !got.IsZero() {
		t.Errorf("unixToTime(0) should be zero; got %v", got)
	}
	if got := fallback("", "def"); got != "def" {
		t.Errorf("fallback empty → %q, want def", got)
	}
	if got := fallback("x", "def"); got != "x" {
		t.Errorf("fallback x → %q, want x", got)
	}
}
