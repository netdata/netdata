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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInventoryIndexCompleteObservations(t *testing.T) {
	var planned []SourceInventory
	index := newInventoryTestIndex(
		t,
		inventoryTestCommands(nil),
		func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
			planned = append(planned, inventory)
			return jobmgr.WorkPlan{}, nil
		},
	)
	stock := inventoryTestConfig("job", confgroup.TypeStock, "stock")
	user := inventoryTestConfig("job", confgroup.TypeUser, "user")
	for _, group := range []*confgroup.Group{
		{Source: "stock", Configs: []confgroup.Config{stock}},
		{Source: "user", Configs: []confgroup.Config{user}},
		{Source: "user", Configs: []confgroup.Config{user}},
		{Source: "stock", Configs: []confgroup.Config{stock}},
		{Source: "user"}, {Source: "stock"},
	} {
		require.NoError(t, index.Apply(t.Context(), []*confgroup.Group{group}))
	}
	require.Len(t, planned, 4)
	assert.Equal(
		t,
		[]uint64{1, 2, 3, 4},
		[]uint64{planned[0].Version, planned[1].Version, planned[2].Version, planned[3].Version},
	)
	assert.Equal(
		t,
		[]confgroup.Config{stock, user},
		[]confgroup.Config{planned[1].Candidates[0].Config, planned[1].Candidates[1].Config},
	)
	assert.Equal(t, planned[0].Candidates, planned[2].Candidates)
	assert.Equal(t, planned[0].Sources, planned[2].Sources)
	assert.Empty(t, planned[3].Candidates)
	assert.Empty(t, planned[3].Sources)
	for _, inventory := range planned {
		assert.Equal(t, stock.FullName(), inventory.FullName)
		assert.True(t, inventory.AutoEnable)
		for _, candidate := range inventory.Candidates {
			assert.NotZero(t, candidate.Incarnation)
			assert.NotZero(t, inventory.Sources[candidate.Source])
		}
	}
}

func TestInventoryIndexObservedRemovalAndReaddition(t *testing.T) {
	var planned []SourceInventory
	index := newInventoryTestIndex(
		t,
		inventoryTestCommands(nil),
		func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
			planned = append(planned, inventory)
			return jobmgr.WorkPlan{}, nil
		},
	)
	config := inventoryTestConfig("job", confgroup.TypeUser, "source")
	group := &confgroup.Group{
		Source:  "source",
		Configs: []confgroup.Config{config},
	}
	require.NoError(t, index.Apply(t.Context(), []*confgroup.Group{group, {Source: "source"}, group}))
	require.Len(t, planned, 3)
	assert.Equal(t, uint64(2), planned[1].Version)
	assert.Empty(t, planned[1].Candidates)
	assert.Empty(t, planned[1].Sources)
	assert.Equal(t, planned[0].Candidates[0].Config, planned[2].Candidates[0].Config)
	assert.Greater(t, planned[2].Candidates[0].Incarnation, planned[0].Candidates[0].Incarnation)
	assert.Greater(t, planned[2].Sources["source"], planned[0].Sources["source"])
}

func TestInventoryIndexCandidateRevisions(t *testing.T) {
	tests := map[string]func(confgroup.Config) confgroup.Config{
		"config":                   func(c confgroup.Config) confgroup.Config { return c.Set("value", 2) },
		"provider":                 func(c confgroup.Config) confgroup.Config { return c.SetProvider("new") },
		"trust":                    func(c confgroup.Config) confgroup.Config { return c.SetTrustDiscoveredTargets(true) },
		"untrusted pipeline owner": func(c confgroup.Config) confgroup.Config { return c.SetDiscoveryPipelineID("new") },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			var planned []SourceInventory
			index := newInventoryTestIndex(
				t,
				inventoryTestCommands(nil),
				func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
					planned = append(planned, inventory)
					return jobmgr.WorkPlan{}, nil
				},
			)
			original := inventoryTestConfig("job", confgroup.TypeDiscovered, "source")
			require.NoError(
				t,
				index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{original}}}),
			)
			updated, err := original.Clone()
			require.NoError(t, err)
			updated = change(updated)
			for range 2 {
				require.NoError(
					t,
					index.Apply(
						t.Context(),
						[]*confgroup.Group{{Source: "source", Configs: []confgroup.Config{updated}}},
					),
				)
			}
			require.Len(t, planned, 2)
			assert.Greater(t, planned[1].Candidates[0].Incarnation, planned[0].Candidates[0].Incarnation)
			assert.Equal(t, planned[0].Sources, planned[1].Sources)
			assert.Equal(t, updated, planned[1].Candidates[0].Config)
		})
	}
}

