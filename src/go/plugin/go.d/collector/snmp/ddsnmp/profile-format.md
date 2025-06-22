# Profile Format

## Overview & Purpose

An **SNMP profile** defines _how_ a device type is monitored through SNMP.
It tells the collector which OIDs to query, how to interpret their values, and how to present them as Netdata metrics, dimensions, and tags.

Profiles let you describe entire device families (routers, switches, printers, UPSes, etc.) declaratively—so that users don’t have to hard-code OIDs or chart logic.

### What an SNMP profile does

When Netdata connects to an SNMP device, it:

1. Reads the device’s system OID (sysObjectID) and system description (sysDescr).
2. Finds the first profile whose selector rules match that information.
3. Uses the profile’s configuration to:
    - Collect scalar metrics (single values such as uptime or temperature).
    - Collect table metrics (rows of data such as per-interface counters).
    - Create virtual metrics (derived or combined values).
    - Extract tags and metadata to enrich the metrics.

Each profile therefore acts as a schema for one or more related device models.

### Example: a minimal “Hello World” profile

```yaml
# hello-world.yaml
selector:
  sysobjectid:
    - 1.3.6.1.4.1.8072.3.2.10  # Matches Net-SNMP demo devices

metrics:
  - name: sysUpTime
    symbol:
      OID: 1.3.6.1.2.1.1.3.0
      name: sysUpTime
    chart_meta:
      description: System uptime
      unit: seconds
```

This profile:

- Matches devices whose sysObjectID is 1.3.6.1.4.1.8072.3.2.10.
- Defines one scalar metric (sysUpTime).
- Creates a chart in Netdata showing system uptime in seconds.

### Profiles in context

A typical monitoring setup includes many profiles:

```
+-------------------+
|   SNMP Collector  |
|-------------------|
| 1. Load profiles  |
| 2. Discover device|
| 3. Match profiles |
| 4. Collect metrics|
+-------------------+
        |
        v
+------------------+
|  Netdata Charts  |
+------------------+
```

When the SNMP collector discovers a device, it evaluates all available profiles and applies **every profile** whose `selector` **rules match** that device.
This means:

- A device can match **multiple profiles**.
- The collector **merges** metrics, tags, and metadata from all of them.
- This enables _modular profile_ design:
    - One profile can define common metrics (e.g., `IF-MIB` for interfaces).
    - Another can define vendor-specific extensions (e.g., Cisco, Juniper).
    - Together, they produce a complete set of metrics for that device.

Profiles can also extend one another explicitly (see the `extends` key), but matching multiple profiles dynamically is the usual way to compose functionality.

## Profile Structure

```yaml
selector: <device matching pattern>
extends: <base profiles to include>
metadata: <device information>
metrics: <what to collect>
metric_tags: <global tags>
static_tags: <static tags>
virtual_metrics: <calculated metrics>
```

### 1. selector

The `selector` decides **which devices this profile applies to**. Netdata evaluates all profiles; any profile whose selector matches a device is **applied and merged** with others.

**Example**:

```yaml
selector:
  - sysobjectid:
      include: ["1.3.6.1.4.1.9.*"]     # regex: Cisco enterprise OID subtree
      exclude: ["1.3.6.1.4.1.9.9.666"]  # optional excludes
    sysdescr:
      include: ["IOS"]                  # substring (case-insensitive)
      exclude: ["emulator", "lab"]      # optional excludes
```

- A profile is applied to a device if **at least one** of its `selector` rules matches the device.
- Each selector rule is a **set of conditions**.
- For the rule to match, **all defined conditions must pass**.
- If both `sysobjectid` and `sysdescr` blocks are present, both must succeed.

Here is a breakdown of the individual conditions:

| Condition Key         | What It Checks                       | Rule to **Pass** (Success)                             | Rule to **Fail** (Immediate)        |
|-----------------------|--------------------------------------|--------------------------------------------------------|-------------------------------------|
| `sysobjectid.include` | Device `sysObjectID`                 | **Must match at least one** item in the list.          | Fails if _no_ items match.          |
| `sysobjectid.exclude` | Device `sysObjectID`                 | **Must not match any** item in the list.               | Fails if _any_ item matches.        |
| `sysdescr.include`    | Device `sysDescr` (case-insensitive) | **Must contain at least one** substring from the list. | Fails if _no_ substrings are found. |
| `sysdescr.exclude`    | Device `sysDescr` (case-insensitive) | **Must not contain any** substring from the list.      | Fails if _any_ substring is found.  |

### 2. extends

