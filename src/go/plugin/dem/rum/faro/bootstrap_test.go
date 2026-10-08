// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"strings"
	"testing"
)

func TestBootstrapHasUserAndFrustration(t *testing.T) {
	js := Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
	})
	for _, want := range []string{"api.setUser = function (u)", "f.setUser(user)", "watchFrustration();", "signal('rage_click', sel)", "signal('dead_click', sel)", "signal('error_click', last.sel)"} {
		if !strings.Contains(js, want) {
			t.Fatalf("snippet missing %q", want)
		}
	}
	if strings.Index(js, "api.setUser = function") > strings.Index(js, "if (bot && !opt.includeBots) { return; }") {
		t.Fatal("setUser must exist even when a bot skips Faro, so site code never breaks")
	}
}

func TestBootstrapBotsAndTracingOptions(t *testing.T) {
	plain := Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
	})
	for _, want := range []string{`"includeBots":false`, `"tracing":null`, "navigator.webdriver === true", "if (bot && !opt.includeBots) { return; }", `"bots":"`} {
		if !strings.Contains(plain, want) {
			t.Fatalf("snippet missing %q", want)
		}
	}
	traced := Bootstrap(
		"shop",
		BootstrapOptions{
			MeasureRate: 1,
			Tracing:     true,
			Propagate:   []string{`^https://api\.example\.com(/|$)`},
		},
	)
	if !strings.Contains(traced, `"tracing":["^https://api\\.example\\.com(/|$)"]`) {
		t.Fatalf("propagation pattern missing:\n%s", traced)
	}
	if !strings.Contains(traced, "load('"+TracingPath+"', start, start") {
		t.Fatal("tracing add-on loader missing")
	}
}

func TestBootstrapCarriesMeasureSampling(t *testing.T) {
	js := Bootstrap("shop", BootstrapOptions{
		MeasureRate: 0.25,
	})
	if !strings.Contains(js, `"sampling":0.25`) ||
		!strings.Contains(js, "cfg.sessionTracking = { samplingRate: opt.sampling }") {
		t.Fatalf("25%% measure sampling missing:\n%s", js)
	}
	if js := Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
	}); !strings.Contains(js, `"sampling":1`) {
		t.Fatalf("100%% sampling not passed:\n%s", js)
	}
}

func TestBootstrapReadsDataVersionEnv(t *testing.T) {
	js := Bootstrap("demo", BootstrapOptions{
		MeasureRate: 1,
	})
	for _, want := range []string{
		"getOwnPropertyDescriptor(Document.prototype, 'currentScript')",
		"getAttribute('data-version')",
		"getAttribute('data-env')",
		"app.version = ver",
		"app.environment = env",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("bootstrap missing %q in:\n%s", want, js)
		}
	}
}

func TestBootstrapKeyQuoting(t *testing.T) {
	js := Bootstrap(`shop"); alert(1); ("`, BootstrapOptions{
		MeasureRate: 1,
	})
	if !strings.Contains(js, `})("shop\"); alert(1); (\"", {`) {
		t.Fatalf("site key not JSON quoted: %s", js)
	}
}