func TestInventoryIndexMalformedReplacementGranularity(t *testing.T) {
	var planned []SourceInventory
	index := newInventoryTestIndex(
		t,
		inventoryTestCommands(nil),
		func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
			planned = append(planned, inventory)
			return jobmgr.WorkPlan{}, nil
		},
	)
	old := inventoryTestConfig("job", confgroup.TypeUser, "source")
	other := inventoryTestConfig("other", confgroup.TypeUser, "source")
	require.NoError(
		t,
		index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{old, other}}}),
	)
	before := planned[0]
	bad, err := old.Clone()
	require.NoError(t, err)
	bad.Set("invalid", make(chan struct{}))
	require.NoError(
		t,
		index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{bad}}}),
	)
	require.Len(t, planned, 3)
	assert.Equal(t, other.FullName(), planned[2].FullName)
	assert.Empty(t, planned[2].Candidates, "unrelated identity removal remains authoritative")
	good := inventoryTestConfig("job", confgroup.TypeUser, "source").Set("replacement", true)
	require.NoError(
		t,
		index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{bad, good}}}),
	)
	require.Len(t, planned, 4)
	assert.Equal(t, good, planned[3].Candidates[0].Config)
	assert.Equal(t, before.Sources, planned[3].Sources)
}

func TestInventoryIndexMalformedNewSourcePresence(t *testing.T) {
	var planned []SourceInventory
	index := newInventoryTestIndex(
		t,
		inventoryTestCommands(nil),
		func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
			planned = append(planned, inventory)
			return jobmgr.WorkPlan{}, nil
		},
	)
	bad := inventoryTestConfig("job", confgroup.TypeUser, "source").Set("invalid", make(chan struct{}))
	group := &confgroup.Group{
		Source:  "source",
		Configs: []confgroup.Config{bad},
	}
	require.NoError(t, index.Apply(t.Context(), []*confgroup.Group{group, group, {Source: "source"}}))
	require.Len(t, planned, 2)
	assert.Empty(t, planned[0].Candidates)
	assert.NotZero(t, planned[0].Sources["source"])
	assert.Empty(t, planned[1].Sources)
}

func TestInventoryIndexDeliveryFailureRetainsOrderedSnapshots(t *testing.T) {
	tests := map[string]struct {
		err   error
		fatal bool
	}{
		"retired":            {err: jobmgr.ErrProcessAttemptRetired},
		"stopped":            {err: fmt.Errorf("prepare: %w", jobmgr.ErrProcessAttemptStopped)},
		"unexpected":         {err: errors.New("delivery failed"), fatal: true},
		"proposal rejection": {err: jobmgr.RejectProposal(errors.New("invalid inventory")), fatal: true},
		"mixed retirement": {
			err:   errors.Join(jobmgr.ErrProcessAttemptRetired, errors.New("delivery failed")),
			fatal: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var planned []SourceInventory
			var requests []jobmgr.Request
			fail := false
			commands := inventoryTestCommands(func(_ context.Context, request jobmgr.Request, _ jobmgr.WorkPlan) error {
				requests = append(requests, request)
				if fail {
					return test.err
				}
				return nil
			})
			index := newInventoryTestIndex(t, commands, func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
				planned = append(planned, inventory)
				return jobmgr.WorkPlan{}, nil
			})
			config := inventoryTestConfig("job", confgroup.TypeUser, "source")
			group := &confgroup.Group{
				Source:  "source",
				Configs: []confgroup.Config{config},
			}
			require.NoError(t, index.Apply(t.Context(), []*confgroup.Group{group}))
			fail = true
			err := index.Apply(t.Context(), []*confgroup.Group{{Source: "source"}})
			if test.fatal {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			fail = false
			require.NoError(t, index.Apply(t.Context(), []*confgroup.Group{group}))
			require.Len(t, planned, 4)
			assert.Equal(t, planned[1], planned[2], "failed tombstone replays before re-add")
			assert.NotEqual(t, requests[1].UID, requests[2].UID, "a replay has a fresh transport identity")
			require.Equal(t, requests[1].LaneKey, requests[2].LaneKey)
			assert.Equal(t, uint64(3), planned[3].Version)
			assert.Greater(t, planned[3].Candidates[0].Incarnation, planned[0].Candidates[0].Incarnation)
		})
	}
}

