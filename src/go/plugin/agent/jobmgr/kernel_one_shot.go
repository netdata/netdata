// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// oneShotTask is kernel-owned bookkeeping for work with no returned resource.
// The accepted terminal action remains owned until its acknowledged task releases.
type oneShotTask struct {
	request lifecycle.TaskRequestRef
	ref     lifecycle.TaskRef
	action  lifecycle.TaskActionKind
	done    bool
	failed  bool
}

func (task *oneShotTask) idle() bool {
	return !task.request.Valid() && !task.ref.Valid() && task.action == 0
}

func (task *oneShotTask) ready() bool {
	return !task.done && !task.failed && task.idle()
}

func (task *oneShotTask) settled() bool {
	return task.done && !task.failed && task.idle()
}

func (task *oneShotTask) failedTerminal() bool {
	return task.failed && task.idle()
}

func (task *oneShotTask) start(start lifecycle.TaskStart, name string) error {
	if !task.request.Valid() || start.Request != task.request || !start.Task.Valid() ||
		task.ref.Valid() || task.action != 0 || task.done || task.failed {
		return fmt.Errorf("jobmgr kernel: invalid %s start acknowledgement", name)
	}
	task.request = lifecycle.TaskRequestRef{}
	task.ref = start.Task
	return nil
}

// The caller decides when ordinary work errors dirty the run. Protocol failures
// dirty it immediately because abandonment requires a dirty run.
func (ck *CommandKernel) completeOneShotTask(
	task *oneShotTask,
	completion lifecycle.TaskCompletion,
	name string,
) error {
	if !task.ref.Valid() || completion.Ref != task.ref || completion.Sequence != 1 ||
		completion.Kind != lifecycle.TaskOutcomeNone || task.action != 0 || task.done || task.failed {
		cause := errors.Join(completion.Err, fmt.Errorf("jobmgr kernel: invalid %s completion", name))
		task.failed = true
		if task.action == 0 && task.ref.Valid() && completion.Ref == task.ref {
			ck.abandonOneShotTask(task, completion.Sequence+1, cause)
		} else {
			// A replay cannot replace an action already accepted by the child.
			ck.run.Dirty(cause)
		}
		return cause
	}
	if completion.Err != nil {
		task.failed = true
	}
	if err := ck.tasks.SendAction(lifecycle.TaskAction{
		Ref:      task.ref,
		Sequence: 2,
		Kind:     lifecycle.TaskActionTerminate,
	}); err != nil {
		ck.abandonOneShotTask(task, 2, err)
	} else {
		task.action = lifecycle.TaskActionTerminate
	}
	return completion.Err
}

func (ck *CommandKernel) abandonOneShotTask(task *oneShotTask, sequence uint8, cause error) {
	task.failed = true
	ck.run.Dirty(cause)
	if err := ck.tasks.Abandon(task.ref, sequence); err != nil {
		ck.run.Dirty(errors.Join(cause, err))
		return
	}
	task.action = lifecycle.TaskActionAbandon
}

func (task *oneShotTask) validateAcknowledgement(ack lifecycle.TaskAcknowledgement, name string) error {
	if !task.ref.Valid() || ack.Ref != task.ref || ack.Sequence != 2 ||
		(ack.Kind != lifecycle.TaskActionTerminate && ack.Kind != lifecycle.TaskActionAbandon) ||
		task.action != ack.Kind || task.done {
		return fmt.Errorf("jobmgr kernel: invalid %s acknowledgement", name)
	}
	return nil
}

// Call only after validating the acknowledgment. Domain acknowledgments, such
// as catalog cleanup, remain the caller's responsibility after successful release.
func (ck *CommandKernel) releaseOneShotTask(
	task *oneShotTask,
	ack lifecycle.TaskAcknowledgement,
	resource string,
) bool {
	if ack.Err != nil {
		task.failed = true
	}
	if ack.Kind == lifecycle.TaskActionAbandon {
		task.failed = true
		ck.recordAbandonment(resource, ack.Ref, ack.Abandoned)
	}
	if err := ck.tasks.Release(ack.Ref); err != nil {
		task.failed = true
		ck.run.Dirty(err)
		return false
	}
	task.ref = lifecycle.TaskRef{}
	task.action = 0
	task.done = !task.failed
	return true
}

func (ck *CommandKernel) completeShutdownWork(task *oneShotTask, completion lifecycle.TaskCompletion, name string) {
	if completion.Err != nil {
		ck.run.Dirty(completion.Err)
	}
	ck.completeOneShotTask(task, completion, name)
}

func (ck *CommandKernel) acknowledgeShutdownWork(
	task *oneShotTask, ack lifecycle.TaskAcknowledgement, name, resource string,
) {
	if err := task.validateAcknowledgement(ack, name); err != nil {
		ck.run.Dirty(err)
		return
	}
	if ack.Err != nil {
		ck.run.Dirty(ack.Err)
	}
	ck.releaseOneShotTask(task, ack, resource)
}
