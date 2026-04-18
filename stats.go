package main

import (
	"context"
	"fmt"
	"time"
)

// Defaults + caps for the beehiiv_stats tool input.
const (
	defaultWindowDays = 30
	defaultPostLimit  = 5
	maxPostLimit      = 20
)

// statsAPI is the slice of the beehiiv API surface that runStats needs.
// Handlers depend on narrow interfaces so tests can supply focused fakes.
type statsAPI interface {
	GetPublication(ctx context.Context, id string) (Publication, error)
	CountSubscribers(ctx context.Context, id string) (int64, error)
	ListPostsWithStats(ctx context.Context, id string, limit int) ([]Post, error)
	GetEngagements(ctx context.Context, id string) (EngagementSummary, error)
}

// statsInput is the tool input as parsed from MCP JSON.
type statsInput struct {
	WindowDays int `json:"window_days,omitempty"`
	PostLimit  int `json:"post_limit,omitempty"`
}

// statsOutput is the tool output serialized back to Claude.
type statsOutput struct {
	Publication     Publication       `json:"publication"`
	Subscribers     SubscriberStats   `json:"subscribers"`
	Engagement      EngagementSummary `json:"engagement"`
	RecentPosts     []Post            `json:"recent_posts"`
	FetchedAt       time.Time         `json:"fetched_at"`
	SnapshotSavedTo string            `json:"snapshot_saved_to,omitempty"`
}

// SubscriberStats carries current count plus a window-over-window delta.
type SubscriberStats struct {
	Current           int64   `json:"current"`
	DeltaWindowDays   int     `json:"delta_window_days"`
	DeltaCount        int64   `json:"delta_count"`
	DeltaPct          float64 `json:"delta_pct"`
	HistorySufficient bool    `json:"history_sufficient"`
}

// runStats fetches fresh data, computes growth vs. the closest prior snapshot,
// writes a new snapshot, and returns the structured output.
// `now` is injected so tests have deterministic timestamps.
func runStats(
	ctx context.Context,
	api statsAPI,
	store *snapshotStore,
	pubID string,
	in statsInput,
	now time.Time,
) (statsOutput, error) {
	window := in.WindowDays
	if window <= 0 {
		window = defaultWindowDays
	}
	limit := in.PostLimit
	if limit <= 0 {
		limit = defaultPostLimit
	}
	if limit > maxPostLimit {
		limit = maxPostLimit
	}

	pub, err := api.GetPublication(ctx, pubID)
	if err != nil {
		return statsOutput{}, fmt.Errorf("get publication: %w", err)
	}
	subs, err := api.CountSubscribers(ctx, pubID)
	if err != nil {
		return statsOutput{}, fmt.Errorf("count subscribers: %w", err)
	}
	posts, err := api.ListPostsWithStats(ctx, pubID, limit)
	if err != nil {
		return statsOutput{}, fmt.Errorf("list posts: %w", err)
	}
	eng, err := api.GetEngagements(ctx, pubID)
	if err != nil {
		return statsOutput{}, fmt.Errorf("get engagements: %w", err)
	}

	subStats := SubscriberStats{Current: subs, DeltaWindowDays: window}

	// Look up a prior snapshot whose timestamp is older than now-window.
	if store != nil {
		windowStart := now.Add(-time.Duration(window) * 24 * time.Hour)
		var prior statsOutput
		found, err := store.LoadClosestOlder(windowStart, &prior)
		if err != nil {
			return statsOutput{}, fmt.Errorf("load prior snapshot: %w", err)
		}
		if found {
			subStats.HistorySufficient = true
			subStats.DeltaCount = subs - prior.Subscribers.Current
			if prior.Subscribers.Current > 0 {
				subStats.DeltaPct = float64(subStats.DeltaCount) / float64(prior.Subscribers.Current) * 100
			}
		}
	}

	out := statsOutput{
		Publication: pub,
		Subscribers: subStats,
		Engagement:  eng,
		RecentPosts: posts,
		FetchedAt:   now.UTC(),
	}

	if store != nil {
		path, err := store.Write(now, out)
		if err != nil {
			return statsOutput{}, fmt.Errorf("write snapshot: %w", err)
		}
		out.SnapshotSavedTo = path
	}

	return out, nil
}
