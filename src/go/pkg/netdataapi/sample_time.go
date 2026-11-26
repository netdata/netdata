// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"strconv"
	"sync/atomic"
	"time"
)

// SampleClock issues the collection times of chart samples.
//
// With `END <seconds> <microseconds> 1` (ENDTimed) the Agent places a sample at
// its time and derives the interval since the chart's previous sample from the
// two times; it ignores a sample whose time is not after the chart's last
// collection time. The clock never issues a time at or before one it issued, at
// microsecond resolution, so a single clock per plugin process keeps the times of
// every chart increasing across jobs, job restarts and wall-clock steps back. The
// zero value is ready to use; it is safe for concurrent use.
type SampleClock struct {
	last atomic.Int64 // unix microseconds of the latest issued time
}

// Next returns the sample time for t, moved just past the latest issued time when
// t is not after it.
func (c *SampleClock) Next(t time.Time) SampleTime {
	us := t.UnixMicro()
	for {
		prev := c.last.Load()
		if us <= prev {
			us = prev + 1
		}
		if c.last.CompareAndSwap(prev, us) {
			return newSampleTime(us)
		}
	}
}

// endTimedLen bounds `END <seconds> <microseconds> 1\n\n` for any int64 time.
const endTimedLen = 40

// SampleTime is the collection time of a run's samples, with its END command
// rendered once: writing it for each chart copies bytes instead of formatting.
// The zero value means no time.
type SampleTime struct {
	usec int64
	n    uint8
	end  [endTimedLen]byte
}

func newSampleTime(usec int64) SampleTime {
	t := SampleTime{usec: usec}
	b := append(t.end[:0], "END "...)
	b = strconv.AppendInt(b, usec/1e6, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, usec%1e6, 10)
	b = append(b, " 1\n\n"...)
	t.n = uint8(len(b))
	return t
}

// IsZero reports whether t holds no time.
func (t SampleTime) IsZero() bool { return t.n == 0 }

// Time returns the sample time.
func (t SampleTime) Time() time.Time { return time.UnixMicro(t.usec) }

// ENDTimed commits the chart's values as collected at t: the Agent stores the
// sample at that time and computes the interval since the chart's previous sample
// from it, independent of when the line arrives. A zero t writes a plain END.
func (a *API) ENDTimed(t SampleTime) {
	if t.n == 0 {
		_, _ = a.Write(end)
		return
	}
	a.send(append(a.line(), t.end[:t.n]...))
}
