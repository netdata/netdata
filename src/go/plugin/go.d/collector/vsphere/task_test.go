// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_task(t *testing.T) {
	var i int64
	job := func() {
		atomic.AddInt64(&i, 1)
	}

	task := newTask(job, time.Millisecond)
	defer func() {
		task.stop()
		task.wait()
	}()

	assert.Eventually(t, func() bool {
		return atomic.LoadInt64(&i) > 0
	}, time.Second, time.Millisecond)
}

func Test_task_state(t *testing.T) {
	tests := map[string]struct {
		state      func(*task) bool
		wantBefore bool
		wantAfter  bool
	}{
		"is stopped": {
			state:      (*task).isStopped,
			wantBefore: false,
			wantAfter:  true,
		},
		"is running": {
			state:      (*task).isRunning,
			wantBefore: true,
			wantAfter:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			task := newTask(func() {}, time.Second)
			assert.Equal(t, tc.wantBefore, tc.state(task))

			task.stop()
			task.wait()
			assert.Equal(t, tc.wantAfter, tc.state(task))
		})
	}
}
