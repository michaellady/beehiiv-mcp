package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const defaultBaseURL = "https://api.beehiiv.com/v2"

// clientOpts configures a client. Zero fields fall back to defaults.
type clientOpts struct {
	APIKey      string
	BaseURL     string
	HTTPClient  *http.Client
	BackoffBase time.Duration
	MaxRetries  int
}

// client is a thin wrapper around net/http for the beehiiv v2 REST API.
// It handles Bearer auth, 429 retry with exponential backoff, and pagination.
type client struct {
	apiKey      string
	baseURL     string
	httpClient  *http.Client
	backoffBase time.Duration
	maxRetries  int
}

func newClient(opts clientOpts) *client {
	c := &client{
		apiKey:      opts.APIKey,
		baseURL:     opts.BaseURL,
		httpClient:  opts.HTTPClient,
		backoffBase: opts.BackoffBase,
		maxRetries:  opts.MaxRetries,
	}
	if c.baseURL == "" {
		c.baseURL = defaultBaseURL
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if c.backoffBase <= 0 {
		c.backoffBase = 500 * time.Millisecond
	}
	if c.maxRetries <= 0 {
		c.maxRetries = 3
	}
	return c
}

// do performs a single request (with retries) and decodes the JSON response
// into out. Returns a wrapped error containing status + body on 4xx/5xx.
func (c *client) do(ctx context.Context, method, path string, query map[string]string, out any) error {
	u, err := c.buildURL(path, query)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if !sleepWithContext(ctx, c.backoff(attempt)) {
				return ctx.Err()
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			lastErr = fmt.Errorf("http %d: %s", resp.StatusCode, truncateBody(body))
			if attempt == c.maxRetries {
				return lastErr
			}
			if !sleepWithContext(ctx, c.backoff(attempt)) {
				return ctx.Err()
			}
			continue
		case resp.StatusCode >= 400:
			return fmt.Errorf("http %d: %s", resp.StatusCode, truncateBody(body))
		}

		if out != nil && len(body) > 0 {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}
	return lastErr
}

// buildURL joins base + path and appends any query parameters.
func (c *client) buildURL(path string, query map[string]string) (string, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("bad path %q: %w", path, err)
	}
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

// backoff returns the sleep duration before the given attempt (0 means just-failed).
// Uses exponential backoff: base, 2×base, 4×base, …
func (c *client) backoff(attempt int) time.Duration {
	// attempt=0 is the first failure; sleep base before attempt=1.
	mult := 1
	for i := 0; i < attempt; i++ {
		mult *= 2
	}
	return time.Duration(mult) * c.backoffBase
}

func truncateBody(b []byte) string {
	const max = 500
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "…"
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// pagedResponse is the minimal shape we need to drive pagination.
// Beehiiv's list endpoints return `page` and `total_pages` alongside `data`.
type pagedResponse[T any] struct {
	Data       []T `json:"data"`
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
}

// paginate walks page=1..total_pages for a list endpoint, appending each page's
// data slice into out. The first request omits the page parameter (so the server's
// default page=1 is used); subsequent requests include page=N.
func paginate[T any](ctx context.Context, c *client, path string, query map[string]string, out *[]T) error {
	q := copyQuery(query)
	page := 0
	for {
		var resp pagedResponse[T]
		if err := c.do(ctx, "GET", path, q, &resp); err != nil {
			return err
		}
		*out = append(*out, resp.Data...)

		if resp.TotalPages <= 1 {
			return nil
		}
		if page == 0 {
			page = 2
		} else {
			page++
		}
		if page > resp.TotalPages {
			return nil
		}
		q = copyQuery(query)
		q["page"] = strconv.Itoa(page)
	}
}

func copyQuery(src map[string]string) map[string]string {
	out := make(map[string]string, len(src)+1)
	for k, v := range src {
		out[k] = v
	}
	return out
}
