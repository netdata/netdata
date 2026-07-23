# Diagnose "No data on zoom" / per-chart data gaps for a node behind Netdata Parents

## Symptom

- Zooming into a time range on a node's charts shows **"No data"**, while the
  same range renders fine at a wider zoom level (coarser points-per-pixel).
- Different charts of the same node show **different multi-hour gaps** at the
  same zoom level.
- Data "comes back on its own" without any restart or operator action.

## Root-cause pattern this how-to detects

The node streams to one or more Netdata Parents and is connected to Cloud
**indirectly** (`connection_type: "indirect"` — the child is not claimed, so
every Cloud query is served by a parent's copy of the data). The parent has
**holes in tier0** (per-second data) while tier1/tier2 (per-minute/per-hour)
are complete. The dashboard's zoomed queries need tier0 → "No data"; wide
queries are satisfied by higher tiers → data appears.

Tier1/tier2 are generated on the parent from the same ingested samples as
tier0 — they are never replicated. So **tier1 full + tier0 empty on the same
agent proves the samples were ingested and tier0 lost them at/after storage**
(each tier writes to its own datafiles and file descriptors). The verified
real-world case behind this how-to: the parent's active tier0 datafile file
descriptor went bad (`EBADF`), every extent write failed for ~5 hours until
the datafile rotated, and all hosts stored on that parent lost tier0 for the
window — while streaming, ingestion, tier1 and tier2 continued normally.
Steps 6–7 below discriminate this from streaming/replication problems.

## Step 0 — prerequisites

Source the token-safe wrappers and resolve space/room/node IDs (see
[SKILL.md](../SKILL.md) discovery endpoints):

```bash
source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
agents_load_env
SPACE="YOUR_SPACE_ID"; ROOM="YOUR_ROOM_ID"; NODE="YOUR_NODE_UUID"
```

## Step 1 — check how the node is connected to Cloud

```bash
agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}' \
  | jq '.nodes[] | select(.nd == "'$NODE'") | {nm, v, state, replication, isPreferred}'
```

`replication.connection_type: "indirect"` means Cloud can only see what the
parents hold — the child's local (usually complete) retention is unreachable.

## Step 2 — reproduce the zoom query and identify the serving agent + tier

Query the suspect window at high resolution (5s/point or finer) with
`options: ["jsonwrap"]`. The response's `.agents[0].nm` is the agent that
served the query; `.db.per_tier[]` shows how many points each tier provided.

```bash
read -r -d '' PAYLOAD <<EOF
{
  "scope": { "nodes": ["$NODE"], "contexts": ["system.cpu"] },
  "selectors": { "nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"] },
  "window": { "after": WINDOW_START_EPOCH, "before": WINDOW_END_EPOCH, "points": 360 },
  "aggregations": { "metrics": [{ "group_by": ["dimension"], "aggregation": "avg" }], "time": { "time_group": "average" } },
  "format": "json2", "options": ["jsonwrap", "minify"], "timeout": 30000
}
EOF
agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/data" "$PAYLOAD" \
  | jq '{served_by: .agents[0].nm, per_tier: [.db.per_tier[] | {tier, points}],
         null_rows: ([.result.data[] | select(([.[1:][] | .[0]] | map(select(. != null)) | length) == 0)] | length),
         rows: (.result.data | length)}'
```

In json2 format each cell is a tuple `[value, anomaly_rate, annotation]` —
test `.[0]` for null, not the tuple itself.

## Step 3 — compare tiers over the same window

Re-run the same payload with `"tier": 0`, then `"tier": 1`, then `"tier": 2`
added to `window`. The signature of this failure mode:

- tier 0 → all rows null, `per_tier[0].points` near zero
- tier 1 and 2 → full data

## Step 4 — map the tier0 holes over a wide window

Force `"tier": 0`, widen the window (several hours), 500 points, then reduce
consecutive all-null rows to gap runs:

```bash
jq -r '.result.data | sort_by(.[0])
  | map({ts: .[0], null: (([.[1:][] | .[0]] | map(select(. != null)) | length) == 0)})
  | reduce .[] as $p ({runs: [], cur: null};
      if $p.null then (if .cur == null then .cur = {s: $p.ts, e: $p.ts} else .cur.e = $p.ts end)
      else (if .cur != null then .runs += [.cur] | .cur = null else . end) end)
  | (.runs + (if .cur then [.cur] else [] end))
  | map("GAP \(.s|strftime("%m-%d %H:%M")) -> \(.e|strftime("%m-%d %H:%M")) UTC (\(((.e-.s)/60)|floor) min)") | .[]'
```

Repeat for several contexts (e.g. `system.cpu` plus a few collector charts).
Per-chart different holes ⇒ per-chart replication starvation, not a collector
outage.

## Step 5 — quantify streaming flapping on the child

Context `netdata.streaming_outbound` (dimensions: `connecting`, `pending`,
`offline`, `waiting`, `replicating`, `running`, `no dst`, `failed`). Query
with `group_by: ["dimension"]`, `aggregation: "max"`, `time_group: "max"`,
~2-minute buckets. Count buckets where `replicating >= 0.5`:

- A healthy child shows `replicating` only right after a (re)connect.
- A flapping child shows `replicating` touching 1 in a large fraction of all
  buckets (in the incident that produced this how-to: 74% and 94% on the two
  affected children — a reconnect + re-replication every ~4–6 minutes, around
  the clock).

Check sibling nodes too: if several children flap, the cause is the network
path or the parent, not the node.

On the parent, context `netdata.streaming_inbound` (instances
`..._permanent` / `..._ephemeral`) gives the aggregate view: the average of
the `replicating` dimension over time is the fraction of time at least one
child was replicating.

## Step 6 — THE CONTROL TEST: check the parent's own local charts

This is the decisive discriminator. Run the Step 4 gap map against the
**parent's own node** (its `system.cpu`, `netdata.server_cpu` — data that
never crossed the network), and against the parent's other children:

