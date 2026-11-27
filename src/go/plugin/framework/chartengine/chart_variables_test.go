// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"cmp"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type variableSeries struct {
	metric string
	labels []metrix.Label
	value  float64
}

func eth(metric string, value float64, extra ...metrix.Label) variableSeries {
	return variableSeries{
		metric: metric,
		labels: append([]metrix.Label{{Key: "device", Value: "eth0"}}, extra...),
		value:  value,
	}
}

func variableAttempt(
	t *testing.T,
	e *Engine,
	store metrix.CollectorStore,
	set *TemplateSet,
	series []variableSeries,
) PlanAttempt {
	t.Helper()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	for _, s := range series {
		meter := store.Write().SnapshotMeter("")
		if len(s.labels) > 0 {
			meter = meter.WithLabels(s.labels...)
		}
		meter.Gauge(s.metric).Observe(s.value)
	}
	require.NoError(t, cc.CommitCycleSuccess())
	attempt, err := e.PreparePlanWithOptions(store.Read(metrix.ReadFlatten()), PlanOptions{
		TemplateSet: set,
	})
	require.NoError(t, err)
	return attempt
}

// netEntry is one per-device chart with received/sent dimensions and the given
// variables, all fed by gauges.
func netEntry(id string, variables ...charttpl.ChartVariable) TemplateEntry {
	return TemplateEntry{
		ID:               id,
		ContextNamespace: "net",
		Groups: []charttpl.Group{{
			Family:  "net",
			Metrics: []string{"received", "sent", "speed", "speed_limit"},
			Charts: []charttpl.Chart{{
				Title:   "Bandwidth",
				Context: "net",
				Units:   "bits/s",
				Instances: &charttpl.Instances{
					ByLabels: []string{"device"},
				},
				Dimensions: []charttpl.Dimension{
					{Selector: "received", Name: "received"},
					{Selector: "sent", Name: "sent"},
				},
				Variables: variables,
			}},
		}},
	}
}

func chartUpdate(plan Plan, chartID string) *UpdateChartAction {
	for _, action := range plan.Actions {
		if update, ok := action.(UpdateChartAction); ok && update.ChartID == chartID {
			return &update
		}
	}
	return nil
}

func TestPlanChartVariablesSurviveLabelReplay(t *testing.T) {
	// Label replay walks the same snapshot a second time; a variable value must
	// still be the chart reducer over the snapshot (sum: 1e9 + 2e9), not doubled.
	speedSeries := []variableSeries{
		eth("speed", 1e9, metrix.Label{
			Key:   "port",
			Value: "a",
		}),
		eth("speed", 2e9, metrix.Label{
			Key:   "port",
			Value: "b",
		}),
	}
	tests := map[string]struct {
		limit  *int
		cycles [][]variableSeries
	}{
		"deferred labels of a new chart under an output limit": {
			limit:  new(100),
			cycles: [][]variableSeries{append([]variableSeries{eth("received", 1)}, speedSeries...)},
		},
		"membership change of a published chart": {
			cycles: [][]variableSeries{
				append([]variableSeries{eth("received", 1), eth("sent", 1)}, speedSeries...),
				append([]variableSeries{eth("received", 2)}, speedSeries...),
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New(WithRuntimeStore(nil))
			require.NoError(t, err)
			set, err := NewTemplateSet(TemplateSetSpec{
				Policy: EnginePolicy{
					MaxOutputSeriesPerContext: tc.limit,
				},
				Entries: []TemplateEntry{netEntry("net", charttpl.ChartVariable{
					Name:     "nic_speed",
					Selector: "speed",
				})},
			})
			require.NoError(t, err)
			store := metrix.NewCollectorStore()
			for i, cycle := range tc.cycles {
				attempt := variableAttempt(t, e, store, set, cycle)
				update := chartUpdate(attempt.Plan(), "net_net_eth0")
				require.NotNil(t, update, "cycle %d", i)
				assert.Equal(t, []UpdateVariableValue{{Name: "nic_speed", Value: 3e9}}, update.Variables, "cycle %d", i)
				require.NoError(t, attempt.Commit())
			}
		})
	}
}

