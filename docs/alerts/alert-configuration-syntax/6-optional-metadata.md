# 3.6 Optional Metadata: class, type, component, and labels

Alert definitions in Netdata can include optional metadata fields that classify, describe, and scope alerts without affecting their evaluation logic. These fields control where alerts appear in UIs, how they're filtered and grouped, and who gets notified.

:::tip

Refer to this section when you're organizing alerts for teams to filter by type or component, writing `summary` and `info` text for UI display and notifications, scoping alerts to specific hosts or chart instances using labels, or routing notifications to different teams based on alert classification.

:::

:::note

Netdata uses "labels" for tagging and scoping, not a `tags:` configuration line. The `host labels:` and `chart labels:` directives provide equivalent functionality.

:::

## 3.6.1 What These Metadata Fields Are For

These fields are optional, but recommended for any alert you expect to manage centrally in Netdata Cloud, use in silencing rules or notification routing, or hand over to different teams (SRE, DBAs, network, application teams).

<details>
<summary><strong>Netdata's alert metadata serves four primary purposes:</strong></summary>
Netdata's alert metadata serves four primary purposes: classification and organization (group alerts by signal type, functional domain, and specific technology), UI filtering and grouping (filter and search in Cloud Events Feed and Agent dashboard), notification routing (send database alerts to DBAs, system alerts to sysadmins), and operational consistency (standardized naming and team workflows).

</details>

**Available metadata fields:**

| Field | Purpose |
|-------|---------|
| `class` | What kind of signal this alert monitors (Errors, Latency, Utilization, Workload) |
| `type` | Functional domain (System, Database, Containers, Web Server, etc.) |
| `component` | Specific technology or subsystem (CPU, PostgreSQL, Docker, nginx, etc.) |
| `summary` | Short title for UI display and notifications |
| `info` | Detailed description with context and remediation guidance |
| `host labels` | Pattern to match which hosts receive this alert |
| `chart labels` | Pattern to filter which chart instances receive this alert |

:::note

These fields show up in Netdata Cloud alert views and event feeds, filtering UI, silencing rules (Chapter 4), and notification routing (Chapter 5). They do not affect how often the alert fires or what its thresholds are.

:::

## 3.6.2 `class`: High-Level Category of the Problem

`class` categorizes the alert in terms of what kind of signal it monitors (the nature of the problem).

**Recommended values:**

| Class | Use Case | Examples |
|-------|----------|----------|
| `Errors` | Error rate monitoring | HTTP 5xx errors, disk read errors, packet drops |
| `Latency` | Response time issues | Query latency, API response time, disk I/O wait |
| `Utilization` | Resource usage monitoring | CPU %, memory %, disk space %, bandwidth % |
| `Workload` | Load and throughput monitoring | Queue depth, requests/sec, connections, transactions |

**Default if omitted:** Unknown

**Example:**

```conf
template: disk_space_usage
       on: disk.space
   calc: $used * 100 / ($avail + $used)
    warn: $this > (($status >= $WARNING) ? (80) : (90))
    crit: ($this > (($status == $CRITICAL) ? (90) : (98))) && $avail < 5
   class: Utilization
    type: System
 component: Disk
```

In this example, `class: Utilization` indicates this is a resource usage alert. SREs can filter for all `Utilization` class alerts when doing capacity planning. You can build silencing rules that target all `Utilization` alerts in non-production environments.

:::tip

**Guidelines for `class`:**

Keep the set of classes small and consistent (use the four recommended values). Prefer signal-based categories (Errors, Latency, Utilization, Workload) that align with monitoring best practices. Document your class vocabulary in your team's runbooks.

:::

## 3.6.3 `type`: Technical Nature of the Symptom

`type` describes the functional domain of the infrastructure this alert covers (broader category).

**Common values** (partial list, see Netdata REFERENCE.md for complete taxonomy):

| Type | Description | Examples |
|------|-------------|----------|
| `System` | General system resources | CPU, memory, disk, network |
| `Database` | Database systems | MySQL, PostgreSQL, MongoDB |
| `Containers` | Container platforms | Docker, Podman |
| `Kubernetes` | Kubernetes orchestration | K8s nodes, pods |
| `Web Server` | Web servers | Apache, nginx |
| `Messaging` | Message brokers | RabbitMQ, Kafka |
| `Search engine` | Search services | Elasticsearch, Solr |

**Default if omitted:** Unknown

**Example:**

```conf
alarm: httpcheck_web_service_bad_status
    on: httpcheck.status
lookup: average -5m of bad_status
   calc: 100 * $this / $success
   warn: $this > 1
   crit: $this > 5
  class: Errors
   type: Web Server
component: nginx
```

Here, `class: Errors` indicates error rate monitoring, and `type: Web Server` identifies the functional domain.

**Guidelines for `type`:**

Use `type` to describe the functional area (System, Database, Containers) rather than the specific problem. Combine with `class` to understand both business impact and functional domain. Use the documented taxonomy from Netdata's stock alerts for consistency.

## 3.6.4 `component`: Subsystem or Ownership Boundary

`component` tells you which specific technology or subsystem this alert monitors (most granular level).

