// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// DumpAnalyzer collects and analyzes metric structure from dump mode
type DumpAnalyzer struct {
	mu        sync.RWMutex
	jobs      map[string]*JobAnalysis // key: job name
	startTime time.Time
}

// JobAnalysis holds analysis for a single job
type JobAnalysis struct {
	Name     string
	Module   string
	Families map[string]*FamilyAnalysis // key: family name
}

// FamilyAnalysis holds analysis for a chart family
type FamilyAnalysis struct {
	Name     string
	Contexts map[string]*ContextAnalysis // key: context
}

// ContextAnalysis holds analysis for a single context
type ContextAnalysis struct {
	Context   string
	Title     string
	Units     string
	ChartType string
	Priority  int
	Instances map[string]*InstanceAnalysis // key: instance ID
}

// InstanceAnalysis holds analysis for a single instance
type InstanceAnalysis struct {
	ID         string
	Name       string
	Labels     map[string]string
	Dimensions map[string]*DimensionAnalysis // key: dimension ID

	// Track redefinitions
	Definitions []InstanceDefinition
}

// InstanceDefinition tracks how an instance was defined
type InstanceDefinition struct {
	Timestamp time.Time
	Title     string
	Units     string
	Labels    map[string]string
}

// DimensionAnalysis holds analysis for a single dimension
type DimensionAnalysis struct {
	ID         string
	Name       string
	Algorithm  string
	Multiplier int
	Divisor    int

	// Collection statistics
	Collections         int
	EmptyCollections    int
	NonEmptyCollections int
	Values              []int64
}

// NewDumpAnalyzer creates a new dump analyzer
func NewDumpAnalyzer() *DumpAnalyzer {
	return &DumpAnalyzer{
		jobs:      make(map[string]*JobAnalysis),
		startTime: time.Now(),
	}
}

// NewDumpWriter creates a dump writer for a specific job
func (da *DumpAnalyzer) NewDumpWriter(jobName, module string) io.Writer {
	return NewDumpWriter(da, jobName, module)
}

// RecordChart records a chart definition
func (da *DumpAnalyzer) RecordChart(jobName, module, family, context, chartID, title, units, chartType string, priority int) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job := da.getOrCreateJob(jobName, module)
	fam := da.getOrCreateFamily(job, family)
	ctx := da.getOrCreateContext(fam, context, title, units, chartType, priority)

	// Chart ID becomes the instance ID
	if _, exists := ctx.Instances[chartID]; !exists {
		ctx.Instances[chartID] = &InstanceAnalysis{
			ID:         chartID,
			Name:       title,
			Labels:     make(map[string]string),
			Dimensions: make(map[string]*DimensionAnalysis),
			Definitions: []InstanceDefinition{
				{
					Timestamp: time.Now(),
					Title:     title,
					Units:     units,
					Labels:    make(map[string]string),
				},
			},
		}
	} else {
		// Track redefinition
		inst := ctx.Instances[chartID]
		def := InstanceDefinition{
			Timestamp: time.Now(),
			Title:     title,
			Units:     units,
			Labels:    make(map[string]string),
		}

		// Check if this is a different definition
		lastDef := inst.Definitions[len(inst.Definitions)-1]
		if lastDef.Title != title || lastDef.Units != units {
			inst.Definitions = append(inst.Definitions, def)
		}
	}
}

// RecordDimension records a dimension definition
func (da *DumpAnalyzer) RecordDimension(jobName, family, context, chartID, dimID, dimName, algorithm string, multiplier, divisor int) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job := da.getOrCreateJob(jobName, "")
	fam := da.getOrCreateFamily(job, family)
	ctx := da.getOrCreateContext(fam, context, "", "", "", 0)
	inst := da.getOrCreateInstance(ctx, chartID)

	if _, exists := inst.Dimensions[dimID]; !exists {
		inst.Dimensions[dimID] = &DimensionAnalysis{
			ID:         dimID,
			Name:       dimName,
			Algorithm:  algorithm,
			Multiplier: multiplier,
			Divisor:    divisor,
			Values:     make([]int64, 0),
		}
	}
}