func TestInventoryIndexPlannerFailurePreservesPayload(t *testing.T) {
	failure := errors.New("planner failed")
	var planned []SourceInventory
	var original SourceInventory
	fail := true
	index := newInventoryTestIndex(
		t,
		inventoryTestCommands(nil),
		func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
			if fail {
				var err error
				original, err = cloneInventory(inventory)
				require.NoError(t, err)
				inventory.Candidates[0].Config.Set("mutated", true)
				inventory.Sources["source"] = 999
				return jobmgr.WorkPlan{}, failure
			}
			planned = append(planned, inventory)
			return jobmgr.WorkPlan{}, nil
		},
	)
	config := inventoryTestConfig("job", confgroup.TypeUser, "source")
	require.ErrorIs(
		t,
		index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{config}}}),
		failure,
	)
	config.Set("caller mutation", true)
	fail = false
	require.NoError(t, index.Apply(t.Context(), []*confgroup.Group{}))
	require.Equal(t, []SourceInventory{original}, planned)
}

func TestInventoryIndexDifferentNamesDeliverIndependently(t *testing.T) {
	blocked := make(chan struct{})
	healthy := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var mu sync.Mutex
	var requests []jobmgr.Request
	healthyCount := 0
	commands := inventoryTestCommands(func(ctx context.Context, request jobmgr.Request, _ jobmgr.WorkPlan) error {
		mu.Lock()
		requests = append(requests, request)
		if request.LaneKey == "module_b" {
			healthyCount++
		}
		count := healthyCount
		mu.Unlock()
		if request.LaneKey == "module_a" {
			close(blocked)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		} else if request.LaneKey == "module_b" && count == 2 {
			close(healthy)
		}
		return nil
	})
	index := newInventoryTestIndex(
		t,
		commands,
		func(SourceInventory) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
	)
	done := make(chan error, 1)
	go func() {
		done <- index.Apply(t.Context(), []*confgroup.Group{
			{Source: "source", Configs: []confgroup.Config{inventoryTestConfig("a", confgroup.TypeUser, "source"), inventoryTestConfig("b", confgroup.TypeUser, "source")}},
			{Source: "other", Configs: []confgroup.Config{inventoryTestConfig("b", confgroup.TypeStock, "other")}},
		})
	}()
	for _, signal := range []<-chan struct{}{blocked, healthy} {
		select {
		case <-signal:
		case <-time.After(5 * time.Second):
			t.Fatal("one name blocked another name's delivery")
		}
	}
	close(release)
	require.NoError(t, <-done)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 3)
}

