# go.d Collector Lifecycle Guide

This document provides a comprehensive guide to the lifecycle and data flows of go.d framework collectors. It covers everything a new developer needs to understand how to build robust, efficient collectors that integrate seamlessly with Netdata.

## Table of Contents

1. [Overview](#overview)
2. [Module Interface](#module-interface)
3. [Job Lifecycle](#job-lifecycle)
4. [Configuration Management](#configuration-management)
5. [Chart Management](#chart-management)
6. [Data Collection](#data-collection)
7. [Connection Management](#connection-management)
8. [Error Handling](#error-handling)
9. [Performance Optimization](#performance-optimization)
10. [Best Practices](#best-practices)

## Overview

The go.d framework provides a structured approach to data collection that ensures reliability, performance, and maintainability. Every collector follows a well-defined lifecycle managed by the job framework.

### Core Principles

1. **Missing Data is Data**: Never fake or cache values - gaps indicate real issues
2. **Graceful Degradation**: Collect what you can, warn about what you can't
3. **Minimal Impact**: Optimize for minimal resource usage on monitored systems
4. **Admin Override**: User configuration always takes precedence over auto-detection

## Module Interface

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

## Job Lifecycle

### 1. Initialization Phase

**Job Creation → Auto-Detection → Start**

```mermaid
graph TD
    A[Job Created] --> B[Init()]
    B --> C[Check()]
    C --> D[Charts()]
    D --> E[Start Collection Loop]
    B --> F[Init Failed]
    C --> G[Check Failed - Retry]
    F --> H[Job Disabled]
    G --> I{Retries Left?}
    I -->|Yes| C
    I -->|No| H
```

### Auto-Detection Process

```go
func (j *Job) AutoDetection() error {
    // 1. Initialize module
    if err := j.init(); err != nil {
        return err  // Fatal - job disabled
    }
    
    // 2. Check connectivity/validity
    if err := j.check(); err != nil {
        return err  // Retryable - decrements AutoDetectTries
    }
    
    // 3. Validate charts
    if err := j.postCheck(); err != nil {
        return err  // Fatal - job disabled
    }
    
    return nil
}
```

**Auto-Detection Behavior:**
- **Stock jobs**: Muted during auto-detection to reduce noise
- **Custom jobs**: Normal logging throughout
- **Retry mechanism**: Controlled by `AutoDetectEvery` and `AutoDetectTries`
- **Panic recovery**: Automatic with stack traces in debug mode

### 2. Collection Phase

**Continuous Collection Loop with Penalty System**

```go
func (j *Job) Start() {
    for {
        select {
        case <-j.stop:
            break
        case t := <-j.tick:
            // Apply penalty for failed collections
            if t%(j.updateEvery+j.penalty()) == 0 {
                j.runOnce()
            }
        }
    }
}
```

**Penalty System:**
- **Progressive backoff**: Failed collections increase collection interval
- **Maximum penalty**: 600 seconds (10 minutes)
- **Recovery**: Successful collections reset penalty

### 3. Cleanup Phase

**Graceful Shutdown with Resource Cleanup**

```go
func (j *Job) Cleanup() {
    // Mark charts as obsolete
    for _, chart := range *j.charts {
        if chart.created {
            chart.MarkRemove()
            j.createChart(chart)  // Send CHART command with "obsolete" flag
        }
    }
}
```

## Configuration Management

### Configuration Structure

```go
type Config struct {
    Vnode       string `yaml:"vnode,omitempty" json:"vnode"`
    UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
    
    // Connection configuration
    DSN     string           `yaml:"dsn" json:"dsn"`
    Timeout confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
    
    // Feature flags
    CollectAdvancedMetrics *bool `yaml:"collect_advanced_metrics" json:"collect_advanced_metrics"`
    
    // Filtering
    InstanceSelector matcher.SimpleExpr `yaml:"instance_selector,omitempty" json:"instance_selector"`
}
```

### Configuration Precedence

**CRITICAL**: Admin configuration always overrides auto-detection:

```go
func (c *Collector) Init() {
    // 1. Detect capabilities
    c.detectVersion()
    
    // 2. Set defaults ONLY if admin hasn't configured
    if c.Config.CollectAdvancedFeatures == nil {
        defaultValue := c.supportsAdvancedFeatures()
        c.Config.CollectAdvancedFeatures = &defaultValue
    }
}

func (c *Collector) collect() {
    // 3. Always attempt what admin configured
    if *c.Config.CollectAdvancedFeatures {
        if err := c.collectAdvancedFeatures(); err != nil {
            c.Warningf("Advanced features failed: %v", err)
            // Continue collection - don't fail entire job
        }
    }
}
```

### Validation Patterns

```go
func (c *Collector) Init(context.Context) error {
    if err := c.validateConfig(); err != nil {
        return fmt.Errorf("config validation: %v", err)  // Fatal error
    }
    
    // Initialize expensive resources once
    if err := c.initClient(); err != nil {
        return fmt.Errorf("init client: %v", err)
    }
    
    return nil
}

func (c *Collector) validateConfig() error {
    if c.URL == "" {
        return errors.New("url is required")
    }
    return nil
}
```

## Chart Management

### Static Charts (Always Present)

```go
var baseCharts = module.Charts{
    {
        ID:       "cpu_usage",
        Title:    "CPU Usage",
        Units:    "percentage",
        Fam:      "cpu",
        Ctx:      "mymodule.cpu_usage",
        Priority: prioCPUUsage,
        Type:     module.Stacked,
        Dims: module.Dims{
            {ID: "user", Name: "user"},
            {ID: "system", Name: "system"},
        },
    },
}

func (c *Collector) Charts() *module.Charts {
    return c.charts  // Return base charts
}
```

### Dynamic Charts (Instance-Based)

```go
func newInstanceCharts(instanceName string) *module.Charts {
    charts := instanceChartsTmpl.Copy()
    
    for _, chart := range *charts {
        chart.ID = fmt.Sprintf(chart.ID, cleanName(instanceName))
        chart.Labels = []module.Label{
            {Key: "instance", Value: instanceName},
        }
        for _, dim := range chart.Dims {
            dim.ID = fmt.Sprintf(dim.ID, cleanName(instanceName))
        }
    }
    
    return charts
}
```

### Chart Lifecycle Management

**Basic Pattern (Simple boolean tracking):**

```go
func (c *Collector) collectInstances(mx map[string]int64) {
    seen := make(map[string]bool)
    
    for _, instance := range getInstances() {
        seen[instance.Name] = true
        
        // Add charts for new instances
        if !c.collected[instance.Name] {
            c.collected[instance.Name] = true
            charts := newInstanceCharts(instance.Name)
            c.charts.Add(*charts...)
        }
        
        // Collect metrics
        collectInstanceMetrics(instance, mx)
    }
    
    // Remove stale instances
    for name := range c.collected {
        if !seen[name] {
            delete(c.collected, name)
            c.removeInstanceCharts(name)
        }
    }
}
```

**Advanced Pattern (Staleness detection with tolerance):**

```go
type cacheEntry struct {
    seen         bool
    notSeenTimes int  // Track consecutive misses
    charts       []*module.Chart
}

func (c *Collector) collectInstances(mx map[string]int64) {
    // Reset seen flags
    for _, entry := range c.cache.entries {
        entry.seen = false
    }
    
    for _, instance := range getInstances() {
        entry := c.cache.entries[instance.Name]
        if entry == nil {
            entry = &cacheEntry{charts: newInstanceCharts(instance.Name)}
            c.cache.entries[instance.Name] = entry
            c.charts.Add(*entry.charts...)
        }
        
        entry.seen = true
        entry.notSeenTimes = 0
        collectInstanceMetrics(instance, mx)
    }
    
    // Remove stale instances with tolerance
    for name, entry := range c.cache.entries {
        if entry.seen {
            continue
        }
        
        // Allow N missed collections before removal
        if entry.notSeenTimes++; entry.notSeenTimes >= maxNotSeenTimes {
            for _, chart := range entry.charts {
                chart.MarkRemove()
                chart.MarkNotCreated()
            }
            delete(c.cache.entries, name)
        }
    }
}
```

### Chart Obsolescence

```go
func (c *Collector) removeInstanceCharts(instanceName string) {
    cleanName := cleanName(instanceName)
    
    for _, chart := range *c.charts {
        if strings.HasPrefix(chart.ID, fmt.Sprintf("instance_%s_", cleanName)) {
            if !chart.Obsolete {
                chart.Obsolete = true
                chart.MarkNotCreated()  // Trigger CHART command with obsolete flag
                c.Debugf("Marked chart %s as obsolete", chart.ID)
            }
        }
    }
}
```

### Dimension ID Management

**CRITICAL**: Understanding dimension IDs vs Names:

1. **Dimension IDs must be unique across the entire job** (all charts, all contexts)
2. **The framework sends dimension Names as both ID and name to Netdata**
3. **NIDL compliance is achieved through shared dimension Names, not IDs**

```go
// For dynamic instances - unique dimension IDs required
chart.Dims = append(chart.Dims, &module.Dim{
    ID:   "thread_pool_instance1_active",  // Must be unique across job
    Name: "active",                         // This is what Netdata sees
})

// In your mx map - must match dim.ID exactly
mx["thread_pool_instance1_active"] = 42
```

### Advanced Chart Creation Patterns

**Configuration-based chart modification:**

```go
func (c *Collector) addContainerCharts(name, image string) {
    charts := containerChartsTmpl.Copy()
    
    // Remove charts based on configuration
    if !c.CollectContainerSize {
        _ = charts.Remove(containerWritableLayerSizeChartTmpl.ID)
    }
    
    if !c.CollectContainerHealth {
        _ = charts.Remove(containerHealthStatusChartTmpl.ID)
    }
    
    // Modify charts for specific container types
    if strings.Contains(image, "database") {
        // Add database-specific dimensions
        for _, chart := range *charts {
            if chart.ID == "container_cpu" {
                chart.Dims = append(chart.Dims, 
                    &module.Dim{ID: "db_cpu_time", Name: "database"})
            }
        }
    }
    
    c.charts.Add(*charts...)
}
```

**Multiple metric type handling:**

```go
func (c *Collector) collectPrometheusMetrics(mx map[string]int64) {
    for _, mf := range c.metricFamilies {
        switch mf.Type() {
        case model.MetricTypeGauge:
            c.collectGauge(mx, mf)
        case model.MetricTypeCounter:
            c.collectCounter(mx, mf)
        case model.MetricTypeSummary:
            c.collectSummary(mx, mf)
        case model.MetricTypeHistogram:
            c.collectHistogram(mx, mf)
        case model.MetricTypeUnknown:
            // Fallback type detection
            if c.isFallbackTypeGauge(mf.Name()) {
                c.collectGauge(mx, mf)
            } else if c.isFallbackTypeCounter(mf.Name()) || 
                      strings.HasSuffix(mf.Name(), "_total") {
                c.collectCounter(mx, mf)
            }
        }
    }
}
```

**Hard limits (NOT RECOMMENDED):**

Some collectors implement hard limits, but this approach has significant drawbacks:

```go
// PROBLEMATIC: All-or-nothing approach
func (c *Collector) collectWithHardLimits(mx map[string]int64) {
    for _, metric := range getAllMetrics() {
        // This skips the ENTIRE metric family if over limit
        if c.MaxTSPerMetric > 0 && len(metric.TimeSeries()) > c.MaxTSPerMetric {
            c.Debugf("metric '%s' num of time series (%d) > limit (%d), skipping",
                metric.Name(), len(metric.TimeSeries()), c.MaxTSPerMetric)
            continue  // Loses ALL data for this metric!
        }
    }
}
```

**Why hard limits are problematic:**
- If limit is 100 and metric has 110 time series, you get 0 instead of 100
- No way to determine which instances are "important"
- Unpredictable data loss from user perspective
- Better to use selector-based filtering (see Cardinality Protection section)

## Data Collection

### Collection Return Values

**Different return scenarios and their meanings:**

```go
func (c *Collector) Collect(context.Context) map[string]int64 {
    mx, err := c.collect()
    if err != nil {
        c.Error(err)  // Log error
    }
    
    if len(mx) == 0 {
        return nil    // Complete failure - shows gaps
    }
    
    return mx        // Partial or complete success
}
```

**Return Value Guidelines:**
- **`nil`**: Complete collection failure - creates gaps
- **Empty map**: No data available - creates gaps  
- **Partial map**: Some metrics missing - missing dimensions get `SETEMPTY`
- **Complete map**: All metrics collected successfully

### Gap Handling Philosophy

**NEVER cache or fake data - missing data is meaningful:**

```go
// CORRECT: Only send metrics actually collected
func (c *Collector) collect() map[string]int64 {
    mx := make(map[string]int64)
    
    currentMetrics := c.fetchFromApplication()
    for name, value := range currentMetrics {
        mx[name] = value  // Only real values
    }
    
    return mx  // Gaps will show if metrics missing
}

// WRONG: Never cache old values
func (c *Collector) collect() map[string]int64 {
    mx := make(map[string]int64)
    
    currentMetrics := c.fetchFromApplication()
    if len(currentMetrics) == 0 {
        // NEVER DO THIS!
        mx["metric"] = c.lastValue  // Hides real problems
    }
    
    return mx
}
```

### Partial Collection Patterns

```go
func (c *Collector) collect() (map[string]int64, error) {
    mx := make(map[string]int64)
    
    // Core metrics - must succeed
    if err := c.collectCoreMetrics(mx); err != nil {
        return nil, fmt.Errorf("core metrics failed: %v", err)
    }
    
    // Optional metrics - failures are warnings
    if c.Config.CollectAdvanced {
        if err := c.collectAdvancedMetrics(mx); err != nil {
            c.Warningf("advanced metrics failed: %v", err)
            // Continue with core metrics
        }
    }
    
    return mx, nil
}
```

### Floating Point Precision

**Basic precision handling:**

```go
const precision = 1000

// Collection: Convert float to int64
floatValue := 1.345
mx["metric"] = int64(floatValue * precision)  // 1345

// Chart definition: Restore precision
{
    Dims: module.Dims{
        {ID: "metric", Name: "value", Div: precision},  // 1345 / 1000 = 1.345
    },
}
```

**Advanced precision for different metric types:**

```go
func (c *Collector) collectPrometheusMetrics(mx map[string]int64) {
    for _, mf := range c.metricFamilies {
        switch mf.Type() {
        case model.MetricTypeGauge:
            // Standard precision for gauges
            mx[id] = int64(mf.Gauge().Value() * precision)
            
        case model.MetricTypeSummary:
            // Double precision for quantiles (more granular)
            for _, q := range mf.Summary().Quantile() {
                dimID := fmt.Sprintf("%s_quantile_%s", id, formatQuantile(q.Quantile()))
                mx[dimID] = int64(q.Value() * precision * precision)
            }
            
        case model.MetricTypeHistogram:
            // Different precision for different bucket types
            for _, bucket := range mf.Histogram().Bucket() {
                bucketID := fmt.Sprintf("%s_bucket_%s", id, formatBound(bucket.UpperBound()))
                mx[bucketID] = int64(bucket.CumulativeCount())  // No precision - count is integer
            }
        }
    }
}
```

### Cardinality Protection

**Best Practice: Selector-Based Filtering**

The recommended approach for cardinality protection is **selective monitoring using regular expressions** rather than hard limits. This pattern is already widely used in production go.d collectors.

**Common Implementations:**

1. **Simple Pattern Matcher** (most common):
```go
type Config struct {
    // Empty by default - monitor everything
    // Users explicitly define what's important to them
    InstanceSelector string `yaml:"instance_selector,omitempty"`
}

func (c *Collector) Init() error {
    if c.InstanceSelector != "" {
        // Supports glob patterns: *, ?, and exclusions with !
        matcher, err := matcher.NewSimplePatternsMatcher(c.InstanceSelector)
        if err != nil {
            return fmt.Errorf("invalid instance selector: %w", err)
        }
        c.instanceMatcher = matcher
    }
    // By default, no filter - collect everything
}

func (c *Collector) collectInstances() {
    for _, instance := range getAllInstances() {
        // Skip if selector is defined and doesn't match
        if c.instanceMatcher != nil && !c.instanceMatcher.MatchString(instance.Name) {
            continue
        }
        
        // Collect the instance
        c.collectInstance(instance)
    }
}
```

2. **Include/Exclude Arrays** (MongoDB style):
```go
type Config struct {
    Databases matcher.SimpleExpr `yaml:"databases,omitempty"`
}

// Configuration:
// databases:
//   includes:
//     - "prod_*"
//     - "staging_*"
//   excludes:
//     - "*_test"
```

3. **Regular Expression** (MQ PCF style):
```go
type Config struct {
    QueueSelector string `yaml:"queue_selector"`
}

func (c *Collector) Init() error {
    if c.QueueSelector != "" {
        c.queueSelectorRegex, err = regexp.Compile(c.QueueSelector)
        if err != nil {
            return fmt.Errorf("invalid queue_selector regex: %w", err)
        }
    }
}
```

**Real-World Examples:**
- **Docker**: `container_selector` to filter containers
- **PostgreSQL**: `collect_databases_matching` for database selection
- **MongoDB**: `databases` include/exclude arrays
- **DB2**: Multiple selectors for different object types
- **MQ PCF**: Regex-based queue and channel selectors
- **VSphere**: Hierarchical path matching for hosts/VMs

**Why This Approach?**
1. **User Control**: Users explicitly choose what to monitor
2. **Predictable**: No surprises about what gets collected
3. **Flexible**: Supports glob patterns, regex, or include/exclude lists
4. **Battle-tested**: Already proven in production collectors
5. **No Arbitrary Decisions**: Avoids "which 100 out of 110?" problem

**Configuration Examples:**
```yaml
# Simple patterns (glob-style)
- name: postgres_prod
    dsn: postgresql://localhost
    collect_databases_matching: "prod_* staging_* !*_test"

# Regular expressions
- name: mq_queues
    queue_selector: "^(PROD|STAGE)\\..*"

# Include/Exclude arrays
- name: mongodb
    databases:
      includes:
        - "myapp_*"
        - "analytics_*"
      excludes:
        - "*_backup"
        - "*_temp"
```

### Advanced Filtering Patterns

**Label-based filtering:**

```go
func (c *Collector) shouldCollectContainer(container containerInfo) bool {
    // Check ignore labels
    if container.Labels["netdata.cloud/ignore"] == "true" {
        return false
    }
    
    // Check namespace filtering
    if c.Config.NamespaceSelector != "" {
        if !c.namespaceSelector.MatchString(container.Namespace) {
            return false
        }
    }
    
    // Check image filtering
    if c.Config.ImageSelector != "" {
        if !c.imageSelector.MatchString(container.Image) {
            return false
        }
    }
    
    return true
}
```

**Version-based feature detection:**

```go
func (c *Collector) collectVersionSpecificMetrics(mx map[string]int64) {
    if c.systemdVersion >= 230 {
        // Feature available in systemd 230+
        units, err := c.getLoadedUnitsByPatterns(conn)
        if err != nil {
            c.Warningf("failed to get units by patterns: %v", err)
            return
        }
        // Process newer API results
    } else {
        // Fallback for older versions
        units, err := c.getLoadedUnits(conn)
        if err != nil {
            c.Warningf("failed to get units: %v", err)
            return
        }
        // Process older API results
    }
}
```

## Connection Management

### Connection Establishment Patterns

**HTTP-based collectors - Create in Init():**

```go
func (c *Collector) Init(context.Context) error {
    httpClient, err := web.NewHTTPClient(c.ClientConfig)
    if err != nil {
        return fmt.Errorf("failed initializing http client: %w", err)
    }
    c.httpClient = httpClient
    return nil
}
```

**Database collectors - Lazy initialization in collect():**

```go
func (c *Collector) collect() (map[string]int64, error) {
    if c.db == nil {
        if err := c.openConnection(); err != nil {
            return nil, err
        }
    }
    
    return c.queryMetrics()
}

func (c *Collector) openConnection() error {
    db, err := sql.Open("postgres", c.DSN)
    if err != nil {
        return fmt.Errorf("error opening connection: %v", err)
    }
    
    // Configure connection pool
    db.SetMaxOpenConns(1)
    db.SetMaxIdleConns(1)
    db.SetConnMaxLifetime(10 * time.Minute)
    
    // CRITICAL: Always test connection
    ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
    defer cancel()
    
    if err := db.PingContext(ctx); err != nil {
        _ = db.Close()  // Clean up failed connection
        return fmt.Errorf("connection test failed: %v", err)
    }
    
    c.db = db
    return nil
}
```

**Non-persistent connections (for specific use cases):**

```go
func (c *Collector) collect() (map[string]int64, error) {
    // Create client each time - for APIs that don't benefit from persistence
    if c.client == nil {
        client, err := c.newClient(c.Config)
        if err != nil {
            return nil, err
        }
        c.client = client
    }
    
    // Close after use - not persistent for Docker/container APIs
    defer func() { _ = c.client.Close() }()
    
    // One-time setup operations
    if !c.verNegotiated {
        c.verNegotiated = true
        c.negotiateAPIVersion()
    }
    
    return c.queryMetrics()
}
```

### Context-Aware Timeout Management

```go
func (c *Collector) collectWithTimeouts(mx map[string]int64) error {
    // Different timeouts for different operations
    
    // Quick operations
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
    defer cancel()
    
    c.Debugf("calling function 'ListUnits'")
    units, err := c.conn.ListUnitsContext(ctx)
    if err != nil {
        return fmt.Errorf("failed to list units: %v", err)
    }
    
    // Longer timeout for expensive operations
    ctx, cancel = context.WithTimeout(context.Background(), c.Timeout.Duration())
    defer cancel()
    
    c.Debugf("calling function 'GetUnitProperties'")
    for _, unit := range units {
        props, err := c.conn.GetUnitPropertiesContext(ctx, unit.Name)
        if err != nil {
            c.Warningf("failed to get properties for %s: %v", unit.Name, err)
            continue  // Skip this unit, continue with others
        }
        
        c.collectUnitProperties(mx, unit.Name, props)
    }
    
    return nil
}
```

### Reconnection Strategies

```go
func (c *Collector) collect() (map[string]int64, error) {
    // Try with existing connection
    if err := c.ping(); err != nil {
        c.Warningf("connection failed, attempting reconnection: %v", err)
        
        // Clean up and reconnect
        c.Cleanup(context.TODO())
        if err := c.openConnection(); err != nil {
            return nil, err
        }
        
        // Retry after reconnection
        if err := c.ping(); err != nil {
            return nil, err
        }
    }
    
    return c.queryMetrics()
}
```

### Resource Cleanup

```go
func (c *Collector) Cleanup(context.Context) {
    if c.db != nil {
        if err := c.db.Close(); err != nil {
            c.Warningf("cleanup error: %v", err)
        }
        c.db = nil
    }
    
    if c.httpClient != nil {
        c.httpClient.CloseIdleConnections()
    }
}
```

## Error Handling

### Error Hierarchy

**Init Errors (Fatal - Job Disabled):**

```go
func (c *Collector) Init(context.Context) error {
    if err := c.validateConfig(); err != nil {
        return fmt.Errorf("config validation: %v", err)  // Fatal
    }
    return nil
}
```

**Check Errors (Retryable during Auto-Detection):**

```go
func (c *Collector) Check(context.Context) error {
    mx, err := c.collect()
    if err != nil {
        return err  // Retryable - decrements AutoDetectTries
    }
    
    if len(mx) == 0 {
        return errors.New("no metrics collected")
    }
    
    return nil
}
```

**Collect Errors (Graceful - Shows Gaps):**

```go
func (c *Collector) Collect(context.Context) map[string]int64 {
    mx, err := c.collect()
    if err != nil {
        c.Error(err)  // Log but don't fail job
    }
    
    if len(mx) == 0 {
        return nil    // Show gaps
    }
    
    return mx
}
```

### Logging Patterns

**Use appropriate log levels:**

```go
// Error: Failures preventing collection
c.Errorf("database connection failed: %v", err)

// Warning: Features not available or partial failures
c.Warningf("advanced metrics not available: %v", err)

// Info: Important lifecycle events
c.Info("check success")
c.Infof("started, collection interval %ds", interval)

// Debug: Detailed troubleshooting information
c.Debugf("query completed in %v", duration)
```

### Panic Recovery

The framework automatically handles panics with recovery and stack traces:

```go
// Framework provides automatic panic recovery
func (j *Job) collect() (result map[string]int64) {
    defer func() {
        if r := recover(); r != nil {
            j.panicked = true
            j.Errorf("PANIC: %v", r)
            if logger.Level.Enabled(slog.LevelDebug) {
                j.Errorf("STACK: %s", debug.Stack())
            }
        }
    }()
    
    result = j.module.Collect(context.TODO())
    return result
}
```

## Performance Optimization

### Resource Creation Strategy

**Create Once in Init():**

```go
func (c *Collector) Init(context.Context) error {
    // Expensive one-time operations
    c.httpClient = web.NewHTTPClient(c.ClientConfig)
    c.selector, _ = matcher.NewSimpleExprMatcher(c.Selector)
    c.responseRegex = regexp.MustCompile(c.ResponsePattern)
    return nil
}
```

**Reuse in collect():**

```go
func (c *Collector) collect() (map[string]int64, error) {
    // Only fresh data collection - reuse expensive objects
    req, _ := web.NewHTTPRequest(c.RequestConfig)
    resp, err := c.httpClient.Do(req)  // Reuse client
    // ...
}
```

### Connection Optimization

```go
// Single connection per collector
db.SetMaxOpenConns(1)
db.SetMaxIdleConns(1) 
db.SetConnMaxLifetime(10 * time.Minute)

// Reuse temporary resources
func (c *Collector) Init() error {
    c.responseQueue = c.mq.CreateTemporaryQueue()  // Once
    return nil
}

func (c *Collector) collect() {
    response := c.mq.QueryMetrics(c.responseQueue)  // Reuse
}
```

### Caching Strategies

```go
type Collector struct {
    recheckSettingsTime  time.Time
    recheckSettingsEvery time.Duration
    addChartsOnce        *sync.Once
}

func (c *Collector) collect() {
    // Cache expensive queries with time-based invalidation
    if time.Since(c.recheckSettingsTime) > c.recheckSettingsEvery {
        c.recheckSettingsTime = time.Now()
        c.recheckExpensiveSettings()
    }
    
    // Add charts only once when feature becomes available
    if c.featureDetected {
        c.addChartsOnce.Do(func() {
            c.charts.Add(*newFeatureCharts()...)
        })
    }
}
```

## Best Practices

### Module Structure

**Basic structure:**

```go
type Collector struct {
    module.Base
    Config `yaml:",inline" json:""`
    
    charts *module.Charts
    
    // Connections
    client someClient
    db     *sql.DB
    
    // State tracking
    collected map[string]bool
    seen      map[string]bool
    
    // Feature detection
    version               string
    addFeatureChartsOnce *sync.Once
}
```

**Advanced structure with comprehensive patterns:**

```go
type Collector struct {
    module.Base
    Config `yaml:",inline" json:""`
    
    charts *module.Charts
    
    // Connection management
    client      someClient
    db          *sql.DB
    connRetries int
    
    // Advanced instance tracking
    cache struct {
        entries map[string]*cacheEntry
        maxAge  time.Duration
    }
    
    // Version and feature detection
    version               string
    systemdVersion        int
    detectedFeatures      map[string]bool
    verNegotiated         bool
    
    // Conditional chart addition
    addFeatureChartsOnce   *sync.Once
    addExtendedChartsOnce  *sync.Once
    addAdvancedChartsOnce  *sync.Once
    
    // Cardinality control
    maxInstances     int
    maxTSPerMetric   int
    
    // Filtering and selection
    namespaceSelector matcher.Matcher
    imageSelector     matcher.Matcher
    
    // Periodic operations
    recheckTime      time.Time
    recheckInterval  time.Duration
    
    // Performance tracking
    lastCollectionTime time.Time
    collectionDuration time.Duration
}
```

### Configuration Best Practices

1. **Use pointer fields for tri-state configuration** (`*bool` instead of `bool`)
2. **Provide sensible defaults** in the constructor
3. **Always validate configuration** in `Init()`
4. **Support admin override** of auto-detection

### Chart Design Rules

1. **Non-overlapping dimensions**: Dimensions must be additive or comparable
2. **Appropriate chart types**: Stacked for volume, Area for bidirectional, Line for independent
3. **Dynamic creation**: Only create charts when metrics are available
4. **Proper obsolescence**: Mark charts obsolete when instances disappear

### Data Collection Rules

1. **Never fake data**: Missing data creates meaningful gaps
2. **Graceful degradation**: Collect what you can, warn about failures
3. **Proper precision**: Use consistent precision multipliers for floats
4. **Respect gaps**: Don't cache old values to fill missing data
5. **Advanced precision**: Use different precision for different metric types (gauges vs quantiles)
6. **Cardinality protection**: Use selector-based filtering, not hard limits
7. **Label-based filtering**: Support application-specific ignore patterns

### Connection Management Rules

1. **Persistent connections**: Create once, reuse throughout lifecycle (preferred)
2. **Non-persistent connections**: Use for APIs that don't benefit from persistence
3. **Proper validation**: Always test connections before storing
4. **Automatic recovery**: Recreate failed connections in collect()
5. **Clean shutdown**: Always implement proper cleanup
6. **Context-aware timeouts**: Use different timeouts for different operations
7. **API version negotiation**: Handle version-specific features gracefully

### Error Handling Rules

1. **Appropriate log levels**: Error for failures, Warning for missing features
2. **Meaningful messages**: Include context about what failed and why
3. **Graceful failures**: Continue with partial data when possible
4. **Admin respect**: Always attempt configured features
5. **Timeout handling**: Use context with appropriate timeouts
6. **Version-based fallbacks**: Implement fallback logic for older versions

### Advanced Patterns

1. **Staleness detection**: Use tolerance-based removal for unstable instances
2. **Multiple metric types**: Handle different Prometheus metric types appropriately
3. **Feature detection**: Implement version-based feature availability
4. **Configuration-based charts**: Modify charts based on user configuration
5. **Fallback type detection**: Handle ambiguous metric types with intelligent fallbacks

### Performance Optimization Rules

1. **Periodic expensive operations**: Cache expensive queries with time-based invalidation
2. **Batch operations**: Group related operations together
3. **Resource reuse**: Reuse buffers, parsers, and temporary objects
4. **Intelligent caching**: Use advanced caching with staleness detection
5. **Connection pooling**: Configure pools appropriately for collector patterns

This comprehensive guide provides the foundation for building robust, efficient go.d collectors that integrate seamlessly with Netdata's monitoring ecosystem. The advanced patterns documented here are based on analysis of production collectors and represent battle-tested approaches for handling complex, real-world monitoring scenarios.