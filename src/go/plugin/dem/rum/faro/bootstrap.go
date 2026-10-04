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

// SDKOrigin hosts the pinned browser SDK and tracing add-on.
const SDKOrigin = "https://cdn.jsdelivr.net"

// BootstrapOptions carries effective site settings. Propagate contains
// already-compiled JavaScript regular expression patterns.
type BootstrapOptions struct {
	MeasureRate float64
	IncludeBots bool
	Tracing     bool
	Propagate   []string
}

//go:embed bootstrap.js
var bootstrapTemplate string

// Bootstrap renders the loader; every caller-supplied value is JSON-encoded
// so a hostile Host header cannot break out of the script.
func Bootstrap(key, base string, opts BootstrapOptions) string {
	k, _ := json.Marshal(key)
	b, _ := json.Marshal(base)
	opt := map[string]any{
		"sampling":    opts.MeasureRate,
		"bots":        beacon.BotPatternJS(),
		"includeBots": opts.IncludeBots,
		"tracing":     nil,
	}
	if opts.Tracing {
		opt["tracing"] = append([]string{}, opts.Propagate...)
	}
	o, _ := json.Marshal(opt)
	return strings.NewReplacer("__KEY__", string(k), "__BASE__", string(b), "__OPT__", string(o), "__VERSION__", Version, "__SDK_ORIGIN__", SDKOrigin).
		Replace(bootstrapTemplate)
}
