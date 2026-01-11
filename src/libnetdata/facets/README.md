# Facets and Logs Query System

The facets engine provides a unified interface for querying, searching, and analyzing log data from multiple sources including systemd journal (Linux) and Windows Event logs.

## Overview

The logs query system uses a two-phase approach:
1. **Discovery Phase**: Get information about available log sources
2. **Query Phase**: Query the actual log data

## Discovery Phase (info=true)

When called with `info=true`, the system returns metadata about available log sources and accepted parameters.

### Response Structure

```json
{
    "_request": {
        // Echo of the request parameters
    },
    "versions": {
        "sources": 1748491820000467  // Version timestamp for source list
    },
    "v": 3,  // API version
    "accepted_params": [
        // List of all parameters the API accepts
    ],
    "required_params": [
        // Array of required parameter definitions
    ],
    "show_ids": false,
    "has_history": true,
    "pagination": {
        "enabled": true,
        "key": "anchor",
        "column": "timestamp",
        "units": "timestamp_usec"
    },
    "status": 200,
    "type": "table",
    "help": "..."
}
```

### Platform-Specific Sources

#### Linux (systemd journal)
Sources are hierarchical and represent different views of journal data:
- `all` - All available logs
- `all-local-logs` - All local logs
- `all-local-namespaces` - Namespace-specific logs
- `all-local-system-logs` - System logs only
- `all-local-user-logs` - User logs only
- `namespace-*` - Specific namespace logs
- `remote-*` - Remote logs (if available)

Each source includes:
- File count
- Total size
- Time coverage (e.g., "1y 6mo 8d 16h 21m 18s")

#### Windows (Event logs)
Sources are Windows Event channels organized by provider:
- `All` - All event channels
- `All-Admin` - Administrative channels
- `All-Classic` - Classic event logs (Application, Security, System)
- `All-Operational` - Operational channels
- Provider-specific channels (e.g., `Microsoft-Windows-*/Operational`)
- Application channels (e.g., `Netdata/Health`, `Netdata/Daemon`)

Each source includes:
- Channel count
- Total size  
- Time coverage
- **Entry count** (unique to Windows)
- `default_selected` flag (indicates if selected by default)

**Windows Events Performance Optimization:**
- Windows Events have few native fields (Level, TimeCreated, EventID, etc.)
- Additional data is stored in XML format within each event
- XML parsing is expensive, so windows-events.plugin uses **lazy loading**:
  - XML is parsed only for rows that will be returned to the user
  - For full-text search, XML is fetched and searched but not parsed
  - Field extraction from XML happens only for visible rows
- This approach balances search capability with performance

## Query Parameters

### Core Parameters

#### Required Parameters
- `__logs_sources` - Multiselect field for choosing log sources to query

#### Time Filtering
- `after` - Start timestamp (Unix seconds)
- `before` - End timestamp (Unix seconds)

#### Pagination
- `anchor` - Pagination anchor (timestamp in microseconds)
- `direction` - "backward" (newest first) or "forward"
- `last` - Number of entries to return (default: 200)

#### Search and Analysis
- `query` - Full-text search query
- `facets` - Array of field names to analyze and return facet counts
- `histogram` - Field name for generating time-based histogram (default: `_PRIORITY` for Linux, `Level` for Windows)

#### Options
- `data_only` - Return only log data without facet analysis
- `delta` - Return incremental updates
- `tail` - Follow mode for real-time updates
- `sampling` - Sample rate for large datasets (default: 1000000)
- `slice` - Time slicing for analysis
- `if_modified_since` - Conditional requests

### Default Values
When parameters are not specified:
- `source_type`: 1 (platform default)
- `direction`: "backward" (newest entries first)
- `last`: 200 entries
- `sampling`: 1000000
- Time range: Last 15 minutes (900 seconds)

## Auto-Selection Behavior

The system is designed to work without user intervention:

1. **Source Auto-Selection**:
   - If no sources are selected, the UI/client should select the first available source
   - On Windows, sources with `default_selected: true` should be pre-selected

2. **Immediate Data Fetch**:
   - After the `info` call, clients should immediately fetch data
   - Use default or auto-selected sources
   - Apply default time range and pagination

## Query Response Format

The query response includes:
- Faceted results with counts (unless `data_only=true`)
- Log entries matching the query
- Histogram data with breakdown per facet value (unless `data_only=true`)
- Pagination information for fetching more results
- Source failures return partial data (no explicit error indication)

## Usage Example Flow

1. **Get available sources**:
   ```
   GET /api/v1/logs?info=true
   ```

