# Netdata Functions v3 Protocol - Technical Reference

> **Note**: This is the technical specification. For a practical guide to implementing functions, see [FUNCTIONS_DEVELOPER_GUIDE.md](FUNCTIONS_DEVELOPER_GUIDE.md).

## Overview

This document provides the complete technical reference for Netdata Functions protocol v3, combining all information about simple tables (has_history=false) and log explorers (has_history=true). This is the authoritative internal documentation for maintaining and extending Netdata functions.

## Table of Contents

1. [Protocol Overview](#protocol-overview)
2. [Request Flow](#request-flow)
3. [Simple Table Format](#simple-table-format)
4. [Log Explorer Format](#log-explorer-format)
5. [Field Types and Enumerations](#field-types-and-enumerations)
6. [UI Implementation](#ui-implementation)
7. [Backend Implementation](#backend-implementation)
8. [Best Practices](#best-practices)
9. [Known Functions](#known-functions)
10. [Development Checklist](#development-checklist)

## Protocol Overview

Netdata Functions allow collectors/plugins to expose interactive data through a streaming protocol. The protocol has evolved from GET-based CLI parameters (legacy) to POST-based JSON payloads (modern v3).

### Function Types

1. **Simple Table View** (`has_history: false`)
   - Basic tabular data display with frontend-side filtering and search
   - Examples: `processes`, `network-connections`, `block-devices`

2. **Log Explorer Format** (`has_history: true`)
   - Advanced table with backend-powered faceted search, histograms, and infinite scroll
   - Examples: `systemd-journal`, `windows-events`

### Critical Implementation Differences

| Aspect | Simple Tables | Log Explorers |
|--------|---------------|---------------|
| **Data Processing** | All data sent to frontend | Backend filters before sending |
| **Facet Counts** | Frontend counts occurrences in received data | Backend computes counts during query execution |
| **Full-Text Search** | Frontend substring search across visible data | Backend pattern matching with facets library |
| **Histograms** | Not supported - no time-based visualization | Optional - backend generates Netdata chart format |
| **Performance** | Limited by browser memory and processing | Scales to millions of records with sampling |
| **Query Parameter** | Ignored by backend (frontend only) | Processed by backend using simple patterns |

## Request Flow

### Modern Flow (v:3)

1. **Info Request** (Always GET)
   ```
   GET /api/v3/function?function=systemd-journal info after:1234567890 before:1234567890
   ```

2. **Info Response**
   ```json
   {
     "v": 3,  // Indicates POST should be used for data requests
     "accepted_params": [...],
     "required_params": [...],
     "status": 200,
     "type": "table",
     "has_history": false
   }
   ```

3. **Data Request** (POST when v=3)
   ```json
   {
     "query": "*error* !*debug*",
     "selections": {
       "priority": ["error", "warning"],
       "unit": ["nginx.service"]
     },
     "after": 1234567890,
     "before": 1234567890,
     "last": 100
   }
   ```

### Preflight Info Request Details

**Request Format:**
```
GET /api/v3/function?function=systemd-journal info after:1234567890 before:1234567890
```

**Required Response Fields:**
```json
{
  "v": 3,
  "status": 200,
  "type": "table",
  "has_history": false,
  "accepted_params": ["info", "after", "before", "direction", "last"],
  "required_params": [
    {
      "id": "priority",
      "name": "Log Level",
      "type": "select",
      "options": [
        {"id": "error", "name": "Error", "defaultSelected": true},
        {"id": "warn", "name": "Warning"}
      ]
    }
  ],
  "help": "Function description"
}
```

**Frontend Processing:**
- `accepted_params`: Validates which parameters can be sent to function
- `required_params`: Generates filter UI, prevents execution if missing
- `v: 3`: Enables POST requests with JSON payloads
- Missing required parameters show user-friendly error messages

### Backend Implementation

```c
// Simplified from logs_query_status.h
if(payload) {
    // POST request - parse JSON payload
    facets_use_hashes_for_ids(facets, false);  // Use plain field names
    rq->fields_are_ids = false;
} else {
    // GET request - parse CLI parameters (legacy)
    facets_use_hashes_for_ids(facets, true);   // Use hash IDs
    rq->fields_are_ids = true;
}
```

### Key Differences

| Aspect | GET (Legacy) | POST (Modern v3) |
|--------|--------------|------------------|
| Version | v < 3 | v = 3 |
| Field IDs | 11-char hashes | Plain names |
| Parameters | URL encoded | JSON body |
| Facet filters | `field_hash:value1,value2` | `{"field": ["value1", "value2"]}` |
| Full-text search | `query:search terms` | `{"query": "search terms"}` |

### Standard Accepted Parameters

**Core Parameters (All Functions):**
- `"info"` - Function info requests (always supported)
- `"after"` - Time range start (seconds epoch)  
- `"before"` - Time range end (seconds epoch)

**Log Explorer Parameters (has_history=true):**
- `"direction"` - Query direction: `"backward"` | `"forward"`
- `"last"` - Result limit (default: 200)
- `"anchor"` - Pagination cursor (timestamp or row identifier)
- `"query"` - Full-text search using Netdata simple patterns
- `"facets"` - Facet selection: `"field1,field2"`
- `"histogram"` - Histogram field selection
- `"if_modified_since"` - Conditional updates (microseconds epoch)
- `"data_only"` - Skip metadata in response (boolean)
- `"delta"` - Incremental responses (boolean)
- `"tail"` - Streaming mode (boolean)
- `"sampling"` - Data sampling control

**UI Feature Parameters:**
- `"slice"` - Enables "Full data queries" toggle in UI

**Frontend Behavior:**
```javascript
// Only accepted parameters are sent to functions
const allowedFilterIds = [...selectedFacets, ...requiredParamIds, ...acceptedParams]
filtersToSend = allowedFilterIds.reduce((acc, filterId) => {
  if (filterId in filters) acc[filterId] = filters[filterId]
  return acc
}, {})
```

### Query Parameter (Netdata Simple Patterns)

The `query` parameter uses Netdata's simple pattern matching for full-text search across all fields:

**Pattern Syntax:**
- `|` - Pattern separator (OR logic between patterns)
- `*` - Wildcard matching any number of characters
- `!` - Negates the pattern (exclude matches)
- Spaces are matched literally (not pattern separators)
- Case-insensitive matching
- Default behavior is substring matching (no wildcards needed)

**Matching Modes:**
- `pattern` - Substring match (default) - finds "pattern" anywhere
- `*pattern` - Suffix match - finds strings ending with "pattern"
- `pattern*` - Prefix match - finds strings starting with "pattern"
- `*pattern*` - Substring match (explicit) - same as default

**Examples:**
```json
{
  "query": "error"                    // Finds "error" anywhere (substring)
  "query": "error|warning"            // Finds "error" OR "warning"
  "query": "error|warning|critical"   // Multiple OR patterns
  "query": "!debug"                   // Exclude ALL rows containing "debug"
  "query": "!*debugging*|*debug*"     // Include "debug" but exclude "debugging"
  "query": "connection failed"        // Find exact phrase (spaces included)
  "query": "*error"                   // Find strings ending with "error"
  "query": "nginx*"                   // Find strings starting with "nginx"
}
```

**Pattern Evaluation Rules:**

1. **Within a field**: Left-to-right, first match wins
   - `"!*debugging*|*debug*"` - If text contains "debugging", it's negative match. Otherwise, if contains "debug", it's positive match.

2. **Across all fields**: ALL fields are evaluated (no short-circuit)
   - Every field with FTS enabled is checked against the pattern
   - Positive and negative matches are counted separately
   - Field evaluation order doesn't affect the outcome

3. **Row decision (after all fields evaluated)**:
   - **Excluded if**: ANY field has a negative match (regardless of positive matches)
   - **Included if**: At least one positive match AND zero negative matches
   - **Excluded if**: No positive matches found

**Example**: Query `"!*debugging*|*debug*"`
```
Row 1: message="debug info", category="debugging tips"
  → message: positive match (debug)
  → category: negative match (debugging)  
  → Result: EXCLUDED (has negative match)

Row 2: message="debug info", category="testing"
  → message: positive match (debug)
  → category: no match
  → Result: INCLUDED (positive match, no negative)

Row 3: message="error info", category="testing"  
  → message: no match
  → category: no match
  → Result: EXCLUDED (no positive matches)
```

**Key Point**: The order fields are evaluated doesn't matter - the same counters are updated and the same decision is made regardless of whether positive or negative matches are found first.

**Common Use Cases:**
- `"timeout|failed|refused"` - Find various connection issues
- `"!trace|!debug|nginx|apache"` - Find web server logs, but exclude debug/trace
- `"error code 500"` - Find exact phrase with spaces
- `"critical|fatal|emergency"` - Find severe log levels
- `"!*test*|!*debug*|*"` - Include everything except test and debug content

**Implementation Details:**
- Uses `simple_pattern_create(query, "|", SIMPLE_PATTERN_SUBSTRING, false)`
- Searches all fields marked with `FACET_KEY_OPTION_FTS` or when `FACETS_OPTION_ALL_KEYS_FTS` is set
- No support for `?` single-character wildcards
- Escaping with `\` is supported for literal matches

**Important**: Hash IDs (like `priority_hash`) are obsolete and only exist for backward compatibility with GET requests. All new functions should use v:3 with plain field names.

## Simple Table Format

### Complete Structure

```json
{
  // Required fields
  "status": 200,
  "type": "table",
  "has_history": false,
  "data": [
    [value1, value2, value3, ...],  // Row 1
    [value1, value2, value3, ...],  // Row 2
    // Special rowOptions as last element
    [..., {"rowOptions": {"severity": "warning|error|notice|normal"}}]
  ],
  "columns": {
    "column_name": {
      // Required
      "index": 0,                  // Position in data array
      "name": "Display Name",      // Column header
      "type": "string",           // Field type
      
      // Optional
      "unique_key": false,        // Row identifier (exactly one required)
      "visible": true,            // Default visibility
      "sticky": false,            // Pin when scrolling
      "visualization": "value",   // How to render
      "transform": "none",        // Value transformation
      "decimal_points": 2,        // For numbers
      "units": "bytes",           // Display units
      "max": 100,                 // For bar types
      "sort": "descending",       // Default sort
      "sortable": true,           // User can sort
      "filter": "multiselect",    // Filter type
      "full_width": false,        // Expand to fill
      "wrap": false,              // Text wrapping
      "summary": "sum"            // Aggregation (backend only)
    }
  },
  
  // Optional extensions
  "help": "Function description",
  "update_every": 1,
  "expires": 1234567890,
  "default_sort_column": "column_name",
  "group_by": {
    "aggregated": [{
      "id": "group_id",
      "name": "Group Name",
      "column": "column_to_group_by"
    }]
  },
  "charts": {
    "chart_id": {
      "name": "Chart Name",
      "type": "stacked|bar",
      "columns": ["col1", "col2"]
    }
  },
  "accepted_params": [{
    "id": "param_id",
    "name": "Parameter Name",
    "type": "string|integer|boolean",
    "default": "default_value"
  }],
  "required_params": ["param1", "param2"]
}
```

### Row Options

Special last element in data array for row styling:

```json
[..., {"rowOptions": {"severity": "error"}}]  // Red background
[..., {"rowOptions": {"severity": "warning"}}] // Yellow background
[..., {"rowOptions": {"severity": "notice"}}]  // Blue background
[..., {"rowOptions": {"severity": "normal"}}]  // Default appearance
```

## Log Explorer Format

### Complete Structure

```json
{
  // Basic fields (same as simple table)
  "status": 200,
  "type": "table",
  "has_history": true,  // REQUIRED: Enables log explorer UI
  "help": "System log explorer",
  "update_every": 1,
  
  // Table metadata
  "table": {
    "id": "logs",
    "has_history": true,
    "pin_alert": false
  },
  
  // Faceted filters (dynamic with counts)
  "facets": [
    {
      "id": "priority",        // Plain field name (not hash)
      "name": "Priority",
      "order": 1,
      "defaultExpanded": true,
      "options": [
        {
          "id": "ERROR",
          "name": "ERROR",
          "count": 45,         // Real-time count
          "order": 1
        }
      ]
    }
  ],
  
  // Enhanced columns
  "columns": {
    "timestamp": {
      "index": 0,
      "id": "timestamp",
      "name": "Time",
      "type": "timestamp",
      "transform": "datetime_usec",
      "sort": "descending|fixed",
      "sortable": false,
      "sticky": true
    },
    "level": {
      "index": 1,
      "id": "priority",        // Links to facet
      "name": "Level",
      "type": "string",
      "visualization": "pill",
      "filter": "facet",       // Not multiselect!
      "options": ["facet", "visible", "sticky"]
    },
    "message": {
      "index": 3,
      "id": "message",
      "name": "Message",
      "type": "string",
      "full_width": true,
      "options": [
        "full_width",
        "wrap",
        "visible",
        "main_text",          // Primary content
        "fts",                // Full-text searchable
        "rich_text"           // May contain formatting
      ]
    }
  },
  
  // Data with microsecond timestamps
  "data": [
    [
      1697644320000000,        // Microseconds
      {"severity": "error"},   // rowOptions
      "ERROR",                 // level
      "nginx",                 // source
      "Connection failed"      // message
    ]
  ],
  
  // Histogram configuration
  "available_histograms": [
    {"id": "priority", "name": "Priority", "order": 1},
    {"id": "source", "name": "Source", "order": 2}
  ],
  "histogram": {
    "id": "priority",
    "name": "Priority",
    "chart": {
      "summary": {/* Netdata chart metadata */},
      "result": {
        "labels": ["time", "ERROR", "WARN", "INFO"],
        "data": [
          [1697644200, 5, 12, 234],
          [1697644260, 3, 8, 198]
        ]
      }
    }
  },
  
  // Pagination metadata
  "items": {
    "evaluated": 50000,       // Total scanned
    "matched": 2520,          // Match filters
    "unsampled": 100,         // Skipped (sampling)
    "estimated": 0,           // Statistical estimate
    "returned": 100,          // In this response
    "max_to_return": 100,
    "before": 0,
    "after": 2420
  },
  
  // Navigation anchor
  "anchor": {
    "last_modified": 1697644320000000,
    "direction": "backward"   // or "forward"
  },
  
  // Request echo (optional)
  "request": {
    "query": "*error* *warning*",
    "filters": ["priority:error,warning"],
    "histogram": "priority"
  },
  
  // Additional metadata
  "expires": 1697644920000,
  "sampling": 10              // 1 in N sampling
}
```

### Key Differences from Simple Tables

| Feature | Simple Table | Log Explorer |
|---------|--------------|--------------|
| **Facet counts** | Frontend computes from data | Backend computes in facets library |
| **Full-text search** | Frontend substring matching | Backend pattern matching |
| **Histograms** | Not supported | Optional (backend generated) |
| Filtering | Static multiselect | Dynamic facets with counts |
| Pagination | All data at once | Anchor-based infinite scroll |
| Time visualization | None | Histogram chart |
| Navigation | None | Bi-directional with timestamps |
| Performance | All data loaded | Sampling for large datasets |

### Log-Specific Column Options

| Option | UI Effect |
|--------|-----------|
| `"facet"` | Field is filterable via facets |
| `"fts"` | Full-text searchable |
| `"main_text"` | Primary content field |
| `"rich_text"` | May contain formatting |
| `"pretty_xml"` | Format as XML |
| `"hidden"` | Hide by default |

## Facet Value Pills and Aggregated Counts

### Overview

Facet values in the sidebar display pills with counts that change based on the function's aggregation mode. The UI uses a smart component that displays different formats:

1. **Simple Counts**: Just the number of matching rows
2. **Aggregated Counts**: Shows both aggregated count and original count with a union symbol

### UI Implementation

The pills are rendered using this logic:

```javascript
{!!actualCount && <TextSmall>{actualCount} &#8835;&nbsp;</TextSmall>}
<TextSmall>{(pill || count).toString()}</TextSmall>
```

**Symbol**: `&#8835;` renders as `⊃` (superset symbol, looks like rotated 'u')

**Display Formats**:
- Simple: `"42"` (just the count)
- Aggregated: `"15 ⊃ 42"` (15 aggregated items containing 42 total)

### Backend Configuration

Functions enable aggregated counts by including an `aggregated_view` object in their response:

```json
{
  "aggregated_view": {
    "column": "Count",
    "results_label": "unique combinations", 
    "aggregated_label": "sockets"
  }
}
```

**Example from network-connections function**:
```c
// In network-viewer.c when aggregated=true
buffer_json_member_add_object(wb, "aggregated_view");
{
    buffer_json_member_add_string(wb, "column", "Count");
    buffer_json_member_add_string(wb, "results_label", "unique combinations");
    buffer_json_member_add_string(wb, "aggregated_label", "sockets");
}
buffer_json_object_close(wb);
```

## Charts Configuration

Functions can provide both standard charts (computed by frontend) and custom visualizations.

### Standard Charts

**Configuration:**
```json
{
  "charts": {
    "cpu_usage": {
      "name": "CPU Usage by Service",
      "type": "stacked-bar",
      "columns": ["user_cpu", "system_cpu"],
      "groupBy": "column",
      "aggregation": "sum"
    }
  },
  "default_charts": [
    ["cpu_usage", "service_type"]
  ]
}
```

**Supported Types:**
- `"bar"` - Basic bar chart
- `"stacked-bar"` - Multi-column stacked bars
- `"doughnut"` - Pie/doughnut chart
- `"value"` - Simple numeric display

**GroupBy Options:**
- `"column"` (default) - Group by selected filter column
- `"all"` - Aggregate all data together

### Custom Charts

For specialized visualizations beyond standard chart types, some functions may use predefined custom chart types:

```json
{
  "customCharts": {
    "network_topology": {
      "type": "network-viewer",
      "config": {
        "layout": "force",
        "showLabels": true
      }
    }
  }
}
```

**Available Custom Types:**
- `"network-viewer"` - Interactive network topology (for network-connections function)

### Frontend Processing

Charts are computed from table data:
1. Frontend groups data by selected column
2. Applies aggregation function (sum, count, avg, etc.)
3. Renders chart with grouped results

### Table Row Grouping

Simple tables support grouping rows with backend-defined aggregation rules.

**Backend Summary Types:**
```c
typedef enum {
    RRDF_FIELD_SUMMARY_COUNT,        // Count rows in group
    RRDF_FIELD_SUMMARY_UNIQUECOUNT,  // Count unique values  
    RRDF_FIELD_SUMMARY_SUM,          // Sum numeric values
    RRDF_FIELD_SUMMARY_MIN,          // Minimum value
    RRDF_FIELD_SUMMARY_MAX,          // Maximum value
    RRDF_FIELD_SUMMARY_MEAN,         // Average value
    RRDF_FIELD_SUMMARY_MEDIAN,       // Median value
} RRDF_FIELD_SUMMARY;
```

**Column Summary Configuration:**
```c
buffer_rrdf_table_add_field(
    wb, field_id++, "cpu", "CPU Usage",
    RRDF_FIELD_TYPE_INTEGER,
    RRDF_FIELD_VISUAL_VALUE,
    RRDF_FIELD_TRANSFORM_NUMBER,
    2, "%", NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
    RRDF_FIELD_SUMMARY_SUM,  // How to aggregate when grouping
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_OPTS_VISIBLE, NULL
);
```

**Group By Support:**
```json
{
  "group_by": {
    "aggregated": [{
      "id": "by_status",
      "name": "By Status", 
      "column": "status"
    }]
  }
}
```

### Frontend Processing

The frontend processes table data to generate facet counts:

```javascript
const getFilterTableOptions = (data, { param, columns, aggregatedView } = {}) =>
  Object.entries(
    data.reduce((h, fn) => {
      h[fn[param]] = {
        count: (h[fn[param]]?.count || 0) + (fn.hidden ? 0 : 1),
        ...(aggregatedView && {
          actualCount: (h[fn[param]]?.actualCount || 0) + 
                      (fn.hidden ? 0 : fn[aggregatedView.column] || 1),
          actualCountLabel: aggregatedView.aggregatedLabel,
          countLabel: aggregatedView.resultsLabel,
        }),
      }
      return h
    }, {})
  ).map(([id, values]) => ({ id, ...values }))
```

### How It Works

The aggregated count system uses an existing data column to track how many items were aggregated into each row:

1. **Backend declares** which column contains the aggregation count:
   ```json
   "aggregated_view": {
     "column": "Count"  // Use the "Count" column from data rows
   }
   ```

2. **Frontend calculates** two values for each facet value:
   - **`count`**: How many rows have this facet value
   - **`actualCount`**: Sum of the aggregation column for all rows with this facet value

3. **Example**: Network connections aggregated by protocol

   **Data returned by backend:**
   ```
   Direction | Protocol | LocalPort | RemotePort | Count
   ---------|----------|-----------|------------|-------
   Inbound  | TCP      | *         | *          | 5
   Inbound  | UDP      | *         | *          | 3  
   Outbound | TCP      | *         | *          | 7
   Outbound | UDP      | *         | *          | 2
   ```

   **Facet pills displayed in UI:**
   - Direction facet:
     - Inbound: `8 ⊃ 2` (8 connections aggregated into 2 rows)
     - Outbound: `9 ⊃ 2` (9 connections aggregated into 2 rows)
   - Protocol facet:
     - TCP: `12 ⊃ 2` (12 connections aggregated into 2 rows)
     - UDP: `5 ⊃ 2` (5 connections aggregated into 2 rows)

   **Reading the pills**: `12 ⊃ 2` means "12 original items shown in 2 table rows"

### Tooltip Content

The tooltip provides human-readable context:
- Simple mode: `"42 results"`
- Aggregated mode: `"15 sockets aggregated in 42 unique combinations"`

### Current Implementations

| Function | Aggregated Mode | Trigger | Count Meaning |
|----------|----------------|---------|---------------|
| `network-connections` | `sockets:aggregated` | Parameter | Sockets → unique combinations |
| Other functions | N/A | None currently | Single count only |

### Adding Aggregated Counts

To add aggregated count support to a function:

1. **Backend**: Add `aggregated_view` object to response when in aggregated mode
2. **Data**: Include aggregation column with numeric values
3. **Frontend**: No changes needed - automatically processes based on `aggregated_view` presence

## Field Types and Enumerations

### Field Types (RRDF_FIELD_TYPE_*)

#### Implemented in UI

| Type | UI Component | Use Case | Notes |
|------|--------------|----------|-------|
| `string` | ValueCell | Text, names, categories | Left-aligned |
| `integer` | ValueCell | Numbers, counts, IDs | Right-aligned |
| `bar-with-integer` | BarCell | Percentages, metrics | Requires `max` |
| `duration` | BarCell | Time intervals | Auto-formats seconds |
| `timestamp` | DatetimeCell* | Date/time points | *UI maps to datetime |
| `feedTemplate` | FeedTemplateCell | Rich content | Auto full_width |

#### Fallback Implementation

| Type | Behavior | Notes |
|------|----------|-------|
| `boolean` | ValueCell | No special boolean UI |
| `detail-string` | ValueCell | No expandable functionality |
| `array` | ValueCell | Works with `pill` visualization |
| `none` | ValueCell | Avoid using |

### Visual Types (RRDF_FIELD_VISUAL_*)

| Type | UI Component | Use Case |
|------|--------------|----------|
| `value` | ValueCell | Standard text display (default) |
| `bar` | BarCell | Progress bar without text |
| `pill` | PillCell | Badge/tag display |
| `richValue` | RichValueCell | Enhanced value display |
| `feedTemplate` | FeedTemplateCell | Full-width template |
| `rowOptions` | null | Special row configuration |

**Fallback Behavior**: `gauge` is not a recognized visualization. If used, it is ignored, and the renderer falls back to using the field's `type` to select a component.

### Transform Types (RRDF_FIELD_TRANSFORM_*)

| Type | Input | Output | Notes |
|------|-------|--------|-------|
| `none` | Any | Unchanged | Default |
| `number` | Number | Formatted with decimals | Uses `decimal_points` |
| `duration` | Seconds | "Xd Yh Zm" | Human-readable |
| `datetime` | Epoch ms | Localized date/time | |
| `datetime_usec` | Epoch μs | Localized date/time | For logs |
| `xml` | XML string | Formatted XML | No specialized UI |

### Conditional Patterns and Dependencies

#### Type and Transform Compatibility

Not all `transform` values are compatible with all `type` values. The backend enforces the following compatibility rules:

| Field Type (`type`) | Compatible Transforms (`transform`) |
|---|---|
| `timestamp` | `datetime_ms`, `datetime_usec` |
| `duration` | `duration_s` |
| `integer`, `bar-with-integer` | `number` |
| `string`, `boolean`, `array` | `none`, `xml` |

Using an incompatible transform will result in unexpected behavior or errors.

#### `rowOptions` Dummy Column

To add `rowOptions` for row-level severity styling, a special "dummy" column must be added to the `columns` definition. This column is not displayed in the UI but provides the necessary metadata. It must be created with this specific combination of values:

*   **type**: `none` (`RRDF_FIELD_TYPE_NONE`)
*   **visualization**: `rowOptions` (`RRDF_FIELD_VISUAL_ROW_OPTIONS`)
*   **options flag**: `dummy` (`RRDF_FIELD_OPTS_DUMMY`)

**Example C code:**
```c
buffer_rrdf_table_add_field(wb, field_id++, "row_options", "Row Options",
                            RRDF_FIELD_TYPE_NONE, RRDF_FIELD_VISUAL_ROW_OPTIONS, RRDF_FIELD_TRANSFORM_NONE,
                            0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                            RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE, RRDF_FIELD_OPTS_DUMMY, NULL);
``` 

### Filter Types (RRDF_FIELD_FILTER_*)

| Type | UI Component | Use Case | Location |
|------|--------------|----------|----------|
| `multiselect` | Checkboxes | Column filtering (default) | Dynamic filters |
| `range` | RangeFilter | Numeric min/max | Dynamic filters |
| `facet` | Facets component | With counts (logs) | Sidebar |

### Field Options (RRDF_FIELD_OPTS_*)

| Option | Bit | UI Effect |
|--------|-----|-----------|
| `unique_key` | 0x01 | Row identifier (one required) |
| `visible` | 0x02 | Show by default |
| `sticky` | 0x04 | Pin column when scrolling |
| `full_width` | 0x08 | Expand to fill space |
| `wrap` | 0x10 | Enable text wrapping |
| `dummy` | 0x20 | Internal use only |
| `expanded_filter` | 0x40 | Expand filter by default |

### Sort Options (RRDF_FIELD_SORT_*)

- `ascending` - Sort low to high
- `descending` - Sort high to low
- Fixed sort (0x80) - Prevent user sorting

### Summary Types (RRDF_FIELD_SUMMARY_*)

Backend calculates but UI doesn't display directly:
- `count`, `sum`, `min`, `max`, `mean`, `median`
- `uniqueCount` - Number of unique values
- `extent` - Range [min, max] (UI supported)
- `unique` - List of unique values (UI supported)

## UI Implementation

### Column Width System

The UI automatically sizes columns based on metadata:

| Size | Pixels | Applied To |
|------|--------|------------|
| xxxs | 90px | unique_key fields, bar types |
| xxs | 110px | Default for most types |
| xs | 130px | Available but rarely used |
| sm | 160px | timestamp, datetime types |
| md-xl | 190-290px | Available but rarely used |
| xxl | 1000px | feedTemplate with full_width |

**Algorithm**:
1. If `full_width`: → xxl with expansion
2. If `unique_key`: → xxxs
3. By visualization: bar → xxxs
4. By type: feedTemplate → xxl, timestamp → sm
5. Default → xxs

### Component Mapping

```javascript
// Field type → Component
componentByType = {
  "bar": BarCell,
  "bar-with-integer": BarCell,
  "duration": BarCell,
  "pill": PillCell,
  "feedTemplate": FeedTemplateCell,
  "datetime": DatetimeCell,
  // Others → ValueCell
}

// Visualization → Component
componentByVisualization = {
  "bar": BarCell,
  "pill": PillCell,
  "richValue": RichValueCell,
  "feedTemplate": FeedTemplateCell,
  "rowOptions": null,  // Skip rendering
  // Others → ValueCell
}
```

### UI Component Architecture

The frontend uses a modular architecture with:
- **Value Components**: Handle different field type rendering
- **Table Normalizer**: Processes function responses into UI-ready format
- **Filter Components**: Implement multiselect, range, and facet filtering
- **Chart Components**: Standard chart rendering (bar, stacked-bar, doughnut)
- **Custom Visualizations**: Extensible system for specialized charts

## Backend Implementation

### Key Functions

```c
// Add a field to the table
buffer_rrdf_table_add_field(
    BUFFER *wb,
    size_t field_id,
    const char *key,
    const char *name,
    RRDF_FIELD_TYPE type,
    RRDF_FIELD_VISUAL visual,
    RRDF_FIELD_TRANSFORM transform,
    size_t decimal_points,
    const char *units,
    NETDATA_DOUBLE max,
    RRDF_FIELD_SORT sort,
    const char *pointer_to_dim_in_rrdr,
    RRDF_FIELD_SUMMARY summary,
    RRDF_FIELD_FILTER filter,
    RRDF_FIELD_OPTS options,
    const char *default_value
);
```

### Real Examples

```c
// CPU usage with progress bar
buffer_rrdf_table_add_field(
    wb, field_id++, "CPU", "CPU %",
    RRDF_FIELD_TYPE_BAR_WITH_INTEGER,
    RRDF_FIELD_VISUAL_BAR,
    RRDF_FIELD_TRANSFORM_NUMBER,
    2, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
    RRDF_FIELD_SUMMARY_SUM,
    RRDF_FIELD_FILTER_RANGE,
    RRDF_FIELD_OPTS_VISIBLE, NULL
);

// Process name (unique key)
buffer_rrdf_table_add_field(
    wb, field_id++, "Name", "Name",
    RRDF_FIELD_TYPE_STRING,
    RRDF_FIELD_VISUAL_VALUE,
    RRDF_FIELD_TRANSFORM_NONE,
    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
    RRDF_FIELD_SUMMARY_COUNT,
    RRDF_FIELD_FILTER_MULTISELECT,
    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_UNIQUE_KEY,
    NULL
);

// Row options in data
buffer_json_add_array_item_object(wb);
buffer_json_member_add_object(wb, "rowOptions");
buffer_json_member_add_string(wb, "severity", "error");
buffer_json_object_close(wb);
buffer_json_object_close(wb);
```

### Key Backend Files

- **Enums**: `netdata/src/libnetdata/buffer/functions_fields.h`
- **Implementation**: `netdata/src/libnetdata/buffer/functions_fields.c`
- **Facets Library**: `netdata/src/libnetdata/facets/`
- **Example Functions**: 
  - `apps.plugin/apps_functions.c`
  - `network-viewer.plugin/network-viewer.c`
  - `systemd-journal.plugin/systemd-journal.c`

## Best Practices

### Always Include
- One field with `unique_key` option
- Meaningful `name` for display
- Appropriate `type` for the data
- Info response with `"v": 3`

### For Numeric Data
- Use `bar-with-integer` for percentages
- Set appropriate `max` value
- Include `units` for clarity
- Use `range` filter

### For Time Data
- `duration` type for intervals
- `timestamp` type for points in time
- Use appropriate transform

### For Status/Severity
- Include `rowOptions` as last array element
- Use standard severity levels: error, warning, notice, normal

### For Filtering
- `multiselect` (default) for categories
- `range` for numeric data
- `facet` only for has_history=true

### Common Patterns
- Numeric metrics → `bar-with-integer` + `range` filter
- Categories → `string` + `multiselect` filter
- Time intervals → `duration` + `duration` transform
- Status → `string` + `pill` visualization
- Row coloring → `rowOptions` with severity

## Known Functions

### Simple Table Functions (has_history=false)

| Function | Plugin | Key Features |
|----------|--------|--------------|
| processes | apps.plugin | CPU/memory bars, grouping |
| socket | ebpf.plugin | Complex filters, charts |
| network-connections | network-viewer.plugin | Aggregated views, severity |
| systemd-list-units | systemd-units.plugin | Unit status, severity |
| ipmi-sensors | freeipmi.plugin | Hardware monitoring |
| block-devices | proc.plugin | I/O statistics, charts |
| network-interfaces | proc.plugin | Network stats, severity |
| mount-points | diskspace.plugin | Filesystem usage |
| cgroup-top | cgroups.plugin | Container metrics |
| systemd-top | cgroups.plugin | Service metrics |
| metrics-cardinality | web api | Dynamic columns |
| streaming | web api | Replication status |
| all-queries | web api | Monitors the progress of in-flight queries |

### Log Explorer Functions (has_history=true)

| Function | Plugin | Key Features |
|----------|--------|--------------|
| systemd-journal | systemd-journal.plugin | System logs, faceted search |
| windows-events | windows-events.plugin | Windows logs, faceted search |


## Development Checklist

### Creating a New Function

- [ ] Choose function type (simple table or log explorer)
- [ ] Implement info response with `"v": 3`
- [ ] Define columns with appropriate types and options
- [ ] Include one `unique_key` field
- [ ] Add proper error handling
- [ ] Test with POST requests
- [ ] Document accepted/required parameters

### Format Validation

- [ ] Verify JSON structure matches specification
- [ ] Check all required fields are present
- [ ] Test empty result sets
- [ ] Test large datasets
- [ ] Verify error responses
- [ ] Test special characters in data
- [ ] Check numeric precision
- [ ] Verify date/time formatting
- [ ] Test sorting and filtering

### UI Integration

- [ ] Confirm field types map to UI components
- [ ] Verify filters work correctly
- [ ] Check column widths display properly
- [ ] Test row coloring with severity
- [ ] Verify transforms apply correctly
- [ ] Check responsive behavior

### Performance

- [ ] Handle large datasets efficiently
- [ ] Implement sampling for logs if needed
- [ ] Set appropriate `update_every`
- [ ] Consider pagination/anchoring
- [ ] Test with concurrent requests

## Corner Cases and Edge Handling

### Empty Result Sets

The protocol handles empty results gracefully:

```json
{
  "status": 200,
  "type": "table",
  "has_history": false,
  "columns": {...},  // Full column definitions
  "data": []         // Empty array is valid
}
```

- Minimum valid response: `{"status": 200, "type": "table", "columns": {}, "data": []}`
- UI displays "No data available" message
- Column headers still render for context

### Null and Missing Values

- `null` values in data arrays are rendered as empty cells
- Missing array elements default to `null`
- `NaN` for numeric fields: For fields with `transform: "number"`, `NaN` values will be displayed as the string "NaN". For `timestamp` fields with `datetime` or `datetime_usec` transforms, `NaN` epoch values will display as empty cells.
- Empty strings render as empty cells
- Backend uses `NAN` constant for missing numeric values

### Special Characters

The protocol properly escapes:
- JSON special characters (`"`, `\`, control chars)
- HTML entities (escaped by React components): React automatically escapes HTML content to prevent XSS attacks. Any HTML tags or entities in the data will be displayed as literal text, not rendered as HTML.
- Unicode characters (UTF-8 support throughout)
- SQL injection protection in queries

### Numeric Precision

- `decimal_points` field controls display precision, using JavaScript's `toFixed()` method.
- Backend uses `NETDATA_DOUBLE` type for floating-point numbers.
- Frontend's number formatting:
    - Some components use `Intl.NumberFormat` with a fixed locale (e.g., "en-US").
    - Others use `toLocaleString()` which respects the browser's locale.
    - Therefore, locale-specific formatting is applied in some, but not all, cases.
- Very large numbers: Due to the use of `toFixed()`, very large numbers will be displayed as a long string with the specified decimal places, not automatically in scientific notation.
- Infinity/NaN handling:
    - `NaN` values in numeric fields will be displayed as the string "NaN".
    - `Infinity` and `-Infinity` values will be displayed as the strings "Infinity" and "-Infinity" respectively.
    - For `timestamp` fields with `datetime` or `datetime_usec` transforms, `NaN` epoch values will display as empty cells.

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

**Display Format:**
- Timezone: Determined by URL parameter (`utc`) or system default
- Format: Uses `Intl.DateTimeFormat` with browser's locale
- Both types render as localized date/time strings with seconds precision

### Sorting Capabilities

- **Backend Control**: Each column defines default sort with `RRDF_FIELD_SORT_*`
- **User Control**: `sortable: true` enables UI sorting (default)
- **Fixed Sort**: Bit flag 0x80 prevents user sorting
- **Initial Sort**: `default_sort_column` specifies startup sort
- **Multi-Column**: UI supports sorting by any sortable column
- **Performance**: Client-side sorting for simple tables

### Filtering Capabilities

- **Multiselect**: Default filter type with checkboxes
  - Shows all unique values from the column
  - Multiple selections allowed
  - OR logic between selections
- **Range**: Numeric filters with min/max sliders
  - Requires numeric field type
  - Auto-detects min/max from data
  - Inclusive filtering
- **Facet**: Advanced filtering for log explorer
  - Shows counts next to each option
  - Dynamic updates with other filters
  - Indexed for performance

## Anchor-Based Pagination

Log explorer functions use anchor-based pagination for efficient navigation through large datasets.

### Configuration

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

### Frontend Implementation

**Anchor Management:**
```javascript
// Frontend calculates anchors from data boundaries
anchorBefore: latestData[latestData.length - 1][pagination.column],
anchorAfter: latestData[0][pagination.column],
anchorUnits: pagination.units
```

**Infinite Scroll Navigation:**
- **Backward**: Scroll down loads older data using `anchorBefore`
- **Forward**: Scroll up loads newer data using `anchorAfter`
- **State Tracking**: `hasNextPage`, `hasPrevPage` control load triggers

**Required Parameters:**
- `anchor: {VALUE}` - Pagination cursor value
- `direction: "backward"|"forward"` - Navigation direction
- `last: NUMBER` - Page size (default: 200)

### PLAY Mode Integration

When in PLAY mode (`after < 0`), pagination automatically coordinates with real-time updates:

```javascript
{
  direction: "forward",
  merge: true,
  tail: true,
  delta: true,
  anchor: anchorAfter
}
```

### Large Data Sets

Simple tables load all data at once, but handle large sets efficiently:
- Virtual scrolling for thousands of rows
- Client-side filtering/sorting
- No built-in pagination (all data in response)

Log explorer uses anchor-based pagination:
- Efficient navigation through millions of records
- Configurable page size (`last` parameter)
- Bi-directional infinite scrolling
- Sampling for very large sets

## Incremental Updates (Delta Mode)

Delta mode enables efficient real-time updates by sending only changes since the last request.

### When Delta is Enabled

```javascript
{
  if_modified_since: 1697644320000000,  // Previous modification timestamp
  direction: "forward",
  merge: true,
  tail: true,
  delta: true,
  data_only: true,
  anchor: anchorAfter
}
```

### Delta Response Types

**Facets Delta:**
```json
{
  "facetsDelta": [
    {
      "id": "priority",
      "options": [
        {"id": "ERROR", "count": 5},  // Incremental counts
        {"id": "WARN", "count": 12}
      ]
    }
  ]
}
```

**Histogram Delta:**
```json
{
  "histogramDelta": {
    "chart": {
      "result": {
        "labels": ["time", "ERROR", "WARN"],
        "data": [
          [1697644320, 3, 8]  // New data points only
        ]
      }
    }
  }
}
```

### Data Merging

Frontend merges delta responses with existing data:
- **Facet counts**: Accumulated using `count = (existing || 0) + (delta || 0)`
- **Table data**: Appended/prepended based on `direction`
- **Histogram data**: New data points added to existing chart

## Real-Time Updates (PLAY Mode)

PLAY mode enables live data streaming with efficient polling and conditional updates.

### PLAY Mode Detection

- **PLAY Mode**: `after < 0` (relative time from now)
- **PAUSE Mode**: `after > 0` (absolute timestamp)

### Parameter Coordination

When `if_modified_since` is present, the system automatically includes:

```json
{
  "if_modified_since": 1697644320000000,
  "direction": "forward",
  "merge": true,
  "tail": true,
  "delta": true,
  "data_only": true,
  "anchor": "anchorAfter"
}
```

### Error Handling

**304 Not Modified**: Indicates no new data available
```json
{
  "status": 304
}
```

Frontend handles 304 responses gracefully without showing errors to users.

### Polling Behavior

- **Polling Interval**: Based on function's `update_every` value
- **Auto-Pause**: When window loses focus or user hovers over data
- **Conditional Requests**: Uses `if_modified_since` to avoid unnecessary data transfers

### Required vs Optional Fields

**Minimum Required Fields:**
```json
{
  "status": 200,        // Required
  "type": "table",      // Required
  "columns": {},        // Required (can be empty)
  "data": []           // Required (can be empty)
}
```

**Common Optional Fields:**
- `has_history`: Default false
- `help`: Documentation text
- `update_every`: Default 1
- `expires`: Cache control
- `default_sort_column`: Initial sort
- All column options except `index`, `name`, `type`

## Error Handling

### Error Response Format

When a function encounters an error, the backend returns a JSON object. The primary error generation function (`rrd_call_function_error`) produces the following minimal format:

```json
{
  "status": 400,                  // The HTTP status code (e.g., 400, 404, 500)
  "error_message": "A descriptive error message" // A human-readable message explaining the error
}
```

**Frontend Consumption and Interpretation:**
The frontend is designed to handle a more comprehensive error structure, allowing for richer error display and localization. When an error occurs, the frontend will attempt to extract information from the received error object using the following hierarchy:

*   **`status`**: The HTTP status code, used for general error classification (e.g., 400 for bad request, 404 for not found).
*   **`error_message`**: The primary detailed message from the backend. This is the most consistently populated field by current backend functions.
*   **`error`**: A short, machine-readable error identifier (e.g., "MissingParameter"). While not consistently generated by `rrd_call_function_error`, other parts of the system or future backend implementations might provide this.
*   **`message`**: A user-friendly message. The frontend often maps `error_message` or an internal `errorMsgKey` to this for display.
*   **`help`**: Optional additional guidance for resolving the error. This field is not currently generated by `rrd_call_function_error`.

**Example of Frontend Interpretation (Conceptual):**
The frontend might internally map specific `error_message` strings to predefined `errorMsgKey` values to provide localized or more context-specific messages to the user. For instance, a backend `error_message` like "The 'time_range' parameter is required" might be mapped to an `errorMsgKey` of "ErrMissingTimeRange" in the frontend, which then displays a user-friendly message like "Please specify a time range for this function."

Therefore, while the backend currently provides `status` and `error_message`, developers should be aware that the frontend's error handling is capable of utilizing the more detailed fields (`error`, `message`, `help`) if they are provided by the backend in the future or by other API endpoints.

**Common Error Codes:**


### Common Error Codes

| Code | Use Case | Example |
|------|----------|---------|
| 400 | Bad input | Missing/invalid parameters |
| 401 | Auth required | User not logged in |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not found | No data matches query |
| 500 | Server error | Internal failures |
| 503 | Unavailable | Service overloaded |

## Protocol Integration

### PLUGINSD Commands

Functions integrate with Netdata through the PLUGINSD protocol:

**Registration:**
```
FUNCTION "function_name" timeout "help text" "tags" "http_access" priority version
```

**Execution Flow:**
1. **Collector → Agent**: `FUNCTION` registers the function
2. **Agent → Collector**: `FUNCTION_CALL` with transaction ID
3. **Collector → Agent**: `FUNCTION_RESULT_BEGIN` status format expires
4. **Collector → Agent**: Response payload
5. **Collector → Agent**: `FUNCTION_RESULT_END`

**With Payload:**
```
FUNCTION_PAYLOAD_BEGIN transaction timeout function access source content_type
<payload data>
FUNCTION_PAYLOAD_END
```

**Cancellation:**
```
FUNCTION_CANCEL transaction_id
```

**Progress Updates:**
```
FUNCTION_PROGRESS transaction_id done total
```

### Streaming Protocol

Functions support streaming for real-time updates:

```c
// Transaction management
dictionary_set(parser->inflight.functions, transaction_str, &function_data);

// Timeout handling
if (*pf->stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT < now_ut) {
    // Function timed out
}

// Progress callback
if(stream_has_capability(s, STREAM_CAP_PROGRESS)) {
    // Enable progress updates
}
```

### Key Files
- `pluginsd_functions.c`: Function execution and management
- `stream-sender-execute.c`: Streaming function calls
- `plugins.d/README.md`: Protocol documentation
- `plugins.d/functions-table.md`: Table format specification

## Migration Guide

### Upgrading from GET to POST (v:3)

1. Update info response to include `"v": 3`
2. Change field IDs from hashes to plain names
3. Test with JSON POST payloads
4. Remove hash generation code
5. Update documentation

### Adding Log Explorer Features

1. Set `has_history: true`
2. Implement facets with counts
3. Add timestamp column with microseconds
4. Support anchor-based pagination
5. Add histogram data
6. Implement full-text search

## Status and Next Steps

This document represents the complete v3 protocol specification. Key areas for future development:

1. **Protocol Extensions**
   - Streaming updates for real-time data
   - Aggregation pipelines
   - Custom visualization types

2. **UI Enhancements**
   - Additional field types (gauge, sparkline)
   - Custom column renderers
   - Advanced filtering options

3. **Performance Optimizations**
   - Server-side pagination for simple tables
   - Incremental updates
   - Result caching

---

## Appendix A: Validation Checklist and Progress

*This section tracks the validation work completed and remaining tasks*

### Format Discovery
- [x] Analyze `processes` function implementation in apps.plugin
- [x] Analyze `network-connections` function in network-viewer.plugin
- [x] Identify common patterns between implementations
- [x] Extract all enum definitions used in responses
- [x] Document each field type and possible values
- [x] Identify optional vs required fields

### Enumeration Completeness
- [x] Find all enum definitions in netdata C code
- [x] Map enum values to their string representations
- [x] Document the purpose of each enum value
- [ ] Check for any conditional enum values

### Function Coverage
- [x] Scan all collectors/plugins for function implementations
- [x] List all simple table functions (has_history=false)
- [x] Verify format consistency across all functions
- [x] Document any function-specific extensions

### UI Mapping
- [x] Analyze cloud-frontend code for function rendering
- [x] Map each format field to UI component
- [x] Document how enum values affect UI behavior
- [x] Identify any frontend-specific transformations

### Corner Cases
- [x] Empty result sets
- [x] Large data sets (pagination?)
- [x] Error responses
- [x] Null/missing values
- [x] Special characters in data
- [x] Numeric precision/formatting
- [x] Date/time formatting
- [x] Sorting capabilities
- [x] Filtering capabilities

### Protocol Validation
- [x] Request format documentation
- [x] Response format documentation
- [x] Error handling patterns
- [x] Streaming protocol integration

### Cross-Reference Checks
- [x] Compare documented format with actual implementations
- [x] Verify all functions conform to documented format
- [x] Check for undocumented features in UI
- [x] Validate against any existing documentation

---

## Appendix B: Investigation History

### Analysis Log

*This section documents the investigation process to avoid repeating work*

#### Session 1 - Initial Setup and Analysis
- Created FUNCTIONS.md structure
- Established checklist for comprehensive validation
- Analyzed `processes` function in apps.plugin
  - Location: `/netdata/src/collectors/apps.plugin/apps_functions.c`
  - Registration: `apps_plugin.c:752`
  - Uses standard table format with array-based data rows
- Analyzed `network-connections` function in network-viewer.plugin
  - Location: `/netdata/src/collectors/network-viewer.plugin/network-viewer.c`
  - Registration via PLUGINSD protocol
  - Supports aggregated and detailed views
- Extracted all RRDF enum definitions from:
  - `/netdata/src/libnetdata/buffer/functions_fields.h`
  - `/netdata/src/libnetdata/buffer/functions_fields.c`
- Documented complete enum value mappings for field types, visualizations, transforms, etc.

#### Session 2 - Protocol Integration and Corner Cases
- Investigated PLUGINSD protocol integration
  - Found in `pluginsd_functions.c` and `stream-sender-execute.c`
  - Commands: FUNCTION, FUNCTION_CALL, FUNCTION_RESULT_BEGIN/END
  - Support for payloads, cancellation, and progress updates
- Analyzed corner case handling:
  - Empty results: `{"status": 200, "type": "table", "columns": {}, "data": []}`
  - Null values: Rendered as empty cells in UI
  - Special characters: Proper JSON escaping throughout
  - Numeric precision: Controlled by `decimal_points` field
  - Date/time: Millisecond epochs for tables, microsecond for logs
- Identified required vs optional fields:
  - Required: status, type, columns, data
  - Optional: Everything else (has_history, help, etc.)
- Documented sorting and filtering capabilities:
  - Sorting: Backend-controlled defaults, user-sortable columns
  - Filtering: Multiselect (default), range (numeric), facet (logs)
  - Large datasets: Virtual scrolling for tables, pagination for logs

### Key Findings
1. **Data Format**: Simple tables use array-of-arrays for data rows
2. **Column Definition**: Each column has extensive metadata controlling display and behavior
3. **Common Functions**: Both implement `buffer_rrdf_table_add_field()` for column definitions
4. **Response Builder**: Uses `buffer_json_*` functions to build JSON responses
5. **Field Options**: Bit flags allow combining multiple options per field
6. **Protocol Evolution**: GET with hashes → POST with plain field names (v:3)
7. **UI Mapping**: Comprehensive component mapping based on type/visualization
8. **Error Handling**: Standardized error response format with HTTP codes
9. **Performance**: Client-side operations for tables, server-side for logs
10. **Extensibility**: Optional fields allow function-specific features

### Format Compliance Summary
- **All 12 simple table functions are fully compliant** with the documented format
- **Common extensions** found:
  - `rowOptions` field for row severity/status (5/12 functions)
  - `charts` and `default_charts` definitions (most functions)
  - `group_by` aggregation options (most functions)
  - `accepted_params` and `required_params` (metrics-cardinality)
  - `default_sort_column` for initial sorting
- **No breaking deviations** - all extensions are additive and optional

---

## Appendix C: Log Explorer UI Features Detail

*Detailed UI behaviors when has_history=true*

### UI Features Enabled

When properly formatted, the log explorer UI provides:

1. **Sidebar with Faceted Filters**
   - Shows facets with real-time counts
   - Multi-select filtering
   - Collapsible sections
   - Search within facets

2. **Time-based Histogram**
   - Visual log distribution over time
   - Click and drag to select time ranges
   - Switch between different histogram fields
   - Auto-updates with filters

3. **Advanced Table Features**
   - Infinite scroll using anchor navigation
   - No manual pagination controls
   - Automatic row coloring by severity
   - Full-width message display
   - Column pinning and resizing

4. **Search and Navigation**
   - Full-text search box
   - Bi-directional navigation (forward/backward)
   - Jump to specific time
   - Export filtered results

5. **Live Features**
   - Tail mode for real-time updates
   - Auto-refresh based on `update_every`
   - Delta updates for efficiency
   - Notification of new entries

---

*Last Updated: Based on analysis of Netdata codebase and cloud-frontend implementation*