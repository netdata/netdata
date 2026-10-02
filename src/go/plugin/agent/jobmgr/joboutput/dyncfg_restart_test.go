// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestRestartObserverRejectsAnotherGenerationAndLaterSuccess(t *testing.T) {
	controller := &DynCfgJobController{}
	identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 2}
	receipt := &restartReceipt{result: make(chan lifecycle.SealedResult, 1)}
	require.NoError(t, controller.restartObservers.register(identity, receipt))
	success := adoptedResult(dyncfg.CommandRestart, dyncfg.StatusRunning, jobFailure{})
	controller.completeRestart(lifecycle.ResourceIdentity{ID: identity.ID, Generation: 1}, success)
	controller.completeRestart(lifecycle.ResourceIdentity{ID: identity.ID, Generation: 3}, success)
	select {
	case <-receipt.result:
		t.Fatal("a different generation answered this restart")
	default:
	}
	failure := adoptedResult(dyncfg.CommandRestart, dyncfg.StatusFailed, jobFailure{class: failureTemporary, message: "startup failed"})
	controller.completeRestart(identity, failure)
	controller.retireRestart(identity)
	controller.completeRestart(identity, success)
	require.Equal(t, failure, <-receipt.result)
	require.Empty(t, receipt.result)
	require.Empty(t, controller.restartObservers.entries)
}

func TestRestartObserverDetachBeforeAndAfterRegistration(t *testing.T) {
	for _, before := range []bool{false, true} {
		controller := &DynCfgJobController{}
		identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 1}
		receipt := &restartReceipt{result: make(chan lifecycle.SealedResult, 1)}
		if before {
			controller.restartObservers.detach(receipt)
		}
		require.NoError(t, controller.restartObservers.register(identity, receipt))
		controller.restartObservers.detach(receipt)
		controller.completeRestart(identity, adoptedResult(dyncfg.CommandRestart, dyncfg.StatusRunning, jobFailure{}))
		require.Empty(t, receipt.result)
		require.Empty(t, controller.restartObservers.entries)
	}
}

func TestRestartObserverFollowsOnlyItsAcceptedActivation(t *testing.T) {
	controller := &DynCfgJobController{}
	reserved := lifecycle.ResourceIdentity{ID: "module_job", Generation: 2}
	installed := lifecycle.ResourceIdentity{ID: reserved.ID, Generation: 4}
	token := acceptedActivationToken{run: 1, generation: 3, uid: "config"}
	other := acceptedActivationToken{run: 1, generation: 5, uid: "config"}
	receipt := &restartReceipt{result: make(chan lifecycle.SealedResult, 1)}
	require.NoError(t, controller.restartObservers.register(reserved, receipt))
	controller.deferRestartToActivation(reserved, token)
	controller.installActivationRestart(other, installed)
	controller.retireActivationRestart(other)
	require.Empty(t, receipt.result)
	controller.installActivationRestart(token, installed)
	controller.retireActivationRestart(token)
	controller.retireRestart(reserved)
	require.Empty(t, receipt.result)
	success := adoptedResult(dyncfg.CommandRestart, dyncfg.StatusRunning, jobFailure{})
	controller.completeRestart(installed, success)
	require.Equal(t, success, <-receipt.result)
	require.Empty(t, controller.restartObservers.entries)
	require.Empty(t, controller.restartObservers.activations)
}

func TestRestartObserverDetachesPendingActivation(t *testing.T) {
	controller := &DynCfgJobController{}
	identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 2}
	token := acceptedActivationToken{run: 1, generation: 3, uid: "config"}
	receipt := &restartReceipt{result: make(chan lifecycle.SealedResult, 1)}
	require.NoError(t, controller.restartObservers.register(identity, receipt))
	controller.deferRestartToActivation(identity, token)
	controller.restartObservers.detach(receipt)
	controller.installActivationRestart(token, identity)
	controller.retireActivationRestart(token)
	require.Empty(t, receipt.result)
	require.Empty(t, controller.restartObservers.entries)
	require.Empty(t, controller.restartObservers.activations)
}
