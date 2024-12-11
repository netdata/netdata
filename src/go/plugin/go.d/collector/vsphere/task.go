// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sync"
	"time"
)

func newTask(doWork func(), doEvery time.Duration) *task {
	task := task{
		done:    make(chan struct{}),
		running: make(chan struct{}),
	}

	go func() {
		t := time.NewTicker(doEvery)
		defer func() {
			t.Stop()
			close(task.running)
		}()
		for {
			select {
			case <-task.done:
				return
			case <-t.C:
				doWork()
			}
		}
	}()

	return &task
}

type task struct {
	once    sync.Once
	done    chan struct{}
	running chan struct{}
}

func (t *task) stop() {
	t.once.Do(func() { close(t.done) })
}

func (t *task) isStopped() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

func (t *task) isRunning() bool {
	select {
	case <-t.running:
		return false
	default:
		return true
	}
}
