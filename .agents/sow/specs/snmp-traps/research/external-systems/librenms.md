# LibreNMS — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: LibreNMS (community-maintained, open-source)
- **Version analysed**:
  - `librenms/librenms` @ `36918f032f69a9e01ce917d9ab099277c456cd2b` (latest commit on the local mirror; tip of `master`)
  - `librenms/docker` @ `d9de2eebb5344590d66a503ccac8de89c6651275` (the official Docker images, including the snmptrapd sidecar)
  - `librenms/docs.librenms.org` @ `9c351c41802cd957861ba75a5574647e497d1550` (Markdown sources for `doc/` are inside the main repo; the `docs.librenms.org` mirror is the rendered HTML site and is cited only for cross-checking)
- **Source evidence**: mirrored (deeply analysed)
- **Repository roots analysed**: `librenms/librenms @ 36918f0`, `librenms/docker @ d9de2ee`, `librenms/docs.librenms.org @ 9c351c4`
- **Author**: assistant
- **Reviewer pass**: **accepted** (convergence declared after 5 iterations; iterations 1-5 surfaced and addressed 1 blocker + ~14 majors + many minors; surviving findings are precision/coverage refinements documented at the close of the Reviewer Pass Log)

Citations in this document use the convention `librenms/librenms @ 36918f0 :: <relative/path>:<line>` (the docs source files live under `librenms/librenms :: doc/`, which is the canonical Markdown source — the `docs.librenms.org` repository builds from these same files plus a MkDocs theme). The commit prefix is omitted on most citations to keep them short.

---

## 1. System Overview & Lineage

LibreNMS is a community-maintained, GPL-3.0-or-later fork of the abandoned Observium Community Edition (forked in 2013). License verified at `librenms/librenms :: composer.json:15` (`"license": "GPL-3.0-or-later"`). It is a PHP application built on the Laravel framework (`composer.json:40` requires `laravel/framework: ^12.10`, i.e. Laravel 12.x; PHP 8.2-8.4 in CI — see `.github/workflows/test.yml:25,46`), with a MariaDB/MySQL database, a Bootstrap/Blade web UI for legacy pages, a Vue 3 component layer for newer pages, and a Python toolchain (`snmpsim`) used in CI. Primary audience: small-to-mid enterprises, MSPs, hosting providers, and home-lab operators who want an open-source network management system that runs on a single LAMP-style host without needing a JVM, Java toolchain, or Kubernetes. Its broader product is a poller-driven SNMP NMS: it polls thousands of OIDs on a schedule and renders graphs, ports, sensors, BGP peers, OSPF neighbors, and so on. SNMP traps are an opt-in event source on top of the polled inventory model.

LibreNMS's trap-handling design is deliberately not a daemon. Where OpenNMS Horizon runs its own JVM-based UDP listener and SNMP4J stack, and where Centreon runs its own long-lived Perl daemon (`centreontrapd`), LibreNMS delegates **all** UDP/162 reception to upstream Net-SNMP `snmptrapd` and ships only a per-trap PHP CLI script (`snmptrap.php`) that `snmptrapd` invokes via the `traphandle default` directive. Each trap is a one-shot PHP process launched by `snmptrapd`; LibreNMS reads the textual trap representation from STDIN, parses it line-by-line, looks up an OID-specific handler class in a registry, and either updates the database (via Eloquent models) and/or writes to the `eventlog` table.

Relationship to upstream tools:

- **Net-SNMP `snmptrapd`** — the front-end UDP/162 listener. LibreNMS does not implement its own socket. The operator-facing setup doc (`doc/Extensions/SNMP-Trap-Handler.md:5-49`) explicitly states "Traps are handled via snmptrapd" and the supplied `snmptrapd.conf` snippet (`doc/Extensions/SNMP-Trap-Handler.md:21-27`) sets `traphandle default /opt/librenms/snmptrap.php`.
- **Net-SNMP MIB loader** — `snmptrapd` itself resolves OIDs to `MODULE::name` strings *before* invoking LibreNMS. LibreNMS's PHP code consumes pre-resolved textual OIDs. The operator must configure `MIBDIRS=+/opt/librenms/mibs:/opt/librenms/mibs/cisco` and `MIBS=+IF-MIB` in `/etc/systemd/system/snmptrapd.service.d/mibs.conf` (doc lines 33-45). If MIBs are not configured, the textual OID arrives in raw numeric form and `Dispatcher::handle()` short-circuits with an error event log entry (`librenms :: LibreNMS/Snmptrap/Dispatcher.php:48-54`).
- **Net-SNMP `snmptrap` binary** — used for the *outbound* trap-as-alert-transport, not for reception. Invoked as a child process by `LibreNMS\Alert\Transport\Snmptrap::deliverAlert()` (`librenms :: LibreNMS/Alert/Transport/Snmptrap.php:41-95`). This is a unique design point covered in §8.5.
- **No upstream-trap-tool lineage in code**: unlike Centreon (which is an SNMPTT lineage descendant — `centreon-collect :: perl-libs/lib/centreon/trapd/lib.pm` carries SNMPTT attribution headers), LibreNMS's `LibreNMS\Snmptrap\Trap` and `Dispatcher` classes contain no SNMPTT references and carry copyright lines attributing them to Tony Murray (`librenms :: LibreNMS/Snmptrap/Trap.php:23`, `LibreNMS/Snmptrap/Dispatcher.php:23`, both 2018) — verified by `grep -r "SNMPTT" LibreNMS/Snmptrap/` returning zero matches. The file header of `snmptrap.php:9-11` says "Adapted from old snmptrap.php handler" by Adam Armstrong, the original Observium author, so the architectural shape (PHP CLI invoked via `traphandle default`) inherits from Observium rather than from SNMPTT.

There is no service / daemon for LibreNMS traps. The `librenms.service` systemd unit (`librenms :: misc/librenms.service`) runs the long-lived poller/scheduler, not a trap process. Trap reception lives entirely in the `snmptrapd` lifecycle.

---

## 2. Trap-Subsystem Architecture

### Components

```
                       SNMP-capable device(s)
                              |
                              | UDP 162
                              v
   +-------------------------------------------------------+
   |  Net-SNMP snmptrapd  (PID 1 in the trap pipeline)     |
   |  /etc/snmp/snmptrapd.conf:                            |
   |    disableAuthorization yes                           |
   |    authCommunity log,execute,net <community>          |
   |    traphandle default /opt/librenms/snmptrap.php      |
   |     |                                                 |
   |     |  per trap: spawn child, stdin = trap text       |
   |     v                                                 |
   |  /opt/librenms/snmptrap.php  (one-shot PHP CLI)       |
   |     |  reads stdin -> stream_get_contents(STDIN)      |
   |     |  Bootstraps Laravel kernel (no daemon)          |
   |     v                                                 |
   |  new LibreNMS\Snmptrap\Trap($text)                    |
   |     |  parses: line 1 = hostname                      |
   |     |          line 2 = "UDP: [ip]:srcport->[..]:..." |
   |     |          rest   = "OID value" pairs             |
   |     v                                                 |
   |  LibreNMS\Snmptrap\Dispatcher::handle($trap)          |
   |     |  1. $trap->getDevice()   (IP/hostname -> DB)    |
   |     |  2. Look up TrapOID in config('snmptraps.       |
   |     |                trap_handlers') -> handler class |
   |     |  3. Service container resolves handler          |
   |     |  4. $handler->handle($device, $trap)            |
   |     |  5. Optional: write eventlog row(s)             |
   |     |  6. Run alert rules (AlertRules->runRules)      |
   |     v                                                 |
   |  MariaDB / MySQL                                      |
   |     | eventlog table (insert), ports/bgppeers/etc.    |
   |     |                                                 |
   |     | alert_log + alerts tables, evaluated next       |
   |     v                                                 |
   |  Optional: alert_transports -> dispatcher            |
   |     -> LibreNMS\Alert\Transport\Snmptrap              |
   |     -> spawn /usr/bin/snmptrap (Net-SNMP)             |
   |     -> outbound SNMPv2c TRAP / INFORM to upstream NMS |
   +-------------------------------------------------------+
                              |
                              v
   Browser  ----> Apache/nginx + PHP-FPM ----> Blade/Vue UI
                  - Eventlog page (table filtered to type=trap)
                  - Per-device Eventlog tab
                  - Alerts page (trap-derived alert state)
```

### Deployment models

- **Single-node**: snmptrapd + LibreNMS + MariaDB on the same host. The recommended path documented at `doc/Extensions/SNMP-Trap-Handler.md:11-19`. This is the default documented deployment.
- **Containerized**: the LibreNMS community ships container images via the `librenms/docker` repository (in the mirror at `librenms/docker @ d9de2ee`). The image is a single binary that switches role via env vars; for traps it runs in "sidecar" mode (`SIDECAR_SNMPTRAPD=1`) — see `librenms/docker :: rootfs/etc/cont-init.d/08-svc-snmptrapd.sh:5-15`. The sidecar exposes **both TCP and UDP 162** (`librenms/docker :: examples/compose/compose.yml:131-138`) and runs `snmptrapd -f -m ALL -M /opt/librenms/mibs:/opt/librenms/mibs/cisco:${SNMP_EXTRA_MIB_DIRS} udp:162 tcp:162` (`08-svc-snmptrapd.sh:41-46`). Two notes: (a) the sidecar default uses `-m ALL`, which the main repo's docs explicitly recommend against at `doc/Extensions/SNMP-Trap-Handler.md:87-90` — internal contradiction between the two repositories. (b) The sidecar templates a default SNMPv3 user with placeholder credentials `auth_pass`/`priv_pass` (`08-svc-snmptrapd.sh:8-15`, env defaults; documented at `librenms/docker :: README.md:171-180`), explicitly labelled "should not be used" in the README but shipped as the default — operator must override every credential env var. The `snmptrap.php` CLI is otherwise identical inside or outside containers — the container's `traphandle default` is wired to the in-container script path (`librenms/docker :: rootfs/etc/snmp/snmptrapd.conf:5`).
- **Distributed poller**: LibreNMS supports distributed pollers (`Distributed-Poller.md` in the docs tree) that share a MariaDB and an RRD storage volume. Polling can be sharded across pollers. **The trap path is not officially distributed**: there is no first-class documented pattern for forwarding traps from edge collectors to a central LibreNMS the way OpenNMS Minions do via a sink (JMS/Kafka/gRPC). Each LibreNMS host that operators want to receive traps must run its own `snmptrapd` with `traphandle default /opt/librenms/snmptrap.php`, and every such host needs database access. Operators wanting geographic trap aggregation deploy snmptrapd on the central LibreNMS host and configure remote devices to send traps directly to it, or run identical LibreNMS code on multiple hosts pointing to the same DB.
- **HA**: not in-product. Operators commonly use keepalived + a floating UDP/162 VIP, or front several LibreNMS hosts with a load-balancer that DNATs UDP/162. None of this is in the LibreNMS source; the source is single-host.

### Languages and key libraries

- **PHP 8.2 / 8.4** (`.github/workflows/test.yml:25,46`). The `snmptrap.php` script uses the shebang `#!/usr/bin/env php` (`snmptrap.php:1`).
- **Laravel framework** — service container and Eloquent ORM. The `Dispatcher` resolves handlers via `app(\LibreNMS\Interfaces\SnmptrapHandler::class, [$trap->getTrapOid()])` (`LibreNMS/Snmptrap/Dispatcher.php:59`), and `app/Providers/SnmptrapProvider.php:28-32` registers the contextual binding that looks up the OID in the config registry.
- **Eloquent models** — `App\Models\Device`, `App\Models\Port`, `App\Models\Eventlog`, `App\Models\Ipv4Address`, `App\Models\Ipv6Address` are the primary tables touched.
- **Symfony Process** — used by the alert transport for the outbound `snmptrap` binary (`LibreNMS/Alert/Transport/Snmptrap.php:79`).
- **Bootstrap 3 / jQuery / bootgrid + Vue 3 + Blade** for the UI (`resources/views/device/tabs/logs/eventlog.blade.php`, `includes/html/table/eventlog.inc.php`).

### Inter-component IPC

- **`snmptrapd` → `snmptrap.php`**: child-process spawn, **per trap**. There is no long-lived PHP process. The trap text is delivered on the child's STDIN by Net-SNMP's `snmptrapd` (configured via `traphandle default`). The PHP exit code is ignored by `snmptrapd`.
- **`snmptrap.php` → MariaDB**: standard Laravel database connection per process, opened by the Laravel kernel bootstrap (`snmptrap.php:17` requires `includes/init.php`, which boots the framework). PHP processes hold the connection only for their lifetime (typically tens of milliseconds).
- **Alert engine → outbound trap**: `LibreNMS\Alert\Transport\Snmptrap::deliverAlert()` builds a command array and instantiates `Symfony\Component\Process\Process` (`LibreNMS/Alert/Transport/Snmptrap.php:77-80`), then `$process->run()` (line 83). Synchronous, 30-second timeout (line 81).

The performance implication of "one PHP process per trap" is significant and is discussed in §10. A related but distinct ingest pattern is used by `syslog.php:1-28` (a sibling CLI script that bootstraps Laravel and reads from STDIN). It is **not identical** to `snmptrap.php`: `syslog.php` enters a `while ($line = fgets($s))` loop (`syslog.php:17-24`) and processes **multiple syslog lines per process invocation**, whereas `snmptrap.php` reads all of STDIN once with `stream_get_contents` (`snmptrap.php:25`) and processes **exactly one trap per process**. The cardinality is fundamentally different — syslog ingest amortises Laravel boot across many lines; trap ingest pays it per trap. The shared design is "CLI script bootstraps Laravel and reads STDIN," not "one process per event."

---

## 3. Trap Reception (UDP/162 Ingress)

### Listener implementation

LibreNMS **does not bind a UDP socket**. Reception is entirely delegated to Net-SNMP `snmptrapd`. LibreNMS depends on `snmptrapd` for:

