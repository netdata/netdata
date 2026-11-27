// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"strconv"
	"time"
)

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

// NewSampleTime returns the sample time for a run that started at t, at
// microsecond resolution.
//
// With `END <seconds> <microseconds> 1` (ENDTimed) the Agent places a sample at
// its time and derives the interval since the chart's previous sample from the
// two times; it ignores a sample whose time is not after the chart's last
// collection time. The run start is therefore used as is: after a wall-clock step
// back the Agent drops the stale samples and restarts the chart's counter rates,
// instead of storing the growth of the step in one point.
func NewSampleTime(t time.Time) SampleTime {
	st := SampleTime{usec: t.UnixMicro()}
	b := append(st.end[:0], "END "...)
	b = strconv.AppendInt(b, st.usec/1e6, 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, st.usec%1e6, 10)
	b = append(b, " 1\n\n"...)
	st.n = uint8(len(b))
	return st
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