Use `extends` to **reuse** and **compose** existing profiles instead of copy-pasting common metrics. Most real profiles extend a few base building blocks, then add device-specific bits.

**Example**:

```yaml
extends:
  - _system-base.yaml              # System basics (uptime, contact, location)
  - _std-if-mib.yaml        # Network interfaces (IF-MIB)
  - _std-ip-mib.yaml        # IP statistics
```

The final, effective profile is the **merge** of all inherited profiles **plus** the content in the current file.

**How inheritance works**:

1. **Order matters** - Profiles are loaded in the order listed
2. **Metrics are combined** - All metrics from all profiles are collected
3. **Later profiles override** - If the same field is defined in multiple profiles, the last one wins

**Common base profiles**:

| Profile             | Provides                            | Use For             |
|---------------------|-------------------------------------|---------------------|
| `_system_base.yaml` | System info (uptime, name, contact) | All devices         |
| `_std-if-mib.yaml`  | Interface statistics (IF-MIB)       | Network devices     |
| `_std-ip-mib.yaml`  | IP statistics (IP-MIB)              | Routers/L3 switches |
| `_std-tcp-mib.yaml` | TCP statistics                      | Servers, firewalls  |
| `_std-udp-mib.yaml` | UDP statistics                      | Servers, firewalls  |
| `_std-ups-mib.yaml` | UPS metrics                         | UPS devices         |

### 3. metadata

The `metadata `section defines **descriptive information** about the device, such as vendor, model, or serial number.

This information is collected once per device and used to populate **device (virtual node) host labels** in Netdata.

**Example**:

```yaml
metadata:
  device:
    fields:
      vendor:
        value: "Cisco"
      model:
        symbol:
          OID: 1.3.6.1.2.1.47.1.1.1.1.2.1
          name: entPhysicalModelName
```

In this example:

- The` vendor` label is set statically to `Cisco`.
- The `model` label is retrieved from the SNMP object entPhysicalModelName.

The exact metadata syntax and field options will be covered in detail later in this document.

### 4. metrics

The `metrics` section defines **what data to collect** from the device — which OIDs to read, how to interpret them, and how to display them as charts in Netdata.

You can define **as many metrics as needed**, mixing both scalar and table types freely.

The collector automatically handles **SNMP GET** and **WALK** operations as appropriate.

There are two main types of metrics:

- **Scalar metrics** — single values (e.g., system uptime, CPU load).
- **Table metrics** — repeated rows (e.g., interfaces, disks, sensors).

**Examples**:

- Scalar metrics:
    ```yaml
    metrics:
      - name: sysUpTime
        symbol:
          OID: 1.3.6.1.2.1.1.3.0
          name: sysUpTime
        chart_meta:
          description: System uptime
          unit: seconds
    ```

- Table metrics:
    ```yaml
    metrics:
      - name: ifInOctets
        table:
          OID: 1.3.6.1.2.1.2.2
          name: ifTable
        symbols:
          - OID: 1.3.6.1.2.1.2.2.1.10
            name: ifInOctets
        metric_tags:
          - tag: interface
            column:
              OID: 1.3.6.1.2.1.31.1.1.1.1
              name: ifName
    ```

More detailed syntax, value modifiers, and tagging options will be covered later in this document.

### 5. metric_tags

The `metric_tags` section defines **global tags** — values collected once from the device and applied to **every metric** produced by the profile.

Global tags help you:

- Attach consistent metadata across all collected data.
- Group and filter metrics in Netdata dashboards and alerts.
- Complement tags defined inside individual metrics (for example, per-interface or per-sensor tags in tables).

**Example**:

```yaml
metric_tags:
  - OID: 1.3.6.1.4.1.14988.1.1.4.1.0
    symbol: mtxrLicSoftwareId
    tag: software_id
  - OID: 1.3.6.1.4.1.14988.1.1.4.4.0
    symbol: mtxrLicVersion
    tag: license_version
```

In this example:

- The tag `software_id` is populated from the SNMP object `mtxrLicSoftwareId`.
- The tag `license_version` is populated from `mtxrLicVersion`.

### 6. static_tags

The `static_tags` section defines **fixed key–value tags** that are attached to every metric produced by the profile.

They are useful for:

- Adding constant metadata such as environment, region, or service.
- Tagging all collected metrics with deployment-specific identifiers.
- Making it easier to group and filter data in dashboards and alerts.

**Example**:

```yaml
static_tags:
  - environment:production
  - region:us-east-1
  - service:network
```

In this example:

