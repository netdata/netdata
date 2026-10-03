// SPDX-License-Identifier: GPL-3.0-or-later
package grouping

import (
	"math"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

// Adapted from apps_plugin.c normalize_utilization. Running processes take
// priority over exited-child attribution. Missing global CPU preserves measured
// rates; a valid zero reduces CPU to zero. Fault scaling retains the original
// CPU-derived heuristic, not a vmstat bound.
func normalize(s *model.Snapshot) {
	if !s.SystemCPUValid {
		return
	}
	var totals [6]float64
	for _, p := range s.Processes {
		for m := model.CPUUser; m <= model.CPUChildrenGuest; m++ {
			if p.Has(m) {
				totals[m] += p.Values[m]
			}
		}
	}
	global := s.SystemCPU
	if s.CPUCount > 0 {
		for i := range global {
			global[i] = math.Min(global[i], float64(s.CPUCount)*100)
		}
	}
	running := totals[0] + totals[1]
	children := totals[3] + totals[4]
	system := global[0] + global[1]
	ownRatio, childRatio := 1.0, 1.0
	switch {
	case system+global[2] == 0:
		ownRatio, childRatio = 0, 0
	case system+global[2] > running+children+totals[2]+totals[5]:
	case system > running && children > 0:
		childRatio = (system - running) / children
	case running > 0:
		ownRatio = system / running
		childRatio = 0
	default:
		ownRatio, childRatio = 0, 0
	}
	ownRatio = math.Max(0, math.Min(1, ownRatio))
	childRatio = math.Max(0, math.Min(1, childRatio))
	ownFaultRatio, childFaultRatio := ownRatio, childRatio
	if running+totals[2] == 0 {
		ownFaultRatio = 1
	}
	if children+totals[5] == 0 {
		childFaultRatio = 1
	}
	for i := range s.Processes {
		p := &s.Processes[i]
		for m := model.CPUUser; m <= model.CPUChildrenGuest; m++ {
			if m <= model.CPUGuest {
				p.Values[m] *= ownRatio
			} else {
				p.Values[m] *= childRatio
			}
		}
		p.Values[model.MinorFaults] *= ownFaultRatio
		p.Values[model.MajorFaults] *= ownFaultRatio
		p.Values[model.ChildrenMinorFaults] *= childFaultRatio
		p.Values[model.ChildrenMajorFaults] *= childFaultRatio
	}
}
