// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextLimitCanonicalIdentityAcrossProviders(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, namespace := range []bool{false, true} {
			for _, partial := range []bool{false, true} {
				t.Run(fmt.Sprintf("native_%t/namespace_%t/partial_%t", native, namespace, partial), func(t *testing.T) {
					firstNS, secondNS, firstContext, secondContext := "", "", "work", "wo'rk"
					canonical := "work"
					if namespace {
						firstNS, secondNS, secondContext = "ns", "n's", "work"
						canonical = "ns.work"
					}
					secondDimension := "name_from_label: client"
					populations := []int{1, 2, 2, 1}
					if partial {
						secondDimension = "name: total"
						populations = []int{2, 4, 4, 2}
					}
					template := fmt.Sprintf(`version: v1
groups:
  - family: Work
    context_namespace: %q
    metrics: [work]
    charts:
      - id: first
        title: Work
        context: %q
        units: tasks
        dimensions:
          - selector: work
            name_from_label: client
  - family: Work
    context_namespace: %q
    metrics: [work]
    charts:
      - id: second
        title: Work
        context: %q
        units: tasks
        dimensions:
          - selector: work
            %s
  - family: Work
    metrics: [work]
    charts:
      - id: complete
        title: Complete
        context: complete
        units: tasks
        dimensions:
          - selector: work
            name: total
`, firstNS, firstContext, secondNS, secondContext, secondDimension)
					limit := 3
					var set *TemplateSet
					var err error
					if native {
						spec, err := charttpl.DecodeYAML([]byte(template))
						require.NoError(t, err)
						var entries []TemplateEntry
						for i, group := range spec.Groups {
							ns := group.ContextNamespace
							group.ContextNamespace = ""
							entries = append(entries, TemplateEntry{ID: fmt.Sprint(i), ContextNamespace: ns, Groups: []charttpl.Group{group}})
						}
						set, err = NewTemplateSet(TemplateSetSpec{Entries: entries, Policy: EnginePolicy{MaxOutputSeriesPerContext: &limit}})
					} else {
						set, err = NewTemplateSetYAML([]byte(template))
						require.NoError(t, err)
						set, err = PrepareTemplateSet(set, WithEnginePolicy(EnginePolicy{MaxOutputSeriesPerContext: &limit}))
					}
					require.NoError(t, err)
					var logs bytes.Buffer
					var omissions []PlanRouteDiagnostic
					e, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode(), WithLogger(logger.NewWithWriter(&logs)), WithPlanRouteDiagnosticObserver(func(d PlanRouteDiagnostic) {
						if d.Decision == PlanRouteContextRejected {
							omissions = append(omissions, d)
						}
					}))
					require.NoError(t, err)
					store := metrix.NewCollectorStore()
					for step, n := range populations {
						logs.Reset()
						omissions = nil
						before := snapshotMaterialized(e.state.materialized)
						reader := contextLimitCycle(t, store, n, 0, 1)
						attempt, err := e.PreparePlanWithOptions(reader, PlanOptions{TemplateSet: set})
						require.NoError(t, err)
						plan := attempt.Plan()
						count := n * 2
						if partial {
							count = n + 1
						}
						updates := contextLimitUpdates(plan)
						if count > limit {
							assert.NotContains(t, updates, "first")
							assert.NotContains(t, updates, "second")
							require.Len(t, omissions, 2)
							for _, d := range omissions {
								assert.Equal(t, canonical, d.Context)
								assert.Equal(t, count, d.ContextSeriesCount)
							}
						} else {
							assert.Contains(t, updates, "first")
							assert.Contains(t, updates, "second")
							assert.Empty(t, omissions)
						}
						require.Contains(t, updates, "complete")
						assert.Equal(t, float64(n), updates["complete"].Values[0].Float64)
						assert.Empty(t, logs.String())
						attempt.Abort()
						assert.Equal(t, before, snapshotMaterialized(e.state.materialized))
						assert.Empty(t, logs.String())
						attempt, err = e.PreparePlanWithOptions(reader, PlanOptions{TemplateSet: set})
						require.NoError(t, err)
						assert.Equal(t, plan, attempt.Plan())
						require.NoError(t, attempt.Commit())
						assert.Equal(t, map[string]int{canonical: count, "complete": 1}, e.state.outputContextCounts)
						switch step {
						case 1:
							assert.Contains(t, logs.String(), "newly omitted=1")
							assert.Contains(t, logs.String(), canonical)
						case 3:
							assert.Contains(t, logs.String(), "no longer omitted=1")
							assert.Contains(t, logs.String(), canonical)
						default:
							assert.Empty(t, logs.String())
						}
					}
				})
			}
		}
	}
}

func TestContextLimitAliasAllocationEnvelope(t *testing.T) {
	for _, n := range []int{1000, 10000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			reader := contextLimitCycle(t, metrix.NewCollectorStore(), n, 0, 1)
			allocations := func(context string) float64 {
				template := fmt.Sprintf(`version: v1
groups:
  - family: Work
    metrics: [work]
    charts:
      - id: work
        title: Work
        context: %q
        units: tasks
        instances: {by_labels: [client]}
        dimensions:
          - selector: work
            name: value
`, context)
				e := newContextLimitEngine(t, 1000, template)
				_, err := buildPlan(e, reader)
				require.NoError(t, err)
				return testing.AllocsPerRun(5, func() {
					if _, err := buildPlan(e, reader); err != nil {
						t.Fatal(err)
					}
				})
			}
			canonical, aliased := allocations("work"), allocations("wo'rk")
			// Only a distinct changed context needs a normalized string and alias
			// map. Neither admitted nor omitted instances allocate another copy.
			assert.LessOrEqual(t, aliased, canonical+8)
		})
	}
}
