package main

// Publication is the top-level beehiiv publication record (subset of fields
// we surface to tools).
type Publication struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Post is a published newsletter issue with its aggregate stats.
type Post struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	PublishDate      string  `json:"publish_date"` // YYYY-MM-DD
	Opens            int64   `json:"opens"`
	OpenRate         float64 `json:"open_rate"`   // 0..1
	Clicks           int64   `json:"clicks"`
	ClickRate        float64 `json:"click_rate"`  // 0..1
	SubscriberGained int64   `json:"subscriber_gained"`
}

// EngagementSummary is the publication-wide aggregate.
type EngagementSummary struct {
	OpenRate  float64 `json:"open_rate"`
	ClickRate float64 `json:"click_rate"`
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