- Every metric collected from matching devices will include these static tags.
- Static tags are merged with any dynamic tags defined elsewhere (metric_tags, table tags, etc.).

### 7. virtual_metrics

The `virtual_metrics` section defines **calculated or combined metrics** — values derived from other metrics rather than collected directly from SNMP.

Virtual metrics are useful for:

- Combining related counters (for example, totals or aggregates).
- Creating fallbacks (for example, prefer 64-bit counters, but use 32-bit if unavailable).
- Summing or grouping metrics per interface, CPU, or other tags.

```yaml
  - name: ifTotalErrors
    sources:
      - { metric: _ifInErrors,  table: ifTable, as: in }
      - { metric: _ifOutErrors, table: ifTable, as: out }
    chart_meta:
      description: Total packets with errors across all interfaces
      family: 'Network/Total/Error'
      unit: "{error}/s"
```

In this example:

- The virtual metric `ifTotalErrors` combines two existing table metrics — inbound and outbound errors.
- Each source is identified by its metric name (`_ifInErrors`, `_ifOutErrors`) and the table it comes from (`ifTable`).
- The `as` field labels the dimensions (`in` and `out`) in the resulting chart.

Virtual metrics behave like regular metrics in Netdata — they can have `chart_meta`, appear in charts, and be used in alerts — but their values are **computed dynamically** based on other metrics collected by the profile.

## Collecting Metrics

This section explains how SNMP data is organized and how it maps to metrics in a profile.

---

### Understanding SNMP Data

SNMP data is organized as a **tree** of numeric addresses called **OIDs (Object Identifiers)** — similar to file paths.

```text
1.3.6.1.2.1.1.3.0
└─┬─┘ └──┬──┘ └┬┘
  │      │     └─ Instance (0 = scalar)
  │      └─ Object (3 = sysUpTime)
  └─ MIB-2 prefix
```

- **MIBs** are collections of OIDs grouped by purpose (e.g., `IF-MIB`, `HOST-RESOURCES-MIB`).
- Each OID points to a value: integer, counter, string, or status.
- Profiles refer to OIDs either directly by number or via their symbolic name.

### Scalar Metrics (Single Values)

Scalar metrics represent **a single value for the entire device**.

Their OIDs always end with `.0`.

```yaml
metrics:
  - name: sysUpTime
    symbol:
      OID: 1.3.6.1.2.1.1.3.0
      name: sysUpTime
```

Common examples include uptime, system name, temperature, and overall status.

### Table Metrics (Multiple Rows)

Table metrics represent **lists of related values** — one row per interface, disk, or sensor.

Each row is identified by an **index**, such as `.1`, `.2`, `.3`.

**Note**: Table data MUST have tags to identify each row in Netdata. Without tags, you can't tell which metric belongs to which interface/CPU/disk

```yaml
metrics:
  - name: ifInOctets
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
    metric_tags:
      - tag: interface
        column:
          OID: 1.3.6.1.2.1.31.1.1.1.1
          name: ifName
```

### Metric Types

Netdata needs to know **how to interpret each metric’s value**.
The type determines how the value is processed and displayed (e.g., as an instantaneous gauge or a per-second rate).

Most of the time, the type is **detected automatically** from the SNMP data type, but you can override it if necessary.

**Automatic Type Detection**:

| **SNMP Type**            | **Default Netdata Type** | **Typical Use**                         |
|--------------------------|--------------------------|-----------------------------------------|
| `Counter32`, `Counter64` | `rate`                   | Network traffic, packet counters        |
| `Gauge32`, `Integer`     | `gauge`                  | Temperatures, CPU usage, current values |
| `TimeTicks`              | `gauge`                  | Uptime, time-based values               |

**Overriding the Metric Type**:

You can explicitly set a metric type using the metric_type field inside a symbol definition.

```yaml
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
        metric_type: gauge  # Override default 'rate'
```

This example forces `ifInOctets` to be treated as a **gauge**, even though it’s an SNMP counter (normally converted to a per-second rate).

## Adding Tags to Metrics

Tags help **organize and filter metric**s in Netdata.

They **identify specific instances** of metrics (for example, which interface or disk a value belongs to) and enable grouping and filtering in the UI.

**Key Concepts**:

- **Table metrics must have tags** — Tags uniquely identify each row (instance). Without tags, table metrics cannot be created.
- **Scalar metrics don’t need tags** — They represent a single value for the entire device.
- **Static tags** apply the same value to all metrics.
- **Dynamic tags** extract values from the device via SNMP.
- **Global tags** (defined in `metric_tags` at the top level) apply to all metrics in the profile.

