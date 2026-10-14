### Understand the alert

The Database Engine works like a traditional database. It dedicates a certain amount of RAM to data caching and indexing, while the rest of the data resides compressed on disk. Unlike other memory modes, the amount of historical metrics stored is based on the amount of disk space you allocate and the effective compression ratio, not a fixed number of metrics collected.

By using both RAM and disk space, the database engine allows for long-term storage of per-second metrics inside of the Netdata Agent itself.

Netdata monitors the number of pages deleted due to failure to flush data to disk in the last 10 minutes. In this situation some metric data was dropped to unblock data collection. To remedy this issue, reduce disk load or use
faster disks. This alert is triggered in critical state when the number deleted pages is greater than 0.

### Useful resources

[Read more about Netdata DB engine](/src/database/engine/README.md)

