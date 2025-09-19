# IBM.D Plugin Framework Documentation

This document describes the IBM.D framework - a new framework built on top of go.d framework that provides compile-time safety, automatic chart management, and drastically reduces collector development complexity.

## Quick Reference

### Key Features
- **Compile-time safety** through code generation from YAML
- **Type-safe dimension API** - compile errors for typos, IDE autocomplete
- **Automatic chart lifecycle** - creation, updates, and obsoletion
- **Global labels** automatically applied to all charts
- **Zero boilerplate** - 80%+ code reduction vs traditional go.d
- **Protocol abstraction** with automatic metrics tracking
- **Dynamic instance management** with configurable obsoletion (default: 60 iterations)

### Critical Rules
1. **Never cache data** - Missing data must show as gaps
2. **Use protocols** - Collectors orchestrate, protocols communicate
3. **Follow directory structure** - Standard layout is mandatory
4. **Let framework manage charts** - Never manual chart operations
5. **Float handling** - Pre-multiply by 1000, use `precision: 1, div: 1000` in YAML
6. **Safe defaults** - mul/div/precision all default to 1 (no transformation)

### Testing
```bash
# Build the plugin
./build-ibm.sh

# Test with dump mode (MUST use script and --dump)
sudo script -c '/usr/libexec/netdata/plugins.d/ibm.d.plugin -m MODULE -d --dump=3s --dump-summary 2>&1' /dev/null

# Check for issues
sudo script -c '/usr/libexec/netdata/plugins.d/ibm.d.plugin -m MODULE -d --dump=3s 2>&1' /dev/null | grep ISSUE
```

