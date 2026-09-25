// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const chartIDCollisionWarning = "chartengine: dropped "

func TestChartIDCollisionWarning(t *testing.T) {
	documentSet := func(t *testing.T) *TemplateSet {
		spec := charttpl.Spec{
			Version: charttpl.VersionV1,
		}
		for _, entry := range collisionEntries([]string{"first", "second"}, "first", "second") {
			spec.Groups = append(spec.Groups, entry.Groups[0])
		}
		data, err := spec.MarshalTemplate()
		require.NoError(t, err)
		set, err := NewTemplateSetYAML([]byte(data))
		require.NoError(t, err)
		return set
	}
	separateMetrics := func(t *testing.T) *TemplateSet {
		return testTemplateSet(t, nativeEntry("a", "shared", "a_requests"), nativeEntry("b", "shared", "b_requests"))
	}

	tests := map[string]struct {
		set    func(t *testing.T) *TemplateSet
		cycles []map[string]float64
		want   []string
	}{
		"one series routed to two entries": {
			set: func(t *testing.T) *TemplateSet {
				return testTemplateSet(t, collisionEntries([]string{"a", "b"}, "a", "b")...)
			},
			cycles: []map[string]float64{{"requests": 1}, {"requests": 2}},
			want:   []string{"dropped 1 series route(s)", "chart 'shared'", "owned by entry 'a' chart g0.c0", "rejected entry 'b' chart g0.c0"},
		},
		"owner from an earlier cycle": {
			set:    separateMetrics,
			cycles: []map[string]float64{{"b_requests": 1}, {"a_requests": 1}},
			want:   []string{"chart 'shared'", "owned by entry 'b' chart g0.c0", "rejected entry 'a' chart g0.c0"},
		},
		"yaml document": {
			set:    documentSet,
			cycles: []map[string]float64{{"requests": 1}, {"requests": 2}},
			want:   []string{"chart 'shared'", "owned by template g0.c0", "rejected template g1.c0"},
		},
		"no collision": {
			set: func(t *testing.T) *TemplateSet {
				return testTemplateSet(t, collisionEntries([]string{"a", "b"}, "a")...)
			},
			cycles: []map[string]float64{{"requests": 1}, {"requests": 2}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			e, err := New(WithRuntimeStore(nil), WithLogger(logger.NewWithWriter(&logs)))
			require.NoError(t, err)
			store := metrix.NewCollectorStore()
			set := tc.set(t)
			for _, values := range tc.cycles {
				attempt := templateAttempt(t, e, store, set, values)
				require.NoError(t, attempt.Commit())
			}

			if tc.want == nil {
				assert.NotContains(t, logs.String(), chartIDCollisionWarning)
				return
			}
			// Collisions recur every cycle; the warning is rate limited.
			assert.Equal(t, 1, strings.Count(logs.String(), chartIDCollisionWarning))
			for _, want := range tc.want {
				assert.Contains(t, logs.String(), want)
			}
		})
	}
}

// Engines can share one logger and its rate limiter: a job's host-scope engines
// do, and so do runtime metrics components. The warning period is per chart type
// ID namespace, so one job warns once across its scopes while components do not
// suppress each other.
func TestChartIDCollisionWarningScope(t *testing.T) {
	tests := map[string]struct {
		typeIDs  []string
		warnings int
	}{
		"engines of one job":        {typeIDs: []string{"module_job", "module_job"}, warnings: 1},
		"engines of two namespaces": {typeIDs: []string{"netdata.component_a", "netdata.component_b"}, warnings: 2},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			shared := logger.NewWithWriter(&logs)
			set := testTemplateSet(t, collisionEntries([]string{"a", "b"}, "a", "b")...)
			for i, typeID := range tc.typeIDs {
				e, err := New(
					WithRuntimeStore(nil),
					WithEmitTypeIDBudgetPrefix(typeID),
					WithLogger(shared.With("engine", i)),
				)
				require.NoError(t, err)
				attempt := templateAttempt(t, e, metrix.NewCollectorStore(), set, map[string]float64{"requests": 1})
				require.NoError(t, attempt.Commit())
			}

			assert.Equal(t, tc.warnings, strings.Count(logs.String(), chartIDCollisionWarning))
		})
	}
}

// Authored charts displacing automatic ones, and automatic routes losing to an
// authored owner, are the intended "template wins" rule, not collisions to report.
func TestChartIDCollisionWarningIgnoresAutogen(t *testing.T) {
	tests := map[string]struct {
		autogenMetric  string
		authoredMetric string
	}{
		"authored chart displaces an autogen owner": {
			autogenMetric:  "aaa_total",
			authoredMetric: "zzz_total",
		},
		"autogen route loses to an authored owner": {
			autogenMetric:  "zzz_total",
			authoredMetric: "aaa_total",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			e, err := New(
				WithRuntimeStore(nil),
				WithLogger(logger.NewWithWriter(&logs)),
				WithEnginePolicy(EnginePolicy{
					Autogen: &AutogenPolicy{
						Enabled: true,
					},
				}),
			)
			require.NoError(t, err)
			template := fmt.Sprintf(`
version: v1
groups:
  - family: Test
    metrics: [svc.%s]
    charts:
      - id: svc.%s-method=GET
        title: Authored
        context: svc.authored
        units: requests/s
        dimensions:
          - selector: svc.%s{method="GET"}
            name: total
`, tc.authoredMetric, tc.autogenMetric, tc.authoredMetric)
			require.NoError(t, e.LoadYAML([]byte(template), 1))

			store := metrix.NewCollectorStore()
			cycle := mustCycleController(t, store)
			meter := store.Write().SnapshotMeter("svc")
			labels := meter.LabelSet(metrix.Label{Key: "method", Value: "GET"})
			cycle.BeginCycle()
			meter.Counter(tc.autogenMetric).ObserveTotal(1, labels)
			meter.Counter(tc.authoredMetric).ObserveTotal(2, labels)
			require.NoError(t, cycle.CommitCycleSuccess())

			_, err = buildPlan(e, store.Read(metrix.ReadFlatten()))
			require.NoError(t, err)
			assert.NotContains(t, logs.String(), chartIDCollisionWarning)
		})
	}
}
