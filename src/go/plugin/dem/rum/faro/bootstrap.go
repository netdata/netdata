// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Version is the pinned Faro web SDK served by the bootstrap.
const Version = "2.11.0"

// BootstrapOptions carries effective site settings. Propagate contains
// already-compiled JavaScript regular expression patterns.
type BootstrapOptions struct {
	MeasureRate        float64
	IncludeBots        bool
	ConsoleLogs        bool
	EventLogs          bool
	FrustrationSignals bool
	Tracing            bool
	Propagate          []string
}

//go:embed bootstrap.js
var bootstrapTemplate string

// Bootstrap renders site policy. The browser resolves transport and asset URLs
// from its external script element; request Host headers are never embedded.
func Bootstrap(key string, opts BootstrapOptions) string {
	k, _ := json.Marshal(key)
	opt := map[string]any{
		"sampling":           opts.MeasureRate,
		"bots":               beacon.BotPatternJS(),
		"includeBots":        opts.IncludeBots,
		"consoleLogs":        opts.ConsoleLogs,
		"eventLogs":          opts.EventLogs,
		"frustrationSignals": opts.FrustrationSignals,
		"tracing":            nil,
	}
	if opts.Tracing {
		opt["tracing"] = append([]string{}, opts.Propagate...)
	}
	o, _ := json.Marshal(opt)
	return strings.NewReplacer("__KEY__", string(k), "__OPT__", string(o), "__SDK_PATH__", SDKPath, "__TRACING_PATH__", TracingPath).
		Replace(bootstrapTemplate)
}
