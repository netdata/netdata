// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

// sourceTestInventory replaces one source fixture slot while retaining the other
// slots in the complete observation. It owns observation identities only.
func sourceTestInventory(
	controller *DynCfgJobController,
	config confgroup.Config,
	enabled, remove bool,
	slot ...string,
) DiscoveredJobInventory {
	inventory := DiscoveredJobInventory{
		FullName:   config.FullName(),
		Version:    1,
		AutoEnable: enabled,
		Sources:    make(map[string]uint64),
	}
	if record := controller.candidates.snapshot(config.FullName()); record != nil {
		inventory = record.Inventory
		inventory.FullName = config.FullName()
		inventory.Version++
		inventory.AutoEnable = enabled
		inventory.Sources = maps.Clone(record.Inventory.Sources)
		if inventory.Sources == nil {
			inventory.Sources = make(map[string]uint64)
		}
		inventory.Candidates = nil
		for _, candidate := range record.Inventory.Candidates {
			inventory.Candidates = append(inventory.Candidates, candidate)
		}
	}
	source := config.Source()
	if len(slot) > 0 && slot[0] != "" {
		source = slot[0]
	}
	if source == "" {
		source = "fixture-source"
	}
	var retained []DiscoveredJobCandidate
	var previous *DiscoveredJobCandidate
	for _, candidate := range inventory.Candidates {
		if candidate.Source == source {
			c := candidate
			previous = &c
			continue
		}
		retained = append(retained, candidate)
	}
	inventory.Candidates = retained
	if remove {
		delete(inventory.Sources, source)
		return inventory
	}
	lifetime := inventory.Sources[source]
	if lifetime == 0 {
		lifetime = inventory.Version
	}
	inventory.Sources[source] = lifetime
	candidate := DiscoveredJobCandidate{
		Config:      config,
		Source:      source,
		Incarnation: inventory.Version,
	}
	if previous != nil && reflect.DeepEqual(previous.Config, config) {
		candidate.Incarnation = previous.Incarnation
	}
	inventory.Candidates = append(inventory.Candidates, candidate)
	return inventory
}

func planSourceTestConfig(
	controller *DynCfgJobController,
	config confgroup.Config,
	enabled, remove bool,
	slot ...string,
) (jobmgr.WorkPlan, error) {
	return controller.PlanInventory(sourceTestInventory(controller, config, enabled, remove, slot...))
}

// A fixture captures the first real executor completion after source publication
// or interactive enablement; callers still control its Apply/Dispose boundary.
type discoveryTestFixture struct {
	Config  confgroup.Config
	Status  dyncfg.Status
	Remove  bool
	Restart bool
	Source  string
}

