# AS/400 Netdata Functions Implementation Plan

## Single Function with Data Source Selector

Instead of multiple functions, implement a single `as400` function with a required `__data_source` parameter to select which AS/400 entity to view.

## Implementation Pattern (Based on network-viewer)

### 1. Function Registration

Register a single function in the AS/400 collector:

```go
// In as400/module.go or collector initialization
fnReg.Register("as400", c.functionAS400View)
```

### 2. Function Declaration to Netdata

```
FUNCTION GLOBAL "as400" 10 "AS/400 system viewer" "top" "member" 100 3
```

### 3. Info Request Response

When Netdata requests `as400 info`, respond with the required parameter:

```c
buffer_json_member_add_array(wb, "accepted_params");
{
    buffer_json_add_array_item_string(wb, "__data_source");
    buffer_json_add_array_item_string(wb, "after");
    buffer_json_add_array_item_string(wb, "before");
    buffer_json_add_array_item_string(wb, "query");
}
buffer_json_array_close(wb);

buffer_json_member_add_array(wb, "required_params");
{
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_string(wb, "id", "__data_source");
        buffer_json_member_add_string(wb, "name", "Data Source");
        buffer_json_member_add_string(wb, "help", "Select the AS/400 data to view");
        buffer_json_member_add_boolean(wb, "unique_view", true);
        buffer_json_member_add_string(wb, "type", "select");
        buffer_json_member_add_array(wb, "options");
        {
            // Active SQL Statements - ALL with elapsed time
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "active_sql");
                buffer_json_member_add_string(wb, "name", "Active SQL Statements (All)");
                buffer_json_member_add_boolean(wb, "defaultSelected", true);
                buffer_json_member_add_string(wb, "category", "sql");
            }
            buffer_json_object_close(wb);

            // Jobs in Error/Wait Status (CRITICAL)
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "error_wait_jobs");
                buffer_json_member_add_string(wb, "name", "Jobs in Error/Wait Status");
                buffer_json_member_add_string(wb, "category", "critical");
            }
            buffer_json_object_close(wb);

            // Message Queues with Errors (CRITICAL)
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "error_messages");
                buffer_json_member_add_string(wb, "name", "Error Messages (Severity > 40)");
                buffer_json_member_add_string(wb, "category", "critical");
            }
            buffer_json_object_close(wb);

            // Lock Waits & Deadlocks (CRITICAL)
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "lock_waits");
                buffer_json_member_add_string(wb, "name", "Lock Waits & Deadlocks");
                buffer_json_member_add_string(wb, "category", "critical");
            }
            buffer_json_object_close(wb);

            // High Resource Jobs (WARNING)
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "high_resource_jobs");
                buffer_json_member_add_string(wb, "name", "High CPU/Memory Jobs");
                buffer_json_member_add_string(wb, "category", "warning");
            }
            buffer_json_object_close(wb);

            // All Active Jobs with elapsed time
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "active_jobs");
                buffer_json_member_add_string(wb, "name", "All Active Jobs (with elapsed time)");
                buffer_json_member_add_string(wb, "category", "monitoring");
            }
            buffer_json_object_close(wb);

            // Subsystem Issues (CRITICAL)
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "subsystem_issues");
                buffer_json_member_add_string(wb, "name", "Subsystem Issues");
                buffer_json_member_add_string(wb, "category", "critical");
            }
            buffer_json_object_close(wb);

            // All Subsystems
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "subsystems");
                buffer_json_member_add_string(wb, "name", "All Subsystems");
                buffer_json_member_add_string(wb, "category", "monitoring");
            }
            buffer_json_object_close(wb);

            // Job Queues
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "job_queues");
                buffer_json_member_add_string(wb, "name", "Job Queues");
            }
            buffer_json_object_close(wb);

            // Message Queues
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "message_queues");
                buffer_json_member_add_string(wb, "name", "Message Queues");
            }
            buffer_json_object_close(wb);

            // Output Queues
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "output_queues");
                buffer_json_member_add_string(wb, "name", "Output Queues (Spool Files)");
            }
            buffer_json_object_close(wb);

            // Disk Units
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "disk_units");
                buffer_json_member_add_string(wb, "name", "Disk Units");
            }
            buffer_json_object_close(wb);

            // Network Interfaces
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "network_interfaces");
                buffer_json_member_add_string(wb, "name", "Network Interfaces");
            }
            buffer_json_object_close(wb);

            // HTTP Servers
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "http_servers");
                buffer_json_member_add_string(wb, "name", "HTTP Servers");
            }
            buffer_json_object_close(wb);

            // Plan Cache Performance Issues
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "plan_cache_issues");
                buffer_json_member_add_string(wb, "name", "SQL Plan Cache - Poor Performers");
                buffer_json_member_add_string(wb, "category", "performance");
            }
            buffer_json_object_close(wb);

            // System Resource Alerts (CRITICAL)
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "system_alerts");
                buffer_json_member_add_string(wb, "name", "System Resource Alerts");
                buffer_json_member_add_string(wb, "category", "critical");
            }
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb); // options
    }
    buffer_json_object_close(wb);
}
buffer_json_array_close(wb); // required_params
```

