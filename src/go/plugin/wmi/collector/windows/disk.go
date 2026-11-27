// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"strconv"
	"strings"
	"time"
)

const queryDisk = "SELECT DeviceID, Size, FreeSpace, FileSystem, VolumeSerialNumber, Access FROM Win32_LogicalDisk " +
	"WHERE DriveType = 3"

// queryDiskDrives reads each physical disk's model and device ID, the labels
// the native Windows Agent adds to its disk charts.
const queryDiskDrives = "SELECT Index, Model, DeviceID FROM Win32_DiskDrive"

// diskDriveBudget caps the disk drive lookup within a poll; it also takes at
// most half of the remaining poll time, leaving room for the inventory and for
// closing the session.
const diskDriveBudget = 5 * time.Second

// diskCounter is a raw counter of the PhysicalDisk counter set that feeds one
// dimension of the native Windows Agent's disk charts.
type diskCounter struct {
	property string
	metric   string
	// total marks a count since the counter started, whose rate Netdata charts.
	total bool
	// milliseconds converts a time in 100 ns units to whole milliseconds, as the
	// native Windows Agent does before charting it.
	milliseconds bool
}

// The byte totals of the counter set's _Total instance also feed the host's
// disk I/O.
const (
	diskReadBytes = iota
	diskWrittenBytes
)

var diskCounters = [...]diskCounter{
	diskReadBytes:    {property: "DiskReadBytesPersec", metric: "disk_read_bytes", total: true},
	diskWrittenBytes: {property: "DiskWriteBytesPersec", metric: "disk_written_bytes", total: true},
	{property: "DiskReadsPersec", metric: "disk_reads", total: true},
	{property: "DiskWritesPersec", metric: "disk_writes", total: true},
	{property: "SplitIOPerSec", metric: "disk_split_io", total: true},
	{property: "CurrentDiskQueueLength", metric: "disk_queue_length"},
	// The native busy time reads % Disk Time, which Windows accumulates as the
	// queue length over time, so it can exceed the elapsed time.
	{property: "PercentDiskTime", metric: "disk_busy_milliseconds", total: true, milliseconds: true},
	{property: "PercentDiskReadTime", metric: "disk_read_time_milliseconds", total: true, milliseconds: true},
	{property: "PercentDiskWriteTime", metric: "disk_write_time_milliseconds", total: true, milliseconds: true},
}

// Average timers: the time of the completed operations in performance counter
// ticks and their number, both 32-bit, and the queue-length integral of the
// same operations in 100 ns units, which does not wrap.
var (
	readLatency     = averageTimerProperties{"avgdisksecperread", "avgdisksecperread_base", "avgdiskreadqueuelength"}
	writeLatency    = averageTimerProperties{"avgdisksecperwrite", "avgdisksecperwrite_base", "avgdiskwritequeuelength"}
	transferLatency = averageTimerProperties{
		"avgdisksecpertransfer",
		"avgdisksecpertransfer_base",
		"avgdiskqueuelength",
	}
)

var queryPhysicalDisks = func() string {
	properties := []string{"Name"}
	for _, counter := range diskCounters {
		properties = append(properties, counter.property)
	}
	properties = append(properties,
		"PercentIdleTime", "PercentIdleTime_Base",
		"AvgDisksecPerRead", "AvgDisksecPerRead_Base", "AvgDiskReadQueueLength",
		"AvgDisksecPerWrite", "AvgDisksecPerWrite_Base", "AvgDiskWriteQueueLength",
		"AvgDisksecPerTransfer", "AvgDisksecPerTransfer_Base", "AvgDiskQueueLength",
		"AvgDiskBytesPerRead", "AvgDiskBytesPerRead_Base",
		"AvgDiskBytesPerWrite", "AvgDiskBytesPerWrite_Base",
		"Frequency_PerfTime",
	)
	return "SELECT " + strings.Join(properties, ", ") + " FROM Win32_PerfRawData_PerfDisk_PhysicalDisk"
}()

type diskSpace struct {
	mountPoint   string
	fileSystem   string
	serialNumber string
	avail        sample
	used         sample
}

// physicalDisk is one physical disk's raw counters, named as the native
// Windows Agent names it: the instance "0 C: D:" is the device "Disk 0" with
// the mount point "C: D:".
type physicalDisk struct {
	device     string
	mountPoint string
	// index is the disk number that Win32_DiskDrive lists the disk under.
	index    uint64
	hasIndex bool
	values   [len(diskCounters)]sample
	// idleTime is the idle time and its timestamp, both in 100 ns units.
	idleTime     [2]sample
	readLatency  averageTimer
	writeLatency averageTimer
	// transferLatency is the native service time.
	transferLatency averageTimer
	readSize        averageBulk
	writeSize       averageBulk
	frequency       sample

	// Set by the poll: the disk's model and device ID, and the values of the
	// interval since the previous sample.
	drive    diskDrive
	interval diskInterval
}

