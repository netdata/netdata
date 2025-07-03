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
}

// contextInfo holds information about a context within a family
type contextInfo struct {
	family      string
	context     string
	charts      []*ChartAnalysis
	minPriority int
}

func (da *DumpAnalyzer) printJobAnalysis(job *JobAnalysis) {
	// First, check for duplicate chart IDs (SEVERE BUG)
	chartIDCounts := make(map[string]int)
	for i := range job.Charts {
		ca := &job.Charts[i]
		chartIDCounts[ca.Chart.ID]++
	}

	// Check for contexts appearing in multiple families (SEVERE BUG)
	contextToFamilies := make(map[string][]string)

	families := make(map[string]map[string]*contextInfo) // family -> context -> info
	familyMinPriority := make(map[string]int)

	// Track issues for summary
	contextIssues := make(map[string][]string)

	// Group charts and check for duplicate contexts
	for i := range job.Charts {
		ca := &job.Charts[i]
		family := ca.Chart.Fam
		if family == "" {
			family = "(no family)"
		}
		ctx := ca.Chart.Ctx

		// Track context to families mapping
		if _, exists := contextToFamilies[ctx]; !exists {
			contextToFamilies[ctx] = []string{}
		}
		if !contains(contextToFamilies[ctx], family) {
			contextToFamilies[ctx] = append(contextToFamilies[ctx], family)
		}

		// Initialize family if needed
		if _, exists := families[family]; !exists {
			families[family] = make(map[string]*contextInfo)
			familyMinPriority[family] = ca.Chart.Priority
		}

		// Update family minimum priority
		if ca.Chart.Priority < familyMinPriority[family] {
			familyMinPriority[family] = ca.Chart.Priority
		}

		// Initialize context if needed
		if _, exists := families[family][ctx]; !exists {
			families[family][ctx] = &contextInfo{
				family:      family,
				context:     ctx,
				charts:      []*ChartAnalysis{},
				minPriority: ca.Chart.Priority,
			}
		}

		// Update context minimum priority
		if ca.Chart.Priority < families[family][ctx].minPriority {
			families[family][ctx].minPriority = ca.Chart.Priority
		}

		families[family][ctx].charts = append(families[family][ctx].charts, ca)
	}

	// Check for severe bugs - duplicate chart IDs and contexts in multiple families
	fmt.Println("\n" + job.Name)

	// Report duplicate chart IDs first (most severe)
	for chartID, count := range chartIDCounts {
		if count > 1 {
			fmt.Printf("🔴 SEVERE BUG: Chart ID '%s' defined %d times - this causes data corruption!\n",
				chartID, count)
			contextIssues[chartID] = append(contextIssues[chartID],
				fmt.Sprintf("SEVERE BUG - chart ID defined %d times (data corruption)", count))
		}
	}

	// Report contexts in multiple families
	for ctx, fams := range contextToFamilies {
		if len(fams) > 1 {
			fmt.Printf("🔴 SEVERE BUG: Context '%s' appears in multiple families: %s\n",
				ctx, strings.Join(fams, ", "))
			contextIssues[ctx] = append(contextIssues[ctx],
				fmt.Sprintf("SEVERE BUG - appears in multiple families: %s", strings.Join(fams, ", ")))
		}
	}

	// Sort families by minimum priority
	var sortedFamilies []string
	for fam := range families {
		sortedFamilies = append(sortedFamilies, fam)
	}
	sort.Slice(sortedFamilies, func(i, j int) bool {
		return familyMinPriority[sortedFamilies[i]] < familyMinPriority[sortedFamilies[j]]
	})

	// Print analysis for each family
	for _, family := range sortedFamilies {
		fmt.Printf("\n├─ family: %s\n", family)

		// Sort contexts by minimum priority
		var sortedContexts []string
		for ctx := range families[family] {
			sortedContexts = append(sortedContexts, ctx)
		}
		sort.Slice(sortedContexts, func(i, j int) bool {
			return families[family][sortedContexts[i]].minPriority <
				families[family][sortedContexts[j]].minPriority
		})

		for i, ctx := range sortedContexts {
			isLast := i == len(sortedContexts)-1
			ctxInfo := families[family][ctx]
			issues := da.printContextAnalysis(ctxInfo, isLast)
			if len(issues) > 0 {
				contextIssues[ctx] = append(contextIssues[ctx], issues...)
			}
		}
	}

	// Print greppable summary
	fmt.Println("\n" + strings.Repeat("═", 80))
	fmt.Println("ISSUE SUMMARY (greppable)")
	fmt.Println(strings.Repeat("═", 80))

	issueCount := 0
	for ctx, issues := range contextIssues {
		if len(issues) > 0 {
			issueCount++
			fmt.Printf("IDENTIFIED ISSUES ON %s: %s\n", ctx, strings.Join(issues, " | "))
		}
	}

	if issueCount == 0 {
		fmt.Println("🟢 NO ISSUES FOUND - All contexts are properly configured!")
	} else {
		fmt.Printf("🔴 TOTAL CONTEXTS WITH ISSUES: %d\n", issueCount)
	}
}

