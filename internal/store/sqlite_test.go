package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/adamdecaf/hasherdash/internal/models"
)

func TestSQLiteHistoryPersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.db")

	st, err := Open(Options{
		HistoryPoints: 10,
		PollSec:       30,
		SQLitePath:    path,
		Retention:     48 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !st.UsingSQLite() {
		t.Fatal("expected sqlite")
	}

	base := time.Now().UTC().Truncate(time.Second).Add(-2 * time.Minute)
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", Hostname: "rig-a", Make: "Bitaxe", Model: "Gamma",
		HashrateTH: 1.25, UpdatedAt: base,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", Hostname: "rig-a", Make: "Bitaxe", Model: "Gamma",
		HashrateTH: 1.30, UpdatedAt: base.Add(time.Minute),
	}})
	st.MarkPoll(nil)

	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 || len(hist[0].Points) != 2 {
		t.Fatalf("history before close: %#v", hist)
	}
	if hist[0].ID != "host:rig-a" {
		t.Fatalf("stable id %q", hist[0].ID)
	}
	if hist[0].Label != "rig-a" {
		t.Fatalf("label %q", hist[0].Label)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// Re-open same file: points must still be there (miners map empty until polled).
	st2, err := Open(Options{
		HistoryPoints: 10,
		PollSec:       30,
		SQLitePath:    path,
		Retention:     48 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	hist2 := st2.History("hashrate", nil, HistoryOptions{})
	if len(hist2) != 1 || len(hist2[0].Points) != 2 {
		t.Fatalf("history after reopen: %#v", hist2)
	}
	if hist2[0].Points[0].V != 1.25 || hist2[0].Points[1].V != 1.30 {
		t.Fatalf("values %#v", hist2[0].Points)
	}
	// No live miner snapshot — label falls back to stable id.
	if hist2[0].Label != "host:rig-a" {
		t.Fatalf("label without miner cache: %q", hist2[0].Label)
	}
}

func TestSQLiteHistoryTimeFilterAndPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.db")
	st, err := Open(Options{
		HistoryPoints: 10,
		PollSec:       30,
		SQLitePath:    path,
		Retention:     time.Hour, // prune older than 1h on MarkPoll
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC().Truncate(time.Second)
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", HashrateTH: 1, UpdatedAt: now.Add(-2 * time.Hour),
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", HashrateTH: 2, UpdatedAt: now.Add(-30 * time.Minute),
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", HashrateTH: 3, UpdatedAt: now,
	}})

	// Filter window: last 45 minutes should include 2 and 3.
	hist := st.History("hashrate", []string{"10.0.0.1"}, HistoryOptions{
		Since: now.Add(-45 * time.Minute),
		Until: now,
	})
	if len(hist) != 1 || len(hist[0].Points) != 2 {
		t.Fatalf("filtered %#v", hist)
	}

	st.MarkPoll(nil) // drops the 2h-old sample
	histAll := st.History("hashrate", nil, HistoryOptions{})
	if len(histAll) != 1 || len(histAll[0].Points) != 2 {
		t.Fatalf("after retention prune %#v", histAll)
	}
}

func TestSQLiteKeepsMetricsOnMinerPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.db")
	st, err := Open(Options{
		HistoryPoints: 10,
		PollSec:       30,
		SQLitePath:    path,
		Retention:     7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Sample timestamps must be inside retention; only LastSeen is ancient.
	sampleAt := time.Now().UTC().Add(-time.Hour)
	oldSeen := time.Now().UTC().Add(-8 * 24 * time.Hour)
	fresh := time.Now().UTC()
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", HashrateTH: 1, UpdatedAt: sampleAt, LastSeen: sampleAt,
	}})
	st.mu.Lock()
	d := st.miners["10.0.0.1"]
	d.LastSeen = oldSeen
	d.UpdatedAt = sampleAt
	st.miners["10.0.0.1"] = d
	st.mu.Unlock()

	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.2", HashrateTH: 2, UpdatedAt: fresh,
	}})

	removed := st.Prune(7 * 24 * time.Hour)
	if len(removed) != 1 || removed[0] != "10.0.0.1" {
		t.Fatalf("removed %#v", removed)
	}
	if _, ok := st.Get("10.0.0.1"); ok {
		t.Fatal("pruned miner should leave live fleet")
	}

	// SQLite samples survive fleet prune so charts keep the series in-window.
	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 2 {
		t.Fatalf("history after miner prune %#v", hist)
	}
	ids := map[string]bool{}
	for _, s := range hist {
		ids[s.ID] = true
	}
	if !ids["10.0.0.1"] || !ids["10.0.0.2"] {
		t.Fatalf("missing series ids %#v", ids)
	}
}

func TestSQLitePathOffUsesMemory(t *testing.T) {
	st, err := Open(Options{
		HistoryPoints: 10,
		PollSec:       30,
		SQLitePath:    SQLitePathOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.UsingSQLite() {
		t.Fatal("expected memory mode")
	}
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", HashrateTH: 9, UpdatedAt: time.Now().UTC(),
	}})
	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 || len(hist[0].Points) != 1 {
		t.Fatalf("%#v", hist)
	}
}

func TestSQLiteRenameMetricsOnMACPromote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.db")
	st, err := Open(Options{
		HistoryPoints: 10,
		PollSec:       30,
		SQLitePath:    path,
		Retention:     48 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UTC().Truncate(time.Second)
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.3", HashrateTH: 1, UpdatedAt: now,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.3", MAC: "DE:AD:BE:EF:00:01", HashrateTH: 2, UpdatedAt: now.Add(time.Minute),
	}})

	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 {
		t.Fatalf("series count %#v", hist)
	}
	if hist[0].ID != "de:ad:be:ef:00:01" {
		t.Fatalf("id %q", hist[0].ID)
	}
	if len(hist[0].Points) != 2 {
		t.Fatalf("points not migrated: %#v", hist[0].Points)
	}
}
