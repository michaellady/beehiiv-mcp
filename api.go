package main

import (
	"context"
	"strconv"
	"time"
)

// This file maps the narrow per-tool interfaces (statsAPI, automationsAPI,
// segmentsAPI, webhooksAPI) onto the HTTP client. The on-the-wire structs
// live here and are translated to the domain types defined in models.go
// so handlers never touch the envelope shapes.

// --- Publication ---

type wirePublication struct {
	Data Publication `json:"data"`
}

func (c *client) GetPublication(ctx context.Context, id string) (Publication, error) {
	var env wirePublication
	if err := c.do(ctx, "GET", "/publications/"+id, nil, &env); err != nil {
		return Publication{}, err
	}
	return env.Data, nil
}

// --- Publication stats (subscribers + aggregate engagement) ---
//
// The /publications/{id}?expand[]=stats endpoint returns everything the
// dashboard needs in one call: active subscriber count, average open/click
// rates (percent), and volume totals. Replaces what used to be two separate
// calls (/subscriptions and /engagement, neither of which actually returns
// aggregate counts in a usable shape).

type wirePublicationStats struct {
	Data struct {
		Publication
		Stats PublicationStats `json:"stats"`
	} `json:"data"`
}

func (c *client) GetPublicationStats(ctx context.Context, id string) (PublicationStats, error) {
	var env wirePublicationStats
	if err := c.do(ctx, "GET", "/publications/"+id,
		map[string]string{"expand[]": "stats"}, &env); err != nil {
		return PublicationStats{}, err
	}
	return env.Data.Stats, nil
}

// --- Posts with stats ---
//
// beehiiv posts expose stats under `stats.email.*` when ?expand[]=stats is
// set. We map the subset we surface to Claude. Rate fields are percents
// (e.g. 37.14) as beehiiv reports them, not 0..1 ratios.

type wirePostStats struct {
	Email struct {
		Recipients int64   `json:"recipients"`
		Opens      int64   `json:"opens"`
		OpenRate   float64 `json:"open_rate"`
		Clicks     int64   `json:"clicks"`
		ClickRate  float64 `json:"click_rate"`
	} `json:"email"`
}

type wirePost struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	PublishDate int64         `json:"publish_date"` // unix seconds
	Stats       wirePostStats `json:"stats"`
}

func (c *client) ListPostsWithStats(ctx context.Context, id string, limit int) ([]Post, error) {
	query := map[string]string{
		"expand[]":  "stats",
		"limit":     strconv.Itoa(limit),
		"order_by":  "publish_date",
		"direction": "desc",
		"status":    "confirmed",
	}
	var out []wirePost
	if err := paginate(ctx, c, "/publications/"+id+"/posts", query, &out); err != nil {
		return nil, err
	}
	// Cap at the requested limit — paginate may have walked past it on a partial page.
	if len(out) > limit {
		out = out[:limit]
	}
	posts := make([]Post, len(out))
	for i, p := range out {
		posts[i] = Post{
			ID:          p.ID,
			Title:       p.Title,
			PublishDate: unixToDate(p.PublishDate),
			Recipients:  p.Stats.Email.Recipients,
			Opens:       p.Stats.Email.Opens,
			OpenRate:    p.Stats.Email.OpenRate,
			Clicks:      p.Stats.Email.Clicks,
			ClickRate:   p.Stats.Email.ClickRate,
		}
	}
	return posts, nil
}

// --- Automations ---

type wireAutomation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Trigger    string `json:"trigger"`
	EmailCount int    `json:"email_count"`
}

func (c *client) ListAutomations(ctx context.Context, pubID string) ([]Automation, error) {
	var raw []wireAutomation
	if err := paginate(ctx, c, "/publications/"+pubID+"/automations", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Automation, len(raw))
	for i, a := range raw {
		out[i] = Automation(a)
	}
	return out, nil
}

type wireJourneyStats struct {
	Data JourneyStats `json:"data"`
}

func (c *client) GetJourneyStats(ctx context.Context, pubID, autoID string) (JourneyStats, error) {
	var env wireJourneyStats
	if err := c.do(ctx, "GET", "/publications/"+pubID+"/automations/"+autoID+"/journeys", nil, &env); err != nil {
		return JourneyStats{}, err
	}
	return env.Data, nil
}

type wireAutomationEmail struct {
	ID        string  `json:"id"`
	Subject   string  `json:"subject"`
	Position  int     `json:"position"`
	DelayDays int     `json:"delay_days"`
	Opens     int64   `json:"opens"`
	OpenRate  float64 `json:"open_rate"`
	Clicks    int64   `json:"clicks"`
	ClickRate float64 `json:"click_rate"`
}

func (c *client) ListAutomationEmails(ctx context.Context, pubID, autoID string) ([]AutomationEmail, error) {
	var raw []wireAutomationEmail
	if err := paginate(ctx, c, "/publications/"+pubID+"/automations/"+autoID+"/emails", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]AutomationEmail, len(raw))
	for i, e := range raw {
		out[i] = AutomationEmail(e)
	}
	return out, nil
}

// --- Segments ---

type wireSegment struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	MemberCount      int64  `json:"member_count"`
	LastCalculatedAt int64  `json:"last_calculated_at"` // unix seconds
}

func (c *client) ListSegments(ctx context.Context, pubID string) ([]Segment, error) {
	var raw []wireSegment
	if err := paginate(ctx, c, "/publications/"+pubID+"/segments", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Segment, len(raw))
	for i, s := range raw {
		out[i] = Segment{
			ID:               s.ID,
			Name:             s.Name,
			Type:             s.Type,
			MemberCount:      s.MemberCount,
			LastCalculatedAt: unixToTime(s.LastCalculatedAt),
		}
	}
	return out, nil
}

// --- Webhooks ---

type wireWebhook struct {
	ID                 string   `json:"id"`
	URL                string   `json:"url"`
	Status             string   `json:"status"`
	Events             []string `json:"events"`
	LastDeliveryAt     int64    `json:"last_delivery_at"`
	LastDeliveryStatus string   `json:"last_delivery_status"`
}

func (c *client) ListWebhooks(ctx context.Context, pubID string) ([]Webhook, error) {
	var raw []wireWebhook
	if err := paginate(ctx, c, "/publications/"+pubID+"/webhooks", nil, &raw); err != nil {
		return nil, err
	}
	out := make([]Webhook, len(raw))
	for i, w := range raw {
		out[i] = Webhook{
			ID:                 w.ID,
			URL:                w.URL,
			Status:             w.Status,
			Events:             w.Events,
			LastDeliveryAt:     unixToTime(w.LastDeliveryAt),
			LastDeliveryStatus: fallback(w.LastDeliveryStatus, "unknown"),
		}
	}
	return out, nil
}

// --- helpers ---

func unixToTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

func unixToDate(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

func fallback(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
