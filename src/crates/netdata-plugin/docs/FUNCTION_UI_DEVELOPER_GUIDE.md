# Netdata Functions: Developer Guide

> **Note**: This is the practical developer guide. For the complete technical specification, see [FUNCTIONS_REFERENCE.md](FUNCTIONS_REFERENCE.md).

## Overview

This guide teaches you how to create Netdata functions that provide interactive data through the web UI. You'll learn to build both simple tables and advanced log explorers.

**What You'll Learn:**
- How to create simple table functions for system monitoring
- How to build log explorer functions with faceted search
- All column types, options, and their UI effects
- Query patterns, filtering, and aggregation
- Real-world examples and best practices

**Quick Navigation:**
- [Part 1: Simple Table Functions](#part-1-simple-table-functions) - Basic monitoring data
- [Part 2: Log Explorer Functions](#part-2-log-explorer-functions) - Historical data with search
- [Part 3: Complete Options Reference](#part-3-complete-options-reference) - Every option explained

---

# Part 1: Simple Table Functions

Simple table functions display current system state - processes, connections, services, etc. They're perfect for "top-like" views and system monitoring.

**Implementation Architecture:**
- **Frontend handles**: Filtering, search, facet counting, sorting
- **Backend provides**: Raw data, column definitions, optional charts
- **Performance**: Limited by browser processing (good for \<10k rows)
- **Query parameter**: Processed in frontend only (substring search)
- **Histograms**: Not supported

## Your First Simple Table Function

Start with this minimal working example:

```json
{
  "status": 200,
  "type": "table",
  "has_history": false,
  "help": "Shows system services",
  "data": [
    ["nginx", 25, "Running"],
    ["mysql", 67, "Stopped"], 
    ["redis", 91, "Running"]
  ],
  "columns": {
    "name": {
      "index": 0,
      "name": "Service",
      "type": "string",
      "unique_key": true
    },
    "cpu": {
      "index": 1, 
      "name": "CPU %",
      "type": "bar-with-integer",
      "max": 100
    },
    "status": {
      "index": 2,
      "name": "Status",
      "type": "string"
    }
  }
}
```

This creates a basic 3-column table showing service names, CPU usage bars, and status.

## Essential Features for Simple Tables

### 1. Required Fields

Every simple table function needs:

```json
{
  "status": 200,           // HTTP status (200 = success)
  "type": "table",         // Always "table"
  "has_history": false,    // Simple table (not log explorer)
  "columns": {...},        // Column definitions
  "data": [...]           // Your actual data
}
```

### 2. Column Definitions

Each column must have:

```json
"column_id": {
  "index": 0,              // Position in data arrays (0-based)
  "name": "Display Name",  // Column header shown to users
  "type": "string"         // How to render the data
}
```

### 3. The Data Array

Data is an array of arrays - each inner array is one row:

```json
"data": [
  ["nginx", 25, "Running"],     // Row 1: index 0=name, 1=cpu, 2=status
  ["mysql", 67, "Stopped"],     // Row 2
  ["redis", 91, "Running"]      // Row 3
]
```

**Important**: Values must be in the same order as column `index` fields.

## Adding Visual Polish

Make your table more useful with these enhancements:

```json
{
  "status": 200,
  "type": "table",
  "has_history": false,
  "help": "System services with CPU usage and status",
  "data": [
    ["nginx", 25, "Running", {"rowOptions": {"severity": "normal"}}],
    ["mysql", 67, "Stopped", {"rowOptions": {"severity": "error"}}],
    ["redis", 91, "Running", {"rowOptions": {"severity": "warning"}}]
  ],
  "columns": {
    "name": {
      "index": 0,
      "name": "Service Name", 
      "type": "string",
      "unique_key": true,
      "sticky": true,
      "filter": "multiselect"
    },
    "cpu": {
      "index": 1,
      "name": "CPU Usage",
      "type": "bar-with-integer",
      "units": "%",
      "max": 100,
      "sort": "descending",
      "filter": "range"
    },
    "status": {
      "index": 2,
      "name": "Status", 
      "type": "string",
      "visualization": "pill",
      "filter": "multiselect"
    }
  },
  "default_sort_column": "cpu"
}
```

**New Features Added:**
- **Row coloring**: `rowOptions` with severity levels
- **Sticky columns**: Pin important columns when scrolling
- **Filters**: Let users filter by categories or numeric ranges
- **Progress bars**: Visual CPU usage display
- **Pills**: Status badges with colors
- **Default sorting**: Start sorted by CPU usage

## Limitations of Simple Tables

Simple tables have certain limitations due to their frontend-only processing architecture:

- **No histograms**: Time-based visualization is not supported
- **Performance limits**: Frontend processing is limited by browser memory (recommend \<10k rows)  
- **No backend search**: Query parameter is not sent to backend - search happens client-side only
- **Static facet counts**: Counts are computed by frontend from all received data

For large datasets or advanced log analysis features, consider using log explorers (`has_history: true`).

## Advanced Simple Table Features

### Aggregated Views

Some functions can show both detailed and aggregated data. When enabled, facet pills show both counts:

```json
{
  "aggregated_view": {
    "column": "Count",
    "results_label": "unique combinations", 
    "aggregated_label": "connections"
  }
}
```

This enables smart facet pills like `"15 ⊃ 42"` meaning "15 connections aggregated into 42 rows".

### Charts Integration

Simple tables support interactive charts computed from your table data. The backend defines available charts, and the frontend computes and renders them.

**Chart Types Available:**
- `"bar"` - Basic bar chart
- `"stacked-bar"` - Multi-column stacked bars (most common)
- `"doughnut"` - Pie/doughnut chart  
- `"value"` - Simple numeric display

**Example Configuration:**
```json
{
  "charts": {
    "cpu_usage": {
      "name": "CPU Usage by Service",
      "type": "stacked-bar",
      "columns": ["user_cpu", "system_cpu", "guest_cpu"],
      "groupBy": "column",
      "aggregation": "sum"
    },
    "memory_breakdown": {
      "name": "Memory Types",
      "type": "doughnut", 
      "columns": ["resident", "virtual", "shared"],
      "groupBy": "all",
      "aggregation": "sum"
    }
  },
  "default_charts": [
    ["cpu_usage", "status"],
    ["memory_breakdown", "status"]
  ]
}
```

**Chart Options:**
- **`groupBy`**: 
  - `"column"` (default): Group by selected filter column
  - `"all"`: Aggregate all data together
- **`aggregation`**: `sum`, `mean`, `max`, `min`, `count`

**How It Works:**
1. Frontend takes your table data
2. Groups by selected column (e.g., "service type") 
3. Aggregates values using specified method (e.g., sum CPU values)
4. Renders chart with one bar/slice per group

### Table Row Grouping

Simple tables support grouping rows with customizable aggregation. When users group by a column, rows with the same value are combined using the aggregation method you specify.

**Enable Grouping Options:**
```json
{
  "group_by": {
    "aggregated": [{
      "id": "by_status",
      "name": "By Status",
      "column": "status"
    }, {
      "id": "by_user", 
      "name": "By User",
      "column": "user"
    }]
  }
}
```

**Define Column Aggregation:**
Each column needs a summary type to control how values are aggregated when grouping:

```c
// In your C backend code
buffer_rrdf_table_add_field(
    wb, field_id++, "cpu_percent", "CPU %",
    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
    RRDF_FIELD_VISUAL_BAR,
    RRDF_FIELD_TRANSFORM_NUMBER,
    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
    RRDF_FIELD_SUMMARY_SUM,      // Sum CPU values when grouping
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_OPTS_VISIBLE, NULL
);

buffer_rrdf_table_add_field(
    wb, field_id++, "process_count", "Processes", 
    RRDF_FIELD_TYPE_INTEGER,
    RRDF_FIELD_VISUAL_VALUE,
    RRDF_FIELD_TRANSFORM_NUMBER,
    0, "processes", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
    RRDF_FIELD_SUMMARY_COUNT,    // Count processes when grouping
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_OPTS_VISIBLE, NULL
);
```

**Available Summary/Aggregation Types:**
- `RRDF_FIELD_SUMMARY_COUNT` - Count rows in group
- `RRDF_FIELD_SUMMARY_SUM` - Sum numeric values  
- `RRDF_FIELD_SUMMARY_MEAN` - Average values
- `RRDF_FIELD_SUMMARY_MIN` - Minimum value
- `RRDF_FIELD_SUMMARY_MAX` - Maximum value
- `RRDF_FIELD_SUMMARY_UNIQUECOUNT` - Count unique values
- `RRDF_FIELD_SUMMARY_MEDIAN` - Median value

**Example Result:**
When user groups by "service type", rows like:
```
nginx_worker1  25%  Running
nginx_worker2  30%  Running  
mysql_main     45%  Running
```

Become:
```
nginx    55%  2 processes  (25% + 30% CPU, 2 processes counted)
mysql    45%  1 process    (45% CPU, 1 process counted)
```

---

# Part 2: Log Explorer Functions

Log explorer functions (`has_history: true`) provide advanced log analysis with full-text search, faceted filtering, time navigation, and histograms. Perfect for systemd journals, event logs, and audit trails.

**Implementation Architecture:**
- **Backend handles**: Query processing, pattern matching, facet counting, filtering
- **Frontend provides**: UI for facets, histogram display, infinite scroll
- **Performance**: Scales to millions of records with sampling and server-side filtering  
- **Query parameter**: Processed by backend using Netdata simple patterns
- **Histograms**: Optional, generated by backend in Netdata chart format
- **Uses facets library**: All processing leverages `libnetdata/facets/`

### Complete Log Explorer Example

```json
{
  "status": 200,
  "type": "table", 
  "has_history": true,
  "help": "System journal with real-time updates",
  "accepted_params": [
    "info", "after", "before", "direction", "last", "anchor",
    "query", "facets", "histogram", "if_modified_since", 
    "data_only", "delta", "tail", "sampling"
  ],
  "table": {"id": "journal", "has_history": true},
  "columns": {
    "timestamp": {
      "index": 0,
      "name": "Time",
      "type": "timestamp",
      "transform": "datetime_usec",
      "sort": "descending|fixed",
      "sticky": true
    },
    "priority": {
      "index": 1, 
      "name": "Level",
      "type": "string",
      "visualization": "pill",
      "filter": "facet",
      "options": ["facet", "visible", "sticky"]
    },
    "message": {
      "index": 2,
      "name": "Message", 
      "type": "string",
      "full_width": true,
      "options": ["full_width", "wrap", "visible", "main_text", "fts"]
    }
  },
  "data": [
    [1697644320000000, {"rowOptions": {"severity": "error"}}, "ERROR", "Service failed"],
    [1697644319000000, {"rowOptions": {"severity": "normal"}}, "INFO", "Service started"]
  ],
  "facets": [
    {
      "id": "priority",
      "name": "Log Level",
      "order": 1,
      "defaultExpanded": true,
      "options": [
        {"id": "ERROR", "name": "ERROR", "count": 45, "order": 1},
        {"id": "INFO", "name": "INFO", "count": 234, "order": 3}
      ]
    }
  ],
  "items": {
    "evaluated": 50000,
    "matched": 2520, 
    "returned": 100,
    "max_to_return": 100
  },
  "anchor": {
    "last_modified": 1697644320000000,
    "direction": "backward"
  }
}
```

**Key Features Demonstrated:**
- `has_history: true` - Enables log explorer UI with infinite scroll
- `accepted_params` - Full parameter support for advanced features
- **Faceted search** - Real-time counts computed by backend
- **Anchor pagination** - Efficient navigation through large datasets
- **Microsecond timestamps** - High precision for log entries
- **Full-text search** - Backend pattern matching with `"fts"` fields
- **Row coloring** - Severity-based visual indicators

## Essential Log Explorer Features

### 1. Faceted Search Sidebar

Facets provide dynamic filtering with real-time counts computed by the backend:

```json
{
  "facets": [
    {
      "id": "level",
      "name": "Log Level", 
      "order": 1,
      "defaultExpanded": true,
      "options": [
        {"id": "ERROR", "name": "ERROR", "count": 45, "order": 1},
        {"id": "WARN", "name": "WARN", "count": 123, "order": 2},
        {"id": "INFO", "name": "INFO", "count": 2341, "order": 3}
      ]
    }
  ]
}
```

**Important**: These counts are calculated by the backend facets library during query execution, not by the frontend. The backend scans through matching records and maintains counters for each facet value.

### 2. Full-Text Search

Enable powerful query search across all fields (processed by backend):

```json
{
  "columns": {
    "message": {
      "options": ["fts"],  // Make field full-text searchable
      // ... other options
    }
  }
}
```

**Important**: Unlike simple tables, the query parameter is sent to the backend and processed server-side using the facets library. The backend performs pattern matching and only returns matching records.

**Query Patterns** (using Netdata simple patterns):
- `"error"` - Find "error" anywhere (substring match)
- `"error|warning"` - Find "error" OR "warning" (pipe separator)
- `"!debug"` - Exclude logs containing "debug"
- `"!*debugging*|*debug*"` - Include "debug" but exclude "debugging"
- `"connection failed"` - Find exact phrase (spaces are literal)

**Pattern Evaluation Rules:**
1. **Within field**: Left-to-right, first match wins
2. **Across fields**: ALL fields evaluated, ANY negative match excludes row
3. **Case-insensitive** matching throughout

### 3. Time-Based Histograms

Visualize log distribution over time (log explorers only):

```json
{
  "available_histograms": [
    {"id": "level", "name": "Log Level", "order": 1},
    {"id": "source", "name": "Source", "order": 2}
  ],
  "histogram": {
    "id": "level",
    "name": "Log Level",
    "chart": {
      "result": {
        "labels": ["time", "ERROR", "WARN", "INFO"],
        "data": [
          [1697644200, 5, 12, 234],
          [1697644260, 3, 8, 198]
        ]
      }
    }
  }
}
```

**Important**: Histograms are NOT supported for simple tables (`has_history: false`). They are optional for log explorers and use the same format as Netdata's `/api/v3/data` endpoint. The backend generates histogram data using the facets library.

### 4. Anchor-Based Navigation

Handle large datasets with efficient pagination:

```json
{
  "items": {
    "evaluated": 50000,      // Total scanned
    "matched": 2520,         // Match filters
    "returned": 100,         // In this response
    "max_to_return": 100
  },
  "anchor": {
    "last_modified": 1697644320000000,
    "direction": "backward"
  }
}
```

### Parameter Integration Patterns

**Basic Monitoring Function:**
```json
{
  "accepted_params": ["info", "after", "before"]
}
```

**Advanced Log Function:**
```json
{
  "accepted_params": [
    "info", "after", "before", "direction", "last", "anchor",
    "query", "facets", "histogram", "if_modified_since",
    "data_only", "delta", "tail", "sampling", "slice"
  ]
}
```

**UI Feature Enablement:**
- `"slice"` → Enables "Full data queries" toggle
- `"direction"` → Enables bidirectional pagination
- `"tail"` → Enables streaming mode
- `"delta"` → Enables incremental updates
- `"query"` → Enables full-text search

## Advanced Log Explorer Features

### Column Options for Logs

Log explorers support special column options:

| Option | Effect |
|--------|--------|
| `"fts"` | Full-text searchable by query |
| `"facet"` | Appears in sidebar filters with counts |
| `"main_text"` | Primary content (usually message) |
| `"rich_text"` | May contain formatting |
| `"hidden"` | Hide by default |

Example:
```json
{
  "message": {
    "type": "string",
    "full_width": true,
    "options": ["full_width", "wrap", "visible", "main_text", "fts", "rich_text"]
  }
}
```

### Log-Specific UI Behavior

When `has_history: true`:
- **Sidebar**: Shows faceted filters instead of simple filters
- **Search box**: Queries all `fts` fields using pattern matching
- **Infinite scroll**: Loads more data as you scroll
- **Time navigation**: Jump to specific time periods
- **Sampling**: For very large datasets

---

# Part 3: Complete Options Reference

This section documents every field type, option, and behavior for quick reference while developing.

## Field Types and UI Rendering

### Text and Categories

```json
{
  "type": "string",
  "visualization": "value"    // Default: plain text, left-aligned
}
```

```json
{
  "type": "string", 
  "visualization": "pill"     // Colored badges for status/categories
}
```

### Numbers and Metrics

```json
{
  "type": "integer",          // Right-aligned numbers
  "transform": "number",      // Respect decimal_points
  "decimal_points": 2
}
```

```json
{
  "type": "bar-with-integer", // Progress bars with values
  "max": 100,                 // Required for bars
  "units": "%"
}
```

### Timestamp Format Requirements

**Simple Tables**: Use milliseconds with `datetime` transform
```json
{
  "type": "timestamp",
  "transform": "datetime",     // Expects milliseconds
  "data": [1697644320000]     // JavaScript Date format
}
```

**Log Explorers**: Use microseconds with `datetime_usec` transform  
```json
{
  "type": "timestamp", 
  "transform": "datetime_usec", // Expects microseconds
  "data": [1697644320000000]    // Microsecond precision
}
```

**Frontend Conversion:**
```javascript
// datetime_usec automatically converts to milliseconds
if (usec) {
  epoch = epoch ? Math.floor(epoch / 1000) : epoch
}
```

**API Parameters**: `after` and `before` are automatically converted from milliseconds to seconds when sent to functions.

```json
{
  "type": "duration",
  "transform": "duration_s"    // Formats seconds as "1d 2h 3m"
}
```

### Rich Content

```json
{
  "type": "feedTemplate",     // Full-width rich content
  "full_width": true          // Automatically applied
}
```

## Essential Column Options

### Layout Control

```json
{
  "unique_key": true,         // Row identifier (exactly one required)
  "sticky": true,             // Pin when scrolling horizontally
  "visible": true,            // Show by default
  "full_width": true,         // Expand to fill available space
  "wrap": true                // Enable text wrapping
}
```

### Filtering Options

```json
{
  "filter": "multiselect"     // Checkboxes (default for simple tables)
}
```

```json
{
  "filter": "range"           // Min/max sliders for numbers
}
```

```json
{
  "filter": "facet"           // Sidebar with counts (log explorers only)
}
```

### Sorting Options

```json
{
  "sort": "descending",       // Default sort direction
  "sortable": true            // Allow user sorting (default)
}
```

```json
{
  "sort": "descending|fixed", // Prevent user from changing sort
  "sortable": false
}
```

## Row-Level Features

### Row Coloring by Severity

Add row coloring by including `rowOptions` as the last element:

```json
{
  "data": [
    ["normal data", "values", {"rowOptions": {"severity": "normal"}}],
    ["warning data", "values", {"rowOptions": {"severity": "warning"}}],
    ["error data", "values", {"rowOptions": {"severity": "error"}}],
    ["notice data", "values", {"rowOptions": {"severity": "notice"}}]
  ]
}
```

**Severity Levels:**
- `"normal"` - Default appearance
- `"warning"` - Yellow background
- `"error"` - Red background  
- `"notice"` - Blue background

## Chart and Grouping Configuration

### Chart Definition

```json
{
  "charts": {
    "resource_usage": {
      "name": "Resource Usage",
      "type": "stacked-bar",
      "columns": ["cpu", "memory", "disk"],
      "groupBy": "column",
      "aggregation": "sum"
    },
    "status_distribution": {
      "name": "Status Distribution", 
      "type": "doughnut",
      "columns": ["count"],
      "groupBy": "all",
      "aggregation": "count"
    }
  },
  "default_charts": [
    ["resource_usage", "service_type"],
    ["status_distribution", "status"]
  ]
}
```

### Grouping Configuration

```json
{
  "group_by": {
    "aggregated": [
      {
        "id": "by_service",
        "name": "By Service Type",
        "column": "service_type"
      },
      {
        "id": "by_status", 
        "name": "By Status",
        "column": "status"
      }
    ]
  }
}
```

## Backend Implementation Examples

### C Code for Simple Tables

```c
// Add a progress bar column
buffer_rrdf_table_add_field(
    wb, field_id++, "cpu", "CPU Usage",
    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
    RRDF_FIELD_VISUAL_BAR,
    RRDF_FIELD_TRANSFORM_NUMBER,
    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
    RRDF_FIELD_SUMMARY_SUM,
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_OPTS_VISIBLE, NULL
);

// Add row coloring
buffer_json_add_array_item_object(wb);
buffer_json_member_add_object(wb, "rowOptions");
buffer_json_member_add_string(wb, "severity", "error");
buffer_json_object_close(wb);
buffer_json_object_close(wb);
```

### Using the Facets Library (Log Explorers)

```c
// Initialize facets for log exploration
FACETS *facets = facets_create(...);

// Set up query search
facets_set_query(facets, query_string);

// Add a faceted field
facets_register_facet_id(facets, "level", 
    FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_REORDER);

// Generate the response
facets_table_config(facets, wb);
```

## Error Handling

Always handle the info request first:

```json
{
  "v": 3,                    // Enable POST requests
  "status": 200,
  "type": "table", 
  "has_history": false,      // or true for log explorers
  "help": "Function description",
  "accepted_params": [...],
  "required_params": [...]
}
```

### Complete Info Response Example

```json
{
  "v": 3,
  "status": 200,
  "type": "table",
  "has_history": true,
  "help": "System log explorer with faceted search",
  "accepted_params": [
    "info", "after", "before", "direction", "last", "anchor", 
    "query", "facets", "histogram", "if_modified_since", 
    "data_only", "delta", "tail", "sampling"
  ],
  "required_params": [
    {
      "id": "unit",
      "name": "System Unit",
      "type": "select", 
      "options": [
        {"id": "nginx.service", "name": "Nginx"},
        {"id": "mysql.service", "name": "MySQL"}
      ]
    }
  ]
}
```

**Critical Fields:**
- `"v": 3` enables POST requests with JSON payloads
- `accepted_params` determines which parameters the function accepts
- `required_params` generates filter UI and validates execution

For errors, return:

```json
{
  "status": 400,
  "error_message": "Descriptive error message"
}
```

### Performance Optimization

**For Large Datasets:**

1. **Enable Sampling**:
```json
{
  "accepted_params": ["sampling"],
  "sampling": 10  // 1 in 10 sampling
}
```

2. **Use Delta Updates**:
```json
{
  "if_modified_since": 1697644320000000,
  "delta": true,
  "data_only": true
}
```

3. **Implement Tail Limiting**:
```c
// Limit tail data to prevent memory issues
if (tail_mode) {
    limit_results_to(500);
}
```

4. **Support Anchor Pagination**:
```json
{
  "pagination": {
    "enabled": true,
    "column": "timestamp", 
    "key": "anchor",
    "units": "timestamp_usec"
  }
}
```

## Best Practices Summary

### For Simple Tables
1. Always include one `unique_key` column
2. Use `bar-with-integer` for metrics with `max` values
3. Add `filter: "range"` for numbers, `"multiselect"` for categories
4. Use `rowOptions` for status indication
5. Set meaningful `default_sort_column`
6. Define column `summary` types for grouping (SUM for metrics, COUNT for processes)
7. Add charts for key metrics with appropriate `groupBy` and `aggregation`
8. Include `group_by` options for common analysis patterns

### For Log Explorers  
1. Set `has_history: true`
2. Use microsecond timestamps with `datetime_usec`
3. Mark important fields with `"fts"` for search
4. Use `filter: "facet"` instead of `"multiselect"`
5. Include proper facets with counts
6. Implement anchor-based pagination

### General
1. Test with `?info` requests first
2. Include helpful `help` text
3. Handle errors gracefully
4. Use appropriate `units` for clarity
5. Follow existing function patterns in your codebase


---

This guide covers everything you need to build both simple monitoring functions and advanced log explorers. For implementation details and edge cases, see [FUNCTIONS_REFERENCE.md](FUNCTIONS_REFERENCE.md).
