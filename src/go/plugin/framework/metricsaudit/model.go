// SPDX-License-Identifier: GPL-3.0-or-later

package metricsaudit

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	writeQueueSize       = 256
	maxWriteErrorSamples = 10
)

// JobID identifies a job uniquely across modules.
type JobID struct {
	Module string
	Name   string
}

func newJobID(moduleName, jobName string) JobID {
	return JobID{
		Module: moduleName,
		Name:   jobName,
	}
}

type writeTask struct {
	label string
	run   func() error
	after func()
	flush chan struct{}
}

// Auditor collects and analyzes metric structure from metrics-audit mode.
type Auditor struct {
	mu              sync.RWMutex
	jobs            map[JobID]*JobAnalysis
	startTime       time.Time
	dataDir         string
	jobDirs         map[JobID]string
	jobDone         map[JobID]bool
	onComplete      func()
	completed       bool
	writeCh         chan writeTask
	writeErrorCount int
	writeErrors     []string
}

// JobAnalysis holds analysis for a single job
type JobAnalysis struct {
	Name            string
	Module          string
	Charts          []ChartAnalysis
	CollectionCount int
	LastCollection  time.Time
	AllSeenMetrics  map[string]bool // Track ALL metrics seen in mx map
}

// ChartAnalysis holds analysis for a single chart
type ChartAnalysis struct {
	Chart           *collectorapi.Chart
	CollectedValues map[string][]int64 // dimension ID -> collected values
	SeenDimensions  map[string]bool    // track which dimensions received data
}

// New creates a new metrics-audit analyzer.
func New() *Auditor {
	return &Auditor{
		jobs:      make(map[JobID]*JobAnalysis),
		startTime: time.Now(),
		jobDirs:   make(map[JobID]string),
		jobDone:   make(map[JobID]bool),
	}
}
