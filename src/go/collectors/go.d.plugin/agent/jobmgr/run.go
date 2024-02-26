// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"slices"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/ticker"
)

func (m *Manager) runRunningJobsHandling(ctx context.Context) {
	tk := ticker.New(time.Second)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case clock := <-tk.C:
			//m.Debugf("tick %d", clock)
			m.notifyRunningJobs(clock)
		}
	}
}

func (m *Manager) notifyRunningJobs(clock int) {
	m.queueMux.Lock()
	defer m.queueMux.Unlock()

	for _, v := range m.queue {
		v.Tick(clock)
	}
}

func (m *Manager) startJob(job Job) {
	m.queueMux.Lock()
	defer m.queueMux.Unlock()

	go job.Start()

	m.queue = append(m.queue, job)
}

func (m *Manager) stopJob(name string) {
	m.queueMux.Lock()
	defer m.queueMux.Unlock()

	idx := slices.IndexFunc(m.queue, func(job Job) bool {
		return job.FullName() == name
	})

	if idx != -1 {
		j := m.queue[idx]
		j.Stop()

		copy(m.queue[idx:], m.queue[idx+1:])
		m.queue[len(m.queue)-1] = nil
		m.queue = m.queue[:len(m.queue)-1]
	}
}

func (m *Manager) stopRunningJobs() {
	m.queueMux.Lock()
	defer m.queueMux.Unlock()

	for i, v := range m.queue {
		v.Stop()
		m.queue[i] = nil
	}
	m.queue = m.queue[:0]
}
