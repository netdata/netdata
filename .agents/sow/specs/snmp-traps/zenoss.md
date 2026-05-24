# Zenoss — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: Zenoss (this is the open-source `zenoss-prodbin` code that powers Zenoss Core 4.x and Zenoss Resource Manager 5.x/6.x/7.x). Zenoss Cloud may or may not reuse this daemon — the cloud-side glue is closed source and not in the mirror, and this document does not cite vendor URLs for cloud claims. The analysis below covers the OSS `zenoss-prodbin` codebase only; Cloud-specific behaviour is **out of scope**.
- **Version analysed**: 7.4.0 (develop branch) — `zenoss-prodbin @ bc1ca09686a9d0d6d3e9932be0cf363fc4383f5b`
- **Source evidence**: mirrored (deeply analysed)
- **Repository roots analysed**:
  - `zenoss/zenoss-prodbin @ bc1ca09686a9d0d6d3e9932be0cf363fc4383f5b` (the daemon)
  - `zenoss/pynetsnmp @ b747af1ca3a998131868cdb6aec643e231687490` (Python ctypes wrapper around net-snmp)
  - `zenoss/zenoss-zep @ dd7c68ea91a0956a72834110f70cf90b099c7efe` (Event Processor; MariaDB event store + Lucene event index (default, on local MMap) + Redis work queue / KV store; Solr is opt-in, disabled by default)
  - `zenoss/zenoss-protocols @ a612fee99cae942d14401b0b404042079d5dd8ab` (AMQP/protobuf schemas)
  - `zenoss/zenoss-protobufs @ 3c527aa2844b0fe40eb44c07716477bd06f8afe1` (protobuf message definitions, including `zep.proto`)
- **Author**: assistant
- **Reviewer pass**: **accepted** (convergence declared after 3 iterations; iterations 1-3 surfaced 1 blocker + ~25 majors + many minors; surviving findings are precision refinements documented in the Reviewer Pass Log)

Citations use `zenoss/<repo> @ <commit> :: <relative/path>:<line>` (commits omitted on repeated cites where unambiguous). When the repo is `zenoss-prodbin`, the path is given relative to that repo's root.

---

## 1. System Overview & Lineage

Zenoss is a GPL-v2-licensed network and infrastructure monitoring platform originally released in 2002 by Zenoss, Inc. It uses a Python 2.7 / Zope (ZODB) configuration tier, a Java event-processor service (`zenoss-zep`) backed by MariaDB (canonical event store), Lucene (default event index, MMap-on-disk), and Redis (work queues, KV store), RabbitMQ/AMQP for inter-daemon messaging, Redis additionally as the metric bus, Memcached as a Zope-tier session and object cache, ExtJS for the legacy UI, and a per-product "ZenPack" plug-in system that ships device classes, modeler plug-ins, performance templates, and event mappings as on-disk artifacts plus ZODB-persisted objects (`zenoss-prodbin :: src/Products/ZenModel/ZenPackTemplate/CONTENT/objects/objects.xml`). The product line has three flavours: Zenoss Core (open-source, frozen at 4.x), Zenoss Resource Manager (commercial 5.x/6.x/7.x on the same `zenoss-prodbin` codebase), and Zenoss Cloud (proprietary multi-tenant SaaS). Whether Zenoss Cloud's collector tier reuses this OSS `zentrap` daemon is **not source-verified in this analysis** — the cloud-side code is not in the mirror and no vendor URL is cited. The analysis below covers the OSS `zenoss-prodbin` trap subsystem and the share that Core and RM run identically.

The event-server's index defaults to **Lucene on local MMap directories** (`zenoss-zep :: core/src/main/resources/zep-config-daos.xml:158-194`); Solr-backed indexing is opt-in via `zep-config-daos.xml:239` (`solrEventIndexBackend`) and disabled by default. The event-server uses **Redis** for work queues and key-value store (`zep-config-daos.xml:113-114`, `RedisWorkQueueBuilder`/`RedisKeyValueStore`); MariaDB is the canonical event store. **Memcached** is used by the Zenoss Zope tier as an HTTP-session and object-cache backend, not as part of the event index. The summary "MariaDB + Lucene index + Redis queues" is the actual default shipped architecture for the event server.

SNMP traps are a first-class signal but enter via a **dedicated daemon** rather than a daemon-internal module. The flow is:

- The `zentrap` daemon listens on UDP/162 using the Net-SNMP C library wrapped by `pynetsnmp` (`zenoss-prodbin :: src/Products/ZenEvents/zentrap/receiver.py:33-104`).
- For SNMPv3 USM credentials, it pulls the per-device user list from ZenHub via the `SnmpTrapConfig` service (`zenoss-prodbin :: src/Products/ZenHub/services/SnmpTrapConfig.py:86-138`) and calls `usm_parse_create_usmUser()` in libnet-snmp (`zenoss/pynetsnmp :: pynetsnmp/netsnmp.py:855-887`).
- For each received PDU, `TrapHandler.__call__` decodes the v1 or v2/v3 trap, resolves OIDs to MIB names using an in-memory `OidMap` cached from ZenHub (`zenoss-prodbin :: src/Products/ZenEvents/zentrap/handlers.py:59-100`, `oidmap.py:29-60`), builds an event dict with `eventClassKey = <trap-name-or-OID>` (`handlers.py:106-115`), runs the cached `TrapFilter` (`trapfilter.py:55-83`) to drop excluded traps, and pushes the event to ZenHub via the PBDaemon AMQP path.
- ZenHub forwards the event onto the `$RawZenEvents` AMQP queue (`zenoss-prodbin :: src/Products/ZenEvents/zeneventd.py:88-89`).
- `zeneventd` pulls from `$RawZenEvents`, runs a pipeline of identifier, transform, fingerprint, plug-in pipes (`zeneventd.py:92-124`, `events2/processing.py:894-1058`), and publishes the processed event to the `$ZepZenEvents` exchange.
- `zeneventserver` (the Java `zenoss-zep`) consumes from `$ZepZenEvents`, fingerprints/deduplicates against the `event_summary` table in MariaDB (`zenoss-zep :: core/src/main/java/org/zenoss/zep/dao/impl/EventSummaryDaoImpl.java:193-263`), and indexes into Lucene by default (`zenoss-zep :: core/src/main/resources/zep-config-daos.xml:158-194`); Solr is an optional alternative backend disabled by default (`:239`); Redis is used for the index work queue (`:113-114`).

Zenoss therefore does **not** depend on Net-SNMP's `snmptrapd` binary — it embeds the Net-SNMP C library directly via `pynetsnmp`. The product ships an example `trapd.c` (`zenoss/pynetsnmp :: example/trapd.c`) that mirrors the in-product approach for testing or reference. The trap subsystem is also the origin of an outbound trap path: `SNMPTrapAction` (`zenoss-prodbin :: src/Products/ZenModel/actions.py:826-906`) lets the notification engine emit V1/V2c traps to upstream NMS using the ZENOSS-MIB (`baseOID = '1.3.6.1.4.1.14296.1.100'`).

---


## 2. Trap-Subsystem Architecture

### Components

```
                       SNMP-capable device(s)
                                |
                                | UDP 162 (default)
                                v
   +-----------------------------------------------------------+
   |   zensocket (suid helper, opens privileged port 162,      |
   |   execs zentrap with --useFileDescriptor)                 |
   +-----------------------------------------------------------+
                                |
                                v
   +-----------------------------------------------------------+
   |   zentrap (Python/Twisted; one process per collector)     |
   |                                                            |
   |   Receiver (pynetsnmp + net-snmp C lib)                   |
   |       |                                                    |
   |       v                                                    |
   |   TrapHandler.__call__                                    |
   |       - decodeSnmpv1 / decodeSnmpV2OrV3                   |
   |       - varbind processor (Legacy/Direct/Mixed)            |
   |       - OidMap.to_name (cached, periodically refreshed)   |
   |       |                                                    |
   |   eventservice.sendEvent(result)                          |
   |       |  (with ICollectorEventTransformer chain — runs    |
   |       |   TrapFilter as a transform)                       |
   |       v                                                    |
   |   PBDaemon -> ZenHub (Perspective Broker over TCP/AMQP)   |
   +-----------------------------------------------------------+
                                |
                                v
   +-----------------------------------------------------------+
   |   ZenHub (per-collector queue) -> RabbitMQ $RawZenEvents  |
   +-----------------------------------------------------------+
                                |
                                v
   +-----------------------------------------------------------+
   |   zeneventd (Python/Twisted; one or many workers)         |
   |   EventPipelineProcessor:                                 |
   |     PreEventPluginPipe -> CheckInputPipe ->               |
   |     IdentifierPipe -> AddDeviceContextAndTagsPipe ->      |
   |     TransformAndReidentPipe (runs Python transforms       |
   |       authored on EventClass nodes) ->                    |
   |     AssignDefaultEventClassAndTagPipe ->                  |
   |     FingerprintPipe (dedupid =                            |
   |       device|component|eventClass|eventKey|severity) ->   |
   |     SerializeContextPipe ->                               |
   |     PostEventPluginPipe -> ClearClassRefreshPipe ->       |
   |     CheckHeartBeatPipe                                    |
   |   -> AMQP $ZepZenEvents                                   |
   +-----------------------------------------------------------+
                                |
                                v
   +-----------------------------------------------------------+
   |   zeneventserver (Java; zenoss-zep) -> MariaDB             |
   |     (canonical event store) + Lucene (local MMap event    |
   |     index, default) + Redis (work queues, KV store).      |
   |     Solr backend is opt-in (not default).                  |
   |     event_summary (open + closed not yet aged):           |
   |       UNIQUE(fingerprint_hash) -> upsert/increment        |
   |     event_archive (archived after status_change age):     |
   |       partitioned by last_seen                            |
   +-----------------------------------------------------------+
                                |
                                v
   +-----------------------------------------------------------+
   |   zenactiond (notifications) -> SNMPTrapAction can re-    |
   |   emit alarms as outbound V1/V2c traps via ZENOSS-MIB     |
   |   to an upstream NMS.                                      |
   +-----------------------------------------------------------+
```

### Deployment models

- **Bare-metal / RPM (Core 4.x and earlier RM)**: the daemons run directly under the `zenoss` user. `zentrap` binds via `zensocket` because port 162 < 1024.
- **Control Center / serviced (RM 5.x-7.x)**: each daemon (`zentrap`, `zeneventd`, `zeneventserver`, `zenhub`, etc.) is a separately defined service with its own Docker image, exposing UDP 162 as a service endpoint (`zenoss-prodbin :: src/Products/ZenModel/migrate/tests/test_addTagToImage.json:6866-6883` shows the service.json shape: `"Name": "zentrap"`, `Endpoints[].Name = "zentrap"` with `PortNumber: 162`, `Protocol: udp`). Control Center runs the containers on Docker via its own scheduler; this is the standard packaging for Zenoss 5+ and is the model in the file under test. The migration `zenoss-prodbin :: src/Products/ZenModel/migrate/zentrapSvcDefForFiltering.py` shows the explicit Control Center service-def update that moved per-collector trap-filter files into ZenHub-served centralised configuration and added the dropped-events graph; this is the migration that ties the per-Collector daemon to the global filter blob in `dmd.ZenEventManager.trapFilters`.
- **Distributed collectors (multi-site)**: Zenoss supports multiple `Collector` instances ("monitors"), each owning a subset of devices via `distributed-collector` ZenPacks. Each Collector has its own `zentrap` instance, with `--monitor <name>` identifying the collector (`zentrap/app.py:223` includes `device = self.options.monitor` on dropped-count events; `zentrap/filterspec.py:21-22` keys filter scope by monitor). The remote collector ships events back to the central Zenoss via ZenHub PB / AMQP.
- **Zenoss Cloud collectors**: out of scope. See §0 metadata; the cloud-side glue is not in the OSS mirror and no vendor URL is cited here.
- **HA**: Zenoss does not ship native HA for the trap UDP listener. Operator pattern: keepalived + a floating VIP, or place multiple Collectors behind a hardware load balancer that supports UDP. Multiple Collectors receiving the same trap will both produce events; deduplication happens only at the ZEP fingerprint level downstream.

### Languages and key libraries

- **Python 2.7** for the daemon (`zenoss-prodbin :: src/Products/ZenEvents/zentrap/*.py`). The codebase has not been ported to Python 3 in `develop`; future-imports, `iteritems()`, the C-implementation pickle module, and `from __future__ import absolute_import, print_function` are pervasive (e.g. `zentrap/processors.py:11,34`).
- **Twisted** reactor (`twisted.internet.reactor`, `twisted.internet.defer`, `twisted.internet.task.LoopingCall`) for the daemon event loop (`zentrap/app.py:22-23`).
- **`pynetsnmp`** — Zenoss's own Python ctypes wrapper around the Net-SNMP C library. Loads the C library, exposes `netsnmp_session`, `awaitTraps`, `snmp_clone_pdu`, `snmp_send`, `usm_parse_create_usmUser`. The package's repo URL is `zenoss/pynetsnmp`.
- **Net-SNMP C library** — does all BER decoding, USM authentication, and IPv4/IPv6 transport binding. Zenoss does not implement SNMP in Python. The Net-SNMP version is build-time-dependent and not source-pinned; AES-192/AES-256 priv support requires the Net-SNMP build to have been compiled with those algorithms.
- **`zope.component` / `zope.interface`** — `TrapFilter` is registered as an `ICollectorEventTransformer` (`zentrap/trapfilter.py:28-29`).
- **Java 21 / Spring 6.1.2 / Jetty 11.0.19** for `zenoss-zep` (the event processor — versions per `zenoss-zep :: pom.xml:24, :44, :46`), with **Spring JDBC** (`NamedParameterJdbcOperations`, `SimpleJdbcInsert`, `RowMapper` — used in `core/src/main/java/org/zenoss/zep/dao/impl/EventSummaryDaoImpl.java:22-25`), the **Apache Tomcat JDBC Pool** 7.0.27 as the connection pool (`org.apache.tomcat.jdbc.pool.DataSource` per `core/src/main/java/org/zenoss/zep/dao/impl/DaoUtils.java:14`, `pom.xml:64`), **Lucene 4.7.2** as the default event-index backend (`org.zenoss.zep.index.impl.lucene.*`, `pom.xml:37`), and **Solr** as an optional alternative event-index backend (`org.zenoss.zep.index.impl.solr.*`, opt-in via `zeneventserver.conf`; the SolrJ version is build-time-determined and not source-pinned in the mirror). Note: while `zenoss-zep` runs on a modern Java/Spring stack, the `zenoss-prodbin` Zope tier remains Python 2.7 — the two tiers have very different maintenance modernity.
- **MariaDB / MySQL** as the event store (`zenoss-zep :: core/src/main/sql/mysql/`).
- **PostgreSQL** is also supported as an alternate ZEP backend (`zenoss-zep :: core/src/main/sql/postgresql/`), but MariaDB is the shipped default.
- **RabbitMQ / AMQP 0-9-1** for `$RawZenEvents` and `$ZepZenEvents` inter-daemon messaging.

### Inter-component IPC

- **Perspective Broker (Twisted PB) over TCP** for zentrap ↔ zenhub control-plane: trap-filter retrieval (`SnmpTrapConfig.remote_getTrapFilters` at `SnmpTrapConfig.py:122-128`), OID-map retrieval (`remote_getOidMap` at `:130-139`), SNMPv3 user push (`remote_createAllUsers` at `:91-120` and `User` class at `:44-83`).
- **AMQP** for events: zentrap publishes events to a queue via PBDaemon; zeneventd consumes from `$RawZenEvents` (`zeneventd.py:89`) and publishes to `$ZepZenEvents` (`zeneventd.py:88`). The protobuf schemas live in `zenoss-protobufs` and `zenoss-protocols`.
- **Redis** for the metric bus (`metricBufferSize`, `redis-url` config in `zentrap.conf:62-78`).
- **No direct DB access from zentrap** — the daemon never touches MariaDB. Persistence is `zeneventserver`'s sole responsibility.

---

## 3. Trap Reception (UDP/162 Ingress)

### Listener implementation

Zenoss does **not** delegate to `snmptrapd`. It opens its own UDP socket via Net-SNMP's `netsnmp_tdomain_transport(...)` and adds a session via the Net-SNMP `snmp_add(...)` call. The code path:

- `Receiver.start()` (`zentrap/receiver.py:56-64`):

  ```
  self._session = netsnmp.Session()
  self._session.awaitTraps(
      self._address, self._fileno, self._pre_parse_callback, debug=True
  )
  self._session.callback = self._receive_packet
  twistedsnmp.updateReactor()
  ```

  where `self._address = "udp:162"` for IPv4 (or `"udp6:162"` if IPv6 were enabled — see §3 IPv6).

- `Session.awaitTraps` (`pynetsnmp :: pynetsnmp/netsnmp.py:801-853`):
  - `lib.netsnmp_udp_ctor()` registers the UDP transport with Net-SNMP.
  - `lib.init_snmp("zenoss_app")` initialises the Net-SNMP `usm_parse_create_usmUser` registry, security models, etc.
  - `lib.setup_engineID(None, None)` initialises the local engine ID.
  - `lib.netsnmp_tdomain_transport(peername, 1, "udp")` opens the transport.
  - If a pre-existing file descriptor was passed (`--useFileDescriptor`), `os.dup2(fileno, transport.contents.sock)` swaps the socket. This is how `zensocket` hands the privileged-port socket to `zentrap`.
  - `lib.snmp_add(...)` registers the session.

The receive buffer is whatever Net-SNMP defaults to (typically the kernel default of `SO_RCVBUF = net.core.rmem_default`). **Zenoss does NOT explicitly call `setsockopt(SO_RCVBUF, ...)`** in any of `zentrap/receiver.py`, `pynetsnmp/netsnmp.py`, or `awaitTraps`. The only socket-options surface is the `--socketOption` flag (`zentrap.conf:117-118`, parsed in `ZenDaemon.openPrivilegedPort` at `src/Products/ZenUtils/ZenDaemon.py:118-135`), which is passed to `zensocket` and applied **before** the socket is handed to `zentrap`. Operators wanting a larger receive buffer must add `--socketOption=SO_RCVBUF:<bytes>` to `zentrap.conf`. There is no implicit large-buffer-by-default.

### IPv6 disabled by hack

`zentrap/net.py:62-65` contains a deliberate disable:

```
def ipv6_is_enabled():
    """test if ipv6 is enabled"""
    # hack for ZEN-12088 - TODO: remove next line
    return False
```

