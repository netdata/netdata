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
)

func TestDecisionIndexAcknowledgesSelectionAndFallback(t *testing.T) {
	var changes []DiscoveredChange
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(t, commands, func(
		change DiscoveredChange,
	) (jobmgr.WorkPlan, error) {
		changes = append(changes, change)
		return jobmgr.WorkPlan{}, nil
	})
	stock := decisionTestConfig(
		"job",
		confgroup.TypeStock,
		"stock",
	)
	user := decisionTestConfig(
		"job",
		confgroup.TypeUser,
		"user",
	)
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
		if err := index.Apply(
			context.Background(),
			[]*confgroup.Group{groups[name]},
		); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(changes) != 4 {
		t.Fatalf("changes=%d, want 4", len(changes))
	}
	if changes[0].Config.UID() != stock.UID() ||
		changes[1].Config.UID() != user.UID() ||
		changes[2].Config.UID() != stock.UID() ||
		!changes[3].Remove {
		t.Fatalf("changes=%+v", changes)
	}
	if census := index.Census(); census != (DecisionCensus{
		Revision: 4,
	}) {
		t.Fatalf("final census=%+v", census)
	}
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
	stock := decisionTestConfig(
		"job",
		confgroup.TypeStock,
		"stock",
	)
	user := decisionTestConfig(
		"job",
		confgroup.TypeUser,
		"user",
	)
	if err := index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "stock",
		Configs: []confgroup.Config{stock},
	}}); err != nil {
		t.Fatal(err)
	}
	commands.err = errors.New("acknowledgement failed")
	if err := index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}}); !errors.Is(err, commands.err) {
		t.Fatalf("error=%v", err)
	}
	acknowledged := index.acknowledged[stock.FullName()]
	if acknowledged.config.UID() != stock.UID() ||
		acknowledged.revision != 1 {
		t.Fatalf("acknowledged=%+v", acknowledged)
	}
	if census := index.Census(); census != (DecisionCensus{
		Sources: 2, Candidates: 2, Acknowledged: 1, Revision: 1,
	}) {
		t.Fatalf("failure census=%+v", census)
	}
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
			if err != nil {
				t.Fatal(err)
			}
			if err := index.Apply(
				context.Background(),
				[]*confgroup.Group{{
					Source:  "source",
					Configs: []confgroup.Config{test.config},
				}},
			); err != nil {
				t.Fatal(err)
			}
			if len(changes) != test.wantCalls {
				t.Fatalf(
					"calls=%d, want %d",
					len(changes),
					test.wantCalls,
				)
			}
			if len(changes) != 0 &&
				changes[0].Status != test.want {
				t.Fatalf(
					"status=%s, want %s",
					changes[0].Status,
					test.want,
				)
			}
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
	for ordinal := 0; ordinal < population; ordinal++ {
		source := fmt.Sprintf("source-%03d", ordinal)
		groups = append(groups, &confgroup.Group{
			Source: source,
			Configs: []confgroup.Config{
				decisionTestConfig(
					fmt.Sprintf("job-%03d", ordinal),
					confgroup.TypeStock,
					source,
				),
			},
		})
	}
	if err := index.Apply(context.Background(), groups); err != nil {
		t.Fatal(err)
	}
	if len(commands.requests) != population {
		t.Fatalf(
			"initial commands=%d, want %d",
			len(commands.requests),
			population,
		)
	}
	if err := index.Apply(
		context.Background(),
		[]*confgroup.Group{{
			Source: "source-000",
			Configs: []confgroup.Config{
				decisionTestConfig(
					"job-000",
					confgroup.TypeStock,
					"source-000",
				).Set("value", "changed"),
			},
		}},
	); err != nil {
		t.Fatal(err)
	}
	if len(commands.requests) != population+1 {
		t.Fatalf(
			"commands after one changed source=%d, want %d",
			len(commands.requests),
			population+1,
		)
	}
	if census := index.Census(); census != (DecisionCensus{
		Sources:      population,
		Candidates:   population,
		Acknowledged: population,
		Revision:     population + 1,
	}) {
		t.Fatalf("changed-source census=%+v", census)
	}
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
				for ordinal := 0; ordinal < population; ordinal++ {
					group.Configs = append(
						group.Configs,
						decisionTestConfig(
							fmt.Sprintf("job-%d", ordinal),
							confgroup.TypeStock,
							"source",
						),
					)
				}
				return []*confgroup.Group{group}
			},
			wantSources: 1,
		},
		"many independent sources": {
			batch: func() []*confgroup.Group {
				groups := make(
					[]*confgroup.Group,
					0,
					population,
				)
				for ordinal := 0; ordinal < population; ordinal++ {
					source := fmt.Sprintf("source-%d", ordinal)
					groups = append(groups, &confgroup.Group{
						Source: source,
						Configs: []confgroup.Config{
							decisionTestConfig(
								fmt.Sprintf("job-%d", ordinal),
								confgroup.TypeStock,
								source,
							),
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
			if err := index.Apply(
				context.Background(),
				test.batch(),
			); err != nil {
				t.Fatal(err)
			}
			if census := index.Census(); census != (DecisionCensus{
				Sources:      test.wantSources,
				Candidates:   population,
				Acknowledged: population,
				Revision:     population,
			}) {
				t.Fatalf("census=%+v", census)
			}
			if len(commands.requests) != population {
				t.Fatalf(
					"commands=%d, want %d",
					len(commands.requests),
					population,
				)
			}
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
		b.Fatal(err)
	}
	groups := [2][]*confgroup.Group{
		{{
			Source: "source",
			Configs: []confgroup.Config{
				decisionTestConfig(
					"job",
					confgroup.TypeStock,
					"source",
				).Set("value", 1),
			},
		}},
		{{
			Source: "source",
			Configs: []confgroup.Config{
				decisionTestConfig(
					"job",
					confgroup.TypeStock,
					"source",
				).Set("value", 2),
			},
		}},
	}
	ctx := context.Background()
	ordinal := 0
	b.ReportAllocs()
	for b.Loop() {
		if err := index.Apply(ctx, groups[ordinal&1]); err != nil {
			b.Fatal(err)
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
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func decisionTestConfig(
	name string,
	sourceType string,
	source string,
) confgroup.Config {
	return confgroup.Config{}.
		SetName(name).
		SetModule("module").
		SetProvider("test").
		SetSourceType(sourceType).
		SetSource(source)
}

type decisionTestCommands struct {
	err      error
	requests []jobmgr.Request
}

func (commands *decisionTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	commands.requests = append(commands.requests, request)
	return commands.err
}

type decisionBenchmarkCommands struct{}

func (decisionBenchmarkCommands) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}
