# Fleet connectivity SLO queries (percent connected, boolean-dimension ratios, device ranking)

## Question

For a fleet of devices (IoT gateways, robots, edge nodes) streaming to
Netdata parents:

1. Single chart, 1 dimension: what percentage of the fleet is connected
   (streaming) right now / over time?
2. Single chart, 1 dimension: what percentage of devices have a boolean
   (0/1) collector dimension set to 1 (e.g. an "upstream reachable"
   check)?
3. The opposite: percentage of devices with that dimension at 0.
4. Rank/filter devices by the percentage of time the boolean dimension
   was 0 (find the devices that never connect).

## Inputs

- `TOKEN`, `SPACE`, `ROOM` (see SKILL.md prerequisites)
- The boolean context and dimension name, e.g. context
  `mycollector.upstream_connectivity`, dimension `Overall`
- A time window (`after` in relative seconds)

## Key insight: `average` of a 0/1 metric IS the ratio

All four questions reduce to one trick: the **average of a 0/1 series
is the fraction of 1s**. No `countif` needed (see gotcha 3):

- Averaged **across devices** at each point in time → fraction of the
  fleet at 1 at that moment (questions 1, 2, 3).
- Averaged **over time** per device → fraction of time that device was
  at 1 (question 4).

## Steps

1. **Percent of fleet connected (from the parents' per-child streaming
   state).** Netdata parents (v2.10.0-nightly 2026-06+ pulse) expose
   `netdata.streaming.in.state` — a per-child one-hot chart with 0/1
   dimensions `running`, `offline`, `archived`, `waiting`,
   `waiting replication`, `replicating`. The fleet ratio is the average
   of `running` across all tracked children:

   ```bash
   source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
   agents_load_env
   agents_query_cloud POST /api/v3/spaces/$SPACE/rooms/$ROOM/data '{
    "scope":{"contexts":["netdata.streaming.in.state"],"dimensions":["running"]},
    "selectors":{"nodes":["*"],"contexts":["*"],"instances":["*"],"dimensions":["*"],"labels":["*"],"alerts":["*"]},
    "window":{"after":-21600,"before":0,"points":12},
    "aggregations":{"metrics":[{"group_by":["selected"],"aggregation":"avg"}],
                    "time":{"time_group":"average"}},
    "format":"json2","options":["jsonwrap","minify","unaligned"],"timeout":60000}' \
   | jq -r '.result.data[] | [(.[0]|todate), ((.[1][0]*10000|round)/100)] | @tsv'
   ```

   One column, values 0–1 (multiply by 100 for %). The denominator is
   every child the parents still track — including `archived` ones
   (see gotcha 1).

2. **Percent of devices with a boolean dimension at 1.** Same pattern
   on the collector context — average the 0/1 dimension across all
   instances:

   ```bash
   agents_query_cloud POST /api/v3/spaces/$SPACE/rooms/$ROOM/data '{
    "scope":{"contexts":["CONTEXT"],"dimensions":["DIMENSION"]},
    "selectors":{"nodes":["*"],"contexts":["*"],"instances":["*"],"dimensions":["*"],"labels":["*"],"alerts":["*"]},
    "window":{"after":-21600,"before":0,"points":12},
    "aggregations":{"metrics":[{"group_by":["selected"],"aggregation":"avg"}],
                    "time":{"time_group":"average"}},
    "format":"json2","options":["jsonwrap","minify","unaligned"],"timeout":60000}' \
   | jq -r '.result.data[] | [(.[0]|todate), ((.[1][0]*10000|round)/100)] | @tsv'
   ```

3. **The opposite ratio** is `100 − (step 2)`, computed client-side:

   ```bash
   ... | jq -r '.result.data[] | [(.[0]|todate), (100 - (.[1][0]*10000|round)/100)] | @tsv'
   ```

   Prefer this client-side subtraction over
   `time_group: countif "=0"`, which needs the care described in
   gotcha 3.

