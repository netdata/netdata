// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// DumpAnalyzer collects and analyzes metric structure from dump mode
type DumpAnalyzer struct {
	mu        sync.RWMutex
	jobs      map[string]*JobAnalysis // key: job name
	startTime time.Time
}

// JobAnalysis holds analysis for a single job
type JobAnalysis struct {
	Name            string
	Module          string
	Charts          []ChartAnalysis
	CollectionCount int
	LastCollection  time.Time
}

// ChartAnalysis holds analysis for a single chart
type ChartAnalysis struct {
	Chart           *module.Chart
	CollectedValues map[string][]int64 // dimension ID -> collected values
	SeenDimensions  map[string]bool    // track which dimensions received data
}

// NewDumpAnalyzer creates a new dump analyzer
func NewDumpAnalyzer() *DumpAnalyzer {
	return &DumpAnalyzer{
		jobs:      make(map[string]*JobAnalysis),
		startTime: time.Now(),
	}
}

// RecordJobStructure records the initial chart structure for a job
func (da *DumpAnalyzer) RecordJobStructure(jobName, moduleName string, charts *module.Charts) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job := &JobAnalysis{
		Name:   jobName,
		Module: moduleName,
		Charts: make([]ChartAnalysis, 0),
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
}

// PrintReport prints the analysis report
func (da *DumpAnalyzer) PrintReport() {
	da.mu.RLock()
	defer da.mu.RUnlock()

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("NETDATA METRICS DUMP ANALYSIS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Duration: %v\n", time.Since(da.startTime))
	fmt.Printf("Jobs: %d\n", len(da.jobs))

	// Sort jobs for consistent output
	var jobNames []string
	for name := range da.jobs {
		jobNames = append(jobNames, name)
	}
	sort.Strings(jobNames)

	for _, jobName := range jobNames {
		job := da.jobs[jobName]
		da.printJobAnalysis(job)
	}

	// Print summary
	da.printSummary()
}

func (da *DumpAnalyzer) printJobAnalysis(job *JobAnalysis) {
	fmt.Printf("\n\nJOB: %s (module: %s)\n", job.Name, job.Module)
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Collections: %d\n", job.CollectionCount)
	if job.CollectionCount > 0 {
		fmt.Printf("Last collection: %v ago\n", time.Since(job.LastCollection))
	}

	// Build hierarchy by family
	families := make(map[string][]*ChartAnalysis)
	familyPriorities := make(map[string]int)

	for i := range job.Charts {
		ca := &job.Charts[i]
		family := ca.Chart.Fam
		if family == "" {
			family = "(no family)"
		}

		families[family] = append(families[family], ca)

		// Track minimum priority for family sorting
		if minPrio, exists := familyPriorities[family]; !exists || ca.Chart.Priority < minPrio {
			familyPriorities[family] = ca.Chart.Priority
		}
	}

	// Sort families by priority
	type familyPrio struct {
		name     string
		priority int
	}
	var sortedFamilies []familyPrio
	for fam, prio := range familyPriorities {
		sortedFamilies = append(sortedFamilies, familyPrio{fam, prio})
	}
	sort.Slice(sortedFamilies, func(i, j int) bool {
		return sortedFamilies[i].priority < sortedFamilies[j].priority
	})

	fmt.Println("\nCHART HIERARCHY:")
	for _, fp := range sortedFamilies {
		fmt.Printf("├── %s (priority: %d)\n", fp.name, fp.priority)

		// Sort charts within family by priority
		charts := families[fp.name]
		sort.Slice(charts, func(i, j int) bool {
			return charts[i].Chart.Priority < charts[j].Chart.Priority
		})

		for i, ca := range charts {
			prefix := "│   ├──"
			if i == len(charts)-1 {
				prefix = "│   └──"
			}

			// Build instance info
			instanceInfo := ""
			if len(ca.Chart.Labels) > 0 {
				var labels []string
				for _, label := range ca.Chart.Labels {
					labels = append(labels, fmt.Sprintf("%s=%s", label.Key, label.Value))
				}
				instanceInfo = fmt.Sprintf(" {%s}", strings.Join(labels, ", "))
			}

			// Count active dimensions
			activeDims := 0
			for dimID, seen := range ca.SeenDimensions {
				if seen && len(ca.CollectedValues[dimID]) > 0 {
					activeDims++
				}
			}

			fmt.Printf("%s %s [%s] - %s%s (%d/%d dims active)\n",
				prefix,
				ca.Chart.Ctx,
				ca.Chart.Units,
				ca.Chart.Title,
				instanceInfo,
				activeDims,
				len(ca.Chart.Dims))

			// Show dimension details if requested
			if activeDims < len(ca.Chart.Dims) {
				da.printInactiveDimensions(ca, "│       ")
			}
		}
	}
}