**Tag Types and Transformations**:

| **Tag Type**    | **Description**                                      | **Available Transformations**                   |
|-----------------|------------------------------------------------------|-------------------------------------------------|
| **Static**      | Fixed values that never change.                      | None (fixed value).                             |
| **Same-Table**  | Values from columns in the same table as the metric. | Mapping, Pattern, Extract Value, Match Pattern. |
| **Cross-Table** | Values from a different table.                       | Mapping, Pattern, Extract Value, Match Pattern. |
| **Row Index**   | Values derived from the SNMP table row index.        | Mapping only.                                   |

**Important**: Only one transformation is applied per tag, in the order shown above. If no transformation applies, the raw value is used.

### Static

Static tags are fixed values that never change.

#### Profile-level static tags

Applied to ALL metrics in the profile:

```yaml
# Global static tags (all metrics)
static_tags:
  - tag: datacenter
    value: "DC1"
  - tag: environment
    value: "production"
```

#### Metric-level static tags

Applied only to specific metrics:

```yaml
# Metric-specific static tags
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
    static_tags:
      - tag: "source"
        value: "snmp"
      - tag: "interface_type"
        value: "physical"
```

### Same-Table

Same-table tags extract values from columns in the same SNMP table as the metric.

```yaml
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
    metric_tags:
      - tag: interface
        symbol:
          OID: 1.3.6.1.2.1.2.2.1.2
          name: ifDescr
```

Produces metrics like: `ifInOctets{interface="eth0"} = 1000`.

### Cross-Table

Cross-table tags allow you to **use data from another SNMP table** as a tag source.

#### Same Index

Pull tag values from another table:

```yaml
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
    metric_tags:
      - tag: interface
        table: ifXTable
        symbol:
          OID: 1.3.6.1.2.1.31.1.1.1.1
          name: ifName
```

#### With Index Transformation

This is common when related tables (for example, IP and interface tables) use **different index structures**.

```yaml
metrics:
  - MIB: IP-MIB
    table:
      OID: 1.3.6.1.2.1.4.31.3
      name: ipIfStatsTable
    symbols:
      - OID: 1.3.6.1.2.1.4.31.3.1.6
        name: ipIfStatsHCInOctets
        chart_meta:
          description: Total inbound IP octets (including errors)
          family: 'Network/Interface/IP/Traffic/Total/In'
          unit: "bit/s"
        scale_factor: 8
      - OID: 1.3.6.1.2.1.4.31.3.1.33
        name: ipIfStatsHCOutOctets
        chart_meta:
          description: Total outbound IP octets
          family: 'Network/Interface/IP/Traffic/Total/Out'
          unit: "bit/s"
        scale_factor: 8
    metric_tags:
      - tag: _interface
        table: ifXTable
        symbol:
          OID: 1.3.6.1.2.1.31.1.1.1.1
          name: ifName
        index_transform:
          - start: 1
            end: 1
```

In this example, the `ipIfStatsTable` uses an **interface index** that includes multiple components, while `ifXTable` uses a simpler index (just the interface number).
The `index_transform` block extracts only the portion of the index needed to align the two tables.

##### How `index_transform` Works

SNMP tables often use **different index formats**.

For example, one table might index rows by interface number, while another adds extra parts like address family or IP address.

`index_transform` tells the collector **which parts of the index to keep** when matching rows between tables.

Think of it as **“cutting out”** a portion of the index from the current table so it lines up with the target table.

**Example**:

- Index from `ipIfStatsTable`: `2.4.192.168.1.10`
- Index from `ifXTable`: `2`
- To match them, keep only the first number:
    ```yaml
      index_transform:
      - start: 1
        end: 1
    ```
    - Now the tag lookup uses index `2` to find the right row in `ifXTable`.

**In short**:

- `start` and `end` define which parts of the index to keep (counting from 1).
- You can list multiple ranges to join non-contiguous parts.
- The goal is simply to make the current table’s index **look like** the target table’s index so the tag matches correctly.

### Row Index

Sometimes the SNMP table index itself is meaningful (e.g., compound identifiers):

```yaml
metrics:
  - MIB: CISCO-FIREWALL-MIB
    table:
      OID: 1.3.6.1.4.1.9.9.147.1.2.2.2
      name: cfwConnectionStatTable
    symbols:
      - OID: 1.3.6.1.4.1.9.9.147.1.2.2.2.1.5
        name: cfwConnectionStatValue
    metric_tags:
      - index: 1  # first index part
        tag: service_type
      - index: 2  # second index part
        tag: stat_type
```