### 4. Function Handler

```go
func (c *Collector) functionAS400View(fn functions.Function) {
    // Parse the function call to get the data source
    var dataSource string
    var params struct {
        DataSource string   `json:"__data_source"`
        Query      string   `json:"query"`
        After      int64    `json:"after"`
        Before     int64    `json:"before"`
    }

    // Handle info request
    if strings.Contains(fn.Name, "info") {
        c.sendAS400InfoResponse(fn)
        return
    }

    // Parse parameters (v3 POST with JSON)
    if fn.ContentType == "application/json" {
        json.Unmarshal(fn.Payload, &params)
        dataSource = params.DataSource
    } else {
        // Parse GET parameters (legacy)
        // Format: "__data_source:active_jobs"
        parts := strings.Split(fn.Name, " ")
        for _, part := range parts {
            if strings.HasPrefix(part, "__data_source:") {
                dataSource = strings.TrimPrefix(part, "__data_source:")
                break
            }
        }
    }

    // Route to appropriate handler based on data source
    switch dataSource {
    // SQL Performance
    case "active_sql":
        c.handleActiveSQL(fn, params)  // ALL SQL with elapsed time
    case "error_wait_jobs":
        c.handleErrorWaitJobs(fn, params)
    case "error_messages":
        c.handleErrorMessages(fn, params)
    case "lock_waits":
        c.handleLockWaits(fn, params)
    case "subsystem_issues":
        c.handleSubsystemIssues(fn, params)
    case "system_alerts":
        c.handleSystemAlerts(fn, params)

    // WARNING - Resource Issues
    case "high_resource_jobs":
        c.handleHighResourceJobs(fn, params)
    case "disk_units":
        c.handleDiskUnits(fn, params)
    case "output_queues":
        c.handleOutputQueues(fn, params)

    // PERFORMANCE - Analysis
    case "plan_cache_issues":
        c.handlePlanCacheIssues(fn, params)

    // MONITORING - General
    case "active_jobs":
        c.handleActiveJobs(fn, params)
    case "subsystems":
        c.handleSubsystems(fn, params)
    case "job_queues":
        c.handleJobQueues(fn, params)
    case "message_queues":
        c.handleMessageQueues(fn, params)
    case "network_interfaces":
        c.handleNetworkInterfaces(fn, params)
    case "http_servers":
        c.handleHTTPServers(fn, params)
    default:
        c.sendErrorResponse(fn, 400, "Invalid or missing __data_source parameter")
    }
}
```

### 5. Data-Specific Handlers

Each handler returns a table format specific to its data:

