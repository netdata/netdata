// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type decisionTestCensus struct {
	sources      int
	candidates   int
	acknowledged int
	rejected     int
	pending      int
	revision     uint64
}

func decisionIndexCensus(index *DecisionIndex) decisionTestCensus {
	census := decisionTestCensus{
		sources:      len(index.sources),
		acknowledged: len(index.acknowledged),
		pending:      len(index.pendingRemovals),
		revision:     index.revision,
	}
	for _, candidates := range index.candidates {
		census.candidates += len(candidates)
	}
	for _, rejected := range index.candidateRejections {
		census.rejected += len(rejected)
	}
	return census
}

func TestDecisionIndexQuarantinesTypedProposalAndContinuesBatch(t *testing.T) {
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			if change.Config.Name() == "bad" {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("unsupported discovered config"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	bad := decisionTestConfig("bad", confgroup.TypeStock, "source")
	good := decisionTestConfig("good", confgroup.TypeStock, "source")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{bad, good},
	}}))
	require.Len(t, commands.requests, 1)
	require.Equal(t, good.FullName(), commands.requests[0].LaneKey)
	require.Equal(t, good.UID(), index.acknowledged[good.FullName()].UID())
	require.EqualValues(t, decisionTestCensus{
		sources:      1,
		candidates:   2,
		acknowledged: 1,
		rejected:     1,
		revision:     1,
	}, decisionIndexCensus(index))
}

func TestDecisionIndexQuarantinesCloneFailureAndContinuesBatch(t *testing.T) {
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
	)
	bad := decisionTestConfig("bad", confgroup.TypeStock, "source").
		Set("unsupported", make(chan struct{}))
	good := decisionTestConfig("good", confgroup.TypeStock, "source")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{bad, good},
	}}))
	require.Len(t, commands.requests, 1)
	require.Equal(t, good.FullName(), commands.requests[0].LaneKey)
	require.NotContains(t, index.pendingRemovals, bad.FullName())

	fixed := decisionTestConfig("bad", confgroup.TypeStock, "source")
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{fixed, good},
	}}))
	require.Len(t, commands.requests, 2)
	require.Equal(t, fixed.FullName(), commands.requests[1].LaneKey)
	require.NotContains(t, index.pendingRemovals, bad.FullName())
}

func TestDecisionIndexCloneFailureDoesNotSuppressValidSameNameCandidate(t *testing.T) {
	good := decisionTestConfig("job", confgroup.TypeStock, "good")
	bad := decisionTestConfig("job", confgroup.TypeUser, "bad").
		Set("unsupported", make(chan struct{}))
	groups := map[string]*confgroup.Group{
		"good": {Source: "good", Configs: []confgroup.Config{good}},
		"bad":  {Source: "bad", Configs: []confgroup.Config{bad}},
	}
	for _, order := range [][]string{{"good", "bad"}, {"bad", "good"}} {
		t.Run(order[0]+" before "+order[1], func(t *testing.T) {
			commands := &decisionTestCommands{}
			index := newDecisionTestIndex(
				t,
				commands,
				func(DiscoveredChange) (jobmgr.WorkPlan, error) {
					return jobmgr.WorkPlan{}, nil
				},
			)

			require.NoError(t, index.Apply(
				context.Background(),
				[]*confgroup.Group{groups[order[0]], groups[order[1]]},
			))
			require.Len(t, commands.requests, 1)
			require.Equal(t, good.FullName(), commands.requests[0].LaneKey)
			require.Equal(t, good.UID(), index.acknowledged[good.FullName()].UID())
		})
	}
}

func TestDecisionIndexRetainsAndRetriesRejectedRemoval(t *testing.T) {
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
	)
	config := decisionTestConfig("job", confgroup.TypeStock, "source")
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{config},
	}}))

	commands.err = jobmgr.RejectProposal(errors.New("temporary proposal rejection"))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "source"}}))
	require.Contains(t, index.acknowledged, config.FullName())
	require.EqualValues(t, decisionTestCensus{
		acknowledged: 1,
		pending:      1,
		revision:     2,
	}, decisionIndexCensus(index))

	commands.err = nil
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "tick"}}))
	require.NotContains(t, index.acknowledged, config.FullName())
	require.EqualValues(t, decisionTestCensus{revision: 3}, decisionIndexCensus(index))
}

