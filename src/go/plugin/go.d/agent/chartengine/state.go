// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"

type engineState struct {
	cfg          engineConfig
	program      *program.Program
	matchIndex   matchIndex
	routeCache   *routeCache
	materialized materializedState
	stats        engineStats
}
