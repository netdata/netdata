# Profile Format

## Overview

An **SNMP profile** defines _how a specific class of devices is monitored through SNMP_.

:::info

SNMP profiles are reusable and declarative — you never need to modify the collector source code to support new devices.

:::

It tells the Netdata SNMP collector:

- which **OIDs** to query
- how to **interpret** the returned values
- how to **transform** them into **metrics**, **dimensions**, **tags**, and **metadata**

Profiles make it possible to describe _entire device families_ (switches, routers, UPSes, firewalls, printers, etc.) declaratively — so you don’t need to hard-code logic in Go or manually define metrics for each device.

Each profile is a single YAML file that can be reused, extended, and combined.

### How Profiles Work

When Netdata connects to an SNMP device, the collector:

1. Reads the device’s **sysObjectID** and **sysDescr**.
2. Evaluates all available profiles.
3. Applies every profile whose [selector](#1-selector) matches.
4. Uses the combined configuration to:
    - Collect [scalar metrics](#scalar-metrics-single-values) (single values like uptime or temperature).
    - Collect [table metrics](#table-metrics-multiple-rows) (multi-row values like per-interface traffic).
    - Build [virtual metrics](#virtual-metrics) (derived totals, fallbacks, or aggregates).
    - Gather [metadata](#3-metadata) and [tags](#5-metric_tags) for labeling and grouping.

**Profile Lifecycle**

```text
┌──────────────────────┐
│ SNMP Device          │ → provides sysObjectID/sysDescr
└──────────┬───────────┘
           ↓
┌──────────────────────┐
│ selector             │ → matches device profile
├──────────────────────┤
│ extends              │ → inherits base profiles
├──────────────────────┤
│ metadata             │ → device info (vendor, model, etc.)
├──────────────────────┤
│ metrics              │ → OIDs to collect
├──────────────────────┤
│ metric_tags          │ → dynamic tags for all metrics
├──────────────────────┤
│ static_tags          │ → fixed tags for all metrics
├──────────────────────┤
│ virtual_metrics      │ → calculated or aggregated metrics
└──────────┬───────────┘
           ↓
┌──────────────────────┐
│ Netdata charts & UI  │ → visualized in dashboard
└──────────────────────┘
```

### Example: Complete SNMP Profile

```yaml
# example-device.yaml

# selects which devices this profile applies to.
selector:
  - sysobjectid:
      include: ["1.3.6.1.4.1.9.*"]     # Cisco devices
    sysdescr:
      include: ["IOS"]

# imports common base metrics
extends:
  - _system-base.yaml
  - _std-if-mib.yaml

# defines device-level labels (virtual node)
metadata:
  device:
    fields:
      vendor:
        value: "Cisco"
      model:
        symbol:
          OID: 1.3.6.1.2.1.47.1.1.1.1.2.1
          name: entPhysicalModelName

#  specifies which OIDs to collect
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
        chart_meta:
          description: Interface inbound traffic
          family: 'Network/Interface/Traffic/In'
          unit: "bit/s"
        scale_factor: 8
    metric_tags:
      - tag: interface
        symbol:
          OID: 1.3.6.1.2.1.31.1.1.1.1
          name: ifName

# add dynamic tags to all metrics
metric_tags:
  - tag: fs_sys_version
    symbol:
      OID: 1.3.6.1.4.1.9.2.1.73.0
      name: fsSysVersion

# add fixed tags to all metrics
static_tags:
  - tag: region
    value: "us-east-1"
  - tag: environment
    value: "production"

# computes combined metrics
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

## Profile Structure

Each SNMP profile is a YAML file that defines **how Netdata collects, interprets, and labels SNMP metrics** from a device.

Profiles are modular — you can extend others, define metadata, and specify what to collect.

```yaml
selector: <device matching pattern>
extends: <base profiles to include>
metadata: <device information>
metrics: <what to collect>
metric_tags: <global tags>
static_tags: <static tags>
virtual_metrics: <calculated metrics>
```

| Section                                   | Purpose                                                   |
|-------------------------------------------|-----------------------------------------------------------|
| [**selector**](#1-selector)               | Defines which devices the profile applies to.             |
| [**extends**](#2-extends)                 | Inherits and merges other base profiles.                  |
| [**metadata**](#3-metadata)               | Collects device-level information (host labels).          |
| [**metrics**](#4-metrics)                 | Defines which OIDs to collect and how to chart them.      |
| [**metric_tags**](#5-metric_tags)         | Defines global dynamic tags collected once per device.    |
| [**static_tags**](#6-static_tags)         | Defines fixed tags applied to all metrics.                |
| [**virtual_metrics**](#7-virtual_metrics) | Defines calculated or aggregated metrics based on others. |

### 1. selector

You use the selector to:

- Target specific **device families** (e.g., Cisco, Juniper, HP)
- Match devices supporting specific **MIBs**
- Exclude unwanted devices

During discovery, Netdata evaluates all profiles; any profile whose selector matches a device is **applied **.

```yaml
selector:
  - sysobjectid:
      include: ["1.3.6.1.4.1.9.*"]     # regex: Cisco enterprise OID subtree
      exclude: ["1.3.6.1.4.1.9.9.666"]  # optional excludes
    sysdescr:
      include: ["IOS"]                  # substring (case-insensitive)
      exclude: ["emulator", "lab"]      # optional excludes
```

**How it works**:

- Each selector rule is a set of conditions (`sysobjectid`, `sysdescr`, etc.).
- For a rule to match, **all its conditions must pass**.
- A profile is applied if **at least one rule** in the `selector` list matches the device.
- If both `sysobjectid` and `sysdescr` are defined within the same rule, **both must succeed**.

**Supported conditions**:

| Key                   | What It Checks                       | Match Criteria (Pass)                            | Fails When...                   |
|-----------------------|--------------------------------------|--------------------------------------------------|---------------------------------|
| `sysobjectid.include` | Device `sysObjectID`                 | Matches **at least one** pattern in the list.    | No items match.                 |
| `sysobjectid.exclude` | Device `sysObjectID`                 | Matches **none** of the listed patterns.         | Any item matches.               |
| `sysdescr.include`    | Device `sysDescr` (case-insensitive) | Contains **at least one** substring in the list. | No listed substrings are found. |
| `sysdescr.exclude`    | Device `sysDescr` (case-insensitive) | Contains **none** of the listed substrings.      | Any listed substring is found.  |

### 2. extends

Use `extends` to inherit metrics, tags, and metadata from another profile instead of duplicating common metrics — perfect for vendor-specific variations of a base MIB.

Most real profiles extend a few shared building blocks and then add device-specific definitions.

```yaml
extends:
  - _system-base.yaml        # System basics (uptime, contact, location)
  - _std-if-mib.yaml         # Network interfaces (IF-MIB)
  - _std-ip-mib.yaml         # IP statistics
```

The final profile is the **merged result** of all inherited profiles plus the content in the current file.

**How inheritance works**:

1. **Order matters** — profiles are loaded in the order listed.
2. **Metrics are merged** — all metrics from all referenced profiles are included.
3. **Later overrides earlier** — if the same field is defined multiple times, the last one wins.

**Common base profiles**

| Profile             | Provides                                  | Typical Use        |
|---------------------|-------------------------------------------|--------------------|
| `_system-base.yaml` | Basic system info (uptime, name, contact) | All devices        |
| `_std-if-mib.yaml`  | Interface statistics (IF-MIB)             | Network devices    |
| `_std-ip-mib.yaml`  | IP-level statistics (IP-MIB)              | Routers, switches  |
| `_std-tcp-mib.yaml` | TCP statistics                            | Servers, firewalls |
| `_std-udp-mib.yaml` | UDP statistics                            | Servers, firewalls |
| `_std-ups-mib.yaml` | Power and UPS metrics                     | UPS devices        |

### 3. metadata

The `metadata` section defines **device-level information** (not metric tags).

It is collected **once per device** and populates the device’s **host labels** in Netdata (the “virtual node” labels shown on the device page).

It always follows the structure `metadata → device → fields`, where each field defines a single label.

Each field can be:

- **Static** — `value:` is a fixed string.
- **Dynamic** — `symbol:` reads the value from an SNMP OID.

```yaml
metadata:
  device:
    fields:
      vendor:
        value: "Cisco"   # static label
      model:
        symbol: # dynamic label from SNMP
          OID: 1.3.6.1.2.1.47.1.1.1.1.2.1
          name: entPhysicalModelName
```

:::info

You can define multiple symbols for fallback — the first valid one will be used.

:::

**How it works**:

- `vendor` is set statically to `"Cisco"`.
- `model` is collected from `entPhysicalModelName`.
- These values appear as **device (virtual node) host labels** in the UI. They are **not** per-metric tags.

:::tip

See [**Tag Transformation**](#tag-transformation) for supported transformations and syntax examples.

:::

### 4. metrics

The `metrics` section defines **what data to collect** from the device — which OIDs to query, how to interpret them, and how to display them as charts in Netdata.

**Metrics can be**:

- **Scalars** — single values that apply to the entire device (for example, uptime).
- **Tables** — repeating rows of related values (for example, interfaces, disks, sensors).

:::note

A metric is **either scalar** (single value) or **table-based** (multiple rows).
Never mix both in the same metric entry.

:::

The collector automatically uses **SNMP GET** for scalars and **SNMP BULKWALK** for tables.

```yaml
metrics:
  - MIB: HOST-RESOURCES-MIB
    symbol:
      OID: 1.3.6.1.2.1.1.3.0
      name: systemUptime
      scale_factor: 0.01  # Value is in hundredths of a second
      chart_meta:
        description: Time since the system was last rebooted or powered on
        family: 'System/Uptime'
        unit: "s"

  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.31.1.1
      name: ifXTable
    symbols:
      - OID: 1.3.6.1.2.1.31.1.1.1.6
        name: ifHCInOctets
        chart_meta:
          description: Traffic
          family: 'Network/Interface/Traffic/In'
          unit: "bit/s"
        scale_factor: 8  # Octets → bits
    metric_tags:
      - tag: interface
        symbol:
          OID: 1.3.6.1.2.1.31.1.1.1.1
          name: ifName
```

**How it works**:

- Each entry defines a metric or table of metrics to collect via SNMP.
- Scalars use a single `symbol`, while tables define a `table` and one or more `symbols`.
- Metrics can include transformations (`extract_value`, `scale_factor`, etc.) and chart metadata.
- Table metrics can include tags (`metric_tags`) to identify rows by interface, disk, or other attributes.

:::tip

See also

- [Collecting Metrics](#collecting-metrics)  — explains SNMP OIDs, scalars, and tables.
- [Scalar Metrics](#scalar-metrics-single-values) — detailed syntax for single-value metrics.
- [Table Metrics](#table-metrics-multiple-rows)— how tables and row indexes work.
- [Adding Tags to Metrics](#adding-tags-to-metrics) — tag types and how to label table rows.
- [Tag Transformations](#tag-transformation) — extract, match, or map tag values.
- [Value Transformations](#value-transformation) — manipulate or scale collected values.
- [Virtual Metrics](#virtual-metrics) — build new metrics from existing ones.

:::

### 5. metric_tags

The `metric_tags` section defines **global dynamic tags** — values collected once from the device and applied to **every metric** in the profile.

They are evaluated during collection, just like other SNMP symbols, and remain the same for all metrics within that device.

**Typical uses**:

- Attach device-wide metadata such as serial number, firmware version, or model.
- Enable grouping and filtering by hardware, OS, or vendor attributes.
- Complement per-metric tags (for example, per-interface or per-sensor tags in tables).

```yaml
metric_tags:
  - tag: fs_sys_serial
    symbol:
      OID: 1.3.6.1.4.1.12356.106.1.1.1.0
      name: fsSysSerial
  - tag: fs_sys_version
    symbol:
      OID: 1.3.6.1.4.1.12356.106.4.1.1.0
      name: fsSysVersion
```

**How it works**:

- Each tag is collected once per device, not per metric or per table row.
- The resulting tag values are attached to **all metrics** collected by the profile.
- Tags can be transformed (for example, reformatted or mapped) using the same rules as per-metric tags.

:::tip

See [**Tag Transformation**](#tag-transformation) for supported transformations and syntax examples.

:::

### 6. static_tags

The `static_tags` section defines **fixed key–value pairs** that are attached to every metric collected by the profile.

They don’t depend on SNMP data and remain constant for all devices using the profile.

**Typical uses**:

- Add environment or deployment identifiers (for example, `environment`, `region`, or `service`).
- Simplify filtering, grouping, and alerting across metrics from multiple devices.
- Provide consistent context (for example, datacenter or team ownership).

```yaml
static_tags:
  - tag: environment
    value: production
  - tag: region
    value: us-east-1
  - tag: service
    value: network
```

**How it works**:

- Each tag is added to **all metrics** collected by the profile.
- Static tags are merged with any dynamic tags defined in `metric_tags`.
- Device-specific or dynamic tags always take precedence if they overlap.

### 7. virtual_metrics

The `virtual_metrics` section defines **calculated metrics** built from other metrics already collected by the profile.

They don’t query SNMP directly — instead, they reuse existing metric values to produce totals, sums, or fallbacks.

:::tip

See [**Virtual Metrics**](#virtual-metrics) for the complete reference, configuration options, and advanced examples.

:::

**Typical uses**:

- Combine related counters (for example, `in` + `out` traffic or errors).
- Create fallbacks (prefer 64-bit counters, fall back to 32-bit if missing).
- Aggregate or group metrics per tag (for example, total per interface or per type).

```yaml
  - name: ifTotalTraffic
    sources:
      - { metric: ifHCInOctets,  table: ifXTable, as: in }
      - { metric: ifHCOutOctets, table: ifXTable, as: out }
    chart_meta:
      description: Total traffic across all interfaces
      family: 'Network/Total/Traffic'
      unit: "bit/s"
```

**How it works**:

- Defines a new virtual metric named `ifTotalTraffic`.
- Uses existing metrics (`ifHCInOctets`, `ifHCOutOctets`) as sources.
- The `as` field names the resulting dimensions (`in`, `out`).
- The resulting chart behaves like a regular metric — visible in dashboards, alertable, and included in exports.

## Collecting Metrics

This section explains how SNMP data is structured and how it maps to metrics in a Netdata profile.

### Understanding SNMP Data

SNMP data is organized as a **hierarchical tree** of numeric identifiers called **OIDs** (**Object Identifiers**).

Each OID uniquely identifies a value on a device — similar to a file path in a filesystem.

```text
1.3.6.1.2.1.1.3.0
│ │ │ │ │ │ │ └── Instance (0 = scalar)
│ │ │ │ │ │ └──── Object (3 = sysUpTime)
│ │ │ │ │ └────── Branch: system (MIB-2)
│ │ │ └────────── MIB-2 root
└─ SNMP global prefix
```

- **MIBs** (Management Information Bases) are named collections of related OIDs.

  Examples: `IF-MIB` (interfaces), `IP-MIB` (IP statistics), `HOST-RESOURCES-MIB` (system info).
- Each OID maps to a **typed value**, such as `Counter64`, `Gauge32`, `Integer`, or `TimeTicks`.
- Some OIDs represent **single values** (scalars), while others represent **tables** of related values (rows).

### Scalar Metrics (Single Values)

Scalar metrics represent a **single value for the entire device**.

Their OIDs always end with `.0`, which denotes the **instance number** for a scalar object.

```yaml
metrics:
  - MIB: HOST-RESOURCES-MIB
    symbol:
      OID: 1.3.6.1.2.1.1.3.0
      name: systemUptime
      scale_factor: 0.01  # Value is in hundredths of a second
      chart_meta:
        description: Time since the system was last rebooted or powered on.
        family: 'System/Uptime'
        unit: "s"
```

**What this does**:

- Collects the `sysUpTime` value once per device.
- The `.0` at the end indicates there is only **one instance** of this value.
- Common scalar metrics: device uptime, total memory, or overall temperature.

### Table Metrics (Multiple Rows)

Table metrics represent **lists of related values**, such as one entry per network interface, disk, or CPU.

Each row in a table is identified by an **index** appended to the base OID — for example:

```text
ifHCInOctets.1 = 1024
ifHCInOctets.2 = 2048
```

- `.1`, `.2`, … are `row indexes` that identify the instance (e.g., interface #1, interface #2).
- Each column (symbol) in the table has its own OID pattern but shares the same row indexes.

> Table metrics **must define at least one tag** (`metric_tags`) to identify each row.
> Without tags, only a single row can be emitted.

```yaml
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.31.1.1
      name: ifXTable
    symbols:
      - OID: 1.3.6.1.2.1.31.1.1.1.6
        name: ifHCInOctets
        chart_meta:
          description: Traffic
          family: 'Network/Interface/Traffic/In'
          unit: "bit/s"
        scale_factor: 8  # Octets → bits
    metric_tags:
      - tag: interface
        symbol:
          OID: 1.3.6.1.2.1.31.1.1.1.1
          name: ifName
```

**How Table Metrics Expand into Rows**

```text
SNMP Table: ifTable
───────────────────────────────────────────────
Index | ifName | ifHCInOctets
───────────────────────────────────────────────
1     | eth0   | 1024
2     | eth1   | 2048
───────────────────────────────────────────────

metric_tags:
  - tag: interface
    symbol:
      OID: 1.3.6.1.2.1.31.1.1.1.1   # ifName

Resulting metrics:
───────────────────────────────────────────────
ifHCInOctets{interface="eth0"} = 1024
ifHCInOctets{interface="eth1"} = 2048
───────────────────────────────────────────────
```

**How it works**:

1. The collector reads both columns (`ifHCInOctets` and `ifName`) from the same table.
2. It aligns rows using their shared SNMP index (`1`, `2`, …).
3. Each metric is emitted with its corresponding tag from the same row.

**What this does**:

- Collects traffic (`ifHCInOctets`) from each interface.
- Tags each row with its name (`ifName`) from the same index.
- Produces metrics like:
    ```text
    ifHCInOctets{interface="eth0"} = 1024
    ifHCInOctets{interface="eth1"} = 2048
     ```

### Metric Types

Each SNMP value has a data type that determines **how Netdata interprets and displays it**.

The collector automatically detects the appropriate **metric type** (e.g., `gauge` or `rate`), but you can override it manually.

**Automatic Type Detection**

| SNMP Type                | Default Netdata Type | Typical Use                          |
|--------------------------|----------------------|--------------------------------------|
| `Counter32`, `Counter64` | `rate`               | Network traffic, packet counters     |
| `Gauge32`, `Integer`     | `gauge`              | Temperatures, usage levels, statuses |
| `TimeTicks`              | `gauge`              | Uptime, time-based values            |

**Overriding the Metric Type**

You can explicitly set a metric’s type using the `metric_type` field inside a symbol definition.

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

**What this does**:

- Forces `ifInOctets` to be treated as a **gauge** (instantaneous value) instead of a rate.
- Normally, `Counter` types are automatically converted to per-second rates.

### Chart Metadata

Each metric or virtual metric can include an optional `chart_meta` block that defines how it appears in Netdata charts.

This metadata **does not affect data collection** — it only controls how the chart is **named** and **grouped**  in the Netdata dashboard.

```yaml
metrics:
  - MIB: IF-MIB
    table:
      OID: 1.3.6.1.2.1.2.2
      name: ifTable
    symbols:
      - OID: 1.3.6.1.2.1.2.2.1.10
        name: ifInOctets
        chart_meta:
          description: Inbound network traffic
          family: 'Network/Interface/Traffic/In'
          unit: "bit/s"
```

| Field         | Type   | Required | Description                                                                                       |
|---------------|--------|----------|---------------------------------------------------------------------------------------------------|
| `description` | string | no       | Human-readable description shown in dashboards and alerts.                                        |
| `family`      | string | no       | Chart grouping path (slashes `/` define hierarchy). Helps organize charts by system or subsystem. |
| `unit`        | string | no       | Display unit, e.g. `"bit/s"`, `"%"`, `"{status}"`, `"Cel"`.                                       |
| `type`        | string | no       | Optional chart style override: `line`, `area`, or `stacked`. Defaults depend on metric type.      |

## Adding Tags to Metrics

Tags add **context and identity** to SNMP metrics.

They let you distinguish between instances (for example, which interface, disk, or IP) and allow filtering and grouping in the Netdata UI.

**The collector**:

- Attaches tags to each metric as labels.
- Uses tags to differentiate rows when building charts.
- Requires at least one tag for every **table metric** (to identify each row).
- Ignores tags for **scalar metrics**, which represent a single value per device.

**Key Concepts**:

| Concept                            | Description                                                                                                                                   |
|------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| **Table metrics must have tags**   | Each table row must be uniquely identified by at least one tag (for example, interface name or index). Without tags, only one row is emitted. |
| **Scalar metrics don’t need tags** | Scalars represent one value for the entire device, not per-instance data.                                                                     |
| **Static tags**                    | Fixed values that never change (for example, datacenter, environment).                                                                        |
| **Dynamic tags**                   | Extracted from SNMP data — from table columns, related tables, or row indexes.                                                                |
| **Global tags**                    | Defined in the profile’s top-level `metric_tags` section and applied to all metrics.                                                          |

**Tag Types and Available Transformations**:

| Tag Type            | Description                                                     | Supported Transformations                                                     |
|---------------------|-----------------------------------------------------------------|-------------------------------------------------------------------------------|
| **Static**          | Fixed tags with constant values.                                | None (value is fixed).                                                        |
| **Same-Table**      | Values from columns in the same table as the metric.            | `mapping`, `extract_value`, `match_pattern` + `match_value`, `match` + `tags` |
| **Cross-Table**     | Values from another table.                                      | `mapping`, `extract_value`, `match_pattern` + `match_value`, `match` + `tags` |
| **Index-Based**     | Values derived from the OID index of each row.                  | `mapping` (optional)                                                          |
| **Index Transform** | Adjusts multi-part indexes so cross-table tags align correctly. | — (structural, not a transformation)                                          |

**Summary**:

- Each **table metric** must define at least one **tag source** (`metric_tags`) to distinguish rows.
- Tags can come from **the same table**, **another table**, or the **row index** itself.
- Tag transformations (`mapping`, `extract_value`, `match_pattern`, `match + tags`) can modify or extract parts of raw values.
- **Static tags** apply globally and are not transformed.
- **Index transformations** are a special mechanism used only for aligning multi-part indexes between tables.

**How the Collector Matches Values and Tags**:

```text
SNMP Table (ifTable)
───────────────────────────────────────────────
Index | ifDescr        | ifInOctets
───────────────────────────────────────────────
1     | eth0           | 1024
2     | eth1           | 2048
───────────────────────────────────────────────

metric_tags:
  - tag: interface
    symbol:
      OID: 1.3.6.1.2.1.2.2.1.2   # ifDescr

Resulting metrics:
───────────────────────────────────────────────
ifInOctets{interface="eth0"} = 1024
ifInOctets{interface="eth1"} = 2048
───────────────────────────────────────────────
```

How it works:

1. The collector walks the table and collects both columns: `ifInOctets` (value) and `ifDescr` (tag source).
2. It aligns them by their shared SNMP index (`1`, `2`, …).
3. Each metric row is tagged with the corresponding column value from the same index.

**Cross-Table Example**:

```text
SNMP Tables: ifTable + ifXTable
───────────────────────────────────────────────
ifTable.ifInOctets.1  = 1024
ifTable.ifInOctets.2  = 2048

ifXTable.ifName.1     = "eth0"
ifXTable.ifName.2     = "eth1"
───────────────────────────────────────────────

metric_tags:
  - tag: interface
    table: ifXTable
    symbol:
      OID: 1.3.6.1.2.1.31.1.1.1.1   # ifName

Result:
───────────────────────────────────────────────
ifInOctets{interface="eth0"} = 1024
ifInOctets{interface="eth1"} = 2048
───────────────────────────────────────────────
```

How it works:

- The collector collects metrics from `ifTable` but fetches tag values from `ifXTable`.
- It matches rows from both tables using their shared index (`.1`, `.2`, …).
- The `interface` tag is populated from `ifXTable.ifName` for each matching row

### Static

Static tags define **fixed key–value pairs** that are attached to metrics without being collected from SNMP.

They are useful for identifying **environment**, **location**, or other context that applies to all collected data.

#### Profile-level Static Tags

Profile-level static tags apply to **all metrics** defined in the profile.

```yaml
# Global static tags (applied to all metrics)
static_tags:
  - tag: datacenter
    value: "DC1"
  - tag: environment
    value: "production"
```

**What this does**:

- These tags are injected into every metric reported by the profile.

**Typical use cases**:

- Identify the datacenter, region, or cluster where the device belongs.
- Mark metrics from staging or production environments.
- Add organization-wide context that doesn’t depend on SNMP data.

#### Metric-level Static Tags

Metric-level static tags apply to **specific metrics only**.

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

**What this does**:

- Adds the tags `source=snmp` and `interface_type=physical` only to the `ifInOctets` metric.
- Does not affect other metrics in the same profile.

> Metric-level static tags are technically supported but rarely needed.
> In most cases, prefer profile-level `static_tags` for consistency and simplicity.

### Same-Table

Same-table tags extract values from **columns in the same SNMP table** as the metric.

They are the most common way to label per-row metrics with identifiers like interface names or indexes.

**The collector**:

- Retrieves both the metric value and the tag column from the same table row.
- Automatically aligns rows by their shared index.
- Adds the tag to every metric collected from that row.

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

**What this does**:

- Collects `ifInOctets` (input bytes) for each row in `ifTable`.
- Reads the `ifDescr` column from the same table to label each row.
- Produces metrics like:
    ```text
    ifInOctets{interface="eth0"} = 1000
    ifInOctets{interface="eth1"} = 2000
    ```

### Cross-Table

Cross-table tags let you **use data from another SNMP table** as a tag source.

**The collector**:

- Reads tag values from the specified `table:` instead of the current one.
- Matches rows between tables by their **index**.
- When index structures differ, an optional `index_transform` can modify the current table’s index to align it with the target.

#### Same Index

Two tables are said to have the **same index** when their row identifiers (OID suffixes after the base OID) are identical — meaning they describe the same entity.

In practice, this means that the row number (index) in one table corresponds directly to the same row in another.

For example:

```text
ifTable.ifInOctets.2  = 123456
ifXTable.ifName.2     = "xe-0/0/1"
```

Both OIDs end with `.2`, so they refer to the same interface.

This allows you to use `ifName` (from `ifXTable`) as a tag for metrics collected from `ifTable`.

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

**What this does**:

- Collects `ifInOctets` from `ifTable`.
- Finds the row with the same index in `ifXTable` (e.g., `.2`).
- Uses `ifName` as the `interface` tag.
- Produces metrics like:
    ```text
    ifInOctets{interface="xe-0/0/1"} = 123456
    ```

#### With Index Transformation

Some tables describe related data but use **different index structures** — meaning their OID suffixes don’t line up directly.

For example, in `ipIfStatsTable` the index contains **two parts**:

```text
ipIfStatsTable.ipIfStatsHCInOctets.2.1 = 38560
ipIfStatsTable.ipIfStatsHCInOctets.2.2 = 44408
```

Here:

- The first component (`2`) is the **IP version** (e.g., 2 = IPv4, 3 = IPv6).
- The second component (`1`, `2`, `3`, …) is the **interface index**.
- `ifXTable`, on the other hand, uses only the interface index (`1`, `2`, `3`, …).

Because the indexes differ, they can’t be matched directly.

To fix this, use `index_transform` to **select only the relevant part of the index** so it matches the target table’s format.

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

**What this does**:

- Collects IP traffic metrics from `ipIfStatsTable`.
- Keeps only the **second index element** (`start: 1`, `end: 1`) from `2.1` → becomes `1`.
- Looks up that interface index in `ifXTable` to find the corresponding `ifName`.
- Produces:
    ```yaml
    ipIfStatsHCInOctets{_interface="xe-0/0/1"} = 38560
    ```

##### How `index_transform` Works

`index_transform` tells the collector which parts of the current table’s index to keep when matching rows across tables.

| Concept            | Example                                                                                       |
|--------------------|-----------------------------------------------------------------------------------------------|
| **Original index** | `2.1` (from `ipIfStatsTable`) → `[ipVersion, ifIndex]`                                        |
| **Target index**   | `1` (from `ifXTable`)                                                                         |
| **Transform**      | `index_transform: [ { start: 1, end: 1 } ]`                                                   |
| **Result**         | The collector keeps only the **second element** (`ifIndex = 1`), which now matches `ifXTable` |

**In short**:

- `start` and `end` positions are **zero-based** (0 = first index element).
- Each range defines which parts of the index to keep.
- You can list multiple ranges to combine non-contiguous parts.
- The goal is to make the current table’s index **match** the target table’s index so tags align correctly.

### Index-Based

Index-based tags extract values directly from the **OID index** of the SNMP table rather than from a column.

This is useful when a table encodes identifiers (like method, code, or port number) as part of the OID itself instead of storing them in separate columns.

**The collector**:

- Splits the table’s row index into numbered parts (1-based).
- For each `index:` rule, assigns a tag using the specified position in the index.
- Converts numeric index components to strings automatically.
- Attaches all resulting tags to the metric collected from that row.

```yaml
metrics:
  - MIB: SIP-COMMON-MIB
    table:
      name: sipCommonStatusCodeTable
      OID: 1.3.6.1.2.1.149.1.5.1
    symbols:
      - OID: 1.3.6.1.2.1.149.1.5.1.1.3
        name: sipCommonStatusCodeIns
        chart_meta:
          family: 'Network/VoIP/SIP/Response/StatusCode/In'
          description: Total number of response messages received with the specified status code
          unit: "{response}/s"
    metric_tags:
      - index: 1
        tag: applIndex
      - index: 2
        tag: sipCommonStatusCodeMethod
      - index: 3
        tag: sipCommonStatusCodeValue
```

**What this does**:

- Extracts the first three components of each row’s OID index and uses them as tags.
- For example, if the full OID is:
    ```text
    1.3.6.1.2.1.149.1.5.1.1.3.1.6.200
    ```
  The collector interprets:
    ```ini
    applIndex=1
    sipCommonStatusCodeMethod=6
    sipCommonStatusCodeValue=200
    ```
- Produces metrics like:
    ```text
    sipCommonStatusCodeIns{applIndex="1", sipCommonStatusCodeMethod="6", sipCommonStatusCodeValue="200"} = 42
    ```

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
