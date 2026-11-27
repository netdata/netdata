// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "math/bits"

// querySystem reads the System counter set, which the native Windows Agent
// reads for the same charts. SystemUpTime is the boot time and
// Timestamp_Object the time of the sample, both in Frequency_Object ticks.
const querySystem = "SELECT Processes, Threads, ContextSwitchesPersec, ProcessorQueueLength, SystemUpTime, " +
	"Timestamp_Object, Frequency_Object FROM Win32_PerfRawData_PerfOS_System"

// systemCounters holds the System counter set's values. contextSwitches is a
// raw total; uptime is in milliseconds, as the native Windows Agent sends it.
type systemCounters struct {
	processes       sample
	threads         sample
	contextSwitches sample
	processorQueue  sample
	uptime          sample
}

func (o *observations) readSystem(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	row := rows[0]
	o.system = systemCounters{
		processes:       readUnsigned(row["processes"]),
		threads:         readUnsigned(row["threads"]),
		contextSwitches: readUnsigned(row["contextswitchespersec"]),
		processorQueue:  readUnsigned(row["processorqueuelength"]),
		uptime: elapsedMilliseconds(
			readUnsigned(row["systemuptime"]),
			readUnsigned(row["timestamp_object"]),
			readUnsigned(row["frequency_object"]),
		),
	}
	s := o.system
	return s.processes.valid || s.threads.valid || s.contextSwitches.valid || s.processorQueue.valid || s.uptime.valid
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
