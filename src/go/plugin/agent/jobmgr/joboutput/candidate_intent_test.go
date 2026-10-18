// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestCandidateFailedTrialsPreserveLogicalResourceIntent(t *testing.T) {
	for _, sourceType := range []string{confgroup.TypeStock, confgroup.TypeUser} {
		t.Run(sourceType, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			checked := make(chan string, 2)
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
				module := &collectorapi.MockCollectorV1{}
				module.CheckFunc = func(context.Context) error {
					checked <- module.Config.OptionStr
					return errors.New("endpoint unavailable")
				}
				return module
			}
			controller.modules["module"] = creator
			owner := &intentTestOwner{}
			installIntentTestOwner(controller, owner)
			commands := runtimeTestBindActivations(t, controller)
			configs := []confgroup.Config{
				acceptedActivationTestConfig(sourceType).Set("option_str", "first").Set("autodetection_retry", 0),
				acceptedActivationTestConfig(sourceType).Set("option_str", "second").Set("autodetection_retry", 0),
			}
			byOption := map[string]confgroup.Config{"first": configs[0], "second": configs[1]}
			inventory := DiscoveredJobInventory{
				FullName: configs[0].FullName(), Version: 1, AutoEnable: true,
				Sources: map[string]uint64{"source": 1},
				Candidates: []DiscoveredJobCandidate{
					{Config: configs[0], Source: "source", Incarnation: 1},
					{Config: configs[1], Source: "source", Incarnation: 1},
				},
			}
			plan, err := planInventoryTest(controller, inventory)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			var binding confgroup.Config
			for generation := uint64(1); generation <= 2; generation++ {
				call := commands.next(t, "internal/jobs/accepted-activation")
				config := byOption[<-checked]
				require.NotNil(t, config)
				require.Nil(t, applyActivationTestSubmission(t, call, nil, generation))
				if binding == nil {
					binding = config
				}
				owner.mu.Lock()
				ownsBinding := owner.enabled[jobConfigIdentity(binding)]
				ownsTrial := config.UID() != binding.UID() && owner.enabled[jobConfigIdentity(config)]
				count := len(owner.enabled)
				owner.mu.Unlock()
				require.True(t, ownsBinding, "logical job retains its resource demand")
				require.False(t, ownsTrial, "a failed trial must not acquire resource demand")
				require.Equal(t, 1, count)
				record, listed := graph.Lookup(config.FullName())
				if sourceType == confgroup.TypeStock {
					require.False(t, listed, "private recovery must not list failed stock")
				} else {
					require.True(t, listed)
					require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
					exposed, err := graphRecordConfig(record)
					require.NoError(t, err)
					require.Equal(t, binding, exposed)
				}
			}
			inventory.Version++
			inventory.Candidates = nil
			inventory.Sources = nil
			plan, err = planInventoryTest(controller, inventory)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			_, _, _, enabled := owner.snapshot()
			require.Zero(t, enabled, "source removal retires private and listed demand")
		})
	}
}

func TestSiblingIntentAdmissionRevalidatesCheckedStore(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	oldScope, newScope := &factoryTestAtomicScope{value: "old"}, &factoryTestAtomicScope{value: "new"}
	oldScope.current.Store(true)
	newScope.current.Store(true)
	var resolutions, checks, reservations atomic.Int32
	controller.factory.config.ConfigModules.config.Configs = testConfigResolver(t, testAtomicResolver(t), func([]string) (secretresolver.AtomicScope, error) {
		resolutions.Add(1)
		if oldScope.current.Load() {
			return oldScope, nil
		}
		return newScope, nil
	})
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		module := &runtimeTestCollector{factoryTestV2: &factoryTestV2{}, run: func(ctx context.Context, ready func()) error {
			ready()
			<-ctx.Done()
			return ctx.Err()
		}}
		module.check = func(context.Context) error {
			if checks.Add(1) == 1 {
				return errors.New("first endpoint unavailable")
			}
			return nil
		}
		return module
	})
	owner := &intentTestOwner{onPrepare: func() {
		if reservations.Add(1) == 2 {
			oldScope.current.Store(false)
		}
	}}
	installIntentTestOwner(controller, owner)
	commands := bindHandoffTestWorkers(t, controller)
	a := acceptedActivationTestConfig(confgroup.TypeUser).Set("option_str", "${store:vault:test:key}").Set("option_int", 1).Set("autodetection_retry", 0)
	b := acceptedActivationTestConfig(confgroup.TypeUser).Set("option_str", "${store:vault:test:key}").Set("option_int", 2).Set("autodetection_retry", 0)
	inventory := DiscoveredJobInventory{
		FullName: a.FullName(), Version: 1, AutoEnable: true, Sources: map[string]uint64{"source": 1},
		Candidates: []DiscoveredJobCandidate{{Config: a, Source: "source", Incarnation: 1}, {Config: b, Source: "source", Incarnation: 1}},
	}
	plan, err := planInventoryTest(controller, inventory)
	require.NoError(t, err)
	applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
	require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1))
	before, exists := graph.Lookup(a.FullName())
	require.True(t, exists)
	current := applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
	t.Cleanup(func() { stopRuntimeTestResource(t, current) })
	require.False(t, oldScope.current.Load(), "sibling reservation must invalidate the checked proof")
	require.Nil(t, current, "stale sibling must not install")
	after, _ := graph.Lookup(a.FullName())
	require.Equal(t, before, after, "stale sibling must not adopt its payload or health")
	prepared, committed, aborted, enabled := owner.snapshot()
	require.Equal(t, []int{2, 1, 1, 1}, []int{prepared, committed, aborted, enabled})
	require.EqualValues(t, 2, checks.Load())

	applyActivationTestSubmission(t, commands.next(t, "internal/jobs/candidate-resume"), nil, 0)
	require.Nil(t, applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3))
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 4)
	require.NotNil(t, current)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
	after, _ = graph.Lookup(a.FullName())
	require.Equal(t, dyncfg.StatusRunning.String(), after.Status)
	require.EqualValues(t, 3, checks.Load(), "stale checked stage must be rebuilt exactly once")
	require.EqualValues(t, 3, resolutions.Load(), "rebuild must resolve the current Store")
	prepared, committed, aborted, enabled = owner.snapshot()
	require.Equal(t, []int{3, 2, 1, 1}, []int{prepared, committed, aborted, enabled})
}
