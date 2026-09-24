// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
)

// StartupDone signals the first readiness or failure outcome. StartupResult
// rechecks the live run, including a failure occurring after that first signal.
func (jg *JobGeneration) StartupDone() <-chan struct{} {
	if jg == nil {
		return nil
	}
	if run := jg.processOwner.managedRun(); run != nil {
		return run.StartupDone()
	}
	return nil
}

// AwaitReady is for callers outside the mutation lane that need an informative
// startup result. Cancellation only ends this wait; Stop owns cancellation.
func (jg *JobGeneration) AwaitReady(ctx context.Context) error {
	if jg == nil || ctx == nil {
		return errors.New("job output: invalid startup wait")
	}
	select {
	case <-jg.StartupDone():
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	if err := jg.StartupResult(); err != nil {
		return err
	}
	jg.mu.Lock()
	defer jg.mu.Unlock()
	if jg.state != JobActivating && jg.state != JobReady {
		return fmt.Errorf("job output: readiness from state %s", jg.state)
	}
	jg.state = JobReady
	return nil
}