func (da *DumpAnalyzer) printInactiveDimensions(ca *ChartAnalysis, indent string) {
	var inactive []string
	for _, dim := range ca.Chart.Dims {
		if !ca.SeenDimensions[dim.ID] {
			inactive = append(inactive, dim.Name)
		}
	}

	if len(inactive) > 0 {
		fmt.Printf("%s⚠️  Inactive dimensions: %s\n", indent, strings.Join(inactive, ", "))
	}
}

func (da *DumpAnalyzer) printSummary() {
	fmt.Println("\n\nSUMMARY:")
	fmt.Println(strings.Repeat("-", 80))

	totalCharts := 0
	totalDimensions := 0
	activeDimensions := 0
	chartsWithIssues := 0

	for _, job := range da.jobs {
		totalCharts += len(job.Charts)

		for _, ca := range job.Charts {
			totalDimensions += len(ca.Chart.Dims)
			hasIssue := false

			for dimID, seen := range ca.SeenDimensions {
				if seen && len(ca.CollectedValues[dimID]) > 0 {
					activeDimensions++
				} else {
					hasIssue = true
				}
			}

			if hasIssue {
				chartsWithIssues++
			}
		}
	}

	fmt.Printf("Total charts: %d\n", totalCharts)
	fmt.Printf("Total dimensions: %d\n", totalDimensions)
	fmt.Printf("Active dimensions: %d (%.1f%%)\n", activeDimensions,
		float64(activeDimensions)*100/float64(totalDimensions))
	fmt.Printf("Charts with inactive dimensions: %d\n", chartsWithIssues)

	// Print chart count by context
	fmt.Println("\nCHARTS BY CONTEXT:")
	contextCounts := make(map[string]int)
	for _, job := range da.jobs {
		for _, ca := range job.Charts {
			contextCounts[ca.Chart.Ctx]++
		}
	}

	// Sort contexts
	var contexts []string
	for ctx := range contextCounts {
		contexts = append(contexts, ctx)
	}
	sort.Strings(contexts)

	for _, ctx := range contexts {
		fmt.Printf("  %s: %d charts\n", ctx, contextCounts[ctx])
	}
}

// PrintDebugInfo prints additional debug information
func (da *DumpAnalyzer) PrintDebugInfo() {
	da.mu.RLock()
	defer da.mu.RUnlock()

	fmt.Println("\n\nDEBUG INFORMATION:")
	fmt.Println(strings.Repeat("-", 80))

	for jobName, job := range da.jobs {
		fmt.Printf("\n[%s] Chart Structure:\n", jobName)

		for _, ca := range job.Charts {
			fmt.Printf("\nChart ID: %s\n", ca.Chart.ID)
			fmt.Printf("  Context: %s\n", ca.Chart.Ctx)
			fmt.Printf("  Title: %s\n", ca.Chart.Title)
			fmt.Printf("  Units: %s\n", ca.Chart.Units)
			fmt.Printf("  Family: %s\n", ca.Chart.Fam)
			fmt.Printf("  Type: %s\n", ca.Chart.Type)
			fmt.Printf("  Priority: %d\n", ca.Chart.Priority)

			if len(ca.Chart.Labels) > 0 {
				fmt.Printf("  Labels:\n")
				for _, label := range ca.Chart.Labels {
					fmt.Printf("    %s: %s\n", label.Key, label.Value)
				}
			}

			fmt.Printf("  Dimensions:\n")
			for _, dim := range ca.Chart.Dims {
				status := "INACTIVE"
				valueCount := 0
				if ca.SeenDimensions[dim.ID] {
					status = "ACTIVE"
					valueCount = len(ca.CollectedValues[dim.ID])
				}

				fmt.Printf("    %s (%s) - %s [%d values collected]\n",
					dim.ID, dim.Name, status, valueCount)

				// Show sample values if collected
				if valueCount > 0 {
					samples := ca.CollectedValues[dim.ID]
					if valueCount > 5 {
						fmt.Printf("      Sample values: %v ... %v\n",
							samples[:3], samples[valueCount-2:])
					} else {
						fmt.Printf("      Values: %v\n", samples)
					}
				}
			}
		}
	}
}
