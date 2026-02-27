// SPDX-License-Identifier: GPL-3.0-or-later

package metricsaudit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type manifestJob struct {
	Name        string `json:"name"`
	Module      string `json:"module"`
	Directory   string `json:"directory"`
	Collections int    `json:"collections"`
}

type manifestPayload struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Jobs        []manifestJob `json:"jobs"`
}

func (da *Auditor) EnableDataCapture(dir string, onComplete func()) {
	da.mu.Lock()
	defer da.mu.Unlock()

	da.dataDir = dir
	da.onComplete = onComplete
	if dir == "" || da.writeCh != nil {
		return
	}

	da.writeCh = make(chan writeTask, writeQueueSize)
	go da.runWriter(da.writeCh)
}

func (da *Auditor) runWriter(ch <-chan writeTask) {
	for task := range ch {
		if task.flush != nil {
			close(task.flush)
			continue
		}

		if task.run != nil {
			if err := task.run(); err != nil {
				da.recordWriteError(fmt.Errorf("%s: %w", task.label, err))
			}
		}

		if task.after != nil {
			task.after()
		}
	}
}

// RegisterJob registers directory info for a job.
func (da *Auditor) RegisterJob(jobName, moduleName, dir string) {
	id := newJobID(moduleName, jobName)

	da.mu.Lock()
	if da.jobDirs == nil {
		da.jobDirs = make(map[JobID]string)
	}
	da.jobDirs[id] = dir
	if da.jobDone == nil {
		da.jobDone = make(map[JobID]bool)
	}
	da.jobDone[id] = false
	da.mu.Unlock()

	if dir == "" {
		return
	}

	for _, sub := range []string{"queries", "rows", "metrics", "meta"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			da.recordWriteError(fmt.Errorf("prepare %s directory for %s[%s]: %w", sub, moduleName, jobName, err))
		}
	}
}

// RecordJobStructure records the initial chart structure for a job.
func (da *Auditor) RecordJobStructure(jobName, moduleName string, charts *collectorapi.Charts) {
	if charts == nil {
		return
	}

	job := &JobAnalysis{
		Name:           jobName,
		Module:         moduleName,
		Charts:         make([]ChartAnalysis, 0, len(*charts)),
		AllSeenMetrics: make(map[string]bool),
	}

	for _, chart := range *charts {
		ca := ChartAnalysis{
			Chart:           chart,
			CollectedValues: make(map[string][]int64),
			SeenDimensions:  make(map[string]bool),
		}
		for _, dim := range chart.Dims {
			ca.CollectedValues[dim.ID] = make([]int64, 0)
			ca.SeenDimensions[dim.ID] = false
		}
		job.Charts = append(job.Charts, ca)
	}

	id := newJobID(moduleName, jobName)
	var dir string
	var captureEnabled bool

	da.mu.Lock()
	da.jobs[id] = job
	dir = da.jobDirs[id]
	captureEnabled = da.dataDir != "" && dir != ""
	da.mu.Unlock()

	if !captureEnabled {
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
	da.enqueueJSONWrite(
		fmt.Sprintf("write metadata for %s[%s]", moduleName, jobName),
		path,
		meta,
		nil,
	)
}

// UpdateJobStructure updates the chart structure for a job with current charts.
// This is needed for collectors that create charts dynamically during collection.
func (da *Auditor) UpdateJobStructure(jobName, moduleName string, charts *collectorapi.Charts) {
	if charts == nil {
		return
	}

	id := newJobID(moduleName, jobName)

	da.mu.Lock()
	defer da.mu.Unlock()

	job, exists := da.jobs[id]
	if !exists {
		return
	}

	existingCharts := make(map[string]*ChartAnalysis)
	for i := range job.Charts {
		existingCharts[job.Charts[i].Chart.ID] = &job.Charts[i]
	}

	job.Charts = make([]ChartAnalysis, 0, len(*charts))
	for _, chart := range *charts {
		var ca ChartAnalysis
		if existing, ok := existingCharts[chart.ID]; ok {
			ca = *existing
			ca.Chart = chart
			for _, dim := range chart.Dims {
				if _, tracked := ca.CollectedValues[dim.ID]; !tracked {
					ca.CollectedValues[dim.ID] = make([]int64, 0)
					ca.SeenDimensions[dim.ID] = false
				}
			}
		} else {
			ca = ChartAnalysis{
				Chart:           chart,
				CollectedValues: make(map[string][]int64),
				SeenDimensions:  make(map[string]bool),
			}
			for _, dim := range chart.Dims {
				ca.CollectedValues[dim.ID] = make([]int64, 0)
				ca.SeenDimensions[dim.ID] = false
			}
		}
		job.Charts = append(job.Charts, ca)
	}
}

// RecordCollection records collected metrics directly from structured data.
func (da *Auditor) RecordCollection(jobName, moduleName string, mx map[string]int64) {
	if mx == nil {
		return
	}

	id := newJobID(moduleName, jobName)

	var seq int
	var metricsDir string
	var metricsPath string
	var captureEnabled bool
	var manifest *manifestPayload
	var onComplete func()

	da.mu.Lock()
	job, exists := da.jobs[id]
	if !exists {
		da.mu.Unlock()
		return
	}

	job.CollectionCount++
	job.LastCollection = time.Now()
	seq = job.CollectionCount

	for metricID := range mx {
		job.AllSeenMetrics[metricID] = true
	}

	for i := range job.Charts {
		ca := &job.Charts[i]
		for _, dim := range ca.Chart.Dims {
			if value, collected := mx[dim.ID]; collected {
				ca.SeenDimensions[dim.ID] = true
				ca.CollectedValues[dim.ID] = append(ca.CollectedValues[dim.ID], value)
			}
		}
	}

	if dir := da.jobDirs[id]; da.dataDir != "" && dir != "" {
		captureEnabled = true
		metricsDir = filepath.Join(dir, "metrics")
		metricsPath = filepath.Join(metricsDir, fmt.Sprintf("metrics-%04d.json", seq))
	}

	manifest, onComplete = da.markJobCollectedLocked(id)
	da.mu.Unlock()

	if captureEnabled {
		payload := struct {
			CollectedAt time.Time        `json:"collected_at"`
			Metrics     map[string]int64 `json:"metrics"`
		}{
			CollectedAt: time.Now(),
			Metrics:     cloneIntMetrics(mx),
		}

		da.enqueueWriteTask(writeTask{
			label: fmt.Sprintf("write metrics snapshot for %s[%s]", moduleName, jobName),
			run: func() error {
				if err := os.MkdirAll(metricsDir, 0o755); err != nil {
					return err
				}
				return writeJSON(metricsPath, payload)
			},
		})
	}

	da.handleCompletionWrite(manifest, onComplete)
}

func (da *Auditor) handleCompletionWrite(manifest *manifestPayload, onComplete func()) {
	if onComplete == nil {
		return
	}
	if manifest == nil {
		go onComplete()
		return
	}

	da.mu.RLock()
	manifestPath := filepath.Join(da.dataDir, "manifest.json")
	da.mu.RUnlock()

	enqueued := da.enqueueJSONWrite("write audit manifest", manifestPath, manifest, func() {
		go onComplete()
	})
	if !enqueued {
		go onComplete()
	}
}

func (da *Auditor) markJobCollectedLocked(id JobID) (*manifestPayload, func()) {
	if da.jobDone == nil {
		return nil, nil
	}
	if _, tracked := da.jobDone[id]; !tracked {
		return nil, nil
	}

	da.jobDone[id] = true
	for jobID, dir := range da.jobDirs {
		if dir == "" {
			continue
		}
		if !da.jobDone[jobID] {
			return nil, nil
		}
	}

	if da.completed {
		return nil, nil
	}
	da.completed = true
	if da.dataDir == "" {
		return nil, da.onComplete
	}

	manifest := da.buildManifestLocked()
	return &manifest, da.onComplete
}

func (da *Auditor) buildManifestLocked() manifestPayload {
	jobs := make([]manifestJob, 0, len(da.jobs))
	for id, job := range da.jobs {
		dir := da.jobDirs[id]
		jobs = append(jobs, manifestJob{
			Name:        id.Name,
			Module:      id.Module,
			Directory:   dir,
			Collections: job.CollectionCount,
		})
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Module == jobs[j].Module {
			return jobs[i].Name < jobs[j].Name
		}
		return jobs[i].Module < jobs[j].Module
	})

	return manifestPayload{
		GeneratedAt: time.Now(),
		Jobs:        jobs,
	}
}

