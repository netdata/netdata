# NIDL Framework: Guiding Netdata's Observability

## Introduction

The Netdata NIDL (Nodes, Instances, Dimensions, Labels) framework is the foundational data model that underpins Netdata's approach to observability. It defines how metrics are structured, collected, stored, and presented, enabling an interactive and intuitive analysis experience without the need for a query language.

For **users**, NIDL transforms raw data into an explorable, multi-dimensional view of their infrastructure. For **developers**, NIDL provides a strict set of guidelines for metric design, ensuring that collected data automatically translates into meaningful and actionable dashboards.

This document serves as both an introduction to the NIDL framework for all Netdata users and a fundamental guide for developers contributing to Netdata's data collection.

## NIDL for Users: Interactive Observability

Imagine your infrastructure's performance data as a complex, multi-dimensional cube. Traditional monitoring tools often require you to learn a specialized query language to extract insights from this cube. Netdata, through the NIDL framework, provides intuitive controls to slice, dice, and examine this cube from any angle using simple dropdown menus.

### The NIDL Components

Every metric collected by Netdata is organized according to these four components:

1.  **Nodes**: Represent individual machines or hosts where Netdata agents are running. In a multi-node environment, this allows you to aggregate and compare metrics across your entire fleet.
2.  **Instances**: Specific entities being monitored within a particular context. For example, in a `disk.io` chart, instances would be individual disk devices like `sda`, `sdb`. In a `containers.cpu` chart, instances would be individual container IDs or names.
3.  **Dimensions**: The individual values or series that make up a chart. All dimensions within a single chart must share the same unit and represent related aspects of the monitored instance. For example, a CPU chart might have `user`, `system`, `iowait`, and `idle` as dimensions.
4.  **Labels**: Key-value pairs that provide additional metadata about an instance or dimension. Labels enable powerful filtering and grouping capabilities. Examples include `kubernetes_namespace=production`, `device_type=ssd`, or `environment=staging`.

### How to Use NIDL for Analysis

Every Netdata chart is an interactive analytical tool. Above each graph, you'll find dropdown menus corresponding to Nodes, Instances, Dimensions, and Labels. These menus are not just for filtering; they provide real-time statistics to guide your investigation:

```
┌───────────┬───────┬────────┬───────────┬────────────┬────────┬────────────┐
│ group by ▼│aggr. ▼│nodes ▼ │instances ▼│dimensions ▼│labels ▼│time aggr. ▼│
└───────────┴───────┴────────┴───────────┴────────────┴────────┴────────────┘
┌───────────────────────────────────────────────────────────────────────────┐
│ ▒▒▒▒▒░░░▒▒▒▒▒▒░░░░▒▒▒  Anomaly ribbon (anomaly rates over time)           │
├───────────────────────────────────────────────────────────────────────────┤
│     ╱╲    ╱╲                                                              │
│    ╱  ╲  ╱  ╲                   GRAPH                                     │
│   ╱    ╲╱    ╲                                                            │
│  ╱            ╲_______________                                            │
├───────────────────────────────────────────────────────────────────────────┤
│ ░░░░█░░░░░  Info ribbon (gaps, resets, partial data)                      │
└───────────────────────────────────────────────────────────────────────────┘
                                    X-axis (time)
─────────────────────────────────────────────────────────────────────────────
Dimension1: 12.3k  │ Dimension2: 8.9k  │ Dimension3: 5.6k  │ Dimension4: 2.3k
```

Each dropdown table displays:
*   **Number of time-series**: How many individual data streams contribute to that item.
*   **Number of instances**: The count of instances (relevant for Nodes, Dimensions, Labels dropdowns).
*   **Volume contribution %**: The percentage of the total chart's value attributed to that item.
*   **Anomaly rate %**: The percentage of time the item exhibited anomalous behavior within the visible window.
*   **Minimum, Average, Maximum values**: Statistical summaries for the item over the visible period.

This rich context enables:

*   **Instant Root Cause Analysis**: If you see a spike on a chart, open the "Instances" dropdown, sort by "Maximum Value," and immediately identify which specific instance (e.g., a container, a disk) is responsible. Similarly, sort by "Anomaly Rate" to pinpoint unusual behavior.
*   **Flexible Grouping and Aggregation**: Change how the chart aggregates data. Group by `kubernetes_namespace` to see CPU usage per namespace, then apply an `average` aggregation to understand typical consumption.
*   **Slicing and Filtering**: Select specific nodes, instances, or label values to narrow down the chart to only the data you care about.

### NIDL Custom Dashboards

NIDL extends beyond individual charts to enable powerful custom dashboards through simple drag-and-drop operations:

*   **Chart Reuse**: Any chart can appear multiple times on a custom dashboard with different NIDL settings
*   **Visual Arrangement**: Resize and position charts to create meaningful layouts
*   **Preserved Interactivity**: Each chart retains its full NIDL capabilities - filtering, grouping, and aggregation
*   **Instant Creation**: No query writing or configuration files - just drag, drop, and customize

Example: Create a Kubernetes dashboard by dragging CPU, memory, and network charts, then setting one to group by namespace, another by pod, and a third showing node-level aggregations - all from the same underlying metrics.

### Benefits of NIDL for Users

*   **No Query Language Required**: Explore and analyze complex data through intuitive point-and-click interactions.
*   **Automatic Visualization**: Every metric collected appears automatically on Netdata dashboards - if it's collected, it's visible. No manual dashboard configuration needed.
*   **Speed of Investigation**: Real-time statistics and interactive controls allow for rapid diagnosis of issues.
*   **Consistent Experience**: All Netdata charts operate under the same NIDL principles, making the interface predictable and easy to learn.
*   **Custom Dashboard Simplicity**: Create powerful custom views through drag-and-drop, with each chart maintaining its full NIDL interactivity - no queries or configuration files required.
*   **Scalability**: The framework works identically whether you're monitoring a single server or thousands of nodes.

### Current Design Boundaries

NIDL makes deliberate UX choices to maintain simplicity and clarity. These aren't technical limitations - the query engine could handle more complexity - but rather design decisions that keep the interface intuitive:

1. **Uniform Aggregation per Chart**: All dimensions in a chart use the same aggregation function. For example, you cannot show "min of mins" and "max of maxes" in the same chart. This keeps the mental model simple: one chart, one aggregation.

2. **Single Context per Chart**: Each chart displays metrics from one context only. While the engine can query multiple contexts simultaneously, combining them would require complex UI controls that could overwhelm users.

3. **Continuous Evolution**: While NIDL controls already enhance and simplify advanced analytics for the vast majority of use cases, Netdata is continuously evolving. Like all monitoring solutions, we identify areas for improvement and actively work to address them.

### Ongoing Enhancements

We've identified several areas where Netdata dashboards can be enhanced to cover even more sophisticated use cases without compromising NIDL's simplicity:

1. **Virtual Contexts**: Enable custom calculations across multiple contexts, appearing as new charts on dashboards - bringing complex correlations to the same simple interface

2. **Advanced Query Mode**: Introduce an optional query editor for power users, thoughtfully integrated to preserve the default NIDL experience

3. **Persistent Virtual Metrics**: Allow complex calculations to be saved as new time-series, making advanced analytics reusable and shareable

These improvements are part of Netdata's commitment to making infrastructure monitoring both powerful and accessible. We continuously refine the balance between capability and simplicity based on real-world usage and community feedback.

## NIDL for Developers: The Engineering Discipline

For Netdata developers, understanding and adhering to the NIDL framework is paramount. In Netdata, **metric design IS dashboard design**. The choices made during data collection directly determine the usability and clarity of the automatically generated dashboards. There is no separate dashboard configuration step to correct poorly structured metrics.

### Why Metric Design is Critical

*   **Algorithmic Dashboards**: Netdata's dashboards are not static configurations; they are algorithmic outputs driven by the metadata embedded in your collected metrics. Your annotations guide this algorithm.
*   **No Second Chances**: Mistakes in metric design cannot be easily rectified later through UI configuration. The integrity of the NIDL framework depends on correct data structuring at the source.
*   **User Experience**: Your design decisions directly impact a user's ability to understand, troubleshoot, and gain insights from their infrastructure. Confusing charts lead to frustration and missed issues.

