// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestConfigIntentRejectsMalformedDiscovery(t *testing.T) {
	for name, fixture := range map[string]struct {
		creator collectorapi.Creator
		field   string
	}{
		"lighthouse": {lighthouse.Creator(lighthouse.Dependencies{}), "url"},
		"journey":    {journey.Creator(journey.Dependencies{}), "script"},
	} {
		for _, readiness := range []string{"available", "missing", "quarantined", "deadline"} {
			t.Run(name+"/"+readiness, func(t *testing.T) {
				controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
				owner := &intentTestOwner{}
				creator := fixture.creator
				creator.JobConfigIntent = owner
				creator.ActivationDependencies = intentAdmissionDependencies(readiness == "missing")
				controller.modules["module"] = creator
				if readiness == "quarantined" {
					injectAdoptionAttemptFailure(controller, injectJobQuarantined)
				}
				if readiness == "deadline" {
					injectAdoptionAttemptFailure(controller, injectJobDeadline)
				}
				config := acceptedActivationTestConfig(
					confgroup.TypeUser,
				).Set(fixture.field, map[string]any{"bad": "shape"})
				permit, _ := issueTestJobPermit(t, config.FullName(), 1)
				prepared, err := prepareDiscoveryTestFixture(t, controller, t.Context(), discoveryTestFixture{
					Config: config,
					Status: dyncfg.StatusRunning,
				}, nil, lifecycle.ResourceTransactionScope{
					ID:        config.FullName(),
					Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1},
				}, permit)
				require.NoError(t, err)
				_, err = prepared.Apply(t.Context())
				require.NoError(t, err)
				_, exists := graph.Lookup(config.FullName())
				require.False(t, exists)
				preparedCount, committed, aborted, enabled := owner.snapshot()
				require.Equal(t, []int{0, 0, 0, 0}, []int{preparedCount, committed, aborted, enabled})
			})
		}
	}
}

func TestConfigIntentRejectsMalformedPassiveEnable(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "missing"}[missing], func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			owner := &intentTestOwner{}
			creator := lighthouse.Creator(lighthouse.Dependencies{})
			creator.JobConfigIntent = owner
			creator.ActivationDependencies = intentAdmissionDependencies(missing)
			controller.modules["module"] = creator
			config := acceptedActivationTestConfig(confgroup.TypeUser).Set("url", map[string]any{"bad": "shape"})
			plan, err := planSourceTestConfig(controller, config, false, false)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			before, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			transaction, err := prepareIntentTestEnable(t, controller, config)
			require.NoError(t, err)
			applied, err := transaction.Apply(t.Context())
			require.NoError(t, err)
			require.Equal(t, 400, applied.ResultStatus())
			after, _ := graph.Lookup(config.FullName())
			require.Equal(t, before, after)
			require.False(t, controller.ActivationEnabled(config.FullName()))
			prepared, committed, aborted, enabled := owner.snapshot()
			require.Equal(t, []int{0, 0, 0, 0}, []int{prepared, committed, aborted, enabled})
		})
	}
}

func TestConfigIntentInvalidDiscoveredReplacementPreservesDemand(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	owner := &intentTestOwner{}
	creator := lighthouse.Creator(lighthouse.Dependencies{})
	creator.JobConfigIntent = owner
	creator.ActivationDependencies = intentAdmissionDependencies(true)
	controller.modules["module"] = creator
	config := acceptedActivationTestConfig(confgroup.TypeUser).Set("url", "https://example.test")
	permit, _ := issueTestJobPermit(t, config.FullName(), 1)
	prepared, err := prepareDiscoveryTestFixture(t, controller, t.Context(), discoveryTestFixture{
		Config: config,
		Status: dyncfg.StatusRunning,
	}, nil, lifecycle.ResourceTransactionScope{
		ID:        config.FullName(),
		Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1},
	}, permit)
	require.NoError(t, err)
	_, err = prepared.Apply(t.Context())
	require.NoError(t, err)
	before, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	replacement, err := config.Clone()
	require.NoError(t, err)
	replacement.Set("url", map[string]any{"bad": "shape"})
	permit, _ = issueTestJobPermit(t, config.FullName(), 2)
	prepared, err = prepareDiscoveryTestFixture(t, controller, t.Context(), discoveryTestFixture{
		Config: replacement,
		Status: dyncfg.StatusRunning,
	}, nil, lifecycle.ResourceTransactionScope{
		ID:        config.FullName(),
		Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 2},
	}, permit)
	require.NoError(t, err)
	_, err = prepared.Apply(t.Context())
	require.NoError(t, err)
	after, _ := graph.Lookup(config.FullName())
	require.Equal(t, before, after)
	preparedCount, committed, aborted, enabled := owner.snapshot()
	require.Equal(t, []int{1, 1, 0, 1}, []int{preparedCount, committed, aborted, enabled})
}