// RecordLabel records a chart label
func (da *DumpAnalyzer) RecordLabel(jobName, family, context, chartID, key, value string) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job := da.getOrCreateJob(jobName, "")
	fam := da.getOrCreateFamily(job, family)
	ctx := da.getOrCreateContext(fam, context, "", "", "", 0)
	inst := da.getOrCreateInstance(ctx, chartID)

	inst.Labels[key] = value

	// Update latest definition
	if len(inst.Definitions) > 0 {
		inst.Definitions[len(inst.Definitions)-1].Labels[key] = value
	}
}

// RecordCollection records a data collection
func (da *DumpAnalyzer) RecordCollection(jobName, family, context, chartID string, values map[string]int64) {
	da.mu.Lock()
	defer da.mu.Unlock()

	job := da.getOrCreateJob(jobName, "")
	fam := da.getOrCreateFamily(job, family)
	ctx := da.getOrCreateContext(fam, context, "", "", "", 0)
	inst := da.getOrCreateInstance(ctx, chartID)

	// Record collection for each dimension
	for dimID, value := range values {
		if dim, exists := inst.Dimensions[dimID]; exists {
			dim.Collections++
			if value != 0 {
				dim.NonEmptyCollections++
			} else {
				dim.EmptyCollections++
			}

			// Store up to 100 values for analysis
			if len(dim.Values) < 100 {
				dim.Values = append(dim.Values, value)
			}
		}
	}

	// Record dimensions that were not collected (empty)
	for dimID, dim := range inst.Dimensions {
		if _, collected := values[dimID]; !collected {
			dim.Collections++
			dim.EmptyCollections++
		}
	}
}

// Helper methods
func (da *DumpAnalyzer) getOrCreateJob(name, module string) *JobAnalysis {
	if job, exists := da.jobs[name]; exists {
		if module != "" && job.Module == "" {
			job.Module = module
		}
		return job
	}

	job := &JobAnalysis{
		Name:     name,
		Module:   module,
		Families: make(map[string]*FamilyAnalysis),
	}
	da.jobs[name] = job
	return job
}

func (da *DumpAnalyzer) getOrCreateFamily(job *JobAnalysis, family string) *FamilyAnalysis {
	if fam, exists := job.Families[family]; exists {
		return fam
	}

	fam := &FamilyAnalysis{
		Name:     family,
		Contexts: make(map[string]*ContextAnalysis),
	}
	job.Families[family] = fam
	return fam
}

func (da *DumpAnalyzer) getOrCreateContext(fam *FamilyAnalysis, context, title, units, chartType string, priority int) *ContextAnalysis {
	if ctx, exists := fam.Contexts[context]; exists {
		// Update fields if provided
		if title != "" {
			ctx.Title = title
		}
		if units != "" {
			ctx.Units = units
		}
		if chartType != "" {
			ctx.ChartType = chartType
		}
		if priority > 0 {
			ctx.Priority = priority
		}
		return ctx
	}

	ctx := &ContextAnalysis{
		Context:   context,
		Title:     title,
		Units:     units,
		ChartType: chartType,
		Priority:  priority,
		Instances: make(map[string]*InstanceAnalysis),
	}
	fam.Contexts[context] = ctx
	return ctx
}

func (da *DumpAnalyzer) getOrCreateInstance(ctx *ContextAnalysis, chartID string) *InstanceAnalysis {
	if inst, exists := ctx.Instances[chartID]; exists {
		return inst
	}

	inst := &InstanceAnalysis{
		ID:         chartID,
		Labels:     make(map[string]string),
		Dimensions: make(map[string]*DimensionAnalysis),
	}
	ctx.Instances[chartID] = inst
	return inst
}

// Analyze performs final analysis
func (da *DumpAnalyzer) Analyze() {
	// Analysis is done incrementally during recording
	// This method could be used for final computations if needed
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

	// Print summary of issues
	da.printIssuesSummary()
}

