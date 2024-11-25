### Understand the alert

The Database Engine works like a traditional database. It dedicates a certain amount of RAM to data caching and indexing, while the rest of the data resides compressed on disk. Unlike other memory modes, the amount of historical metrics stored is based on the amount of disk space you allocate and the effective compression ratio, not a fixed number of metrics collected.

By using both RAM and disk space, the database engine allows for long-term storage of per-second metrics inside of the Netdata Agent itself.

The Netdata Agent monitors the number of IO errors in the last 10 minutes. The dbengine is experiencing I/O errors (CRC errors, out of space, bad disk, etc.).

This alert is triggered in critical state when the number of IO errors is greater that 0.

### Useful resources

[Read more about Netdata DB engine](/src/database/engine/README.md)

