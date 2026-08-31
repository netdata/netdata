package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// errorLogInterval is the minimum gap between repeated stderr messages from
// a single error site.  A persistent failure (e.g. unhealthy BPF map) would
// otherwise emit one line per collection cycle, flooding the operator log;
// 60 s strikes a balance between visibility and noise.
const errorLogInterval = 60 * time.Second

var (
	errorLogMu      sync.Mutex
	errorLogLastLog = map[string]time.Time{}
)

// rateLimitedStderr writes a message to stderr the first time and at most once
// per errorLogInterval.  The site key identifies the error site; use a short
// stable string per call site (e.g. "cachestat.snapshot").
//
// The message is formatted only when it will actually be written, so a site
// that fails every collection cycle costs one map lookup rather than a
// Sprintf.
func rateLimitedStderr(site, format string, args ...any) {
	if !claimErrorLogSlot(site) {
		return
	}

	fmt.Fprintf(os.Stderr, format, args...)
}

// claimErrorLogSlot reports whether this site may log now, recording the time
// when it may.
func claimErrorLogSlot(site string) bool {
	errorLogMu.Lock()
	defer errorLogMu.Unlock()

	now := time.Now()
	if last, ok := errorLogLastLog[site]; ok && now.Sub(last) < errorLogInterval {
		return false
	}
	errorLogLastLog[site] = now
	return true
}

// logPluginErr writes a "ebpf-go.plugin: <module> <what> failed: <err>\n"
// message through rateLimitedStderr.  Used by the cachestat / socket / dns
// collectors so each error site logs at most once per errorLogInterval.
func logPluginErr(site, module, what string, err error) {
	rateLimitedStderr(site, "ebpf-go.plugin: %s %s failed: %v\n", module, what, err)
}

// warnIntegrationDisabled explains, once per module, why an enabled collector is
// only producing its host-wide charts.
//
// The per-application and per-cgroup charts come from apps.plugin and
// cgroups.plugin reading this plugin's shared-memory segment, and the plugin
// only collects and publishes per-PID rows when `apps` and/or `cgroups` are on.
// Both ship as `no` in the stock ebpf.d.conf, so enabling a module alone is not
// enough — which is easy to miss because the global charts do appear.
//
// Only opt-in modules should call this: a module that is enabled by default
// would print on every stock install.
func warnIntegrationDisabled(module string) {
	fmt.Fprintf(os.Stderr,
		"ebpf-go.plugin: %s: global charts only — per-application and per-cgroup charts need "+
			"'apps = yes' and/or 'cgroups = yes' in the [global] section of ebpf.d.conf\n",
		module)
}
