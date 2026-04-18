package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

type sample struct {
	Subscribers int    `json:"subscribers"`
	Note        string `json:"note"`
}

func TestSnapshot_WriteCreatesFileAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	store := newSnapshotStore(dir)
	when := time.Date(2026, 4, 18, 20, 15, 0, 0, time.UTC)
	in := sample{Subscribers: 1234, Note: "hello"}

	path, err := store.Write(when, in)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}

	var out sample
	found, err := store.LoadClosestOlder(when.Add(time.Minute), &out)
	if err != nil {
		t.Fatalf("LoadClosestOlder: %v", err)
	}
	if !found {
		t.Fatalf("expected a snapshot to be found")
	}
	if out != in {
		t.Errorf("round-trip mismatch: got %+v, want %+v", out, in)
	}
}

func TestSnapshot_PruneKeepsNewestN(t *testing.T) {
	dir := t.TempDir()
	store := newSnapshotStore(dir)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if _, err := store.Write(base.Add(time.Duration(i)*time.Hour), sample{Subscribers: i}); err != nil {
			t.Fatalf("Write #%d: %v", i, err)
		}
	}

	deleted, err := store.Prune(3)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	remaining, _ := filepath.Glob(filepath.Join(dir, "stats-*.json"))
	sort.Strings(remaining)
	if len(remaining) != 3 {
		t.Fatalf("remaining = %d, want 3", len(remaining))
	}
	// The three surviving files should be the latest three timestamps (hours 2, 3, 4).
	for i, p := range remaining {
		want := base.Add(time.Duration(i+2) * time.Hour).UTC()
		if !contains(p, snapshotTimestamp(want)) {
			t.Errorf("remaining[%d] = %q, want to include %q", i, p, snapshotTimestamp(want))
		}
	}
}

func TestSnapshot_PruneWithFewerFilesIsNoop(t *testing.T) {
	dir := t.TempDir()
	store := newSnapshotStore(dir)

	for i := 0; i < 2; i++ {
		_, _ = store.Write(time.Now().Add(time.Duration(i)*time.Second), sample{Subscribers: i})
	}

	deleted, err := store.Prune(30)
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestSnapshot_LoadClosestOlderPicksLatestBeforeAsOf(t *testing.T) {
	dir := t.TempDir()
	store := newSnapshotStore(dir)

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i, s := range []int{100, 200, 300, 400} {
		if _, err := store.Write(base.Add(time.Duration(i)*24*time.Hour), sample{Subscribers: s}); err != nil {
			t.Fatalf("Write #%d: %v", i, err)
		}
	}

	// Ask for a snapshot "as of" day 2.5 — should return the day-2 snapshot (Subscribers=300).
	asOf := base.Add(2*24*time.Hour + 12*time.Hour)
	var out sample
	found, err := store.LoadClosestOlder(asOf, &out)
	if err != nil {
		t.Fatalf("LoadClosestOlder: %v", err)
	}
	if !found {
		t.Fatal("expected to find a snapshot older than asOf")
	}
	if out.Subscribers != 300 {
		t.Errorf("Subscribers = %d, want 300 (day 2)", out.Subscribers)
	}
}

func TestSnapshot_LoadClosestOlderReturnsFalseWhenNoneOlder(t *testing.T) {
	dir := t.TempDir()
	store := newSnapshotStore(dir)

	when := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	_, _ = store.Write(when, sample{Subscribers: 42})

	// Ask "as of" a time before any snapshot exists.
	var out sample
	found, err := store.LoadClosestOlder(when.Add(-time.Hour), &out)
	if err != nil {
		t.Fatalf("LoadClosestOlder: %v", err)
	}
	if found {
		t.Errorf("found should be false when no snapshot predates asOf")
	}
}

func TestSnapshot_DirCreatedOnFirstWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "snaps")
	// dir does not exist yet
	store := newSnapshotStore(dir)
	if _, err := store.Write(time.Now(), sample{Subscribers: 1}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("snapshot dir should have been created: %v", err)
	}
}