func (da *DumpAnalyzer) printContextAnalysis(ctxInfo *contextInfo, isLast bool) []string {
	charts := ctxInfo.charts
	var issues []string

	// Analyze titles
	titles := make(map[string]int)
	for _, ca := range charts {
		titles[ca.Chart.Title]++
	}

	// Analyze units
	units := make(map[string]int)
	for _, ca := range charts {
		units[ca.Chart.Units]++
	}

	// Analyze priorities
	priorities := make(map[int]int)
	for _, ca := range charts {
		priorities[ca.Chart.Priority]++
	}

	// Analyze label keys
	labelKeysByChart := make(map[string]map[string]bool) // chartID -> set of keys
	allLabelKeys := make(map[string]bool)
	for _, ca := range charts {
		labelKeysByChart[ca.Chart.ID] = make(map[string]bool)
		for _, label := range ca.Chart.Labels {
			labelKeysByChart[ca.Chart.ID][label.Key] = true
			allLabelKeys[label.Key] = true
		}
	}

	// Analyze dimensions
	dimsByChart := make(map[string]map[string]*module.Dim) // chartID -> dimID -> dim
	allDimIDs := make(map[string]bool)
	for _, ca := range charts {
		dimsByChart[ca.Chart.ID] = make(map[string]*module.Dim)
		for _, dim := range ca.Chart.Dims {
			dimsByChart[ca.Chart.ID][dim.ID] = dim
			allDimIDs[dim.ID] = true
		}
	}

	// Tree prefixes
	ctxPrefix := "├─"
	treePrefix := "│  "
	if isLast {
		ctxPrefix = "└─"
		treePrefix = "   "
	}

	// Print context header
	fmt.Printf("%s ⚡ context: %s\n", ctxPrefix, ctxInfo.context)

	// Print titles
	if len(titles) == 1 {
		for title := range titles {
			fmt.Printf("%s   ├─ title: %s ✅\n", treePrefix, title)
		}
	} else {
		fmt.Printf("%s   ├─ title: ❌ INCONSISTENT (%d different titles)\n", treePrefix, len(titles))
		for title, count := range titles {
			fmt.Printf("%s   │     ├─ %s (in %d charts)\n", treePrefix, title, count)
		}
		issues = append(issues, fmt.Sprintf("inconsistent titles (%d different)", len(titles)))
	}

	// Print units
	if len(units) == 1 {
		for unit := range units {
			fmt.Printf("%s   ├─ units: %s ✅\n", treePrefix, unit)
		}
	} else {
		fmt.Printf("%s   ├─ units: ❌ INCONSISTENT (%d different units)\n", treePrefix, len(units))
		for unit, count := range units {
			fmt.Printf("%s   │     ├─ %s (in %d charts)\n", treePrefix, unit, count)
		}
		issues = append(issues, fmt.Sprintf("inconsistent units (%d different)", len(units)))
	}

	// Print priorities
	if len(priorities) == 1 {
		for priority := range priorities {
			fmt.Printf("%s   ├─ priority: %d ✅\n", treePrefix, priority)
		}
	} else {
		fmt.Printf("%s   ├─ priority: %d 🟡 INCONSISTENT (%d different priorities)\n",
			treePrefix, ctxInfo.minPriority, len(priorities))
		for priority, count := range priorities {
			fmt.Printf("%s   │     ├─ %d (in %d charts)\n", treePrefix, priority, count)
		}
		issues = append(issues, fmt.Sprintf("inconsistent priorities (%d different)", len(priorities)))
	}

	// Print label keys
	fmt.Printf("%s   ├─ label keys: ", treePrefix)
	if len(allLabelKeys) == 0 {
		fmt.Printf("(none) ✅\n")
	} else {
		var labelKeyList []string
		for key := range allLabelKeys {
			labelKeyList = append(labelKeyList, key)
		}
		sort.Strings(labelKeyList)

		var labelKeyStatus []string
		hasInconsistentLabels := false
		for _, key := range labelKeyList {
			allHaveIt := true
			for chartID := range labelKeysByChart {
				if !labelKeysByChart[chartID][key] {
					allHaveIt = false
					hasInconsistentLabels = true
					break
				}
			}
			if allHaveIt {
				labelKeyStatus = append(labelKeyStatus, fmt.Sprintf("%s✅", key))
			} else {
				labelKeyStatus = append(labelKeyStatus, fmt.Sprintf("%s❌", key))
			}
		}
		fmt.Printf("%s", strings.Join(labelKeyStatus, ", "))
		if hasInconsistentLabels {
			fmt.Printf(" 🟡 SOME MISSING")
			issues = append(issues, "inconsistent label keys")
		}
		fmt.Printf("\n")
	}

	// Print dimensions
	fmt.Printf("%s   ├─ dimensions:\n", treePrefix)
	var dimIDList []string
	for dimID := range allDimIDs {
		dimIDList = append(dimIDList, dimID)
	}
	sort.Strings(dimIDList)

	hasInactiveDims := false
	hasMissingDims := false

	for _, dimID := range dimIDList {
		// Check if all charts have this dimension
		allHaveIt := true
		var sampleDim *module.Dim
		activeCount := 0

		for _, ca := range charts {
			if dim, exists := dimsByChart[ca.Chart.ID][dimID]; exists {
				if sampleDim == nil {
					sampleDim = dim
				}
				// Check if dimension is active
				if ca.SeenDimensions[dimID] && len(ca.CollectedValues[dimID]) > 0 {
					activeCount++
				}
			} else {
				allHaveIt = false
				hasMissingDims = true
			}
		}

		if sampleDim != nil {
			mul := sampleDim.Mul
			if mul == 0 {
				mul = 1
			}
			div := sampleDim.Div
			if div == 0 {
				div = 1
			}

			emoji := "❌"
			if activeCount == len(charts) {
				emoji = "✅"
			} else if activeCount > 0 {
				emoji = "🟡"
				hasInactiveDims = true
			} else {
				hasInactiveDims = true
			}

			dimStatus := ""
			if !allHaveIt {
				dimStatus = " ❌ NOT IN ALL CHARTS"
			}

			fmt.Printf("%s   │     ├─ %s %s (%s) x%d ÷%d%s\n",
				treePrefix, emoji, dimID, sampleDim.Name, mul, div, dimStatus)
		}
	}

	if hasInactiveDims {
		issues = append(issues, "inactive dimensions")
	}
	if hasMissingDims {
		issues = append(issues, "missing dimensions in some charts")
	}

	// Print instances
	fmt.Printf("%s   └─ instances:\n", treePrefix)
	for i, ca := range charts {
		labelPairs := []string{}
		for _, label := range ca.Chart.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", label.Key, label.Value))
		}
		labelStr := ""
		if len(labelPairs) > 0 {
			labelStr = fmt.Sprintf(" {%s}", strings.Join(labelPairs, ", "))
		}

		// Extract name from ID if possible
		name := ""
		if ca.Chart.OverID != "" {
			name = ca.Chart.OverID
		}

		instPrefix := "├─"
		if i == len(charts)-1 {
			instPrefix = "└─"
		}

		fmt.Printf("%s       %s %s (%s)%s\n", treePrefix, instPrefix, ca.Chart.ID, name, labelStr)
	}

	return issues
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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
