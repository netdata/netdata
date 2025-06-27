# Writing Go Collectors for Netdata

This guide documents the patterns, conventions, and best practices for writing Go collectors (modules) for Netdata's go.d.plugin framework.

## Table of Contents

1. [Collector Structure](#collector-structure)
2. [File Organization](#file-organization)
3. [Configuration](#configuration)
4. [Metrics and Charts](#metrics-and-charts)
5. [Dynamic Instance Management](#dynamic-instance-management)
6. [Cardinality Control](#cardinality-control)
7. [Error Handling](#error-handling)
8. [Testing](#testing)
9. [Documentation](#documentation)
10. [CGO and Plugin Separation](#cgo-and-plugin-separation)
11. [## Checklist](#check-list)

## Collector Structure

### Basic Module Interface

Every collector must implement the `module.Module` interface:

```go
type Module interface {
    Init(context.Context) error
    Check(context.Context) error
    Charts() *Charts
    Collect(context.Context) map[string]int64
    Cleanup(context.Context)
    Configuration() any
}
```

### Module Registration

Register your module in the `init()` function:

```go
//go:embed "config_schema.json"
var configSchema string

func init() {
    module.Register("mymodule", module.Creator{
        JobConfigSchema: configSchema,
        Create:          func() module.Module { return New() },
        Config:          func() any { return &Config{} },
    })
}
```

### Base Structure

```go
type MyModule struct {
    module.Base                    // Embed Base for logging
    Config `yaml:",inline" json:""`
    
    charts *module.Charts
    
    // Client connections
    client someClient
    
    // State tracking for dynamic instances
    collected map[string]bool      // Track what we've collected
    seen      map[string]bool      // Track what we've seen this cycle
    
    // For conditional chart additions
    addFeatureOnce *sync.Once     // Use for features that might not be available
}
```

## File Organization

Each collector should have these files:

```text
collector/mymodule/
├── README.md              # Symlink to integrations doc
├── collector.go           # Main collector implementation
├── collect.go            # Collection logic (optional)
├── charts.go             # Chart definitions
├── config_schema.json    # Configuration schema
├── metadata.yaml         # Netdata marketplace metadata
├── init.go               # Init helpers (optional)
└── testdata/             # Test fixtures
```

### File Responsibilities

- **collector.go**: Module struct, Init(), Check(), configuration
- **collect.go**: Collect() implementation (can be in collector.go for simple modules)
- **collect_*.go**: Split collection logic for complex collectors (e.g., collect_databases.go)
- **charts.go**: Chart templates and creation functions
- **config_schema.json**: JSON schema for web UI configuration
- **metadata.yaml**: Marketplace metadata, metric descriptions
- **init.go**: Validation and initialization helpers (optional)

## Configuration

### Configuration Structure

```go
type Config struct {
    UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
    Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
    
    // For HTTP-based collectors
    web.HTTP `yaml:",inline" json:""`
    
    // For other connection types
    DSN      string           `yaml:"dsn,omitempty" json:"dsn"`
    Timeout  confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
    
    // Collection flags
    CollectSomeMetric bool `yaml:"collect_some_metric" json:"collect_some_metric"`
    
    // Cardinality limits (not universally used)
    MaxInstances int    `yaml:"max_instances,omitempty" json:"max_instances"`
    
    // Filtering (implementation varies)
    InstanceSelector matcher.SimpleExpr `yaml:"instance_selector,omitempty" json:"instance_selector"`
}
```

### Stock Config Files

Stock configuration files in go.d/config follow a simple pattern:

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

## Metrics and Charts

### Chart Definition Patterns

Base charts (always present):

```go
var baseCharts = module.Charts{
    {
        ID:       "metric_name",
        Title:    "Metric Title",
        Units:    "units",
        Fam:      "family",
        Ctx:      "module.metric_name",
        Priority: prioMetricName,
        Dims: module.Dims{
            {ID: "dimension_1", Name: "dim1"},
            {ID: "dimension_2", Name: "dim2"},
        },
    },
}
```

Template charts (for dynamic instances):

```go
var instanceChartsTmpl = module.Charts{
    {
        ID:       "instance_%s_metric",
        Title:    "Instance Metric",
        Units:    "units",
        Fam:      "instances",
        Ctx:      "module.instance_metric",
        Priority: prioInstanceMetric,
        Dims: module.Dims{
            {ID: "instance_%s_value", Name: "value"},
        },
    },
}
```

### Creating Instance Charts

```go
func newInstanceCharts(instance string) *module.Charts {
    charts := instanceChartsTmpl.Copy()
    
    for _, chart := range *charts {
        chart.ID = fmt.Sprintf(chart.ID, cleanName(instance))
        chart.Labels = []module.Label{
            {Key: "instance", Value: instance},
        }
        for _, dim := range chart.Dims {
            dim.ID = fmt.Sprintf(dim.ID, cleanName(instance))
        }
    }
    
    return charts
}
```

### Clean Names

Always clean instance names for use in metric IDs:

```go
func cleanName(name string) string {
    r := strings.NewReplacer(
        " ", "_",
        ".", "_",
        "-", "_",
        "/", "_",
        ":", "_",
        "=", "_",
    )
    return strings.ToLower(r.Replace(name))
}
```

## Dynamic Instance Management

### Lifecycle Pattern

```go
func (m *MyModule) collectInstances(mx map[string]int64) {
    // Reset seen map each collection
    m.seen = make(map[string]bool)
    
    for _, instance := range getInstances() {
        m.seen[instance.Name] = true
        
        // Create charts if new instance
        if !m.collected[instance.Name] {
            m.collected[instance.Name] = true
            if err := m.charts.Add(*newInstanceCharts(instance.Name)...); err != nil {
                m.Warning(err)
            }
        }
        
        // Collect metrics
        mx[fmt.Sprintf("instance_%s_metric", cleanName(instance.Name))] = instance.Value
    }
    
    // Remove stale instances
    for name := range m.collected {
        if !m.seen[name] {
            delete(m.collected, name)
            m.removeInstanceCharts(name)
        }
    }
}
```

### Alternative Pattern: Updated Flag

Some collectors use an updated flag pattern:

```go
type dbMetrics struct {
    name    string
    updated bool
    // metrics...
}

func (m *MyModule) collect() {
    // Mark all as not updated
    for _, db := range m.dbs {
        db.updated = false
    }
    
    // Collect and mark updated
    for _, db := range getDatabases() {
        metrics := m.dbs[db.Name]
        if metrics == nil {
            metrics = &dbMetrics{name: db.Name}
            m.dbs[db.Name] = metrics
            m.addDBCharts(db.Name)
        }
        metrics.updated = true
        // collect metrics...
    }
    
    // Remove stale
    for name, db := range m.dbs {
        if !db.updated {
            delete(m.dbs, name)
            m.removeDBCharts(name)
        }
    }
}
```

## Cardinality Control

**Note**: Not all collectors implement explicit cardinality limits. It depends on the use case.

### Simple Limit Pattern

```go
func (m *MyModule) collectWithLimits(mx map[string]int64) {
    collected := 0
    
    for _, item := range getAllItems() {
        // Check limit
        if m.Config.MaxInstances > 0 && collected >= m.Config.MaxInstances {
            if collected == m.Config.MaxInstances {
                m.Warningf("reached max_instances limit (%d), some instances will not be collected", 
                    m.Config.MaxInstances)
            }
            break
        }
        
        // Collect...
        collected++
    }
}
```

### Selector Pattern (More Common)

Many collectors use matcher expressions for filtering:

```go
// Config
type Config struct {
    ContainerSelector matcher.SimpleExpr `yaml:"container_selector"`
}

// Usage
func (m *MyModule) collectContainers() {
    for _, container := range getContainers() {
        // Skip if selector doesn't match
        if !m.containerSelector.MatchString(container.Name) {
            continue
        }
        
        // Skip ignored containers (label-based)
        if container.Labels["netdata.cloud/ignore"] == "true" {
            continue
        }
        
        // Collect...
    }
}
```

### Best Practices

1. **Use selectors over hard limits** when possible
2. **Support label-based filtering** for dynamic environments
3. **Set reasonable defaults**: 10-200 depending on metric type
4. **Log when limits are hit**: So users know data is incomplete

## Configuration Precedence and Auto-Detection

### Admin Configuration Always Wins

For collectors that support multiple versions or editions of the monitored application, follow this critical principle:

**Admin configuration MUST always take precedence over auto-detection.**

#### Implementation Pattern

```go
type Config struct {
    // Use pointers for tri-state: nil (not set), true, false
    CollectAdvancedFeatures *bool `yaml:"collect_advanced_features" json:"collect_advanced_features"`
}

func (c *Collector) Init() {
    // 1. Detect version/edition
    c.detectVersion()
    
    // 2. Set defaults ONLY if admin hasn't configured
    if c.Config.CollectAdvancedFeatures == nil {
        // Auto-detection sets intelligent defaults
        defaultValue := c.supportsAdvancedFeatures()
        c.Config.CollectAdvancedFeatures = &defaultValue
    }
    // If admin set it (true or false), we respect their choice
}

func (c *Collector) collect() {
    // 3. Always attempt what admin configured
    if *c.Config.CollectAdvancedFeatures {
        if err := c.collectAdvancedFeatures(); err != nil {
            // Log but don't fail entire collection
            c.Warningf("Advanced features collection failed: %v", err)
        }
    }
}
```

#### Why This Matters

1. **Version detection can be wrong**: Vendors backport features, custom builds exist
2. **Admins know their environment**: They might have special configurations
3. **Testing new versions**: Admins need to test features on versions we haven't validated
4. **Enterprise flexibility**: Production environments often have unique requirements

#### Anti-Pattern to Avoid

```go
// DON'T DO THIS - prevents admin override
if c.CollectFeature && c.versionSupportsFeature() {
    c.collectFeature()  // Admin can't force collection on "unsupported" versions
}

// DO THIS INSTEAD - respects admin choice
if c.CollectFeature {
    c.collectFeature()  // Try regardless of version, handle errors gracefully
}
```

## Error Handling

### Init Errors

```go
func (m *MyModule) Init(ctx context.Context) error {
    // Validate required config
    if err := m.validateConfig(); err != nil {
        return err
    }
    
    // Create clients
    if err := m.initClient(); err != nil {
        return err
    }
    
    // Initialize selectors
    if m.Config.ContainerSelector != "" {
        m.containerSelector, err = matcher.NewSimpleExprMatcher(m.Config.ContainerSelector)
        if err != nil {
            return fmt.Errorf("invalid container selector: %w", err)
        }
    }
    
    return nil
}

// Separate validation for clarity
func (m *MyModule) validateConfig() error {
    if m.URL == "" {
        return errors.New("url is required")
    }
    return nil
}
```

### Collection Errors

```go
func (m *MyModule) Collect(ctx context.Context) map[string]int64 {
    mx, err := m.collect(ctx)
    if err != nil {
        m.Error(err)
        return nil
    }
    
    if len(mx) == 0 {
        return nil
    }
    
    return mx
}
```

### Client Reuse Pattern

Maintain persistent connections and recreate on failure:

```go
func (m *MyModule) collect(ctx context.Context) (map[string]int64, error) {
    // Check if client needs to be created/recreated
    if m.db == nil {
        if err := m.openDBConnection(); err != nil {
            return nil, err
        }
    }
    
    mx := make(map[string]int64)
    
    // Try to collect
    if err := m.ping(ctx); err != nil {
        // Try to reconnect once
        m.Cleanup(ctx)
        if err := m.openDBConnection(); err != nil {
            return nil, err
        }
        // Retry ping
        if err := m.ping(ctx); err != nil {
            return nil, err
        }
    }
    
    // Continue with collection...
    return mx, nil
}
```

### Partial Failures

Continue collecting what you can:

```go
func (m *MyModule) collect() (map[string]int64, error) {
    mx := make(map[string]int64)
    
    // Collect primary metrics
    if err := m.collectPrimary(mx); err != nil {
        return nil, err  // Fatal - can't continue
    }
    
    // Collect optional metrics
    if m.Config.CollectOptional {
        if err := m.collectOptional(mx); err != nil {
            m.Warningf("failed to collect optional metrics: %v", err)
            // Continue - these are optional
        }
    }
    
    return mx, nil
}
```

### Conditional Feature Support

Use sync.Once for features that might not be available:

```go
type MyModule struct {
    // ... other fields
    addExtendedChartsOnce *sync.Once
}

func (m *MyModule) collectExtended(mx map[string]int64) error {
    data, err := m.getExtendedData()
    if err != nil {
        return err
    }
    
    // Add charts only if feature is available
    if m.addExtendedChartsOnce != nil {
        m.addExtendedChartsOnce.Do(func() {
            m.addExtendedChartsOnce = nil
            m.addExtendedCharts()
        })
    }
    
    // Collect metrics...
    return nil
}
```

## Testing

### Test Structure

```go
func TestMyModule_Collect(t *testing.T) {
    tests := map[string]struct {
        prepare  func() *MyModule
        expected map[string]int64
        wantErr  bool
    }{
        "success case": {
            prepare: func() *MyModule {
                m := New()
                m.Config.URL = "http://localhost"
                return m
            },
            expected: map[string]int64{
                "metric": 100,
            },
        },
    }
    
    for name, tt := range tests {
        t.Run(name, func(t *testing.T) {
            m := tt.prepare()
            require.NoError(t, m.Init())
            
            mx := m.Collect()
            assert.Equal(t, tt.expected, mx)
        })
    }
}
```

## Documentation

### README.md Structure

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

### metadata.yaml Key Sections

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

## CGO and Plugin Separation

### When to Create a Separate Plugin

Create a separate plugin (like ibm.d.plugin) when:

1. **CGO is required**: The collector needs C libraries
2. **Special dependencies**: Requires external libraries users might not have
3. **Licensing concerns**: Dependencies have incompatible licenses
4. **Optional feature**: Not all users need it

### Structure for CGO Plugins

```
src/go/
├── cmd/
│   └── myplugin/
│       ├── main.go      # Plugin entry point
│       └── config.go    # Plugin configuration
└── plugin/
    └── my.d/
        ├── collector/   # Collectors requiring CGO
        └── config/      # Configuration files
```

### Registration Pattern

```go
// cmd/myplugin/main.go
package main

import (
    _ "github.com/netdata/netdata/go/plugins/plugin/my.d/collector/module1"
    _ "github.com/netdata/netdata/go/plugins/plugin/my.d/collector/module2"
)

const pluginName = "my.d.plugin"
```

## Additional Patterns

### Metric Value Precision

When dealing with floating-point values, use consistent precision:

```go
const precision = 1000  // Common pattern in collectors

// Convert float to int64 with precision
mx["metric_name"] = int64(floatValue * precision)

// For very small values (e.g., percentiles)
mx["percentile_99"] = int64(value * precision * precision)
```

### Complex Collectors Organization

For collectors with many metrics, split collection logic:

```text
collector/mymodule/
├── collector.go         # Main structure
├── collect.go          # Top-level collection
├── collect_databases.go # Database metrics
├── collect_tables.go   # Table metrics
├── collect_queries.go  # Query metrics
└── charts.go          # All chart definitions
```

### Vnode Support

Many collectors support vnode for configuration management:

```go
type Config struct {
    Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
    UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
    // ... other fields
}
```

## Alert Best Practices

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

3. **Examples of good thresholds**:
   ```
   # Fixed thresholds on percentages (preferred for percentage metrics)
   warn: $this > 80   # for CPU usage %
   crit: $this > 90   # for memory usage %
   
   # Using predictions (preferred for non-percentage metrics)
   warn: $this > (($1h_min_value * 2) + ($1h_average * 2))
   
   # Using rolling averages (for non-percentage metrics)
   warn: $this > ($10m_average * 1.5)
   
   # Rate of change
   warn: abs($this - $1m_average) > ($1h_average * 0.2)
   ```

4. **Examples of acceptable fixed thresholds**:
   ```
   # Percentages (utilization, usage, ratios)
   warn: $this > 80   # 80% threshold
   crit: $this > 90   # 90% threshold
   
   # Errors/failures (should be zero)
   warn: $this > 0
   
   # Binary status (0 = OK, 1 = failed)
   warn: $this == 1
   
   # Time-based (queries taking too long)
   warn: $this > 5000  # 5 seconds
   ```

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

3. **Alert routing example**:
   ```yaml
   # Silent alert - won't wake anyone at 3AM
   template: db2_buffer_pool_hit_ratio_low
   to: silent
   
   # Critical alert - requires immediate attention
   template: db2_deadlock_critical
   to: dba
   ```

### Alert Configuration Structure

```yaml
template: collector_metric_condition
      on: collector.metric
   class: Utilization|Errors|Latency|Workload|Availability
    type: System|Database|Web Server|Application
component: ServiceName
    calc: $dimension * 100 / $total  # for calculated metrics
  lookup: average -5m unaligned of dimension  # for time-based
   units: %|ms|requests|errors
   every: 10s
    warn: # warning condition
    crit: # critical condition
   delay: down 5m multiplier 1.5 max 1h
 summary: Short description
    info: Detailed description with ${value} placeholder
      to: role|silent
```

## Version Information Labels

For collectors that monitor multiple versions of an application, **always expose version information as chart labels**. This enables users to filter by version, group metrics by version, and understand which features are available.

### Pattern: Detect and Label All Charts

```go
// After detecting version/edition during collection
func (c *Collector) detectVersion(ctx context.Context) error {
    // Version detection logic...
    c.version = detectedVersion
    c.edition = detectedEdition
    
    // Add labels to all existing charts after detection
    c.addVersionLabelsToCharts()
    return nil
}

// Add version labels to all charts (base and instance)
func (c *Collector) addVersionLabelsToCharts() {
    versionLabels := []module.Label{
        {Key: "app_version", Value: c.version},
        {Key: "app_edition", Value: c.edition},
    }
    
    // Add to all existing charts
    for _, chart := range *c.charts {
        chart.Labels = append(chart.Labels, versionLabels...)
    }
}
```

### Include in Instance Chart Creation

```go
// Chart creation functions should include version labels
func (c *Collector) newInstanceCharts(instance *instanceMetrics) *module.Charts {
    // ... chart creation logic
    
    for _, chart := range charts {
        chart.Labels = []module.Label{
            {Key: "instance", Value: instance.name},
            {Key: "app_version", Value: c.version},    // Version info
            {Key: "app_edition", Value: c.edition},    // Edition info
        }
    }
    return &charts
}
```

### Benefits

- **Filtering**: Users can filter dashboards by version: `app_version="11.5.7"`
- **Grouping**: Aggregate metrics across multiple instances of the same version
- **Troubleshooting**: Quickly identify version-specific issues
- **Migration Planning**: Compare performance across versions during upgrades

## Netdata Chart Design Rules

### RULE #1: Non-Overlapping Dimensions

**Dimensions within a single chart MUST NOT be overlapping or of different types.**

Netdata is NOT like Prometheus/Grafana. The dashboard provides point-and-click slicing and dicing, which means dimensions get aggregated (summed or averaged) when grouping by anything except "by dimension". Mixing dimension types leads to nonsensical values.

#### Examples of WRONG Design:
```go
// BAD: Mixing current values with historical peak
{
    ID: "threads",
    Dims: module.Dims{
        {ID: "current", Name: "current"},    // Current value
        {ID: "daemon", Name: "daemon"},      // Subset of current
        {ID: "peak", Name: "peak"},          // Historical maximum
    },
}
// Problem: Aggregating gives meaningless sum of current + historical values
```

```go
// BAD: Mixing used with limits
{
    ID: "memory",
    Dims: module.Dims{
        {ID: "used", Name: "used"},          // Current usage
        {ID: "committed", Name: "committed"}, // Different concept
        {ID: "max", Name: "max"},            // Configuration limit
    },
}
// Problem: Sum of used + max makes no sense
```

#### Examples of CORRECT Design:
```go
// GOOD: Split into logical charts with additive dimensions
{
    ID: "threads_current",
    Family: "threads",
    Type: module.Stacked,
    Dims: module.Dims{
        {ID: "daemon", Name: "daemon"},
        {ID: "other", Name: "other"},    // calculated as total - daemon
    },
}

{
    ID: "threads_peak", 
    Family: "threads",
    Dims: module.Dims{
        {ID: "peak", Name: "peak"},
    },
}
```

```go
// GOOD: Memory usage as additive components
{
    ID: "memory_usage",
    Family: "memory",
    Type: module.Stacked,
    Dims: module.Dims{
        {ID: "used", Name: "used"},
        {ID: "free", Name: "free"},      // calculated as committed - used
    },
}

{
    ID: "memory_limits",
    Family: "memory", 
    Dims: module.Dims{
        {ID: "committed", Name: "committed"},
        {ID: "max", Name: "max"},
    },
}
```

#### Key Principles:
1. **Dimensions must be additive or comparable** - they should make sense when summed
2. **Use chart families** to group related metrics
3. **Calculate derived values** (like "other" = total - specific) to maintain additivity
4. **Separate different measurement types** into different charts
5. **Historical/peak values** should be in separate charts from current values

### RULE #2: Chart Types Based on Data Semantics

**Choose chart types based on what the data represents and how users need to visualize it.**

#### Chart Type Guidelines:

**1. STACKED Charts** - Use when showing volume/size where users need to see "the whole"
- Memory usage (used + free = total)
- Disk space (used + available = total)
- CPU states (user + system + idle + wait = 100%)
- Any resource where parts sum to a meaningful total

```go
// GOOD: Memory usage as stacked chart
{
    ID: "memory_usage",
    Type: module.Stacked,
    Dims: module.Dims{
        {ID: "used", Name: "used"},
        {ID: "free", Name: "free"},
    },
}
```

**2. AREA Charts** - Use for bidirectional volume (in/out, read/write, rx/tx)
- Network traffic (received positive, sent negative)
- Disk I/O (reads positive, writes negative)  
- Any flow with dual directions

```go
// GOOD: Network traffic as area chart
{
    ID: "network_traffic",
    Type: module.Area,
    Dims: module.Dims{
        {ID: "received", Name: "received"},
        {ID: "sent", Name: "sent", Mul: -1},  // Negative for opposite direction
    },
}
```

**3. LINE Charts** - Use for independent metrics
- Counters (requests/sec, errors/sec)
- Rates and percentages
- Durations (response time, latency)
- Independent values that don't sum to a whole

```go
// GOOD: Response time as line chart
{
    ID: "response_time",
    Type: module.Line,
    Dims: module.Dims{
        {ID: "avg", Name: "average"},
        {ID: "p95", Name: "95th percentile"},
    },
}
```

**4. HEATMAP Charts** - Use for distribution/histogram data
- Response time distributions
- Request size distributions
- Any bucketed/histogram data

#### Examples of WRONG Usage:
```go
// BAD: Using line for memory (should be stacked)
{
    ID: "memory",
    Type: module.Line,  // WRONG!
    Dims: module.Dims{
        {ID: "used", Name: "used"},
        {ID: "free", Name: "free"},
    },
}

// BAD: Using stacked for independent rates
{
    ID: "request_rates", 
    Type: module.Stacked,  // WRONG!
    Dims: module.Dims{
        {ID: "success", Name: "success/s"},
        {ID: "errors", Name: "errors/s"},
    },
}
```

#### Key Benefits:
1. **Stacked charts** instantly show resource utilization and pressure
2. **Area charts** beautifully visualize flow direction and volume
3. **Line charts** clearly show trends without implying relationships
4. **Proper types** enable meaningful aggregations in Netdata's UI

Note: The query engine intelligently handles negative values in area charts, converting them to positive when grouping by dimensions other than the default.

### RULE #3: Incremental Charts and Rate Units

**For incremental/rate charts, ALWAYS specify the time unit in the chart units, and provide raw counter values to Netdata.**

Netdata's dashboard allows users to convert rates between different time units (e.g., /s to /min to /hour). This only works when:
1. The time unit is explicitly specified in the chart units
2. The raw counter value is provided (not pre-calculated rates)

#### Correct Implementation:

```go
// GOOD: Units specify "/s" and dimension uses Incremental algorithm
{
    ID:       "requests",
    Title:    "HTTP Requests",
    Units:    "requests/s",     // MUST specify time unit
    Type:     module.Line,
    Dims: module.Dims{
        {ID: "http_requests_total", Name: "requests", Algo: module.Incremental},
    },
}

// GOOD: Other time-based rates
{
    Units: "errors/s",         // Per second
    Units: "connections/s",    // Per second  
    Units: "packets/s",        // Per second
    Units: "operations/min",   // Per minute
    Units: "backups/hour",     // Per hour
}
```

#### WRONG Implementation:

```go
// BAD: Missing time unit
{
    Units: "requests",         // WRONG! No time unit
    Dims: module.Dims{
        {ID: "requests", Name: "requests", Algo: module.Incremental},
    },
}

// BAD: Pre-calculating rate
func (c *Collector) collect() {
    requestsPerSec := (current - previous) / timeElapsed  // WRONG!
    mx["requests"] = requestsPerSec
}

// GOOD: Provide raw counter
func (c *Collector) collect() {
    mx["requests_total"] = totalRequests  // Raw counter value
    // Netdata calculates the rate using Algo: module.Incremental
}
```

#### Key Principles:

1. **Always use raw counters** - Send the actual counter value, not a pre-calculated rate
2. **Specify time units** - Use "/s", "/min", "/hour" in the Units field
3. **Use Incremental algorithm** - Set `Algo: module.Incremental` for rate calculation
4. **Be consistent** - Most rates should be "/s" unless there's a specific reason

#### Common Rate Units:
- `requests/s` - HTTP requests, API calls
- `operations/s` - Database operations, transactions
- `bytes/s` or `B/s` - Data transfer rates
- `packets/s` - Network packets
- `errors/s` - Error rates
- `connections/s` - Connection rates
- `messages/s` - Message queue rates
- `queries/s` - Database queries

This enables Netdata's powerful rate conversion feature, allowing users to view the same metric as requests/second, requests/minute, or requests/hour based on their preference.

### RULE #4: Collect Everything Available

**Netdata's distributed architecture enables ingestion of SIGNIFICANTLY more metrics than centralized systems. Collect EVERYTHING unless there's a compelling reason not to.**

Netdata's unique strengths:
- **Distributed collection** - No central bottleneck
- **High-resolution metrics** - Per-second granularity
- **ML-based anomaly detection** - Correlates seemingly unrelated metrics
- **Automatic dependency discovery** - Reveals hidden relationships between systems

#### Collection Philosophy:

**DEFAULT: COLLECT EVERYTHING**

Only exclude metrics when:
1. **100% redundant** - Exact duplicate of another metric already collected
2. **Completely useless** - Provides zero operational value (very rare)
3. **Performance impact** - Collection significantly affects the monitored application
4. **Resource intensive** - Metric calculation is expensive on the target system

#### Implementation Guidelines:

```go
// GOOD: Collect all available metrics
func (c *Collector) Init() {
    // By default, collect everything
    c.CollectCPUMetrics = true
    c.CollectMemoryMetrics = true
    c.CollectDiskMetrics = true
    c.CollectNetworkMetrics = true
    c.CollectApplicationMetrics = true
    
    // Only disable if resource intensive
    c.CollectExpensiveMetrics = false  // User can enable if needed
}
```

```go
// GOOD: Group metrics by family for organization
{
    // CPU metrics family
    {Family: "cpu", /* all CPU-related metrics */},
    
    // Memory metrics family  
    {Family: "memory", /* all memory metrics */},
    
    // Application-specific family
    {Family: "servlet", /* all servlet metrics */},
    {Family: "session", /* all session metrics */},
}
```

#### Examples:

**Seemingly "unimportant" metrics that prove valuable:**
- **Thread pool size** - Correlates with response time degradation
- **Session counts** - Predicts memory pressure
- **CPU available processors** - Reveals container limit changes
- **GC time per cycle** - Early warning for memory issues

**Configuration for expensive metrics:**
```yaml
# Disabled by default due to performance impact
collect_detailed_query_plans: false
collect_execution_traces: false

# Enabled by default - no significant impact
collect_cpu_metrics: true
collect_memory_metrics: true
collect_servlet_metrics: true
```

#### Key Principles:

1. **Storage is cheap, insights are valuable** - Disk space costs less than missing critical data
2. **ML needs data variety** - More metrics enable better anomaly detection
3. **Correlations emerge over time** - Relationships aren't always obvious immediately
4. **Proper families organize the noise** - Good categorization makes many metrics manageable
5. **Users can always disable** - But defaults should be comprehensive

#### Anti-patterns to AVOID:

```go
// BAD: Arbitrary filtering
if metricName.Contains("internal") {
    continue  // DON'T arbitrarily skip metrics
}

// BAD: Pre-judging importance
if value < 100 {
    continue  // DON'T decide what's important for users
}

// BAD: Hardcoded collection limits
if metricsCount > 50 {
    break  // DON'T limit metric collection
}
```

Remember: Netdata's strength lies in comprehensive, high-resolution data collection. When in doubt, collect it!

### RULE #5: Chart Family Hierarchies

**Netdata supports hierarchical families using "/" as a delimiter, creating expandable tree structures in the UI. Use this wisely.**

#### How it Works:
- Family string "cpu/core/usage" creates a 3-level tree
- **First 2 levels auto-expand** (grouping: true property)
- **Level 3 and deeper start collapsed** (no grouping property)
- No hard depth limit - but deep trees become cumbersome to navigate
- Enables logical organization of complex metrics

#### Good Practices:

```go
// GOOD: Logical grouping with multiple charts per leaf
{
    Family: "database/connections",  // Multiple connection-related charts
    // - active connections
    // - idle connections  
    // - connection rate
    // - connection errors
}

// GOOD: Two-level hierarchy for clarity
{
    Family: "servlet/performance",   // Groups performance metrics
    // - request rate
    // - response time
    // - error rate
}
```

#### BAD Practices:

```go
// BAD: Deep hierarchy with single chart
{
    Family: "system/cpu/core0/usage",  // Only one chart at this leaf!
}

// BAD: Over-categorization
{
    Family: "app/module/submodule/component/metric",  // Too deep!
}
```

#### Guidelines:

1. **Use hierarchies when you have multiple related charts** to group
2. **Limit depth to 2-3 levels** maximum
3. **Ensure multiple charts per leaf** - avoid single-chart leaves
4. **Keep it intuitive** - hierarchies should make finding metrics easier, not harder
5. **Consider flat families** for small groups (< 5 charts)

#### Example Structure:
```
Application
├── jvm/memory         (4 charts: heap usage, committed, max, utilization)
├── jvm/gc            (2 charts: collections, time)
├── servlet           (2 charts: requests, response time)
└── session           (2 charts: active, lifecycle)
```

NOT:
```
Application
└── jvm
    └── memory
        └── heap
            └── usage  (1 chart - BAD!)
```

The goal is logical organization that helps users navigate, not unnecessary nesting that makes charts harder to find.

## Best Practices Summary

1. **Consider cardinality control** for dynamic instances - but not always required
2. **Make configuration files self-documenting** with examples
3. **Handle errors gracefully** - partial data is better than no data
4. **Use labels** for instance identification in charts
5. **Expose version information as labels** on all charts for multi-version applications
6. **Clean instance names** before using in metric IDs
7. **Track instance lifecycle** to add/remove charts dynamically
8. **Provide comprehensive metadata** for the marketplace
9. **Test with large-scale scenarios** if implementing cardinality limits
10. **Log warnings** when limits are reached or data is filtered
11. **Follow existing patterns** - check similar collectors for examples
12. **Use context parameters** in all interface methods
13. **Reuse clients** when possible, reconnect on failure
14. **Split complex collectors** into multiple collection files
15. **Be consistent with precision** when converting floats
16. **Use appropriate thresholds** - fixed for percentages and hard errors (i.e. testing non-zero errors), dynamic for other metrics
17. **Route alerts appropriately** - use silent for non-critical issues

## Checklist

When finishing a new collector or improvements on a collector, follow this checklist:

1. Perform an independent **Chart Design Review** for ALL charts in the collector. For each chart, verify and document:
   - **Context**: The context string used (e.g., `websphere_mp.jvm_memory_heap_usage`)
   - **Title**: The chart title shown to users
   - **Instances**: How instances are identified (labels, naming patterns)
   - **Units**: The units used (must be valid Netdata units, note if uncertain)
   - **Labels**: All labels applied (e.g., `server_name`, `app_version`)
   - **Family**: The family grouping related charts
   - **Dimensions**: List each dimension with its original metric name
   - **Rule #1 Compliance**: Verify dimensions are non-overlapping and additive
   
   Additionally, provide a **Chart Hierarchy Tree** showing the organization of all charts:
   ```
   WebSphere MP
   ├── memory (priority: 1100)
   │   ├── websphere_mp.jvm_memory_heap_usage [bytes] - JVM Heap Memory Usage
   │   ├── websphere_mp.jvm_memory_heap_committed [bytes] - JVM Heap Memory Committed
   │   ├── websphere_mp.jvm_memory_heap_max [bytes] - JVM Heap Memory Maximum
   │   └── websphere_mp.jvm_heap_utilization [percentage] - JVM Heap Utilization
   ├── cpu (priority: 1200)
   │   ├── websphere_mp.cpu_usage [percentage] - JVM CPU Usage
   │   ├── websphere_mp.cpu_time [seconds] - JVM CPU Time
   │   ├── websphere_mp.cpu_processors [processors] - Available Processors
   │   └── websphere_mp.system_load [load] - System Load Average
   ├── gc (priority: 1300)
   │   ├── websphere_mp.jvm_gc_collections [collections/s] - JVM Garbage Collection
   │   └── websphere_mp.jvm_gc_time [milliseconds] - JVM GC Time
   ├── threads (priority: 1400)
   │   ├── websphere_mp.jvm_threads_current [threads] - JVM Current Threads
   │   └── websphere_mp.jvm_threads_peak [threads] - JVM Peak Threads
   ├── servlet (priority: 1500)
   │   ├── websphere_mp.servlet_requests [requests/s] - Servlet Requests
   │   └── websphere_mp.servlet_response_time [milliseconds] - Servlet Response Time
   └── session (priority: 1600)
       ├── websphere_mp.session_active [sessions] - Active Sessions
       └── websphere_mp.session_lifecycle [sessions/s] - Session Lifecycle
   ```
   Sort families by ascending priority (MIN of contained chart priorities defines family priority)
2. Review the code for any mock data, remaining TODOs, frictional logic, frictional API endpoints, frictional response formats, frictional members, etc. An independent reality check **MUST** be performed.
3. Review the code to ensure that different versions of the monitored application can be monitored. Our code **MUST** support all possible versions that may be in production out there and gracefully enable/disable features based on what is available. **CRITICAL**: Admin configuration **MUST** always take precedence over auto-detection. Auto-detection should only set intelligent defaults that admins can override. If an admin explicitly enables a feature, the collector **MUST** attempt to use it (with appropriate error handling) regardless of detected version.
4. Review all logs generated by the code to ensure users a) have enough information to understand the selections the code made, b) have descriptive and detailed logs on failures, c) they are not spammed by repeated logs on failures that are supposed to be permanent (like a not supported feature by the monitored application)
5. The code, the stock configuration, the metadata.yaml, the config_schema.json and any stock alerts **MUST** match 100%. Even the slightest variation between them in config keys, possible values, enum values, contexts, dimension names, etc LEADS TO A NON WORKING SOLUTION. So, at the end of every change, an independent sync check **MUST** be performed. While doing this work, ensure also README.md reflects the facts.
6. The metadata.yaml is the PRIMARY documentation source shown on the Netdata integrations page - ensure troubleshooting information, setup instructions, and all documentation is up-to-date in BOTH README.md AND metadata.yaml.
7. Once the above work is finished, the code **MUST** compile without errors or warnings. Use `build-claude` for build directory to avoid any interference with other builds and IDEs.
8. Once all the above work is done and there are no outstanding issues, perform an independent usefulness check of the module being worked and provide a DevOps/SRE expert opinion on how useful, complete, powerful, comprehensive the module is.

## Build Notes
- To compile ibm.d.plugin, use cmake in `build-claude` directory with target `ibm-plugin`
