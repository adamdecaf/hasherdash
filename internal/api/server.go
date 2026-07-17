package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/adamdecaf/minerdash/internal/store"
	"github.com/adamdecaf/minerdash/web"
)

// Server is the HTTP API + static UI.
type Server struct {
	store *store.Store
	mux   *http.ServeMux
}

// New creates an HTTP handler.
func New(st *store.Store) *Server {
	s := &Server{store: st, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/meta", s.handleMeta)
	s.mux.HandleFunc("GET /api/miners", s.handleMiners)
	s.mux.HandleFunc("GET /api/miners/{id}", s.handleMiner)
	s.mux.HandleFunc("GET /api/history", s.handleHistory)
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
	writeJSON(w, http.StatusOK, s.store.History(metric, ids))
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
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
