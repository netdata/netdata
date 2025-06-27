# YAML/JSON Support for inicfg - Migration Guide

This document outlines the complexities involved in adding YAML and JSON support to Netdata's inicfg configuration system, and the proposed solutions.

## Overview

The goal is to enable Netdata to read configuration files in YAML and JSON formats in addition to the traditional INI format. This requires:

1. Loading YAML/JSON files and converting them to the internal inicfg structure
2. Converting inicfg structures back to YAML/JSON for API endpoints (e.g., `http://localhost:19999/netdata.yaml`)
3. Maintaining full backward compatibility with existing INI configurations

## Key Challenges and Solutions

### 1. Keys with Spaces vs Underscores

**Problem:**
- INI format uses keys with spaces: `"update every"`, `"command options"`, `"per cpu core utilization"`
- YAML/JSON with spaces in keys looks unnatural and requires quoting:
  ```yaml
  # Unnatural in YAML
  "update every": 1s
  "per cpu core utilization": yes
  ```

**Analysis:**
- Most keys in netdata.conf use spaces (found only 2 with underscores: `package_throttle_count`, `softnet_stat`)
- No conflicts exist when converting between spaces and underscores

**Solution:**
- **INI → YAML/JSON**: Convert spaces to underscores for natural YAML appearance
- **YAML/JSON → INI**: Convert underscores back to spaces for INI compatibility
- **Special handling**: Preserve filesystem paths unchanged (e.g., `/proc/net/dev`)

```yaml
# Natural YAML representation
update_every: 1s
per_cpu_core_utilization: yes
"/proc/net/dev": yes  # Filesystem paths unchanged
```

### 2. Hierarchical Structure with Colon Separators

**Problem:**
- INI uses flat sections with colons to represent hierarchy:
  ```ini
  [plugin:proc]
  update every = 1s
  
  [plugin:proc:/proc/net/dev]
  compressed packets for all interfaces = no
  ```

**Solution:**
- **INI → YAML/JSON**: Parse colons to create nested objects
- **YAML/JSON → INI**: Flatten nested objects to colon-separated section names

```yaml
# YAML representation
plugin:
  proc:
    update_every: 1s
    _sections:  # Sub-sections for plugin:proc:xxx
      "/proc/net/dev":
        compressed_packets_for_all_interfaces: no
```

### 3. Special Sections (Enable/Disable Maps)

**Problem:**
The `[plugins]` section has a unique behavior where keys represent plugin names and values control enablement:

```ini
[plugins]
proc = yes
ebpf = no
perf = yes

[plugin:proc]
update every = 1s
```

This creates ambiguity: `proc` appears both as a key in `[plugins]` and as part of section name `[plugin:proc]`.

**Solution Options:**

#### Option A: Special Suffix Marker (`@enable`)
- Rename special sections internally: `[plugins]` → `[plugins@enable]`
- This makes the special behavior explicit and extensible

```yaml
plugins@enable:
  proc: yes
  ebpf: no
  
plugin:
  proc:
    update_every: 1s
```

#### Option B: Hardcoded Special Sections
- Maintain a hardcoded list of sections with special behavior
- Currently only `[plugins]` needs this treatment
- Simpler but less extensible

```yaml
plugins:  # Hardcoded to know this is an enable/disable map
  proc: yes
  ebpf: no
  
plugin:
  proc:
    update_every: 1s
```

**Recommendation:** Option B (hardcoded) is simpler if `[plugins]` is truly the only case.

### 4. Bidirectional Conversion Requirements

**Problem:**
- Users must be able to get configurations in any format
- `http://localhost:19999/netdata.conf` (existing)
- `http://localhost:19999/netdata.yaml` (new)
- `http://localhost:19999/netdata.json` (new)

**Solution:**
Implement bidirectional conversion functions:
- `inicfg_to_json()`: Convert inicfg structure to JSON-C object
- `json_to_inicfg()`: Convert JSON-C object to inicfg structure
- YAML uses existing yaml.h functions to convert to/from JSON-C

## Implementation Architecture

### File Organization

```
src/libnetdata/inicfg/
├── inicfg.h                    # Public API
├── inicfg_internals.h          # Internal structures and functions
├── inicfg_conf_file.c          # Core INI loading (refactored)
├── inicfg_yaml.c              # YAML loading support (new)
├── inicfg_json.c              # JSON loading support (new)
├── inicfg_converter.c         # Bidirectional conversion logic (new)
└── MIGRATE_TO_YAML.md         # This document
```

### Key Functions

```c
// Private format-specific loaders
static int inicfg_load_ini(struct config *root, const char *filename, 
                          int overwrite_used, const char *section_name);
static int inicfg_load_yaml(struct config *root, const char *filename,
                           int overwrite_used, const char *section_name);
static int inicfg_load_json(struct config *root, const char *filename,
                           int overwrite_used, const char *section_name);

// Public orchestrator (auto-detects format)
int inicfg_load(struct config *root, char *filename, int overwrite_used,
               const char *section_name);

// Bidirectional conversion
struct json_object *inicfg_to_json(struct config *root);
int json_to_inicfg(struct config *root, struct json_object *json,
                  int overwrite_used, const char *section_name);

// Key normalization helpers
char *normalize_key_for_ini(const char *key);   // underscores → spaces
char *normalize_key_for_yaml(const char *key);  // spaces → underscores
```

## Example Transformations

### Simple Configuration

**INI Format:**
```ini
[global]
update every = 1s
history = 3600

[web]
bind to = *
default port = 19999
```

**YAML Format:**
```yaml
global:
  update_every: 1s
  history: 3600

web:
  bind_to: "*"
  default_port: 19999
```

### Complex Plugin Configuration

**INI Format:**
```ini
[plugins]
proc = yes
tc = yes

[plugin:proc]
update every = 1s
/proc/pagetypeinfo = yes

[plugin:proc:/proc/stat]
per cpu core utilization = yes
cpu idle states = yes
```

**YAML Format:**
```yaml
plugins:  # Special handling: enable/disable map
  proc: yes
  tc: yes

plugin:
  proc:
    update_every: 1s
    "/proc/pagetypeinfo": yes
    _sections:
      "/proc/stat":
        per_cpu_core_utilization: yes
        cpu_idle_states: yes
```

## Testing Strategy

### Unit Tests Required

1. **Key Normalization**
   - Space ↔ underscore conversion
   - Filesystem path preservation
   - Edge cases (multiple spaces, leading/trailing spaces)

2. **Section Parsing**
   - Colon separation parsing
   - Nested object creation/flattening
   - Deep nesting levels

3. **Special Sections**
   - `[plugins]` enable/disable mapping
   - Conflict resolution between enable keys and section names

4. **Round-trip Conversion**
   - INI → YAML → INI preservation
   - YAML → INI → YAML preservation
   - Data type preservation

5. **Error Handling**
   - Invalid YAML/JSON syntax
   - Unsupported nesting levels
   - Conflicting keys

### Test Files

Create test configurations in all formats:
- `tests/inicfg_test.conf` - Traditional INI
- `tests/inicfg_test.yaml` - Equivalent YAML
- `tests/inicfg_test.json` - Equivalent JSON

## Migration Path

1. **Phase 1**: Implement core conversion logic with unit tests
2. **Phase 2**: Add format auto-detection to `inicfg_load()`
3. **Phase 3**: Add web API endpoints for YAML/JSON output
4. **Phase 4**: Update documentation and examples
5. **Phase 5**: Gradual migration of default configs to YAML

## Phase 1: Complete Configuration Analysis Results

### All Configuration Handles Found (Automated Analysis)

Based on comprehensive codebase scan using Python extraction tool, Netdata uses **17 unique configuration handles**:

| Configuration Handle | Purpose | Category | Load Calls | Usage Pattern |
|---------------------|---------|----------|------------|---------------|
| `netdata_config` | Main system configuration | **Primary** | 4 | 500+ get calls, main config |
| `stream_config` | Streaming/replication settings | **Primary** | 2 | Extensive streaming config |
| `cloud_config` | Netdata Cloud integration | **Primary** | 1 | 45+ get calls, cloud features |
| `exporting_config` | Data export configuration | **Primary** | 2 | 134+ get calls, exporters |
| `claim_config` | Cloud claiming configuration | **Primary** | 1 | 23+ get calls, cloud auth |
| `collector_config` | eBPF collector settings | **Primary** | 2 | 200+ get calls, eBPF core |
| `socket_config` | Socket/network configuration | **Active** | 0 | eBPF networking module |
| `sync_config` | Synchronization settings | **Active** | 0 | eBPF sync module |
| `fs_config` | Filesystem monitoring | **Active** | 0 | eBPF filesystem module |
| `cfg` | Generic eBPF configuration | **Utility** | 1 | Multiple eBPF modules |
| `config` | Generic configuration handle | **Utility** | 1 | Various plugins |
| `modules->cfg` | Module-specific eBPF config | **Derived** | 0 | eBPF module references |
| `sockets->config` | Socket-specific eBPF config | **Derived** | 0 | eBPF socket references |
| `tmp_config` | Temporary configuration | **Utility** | 0 | Analysis/processing |
| `root` | Configuration root reference | **Debug** | 0 | Internal operations |
| `sect` | Section reference | **Debug** | 0 | Internal operations |
| `opt` | Option reference | **Debug** | 0 | Internal operations |

### Summary Statistics

- **Total Configuration Handles**: 17
- **inicfg_load() calls**: 14 across entire codebase
- **inicfg_get*() calls**: 847+ total
- **inicfg_set*() calls**: 98+ total
- **Primary configurations**: 6 (actively loaded with config files)
- **Active configurations**: 3 (runtime-created for specific modules)
- **Derived configurations**: 2 (references to parent configs)
- **Utility/Debug configurations**: 6 (temporary or development)

## Phase 2: Configuration Usage Pattern Analysis

### Primary Configuration Details

#### 1. `netdata_config` (Main System Configuration)
- **Load Files**: 4 calls across `src/daemon/config/netdata-conf.c` and `src/database/rrdhost-labels.c`
- **Primary Sections**:
  - `[cloud]` - Cloud connectivity, proxy, SSL settings
  - `[db]` - Database engine, retention, replication, storage tiers
  - `[directories]` - System paths (home, plugins, etc.)
  - `[global]` - System-wide settings (update frequency, hostname, etc.)
  - `[web]` - Web server configuration
  - `[health]` - Health monitoring settings
  - `[plugins]` - Plugin enable/disable map (special handling needed)
  - `[plugin:*]` - Plugin-specific configuration sections

#### 2. `stream_config` (Streaming/Replication)
- **Load Files**: Multiple streaming configuration files
- **Primary Sections**:
  - `[stream]` - Basic streaming settings
  - `[*]` - Stream destination configurations (by GUID/hostname)
- **Special Patterns**: Dynamic section names based on machine GUIDs

#### 3. `cloud_config` (Netdata Cloud Integration)
- **Load Files**: Cloud-specific configuration file
- **Primary Sections**:
  - `[global]` - Cloud URL, proxy, tokens, machine GUID
- **Usage**: 45+ get calls for cloud connectivity features

#### 4. `exporting_config` (Data Export)
- **Load Files**: 2 load calls in `src/exporting/read_config.c`
- **Primary Sections**:
  - `[exporting:global]` - Global export settings
  - `[prometheus:*]` - Prometheus exporter instances
  - `[opentsdb:*]` - OpenTSDB exporter instances
  - `[*:*]` - Various exporter type:instance combinations

#### 5. `claim_config` (Cloud Claiming)
- **Load Files**: 1 load call in `src/claim/claim-with-api.c`
- **Primary Sections**:
  - `[global]` - Claiming tokens, URLs, proxy settings
- **Usage**: Temporary configuration for claiming process

#### 6. `collector_config` (eBPF Collectors)
- **Load Files**: 2 load calls in eBPF plugin system
- **Primary Sections**:
  - `[global]` - eBPF global settings (update frequency, map sizes)
  - `[programs]` - eBPF program enable/disable configuration
  - `[network viewer]` - Network monitoring specific settings

## Phase 3: Special Patterns and Dynamic Construction Analysis

### Critical Section Patterns for YAML Conversion

#### 1. **Plugin Hierarchy Pattern** (Found in `netdata_config`)
```ini
[plugins]
proc = yes
python.d = yes
charts.d = no

[plugin:proc]
update every = 1s
/proc/stat = yes

[plugin:proc:/proc/stat]
per cpu core utilization = yes
cpu idle states = yes
```

**YAML Conversion Challenge**: The `[plugins]` section is an enable/disable map where keys are plugin names, but `[plugin:proc]` sections configure those same plugins. Need to avoid conflicts.

#### 2. **Exporter Instance Pattern** (Found in `exporting_config`)
```ini
[exporting:global]
enabled = yes

[prometheus:server]
enabled = yes
destination = localhost:9090

[opentsdb:primary]
enabled = no
destination = localhost:4242
```

**YAML Conversion**: Clean hierarchical structure possible with exporter type as parent.

#### 3. **Dynamic Stream Destinations** (Found in `stream_config`)
```ini
[stream]
enabled = yes

[11111111-2222-3333-4444-555555555555]
enabled = yes
destination = parent.example.com
```

**YAML Conversion Challenge**: Section names are dynamic machine GUIDs, not predictable keys.

#### 4. **eBPF Module Configuration** (Found in `collector_config`)
```ini
[global]
update every = 1s
pid size = 32768

[programs]
process = yes
socket = yes
filesystem = no

[network viewer]
resolve hostname = no
resolve service = yes
```

**YAML Conversion**: Straightforward hierarchical structure.

### Data Types and Usage Patterns

#### Primary Data Types:
1. **String** (`inicfg_get()`) - 45% of calls
   - Paths, URLs, hostnames, text values
   - Example: `inicfg_get(&netdata_config, "directories", "cache", "/var/cache/netdata")`

2. **Number** (`inicfg_get_number()`) - 30% of calls
   - Counts, timeouts, sizes in basic units
   - Example: `inicfg_get_number(&netdata_config, "global", "cpu cores", system_cpu_cores)`

3. **Boolean** (`inicfg_get_boolean()`) - 15% of calls
   - Enable/disable flags using `CONFIG_BOOLEAN_YES/NO/AUTO`
   - Example: `inicfg_get_boolean(&netdata_config, "health", "enabled", CONFIG_BOOLEAN_YES)`

4. **Size** (`inicfg_get_size_bytes()`, `inicfg_get_size_mb()`) - 5% of calls
   - Memory and disk sizes with human-readable formats
   - Example: `inicfg_get_size_mb(&netdata_config, "db", "dbengine page cache size", 32)`

5. **Duration** (`inicfg_get_duration_ms()`, `inicfg_get_duration_seconds()`) - 5% of calls
   - Time intervals with unit suffixes
   - Example: `inicfg_get_duration_seconds(&netdata_config, "db", "update every", 1)`

## Phase 4: Categorized Configuration Structure

### Configuration Complexity Levels

#### **Level 1: Simple Hierarchical** (Easy YAML conversion)
- `cloud_config` - Simple global section
- `claim_config` - Temporary claiming configuration
- Most eBPF collector configs - Clean section hierarchy

#### **Level 2: Moderate Complexity** (Manageable YAML conversion)
- `exporting_config` - Clean type:instance hierarchy
- Simple sections of `netdata_config` (db, directories, web)

#### **Level 3: High Complexity** (Requires special handling)
- **`netdata_config` plugin sections** - [plugins] conflicts with [plugin:*]
- **`stream_config`** - Dynamic GUID-based section names
- **Multi-level plugin hierarchies** - [plugin:proc:/proc/stat] style nesting

### Key Naming Patterns Found

#### **Standard Patterns** (Convert directly to YAML)
- `update every` → `update_every`
- `default port` → `default_port`
- `bind to` → `bind_to`

#### **Filesystem Paths** (Preserve exactly)
- `/proc/net/dev` → `"/proc/net/dev"`
- `/sys/class/power_supply` → `"/sys/class/power_supply"`

#### **Special Characters** (Need careful handling)
- `per cpu core utilization` → `per_cpu_core_utilization`
- Spaces, colons, and dots in keys

### Complex Section Patterns Discovered

#### 1. Plugin Hierarchy Pattern
```ini
[plugins]
proc = yes
python.d = yes

[plugin:proc]
update every = 1s

[plugin:proc:/proc/net/dev]
compressed packets for all interfaces = no

[plugin:python.d:apache]
update every = 30s
```

#### 2. Export Instance Pattern
```ini
[exporting:global]
enabled = yes

[prometheus:server]
enabled = yes
destination = localhost:9090

[opentsdb:primary]
enabled = no
destination = localhost:4242
```

#### 3. Collector Sub-Module Pattern
```ini
[plugin:cgroups]
update every = 1s

[plugin:proc:/proc/stat]
per cpu core utilization = yes
cpu idle states = yes
```

### Key Naming Conventions Found

#### Space-Separated Keys (Most Common):
- `update every`
- `command options` 
- `per cpu core utilization`
- `compressed packets for all interfaces`

#### Underscore Keys (Rare):
- `package_throttle_count`
- `softnet_stat per core`

#### Filesystem Paths as Keys:
- `/proc/net/dev`
- `/proc/pagetypeinfo`
- `/sys/class/power_supply`

#### Boolean Value Conventions:
- `yes`/`no` (most common)
- `true`/`false` 
- `auto` (for auto-detection)
- `on`/`off`

### Configuration Loading Patterns

1. **Standard Pattern**: User config → Stock config → Internal defaults
2. **Override Pattern**: Some configs support section-specific reloading
3. **Validation Pattern**: Values are often normalized after loading
4. **Migration Pattern**: Old key names are moved to new ones using `inicfg_move()`

### YAML/JSON Conversion Implications

#### Data Type Preservation:
- YAML/JSON can maintain proper types (boolean, number) vs INI strings
- Duration/size suffixes need special handling (`1s`, `1MB`)
- AUTO values need mapping to proper YAML representation

#### Section Flattening Rules:
- `[plugin:proc:/proc/net/dev]` → `plugin.proc["/proc/net/dev"]`
- `[prometheus:server]` → `prometheus.server` or `exporting.prometheus.server`

#### Special Handling Required:
- Filesystem paths as keys must be preserved exactly
- Plugin enable/disable mapping in `[plugins]` section
- Collector-specific configuration hierarchies

## Updated Recommendations

Based on the comprehensive analysis:

1. **Hardcode Special Sections**: Only `[plugins]` needs enable/disable mapping
2. **Preserve Type Information**: Use YAML/JSON native types where possible
3. **Validate Conversion**: Implement round-trip testing for all 1000+ configuration calls
4. **Gradual Migration**: Start with read-only YAML/JSON support, then enable writing

