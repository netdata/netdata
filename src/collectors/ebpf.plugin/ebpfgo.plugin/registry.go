// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"maps"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// NewEbpfRegistry returns the registry of eBPF collectors.
// All collectors self-register via init() through their init() functions,
// which populate collectorapi.DefaultRegistry.
func NewEbpfRegistry() collectorapi.Registry {
	// Clone the default registry which has all eBPF collectors self-registered
	return maps.Clone(collectorapi.DefaultRegistry)
}
