# DEM process runtime owner

`Host` connects accepted enabled configuration to one immutable installed runtime
manifest. The caller supplies the store and trusted manifest, or an unavailable
reason when managed setup is absent. That setup failure does not stop RUM.

`Prepare` reserves a process-owned claim without downloads or idle timestamp
changes. `Commit` transfers identity ownership in memory and wakes the worker;
`Abort` releases only the pending reservation. The durable pending reservation
already protects the generation. The worker persists accepted demand separately
from bundle readiness and retains protection on a failed commit; enabling another
job cannot interrupt a prepared runtime. Overlapping run owners of the same
configuration identity are counted separately. Replacement
retires the previous claim only after the new reservation exists.

`Run` owns preparation and maintenance. It starts at most one cancellable download,
retries failed preparation, polls for a matching import by another store consumer,
and sweeps at startup and hourly even with no browser jobs. Download prohibition
and a missing installed URL produce setup unavailability; cached or imported
matching files can still become ready. There is no global configuration replay
barrier: expired or pressured cache entries can disappear before saved enables
arrive. Preparation and cleanup run outside the configuration mutation lane.

`Ready` and `Dependencies` are pure memory lookups. Each run subscribes to named
availability notifications before checking readiness and unregisters after joining.
Notifications are dispatched outside the host mutex. A successful lookup is not an
execution lease: every runner probe or invocation must acquire `BeginUse`, pass its
completion handle to the supervised execution, then finish with real drainage proof.

`Status` reports preparation and maintenance errors independently of read-only store
inventory. A usable matching runtime does not become unavailable solely because
claim bookkeeping or cleanup failed. Setup failures for enabled jobs and maintenance
failures are logged when they change; repeated polls and zero-job cache misses are
quiet. Maintenance operations retain independent errors until their own successful
retry. Execution-finish errors are recorded in both owner and store health, and
cleanup retries failed retirement without changing a completed monitoring outcome.
Stale readiness is checked again when acquiring each execution pin.

The process calls `Close` after all jobs and configuration transitions have joined.
Close cancels and joins the worker and downloader, releases demand, and leaves the
caller-supplied Store open. The caller then closes the Store. Retained uncertain
execution pins remain governed by the Store's independent completion proof.