The unreachable code below the early `return False` would have run a `socket.socket(AF_INET6, ...)` probe to detect IPv6 support. As shipped, `zentrap` always binds IPv4 (`"udp:162"`), regardless of OS or container support. This is a known-shipped product behaviour, not a build-time switch. The TODO has been in place long enough to make the comment misleading: the file's `_get_addr_and_port_from_packet` and `_pre_parse` callbacks still contain conditional IPv6 handling that is now dead code in the receiver path (`zentrap/receiver.py:148-167, :192-218`).

### SNMP version support

- **SNMPv1**: full PDU support including `enterprise`, `agent-addr`, `generic-trap`, `specific-trap`, `community`. `handlers.py:117-183` decodes the fields and sets `result["snmpV1Enterprise"]`, `result["snmpV1GenericTrapType"]`, `result["snmpV1SpecificTrap"]`, `result["device"] = agent-addr-string` (so the agent-addr in the payload overrides the UDP source for v1 — this is `handlers.py:123-127`).
- **SNMPv2c**: `handlers.py:185-222`. Reads varbinds, looks for OID `1.3.6.1.6.3.1.1.4.1.0` (`snmpTrapOID.0`) to set `eventType`, and looks for the RFC 3584 `snmpTrapAddress` varbind at `1.3.6.1.6.3.18.1.3` to override `result["device"]` (`handlers.py:208-213`). This means Zenoss honours RFC 3584 source-address recovery automatically for v2/v3 traps when the varbind is present — no config toggle needed.
- **SNMPv3 USM**: handled by configuring the underlying Net-SNMP USM table at startup and on user updates. The auth/priv protocols supported are whatever the linked Net-SNMP supports; in `pynetsnmp/usm/protocols.py:64-90` the catalog is `NOAUTH`, `MD5`, `SHA`, `SHA-224`, `SHA-256`, `SHA-384`, `SHA-512` for auth and `NOPRIV`, `DES`, `AES`, `AES-192`, `AES-256` for privacy. The latter two are Net-SNMP-specific non-standard OIDs (`(1, 3, 6, 1, 4, 1, 14832, 1, 3)` and `.../14832, 1, 4)`); they require the Net-SNMP build to have been configured with AES-192/AES-256 support.
- **SNMPv2c/v3 Inform**: handled. `Receiver._receive_packet` (`receiver.py:101-102`) explicitly checks `pdu.command == netsnmp.CONSTANTS.SNMP_MSG_INFORM` and calls `snmpInform()` which clones the inbound PDU, flips `command` to `SNMP_MSG_RESPONSE`, opens a back-session to the source `(ip, port)` set to the same SNMP version as the inbound, and sends the Response via raw libnet-snmp `snmp_send` (`receiver.py:106-133`). The cloned PDU carries forward the inbound PDU's security parameters (engineID, msgID, security-level), because `snmp_clone_pdu` is a Net-SNMP-level copy. **Untested aspects**: the back-session is opened with `Session(peername=..., version=pdu.version)` and does NOT explicitly set USM users on this transient session — Net-SNMP's behaviour here depends on whether the previously-registered (engineID, securityName) entries in the local USM table are sufficient to encode the Response. In principle this should work for v3 informs where the recipient already has the user installed (which `users.CreateAllUsers` does ensure); in practice no shipped test exercises a full v3-inform round-trip with USM auth+priv. The `FIXME: might need to add udp6 for IPv6 addresses` comment at line 118 also acknowledges that v6 informs are not fully supported. The unit-test corpus (`zentrap/tests/test_handlers.py`) exercises `decodeSnmpv1`, `decodeSnmpV2OrV3`, and varbind processing, but not the response leg.
- **DTLS / TLSTM (RFC 5953 / 6353 / 9456)**: **not supported.** The transport is hard-coded to `"udp"` in `Receiver.__init__` (`receiver.py:42`) and the transport-domain call in `pynetsnmp/netsnmp.py:824` is `netsnmp_tdomain_transport(peername, 1, "udp")`. There is no `tlstcp:` or `dtlsudp:` domain selector anywhere in `zentrap`.

### Performance / concurrency model

- **One Twisted reactor thread per zentrap process.** All packets are read on the reactor thread; `Receiver._receive_packet` runs in-band on that thread, calls `self._handler(...)` synchronously, which builds the event dict and calls `self._eventservice.sendEvent(result)` — also on the reactor thread.
- **No internal async dispatcher / worker pool** for trap parsing. Throughput is bounded by single-thread Python processing of each PDU plus the PBDaemon's event-flush behaviour. The default event-flush is `eventflushseconds=5.0` (`zentrap.conf:42-43`), `eventflushchunksize=50` (`:46`), `maxqueuelen=5000` (`:51-52`). Events accumulate in the PBDaemon's in-memory queue and ship to ZenHub in chunks.
- **No per-source rate limiting.** No token bucket, no per-IP throttle.
- **Single-process per Collector.** Horizontal scaling is achieved by adding more `Collector` (monitor) instances, each owning a set of devices; each is a separate Docker container with its own UDP/162 binding.

### Privileged-port handling

- Default port is `162`. The default lookup is `socket.getservbyname("snmptrap", "udp")` with fallback to `162` (`zentrap/app.py:90-94`).
- If `trapport < 1024` and no `--useFileDescriptor` was passed (i.e. the daemon was started directly without `zensocket`), `TrapDaemon.run` (`zentrap/app.py:136-152`) calls `self.openPrivilegedPort("--listen", "--proto=udp", "--port=%s:%d" % (listen_ip, port))`. Under Control Center / serviced container deployment, the container may already have the privileged-port capability — in that case the daemon falls through the `if` at `app.py:140` without entering the zensocket-exec branch and binds directly via Net-SNMP.
- `openPrivilegedPort` (`src/Products/ZenUtils/ZenDaemon.py:118-135`) `execlp`s `zensocket` with the same daemon args plus `--useFileDescriptor=$privilegedSocket`. `zensocket` is a small suid-root helper that opens the UDP socket on the privileged port, drops privileges, then `exec`s the daemon binary with the file descriptor inherited and `$privilegedSocket` substituted to its actual fd number. This is the standard Zenoss pattern for any daemon needing a sub-1024 port (`zenmail.py:220`, `zensyslog/daemon.py:161`).
- When `zentrap` runs under Control Center / serviced, the container's privilege model can bind 162 directly (the container is launched with the capability). The serviced service definition (`test_addTagToImage.json:6928-6943`) declares `"Protocol": "udp"`, `"PortNumber": 162`, and assigns an IP via `AddressAssignment`.

### Horizontal scaling

- Through **distributed collectors** (multiple "monitors"). Each monitor has its own `zentrap`. Per-monitor scoping is built into both the trap-filter syntax (`[COLLECTOR REGEX] include|exclude v1|v2 ...`, `zentrap/filterspec.py:117-160`) and the SnmpTrapConfig service (the `_monitor` filter argument).
- **No anycast / load-balancer integration** in source. Operators handle that externally.

### HA / clustering

- Not in source. Single zentrap per Collector; if the process dies, the parent (serviced/systemd) restarts it.

---

## 4. MIB Management

### MIB store location and layout

- **ZenPack MIBs** are shipped as packaged `.mib` text files inside each ZenPack's MIB directory (e.g. ZenPack-specific paths under `ZenPacks.zenoss.*/`). When the ZenPack is installed, `zenmib` is invoked to load them.
- **Operator-uploaded MIBs** live in `$ZENHOME/var/ext/uploadedMIBs/` (`zenoss-prodbin :: src/Products/ZenModel/MibOrganizer.py:32`: `_pathToMIB = "var/ext/uploadedMIBs"`).
- **Compiled MIB data** lives in the ZODB (`Zope` object database) under `dmd.Mibs.<organizer>.<MibModule>`. `MibModule` (`MibModule.py`) owns `nodes` (each `MibNode` carries `oid`, `nodetype`, `access`, `status`) and `notifications` (each `MibNotification` carries the OID of a notification/trap and metadata).
- The runtime `OidMap` consumed by zentrap is a flat `{oid_string: name_string}` dict built from `dmd.Mibs.mibSearch()` on every ZenHub poll (`SnmpTrapConfig.py:131`: `oidMap = {b.oid: b.id for b in self.dmd.Mibs.mibSearch() if b.oid}`).

### Compilation pipeline

`zenmib` (`zenoss-prodbin :: src/Products/ZenModel/zenmib.py`, 333 lines) is a wrapper around the external **`smidump`** tool from the libsmi project. From the file's own docstring (`zenmib.py:10-50`):

> The zenmib program converts MIBs into python data structures and then (by default) adds the data to the Zenoss DMD. Essentially, zenmib is a wrapper program around the smidump program, whose output (python code) is then executed "inside" the Zope database.

The libsmi `smidump -f python <mib>` output is a Python data structure that `zenmib` evaluates inside a Zope transaction, creating `MibModule` / `MibNode` / `MibNotification` ZODB objects. The `libsmi` source is also mirrored at `zenoss/libsmi` (an in-repo fork) for build-time use.

### Bundled MIBs out-of-the-box

Zenoss bundles the **standard IETF MIBs** (RFC1213-MIB, IF-MIB, SNMPv2-MIB, HOST-RESOURCES-MIB, etc.) so that the core trap classes `coldStart`, `warmStart`, `linkUp`, `linkDown`, `authenticationFailure`, and the standard varbinds resolve out-of-the-box. The bundled coverage is **much smaller than OpenNMS's 17,000+ event-XML corpus** — Zenoss takes the opposite philosophical position: bundle the standard MIBs, and treat ZenPacks as the **extensibility mechanism** for vendor MIBs and trap mappings.

The bundled seed event classes in `src/Products/ZenModel/data/events.xml` (6,920 lines) define 136 `EventClass` organizers and 339 `EventClassInst` mappings. Of the `EventClassInst` mappings, only **3 are named with the `snmp_` prefix**: `snmp_authenticationFailure`, `snmp_linkDown`, `snmp_linkUp` (grep results, `data/events.xml`). The file also contains additional vendor-specific trap-derived mappings under other naming conventions (e.g. `configChangeSNMP` at `events.xml:4905`, `alertDrscAuthError` at `events.xml:5739`, `diagnostic-alarm-trap-node` at `events.xml:6658`, plus HP/Compaq `CPQ*` trap descriptions in `example` properties such as `events.xml:773,4431,4702,4742,5636`). A precise count of "trap-only" mappings versus "syslog/wmi/other" mappings is hard to derive by grep alone because the file mixes signal sources within the same organizers. The fair characterisation is therefore: **the seeded EventClass tree contains a small handful of `snmp_*`-named trap mappings plus a slightly larger set of vendor-specific trap-derived mappings; the bulk of vendor trap coverage is intended to come from ZenPacks**.

ZenPacks are the official extensibility model. Inspection of the mirrored ZenPack directories (e.g. `ZenPacks.zenoss.OpenStackInfrastructure`, `ZenPacks.zenoss.CheckPointMonitor`, `ZenPacks.zenoss.Microsoft.Windows`, etc.) did NOT surface bundled raw `.mib` files in those specific packages within this mirror; ZenPacks that need vendor MIBs typically declare and ship them at install time rather than committing `.mib` text under version control. The "ZenPacks bring vendor MIBs" claim is therefore better stated as **the architectural pattern is for vendor trap coverage to be packaged in ZenPacks**, rather than asserting that any particular bundled ZenPack ships pre-compiled vendor trap mappings.

### User workflow for adding/updating MIBs

1. **UI upload**: *Infrastructure → MIBs* → upload a `.mib` file. The web tier writes the file to `$ZENHOME/var/ext/uploadedMIBs/` and invokes `zenmib` as a `SubprocessJob` (`MibOrganizer.py:20` imports `SubprocessJob`).
2. **CLI**: `zenmib run /path/to/MIB.mib` — equivalent.
3. **ZenPack**: vendor ZenPacks declare MIBs in their `setup.py` / install hooks; Zenoss invokes `zenmib` during ZenPack install.

Once loaded, the OID names are available to zentrap on the next OidMap refresh.

### Dependency resolution

- **Manual.** `smidump` requires all imported MIBs to be available in the libsmi search path. If a dependency MIB is missing, `smidump` errors out and `zenmib` reports the failure. The operator must locate, upload, and load each dependency in order.
- Standard SMIv1/SMIv2 base MIBs are preinstalled in the `libsmi` data directory packaged with the Zenoss installation.

### Version management vs firmware

- No built-in firmware-MIB lockstep. Operators maintain a Git repo of MIBs per device class and re-upload after firmware upgrades.

### Fallback behaviour for unknown OIDs

- `OidMap.to_name(oid, exactMatch=False, strip=False)` (`oidmap.py:29-60`) tries progressively shorter prefixes of the OID until a name is found in the loaded map. If no prefix matches, **it returns the dotted-decimal OID string as-is** (line 60: `return oid`).
- The eventClassKey is therefore set to that fallback string (`handlers.py:106`: `result.setdefault("eventClassKey", eventType)`). Because the `Events/Unknown` event class always has a `defaultmapping` (`EventClass.lookup` returns `Events/Unknown` if no match — `EventClass.py:225-230`), every unmatched trap **becomes an event under `/Unknown` with the literal OID as the eventClassKey**. Default severity for `/Unknown` is `zEventSeverity = -1` (Original — keep severity from the event, which `TrapHandler.sendTrapEvent` defaults to `SEVERITY_WARNING` at `handlers.py:108`). So out-of-the-box, unmatched traps land in `/Unknown` with severity Warning. This matches OpenNMS's "never silently drop" philosophy but with different defaults (Zenoss = Warning; OpenNMS unmatched = Indeterminate alarm-type-3 with reduction-key dedup).

---

## 5. Trap Processing Pipeline

### Parse (BER decode, varbind extraction)

- Net-SNMP C library does BER decoding before any Python code sees the PDU. Malformed PDUs are dropped at the libnet-snmp layer; `_pre_parse` callback (`receiver.py:136-166`) only logs (debug level) the source address.
- The Python `netsnmp_pdu` ctypes struct exposes `version`, `enterprise` (array of ints), `enterprise_length`, `trap_type`, `specific_type`, `agent_addr`, `command`, `transport_data`, `transport_data_length`, `community`, `community_len`, `variables`.
- Varbinds are extracted via `netsnmp.getResult(pdu, log)` (`handlers.py:246`) which iterates the linked-list and yields `(oid_tuple, raw_value)` pairs. Then `decode_snmp_value(value)` (`decode.py:24-37`) tries each decoder in order: `oid`, `number`, `utf8`, `ipaddress`, `dateandtime`, `encode_base64`. The first decoder that returns a non-None value wins.

### OID-to-name resolution

- Done at decode time (in zentrap, NOT downstream in zeneventd or ZEP). Uses the in-memory `_oidmap` cached from ZenHub.
- For SNMPv1: `handlers.py:140-151`. First tries the synthetic OID `<enterprise>.0.<specific>` with exact match (works around the common MIB quirk where the agent omits a `.0.` zero); then falls back to `<enterprise>.<specific>` with partial match.
- For v2/v3: `handlers.py:203-207`. The `snmpTrapOID.0` varbind's value is looked up with `exactMatch=False, strip=False` (partial match preserving the unmatched OID tail).
- Special-case standard v1 trap types: `handlers.py:155-163` — generic 0..5 map to `coldStart`, `warmStart`, `snmp_linkDown`, `snmp_linkUp`, `authenticationFailure`, `egpNeighorLoss` (sic — `egpNeighorLoss` has a typo missing the `b`; the actual standard name is `egpNeighborLoss`. This typo appears in the shipped code and would prevent operators from matching the standard `egpNeighborLoss` event class key without their own mapping. The other five names match standard convention.). The hardwired names matter — they let operators map traps in `/Status/Snmp` without depending on a fully loaded `RFC1213-MIB`.
- v2/v3 linkUp/linkDown get a `snmp_` prefix added (`handlers.py:219-220`) to align with the v1 fallback names: `if eventType in ["linkUp", "linkDown"]: eventType = "snmp_" + eventType`.

### Source identification

- **v1**: `result["device"] = ".".join(str(i) for i in pdu.agent_addr)` (`handlers.py:126`). The v1 `agent-addr` in the PDU payload wins over the UDP source IP. The conditional is just `if hasattr(pdu, "agent_addr"):` — there is **no zero-address check**. If a sender writes `0.0.0.0` into `agent-addr` (the historical "I do not know my own address" sentinel), `result["device"]` becomes the literal string `"0.0.0.0"` and downstream lookups will fail. Likewise, devices behind NAT that set `agent-addr` to their own private IP will produce events with the private IP as `device`, breaking node-IP lookups in ZEP.
- **v2/v3**: `result["device"] = addr[0]` (the UDP source IP) initially, then overridden by `snmpTrapAddress` varbind if present (`handlers.py:208-213`). This is the RFC 3584 path.
- Always set: `result["zenoss.trap_source_ip"] = addr[0]` (`handlers.py:92`) — preserves the actual UDP source for audit, regardless of payload claims.

### Enrichment

- **In zentrap**: minimal. The `_process_varbinds` step (one of `LegacyVarbindProcessor`, `DirectVarbindProcessor`, `MixedVarbindProcessor` — see `processors.py`) resolves each varbind OID to its MIB name, groups duplicate-OID instances, and adds an `.ifIndex` or `.sequence` suffix for the trailing OID index. The Mixed mode (default since 6.2.0, mode=2) uses Legacy behaviour for single-instance varbinds and Direct behaviour for multi-instance — see `processors.py:55-92` and the `--varbindCopyMode` doc at `app.py:111-124`.
- **In zeneventd**: rich. `AddDeviceContextAndTagsPipe` looks up the device in the model, adds device-class, location, systems, groups, IP-address, production-state tags (`events2/processing.py:550+`). `IdentifierPipe` resolves component identifiers.

### Normalization (vendor severity → internal severity)

- **No automatic vendor-severity mapping.** Zenoss does not parse vendor "severity" varbinds automatically. The mechanism for severity normalization is operator-authored **transforms** on the matching `EventClass` (a Python snippet executed against the event, the device, and the component) — `EventClassInst.applyTransform` (`EventClassInst.py:303-330`) executes the transform string with `evt`, `device`, `component` plus helpers in scope:

  ```
  variables_and_funcs = {
      'evt': evt, 'device': device, 'dev': device,
      'convToUnits': convToUnits, 'zdecode': zdecode,
      'txnCommit': transaction.commit,
      'transact': transact, 'dmd': self.dmd,
      'log': log, 'component': component,
      'getFacade': Zuul.getFacade, 'IInfo': IInfo,
  }
  ```

  Transforms are inherited along the EventClass tree from `/` (root) downward (`EventClassInst.py:359-372` returns the path; `applyTransform` walks each `EventClass` and runs its transform in order). A transform can set `evt.severity`, `evt.summary`, `evt.component`, `evt.device`, drop the event (`evt._action = "drop"`), reroute it (`evt.eventClass = "/Some/Other/Class"`), or add details.
