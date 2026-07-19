package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/adamdecaf/hasherdash/internal/store"
	"github.com/adamdecaf/hasherdash/web"
)

// RescanRequester kicks a full discovery scan + poll cycle.
type RescanRequester interface {
	RequestRescan()
}

// Server is the HTTP API + static UI.
type Server struct {
	store  *store.Store
	rescan RescanRequester
	mux    *http.ServeMux
}

// New creates an HTTP handler. rescan may be nil (POST /api/rescan returns 503).
func New(st *store.Store, rescan RescanRequester) *Server {
	s := &Server{store: st, rescan: rescan, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/meta", s.handleMeta)
	s.mux.HandleFunc("GET /api/miners", s.handleMiners)
	s.mux.HandleFunc("GET /api/miners/{id}", s.handleMiner)
	s.mux.HandleFunc("GET /api/history", s.handleHistory)
	s.mux.HandleFunc("POST /api/rescan", s.handleRescan)
	s.mux.Handle("GET /", web.Handler())
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler {
	return withCORS(withLog(s.mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().UTC(),
	})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Meta())
}

func (s *Server) handleMiners(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.List())
}

func (s *Server) handleMiner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, ok := s.store.Get(id)
	if !ok {
		http.Error(w, "miner not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	if s.rescan == nil {
		http.Error(w, "rescan not available", http.StatusServiceUnavailable)
		return
	}
	s.rescan.RequestRescan()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": "rescan started",
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "hashrate"
	}
	switch metric {
	case "hashrate", "temp", "asic_temp", "asic_temp_min", "vr_temp", "vr_temp_min", "wattage", "efficiency", "chips":
	default:
		http.Error(w, "invalid metric", http.StatusBadRequest)
		return
	}
	var ids []string
	if raw := r.URL.Query().Get("ids"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				ids = append(ids, p)
			}
		}
	}
	opts := store.HistoryOptions{}
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid since (use RFC3339)", http.StatusBadRequest)
			return
		}
		opts.Since = t.UTC()
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("until")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			http.Error(w, "invalid until (use RFC3339)", http.StatusBadRequest)
			return
		}
		opts.Until = t.UTC()
	}
	// window=4h|12h|1d|… — relative to now when since is unset.
	if opts.Since.IsZero() {
		if raw := strings.TrimSpace(r.URL.Query().Get("window")); raw != "" {
			d, err := parseWindow(raw)
			if err != nil {
				http.Error(w, "invalid window", http.StatusBadRequest)
				return
			}
			if d > 0 {
				opts.Since = time.Now().UTC().Add(-d)
			}
		}
	}
	writeJSON(w, http.StatusOK, s.store.History(metric, ids, opts))
}

// parseWindow accepts Go durations (4h, 30m) and day shorthands (1d, 3d, 7d).
func parseWindow(raw string) (time.Duration, error) {
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || days <= 0 {
			return 0, strconv.ErrSyntax
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return 0, strconv.ErrSyntax
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			start := time.Now()
			next.ServeHTTP(w, r)
			// Keep logs light — only API.
			_ = start
			return
		}
		next.ServeHTTP(w, r)
	})
}
