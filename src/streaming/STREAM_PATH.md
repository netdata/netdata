# Streaming-path subsystem — maintenance reference

## 1. What `stream.path.array` represents

For every `RRDHOST` known to the local agent — its own `localhost`, every
real child currently or previously connected, and every vnode registered
locally — the `host->stream.path` field stores a small ordered array of
[`STREAM_PATH`](stream-path.c) entries. Each entry describes one hop in the
chain that this host's data takes from its origin upstream toward whichever
agent currently serves as the apex of the chain.

The chain is per-host. On any given agent, two hosts can have different
chains stored.

## 2. Slot semantics

Entries are sorted ascending by the `hops` field (`compare_by_hops`). Slot 0
has the lowest hops and is the **origin** — the agent where the data was
collected. Slots 1+ are the **upstream hops**: the agent at slot 1 is the
direct parent of slot 0, the agent at slot 2 is the parent of slot 1, etc.

Helper for read access by index: `rrdhost_stream_path_get_host_ids(host,
from, host_ids, max)` (returns just the UUIDs starting at `from`).

For full per-entry data with a callback: `rrdhost_stream_path_visit(host,
from, cb, userdata)` (added by SOW-0012; mirrors the get_host_ids locking
pattern but exposes hostname, hops, since, capabilities, flags, etc.).

## 3. Storage rule (received vs emitted)

This is the single most important invariant to understand:

- **Storage**: `stream_path_set_from_json(host, json, from_parent)` clears
  the existing array and stores exactly the entries received in the JSON,
  then sorts by hops. **Self is not added at storage time.**
- **Emission**: `rrdhost_stream_path_to_json(wb, host, key, add_version)`
  iterates the stored array. For any entry whose `host_id == localhost->
  host_id`, it overlays the entry with a fresh `rrdhost_stream_path_self()`
  to refresh the volatile fields (timing medians, retention boundaries). If
  no stored entry matches localhost, it appends a fresh self at the end.

Why the asymmetry exists: `rrdhost_stream_path_self()` reads `first_time_t`
from `rrdhost_retention()` and `start_time_ms` / `shutdown_time_ms` from
`get_agent_event_time_median()`. These change continuously. Storing them
would either require a watcher to keep storage current, or accept that
storage always lags the live values. Appending self at emit time avoids
both problems by always emitting current self while keeping the stored array
constant for everyone else's view of the host.

**Consequence**: any consumer that reads `host->stream.path.array` directly
(or via `rrdhost_stream_path_get_host_ids` / the visitor) is reading
storage, which may lag the live wire format. See §7.

## 4. Propagation rules

`stream_path_set_from_json(host, json, from_parent)` runs the
[propagation switch](stream-path.c) at the bottom:

```c
if(!XXH128_isEqual(old_hash, new_hash)) {
    if(!from_parent)
        stream_path_send_to_parent(host);
    stream_path_send_to_child(host);
}
```

| Trigger source | `from_parent` | Path emitted UP | Path emitted DOWN |
|---|---|---|---|
| Path message received from a **child** (via `pluginsd_parser`) | `false` | YES (only meaningful on a proxy where `host->sender` is set) | YES (echo back to the child with self appended) |
| Path message received from our **upstream parent** (via `stream-sender-execute`) | `true` | NO (cycle terminator) | YES (forward downstream to our children, only meaningful on a proxy) |

Multi-hop convergence works because each forwarded host on a proxy gets its
own sender — see `stream-receiver-connection.c:188` where the receiver
creates the new host with `config.send.enabled` propagated. The same host
record on the proxy has both a receiver (from below) and a sender (to
above), so a child-originated update flows all the way to the apex.

## 5. The cycle terminator

The `if(!from_parent)` guard at `stream-path.c:423` is **load-bearing**.
Without it, every parent-to-child echo would generate a child-to-parent
re-send, generating another echo, forever.

Combined with the volatile self-overlay (see §3), even the hash check at
`stream-path.c:422` cannot stop oscillation: the live timing fields change
between emits, so the hash would differ on every round-trip even when the
structural content is identical.

If a future protocol redesign needs to remove this guard (e.g., to make
storage authoritative on every node so consumers don't have to read stored
data with a "may be stale" caveat), it must FIRST replace the cycle
terminator with a different mechanism (e.g., a per-message generation
counter that gets incremented only by the originating node) AND make the
self-overlay deterministic for hash purposes (split volatile fields out of
the hash input).

## 6. Update triggers

`stream_path_send_to_parent` and `stream_path_send_to_child` are called
from a small, sparse set of triggers — they do NOT fire on every metric
write:

- **Connect** — `stream_sender_on_ready_to_dispatch` at
  `stream-sender.c:157` calls `stream_path_send_to_parent(localhost)` when
  the sender becomes ready.
- **Retention boundary movement** — `rrdcontext_post_process_updates` at
  `rrdcontext-worker.c:99-104` calls `stream_path_retention_updated(host)`
  only when `host->retention.first_time_s` changes. This is sparse — once
  per DB rotation / archive event.
- **node_id update** — `sqlite_metadata.c:288` (when persisted) and
  `command-nodeid.c:163` (when receiving a NODE_ID command from a child)
  call `stream_path_node_id_updated(host)`.
- **Parent disconnect** — `stream-sender.c:171` calls
  `stream_path_parent_disconnected(s->host)` when our upstream connection
  drops. This truncates self's stored chain back to localhost.

Storage convergence has lag. Right after a child connects, the parent's
storage of the child's path is `[child]` only. After the next sparse trigger
fires on the child (typically minutes-to-hours), the parent's storage
catches up to the full chain.

## 7. Consumer guidance

For any feature that asks "is X a parent? what is X's chain?", read live
state where possible (`rrdhost_status()` exposes `s.ingest.type` and
`s.ingest.status` per host, no need to touch `host->receiver` or
`host->sender` directly). Fall back to stored paths only as a hint, with
the understanding that storage lags the live state.

Known consumers of `host->stream.path.array` in this tree:

- `src/web/api/functions/function-topology-streaming.c` — the
  `topology:streaming` Function. Uses both `streaming_topology_get_path_ids`
  (paths for actor/link emission) and `rrdhost_stream_path_visit` (Bug C
  synthesis from SOW-0012). Reads stored data, falls back to the
  rrdhost-derived live data via `rrdhost_status()` for the localhost
  classification (Bug A in SOW-0012).
- `src/database/contexts/api_v2_contexts.c:425, :510` — emits
  `streaming_path` per host via `rrdhost_stream_path_to_json` (so the
  emit-time self-append fires). No direct array access.
- `src/streaming/stream-path.c:145-158` — `rrdhost_stream_path_total_reboot_time_ms`
  walks the stored array looking for localhost's own entry. **This shares
  Bug A's blind assumption**: on the apex parent, the localhost entry is
  not in storage, so this function returns 0 silently. Tracked as a
  same-failure follow-up in SOW-0012.

## 8. vnode special case

Vnodes do not stream — they are virtual hosts collected directly by the
local agent (e.g., SNMP devices). Their stored `stream.path.array` is empty.
At JSON-emit time, `rrdhost_stream_path_to_json` appends a fresh self
(the collecting agent's identity) so the wire format always shows a vnode
hop into the collector at slot 0.

For any consumer that synthesizes actors from path entries (e.g., the
topology function's Bug C synthesis), vnodes contribute nothing — their
visits via `rrdhost_stream_path_visit` return zero entries.

## 9. Topology-function classification logic

The `topology:streaming` Function classifies each entry in
`rrdhost_root_index` as `parent`, `child`, `vnode`, or `stale`. The intended
rule (project-owner confirmed in SOW-0012):

1. **Source of truth = `rrdhost_root_index`.** Every actor on the
   topology graph corresponds to either an entry in this index OR a
   synthesized "remote parent" derived from path data (Bug C synthesis).
2. **For each non-vnode node, slot 0 of its `stream.path.array` is the
   origin (a child role in this chain) and slots 1+ are upstream parents.**
3. **A node X is classified `parent` if X appears at slot 1+ in any
   node's path.** Otherwise `child`.
4. **vnodes** are tagged `vnode` directly — they have no upstream chain
   visible in their own stored data.
5. **Stale** nodes (`s.ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED`) are
   tagged `stale` directly and rendered as a stale link to localhost.

For the localhost itself, classification reads live state via
`rrdhost_status()` (count of non-virtual children with `s.ingest.type ==
CHILD` and `s.ingest.status` ∈ `{ONLINE, REPLICATING}`). This was Bug A in
SOW-0012 — the path-based check was unreliable on apex agents because
storage hadn't converged yet.

For the cross-agent merge (Cloud combines topology responses from multiple
parents into a single graph), the agent's response carries a stable
`agent_id` at the top level. The merge layer maps it to the canonical
actor_id form (`netdata-machine-guid:<agent_id>`) and uses that to identify
which actor in `actors[]` is the response's "self" — that response is the
authoritative source for that actor's attributes.

For the bug history, decisions, and design rationale, see
[`SOW-0012`](../../.agents/sow/done/SOW-0012-20260505-streaming-topology-classification-bugs.md)
once it lands in `done/`. While the SOW is in progress it lives at
`.agents/sow/current/`.
