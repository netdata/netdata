// SPDX-License-Identifier: GPL-3.0-or-later

package module

import (
	"context"
	"fmt"
)

const (
	UpdateEvery        = 1
	AutoDetectionRetry = 0
	Priority           = 70000
)

// Defaults is a set of module default parameters.
type Defaults struct {
	UpdateEvery        int
	AutoDetectionRetry int
	Priority           int
	Disabled           bool
}

// SortOption represents a sort option for function responses.
// ID is a semantic identifier used in requests (e.g., "total_time").
// Column is the actual SQL column name for ORDER BY (e.g., "total_exec_time").
type SortOption struct {
	ID      string // Semantic ID used in __sort parameter
	Column  string // Actual SQL column name for ORDER BY
	Label   string // Human-readable label for UI dropdown
	Default bool   // Is this the default selection?
}

// MethodConfig describes a function method provided by a module.
type MethodConfig struct {
	ID          string       // Method ID (e.g., "top-queries")
	Name        string       // Display name (e.g., "Top Queries")
	Help        string       // Description for UI
	SortOptions []SortOption // Available sort options for this method
}

// FunctionResponse is the response from a module's HandleMethod.
type FunctionResponse struct {
	Status            int            // HTTP-like status code (200, 400, 403, 500, 503)
	Message           string         // Error message (if Status != 200)
	Help              string         // Help text for this response
	Columns           map[string]any // Column definitions for the table
	Data              any            // Row data: [][]any (array of arrays, ordered by column index)
	DefaultSortColumn string         // Default sort column ID

	// Dynamic sort options based on detected database capabilities
	// If provided, these override the static MethodConfig.SortOptions
	SortOptions []SortOption

	// Chart configuration for visualization
	Charts        map[string]ChartConfig   // Chart definitions (chartID -> config)
	DefaultCharts [][]string               // Default charts: [[chartID, groupByID], ...]
	GroupBy       map[string]GroupByConfig // Group-by options (groupByID -> config)
}

// ChartConfig defines a chart for visualization.
type ChartConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`    // "stacked-bar", "line", etc.
	Columns []string `json:"columns"` // Column IDs to include in chart
}

// GroupByConfig defines a grouping option for function responses.
type GroupByConfig struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"` // Columns to group by
}

type (
	// Creator is a Job builder.
	// Optional function fields (Methods/HandleMethod) enable the FunctionProvider pattern:
	// modules that set these fields can expose data functions to the UI.
	Creator struct {
		Defaults
		Create          func() Module
		JobConfigSchema string
		Config          func() any

		// Optional: FunctionProvider fields for exposing data functions
		// If Methods is non-nil, this module provides functions
		Methods func() []MethodConfig

		// HandleMethod handles a function request for a specific job
		// ctx: context with timeout from function request
		// job: the job instance to query
		// method: the method name (e.g., "top-queries")
		// sortBy: the sort column ID (from SortOption.ID)
		// Returns: FunctionResponse with data or error
		HandleMethod func(ctx context.Context, job *Job, method, sortBy string) *FunctionResponse
	}
	// Registry is a collection of Creators.
	Registry map[string]Creator
)

// DefaultRegistry DefaultRegistry.
var DefaultRegistry = Registry{}

// Register registers a module in the DefaultRegistry.
func Register(name string, creator Creator) {
	DefaultRegistry.Register(name, creator)
}

// Register registers a module.
func (r Registry) Register(name string, creator Creator) {
	if _, ok := r[name]; ok {
		panic(fmt.Sprintf("%s is already in registry", name))
	}
	r[name] = creator
}

func (r Registry) Lookup(name string) (Creator, bool) {
	v, ok := r[name]
	return v, ok
}
