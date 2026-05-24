# Netdata's Existing `plugin-ipc` (netipc) Library — Inventory and Analysis

Purpose: provide the working context for the SNMP-trap design discussion. A trap listener will almost certainly be its own plugin process. For the purpose of clarifying what netipc actually does, it is important to **separate the two distinct IPC paths** that exist in Netdata, because they are very different in shape and purpose.

## What netipc IS — and what it is NOT

**Plugin → Netdata Agent (data path, NOT netipc)**

A Netdata plugin pushes its data to the agent over **stdio pipes** using the existing plugin protocol (the PLUGINSD line protocol on the agent side, the `rt::PluginRuntime` Rust crate or equivalents on the plugin side). This is the simplest and fastest method to push collected data into the agent. It is one-way (plugin → agent), framed by text or binary lines, and bounded by the OS pipe buffer. NetFlow uses this. The future trap-listener plugin will use this too.

`plugin-ipc` (netipc) is **not involved** in this data path. It does not carry metric samples or log lines from plugins to the agent.

**Plugin ↔ Plugin (enrichment fabric, this IS netipc)**

`plugin-ipc` exists for a different problem: a plugin sometimes needs information **from another plugin** to enrich its own output (or wants to expose its own derived state to other plugins). Examples:

- `ebpf.plugin` watches process events and needs cgroup names for them. `cgroups.plugin` already knows that mapping. Today, `cgroups.plugin` runs a netipc server publishing a typed `cgroups-snapshot` service; `ebpf.plugin` is the client and looks up `pid → cgroup` at enrichment time. Netdata-the-agent is not on the path.
- A future trap listener will need to enrich incoming PDUs with "what device is this source IP? what profile applies? what is the ifIndex→name table?" — the natural shape is the trap listener as a netipc *client* of the SNMP-polling plugin (which is already the in-tree authority for those mappings) and possibly of `snmp_topology`.
- A future trap listener may also want to *publish* its own state to other plugins (e.g., "I just saw `linkDown` for ifIndex 42 on 10.0.0.1") so that the topology plugin can react instantly without polling — the trap plugin would then be a netipc *server* for that small service.

So netipc is a **private, in-host, cross-plugin RPC fabric**. It is a small client/server ecosystem between plugins, not visible to the outside world, used so plugins can expose data and lookups to each other without going through the agent. The cross-language interop (C/Rust/Go bindings of the same wire protocol) exists precisely because the plugin ecosystem is multi-language and any plugin in any language must be able to consume/expose any service.

**Important state of the fabric**: Today netipc ships exactly **one** public typed service — `cgroups-snapshot`. The library itself is mature (cross-language interop, 90%+ coverage, full benchmark matrix on POSIX and Windows), but the **service catalogue is small**. Anyone adding a new cross-plugin lookup must add a new typed service in the upstream library first (in C, Rust, and Go), then connect server and client(s).

Upstream repository: `netdata/plugin-ipc` @ `e06a5a07bf2d` (authoritative specs live in `docs/` inside that repo). Netdata working tree: `netdata/netdata @ 6a515000ac` (branch `snmptraps`).

---

## 1. Library Overview

- **What it is**: a localhost-only typed-RPC stack with a 32-byte outer envelope, three orthogonal API levels (L1 transport, L2 typed RPC, L3 snapshot cache), and one parallel Codec layer. Used for cross-language IPC between Netdata plugins and helper services.
- **Canonical source**: `github.com/netdata/plugin-ipc` (the library evolves there; the Netdata repo carries a vendored copy synced by `vendor-to-netdata.sh` from the upstream).
- **Languages, all first-class**: C (POSIX + Windows/MSYS), Rust (POSIX + native Windows), Go (POSIX + native Windows, **pure Go, no cgo**). Wire interop across all three is mandatory and tested.
- **License/ownership**: source files in `plugin-ipc` itself carry no SPDX headers; the Netdata-side wrapper (`src/libnetdata/netipc/netipc_netdata.c:1`) is `SPDX-License-Identifier: GPL-3.0-or-later`. Owned and authored under the Netdata umbrella; not a third-party dependency.
- **Maturity (per upstream `README.md`)**: Linux `ctest 37/37` passing, C coverage 94.1%, Go 95.8%, Rust 98.57%. Windows `ctest 28/28`, C 93.2%, Go 95.4%, Rust 92.08%. Benchmarks generated 2026-03-25 against `costa-desktop` (POSIX, 201 matrix rows) and `win11` (Windows, 201 matrix rows). Checked-in `benchmarks-posix.md` and `benchmarks-windows.md`.
- **Performance ceiling (POSIX, current bench artifact)**: UDS baseline ping-pong 183k–231k req/s; SHM ping-pong 2.46M–3.45M req/s; UDS batch ping-pong up to 40M items/s; SHM pipeline+batch up to 102M items/s.
- **Honesty point from upstream README** (`README.md:292-298`): "core build, transport, service, interop, and benchmark validation is strong on both Linux and Windows", but "Linux still has broader chaos / hardening / stress breadth than Windows". Functional parity yes; full validation parity no.

---

## 2. Vendored Copy in the Netdata Tree

