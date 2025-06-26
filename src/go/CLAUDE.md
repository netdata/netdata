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

## Best Practices Summary

1. **Consider cardinality control** for dynamic instances - but not always required
2. **Make configuration files self-documenting** with examples
3. **Handle errors gracefully** - partial data is better than no data
4. **Use labels** for instance identification in charts
5. **Clean instance names** before using in metric IDs
6. **Track instance lifecycle** to add/remove charts dynamically
7. **Provide comprehensive metadata** for the marketplace
8. **Test with large-scale scenarios** if implementing cardinality limits
9. **Log warnings** when limits are reached or data is filtered
10. **Follow existing patterns** - check similar collectors for examples
11. **Use context parameters** in all interface methods
12. **Reuse clients** when possible, reconnect on failure
13. **Split complex collectors** into multiple collection files
14. **Be consistent with precision** when converting floats
15. **Use appropriate thresholds** - fixed for percentages and hard errors (i.e. testing non-zero errors), dynamic for other metrics
16. **Route alerts appropriately** - use silent for non-critical issues

## Checklist

When finishing a new collector or improvements on a collector, follow this checklist:

1. Review the code for any mock data, remaining TODOs, frictional logic, frictional API endpoints, frictional response formats, frictional members, etc. An independent reality check **MUST** be performed.
2. The code, the stock configuration, the metadata.yaml, the config_schema.json and any stock alerts **MUST** match 100%. Even the slightest variation between them in config keys, possible values, enum values, contexts, dimension names, etc LEADS TO A NON WORKING SOLUTION. So, at the end of every change, an independent sync check **MUST** be performed. While doing this work, ensure also README.md reflects the facts.
3. Once the above work is finished, the code **MUST** compile without errors or warnings. Use `build-claude` for build directory to avoid any interference with other builds and IDEs.
4. Once all the above work is done and there are no outstanding issues, perform an independent usefulness check of the module being worked and provide a DevOps/SRE expert opinion on how useful, complete, powerful, comprehensive the module is.