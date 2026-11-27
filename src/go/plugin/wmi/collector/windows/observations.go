// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"strconv"
	"strings"
)

// observations holds one poll's values in base units: bytes, raw counters,
// raw processor times and percent.
type observations struct {
	cpuTimes   cpuSample
	cpu        cpuUtilization
	memoryFree sample
	memoryUsed sample
	// swapFree and swapUsed are the available commit and the commit charge.
	swapFree       sample
	swapUsed       sample
	memoryCounters [len(memoryCounters)]sample
	pageFaults     pageFaults
	pagingFiles    []pagingFile
	numaNodes      []numaNode
	system         systemCounters
	interfaces     []networkInterface
	// netTotalReceived and netTotalSent sum the byte totals of the Network
	// Interface instances; physicalInterfaces names them.
	netTotalReceived   sample
	netTotalSent       sample
	physicalInterfaces map[string]bool
	tcp                [ipVersions][len(tcpCounters)]sample
	udp                [ipVersions][len(udpCounters)]sample
	drives             []diskSpace
	physicalDisks      []physicalDisk
	// physicalDisksRead marks a usable PhysicalDisk reply.
	physicalDisksRead bool
	// diskTotalRead and diskTotalWritten are the PhysicalDisk counter set's
	// _Total byte counts.
	diskTotalRead    sample
	diskTotalWritten sample
}

// rawCounter is a raw performance counter that feeds one chart dimension.
type rawCounter struct {
	property string
	metric   string
	// total marks a count since the counter started, whose rate Netdata charts.
	total bool
}

// counterQuery selects the leading properties and then the counters from a
// counter set's class.
func counterQuery(class string, counters []rawCounter, leading ...string) string {
	properties := append([]string(nil), leading...)
	for _, counter := range counters {
		properties = append(properties, counter.property)
	}
	return "SELECT " + strings.Join(properties, ", ") + " FROM " + class
}

// readCounters reads one instance's counters and reports whether any of them
// has a value.
func readCounters(row map[string]any, counters []rawCounter, values []sample) bool {
	var usable bool
	for i, counter := range counters {
		values[i] = readUnsigned(row[strings.ToLower(counter.property)])
		usable = usable || values[i].valid
	}
	return usable
}

// sample distinguishes a reported zero from a missing or malformed value.
type sample struct {
	value uint64
	valid bool
}

func used(total, free sample) sample {
	if total.valid && free.valid && total.value >= free.value {
		return sample{
			value: total.value - free.value,
			valid: true,
		}
	}
	return sample{}
}

func kibToBytes(s sample) sample {
	if !s.valid || s.value > math.MaxUint64/1024 {
		return sample{}
	}
	return sample{
		value: s.value * 1024,
		valid: true,
	}
}

// readUnsigned accepts the non-negative integers DCOM decodes and the decimal
// text WinRM returns.
func readUnsigned(value any) sample {
	switch v := value.(type) {
	case uint8:
		return sample{
			value: uint64(v),
			valid: true,
		}
	case uint16:
		return sample{
			value: uint64(v),
			valid: true,
		}
	case uint32:
		return sample{
			value: uint64(v),
			valid: true,
		}
	case uint64:
		return sample{
			value: v,
			valid: true,
		}
	case int8:
		return signedSample(int64(v))
	case int16:
		return signedSample(int64(v))
	case int32:
		return signedSample(int64(v))
	case int64:
		return signedSample(v)
	case string:
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return sample{
				value: n,
				valid: true,
			}
		}
	}
	return sample{}
}

func signedSample(v int64) sample {
	if v < 0 {
		return sample{}
	}
	return sample{
		value: uint64(v),
		valid: true,
	}
}
