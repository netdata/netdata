// SPDX-License-Identifier: GPL-3.0-or-later

package pinger

import "time"

type Sample struct {
	Host string

	PacketsSent int64
	PacketsRecv int64

	PacketLossPct float64

	RTT    RTTSummary
	Jitter JitterSummary
}

type RTTSummary struct {
	Valid bool

	Min    time.Duration
	Max    time.Duration
	Avg    time.Duration
	StdDev time.Duration
}

func (r RTTSummary) VarianceMicrosecondsSquared() int64 {
	if !r.Valid {
		return 0
	}

	us := r.StdDev.Microseconds()
	return us * us
}

func (r RTTSummary) VarianceMillisecondsSquared() float64 {
	if !r.Valid {
		return 0
	}

	ms := float64(r.StdDev) / float64(time.Millisecond)
	return ms * ms
}

type JitterSummary struct {
	InstantValid bool
	Mean         time.Duration

	SmoothedValid bool
	EWMA          time.Duration
	SMA           time.Duration
}
