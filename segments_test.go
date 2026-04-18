package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSegmentsAPI struct {
	segs []Segment
	err  error
}

func (f *fakeSegmentsAPI) ListSegments(ctx context.Context, pubID string) ([]Segment, error) {
	return f.segs, f.err
}

func TestRunSegments_EmptyList(t *testing.T) {
	api := &fakeSegmentsAPI{}
	now := time.Now()
	out, err := runSegments(context.Background(), api, "pub_1", now)
	if err != nil {
		t.Fatalf("runSegments: %v", err)
	}
	if len(out.Segments) != 0 {
		t.Errorf("Segments = %d, want 0", len(out.Segments))
	}
	if out.TotalSegmentCount != 0 {
		t.Errorf("TotalSegmentCount = %d, want 0", out.TotalSegmentCount)
	}
}

func TestRunSegments_StaticSegmentNeverStale(t *testing.T) {
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	old := now.Add(-30 * 24 * time.Hour)
	api := &fakeSegmentsAPI{segs: []Segment{
		{ID: "s1", Name: "Paid subscribers", Type: "static", MemberCount: 47, LastCalculatedAt: old},
	}}

	out, err := runSegments(context.Background(), api, "pub_1", now)
	if err != nil {
		t.Fatalf("runSegments: %v", err)
	}
	if out.Segments[0].Stale {
		t.Errorf("static segment should never be stale regardless of age")
	}
}

func TestRunSegments_DynamicFreshIsNotStale(t *testing.T) {
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	api := &fakeSegmentsAPI{segs: []Segment{
		{ID: "s2", Name: "Recent opens", Type: "dynamic", MemberCount: 300, LastCalculatedAt: now.Add(-2 * time.Hour)},
	}}

	out, err := runSegments(context.Background(), api, "pub_1", now)
	if err != nil {
		t.Fatalf("runSegments: %v", err)
	}
	if out.Segments[0].Stale {
		t.Errorf("dynamic segment recalculated 2h ago should not be stale")
	}
}

func TestRunSegments_DynamicStaleAfter24h(t *testing.T) {
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	api := &fakeSegmentsAPI{segs: []Segment{
		{ID: "s3", Name: "High engagement", Type: "dynamic", MemberCount: 120, LastCalculatedAt: now.Add(-48 * time.Hour)},
	}}

	out, err := runSegments(context.Background(), api, "pub_1", now)
	if err != nil {
		t.Fatalf("runSegments: %v", err)
	}
	if !out.Segments[0].Stale {
		t.Errorf("dynamic segment recalculated 48h ago should be stale")
	}
}

func TestRunSegments_TotalCountAndFetchedAt(t *testing.T) {
	now := time.Date(2026, 4, 18, 20, 0, 0, 0, time.UTC)
	api := &fakeSegmentsAPI{segs: []Segment{
		{ID: "s1", Type: "static"},
		{ID: "s2", Type: "dynamic", LastCalculatedAt: now},
		{ID: "s3", Type: "dynamic", LastCalculatedAt: now.Add(-time.Hour)},
	}}

	out, err := runSegments(context.Background(), api, "pub_1", now)
	if err != nil {
		t.Fatalf("runSegments: %v", err)
	}
	if out.TotalSegmentCount != 3 {
		t.Errorf("TotalSegmentCount = %d, want 3", out.TotalSegmentCount)
	}
	if !out.FetchedAt.Equal(now.UTC()) {
		t.Errorf("FetchedAt = %v, want %v", out.FetchedAt, now.UTC())
	}
}

func TestRunSegments_ListErrorPropagates(t *testing.T) {
	sentinel := errors.New("upstream down")
	api := &fakeSegmentsAPI{err: sentinel}
	_, err := runSegments(context.Background(), api, "pub_1", time.Now())
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want to wrap %v", err, sentinel)
	}
}