func (da *DumpAnalyzer) printJobAnalysis(job *JobAnalysis) {
	fmt.Printf("\n\nJOB: %s (module: %s)\n", job.Name, job.Module)
	fmt.Println(strings.Repeat("-", 80))

	// Sort families
	var families []string
	for name := range job.Families {
		families = append(families, name)
	}
	sort.Strings(families)

	// Build hierarchy
	fmt.Println("\nCHART HIERARCHY:")
	for _, famName := range families {
		fam := job.Families[famName]
		fmt.Printf("├── %s\n", famName)

		// Sort contexts by priority
		type ctxPrio struct {
			name     string
			priority int
			ctx      *ContextAnalysis
		}
		var contexts []ctxPrio
		for name, ctx := range fam.Contexts {
			contexts = append(contexts, ctxPrio{name, ctx.Priority, ctx})
		}
		sort.Slice(contexts, func(i, j int) bool {
			return contexts[i].priority < contexts[j].priority
		})

		for i, cp := range contexts {
			prefix := "│   ├──"
			if i == len(contexts)-1 {
				prefix = "│   └──"
			}

			// Analyze instance type
			instanceType := da.inferInstanceType(cp.ctx)
			instanceCount := len(cp.ctx.Instances)

			fmt.Printf("%s %s [%s] %s - %d instances (%s)\n",
				prefix, cp.name, cp.ctx.Units, cp.ctx.Title, instanceCount, instanceType)

			// Show dimension consistency
			if !da.checkDimensionConsistency(cp.ctx) {
				fmt.Printf("│       ⚠️  INCONSISTENT DIMENSIONS across instances\n")
			}

			// Show redefinition issues
			for _, inst := range cp.ctx.Instances {
				if len(inst.Definitions) > 1 {
					fmt.Printf("│       ⚠️  Instance '%s' redefined %d times\n", inst.ID, len(inst.Definitions))
				}
			}
		}
	}
}

func (da *DumpAnalyzer) inferInstanceType(ctx *ContextAnalysis) string {
	// Look at instance IDs/labels to infer type
	var sampleInstances []string
	for id := range ctx.Instances {
		sampleInstances = append(sampleInstances, id)
		if len(sampleInstances) >= 3 {
			break
		}
	}

	// Check common patterns
	lower := strings.ToLower(ctx.Context)
	if strings.Contains(lower, "thread") && strings.Contains(lower, "pool") {
		return "thread pools"
	}
	if strings.Contains(lower, "jdbc") {
		return "datasources"
	}
	if strings.Contains(lower, "servlet") {
		return "servlets"
	}
	if strings.Contains(lower, "session") {
		return "sessions"
	}

	return "instances"
}

func (da *DumpAnalyzer) checkDimensionConsistency(ctx *ContextAnalysis) bool {
	var refDims []string
	first := true

	for _, inst := range ctx.Instances {
		var dims []string
		for dimID := range inst.Dimensions {
			dims = append(dims, dimID)
		}
		sort.Strings(dims)

		if first {
			refDims = dims
			first = false
		} else {
			if !da.slicesEqual(refDims, dims) {
				return false
			}
		}
	}

	return true
}

func (da *DumpAnalyzer) slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (da *DumpAnalyzer) printIssuesSummary() {
	fmt.Println("\n\nISSUES SUMMARY:")
	fmt.Println(strings.Repeat("-", 80))

	issues := 0

	for jobName, job := range da.jobs {
		for famName, fam := range job.Families {
			for ctxName, ctx := range fam.Contexts {
				// Check dimension consistency
				if !da.checkDimensionConsistency(ctx) {
					fmt.Printf("⚠️  [%s/%s/%s] Inconsistent dimensions across instances\n",
						jobName, famName, ctxName)
					issues++
				}

				// Check redefinitions
				for instID, inst := range ctx.Instances {
					if len(inst.Definitions) > 1 {
						fmt.Printf("⚠️  [%s/%s/%s/%s] Instance redefined %d times\n",
							jobName, famName, ctxName, instID, len(inst.Definitions))
						issues++
					}

					// Check empty collections
					for dimID, dim := range inst.Dimensions {
						if dim.Collections > 0 && dim.NonEmptyCollections == 0 {
							fmt.Printf("⚠️  [%s/%s/%s/%s/%s] Dimension always empty (%d collections)\n",
								jobName, famName, ctxName, instID, dimID, dim.Collections)
							issues++
						}
					}
				}
			}
		}
	}

	if issues == 0 {
		fmt.Println("✓ No issues found")
	} else {
		fmt.Printf("\nTotal issues: %d\n", issues)
	}
}

// DumpWriter intercepts Netdata protocol commands and records them
type DumpWriter struct {
	analyzer *DumpAnalyzer
	jobName  string
	module   string

	// Current state
	currentChart   string
	currentFamily  string
	currentContext string
	inCollection   bool
	collectionData map[string]int64
}

// NewDumpWriter creates a new dump writer
func NewDumpWriter(analyzer *DumpAnalyzer, jobName, module string) *DumpWriter {
	return &DumpWriter{
		analyzer:       analyzer,
		jobName:        jobName,
		module:         module,
		collectionData: make(map[string]int64),
	}
}

