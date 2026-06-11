// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"gopkg.in/yaml.v2"
)

// chartExpireAfterCycles mirrors V1's stale-chart removal (a chart was dropped after 10 missed
// cycles): chartengine autogen removes a chart/dimension after this many successful cycles in
// which its series is not seen.
const chartExpireAfterCycles = 10

// newAutogenSpec is the base per-job chart-template spec: the per-app context
// namespace ("prometheus" / "prometheus.<app>") with autogen enabled and
// V1-matching expiry. A stub group satisfies the schema; chartengine autogen
// builds one chart per scraped metric, prefixing the context with the namespace
// (autogen joins namespace + "." + metric, so the app's separating dot is part
// of the namespace itself), so contexts match V1 (prometheus[.app].<metric>).
func newAutogenSpec(app string) charttpl.Spec {
	namespace := "prometheus"
	if app != "" {
		namespace = "prometheus." + app
	}

	return charttpl.Spec{
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
}

// buildChartTemplate returns the pure-autogen per-job chart template (no profiles).
func buildChartTemplate(app string) (string, error) {
	return marshalChartSpec(newAutogenSpec(app))
}

// buildMergedChartTemplate returns the per-job chart template: the autogen base
// plus the selected profiles' curated groups appended read-only (autogen stays
// the fallback for families no profile charts). With no profiles it is identical
// to buildChartTemplate.
func buildMergedChartTemplate(app string, profiles []promprofiles.Profile) (string, error) {
	spec := newAutogenSpec(app)
	for _, p := range profiles {
		spec.Groups = append(spec.Groups, p.Template)
	}
	return marshalChartSpec(spec)
}

func marshalChartSpec(spec charttpl.Spec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", fmt.Errorf("build prometheus chart template: %w", err)
	}

	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("marshal prometheus chart template: %w", err)
	}
	return string(raw), nil
}
