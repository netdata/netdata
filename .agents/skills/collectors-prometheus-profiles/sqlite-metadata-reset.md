# SQLite metadata during profile iteration

Do not reset Netdata SQLite metadata as a routine profile-authoring step.
Profile correctness is proved by the collector/chartengine validation path and
live advancing charts, not by deleting historical metadata until the dashboard
looks clean.

## Why stale entries can remain

Contexts, chart IDs, dimensions, and labels are durable identity surfaces. A
profile edit can create a new identity while the previous chart metadata and
stored history remain available until normal lifecycle/retention removes or
ages them. A restart does not imply that all historical identities disappear.

This is normally desirable:

- history remains queryable across temporary configuration changes;
- rollback can reconnect to the previous identity;
- metadata convergence and data retention follow their own lifecycle instead of
  destructive authoring shortcuts.

First distinguish three cases:

1. **Current profile defect:** the live job is still creating unwanted autogen,
   duplicate, or misrouted charts. Fix job/profile/runtime behavior.
2. **Inactive historical metadata:** old identities are no longer advancing.
   Leave them to normal lifecycle/retention unless there is a concrete operator
   harm.
3. **Metadata/database fault:** current state is inconsistent with the running
   collector even after identity and lifecycle behavior are understood. This is
   a separate database investigation, not profile iteration.

## Why a generic SQL recipe is unsafe

Netdata metadata spans more than one obvious context row. Correct handling can
depend on:

- the current metadata schema and foreign-key/reference relationships;
- host/vnode identity, not just context prefix;
- chart and dimension records plus stored history;
- SQLite WAL and shared-memory state;
- in-memory indexes owned by the running Agent;
- failure/rollback behavior if only part of a mutation succeeds.

Deleting rows by a context-like pattern while Netdata is running can leave the
on-disk database, WAL, and in-memory state disagreeing. Moving or deleting a
single database file can also affect unrelated applications and historical
data. Therefore this skill intentionally provides no copy-paste SQL reset.

## Exceptional reset boundary

Treat a reset as a destructive production/database change. Before proposing
one:

1. Prove the live collector no longer creates the unwanted identity.
2. Identify the exact current schema and all records/history affected from the
   checked-out source and the running Agent version.
3. Define the host/vnode and context scope precisely; a prefix alone is not
   enough evidence.
4. Define a consistent backup that includes the main database and any WAL/SHM
   state, plus a tested rollback path.
5. Plan service quiescence so no process writes while files or rows change.
6. State the historical data and rollback capability that will be lost.
7. Obtain explicit approval for the exact stop, backup, mutation, verification,
   and restart sequence.

If any assumption changes during execution, stop. Do not improvise a broader
query, remove another database file, or continue after a partial result.

## Preferred verification without deletion

During normal profile work:

- validate the exact dump/job/profile through the repository Go validator;
- verify only the intended production job/profile is active;
- compare active/advancing chart identities before and after cutover;
- distinguish active charts from inactive historical contexts;
- keep stable context/ID names when the semantics have not changed.

This preserves history while proving that future collection follows the new
design.