// Write implements io.Writer interface
func (dw *DumpWriter) Write(p []byte) (n int, err error) {
	// Parse Netdata protocol commands
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		dw.parseCommand(line)
	}

	return len(p), nil
}

func (dw *DumpWriter) parseCommand(line string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "CHART":
		dw.parseChart(parts)
	case "DIMENSION":
		dw.parseDimension(parts)
	case "CLABEL":
		dw.parseLabel(parts)
	case "BEGIN":
		dw.beginCollection(parts)
	case "SET":
		dw.setValue(parts)
	case "SETEMPTY":
		dw.setEmpty(parts)
	case "END":
		dw.endCollection()
	}
}

func (dw *DumpWriter) parseChart(parts []string) {
	// CHART type.id name title units family context charttype priority update_every options plugin module
	if len(parts) < 10 {
		return
	}

	typeID := strings.Trim(parts[1], "'\"")
	idParts := strings.SplitN(typeID, ".", 2)
	if len(idParts) != 2 {
		return
	}

	chartID := idParts[1]
	title := strings.Trim(parts[3], "'\"")
	units := strings.Trim(parts[4], "'\"")
	family := strings.Trim(parts[5], "'\"")
	context := strings.Trim(parts[6], "'\"")
	chartType := strings.Trim(parts[7], "'\"")
	priority := 0
	fmt.Sscanf(parts[8], "%d", &priority)

	dw.currentChart = chartID
	dw.currentFamily = family
	dw.currentContext = context

	dw.analyzer.RecordChart(dw.jobName, dw.module, family, context, chartID, title, units, chartType, priority)
}

func (dw *DumpWriter) parseDimension(parts []string) {
	// DIMENSION id name algorithm multiplier divisor options
	if len(parts) < 6 {
		return
	}

	dimID := strings.Trim(parts[1], "'\"")
	dimName := strings.Trim(parts[2], "'\"")
	algorithm := strings.Trim(parts[3], "'\"")
	multiplier := 1
	divisor := 1
	fmt.Sscanf(parts[4], "%d", &multiplier)
	fmt.Sscanf(parts[5], "%d", &divisor)

	dw.analyzer.RecordDimension(dw.jobName, dw.currentFamily, dw.currentContext, dw.currentChart, dimID, dimName, algorithm, multiplier, divisor)
}

func (dw *DumpWriter) parseLabel(parts []string) {
	// CLABEL key value source
	if len(parts) < 3 {
		return
	}

	key := strings.Trim(parts[1], "'\"")
	value := strings.Trim(parts[2], "'\"")

	dw.analyzer.RecordLabel(dw.jobName, dw.currentFamily, dw.currentContext, dw.currentChart, key, value)
}

func (dw *DumpWriter) beginCollection(parts []string) {
	// BEGIN type.id microseconds
	if len(parts) < 2 {
		return
	}

	typeID := strings.Trim(parts[1], "'\"")
	idParts := strings.SplitN(typeID, ".", 2)
	if len(idParts) == 2 {
		dw.currentChart = idParts[1]
	}

	dw.inCollection = true
	dw.collectionData = make(map[string]int64)
}

func (dw *DumpWriter) setValue(parts []string) {
	// SET id = value  or  SET id (for empty/gap)
	if len(parts) < 2 || !dw.inCollection {
		return
	}

	dimID := strings.Trim(parts[1], "'\"")
	var value int64

	// Check if value is provided (SET id = value)
	if len(parts) >= 4 && parts[2] == "=" {
		fmt.Sscanf(parts[3], "%d", &value)
	}
	// If no value provided (SET id), value remains 0 which represents empty/gap

	dw.collectionData[dimID] = value
}

func (dw *DumpWriter) setEmpty(parts []string) {
	// SETEMPTY id
	if len(parts) < 2 || !dw.inCollection {
		return
	}

	dimID := strings.Trim(parts[1], "'\"")
	dw.collectionData[dimID] = 0 // Empty value
}

func (dw *DumpWriter) endCollection() {
	if !dw.inCollection {
		return
	}

	dw.analyzer.RecordCollection(dw.jobName, dw.currentFamily, dw.currentContext, dw.currentChart, dw.collectionData)
	dw.inCollection = false
}
