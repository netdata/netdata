// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	collector "github.com/netdata/netdata/go/plugins/plugin/dem/collector/rum"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/httpapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/require"
)

// TestOnboardingBrowser is opt-in: provide DEM_BROWSER_EXECUTABLE and
// DEM_PUPPETEER_MODULE (an existing puppeteer-core module's absolute path).
// It installs nothing. Run with go test -run '^TestOnboardingBrowser$' -v
// ./plugin/dem/rum/httpapi from src/go. Browser failures are fatal once opted in.
func TestOnboardingBrowser(t *testing.T) {
	browserPath, modulePath := os.Getenv("DEM_BROWSER_EXECUTABLE"), os.Getenv("DEM_PUPPETEER_MODULE")
	if browserPath == "" && modulePath == "" {
		t.Skip("set DEM_BROWSER_EXECUTABLE and DEM_PUPPETEER_MODULE for real-browser integration")
	}
	require.NotEmpty(t, browserPath)
	require.NotEmpty(t, modulePath)
	node, err := exec.LookPath("node")
	require.NoError(t, err)
	for _, path := range []string{browserPath, modulePath} {
		_, err := os.Stat(path)
		require.NoError(t, err)
	}
	cases := []string{
		"host-csp", "nonce-csp", "strict-dynamic", "dynamic", "isolated", "anonymous-cors", "clobbered-script",
		"tracing-failure", "core-failure", "disabled", "wrong-origin", "alias-coherent", "alias-bootstrap-only",
	}
	hub := registry.New()
	db, err := journal.Open(context.Background(), filepath.Join(t.TempDir(), "history.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	type requestRecord struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Status int    `json:"status"`
	}
	var requestsMu sync.Mutex
	var requests []requestRecord
	receiverConfig := config.Receiver{
		MaxBodyBytes: 1 << 20,
		RateLimit: config.RateLimit{
			PerIPPerMin:   10000,
			PerSitePerSec: 1000,
		},
	}
	handler := httpapi.New(&receiverConfig, hub, nil).Handler()
	fn := functions.New(query.New(hub, history.NewStore(db)))
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/function":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(fn.HandleRaw(r.Context(), funcapi.RawMethodRequest{
				Method: "rum-sites",
			}).RawResponse)
			return
		case "/requests":
			requestsMu.Lock()
			defer requestsMu.Unlock()
			_ = json.NewEncoder(w).Encode(requests)
			return
		}
		recorded := &browserResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		defer func() {
			requestsMu.Lock()
			requests = append(requests, requestRecord{r.Method, r.URL.Path, recorded.status})
			requestsMu.Unlock()
		}()
		for _, prefix := range []string{"/coherent", "/bootstrap-only"} {
			if strings.HasPrefix(r.URL.Path, prefix+"/rum/") && strings.HasSuffix(r.URL.Path, ".js") &&
				!strings.Contains(r.URL.Path, "/assets/") {
				http.Redirect(recorded, r, "/proxy"+strings.TrimPrefix(r.URL.Path, prefix), http.StatusFound)
				return
			}
		}
		for _, prefix := range []string{"/proxy", "/coherent"} {
			if strings.HasPrefix(r.URL.Path, prefix+"/") {
				http.StripPrefix(prefix, handler).ServeHTTP(recorded, r)
				return
			}
		}
		http.NotFound(recorded, r)
	}))
	t.Cleanup(receiver.Close)
	unpublish := hub.PublishReceiver(registry.Availability{
		Serving:   true,
		PublicURL: receiver.URL + "/proxy",
	})
	t.Cleanup(unpublish.Close)

	// This page server is a different origin from the receiver. Only actual
	// browser-created telemetry enters the production handler and aggregator.
	receiverURL := receiver.URL
	website := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A scriptless injected image controls this harmless attacker endpoint.
		// Executing it means the loader trusted a named element as currentScript.
		if strings.HasPrefix(r.URL.Path, "/injected/rum/assets/") {
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = fmt.Fprint(w, "window.unexpectedSDKExecuted = true;")
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/")
		found := false
		for _, candidate := range cases {
			if candidate == key {
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		prefix := "/proxy"
		if key == "alias-coherent" {
			prefix = "/coherent"
		}
		if key == "alias-bootstrap-only" {
			prefix = "/bootstrap-only"
		}
		src := receiverURL + prefix + "/rum/" + key + ".js"
		nonce, extra := "", ""
		scripts := receiverURL
		switch key {
		case "nonce-csp", "dynamic":
			scripts, nonce = "'nonce-fixture-nonce'", ` nonce="fixture-nonce"`
		case "strict-dynamic", "clobbered-script":
			scripts, nonce = "'nonce-fixture-nonce' 'strict-dynamic'", ` nonce="fixture-nonce"`
		case "isolated":
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		case "anonymous-cors":
			extra = ` crossorigin="anonymous"`
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// script-src-elem deliberately overrides script-src, exercising browser CSP
		// enforcement rather than a server-side approximation of the policy.
		w.Header().
			Set("Content-Security-Policy", "default-src 'none'; script-src 'none'; script-src-elem "+scripts+"; connect-src "+receiverURL)
		tag := `<script async` + nonce + extra + ` src="` + html.EscapeString(src) + `"></script>`
		if key == "dynamic" {
			encoded, _ := json.Marshal(src)
			tag = `<script nonce="fixture-nonce">const s=document.createElement('script');s.async=true;s.nonce='fixture-nonce';s.src=` + string(
				encoded,
			) + `;document.head.appendChild(s);</script>`
		}
		if key == "clobbered-script" {
			tag = `<img name="currentScript" src="/injected/rum/site.js">` + tag
		}
		_, _ = fmt.Fprint(
			w,
			"<!doctype html><meta charset=utf-8><title>RUM onboarding fixture</title>"+tag+"<h1>Browser installation fixture</h1>",
		)
	}))
	t.Cleanup(website.Close)

	for _, key := range cases {
		c := collector.New(collector.Dependencies{
			Registry: hub,
			History:  history.NewStore(db),
		})
		c.Name, c.Bots = key, config.BotsInclude // Explicit eligibility for headless browser fixtures.
		c.AllowedOrigins = []string{website.URL}
		if key == "wrong-origin" {
			c.AllowedOrigins = []string{"https://not-allowed.example"}
		}
		if key == "disabled" {
			zero := 0.0
			c.MeasureSampleRate = &zero
		}
		if key == "tracing-failure" {
			// No tracing bundle means no spans. Keep any exporter connection within
			// this fixture rather than using a developer's default OTLP service.
			endpoint := receiver.URL
			c.Tracing = &config.Tracing{
				Enabled: true,
				Destination: config.Destination{
					Endpoint: &endpoint,
				},
			}
		}
		require.NoError(t, c.Init(context.Background()))
		ctx, cancel := context.WithCancel(context.Background())
		ready, done := make(chan struct{}), make(chan error, 1)
		go func() { done <- c.Run(ctx, func() { close(ready) }) }()
		t.Cleanup(func() {
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				t.Error("site runtime did not stop")
			}
		})
		select {
		case <-ready:
		case err := <-done:
			t.Fatalf("site %s stopped before ready: %v", key, err)
		case <-time.After(10 * time.Second):
			t.Fatalf("site %s did not become ready", key)
		}
	}
	options, err := json.Marshal(map[string]any{"website": website.URL, "receiver": receiver.URL, "cases": cases})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, node, "testdata/onboarding-browser.cjs", string(options))
	output, err := command.CombinedOutput()
	t.Logf("browser integration:\n%s", output)
	require.NoError(t, err)
}

type browserResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *browserResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *browserResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