type averageTimerProperties struct {
	ticks, operations, queueTime string
}

type averageTimer struct {
	ticks, operations, queueTime sample
}

// averageBulk is a 64-bit total, such as bytes, and the 32-bit number of the
// operations it counts.
type averageBulk struct {
	total, operations sample
}

type diskDrive struct {
	model    string
	deviceID string
}

// diskInterval holds what the native Windows Agent derives from two samples:
// the utilization in whole percent, latencies in microseconds and average
// operation sizes in bytes.
type diskInterval struct {
	utilization sample
	readAwait   sample
	writeAwait  sample
	serviceTime sample
	readSize    sample
	writeSize   sample
}

// Win32_LogicalDisk Access value of media that can only be read.
const accessReadOnly = 1

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

// readPhysicalDisks keeps each disk's first instance. The counter set's _Total
// feeds the host's disk I/O.
func (o *observations) readPhysicalDisks(rows []map[string]any) bool {
	var usable bool
	for _, row := range rows {
		name, _ := row["name"].(string)
		if strings.EqualFold(name, "_Total") {
			o.diskTotalRead = readUnsigned(row[strings.ToLower(diskCounters[diskReadBytes].property)])
			o.diskTotalWritten = readUnsigned(row[strings.ToLower(diskCounters[diskWrittenBytes].property)])
			usable = usable || o.diskTotalRead.valid || o.diskTotalWritten.valid
			continue
		}
		disk, ok := readPhysicalDisk(name, row)
		if !ok || o.hasPhysicalDisk(disk.device) {
			continue
		}
		usable = true
		o.physicalDisks = append(o.physicalDisks, disk)
	}
	o.physicalDisksRead = len(rows) == 0 || usable
	return o.physicalDisksRead
}

// readPhysicalDisk reads an instance that reports at least one counter.
func readPhysicalDisk(name string, row map[string]any) (physicalDisk, bool) {
	first, rest, _ := strings.Cut(strings.TrimSpace(name), " ")
	if first == "" {
		return physicalDisk{}, false
	}
	disk := physicalDisk{
		device:          first,
		mountPoint:      strings.TrimSpace(rest),
		idleTime:        [2]sample{readUnsigned(row["percentidletime"]), readUnsigned(row["percentidletime_base"])},
		readLatency:     readLatency.read(row),
		writeLatency:    writeLatency.read(row),
		transferLatency: transferLatency.read(row),
		readSize: averageBulk{
			total:      readUnsigned(row["avgdiskbytesperread"]),
			operations: readUnsigned(row["avgdiskbytesperread_base"]),
		},
		writeSize: averageBulk{
			total:      readUnsigned(row["avgdiskbytesperwrite"]),
			operations: readUnsigned(row["avgdiskbytesperwrite_base"]),
		},
		frequency: readUnsigned(row["frequency_perftime"]),
	}
	if index, err := strconv.ParseUint(first, 10, 32); err == nil {
		disk.device = "Disk " + strconv.FormatUint(index, 10)
		disk.index, disk.hasIndex = index, true
	}
	reported := disk.idleTime[0].valid || disk.readLatency.ticks.valid || disk.writeLatency.ticks.valid ||
		disk.transferLatency.ticks.valid || disk.readSize.total.valid || disk.writeSize.total.valid
	for i, counter := range diskCounters {
		disk.values[i] = readUnsigned(row[strings.ToLower(counter.property)])
		if counter.milliseconds && disk.values[i].valid {
			disk.values[i].value /= 10000
		}
		reported = reported || disk.values[i].valid
	}
	return disk, reported
}

func (p averageTimerProperties) read(row map[string]any) averageTimer {
	return averageTimer{
		ticks:      readUnsigned(row[p.ticks]),
		operations: readUnsigned(row[p.operations]),
		queueTime:  readUnsigned(row[p.queueTime]),
	}
}

func (o *observations) hasPhysicalDisk(device string) bool {
	for _, disk := range o.physicalDisks {
		if disk.device == device {
			return true
		}
	}
	return false
}

// unknownDiskDrive reports whether a numbered disk of this poll has no cached
// Win32_DiskDrive entry yet.
func (o *observations) unknownDiskDrive(drives map[string]diskDrive) bool {
	for _, disk := range o.physicalDisks {
		if _, known := drives[disk.device]; disk.hasIndex && !known {
			return true
		}
	}
	return false
}