| Concern | Reality |
|---|---|
| Mechanism | **Plain vendored copy**, not a submodule. `git submodule status` shows only `src/aclk/aclk-schemas` and `src/collectors/debugfs.plugin/libsensors/vendored` — no netipc submodule. |
| Sync script | `netdata/plugin-ipc @ e06a5a07bf2d :: vendor-to-netdata.sh [netdata-repo-root]` copies C, Rust, Go sources from upstream into the matching paths inside a Netdata checkout. Does **not** touch the Netdata-specific wrapper or build files (`vendor-to-netdata.sh:1-15`). |
| Drift check | `netdata/plugin-ipc @ e06a5a07bf2d :: diff-netdata-vendor.sh` diffs the in-tree copy against the upstream tree. |
| Pinned to commit | No machine-enforced pin — current Netdata-tree files match upstream as of the last manual run. Most recent Netdata-side commit touching the vendored tree: `79d43657a3 feat: replace cgroups-ebpf SHM transport with netipc IPC (#22221)`. |
| C library layout | `src/libnetdata/netipc/include/netipc/*.h` (public headers), `src/libnetdata/netipc/src/{protocol,transport,service}/...` (impl). Built as a static library `netipc` linked into `libnetdata` (`CMakeLists.txt:2352-2408`). |
| Netdata-specific glue | `src/libnetdata/netipc/netipc_netdata.{c,h}` (lives in the Netdata repo, not in upstream) — provides `netipc_auth_token()` (xxh3 of `NETDATA_INVOCATION_ID`). |
| Rust crate | `src/crates/netipc/` registered as a workspace member in `src/crates/Cargo.toml:32` (`"netipc"`). Crate name `netipc`, version `0.1.0`, deps `libc 0.2`, dev `proptest 1`. |
| Go package | `src/go/pkg/netipc/{protocol,transport/{posix,windows},service/{raw,cgroups}}`. Import path: `github.com/netdata/netdata/go/plugins/pkg/netipc/...` (per `src/go/go.mod:1` and use sites in `src/go/pkg/netipc/transport/posix/uds.go:23`). |
| Build platforms | POSIX (Linux is the only POSIX variant compiled) and Windows. The CMake netipc target is only added when `OS_LINUX OR OS_WINDOWS` (`CMakeLists.txt:2355, 2367, 2406`). FreeBSD and macOS have no netipc target today. |

The upstream and the Netdata-vendored copies are intentionally identical inside `src/libnetdata/netipc/src/`, `src/crates/netipc/src/`, and `src/go/pkg/netipc/`. The Netdata wrapper is **only** `netipc_netdata.{c,h}` and CMake glue.

---

## 3. Architecture

### 3.1 Layer model

From `netdata/plugin-ipc @ e06a5a07bf2d :: docs/README.md:1-37`:

```
+----------------------------------------------+
|              Level 3: Snapshot                |
|     Refresh, caching, lookup helpers          |
|             (built on Level 2)                |
+----------------------------------------------+
|           Level 2: Orchestration              |
|   Client context, managed server, retry       |
|          (built on Level 1 + Codec)           |
+----------------------+-----------------------+
|    Level 1:          |       Codec:          |
|    Transport         |   Wire <-> Typed       |
|                      |                       |
|  Connections, send,  |  Encode, decode,      |
|  receive, framing,   |  builders, views.     |
|  chunking, batching, |  Pure bytes. No I/O.  |
|  pipelining, auth,   |  No transport.        |
|  sequencing, SHM.    |                       |
+----------------------+-----------------------+
```

L1 and Codec are independent; L2 composes them; L3 builds on L2.

### 3.2 Process model

- **Client–server, not peer-to-peer**. A "service" has one listener (provider plugin) and N clients (consumer plugins).
- **Service-oriented identity**: clients connect to `service_namespace + service_name` (e.g. `/run/netdata` + `cgroups-snapshot`), not to a plugin/process identity (`level1-transport.md:156-172`).
- **Side-car style by virtue of how it is integrated**: the C library is statically linked into `netdata` itself and into external plugins; the Rust crate and Go package are linked into their respective plugins. There is no central broker process. Each service is one listener inside its owning plugin.
- **Concurrency**: the managed server runs `worker_count` concurrent sessions (`netipc_service.h:266-273`). Per-session goroutine in Go (`netipc-integrator-skill.md:249-250`).
- **No hidden threads on the client/cache side**: refresh is caller-driven (`netipc_service.h:166-167`, `netipc-integrator-skill.md:235-241`).

### 3.3 Transports (baseline + negotiated fast path)

From `level1-transport.md:450-496`:

| Platform | Baseline (always available) | Fast path (negotiated) | SHM in-flight depth |
|---|---|---|---|
| POSIX / Linux | UDS `SOCK_SEQPACKET` (message-oriented, packet-limited) | POSIX SHM with futex/hybrid synchronization | **1 in-flight per direction** |
| Windows | Named Pipe (message mode) | Win SHM with `WaitOnAddress` / named events | 1 in-flight per direction |
| FreeBSD / macOS | UDS only — no SHM backend | — | — |

Profile selection: client advertises a bitmask of supported profiles in `HELLO`; server picks the highest mutually supported profile and locks it for the session in `HELLO_ACK`. There is **no same-session fallback** (`level1-transport.md:230-247`). If SHM attach fails on the client side, the client must close the session and reconnect on baseline (`level1-transport.md:289-306`).

### 3.4 Framing and ordering

- **Outer header is a fixed 32 bytes** (see §4). Single messages put the payload immediately after; batches insert an item-directory + packed items.
- **Pipelining on baseline**: full pipelining. Multiple in-flight `message_id`s, responses may arrive out of order (`level1-transport.md:76-86`).
- **Pipelining on SHM**: NOT supported in the current layout — one in-flight publication per direction (`level1-transport.md:486-492`, `level1-posix-shm.md:105,183`). The way to "pipeline" over SHM today is to batch many items into one outer message.
- **No chunk interleaving**: a chunked logical message owns the wire from first to last byte; a different message cannot interleave (`level1-transport.md:120-132`).
- **Transparent chunking**: messages larger than the negotiated packet size are split by the sender, reassembled by the receiver in one go (`level1-transport.md:369-392`, `netipc_protocol.h:122-134`).