func TestPlanChartVariablesFollowChartOwnership(t *testing.T) {
	// Two entries render the same chart ID; the variable attaches only when its
	// template owns the chart (the first entry wins a contested new chart).
	plain := netEntry("plain")
	withVar := netEntry("with_var", charttpl.ChartVariable{
		Name:     "limit",
		Selector: "speed_limit",
	})
	tests := map[string]struct {
		entries    []TemplateEntry
		wantValues []UpdateVariableValue
		wantNames  []string
		wantFacts  []variableFact
	}{
		"owner declares the variable": {
			entries:    []TemplateEntry{withVar, plain},
			wantValues: []UpdateVariableValue{{Name: "limit", Value: 5}},
			wantNames:  []string{"limit"},
			wantFacts: []variableFact{
				{decision: PlanRouteVariableResolved, name: "limit", chartID: "net_net_eth0"},
				{decision: PlanRouteVariableAccepted, name: "limit", chartID: "net_net_eth0"},
			},
		},
		"another template owns the chart": {
			entries: []TemplateEntry{plain, withVar},
			wantFacts: []variableFact{
				{decision: PlanRouteVariableResolved, name: "limit", chartID: "net_net_eth0"},
				{
					decision: PlanRouteVariableUnbound,
					reason:   PlanRouteReasonVariableChartOtherOwner,
					name:     "limit",
					chartID:  "net_net_eth0",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var facts []variableFact
			e, err := New(WithRuntimeStore(nil), WithPlanRouteDiagnosticObserver(collectVariableFacts(&facts)))
			require.NoError(t, err)
			set := testTemplateSet(t, tc.entries...)
			attempt := variableAttempt(t, e, metrix.NewCollectorStore(), set, []variableSeries{
				eth("received", 1),
				eth("speed_limit", 5),
			})
			update := chartUpdate(attempt.Plan(), "net_net_eth0")
			require.NotNil(t, update)
			assert.Equal(t, tc.wantValues, update.Variables)
			created := findCreateChartAction(attempt.Plan())
			require.NotNil(t, created)
			assert.Equal(t, tc.wantNames, created.Variables)
			assert.Equal(t, tc.wantFacts, sortedVariableFacts(facts))
		})
	}
}

func TestPlanChartVariablesRecreationResetsRemovedNames(t *testing.T) {
	// A template change that keeps the chart ID re-creates the chart without an
	// obsolete step, and the Agent keeps chart variables across a re-definition;
	// names the new template drops are preset to nan with the declared ones.
	limit := charttpl.ChartVariable{
		Name:     "limit",
		Selector: "speed_limit",
	}
	burst := charttpl.ChartVariable{
		Name:     "burst",
		Selector: "speed",
	}
	extra := charttpl.ChartVariable{
		Name:     "extra",
		Selector: "speed",
	}
	tests := map[string]struct {
		from, to TemplateEntry
		want     []string
		// wantLater is a later re-creation under the new template alone.
		wantLater []string
	}{
		"dropped variable is reset": {
			from:      netEntry("net", limit, burst),
			to:        netEntry("net", limit),
			want:      []string{"limit", "burst"},
			wantLater: []string{"limit"},
		},
		"all variables dropped": {
			from: netEntry("net", limit),
			to:   netEntry("net"),
			want: []string{"limit"},
		},
		"added variable needs no reset": {
			from:      netEntry("net", limit),
			to:        netEntry("net", limit, extra),
			want:      []string{"limit", "extra"},
			wantLater: []string{"limit", "extra"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New(WithRuntimeStore(nil))
			require.NoError(t, err)
			store := metrix.NewCollectorStore()
			series := []variableSeries{eth("received", 1), eth("speed", 2), eth("speed_limit", 3)}

			attempt := variableAttempt(t, e, store, testTemplateSet(t, tc.from), series)
			require.NotNil(t, findCreateChartAction(attempt.Plan()))
			require.NoError(t, attempt.Commit())

			next := testTemplateSet(t, tc.to)
			attempt = variableAttempt(t, e, store, next, series)
			created := findCreateChartAction(attempt.Plan())
			require.NotNil(t, created, "a changed entry re-creates its chart")
			assert.Equal(t, tc.want, created.Variables)
			assert.Nil(t, findRemoveChartAction(attempt.Plan()), "the chart is re-defined, not obsoleted")
			require.NoError(t, attempt.Commit())

			// Later re-creations under the same template preset only its own names.
			for range 6 {
				attempt = variableAttempt(t, e, store, next, nil)
				require.NoError(t, attempt.Commit())
			}
			attempt = variableAttempt(t, e, store, next, series)
			created = findCreateChartAction(attempt.Plan())
			require.NotNil(t, created, "the expired chart is created again")
			assert.Equal(t, tc.wantLater, created.Variables)
			require.NoError(t, attempt.Commit())
		})
	}
}

func TestTemplateTransitionComparesChartVariables(t *testing.T) {
	limit := charttpl.ChartVariable{
		Name:     "limit",
		Selector: "speed_limit",
	}
	scaled := charttpl.ChartVariable{
		Name:     "limit",
		Selector: "speed_limit",
		Options: &charttpl.VariableOptions{
			Divisor: 1000,
		},
	}
	tests := map[string]struct {
		next         TemplateEntry
		wantRecreate bool
	}{
		"identical entry from a separate set keeps the chart": {
			next: netEntry("net", limit),
		},
		"changed variable scale re-creates the chart": {
			next:         netEntry("net", scaled),
			wantRecreate: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, err := New(WithRuntimeStore(nil))
			require.NoError(t, err)
			store := metrix.NewCollectorStore()
			series := []variableSeries{eth("received", 1), eth("speed_limit", 3000)}
			attempt := variableAttempt(t, e, store, testTemplateSet(t, netEntry("net", limit)), series)
			require.NoError(t, attempt.Commit())

			attempt = variableAttempt(t, e, store, testTemplateSet(t, tc.next), series)
			assert.Equal(t, tc.wantRecreate, findCreateChartAction(attempt.Plan()) != nil)
			require.NoError(t, attempt.Commit())
		})
	}
}