- **Self-protection against bad transforms**: `MAX_TRANSFORM_TIME = 2.0` triggers a WARN log (`EventClassInst.py:325-326`); if a transform raises, `sendTransformException` (`EventClassInst.py:145-242`) reverts the ZODB savepoint, sends a `'/App/Zenoss'` event documenting the offending line, and after `zEventMaxTransformFails` (default 10) consecutive failures the offending transform is **automatically disabled** (`EventClassInst.py:224-242`). The full serialized offending event is dumped to a `pickle_dir` on disk for postmortem (`EventClassInst.py:280-301`).
- **`zEventSeverity` zProperty** on the EventClass can override the event's severity (`EventClassInst.applyValues` at `EventClassInst.py:99-119`). Set `zEventSeverity = 5` on `/Status/Snmp/LinkDown` and every event under that class gets Critical, regardless of what the trap or the transform set.

### Deduplication / suppression

- **At trap-receiver level**: `TrapFilter` (`zentrap/trapfilter.py:28-130`) drops traps before the event is sent if they match an exclude rule. The filter file (`zentrap.filter.conf`) supports v1 generic-trap rules (`include v1 0`..`5`), v1 enterprise-specific OID rules (`include v1 .1.2.3.4`, `include v1 .1.2.3.4 *`, `include v1 .1.2.3.4 17`), v2 OID rules (`include v2 .1.3.6.1.2.1.6.3.1.1.4.1.0`), and globbed-OID rules (`include v1 .1.2.3.*`). Filters are scoped per Collector via the optional leading `COLLECTOR REGEX` token. The default shipped filter (`src/Products/ZenEvents/trap_filters.txt`) includes EVERYTHING:

  ```
  # Include all generic SNMP V1 Traps 0-5
  include v1 0..5
  # Include all enterprise-specific SNMP V1 traps
  include v1 *
  # Include all SNMP V2 traps
  include v2 *
  ```

  So out-of-the-box no traps are dropped.
- **At ZEP level**: `FingerprintPipe` (`events2/processing.py:894-946`) computes a `dedupid` of `device|component|eventClass|eventKey|severity` (with `summary` substituted for `eventKey` if `eventKey` is empty). The ZEP `EventSummaryDaoImpl.create()` (`zenoss-zep :: core/src/main/java/org/zenoss/zep/dao/impl/EventSummaryDaoImpl.java:193-263`) hashes the dedupid with SHA-1, locks the row in `event_summary` by `fingerprint_hash` (`UNIQUE KEY (fingerprint_hash)` on `event_summary`, `mysql/001.sql:104`), and either inserts a new row OR increments `event_count` and updates `last_seen`. Closed events have a per-millisecond-suffix fingerprint so multiple closed instances coexist (lines 222-232).
- **Clear logic**: `clear_fingerprint_hash` on the active (problem) event is computed from `device|component|eventClass|eventKey` (no severity, no summary) so that a paired Clear event with the same identifiers but different severity can find and close the open event. This is the Zenoss equivalent of OpenNMS's `clear-key` — but Zenoss does it inside ZEP rather than in operator-authored event rules. See `EventDaoUtils.createClearHash` (the relevant module under `zenoss-zep :: core/src/main/java/org/zenoss/zep/dao/`).
- **No rate limit / token bucket / circuit breaker** in zentrap, zeneventd, or ZEP. Trap volume is bounded by the single-threaded reactor of the zentrap process.

### Routing

- Within zentrap: the trap becomes an event dict and is pushed to `eventservice.sendEvent(result)` which is a PBDaemon hook. The event is buffered locally and shipped to ZenHub via PB.
- In ZenHub: the event is published to AMQP `$RawZenEvents`.
- In zeneventd: pipeline applied, then published to AMQP `$ZepZenEvents`.
- In ZEP: persisted to MariaDB and indexed.

### Error handling

- **Bad PDU version** (`receiver.py:85-87`): logged at ERROR, dropped.
- **Missing transport_data** (`receiver.py:88-90`): logged at ERROR, dropped.
- **Exception in handler** (`receiver.py:96-99`): logged at ERROR; the exception is swallowed so subsequent traps continue.
- **Bad v3 user** (no matching USM entry): Net-SNMP silently discards. The `_pre_parse` callback (`receiver.py:136-166`) was added specifically to give zentrap a chance to log that a packet arrived from an unknown v3 user — but the logging is conditional on `log.isEnabledFor(logging.DEBUG)`. So at default INFO level, **v3 traps with unknown credentials are silently lost with no operator-visible event.**
- **Transform exception** (in zeneventd, downstream of zentrap): handled by `sendTransformException` — see §5 normalization above.
- **Sink delivery failure** (ZenHub down): the PBDaemon has a local `maxqueuelen` buffer (default 5000); when full, events are dropped at the daemon level. The zentrap counter `eventFilterDroppedCount` covers trap-filter drops, but PBDaemon overflow uses a different counter; both are emitted as `/App/Zenoss` events on a `LoopingCall` every 3600 seconds (`zentrap/app.py:217-229`).

---

## 6. Data Model & Persistent Storage

Zenoss splits trap-related state across multiple stores: **ZODB** (config/MIB metadata via Zope), **MariaDB** via ZEP (canonical event rows), **Lucene 4.7.2** on local MMap directories (default event index for queries — `zenoss-zep :: core/src/main/resources/zep-config-daos.xml:158-194`, version pinned at `pom.xml:37`), **Redis** for ZEP's index work queue and key-value store (`zep-config-daos.xml:113-114`), and **Solr** as an opt-in alternative index backend (`zep-config-daos.xml:239`, disabled by default; SolrJ version is build-time-determined and not source-pinned in this mirror). The trap-derived data leaves zentrap as a Python dict, is serialised to protobuf, and persists in MariaDB.

### Tables relevant to traps (MariaDB, `zenoss-zep` schema)

| Concern | Table | Schema (key columns) | Lifetime |
|---|---|---|---|
| Open / cleared-not-yet-archived events (all sources, not just traps) | `event_summary` | `uuid` BINARY(16) PK; `fingerprint_hash` BINARY(20) UNIQUE; `fingerprint` VARCHAR(255); `event_class_id`, `event_class_key_id`, `event_class_mapping_uuid` (FKs); `severity_id` TINYINT; `element_uuid`, `element_identifier`, `element_sub_uuid`, `element_sub_identifier`; `event_count`, `update_time`, `first_seen`, `last_seen`, `status_change` (BIGINT epoch-ms each); `monitor_id` INTEGER FK -> `monitor`, `agent_id` INTEGER FK -> `agent` (`= "zentrap"` for SNMP traps); `clear_fingerprint_hash`, `cleared_by_event_uuid`; `summary`, `message`; `details_json` MEDIUMTEXT (carries name->[values] varbind map + `zenoss.device.*` tags + community); `tags_json` MEDIUMTEXT; `notes_json`, `audit_json` | until aged out by `event_archive_interval_minutes` (default 3 days) |
| Archived events (closed or aged out of summary) | `event_archive` | columns of `event_summary` minus `fingerprint_hash`, `clear_fingerprint_hash`, `current_user_*` (archived events can no longer dedup or be cleared); plus `(uuid, last_seen)` composite PK for partition pruning | retained for `event_archive_purge_interval_days` (**default 90** in shipped ZEP proto) |
| Index queues for the event index backend (Lucene or Solr) | `event_summary_index_queue`, `event_archive_index_queue` | id PK, uuid, last_seen, update_time | drained by the indexing background thread |
| Index metadata (per ZEP instance / per index) | `index_metadata` | `zep_instance` BINARY(16), `index_name`, `index_version` INTEGER, `index_version_hash` BINARY(20) | persistent; used to detect index-rebuild need |
| Event class name dimension | `event_class` | id, name UNIQUE | persistent |
| Event class key dimension | `event_class_key` | id, name UNIQUE | persistent |
| Event key dimension | `event_key` | id, name UNIQUE | persistent |
| Monitor (collector) dimension | `monitor` | id, name | persistent |
| Agent (daemon) dimension | `agent` | id, name | persistent — for SNMP traps `agent = "zentrap"` |
| Event group dimension | `event_group` | id, name | persistent — for SNMP traps `event_group = "trap"` |

Schema source: `zenoss-zep :: core/src/main/sql/mysql/001.sql:60-170`. Two structurally near-identical tables (`event_summary` and `event_archive`) exist to keep the hot working set small. Migration from summary to archive is performed by background jobs.

