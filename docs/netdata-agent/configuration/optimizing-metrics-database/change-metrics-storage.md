# Change how long Netdata stores metrics

Netdata offers a granular approach to data retention, allowing you to manage storage based on both **time** and **disk
space**. This provides greater control and helps you optimize storage usage for your specific needs.

**Default Retention Limits**:

| Tier |     Resolution      | Time Limit | Size Limit (min 256 MB) |
|:----:|:-------------------:|:----------:|:-----------------------:|
|  0   |  high (per second)  |    14d     |          1 GiB          |
|  1   | middle (per minute) |    3mo     |          1 GiB          |
|  2   |   low (per hour)    |     2y     |          1 GiB          |

> **Note**: If a user sets a disk space size less than 256 MB for a tier, Netdata will automatically adjust it to 256 MB.

With these defaults, Netdata requires approximately 4 GiB of storage space (including metadata).

## Retention Settings

> **In a parent-child setup**, these settings manage the shared storage space used by the Netdata parent Agent for storing metrics collected by both the parent and its child nodes.

You can fine-tune retention for each tier by setting a time limit or size limit. Setting a limit to 0 disables it,
allowing for no time-based deletion for that tier or using all available space, respectively. This enables various
retention strategies as shown in the table below:

| Setting                        | Retention Behavior                                                                                                                        |
|--------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| Size Limit = 0, Time Limit > 0 | **Time-based only:** data is stored for a specific duration regardless of disk usage.                                                     |
| Time Limit = 0, Size Limit > 0 | **Space-based only:** data is stored until it reaches a certain amount of disk space, regardless of time.                                 |
| Time Limit > 0, Size Limit > 0 | **Combined time and space limits:** data is deleted once it reaches either the time limit or the disk space limit, whichever comes first. |

You can change these limits in `netdata.conf`:

```text
[db]
    mode = dbengine
    storage tiers = 3

    # Tier 0, per second data. Set to 0 for no limit.
    dbengine tier 0 retention size = 1GiB
    dbengine tier 0 retention time = 14d

    # Tier 1, per minute data. Set to 0 for no limit.
    dbengine tier 1 retention size = 1GiB
    dbengine tier 1 retention time = 3mo

    # Tier 2, per hour data. Set to 0 for no limit.
    dbengine tier 2 retention size = 1GiB
    dbengine tier 2 retention time = 2y
```

## Monitoring Retention Utilization

Netdata provides a visual representation of storage utilization for both time and space limits across all tiers within
the 'dbengine retention' subsection of the 'Netdata Monitoring' section on the dashboard. This chart shows exactly how
your storage space (disk space limits) and time (time limits) are used for metric retention.

## Legacy configuration

### v1.99.0 and prior

Netdata prior to v2 supports the following configuration options in  `netdata.conf`.
They have the same defaults as the latest v2, but the unit of each value is given in the option name, not at the value.

```text
storage tiers = 3
# Tier 0, per second data. Set to 0 for no limit.
dbengine tier 0 disk space MB = 1024
dbengine tier 0 retention days = 14
# Tier 1, per minute data. Set to 0 for no limit.
dbengine tier 1 disk space MB = 1024
dbengine tier 1 retention days = 90
# Tier 2, per hour data. Set to 0 for no limit.
dbengine tier 2 disk space MB = 1024
dbengine tier 2 retention days = 730
```

### v1.45.6 and prior

Netdata versions prior to v1.46.0 relied on a disk space-based retention.

**Default Retention Limits**:

| Tier |     Resolution      | Size Limit |
|:----:|:-------------------:|:----------:|
|  0   |  high (per second)  |   256 MB   |
|  1   | middle (per minute) |   128 MB   |
|  2   |   low (per hour)    |   64 GiB   |

You can change these limits in `netdata.conf`:

```text
[db]
    mode = dbengine
    storage tiers = 3
    # Tier 0, per second data
    dbengine multihost disk space MB = 256
    # Tier 1, per minute data
    dbengine tier 1 multihost disk space MB = 1024
    # Tier 2, per hour data
    dbengine tier 2 multihost disk space MB = 1024
```

### v1.35.1 and prior

These versions of the Agent do not support tiers. You could change the metric retention for the parent and
all of its children only with the `dbengine multihost disk space MB` setting. This setting accounts the space allocation
for the parent node and all of its children.

To configure the database engine, look for the `page cache size MB` and `dbengine multihost disk space MB` settings in
the `[db]` section of your `netdata.conf`.

```text
[db]
    dbengine page cache size MB = 32
    dbengine multihost disk space MB = 256
```

### v1.23.2 and prior

_For Netdata Agents earlier than v1.23.2_, the Agent on the parent node uses one dbengine instance for itself, and
another instance for every child node it receives metrics from. If you had four streaming nodes, you would have five
instances in total (`1 parent + 4 child nodes = 5 instances`).

The Agent allocates resources for each instance separately using the `dbengine disk space MB` (**deprecated**) setting.
If `dbengine disk space MB`(**deprecated**) is set to the default `256`, each instance is given 256 MiB in disk space,
which means the total disk space required to store all instances is,
roughly, `256 MiB * 1 parent * 4 child nodes = 1280 MiB`.

#### Backward compatibility

All existing metrics belonging to child nodes are automatically converted to legacy dbengine instances and the localhost
metrics are transferred to the multihost dbengine instance.

All new child nodes are automatically transferred to the multihost dbengine instance and share its page cache and disk
space. If you want to migrate a child node from its legacy dbengine instance to the multihost dbengine instance, you
must delete the instance's directory, which is located in `/var/cache/netdata/MACHINE_GUID/dbengine`, after stopping the
Agent.
