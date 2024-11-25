### Understand the alert

The Database Engine works like a traditional database. It dedicates a certain amount of RAM to data caching and indexing, while the rest of the data resides compressed on disk. Unlike other memory modes, the amount of historical metrics stored is based on the amount of disk space you allocate and the effective compression ratio, not a fixed number of metrics collected.

By using both RAM and disk space, the database engine allows for long-term storage of per-second metrics inside of the Netdata Agent itself.

Netdata monitors the number of filesystem errors in the last 10 minutes. The Dbengine is experiencing filesystem errors (too many open files, wrong permissions, etc.)

This alert is triggered in warning state when the number of filesystem errors is greater than 0.

### Useful resources

[Read more about Netdata DB engine](/src/database/engine/README.md)

