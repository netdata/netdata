### Understand the alert

The Database Engine works like a traditional database. It dedicates a certain amount of RAM to data caching and indexing, while the rest of the data resides compressed on disk. Unlike other memory modes, the amount of historical metrics stored is based on the amount of disk space you allocate and the effective compression ratio, not a fixed number
of metrics collected.

By using both RAM and disk space, the database engine allows for long-term storage of per-second metrics inside of the Netdata Agent itself.

Netdata monitors the number of times when `dbengine` dirty pages were over 50% of the instance page cache in the last 10 minutes. In this situation, the metric data are at risk of not being stored in the database. To remedy this issue, reduce disk load or use faster disks.

This alert is triggered in warn state when the number of `dbengine` dirty pages which were over 50% of the instance is greater than 0.

### Useful resources

[Read more about Netdata DB engine](/src/database/engine/README.md)

