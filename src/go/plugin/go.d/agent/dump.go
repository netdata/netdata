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
	AllSeenMetrics  map[string]bool // Track ALL metrics seen in mx map
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
}

// UpdateJobStructure updates the chart structure for a job with current charts
// This is needed for collectors that create charts dynamically during collection
func (da *DumpAnalyzer) UpdateJobStructure(jobName string, charts *module.Charts) {
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

// PrintSummary prints a consolidated summary across all jobs
func (da *DumpAnalyzer) PrintSummary() {
	da.mu.RLock()
	defer da.mu.RUnlock()

	// First print the regular report
	da.PrintReport()

	// Then print the consolidated summary
	fmt.Println("\n" + strings.Repeat("═", 80))
	fmt.Println("CONSOLIDATED SUMMARY ACROSS ALL JOBS")
	fmt.Println(strings.Repeat("═", 80))

	// Collect all contexts across all jobs
	type contextSummary struct {
		family    string
		context   string
		title     string
		units     string
		priority  int
		chartType string
		labelKeys []string
		dimNames  []string
		instances int
		jobs      map[string]bool
	}

	contextMap := make(map[string]*contextSummary) // context -> summary

	for jobName, job := range da.jobs {
		for i := range job.Charts {
			ca := &job.Charts[i]
			
			ctx := ca.Chart.Ctx
			if _, exists := contextMap[ctx]; !exists {
				// Collect unique label keys
				labelKeysMap := make(map[string]bool)
				for _, label := range ca.Chart.Labels {
					labelKeysMap[label.Key] = true
				}
				labelKeys := []string{}
				for key := range labelKeysMap {
					labelKeys = append(labelKeys, key)
				}
				sort.Strings(labelKeys)

				// Collect unique dimension names
				dimNamesMap := make(map[string]bool)
				for _, dim := range ca.Chart.Dims {
					dimName := dim.Name
					if dimName == "" {
						dimName = dim.ID
					}
					dimNamesMap[dimName] = true
				}
				dimNames := []string{}
				for name := range dimNamesMap {
					dimNames = append(dimNames, name)
				}
				sort.Strings(dimNames)

				contextMap[ctx] = &contextSummary{
					family:    ca.Chart.Fam,
					context:   ctx,
					title:     ca.Chart.Title,
					units:     ca.Chart.Units,
					priority:  ca.Chart.Priority,
					chartType: ca.Chart.Type.String(),
					labelKeys: labelKeys,
					dimNames:  dimNames,
					instances: 0,
					jobs:      make(map[string]bool),
				}
			}

			// Update instance count and job tracking
			contextMap[ctx].instances++
			contextMap[ctx].jobs[jobName] = true

			// Update label keys and dimension names if needed
			for _, label := range ca.Chart.Labels {
				found := false
				for _, key := range contextMap[ctx].labelKeys {
					if key == label.Key {
						found = true
						break
					}
				}
				if !found {
					contextMap[ctx].labelKeys = append(contextMap[ctx].labelKeys, label.Key)
					sort.Strings(contextMap[ctx].labelKeys)
				}
			}

			for _, dim := range ca.Chart.Dims {
				dimName := dim.Name
				if dimName == "" {
					dimName = dim.ID
				}
				found := false
				for _, name := range contextMap[ctx].dimNames {
					if name == dimName {
						found = true
						break
					}
				}
				if !found {
					contextMap[ctx].dimNames = append(contextMap[ctx].dimNames, dimName)
					sort.Strings(contextMap[ctx].dimNames)
				}
			}
		}
	}

	// Group contexts by family
	familyMap := make(map[string][]*contextSummary)
	for _, cs := range contextMap {
		family := cs.family
		if family == "" {
			family = "(no family)"
		}
		familyMap[family] = append(familyMap[family], cs)
	}

	// Sort families and contexts
	var families []string
	for fam := range familyMap {
		families = append(families, fam)
	}
	sort.Strings(families)

	// Print summary with tree structure using colons
	for i, family := range families {
		if i == 0 {
			fmt.Printf("\n┌─ family: %s\n", family)
		} else {
			fmt.Printf("\n├─ family: %s\n", family)
		}
		
		// Sort contexts by priority
		contexts := familyMap[family]
		sort.Slice(contexts, func(i, j int) bool {
			return contexts[i].priority < contexts[j].priority
		})

		for j, cs := range contexts {
			isLastContext := j == len(contexts)-1
			contextPrefix := "├──"
			detailPrefix := "│   ├─"
			lastDetailPrefix := "│   └─"
			
			if isLastContext {
				contextPrefix = "└──"
				detailPrefix = "    ├─"
				lastDetailPrefix = "    └─"
			}
			
			fmt.Printf("│  %s context: %s, unit: %s, prio: %d, type: %s\n",
				contextPrefix, cs.context, cs.units, cs.priority, cs.chartType)
			fmt.Printf("│  %s title: %s\n", detailPrefix, cs.title)
			
			if len(cs.labelKeys) > 0 {
				fmt.Printf("│  %s labels: %s\n", detailPrefix, strings.Join(cs.labelKeys, ", "))
			} else {
				fmt.Printf("│  %s labels: (none)\n", detailPrefix)
			}
			
			fmt.Printf("│  %s dimensions: %s\n", detailPrefix, strings.Join(cs.dimNames, ", "))
			fmt.Printf("│  %s instances: %d, jobs: %d\n", lastDetailPrefix, cs.instances, len(cs.jobs))
		}
	}
	
	// Add a bottom border for the last family
	if len(families) > 0 {
		fmt.Println("└─────────────────────────────────────────────────────────────")
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

	// Check for duplicate dimension IDs across ALL charts (SEVERE BUG)
	allDimIDs := make(map[string][]string) // dimID -> []chartIDs
	for i := range job.Charts {
		ca := &job.Charts[i]
		for _, dim := range ca.Chart.Dims {
			if _, exists := allDimIDs[dim.ID]; !exists {
				allDimIDs[dim.ID] = []string{}
			}
			allDimIDs[dim.ID] = append(allDimIDs[dim.ID], ca.Chart.ID)
		}
	}

	// Report duplicate dimension IDs
	for dimID, chartIDs := range allDimIDs {
		if len(chartIDs) > 1 {
			fmt.Printf("🔴 SEVERE BUG: Dimension ID '%s' is used in %d charts: %s\n",
				dimID, len(chartIDs), strings.Join(chartIDs, ", "))
			// Add to issues for each affected context
			for _, chartID := range chartIDs {
				// Find the context for this chart
				for i := range job.Charts {
					if job.Charts[i].Chart.ID == chartID {
						ctx := job.Charts[i].Chart.Ctx
						contextIssues[ctx] = append(contextIssues[ctx],
							fmt.Sprintf("SEVERE BUG - dimension ID '%s' is shared with charts: %s", dimID, strings.Join(chartIDs, ", ")))
						break
					}
				}
			}
		}
	}

	// Proper excess metrics analysis
	da.analyzeMetricDimensionMatching(job, allDimIDs, contextIssues)

	// Family structure analysis
	da.analyzeFamilyStructureForJob(job, contextIssues)

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
		fmt.Printf("\n├─ family= %s\n", family)

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

	errorCount := 0
	warningCount := 0
	for ctx, issues := range contextIssues {
		if len(issues) > 0 {
			for _, issue := range issues {
				emoji := "❌"
				if strings.Contains(issue, "WARNING") {
					emoji = "🟡"
					warningCount++
				} else if strings.Contains(issue, "SEVERE BUG") {
					emoji = "🔴"
					errorCount++
				} else {
					errorCount++
				}
				fmt.Printf("%s IDENTIFIED ISSUES ON %s: %s\n", emoji, ctx, issue)
			}
		}
	}
	
	issueCount := errorCount // Only count real errors for final status

	// Calculate statistics for the summary independently to avoid interfering with tree logic
	statsFamilies := make(map[string]bool)
	statsContexts := make(map[string]bool)
	statsInstances := 0
	statsTimeSeries := 0
	statsCollectedValues := 0
	
	// Calculate distinct {context}.{dimension} combinations
	uniqueContextDimensions := make(map[string]bool)

	for i := range job.Charts {
		ca := &job.Charts[i]
		statsInstances++

		// Track unique families and contexts for stats
		family := ca.Chart.Fam
		if family == "" {
			family = "(no family)"
		}
		statsFamilies[family] = true
		statsContexts[ca.Chart.Ctx] = true

		// Count dimensions (time-series) and track distinct {context}.{dimension} combinations
		statsTimeSeries += len(ca.Chart.Dims)
		for _, dim := range ca.Chart.Dims {
			statsCollectedValues += len(ca.CollectedValues[dim.ID])
			// Use dimension name for display, fall back to ID if name is empty
			dimName := dim.Name
			if dimName == "" {
				dimName = dim.ID
			}
			// Create unique key as context.dimension
			uniqueKey := fmt.Sprintf("%s.%s", ca.Chart.Ctx, dimName)
			uniqueContextDimensions[uniqueKey] = true
		}
	}

	// Count total distinct {context}.{dimension} combinations
	statsDistinctDimensions := len(uniqueContextDimensions)

	// Count unique metrics in mx map
	uniqueMetricsInMx := len(job.AllSeenMetrics)

	// Generate summary with detailed stats
	warningText := ""
	if warningCount > 0 {
		warningText = fmt.Sprintf(", %d warnings", warningCount)
	}
	
	if issueCount == 0 && statsTimeSeries == uniqueMetricsInMx {
		if warningCount > 0 {
			fmt.Printf("🟢 NO ISSUES FOUND%s, job %s defines: %d families, %d contexts, %d dimensions, %d instances, %d time-series, collects: %d unique metrics\n",
				warningText, job.Name, len(statsFamilies), len(statsContexts), statsDistinctDimensions, statsInstances, statsTimeSeries, uniqueMetricsInMx)
		} else {
			fmt.Printf("🟢 NO ISSUES FOUND, job %s defines: %d families, %d contexts, %d dimensions, %d instances, %d time-series, collects: %d unique metrics\n",
				job.Name, len(statsFamilies), len(statsContexts), statsDistinctDimensions, statsInstances, statsTimeSeries, uniqueMetricsInMx)
		}
	} else if issueCount == 0 && statsTimeSeries != uniqueMetricsInMx {
		// Mismatch between time-series and unique metrics even though no specific issues found
		fmt.Printf("🟡 DIMENSION MISMATCH%s, job %s defines: %d families, %d contexts, %d dimensions, %d instances, %d time-series, collects: %d unique metrics\n",
			warningText, job.Name, len(statsFamilies), len(statsContexts), statsDistinctDimensions, statsInstances, statsTimeSeries, uniqueMetricsInMx)
	} else {
		fmt.Printf("🔴 ISSUES FOUND%s, job %s defines: %d families, %d contexts, %d dimensions, %d instances, %d time-series, collects: %d unique metrics\n",
			warningText, job.Name, len(statsFamilies), len(statsContexts), statsDistinctDimensions, statsInstances, statsTimeSeries, uniqueMetricsInMx)
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
	fmt.Printf("%s ⚡ context= %s\n", ctxPrefix, ctxInfo.context)

	// Print titles
	if len(titles) == 1 {
		for title := range titles {
			fmt.Printf("%s   ├─ title= %s ✅\n", treePrefix, title)
		}
	} else {
		fmt.Printf("%s   ├─ title= ❌ INCONSISTENT (%d different titles)\n", treePrefix, len(titles))
		for title, count := range titles {
			fmt.Printf("%s   │     ├─ %s (in %d charts)\n", treePrefix, title, count)
		}
		issues = append(issues, fmt.Sprintf("inconsistent titles (%d different)", len(titles)))
	}

	// Print units, priority, and chart type on one line
	unitsStr := ""
	unitsEmoji := " ✅"
	if len(units) == 1 {
		for unit := range units {
			unitsStr = unit
		}
	} else {
		unitsStr = fmt.Sprintf("INCONSISTENT (%d different)", len(units))
		unitsEmoji = " ❌"
		issues = append(issues, fmt.Sprintf("inconsistent units (%d different)", len(units)))
	}

	priorityStr := ""
	priorityEmoji := " ✅"
	if len(priorities) == 1 {
		for priority := range priorities {
			priorityStr = fmt.Sprintf("%d", priority)
		}
	} else {
		priorityStr = fmt.Sprintf("%d (INCONSISTENT: %d different)", ctxInfo.minPriority, len(priorities))
		priorityEmoji = " 🟡"
		issues = append(issues, fmt.Sprintf("inconsistent priorities (%d different)", len(priorities)))
	}

	// Collect chart types
	chartTypes := make(map[string]int)
	for _, ca := range charts {
		chartTypes[ca.Chart.Type.String()]++
	}
	
	typeStr := ""
	typeEmoji := " ✅"
	if len(chartTypes) == 1 {
		for typ := range chartTypes {
			typeStr = typ
		}
	} else {
		typeStr = fmt.Sprintf("INCONSISTENT (%d different)", len(chartTypes))
		typeEmoji = " ❌"
		issues = append(issues, fmt.Sprintf("inconsistent chart types (%d different)", len(chartTypes)))
	}

	fmt.Printf("%s   ├─ units= %s%s, priority= %s%s, type= %s%s\n",
		treePrefix, unitsStr, unitsEmoji, priorityStr, priorityEmoji, typeStr, typeEmoji)

	// Print label keys
	fmt.Printf("%s   ├─ label keys= ", treePrefix)
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
			issues = append(issues, "WARNING - inconsistent label keys (natural for heterogeneous instances)")
		}
		fmt.Printf("\n")
	}

	// Collect all dimension names across all charts
	dimNamesByChart := make(map[string]map[string]string) // chartID -> dimName -> dimID
	allDimNames := make(map[string]bool)

	for _, ca := range charts {
		dimNamesByChart[ca.Chart.ID] = make(map[string]string)
		for _, dim := range ca.Chart.Dims {
			name := dim.Name
			if name == "" {
				name = dim.ID
			}
			dimNamesByChart[ca.Chart.ID][name] = dim.ID
			allDimNames[name] = true
		}
	}

	// Print dimensions (names only at context level)
	fmt.Printf("%s   ├─ dimensions=\n", treePrefix)
	var dimNameList []string
	for dimName := range allDimNames {
		dimNameList = append(dimNameList, dimName)
	}
	sort.Strings(dimNameList)

	// Check multipliers, dividers, and algorithms consistency across all charts for each dimension
	dimMultDivInfo := make(map[string]map[string][]int) // dimName -> "mul"/"div" -> []values
	dimAlgoInfo := make(map[string][]string) // dimName -> []algorithms
	contextAlgorithms := make(map[string]bool) // track all algorithms used in this context
	
	for dimName := range allDimNames {
		dimMultDivInfo[dimName] = map[string][]int{
			"mul": []int{},
			"div": []int{},
		}
		dimAlgoInfo[dimName] = []string{}
		
		// Collect all multipliers, dividers, and algorithms for this dimension name across charts
		for _, ca := range charts {
			for _, dim := range ca.Chart.Dims {
				name := dim.Name
				if name == "" {
					name = dim.ID
				}
				if name == dimName {
					// Treat 0 as 1 (default value)
					mul := dim.Mul
					if mul == 0 {
						mul = 1
					}
					div := dim.Div
					if div == 0 {
						div = 1
					}
					dimMultDivInfo[dimName]["mul"] = append(dimMultDivInfo[dimName]["mul"], mul)
					dimMultDivInfo[dimName]["div"] = append(dimMultDivInfo[dimName]["div"], div)
					
					// Collect algorithm
					algo := dim.Algo.String()
					dimAlgoInfo[dimName] = append(dimAlgoInfo[dimName], algo)
					contextAlgorithms[algo] = true
				}
			}
		}
	}
	
	// Check for mixed algorithms in the context
	if len(contextAlgorithms) > 1 {
		algoList := []string{}
		for algo := range contextAlgorithms {
			algoList = append(algoList, algo)
		}
		sort.Strings(algoList)
		issues = append(issues, fmt.Sprintf("mixed dimension algorithms (%s)", strings.Join(algoList, ", ")))
	}
	
	// Check for rate units with absolute algorithm
	if len(units) == 1 && len(contextAlgorithms) == 1 {
		for unit := range units {
			for algo := range contextAlgorithms {
				// Check if unit contains rate indicator (per second, per minute, etc.)
				if strings.Contains(unit, "/") && algo == "absolute" {
					issues = append(issues, fmt.Sprintf("WARNING - rate unit '%s' with absolute algorithm (should use incremental)", unit))
				}
			}
		}
	}
	
	// Check for generic units that indicate mixed metric types
	if len(units) == 1 {
		for unit := range units {
			lowerUnit := strings.ToLower(unit)
			// Check for generic counting units
			if lowerUnit == "value" || lowerUnit == "values" || 
			   lowerUnit == "count" || lowerUnit == "counts" ||
			   lowerUnit == "number" || lowerUnit == "numbers" ||
			   lowerUnit == "amount" || lowerUnit == "amounts" ||
			   lowerUnit == "quantity" || lowerUnit == "quantities" {
				issues = append(issues, fmt.Sprintf("WARNING - generic unit '%s' suggests mixed metric types (apples and oranges)", unit))
			}
		}
	}

	hasMissingDims := false
	hasMultDivInconsistency := false
	for i, dimName := range dimNameList {
		// Check if all charts have this dimension name
		allHaveIt := true
		for chartID := range dimNamesByChart {
			if _, exists := dimNamesByChart[chartID][dimName]; !exists {
				allHaveIt = false
				hasMissingDims = true
				break
			}
		}

		prefix := "├─"
		if i == len(dimNameList)-1 {
			prefix = "└─"
		}

		dimStatus := ""
		if !allHaveIt {
			dimStatus = " 🟡 NOT IN ALL CHARTS"
		}

		// Check multiplier/divider consistency
		mulValues := dimMultDivInfo[dimName]["mul"]
		divValues := dimMultDivInfo[dimName]["div"]
		algoValues := dimAlgoInfo[dimName]
		
		// Get unique multipliers, dividers, and algorithms
		uniqueMuls := make(map[int]bool)
		uniqueDivs := make(map[int]bool)
		uniqueAlgos := make(map[string]bool)
		for _, m := range mulValues {
			uniqueMuls[m] = true
		}
		for _, d := range divValues {
			uniqueDivs[d] = true
		}
		for _, a := range algoValues {
			uniqueAlgos[a] = true
		}
		
		// Format multiplier/divider/algorithm info
		multDivAlgoStr := ""
		multDivAlgoEmoji := " ✅"
		
		// Check consistency
		if len(uniqueMuls) > 1 || len(uniqueDivs) > 1 || len(uniqueAlgos) > 1 {
			hasMultDivInconsistency = true
			multDivAlgoEmoji = " ❌"
		}
		
		// Format the multiplier/divider/algorithm string - ALWAYS show them
		if len(uniqueMuls) == 1 && len(uniqueDivs) == 1 && len(uniqueAlgos) == 1 {
			var mul, div int
			var algo string
			for m := range uniqueMuls {
				mul = m
			}
			for d := range uniqueDivs {
				div = d
			}
			for a := range uniqueAlgos {
				algo = a
			}
			
			// Always show multiplier, divider, and algorithm
			multDivAlgoStr = fmt.Sprintf(" ×%d ÷%d %s", mul, div, algo)
		} else {
			// Show all variations if inconsistent
			parts := []string{}
			
			if len(uniqueMuls) == 1 {
				var mul int
				for m := range uniqueMuls {
					mul = m
				}
				parts = append(parts, fmt.Sprintf("×%d", mul))
			} else {
				mulStrs := []string{}
				for m := range uniqueMuls {
					mulStrs = append(mulStrs, fmt.Sprintf("%d", m))
				}
				parts = append(parts, fmt.Sprintf("×(%s)", strings.Join(mulStrs, ",")))
			}
			
			if len(uniqueDivs) == 1 {
				var div int
				for d := range uniqueDivs {
					div = d
				}
				parts = append(parts, fmt.Sprintf("÷%d", div))
			} else {
				divStrs := []string{}
				for d := range uniqueDivs {
					divStrs = append(divStrs, fmt.Sprintf("%d", d))
				}
				parts = append(parts, fmt.Sprintf("÷(%s)", strings.Join(divStrs, ",")))
			}
			
			if len(uniqueAlgos) == 1 {
				var algo string
				for a := range uniqueAlgos {
					algo = a
				}
				parts = append(parts, algo)
			} else {
				algoStrs := []string{}
				for a := range uniqueAlgos {
					algoStrs = append(algoStrs, a)
				}
				sort.Strings(algoStrs)
				parts = append(parts, fmt.Sprintf("(%s)", strings.Join(algoStrs, ",")))
			}
			
			multDivAlgoStr = fmt.Sprintf(" %s", strings.Join(parts, " "))
		}

		fmt.Printf("%s   │     %s %s%s%s%s\n", treePrefix, prefix, dimName, multDivAlgoStr, multDivAlgoEmoji, dimStatus)
	}

	if hasMissingDims {
		issues = append(issues, "WARNING - missing dimensions in some charts (natural for heterogeneous instances)")
	}
	
	if hasMultDivInconsistency {
		// Add detailed multiplier/divider inconsistency issues
		for dimName, info := range dimMultDivInfo {
			mulValues := info["mul"]
			divValues := info["div"]
			
			uniqueMuls := make(map[int]int)
			uniqueDivs := make(map[int]int)
			for _, m := range mulValues {
				uniqueMuls[m]++
			}
			for _, d := range divValues {
				uniqueDivs[d]++
			}
			
			if len(uniqueMuls) > 1 {
				mulStrs := []string{}
				for m, count := range uniqueMuls {
					mulStrs = append(mulStrs, fmt.Sprintf("%d (in %d charts)", m, count))
				}
				issues = append(issues, fmt.Sprintf("dimension '%s' has inconsistent multipliers: %s", dimName, strings.Join(mulStrs, ", ")))
			}
			
			if len(uniqueDivs) > 1 {
				divStrs := []string{}
				for d, count := range uniqueDivs {
					divStrs = append(divStrs, fmt.Sprintf("%d (in %d charts)", d, count))
				}
				issues = append(issues, fmt.Sprintf("dimension '%s' has inconsistent dividers: %s", dimName, strings.Join(divStrs, ", ")))
			}
		}
	}

	// Check if any dimensions are missing data across all instances
	missingDataDetails := []string{}
	for _, ca := range charts {
		for _, dim := range ca.Chart.Dims {
			if !ca.SeenDimensions[dim.ID] || len(ca.CollectedValues[dim.ID]) == 0 {
				dimName := dim.Name
				if dimName == "" {
					dimName = dim.ID
				}
				// Show both ID and name for clarity
				dimInfo := fmt.Sprintf("'%s'", dim.ID)
				if dim.Name != "" && dim.Name != dim.ID {
					dimInfo = fmt.Sprintf("'%s' ('%s')", dim.ID, dim.Name)
				}
				missingDataDetails = append(missingDataDetails, fmt.Sprintf("dimension %s on chart '%s' is not collected", dimInfo, ca.Chart.ID))
			}
		}
	}

	// Add all missing data issues
	issues = append(issues, missingDataDetails...)

	// Print instances
	fmt.Printf("%s   └─ instances=\n", treePrefix)
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
		instTreePrefix := "│  "
		if i == len(charts)-1 {
			instPrefix = "└─"
			instTreePrefix = "   "
		}

		fmt.Printf("%s       %s %s (%s)%s\n", treePrefix, instPrefix, ca.Chart.ID, name, labelStr)

		// Print dimension status for this instance
		for _, dim := range ca.Chart.Dims {
			dimName := dim.Name
			if dimName == "" {
				dimName = dim.ID
			}

			emoji := "❌"
			valueStr := ""
			if ca.SeenDimensions[dim.ID] && len(ca.CollectedValues[dim.ID]) > 0 {
				emoji = "✅"
				
				// Format sample values
				values := ca.CollectedValues[dim.ID]
				if len(values) > 5 {
					// Show first 3 and last 2 values for long series
					firstVals := []string{}
					for i := 0; i < 3; i++ {
						firstVals = append(firstVals, fmt.Sprintf("%d", values[i]))
					}
					lastVals := []string{}
					for i := len(values) - 2; i < len(values); i++ {
						lastVals = append(lastVals, fmt.Sprintf("%d", values[i]))
					}
					valueStr = fmt.Sprintf(": [%s, ..., %s] ", strings.Join(firstVals, ", "), strings.Join(lastVals, ", "))
				} else {
					// Show all values for short series
					valStrs := []string{}
					for _, v := range values {
						valStrs = append(valStrs, fmt.Sprintf("%d", v))
					}
					valueStr = fmt.Sprintf(": [%s] ", strings.Join(valStrs, ", "))
				}
			}

			// Format multiplier/divider and algorithm for this specific dimension
			mul := dim.Mul
			div := dim.Div
			// Treat 0 as 1 (what the framework does)
			if mul == 0 {
				mul = 1
			}
			if div == 0 {
				div = 1
			}
			
			// Get algorithm
			algo := string(dim.Algo)
			if algo == "" {
				algo = "absolute"
			}
			
			// Always show multiplier, divider and algorithm
			multDivAlgoStr := fmt.Sprintf(" ×%d ÷%d %s", mul, div, algo)

			fmt.Printf("%s       %s     %s %s%s%s %s\n", treePrefix, instTreePrefix, emoji, dimName, multDivAlgoStr, valueStr, dim.ID)
		}

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

// analyzeMetricDimensionMatching performs comprehensive analysis of dimension/metric matching
func (da *DumpAnalyzer) analyzeMetricDimensionMatching(job *JobAnalysis, allDimIDs map[string][]string, contextIssues map[string][]string) {
	// 1. Find duplicate dimension IDs across charts (already done above but let's be explicit)
	duplicateDimensions := []string{}
	for dimID, chartIDs := range allDimIDs {
		if len(chartIDs) > 1 {
			duplicateDimensions = append(duplicateDimensions, dimID)
			// Find affected contexts
			affectedContexts := make(map[string]bool)
			for _, chartID := range chartIDs {
				for i := range job.Charts {
					if job.Charts[i].Chart.ID == chartID {
						affectedContexts[job.Charts[i].Chart.Ctx] = true
						break
					}
				}
			}
			for ctx := range affectedContexts {
				contextIssues[ctx] = append(contextIssues[ctx],
					fmt.Sprintf("SEVERE BUG - dimension '%s' is used in multiple charts: %s", dimID, strings.Join(chartIDs, ", ")))
			}
		}
	}

	// 2. Get unique dimension IDs from charts
	chartDimensions := make(map[string]bool)
	for dimID := range allDimIDs {
		chartDimensions[dimID] = true
	}

	// 3. Get unique dimension IDs from values map (AllSeenMetrics)
	valuesDimensions := make(map[string]bool)
	for metricID := range job.AllSeenMetrics {
		valuesDimensions[metricID] = true
	}

	// 4. Find dimensions in charts but not in values (missing data)
	missingValues := []string{}
	for dimID := range chartDimensions {
		if !valuesDimensions[dimID] {
			missingValues = append(missingValues, dimID)
		}
	}

	// 5. Find dimensions in values but not in charts (excess metrics)
	excessMetrics := []string{}
	for metricID := range valuesDimensions {
		if !chartDimensions[metricID] {
			excessMetrics = append(excessMetrics, metricID)
		}
	}

	// Group missing values by context for reporting
	if len(missingValues) > 0 {
		contextMissingValues := make(map[string][]string)
		for _, dimID := range missingValues {
			// Find which context this dimension belongs to
			for i := range job.Charts {
				ca := &job.Charts[i]
				for _, dim := range ca.Chart.Dims {
					if dim.ID == dimID {
						contextMissingValues[ca.Chart.Ctx] = append(contextMissingValues[ca.Chart.Ctx], dimID)
						break
					}
				}
			}
		}

		for ctx, dims := range contextMissingValues {
			sort.Strings(dims)
			contextIssues[ctx] = append(contextIssues[ctx],
				fmt.Sprintf("dimensions %s in charts do not have collected values", strings.Join(dims, ", ")))
		}
	}

	// Report excess metrics
	if len(excessMetrics) > 0 {
		sort.Strings(excessMetrics)
		contextIssues["_general"] = append(contextIssues["_general"],
			fmt.Sprintf("dimensions %s in the values map, do not exist in charts", strings.Join(excessMetrics, ", ")))
	}

	// Print success messages with counts if no issues
	if len(duplicateDimensions) == 0 {
		fmt.Printf("✅ DIMENSION UNIQUENESS: All %d dimensions have unique IDs across charts\n", len(chartDimensions))
	}

	if len(missingValues) == 0 && len(excessMetrics) == 0 {
		fmt.Printf("✅ DIMENSION/VALUES MATCHING: %d chart dimensions perfectly match %d collected values\n",
			len(chartDimensions), len(valuesDimensions))
	} else {
		if len(missingValues) > 0 {
			fmt.Printf("❌ MISSING VALUES: %d chart dimensions have no collected values\n", len(missingValues))
		}
		if len(excessMetrics) > 0 {
			fmt.Printf("❌ EXCESS VALUES: %d collected values have no corresponding chart dimensions\n", len(excessMetrics))
		}
	}
}

// gcd calculates the greatest common divisor
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// analyzeFamilyStructureForJob performs family-level structural analysis for a single job
func (da *DumpAnalyzer) analyzeFamilyStructureForJob(job *JobAnalysis, contextIssues map[string][]string) {
	// Get all charts from this job
	allCharts := []*ChartAnalysis{}
	for i := range job.Charts {
		allCharts = append(allCharts, &job.Charts[i])
	}

	// Group charts by family
	type familyInfo struct {
		contexts      map[string][]*ChartAnalysis // context -> charts
		labelPairs    map[string]int              // "key=value" -> count
		hasSubfamilies bool
		subfamilies   map[string]bool
	}
	
	families := make(map[string]*familyInfo) // family -> info
	topLevelFamilies := make(map[string]bool)

	for _, ca := range allCharts {
		family := ca.Chart.Fam
		if family == "" {
			family = "(no family)"
		}

		// We'll check family depth later after all families are processed

		// Extract top-level family
		topLevel := family
		if idx := strings.Index(family, "/"); idx != -1 {
			topLevel = family[:idx]
		}
		topLevelFamilies[topLevel] = true

		// Initialize family info
		if _, exists := families[family]; !exists {
			families[family] = &familyInfo{
				contexts:    make(map[string][]*ChartAnalysis),
				labelPairs:  make(map[string]int),
				subfamilies: make(map[string]bool),
			}
		}

		// Track contexts
		ctx := ca.Chart.Ctx
		families[family].contexts[ctx] = append(families[family].contexts[ctx], ca)

		// Track label pairs
		for _, label := range ca.Chart.Labels {
			pair := fmt.Sprintf("%s=%s", label.Key, label.Value)
			families[family].labelPairs[pair]++
		}

		// Check for subfamilies
		if strings.Contains(family, "/") {
			parentFamily := family[:strings.Index(family, "/")]
			if _, exists := families[parentFamily]; !exists {
				families[parentFamily] = &familyInfo{
					contexts:    make(map[string][]*ChartAnalysis),
					labelPairs:  make(map[string]int),
					subfamilies: make(map[string]bool),
				}
			}
			families[parentFamily].hasSubfamilies = true
			families[parentFamily].subfamilies[family] = true
		}
	}

	// Rule 0: Check family depth (deferred until all families are processed)
	for family, info := range families {
		slashCount := strings.Count(family, "/")
		if slashCount > 2 {
			// Add to the first context in this family
			for ctx := range info.contexts {
				contextIssues[ctx] = append(contextIssues[ctx], 
					fmt.Sprintf("family '%s' exceeds maximum depth of 3 (has %d slashes); possible cause: over-nested hierarchy; possible fix: flatten to maximum 3 levels", family, slashCount))
				break
			}
		}
	}

	// Rule 1: Check label consistency within families
	for family, info := range families {
		if len(info.contexts) < 2 {
			continue // Skip single-context families
		}
		
		// Calculate total charts in this family
		totalCharts := 0
		for _, charts := range info.contexts {
			totalCharts += len(charts)
		}
		
		// Find inconsistent label pairs
		inconsistentPairs := []string{}
		
		// The base unit is the number of contexts in the family
		// Each label key-value pair should appear in multiples of this
		baseUnit := len(info.contexts)
		
		// Check that each label pair count is a multiple of the base unit
		for pair, actualCount := range info.labelPairs {
			if baseUnit > 0 && actualCount % baseUnit != 0 {
				inconsistentPairs = append(inconsistentPairs, fmt.Sprintf("'%s': %d", pair, actualCount))
			}
		}
		
		if len(inconsistentPairs) > 0 {
			// Limit to first 10 pairs for readability
			displayPairs := inconsistentPairs
			if len(inconsistentPairs) > 10 {
				displayPairs = inconsistentPairs[:10]
				displayPairs = append(displayPairs, fmt.Sprintf("... and %d more", len(inconsistentPairs)-10))
			}
			
			// Add to all contexts in this family
			for ctx := range info.contexts {
				contextIssues[ctx] = append(contextIssues[ctx], 
					fmt.Sprintf("family '%s' has inconsistent label pairs. Each key-value pair should appear in multiples of %d (the number of contexts), but got: %s; possible cause: not all instances have the same labels; possible fix: ensure all charts in the family have consistent labels or split into separate families", 
						family, baseUnit, strings.Join(displayPairs, ", ")))
			}
		}
	}

	// Rule 2: Check same number of instances per context in a family
	for family, info := range families {
		if len(info.contexts) > 1 {
			instanceCounts := make(map[int][]string)
			for ctx, charts := range info.contexts {
				count := len(charts)
				instanceCounts[count] = append(instanceCounts[count], ctx)
			}
			
			if len(instanceCounts) > 1 {
				details := []string{}
				for count, contexts := range instanceCounts {
					details = append(details, fmt.Sprintf("%d instances: %s", count, strings.Join(contexts, ", ")))
				}
				// Add to all contexts in this family
				for ctx := range info.contexts {
					contextIssues[ctx] = append(contextIssues[ctx], 
						fmt.Sprintf("family '%s' has different number of instances per context (%s); possible cause: monitoring different types of objects or missing data collection; possible fix: split into separate families or fix data collection", 
							family, strings.Join(details, "; ")))
				}
			}
		}
	}

	// Rule 3: Check snake_case contexts
	for _, ca := range allCharts {
		ctx := ca.Chart.Ctx
		if !isSnakeCase(ctx) {
			contextIssues[ctx] = append(contextIssues[ctx], 
				fmt.Sprintf("context '%s' is not in snake_case format; possible cause: incorrect naming convention; possible fix: use lowercase with underscores (e.g., 'my_metric_name')", ctx))
		}
	}

	// Rule 4: Check families with >10 contexts
	for family, info := range families {
		if len(info.contexts) > 10 {
			// Add to all contexts in this family
			for ctx := range info.contexts {
				contextIssues[ctx] = append(contextIssues[ctx], 
					fmt.Sprintf("family '%s' has %d contexts (exceeds recommended 10); possible cause: too many metric types in one family; possible fix: split into subfamilies or make some contexts into instances with labels", 
						family, len(info.contexts)))
			}
		}
	}

	// Rule 5: Check generic family names
	genericFamilies := map[string]bool{
		"other":          true,
		"infrastructure": true,
		"runtime":        true,
	}
	
	for family := range families {
		// Check only the base family name (before /)
		baseName := family
		if idx := strings.Index(family, "/"); idx != -1 {
			baseName = family[:idx]
		}
		
		if genericFamilies[strings.ToLower(baseName)] {
			// Add to all contexts in this family
			for ctx := range families[family].contexts {
				contextIssues[ctx] = append(contextIssues[ctx], 
					fmt.Sprintf("family '%s' uses generic name '%s'; possible cause: unclear categorization; possible fix: use specific names like 'database', 'webserver', 'messaging', etc.", 
						family, baseName))
			}
		}
	}

	// Rule 6: Check families with both direct contexts and subfamilies
	for family, info := range families {
		if len(info.contexts) > 0 && info.hasSubfamilies {
			// This is a parent family with both direct contexts and subfamilies
			if !strings.Contains(family, "/") {
				// Add to all contexts in this family
				for ctx := range info.contexts {
					contextIssues[ctx] = append(contextIssues[ctx], 
						fmt.Sprintf("family '%s' has both direct contexts and subfamilies; possible cause: mixed hierarchy; possible fix: move direct contexts to '%s/overview' or similar", 
							family, family))
				}
			}
		}
	}

	// Rule 7: Check top-level family count
	if len(topLevelFamilies) > 15 {
		familyList := []string{}
		for f := range topLevelFamilies {
			familyList = append(familyList, f)
		}
		sort.Strings(familyList)
		
		// Add to general issues (first context found)
		for _, ca := range allCharts {
			contextIssues[ca.Chart.Ctx] = append(contextIssues[ca.Chart.Ctx], 
				fmt.Sprintf("found %d top-level families (exceeds recommended 15): %s; possible cause: too many categories; possible fix: consolidate related families or use subfamilies", 
					len(topLevelFamilies), strings.Join(familyList, ", ")))
			break // Only add once
		}
	}

	// Rule 8: Check subfamily counts
	for family, info := range families {
		if !strings.Contains(family, "/") && info.hasSubfamilies {
			// This is a parent family, check its subfamilies
			subfamilyCount := len(info.subfamilies)
			
			// Check for singleton subfamily without siblings
			if subfamilyCount == 1 {
				// Get the single subfamily name
				var singleSubfamily string
				for sf := range info.subfamilies {
					singleSubfamily = sf
				}
				// Add to contexts in the parent family (if any) or the subfamily
				if len(info.contexts) > 0 {
					for ctx := range info.contexts {
						contextIssues[ctx] = append(contextIssues[ctx], 
							fmt.Sprintf("family '%s' has only one subfamily '%s'; possible cause: incomplete hierarchy; possible fix: either add more subfamilies or flatten the structure", 
								family, singleSubfamily))
					}
				} else {
					// Add to contexts in the single subfamily
					if subfamilyInfo, exists := families[singleSubfamily]; exists {
						for ctx := range subfamilyInfo.contexts {
							contextIssues[ctx] = append(contextIssues[ctx], 
								fmt.Sprintf("family '%s' has only one subfamily '%s'; possible cause: incomplete hierarchy; possible fix: either add more subfamilies or flatten the structure", 
									family, singleSubfamily))
						}
					}
				}
			}
			
			// Check for too many subfamilies
			if subfamilyCount > 5 {
				// Add to contexts in the parent family (if any) or all subfamily contexts
				if len(info.contexts) > 0 {
					for ctx := range info.contexts {
						contextIssues[ctx] = append(contextIssues[ctx], 
							fmt.Sprintf("family '%s' has %d subfamilies (exceeds recommended 5); possible cause: too many subcategories; possible fix: consolidate related subfamilies or create a deeper hierarchy", 
								family, subfamilyCount))
					}
				} else {
					// Add to one context from each subfamily
					for subfamily := range info.subfamilies {
						if subfamilyInfo, exists := families[subfamily]; exists {
							for ctx := range subfamilyInfo.contexts {
								contextIssues[ctx] = append(contextIssues[ctx], 
									fmt.Sprintf("family '%s' has %d subfamilies (exceeds recommended 5); possible cause: too many subcategories; possible fix: consolidate related subfamilies or create a deeper hierarchy", 
										family, subfamilyCount))
								break // Only add to one context per subfamily
							}
						}
					}
				}
			}
		}
	}
}

// isSnakeCase checks if a string is in snake_case format
func isSnakeCase(s string) bool {
	// Should be lowercase with underscores, dots allowed for contexts
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.') {
			return false
		}
	}
	return true
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
