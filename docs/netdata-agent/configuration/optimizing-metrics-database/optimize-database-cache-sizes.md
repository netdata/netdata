# Optimize Database cache sizes


There are two cache sizes that can be configured in `netdata.conf`:

1. `[db].dbengine page cache size`: this is the main cache that keeps metrics data into memory. When data is not found in it, the extent cache is consulted, and if not found in that too, they are loaded from the disk.
2. `[db].dbengine extent cache size`: this is the compressed extent cache. It keeps in memory compressed data blocks, as they appear on disk, to avoid reading them again. Data found in the extent cache but not in the main cache have to be uncompressed to be queried.

Both of them are dynamically adjusted to use some of the total memory computed above. The configuration in `netdata.conf` allows providing additional memory to them, increasing their caching efficiency.