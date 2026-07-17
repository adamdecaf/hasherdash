package store

import (
	"testing"
	"time"

	"github.com/adamdecaf/minerdash/internal/models"
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

	hist := st.History("hashrate", nil)
	if len(hist) != 1 || len(hist[0].Points) != 2 {
		t.Fatalf("history %#v", hist)
	}

	meta := st.Meta()
	if meta.MinerCount != 1 || len(meta.Makes) != 1 {
		t.Fatalf("meta %#v", meta)
	}
}
