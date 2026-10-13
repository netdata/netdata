# nRPC — Netdata's ad-hoc RPC system

nRPC is the mechanism behind the product feature called **Functions**: named
callable endpoints that plugins, streaming children and the daemon itself
expose, and that users, parents and Netdata Cloud invoke on demand.

This file is the entry point. It gives you the vocabulary, the object model,
the lifecycles and the locking rules — the global picture the per-site code
comments assume you already have. Read it once, top to bottom; afterwards
every file in this directory should read linearly.

## Vocabulary

Pairs and terms that trip people up, in the order the code will hit you with
them:

| Term | Meaning |
|---|---|
| function (wire) / method (code) | The same thing. The wire keywords (`FUNCTION`, `FUNCTION_DEL`, ...), HTTP endpoints and registered name strings are contracts and keep the "function" word; the code says method/call/handler for the mechanism. |
| transaction (wire) / call_id (code) | The same correlation id. Every plugin SDK says "transaction"; this code says call_id. |
| owner (`NRPC_OWNER`) | The identity of a registry and everyone who talks to it: the owning host OBJECT carried as an opaque token. Index key and callback argument at once. The component never dereferences it. |
| registry (`struct nrpc_registry`) | One host's table of methods, living in a component-global index keyed by the owner. |
| outer / inner | OUTER = the component-global registries index (owner → registry). INNER = one registry's own dictionaries: `dict` (name → slot) and `pending_dels.dict`. An "outer handle" pins the registry struct's memory — but NOT the objects it points to; that is the gate's job. |
| slot (`struct nrpc_method_slot`) | The stable INNER dictionary value for one method name: a spinlock, a pointer to the current descriptor, and the tombstone. Identity lives here. |
| descriptor (`struct nrpc_method`) | One registration event: an IMMUTABLE, refcounted object holding the method's whole state (attributes, handler, transport ref, serving-handle ref, epoch stamp). State lives here. A re-registration builds a new one and swaps the slot's pointer as one unit; no field is ever mutated in place. |
| registration struct (`*_desc`) | The caller-filled stack input to `nrpc_method_register()` / `nrpc_method_register_builtin()`. Never stored; the descriptor is built from it. |
| entry ref / dispatcher ref | The two counters of `NRPC_LIFETIME` (nrpc-lifetime.h). Entry refs are held by every party that STORES an object and decide when it is freed. Dispatcher refs are transient, taken acquire-or-fail around one USE, and are what teardown drains. |
| gate | A dispatcher counter used as an operation barrier. Every "gate" in this component is the same `NRPC_LIFETIME` dispatcher counter — only the object being gated differs: the registry's operation gate (inner-dictionary use), the serving handle's dispatch gate (cancel/progress), the transport's send gate. |
| retire / drain | Teardown step one on any `NRPC_LIFETIME`: mark the dispatcher counter dead (all later acquires fail forever), then wait for the holders already inside to leave. |
| serving handle (`struct nrpc_serving_handle`) | The liveness token of a thread that registers methods. Every descriptor that thread built points at it; when the thread exits, all of its methods become unavailable at once. |
| transport (`struct nrpc_transport`) | Carries calls to remote handlers (pluginsd for local plugins, streaming for children): the lifetime shell plus the owner's payload (for pluginsd, the PARSER). |
| owner vtable | The callbacks and epoch pointer an owner supplies at registry creation (`struct nrpc_registry_owner`), stored in the entry, guarded by `owner_spinlock`. |
| disarm | Destroy clearing the owner vtable (name freed, epoch NULLed, callbacks NULLed) so late readers degrade to no-ops instead of calling into a dying owner. A NULL epoch IS the "disarmed" marker. |
| epoch | The owner's liveness GENERATION (`OBJECT_STATE`, libnetdata/object-state). The owner bumps it whenever the host goes unreachable and comes back — e.g. a streaming child reconnect. Each descriptor is stamped with the epoch current at its registration and is available only while the two match, so a reconnect silently retires everything the previous connection registered until it re-registers. |
| tombstone | `slot->unregistered`: set by an unregister between its validation and its `dictionary_del`, cleared by any re-registration. Lets lookups in that window answer "unregistered by the plugin" instead of using a dying entry. Read atomically with the descriptor pointer by `nrpc_slot_acquire()`. |
| dyncfg | An application built ON TOP of this component (the reserved `config` method names). Not part of the mechanism. |

