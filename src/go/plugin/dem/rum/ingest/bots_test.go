// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

const headlessUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36"

// Bots are filtered by default: counted, never ingested.
func TestCollectFiltersBots(t *testing.T) {
	sink := newRecSink()
	_, ts := newTestServer(t, testCfg(), sink, nil)
	origin := "https://shop.example.com"
	for _, hdr := range []map[string]string{
		{"Origin": origin, "User-Agent": headlessUA},
		{"Origin": origin, "User-Agent": "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
	} {
		if resp := post(t, ts, "/rum/shop/collect", faroBody, hdr); resp.StatusCode != 204 {
			t.Fatalf("bot beacon status %d", resp.StatusCode)
		}
	}
	if resp := post(t, ts, "/rum/shop/collect?bot=1", faroBody, map[string]string{"Origin": origin}); resp.StatusCode != 204 {
		t.Fatalf("snippet-flagged bot status %d", resp.StatusCode)
	}
	n, rejects := sink.snapshot()
	if n != 0 || rejects["shop/"+beacon.RejectBot] != 3 {
		t.Fatalf("ingested %d, rejects %v", n, rejects)
	}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": origin}); resp.StatusCode != 202 {
		t.Fatalf("a real browser must still be accepted: %d", resp.StatusCode)
	}
}

func TestCollectKeepsBotsWhenIncluded(t *testing.T) {
	cfg := testCfg()
	cfg.Sites[0].Bots = config.BotsInclude
	sink := newRecSink()
	_, ts := newTestServer(t, cfg, sink, nil)
	if resp := post(t, ts, "/rum/shop/collect", faroBody, map[string]string{"Origin": "https://shop.example.com", "User-Agent": headlessUA}); resp.StatusCode != 202 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ingested) != 1 || sink.ingested[0].Device != beacon.DeviceBot {
		t.Fatalf("ingested %+v", sink.ingested)
	}
}

// The snippet skips Faro for bots unless the site keeps them, and only
// loads the tracing add-on when tracing is on.
func TestBootstrapBotsAndTracingOptions(t *testing.T) {
	plain := bootstrapJS("shop", "https://rum.example", &site{
		measureRate: 1,
	})
	for _, want := range []string{`"includeBots":false`, `"tracing":null`, "navigator.webdriver === true", "if (bot && !opt.includeBots) { return; }", `"bots":"`} {
		if !strings.Contains(plain, want) {
			t.Fatalf("snippet missing %q", want)
		}
	}
	cfg := testCfg()
	cfg.Sites[0].Tracing = &config.RumTracing{
		Enabled:     true,
		PropagateTo: []string{"https://api.example.com"},
	}
	st := compileSite(cfg.Sites[0])
	traced := bootstrapJS("shop", "https://rum.example", st)
	if !strings.Contains(traced, `"tracing":["^https://api\\.example\\.com(/|$)"]`) {
		t.Fatalf("propagation pattern missing:\n%s", traced)
	}
	if !strings.Contains(traced, "load('faro-web-tracing', start, start)") {
		t.Fatal("tracing add-on loader missing")
	}
}
