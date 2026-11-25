// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"strconv"
	"strings"
)

// observations holds one poll's values in base units: bytes, byte counters and percent.
type observations struct {
	cpuTimes        cpuTimes
	cpuBusy         percent
	memoryUsed      sample
	memoryAvailable sample
	interfaces      []networkTraffic
	drives          []diskSpace
}

// sample distinguishes a reported zero from a missing or malformed value.
type sample struct {
	value uint64
	valid bool
}

// percent is a computed share that may be unavailable.
type percent struct {
	value float64
	valid bool
}

// cpuTimes is one raw sample of the _Total processor counters, in 100 ns units.
type cpuTimes struct {
	idle      uint64 // PercentProcessorTime: idle time (PERF_100NSEC_TIMER_INV)
	timestamp uint64 // Timestamp_Sys100NS
	valid     bool
}

// networkTraffic holds an interface's byte totals since its counters started.
type networkTraffic struct {
	name     string
	received sample
	sent     sample
}

type diskSpace struct {
	drive string
	used  sample
	free  sample
}

func (o *observations) readCPU(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	idle := readUnsigned(rows[0]["percentprocessortime"])
	timestamp := readUnsigned(rows[0]["timestamp_sys100ns"])
	if !idle.valid || !timestamp.valid {
		return false
	}
	o.cpuTimes = cpuTimes{
		idle:      idle.value,
		timestamp: timestamp.value,
		valid:     true,
	}
	return true
}

// busyPercent is the share of the interval between two samples that the
// processors were busy. It is unavailable without a previous sample and after
// a counter reset, such as a reboot.
func busyPercent(prev, cur cpuTimes) percent {
	if !prev.valid || !cur.valid || cur.timestamp <= prev.timestamp || cur.idle < prev.idle {
		return percent{}
	}
	elapsed := cur.timestamp - prev.timestamp
	idle := min(cur.idle-prev.idle, elapsed)
	return percent{
		value: 100 * float64(elapsed-idle) / float64(elapsed),
		valid: true,
	}
}

// readMemory converts Win32_OperatingSystem's KiB values to bytes.
func (o *observations) readMemory(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	total := kibToBytes(readUnsigned(rows[0]["totalvisiblememorysize"]))
	free := kibToBytes(readUnsigned(rows[0]["freephysicalmemory"]))
	o.memoryAvailable = free
	o.memoryUsed = used(total, free)
	return free.valid
}

func (o *observations) readNetwork(rows []map[string]any) bool {
	for _, row := range rows {
		name, _ := row["name"].(string)
		if strings.TrimSpace(name) == "" || name == "_Total" {
			continue
		}
		received := readUnsigned(row["bytesreceivedpersec"])
		sent := readUnsigned(row["bytessentpersec"])
		if received.valid || sent.valid {
			o.interfaces = append(o.interfaces, networkTraffic{
				name:     name,
				received: received,
				sent:     sent,
			})
		}
	}
	return len(rows) == 0 || len(o.interfaces) > 0
}

func (o *observations) readDisks(rows []map[string]any) bool {
	for _, row := range rows {
		drive, _ := row["deviceid"].(string)
		if strings.TrimSpace(drive) == "" {
			continue
		}
		size := readUnsigned(row["size"])
		free := readUnsigned(row["freespace"])
		if free.valid {
			o.drives = append(o.drives, diskSpace{
				drive: drive,
				used:  used(size, free),
				free:  free,
			})
		}
	}
	return len(rows) == 0 || len(o.drives) > 0
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