## Object model

```text
NRPC_OWNER (opaque host token)
   |
   |  component-global registries index ("outer"; key = owner as hex)
   v
struct nrpc_registry ------------------ owner vtable (name, epoch, callbacks;
   |         |                          guarded by owner_spinlock; DISARMED
   |         |                          at destroy)
   |         +-- NRPC_LIFETIME lifetime (the operation GATE: dispatcher
   |         |                          counter only)
   |         +-- pending_dels.dict      (queued FUNCTION_DEL names)
   v
dict ("inner"; key = sanitized method name)
   |
   v
struct nrpc_method_slot { spinlock; method; unregistered }   <- identity
   |
   v
struct nrpc_method (IMMUTABLE, refcounted)                   <- state
   |-- help/tags STRING copies
   |-- handler + handler_data      (for PLUGIN/STREAM sources handler_data
   |                                IS the struct nrpc_transport *, entry-
   |                                pinned; DAEMON data is caller-owned and
   |                                never pinned)
   |-- epoch stamp
   +-- struct nrpc_serving_handle *  (entry-pinned)

   both pins are NRPC_LIFETIME entry refs
```

Holders of a descriptor reference (in-flight calls, `NRPC_METHOD_ACQUIRED`
handles, catalog visits) read it lock-free forever — there is nothing mutable
to race on — and release touches no dictionary, so a late release after the
host died cannot dangle.

## One invocation, three views

The same call appears as three structs, in order:

1. `struct nrpc_call_spec` — the CALLER's input, stack-filled (nrpc.h).
2. `struct nrpc_call` — the in-flight record, private to nrpc-calls.c,
   tracked in the process-wide in-flight calls table keyed by call_id.
3. `struct nrpc_request` — what the HANDLER sees.

## Execution modes — and who deletes the in-flight record

| Mode | Chosen by | Handler runs | Record deleted by |
|---|---|---|---|
| sync | `descriptor->sync == true` | inline, caller's thread | `nrpc_call()` itself, right after the handler returns |
| async, no wait | `sync == false`, `spec->wait == false` | started inline; answers later | `nrpc_call_nowait_finished()` when the result arrives |
| async, wait | `sync == false`, `spec->wait == true` | started inline; caller blocks | `nrpc_call_wait_free()` — by the waiter if the answer arrived in time, else by the completion callback (see the ownership-protocol comment in nrpc-calls.c) |

The async rows rest on one contract: a handler MUST invoke `result.cb`
exactly once, on success and failure alike — that call is what retires the
record; the handler's return value only says whether the call was accepted.

Cancel and progress dispatch through the CALL's own descriptor: they gate on
the serving handle of the registration whose handler actually ran, never on
whatever currently owns the name in the registry.

## Lifecycles

**Register a method** (`nrpc_method_register`):
acquire the registry handle → enter the gate → sanitize/validate the name →
build the complete descriptor (strings copied, transport entry-pinned,
serving handle acquired, epoch stamped, priority normalized) →
`dictionary_set`: on insert the slot takes the descriptor over; on conflict
the callback swaps the slot pointer and releases the displaced descriptor →
leave the gate → notify the owner (`changed` callback, outside the gate).

**Execute a call** (`nrpc_call`):
authorize (gate + find → an OWNED descriptor ref + RESTRICTED/access checks)
→ insert the in-flight record → run the handler per the mode table above →
the record's deletion releases the descriptor ref.

**Unregister a method** (`nrpc_method_unregister`):
gate → pin the current descriptor → validate against IT (plugin-source
ownership, dyncfg protection) → under the slot lock, verify the slot still
holds that exact descriptor and set the tombstone → `dictionary_del` →
queue the FUNCTION_DEL name (if the owner streams) → leave the gate →
notify the owner.

**Tear down a host's registry** (`nrpc_registry_destroy`):
disarm the vtable + unlink from the outer index (ONE critical section under
`owner_spinlock`) → release the lock → retire the gate (all later operations
acquire-fail; drain the ones inside) → destroy the two inner dictionaries.
Descriptors still pinned by in-flight calls simply outlive their dictionary.

**Tear down a serving thread** (`nrpc_serving_finished`):
retire the handle's dispatcher counter (drains in-flight cancel/progress
dispatches) → drop the thread's base entry ref; the handle is freed when the
last descriptor pointing at it dies.

