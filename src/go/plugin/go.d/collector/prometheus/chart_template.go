// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
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
// counterRawCharts selects counters that also get a raw-value chart
// (prometheus[.app].<metric>.raw) next to their rate chart.
func newAutogenSpec(app string, counterRawCharts []string) charttpl.Spec {
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
				CounterRawCharts:         slices.Clone(counterRawCharts),
				ExpireAfterSuccessCycles: chartExpireAfterCycles,
			},
		},
		Groups: []charttpl.Group{{Family: "prometheus"}},
	}
}

// buildChartTemplate returns the pure-autogen per-job chart template (no profiles).
func buildChartTemplate(app string, counterRawCharts []string) (string, error) {
	return marshalChartSpec(newAutogenSpec(app, counterRawCharts))
}

// buildMergedChartTemplate returns the per-job chart template: the autogen base
// plus the selected profiles' curated groups appended read-only (autogen stays
// the fallback for families no profile charts). With no profiles it is identical
// to buildChartTemplate.
func buildMergedChartTemplate(app string, counterRawCharts []string, profiles []promprofiles.Profile) (string, error) {
	spec := newAutogenSpec(app, counterRawCharts)
	for _, p := range profiles {
		if selector := p.AutogenSelector(); selector != nil {
			spec.Engine.Autogen.Rules = append(spec.Engine.Autogen.Rules, charttpl.EngineAutogenRule{
				Scope:    p.Match,
				Selector: *selector,
			})
		}
		// Template() returns an independent deep copy, so mutating g below cannot
		// corrupt the shared process-wide catalog.
		g, err := p.Template()
		if err != nil {
			return "", err
		}
		// The profile's root context_namespace is the exporter-type segment
		// (prometheus.<app>.<ns>.<context>). When the resolved app equals that namespace —
		// e.g. app fell back to the profile's own app: because the job has no app set
		// (by the user or service discovery) — drop the redundant segment so the
		// context is prometheus.<app>.<context>, not prometheus.<app>.<app>.<context>.
		if g.ContextNamespace == app {
			g.ContextNamespace = ""
		}
		spec.Groups = append(spec.Groups, g)
	}
	return marshalChartSpec(spec)
}

func marshalChartSpec(spec charttpl.Spec) (string, error) {
	raw, err := spec.MarshalTemplate()
	if err != nil {
		return "", fmt.Errorf("build prometheus chart template: %w", err)
	}
	return raw, nil
}
