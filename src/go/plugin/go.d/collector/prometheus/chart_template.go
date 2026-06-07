// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"gopkg.in/yaml.v2"
)

// chartExpireAfterCycles mirrors V1's stale-chart removal (10 missed cycles, see collect.go):
// chartengine autogen removes a chart/dimension after this many successful cycles in which its
// series is not seen.
const chartExpireAfterCycles = 10

// buildChartTemplate returns the per-job chart template (charttpl YAML) for the prometheus collector.
// It is a pure-autogen template: a stub group satisfies the schema, and chartengine autogen builds
// one chart per scraped metric, prefixing the context with context_namespace. The namespace is
// "prometheus" or "prometheus.<app>" so contexts match V1 (prometheus.<metric> /
// prometheus.<app>.<metric>); autogen joins namespace + "." + metric, so the app's separating dot is
// part of the namespace itself.
func buildChartTemplate(app string) (string, error) {
	namespace := "prometheus"
	if app != "" {
		namespace = "prometheus." + app
	}

	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: namespace,
		Engine: &charttpl.Engine{
			Autogen: &charttpl.EngineAutogen{
				Enabled:                  true,
				ExpireAfterSuccessCycles: chartExpireAfterCycles,
			},
		},
		Groups: []charttpl.Group{{Family: "prometheus"}},
	}

	if err := spec.Validate(); err != nil {
		return "", fmt.Errorf("build prometheus chart template: %w", err)
	}

	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal prometheus chart template: %w", err)
	}
	return string(raw), nil
}
