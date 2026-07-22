// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type decisionTestCensus struct {
	sources      int
	candidates   int
	acknowledged int
	revision     uint64
}

func decisionIndexCensus(index *DecisionIndex) decisionTestCensus {
	census := decisionTestCensus{
		sources:      len(index.sources),
		acknowledged: len(index.acknowledged),
		revision:     index.revision,
	}
	for _, candidates := range index.candidates {
		census.candidates += len(candidates)
	}
	return census
}

func TestDecisionIndexAcknowledgesSelectionAndFallback(t *testing.T) {
	var changes []DiscoveredChange
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(t, commands, func(
		change DiscoveredChange,
	) (jobmgr.WorkPlan, error) {
		changes = append(changes, change)
		return jobmgr.WorkPlan{}, nil
	})
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")
	groups := map[string]*confgroup.Group{
		"stock": {
			Source:  "stock",
			Configs: []confgroup.Config{stock},
		},
		"user": {
			Source:  "user",
			Configs: []confgroup.Config{user},
		},
		"remove user": {
			Source: "user",
		},
		"remove stock": {
			Source: "stock",
		},
	}
	for _, name := range []string{
		"stock",
		"user",
		"remove user",
		"remove stock",
	} {
		require.NoError(t, index.Apply(
			context.Background(),
			[]*confgroup.Group{groups[name]},
		),
		)
	}
	require.EqualValues(t, 4, len(changes))
	require.False(t, changes[0].Config.UID() != stock.UID() ||
		changes[1].Config.UID() != user.UID() ||
		changes[2].Config.UID() != stock.UID() ||
		!changes[3].Remove)
	require.EqualValues(t, decisionTestCensus{revision: 4}, decisionIndexCensus(index))
}

func TestDecisionIndexFailureKeepsLastAcknowledgedSelection(t *testing.T) {
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "stock",
		Configs: []confgroup.Config{stock},
	}}),
	)

	commands.err = errors.New("acknowledgement failed")

	err := index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}})
	require.ErrorIs(t, err, commands.err)

	acknowledged := index.acknowledged[stock.FullName()]
	require.Equal(t, stock.UID(), acknowledged.UID())
	require.EqualValues(t, decisionTestCensus{
		sources: 2, candidates: 2, acknowledged: 1, revision: 1,
	}, decisionIndexCensus(index))
}

func TestDecisionIndexConfigurationPolicy(t *testing.T) {
	tests := map[string]struct {
		runJob    []string
		auto      bool
		config    confgroup.Config
		wantCalls int
		want      dyncfg.Status
	}{
		"all accepted": {
			config:    decisionTestConfig("job", confgroup.TypeStock, "stock"),
			wantCalls: 1,
			want:      dyncfg.StatusAccepted,
		},
		"all running": {
			auto:      true,
			config:    decisionTestConfig("job", confgroup.TypeStock, "stock"),
			wantCalls: 1,
			want:      dyncfg.StatusRunning,
		},
		"run-job included": {
			runJob:    []string{"job"},
			config:    decisionTestConfig("job", confgroup.TypeStock, "stock"),
			wantCalls: 1,
			want:      dyncfg.StatusAccepted,
		},
		"run-job excluded": {
			runJob: []string{"other"},
			config: decisionTestConfig("job", confgroup.TypeStock, "stock"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var changes []DiscoveredChange
			index, err := NewDecisionIndex(DecisionConfig{
				Generation: 1,
				RunJob:     test.runJob,
				AutoEnable: test.auto,
				Commands:   &decisionTestCommands{},
				Plan: func(
					change DiscoveredChange,
				) (jobmgr.WorkPlan, error) {
					changes = append(changes, change)
					return jobmgr.WorkPlan{}, nil
				},
			})
			require.NoError(t, err)

			require.NoError(t, index.Apply(
				context.Background(),
				[]*confgroup.Group{{
					Source:  "source",
					Configs: []confgroup.Config{test.config},
				}},
			),
			)

			require.EqualValues(t, test.wantCalls, len(changes))
			require.False(t, len(changes) != 0 && changes[0].Status != test.want)
		})
	}
}