func TestPlanChartVariableRouteDiagnostics(t *testing.T) {
	var facts []variableFact
	e, err := New(WithRuntimeStore(nil), WithPlanRouteDiagnosticObserver(collectVariableFacts(&facts)))
	require.NoError(t, err)
	set := testTemplateSet(
		t,
		netEntry("net", charttpl.ChartVariable{
			Name:     "limit",
			Selector: `speed_limit{kind="max"}`,
		}),
	)
	attempt := variableAttempt(t, e, metrix.NewCollectorStore(), set, []variableSeries{
		eth("received", 1),
		eth("speed_limit", 1, metrix.Label{
			Key:   "kind",
			Value: "min",
		}),
		{metric: "speed_limit", labels: []metrix.Label{{Key: "kind", Value: "max"}}, value: 2},
		{
			metric: "speed_limit",
			labels: []metrix.Label{{Key: "device", Value: "eth9"}, {Key: "kind", Value: "max"}},
			value:  3,
		},
	})
	assert.Empty(t, chartUpdate(attempt.Plan(), "net_net_eth0").Variables)
	assert.Equal(t, []variableFact{
		{decision: PlanRouteVariableSelectorRejected, name: "limit"},
		{decision: PlanRouteVariableIdentityRejected, name: "limit"},
		{decision: PlanRouteVariableResolved, name: "limit", chartID: "net_net_eth9"},
		{
			decision: PlanRouteVariableUnbound,
			reason:   PlanRouteReasonVariableChartNotUpdated,
			name:     "limit",
			chartID:  "net_net_eth9",
		},
	}, sortedVariableFacts(facts))
}

type variableFact struct {
	decision PlanRouteDecision
	reason   PlanRouteReason
	name     string
	chartID  string
}

func collectVariableFacts(out *[]variableFact) func(PlanRouteDiagnostic) {
	return func(fact PlanRouteDiagnostic) {
		if fact.VariableName == "" {
			return
		}
		*out = append(*out, variableFact{
			decision: fact.Decision,
			reason:   fact.Reason,
			name:     fact.VariableName,
			chartID:  fact.ChartID,
		})
	}
}

var variableDecisionOrder = []PlanRouteDecision{
	PlanRouteVariableSelectorRejected,
	PlanRouteVariableIdentityRejected,
	PlanRouteVariableResolved,
	PlanRouteVariableAccepted,
	PlanRouteVariableUnbound,
}

// sortedVariableFacts orders facts by decision stage, then chart, because the
// scan order of series is not part of the contract.
func sortedVariableFacts(facts []variableFact) []variableFact {
	out := slices.Clone(facts)
	slices.SortFunc(out, func(a, b variableFact) int {
		return cmp.Or(
			cmp.Compare(
				slices.Index(variableDecisionOrder, a.decision),
				slices.Index(variableDecisionOrder, b.decision),
			),
			cmp.Compare(a.chartID, b.chartID),
			cmp.Compare(a.name, b.name),
		)
	})
	return out
}