- Parent's own charts have the **same tier0 hole** ⇒ the problem is local to
  the parent's dbengine tier0 storage. Streaming/replication is NOT the
  cause, no matter how bad the flapping looks.
- Parent's own charts are clean, only streamed children have holes ⇒ the
  problem is in delivery (streaming/replication starvation).

Also run the gap map on the *other* parents' own charts — if only one parent
is affected, the fault is that machine, not the fleet.

Corroborating signals on the affected parent for the local-storage case
(query them over the incident window; use auto/tier1 so you can see values
inside the tier0 hole):

- `netdata.db_samples_collected` steady across the hole ⇒ ingestion never
  stopped; the engine accepted the samples.
- `netdata.uptime` monotonic ⇒ no restart, so the loss was not a crash.
- `netdata.memory` / `system.ram` sane ⇒ not memory pressure.

## Step 7 — read the parent's daemon log for storage errors

The parent's own error log names the failing operation. On a Windows parent,
Netdata registers Event Log channels — query the `windows-events` Function
through Cloud (source ids from `info=true`; note `Netdata/Daemon` is a small
circular buffer, ~1 MiB, so hours-old triggers may already be overwritten —
check early):

```bash
read -r -d '' PAYLOAD <<EOF
{
  "after": WINDOW_START_EPOCH, "before": WINDOW_END_EPOCH, "last": 300,
  "query": "dbengine",
  "selections": { "__logs_sources": ["Netdata/Daemon"] }
}
EOF
agents_call_function --via cloud --node "PARENT_NODE_UUID" \
  --function windows-events --body "$PAYLOAD"
```

On a Linux parent, use `systemd-journal` with the same payload shape.

In the verified incident this returned
`DBENGINE: Tier 0, bad file descriptor` (Unix errno 9) repeating every 10
seconds (the dbengine extent-write error is rate-limited to 1 per 10s) for
~5 hours, ending seconds before `created datafile-1-NNNN` — and the
journalfile of the dying datafile indexed at a tiny fraction of its healthy
neighbors (6.63MiB/724 extents vs 187MiB/32893 extents). Every extent write
during the window was dropped; rotation to a fresh datafile (new fd)
self-healed the storage. The Windows `System` channel around the trigger
time can reveal external causes (updates, snapshots, time changes).

## Interpreting the result

| Evidence | Conclusion |
|---|---|
| Zoom query served by a parent, tier0 all null, tier1/2 full | Parent has tier0 holes; dashboard zoom hits tier0. Samples WERE ingested (tier1 proves it) — tier0 lost them locally |
| Parent's OWN charts have the same hole (Step 6) | Parent-local dbengine tier0 storage failure — not streaming |
| Only streamed children have holes, parent's own charts clean | Delivery problem: streaming flaps / replication starvation |
| Different gap boundaries per context | Page-granular loss (each metric's pages span different ranges) |
| `replicating` active in most buckets on `netdata.streaming_outbound` | Child↔parent link flaps or parent throttles replication (chronic issue worth its own follow-up, but not necessarily the data-loss cause) |
| `connection_type: "indirect"` | Cloud cannot fall back to the child's local retention |
| Daemon log: `DBENGINE: Tier N, <io error>` every 10s (Step 7) | Extent writes failing; data dropped at flush; identify what invalidated the fd/filesystem |

Fix directions depend on which case Step 6 proved: for parent-local storage
failures, investigate the parent machine (filesystem, AV/backup agents
touching the dbengine files, disk health) and report the daemon-log evidence
to Netdata engineering; for delivery problems, fix the network path /
parent capacity, consider claiming the children directly, and upgrade agents
if the version predates streaming stability fixes.

Note: replication in the streaming protocol transfers **tier0 only**
(`stream-replication-sender.c` queries `rd->tiers[0].smh`); tier1/tier2 are
generated on each agent from what it ingests. This is why "tier1 has it,
tier0 doesn't" can never be produced by replication — it always means local
tier0 storage loss on the serving agent.

## Source guides

- [query-metrics.md](../query-metrics.md) — request body reference
  (scope, selectors, window, aggregations, response shape)
- [query-logs.md](../query-logs.md) — systemd-journal / windows-events
  Function payload reference
- SKILL.md — discovery endpoints (`/spaces`, `/rooms`, `/nodes`)