### 3.5 Backpressure and error semantics

- **No automatic retry inside L1** (`level1-transport.md:147-154`). A send fails, it fails. A connection drop fails all in-flight `message_id`s for that session. L2 adds at-least-once retry on top.
- **No protocol-level backpressure window** beyond the one-in-flight SHM limit and the negotiated payload/batch ceilings.
- **Connection failure is total for the session**: no partial recovery (`level1-transport.md:135-145`).
- **Protocol violations terminate the session immediately** — the server does not try to "skip" a malformed message (`level1-transport.md:408-422`).
- **Negotiated hard cap on request payload**: 1 MiB (`netipc_protocol.h:71`).

### 3.6 Native wait-object exposure

L1 exposes the underlying transport descriptor (POSIX `fd`, Windows `HANDLE`) so callers can plug sessions and listeners into their own event loops (`level1-transport.md:434-449`). This matters for a trap listener that already has an `epoll`/`select` loop for UDP/162.

---

## 4. Wire Format / Schema

All multi-byte fields use **host byte order** — this is localhost-only IPC, never cross-host. Source: `netdata/plugin-ipc @ e06a5a07bf2d :: docs/level1-wire-envelope.md`.

### 4.1 Outer message header (32 bytes, every message)

| Offset | Size | Type | Field | Notes |
|---|---|---|---|---|
| 0 | 4 | u32 | `magic` | `0x4e495043` ("NIPC") |
| 4 | 2 | u16 | `version` | Must be `1` |
| 6 | 2 | u16 | `header_len` | Must be `32` |
| 8 | 2 | u16 | `kind` | 1=REQUEST, 2=RESPONSE, 3=CONTROL |
| 10 | 2 | u16 | `flags` | bit 0 = `BATCH` (`0x0001`) |
| 12 | 2 | u16 | `code` | method id or control opcode |
| 14 | 2 | u16 | `transport_status` | envelope-level only, never business outcome |
| 16 | 4 | u32 | `payload_len` | bytes after the header |
| 20 | 4 | u32 | `item_count` | 1 for single, N for batch |
| 24 | 8 | u64 | `message_id` | client-assigned correlation id |

Compile-time enforced. Source: `netipc_protocol.h:101-112`, `level1-wire-envelope.md:12-29`.

`transport_status` enum (`netipc_protocol.h:43-49`): `OK`, `BAD_ENVELOPE`, `AUTH_FAILED`, `INCOMPATIBLE`, `UNSUPPORTED`, `LIMIT_EXCEEDED`, `INTERNAL_ERROR`. These are **envelope-level** outcomes; per-item or business errors live inside the payload (codec-defined).

### 4.2 Method codes today (one global registry)

From `netipc_protocol.h:56-58`:

| Code | Name | Status |
|---|---|---|
| 1 | `INCREMENT` | test/benchmark only (8-byte u64 ping-pong) |
| 2 | `CGROUPS_SNAPSHOT` | production, only public typed service |
| 3 | `STRING_REVERSE` | test/benchmark only (variable-length string ping-pong) |

The method-code space is **shared across all services** — every method has a globally unique code (`level1-wire-envelope.md:79-81`). A new typed service (e.g. trap delivery) gets a new code allocated centrally in upstream `plugin-ipc`.

### 4.3 Batch directory

When `flags & BATCH` is set and `item_count > 1`:

- Payload starts with `item_count` 8-byte directory entries (`offset`, `length`) at 8-byte alignment.
- Then packed item payloads, each 8-byte aligned.
- **Batches are homogeneous**: the outer header's `code` applies to **all items** (`level1-transport.md:104-110`, `level1-wire-envelope.md:84-92`). A mixed-method batch is **not representable** on the wire. If a service wants to multiplex sub-types, it does so inside its own payload format, not in the outer envelope.

### 4.4 Chunk continuation header (32 bytes, for messages > negotiated packet size)

Source: `netipc_protocol.h:125-134`, `level1-wire-envelope.md:123-156`. Magic `0x4e43484b` ("NCHK"), carries `message_id`, `total_message_len`, `chunk_index`, `chunk_count`, `chunk_payload_len`.

### 4.5 Handshake (`HELLO` 44 bytes, `HELLO_ACK` 48 bytes)

`HELLO` carries the client's proposed `supported_profiles`, `preferred_profiles`, request/response payload and batch-item ceilings, an opaque `u64 auth_token`, and the client's `packet_size`. `HELLO_ACK` returns the server's final decisions plus a `session_id`. Auth is exact-match on the `u64 auth_token` value — **the library never reads tokens from env or files** (`level1-transport.md:198-208`). The Netdata wrapper derives this token from `XXH3_64bits(NETDATA_INVOCATION_ID)` (`netipc_netdata.c:9-12`).

### 4.6 Encoding philosophy

- **Pure byte manipulation, zero-allocation decode** (`codec.md:38-58`). Decoded "View" types borrow the underlying buffer.
- **Per-payload internal structure** is: fixed scalar header + `offset+length` pairs for variable fields + a packed variable-data area. Strings always end with `NUL`; decoders validate the NUL (`codec.md:99-114`).
- **Self-contained payloads**: each payload can be decoded in isolation given only its byte range.

