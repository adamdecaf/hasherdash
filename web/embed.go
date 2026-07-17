package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFS embed.FS

// Handler serves the embedded SPA assets.
func Handler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SPA-style: unknown paths fall back to index.html for deep links.
		if r.URL.Path != "/" {
			// Let FileServer try; if missing, serve index.
			open, err := sub.Open(r.URL.Path[1:])
			if err != nil {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				fileServer.ServeHTTP(w, r2)
				return
			}
			_ = open.Close()
		}
		fileServer.ServeHTTP(w, r)
	})
}