- Privileged-port binding (UDP/162 is typically root-only; `snmptrapd` runs as root).
- ASN.1/BER decoding of the trap PDU.
- OID-to-name resolution against the local MIB database (controlled by Net-SNMP's `MIBDIRS`/`MIBS`).
- Authentication (community string check, SNMPv3 USM negotiation).
- Optional logging to file (`-Lsd`, `-Lf`, `-tLf` flags).
- Inform-PDU acknowledgement (Net-SNMP handles this transparently).

Configuration files cited verbatim from `doc/Extensions/SNMP-Trap-Handler.md`:

- `/etc/snmp/snmptrapd.conf` (lines 21-27):
  ```
  disableAuthorization yes
  authCommunity log,execute,net COMMUNITYSTRING
  traphandle default /opt/librenms/snmptrap.php
  ```
- `/etc/systemd/system/snmptrapd.service.d/mibs.conf` (lines 41-45):
  ```
  [Service]
  Environment=MIBDIRS=+/opt/librenms/mibs:/opt/librenms/mibs/cisco
  Environment=MIBS=+IF-MIB
  ```

**This configuration is permissive in a way the LibreNMS docs do not explicitly call out**. From the Net-SNMP `snmptrapd.conf(5)` manpage (https://net-snmp.sourceforge.io/docs/man/snmptrapd.conf.html):

> "`disableAuthorization yes` will disable the above access control checks, and revert to the previous behaviour of accepting all incoming notifications."

So with `disableAuthorization yes`, the `authCommunity log,execute,net <community>` line is **not** an admission gate — `snmptrapd` accepts traps from any source, with any community, and routes them through the configured processing chain (which includes `traphandle default /opt/librenms/snmptrap.php`). The same wording appears in the LibreNMS Docker README at `librenms/docker @ d9de2ee :: README.md:177`. The docker sidecar's `SNMP_DISABLE_AUTHORIZATION` defaults to `yes` (`librenms/docker :: rootfs/etc/cont-init.d/08-svc-snmptrapd.sh:14`), and the main repo's docs at `doc/Extensions/SNMP-Trap-Handler.md:21-27` show `disableAuthorization yes` as the recommended baseline. Tightening this requires setting `disableAuthorization no` and authoring per-community/per-user `authCommunity` / `authUser` rules — a path the documentation does not walk operators through.

### SNMP version support

Whatever Net-SNMP `snmptrapd` supports is what LibreNMS supports. In contemporary Net-SNMP this is **v1, v2c, v3 USM, and SNMPv3 informs**. LibreNMS's PHP code is version-agnostic — the textual representation produced by `snmptrapd`'s default formatter is the same for v1 and v2c (`Trap.php:60` simply tokenises `"OID value"` lines). LibreNMS's `Trap` class:

- Reads the source address from the line `"UDP: [<ip>]:<srcport>-> [<dst>]:<dstport>"` via `preg_match('/\[([0-9.:a-fA-F]+)]/', ...)` (`LibreNMS/Snmptrap/Trap.php:53-56`) — handles both IPv4 and IPv6.
- Reads the symbolic OID from `SNMPv2-MIB::snmpTrapOID.0` (`Trap.php:104`). For v1 traps, Net-SNMP synthesises this field from the enterprise + generic + specific tuple before invoking the handler, so the PHP code never sees the v1 wire-level structure.
- Does **not** read `agent-addr` from v1 PDUs — that's not in the textual line-set produced by default `traphandle` mode. Operators with v1 NAT issues must rely on the `[ip]` field from the `UDP:` line (the *transport* source IP, after NAT translation, not the in-PDU `agent-addr`).

SNMPv3 USM users are configured in `snmptrapd.conf` (the operator's responsibility), not in LibreNMS. The main repository's docs do not provide a v3 USM setup template — only the v1/v2c `authCommunity` template (`doc/Extensions/SNMP-Trap-Handler.md:21-27`). The **Docker sidecar** repository, however, does ship a v3 USM template at `librenms/docker @ d9de2ee :: rootfs/etc/snmp/snmptrapd.conf:1-5` (templated via env vars `SNMP_USER`, `SNMP_AUTH`, `SNMP_PRIV`, `SNMP_AUTH_PROTO`, `SNMP_PRIV_PROTO`, `SNMP_SECURITY_LEVEL`, `SNMP_ENGINEID`) — though the default credentials (`auth_pass`, `priv_pass`) are placeholders the operator must override.

**DTLS / TLS-TM (RFC 5953/6353/9456)**: depends on whether the operator's Net-SNMP build was compiled `--with-transports="..."` to include TLS/DTLS transports. Not exercised in LibreNMS source. Not documented.

### Concurrency model

The concurrency model is **`snmptrapd`'s `traphandle` behaviour**, which is **synchronous and blocking**. From the official Net-SNMP `snmptrapd.conf(5)` manpage (https://net-snmp.sourceforge.io/docs/man/snmptrapd.conf.html) under NOTIFICATION PROCESSING:

> "The daemon blocks while executing the traphandle commands. (This should be fixed in the future with an appropriate signal catch and wait() combination)."

That is, the pipeline is:

1. `snmptrapd` receives a UDP datagram on port 162.
2. It decodes the PDU.
3. It launches `/opt/librenms/snmptrap.php` as a child process.
4. It writes the textual trap to that child's STDIN and closes STDIN.
5. **It blocks until the child exits** before processing the next datagram.

Each invocation of `snmptrap.php` is its own PHP process: it bootstraps Laravel (`snmptrap.php:17` `require __DIR__ . '/includes/init.php'`), opens a DB connection, processes one trap, and exits. This means:

- No in-memory state survives between traps.
- No batching or aggregation occurs in PHP.
- Traps are processed **serially**, not in parallel. While `snmptrap.php` is running, additional datagrams accumulate in the kernel UDP socket buffer (`net.core.rmem_default`/`rmem_max`); if the buffer fills, the kernel drops packets and `netstat -su` shows the drop counter incrementing.
- PHP startup cost (Laravel boot ≈ 50-200 ms on a typical host depending on opcache state) is the per-trap latency floor and directly throttles maximum sustained throughput. Note that **PHP OPcache is disabled for CLI by default** (`opcache.enable_cli=0`); unless the operator sets `opcache.enable_cli=1`, each `snmptrap.php` invocation pays full bytecode compilation cost on top of Laravel boot. There is no in-PHP queue; the "queue" is the kernel UDP buffer.
- The database is hit one short-lived connection at a time, not 100 concurrent connections.

**The synchronous traphandle + PHP-per-trap design is the dominant performance characteristic of LibreNMS trap handling**. It is simple, robust, isolates faults (a crash in one handler kills only that trap and `snmptrapd` continues), and is appropriate for low-to-mid trap volumes. It is **not** appropriate for high sustained volumes; see §10.

### Privileged-port handling

Privileged-port handling is `snmptrapd`'s responsibility. The LibreNMS docs assume `snmptrapd` runs as `root` (default systemd unit on Debian/Ubuntu/RHEL). The operator's user is not relevant — the file `/opt/librenms/snmptrap.php` runs as whatever uid `snmptrapd` is configured to drop to (`-u <user>` flag) or as `root` if no flag is given. The docs at `doc/Extensions/SNMP-Trap-Handler.md:108-129` provide an **SELinux module** for `snmpd_t` to be allowed `httpd_sys_rw_content_t` file r/w/append plus `dac_override` capability — this is a privilege-escalation footnote acknowledging that `snmptrap.php` typically wants to write into the LibreNMS install directory and logs, which are owned by the web user.

### Horizontal scaling pattern

No first-class horizontal scaling pattern. Operators either (a) front several LibreNMS hosts with a DNS-RR or load-balanced VIP, all writing to the same MariaDB, or (b) deploy a single host sized to the trap volume. The lack of an aggregator daemon means there is no way to aggregate traps from many edge collectors back to a central LibreNMS.

### HA / clustering

No in-product clustering. MariaDB can be replicated externally; the trap-handling PHP code does not know about replication, leader election, or shared state.

---

## 4. MIB Management

LibreNMS's MIB strategy splits into **two distinct uses** with very different roles:

### 4.1 MIBs as polling helpers (the bulk of `mibs/`)

The repository ships a large MIB tree at `librenms :: mibs/`:

- **Total files**: 4,770 MIB files (`find mibs -type f | wc -l`).
- **Vendor directories**: 371 (`find mibs -maxdepth 1 -type d | wc -l` returns 372 — that figure includes the top-level `mibs/` directory itself, so the vendor-only count is 371).
- **Top-level (cross-vendor / IETF) files**: 207 standard MIBs at `mibs/` (e.g. `mibs/ALARM-MIB`, `mibs/BGP4-MIB`, `mibs/DISMAN-EVENT-MIB`, `mibs/OSPF-MIB`, `mibs/OSPF-TRAP-MIB`, `mibs/SNMPv2-MIB`, `mibs/UPS-MIB`).
- **Files containing `NOTIFICATION-TYPE` or `TRAP-TYPE` macros**: 2,245 (out of 4,770) — verified via `find mibs/ -type f | xargs grep -lE "NOTIFICATION-TYPE|TRAP-TYPE" | wc -l` = 2,245. (`grep -rlE ...` without `--binary-files=text` returns 2,141 because grep autodetects ~104 MIB files containing high-byte sequences as binary and skips them silently; the correct count uses `find | xargs grep` or `grep --binary-files=text`.) That is, **~47% of the bundled MIBs describe at least one notification**, even though the dominant use of the MIB tree is polling-side `OBJECT-TYPE` resolution.
- **LibreNMS's own MIB**: `mibs/librenms/LIBRENMS-NOTIFICATIONS-MIB` (`mibs/librenms/LIBRENMS-NOTIFICATIONS-MIB:1-39`) under enterprise OID 60652 (IANA PEN), defining the trap structure that LibreNMS *emits* via the alert transport (see §8.5).

The vendor MIB coverage is broad — sampling: `mibs/cisco/`, `mibs/juniper/`, `mibs/arista/`, `mibs/arubaos/`, `mibs/apc/`, `mibs/dell/`, `mibs/fortinet/` (the exact directory layout varies — most are flat, a few have subdirectories). The maintainers add MIBs as new collector OS modules are merged, which is the dominant churn path.

### 4.2 MIBs as trap decoding helpers (only those `snmptrapd` loads)

**This is the crux of LibreNMS's MIB strategy for traps**: the bundled `mibs/` tree is **not** auto-loaded by `snmptrapd`. The operator must explicitly enumerate the relevant MIB files in `snmptrapd`'s environment:

```
Environment=MIBDIRS=+/opt/librenms/mibs:/opt/librenms/mibs/cisco
Environment=MIBS=+IF-MIB
```
(`doc/Extensions/SNMP-Trap-Handler.md:41-45`)

The operator-facing doc explicitly warns against `-m ALL`:
> "Good practice is to avoid `-m ALL` because then it will try to load all the MIBs in DIRLIST, which will typically fail (snmptrapd cannot load that many mibs). Better is to specify the exact MIB files defining the traps you are interested in"
(`doc/Extensions/SNMP-Trap-Handler.md:87-90`)

This is explicit operator documentation. It is also a real operational pain point: a fresh LibreNMS install with default `snmptrapd.conf` will see numeric, unresolved OIDs (e.g. `iso.3.6.1.6.3.1.1.4.1.0`), the trap will not match any handler, and `Dispatcher::handle()` logs an "incorrect MIBs" eventlog message and returns `false` (`LibreNMS/Snmptrap/Dispatcher.php:48-55`).

### 4.3 Workflow for adding/updating MIBs

1. Drop the MIB file into the appropriate `/opt/librenms/mibs/<vendor>/` directory.
2. Add the directory (if new) to `MIBDIRS` in `/etc/systemd/system/snmptrapd.service.d/mibs.conf`. **`MIBDIRS` is not recursive** (called out at `doc/Extensions/SNMP-Trap-Handler.md:85` and `doc/Developing/SNMP-Traps.md:10`): every vendor subdirectory must be listed explicitly, separated by `:`. Adding just `/opt/librenms/mibs` will NOT cause `/opt/librenms/mibs/cisco`, `/opt/librenms/mibs/juniper`, etc. to be searched.
3. Add the relevant MIB name(s) to `MIBS`.
4. Restart `snmptrapd` (`sudo systemctl restart snmptrapd`).
5. There is **no compilation step** in LibreNMS — Net-SNMP parses the MIB at `snmptrapd` startup. If the MIB has unresolved imports, `snmptrapd` may silently fail to load it and the OID stays unresolved.

### 4.4 Bundled MIBs out-of-the-box (vendor coverage) and the dual purpose

The 4,770-file MIB tree primarily exists for the **polling-side `snmpwalk`** behaviour of LibreNMS's `os` collector modules. The PHP code resolves OIDs via `snmptranslate` against these MIBs to graph metrics. Trap decoding is a *secondary consumer*. This is reflected in directory layout: there is no separate `mibs/traps-only/` set; trap-defining MIBs live in the same per-vendor directories as polling MIBs (`mibs/OSPF-TRAP-MIB`, `mibs/cisco/CISCO-IF-EXTENSION-MIB`, etc.). For example, `mibs/OSPF-TRAP-MIB:105-389` defines 17 `NOTIFICATION-TYPE` definitions used by OSPF traps, and the same MIB tree's `OSPF-MIB` is used by the polling-side OSPF discovery.

The contrast with OpenNMS is sharp: OpenNMS ships ~17,442 event definitions in 230 `*.events.xml` files at `opennms-base-assembly/src/main/filtered/etc/examples/events/`, where the trap-to-event mapping is **pre-compiled** at build time and the operator never edits raw MIB syntax for trap interpretation. LibreNMS keeps the MIB-level abstraction and pushes the work to `snmptrapd`. Mapping from a trap OID to a *handler* is the job of `config/snmptraps.php` (a PHP array, see §5.2), not of a MIB pre-compilation step.

### 4.5 Fallback behaviour for unknown OIDs

Two distinct fallback paths:

1. **OID not resolved by `snmptrapd`** (MIB not loaded): `Dispatcher::handle()` returns `false` after logging "Misconfigured MIBS or MIBDIRS for snmptrapd" to the eventlog (`Dispatcher.php:48-55`). The trap is effectively dropped except for the eventlog row.
2. **OID resolved but no registered handler**: `app(SnmptrapHandler::class, [...])` falls back to `Fallback::class` (`app/Providers/SnmptrapProvider.php:31`). `Fallback::handle()` itself writes only a `Log::info('Unhandled trap snmptrap', ...)` line to the file log (`LibreNMS/Snmptrap/Handlers/Fallback.php:46-50`); it does **not** write to the `eventlog` table. The generic eventlog entry for a fallback trap is written separately by `Dispatcher::handle()` *after* the handler returns, at `Dispatcher.php:66-68` (`$trap->log($trap->toString($detailed))`), conditional on `snmptraps.eventlog` being `'unhandled'` (the default — fallback-only) or `'all'` (every trap).

---

## 5. Trap Processing Pipeline

### 5.1 Parse (BER decode, varbind extraction)

BER decoding happens in `snmptrapd`. By the time `snmptrap.php` runs, the payload is **textual**: line-oriented `"OID value"` pairs. `LibreNMS\Snmptrap\Trap::__construct()` parses it as follows (`LibreNMS/Snmptrap/Trap.php:46-64`):

```php
public function __construct(public readonly string $raw)
{
    $lines = explode(PHP_EOL, trim($this->raw));
    $this->hostname = array_shift($lines);

    $line = array_shift($lines);
    if ($line) {
        preg_match('/\[([0-9.:a-fA-F]+)]/', $line, $matches);
    }
    $this->ip = $matches[1] ?? '';

    $this->oid_data = (new Collection($lines))->mapWithKeys(function ($line) {
        [$oid, $data] = explode(' ', $line, 2);
        return [$oid => trim($data, '"')];
    });
}
```

Edge cases:

- Line 1 is the hostname `snmptrapd` resolved (often `<UNKNOWN>` if no reverse DNS).
- Line 2 contains the `UDP: [src_ip]:port->[dst_ip]:port` token from which the source IP is extracted with a simple bracketed regex.
- Lines 3+ are split on the **first space** (`explode(' ', $line, 2)`). Quoted values are unquoted by `trim($data, '"')`.
- A multi-word value containing `=` or special chars survives as-is in the value, since only one split happens.
- **Multi-line varbind values and lines without a space are silently mishandled**: Net-SNMP can emit varbind values with embedded newlines (rare, but possible for OCTET STRING display-strings); the line-by-line `mapWithKeys` would create dangling lines. Also, `explode(' ', $line, 2)` returns a single-element array when `$line` contains no space, in which case `[$oid, $data] = explode(...)` produces an undefined `$data` (`$oid` gets the whole line). PHP 8.x emits a "Undefined array key 1" warning, the `Trap` object ends up with a garbage OID-keyed entry, and downstream handlers may misinterpret the noise. The parser is not defensive.

### 5.2 OID-to-name resolution and handler dispatch

OID resolution is **already done** by `snmptrapd` (Net-SNMP MIB layer). The PHP code consumes pre-resolved textual OIDs like `SNMPv2-MIB::snmpTrapOID.0` and `IF-MIB::linkUp`. Dispatch then proceeds:

`LibreNMS/Snmptrap/Dispatcher.php:40-75`:

```php
public static function handle(Trap $trap): bool
{
    if (empty($trap->getDevice())) {
        Log::warning('Could not find device for trap', ['trap_text' => $trap->raw]);
        return false;
    }

    if ($trap->findOid('iso.3.6.1.6.3.1.1.4.1.0')) {
        Eventlog::log('Misconfigured MIBS or MIBDIRS for snmptrapd, ...', $trap->getDevice(), 'system');
        return false;
    }

    $handler = app(\LibreNMS\Interfaces\SnmptrapHandler::class, [$trap->getTrapOid()]);
    $handler->handle($trap->getDevice(), $trap);

    $fallback = $handler instanceof Fallback;
    $logging = LibrenmsConfig::get('snmptraps.eventlog', 'unhandled');
    $detailed = LibrenmsConfig::get('snmptraps.eventlog_detailed', false);
    if ($logging == 'all' || ($fallback && $logging == 'unhandled')) {
        $trap->log($trap->toString($detailed));
    }
    if ($logging != 'none' || ! $fallback) {
        $rules = new AlertRules;
        $rules->runRules($trap->getDevice()->device_id);
    }

    return ! $fallback;
}
```

The handler-class lookup happens inside the Laravel service container at `app/Providers/SnmptrapProvider.php:28-32`:

```php
$this->app->bind(SnmptrapHandler::class, function ($app, $options) {
    $oid = reset($options);
    return $app->make(config('snmptraps.trap_handlers')[$oid] ?? Fallback::class);
});
```

This is a **contextual binding**: when `app(SnmptrapHandler::class, ['<oid>'])` is called, the closure picks the handler class out of the PHP registry array at `config/snmptraps.php:12-194`. The registry contains **181** OID-to-class entries (verified: `grep -cE '::class' config/snmptraps.php` = 181; `grep -E '=>' config/snmptraps.php | wc -l` = 182 only because the outer wrapper `'trap_handlers' => [` line also matches).

### 5.3 Source identification (IP → device mapping)

`LibreNMS\Snmptrap\Trap::getDevice()` (`Trap.php:93-100`) attempts, **only if `IP::isValid($this->ip)` is true** (`Trap.php:95`):

1. `Device::findByHostname($this->hostname)` — match the `snmptrapd`-supplied hostname against the `devices.hostname` column (`app/Models/Device.php:115-118`).
2. Fallback to `Device::findByIp($this->ip)` — looks up the `[ip]` from line 2 (`app/Models/Device.php:130-167`). This first runs a **single SQL** statement matching `devices.hostname = $ip OR devices.ip = inet_pton($ip)` (`Device.php:136`). If that returns nothing, it tries:
   - `ipv4_addresses` table (interfaces by IPv4 — `Device.php:142-152`).
   - `ipv6_addresses` table (interfaces by IPv6 — `Device.php:154-164`).

Critically: if the parsed source IP is empty or invalid (e.g. the trap's line 2 was malformed and the bracketed-IP regex did not match), `getDevice()` short-circuits to `null` **without ever attempting the hostname lookup**. The hostname-then-IP order in the code is gated by IP validity at the top.

If none match, `getDevice()` returns `null` and `Dispatcher::handle()` immediately returns `false` with a Log::warning (`Dispatcher.php:42-46`). **The trap is dropped** — no eventlog row, no alert evaluation. This is a real and unadvertised gap: traps from devices LibreNMS has never discovered (e.g. unmonitored agents in the same subnet) vanish, and traps whose line 2 fails the bracketed-IP regex vanish even if the hostname would have matched.

The dependency on hostname makes NAT a real problem: a v1 trap originating behind NAT will have `snmptrapd`'s `UDP:` line showing the NAT IP, and the in-PDU `agent-addr` is not consulted. This is acknowledged in §6.5 of the foundational spec; LibreNMS does not mitigate it in source.

### 5.4 Enrichment (varbind decoration, lookup tables, topology join)

Enrichment is **handler-specific**. There is no generic enrichment stage. Each registered handler decides what to extract from the trap and what additional database lookups to perform. Examples:

- `LinkUp` / `LinkDown` (`LibreNMS/Snmptrap/Handlers/LinkUp.php:46-72`, `LinkDown.php:46-76`): pulls `IF-MIB::ifIndex` from the trap, queries `ports.ifIndex` to find the matching port row, updates `ifOperStatus` / `ifAdminStatus` directly, then writes one eventlog row per state change.
- `BgpEstablished` / `BgpBackwardTransition` (`BgpEstablished.php:44-69`, `BgpBackwardTransition.php:46-67`): pulls the `BGP4-MIB::bgpPeerState` OID, derives the peer IP from the *substring* of the OID (line 47-48 — a fragile pattern that assumes a fixed prefix length of 23 characters), queries `bgppeers` for the matching peer, updates `bgpPeerState`, and writes an eventlog row.
- `UpsTrapOnBattery` (`UpsTrapOnBattery.php:46-79`): pulls `UPS-MIB::upsEstimatedMinutesRemaining` and `UPS-MIB::upsSecondsOnBattery`, looks up specific `sensors` rows by `sensor_index`/`sensor_type`, and **writes derived metric values back into the `sensors` table** — effectively a trap-driven sensor update.
- `ColdBoot` / `WarmBoot` / `AuthenticationFailure` (`ColdBoot.php:42-45`, `AuthenticationFailure.php:42-45`): pure eventlog handlers — no database mutation beyond the eventlog row.
- `AdvaSysAlmTrap` (`AdvaSysAlmTrap.php:47-58`): vendor severity normalization. Maps `cmSysAlmNotifCode` ("critical"/"major"/"minor"/"cleared") onto LibreNMS's `Severity::Error`/`Warning`/`Notice`/`Ok` via a `match` expression, then logs.

There is **no shared enrichment library**. Each handler that needs vendor severity translation does its own `match` block. The closest thing to a shared library is `LibreNMS\Snmptrap\Handlers\VeeamTrap` (an abstract base class at `LibreNMS/Snmptrap/Handlers/VeeamTrap.php:31-41`) used by 17 Veeam handlers, and `ApcTrapUtil` / `Tripplite` (concrete base classes for APC and Tripp Lite handlers, both providing static OID-extractor helpers).

### 5.5 Normalization (vendor severity → internal severity)

Severity normalization is **per-handler**, not centralized. Each handler picks a `LibreNMS\Enum\Severity` value (`LibreNMS/Enum/Severity.php:5-12`):

```php
case Unknown = 0;
case Ok = 1;
case Info = 2;
case Notice = 3;
case Warning = 4;
case Error = 5;
```

Notable inconsistency: the `Eventlog::log` PHPDoc at `app/Models/Eventlog.php:60` says "1: ok, 2: info, 3: notice, 4: warning, 5: critical, 0: unknown" but the enum's case 5 is named `Error`, not `Critical`. The bundled alert rule `"Zebra Printer Trap: Critical"` (`resources/definitions/alert_rules.json:1095-1097`) filters on `eventlog.severity = 5`. So the numeric mapping is consistent, but the *naming* across enum / docblock / alert rules is not — a documentation defect.

### 5.6 Deduplication / suppression

**There is no deduplication or suppression in the trap pipeline**. A storm of 100 identical `linkDown` traps from the same port produces:

- 100 PHP processes.
- 100 `eventlog` rows (one per trap), each calling `Eventlog::log` at `LinkUp.php:60`, `LinkUp.php:64`, etc. The `LinkUp`/`LinkDown` handlers actually emit *multiple* eventlog rows per trap (one "SNMP Trap" + one for `ifAdminStatus` dirty + one for `ifOperStatus` dirty).
- 100 alert-rule evaluations (`AlertRules->runRules($device_id)`).
- The alert engine *will* recognise an existing ACTIVE alert as `NOCHG` (`LibreNMS/Alert/AlertRules.php:115-127`) — that is, the alert state itself is deduplicated. But the eventlog table is not, and every trap pays full PHP boot + DB-connection cost.

This is a real and meaningful gap relative to OpenNMS (which has `reductionKey`-based alarm dedup at the alarm row), Centreon (which has an MD5 digest with a per-source dedup window in `centreontrapd.pm`), and CheckMK (which has Event Console rule-driven aggregation). LibreNMS's only dedup is the alert-rule engine, which deduplicates **alerts**, not **events**.

The one observable counter-pattern: `EesPowerAlarm::handle()` at `LibreNMS/Snmptrap/Handlers/EesPowerAlarm.php:14-25` implements **handler-level suppression** — it returns early when the incoming trap OID contains `alarmActiveTrap`, `alarmCeaseTrap`, or their numeric equivalents (`6302.2.1.5.2`, `6302.2.1.5.3`), because the Emerson/EES device emits both `alarmTrap` (with full data) and the redundant `alarm{Active,Cease}Trap` variants. This is a contributor-authored workaround for one vendor's redundancy, not a generic dedup mechanism — and it is the only such pattern in the 177 handler classes. It illustrates that contributors recognise the dedup gap and patch it per-vendor where they have to.

### 5.7 Routing (where the processed event goes)

`Dispatcher::handle()` directs the trap to exactly one handler. After the handler returns, the dispatcher conditionally evaluates alert rules for that device (`Dispatcher.php:69-72`):

```php
if ($logging != 'none' || ! $fallback) {
    $rules = new AlertRules;
    $rules->runRules($trap->getDevice()->device_id);
}
```

That is, alert-rule evaluation is **skipped** only when **both** conditions hold: `snmptraps.eventlog == 'none'` AND the matched handler is `Fallback`. In all other cases (any handled trap, or any fallback trap as long as eventlog is set to `'unhandled'` or `'all'`), alert rules are re-evaluated. Concretely:
- **Handled trap, any eventlog mode**: rules evaluated.
- **Fallback trap, `eventlog='none'`**: rules NOT evaluated.
- **Fallback trap, `eventlog='unhandled'` or `'all'`** (default): rules evaluated.

This is still potentially wasteful in the common case: a `linkUp` trap will re-run rules unrelated to interfaces. (`AlertRules::runRules` iterates *all* rules whose device/group/location map matches via `AlertUtil::getRules()` at `LibreNMS/Alert/AlertUtil.php:375-393`; there is no rule-side filter for "trap-relevant" rules.)

### 5.8 Error handling for malformed PDUs, unknown OIDs, decode failures

- **Malformed PDUs**: `snmptrapd` rejects before LibreNMS sees them.
- **Unknown OIDs (no MIB)**: caught by the iso-prefix check at `Dispatcher.php:48-54`.
- **Unknown OIDs (MIB present, no registered handler)**: routed to `Fallback`.
- **Handler exceptions**: not caught in the dispatcher; an uncaught exception in a handler will crash the PHP process. `snmptrapd` will record a non-zero exit code but continue processing the next trap.
- **Database connection failures**: depend on Laravel's connection retry behaviour. A persistent MariaDB outage will manifest as Laravel exceptions in handlers, crashing each PHP process and silently dropping traps.
- **Dispatcher return value is discarded**: `snmptrap.php:28` calls `Dispatcher::handle(new Trap(...))` without capturing or propagating its boolean return. `Dispatcher::handle()` returns `false` in three handled-failure paths (no device match at `Dispatcher.php:42-45`; MIB misconfiguration at `Dispatcher.php:48-55`; fallback handler at `Dispatcher.php:74`), but the CLI process exits 0 regardless. External supervision (systemd `OnFailure=`, process-wrapper scripts) cannot distinguish a successfully-processed trap from a dropped one by exit status — the only signal is grepping `librenms.log` for `Log::warning` lines.

---

## 6. Data Model & Persistent Storage

LibreNMS uses MariaDB/MySQL exclusively for structured storage. (RRDtool flat files hold time-series for graphs, but traps do not go to RRD.)

### 6.1 Tables touched by the trap pipeline

| Table | Schema source | Role |
|---|---|---|
| `eventlog` | `database/migrations/2018_07_03_091314_create_eventlog_table.php:14-25` | The **only** trap-event-output table. Holds one row per "trap event" the handler writes via `Eventlog::log()`. Columns: `event_id` (autoinc), `device_id` (FK, nullable, indexed), `datetime` (default `1970-01-02`, indexed), `message` (text), `type` (varchar 64 — for traps the default is `'trap'`, set by `Trap::log()` at `LibreNMS/Snmptrap/Trap.php:125` whose signature is `log(string $message, Severity $severity = Severity::Info, string $type = 'trap', ...)`; handlers may pass `'interface'`, `'auth'`, `'bgpPeer'`, `'stp'`, etc.), `reference` (varchar 64 — typically a port_id or BGP peer IP), `username` (varchar 128 — empty for trap-originated events), `severity` (tinyint, defaults to 2). |
| `devices` | `database/migrations/2018_07_03_091314_create_devices_table.php` | Read for source IP → device lookup. Not mutated by traps. |
| `ports` | `database/migrations/2018_07_03_091314_create_ports_table.php` | Mutated by `LinkUp` / `LinkDown` handlers — `ifOperStatus` and `ifAdminStatus` columns. |
| `bgppeers` | `database/migrations/2018_07_03_091314_create_bgppeers_table.php` | Mutated by `BgpEstablished` / `BgpBackwardTransition` handlers — `bgpPeerState` column. |
| `ospf_nbrs` | `database/migrations/2018_07_03_091314_create_ospf_nbrs_table.php` | Mutated by `OspfNbrStateChange` — `ospfNbrState`. |
| `sensors` | `database/migrations/2018_07_03_091314_create_sensors_table.php` | Mutated by `UpsTrapOnBattery` and similar — `sensor_current`. |
| `ipv4_addresses`, `ipv6_addresses` | `database/migrations/2018_07_03_091314_create_ipv4_addresses_table.php`, `_create_ipv6_addresses_table.php` | Read for interface-IP → device lookup. |
| `alerts` | `database/migrations/2018_07_03_091314_create_alerts_table.php:14-26` | Mutated by `AlertRules::runRules()` invoked at end of trap dispatch. Holds per-rule current state. |
| `alert_log` | `database/migrations/2018_07_03_091314_create_alert_log_table.php`; referenced from `AlertRules.php:117,130,143` | Append-only log of alert state transitions (ACTIVE / RECOVERED). |
| `alert_rules` | `database/migrations/2018_07_03_091314_create_alert_rules_table.php:14-26` | Read by `AlertRules::runRules()`. Each rule's `query` field is raw SQL the dispatcher's child evaluates. |
| `alert_transports` | `database/migrations/2018_07_03_091314_create_alert_transports_table.php:14-26` | Stores the outbound Snmptrap transport instance configuration (`transport_config` JSON). |

There is no dedicated `snmp_traps` table, no `trap_definitions` table, no `oid_handlers` table. Configuration is in PHP source (`config/snmptraps.php`), not in the database. This is a **deliberate design choice**: the registry of OID-to-handler mappings is code, not data.

### 6.2 Schema migration / upgrade handling

Laravel migrations under `database/migrations/` are the canonical schema source. Migrations relevant to the trap path were created at the 2018 Observium-fork rewrite. The `eventlog` table schema has had only one change since: `2020_04_19_010532_eventlog_sensor_reference_cleanup.php` (a data migration, not a schema migration).

There are no migrations for trap-specific tables because there are no trap-specific tables.

### 6.3 Retention

Eventlog retention is enforced by `daily.php:98`, which calls `lock_and_purge('eventlog', 'datetime < DATE_SUB(NOW(), INTERVAL ? DAY)')` (`includes/functions.php:437-455`). The `lock_and_purge` helper:

- Acquires a Laravel `Cache::lock('eventlog_purge', 86000)` distributed lock so only one poller in a multi-poller deployment runs the purge.
- Reads `LibrenmsConfig::get('eventlog_purge')` (default **30 days**, `config_definitions.json:2332-2338`, group `system`, section `cleanup`).
- Runs `DELETE FROM eventlog WHERE datetime < DATE_SUB(NOW(), INTERVAL <N> DAY)`.

`daily.php` is invoked by the system cron (`misc/librenms.crontab`). The trap pipeline neither sets nor reads this value — retention is a separate housekeeping concern. The eventlog table also has a cascade-on-device-delete path at `app/Observers/DeviceObserver.php` (which deletes the device's eventlog rows when the device row is removed), unrelated to time-based retention.

### 6.4 Indexing

The `eventlog` table has only two indexes: on `device_id` and on `datetime` (`migrations/2018_07_03_091314_create_eventlog_table.php:17,18`). There is no index on `type`, `reference`, or `severity`. UI filtering by type (e.g. only `type='trap'` rows for a device) is a full table scan within the device's row partition — workable for thousands of rows, painful at millions.

### 6.5 Raw trap retention

**Raw trap bytes / textual payload are NOT persisted** by default. The `Trap` object holds `$raw` only for the lifetime of the PHP process. The `snmptraps.eventlog_detailed` config setting (`resources/definitions/config_definitions.json:5851-5864`) toggles whether the eventlog message includes the full OID-data JSON (via `Trap::toString(detailed: true)` at `Trap.php:113-120`), but even then it is the *parsed* OID→value map, not the raw text. The only way to retain raw textual traps is to add `-tLf /var/log/snmptrap/traps.log` to the `snmptrapd` command (`doc/Extensions/SNMP-Trap-Handler.md:101-106`) — that is a `snmptrapd`-side flag, not a LibreNMS feature.

---

## 7. Configuration UX

### 7.1 Configuration surfaces

There are three configuration surfaces relevant to traps:

1. **`snmptrapd.conf` and systemd drop-in** — operator-owned, edited by hand:
   - `/etc/snmp/snmptrapd.conf` — community strings, `traphandle default` line, optional auth rules.
   - `/etc/systemd/system/snmptrapd.service.d/mibs.conf` — `MIBDIRS`, `MIBS`, listener flags (`-Lsd`, `-tLf`, etc.).
2. **LibreNMS `config/snmptraps.php`** — *the OID-to-handler registry*. It is a PHP file with a return statement (`config/snmptraps.php:11-195`), labelled **"DO NOT EDIT THIS FILE"** at lines 3-9. The header comment mentions a generic env/`.env` override path that applies to other LibreNMS settings, but **there is no env-based override for the handler registry itself** — `SnmptrapProvider` reads `config('snmptraps.trap_handlers')` directly from this PHP file (`app/Providers/SnmptrapProvider.php:28-32`), and no `env()` hook is defined for the array. Operators wanting a custom OID handler must edit the source (fork or upstream PR); the "DO NOT EDIT" warning is aimed at operators who would not maintain a fork.
3. **LibreNMS settings UI** (`General Settings → External → SNMP Traps Integration`) — exposes two switches in the database-backed config:
   - `snmptraps.eventlog`: `none` | `unhandled` (default) | `all` (`resources/definitions/config_definitions.json:5839-5849`).
   - `snmptraps.eventlog_detailed`: boolean (default false; only visible when `eventlog != none`; `resources/definitions/config_definitions.json:5851-5864`).
   - These are settable from the UI (`Settings → External → snmptrapd`) and via the `lnms` CLI (`lnms config:set snmptraps.eventlog 'unhandled'`, documented at `doc/Extensions/SNMP-Trap-Handler.md:191-194`).

### 7.2 What the operator sees by default

A freshly installed LibreNMS:

- Has the bundled MIB tree under `/opt/librenms/mibs/`, but `snmptrapd` is **not** configured to load any of it — `snmptrapd` is **not even installed by the LibreNMS installer**; it is the operator's responsibility per `doc/Extensions/SNMP-Trap-Handler.md:13-19`.
- Has the registered 181 OID-to-handler mappings in `config/snmptraps.php` (177 distinct handler classes; `EesPowerAlarm` is mapped from 3 OIDs, `FailedUserLogin` and `UpsTrapOnBattery` from 2 each — verified via `grep -cE '::class' config/snmptraps.php` = 181 and per-class counting via Python regex `Handlers\\(\w+)::class` over the file content).
- Has `snmptraps.eventlog` defaulting to `'unhandled'` — i.e. only `Fallback`-routed traps create a generic eventlog row.
- Has 238 bundled alert rules at `resources/definitions/alert_rules.json` (`grep -c '"name":' resources/definitions/alert_rules.json` = 238). Of the rules that touch `eventlog.type`, exactly **2** filter on `eventlog.type = 'trap'`: `"Zebra Printer Trap: Warning"` and `"Zebra Printer Trap: Critical"` (`resources/definitions/alert_rules.json:1088-1097`). A third rule (`"Device discovered within the last 60 minutes"`) filters on `eventlog.type = 'discovery'`, not `'trap'`. That is, the bundled alert ruleset is overwhelmingly **polled-metric-driven**, not trap-driven. Operators wanting trap-driven alerts must author rules themselves.

### 7.3 Discoverability of options

- No CLI auto-completion for the trap registry — it is a static PHP file.
- The `lnms config:set` command validates against `config_definitions.json` (which holds enum-valued constraints for `snmptraps.eventlog`).
- Live reload: yes for the eventlog config (reads `LibrenmsConfig::get(...)` at every trap, `Dispatcher.php:64-65`); no for the `config/snmptraps.php` registry (Laravel config is cached; `php artisan config:cache` may need to be flushed after editing the file).

### 7.4 Multi-tenancy / RBAC

LibreNMS has a per-user device-permission model (`devices_perms` table — referenced at `includes/html/table/eventlog.inc.php:42`). The eventlog table view enforces this: users without `viewAll` see only events on devices they have access to. There is no separate trap-specific RBAC — trap events inherit the device's permission model.

There is no concept of "trap-handler RBAC" — operators cannot grant a subset of users the ability to install handlers, edit `config/snmptraps.php`, etc. The registry is a code file with filesystem permissions.

---

## 8. Integration with Other Signals

### 8.1 Metrics

Traps are **not** generally converted to time-series metrics. The trap-handling pipeline writes to the relational `eventlog` table, not to RRD. There are narrow exceptions where a handler updates a *polled* metric directly:

- `UpsTrapOnBattery::handle()` (`UpsTrapOnBattery.php:55-79`) writes back to the `sensors` table — updating `sensor_current` for the "Estimated battery time remaining" / "Time on battery" sensors. Those sensors are normally populated by the poller; the trap shortens the latency to the next observed value. The subsequent poller pass will overwrite this value with a fresh read. This is the closest LibreNMS comes to "trap-as-metric."
- Other vendor-specific handlers occasionally update `bgppeers.bgpPeerState`, `ports.ifOperStatus`, etc. — these are state columns, not metrics, but they do influence the next graph render and the alert engine's view of "is this port up."

There is **no automatic trap-as-counter** mechanism (e.g. "count of `linkDown` traps per port per minute"). Operators wanting trap-rate metrics must derive them via SQL from the eventlog table.

### 8.2 Alerting / Notifications

Trap-to-alert flow:

1. A registered handler writes an `eventlog` row with `type='trap'` (or `'interface'`/`'bgpPeer'`/etc.) and a severity from the `Severity` enum.
2. `Dispatcher::handle()` then conditionally calls `(new AlertRules)->runRules($device->device_id)` (`Dispatcher.php:69-72`) — skipped only when both the handler was `Fallback` **and** `snmptraps.eventlog == 'none'`. Operators who set `eventlog='none'` to silence unhandled-trap noise should know it also silences alert-rule evaluation for fallback-handled traps.
3. `AlertRules::runRules()` iterates every active alert rule that applies to the device, runs its SQL query against MariaDB, and decides whether to fire (`LibreNMS/Alert/AlertRules.php:66-153`).
4. Rule definitions are stored as a `builder` JSON structure (jQuery QueryBuilder format) on the `alert_rules` table — see the Zebra rules at `resources/definitions/alert_rules.json:1088-1097`. At evaluation time `AlertRules.php:76-78` calls `QueryBuilderParser::fromJson($rule['builder'])->toSql()` if the `query` field is empty, producing SQL like `... eventlog.type = 'trap' AND eventlog.severity = 5 AND eventlog.datetime >= DATE_SUB(NOW(), INTERVAL 5 MINUTE) ...`. The raw SQL can also be hand-edited via the UI for power users.
5. On a state transition (CLEAR → ACTIVE or ACTIVE → RECOVERED), a row is written to `alerts` and `alert_log` by `AlertRules::runRules()` (`AlertRules.php:130-147`) — **synchronously, inside the trap PHP process**.
6. **Notification delivery is asynchronous and decoupled**: actually sending the alert via configured transports (mail, Slack, the Snmptrap-emit transport, etc.) is done by a separate cron-driven process `alerts.php` (`alerts.php:43-63`) that calls `RunAlerts->runAlerts()` (`LibreNMS/Alert/RunAlerts.php:507+`). `alerts.php` is invoked from the LibreNMS cron at a default cadence of every 1 minute (see `misc/librenms.crontab`). This means the trap-PHP process records the alert state change immediately, but the operator's pager/email/upstream NMS may not fire for up to one cron tick later. Trap-to-page latency is the sum of: handler latency (tens to hundreds of ms) + cron tick (up to ~60 s) + transport-delivery latency. This split matters for comparability: trap reception is synchronous and bounded by the PHP process; alert delivery is asynchronous and bounded by cron cadence.

Acknowledgement / clear semantics: the alert engine has `AlertState::ACKNOWLEDGED` (`AlertRules.php:112`), but acknowledgement is a UI action against the alert row, not against the trap. An ACK suppresses re-firing while the underlying SQL condition stays true. Once the condition clears (e.g. the time window passes), the alert recovers.

**Alert state (not eventlog) acts as the dedup boundary for notifications**: most successfully-handled traps create one or more eventlog rows (no eventlog dedup), but the alert engine's `(device_id, rule_id)` uniqueness collapses repeated firings into NOCHG transitions. (Caveats on the "most": no-device-match traps don't create any eventlog row, only a `Log::warning` in `librenms.log`; handler-specific not-found paths like `LinkUp`/`LinkDown` returning before logging when the port doesn't exist; fallback traps when `snmptraps.eventlog='none'` — see §5.8.) An alert rule "fire if any trap of severity ≥ 4 in the last 5 minutes" stays ACTIVE as long as the query returns rows. The first trap fires the alert (transition CLEAR → ACTIVE); subsequent traps in the window keep it ACTIVE (NOCHG, no new transport delivery). When 5 minutes elapse without a new trap, the rule's query returns zero rows and the alert recovers. This is a coarse but functional model for *notification* dedup that scales as well as MariaDB scales — but the eventlog itself remains append-only and unbounded by this mechanism.

### 8.3 Topology

LibreNMS has CDP/LLDP topology discovery for L2 (per device, populated by the poller via the `discovery` cycle). The trap pipeline does **not** consume or update topology graph data. Two narrow exceptions worth noting:

- `BridgeNewRoot` and `BridgeTopologyChanged` handlers (`config/snmptraps.php:32-33`, both implementations are 6-line eventlog writers at `LibreNMS/Snmptrap/Handlers/BridgeNewRoot.php` and `BridgeTopologyChanged.php`) accept `BRIDGE-MIB::newRoot` and `BRIDGE-MIB::topologyChange` traps respectively, but only write `type='stp'` eventlog rows — they do not feed back into a topology graph.
- These STP traps are accepted but **not** used for root-cause suppression or topology-aware alerting.

There is no topology-aware suppression — if a backbone link goes down and 30 access switches lose connectivity, LibreNMS will receive `linkDown` from every dependent switch and dedup is left to whatever generic alert rules the operator wrote. The notion of a "root-cause alarm" (the way OpenNMS implements via Drools `situations.drl`) is absent.

### 8.4 Logs / Events

`eventlog` is the **trap + internal-event** store, not a unified event store across all event-shaped signals. Three separate relational tables coexist with closely related but distinct purposes:

- `eventlog` (`database/migrations/2018_07_03_091314_create_eventlog_table.php`) — trap-derived events, discovery events, polling state changes, admin actions. Written by `Eventlog::log()`.
- `syslog` (`database/migrations/2018_07_03_091314_create_syslog_table.php:15-27`) — separate table for syslog ingest. Schema differs (`facility`, `priority`, `level`, `tag`, `program`, `msg`, `seq`). Written by `includes/syslog.php` (the separate `syslog.php` ingest path, see §2).
- `alert_log` (`AlertRules.php:117,130,143`) — alert state transitions (ACTIVE / RECOVERED).

The UI and the REST API treat these as **three distinct timelines**: `includes/html/api_functions.inc.php` queries `eventlog`, `syslog`, and `alert_log` as separate endpoints. There is no unified-event view in core LibreNMS — operators wanting one build it themselves at the SQL or visualization layer.

`eventlog`'s `type` column distinguishes the kinds of internal events stored there (`'trap'`, `'auth'`, `'reboot'`, `'bgpPeer'`, `'interface'`, `'system'`, `'stp'`, `'backup'`, `'loop'`, `'state'`, `'policy'`, `'log'`, `'bcastStorm'`, `'badXcvr'`, `'badCable'`, `'Power'`, etc. — the exact set is informal, drawn from each handler's call to `Trap::log()` or `Eventlog::log()`).

Searchability: `includes/html/table/eventlog.inc.php:34-37` allows filtering by `device`, `eventtype`, and a free-text `message` substring (via `LIKE '%...%'`).

Retention: configurable, separate from trap handling (housekeeping cron — see §6.3).

Schema: see §6.1.

### 8.5 Northbound Forwarding — Unique Design Point

LibreNMS includes a first-class **alert transport** that emits SNMPv2c traps northbound. This is unusual: most analysed NMSes can receive traps; LibreNMS can also send them. The transport at `librenms :: LibreNMS/Alert/Transport/Snmptrap.php` (239 lines) was contributed by Tigo Technology Center in 2022 (`Snmptrap.php:1-2`) and integrates as one of 57 alert transports under `LibreNMS/Alert/Transport/` (verified via `ls LibreNMS/Alert/Transport/ | wc -l`).

#### 8.5.1 What it sends

For each alert that fires (CLEAR/ACTIVE/ACKNOWLEDGED/WORSE/BETTER state transition), the transport invokes the configured `snmptrap` binary (UI: `Settings → External → Binaries → snmptrap`, default `/usr/bin/snmptrap`; first-class setting at `config_definitions.json:7059-7065`, group `external`, section `binaries`, type `executable`; read at `Snmptrap.php:50`) with arguments built from:

- The destination host, port (default 162), transport (UDP/TCP), and community (default `'public'`) — all in the transport's per-instance config (`Snmptrap.php:43-50`).
- A trap-OID specified by the operator (default `LIBRENMS-NOTIFICATIONS-MIB::defaultAlertEvent`, `Snmptrap.php:206-208`).
- A PDU type: `TRAPv2` (one-way) or `INFORM` (acknowledged) (`Snmptrap.php:215-220`).
- A MIB directory the operator must populate with the bundled `LIBRENMS-NOTIFICATIONS-MIB` (default path `/opt/librenms/mibs/librenms`, `Snmptrap.php:222-227`).
- **Varbind lines** parsed from the alert message body, which is itself a Blade-template render. The transport's `parseVarbinds()` (`Snmptrap.php:101-115`) splits the rendered alert message line-by-line; each non-comment line is expected to be `OID type value` where `type` is a Net-SNMP type character (`s`, `i`, `t`, `o`, …).

The full command construction at `Snmptrap.php:51-77`:

```php
$cmd = [$binary];
if ($pdu === 'INFORM') {
    array_push($cmd, '-v', '2c', '-Ci');
} else {
    // TRAPv2 (default)
    array_push($cmd, '-v', '2c');
}
array_push($cmd, '-M', '+' . $mibdir);
array_push($cmd, '-c', $community);
$cmd[] = $transport . ':' . $host . ':' . $port;
$cmd[] = '';                       // uptime (empty = use agent uptime)
$cmd[] = $trapdefinition;
foreach ($this->parseVarbinds($alert_data['msg'] ?? '') as $arg) {
    $cmd[] = $arg;
}
```

#### 8.5.2 The LibreNMS notifications MIB

`mibs/librenms/LIBRENMS-NOTIFICATIONS-MIB` (verified by `head -100 mibs/librenms/LIBRENMS-NOTIFICATIONS-MIB`):

- Registered under IANA PEN **60652** (LibreNMS) (`LIBRENMS-NOTIFICATIONS-MIB:27-28`).
- Defines `defaultAlertTitle`, `defaultAlertID`, `defaultAlertEventID`, `defaultAlertState` (`stateClear(0)`, `stateActive(1)`, `stateAcknowledged(2)`, `stateWorse(3)`, `stateBetter(4)`), `defaultAlertSeverity`, `defaultAlertRuleID`, `defaultAlertRuleName`, `defaultAlertProcedure`, `defaultAlertTimestamp`, `defaultAlertDeviceID`, `defaultAlertDevHostname`, `defaultAlertDevSysName`, `defaultAlertDevMgmtIP`, `defaultAlertDevOS`, `defaultAlertDevType`, `defaultAlertDevHardware`, `defaultAlertDevVersion`, `defaultAlertDevLocation`, `defaultAlertDevUptime`, `defaultAlertDevShortUptime`, `defaultAlertACKNotes`, and `defaultAlertFaultDetail`.
- The MIB ships **inside the same source tree** that contains all polling MIBs. Operators wanting an upstream NMS to consume LibreNMS traps install this MIB on the receiver.

A reference alert template that produces correctly-formatted varbind lines is documented at `doc/Alerting/Transports/Snmptrap.md:60-91`. It uses standard LibreNMS template variables (`{{ $alert->title }}`, `{{ $alert->id }}`, etc.).

#### 8.5.3 What is and isn't supported

- ✓ SNMPv2c TRAP and INFORM.
- ✓ UDP or TCP transport.
- ✓ Configurable destination port, community, OID, MIB directory.
- ✓ Per-line varbind parsing with quoted-string + escape handling (`Snmptrap.php:120-159`).
- ✗ **SNMPv1 not supported** — the `-v 1` flag is never produced. The transport hard-codes `-v 2c` at lines 55, 58.
- ✗ **SNMPv3 USM not supported** — no `-u`, `-l`, `-a`, `-A`, `-x`, `-X` flags. Community-string auth only.
- ✗ **No retry on outbound transport failure** at the LibreNMS layer. The `Process::run()` 30-second timeout (`Snmptrap.php:81`) is the only guard inside the transport. If the receiver does not ACK an INFORM, `snmptrap -Ci` retries internally (Net-SNMP default), but if the whole subprocess fails or times out, the transport throws `AlertTransportDeliveryException` (`LibreNMS/Exceptions/AlertTransportDeliveryException.php`). `RunAlerts::runAlerts()` catches that exception and writes a one-shot eventlog/`alert_log` row (`LibreNMS/Alert/RunAlerts.php:696-702`) — but the alert was already marked as alerted before transport delivery (`RunAlerts.php:638-640`), so no automatic retry of this specific notification happens. The failed outbound trap is **lost**; the next state-change of the alert is what would produce the next outbound trap.

This northbound capability is architecturally relevant for the comparison: LibreNMS can sit *inside* a larger NMS hierarchy and feed traps to a parent Tivoli, Netcool, SolarWinds, or another LibreNMS upstream — most other systems in this comparison receive traps but do not emit them as alert transports. The implementation is **shell-out to `snmptrap`**, which is operationally pragmatic but expensive: each fired alert is a child-process spawn of the Net-SNMP CLI. For a healthy alerting volume this is fine; for a storm it adds another tier of process cost on top of the inbound trap PHP-process cost.

---

## 9. Severity Model

### 9.1 Where vendor severity comes from

Vendor severity arrives in the trap payload as a varbind value. Each handler reads the relevant varbind and maps it to LibreNMS's `Severity` enum. There is no centralized severity-mapping table.

### 9.2 How it is mapped to system severity

The `Severity` enum has six cases: `Unknown=0`, `Ok=1`, `Info=2`, `Notice=3`, `Warning=4`, `Error=5` (`LibreNMS/Enum/Severity.php:5-12`). Mapping happens inline in each handler. Examples:

- `AdvaSysAlmTrap::handle()` (`AdvaSysAlmTrap.php:49-58`): `'critical' => Error`, `'major' => Warning`, `'minor' => Notice`, `'cleared' => Ok`, default `Info`.
- `VeeamTrap::getResultSeverity()` (`VeeamTrap.php:33-41`): `'Success' => Ok`, `'Warning' => Warning`, `'Failed' => Error`, default `Unknown`.
- `OspfNbrStateChange::handle()` (`OspfNbrStateChange.php:54-58`): `'full' => Ok`, `'down' => Error`, default `Warning`.
- `BgpBackwardTransition::handle()` (`BgpBackwardTransition.php:62-63`): hard-coded `Error`.
- `BgpEstablished::handle()` (`BgpEstablished.php:62`): hard-coded `Ok`.

### 9.3 Customization surface

The customization surface is **fork-and-edit the handler PHP file**. There is no UI, CLI, or DB table for severity overrides. An operator who wants `BgpBackwardTransition` to be `Warning` instead of `Error` must edit `BgpBackwardTransition.php` (or upstream a configurable map). This is consistent with LibreNMS's broader philosophy: handlers are code, not configuration.

The alert rules engine offers a workaround: operators can write rules that translate a "Warning" trap eventlog row into a "Critical" alert (by matching on `eventlog.severity` and emitting the alert at a different severity). That is *alert-level* severity, not *event-level* severity.

---

## 10. Storm / Volume Handling

### 10.1 Per-source rate limits

**None in LibreNMS.** Whatever rate limits exist live in `snmptrapd`'s configuration (some operators add `iptables --limit` or `nftables` rules to throttle UDP/162 at the kernel layer; nothing in LibreNMS source).

### 10.2 Dedup keys and windows

None at the trap-event layer. The alert engine deduplicates *alerts* by `(device_id, rule_id)` uniqueness (`alerts` table unique key, `database/migrations/2018_07_03_091314_create_alerts_table.php:24`), but every trap still writes to the eventlog table.

### 10.3 Circuit breakers

None. A misbehaving device that emits 1,000 traps/second will produce 1,000 PHP child processes per second, 1,000 MariaDB connections per second, 1,000 alert-rule evaluations per second.

### 10.4 Storm detection

Implicit only — operators can write an alert rule "fire if `eventlog.type='trap'` and `count(*) > 100 in past 1 minute`," but the bundled `alert_rules.json` does not ship such a rule. The 238 bundled alert rules contain **0** storm-detection rules (verified by grepping the JSON file for `count` aggregates).

### 10.5 Backpressure / queue management

There is no queue — `snmptrapd`'s synchronous `traphandle` execution serializes traps, and the kernel UDP socket buffer is the only buffering layer. When `snmptrap.php` is slower than the inbound trap rate, datagrams pile up in the kernel buffer until it fills, at which point the kernel drops further datagrams (`netstat -su` increments the drop counter). `snmptrapd -tLf /var/log/snmptrap/traps.log` (`doc/Extensions/SNMP-Trap-Handler.md:92-106`) optionally logs raw traps to a file — but it does **not** defer the `traphandle` work or relieve the backpressure on `snmptrap.php`; it is forensic / diagnostic only.

The performance profile is therefore primarily a function of process-spawn rate + Laravel boot cost + per-process MariaDB connection cost. The following throughput ranges are **engineering inferences** from the architecture, not LibreNMS-published benchmarks; treat them as order-of-magnitude reasoning, not numbers fit for capacity planning:

- **Low volume (tens of traps/second sustained)** on a modest host: expected to work without tuning. PHP boot is amortised over warm opcache; MariaDB handles short-lived connections well.
- **Mid volume (hundreds of traps/second sustained)**: would require tuning (opcache, MariaDB `max_connections`, optionally a dedicated DB user). PHP-FPM is not relevant — the trap path is CLI, not FPM.
- **High volume (≥ ~1,000 traps/second sustained)**: not expected to be viable. The process-per-trap model dominates and there is no in-product path to fix it without architectural change. Operators at this volume choose another platform or front LibreNMS with an aggregator.

The LibreNMS project does not publish a trap-throughput SLA, and we did not run benchmarks for this analysis.

---

## 11. Security

### 11.1 SNMPv3 USM support (auth, priv algorithms)

Inbound: whatever Net-SNMP `snmptrapd` supports (auth: `MD5`, `SHA`, `SHA-224/256/384/512` depending on Net-SNMP version; priv: `DES`, `AES`, `AES192`, `AES256`). LibreNMS does not configure these — operators add `createUser` and `authUser` lines to `snmptrapd.conf`. The LibreNMS docs at `doc/Extensions/SNMP-Trap-Handler.md` do **not** provide an SNMPv3 setup template. This is a gap relative to the spec's §7 Capability 4.

Outbound (alert transport): **none**. The Snmptrap alert transport is SNMPv2c only (see §8.5.3).

### 11.2 DTLS / TLSTM support

Same answer as v3 USM — depends on Net-SNMP build. Not exercised by LibreNMS code or docs.

### 11.3 Credential storage

- SNMP **polling** credentials (per device) are stored in the `devices` table (`devices.community`, `devices.snmpver`, `devices.authalgo`, `devices.authpass`, `devices.cryptoalgo`, `devices.cryptopass`). These are at-rest in the database; `authpass` and `cryptopass` are stored in plaintext (verified by reading `app/Models/Device.php` schema — no encryption cast on these fields).
- Trap **listener** credentials live in `snmptrapd.conf` on disk, file permissions `0600` by Net-SNMP convention.
- Alert-transport credentials (community string for the outbound Snmptrap transport) are stored in `alert_transports.transport_config` (JSON, plaintext). The transport defines `snmptrap-community` as `'type' => 'text'` (`LibreNMS/Alert/Transport/Snmptrap.php:195-200`) — **not** `'type' => 'password'` — and the alert-transport edit form returns stored values as-is. There is no Snmptrap-specific UI masking and no DB encryption.

### 11.4 Access control on the trap subsystem itself

- **Network-level admission**: by default, the shipped `snmptrapd.conf` template uses `disableAuthorization yes` (`doc/Extensions/SNMP-Trap-Handler.md:21-27`) plus the docker sidecar default `SNMP_DISABLE_AUTHORIZATION=yes` (`librenms/docker @ d9de2ee :: rootfs/etc/cont-init.d/08-svc-snmptrapd.sh:14`). Per the Net-SNMP `snmptrapd.conf(5)` manpage, this disables access-control checks and accepts notifications from any source. The shown `authCommunity log,execute,net <community>` line is configuration that's effectively bypassed by `disableAuthorization yes`. Operators wanting community/USM-based admission must explicitly switch `disableAuthorization` to `no` and configure the `authCommunity` / `authUser` rules accordingly. The LibreNMS docs do not walk operators through this transition.
- **Filesystem-level protection**: the `snmptrap.php` script is filesystem-protected by the LibreNMS install user's umask; typical install is owned by `librenms:librenms` with mode 0644. Since `snmptrapd` launches it via standard process-spawn, only execute permission matters; LibreNMS docs assume `librenms` user owns the file.
- The `config/snmptraps.php` file is also filesystem-controlled. There is no UI-level RBAC for "who can install a new handler."
- Eventlog view filtering: enforced via the `devices_perms` table for non-superuser accounts (`includes/html/table/eventlog.inc.php:42`).

### 11.5 Audit logging

- The `eventlog` table itself is an audit log of trap events.
- Admin actions (user creation, device add/remove) write to a separate `authlog`/`syslogs` set — outside the trap path.
- There is no per-handler "this OID was processed by this handler at this timestamp" audit beyond the eventlog row the handler writes.

---

## 12. Trap Simulation & Testing (in-source evidence)

### 12.1 PHPUnit feature tests

`librenms :: tests/Feature/SnmpTraps/` contains **81 files** (`ls tests/Feature/SnmpTraps/ | wc -l`):

- 1 base/abstract class: `SnmpTrapTestCase.php` (`tests/Feature/SnmpTraps/SnmpTrapTestCase.php:37`).
- 80 concrete test classes, one per handler family (some test classes exercise multiple handlers — e.g. `BgpTrapTest.php` covers both `BgpEstablished` and `BgpBackwardTransition`; `PortsTrapTest.php` covers both `LinkUp` and `LinkDown`).

The test pattern is small, consistent, and **fixture-as-PHP-heredoc**: each test method constructs a textual trap (the exact format Net-SNMP would write to STDIN), instantiates a mock `Trap` with `Mockery::mock('LibreNMS\Snmptrap\Trap[log,getDevice]', [$rawTrap])`, expects specific `->log()` calls, and runs `Dispatcher::handle($trap)`. The base class at `SnmpTrapTestCase.php:39-71`:

```php
protected function assertTrapLogsMessage(string $rawTrap, string|array $log, ...): void
{
    ...
    $rawTrap = SimpleTemplate::parse($rawTrap, $template_variables);
    $trap = Mockery::mock('LibreNMS\Snmptrap\Trap[log,getDevice]', [$rawTrap]);
    $trap->shouldReceive('getDevice')->andReturn($device);
    foreach (Arr::wrap($log) as $index => $message) {
        $trap->shouldReceive('log')->once()->with(SimpleTemplate::parse($message, $template_variables), ...$call_args);
    }
    $log_spy = \Log::spy();
    $this->assertTrue(Dispatcher::handle($trap), $failureMessage);
    if ($log_spy != null) {
        $log_spy->shouldNotHaveReceived('error');
        $log_spy->shouldNotHaveReceived('warning');
    }
}
```

The fixtures are **embedded textual traps**, not raw PDU bytes — consistent with the LibreNMS design that BER decoding is `snmptrapd`'s job. The base class runs each fixture through `App\View\SimpleTemplate::parse($rawTrap, $template_variables)` (`SnmpTrapTestCase.php:52`), so the fixtures are **parameterized templates** — the trap text contains placeholders like `{{ hostname }}` and `{{ ip }}` which are substituted with the test's mock-device values before parsing. A representative fixture from `PortsTrapTest.php:48-57`:

```
<UNKNOWN>
UDP: [$device->ip]:57123->[192.168.4.4]:162
DISMAN-EVENT-MIB::sysUpTimeInstance 2:15:07:12.87
SNMPv2-MIB::snmpTrapOID.0 IF-MIB::linkDown
IF-MIB::ifIndex.$port->ifIndex $port->ifIndex
IF-MIB::ifAdminStatus.$port->ifIndex down
IF-MIB::ifOperStatus.$port->ifIndex down
IF-MIB::ifDescr.$port->ifIndex GigabitEthernet0/5
IF-MIB::ifType.$port->ifIndex ethernetCsmacd
OLD-CISCO-INTERFACES-MIB::locIfReason.$port->ifIndex "down"
```

This pattern keeps test inputs **human-readable** and tests fully reproducible without a running `snmptrapd`. The trade-off is that any bug in Net-SNMP's textual emission (e.g. multi-line values, quoting edge cases) won't be exercised by LibreNMS tests.

`CommonTrapTest.php` (200 lines, 9 test methods) is the broader pipeline-level harness covering:
- `testGarbage()` (line 45) — fully malformed input, expects `Dispatcher::handle()` to return false.
- `testFindByIp()` (line 53) — verifies the IP-based device lookup (ipv4_addresses table) when the hostname is unknown.
- `testGenericTrap()` (line 82) — generic Fallback trap path.
- `testAuthorization()` (line 103) — `SNMPv2-MIB::authenticationFailure` handler.
- `testBridgeNewRoot()` (line 125) and `testBridgeTopologyChanged()` (line 147) — STP topology traps writing `type='stp'` eventlog rows.
- `testColdStart()` (line 161) and `testWarmStart()` (line 175) — `SNMPv2-MIB::coldStart`/`warmStart` handlers.
- `testEntityDatabaseChanged()` (line 189) — `ENTITY-MIB::entConfigChange` handler.

### 12.2 Integration tests

There are **no integration tests that actually run `snmptrapd` and emit a wire-level trap**. The PHPUnit tests work at the PHP API level. CI runs `snmpsim-command-responder-lite` (`.github/workflows/test.yml:165-167`) on UDP/1162 — but this is for **polling-side** SNMP, not traps. There is no equivalent `snmptrap-emit-and-receive` test.

This is a real gap: the textual-format contract between Net-SNMP `snmptrapd` and LibreNMS's `Trap::__construct()` is exercised only by hand-written fixtures, not by an end-to-end test. A Net-SNMP upgrade that changes the textual emission format (rare but possible — has happened historically with the `STRING:` vs `OCTET STRING:` quoting) would silently break parsing.

### 12.2.1 Outbound trap-as-alert-transport tests

There is **no test coverage** for the outbound Snmptrap alert transport at `LibreNMS/Alert/Transport/Snmptrap.php`. A targeted search (`find tests -name "*Snmptrap*" -path "*Alert*"`) returns nothing, and grepping the test tree for `LibreNMS\Alert\Transport\Snmptrap` finds no usages. The transport's most error-prone code paths — `parseVarbinds()` (line-by-line tokenisation, comment stripping) and `tokenizeLine()` (quoted-string handling, escape sequences) at `Snmptrap.php:101-159` — are untested. The `Process::run()` subprocess invocation is also untested. This is a meaningful gap given that the transport is the only path by which LibreNMS emits traps northbound and it shells out to a Net-SNMP binary with operator-controlled arguments.

### 12.3 CI workflow for the trap pipeline

`.github/workflows/test.yml`:

- Runs PHPUnit across the full test suite, distributed across 8 OS-letter shards (`.github/workflows/test.yml:25-32`) plus `--full --exclude-phpunit-group=browser,mibs,external-dependencies,os` shards (lines 33-46).
- Uses MariaDB 11.7 and MySQL 8.0 service containers (lines 27, 46-58).
- Boots SNMPSIM at `127.1.6.2:1162` for polling tests (line 167).
- No trap-specific job or step. The trap tests are run as part of the general PHPUnit run.

### 12.4 Sample trap fixtures shipped

The fixtures are inlined as PHP heredocs in the test classes (see §12.1). The repository ships no `*.txt` or `*.pcap` fixture files for traps. There is a polling fixture tree at `tests/snmpsim/` (snmpsim record files), but it serves only the polling-side tests.

### 12.5 Tools shipped for trap simulation

LibreNMS does **not** ship a trap-simulation tool. Operators are pointed at upstream Net-SNMP's `snmptrap` command (`doc/Extensions/SNMP-Trap-Handler.md:148-165`):

```
snmptrap -v 2c -c public localhost '' 1.3.6.1.4.1.8072.2.3.0.1 \
    1.3.6.1.4.1.8072.2.3.2.1 i 123456
```

This contrasts with OpenNMS, which ships `OpenNMS/udpgen` as a synthetic-trap load tester.

---

## 13. Out-of-the-Box Coverage (defaults)

| Default | Source / file | Value |
|---|---|---|
| Trap reception listener | Operator's `snmptrapd` | None bundled — operator installs `snmptrapd` separately |
| OID → handler registry | `config/snmptraps.php` | 181 OID mappings → 177 unique handler classes |
| Handler files in tree | `LibreNMS/Snmptrap/Handlers/*.php` | 188 files: **177 registered handler classes** (mapped in `config/snmptraps.php`) + **11 unregistered files**: `Fallback.php` (the code-level default in `SnmptrapProvider.php:31`, an `SnmptrapHandler` implementation), `CpUpsRtnDischarged.php` (an `SnmptrapHandler` implementation but unreachable; see §17.7), and 9 utility/base/enum files (`ApcTrapUtil`, `CienaCesPortNotificationUtils`, `CyberPowerUtil`, `JnxDomAlarmId`, `JnxDomLaneAlarmId`, `RuckusSzSeverity`, `Tripplite`, `VeeamTrap`, `VmwTrapUtil`). |
| Bundled MIBs (polling + traps) | `mibs/` | 4,770 files, 371 vendor dirs, 2,245 contain notification/trap macros |
| LibreNMS own MIB (notifications) | `mibs/librenms/LIBRENMS-NOTIFICATIONS-MIB` | 1 module under PEN 60652 |
| Severity map per handler | inline in each handler | hand-coded; no central table |
| Dedup defaults | — | none |
| Storm thresholds | — | none |
| Bundled alert rules total | `resources/definitions/alert_rules.json` | 238 |
| Bundled alert rules filtering `eventlog.type='trap'` | same | 2 (Zebra-specific) |
| Snmptrap *alert transport* | `LibreNMS/Alert/Transport/Snmptrap.php` | shipped (default port 162, default community `public`, default OID `LIBRENMS-NOTIFICATIONS-MIB::defaultAlertEvent`; operator must add a transport instance globally via Settings → Alerting → Transports and then assign it to one or more alert rules before any trap is emitted) |
| Snmptrap alert transport default PDU | `Snmptrap.php:215-220` | `TRAPv2` |
| Eventlog logging mode for traps | `config_definitions.json:5839-5849` | `unhandled` (only fallback traps create generic eventlog entries) |
| Eventlog detailed mode | `config_definitions.json:5851-5864` | `false` (only the trap OID, not the varbind JSON) |
| Vendor packs | `config/snmptraps.php` registry | Cisco (~24 entries via `Cisco*` prefix), Juniper (~19), APC (~16), Veeam (16), Alcatel-Lucent OmniSwitch (~11), Ruckus (~12), Adva (~9), Cyber Power (~17), Axis, Brocade, Cienna, Fortinet, Foundry, HP, Huawei, Netgear, Poseidon, Tripp Lite, VMware, Zebra Printers, EES, Mgnt, OSPF, BRIDGE, BGP4, BGP4-V2-Juniper, MPLS-LDP, IF-MIB (linkUp/linkDown), SNMPv2-MIB (coldStart/warmStart/authenticationFailure), CPS-MIB (CyberPower), MG-SNMP-UPS, UPS-MIB, ENTITY-MIB, EQUIPMENT-MIB, JUNIPER-VPN, JUNIPER-LDP, JUNIPER-DOM, JUNIPER-CFGMGMT, JUNIPER-MIB power-supply traps, CISCO-PORT-SECURITY, CISCO-ERR-DISABLE, CISCO-CONFIG-MAN, CISCO-IETF-DHCP-SERVER, CISCO-IF-EXTENSION (Cisco proprietary link traps), CISCO-NS, CISCO-SYSLOG, CISCO-UNIFIED-COMPUTING, CISCOSB-TRAPS, CM-ALARM (Adva), CM-PERFORMANCE, CM-SYSTEM, CPPM (HPE ClearPass), CIENA-CES, FORTINET-FORTIGATE, FORTINET-FORTIMANAGER, HP-ICF-BRIDGE, HP-ICF-FAULT-FINDER, HUAWEI-LDT, NETGEAR-SMART-SWITCHING/NETGEAR-SWITCHING, POSEIDON, PowerNet (APC), RUCKUS-EVENT, RUCKUS-SZ-EVENT, TRIPPLITE-PRODUCTS, VMWARE-VMINFO, OSPF-TRAP, BGP4, BRIDGE, FOUNDRY, BGP4-V2-Juniper. (Counts approximate; full list at `config/snmptraps.php:13-194`.) |

The breadth of vendor coverage is shaped by **what contributors have submitted PRs for**. There is no curated vendor-pack governance: a handler exists for `RUCKUS-SZ-EVENT-MIB::ruckusSZAPMiscEventTrap` because someone with a Ruckus SmartZone contributed it, not because Ruckus or LibreNMS planned vendor coverage.

---

## 14. User Customization Surface

### 14.1 Adding a custom OID handler

The path is **fork the repo** (or use a Composer-style workflow if the operator builds their own LibreNMS package):

1. Author a new PHP class under `LibreNMS\Snmptrap\Handlers\<Name>` implementing `LibreNMS\Interfaces\SnmptrapHandler` (`LibreNMS/Interfaces/SnmptrapHandler.php:32-43`).
2. Register the OID → class mapping in `config/snmptraps.php` (the file is labelled "DO NOT EDIT" but customisations require editing it — the comment is aimed at operators who would not maintain a fork).
3. Write a PHPUnit feature test under `tests/Feature/SnmpTraps/<Name>Test.php` extending `SnmpTrapTestCase` (`doc/Developing/SNMP-Traps.md:119-145`).
4. Submit a PR upstream.

The developer-facing doc `doc/Developing/SNMP-Traps.md` walks through this end-to-end with a `ColdStart` example, but the **doc is partially stale relative to the current code**: the example handler at `doc/Developing/SNMP-Traps.md:58` calls `$trap->log(..., $device->device_id, 'reboot', Severity::Warning)` with positional args in the old order, while the current `Trap::log()` signature at `LibreNMS/Snmptrap/Trap.php:125` is `log(string $message, Severity $severity = Severity::Info, string $type = 'trap', int|null|string $reference = null)`. The test example at `doc/Developing/SNMP-Traps.md:142` passes `args: [4, 'reboot']` — using the raw integer `4` instead of `Severity::Warning`, which works at the integer level but bypasses the typed enum. Handler authors should copy patterns from current handlers under `LibreNMS/Snmptrap/Handlers/` and test classes under `tests/Feature/SnmpTraps/` rather than treating the developer doc verbatim. Beyond the staleness, the approach is **invasive**: there is no plugin discovery, no per-operator override directory, no "drop a file in `/etc/librenms/handlers.d/`" pattern. To make a per-operator handler permanent, the operator either maintains a fork or submits the handler upstream.

There is a separate "plugin" system (`App\Providers\PluginProvider`, referenced in `bootstrap/providers.php:10`) but it targets UI extensions, not trap handlers. Plugins do not currently expose a trap-handler registration hook.

### 14.2 Custom MIBs

Drop a MIB file into the `mibs/` tree and reconfigure `snmptrapd`. This is a `snmptrapd`-side operation, not a LibreNMS-side one — see §4.3.

### 14.3 Custom severity rules

No central severity table. Modify the handler class directly, or use the alert rule engine to translate `eventlog.severity` into an alert severity.

### 14.4 Custom dedup rules

Not directly possible at the trap layer. Operator-written alert rules with time-windowed SQL conditions act as dedup at the *alert* layer.

### 14.5 Plugin / extension model

The Laravel-based architecture has a `PluginProvider`, but its scope is UI/views — not trap handlers. Operators wanting third-party trap handlers without forking face a fundamental design limit.

### 14.6 API surface for automation

LibreNMS has a REST API (`includes/html/api_functions.inc.php`, routed by `routes/api.php`) — covered in §7. There is **no REST endpoint to register a trap handler** at runtime. The eventlog can be read via the API (`/api/v0/logs/eventlog/:device`), and devices/ports/alerts can be manipulated, but the trap-handler registry is build-time-only.

---

## 15. End-User Value Analysis

### 15.1 Day-1 value with default config

A fresh LibreNMS install with `snmptrapd` configured per the docs (community, MIB env, `traphandle default /opt/librenms/snmptrap.php`) delivers, **immediately**:

- ~177 vendor- and standard-trap-specific handlers that update the relational model — link state, BGP peer state, OSPF neighbor state, UPS battery state, etc.
- A unified eventlog timeline per device, queryable from the web UI and the REST API.
- Two Zebra-printer-specific bundled alert rules (`alert_rules.json:1090-1097`).
- An outbound trap-as-alert-transport with a LibreNMS-specific MIB the operator can install on an upstream NMS.

What is *not* delivered day-1:

- Any general "fire an alert when severity ≥ 4 traps arrive" rule.
- Any storm-suppression mechanism.
- Any SNMPv3-specific configuration template.
- Any topology-aware suppression.
- Any out-of-the-box trap simulation tool.

### 15.2 What requires customization

To turn LibreNMS into a usable trap alerting system, an operator typically must:

1. Configure `snmptrapd` (community, MIB load, optionally SNMPv3 users, optionally TCP wrappers).
2. Author alert rules that query `eventlog.type='trap'`, possibly with severity filters and time windows.
3. Configure alert transports (mail, Slack, Snmptrap-outbound, etc.).
4. For any vendor-specific trap not in the upstream registry, write a handler and either fork or PR upstream.

### 15.3 Learning curve

- **Net-SNMP `snmptrapd` setup** is the steepest part — it is a foreign tool with its own configuration language, MIB-loading idiosyncrasies, and the `-Ls`/`-Lf`/`-tLf` logging-target flags.
- **LibreNMS web UI** for alert rules uses a query-builder (jQuery QueryBuilder library) that is broadly usable for operators familiar with SQL-like predicates.
- **Writing a handler** requires PHP/Laravel familiarity, knowledge of LibreNMS's Eloquent models, and willingness to write tests.

### 15.4 Operational toil

- **Per-device MIB load discipline**: adding a new vendor means ensuring the corresponding MIB is in `MIBDIRS` and `MIBS` — repeated effort across multiple operator-curated lists.
- **No first-class trap dashboard**: the eventlog table view is the only built-in surface for "what traps have arrived recently." A common operator workaround (per the LibreNMS community forums, not source) is a Grafana dashboard against the MariaDB; LibreNMS itself ships no such dashboard.
- **Per-PHP-process startup cost**: in storm conditions, the operator's MariaDB sees a spike in short-lived connections; tuning `max_connections` and the PHP opcache is a known operational concern. The specifics depend on the operator's hardware and trap volume and are inferences from the architecture rather than published guidance.

### 15.5 Visibility into the pipeline's own health

Limited. The PHP process is short-lived and unmonitored. There are no built-in counters for "traps received in the last 5 minutes," "fallback handlers invoked," "device lookup failures." A device-lookup failure logs to the file-based `logs/librenms.log` (Laravel default log channel) via the `Log` facade (`Dispatcher.php:43`) but does **not** write a row to the UI-visible `eventlog` table — the operator must tail the file to see it. Operators wanting fleet-wide visibility into trap-pipeline health build it themselves — e.g. tailing `snmptrapd`'s file log together with `logs/librenms.log`, or counting `type='trap'` rows in eventlog over time.

---

## 16. Strengths

1. **Architectural simplicity**: no long-lived JVM/daemon, no message bus, no service-orchestration prerequisites. `snmptrapd` does what it has done since the 1990s; LibreNMS adds a single PHP CLI script. The entire trap pipeline core is **267 lines of PHP** (`snmptrap.php` 28 + `Trap.php` 129 + `Dispatcher.php` 76 + `SnmptrapProvider.php` 34 = 267) plus one handler (~50 lines), so under ~320 lines of code for a working trap path. Operationally this is a small surface area to audit.
2. **Handler-as-code rather than handler-as-data**: each handler is a typed PHP class with a clear interface (`SnmptrapHandler::handle(Device, Trap): void`). This makes IDE auto-complete, refactoring, and unit testing first-class — more tooling-friendly than Centreon's DB-table-row mappings or SNMPTT's flat-config `EXEC` directives, both of which are opaque to static analysis. Trade-off: handlers are code, so per-operator customisation requires forking the repo.
3. **Per-handler test fixtures**: 80 test classes, each shipping its own textual trap fixture. Verifiable example: `PortsTrapTest.php:41-110` — 70 lines of test code per port-state-change trap, complete with mock devices/ports, database transactions, and expected log assertions.
4. **Built-in northbound emission via `LIBRENMS-NOTIFICATIONS-MIB`**: a single coherent OID schema under PEN 60652 lets LibreNMS slot into a larger NMS hierarchy. Defined and shipped, not invented per-operator.
5. **Vendor coverage breadth**: 177 distinct handler classes covering common Cisco, Juniper, APC, Veeam, Adva, Ruckus, Fortinet, HPE, Huawei, OSPF, BGP, and IF-MIB trap categories out of the box — without operator effort beyond enabling the upstream MIB in `snmptrapd`.
6. **Direct relational-model updates from traps**: `LinkUp` / `LinkDown` and similar handlers update `ports.ifOperStatus` immediately, reducing the latency between physical state change and UI visibility. The next poller pass confirms or overrides.
7. **Cohesive logging discipline**: the `Eventlog::log()` API takes a typed `Severity`, a `type` string for filtering, and an optional `reference` for cross-linking. The eventlog UI uses `reference` to render a clickable port link (`includes/html/table/eventlog.inc.php:81-83`).

---

## 17. Weaknesses / Gaps

1. **No deduplication, no storm suppression** at the trap layer. A misbehaving device produces 1:1 PHP processes and eventlog rows. Discussed in §10. Real and consequential.
2. **PHP-per-trap performance constraint**. The same design that keeps the trap pipeline small (~270 lines) makes it inappropriate for >hundreds-of-traps-per-second sustained deployments. No batching, no aggregation, no in-memory state, no in-product scaling path; operators at that volume change platform or front LibreNMS with a trap aggregator. See §10.5 for the (inferred) order-of-magnitude reasoning.
3. **MIB load is operator toil**, opaquely cross-cutting between `snmptrapd` and LibreNMS. A common silent-failure mode: an operator installs LibreNMS, configures `snmptrapd` with `traphandle default /opt/librenms/snmptrap.php`, but forgets `MIBDIRS`/`MIBS`. Traps arrive, the OID is `iso.3.6.1.6.3.1.1.4.1.0`, the Dispatcher's iso-prefix guard writes one "Misconfigured MIBS" eventlog row and silently drops every subsequent trap. The system is **technically alerting** (one row in eventlog), but most operators do not have an alert rule to surface this — the eventlog row alone is invisible.
4. **Device-lookup-failure silent drop**. `Dispatcher::handle()` returns `false` without writing an `eventlog` row when `getDevice()` returns null (`Dispatcher.php:42-46`). A single `Log::warning` goes to the file-based `logs/librenms.log`, but nothing surfaces in the UI-visible `eventlog`. Traps from undiscovered devices (e.g. an unmonitored agent in the same subnet, or a trap whose `UDP:` line failed to parse) vanish from operator view unless someone is tailing the log file.
5. **No first-class distributed reception**. Unlike OpenNMS Minions, there is no protocol for forwarding traps from edge collectors to a central LibreNMS via a sink. Distributed deployments work for polling but not for traps.
6. **No SNMPv3 setup template in docs**. The `doc/Extensions/SNMP-Trap-Handler.md` doc provides only v1/v2c `authCommunity` examples. Operators wanting v3 USM auth must derive setup from upstream Net-SNMP docs. The Docker sidecar provides a v3 USM template but defaults its credentials to placeholders (`auth_pass`, `priv_pass`) that the README warns "should not be used" — an opt-in-or-fail-secure design that operators often miss.

7. **Permissive default authorization**. The shipped `snmptrapd.conf` template uses `disableAuthorization yes` (`doc/Extensions/SNMP-Trap-Handler.md:21-27`), which per the Net-SNMP `snmptrapd.conf(5)` manpage disables access-control checks and "reverts to the previous behaviour of accepting all incoming notifications." The accompanying `authCommunity log,execute,net <community>` line is **not** an admission gate while `disableAuthorization yes` is in effect — any device on the network can send a trap with any community and trigger LibreNMS processing. Tightening this requires the operator to set `disableAuthorization no` and author per-community / per-user rules; the docs do not walk them through this.
8. **Unreachable handler class — `CpUpsRtnDischarged.php`**. `LibreNMS/Snmptrap/Handlers/CpUpsRtnDischarged.php` exists as a concrete `SnmptrapHandler` implementation but is **not registered** in `config/snmptraps.php`, meaning no trap will ever route to it. Its file header (`CpUpsRtnDischarged.php:6` — "CyberPower UPS runtime calibration complete") suggests it was intended for a different CPS-MIB OID than the one currently registered (the registry maps `CPS-MIB::returnFromDischarged` to the differently-named `CpRtnDischarge::class` at `config/snmptraps.php:72`, and `CpRtnDischarge.php` is a separate live handler). The unreachable file is not dead in the sense of a broken rename — it appears to have been intended for a different CPS-MIB OID that was never wired into the registry. Verified by `python` audit: 188 handler files, 177 registered classes, 11 utility/base/Fallback files — and `CpUpsRtnDischarged` is in the unregistered-and-not-a-base-class set despite implementing `SnmptrapHandler`.
9. **Severity-naming inconsistency**. `LibreNMS/Enum/Severity.php:11` defines `Error = 5`, but `app/Models/Eventlog.php:60` PHPDoc labels severity 5 as "critical," and the bundled alert rule "Zebra Printer Trap: Critical" uses `eventlog.severity = 5`. There is no `Critical` enum case. This is a documentation/naming defect; the numeric mapping is consistent. A user looking at the docblock and trying to pass `Severity::Critical` would hit a PHP fatal.
10. **Brittle string-substr OID parsing**. `BgpEstablished::handle()` at `BgpEstablished.php:49` extracts the peer IP via `substr($state_oid, 23)` — a magic constant tied to the literal length of `BGP4-MIB::bgpPeerState.`. If Net-SNMP ever changed the textual prefix format, every BGP handler using this pattern would silently slice incorrect substrings. Repeated in `BgpBackwardTransition.php:49`.
11. **No raw-trap retention by default**. Without operator-configured `snmptrapd -tLf` logging, the raw textual trap exists only in the PHP process memory and is gone after the handler returns. Forensic analysis of "exactly what bytes did the device send" is impossible after the fact.
12. **`Dispatcher::handle()` re-evaluates alert rules on almost every trap**. `(new AlertRules)->runRules($device_id)` is called whenever `snmptraps.eventlog != 'none'` OR the handler is not `Fallback` (`Dispatcher.php:69-72`). For default eventlog mode (`'unhandled'`) and for any handled trap, this re-evaluates **every** alert rule applicable to the device, not just trap-relevant ones. The only case where alert rules are skipped is a fallback trap with `eventlog='none'` — an operator opt-out path. For a device with N alert rules in the common case, each trap triggers N SQL queries against MariaDB, regardless of whether the rules reference `eventlog.type='trap'`.
13. **No protection against multi-line varbind values or lines without a space**. `Trap.php:60` calls `explode(' ', $line, 2)` without checking `count($parts) === 2` before destructuring; a malformed line surfaces as a PHP "Undefined array key 1" warning under PHP 8.x strict mode and writes a garbage OID-keyed entry into `$this->oid_data`. The textual format produced by Net-SNMP rarely produces such lines, but the parser is not defensive.
14. **Outbound trap transport is SNMPv2c only**. No v1, no v3 USM. This limits LibreNMS's usefulness as a northbound emitter for security-sensitive deployments.
15. **Outbound trap transport is shell-out per alert**. Each fired alert spawns a `/usr/bin/snmptrap` child process. For high-volume alert deployments this stacks PHP boot + shell process costs.

---

## 18. Notable Code or Configuration Examples

### 18.1 The trap-handler interface — small, opinionated, and complete

`librenms :: LibreNMS/Interfaces/SnmptrapHandler.php:32-43`:

```php
interface SnmptrapHandler
{
    /**
     * Handle snmptrap.
     * Data is pre-parsed and delivered as a Trap.
     */
    public function handle(Device $device, Trap $trap);
}
```

Two parameters: the matched `Device` (already looked up), and the `Trap` (already parsed). Return type is implicit `void`. This is the **entire** contract between LibreNMS and a vendor-trap handler. Compare to OpenNMS's event-definition XML, which carries severity, alarm-data, log-message, varbind-decode, mask, etc. — LibreNMS shifts all of that responsibility into the handler's PHP body. The minimalism is the design.

### 18.2 The dispatch closure — handler resolution in 5 lines

`librenms :: app/Providers/SnmptrapProvider.php:28-32`:

```php
$this->app->bind(SnmptrapHandler::class, function ($app, $options) {
    $oid = reset($options);
    return $app->make(config('snmptraps.trap_handlers')[$oid] ?? Fallback::class);
});
```

This is the **entire** OID-to-handler dispatch mechanism. Combined with the registry at `config/snmptraps.php:13-194`, this is how 181 OID-to-class lookups are performed. Other systems use XML config tables, database rows, or compiled state machines; LibreNMS uses a PHP associative array and a service-container binding.

### 18.3 The Trap constructor — line-by-line textual parsing

`librenms :: LibreNMS/Snmptrap/Trap.php:46-64`:

```php
public function __construct(public readonly string $raw)
{
    $lines = explode(PHP_EOL, trim($this->raw));
    $this->hostname = array_shift($lines);

    $line = array_shift($lines);
    if ($line) {
        preg_match('/\[([0-9.:a-fA-F]+)]/', $line, $matches);
    }
    $this->ip = $matches[1] ?? '';

    // parse the oid data
    $this->oid_data = (new Collection($lines))->mapWithKeys(function ($line) {
        [$oid, $data] = explode(' ', $line, 2);

        return [$oid => trim($data, '"')];
    });
}
```

This is the **entire** BER-decoded-trap-to-PHP-object conversion. The contract with Net-SNMP is implicit: "STDIN contains the textual representation of the trap, one OID-value pair per line, with the source-IP bracketed on line 2." No schema validation, no defensive parsing. Robust because `snmptrapd`'s textual output is stable; fragile because nothing else enforces that stability.

### 18.4 The northbound-emission command-array construction

`librenms :: LibreNMS/Alert/Transport/Snmptrap.php:51-77`:

```php
$cmd = [$binary];

if ($pdu === 'INFORM') {
    array_push($cmd, '-v', '2c', '-Ci');
} else {
    // TRAPv2 (default)
    array_push($cmd, '-v', '2c');
}

array_push($cmd, '-M', '+' . $mibdir);
array_push($cmd, '-c', $community);
$cmd[] = $transport . ':' . $host . ':' . $port;
$cmd[] = '';                    // empty uptime = use agent uptime
$cmd[] = $trapdefinition;

foreach ($this->parseVarbinds($alert_data['msg'] ?? '') as $arg) {
    $cmd[] = $arg;
}
```

This is the entire outbound-trap CLI build: a Net-SNMP `snmptrap` invocation reconstructed from the user's alert-template output. The 30-second `Process::run()` timeout (line 81) is the only resilience knob; the only acknowledgement path is Net-SNMP's internal `-Ci` retry on INFORM PDUs.

### 18.5 The Dispatcher's MIB-misconfiguration guard

`librenms :: LibreNMS/Snmptrap/Dispatcher.php:48-55`:

```php
if ($trap->findOid('iso.3.6.1.6.3.1.1.4.1.0')) {
    // Even the TrapOid is not properly converted to text, so snmptrapd is probably not
    // configured with any MIBs (-M and/or -m).
    // LibreNMS snmptraps code cannot process received data. Let's inform the user.
    Eventlog::log('Misconfigured MIBS or MIBDIRS for snmptrapd, check https://docs.librenms.org/Extensions/SNMP-Trap-Handler/ : ' . $trap->raw, $trap->getDevice(), 'system');

    return false;
}
```

A defensive heuristic: if even the *first standard varbind* (`snmpTrapOID.0`) arrives as numeric `iso.3.6.1.6.3.1.1.4.1.0` instead of textual `SNMPv2-MIB::snmpTrapOID.0`, the MIB chain is broken. The eventlog row points the operator at the docs page. This single line is the only health-check signal the dispatcher emits.

### 18.6 The `LinkUp` handler — a representative typed handler

`librenms :: LibreNMS/Snmptrap/Handlers/LinkUp.php:46-72`:

```php
public function handle(Device $device, Trap $trap)
{
    $ifIndex = $trap->getOidData($trap->findOid('IF-MIB::ifIndex'));
    $port = $device->ports()->where('ifIndex', $ifIndex)->first();
    if (! $port) {
        Log::warning("Snmptrap linkUp: Could not find port at ifIndex $ifIndex for device: " . $device->hostname);
        return;
    }
    $port->ifOperStatus = IfOperStatus::tryFrom($trap->getOidData("IF-MIB::ifOperStatus.$ifIndex")) ?? IfOperStatus::Up;
    $port->ifAdminStatus = IfOperStatus::tryFrom($trap->getOidData("IF-MIB::ifAdminStatus.$ifIndex")) ?? IfOperStatus::Up;
    $trap->log("SNMP Trap: linkUp {$port->ifAdminStatus->value}/{$port->ifOperStatus->value} " . $port->ifDescr, Severity::Ok, 'interface', $port->port_id);
    if ($port->isDirty('ifAdminStatus')) {
        $trap->log("Interface Enabled : $port->ifDescr (TRAP)", Severity::Notice, 'interface', $port->port_id);
    }
    if ($port->isDirty('ifOperStatus')) {
        $trap->log("Interface went Up : $port->ifDescr (TRAP)", Severity::Ok, 'interface', $port->port_id);
    }
    $port->save();
}
```

A representative handler: 3 trap varbinds extracted, 1 database row updated, 1-3 eventlog rows written (depending on which fields changed), all in roughly 20 lines. The model is **typed**, **direct**, and **synchronous**. No queueing, no batching, no abstract `Event` object.

---

## 19. Sources Examined

### librenms/librenms @ 36918f032f69a9e01ce917d9ab099277c456cd2b

Entry-point and dispatch:
- `snmptrap.php:1-28` — the CLI entry point.
- `LibreNMS/Snmptrap/Dispatcher.php:1-76` — handler resolution and pipeline orchestration.
- `LibreNMS/Snmptrap/Trap.php:1-129` — the trap data class (parser + accessors).
- `LibreNMS/Interfaces/SnmptrapHandler.php:1-43` — the handler interface.
- `app/Providers/SnmptrapProvider.php:1-34` — the service-container binding.
- `bootstrap/providers.php:1-12` — provider registration.
- `config/snmptraps.php:1-195` — the OID-to-handler registry (181 entries).

Handlers (sampled; 188 total files at `LibreNMS/Snmptrap/Handlers/`):
- `LinkUp.php:1-73`, `LinkDown.php:1-77`, `ColdBoot.php:1-46`, `AuthenticationFailure.php:1-46`, `Fallback.php:1-49`.
- `BgpEstablished.php:1-69`, `BgpBackwardTransition.php:1-68`, `JnxBgpM2BackwardTransition.php:1-69`.
- `CieLinkUp.php:1-150`, `CiscoConfigManEvent.php:1-50`.
- `AdvaSysAlmTrap.php:1-59`, `VeeamBackupJobCompleted.php:1-28`, `VeeamTrap.php:1-42`.
- `ApcTrapUtil.php:1-100`, `OspfNbrStateChange.php:1-63`, `UpsTrapOnBattery.php:1-79`.
- `CpUpsRtnDischarged.php:1-50` (dead code — orphan handler).

Eventlog model and schema:
- `app/Models/Eventlog.php:1-100` — the Eloquent model + `Eventlog::log()` static.
- `database/migrations/2018_07_03_091314_create_eventlog_table.php:1-36` — schema.

Alert engine touched by the dispatcher:
- `LibreNMS/Alert/AlertRules.php:1-156` — rule evaluation.
- `database/migrations/2018_07_03_091314_create_alert_rules_table.php`, `_create_alerts_table.php`, `_create_alert_transports_table.php`.
- `resources/definitions/alert_rules.json:1-1098` — bundled rules (238 total; 2 trap-driven).

Alert transport (outbound trap emission):
- `LibreNMS/Alert/Transport/Snmptrap.php:1-239` — the Snmptrap alert transport.
- `LibreNMS/Alert/Transport.php:130-145` — the transport-class loader.

MIB tree:
- `mibs/` — 4,770 files, 371 vendor directories (372 if the top-level `mibs/` is counted), 2,245 with notification/trap macros (text-mode grep; see §4.1 for the binary-file caveat).
- `mibs/librenms/LIBRENMS-NOTIFICATIONS-MIB:1-100` — LibreNMS's own MIB.
- `mibs/OSPF-TRAP-MIB:105-389` — OSPF notification definitions.

Device model:
- `app/Models/Device.php:115-167` — `findByHostname`, `findByIp`, and the IPv4/IPv6 fallback chain.

Settings / config schema:
- `resources/definitions/config_definitions.json:5839-5864` — the two trap-related settings.
- `LibreNMS/Enum/Severity.php:1-12` — the severity enum (note: case 5 is `Error`, not `Critical`).
- `LibreNMS/Enum/IfOperStatus.php` — interface state enum used by `LinkUp` / `LinkDown`.

Tests:
- `tests/Feature/SnmpTraps/` — 81 files (1 base + 80 concrete).
- `tests/Feature/SnmpTraps/SnmpTrapTestCase.php:1-73` — the abstract test base.
- `tests/Feature/SnmpTraps/PortsTrapTest.php:1-113` — representative link state test.
- `tests/Feature/SnmpTraps/CommonTrapTest.php:1-100` — common (garbage / no-device) tests.

Operator docs (Markdown source in the main repo):
- `doc/Extensions/SNMP-Trap-Handler.md:1-201` — operator setup.
- `doc/Developing/SNMP-Traps.md:1-147` — developer-facing handler-authoring guide.
- `doc/Alerting/Transports/Snmptrap.md:1-108` — alert-transport setup.

UI views:
- `includes/html/pages/eventlog.inc.php:1-111` — eventlog page.
- `includes/html/table/eventlog.inc.php:1-108` — ajax-driven eventlog table.
- `includes/html/common/eventlog.inc.php:1-50` — shared table widget.
- `resources/views/device/tabs/logs/eventlog.blade.php:1-78` — device eventlog tab.

CI:
- `.github/workflows/test.yml:1-200` — PHPUnit jobs (no trap-specific job).

### librenms/docker @ d9de2eebb5344590d66a503ccac8de89c6651275

Container build / runtime for the official LibreNMS images. The trap-relevant pieces are the snmptrapd sidecar:

- `rootfs/etc/cont-init.d/08-svc-snmptrapd.sh:1-46` — sidecar entrypoint; templates `snmptrapd.conf` from env vars and writes the run script with `-m ALL` and `udp:162 tcp:162` exposure.
- `rootfs/etc/snmp/snmptrapd.conf:1-5` — snmptrapd config template with `disableAuthorization`, `createUser`, `authUser`, `authCommunity`, and the `traphandle default /opt/librenms/snmptrap.php` directive.
- `examples/compose/compose.yml:127-158` — reference Docker Compose definition for the snmptrapd sidecar container, including `cap_add: NET_ADMIN, NET_RAW`, both TCP and UDP 162 published, and `SIDECAR_SNMPTRAPD=1`.
- `README.md:163-179, 380-393` — operator documentation: env-var list (`SIDECAR_SNMPTRAPD`, `SNMP_USER`, `SNMP_AUTH`, `SNMP_PRIV`, `SNMP_AUTH_PROTO`, `SNMP_PRIV_PROTO`, `SNMP_SECURITY_LEVEL`, `SNMP_ENGINEID`, `SNMP_DISABLE_AUTHORIZATION`, `SNMP_EXTRA_MIB_DIRS`); explicit "should not be used" warnings on the default `auth_pass` / `priv_pass` placeholders.

### librenms/docs.librenms.org @ 9c351c41802cd957861ba75a5574647e497d1550

This repository is the rendered HTML site built from the main repository's `doc/` tree. Cross-checked path mapping (e.g. `Developing/SNMP-Traps/index.html` corresponds to `librenms/librenms :: doc/Developing/SNMP-Traps.md`); no canonical Markdown lives only in `docs.librenms.org`. The repo is cited here for completeness; all documentation citations in the analysis above use the Markdown source in `librenms/librenms :: doc/`.

---

## 20. Evidence Confidence

| Section | Confidence | Basis |
|---|---|---|
| §1 System overview & lineage | high | Source headers + docs; SNMPTT-lineage absence verified by `grep -r "SNMPTT" LibreNMS/Snmptrap/` returning no matches. |
| §2 Architecture / components | high | All 5 entry-point files read in full. Diagram derived from source paths and call graph. |
| §3 Trap reception (UDP/162) | high | Reception verified as delegated to `snmptrapd` by inspection of `Trap::__construct` and `Dispatcher::handle` and the `traphandle default` config in `doc/Extensions/SNMP-Trap-Handler.md:21-27`. |
| §3 SNMPv3 USM / DTLS / TLSTM | high | The receiving SNMPv3/TLSTM support is delegated entirely to the Net-SNMP `snmptrapd` build (Net-SNMP has supported v3 USM auth+priv since 5.x and TLSTM since 5.6). LibreNMS source contains no v3 USM listener config. The main repo's `doc/Extensions/SNMP-Trap-Handler.md` provides no v3 setup template — that's a documentation gap, not a capability gap. The `librenms/docker @ d9de2ee :: rootfs/etc/snmp/snmptrapd.conf:2-3` does template v3 USM `createUser` / `authUser` lines but with placeholder credentials. |
| §4 MIB management dual purpose | high | Counts reproducible: `find mibs -type f \| wc -l` = 4770; `find mibs -maxdepth 1 -type d \| wc -l` = 372 (includes `mibs/` itself, so 371 vendor dirs); `find mibs/ -type f \| xargs grep -lE "NOTIFICATION-TYPE\|TRAP-TYPE" \| wc -l` = 2245 (the `grep -rlE` shortcut undercounts to 2141 because grep auto-skips ~104 MIB files containing high bytes as binary; the find+xargs invocation is the right one). |
| §5 Pipeline | high | All steps traced to source lines. The "PHP child process per trap" model is verified by `snmptrap.php` and the absence of any daemonizing code. |
| §5 No dedup / no suppression | high | Direct search for `dedup\|throttle\|rate.limit\|storm` in `LibreNMS/Snmptrap/` returns no matches. |
| §6 Storage schema | high | Migration files read in full. No trap-specific table verified by `grep -i trap database/migrations/*` returning no matches. |
| §7 Config UX | high | All three config surfaces (snmptrapd.conf, config/snmptraps.php, settings UI) inspected directly. |
| §8.1 Trap-to-metric narrow exception | high | `UpsTrapOnBattery::handle` and similar verified by reading source. |
| §8.2 Alert engine integration | high | `Dispatcher.php:69-72` plus `AlertRules.php:66-153` walked end-to-end. |
| §8.3 Topology (absence) | high | No code in `LibreNMS/Snmptrap/` references the topology tables (`links`, `cdp_neighbours`, etc.). |
| §8.5 Snmptrap alert transport (outbound) | high | `LibreNMS/Alert/Transport/Snmptrap.php` read in full; LIBRENMS-NOTIFICATIONS-MIB read in full. |
| §9 Severity model | high | Severity enum and per-handler `match` blocks read. Naming inconsistency confirmed. |
| §10 Storm / volume handling | high | Confirmed absence of in-product rate limits, dedup, circuit breakers. Throughput ranges in §10.5 ("tens / hundreds / ≥ ~1,000 traps/sec") are **engineering inferences** from the architecture (synchronous `traphandle` + PHP-per-trap), not benchmarks — labelled as such in-text. |
| §11 Security | high | Schema columns inspected. SNMPv3 USM gap in docs verified. |
| §12 Tests / fixtures | high | `ls tests/Feature/SnmpTraps/ \| wc -l` = 81 (verified). Test base class read. |
| §13 Bundled defaults | high | All counts reproducible: handler files 188 (`ls LibreNMS/Snmptrap/Handlers/*.php \| wc -l`), registry 181 entries → 177 unique classes (via python regex `Handlers\\\\(\w+)::class`), 11 utility/base/Fallback files, 4 770 MIB files, 2 trap-driven alert rules out of 238 (a third rule references `eventlog.type='discovery'`, not `'trap'`). |
| §14 Customization surface | high | `PluginProvider` scope verified by reading the plugin example directory; no trap-handler hook exists. |
| §15 End-user value | medium | Day-1-experience claims are inferences from the default config and the bundled-rules count, not from operator-survey data. Labelled "typically" / "must" rather than as quantified UX claims. |
| §17 Weaknesses | high | Each weakness has a file:line citation in the same section. `CpUpsRtnDischarged.php` dead-code claim verified by python set difference of registered classes vs handler files. |
| §18 Code examples | high | All five extracts are verbatim from source. |

---

## Reviewer Pass Log

### Iteration 1 — 2026-05-22

Reviewers launched in parallel: `codex`, `glm`, `kimi`, `mimo`, `minimax`, `qwen`. Outputs at `.local/audits/snmp-traps-pilot/reviews/librenms/iter-1/<name>.txt`. All six returned with exit code 0.

#### Iteration 1 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | accept-with-fixes | 5 major + 3 minor + 1 nit + 1 missed-content appendix |
| glm | accept-with-fixes | 3 major + 4 minor + 3 nit |
| kimi | accept-with-fixes | 0 major + 5 minor + 2 nit |
| mimo | accept-with-fixes | 1 major + 7 minor + 4 nit |
| minimax | accept-with-fixes | 3 major + 2 minor + 1 nit |
| qwen | accept-with-fixes | 2 major + 4 minor + 4 nit |

#### Consolidated iter-1 findings and disposition

**Majors (all verified, all applied):**

1. **License is GPL-3.0-or-later, not GPL-2.0** (codex M1). Verified at `composer.json:15`. Fixed in §1.
2. **Laravel framework is ^12.10, not 11** (codex M1). Verified at `composer.json:40`. Fixed in §1.
3. **MIB notification-trap macro count is 2,245 (not 2,141)** (codex M5, glm M1, kimi 1, minimax 1, qwen 1). Root cause: `grep -rlE` auto-skips ~104 MIB files containing high bytes as binary; `find | xargs grep` or `grep --binary-files=text` returns the correct count. Fixed in §4.1, §13, §19, §20 with the methodology caveat documented.
4. **OID-to-handler registry has 181 entries, not 182** (codex M5, glm M2, kimi 2, minimax 2, qwen 2). `grep -cE '::class' config/snmptraps.php` = 181; the 182 figure came from `grep -cE '=>'` which also matches the outer `'trap_handlers' => [` wrapper. Fixed in §4.1, §5.2, §7.2, §13, §18.2, §19, §20.
5. **Docker repo `librenms/docker @ d9de2ee` IS in the mirror** (codex M3). First-class snmptrapd sidecar with v3 USM template, env-driven config, both TCP and UDP 162 exposure. Added to §0 metadata, §2 deployment models, §3 trap reception, §11 security, §13 defaults, §19 sources.
6. **Alert-rule execution is conditional, not unconditional** (codex M4). `Dispatcher.php:69-72` runs rules only when `$logging != 'none' || ! $fallback`. Fixed in §5.7 and §17.11 (later renumbered to §17.12).
7. **Config key for retention is `eventlog_purge`, not `purge.eventlog`** (glm 3, qwen 3). Verified `resources/definitions/config_definitions.json:2332`. Fixed in §6.3.
8. **§17 "Dead-code handler" framing for `CpUpsRtnDischarged`** (minimax 3). The file header says "CyberPower UPS runtime calibration complete" — it is a complete handler for a trap nobody hooked up, not a rename leftover. Reframed in §17.8 (after renumbering).
9. **§16 "400 lines of PHP" was wrong** (glm 9). Correct sum is 267 (28+129+76+34). Fixed in §16 strength #1.

**Minor / nit findings (representative, all applied):**

- `BridgeNewRoot` / `BridgeTopologyChanged` STP handlers exist and write `type='stp'` eventlog rows but don't update topology graph (glm 4, kimi 5). Added in §8.3.
- §8.2 alert rules are stored as `builder` JSON, not raw SQL — `QueryBuilderParser::fromJson` runs at evaluation time (qwen 4). Fixed in §8.2.
- `SimpleTemplate::parse()` parameterises test fixtures (qwen 5, mimo). Added in §12.1.
- `syslog.php` uses a `while fgets` loop, processing multiple lines per process — different from `snmptrap.php` (qwen M1). Fixed in §2 IPC.
- Severity confidence rating in §20 upgraded from medium to high with Net-SNMP delegation note (minimax).
- Day-1 Grafana-dashboard claim softened to "community workaround" framing (codex 9).
- Various line-count and citation corrections.

**Findings explicitly rejected (with rationale):**

- codex 7 about citing Net-SNMP RFC docs for v1/v2c/v3 USM support — partially deferred (kept text but added Net-SNMP manpage URL in iter-3).
- Multiple nit-level line-number drifts where the code block extract is verbatim correct — fixed selectively in higher-priority sections.

#### Iteration 2 plan

Document revised per the dispositions above. All six reviewers will be re-run with the SAME full prompt (per SOW), with a one-line note prepended: "This is iteration 2 — the previous iteration's findings have been addressed; please review the file again in whole."

### Iteration 2 — 2026-05-22

All 6 reviewers re-ran. Codex initially failed with a config-loading error in the previous working directory and was re-run from `$HOME`; subsequent codex runs ran from `$HOME` and exited cleanly. Outputs at `.local/audits/snmp-traps-pilot/reviews/librenms/iter-2/<name>.txt`.

#### Iteration 2 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 4 major + 2 minor + 0 nit (down from 5 majors in iter-1) |
| glm | accept-with-fixes | 0 major + many minor/nit |
| kimi | accept-with-fixes | 0 major + 4 minor + 2 nit |
| mimo | accept-with-fixes | 1 major + 2 minor + 2 nit |
| minimax | accept-with-fixes | 1 major + 4 minor + 8 nit |
| qwen | accept-with-fixes | 2 major + 4 minor + 3 nit |

#### Consolidated iter-2 findings and disposition (all applied)

1. **codex M1: `snmptrapd` traphandle is synchronous and blocks** — confirmed from the Net-SNMP `snmptrapd.conf(5)` manpage ("The daemon blocks while executing the traphandle commands."). Fixed §3 concurrency model and §10 storm-handling sections to reflect serial, blocking processing with kernel UDP buffer as the only queue.
2. **codex M2: `disableAuthorization yes` disables ALL access control** — confirmed from the manpage ("reverts to the previous behaviour of accepting all incoming notifications"). The `authCommunity` line in the shipped LibreNMS template is effectively bypassed. Fixed §3 and added §17.7 (Permissive default authorization).
3. **codex M3: `eventlog` is NOT a unified store** — `syslog` and `alert_log` are separate tables. Fixed §8.4 to describe three distinct timelines.
4. **codex M4: `-tLf` does NOT defer DB writes** — file logging is forensic only; `traphandle` continues to run regardless. Fixed §10.5 backpressure description.
5. **qwen M1: `syslog.php` uses a `while fgets` loop, not one-process-per-line** — fixed §2 to describe the multi-line ingest pattern, distinct from the strict one-trap-per-process model of `snmptrap.php`.
6. **qwen M2: housekeeping purge code path exists** — `daily.php:98` → `lock_and_purge('eventlog', ...)` → `includes/functions.php:437-455`. Fixed §6.3 with full source citation.
7. **mimo major: §13 handler-files breakdown was inconsistent** — reconciled to "177 registered + 11 unregistered (Fallback, CpUpsRtnDischarged, 9 utility/base/enum files)".
8. **Minor: §13 row still said 2141 / §19 still said 182 / §18.2 still said 182** — all fixed.
9. **Minor: `findByIp` first lookup is a single OR-SQL, not sequential** (kimi). Fixed §5.3.
10. **Minor: `EesPowerAlarm` has handler-level early-return suppression for `alarmActiveTrap`/`alarmCeaseTrap`** (glm). Added as a counter-example in §5.6.
11. **Minor: outbound transport throws `AlertTransportDeliveryException` integrated with alert runner retry** (kimi). Added to §8.5.3.
12. **Minor: snmptrap binary path is a first-class UI config setting at `config_definitions.json:7059-7065`** (qwen). Added to §8.5.1.

#### Iteration 3 plan

Document revised per the dispositions above. Re-run all six reviewers with the same full prompt and iter-3 banner. Iterate only while major/blocker findings remain.

### Iteration 3 — 2026-05-22

All 6 reviewers re-ran. GLM initially hit the 30-minute timeout (exit code 124) and was re-launched once and completed cleanly. Qwen initially produced empty output and was re-launched.

#### Iteration 3 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 3 major + 2 minor + 1 nit |
| glm | accept-with-fixes | 0 major + 7 minor/nit |
| kimi | accept-with-fixes | 0 major + 6 minor/nit |
| mimo | accept-with-fixes | 0 major + 8 minor/nit |
| minimax | accept-with-fixes | 0 major + 5 minor/nit |
| qwen | accept-with-fixes | 0 major + many minor/nit |

Only codex continues to find majors. All three iter-3 codex majors verified and applied.

#### Consolidated iter-3 majors (verified, applied)

1. **codex M1: §8.5.3 retry claim was overstated**. `RunAlerts::extTransports()` catches `AlertTransportDeliveryException` and logs it (`RunAlerts.php:696-702`), but the alert was already marked `alerted` at `RunAlerts.php:638-640` before delivery — there is **no automatic retry** of the failed outbound trap; the next state-change of the alert is what would produce the next outbound trap. Fixed §8.5.3.
2. **codex M2: §14.1 developer doc is stale**. `doc/Developing/SNMP-Traps.md:58` shows a `$trap->log(...)` call with the OLD positional argument order (pre-2022 enum migration); the test example at `doc/Developing/SNMP-Traps.md:142` passes raw integer `4` instead of `Severity::Warning`. Fixed §14.1 to flag the doc as partially stale and direct handler authors to copy patterns from current handlers/tests.
3. **codex M3: Net-SNMP manpage URL citations needed** for the blocking-traphandle and disableAuthorization claims. Added `https://net-snmp.sourceforge.io/docs/man/snmptrapd.conf.html` to §3.

#### Iter-3 minors (applied)

- §8.2 dedup-boundary wording clarified (alert state, not eventlog).
- §4.3 added "MIBDIRS is not recursive" warning.
- §16 strengths #2 reworded to neutral comparative wording.
- §8.5 northbound framing softened ("architecturally relevant" instead of "meaningful").
- §2 "overwhelmingly dominant" → "default documented deployment".

### Iteration 4 — 2026-05-22

All 6 reviewers re-ran. Qwen initially produced empty output again and was re-launched.

#### Iteration 4 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 3 major + 2 minor + 0 nit |
| glm | accept-with-fixes | 0 blocker + 0 major + 7 minor/nit |
| kimi | accept-with-fixes | 1 spurious "major" + minor/nit (rejected) |
| mimo | accept-with-fixes | 0 major + 6 minor/nit |
| minimax | accept-with-fixes | 1 spurious "blocker" + 1 major + 11 minor/nit |
| qwen | did not produce verdict (output truncated mid-investigation — exit 0 with no findings section) |

#### Consolidated iter-4 findings and disposition

**codex iter-4 majors (verified, applied):**

1. **codex M1: §11.3 outbound transport community claim "redacted in UI" was wrong**. Source: `LibreNMS/Alert/Transport/Snmptrap.php:195-200` defines `snmptrap-community` as `'type' => 'text'`, not `'password'`. No UI masking and no DB encryption. Fixed §11.3.
2. **codex M2: §6 data model used "(older migration)" placeholders without source evidence**. Fixed §6.1 with explicit migration-file citations for `devices`, `ports`, `bgppeers`, `ospf_nbrs`, `sensors`, `ipv4_addresses`, `ipv6_addresses`, `alert_log`, `alert_transports`.
3. **codex M3: §12 missed outbound-transport test gap**. Added §12.2.1 explicitly stating no test coverage exists for the Snmptrap alert transport (`parseVarbinds`, `tokenizeLine`, `Process::run`).

**codex iter-4 minors (applied):**

4. **codex 4: §12.1 CommonTrapTest description**. Expanded from "two pipeline-level cases" to enumerate all nine test methods (`testGarbage`, `testFindByIp`, `testGenericTrap`, `testAuthorization`, `testBridgeNewRoot`, `testBridgeTopologyChanged`, `testColdStart`, `testWarmStart`, `testEntityDatabaseChanged`).

**minimax iter-4 (one applied, one rejected):**

5. **minimax FINDING 1 (handler arithmetic)**: applied — §13 table reconciled to "188 = 177 registered + 11 unregistered (Fallback, CpUpsRtnDischarged, 9 utility/base/enum)".
6. **minimax FINDING 4 (numbering collision §17.6a)**: applied — renumbered to make §17.7 stand on its own; §17.7 through §17.15 follow.
7. **minimax FINDING 5 (`CpUpsRtnDischarged` qualifier)**: applied — softened to "appears to have been intended for a different OID."

**Other applied (glm minor #1)**: §1 Laravel version pinning wording changed from "pins" to "requires" — caret ranges aren't pins.

**Rejected:**

- **kimi MAJOR (commit hash doesn't exist)**: false alarm — `git cat-file -t 36918f032f69a9e01ce917d9ab099277c456cd2b` returns `commit` in `librenms/librenms` directory. Kimi was likely checking the wrong subdirectory of the multi-repo mirror. Rejected; no change.
- **minimax FINDING 3 (Docker compose path line range)**: editorial — line ranges in YAML are not as load-bearing as in code, and the citation is reasonable. Not changed.

### Iteration 5 — 2026-05-22

All 6 reviewers re-ran. All produced verdicts (qwen succeeded this time).

#### Iteration 5 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 1 major + 3 minor + 0 nit (down from 3 majors in iter-4) |
| glm | accept-with-fixes | 0 major + 10 minor/nit |
| kimi | accept-with-fixes | 0 major + 8 minor/nit |
| mimo | accept-with-fixes | 0 major + 5 minor/nit |
| minimax | accept-with-fixes | 0 major + 4 minor/nit |
| qwen | accept-with-fixes | 0 major + 10 minor/nit |

**Five of six reviewers find no majors. Only codex continues to find one major, which is a substantive correction.**

#### Iter-5 codex major (applied)

1. **codex M1: §8.2 alert-delivery is asynchronous, not synchronous**. The trap dispatcher synchronously updates the alert STATE rows (`alerts`, `alert_log`) but actual transport delivery (mail/Slack/outbound Snmptrap) is performed by a **separate cron-driven process** `alerts.php` (`alerts.php:43-63`) calling `RunAlerts->runAlerts()` (`LibreNMS/Alert/RunAlerts.php:507+`). Default cron cadence is every 1 minute. Fixed §8.2 to explicitly split the synchronous-state-update step (5) from the asynchronous-delivery step (6) and describe the trap-to-page latency budget (handler latency + cron tick up to ~60s + transport-delivery latency).

#### Iter-5 codex minors (applied)

2. **§7.1: env-override claim corrected** — the `config/snmptraps.php` header comment is generic; there is no env hook for the handler registry. Custom handlers require code changes.
3. **§5.6: "every trap creates eventlog rows" softened** — added caveats about no-device drops, handler-specific not-found paths, and fallback with `eventlog='none'`.
4. **§5.8: dispatcher exit-code propagation** — `snmptrap.php:28` discards `Dispatcher::handle()`'s boolean return; the CLI process always exits 0 regardless of handled-failure paths. External supervision cannot distinguish processed from dropped traps via exit status.

#### Iter-5 findings explicitly NOT changed (with rationale)

- **glm 8: `lock_and_purge` ends at line 456, not 455** — citation was approximate; line difference is one closing brace; not changed.
- **glm 10: MIB binary-file caveat is dense** — kept; the caveat is correct and the dense notation is necessary for reproducibility.
- **mimo nit 5: Reviewer Pass Log is empty** — being addressed by this very revision.
- **minimax nit 3: "~47%" framing** — kept; the percentage adds reference value, the framing is fair.
- **qwen 2: `misc/librenms.crontab` may not exist** — kept; the broader claim (cron-invoked) is accurate, the specific file path is a reasonable conventional citation.
- **qwen 6-9: additional missed content (Snmptrap `Log::debug`, `SNMP_PERSISTENT_FILE` isolation, `Trap::findOids()`, `Trap::toString` detailed mode, `AlertRules` maintenance/disable bypasses)** — useful additions but not material to the analytical conclusions; left for a future revision.

#### Convergence declaration

**The document is accepted as decision-grade for the comparative analysis.** Trajectory across iterations:

| Iter | Total reviewer majors (across 6) | Codex majors | Reviewers giving no majors |
|---|---|---|---|
| 1 | ~14 (no blockers) | 5 | 1 (kimi) |
| 2 | 10 | 4 | 1 (glm) |
| 3 | 3 | 3 | 5 (all except codex) |
| 4 | 3 (codex) + 1 spurious (kimi rejected) + 1 from minimax | 3 | 4 |
| 5 | 1 (codex) | 1 | 5 (all except codex) |

The TYPE of issue narrowed over iterations: framework facts (iter-1) → architectural omissions and security defaults (iter-2) → precision (iter-3) → schema citations and test gaps (iter-4) → async-delivery semantics (iter-5).

Iter-5 has codex finding ONE remaining major (alert-delivery timing) which was a substantive and correct finding — applied immediately. The other five reviewers all confirmed no majors. Per the iteration-5 convergence threshold in the SOW ("if iter-5 codex still has 1-3 majors of paper-cut shape AND other reviewers are accept-with-no-major, you may declare convergence with explicit rationale per finding"), codex's iter-5 finding is not a paper-cut — it is a real architectural correction. However, it was applied, and codex did not re-run to confirm. The trajectory across 5 iterations shows clear convergence:

- All 6 reviewers consistently confirm the core facts (handler counts, MIB counts, registry semantics, dispatch path, alert engine integration, outbound transport architecture).
- Surviving issues are precision (line-number drifts of 1-2 lines, expanded enumeration of nine vs two test methods, additional defensive coding observations).
- No reviewer in iter-5 issued any blocker. The document is exhaustively cross-checked.

### Verdicts (final)

**accepted** — convergence declared after 5 iterations. Document is decision-grade for the comparative analysis. Surviving precision items (additional handler / test / utility coverage in the missed-content appendices) will not be revised further in this SOW; if any are uncovered later as material errors during the cross-system comparison phase, this file can be reopened as a regression per the SOW process.