func TestDecisionIndexAcknowledgesSelectionAndFallback(t *testing.T) {
	var changes []DiscoveredChange
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(t, commands, func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
		changes = append(changes, change)
		return jobmgr.WorkPlan{}, nil
	})
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")
	groups := map[string]*confgroup.Group{
		"stock":        {Source: "stock", Configs: []confgroup.Config{stock}},
		"user":         {Source: "user", Configs: []confgroup.Config{user}},
		"remove user":  {Source: "user"},
		"remove stock": {Source: "stock"},
	}
	for _, name := range []string{"stock", "user", "remove user", "remove stock"} {
		require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{groups[name]}))
	}
	require.EqualValues(t, 4, len(changes))
	require.False(t, changes[0].Config.UID() != stock.UID() ||
		changes[1].Config.UID() != user.UID() ||
		changes[2].Config.UID() != stock.UID() ||
		!changes[3].Remove)
	require.EqualValues(t, decisionTestCensus{
		revision: 4,
	}, decisionIndexCensus(index))
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

	err := index.Apply(context.Background(), []*confgroup.Group{{Source: "user", Configs: []confgroup.Config{user}}})
	require.ErrorIs(t, err, commands.err)

	acknowledged := index.acknowledged[stock.FullName()]
	require.Equal(t, stock.UID(), acknowledged.UID())
	require.EqualValues(t, decisionTestCensus{
		sources:      2,
		candidates:   2,
		acknowledged: 1,
		revision:     2,
	}, decisionIndexCensus(index))
}

func TestDecisionIndexRejectedHigherPrioritySelectionKeepsIncumbent(t *testing.T) {
	rejectUser := true
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			if rejectUser && change.Config.SourceType() == confgroup.TypeUser {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("invalid user config"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "stock",
		Configs: []confgroup.Config{stock},
	}}))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}}))
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())
	require.EqualValues(t, 1, decisionIndexCensus(index).rejected)

	rejectUser = false
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "tick"}}))
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())
	require.EqualValues(t, 1, decisionIndexCensus(index).rejected)

	user = decisionTestConfig("job", confgroup.TypeUser, "user").Set("changed", true)
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}}))
	require.Equal(t, user.UID(), index.acknowledged[user.FullName()].UID())
	require.Zero(t, decisionIndexCensus(index).rejected)

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "user"}}))
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())
}

func TestDecisionIndexRejectedSameSourceReplacementKeepsIncumbent(t *testing.T) {
	var changes []DiscoveredChange
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			changes = append(changes, change)
			if !change.Remove && change.Config.Get("revision") == "rejected" {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("invalid replacement"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	lower := decisionTestConfig("job", confgroup.TypeStock, "lower")
	original := decisionTestConfig("job", confgroup.TypeUser, "source").Set("revision", "original")
	replacement := decisionTestConfig("job", confgroup.TypeUser, "source").Set("revision", "rejected")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{
		{Source: "lower", Configs: []confgroup.Config{lower}},
		{Source: "source", Configs: []confgroup.Config{original}},
	}))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{replacement},
	}}))

	require.Len(t, changes, 2)
	require.False(t, changes[0].Remove)
	require.False(t, changes[1].Remove)
	require.Len(t, commands.requests, 1)
	require.Equal(t, original.UID(), index.acknowledged[original.FullName()].UID())

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "source"}}))
	require.Len(t, changes, 3)
	require.False(t, changes[2].Remove)
	require.Equal(t, lower.UID(), changes[2].Config.UID())
	require.Len(t, commands.requests, 2)
	require.Equal(t, lower.UID(), index.acknowledged[lower.FullName()].UID())
}

func TestDecisionIndexSubmittedSameSourceReplacementRejectionKeepsIncumbent(t *testing.T) {
	var changes []DiscoveredChange
	commands := &sequenceDecisionTestCommands{
		results: []error{
			nil,
			jobmgr.RejectProposal(errors.New("rejected during command preparation")),
		},
	}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			changes = append(changes, change)
			return jobmgr.WorkPlan{}, nil
		},
	)
	original := decisionTestConfig("job", confgroup.TypeUser, "source").Set("revision", "original")
	replacement := decisionTestConfig("job", confgroup.TypeUser, "source").Set("revision", "rejected")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{original},
	}}))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{replacement},
	}}))

	require.Len(t, changes, 2)
	require.False(t, changes[0].Remove)
	require.False(t, changes[1].Remove)
	require.Len(t, commands.requests, 2)
	require.Equal(t, original.UID(), index.acknowledged[original.FullName()].UID())
}

