package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_SendsBearerAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newClient(clientOpts{APIKey: "test-key", BaseURL: srv.URL})

	var out map[string]any
	if err := c.do(context.Background(), "GET", "/whatever", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
}

func TestClient_DecodesJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"name":"Enterprise Vibe Code","id":"pub_123"}`))
	}))
	defer srv.Close()

	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})

	type publication struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	var got publication
	if err := c.do(context.Background(), "GET", "/publications/pub_123", nil, &got); err != nil {
		t.Fatalf("do: %v", err)
	}
	if got.Name != "Enterprise Vibe Code" || got.ID != "pub_123" {
		t.Errorf("got %+v, want {Name:Enterprise Vibe Code ID:pub_123}", got)
	}
}

func TestClient_AppendsQueryParameters(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})
	_ = c.do(context.Background(), "GET", "/posts", map[string]string{"limit": "5", "expand": "stats"}, &struct{}{})

	if !contains(gotQuery, "limit=5") || !contains(gotQuery, "expand=stats") {
		t.Errorf("query = %q, missing limit/expand", gotQuery)
	}
}

func TestClient_RetriesOn429AndSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.Header().Set("RateLimit-Reset", strconv.FormatInt(time.Now().Unix()+1, 10))
			http.Error(w, "rate limit", http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newClient(clientOpts{
		APIKey:      "k",
		BaseURL:     srv.URL,
		BackoffBase: time.Millisecond,
		MaxRetries:  3,
	})

	var out map[string]any
	if err := c.do(context.Background(), "GET", "/x", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts = %d, want 3 (2 retries + 1 success)", got)
	}
}

func TestClient_GivesUpAfterMaxRetries(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		http.Error(w, "rate limit", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newClient(clientOpts{
		APIKey:      "k",
		BaseURL:     srv.URL,
		BackoffBase: time.Millisecond,
		MaxRetries:  2,
	})

	var out map[string]any
	err := c.do(context.Background(), "GET", "/x", nil, &out)
	if err == nil {
		t.Fatal("do should fail after max retries")
	}
	// MaxRetries=2 → 1 initial + 2 retries = 3 attempts
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempts = %d, want 3 (1 initial + 2 retries)", got)
	}
}

func TestClient_Returns4xxAsErrorWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer srv.Close()

	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})

	var out struct{}
	err := c.do(context.Background(), "GET", "/x", nil, &out)
	if err == nil {
		t.Fatal("do should fail on 401")
	}
	if !contains(err.Error(), "401") || !contains(err.Error(), "bad token") {
		t.Errorf("error = %q, want to include 401 and bad token", err.Error())
	}
}

func TestClient_PaginatesUntilEmpty(t *testing.T) {
	pages := map[string]string{
		"":  `{"data":[{"id":"a"},{"id":"b"}],"page":1,"total_pages":2}`,
		"2": `{"data":[{"id":"c"}],"page":2,"total_pages":2}`,
	}
	var reqPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		reqPages = append(reqPages, p)
		_, _ = w.Write([]byte(pages[p]))
	}))
	defer srv.Close()

	c := newClient(clientOpts{APIKey: "k", BaseURL: srv.URL})

	type item struct {
		ID string `json:"id"`
	}
	var items []item
	if err := paginate(context.Background(), c, "/items", nil, &items); err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if len(items) != 3 || items[0].ID != "a" || items[2].ID != "c" {
		t.Errorf("items = %+v, want a/b/c", items)
	}
	// First page request has no explicit page param; second requests page=2.
	if len(reqPages) != 2 || reqPages[1] != "2" {
		t.Errorf("requested pages = %v, want [\"\",\"2\"]", reqPages)
	}
}

func TestClient_ContextCancelStopsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newClient(clientOpts{
		APIKey:      "k",
		BaseURL:     srv.URL,
		BackoffBase: 50 * time.Millisecond,
		MaxRetries:  5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	var out map[string]any
	err := c.do(ctx, "GET", "/x", nil, &out)
	if err == nil {
		t.Fatal("do should fail when context is cancelled")
	}
}

func TestClient_DefaultsApplied(t *testing.T) {
	c := newClient(clientOpts{APIKey: "k"})
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q, want %q", c.baseURL, defaultBaseURL)
	}
	if c.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", c.maxRetries)
	}
	if c.backoffBase <= 0 {
		t.Errorf("backoffBase = %v, want > 0", c.backoffBase)
	}
}

// Silence unused warnings when other tests reference json package indirectly.
var _ = json.Marshal