func intentAdmissionDependencies(missing bool) func(map[string]any) ([]collectorapi.ActivationDependency, error) {
	return func(map[string]any) ([]collectorapi.ActivationDependency, error) {
		return []collectorapi.ActivationDependency{{Kind: "runtime", Name: "browser", Ready: func() error {
			if missing {
				return errors.New("runtime missing")
			}
			return nil
		}}}, nil
	}
}

func TestConfigIntentAdmissionWaitsForExactValidationAttempt(t *testing.T) {
	for name, cut := range map[string]bool{"busy": false, "deadline": true} {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			owner := &intentTestOwner{}
			creator := lighthouse.Creator(lighthouse.Dependencies{})
			creator.JobConfigIntent = owner
			creator.ActivationDependencies = intentAdmissionDependencies(true)
			controller.modules["module"] = creator
			config := acceptedActivationTestConfig(confgroup.TypeUser).Set("url", "https://example.test")
			authority := controller.factory.config.Attempts
			identity := jobTestAttemptIdentity(configOperationValidateIntent, config)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			attempt, err := authority.StartProcessAttempt(t.Context(), jobmgr.ProcessAttemptPlan{
				Identity: identity,
				Target:   controller.factory.config.Epoch,
				Work:     func(context.Context, jobmgr.ProcessAttemptAdmission) error { close(entered); <-release; return nil },
			})
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("validation attempt did not start")
			}
			if cut {
				require.True(t, authority.CutProcessAttempt(identity, jobmgr.ErrProcessAttemptDeadline))
				require.ErrorIs(t, attempt.Await(t.Context()), jobmgr.ErrProcessAttemptDeadline)
			}
			physicalRelease, ok := authority.ProcessAttemptReleased(identity)
			require.True(t, ok)
			commands := runtimeTestBindActivations(t, controller)
			t.Cleanup(func() {
				unblock()
				if cut {
					require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptDeadline)
				} else {
					require.NoError(t, attempt.Await(context.Background()))
				}
			})
			plan, err := planSourceTestConfig(controller, config, true, false)
			require.NoError(t, err)
			applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
			applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1)
			_, exists := graph.Lookup(config.FullName())
			require.False(t, exists, "validation wait must not adopt unproven configuration")
			prepared, committed, _, enabled := owner.snapshot()
			require.Equal(t, []int{0, 0, 0}, []int{prepared, committed, enabled})
			controller.scheduler.accepted.mu.Lock()
			entry := controller.scheduler.accepted.entries[config.FullName()]
			var waiting bool
			var waitingRelease <-chan struct{}
			if entry != nil {
				waiting, waitingRelease = entry.waiting, entry.release
			}
			controller.scheduler.accepted.mu.Unlock()
			require.True(t, waiting)
			require.Equal(t, physicalRelease, waitingRelease, "wait for raw validation, not candidate execution")
			controller.NotifyDependencyChanged("runtime", "browser")
			select {
			case <-commands.queue:
				t.Fatal("candidate advanced before physical validation release")
			case <-time.After(20 * time.Millisecond):
			}
			prepared, committed, _, enabled = owner.snapshot()
			require.Equal(t, []int{0, 0, 0}, []int{prepared, committed, enabled})
			unblock()
			select {
			case <-physicalRelease:
			case <-time.After(time.Second):
				t.Fatal("validation did not physically release")
			}
			applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			prepared, committed, _, enabled = owner.snapshot()
			require.Equal(t, []int{1, 1, 1}, []int{prepared, committed, enabled})
		})
	}
}