func (da *Auditor) enqueueJSONWrite(label, path string, payload any, after func()) bool {
	return da.enqueueWriteTask(writeTask{
		label: label,
		run: func() error {
			return writeJSON(path, payload)
		},
		after: after,
	})
}

func (da *Auditor) enqueueWriteTask(task writeTask) bool {
	da.mu.RLock()
	ch := da.writeCh
	da.mu.RUnlock()
	if ch == nil {
		return false
	}

	select {
	case ch <- task:
		return true
	default:
		da.recordWriteError(fmt.Errorf("%s: write queue is full", task.label))
		return false
	}
}

func (da *Auditor) flushWriteQueue(timeout time.Duration) bool {
	da.mu.RLock()
	ch := da.writeCh
	da.mu.RUnlock()
	if ch == nil {
		return true
	}

	ack := make(chan struct{})
	task := writeTask{flush: ack}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ch <- task:
	case <-timer.C:
		da.recordWriteError(fmt.Errorf("flush write queue: timed out while enqueueing sentinel"))
		return false
	}

	timer.Reset(timeout)
	select {
	case <-ack:
		return true
	case <-timer.C:
		da.recordWriteError(fmt.Errorf("flush write queue: timed out waiting for sentinel"))
		return false
	}
}

func (da *Auditor) recordWriteError(err error) {
	if err == nil {
		return
	}

	da.mu.Lock()
	defer da.mu.Unlock()
	da.writeErrorCount++
	if len(da.writeErrors) < maxWriteErrorSamples {
		da.writeErrors = append(da.writeErrors, err.Error())
	}
}

func cloneIntMetrics(mx map[string]int64) map[string]int64 {
	if len(mx) == 0 {
		return map[string]int64{}
	}
	out := make(map[string]int64, len(mx))
	for k, v := range mx {
		out[k] = v
	}
	return out
}

func writeJSON(path string, payload any) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