func TestDecisionIndexRejectedOtherSourceDoesNotMaskIncumbentSourceRemoval(t *testing.T) {
	var changes []DiscoveredChange
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			changes = append(changes, change)
			if !change.Remove && change.Config.SourceType() == confgroup.TypeUser {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("invalid user config"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "stock",
		Configs: []confgroup.Config{stock},
	}}))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}}))
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())
	require.Len(t, commands.requests, 1)

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "stock"}}))
	require.Len(t, changes, 3)
	require.True(t, changes[2].Remove)
	require.Len(t, commands.requests, 2)
	require.NotContains(t, index.acknowledged, stock.FullName())
}

func TestDecisionIndexRejectedReplacementDoesNotMaskHigherPrioritySource(t *testing.T) {
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			if change.Config.Get("revision") == "rejected" {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("invalid replacement"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock").Set("revision", "original")
	replacement := decisionTestConfig("job", confgroup.TypeStock, "stock").Set("revision", "rejected")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "stock",
		Configs: []confgroup.Config{stock},
	}}))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "stock",
		Configs: []confgroup.Config{replacement},
	}}))
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}}))
	require.Len(t, commands.requests, 2)
	require.Equal(t, user.UID(), index.acknowledged[user.FullName()].UID())
}

func TestDecisionIndexRejectedHigherPriorityFallsBackInSameBatch(t *testing.T) {
	var planned []string
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			planned = append(planned, change.Config.UID())
			if change.Config.SourceType() == confgroup.TypeUser && change.Config.Get("fixed") != true {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("invalid user config"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{
		{Source: "stock", Configs: []confgroup.Config{stock}},
		{Source: "user", Configs: []confgroup.Config{user}},
	}))
	require.Equal(t, []string{user.UID(), stock.UID()}, planned)
	require.Len(t, commands.requests, 1)
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{user},
	}}))
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "tick"}}))
	require.Equal(t, []string{user.UID(), stock.UID()}, planned)
	require.Len(t, commands.requests, 1)

	fixed := decisionTestConfig("job", confgroup.TypeUser, "user").Set("fixed", true)
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "user",
		Configs: []confgroup.Config{fixed},
	}}))
	require.Equal(t, []string{user.UID(), stock.UID(), fixed.UID()}, planned)
	require.Len(t, commands.requests, 2)
	require.Equal(t, fixed.UID(), index.acknowledged[fixed.FullName()].UID())
}

func TestDecisionIndexSubmittedRejectionFallsBackInSameBatch(t *testing.T) {
	var planned []string
	commands := &sequenceDecisionTestCommands{
		results: []error{
			jobmgr.RejectProposal(errors.New("rejected during command preparation")),
			nil,
		},
	}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			planned = append(planned, change.Config.UID())
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := decisionTestConfig("job", confgroup.TypeStock, "stock")
	user := decisionTestConfig("job", confgroup.TypeUser, "user")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{
		{Source: "stock", Configs: []confgroup.Config{stock}},
		{Source: "user", Configs: []confgroup.Config{user}},
	}))
	require.Equal(t, []string{user.UID(), stock.UID()}, planned)
	require.Len(t, commands.requests, 2)
	require.Equal(t, stock.UID(), index.acknowledged[stock.FullName()].UID())
}

func TestDecisionIndexSourceMetadataChangeClearsCandidateRejection(t *testing.T) {
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
			if change.Config.Provider() == "test" {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("rejected provider metadata"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	original := decisionTestConfig("job", confgroup.TypeUser, "source")
	changed := decisionTestConfig("job", confgroup.TypeUser, "source").SetProvider("updated")
	require.Equal(t, original.Hash(), changed.Hash())
	require.NotEqual(t, original.UID(), changed.UID())

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{original},
	}}))
	require.Empty(t, commands.requests)
	require.Empty(t, index.acknowledged)

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{changed},
	}}))
	require.Len(t, commands.requests, 1)
	require.Equal(t, changed.UID(), index.acknowledged[changed.FullName()].UID())
}

