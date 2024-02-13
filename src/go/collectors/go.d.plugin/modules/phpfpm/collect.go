// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"math"

	"github.com/netdata/go.d.plugin/pkg/stm"
)

func (p *Phpfpm) collect() (map[string]int64, error) {
	st, err := p.client.getStatus()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(st)
	if !hasIdleProcesses(st.Processes) {
		return mx, nil
	}

	calcIdleProcessesRequestsDuration(mx, st.Processes)
	calcIdleProcessesLastRequestCPU(mx, st.Processes)
	calcIdleProcessesLastRequestMemory(mx, st.Processes)
	return mx, nil
}

func calcIdleProcessesRequestsDuration(mx map[string]int64, processes []proc) {
	statProcesses(mx, processes, "ReqDur", func(p proc) int64 { return int64(p.Duration) })
}

func calcIdleProcessesLastRequestCPU(mx map[string]int64, processes []proc) {
	statProcesses(mx, processes, "ReqCpu", func(p proc) int64 { return int64(p.CPU) })
}

func calcIdleProcessesLastRequestMemory(mx map[string]int64, processes []proc) {
	statProcesses(mx, processes, "ReqMem", func(p proc) int64 { return p.Memory })
}

func hasIdleProcesses(processes []proc) bool {
	for _, p := range processes {
		if p.State == "Idle" {
			return true
		}
	}
	return false
}

type accessor func(p proc) int64

func statProcesses(m map[string]int64, processes []proc, met string, acc accessor) {
	var sum, count, min, max int64
	for _, proc := range processes {
		if proc.State != "Idle" {
			continue
		}

		val := acc(proc)
		sum += val
		count += 1
		if count == 1 {
			min, max = val, val
			continue
		}
		min = int64(math.Min(float64(min), float64(val)))
		max = int64(math.Max(float64(max), float64(val)))
	}

	m["min"+met] = min
	m["max"+met] = max
	m["avg"+met] = sum / count
}
