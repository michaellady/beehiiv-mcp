package main

import (
	"context"
	"testing"
	"time"
)

type fakeStatsAPI struct {
	pub          Publication
	pubErr       error
	subscribers  int64
	subsErr      error
	posts        []Post
	postsErr     error
	engagement   EngagementSummary
	engageErr    error
	lastPostArgs struct {
		limit     int
		withStats bool
	}
}

func (f *fakeStatsAPI) GetPublication(ctx context.Context, id string) (Publication, error) {
	return f.pub, f.pubErr
}
func (f *fakeStatsAPI) CountSubscribers(ctx context.Context, id string) (int64, error) {
	return f.subscribers, f.subsErr
}
func (f *fakeStatsAPI) ListPostsWithStats(ctx context.Context, id string, limit int) ([]Post, error) {
	f.lastPostArgs.limit = limit
	f.lastPostArgs.withStats = true
	return f.posts, f.postsErr
}
func (f *fakeStatsAPI) GetEngagements(ctx context.Context, id string) (EngagementSummary, error) {
	return f.engagement, f.engageErr
}

func TestRunStats_HappyPath(t *testing.T) {
	api := &fakeStatsAPI{
		pub:         Publication{ID: "pub_1", Name: "Enterprise Vibe Code"},
		subscribers: 1200,
		posts: []Post{
			{ID: "p1", Title: "Hello", PublishDate: "2026-04-14", Opens: 1203, OpenRate: 0.38, Clicks: 56, ClickRate: 0.045, SubscriberGained: 12},
		},
		engagement: EngagementSummary{OpenRate: 0.423, ClickRate: 0.081},
	}
	store := newSnapshotStore(t.TempDir())

	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	out, err := runStats(context.Background(), api, store, "pub_1", statsInput{WindowDays: 30, PostLimit: 5}, now)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}

	if out.Publication.ID != "pub_1" || out.Publication.Name != "Enterprise Vibe Code" {
		t.Errorf("Publication = %+v", out.Publication)
	}
	if out.Subscribers.Current != 1200 {
		t.Errorf("Current subscribers = %d, want 1200", out.Subscribers.Current)
	}
	if out.Subscribers.HistorySufficient {
		t.Errorf("HistorySufficient should be false on first run (no prior snapshot)")
	}
	if out.Engagement.OpenRate != 0.423 {
		t.Errorf("OpenRate = %v, want 0.423", out.Engagement.OpenRate)
	}
	if len(out.RecentPosts) != 1 || out.RecentPosts[0].ID != "p1" {
		t.Errorf("RecentPosts = %+v", out.RecentPosts)
	}
	if out.FetchedAt.IsZero() {
		t.Errorf("FetchedAt should be set")
	}
	if out.SnapshotSavedTo == "" {
		t.Errorf("SnapshotSavedTo should be set")
	}
	if api.lastPostArgs.limit != 5 {
		t.Errorf("ListPosts limit = %d, want 5", api.lastPostArgs.limit)
	}
}

func TestRunStats_GrowthCalculatedFromPriorSnapshot(t *testing.T) {
	store := newSnapshotStore(t.TempDir())

	// Seed a snapshot from 30+ days ago with 1000 subscribers. We write it via
	// the store so the real asOf lookup path is exercised.
	old := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	_, err := store.Write(old, statsOutput{
		Subscribers: SubscriberStats{Current: 1000},
		FetchedAt:   old,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	api := &fakeStatsAPI{
		pub:         Publication{ID: "pub_1"},
		subscribers: 1200,
	}
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	out, err := runStats(context.Background(), api, store, "pub_1", statsInput{WindowDays: 30, PostLimit: 5}, now)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}

	if !out.Subscribers.HistorySufficient {
		t.Fatalf("HistorySufficient should be true when an older snapshot exists")
	}
	if out.Subscribers.DeltaCount != 200 {
		t.Errorf("DeltaCount = %d, want 200", out.Subscribers.DeltaCount)
	}
	if diff := out.Subscribers.DeltaPct - 20.0; diff < -0.001 || diff > 0.001 {
		t.Errorf("DeltaPct = %v, want 20.0", out.Subscribers.DeltaPct)
	}
	if out.Subscribers.DeltaWindowDays != 30 {
		t.Errorf("DeltaWindowDays = %d, want 30", out.Subscribers.DeltaWindowDays)
	}
}

func TestRunStats_GrowthInsufficientHistoryWhenOnlyRecentSnapshots(t *testing.T) {
	store := newSnapshotStore(t.TempDir())

	// Seed a snapshot from 3 days ago — not old enough for a 30-day window.
	recent := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	_, _ = store.Write(recent, statsOutput{Subscribers: SubscriberStats{Current: 1150}, FetchedAt: recent})

	api := &fakeStatsAPI{pub: Publication{ID: "pub_1"}, subscribers: 1200}
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	out, err := runStats(context.Background(), api, store, "pub_1", statsInput{WindowDays: 30, PostLimit: 5}, now)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if out.Subscribers.HistorySufficient {
		t.Errorf("HistorySufficient should be false; newest snapshot is only 3 days old")
	}
	if out.Subscribers.DeltaCount != 0 {
		t.Errorf("DeltaCount = %d, want 0 when history insufficient", out.Subscribers.DeltaCount)
	}
}

func TestRunStats_ZeroSubscribersSurvives(t *testing.T) {
	store := newSnapshotStore(t.TempDir())
	api := &fakeStatsAPI{pub: Publication{ID: "pub_1"}, subscribers: 0}
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)

	out, err := runStats(context.Background(), api, store, "pub_1", statsInput{WindowDays: 30, PostLimit: 5}, now)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if out.Subscribers.Current != 0 {
		t.Errorf("Current = %d, want 0", out.Subscribers.Current)
	}
}

func TestRunStats_DefaultsWhenInputZero(t *testing.T) {
	store := newSnapshotStore(t.TempDir())
	api := &fakeStatsAPI{pub: Publication{ID: "pub_1"}, subscribers: 100}
	now := time.Now()

	out, err := runStats(context.Background(), api, store, "pub_1", statsInput{}, now)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if out.Subscribers.DeltaWindowDays != defaultWindowDays {
		t.Errorf("DeltaWindowDays = %d, want default %d", out.Subscribers.DeltaWindowDays, defaultWindowDays)
	}
	if api.lastPostArgs.limit != defaultPostLimit {
		t.Errorf("Posts limit = %d, want default %d", api.lastPostArgs.limit, defaultPostLimit)
	}
}

func TestRunStats_PostLimitCapped(t *testing.T) {
	store := newSnapshotStore(t.TempDir())
	api := &fakeStatsAPI{pub: Publication{ID: "pub_1"}, subscribers: 100}
	now := time.Now()

	_, err := runStats(context.Background(), api, store, "pub_1",
		statsInput{PostLimit: 999}, now)
	if err != nil {
		t.Fatalf("runStats: %v", err)
	}
	if api.lastPostArgs.limit != maxPostLimit {
		t.Errorf("Posts limit = %d, want capped to %d", api.lastPostArgs.limit, maxPostLimit)
	}
}
