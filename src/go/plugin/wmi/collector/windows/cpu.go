// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// cpuModes are the native Windows Agent's CPU chart dimensions, in its order,
// and cpuCounters the raw Processor Information properties behind them. Each
// counter is the time spent in its mode, in 100 ns units.
var (
	cpuModes    = [...]string{"irq", "user", "system", "dpc", "idle"}
	cpuCounters = [len(cpuModes)]string{
		"percentinterrupttime",
		"percentusertime",
		"percentprivilegedtime",
		"percentdpctime",
		"percentidletime",
	}
)

// cpuTimes is one processor's raw time per mode, in cpuModes order.
type cpuTimes [len(cpuModes)]uint64

// cpuShares is each mode's percent share of an interval, in cpuModes order.
type cpuShares [len(cpuModes)]float64

// cpuSample is one poll's raw processor times: the total of all processors
// and each logical processor by its native label (cpu0, cpu1, ...).
type cpuSample struct {
	total    cpuTimes
	hasTotal bool
	cores    map[string]cpuTimes
}

func (s cpuSample) empty() bool { return !s.hasTotal && len(s.cores) == 0 }

// cpuUtilization holds the mode shares of the interval between two samples.
type cpuUtilization struct {
	total    cpuShares
	hasTotal bool
	cores    map[string]cpuShares
}

// readCPU reads the Processor Information counter set, which the native
// Windows Agent prefers. Unlike the legacy Processor set, it covers every
// processor group on hosts with more than 64 logical processors, and its idle
// time is the idle thread's time.
func (o *observations) readCPU(rows []map[string]any) bool {
	type core struct {
		node, index uint64
		times       cpuTimes
		valid       bool
	}
	var cores []core
	for _, row := range rows {
		times, valid := readCPUTimes(row)
		name, _ := row["name"].(string)
		if name == "_Total" {
			o.cpuTimes.total, o.cpuTimes.hasTotal = times, valid
			continue
		}
		// Per-core instances are named "node,index"; "node,_Total" is a NUMA
		// node's total, which the native charts leave out.
		nodeText, indexText, _ := strings.Cut(name, ",")
		node, nodeErr := strconv.ParseUint(nodeText, 10, 32)
		index, indexErr := strconv.ParseUint(indexText, 10, 32)
		if nodeErr == nil && indexErr == nil {
			cores = append(cores, core{
				node:  node,
				index: index,
				times: times,
				valid: valid,
			})
		}
	}
	// The native plugin numbers cores by asking the host for its NUMA layout,
	// which WMI does not expose. Numbering them in node and index order gives
	// the same labels when each node holds consecutive processors, as on
	// single-node hosts. A core with unreadable counters keeps its number, so
	// the cores after it keep theirs.
	slices.SortFunc(cores, func(a, b core) int {
		return cmp.Or(cmp.Compare(a.node, b.node), cmp.Compare(a.index, b.index))
	})
	o.cpuTimes.cores = make(map[string]cpuTimes, len(cores))
	for i, c := range cores {
		if c.valid {
			o.cpuTimes.cores["cpu"+strconv.Itoa(i)] = c.times
		}
	}
	return !o.cpuTimes.empty()
}

func readCPUTimes(row map[string]any) (cpuTimes, bool) {
	var times cpuTimes
	for i, counter := range cpuCounters {
		s := readUnsigned(row[counter])
		if !s.valid {
			return cpuTimes{}, false
		}
		times[i] = s.value
	}
	return times, true
}

// cpuUtilizationBetween computes the shares of every processor present in
// both samples.
func cpuUtilizationBetween(prev, cur cpuSample) cpuUtilization {
	var u cpuUtilization
	if prev.hasTotal && cur.hasTotal {
		u.total, u.hasTotal = modeShares(prev.total, cur.total)
	}
	for label, times := range cur.cores {
		before, ok := prev.cores[label]
		if !ok {
			continue
		}
		if shares, ok := modeShares(before, times); ok {
			if u.cores == nil {
				u.cores = make(map[string]cpuShares, len(cur.cores))
			}
			u.cores[label] = shares
		}
	}
	return u
}

// modeShares gives each mode's share, in percent, of the time all modes
// accumulated between two samples: the native chart's percent-of-total-delta
// algorithm. Privileged time already includes interrupt and DPC time, so, as on
// the native chart, those count twice in the total. A counter that went back,
// such as after a reboot, or an interval without time leaves no value.
func modeShares(prev, cur cpuTimes) (cpuShares, bool) {
	var deltas [len(cpuModes)]float64
	var total float64
	for i := range cur {
		if cur[i] < prev[i] {
			return cpuShares{}, false
		}
		deltas[i] = float64(cur[i] - prev[i])
		total += deltas[i]
	}
	if total == 0 {
		return cpuShares{}, false
	}
	var shares cpuShares
	for i, delta := range deltas {
		shares[i] = 100 * delta / total
	}
	return shares, true
}