func prepareDiscoveryTestFixture(
	t *testing.T,
	controller *DynCfgJobController,
	ctx context.Context,
	fixture discoveryTestFixture,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	t.Helper()
	config := fixture.Config
	if fixture.Remove || fixture.Status == dyncfg.StatusAccepted {
		plan, err := planSourceTestConfig(
			controller,
			config,
			fixture.Status == dyncfg.StatusRunning,
			fixture.Remove,
			fixture.Source,
		)
		if err != nil {
			return nil, err
		}
		return plan.Transaction.Prepare(ctx, current, scope, permit)
	}

	if fixture.Restart && config.SourceType() != confgroup.TypeDyncfg {
		if record := controller.candidates.snapshot(config.FullName()); record != nil && record.Binding != nil {
			return controller.Prepare(
				ctx,
				DynCfgJobRequest{
					Args: []string{controller.configID(config.Module(), config.Name()), "restart"},
				},
				current,
				scope,
				permit,
			)
		}
	}

	var previousWork candidateWorkToken
	if record := controller.candidates.snapshot(config.FullName()); record != nil {
		if record.Proposal != nil {
			previousWork = record.Proposal.Token
		} else if record.Active != nil {
			previousWork = record.Active.Token
		}
	}
	commands := runtimeTestBindActivations(t, controller)
	shortScope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
	}
	if current != nil {
		shortScope.Current = current.Identity()
	}
	var observation lifecycle.PreparedResourceTransaction
	var err error
	if config.SourceType() == confgroup.TypeDyncfg {
		// A stored disabled graph is the baseline for an actual ENABLE command.
		if _, exists := controller.graph.Lookup(config.FullName()); !exists {
			seedDynCfgJobGraphRecord(t, controller, controller.graph, config, dyncfg.StatusDisabled)
		}
		observation, err = controller.Prepare(
			ctx,
			DynCfgJobRequest{
				Args: []string{controller.configID(config.Module(), config.Name()), "enable"},
			},
			current,
			shortScope,
			lifecycle.LongLivedPermit{},
		)
	} else {
		var plan jobmgr.WorkPlan
		plan, err = planSourceTestConfig(controller, config, true, false, fixture.Source)
		if err == nil {
			observation, err = plan.Transaction.Prepare(ctx, current, shortScope, lifecycle.LongLivedPermit{})
		}
	}
	if err != nil {
		return nil, err
	}
	applied, err := observation.Apply(ctx)
	if err != nil {
		return nil, err
	}
	_, _, current = applied.Ownership()

	record := controller.candidates.snapshot(config.FullName())
	var nextWork candidateWorkToken
	if record != nil {
		if record.Proposal != nil {
			nextWork = record.Proposal.Token
		} else if record.Active != nil {
			nextWork = record.Active.Token
		}
	}
	if nextWork == (candidateWorkToken{}) || nextWork == previousWork {
		// An unchanged observation has no new executor completion.
		return controller.noop(scope, current, permit, noResponseResult())
	}
	call := commands.next(t, "internal/jobs/accepted-activation")
	require.Equal(t, config.FullName(), call.plan.Transaction.ID)
	scope.Current = lifecycle.ResourceIdentity{}
	if current != nil {
		scope.Current = current.Identity()
	}
	prepared, err := call.plan.Transaction.Prepare(ctx, current, scope, permit)
	if err != nil {
		call.ack <- err
		return nil, err
	}
	return &runtimeTestActivationTransaction{
		PreparedResourceTransaction: prepared,
		ack:                         call.ack,
	}, nil
}

// This authority records forbidden process work in cancelled-context tests.
type unexpectedProcessAttemptAuthority struct{ calls atomic.Int32 }

func (a *unexpectedProcessAttemptAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	a.calls.Add(1)
	return nil, errors.New("test: cancelled work started")
}

func (a *unexpectedProcessAttemptAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	a.calls.Add(1)
	return errors.New("test: cancelled work superseded")
}
func (*unexpectedProcessAttemptAuthority) CutProcessAttempt(jobmgr.ProcessAttemptIdentity, error) bool {
	return false
}

func (*unexpectedProcessAttemptAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}

func seedTestCandidateRetry(
	t *testing.T,
	controller *DynCfgJobController,
	config confgroup.Config,
	after int,
) candidateWakeToken {
	t.Helper()
	delta, err := controller.candidates.prepareAdoptExact(
		config,
		true,
		dyncfg.StatusFailed,
		true,
		after,
		controller.scheduler.retries.now(),
	)
	require.NoError(t, err)
	commit, err := controller.prepareCandidateCommit(delta, false)
	require.NoError(t, err)
	commit()
	record := controller.candidates.snapshot(config.FullName())
	require.NotNil(t, record.Wake)
	return record.Wake.Token
}

func countTestEvent(events []string, match string) int {
	n := 0
	for _, event := range events {
		if event == match {
			n++
		}
	}
	return n
}

func seedTestCandidateBinding(
	t *testing.T,
	controller *DynCfgJobController,
	config confgroup.Config,
	status dyncfg.Status,
) {
	t.Helper()
	// The graph and policy record are one baseline fixture. The command under
	// test still owns its real preparation, acceptance and execution transitions.
	if config.SourceType() != confgroup.TypeDyncfg {
		inventory := sourceTestInventory(controller, config, false, false)
		delta, err := controller.candidates.prepareInventory(inventory, controller.scheduler.retries.now())
		require.NoError(t, err)
		require.True(t, delta.apply())
	}
	delta, err := controller.candidates.prepareAdoptExact(
		config,
		false,
		status,
		true,
		0,
		controller.scheduler.retries.now(),
	)
	require.NoError(t, err)
	require.True(t, delta.apply())

}