```go
func (c *Collector) handleActiveSQL(fn functions.Function, params QueryParams) {
    // Shows ALL active SQL statements with elapsed time column
    // Users can sort/filter to find long runners
    // Row colors: red >5min, yellow >1min, blue >10sec

    ctx := context.Background()

    // Query all SQL jobs - no filtering by elapsed time
    query := `
        SELECT
            JOB_NAME,
            JOB_USER,
            JOB_STATUS,
            ELAPSED_TIME,
            SQL_STATEMENT_TEXT,
            CPU_PERCENTAGE,
            TEMPORARY_STORAGE
        FROM TABLE(QSYS2.ACTIVE_JOB_INFO()) X
        WHERE SQL_STATEMENT_TEXT IS NOT NULL
           OR FUNCTION LIKE 'QZD%'
        ORDER BY ELAPSED_TIME DESC
    `

    // Let the dashboard handle filtering and sorting
    // Default sort by elapsed_time descending shows worst offenders first
}

func (c *Collector) handleActiveJobs(fn functions.Function, params QueryParams) {
    ctx := context.Background()
    jobs, err := c.client.GetActiveJobs(ctx)
    if err != nil {
        c.sendErrorResponse(fn, 500, err.Error())
        return
    }

    // Build response
    var buf bytes.Buffer
    wb := bufferCreate(&buf)

    buffer_json_initialize(wb)
    buffer_json_member_add_int64(wb, "status", 200)
    buffer_json_member_add_string(wb, "type", "table")
    buffer_json_member_add_boolean(wb, "has_history", false)
    buffer_json_member_add_int64(wb, "update_every", 1)

    // Define columns specific to active jobs
    buffer_json_member_add_object(wb, "columns")
    {
        // Job Name column
        buffer_json_member_add_object(wb, "job_name")
        {
            buffer_json_member_add_int64(wb, "index", 0)
            buffer_json_member_add_string(wb, "name", "Job Name")
            buffer_json_member_add_string(wb, "type", "string")
            buffer_json_member_add_boolean(wb, "unique_key", true)
            buffer_json_member_add_string(wb, "filter", "multiselect")
            buffer_json_member_add_boolean(wb, "sortable", true)
        }
        buffer_json_object_close(wb)

        // Add other columns: subsystem, status, cpu%, memory, etc.
    }
    buffer_json_object_close(wb) // columns

    // Add data
    buffer_json_member_add_array(wb, "data")
    {
        for _, job := range jobs {
            // Apply query filter if provided
            if params.Query != "" && !matchesQuery(job, params.Query) {
                continue
            }

            buffer_json_add_array_item_array(wb)
            {
                buffer_json_add_array_item_string(wb, job.Name)
                buffer_json_add_array_item_string(wb, job.Subsystem)
                buffer_json_add_array_item_string(wb, job.Status)
                buffer_json_add_array_item_double(wb, job.CPUPercent)
                buffer_json_add_array_item_int64(wb, job.MemoryMB)
                buffer_json_add_array_item_int64(wb, job.ElapsedSeconds)
                buffer_json_add_array_item_int64(wb, job.ThreadCount)

                // Add row options for severity
                if job.Status == "MSGW" {
                    buffer_json_add_array_item_object(wb)
                    {
                        buffer_json_member_add_object(wb, "rowOptions")
                        {
                            buffer_json_member_add_string(wb, "severity", "warning")
                        }
                        buffer_json_object_close(wb)
                    }
                    buffer_json_object_close(wb)
                }
            }
            buffer_json_array_close(wb)
        }
    }
    buffer_json_array_close(wb) // data

    buffer_json_finalize(wb)

    // Send response
    c.api.FUNCRESULT(netdataapi.FunctionResult{
        UID:             fn.UID,
        ContentType:     "application/json",
        Payload:         buf.String(),
        Code:            "200",
        ExpireTimestamp: strconv.FormatInt(time.Now().Add(1*time.Second).Unix(), 10),
    })
}
```

## SQL Queries for Critical Tables