### Core Principles for NIDL-Compliant Collectors

To ensure your collected metrics integrate seamlessly with the NIDL framework and produce meaningful charts, adhere to the following principles:

#### 1. One Instance Type Per Context

Each Netdata **context** (which corresponds to a single chart on the dashboard) must contain only one type of instance. Mixing different types of entities within the same context will lead to confusing dropdown menus and meaningless aggregations.

**Correct Example**:
```
Context: mysql.db.queries
Instances: database1, database2, database3
Dimensions: select, insert, update, delete
```
*Explanation*: All instances are of type "database".

**Incorrect Example**:
```
Context: mysql.queries
Instances: server1, database1, table1  // Mixed instance types
Dimensions: select, insert, update, delete
```
*Explanation*: Mixing server, database, and table instances in one context makes the "Instances" dropdown unusable for comparison or drill-down.

#### 2. Related Dimensions Only

All **dimensions** within a single chart (context) must be logically related, share the same unit, and make sense when aggregated together.

**Correct Example**:
```
Context: system.cpu
Dimensions: user, system, iowait, idle
Unit: percentage
```
*Explanation*: All dimensions represent parts of CPU time and sum to 100%.

**Incorrect Example**:
```
Context: system.health
Dimensions: cpu_percent, free_memory_mb, disk_io_ops
```
*Explanation*: These dimensions have different units and represent unrelated metrics, making aggregation or comparison within the same chart meaningless.

#### 3. Hierarchical Separation (Contexts, not Families for Instances)

For hierarchical data (e.g., a database server, its databases, its tables, its indexes), create **separate contexts** for each level of the hierarchy. Do not attempt to combine different hierarchical levels into a single context.

**Example: MySQL Monitoring**

Instead of one `mysql.operations` context trying to cover everything, create distinct contexts:

*   `mysql.operations`: Instances are the database servers themselves.
*   `mysql.db.operations`: Instances are individual databases (e.g., `users_db`, `orders_db`).
*   `mysql.table.operations`: Instances are individual tables within databases.
*   `mysql.index.operations`: Instances are individual indexes within tables.

