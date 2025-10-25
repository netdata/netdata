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

## Tag Transformation

Tag transformations let you **modify or extract parts of SNMP values** to produce clear, human-readable tags.

They work the same in **both** places:

- `metadata` (e.g., device model, OS name), and
- `metric_tags` (e.g., per-row interface labels).

**Available Tag Transformations**:

| Transformation                   | Purpose                                         | Example Input → Output                                           |
|----------------------------------|-------------------------------------------------|------------------------------------------------------------------|
| `mapping`                        | Replace numeric/string codes with names.        | `1 → "ethernet"`, `161 → "lag"`                                  |
| `extract_value`                  | Extract a substring via regex (first group).    | `"RouterOS CCR2004-16G-2S+" → "CCR2004-16G-2S+"`                 |
| `match_pattern` + `match_value`  | Replace the value using regex groups or static. | `"Palo Alto Networks VM-Series firewall" → "VM-Series firewall"` |
| `match` + `tags` (multiple tags) | Create **several** tags from one value.         | `"xe-0/0/1" → if_family=xe, fpc=0, pic=0, port=1`                |

**Combination & Behavior**:

| Rule                        | Description                                                                                                                                   |
|-----------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| **Where**                   | Can be used inside `metadata.*.fields.*.symbols[]` and `metric_tags[]`.                                                                       |
| **Order of application**    | 1️⃣ `match_pattern` + `match_value` **or** `extract_value` (whichever is present) → 2️⃣ `mapping` → 3️⃣ `match` + `tags` (if defined).        |
| **No match behavior**       | • `extract_value`: keeps the original value.<br/>• `match_pattern`: skips the value (tag not emitted).<br/>• `match` + `tags`: emits no tags. |
| **Multiple symbols**        | If multiple `symbols` are listed for the same tag, the **first non-empty result** is used.                                                    |
| **Mapping key consistency** | Keys in a `mapping` must all be the same type — all numeric or all string.                                                                    |
| **Safety**                  | Keep regexes simple and, when possible, **anchor them** (e.g. `^pattern$`) to prevent unwanted matches.                                       |

**Quick Syntax Recap**:

- `mapping`
    ```yaml
    mapping:
      6: "ethernet"
      161: "lag"
    ```
- `extract_value`
    ```yaml
     extract_value: 'RouterOS ([A-Za-z0-9-+]+)'   # first capture group is used
     ```

- `match_pattern` + `match_value`
    ```yaml
    match_pattern: 'Palo Alto Networks\s+(PA-\d+ series firewall|VM-Series firewall)'
    match_value: '$1'   # or a static value like 'Router' when matched
    ```
- `match` + `tags` (multiple tags)
    ```yaml
    match: '^([A-Za-z]+)[-_]?(\d+)\/(\d+)\/(\d+)$'
    tags:
      if_family: $1
      fpc: $2
      pic: $3
      port: $4
    ```

### Mapping

Use `mapping` to replace raw tag values with **human-readable text labels**.

**The collector**:

- Looks up the raw value in the mapping table.
- Replaces it with the corresponding string.
- If the value is not found in the mapping, the **original value** is **kept**.
- Keys can be numeric or string, but must be consistent in type.
- Mapping is applied to tag values from `metadata` or `metric_tags`.

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
      - tag: if_type
        symbol:
          OID: 1.3.6.1.2.1.2.2.1.3
          name: ifType
        mapping:
          1: "other"
          6: "ethernet"
          24: "loopback"
          131: "tunnel"
          161: "lag"
