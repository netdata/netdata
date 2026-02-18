// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

// MethodConfig describes a function method provided by a module.
type MethodConfig struct {
	ID             string        // Method ID (e.g., "top-queries")
	Name           string        // Display name (e.g., "Top Queries")
	UpdateEvery    int           // Default UI refresh interval
	Help           string        // Description for UI
	RequireCloud   bool          // Indicates whether the method requires cloud connection
	ResponseType   string        // Response schema type (default "table")
	RequiredParams []ParamConfig // Required parameters for this method (including __sort if used)
}

// FunctionResponse is the response from a module's HandleMethod.
type FunctionResponse struct {
	Status            int            // HTTP-like status code (200, 400, 403, 500, 503)
	Message           string         // Error message (if Status != 200)
	Help              string         // Help text for this response
	ResponseType      string         // Override response schema type (defaults to MethodConfig.ResponseType)
	Columns           map[string]any // Column definitions for the table
	Data              any            // Row data: [][]any (array of arrays, ordered by column index)
	DefaultSortColumn string         // Default sort column ID

	// Optional dynamic required params (override MethodConfig.RequiredParams)
	RequiredParams []ParamConfig

	// Chart configuration for visualization (embedded for JSON compatibility)
	ChartingConfig
}

// ChartingConfig groups chart visualization settings.
// Embedded in FunctionResponse - JSON fields are promoted to top level.
type ChartingConfig struct {
	Charts        map[string]ChartConfig   // Chart definitions (chartID -> config)
	DefaultCharts DefaultCharts            // Default charts to display
	GroupBy       map[string]GroupByConfig // Group-by options (groupByID -> config)
}

// DefaultChart represents a chart with its grouping.
type DefaultChart struct {
	Chart   string // Chart ID to display
	GroupBy string // Column to group by
}

// DefaultCharts is a list of default charts.
type DefaultCharts []DefaultChart

// Build converts DefaultCharts to [][]string for JSON response.
// Output format: [["chartID", "groupByID"], ...]
func (dc DefaultCharts) Build() [][]string {
	if len(dc) == 0 {
		return nil
	}
	result := make([][]string, len(dc))
	for i, c := range dc {
		result[i] = []string{c.Chart, c.GroupBy}
	}
	return result
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
