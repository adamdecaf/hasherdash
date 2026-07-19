package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adamdecaf/hasherdash/internal/models"
	_ "modernc.org/sqlite"
)

// SQLitePathOff disables on-disk metrics and keeps samples only in memory rings.
const SQLitePathOff = "off"

type metricSample struct {
	minerID string
	metric  string
	ts      time.Time
	value   float64
}

func openMetricsDB(path string) (*sql.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.EqualFold(path, SQLitePathOff) {
		return nil, nil
	}

	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create sqlite dir %s: %w", dir, err)
			}
		}
	}

	// modernc.org/sqlite driver name is "sqlite".
	dsn := path
	if path == ":memory:" {
		// Shared in-memory DB so multiple connections see the same data.
		dsn = "file:hasherdash_mem?mode=memory&cache=shared"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// History queries + poll writes; keep a small pool.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite wal: %w", err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite synchronous: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite busy_timeout: %w", err)
	}

	if err := migrateMetrics(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrateMetrics(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS metric_samples (
  miner_id TEXT    NOT NULL,
  metric   TEXT    NOT NULL,
  ts       INTEGER NOT NULL,
  value    REAL    NOT NULL,
  PRIMARY KEY (miner_id, metric, ts)
);
CREATE INDEX IF NOT EXISTS idx_metric_samples_metric_ts
  ON metric_samples (metric, ts);
CREATE INDEX IF NOT EXISTS idx_metric_samples_metric_miner_ts
  ON metric_samples (metric, miner_id, ts);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("sqlite migrate: %w", err)
	}
	return nil
}

func (s *Store) insertSamples(samples []metricSample) error {
	if s.db == nil || len(samples) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
INSERT INTO metric_samples (miner_id, metric, ts, value)
VALUES (?, ?, ?, ?)
ON CONFLICT(miner_id, metric, ts) DO UPDATE SET value = excluded.value
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sm := range samples {
		ts := sm.ts.UTC()
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		if _, err := stmt.Exec(sm.minerID, sm.metric, ts.UnixMilli(), sm.value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) historyFromDB(metric string, ids []string, opts HistoryOptions) ([]models.Series, error) {
	if s.db == nil {
		return nil, nil
	}

	want := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			want[id] = true
		}
	}
	useAll := len(want) == 0

	args := []any{metric}
	var b strings.Builder
	b.WriteString(`SELECT miner_id, ts, value FROM metric_samples WHERE metric = ?`)
	if !opts.Since.IsZero() {
		b.WriteString(` AND ts >= ?`)
		args = append(args, opts.Since.UTC().UnixMilli())
	}
	if !opts.Until.IsZero() {
		b.WriteString(` AND ts <= ?`)
		args = append(args, opts.Until.UTC().UnixMilli())
	}
	if !useAll {
		b.WriteString(` AND miner_id IN (`)
		first := true
		for id := range want {
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteByte('?')
			args = append(args, id)
		}
		b.WriteByte(')')
	}
	b.WriteString(` ORDER BY miner_id ASC, ts ASC`)

	rows, err := s.db.Query(b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := map[string]*models.Series{}
	var order []string
	for rows.Next() {
		var id string
		var tsMs int64
		var v float64
		if err := rows.Scan(&id, &tsMs, &v); err != nil {
			return nil, err
		}
		ser, ok := byID[id]
		if !ok {
			ser = &models.Series{
				ID:     id,
				Label:  id,
				Metric: metric,
			}
			byID[id] = ser
			order = append(order, id)
		}
		ser.Points = append(ser.Points, models.HistoryPoint{
			T: time.UnixMilli(tsMs).UTC(),
			V: v,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach identity labels from the live miner cache.
	s.mu.RLock()
	for _, id := range order {
		ser := byID[id]
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
	}
	s.mu.RUnlock()

	out := make([]models.Series, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) deleteMetricsForMiners(ids []string) error {
	if s.db == nil || len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(`DELETE FROM metric_samples WHERE miner_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) pruneMetricsBefore(cutoff time.Time) (int64, error) {
	if s.db == nil || cutoff.IsZero() {
		return 0, nil
	}
	res, err := s.db.Exec(`DELETE FROM metric_samples WHERE ts < ?`, cutoff.UTC().UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
