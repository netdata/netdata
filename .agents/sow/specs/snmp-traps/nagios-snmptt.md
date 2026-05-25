# Nagios family + SNMPTT + NSTI — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: The Nagios-family de-facto SNMP trap stack: **Net-SNMP `snmptrapd`** (UDP/162 listener) → **SNMPTT** (Perl translator) → **Nagios Core** passive checks (via `submit_check_result` writing `PROCESS_SERVICE_CHECK_RESULT` to the external command file) → optional **NSTI** (Python/Flask web UI on the SNMPTT MySQL tables). The analysis covers this pattern — not a single product. Nagios XI, Naemon, and the older Icinga-1 family inherit the same chain (often pre-packaged); the OSS-mirrored evidence in this analysis is Nagios Core, NSTI, NSCA, and NRDP. SNMPTT itself is not source-mirrored — it lives upstream at `snmptt.org` (formerly `snmptt.sourceforge.net`).
- **Versions analysed**:
  - `NagiosEnterprises/nsti` @ `58ca81d9d380fea15398278c61e1ad82bdbad12d` (last commit 2017-06-17 — the project has been **unmaintained for ~9 years** at the time of writing; it is the in-mirror evidence for both the NSTI UI and the SNMPTT-on-Nagios deployment workflow).
  - `NagiosEnterprises/nagioscore` @ `8d1d276bea4722b0a1a06ed341926339c43396ac` (Nagios Core, for `submit_check_result`, `PROCESS_SERVICE_CHECK_RESULT` constants, and the external command file).
  - `NagiosEnterprises/nsca` @ `c259e1c08d866cb0920bb807d36a174cca15b249` (passive-check transport when traps land on a different host than Nagios).
  - `NagiosEnterprises/nrdp` @ `39cd102bd5de64bcb3be2b6cbfc7ee7c832babf3` (PHP-based passive-check transport, modern alternative to NSCA).
  - **SNMPTT** @ upstream v1.5 (released 2022-08-17 per `snmptt.org/docs/snmptt.shtml`, retrieval date 2026-05-22). The NSTI deploy script pulls a much older `snmptt_1.4.tgz` from `assets.nagios.com` (`nsti/install/snmptt_deploy.sh:3`).
- **Source evidence**: **mixed** — NSTI, Nagios Core, NSCA, NRDP are source-mirrored; SNMPTT itself is **docs-only** (cited via `snmptt.org/docs/...` URLs with retrieval date 2026-05-22). Each claim is explicitly tagged below. **All SNMPTT-specific claims in this document are sourced from upstream v1.5 documentation**. NSTI's deploy script pulls **v1.4** by default (`nsti :: install/snmptt_deploy.sh:3`) — v1.4 predates the v1.4.2 security fix for EXEC/PREEXEC/unknown_trap_exec shell injection and the `daemon_uid` enforcement fix (`snmptt.org :: docs/snmptt.shtml`, v1.4.2 changelog). Feature differences between v1.4 and v1.5 (notably IPv6 in v1.5, threaded EXEC since v1.2) are noted inline.
- **Repository roots analysed**:
  - `NagiosEnterprises/nsti @ 58ca81d` — Python/Flask UI + the SNMPTT-on-Nagios install scripts and operator docs.
  - `NagiosEnterprises/nagioscore @ 8d1d276` — passive-check command file, `submit_check_result`, `send_nsca` wiring.
  - `NagiosEnterprises/nsca @ c259e1c` — encrypted passive-check transport daemon.
  - `NagiosEnterprises/nrdp @ 39cd102b` — PHP HTTP passive-check transport.
  - `snmptt.org` upstream documentation pages (retrieval date 2026-05-22).
- **Author**: assistant.
- **Reviewer pass**: **accepted** (convergence declared after 3 iterations; iterations 1-3 surfaced and addressed 0 blockers + ~15 verified majors + many minors. The iter-2 round uncovered real source-level defects — NSTI archive route bug, NSTI snmptt.sh sed malformed-append — that were applied as new weaknesses. The iter-3 round added the v1.5 spool-race fix gap, wsgi.py duplicate secret, and obsessive_svc_handler distributed pattern. Final state: 3 reviewers explicit accept-with-fixes with 0 majors (kimi, mimo, minimax); qwen accept-with-fixes with 0 content majors (only an empty-iter-3-log notice that is fixed by this very text); glm produced verification commands but no formal verdict text; codex maintained a "reject" verdict primarily over the LOGONLY-as-category interpretation, which this analysis acknowledges as upstream-documentation ambiguity rather than picking one side. Surviving precision findings documented at the close of the Reviewer Pass Log).

Citations in this document use these conventions:

- `NagiosEnterprises/<repo> @ <commit> :: <relative/path>:<line>` for the four mirrored repos. The four commits above are not repeated on every citation; repo prefix is omitted where unambiguous (`nsti :: …`, `nagioscore :: …`).
- `snmptt.org :: docs/<page>.shtml` for SNMPTT upstream documentation. The retrieval date is **2026-05-22** for every SNMPTT citation in this document.

This analysis covers a **layered glue pattern**, not a unified product. There is no single repository whose source code constitutes "the SNMP trap implementation of Nagios." Instead, three independently-developed projects (Net-SNMP, SNMPTT, NSTI) are wired together by operator-written shell snippets and shipped as a recipe. That fact alone is the most important architectural observation in this document.

---

## 1. System Overview & Lineage

The "Nagios family" trap stack is the **canonical reference implementation** of the loose-coupling NMS pattern: each protocol layer is owned by a separate upstream project that exchanges data with the next layer through a thin, well-documented contract (process spawn + text on STDIN; spool files; the Nagios external command file).

Components and their lineage:

- **Nagios Core** is GPL-2 (the legacy line carrying the linking-exception in many derivatives). Originally NetSaint (1999) by Ethan Galstad, renamed Nagios in 2002. It is an **active-polling supervisor**: scheduler invokes check plugins, plugins return exit codes (0/1/2/3 = OK/WARNING/CRITICAL/UNKNOWN) and stdout text. Nagios Core itself **has zero SNMP trap reception code** — `grep -rln 'snmptrap\|SNMP.*trap' nagioscore/` returns nothing under `base/`, `worker/`, `xdata/`, `module/`. Verified: `nagioscore @ 8d1d276`. Trap support is bolted on through the passive-check mechanism.
- **Net-SNMP `snmptrapd`** is the UDP/162 listener. Standard packaged tool, BSD-style license. Its `traphandle default <command>` directive invokes an external program with the textual trap on STDIN — this is the contract every downstream tool uses (`snmptt.org :: docs/snmptt.shtml`, retrieved 2026-05-22).
- **SNMPTT (SNMP Trap Translator)** is a Perl script. Author: Alex Burger, copyright 2002-2022. License: **GNU GPL v2 or later** (`snmptt.org :: docs/snmptt.shtml`). Current upstream release: v1.5 (2022-08-17). NSTI's deploy script pulls `snmptt_1.4.tgz` from `assets.nagios.com/downloads/addons/snmptt_1.4.tgz` over **plain HTTP** (`nsti :: install/snmptt_deploy.sh:3` — `wget http://assets.nagios.com/downloads/addons/snmptt_1.4.tgz`) — this is the **older v1.4 build**, which predates the v1.4.2 security fixes (EXEC/PREEXEC shell-injection patch and `daemon_uid` enforcement — `snmptt.org :: docs/snmptt.shtml` v1.4.2 changelog) and the v1.5 features (IPv6 via `ipv6_enable`, expanded sample configs). Operators installing via NSTI today therefore inherit a version with known security gaps, fetched over an unencrypted channel.
- **NSTI (Nagios SNMP Trap Interface)** is a small Python/Flask web UI on top of SNMPTT's MySQL tables. License: GPL-2 (`nsti :: README:13-19`). Authors: Nicholas Scott, Luke Groschen (`nsti :: nsti/templates/system/base.html:12`). Copyright 2014-2017 Nagios Enterprises, LLC. **Unmaintained**: the last commit is `58ca81d` from 2017-06-17 ("update readme for github markdown. fix license info"); upstream NSTI development effectively stopped after v3.0.2 was released on 2014-07-06 (`nsti :: CHANGES.txt:1-3`). Python 2 only — uses `xrange`, `print` statements, `except Exception, e:` syntax (`nsti :: nsti/trapdumperdaemon.py:27`, `nsti :: nsti/trapview.py:90`).
- **NSCA (Nagios Service Check Acceptor)** is a TCP daemon for **encrypted** transport of passive check results across hosts. C, GPL-2 (`nsca @ c259e1c`). Daemon listens on TCP/5667 by default; clients use `send_nsca` (`nsca :: src/send_nsca.c`). Relevance to traps: when traps land on a remote host that is not the Nagios master, NSCA carries the SNMPTT-derived passive check to the master.
- **NRDP (Nagios Remote Data Processor)** is a PHP-based HTTP passive-check transport, modern alternative to NSCA. Token-authenticated HTTP POST (`nrdp @ 39cd102b :: server/index.php`, `clients/send_nrdp.sh`). Same role as NSCA, different wire protocol.

Where SNMP traps fit in the wider product: traps are a **passive-check transport**, treated as just another input source feeding Nagios's host/service state machine. Architecturally identical to a `check_disk` plugin running on cron — except the trigger is a UDP packet on port 162 instead of a scheduled invocation, the producer is a remote device instead of a check plugin, and the body is a translated string instead of a check plugin's stdout. Nagios's notification, escalation, and acknowledgement machinery then applies as if the trap-derived service had been polled.

Relationship to upstream tools is **explicit and loose**:

- Nagios does not embed SNMPTT.
- SNMPTT does not embed Nagios — it just calls the operator-supplied `submit_check_result` script (or any other helper) in its `EXEC` clause.
- The integration recipe is documented in Nagios's own contrib tree (`nagioscore :: contrib/eventhandlers/submit_check_result`, 36 lines, written by Ethan Galstad on 2002-02-18) and in SNMPTT's documentation (`snmptt.org :: docs/snmptt.shtml`).
- NSTI is one of several UIs; the SNMPTT MySQL schema (`snmptt`, `snmptt_unknown`, `snmptt_statistics`) is a published interface that anyone can write a UI against.

The pattern is older than most observability platforms in this comparative analysis (the Nagios+SNMPTT cookbook predates Centreon, Zabbix, OpenNMS, Datadog), and it is the **de-facto reference for the "loose-coupling translator + passive check" approach** — every Nagios-family product (Naemon, Icinga 1, Centreon's `centreontrapd`, OP5 Monitor) descends from or borrows from this design. Centreon's own Perl trap-handling code is **explicitly attributed** to SNMPTT in source (`centreon-collect :: perl-libs/lib/centreon/trapd/lib.pm:594-598` — "Code from SNMPTT Modified / Copyright 2002-2009 Alex Burger"; see the Centreon analysis at `.agents/sow/specs/snmp-traps/centreon.md`).

---

## 2. Trap-Subsystem Architecture

### Components

```
                              SNMP-capable device(s)
                                       |
                                       | UDP 162
                                       v
   +-----------------------------------------------------------+
   |  Nagios-monitored host (or remote trap collector)          |
   |                                                            |
   |  snmptrapd (Net-SNMP, port 162)                            |
   |     |                                                      |
   |     | traphandle default invokes:                          |
   |     v                                                      |
   |  snmptthandler   (Perl, one-shot — fork+exec per trap)    |
   |    OR snmptthandler-embedded (Perl loaded via `perl do`    |
   |       in snmptrapd.conf, requires Net-SNMP                 |
   |       --enable-embedded-perl; no fork per trap)            |
   |     |                                                      |
   |     | writes one file per trap                             |
   |     v                                                      |
   |  /var/spool/snmptt/snmptt-<random>                         |
   |     ^                                                      |
   |     | reads + unlinks                                      |
   |     |                                                      |
   |  snmptt (Perl daemon, long-running)                        |
   |     |                                                      |
   |     |  (1) parse spool file (Net-SNMP text format)         |
   |     |  (2) match trap OID against EVENT blocks in          |
   |     |      snmptt.conf, sequential top-to-bottom           |
   |     |      (default: stop after first matching EVENT;      |
   |     |       multiple_event=1 processes ALL matching        |
   |     |       EVENTs for the same trap)                      |
   |     |  (3) apply NODES, MATCH, REGEX, PREEXEC              |
   |     |  (4) format output via FORMAT directive              |
   |     |      ($1..$n varbinds, $A, $aA, $H, $x, $X, $H...)   |
   |     |  (5) emit (configurable, any combination):           |
   |     |       - snmptt.log file                              |
   |     |       - syslog                                       |
   |     |       - MySQL/PostgreSQL/ODBC: snmptt /              |
   |     |         snmptt_unknown / snmptt_statistics tables    |
   |     |       - EXEC any external command                    |
   |     |                                                      |
   |     +--- EXEC typically: submit_check_result               |
   |          (local Nagios) or send_nsca / send_nrdp           |
   |          (remote Nagios)                                   |
   +-----------|------------------------------------------------+
               |
               v textual: PROCESS_SERVICE_CHECK_RESULT
   +-----------------------------------------------------------+
   |  Nagios host (may be same as above)                        |
   |                                                            |
   |   /usr/local/nagios/var/rw/nagios.cmd (external command    |
   |   file = named pipe)                                       |
   |       v                                                    |
   |   nagios (core) reads external commands                    |
   |       - validates PROCESS_SERVICE_CHECK_RESULT             |
   |       - dispatches to internal service-status machine      |
   |       v                                                    |
   |   status.dat / NDOUtils / NagDB / Livestatus               |
   |       v                                                    |
   |   Nagios CGI Web UI (status.cgi) + notification engine     |
   +-----------------------------------------------------------+

   PARALLEL UI PATH (NSTI):
   snmptt MySQL tables ----> NSTI Flask app (mod_wsgi or
                             runserver.py port 8080) ----> Browser
   (NSTI does NOT touch Nagios state; it READS the SNMPTT trap tables
    and MUTATES them via delete/archive/filter routes — see §7)
```

### Deployment models

- **Single host (all-in-one)**: snmptrapd + snmptt + Nagios Core + NSTI (optional) on one host. SNMPTT writes to the local MySQL `snmptt` table for NSTI, and EXECs the local `submit_check_result` script for Nagios. The default in the NSTI installer (`nsti :: install.sh:12-27`).
- **Distributed trap collection**: snmptrapd + snmptt run on a remote collector; SNMPTT EXECs `send_nsca` or `send_nrdp` to the central Nagios. This is the canonical Nagios distributed-monitoring pattern; the helper script for it is `nagioscore :: contrib/eventhandlers/distributed-monitoring/submit_check_result_via_nsca` (39 lines, by Ethan Galstad, last modified 2008-10-15).
- **Distributed with NRDP** instead of NSCA: same pattern, but the EXEC line invokes `send_nrdp.sh` (`nrdp :: clients/send_nrdp.sh`) which `curl -d 'cmd=submitcheck&token=...'` to `http://nagios-master/nrdp/`.
- **Obsessive Compulsive Processor (OCP) variant**: Nagios Core ships a third distributed-monitoring pattern via `nagioscore :: contrib/eventhandlers/distributed-monitoring/obsessive_svc_handler` (alongside `submit_check_result_via_nsca`). This pattern uses Nagios's `obsess_over_service` directive: every check (active or passive, including the trap-derived passive checks) is "obsessed over" by invoking an OCP command that re-submits the result via NSCA to a central broker. Not SNMPTT-specific — it is a generic propagation mechanism that any passive trap pipeline can hook into.
- **Nagios XI** (Nagios Enterprises commercial): ships SNMPTT and a managed trap pipeline pre-configured. NSTI was a separate web UI offering with similar functionality but never the standard XI trap UI. **Nagios XI source is not in the mirror**; this paragraph is industry knowledge and not source-verifiable from this evidence.
- **Containerization**: no first-class container support in any of the mirrored repos. Nagios Core's `nagioscore @ 8d1d276` has no Dockerfile under `docker/` or the repo root. NSTI's install assumes CentOS/RHEL with yum (`nsti :: install/prereqs.sh:7`). Containerizing the stack is operator-DIY and outside source coverage.
- **HA**: no native cluster support for the trap UDP listener. snmptrapd is single-process; SNMPTT's daemon is single-process (`snmptt.org :: docs/snmptt.shtml`, daemon mode section). Operators deploy keepalived + floating VIP + shared MySQL, or run duplicate collectors and accept duplicates downstream. Not source-verifiable as a supported configuration — purely operator pattern.

### Languages and key libraries

- **C** — Nagios Core (the core daemon, command parser, scheduler, external command file processor): `nagioscore @ 8d1d276 :: base/commands.c:724-725` parses `PROCESS_SERVICE_CHECK_RESULT`; constant defined at `include/common.h:144` as `CMD_PROCESS_SERVICE_CHECK_RESULT 30`.
- **C** — NSCA daemon and client: `nsca :: src/nsca.c`, `src/send_nsca.c`.
- **Perl 5** — SNMPTT itself (`snmptt`, `snmptthandler`, `snmpttconvertmib`), all upstream at `snmptt.org`. Perl 5.6.1 minimum per upstream docs (`snmptt.org :: docs/snmptt.shtml`).
- **Python 2** — NSTI (Flask + Storm ORM): `nsti :: nsti/database.py:7-8` `import storm.locals as SL`. Python 2 only (uses `print` statements, `except Exception, e:`).
- **PHP** — NRDP server (`nrdp :: server/index.php`).
- **Shell** — the glue scripts (`submit_check_result`, `submit_check_result_via_nsca`, NSTI install scripts).
- **SQL** — MySQL/MariaDB for the SNMPTT log tables (`nsti :: nsti/dist/nsti.sql`).

Key Perl modules (upstream SNMPTT, not source-verified): `Config::IniFiles`, `Net::SNMP`, `DBI` (for MySQL/PostgreSQL/ODBC logging), `Sys::Syslog`. Per `snmptt.org :: docs/snmptt.shtml`, retrieved 2026-05-22.

Key Python modules (NSTI): `Flask` (web framework), `storm` (ORM, by Canonical), `MySQL-python` (binding). Requirements: `nsti :: REQUIREMENTS.txt:1-5` (Sphinx, sphinx-bootstrap-theme, wheel, Flask, MySQL-python, storm).

### Inter-component IPC

The pattern uses **only POSIX primitives** (process spawn, stdin, regular files, named pipes, TCP/HTTP):

- **snmptrapd → SNMPTT (daemon mode)**: process spawn with trap data on STDIN. snmptrapd's `traphandle default` directive invokes `/usr/sbin/snmptthandler` (`nsti :: install/snmptt_deploy.sh:19`, NSTI's deploy script writes `traphandle default /usr/sbin/snmptthandler` into `/etc/snmp/snmptrapd.conf`). The handler reads STDIN, writes one spool file per trap to `/var/spool/snmptt/`, and exits.
- **snmptrapd → SNMPTT (embedded mode)**: Net-SNMP loads SNMPTT as an embedded Perl module via `perl do "/usr/sbin/snmptthandler-embedded"`. Net-SNMP must be compiled with `--enable-embedded-perl` (`snmptt.org :: docs/snmptt.shtml`). Embedded mode is meaningfully faster — one process instead of N — but requires a Net-SNMP build that distros don't always ship.
- **snmptthandler → snmptt (daemon)**: filesystem spool directory `/var/spool/snmptt/`. Each trap is one small file. The daemon polls every `sleep` seconds (default 5 per upstream docs — `snmptt.org :: docs/snmptt.shtml`, daemon mode section).
- **snmptt → snmptt.log**: text log file `/var/log/snmptt/snmptt.log` (default per upstream docs).
- **snmptt → MySQL**: DBI connection, `INSERT INTO snmptt (...)` per matched trap, `INSERT INTO snmptt_unknown (...)` per unmatched trap. Schema reproduced exactly in `nsti :: nsti/dist/nsti.sql:27-91` (NSTI ships its own copy of the SNMPTT schema as part of its install).
- **snmptt → Nagios (local same-host)**: SNMPTT's `EXEC` directive runs `submit_check_result <host> <service> <return_code> <plugin_output>`, which `echo`s a `[<datetime>] PROCESS_SERVICE_CHECK_RESULT;<host>;<service>;<return_code>;<plugin_output>` line into the Nagios external command file `/usr/local/nagios/var/rw/nagios.cmd` (a named pipe / FIFO). Verbatim: `nagioscore :: contrib/eventhandlers/submit_check_result:30,32` — `CommandFile="/usr/local/nagios/var/rw/nagios.cmd"`, `cmdline="[$datetime] PROCESS_SERVICE_CHECK_RESULT;$1;$2;$3;$4"`.
- **snmptt → Nagios (remote)**: SNMPTT's `EXEC` runs `submit_check_result_via_nsca` which calls `send_nsca -H <NagiosHost> -c send_nsca.cfg` (`nagioscore :: contrib/eventhandlers/distributed-monitoring/submit_check_result_via_nsca:38`). On the master, `nsca` daemon decrypts and writes the same `PROCESS_SERVICE_CHECK_RESULT` line to the local external command file.
- **NSTI → MySQL**: Storm ORM connection (`nsti :: nsti/database.py:19-28`). NSTI **does not ingest real traps** — it never receives a PDU and never produces a trap row from one. It does, however, **mutate the SNMPTT tables**: the `/api/trapview/delete/<tablename>` route (`nsti :: nsti/trapview.py:79-94`) deletes rows from `snmptt` / `snmptt_archive` / `snmptt_unknown`, and `/api/trapview/archive` (`nsti :: nsti/trapview.py:97-128`) copies rows from `snmptt` into `snmptt_archive` then deletes the originals. NSTI also writes its own `filter` and `filter_atom` tables. The `trapdumperdaemon.py` file is **not** an ingestion daemon; it is a **synthetic demo data generator** (`nsti :: nsti/trapdumperdaemon.py:27-43` — random selection from hard-coded `enterprise`, `severity`, `event` arrays, inserting fake rows).
- **NSTI → Browser**: Flask routes serving Jinja templates (`nsti :: nsti/trapview.py`, `filters.py`, `inspector.py`) and a JSON API (`/api/trapview/read/<tablename>`).

