// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// DumpAnalyzer collects and analyzes metric structure from dump mode
type DumpAnalyzer struct {
	mu         sync.RWMutex
	jobs       map[string]*JobAnalysis // key: job name
	startTime  time.Time
	dataDir    string
	jobDirs    map[string]string
	jobDone    map[string]bool
	onComplete func()
	completed  bool
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

// NewDumpAnalyzer creates a new dump analyzer
func NewDumpAnalyzer() *DumpAnalyzer {
	return &DumpAnalyzer{
		jobs:      make(map[string]*JobAnalysis),
		startTime: time.Now(),
		jobDirs:   make(map[string]string),
		jobDone:   make(map[string]bool),
	}
}