This produces metrics like: `cfwConnectionStatValue{service_type="80", stat_type="bytes"} = 123456`.

## Tag Transformations

Tag transformations let you **modify or extract parts of SNMP values** to create meaningful, human-readable tags.

They are most often used with table columns or cross-table tags to normalize raw data.

**Available Tag Transformations**:

| **Transformation**              | **Purpose**                                        | **Example Input → Output**                                |
|---------------------------------|----------------------------------------------------|-----------------------------------------------------------|
| `mapping`                       | Convert numeric or string codes to readable names. | `1 → up`                                                  |
| `extract_value`                 | Extract a substring using regex.                   | `"eth0" → "0"`                                            |
| `match_pattern` + `match_value` | Rebuild a string from regex groups.                | `"Version 15.2.4" → "15.2"`                               |
| `pattern`                       | Create multiple tags from one string.              | `"GigabitEthernet1/0/24" → type=GigabitEthernet, port=24` |
| `format`                        | Apply predefined data conversions.                 | Raw MAC → `AA:BB:CC:DD:EE:FF`                             |

### With Mapping

Use mapping to convert raw SNMP values (usually integers) into descriptive strings:

- You want to replace numeric codes with readable names.
- You need to standardize tag values across vendors or devices.

```yaml
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.7
        name: ifAdminStatus
        chart_meta:
          description: Current administrative state of the interface
          family: 'Network/Interface/Status/Admin'
          unit: "{status}"
        mapping:
          1: up
          2: down
          3: testing
```

Also works for string-to-string mappings:

```yaml
mapping:
  "GigabitEthernet": "gbe"
  "TenGigabitEthernet": "10gbe"
  "FastEthernet": "fe"
```

### With Extract Value

Use `extract_value` to capture a part of a string using a **regular expression**.

```yaml
metadata:
  device:
    fields:
      model:
        symbols:
          # RouterOS model extraction (e.g. 'RouterOS CCR2004-16G-2S+' => 'CCR2004-16G-2S+')
          - OID: 1.3.6.1.2.1.1.1.0
            name: sysDescr
            extract_value: 'RouterOS ([A-Za-z0-9-+]+)'
```

### With Match Pattern

Use `match_pattern` and `match_value` together to build a tag value using multiple **regex capture groups**.

```yaml
metric_tags:
  - tag: device_version
    symbol:
      OID: 1.3.6.1.2.1.1.1.0
      name: sysDescr
      match_pattern: 'Version (\d+)\.(\d+)\.(\d+)'
      match_value: '$1.$2'  # Produces "15.2" from "Version 15.2.4"
```

- `$1`, `$2`, `$3` refer to regex capture groups in match_pattern.
- Useful for **reformatting strings** (like firmware versions, model names, or build numbers).

### With Pattern (Multiple Tags)

Use `pattern` and `tags` to create multiple tags from one SNMP value.

```yaml
metric_tags:
  - symbol:
      OID: 1.3.6.1.2.1.2.2.1.2
      name: ifDescr
    pattern: '(\w+?)(\d+)/(\d+)/(\d+)'
    tags:
      interface_type: $1  # "GigabitEthernet"
      module: $2          # "1"
      slot: $3            # "0"
      port: $4            # "24"
```

For example, `GigabitEthernet1/0/24` becomes: `interface_type=gigabitEthernet`, `module=1`, `slot=0`, `port=24`.

**Use pattern when**:

- One SNMP field encodes several useful pieces of information.
- You need multiple tags extracted from a single value.

### Format Conversions

Use `format` for predefined data conversions — the collector automatically translates binary or encoded data into readable formats.

```yaml
metric_tags:
  - tag: mac_address
    symbol:
      OID: 1.3.6.1.2.1.2.2.1.6
      name: ifPhysAddress
      format: mac_address  # Converts bytes → "AA:BB:CC:DD:EE:FF"

  - tag: ip_address
    symbol:
      OID: 1.3.6.1.4.1.9.9.500.1.2.1.1.4
      name: cdpCacheAddress
      format: ip_address  # Converts bytes → "192.168.1.1"
```

Supported formats include:

- `mac_address`
- `ip_address`

## Value Transformations

Value transformations process **SNMP values into numeric metrics**.

While tag transformations produce text labels, value transformations must ultimately yield **numbers** that Netdata can graph, alert on, and aggregate.

**Key Concepts**:

- Transformations are applied in order: `Extract / Match → Mapping → Scaling`.
- String values are auto-converted: If a string looks like a number (e.g., `"123"`), no extra parsing is needed.
- Only one string transformation per value: Use either `extract_value` or `match_pattern` — not both.
- Scaling is always last: Applied after all string processing and mappings.

**Available Value Transformation**:

| **Transformation** | **Purpose**                          | **Input Type** | **Example Use Case**                  |
|--------------------|--------------------------------------|----------------|---------------------------------------|
| `extract_value`    | Extract number from a string         | String         | `"25C"` → `25`                        |
| `match_pattern`    | Parse and rebuild strings with regex | String         | `"Version 15.2.4"` → `"15.2"` → `152` |
| `mapping`          | Map specific values to numbers       | Any            | `"onBattery"` → `2`                   |
| `scale_factor`     | Convert units                        | Numeric        | `KB → bytes (×1024)`                  |

### With Extract Value

- Use **extract_value** to pull numeric parts out of strings.
- The **first regex capture group** becomes the metric value.

```yaml
symbols:
  - OID: 1.3.6.1.4.1.232.6.2.6.7.1.3.1.4
    name: cpuTemperature
    extract_value: '(\d+)C'      # "25C" → 25
    unit: "celsius"

  - OID: 1.3.6.1.4.1.12124.1.13.1.3
    name: fanSpeed
    extract_value: '(\d+)\s*RPM' # "1200 RPM" → 1200
    unit: "rpm"

  - OID: 1.3.6.1.4.1.2.3.51.2.2.7.1.0
    name: batteryVoltage
    extract_value: '(\d+\.\d+)V' # "13.6V" → 13.6
    unit: "volts"
```

### With Match Pattern

- Use `match_pattern` and `match_value` for **complex string parsing**.
- This allows multiple capture groups and string reconstruction before mapping or scaling.

```yaml
symbols:
  - OID: 1.3.6.1.2.1.1.1.0
    name: version_number
    match_pattern: 'Version (\d+)\.(\d+)\.(\d+)'
    match_value: '$1$2'  # "Version 15.2.4" → "152"
    mapping:
      "152": "152"  # Major version 15.2
      "160": "160"  # Major version 16.0
```

**Note**: The final result must still resolve to a **number** — directly or through a mapping.

### With Mapping

- Use `mapping` to convert specific string or numeric values into numbers.
- It’s applied **after** extraction/matching, but **before** scaling.

```yaml
symbols:
  - OID: 1.3.6.1.4.1.318.1.1.1.4.1.1.0
    name: upsStatus
    mapping:
      "onLine": "1"       # Normal operation
      "onBattery": "2"    # Running on battery
      "onBypass": "3"     # In bypass mode
      "off": "0"          # Powered off
```

Mappings are also used to standardize vendor-specific string states across devices.

### With Scale Factor

- Use `scale_factor` to **unit conversions** (e.g., KB to bytes, seconds to milliseconds) or **precision adjustments**.
- It’s applied **last**, after all string processing and mappings.

```yaml
symbols:
  # Memory/Storage conversions
  - OID: 1.3.6.1.4.1.9.9.48.1.1.1.5.1
    name: memoryUsed
    scale_factor: 1024    # KB → bytes
    unit: "bytes"

  # Temperature conversion
  - OID: 1.3.6.1.4.1.9.9.13.1.3.1.3
    name: temperature
    scale_factor: 0.1     # Tenths → degrees
    unit: "celsius"

  # Percentage conversion
  - OID: 1.3.6.1.4.1.2.3.51.2.22.1.5.1.1.4
    name: cpuUsage
    scale_factor: 0.01    # Hundredths → percent
    unit: "percent"

  # Power conversion
  - OID: 1.3.6.1.4.1.318.1.1.1.12.2.3.1.1.2
    name: power
    scale_factor: 10      # Tens of watts → watts
    unit: "watts"
```

### Combining Transformations

Transformations can be combined, and are applied in this order:

1. **Extract Value** OR **Match Pattern** → string processing
2. **Mapping** → normalization to numeric codes
3. **Scale Factor** → numeric unit conversion

```yaml
symbols:
  # Example 1: Temperature sensor returns "temp: 250 tenths"
  - OID: 1.3.6.1.4.1.12345.1.1.1
    name: sensor_temperature
    extract_value: 'temp: (\d+)'  # Step 1: extract "250"
    scale_factor: 0.1             # Step 2: 250 → 25.0
    unit: "celsius"

  # Example 2: String state mapped to a score
  - OID: 1.3.6.1.4.1.12345.1.1.2
    name: device_health_score
    mapping:
      "healthy": "100"
      "degraded": "50"
      "failed": "0"
    scale_factor: 0.01            # Step 2: convert to ratio
    unit: "ratio"
```