```

**What this does**:

- Replaces numeric interface type codes (1, 6, 24, 131, 161) with readable names (`other`, `ethernet`, `loopback`, `tunnel`, `lag`).
- If a device reports an unknown type, the original numeric value is used.
- Works identically for `metadata` fields and `metric_tags`.

### Extract Value

Use `extract_value` to capture a part of a string using a **regular expression**.

**The collector**:

- Applies the pattern to the raw value.
- Replaces the value with the **first capture group** `( … )`.
- Keeps the **original value** if no match is found.
- Searches anywhere in the string unless you anchor the pattern with `^` or `$`.
- Uses the **first non-empty** result when multiple `symbols` are defined.

```yaml
metadata:
  device:
    fields:
      model:
        symbols:
          # Example: 'RouterOS CCR2004-16G-2S+' → 'CCR2004-16G-2S+'
          - OID: 1.3.6.1.2.1.1.1.0
            name: sysDescr
            extract_value: 'RouterOS ([A-Za-z0-9-+]+)'
          # Example: 'CSS326-24G-2S+ SwOS v2.13' → 'CSS326-24G-2S+'
          - OID: 1.3.6.1.2.1.1.1.0
            name: sysDescr
            extract_value: '([A-Za-z0-9-+]+) SwOS'
```

### Match Pattern

Use `match_pattern` and `match_value` together to build a tag value using multiple **regex capture groups**.

**The collector**:

- Tests the value against the regular expression in `match_pattern`.
- If it **matches**, replaces the value with `match_value`.
- Within `match_value`, you can reference **capture groups** using `$1`, `$2`, `$3`, etc.
- If the value **does not match**, it is skipped (ignored).
- Works both for reformatting captured text and for assigning a static replacement when matched.

**Example 1 — Reformat using capture groups**:

```yaml
metadata:
  device:
    fields:
      product_name:
        symbol:
          OID: 1.3.6.1.2.1.1.1.0
          name: sysDescr
          match_pattern: 'Palo Alto Networks\s+(PA-\d+ series firewall|WildFire Appliance|VM-Series firewall)'
          match_value: "$1"
          # Examples:
          #  - Palo Alto Networks VM-Series firewall  →  VM-Series firewall
          #  - Palo Alto Networks PA-3200 series firewall  →  PA-3200 series firewall
          #  - Palo Alto Networks WildFire Appliance  →  WildFire Appliance
```

**Example 2 — Assign static value on match**:

```yaml
metadata:
  device:
    fields:
      type:
        symbols:
          - OID: 1.3.6.1.2.1.1.1.0
            name: sysDescr
            # RouterOS devices
            match_pattern: 'RouterOS (CCR.*)'
            match_value: 'Router'
```

### Match (Multiple Tags)

Use `match` and `tags` to create **multiple tags** from a single SNMP value using a **regular expression** with capture groups.

**The collector**:

- Applies the regex in match to the raw value.
- If it `matches`, creates all tags listed under `tags`, substituting `$1`, `$2`, `$3`, etc. from the capture groups.
- If it **doesn’t match**, none of those tags are added.
- Tags that resolve to an **empty capture** are not added.

**Example 1 — Split OS name and model from sysDescr (metadata)**:

```yaml
metadata:
  device:
    fields:
      type:
        symbols:
          - OID: 1.3.6.1.2.1.1.1.0
            name: sysDescr
            match: '^(\S+)\s+(.*)$'
            tags:
              os_name: $1    # e.g. 'RouterOS'
              model: $2      # e.g. 'CCR2004-16G-2S+'
```

> Input like `RouterOS CCR2004-16G-2S+` becomes: `os_name=RouterOS`, `model=CCR2004-16G-2S+`.

**Example 2 — Derive multiple labels from interface names (metric_tags)**:

```yaml
metric_tags:
  - symbol:
      OID: 1.3.6.1.2.1.2.2.1.2
      name: ifDescr
    match: '^([A-Za-z]+)[-_]?(\d+)\/(\d+)\/(\d+)$'
    tags:
      if_family: $1   # e.g. 'xe' or 'ge' or 'GigabitEthernet' → 'GigabitEthernet'
      fpc: $2         # '0'
      pic: $3         # '0'
      port: $4        # '1'