There is no protobuf, no flatbuffers, no JSON, no MessagePack — it is a hand-rolled binary contract with three identical implementations (C, Rust, Go) cross-tested as the source of truth (`codec.md:200-228`).

---

## 5. Language Bindings

### 5.1 C (public)

Header bundle: `src/libnetdata/netipc/include/netipc/netipc_service.h` (`netipc_service.h:1-547`).

```c
// Provider side (real example: src/collectors/cgroups.plugin/cgroup-netipc.c:129-169)
static nipc_managed_server_t srv;
uint64_t auth = netipc_auth_token();
nipc_server_config_t cfg = {
    .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
    .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
    .auth_token         = auth,
};
nipc_cgroups_service_handler_t handler = { .handle = my_handler, .user = NULL };
nipc_server_init_typed(&srv, os_run_dir(true), "cgroups-snapshot", &cfg, 2, &handler);
nd_thread_create("P[cgroupsipc]", 0, (void(*)(void*))nipc_server_run, &srv);

// Consumer side (real example: src/collectors/ebpf.plugin/ebpf_cgroup.c:283-303)
nipc_cgroups_cache_t cache;
nipc_client_config_t ccfg = { .supported_profiles = ..., .auth_token = netipc_auth_token() };
nipc_cgroups_cache_init(&cache, os_run_dir(false), "cgroups-snapshot", &ccfg);
// in loop:
if (nipc_cgroups_cache_refresh(&cache)) { for (uint32_t i=0; i<cache.item_count; i++) ... }
```

Critical pairing rule from `netipc-integrator-skill.md:217-228`: use `os_run_dir(true)` on the provider, `os_run_dir(false)` on the consumer, and the same `netipc_auth_token()` on both. Mismatch → `AUTH_FAILED` or `NOT_FOUND` forever.

### 5.2 Rust

Crate path: `src/crates/netipc/` (workspace member; see `src/crates/Cargo.toml:32`). Public module: `netipc::service::cgroups` (`src/crates/netipc/src/service/cgroups.rs:101-247`).

```rust
use netipc::service::cgroups::{CgroupsClient, ClientConfig, CgroupsCache};
use netipc::protocol::{PROFILE_BASELINE, PROFILE_SHM_FUTEX};

let cfg = ClientConfig {
    supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
    preferred_profiles: PROFILE_SHM_FUTEX,
    max_request_batch_items: 0,
    max_response_payload_bytes: 0,
    auth_token: my_auth,
};
let mut cache = CgroupsCache::new("/run/netdata", "cgroups-snapshot", cfg);
loop {
    if cache.refresh() { /* items rebuilt */ }
    if let Some(item) = cache.lookup(hash, name) { /* use item */ }
}
```

The Rust crate is wired only with the **`cgroups-snapshot` typed facade**. For a brand new service kind, both raw (`src/crates/netipc/src/service/raw.rs:1411-1712`) and typed (`cgroups.rs`) machinery must be added upstream. No NetFlow / topology / log consumer of the netipc crate exists today — confirmed: `grep -rn 'netipc' src/crates/netflow-plugin/` returns no hits.

### 5.3 Go

Import path: `github.com/netdata/netdata/go/plugins/pkg/netipc/{protocol,service/raw,service/cgroups,transport/posix,transport/windows}` (`src/go/go.mod:1`, `src/go/pkg/netipc/service/raw/client.go:14`).

```go
import (
    "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
    "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
    "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

cfg := posix.ClientConfig{
    SupportedProfiles: protocol.ProfileBaseline | protocol.ProfileShmFutex,
    PreferredProfiles: protocol.ProfileShmFutex,
    AuthToken:         myAuth,
}
client := raw.NewSnapshotClient("/run/netdata", "cgroups-snapshot", cfg)
defer client.Close()
for {
    client.Refresh()
    if !client.Ready() { time.Sleep(1*time.Second); continue }
    view, err := client.CallSnapshot()
    // ...
}
```

Public Go API surface (`src/go/pkg/netipc/service/raw/client.go:62-74`): `NewSnapshotClient`, `NewIncrementClient`, `NewStringReverseClient`. The typed cgroups facade lives at `src/go/pkg/netipc/service/cgroups/`. **Pure Go, no cgo**: confirmed in `netdata/plugin-ipc @ e06a5a07bf2d :: README.md:88`.

### 5.4 Cross-binding interoperability

From `netdata/plugin-ipc @ e06a5a07bf2d :: README.md:175-189`: the test suite validates C client → C/Rust/Go server, Rust client → C/Rust/Go server, Go client → C/Rust/Go server, on both baseline and SHM transports, for both POSIX and Windows. The wire envelope is the contract; an implementation that produces different bytes for the same logical message is wrong.

---

## 6. Plugin Lifecycle

### 6.1 Service discovery

- Clients resolve a service by name in a known run-dir (`os_run_dir()` on Linux is typically `/run/netdata`; on Windows it derives from a named-pipe namespace, see `level1-windows-np.md`).
- The library handles **stale endpoints**: if a socket file exists from a dead process, `nipc_server_run` reclaims it; if a live process owns it, listen fails with `address already in use` (`level1-transport.md:526-535`).
- There is no registry, broker, or service-discovery protocol. The service name is the entire contract.

### 6.2 Handshake

Single `HELLO` / `HELLO_ACK` exchange on the baseline transport. Negotiates: auth, profile, max request payload, max batch items (request and response), max response payload, packet size (`level1-transport.md:195-225`). After `HELLO_ACK = OK`, the data-plane may upgrade to SHM if that was the selected profile. Limits are **final and locked** for the session.

