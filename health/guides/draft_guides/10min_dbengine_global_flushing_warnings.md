# 10min_dbengine_global_flushing_warnings

**Netdata | DB engine**

*The Database Engine works like a traditional database. It dedicates a certain amount of RAM to data caching and
indexing, while the rest of the data resides compressed on disk. Unlike other memory modes, the amount of historical
metrics stored is based on the amount of disk space you allocate and the effective compression ratio, not a fixed number
of metrics collected.*

By using both RAM and disk space, the database engine allows for long-term storage of per-second metrics inside of the
Agent itself.

The Netdata Agent monitors the number of times when `dbengine` dirty pages were over 50% of the instance page cache in
the last 10 minutes. In this situation, the metric data are at risk of not being stored in the database. To remedy this
issue, reduce disk load or use faster disks.

This alert is triggered in warn state when the number of `dbengine` dirty pages which were over 50% of the instance is
greater than 0.

[See more about DB engine](https://learn.netdata.cloud/docs/agent/database/engine)

![](https://drive.google.com/uc?export=view&id=1elXR92OQn3sWVGXUCjpGi-NwcLNYE24g)
