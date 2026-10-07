// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBootstrapDoesNotEmbedAdvertisedOrForwardedAddress(t *testing.T) {
	cfg := testCfg()
	cfg.PublicURL = "https://advertised.example.org/edge"
	cfg.Sites[0].PublicURL = "https://override.example.org/prefix"
	server := newFixture(cfg, newRecSink(), nil)
	req := httptest.NewRequest(http.MethodGet, "http://internal/rum/shop.js", nil)
	req.Header.Set("X-Forwarded-Host", "untrusted.example.org")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	for _, address := range []string{"advertised.example.org", "override.example.org", "internal", "untrusted.example.org"} {
		assert.NotContains(t, rec.Body.String(), address)
	}
	assert.Contains(t, rec.Body.String(), "getOwnPropertyDescriptor(Document.prototype, 'currentScript')")
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "cross-origin", rec.Header().Get("Cross-Origin-Resource-Policy"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}
