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
		IP: "10.0.0.1", HashrateTH: 1, UpdatedAt: old, LastSeen: old,
	}})
	// Force LastSeen old (Upsert sets LastSeen=now on success).
	st.mu.Lock()
	d := st.miners["10.0.0.1"]
	d.LastSeen = old
	d.UpdatedAt = old
	st.miners["10.0.0.1"] = d
	st.mu.Unlock()

	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.2", HashrateTH: 2, UpdatedAt: fresh,
	}})

	// Prune returns IPs for discovery forget.
	removed := st.Prune(7 * 24 * time.Hour)
	if len(removed) != 1 || removed[0] != "10.0.0.1" {
		t.Fatalf("removed %#v", removed)
	}
	if _, ok := st.Get("10.0.0.1"); ok {
		t.Fatal("old should be gone from live fleet")
	}
	if _, ok := st.Get("10.0.0.2"); !ok {
		t.Fatal("fresh should remain")
	}
	// History for the pruned miner stays until retention ages it out.
	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 2 {
		t.Fatalf("want history for live + pruned, got %#v", hist)
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
			IP: "10.0.0.1", HashrateTH: float64(i),
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

func TestUpsertGroupsByMACAcrossIPChange(t *testing.T) {
	st := New(20, 30)
	now := time.Now().UTC()
	mac := "AA:BB:CC:DD:EE:01"

	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.10", MAC: mac, Hostname: "rig-a",
		HashrateTH: 10, UpdatedAt: now,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.99", MAC: mac, Hostname: "rig-a",
		HashrateTH: 11, UpdatedAt: now.Add(time.Minute),
	}})

	list := st.List()
	if len(list) != 1 {
		t.Fatalf("want 1 miner after IP change, got %d: %#v", len(list), list)
	}
	wantID := "aa:bb:cc:dd:ee:01"
	if list[0].ID != wantID {
		t.Fatalf("id %q want %q", list[0].ID, wantID)
	}
	if list[0].IP != "10.0.0.99" {
		t.Fatalf("ip %q", list[0].IP)
	}
	if list[0].HashrateTH != 11 {
		t.Fatalf("hashrate %v", list[0].HashrateTH)
	}

	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 || hist[0].ID != wantID {
		t.Fatalf("history series %#v", hist)
	}
	if len(hist[0].Points) != 2 {
		t.Fatalf("want continuous history, got %d points", len(hist[0].Points))
	}
	if hist[0].Label != "rig-a" {
		t.Fatalf("label %q", hist[0].Label)
	}
}

func TestUpsertGroupsByHostnameWhenNoMAC(t *testing.T) {
	st := New(10, 30)
	now := time.Now().UTC()
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", Hostname: "rack1-gamma", HashrateTH: 1, UpdatedAt: now,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.2", Hostname: "rack1-gamma", HashrateTH: 2, UpdatedAt: now.Add(time.Second),
	}})
	list := st.List()
	if len(list) != 1 {
		t.Fatalf("want 1 miner grouped by hostname, got %d", len(list))
	}
	if list[0].ID != "host:rack1-gamma" {
		t.Fatalf("id %q", list[0].ID)
	}
	if list[0].IP != "10.0.0.2" {
		t.Fatalf("ip %q", list[0].IP)
	}
}

func TestUpsertDoesNotCollapseGenericHostnames(t *testing.T) {
	st := New(10, 30)
	now := time.Now().UTC()
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.1", Hostname: "bitaxe", HashrateTH: 1, UpdatedAt: now,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.2", Hostname: "bitaxe", HashrateTH: 2, UpdatedAt: now.Add(time.Second),
	}})
	if n := len(st.List()); n != 2 {
		t.Fatalf("generic hostname must not merge fleet, got %d", n)
	}
}

func TestUpsertErrorMatchesByIP(t *testing.T) {
	st := New(10, 30)
	now := time.Now().UTC()
	mac := "aa:bb:cc:dd:ee:02"
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.5", MAC: mac, Hostname: "rig-b",
		HashrateTH: 3, UpdatedAt: now,
	}})
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.5", Error: "timeout", UpdatedAt: now.Add(time.Minute),
	}})

	d, ok := st.Get(mac)
	if !ok {
		t.Fatal("missing miner under mac id")
	}
	if d.Error != "timeout" {
		t.Fatalf("error %q", d.Error)
	}
	if d.HashrateTH != 3 || d.Hostname != "rig-b" {
		t.Fatalf("wiped good data: %#v", d)
	}
	if len(st.List()) != 1 {
		t.Fatalf("ghost error row created: %#v", st.List())
	}
}

func TestUpsertPromotesIPIdentityToMAC(t *testing.T) {
	st := New(10, 30)
	now := time.Now().UTC()
	// First poll failed / no MAC yet — keyed by IP.
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.7", HashrateTH: 1, UpdatedAt: now,
	}})
	if _, ok := st.Get("10.0.0.7"); !ok {
		t.Fatal("expected ip-keyed miner")
	}
	st.Upsert(models.Detail{Snapshot: models.Snapshot{
		IP: "10.0.0.7", MAC: "11:22:33:44:55:66", Hostname: "gamma",
		HashrateTH: 2, UpdatedAt: now.Add(time.Second),
	}})

	if _, ok := st.Get("10.0.0.7"); ok {
		t.Fatal("ip key should be gone after mac promotion")
	}
	d, ok := st.Get("11:22:33:44:55:66")
	if !ok {
		t.Fatal("missing mac-keyed miner")
	}
	if d.HashrateTH != 2 {
		t.Fatalf("hashrate %v", d.HashrateTH)
	}
	hist := st.History("hashrate", nil, HistoryOptions{})
	if len(hist) != 1 || hist[0].ID != "11:22:33:44:55:66" || len(hist[0].Points) != 2 {
		t.Fatalf("history after promote %#v", hist)
	}
}