## Table of Contents
1. [Framework Overview](#framework-overview)
2. [Core Principles](#core-principles)
3. [Core Components](#core-components)
4. [Module Structure](#module-structure)
5. [Contexts and Metrics](#contexts-and-metrics)
6. [State Management](#state-management)
7. [Configuration](#configuration)
8. [Collection Patterns](#collection-patterns)
9. [Framework Features](#framework-features)
10. [Protocol Abstraction](#protocol-abstraction)
11. [Units and Precision](#units-and-precision)
12. [Automated Documentation Generation](#automated-documentation-generation)
13. [Development Guidelines](#development-guidelines)
14. [Creating a New Collector](#creating-a-new-collector)
15. [Code Generation](#code-generation)
16. [Data Collection Variability](#data-collection-variability)
17. [Sources of Truth](#sources-of-truth)
18. [Mandatory Requirements](#mandatory-requirements)

## Framework Overview

The IBM.D framework is designed to:
- Provide compile-time safety for all metric operations
- Automatically manage chart lifecycle (creation, updates, obsoletion)
- Handle dimension ID uniqueness across the entire job
- Support dynamic instance management with labels
- Reduce boilerplate code by 80%+
- Prevent common pitfalls in collector development

## Core Principles

1. **Compile-Time Safety**: All metric operations and dimension names are type-checked at compile time
2. **Type-Safe Dimensions**: Impossible to misspell dimension names - compile errors catch all typos
3. **Zero Manual Chart Management**: Framework handles all chart lifecycle automatically
4. **Protocol Abstraction**: Collectors orchestrate, protocols handle communication
5. **Automatic Obsoletion**: Instances disappear automatically when no longer present
6. **Global Label Support**: Job-level labels apply to all charts transparently
7. **No Boilerplate**: 80%+ reduction in code compared to traditional go.d modules
8. **Data Integrity**: Never cache or fake data - missing data shows as gaps
9. **Separation of Concerns**: Collectors only orchestrate, never implement protocols

## Core Components

### 1. Framework Package (`framework/`)

The framework provides base functionality that all modules inherit:

```go
// Collector is the base type for all framework-based collectors
type Collector struct {
    module.Base
    Config              Config
    State               *CollectorState
    registeredContexts  []interface{}
    contextMap          map[string]interface{}
    charts              *module.Charts
    impl                CollectorImpl
    globalLabels        []module.Label
}
```

Key framework files:
- `collector.go` - Base collector implementation
- `state.go` - Metric state management  
- `types.go` - Core type definitions
- `protocols.go` - Protocol client helpers with backoff support

### 2. Code Generator (`shared/metricgen/`)

The metricgen tool generates type-safe metric collection code from YAML:

```bash
//go:generate go run github.com/netdata/netdata/go/plugins/plugin/ibm.d/shared/metricgen
```

### 3. Module Implementation

Each module implements the `CollectorImpl` interface:

```go
type CollectorImpl interface {
    CollectOnce() error  // Single collection iteration
}
```

## Module Structure

### Standard Module Layout

```
module/
├── collector.go           # Core struct and CollectOnce orchestration
├── init.go               # Init, Check, Cleanup lifecycle methods
├── collect_*.go          # Collection logic by metric type
├── config.go             # Configuration structure
├── module.go             # Module registration
├── config_schema.json    # Web UI configuration schema
├── contexts/
│   ├── contexts.yaml     # Metric definitions (source of truth)
│   ├── doc.go           # go:generate directive
│   └── zz_generated_contexts.go  # Generated code (never edit)
└── protocol/             # Protocol-specific packages (optional)
    └── client.go
```

### Configuration Files

Stock configuration goes in `config/ibm.d/`:
```
config/ibm.d/
├── module_name.conf      # Stock configuration examples
```

## Contexts and Metrics

### contexts.yaml Structure

```yaml
package: contexts

classes:
  - name: Queue              # Class name (becomes QueueLabels struct)
    contexts:
      - name: Depth
        context: mq.queue.depth
        title: Queue Depth
        units: messages
        family: queues
        type: line
        priority: 1000
        dimensions:
          - { name: current, algo: absolute }
          - { name: max, algo: absolute }
    labels:
      - { key: queue, type: string }
      - { key: type, type: string }
```

### Generated Code Usage

```go
// Type-safe label creation
labels := contexts.QueueLabels{
    Queue: "SYSTEM.DEFAULT.LOCAL.QUEUE",
    Type:  "local",
}

// Type-safe metric setting with compile-time dimension validation
contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
    Current: 42,     // Compile-time checked field name
    Max:     1000,   // IDE autocomplete shows available fields
})
```

### Dimension ID Uniqueness

The framework ensures dimension IDs are unique across the entire job:
- Format: `{instance_id}.{dimension_name}`
- Example: `mq.queue.depth.SYSTEM_DEFAULT_LOCAL_QUEUE.local.current`

## State Management

### Setting Metrics - Type-Safe API

The framework provides **compile-time type-safe** metric setting that prevents dimension name errors:

```go
// Global metrics (no labels) - using type-safe API
contexts.System.CPUUsage.Set(c.State, contexts.EmptyLabels{}, contexts.SystemCPUUsageValues{
    User:   75,
    System: 25,
})

// Labeled metrics - using type-safe API
labels := contexts.QueueLabels{Queue: "Q1", Type: "local"}
contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
    Current: 100,
    Max:     1000,
})
```

### Type-Safe Dimension Validation

The framework ensures **compile-time validation** of dimension names:

```go
// ❌ OLD WAY - Runtime errors possible
c.State.Set(contexts.Queue.Depth, labels, map[string]int64{
    "currnet": 100,  // Typo! Silently ignored at runtime
    "max": 1000,
})

// ✅ NEW WAY - Compile-time safety
contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
    Currnet: 100,  // Compile error! Field 'Currnet' does not exist
    Max: 1000,
})
```

**Benefits:**
- **Impossible to misspell dimension names** - compile errors catch typos
- **IDE autocomplete** - see available dimensions as you type
- **Refactoring safety** - renaming dimensions updates all usages
- **Works with conditional metrics** - validation at compile time, not runtime
- **Zero overhead** - type safety with no performance cost

### Automatic Instance Management

- Framework tracks all instances automatically
- Charts created on first data point
- Charts marked obsolete after `obsoletion_iterations` without data
- No manual instance tracking needed

## Configuration

### Framework Configuration

```go
type Config struct {
    ObsoletionIterations int    `yaml:"obsoletion_iterations"`  // Default: 60
    UpdateEvery          int    `yaml:"update_every"`          // Default: 1
    CollectionGroups     map[string]int    // Future: Named collection intervals
}
```

### Module Configuration

```go
type Config struct {
    framework.Config `yaml:",inline" json:",inline"`  // Embed framework config
    
    // Module-specific fields
    Endpoint string `yaml:"endpoint" json:"endpoint"`
    Timeout  int    `yaml:"timeout" json:"timeout"`
}
```

### Configuration Defaults

**CRITICAL**: Default values must be defined in ONE place to ensure consistency.

1. **Define defaults in `defaultConfig()` function** in init.go:
```go
// defaultConfig returns a new Config with all default values set
func defaultConfig() Config {
    return Config{
        // Connection defaults
        Host:    "localhost",
        Port:    1414,
        Channel: "SYSTEM.DEF.SVRCONN",
        
        // Collection defaults
        CollectQueues:   true,
        CollectChannels: true,
        
        // Selector defaults
        QueueSelector: "*",
    }
}
```

2. **Use `defaultConfig()` in module registration** (module.go):
```go
func New() *Collector {
    return &Collector{
        Collector: framework.Collector{
            Config: framework.Config{
                UpdateEvery:          1,
                ObsoletionIterations: 60,
            },
        },
        Config: defaultConfig(),  // Initialize with defaults
    }
}

func init() {
    module.Register("mymodule", module.Creator{
        Config: func() any {
            config := defaultConfig()  // Same defaults for UI
            return &config
        },
    })
}
```

This ensures:
- YAML unmarshaling preserves defaults for unspecified fields
- Documentation generator extracts defaults from `defaultConfig()`
- Single source of truth for all default values

### Configuration Field Comments

**CRITICAL**: All configuration fields MUST have descriptive comments that become the field descriptions in generated documentation.

The automated documentation generator (`docgen`) extracts field descriptions directly from Go comments in config.go using AST parsing. These comments become the user-facing documentation in:
- README.md configuration tables
- config_schema.json for web UI
- metadata.yaml for integrations page

#### Best Practices for Config Comments

```go
type Config struct {
    framework.Config `yaml:",inline" json:",inline"`

    // IBM MQ Queue Manager name to connect to
    QueueManager string `yaml:"queue_manager" json:"queue_manager"`
    // IBM MQ server hostname or IP address  
    Host         string `yaml:"host" json:"host"`
    // IBM MQ server port number
    Port         int    `yaml:"port" json:"port"`
    // Username for IBM MQ authentication
    User         string `yaml:"user" json:"user"`
    // Password for IBM MQ authentication  
    Password     string `yaml:"password" json:"password"`
    
    // Enable collection of queue metrics
    CollectQueues   bool `yaml:"collect_queues" json:"collect_queues"`
    // Enable collection of system queue metrics (may impact performance)
    CollectSystemQueues bool `yaml:"collect_system_queues" json:"collect_system_queues"`
    
    // Pattern to filter queues (wildcards supported)
    QueueSelector   string `yaml:"queue_selector" json:"queue_selector"`
    // Enable collection of queue statistics (destructive operation)
    CollectResetQueueStats bool `yaml:"collect_reset_queue_stats" json:"collect_reset_queue_stats"`
}
```

#### Comment Guidelines

1. **Descriptive, not redundant**: "IBM MQ Queue Manager name to connect to" not "QueueManager field"
2. **Include important details**: Mention wildcards support, performance impact, destructive operations
3. **Protocol-specific**: Use proper terminology (IBM MQ, not generic "server")
4. **Concise but complete**: One line that tells users exactly what the field does
5. **Actionable**: Help users understand how to configure the field

#### Examples of Good vs Bad Comments

```go
// ❌ BAD: Generic and redundant
// Host configuration option
Host string `yaml:"host" json:"host"`

// ✅ GOOD: Specific and informative  
// IBM MQ server hostname or IP address
Host string `yaml:"host" json:"host"`

// ❌ BAD: Missing important context
// Enable system queues
CollectSystemQueues bool `yaml:"collect_system_queues" json:"collect_system_queues"`

// ✅ GOOD: Includes performance implications
// Enable collection of system queue metrics (may impact performance)
CollectSystemQueues bool `yaml:"collect_system_queues" json:"collect_system_queues"`

// ❌ BAD: No explanation of destructive nature
// Enable reset queue stats
CollectResetQueueStats bool `yaml:"collect_reset_queue_stats" json:"collect_reset_queue_stats"`

// ✅ GOOD: Warns about destructive operation
// Enable collection of queue statistics (destructive operation)
CollectResetQueueStats bool `yaml:"collect_reset_queue_stats" json:"collect_reset_queue_stats"`
```

#### Architecture Benefits

This comment-based approach maintains proper separation of concerns:
- **Framework remains generic**: No module-specific logic in docgen
- **Module-specific documentation**: Each module provides its own descriptions
- **Single source of truth**: Comments in config.go drive all documentation
- **Compile-time validation**: Comments are extracted via AST parsing, ensuring accuracy
- **Developer efficiency**: Write comments once, documentation generates automatically

## Collection Patterns

### Basic Collection Flow

```go
func (c *Collector) CollectOnce() error {
    // 1. Ensure connection (protocol handles reconnection internally)
    if !c.client.IsConnected() {
        if err := c.client.Connect(); err != nil {
            return err  // Let module log the error
        }
    }
    
    // 2. Update global labels (protocol caches and refreshes on reconnect)
    version, edition, err := c.client.GetVersion()
    if err == nil {
        c.SetGlobalLabel("app_version", version)
        c.SetGlobalLabel("app_edition", edition)
        c.SetGlobalLabel("endpoint", c.config.Endpoint)
    }
    
    // 3. Collect different metric types
    if err := c.collectSystemMetrics(); err != nil {
        return err
    }
    
    if c.config.CollectQueues {
        if err := c.collectQueueMetrics(); err != nil {
            c.Warningf("failed to collect queue metrics: %v", err)
            // Continue - partial collection is OK
        }
    }
    
    return nil
}
```

### Protocol-Level Connection Management

Protocols handle their own connection lifecycle and caching:

```go
// Protocol client implementation
type Client struct {
    conn           Connection
    backoff        *framework.ExponentialBackoff
    
    // Cached static data (refreshed on reconnection)
    cachedVersion  string
    cachedEdition  string
}

func (c *Client) Connect() error {
    // Connect with backoff
    err := c.backoff.Retry(func() error {
        return c.doConnect()
    })
    if err != nil {
        return err
    }
    
    // Refresh static data on successful connection
    c.refreshStaticData()
    return nil
}

func (c *Client) GetVersion() (string, string, error) {
    // Returns cached data - refreshed only on reconnection
    return c.cachedVersion, c.cachedEdition, nil
}
```

### Incremental Metrics

For counters that increase over time:

```go
// Collector tracks raw value
counterValue, err := c.client.GetCounter()
if counterValue > c.lastCounter {  // Only send if changed
    contexts.System.Operations.Set(c.State, contexts.EmptyLabels{}, contexts.SystemOperationsValues{
        Total: counterValue,  // Raw counter value
    })
    c.lastCounter = counterValue
}
// Framework calculates rate because dimension has algo: incremental
```

### Dynamic Instances

```go
func (c *Collector) collectQueues() error {
    queues, err := c.client.ListQueues()
    if err != nil {
        return err
    }
    
    for _, queue := range queues {
        labels := contexts.QueueLabels{
            Queue: queue.Name,
            Type:  queue.Type,
        }
        
        contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
            Current: queue.Depth,
            Max:     queue.MaxDepth,
        })
    }
    // Framework handles chart creation/obsoletion automatically
    return nil
}
```

## Framework Features

### 1. Global Labels

Set job-level labels that apply to all charts:

```go
// In Check() or CollectOnce()
version, edition, err := c.client.GetVersion()
if err == nil {
    c.SetGlobalLabel("app_version", version)
    c.SetGlobalLabel("app_edition", edition)
    c.SetGlobalLabel("endpoint", c.config.Endpoint)
}
```

### 2. Automatic Chart Management

- Charts created automatically when data first appears
- Charts marked obsolete after configured iterations without data
- Dimension IDs guaranteed unique across job
- Labels preserved in consistent order

### 3. Exponential Backoff

Framework handles reconnection automatically:

```go
func (c *Collector) CollectOnce() error {
    if err := c.doSomething(); err != nil {
        return framework.ClassifyError(err, framework.ErrorTemporary)  // Framework handles backoff
    }
}
```

Error types:
- `ErrorTemporary` - Retry with backoff
- `ErrorFatal` - Disable module
- `ErrorAuth` - Authentication failure (disable module)

### 4. Protocol-Level Reconnection Management

Reconnection is handled entirely at the protocol level, not by the framework. Each protocol manages its own connection lifecycle, backoff, and static data caching:

```go
// Protocol implementation with reconnection management
type Client struct {
    protocol         *framework.ProtocolClient  // Framework helper
    conn             Connection
    
    // Cached static data (refreshed on reconnection)
    cachedVersion    string
    cachedEdition    string
}

func NewClient(state *framework.CollectorState) *Client {
    return &Client{
        protocol: framework.NewProtocolClient("example", state),
    }
}

func (c *Client) Connect() error {
    // Protocol uses its own backoff
    backoff := c.protocol.GetBackoff()
    
    err := backoff.Retry(func() error {
        conn, err := dial(c.endpoint)
        if err != nil {
            return err
        }
        c.conn = conn
        
        // Mark connection in protocol helper
        c.protocol.MarkConnected()
        
        // Refresh static data on every connection
        c.refreshStaticData()
        return nil
    })
    
    return err
}

func (c *Client) GetVersion() (string, string, error) {
    if !c.IsConnected() {
        return "", "", errors.New("not connected")
    }
    // Return cached data - no network call needed
    return c.cachedVersion, c.cachedEdition, nil
}

func (c *Client) refreshStaticData() {
    // Called internally on every connection
    info, _ := c.conn.GetServerInfo()
    c.cachedVersion = info.Version
    c.cachedEdition = info.Edition
}
```

**Collector remains simple:**
```go
func (c *Collector) CollectOnce() error {
    // Ensure connection (protocol handles reconnection internally)
    if !c.client.IsConnected() {
        if err := c.client.Connect(); err != nil {
            return err
        }
    }
    
    // Get static data - protocol returns cached values
    version, edition, err := c.client.GetVersion()
    if err == nil {
        c.SetGlobalLabel("app_version", version)
        c.SetGlobalLabel("app_edition", edition)
    }
    
    // Continue with collection...
}
```

**Key Benefits:**
- **Independent failure domains** - Each protocol manages its own connection
- **Protocol-specific backoff** - Different retry strategies per protocol
- **Automatic static data refresh** - Protocols cache and refresh on reconnect
- **Collector simplicity** - No awareness of reconnection events needed
- **Multi-protocol support** - Each protocol can fail/reconnect independently

### 5. Collection Intervals (Future)

```yaml
# contexts.yaml
- name: BackupStatus
  min_update_every: 60  # Collect every 60s minimum
```

```go
if c.IsTimeFor("db2.backup_status") {
    c.collectBackupStatus()
}
```

## Protocol Abstraction

### Protocol Package Structure

```go
// protocol/client.go
type Client struct {
    // Connection details
}

func (c *Client) Connect() error { }
func (c *Client) IsConnected() bool { }
func (c *Client) Disconnect() error { }

// Domain-specific methods
func (c *Client) ListQueues() ([]Queue, error) { }
func (c *Client) GetQueueDepth(name string) (int64, error) { }
```

### Multiple Protocols

Modules can use multiple protocols:

```go
// MQ collector using PCF and REST
queues, err := c.pcfClient.ListQueues()      // Via PCF
stats, err := c.restClient.GetStats()        // Via REST API
```

## Development Guidelines

### 1. File Organization

- Keep `collector.go` minimal - just struct and orchestration
- Split collection logic into `collect_*.go` files
- Lifecycle methods in `init.go`
- Protocol code in separate packages

### 2. State Management

- Collectors track raw values only
- Framework handles rate calculations for incremental algorithms
- Use persistent data structures for stateful metrics

### 3. Error Handling

- Return errors to framework for automatic retry
- Don't implement reconnection logic
- Use appropriate error types (temporary vs fatal)

### 4. Configuration

- Embed `framework.Config` for standard options
- Use struct tags for validation and defaults
- Keep configuration minimal and intuitive
- **MANDATORY**: Add descriptive comments to ALL configuration fields (used for auto-generated documentation)

### 5. Testing

- Test with `--dump` mode to verify output
- Check for dimension ID uniqueness
- Verify chart obsoletion works correctly

## Example Module

See `/modules/example/` for a complete working example that demonstrates:
- Type-safe dimension API with compile-time validation
- Dynamic instance management
- Incremental and absolute metrics  
- Global labels
- Protocol abstraction
- Proper file organization

## Common Patterns

### Persistent Instance Data

```go
type ItemData struct {
    Counter int64
    LastSeen time.Time
}

type Collector struct {
    framework.Collector
    itemData map[string]*ItemData  // Persists across collections
}
```

### Batch Processing

```go
const batchSize = 50
for i := 0; i < len(items); i += batchSize {
    end := i + batchSize
    if end > len(items) {
        end = len(items)
    }
    batch := items[i:end]
    // Process batch
}
```

### Conditional Collection

```go
if c.config.CollectAdvanced && c.supportsAdvanced {
    c.collectAdvancedMetrics()
}
```

## Migration from Traditional go.d Modules

Key differences:
1. No manual chart management
2. Use generated contexts instead of hardcoded strings
3. State.Set() instead of mx map
4. Framework handles dimension ID uniqueness
5. Automatic instance lifecycle management

## Automated Documentation Generation

The framework includes a comprehensive automated documentation generator (`docgen`) that eliminates manual documentation maintenance by generating metadata.yaml, config_schema.json, and README.md files directly from your sources of truth.

### How It Works

The docgen tool intelligently extracts information from three key sources:

1. **contexts.yaml** - Your metric definitions (families, contexts, dimensions, labels)
2. **config.go** - Your configuration struct (using AST parsing for 100% accuracy)
3. **module.yaml** - Module-specific metadata (optional, generates defaults if missing)

Then automatically generates all required documentation files:

1. **metadata.yaml** - Complete Netdata marketplace metadata with metric scopes and configuration
2. **config_schema.json** - Valid JSON Schema for web UI configuration forms
3. **README.md** - Comprehensive documentation with metric tables and troubleshooting guides

### Integration with go:generate

Add this directive to any Go file in your module (typically `generate.go`):

```go
//go:generate go run ../../docgen -module=mymodule -contexts=contexts/contexts.yaml -config=config.go -module-info=module.yaml
```

Then run:

```bash
go generate
```

### AST-Powered Configuration Parsing

The tool uses Go's AST (Abstract Syntax Tree) parser to extract configuration fields with 100% accuracy:

- **Comment Extraction**: Reads field descriptions directly from Go comments in config.go
- **Type Detection**: Automatically converts Go types to JSON Schema types (`string`, `int64` → `integer`, `bool` → `boolean`)
- **Tag Parsing**: Extracts YAML/JSON field names from struct tags
- **Required Fields**: Determines optional vs required based on `omitempty` tags
- **Default Value Extraction**: Parses `defaultConfig()` function from init.go to extract actual default values
- **Framework Awareness**: Automatically excludes embedded `framework.Config` fields

Example config.go parsing:
```go
type Config struct {
    framework.Config `yaml:",inline" json:",inline"`  // Automatically excluded
    
    // Connection endpoint URL
    Endpoint string `yaml:"endpoint" json:"endpoint"`                    // Required (no omitempty)
    // Username for authentication  
    Username string `yaml:"username,omitempty" json:"username"`          // Optional 
    // Password for authentication
    Password string `yaml:"password,omitempty" json:"password"`          // Optional, format: password
    // Connection timeout in seconds
    Timeout  int    `yaml:"timeout,omitempty" json:"timeout"`            // Optional integer
    // Enable SSL/TLS encrypted connection
    EnableSSL bool  `yaml:"enable_ssl" json:"enable_ssl"`               // Required boolean
}
```

Generates JSON Schema (titles from Go comments):
```json
{
  "properties": {
    "endpoint": { "type": "string", "title": "Connection endpoint URL" },
    "username": { "type": "string", "title": "Username for authentication" },
    "password": { "type": "string", "format": "password", "title": "Password for authentication" },
    "timeout": { "type": "integer", "title": "Connection timeout in seconds", "minimum": 1 },
    "enable_ssl": { "type": "boolean", "title": "Enable SSL/TLS encrypted connection", "default": false }
  },
  "required": ["endpoint", "enable_ssl"]
}
```

### Metric Scope Generation

The tool intelligently organizes metrics into scopes based on label definitions:

```yaml
# contexts.yaml
QueueManager:
  labels: []  # No labels = global scope
  contexts:
    - name: Status
      context: mq.qmgr.status
      # ... generates global scope metrics

Queue:
  labels: [queue, type]  # Labels = per-instance scope  
  contexts:
    - name: Depth
      context: mq.queue.depth
      # ... generates "Per queue" scope with queue/type labels
```

Generates metadata.yaml scopes:
```yaml
scopes:
  - name: global
    description: These metrics refer to the entire monitored instance.
    labels: []
    metrics:
      - name: mq.qmgr.status
        description: Queue Manager Status
        # ...
  
  - name: queue  
    description: These metrics refer to individual queue instances.
    labels:
      - name: queue
        description: Queue name
      - name: type
        description: Queue type
    metrics:
      - name: mq.queue.depth
        description: Queue Depth
        # ...
```

### Module Metadata Enhancement

Create `module.yaml` for rich module information:

```yaml
name: mq
display_name: IBM MQ (PCF Protocol)
description: |
  Monitors IBM MQ queue managers, queues, channels, and listeners
  using the PCF (Programmable Command Format) protocol.
icon: ibm-mq.svg
categories:
  - data-collection.message-brokers
link: https://www.ibm.com/products/mq
keywords:
  - message queue
  - middleware
  - enterprise messaging
```

### Benefits and Impact

#### 1. Perfect Synchronization (100% Accuracy)
- **No drift**: Documentation always matches actual code
- **Type safety**: AST parsing prevents documentation errors
- **Automatic updates**: Change code → run `go generate` → documentation updates

#### 2. Massive Development Speed Increase
- **80%+ time reduction** in documentation maintenance
- **Zero manual file writing** for metadata, schema, README
- **Focus on code** instead of documentation synchronization
- **Instant documentation** for new modules

#### 3. Error Prevention
- **AST parsing** eliminates manual transcription errors
- **Type-safe schemas** prevent configuration UI bugs
- **Consistent formatting** across all modules
- **Validation** ensures all required fields are documented

#### 4. Framework Integration
- **Framework-aware**: Knows about IBM.D patterns and conventions
- **Base units support**: Documents unit conversions correctly
- **Label handling**: Generates proper scope documentation for dynamic instances
- **Smart defaults**: Understands common patterns (timeout, endpoint, ssl, etc.)

### Example Workflow

1. **Develop your module**:
   ```bash
   # Create contexts.yaml with your metrics
   vim contexts/contexts.yaml
   
   # Create config.go with your configuration  
   vim config.go
   
   # Optionally create module.yaml for metadata
   vim module.yaml
   ```

2. **Generate documentation**:
   ```bash
   go generate  # Runs docgen automatically
   ```

3. **Review generated files**:
   ```bash
   ls *.yaml *.json *.md
   # metadata.yaml ✓
   # config_schema.json ✓  
   # README.md ✓
   ```

4. **Commit everything**:
   ```bash
   git add .
   git commit -m "Add mymodule with auto-generated documentation"
   ```

### Advanced Features

#### Smart Field Descriptions
The tool includes intelligent defaults for common field patterns:
- `endpoint`, `url` → "Connection endpoint/URL"
- `username`, `user` → "Username for authentication"  
- `password`, `pass` → "Password for authentication" (format: password)
- `timeout` → "Timeout in seconds" (minimum: 1)
- `ssl`, `tls` → "Enable SSL/TLS encryption"

#### Complex Type Handling
Supports various Go types:
- `string` → JSON Schema `string`
- `int`, `int64`, `uint` → JSON Schema `integer` 
- `bool` → JSON Schema `boolean`
- `time.Duration` → JSON Schema `integer` (seconds)
- Custom types → JSON Schema `string` (fallback)

#### Validation Rules
Automatically generates appropriate constraints:
- Timeout fields: `minimum: 1, maximum: 300`
- Port fields: `minimum: 1, maximum: 65535`
- Boolean fields: Proper default values
- String fields: Format hints (password, email, etc.)

### File Locations and Structure

```
modules/mymodule/
├── contexts/
│   ├── contexts.yaml           # Source: Metric definitions
│   └── doc.go                  # Contains go:generate directive
├── config.go                   # Source: Configuration struct
├── module.yaml                 # Source: Module metadata (optional)
├── generate.go                 # Contains go:generate directive  
├── metadata.yaml              # Generated ✓
├── config_schema.json         # Generated ✓
└── README.md                  # Generated ✓
```

### Testing the Documentation

After generation, validate the output:

```bash
# Validate JSON schema
jq . config_schema.json > /dev/null && echo "Valid JSON"

# Check metadata structure  
grep -q "plugin_name: ibm.d.plugin" metadata.yaml && echo "Metadata OK"

# Verify README sections
grep -q "## Configuration" README.md && echo "README complete"
```

### Integration with Development Workflow

1. **Module development**:
   - Focus on code (`collector.go`, `config.go`, `contexts.yaml`)
   - Documentation generates automatically

2. **Configuration changes**:
   - Modify `config.go` struct
   - Run `go generate`
   - JSON schema updates automatically

3. **Metric changes**:
   - Update `contexts.yaml`
   - Run `go generate`  
   - Metadata and README update automatically

4. **Pre-commit validation**:
   ```bash
   # Add to git hooks
   go generate && git add *.yaml *.json *.md
   ```

### Troubleshooting

**"Failed to parse Go file"**:
- Ensure `config.go` has valid syntax
- Config struct must be exported (capitalized)
- Check YAML/JSON struct tags

**"No configuration fields found"**:
- Struct must be named exactly `Config`
- Fields must be exported
- Framework fields are automatically excluded

**"Invalid JSON schema generated"**:
- Check field types are supported
- Verify struct tag syntax
- Test with `jq . config_schema.json`

### Future Enhancements

1. **Comment extraction**: Parse Go comments for field descriptions
2. **Validation patterns**: Support regex patterns, enum values
3. **Custom templates**: Module-specific documentation templates  
4. **Multi-language**: Generate docs in multiple languages
5. **Interactive examples**: Generate working configuration examples

See `docgen/README.md` for complete technical details and examples.

## Future Enhancements

1. Protocol observability metrics
2. Collection interval groups  
3. Built-in batching helpers
4. Metric validation and bounds checking
5. Multi-language documentation generation

## Units and Precision

### Base Units Architecture

The framework converts all metrics to **base units** before sending to Netdata. This ensures consistent units throughout the system for APIs, exports, and storage.

### How It Works

1. **Collector provides values** in any unit (bytes, KB, seconds, milliseconds, etc.)
2. **Framework converts to base units** using `mul` and `div` from YAML
3. **Framework applies precision** for float representation
4. **Netdata receives base units** with `Mul=1, Div=precision`

### Process Flow

```go
// 1. Collector provides value in any unit
value := int64(5000)  // 5000 milliseconds

// 2. Framework applies unit conversion (ms → seconds)
converted = (5000 * 1) / 1000 = 5  // Base unit: seconds

// 3. Framework applies precision for display
final = 5 * 1000 = 5000

// 4. Netdata receives 5000 with Mul=1, Div=1000
// Display: 5000 / 1000 = 5.000 seconds
```

**Result**: Netdata database stores base units (seconds), API returns base units, exports are in base units.

### Common Patterns

The base units architecture simplifies unit handling by converting everything to base units (bytes, seconds, percentage, etc.) before sending to Netdata. Here are common patterns:

#### Pattern 1: Time Conversion (Milliseconds → Seconds)
```go
// Collector provides milliseconds
responseTimeMs := int64(45)  // 45 milliseconds
contexts.System.ResponseTime.Set(c.State, contexts.EmptyLabels{}, contexts.SystemResponseTimeValues{
    ResponseTime: responseTimeMs,
})

// YAML - Convert to base unit (seconds)
dimensions:
  - name: response_time
    div: 1000       # Convert ms to seconds: 45ms / 1000 = 0.045s
    precision: 1000 # Preserve 3 decimal places
    
// Framework processing:
// 1. Apply precision: 45 * 1000 = 45000
// 2. Apply unit conversion: (45000 * 1) / 1000 = 45
// 3. Send to Netdata: value=45, Mul=1, Div=1000
// 4. Netdata displays: 45 / 1000 = 0.045 seconds ✓
```

#### Pattern 2: Float Values (Pre-multiply for Precision)
```go
// Collector provides float as integer (pre-multiplied)
cpuUsage := 87.654  // 87.654% CPU usage
contexts.System.CPU.Set(c.State, contexts.EmptyLabels{}, contexts.SystemCPUValues{
    Usage: int64(cpuUsage * 1000),  // 87654 (preserving 3 decimals)
})

// YAML - Value already has precision applied
dimensions:
  - name: usage
    div: 1000      # Restore decimal places: 87654 / 1000 = 87.654
    precision: 1   # No framework precision multiplication
    
// Framework processing:
// 1. Apply precision: 87654 * 1 = 87654
// 2. Apply unit conversion: (87654 * 1) / 1000 = 87
// 3. Send to Netdata: value=87, Mul=1, Div=1
// 4. Netdata displays: 87 / 1 = 87% ✓ (precision preserved through div)
```

#### Pattern 3: Bytes Conversion (Bytes → Base Units)
```go
// Collector provides bytes
memoryBytes := int64(1073741824)  // 1GB in bytes
contexts.System.Memory.Set(c.State, contexts.EmptyLabels{}, contexts.SystemMemoryValues{
    Used: memoryBytes,
})

// YAML - Keep in bytes (already base unit)
dimensions:
  - name: used
    # mul: 1, div: 1 (defaults) - bytes are already base unit
    precision: 1  # No decimal places needed for bytes
    
// Framework processing:
// 1. Apply precision: 1073741824 * 1 = 1073741824
// 2. No unit conversion needed
// 3. Send to Netdata: value=1073741824, Mul=1, Div=1
// 4. Netdata displays: 1073741824 bytes ✓
```

#### Pattern 4: Rate Conversion (Per-minute → Per-second)
```go
// Collector provides requests per minute
requestsPerMin := int64(1200)  // 1200 requests/minute
contexts.System.Requests.Set(c.State, contexts.EmptyLabels{}, contexts.SystemRequestsValues{
    Rate: requestsPerMin,
})

// YAML - Convert to per-second (base unit for rates)
dimensions:
  - name: rate
    div: 60        # Convert /min to /sec: 1200/min / 60 = 20/sec
    precision: 100 # Preserve 2 decimal places
    
// Framework processing:
// 1. Apply precision: 1200 * 100 = 120000
// 2. Apply unit conversion: (120000 * 1) / 60 = 2000
// 3. Send to Netdata: value=2000, Mul=1, Div=100
// 4. Netdata displays: 2000 / 100 = 20.00 requests/sec ✓
```

### Base Units Architecture Summary

1. **Precision first**: Framework applies precision multiplication before unit conversion
2. **Integer-safe conversions**: Precision prevents loss during integer division
3. **Base units everywhere**: All values in Netdata database are in base units
4. **Consistent exports**: APIs and exports use the same base units
5. **Collector flexibility**: Collectors can provide values in any unit
6. **Framework handles conversion**: No unit math in collector code

### Key Rules

1. **Safety first**: All parameters (mul, div, precision) default to 1 - no accidental transformations
2. **Floats require pre-multiplication**: Collectors must multiply floats before casting to int64
3. **Framework handles precision**: Multiplies values by `precision`, sets chart Div to `div * precision`
4. **Clear separation**: Mul for unit conversion, Div for scaling, Precision for framework multiplication
5. **Test the math**: Always verify the final displayed value matches expectations
6. **Document units**: Specify units clearly in chart definitions

## Code Generation

### metricgen Tool

The framework uses code generation to create type-safe metric contexts from YAML definitions:

```bash
//go:generate go run github.com/netdata/netdata/go/plugins/plugin/ibm.d/metricgen
```

### contexts.yaml Format

```yaml
# Define metric classes with their labels and contexts
QueueManager:
  labels: []  # No labels for global metrics
  contexts:
    - name: Status
      context: mq.qmgr.status
      family: qmgr
      title: Queue Manager Status
      units: status
      type: line
      priority: 1000
      dimensions:
        - { name: running, algo: absolute }
        - { name: stopped, algo: absolute }
    
    - name: CPUUsage
      context: mq.qmgr.cpu_usage
      family: qmgr
      title: Queue Manager CPU Usage
      units: percentage
      type: line
      priority: 1001
      dimensions:
        - { name: cpu, algo: absolute, precision: 1000 }  # Float value

Queue:
  labels: [queue, type]  # Labels define instance identity
  contexts:
    - name: Depth
      context: mq.queue.depth
      family: queues
      title: Queue Depth
      units: messages
      type: stacked
      priority: 2000
      dimensions:
        - { name: current, algo: absolute }  # Integer, precision: 1 (default)
        - { name: max, algo: absolute }      # Integer, precision: 1 (default)
```

### Generated Code

The metricgen tool generates:
- Type-safe label structs with validation
- Context definitions with compile-time checked dimensions
- **Values structs** for each context with typed dimension fields
- **Set() methods** that provide compile-time dimension validation
- Instance ID generation methods
- Clean label value functions

Example generated code:
```go
// QueueLabels defines the required labels for Queue contexts
type QueueLabels struct {
    Queue string
    Type  string
}

// QueueDepthValues defines the type-safe values for Queue.Depth context
type QueueDepthValues struct {
    Current int64
    Max     int64
}

// QueueDepthContext provides type-safe operations for Queue.Depth context
type QueueDepthContext struct {
    framework.Context[QueueLabels]
}

// Set provides type-safe dimension setting for Queue.Depth context
func (c QueueDepthContext) Set(state *framework.CollectorState, labels QueueLabels, values QueueDepthValues) {
    state.Set(&c.Context, labels, map[string]int64{
        "current": values.Current,
        "max":     values.Max,
    })
}

// InstanceID generates a unique instance ID
func (l QueueLabels) InstanceID(contextName string) string {
    return contextName + "." + cleanLabelValue(l.Queue) + "_" + cleanLabelValue(l.Type)
}

// Queue contains all metric contexts for Queue
var Queue = struct {
    Depth QueueDepthContext
    // ... other contexts
}{
    Depth: QueueDepthContext{
        Context: framework.Context[QueueLabels]{
            Name:       "mq.queue.depth",
            Family:     "queues",
            // ... full context definition
        },
}
```

## Data Collection Variability

### Protocol Request/Response Patterns

Different protocols have varying data collection characteristics:

1. **Batch Operations**: Some protocols return multiple items per request
   ```go
   // PCF can return details for multiple queues in one call
   queues := pcfClient.GetQueueDetailsBatch(queueNames)
   ```

2. **Individual Queries**: Others require separate requests per item
   ```go
   // Some protocols need individual calls
   for _, queue := range queueList {
       details := client.GetQueueDetails(queue)
   }
   ```

3. **Streaming Data**: Some protocols provide continuous data streams
   ```go
   // WebSphere PMI can stream metrics
   stream := pmiClient.StreamMetrics(filter)
   for metric := range stream {
       // Process streaming data
   }
   ```

### Handling Variability

The framework abstracts these differences:
- Collectors focus on orchestration logic
- Protocols handle communication specifics
- Framework manages state regardless of collection pattern

## Sources of Truth

### Configuration Hierarchy

1. **contexts.yaml**: Source of truth for all metric definitions
   - Context names, families, titles, units
   - Dimension names and algorithms
   - Label definitions and ordering
   - Priority and update intervals

2. **config.go**: Module configuration structure
   - User-configurable options
   - Default values
   - Validation rules

3. **config_schema.json**: Web UI configuration schema
   - Must match config.go exactly
   - Defines UI controls and validation

4. **Stock config file**: Example configuration
   - Must match available options
   - Should include helpful comments

### Documentation Sources

1. **metadata.yaml**: Primary documentation for integrations page
   - Metric descriptions
   - Setup instructions
   - Troubleshooting guide

2. **README.md**: Detailed documentation
   - Should be consistent with metadata.yaml
   - May include additional examples

3. **Code comments**: Implementation details
   - Focus on "why" not "what"
   - Document non-obvious logic

## Mandatory Requirements

### 1. Separation of Concerns

**MANDATORY**: Collectors must NEVER implement protocol logic directly.

✅ **CORRECT Structure**:
```
modules/mq/
├── collector.go      # Orchestration only
├── collect_*.go      # Collection logic
└── protocol/         # Protocol implementation
    └── pcf/
        └── client.go # PCF protocol details
```

❌ **INCORRECT**: Mixing protocol code with collection logic

### 2. Protocol Libraries

**MANDATORY**: Use protocol packages for all external communication.

```go
// CORRECT: Use protocol package
import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"

func (c *Collector) Init() error {
    c.pcfClient = pcf.NewClient(c.config.ConnectionString)
    return c.pcfClient.Connect()
}
```

### 3. Parsing Libraries

**MANDATORY**: Use utils/libraries for parsing responses.

```go
// CORRECT: Use parsing utilities
import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/utils/xmlparse"

response := client.GetData()
parsed := xmlparse.Parse(response)
```

### 4. Directory Structure

**MANDATORY**: Follow the standard module layout exactly.

```
modules/{module_name}/
├── collector.go           # Main struct and orchestration
├── init.go               # Lifecycle methods
├── collect_*.go          # Collection logic split by type
├── config.go             # Configuration
├── module.go             # Registration
├── contexts/
│   ├── contexts.yaml     # Metric definitions
│   ├── doc.go           # go:generate directive
│   └── zz_generated_*.go # Generated code
└── protocol/             # If module-specific protocol
```

### 5. Instance Obsoletion

**MANDATORY**: Let the framework handle instance lifecycle.

```go
// CORRECT: Framework handles obsoletion automatically
for _, item := range items {
    labels := contexts.ItemLabels{Name: item.Name}
    c.State.Set(contexts.Item.Metric, labels, values)
}
// Items not seen for ObsoletionIterations are obsoleted automatically
```

### 6. Data Integrity

**MANDATORY**: Never fake or cache metric values.

```go
// CORRECT: Skip metrics that fail to collect
value, err := client.GetMetric()
if err != nil {
    c.Warningf("failed to collect metric: %v", err)
    // Don't set any value - let gap appear
    return
}
contexts.System.Metric.Set(c.State, contexts.EmptyLabels{}, contexts.SystemMetricValues{
    Value: value,
})
```

### 7. Handling Partial Dimension Collection

**IMPORTANT**: When using the type-safe API, you MUST provide values for ALL dimensions defined in the context. If collection fails for any dimension, skip the entire metric.

```go
// CORRECT: Skip entire metric if any dimension fails
current, err1 := getQueueDepth()
max, err2 := getQueueMaxDepth()

if err1 == nil && err2 == nil {
    // All dimensions collected successfully
    contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
        Current: current,
        Max:     max,
    })
} else {
    // One or more dimensions failed - skip entire metric
    c.Debugf("skipping queue depth metric: current err=%v, max err=%v", err1, err2)
}
```

**BAD PRACTICE**: Bypassing the type-safe API to send partial data:
```go
// ❌ BAD PRACTICE - DO NOT DO THIS
mx := make(map[string]int64)
if current, err := getQueueDepth(); err == nil {
    mx["current"] = current
}
if max, err := getQueueMaxDepth(); err == nil {
    mx["max"] = max
}
// Bypassing type safety defeats the purpose of the framework
c.State.Set(contexts.Queue.Depth, labels, mx)  // DON'T DO THIS
```

**Why this is bad:**
1. **Defeats compile-time safety** - The whole point is to catch errors at compile time
2. **Inconsistent data** - Charts may show partial data which is misleading
3. **Framework philosophy** - "Missing data = data" - gaps are meaningful
4. **Maintenance nightmare** - Future developers lose type safety guarantees

**Remember**: The framework's philosophy is that missing data shows as gaps, which is valuable information about system health.

## Creating a New Collector

### Step-by-Step Guide

#### 1. Create Module Structure

```bash
cd src/go/plugin/ibm.d/modules
mkdir -p mynewmodule/{contexts,protocol}
cd mynewmodule
```

#### 2. Define Metrics (contexts/contexts.yaml)

```yaml
# contexts/contexts.yaml
System:
  labels: []
  contexts:
    - name: Status
      context: mynewmodule.system.status
      family: system
      title: System Status
      units: status
      type: line
      priority: 1000
      dimensions:
        - { name: up, algo: absolute }
        - { name: down, algo: absolute }

Component:
  labels: [name, type]
  contexts:
    - name: Performance
      context: mynewmodule.component.performance
      family: components
      title: Component Performance
      units: operations/s
      type: line
      priority: 2000
      dimensions:
        - { name: requests, algo: incremental }
        - { name: errors, algo: incremental }
```

#### 3. Create Generation File (contexts/doc.go)

```go
// Package contexts contains generated metric contexts
package contexts

//go:generate go run github.com/netdata/netdata/go/plugins/plugin/ibm.d/metricgen
```

#### 4. Generate Context Code

```bash
cd contexts
go generate
```

#### 5. Create Configuration (config.go)

```go
package mynewmodule

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"

type Config struct {
    framework.Config `yaml:",inline" json:",inline"`
    
    Endpoint string `yaml:"endpoint" json:"endpoint"`
    Username string `yaml:"username" json:"username"`
    Password string `yaml:"password" json:"password"`
    
    CollectComponents bool `yaml:"collect_components" json:"collect_components"`
}

```

#### 6. Create Collector Structure (collector.go)

```go
package mynewmodule

import (
    "github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
    "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mynewmodule/contexts"
    "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mynewmodule/protocol"
)

type Collector struct {
    framework.Collector
    config Config
    client *protocol.Client
}

func (c *Collector) CollectOnce() error {
    // Ensure connection
    if !c.client.IsConnected() {
        if err := c.client.Connect(); err != nil {
            return framework.ClassifyError(err, framework.ErrorTemporary)
        }
    }
    
    // Collect metrics
    if err := c.collectSystemStatus(); err != nil {
        return err
    }
    
    if c.config.CollectComponents {
        if err := c.collectComponents(); err != nil {
            c.Warningf("failed to collect components: %v", err)
            // Continue - partial collection is OK
        }
    }
    
    return nil
}
```

#### 7. Implement Lifecycle Methods (init.go)

```go
package mynewmodule

import (
    "context"
    "fmt"
    
    "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mynewmodule/contexts"
    "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mynewmodule/protocol"
)

func (c *Collector) Init(ctx context.Context) error {
    // Call SetDefaults if you have one (optional)
    c.config.SetDefaults()
    
    // Initialize framework
    if err := c.Collector.Init(&c, contexts.GetAllContexts()); err != nil {
        return err
    }
    
    // Create client
    c.client = protocol.NewClient(protocol.Config{
        Endpoint: c.config.Endpoint,
        Username: c.config.Username,
        Password: c.config.Password,
    }, c.State)
    
    return nil
}

func (c *Collector) Check(ctx context.Context) error {
    // Try to connect
    if err := c.client.Connect(); err != nil {
        return fmt.Errorf("connection check failed: %w", err)
    }
    
    // Get version for global labels
    info, err := c.client.GetSystemInfo()
    if err != nil {
        return fmt.Errorf("failed to get system info: %w", err)
    }
    
    // Set global labels
    c.SetGlobalLabel("version", info.Version)
    c.SetGlobalLabel("edition", info.Edition)
    c.SetGlobalLabel("endpoint", c.config.Endpoint)
    
    return nil
}

func (c *Collector) Cleanup(ctx context.Context) {
    if c.client != nil {
        c.client.Disconnect()
    }
}
```

#### 8. Implement Collection Logic (collect_*.go)

```go
// collect_system.go
package mynewmodule

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mynewmodule/contexts"

func (c *Collector) collectSystemStatus() error {
    status, err := c.client.GetSystemStatus()
    if err != nil {
        return err
    }
    
    contexts.System.Status.Set(c.State, contexts.EmptyLabels{}, contexts.SystemStatusValues{
        Up:   status.Up,
        Down: status.Down,
    })
    
    return nil
}

// collect_components.go
package mynewmodule

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mynewmodule/contexts"

func (c *Collector) collectComponents() error {
    components, err := c.client.ListComponents()
    if err != nil {
        return err
    }
    
    for _, comp := range components {
        labels := contexts.ComponentLabels{
            Name: comp.Name,
            Type: comp.Type,
        }
        
        contexts.Component.Performance.Set(c.State, labels, contexts.ComponentPerformanceValues{
            Requests: comp.RequestCount,
            Errors:   comp.ErrorCount,
        })
    }
    
    return nil
}
```

#### 9. Create Module Registration (module.go)

```go
package mynewmodule

import (
    _ "embed"
    
    "github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
    "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func New() *Collector {
    return &Collector{
        Collector: framework.Collector{
            Config: framework.Config{
                UpdateEvery:          1,
                ObsoletionIterations: 60,
            },
        },
    }
}

func (c *Collector) Configuration() any {
    return &c.config
}

func init() {
    module.Register("mynewmodule", module.Creator{
        JobConfigSchema: configSchema,
        Create: func() module.Module {
            return New()
        },
        Config: func() any {
            return &Config{}
        },
    })
}
```

#### 10. Create Configuration Schema (config_schema.json)

```json
{
  "jsonSchema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "title": "MyNewModule collector configuration",
    "type": "object",
    "properties": {
      "update_every": {
        "title": "Update every",
        "type": "integer",
        "minimum": 1,
        "default": 1
      },
      "endpoint": {
        "title": "Endpoint",
        "type": "string",
        "default": "http://localhost:8080"
      },
      "username": {
        "title": "Username",
        "type": "string"
      },
      "password": {
        "title": "Password",
        "type": "string",
        "format": "password"
      },
      "collect_components": {
        "title": "Collect component metrics",
        "type": "boolean",
        "default": true
      }
    },
    "required": ["endpoint"]
  }
}
```

#### 11. Create Stock Configuration

```bash
# Create stock config in proper location
mkdir -p ../../config/ibm.d
cat > ../../config/ibm.d/mynewmodule.conf << 'EOF'
## All available configuration options:
## https://github.com/netdata/netdata/tree/master/src/go/plugin/ibm.d/modules/mynewmodule#configuration

#jobs:
#  - name: local
#    endpoint: http://localhost:8080
#    username: admin
#    password: secret
#    collect_components: true
EOF
```

#### 12. Test Your Collector

```bash
# Build the plugin
cd ~/src/netdata-ktsaou.git
./build-ibm.sh

# Test with dump mode
sudo script -c '/usr/libexec/netdata/plugins.d/ibm.d.plugin -m mynewmodule -d --dump=3s --dump-summary 2>&1' /dev/null

# Check for issues
sudo script -c '/usr/libexec/netdata/plugins.d/ibm.d.plugin -m mynewmodule -d --dump=3s 2>&1' /dev/null | grep ISSUE
```

### Key Points for New Collectors

1. **Start with contexts.yaml** - Define your metrics first
2. **Use code generation** - Let metricgen create type-safe code
3. **Follow the structure** - Use standard directory layout
4. **Leverage the framework** - Don't implement chart management
5. **Keep protocols separate** - Use protocol packages
6. **Test incrementally** - Build and test as you go
7. **Document everything** - Update metadata.yaml and README.md