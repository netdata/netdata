// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