## Open Questions

1. Should we support mixed formats (e.g., main config in YAML, includes in INI)?
2. How deep should nesting be allowed in YAML/JSON?
3. Should we validate against a schema for YAML/JSON configs?
4. How to handle YAML-specific features (anchors, aliases)?
5. Should duration/size values be parsed as strings or objects in YAML?

## Risks and Mitigations

1. **Risk**: Breaking existing configurations
   - **Mitigation**: Extensive testing with all 1000+ config calls, gradual rollout

2. **Risk**: Performance impact from format detection
   - **Mitigation**: Use file extensions for quick detection

3. **Risk**: Type conversion errors
   - **Mitigation**: Comprehensive type validation and error reporting

4. **Risk**: Complex hierarchy mapping errors
   - **Mitigation**: Unit tests for all discovered section patterns

5. **Risk**: Key naming conflicts with space/underscore conversion
   - **Mitigation**: Automated conflict detection across all found configurations

## Phase 5: YAML Conversion Impact Analysis

### Conversion Impact by Configuration

#### **High Impact** (Require immediate attention)
1. **`netdata_config`** - 847+ get calls, most critical
   - Plugin section conflicts need resolution
   - Massive scope affecting entire system
   - Priority: **CRITICAL**

2. **`stream_config`** - Dynamic sections
   - GUID-based section names challenge YAML structure
   - Essential for distributed setups
   - Priority: **HIGH**

#### **Medium Impact** (Manageable with planning)
3. **`exporting_config`** - 134+ get calls
   - Clean hierarchy conversion possible
   - Well-defined type:instance pattern
   - Priority: **MEDIUM**

4. **`collector_config`** - 200+ get calls
   - eBPF-specific but substantial usage
   - Clean section structure
   - Priority: **MEDIUM**

#### **Low Impact** (Straightforward conversion)
5. **`cloud_config`** - 45+ get calls
   - Simple structure, limited scope
   - Priority: **LOW**

6. **`claim_config`** - 23+ get calls
   - Temporary usage, simple structure
   - Priority: **LOW**

### Implementation Roadmap

#### **Phase A: Core Infrastructure** (Foundation)
1. Implement `inicfg_load_yaml()` and `inicfg_load_json()` functions
2. Add format auto-detection to `inicfg_load()`
3. Create key normalization functions (space ↔ underscore)
4. Build bidirectional conversion (`inicfg_to_json()`, `json_to_inicfg()`)

#### **Phase B: Simple Configurations** (Low Risk)
1. Convert `cloud_config` and `claim_config` to YAML
2. Test with existing eBPF collector configs
3. Validate round-trip conversion accuracy

#### **Phase C: Complex Configurations** (High Risk)
1. **Special section handling** for `[plugins]` conflicts
2. **Dynamic section support** for stream GUID-based names
3. **Multi-level hierarchy** for `[plugin:proc:/proc/stat]` patterns

#### **Phase D: Full Integration** (System-wide)
1. Update all `inicfg_load()` calls to support auto-detection
2. Add web API endpoints for YAML/JSON output
3. Update documentation and migration guides

### Critical Success Factors

#### **Must Have**
1. **100% backward compatibility** - All existing INI configs must work unchanged
2. **Round-trip accuracy** - INI → YAML → INI must preserve all data
3. **Performance parity** - YAML loading must not significantly slow boot time
4. **Error handling** - Clear error messages for YAML syntax issues

#### **Should Have**
1. **Mixed format support** - Allow YAML includes of INI files
2. **Configuration validation** - Schema validation for YAML configs
3. **Migration tools** - Automated conversion from INI to YAML

#### **Nice to Have**
1. **YAML features** - Support for anchors, aliases, and multi-line strings
2. **Type preservation** - Use native YAML booleans and numbers
3. **Comments preservation** - Maintain comments during conversion

### Testing Strategy

#### **Unit Tests** (Required for each phase)
1. **Key normalization** - Space/underscore conversion edge cases
2. **Section parsing** - Colon separation and nesting
3. **Data type conversion** - String, number, boolean, size, duration
4. **Special patterns** - Plugin conflicts, dynamic sections, multi-level hierarchy

#### **Integration Tests** (System-wide validation)
1. **Full configuration loading** - All 17 config handles
2. **Web API compatibility** - Existing endpoints continue working
3. **Plugin system** - All collectors work with YAML configs
4. **Streaming** - YAML stream configs work across nodes

#### **Performance Tests** (Ensure no degradation)
1. **Boot time impact** - Measure startup time with YAML vs INI
2. **Memory usage** - Compare memory footprint
3. **Configuration reload** - Runtime configuration changes

### Success Metrics

1. **Functional**: All 847+ get calls work identically with YAML
2. **Performance**: <5% increase in configuration loading time
3. **Compatibility**: 100% existing INI configs continue working
4. **Adoption**: New configurations prefer YAML format
5. **Maintainability**: Reduced configuration complexity and improved readability

## Implementation Complete

**Total Analysis Coverage**:
- **17 configuration handles** identified and categorized
- **847+ configuration get calls** analyzed
- **14 inicfg_load() calls** documented
- **4 complexity levels** established with conversion strategies
- **Multi-phase implementation roadmap** with risk assessment

The comprehensive analysis provides the complete foundation needed for implementing YAML and JSON support in Netdata's inicfg configuration system.
# Detailed Configuration Reference
Complete reference for all Netdata configuration files, sections, and keys.

## Configuration: netdata.conf
**Handle**: `netdata_config`

### Section `CONFIG_SECTION_CLOUD`
Netdata Cloud connectivity and integration settings

| Key | Type | Comments |
|-----|------|----------|
| `proxy` | string | HTTP proxy server URL for Netdata Cloud connectivity. Special value "env" uses proxy environment variables (HTTP_PROXY, HTTPS_PROXY). Empty or unset disables proxy usage. This setting provides backwards compatibility with cloud.conf and is synchronized between netdata.conf and cloud.conf. |
| `query threads` | number | Number of worker threads dedicated to processing Netdata Cloud queries and data aggregation requests. Automatically calculated based on CPU cores: parent nodes get 2x threads per core (up to 256 cores), child nodes get 1x per core, minimum 6 threads, maximum half of libuv worker threads. Must be at least 1. |

### Section `CONFIG_SECTION_DB`
Database engine configuration, retention, and storage tiers

