// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func resolveRuntimeAlgorithm(configured program.Algorithm, kind metrix.MetricKind) program.Algorithm {
	if configured != program.AlgorithmAuto {
		return configured
	}
	if kind == metrix.MetricKindCounter {
		return program.AlgorithmIncremental
	}
	return program.AlgorithmAbsolute
}

func finalizeRouteAlgorithm(route routeBinding, kind metrix.MetricKind) routeBinding {
	route.Algorithm = resolveRuntimeAlgorithm(route.Algorithm, kind)
	return route
}
