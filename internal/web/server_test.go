package web

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/StarkTech-coder/sima/internal/model"
)

type stub struct{ connected bool }

func (*stub) View() model.View                    { return model.View{Ready: true} }
func (s *stub) SetConnected(connected bool) error { s.connected = connected; return nil }

func TestDashboardAndControls(t *testing.T) {
	s := &stub{}
	h := Handler(s)
	for _, path := range []string{"/", "/app.js", "/style.css", "/api/state"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
	}
	for _, tc := range []struct {
		body, origin string
		want         int
	}{
		{`{"connected":true}`, "", 204},
		{`{"connected":false}`, "https://untrusted.example", 403},
		{`{}`, "", 400},
		{`{"connected":false} {}`, "", 400},
		{`{"connected":false,"extra":1}`, "", 400},
	} {
		r := httptest.NewRequest("POST", "/api/link", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: got %d want %d", tc.body, w.Code, tc.want)
		}
	}
	if !s.connected {
		t.Fatal("invalid request changed gate")
	}
}

func TestEmbeddedAssetsAndPreviewPolicy(t *testing.T) {
	h := Handler(&stub{})
	for _, tc := range []struct{ path, mediaType, content string }{
		{"/", "text/html", `src="/app.js" defer`},
		{"/style.css", "text/css", ":root"},
		{"/app.js", "text/javascript", "function render(s)"},
		{"/favicon.svg", "image/svg+xml", "<svg"},
		{"/api/state", "application/json", `"ready":true`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			mediaType, _, err := mime.ParseMediaType(w.Header().Get("Content-Type"))
			if w.Code != http.StatusOK || err != nil || mediaType != tc.mediaType || !strings.Contains(w.Body.String(), tc.content) {
				t.Fatalf("asset %s: status=%d type=%s err=%v", tc.path, w.Code, mediaType, err)
			}
			policy := w.Header().Get("Content-Security-Policy")
			for _, required := range []string{"script-src 'self'", "connect-src 'self'", "frame-ancestors 'self' vscode-webview: vscode-file://vscode-app https://*.vscode-cdn.net"} {
				if !strings.Contains(policy, required) {
					t.Fatalf("missing policy %q: %s", required, policy)
				}
			}
			if strings.Contains(policy, "frame-ancestors 'none'") || strings.Contains(policy, "frame-ancestors *") || w.Header().Get("X-Frame-Options") != "" {
				t.Fatal("preview blocked or arbitrary framing allowed")
			}
		})
	}
	for _, path := range []string{"/missing.js", "/assets/app.js"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("missing asset returned %d", w.Code)
		}
	}
}