## Virtual Metrics

- Virtual metrics are **calculated metrics** built from other metrics in your profile (or inherited ones).
- They don’t query SNMP; they **reuse existing metric values** to create totals, fallbacks, or per-row aggregations.
- Once computed, they behave like normal metrics: charted, tagged, and alertable.

Common use cases:

- **Fallbacks**: prefer 64-bit counters, fall back to 32-bit if missing.
- **Sums/Combines**: add related metrics (e.g., in + out traffic).
- **Per-row totals**: aggregate multiple columns into one per-interface metric.

### Structure

```yaml
virtual_metrics:
  - name: <string>
    sources:
      - { metric: <metricName>, table: <tableName>, as: <dimensionName> }
      # Optional: direct primary source set

    alternatives:
      - sources:
          - { metric: <metricNameA>, table: <tableName>, as: <dimensionName> }
          - { metric: <metricNameB>, table: <tableName>, as: <dimensionName> }
      - sources:
          - { metric: <fallbackMetricA>, table: <tableName>, as: <dimensionName> }
          - { metric: <fallbackMetricB>, table: <tableName>, as: <dimensionName> }

    per_row: <true|false>
    group_by: <label | [labels]>
    chart_meta:
      description: ...
      family: ...
      unit: ...
```

### Config reference

#### Virtual metric item

| Field        | Type                 | Required | Default | Applies to               | Notes                                                                               |
|--------------|----------------------|----------|---------|--------------------------|-------------------------------------------------------------------------------------|
| name         | string               | yes      | —       | all                      | Unique within the profile. Used as metric/chart base name.                          |
| sources      | array\<Source\>      | no*      | —       | totals, per_row, grouped | Direct source set. Ignored if `alternatives` exists (alternatives take precedence). |
| alternatives | array\<Alternative\> | no*      | —       | totals, per_row, grouped | Ordered fallback sets. First alternative whose sources produce data wins.           |
| per_row      | bool                 | no       | false   | per-row/grouped          | `true` → one output per input row; sources become dimensions; row tags attach.      |
| group_by     | string               | array    | no      | —                        | per-row/grouped                                                                     | Label(s) used as row-key hints (in order). Missing/empty hints fall back to a full-tag stable key. With `per_row:false`, this is a PromQL-like “sum by (…)”. |
| chart_meta   | object               | no       | —       | all                      | Presentation only (`description`, `family`, `unit`, `type`).                        |

> * At least one of sources or alternatives must be provided.

#### Source object

| Field  | Type   | Required | Notes                                                                                               |
|--------|--------|----------|-----------------------------------------------------------------------------------------------------|
| metric | string | yes      | Name of a previously collected metric (scalar or table column metric).                              |
| table  | string | yes      | Table name for the originating metric (must match the metric’s table when used in per-row/grouped). |
| as     | string | yes      | Dimension name in the composite (e.g., `in`, `out`).                                                |

#### Alternative

| Field   | Type            | Required | Notes                                                                                                                                                                                   |
|---------|-----------------|----------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| sources | array\<Source\> | yes      | All sources in an alternative are evaluated together. If **none** produce data, the collector tries the next alternative. Per-row/group rules apply **within** the winning alternative. |

#### Rules & Constraints

| Rule                              | Description                                                                                                                                            |
|-----------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Precedence**                    | If both `sources` and `alternatives` exist, `alternatives` take precedence.                                                                            |
| **Same-table requirement**        | When `per_row` or `group_by` is used, all sources must originate from the same table. For alternatives, this rule applies within each alternative set. |
| **per_row: true**                 | One output per input row; multiple sources become chart dimensions (`as`); row tags attach automatically.                                              |
| **group_by (with per_row:true)**  | Acts as row-key hints (in order). Missing or empty hints fall back to a full-tag composite key.                                                        |
| **group_by (with per_row:false)** | Aggregates rows by the listed labels, similar to PromQL’s `sum by (...)`.                                                                              |
| **Alternative evaluation**        | Alternatives are checked in order. The first whose sources produce data becomes the “winner”; others are ignored.                                      |
| **Parent metadata**               | The virtual metric emits charts using its own `name` and `chart_meta`, even when data comes from an alternative.                                       |
| **Dimensions**                    | Each `as` value defines a dimension in the resulting chart (e.g., `in`, `out`, `total`).                                                               |
| **Totals vs per-row**             | Omitting both `per_row` and `group_by` produces a single total chart across all rows (device-wide view).                                               |