**Tear down a transport** (owner protocol, nrpc-transport.h):
mark dead and drain → invalidate the owner payload → drop the base ref.

## Locking and references

Lock order, strict, outermost first:

```text
owner_spinlock  ->  outer index locks  ->  inner dictionary locks  ->  slot spinlock
```

- The **gate is not a lock** — it is a refcount, so it has no position in the
  order. Destroy retires it only AFTER releasing `owner_spinlock` (gated
  readers snapshot the epoch under that lock; retiring while holding it would
  deadlock the drain).
- The slot spinlock is always innermost and is never held across any
  dictionary call.
- Owner callbacks: `changed` fires OUTSIDE the gate; `wants_del_journal`
  fires INSIDE it and must stay lock-free.
- **THE invariant: gated sections must never block.** Some registry destroys
  run under the rrd write lock, and the retire drain waits for every gated
  reader — a gated section that blocks turns that wait into an agent-wide
  stall. No gated section takes an rrd lock or performs blocking I/O.

Reference model in one line: dictionary item refs pin STRUCT MEMORY
(registry, slot); descriptor refs pin STATE (and, transitively, the serving
handle and transport); dispatcher refs prove an object is safe to USE right
now.

## File map

Reading order — conceptual, the glossary and model before the mechanics
(the two `-internals` headers and the transport are included by files
listed above them):

| File | Owns |
|---|---|
| `nrpc.h` | the whole public API in one document (plus the transport header it includes for you): concept glossary, owner contract, registration, calls, serving threads, builtins, the catalog |
| `nrpc-lifetime.h` | the two-counter lifetime shell — the vocabulary every teardown uses |
| `nrpc-transport.h` | the transport: that shell plus the owner's payload (separate because it is legitimately included on its own) |
| `nrpc-internals.h` | the internal contract: registry entry, gate, slot/descriptor model, registries index |
| `nrpc-serving-internals.h` | the serving handle's internal surface (kept tiny so the implementation stays a leaf) |
| `nrpc-registry.c` | registries index, descriptor lifecycle, register/unregister/find/availability |
| `nrpc-calls.c` | in-flight calls table, authorization, the three execution modes, cancel/progress |
| `nrpc-catalog.c` | every consumer that iterates the registry: streaming re-list, JSON, dict export, cloud manifest, pending FUNCTION_DELs |
| `nrpc-serving.c` | the serving handle: a registering thread's liveness token |
| `nrpc-builtin.c` | the thin adapter for daemon-implemented synchronous methods |
| `nrpc-unittest.c` | the executable contract suite — the suite headers are prose documentation of what the component promises |

Task index:

- *Add a daemon-implemented function* → `nrpc-builtin.c` (a one-screen file
  that is exactly that recipe).
- *Why does my function show as unavailable?* → the availability pipeline:
  first the tombstone (slot-side, handed to the caller by
  `nrpc_slot_acquire()`), then `nrpc_method_is_available_at()` in
  nrpc-registry.c: serving handle running, entry armed, epoch match — in
  that order.
- *Understand teardown* → nrpc-lifetime.h first, then `nrpc_registry_destroy()`.
- *Change what parents/Cloud/users see* → nrpc-catalog.c.
- *Cancel/progress plumbing* → nrpc-calls.c, `nrpc_call_cancel_internal()`
  and `nrpc_call_progress()`.

## Neighbours that are NOT in this directory

Grep will otherwise mislead you:

- `nrpc_call_error()` — libnetdata/json/json-c-parser-inline.c (declared in
  json-c-parser-inline.h).
- `nrpc_sanitize_name()` — libnetdata/sanitizers/sanitizers-functions.c.
- `PLUGINSD_KEYWORD_FUNCTION*`, `PLUGINSD_FUNCTION_CONFIG` and
  `functions_stop_monotonic_update_on_progress()` —
  libnetdata/functions_evloop/functions_evloop.h.
- `PLUGINSD_LINE_MAX` — libnetdata/libnetdata.h (functions_evloop only uses
  it).
- The owner side of the vtable (what a host actually does in `changed` /
  `wants_del_journal`, and the epoch it hands over) — database/rrdhost.c,
  above `rrdhost_nrpc_registry_owner()`.