2. **Parse response and auto-select sources**:
   - Use sources with `default_selected: true` (Windows)
   - Or select first source (Linux)

3. **Query logs**:
   ```
   POST /api/v1/logs
   {
     "__logs_sources": ["Application", "System"],
     "after": 1748527000,
     "before": 1748528000,
     "query": "error",
     "last": 100
   }
   ```

## Modes of Operation

### Fast Data Query (`data_only=true`)
The fastest query mode that seeks directly to the anchor point (or time boundary) and returns the next `last` entries in the specified `direction`:
- Does not scan the entire time window
- No facet calculation unless `delta=true` is specified
- Uses learned out-of-order deltas to minimize data scanning
- Ideal for pagination and real-time updates

### Full Analysis Query (`data_only=false`) - Default
Scans the entire time window to calculate complete facet counts and histogram data:
- Returns the same `last` entries as data_only mode
- Provides comprehensive statistics for the entire time range
- Required for initial queries to understand data distribution
- Must complete full scan due to potential out-of-order entries

### Real-time Following (`tail=true`)
Combined with `data_only=true` and `delta=true` for efficient log following:
- Scans only new entries since last anchor
- When used with `if_modified_since`, returns HTTP 304 if no changes
- Change detection:
  - Linux: Uses inotify for file watching
  - Windows: Polls providers for latest timestamps
- Equivalent to `tail -f` or `journalctl -f`
- Clients typically poll once per second for updates

## Advanced Parameters

### Sampling (`sampling`)
Controls when statistical sampling begins:
- Default: 1,000,000 entries (unusually high for accurate results)
- Sampling algorithm (systemd-journal only):
  - Estimates volume per journal file based on time window
  - Distributes sampling proportionally across files
  - Maintains temporal representation across the dataset
- Provides accurate counts up to the sampling threshold
- Above threshold, provides statistically representative estimates
- Sampling stages:
  - First stage: Skip facet processing, continue row counting
  - Second stage: Skip rows, estimate counts
  - Histogram shows additional dimensions: `unsampled` and `estimated`

### Slicing (`slice`)
Database-level filtering optimization (Linux only):
- `slice=false` (default):
  - Facets library reads all data
  - Knows counts for all facet values (selected and non-selected)
  - Shows all possible filter options to users
- `slice=true` (when backend supports it):
  - Database uses indexes to filter data
  - Faster queries for filtered datasets
  - Non-selected facet values may show as zero
  - Backend provides list of all possible values separately

Windows Events note: Does not support slicing; Netdata maintains internal cache of possible facet values.

## Performance Characteristics

### Processing Speed
- Typical: ~200,000 rows/second on modern hardware
- Factors affecting speed:
  - Query complexity
  - Number of sources
  - Time window size
  - Filtering and facets

### Progress Reporting
For long-running queries:
- UI can request progress updates
- Each progress check extends the timeout
- Immediate cancellation on user action
- Prevents timeout during active monitoring

### Out-of-Order Data Handling
Log databases often contain out-of-order entries:
- Plugins learn maximum out-of-order deltas per source
- Linux: Per journal file
- Windows: Per event provider
- Enables efficient minimal scanning for `data_only=true` queries

## Implementation Notes

- The backend automatically detects the platform and returns appropriate sources
- The same query interface works for both systemd journal and Windows Events
- Sources can change over time (tracked by `versions.sources`)
- Pagination uses microsecond timestamps for precise positioning
- The system supports real-time following with `tail=true`
- All caching and optimization is transparent to the caller
- Authentication handled via USER_AUTH structure from Netdata Cloud SSO
- Plugins run with root/Administrator privileges for full data access
- User preferences (filters, time windows, facets) persist at dashboard level
- All queries are logged for audit purposes (standard system logging)

## Integration with Netdata Architecture

### Plugin Communication
- The facets library runs within plugins
- Plugins can run anywhere in the Netdata ecosystem (parent nodes, child nodes, etc.)
- Communication with plugins happens via **rrdfunctions** - Netdata's function execution framework
- rrdfunctions provide the transport layer to send requests to plugins and receive responses

### MCP (Model Context Protocol) Integration
- Netdata has an MCP server implementation for LLM interactions
- Functions that return `has_history=true` (like logs) are currently excluded from MCP's table processing
- Regular functions return simple table format: `{"type": "table", "data": [...], "columns": {...}}`
- Logs functions return a different JSON structure with additional fields for:
  - Faceted search results
  - Histogram data
  - Pagination information (anchors)
  - Time-based navigation
  - Dynamic field discovery

#### JSON Structure Differences: Regular Tables vs Logs

