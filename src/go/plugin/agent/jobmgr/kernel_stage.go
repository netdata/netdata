// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func (ck *CommandKernel) startPreClaimStage(operation *commandOperation) {
	if operation == nil || operation.plan.Stage == nil || operation.stageStarted ||
		operation.stagePending || operation.stageReleased {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid pre-claim stage start"))
		return
	}
	operation.stageStarted = true
	operation.stagePending = true
	operation.plan.Stage.Start()
	ready := operation.plan.Stage.Ready()
	go func() {
		select {
		case <-ready:
			select {
			case ck.preClaimStages <- preClaimStageReady{operation: operation}:
			case <-ck.done:
			}
		case <-ck.done:
		}
	}()
}

func (ck *CommandKernel) servicePreClaimStage(event preClaimStageReady) {
	operation := event.operation
	if operation == nil ||
		ck.operations[operation.UID] != operation ||
		!operation.stageStarted ||
		!operation.stagePending ||
		operation.stageReleased {
		return
	}
	operation.stagePending = false
	if operation.cancelled || operation.TimedOut() || operation.State >= lifecycle.OperationDisposing {
		return
	}
	if operation.lane != nil &&
		operation.lane.active == nil &&
		operation.lane.head == operation {
		ck.markReady(operation.lane)
	}
}

func (ck *CommandKernel) cancelPreClaimStage(operation *commandOperation, cause error) {
	if operation == nil ||
		operation.plan.Stage == nil ||
		!operation.stageStarted ||
		operation.stageReleased {
		return
	}
	operation.plan.Stage.Cancel(cause)
}

func (ck *CommandKernel) releasePreClaimStage(operation *commandOperation) {
	if operation == nil || operation.plan.Stage == nil || operation.stageReleased {
		return
	}
	operation.stageReleased = true
	operation.stagePending = false
	operation.plan.Stage.Release()
}
