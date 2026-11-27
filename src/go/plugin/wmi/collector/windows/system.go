// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"math"
	"math/bits"
	"time"
)

// querySystem reads the System counter set, which the native Windows Agent
// reads for the same charts. SystemUpTime is the boot time and
// Timestamp_Object the time of the sample, both in Frequency_Object ticks.
const querySystem = "SELECT Processes, Threads, ContextSwitchesPersec, ProcessorQueueLength, SystemUpTime, " +
	"Timestamp_Object, Frequency_Object FROM Win32_PerfRawData_PerfOS_System"

const (
	// The System counter set's Timestamp_Object is the host's clock as a
	// FILETIME: UTC, in 100 ns ticks since 1601-01-01.
	fileTimeFrequency = 10_000_000
	fileTimeUnixEpoch = 116444736000000000

	// maxClockQueryTime bounds the error of a clock offset. The host read its
	// clock while the query ran, so comparing it with the middle of the query
	// is off by at most half the query time. A slower query, such as the first
	// one after the host unloaded its performance counter provider, gives no
	// offset.
	maxClockQueryTime = time.Second
)

// systemCounters holds the System counter set's values. contextSwitches is a
// raw total; uptime is in milliseconds, as the native Windows Agent sends it.
type systemCounters struct {
	processes       sample
	threads         sample
	contextSwitches sample
	processorQueue  sample
	uptime          sample
	clockOffset     clockOffset
}

// clockOffset is how far the host's clock is ahead of the poller's.
type clockOffset struct {
	value time.Duration
	valid bool
}

// queryTiming is the poller's clock around one query: when the query was sent
// and how long its reply took.
type queryTiming struct {
	sent     time.Time
	duration time.Duration
}

func (o *observations) readSystem(rows []map[string]any, timing queryTiming) bool {
	if len(rows) != 1 {
		return false
	}
	row := rows[0]
	now, frequency := readUnsigned(row["timestamp_object"]), readUnsigned(row["frequency_object"])
	o.system = systemCounters{
		processes:       readUnsigned(row["processes"]),
		threads:         readUnsigned(row["threads"]),
		contextSwitches: readUnsigned(row["contextswitchespersec"]),
		processorQueue:  readUnsigned(row["processorqueuelength"]),
		uptime:          elapsedMilliseconds(readUnsigned(row["systemuptime"]), now, frequency),
		clockOffset:     clockOffsetAt(now, frequency, timing),
	}
	s := o.system
	return s.processes.valid || s.threads.valid || s.contextSwitches.valid || s.processorQueue.valid ||
		s.uptime.valid || s.clockOffset.valid
}

// elapsedMilliseconds converts the ticks between start and now to
// milliseconds. A start after now gives no value.
func elapsedMilliseconds(start, now, frequency sample) sample {
	if !start.valid || !now.valid || !frequency.valid || frequency.value == 0 || now.value < start.value {
		return sample{}
	}
	hi, lo := bits.Mul64(now.value-start.value, 1000)
	if hi >= frequency.value {
		return sample{}
	}
	millis, _ := bits.Div64(hi, lo, frequency.value)
	return sample{
		value: millis,
		valid: true,
	}
}

// clockOffsetAt compares the host's clock, read during the query, with the
// poller's clock at the middle of the query.
func clockOffsetAt(hostClock, frequency sample, timing queryTiming) clockOffset {
	// Windows rejects FILETIME values above the signed 64-bit range.
	isFileTime := hostClock.valid && hostClock.value <= math.MaxInt64 &&
		frequency.valid && frequency.value == fileTimeFrequency
	bounded := timing.duration >= 0 && timing.duration <= maxClockQueryTime
	if !isFileTime || !bounded {
		return clockOffset{}
	}
	ticks := int64(hostClock.value) - fileTimeUnixEpoch
	host := time.Unix(ticks/fileTimeFrequency, ticks%fileTimeFrequency*100)
	offset := host.Sub(timing.sent.Add(timing.duration / 2))
	// Sub clamps offsets beyond about 292 years, which are not measurements.
	if offset == math.MinInt64 || offset == math.MaxInt64 {
		return clockOffset{}
	}
	return clockOffset{
		value: offset,
		valid: true,
	}
}