```

- Handles common patterns like `xe-0/0/1`, `ge-0/0/0`, or `GigabitEthernet1/0/24`.
- Output tags might be: `if_family=xe`, `fpc=0`, `pic=0`, `port=1`.

## Value Transformation

Value transformations let you **process or normalize raw SNMP metric values** before they are stored and charted.

They are applied **per symbol (per OID)** during SNMP data collection. They modify only **metric values**, not tags or metadata, and are **not applied to virtual metrics**.

These transformations are typically used to:

- Extract numeric substrings from mixed strings.
- Scale or convert units (bytes → bits, megabits → bits).
- Map discrete states (1 = up, 2 = down, etc.) into named dimensions.

**Available Value Transformations**:

| Transformation                  | Purpose                                                           | Example Input → Output              |
|---------------------------------|-------------------------------------------------------------------|-------------------------------------|
| `mapping`                       | Convert numeric or string codes into state dimensions.            | `1 → up`, `2 → down`, `3 → testing` |
| `extract_value`                 | Extract a numeric substring via regex.                            | `"23.8 °C" → "23"`                  |
| `scale_factor`                  | Multiply values by a constant to adjust units.                    | `"1.5" (MBps) × 8 → 12 (Mbps)`      |
| `match_pattern` + `match_value` | *Not applicable* for metric values (use `extract_value` instead). | —                                   |

**Combination & Behavior**:

| Rule                      | Description                                                                                                               |
|---------------------------|---------------------------------------------------------------------------------------------------------------------------|
| **Where**                 | Value transformations are used inside `metrics[*].symbol` or `metrics[*].symbols[]`.                                      |
| **Order of application**  | 1️⃣ `extract_value` (if present) → 2️⃣ `mapping` → 3️⃣ `scale_factor`.                                                    |
| **Scale factor position** | `scale_factor` is always applied **last**, after all other transformations.                                               |
| **Data type handling**    | Transformations preserve numeric type (integer/float) unless the mapping converts it to a multi-value metric.             |
| **Error handling**        | If a transformation fails (e.g., regex doesn’t match), the collector keeps the original value.                            |
| **Applicability**         | Transformations affect metric values only — not metadata or tags.                                                         |
| **Mapping behavior**      | Always produces a multi-value metric where each mapped entry becomes a dimension; the active one reports `1`, others `0`. |

**Quick Syntax Recap**:

- `mapping`
    ```yaml
    mapping:
      1: up
      2: down
      3: testing
     ``` 

- `extract_value`
    ```yaml
    extract_value: '(\d+)'   # First capture group is used
    ```

- `scale_factor`
    ```yaml
    scale_factor: 8   # Octets → bits
    ```

### Mapping

Use `mapping` to convert raw metric values into **state dimensions**.

Each mapping entry defines a **dimension name** and the numeric or string value that triggers it.

**The collector**:

- Evaluates the value against the mapping table.
- For each mapping entry, creates a **dimension** named after the mapped key.
- Sets that dimension to `1` if the current value matches the key, or `0` otherwise.
- If the value doesn’t match any key, all mapped dimensions are `0`.
- Works only for **metric values**, not for tags or metadata.

```yaml
metrics:
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

**What this does**:

- Converts SNMP integer values (1, 2, 3) into a **multi-value metric** with dimensions `up`, `down`, and `testing`.
- The dimension corresponding to the current value reports `1`; all others report `0`.

### Extract Value

Use `extract_value` to extract a **numeric or string portion** from the raw SNMP value using a **regular expression**.

This is often used when a metric is encoded as a string that contains numeric data (e.g. `"23.8 °C"`).

**The collector**:

- Applies the regular expression to the raw SNMP value.
- Uses the **first capture group** `( … )` as the new metric value.
- If the pattern doesn’t match, the original value is kept.
- Works for any metric type (string or numeric).
- When multiple `symbols` are defined, the first non-empty result is used.

```yaml
metrics:
  - MIB: CORIANT-GROOVE-MIB
    table:
      OID: 1.3.6.1.4.1.42229.1.2.3.1.1
      name: shelfTable
    symbols:
      - OID: 1.3.6.1.4.1.42229.1.2.3.1.1.1.3
        name: coriant.groove.shelfInletTemperature
        # Example: "23.8 °C" → "23"
        extract_value: '(\d+)'
        chart_meta:
          description: Shelf inlet temperature
          family: 'Hardware/Shelf/Temperature/Inlet'
          unit: "Cel"
```

