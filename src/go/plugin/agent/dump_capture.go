// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func (da *DumpAnalyzer) EnableDataCapture(dir string, onComplete func()) {
	da.mu.Lock()
	defer da.mu.Unlock()
	da.dataDir = dir
	da.onComplete = onComplete
}

// RegisterJob registers directory info for a job.
func (da *DumpAnalyzer) RegisterJob(jobName, moduleName, dir string) {
	da.mu.Lock()
	defer da.mu.Unlock()
	if dir == "" {
		return
	}
	if da.jobDirs == nil {
		da.jobDirs = make(map[string]string)
	}
	da.jobDirs[jobName] = dir
	if da.jobDone == nil {
		da.jobDone = make(map[string]bool)
	}
	da.jobDone[jobName] = false
	// Ensure expected sub-directories exist
	_ = os.MkdirAll(filepath.Join(dir, "queries"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "rows"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "metrics"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "meta"), 0o755)
}

// RecordJobStructure records the initial chart structure for a job
func (da *DumpAnalyzer) RecordJobStructure(jobName, moduleName string, charts *collectorapi.Charts) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job := &JobAnalysis{
		Name:           jobName,
		Module:         moduleName,
		Charts:         make([]ChartAnalysis, 0),
		AllSeenMetrics: make(map[string]bool),
	}

	// Copy chart structure
	for _, chart := range *charts {
		ca := ChartAnalysis{
			Chart:           chart,
			CollectedValues: make(map[string][]int64),
			SeenDimensions:  make(map[string]bool),
		}

		// Initialize dimension tracking
		for _, dim := range chart.Dims {
			ca.CollectedValues[dim.ID] = make([]int64, 0)
			ca.SeenDimensions[dim.ID] = false
		}

		job.Charts = append(job.Charts, ca)
	}

	da.jobs[jobName] = job
	da.writeJobMetadata(jobName, moduleName)
}

// UpdateJobStructure updates the chart structure for a job with current charts
// This is needed for collectors that create charts dynamically during collection
func (da *DumpAnalyzer) UpdateJobStructure(jobName string, charts *collectorapi.Charts) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job, exists := da.jobs[jobName]
	if !exists {
		return // Job not found, cannot update
	}

	// Create a map of existing chart data to preserve collected values
	existingCharts := make(map[string]*ChartAnalysis)
	for i := range job.Charts {
		existingCharts[job.Charts[i].Chart.ID] = &job.Charts[i]
	}

	// Rebuild chart list while preserving existing data
	job.Charts = make([]ChartAnalysis, 0)

	// Copy current chart structure
	for _, chart := range *charts {
		var ca ChartAnalysis

		// Check if we have existing data for this chart
		if existing, exists := existingCharts[chart.ID]; exists {
			// Preserve existing chart analysis but update the chart reference
			ca = *existing
			ca.Chart = chart

			// Add any new dimensions that weren't tracked before
			for _, dim := range chart.Dims {
				if _, tracked := ca.CollectedValues[dim.ID]; !tracked {
					ca.CollectedValues[dim.ID] = make([]int64, 0)
					ca.SeenDimensions[dim.ID] = false
				}
			}
		} else {
			// New chart - create fresh tracking
			ca = ChartAnalysis{
				Chart:           chart,
				CollectedValues: make(map[string][]int64),
				SeenDimensions:  make(map[string]bool),
			}

			// Initialize dimension tracking
			for _, dim := range chart.Dims {
				ca.CollectedValues[dim.ID] = make([]int64, 0)
				ca.SeenDimensions[dim.ID] = false
			}
		}

		job.Charts = append(job.Charts, ca)
	}
}

// RecordCollection records collected metrics directly from structured data
func (da *DumpAnalyzer) RecordCollection(jobName string, mx map[string]int64) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job, exists := da.jobs[jobName]
	if !exists {
		return
	}

	job.CollectionCount++
	job.LastCollection = time.Now()

	// Track ALL metrics in mx map
	for metricID := range mx {
		job.AllSeenMetrics[metricID] = true
	}

	// Record values for each chart
	for i := range job.Charts {
		ca := &job.Charts[i]

		// Check each dimension in this chart
		for _, dim := range ca.Chart.Dims {
			if value, collected := mx[dim.ID]; collected {
				ca.SeenDimensions[dim.ID] = true
				ca.CollectedValues[dim.ID] = append(ca.CollectedValues[dim.ID], value)
			}
		}
	}

	da.writeMetrics(jobName, job.CollectionCount, mx)
	da.markJobCollected(jobName)
}

func (da *DumpAnalyzer) writeJobMetadata(jobName, moduleName string) {
	if da.dataDir == "" {
		return
	}
	dir, ok := da.jobDirs[jobName]
	if !ok || dir == "" {
		return
	}
	meta := struct {
		Job      string            `json:"job"`
		Module   string            `json:"module"`
		Created  time.Time         `json:"created_at"`
		Metadata map[string]string `json:"metadata"`
	}{
		Job:     jobName,
		Module:  moduleName,
		Created: time.Now(),
		Metadata: map[string]string{
			"module": moduleName,
		},
	}
	path := filepath.Join(dir, "meta", "job.json")
	_ = writeJSON(path, meta)
}

func (da *DumpAnalyzer) writeMetrics(jobName string, seq int, mx map[string]int64) {
	if da.dataDir == "" {
		return
	}
	dir, ok := da.jobDirs[jobName]
	if !ok || dir == "" {
		return
	}
	metricsDir := filepath.Join(dir, "metrics")
	_ = os.MkdirAll(metricsDir, 0o755)
	payload := struct {
		CollectedAt time.Time        `json:"collected_at"`
		Metrics     map[string]int64 `json:"metrics"`
	}{
		CollectedAt: time.Now(),
		Metrics:     mx,
	}
	filename := fmt.Sprintf("metrics-%04d.json", seq)
	path := filepath.Join(metricsDir, filename)
	_ = writeJSON(path, payload)
}

func (da *DumpAnalyzer) markJobCollected(jobName string) {
	if da.dataDir == "" {
		return
	}
	if da.jobDone == nil {
		return
	}
	da.jobDone[jobName] = true
	for job, dir := range da.jobDirs {
		if dir == "" {
			continue
		}
		if !da.jobDone[job] {
			return
		}
	}
	if da.completed {
		return
	}
	da.completed = true
	da.writeManifest()
	if da.onComplete != nil {
		go da.onComplete()
	}
}

func (da *DumpAnalyzer) writeManifest() {
	if da.dataDir == "" {
		return
	}
	type manifestJob struct {
		Name        string `json:"name"`
		Module      string `json:"module"`
		Directory   string `json:"directory"`
		Collections int    `json:"collections"`
	}
	var jobs []manifestJob
	for name, job := range da.jobs {
		dir := da.jobDirs[name]
		jobs = append(jobs, manifestJob{
			Name:        name,
			Module:      job.Module,
			Directory:   dir,
			Collections: job.CollectionCount,
		})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Name < jobs[j].Name })
	manifest := struct {
		GeneratedAt time.Time     `json:"generated_at"`
		Jobs        []manifestJob `json:"jobs"`
	}{
		GeneratedAt: time.Now(),
		Jobs:        jobs,
	}
	_ = writeJSON(filepath.Join(da.dataDir, "manifest.json"), manifest)
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// contextInfo holds information about a context within a family