### Active SQL Statements (ALL - sorted by elapsed time)
```sql
SELECT
    JOB_NAME,
    JOB_USER,
    AUTHORIZATION_NAME,
    JOB_STATUS,
    SQL_STATEMENT_TEXT,
    ELAPSED_TIME,           -- Total elapsed seconds
    ELAPSED_CPU_TIME,        -- CPU milliseconds
    ELAPSED_TIME_SECONDS,    -- Human readable
    CPU_PERCENTAGE,
    TEMPORARY_STORAGE,
    TEMPORARY_STORAGE_MB,    -- Converted to MB
    ELAPSED_DISK_IO_COUNT,
    ELAPSED_PAGE_FAULT_COUNT,
    -- Row severity based on elapsed time
    CASE
        WHEN ELAPSED_TIME > 300 THEN 'error'     -- > 5 minutes
        WHEN ELAPSED_TIME > 60 THEN 'warning'    -- > 1 minute
        WHEN ELAPSED_TIME > 10 THEN 'notice'     -- > 10 seconds
        ELSE 'normal'
    END AS ROW_SEVERITY
FROM TABLE(QSYS2.ACTIVE_JOB_INFO(
    SUBSYSTEM_LIST_FILTER => '*ALL',
    JOB_NAME_FILTER => 'QZDASOINIT*',    -- SQL jobs
    CURRENT_USER_LIST_FILTER => '*ALL',
    DETAILED_INFO => 'ALL'
)) X
WHERE JOB_TYPE IN ('BCH', 'INT', 'SBS', 'SYS')
  AND (SQL_STATEMENT_TEXT IS NOT NULL
       OR FUNCTION = 'QSQSRVR'           -- SQL server jobs
       OR FUNCTION LIKE 'QZD%')          -- Database server jobs
ORDER BY ELAPSED_TIME DESC               -- Longest running first
```

**Column formatting in response:**
- `elapsed_time`: type="duration", units="seconds", sort="descending" (default), visualization="heatmap"
- `elapsed_time_formatted`: type="string" ("5h 23m 15s" format for readability)
- `temporary_storage_mb`: type="integer", units="MB"
- `cpu_percentage`: type="number", units="percentage", visualization="bar", max=100
- `sql_statement_text`: type="string", full_width=true, wrap=true

**Benefits of showing ALL queries:**
1. **No blind spots** - See everything, not just "long runners"
2. **Pattern detection** - Multiple similar queries might indicate a problem
3. **Early warning** - Spot queries starting to slow down before they become critical
4. **User control** - Let users decide what's "too long" for their workload
5. **Sort flexibility** - Sort by CPU%, temp storage, I/O count, not just time

### All Active Jobs (with elapsed time for all)
```sql
SELECT
    JOB_NAME,
    SUBSYSTEM,
    JOB_USER,
    JOB_TYPE,
    JOB_STATUS,
    FUNCTION_TYPE,
    FUNCTION,
    JOB_ACTIVE_TIME,        -- When job started
    ELAPSED_TIME,           -- Seconds since start
    ELAPSED_CPU_TIME,       -- CPU milliseconds used
    CPU_PERCENTAGE,
    TEMPORARY_STORAGE,
    THREAD_COUNT,
    -- Visual indicator
    CASE
        WHEN JOB_STATUS IN ('MSGW', 'LCKW') THEN 'error'
        WHEN ELAPSED_TIME > 3600 THEN 'warning'   -- > 1 hour
        WHEN CPU_PERCENTAGE > 80 THEN 'notice'
        ELSE 'normal'
    END AS ROW_SEVERITY
FROM TABLE(QSYS2.ACTIVE_JOB_INFO()) X
ORDER BY ELAPSED_TIME DESC    -- Longest running first
```

### Jobs in Error/Wait Status
```sql
SELECT
    JOB_NAME,
    SUBSYSTEM,
    JOB_USER,
    JOB_STATUS,
    FUNCTION_TYPE,
    FUNCTION,
    JOB_ACTIVE_TIME,
    CPU_PERCENTAGE,
    TEMPORARY_STORAGE
FROM TABLE(QSYS2.ACTIVE_JOB_INFO()) X
WHERE JOB_STATUS IN ('MSGW', 'LCKW', 'DEQW', 'DLYW', 'SEMW')
ORDER BY JOB_ACTIVE_TIME DESC
```