**What this does**:

- Applies the regex `(\d+)` to the string `"23.8 °C"`.
- Extracts only the numeric part `"23"` and uses it as the metric value.
- If the value doesn’t match, the original string is retained.
- Ideal for string metrics that embed numbers, units, or labels.

### Scale Factor

Use `scale_factor` to **multiply collected metric values** by a constant.

This transformation is typically used to convert between units (for example, bytes to bits).

**The collector**:

- Multiplies the raw SNMP value by the specified factor.
- Applies to both integer and floating-point values.
- Keeps the result in the same numeric type (integer or float).
- Works for **metric values only** (not for tags or metadata).
- Applies **after all other transformations** on the same metric (such as `extract_value`).

```yaml
metrics:
  - MIB: IP-MIB
    table:
      OID: 1.3.6.1.2.1.4.31.1
      name: ipSystemStatsTable
    symbols:
      - OID: 1.3.6.1.2.1.4.31.1.1.6
        name: ipSystemStatsHCInOctets
        chart_meta:
          description: Octets received in input IP datagrams
          family: 'Network/IP/Traffic/Total/In'
          unit: "bit/s"
        scale_factor: 8   # Octets → bits

  - MIB: IF-MIB
    symbol:
      OID: 1.3.6.1.2.1.31.1.1.1.15
      name: ifHighSpeed
      chart_meta:
        description: Estimate of the interface's current bandwidth
        family: 'Network/Interface/Speed'
        unit: "bit/s"
      scale_factor: 1000000   # Megabits → bits
```

**What this does**:

- Multiplies octet counters by `8`, reporting traffic in **bits per second** instead of bytes.
- Converts `ifHighSpeed` from **megabits** to **bits**.
- Ensures scaling happens **after** other transformations, such as value extraction or regex processing.

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

| Item               | Field          | Type                 | Required | Default | Applies to               | Description                                                                                                                                                                     |
|--------------------|----------------|----------------------|----------|---------|--------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Virtual Metric** | `name`         | string               | yes      | —       | all                      | Unique within the profile. Used as metric/chart base name.                                                                                                                      |
|                    | `sources`      | array\<Source\>      | no*      | —       | totals, per_row, grouped | Direct source set. Ignored if `alternatives` exist (alternatives take precedence).                                                                                              |
|                    | `alternatives` | array\<Alternative\> | no*      | —       | totals, per_row, grouped | Ordered fallback sets. The first alternative whose sources produce data is used.                                                                                                |
|                    | `per_row`      | bool                 | no       | false   | per-row/grouped          | When `true`, emits one output per input row; sources become dimensions; row tags attach.                                                                                        |
|                    | `group_by`     | string / array       | no       | —       | per-row/grouped          | Label(s) used as row-key hints (in order). Missing/empty hints fall back to a full-tag stable key. With `per_row:false`, this acts like PromQL’s `sum by (...)`.                |
|                    | `chart_meta`   | object               | no       | —       | all                      | Presentation metadata (`description`, `family`, `unit`, `type`).                                                                                                                |
| **Source**         | `metric`       | string               | yes      | —       | —                        | Name of an existing metric (scalar or table column metric).                                                                                                                     |
|                    | `table`        | string               | yes      | —       | —                        | Table name for the originating metric. Must match the metric’s table when used in per-row/grouped.                                                                              |
|                    | `as`           | string               | yes      | —       | —                        | Dimension name within the composite (e.g., `in`, `out`).                                                                                                                        |
| **Alternative**    | `sources`      | array\<Source\>      | yes      | —       | —                        | All sources in an alternative are evaluated together. If none produce data, the collector tries the next alternative. Per-row/group rules apply within the winning alternative. |

> At least one of `sources` or `alternatives` **must be defined**.

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

**What this does**:

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
