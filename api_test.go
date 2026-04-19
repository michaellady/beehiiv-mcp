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

func TestAPI_GetPublicationStats(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1": `{
			"data":{"id":"pub_1","name":"EVC",
			 "stats":{"active_subscriptions":179,"active_free_subscriptions":179,
			 "average_open_rate":44.45,"average_click_rate":11.98,
			 "total_sent":1147,"total_delivered":1145,
			 "total_unique_opened":509,"total_clicked":61}}
		}`,
	})
	s, err := c.GetPublicationStats(context.Background(), "pub_1")
	if err != nil {
		t.Fatalf("GetPublicationStats: %v", err)
	}
	if s.ActiveSubscriptions != 179 {
		t.Errorf("ActiveSubscriptions = %d, want 179", s.ActiveSubscriptions)
	}
	if s.AverageOpenRate != 44.45 {
		t.Errorf("AverageOpenRate = %v, want 44.45", s.AverageOpenRate)
	}
	if s.AverageClickRate != 11.98 {
		t.Errorf("AverageClickRate = %v, want 11.98", s.AverageClickRate)
	}
	if s.TotalDelivered != 1145 {
		t.Errorf("TotalDelivered = %d, want 1145", s.TotalDelivered)
	}
}

func TestAPI_ListPostsWithStats_MapsFields(t *testing.T) {
	c := withAPI(t, map[string]string{
		"GET /publications/pub_1/posts": `{
			"data":[
				{"id":"p1","title":"Hello","publish_date":1744669200,
				 "stats":{"email":{"recipients":175,"opens":97,"open_rate":37.14,"clicks":33,"click_rate":7.69}}}
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
	if p.Recipients != 175 || p.Opens != 97 || p.OpenRate != 37.14 || p.Clicks != 33 || p.ClickRate != 7.69 {
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

func TestAPI_ListSubscriptions_WalksCursorsAndStopsAtSince(t *testing.T) {
	// Page 1 (newest 3): cursor "p2"
	// Page 2 (oldest 2): no cursor, has_more false
	// since cuts off the 4th sub
	pages := map[string]string{
		"": `{
			"data":[
				{"id":"s1","email":"a@x.com","status":"active","created":1745000000,"utm_source":"youtube","referring_site":""},
				{"id":"s2","email":"b@x.com","status":"active","created":1744900000,"utm_source":"linkedin","referring_site":""},
				{"id":"s3","email":"c@x.com","status":"active","created":1744800000,"utm_source":"","referring_site":""}
			],
			"has_more":true,"next_cursor":"p2","limit":3
		}`,
		"p2": `{
			"data":[
				{"id":"s4","email":"d@x.com","status":"active","created":1744700000,"utm_source":"","referring_site":""},
				{"id":"s5","email":"e@x.com","status":"active","created":1744600000,"utm_source":"","referring_site":""}
			],
			"has_more":false,"next_cursor":"","limit":3
		}`,
	}
	var reqCursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := r.URL.Query().Get("cursor")
		reqCursors = append(reqCursors, c)
		body, ok := pages[c]
		if !ok {
			http.Error(w, "unmapped cursor "+c, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})

	// Since = 1744750000 → cuts off s4 and s5 (created earlier).
	since := unixToTime(1744750000)
	subs, err := c.ListSubscriptions(context.Background(), "pub_1", since, 100)
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 3 {
		t.Errorf("got %d subs, want 3 (s1/s2/s3)", len(subs))
	}
	if len(reqCursors) < 2 {
		t.Errorf("requested cursors = %v, expected at least 2 pages", reqCursors)
	}
	if subs[0].UTMSource != "youtube" {
		t.Errorf("UTM mapping broken: %+v", subs[0])
	}
}

func TestAPI_ListSubscriptions_RespectsMax(t *testing.T) {
	body := `{
		"data":[
			{"id":"s1","created":1745000000},{"id":"s2","created":1744900000},
			{"id":"s3","created":1744800000},{"id":"s4","created":1744700000}
		],
		"has_more":true,"next_cursor":"p2","limit":4
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})

	subs, err := c.ListSubscriptions(context.Background(), "pub_1", unixToTime(0), 2)
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("got %d subs, want 2 (capped by max)", len(subs))
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
