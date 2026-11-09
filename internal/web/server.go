// Package web serves a fully local, embedded dashboard. No frontend dependency or CDN is used.
package web

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/StarkTech-coder/sima/internal/model"
)

//go:embed assets/*
var assets embed.FS

// Simple Browser embeds the dashboard in a VS Code webview. Denying all
// ancestors produces a blank preview even when every asset is served correctly.
// Allow only this origin and VS Code's desktop/webview origins, not arbitrary sites.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'self' vscode-webview: vscode-file://vscode-app https://*.vscode-cdn.net"

type Application interface {
	View() model.View
	SetConnected(bool) error
}

func Handler(app Application) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(app.View())
	})
	mux.HandleFunc("POST /api/link", func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "origin rejected", http.StatusForbidden)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, "cross-site control rejected", http.StatusForbidden)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "JSON required", http.StatusUnsupportedMediaType)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var input struct {
			Connected *bool `json:"connected"`
		}
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(&input); err != nil || input.Connected == nil {
			http.Error(w, "invalid control", http.StatusBadRequest)
			return
		}
		if d.Decode(new(any)) != io.EOF {
			http.Error(w, "trailing JSON", http.StatusBadRequest)
			return
		}
		if err := app.SetConnected(*input.Connected); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	files, _ := fs.Sub(assets, "assets")
	mux.Handle("GET /", http.FileServer(http.FS(files)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		mux.ServeHTTP(w, r)
	})
}