func TestDecisionIndexReconcilesOnlyChangedSourceRecords(t *testing.T) {
	const population = 300
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
	)
	groups := make([]*confgroup.Group, 0, population)
	for ordinal := range population {
		source := fmt.Sprintf("source-%03d", ordinal)
		groups = append(groups, &confgroup.Group{
			Source: source,
			Configs: []confgroup.Config{
				decisionTestConfig(fmt.Sprintf("job-%03d", ordinal), confgroup.TypeStock, source),
			},
		})
	}

	require.NoError(t, index.Apply(context.Background(), groups))

	require.EqualValues(t, population, len(commands.requests))

	require.NoError(t, index.Apply(
		context.Background(),
		[]*confgroup.Group{{
			Source: "source-000",
			Configs: []confgroup.Config{
				decisionTestConfig("job-000", confgroup.TypeStock, "source-000").Set("value", "changed"),
			},
		}},
	),
	)

	require.EqualValues(t, population+1, len(commands.requests))
	require.EqualValues(t, decisionTestCensus{
		sources:      population,
		candidates:   population,
		acknowledged: population,
		revision:     population + 1,
	}, decisionIndexCensus(index))
}

func TestDecisionIndexHasNoFixedPopulationCeiling(t *testing.T) {
	const population = 300
	tests := map[string]struct {
		batch       func() []*confgroup.Group
		wantSources int
	}{
		"many configs from one source": {
			batch: func() []*confgroup.Group {
				group := &confgroup.Group{Source: "source"}
				for ordinal := range population {
					group.Configs = append(
						group.Configs,
						decisionTestConfig(fmt.Sprintf("job-%d", ordinal), confgroup.TypeStock, "source"),
					)
				}
				return []*confgroup.Group{group}
			},
			wantSources: 1,
		},
		"many independent sources": {
			batch: func() []*confgroup.Group {
				groups := make([]*confgroup.Group, 0, population)
				for ordinal := range population {
					source := fmt.Sprintf("source-%d", ordinal)
					groups = append(groups, &confgroup.Group{
						Source: source,
						Configs: []confgroup.Config{
							decisionTestConfig(fmt.Sprintf("job-%d", ordinal), confgroup.TypeStock, source),
						},
					})
				}
				return groups
			},
			wantSources: population,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			commands := &decisionTestCommands{}
			index := newDecisionTestIndex(
				t,
				commands,
				func(DiscoveredChange) (jobmgr.WorkPlan, error) {
					return jobmgr.WorkPlan{}, nil
				},
			)

			require.NoError(t, index.Apply(context.Background(), test.batch()))

			require.EqualValues(t, decisionTestCensus{
				sources:      test.wantSources,
				candidates:   population,
				acknowledged: population,
				revision:     population,
			}, decisionIndexCensus(index))
			require.EqualValues(t, population, len(commands.requests))
		})
	}
}

func BenchmarkBDecisionIndexApply(b *testing.B) {
	index, err := NewDecisionIndex(DecisionConfig{
		Generation: 1,
		Commands:   decisionBenchmarkCommands{},
		Plan: func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
	})
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	groups := [2][]*confgroup.Group{
		{{
			Source: "source",
			Configs: []confgroup.Config{
				decisionTestConfig("job", confgroup.TypeStock, "source").Set("value", 1),
			},
		}},
		{{
			Source: "source",
			Configs: []confgroup.Config{
				decisionTestConfig("job", confgroup.TypeStock, "source").Set("value", 2),
			},
		}},
	}
	ctx := context.Background()
	ordinal := 0
	b.ReportAllocs()
	for b.Loop() {
		if err := index.Apply(ctx, groups[ordinal&1]); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		ordinal++
	}
}

func newDecisionTestIndex(
	t *testing.T,
	commands PreparedCommandPort,
	plan PlanDiscovered,
) *DecisionIndex {
	t.Helper()
	index, err := NewDecisionIndex(DecisionConfig{
		Generation: 1,
		AutoEnable: true,
		Commands:   commands,
		Plan:       plan,
	})
	require.NoError(t, err)
	return index
}

func decisionTestConfig(
	name string,
	sourceType string,
	source string,
) confgroup.Config {
	return confgroup.Config{}.SetName(name).SetModule("module").SetProvider("test").SetSourceType(sourceType).
		SetSource(source)
}

type decisionTestCommands struct {
	err      error
	requests []jobmgr.Request
}

func (dtc *decisionTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	dtc.requests = append(dtc.requests, request)
	return dtc.err
}

type decisionBenchmarkCommands struct{}

func (decisionBenchmarkCommands) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}
