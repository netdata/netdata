# Writing Go Collectors for Netdata

This guide documents the patterns, conventions, and best practices for writing Go collectors (modules) for Netdata's go.d.plugin framework.

## File Organization

Each collector MUST have a directory structure like this:

```text
collector/mymodule/
├── README.md             # Developer documentation about the plugin
├── collector.go          # Main collector implementation
├── collect.go            # Collection logic (optional)
├── charts.go             # Chart definitions
├── config_schema.json    # Configuration schema
├── metadata.yaml         # Netdata marketplace metadata (includes user documentation about the plugin)
├── init.go               # Init helpers (optional)
└── testdata/             # Test fixtures
```

- **collector.go**: Module struct, Init(), Check(), configuration
- **collect.go**: Collect() implementation (can be in collector.go for simple modules)
- **collect_*.go**: Split collection logic for complex collectors (e.g., collect_databases.go)
- **charts.go**: Chart templates and creation functions
- **config_schema.json**: JSON schema for web UI configuration
- **metadata.yaml**: Marketplace metadata, metric descriptions
- **init.go**: Validation and initialization helpers (optional)

## Stock Config Files

Each collector MUST have a stock configuration file in go.d/config or ibm.d/config following a simple pattern:

```yaml
## All available configuration options, their descriptions and default values:
## https://github.com/netdata/netdata/tree/master/src/go/plugin/go.d/collector/mymodule#configuration

#jobs:
#  - name: local
#    url: http://localhost:8080
#
#  - name: remote
#    url: http://203.0.113.0
#    username: username
#    password: password
```

For more detailed documentation, create self-documenting configs in custom plugins:

```yaml
## netdata configuration for MyModule
## This collector monitors...

## Prerequisites:
## 1. First requirement
## 2. Second requirement

jobs:
  ## Example: Basic configuration
  # - name: local
  #   url: http://localhost:8080
  #   update_every: 5
  
  ## Example: With authentication
  # - name: prod
  #   url: https://prod.example.com
  #   username: monitor
  #   password: secret
```

## Dynamic charts with activity check

Collectors must NEVER filter the monitored items based on activity or volume. Netdata expects a stable number of metrics. Of course charts and dimensions can come and go, but it is important this NOT to be too frequent for a large number of metrics.

The best practice is to have 2 configuration parameters:

- `maxXXX`, as a number
- `selectXXX` as a simpler pattern (glob)

The combination of the two works like this: "If the number of items is less than {maxXXX} chart them all, otherwise chart only the ones matched by {selectXXX}".

This ensures that small deployments are monitored in full, while bigger deployments require configuration in order to enable monitoring this kind of items.

The default `maxXXX` MUST be 100-500.
The default `selectXXX` should be unset or empty to match nothing.

## Chart Obsolescence Mechanism

Collectors MUST obsolete charts when they are no longer collected.

IMPORTANT: Obsoletion flushes Netdata memory. Unecessary obsolation and recreation leads to increased storage footprint for the metrics, because TSTB flushes them prematurely before having enough samples for Gorilla compression + ZSTD to efficiently compress them.

Creating charts and obsoleting them in a flip-flot fashion is a bad practice.

If there is the risk for charts to flip-flop active-obsolete, the best practice is to obsolete them after 1 minute of absence.

The go.d framework provides a built-in mechanism for marking charts as obsolete when they are no longer needed. This is available for ALL charts - both static (base) charts and dynamic instance charts.

1. **Marking a Chart as Obsolete**:
   ```go
   // Method available on any chart
   func (c *Chart) MarkRemove() {
       c.Obsolete = true    // Adds "obsolete" to CHART command options
       c.remove = true      // Flags for removal from charts slice
   }
   ```

2. **Framework Behavior**:
   - When a chart is marked obsolete, the framework appends "obsolete" to the options field in the CHART command
   - Example output: `CHART 'module.metric' '' 'Title' 'units' 'family' 'context' 'line' '100' '1' 'obsolete' 'plugin' 'module'`
   - Netdata sees the "obsolete" flag and knows to clean up the chart

3. **No Dimension Values Sent**:
   - The framework still sends BEGIN/END for obsolete charts
   - But it sends SETEMPTY for all dimensions (no values)
   - This creates empty updates that signal the chart should be removed

## Configuration Precedence and Auto-Detection

For collectors that support multiple versions or editions of the monitored application, follow this critical principle:

Admin configuration MUST always take precedence over auto-detection.

1. **Version detection can be wrong**: Vendors backport features, custom builds exist
2. **Admins know their environment**: They might have special configurations
3. **Testing new versions**: Admins need to test features on versions we haven't validated
4. **Enterprise flexibility**: Production environments often have unique requirements

## Floating Point Precision Handling

### Understanding Netdata's Precision System

