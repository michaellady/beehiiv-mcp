package main

import "time"

// Publication is the top-level beehiiv publication record (subset of fields
// we surface to tools).
type Publication struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Post is a published newsletter issue with its aggregate stats.
// Rate fields are PERCENT values as beehiiv reports them (e.g. 37.14 = 37.14%),
// not 0..1 ratios.
type Post struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	PublishDate string  `json:"publish_date"` // YYYY-MM-DD
	Recipients  int64   `json:"recipients"`
	Opens       int64   `json:"opens"`
	OpenRate    float64 `json:"open_rate"`
	Clicks      int64   `json:"clicks"`
	ClickRate   float64 `json:"click_rate"`
}

// PublicationStats is the subscribers + engagement aggregate returned by
// GET /publications/{id}?expand[]=stats. We surface the slice the dashboard
// cares about; beehiiv exposes several more fields we don't need yet.
type PublicationStats struct {
	ActiveSubscriptions        int64   `json:"active_subscriptions"`
	ActiveFreeSubscriptions    int64   `json:"active_free_subscriptions"`
	ActivePremiumSubscriptions int64   `json:"active_premium_subscriptions"`
	AverageOpenRate            float64 `json:"average_open_rate"`  // percent, e.g. 44.45
	AverageClickRate           float64 `json:"average_click_rate"` // percent
	TotalSent                  int64   `json:"total_sent"`
	TotalDelivered             int64   `json:"total_delivered"`
	TotalUniqueOpened          int64   `json:"total_unique_opened"`
	TotalClicked               int64   `json:"total_clicked"`
}

// EngagementSummary is the slice of PublicationStats we show in tool output.
type EngagementSummary struct {
	OpenRate  float64 `json:"open_rate"`  // percent
	ClickRate float64 `json:"click_rate"` // percent
}

// Automation is a beehiiv automation flow (welcome series, drip, etc.).
type Automation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`  // active | paused | draft
	Trigger    string `json:"trigger"` // subscription_created, tag_added, ...
	EmailCount int    `json:"email_count"`
}

// JourneyStats summarizes subscriber flow through an automation.
type JourneyStats struct {
	Active    int64 `json:"active_subscriptions"`
	Completed int64 `json:"completed_subscriptions"`
	Exited    int64 `json:"exited_subscriptions"`
}

// AutomationEmail is one message in an automation sequence.
type AutomationEmail struct {
	ID        string  `json:"id"`
	Subject   string  `json:"subject"`
	Position  int     `json:"position"`
	DelayDays int     `json:"delay_days"`
	Opens     int64   `json:"opens"`
	OpenRate  float64 `json:"open_rate"`
	Clicks    int64   `json:"clicks"`
	ClickRate float64 `json:"click_rate"`
}

// Segment is a saved subscriber filter.
type Segment struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"` // static | dynamic
	MemberCount      int64     `json:"member_count"`
	LastCalculatedAt time.Time `json:"last_calculated_at"`
}

// Webhook is a registered outbound event listener.
type Webhook struct {
	ID                 string    `json:"id"`
	URL                string    `json:"url"`
	Status             string    `json:"status"` // active | paused
	Events             []string  `json:"events"`
	LastDeliveryAt     time.Time `json:"last_delivery_at"`
	LastDeliveryStatus string    `json:"last_delivery_status"` // success | failure | unknown
}