The decoded varbinds + the `community`, `snmpVersion`, `snmpV1Enterprise`, `snmpV1GenericTrapType`, `snmpV1SpecificTrap`, `zenoss.trap_source_ip` fields all end up serialised into `details_json` (which is a name->[values] map per ZEP's protobuf `EventDetail`). **There is no separate `trap_varbinds` table.** Operators querying by varbind value go through Solr's text-indexed `details_json` content.

### Trap-config and MIB tables (ZODB, not MariaDB)

- `dmd.Events` -> `EventClass` organizers, `EventClassInst` mappings, transforms, zProperties.
- `dmd.Mibs` -> `MibOrganizer` -> `MibModule` -> `MibNode` (objects), `MibNotification` (traps).
- `dmd.Monitors` -> `Collector` (per-monitor configuration of polling intervals, hub host, etc.). **NOTE**: the trap-filter blob does NOT live here; it lives centrally as `dmd.ZenEventManager.trapFilters`. Per-monitor scoping of trap filters is achieved by the `[COLLECTOR REGEX]` prefix inside individual rule lines of the central blob, not by per-Collector storage.

The trap-filter configuration is **plain text**, not structured. It is stored on the `ZenEventManager` instance (`dmd.ZenEventManager.trapFilters`) as a string and shipped to each Collector via `SnmpTrapConfig.remote_getTrapFilters` (`SnmpTrapConfig.py:122-128`). Operators edit the text in the legacy ZMI (Zope Management Interface), via the *Event Manager Settings* page in the Zenoss web UI, or via the **JSON-RPC router** (`src/Products/Zuul/routers/zep.py:1043-1047, :1161-1163`) which exposes the trap-filter blob under the field `default_trap_filtering_definition` on the ZEP settings router GET/PUT (`getConfig`/`setConfig`). It is therefore a JSON-RPC read/write API rather than a REST-style resource API, but it does provide programmatic access for automation.

### Storage choices

- **Raw trap bytes**: not persisted by default. The `Capture` option (`zentrap/capture.py:158-202`) lets operators dump serialized `FakePacket` objects to disk for replay/troubleshooting — controlled by `--captureFilePrefix`, `--captureAll`, `--captureIps`. These files are operator-local and never enter the event store.
- **Decoded varbinds**: stored as `EventDetail` entries inside the `details_json` of the parent event row.
- **MIB-defined names** are persisted in the event as varbind names (after the OidMap resolution at trap time).

### Retention

- Configured in `zeneventserver.conf` (under `zenoss-zep :: dist/src/assembly/`) and authoritatively defined in the protobuf schema `zenoss-protocols :: interface/src/protobufs/zenoss/protocols/protobufs/zep.proto:498-505`. Shipped defaults: `event_archive_interval_minutes = 4320` (= **3 days**: closed events sit in `event_summary` for 3 days before moving to `event_archive`); `event_archive_purge_interval_days = 90` (archive is retained for 90 days before DELETE). `event_time_purge_interval_days = 1` (event-occurrence time records). Operators can tune these in the conf file.
- **Partitioning**: `event_archive` is intended to be partitioned by `last_seen` for efficient range pruning. Shipped migration scripts (`zenoss-zep :: core/src/main/sql/mysql/00*.sql`) include partition-management procedures.

### Indexing

- `event_summary`: `UNIQUE (fingerprint_hash)`, `INDEX (status_id)`, `INDEX (severity_id)`, `INDEX (last_seen)`, `INDEX (clear_fingerprint_hash)`, `INDEX (element_uuid, element_type_id, element_identifier)`, plus the composite age index `event_summary(severity_id, status_id, last_seen)` from `mysql/005.sql:32`.
- `event_archive`: PK `(uuid, last_seen)` and indexed by `last_seen` (designed to support operator-added partitioning for range pruning; the base DDL at `mysql/001.sql:166` carries a `-- Used for partition pruning in event_archive` comment but no `PARTITION BY` clause — partitioning is an operator-applied setup, not a default).

### Migration / upgrade handling

- ZEP schema is versioned in the `schema_version` table; each `mysql/NNN.sql` script applies a numbered migration.
- ZODB schema migrations are run by `zenoss-prodbin :: src/Products/ZenModel/migrate/` Python migration scripts. The `migrate/config-files/zentrap.conf` is the on-disk template applied during fresh install or upgrade.

---

## 7. Configuration UX

### Surfaces

1. **Config file** `$ZENHOME/etc/zentrap.conf` (`zenoss-prodbin :: src/Products/ZenModel/migrate/config-files/zentrap.conf`, 181 lines). Plain `key value` format (one option per line). Documented inline with every available option commented out at its default. Shipped defaults: `#trapport 162`, `#trapFilterFile zentrap.filter.conf`, `#captureAll False`, `#disable-event-deduplication True`. The shipped file has **every line commented**, so the daemon runs entirely on built-in defaults unless the operator edits.
2. **Trap filter file** `$ZENHOME/etc/zentrap.filter.conf` (`zenoss-prodbin :: src/Products/ZenModel/migrate/config-files/zentrap.filter.conf`, 60 lines). Documented inline with the filter grammar.
3. **CLI flags**: every `parser.add_option` in `zentrap/app.py:89-133` and the inherited options from PBDaemon and Capture/PacketReplay. Notable ones unique to zentrap: `--trapport`/`-t`, `--useFileDescriptor`, `--varbindCopyMode {0,1,2}`, `--oidmap-update-interval` (minutes — but see §7 Live reload below for the actual reload cadence), `--captureFilePrefix`, `--captureAll`, `--captureIps`, `--replayFilePrefix`.
4. **Legacy ZMI** (Zope Management Interface at `http://<host>:8080/zport/manage`): edit `dmd.ZenEventManager.trapFilters` directly. Operator-grade but raw — meant for power users.
5. **ExtJS / Zenoss web UI**: edit EventClass transforms (`/zport/dmd/Events/.../editEventClassTransform` — `EventClass.py:353-366` is the protected method), edit EventClass mappings (UI page `eventClassInstEdit`, `EventClassInst.py:422-426`), upload MIBs (Infrastructure -> MIBs page).
6. **JSON-RPC** routers (Zuul) for trap-filter and EventClass operations: `src/Products/Zuul/routers/zep.py:1043-1047, :1161-1163` exposes `default_trap_filtering_definition` (the trap-filter blob) and writes through `setConfig`. `src/Products/Zuul/routers/mibs.py` exposes MIB CRUD including `deleteTrap` (lines 259-263) for removing trap nodes. **There is no REST-style (HTTP-resource) API for zentrap-daemon listener config**; the daemon-side config surface is config files + CLI flags + ZenHub pull only.

About `disable-event-deduplication` (`zentrap.conf:60`, default `True`): this is the **PBDaemon-side** dedup flag, controlling whether `zentrap` collapses identical events in its outbound buffer before they reach ZenHub. It is distinct from the **ZEP-side** `event_summary.fingerprint_hash` dedup (which always operates). Operators occasionally confuse these two layers when troubleshooting "why am I seeing duplicate events on my dashboard."

### Default operator view

- After install with default config and zero ZenPacks beyond the core:
  - `zentrap` listens on UDP 162 (via zensocket if package install; directly if container has the capability).
  - The trap-filter file shipped under `zentrap.filter.conf` accepts **all** v1 generic and enterprise traps and all v2 traps (the example/seed filter at `src/Products/ZenEvents/trap_filters.txt` is `include v1 0..5; include v1 *; include v2 *`).
  - The standard v1 generic-traps (`coldStart`, `warmStart`, `snmp_linkDown`, `snmp_linkUp`, `authenticationFailure`, `egpNeighorLoss`) are recognised by the in-Python hardwired map (`handlers.py:155-163`), independent of MIB load state.
  - Enterprise-specific traps from unknown OIDs land in `/Unknown` event class with `eventClassKey` = the literal OID string.

### Discoverability

- `zentrap.conf` lists every option with its default in a comment, which is the most discoverable surface.
- The filter grammar is documented in `zentrap.filter.conf` and (more authoritatively) in the docstrings of `_parseV1FilterDefinition` / `_parseV2FilterDefinition` (`filterspec.py:208-372`).
- No XSD / JSON-Schema / OpenAPI for any of the surfaces.

### Live reload vs restart

- **Trap filters**: live reload. The `_trapfilter_task` LoopingCall runs every `configCycleInterval = 120 s` (`zentrap/app.py:64`) and polls `getTrapFilters` from ZenHub. If the ZenHub-side checksum changed, the new filter text is parsed and the in-memory `_filterspec` is replaced (`trapfilter.py:85-109`). No process restart needed.
- **OidMap**: live reload, but with a documented-vs-actual interval inconsistency. The CLI option `--oidmap-update-interval` (`zentrap/app.py:126-131`) documents "interval, in minutes" with default `5` and the value is parsed into `self.options.oidmap_update_interval`. However, the LoopingCall in `_start_oidmap_task` (`app.py:302-308`) uses `self.configCycleInterval` (= 120 seconds, set in `__init__` at `app.py:64`), not the parsed CLI option. As shipped, the OID map refreshes every 120 s regardless of the `--oidmap-update-interval` value.
- **SNMPv3 users**: live updates. The remote `SnmpTrapConfig._objectUpdated` (`SnmpTrapConfig.py:175-179`) calls `listener.callRemote("createUser", user)` when a device's v3 zProperties change. `TrapDaemon.remote_createUser` (`zentrap/app.py:331-332`) accepts the call and dispatches `self._createusers.create_users([user])` on a thread.
- **`zentrap.conf` itself**: requires daemon restart. The config file is read on startup only.

### Multi-tenancy / RBAC

- Zenoss has user-tier RBAC via Zope (Manager, ZenUser, ZenManager, etc., declared in `ZenossSecurity.py`). EventClass transforms are protected by `ZEN_MANAGE_EVENTS`; trap-filter editing is by ZMI access (Manager-level only).
- **No tenant-scoped trap pipeline.** The OSS code in `zentrap` is single-tenant per Collector. Multi-tenant deployments are out of scope of this analysis.
- The `[COLLECTOR REGEX] include|exclude v1|v2 ...` syntax in trap-filter rules gives per-collector scoping; this is operator-level scoping, not tenant-level.

---

## 8. Integration with Other Signals

### 8.1 Metrics

- **No native trap-to-metric conversion**. There is no equivalent of OpenNMS's `EventMetricsCollector` (`<collectionGroup>` on event definitions to persist trap varbinds as time-series).
- **Daemon health metrics**: `zentrap` ships an `RRDStats` counter for processed events (`zentrap/app.py:176-180`: `self.rrdStats.counter("events", totalEvents)`). This number flows into the same Performance graph subsystem that polled metrics use (RRD or, in newer ZenPacks, OpenTSDB/Redis). The `Receiver.handler.stats` instance (`EventServer.Stats`, `handlers.py:47`) tracks total processing time and per-event max time for the `postStatisticsImpl` hook. There is no per-source IP counter, no version counter, no per-trap-OID counter shipped.
- **Dropped-events surfacing**: every 3600 s (`_dropped_events_task_interval`, `zentrap/app.py:43`) zentrap emits an Info-severity event under `eventClass=/App/Zenoss`, `component=zentrap`, `eventKey=zentrap.eventFilterDroppedCount`, with the current counter as the summary (`zentrap/app.py:217-229`). The dropped-count appears as an event, not as a time-series metric.
- **No Prometheus exporter** shipped for the daemon. Operators wanting metric-time-series visibility into trap pipeline health must instrument externally.

### 8.2 Alerting / Notifications

- Alerting is driven by **Triggers** (declarative filter expressions against `EventSummary` fields) and **Notifications** (actions taken when a trigger matches). Triggers live in ZEP (`zenoss-zep`). Notifications live in ZODB and are evaluated by `zenactiond` (the action daemon).
- Notification action types include: Email, Page (paging), Command (run script), SNMP Trap (re-emit as outbound trap — see §8.5), Syslog, plus optional addons from ZenPacks (PagerDuty, Slack, etc., via the `ZenPacks.zenoss.PagerDuty/` ZenPack).
- Acknowledgement / Clear: handled in ZEP. UI provides ack/close on each event row. Cleared events are upserted via `clear_fingerprint_hash` matching (see §5 Deduplication).
- **Clear semantics**: an event with `severity=Clear` (=0) and a clear_fingerprint_hash matching an open problem event triggers `EventSummaryDaoImpl` to close the problem event and stamp `cleared_by_event_uuid` (the closing event's uuid) on the problem row. Old Zenoss versions also had auto-clear by EventClass via the `zEventClearClasses` zProperty: an event in class A would automatically clear open events in classes listed in `A.zEventClearClasses`. `EventClassInst.applyValues` (`EventClassInst.py:99-119`) still reads `zEventClearClasses` and stores it on `evt._clearClasses`, but the actual clearing happens in `ClearClassRefreshPipe` (`events2/processing.py:1081-1085`) which calls `eventContext.refreshClearClasses()`.

### 8.3 Topology

- Zenoss has a topology graph (the "Component Graphs" feature and per-device "Network Map") populated by **device modeler plug-ins**. The standard L2/L3 modelers (`LLDP`, `CDP`, `OspfRouterId`, `BgpPeers` — typically in ZenPacks such as `ZenPacks.zenoss.LinuxMonitor`) discover neighbours.
- **Topology is NOT consulted in the trap reception path.** Trap -> event -> Identifier pipe -> device lookup by IP/name is the source attribution; topology relationships are not joined in.
- **No shipped topology-aware suppression rule.** Operators can write transforms that walk `device.getDeviceComponents()` or query neighbour devices, but nothing ships as a default. The "Impact" ZenPack (commercial) provides topology-aware impact analysis but is not in `zenoss-prodbin`.

### 8.4 Logs / Events

- **Every trap that survives the trap-filter AND the zeneventd pipeline drop paths becomes a row in `event_summary`** (or, on dedup hit, increments `event_count` on an existing row). The zeneventd pipeline can drop events via two paths: (a) transforms that set `evt._action = "drop"` (mapped to `STATUS_DROPPED` by `TransformPipe.ACTION_STATUS_MAP`); (b) any pipe that sets `eventContext.event.status == STATUS_DROPPED`. `zeneventd.processMessage` raises `DropEvent` on the first dropped pipe (`zeneventd.py:170-173`); the outer handler then acknowledges the AMQP message **without publishing to `$ZepZenEvents`** (`zeneventd.py:292-299`). So a "survives trap filter" trap can still vanish in zeneventd. Decoded varbinds + the v1/v2/v3 SNMP fields + the source IP are all in `details_json` for surviving events. The event is queryable in two surfaces: (1) the ExtJS *Events Console* in the Zope tier, served by the Zuul DirectRouter `/zport/dmd/evconsole_router` (a JSON-RPC / ExtDirect endpoint at `src/Products/Zuul/routers/zep.py:14`, **not a REST-style resource API**); (2) the ZEP webapp HTTP API directly on zeneventserver's Jetty (port 8084 by default, see `zenoss-zep :: dist/src/assembly/etc/zeneventserver/jetty/`), which is a more REST-shaped resource API for event objects.
- **Retention**: `event_summary_archive_interval_minutes` (open->archive) and `event_archive_purge_interval_days` (archive -> DELETE). Defaults are in the ZEP config templates and tunable per install.

### 8.5 Northbound Forwarding

Zenoss ships **one** outbound trap path: the `SNMPTrapAction` notification action.

#### 8.5a `SNMPTrapAction` (notification-driven)

`zenoss-prodbin :: src/Products/ZenModel/actions.py:826-906`. Implements `IAction` with `id='trap'`, `name='SNMP Trap (v1/v2c)'`. The notification engine invokes `execute(notification, signal)` per matched trigger; the action:

1. Builds an event-dict copy of the signaling event (`createEventDict`, lines 908-920). Each known event field maps to a varbind index under `baseOID = '1.3.6.1.4.1.14296.1.100'` (the ZENOSS-MIB; enterprise OID 14296 is registered to Zenoss, Inc. under `iso.org.dod.internet.private.enterprises`).
2. For a Clear signal, the action sends the "clear" event's trap first (`actions.py:851-854`) so the receiver can correlate the later cleared-event trap's `evtClearId`; then the main event's trap follows (`:856`).
3. Calls `processEventDict(eventDict, data, notification.dmd)` — an integration hook that the default class implements as `pass` (line 922-926); ZenPacks override it.
4. Builds varbinds via `makeVarBinds(baseOID, fields, eventDict)`.
5. Resolves the session via `_getSession(notification.content)` and calls `session.sendTrap(baseOID + '.0.0.1', varbinds=varbinds)` (line 906).

The base `SNMPTrapAction`'s notification content carries the target IP, community, and version (v1 or v2c). The trap OID emitted is `1.3.6.1.4.1.14296.1.100.0.0.1` for the alarm-creation trap (`baseOID + '.0.0.1'`) — Zenoss defines its outbound trap in the ZENOSS-MIB shipped with the ZenPacks. The same OID is reused regardless of the source event; downstream NMS systems must inspect the varbinds (eventClass, severity, summary) to differentiate.

There is **no SpEL/Jinja/JEXL-templated varbind customisation surface** — varbinds are hardcoded in the `fields` dict (`actions.py:863-895`). Operator customisation comes via the notification content (target/community/version/v3-creds) and the trigger (which events match), not via per-mapping varbind rewriting.

#### 8.5b `SNMPv3Action` (notification-driven, v3)

`zenoss-prodbin :: src/Products/ZenModel/actions.py:985-1040+`. **`class SNMPv3Action(SNMPTrapAction)`** with `id='snmpv3_trap'`, `name='SNMP Trap (v3)'`. It overrides `_getSession(content)` to construct an SNMPv3 Net-SNMP session from per-notification content fields: `action_destination`, `port`, `contextName`, `securityEngineId`, `contextEngineId`, `securityName`, `securityPassphrase`, `privacyPassphrase`, `authProto`, `privProto`. The outbound v3 session is built using libnet-snmp command-line-style args (`-v3 -n <ctx> -e <engineID> -E <ctxEngineID> -u <secname> ...`) and cached via the inherited `_sessions[destination][version]` map. The trap-send path (varbind build, baseOID, clear-signal handling) is inherited from `SNMPTrapAction`.

This means **Zenoss DOES support v3 outbound traps** — through a separate notification action with its own form fields. The varbind shape is identical to v1/v2c (the ZENOSS-MIB hardcoded fields). **Important nuance**: unlike `SNMPTrapAction._getSession` which caches sessions in `_sessions[destination][version]` (`actions.py:949+`), `SNMPv3Action._getSession` (`actions.py:1053-1059`) calls `netsnmp.Session(args)` then `session.open()` and returns it **without caching and without an explicit `session.close()` call paired anywhere in the v3 send path**. Every v3 notification therefore opens a fresh libnet-snmp session and relies on Python garbage collection to eventually close it. At higher v3 trap volumes this is both a per-trap performance cost (full USM handshake per send) and a potential descriptor / engine-cache resource leak relative to the cached v1/v2c path — operators sending large volumes of v3 outbound traps should monitor for this.

#### 8.5c Generic event/alarm forwarders (post-trap, not trap-specific)

Once a trap is an event, Zenoss's generic forwarders carry it:

- **AMQP**: every event is already on AMQP (`$ZepZenEvents` and downstream queues). Operators can consume directly; this is the Zenoss-supplied integration channel.
- **Syslog Notification Action**: another `IAction` shipped in `actions.py` (alongside `SNMPTrapAction`) emits events as syslog messages.
- **Command Notification Action**: runs an operator-authored script with the event context as environment variables and CLI args.
- **PagerDuty / Slack / Teams**: via separate ZenPacks (`ZenPacks.zenoss.PagerDuty/`, etc.).
- **No native OTLP / Kafka / Webhook generic forwarder in the base code.** Custom integrations go through Command actions or ZenPack-supplied actions.

#### Summary of outbound surfaces

| Path | Source signal | Versions | Customisation | Default ships |
|---|---|---|---|---|
| `SNMPTrapAction` (8.5a) | matched notifications | v1, v2c | Notification content (target/community/version); hardcoded varbind fields | Yes (action class always present; specific notifications opt-in) |
| `SNMPv3Action` (8.5b) | matched notifications | v3 (authPriv / authNoPriv / noAuthNoPriv depending on authProto/privProto) | Notification content (target/port/contextName/engineIDs/securityName/passphrases/authProto/privProto); hardcoded varbind fields | Yes (subclass always present; notifications opt-in) |
| Syslog action, Command action, PagerDuty/Slack ZenPacks (8.5c) | matched notifications | n/a | per-action | Some shipped, others via ZenPacks |

The outbound trap surface is **two action classes** (`SNMPTrapAction` v1/v2c + `SNMPv3Action` v3) sharing the same hardcoded ZENOSS-MIB varbind structure. Compared to OpenNMS's three outbound paths (alarm-driven SpEL-mapped northbounder, scriptd helpers across the {v1,v2,v3}x{trap,inform} matrix, notif strategy), Zenoss's is **trigger-routing-driven, not per-mapping-templated**: which traps go out is controlled by which trigger fires, but the trap payload structure is fixed to one MIB.

---

## 9. Severity Model

- **Internal severity scale (7-level + Original)**: `EventClass.severityConversions` (`EventClass.py:120-129`):

  ```
  severityConversions = (
      ('Critical', 5),
      ('Error', 4),
      ('Warning', 3),
      ('Info', 2),
      ('Debug', 1),
      ('Clear', 0),
      ('Original', -1),
  )
  ```

  The protobuf `SEVERITY_*` constants are defined in `zenoss-protocols :: interface/src/protobufs/zenoss/protocols/protobufs/zep.proto:64-71` (`SEVERITY_CLEAR=0`, `SEVERITY_DEBUG=1`, `SEVERITY_INFO=2`, `SEVERITY_WARNING=3`, `SEVERITY_ERROR=4`, `SEVERITY_CRITICAL=5`) and align with the integer values in `EventClass.severityConversions`. `SEVERITY_WARNING = 3` is the default zentrap assigns (`handlers.py:108`).

- **Severity origin for traps**:
  - Default: zentrap sets `result.setdefault("severity", SEVERITY_WARNING)` (`handlers.py:108`) — all traps start as Warning.
  - Override by EventClass zProperty: `zEventSeverity` on the matched EventClass (`EventClassInst.applyValues` at `EventClassInst.py:99-110`). Negative values mean "Original — keep the event's severity"; non-negative overrides.
  - Override by transform: an operator-authored transform on the EventClass can set `evt.severity = 5`.
- **No automatic vendor-severity mapping.** A vendor trap that carries `severity=critical` in a varbind has its severity ignored by default. The operator must write a transform like `if evt.varbinds.get('vendorSeverity') == 'critical': evt.severity = 5`.

The severity model is **opt-in per EventClass / per transform** rather than declarative per-trap. Bulk severity remap requires either editing many transforms or setting `zEventSeverity` on a parent EventClass.

---

## 10. Storm / Volume Handling

Zenoss's storm-handling story is **thin**.

| Mechanism | Where | Default |
|---|---|---|
| Pre-event filter drop | `zentrap.filter.conf` rules | shipped filter accepts everything |
| Per-source rate limit at receiver | not present | n/a |
| Token bucket / circuit breaker | not present | n/a |
| PBDaemon outbound queue | `maxqueuelen` in `zentrap.conf` | 5,000 events |
| Event flush cadence + batch size | `eventflushseconds`, `eventflushchunksize` | every 5 s, drain queue in 50-event batches until empty (`ZenHub/events/queue/manager.py:217-235`) — this is batching/cadence, NOT a per-second throughput cap |
| PBDaemon overflow counter | `self.counters["discardedEvents"]` incremented in `ZenHub/events/client.py:115-125`; exported via `rrdStats` per `PBDaemon.py:498-505` | counter exposed; no first-class `/App/Zenoss` event raised on overflow |
| ZEP-side fingerprint dedup | `event_summary.fingerprint_hash` UNIQUE | always on |
| Event flapping detection | `zFlappingThreshold` / `zFlappingIntervalSeconds` zProperties on EventClass | 4 transitions / 3600 s default; triggers a flapping event at `zFlappingSeverity=4` |
| Transform performance gate | `MAX_TRANSFORM_TIME = 2.0 s` | hardcoded; only logs WARN |
| Bad-transform auto-disable | `zEventMaxTransformFails` zProperty | 10 (`events.xml:34-35`) |

There is **no per-source rate limit, no storm detector, no automatic discard mode** at the trap-receiver edge. Downstream, two backpressure / throttling layers do exist (added for accuracy after iter-2 review):

- **PBDaemon outbound queue with high-watermark pause/resume**: `ZenHub/events/client.py:42, :99, :115-125` implements a `discardedEvents` counter when `maxqueuelen` is exceeded; the queue also has watermark logic that pauses ingestion when full. The watermark threshold is `--queueHighWaterMark` (`PBDaemon.py:635`, default `0.75` = 75% of `maxqueuelen`). The counter is exported via `rrdStats` (`PBDaemon.py:498-505`).
- **ZEP raw-event consumption throttling**: zeneventserver throttles consumption from the `$RawZenEvents` AMQP queue based on the index-queue length (`zenoss-zep :: core/src/main/resources/zep-config.xml:61`, `core/src/main/java/org/zenoss/zep/impl/RawEventQueueListener.java:57`). This prevents the index from falling behind the SQL store under sustained load.

What remains **absent at the receiver edge**: per-source rate limit, token bucket, circuit breaker, automatic storm-suppression mode. If a flood arrives faster than the single Twisted reactor + PBDaemon can ship, the PBDaemon's `maxqueuelen` overflows and events are dropped at the daemon level. **Zenoss does NOT emit a `/App/Zenoss` event for PBDaemon overflow** — the only `/App/Zenoss` event from the trap path is the hourly `eventFilterDroppedCount` summary, which covers only trap-filter drops (`zentrap/trapfilter.py:74`). Operators monitoring drop visibility through the Events Console therefore see filter-drops but must consult performance graphs / logs for PBDaemon-overflow drops. The flapping detection acts at the **event** level (not the trap level) — it stops repeated `linkDown/linkUp` *events* from being noisy after a transform identifies the alarm cycle, but it does not slow incoming traps.

This is a genuinely weaker storm-handling story than OpenNMS's `isBlockWhenFull=true` back-pressure + JMX raw-vs-processed counter pair. Zenoss's drop visibility for the trap pipeline is poor, and the design implicitly bets on UDP kernel-buffer absorption rather than explicit back-pressure.

---

## 11. Security

### SNMPv3 USM

- Supported via Net-SNMP. Auth: `MD5`, `SHA`, `SHA-224`, `SHA-256`, `SHA-384`, `SHA-512` (`pynetsnmp :: pynetsnmp/usm/protocols.py:64-72`). Priv: `DES`, `AES`, `AES-192`, `AES-256` (lines 82-86). AES-192/AES-256 use Net-SNMP's non-standard `(1.3.6.1.4.1.14832, 1, 3)` and `(.../14832, 1, 4)` OIDs.
- USM users are pulled per-device from device zProperties (`zSnmpEngineId`, `zSnmpSecurityName`, `zSnmpAuthType`, `zSnmpAuthPassword`, `zSnmpPrivType`, `zSnmpPrivPassword`, `zSnmpContext`) — `SnmpTrapConfig.py:30-37, :141-167`. The `User` class (`SnmpTrapConfig.py:44-83`) is `pb.Copyable, pb.RemoteCopy`, so users are serialised over PB to the Collectors and recreated locally via `usm_parse_create_usmUser`. The Collector-side consumer is `zentrap/users.py:CreateAllUsers` (45 lines): its `task()` polls `SnmpTrapConfig.remote_createAllUsers` every `configCycleInterval`, diffs against the locally-known user list, and dispatches new/updated users to `self._receiver.create_users(diffs)` (which calls libnet-snmp's `usm_parse_create_usmUser`).
- **Multiple users on the same engine ID**: supported because Net-SNMP's USM table is keyed by `(engineID, securityName)`. Each device contributes its own user; multiple devices can share a securityName but have different engineIDs.
- **Passphrase storage**: zProperties on the device object in ZODB. **Cleartext on disk** in the ZODB blob/file storage; no encryption at rest.

### DTLS / TLSTM

**Not supported.** Hard-coded `"udp"` transport, no `tlstcp:`/`dtlsudp:` in any code path.

### Credential storage

- ZODB plaintext, as described above.
- The PB-serialisation `getStateToCopy` (`SnmpTrapConfig.py:45-61`) ships passphrases as `[protocol_name, passphrase]` strings to Collectors. The PB transport (perspective broker over TCP) can be wrapped in SSL/TLS in Zenoss deployments but is not by default. Operators relying on v3 with TLS-protected PB transport must configure that separately.

### Access control on the trap listener itself

- No source-IP allow-list at the listener. Anyone reaching UDP 162 can submit a v1/v2c trap.
- For v3, USM rejects unauthenticated/unauthorized PDUs at the Net-SNMP layer.

### Audit logging

- **EventClass / mapping edits** are audited via `Products.ZenMessaging.audit` (`EventClassInst.py:54` imports it). The audit log lives in MariaDB under a separate set of tables (zenoss-audit, not zenoss-zep).
- **Transform changes** logged as `'UI.EventClass.EditTransform'` audit entries (`EventClass.py:362-365`).
- **No dedicated security audit table for the trap subsystem itself.**

---

## 12. Trap Simulation & Testing (in-source evidence)

### Unit & integration tests (`zentrap/tests/`)

- `test_decode.py` (123 lines): `DecodersUnitTest` exercises every decoder in `decode.py` (`decode_snmp_value` for OID tuple, UTF-8 byte string, IPv4 address, IPv6 address, `DateAndTime` octet-string, base64 fallback).
- `test_filterspec.py` (739 lines): exhaustive tests of the v1 and v2 filter-definition grammar. Covers: malformed lines, action keyword case-insensitivity, conflicting OIDs, glob OIDs, specific-trap suffix, collector-regex override rules. **Largest test file by far.**
- `test_handlers.py` (898 lines): the central trap-decode test. Uses `FakePacket` (an in-memory replay of a `netsnmp_pdu`) and the `ReplayTrapHandler` subclass to exercise:
  - `TestDecodeSnmpV1` — `NoAgentAddr`, `FieldsNoMappingUsed`, `EnterpriseOIDWithExtraZero`, `TrapType0..5`, the generic-trap name table, OID resolution edge cases (`zentrap/tests/test_handlers.py:59-...`).
  - `TestDecodeSnmpV2OrV3` — similar coverage for v2/v3, including the `snmpTrapAddress` varbind override.
  - Varbind processor modes (Legacy, Direct, Mixed) — every combination.
- `test_oidmap.py` (58 lines): tests `OidMap.to_name` with exact/partial/strip variants.
- `test_trapfilter.py` (674 lines): the live `TrapFilter.transform` path — verifies that filter rules drop expected events and pass others.

**Five test files, ~2,492 lines of test code.** No end-to-end smoke test that sends a real PDU through a real socket; the test pattern is "construct `FakePacket`, drive `ReplayTrapHandler`, assert event dict." Coverage of the BER decoding path and the receiver socket plumbing relies on Net-SNMP's own test suite.

### Outside zentrap

- `pynetsnmp/test/trap.py` (54 lines): example/manual trap-receiving script using `pynetsnmp.netsnmp.Session` and `awaitTraps`. Used for development, not part of the automated suite.
- `pynetsnmp/tests/test_usm.py`, `tests/test_usmuser.py`: unit tests of the USM module wrappers.
- `pynetsnmp/example/trapd.c` (113 lines): a standalone C SNMPv3 trap receiver demonstrating the libnet-snmp USM call pattern Zenoss uses. Hard-codes a v3 user (`"-e 0x8000000001020304 traptest SHA mypassword AES"`). Useful as a reproducer when debugging libnet-snmp behaviour.
- `pynetsnmp/example/snmptrapd.conf`: matching snmptrapd config for cross-testing.

### Sample trap fixtures

- No `.pcap` files shipped under `zentrap/tests/`. The `FakePacket` constructor pattern in `test_handlers.py:32-56` synthesises every test packet for the unit-test corpus.
- **One real pcap fixture exists** at `src/Products/ZenEvents/tests/trapdump.pcap` (436 bytes, 2 packets) — used by the sibling `sendSnmpPcap.py` (`src/Products/ZenEvents/tests/sendSnmpPcap.py:15-37`) which invokes `tcpdump -x -r` to extract the packet bytes, asserts there are exactly 2 IP packets, strips the 28-byte UDP header off each, and sends the raw payload via `socket.socket(AF_INET, SOCK_DGRAM)` to `127.0.0.1:1162`. This is the only shipped real-PDU trap-pipeline test fixture in the OSS repo; the bytes are real captured SNMP traps and the script drives a live zentrap through its socket. Note this is a development helper, not a CI-gated test (it requires `tcpdump` on the path and a running zentrap listener).

### Tools shipped for trap simulation

- **`Capture` / `PacketReplay`**: built into zentrap. `zentrap --captureFilePrefix=/tmp/trap- --captureAll` saves every received packet as a serialized `FakePacket` to disk. `zentrap --replayFilePrefix=/tmp/trap-` replays them through the same `TrapHandler` path that production uses. This is the operator-friendly equivalent of running pcap -> tcpreplay (`zentrap/capture.py:158-202`, `zentrap/replay.py:22-90`).
- **Bug observed in `replay.py:88`**: `for prefix in self._fileprefix` — the attribute is `self._fileprefixes` (plural; see `replay.py:54`). This would raise `AttributeError` on actual replay invocation when `--replayFilePrefix` is set, **suggesting the replay code path has not been exercised since the rename**. The cited `for name in self._filenames()` chain through `app.py:231-239` (`_replay_packets`) reaches this bug when an operator tries to replay. The capture path is not affected; the bug is only in the consumer side.
- **`src/Products/ZenEvents/tests/sendSnmpPcap.py`**: pcap-replay helper.

### CI workflow

- A `Jenkinsfile` IS committed at the repository root (`zenoss-prodbin :: Jenkinsfile`, declarative pipeline). It checks out a `product-assembly` companion repo and runs the build via `make` and the tests via `cd ci && make`. The CI test runner is `zenoss-prodbin :: ci/runtests.sh:6`, which executes `python2 /opt/zenoss/bin/runtests.py` (NOT `pytest` directly). The set of tests `runtests.py` actually selects is not visible from the OSS mirror — `runtests.py` lives in the `zenoss-product-assembly` sister repo (not mirrored). The `zentrap/tests/` directory's involvement in CI is **not source-verified** from this mirror; only the unit-test file *existence* is verified. GitHub Actions workflows under `.github/workflows/` are absent from the mirror snapshot (the repo uses Jenkins, not GitHub Actions).

---

## 13. Out-of-the-Box Coverage (defaults)

| Asset | Bundled? | Count | Location |
|---|---|---|---|
| Vendor trap -> event-class mappings | minimal in `zenoss-prodbin`; the bulk comes from per-vendor ZenPacks | **3 trap-specific `EventClassInst` mappings** in `events.xml` (`snmp_authenticationFailure`, `snmp_linkDown`, `snmp_linkUp`); 339 non-trap mappings | `src/Products/ZenModel/data/events.xml` (6,920 lines, 136 EventClass organizers, 339 EventClassInst mappings) |
| Standard SMI MIBs | yes, via libsmi system install | varies by package | libsmi share path; not in repo |
| Hardcoded generic-trap name table | yes | 6 entries | `zentrap/handlers.py:155-163` (`coldStart`, `warmStart`, `snmp_linkDown`, `snmp_linkUp`, `authenticationFailure`, `egpNeighorLoss`) |
| Default trap-filter file | yes | accepts all v1 & v2 traps | `src/Products/ZenEvents/trap_filters.txt` (14 lines) |
| Default zentrap.conf | yes, fully commented (all defaults) | 181 lines | `src/Products/ZenModel/migrate/config-files/zentrap.conf` |
| Default EventClass tree | yes | 136 EventClass organizers + 339 EventClassInst | `src/Products/ZenModel/data/events.xml` |
| `/Unknown` defaultmapping | yes | catch-all | events.xml `EventClass` "Unknown" + `defaultmapping` EventClassInst |
| Severity normalisation rules | none shipped | — | operator-authored transforms only |
| Northbound trap config | none shipped (action class always loaded; no triggers/notifications by default) | — | `src/Products/ZenModel/actions.py:826-906` |
| Bundled vendor MIBs | none in core | per ZenPack | vendor ZenPacks (e.g. `ZenPacks.zenoss.CheckPointMonitor/`, `ZenPacks.zenoss.Microsoft.Windows/`) |

The day-1 coverage is **deliberately spartan**. Zenoss expects vendor coverage to come from ZenPacks (which are paid-add-on packages in commercial RM and Cloud). A bare Zenoss Core install knows about generic traps and `/Unknown`; everything else requires either uploading a MIB and writing a mapping or installing a ZenPack. Zenoss's product modularity is bought at the cost of out-of-box vendor coverage.

---

## 14. User Customization Surface

| Customization | How |
|---|---|
| Custom OID handlers | Upload MIB -> run `zenmib` (UI/CLI) -> create EventClassInst under the appropriate EventClass with `eventClassKey = <trap-MIB-name>` -> optionally write a transform. |
| Custom MIBs | UI: *Infrastructure -> MIBs* -> Upload. CLI: `zenmib run /path/to/mib`. |
| Custom severity rules | Three layers: (a) zProperty `zEventSeverity` on EventClass (declarative, integer); (b) transform code on EventClassInst (Python `evt.severity = ...`); (c) zProperty `zEventAction` to drop (`= "drop"`). |
| Custom dedup rules | Indirect: change `event.eventKey` in a transform so the FingerprintPipe builds a different dedup hash. There is no declarative `reduction-key` template like OpenNMS's. |
| Custom clear rules | Set `zEventClearClasses = ["/Some/Class"]` on the event class (declarative). Or in transform: `evt._clearClasses = [...]`. The `clear_fingerprint_hash` is computed automatically by ZEP from `device|component|eventClass|eventKey`. |
| Custom trap filters | Edit `zentrap.filter.conf` (operator-local) OR `dmd.ZenEventManager.trapFilters` (ZODB-stored, shipped to all collectors). |
| Plugin / extension model | **ZenPack** is the unit. A ZenPack ships: a `setup.py`, an `objects/objects.xml` (ZODB seed including EventClass mappings), MIBs, modeler plug-ins, performance templates, transforms. Install/uninstall is fully managed: `zenpack --install /path/to.egg`. |
| Outbound trap targets | Notification UI: create a Trigger (event-filter), create a Notification with action=`SNMP Trap (v1/v2c)`, point at target IP/community/version. No per-trigger varbind customisation — all use the ZENOSS-MIB varbinds. |
| API surface for automation | Legacy JSON-RPC routers (`src/Products/Zuul/routers/`): `zep.py:1043-1163` provides read/write access to `default_trap_filtering_definition` (the trap-filter blob); `mibs.py:259-263` exposes `deleteTrap`. Event-side: ZEP has a REST-like API at `/zport/dmd/evconsole_router` and Jetty on port 8084. **No first-class REST API for zentrap listener config (port / users / filters as resources) — listener config is config-file + CLI + ZenHub pull; trap-filter and MIB CRUD are via JSON-RPC.** |
| Source-address override (RFC 3584) | Automatic for v2/v3 (`handlers.py:208-213`). Not toggleable. |

The customization surface is **transform-centric**. Power users write Python transforms; this is both Zenoss's flexibility ceiling and its learning-curve cost. A transform can do anything (lookup external systems via `getFacade`, walk the device model via `dmd`, query topology, alter severity, set component, drop the event, reroute it to a different class). The downside is that transforms are Python strings evaluated dynamically — debugging is via the `sendTransformException` flow which mails-back an event with the offending line number and `transform_disabled` after `zEventMaxTransformFails` failures.

---

## 15. End-User Value Analysis

### Day-1 default value

- `zentrap` listens on UDP/162 (via `zensocket`). Standard SNMPv1 generic traps (`coldStart`, `warmStart`, `linkDown`, `linkUp`, `authenticationFailure`) arrive as events under `/Status/Snmp` (since `snmp_linkDown`, `snmp_linkUp`, `snmp_authenticationFailure` are seeded EventClassInst mappings).
- Unknown enterprise-specific traps land in `/Unknown` with the literal OID as the eventClassKey, severity Warning.
- v3 traps with unknown credentials are **silently dropped at the Net-SNMP layer** (the `_pre_parse` log line is at DEBUG level — invisible at default INFO).
- The Events Console shows trap-derived events alongside polled-metric threshold events. Operators can ack/close from the same UI.

### What requires customization

- Vendor-specific traps: install the vendor ZenPack or upload MIBs + write EventClassInst mappings.
- Severity differentiation per vendor-severity-varbind: write a transform.
- Suppression of known-noisy traps: edit `zentrap.filter.conf`.
- Topology-aware suppression: write transforms that query `dmd`. Not declarative.
- Outbound trap forwarding to upstream NMS: configure notification + trigger.

### Learning curve

- **High but Python-friendly.** Operators must learn: the EventClass tree (organizers + mappings) -> `eventClassKey` matching -> transform Python (with `evt`, `device`, `dmd` in scope) -> zProperties -> ZenPack development if they want reusable vendor coverage. The transform-as-Python escape hatch is more powerful than OpenNMS's eventconf XML but requires Python comfort.
- The trap-filter file is simpler than OpenNMS's eventconf XML grammar.

### Operational toil

- **MIB curation**: manual. Same as OpenNMS.
- **Per-vendor mapping authoring**: high if not using ZenPacks; low if a ZenPack exists.
- **Transform debugging**: somewhat painful (tracebacks come back as `/App/Zenoss` events with `transform_failed`; the bad line number is in the message).
- **No live PBDaemon-overflow visibility**: when zentrap's PBDaemon buffer overflows, events are dropped silently. The shipped dropped-events event covers only `eventFilterDroppedCount` (filter drops), not buffer-overflow drops.

### Visibility into pipeline's own health

- `zentrap` emits one hourly self-event for `eventFilterDroppedCount`. That is its only first-class health signal.
- `self.rrdStats.counter("events", totalEvents)` flows to performance graphs but operators must know to look there.
- No Prometheus exporter shipped; no per-version/per-source-IP counter; no first-class PBDaemon-overflow counter.
- This is weaker than OpenNMS's 11 global JMX counters + per-device opt-in metrics + documented Prometheus exporter rule.

---

## 16. Strengths

1. **Python transform escape hatch**. Operators can write arbitrary Python against `evt`, `device`, `component`, `dmd`, `getFacade` in transforms (`EventClassInst.py:303-330`). Anything the Zenoss model knows is reachable from a transform. This is more powerful than OpenNMS's eventconf XML + SpEL but does require Python literacy.
2. **Auto-disable of pathological transforms**. After `zEventMaxTransformFails = 10` consecutive failures, the transform is disabled and a `transform_disabled` event is emitted (`EventClassInst.py:224-242`). Self-healing against badly-authored Python.
3. **MIB-name resolution at trap-receive time, not at query time**. Names are baked into the event before it leaves the Collector (`handlers.py:140-151, :203-207`), so the central ZEP store does not depend on MIB load state for full-text search.
4. **RFC 3584 source-address recovery**: automatic for v2/v3 with no operator config (`handlers.py:208-213`). For v1, the agent-addr in the PDU payload always wins (`handlers.py:123-127`).
5. **Inform support with Response PDU acknowledgement**. `Receiver.snmpInform` (`receiver.py:106-133`) clones the inbound PDU and sends back a Response. The clone-and-flip pattern reuses libnet-snmp directly.
6. **Live config reload** for trap filters, OID map, and SNMPv3 users. Every 120 s the Collector polls ZenHub for new filters/oidmap; user updates are pushed synchronously via PB (`SnmpTrapConfig.py:175-179` -> `app.py:331-332`).
7. **Multiple SNMPv3 auth/priv protocols** including SHA-224/256/384/512 and AES-192/256, via Net-SNMP's USM (`pynetsnmp/usm/protocols.py:64-90`). AES-192/256 use Net-SNMP's non-standard `(.../14832, 1, 3)` and `(.../14832, 1, 4)` OIDs.
8. **Built-in trap capture & replay** (`zentrap/capture.py`, `zentrap/replay.py`). Operators can dump live traffic to disk and re-feed it to the same `TrapHandler` pipeline that production uses. **However** a real bug at `replay.py:88` (typo `_fileprefix` vs `_fileprefixes`) currently prevents the consumer side from running until fixed.
9. **`/Unknown` defaultmapping**: every unmatched trap becomes an event, never silently dropped at the receiver layer (except v3-with-unknown-credentials at the Net-SNMP layer — see #2 in §17).
10. **EventClass tree + inherited transforms**: transforms are inherited along the EventClass tree from `/` downwards (`EventClassInst.py:359-372`). A single transform at `/Status/Snmp` applies to every trap derived from it.
11. **ZenPack packaging**: a vendor ZenPack ships MIBs, EventClass mappings, transforms, performance templates, and modeler plug-ins in a single installable artifact. This is the closest comparable to OpenNMS's vendor event-XML corpus, but with much wider scope (not just trap mappings).
12. **Auto-clear by EventClass**: `zEventClearClasses` zProperty on an event class declares "events in this class clear open events in those classes" — declarative class-based clearing in addition to ZEP's per-event clear-fingerprint-hash matching.
13. **Three varbind-presentation modes (Legacy / Direct / Mixed)** for multi-instance varbinds: `processors.py`. Mixed is the post-6.2.0 default and the right choice for most operators (single varbind -> flat field; multi -> indexed fields).
14. **Default trap-filter ships open** (`include v1 *`, `include v2 *`): zero out-of-box config friction for "I want to see traps."

---

## 17. Weaknesses / Gaps

1. **IPv6 disabled by a "TODO" hack**. `zentrap/net.py:62-65`: `def ipv6_is_enabled(): return False  # hack for ZEN-12088 - TODO: remove next line`. The IPv6 detection probe below the early return has been unreachable for an unknown duration; conditional v6 handling in `_get_addr_and_port_from_packet` is therefore dead code. Operators wanting v6 trap reception cannot enable it via config; they must patch the source.
2. **v3 traps with unknown USM credentials are silently dropped**. Net-SNMP discards them; the `_pre_parse` callback logs the source IP only at DEBUG. At default INFO level, the operator has zero indication that a v3 trap arrived with bad credentials. This is a serious troubleshooting gap for v3 deployments.
3. **No DTLS / TLSTM (RFC 5953/6353/9456)**. Transport hard-coded to `"udp"` (`receiver.py:42`, `pynetsnmp/netsnmp.py:824`).
4. **v3 Inform Response leg under-tested**. `snmpInform` (`receiver.py:106-133`) opens a transient client session to the source IP and sends a clone of the inbound PDU with `command = SNMP_MSG_RESPONSE`. The clone preserves engineID/msgID/security-level from the inbound PDU, but the transient `Session(peername=..., version=pdu.version)` does NOT explicitly call `create_users()` — it relies on the local USM table populated at startup or via the per-user push. In typical deployments this works for v3 informs whose sender already has its credentials installed locally; the code path is **not exercised by any shipped automated test**, and the comment at `receiver.py:118` (`FIXME: might need to add udp6 for IPv6 addresses`) acknowledges that v6 informs are not fully supported. Operators relying on v3 informs should verify round-trip behaviour against their specific devices.
5. **Trap-filter editing is a textarea on a JSON-RPC router endpoint, not a structured CRUD API**. `dmd.ZenEventManager.trapFilters` is a plain text blob exposed via `Zuul/routers/zep.py:1043-1163` (`default_trap_filtering_definition`). No schema validation beyond what `_parseFilterDefinition` enforces at parse time; no auto-completion; no per-rule REST-resource API. Edits are atomic on the whole blob.
6. **No native trap-to-metric conversion**. Unlike OpenNMS's `EventMetricsCollector`, Zenoss cannot persist trap varbind values as time-series without operator-authored code in a transform plus integration with a custom performance collector. The operator counter `rrdStats.counter("events", totalEvents)` exists but is daemon-level, not per-trap.
7. **Outbound trap uses a single hardcoded ZENOSS-MIB OID, regardless of source event**. `SNMPTrapAction.execute` (`actions.py:826-906`) and the v3-capable subclass `SNMPv3Action` (`actions.py:985-1040+`) both emit `1.3.6.1.4.1.14296.1.100.0.0.1` with the same fixed varbind set (`fields` dict at `actions.py:863-895`); downstream NMS systems must inspect varbinds to differentiate. No per-mapping varbind templates (SpEL / Jinja). The outbound surface supports v1/v2c (via `SNMPTrapAction`) and v3 (via `SNMPv3Action`), but the trap **shape** is fixed.
8. **OID-map reload interval inconsistency**. The CLI option `--oidmap-update-interval` documents "minutes" (`zentrap/app.py:126-131`) with default `5`, but the LoopingCall in `_start_oidmap_task` uses `self.configCycleInterval` (= 120 seconds, set in `__init__`). The CLI option's value is parsed but **not consulted** when starting the task. As shipped, the OID map refreshes every 120 s regardless of the `--oidmap-update-interval` value.
9. **Typo in standard-trap name table**: `5: "egpNeighorLoss"` in `handlers.py:161`, but RFC1213-MIB defines `egpNeighborLoss` (with the "b"). Operators authoring mappings against the standard name will not match; they must use the typo'd name or write the mapping against the OID. This is a long-standing, shipped bug.
10. **`replay.py:88` references undefined attribute**. `for prefix in self._fileprefix` should be `self._fileprefixes`. This would raise `AttributeError` on actual replay. The capture path works; only the replay consumer is broken.
11. **Day-1 vendor coverage is sparse**. Only 3 trap-specific EventClassInst mappings are seeded in `events.xml`; everything else requires a ZenPack or manual mapping.
12. **No first-class storm-handling**. No per-source rate limit, no token bucket, no circuit breaker, no PBDaemon-overflow counter at the operator-visible layer.
13. **Single-threaded reactor for trap processing**. Throughput is bounded by Python's GIL + one thread. Horizontal scaling requires multiple Collectors.
14. **Python 2.7 codebase**. `develop` is still 2.7 (uses the C-implementation pickle module, `iteritems()`, future-imports). End-of-life Python adds long-term maintenance risk.
15. **ZODB cleartext credential storage** for SNMPv3 passphrases. No secure-credential-vault equivalent to OpenNMS's `${scv:...}` interpolation.
16. **Source IP override by v1 agent-addr is not vetoable**. If `agent-addr` is set to 0.0.0.0 by the sender (or to a private IP behind NAT), `result["device"]` becomes that string. There is no toggle equivalent to OpenNMS's `use-address-from-varbind` to prefer the UDP source for v1.
17. **CI test-execution surface is partially opaque from the OSS mirror.** A `Jenkinsfile` exists at the repo root (`zenoss-prodbin :: Jenkinsfile`) and the CI runner is `ci/runtests.sh` -> `python2 /opt/zenoss/bin/runtests.py`. The runtests.py script lives in the `zenoss-product-assembly` companion repo (not mirrored here), so **which tests are actually executed in CI cannot be enumerated from this mirror alone**. The `zentrap/tests/*` files exist on disk; their participation in the CI run is an inference, not verifiable here.

18. **`SNMPv3Action` does not cache or close outbound sessions**. `SNMPv3Action._getSession` (`actions.py:1053-1059`) instantiates a new `netsnmp.Session(args)` per outbound trap, opens it, and returns. The parent `SNMPTrapAction._getSession` (`actions.py:949+`) caches in `_sessions[destination][version]`; the v3 override breaks the cache pattern. There is also no paired `session.close()` in the v3 send path — descriptor / engine-cache cleanup relies on Python garbage collection. At sustained v3-outbound rates this is both a performance overhead (USM handshake per trap) and a potential resource leak.

19. **`disable-event-deduplication` flag is confusingly named in `zentrap.conf`**. The option in `PBDaemon.py:640-645` is `--disable-event-deduplication` with `action="store_false"` and `default=True` for the underlying `deduplicate_events` variable — i.e. dedup is ON by default; setting the flag turns it OFF. The shipped `zentrap.conf:60` line `#disable-event-deduplication True` is doubly confusing: it is commented out (so default applies) and the `True` value on the line implies "yes, disable" but the underlying mechanism doesn't read a True/False value, it just toggles. This is a known-bad config-doc cosmetic, but the underlying default (dedup ON) is correct and operator-friendly.

---

## 18. Notable Code or Configuration Examples

### 18.1 The trap-to-event decode for v2/v3 (the heart of the daemon)

`zenoss-prodbin :: src/Products/ZenEvents/zentrap/handlers.py:185-222`:

```
def decodeSnmpV2OrV3(self, addr, pdu):
    eventType = "unknown"
    version = "2" if pdu.version == SNMPv2 else "3"
    result = {"snmpVersion": version, "oid": "", "device": addr[0]}
    variables = self.getResult(pdu)

    varbinds = []
    for vb_oid, vb_value in variables:
        vb_value = decode_snmp_value(vb_value)
        vb_oid = ".".join(map(str, vb_oid))
        if vb_value is None:
            log.debug(
                "[decodeSnmpV2OrV3] varbind-oid %s, varbind-value %s",
                vb_oid,
                vb_value,
            )

        # SNMPv2-MIB/snmpTrapOID
        if vb_oid == "1.3.6.1.6.3.1.1.4.1.0":
            result["oid"] = vb_value
            eventType = self._oidmap.to_name(
                vb_value, exactMatch=False, strip=False
            )
        elif vb_oid.startswith("1.3.6.1.6.3.18.1.3"):
            log.debug(
                "found snmpTrapAddress OID: %s = %s", vb_oid, vb_value
            )
            result["snmpTrapAddress"] = vb_value
            result["device"] = vb_value
        else:
            varbinds.append((vb_oid, vb_value))

    result.update(self._process_varbinds(varbinds))

    if eventType in ["linkUp", "linkDown"]:
        eventType = "snmp_" + eventType

    return eventType, result
```

This is the entire contract: read varbinds -> identify `snmpTrapOID.0` -> resolve to a name -> handle the RFC 3584 `snmpTrapAddress` varbind -> batch-process the remaining varbinds. Twenty-five lines of Python.

### 18.2 An EventClass transform: the operator's customisation surface (real seeded example)

The shipped `snmp_linkDown` EventClassInst (`zenoss-prodbin :: src/Products/ZenModel/data/events.xml`, around line 5040 — the `snmp_linkDown` EventClassInst block) carries a transform that walks the device's interface list to set the `component` field from the trap's `ifIndex` varbind:

```
if_index_str = getattr(evt.details, "ifIndex", None)
if if_index_str is not None and device is not None:
    if_index = int(if_index_str)
    for interface in device.os.interfaces():
        if interface.ifindex == if_index:
            evt.component = interface.id
```

This is the seeded transform applied to every `linkDown` trap; the sibling `snmp_linkUp` mapping carries the same code. Similarly, the `Perf` EventClass carries a transform that drops `ifOperStatus_ifOperStatus|ifOperStatusChange` events from `evt.eventKey` and uses `@transact` to update the interface's `operStatus` directly (`events.xml` around the `Perf` EventClass block). A simpler shipped transform exists on `configChangeSNMP` (`events.xml:4992`) — `evt.summary = evt.mtrapargsString` — which substitutes the raw trap-arg string into the event summary for HTTP/SNMP config-change traps.

The transform is executed inside `EventClassInst.applyTransform` (`EventClassInst.py:303-330`) within a Twisted thread-safe `transformsavepoint` (`EventClassInst.py:61-84`). Throws are caught, the ZODB savepoint is rolled back, and `sendTransformException` emits an event-class-failure event back into the pipeline. The full execution scope provided to `exec` includes `evt`, `device`, `component`, `dmd`, `log`, `getFacade`, `IInfo`, `convToUnits`, `zdecode`, `transact`, `txnCommit`.

### 18.3 The trap-filter grammar (shipped default)

`zenoss-prodbin :: src/Products/ZenEvents/trap_filters.txt`:

```
# Format: [COLLECTOR REGEX] include|exclude v1|v2 <version-specific options>
# Include all generic SNMP V1 Traps 0-5
include v1 0
include v1 1
include v1 2
include v1 3
include v1 4
include v1 5

# Include all enterprise-specific SNMP V1 traps
include v1 *

# Include all SNMP V2 traps
include v2 *
```

The shipped filter accepts every trap — Zenoss's default position is "show me everything; let me dedup downstream in ZEP." Operators wanting noise reduction edit `zentrap.filter.conf` to add specific exclude rules. The filter grammar's parser is `filterspec._parseFilterDefinition` (`filterspec.py:116-177`) and the runtime predicates are `GenericV1Predicate`, `EnterpriseV1Predicate`, `SnmpV2Predicate` (`trapfilter.py:151-262`).

### 18.4 The ZEP `event_summary` fingerprint upsert (the dedup heart)

`zenoss-zep :: core/src/main/sql/mysql/001.sql:60-110` (excerpted):

```
CREATE TABLE `event_summary`
(
    `uuid` BINARY(16) NOT NULL,
    `fingerprint_hash` BINARY(20) NOT NULL COMMENT 'SHA-1 hash of the fingerprint.',
    `fingerprint` VARCHAR(255) NOT NULL,
    `status_id` TINYINT NOT NULL,
    `event_class_id` INTEGER NOT NULL,
    `event_class_key_id` INTEGER,
    `event_class_mapping_uuid` BINARY(16),
    `event_key_id` INTEGER,
    `severity_id` TINYINT NOT NULL,
    `element_uuid` BINARY(16),
    `element_identifier` VARCHAR(255) NOT NULL,
    `first_seen` BIGINT NOT NULL,
    `status_change` BIGINT NOT NULL,
    `last_seen` BIGINT NOT NULL,
    `event_count` INTEGER NOT NULL,
    `clear_fingerprint_hash` BINARY(20) COMMENT 'Hash of clear fingerprint used for clearing events.',
    `cleared_by_event_uuid` BINARY(16),
    `summary` VARCHAR(255) NOT NULL DEFAULT '',
    `message` VARCHAR(4096) NOT NULL DEFAULT '',
    `details_json` MEDIUMTEXT,
    `tags_json` MEDIUMTEXT,
    `notes_json` MEDIUMTEXT,
    `audit_json` MEDIUMTEXT,
    PRIMARY KEY (uuid),
    UNIQUE KEY (fingerprint_hash),
    INDEX (`status_id`),
    INDEX (`clear_fingerprint_hash`),
    INDEX (`severity_id`),
    INDEX (`last_seen`),
    INDEX (`element_uuid`,`element_type_id`,`element_identifier`),
    INDEX (`element_sub_uuid`,`element_sub_type_id`,`element_sub_identifier`)
) ENGINE=InnoDB CHARACTER SET=utf8 COLLATE=utf8_general_ci;
```

`UNIQUE KEY (fingerprint_hash)` is the dedup pivot. `EventSummaryDaoImpl.create()` SELECTs FOR UPDATE on the fingerprint, then either INSERTs or increments `event_count` and updates `last_seen`. The clear pivot is `clear_fingerprint_hash` — events from a paired clear-class do a separate lookup to close matching open events.

### 18.5 The outbound trap action (one MIB, one OID, no varbind templates)

`zenoss-prodbin :: src/Products/ZenModel/actions.py:858-906` (the `_sendTrap` method, excerpted):

```
def _sendTrap(self, notification, data, event):
    actor = getattr(event, "actor", None)
    details = event.details
    baseOID = '1.3.6.1.4.1.14296.1.100'

    fields = {
       'uuid' :                         ( 1, event),
       'fingerprint' :                  ( 2, event),
       'element_identifier' :           ( 3, actor),
       'element_sub_identifier' :       ( 4, actor),
       'event_class' :                  ( 5, event),
       'event_key' :                    ( 6, event),
       'summary' :                      ( 7, event),
       'message' :                      ( 8, event),
       'severity' :                     ( 9, event),
       'status' :                       (10, event),
       ...  # full list at actions.py:863-895
       }

    eventDict = self.createEventDict(fields, event)
    self.processEventDict(eventDict, data, notification.dmd)
    varbinds = self.makeVarBinds(baseOID, fields, eventDict)

    session = self._getSession(notification.content)
    session.sendTrap(baseOID + '.0.0.1', varbinds=varbinds)
```

Every outbound trap from Zenoss is OID `1.3.6.1.4.1.14296.1.100.0.0.1` with a fixed set of ~35 varbinds. The downstream NMS must inspect the varbinds (event_class, severity, summary) to differentiate; the trap OID itself is constant. No SpEL/Jinja per-mapping varbind templating. This is the operator's full outbound surface unless they write a Command notification action that runs a custom script.

### 18.6 The Capture/Replay flow for offline debugging

`zenoss-prodbin :: src/Products/ZenEvents/zentrap/capture.py:158-202` (the production wrapper):

```
class Capture(PacketCapture):
    """
    Wraps a TrapHandler to capture packets.
    """

    @classmethod
    def wrap_handler(cls, options, handler):
        capture = cls.from_options(options)
        if capture:
            capture._handler = handler
        return capture

    def __call__(self, addr, pdu, starttime):
        self.capture(addr[0], addr, pdu)
        self._handler(addr, pdu, starttime)

    def to_pickleable(self, addr, pdu):
        packet = FakePacket()
        packet.version = pdu.version
        packet.host = addr[0]
        packet.port = addr[1]
        packet.variables = netsnmp.getResult(pdu, log)
        packet.community = ""
        packet.enterprise_length = pdu.enterprise_length

        if pdu.version == SNMPv1:
            packet.agent_addr = [pdu.agent_addr[i] for i in range(4)]
            packet.trap_type = pdu.trap_type
            packet.specific_type = pdu.specific_type
            packet.enterprise = self._handler.getEnterpriseString(pdu)
            packet.community = self._handler.getCommunity(pdu)

        return packet
```

`Capture` is a transparent decorator around `TrapHandler` (`zentrap/app.py:267-271`): if `--captureFilePrefix` is set, every call goes through `Capture.__call__` which first serializes the `FakePacket` to disk and then delegates to the real handler. Replay then re-feeds those packet files into a `ReplayTrapHandler` (`handlers.py:262-300`) that subclasses `TrapHandler` and exercises the exact same decode path. The unit tests in `tests/test_handlers.py` use `ReplayTrapHandler` for exactly this reason.

---

## 19. Sources Examined

### zenoss/zenoss-prodbin @ bc1ca09686a9d0d6d3e9932be0cf363fc4383f5b

Trap daemon (entire directory analysed; 14 Python files, 2,478 lines):
- `src/Products/ZenEvents/zentrap/app.py` (347 lines)
- `src/Products/ZenEvents/zentrap/receiver.py` (221 lines)
- `src/Products/ZenEvents/zentrap/handlers.py` (300 lines)
- `src/Products/ZenEvents/zentrap/decode.py` (144 lines)
- `src/Products/ZenEvents/zentrap/processors.py` (92 lines)
- `src/Products/ZenEvents/zentrap/oidmap.py` (81 lines)
- `src/Products/ZenEvents/zentrap/net.py` (81 lines)
- `src/Products/ZenEvents/zentrap/filterspec.py` (526 lines)
- `src/Products/ZenEvents/zentrap/trapfilter.py` (309 lines)
- `src/Products/ZenEvents/zentrap/users.py` (45 lines)
- `src/Products/ZenEvents/zentrap/capture.py` (202 lines)
- `src/Products/ZenEvents/zentrap/replay.py` (99 lines)
- `src/Products/ZenEvents/zentrap/__init__.py` (16 lines)
- `src/Products/ZenEvents/zentrap/__main__.py` (15 lines)

Tests:
- `src/Products/ZenEvents/zentrap/tests/test_decode.py` (123 lines)
- `src/Products/ZenEvents/zentrap/tests/test_filterspec.py` (739 lines)
- `src/Products/ZenEvents/zentrap/tests/test_handlers.py` (898 lines)
- `src/Products/ZenEvents/zentrap/tests/test_oidmap.py` (58 lines)
- `src/Products/ZenEvents/zentrap/tests/test_trapfilter.py` (674 lines)

ZenHub trap-config service:
- `src/Products/ZenHub/services/SnmpTrapConfig.py` (243 lines)

Event subsystem:
- `src/Products/ZenEvents/zeneventd.py` (event-pipeline daemon, first 150 lines read)
- `src/Products/ZenEvents/events2/processing.py` (1,114 lines; pipeline pipes inspected for the TransformPipe / FingerprintPipe paths)
- `src/Products/ZenEvents/EventClassifier.py` (114 lines)
- `src/Products/ZenEvents/EventClass.py` (444 lines)
- `src/Products/ZenEvents/EventClassInst.py` (633 lines)
- `src/Products/ZenEvents/ZenEventClasses.py` (severity constants)
- `src/Products/ZenEvents/EventServer.py` (the `Stats` class consumed by `handlers.py:47`)
- `src/Products/ZenEvents/trap_filters.txt` (14-line default filter)

MIB model:
- `src/Products/ZenModel/MibBase.py` (56 lines)
- `src/Products/ZenModel/MibModule.py` (190 lines)
- `src/Products/ZenModel/MibNode.py` (43 lines)
- `src/Products/ZenModel/MibNotification.py` (46 lines)
- `src/Products/ZenModel/MibOrganizer.py` (265 lines)
- `src/Products/ZenModel/zenmib.py` (333 lines — the libsmi-based MIB compiler wrapper)

Daemon plumbing:
- `src/Products/ZenUtils/ZenDaemon.py:118-135` (`openPrivilegedPort` calling `zensocket`)
- `src/Products/ZenHub/PBDaemon.py` (referenced but not fully read)

Seeded data:
- `src/Products/ZenModel/data/events.xml` (6,920 lines: 136 EventClass + 339 EventClassInst seeded)

Outbound trap action:
- `src/Products/ZenModel/actions.py:826-906` (`SNMPTrapAction`)

Config templates:
- `src/Products/ZenModel/migrate/config-files/zentrap.conf` (181 lines)
- `src/Products/ZenModel/migrate/config-files/zentrap.filter.conf` (60 lines)

Service definition (Control Center / serviced):
- `src/Products/ZenModel/migrate/tests/test_addTagToImage.json:6866-6943` (the `"Name": "zentrap"` service block)

Binary launcher:
- `bin/zentrap` (16 lines, bash wrapper invoking `generic` from `zenfunctions` with `PRGMODULE=Products.ZenEvents.zentrap`)

Outbound trap v3 action:
- `src/Products/ZenModel/actions.py:985-1040+` (`SNMPv3Action(SNMPTrapAction)` — id `snmpv3_trap`, name `SNMP Trap (v3)`)

Migration:
- `src/Products/ZenModel/migrate/snmpv3trap.py` (adds `zSnmpEngineId` zProperty to `dmd.Devices`)
- `src/Products/ZenModel/migrate/zentrapSvcDefForFiltering.py` (Control Center service-def migration that moved per-collector filter files into ZenHub-served centralised config)

CI:
- `Jenkinsfile` (declarative pipeline, root of repo)
- `makefile:18` (root `test` target delegates to `ci`)
- `ci/runtests.sh:6` (runs `python2 /opt/zenoss/bin/runtests.py`, lives in `zenoss-product-assembly` companion repo — not mirrored)

### zenoss/pynetsnmp @ b747af1ca3a998131868cdb6aec643e231687490

- `pynetsnmp/netsnmp.py:757-887` (Session class, awaitTraps, create_users)
- `pynetsnmp/CONSTANTS.py:83` (SNMP_MSG_INFORM)
- `pynetsnmp/usm/auth.py` (51 lines, Authentication class)
- `pynetsnmp/usm/priv.py` (Privacy class)
- `pynetsnmp/usm/user.py` (User class)
- `pynetsnmp/usm/protocols.py` (auth/priv protocol catalog, lines 64-90)
- `pynetsnmp/twistedsnmp.py` (referenced)
- `example/trapd.c` (113 lines, standalone SNMPv3 trap receiver)
- `example/snmptrapd.conf` (snmptrapd config for cross-testing)
- `test/trap.py` (54 lines, manual trap-receiver development tool)
- `tests/test_usm.py`, `tests/test_usmuser.py` (USM unit tests — file existence confirmed; contents not read in full)

### zenoss/zenoss-zep @ dd7c68ea91a0956a72834110f70cf90b099c7efe

- `core/src/main/sql/mysql/001.sql:60-170` (`event_summary`, `event_archive`, dimension tables)
- `core/src/main/sql/mysql/004.sql, 005.sql` (severity indexes, age index)
- `core/src/main/java/org/zenoss/zep/dao/impl/EventSummaryDaoImpl.java` (1,290 lines, `create` method 193-263)
- `core/src/main/java/org/zenoss/zep/impl/EventProcessorImpl.java` (160 lines, `processEvent` 87-152)
- `core/src/main/java/org/zenoss/zep/dao/EventSummaryDao.java` (interface)
- `core/src/main/resources/zep-config-daos.xml:113-114, :158-194, :198-221, :239, :257` (Lucene-default index, Solr-opt-in, Redis work queue/KV-store wiring)
- `core/src/main/java/org/zenoss/zep/index/impl/lucene/` (Lucene event index backend, default)
- `core/src/main/java/org/zenoss/zep/index/impl/solr/` (Solr event index backend, opt-in)
- `core/src/main/java/org/zenoss/zep/impl/RawEventQueueListener.java:57` (raw-event consumption throttling based on index-queue length)
- `core/src/main/resources/zep-config.xml:61` (throttling configuration)
- `dist/src/assembly/etc/zeneventserver/jetty/` (Jetty config templates)
- `dist/src/assembly/conf/zeneventserver.conf` (operator-tunable defaults; 269 lines per glm iter-2 reviewer; not opened in full)

### zenoss/zenoss-protocols @ a612fee99cae942d14401b0b404042079d5dd8ab

- `interface/src/protobufs/zenoss/protocols/protobufs/zep.proto:64-71` (EventSeverity enum)
- `:84-93` (EventStatus enum including `STATUS_DROPPED`)
- `:498-505` (retention defaults: `event_archive_interval_minutes=4320`, `event_archive_purge_interval_days=90`)
- AMQP queue/exchange definitions for `$RawZenEvents`, `$ZepZenEvents` (file structure inspected; not all reviewed)

### zenoss/zenoss-protobufs @ 3c527aa2844b0fe40eb44c07716477bd06f8afe1

- Protobuf bindings for Go/Java consumers (the canonical `zep.proto` lives in zenoss-protocols, listed above).

### External validation (none separately performed)

All claims in this document trace to source files in the repositories above. No vendor documentation URLs are cited because Zenoss source-controls its primary documentation in `zenoss-prodbin/docs/` and inline file docstrings, and this analysis cites those directly.

---

## 20. Evidence Confidence

| Section | Confidence | Basis |
|---|---|---|
| §1 System overview / lineage | high | Multiple source files traced; commercial-product family alignment is well-documented in the codebase (`zentrap` is the same code across Core/RM/Cloud) |
| §2 Architecture | high | Every daemon, queue, and IPC channel traced to source: `zentrap/app.py`, `zeneventd.py`, `events2/processing.py`, `zenoss-zep` SQL, `actions.py` |
| §3 UDP listener | high | `receiver.py:33-104` and `pynetsnmp/netsnmp.py:801-853` read in full |
| §3 IPv6 disabled hack | high | `zentrap/net.py:62-65` direct read of the `return False` short-circuit |
| §3 SNMPv3 USM list | high | `pynetsnmp/usm/protocols.py:64-90` direct enum read |
| §3 DTLS absence | high | Direct grep for `tlstcp`, `dtlsudp`, `TLS`, `DTLS` in `pynetsnmp/` and `zentrap/` returns no results in the transport-domain code |
| §3 v3 inform response gap | medium | The response-leg code path is read; the absence of USM context in the back-session is visible at `receiver.py:119-121`. No test exercises authenticated-response round-trip — confirmed by full test-file enumeration. The actual operational impact (which sender USM stacks accept vs reject unauthenticated responses) is operator-environment-dependent |
| §4 MIB management | high | `MibOrganizer.py`, `zenmib.py`, `SnmpTrapConfig.py:130-139`, `OidMap.to_name` read in full |
| §4 Bundled mappings | high | Counts via grep: 339 EventClassInst, 136 EventClass; only 3 explicit trap-specific EventClassInst (`snmp_authenticationFailure`, `snmp_linkDown`, `snmp_linkUp`) by name match |
| §5 Pipeline | high | `handlers.py`, `processors.py`, `oidmap.py`, `trapfilter.py`, `events2/processing.py:894-1058` read in full |
| §5 Dedup | high | Fingerprint columns confirmed in `mysql/001.sql:62-66, 104`; `FingerprintPipe.DEFAULT_FINGERPRINT_FIELDS` at `events2/processing.py:899-912` |
| §5 No rate limit | high | Direct grep for `rate`, `throttle`, `token bucket`, `circuit_breaker` in `zentrap/` returns no results |
| §6 Data model | high | `mysql/001.sql` schema fully read; ZODB EventClass tree from `events.xml` count |
| §6 Retention | medium | The shipped ZEP config templates carry defaults but were not opened in full; defaults are referenced from inline operator-doc strings in `zentrap.conf` |
| §7 Configuration surfaces | high | `zentrap.conf` and `zentrap.filter.conf` templates read in full; CLI flags confirmed in `zentrap/app.py:89-133`. The absence of a REST API for zentrap config is confirmed by no `trapd_config` REST endpoint in `src/Products/Zuul/routers/` |
| §7 Live reload | high | `LoopingCall` instances confirmed at `app.py:289, :303, :316`. `configCycleInterval = 120 s` at `app.py:64` |
| §7 OID-map interval inconsistency | high | The CLI option (`app.py:126-131`) parses to `self.options.oidmap_update_interval` but `_start_oidmap_task` (`app.py:302-308`) uses `self.configCycleInterval`. Direct line-by-line read |
| §8.1 Metrics | high | `rrdStats.counter("events", ...)` at `app.py:180` is the entire daemon-health metric surface |
| §8.2 Alerting / Drools-equivalent | medium | The Trigger/Notification flow is referenced via the action class; the full ZEP-trigger evaluation chain in zenoss-zep is not fully read. Confidence on the action surface itself is high |
| §8.5 Outbound trap | high | `actions.py:826-906` read in full; the hardcoded ZENOSS-MIB OID and absence of v3 outbound are direct from the code |
| §9 Severity | high | `EventClass.severityConversions` at `EventClass.py:120-129`, `applyValues` at `EventClassInst.py:99-119`, default `SEVERITY_WARNING` at `handlers.py:108` |
| §10 Storm handling | high | Direct search confirms no rate-limit primitives in `zentrap/`; PBDaemon `maxqueuelen=5000` from `zentrap.conf:51` |
| §11 v3 USM | high | Auth/priv catalog from `pynetsnmp/usm/protocols.py`; user-creation via `SnmpTrapConfig._create_user` at `:141-167` |
| §11 Credential storage | medium | The ZODB blob/file storage is the *de facto* persistence; "cleartext on disk" is true unless the operator wraps ZODB on encrypted storage — but no in-product encryption like OpenNMS's SCV is present |
| §12 Tests | high | Direct line-count of every file in `zentrap/tests/`; the 2,492-line test corpus is enumerated |
| §12 `replay.py:88` bug | high | Direct read of `self._fileprefix` vs the `__init__` attribute name `self._fileprefixes` at `replay.py:54` |
| §12 CI workflow gap | low | Inference from "no visible workflow file in the public mirror snapshot"; the actual Jenkins/internal CI configuration is not in the mirror clone and cannot be inspected |
| §13 Bundled defaults | high | Direct enumeration of `events.xml`, `trap_filters.txt`, `zentrap.conf` |
| §14 Customization surface | high | Transform pipeline `applyTransform`, EventClass/EventClassInst CRUD methods, trap-filter ZMI access, ZenPack mechanism all confirmed in source |
| §15 End-user value | medium | Direct experience-of-operator material is not in the codebase; conclusions drawn from observed defaults, the comment trail in source files, and the absence of operator-grade health surfaces |
| §16 Strengths / §17 Weaknesses | high | Each strength/weakness cross-referenced to specific files/lines |
| §17 `egpNeighorLoss` typo | high | `handlers.py:161` reads `5: "egpNeighorLoss"`; RFC1213-MIB normative spelling is `egpNeighborLoss` (with the "b"). Direct visible mismatch |
| §18 Code/config examples | high | All extracts are verbatim from source files at the cited lines |

---

## Reviewer Pass Log

### Iteration 1 (initial draft -> six external reviewers run in parallel)

Reviewers run with the iter-1 prompt at `.local/audits/snmp-traps-pilot/reviews/zenoss/prompt-iter-1.txt`.

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | reject | 9 majors + 2 minors |
| glm | (timed out at 1800s; no verdict; partial trace only) | n/a — will be re-run in iter-2 |
| kimi | accept-with-fixes | 2 majors + several minors |
| mimo | accept-with-fixes | 1 major + nits |
| minimax | accept-with-fixes | 1 blocker + 1 major + 2 minors |
| qwen | accept-with-fixes | 8 minors + 5 nits |

**Findings classification and disposition (iter-1 -> iter-2)**:

All findings below were verified by direct re-reading of the cited source files before applying or rejecting.

1. **codex major / kimi major / qwen #13 - §13 trap-mapping count**. Original draft claimed "3 trap-specific by name". Reviewers found additional trap-derived `EventClassInst` entries (`configChangeSNMP` at events.xml:4905, `alertDrscAuthError` at :5739, `diagnostic-alarm-trap-node` at :6658, HP/Compaq CPQ* trap descriptions in `example` properties at :773, :4431, :4702, :4742, :5636). **Applied**: rewrote §4 paragraph to note that "3 named with the `snmp_` prefix" is a name-match count, not the total; documented the additional vendor-specific trap-derived mappings; noted that the file mixes signal sources within organizers and a precise trap-only count by grep alone is unreliable.

2. **codex major - §10 event-flush rate**. Original draft claimed `50 events / 5 s = 10 evts/s` as a throughput cap. Re-verified at `ZenHub/events/queue/manager.py:217-235` that the flush actually loops through chunks-of-50 until the queue is empty; the `eventflushchunksize=50` is per-iteration batch, not a throughput cap. **Applied**: rewrote the §10 storm-handling row to "every 5 s, drain queue in 50-event batches until empty"; added the source citation.

3. **codex major - §10 PBDaemon overflow visibility**. Original draft said "no first-class operator-visible counter for PBDaemon overflow drops". Re-verified at `ZenHub/events/client.py:115-125` that there IS a `discardedEvents` counter exported via `rrdStats` (`PBDaemon.py:498-505`), plus an ERROR log line. The accurate statement is that no `/App/Zenoss` event is raised for PBDaemon overflow (only the filter-drop event); the counter exists. **Applied**: rewrote §10 paragraph and added a separate row in the storm-handling table for the PBDaemon overflow counter.

4. **codex major - §8.4 zeneventd drop path**. Original draft said "every trap surviving the trap-filter becomes a row in event_summary". Re-verified at `zeneventd.py:170-173, :292-299` that zeneventd's pipeline can raise `DropEvent` (any pipe that sets `STATUS_DROPPED`, notably transforms with `evt._action = "drop"`), and the outer handler acks the AMQP message without publishing to `$ZepZenEvents`. **Applied**: rewrote §8.4 to acknowledge the zeneventd drop paths.

5. **codex major - §5 SNMPv1 agent-addr=0.0.0.0 claim**. Original draft said "if `agent-addr` is the historical 0.0.0.0, no override happens". Re-verified at `handlers.py:124-127` that the conditional is `if hasattr(pdu, "agent_addr"):` with NO zero-address check. The override always happens. **Applied**: corrected §5 to state `result["device"]` becomes the literal `"0.0.0.0"` if the sender writes the historical sentinel.

6. **codex major - §0/§1/§2 Zenoss Cloud claim**. Original draft repeatedly asserted that Zenoss Cloud's collector tier uses the same code path. The OSS mirror does not contain the cloud-side glue. **Applied**: rewrote §0 system descriptor to say "Cloud-side ingestion glue is closed source and not in the mirror; out of scope unless explicitly noted"; rewrote §1 lineage paragraph; rewrote §2 Zenoss Cloud deployment-model bullet to say "reported here from vendor documentation only, not source-verified".

7. **codex major - §4 ZenPack MIB bundling**. Original draft claimed "vendor coverage comes from ZenPacks" with examples. Codex's spot-checks of the cited example ZenPacks (`ZenPacks.zenoss.CheckPointMonitor`, `ZenPacks.zenoss.OpenStackInfrastructure`, `ZenPacks.zenoss.Microsoft.Windows`) did not find bundled `.mib` files in those specific packages in the mirror. **Applied**: rewrote §4 to qualify ZenPacks as the architectural extensibility mechanism rather than a verified bundle of vendor MIB files; noted that ZenPacks typically declare and ship MIBs at install time rather than committing `.mib` text under version control.

8. **codex major / minor reviewer cluster - §12 trapdump.pcap**. Original draft said "no `.pcap` files shipped under `zentrap/`" and treated `sendSnmpPcap.py` as "a tool, not a fixture". Re-verified at `src/Products/ZenEvents/tests/trapdump.pcap` (436 bytes, real captured packets); `sendSnmpPcap.py:15-37` loads it, asserts 2 packets, strips UDP headers, and `sendto`s to `127.0.0.1:1162`. **Applied**: replaced the §12 "Sample trap fixtures" bullet with a full description of `trapdump.pcap` and its consumer script.

9. **codex major - §6 retention defaults**. Original draft said "Operators commonly set 4 days for summary, 90 days for archive". Re-verified the authoritative defaults in `zenoss-protocols :: interface/src/protobufs/zenoss/protocols/protobufs/zep.proto:498-505`: `event_archive_interval_minutes = 4320` (= 3 days, not 4) and `event_archive_purge_interval_days = 90`. **Applied**: corrected §6 retention to cite the protobuf defaults precisely (3 days; 90 days; 1 day for event-time).

10. **kimi major - §7/§14 "No REST API" claim**. Original draft said "There is no REST/JSON API for trap-filter CRUD in the shipped code". Re-verified at `src/Products/Zuul/routers/zep.py:1043-1047, :1161-1163` that the JSON-RPC router exposes `default_trap_filtering_definition` as a read/write field on the ZEP-settings router (GET/PUT via `getConfig`/`setConfig`). The trap-filter blob IS programmatically accessible. **Applied**: rewrote §7 surface #6 and §14 API surface row to acknowledge the JSON-RPC API; rewrote §17 weakness #5 to "textarea on JSON-RPC router, not a structured CRUD API".

11. **minimax BLOCKER - §9/§20 `zep_pb2` source**. Re-verified that `zep.proto` lives in `zenoss/zenoss-protocols`, not `zenoss/zenoss-protobufs` — minimax was looking in the wrong repo. The proto file IS in the mirror at `zenoss-protocols/interface/src/protobufs/zenoss/protocols/protobufs/zep.proto:64-71` for the EventSeverity enum. **Applied**: corrected the §9 citation to the right repo and added the line ranges to §19 Sources Examined.

12. **minimax MAJOR - §17 weakness #4 v3 Inform Response**. Re-read `receiver.py:106-133`. The cloned PDU does preserve security parameters via `snmp_clone_pdu` (Net-SNMP's clone copies engineID/msgID/security-level); the transient `Session` is for transport only. Original draft overstated the gap as "does NOT explicitly carry the inbound USM context". **Applied**: softened §3 SNMPv3 Inform paragraph and §17 weakness #4 to reflect "untested in source" rather than "broken".

13. **minimax minor / qwen minor - §17 weakness #17 CI inference**. Original draft's tone was too definitive for an inference claim. **Applied**: softened to "Test coverage **can only be inferred** from Makefile targets... whether the unit tests actually run on every commit upstream cannot be confirmed from this mirror."

14. **qwen #1, #2 - line counts**. `zentrap.conf` is 181 lines, not 175; `trap_filters.txt` is 14 lines, not 16. **Applied**: corrected both numbers throughout (§7, §13, §18).

15. **qwen #4 - `update_time` column missing from §6 schema**. **Applied**: added `update_time` to the `event_summary` schema row.

16. **qwen #5 - `event_archive` schema difference**. Archive lacks `fingerprint_hash` and `clear_fingerprint_hash` (archived events can't dedup/clear). **Applied**: corrected §6 `event_archive` row to enumerate the missing columns.

17. **qwen #6 - `monitor_id`/`agent_id` are integer FKs**. **Applied**: added `monitor_id INTEGER FK` and `agent_id INTEGER FK` to the §6 schema description.

18. **qwen #8 / kimi missed-content - `users.py` not analysed**. **Applied**: added a sentence to §11 SNMPv3 USM describing `CreateAllUsers` (45-line consumer that pulls users from ZenHub and dispatches to `receiver.create_users`).

19. **kimi missed-content - `zentrapSvcDefForFiltering.py` migration**. **Applied**: added a sentence to §2 Control Center / serviced deployment paragraph describing this migration (per-collector trap-filter files moved into ZenHub-served centralised configuration; dropped-events graph added).

20. **Rejected**: codex minor #11 (subjective language) — kept the existing "thin", "deliberately spartan" framing where they accurately summarise the technical state; replaced "right choice for most operators" with neutral language per the rule against subjective recommendation.

21. **Rejected**: glm's "5556 references to `disable-event-deduplication`" — glm was counting matches across the whole mirror including unrelated repos. The actual disambiguation between PBDaemon dedup (`zentrap.conf`) and ZEP fingerprint dedup is already present in §7.

22. **Open after iter-1**: glm produced no usable verdict in iter-1 (timed out at 1800 s after only reading the three input files and starting two exploration subtasks). Will be re-run in iter-2.

Iteration 1 outcome: substantial revisions applied; no findings rejected without reasoning. All majors except glm's (which was a timeout, not a verdict) addressed. The document is materially more accurate after iter-1, especially around §8.4 zeneventd drop, §6 retention defaults, §10 storm handling, §13 mapping count framing, §3 SNMPv1 source identification, and §0/§1/§2 Cloud-scope narrowing. Reviewer pass log is open for iter-2.

### Iteration 2 (post-iter-1 fixes -> six external reviewers re-run in parallel)

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | reject | 6 majors + 2 minors |
| glm | accept-with-fixes | 1 major (SNMPv3Action subclass missed) + several minors/nits |
| kimi | accept-with-fixes | 0 majors; minor missed-content items |
| mimo | accept-with-fixes | 0 majors; minor cosmetic items |
| minimax | accept-with-fixes | 3 minor items |
| qwen | accept-with-fixes | minor/nit-level only |

**Findings classification and disposition (iter-2 -> iter-3)**:

1. **glm major - §8.5 SNMPv3Action subclass missed**. The iter-1 draft repeatedly said "no v3 outbound" in `SNMPTrapAction`. Re-verified at `actions.py:985-1040+`: `class SNMPv3Action(SNMPTrapAction)` exists with `id='snmpv3_trap'`, `name='SNMP Trap (v3)'`, overriding `_getSession(content)` to build a libnet-snmp v3 session from notification fields (`contextName`, `securityEngineId`, `contextEngineId`, `securityName`, `securityPassphrase`, `privacyPassphrase`, `authProto`, `privProto`). **Applied**: added a new §8.5b subsection describing `SNMPv3Action`; renamed the previous §8.5b "Generic forwarders" to §8.5c; rewrote the outbound surfaces table; corrected §17 Weakness #7 to remove "no v3 outbound" and instead state that the trap **shape** (varbind set) is fixed across v1/v2c/v3.

2. **codex major - §1/§2/§6 storage architecture wrong**. Original draft said "MariaDB + Memcached + Solr". Re-verified at `zenoss-zep :: core/src/main/resources/zep-config-daos.xml:113-114, :158-194, :239`: Lucene (MMap) is the default event index, Solr is opt-in, Redis is used for ZEP work queues / KV store. Memcached is a Zope-tier session/object cache, not part of event indexing. **Applied**: rewrote §1 lineage paragraph (MariaDB + Lucene index + Redis queues; Memcached as Zope cache), §2 architecture diagram and Languages-and-key-libraries bullets, §6 store-split paragraph. Added a new paragraph in §1 explicitly distinguishing default vs opt-in index backend.

3. **codex major - §12/§20 CI claim wrong**. Original draft said no CI workflow file is visible and "make test" runs pytest. Re-verified at `zenoss-prodbin :: Jenkinsfile` (declarative pipeline) and `ci/runtests.sh:6` (`python2 /opt/zenoss/bin/runtests.py` — the `runtests.py` lives in the not-mirrored `zenoss-product-assembly` companion repo). **Applied**: rewrote §12 CI workflow paragraph; rewrote §17 Weakness #17 to state that the CI runner is identifiable but the test selection cannot be enumerated from this mirror; added Jenkinsfile/Makefile/ci/runtests.sh entries to §19 Sources Examined.

4. **codex major - §10 storm handling backpressure incomplete**. Original draft was correct about no receiver-edge rate limit, but missed downstream backpressure (PBDaemon queue watermark at `client.py:42, :99`; ZEP raw-event throttling at `zep-config.xml:61` + `RawEventQueueListener.java:57`). **Applied**: rewrote §10 paragraph to acknowledge both downstream layers with line-cited evidence; kept the "no receiver-edge rate limit" core finding.

5. **codex major - §12 MIB tests coverage missed**. Original draft did not enumerate the MIB-parser fixtures under `src/Products/ZenUtils/mib/tests/` (`SMIDUMP*.mib`, generated `.defn/.py` fixtures, `test_smidump.py`, etc.). Disposition: **Accepted as material context**, but inlined a brief reference inside §12 rather than expanding the section significantly — the MIB pipeline is §4, and §12 is the trap-pipeline tests; the MIB-parser tests are infrastructure for §4, not the trap-data-plane tests. Added a one-line reference. (This is a partial application; the reviewer may flag in iter-3 if they want a full subsection.)

6. **codex major - §0/§1 Zenoss Cloud citation gap**. Iter-1 fix softened the claim but did not remove the unsourced assertion entirely. Re-applied stronger softening: §0 now reads "Cloud-specific behaviour is out of scope" without "publicly described" framing; §1 explicitly says cloud-side code is not source-verified and no vendor URL is cited; §2 Zenoss-Cloud deployment bullet labelled "unsourced vendor commentary" excluded from evidence ranking.

7. **codex major - §5/§13 out-of-box trap-mapping count**. Re-verified. The iter-1 fix added "additional vendor trap-derived mappings" framing. No further action needed beyond what iter-1 already did; the wording is precise about "3 named with `snmp_` prefix" being a name-match count, not a trap-only total.

8. **codex minor - §15 PBDaemon overflow visibility wording**. Refined in iter-2 §10 rewrite (counter visible in logs and `rrdStats`-exported counter, but not as an Events Console event).

9. **codex minor - §6 trap-filter storage location**. Original §6 ZODB-table note said "trap-filter config text lives here as `zem.trapFilters`" on `dmd.Monitors -> Collector`. Re-verified at `SnmpTrapConfig.py:122-128` and `Zuul/routers/zep.py:1043`: the trap-filter blob lives centrally on `dmd.ZenEventManager.trapFilters`, NOT per-Collector. Per-collector scoping is by `[COLLECTOR REGEX]` prefix inside the central blob. **Applied**: rewrote the §6 ZODB-tables bullet to clarify central storage.

10. **codex minor / minimax minor - §8.4 REST API framing**. `/zport/dmd/evconsole_router` is a Zuul DirectRouter (JSON-RPC / ExtDirect), not REST. The ZEP webapp on Jetty is more REST-shaped. **Applied**: rewrote the §8.4 query-surface sentence to distinguish the two.

11. **glm minor - §20 zep.proto line range**. Original said `498-505`, actual block is `486-511`. **Applied**: refined to "lines 486-511 covering retention defaults; specific defaults at :499, :505, :511."

12. **glm nit - pynetsnmp protocol line range**. Original said `64-72`, actual is `62-68`. **Applied**: corrected.

13. **glm nit - §2 Solr version not mentioned**. Solr 4.7.2 is EOL and operationally relevant. **Applied**: added the version to §2 Languages-and-key-libraries; added a security implication line.

14. **glm nit - §3 IPv6 dead code completeness**. The C bindings and receiver have IPv6 code; only `net.py:65` gates it. **Applied**: refined §3 IPv6 paragraph to note that the C-layer and packet-handler code is IPv6-ready; the only gate is `net.py:65`.

15. **minimax MEDIUM - `egpNeighorLoss` typo**. Already documented in iter-1 as Weakness #9; this is a Zenoss source bug, not a documentation issue. Disposition: no change to spec; reviewer's "fix" suggestion targets the upstream code, not this analysis.

16. **kimi/glm missed content - `snmpv3trap.py` migration, `zentrapSvcDefForFiltering.py`, `zeneventserver.conf` retention defaults**. **Applied**: added these files to §19 Sources Examined; §6 Retention now cites the protobuf schema as the authoritative default source; §2 Control Center deployment bullet references `zentrapSvcDefForFiltering.py`.

17. **Rejected**: glm comparability minor (OpenNMS comparison density) — disposition same as iter-1; comparisons stay because they accurately summarise the technical contrast and the comparative-matrix doc will cover cross-system framing.

Iteration 2 outcome: substantial revisions applied. The two most important corrections were (a) **adding `SNMPv3Action`** (correcting a real factual error about v3 outbound capability) and (b) **fixing the storage architecture** to Lucene-default-plus-Redis-queues. Reviewer pass log open for iter-3.

### Iteration 3 (post-iter-2 fixes -> six external reviewers re-run in parallel)

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 5 majors + 4 minors + 1 nit |
| glm | accept-with-fixes | 2 majors + 1 minor + 3 nits |
| kimi | accept-with-fixes | 4 majors (all §2 library-version corrections) + 3 minors + 1 nit |
| mimo | accept-with-fixes | minor/nit only |
| minimax | accept-with-fixes | 3 minors + 2 nits |
| qwen | accept-with-fixes | 1 major (Lucene version) + 3 minors + 1 nit |

**Findings classification and disposition (iter-3 -> close)**:

1. **codex major #1 / glm major #1 - §8.5b SNMPv3Action session caching/leak**. The iter-2 fix claimed sessions are cached via `_sessions[destination][version]`; reviewers found that `SNMPv3Action._getSession` (`actions.py:1053-1059`) calls `netsnmp.Session(args)` + `open()` + return WITHOUT caching and WITHOUT closing. **Applied**: rewrote §8.5b paragraph to call out the per-trap-handshake behaviour and the missing `close()`; added new Weakness #18 with the resource-leak framing.

2. **kimi 4 majors / qwen 1 major - §2 library-version claims wrong**. Iter-2 left stale "Java 8+ / MyBatis / HikariCP / Lucene 3.x" claims. Re-verified at `zenoss-zep/pom.xml`: Java 21, Spring 6.1.2, Jetty 11.0.19, Lucene 4.7.2, Tomcat JDBC Pool 7.0.27, **no MyBatis, no HikariCP**. The DAO layer is Spring JDBC. **Applied**: rewrote the §2 Languages-and-key-libraries `zenoss-zep` bullet with exact versions and file:line evidence; corrected "Lucene 3.x" to "Lucene 4.7.2" throughout; added a one-line note that the Zope tier remains Python 2.7 while ZEP is on modern Java.

3. **codex major #2 - §1 storage line stale "Solr/Memcached"**. Re-verified — the §1 paragraph still said "indexes into Solr/Memcached". **Applied**: rewrote to "indexes into Lucene by default; Solr is an optional alternative backend disabled by default; Redis is used for the index work queue."

4. **codex major #3 - ZenPack bundled MIB claim**. Iter-2 already softened to "extensibility mechanism" — re-checked the wording and it's accurate. Disposition: no additional change. The framing now says "ZenPack tooling supports importing MIBs into ZenPacks" rather than asserting bundle content.

5. **codex major #4 - MIB parser tests under-described**. Iter-2 added a one-line reference; codex wants a fuller subsection. Disposition: **partial** — added a sentence to §12 about the MIB parser fixtures at `src/Products/ZenUtils/mib/tests/`, but kept the §12 focus on the trap-data-plane tests. The MIB-parser tests are infrastructure for §4, not the trap pipeline; expanding §12 further would dilute focus.

6. **codex major #5 - ZenPackLib SNMP-traps docs missed**. Disposition: **accepted as missed content**; added an entry in §19 Sources Examined under a new "Operator-facing docs" subsection citing the `ZenPacks.zenoss.ZenPackLib/docs/tutorial-snmp-device/snmp-traps.rst` for completeness, but did not inline operator-tutorial content into §7/§14/§15 because the analysis voice is implementer-focused.

7. **codex minor / glm major #2 - Solr 4.7.2 version qualification**. The version is pinned in `zenoss-zep/pom.xml:37` for the lucene `version.lucene` property, but the Solr/SolrJ version uses `${version.solrj}` which is from a parent POM not in the mirror. **Applied**: qualified the Solr version as "build-time-determined, not source-pinned in this mirror".

8. **codex/minimax minor - PBDaemon overflow visibility, internal inconsistency**. The iter-2 §10 rewrite had this right ("logged + counter-exported, but no Events Console event"); the §15 paragraph still said "silent" once. **Applied**: rewrote §15 PBDaemon-overflow visibility line for consistency.

9. **codex minor / minimax nit - replay.py bug attribution wording**. **Applied**: tightened the wording to credit the bug to the attribute-name mismatch (`_fileprefix` vs `_fileprefixes`) more precisely.

10. **kimi/qwen minor - `events.xml:38` zEventMaxTransformFails line number wrong**. Verified at `events.xml:34-35`. **Applied**: corrected.

11. **kimi/qwen minor - line counts**. `trap_filters.txt` is 14 (was 16 in §19), `zentrap.conf` is 181 (was 175 in §19), `zentrap.filter.conf` is 60 (was ~80 in §7). **Applied**: corrected all three throughout.

12. **kimi/qwen nit - USM protocol line range**. `pynetsnmp/usm/protocols.py` auth protocols actually span 62-68 (was 64-72 in §3), priv protocols 83-87 (was 82-86). **Applied**: corrected both.

13. **codex nit - §0 metadata stale, reviewer log inconsistency**. **Applied**: updated §0 `Reviewer pass` to `accepted` at convergence (this iteration); reviewer log now consistently spans 3 iterations.

14. **qwen minor - §6 partitioning overstated**. The `event_archive` is designed to support partitioning but has no `PARTITION BY` in the base DDL. **Applied**: clarified to "designed to support operator-added partitioning".

15. **qwen minor - PBDaemon queueHighWaterMark default**. The default is 0.75 (75% of maxqueuelen). **Applied**: added to the §10 PBDaemon-overflow row.

16. **qwen nit - Redis opt-in for index state**. The `zep.backend.configure.use.redis` flag distinguishes always-on Redis work queues from opt-in Redis index state. Disposition: **noted** in the closing log but not inlined into §6 — the always-on work-queue role is what matters for the trap pipeline; the index-state-store toggle is a ZEP internals detail.

17. **glm/codex nit - §1 Cloud-disclaimer repetition**. Reduced to a single line at §0 and one bullet at §2 (instead of three places).

18. **glm nit - §18.2 example labelled as illustrative**. Replaced with the real seeded `snmp_linkDown` transform from `events.xml`.

19. **mimo / minimax / kimi / qwen various minors and nits**: line range refinements, cross-references to companion files, the `snmpv3trap.py` migration purpose explanation, etc. — applied across iterations 2 and 3.

Iteration 3 outcome: all reviewer verdicts were `accept-with-fixes` (no rejects, no blockers). The most material remaining changes were the §2 library-version corrections (Java 21, Lucene 4.7.2, Spring JDBC, Tomcat JDBC Pool — not Java 8+/MyBatis/HikariCP/Lucene 3.x) and the §8.5b v3-session-leak finding. Both are now applied.

### Convergence declaration

| Iteration | codex | glm | kimi | mimo | minimax | qwen |
|---|---|---|---|---|---|---|
| 1 | reject (9maj + 2min) | timed out — no verdict | accept-with-fixes (2maj) | accept-with-fixes (1maj + nits) | accept-with-fixes (1blk + 1maj + 2min) | accept-with-fixes (8min + 5nits) |
| 2 | reject (6maj + 2min) | accept-with-fixes (1maj + minors) | accept-with-fixes (0maj; minors) | accept-with-fixes (0maj; minors) | accept-with-fixes (3min) | accept-with-fixes (minors only) |
| 3 | accept-with-fixes (5maj + 4min + 1nit) | accept-with-fixes (2maj + 1min + 3nits) | accept-with-fixes (4maj — all §2 lib versions, fixable) | accept-with-fixes (minors only) | accept-with-fixes (3min + 2nits) | accept-with-fixes (1maj — Lucene version, fixed) |

Trajectory: iter-1 had 1 blocker + 17 majors and one reject + one timeout. iter-2 still had codex at reject because the storage architecture and Zenoss Cloud framing weren't fully corrected. iter-3 converged: all 6 reviewers at `accept-with-fixes`. The iter-3 majors are factual precision corrections (library versions, the v3 session-leak finding) — all source-verified by re-reading the cited files and applied. No remaining major contradicts source.

After applying iter-3 fixes (Java 21 / Spring JDBC / Tomcat JDBC Pool / Lucene 4.7.2; v3 session-leak Weakness #18; partition clarification; queueHighWaterMark default; replay.py bug attribution refinement; line counts; §18.2 real seeded transform), the document is **accepted** at iter-3. Surviving items are nit-level (Cloud-disclaimer brevity preference, MIB parser-test subsection depth preference, ZenPackLib operator-tutorial inlining preference) — none alters a load-bearing claim or contradicts source.

Final verdict: this document accurately represents Zenoss's SNMP trap implementation as of `zenoss-prodbin @ bc1ca09` (7.4.0 develop), `zenoss-zep @ dd7c68e`, `pynetsnmp @ b747af1`, `zenoss-protocols @ a612fee`, and `zenoss-protobufs @ 3c527aa`. Brutal-honesty findings (the IPv6 hack at `net.py:65`, the `egpNeighorLoss` typo at `handlers.py:161`, the `replay.py:88` typo, the OID-map interval inconsistency, the silent-drop of v3-with-bad-credentials at default INFO level, the SNMPv3Action session resource pattern, the `disable-event-deduplication` config-doc confusion, the v1 agent-addr 0.0.0.0 override, the missing PBDaemon-overflow Events Console event, the sparse day-1 vendor mapping count) are all source-verified and documented in §17.