Rejection classes (`level1-wire-envelope.md:251-263`):
- `AUTH_FAILED` — wrong token
- `UNSUPPORTED` — no mutually supported profile
- `INCOMPATIBLE` — bad `layout_version` or negotiated packet ≤32 bytes
- `LIMIT_EXCEEDED` — proposed payload > 1 MiB
- `BAD_ENVELOPE` — malformed reserved/padding

### 6.3 State machine (L2 client)

From `netipc_service.h:49-57`: `DISCONNECTED` → `CONNECTING` → `READY`, with fault states `NOT_FOUND`, `AUTH_FAILED`, `INCOMPATIBLE`, `BROKEN`.

Per `netipc-integrator-skill.md:442-462`:
- `NOT_FOUND` and `BROKEN` are routine — keep retrying.
- `AUTH_FAILED` and `INCOMPATIBLE` are misconfiguration — stop retrying.

### 6.4 Retry semantics

- L2 typed calls are **at-least-once** (`netipc-integrator-skill.md:464-479`). Handlers must be duplicate-safe.
- A previously-`READY` client that hits a call failure does **one** reconnect + retry; overflow-driven resize may reconnect several times while capacities grow.
- L3 cache preserves the previous successful snapshot across refresh failures (`netipc_service.h:498-507`, `level3-snapshot-api.md`).

### 6.5 Shutdown

- `nipc_server_stop` flips a flag; `nipc_server_drain(timeout_ms)` waits for in-flight sessions then closes; `nipc_server_destroy` releases all resources (`netipc_service.h:384-412`).
- Closing a session does not affect other sessions or the listener.

### 6.6 Identity / authentication

There is no PKI, no per-user identity, no service principal. The entire authentication model is **a single shared `u64` token** between provider and consumer. In Netdata's case both sides derive it from the same `NETDATA_INVOCATION_ID` (`netipc_netdata.c:9-12`). This is sufficient for a localhost trust boundary where everything is owned by the same agent — it is **not** sufficient if a separate-trust component needed to talk to the agent (an SNMP trap forwarder running as a different user, for instance, would need a different design).

---

## 7. Real Consumers in the Netdata Tree

Search results today (`grep -rln 'netipc' src/ --include='*.c' --include='*.h' --include='*.go' --include='*.rs'`):

| Component | Role | Source | Status |
|---|---|---|---|
| `cgroups.plugin` | **Server** for `cgroups-snapshot` | `src/collectors/cgroups.plugin/cgroup-netipc.c:1-186` | Production. Linux only (`#ifdef OS_LINUX`). Replaces a legacy ad-hoc SHM channel. |
| `ebpf.plugin` | **L3 cache client** of `cgroups-snapshot` | `src/collectors/ebpf.plugin/ebpf_cgroup.c:283-358` | Production. Linux only. |
| `libnetdata` | links the netipc static lib | `CMakeLists.txt:2363-2407` | Compiled-in unconditionally on Linux and Windows. |

**There are no Rust or Go consumers of netipc in the Netdata tree today.** Specifically:
- `src/crates/netflow-plugin/` — `grep -rn 'netipc' src/crates/netflow-plugin/` returns no hits. NetFlow runs as an external Rust plugin and speaks the **stdio plugin protocol** to `netdata`, not netipc.
- `src/go/plugin/...` — no consumer of `pkg/netipc` outside the package itself.

So at the time of writing, netipc is **one C provider talking to one C consumer**. The Rust and Go bindings are kept synchronized for the protocol/codec/transport layers, fully tested and benchmarked, but no production Rust/Go integration exists yet. The vendored Rust crate and Go package exist precisely so that future cross-language integrations (such as a Rust trap listener talking to a Go enrichment service, or vice-versa) can be added without re-vendoring.

---

## 8. Performance Properties

From `netdata/plugin-ipc @ e06a5a07bf2d :: README.md:191-246` (checked-in benchmark artifacts, 2026-03-25):

### 8.1 POSIX (Linux, `costa-desktop`)

| Pattern | Baseline UDS | SHM |
|---|---|---|
| Single ping-pong | 183k–231k req/s | 2.46M–3.45M req/s |
| Batch ping-pong | 27M–40M items/s | 31M–64M items/s |
| Pipeline (depth=16) | 568k–735k req/s | n/a (1 in-flight) |
| Pipeline + batch (depth=16) | 51M–102M items/s | n/a |
| Snapshot refresh (L3) | 159k–205k req/s | 1.01M–1.74M req/s |
| Local cache lookup (L3) | C 172M req/s, Go 114M, Rust 203M | (same) |

### 8.2 Windows (`win11`)

| Pattern | Named Pipe | Win SHM |
|---|---|---|
| Single ping-pong | 18k–21k req/s | 2.10M–2.72M req/s |
| Batch ping-pong | 7M–8.5M items/s | 36M–58M items/s |
| Pipeline + batch (depth=16) | 28M–41M items/s | n/a |
| Snapshot refresh (L3) | 16k–21k req/s | 857k–1.26M req/s |

### 8.3 Properties

- **Throughput ceiling**: well above what any SNMP-trap workload could plausibly produce. Even pathological storm scenarios (foundational spec §6.6 talks of "10k+ traps in seconds") are 3–4 orders of magnitude below the SHM ceiling.
- **Max single-message payload**: 1 MiB negotiated cap (`netipc_protocol.h:71`). Larger than any single trap PDU could ever be.
- **Async / zero-copy**: decode is zero-copy (View types borrow the payload buffer, `codec.md:51-79`). The library is not async itself — calls are blocking — but it exposes native `fd`/`HANDLE` so the **caller's** event loop can be async.
- **Backpressure**: per-direction one-in-flight on SHM (the natural backpressure is "wait for the slot to free"). On baseline UDS, full pipelining; backpressure becomes the OS's socket buffer.