**Common patterns** (from Netdata's stock alerts):

| Type | Typical Components |
|------|-------------------|
| `System` | `CPU`, `Memory`, `Disk`, `Network`, `Power` |
| `Database` | `MySQL`, `PostgreSQL`, `Redis`, `MongoDB` |
| `Containers` | `Docker`, `Podman`, `LXC` |
| `Kubernetes` | `kubelet`, `kube-proxy`, `etcd` |
| `Web Server` | `Apache`, `nginx` |
| `Messaging` | `RabbitMQ`, `Kafka` |

:::note

**Unlike `class` and `type`, component values are not formally enumerated** in Netdata's documentation. Choose values that align with your service names, teams, or operational boundaries, and maintain consistency across your organization.

:::

**Example:**

```conf
alarm: mysql_connections
    on: mysql.connections
lookup: average -5m of connected_clients
   warn: $this > 150
   crit: $this > 190
  class: Utilization
   type: Database
component: MySQL
```

Here, `component: MySQL` makes it obvious this belongs to the database team monitoring MySQL specifically. Cloud views and dashboards can be filtered by `component` so DBAs only see their alerts.

**Guidelines for `component`:**

Use product names consistently (for example, `PostgreSQL` not `Postgres`, `Elasticsearch` not `ES`). Capitalize consistently (for example, `CPU` not `cpu`, `MySQL` not `mysql`). Align `component` names with how you structure teams, services, or documentation.

## 3.6.5 How Metadata Is Used in Netdata Cloud

While the Netdata Agent's health engine ignores metadata for evaluation purposes, Netdata Cloud uses these fields for:

| Use Case | Description | See Also |
|----------|-------------|----------|
| **Alert list filtering and search** | Quickly find alerts by `class`, `component` | Cloud UI |
| **Events feed filtering** | Isolate events from a particular subsystem or impact area | Cloud Events Tab |
| **Silencing rules** | Match rules based on alert name, context, and metadata | **4.3 Silencing in Netdata Cloud** |
| **Notification routing** | Route different classes/components to different integrations or roles | **Chapter 5 Receiving Notifications** |

Examples of Cloud workflows that rely on metadata:

- "Mute all `class: capacity` alerts for `component: storage` in the `staging` room during this maintenance window."
- "Send `class: availability` alerts for `component: payments-service` to the on-call team's PagerDuty integration."
- "Show me only `class: latency` alerts from `component: api` over the last 24 hours in the events feed."

Having metadata consistently set across alerts is what makes these workflows reliable.

## 3.6.6 Designing a Metadata Scheme for Your Environment

To avoid inconsistent values (for example, `db`, `database`, `Database`), define a simple metadata schema for your organization:

<details>
<summary><strong>Step-by-Step Metadata Design</strong></summary>

1. **Agree on a small fixed set of `class` values**
   
   Example: `availability`, `performance`, `capacity`, `reliability`, `security`

2. **Define recommended `type` values aligned with your SRE vocabulary**
   
   Example: `latency`, `error`, `saturation`, `traffic`, `anomaly`

3. **Map `component` values to your services or teams**
   
   Example: `database.mysql`, `application.api`, `network.edge`, `storage.ceph`

4. **Document the scheme** in your runbooks and code review guidelines for alert definitions

5. **Review metadata periodically** (see **12.3 Maintaining Alert Configurations Over Time**) to remove obsolete components and ensure consistency

</details>

## 3.6.7 Example: Fully Annotated Alert

Below is a complete example that pulls together all metadata fields with the configuration concepts from Chapter 3:

```conf
template: httpcheck_web_service_bad_status
       on: httpcheck.status
    lookup: average -5m unaligned percentage of bad_status
      calc: $this
      units: %
       warn: $this >= 10 AND $this < 40
       crit: $this >= 40
      delay: down 5m multiplier 1.5 max 1h
        to: webmaster
     class: Workload
      type: Web Server
  component: HTTP endpoint
    summary: HTTP check for ${label:url} unexpected status
       info: Percentage of HTTP responses from ${label:url} with unexpected status in the last 5 minutes
```

In this definition:

- **Alert logic** (what fires when): `lookup`, `calc`, `warn`, `crit`, `every`, `delay`, `to`
- **Metadata** (how we organize/filter it): `class`, `type`, `component`
- **User-facing text**: `summary`, `info` with template substitutions (`${label:service}`)

:::note

When this alert appears in Cloud, SREs can filter for `class: Workload` or `component: HTTP endpoint`. On-call routing can treat all `HTTP endpoint` workload alerts the same. Silencing rules can target specific components during controlled maintenance.

:::

## Key takeaway 

Treat metadata as part of the design of your alerting system, not as an afterthought. A small, consistent vocabulary for `class`, `type`, and `component` pays off quickly when working with Netdata Cloud, silencing rules, and on-call workflows.

## What's Next

- **[Chapter 4: Controlling Alerts and Noise](../controlling-alerts-noise/index.md)** explains how to disable or silence alerts and use delays/hysteresis to reduce noise
- **[5.4 Controlling Who Gets Notified (Roles, Recipients, Severity)](../receiving-notifications/4-controlling-recipients.md)** shows how metadata and alert properties influence notification routing
- **[12.1 Designing Useful Alerts](../best-practices/1-designing-useful-alerts.md)** and **[12.3 Maintaining Alert Configurations Over Time](../best-practices/3-maintaining-configurations.md)** provide best practices for treating metadata as part of your alert design and lifecycle