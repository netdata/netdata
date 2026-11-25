// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"strconv"
	"strings"
)

// observations holds one poll's metric values in base units (bytes, bytes/s, percent).
type observations struct {
	cpuBusy         sample
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
	busy := readUnsigned(rows[0]["percentprocessortime"])
	if !busy.valid || busy.value > 100 {
		return false
	}
	o.cpuBusy = busy
	return true
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