func TestDecisionIndexRemovedCandidateClearsRejection(t *testing.T) {
	reject := true
	commands := &decisionTestCommands{}
	index := newDecisionTestIndex(
		t,
		commands,
		func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			if reject {
				return jobmgr.WorkPlan{}, jobmgr.RejectProposal(errors.New("rejected candidate"))
			}
			return jobmgr.WorkPlan{}, nil
		},
	)
	config := decisionTestConfig("job", confgroup.TypeUser, "source")

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{config},
	}}))
	require.EqualValues(t, 1, decisionIndexCensus(index).rejected)

	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{Source: "source"}}))
	require.Zero(t, decisionIndexCensus(index).rejected)

	reject = false
	require.NoError(t, index.Apply(context.Background(), []*confgroup.Group{{
		Source:  "source",
		Configs: []confgroup.Config{config},
	}}))
	require.Len(t, commands.requests, 1)
	require.Equal(t, config.UID(), index.acknowledged[config.FullName()].UID())
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
				Plan: func(change DiscoveredChange) (jobmgr.WorkPlan, error) {
					changes = append(changes, change)
					return jobmgr.WorkPlan{}, nil
				},
			})
			require.NoError(t, err)

			require.NoError(t, index.Apply(
				context.Background(),
				[]*confgroup.Group{{Source: "source", Configs: []confgroup.Config{test.config}}},
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
				group := &confgroup.Group{
					Source: "source",
				}
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

func TestDecisionIndexReconcilesDifferentIdentitiesConcurrently(t *testing.T) {
	commands := newConcurrentDecisionTestCommands("module_job-a")
	index := newDecisionTestIndex(
		t,
		commands,
		func(DiscoveredChange) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
	)
	configs := []confgroup.Config{
		decisionTestConfig("job-a", confgroup.TypeStock, "source"),
		decisionTestConfig("job-b", confgroup.TypeStock, "source"),
	}

	done := make(chan error, 1)
	go func() {
		done <- index.Apply(context.Background(), []*confgroup.Group{{
			Source:  "source",
			Configs: configs,
		}})
	}()

	select {
	case <-commands.blocked:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "blocked identity was not submitted")
	}
	select {
	case <-commands.healthy:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "unrelated identity waited behind blocked identity")
	}
	close(commands.release)
	require.NoError(t, <-done)
	require.Equal(t, configs[0].UID(), index.acknowledged[configs[0].FullName()].UID())
	require.Equal(t, configs[1].UID(), index.acknowledged[configs[1].FullName()].UID())
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
			Source:  "source",
			Configs: []confgroup.Config{decisionTestConfig("job", confgroup.TypeStock, "source").Set("value", 1)},
		}},
		{{
			Source:  "source",
			Configs: []confgroup.Config{decisionTestConfig("job", confgroup.TypeStock, "source").Set("value", 2)},
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

func newDecisionTestIndex(t *testing.T, commands PreparedCommandPort, plan PlanDiscovered) *DecisionIndex {
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

func decisionTestConfig(name string, sourceType string, source string) confgroup.Config {
	return confgroup.Config{}.SetName(name).SetModule("module").SetProvider("test").SetSourceType(sourceType).
		SetSource(source)
}

type decisionTestCommands struct {
	mu       sync.Mutex
	err      error
	requests []jobmgr.Request
}

func (dtc *decisionTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	dtc.mu.Lock()
	defer dtc.mu.Unlock()
	dtc.requests = append(dtc.requests, request)
	return dtc.err
}

type sequenceDecisionTestCommands struct {
	mu       sync.Mutex
	results  []error
	requests []jobmgr.Request
}

func (sdtc *sequenceDecisionTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	sdtc.mu.Lock()
	defer sdtc.mu.Unlock()
	sdtc.requests = append(sdtc.requests, request)
	if len(sdtc.results) == 0 {
		return nil
	}
	result := sdtc.results[0]
	sdtc.results = sdtc.results[1:]
	return result
}

type decisionBenchmarkCommands struct{}

func (decisionBenchmarkCommands) SubmitPreparedAndWait(context.Context, jobmgr.Request, jobmgr.WorkPlan) error {
	return nil
}

type concurrentDecisionTestCommands struct {
	blockedID string
	blocked   chan struct{}
	healthy   chan struct{}
	release   chan struct{}
}

func newConcurrentDecisionTestCommands(blockedID string) *concurrentDecisionTestCommands {
	return &concurrentDecisionTestCommands{
		blockedID: blockedID,
		blocked:   make(chan struct{}),
		healthy:   make(chan struct{}),
		release:   make(chan struct{}),
	}
}

func (cdtc *concurrentDecisionTestCommands) SubmitPreparedAndWait(
	ctx context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	if request.LaneKey == cdtc.blockedID {
		close(cdtc.blocked)
		select {
		case <-cdtc.release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	close(cdtc.healthy)
	return nil
}