type intentAdmissionQuarantinedAuthority struct {
	jobmgr.ProcessAttemptAuthority
	identity jobmgr.ProcessAttemptIdentity
}

func (a intentAdmissionQuarantinedAuthority) StartProcessAttempt(ctx context.Context, plan jobmgr.ProcessAttemptPlan) (jobmgr.ProcessAttempt, error) {
	if plan.Identity == a.identity {
		return nil, jobmgr.ErrProcessAttemptQuarantined
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}

func TestConfigIntentAdmissionQuarantineRejectsWithoutAcquisition(t *testing.T) {
	for _, route := range []string{"discovery", "enable"} {
		t.Run(route, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			owner := &intentTestOwner{}
			creator := lighthouse.Creator(lighthouse.Dependencies{})
			creator.JobConfigIntent = owner
			creator.ActivationDependencies = intentAdmissionDependencies(true)
			controller.modules["module"] = creator
			config := acceptedActivationTestConfig(confgroup.TypeUser).Set("url", "https://example.test")
			controller.factory.config.Attempts = intentAdmissionQuarantinedAuthority{
				ProcessAttemptAuthority: controller.factory.config.Attempts,
				identity:                jobTestAttemptIdentity(configOperationValidateIntent, config),
			}
			if route == "enable" {
				plan, err := planSourceTestConfig(controller, config, false, false)
				require.NoError(t, err)
				applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
				before, exists := graph.Lookup(config.FullName())
				require.True(t, exists)
				prepared, err := prepareIntentTestEnable(t, controller, config)
				require.NoError(t, err)
				applied, err := prepared.Apply(t.Context())
				require.NoError(t, err)
				require.Equal(t, 503, applied.ResultStatus())
				require.False(t, controller.ActivationEnabled(config.FullName()))
				after, _ := graph.Lookup(config.FullName())
				require.Equal(t, before, after)
			} else {
				commands := runtimeTestBindActivations(t, controller)
				plan, err := planSourceTestConfig(controller, config, true, false)
				require.NoError(t, err)
				applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
				applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1)
				_, exists := graph.Lookup(config.FullName())
				require.False(t, exists)
			}
			prepared, committed, _, enabled := owner.snapshot()
			require.Equal(t, []int{0, 0, 0}, []int{prepared, committed, enabled})
		})
	}
}

func TestConfigIntentAdmissionQuarantineAdvancesValidSibling(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	owner := &intentTestOwner{}
	creator := lighthouse.Creator(lighthouse.Dependencies{})
	creator.JobConfigIntent = owner
	creator.ActivationDependencies = intentAdmissionDependencies(true)
	controller.modules["module"] = creator
	config := acceptedActivationTestConfig(confgroup.TypeUser).Set("url", "https://quarantined.example.test")
	sibling := acceptedActivationTestConfig(confgroup.TypeStock).Set("url", "https://valid.example.test")
	controller.factory.config.Attempts = intentAdmissionQuarantinedAuthority{
		ProcessAttemptAuthority: controller.factory.config.Attempts,
		identity:                jobTestAttemptIdentity(configOperationValidateIntent, config),
	}
	commands := runtimeTestBindActivations(t, controller)
	inventory := sourceTestInventory(controller, config, true, false, "user-source")
	inventory.Sources["stock-source"] = 1
	inventory.Candidates = append(inventory.Candidates, DiscoveredJobCandidate{
		Config:      sibling,
		Source:      "stock-source",
		Incarnation: 1,
	})
	plan, err := controller.PlanInventory(inventory)
	require.NoError(t, err)
	applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
	applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 1)
	_, exists := graph.Lookup(config.FullName())
	require.False(t, exists)
	prepared, committed, _, enabled := owner.snapshot()
	require.Equal(t, []int{0, 0, 0}, []int{prepared, committed, enabled})
	applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	adopted, err := graphRecordConfig(record)
	require.NoError(t, err)
	require.Equal(t, sibling, adopted)
	prepared, committed, _, enabled = owner.snapshot()
	require.Equal(t, []int{1, 1, 1}, []int{prepared, committed, enabled})
}
