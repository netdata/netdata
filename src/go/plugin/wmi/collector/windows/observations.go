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
	system     systemCounters
	interfaces []networkInterface
	// netTotalReceived and netTotalSent sum the byte totals of the Network
	// Interface instances; physicalInterfaces names them.
	netTotalReceived   sample
	netTotalSent       sample
	physicalInterfaces map[string]bool
	drives             []diskSpace
}

// sample distinguishes a reported zero from a missing or malformed value.
type sample struct {
	value uint64
	valid bool
}

type diskSpace struct {
	mountPoint   string
	fileSystem   string
	serialNumber string
	avail        sample
	used         sample
}

// Win32_LogicalDisk Access value of media that can only be read.
const accessReadOnly = 1

// readMemory converts Win32_OperatingSystem's KiB values to bytes. Free
// physical memory is the memory available to processes, as the native Windows
// Agent's free dimension.
func (o *observations) readMemory(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	total := kibToBytes(readUnsigned(rows[0]["totalvisiblememorysize"]))
	free := kibToBytes(readUnsigned(rows[0]["freephysicalmemory"]))
	o.memoryFree = free
	o.memoryUsed = used(total, free)
	return free.valid
}

// readDisks leaves out read-only drives, as the native Windows Agent does, so
// a full read-only volume does not hold a disk space alert forever.
func (o *observations) readDisks(rows []map[string]any) bool {
	var usable bool
	for _, row := range rows {
		drive, _ := row["deviceid"].(string)
		if strings.TrimSpace(drive) == "" {
			continue
		}
		size := readUnsigned(row["size"])
		free := readUnsigned(row["freespace"])
		if !free.valid {
			continue
		}
		usable = true
		if access := readUnsigned(row["access"]); access.valid && access.value == accessReadOnly {
			continue
		}
		o.drives = append(o.drives, diskSpace{
			mountPoint:   drive,
			fileSystem:   fileSystem(row),
			serialNumber: volumeSerialNumber(row),
			avail:        free,
			used:         used(size, free),
		})
	}
	return len(rows) == 0 || usable
}

// fileSystem names an unformatted volume's file system "unknown", as the
// native Windows Agent does.
func fileSystem(row map[string]any) string {
	if name := textProperty(row, "filesystem"); name != "" {
		return name
	}
	return "unknown"
}

// volumeSerialNumber formats WMI's hexadecimal serial as the native Windows
// Agent does: 0x and uppercase digits without leading zeros, and 0x0 when the
// serial is unknown.
func volumeSerialNumber(row map[string]any) string {
	n, _ := strconv.ParseUint(textProperty(row, "volumeserialnumber"), 16, 32)
	return "0x" + strings.ToUpper(strconv.FormatUint(n, 16))
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
