package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// snapshotStore writes timestamped JSON snapshots to a directory and exposes
// prune + closest-older lookup for trend-delta computation.
type snapshotStore struct {
	dir string
}

func newSnapshotStore(dir string) *snapshotStore {
	return &snapshotStore{dir: dir}
}

// snapshotTimestamp renders a time.Time as a filename-safe UTC timestamp.
// Colons from RFC3339 are replaced with hyphens so the string can live in
// filenames cross-platform.
func snapshotTimestamp(t time.Time) string {
	return strings.ReplaceAll(t.UTC().Format(time.RFC3339), ":", "-")
}

// Write serializes data as JSON to stats-<timestamp>.json. The directory is
// created if missing. Returns the full path written.
func (s *snapshotStore) Write(when time.Time, data any) (string, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir snapshots: %w", err)
	}
	path := filepath.Join(s.dir, "stats-"+snapshotTimestamp(when)+".json")
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	return path, nil
}

// Prune deletes the oldest snapshots until at most `keep` remain. Returns the
// number of files deleted.
func (s *snapshotStore) Prune(keep int) (int, error) {
	files, err := s.list()
	if err != nil {
		return 0, err
	}
	if len(files) <= keep {
		return 0, nil
	}
	// files is oldest-first; delete everything before the last `keep`.
	toDelete := files[:len(files)-keep]
	for _, f := range toDelete {
		if err := os.Remove(f); err != nil {
			return 0, fmt.Errorf("remove %s: %w", f, err)
		}
	}
	return len(toDelete), nil
}

// LoadClosestOlder decodes the newest snapshot whose timestamp is strictly
// before asOf into `into`. Returns (false, nil) when no older snapshot exists.
func (s *snapshotStore) LoadClosestOlder(asOf time.Time, into any) (bool, error) {
	files, err := s.list()
	if err != nil {
		return false, err
	}
	// Walk newest-first; the first file with timestamp < asOf wins.
	for i := len(files) - 1; i >= 0; i-- {
		ts, ok := parseSnapshotTimestamp(filepath.Base(files[i]))
		if !ok {
			continue
		}
		if ts.Before(asOf) {
			buf, err := os.ReadFile(files[i])
			if err != nil {
				return false, fmt.Errorf("read %s: %w", files[i], err)
			}
			if err := json.Unmarshal(buf, into); err != nil {
				return false, fmt.Errorf("decode %s: %w", files[i], err)
			}
			return true, nil
		}
	}
	return false, nil
}

// list returns snapshot file paths sorted oldest-first.
func (s *snapshotStore) list() ([]string, error) {
	pattern := filepath.Join(s.dir, "stats-*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob snapshots: %w", err)
	}
	// Filter out anything whose basename doesn't parse as a timestamp.
	out := matches[:0]
	for _, m := range matches {
		if _, ok := parseSnapshotTimestamp(filepath.Base(m)); ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ti, _ := parseSnapshotTimestamp(filepath.Base(out[i]))
		tj, _ := parseSnapshotTimestamp(filepath.Base(out[j]))
		return ti.Before(tj)
	})
	return out, nil
}

func parseSnapshotTimestamp(name string) (time.Time, bool) {
	const prefix = "stats-"
	const suffix = ".json"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return time.Time{}, false
	}
	body := name[len(prefix) : len(name)-len(suffix)]
	// Colons in RFC3339 were replaced with hyphens at write time; we need to
	// restore them in the time portion only (after the 'T').
	tIdx := strings.Index(body, "T")
	if tIdx < 0 {
		return time.Time{}, false
	}
	// The time portion may end in Z or an offset; count hyphens after T and
	// convert them to colons in positions that correspond to HH:MM:SS.
	timePart := body[tIdx+1:]
	timeRestored := restoreTimeSeparators(timePart)
	rebuilt := body[:tIdx+1] + timeRestored
	ts, err := time.Parse(time.RFC3339, rebuilt)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

// restoreTimeSeparators turns the first two hyphens in HH-MM-SS back into
// colons. Any trailing offset (e.g. "-07-00" or "Z") is preserved by only
// replacing the first two hyphens we encounter.
func restoreTimeSeparators(timePart string) string {
	out := make([]byte, 0, len(timePart))
	replaced := 0
	for i := 0; i < len(timePart); i++ {
		if timePart[i] == '-' && replaced < 2 {
			out = append(out, ':')
			replaced++
		} else {
			out = append(out, timePart[i])
		}
	}
	return string(out)
}
