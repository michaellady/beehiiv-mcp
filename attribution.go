package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultAttributionWindow = 30
	defaultAttributionLimit  = 500
	maxAttributionLimit      = 5000
	topCampaignsCount        = 10
	topReferrersCount        = 10
)

// attributionAPI is the slice of the beehiiv API surface runAttribution uses.
// ListSubscriptions walks cursor pagination and returns subscribers created
// at or after `since`, up to `max` records.
type attributionAPI interface {
	ListSubscriptions(ctx context.Context, pubID string, since time.Time, max int) ([]Subscription, error)
}

type attributionInput struct {
	WindowDays int `json:"window_days,omitempty"`
	Limit      int `json:"limit,omitempty"`
}

type attributionOutput struct {
	WindowDays        int                 `json:"window_days"`
	TotalSubsInWindow int64               `json:"total_subs_in_window"`
	BySource          []attributionBucket `json:"by_source"`
	TopCampaigns      []kvCount           `json:"top_campaigns"`
	TopReferringSites []kvCount           `json:"top_referring_sites"`
	DailyCounts       []dailyCount        `json:"daily_counts"`
	Truncated         bool                `json:"truncated"`
	FetchedAt         time.Time           `json:"fetched_at"`
}

type attributionBucket struct {
	Source string  `json:"source"`
	Count  int64   `json:"count"`
	Pct    float64 `json:"pct"`
}

type kvCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type dailyCount struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int64  `json:"count"`
}

// classifySource buckets a subscriber into a single acquisition channel.
// Rules are checked in order; first match wins. Case-insensitive.
func classifySource(s Subscription) string {
	src := strings.ToLower(s.UTMSource)
	med := strings.ToLower(s.UTMMedium)
	ch := strings.ToLower(s.UTMChannel)
	ref := strings.ToLower(s.ReferringSite)

	anyContains := func(needle string, fields ...string) bool {
		for _, f := range fields {
			if strings.Contains(f, needle) {
				return true
			}
		}
		return false
	}

	switch {
	case anyContains("youtube", src, med, ch, ref):
		return "youtube"
	case anyContains("linkedin", src, med, ch, ref):
		return "linkedin"
	case anyContains("twitter", src, med, ch, ref), anyContains("x.com", ref):
		return "twitter"
	case anyContains("threads", src, med, ch, ref):
		return "threads"
	case anyContains("instagram", src, med, ch, ref):
		return "instagram"
	case anyContains("facebook", src, med, ch, ref):
		return "facebook"
	case anyContains("reddit", src, med, ch, ref):
		return "reddit"
	case anyContains("hacker", ref), anyContains("ycombinator", ref):
		return "hackernews"
	case anyContains("organic", med):
		return "organic"
	case src == "" && med == "" && ch == "" && ref == "":
		return "direct"
	case ref != "":
		return "referral"
	default:
		return "other"
	}
}

func runAttribution(
	ctx context.Context,
	api attributionAPI,
	pubID string,
	in attributionInput,
	now time.Time,
) (attributionOutput, error) {
	window := in.WindowDays
	if window <= 0 {
		window = defaultAttributionWindow
	}
	limit := in.Limit
	if limit <= 0 {
		limit = defaultAttributionLimit
	}
	if limit > maxAttributionLimit {
		limit = maxAttributionLimit
	}

	since := now.Add(-time.Duration(window) * 24 * time.Hour)
	subs, err := api.ListSubscriptions(ctx, pubID, since, limit)
	if err != nil {
		return attributionOutput{}, fmt.Errorf("list subscriptions: %w", err)
	}

	total := int64(len(subs))

	// Per-source bucket.
	bucketCounts := map[string]int64{}
	campaignCounts := map[string]int64{}
	referrerCounts := map[string]int64{}
	dayCounts := map[string]int64{}

	for _, s := range subs {
		bucketCounts[classifySource(s)]++
		if s.UTMCampaign != "" {
			campaignCounts[s.UTMCampaign]++
		}
		if s.ReferringSite != "" {
			referrerCounts[s.ReferringSite]++
		}
		day := s.Created.UTC().Format("2006-01-02")
		dayCounts[day]++
	}

	out := attributionOutput{
		WindowDays:        window,
		TotalSubsInWindow: total,
		BySource:          summarizeBuckets(bucketCounts, total),
		TopCampaigns:      topN(campaignCounts, topCampaignsCount),
		TopReferringSites: topN(referrerCounts, topReferrersCount),
		DailyCounts:       fillDailyCounts(dayCounts, now, window),
		Truncated:         total >= int64(limit),
		FetchedAt:         now.UTC(),
	}
	return out, nil
}

// summarizeBuckets turns the counts map into a slice sorted by count desc,
// computing the percentage of each bucket vs the total.
func summarizeBuckets(counts map[string]int64, total int64) []attributionBucket {
	out := make([]attributionBucket, 0, len(counts))
	for src, n := range counts {
		pct := 0.0
		if total > 0 {
			pct = float64(n) / float64(total) * 100
		}
		out = append(out, attributionBucket{Source: src, Count: n, Pct: roundPct(pct)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Source < out[j].Source
	})
	return out
}

// topN picks the N highest-count entries, ties broken by key string.
func topN(counts map[string]int64, n int) []kvCount {
	out := make([]kvCount, 0, len(counts))
	for k, v := range counts {
		out = append(out, kvCount{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// fillDailyCounts returns exactly windowDays entries ending on the day
// containing `now`, filling zeros so consumers get a stable ordered series.
func fillDailyCounts(counts map[string]int64, now time.Time, windowDays int) []dailyCount {
	endDay := now.UTC().Truncate(24 * time.Hour)
	startDay := endDay.Add(-time.Duration(windowDays-1) * 24 * time.Hour)
	days := make([]dailyCount, 0, windowDays)
	for d := startDay; !d.After(endDay); d = d.Add(24 * time.Hour) {
		key := d.Format("2006-01-02")
		days = append(days, dailyCount{Date: key, Count: counts[key]})
	}
	return days
}

func roundPct(v float64) float64 {
	// Two-decimal rounding keeps JSON compact without losing meaningful precision.
	return float64(int(v*100+0.5)) / 100
}