**Regular Table Functions** (e.g., processes, network connections):
```json
{
  "status": 200,
  "type": "table",
  "has_history": false,
  "columns": { /* column definitions */ },
  "data": [ /* simple arrays of values */ ],
  "charts": { /* optional chart configs */ }
}
```

**Logs Functions** (e.g., systemd-journal, Windows events):
```json
{
  "status": 200,
  "type": "table",
  "has_history": true,
  "_request": { /* complete request parameters */ },
  "columns": { /* column definitions with facet support */ },
  "data": [ 
    /* arrays starting with timestamp, rowOptions, then values */
  ],
  "facets": { /* available filters with counts */ },
  "histogram": { /* time-series visualization */ },
  "pagination": { /* anchor-based navigation */ },
  "_journal_files": { /* source metadata */ },
  "_sampling": { /* sampling statistics */ },
  "items": 12345,
  "last_modified": 1234567890,
  /* many more metadata fields */
}
```

Key differences:
- Logs have 20+ additional top-level fields
- Data rows include timestamps and metadata
- Built-in support for faceted filtering and time navigation
- Rich metadata about data sources and query performance

### Dynamic Schema Challenge
- systemd-journal can store any structured data with custom fields
- Plugins discover new fields dynamically as they process data
- LLMs need a way to discover available fields to build intelligent queries
- Current MCP implementation doesn't handle this dynamic schema discovery

#### Example: Schema Variability in systemd-journal
The same systemd-journal can contain completely different datasets with different schemas:

1. **Standard System Logs** - Traditional journal fields:
   - System fields: `_HOSTNAME`, `_UID`, `_GID`, `_PID`, `_COMM`, `_EXE`
   - Message fields: `MESSAGE`, `PRIORITY`, `SYSLOG_FACILITY`, `SYSLOG_IDENTIFIER`
   - Systemd fields: `_SYSTEMD_UNIT`, `_SYSTEMD_CGROUP`, `_SYSTEMD_SLICE`
   - Boot/runtime fields: `_BOOT_ID`, `_MACHINE_ID`, `_RUNTIME_SCOPE`
   
2. **Netdata Agent Events** - Custom application data with `AE_` prefix:
   - Agent metadata: `AE_AGENT_ID`, `AE_AGENT_VERSION`, `AE_AGENT_STATUS`
   - Hardware info: `AE_HW_BOARD_NAME`, `AE_HW_CHASSIS_TYPE`, `AE_HW_SYS_VENDOR`
   - Cloud/container: `AE_HOST_CLOUD_PROVIDER`, `AE_HOST_CONTAINER`, `AE_AGENT_KUBERNETES`
   - Crash analytics: `AE_AGENT_CRASHES`, `AE_FATAL_FAULT_ADDRESS`, `AE_FATAL_THREAD`
   - Performance: `AE_AGENT_UPTIME`, `AE_AGENT_TIMINGS_INIT`, `AE_AGENT_TIMINGS_EXIT`

3. **Other Structured Data** - Any application can log structured data:
   - Netdata alerts: `ND_ALERT_NAME`, `ND_ALERT_STATUS`, `ND_ALERT_CLASS`
   - Custom applications: Arbitrary fields specific to each application
   - IoT devices: Sensor readings, device states, telemetry data
   - Business applications: Transaction IDs, user actions, audit trails

This variability means:
- The same logs query API must handle completely different schemas
- Fields available for filtering/faceting vary by dataset
- LLMs need to discover what fields exist before building meaningful queries
- Traditional fixed-schema approaches don't work

#### Platform-Specific Schema Characteristics

**Linux (systemd-journal)**:
- All fields are native journal fields - no lazy loading needed
- Can have **thousands** of different fields in a single dataset
- Multiple datasets can be queried in parallel (multiplexed, interleaved)
- Fields are discovered dynamically as data is processed
- Fast field access and filtering
- This massive scalability and flexibility makes systemd-journal extremely powerful as a structured data store

**Windows (Event logs)**:
- Limited native fields (Level, TimeCreated, EventID, Provider, etc.)
- Rich data stored in XML format within each event
- Lazy XML parsing for performance:
  - Full-text search scans XML without parsing
  - XML parsing happens only for returned rows
  - Balances search capability with performance constraints
- Field discovery requires XML inspection

### Future Direction
- Evolve MCP's function processing to handle both regular tables and logs uniformly
- Create MCP tools that:
  - Support dynamic field discovery
  - Enable faceted search and analysis
  - Provide intelligent query building based on discovered schema
  - Handle pagination and time-based navigation
  - Leverage the full power of the facets engine
