package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeAttributionAPI struct {
	subs []Subscription
	err  error
	// capture args
	lastSince time.Time
	lastMax   int
}

func (f *fakeAttributionAPI) ListSubscriptions(ctx context.Context, pubID string, since time.Time, max int) ([]Subscription, error) {
	f.lastSince = since
	f.lastMax = max
	if f.err != nil {
		return nil, f.err
	}
	// Return only those created on or after `since` so handler logic can assume
	// the API respects its bound.
	out := make([]Subscription, 0, len(f.subs))
	for _, s := range f.subs {
		if !s.Created.Before(since) {
			out = append(out, s)
		}
	}
	return out, nil
}

func TestClassifySource_YouTubeViaUTM(t *testing.T) {
	if got := classifySource(Subscription{UTMSource: "youtube.com"}); got != "youtube" {
		t.Errorf("got %q, want youtube", got)
	}
	if got := classifySource(Subscription{UTMSource: "YouTube"}); got != "youtube" {
		t.Errorf("case-insensitive: got %q, want youtube", got)
	}
}

func TestClassifySource_YouTubeViaReferrer(t *testing.T) {
	got := classifySource(Subscription{ReferringSite: "https://www.youtube.com/watch?v=abc"})
	if got != "youtube" {
		t.Errorf("got %q, want youtube", got)
	}
}

func TestClassifySource_LinkedInVariants(t *testing.T) {
	for _, s := range []Subscription{
		{UTMSource: "linkedin"},
		{UTMMedium: "linkedin-newsletter"},
		{ReferringSite: "https://www.linkedin.com/pulse/xyz"},
	} {
		if got := classifySource(s); got != "linkedin" {
			t.Errorf("%+v → %q, want linkedin", s, got)
		}
	}
}

func TestClassifySource_Direct(t *testing.T) {
	if got := classifySource(Subscription{}); got != "direct" {
		t.Errorf("all-empty → %q, want direct", got)
	}
}

func TestClassifySource_Referral(t *testing.T) {
	got := classifySource(Subscription{ReferringSite: "https://some-random-blog.example.com/post"})
	if got != "referral" {
		t.Errorf("unknown referrer → %q, want referral", got)
	}
}

func TestClassifySource_HackerNews(t *testing.T) {
	got := classifySource(Subscription{ReferringSite: "https://news.ycombinator.com/item?id=42"})
	if got != "hackernews" {
		t.Errorf("HN referrer → %q, want hackernews", got)
	}
}

func TestClassifySource_OrganicFallback(t *testing.T) {
	got := classifySource(Subscription{UTMMedium: "organic"})
	if got != "organic" {
		t.Errorf("organic medium → %q, want organic", got)
	}
}

func TestClassifySource_Other(t *testing.T) {
	got := classifySource(Subscription{UTMSource: "somewhere", UTMMedium: "paid"})
	if got != "other" {
		t.Errorf("unknown source → %q, want other", got)
	}
}

func TestRunAttribution_EmptyResult(t *testing.T) {
	api := &fakeAttributionAPI{}
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	out, err := runAttribution(context.Background(), api, "pub_1", attributionInput{}, now)
	if err != nil {
		t.Fatalf("runAttribution: %v", err)
	}
	if out.TotalSubsInWindow != 0 || len(out.BySource) != 0 {
		t.Errorf("empty result not empty: %+v", out)
	}
	if out.WindowDays != defaultAttributionWindow {
		t.Errorf("WindowDays = %d, want default %d", out.WindowDays, defaultAttributionWindow)
	}
}

func TestRunAttribution_BucketsAndPercentages(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	newSub := func(days int, s Subscription) Subscription {
		s.Created = now.Add(-time.Duration(days) * 24 * time.Hour)
		return s
	}
	api := &fakeAttributionAPI{subs: []Subscription{
		newSub(1, Subscription{UTMSource: "youtube"}),
		newSub(2, Subscription{UTMSource: "youtube"}),
		newSub(3, Subscription{ReferringSite: "https://youtube.com/"}),
		newSub(4, Subscription{UTMSource: "linkedin"}),
		newSub(5, Subscription{}), // direct
	}}

	out, err := runAttribution(context.Background(), api, "pub_1", attributionInput{WindowDays: 30}, now)
	if err != nil {
		t.Fatalf("runAttribution: %v", err)
	}
	if out.TotalSubsInWindow != 5 {
		t.Errorf("Total = %d, want 5", out.TotalSubsInWindow)
	}

	want := map[string]int64{"youtube": 3, "linkedin": 1, "direct": 1}
	got := map[string]int64{}
	for _, b := range out.BySource {
		got[b.Source] = b.Count
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("bucket %s: got %d, want %d", k, got[k], v)
		}
	}
	// Buckets are sorted descending by count.
	for i := 1; i < len(out.BySource); i++ {
		if out.BySource[i].Count > out.BySource[i-1].Count {
			t.Errorf("buckets not sorted desc: %+v", out.BySource)
			break
		}
	}
	// Percentages sum to ~100.
	var totalPct float64
	for _, b := range out.BySource {
		totalPct += b.Pct
	}
	if totalPct < 99.9 || totalPct > 100.1 {
		t.Errorf("pct sum = %v, want ~100", totalPct)
	}
}