Each of these contexts will generate its own chart, ensuring that the "Instances" dropdown for each chart is clean and coherent (e.g., the `mysql.db.operations` chart's "Instances" dropdown will only list databases, not servers or tables).

**Families** in Netdata are used to group *charts* (contexts) on the dashboard, not to define instance hierarchies within a single chart. You can use families to organize related contexts (e.g., a "MySQL" family containing all `mysql.*` contexts, or a "Connections" family grouping all connection-related contexts across different services).

#### 4. Thoughtful Labels

Use **labels** to provide meaningful metadata that enables flexible filtering and grouping. Labels should be consistent across instances and provide valuable context for analysis.

**Example**: For container metrics, useful labels might include:
*   `kubernetes_pod_name`
*   `kubernetes_namespace`
*   `docker_image`
*   `environment`

These labels allow users to slice their data by specific pods, namespaces, or environments, enhancing the analytical power of the chart.

### Practical Examples of NIDL Design

#### Example 1: Disk I/O

**Context Design**:
*   **Context**: `disk.io`
*   **Instances**: `sda`, `sdb`, `sdc` (individual disk devices)
*   **Dimensions**: `read`, `write` (bytes/s)
*   **Labels**: `device_type=ssd`, `mount_point=/var`

**What this enables**: Users can compare I/O across disks, see read/write patterns per disk, filter by device type, and group by mount point.

#### Example 2: Container Monitoring

**Context Design**:
*   **Context**: `containers.cpu`
*   **Instances**: `container1`, `container2`, `container3` (individual containers)
*   **Dimensions**: `user`, `system` (percentage)
*   **Labels**: `image=nginx`, `namespace=production`, `pod=web-server`

**What this enables**: Users can compare CPU usage across containers, identify system vs. user CPU consumption, filter by image type, and group by Kubernetes namespace or pod.

#### Example 3: Application Metrics (Separation of Concerns)

Instead of one generic metric, separate by concerns and hierarchical levels:

*   **Context**: `app.requests`
    *   **Instances**: `endpoint1`, `endpoint2`
    *   **Dimensions**: `success`, `client_error`, `server_error` (requests/s)
*   **Context**: `app.response_time`
    *   **Instances**: `endpoint1`, `endpoint2`
    *   **Dimensions**: `p50`, `p95`, `p99` (milliseconds)
*   **Context**: `app.active_connections`
    *   **Instances**: `server1`, `server2`
    *   **Dimensions**: `active`, `idle` (connections)

### NIDL and Data Ingestion

When ingesting metrics from other observability solutions (e.g., Prometheus), it's common to encounter multi-dimensional metrics that combine several instance types or hierarchical levels into a single metric.

**Prometheus-style (example)**:
`mysql_operations{server="prod1", database="db1", table="users", operation="read"} = 1234`

This single metric contains enough information to extract server-level, database-level, and table-level views. With NIDL, you could technically import this as-is and use the dropdown menus to aggregate by different labels.

**However, this misses the bigger picture of dashboard design**.

Consider how you'd structure a dashboard if designing it by hand:
- **Server section**: Server health, total load, overall performance
- **Database section**: Per-database metrics like connections, table count, size, plus aggregated performance from tables
- **Table section**: Detailed table-level operations, row counts, index usage

Each section tells its own story with metrics appropriate to that level.

**Netdata's Best Practice**: Create this natural structure through separate contexts:

*   `mysql.operations` - Server-level view with server-specific metrics
*   `mysql.db.operations` - Database-level view combining:
    - Native database metrics (connections, table count)
    - Pre-aggregated table metrics for this level
*   `mysql.table.operations` - Detailed table-level metrics

This approach delivers:
*   **Intuitive dashboard structure** - Users immediately understand what each section represents
*   **Complete metrics at each level** - Not just aggregations, but level-specific insights
*   **Natural navigation** - From overview (server) to specific (tables)
*   **Clear mental model** - Each chart answers questions appropriate to its level

The key insight: Pre-aggregation isn't just about performance - it's about creating a well-structured dashboard where each section has a clear purpose and tells a complete story.

### Practical Guide: Designing NIDL-Compliant Collectors

This guide walks through the thought process of designing metrics for a new collector, using database servers (PostgreSQL) and application servers (WebSphere) as examples.

#### Step 1: Identify Key Components and Characteristics

**Question to answer**: What are the major functional areas of this application that need monitoring?

**Example - PostgreSQL**:
- Server health & connections
- Query performance
- Database operations
- Table operations
- Replication status
- Storage engine internals

**Example - WebSphere**:
- JVM health (memory, GC, threads)
- Web container (servlets, sessions, JSP)
- Connection pools (JDBC, JMS)
- Message queuing
- Security & transactions

#### Step 2: Organize into Families

**Decision**: Flat structure (\<10 families) or Tree structure (\>10 families)?

**PostgreSQL - Flat Structure**:
```
connections
queries  
databases
tables
replication
```

**WebSphere - Tree Structure**:
```
jvm/memory
jvm/gc
jvm/threads
web/servlets
web/sessions
connections/jdbc
connections/jms
```

**Rules**:
- No family can have both charts and subfamilies
- Use "overview" only for metrics that don't fit subfamilies
- Each leaf should have 3+ charts to justify existence

#### Step 3: Validate Metric Belonging and Instance Consistency

For each family/subfamily:

1. **List all metrics** that belong here
2. **Verify they're all about the same thing** (e.g., all about servlets)
3. **Identify the instance type** for each metric
4. **Ensure 90%+ share the same instance definition**

**Example - web/servlets**:
```
✓ servlet.requests      → Instance: each servlet
✓ servlet.response_time → Instance: each servlet  
✓ servlet.errors        → Instance: each servlet
? servlet.total_count   → Instance: server (put first as summary)
✗ session.count         → Wrong topic! (move to web/sessions)
```

#### Step 4: Group Metrics into Contexts

For each group of related metrics with the same instance type:

1. **Identify shared characteristics**:
   - Same unit of measurement
   - Related dimensions that sum meaningfully
   - Tell one coherent story

2. **Create contexts**:

**Example - PostgreSQL Tables**:
```
Context: postgres.table.operations
Instances: users_table, orders_table, products_table
Dimensions: select, insert, update, delete
Unit: operations/s
Title: "Table Operations"

Context: postgres.table.size
Instances: users_table, orders_table, products_table  
Dimensions: size, indexes_size
Unit: bytes
Title: "Table Size"
```

**Common mistake to avoid**:
```
WRONG - Mixed dimensions per instance:
Context: postgres.table.operations
Instance: users_table → Dimensions: select, insert, update
Instance: orders_table → Dimensions: select, delete  // Missing insert, update!
```

#### Step 5: Validate Context Design

For each context, verify:

- [ ] **One instance type** (don't mix tables with databases)
- [ ] **One unit** (don't mix bytes with operations/s)
- [ ] **Consistent dimensions** across all instances
- [ ] **Meaningful aggregations** (summing dimensions makes sense)
- [ ] **Clear title** that explains what's being measured

#### Step 6: Handle Edge Cases

**Summary metrics** (different instance type):
- Place at the beginning of the section
- Or move to parent/overview section

**Rate metrics**:
- Always specify time unit: "requests/s", not "requests"
- Use Incremental algorithm on raw counters

**Complex hierarchies**:
- Create separate contexts for each level
- Pre-aggregate at collection time
- Don't rely on query-time aggregation

#### Complete Example: PostgreSQL Tables Section

```
Family: tables

1. postgres.table.count
   Instance: server
   Dimensions: total_tables
   Unit: tables
   Title: "Total Tables"
   (Summary chart - goes first)

2. postgres.table.operations  
   Instances: each table
   Dimensions: select, insert, update, delete
   Unit: operations/s
   Title: "Table Operations"

3. postgres.table.size
   Instances: each table
   Dimensions: data_size, indexes_size  
   Unit: bytes
   Title: "Table Size"

4. postgres.table.maintenance
   Instances: each table
   Dimensions: vacuum_time, analyze_time
   Unit: seconds
   Title: "Table Maintenance"
```

#### Final Checklist

- [ ] Each family represents a major functional area
- [ ] Navigation structure is intuitive (\<10 flat, \>10 tree)
- [ ] All metrics in a family are about the same topic
- [ ] Each context has consistent instance types
- [ ] All instances in a context have identical dimensions
- [ ] Units are consistent within each context
- [ ] Summary metrics are properly positioned
- [ ] Rate metrics include time units
- [ ] Chart titles clearly describe what's measured

Following this guide ensures your collector creates a coherent, navigable dashboard that tells clear stories about each aspect of the monitored application.

### The Role of Netdata's Storage Efficiency

Netdata's highly efficient storage engine (0.5 bytes per sample on the high-resolution tier) is crucial for the NIDL framework's success. This efficiency allows Netdata to:

*   **Collect data at multiple levels** (e.g., server, database, table) without significant storage penalties.
*   **Pre-summarize at collection time**, rather than relying on expensive query-time aggregations.
*   **Maintain full granularity** at each level, ensuring no loss of detail.

This means that the NIDL discipline of creating separate contexts for each level is not just about clarity; it's also a performance optimization. By doing the "heavy lifting" once at collection time, Netdata ensures fast dashboards and instant responses for users.

## Conclusion

The NIDL framework is the backbone of Netdata's "no query language needed" philosophy. It empowers users with intuitive, interactive data exploration capabilities. However, this power comes with a critical responsibility for developers: to design metrics thoughtfully and adhere strictly to NIDL principles.

By embracing the NIDL framework, collector developers are not merely writing data collection code; they are designing the entire observability experience. Their discipline in defining coherent contexts, consistent instances, related dimensions, and meaningful labels directly translates into clear, actionable, and automatically generated dashboards that empower every Netdata user.
