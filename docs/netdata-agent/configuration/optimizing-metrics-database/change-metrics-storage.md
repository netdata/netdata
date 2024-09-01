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

> **In a parent-child setup**, these settings manage the shared storage space used by the Netdata parent agent for
> storing metrics collected by both the parent and its child nodes.

You can fine-tune retention for each tier by setting a time limit or size limit. Setting a limit to 0 disables it,
allowing for no time-based deletion for that tier or using all available space, respectively. This enables various
retention strategies as shown in the table below:

| Setting                        | Retention Behavior                                                                                                                        |
|--------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|
| Size Limit = 0, Time Limit > 0 | **Time-based only:** data is stored for a specific duration regardless of disk usage.                                                     |
| Time Limit = 0, Size Limit > 0 | **Space-based only:** data is stored until it reaches a certain amount of disk space, regardless of time.                                 |
| Time Limit > 0, Size Limit > 0 | **Combined time and space limits:** data is deleted once it reaches either the time limit or the disk space limit, whichever comes first. |

You can change these limits in `netdata.conf`:

```
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

#### Backward compatibility

All existing metrics belonging to child nodes are automatically converted to legacy dbengine instances and the localhost
metrics are transferred to the multihost dbengine instance.

All new child nodes are automatically transferred to the multihost dbengine instance and share its page cache and disk
space. If you want to migrate a child node from its legacy dbengine instance to the multihost dbengine instance, you
must delete the instance's directory, which is located in `/var/cache/netdata/MACHINE_GUID/dbengine`, after stopping the
Agent.
