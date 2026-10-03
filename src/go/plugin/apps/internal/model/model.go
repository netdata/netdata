// SPDX-License-Identifier: GPL-3.0-or-later

// Package model defines the owned values exchanged by the native scanner,
// grouping policy, metric writer and process Function.
package model

import "time"

// Metric indexes match the private C snapshot ABI; rates are already derived.
const (
	CPUUser = iota
	CPUSystem
	CPUGuest
	CPUChildrenUser
	CPUChildrenSystem
	CPUChildrenGuest
	MinorFaults
	MajorFaults
	ChildrenMinorFaults
	ChildrenMajorFaults
	VirtualMemory
	ResidentMemory
	SharedMemory
	SwapMemory
	ProportionalMemory
	EstimatedMemory
	ReadBytes
	WriteBytes
	LogicalReadBytes
	LogicalWriteBytes
	ReadCalls
	WriteCalls
	VoluntarySwitches
	InvoluntarySwitches
	Threads
	Uptime
	FDLimitPercent
	PSSAge
	MetricCount
)

const (
	FDFile = iota
	FDSocket
	FDPipe
	FDInotify
	FDEvent
	FDTimer
	FDSignal
	FDEpoll
	FDOther
	FDTypeCount
)

type Key struct {
	PID       int32
	StartTime uint64
}

type Process struct {
	Key         Key
	PPID        int32
	UID         uint32
	GID         uint32
	Comm        string
	Cmdline     string // Owned procfs argv bytes, including NUL separators.
	State       string
	Application string
	Values      [MetricCount]float64
	Valid       uint64
	FDCounts    [FDTypeCount]uint64
	FDValid     bool
}

func (p Process) Has(metric int) bool { return p.Valid&(uint64(1)<<metric) != 0 }

type Snapshot struct {
	Generation  uint64
	CollectedAt time.Time
	Processes   []Process
	// CPU percentages: 100 is one fully used core, in user/system/guest order.
	SystemCPU      [3]float64
	SystemCPUValid bool
	CPUCount       int
	Stats          ScanStats
}

type ScanStats struct {
	FileReads   uint64
	FDLinksRead uint64
	ReadErrors  uint64
}

// Assignment refers to one row in the current generation. Group IDs are stable
// while a group exists; zero means that the aggregation axis is disabled.
type Assignment struct {
	Key    Key
	Groups [3]uint32 // application, user, group
}

type GroupFD struct {
	ID     uint32
	Counts [FDTypeCount]uint64
	Valid  bool
}

type Group struct {
	ID        uint32
	Kind      string
	Name      string
	Values    [MetricCount]float64
	Valid     uint64
	Processes uint64
	FDCounts  [FDTypeCount]uint64
	FDValid   bool
	UptimeMax float64
}

func (g Group) Has(metric int) bool { return g.Valid&(uint64(1)<<metric) != 0 }
