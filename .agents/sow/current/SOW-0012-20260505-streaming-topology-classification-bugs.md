# SOW-0012 - Streaming topology: parent/child classification, missing-parent rendering, and "Since" column type

## Status

Status: in-progress

Sub-state: triage complete; live cloud evidence captured; root causes located; reviewed by six external assistants in three iterations; user decisions recorded 2026-05-06; implementation in progress; backend topology filters removed from scope by project-owner decision.

## Requirements

### Purpose

Make the `topology:streaming` Function (`function-streaming.c::function_streaming_topology`) produce correct, useful output on **both sides of a streaming relationship**: when invoked on a parent agent and when invoked on a child agent. The view from a child must (a) classify the child correctly, (b) render its upstream parent(s) as actors, and (c) render time fields in human form.

This is in service of the broader topology PR (https://github.com/netdata/netdata/pull/22110, merged April 14 as commit `7da4565`) being usable by SREs without manual decoding of Unix epoch values or manual reasoning about misclassified node types.

### User Request

The reporter raised four concerns, summarised:

1. "Child agent is identified as parent — bug related to vnodes?"
2. "In the streaming topology, the 'Since' field shows number 1,777,992,161 — assume this is a timestamp not rendered properly."
3. "I also hoped to have it show its parents."
4. "The streaming topology function on the child seems broken. No facets, no filters, nothing."

The reporter shared a screenshot of `topology:streaming on [child-1] ([CHILD_IP])` with `0 results` in the result counter, the child's modal popup showing `Type: parent`, `Children: 1`, and a streaming-path table where the `Since` column rendered raw integers (`1,777,992,161` and `1,777,992,171`).

### Assistant Understanding

Facts (verified via live Cloud queries on the reporter's space, 2026-05-05; raw responses preserved at `<repo>/.local/audits/streaming-topology/` which is gitignored):

- Topology: `[parent-1]` is the parent; `[child-1]` is the only real streaming child. Six other actors (legacy stale streams) appear as `vnode` (marked virtual).
- On the **parent** (`[parent-1]`) view: `actors_n=8`, `links_n=7`. **`[parent-1]` itself is classified `node_type:"child"`, `child_count:0`** — wrong; it has 1 active streaming child. `[child-1]` is correctly `child`. Link `[child-1] → [parent-1]` correctly emitted with `link_type:"streaming"`.
- On the **child** (`[child-1]`) view: `actors_n=1`, `links_n=0`. The single actor is `[child-1]` itself, classified `node_type:"parent"` with `child_count:1`. `[parent-1]` (the actual parent) is **not present as an actor**. The actor's `streaming_path` field is `[[child-1], [parent-1]]` — so the path data is known to the function, but is not turned into a second actor.
- The streaming-path table on each actor (emitted via `rrdhost_stream_path_to_json` at function-streaming.c:1327) carries `since` as raw Unix epoch seconds (`1777992161` for the child, `1777992171` for the parent).
- Function name is `topology:streaming` (registered at functions.c:23). Cloud Function URL: `POST /api/v2/nodes/{nodeId}/function?function=topology:streaming`.

Inferences:

- The "no facets, no filters" symptom on the child is a consequence of the response being almost empty (1 actor, 0 links). Once the child's view contains a correctly classified child plus a synthetic parent actor plus the upstream link, the FE will have data to populate facets/filters from.
- The misclassification on the parent and the misclassification on the child are produced by **different bugs that happen to land on the same `parent_child_count` dictionary** — see Analysis.

Resolved (after iteration-2 review with cloud-frontend access):

- The cloud-frontend's topology table renderer accepts `"timestamp"` columns in **either seconds or milliseconds**: at `cloud-frontend/src/domains/functions/components/topology/actorModal/dataTable.js:52-58` it auto-detects via `ms > 1e12 ? ms : ms * 1000`. The agent emits `since` in seconds; the FE multiplies by 1000 → correct date rendering. Decision 4 changes accordingly (Option 2 chosen — see below).
- Re-emitting `stream_path_send_to_parent(host)` after a parent-originated path update **does** create an oscillation risk. `rrdhost_stream_path_to_json` overlays a fresh `rrdhost_stream_path_self()` at every emit (stream-path.c:181-202), and that fresh self pulls live values (`first_time_t` from `rrdhost_retention()`, `start_time_ms`/`shutdown_time_ms` from `get_agent_event_time_median()`) that change continuously. The XXH128 hash is computed over stored content but the emit produces fresh content each time, so the receiving side's hash differs every round-trip → perpetual re-sends. This rules out a naive protocol fix and reinforces the SOW's original Option 3 Hybrid recommendation for Decision 1.

### Acceptance Criteria

- On the **parent view**: parent's actor has `node_type:"parent"` and `child_count >= 1`. Verification: `agents_call_function --via cloud --node $PARENT --function topology:streaming` returns `actors[0].attributes` with `node_type=="parent"` and `child_count > 0` for the localhost actor.
- On the **child view**: child's actor has `node_type:"child"` and `child_count==0`. Verification: same Cloud call against the child's node id; `actors[0].attributes.node_type=="child"`.
- On the **child view**: at least one synthetic actor for the parent is present, and a `link` from child to parent with `link_type=="streaming"`. Verification: `data.actors[]` contains an entry for the parent's machine GUID with `actor_type=="parent"` (the existing actor type — no new type introduced; see Decision 3); `data.links[]` contains an entry with `src_actor_id` = child and `dst_actor_id` = parent. For deeper chains, `data.links[]` contains links between every consecutive non-localhost path slot.
- The `Since` column in the streaming-path table on each actor renders as a human date in the FE, not a raw integer. Verification: visual check in Netdata Cloud UI on both parent and child views.
- The backend always exposes the complete topology graph. `topology:streaming` must not accept or apply backend graph-pruning filters such as `node_type`, `ingest_status`, or `stream_status`. Facets or filtering in the UI are client-side concerns over the complete response. Verification: `accepted_params` contains only non-pruning params, currently `info`, and actors/links are emitted without backend filter checks.
- The two functions previously co-located in `src/web/api/functions/function-streaming.{c,h}` are split into one file each: `function-netdata-streaming.{c,h}` (registered as `netdata-streaming`) and `function-topology-streaming.{c,h}` (registered as `topology:streaming`). The simple-table function is renamed `function_streaming` → `function_netdata_streaming` for symbol/file consistency. Verification: the old files are removed; both new files compile and the registered Function names continue to work end-to-end via Cloud calls.
- A maintenance reference document `src/streaming/STREAM_PATH.md` is added describing the streaming-path subsystem (storage rule, propagation rules, cycle terminator, update triggers, vnode special case, intended classification logic, mergeability requirements). Verification: the file exists, is **not referenced in `docs/.map/map.yaml`** (so not picked up by the learn-ingestion pipeline; co-location under `src/streaming/` is not by itself sufficient — `src/streaming/README.md` IS in the map and gets ingested), and covers the nine outline points in the Plan section below.
- Tests covering the four root causes — see Validation Plan.

## Analysis

Sources checked:

- `src/web/api/functions/function-streaming.c` (full file, 2459 lines)
- `src/web/api/functions/function-streaming.h`
- `src/web/api/functions/functions.c` (Function registration)
- `src/streaming/stream-path.c` (full file)
- `src/streaming/stream-path.h`
- `src/streaming/stream-sender.c:147-161` (`stream_sender_on_ready_to_dispatch`, only caller of `stream_path_send_to_parent` from connection setup)
- `src/streaming/stream-sender-execute.c:109-110` (sender-side receive of upstream STREAM_PATH)
- `src/plugins.d/pluginsd_parser.c:1211` (parser-side receive of STREAM_PATH from a child)
- `<dashboard-repo>/src/domains/functions/components/topology/actorModal/dataTable.js` @ commit `8d0258eb60aa32e3ee5fdd2144ef10b44f7995bc` (FE timestamp handling — auto-detects seconds vs ms)
- `<dashboard-repo>/src/domains/functions/topology/payload.js` @ commit `8d0258eb60aa32e3ee5fdd2144ef10b44f7995bc` (FE actor-id dedup, link-derived synthesis)
- Live data: sanitized summaries derived from raw Function responses stored under `<repo>/.local/audits/streaming-topology/` (gitignored)
- Reporter screenshot of the broken UI (saved locally; not committed)

### Intended classification logic (the rule the function should implement)

Confirmed with project owner 2026-05-06. The topology function is meant to classify each entry in `rrdhost_root_index` according to the following rule:

1. **Source of truth = `rrdhost_root_index`.** This is the local agent's authoritative list of nodes it knows about: localhost, every real child currently or previously connected (including stale/archived ones), and every vnode registered locally.
2. **For each non-vnode node, read its `stream.path.array`.** Slot 0 is the origin of that chain (the originating "child"). Slots 1+ are the upstream hops.
3. **Tag entries by slot:**
   - Slot 0 → origin (a child role in this chain).
   - Slots 1+ → parent role in this chain.
4. **Globally,** a node X is classified as `parent` if X appears at slot 1+ in **any** node's path. Otherwise X is `child`.
5. **vnodes are special.** Their stored path is empty (vnodes do not stream themselves — their data is collected by the local agent). At JSON-emit time, slot 0 of a vnode's path is filled by the collecting agent (via emit-time self-append). vnodes are tagged `vnode` directly and skip the slot-1+ counter step.
6. **Stale nodes are special.** A node whose ingest status is `RRDHOST_INGEST_STATUS_ARCHIVED` is tagged `stale` directly and rendered with a stale link back to the collecting agent (localhost).

Both Bug A and Bug B sit on the same step (#3 — slot-1+ extraction feeding the global classification at #4) but fail for different reasons. Bug A: the data being read at slot 1+ is incomplete (stored paths haven't converged yet on the apex parent). Bug B: the slot-1+ extraction itself is contaminated (the helper appends localhost into a result that is supposed to contain only upstream entries). Together they cause the classification to be wrong on **both** sides of the same parent-child relationship — the parent shows as a child (Bug A), the child shows as a parent (Bug B).

### Intended actor set per agent

Confirmed with project owner 2026-05-06. From any single agent's perspective, the topology view emits the following actors. All edges/chains route through `self`; nothing branches around it.

| Source | Actor type | Where the data comes from | Link to self |
|---|---|---|---|
| `self` (localhost) | `parent` (if it has any active streaming children) or `child` (otherwise) | `rrdhost_root_index` (localhost) | — (origin of self's chain) |
| Active vnodes | `vnode` | `rrdhost_root_index` (entries with `RRDHOST_OPTION_VIRTUAL_HOST`) | `vnode → self`, link_type derives from how the agent collects the vnode |
| Active children (real, currently streaming in) | `child` | `rrdhost_root_index` (entries with active receiver, not virtual, not archived) | `child → self`, link_type `streaming` |
| Stale children (archived, last seen on this agent) | `stale` | `rrdhost_root_index` (entries with `RRDHOST_INGEST_STATUS_ARCHIVED`) | `stale → self`, link_type `stale` |
| Upstream parents of self | `parent` (existing actor type, no new type introduced) | self's `stream.path.array` slots 1+ (synthesized actors — they are not in `rrdhost_root_index`) | `self → parent_1`, `parent_1 → parent_2`, ... — link_type `streaming`, one link per consecutive non-localhost path slot |

Children's full chains, after path convergence, look like `[child, self, parent_1, parent_2, ...]`. The tail `[self, parent_1, ...]` is identical to self's own chain, so after dedup by `actor_id`, the children's chains add no new upstream actors — they share self's chain. Same for vnodes and stale children.

Counting example: an agent with 100 vnodes + 100 active children + 20 stale children + 2 upstream parents emits **223 actors total** (1 self + 100 + 100 + 20 + 2) and **222 unique links** (100 + 100 + 20 + 1 + 1).

Sizing note: this matches the original PR description's caveat that very large topologies produce large responses (carried over from the unfixed scope, not introduced by this SOW). The synthesized upstream `parent` actors are bounded by the chain depth (typically 1–3), so this SOW does not add a sizing risk.

### Topology must be mergeable across agents

Confirmed with project owner 2026-05-06. The Cloud frontend (or any consumer) must be able to merge the per-agent topology responses from N parents into a single unified view. The agent's response shape and per-actor / per-link attributes must support this.

What each parent contributes to a merged view (where it does not overlap with what other parents already contributed):

- **Retention** for each child the parent received data from — already emitted per-actor in the `retention` and (per-host) inbound/outbound tables.
- **Its own vnodes** — vnodes are local to the collecting agent, so two parents typically do not overlap on vnodes (unless misconfigured).
- **Its own stale children** — these can overlap (a child that streamed to A, then moved to B, will appear `stale` on A and `child` on B). The merge must pick the authoritative current state.
- **Its own active children and upstream parents**, where they don't overlap.

Mergeability requirements satisfied by the current response schema:

1. **Stable, deterministic actor IDs.** Every actor's `actor_id` is `netdata-machine-guid:<guid>` produced by `streaming_topology_actor_id_from_guid` (`function-streaming.c:107-115`). The same guid produces the same id across agents. Merge dedups by id.
2. **Stable link tuples.** Each link is keyed by (`src_actor_id`, `dst_actor_id`, `link_type`). Same src + dst + type across two parents = the same link.
3. **Timestamps for tie-breaking.** Each link carries `discovered_at` (when this agent first saw the connection) and `last_seen` (the most recent observation). When the same actor appears as `child` on one parent and `stale` on another, the merge can compare timestamps and decide the authoritative current state (the `last_seen` of the streaming link should be more recent than the `last_seen` of the stale link). The Bug C synthesis must preserve these fields on synthesized actors and links — derive `since`/timestamps from the `STREAM_PATH` struct fields (`since`, `first_time_t`).
4. **No agent claims authority over hosts it doesn't see directly.** Each parent reports only the children/vnodes that connect to it, plus its own upstream chain. A child of parent-A is not represented in parent-B's response unless that child has also (currently or historically) connected to B.

Implication for Bug C synthesis: a synthesized `parent` actor on a child's view must use the same canonical id format (`netdata-machine-guid:<guid>`) so dedup by `actor_id` works across responses. Synthesized links must carry `discovered_at`/`last_seen` derived from the `STREAM_PATH` struct's `since` and `first_time_t` fields (NOT from `now`), so two parents reporting the same upstream produce comparable timestamps and the merge layer can resolve overlaps deterministically.

The aggregator does not need a new "self" marker on the response. The existing top-level `data.agent_id` (function-streaming.c:840-841) carries the emitting agent's GUID; the aggregator can compute the corresponding self-actor_id as `netdata-machine-guid:<agent_id>` to find the localhost actor in `actors[]`. Authority resolution per actor: when actor X appears in multiple responses, the response whose `agent_id` matches X's GUID is authoritative for X's attributes (full retention, full inbound/outbound, etc.); other responses contribute confirmations and link records but do not override.

Implication for the migration scenario (a child moved from A to B): both A's and B's responses are valid local truth. The merge layer holds both link records (`child→A stale` and `child→B streaming`), with `last_seen` distinguishing them; the actor's "current state" in the merged view derives from the most recent link.

Current state:

The function has four observable defects. Each is documented with file:line evidence and a concrete reproduction trace. Bugs A and B are **distinct root causes** that both feed the same `parent_child_count` dictionary; Bugs C and D are independent of A and B. Fixing any one bug does not fix the others.

### Bug A — On a parent's view, the parent classifies itself as `child`

Symptom: `actors[0]` in the parent's response: `actor_type:"child"`, `attributes.child_count:0`. The localhost is classified as a non-parent on its own view.

Root cause (mapped to the intended logic above): step #3 (slot-1+ extraction) reads stored paths that haven't converged yet. The data feeding the classification is incomplete — the parent does not yet appear at slot 1+ in the child's stored path on the apex parent's side, because convergence is sparse-trigger-driven and hasn't fired since the child connected.

Design context (verified by code review and confirmed with project owner):

The streaming-path subsystem is designed for full multi-hop convergence. In a chain `child → proxy → grandparent → ...`, every agent in the chain eventually stores the full path for any host whose data flows through, by this mechanism:

- **From-below (`from_parent=false`):** store, then propagate UP via `stream_path_send_to_parent(host)` (works on a proxy because each forwarded host has its own sender — `stream-receiver-connection.c:188` creates the child host record with a sender when the local agent has streaming forwarding configured) and DOWN via `stream_path_send_to_child(host)`. Bidirectional.
- **From-above (`from_parent=true`):** store, propagate DOWN only. The `if(!from_parent)` guard at stream-path.c:423 is the cycle terminator.
- **Self is appended at JSON-emit time** (`rrdhost_stream_path_to_json` at stream-path.c:198-202), not at storage time. This avoids storing stale local fields (timing medians, retention edges) that would diverge from "live" self.
- **Update triggers** are sparse: connect, retention boundary movement (`first_time_s` change at `rrdcontext-worker.c:99-104`), node_id assignment, parent disconnect.

Trace (verified) — why the bug appears on the **apex parent** in this specific topology:

1. The child starts streaming to the parent. The child's stored `stream.path.array` is initially empty.
2. On `stream_sender_on_ready_to_dispatch` (stream-sender.c:157), the child calls `stream_path_send_to_parent(localhost)`. The payload, built by `rrdhost_stream_path_to_json` (stream-path.c:177-209), contains only the appended `tmp = rrdhost_stream_path_self(localhost)` entry — `[{child, hops=0}]`.
3. The parent receives at `pluginsd_parser.c:1211`: `stream_path_set_from_json(child, json, from_parent=false)`. The parent stores `child->stream.path.array = [{child, hops=0}]`.
4. After the hash changes (stream-path.c:422-429), the parent calls `stream_path_send_to_parent(host=child_host_on_parent)` and `stream_path_send_to_child(host=child_host_on_parent)`:
   - `stream_path_send_to_parent` is a **no-op on the apex parent** in this topology because the apex has no upstream — `child_host_on_parent->sender == NULL` (the local agent does not forward to a grandparent). On a proxy this would propagate the path up. **The apex case is what breaks convergence in this specific scenario.**
   - `stream_path_send_to_child` does the JSON-emit-time self-append and sends the child the enriched view including the parent. The child stores `[{child, hops=0}, {parent, hops=1}]`.
5. After this initial round-trip, the parent's stored copy of the child's path is `[{child}]` — the parent's own host_id is not yet stored. Convergence on the parent depends on the child re-emitting later.
6. The child re-emits via `stream_path_send_to_parent(localhost)` only on sparse triggers — retention boundary movement (`first_time_s` change), node_id update, or reconnect. **In a freshly-connected child (e.g., after the reporter removed cloud.d on the child), none of these have fired yet.** The parent's storage stays at the initial `[{child}]`.
7. Phase 1 of `function_streaming_topology` (function-streaming.c:780-799) iterates `rrdhost_root_index` and, for each host, calls `streaming_topology_get_path_ids(host, from=1, ...)`:
   - For `host=parent` (localhost on apex): `parent->stream.path.array` is empty → returns 0.
   - For `host=child`: stored `[{child, hops=0}]` only → `rrdhost_stream_path_get_host_ids(child, 1, ...)` returns 0 (nothing at position 1+) → the localhost-append at function-streaming.c:243-244 is gated by `n > 0` so it does not fire → returns 0.
   - For each vnode: empty stored path → returns 0.
8. `parent_child_count[parent's UUID]` is therefore **never** incremented.
9. Actor classification (function-streaming.c:858-866): `cc = NULL` → `node_type = "child"` → wrong.

So the bug is **not** "the parent never knows about itself" — the design eventually converges. The bug is that the topology function reads stored paths for classification, and those stored paths can lag behind the converged state for an arbitrary period (until the next retention/node_id event on the originating child). In a "fresh" topology (recent reconnect, recent cloud.d removal, etc.) the lag is observable. The function returning a wrong answer because the storage hasn't caught up yet is incorrect behavior — the function should report current truth, not last-converged-state.

### Bug B — On a child's view, the child classifies itself as `parent`

Symptom: `actors[0]` in the child's response: `actor_type:"parent"`, `attributes.child_count:1`. The localhost is classified as a parent of itself.

Root cause (mapped to the intended logic above): step #3 (slot-1+ extraction) is **contaminated** at the helper. `streaming_topology_get_path_ids` blindly appends `localhost->host_id` to the result for **any** value of `from`, including `from=1` which is used by Phase 1 parent-counting. The append is correct semantics for `from=0` (full-path rendering — slot 0 plus slots 1+) but wrong for `from=1` (only slots 1+, by definition the host itself must not be there). The helper sneaks the localhost into the slot-1+ result, polluting the count.

File: `src/web/api/functions/function-streaming.c:225-247`

```c
static uint16_t streaming_topology_get_path_ids(RRDHOST *host, uint16_t from, ND_UUID *host_ids, uint16_t max) {
    uint16_t n = rrdhost_stream_path_get_host_ids(host, from, host_ids, max);
    uint16_t filtered_n = 0;

    bool found_localhost = false;
    for(uint16_t i = 0; i < n; i++) {
        if(UUIDiszero(host_ids[i]))
            continue;
        host_ids[filtered_n++] = host_ids[i];
        if(UUIDeq(host_ids[i], localhost->host_id)) {
            found_localhost = true;
        }
    }
    n = filtered_n;

    // append localhost if not found (same as rrdhost_stream_path_to_json)
    if(!found_localhost && n < max && n > 0)
        host_ids[n++] = localhost->host_id;

    return n;
}
```

Trace (verified):

1. The child's stored path on itself is `[{child, hops=0}, {parent, hops=1}]` (set after the child receives the parent's stream-path back; verified via `actors[0].tables.streaming_path` in the live response).
2. Phase 1: `streaming_topology_get_path_ids(child, from=1, ...)`:
   - `rrdhost_stream_path_get_host_ids(child, 1, ...)` returns `[parent's host_id]` (n=1).
   - found_localhost: parent != child (localhost) → false.
   - Append condition `!found_localhost && n > 0` → **append child** → returns `[parent, child]` (n=2).
3. Loop at function-streaming.c:786-798 increments both `parent_child_count[parent] = 1` (correct) and `parent_child_count[child] = 1` (incorrect).
4. Actor classification for the child (function-streaming.c:864-865): `cc = 1` → `node_type = "parent"` → wrong.
5. `attributes.child_count` (function-streaming.c:923) reads the same value, so the modal shows `Children: 1` as in the screenshot.

This bug is independent of Bug A. Even if Bug A is fixed at the protocol layer, Bug B will still misclassify any child agent on its own view.

Every caller of `streaming_topology_get_path_ids` (verified by grep — there are **6**, not 5):

- `function-streaming.c:785` — Phase 1 parent counting. `from=1`. **Wants the append off.**
- `function-streaming.c:801` — Phase 1 descendants computation. `from=0`. Wants the append on.
- `function-streaming.c:971` — Per-actor `streaming_path` field for FE highlight_path. `from=0`. Wants the append on.
- `function-streaming.c:1045` — Observer-mode inbound table source resolution. `from=0`. Wants the append on. *(Missed in the iteration-1 SOW; identified by iter-1 reviewers; verified.)*
- `function-streaming.c:1269` — Outbound table (non-observer parent). `from=0`. Wants the append on.
- `function-streaming.c:1407` — Phase 4 link emission (looks at `link_ids[1]` — the direct parent). `from=0`, `max=2`. Wants the append on (this is how the child→parent link is drawn even on the parent despite Bug A).

So one caller wants the append off, five want it on. The narrowest fix is to gate the append on `from == 0` inside `streaming_topology_get_path_ids` itself. No call site changes needed.

### Bug C — On a child's view, the parent agent is not rendered as an actor

Symptom: child's response has `actors_n: 1`, `links_n: 0`. The parent is referenced by `actors[0].streaming_path[1]` and known to the function via the child's `stream.path.array[1]`, but no actor is emitted for the parent.

Root cause: actors are emitted exclusively from `rrdhost_root_index` (function-streaming.c:847-848 in Phase 3, again at 1372-1375 in Phase 4 for links). On a child agent, that index contains only localhost. Path entries that point to nodes outside the local index are never synthesized.

This is what the reporter asked for in their second message — *"I also hoped to have it show its parents"*. The information is local on the child (in `STREAM_PATH` array entries which carry `hostname`, `host_id`, `node_id`, `claim_id`, `hops`, `since`, `first_time_t`, `start_time_ms`, `shutdown_time_ms`, `capabilities`, `flags`); it is just not turned into actor records.

The cloud-frontend can synthesize endpoint nodes from links if a link references a missing actor (`cloud-frontend/src/domains/functions/topology/payload.js:395-404`), but it does not synthesize actors from the per-actor `streamingPath` array. Since the child view has zero links, the FE fallback does not rescue this case.

### Bug D — `Since` column declared as `"number"`, FE renders raw integer

Symptom: streaming-path table shows `1,777,992,161` instead of a date.

Root cause: `function-streaming.c:355` declares the column as `"number"`. The path's `since` field is emitted in seconds (stream-path.c:88, `buffer_json_member_add_uint64(wb, "since", p->since)`).

After iteration-2 review with the cloud-frontend source available: the FE accepts `"timestamp"` columns in **either seconds or milliseconds** (`cloud-frontend/src/domains/functions/components/topology/actorModal/dataTable.js:52-58`):

```javascript
case "timestamp": {
    const ms = Number(val)
    if (isNaN(ms) || ms === 0) return "-"
    const ts = ms > 1e12 ? ms : ms * 1000
    // ... renders as date
}
```

So changing the column type to `"timestamp"` is sufficient — no data emission change needed. (Compare to `db_from`/`db_to` at function-streaming.c:374-375 which use `"timestamp"` and emit ms via `* MSEC_PER_SEC` — a stylistic convention but not required by the FE.)

### Bug E — "no facets, no filters" on the child view (consequence)

Symptom: screenshot shows the right pane with the Function picker tree but no facet pills.

Root cause: with one actor and zero links, the FE has no field distributions to populate facets from. Fixing A+B+C should populate the response with the child (correctly classified), the parent (synthetic actor for the upstream parent), and the link between them, which gives the FE enough material for facets.

Risks:

- The fix is contained inside `function-streaming.c`. The streaming-path subsystem stays as-is — no protocol change, no wire-format change, no change to `stream-path.c`. The cycle terminator at stream-path.c:423 (`if(!from_parent) stream_path_send_to_parent(host);`) is load-bearing and must not be touched: it's what stops a child→parent→grandparent chain from re-sending forever after each downstream echo.
- (Bug B fix) flipping the localhost-append off for `from > 0` is a behavior change in a small public-ish helper. The six call sites must be re-validated; in particular the link-drawing path at function-streaming.c:1407 must continue to work for stale/disconnected children. (Verified: with `from=0`, the gate stays on.)
- (Bug C fix) synthesizing actors from path entries reuses the existing `actor_type:"parent"`. The synthesized actors use the same canonical id format (`streaming_topology_actor_id_from_guid` at function-streaming.c:107-115) so the FE highlight_path lookup matches across the per-actor `streaming_path` field references and the new actor records. Sub-specs the implementation must pin down: (a) de-duplication against `rrdhost_root_index` so a path entry whose host_id matches a real local host does not produce a duplicate actor; (b) which sub-tables are populated — the synthetic actor's `tables` block is built directly from the `STREAM_PATH` struct fields so the streaming-path table renders; the inbound/outbound/retention tabs that depend on data the agent doesn't have for a remote parent will be empty (acceptable for a single-agent view; aggregated views in Cloud will fill them from the parent's own response); (c) link-emission pass for the synthesized chain — emit a `link_type:"streaming"` for **every consecutive non-localhost path slot** (`slot[i] → slot[i+1]` for `i >= 1`), not only `slot[0] → slot[1]`. The existing Phase 4 loop at function-streaming.c:1372-1467 only emits one link per `rrdhost_root_index` host with `max=2` at line 1407; the new synthesis pass emits the multi-hop links from the path; (d) link timestamps — synthesized `discovered_at` and `last_seen` derived from the `STREAM_PATH.since` and `STREAM_PATH.first_time_t` fields of each path entry, so cross-agent merge of the same upstream link produces comparable values.
- (Bug D fix) changing only the column type (Option 2) is a pure metadata change; it does not touch the data emission or the inter-agent wire format. Risk: very low.
- Same-failure search: `rrdhost_stream_path_total_reboot_time_ms` (stream-path.c:145-158) shares Bug A's blind assumption (that localhost is in localhost's own stored path). On a top parent it returns 0 silently. Add to the scan list during validation. The `topology:snmp` Function (separate) may have similar column-type issues; check `since`/timestamp columns there as part of validation, even if no fix is needed.

## Pre-Implementation Gate

Status: ready (user decisions recorded 2026-05-06: Decision 1 = Option 1, Decision 2 = Option 1, Decision 3 = Option 1 with no new actor type, Decision 4 = Option 2, hardening deferred to separate SOW)

Problem / root-cause model:

- See Bugs A–E in Analysis. Two distinct root causes (A: stale storage on parent; B: spurious localhost-append on `from > 0`) plus two missing-feature-style root causes (C: actors only iterate `rrdhost_root_index`; D: column type/unit). E is a downstream consequence of A+B+C.

Evidence reviewed:

- Code: `src/web/api/functions/function-streaming.c` (Phase 1: 759-829; Phase 3 actors: 845-1361; Phase 4 links: 1363-1505; helpers: 56-247, 313-360), `src/web/api/functions/functions.c:20-30`, `src/streaming/stream-path.c` (state machine: 79-96 emit, 98-143 self, 177-209 to_json, 219-247 send paths, 249-259 get_host_ids, 261-288 disconnect handlers, 368-435 set_from_json), `src/streaming/stream-sender.c:147-161`, `src/streaming/stream-sender-execute.c:109-110`, `src/plugins.d/pluginsd_parser.c:1211`.
- Cloud-frontend (commit `8d0258eb60aa32e3ee5fdd2144ef10b44f7995bc`): `<dashboard-repo>/src/domains/functions/components/topology/actorModal/dataTable.js:52-58` (timestamp auto-detection), `<dashboard-repo>/src/domains/functions/topology/payload.js:263-265, 350-383, 395-404` (actor-id dedup; link-derived synthesis fallback).
- Live evidence: Cloud Function responses for the reporter's parent and child, captured 2026-05-05, stored under `<repo>/.local/audits/streaming-topology/` (gitignored). Sanitized facts above.
- Reporter screenshot of the failing UI (viewed locally; not committed).
- Independent read-only review batches completed in two iterations. Iteration outputs preserved under `<repo>/.local/audits/streaming-topology/` (gitignored).

Affected contracts and surfaces:

- The `topology:streaming` Function output (consumed by Netdata Cloud frontend topology renderer).
- The presentation metadata under `presentation.actor_types.{parent,child,vnode,stale}` (function-streaming.c:561-615) — Decision 3 reuses the existing `parent` type for synthesized upstream actors; **no new actor type is introduced**.
- Internal helper `streaming_topology_get_path_ids` semantics — Decision 2.
- Streaming-path table column metadata at function-streaming.c:345-360 — Decision 4.
- File layout: `src/web/api/functions/function-streaming.{c,h}` is split into `function-netdata-streaming.{c,h}` and `function-topology-streaming.{c,h}`; `function_streaming` is renamed to `function_netdata_streaming`. Plus a new maintenance reference at `src/streaming/STREAM_PATH.md`. See Plan items 0 and 6.
- Decision 1 (localhost live-state classification) does NOT affect inter-agent streaming. The streaming protocol and `stream-path.c` are NOT touched by this SOW. (Defensive hardening for the streaming-path JSON parser — array length clamp + scalar range checks — is moved to a separate hardening SOW; out of scope here.)

Existing patterns to reuse:

- `streaming_topology_actor_id_from_guid` (function-streaming.c:107-115) for synthetic actor IDs.
- `STREAM_PATH` struct fields (stream-path.h, stream-path.c:79-96) for synthetic-actor attributes.
- The existing `parent` actor preset (function-streaming.c:563-575) is reused as-is for synthesized upstream actors. No new presentation block is needed.

Risk and blast radius:

- Decision 1 (localhost live-state classification) is contained in `function-streaming.c`. Implementation must populate **both** `parent_child_count[localhost]` AND `parent_descendants[localhost]` from the same root-index walk; otherwise the `received_nodes` block (function-streaming.c:982-1002) stays empty even when `child_count > 0`. **Zero changes to the streaming protocol or to `stream-path.c`.**
- Decisions 2 and 3 are local to `function-streaming.c`.
- Decision 4 Option 2 is pure metadata; trivial blast radius.

Sensitive data handling plan:

- The Cloud responses captured for evidence contain customer-identifying info (real hostnames, private IPs, agent UUIDs, claim IDs, machine GUIDs, host labels). Raw responses are stored under `<repo>/.local/audits/streaming-topology/` (gitignored).
- This SOW has been sanitized: hostnames are `[parent-1]`, `[child-1]`, vnodes unnamed; private IPs `[CHILD_IP]`; UUIDs not written; reporter referred to as "the reporter"; the customer space referred to as the reporter's space; Slack identifiers not included.
- Test fixtures derived from the Cloud responses must use sanitized hostnames/UUIDs (e.g., `parent-1`, `child-1`, `00000000-0000-0000-0000-000000000001`).
- No tokens, bearers, or claim IDs appear in this SOW or in any planned test fixture.

Implementation plan (deferred; depends on user decisions below):

0. **File split (precondition).** Confirmed with project owner 2026-05-06 — both streaming functions move to their own files, with `function-streaming.{c,h}` deleted. Steps:
   - Create `src/web/api/functions/function-topology-streaming.{c,h}`. Move `function_streaming_topology` and every `streaming_topology_*` static helper, plus the `RRDFUNCTIONS_STREAMING_TOPOLOGY_HELP` macro, into the new file. New header exports `function_streaming_topology` and the help macro. The former `value_in_csv` helper is removed by Decision 5 because the topology function no longer applies backend graph filters.
   - Create `src/web/api/functions/function-netdata-streaming.{c,h}`. Move `function_streaming` (currently at function-streaming.c:1499) and the `RRDFUNCTIONS_STREAMING_HELP` macro into the new file. **Rename** the C symbol `function_streaming` → `function_netdata_streaming` to match the registered Function name (`netdata-streaming`) and the file name. Update the registration in `functions.c:8-18`.
   - Delete the old `src/web/api/functions/function-streaming.{c,h}` files.
   - Update the build manifest (CMake source list under `src/web/api/functions/` or its parent) to remove `function-streaming.c` and add the two new sources.
   - All four bug fixes below (steps 1–4) land in the new `function-topology-streaming.c` after the split commit. The split is a single mechanical commit with no behavior change.
1. Bug B fix (Decision 2). Lowest risk; isolated to `streaming_topology_get_path_ids` (in the new `function-topology-streaming.c`). Ship after the split.
2. Bug A fix (Decision 1). Risk depends on chosen option.
3. Bug C fix (Decision 3). Adds synthetic upstream actors and corresponding link entries on the child view. Reuses existing `actor_type:"parent"` — no new actor type and no presentation block changes.
4. Bug D fix (Decision 4). Smallest patch; metadata only.
5. **(Removed.)** Defensive hardening for the streaming-path JSON parser (array length clamp + scalar range checks) is moved to a separate hardening SOW. This SOW does not touch `src/streaming/stream-path.c` at all.
6. **Maintenance documentation.** Confirmed with project owner 2026-05-06 — write `src/streaming/STREAM_PATH.md` co-located with the streaming-path source. The file must **not be referenced in `docs/.map/map.yaml`** so the learn-ingestion pipeline ignores it. (Co-location under `src/streaming/` is not by itself sufficient — `src/streaming/README.md` IS in the map.) Covers:
   1. What `stream.path.array` represents — chain of hops a host's data takes; per-host on every agent that knows about the host.
   2. Slot semantics — slot 0 = origin; slots 1+ = upstream hops (parents/proxies).
   3. Storage rule — `stream_path_set_from_json` stores received entries as-is; `rrdhost_stream_path_to_json` overlays/appends a fresh `rrdhost_stream_path_self()` at JSON-emit time only. Why the asymmetry exists (avoids storing volatile local fields like `first_time_t` and agent-event medians).
   4. Propagation — bidirectional from-below (`from_parent=false`: up + down); down-only from-above (`from_parent=true`: down only). Multi-hop convergence works because each forwarded host on a proxy gets its own sender (`stream-receiver-connection.c:188`).
   5. The cycle terminator — `if(!from_parent)` at `stream-path.c:423`. Why it must not be touched. What a future protocol redesign would need to replace it with first.
   6. Update triggers — connect (`stream-sender.c:157`); retention boundary movement, specifically `first_time_s` change (`rrdcontext-worker.c:99-104`); node_id update (`sqlite_metadata.c:288`, `command-nodeid.c:163`); parent disconnect (`stream-sender.c:171`). Note: triggers are sparse — convergence has lag; consumers must not assume "stored = current truth".
   7. Consumer guidance — for any feature that asks "is X a parent? what is X's chain?", read live state where possible; fall back to stored paths only as a hint. Lists known consumers: `function-topology-streaming.c` (post-split), `api_v2_contexts.c:425, 510`, `rrdhost_stream_path_total_reboot_time_ms` (`stream-path.c:145-158`).
   8. vnode special case — vnodes don't stream; their stored path is empty; emit-time self-append fills slot 0 with the collecting agent.
   9. Topology-function intended classification logic — short summary mirroring the SOW's "Intended classification logic" and "Intended actor set per agent" sections; cross-reference SOW-0012 in `.agents/sow/done/` for the bug history and fix design.
7. Tests covering each bug at the function level (parent view, child view, mid-chain view). Real-use validation against the reporter's environment after each fix lands.
8. Same-failure search across the codebase: any other column declared `"number"` that holds Unix epoch; any other consumer of `host->stream.path.array` that assumes self is stored — including `rrdhost_stream_path_total_reboot_time_ms` (`stream-path.c:145-158`) which shares the same blind assumption.

Validation plan:

- Unit-style: build a minimal test that constructs synthetic `STREAM_PATH` arrays for a 2-host parent/child scenario and asserts the classification, `child_count`, `actors[]`, `links[]`, and the streaming-path table types and values. The function emits text to a `BUFFER`, so the test runs the emission path and parses the JSON.
- Integration-style: re-run the captured Cloud queries after the fix is deployed to a build of the parent and the child, and re-fetch the screenshot UI on Cloud.
- Same-failure scan: `grep` for columns of type `"number"` that hold timestamps elsewhere in `function-streaming.c` and other Function emitters; check `rrdhost_stream_path_total_reboot_time_ms`.
- Decision 1 specific: verify that `received_nodes` populates correctly on the parent view after the fix (i.e., that `parent_descendants[localhost]` was also populated by the live-state walk).

Artifact impact plan:

- `AGENTS.md`: not affected. No workflow change.
- Runtime project skills: not affected directly.
- Specs (`.agents/sow/specs/`): no spec change. The streaming protocol is untouched. The topology function's classification semantics are local to `function-streaming.c` and need no spec.
- End-user/operator docs: Netdata Cloud frontend public docs do not document the streaming-topology Function's response shape at the field level; no docs update needed.
- End-user/operator skills: `query-netdata-cloud` skill should add a how-to for "Calling `topology:streaming` and interpreting actor types" — mandated by the skill's own live-catalog rule.
- SOW lifecycle: this SOW is opened in `pending/` and will move to `current/` only after Decisions 1–4 are answered.

Open-source reference evidence:

- No external reference required for these bugs — they are entirely internal to Netdata's streaming/topology code path.

Open decisions:

See "Implications And Decisions" below. Implementation cannot start until each is answered.

## Implications And Decisions

### Decision 1 — Fix scope for Bug A (parent classification)

Background: the streaming-path subsystem is designed for full multi-hop convergence and **must not be touched**. Each forwarded host has its own sender on a proxy, and `stream_path_send_to_parent` propagates child paths up the chain through proxies. The cycle terminator at stream-path.c:423 is load-bearing. The bug is that on the **apex parent** in any topology, convergence depends on the child re-emitting after a sparse trigger (retention boundary movement, node_id update, reconnect). Until that fires, the apex's storage of the child's path is stuck at the initial `[{child}]` from connect-time, so Phase 1 sees nothing. The topology function reads stored paths and therefore returns a wrong answer until convergence completes.

The fix is **entirely inside the topology function** (`function-streaming.c`). The streaming-path subsystem stays as-is.

Options:

1. **Localhost-only live-state fix.** For `host == localhost` only, replace Phase 1's path-based counting with a direct check on the local agent state via `rrdhost_status()` (the same helper the function already uses at lines 850, 1021, 1113, 1158, 1220, 1289 — do **not** access `host->receiver` directly). A host counts as an active streaming child iff `rrdhost_status(host, ...).ingest.type == RRDHOST_INGEST_TYPE_CHILD` and `s.ingest.status` is in `{RRDHOST_INGEST_STATUS_ONLINE, RRDHOST_INGEST_STATUS_REPLICATING}`. `child_count` becomes the count of such hosts. The implementation must populate **both** `parent_child_count[localhost]` AND `parent_descendants[localhost]` from the same root-index walk, so `received_nodes` (function-streaming.c:982-1002) stays consistent with the new count. The existing slot-0+ append at function-streaming.c:815-822 must skip localhost in this fix's path so we do not double-write `parent_descendants[localhost]`. For non-localhost hosts, keep the existing path-based logic unchanged.
   - Pros: contained in `function-streaming.c`. Zero streaming-protocol change. Trivially correct on the localhost side regardless of stored-path lag.
   - Cons: multi-hop / non-localhost parent classification (e.g., the topology view from a child showing whether its parent is itself a parent of further children) still depends on the stored streaming_path of children. Heals on the next sparse trigger.
   - Risk: low.

2. **All-host live-state fix.** Same as Option 1 but extend the live-state check to non-localhost hosts too: walk `rrdhost_root_index` once and compute parent classification for every host based on observable state (receivers attached, sender attached, virtual flag, archive status), not stored paths.
   - Pros: classification is correct immediately for every host on every view, regardless of stored-path lag.
   - Cons: more code, more cases to enumerate. Non-localhost hosts cannot be checked via `rrdhost_status()` for receiver-presence in the same way as localhost — that information is only locally observable for hosts whose children are also locally registered (the proxy/apex case). For pure remote children that are not themselves proxies, there is nothing more to count anyway, so the additional code mostly degenerates back to the Option 1 case.
   - Risk: low-medium (more surface to test).

**Recommendation: Option 1.** Smallest change that fixes the user-visible bug. Non-localhost classification heals on the next sparse trigger — acceptable degradation for this SOW. If, after this lands, the residual surfaces in real use, a follow-up SOW can extend the live-state check to all hosts (still no protocol change).

**Residual note (line 1197):** Option 1 fixes the classification but leaves a related symptom — the observer-mode outbound table at `function-streaming.c:1196-1208` calls `rrdhost_stream_path_get_host_ids(host, 0, ...)` directly to find the streaming destination. On the apex parent the stored path for a child is `[{child}]` only at connect-time, so this lookup finds no destination and `dst_hostname` falls back to the peer IP at function-streaming.c:1209. The IP fallback already produces a usable label; this heals on the next sparse trigger like the classification does. **No protocol change; if needed later, fix in `function-streaming.c` by reading live state for the destination too.**

### Decision 2 — Fix for Bug B (localhost-append for `from > 0`)

Background: `streaming_topology_get_path_ids` blindly appends `localhost->host_id` for any value of `from`. Of the six call sites (verified by grep), only one (`from=1`, Phase 1 parent counting) wants the append off.

Options:

1. **Gate the append on `from == 0`.** Smallest change: at function-streaming.c:243-244, change the condition from `if(!found_localhost && n < max && n > 0)` to `if(from == 0 && !found_localhost && n < max && n > 0)`.
   - Pros: minimal diff; existing five "want append on" call sites unchanged.
   - Cons: ties the append semantic to a magic value (`from == 0` happens to mean "full path"). Caller still has to know.
   - Risk: very low.

2. **Add an explicit boolean parameter.** `streaming_topology_get_path_ids(host, from, append_localhost, host_ids, max)`. Update all six call sites.
   - Pros: explicit semantics; future-proof.
   - Cons: more invasive diff; six call sites to touch.
   - Risk: low.

3. **Split into two helpers.** `streaming_topology_get_full_path_ids` (with append) and `streaming_topology_get_upstream_path_ids` (without). Make the intent obvious from the function name.
   - Pros: most readable.
   - Cons: most invasive diff.
   - Risk: low.

**Recommendation: Option 1.** Smallest, safest change. Pair with a comment at the helper explaining why the gate exists. If more flexibility is needed later, refactor in a follow-up.

### Decision 3 — Fix scope for Bug C (rendering parents on child view)

Background: actors are emitted only for hosts in `rrdhost_root_index`. The child has the parent's metadata in its `STREAM_PATH` array but no rrdhost record for it.

Options:

1. **Synthesize `parent` actors from `STREAM_PATH` entries that point outside `rrdhost_root_index`.** For each path entry whose host_id is not localhost and not already in `rrdhost_root_index`, emit an actor with the existing `actor_type:"parent"`. Use STREAM_PATH fields for attributes (hostname, host_id, node_id, claim_id, since, flags, capabilities, hops, first_time_t, start_time_ms, shutdown_time_ms). Add multi-hop links between every consecutive non-localhost path slot in a separate pass after the existing Phase 4 link loop.
   - Sub-specs the implementation must define before coding:
     - (a) De-duplication: skip emission if the path entry's host_id matches any host already iterated from `rrdhost_root_index`. (FE drops duplicate backend actor IDs at `<dashboard-repo>/src/domains/functions/topology/payload.js:263-265` — relying on FE dedup is not the agent's contract.)
     - (b) Sub-tables: synthetic actors populate only the `streaming_path` table from the STREAM_PATH struct fields. The `inbound`, `outbound`, `retention` tabs that depend on data the agent doesn't have for a remote upstream will render empty — acceptable for a single-agent view; aggregated views in Cloud will fill them from the parent's own response (whose `agent_id` matches that parent's actor_id, making it the authoritative source for that actor — see "Topology must be mergeable across agents" section).
     - (c) Link emission: a second loop after the Phase 4 link loop. For each synthesized chain, emit `link_type:"streaming"` for **every consecutive non-localhost path slot pair** (`slot[i] → slot[i+1]` for `i >= 1`), not only `slot[0] → slot[1]`. This means deep chains (e.g., `[child, parent_1, parent_2, parent_3]`) get all transitive links emitted: `parent_1 → parent_2`, `parent_2 → parent_3`. The existing Phase 4 already emits `child → parent_1` (`function-streaming.c:1407` with `max=2`).
     - (d) Link timestamps: synthesized links derive `discovered_at` and `last_seen` from the STREAM_PATH `since` and `first_time_t` fields of each path entry, NOT from `now`. This preserves cross-agent merge semantics.
   - FE coupling: with `actor_type:"parent"` reused, no change to `presentation.actor_types`, no change to `presentation.legend.actors`, no FE update required.
   - Pros: solves the reporter's third complaint. Uses data already local. No new actor type, no FE coupling, no presentation block additions.
   - Cons: synthesized parent's modal will show empty inbound/outbound/retention tabs in single-agent view. Aggregated Cloud views fill them.
   - Risk: low-medium. Touches actor emission loop and adds a small link-emission loop. Reuses existing infrastructure throughout.

2. **Defer.** Track in a follow-up SOW.
   - Pros: smaller PR, easier to review.
   - Cons: child view stays useless until the follow-up lands. Two of the reporter's three complaints (#1 the misclassification, #3 the parents-not-shown) are about the child view; #1 is fixed by Bug B alone but #3 is not addressed.

**Recommendation: Option 1.** The data is already local; the reporter asked explicitly; reusing the existing `parent` actor type avoids any FE coupling.

### Decision 4 — Fix for Bug D (Since column type)

Background: `since` is emitted as `uint64` Unix-epoch seconds at stream-path.c:88. Column type at function-streaming.c:355 is `"number"`. Frontend renders the raw integer (with locale-formatted separators).

Iteration-2 review with cloud-frontend access confirmed:

- The FE's `"timestamp"` case at `<dashboard-repo>/src/domains/functions/components/topology/actorModal/dataTable.js:52-58` @ commit `8d0258eb60aa32e3ee5fdd2144ef10b44f7995bc` auto-detects ms vs seconds: `const ts = ms > 1e12 ? ms : ms * 1000`. A value of `1777992161` (< 1e12) is multiplied by 1000 → correct date. A value of `1777992161000` (> 1e12) is used directly → also correct.

Options:

1. **Change column type AND post-process to ms inside `function-streaming.c` only.** Introduce a local helper that emits the path with `since * MSEC_PER_SEC`. Matches the in-tree style used for `db_from`/`db_to`.
   - Pros: stylistic consistency with the rest of `function-streaming.c`.
   - Cons: requires a custom emit path duplicating `rrdhost_stream_path_to_json`.
   - Risk: low.

2. **Change column type only.** Set type to `"timestamp"` at function-streaming.c:355; leave the data emission path unchanged (raw seconds). The FE auto-detects and converts.
   - Pros: minimal diff (one string change). No new helper.
   - Cons: depends on a FE auto-detection heuristic. If the FE ever tightens to require ms, this silently regresses. The threshold `1e12` (year 33658 in seconds, year 2001 in ms) is comfortably distant from realistic timestamps either way.
   - Risk: very low.

3. **Use a different column type** (e.g., `"datetime"` or `"duration_ago"`).
   - Pros: matches FE's intended semantic if `"timestamp"` is not the right type.
   - Cons: requires reading FE topology-table renderer to confirm the supported types — the topology-modal renderer's switch shows only `"timestamp"`, `"duration"`, `"badge"`, `"actor_link"`, plus a default. So this is not strictly necessary.

**Recommendation: Option 2.** Switched from Option 1 after iteration-2 FE review. The FE's auto-detection means there is no unit to align — the simplest fix is correct. The in-tree style argument for Option 1 is real but not load-bearing.

### Decision 5 — Backend graph filters

Background: the topology function currently advertises and applies optional backend graph-pruning filters (`node_type`, `ingest_status`, `stream_status`). The UI does not expose these filters, and graph pruning in the backend makes the topology response incomplete: actors can disappear, links can be suppressed, and synthetic parent emission has to reason about filtered-out local hosts. This conflicts with the product requirement that topology should expose the full graph.

Options:

1. **Remove backend graph filters.** Keep only `info` as an accepted non-pruning parameter. Always emit all actors and links known to the local agent plus synthetic upstream parents derived from `STREAM_PATH` when no local `RRDHOST` exists. UI facets may still filter or highlight client-side over the full response.
   - Pros: complete topology response; simpler actor/link flow; no filtered-out actor edge cases; matches the current UI behavior.
   - Cons: callers cannot request a smaller backend response by status/type.
   - Risk: low. These filters are not visible in the UI and are optional today.

2. **Keep backend filters.** Preserve current accepted params and filtering branches.
   - Pros: existing API shape unchanged for any hidden caller.
   - Cons: incomplete graph responses; complex synth/dedup semantics; contradicts the project-owner requirement that all nodes must be exposed.
   - Risk: medium. Keeps the current class of graph correctness traps.

**Decision: Option 1.** Remove `node_type`, `ingest_status`, and `stream_status` from `accepted_params` and delete backend actor/link/synthetic filter checks. `info` remains because it controls response metadata shape, not graph pruning.

## Plan

User decisions recorded 2026-05-06: Decision 1 = Option 1 (localhost-only live-state); Decision 2 = Option 1 (gate append on `from==0`); Decision 3 = Option 1 (synthesize `parent` actors, no new actor type); Decision 4 = Option 2 (column type only, FE auto-detects); Decision 5 = Option 1 (remove backend graph filters; keep only `info`). Defensive hardening for `stream-path.c` deferred to a separate SOW.

PR commit order: split → B → A → D → C → doc → tests → validation.

0. **File split** (precondition; mechanical, no behavior change). Create `function-topology-streaming.{c,h}` and `function-netdata-streaming.{c,h}`; move `function_streaming_topology` + helpers and `function_streaming` (renamed `function_netdata_streaming`) respectively; delete `function-streaming.{c,h}`; update `functions.c` includes/registrations and the build manifest. `STREAMING_FUNCTION_UPDATE_EVERY` is duplicated in each new file (per project owner's call — no shared header). All subsequent line-number references in this SOW are pre-split; the bug-fix commits land in the new `function-topology-streaming.c` and may be at different line numbers.
1. **Bug B fix** (Decision 2 = Option 1): one-line change at the equivalent of `function-streaming.c:243-244` (now in `function-topology-streaming.c`) plus comment. Add unit-style test asserting Phase 1 parent-counting on a child agent yields `parent_child_count[localhost] == 0`.
2. **Bug A fix** (Decision 1 = Option 1, localhost live-state): rewrite Phase 1's localhost write into both `parent_child_count` and `parent_descendants` from a `rrdhost_root_index` walk that uses `rrdhost_status()` (the abstraction the function already uses) and counts hosts where `s.ingest.type == RRDHOST_INGEST_TYPE_CHILD` and `s.ingest.status ∈ {ONLINE, REPLICATING}`. **No `host->receiver` access.** Skip the slot-0+ append for localhost in `parent_descendants` to avoid double-write. No protocol change.
3. **Bug C fix** (Decision 3 = Option 1, no new actor type): add synthetic-actor emission loop after the existing `rrdhost_root_index` loop in Phase 3 (dedup against local `rrdhost_root_index` actors, streaming_path table built directly from STREAM_PATH struct fields). Synthesized actors use `actor_type:"parent"` — existing type, no presentation block changes. Add a second link-emission loop after the existing Phase 4 loop that emits `link_type:"streaming"` for every consecutive non-localhost path slot pair (`slot[i] → slot[i+1]` for `i >= 1`); link timestamps derived from STREAM_PATH `since`/`first_time_t`, not `now`.
4. **Bug D fix** (Decision 4 = Option 2): change the `"since"` column type from `"number"` to `"timestamp"`. No data change.
5. **Backend filter removal** (Decision 5 = Option 1): remove `node_type`, `ingest_status`, and `stream_status` parsing, accepted params, and actor/link/synthetic emission checks. Keep `info`.
6. **Maintenance documentation**: write `src/streaming/STREAM_PATH.md` per the outline in the implementation plan above. Verify the file is **not referenced in `docs/.map/map.yaml`** (co-location under `src/streaming/` is not by itself sufficient).
7. **Same-failure scan**: grep for other `"number"`-typed columns holding Unix-epoch values; grep for other consumers of `host->stream.path.array` that may share Bug A's assumption (specifically `rrdhost_stream_path_total_reboot_time_ms` at `stream-path.c:145-158`).
8. **Tests**: see Validation Plan; minimum coverage = one test per bug, plus a test for the line 1197 outbound-table behavior (verify peer-IP fallback works for stale-storage cases).
9. **Validation**: build & deploy to the reporter's parent and child; re-fetch Cloud responses; verify all acceptance criteria.

## Execution Log

### 2026-05-05

- Read full Slack thread reporting the issue (11 messages including the screenshot the reporter shared).
- Loaded `query-netdata-cloud` skill helpers from `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`.
- Resolved the reporter's space, room, and node UUIDs.
- Captured live Cloud Function responses for `topology:streaming` on both parent and child; saved under `<repo>/.local/audits/streaming-topology/`.
- Confirmed Bugs A, B, C, D, E with file:line evidence.
- Drafted SOW iteration-1.
- Ran an independent read-only review batch for iteration-1; outputs preserved under `<repo>/.local/audits/streaming-topology/`.
- Ran the same independent review scope for iteration-2 with the cloud-frontend source path included; outputs preserved under `<repo>/.local/audits/streaming-topology/`.
- Iteration-2 corrections applied to this SOW: caller count 5 → 6 (added line 1045); Bug A trace step 4 reworded (the call exists but is a no-op due to `host->sender == NULL` on the parent side); Decision 4 recommendation switched to Option 2 (FE auto-detects seconds via `ms > 1e12 ? ms : ms * 1000`); Decision 1 sub-spec added (must populate both `parent_child_count[localhost]` and `parent_descendants[localhost]`); Decision 3 sub-specs added (filter, dedup, sub-tables, second link pass); same-failure scan target added (`rrdhost_stream_path_total_reboot_time_ms`); line 1197 residual noted; uint16_t truncation hardening item added; SOW sanitized (hostnames, UUIDs, IPs, names, customer-space identifiers replaced with placeholders).
- 2026-05-06 — confirmed with project owner that the streaming-path subsystem must not be touched by this SOW. Decision 1 collapsed from three options (protocol fix / function-only / hybrid) to two options that are both contained in `function-streaming.c`: (1) localhost-only live-state classification — recommended; (2) all-host live-state classification — deferred to a follow-up if the residual surfaces. Removed all references to a protocol fix follow-up. The cycle terminator at stream-path.c:423 stays load-bearing and untouched.
- 2026-05-06 — added the project owner's intended classification logic (six rules) and intended actor set (per-agent count formula) to the Analysis section. Added the cross-agent mergeability requirements section (stable actor IDs, stable link tuples, timestamps for tie-breaking, no overreach). Synthesized upstream `parent` actors must preserve the canonical actor-id format and derive link timestamps from the `STREAM_PATH` struct so the merge layer can reconcile views from multiple parents.
- 2026-05-06 — added scope: split `function-streaming.{c,h}` into `function-netdata-streaming.{c,h}` and `function-topology-streaming.{c,h}`, with `function_streaming` renamed to `function_netdata_streaming`. The bug fixes land in the new topology file. Added scope: write `src/streaming/STREAM_PATH.md` as a co-located maintenance reference (not picked up by learn ingestion). Both additions confirmed with project owner.
- 2026-05-06 — iteration-3 independent read-only review batch completed; outputs preserved under `<repo>/.local/audits/streaming-topology/`. Project owner's corrections applied: (1) **dropped `remote_parent` actor type** — synthesized upstream actors reuse the existing `actor_type:"parent"`, no new type, no presentation block changes, no FE coupling; (2) Decision 3 sub-spec adds multi-hop link emission for every consecutive non-localhost path slot, not only `slot[0] → slot[1]`; (3) Decision 3 sub-spec mandates synthesized link timestamps derive from STREAM_PATH `since`/`first_time_t`, not `now`; (4) Decision 1 wording corrected — use `rrdhost_status()` (`s.ingest.type == CHILD` and `s.ingest.status ∈ {ONLINE, REPLICATING}`), no `host->receiver` access; (5) Decision 1 sub-spec — skip slot-0+ append for localhost to avoid double-write to `parent_descendants[localhost]`; (6) **deferred all `stream-path.c` defensive hardening** (uint16_t array clamp + scalar range checks) to a separate hardening SOW — this SOW now touches zero shared subsystems beyond the topology function and a new doc; (7) replaced workstation paths (`~/src/...`) with repo-relative paths (`<repo>/...`) and dashboard-repo placeholder (`<dashboard-repo>/...`); (8) corrected STREAM_PATH.md framing — the protection from learn ingestion is "not referenced in `docs/.map/map.yaml`", not "not under `docs/`"; (9) pinned cloud-frontend evidence to commit `8d0258eb60aa32e3ee5fdd2144ef10b44f7995bc` next to the `dataTable.js:52-58` citation. STREAMING_FUNCTION_UPDATE_EVERY shared macro: project owner's call — duplicate per file, no shared header. Aggregator self-marker proposal dropped — existing `data.agent_id` is sufficient for cross-agent merge.
- 2026-05-06 — project-owner decision: remove backend topology graph filters because the UI does not expose them and the backend must return the full graph. Recorded as Decision 5. Implementation updated `function-topology-streaming.c` so `accepted_params` only advertises `info`; removed `node_type`, `ingest_status`, and `stream_status` parsing and all actor/link/synthetic filter checks.
- 2026-05-06 — revisited topology flow after removing filters. Added a separate `local_actor_ids` set for all `RRDHOST`-backed actors, keeping `emitted_actors` as only the actors actually written to JSON. Synthetic upstream parents are now skipped when a local `RRDHOST` actor exists, not because a filtered actor happened to be present or absent. `parent_descendants[localhost]` is now written by one live-state pass: vnodes as `virtual`, active/repl children as `streaming`, and disconnected/history-only local hosts as `stale`; the path walk no longer has a competing localhost stale branch. Phase 4 now emits vnode links as `link_type:"virtual"` to localhost regardless of stored path length, registers each emitted link in `emitted_links`, and the synthetic link pass relies on that shared dedup instead of the previous `phase4_emitted` heuristic.
- 2026-05-06 — validation so far: focused compiler syntax check of `src/web/api/functions/function-topology-streaming.c` passed using the existing `functions.c` compile flags from `compile_commands.json`. Full `cmake --build build-clion --target netdata -j4` did not reach code compilation because CMake reconfigured and failed while fetching the pre-existing Sentry/crashpad dependency (`mini_chromium` HTTP 400 / expected acknowledgments). This is an environment/dependency fetch failure, not a compiler error from the topology file.
- 2026-05-06 — after project owner installed the build locally, queried the installed local agent through bearer-protected direct API without printing tokens or durable identifiers. Sanitized runtime summary: `status=200`, `accepted_params=["info"]`, `actors=22`, `links=22`, actor type counts `child=13`, `parent=2`, `stale=1`, `vnode=6`, link type counts `streaming=15`, `virtual=6`, `stale=1`, duplicate actor ids `0`, duplicate link tuples `0`, vnode stale links `0`.
- 2026-05-06 — project owner decision: do not add tests before first PR publication. Open a draft PR now so online reviewers can inspect the code. Tests remain a known validation gap and this SOW remains in progress until follow-up validation is complete or the project owner explicitly accepts closure without tests.

## Validation

Partial, implementation still in progress.

- Focused syntax validation passed for `src/web/api/functions/function-topology-streaming.c` using the existing `functions.c` compile command from `compile_commands.json`, replacing the source path and using `-fsyntax-only`.
- Full local build attempted with `cmake --build build-clion --target netdata -j4`. It failed during CMake reconfigure while fetching the pre-existing Sentry/crashpad dependency (`external/crashpad/third_party/mini_chromium/mini_chromium`, HTTP 400 / expected acknowledgments) before compiling Netdata sources.
- Same-failure search for removed backend filter identifiers in `function-topology-streaming.c` found no remaining `value_in_csv`, `filter_node_type`, `filter_ingest_status`, `filter_stream_status`, `node_type:`, `ingest_status:`, or `stream_status:` references.
- Installed-agent runtime validation via direct local Function call: `status=200`; `accepted_params` contains only `info`; graph contains 22 actors and 22 links; actor/link duplicate checks are zero; vnode stale-link check is zero.
- Tests: deferred by project-owner decision for the first draft PR. This is a known gap for reviewers, not a completed validation item.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Followup

- If the multi-hop / non-localhost classification residual surfaces in real use after this SOW lands, open a follow-up SOW that extends the live-state classification (Decision 1 Option 1) to all hosts (Option 2 in this SOW). **No protocol change is contemplated** — both this SOW and any follow-up are entirely inside the topology function.
- Add a how-to under `docs/netdata-ai/skills/query-netdata-cloud/how-tos/` describing how to call `topology:streaming` and how to interpret actor types and the streaming-path table.
- **Open a separate SOW for streaming-path defensive hardening** in `src/streaming/stream-path.c`: (a) clamp incoming JSON array length to `UINT16_MAX` before `callocz` to prevent uint16_t truncation of `host->stream.path.size`/`used`; (b) add scalar range checks for `hops` (int16_t), `start_time_ms` (uint32_t), `shutdown_time_ms` (uint32_t) against negative inputs from a malformed peer at `stream-path.c:313-317`. Pre-existing defensive gap, not introduced by this work.

## Regression Log

None yet.
