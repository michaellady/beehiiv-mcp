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
