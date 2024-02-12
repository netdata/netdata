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

	task := newTask(job, time.Millisecond*200)
	defer task.stop()
	time.Sleep(time.Second)
	assert.True(t, atomic.LoadInt64(&i) > 0)
}

func Test_task_isStopped(t *testing.T) {
	task := newTask(func() {}, time.Second)
	assert.False(t, task.isStopped())

	task.stop()
	time.Sleep(time.Millisecond * 500)
	assert.True(t, task.isStopped())
}

func Test_task_isRunning(t *testing.T) {
	task := newTask(func() {}, time.Second)
	assert.True(t, task.isRunning())

	task.stop()
	time.Sleep(time.Millisecond * 500)
	assert.False(t, task.isRunning())
}
