package store

import (
	"database/sql"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/adamdecaf/hasherdash/internal/models"
)

// Options configures a Store.
type Options struct {
	// HistoryPoints is the in-memory ring size when SQLite is off.
	// With SQLite, samples are retained by Retention instead.
	HistoryPoints int
	// PollSec is exposed in Meta for the UI.
	PollSec int
	// SQLitePath is the metrics DB path. Empty or "off" keeps history in memory only.
	// Use ":memory:" for an ephemeral SQLite DB (tests).
	SQLitePath string
	// Retention is how long SQLite metric samples are kept. Non-positive disables time prune.
	Retention time.Duration
	// Logger receives non-fatal SQLite errors (optional).
	Logger *log.Logger
}

// Store is an in-memory fleet cache with per-miner metric history
// (SQLite-backed when configured, otherwise ring buffers).
type Store struct {
	mu      sync.RWMutex
	miners  map[string]models.Detail
	history map[string]map[string]*ring // id -> metric -> ring (memory mode)
	points  int

	db        *sql.DB
	retention time.Duration
	logger    *log.Logger

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

// New creates a memory-only store (no SQLite). Prefer Open for production.
func New(historyPoints int, pollSec int) *Store {
	st, err := Open(Options{HistoryPoints: historyPoints, PollSec: pollSec})
	if err != nil {
		// Memory-only Open cannot fail.
		panic(err)
	}
	return st
}

// Open creates a store, optionally with SQLite-backed metric history.
func Open(opts Options) (*Store, error) {
	if opts.HistoryPoints < 10 {
		opts.HistoryPoints = 10
	}
	s := &Store{
		miners:    make(map[string]models.Detail),
		history:   make(map[string]map[string]*ring),
		points:    opts.HistoryPoints,
		pollSec:   opts.PollSec,
		retention: opts.Retention,
		logger:    opts.Logger,
	}
	db, err := openMetricsDB(opts.SQLitePath)
	if err != nil {
		return nil, err
	}
	s.db = db
	return s, nil
}

// UsingSQLite reports whether metric history is persisted to SQLite.
func (s *Store) UsingSQLite() bool {
	return s.db != nil
}

// Close releases the SQLite handle (if any). Safe to call multiple times.
func (s *Store) Close() error {
	s.mu.Lock()
	db := s.db
	s.db = nil
	s.mu.Unlock()
	if db == nil {
		return nil
	}
	return db.Close()
}

// SetPolling marks whether a poll cycle is in progress.
func (s *Store) SetPolling(v bool) {
	s.mu.Lock()
	s.polling = v
	s.mu.Unlock()
}

// Upsert replaces a miner snapshot and appends history samples on success.
// Miners are keyed by a stable identity (MAC → serial → hostname → IP) so a
// device that changes address stays one row with continuous history.
// Failed polls merge an error onto any existing snapshot without wiping
// identity / last-good telemetry, and do not advance LastSeen.
func (s *Store) Upsert(d models.Detail) {
	var samples []metricSample
	var rekeys [][2]string // oldID → newID

	s.mu.Lock()
	now := time.Now().UTC()
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	models.ApplyStableID(&d.Snapshot)

	existingID, existing, had := s.findMinerLocked(d)

	if d.Error != "" {
		if had {
			merged := existing
			merged.Error = d.Error
			merged.UpdatedAt = d.UpdatedAt
			// Keep stable id / MAC / hostname from the last good poll.
			if merged.LastSeen.IsZero() {
				// First contact was an error — start the TTL clock now.
				merged.LastSeen = now
			}
			// Refresh IP if the error poll still knows where we tried.
			if d.IP != "" {
				merged.IP = d.IP
			}
			s.miners[existingID] = merged
		} else {
			if d.LastSeen.IsZero() {
				d.LastSeen = now
			}
			s.miners[d.ID] = d
		}
		s.mu.Unlock()
		return
	}

	d.Error = ""
	d.LastSeen = now

	if had && existingID != d.ID {
		// Promote IP/hostname-keyed row to MAC (or hostname) identity.
		s.rekeyLocked(existingID, d.ID)
		rekeys = append(rekeys, [2]string{existingID, d.ID})
	}
	// Drop any other row that still points at this IP under a different id
	// (stale IP-keyed ghost after the device moved and we re-identified it).
	if d.IP != "" {
		for id, m := range s.miners {
			if id == d.ID {
				continue
			}
			if m.IP == d.IP {
				s.rekeyLocked(id, d.ID)
				rekeys = append(rekeys, [2]string{id, d.ID})
			}
		}
	}

	s.miners[d.ID] = d

	samples = collectSamples(d)
	if s.db == nil {
		for _, sm := range samples {
			s.pushLocked(sm.minerID, sm.metric, sm.value, sm.ts)
		}
	}
	s.mu.Unlock()

	if s.db != nil {
		for _, pair := range rekeys {
			if err := s.renameMetrics(pair[0], pair[1]); err != nil {
				s.logf("store: rename metrics %s → %s: %v", pair[0], pair[1], err)
			}
		}
		if len(samples) > 0 {
			if err := s.insertSamples(samples); err != nil {
				s.logf("store: insert metrics for %s: %v", d.ID, err)
			}
		}
	}
}

// findMinerLocked locates an existing fleet entry for this poll.
// Match order: exact stable id → same MAC → same serial → same IP →
// distinctive hostname (only when the poll has no MAC/serial).
func (s *Store) findMinerLocked(d models.Detail) (id string, existing models.Detail, ok bool) {
	if d.ID != "" {
		if m, hit := s.miners[d.ID]; hit {
			return d.ID, m, true
		}
	}
	mac := models.NormalizeMAC(d.MAC)
	if mac != "" {
		for mid, m := range s.miners {
			if models.NormalizeMAC(m.MAC) == mac {
				return mid, m, true
			}
		}
	}
	serial := strings.ToLower(strings.TrimSpace(d.Serial))
	if serial != "" {
		for mid, m := range s.miners {
			if strings.ToLower(strings.TrimSpace(m.Serial)) == serial {
				return mid, m, true
			}
		}
	}
	if d.IP != "" {
		for mid, m := range s.miners {
			if m.IP == d.IP {
				return mid, m, true
			}
		}
	}
	// Hostname is a weaker signal; only when we lack stronger hardware ids.
	if mac == "" && serial == "" {
		if host := models.DistinctiveHostname(d.Hostname); host != "" {
			for mid, m := range s.miners {
				if models.NormalizeMAC(m.MAC) != "" || strings.TrimSpace(m.Serial) != "" {
					continue
				}
				if models.DistinctiveHostname(m.Hostname) == host {
					return mid, m, true
				}
			}
		}
	}
	return "", models.Detail{}, false
}

// rekeyLocked moves a miner and its in-memory history from oldID to newID.
// Caller holds s.mu. SQLite rows are renamed separately after unlock.
func (s *Store) rekeyLocked(oldID, newID string) {
	if oldID == "" || newID == "" || oldID == newID {
		return
	}
	if old, ok := s.miners[oldID]; ok {
		delete(s.miners, oldID)
		if _, exists := s.miners[newID]; !exists {
			old.ID = newID
			s.miners[newID] = old
		}
	}
	if oldHist, ok := s.history[oldID]; ok {
		delete(s.history, oldID)
		if s.history[newID] == nil {
			s.history[newID] = oldHist
			return
		}
		// Merge rings into the destination (best-effort; rare overlap).
		for metric, src := range oldHist {
			dst := s.history[newID][metric]
			if dst == nil {
				s.history[newID][metric] = src
				continue
			}
			for _, p := range src.slice() {
				dst.push(p)
			}
		}
	}
}

func collectSamples(d models.Detail) []metricSample {
	t := d.UpdatedAt
	if t.IsZero() {
		t = time.Now().UTC()
	}
	out := []metricSample{
		{d.ID, "hashrate", t, d.HashrateTH},
	}
	// "temp" charts the hottest ASIC reading when available, else average.
	temp := d.AvgTempC
	if d.HasASICTemp {
		temp = d.ASICTempMax
	}
	out = append(out, metricSample{d.ID, "temp", t, temp})
	if d.HasASICTemp {
		out = append(out,
			metricSample{d.ID, "asic_temp", t, d.ASICTempMax},
			metricSample{d.ID, "asic_temp_min", t, d.ASICTempMin},
		)
	}
	if d.HasVRTemp {
		out = append(out,
			metricSample{d.ID, "vr_temp", t, d.VRTempMax},
			metricSample{d.ID, "vr_temp_min", t, d.VRTempMin},
		)
	}
	out = append(out,
		metricSample{d.ID, "wattage", t, d.Wattage},
		metricSample{d.ID, "efficiency", t, d.Efficiency},
		metricSample{d.ID, "chips", t, float64(d.TotalChips)},
	)
	return out
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

// MarkPoll records poll cycle completion and prunes expired metric samples.
func (s *Store) MarkPoll(err error) {
	s.mu.Lock()
	s.lastPollAt = time.Now().UTC()
	s.polling = false
	if err != nil {
		s.lastPollErr = err.Error()
	} else {
		s.lastPollErr = ""
	}
	retention := s.retention
	s.mu.Unlock()

	if s.db != nil && retention > 0 {
		cutoff := time.Now().UTC().Add(-retention)
		if _, err := s.pruneMetricsBefore(cutoff); err != nil {
			s.logf("store: prune metrics: %v", err)
		}
	}
}

// Prune removes miners whose LastSeen is older than ttl from the live fleet.
// Metric samples are kept so charts still show a departed miner until their
// points age out of history_retention / the selected window. Returns the
// pruned miners' IPs (for discovery forget). A non-positive ttl disables pruning.
func (s *Store) Prune(ttl time.Duration) []string {
	if ttl <= 0 {
		return nil
	}
	s.mu.Lock()
	cutoff := time.Now().UTC().Add(-ttl)
	var removedIPs []string
	for id, d := range s.miners {
		seen := d.LastSeen
		if seen.IsZero() {
			seen = d.UpdatedAt
		}
		if seen.IsZero() || !seen.Before(cutoff) {
			continue
		}
		delete(s.miners, id)
		// Keep s.history rings so memory-mode charts retain the series until
		// process restart; SQLite samples are owned by retention prune only.
		if ip := strings.TrimSpace(d.IP); ip != "" {
			removedIPs = append(removedIPs, ip)
		}
	}
	sort.Strings(removedIPs)
	s.mu.Unlock()
	return removedIPs
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

// HistoryOptions filters returned series points.
type HistoryOptions struct {
	Since time.Time // inclusive; zero = no lower bound
	Until time.Time // inclusive; zero = no upper bound
}

// History returns series for the given metric and miner IDs (empty IDs = all).
func (s *Store) History(metric string, ids []string, opts HistoryOptions) []models.Series {
	if s.db != nil {
		out, err := s.historyFromDB(metric, ids, opts)
		if err != nil {
			s.logf("store: history query: %v", err)
			return nil
		}
		return out
	}
	return s.historyFromMemory(metric, ids, opts)
}

func (s *Store) historyFromMemory(metric string, ids []string, opts HistoryOptions) []models.Series {
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
		points := filterPoints(r.slice(), opts.Since, opts.Until)
		ser := models.Series{
			ID:     id,
			Label:  id,
			Metric: metric,
			Points: points,
		}
		if d, ok := s.miners[id]; ok {
			ser.Make = d.Make
			ser.Model = d.Model
			ser.Firmware = d.Firmware
			ser.Algo = d.Algo
			ser.Label = seriesLabel(d, id)
		}
		out = append(out, ser)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func filterPoints(pts []models.HistoryPoint, since, until time.Time) []models.HistoryPoint {
	if since.IsZero() && until.IsZero() {
		return pts
	}
	out := make([]models.HistoryPoint, 0, len(pts))
	for _, p := range pts {
		t := p.T
		if !since.IsZero() && t.Before(since) {
			continue
		}
		if !until.IsZero() && t.After(until) {
			continue
		}
		out = append(out, p)
	}
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

func (s *Store) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// SQLitePathLabel is a short description for logs.
func SQLitePathLabel(path string) string {
	if path == "" || path == SQLitePathOff {
		return "off (memory)"
	}
	return path
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// seriesLabel prefers human-friendly identity over the raw stable id.
func seriesLabel(d models.Detail, id string) string {
	if d.Hostname != "" {
		return d.Hostname
	}
	if d.MAC != "" && d.IP != "" {
		if d.Model != "" {
			return d.Model + " " + d.IP
		}
		return d.IP
	}
	if d.Model != "" {
		return d.Model + " " + id
	}
	return id
}
