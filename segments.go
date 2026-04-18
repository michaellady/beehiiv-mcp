package main

import (
	"context"
	"fmt"
	"time"
)

// staleAfter is how long a dynamic segment can go without recalculation
// before we flag it as stale.
const staleAfter = 24 * time.Hour

type segmentsAPI interface {
	ListSegments(ctx context.Context, pubID string) ([]Segment, error)
}

type segmentsOutput struct {
	Segments          []SegmentReport `json:"segments"`
	TotalSegmentCount int             `json:"total_segment_count"`
	FetchedAt         time.Time       `json:"fetched_at"`
}

// SegmentReport is a Segment with a derived staleness flag.
type SegmentReport struct {
	Segment
	Stale bool `json:"stale"`
}

func runSegments(ctx context.Context, api segmentsAPI, pubID string, now time.Time) (segmentsOutput, error) {
	segs, err := api.ListSegments(ctx, pubID)
	if err != nil {
		return segmentsOutput{}, fmt.Errorf("list segments: %w", err)
	}
	reports := make([]SegmentReport, len(segs))
	for i, s := range segs {
		reports[i] = SegmentReport{
			Segment: s,
			Stale:   isSegmentStale(s, now),
		}
	}
	return segmentsOutput{
		Segments:          reports,
		TotalSegmentCount: len(segs),
		FetchedAt:         now.UTC(),
	}, nil
}

// isSegmentStale returns true only for dynamic segments whose last recalculation
// is older than staleAfter. Static segments are curated lists — they never go stale.
func isSegmentStale(s Segment, now time.Time) bool {
	if s.Type != "dynamic" {
		return false
	}
	if s.LastCalculatedAt.IsZero() {
		return false
	}
	return now.Sub(s.LastCalculatedAt) > staleAfter
}
