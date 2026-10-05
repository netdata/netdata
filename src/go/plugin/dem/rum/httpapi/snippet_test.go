// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPingAndRejectedOrigin(t *testing.T) {
	s := newFixture(testCfg(), newRecSink(), nil)
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rum/ping", nil))
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("ping = %d %v", rec.Code, rec.Header())
	}

	req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://www.shop.example.com")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("collect from a foreign origin = %d", rec.Code)
	}
	got, ok := s.route("shop").diagnostics.LastRejectedOrigin()
	if !ok || got.Origin != "https://www.shop.example.com" {
		t.Fatalf("last rejected = %+v %v", got, ok)
	}
}

func TestBootstrapUsesConfiguredPublicBase(t *testing.T) {
	for _, tt := range []struct{ name, siteURL, want string }{{"receiver", "", "https://rum.example.org/edge"}, {"site override", "https://site.example.org/telemetry", "https://site.example.org/telemetry"}} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testCfg()
			cfg.PublicURL = "https://rum.example.org/edge"
			cfg.Sites[0].PublicURL = tt.siteURL
			server := newFixture(cfg, newRecSink(), nil)
			req := httptest.NewRequest("GET", "http://internal/rum/shop.js", nil)
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if !strings.Contains(rec.Body.String(), `"`+tt.want+`"`) {
				t.Fatalf("bootstrap did not preserve configured public base %q", tt.want)
			}
		})
	}
}