func TestRunAttribution_WindowFilterSentToAPI(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	api := &fakeAttributionAPI{}
	_, err := runAttribution(context.Background(), api, "pub_1", attributionInput{WindowDays: 7}, now)
	if err != nil {
		t.Fatalf("runAttribution: %v", err)
	}
	wantSince := now.Add(-7 * 24 * time.Hour)
	if !api.lastSince.Equal(wantSince) {
		t.Errorf("since = %v, want %v", api.lastSince, wantSince)
	}
}

func TestRunAttribution_TopCampaignsAndReferrers(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	ago := func(d int) time.Time { return now.Add(-time.Duration(d) * time.Hour) }

	api := &fakeAttributionAPI{subs: []Subscription{
		{Created: ago(1), UTMCampaign: "launch_week", UTMSource: "youtube", ReferringSite: "https://youtube.com/x"},
		{Created: ago(2), UTMCampaign: "launch_week", UTMSource: "youtube", ReferringSite: "https://youtube.com/x"},
		{Created: ago(3), UTMCampaign: "newsletter", UTMSource: "linkedin", ReferringSite: "https://linkedin.com/y"},
		{Created: ago(4), UTMCampaign: "", UTMSource: "", ReferringSite: ""}, // direct, no campaign
	}}
	out, _ := runAttribution(context.Background(), api, "pub_1", attributionInput{}, now)

	if len(out.TopCampaigns) != 2 {
		t.Fatalf("TopCampaigns = %+v, want 2 entries (empty campaigns filtered)", out.TopCampaigns)
	}
	if out.TopCampaigns[0].Key != "launch_week" || out.TopCampaigns[0].Count != 2 {
		t.Errorf("top campaign wrong: %+v", out.TopCampaigns[0])
	}
	if len(out.TopReferringSites) < 1 {
		t.Fatalf("TopReferringSites empty")
	}
}

func TestRunAttribution_DailyCountsFilledForWindow(t *testing.T) {
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	api := &fakeAttributionAPI{subs: []Subscription{
		{Created: now.Add(-1 * 24 * time.Hour)},
		{Created: now.Add(-1 * 24 * time.Hour)},
		{Created: now.Add(-3 * 24 * time.Hour)},
	}}
	out, _ := runAttribution(context.Background(), api, "pub_1", attributionInput{WindowDays: 5}, now)

	if len(out.DailyCounts) != 5 {
		t.Fatalf("DailyCounts len = %d, want 5 (one per window day)", len(out.DailyCounts))
	}
	// Dates should be ascending.
	for i := 1; i < len(out.DailyCounts); i++ {
		if out.DailyCounts[i].Date < out.DailyCounts[i-1].Date {
			t.Errorf("daily counts not ascending: %+v", out.DailyCounts)
			break
		}
	}
	// Total across days should equal total subs in window.
	var sum int64
	for _, d := range out.DailyCounts {
		sum += d.Count
	}
	if sum != 3 {
		t.Errorf("sum of daily counts = %d, want 3", sum)
	}
}

func TestRunAttribution_DefaultsAndCapsLimit(t *testing.T) {
	now := time.Now()
	api := &fakeAttributionAPI{}
	// Default limit should be applied.
	_, _ = runAttribution(context.Background(), api, "pub_1", attributionInput{}, now)
	if api.lastMax != defaultAttributionLimit {
		t.Errorf("default limit not propagated: got %d, want %d", api.lastMax, defaultAttributionLimit)
	}
	// Over-cap should be clamped.
	_, _ = runAttribution(context.Background(), api, "pub_1", attributionInput{Limit: maxAttributionLimit * 10}, now)
	if api.lastMax != maxAttributionLimit {
		t.Errorf("over-cap limit not clamped: got %d, want %d", api.lastMax, maxAttributionLimit)
	}
}

func TestRunAttribution_APIErrorPropagates(t *testing.T) {
	sentinel := errors.New("boom")
	api := &fakeAttributionAPI{err: sentinel}
	_, err := runAttribution(context.Background(), api, "pub_1", attributionInput{}, time.Now())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap %v", err, sentinel)
	}
}