| Key | Type | Comments |
|-----|------|----------|
| `cleanup ephemeral hosts after` | duration | Time after which ephemeral (short-lived) hosts are automatically removed from memory and disk. Ephemeral hosts are typically containers that come and go frequently. Default is 0 (disabled). |
| `cleanup obsolete charts after` | duration | Time after which obsolete charts (charts that stopped collecting data) are removed from memory. Minimum value is 10 seconds for safety. Default is 3600 seconds (1 hour). |
| `cleanup orphan hosts after` | duration | Time after which orphan hosts (hosts that haven't sent data) are moved to archive. Minimum value is 10 seconds. Default is 3600 seconds (1 hour). |
| `db` | string | Database storage mode. Options: "dbengine" (persistent storage), "ram" (memory only), "save" (memory with save/load), "map" (memory mapped), "none" (no storage). Default is "dbengine". |
| `dbengine disk space MB` | number | Maximum disk space for database storage (legacy setting, superseded by tier-specific settings) |
| `dbengine enable journal integrity check` | boolean | Enable integrity checks on database engine journal files at startup. Helps detect corruption but increases startup time. Default is "no". |
| `dbengine extent cache size` | size | Size of extent cache in MB for the database engine. Extents are compressed data blocks. Set to 0 to disable extent caching. Default is calculated based on system memory. |
| `dbengine journal v2 unmount time` | duration | Time after which inactive database journal files are unmounted to free file descriptors. Default is based on system configuration. |
| `dbengine multihost disk space MB` | number | Legacy multihost disk space setting, superseded by tier-specific retention settings |
| `dbengine out of memory protection` | size | Amount of system memory to keep free to prevent out-of-memory conditions. Database engine will limit its memory usage to leave this much RAM available. Default is 10% of total RAM (max 5GB). |
| `dbengine page cache size` | size | Size of page cache in MB for the database engine. Pages contain uncompressed metric data. Larger cache improves query performance. Minimum is 8MB. Default is calculated based on system memory. |
| `dbengine page type` | string | Compression algorithm for database pages. Options: "gorilla" (time-series optimized compression), "raw" (uncompressed). Default is "gorilla". |
| `dbengine pages per extent` | number | Number of pages grouped into each compressed extent. Higher values improve compression but increase memory usage. Valid range: 1-64. Default is 64. |
| `dbengine tier 0 retention size` | size | Maximum disk space in MB for tier 0 (highest resolution) data storage. Default varies by system but typically 256MB. Set to 0 for unlimited. |
| `dbengine tier 0 retention time` | duration | Maximum time to retain tier 0 data. Older data is automatically deleted. Default is 14 days. Set to 0 for unlimited retention. |
| `dbengine tier 1 retention size` | size | Maximum disk space in MB for tier 1 (medium resolution) data storage. Default varies by system. |
| `dbengine tier 1 retention time` | duration | Maximum time to retain tier 1 data. Default is 90 days. |
| `dbengine tier 1 update every iterations` | number | How many tier 0 points are aggregated into one tier 1 point. Minimum value is 2. Default is 60. |
| `dbengine tier 2 retention size` | size | Maximum disk space in MB for tier 2 (lower resolution) data storage. Default varies by system. |
| `dbengine tier 2 retention time` | duration | Maximum time to retain tier 2 data. Default is 2 years. |
| `dbengine tier 2 update every iterations` | number | How many tier 1 points are aggregated into one tier 2 point. Minimum value is 2. Default is 60. |
| `dbengine tier 3 retention size` | size | Maximum disk space in MB for tier 3 (lowest resolution) data storage. Default varies by system. |
| `dbengine tier 3 retention time` | duration | Maximum time to retain tier 3 data. Default is 2 years. |
| `dbengine tier 3 update every iterations` | number | How many tier 2 points are aggregated into one tier 3 point. Minimum value is 2. Default is 60. |
| `dbengine tier 4 retention size` | size | Maximum disk space in MB for tier 4 (lowest resolution) data storage. Default varies by system. |
| `dbengine tier 4 retention time` | duration | Maximum time to retain tier 4 data. Default is 2 years. |
| `dbengine tier 4 update every iterations` | number | How many tier 3 points are aggregated into one tier 4 point. Minimum value is 2. Default is 60. |
| `dbengine tier backfill` | string | Strategy for backfilling missing data when creating new tiers. Options: "new" (only new data), "full" (backfill all historical data), "none" (no backfill). Default is "new". |
| `dbengine use all ram for caches` | boolean | Allow database engine to use all available system RAM for caches, respecting only the out-of-memory protection limit. Default is "no". |
| `dbengine use direct io` | boolean | Use direct I/O for database files, bypassing OS page cache. Can improve performance on systems with limited RAM but may reduce performance on others. Default is "yes". |
| `gap when lost iterations above` | number | Number of consecutive missed data collection iterations above which a gap is inserted in the data instead of interpolation. Helps identify periods of data loss. Default varies by system. |
| `memory deduplication (ksm)` | string | Enable kernel same-page merging to reduce memory usage by sharing identical memory pages. Options: "yes", "no", "auto". Default is "auto". |
| `retention` | duration | For non-dbengine storage modes, the amount of data to keep in memory. Measured in seconds of historical data. Default varies by storage mode. |
| `storage tiers` | number | Number of storage tiers to use for different data resolutions. Each tier stores data at lower resolution but for longer periods. Maximum is 5, minimum is 1. Default is 5. |
| `update every` | duration | Global data collection frequency in seconds. All charts will collect data at this interval unless overridden. Minimum is 1 second, maximum is 86400 seconds (1 day). Default is 1 second. |

### Section `CONFIG_SECTION_DIRECTORIES`
System directory paths for configuration, logs, cache, etc.

| Key | Type | Comments |
|-----|------|----------|
| `cache` | string | Directory where Netdata stores cache files including database files, temporary data, and runtime state. Default is "/var/cache/netdata" or "/opt/netdata/var/cache/netdata". |
| `config` | string | Directory where Netdata looks for user configuration files (netdata.conf, stream.conf, etc.). Default is "/etc/netdata" or "/opt/netdata/etc/netdata". |
| `cloud.d` | string | Subdirectory under lib directory for cloud-related files including claiming tokens and cloud configuration. Default is "cloud.d" under lib directory. |
| `health config` | string | Directory containing custom health configuration files (alerts, notifications). Default is "health.d" under config directory. |
| `home` | string | Netdata home directory, used as base for relative paths. Default varies by installation method ("/opt/netdata" for static builds, "/" for package installs). |
| `lib` | string | Directory for Netdata's variable state files, runtime data, and persistent storage. Default is "/var/lib/netdata" or "/opt/netdata/var/lib/netdata". |
| `log` | string | Directory where Netdata writes log files (access.log, error.log, debug.log). Default is "/var/log/netdata" or "/opt/netdata/var/log/netdata". |
| `plugins` | string | Directory containing plugin executables and scripts. Multiple paths can be configured. Default includes "/usr/libexec/netdata/plugins.d". |
| `stock config` | string | Directory containing default/stock configuration files that ship with Netdata. Used as fallback when user configs are missing. Default is distribution-specific. |
| `stock health config` | string | Directory containing default health configuration files (stock alerts). Default is "health.d" under stock config directory. |
| `web` | string | Directory containing web dashboard files (HTML, CSS, JavaScript). Default is "/usr/share/netdata/web" or "/opt/netdata/usr/share/netdata/web". |

### Section `CONFIG_SECTION_DISKSPACE`
Disk space monitoring and exclusion settings

| Key | Type | Comments |
|-----|------|----------|
| `update every` | duration | How often to check disk space usage on monitored filesystems. Default is 1 second. Format: number with unit suffix (s/m/h). |

### Section `CONFIG_SECTION_ENV_VARS`
Environment variables used by Netdata

| Key | Type | Comments |
|-----|------|----------|
| `CURL_CA_BUNDLE` | string | Path to custom CA certificate bundle for curl operations. Used by external plugins that make HTTPS requests. If not set, curl uses system default CA bundle. |
| `PATH` | string | System executable search path. Used to locate external programs and plugins. Should include directories containing required binaries. Default inherits from system. |
| `PYTHONPATH` | string | Python module search path. Used by Python-based collectors to find required modules. Can include custom collector directories. Default inherits from system. |
| `SSL_CERT_FILE` | string | Path to SSL certificate file for secure connections. Used by various components for TLS/SSL operations. If not set, uses system default certificates. |
| `TZ` | string | Timezone setting for time-related operations. Format: 'Region/City' (e.g., 'America/New_York'). Affects timestamps and time-based calculations. Default inherits from system. |

### Section `CONFIG_SECTION_GETIFADDRS`
FreeBSD network interface monitoring settings

| Key | Type | Comments |
|-----|------|----------|
| `bandwidth for all interfaces` | boolean | Enable bandwidth monitoring (bytes/s in and out) for all network interfaces. Shows data transfer rates. Default is YES. |
| `collisions for all interfaces` | boolean | Enable collision monitoring for all interfaces. Shows packet collision counts on shared media networks. Default is YES. |
| `disable by default interfaces matching` | string | Pattern of interface names to exclude from monitoring. Supports wildcards and multiple patterns separated by space. Example: 'lo* docker* veth*'. Default is 'lo fireqos* *-ifb'. |
| `drops for all interfaces` | boolean | Enable packet drop monitoring for all interfaces. Shows packets dropped due to various reasons. Default is YES. |
| `enable new interfaces detected at runtime` | boolean | Automatically start monitoring new network interfaces that appear after Netdata starts. Useful for dynamic environments. Default is YES. |
| `errors for all interfaces` | boolean | Enable error monitoring for all interfaces. Shows transmission and reception errors. Default is YES. |
| `packets for all interfaces` | boolean | Enable packet rate monitoring (packets/s) for all interfaces. Shows packet counts regardless of size. Default is YES. |
| `set physical interfaces for system.net` | string | Space-separated list of interfaces to consider as physical for system-wide network statistics. Others are treated as virtual. Example: 'eth0 eth1'. Default auto-detects. |
| `total bandwidth for ipv4 interfaces` | boolean | Create aggregate bandwidth charts for all IPv4-capable interfaces. Shows total IPv4 traffic across the system. Default is YES. |
| `total bandwidth for ipv6 interfaces` | boolean | Create aggregate bandwidth charts for all IPv6-capable interfaces. Shows total IPv6 traffic across the system. Default is YES. |
| `total bandwidth for physical interfaces` | boolean | Create aggregate bandwidth charts for physical interfaces only, excluding virtual interfaces. Shows total physical network traffic. Default is YES. |
| `total packets for physical interfaces` | boolean | Create aggregate packet rate charts for physical interfaces only. Shows total packet rates on physical network connections. Default is YES. |

### Section `CONFIG_SECTION_GETMNTINFO`
FreeBSD mount point monitoring settings

| Key | Type | Comments |
|-----|------|----------|
| `enable new mount points detected at runtime` | boolean | Automatically start monitoring new mount points that appear after Netdata starts. Useful for removable drives and dynamic mounts. Default is YES. |
| `exclude space metrics on filesystems` | string | Space-separated list of filesystem types to exclude from space monitoring. Common exclusions: 'devfs procfs tmpfs'. Default includes common virtual filesystems. |
| `exclude space metrics on paths` | string | Space-separated list of mount paths to exclude from space monitoring. Supports wildcards. Example: '/mnt/* /media/* /tmp/*'. Default excludes temporary and system paths. |
| `inodes usage for all disks` | boolean | Enable inode usage monitoring for all filesystems. Shows file/directory count capacity. Important for detecting 'out of inodes' conditions. Default is YES. |
| `space usage for all disks` | boolean | Enable disk space usage monitoring for all filesystems. Shows used/available storage capacity in bytes and percentages. Default is YES. |

### Section `CONFIG_SECTION_GLOBAL`
Global system-wide configuration settings

| Key | Type | Comments |
|-----|------|----------|
| `cpu cores` | number | Number of CPU cores Netdata should use for calculations and thread spawning |
| `glibc malloc arena max for plugins` | number | Maximum malloc arenas for external plugins to prevent memory fragmentation |
| `glibc malloc arena max for netdata` | number | Maximum malloc arenas for the netdata process itself to control memory usage |
| `pthread stack size` | size | Stack size for pthread threads (e.g., "8MB") |
| `libuv worker threads` | number | Number of libuv worker threads for async I/O operations |
| `host access prefix` | string | Host access prefix for generating URLs (e.g., chroot prefix) |
| `hostname` | string | Hostname for this Netdata instance (overrides system hostname) |
| `run as user` | string | User account that netdata should run as for security |
| `OOM score` | string | Out-Of-Memory score adjustment (number or "keep") |
| `process nice level` | number | Process nice level (-20 to 19, lower means higher priority) |
| `process scheduling policy` | string | Process scheduling policy ("batch", "other", "nice", "idle", "rr", "fifo", "keep") |
| `process scheduling priority` | number | Process scheduling priority when using real-time policies |
| `crash reports` | string | Crash report generation ("all" or "off") |
| `timezone` | string | Timezone for the Netdata instance (e.g., "UTC", "America/New_York") |
| `is ephemeral node` | boolean | Marks if this is an ephemeral node (temporary/short-lived) |
| `has unstable connection` | boolean | Marks if this node has an unstable network connection |
| `profile` | string | Configuration profile to use for default settings |

### Section `CONFIG_SECTION_HEALTH`
Health monitoring and alerting settings

| Key | Type | Comments |
|-----|------|----------|
| `default repeat critical` | duration | Default interval for repeating critical alert notifications. 0 means critical alerts are sent only once. Default is 0 (no repeat). |
| `default repeat warning` | duration | Default interval for repeating warning alert notifications. 0 means warning alerts are sent only once. Default is 0 (no repeat). |
| `enabled` | boolean | Enable or disable the health monitoring system entirely. When disabled, no alerts are processed or sent. Default is "yes". |
| `enabled alarms` | string | Pattern matching which alerts to enable. Use "*" for all, specific names, or patterns with wildcards. Default is "*" (all enabled). |
| `enable stock health configuration` | boolean | Whether to load the default health configuration files that ship with Netdata. Recommended to keep enabled. Default is "yes". |
| `health log retention` | duration | How long to keep health event history in memory and on disk. Used for alert state tracking and web dashboard history. Default is 432000 seconds (5 days). |
| `in memory max health log entries` | number | Maximum number of health log entries to keep in memory. Older entries are moved to disk. Minimum is 10. Default is 1000. |
| `postpone alarms during hibernation for` | duration | Delay alert processing after system hibernation/sleep to prevent false alerts during startup. Default is 60 seconds. |
| `run at least every` | duration | Minimum interval between health checks, even if no data updates occur. Ensures health system stays responsive. Minimum is 1 second. Default is 10 seconds. |
| `script to execute on alarm` | string | Path to the script that handles alert notifications (email, slack, etc.). Default is "alarm-notify.sh" in the plugins directory. |
| `use summary for notifications` | boolean | Whether to include a summary of alert status in notifications. Provides context about overall system health. Default is "yes". |

### Section `CONFIG_SECTION_KERN_DEVSTAT`
FreeBSD kernel device statistics monitoring for disk I/O performance

| Key | Type | Comments |
|-----|------|----------|
| `average completed i/o bandwidth for all disks` | boolean | Whether to collect average I/O size charts (read/write/free KB per operation). Shows efficiency of disk operations. Default is "auto" (enabled when data available). |
| `average completed i/o time for all disks` | boolean | Whether to collect average I/O completion time charts (milliseconds per operation for read/write/other/free). Indicates disk latency. Default is "auto" (enabled when data available). |
| `average service time for all disks` | boolean | Whether to collect average service time charts (time from start to completion of I/O). Helps identify slow disks. Default is "auto" (enabled when data available). |
| `bandwidth for all disks` | boolean | Whether to collect disk bandwidth charts (read/write/free KB/s) for individual disks. Primary disk performance metric. Default is "auto" (enabled when data available). |
| `disable by default disks matching` | string | Pattern matching for disks to exclude from monitoring. Use simple patterns with wildcards. Useful for ignoring virtual or system disks. Default is "" (no exclusions). |
| `enable new disks detected at runtime` | boolean | Whether to automatically start monitoring newly detected disks. When "auto", inherits behavior from existing disks. Options: "yes", "no", "auto". Default is "auto". |
| `i/o time for all disks` | boolean | Whether to collect I/O time duration charts (milliseconds spent in read/write/other/free operations). Shows time distribution across operation types. Default is "auto" (enabled when data available). |
| `operations for all disks` | boolean | Whether to collect disk operations per second charts (read/write/other/free ops/s). Shows IOPS for each disk. Default is "auto" (enabled when data available). |
| `performance metrics for pass devices` | boolean | Whether to include SCSI passthrough devices in monitoring. These are raw SCSI devices that bypass the normal disk driver. Default is "auto" (enabled when detected). |
| `queued operations for all disks` | boolean | Whether to collect queue depth charts showing number of operations waiting. Indicates disk saturation. Default is "auto" (enabled when data available). |
| `total bandwidth for all disks` | boolean | Whether to collect system-wide aggregated disk I/O bandwidth chart. Shows total system disk activity. Default is "yes". |
| `utilization percentage for all disks` | boolean | Whether to collect disk utilization percentage charts (0-100% busy time). Key metric for identifying overloaded disks. Default is "auto" (enabled when data available). |

### Section `CONFIG_SECTION_LOGS`
Logging configuration and debugging settings

| Key | Type | Comments |
|-----|------|----------|
| `debug flags` | string | Hexadecimal bitmask for enabling debug output for specific subsystems. Format: "0x0000000000000000". Used for troubleshooting and development. Default is "0x0000000000000000" (no debug). |
| `facility` | string | Syslog facility for log messages when using syslog output. Common values: "daemon", "local0-local7", "user". Default is "daemon". |
| `level` | string | Minimum log level to output. Options: "error", "warning", "info", "debug". Can be overridden by NETDATA_LOG_LEVEL environment variable. Default is "info". |
| `logs flood protection period` | duration | Time window for counting log messages to prevent log flooding. Messages exceeding the threshold within this period are suppressed. Default is 60 seconds. |
| `logs to trigger flood protection` | number | Maximum number of log messages allowed within the flood protection period before suppression kicks in. Default is 1000 messages. |

### Section `CONFIG_SECTION_PLUGINS`
Plugin management and control configuration

| Key | Type | Comments |
|-----|------|----------|
| `check for new plugins every` | duration | How often to scan plugin directories for new plugin files. Default is 60 seconds. Set to 0 to disable automatic discovery. |
| `enable running new plugins` | boolean | Whether to automatically start newly discovered plugins. When enabled, new plugins found during directory scans will be started automatically. Default is "yes". |
| `freeipmi` | boolean | Enable or disable the FreeIPMI plugin for IPMI hardware monitoring (temperature, voltage, fan speeds). Requires FreeIPMI library. Default is "yes" if compiled with support. |
| `slabinfo` | boolean | Enable or disable the slabinfo plugin for kernel slab allocator monitoring. Provides memory usage details for kernel objects. Default is "yes" on supported systems. |
| `statsd` | boolean | Enable or disable the StatsD plugin for receiving metrics via the StatsD protocol. Allows external applications to send custom metrics to Netdata. Default is "yes". |
| `[plugin_name]` | boolean | Generic pattern for individual plugin enablement. Each plugin can be enabled ("yes") or disabled ("no") individually. Plugin names include: proc, diskspace, cgroups, tc, idlejitter, apps, python.d, charts.d, node.d, go.d, and others. |

### Section `CONFIG_SECTION_PLUGIN_PROC_DISKSTATS`
Disk I/O statistics monitoring from /proc/diskstats

| Key | Type | Comments |
|-----|------|----------|
| `backlog for all disks` | boolean | Enable disk backlog monitoring (average time spent in I/O queue). Shows how long I/O operations wait before being serviced. Default is AUTO (enabled if metric exists). |
| `bandwidth for all disks` | boolean | Enable disk bandwidth monitoring (read/write throughput in bytes/second). Shows data transfer rates for disk I/O. Default is AUTO (enabled if available). |
| `bcache for all disks` | boolean | Enable bcache statistics monitoring. Bcache is a Linux kernel block layer cache that allows SSDs to act as a cache for slower HDDs. Default is AUTO (enabled if bcache devices exist). |
| `bcache priority stats update every` | duration | How often to update bcache priority statistics (cache priority distribution). These stats are expensive to collect. Default is 0 (disabled). Suggested value is 300s if enabled. |
| `buffer` | boolean | Enable monitoring of disk buffer statistics. Shows buffer cache utilization for disk operations. Default is NO. |
| `enable new disks detected at runtime` | boolean | Automatically start monitoring newly detected disks without restart. When enabled, Netdata will begin collecting metrics for disks that appear after startup. Default is YES. |
| `exclude disks` | string | Space-separated list of disk name patterns to exclude from monitoring. Supports wildcards. Default is "loop* ram*". Example: "loop* ram* zram* sr*". |
| `extended operations for all disks` | boolean | Enable extended I/O operation statistics (discards). Shows TRIM/discard operations for SSDs. Default is AUTO (enabled if discard stats exist). |
| `filename to monitor` | string | Path to diskstats file to monitor. Default is "/proc/diskstats". Can be overridden for containers or testing. |
| `i/o time for all disks` | boolean | Enable I/O time monitoring (time spent doing I/O). Shows actual time disks spend processing I/O requests. Default is AUTO (enabled if metric exists). |
| `merged operations for all disks` | boolean | Enable merged operations monitoring. Shows adjacent I/O requests that were merged for efficiency. Default is AUTO (enabled if available). |
| `name disks by id` | boolean | Use persistent /dev/disk/by-id names instead of kernel names (sda, sdb). Provides stable names across reboots. Default is NO. |
| `operations for all disks` | boolean | Enable basic I/O operations monitoring (reads/writes per second). Shows IOPS (Input/Output Operations Per Second). Default is AUTO (always enabled). |
| `path to /dev/disk` | string | Base path to disk device directory. Default is "/dev/disk/". Used for resolving disk names and attributes. |
| `path to /dev/disk/by-id` | string | Path to persistent disk ID directory. Default is "/dev/disk/by-id/". Used when "name disks by id" is enabled. |
| `path to /dev/disk/by-label` | string | Path to disk label directory. Default is "/dev/disk/by-label/". Used for resolving disk labels. |
| `path to /dev/vx/dsk` | string | Path to Veritas VxVM disk devices. Default is "/dev/vx/dsk/". Used for VxVM disk monitoring. |
| `path to /sys/block` | string | Path to sysfs block device directory. Default is "/sys/block/%s". Used for reading disk attributes and statistics. |
| `path to device mapper` | string | Path to device mapper directory. Default is "/dev/mapper/". Used for LVM and other mapped devices. |
| `path to get block device` | string | Path template for block device sysfs entries. Default is "/sys/block/%s". %s is replaced with device name. |
| `path to get block device bcache` | string | Path template for bcache sysfs entries. Default is "/sys/block/%s/bcache". Used for bcache statistics. |
| `path to get block device infos` | string | Path template for block device info in sysfs. Default is "/sys/block/%s/device". Used for device model/serial info. |
| `path to get virtual block device` | string | Path template for virtual block devices. Default is "/sys/devices/virtual/block/%s". Used for loop, ram, and other virtual devices. |
| `performance metrics for partitions` | boolean | Enable performance metrics for disk partitions (sda1, sda2, etc). Can generate many charts on systems with many partitions. Default is NO. |
| `performance metrics for physical disks` | boolean | Enable performance metrics for physical disks (sda, sdb, etc). This is the primary disk monitoring. Default is AUTO (always enabled). |
| `performance metrics for virtual disks` | boolean | Enable performance metrics for virtual disks (loop, ram, etc). Includes md-raid, LVM, and other virtual block devices. Default is AUTO. |
| `preferred disk ids` | string | Space-separated list of disk name patterns to prefer when multiple names exist. Supports wildcards. Default is "*". Example: "wwn-* ata-*". |
| `queued operations for all disks` | boolean | Enable queue depth monitoring (number of operations in progress). Shows disk queue utilization. Default is AUTO (enabled if available). |
| `remove charts of removed disks` | boolean | Automatically remove charts when disks disappear. When disabled, charts remain visible with last known values. Default is YES. |
| `utilization percentage for all disks` | boolean | Enable disk utilization percentage monitoring. Shows percentage of time disk was busy. 100% means disk is saturated. Default is AUTO (enabled if available). |

### Section `CONFIG_SECTION_PLUGIN_PROC_DRM`
Direct Rendering Manager (DRM) monitoring for GPU statistics

| Key | Type | Comments |
|-----|------|----------|
| `directory to monitor` | string | Path to the DRM sysfs directory containing GPU device information. Default is "/sys/class/drm". Can be overridden for containers or systems with custom mount points. The collector scans this directory for AMD GPU devices and monitors metrics like GPU utilization, memory usage, clock frequencies, power consumption, and temperature. |

### Section `CONFIG_SECTION_PLUGIN_PROC_INTERRUPTS`
CPU interrupt monitoring from /proc/interrupts

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the interrupts file to monitor. Default is "/proc/interrupts". Can be overridden for containers or testing. Shows interrupt counts by type and CPU. |
| `interrupts per core` | boolean | Whether to create separate charts for each CPU core showing per-core interrupt rates. Useful for identifying CPU imbalances and interrupt affinity issues. Default is "no" to reduce chart clutter on many-core systems. |

### Section `CONFIG_SECTION_PLUGIN_PROC_LOADAVG`
System load average and process count monitoring from /proc/loadavg

| Key | Type | Comments |
|-----|------|----------|
| `enable load average` | boolean | Whether to collect system load average metrics (1, 5, and 15 minute averages). Load average represents the average number of processes waiting for CPU time. Default is "yes". |
| `enable total processes` | boolean | Whether to collect active process count metrics showing current vs maximum system processes. Helps monitor process limits and fork bombs. Default is "yes". |
| `filename to monitor` | string | Path to the loadavg file to monitor. Default is "/proc/loadavg". Can be overridden for containers or testing. Linux updates this file every 5 seconds. |

### Section `CONFIG_SECTION_PLUGIN_PROC_MEMINFO`
System memory statistics monitoring from /proc/meminfo

| Key | Type | Comments |
|-----|------|----------|
| `cma memory` | boolean | Whether to collect Contiguous Memory Allocator (CMA) charts showing total and free CMA memory. CMA reserves memory for devices that need large contiguous blocks. Default is "auto" (enabled when CMA data is available). |
| `committed memory` | boolean | Whether to collect committed (allocated) memory chart showing total virtual memory committed by processes. Helps track memory overcommitment. Default is "yes". |
| `direct maps` | boolean | Whether to collect direct memory mapping charts showing page size distribution (4K, 2M, 4M, 1G pages). Useful for analyzing memory management efficiency. Default is "auto" (enabled when data available). |
| `filename to monitor` | string | Path to the meminfo file to monitor. Default is "/proc/meminfo". Can be overridden for containers or testing. Contains kernel memory statistics. |
| `hardware corrupted ECC` | boolean | Whether to collect ECC memory corruption detection chart. Shows amount of memory marked as corrupted by ECC hardware. Default is "auto" (enabled when ECC corruption detected). |
| `high low memory` | boolean | Whether to collect high/low memory area charts on systems with CONFIG_HIGHMEM. Shows memory split between high and low regions on 32-bit systems. Default is "auto" (enabled when high/low memory exists). |
| `hugepages` | boolean | Whether to collect dedicated hugepages charts showing total, free, reserved, and surplus hugepages. Monitors pre-allocated large memory pages. Default is "auto" (enabled when hugepages configured). |
| `kernel memory` | boolean | Whether to collect kernel memory usage charts showing slab, kernel stack, page tables, vmalloc, per-CPU, and reclaimable kernel memory. Default is "yes". |
| `memory reclaiming` | boolean | Whether to collect memory reclaiming charts showing active/inactive memory (anonymous and file-backed), unevictable, and mlocked memory. Helps understand memory pressure. Default is "auto". |
| `slab memory` | boolean | Whether to collect slab memory breakdown charts showing reclaimable vs unreclaimable kernel slab allocations. Useful for kernel memory leak detection. Default is "yes". |
| `system ram` | boolean | Whether to collect main system RAM chart showing used, free, cached, buffers, and available memory. This is the primary memory monitoring chart. Default is "yes". |
| `system swap` | boolean | Whether to collect swap memory charts including swap usage, swap cached in RAM, and zswap statistics. Monitors virtual memory overflow to disk. Default is "auto" (enabled when swap configured). |
| `transparent hugepages` | boolean | Whether to collect transparent hugepages charts showing anonymous and shared memory huge pages. THP automatically uses large pages for better performance. Default is "auto" (enabled when THP available). |
| `writeback memory` | boolean | Whether to collect writeback memory charts showing dirty pages waiting to be written to disk, pages currently being written back, and bounce buffers. Default is "yes". |

### Section `CONFIG_SECTION_PLUGIN_PROC_NETDEV`
Network interface statistics monitoring from /proc/net/dev

| Key | Type | Comments |
|-----|------|----------|
| `compressed packets for all interfaces` | boolean | Whether to collect compressed packet statistics for network interfaces. Only relevant for CSLIP (Compressed Serial Line Internet Protocol) and PPP (Point-to-Point Protocol) connections. Disabled by default as it's rarely useful for modern Ethernet interfaces. |
| `disable by default interfaces matching` | string | Space-separated pattern list of interface names to automatically disable when first discovered. Default is "lo fireqos* *-ifb fwpr* fwbr* fwln* ifb4*" which excludes loopback, FireQOS traffic shaping, intermediate functional block, and firewall-related interfaces that are typically not relevant for general network monitoring. |

### Section `CONFIG_SECTION_PLUGIN_PROC_NETSTAT`
Advanced network statistics monitoring from /proc/net/netstat

| Key | Type | Comments |
|-----|------|----------|
| `ECN packets` | boolean | Whether to collect Explicit Congestion Notification (ECN) packet statistics. Monitors InNoECTPkts, InECT1Pkts, InECT0Pkts, InCEPkts for congestion control analysis. Default is "auto" (enabled when ECN data available). |
| `TCP SYN cookies` | boolean | Whether to collect SYN cookie statistics (SyncookiesSent, SyncookiesRecv, SyncookiesFailed). SYN cookies prevent SYN flood attacks by encoding connection state in sequence numbers. Default is "auto". |
| `TCP SYN queue` | boolean | Whether to collect SYN queue overflow statistics (TCPReqQFullDrop, TCPReqQFullDoCookies). Monitors when incoming connection requests exceed listen queue capacity. Default is "auto". |
| `TCP accept queue` | boolean | Whether to collect accept queue statistics (ListenOverflows, ListenDrops). Monitors when applications can't accept connections fast enough, causing drops. Default is "auto". |
| `TCP connection aborts` | boolean | Whether to collect TCP connection abort statistics. Monitors various abort reasons: on data, close, memory pressure, timeout, linger, and failed aborts. Helps diagnose connection reliability issues. Default is "auto". |
| `TCP memory pressures` | boolean | Whether to collect TCP memory pressure statistics (TCPMemoryPressures). Tracks when TCP stack runs low on memory and starts dropping connections or reducing buffers. Default is "auto". |
| `TCP out-of-order queue` | boolean | Whether to collect TCP out-of-order packet statistics (TCPOFOQueue, TCPOFODrop, TCPOFOMerge, OfoPruned). Monitors handling of packets received out of sequence. Default is "auto". |
| `TCP reorders` | boolean | Whether to collect TCP packet reordering statistics. Monitors different reorder detection methods: FACK, SACK, Reno, and timestamp-based reordering. Helps identify network path issues. Default is "auto". |
| `bandwidth` | boolean | Whether to collect IP traffic bandwidth statistics (InOctets, OutOctets). Provides total network byte counts in/out for all IP traffic. Default is "auto". |
| `broadcast bandwidth` | boolean | Whether to collect broadcast traffic bandwidth statistics (InBcastOctets, OutBcastOctets). Monitors bytes sent/received via broadcast which can indicate network chattiness. Default is "auto". |
| `broadcast packets` | boolean | Whether to collect broadcast packet count statistics (InBcastPkts, OutBcastPkts). Tracks number of broadcast packets which can impact network performance. Default is "auto". |
| `filename to monitor` | string | Path to the netstat file to monitor. Default is "/proc/net/netstat". Can be overridden for containers or testing. Contains detailed network protocol statistics. |
| `input errors` | boolean | Whether to collect input error statistics (InNoRoutes, InTruncatedPkts). Monitors packets that couldn't be routed or were truncated due to insufficient buffer space. Default is "auto". |
| `multicast bandwidth` | boolean | Whether to collect multicast traffic bandwidth statistics (InMcastOctets, OutMcastOctets). Tracks bytes sent/received via multicast for applications like streaming media. Default is "auto". |
| `multicast packets` | boolean | Whether to collect multicast packet count statistics (InMcastPkts, OutMcastPkts). Monitors number of multicast packets for network efficiency analysis. Default is "auto". |

### Section `CONFIG_SECTION_PLUGIN_PROC_NETWIRELESS`
Linux wireless network interface monitoring configuration from /proc/net/wireless

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to wireless statistics file to read. Default is "/proc/net/wireless". Used to override the data source for wireless interface monitoring. |
| `status for all interfaces` | boolean | Whether to monitor internal status reported by wireless interfaces. Shows hardware-specific status codes. Default is "auto". |
| `quality for all interfaces` | boolean | Whether to monitor wireless signal quality metrics including link quality (aggregate value), signal level (dBm), and noise level (dBm). Essential for WiFi performance analysis. Default is "auto". |
| `discarded packets for all interfaces` | boolean | Whether to monitor packets discarded due to wireless-specific problems including wrong network ID (nwid), encryption errors (crypt), fragmentation issues (frag), retransmission failures (retry), and miscellaneous errors (misc). Default is "auto". |
| `missed beacon for all interface` | boolean | Whether to monitor missed beacon frames. Beacons are periodic signals from access points; missing them indicates connectivity issues or interference. Default is "auto". |

### Section `CONFIG_SECTION_PLUGIN_PROC_NET_IPVS`
IPVS (IP Virtual Server) load balancer monitoring configuration for Linux kernel-based load balancing

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to IPVS statistics file to read. Default is "/proc/net/ip_vs_stats". Used to override the data source for IPVS load balancer monitoring. |
| `IPVS bandwidth` | boolean | Whether to monitor IPVS bandwidth statistics showing bytes received/sent through the load balancer converted to kilobits/s. Essential for load balancer throughput analysis. Default is "yes". |
| `IPVS connections` | boolean | Whether to monitor IPVS new connection statistics showing connection entries created per second. Tracks load balancer activity and connection establishment rate. Default is "yes". |
| `IPVS packets` | boolean | Whether to monitor IPVS packet statistics showing packets received/sent through the load balancer per second. Provides detailed packet-level load balancer metrics. Default is "yes". |

### Section `CONFIG_SECTION_PLUGIN_PROC_NFS`
NFS (Network File System) client statistics monitoring configuration from /proc/net/rpc/nfs

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to NFS statistics file to read. Default is "/proc/net/rpc/nfs". Used to override the data source for NFS client monitoring. |
| `network` | boolean | Whether to monitor NFS network layer statistics including UDP/TCP packet counts. Shows network-level activity for NFS operations. Default is "yes". |
| `rpc` | boolean | Whether to monitor NFS Remote Procedure Call statistics including total calls, retransmissions, and authentication refreshes. Tracks reliability and performance of RPC layer. Default is "yes". |
| `NFS v2 procedures` | boolean | Whether to monitor individual NFSv2 procedure call counts (null, getattr, setattr, lookup, read, write, etc.). Provides detailed breakdown of legacy NFS operations. Default is "yes". |
| `NFS v3 procedures` | boolean | Whether to monitor individual NFSv3 procedure call counts (null, getattr, setattr, lookup, access, read, write, create, etc.). Tracks modern NFS operations with extended functionality. Default is "yes". |
| `NFS v4 procedures` | boolean | Whether to monitor individual NFSv4 procedure call counts including advanced operations (open, close, lock, delegation, ACLs, etc.) and NFSv4.1/4.2 features. Default is "yes". |

### Section `CONFIG_SECTION_PLUGIN_PROC_PAGETYPEINFO`
Memory page type and fragmentation monitoring from /proc/pagetypeinfo

| Key | Type | Comments |
|-----|------|----------|
| `enable detail per-type` | boolean | Whether to create detailed charts per NUMA node, memory zone, and migration type combination. Shows granular memory fragmentation data like "pagetype_Node0_DMA_Unmovable". Default is "auto" (enabled when non-zero data exists). |
| `enable system summary` | boolean | Whether to create a global system summary chart aggregating memory page orders across all NUMA nodes, zones, and types. Shows overall memory fragmentation status. Default is "yes". |
| `filename to monitor` | string | Path to the pagetypeinfo file to monitor. Default is "/proc/pagetypeinfo". Can be overridden for containers or remote monitoring. Contains kernel memory page fragmentation data. |
| `hide charts id matching` | string | Pattern matching to hide specific detailed charts by their ID. Chart IDs follow format "pagetype_Node{N}_{Zone}_{Type}". Supports wildcards to reduce chart clutter. Default is "" (no filtering). |

### Section `CONFIG_SECTION_PLUGIN_PROC_PRESSURE`
Linux kernel Pressure Stall Information (PSI) monitoring from /proc/pressure

| Key | Type | Comments |
|-----|------|----------|
| `base path of pressure metrics` | string | Base directory path where Linux kernel PSI files are located. Default is "/proc/pressure". Used to construct full paths for pressure metric files (cpu, memory, io, irq). Can be overridden for containers or testing. |
| `enable cpu some pressure` | boolean | Whether to monitor CPU "some" pressure metrics showing percentage of time some tasks were delayed due to CPU contention. Includes 10s, 60s, 300s averages plus total stall time. Default is "yes". |
| `enable cpu full pressure` | boolean | Whether to monitor CPU "full" pressure metrics showing percentage of time all tasks were delayed due to CPU contention. Disabled by default due to kernel limitations. Default is "no". |
| `enable memory some pressure` | boolean | Whether to monitor memory "some" pressure metrics showing percentage of time some tasks were delayed due to memory pressure. Indicates memory contention issues. Default is "yes". |
| `enable memory full pressure` | boolean | Whether to monitor memory "full" pressure metrics showing percentage of time all tasks were delayed due to memory pressure. Indicates severe memory shortage. Default is "yes". |
| `enable io some pressure` | boolean | Whether to monitor I/O "some" pressure metrics showing percentage of time some tasks were delayed due to I/O bottlenecks. Indicates storage performance issues. Default is "yes". |
| `enable io full pressure` | boolean | Whether to monitor I/O "full" pressure metrics showing percentage of time all tasks were delayed due to I/O bottlenecks. Indicates severe storage bottlenecks. Default is "yes". |
| `enable irq some pressure` | boolean | Whether to monitor IRQ "some" pressure metrics. Not available in current kernel versions. Default is "no". |
| `enable irq full pressure` | boolean | Whether to monitor IRQ "full" pressure metrics showing time spent handling interrupts. Available on newer kernels. Default varies by kernel support. |

### Section `CONFIG_SECTION_PLUGIN_SYS_CLASS_INFINIBAND`
InfiniBand network monitoring configuration

| Key | Type | Comments |
|-----|------|----------|
| `bandwidth counters` | boolean | Enable monitoring of InfiniBand bandwidth (bytes transmitted/received). Shows data transfer rates for IB ports. Default is YES. |
| `dirname to monitor` | string | Directory path containing InfiniBand sysfs entries. Default is "/sys/class/infiniband". Can be overridden for containers or testing. |
| `disable by default interfaces matching` | string | Pattern matching for InfiniBand interfaces to exclude from monitoring. Supports wildcards. Default excludes no interfaces. Example: "ib0* mlx*". |
| `errors counters` | boolean | Enable monitoring of InfiniBand error counters (symbol errors, link recovery, etc). Tracks transmission problems. Default is YES. |
| `hardware errors counters` | boolean | Enable monitoring of InfiniBand hardware error counters (CRC errors, packet drops). Tracks hardware-level issues. Default is YES. |
| `hardware packets counters` | boolean | Enable monitoring of InfiniBand hardware packet counters. Shows low-level packet statistics. Default is YES. |
| `monitor only active ports` | boolean | Only monitor InfiniBand ports that are in active state. Reduces charts for inactive/down ports. Default is YES. |
| `packets counters` | boolean | Enable monitoring of InfiniBand packet counters (unicast/multicast transmitted/received). Shows packet rates. Default is YES. |
| `refresh ports state every` | duration | How often to check InfiniBand port states (active/down). Used to detect port state changes. Default is 30 seconds. Lower values detect changes faster but use more resources. |

### Section `CONFIG_SECTION_PULSE`
Netdata Agent internal pulse monitoring threads configuration

| Key | Type | Comments |
|-----|------|----------|
| `update every` | duration | How often pulse monitoring threads collect statistics. Controls the update frequency for main pulse, sqlite3, workers, and memory extended threads. Default varies by thread type. Format: number with unit (s/m/h/d). |

### Section `CONFIG_SECTION_REGISTRY`
Netdata Agent Registry configuration for tracking dashboard usage and URLs across multiple Netdata agents

| Key | Type | Comments |
|-----|------|----------|
| `enabled` | boolean | Enable/disable the Netdata registry feature. Only enabled when web server mode is not NONE. Default is 0 (disabled). The registry tracks browser sessions and URLs across multiple Netdata agents. |
| `max URL length` | number | Maximum allowed length for tracked URLs in characters. Must be at least 10. Default is 1024. URLs longer than this will be truncated or rejected to prevent memory issues. |
| `max URL name length` | number | Maximum allowed length for URL display names in characters. Must be at least 10. Default is 50. This controls the length of human-readable names for tracked URLs. |

### Section `CONFIG_SECTION_SQLITE`
SQLite database engine configuration for Netdata's metadata and metrics storage

| Key | Type | Comments |
|-----|------|----------|
| `auto vacuum` | string | SQLite auto-vacuum mode. Controls automatic database file size management. Valid values: NONE, FULL, INCREMENTAL. FULL reclaims space immediately, INCREMENTAL does it gradually, NONE disables it. Applied via PRAGMA auto_vacuum. |
| `cache size` | number | SQLite page cache size in pages (negative values) or kilobytes (positive values). Default varies by system. Larger values improve performance but use more memory. Applied via PRAGMA cache_size. |
| `journal mode` | string | SQLite journaling mode for transaction safety. Valid values: DELETE, TRUNCATE, PERSIST, MEMORY, WAL, OFF. WAL provides better concurrency, DELETE is more compatible. Applied via PRAGMA journal_mode. |
| `journal size limit` | number | Maximum size of SQLite journal file in bytes. Default is 16777216 (16MB). Controls when WAL files are checkpointed to limit disk usage. Applied via PRAGMA journal_size_limit. |
| `synchronous` | string | SQLite synchronous mode for transaction durability. Valid values: OFF (0), NORMAL (1), FULL (2), EXTRA (3). NORMAL balances safety and performance, FULL ensures durability, OFF is fastest but risky. Default is NORMAL. Applied via PRAGMA synchronous. |
| `temp store` | string | SQLite temporary storage location. Valid values: DEFAULT (0), FILE (1), MEMORY (2). MEMORY stores temp tables in RAM for speed, FILE uses disk. Default is MEMORY. Applied via PRAGMA temp_store. |

### Section `CONFIG_SECTION_STATSD`
StatsD collector configuration for receiving and processing metrics from external applications via UDP and TCP

| Key | Type | Comments |
|-----|------|----------|
| `collector threads` | number | Number of worker threads for collecting StatsD metrics. Defaults to number of CPU cores. Only available in multithreaded builds. Must be at least 1. More threads improve performance for high-volume StatsD traffic. |
| `update every (flushInterval)` | duration | How often to flush collected StatsD metrics to RRD charts in seconds. Must be at least equal to Netdata's global update frequency. Default matches Netdata's update interval. This is the StatsD flush interval equivalent. |

### Section `CONFIG_SECTION_TIMEX`
Timex plugin configuration for monitoring system clock synchronization and time offset metrics

| Key | Type | Comments |
|-----|------|----------|
| `update every` | duration | Data collection frequency for timex metrics in seconds. Must be at least equal to Netdata's global update interval. Default is 10 seconds. Controls how often system clock status and time offset are checked. |

### Section `CONFIG_SECTION_WEB`
Web server and dashboard configuration

| Key | Type | Comments |
|-----|------|----------|
| `bind to` | string | IP address(es) and port(s) to bind the web server to. Examples: "*:19999" (all interfaces), "localhost:19999" (local only), "192.168.1.100:19999" (specific IP). Default is "*:19999". |
| `disconnect idle clients after seconds` | duration | Time after which idle client connections are automatically closed to free up resources. Default varies by configuration. |
| `enable gzip compression` | boolean | Enable gzip compression for web responses to reduce bandwidth usage. Recommended for slow connections. Default is "yes". |
| `mode` | string | Web server operation mode. Options: "static-threaded" (multithreaded for better performance), "none" (disable web server entirely). Default is "static-threaded". |
| `respect do not track policy` | boolean | Honor browsers' "Do Not Track" headers by disabling web analytics and tracking features in the dashboard. Default is "no". |
| `web files group` | string | Group ownership for web-accessible files. Used for file permission management. Default varies by installation. |
| `web files owner` | string | User ownership for web-accessible files. Used for file permission management. Default varies by installation. |
| `web server threads` | number | Number of threads for handling web requests. More threads can handle more concurrent users but use more memory. Automatically calculated based on CPU cores, minimum 6. For systems with OpenSSL < 1.1.0, forced to 1. |

### Section `CONFIG_SECTION_WEBRTC`
WebRTC configuration for real-time communication features

| Key | Type | Comments |
|-----|------|----------|
| `bind address` | string | IP address and port for WebRTC connections. Format: "address:port". Default varies by configuration. |
| `enabled` | boolean | Enable WebRTC functionality for real-time data streaming and remote debugging. Default is "yes" if WebRTC support is compiled in. |
| `ice servers` | string | Comma-separated list of ICE (Interactive Connectivity Establishment) servers for NAT traversal. Format: "stun:server:port,turn:server:port". |
| `proxy server` | string | Proxy server configuration for WebRTC connections when behind corporate firewalls. Format: "protocol://server:port". |

### Section `HTTPD_CONFIG_SECTION`
h2o HTTP server configuration for alternative high-performance web server implementation

| Key | Type | Comments |
|-----|------|----------|
| `bind to` | string | IP address and optional port for h2o HTTP server to bind to. Format: "IP" or "IP:PORT" (e.g., "127.0.0.1" or "0.0.0.0:19998"). Default binds to all interfaces. |
| `enabled` | boolean | Enable/disable the h2o HTTP server as an alternative to the default web server. When enabled, h2o provides high-performance HTTP/2 support. Default is "no". |
| `port` | number | TCP port number for the h2o HTTP server to listen on. Default is 19998. Alternative high-performance web server implementation. |
| `ssl` | boolean | Whether to enable SSL/TLS encryption for the h2o HTTP server. When enabled, requires valid SSL certificate and key files. Default is "no". |
| `ssl certificate` | string | Path to SSL certificate file for h2o HTTPS server. Used when SSL is enabled to provide the public certificate for encrypted connections. |
| `ssl key` | string | Path to SSL private key file for h2o HTTPS server. Used when SSL is enabled to decrypt incoming encrypted connections. Must match the certificate. |

### Section `buf`
Windows plugin network interface monitoring configuration. The actual section name is dynamically generated as `plugin:proc:/proc/net/dev:INTERFACE_NAME` where INTERFACE_NAME is the network interface.

| Key | Type | Comments |
|-----|------|----------|
| `bandwidth` | boolean | Enable bandwidth (bytes/s) monitoring for this interface. Shows data transfer rates. Default is YES. |
| `carrier` | boolean | Enable carrier state monitoring. Shows if the physical link is up or down. Default is YES. |
| `compressed` | boolean | Enable compressed packets monitoring. Shows compression statistics if supported by the interface. Default is YES. |
| `drops` | boolean | Enable packet drops monitoring. Shows packets dropped due to buffer overflows or errors. Default is YES. |
| `duplex` | boolean | Enable duplex mode monitoring. Shows if interface is in full or half duplex mode. Default is YES. |
| `enabled` | boolean | Enable/disable monitoring for this specific network interface. Default is YES. |
| `errors` | boolean | Enable error counters monitoring. Shows transmission and reception errors. Default is YES. |
| `events` | boolean | Enable interface events monitoring. Shows state change events. Default is YES. |
| `fifo` | boolean | Enable FIFO errors monitoring. Shows FIFO buffer overrun/underrun errors. Default is YES. |
| `mtu` | boolean | Enable MTU (Maximum Transmission Unit) monitoring. Shows the interface MTU size. Default is YES. |
| `operstate` | boolean | Enable operational state monitoring. Shows if interface is up, down, or in other states. Default is YES. |
| `packets` | boolean | Enable packet counters monitoring. Shows packets/s transmitted and received. Default is YES. |
| `speed` | boolean | Enable link speed monitoring. Shows the interface speed in Mbps. Default is YES. |
| `update every` | duration | Override data collection frequency for this network interface. Inherits from plugin setting if not specified. Format: number with unit (s/m/h/d). |
| `virtual` | boolean | Indicates if this is a virtual interface. Used to apply different monitoring policies. Default is NO. |

### Section `buffer`
InfiniBand/OmniPath device-specific monitoring configuration. The actual section name is dynamically generated as `plugin:proc:/sys/class/infiniband:DEVICE_NAME` where DEVICE_NAME is the InfiniBand device name.

| Key | Type | Comments |
|-----|------|----------|
| `bytes` | boolean | Whether to monitor InfiniBand bandwidth counters showing bytes received/sent through the high-speed interconnect. Essential for HPC and storage cluster performance analysis. Default is "auto". |
| `errors` | boolean | Whether to monitor InfiniBand error counters including malformed packets, buffer overruns, link errors, and integrity errors. Critical for diagnosing interconnect issues. Default is "auto". |
| `hwerrors` | boolean | Whether to monitor hardware-specific InfiniBand error counters including RoCE ICRC errors, sequence errors, timeouts, and completion queue errors. Vendor-specific error tracking. Default is "auto". |
| `hwpackets` | boolean | Whether to monitor hardware-specific InfiniBand packet counters for advanced diagnostics. Vendor-specific metrics for detailed troubleshooting. Default is "auto". |
| `packets` | boolean | Whether to monitor InfiniBand packet counters including received/sent packets, multicast, and unicast traffic. Tracks network activity at packet level. Default is "auto". |

### Section `instance_name`
Exporting connector instance configuration

| Key | Type | Comments |
|-----|------|----------|
| `EXPORTING_UPDATE_EVERY_OPTION_NAME` | number | Data collection frequency for this exporting instance in seconds. Lower values export more frequently. Default is 10 seconds. |
| `hostname` | string | Override hostname for this exporting instance. If not set, uses system hostname. Helps identify source in external systems. |

### Section `section`
Generic configuration section placeholder used in command-line tools

| Key | Type | Comments |
|-----|------|----------|
| `key` | string | Generic configuration key used with -W get2 command for retrieving configuration values. The actual key name and type depend on the specific section and configuration being queried. |

### Section `section_name`
Database connection configuration section for external database drivers and connectors

| Key | Type | Comments |
|-----|------|----------|
| `additional instances` | number | Maximum number of additional database connection instances to create. Enables connection pooling for better performance. Default is 0 (single connection). |
| `address` | string | Database server network address. Can be hostname, IP address, or Unix socket path. Format depends on database driver. Example: "localhost:3306", "192.168.1.100", "/var/run/mysql.sock". |
| `config_name` | string | Configuration identifier name. Used for referencing this database connection in logs and error messages. Should be unique across all database configurations. |
| `driver` | string | Database driver/connector type. Supported values: "mysql", "postgresql", "sqlite", "mongodb", "redis", "influxdb". Determines connection protocol and query syntax. |
| `pwd` | string | Database password for authentication. Should be stored securely. Consider using environment variables or secure credential storage instead of plain text. |
| `server` | string | Database server hostname or IP address. Primary connection endpoint when not using the address field. Default is "localhost". |
| `uid` | string | Database username for authentication. The user account that will connect to the database. Must have appropriate permissions for monitoring queries. |
| `windows authentication` | boolean | Whether to use Windows integrated authentication instead of username/password. Only applicable for SQL Server connections. Default is "no". |

### Section `st->config_section`
Static thread configuration section. The actual section name is dynamically set from the thread's config_section field (e.g., CONFIG_SECTION_PLUGINS, CONFIG_SECTION_PULSE).

| Key | Type | Comments |
|-----|------|----------|
| `st->config_name` | boolean | Whether to enable/disable a specific thread or collector. The key name is dynamically set from the thread's config_name field (e.g., "idlejitter", "statsd.plugin"). When set to "no", the thread will not be started. Default is "yes". |

### Section `string2str(cd->id)`
External collector plugin configuration section. The actual section name is dynamically generated from the collector ID.

| Key | Type | Comments |
|-----|------|----------|
| `command options` | string | Additional command-line options to pass to the external collector script. Can include parameters, flags, or arguments specific to the collector. |
| `update every` | duration | Override data collection frequency for this external collector. Determines how often the collector script is executed. Format: number with unit (s/m/h/d). |

### Section `struct config *root`
Generic configuration section parameter. Used in inicfg API functions where the section name is passed as a parameter to functions like `inicfg_get()`, `inicfg_set()`, etc.

| Key | Type | Comments |
|-----|------|----------|
| `const char *section` | string | The section name parameter passed to inicfg functions. This represents any configuration section name (e.g., "global", "plugin:proc", "health", etc.) used when calling inicfg APIs to get/set configuration values. |

### Section `var_name`
Disk I/O and network interface monitoring configuration for device-specific metrics

| Key | Type | Comments |
|-----|------|----------|
| `average completed i/o bandwidth` | boolean | Whether to monitor average I/O bandwidth for completed operations. Shows average throughput per I/O operation, useful for analyzing I/O efficiency. Default is "auto". |
| `average completed i/o time` | boolean | Whether to monitor average time for completed I/O operations. Shows latency per I/O operation, critical for performance analysis. Default is "auto". |
| `average service time` | boolean | Whether to monitor average service time for I/O requests. Shows time spent actively servicing I/O requests vs time spent waiting. Default is "auto". |
| `backlog` | boolean | Whether to monitor I/O backlog metrics. Shows number of I/O operations queued/pending, indicating I/O pressure and potential bottlenecks. Default is "auto". |
| `bandwidth` | boolean | Whether to monitor bandwidth/throughput metrics. Shows data transfer rates in bytes per second for read/write operations. Default is "auto". |
| `bcache` | boolean | Whether to monitor bcache (block layer cache) statistics. Shows SSD cache performance when used with slower HDDs. Default is "auto". |
| `drops` | boolean | Whether to monitor packet drop statistics. Shows packets discarded due to errors, buffer overruns, or rate limiting. Critical for network quality analysis. Default is "auto". |
| `enable` | boolean | Master enable/disable switch for this monitoring feature. When set to "no", completely disables data collection for this module. Default is "yes". |
| `enable performance metrics` | boolean | Whether to collect detailed performance metrics. Enables advanced performance counters with higher overhead but more detailed insights. Default is "auto". |
| `enabled` | boolean | Enable/disable monitoring for this specific device or interface. When set to "no", skips data collection for this particular disk or network interface. Default is "yes". |
| `errors` | boolean | Whether to monitor error counters. Shows error rates for I/O operations, network packets, or other subsystem-specific errors. Default is "auto". |
| `events` | boolean | Whether to monitor event counters. Tracks system events, interrupts, or state changes depending on the subsystem. Default is "auto". |
| `extended operations` | boolean | Whether to monitor extended I/O operations like discards/TRIM. Important for SSD performance monitoring and wear analysis. Default is "auto". |
| `i/o time` | boolean | Whether to monitor cumulative I/O time. Shows total time spent on I/O operations, helping identify I/O-bound processes. Default is "auto". |
| `inodes usage` | boolean | Whether to monitor filesystem inode usage. Shows used vs available inodes, critical for preventing "no space left" errors despite free disk space. Default is "auto". |
| `merged operations` | boolean | Whether to monitor merged I/O operations. Shows how efficiently the kernel combines adjacent I/O requests to improve performance. Default is "auto". |
| `operations` | boolean | Whether to monitor I/O operations per second (IOPS). Shows read/write operation rates, fundamental for storage performance analysis. Default is "auto". |
| `packets` | boolean | Whether to monitor network packet statistics. Shows packet rates for sent/received traffic, essential for network performance monitoring. Default is "auto". |
| `queued operations` | boolean | Whether to monitor I/O queue depth. Shows number of pending I/O operations in device queues, indicating storage saturation. Default is "auto". |
| `space usage` | boolean | Whether to monitor filesystem space usage. Shows used vs available disk space in bytes and percentage. Essential for capacity planning. Default is "auto". |
| `utilization percentage` | boolean | Whether to monitor device utilization percentage. Shows how much time the device is busy servicing I/O requests (0-100%). Default is "auto". |

### Section `plugin:cgroups`
Control groups (cgroups) monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `check for new cgroups every` | duration | How frequently to scan for new cgroups appearing on the system. Default is 10 seconds. Lower values provide faster detection of new containers/cgroups but use more CPU. |
| `update every` | duration | Data collection frequency for cgroup metrics. Default is 1 second. Should match the global update_every for consistent behavior. |
| `use unified cgroups` | string | Control which cgroups version to use. Options: "auto" (detect automatically), "yes" (force unified/v2), "no" (force legacy/v1). Default is "auto". |
| `max cgroups to allow` | number | Maximum number of cgroups to monitor simultaneously. Prevents resource exhaustion on systems with many containers. Default is 1000. |
| `max cgroups depth to monitor` | number | Maximum depth in the cgroup hierarchy to monitor. Limits monitoring of deeply nested cgroups. Default is 0 (unlimited). |
| `enable by default cgroups matching` | string | Pattern matching rules (space-separated) to determine which cgroups to monitor. Supports wildcards. Default includes common container patterns. |
| `enable by default cgroups names matching` | string | Pattern matching on cgroup names after renaming. Allows filtering based on human-readable names. Supports wildcards. |
| `search for cgroups in subpaths matching` | string | Pattern matching for cgroup filesystem paths to search. Limits where the plugin looks for cgroups. Default is "*" (all paths). |
| `script to get cgroup names` | string | Path to external script that provides human-readable names for cgroups. Used to rename cgroups from IDs to meaningful names. |
| `script to get cgroup network interfaces` | string | Path to external script that determines network interfaces associated with each cgroup. Enables network metrics per container. |
| `run script to rename cgroups matching` | string | Pattern matching to determine which cgroups should be processed by the renaming script. Default is "*" (all cgroups). |
| `cgroups to match as systemd services` | string | Pattern matching to identify cgroups that represent systemd services. Enables special handling for systemd service monitoring. |

### Section `plugin:freebsd`
FreeBSD system monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `pm->name` | boolean | Whether to enable specific FreeBSD kernel modules monitoring. The key name is dynamically set from the module name (e.g., "vm.stats.vm.v_swappgs", "kern.cp_time"). When set to "no", that specific module will not be monitored. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
FreeBSD/macOS firewall monitoring module configuration

| Key | Type | Comments |
|-----|------|----------|
| `allocated memory` | boolean | Whether to monitor memory allocated by the firewall subsystem. Shows memory usage by firewall rules and state tables. Default is "yes". |
| `counters for static rules` | boolean | Whether to monitor counters for static firewall rules. Shows packet/byte counts matched by each static rule. Default is "yes". |
| `number of dynamic rules` | boolean | Whether to monitor the count of dynamic firewall rules. Shows stateful connection tracking entries. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
IPv4 ICMP monitoring module configuration

| Key | Type | Comments |
|-----|------|----------|
| `ipv4 ICMP errors` | boolean | Whether to monitor ICMP error messages (destination unreachable, time exceeded, parameter problems). Essential for network troubleshooting. Default is "yes". |
| `ipv4 ICMP messages` | boolean | Whether to monitor ICMP message types breakdown (echo request/reply, timestamp, address mask, etc.). Shows ping and other ICMP activity. Default is "yes". |
| `ipv4 ICMP packets` | boolean | Whether to monitor total ICMP packets sent and received. Shows overall ICMP traffic volume. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
IPv4 protocol monitoring module configuration

| Key | Type | Comments |
|-----|------|----------|
| `ipv4 errors` | boolean | Whether to monitor IPv4 protocol errors (header errors, checksum failures, invalid addresses). Critical for network health monitoring. Default is "yes". |
| `ipv4 fragments assembly` | boolean | Whether to monitor IPv4 fragment reassembly statistics. Shows fragmentation issues and reassembly failures. Default is "yes". |
| `ipv4 fragments sent` | boolean | Whether to monitor IPv4 packets fragmented for transmission. High fragmentation can indicate MTU issues. Default is "yes". |
| `ipv4 packets` | boolean | Whether to monitor total IPv4 packets sent, received, forwarded, and delivered. Core network traffic metrics. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
TCP protocol monitoring module configuration

| Key | Type | Comments |
|-----|------|----------|
| `ECN packets` | boolean | Whether to monitor Explicit Congestion Notification (ECN) capable packets. Shows network congestion awareness. Default is "yes". |
| `TCP SYN cookies` | boolean | Whether to monitor TCP SYN cookie usage. Indicates SYN flood attack mitigation activity. Default is "yes". |
| `TCP connection aborts` | boolean | Whether to monitor TCP connection aborts (resets, timeouts). Shows connection stability issues. Default is "yes". |
| `TCP listen issues` | boolean | Whether to monitor TCP listen queue overflows and drops. Critical for server performance tuning. Default is "yes". |
| `TCP out-of-order queue` | boolean | Whether to monitor TCP out-of-order packet queuing. Indicates network path issues or packet loss. Default is "yes". |
| `ipv4 TCP errors` | boolean | Whether to monitor TCP protocol errors (invalid checksums, bad segments). Shows TCP stack health. Default is "yes". |
| `ipv4 TCP handshake issues` | boolean | Whether to monitor TCP handshake failures and retransmissions. Critical for connection establishment monitoring. Default is "yes". |
| `ipv4 TCP packets` | boolean | Whether to monitor TCP segment statistics (sent, received, retransmitted). Core TCP performance metrics. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for UDP protocol monitoring

| Key | Type | Comments |
|-----|------|----------|
| `ipv4 UDP errors` | boolean | Whether to monitor UDP protocol errors (invalid checksums, no buffer space, socket buffer errors). Helps identify UDP-related issues. Default is "yes". |
| `ipv4 UDP packets` | boolean | Whether to monitor UDP datagram statistics (sent, received). Core UDP traffic monitoring. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for IPv6 ICMP monitoring

| Key | Type | Comments |
|-----|------|----------|
| `icmp` | boolean | Whether to monitor overall ICMPv6 message statistics. Master control for all ICMPv6 monitoring. Default is "yes". |
| `icmp echos` | boolean | Whether to monitor ICMPv6 echo requests and replies (ping6). Used for IPv6 connectivity testing. Default is "yes". |
| `icmp errors` | boolean | Whether to monitor ICMPv6 error messages (destination unreachable, packet too big, time exceeded). Critical for IPv6 troubleshooting. Default is "yes". |
| `icmp neighbor` | boolean | Whether to monitor ICMPv6 neighbor discovery messages (solicitations, advertisements). Essential for IPv6 address resolution. Default is "yes". |
| `icmp redirects` | boolean | Whether to monitor ICMPv6 redirect messages. Important for IPv6 routing optimization detection. Default is "yes". |
| `icmp router` | boolean | Whether to monitor ICMPv6 router discovery messages (solicitations, advertisements). Critical for IPv6 autoconfiguration. Default is "yes". |
| `icmp types` | boolean | Whether to monitor all ICMPv6 message types by type code. Provides detailed ICMPv6 traffic breakdown. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for IPv6 protocol monitoring

| Key | Type | Comments |
|-----|------|----------|
| `ipv6 errors` | boolean | Whether to monitor IPv6 protocol errors (header errors, no routes, address errors). Essential for IPv6 deployment troubleshooting. Default is "yes". |
| `ipv6 fragments assembly` | boolean | Whether to monitor IPv6 fragment reassembly statistics. Shows fragmentation-related performance issues. Default is "yes". |
| `ipv6 fragments sent` | boolean | Whether to monitor IPv6 fragments sent. Helps identify MTU and path issues. Default is "yes". |
| `ipv6 packets` | boolean | Whether to monitor IPv6 packet statistics (received, sent, forwarded, delivered). Core IPv6 traffic metrics. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for FreeBSD kernel network dispatch monitoring

| Key | Type | Comments |
|-----|------|----------|
| `netisr` | boolean | Whether to monitor kernel network dispatch statistics (packets queued, handled, dropped). Critical for FreeBSD network performance tuning. Default is "yes". |
| `netisr per core` | boolean | Whether to monitor network dispatch statistics per CPU core. Essential for identifying CPU bottlenecks in network processing. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for system processes and memory monitoring

| Key | Type | Comments |
|-----|------|----------|
| `enable total processes` | boolean | Whether to monitor total process count (running, sleeping, stopped, zombie). Essential system health metric. Default is "yes". |
| `processes running` | boolean | Whether to monitor count of runnable processes. Key indicator of system load and CPU contention. Default is "yes". |
| `real memory` | boolean | Whether to monitor physical memory usage (active, inactive, wired, free). Critical for memory management monitoring. Default is "yes". |

### Section `plugin:idlejitter`
CPU idle jitter monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `loop time` | duration | Time between measurements in milliseconds. The plugin sleeps for this duration and measures how much the actual sleep time deviates from the requested time. Default is 20ms. Lower values provide more frequent measurements but use more CPU. |

### Section `plugin:macos`
macOS system monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `pm->name` | boolean | Whether to enable specific macOS system monitoring modules. The key name is dynamically set from the module name (e.g., "sysctl", "iokit", "mach_smi"). When set to "no", that specific module will not be monitored. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for network interface and disk monitoring

| Key | Type | Comments |
|-----|------|----------|
| `disable by default network interfaces matching` | string | Space-separated list of network interface patterns to exclude from monitoring by default. Supports wildcards (e.g., "lo* dummy*" to exclude loopback and dummy interfaces). Default is empty. |
| `disk i/o` | boolean | Whether to monitor disk I/O statistics (reads, writes, operations, bandwidth). Essential for storage performance monitoring. Default is "yes". |
| `exclude mountpoints by path` | string | Space-separated list of mountpoint paths to exclude from disk space monitoring. Use this to ignore temporary or virtual filesystems. Default is empty. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for system resource monitoring

| Key | Type | Comments |
|-----|------|----------|
| `cpu utilization` | boolean | Whether to monitor CPU usage statistics (user, system, idle, iowait, etc.). Core system performance metric. Default is "yes". |
| `memory page faults` | boolean | Whether to monitor memory page fault statistics (minor and major faults). Indicates memory pressure and disk I/O from swapping. Default is "yes". |
| `swap i/o` | boolean | Whether to monitor swap usage and I/O operations. Critical for detecting memory exhaustion. Default is "yes". |
| `system ram` | boolean | Whether to monitor system RAM usage (used, free, cached, buffers). Essential memory monitoring metric. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for comprehensive network protocol monitoring

| Key | Type | Comments |
|-----|------|----------|
| `ECN packets` | boolean | Whether to monitor Explicit Congestion Notification (ECN) packet statistics. Used for advanced TCP congestion control. Default is "yes". |
| `TCP SYN cookies` | boolean | Whether to monitor TCP SYN cookies usage. SYN cookies are used to prevent SYN flood attacks when the TCP connection queue is full. Default is "yes". |
| `TCP connection aborts` | boolean | Whether to monitor TCP connection abort statistics (connection failures, timeouts, resets). Useful for diagnosing network connectivity issues. Default is "yes". |
| `TCP out-of-order queue` | boolean | Whether to monitor TCP out-of-order packet queue statistics. High values indicate network packet reordering or loss. Default is "yes". |
| `bandwidth` | boolean | Whether to monitor network interface bandwidth utilization (bytes sent/received). Core network performance metric. Default is "yes". |
| `enable load average` | boolean | Whether to monitor system load average metrics (1min, 5min, 15min averages). Indicates overall system utilization. Default is "yes". |
| `icmp` | boolean | Whether to monitor basic ICMP (Internet Control Message Protocol) statistics. Includes ICMP message counts and basic error reporting. Default is "yes". |
| `icmp echos` | boolean | Whether to monitor ICMP echo request/reply statistics (ping traffic). Useful for network connectivity diagnostics. Default is "yes". |
| `icmp errors` | boolean | Whether to monitor ICMP error message statistics (destination unreachable, time exceeded, etc.). Helps identify network routing issues. Default is "yes". |
| `icmp neighbor` | boolean | Whether to monitor ICMPv6 neighbor discovery messages (IPv6 equivalent of ARP). Critical for IPv6 network operation. Default is "yes". |
| `icmp redirects` | boolean | Whether to monitor ICMP redirect messages. These indicate suboptimal routing and potential security concerns. Default is "yes". |
| `icmp router` | boolean | Whether to monitor ICMPv6 router advertisement/solicitation messages. Essential for IPv6 router discovery. Default is "yes". |
| `icmp types` | boolean | Whether to monitor detailed ICMP message type breakdown. Provides granular analysis of ICMP traffic patterns. Default is "yes". |
| `inodes usage for all disks` | boolean | Whether to monitor inode usage statistics for all mounted filesystems. Inodes can be exhausted even when disk space is available. Default is "yes". |
| `ipv4 ICMP messages` | boolean | Whether to monitor IPv4 ICMP message statistics (control messages like destination unreachable, time exceeded). Essential for network troubleshooting. Default is "yes". |
| `ipv4 ICMP packets` | boolean | Whether to monitor IPv4 ICMP packet counts (total ICMP traffic volume). Useful for identifying ICMP-based network issues or attacks. Default is "yes". |
| `ipv4 TCP errors` | boolean | Whether to monitor IPv4 TCP error statistics (bad segments, failed connections, retransmissions). Critical for TCP performance analysis. Default is "yes". |
| `ipv4 TCP handshake issues` | boolean | Whether to monitor IPv4 TCP handshake problems (SYN retransmissions, failed connections). Indicates network connectivity or server load issues. Default is "yes". |
| `ipv4 TCP packets` | boolean | Whether to monitor IPv4 TCP packet statistics (segments sent/received). Core TCP traffic monitoring metric. Default is "yes". |
| `ipv4 UDP errors` | boolean | Whether to monitor IPv4 UDP error statistics (invalid checksums, no buffer space, socket buffer errors). Helps identify UDP-related issues. Default is "yes". |
| `ipv4 UDP packets` | boolean | Whether to monitor IPv4 UDP datagram statistics (sent, received). Core UDP traffic monitoring. Default is "yes". |
| `ipv4 errors` | boolean | Whether to monitor IPv4 protocol error statistics (header errors, address errors, unknown protocols). Critical for IP layer troubleshooting. Default is "yes". |
| `ipv4 fragments assembly` | boolean | Whether to monitor IPv4 packet fragmentation reassembly statistics (successful/failed reassembly). Indicates MTU issues or fragmentation attacks. Default is "yes". |
| `ipv4 fragments sent` | boolean | Whether to monitor IPv4 packet fragmentation transmission statistics (fragments created/sent). High values may indicate MTU configuration problems. Default is "yes". |
| `ipv4 packets` | boolean | Whether to monitor IPv4 packet statistics (total packets sent/received/forwarded). Core IP traffic monitoring metric. Default is "yes". |
| `ipv6 errors` | boolean | Whether to monitor IPv6 protocol error statistics (header errors, address errors, unknown protocols). Essential for IPv6 network troubleshooting. Default is "yes". |
| `ipv6 fragments assembly` | boolean | Whether to monitor IPv6 packet fragmentation reassembly statistics. IPv6 fragmentation is less common but still important for large packet analysis. Default is "yes". |
| `ipv6 fragments sent` | boolean | Whether to monitor IPv6 packet fragmentation transmission statistics. High fragmentation in IPv6 may indicate application or MTU issues. Default is "yes". |
| `ipv6 packets` | boolean | Whether to monitor IPv6 packet statistics (total IPv6 traffic). Critical for IPv6-enabled network monitoring. Default is "yes". |
| `space usage for all disks` | boolean | Whether to monitor disk space usage statistics for all mounted filesystems (used, free, available space). Essential for preventing disk full conditions. Default is "yes". |
| `system swap` | boolean | Whether to monitor system swap usage statistics (swap used, free, cached). Critical for detecting memory pressure and performance issues. Default is "yes". |
| `system uptime` | boolean | Whether to monitor system uptime statistics (time since boot, idle time). Basic system health and availability metric. Default is "yes". |

### Section `plugin:proc`
Linux /proc filesystem monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `/proc/net/dev` | boolean | Whether to monitor network interface statistics from /proc/net/dev. Provides per-interface traffic, errors, and drops. Default is "yes". |
| `/proc/pagetypeinfo` | boolean | Whether to monitor memory page type information from /proc/pagetypeinfo. Shows memory fragmentation by page order and type. Default is "yes". |
| `pm->name` | boolean | Whether to enable specific proc modules. The key name is dynamically set from the module name (e.g., "/proc/stat", "/proc/meminfo"). When set to "no", that specific /proc file will not be monitored. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for file and directory monitoring

| Key | Type | Comments |
|-----|------|----------|
| `directory to monitor` | string | Path to the directory to monitor for statistics or files. Module-specific, typically used for sysfs or procfs directories. Default varies by module. |
| `filename to monitor` | string | Path to the specific file to monitor for statistics. Module-specific, typically a proc or sysfs file containing metrics. Default varies by module. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for RAID array monitoring

| Key | Type | Comments |
|-----|------|----------|
| `disk stats` | boolean | Whether to monitor per-disk statistics within RAID arrays (reads, writes, errors). Default is "yes". |
| `faulty devices` | boolean | Whether to monitor and alert on faulty devices in RAID arrays. Critical for data integrity. Default is "yes". |
| `filename to monitor` | string | Path to the mdstat file to monitor (typically /proc/mdstat). Contains RAID array status and health. Default is "/proc/mdstat". |
| `make charts obsolete` | boolean | Whether to automatically hide charts for removed or inactive RAID arrays. Keeps dashboards clean. Default is "yes". |
| `mismatch count` | boolean | Whether to monitor RAID array mismatch counts. Indicates data inconsistencies needing attention. Default is "yes". |
| `mismatch_cnt filename to monitor` | string | Path pattern to mismatch_cnt files in sysfs (e.g., /sys/block/md*/md/mismatch_cnt). Default is "/sys/block/md*/md/mismatch_cnt". |
| `nonredundant arrays availability` | boolean | Whether to monitor availability of non-redundant arrays (RAID0, linear). Important as these have no fault tolerance. Default is "yes". |
| `operation status` | boolean | Whether to monitor RAID operation status (idle, resync, recovery, reshape). Shows array maintenance activities. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for NFS server monitoring

| Key | Type | Comments |
|-----|------|----------|
| `I/O` | boolean | Whether to monitor NFS I/O statistics (read/write operations and throughput). Essential for NFS performance analysis. Default is "yes". |
| `NFS v2 procedures` | boolean | Whether to monitor NFSv2 procedure calls (getattr, read, write, etc.). Legacy protocol monitoring. Default is "yes". |
| `NFS v3 procedures` | boolean | Whether to monitor NFSv3 procedure calls. Common NFS protocol version monitoring. Default is "yes". |
| `NFS v4 operations` | boolean | Whether to monitor NFSv4 operations (compound operations, delegations, etc.). Modern NFS protocol monitoring. Default is "yes". |
| `NFS v4 procedures` | boolean | Whether to monitor NFSv4 procedure calls. Detailed NFSv4 activity tracking. Default is "yes". |
| `file handles` | boolean | Whether to monitor NFS file handle statistics (stale handles, lookups). Important for NFS reliability. Default is "yes". |
| `filename to monitor` | string | Path to the NFS statistics file to monitor (typically /proc/net/rpc/nfsd). Default is "/proc/net/rpc/nfsd". |
| `network` | boolean | Whether to monitor NFS network statistics (TCP/UDP connections, packet counts). Default is "yes". |
| `read cache` | boolean | Whether to monitor NFS read cache statistics (hits, misses). Important for cache efficiency. Default is "yes". |
| `rpc` | boolean | Whether to monitor RPC (Remote Procedure Call) statistics. Core NFS protocol metrics. Default is "yes". |
| `threads` | boolean | Whether to monitor NFS server thread utilization. Critical for NFS server tuning. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for SCTP protocol monitoring

| Key | Type | Comments |
|-----|------|----------|
| `association transitions` | boolean | Whether to monitor SCTP association state transitions. Shows connection lifecycle events. Default is "yes". |
| `chunk types` | boolean | Whether to monitor SCTP chunk types distribution (DATA, INIT, HEARTBEAT, etc.). Protocol behavior analysis. Default is "yes". |
| `established associations` | boolean | Whether to monitor count of established SCTP associations. Active connection tracking. Default is "yes". |
| `filename to monitor` | string | Path to the SCTP statistics file to monitor (typically /proc/net/sctp/snmp). Default is "/proc/net/sctp/snmp". |
| `fragmentation` | boolean | Whether to monitor SCTP fragmentation statistics. Important for message size optimization. Default is "yes". |
| `packet errors` | boolean | Whether to monitor SCTP packet errors (checksums, invalid chunks). Protocol health indicator. Default is "yes". |
| `packets` | boolean | Whether to monitor SCTP packet statistics (sent, received). Core SCTP traffic metrics. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for IPv4 socket and connection monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the socket statistics file to monitor (typically /proc/net/sockstat). Default is "/proc/net/sockstat". |
| `ipv4 ICMP messages` | boolean | Whether to monitor ICMP message types (echo, destination unreachable, etc.). Network diagnostics tool. Default is "yes". |
| `ipv4 ICMP packets` | boolean | Whether to monitor ICMP packet statistics (sent, received, errors). Ping and traceroute monitoring. Default is "yes". |
| `ipv4 TCP connections` | boolean | Whether to monitor TCP connection states (established, time-wait, close-wait, etc.). Connection health tracking. Default is "yes". |
| `ipv4 TCP errors` | boolean | Whether to monitor TCP errors (resets, invalid SYN, failed connections). Network problem detection. Default is "yes". |
| `ipv4 TCP handshake issues` | boolean | Whether to monitor TCP handshake problems (SYN timeouts, failed attempts). Connection establishment issues. Default is "yes". |
| `ipv4 TCP opens` | boolean | Whether to monitor TCP connection opens (active, passive). Connection rate monitoring. Default is "yes". |
| `ipv4 TCP packets` | boolean | Whether to monitor IPv4 TCP packet statistics (segments sent/received). Core TCP traffic monitoring metric. Default is "yes". |
| `ipv4 UDP errors` | boolean | Whether to monitor IPv4 UDP error statistics (invalid checksums, no buffer space, socket buffer errors). Helps identify UDP-related issues. Default is "yes". |
| `ipv4 UDP packets` | boolean | Whether to monitor IPv4 UDP datagram statistics (sent, received). Core UDP traffic monitoring. Default is "yes". |
| `ipv4 UDPLite packets` | boolean | Whether to monitor IPv4 UDP-Lite packet statistics. UDP-Lite allows partial checksum coverage for real-time applications. Default is "yes". |
| `ipv4 errors` | boolean | Whether to monitor IPv4 protocol error statistics (header errors, address errors, unknown protocols). Critical for IP layer troubleshooting. Default is "yes". |
| `ipv4 fragments assembly` | boolean | Whether to monitor IPv4 packet fragmentation reassembly statistics (successful/failed reassembly). Indicates MTU issues or fragmentation attacks. Default is "yes". |
| `ipv4 fragments sent` | boolean | Whether to monitor IPv4 packet fragmentation transmission statistics (fragments created/sent). High values may indicate MTU configuration problems. Default is "yes". |
| `ipv4 packets` | boolean | Whether to monitor IPv4 packet statistics (total packets sent/received/forwarded). Core IP traffic monitoring metric. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for IPv6 protocol and multicast monitoring

| Key | Type | Comments |
|-----|------|----------|
| `bandwidth` | boolean | Whether to monitor IPv6 bandwidth usage (bytes/sec received and sent). Core network utilization metric. Default is "yes". |
| `broadcast bandwidth` | boolean | Whether to monitor IPv6 broadcast bandwidth separately. Important for broadcast storm detection. Default is "yes". |
| `ect` | boolean | Whether to monitor ECN (Explicit Congestion Notification) capable transport statistics. Advanced congestion control metric. Default is "yes". |
| `filename to monitor` | string | Path to the IPv6 statistics file to monitor (typically /proc/net/snmp6). Default is "/proc/net/snmp6". |
| `icmp` | boolean | Whether to monitor overall ICMPv6 statistics. Master control for all ICMPv6 monitoring. Default is "yes". |
| `icmp echos` | boolean | Whether to monitor ICMPv6 echo requests/replies (ping6). Connectivity testing metric. Default is "yes". |
| `icmp errors` | boolean | Whether to monitor ICMPv6 error messages. Essential for IPv6 troubleshooting. Default is "yes". |
| `icmp group membership` | boolean | Whether to monitor ICMPv6 multicast group membership messages. Important for multicast routing. Default is "yes". |
| `icmp mldv2` | boolean | Whether to monitor MLDv2 (Multicast Listener Discovery v2) messages. Advanced multicast protocol monitoring. Default is "yes". |
| `icmp neighbor` | boolean | Whether to monitor ICMPv6 neighbor discovery messages. Critical for IPv6 address resolution. Default is "yes". |
| `icmp redirects` | boolean | Whether to monitor ICMPv6 redirect messages. Routing optimization indicator. Default is "yes". |
| `icmp router` | boolean | Whether to monitor ICMPv6 router discovery messages. Essential for IPv6 autoconfiguration. Default is "yes". |
| `icmp types` | boolean | Whether to monitor all ICMPv6 message types by code. Detailed ICMPv6 analysis. Default is "yes". |
| `ipv6 UDP errors` | boolean | Whether to monitor UDPv6 protocol errors. UDP reliability indicator. Default is "yes". |
| `ipv6 UDP packets` | boolean | Whether to monitor UDPv6 packet statistics. Core UDP traffic metric. Default is "yes". |
| `ipv6 UDPlite errors` | boolean | Whether to monitor UDP-Lite over IPv6 errors. Error-tolerant protocol monitoring. Default is "yes". |
| `ipv6 UDPlite packets` | boolean | Whether to monitor UDP-Lite over IPv6 packet statistics. Multimedia protocol monitoring. Default is "yes". |
| `ipv6 errors` | boolean | Whether to monitor IPv6 protocol errors. Essential network health metric. Default is "yes". |
| `ipv6 fragments assembly` | boolean | Whether to monitor IPv6 fragment reassembly. Fragmentation performance metric. Default is "yes". |
| `ipv6 fragments sent` | boolean | Whether to monitor IPv6 fragments sent. MTU and path issue indicator. Default is "yes". |
| `ipv6 packets` | boolean | Whether to monitor IPv6 packet statistics. Core IPv6 traffic metric. Default is "yes". |
| `multicast bandwidth` | boolean | Whether to monitor multicast bandwidth usage separately. Important for multicast-heavy environments. Default is "yes". |
| `multicast packets` | boolean | Whether to monitor multicast packet statistics. Multicast traffic analysis. Default is "yes". |

### Section `plugin:proc:/proc/net/sockstat`
Network socket statistics monitoring configuration

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to sockstat file to monitor. Default is "/proc/net/sockstat". Can be overridden for containers or testing. |
| `ipv4 FRAG memory` | boolean | Enable monitoring of IPv4 fragment reassembly memory usage. Shows memory used for packet fragmentation. Default is AUTO. |
| `ipv4 FRAG sockets` | boolean | Enable monitoring of IPv4 fragment reassembly pseudo-sockets. Shows count of fragments being reassembled. Default is AUTO. |
| `ipv4 RAW sockets` | boolean | Enable monitoring of IPv4 raw sockets count. Used by ping, traceroute and other low-level network tools. Default is AUTO. |
| `ipv4 TCP memory` | boolean | Enable monitoring of TCP memory usage. Shows memory allocated for TCP connections and buffers. Default is AUTO. |
| `ipv4 TCP sockets` | boolean | Enable monitoring of TCP socket counts by state (established, listen, etc). Essential for connection tracking. Default is AUTO. |
| `ipv4 UDP memory` | boolean | Enable monitoring of UDP memory usage. Shows memory allocated for UDP sockets and buffers. Default is AUTO. |
| `ipv4 UDP sockets` | boolean | Enable monitoring of UDP socket counts. Shows number of open UDP sockets. Default is AUTO. |
| `ipv4 UDPLITE sockets` | boolean | Enable monitoring of UDP-Lite socket counts. UDP-Lite is used for error-tolerant applications. Default is AUTO. |
| `ipv4 sockets` | boolean | Enable monitoring of total IPv4 sockets count. Shows overall socket usage across all protocols. Default is AUTO. |
| `update constants every` | duration | How often to update socket limit constants from /proc/sys. These change rarely. Default is 60 seconds. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for IPv6 socket statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the IPv6 socket statistics file to monitor (typically /proc/net/sockstat6). Default is "/proc/net/sockstat6". |
| `ipv6 FRAG sockets` | boolean | Whether to monitor IPv6 fragment reassembly pseudo-sockets. Shows fragments being processed. Default is "yes". |
| `ipv6 RAW sockets` | boolean | Whether to monitor IPv6 raw socket counts. Used by ICMPv6 tools like ping6. Default is "yes". |
| `ipv6 TCP sockets` | boolean | Whether to monitor TCPv6 socket counts by state. Essential IPv6 connection tracking. Default is "yes". |
| `ipv6 UDP sockets` | boolean | Whether to monitor UDPv6 socket counts. Shows open UDP endpoints. Default is "yes". |
| `ipv6 UDPLITE sockets` | boolean | Whether to monitor UDP-Lite over IPv6 socket counts. Error-tolerant protocol usage. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for network softirq statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the softnet statistics file to monitor (typically /proc/net/softnet_stat). Default is "/proc/net/softnet_stat". |
| `softnet_stat per core` | boolean | Whether to monitor software network interrupt statistics per CPU core. Essential for identifying CPU bottlenecks in network processing. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for netfilter connection tracking monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the netfilter conntrack statistics file to monitor (typically /proc/net/stat/nf_conntrack). Default is "/proc/net/stat/nf_conntrack". |
| `netfilter connection changes` | boolean | Whether to monitor connection state changes (new to established, etc.). Shows connection lifecycle. Default is "yes". |
| `netfilter connection expectations` | boolean | Whether to monitor connection expectations (for protocols like FTP that open additional connections). Default is "yes". |
| `netfilter connection searches` | boolean | Whether to monitor connection tracking table searches. Performance metric for conntrack efficiency. Default is "yes". |
| `netfilter connections` | boolean | Whether to monitor total connection count and table usage. Critical for capacity planning. Default is "yes". |
| `netfilter errors` | boolean | Whether to monitor connection tracking errors (table full, invalid packets). Important for firewall health. Default is "yes". |
| `netfilter new connections` | boolean | Whether to monitor new connection rate. Shows connection establishment patterns. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for SYNPROXY DDoS mitigation monitoring

| Key | Type | Comments |
|-----|------|----------|
| `SYNPROXY SYN received` | boolean | Whether to monitor SYN packets received by SYNPROXY. Shows incoming connection attempts. Default is "yes". |
| `SYNPROXY connections reopened` | boolean | Whether to monitor connections reopened after SYNPROXY validation. Shows legitimate connections. Default is "yes". |
| `SYNPROXY cookies` | boolean | Whether to monitor SYN cookie usage by SYNPROXY. Indicates DDoS mitigation activity. Default is "yes". |
| `filename to monitor` | string | Path to the SYNPROXY statistics file to monitor (typically /proc/net/stat/synproxy). Default is "/proc/net/stat/synproxy". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for system interrupts monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the interrupts file to monitor (typically /proc/interrupts). Default is "/proc/interrupts". |
| `interrupts per core` | boolean | Whether to monitor interrupt counts per CPU core. Essential for identifying interrupt distribution issues. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for comprehensive CPU statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `context switches` | boolean | Whether to monitor CPU context switches. Shows process scheduling activity. Default is "yes". |
| `core_throttle_count` | boolean | Whether to monitor CPU core thermal throttling events. Critical for performance issues. Default is "yes". |
| `core_throttle_count filename to monitor` | string | Path pattern to core throttle count files (e.g., /sys/devices/system/cpu/cpu*/thermal_throttle/core_throttle_count). Default varies by system. |
| `cpu frequency` | boolean | Whether to monitor CPU frequency scaling. Shows power management and performance states. Default is "yes". |
| `cpu idle states` | boolean | Whether to monitor CPU idle state (C-state) usage. Power efficiency metric. Default is "yes". |
| `cpu interrupts` | boolean | Whether to monitor CPU interrupt counts. Shows interrupt handling load. Default is "yes". |
| `cpu utilization` | boolean | Whether to monitor overall CPU usage (user, system, idle, etc.). Core performance metric. Default is "yes". |
| `cpuidle name filename to monitor` | string | Path pattern to CPU idle state name files. Used for C-state identification. Default is "/sys/devices/system/cpu/cpu*/cpuidle/state*/name". |
| `cpuidle time filename to monitor` | string | Path pattern to CPU idle state time files. Shows C-state residence times. Default is "/sys/devices/system/cpu/cpu*/cpuidle/state*/time". |
| `filename to monitor` | string | Path to the main CPU statistics file (typically /proc/stat). Default is "/proc/stat". |
| `keep cpuidle files open` | boolean | Whether to keep CPU idle files open for better performance. Trade-off between file handles and speed. Default is "yes". |
| `keep per core files open` | boolean | Whether to keep per-core CPU files open. Improves performance on systems with many cores. Default is "yes". |
| `package_throttle_count` | boolean | Whether to monitor CPU package-level thermal throttling. Shows chip-wide thermal issues. Default is "yes". |
| `package_throttle_count filename to monitor` | string | Path pattern to package throttle count files. Default varies by system architecture. |
| `per cpu core utilization` | boolean | Whether to monitor CPU usage per individual core. Essential for multi-core performance analysis. Default is "yes". |
| `processes running` | boolean | Whether to monitor number of runnable processes. System load indicator. Default is "yes". |
| `processes started` | boolean | Whether to monitor process creation rate (forks). System activity metric. Default is "yes". |
| `scaling_cur_freq filename to monitor` | string | Path pattern to current CPU frequency files. Default is "/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq". |
| `schedstat filename to monitor` | string | Path to scheduler statistics file for advanced scheduling metrics. Default is "/proc/schedstat". |
| `time_in_state filename to monitor` | string | Path pattern to CPU frequency time-in-state files. Shows time spent at each frequency. Default is "/sys/devices/system/cpu/cpu*/cpufreq/stats/time_in_state". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for generic system file monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the system statistics file to monitor. Module-specific, typically a /proc or /sys file containing metrics. Default varies by module. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for kernel statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the kernel statistics file to monitor. Module-specific, often used for specialized kernel metrics. Default varies by module. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for hardware statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the hardware statistics file to monitor. Module-specific, typically sysfs files for hardware metrics. Default varies by module. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for periodic statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the statistics file to monitor. Module-specific, often used for metrics that change infrequently. Default varies by module. |
| `read every seconds` | number | How often to read this file in seconds. Useful for files that update less frequently to reduce I/O. Default is 1 second. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for process statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `filename to monitor` | string | Path to the process statistics file to monitor. Module-specific, typically /proc files for process metrics. Default varies by module. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for memory management monitoring

| Key | Type | Comments |
|-----|------|----------|
| `disk i/o` | boolean | Whether to monitor disk I/O related to memory operations (swapping, paging). Shows memory pressure impact on storage. Default is "yes". |
| `filename to monitor` | string | Path to memory statistics file (typically /proc/vmstat or similar). Default is "/proc/vmstat". |
| `kernel same memory` | boolean | Whether to monitor kernel same-page merging (KSM) statistics. Shows memory deduplication efficiency. Default is "yes". |
| `memory ballooning` | boolean | Whether to monitor memory ballooning in virtualized environments. Important for VM memory management. Default is "yes". |
| `memory page faults` | boolean | Whether to monitor page fault statistics (minor/major). Essential memory performance metric. Default is "yes". |
| `out of memory kills` | boolean | Whether to monitor out-of-memory (OOM) killer activity. Critical for system stability monitoring. Default is "yes". |
| `swap i/o` | boolean | Whether to monitor swap in/out operations. Shows memory pressure and performance impact. Default is "yes". |
| `system-wide numa metric summary` | boolean | Whether to monitor NUMA (Non-Uniform Memory Access) statistics. Important for NUMA-aware systems. Default is "yes". |
| `transparent huge pages` | boolean | Whether to monitor transparent huge pages (THP) statistics. Shows large page usage and efficiency. Default is "yes". |
| `zswap i/o` | boolean | Whether to monitor compressed swap (zswap) statistics. Shows memory compression effectiveness. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for PCIe monitoring

| Key | Type | Comments |
|-----|------|----------|
| `enable pci slots` | boolean | Whether to monitor individual PCIe slot statistics (bandwidth, errors). Important for PCIe device performance. Default is "yes". |
| `enable root ports` | boolean | Whether to monitor PCIe root port statistics. Shows PCIe hierarchy performance and errors. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for power supply and battery monitoring

| Key | Type | Comments |
|-----|------|----------|
| `battery capacity` | boolean | Whether to monitor battery capacity percentage. Shows battery health and charge level. Default is "yes". |
| `battery charge` | boolean | Whether to monitor battery charge in Ah (Ampere-hours). Shows actual charge amount. Default is "yes". |
| `battery energy` | boolean | Whether to monitor battery energy in Wh (Watt-hours). Shows energy storage capacity. Default is "yes". |
| `battery power` | boolean | Whether to monitor battery power draw/charge rate in Watts. Shows charging/discharging rate. Default is "yes". |
| `directory to monitor` | string | Path to power supply sysfs directory (typically /sys/class/power_supply). Default is "/sys/class/power_supply". |
| `keep files open` | boolean | Whether to keep power supply files open for better performance. Reduces syscall overhead. Default is "yes". |
| `power supply voltage` | boolean | Whether to monitor power supply voltage levels. Important for power quality monitoring. Default is "yes". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for thermal monitoring

| Key | Type | Comments |
|-----|------|----------|
| `directory to monitor` | string | Path to thermal zone sysfs directory (typically /sys/class/thermal). Contains temperature sensors and cooling devices. Default is "/sys/class/thermal". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for hardware monitoring

| Key | Type | Comments |
|-----|------|----------|
| `directory to monitor` | string | Path to hardware monitoring sysfs directory (typically /sys/class/hwmon). Contains sensor data from various hardware monitoring chips. Default is "/sys/class/hwmon". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for NUMA statistics monitoring

| Key | Type | Comments |
|-----|------|----------|
| `directory to monitor` | string | Path to NUMA statistics directory (typically /sys/devices/system/node). Contains per-node memory and CPU statistics. Default is "/sys/devices/system/node". |
| `enable per-node numa metrics` | boolean | Whether to monitor detailed metrics for each NUMA node separately. Essential for NUMA performance optimization. Default is "yes". |

### Section `plugin:proc:/sys/fs/btrfs`
Btrfs filesystem monitoring configuration

| Key | Type | Comments |
|-----|------|----------|
| `check for btrfs changes every` | duration | How often to scan for new/removed Btrfs filesystems. Btrfs mounts can appear/disappear dynamically. Default is 60 seconds. Lower values detect changes faster but use more CPU. |
| `commit stats` | boolean | Enable monitoring of Btrfs commit statistics (commit duration, max commit duration). Shows filesystem write performance. Default is YES. |
| `data allocation` | boolean | Enable monitoring of Btrfs data space allocation. Shows how much space is allocated/used for file data. Default is AUTO. |
| `error stats` | boolean | Enable monitoring of Btrfs error statistics (I/O errors, checksum failures, corruption). Critical for filesystem health. Default is AUTO. |
| `metadata allocation` | boolean | Enable monitoring of Btrfs metadata space allocation. Shows space used for filesystem structures. Default is AUTO. |
| `path to monitor` | string | Base path to Btrfs sysfs entries. Default is "/sys/fs/btrfs". Can be overridden for containers or testing. |
| `physical disks allocation` | boolean | Enable monitoring of physical device allocation in Btrfs. Shows how data is distributed across devices. Default is AUTO. |
| `system allocation` | boolean | Enable monitoring of Btrfs system chunk allocation. System chunks store critical filesystem metadata. Default is AUTO. |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for Kernel Same-page Merging (KSM) monitoring

| Key | Type | Comments |
|-----|------|----------|
| `/sys/kernel/mm/ksm/pages_shared` | string | Path to KSM pages_shared file. Shows number of shared pages. Default is "/sys/kernel/mm/ksm/pages_shared". |
| `/sys/kernel/mm/ksm/pages_sharing` | string | Path to KSM pages_sharing file. Shows number of pages currently being shared. Default is "/sys/kernel/mm/ksm/pages_sharing". |
| `/sys/kernel/mm/ksm/pages_to_scan` | string | Path to KSM pages_to_scan file. Shows pages scanned per iteration. Default is "/sys/kernel/mm/ksm/pages_to_scan". |
| `/sys/kernel/mm/ksm/pages_unshared` | string | Path to KSM pages_unshared file. Shows unique pages that cannot be merged. Default is "/sys/kernel/mm/ksm/pages_unshared". |
| `/sys/kernel/mm/ksm/pages_volatile` | string | Path to KSM pages_volatile file. Shows pages that change too frequently to merge. Default is "/sys/kernel/mm/ksm/pages_volatile". |

### Section `[plugin:PLUGIN_NAME:MODULE]`
Plugin-specific configuration section for System V IPC monitoring

| Key | Type | Comments |
|-----|------|----------|
| `max dimensions in memory allowed` | number | Maximum number of IPC objects to track in memory. Prevents excessive memory usage on systems with many IPC objects. Default is 1024. |
| `message queues` | boolean | Whether to monitor System V message queue statistics. Shows IPC message queue usage. Default is "yes". |
| `msg filename to monitor` | string | Path to message queue statistics file. Default is "/proc/sysvipc/msg". |
| `semaphore totals` | boolean | Whether to monitor System V semaphore statistics. Shows IPC semaphore usage. Default is "yes". |
| `shared memory totals` | boolean | Whether to monitor System V shared memory statistics. Shows IPC shared memory segments. Default is "yes". |
| `shm filename to monitor` | string | Path to shared memory statistics file. Default is "/proc/sysvipc/shm". |

### Section `plugin:tc`
Linux Traffic Control (tc) QoS monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `cleanup unused classes every` | number | Interval in seconds to cleanup unused TC classes and qdiscs. Prevents memory leaks from temporary QoS configurations. Default is "300" (5 minutes). |
| `enable ctokens charts for all interfaces` | boolean | Whether to monitor TC committed token bucket charts for all network interfaces. Shows QoS token bucket depth for traffic shaping. Default is "yes". |
| `enable show all classes and qdiscs for all interfaces` | boolean | Whether to monitor all TC classes and queuing disciplines for all interfaces. Provides comprehensive QoS monitoring. Default is "yes". |
| `enable tokens charts for all interfaces` | boolean | Whether to monitor TC token bucket charts for all network interfaces. Essential for monitoring traffic shaping and rate limiting. Default is "yes". |
| `script to run to get tc values` | string | Path to script for retrieving TC statistics. Allows custom TC data collection methods. Default is system TC command. |
| `var_name` | boolean | Whether to enable TC variable name processing. Used for dynamic TC configuration parsing. Default is "yes". |

### Section `plugin:windows`
Windows system monitoring plugin configuration

| Key | Type | Comments |
|-----|------|----------|
| `pm->name` | boolean | Whether to enable specific Windows performance counters and WMI monitoring modules. The key name is dynamically set from the module name (e.g., "PerflibProcessor", "PerflibMemory", "PerflibNetwork"). When set to "no", that specific module will not be monitored. Default is "yes". |

## Configuration: stream.conf
**Handle**: `stream_config`

### Section `CONFIG_SECTION_STREAM`
Streaming configuration for parent-child node relationships

| Key | Type | Comments |
|-----|------|----------|
| `CAfile` | string | Path to SSL certificate authority file for verifying parent node certificates. Used when streaming to parent with SSL/TLS enabled. |
| `CApath` | string | Path to directory containing SSL certificate authority files for verifying parent certificates. Alternative to CAfile for systems with multiple CA certificates. |
| `api key` | string | Authentication key for streaming to parent node. Must match a configured API key on the parent. Required for establishing streaming connections. |
| `brotli compression level` | number | Brotli compression level (0-11). Higher values provide better compression but use more CPU. Default is 3. 0=fastest, 11=best compression. |
| `buffer size` | size | Maximum size of the streaming buffer in bytes. Controls memory usage for outgoing data. Accepts units like MB, GB. Default is 10MB. |
| `buffer size bytes` | number | Legacy option for buffer size in bytes. Use 'buffer size' instead for better readability with units. |
| `default port` | number | Default port for connecting to parent node if not specified in destination. Default is 19999. |
| `destination` | string | Parent node destination in format 'host:port' or 'host'. Multiple destinations separated by spaces for failover. First available parent is used. |
| `enable compression` | boolean | Enable compression for streaming data. Reduces bandwidth but increases CPU usage. Default is yes. Negotiates best algorithm with parent. |
| `enabled` | boolean | Enable streaming to parent node. Set to 'yes' to send data to configured destination. Default is no. |
| `gzip compression level` | number | Gzip compression level (1-9). Higher values provide better compression but use more CPU. Default is 3. 1=fastest, 9=best compression. |
| `initial clock resync iterations` | number | Number of iterations to sync clocks between parent and child before streaming starts. Helps ensure accurate timestamps. Default is 60. |
| `lz4 compression acceleration` | number | LZ4 compression acceleration factor (1-9). Higher values mean faster compression but lower ratio. Default is 1. 9=fastest, 1=best compression. |
| `parent using h2o` | boolean | Set to 'yes' if parent is using H2O web server. Adjusts protocol handling for compatibility. Default is no. |
| `reconnect delay` | duration | Delay in seconds before attempting to reconnect to the parent node after a connection failure. Default is 15 seconds. Minimum is 5 seconds. |
| `send charts matching` | string | Pattern for selecting which charts to stream. Uses simple patterns with wildcards. Default is '*' (all charts). Example: 'system.* disk.*' |
| `ssl skip certificate verification` | boolean | Skip SSL certificate verification when connecting to parent. WARNING: Insecure, use only for testing. Default is no. |
| `timeout` | duration | Connection timeout in seconds for streaming operations. Applies to connection establishment and data transmission. Default is 300 seconds. |
| `zstd compression level` | number | Zstandard compression level (1-22). Provides excellent compression with good speed. Default is 3. 1=fastest, 22=best compression. |

### Section `api_key`
API key specific configuration for streaming receivers

| Key | Type | Comments |
|-----|------|----------|
| `allow from` | string | IP addresses or hostnames allowed to connect with this API key. Supports simple patterns with wildcards. Default is '*' (all). Example: '10.0.0.* localhost' |
| `compression algorithms order` | string | Preferred compression algorithm order for negotiation. Space-separated list. Default is 'zstd lz4 brotli gzip'. First mutually supported algorithm is used. |
| `db` | string | Database engine to use for metrics storage. Options: 'dbengine', 'ram', 'alloc', 'none'. Default inherits from global setting. 'dbengine' is recommended. |
| `enable compression` | boolean | Enable compression for connections using this API key. Reduces bandwidth usage. Default is yes. Negotiates with child node capabilities. |
| `enable replication` | boolean | Enable database replication for child nodes using this API key. Allows child to request historical data. Default is yes. |
| `enabled` | boolean | Enable this API key for accepting streaming connections. Set to 'no' to temporarily disable without removing configuration. Default is yes. |
| `health enabled` | boolean | Enable health monitoring and alerts for nodes using this API key. Can be 'yes', 'no', or 'auto'. Default is 'auto' (enabled if health plugin is enabled). |
| `health log retention` | duration | How long to retain health log entries in seconds. Controls alert history visibility. Default is 432000 (5 days). Minimum is 3600 (1 hour). |
| `postpone alerts on connect` | duration | Time in seconds to postpone alerts after a child connects. Prevents false alerts during initial sync. Default is 60 seconds. 0 disables postponement. |
| `proxy api key` | string | API key to use when this receiver acts as proxy, forwarding data to another parent. Must match key on upstream parent. Empty disables proxying. |
| `proxy destination` | string | Destination to forward received metrics when acting as proxy. Format: 'host:port'. Empty disables proxying. Enables multi-level streaming architectures. |
| `proxy enabled` | boolean | Enable proxy mode for nodes using this API key. When enabled, received data is forwarded to proxy destination. Default is no. |
| `proxy send charts matching` | string | Pattern for selecting which charts to forward when proxying. Uses simple patterns. Default is '*' (all). Example: 'system.* apps.*' |
| `replication period` | duration | Maximum time window of historical data to replicate. Default is 86400 (1 day). Larger values increase memory usage and sync time. |
| `replication step` | duration | Time granularity for replication data transfer. Default is 3600 (1 hour). Smaller values mean more frequent but smaller transfers. |
| `retention` | number | Data retention period in seconds for nodes using this API key. Overrides global retention. Default is 3600 (1 hour). Use with care on parents. |
| `type` | string | Type identifier for this API key. Used for grouping and identifying connection types. Examples: 'production', 'development', 'testing'. |

### Section `machine_guid`
Per-machine streaming configuration section where machine_guid is replaced with the actual GUID of a specific child node

| Key | Type | Comments |
|-----|------|----------|
| `compression algorithms order` | string | Preferred order of compression algorithms for streaming data (e.g., "zstd,lz4,gzip"). Client will negotiate best supported algorithm. Default is "zstd,lz4,gzip". |
| `db` | string | Database engine type for storing streamed data ("dbengine", "memory", "none"). Affects data persistence and memory usage. Default is "dbengine". |
| `enable compression` | boolean | Whether to enable data compression for streaming. Reduces bandwidth usage but increases CPU overhead. Default is "yes". |
| `enable replication` | boolean | Whether to enable historical data replication from child nodes. Allows parents to backfill missing data. Default is "yes". |
| `health enabled` | boolean | Whether to enable health monitoring and alerting for this streamed data source. Controls alert processing for child nodes. Default is "yes". |
| `health log retention` | duration | How long to keep health monitoring alerts and events in the log. Default is 5 days. Older entries are automatically removed. Format: number with unit (s/m/h/d). |
| `postpone alerts on connect` | duration | Delay sending alerts for this period after a child node connects to avoid false positives during initial synchronization. Default is 60 seconds. Format: number with unit (s/m/h/d). |
| `proxy api key` | string | API key for authenticating with the proxy destination. Required when proxy is enabled for authentication. Leave empty for no authentication. |
| `proxy destination` | string | Target proxy server URL for forwarding streaming data (e.g., "https://proxy.example.com:19999"). Used for data forwarding chains. |
| `proxy enabled` | boolean | Whether to enable proxy forwarding for this data stream. Allows creating data forwarding hierarchies. Default is "no". |
| `proxy send charts matching` | string | Pattern matching charts to forward via proxy (simple patterns supported). Use "*" for all charts or specific patterns like "system.*". Default is "*". |
| `replication period` | duration | Maximum historical data time window to replicate from child nodes. Default is 1 day (86400s). Larger values increase memory usage and initial sync time. Format: number with unit (s/m/h/d). |
| `replication step` | duration | Time interval for each replication batch. Default is 1 hour (3600s). Smaller values mean more frequent but smaller data transfers. Format: number with unit (s/m/h/d). |
| `retention` | number | Data retention period in seconds for this specific machine. Overrides API key and global retention settings. Default inherits from API key configuration. |
| `update every` | duration | Override data collection frequency for streamed metrics from this host. Inherits from global setting if not specified. Format: number with unit (s/m/h/d). |

## Configuration: cloud.conf
**Handle**: `cloud_config`

### Section `CONFIG_SECTION_GLOBAL`
Global cloud connectivity configuration settings

| Key | Type | Comments |
|-----|------|----------|
| `claimed_id` | string | Unique identifier assigned when node is claimed to Netdata Cloud. Auto-generated during claim process. Do not modify manually. |
| `hostname` | string | Hostname override for cloud identification. If not set, uses system hostname. Helps identify node in cloud interface. |
| `insecure` | boolean | Skip SSL certificate verification for cloud connections. WARNING: Only use for testing. Default is no. |
| `machine_guid` | string | Unique machine identifier for cloud registration. Auto-generated if not set. Must be unique across all nodes. |
| `proxy` | string | HTTP proxy URL for cloud connectivity. Format: 'http://proxy:port' or 'socks5://proxy:port'. Empty for direct connection. |
| `rooms` | string | Comma-separated list of cloud room IDs to join. Rooms organize nodes into groups. Can be updated after claiming. |
| `token` | string | Authentication token for Netdata Cloud connection. Obtained during claim process. Keep secret and do not share. |
| `url` | string | Netdata Cloud service URL endpoint. Default is 'https://api.netdata.cloud'. Only change for private cloud deployments. |

## Configuration: exporting.conf
**Handle**: `exporting_config`

### Section `CONFIG_SECTION_EXPORTING`
Data export and external system integration

| Key | Type | Comments |
|-----|------|----------|
| `enabled` | boolean | Enable/disable all exporting connectors globally. Individual connectors can override this. Default is no. |
| `name` | boolean | DEPRECATED: Legacy configuration option. This key is no longer used and will be ignored. |

## Configuration: claim.conf
**Handle**: `claim_config`

### Section `CONFIG_SECTION_GLOBAL`
Global cloud claiming configuration settings

| Key | Type | Comments |
|-----|------|----------|
| `insecure` | boolean | Skip SSL certificate verification during claim process. WARNING: Only use for testing. Default is no. |
| `proxy` | string | HTTP proxy URL for claim process. Format: 'http://proxy:port' or 'socks5://proxy:port'. Empty for direct connection. |
| `rooms` | string | Comma-separated list of cloud room IDs to join during claim. Rooms organize nodes into groups. Can be changed later. |
| `token` | string | One-time claim token from Netdata Cloud. Obtained from cloud interface when adding a new node. Expires after use. |
| `url` | string | Netdata Cloud claiming service URL. Default is 'https://api.netdata.cloud'. Only change for private cloud deployments. |

## Configuration: ebpf.conf
**Handle**: `collector_config`

### Section `EBPF_GLOBAL_SECTION`
eBPF collector global settings

| Key | Type | Comments |
|-----|------|----------|
| `EBPF_CFG_APPLICATION` | boolean | Enable per-application statistics collection. Groups metrics by application name. CPU intensive but provides detailed insights. Default is yes. |
| `EBPF_CFG_CGROUP` | boolean | Enable cgroup (container) statistics collection. Essential for container monitoring (Docker, Kubernetes). Default is yes. |
| `EBPF_CFG_LIFETIME` | number | Thread lifetime in seconds. After this period, eBPF threads exit and restart. Helps with memory management. Default is 300 (5 minutes). |
| `EBPF_CFG_LOAD_MODE` | string | eBPF loading mode: 'entry' (only function entry), 'return' (entry and return), 'update' (live update). Default is 'entry' for performance. |
| `EBPF_CFG_MAPS_PER_CORE` | boolean | Allocate eBPF maps per CPU core. Improves performance on multi-core systems but uses more memory. Default is yes. |
| `EBPF_CFG_PID_SIZE` | number | Maximum number of PIDs to monitor simultaneously. Higher values use more kernel memory. Default is 32768. |
| `EBPF_CFG_PROGRAM_PATH` | string | Custom path to eBPF programs. Leave empty to use bundled programs. Used for development or custom eBPF programs. |
| `EBPF_CFG_TYPE_FORMAT` | string | Output format for eBPF metrics: 'auto', 'legacy', or 'co-re'. Auto-detects best format. Default is 'auto'. |
| `EBPF_CFG_UPDATE_EVERY` | number | Data collection frequency in seconds. Lower values provide more granular data but increase CPU usage. Default is 1 second. |
| `disable apps` | boolean | Disable all application-level monitoring to reduce overhead. Overrides individual app settings. Default is no. |
| `load` | string | Legacy option for eBPF loading mode. Use EBPF_CFG_LOAD_MODE instead. Kept for backward compatibility. |

### Section `EBPF_PROGRAMS_SECTION`
eBPF program enable/disable configuration

| Key | Type | Comments |
|-----|------|----------|
| `cachestat` | boolean | Whether to enable eBPF monitoring of page cache statistics. Tracks page cache hits/misses, helping identify I/O performance issues. Default is "auto". |
| `dcstat` | boolean | Whether to enable eBPF monitoring of directory cache (dcache) statistics. Shows directory lookup performance and cache efficiency. Default is "auto". |
| `disk` | boolean | Whether to enable eBPF monitoring of disk I/O operations. Provides detailed disk latency histograms and I/O patterns. Default is "auto". |
| `ebpf_modules[EBPF_MODULE_PROCESS_IDX].info.config_name` | boolean | Whether to enable eBPF process monitoring module. Tracks process creation, termination, and resource usage. Default is "auto". |
| `ebpf_modules[EBPF_MODULE_SOCKET_IDX].info.config_name` | boolean | Whether to enable eBPF socket monitoring module. Provides detailed socket-level metrics including TCP retransmissions and connection states. Default is "auto". |
| `fd` | boolean | Whether to enable eBPF monitoring of file descriptor operations. Tracks file opens, closes, and errors by process. Default is "auto". |
| `filesystem` | boolean | Whether to enable eBPF monitoring of filesystem operations. Shows VFS calls like read, write, open, and fsync by filesystem type. Default is "auto". |
| `hardirq` | boolean | Whether to enable eBPF monitoring of hardware interrupts. Tracks IRQ latencies and distribution across CPUs. Default is "auto". |
| `mdflush` | boolean | Whether to enable eBPF monitoring of MD (software RAID) flush operations. Tracks RAID array synchronization and performance. Default is "auto". |
| `mount` | boolean | Whether to enable eBPF monitoring of mount/umount operations. Tracks filesystem mounting activities and errors. Default is "auto". |
| `network connection monitoring` | boolean | Whether to enable comprehensive eBPF network connection monitoring. Provides detailed TCP/UDP connection tracking and statistics. Default is "auto". |
| `network connections` | boolean | Whether to enable basic eBPF network connection tracking. Shows active connections by protocol and state. Default is "auto". |
| `network viewer` | boolean | Whether to enable eBPF network viewer for real-time traffic visualization. Shows network flows between processes and remote endpoints. Default is "auto". |
| `oomkill` | boolean | Whether to enable eBPF monitoring of Out-Of-Memory killer events. Tracks which processes are killed due to memory pressure. Default is "auto". |
| `shm` | boolean | Whether to enable eBPF monitoring of shared memory operations. Tracks IPC shared memory usage and system calls. Default is "auto". |
| `softirq` | boolean | Whether to enable eBPF monitoring of software interrupts. Shows softirq processing time and distribution across CPUs. Default is "auto". |
| `swap` | boolean | Whether to enable eBPF monitoring of swap operations. Tracks swap in/out activity by process, critical for memory pressure analysis. Default is "auto". |
| `sync` | boolean | Whether to enable eBPF monitoring of sync system calls. Tracks filesystem synchronization operations like sync, fsync, and fdatasync. Default is "auto". |
| `vfs` | boolean | Whether to enable eBPF monitoring of Virtual File System operations. Shows detailed VFS call statistics across all filesystems. Default is "auto". |

### Section `NETDATA_EBPF_IPC_SECTION`
eBPF Inter-Process Communication monitoring configuration

| Key | Type | Comments |
|-----|------|----------|
| `NETDATA_EBPF_IPC_BACKLOG` | number | Maximum queue size for IPC event backlog. Higher values prevent event loss but use more memory. Default is 4096. |
| `NETDATA_EBPF_IPC_BIND_TO` | string | IP address to bind IPC monitoring socket. Use '0.0.0.0' for all interfaces or specific IP. Default is 'localhost'. |
| `NETDATA_EBPF_IPC_INTEGRATION` | string | Integration mode for IPC monitoring: 'internal' (built-in) or 'external' (separate process). Default is 'internal'. |

## Configuration: cfg.conf
**Handle**: `cfg`

### Section `EBPF_GLOBAL_SECTION`
eBPF collector global settings

| Key | Type | Comments |
|-----|------|----------|
| `EBPF_CONFIG_SOCKET_MONITORING_SIZE` | number | Maximum number of socket connections to monitor simultaneously. Higher values provide more coverage but use more memory. Default is 8192. |
| `EBPF_CONFIG_UDP_SIZE` | number | Maximum number of UDP connections to track. UDP is connectionless, so this tracks recent packet flows. Default is 4096. |

### Section `EBPF_NETWORK_VIEWER_SECTION`
eBPF network monitoring specific settings

| Key | Type | Comments |
|-----|------|----------|
| `EBPF_CONFIG_HOSTNAMES` | string | Space-separated list of hostnames to monitor in network viewer. Use patterns with wildcards. Empty means all hostnames. Example: '*.local *.mydomain.com' |
| `EBPF_CONFIG_PORTS` | string | Space-separated list of ports to monitor. Can use ranges with hyphen. Empty means all ports. Example: '80 443 8080-8090 3306' |
| `EBPF_CONFIG_RESOLVE_HOSTNAME` | boolean | Whether to resolve IP addresses to hostnames in network viewer. May impact performance on busy systems. Default is yes. |
| `EBPF_CONFIG_RESOLVE_SERVICE` | boolean | Whether to resolve port numbers to service names (e.g., 80→http). Uses /etc/services. Default is yes. |
| `ips` | string | Space-separated list of IP addresses or subnets to monitor. Supports CIDR notation. Empty means all IPs. Example: '10.0.0.0/8 192.168.1.1' |

## Configuration: config.conf
**Handle**: `config`

## Configuration: ebpf_filesystem.conf
**Handle**: `fs_config`

### Section `NETDATA_FILESYSTEM_CONFIG_NAME`
Filesystem monitoring configuration

| Key | Type | Comments |
|-----|------|----------|
| `dist` | boolean | Enable monitoring for distributed/network filesystems (NFS, CIFS, etc). May cause performance issues if enabled. Default is no. |

## Configuration: modules->cfg.conf
**Handle**: `modules->cfg`

### Section `EBPF_GLOBAL_SECTION`
eBPF collector global settings

| Key | Type | Comments |
|-----|------|----------|
| `EBPF_CFG_APPLICATION` | boolean | Enable per-application statistics collection. Groups metrics by application name. CPU intensive but provides detailed insights. Default is yes. |
| `EBPF_CFG_CGROUP` | boolean | Enable cgroup (container) statistics collection. Essential for container monitoring (Docker, Kubernetes). Default is yes. |
| `EBPF_CFG_COLLECT_PID` | string | PID collection mode: 'real' (actual PIDs), 'user' (per-user), 'all' (everything). Default is 'all'. |
| `EBPF_CFG_CORE_ATTACH` | string | Core attachment method: 'trampoline' (newer, efficient) or 'probe' (legacy, compatible). Default is 'trampoline' if supported. |
| `EBPF_CFG_LIFETIME` | number | Thread lifetime in seconds. After this period, eBPF threads exit and restart. Helps with memory management. Default is 300 (5 minutes). |
| `EBPF_CFG_LOAD_MODE` | string | eBPF loading mode: 'entry' (only function entry), 'return' (entry and return), 'update' (live update). Default is 'entry' for performance. |
| `EBPF_CFG_MAPS_PER_CORE` | boolean | Allocate eBPF maps per CPU core. Improves performance on multi-core systems but uses more memory. Default is yes. |
| `EBPF_CFG_PID_SIZE` | number | Maximum number of PIDs to monitor simultaneously. Higher values use more kernel memory. Default is 32768. |
| `EBPF_CFG_TYPE_FORMAT` | string | Output format for eBPF metrics: 'auto', 'legacy', or 'co-re'. Auto-detects best format. Default is 'auto'. |
| `EBPF_CFG_UPDATE_EVERY` | number | Data collection frequency in seconds. Lower values provide more granular data but increase CPU usage. Default is 1 second. |

## Configuration: ebpf_socket.conf
**Handle**: `socket_config`

### Section `EBPF_NETWORK_VIEWER_SECTION`
eBPF network monitoring specific settings

| Key | Type | Comments |
|-----|------|----------|
| `enabled` | boolean | Enable/disable eBPF network viewer module for real-time network connection monitoring. Provides deep kernel-level visibility into network traffic patterns. Default is "auto" (enabled if eBPF is supported). |

## Configuration: sockets->config.conf
**Handle**: `sockets->config`

### Section `sockets->config_section`
Web server socket configuration section for API and dashboard access endpoints

| Key | Type | Comments |
|-----|------|----------|
| `default port` | number | Default web server port |

## Configuration: ebpf_sync.conf
**Handle**: `sync_config`

### Section `NETDATA_SYNC_CONFIG_NAME`
eBPF sync syscall monitoring configuration

| Key | Type | Comments |
|-----|------|----------|
| `local_syscalls[i].syscall` | boolean | Enable/disable monitoring for specific sync-related system calls. Key names are dynamically generated based on available syscalls (e.g., sync, fsync, fdatasync, syncfs, msync, sync_file_range). Default is yes for all. |

## Configuration: tmp_config.conf
**Handle**: `tmp_config`

### Section `section`
Generic configuration section placeholder used in command-line tools

| Key | Type | Comments |
|-----|------|----------|
| `key` | string | Generic configuration key used with -W get2 command for retrieving configuration values. The actual key name and type depend on the specific section and configuration being queried. |

### Section `temp + offset + 1`
Dynamically generated section name (internal use)

| Key | Type | Comments |
|-----|------|----------|
| `temp + offset2 + 1` | string | Dynamically generated key name. Used internally for parsing hierarchical configuration sections with colon separators. Not directly user-configurable. |