### Error Messages
```sql
SELECT
    MESSAGE_QUEUE_LIBRARY,
    MESSAGE_QUEUE_NAME,
    MESSAGE_ID,
    MESSAGE_TYPE,
    MESSAGE_SUBTYPE,
    SEVERITY,
    MESSAGE_TIMESTAMP,
    MESSAGE_TEXT,
    MESSAGE_SECOND_LEVEL_TEXT,
    FROM_USER,
    FROM_JOB,
    FROM_PROGRAM
FROM TABLE(QSYS2.MESSAGE_QUEUE_INFO(
    MESSAGE_FILTER => 'INQUIRY',
    SEVERITY_FILTER => 40
)) X
WHERE SEVERITY >= 40
ORDER BY MESSAGE_TIMESTAMP DESC
```

### Lock Waits
```sql
-- Record locks
SELECT
    JOB_NAME,
    LOCK_STATE,
    LOCK_STATUS,
    LOCK_TYPE,
    OBJECT_NAME,
    OBJECT_LIBRARY,
    OBJECT_TYPE,
    MEMBER_NAME
FROM QSYS2.RECORD_LOCK_INFO
WHERE LOCK_STATE = 'WAITING'

-- Object locks
SELECT
    JOB_NAME,
    LOCK_STATE,
    LOCK_TYPE,
    OBJECT_NAME,
    OBJECT_LIBRARY,
    OBJECT_TYPE
FROM QSYS2.OBJECT_LOCK_INFO
WHERE LOCK_STATE = 'WAITING'
```

## Key Benefits of Single Function Approach

1. **Unified Interface**: Single entry point for all AS/400 data
2. **UI Integration**: Dashboard shows a dropdown to select data source
3. **Consistent Behavior**: Same filtering, searching, and pagination across all views
4. **Easier Maintenance**: One function handler to maintain
5. **Dynamic Options**: Can easily add/remove data sources without changing function signature

## UI Behavior

The Netdata dashboard will:
1. Show "AS/400" function in the list
2. When clicked, present a dropdown for "Data Source" (required)
3. User must select a data source before viewing data
4. Additional filters appear based on the selected data source
5. Table columns adapt to the selected data type

## Example JSON Payloads

### Request (POST v3)
```json
{
  "__data_source": "active_jobs",
  "query": "*MSGW*",
  "after": 1234567890,
  "before": 1234567899,
  "subsystem": ["QUSRWRK", "QBATCH"],
  "status": ["ACTIVE", "MSGW"]
}
```

### Response for Active Jobs
```json
{
  "status": 200,
  "type": "table",
  "has_history": false,
  "columns": {
    "job_name": {
      "index": 0,
      "name": "Job Name",
      "type": "string",
      "unique_key": true
    },
    "subsystem": {
      "index": 1,
      "name": "Subsystem",
      "type": "string",
      "filter": "multiselect"
    },
    "status": {
      "index": 2,
      "name": "Status",
      "type": "string",
      "visualization": "pill"
    },
    "cpu_percent": {
      "index": 3,
      "name": "CPU %",
      "type": "number",
      "units": "percentage"
    }
  },
  "data": [
    ["JOB001", "QUSRWRK", "ACTIVE", 12.5],
    ["BACKUP", "QBATCH", "MSGW", 0.0, {"rowOptions": {"severity": "warning"}}]
  ]
}
```

## Implementation Steps

1. **Enable Function Support in ibm.d plugin**
   - Modify `src/go/cmd/ibmdplugin/main.go` to initialize function manager
   - Wire function manager to job manager

2. **Register Single Function**
   - Add `fnReg.Register("as400", c.functionAS400View)` in AS/400 module

3. **Implement Info Handler**
   - Return `required_params` with `__data_source` selector
   - List all available data sources as options

4. **Implement Data Router**
   - Parse `__data_source` parameter
   - Route to appropriate sub-handler

5. **Implement Data Handlers**
   - One handler per data source
   - Each returns appropriate table format
   - Apply query filters and pagination

6. **Test with Dashboard**
   - Verify function appears in UI
   - Test data source selection
   - Validate filtering and search