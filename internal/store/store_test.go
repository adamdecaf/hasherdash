package store

import (
	"testing"
	"time"

	"github.com/adamdecaf/hasherdash/internal/models"
)

func TestUpsertAndHistory(t *testing.T) {
	st := New(10, 30)
	now := time.Now().UTC()
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		ID: "10.0.0.1", IP: "10.0.0.1", Make: "Bitmain", Model: "S19",
		HashrateTH: 100, AvgTempC: 60, UpdatedAt: now,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		ID: "10.0.0.1", IP: "10.0.0.1", Make: "Bitmain", Model: "S19",
		HashrateTH: 101, AvgTempC: 61, UpdatedAt: now.Add(time.Second),
	}})

	list := st.List()
	if len(list) != 1 {
		t.Fatalf("list len %d", len(list))
	}
	if list[0].HashrateTH != 101 {
		t.Fatalf("hashrate %v", list[0].HashrateTH)
	}

	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 || len(hist[0].Points) != 2 {
		t.Fatalf("history %#v", hist)
	}

	meta := st.Meta()
	if meta.MinerCount != 1 || len(meta.Makes) != 1 {
		t.Fatalf("meta %#v", meta)
	}
}

func TestUpsertErrorPreservesSnapshot(t *testing.T) {
	st := New(10, 30)
	now := time.Now().UTC()
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		ID: "10.0.0.2", IP: "10.0.0.2", Make: "Bitaxe", Model: "Ultra",
		HashrateTH: 1.5, UpdatedAt: now, LastSeen: now,
	}})
	before, ok := st.Get("10.0.0.2")
	if !ok {
		t.Fatal("missing after success")
	}
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		ID: "10.0.0.2", IP: "10.0.0.2", Error: "timeout", UpdatedAt: now.Add(time.Minute),
	}})

	d, ok := st.Get("10.0.0.2")
	if !ok {
		t.Fatal("missing miner")
	}
	if d.Make != "Bitaxe" || d.HashrateTH != 1.5 {
		t.Fatalf("wiped good data: %#v", d)
	}
	if d.Error != "timeout" {
		t.Fatalf("error %q", d.Error)
	}
	if !d.LastSeen.Equal(before.LastSeen) {
		t.Fatalf("LastSeen advanced on error: before=%v after=%v", before.LastSeen, d.LastSeen)
	}
	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 || len(hist[0].Points) != 1 {
		t.Fatalf("error poll should not append history: %#v", hist)
	}
}

func TestPruneByTTL(t *testing.T) {
	st := New(10, 30)
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	fresh := time.Now().UTC()
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		ID: "old", IP: "10.0.0.1", HashrateTH: 1, UpdatedAt: old, LastSeen: old,
	}})
	// Force LastSeen old (Upsert sets LastSeen=now on success).
	st.mu.Lock()
	d := st.miners["old"]
	d.LastSeen = old
	d.UpdatedAt = old
	st.miners["old"] = d
	st.mu.Unlock()

	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		ID: "fresh", IP: "10.0.0.2", HashrateTH: 2, UpdatedAt: fresh,
	}})

	removed := st.Prune(7 * 24 * time.Hour)
	if len(removed) != 1 || removed[0] != "old" {
		t.Fatalf("removed %#v", removed)
	}
	if _, ok := st.Get("old"); ok {
		t.Fatal("old should be gone")
	}
	if _, ok := st.Get("fresh"); !ok {
		t.Fatal("fresh should remain")
	}
	if st.Prune(0) != nil {
		t.Fatal("ttl<=0 should not prune")
	}
}

func TestHistoryTimeFilter(t *testing.T) {
	st := New(20, 30)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		st.Upsert(models.Detail{Snapshot: models.Snapshot{
			ID: "m1", IP: "10.0.0.1", HashrateTH: float64(i),
			UpdatedAt: base.Add(time.Duration(i) * time.Hour),
		}})
	}
	// Override timestamps on history ring by re-pushing with known times via direct access would be hard;
	// instead push through Upsert which uses UpdatedAt — already done above.

	since := base.Add(2 * time.Hour)
	until := base.Add(3 * time.Hour)
	hist := st.History("hashrate", nil, HistoryOptions{Since: since, Until: until})
	if len(hist) != 1 {
		t.Fatalf("series %#v", hist)
	}
	// Points at t=2h and t=3h inclusive.
	if len(hist[0].Points) != 2 {
		t.Fatalf("want 2 points, got %d: %#v", len(hist[0].Points), hist[0].Points)
	}
}