Netdata's Go collectors MUST send **integer values** to Netdata, but many metrics are naturally floating-point (percentages, response times, load averages, etc.). Netdata uses a precision system to handle this conversion while preserving decimal places in the database.

**How It Works:**

1. **Collection**: Get floating-point value from source
2. **Multiply by precision**: Convert to integer for transmission
3. **Send to Netdata**: Integer value via protocol
4. **Chart definition**: Specify division to restore original value
5. **Database storage**: Netdata stores the restored float value

### Precision Best Practices

1. **Use consistent precision**: Most Go collectors use `precision = 1000`
2. **Apply precision once**: During collection phase only
3. **Always divide by precision**: In chart definitions for float metrics
4. **Separate concerns**: Unit conversion (`Mul`) and precision (`Div`) are independent
5. **Test the math**: Verify the final value matches the original float

## Missing Data is Data

**CRITICAL**: Never fill gaps. For Netdata, the absence of data collection samples is crucial. Netdata visualizes gaps in data collection, marking missing points as empty. This happens automatically when collectors DO NOT SEND samples at the predefined collection interval. The Golden Rule: When data collection fails, DO NOT SEND ANY VALUE. Skip the metric entirely.

The ONLY exception is for metrics derived from events (logs, message queues, etc.) where sparse data is expected.

## Persistent Connections

When possible Connect **ONCE** and maintain the connection for the collector's lifetime.

When reconnection is necessary:
1. **Always log the error** - Users need to know about connection issues
2. **Implement backoff** - Don't hammer the target with reconnection attempts
3. **Maintain connection state** - Track connection health

## Minimize Application Impact

**CRITICAL**: Reuse temporary objects between collection cycles. Don't create new temporary resources for each collection.

## Ideal collector lifecycle

1. **One connection per collector** - Not per collection cycle
2. **One set of temporary resources** - Created at init, cleaned at shutdown
3. **Minimal queries** - Batch requests when possible
4. **Respect rate limits** - Don't overload the monitored application
5. **Clean shutdown** - Always clean up resources in Cleanup()

## Documentation

### README.md Structure (Developer/User documentation)

```markdown
# Module name collector

## Overview
What this collector does and what it monitors.

## Collected metrics
List of all metrics with descriptions.

## Configuration
Configuration examples and options.

## Requirements
Any prerequisites or dependencies.

## Troubleshooting
Common issues and solutions.
```

### metadata.yaml Key Sections (source of truth for User Documentation)

metadata.yaml drives the integrations list presented in Netdata dashboard and sites

```yaml
plugin_name: go.d.plugin
modules:
  - meta:
      module_name: mymodule
      monitored_instance:
        name: My Service
        link: https://example.com
        categories:
          - data-collection.category
        icon_filename: "icon.svg"
    overview:
      data_collection:
        metrics_description: |
          Detailed description of what metrics are collected.
        method_description: |
          How the collector gathers these metrics.
    setup:
      prerequisites:
        list:
          - title: Requirement
            description: What needs to be done
    metrics:
      folding:
        title: Metrics
        enabled: false
      description: ""
      availability: []
      scopes:
        - name: global
          description: These metrics refer to the entire instance.
          labels: []
          metrics:
            - name: module.metric_name
              description: Metric description
              unit: units
              chart_type: line
              dimensions:
                - name: dimension1
                - name: dimension2
```

### Vnode Support

Collectors MUST support vnode for configuration management:

```go
type Config struct {
    Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
    UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
    // ... other fields
}
```

## Alert Best Practices

Alerts should be stores in `src/health/health.d/{module}.conf`

Netdata uses alert templates that are automatically applied to a single instance (chart in the code).

### Threshold Selection

Netdata follows specific patterns for alert thresholds:

1. **Avoid fixed thresholds** except for:
   - Error count metrics (e.g., deadlocks > 0)
   - Status/state metrics (e.g., backup failed = 1)
   - Long-running operations (e.g., queries > X seconds)
   - **Percentage-based metrics** (e.g., CPU usage %, memory usage %, disk usage %)

2. **Preferred approaches**:
   - **Fixed thresholds on percentages**: Simple thresholds (e.g., > 80%, > 90%) work well for percentage metrics
   - **Rolling windows with predictions**: For non-percentage metrics, compare current values to predicted normal ranges
   - **Dynamic baselines**: Use historical data to establish normal behavior when percentages aren't available
   - **Percentile-based thresholds**: Adapt to workload patterns
   - **Rate of change detection**: Alert on sudden spikes or drops

**Note**: The dynamic threshold rule primarily applies when we cannot express the metric as a percentage. For percentage-based metrics (0-100%), fixed thresholds are appropriate and preferred for their simplicity and clarity.

### Alert Severity and Routing