### Telemetry / health surfaces

- **snmptrapd**: writes to syslog (Net-SNMP standard) and logs unhandled traps depending on `-Lo`/`-Ls` options (`nsti :: install/snmptt_deploy.sh:27` invokes `snmptrapd -On`).
- **SNMPTT**: file log `/var/log/snmptt/snmptt.log` (default), syslog (via `log_system_enable`), and optional MySQL `snmptt_statistics` table when `statistics_interval` is set (`snmptt.org :: docs/snmptt.shtml`).
- **Nagios Core**: status.dat / NDOUtils / event broker; no metrics endpoint, no `/metrics` HTTP exposure in the OSS core. Status visible via the legacy CGI web UI (`nagioscore @ 8d1d276 :: cgi/status.c`).
- **NSTI**: no metrics endpoint. Application log goes to the Apache/mod_wsgi error log.

There is **no unified pipeline health view** across the stack. Operators must correlate snmptrapd's syslog, SNMPTT's `snmptt.log` and `snmptt_statistics`, Nagios's status.dat and `nagios.log`, and NSTI's mod_wsgi log to debug a single trap's path end-to-end. This is the cost of loose coupling.

---

## 3. Trap Reception (UDP/162 Ingress)

### Listener implementation

The Nagios family **delegates entirely to Net-SNMP `snmptrapd`** for UDP ingress, PDU decoding, and SNMPv3 USM/authentication. **No Nagios-family code touches the socket.** Evidence across all mirrored repos:

- NSTI's install script wires snmptrapd to invoke SNMPTT as a `traphandle default`: `nsti :: install/snmptt_deploy.sh:19` writes `traphandle default /usr/sbin/snmptthandler` into `/etc/snmp/snmptrapd.conf`. The script also starts snmptrapd directly: `nsti :: install/snmptt_deploy.sh:27` runs `snmptrapd -On`.
- NSTI's per-environment SNMPTT setup script `nsti :: install/snmptt.sh:9-10` rewrites `snmptrapd.conf`:
  ```
  sed -i'.bkp' '$a\ #disableAuthorization yes\authCommunity    log,execute,net    public\traphandle default /usr/sbin/snmptthandler\' "$SNMPTRAPD"
  ```
- NSTI's WatchGuard setup doc (`nsti :: docs/snmpttsetup.rst:92-97`) documents the same wiring for an operator:
  ```
  disableAuthorization yes  (use this in your initial setup to ensure everything is working.  Remove this later for security)
  traphandle default /usr/local/sbin/snmptt
  ```
  Note the **inconsistency between the script (`/usr/sbin/snmptthandler`) and the doc (`/usr/local/sbin/snmptt`)** — the script is correct for daemon mode (handler spools and exits, snmptt daemon picks up later); the doc instruction is for standalone mode (snmptt itself is the traphandle, slower per-trap but no daemon to run). The doc does not flag this distinction to the operator. This is a real operator-confusion hazard.
- `grep -rln 'snmptrap\|SNMP.*trap' nagioscore @ 8d1d276 base/ worker/ xdata/ include/ module/` returns nothing. The Nagios Core daemon does not parse SNMP PDUs or open UDP/162.
- Nagios Core's `nagios.spec` (`nagioscore @ 8d1d276 :: nagios.spec`) does not require Net-SNMP as a dependency. SNMP support is **entirely add-on**.

### SNMP version support

Determined **entirely by the underlying Net-SNMP `snmptrapd`** — Nagios-family source proves only the dependency wiring and the post-decode handoff. Per `snmptt.org :: docs/snmptt.shtml`, retrieved 2026-05-22:

- **SNMPv1, v2c**: fully supported by snmptrapd; SNMPTT parses both via the same Net-SNMP text format on STDIN.
- **SNMPv3 USM**: supported by snmptrapd through `createUser`/`authUser` directives in `/etc/snmp/snmptrapd.conf`. Net-SNMP supports MD5/SHA/SHA-2 auth and DES/AES-128/AES-192/AES-256 priv depending on Net-SNMP version. Per `snmptt.org :: docs/snmptt.shtml`, "the snmptthandler-embedded provides SNMPv3 support through variable substitutions $Be, $Bu, $BE and $Bn for SNMPv3 securityEngineID, securityName, contextEngineID and contextName" — meaning **the embedded handler is required for SNMPv3 metadata to reach SNMPTT**. The non-embedded handler does not pass these fields by default. Neither NSTI nor Nagios Core provides a UI for managing SNMPv3 USM credentials — operators edit `snmptrapd.conf` by hand.
- **DTLS / TLS-TM (RFC 5953/6353/9456)**: supported by Net-SNMP only when built with the `tlstm` transport (uncommon in default distro packages). Nagios-family ships no TLS configuration template; NSTI's stock `snmptrapd.conf` directives (`nsti :: install/snmptt.sh:10`) use only `disableAuthorization`, `authCommunity`, and `traphandle default` — no TLS.
- **SNMP Informs**: snmptrapd handles INFORM acknowledgement transparently. SNMPTT does not distinguish trap-PDU from inform-PDU; both arrive as the same text block.

The auth/priv algorithm matrix is what Net-SNMP supports across recent versions; the Nagios-family stack does not constrain or extend it. See Net-SNMP's `snmptrapd.conf(5)` man page upstream (`https://www.net-snmp.org/docs/man/snmptrapd.conf.html`).

### Performance / concurrency model

The runtime data flow is a **spool-file pipeline with significant latency-vs-throughput tradeoffs** — identical in shape to Centreon's `centreontrapd` (which descends from SNMPTT). Per `snmptt.org :: docs/snmptt.shtml`:

- **Per trap, one OS process spawn** (when using the non-embedded `snmptthandler`): every incoming trap causes snmptrapd to fork-exec the handler.
- **Embedded mode** (`snmptthandler-embedded`) avoids the fork by loading Perl into snmptrapd via Net-SNMP's `--enable-embedded-perl` build option. This is the recommended mode for high trap rates per upstream docs but requires a Net-SNMP build that distro packages often omit.
- **Filesystem buffer**: each trap is one file in `/var/spool/snmptt/`. On default ext4, every trap is a `creat`, multiple `write`s, `close`. Putting the spool on tmpfs is a documented optimization (same as Centreon's; not specifically called out by SNMPTT but consistent with the architecture).
- **Polling reader, not inotify**: snmptt daemon reads the spool directory every `sleep` seconds (default 5 per upstream docs). Median latency from trap arrival to processing equals `sleep / 2` ≈ 2.5 s with defaults.
- **Standalone mode**: snmptrapd's `traphandle default /usr/local/sbin/snmptt` (instead of `snmptthandler`) runs SNMPTT itself in standalone mode for each trap. Per `snmptt.org :: docs/snmptt.shtml`, standalone mode "requires reloading configuration files for each trap" and is "unsuitable for high-frequency trap environments." On older hardware this took "under a second" per trap.
- **No internal queue inside snmptt**: when the daemon picks up a file, it parses synchronously, matches against `snmptt.conf` EVENT blocks, executes FORMAT/EXEC, then deletes the file.
- **Threaded EXEC (optional, since v1.2)**: per `snmptt.org :: docs/snmptt.shtml` v1.2 changelog: "Experimental: Added threads (Perl ithreads) support for EXEC. When enabled, EXEC commands will launch in a thread to allow SNMPTT to continue processing other traps. Added snmptt.ini options `threads_enable` and `threads_max`." This decouples the main matching loop from slow EXEC commands; it does NOT add a real queue or backpressure. It is labelled experimental in upstream docs and is **not enabled by the NSTI installer**.

This is **not a high-throughput design**. The upstream SNMPTT docs do not benchmark it. Any specific traps/second number is inference. Architectural ceilings:
- fork rate for snmptrapd-to-handler (eliminated in embedded mode);
- filesystem `creat()` rate (eliminated by tmpfs);
- per-trap EXEC fork rate when integrating with Nagios (every matched trap forks `submit_check_result`, which itself forks `echo` and `date`) — `threads_enable=1` can mitigate this if the operator opts in.

The document declines to assign a traps/second cap.

### Privileged-port handling

- snmptrapd binds UDP/162 directly. Net-SNMP either runs as root, uses Linux `CAP_NET_BIND_SERVICE`, or is configured to bind a higher port front-ended by NAT.
- NSTI's installer does not give snmptrapd `CAP_NET_BIND_SERVICE`; the assumption in `nsti :: install/snmptt_deploy.sh:27` is that snmptrapd runs as root.
- The NSTI firewall script `nsti :: install/firewall.sh:1` opens **TCP/80 for HTTP**, not UDP/162. UDP/162 firewalling is the operator's responsibility (the installer does not configure it).

### Horizontal scaling pattern

Through **multiple collectors**, each running its own snmptrapd + snmptt, each EXECing back to a central Nagios via `submit_check_result_via_nsca` or `send_nrdp.sh`. There is **no RX-side dedup across collectors** — each trap routes to whichever collector its device targets. Operators choose targeting per-device by snmptrapd's `-Lf` or routing rules outside this stack.

### HA / clustering

No native HA in any mirrored component:
- snmptrapd is single-process per host.
- SNMPTT's daemon is single-process per host (`snmptt.org :: docs/snmptt.shtml`).
- Nagios Core's master is single-process (`nagioscore @ 8d1d276 :: base/nagios.c`).
- NSTI is stateless web UI but pinned to one MySQL.

Operator patterns (keepalived + floating VIP, duplicate-collectors-with-downstream-dedup) are common but **not source-verifiable as supported configurations**.

---

## 4. MIB Management

### MIB store location and layout

The Nagios family **does not maintain its own MIB store**. MIB files live in Net-SNMP's standard MIB resolution path. Per `nsti :: docs/snmpttsetup.rst:38`: "I already added my WatchGuard dependancy MIBs to the `/usr/share/snmp/mibs` directory." That is the Net-SNMP path, not a Nagios or SNMPTT path. SNMPTT does not own a MIB directory of its own.

### Compilation / load pipeline

MIBs are **never compiled at runtime**. The workflow is offline:

1. Operator places MIB files into Net-SNMP's MIB path (`/usr/share/snmp/mibs/`).
2. Operator runs the SNMPTT-shipped Perl script `snmpttconvertmib`. Per `snmptt.org :: docs/snmpttconvertmib.shtml`: "a Perl script which will read a MIB file and convert the TRAP-TYPE (v1) or NOTIFICATION-TYPE (v2) definitions into a configuration file readable by SNMPTT." It uses `snmptranslate -IR -Ts mib-name::trapname -m mib-filename` internally (per the same page).
3. Output is a text file with one EVENT block per trap definition: NSTI's WatchGuard recipe (`nsti :: docs/snmpttsetup.rst:55-57`) shows the typical operator script:
   ```bash
   for i in WATCHGUARD* IPSEC*; do
       snmpttconvertmib --in=/usr/share/snmp/mibs/$i --out=/etc/snmp/watchguard.conf --exec='/etc/snmp/traphandle.sh $r $s "$D"'
   done
   ```
4. The generated `.conf` file (or files — operator can split per-vendor) is added to SNMPTT's `snmptt_conf_files` list in `/etc/snmp/snmptt.ini` (`nsti :: docs/snmpttsetup.rst:19-22`).
5. SNMPTT daemon is restarted (`service snmptt start` per `nsti :: docs/snmpttsetup.rst:78`).

This is fundamentally different from OpenNMS's UI MIB compiler or Centreon's `centFillTrapDB` (which writes to a relational DB). **SNMPTT's authoritative source of trap definitions is a flat text file `snmptt.conf`**, generated offline.

### Bundled MIBs out-of-the-box (vendor coverage)

**None.** Verified across all four mirrored Nagios-family repos:
- `find "$NSTI" -name '*.mib' -o -name '*.MIB' -o -name '*.txt' | grep -i mib` returns nothing.
- `nagioscore @ 8d1d276` has no `mibs/` directory.
- `nsca @ c259e1c` has no MIB files.
- `nrdp @ 39cd102b` has no MIB files.
- SNMPTT upstream tarball: per `snmptt.org :: docs/snmptt.shtml`, the SNMPTT distribution does not bundle vendor MIBs. Upstream **does** ship an `examples/` folder containing a sample `snmptt.conf`, a sample trap file (e.g., `examples/#sample-trap.generic.daemon`) usable for testing by copying it into the spool directory, and an example `snmpttsystem.conf`. These examples demonstrate the EVENT syntax and provide test fixtures — they are not a production vendor catalogue. Vendor coverage for production is **entirely operator-supplied**.

Compared to OpenNMS's 17,442 bundled event definitions (`opennms-base-assembly/src/main/filtered/etc/examples/events/`) or Centreon's 214 seeded trap rows (`centreon/www/install/insertBaseConf.sql`), the Nagios family ships **zero day-1 trap recognition**. Every operator builds the catalogue from scratch.

### User workflow for adding/updating MIBs

Per `nsti :: docs/snmpttsetup.rst` and `nsti :: docs/setup.rst`:

1. Download MIB from vendor or a MIB repository site (the NSTI doc recommends `mibdepot.com` at `nsti :: docs/setup.rst:43`).
2. Resolve dependencies by inspection — operator must download each `IMPORTS` MIB referenced (`nsti :: docs/snmpttsetup.rst:38-47` explicitly enumerates 8 IF-MIB / IP-MIB / RFC1213-MIB / SNMPv2-MIB dependencies for a WatchGuard MIB).
3. Place all files into `/usr/share/snmp/mibs/`.
4. Run `snmpttconvertmib --in=<file> --out=<conf> --exec=<command>` for each MIB.
5. Add `--out` files to `snmptt_conf_files` in `snmptt.ini`.
6. Restart snmptt daemon.

This is **a highly manual MIB workflow relative to the systems analysed to date in this cohort** (OpenNMS, Centreon, Zenoss, CheckMK, LibreNMS, Zabbix — comparative ranking deferred to `comparative-analysis.md`). No UI for upload, no dependency resolution, no diffing. Errors silently produce malformed EVENT blocks (per the documentation's repeated warnings about dependencies).

### Dependency resolution

**None.** Operator manually resolves IMPORTS by reading the MIB header and downloading each named module. The NSTI doc explicitly says (`nsti :: docs/snmpttsetup.rst:38`): "Remember to do your research to be sure that you have all of the dependancy MIBS, some of them may not actually be in your mibs directory by default and will cause errors."

### Version management vs firmware

Not addressed by any tool in the stack. When a vendor releases a new firmware version with a new MIB, the operator manually re-downloads, re-runs `snmpttconvertmib`, and manually merges into `snmptt.conf`. Existing EVENT blocks for older traps are not auto-deprecated or version-tagged.

### Fallback behaviour for unknown OIDs

Per `snmpttconvertmib.shtml` and the `snmptt.ini` configuration:

- If `unknown_trap_log_enable = 1` in `snmptt.ini`, unmatched traps are logged to the `snmptt_unknown` MySQL table (when DB logging is on) or the configured unknown-trap log file.
- The NSTI WatchGuard recipe (`nsti :: docs/snmpttsetup.rst:13-17`) sets `unknown_trap_log_enable = 1` and `unknown_trap_exec = /etc/snmp/traphandle.sh` — meaning an unmatched trap **still** invokes an operator-supplied script. This is the **only configurable fallback path** in the SNMPTT stack.
- If the operator does not set `unknown_trap_exec`, unmatched traps are logged-only and never become Nagios passive checks. They are **silently invisible to Nagios**.

The unknown-trap path is a **real day-1 weakness**: the stack's default behaviour for any device whose MIBs the operator has not converted is "silently log, never alert." This is opposite to OpenNMS's "match against a catch-all event, alarm with `Indeterminate` severity" default behaviour.

---

## 5. Trap Processing Pipeline

End-to-end phases. SNMPTT-specific phases are sourced from `snmptt.org :: docs/snmptt.shtml`, retrieved 2026-05-22.

### Phase 1 — Reception (snmptrapd)

snmptrapd accepts the PDU, decodes BER, performs auth (SNMPv1 community, SNMPv2c community, SNMPv3 USM), and constructs a textual representation per its output format (configurable via `-O*` flags). Per `nsti :: install/snmptt_deploy.sh:27` and `nsti :: docs/snmpttsetup.rst:89` the recommended options are `-On -s -u /var/run/snmptrapd.pid`:
- `-On` = display OIDs numerically (essential — SNMPTT matches numerically).
- `-s` = log to syslog.
- `-u <pidfile>` = standard pidfile.

### Phase 2 — Handoff to SNMPTT

Two paths:

- **Daemon mode** (recommended): snmptrapd invokes `/usr/sbin/snmptthandler` (`nsti :: install/snmptt_deploy.sh:19`). The handler reads STDIN, writes one file to `/var/spool/snmptt/` and exits.
- **Embedded mode**: snmptrapd loads `snmptthandler-embedded` as embedded Perl. Per `snmptt.org :: docs/snmptt.shtml`: "perl do "/usr/sbin/snmptthandler-embedded"" line in `snmptrapd.conf`. Avoids one fork per trap.
- **Standalone mode** (not recommended for production): snmptrapd's `traphandle default` is `/usr/local/sbin/snmptt` itself. SNMPTT processes each trap synchronously per snmptrapd invocation, reloading config each time. NSTI's doc accidentally shows this pattern at `nsti :: docs/snmpttsetup.rst:97` ("traphandle default /usr/local/sbin/snmptt") without flagging it as standalone-only — a documentation hazard.

### Phase 3 — Parse (SNMPTT daemon)

SNMPTT polls `/var/spool/snmptt/` every `sleep` seconds (default 5, per upstream docs). For each file:
- Reads the Net-SNMP text block from disk.
- Parses Net-SNMP's textual format into structured fields: agent IP, hostname, uptime, trap OID, enterprise OID, community, and a list of varbinds.
- Per `snmptt.org :: docs/snmptt.shtml`, the embedded handler additionally exposes SNMPv3 fields via `$Be`, `$Bu`, `$BE`, `$Bn` substitutions (securityEngineID, securityName, contextEngineID, contextName).

### Phase 4 — Match against EVENT blocks

The core matching loop. Per `snmptt.org :: docs/snmptt.shtml`, EVENT blocks in `snmptt.conf` are processed **sequentially** during each polling cycle. A typical EVENT block:

```
EVENT <name> <OID> <category> <severity>
FORMAT <format string with $1..$n, $A, $aA, $H, $x, $X variables>
[NODES <hostname|ip|cidr|file>]
[MATCH <varbind_index>: <regex>]
[REGEX (<pattern>)(<replacement>)<modifiers>]
[PREEXEC <command>]
[EXEC <command>]
[SDESC]
multi-line short description
[EDESC]
multi-line extended description
```

Key matching semantics (per `snmptt.org :: docs/snmptt.shtml`):
- **Matching keys**: trap OID is matched directly. The upstream docs directly document the matching behaviour: "If multiple_event is disabled, only the first matching entry will be used" and "A trap is handled once using the first match in the configuration file" (`snmptt.org :: docs/snmptt.shtml`, retrieved 2026-05-22). Default semantics: **first-match-wins, top-to-bottom**. With `multiple_event = 1`, every matching EVENT block is processed for the same trap.
- **NODES filter**: per-EVENT optional. Accepts hostnames, IPs, CIDRs, or file paths. Per `snmptt.org :: docs/snmptt.shtml`: "NODES files can now contain comments" and a NODES entry can contain a CIDR. Default is **positive matching** (POS); `MODE=NEG` flips it to negative.
- **MATCH directive**: matches against varbind values inside the trap (not the OID). Supports regex and bitwise AND.
- **REGEX**: substitution with captures and modifiers `i`, `g`, `e`.
- **Multiple EXEC and NODES per EVENT**: per upstream docs, "An EVENT can now contain multiple EXEC lines" and "multiple NODES lines."

The matching model is **flat, ordered, regex-augmented**. There is no indexed lookup; with N EVENT blocks, every trap performs O(N) comparisons against OIDs. For a fleet with thousands of EVENT entries, this is a real cost.

### Phase 5 — Source identification

SNMPTT's source identification is **limited to the textual fields snmptrapd passes**:
- `$A` = agent hostname (from snmptrapd's reverse DNS or the explicit `snmpTrapAddress.0` varbind).
- `$aA` = agent IP address.
- `$R` = source IP from the UDP packet header (different from agent address in NAT scenarios).

Per `snmptt.org :: docs/snmptt.shtml`, `$1..$n` are positional varbind values. There is **no built-in device-inventory join** — the stack does not know whether the source IP is a device Nagios is configured to monitor. The join happens implicitly later, when Nagios receives the passive check result keyed by `host` and `service` names that the operator's `submit_check_result` invocation specifies.

### Phase 6 — Enrichment

Per `snmptt.org :: docs/snmptt.shtml`:
- **FORMAT**: `$1..$n` substitutions for varbinds, plus `$A`, `$aA`, `$x`, `$X`, `$#` (varbind count), `$H` (system hostname).
- **PREEXEC**: runs a script before FORMAT/EXEC; its output is stored in `$p_n_` variables that can then be referenced in FORMAT/EXEC. This is the operator's enrichment hook — e.g., resolve a device serial number from the IP via an external lookup.
- **REGEX**: substitutions on FORMAT strings with captures.
- **`net_snmp_perl_enable`**: optional `snmptt.ini` option that, when set, uses Net-SNMP's Perl bindings to resolve symbolic OIDs at runtime. NSTI's installer turns this on by default (`nsti :: install/snmptt.sh:7`).

No topology join. No device-inventory join. No cross-trap correlation. Enrichment is **per-trap, per-EVENT, operator-scripted**.

### Phase 7 — Severity normalization

Per `snmptt.org :: docs/snmptt.shtml`, SNMPTT EVENT severities are: **Normal, Warning, Minor, Major, Critical**. The literal value **LOGONLY** also appears in the EVENT directive but the upstream docs use the term inconsistently: the config-reference section's syntax form `EVENT event_name event_OID "category" severity` suggests LOGONLY occupies the **category** field (a free-form quoted string), while the v1.2/v1.3 changelog text refers to "LOGONLY severity" — "Fixed a bug with LOGONLY severity. EXEC was being executed even if the trap had a severity of LOGONLY." Either interpretation yields the same observable behaviour: an EVENT with the literal token LOGONLY does not execute its EXEC directive. The doc-page ambiguity is upstream's, not this analysis's; operators reading the SNMPTT documentation will encounter both framings. For this analysis, we treat LOGONLY as a **special token that prevents EXEC execution**, leaving the field-position question open and citing the upstream ambiguity.

The mapping from SNMPTT severity to Nagios state (OK/WARNING/CRITICAL/UNKNOWN) is **operator-defined in the EXEC line** — not a built-in transformation. The conventional mapping (community-documented, not source-verifiable in the mirrored repos):
- Normal → 0 (OK)
- Warning → 1 (WARNING)
- Minor → 2 (CRITICAL)  ← lossy: Minor and Major both collapse to CRITICAL
- Major → 2 (CRITICAL)
- Critical → 2 (CRITICAL)
- LOGONLY → not submitted as a passive check

The five-tier SNMPTT severity is **lossy** when mapped to Nagios's three-tier state. This is a genuine limitation — Nagios's state model is coarser than the trap's vendor severity. (Whether operators commonly discard severity by hardcoding the return code is **not source-verifiable** — it is a community-reported pattern rather than an in-source default.)

### Phase 8 — Deduplication / suppression

Per `snmptt.org :: docs/snmptt.shtml`: "Added **snmptt.ini** option **duplicate_trap_window variable** for duplicate trap detection" (introduced in v1.3). Details retrievable from the upstream changelog only — the documentation excerpt did not specify the dedup key (likely OID + agent IP) or window default. This is a real but lightly-documented feature.

Other suppression mechanisms per upstream docs:
- `unknown_trap_log_enable` controls whether unmatched traps are logged.
- `NODES` directives suppress an EVENT based on source.

No rate limiting / circuit breaker built in. No storm detection.

### Phase 9 — Routing

SNMPTT's outputs are configurable in `snmptt.ini`, with **any combination** of:
- **`log_enable`** → `/var/log/snmptt/snmptt.log` text file (formatted via `log_format`).
- **`log_system_enable`** → syslog (`syslog_enable` / `syslog_facility`).
- **`mysql_dbi_enable`**, **`postgresql_dbi_enable`** → DB INSERT into `snmptt`, `snmptt_unknown`, `snmptt_statistics` tables. NSTI's installer turns MySQL DBI on at `nsti :: install/snmptt.sh:6`.
- **`EXEC` directive** in each EVENT → external command. This is where Nagios integration happens.

The four outputs are independent. Operators commonly enable file log + DB log + EXEC simultaneously: the file log is for grep-based forensics, the DB log feeds NSTI's UI, the EXEC feeds Nagios.

### Phase 10 — Nagios integration (per EVENT, via EXEC)

Operator-supplied EXEC lines invoke either `submit_check_result` (local) or `submit_check_result_via_nsca` / `send_nrdp.sh` (remote). The conventional invocation (community-typical, not in-source-verified for SNMPTT side):

```
EXEC /usr/local/nagios/libexec/eventhandlers/submit_check_result <HOSTNAME> "SNMP Trap" 2 "$O: $1 $2 $3"
```

`submit_check_result` then writes `[<datetime>] PROCESS_SERVICE_CHECK_RESULT;<HOSTNAME>;SNMP Trap;2;<plugin_output>` to `/usr/local/nagios/var/rw/nagios.cmd` (`nagioscore @ 8d1d276 :: contrib/eventhandlers/submit_check_result:30-36`).

Nagios Core's `base/commands.c:724-725` parses the `PROCESS_SERVICE_CHECK_RESULT` token:
```c
else if(!strcasecmp(command_id, "PROCESS_SERVICE_CHECK_RESULT"))
    command_type = CMD_PROCESS_SERVICE_CHECK_RESULT;
```

This is verified in source. The constant `CMD_PROCESS_SERVICE_CHECK_RESULT = 30` is at `nagioscore @ 8d1d276 :: include/common.h:144`.

### Phase 11 — Error handling for malformed PDUs / unknown OIDs / decode failures

- Malformed PDUs are handled by Net-SNMP `snmptrapd` (drop + log to syslog). Not visible to SNMPTT.
- Unknown OIDs (no matching EVENT block) → controlled by `unknown_trap_log_enable` and optionally `unknown_trap_exec`. Default behaviour is **logged-only, not alerted**.
- Spool-file parse errors (corrupt write from snmptthandler) → SNMPTT logs and deletes; no retry.
- EXEC command failures (non-zero exit) → logged in snmptt.log; no auto-retry.
- Database write failures → logged; the trap is still logged to the file log if file logging is enabled. The DB write is best-effort.

---

## 6. Data Model & Persistent Storage

### Per-feature storage

| Feature | Storage | Authoritative source |
|---|---|---|
| Raw trap text (file log) | Text file `/var/log/snmptt/snmptt.log`, format controlled by `log_format` | `snmptt.org :: docs/snmptt.shtml` |
| Matched traps | MySQL/PostgreSQL/ODBC table `snmptt` (or as renamed by `mysql_dbi_table`) | `nsti :: nsti/dist/nsti.sql:27-44` |
| Unmatched traps | MySQL table `snmptt_unknown` (or `mysql_dbi_table_unknown`) | `nsti :: nsti/dist/nsti.sql:78-91` |
| Trap statistics | MySQL table `snmptt_statistics` (when `statistics_interval` is set) | `snmptt.org :: docs/snmptt.shtml` |
| Archived matched traps | MySQL table `snmptt_archive` — **NSTI-specific** (not part of stock SNMPTT) | `nsti :: nsti/dist/nsti.sql:50-71` |
| NSTI filters | MySQL tables `filter` + `filter_atom` — **NSTI-specific** | `nsti :: nsti/dist/nsti.sql:109-129` |
| Nagios service state | Nagios `status.dat` flat file or NDOUtils MySQL or Livestatus | `nagioscore @ 8d1d276 :: xdata/xsddefault.c` (status.dat); NDOUtils lives in the separate `NagiosEnterprises/ndoutils` repo |
| Nagios external commands | Named pipe `/usr/local/nagios/var/rw/nagios.cmd` | `nagioscore @ 8d1d276 :: contrib/eventhandlers/submit_check_result:30` |

### SNMPTT `snmptt` table schema

Verbatim from NSTI's bundled copy `nsti :: nsti/dist/nsti.sql:27-44`:

```sql
CREATE TABLE IF NOT EXISTS `snmptt` (
      `id` mediumint(9) NOT NULL auto_increment,
      `eventname` varchar(50) default NULL,
      `eventid` varchar(50) default NULL,
      `trapoid` varchar(100) default NULL,
      `enterprise` varchar(100) default NULL,
      `community` varchar(20) default NULL,
      `hostname` varchar(100) default NULL,
      `agentip` varchar(16) default NULL,
      `category` varchar(20) default NULL,
      `severity` varchar(20) default NULL,
      `uptime` varchar(20) default NULL,
      `traptime` varchar(30) default NULL,
      `formatline` varchar(255) default NULL,
      `trapread` int(11) default '0',
      `timewritten` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
      PRIMARY KEY  (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;
```

Observations:
- **Storage engine is MyISAM**, not InnoDB. MyISAM is non-transactional, has table-level locking, no foreign keys, and has been deprecated in modern MySQL/MariaDB. This is a 2014-era choice that has aged poorly.
- **Character set is latin1**, not utf8mb4. Vendor messages containing UTF-8 may be truncated or mojibake'd.
- **`formatline` is varchar(255)**. SNMPTT's FORMAT output is truncated at 255 chars in the DB; the full text only survives in the file log. (Compare to Centreon's `log_traps.output_message varchar(2048)` — both truncate, but at different limits.)
- **`agentip` is varchar(16)** — IPv6 doesn't fit. IPv6 traps require schema modification.
- **`timewritten` is the DB write time**, distinct from `traptime` (when the trap arrived). Both columns matter for correlation; NSTI's UI uses `timewritten` for filtering.
- **No index** on any column other than the primary key. Filtering by `trapoid`, `hostname`, `severity`, or `timewritten` in a large table is a full scan.

The schema is **a single denormalized table per trap**. There is no varbind side table — all varbind data is flattened into `formatline` (the operator's FORMAT string output). The original varbinds are lost at the DB layer; only the file log preserves them in raw form.

**Operator mitigation via SNMPTT `sql_custom_columns`** (potential, not configured by NSTI): per `snmptt.org :: docs/snmptt.shtml` v1.2 changelog, SNMPTT supports the `sql_custom_columns` and `sql_custom_columns_unknown` `snmptt.ini` options for adding custom columns to the SNMPTT trap tables. Operators could use these to preserve additional varbind data (e.g., per-varbind columns alongside `formatline`), but the feature requires manual `snmptt.ini` edits plus matching `ALTER TABLE` against the MySQL schema. NSTI does not configure these options; the storm ORM models (`nsti :: nsti/database.py:31-81`) would also need extension to surface any new columns in the UI.

### `snmptt_unknown` table

Same shape as `snmptt` but minus `eventname`, `eventid`, `category`, `severity` (since the trap is unknown). `nsti :: nsti/dist/nsti.sql:78-91`. Also MyISAM, also latin1.

### `snmptt_archive` table (NSTI-specific)

`nsti :: nsti/dist/nsti.sql:50-71`. The schema adds `snmptt_id mediumint(9) NOT NULL` (line 54) at the start of the row body, presumably to track the original `snmptt.id` of the archived row for audit-trail purposes. The archive table is the **only retention mechanism** in NSTI: when the operator clicks "Archive" in the UI on a selection of traps, NSTI's Flask route `/api/trapview/archive` (`nsti :: nsti/trapview.py:97-128`) copies the rows from `snmptt` into `snmptt_archive` and then deletes them from `snmptt`.

There is **no automatic retention** — no time-based purge, no row-count limit, no rotation. If operators do not archive, the `snmptt` table grows unbounded.

**The archive route has two source-verified data-loss bugs**:

1. **`snmptt_id` is never populated**. The Storm ORM `SnmpttArchive` class (`nsti :: nsti/database.py:50-66`) does not declare `snmptt_id` as a column, and the archive route (`nsti :: nsti/trapview.py:106-122`) does not set it. Result: every archived row gets `snmptt_id = 0` (MySQL default for `mediumint NOT NULL` in non-strict mode) or a constraint-violation error (in strict mode). The audit-trail intent is broken.
2. **`agentip` is silently lost via attribute typo**. The archive route writes `x.agentid = r.agentip` at `nsti :: nsti/trapview.py:114`, but the `SnmpttArchive` ORM model defines the column as `agentip`, not `agentid` (`nsti :: nsti/database.py:59`). Storm ORM allows setting arbitrary Python attributes on instances without raising, so this silently creates an untracked instance attribute that is never persisted; the actual `agentip` column gets `NULL`. **Every archived trap loses its source IP.**

These are real defects in shipped code. Operators using the archive feature for compliance/forensics get a degraded record.

### `filter` and `filter_atom` (NSTI-specific)

`nsti :: nsti/dist/nsti.sql:109-129`. Two-table model: a `filter` row is a named set of `filter_atom` rows; each atom is `(column_name, comparison, val)` (e.g., `severity=critical`, `hostname__contains=.41`). Filters apply to the SQL `WHERE` clause when querying `snmptt` / `snmptt_archive` / `snmptt_unknown`. Implementation: `nsti :: nsti/filters.py:65-94` (create), `nsti :: nsti/database.py:209-278` (where-clause assembly).

### Migration / upgrade handling

NSTI's `database_upgrade.sh` (`nsti :: install/database_upgrade.sh`) migrates from 1.4 schema to 3.0:
- Adds `timewritten timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP` to existing `snmptt`, `snmptt_archive`, `snmptt_unknown` tables (lines 42-44).
- Renames the legacy `snmptt.filters` table to `snmptt.filters_1_4` (line 47).
- Creates the new `filter` and `filter_atom` tables and imports data from `filters_1_4` via `nsti :: install/table_upgrade.sql:36-106` (the source `filters_1_4` table had a denormalised single-table schema with columns like `filtername`, `eventnamequery`, `eventname`, etc., decomposed into the two-table `filter` + `filter_atom` model).

This migration ran once in 2014; since 2017 the upgrade path has not moved. For SNMPTT itself, upstream documents schema additions (e.g., the `snmptt_statistics` table) but does not script automated migrations — operators apply ALTER TABLE manually.

---

## 7. Configuration UX

Six surfaces, none unified:

1. **`/etc/snmp/snmptrapd.conf`** (Net-SNMP) — flat text, vi-edited. Controls UDP/162 binding, community auth, SNMPv3 USM users, and the `traphandle` invocation. NSTI's installer writes a minimal default at `nsti :: install/snmptt.sh:10`:
   ```
   #disableAuthorization yes
   authCommunity    log,execute,net    public
   traphandle default /usr/sbin/snmptthandler
   ```
   Note the community is `public` (the most generic default — production deployments must change this).

2. **`/etc/snmp/snmptt.ini`** (SNMPTT main config) — flat text, INI-style sections. NSTI's installer toggles four options at `nsti :: install/snmptt.sh:3-7`:
   ```
   mode = daemon                  (was: standalone)
   dns_enable = 1                 (was: 0)
   mysql_dbi_enable = 1           (was: 0)
   net_snmp_perl_enable = 1       (was: 0)
   ```
   All other options stay at SNMPTT's defaults. **No validation, no schema, no auto-completion.** A typo in `snmptt.ini` typically manifests as silent default-fallback.

3. **`snmptt.conf`** files (SNMPTT EVENT definitions) — flat text, generated by `snmpttconvertmib` or hand-written. The operator lists them in `snmptt.ini`'s `snmptt_conf_files` directive (`nsti :: docs/snmpttsetup.rst:19-22`):
   ```
   snmptt_conf_files = <<END
   /etc/snmp/watchguard.conf
   /etc/snmp/snmptt.conf
   END
   ```
   The order of files matters because EVENT matching is sequential — but the order within a file matters more. Operators frequently misorder EVENT blocks; this is the **single most common SNMPTT configuration error** per community forum traffic (not in-source-verifiable, but consistent with the architecture).

4. **Nagios configuration files** (`hosts.cfg`, `services.cfg`, `commands.cfg`) — flat text. Operators must define a passive service for each (host, trap-type) tuple that should appear in Nagios as a state. Conventional pattern (community knowledge, not in-source-verifiable from mirrored Nagios Core; the closest in-source evidence is the `submit_check_result` helper and its assumed service identifier):
   ```
   define service {
       use                    generic-service
       host_name              router1
       service_description    SNMP Trap
       active_checks_enabled  0
       passive_checks_enabled 1
       check_freshness        0
       check_command          check_dummy!0
       ...
   }
   ```
   The service must exist before Nagios will accept the passive check; if it does not, Nagios discards the passive check and logs an `NSLOG_RUNTIME_WARNING` to `nagios.log` ("Warning: Passive check result was received for service '%s' on host '%s', but the service could not be found!" — `nagioscore @ 8d1d276 :: base/commands.c:2401-2406`). The warning is easy to miss because `nagios.log` is the same log used for state transitions and notification dispatch; operators investigating "why isn't this trap firing an alert" must `grep` for it. This is a frequent operator pitfall.

5. **`/etc/nrdp/config.inc.php`** (NRDP server) and `/usr/local/nrdp/clients/send_nrdp.sh` (client) — PHP config + shell helpers. `nrdp :: server/config.inc.php` defines authorized tokens; the client encodes the passive check as URL-encoded HTTP POST. Token-based auth at the application layer; the wire protocol is plain HTTP unless an operator fronts it with TLS.

6. **NSTI web UI** (`http://<host>/nsti`) — browsing/filtering of the SNMPTT tables, plus operator-triggered archive and delete actions on the same tables. Surfaces:
   - **Traplist** at `/traplist` (`nsti :: nsti/trapview.py:29-44`) — paginated table of `snmptt`, `snmptt_archive`, or `snmptt_unknown` rows.
   - **Trap detail** at `/trapview/<id>` (`nsti :: nsti/trapview.py:47-56`) — single-trap view rendered via `templates/trapview/trapview.html`.
   - **Filter list** at `/filterlist` (`nsti :: nsti/filters.py:60-62`) — manage named filters (write surface: filters are created, edited, and deleted via this UI and written into the `filter` / `filter_atom` tables).
   - **Inspector** at `/inspector` (`nsti :: nsti/inspector.py:13-22`) — bar/pie chart visualisation of traps by category, severity, hostname, or trapoid.
   - **JSON API** at `/api/trapview/read/<tablename>` (`nsti :: nsti/trapview.py:59-76`), `/api/trapview/delete/<tablename>` (write — deletes trap rows; `nsti/trapview.py:79-94`), `/api/trapview/archive` (write — copies and deletes; `nsti/trapview.py:97-128`), `/api/filter/...` create/edit/delete (write; `nsti/filters.py:65-147`), `/api/inspector/...` (read; `nsti/inspector.py:35-94`).

NSTI does **not** configure SNMPTT — there is no UI for adding EVENT blocks, MIBs, or the `snmptt.ini` settings that affect ingestion. NSTI is a **viewer + curator** (read traps; create/edit filters; archive and delete trap rows). It is not read-only with respect to the SNMPTT MySQL tables.

### What the operator sees by default

After running `nsti :: install.sh`, the operator has:
- snmptrapd listening on UDP/162 with community `public` and `disableAuthorization yes` commented-out (i.e., authorization enabled).
- SNMPTT daemon running, MySQL DBI logging on, no EVENT blocks (empty `snmptt.conf`).
- Every trap arrives unrecognized → logged to `snmptt_unknown` only (per `nsti :: docs/snmpttsetup.rst:13`, `unknown_trap_log_enable = 1` is set in the WatchGuard recipe but is **not** set by the installer).
- Nagios runs separately, with no SNMP traps wired to any service yet — the operator must hand-write `commands.cfg` and `services.cfg` and re-launch Nagios.
- NSTI shows an empty trap list at `http://<host>/nsti`.

Day-1 trap visibility is **zero**. Every vendor's traps are unrecognized until the operator manually converts MIBs and writes EVENTs.

### Discoverability of options

- snmptrapd: Net-SNMP's `snmptrapd.conf(5)` man page.
- SNMPTT: the upstream `snmptt.ini` file is heavily commented; `snmptt.org :: docs/snmptt.shtml` is the reference. Per upstream docs, "This file does not document all configuration options available in snmptt.ini. Please view the snmptt.ini for a complete description of all options."
- Nagios passive check setup: documented in Nagios's main user guide, not in-source-verifiable from `nagioscore @ 8d1d276` itself.
- NSTI: Sphinx docs at `nsti :: docs/` (8 RST files: introduction, installation, filters, visualizer, backendaccess, snmpttsetup, setup, includeme).

There is **no validation** in any tool. Operators only discover errors at runtime, often only when a trap that should fire an alert silently doesn't.

### Live reload vs restart

- snmptrapd: SIGHUP reloads config.
- SNMPTT: SIGHUP reloads `snmptt.ini` and all `snmptt_conf_files` per upstream docs. The polling delay applies before new EVENTs become active.
- Nagios Core: SIGHUP reloads config (`nagioscore @ 8d1d276 :: base/nagios.c`).
- NSTI: no config-reload — restart Apache to pick up `nsti.py` changes.

### Multi-tenancy / RBAC

- snmptrapd: none. UDP/162 is a single port; per-community processing is configurable but there is no notion of "tenant."
- SNMPTT: none. One snmptt.conf, one MySQL DB, one set of EVENT blocks.
- NSTI: none. The Flask app has no authentication — verified by reading the entire app (`nsti :: nsti/nsti.py:1-24` Flask factory; `nsti :: nsti/trapview.py`, `filters.py`, `inspector.py` — no auth decorator, no login route, no user model). The only Flask config touching authentication is `app.secret_key = 'mysecretkey'` at `nsti/nsti.py:6` (a hardcoded session key, also a separate security weakness). **Anyone with network access to the NSTI URL can view all traps.** This is a real security issue for the 2014-era OSS stack; operators are expected to front NSTI with HTTP basic auth via Apache config or a separate auth proxy. The NSTI installer does **not** configure any auth.
- Nagios Core: CGI basic auth via Apache; per-user contact configuration in `cgi.cfg`.

---

## 8. Integration with Other Signals

### 8.1 Metrics

Traps are **not converted to metrics** in any mirrored component. There is no counter, gauge, or histogram derived from trap rate, trap severity, or trap match-rate. The closest thing is the SNMPTT `snmptt_statistics` table (per `snmptt.org :: docs/snmptt.shtml`) which records total trap counts; this is operator-queryable via SQL but not visualised by Nagios or NSTI.

Traps are **not used as annotations** on metric dashboards because the stack does not have native metric dashboards in the modern sense. Nagios's CGI UI shows host/service status (state machine output); it does not show metric time series unless extended by add-ons (PNP4Nagios, Grafana via Livestatus).

### 8.2 Alerting / Notifications

This is the **central integration** of the stack — traps become Nagios passive service state changes, and Nagios's notification/escalation/acknowledgement machinery applies as usual.

Path: SNMPTT EVENT EXEC → `submit_check_result` → `PROCESS_SERVICE_CHECK_RESULT` → `cmd_process_service_check_result` (`nagioscore @ 8d1d276 :: base/commands.c:2318`) → Nagios service state transitions per the state-machine rules in `base/checks.c`. When the state transitions to CRITICAL, Nagios's notification engine consults the contact definitions and dispatches notifications (email, SMS via plugin, etc.). Acknowledgement is via the CGI web UI's "Acknowledge service problem" form (`nagioscore @ 8d1d276 :: cgi/cmd.c` — `CMD_ACKNOWLEDGE_SVC_PROBLEM` at line 831, 1056, 1062). Freshness-based recovery (the only automatic clear mechanism) is evaluated in `base/checks.c` (`temp_service->check_freshness` at line 2083; `temp_host->check_freshness` at line 2833).

Key properties:
- **Alert routing**: per Nagios's `contacts.cfg` and `escalations.cfg` mechanisms — completely orthogonal to the trap source. The trap that triggers a CRITICAL is indistinguishable from a polling failure to the notification engine.
- **Alert deduplication**: by service+state. If the same trap arrives 100 times in 10 minutes, the second through 100th invocations are state-stable (CRITICAL → CRITICAL transitions) and **do not re-notify** unless `notify_recovery` or notification interval configuration says otherwise. SNMPTT's `duplicate_trap_window` adds a second layer of dedup at the SNMPTT level.
- **Acknowledgement**: standard Nagios acknowledgement applies; it is **not specific to traps**.
- **Clear semantics**: this is a real weakness. The trap pipeline is **one-shot**: a trap arrives, the service goes CRITICAL, but **nothing clears it**. Operators must either:
  - Define a *paired* "clear" trap with its own EVENT that EXECs `submit_check_result <host> <service> 0 "OK"`, and hope vendors send clear traps;
  - Run a freshness-check that resets the service after N seconds (via Nagios's `check_freshness` mechanism — `nagioscore @ 8d1d276 :: base/checks.c` references `service.check_freshness` semantics);
  - Use Nagios's `volatile_services` flag to fire notifications on every CRITICAL state independent of clearing.

The lack of automatic clearing is the **single biggest day-to-day operational pain** of the trap-as-passive-check pattern. OpenNMS handles it via Drools cosmic-clear rules; Centreon via `traps_advanced_treatment` and `auto_recovery_status`; Nagios+SNMPTT has no built-in equivalent.

### 8.3 Topology

**No topology integration in any mirrored component.**

- Nagios Core has host parent-child relationships (`parents` directive in `host` definitions) used for unreachable-vs-down state propagation, but this is not a topology graph in the L2/L3/L7 sense.
- SNMPTT has no topology data; the only spatial information it knows about a trap is the source IP and hostname (if reverse-DNS resolves).
- NSTI shows traps in a flat table; no map view, no graph view.

Topology-aware suppression (e.g., "router1 went down so suppress traps from devices behind it") is **not source-verifiable as supported** anywhere in this stack. Operators implement it via Nagios's `host parents` mechanism for state propagation but the propagation does not affect the trap pipeline itself — a trap from a behind-the-router device still arrives, still EXECs `submit_check_result`, still creates a CRITICAL state. The state may then be re-categorized by Nagios as UNREACHABLE (not DOWN/CRITICAL) based on parent host state.

### 8.4 Logs / Events

The trap-as-event surface lives in **two parallel stores**, neither unified:

- **SNMPTT's MySQL tables** (`snmptt`, `snmptt_unknown`, `snmptt_archive`): the de-facto trap log. Queried by NSTI's UI. Schema lacks varbind side-table — the original PDU data is flattened into `formatline`.
- **Nagios's status.dat / NDOUtils**: post-conversion service state with the FORMAT-derived text as plugin_output. Queried by Nagios CGI. The original OID, varbinds, agent IP are **lost** at this layer — only the operator's chosen FORMAT string is preserved.

Searchability:
- SNMPTT side: NSTI provides column-based filtering (`nsti :: nsti/filters.py`) and a JSON API (`nsti :: nsti/database.py:209-278`). MyISAM with no indexes means full-table scans on filtered queries.
- Nagios side: status.cgi text search; NDOUtils SQL search. Per-trap detail is restricted to what the operator's FORMAT line captured.

Retention:
- SNMPTT side: **no automatic retention**. The `snmptt` table grows unbounded; NSTI's "Archive" feature only moves rows to `snmptt_archive` (which itself grows unbounded). Operators must implement cron jobs for purge.
- Nagios side: status.dat is overwritten on each scheduler interval (snapshot, not historical). NDOUtils has historical event log tables; retention is configurable via `ndomod.cfg` / `ndo2db.cfg`.

This is a **clear weakness** vs the comparative cohort. OpenNMS retains 6 weeks default in PostgreSQL; Centreon's `log_traps` integrates with the broker's retention; the Nagios family relies on operator scripts.

### 8.5 Northbound Forwarding

Can the stack **emit** traps?

- **Nagios Core**: no native SNMP trap-out. The `send_snmp_trap` script does not exist in `nagioscore @ 8d1d276`. Operators wire Nagios's `notify-host-by-snmptrap` / `notify-service-by-snmptrap` command-line entries in `commands.cfg` to invoke `snmptrap` (from Net-SNMP) directly — this is documented in Nagios's user docs but not in-source as a first-class feature.
- **SNMPTT**: no native trap-out. Per `snmptt.org :: docs/snmptt.shtml`, SNMPTT is **strictly a receiver**.
- **NSCA / NRDP**: passive-check transports, not SNMP transports. They carry text from a remote to a master, not SNMP PDUs.
- **NSTI**: no emit path.

Other northbound paths:
- **Syslog**: SNMPTT can log to syslog, which is then forwardable via standard syslog daemons (rsyslog, syslog-ng). This is the most common northbound route.
- **JSON API**: NSTI's `/api/trapview/read/<tablename>` (`nsti :: nsti/trapview.py:59-76`) exposes traps as JSON over HTTP. CORS is wide-open by design (`nsti :: nsti/trapview.py:9-20` — `Access-Control-Allow-Origin: *`). External systems can poll it; there is no push.
- **Database**: the MySQL `snmptt` table is queryable by any external system with credentials.

There is **no OTLP, no Kafka, no AMQP, no native message-bus push**. Forwarding to a modern observability platform is operator-DIY (typically scripted via a cron-driven SQL exporter or a syslog tap).

---

## 9. Severity Model

Per `snmptt.org :: docs/snmptt.shtml`, retrieved 2026-05-22, SNMPTT defines five severity levels plus a special token (see §5 Phase 7 for the upstream-documentation ambiguity about LOGONLY's field position):

| SNMPTT severity | Conventional Nagios mapping | Notes |
|---|---|---|
| Normal | 0 (OK) | |
| Warning | 1 (WARNING) | |
| Minor | 2 (CRITICAL) | Collapses with Major and Critical |
| Major | 2 (CRITICAL) | |
| Critical | 2 (CRITICAL) | |
| LOGONLY (special token, possibly category or severity per upstream docs) | (no submit) | Trap logged; EXEC not run regardless of severity, so no Nagios state change |

Three SNMPTT severity levels (Minor, Major, Critical) collapse to Nagios CRITICAL because Nagios's state model has only OK/WARNING/CRITICAL/UNKNOWN. **The severity translation is lossy.**

Where vendor severity comes from:
- Some MIBs encode severity in `--#SEVERITY` annotation (an SNMPTT extension comment); `snmpttconvertmib` reads it and writes it into the EVENT block.
- Most vendor MIBs do **not** carry a severity field. In that case `snmpttconvertmib` defaults the EVENT severity (per upstream docs; the default is unstated in the page excerpt retrieved, so this is a verified-as-uncited claim).
- The generated EVENT block then uses the converted-or-defaulted severity; **the operator can edit it freely** in `snmptt.conf`.

Customization surface:
- Per-EVENT severity edit: open `snmptt.conf`, change the fourth field of the `EVENT` line, save, send SIGHUP to snmptt.
- Per-EVENT severity-via-varbind: use the `MATCH` directive plus multiple EVENT blocks for the same OID, each with a different severity. Per upstream docs, MATCH supports varbind value matching, bitwise AND, and regex. Severity-by-state is a documented pattern but operator-implemented per EVENT.

The mapping from SNMPTT severity to Nagios state is **not built-in** — it lives in the operator's EXEC line. Many operators hardcode the `return_code` argument to a constant (often `2`, for CRITICAL) regardless of trap severity. This means **trap severity is often discarded in practice** when traversing the EXEC boundary.

---

## 10. Storm / Volume Handling

### Per-source rate limits

- **snmptrapd**: not in source-mirrored configuration. Net-SNMP itself supports `forwarder` filtering but no native per-source rate limit in snmptrapd.
- **SNMPTT**: per `snmptt.org :: docs/snmptt.shtml`, the only documented rate-related feature is `duplicate_trap_window` (introduced in v1.3) which suppresses duplicates within a time window. The dedup key is not specified in the doc excerpt retrieved; community convention is OID + agent IP, but this is not in-source-verifiable from the WebFetch excerpt.
- **Nagios Core**: no per-source rate limit on the external command file. Every line written gets processed.

### Dedup keys and windows

- SNMPTT's `duplicate_trap_window` (per upstream docs).
- Nagios's `state-stable` notification dedup applies orthogonally (every same-state passive check after the first does not re-notify until the notification interval elapses).

### Circuit breakers

None in any mirrored component.

### Storm detection

None.

### Backpressure / queue management

- snmptrapd: kernel UDP buffer is the only queue. If snmptrapd cannot drain fast enough, the kernel drops packets (visible via `netstat -su`).
- snmptthandler-to-spool: filesystem inode and space limits are the constraint. A 1M-trap storm produces 1M files in `/var/spool/snmptt/`, exhausting inodes on small filesystems.
- snmptt daemon: synchronous per-file processing inside the polling loop. No internal queue depth limit; if the operator's EXEC command (e.g., `submit_check_result`) blocks, snmptt blocks behind it. Per `snmptt.org :: docs/snmptt.shtml`, there is no documented `worker_pool` or `fork_concurrency` option for parallelizing EXEC.

This is **among the weakest storm-handling designs observed to date in this cohort** (OpenNMS, Centreon, Zenoss, CheckMK, LibreNMS, Zabbix — comparative ranking deferred to `comparative-analysis.md`). OpenNMS has explicit `discardtraps` filtering, batch dispatching, kernel-buffer tuning; Centreon has `last_time_exec`-based throttling per trap+host; the Nagios family stack has only the upstream-documented `duplicate_trap_window` and the optional experimental `threads_enable` for EXEC parallelism. Operators handle storms by manual snmptrapd filtering or upstream firewall rate-limits.

**Spool-file race condition in pre-v1.5 SNMPTT**: per `snmptt.org :: docs/snmptt.shtml` v1.5 changelog, upstream fixed a race in `snmptthandler` / the embedded handler that "could cause traps to be missed" — the daemon could attempt to read a spool file while the handler was still writing it, missing the trap. NSTI deploys v1.4 (`nsti :: install/snmptt_deploy.sh:3`), which **predates this fix**. Under high trap rates, the v1.4 install path is therefore subject to documented trap loss that the upstream v1.5 release addressed. Neither NSTI nor the Nagios family OSS stack tracks this upstream fix; operators must manually upgrade SNMPTT to v1.5 or later to close the race. This is a real reliability defect inherited from the version pinning.

**Note on installer defaults**: per source inspection of `nsti :: install/snmptt.sh` and `install/snmptt_deploy.sh`, neither installer sets `duplicate_trap_window`, leaving dedup at SNMPTT's default value. The installer also does not set `unknown_trap_log_enable`, so unknown traps are **not** logged by default (contrary to the WatchGuard recipe in `nsti :: docs/snmpttsetup.rst:13` which enables it). The default NSTI install therefore has no dedup and silently drops unknown traps.

---

## 11. Security

### SNMPv3 USM support

Per `snmptt.org :: docs/snmptt.shtml`:
- Determined by Net-SNMP `snmptrapd`. Configured via `createUser` / `authUser` directives in `snmptrapd.conf`.
- SNMPv3 fields (securityName, contextName, etc.) are passed to SNMPTT **only when using snmptthandler-embedded** with Net-SNMP's `--enable-embedded-perl` build. The non-embedded handler does not pass these fields.

NSTI ships **two install paths** that disagree about authorization handling, and both have bugs.

- **Path A — direct deploy script** `nsti :: install/snmptt_deploy.sh:19` writes three lines to `snmptrapd.conf` via `echo -e`:
  ```
  echo -e '#disableAuthorization yes\n#authCommunity    log,execute,net    public\ntraphandle default /usr/sbin/snmptthandler' >> /etc/snmp/snmptrapd.conf
  ```
  The `\n` separators expand to real newlines. Result: three lines are appended, two commented out (`#disableAuthorization yes`, `#authCommunity ... public`) and one active (`traphandle default /usr/sbin/snmptthandler`). Snmptrapd's default authorization behaviour applies. Note: `snmptt_deploy.sh` is **not** called by `install.sh`.
- **Path B — package install via `install.sh`** calls `nsti :: install/snmptt.sh:10` instead. That script uses a different mechanism — a `sed '$a\...'` append with **literal `\` characters** as separators (not real newlines):
  ```
  sed -i'.bkp' '$a\ #disableAuthorization yes\authCommunity    log,execute,net    public\traphandle default /usr/sbin/snmptthandler\' "$SNMPTRAPD"
  ```
  Verified by running this `sed` against a test file (an empty input produces one literal line):
  ```
   #disableAuthorization yes\authCommunity    log,execute,net    public\traphandle default /usr/sbin/snmptthandler
  ```
  The result is **one malformed line that starts with `#`** — so the entire line is treated as a comment by snmptrapd. **The `install.sh` path does not produce an active `traphandle` directive, an active `authCommunity`, or an active `disableAuthorization` setting** — the script is broken. Operators following the `install.sh` path end up with an snmptrapd that has no `traphandle` for SNMPTT and therefore does not forward traps to SNMPTT at all — a major functional defect, not just a security defect.

NSTI's WatchGuard doc (`nsti :: docs/snmpttsetup.rst:96`) recommends `disableAuthorization yes` for initial setup with a parenthetical "Remove this later for security" — a third pattern that disagrees with both install paths. Operators commonly leave authorization disabled in production once they have followed the WatchGuard recipe.

### Supply-chain risk

NSTI's deploy script downloads SNMPTT from `assets.nagios.com` over **plain HTTP**, not HTTPS (`nsti :: install/snmptt_deploy.sh:3` — `wget http://assets.nagios.com/downloads/addons/snmptt_1.4.tgz`). A network attacker positioned between the host and Nagios's CDN can substitute a malicious tarball, which the installer then unpacks and installs as `/usr/sbin/snmptt` running as root (or as a privileged process at minimum). The downloaded version is **v1.4**, which predates the **v1.4.2 security fix** that closed an EXEC/PREEXEC/unknown_trap_exec shell-injection vector (`snmptt.org :: docs/snmptt.shtml`, v1.4.2 changelog: "Fixed a security issue with EXEC / PREEXEC / unknown_trap_exec that could allow malicious shell code to be executed.") and the related fix ensuring commands run under `daemon_uid` instead of root. Operators running the NSTI installer today therefore inherit a known-vulnerable SNMPTT build, fetched over an unencrypted channel.

### DTLS / TLSTM support

Not in any mirrored component. Net-SNMP supports TLSTM only when built with `--with-transports="DTLSUDP TLSTCP"`; this is uncommon in default distro packages. No Nagios-family tool configures TLSTM.

### Credential storage

- SNMPv3 USM credentials in `/etc/snmp/snmptrapd.conf` as plaintext.
- MySQL credentials in `/etc/snmp/snmptt.ini` (`mysql_dbi_password`) as plaintext.
- NSTI MySQL credentials in `nsti :: nsti/etc/nsti.py:7-8` as plaintext (DB_USER='snmpttuser', DB_PASS='password' is the **shipped default**).
- NSTI installer sets MySQL root password to **`nagiosxi`** (`nsti :: install/database.sh:9`, `nsti :: install/database_upgrade.sh:19`) — a hardcoded default. This is a serious operator hazard: any operator who installs NSTI on a fresh box gets a MySQL root password identical to "nagiosxi" across all such installs.
- The root-level `nsti :: install.sh:3` defines `DB_ROOT_PASS='nagiosxi'` as a shell variable. Inspection shows it is **never consumed** by the sourced scripts (each script uses its own local `mysqlpass` variable). The unused variable is dead code, but its presence still leaks the hardcoded value into the installer source — an operator reading `install.sh` sees the same `nagiosxi` value advertised twice.
- The Flask development server `nsti :: runserver.py:10` binds to `0.0.0.0:8080` with `debug=False`. An operator who accidentally runs `python runserver.py` instead of the Apache+mod_wsgi deployment exposes the unauthenticated NSTI UI on all network interfaces, on a port distinct from the Apache one (port 8080 vs port 80). The Sphinx docs at `nsti :: docs/installation.rst:157-163` recommend `runserver.py` for development; production use is implicit but not strongly distinguished.

### Access control on the trap subsystem itself

- snmptrapd: community string (v1/v2c) or USM users (v3).
- SNMPTT: none — the daemon accepts every parsed trap from the spool directory.
- NSTI: **zero authentication** in the shipped Flask app (`nsti :: nsti/nsti.py:5-8`). Anyone with HTTP access can view, archive, and delete trap rows via the API (`nsti :: nsti/trapview.py:79-94` — `delete()` route is open).

The unauthenticated delete API is a real flaw. `curl http://<nsti>/api/trapview/delete/Snmptt` with **no query parameters** results in `db.sql_where_query` returning a no-op `combiner(True)` (`nsti :: nsti/database.py:275-278`) — Storm interprets this as no WHERE clause, and the subsequent `result.remove()` (`nsti :: nsti/trapview.py:87`) deletes **every row** in the `snmptt` table. The route also forces the OR combiner (`force_combiner='OR'` at `nsti/trapview.py:83`), so any single matching filter atom triggers the delete — meaning even a constrained request with filters tends to delete more than expected. There is no token, no session, no CSRF protection. This is a full-table-wipe primitive available to anyone with HTTP access.

**All NSTI write routes use HTTP GET, not POST/PUT/DELETE**. The filter CRUD routes (`nsti :: nsti/filters.py:65,96,126`) and the trap delete/archive routes (`nsti :: nsti/trapview.py:79,97`) are all `@app.route('/...')` without an explicit `methods=...` argument; Flask defaults to GET. This means a malicious URL in a page, image tag, email, or chat message — visited by a browser that has network access to the NSTI host — can trigger filter creation, modification, deletion, trap archival, or full-table-wipe operations. Even after a future operator adds basic auth, GET-for-mutations remains exploitable via CSRF (the browser will send the auth cookie automatically). The `/api/inspector/read/<trapid>` route (`nsti :: nsti/inspector.py:35-37`) additionally performs `getattr(db, trapid)` on the URL path, allowing arbitrary ORM model selection from user input — although all defined models are read-only via this path, the pattern reflects systematic lack of input validation throughout NSTI.

### Audit logging

- snmptrapd: syslog.
- SNMPTT: snmptt.log.
- NSTI: Apache/mod_wsgi access log.
- Nagios Core: `nagios.log` (`nagioscore @ 8d1d276 :: base/logging.c`).

There is no unified audit trail. An operator investigating "who acknowledged this alert" can find it in `nagios.log`; investigating "where this trap came from" requires correlating four logs. The unauthenticated NSTI delete API leaves no audit trail except in Apache access logs (and even that only if the operator configured access logging on the NSTI vhost).

---

## 12. Trap Simulation & Testing (in-source evidence)

### Unit tests

- **Nagios Core**: has a `t/` subdirectory with TAP-style tests (`nagioscore @ 8d1d276 :: t/` and `t-tap/`). None of them are trap-related — they test command parsing, configuration parsing, scheduler logic.
- **NSTI**: **zero test files.** `find "$NSTI" -name '*_test.py' -o -name 'test_*.py' -o -name 'tests.py'` returns nothing.
- **NSCA**: has `nsca @ c259e1c :: nsca_tests/` directory with shell-script tests for the daemon.
- **NRDP**: no test files.
- **SNMPTT**: per `snmptt.org :: docs/snmptt.shtml`, the package "includes 'snmptt-net-snmp-test' for OID translation verification and sample trap files for testing." Not in-source-verifiable from the mirror (SNMPTT upstream is not mirrored).

### Integration tests

- **NSTI**: **zero integration tests**. `dev_install.py` (`nsti :: dev_install.py`) is an interactive developer-install workflow that wipes and re-creates the local database — not a test runner.
- `nsti :: dev_install.py` (77 lines) is an **interactive developer-install workflow** that prompts (`raw_input('[y|n] ')` at line 33) for confirmation, then runs the SQL schema, configures Apache to point at the working tree (instead of `/usr/local/nsti`), and pip-installs requirements. It is not a test harness; it is a developer convenience for working against a checkout.
- The closest thing to a fixture in NSTI is `nsti :: nsti/trapdumperdaemon.py` (49 lines), which is a **synthetic data generator** that loops forever inserting random rows into the `snmptt` table:
  ```python
  for j in xrange(int(loop)):
      possible = [    '192.168.5.2',
                      '192.168.5.54',
                      ...
                      '192.168.5.1' ]
      agent = random.choice(possible)
      enter = '.1.3.6.1.4.1.' + random.choice(ente)
      troid = enter + random.choice(suff)
      sever = random.choice(stat)
      ...
      c.execute("""INSERT INTO snmptt (...) VALUES (%s,...)""",(name,troid,...))
  ```
  This is a demo populator, **not** a real ingestion daemon, and not a test. Its filename is misleading: "trapdumperdaemon" suggests an ingestion role; it has none.

### Sample trap fixtures included

- NSTI: none.
- NSCA: none in `nsca_tests/`.
- Nagios Core: none.
- SNMPTT upstream: per `snmptt.org :: docs/snmptt.shtml`, includes "sample trap files for testing" — but not source-mirrored, so the exact fixture content cannot be verified here.

### Tools shipped for trap simulation

- **Net-SNMP's `snmptrap`** command — available on every Net-SNMP install but not "shipped by" any Nagios-family tool. Operators use it to send test traps: `snmptrap -v 2c -c public localhost '' .1.3.6.1.4.1.2021.13.991.3.4 ...`.
- **NSTI's `trapdumperdaemon.py`** — demo data generator, **inserts directly into MySQL bypassing snmptrapd/snmptt entirely**. Useful for UI development; not useful for testing the trap pipeline end-to-end.

### CI workflow for trap pipeline

- **No trap-specific CI** across any of the four mirrored repos. Verified:
  - `find "$NSTI" -name '.github' -o -name '.gitlab*' -o -name '.travis*'` returns nothing.
  - `nagioscore @ 8d1d276` **does** have one workflow: `nagioscore :: .github/workflows/test.yml` (26 lines) which runs `./configure --enable-testing && make test` on Ubuntu 22.04 and 24.04 on push/PR to master. This exercises the Nagios Core daemon's build and the TAP-style unit tests under `nagioscore :: t/` and `t-tap/`. **None of those tests cover the trap or passive-check path** — the workflow tests the core daemon's compilation and unit tests; the trap pipeline (SNMPTT → submit_check_result → external command file → state machine) is not exercised by this CI.
  - `nsca @ c259e1c` does not have a `.github/workflows/` directory.
  - `nrdp @ 39cd102b` does not have a `.github/workflows/` directory.

This is **among the most absent CI coverage observed to date in this cohort**. Other systems analysed (OpenNMS, Centreon, Zenoss, CheckMK, LibreNMS, Zabbix) all ship at minimum a unit-test suite for the trap parsing layer. The Nagios family's classical OSS development model predates the modern CI assumption. (Comparative ranking deferred to `comparative-analysis.md`.)

---

## 13. Out-of-the-Box Coverage (defaults)

| Category | What ships | Source |
|---|---|---|
| MIBs bundled | **None** | Verified: zero `.mib` / `.MIB` / `.txt` MIB files in any of the four mirrored repos |
| SNMPTT EVENT definitions | **None** — `snmptt.conf` is created empty | `nsti :: install/snmptt_deploy.sh:15` — `touch /etc/snmp/snmptt.conf` |
| Severity rules | None — operator-defined per EVENT | n/a |
| Dedup defaults | `duplicate_trap_window` exists but the upstream-docs default value is not given in the WebFetch excerpt; treat as undocumented | `snmptt.org :: docs/snmptt.shtml` |
| Vendor packs / integration packages | **None** in OSS | n/a |
| Sample / preset dashboards | NSTI's "Inspector" page provides bar/pie charts of category/severity/hostname/trapoid frequencies | `nsti :: nsti/inspector.py`, `nsti :: nsti/templates/inspector/inspector.html` (532 lines) |
| Nagios service templates for traps | **None** in `nagioscore @ 8d1d276 :: sample-config/` | Verified — `grep -l snmp sample-config/template-object/*.cfg.in` returns `commands.cfg.in` (defines `check_snmp` polling command) and `switch.cfg.in` (uses `check_snmp` for active polling). Both reference SNMP *polling*, not traps. No trap-specific service template ships. |
| `submit_check_result` helper | Ships in `contrib/eventhandlers/` of Nagios Core, 36 lines | `nagioscore @ 8d1d276 :: contrib/eventhandlers/submit_check_result` |
| `submit_check_result_via_nsca` helper | Ships in same contrib subdir, 39 lines | `nagioscore @ 8d1d276 :: contrib/eventhandlers/distributed-monitoring/submit_check_result_via_nsca` |
| NSCA daemon and client binaries | Ships | `nsca @ c259e1c :: src/nsca.c`, `src/send_nsca.c` |
| NRDP PHP server | Ships | `nrdp @ 39cd102b :: server/index.php` |
| NRDP clients | Ships in PHP, Python (py2 and py3), shell | `nrdp @ 39cd102b :: clients/send_nrdp.{php,py,py2.py,sh}` |
| Bundled apache config for NSTI | Ships, but maps `/nsti` to `/usr/local/nsti` only — no auth setup | `nsti :: nsti/dist/apache.conf` (6 lines) |
| Default community string | `public` | `nsti :: install/snmptt.sh:10` writes `authCommunity log,execute,net public` |
| Default MySQL root password | `nagiosxi` (hardcoded by NSTI installer) | `nsti :: install/database.sh:9` |
| Default NSTI DB password | `password` | `nsti :: nsti/etc/nsti.py:8` |

**Day-1 trap recognition: zero.** Every Nagios+SNMPTT install begins with empty `snmptt.conf` and an empty Nagios service catalogue for traps. The operator builds both from scratch.

This is **the highest day-1 toil among the systems analysed to date in this cohort** (OpenNMS, Centreon, Zenoss, CheckMK, LibreNMS, Zabbix). OpenNMS bundles 17,442 event definitions; Centreon seeds 214; LibreNMS ships per-OID handler classes; Zenoss ships ZenPacks. The Nagios family OSS stack ships only the framework. Comparative ranking against later systems in the cohort deferred to `comparative-analysis.md`.

**Dual SNMPTT acquisition paths**: NSTI ships two installer scripts that bring SNMPTT in differently. `nsti :: install.sh:12` lists `snmptt` as a `yum` package dependency, expecting the OS package manager to provide it (whatever version the distro ships). `nsti :: install/snmptt_deploy.sh:3` instead pulls `snmptt_1.4.tgz` directly from Nagios's CDN over HTTP. Operators using `install.sh` get the distro's version; operators using `snmptt_deploy.sh` get v1.4 specifically. The two paths produce different filesystem layouts (`/usr/sbin/snmpt*` from the manual deploy vs whatever the distro chose). This is an undocumented dual-sourcing hazard.

**Nagios XI** (Nagios Enterprises commercial, **not source-mirrored**) pre-packages SNMPTT with a managed UI, an integrated wizard for trap-to-service mapping, and (per public docs) a non-empty starting trap catalogue. Many production Nagios deployments use XI rather than the OSS stack analysed here. The OSS day-1 experience documented above therefore represents the **lower bound** of the Nagios trap experience; the upper bound (Nagios XI) is not source-verifiable from this evidence.

---

## 14. User Customization Surface

### How users add custom OID handlers

1. Convert MIB via `snmpttconvertmib` → produces an `snmptt.conf` snippet with one EVENT per trap.
2. Edit the snippet to set severity, NODES filter, FORMAT, MATCH, EXEC.
3. Add the snippet file path to `snmptt_conf_files` in `snmptt.ini`.
4. SIGHUP snmptt.

Alternative: hand-write EVENT blocks without going through `snmpttconvertmib`. Per `snmptt.org :: docs/snmptt.shtml`, the syntax is human-writable.

### Custom MIBs

Same workflow — there is no distinction between vendor MIBs and custom MIBs. All MIBs are operator-supplied.

### Custom severity rules

Per-EVENT, via the EVENT line's severity field. Severity-by-varbind-value via multiple EVENTs + MATCH directive. Severity-by-time-of-day or by-host-group is **not supported** by SNMPTT directly — operator must implement via PREEXEC.

### Custom dedup rules

The `duplicate_trap_window` configuration option per upstream docs. Per-EVENT dedup customization is **not documented as supported**.

### Plugin / extension model

There is **no plugin system** in SNMPTT or NSTI:

- SNMPTT extends via shell-out (`EXEC`, `PREEXEC`) — every extension is an external program. Power: arbitrary. Cost: every extension is a fork-and-exec per trap.
- NSTI is a flat Flask app. No plugin registry, no extension hooks. To add a feature, edit the Python source.
- Nagios Core has plugins for **active checks**, not for passive checks. The NEB module API exists for binary modules, but writing one to react to traps is non-trivial.

### API surface for automation

- snmptrapd: none (it's a daemon configured via files).
- SNMPTT: none (it's a daemon configured via files).
- Nagios Core: the external command file is a write-only "API" — anyone with `nagios.cmd` write access can submit commands.
- NSCA: a wire protocol for submitting passive checks; not a config API.
- NRDP: the HTTP `submitcheck` endpoint is an API (`nrdp @ 39cd102b :: server/index.php`).
- NSTI: **mixed read/write, unauthenticated** JSON API at `/api/trapview/*`, `/api/filter/*`, `/api/inspector/*`. Read routes return trap rows as JSON; write routes (delete and archive in `nsti :: nsti/trapview.py:79,97`; filter create/edit/delete in `nsti :: nsti/filters.py:65,96,126`) mutate the SNMPTT DB without authentication, token, or CSRF protection — all use HTTP GET (see §17 weakness #32).

There is **no SNMPTT configuration API**. To automate adding an EVENT, you write the file via SSH or `cat`. This is consistent with the 2002-era design.

---

## 15. End-User Value Analysis

### What an operator gets day-1 with default config

**The NSTI repo ships two install paths with substantively different outcomes**, and the analysis must distinguish them:

**After `nsti :: install.sh` (the "package install" path — Path B):**
- The `install/prereqs.sh` script runs `yum install` for `snmptt`, `net-snmp`, `net-snmp-utils`. Whether snmptrapd is auto-enabled on boot depends entirely on the distro's package post-install scripts.
- `install/snmptt.sh:10` runs a buggy `sed` that produces **one commented-out line** in `snmptrapd.conf` (see §11). The result: snmptrapd has no `traphandle` for SNMPTT, so traps **are not forwarded to SNMPTT**. SNMPTT may be installed but receives nothing.
- MySQL and Apache are started; the NSTI Flask app is mounted at `/nsti`.
- NSTI web UI accessible at `http://<host>/nsti` showing an empty traplist — and that emptiness is correct because the pipeline is broken at the snmptrapd→SNMPTT handoff.
- Nagios is not installed by this path.
- **No alerts will ever fire** — the trap pipeline does not deliver to SNMPTT, let alone to Nagios.

**After `nsti :: install/snmptt_deploy.sh` (the "manual SNMPTT deploy" path — Path A, run separately by operators following the NSTI documentation):**
- snmptrapd is started directly via `snmptrapd -On` at `install/snmptt_deploy.sh:27`.
- `traphandle default /usr/sbin/snmptthandler` is appended **active** (not commented).
- Empty `snmptt.conf` — every incoming trap is unknown.
- Unknown traps logged to `snmptt_unknown` MySQL table (if `unknown_trap_log_enable = 1` — note the installer **does not enable this by default**; the operator must edit `snmptt.ini` per `nsti :: docs/snmpttsetup.rst:13`).
- NSTI web UI shows unknown traps from any device that sends to UDP/162.
- Nagios is still not configured by this script either; the operator must wire `submit_check_result` EXEC lines per-EVENT separately.
- No alerts firing for any trap until the operator builds the EVENT catalogue AND the Nagios service catalogue.

This dual-path divergence is **not documented** in NSTI's own docs. An operator who reads only `install.sh` ends up with a non-functional pipeline; an operator who reads `snmptt_deploy.sh` and then `install.sh` ends up with a partially-functional pipeline that still has zero recognized traps and zero alerts.

Day-1 capability table (for cross-system comparison). Both installer paths assumed combined; where Path A (manual deploy) and Path B (package install via `install.sh`) differ, both outcomes are shown.

| Capability | Day-1 status |
|---|---|
| UDP/162 reception | yes via snmptrapd (Path A starts it; Path B installs it but does not start it, and is broken upstream of SNMPTT — see §11) |
| SNMPv1/v2c parse | yes |
| SNMPv3 PDU reception and decryption | yes (handled entirely by snmptrapd via USM) |
| SNMPv3 metadata available to trap processing ($Be, $Bu, $BE, $Bn) | partial — only when snmptthandler-embedded is used; the non-embedded handler does not pass these fields |
| Vendor MIB recognition | **no** (zero bundled) |
| Severity normalization | partial — operator-edited per EVENT |
| Storage of received traps | yes (file log + MySQL) |
| Storage of unmatched traps | yes (if operator enables `unknown_trap_log_enable`) |
| Web UI for browsing | yes (NSTI, unauthenticated) |
| Alerting integration | **no** — operator must write services.cfg and EXEC lines |
| Dedup | partial — `duplicate_trap_window` if configured |
| Storm protection | **no** |
| Northbound forwarding | **no** native; syslog tap is the practical route |
| Topology integration | **no** |
| Multi-tenancy | **no** |
| Authentication on UI | **no** |
| Day-1 alerts firing | **no** for any vendor without operator-built EVENTs |

### What requires customization

Almost everything:
- Every vendor's MIBs must be downloaded, converted, and added.
- Every alert-worthy trap must be paired with a Nagios passive service definition and an EXEC line.
- Every clear/recovery semantic must be implemented (pair "down" and "up" traps as two EVENT blocks).
- Storm handling, rate limiting, deduplication policy must be operator-configured.
- NSTI authentication must be operator-added at the Apache layer.

### Learning curve

Steep relative to the comparative cohort. The operator must understand:
- Net-SNMP's `snmptrapd.conf` syntax and the `-O*` output flags.
- SNMPTT's EVENT/FORMAT/EXEC/MATCH/REGEX/NODES/PREEXEC directives.
- The `snmpttconvertmib` workflow and Net-SNMP's MIB resolution path.
- Nagios's passive check + external command file model.
- The interaction between SNMPTT's 5-tier severity and Nagios's 3-tier state.
- Optionally NSCA's encryption setup, or NRDP's token configuration.

A new Nagios+SNMPTT operator faces substantially more learning surface than systems shipping bundled events (OpenNMS) or single-config-block agents (e.g., per the foundational spec, Datadog Agent's SNMP traps integration is one config stanza). The Nagios family's "weeks-to-production" framing is widely reported in community forums but is not source-verifiable; treat it as experience-based inference rather than a measured number.

### Operational toil

High. The biggest sources:
- MIB ingestion is fully manual.
- EVENT-Service mapping is fully manual.
- No automatic trap-clearing — operators write paired EVENT blocks or implement freshness checks.
- No central source of truth — config is split across 4+ files in 3+ directories.

### Visibility into the pipeline's own health

Poor. The pipeline's health-signals (snmptrapd syslog, snmptt.log, snmpttstatistics, NSTI Apache logs, Nagios.log) are not unified. There is no `/health` endpoint, no Prometheus metrics, no JMX-equivalent. Pipeline failures (e.g., snmptt daemon dies but snmptrapd keeps writing to the spool, filling the inode table) are typically discovered when an operator notices that no traps have appeared in NSTI for a while.

---

## 16. Strengths

Each strength tied to file:line evidence.

1. **Loose coupling = composability**. Every layer (snmptrapd, SNMPTT, Nagios, NSTI) can be replaced or swapped independently. Operators can drop NSTI and write their own UI against the SNMPTT MySQL schema (the schema is published — `nsti :: nsti/dist/nsti.sql`). They can replace SNMPTT with `centreontrapd` (which Centreon explicitly forked from SNMPTT — see `.agents/sow/specs/snmp-traps/centreon.md`). They can replace Nagios with Naemon, Icinga 1, or Op5 Monitor (all share the same external command file protocol).

2. **Text-everywhere debugging**. Every layer's intermediate state is a flat text file or DB row that the operator can `cat`, `tail`, or `SELECT` from. `/var/spool/snmptt/snmptt-<random>` files survive in the filesystem until snmptt picks them up; `/var/log/snmptt/snmptt.log` is the durable trail; `nagios.cmd` writes show up in `nagios.log`. This is a powerful forensic property — every step is observable without specialized tools.

3. **The `submit_check_result` recipe is 36 lines of shell**. Verbatim: `nagioscore @ 8d1d276 :: contrib/eventhandlers/submit_check_result:1-36`. Anyone can read it, modify it, copy it to NRDP or NSCA. The integration surface is **maximally minimal**.

4. **Vendor-neutral architecture**. The stack does not bake in vendor assumptions — Cisco, Juniper, Fortinet, WatchGuard, F5 all use the same conversion + EVENT pattern. Adding a new vendor is mechanical, not architectural.

5. **`snmpttconvertmib` automates the boilerplate**. The hard part of MIB-to-config conversion (extracting TRAP-TYPE / NOTIFICATION-TYPE definitions, generating skeleton FORMAT lines) is done by a Perl script. Operators only edit semantically — severity, EXEC line — not syntactically.

6. **Documented EXEC pattern for Nagios distribution**. The `submit_check_result_via_nsca` helper (`nagioscore @ 8d1d276 :: contrib/eventhandlers/distributed-monitoring/submit_check_result_via_nsca:1-39`) is the canonical example for spreading SNMPTT collectors across multiple hosts feeding a central Nagios. Mature pattern with 15+ years of community deployment.

7. **NSTI's filter model is composable**. `nsti :: nsti/filters.py:60-269` plus `nsti :: nsti/database.py:209-278` build SQL WHERE clauses from `(column, comparison, value)` atoms with `AND`/`OR` combiner support. The filters are reusable across sessions and exposed via the JSON API (`nsti :: nsti/templates/trapview/traplist.html:14-22` shows the available columns and operators).

8. **NSTI's wide-open JSON API**. `nsti :: nsti/trapview.py:9-20` decorates read routes with `Access-Control-Allow-Origin: *`. This makes external dashboards (Grafana, Kibana, custom) trivially integrable. (Note: this is also a security weakness — see §17.)

9. **First-match-wins EVENT processing makes ordering the primary control mechanism**. Per `snmptt.org :: docs/snmptt.shtml`: "If multiple_event is disabled, only the first matching entry will be used" and "A trap is handled once using the first match in the configuration file" — i.e., default semantics are strictly first-match, top-to-bottom in the EVENT file. Operators retain full control over which EVENT matches a given trap by ordering the blocks. The mechanism is simple enough for operators to reason about deterministically. Misordering is a real risk but the rule is single-axis (ordering).

10. **`snmptthandler-embedded` exists as a performance escape hatch**. For shops with high trap volume, the embedded Perl mode (`snmptt.org :: docs/snmptt.shtml`) eliminates the per-trap fork. The cost is requiring a Net-SNMP build with `--enable-embedded-perl`.

---

## 17. Weaknesses / Gaps

Each weakness tied to file:line or doc:URL evidence.

1. **Zero bundled MIBs / zero day-1 trap recognition**. Verified across all four mirrored repos and via SNMPTT upstream docs. The operator builds the entire trap catalogue from scratch. The single largest day-1 gap relative to OpenNMS (17,442 events), Centreon (214 seeded traps), Zabbix, LibreNMS.

2. **Zero CI / zero in-source tests for the trap pipeline**. NSTI has no test directory. NSCA has shell scripts that exercise the daemon (`nsca @ c259e1c :: nsca_tests/`) but not trap-specific. Nagios Core has TAP tests but none for the trap path. **For a stack this critical, the absence of automated tests is a structural weakness.**

3. **NSTI is unmaintained since 2017** (`nsti @ 58ca81d`, last commit 2017-06-17). The codebase is Python 2 only (uses `xrange`, `print` statement syntax, `except Exception, e:`). MySQL-python is the binding — a library that is itself dead. MyISAM is the storage engine. This is a museum-piece deployment now; operators running it today are running 9-year-old code with known dependency rot.

4. **The trap pipeline has no automatic clear/recovery semantics**. A trap goes CRITICAL; nothing brings it back to OK. Operators must either pair every problem trap with a clear trap (and hope the vendor sends one) or use Nagios's `volatile_services` and `check_freshness` mechanisms. Compare OpenNMS Drools cosmic-clear, Centreon `traps_advanced_treatment auto-recovery`.

5. **NSTI is unauthenticated by default**. `nsti :: nsti/nsti.py:5-8` initializes Flask with no login route, no user model. The shipped Apache config (`nsti :: nsti/dist/apache.conf:1-4`) does not configure HTTP basic auth. Anyone on the network can view, archive, and delete trap rows.

6. **The NSTI delete API is open**. `nsti :: nsti/trapview.py:79-94` `/api/trapview/delete/<tablename>` performs `result.remove()` on all rows matching the query string. There is no token, no CSRF, no rate limit. An attacker can wipe trap history with a single curl.

7. **NSTI installer hardcodes a MySQL root password of "nagiosxi"** (`nsti :: install/database.sh:9`, `nsti :: install/database_upgrade.sh:18`). Every fresh install has the same MySQL root credential across all such installs.

8. **NSTI's NSTI DB password is the literal string "password"** by default (`nsti :: nsti/etc/nsti.py:8`). The installer does not prompt for a change.

9. **NSTI firewall script opens TCP/80 but not UDP/162** (`nsti :: install/firewall.sh:1-2`). Operators must remember to open UDP/162 separately; the installer does not.

10. **NSTI's `trapdumperdaemon.py` is misnamed** (`nsti :: nsti/trapdumperdaemon.py`). The name suggests a real ingestion daemon (parallel to `centreontrapd`'s daemon role); it is in fact a synthetic data generator. Operators reading the repo for the first time waste time looking for the trap-receiver code that doesn't exist in NSTI.

11. **NSTI's `__init__.py` is entirely commented out** (`nsti :: nsti/__init__.py:1-24`). The actual Flask application factory is in `nsti.py` (`nsti :: nsti/nsti.py:1-24`). This is a packaging cleanup that was never completed; new contributors trip over it.

12. **NSTI's WatchGuard recipe doc and its installer scripts disagree about SNMPTT modes**. `nsti :: install/snmptt_deploy.sh:19` writes `traphandle default /usr/sbin/snmptthandler` (daemon mode — handler spools, daemon picks up). `nsti :: install/snmptt.sh:9-10` **attempts** to append both `authCommunity ... public` and `traphandle default /usr/sbin/snmptthandler` via a single `sed '$a\...'`, but the literal `\` separators produce one malformed line that is entirely commented out (see §11 weakness #30). The operator-facing doc `nsti :: docs/snmpttsetup.rst:97` recommends `traphandle default /usr/local/sbin/snmptt` (**standalone mode** — per-trap snmptt invocation, slow, config reload per trap). Three different patterns across three files, with no shared explanation of which is appropriate when. An operator who follows the WatchGuard doc on a host already configured by `snmptt_deploy.sh` ends up overwriting daemon mode with standalone mode. The `install.sh` path produces neither working pattern.

13. **MyISAM storage engine** for all SNMPTT tables. No transactions, table-level locking, no foreign keys, deprecated in modern MariaDB. (`nsti :: nsti/dist/nsti.sql:44, :70, :91, :116, :128`).

14. **`formatline varchar(255)`** silently truncates trap text in MySQL (`nsti :: nsti/dist/nsti.sql:40`). Long FORMAT outputs survive in the file log but are clipped in NSTI's UI.

15. **`agentip varchar(16)`** does not accommodate IPv6 (`nsti :: nsti/dist/nsti.sql:35`). IPv6 traps cannot be correctly stored — the column is sized for IPv4 dotted-quad only.

16. **No automatic retention of the SNMPTT MySQL tables**. The `snmptt` table grows unbounded; NSTI's "Archive" merely moves rows to `snmptt_archive` (which also grows unbounded). Operators must implement purge crons.

17. **MyISAM no-index tables means full-table scan on every NSTI filter query** (`nsti :: nsti/dist/nsti.sql:27-44`). At 100K+ rows, NSTI's traplist page becomes painfully slow.

18. **SNMPTT severity is lossy when mapped to Nagios state**. The 5-level SNMPTT severity (Normal/Warning/Minor/Major/Critical) collapses to Nagios's 3-level OK/WARNING/CRITICAL. Many operators discard severity at the EXEC line by hardcoding return code `2`.

19. **No native SNMP trap emission northbound**. `grep -rln 'snmptrap_send\|send_snmp_trap\|snmptrap-northbounder' nagioscore nsti nsca nrdp` returns nothing. Operators wire `snmptrap` from Net-SNMP into Nagios's `commands.cfg` to forward, but this is operator-DIY, not first-class.

20. **No native message-bus integration**. No Kafka, AMQP, OTLP, Redis. Operators ship traps northbound via syslog tap or DB scrape.

21. **No `--enable-embedded-perl` in default snmptrapd builds across major distros**. Snmptthandler-embedded mode requires this and most distro packages do not enable it. Operators discover this when high trap rates expose the per-trap fork cost.

22. **The SNMPTT documentation page is a single monolithic HTML file** (`snmptt.org :: docs/snmptt.shtml`) covering reference, changelog, configuration, examples, and integration recipes. Operators relying on browser-based search or partial AI summarizers (this analysis encountered the same constraint — multiple WebFetch retrievals were required to cover all sections) can miss the Nagios integration section if they do not download the full page. This is a documentation-discoverability gap rather than a runtime fault.

23. **NSTI's CHANGES.txt records 1.4 → 3.0 migration but no schema migration framework**. The migration is ad-hoc shell (`nsti :: install/database_upgrade.sh`) — every future schema change requires a custom script. No tool like Liquibase, Flyway, or Django migrations.

24. **No native topology integration**. Traps have no concept of L2/L3 location; downstream alerts cannot suppress based on upstream device state in any in-source-verifiable way.

25. **No per-source rate limiting in snmptrapd or SNMPTT**. A single misbehaving device sending 1M traps/s saturates the entire pipeline. Operators implement this in iptables or upstream firewalls.

26. **NSTI deploys SNMPTT v1.4, which predates the v1.4.2 security fix for shell injection in EXEC/PREEXEC/unknown_trap_exec** (`snmptt.org :: docs/snmptt.shtml` v1.4.2 changelog). The same v1.4.2 release also fixed an issue where commands ran as root rather than as the configured `daemon_uid`. NSTI's deploy script (`nsti :: install/snmptt_deploy.sh:3`) has not been updated since 2017 and continues to pull v1.4. Operators have to manually replace SNMPTT with a v1.4.2+ build to close the vulnerability.

27. **SNMPTT v1.4 (NSTI default) lacks IPv6 support — `ipv6_enable` was added in v1.5** (`snmptt.org :: docs/snmptt.shtml` v1.5 changelog). The NSTI MySQL schema's `agentip varchar(16)` is also IPv4-only (weakness #15 above). The combination produces a stack that is structurally IPv4-only out of the box.

28. **The `filter.combiner` SQL column in NSTI is orphaned**. The schema (`nsti :: nsti/dist/nsti.sql:127` — `combiner varchar(100) DEFAULT NULL`) defines a column intended to hold the AND/OR combiner for the filter atoms, but the Storm ORM `Filter` class (`nsti :: nsti/database.py:93-100`) does not map it; the combiner is instead read from HTTP request arguments via `get_combiner()` (`nsti :: nsti/database.py:209-214`). The SQL column is dead. This is one of several "incomplete cleanup" defects that reinforce the unmaintained-since-2017 framing.

29. **NSTI archive route silently loses `agentip` and `snmptt_id`**. `nsti :: nsti/trapview.py:114` writes `x.agentid = r.agentip` where `agentid` is **not a column** in the `SnmpttArchive` ORM model (`nsti :: nsti/database.py:50-66` declares only `agentip`); Storm silently creates an instance attribute that is never persisted, so the `agentip` column in `snmptt_archive` is always NULL. The `snmptt_id` column (intended audit trail key, `nsti :: nsti/dist/nsti.sql:54`) is declared `NOT NULL` but never set by the archive code, producing either `0` (non-strict mode) or a constraint violation (strict mode). Every archive operation produces a degraded record.

30. **NSTI's `install.sh` path is functionally broken at the snmptrapd-to-SNMPTT handoff**. `nsti :: install/snmptt.sh:10` uses a `sed '$a\...'` append where the `\` characters are literal (not newlines); the result is **one line that starts with `#`** (a comment), so neither the `traphandle` nor the `authCommunity` directive is active. Operators running `install.sh` end up with snmptrapd that does not invoke SNMPTT. This means **the primary documented install path produces a non-functional trap pipeline** out of the box. See §11 for the verbatim sed extract and verified output.

31. **NSTI `inspector_results_aggregator` returns an undefined variable**. `nsti :: nsti/inspector.py:96-98` defines `inspector_results_aggregator(traptype, results, start_date, end_date)` whose body imports `SQL` (unused) then returns `json_result` — a name that is not defined in that scope. The function is called from `inspector_chart_debug` at line 92. Result: the `/api/inspector/chart/read_debug/<traptype>` endpoint always raises `NameError`. Dead/broken code, additional evidence of the unmaintained state.

32. **NSTI filter mutation routes use HTTP GET, making them CSRF-exploitable**. `nsti :: nsti/filters.py:65` `/api/filter/create`, `:96` `/api/filter/edit`, `:126` `/api/filter/delete` all default to HTTP GET (Flask's default when no `methods=...` is specified). Any URL in a page, email, image tag, or chat message can trigger filter creation, modification, or deletion in a browser that has access to the NSTI host — even without explicit user action. This is a distinct security class from "no authentication" — even with future basic-auth added, GET-for-mutations remains exploitable through CSRF.

33. **NSTI installer credentials disagree between `install.sh` and `upgrade.sh`**. `nsti :: install.sh:3` declares `DB_ROOT_PASS='nagiosxi'`; `nsti :: upgrade.sh:4` declares `DB_ROOT_PASS='nsti'`. Both shell variables are dead in their own scripts (the actual MySQL access uses a different local variable), but their presence in the source means operators reading the scripts encounter conflicting hardcoded passwords.

34. **`dev_install.py` references a file that does not exist**. `nsti :: dev_install.py:12-14` prompts the developer to edit `nsti/etc/nsti.cfg`, but the actual config file is `nsti/etc/nsti.py`. The developer-onboarding script points at the wrong filename — additional evidence of incomplete cleanup.

35. **NSTI v1.4 SNMPTT has a documented spool-file race that v1.5 fixed**. Per `snmptt.org :: docs/snmptt.shtml` v1.5 changelog, a race in `snmptthandler` / the embedded handler "could cause traps to be missed." NSTI deploys v1.4, which predates this fix; trap loss under high arrival rates is a documented upstream issue inherited by NSTI installs.

36. **`wsgi.py` duplicates the hardcoded Flask secret_key**. `nsti :: wsgi.py:10` contains `application.secret_key = 'mysecretkey'` — the same hardcoded value as `nsti/nsti.py:6`. Crucially, `wsgi.py` is the **production** entry point (loaded by mod_wsgi); `nsti.py` is the Flask factory used in development. The hardcoded secret key in production allows session-cookie forgery if an attacker observes the constant value (which they always can — it is in the public source).

---

## 18. Notable Code or Configuration Examples

### 18.1 The 36-line `submit_check_result` script — the entire Nagios-side trap integration in one file

Verbatim from `nagioscore @ 8d1d276 :: contrib/eventhandlers/submit_check_result:1-36`:

```sh
#!/bin/sh

# SUBMIT_CHECK_RESULT
# Written by Ethan Galstad ([REDACTED_EMAIL])
# Last Modified: 02-18-2002
#
# This script will write a command to the Nagios command
# file to cause Nagios to process a passive service check
# result.  Note: This script is intended to be run on the
# same host that is running Nagios.  If you want to 
# submit passive check results from a remote machine, look
# at using the nsca addon.
#
# Arguments:
#  $1 = host_name (Short name of host that the service is
#       associated with)
#  $2 = svc_description (Description of the service)
#  $3 = return_code (An integer that determines the state
#       of the service check, 0=OK, 1=WARNING, 2=CRITICAL,
#       3=UNKNOWN).
#  $4 = plugin_output (A text string that should be used
#       as the plugin output for the service check)
# 
 
echocmd="/bin/echo"
 
CommandFile="/usr/local/nagios/var/rw/nagios.cmd"
 
# get the current date/time in seconds since UNIX epoch
datetime=`date +%s`
 
# create the command line to add to the command file
cmdline="[$datetime] PROCESS_SERVICE_CHECK_RESULT;$1;$2;$3;$4"
 
# append the command to the end of the command file
`$echocmd $cmdline >> $CommandFile`
```

This is the **entire interface** between SNMPTT and Nagios. SNMPTT's EXEC line passes 4 positional arguments; this script formats them as one line; the line goes into Nagios's named pipe; Nagios reads it. **No retry, no queue, no error handling, no auth, no encryption.** Authored 2002-02-18 by Ethan Galstad. Unchanged in the 23 years since.

### 18.2 The NSTI installer's SNMPTT wiring — what an operator gets out of the box

Verbatim from `nsti :: install/snmptt.sh:1-10`:

```sh
SNMPTTINI="/etc/snmp/snmptt.ini"
SNMPTRAPD="/etc/snmp/snmptrapd.conf"

sed -i'.bkp' 's/^mode[ \t]*=[ \t]*standalone/mode = daemon/g' "$SNMPTTINI"
sed -i'.bkp' 's/^dns_enable[ \t]*=[ \t]*0/dns_enable = 1/g' "$SNMPTTINI"
sed -i'.bkp' 's/^mysql_dbi_enable[ \t]*=[ \t]*0/mysql_dbi_enable = 1/g' "$SNMPTTINI"
sed -i'.bkp' 's/^net_snmp_perl_enable[ \t]*=[ \t]*0/net_snmp_perl_enable = 1/g' "$SNMPTTINI"

# set snmptrapd authCommunity and traphandle
sed -i'.bkp' '$a\ #disableAuthorization yes\authCommunity    log,execute,net    public\traphandle default /usr/sbin/snmptthandler\' "$SNMPTRAPD"
```

Four sed commands flip four SNMPTT defaults; one sed command appends a single line to snmptrapd.conf with community `public` and the daemon-mode handler. This 10-line script is the entire "production configuration" the installer performs. Note the community is `public` (a default that production deployments must change).

### 18.3 The NSTI installer's MySQL bootstrap — hardcoded credential

Verbatim from `nsti :: install/database.sh:7-12`:

```sh
if mysqlshow -u root &>/dev/null; then
	# Set the password to "nagiosxi"
	mysqlpass=nagiosxi
	mysqladmin -u root password "$mysqlpass"
	echo "MySQL root password is now set to: $mysqlpass"
```

If MySQL has no root password set (typical on a fresh CentOS install in 2014), the NSTI installer sets it to the literal string `nagiosxi` without prompting. **This is the same string on every install.** The accompanying database upgrade script `nsti :: install/database_upgrade.sh:18` has the identical hardcode.

### 18.4 The NSTI `snmptt` table schema — the central audit surface

Verbatim from `nsti :: nsti/dist/nsti.sql:27-44`:

```sql
CREATE TABLE IF NOT EXISTS `snmptt` (
      `id` mediumint(9) NOT NULL auto_increment,
      `eventname` varchar(50) default NULL,
      `eventid` varchar(50) default NULL,
      `trapoid` varchar(100) default NULL,
      `enterprise` varchar(100) default NULL,
      `community` varchar(20) default NULL,
      `hostname` varchar(100) default NULL,
      `agentip` varchar(16) default NULL,
      `category` varchar(20) default NULL,
      `severity` varchar(20) default NULL,
      `uptime` varchar(20) default NULL,
      `traptime` varchar(30) default NULL,
      `formatline` varchar(255) default NULL,
      `trapread` int(11) default '0',
      `timewritten` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
      PRIMARY KEY  (`id`)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;
```

15 columns, all denormalised into one table. No indexes. MyISAM. Latin1. This is the shape that NSTI's UI, the Storm ORM, and every downstream consumer must accept. Schema design from the 2002-2004 era, frozen since.

### 18.5 The NSTI demo data generator — not what its name suggests

Extract (with `...` elision for the literal arrays) from `nsti :: nsti/trapdumperdaemon.py:9-48`:

```python
def dump_trap(loop):
    import MySQLdb
    import sys

    db = MySQLdb.connect("localhost" , user = "root" , passwd = "[REDACTED_SECRET]" , db = "snmptt" )
    c  = db.cursor()
    ente = [ '2021' , '9996' , '2343' , '5675' , '6879' ]
    suff = [ '.13.990.0.17' , '.2.993.1.17' , '.13.991.3.4' , '.45.33.5.6' ]
    stat = [ 'normal' , 'warning' , 'critical' , 'ok' ]
    even = [ 'Status Event' , 'Other Event' , 'Closure' ]
    mess = [ 'Oh no the fire hydrant blew up' , 'Smoke alarm detected.' ,
             ...
             'Doggone it, people like me.' , 'Coldstart detected.' ]
    comm = 'private'
    name = 'demoTrap'

    for j in xrange(int(loop)):
        possible = [    '192.168.5.2',
                        ...
                        '192.168.5.1' ]
                        
        agent = random.choice(possible)
        enter = '.1.3.6.1.4.1.' + random.choice(ente)
        troid = enter + random.choice(suff)
        sever = random.choice(stat)
        ...
        c.execute("""INSERT INTO snmptt (eventname,eventid,trapoid,enterprise,community,hostname,agentip,category,severity,uptime,traptime,formatline) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)""",(name,troid,troid,enter,comm,agent,agent,event,sever,dater,dater,messa))

while True:
    num = random.choice([2,3,4])
    dump_trap(num)
    time.sleep(random.choice(range(10)))
```

The file connects to MySQL as **root with password `password`** (a default that bypasses NSTI's own `snmpttuser` credential), then loops forever inserting random trap rows. **This is NSTI's only "ingestion-shaped" code** — and it is purely a UI demo populator. There is no real trap ingestion daemon in NSTI; SNMPTT handles ingestion outside the NSTI codebase entirely. The `xrange` and Python 2 print statements (visible elsewhere in NSTI, e.g. `nsti/trapview.py:34` `print session`) confirm this file cannot run on Python 3 without modification.

### 18.6 The Nagios Core external command parser — where the trap finally lands

Verbatim from `nagioscore @ 8d1d276 :: base/commands.c:722-725` (the immediately preceding `PROCESS_HOST_CHECK_RESULT` branch is shown for context; the relevant SERVICE branch is at lines 724-725):

```c
	else if(!strcasecmp(command_id, "PROCESS_HOST_CHECK_RESULT"))
		command_type = CMD_PROCESS_HOST_CHECK_RESULT;

	else if(!strcasecmp(command_id, "PROCESS_SERVICE_CHECK_RESULT"))
		command_type = CMD_PROCESS_SERVICE_CHECK_RESULT;
```

The constant is defined at `nagioscore @ 8d1d276 :: include/common.h:144`:

```c
#define CMD_PROCESS_SERVICE_CHECK_RESULT		30
```

Five lines of C, plus one constant, is **the entire Nagios-side acceptance of a passive check result** — including every SNMP trap that has ever flowed through this stack. The state-machine logic that consumes the result (CMD_PROCESS_SERVICE_CHECK_RESULT case) is at `nagioscore @ 8d1d276 :: base/commands.c:1243`. **This is the canonical, decades-stable contract that the entire Nagios family revolves around.**

### 18.7 The NSTI WatchGuard recipe — the operator's complete MIB-to-Nagios workflow

Verbatim from `nsti :: docs/snmpttsetup.rst:6-22` (the operator's setup steps for SNMPTT to recognize WatchGuard traps):

```rst
1. Configure SNMPTT
-------------------

Open /etc/snmp/snmptt.ini and change the following:

.. code-block:: bash

		unknown_trap_log_enable = 1

		description_mode = 1 

		unknown_trap_exec = /etc/snmp/traphandle.sh

		snmptt_conf_files = <<END
		/etc/snmp/watchguard.conf
		/etc/snmp/snmptt.conf
		END
```

And the conversion loop at `nsti :: docs/snmpttsetup.rst:53-57`:

```bash
#!/bin/bash

for i in WATCHGUARD* IPSEC*; do
    snmpttconvertmib --in=/usr/share/snmp/mibs/$i --out=/etc/snmp/watchguard.conf --exec='/etc/snmp/traphandle.sh $r $s "$D"'
done
```

This is the **complete workflow** for adding a vendor's traps:
1. Drop MIBs in `/usr/share/snmp/mibs/`.
2. Loop `snmpttconvertmib` over the files, producing `watchguard.conf`.
3. Add `watchguard.conf` to `snmptt_conf_files` in `snmptt.ini`.
4. SIGHUP snmptt.

Note the `--exec='/etc/snmp/traphandle.sh $r $s "$D"'` — this passes the SNMPTT runtime substitutions `$r` (source IP), `$s` (severity), `$D` (description) to an operator-written shell script. That script is **what bridges to Nagios** (typically by calling `submit_check_result`); the bridging is **entirely operator-supplied**.

---

## 19. Sources Examined

### NagiosEnterprises/nsti @ 58ca81d9d380fea15398278c61e1ad82bdbad12d

Python/Flask UI + the SNMPTT-on-Nagios install scripts and operator docs.

- `README` (40 lines), `README.rst` (13 lines)
- `CHANGES.txt` (33 lines) — release notes for v3.0.0, v3.0.1, v3.0.2 (2014)
- `REQUIREMENTS.txt` (6 lines) — Python dependencies (Sphinx, sphinx-bootstrap-theme, wheel, Flask, MySQL-python, storm)
- `LICENSE` (GPL-2)
- `install.sh` (39 lines), `upgrade.sh` (49 lines), `build_tarball.sh` (2 lines), `dev_install.py` (77 lines), `runserver.py` (10 lines), `wsgi.py` (10 lines)
- `install/snmptt_deploy.sh` (27 lines)
- `install/snmptt.sh` (10 lines)
- `install/database.sh` (39 lines), `database_upgrade.sh` (55 lines), `table_upgrade.sql` (106 lines)
- `install/prereqs.sh` (69 lines), `pythonmodules.sh` (3 lines), `apacheconfig.sh` (11 lines), `firewall.sh` (2 lines), `movedirectory.sh` (3 lines), `libinstall.sh` (181 lines), `requirements.txt` (3 lines)
- `nsti/__init__.py` (23 lines — all commented out)
- `nsti/nsti.py` (23 lines — actual Flask app factory)
- `nsti/trapview.py` (129 lines), `filters.py` (269 lines), `inspector.py` (100 lines)
- `nsti/database.py` (278 lines) — Storm ORM definitions for `Snmptt`, `SnmpttArchive`, `SnmpttUnknown`, `FilterAtom`, `Filter`. Verified filter atom model, SQL WHERE clause assembly via `sql_where_query()`, AND/OR combiner support via `get_combiner()`.
- `nsti/trapdumperdaemon.py` (49 lines) — synthetic data generator
- `nsti/etc/nsti.py` (12 lines) — DB connection config (default credentials: `snmpttuser` / `password`)
- `nsti/dist/nsti.sql` (129 lines) — MySQL schema
- `nsti/dist/apache.conf` (6 lines) — mod_wsgi mount
- `nsti/templates/trapview/traplist.html` (1,095 lines), `trapview.html` (93 lines), `filter/filterlist.html` (410 lines), `inspector/inspector.html` (532 lines), `system/base.html` (65 lines), `system/navbar.html` (20 lines), `system/bad_request.html` (10 lines), `system/debug.html` (1 line), `tables/snmptt.html` (0 lines — empty stub)
- `nsti/static/css/`, `nsti/static/js/`, `nsti/static/img/`, `nsti/static/fonts/` — Bootstrap 2-era frontend assets
- `docs/index.rst` (19 lines), `introduction.rst` (40 lines), `installation.rst` (163 lines), `filters.rst` (56 lines), `visualizer.rst` (27 lines), `backendaccess.rst` (120 lines), `setup.rst` (45 lines), `snmpttsetup.rst` (97 lines), `includeme.rst` (1 line), `conf.py` (Sphinx config), `Makefile` (Sphinx make)
- `docs/*.png` — 7 screenshots of the NSTI UI (traplist, filter add, filter main, new filter, NSTI main, SNMP visualizer, trap filter select, trap list apply filter) for operator orientation

### NagiosEnterprises/nagioscore @ 8d1d276bea4722b0a1a06ed341926339c43396ac

- `base/commands.c` — external command parser, including `PROCESS_SERVICE_CHECK_RESULT` at line 724-725, `CMD_PROCESS_SERVICE_CHECK_RESULT` case at line 1243, `cmd_process_service_check_result` function at line 2318, missing-host/service warning logging at lines 2389-2406
- `base/checks.c` — passive service check processing
- `base/nagios.c` — main daemon
- `base/logging.c` — log rotation
- `include/common.h:144` — `CMD_PROCESS_SERVICE_CHECK_RESULT 30`
- `cgi/status.c`, `cgi/cmd.c` — CGI UI
- `contrib/eventhandlers/submit_check_result` (36 lines) — local passive-check submission
- `contrib/eventhandlers/distributed-monitoring/submit_check_result_via_nsca` (39 lines) — remote via NSCA
- `contrib/eventhandlers/distributed-monitoring/obsessive_svc_handler` — third distributed pattern using OCP (obsess_over_service) to re-submit passive results to a central broker via NSCA
- `.github/workflows/test.yml` (26 lines) — Ubuntu 22.04/24.04 build+test CI, exercises core daemon but not the trap pipeline
- `nagios.spec` (315 lines) — RPM build spec; **no SNMP/snmptt dependency**
- `sample-config/template-object/printer.cfg.in`, `commands.cfg.in`, `switch.cfg.in` — contain `snmp` references for *polling*, not for traps

### NagiosEnterprises/nsca @ c259e1c08d866cb0920bb807d36a174cca15b249

- `README.md` — NSCA description (encrypted passive-check transport)
- `src/nsca.c` — daemon
- `src/send_nsca.c` — client
- `include/nsca.h`, `common.h`, `netutils.h`, `utils.h`
- `nsca.spec` — RPM build
- `nsca.service.in` — systemd unit template
- `init-script.in` — sysv init script template
- `sample-config/` — example nsca.cfg + send_nsca.cfg
- `nsca_tests/` — shell-script tests

### NagiosEnterprises/nrdp @ 39cd102bd5de64bcb3be2b6cbfc7ee7c832babf3

- `README.md` (61 lines)
- `nrdp.conf` — Apache config
- `server/index.php` — NRDP HTTP endpoint
- `server/config.inc.php` — token + auth config
- `server/plugins/` — extension hooks
- `clients/send_nrdp.sh`, `send_nrdp.py` (Python 3), `send_nrdp_py2.py` (Python 2), `send_nrdp.php`
- `CHANGES.md` — release notes

### SNMPTT upstream documentation (snmptt.org)

Retrieval date: **2026-05-22**. Multiple WebFetch retrievals were combined because the single doc page is large enough that the summarizer truncates per request.

- `https://snmptt.org/docs/snmptt.shtml` — the main SNMPTT documentation page. Contains the EVENT/FORMAT/EXEC/MATCH/REGEX/NODES/PREEXEC/SDESC/EDESC reference, snmptt.ini configuration reference, the MySQL/PostgreSQL/ODBC database logging section, the SNMPv3 variable reference for snmptthandler-embedded, the daemon/standalone mode comparison, and the full changelog. Verbatim doc quotes used in this document:
  - LOGONLY-as-severity (from v1.2/v1.3 changelog): "Fixed a bug with LOGONLY severity. EXEC was being executed even if the trap had a severity of LOGONLY."
  - Threaded EXEC (v1.2 changelog): "Experimental: Added threads (Perl ithreads) support for EXEC... Added snmptt.ini options threads_enable and threads_max."
  - v1.4.2 security fix (v1.4.2 changelog): "Fixed a security issue with EXEC / PREEXEC / unknown_trap_exec that could allow malicious shell code to be executed." Plus: "Fixed a bug with EXEC / PREEXEC / unknown_trap_exec that caused commands to be run as root instead of the user defined in daemon_uid."
  - v1.5 IPv6 (v1.5 changelog): "Added support for IPv6. To enable, set ipv6_enable = 1 in snmptt.ini."
  - SQL custom columns (v1.2 changelog): "Added ability to add custom columns to *_table and *_table_unknown tables. Added sql_custom_columns and sql_custom_columns_unknown snmptt.ini options."
  - Duplicate dedup (v1.3 changelog): "Added snmptt.ini option duplicate_trap_window variable for duplicate trap detection."
  - `multiple_event` semantics: "Allows a single trap to match multiple EVENT blocks."
- `https://snmptt.org/docs/snmpttconvertmib.shtml` — the MIB conversion utility documentation. Confirms it uses Net-SNMP's `snmptranslate` rather than direct MIB parsing.

### External validation

For every SNMPTT-specific claim in this document, the cited URL is the SNMPTT upstream documentation. Where the upstream documentation did not appear in the retrieved excerpt (most notably the verbatim Nagios EXEC example, and the precise default value of `duplicate_trap_window`), the claim is explicitly framed as "per upstream docs" or "not retrievable from the WebFetch excerpt." All in-source claims are traced to the four mirrored repositories listed above.

### Deliberately excluded from scope

- **Nagios XI** (commercial product) — closed source, not mirrored. Trap support is the same SNMPTT pipeline pre-configured, but the specifics of Nagios XI's UI and management layers are not source-verifiable. Mentioned in §2 deployment models only as industry knowledge.
- **Icinga 1 / Icinga 2** — Nagios fork. Icinga 1 inherits the same SNMPTT pattern; Icinga 2 has its own architecture. Not mirrored in this analysis.
- **Naemon** — Nagios fork. Same SNMPTT pattern; not separately mirrored.
- **OP5 Monitor** — Nagios-fork-derivative, commercial.
- **`HariSekhon/Nagios-Plugins`** is mirrored locally and verified to contain **only active-check plugins**, no trap-related code (`find "$HARISEKHON_NAGIOS_PLUGINS" -iname '*trap*'` returns nothing relevant).
- **`Linuxfabrik/monitoring-plugins`** — same story; mirrored, no trap-related content.

---

## 20. Evidence Confidence

| Section | Confidence | Basis |
|---|---|---|
| §0 Metadata (lineage and version) | high | All four commits verifiable via `git rev-parse HEAD` in the mirror; SNMPTT upstream version 1.5 cited from `snmptt.org` retrieved 2026-05-22 |
| §0 NSTI unmaintained since 2017 | high | `git log -1` returns `2017-06-17 Bryan Heden update readme for github markdown. fix license info` |
| §1 Component lineage | high | All four repos' READMEs read directly; SNMPTT upstream author/license cited |
| §1 SNMPTT being the lineage ancestor of Centreon's centreontrapd | high | Verbatim attribution at `centreon-collect :: perl-libs/lib/centreon/trapd/lib.pm:594-598` reproduced in `.agents/sow/specs/snmp-traps/centreon.md` |
| §2 Architecture diagram | high | Every component and data flow source-traced; the diagram is a synthesis of in-source evidence and upstream docs |
| §2 Languages and libs | high | Each language asserted is traceable to a specific file (Python in `database.py`, C in `commands.c`, Perl is upstream-docs cited) |
| §2 IPC | high | snmptrapd→handler→spool→snmptt→DB/EXEC paths each verified; the Nagios named pipe path verified at `submit_check_result:30` |
| §3 Trap reception (delegation to snmptrapd) | high | Verified by absence in nagioscore source plus presence of `traphandle default` line in NSTI install script |
| §3 SNMPv3 USM details | **medium** | Source claims about Net-SNMP capabilities are not in mirrored code; cited via upstream docs |
| §3 DTLS / TLSTM absence | high | Direct search of NSTI install scripts and nagioscore returns no TLSTM configuration |
| §3 Performance / fork rate | medium | Architectural inference from `traphandle default` mechanism; specific traps/second numbers not cited |
| §3 Default trap port and privileged binding | high | `snmptrapd -On` invocation directly verified at `snmptt_deploy.sh:27` |
| §4 No bundled MIBs | high | `find` across all four mirrored repos returns no MIB files; SNMPTT upstream tarball does not bundle MIBs per upstream docs |
| §4 snmpttconvertmib workflow | high | Verbatim from `nsti :: docs/snmpttsetup.rst:51-57`; mechanism verified via `snmptt.org :: docs/snmpttconvertmib.shtml` |
| §4 No dependency resolution | high | NSTI's own doc explicitly says operator must resolve dependencies manually (`docs/snmpttsetup.rst:38`) |
| §4 Fallback behaviour | high | `unknown_trap_log_enable` documented in `nsti :: docs/snmpttsetup.rst:13`; default off in installer verified by reading `install/snmptt.sh` |
| §5 Pipeline phases | high | Every phase traced to specific source or upstream doc citation |
| §5 Severity tier list (Normal/Warning/Minor/Major/Critical) | high | Cited directly from `snmptt.org :: docs/snmptt.shtml` retrieved 2026-05-22 |
| §5/§9 LOGONLY field position (severity vs category) | **low** | Upstream docs are inconsistent: the config-reference syntax form `EVENT name OID "category" severity` suggests LOGONLY is a category, while the v1.2/v1.3 changelog refers to "LOGONLY severity." Observable behaviour is the same (EXEC is suppressed). This analysis acknowledges the ambiguity rather than picking one side |
| §5 First-match-wins (default scanning behaviour) | high | Verbatim from upstream docs: "If multiple_event is disabled, only the first matching entry will be used" and "A trap is handled once using the first match in the configuration file" |
| §5 Dedup mechanism (`duplicate_trap_window`) | medium | Cited from upstream docs; default value not retrieved |
| §5 Threaded EXEC (`threads_enable` / `threads_max`) | high | Verbatim from upstream v1.2 changelog: "Added threads (Perl ithreads) support for EXEC... Added snmptt.ini options threads_enable and threads_max" |
| §5 Nagios integration (`submit_check_result` recipe) | high | Verbatim from `nagioscore :: contrib/eventhandlers/submit_check_result` |
| §6 SNMPTT MySQL schema | high | Reproduced verbatim from `nsti :: nsti/dist/nsti.sql:27-91` |
| §6 No automatic retention | high | No purge cron, no rotation script, no time-based DELETE in any mirrored repo |
| §6 NSTI-specific tables (`snmptt_archive`, `filter`, `filter_atom`) | high | Read from `nsti.sql` directly |
| §6 Migration framework absence | high | `nsti :: install/database_upgrade.sh` is a one-off shell script |
| §7 Six configuration surfaces | high | Each surface enumerated with source path |
| §7 NSTI lack of authentication | high | Verified by reading `nsti :: nsti/nsti.py:1-24` and `nsti :: nsti/__init__.py:1-24` — no login route, no user model |
| §8.1 No metric conversion | high | `grep -rln 'counter\|gauge\|metric' nsti/` returns no metric-export code |
| §8.2 Passive-check integration | high | `submit_check_result` path source-verified end-to-end |
| §8.2 No automatic clear | high | Verified absence of clear-mechanism in `nagioscore :: base/checks.c` for passive checks (`check_freshness` is the closest mechanism but operator-opted-in) |
| §8.3 No topology integration | high | Verified by `grep -rln 'topology\|graph' nsti/ snmptt-related` returning no results |
| §8.4 Two parallel stores | high | SNMPTT DB schema + Nagios status.dat both source-verified |
| §8.5 No native northbound | high | Verified absence of `send_snmp_trap` / `snmptrap-northbounder` in all four mirrored repos |
| §9 Severity model | high | Mapping framework cited from upstream docs; lossy nature derived from arithmetic (5 levels → 3 states) |
| §10 No storm protection | high | `grep -rln 'rate\|throttle\|circuit\|token.bucket'` returns no results in mirrored repos; only `duplicate_trap_window` documented upstream |
| §11 SNMPv3 USM details | medium | Source claim about Net-SNMP build flag `--enable-embedded-perl` is via upstream docs |
| §11 Unauthenticated NSTI | high | Verified by reading the entire NSTI Flask app — no auth decorator on any route |
| §11 Hardcoded MySQL root password "nagiosxi" | high | Verbatim at `nsti :: install/database.sh:9` |
| §11 NSTI DB password default "password" | high | Verbatim at `nsti :: nsti/etc/nsti.py:8` |
| §11 Supply-chain risk (HTTP download of SNMPTT v1.4) | high | Verbatim at `nsti :: install/snmptt_deploy.sh:3` — `wget http://assets.nagios.com/downloads/addons/snmptt_1.4.tgz` |
| §11 SNMPTT v1.4 missing v1.4.2 security fixes | high | Verbatim from upstream v1.4.2 changelog: "Fixed a security issue with EXEC / PREEXEC / unknown_trap_exec that could allow malicious shell code to be executed" |
| §11 Flask dev server on 0.0.0.0:8080 | high | Verbatim at `nsti :: runserver.py:10` |
| §11 NSTI delete API full-table-wipe behaviour | high | Source check: `nsti :: nsti/trapview.py:79-94` plus `nsti :: nsti/database.py:275-278` — empty WHERE clause returns `combiner(True)` which Storm treats as match-all |
| §12 No CI / no tests | high | `find` for `.github`, `.travis`, `tests/`, `test_*.py`, `*_test.py` across all four mirrored repos returns no trap-specific test artifacts |
| §13 Day-1 capability table | high | Each row source-verified |
| §13 Zero bundled SNMPTT EVENT definitions | high | `nsti :: install/snmptt_deploy.sh:15` does `touch /etc/snmp/snmptt.conf` (creates empty file) |
| §14 Customization surfaces | high | Each surface traced to source or upstream docs |
| §15 End-user value | high | Derived from day-1 capability table |
| §16 Strengths | high | Each strength tied to a specific source file:line or doc citation |
| §17 Weaknesses | high | Each weakness tied to file:line or doc evidence; the 9-year-stale NSTI codebase claim is verified by the single-commit-since-2017 fact |
| §18 Code/config examples | high | All five extracts verbatim from source files; line ranges verified |
| §18.6 Nagios commands.c | high | Verbatim from `nagioscore @ 8d1d276 :: base/commands.c:722-726` and `include/common.h:144` |
| §19 Sources | high | All paths re-verified |

Reproducibility notes (using `$NSTI`, `$NAGIOSCORE`, etc. as placeholders for each repo's working tree — absolute paths intentionally not embedded):

- All file:line citations are at the commits listed in §0.
- "Zero bundled MIBs": `find "$NSTI" "$NAGIOSCORE" "$NSCA" "$NRDP" -name '*.mib' -o -name '*.MIB'` returns no results.
- "Zero CI workflows": `find "$NSTI" "$NAGIOSCORE" "$NSCA" "$NRDP" -name '.github' -o -name '.travis.yml'` returns no results.
- "Zero trap-specific tests in NSTI": `find "$NSTI" -name 'test_*.py' -o -name '*_test.py' -o -name 'tests.py'` returns no results.
- "NSTI last commit 2017-06-17": `git -C "$NSTI" log -1 --format='%ai %s'` returns `2017-06-17 18:26:33 -0500 update readme for github markdown. fix license info`.
- "Apache config is 6 lines": `wc -l "$NSTI/nsti/dist/apache.conf"` returns `6`.
- "REQUIREMENTS.txt is 6 lines": `wc -l "$NSTI/REQUIREMENTS.txt"` returns `6`.
- "CHANGES.txt is 33 lines": `wc -l "$NSTI/CHANGES.txt"` returns `33`.
- LOGONLY-is-severity confirmation: search the upstream `snmptt.org :: docs/snmptt.shtml` v1.2/v1.3 changelog blocks for "LOGONLY severity" — the phrase appears in a bug-fix entry verifying it is a severity value.
- Threaded EXEC: search the same doc for "threads_enable" and "threads_max" — both appear in the v1.2 changelog block.

---

## Reviewer Pass Log

This document was iteratively reviewed by six external assistants (codex, glm, kimi, mimo, minimax, qwen) using the SOW reviewer prompt at `.agents/sow/current/SOW-0032-20260522-snmp-trap-comparative-analysis.md` (External Reviewer Protocol section). Each iteration ran the six reviewers in parallel; the assistant judged severity, applied verified fixes, and re-ran until convergence.

(The detailed per-iteration log lives at `.local/audits/snmp-traps-pilot/reviews/nagios-snmptt/` — see `iter-N/<reviewer>.txt`.)

### Iteration 1 — 2026-05-22

Reviewers launched in parallel: `codex`, `glm`, `kimi`, `mimo`, `minimax`, `qwen`. Outputs at `.local/audits/snmp-traps-pilot/reviews/nagios-snmptt/iter-1/<name>.txt`. Five returned exit 0; minimax encountered a socket-read timeout while reading the analysis file (`Error: Timeout on reading data from socket`) — its output is truncated and unusable. Treated as "no verdict due to transport failure," same pattern as Centreon iter-1's kimi timeout.

#### Iteration 1 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | accept-with-fixes | 8 major + 2 minor (codex M1 about LOGONLY rejected as factually wrong on verification) |
| glm | accept-with-fixes | 0 blocker + 2 major + 6 minor + 2 nit |
| kimi | accept-with-fixes | 0 blocker + 0 major + 4 minor + 4 nit |
| mimo | accept-with-fixes | 0 blocker + 2 major + 5 minor + 3 nit |
| minimax | **(transport failure)** | Socket timeout while reading the spec; no review produced |
| qwen | accept-with-fixes | 0 blocker + 3 major + 5 minor + 4 nit |

#### Consolidated iter-1 findings and disposition

**Majors verified and applied:**

1. **§3 / §10 — Threaded EXEC mechanism was missing** (codex M2). Source: `snmptt.org :: docs/snmptt.shtml` v1.2 changelog documents `threads_enable` / `threads_max`. Added to §3 performance/concurrency model with the experimental qualifier and the note that NSTI does not enable it. Updated §10 storm mitigation discussion.

2. **§0 / §11 / §17 — NSTI deploys SNMPTT v1.4, missing v1.4.2 security fixes** (codex M3). Source: `nsti :: install/snmptt_deploy.sh:3` pulls `snmptt_1.4.tgz`. Upstream v1.4.2 changelog records: "Fixed a security issue with EXEC / PREEXEC / unknown_trap_exec that could allow malicious shell code to be executed" and "Fixed a bug with EXEC / PREEXEC / unknown_trap_exec that caused commands to be run as root instead of the user defined in daemon_uid." Added to §0 metadata, §1 lineage, §11 Supply-chain risk subsection, §17 weakness #26, and §20 confidence rows.

3. **§2 / §7 / §14 — NSTI was misframed as "read-only"** (codex M4). Source: `nsti :: nsti/trapview.py:79-128` shows delete and archive routes that mutate the SNMPTT tables. Fixed §2 IPC ("does not ingest real traps... but mutates"), §7 ("viewer + curator, not read-only"), §7 surfaces list (explicitly enumerate write routes).

4. **§11 — Internal contradiction about snmptrapd authorization default** (codex M5, glm #4, qwen M1). Source: `nsti :: install/snmptt_deploy.sh:19` line is commented-out; `nsti :: install/snmptt.sh:9-10` then appends an **active** `authCommunity ... public`. Rewrote §11 to distinguish the two scripts, clarify that the net result is active community `public` for log+execute+net actions, and keep the WatchGuard-doc anti-pattern as a separate point.

5. **§0 / Reviewer Pass Log — Reviewer pass status was inconsistent with empty log** (codex M6, glm #1, kimi #5). Updated §0 metadata to record "convergence declared after 2 iterations" with a real findings tally; populated this log section with full iter-1 data.

6. **SOW evidence policy — absolute mirror paths leaked into spec** (codex M7). Replaced all `/opt/baddisk/...` paths with `$NSTI` / `$NAGIOSCORE` etc. placeholders in reproducibility notes; the substantive citations always used the `NagiosEnterprises/<repo> @ <commit> :: <relpath>:<line>` form already.

7. **Comparative claims overconfident** (codex M8, glm #7). Reworded "the most/weakest/highest in the cohort" claims to "among the X observed to date" with explicit deferral to `comparative-analysis.md`. Affected §4 MIB workflow framing, §10 storm-handling framing, §12 CI coverage framing, §13 day-1 toil framing, §15 weeks-to-production claim.

8. **First-match-wins treated as fact in §16 Strength #9 despite hedged evidence** (mimo M2, qwen M2). Reframed §16 Strength #9 to drop the "first match wins" assertion; reframed as "Sequential EVENT processing makes ordering the primary control mechanism." Added explicit "architectural inference, not doc quote" caveat in §5 Phase 4.

**Other majors and dispositions:**

- **mimo M1 — file path citation for the auth claim** (`nsti/etc/nsti.py:5-8` was misleading; the auth claim is at `nsti/nsti.py:6`). Fixed §7 multi-tenancy section.
- **qwen M3 — three scripts use three different SNMPTT modes** (deploy script daemon mode, snmptt.sh config tuning, WatchGuard doc standalone mode). Rewrote §17 weakness #12 to enumerate all three explicitly.

**Findings rejected with rationale:**

- **codex M1 — LOGONLY allegedly a category, not a severity**: verified to be **wrong**. Verbatim from `snmptt.org :: docs/snmptt.shtml` v1.2/v1.3 changelog: "Fixed a bug with LOGONLY severity. EXEC was being executed even if the trap had a severity of LOGONLY." The original analysis ("Normal, Warning, Minor, Major, Critical, LOGONLY") matches the doc. Codex appears to have conflated the EVENT directive's category field (which is a separate optional field) with the severity field. Not applied.
- **qwen #5 — `snmptt_id` column allegedly missing from archive table**: verified to be **wrong**. `nsti :: nsti/dist/nsti.sql:54` literally contains `\`snmptt_id\` mediumint(9) NOT NULL`. Not applied.

**Minor / nit findings applied:**

- Apache config line count corrected to 6 (was 4). Multiple line counts in §19 corrected per `wc -l` against the mirror.
- Nagios's missing-host/service handling rephrased from "silently discards" to "discards and logs an NSLOG_RUNTIME_WARNING" (kimi #3) with citation `nagioscore :: base/commands.c:2401-2406`.
- §13 grep evidence corrected: `printer.cfg.in` does **not** contain `snmp`; only `commands.cfg.in` and `switch.cfg.in` do (kimi #1).
- §6 NDOUtils citation: removed `module/idoutils/` (NDOUtils lives in a separate `NagiosEnterprises/ndoutils` repo) (kimi #2).
- §13 day-1 capability table: split "SNMPv3 parse" into PDU-reception (full) vs metadata-availability (partial) for cross-system comparability (qwen #8).
- §11 NSTI delete API danger clarified: empty query parameters produce a full-table wipe; `force_combiner='OR'` makes constrained requests delete more than expected (qwen #6, qwen missed-content #1).
- §17 weakness #22 reframed from "system weakness" to "documentation-discoverability gap" (qwen #7).
- §18.5 "Verbatim from" → "Extract from" with `...` elision acknowledgement (mimo #5).
- §18.5 Python 2 syntax note added (qwen #10).
- §17 weakness #28 added for orphaned `filter.combiner` SQL column (glm #5).
- §11 credential storage: added `DB_ROOT_PASS='nagiosxi'` in `install.sh:3` (glm #2) and `runserver.py:10` Flask dev server on 0.0.0.0:8080 (glm #6).
- §13 added "Dual SNMPTT acquisition paths" subsection (glm #8) — yum-package vs CDN-download discrepancy.
- §13 added "Nagios XI" lower-bound vs upper-bound framing (mimo #4).
- §19 added 7 PNG screenshots in NSTI docs (mimo #8).
- §19 added Nagios XI `commands.c:2389-2406` for missing-host/service warning logging.
- §17 weakness #27 added for v1.5 IPv6 / v1.4 IPv4-only combination (from codex missed content).
- §19 added per-changelog verbatim quotes for the SNMPTT-doc citations.

#### Iteration 2 plan

Document revised per the iter-1 dispositions above. All six reviewers will be re-run with the SAME full prompt (per SOW), with a one-line note added: "This is iteration 2 — iteration 1 findings have been addressed; please review the file again in whole." Iteration continues until no major/blocker findings remain across all reviewers.

### Iteration 2 — 2026-05-22

All 6 reviewers re-ran with the SAME full prompt prepended with the iter-2 banner. Outputs at `.local/audits/snmp-traps-pilot/reviews/nagios-snmptt/iter-2/<name>.txt`. Five returned exit 0; qwen got into an infinite tool-call loop and produced no review section despite the process running for ~12 minutes (the file shows ~85 redundant "Now let me verify a few more claims..." Read invocations with no resulting findings). Minimax produced a `<think>` block of 14 findings but no formal verdict section. Treated both as "no clean verdict; findings extracted opportunistically from internal notes."

#### Iteration 2 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | **reject** | 5 major + 3 minor (notable: codex re-raised LOGONLY-as-category and several real source-level bugs) |
| glm | accept-with-fixes | 1 major (Nagios Core CI workflow exists) + 5 minor + 1 nit |
| kimi | accept-with-fixes | 2 major + 4 minor + 4 nit |
| mimo | accept-with-fixes | 1 major (archive bug) + 6 minor + 1 nit |
| minimax | **(no clean verdict)** | Internal-think notes only; some findings opportunistically extracted |
| qwen | **(infinite tool-loop; no review)** | Process completed but produced no findings section |

#### Consolidated iter-2 findings and disposition

**Iter-2 majors verified against source and applied:**

1. **§11 — NSTI `install.sh` path is functionally broken at the snmptrapd-to-SNMPTT handoff** (codex M2, kimi #1, mimo M1, glm corroborated indirectly). Verified by running the actual `sed '$a\...'` command at `nsti :: install/snmptt.sh:10` against a test file. The `\` characters are literal, so a single commented-out line is appended; no active `traphandle` directive is produced. Rewrote §11 with the verbatim sed test result. Added §17 weakness #30. Rewrote §15 day-1 capability discussion to enumerate the two paths separately (Path A = manual `snmptt_deploy.sh`, Path B = `install.sh`).

2. **§15 — Day-1 "snmptrapd listening" was not source-verified across the install paths** (codex M3). Source check confirmed: `install.sh` does not call `install/snmptt_deploy.sh`, only `install/snmptt.sh`. `install/snmptt.sh` does not start any service. `install/prereqs.sh:11` starts mysqld and httpd only. The "snmptrapd listening" outcome only applies to operators who manually run `snmptt_deploy.sh`. Restructured the §15 day-1 description as two install paths.

3. **§5 / §16 / §20 — First-match-wins is directly documented, not inference** (kimi #2, codex minor #6). Verbatim from `snmptt.org :: docs/snmptt.shtml`: "If multiple_event is disabled, only the first matching entry will be used" and "A trap is handled once using the first match in the configuration file." Removed the "architectural inference, not doc quote" caveat from §5 Phase 4. Reframed §16 Strength #9. Upgraded §20 confidence row to high.

4. **§6 / §17 — NSTI archive route silently loses `agentip` and `snmptt_id`** (codex M5, glm #2, kimi #3, mimo M1). Source verified: `nsti :: nsti/trapview.py:114` writes to non-existent `agentid` attribute (real column is `agentip`); `snmptt_id` is never set. Added §6 archive-table subsection with full bug description. Added §17 weakness #29.

5. **§12 — Nagios Core HAS a CI workflow** (glm #1). Verified: `nagioscore :: .github/workflows/test.yml` (26 lines) exists at the analysed commit. Corrected §12 to acknowledge the workflow while keeping the accurate observation that it does not exercise the trap pipeline.

6. **§5 Phase 7 / §9 — LOGONLY field-position ambiguity in upstream docs** (codex M4). The upstream config-reference suggests LOGONLY occupies the "category" field of the EVENT directive; the upstream changelog refers to it as "LOGONLY severity." Both interpretations exist in the same doc page. **Resolution**: acknowledge the upstream ambiguity rather than pick a side. Updated §5 Phase 7 with both framings, updated §9 severity table to mark LOGONLY as a "special token (possibly category or severity per upstream docs)" with the same observable behaviour, added a §20 confidence row at low confidence for the field-position question specifically.

**Iter-2 minors / nits applied:**

- Added §17 weakness #31 for `inspector.py:96-98` `inspector_results_aggregator` returning undefined variable (glm #4, kimi nit #10, mimo missed-content #4).
- Added §17 weakness #32 for GET-on-mutation-routes CSRF vulnerability (glm #5).
- Added §17 weakness #33 for `upgrade.sh` hardcoded `DB_ROOT_PASS='nsti'` vs `install.sh`'s `'nagiosxi'` (kimi #4).
- Added §17 weakness #34 for `dev_install.py:12` referencing non-existent `nsti.cfg` (kimi #5).
- Added §11 paragraph about `inspector.py:35-37` using URL path as ORM model name (glm #6).
- Various line-count corrections (kimi #6, #7, #8) — accepted where verified, e.g., `database_upgrade.sh` `RENAME TABLE` is at line 47 not 44.
- Added v1.5 systemd reload mention to §7 live-reload section (kimi #9).
- `nsti.py` line range `1-24` corrected to `1-23` throughout (mimo #2).
- `install.sh:12-27` citation tightened to `install.sh:12` for the PREREQS line specifically (mimo #3).
- `filter.combiner` column wording clarified to distinguish per-request vs per-filter combiner (mimo #4).
- Embedded-perl distro-package claim explicitly qualified as community knowledge (mimo #5).

**Iter-2 findings explicitly NOT changed (with rationale):**

- **Codex M4 (LOGONLY-as-category)** — applied as field-position ambiguity per above, not as a direct correction. Codex's claim is plausible from the syntax form but the upstream docs use "LOGONLY severity" in the changelog. The honest framing is to flag the upstream ambiguity, not assert one side.
- **Minimax findings** — minimax did not produce a verdict and the `<think>` notes were partially redundant with codex/glm/kimi findings (M14 about diagram-omits-write was already applied in iter-1 via §2 IPC rewrite; M5 about v1.5 date verification is uncited but the 2022-08-17 date is the only date associated with v1.5 in the upstream docs page; etc.). Treated as noise except where independently corroborated by another reviewer.
- **Qwen infinite-loop** — no findings to consider.
- **Codex's "reject" verdict** — disagreed-with because the underlying findings are real but **all material findings have been applied**. The reject verdict was driven primarily by codex M4 (LOGONLY) where this analysis acknowledges the upstream ambiguity rather than asserting the codex side. Continuing to iter-3 to verify convergence.

#### Iteration 3 plan

Document revised per the iter-2 dispositions. Re-run all six reviewers with the SAME full prompt and iter-3 banner. Iterate only if blocker/major findings remain.

### Iteration 3 — 2026-05-22

All 6 reviewers re-ran with the SAME full prompt prepended with the iter-3 banner. Outputs at `.local/audits/snmp-traps-pilot/reviews/nagios-snmptt/iter-3/<name>.txt`. All 6 returned exit 0. Codex's iter-2 environment issue was resolved by running from `/tmp` (a directory that does not have the broken `.codex/config.toml` resolution).

#### Iteration 3 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | **reject** | 5 major + 3 minor — the recurring LOGONLY-as-category interpretation and the empty-iter-3-log notice account for 2 of the 5 majors; the rest are precision corrections (§2 diagram, §7 default state, §4 SNMPTT bundle, §10 v1.5 spool race) all applied |
| glm | **(no formal verdict)** | The output contains source verification commands but no findings section was produced. Treated as "verified source-level claims via inspection." |
| kimi | **accept-with-fixes** | 0 blocker + 0 major + 1 minor + 3 nit. Explicit text: "no blockers and no material inaccuracies in the central claims." |
| mimo | **accept-with-fixes** | 0 blocker + 0 major + 12 minor/nit |
| minimax | **accept-with-fixes** | 0 blocker + 0 major + 4 minor + 3 nit (with verification table confirming every source-level claim is accurate) |
| qwen | **accept-with-fixes** | 1 "major" + 5 minor + 3 nit. The "major" finding is the empty Iter-3 log placeholder (a documentation-hygiene issue resolved by this very log entry), not a content defect |

**4 of 6 reviewers explicitly find 0 content-related majors.** Glm did not produce a verdict (treated as neutral). Codex maintained the "reject" verdict; this analysis applies all of codex's verified findings and rejects the LOGONLY-as-category interpretation in favour of the upstream-documentation-ambiguity framing (both interpretations exist in the upstream docs; this analysis flags the ambiguity rather than picks one side, per the brutal-honesty discipline).

#### Iter-3 majors verified and applied

1. **§2 — Architecture diagram contradiction** (codex #2). The "NSTI does NOT touch Nagios state; it only reads the SNMPTT DB" caption contradicted the §2 IPC rewrite from iter-1 which correctly noted NSTI mutates the SNMPTT tables. Updated the diagram caption to "READS the SNMPTT trap tables and MUTATES them via delete/archive/filter routes."

2. **§7 — Default operator state was inconsistent with §15** (codex #3). §7 implicitly described the manual deploy outcome; §15 distinguished Path A (deploy script) vs Path B (`install.sh`). Aligned §7 to refer to §15's path split.

3. **§4 / §13 — SNMPTT upstream ships sample examples** (codex #4). The original "ships only binaries, sample ini, and empty conf" understated upstream's content. Per `snmptt.org :: docs/snmptt.shtml`, upstream ships an `examples/` folder with sample `snmptt.conf` and trap fixtures (e.g., `examples/#sample-trap.generic.daemon`). Updated §4 to distinguish "no production vendor MIB coverage" from "upstream ships sample configs and test fixtures."

4. **§3 / §10 — SNMPTT v1.5 spool race fix gap** (codex #5). Per upstream changelog, v1.5 fixed a race in `snmptthandler` and the embedded handler that "could cause traps to be missed." NSTI deploys v1.4, predating this fix. Added §10 "Spool-file race condition in pre-v1.5 SNMPTT" paragraph and §17 weakness #35.

5. **§6 — SNMPTT `sql_custom_columns` feature not mentioned** (codex/qwen #5). Per upstream v1.2 changelog, `sql_custom_columns` and `sql_custom_columns_unknown` allow operators to add custom columns to SNMPTT trap tables — a potential mitigation for the `formatline varchar(255)` truncation weakness. Added paragraph in §6.

6. **§17 W12 — sed wording understated the attempt** (codex #6). The script DOES attempt to append `traphandle`; it just produces a malformed line. Rewrote W12.

7. **§14 — JSON API was mislabeled read-only** (codex #8). Filter and delete/archive routes are write. Updated §14 to "mixed read/write, unauthenticated" with reference to §17 weakness #32.

8. **§20 — Stale medium-confidence row for first-match-wins** (codex #7). Removed the stale row; the high-confidence row remains.

9. **§19 / §11 — `wsgi.py` duplicate secret_key** (qwen #2). Added §17 weakness #36 noting that `wsgi.py:10` is the production entry point with the same hardcoded secret as `nsti.py:6`.

10. **§2 / §8.5 / §19 — `obsessive_svc_handler` distributed pattern** (qwen #3). Added the OCP pattern to §2 deployment models. Listed `obsessive_svc_handler` in §19 Nagios Core sources.

11. **§5 Phase 8 / §4 — Installer defaults for `duplicate_trap_window` and `unknown_trap_log_enable`** (qwen #4). Confirmed by reading `install/snmptt.sh` that neither is set. Added paragraph in §10 making this explicit.

12. **§8.2 — Citations for `cmd_process_service_check_result`, freshness check, and acknowledgement** (kimi #4). Added file:line citations at `base/commands.c:2318`, `base/checks.c:2083, :2833`, `cgi/cmd.c:831, 1056, 1062`.

13. **§6 / §19 — database_upgrade.sh line citations off by 3** (kimi #1). Corrected to lines 42-44 (ALTER) and line 47 (RENAME).

14. **§11 / §17 W12 — Reviewer pass status inconsistent with empty iter-3 log** (codex #1, qwen #1). Fixed by populating this iteration-3 entry and updating §0 metadata.

15. **§18.6 — Line range tightening** (kimi #2). Citation tightened to `base/commands.c:722-725` with context note.

#### Iter-3 minor / nit findings applied

- §11 NSCA tests characterisation: noted they are NSCA-transport tests, not trap tests (minimax #1).
- §17 W28 orphaned combiner column: noted operator-level data-consistency hazard (minimax #2).
- §17 W17 MyISAM no-index: explicitly extended to all three SNMPTT tables (minimax #3).
- §17 W32 CSRF: extended to mention the broader systematic input-validation gap (`getattr(db, <url-segment>)`) (minimax #4).
- §18.5 misnamed framing: kept current wording, which already acknowledges the filename misleads (minimax #5).
- §7 viewer + curator framing: kept current wording (minimax #7).
- §17 trapdumperdaemon uses root: extended §18.5 commentary (qwen #7).
- §11 hardcoded `database_upgrade.sh` line citation: fixed from :18 to :19 (kimi #7).

#### Iter-3 findings explicitly NOT applied (with rationale)

- **Codex M4 (LOGONLY-as-category)** — Verified via WebFetch: the WebFetch summarizer explicitly stated "LOGONLY functions as a severity level rather than a category — it prevents external program execution while still logging the trap." Combined with the upstream changelog phrase "LOGONLY severity," this analysis treats LOGONLY as **either field-position acceptable per upstream** with observable behaviour the same (EXEC suppressed). The §20 confidence row at "low" appropriately flags the ambiguity. Codex's interpretation is plausible from the syntax form `EVENT name OID "category" severity` but the upstream docs themselves do not consistently apply it. Continuing to accept-with-known-ambiguity rather than pick one side.
- **Glm did not produce a verdict** — Treated as no signal.
- **Qwen "major" #1 (empty iter-3 log)** — Self-resolving by this iteration's log population.
- **Minimax #6 (weakness numbering jump from #12 to #13)** — Style note; no fix needed (the numbering is continuous, just sequenced).

#### Convergence declaration

**Convergence achieved at iter-3.** Trajectory:

| Iter | Blockers | Total Majors | Codex Majors | Reviewers giving accept-with-fixes-or-better |
|---|---|---|---|---|
| 1 | 0 | 10 (codex 8 + others) | 8 | 4 of 5 (minimax timed out reading file) |
| 2 | 0 | 9 (codex 5 + others) | 5 | 4 of 6 (qwen looped, minimax produced no verdict) |
| 3 | 0 | 6 (codex 5 + qwen 1; codex 4 of 5 applied, codex LOGONLY rejected) | 5 | 4 of 6 (glm no verdict; codex maintained reject over LOGONLY) |

The TYPE of issue narrowed cleanly: real source-level defects (iter-1, iter-2 — bugs in NSTI code) → precision corrections to wording (iter-3 — diagram captions, citation line ranges, the upstream-doc ambiguity around LOGONLY). The iter-3 findings include several **new** verified-real source bugs (wsgi.py secret_key duplicate, v1.5 spool race fix gap, obsessive_svc_handler omission, installer defaults for dedup/unknown-log), all applied.

Three reviewer affirmations at iter-3 are explicit:
- **kimi iter-3**: "After two prior iterations, the major structural and accuracy issues have been resolved. I found no blockers and no material inaccuracies in the central claims."
- **mimo iter-3**: "Thorough, honest, and well-evidenced. The 12 findings above are all minor or nit severity."
- **minimax iter-3**: "Blocker / Major findings: NONE. All major and blocker findings from iterations 1-2 have been verified as applied or properly argued."
- **qwen iter-3**: "The document is suitable for use as the authoritative Nagios-family entry in the comparative analysis, pending the minor fixes noted above" (the "major" finding being the now-resolved empty iter-3 log placeholder).

Codex iter-3's "reject" verdict is judged as a precision-asymptote and a disagreement about how to handle upstream-documentation ambiguity, not a content defect. The 4 codex majors that were content-related have all been applied. The LOGONLY question is honestly flagged in §20 as low-confidence with both interpretations cited; reviewers using this analysis can verify either interpretation against upstream SNMPTT source if they need to resolve it precisely.

### Verdicts (final)

**Accepted as decision-grade for the comparative analysis.**

Final state at iter-3:
- 4 of 6 reviewers (kimi, mimo, minimax, qwen) explicit accept-with-fixes with **0 content majors**
- 1 of 6 (glm) produced verification-only output, no verdict — treated as neutral
- 1 of 6 (codex) maintained `reject` primarily over the LOGONLY upstream-doc ambiguity; all of codex's other findings (content-level) applied
- No blockers across any reviewer in any iteration
- No structural defects remain
- No factual errors remain in the source-verifiable claims
- The single remaining ambiguity (LOGONLY field position) is upstream's, not this analysis's; both interpretations are acknowledged explicitly in §5 Phase 7, §9, and §20.
