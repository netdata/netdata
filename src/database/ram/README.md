# RAM database modes

With `[db].db = ram` or `alloc`, each dimension keeps its recent samples in memory only. They are lost when the Agent
stops.

- **Agents built with the Database Engine** (the default): `ram` and `alloc` are the same. Tier 0 is an in-memory
  Database Engine instance: samples are Gorilla-compressed in pages, and no files are written. `[db].retention` is the
  number of samples each dimension aims to keep, so the retained time is about that number multiplied by each chart's
  collection interval. It is a target: pages are released whole once they end before the retention, so a dimension can
  keep more samples than configured. Memory grows with the samples actually stored; it is not allocated up front. When Netdata can detect the
  system memory and the system has less free memory than `[db].dbengine out of memory protection` (by default 10% of
  RAM, up to 5 GiB), the Database Engine releases pages early, the oldest that are not being queried first, and the
  retention gets shorter.
- **Agents built without the Database Engine** (`--disable-dbengine`): `ram` and `alloc` use the round-robin buffers
  in this directory, four bytes per sample, allocated when a dimension is created. `ram` rounds each buffer up to
  whole memory pages and lets the kernel deduplicate them (KSM); `alloc` allocates the exact number of samples.

`[db].db = none` keeps no samples (`src/database/storage-none.c`).