1. **Silent alerts**: Use `to: silent` for alerts that don't require immediate human action
   - Performance degradation that's not critical
   - Utilization approaching limits but not critical
   - Informational alerts for capacity planning

2. **Non-silent alerts**: Only for issues requiring immediate action
   - Service failures
   - Critical resource exhaustion
   - Data corruption risks
   - Security breaches

### Alert Configuration Structure

```yaml
template: collector_metric_condition
      on: collector.metric
   class: Utilization|Errors|Latency|Workload|Availability
    type: System|Database|Web Server|Application
component: ServiceName
  lookup: average -5m unaligned of dimension  # for time-based, the result in $this
    calc: $dimension * 100 / $total  # for calculated metrics, can use $this, the result in $this
   units: %|ms|requests|errors
   every: 10s
    warn: # warning condition, can use $this
    crit: # critical condition, can use $this
   delay: down 5m multiplier 1.5 max 1h
 summary: Short description
    info: Detailed description with ${value} placeholder
      to: role|silent
```

## Charts design

The collectors MUST always (when possible) add labels for the versions of the application. This enables users to filter by version, group metrics by version, and understand which features are available.

1. **Dimensions of chart must be additive or comparable** - they should make sense when summed (exception: percentages, boolean statuses)
2. **All dimensions must share the same units** - never mix bytes with milliseconds, or gauges with rates
3. **Context names should describe what's measured** - `thread_pools.threads` clearly indicates thread counts
4. **Group metrics that tell the same story** - min/current/max belong together when they have same units
5. **Use chart families** to group related metrics
6. **Calculate derived values** (like "other" = total - specific) to maintain additivity
7. **Separate different measurement types** into different charts
8. **Historical/peak values** should be in separate charts from current values
9. **Gauges and rates must be in separate contexts** - point-in-time values vs incremental counters

## Collect Everything Available

**Netdata's distributed architecture enables ingestion of SIGNIFICANTLY more metrics than centralized systems. Collect EVERYTHING unless there's a compelling reason not to.**

**DEFAULT: COLLECT EVERYTHING**

Only exclude metrics when:
1. **100% redundant** - Exact duplicate of another metric already collected
2. **Completely useless** - Provides zero operational value (very rare)
3. **Performance impact** - Collection significantly affects the monitored application
4. **Resource intensive** - Metric calculation is expensive on the target system
5. **Configuration constants** - Metrics that never or rarely change (although some are useful)

##  MOCK DATA

Review the code for any mock data, remaining TODOs, frictional logic, frictional API endpoints, frictional response formats, frictional members, etc. An independent reality check **MUST** be performed.

## LOGS

Review all logs generated by the code to ensure users a) have enough information to understand the selections the code made, b) have descriptive and detailed logs on failures, c) they are not spammed by repeated logs on failures that are supposed to be permanent (like a not supported feature by the monitored application)

## CONFIGURATION

The code, the stock configuration, the metadata.yaml, the config_schema.json and any stock alerts **MUST** match 100%. Even the slightest variation between them in config keys, possible values, enum values, contexts, dimension names, etc LEADS TO A NON WORKING SOLUTION. So, at the end of every change, an independent sync check **MUST** be performed. While doing this work, ensure also README.md reflects the facts.

## USE DOCUMENTATION AND INFORMATION

metadata.yaml is the PRIMARY documentation source shown on the Netdata integrations page - ensure troubleshooting information, setup instructions, and all documentation is up-to-date in BOTH README.md AND metadata.yaml.

## CRITICAL: Data Integrity Rules

- **NEVER FAKE DATA COLLECTION VALUES!** YOU ARE NEVER ALLOWED TO SET DATA COLLECTION VALUES TO ZERO, OR ANY VALUE! THIS IS A DATA COLLECTION SYSTEM AND IT SHOULD REFLECT THE ACTUAL DATA COLLECTED VALUES! MISSING DATA = DATA! WHEN A VALUE IS MISSING, NETDATA CREATES GAPS ON THE CHARTS. THIS IS IMPORTANT INFORMATION FOR THE MONITORED APPLICATIONS AND SHOULD NEVER BE FILLED WITH ZEROS OR MOCK DATA!
- When we create charts for arrays of items, we never use activity based filtering to control cardinality (e.g. create charts for the tables that read > X rows, create charts for the connections that transfer > X bytes, etc). But we can use activity based filtering when we count items (e.g. a metric with the number of queries running for more than X minutes).

## CRITICAL: NIDL-Framework

The file /docs/NIDL-Framework.md describes how Netdata metrcis, charts and dashboards work. You **MUST** read it before working with metrics in collectors.
IMPORTANT: "chart" for go.d modules is "instance" for NIDL and "context" for go.d modules in "chart" for NIDL.
