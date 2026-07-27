# Extreme cardinality protection

Extreme cardinality occurs when a node continually creates new, short-lived chart instances. Common causes include ephemeral containers and collectors that repeatedly discover transient resources.

Netdata's extreme cardinality protection limits the long-term resource cost of this churn. It removes access to selected historical metrics after they have no samples left in tier 0, allowing their obsolete metadata to be cleaned up later.

:::warning

The protection deliberately makes some long-term historical samples inaccessible. Review the thresholds before lowering them or enabling the protection in an environment where old, short-lived instances must remain queryable.

:::

## What the protection measures

Netdata evaluates cardinality independently for each node and context:

- A **context** groups charts that describe the same type of resource, such as disks, network interfaces, or containers.
- An **instance** is one chart within that context, such as a specific disk or container.
- A **metric** is one dimension within an instance.

An instance is eligible for cleanup only after all its remaining metrics have no retention in tier 0. Netdata calculates the context's ephemerality as the percentage of its retained instances that meet this condition.

Counts shown in dashboards can aggregate multiple nodes. They do not necessarily match the per-node, per-context counts used by this protection.

## When cleanup runs

The protection is enabled by default when the Agent uses `dbengine` with more than one storage tier. It is disabled by default for other database configurations.

After the first real rotation in any `dbengine` tier, Netdata schedules a full retention scan approximately two minutes later. During that scan, a context is eligible only when:

- the protection is enabled;
- the number of instances without tier-0 retention is at least `extreme cardinality keep instances`; and
- those instances represent at least `extreme cardinality min ephemerality` percent of all retained instances in the context.

Netdata retains at least the larger of:

- the configured `extreme cardinality keep instances` value; or
- the configured ephemerality percentage of all instances in the context.

Only eligible instances above that retained amount are cleaned. Meeting both thresholds does not guarantee that anything will be removed; there must also be eligible instances above the amount Netdata keeps.

With the defaults, for example, a context with 3,000 instances and 2,000 instances without tier-0 retention has 66% ephemerality. Netdata keeps 1,500 of those instances and clears long-term retention from up to 500 eligible instances.

## What cleanup keeps and removes

Netdata never selects an instance or metric that is currently being collected, even if it has not yet established tier-0 retention.

For selected archived instances, Netdata clears the retention records for eligible metrics in every configured storage tier. Those historical samples become unavailable to queries immediately. The operation does not rewrite or compact existing `dbengine` datafiles; their physical blocks continue through normal database rotation.

Netdata emits a notice only when it actually clears retention from at least one metric.

## Metadata cleanup and disk space

Metric metadata, including chart definitions, dimensions, and chart labels, is stored in `netdata-meta.db` in the Agent's [cache directory](/docs/netdata-agent/backup-and-restore-an-agent.md). The directory varies by operating system and installation method.

Clearing metric retention does not synchronously delete its SQLite metadata. Metadata maintenance:

- starts its first scan about 30 minutes after the Agent starts;
- processes large tables in bounded batches;
- rescans dimensions weekly; and
- deletes a chart and its labels after the chart's final dimension is removed.

Depending on when retention is cleared relative to the scan, obsolete rows can remain until a later weekly pass.

Deleted rows first become free pages that SQLite can reuse inside `netdata-meta.db`. With the default `[sqlite] auto vacuum = INCREMENTAL` setting, Netdata checks for reclaimable pages at most once per minute. It requests an incremental vacuum only when free pages exceed 5% of the database, and each pass requests 10% of the current free pages.

As a result:

- the database file can shrink gradually after enough pages become free;
- smaller amounts of free space can remain allocated and be reused by later writes;
- cleanup and file shrinkage do not have a fixed completion time;
- with `NONE` or `OFF` auto-vacuum, freed pages remain available for reuse but are not automatically returned by this mechanism; and
- `FULL` auto-vacuum follows SQLite's separate full-auto-vacuum behavior.

These outcomes are normal and do not mean the cleanup failed. See the [SQLite configuration reference](/src/daemon/config/README.md) for the `auto vacuum` setting.

## Configure the protection

The settings are in the `[db]` section of `netdata.conf`:

| Setting                                | Default                                         | Set to               | Effect                                                                                                                                        |
|:---------------------------------------|:------------------------------------------------|:---------------------|:----------------------------------------------------------------------------------------------------------------------------------------------|
| `extreme cardinality protection`       | `yes` for multi-tier `dbengine`; otherwise `no` | `yes` or `no`        | Enables or disables forced cardinality cleanup. Normal size- and time-based retention still expires data when this protection is disabled.    |
| `extreme cardinality keep instances`   | `1000`                                          | `1` to `1000000`     | Sets the minimum eligible-instance count and a lower bound on how many eligible instances remain. Higher values make cleanup less aggressive. |
| `extreme cardinality min ephemerality` | `50`                                            | `0` to `100` percent | Sets the minimum ephemerality and contributes to the number of eligible instances retained. Higher values make cleanup less aggressive.       |

The defaults need no manual configuration for a multi-tier `dbengine` Agent. To retain more short-lived instances, use higher thresholds, for example:

```ini
[db]
    extreme cardinality keep instances = 2000
    extreme cardinality min ephemerality = 75
```

Use the [Netdata Agent configuration guide](/docs/netdata-agent/configuration/README.md#edit-configuration-files) to locate and edit `netdata.conf`, then [restart the Agent](/docs/netdata-agent/start-stop-restart.md) for the changes to take effect.

## Verify cleanup

When cleanup removes retention, the notice follows this pattern:

```text
EXTREME CARDINALITY PROTECTION: host '<HOST>', context '<CONTEXT>', total active instances <TOTAL>, not in tier0 <WITHOUT_TIER0>, ephemerality <PERCENT>%: forcefully cleared the retention of <METRICS> metrics and <INSTANCES> instances, having non-tier0 retention from <START_TIME> to <END_TIME>.
```

The event uses this message ID:

```text
d1f59606dd4d41e3b217a0cfcae8e632
```

On Linux systems that use the Netdata journal namespace, query it with:

```bash
sudo journalctl --namespace=netdata MESSAGE_ID=d1f59606dd4d41e3b217a0cfcae8e632
```

You can also open the **Logs** tab and filter `MESSAGE_ID` by **Netdata extreme cardinality**. For platform-specific log access and permission requirements, see [Netdata Logging](/src/libnetdata/log/README.md#platform-specific-log-access).
