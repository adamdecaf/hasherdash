package store

import (
	"sort"
	"sync"
	"time"

	"github.com/adamdecaf/minerdash/internal/models"
)

// Store is an in-memory fleet cache with per-miner metric history.
type Store struct {
	mu      sync.RWMutex
	miners  map[string]models.Detail
	history map[string]map[string]*ring // id -> metric -> ring
	points  int

	lastPollAt  time.Time
	lastPollErr string
	polling     bool
	pollSec     int
}

type ring struct {
	buf  []models.HistoryPoint
	head int
	full bool
}

func newRing(n int) *ring {
	return &ring{buf: make([]models.HistoryPoint, n)}
}

func (r *ring) push(p models.HistoryPoint) {
	r.buf[r.head] = p
	r.head = (r.head + 1) % len(r.buf)
	if r.head == 0 {
		r.full = true
	}
}

func (r *ring) slice() []models.HistoryPoint {
	if !r.full {
		out := make([]models.HistoryPoint, r.head)
		copy(out, r.buf[:r.head])
		return out
	}
	out := make([]models.HistoryPoint, len(r.buf))
	copy(out, r.buf[r.head:])
	copy(out[len(r.buf)-r.head:], r.buf[:r.head])
	return out
}

// New creates a store that keeps historyPoints samples per metric per miner.
func New(historyPoints int, pollSec int) *Store {
	if historyPoints < 10 {
		historyPoints = 10
	}
	return &Store{
		miners:  make(map[string]models.Detail),
		history: make(map[string]map[string]*ring),
		points:  historyPoints,
		pollSec: pollSec,
	}
}

// SetPolling marks whether a poll cycle is in progress.
func (s *Store) SetPolling(v bool) {
	s.mu.Lock()
	s.polling = v
	s.mu.Unlock()
}

// Upsert replaces a miner snapshot and appends history samples.
func (s *Store) Upsert(d models.Detail) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.LastSeen.IsZero() && d.Error == "" {
		d.LastSeen = now
	}
	s.miners[d.ID] = d

	if d.Error != "" {
		return
	}
	s.pushLocked(d.ID, "hashrate", d.HashrateTH, d.UpdatedAt)
	// "temp" charts the hottest ASIC reading when available, else average.
	temp := d.AvgTempC
	if d.HasASICTemp {
		temp = d.ASICTempMax
	}
	s.pushLocked(d.ID, "temp", temp, d.UpdatedAt)
	if d.HasASICTemp {
		s.pushLocked(d.ID, "asic_temp", d.ASICTempMax, d.UpdatedAt)
		s.pushLocked(d.ID, "asic_temp_min", d.ASICTempMin, d.UpdatedAt)
	}
	if d.HasVRTemp {
		s.pushLocked(d.ID, "vr_temp", d.VRTempMax, d.UpdatedAt)
		s.pushLocked(d.ID, "vr_temp_min", d.VRTempMin, d.UpdatedAt)
	}
	s.pushLocked(d.ID, "wattage", d.Wattage, d.UpdatedAt)
	s.pushLocked(d.ID, "efficiency", d.Efficiency, d.UpdatedAt)
	s.pushLocked(d.ID, "chips", float64(d.TotalChips), d.UpdatedAt)
}

func (s *Store) pushLocked(id, metric string, v float64, t time.Time) {
	if s.history[id] == nil {
		s.history[id] = make(map[string]*ring)
	}
	r := s.history[id][metric]
	if r == nil {
		r = newRing(s.points)
		s.history[id][metric] = r
	}
	r.push(models.HistoryPoint{T: t, V: v})
}

// MarkPoll records poll cycle completion.
func (s *Store) MarkPoll(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPollAt = time.Now().UTC()
	s.polling = false
	if err != nil {
		s.lastPollErr = err.Error()
	} else {
		s.lastPollErr = ""
	}
}

// List returns all miner snapshots sorted by IP.
func (s *Store) List() []models.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Snapshot, 0, len(s.miners))
	for _, d := range s.miners {
		out = append(out, d.Snapshot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

// Get returns detail for one miner.
func (s *Store) Get(id string) (models.Detail, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.miners[id]
	return d, ok
}

// History returns series for the given metric and miner IDs (empty IDs = all).
func (s *Store) History(metric string, ids []string) []models.Series {
	s.mu.RLock()
	defer s.mu.RUnlock()

	want := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			want[id] = true
		}
	}
	useAll := len(want) == 0

	var out []models.Series
	for id, metrics := range s.history {
		if !useAll && !want[id] {
			continue
		}
		r := metrics[metric]
		if r == nil {
			continue
		}
		ser := models.Series{
			ID:     id,
			Label:  id,
			Metric: metric,
			Points: r.slice(),
		}
		if d, ok := s.miners[id]; ok {
			ser.Make = d.Make
			ser.Model = d.Model
			ser.Firmware = d.Firmware
			ser.Algo = d.Algo
			if d.Hostname != "" {
				ser.Label = d.Hostname
			} else if d.Model != "" {
				ser.Label = d.Model + " " + id
			}
		}
		out = append(out, ser)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Meta returns fleet status and distinct filter values.
func (s *Store) Meta() models.Meta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	makes := map[string]struct{}{}
	modelsSet := map[string]struct{}{}
	firmwares := map[string]struct{}{}
	algos := map[string]struct{}{}
	for _, d := range s.miners {
		if d.Make != "" {
			makes[d.Make] = struct{}{}
		}
		if d.Model != "" {
			modelsSet[d.Model] = struct{}{}
		}
		if d.Firmware != "" {
			firmwares[d.Firmware] = struct{}{}
		}
		if d.Algo != "" {
			algos[d.Algo] = struct{}{}
		}
	}
	return models.Meta{
		PollIntervalSec: s.pollSec,
		HistoryPoints:   s.points,
		LastPollAt:      s.lastPollAt,
		LastPollErr:     s.lastPollErr,
		MinerCount:      len(s.miners),
		Polling:         s.polling,
		Makes:           sortedKeys(makes),
		Models:          sortedKeys(modelsSet),
		Firmwares:       sortedKeys(firmwares),
		Algos:           sortedKeys(algos),
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
