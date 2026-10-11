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
				plan, err := controller.PlanDiscovered(
					DiscoveredJobChange{Config: config, Status: dyncfg.StatusRunning},
				)
				require.NoError(t, err)
				permit, _ := issueTestJobPermit(t, config.FullName(), 1)
				prepared, err := plan.Transaction.Prepare(
					t.Context(),
					nil,
					lifecycle.ResourceTransactionScope{
						ID:        config.FullName(),
						Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1},
					},
					permit,
				)
				if prepared != nil {
					_, applyErr := prepared.Apply(t.Context())
					require.NoError(t, applyErr)
				} else {
					require.NoError(t, permit.AbortUnused())
				}
				require.True(t, jobmgr.IsProposalRejection(err), "invalid config must be rejected: %v", err)
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
			plan, err := controller.PlanDiscovered(DiscoveredJobChange{Config: config, Status: dyncfg.StatusAccepted})
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
	plan, err := controller.PlanDiscovered(DiscoveredJobChange{Config: config, Status: dyncfg.StatusRunning})
	require.NoError(t, err)
	applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 1)
	before, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	replacement, err := config.Clone()
	require.NoError(t, err)
	replacement.Set("url", map[string]any{"bad": "shape"})
	plan, err = controller.PlanDiscovered(DiscoveredJobChange{Config: replacement, Status: dyncfg.StatusRunning})
	require.NoError(t, err)
	permit, _ := issueTestJobPermit(t, config.FullName(), 2)
	prepared, err := plan.Transaction.Prepare(
		t.Context(),
		nil,
		lifecycle.ResourceTransactionScope{
			ID:        config.FullName(),
			Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 2},
		},
		permit,
	)
	if prepared != nil {
		_, applyErr := prepared.Apply(t.Context())
		require.NoError(t, applyErr)
	} else {
		require.NoError(t, permit.AbortUnused())
	}
	require.True(t, jobmgr.IsProposalRejection(err), "invalid replacement must be rejected: %v", err)
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
		Identity: identity, Target: controller.factory.config.Epoch,
		Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error { close(entered); <-release; return nil },
	})
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("validation attempt did not start")
	}
	physicalRelease, ok := authority.ProcessAttemptReleased(identity)
	require.True(t, ok)
	commands := &activationTestCommands{queue: make(chan activationTestSubmission, 16), stop: make(chan struct{})}
	require.NoError(
		t,
		controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("background failure: %v", err) }),
	)
	t.Cleanup(func() {
		unblock()
		require.NoError(t, attempt.Await(context.Background()))
		close(commands.stop)
		controller.scheduler.StopBackgroundWorkers()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
	})
	plan, err := controller.PlanDiscovered(DiscoveredJobChange{Config: config, Status: dyncfg.StatusRunning})
	require.NoError(t, err)
	applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 1)
	_, exists := graph.Lookup(config.FullName())
	require.False(t, exists, "validation wait must not adopt unproven configuration")
	prepared, committed, _, enabled := owner.snapshot()
	require.Zero(t, prepared)
	require.Zero(t, committed)
	require.Zero(t, enabled)
	controller.scheduler.pending.mu.Lock()
	pending := controller.scheduler.pending.entries[config.FullName()]
	var pendingRelease <-chan struct{}
	if pending != nil {
		pendingRelease = pending.release
	}
	controller.scheduler.pending.mu.Unlock()
	require.NotNil(t, pending)
	require.Equal(t, physicalRelease, pendingRelease, "wait for raw validation, not candidate execution")
	unblock()
	require.NoError(t, attempt.Await(t.Context()))
	retry := commands.next(t, "internal/jobs/pending")
	applyActivationTestSubmission(t, retry, nil, 2)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	prepared, committed, _, enabled = owner.snapshot()
	require.Equal(t, []int{1, 1, 1}, []int{prepared, committed, enabled})
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
			controller.factory.config.Attempts = attemptFailureTestAuthority{
				delegate:  controller.factory.config.Attempts,
				namespace: jobmgr.ProcessAttemptJobTest,
				err:       jobmgr.ErrProcessAttemptQuarantined,
			}
			config := acceptedActivationTestConfig(confgroup.TypeUser).Set("url", "https://example.test")
			if route == "enable" {
				plan, err := controller.PlanDiscovered(
					DiscoveredJobChange{Config: config, Status: dyncfg.StatusAccepted},
				)
				require.NoError(t, err)
				applyActivationTestSubmission(t, activationTestSubmission{plan: plan}, nil, 0)
				prepared, err := prepareIntentTestEnable(t, controller, config)
				require.NoError(t, err)
				applied, err := prepared.Apply(t.Context())
				require.NoError(t, err)
				require.Equal(t, 503, applied.ResultStatus())
				require.False(t, controller.ActivationEnabled(config.FullName()))
			} else {
				plan, err := controller.PlanDiscovered(DiscoveredJobChange{Config: config, Status: dyncfg.StatusRunning})
				require.NoError(t, err)
				permit, _ := issueTestJobPermit(t, config.FullName(), 1)
				prepared, err := plan.Transaction.Prepare(t.Context(), nil, lifecycle.ResourceTransactionScope{ID: config.FullName(), Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1}}, permit)
				require.True(t, jobmgr.IsProposalRejection(err))
				require.Nil(t, prepared)
				require.NoError(t, permit.AbortUnused())
				_, exists := graph.Lookup(config.FullName())
				require.False(t, exists)
			}
			prepared, committed, _, enabled := owner.snapshot()
			require.Zero(t, prepared)
			require.Zero(t, committed)
			require.Zero(t, enabled)
		})
	}
}