// diskDrivesOf matches the Win32_DiskDrive rows to this poll's numbered disks
// by disk number. A disk that Windows does not list there gets an empty entry,
// so it is not looked up again while it exists.
func (o *observations) diskDrivesOf(rows []map[string]any) map[string]diskDrive {
	byIndex := make(map[uint64]diskDrive, len(rows))
	for _, row := range rows {
		if index := readUnsigned(row["index"]); index.valid {
			byIndex[index.value] = diskDrive{
				model:    textProperty(row, "model"),
				deviceID: textProperty(row, "deviceid"),
			}
		}
	}
	drives := make(map[string]diskDrive, len(o.physicalDisks))
	for _, disk := range o.physicalDisks {
		if disk.hasIndex {
			drives[disk.device] = byIndex[disk.index]
		}
	}
	return drives
}

// counter32Range is the number of values of a 32-bit counter.
const counter32Range = 1 << 32

// delta32 returns the increase of a 32-bit counter. As in the native Windows
// Agent, an increase of half the range or more is indistinguishable from a
// reset or a stale sample and is not valid.
func delta32(previous, current uint64) (uint64, bool) {
	delta := uint64(uint32(current - previous))
	return delta, delta < counter32Range/2
}

// utilizationPercent is the native disk utilization: 100 less the idle share
// of the interval, in whole percent.
func utilizationPercent(previous, current [2]sample) sample {
	idle, stamp, prevIdle, prevStamp := current[0], current[1], previous[0], previous[1]
	if !idle.valid || !stamp.valid || !prevIdle.valid || !prevStamp.valid || prevStamp.value == 0 ||
		idle.value < prevIdle.value || stamp.value <= prevStamp.value {
		return sample{}
	}
	idleTime := idle.value - prevIdle.value
	if idleTime > math.MaxUint64/100 {
		return sample{}
	}
	return sample{
		value: 100 - min(100, 100*idleTime/(stamp.value-prevStamp.value)),
		valid: true,
	}
}

// averageMicroseconds is the native average operation time of the interval,
// in microseconds, truncated as the native Windows Agent charts it. The 32-bit
// time can wrap more than once per interval on a busy disk; the queue-length
// integral of the same operations, converted to ticks, gives the number of
// wraps. A decreasing integral means the counters restarted.
func averageMicroseconds(previous, current averageTimer, frequency sample) sample {
	if !previous.valid() || !current.valid() || !frequency.valid || frequency.value == 0 ||
		current.queueTime.value < previous.queueTime.value {
		return sample{}
	}
	operations, ok := delta32(previous.operations.value, current.operations.value)
	if !ok {
		return sample{}
	}
	ticks := uint64(uint32(current.ticks.value - previous.ticks.value))
	queueTicks := float64(current.queueTime.value-previous.queueTime.value) * float64(frequency.value) / 1e7
	if wraps := math.Round((queueTicks - float64(ticks)) / counter32Range); wraps > 0 {
		ticks += uint64(wraps) * counter32Range
	}
	if operations == 0 {
		// No operation completed: no time either, or the samples disagree.
		return sample{
			value: 0,
			valid: ticks == 0,
		}
	}
	milliseconds := float64(ticks) * 1000 / (float64(frequency.value) * float64(operations))
	return sample{
		value: uint64(milliseconds * 1000),
		valid: true,
	}
}

func (t averageTimer) valid() bool { return t.ticks.valid && t.operations.valid && t.queueTime.valid }

// averagePerOperation is the native average of a 64-bit total per operation
// over the interval, such as the bytes per read.
func averagePerOperation(previous, current averageBulk) sample {
	if !previous.total.valid || !previous.operations.valid || !current.total.valid || !current.operations.valid ||
		current.total.value < previous.total.value {
		return sample{}
	}
	operations, ok := delta32(previous.operations.value, current.operations.value)
	if !ok {
		return sample{}
	}
	total := current.total.value - previous.total.value
	if operations == 0 {
		return sample{
			value: 0,
			valid: total == 0,
		}
	}
	return sample{
		value: total / operations,
		valid: true,
	}
}

// intervalSince derives the interval values from the disk's previous sample.
func (d physicalDisk) intervalSince(previous physicalDisk) diskInterval {
	return diskInterval{
		utilization: utilizationPercent(previous.idleTime, d.idleTime),
		readAwait:   averageMicroseconds(previous.readLatency, d.readLatency, d.frequency),
		writeAwait:  averageMicroseconds(previous.writeLatency, d.writeLatency, d.frequency),
		serviceTime: averageMicroseconds(previous.transferLatency, d.transferLatency, d.frequency),
		readSize:    averagePerOperation(previous.readSize, d.readSize),
		writeSize:   averagePerOperation(previous.writeSize, d.writeSize),
	}
}