func TestInventoryIndexConfiguration(t *testing.T) {
	tests := map[string]struct {
		runJob     []string
		autoEnable bool
		want       int
	}{
		"automatic": {
			autoEnable: true,
			want:       1,
		}, "passive": {want: 1}, "matching filter": {runJob: []string{"job"}, want: 1}, "excluded filter": {runJob: []string{"other"}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var planned []SourceInventory
			index, err := NewInventoryIndex(
				InventoryConfig{
					Generation: 1,
					RunJob:     test.runJob,
					AutoEnable: test.autoEnable,
					Commands:   inventoryTestCommands(nil),
					Plan: func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
						planned = append(planned, inventory)
						return jobmgr.WorkPlan{}, nil
					},
				},
			)
			require.NoError(t, err)
			require.NoError(
				t,
				index.Apply(
					t.Context(),
					[]*confgroup.Group{
						{
							Source:  "source",
							Configs: []confgroup.Config{nil, inventoryTestConfig("job", confgroup.TypeUser, "source")},
						},
					},
				),
			)
			require.Len(t, planned, test.want)
			if test.want > 0 {
				assert.Equal(t, test.autoEnable, planned[0].AutoEnable)
			}
		})
	}
}

func newInventoryTestIndex(t *testing.T, commands PreparedCommandPort, plan PlanInventory) *InventoryIndex {
	t.Helper()
	index, err := NewInventoryIndex(InventoryConfig{
		Generation: 1,
		AutoEnable: true,
		Commands:   commands,
		Plan:       plan,
	})
	require.NoError(t, err)
	return index
}
func inventoryTestConfig(name, sourceType, source string) confgroup.Config {
	return confgroup.Config{}.SetName(name).
		SetModule("module").
		SetProvider("test").
		SetSourceType(sourceType).
		SetSource(source)
}

type inventoryTestCommands func(context.Context, jobmgr.Request, jobmgr.WorkPlan) error

func (commands inventoryTestCommands) SubmitPreparedAndWait(
	ctx context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	if commands != nil {
		return commands(ctx, request, plan)
	}
	return nil
}

func TestInventoryIndexTransportIdentityAcceptsUnpublishableConfigNames(t *testing.T) {
	var mu sync.Mutex
	var delivered []jobmgr.Request
	var observed []string
	commands := inventoryTestCommands(func(_ context.Context, request jobmgr.Request, _ jobmgr.WorkPlan) error {
		if err := request.Validate(); err != nil {
			return err
		}
		mu.Lock()
		delivered = append(delivered, request)
		mu.Unlock()
		return nil
	})
	index := newInventoryTestIndex(t, commands, func(inventory SourceInventory) (jobmgr.WorkPlan, error) {
		observed = append(observed, inventory.FullName)
		return jobmgr.WorkPlan{}, nil
	})
	bad := inventoryTestConfig("bad=name", confgroup.TypeUser, "source")
	good := inventoryTestConfig("healthy", confgroup.TypeUser, "source")
	require.NoError(
		t,
		index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{bad, good}}}),
	)
	require.ElementsMatch(
		t,
		[]string{bad.FullName(), good.FullName()},
		observed,
		"controller planning receives invalid metadata for quarantine",
	)
	require.Len(t, delivered, 2, "invalid metadata does not prevent source sibling delivery")
	require.NotEqual(t, delivered[0].UID, delivered[1].UID)
	for _, request := range delivered {
		require.NotContains(t, request.UID, request.LaneKey)
	}
}

func TestInventoryIndexDeliverySequenceOverflowFailsClosed(t *testing.T) {
	var calls int
	index := newInventoryTestIndex(
		t,
		inventoryTestCommands(func(context.Context, jobmgr.Request, jobmgr.WorkPlan) error { calls++; return nil }),
		func(SourceInventory) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
	)
	index.deliverySequence = ^uint64(0)
	config := inventoryTestConfig("job", confgroup.TypeUser, "source")
	err := index.Apply(t.Context(), []*confgroup.Group{{Source: "source", Configs: []confgroup.Config{config}}})
	require.ErrorContains(t, err, "delivery sequence wrapped")
	require.Zero(t, calls)
	require.Len(t, index.pending[config.FullName()], 1, "overflow cannot acknowledge the observation")
}