### Examples

#### Per-row aggregation from one table (in + out traffic)

```yaml
virtual_metrics:
  - name: ifTotalTraffic
    sources:
      - { metric: _ifHCInOctets,  table: ifXTable, as: in }
      - { metric: _ifHCOutOctets, table: ifXTable, as: out }
    per_row: true
    group_by: ["interface"]
    chart_meta:
      description: Traffic per interface
      family: 'Network/Interface/Traffic'
      unit: "bit/s"
```

**What this does**:

- Creates **one output per input row** in `ifXTable`.
- Each chart represents one interface with two dimensions: `in` and `out`.
- `group_by: ["interface"]` provides key hints to keep per-interface charts stable.
- If a hint is missing or empty, a full-tag composite key is used instead.
- **Constraint**: `per_row` or `group_by` requires all sources to come from the same table.

#### Total aggregation (sum across all interfaces)

```yaml
virtual_metrics:
  - name: ifTotalTraffic
    sources:
      - { metric: _ifHCInOctets,  table: ifXTable, as: in }
      - { metric: _ifHCOutOctets, table: ifXTable, as: out }
    chart_meta:
      description: Total traffic across all interfaces
      family: 'Network/Total/Traffic'
      unit: "bit/s"
```

**What this does**:

- Aggregates data from **all rows** in `ifXTable` into a **single chart**.
- Produces two dimensions (`in`, `out`) representing the total interface traffic for the entire device.
- No `per_row` or `group_by` fields → a single total chart (device-wide view).

#### Grouped aggregation (sum by label)

```yaml
virtual_metrics:
  - name: ifTypeTraffic
    sources:
      - { metric: _ifHCInOctets,  table: ifXTable, as: in }
      - { metric: _ifHCOutOctets, table: ifXTable, as: out }
    per_row: false
    group_by: ["ifType"]
    chart_meta:
      description: Traffic aggregated by interface type
      family: 'Network/InterfaceType/Traffic'
      unit: "bit/s"
```

What this does

- Performs **PromQL-like “sum by (ifType)” aggregation**.
- Combines all rows sharing the same `ifType` label into grouped totals.
- Result: one chart with `in` and `out` dimensions aggregated by interface type.
- **Constraint**: all sources must come from the same table.

#### Alternatives (total; prefer 64-bit, fallback to 32-bit)

```yaml
virtual_metrics:
  - name: ifTotalPacketsUcast
    alternatives:
      - sources:
          - { metric: _ifHCInUcastPkts,  table: ifXTable, as: in }
          - { metric: _ifHCOutUcastPkts, table: ifXTable, as: out }
      - sources:
          - { metric: _ifInUcastPkts,  table: ifTable, as: in }
          - { metric: _ifOutUcastPkts, table: ifTable, as: out }
    chart_meta:
      description: Total unicast packets across all interfaces (in/out)
      family: 'Network/Total/Packet/Unicast'
      unit: "{packet}/s"
```

**What this does**:

- Defines two **alternatives**, each as a list of sources.
- At runtime, the collector **picks the first alternative whose sources produce data** (HC first).
- Once a winner is found, **later alternatives are ignored**.
- The parent emits metrics using **its own** `name` and `chart_meta`, sourcing values from the selected child.
- If both `sources` and `alternatives` are present, `alternatives` take precedence.

#### Composite (multi-source total: unicast/multicast/broadcast

```yaml
virtual_metrics:
  - name: ifTotalPacketsByKind
    sources:
      - { metric: _ifHCInUcastPkts,      table: ifXTable, as: in_ucast }
      - { metric: _ifHCOutUcastPkts,     table: ifXTable, as: out_ucast }
      - { metric: _ifHCInMulticastPkts,  table: ifXTable, as: in_mcast }
      - { metric: _ifHCOutMulticastPkts, table: ifXTable, as: out_mcast }
      - { metric: _ifHCInBroadcastPkts,  table: ifXTable, as: in_bcast }
      - { metric: _ifHCOutBroadcastPkts, table: ifXTable, as: out_bcast }
    chart_meta:
      description: Total packets across all interfaces by kind (in/out)
      family: 'Network/Total/Packet/ByKind'
      unit: "{packet}/s"
```

What this does

- Builds a **single total chart** combining multiple related packet counters.
- Each `as` becomes a **dimension** (`in_ucast`, `out_ucast`, `in_mcast`, …).
- No `per_row`/`group_by` → totals aggregated across all interfaces.