4. **Rank devices by percent of time the dimension was 1 (or 0).**
   Group by node, average over the whole window into a single point,
   then sort client-side:

   ```bash
   agents_query_cloud POST /api/v3/spaces/$SPACE/rooms/$ROOM/data '{
    "scope":{"contexts":["CONTEXT"],"dimensions":["DIMENSION"]},
    "selectors":{"nodes":["*"],"contexts":["*"],"instances":["*"],"dimensions":["*"],"labels":["*"],"alerts":["*"]},
    "window":{"after":-86400,"before":0,"points":1},
    "aggregations":{"metrics":[{"group_by":["node"],"aggregation":"avg"}],
                    "time":{"time_group":"average"}},
    "format":"json2","options":["jsonwrap","minify","unaligned"],"timeout":120000}' > /tmp/rank.json

   # bottom 20 = devices with the LOWEST fraction of time at 1
   jq -r '.view.dimensions as $d
          | [range(0; ($d.ids|length)) | {n:$d.names[.], v:$d.sts.avg[.]}]
          | sort_by(.v) | .[:20][]
          | [((.v*10000|round)/100|tostring)+"%", .n] | @tsv' /tmp/rank.json
   ```

   `sort_by(.v)` ascending = most-disconnected first; `sort_by(-.v)`
   for the healthiest. The per-device value is in
   `view.dimensions.sts.avg[]` when `points:1`.

## Output

- Steps 1–3: a timestamped series of fleet-wide percentages (one
  dimension — chartable as-is).
- Step 4: a ranked table `percent<TAB>hostname` of all devices.

## Notes / gotchas

1. **Denominator semantics.** In step 1 the denominator is all
   children the parents track, including `archived` (long-gone or
   re-parented duplicates), until their charts obsolete. In steps 2–4
   the denominator is only devices whose collector produced data in
   the window — fully offline devices drop out of the average
   entirely (they have gaps, and gap points are excluded from group-by
   aggregation). Pair step 1 (connectivity) with step 2 (health of the
   connected) for the complete picture.
2. **Instances vs room nodes will not match.** Parents can track more
   children than the room shows as nodes (archived duplicates after
   re-parenting), so do not expect `totals.instances` to equal the
   room node count.
3. **`countif` needs care on fleet-wide queries (measured
   2026-07-06, re-diagnosed 2026-07-25).** On a multi-parent space,
   `time_group: countif "=0"` returned ~29–43% of the true value.
   Two causes were involved; only the second is still live:
   - **Row order — fixed.** Agents return rows newest-first, while
     the multi-agent merge assumed oldest-first, so a fleet query
     could come back with only the newest point populated and every
     other point null, with `pa` still reporting those points as
     fine. Netdata Cloud now normalises row order when it ingests
     each agent response; older deployments needed `"flip"` in
     `options` as a workaround.
   - **Storage resolution — still applies.** `countif` tests its
     condition against whatever is stored, and above tier 0 that is
     a per-minute **average** (see gotcha 4). A minute that was down
     40s of 60 stores 0.67 — not exactly 0 — so it never matches
     `=0`; only fully-down minutes are counted and the total comes
     out low. Request `"tier": 0` and confirm under `db.per_tier`
     that tier 0 actually served the data.

   `time_group: average` is immune to the second cause because the
   tier stores exactly that, which is why it looked correct on the
   same data. The average-of-boolean trick above is exact at every
   tier and remains the recommended pattern.
   (`options: ["percentage"]` separately returned >100%: that option
   expresses each item as a share of the group total, which is
   meaningless applied to values that are already percentages.)
4. **Tier-0 retention on busy parents is short.** A parent with
   thousands of children may hold only minutes of per-second data
   (observed: ~18 minutes with ~5.6k children). Queries over longer
   windows silently use tier 1+ (per-minute averages) — fine for
   the average-of-boolean trick, and the second cause of the
   `countif` undercount in gotcha 3.
5. **Boolean-ness matters.** These patterns assume the dimension is
   strictly 0/1. If a collector emits other values, the average is no
   longer a ratio.

## Source guides

- [query-metrics.md](../query-metrics.md) — request body reference
  (scope, selectors, window, aggregations, response shape)
- SKILL.md — discovery endpoints (`/spaces`, `/rooms`, `/nodes`)
