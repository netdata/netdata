// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrapAndDemo(t *testing.T) {
	cfg := testCfg()
	cfg.Sites[0].Name = "demo"
	sink := newRecSink()
	_, ts := newTestServer(t, cfg, sink, nil)

	resp, err := ts.Client().Get(ts.URL + "/rum/demo.js")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), "javascript") {
		t.Fatalf("bootstrap status %d type %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	js := string(body)
	for _, want := range []string{faro.SDKPath, faro.TracingPath, `})("demo", {`, "encodeURIComponent(k) + '/collect'"} {
		if !strings.Contains(js, want) {
			t.Fatalf("bootstrap missing %q in:\n%s", want, js)
		}
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("bootstrap is a public asset and must permit anonymous CORS reads")
	}

	resp, err = ts.Client().Get(ts.URL + "/rum/demo")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `src="demo.js"`) {
		t.Fatalf("demo status %d body %s", resp.StatusCode, body)
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

// The snippet is revalidated, never cached for a fixed time: an
// unchanged snippet answers 304, a settings change a new body.
func TestBootstrapRevalidates(t *testing.T) {
	s, ts := newTestServer(t, testCfg(), newRecSink(), nil)
	get := func(etag string) *http.Response {
		req, _ := http.NewRequest("GET", ts.URL+"/rum/shop.js", nil)
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}
	first := get("")
	etag := first.Header.Get("ETag")
	if first.StatusCode != 200 || first.Header.Get("Cache-Control") != "private, no-cache" || etag == "" {
		t.Fatalf("status %d cache %q etag %q", first.StatusCode, first.Header.Get("Cache-Control"), etag)
	}
	if resp := get(etag); resp.StatusCode != http.StatusNotModified {
		t.Fatalf("unchanged snippet: %d, want 304", resp.StatusCode)
	}
	cfg := testCfg()
	cfg.Sites[0].MeasureSampleRate = new(0.5)
	s.Update(cfg)
	if resp := get(etag); resp.StatusCode != 200 || resp.Header.Get("ETag") == etag {
		t.Fatalf("changed settings must send a new snippet: %d", resp.StatusCode)
	}
}

func TestConsoleOptInBootstrapAndAdmission(t *testing.T) {
	for _, tt := range []struct {
		name string
		logs *config.EventLogs
		on   bool
	}{
		{"absent", nil, false},
		{"disabled saved console", &config.EventLogs{
			IncludeConsoleLogs: true,
		}, false},
		{"logs without console", &config.EventLogs{
			Enabled: true,
		}, false},
		{"enabled", &config.EventLogs{
			Enabled:            true,
			IncludeConsoleLogs: true,
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testCfg()
			cfg.Sites[0].EventLogs = tt.logs
			sink := newRecSink()
			s := newFixture(cfg, sink, nil)
			h := s.Handler()
			get := httptest.NewRecorder()
			h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/rum/shop.js", nil))
			require.Equal(t, http.StatusOK, get.Code)
			expected := `"consoleLogs":false`
			if tt.on {
				expected = `"consoleLogs":true`
			}
			assert.Contains(t, get.Body.String(), expected)
			body := `{"logs":[{"level":"error","message":"console.error: optional"}],"exceptions":[{"type":"Error","value":"core exception"}]}`
			ingest := func() {
				req := httptest.NewRequest(http.MethodPost, "/rum/shop/collect", strings.NewReader(body))
				req.Header.Set("Origin", "https://shop.example.com")
				req.Header.Set("User-Agent", browserUA)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				require.Equal(t, http.StatusAccepted, rec.Code)
			}
			ingest()
			require.Len(t, sink.ingested, 1)
			require.Len(t, sink.ingested[0].Errors, 1)
			assert.Equal(t, "core exception", sink.ingested[0].Errors[0].Message)
			if tt.on {
				require.Len(t, sink.ingested[0].Logs, 1)
			} else {
				assert.Empty(t, sink.ingested[0].Logs)
			}
			if tt.on {
				cfg.Sites[0].EventLogs = &config.EventLogs{
					IncludeConsoleLogs: true,
				}
				s.Update(cfg)
				refreshed := httptest.NewRecorder()
				h.ServeHTTP(refreshed, httptest.NewRequest(http.MethodGet, "/rum/shop.js", nil))
				assert.Contains(t, refreshed.Body.String(), `"consoleLogs":false`)
				assert.NotEqual(t, get.Header().Get("ETag"), refreshed.Header().Get("ETag"))
				ingest() // A page with the previous bootstrap still sends the same logs.
				require.Len(t, sink.ingested, 2)
				assert.Empty(t, sink.ingested[1].Logs)
				require.Len(t, sink.ingested[1].Errors, 1)
			}
		})
	}
}