---

## 9. Tests, Fixtures, CI

From `netdata/plugin-ipc @ e06a5a07bf2d :: tests/`:

| Asset | What it does |
|---|---|
| `run-coverage-{c,go,rust}.sh` | per-language coverage; enforced 90%+ minimums |
| `run-coverage-{c,go,rust}-windows.sh` | Windows variants |
| `fuzz_protocol.c`, `run-extended-fuzz.sh` | fuzz the codec |
| `run-sanitizer-asan.sh`, `run-sanitizer-tsan.sh`, `run-valgrind.sh` | sanitizers and leak/race detection |
| `run-go-race.sh` | Go race detector for transport tests |
| `run-posix-bench.sh`, `generate-benchmarks-{posix,windows}.sh` | bench matrix generators that **fail closed** on incomplete matrices |
| `run-verifier-windows.sh` | Windows native sign-off harness |
| `interop_codec.sh` + `src/crates/netipc/src/bin/interop_codec.rs` | cross-language codec interop |

Interop tests confirm bytes-identical encoding across C/Rust/Go for every method type. The benchmark generators reject incomplete matrices — if any row in the 3x3 language pair × transport × pattern matrix is missing, the report fails.

CI workflows live in the upstream repo, not in the Netdata tree (the Netdata repo only builds the static lib and runs Netdata's own tests).

---

## 10. Reusable Pieces for an SNMP Trap Listener

To restate the boundary (see top-of-file overview): netipc is **not** the path the trap listener will use to push trap records to the Netdata Agent — that path is stdio pipes (PLUGINSD), just like NetFlow. netipc is the path for **plugin-to-plugin enrichment lookups and small published states**. The reusable pieces below are framed around that.

### 10.1 Trap listener as a netipc CLIENT (enrichment lookups)

A trap listener arriving at a varbind-decoded PDU needs to answer questions whose authoritative answers live in other plugins. Reusable netipc machinery:

1. **L1 transport with native `fd`/`HANDLE` exposure**. Already 3+ orders of magnitude faster than needed for sub-millisecond enrichment lookups per trap. The trap listener's existing `epoll` / `tokio` loop can include the netipc session FD directly instead of spawning a worker thread (`level1-transport.md`).
2. **Handshake / lifecycle / reconnect machinery**. Stale-endpoint reclamation, at-least-once retry on L2, fail-closed `BROKEN`/`READY` state model (`level1-transport.md:526-535`) — exactly the guarantees a trap-enrichment client wants when the upstream service plugin restarts.
3. **L3 snapshot pattern for stable lookups**. `snmp.plugin` could expose, via netipc snapshot, the maps:
   - `sourceIP → vnode-id, profile, sysObjectID, sysDescr`
   - `(node, ifIndex) → ifName, ifAlias, ifOperStatus`
   - `sysObjectID prefix → vendor / device family`
   The snapshot model (refresh-driven, caller-controlled, no hidden threads) is the right fit: the listener takes a snapshot once per N traps or per timer and reads against the local cached copy without an RPC per varbind.
4. **Codec primitives for variable-length payloads** (offset+length pairs, NUL-terminated strings, packed area with 8-byte alignment). When the listener needs to publish back a small structured reply (e.g. "the topology client wants the canonical decoded form of this PDU"), the codec primitives at `codec.md:99-114` and the precedent at `codec-cgroups-snapshot.md:120-148` show how heterogeneous content (analogous to varbinds) is encoded.
5. **Cross-language interop**. The trap listener is likely Rust (per the foundational spec's recommendation); `snmp.plugin` is Go (today). The Rust netipc crate consumes the Go-served service over the same wire protocol with no marshalling layer beyond the codec.
6. **The cgroups-snapshot precedent as a template** (`cgroup-netipc.c:36-122` server side, `ebpf_cgroup.c:283-358` client side) — concrete reading material for whoever adds a new typed service for SNMP enrichment.

### 10.2 Trap listener as a netipc SERVER (publishing trap state to other plugins)

A trap listener has useful state for other plugins. Reusable patterns:

7. **Publish a "recent traps" snapshot** so that, for example, a future topology-aware plugin can read "all `linkDown`/`linkUp` traps in the last 60 seconds" and react without polling the trap listener directly. The L3 snapshot model fits because the consumer pulls on demand.
8. **Batch primitives** (`nipc_batch_builder_init`/`add`/`finish`, `netipc_protocol.h:194-221`) for the published service: many trap records ship as one batched response per consumer poll.
9. **Builder/view ephemerality model** — decoded views borrow the buffer; consumers must copy or process eagerly. This is the right discipline for a recent-events ring buffer (consumers process or copy out within the snapshot window).
10. **Auth via shared `NETDATA_INVOCATION_ID`** (`netipc_netdata.c:9-12`). For a trap listener launched as a child of `netdata`, inheritance gives the shared secret for free. For a binary launched separately by the operator (e.g. with `CAP_NET_BIND_SERVICE`), the listener must read the agent's invocation id from somewhere — see §11 caution 8.

---

## 11. NOT Reusable / Cautions

**Reframe first**: most of the original cautions assumed netipc would carry trap records to the agent. Per the corrected overview at the top, that is the **stdio plugin protocol's job**, not netipc's. The cautions below are for the actual netipc uses — enrichment client and small published-state server.

1. **One endpoint = one method**. Public L2/L3 is not a multi-method RPC router (`netipc-integrator-skill.md:33-35,281-283`). Each new lookup the trap listener needs (e.g. `IP→profile`, `(node,ifIndex)→name`, `sysObjectID→vendor`) is a separate service kind, each requiring upstream library work in C + Rust + Go before it can be used in the Netdata tree (`netipc-integrator-skill.md:692-702`). Plan the enrichment surface carefully so we add as few new service kinds as possible — prefer one fat snapshot per provider plugin over many small ones.
2. **Service model is request/response (or refresh-snapshot), not push-stream**. There is no native "server pushes events to client" pattern. If we publish trap state via netipc, **consumers must poll**. That is fine for periodic topology re-reads or batched event consumption, but bad for sub-second event-driven reactions. For low-latency reactivity, the better pattern is to keep the data on the stdio path into the agent and let the agent's existing event mechanisms (functions, alerts) drive other plugins.
3. **No retention / persistence layer**. netipc is wire only — no journal, no replay, no durable buffer. If a consumer plugin restarts and reads the trap-listener's snapshot, anything that aged out of the listener's in-memory ring is gone. Persistence has to live in the trap listener (or in the agent's storage path), not in netipc.
4. **Schemas are codec-defined, not self-describing**. No TLV, no schema registry. SNMP varbinds are heterogeneous — adding a new varbind type to a published service requires a coordinated upstream library change across three languages. Keep published schemas as narrow as possible (only the fields other plugins genuinely need, not the full PDU).
5. **One-in-flight SHM constraint**. SHM throughput requires batched responses. A "ask for the IP→profile mapping" snapshot call is one big response; that fits well. A "give me each trap as it arrives" pattern would lose the SHM advantage; that's the wrong shape for netipc anyway (see caution 2).
6. **Localhost only — no cross-host transport**. Host byte order on the wire, UDS or Win named pipe (`level1-wire-envelope.md:10`). All netipc participants must be on the same host. This is fine for plugin-to-plugin lookups; it is unrelated to northbound trap forwarding (which lives in the trap listener itself, not netipc).
7. **No FreeBSD / macOS**. Today the Netdata-tree CMake and the vendored library do not compile netipc on FreeBSD/macOS (`CMakeLists.txt:2355-2378`). If the trap listener runs on those platforms, it must work without netipc-based enrichment there. The agent runs on those; the trap subsystem cannot use netipc as a hard dependency on those platforms.
8. **Auth is shared-secret (`NETDATA_INVOCATION_ID`), agent-local**. A trap listener that needs `CAP_NET_BIND_SERVICE` for port 162 either (a) is launched as a child of `netdata` (inherits the invocation id, but the agent's lifecycle now drives privileged port binding) or (b) is launched separately with elevated caps and reads the invocation id from a known path. (b) is operationally easier but adds a deployment step. (a) is currently the only blessed path.
9. **Builder error semantics are sticky / overflow-bounded**. If a snapshot builder fills the response buffer, `NIPC_ERR_OVERFLOW` is returned and the cgroups-style handler **truncates and continues** (`cgroup-netipc.c:82-91`). For trap *enrichment* snapshots this is acceptable; for a "recent traps" published service, it means consumers may see a truncated window during storms — the consumer logic must handle "last entry may be partial / earlier entries may be missing".
10. **Public L2/L3 hides the negotiated profile**. Operators cannot tell from L2 whether the SHM fast path is active or whether it fell back to UDS. The cgroups-ebpf integration logs this from the **integration side** (`ebpf_cgroup.c:74-79`). If trap-enrichment SLAs ever depend on the SHM path, the listener must surface that itself.

---

## 12. Open Questions For Trap Design

(Decisions for Costa, surfaced by this inventory; not decided here.)

1. **Data path to the agent**: confirm stdio pipes (PLUGINSD-style, as NetFlow does) is the path for the trap listener to push records to `netdata`. netipc is not on this path. The only real question here is the framing (Rust crate `rt::PluginRuntime` line protocol, or something else).

2. **Cross-plugin enrichment surface — netipc CLIENT role**: which existing or new netipc services does the trap listener need to call?
   - From `snmp.plugin`: `sourceIP → vnode-id, profile, sysObjectID`. ✓ obvious need.
   - From `snmp.plugin` or `snmp_topology.plugin`: `(node, ifIndex) → ifName, ifAlias`. ✓ obvious need for `linkDown`/`linkUp` decode.
   - From `snmp_topology`: peer-interface lookup for trap-driven downstream-suppression decisions? Open.
   - Implication: `snmp.plugin` (Go) and possibly `snmp_topology.plugin` (Go) would need to expose new typed netipc services — work in the upstream `plugin-ipc` library in all three languages first.

3. **Cross-plugin publish surface — netipc SERVER role**: should the trap listener publish anything back via netipc? Concretely useful candidates:
   - "Recent traps in the last N seconds, filterable by source / trapOID" — lets the topology plugin react to fresh state changes without polling the listener.
   - "Cumulative trap counters per device" — lets a dashboard plugin render trap-rate alongside polled metrics.
   - **Costa's example**: a Rust trap listener publishing live trap data that a Go-based topology plugin can query with small overhead — practical because the C/Rust/Go bindings are wire-compatible, and SHM batched response can carry hundreds of records per poll without a per-trap roundtrip. Not a recommendation, but a real practical possibility we should evaluate against the alternatives.

4. **Cross-language interop, concrete**: the trap listener is best-fit in Rust (per the foundational spec and per the NetFlow precedent). Consumers across snmp / snmp_topology are Go. The cross-language netipc bindings are explicitly designed for this — a Rust server + Go client(s) is a supported, tested topology (full benchmark matrix). Nothing in the language pairing argues against it.

5. **Listener deployment shape**: in-tree (`src/crates/<trap-crate>/`) or out-of-tree (separate package)? In-tree gives child-of-netdata lifecycle and free `NETDATA_INVOCATION_ID` inheritance for netipc auth. Out-of-tree means working out a path for sharing the invocation id.

6. **CAP_NET_BIND_SERVICE for port 162**: where does it get granted, and how does it interact with the listener-being-a-child-of-netdata model? This is the same question NetFlow already answered for its own use of low ports; check how NetFlow does it before re-solving.

7. **Persistence boundary**: where does the durable buffer for traps live? In the listener's own ring (lost on restart unless mmap-backed)? In the agent's journal-engine (NetFlow's choice)? In TSDB as counter metrics with annotation labels? Each gives a different "what survives a crash" answer. netipc itself does not persist anything.

8. **Storm handling**: dedup window in the listener (before any publication anywhere) per the foundational spec §6.6. netipc gives framing but no dedup primitives — so dedup logic lives in the listener, **before** publication on either the stdio path or any netipc service.

9. **Failure mode visibility on netipc paths**: today netipc public L2/L3 does not expose negotiated profile, SHM-vs-baseline, or in-flight depth. If trap-driven topology reactions ever depend on the SHM path, the listener must surface those facts itself for operator debugging.

10. **Migration path**: if we start with no netipc participation (pipes-only data path, no enrichment), can we add netipc enrichment later without breaking on-the-wire compatibility for the stdio data? Yes — they are independent paths. So the question is really "phase 1: pipes-only, phase 2: add netipc enrichment when the costs justify."

---

## 13. Evidence Trail

- Upstream repo state: `netdata/plugin-ipc` @ `e06a5a07bf2d` (remote `github.com/netdata/plugin-ipc`)
- Upstream README sections cited: `README.md:1-13, 79-89, 175-189, 191-246, 257-298, 292-298, 386-407`
- Upstream specs: `docs/README.md:1-122`, `docs/level1-transport.md`, `docs/level1-wire-envelope.md`, `docs/codec.md`, `docs/codec-cgroups-snapshot.md`, `docs/netipc-integrator-skill.md`
- Netdata vendoring mechanism: `netdata/plugin-ipc @ e06a5a07bf2d :: vendor-to-netdata.sh:1-15`, `netdata/plugin-ipc @ e06a5a07bf2d :: diff-netdata-vendor.sh`
- Netdata-tree files (no submodule, plain vendored): `src/libnetdata/netipc/`, `src/crates/netipc/`, `src/go/pkg/netipc/`, `src/libnetdata/netipc/netipc_netdata.{c,h}`
- Build wiring: `CMakeLists.txt:2352-2408` (static lib `netipc`, OS-gated), `CMakeLists.txt:1256-1257` (Netdata wrapper as part of `libnetdata`), `CMakeLists.txt:2045-2046` (cgroups consumer files), `src/crates/Cargo.toml:32` (Rust workspace member), `src/go/go.mod:1` (Go module root)
- Netdata commit that introduced the cgroups-ebpf path: `79d43657a3 feat: replace cgroups-ebpf SHM transport with netipc IPC (#22221)`
- Real producer: `src/collectors/cgroups.plugin/cgroup-netipc.c:1-186` (Linux only via `#ifdef OS_LINUX`)
- Real consumer: `src/collectors/ebpf.plugin/ebpf_cgroup.c:16-358`
- Netdata-side auth derivation: `src/libnetdata/netipc/netipc_netdata.c:9-12` (`XXH3_64bits(NETDATA_INVOCATION_ID)`)
- Public C API: `src/libnetdata/netipc/include/netipc/netipc_service.h:49-547`, `src/libnetdata/netipc/include/netipc/netipc_protocol.h:25-459`
- Public Rust API: `src/crates/netipc/src/service/cgroups.rs:30-247`, `src/crates/netipc/src/service/raw.rs:1411-1712`
- Public Go API: `src/go/pkg/netipc/service/raw/client.go:51-74`, `src/go/pkg/netipc/service/cgroups/client.go`
- SHM single-in-flight constraint: `docs/level1-transport.md:486-492`, `docs/level1-posix-shm.md:105,183`
- Method code registry: `src/libnetdata/netipc/include/netipc/netipc_protocol.h:56-58`, `docs/level1-wire-envelope.md:71-81`
- Wire constants and validation: `src/libnetdata/netipc/include/netipc/netipc_protocol.h:29-95`, `docs/level1-wire-envelope.md:314-330`
- License signal: no SPDX on plugin-ipc sources; `SPDX-License-Identifier: GPL-3.0-or-later` on Netdata-tree wrapper (`src/libnetdata/netipc/netipc_netdata.c:1`)
- Skill discovery already mentions netipc: `.agents/skills/project-writing-collectors/SKILL.md:193-205,428-440`
- Cross-check: no Rust consumers of netipc (`grep -rn 'netipc' src/crates/` returns only the crate itself and the workspace registration); no Go consumers outside `pkg/netipc` (`grep -rln 'netipc' --include='*.go' src/ | grep -v 'pkg/netipc'` is empty)
- Recent SOW mentioning netipc: `.agents/sow/done/SOW-0009-20260502-project-writing-collectors-skill.md:126,254-255,270,355,383`

End of inventory.
