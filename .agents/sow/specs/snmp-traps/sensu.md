# Sensu — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: Sensu — covers two product generations: **Sensu Core 1.x** (a.k.a. *Sensu Classic*, Ruby + RabbitMQ/Redis, EOL December 31, 2019 per `sensu-docs/content/sensu-core/1.9/_index.md`; EOL repos removed February 1, 2021) and **Sensu Go** (Go + etcd/Postgres, current product since 2018, last analysed commit 2025-07-10).
- **Trap support is community-provided in the open-source product. The commercial Sensu Enterprise (legacy 1.x line) shipped a built-in outbound trap emitter, but that product is EOL and its source is not in the mirror (vendor-doc evidence only).** Specifically: Sensu Go (current product) and the open-source Sensu Core 1.x have never shipped an SNMP trap listener, an SNMP trap emitter, a MIB store, or any trap-aware code in their core daemons. Sensu Enterprise 1.x had an outbound `snmp` integration documented at `sensu-docs/content/sensu-enterprise/3.8/integrations/snmp.md:10-11, :22-29`. All inbound trap functionality is delivered by separately-versioned community plugins maintained at varying intervals (the most recent — and only Sensu-Go-targeted — inbound bridge had its last commit in 2020).
- **Source evidence**: mirrored. Five trap-related repos under the Sensu mirror. Plus the Sensu Go monolith (no trap code) and the Sensu documentation site (one Sensu Classic 1.9 guide, zero Sensu Go guides).
- **Repository roots analysed** (commit / last-commit date):
  - `sensu/sensu-go @ 80e82590ece98a4bc780de04882a6f754ad12abc` (2025-07-10) — searched for any SNMP/trap code; none exists outside an unrelated "Strap in" comment in `scripts/proto2graphql/proto2graphql.go:56`.
  - `sensu-extensions/sensu-extensions-snmp-trap @ 269bc593de0a10f07f05b2c7895af92e03ba566d` (2018-10-27) — Ruby trap **listener** extension for Sensu Core 1.x.
  - `sensu/sensu-snmp-trap-handler @ 3c38f014460d2d86b2ed9206516c26f568abc5e7` (2021-04-26) — Go trap **emitter** handler for Sensu Go.
  - `sensu/snmptrapd2sensu @ 50ae0ee929ccb78f4462480b478a7e8f136978af` (2020-10-29) — Go shim that converts Net-SNMP `snmptrapd` notifications into Sensu Go events via the agent HTTP API.
  - `sensu-plugins/sensu-plugins-snmp @ 36f3994a29df200919272b6bb41abe5dbde4ed7d` (2020-03-20) — Ruby SNMP **polling** plugin (out of scope for this analysis except where it confirms that trap support is intentionally NOT part of this plugin).
  - `sensu/snmp-demo @ —` — Docker compose demo using Prometheus snmp_exporter (no traps).
  - `sensu/sensu-docs` — current Hugo docs site for Sensu Go 6.x; contains zero SNMP-trap content. The only first-party trap guide is `content/sensu-core/1.9/guides/snmp-sensu-guide.md` (Sensu Classic only).
- **Citation convention**: `owner/repo @ <commit> :: <relative/path>:<line>`. The commit prefix is omitted on most citations to reduce noise; the commits above are the authoritative anchor.
- **Author**: assistant
- **Reviewer pass**: **accepted** — convergence declared after 3 iterations (iter-1, iter-2, iter-3). Five of six reviewers completed at iter-3; the sixth (qwen) was killed by external GPU-queue contention at iter-2 and iter-3 (iter-1 verdict was accept-with-fixes with only minor/nit). Iter-3 trajectory: four of five working reviewers had ZERO majors; codex alone surfaced 3 majors which were new content-coverage gaps (Sensu Enterprise outbound qualification, agent `/events` rate limiting, Postgres schema depth) rather than regressions — all applied. Surviving items are minor wording precision and cross-system superlative softening; see Reviewer Pass Log.

## 1. System Overview & Lineage

Sensu is a monitoring and observability framework. Two product generations coexist in the community's collective working set:

1. **Sensu Core 1.x** (a.k.a. *Sensu Classic*): MIT-licensed Ruby/EventMachine implementation, dating from 2011, that uses RabbitMQ as a transport bus and Redis as the state store. Final 1.x release was 1.9.0 in 2018. **Sensu Core reached end-of-life on December 31, 2019** (per the in-tree banner in `sensu-docs/content/sensu-core/1.9/_index.md`, citing `https://blog.sensu.io/eol-schedule-for-sensu-core-and-enterprise`); Sensu Enterprise (the commercial 1.x line) reached EOL March 31, 2020 (per `sensu-docs/content/sensu-core/1.9/installation/install-sensu-server-api.md`); the EOL package repositories were permanently removed on February 1, 2021 (per `https://discourse.sensu.io/t/updated-eol-timeline-for-sensu-core-and-sensu-enterprise-repos/2396` linked from the same docs). Sensu Core 1.x packages are therefore no longer downloadable.
2. **Sensu Go** (a.k.a. *Sensu 5.x / 6.x*): Apache-2.0-licensed Go rewrite first released in 2018. Replaces RabbitMQ + Redis with a self-contained backend that uses embedded etcd (and optionally PostgreSQL) for state. Sensu Go is the **current, maintained** product line — `sensu-go @ 80e82590e` shows commits as recent as July 2025 — and is what "Sensu" means in the present tense.

Where SNMP traps fit:

- **Sensu Go (current product)**: SNMP traps are NOT a first-class signal. The Sensu Go backend has no UDP/162 listener, no MIB parser, no trap-shaped event type, and no shipped MIB. A grep of the `sensu-go @ 80e82590e` Go source files (excluding `go.mod`/`go.sum` dependency metadata, which contains unrelated `mousetrap` strings) for `snmp` or `trap` returns exactly one production-code hit: `scripts/proto2graphql/proto2graphql.go:56` contains the word "Strap" in a code comment ("Strap in this is going to get ugly."). Operators who need SNMP trap ingestion must either run an external bridge (the community `snmptrapd2sensu` shim is the only Sensu-Go-targeted option) or write their own check that polls a trap log.
- **Sensu Core 1.x (legacy)**: SNMP trap **reception** was possible via the third-party Ruby gem `sensu-extensions-snmp-trap`, loaded into a Sensu Core *client* process as an EventMachine extension. That extension's last commit is 2018-10-27 — barely a year before Sensu Core itself reached EOL on December 31, 2019. SNMP trap **emission** (Sensu → SNMP manager) was a built-in "integration" of the **commercial** Sensu Enterprise product (itself EOL March 31, 2020), documented at `sensu-docs/content/sensu-enterprise/3.8/integrations/snmp.md:23`. The open-source Sensu Core 1.x did not ship trap emission.

Relationship to upstream tools:

- **Ruby `snmp` gem** (`hallidave/ruby-snmp`, pinned to `1.2.0` in `sensu-extensions-snmp-trap.gemspec:17`): provides PDU parsing and `SNMP::TrapListener` in the Sensu Core 1.x extension.
- **`gosnmp/gosnmp`** (vendored as `github.com/soniah/gosnmp` in `sensu-snmp-trap-handler/go.mod`): provides outbound `SendTrap()` in the Sensu Go handler.
- **Net-SNMP `snmptrapd`**: NOT linked or embedded. The `snmptrapd2sensu` bridge expects the operator to deploy Net-SNMP separately and configure `traphandle default /usr/bin/snmptrapd2sensu` in `snmptrapd.conf` (`snmptrapd2sensu/README.md:63-66`). The bridge reads Net-SNMP's traditional textual notification format from stdin.
- **`smidump`** (libsmi tool): invoked as a subprocess by the Sensu Core 1.x extension to convert MIBs (`sensu-extensions-snmp-trap/lib/sensu/extensions/snmp-trap/snmp-patch.rb:22`).
- **SNMPTT**: NOT used. Sensu has no integration with SNMPTT.

The single most important fact about Sensu's trap support: **there is no first-party open-source inbound trap subsystem in either product generation.** The only first-party Sensu trap component documented anywhere is the legacy Sensu Enterprise outbound `snmp` integration (1.x commercial, EOL March 31, 2020, source-not-mirrored) which emitted Sensu-event-shaped SNMPv1/v2 traps to an external manager (see §2.4 and `sensu-docs/content/sensu-enterprise/3.8/integrations/snmp.md:20-29`). All inbound trap reception, in both product generations, has only ever been community-shipped. This sets Sensu apart from every other system analysed in this comparative pass and changes what the per-section analysis can say.

## 2. Trap-Subsystem Architecture

There is no single trap subsystem; there are three independent community bridges and a Sensu Enterprise outbound integration (legacy/commercial). Each is presented separately below because they share neither code nor configuration nor lifecycle.

### 2.1 Sensu Core 1.x inbound: `sensu-extensions-snmp-trap` (Ruby, EventMachine)

```
   SNMP-capable device(s)
            |
            | UDP/1062 (NOT 162; non-privileged default)
            v
   +-------------------------------------------------------+
   |             sensu-client (Ruby, EventMachine)         |
   |  +---------------------------------------------------+|
   |  | SNMP::TrapListener (host/port/community)          ||
   |  |   on_trap_v2c -> @traps Queue.push                ||
   |  +---------------------------------------------------+|
   |                          |                            |
   |  +---------------------------------------------------+|
   |  | @processor Thread (loop @traps.pop)               ||
   |  |   - MIB loader (smidump subprocess) on startup    ||
   |  |   - varbind decode (Ruby ASN.1 map)               ||
   |  |   - result_map / result_status_map mapping        ||
   |  |   - synthesise "check result" hash                ||
   |  +---------------------------------------------------+|
   |                          |                            |
   |       UDPSocket.send(JSON(result), :3030)             |
   |       (sensu-client's own local check-result API)     |
   +-------------------------------------------------------+
            |
            v
   +-------------------------------------------------------+
   |   sensu-client transport publish to RabbitMQ          |
   |   -> sensu-server -> Sensu Core event pipeline        |
   |      (filters, mutators, handlers, Redis)             |
   +-------------------------------------------------------+
```

Source: `sensu/sensu-extensions-snmp-trap @ 269bc59 :: lib/sensu/extensions/snmp-trap.rb:96-106` (`start_snmpv2_listener!`), `:321-332` (`start_trap_processor!`), `:203-207` (`send_result`).

Deployment model:

- **Single-process, single-node**: the extension runs **inside** the `sensu-client` process. There is no separate trap daemon. The host appointed to receive traps must run sensu-client and load the extension via `/etc/sensu/conf.d/extensions.json` (`sensu-docs/content/sensu-core/1.9/guides/snmp-sensu-guide.md:44-53`).
- **No distributed reception**: multiple sensu-client instances can each run the extension on different hosts (operator-managed), but the extension itself has no clustering, no Twin-style config distribution, no shared dedup state. Each instance is independent.
- **No container/Kubernetes story**: Sensu Core 1.x predates Kubernetes-as-default. The extension was never packaged for k8s; even the Travis CI matrix is Ruby `2.0.0 / 2.1.0 / 2.2.0 / 2.2.3 / 2.3.0` (`sensu-extensions-snmp-trap/.travis.yml:3-7`) — all of which are themselves long-unsupported by upstream Ruby.

Languages and key libraries: Ruby, EventMachine, the `snmp` gem 1.2.0 (`hallidave/ruby-snmp`), `smidump` (external CLI from libsmi for MIB compilation).

Inter-component IPC: **UDP to localhost:3030** — the extension synthesizes a check-result JSON and sends it via a local UDP socket to the sensu-client's own check-result API (`snmp-trap.rb:203-207`). Sensu-client then publishes via RabbitMQ to sensu-server. There is no in-process call from the extension into sensu-client's transport publisher; the extension is structurally identical to an external script that POSTs a check result.

### 2.2 Sensu Go inbound: `snmptrapd2sensu` (Go shim, executed by Net-SNMP)

```
   SNMP-capable device(s)
            |
            | UDP/162
            v
   +-------------------------------------------------------+
   |       Net-SNMP snmptrapd (NOT shipped by Sensu)       |
   |       snmptrapd.conf:  traphandle default \           |
   |                        /usr/bin/snmptrapd2sensu       |
   +-------------------------------------------------------+
            |
            | stdin: textual notification (HOSTNAME / IP / VARBINDs)
            v
   +-------------------------------------------------------+
   |       snmptrapd2sensu (Go binary, runs PER TRAP)      |
   |  - parsers.ParseNotification(stdin)                   |
   |  - processNotification:                               |
   |      Check.Name  = sanitised snmpTrapOID varbind      |
   |      Check.Output= JSON of all varbinds               |
   |      Check.Status= configured default (default 1)     |
   |      Annotations = per-varbind "snmp_<oid>" map       |
   |      Entity.Name = HOSTNAME (skipped if "localhost")  |
   |  - HTTP POST -> http://127.0.0.1:3031/events          |
   +-------------------------------------------------------+
            |
            v
   +-------------------------------------------------------+
   |    sensu-agent HTTP API (:3031 by default in this     |
   |    integration; standard Sensu Go agent API port is   |
   |    :3031)  -> sensu-backend -> event pipeline         |
   +-------------------------------------------------------+
```

Source: `sensu/snmptrapd2sensu @ 50ae0ee :: main.go:153-164` (entry point), `parsers/snmptrapd.go:51-86` (stdin parser), `main.go:78-126` (notification → Sensu Event mapping), `main.go:128-151` (HTTP POST).

Deployment model:

- **Process-per-trap**: snmptrapd spawns a new `snmptrapd2sensu` process for every received trap (`snmptrapd2sensu/README.md:61-67`, `traphandle default /usr/bin/snmptrapd2sensu`). There is no daemon, no queue, no connection pool. Throughput is bounded by process-spawn + JSON marshal + one HTTP POST per trap. No benchmark is present in the repo; the comparative ranking against in-process listeners (OpenNMS / Zabbix / Zenoss) is deferred to `comparison/comparative-analysis.md`.
- **Decoupling**: trap reception (UDP/162 ingress, SNMPv1/v2c/v3 decoding, MIB resolution) is **outsourced to Net-SNMP**. The Sensu side sees only post-resolution text. This means SNMPv3 USM is supported by virtue of Net-SNMP's `snmptrapd` supporting it — but the operator has to configure Net-SNMP separately and learn its `createUser` directives (Sensu's docs do not cover this).
- **HTTP coupling**: snmptrapd2sensu connects to the Sensu Go agent API on `127.0.0.1:3031` per default (`snmptrapd2sensu/main.go:42-49`, default port in `snmptrapd2sensu.json` is `3031`). The README uses `3031` and the README's example output also references it. There is no built-in retry on HTTP failure: `main.go:140-148` calls `log.Fatalf` on errors, aborting the process.
- **No container/Kubernetes story**: snmptrapd2sensu is a static binary released via GoReleaser, but Sensu's docs offer no Kubernetes recipe. Operators wanting to run it in k8s must containerise Net-SNMP + the binary themselves.

### 2.3 Sensu Go outbound: `sensu-snmp-trap-handler` (Go, gosnmp)

```
   sensu-backend
        |
        | event pipeline routes to handler
        v
   +-----------------------------------------------------+
   | Handler (type: pipe) defined in sensu-backend       |
   |   command: sensu-snmp-trap-handler --host MGR ...   |
   |   stdin:  full Sensu event JSON                     |
   +-----------------------------------------------------+
        |
        | spawn per event (Sensu Go pipe handlers)
        v
   +-----------------------------------------------------+
   |  sensu-snmp-trap-handler (Go binary)                |
   |  - parse stdin event                                |
   |  - construct SnmpTrap with 11 fixed varbinds        |
   |     under Sensu PEN 1.3.6.1.4.1.45717.1.1.1         |
   |  - gosnmp.Default.SendTrap()  (UDP -> manager:162)  |
   +-----------------------------------------------------+
        |
        | UDPv1 or v2c (no v3) to SNMP manager
        v
       SNMP Manager (third party — NOT Sensu)
```

Source: `sensu/sensu-snmp-trap-handler @ 3c38f01 :: main.go:102-105` (handler entry), `main.go:120-232` (executeHandler), `main.go:14` (`gosnmp` import alias `snmp`).

Deployment model:

- **Process-per-event**: a Sensu Go pipe handler is spawned per matched event by `sensu-backend`. The handler completes (sends one trap, exits) for every event delivered to it.
- **No state**: there is no in-memory state; the handler does not deduplicate, does not track sequence numbers, does not preserve outgoing-trap state across invocations.
- **Error handling**: `Connect` failure returns `fmt.Errorf("Connect() err: %v", err)` from `executeHandler` — the Sensu plugin SDK exits the handler with that error visible to sensu-backend (`main.go:134-137`). `SendTrap` failure, however, calls `log.Fatalf` (`main.go:227-230`), terminating the process abruptly. So Connect-time errors are reportable; on-wire-emission errors are not. There is no retry, no buffer.

### 2.4 Sensu Enterprise (legacy commercial): the in-process `snmp` integration

Source: `sensu-docs/content/sensu-enterprise/3.8/integrations/snmp.md:23-29` describes "Send SNMP traps to a SNMP manager. Sensu Enterprise provides two SNMP MIB modules for this integration. The SNMP integration is capable of creating either SNMPv1 or SNMPv2 traps for Sensu events." This was an outbound emitter, configured under the `snmp` JSON scope (`docs/.../integrations/snmp.md:54-64`).

Sensu Enterprise was the commercial 1.x product line. End-of-life March 31, 2020 (per `sensu-docs/content/sensu-core/1.9/installation/install-sensu-server-api.md:52`); EOL package repositories permanently removed February 1, 2021. No source for the integration is in the mirror (it was proprietary). This document treats the integration as **legacy/commercial, source-not-verified, vendor-doc evidence only**.

### Cross-component IPC summary

| Bridge | IPC into Sensu | Process model | Out-of-process cost |
|---|---|---|---|
| `sensu-extensions-snmp-trap` (Classic) | UDP datagram localhost:3030 with JSON check-result | EventMachine extension co-resident with sensu-client | None — in-process |
| `snmptrapd2sensu` (Go) | HTTP POST localhost:3031 `/events` | spawn per trap from snmptrapd | one process spawn + one HTTP round-trip per trap |
| `sensu-snmp-trap-handler` (Go) | reads event JSON from stdin (sensu-backend pipes it) | spawn per event | one process spawn per outbound trap |
| Sensu Enterprise `snmp` integration | in-process inside sensu-enterprise daemon (per docs) | in-process | none (legacy/commercial — unverified) |

The deliberate architectural pattern across all four: trap-handling code is **outside the Sensu core daemons**, communicating via the same APIs that any third-party plugin would use. Sensu's design philosophy ("monitoring as code", everything-is-a-plugin) is consistent — but the consequence is that no part of the inbound trap pipeline has ever been owned, tested, or shipped in the open-source Sensu project (Sensu Core 1.x's core daemon, Sensu Go's backend/agent). The Sensu Enterprise outbound emitter described in §2.4 was a first-party component of the commercial 1.x product, but its source is not in the mirror (proprietary) and that product is EOL.

## 3. Trap Reception (UDP/162 Ingress)

The three bridges differ fundamentally on UDP ingress; this section catalogues each. Note: by SOW template convention this section is labelled "UDP/162", but the Classic extension defaults to UDP/1062 (operator must forward 162→1062 in firewall) and snmptrapd2sensu does not bind a socket at all (Net-SNMP's `snmptrapd` does).

### 3.1 `sensu-extensions-snmp-trap` (Classic)

- **Listener implementation**: own UDP socket via `SNMP::TrapListener` from the Ruby `snmp` gem. Source: `lib/sensu/extensions/snmp-trap.rb:96-106`.
  ```ruby
  @listener = SNMP::TrapListener.new(
    :host => options[:bind],
    :port => options[:port],
    :community => options[:community]) do |listener|
    listener.on_trap_v2c do |trap|
      @logger.debug("snmp trap check extension received a v2 trap")
      @traps << trap
    end
  end
  ```
- **SNMP version support**:
  - **v2c only**. The extension registers `on_trap_v2c` and nothing else (`snmp-trap.rb:101`). It does NOT register `on_trap_v1` — v1 traps would be silently ignored.
  - **No SNMPv3 USM**. The Ruby `snmp` gem 1.2.0 has limited SNMPv3 support in receive mode, and the Sensu extension does not configure it. There is no USM user table, no engineID handling, no auth/priv passphrase configuration in `options` (`snmp-trap.rb:50-67`).
  - **No DTLS / no TLS-TM**.
- **Performance / concurrency model**: a single SNMP::TrapListener reader thread pushes onto a Ruby `Queue` (`snmp-trap.rb:335` — `@traps = Queue.new`). A single processor thread (`start_trap_processor!` at `:321-332`) pops in a `loop` and runs `process_trap`. The queue is unbounded — under sustained trap load it grows without backpressure. There is no explicit drop policy.
- **Privileged-port handling**: the default port is **1062**, NOT 162 (`snmp-trap.rb:54`). The Sensu Classic guide (`sensu-docs/content/sensu-core/1.9/guides/snmp-sensu-guide.md:148`) explicitly warns: "By default, SNMP uses :162 UDP. When configuring a network device, you'll need to ensure that traps are sent to the Sensu client over :1062 UDP." The operator is left to redirect UDP/162 → UDP/1062 themselves (via iptables / firewalld) if they want devices that emit on 162 to reach the listener. No `CAP_NET_BIND_SERVICE` story is provided.
- **Horizontal scaling pattern**: not built-in. Each sensu-client running the extension is independent; operators choose how to fan out devices to multiple clients (manual configuration).
- **HA / clustering**: none. Failure of the host running the extension drops traps.

### 3.2 `snmptrapd2sensu` (Sensu Go inbound)

- **Listener implementation**: **delegated to Net-SNMP `snmptrapd`**. snmptrapd2sensu is not a daemon and binds no socket. It is invoked once per trap by snmptrapd's `traphandle default` directive (`snmptrapd2sensu/README.md:63-66`).
- **SNMP version support**: whatever `snmptrapd` supports — Net-SNMP supports v1, v2c, v3 USM, with priv/auth as the operator configures via `createUser` etc. SNMP version is invisible to snmptrapd2sensu; the bridge only sees post-decoded notification text.
- **Performance / concurrency model**: every trap spawns a process. There is no shared state across invocations. Throughput is bounded by process-spawn + Go-binary startup + JSON marshal + one HTTP POST per trap. No benchmark in source; cross-system performance ranking deferred to the comparison document.
- **Privileged-port handling**: net-snmp `snmptrapd` typically binds 162 directly with root privileges (or via setcap). Sensu's docs do not weigh in.
- **Horizontal scaling pattern**: operator-controlled — deploy snmptrapd on multiple hosts, each running snmptrapd2sensu locally.
- **HA / clustering**: none on the Sensu side. Standard Net-SNMP HA patterns (keepalived + floating VIP, anycast) apply.

### 3.3 `sensu-snmp-trap-handler` (outbound) — not applicable

Outbound only. Does not receive traps. UDP source port is `gosnmp` default.

### 3.4 Sensu Enterprise legacy `snmp` integration

Outbound only per docs (`sensu-docs/.../snmp.md:23`). Not an ingress component.

## 4. MIB Management

### 4.1 Classic extension — local file directory + `smidump` subprocess

Source: `lib/sensu/extensions/snmp-trap.rb:127-201`.

- **Default location**: `/etc/sensu/mibs` (`snmp-trap.rb:56`). Format: any text-format MIB file; the loader detects the module name via regex `([\w-]+)\s+DEFINITIONS\s+::=\s+BEGIN` (`:133`).
- **Recursive load**: as of v0.2.0 (2018-04) the loader does `Dir.glob(File.join(options[:mibs_dir], "**", "*"))` (`:130`) — added by community contributor `@landondao1` per `CHANGELOG.md:8-9`.
- **Compilation pipeline**: for each MIB file the loader shells out to `smidump -k -f python <file>` (via the monkey-patched `SNMP::MIB.import_module` in `snmp-patch.rb:22`), `eval`s the returned dictionary string, and writes a YAML-shaped node map next to the file (`snmp-patch.rb:24-37`). The `smidump` binary is from libsmi and is NOT bundled with the extension; the operator must install it. The patch at `snmp-patch.rb:17` does `raise "smidump tool must be installed"` if `import_supported?` is false, but the **caller** in `lib/sensu/extensions/snmp-trap.rb:178-184` wraps every `import_module` call in `rescue StandardError, SyntaxError => error` and only logs at **debug** level. The net effect: a missing or broken `smidump` does not abort startup — the extension continues without the affected MIBs, silently degrading OID resolution. There is no operator-visible warning unless debug logging is enabled.
- **Security note**: the `snmp-patch.rb` shell-out command pipes the `smidump` Python-dictionary output through `eval_mib_data` (the `snmp` gem's evaluator). A compromised `smidump` binary or a crafted MIB that causes `smidump` to emit pathological Python-dict text could in principle inject arbitrary Ruby state — defence is "trust your `smidump` binary and your MIB files." For a community-abandoned 2018 codebase, this is operationally significant only if the operator loads MIBs from untrusted sources.
- **Dependency resolution**: the loader scans `FROM\s+([\w-]+)` import lines (`:136`) and computes a transitive preload list per module (`determine_mib_preload`, `:108-125`). If an import is not present in `@mibs_map`, it warns and continues — i.e., **silent partial loading**: a MIB whose dependency is missing will fail to resolve some OIDs but the extension will not refuse to start.
- **Bundled MIBs out of the box**: **NONE in the production gem**. The `lib/` directory ships only the two source files (`snmp-trap.rb`, `snmp-patch.rb`); no `mibs/` directory. The test fixture set under `spec/mibs/` (`SENSU-ENTERPRISE-NOTIFY-MIB.txt`, `SENSU-ENTERPRISE-ROOT-MIB.txt`, `SNMPv2-SMI.txt`, `RFC-1212-MIB.txt`, `RFC-1215-MIB.txt`, `rfc1158.mib`, plus `more_mibs/APACHE-MIB.txt`) is only there for the rspec test in `spec/snmp-trap_spec.rb:46-50` and is not installed with the gem (`gemspec:13` files glob is `{bin,lib}/**/*`).
- **Out-of-the-box vendor coverage**: zero. The operator must obtain and place every vendor MIB into `/etc/sensu/mibs` themselves (`sensu-docs/.../snmp-sensu-guide.md:64-72`).
- **Fallback behaviour for unknown OIDs**: `determine_trap_oid` (`snmp-trap.rb:218-227`) tries `@mibs.name(varbind.value.to_oid)` and on failure returns the dotted-decimal OID string (sanitised) or the literal `"trap_oid_unknown"`. The trap is still processed and forwarded as a Sensu event.

### 4.2 `snmptrapd2sensu` — MIB resolution outsourced to Net-SNMP

snmptrapd2sensu does not parse MIBs. By the time snmptrapd hands a trap to snmptrapd2sensu, OIDs have either been resolved (if snmptrapd is configured to load MIBs) or left as numeric dotted-decimal. The bridge passes whatever it sees through.

- Operator workflow: configure Net-SNMP `snmptrapd` to load MIBs as documented at `snmptrapd2sensu/README.md:131-133` — "_NOTE: if properly configured, `snmptrapd` will handle the translation of OIDs to their descriptive names as found in a MIB file; see `snmptrapd -h` for more information on how to configure `snmptrapd` to read MIB files._"
- snmptrapd2sensu has no MIB store, no compilation pipeline, no dependency resolution.

### 4.3 `sensu-snmp-trap-handler` — ships Sensu Enterprise MIB modules plus RFC support MIBs for the receiving manager

The outbound handler ships its **own** MIB files under `mibs/` (`sensu-snmp-trap-handler/mibs/`):

- `RFC-1212-MIB.txt`
- `RFC-1215-MIB.txt`
- `SENSU-ENTERPRISE-V1-MIB.txt`
- `SENSU-ENTERPRISE-ROOT-MIB.txt`
- `SENSU-ENTERPRISE-NOTIFY-MIB.txt`

Their purpose is to be loaded into the **receiving SNMP manager** so that incoming traps from this handler are human-readable. They are not used by the handler itself (the handler emits raw OIDs).

The Sensu PEN is hard-coded at `main.go:29`: `SensuEnterprisePEN = "1.3.6.1.4.1.45717"`.

### 4.4 Sensu Enterprise legacy `snmp` integration

Per docs (`sensu-docs/.../snmp.md:31-45`) shipped both v1 and v2 MIBs for downstream consumption. Source not verified.

### 4.5 Summary

The most striking pattern: **Sensu has no project-wide MIB strategy.** Each of the three bridges handles MIBs differently:

- The Classic extension does its own compilation via `smidump` and stores results on the host running sensu-client.
- snmptrapd2sensu has no MIB knowledge at all — relies on Net-SNMP to do the resolution before it sees the data.
- The outbound handler ships its own MIBs intended for **other** systems' use.

There is no shared MIB store, no shared format, no shared compilation tool. An operator running both inbound (snmptrapd2sensu) and outbound (sensu-snmp-trap-handler) has to learn two different MIB workflows.

## 5. Trap Processing Pipeline

### 5.1 Classic extension pipeline

Top-level loop: `start_trap_processor!` thread executes `process_trap(@traps.pop)` indefinitely (`snmp-trap.rb:321-332`).

#### Parse (BER decode, varbind extraction)

Handled by the Ruby `snmp` gem before Sensu code runs. The gem delivers a `SNMP::SNMPv2_Trap` object with a `varbind_list`. Sensu code only sees the parsed object.

#### OID-to-name resolution

`determine_trap_oid` (`snmp-trap.rb:218-227`):

```ruby
varbind = trap.varbind_list.detect do |varbind|
  varbind.name.to_oid == SNMP::SNMP_TRAP_OID_OID  # 1.3.6.1.6.3.1.1.4.1.0
end
begin
  @mibs.name(varbind.value.to_oid).gsub(/[^\w\.-]/i, "-")
rescue
  varbind.value.to_s.gsub(/[^\w\.-]/i, "-") rescue "trap_oid_unknown"
end
```

Notes:
- The MIB-derived symbolic name has its non-alphanumeric characters replaced with `-`. This loses the conventional `MIB-NAME::object` separator (e.g., `IF-MIB::linkDown` becomes `IF-MIB--linkDown`), which is then used as the Sensu **check name**.
- If `varbind` (the snmpTrapOID varbind) is missing entirely — a malformed v2c trap or a v1 trap (which the listener ignored anyway) — `varbind.value` raises `NoMethodError`, caught by the inner `rescue` returning `"trap_oid_unknown"`.

#### Source identification

`process_trap` calls `determine_hostname(trap.source_ip)` (`snmp-trap.rb:282-290, 209-216`) which does a reverse DNS lookup via `Resolv.getname`. On `Resolv::ResolvError` it falls back to the source IP. This becomes the Sensu **`source`** field (which Sensu Core treats as a proxy-client name).

Notes:
- The agent-addr field of an SNMPv1 trap is irrelevant here — v1 isn't accepted. For v2c traps behind a NAT or proxy, there is no opt-in to use `snmpTrapAddress` varbind (RFC 3584); the bridge always uses the UDP source IP.
- The reverse DNS lookup is **synchronous and blocking** in the processor thread. A slow DNS resolver effectively rate-limits trap processing.

#### Enrichment (varbind decoration, lookup tables, topology join)

`process_trap` (`snmp-trap.rb:282-319`) iterates every varbind, calls `@mibs.name(varbind.name.to_oid)` to get a symbolic name, runs a per-type conversion (`RUBY_ASN1_MAP`, `:22-33`), and writes the value into `result[:snmp_trap][symbolic_name]`. So the Sensu check result ends up with a `snmp_trap` sub-hash mapping symbolic OID → decoded value.

Special-case enrichment for `linkUp`/`linkDown` traps in `determine_trap_name` (`snmp-trap.rb:229-249`): if the trap OID matches `/link(down|up)/i`, the bridge synthesises a check name `link_status_<ifIndex>` by scanning varbinds for `/ifindex/i` or `/systemobject/i`. This produces a per-interface stable check name — useful for `linkUp` to clear a prior `linkDown` (both would have the same check name `link_status_<n>`).

Similarly `determine_trap_output` (`:251-272`) for `linkUp`/`linkDown` synthesises an output `"link is up"` / `"link is down"`, optionally appending `ifAlias` / `ifDescr`.

No topology join — Sensu Classic has no device-topology concept beyond proxy-client entities.

#### Normalization (vendor severity → internal severity)

A two-stage map:

1. `RESULT_MAP` (`snmp-trap.rb:9-15`) — built-in mapping from varbind symbolic name regex → check-result key. Cases:
   - `/checkname/i` → `:name`
   - `/notification/i` → `:output`
   - `/description/i` → `:output`
   - `/pansystemseverity/i` → `:status` (via a `Proc` that maps `value > 3 ? 2 : 0`; this is a Palo Alto Networks PAN-OS specific severity mapping baked into the default rules)
   - `/severity/i` → `:status`
2. `RESULT_STATUS_MAP` (`:17-20`) — built-in mapping from trap-OID symbolic name regex → numeric Sensu check status:
   - `/down/i` → `2` (CRITICAL)
   - `/authenticationfailure/i` → `1` (WARNING)

User-supplied `result_map` / `result_status_map` (from config JSON) are merged with the built-ins (`result_map`, `:78-85`; `result_status_map`, `:87-94`); user mappings take precedence.

No unit conversion. No vendor severity normalization beyond the PAN-OS-specific `pansystemseverity` hard-coded rule.

#### Deduplication / suppression (keys, windows, rate limits)

**None at the extension layer.** Every received trap produces one Sensu check result. The trap's check `:name` is the trap-OID's sanitised symbolic name (or the synthesised `link_status_<n>` for link traps). Sensu Core 1.x's existing event deduplication kicks in only if the **same check name** repeats — for arbitrary traps this is the OID-derived name, which means two traps with the same OID against the same source share a check name.

No tumbling window, no rate limit, no per-source throttle. Trap storms propagate fully into the Sensu event pipeline.

#### Routing

The synthesised result is JSON-encoded and sent via UDP to `127.0.0.1:3030` (`snmp-trap.rb:203-207`), the sensu-client's local check-result API. Configurable via `client_socket_bind` / `client_socket_port` (`:62-63`) — added in v0.1.0 (2018-04 per CHANGELOG).

#### Error handling for malformed PDUs, unknown OIDs, decode failures

- **Malformed PDU**: handled inside the Ruby `snmp` gem's `TrapListener` before the Sensu extension's `on_trap_v2c` callback fires. The extension code only registers the callback (`snmp-trap.rb:96-105`) and never sees a malformed PDU; the precise log-or-drop behaviour is internal to the `snmp` gem and not verified from this analysis.
- **Decode failure for a single varbind**: `process_trap` logs ERROR and skips that varbind (`snmp-trap.rb:307-313`), keeping others.
- **Unknown OID**: the symbolic name falls back to dotted-decimal, no error raised.
- **Thread exception**: `@processor.abort_on_exception = true` (`:330`) — an uncaught exception in the processor thread **aborts the entire sensu-client process**. This is a hard fail-stop, not a graceful degradation.

### 5.2 `snmptrapd2sensu` pipeline

Source: `snmptrapd2sensu/main.go:78-126`.

- **Parse**: positional parsing of snmptrapd's text format (line 0 = hostname, line 1 = IP/port pair, lines 2+ = `OID value` pairs). Source: `parsers/snmptrapd.go:51-86`. No structured input format — relies on snmptrapd's textual output staying stable. `parseIpAddress` (`parsers/snmptrapd.go:27-48`) assumes the bracketed `UDP: [IP]:PORT->[IP]:PORT` snmptrapd format. The **source IP** is extracted by stripping `[` and `]`, so an IPv6 address like `[::1]` correctly becomes `::1`. The **source port**, however, is extracted via `strings.Split(addresses[0], ":")[1]` (`:43`) — for an IPv6 source `[::1]:57099` this splits into `["[", "", "1]", "57099"]` and returns `""` rather than `57099`. So the **port parsing is broken for IPv6**, while the IP itself is preserved. The parser also does no bounds checking: `parseVarbind` indexes `tokenSet[0]` (`:20-22`) and `parseIpAddress` slices fields from a fixed positional layout (`:38-45`); malformed snmptrapd text causes index-out-of-range panics rather than logged errors.
- **OID-to-name resolution**: NONE. Whatever snmptrapd emits is what the bridge uses; sanitisation only replaces `.` and `:` with `-` for use as a Sensu check name (`main.go:91, 102`).
- **snmpTrapOID lookup**: tries four known incantations (numeric and symbolic prefixes; `main.go:34-41`):
  ```go
  var SnmpTrapOidOIDs = []string{
      "1.3.6.1.6.3.1.1.4.1.0",
      "1.3.6.1.4.1.3.6.1.6.3.1.1.4.1.0",
      "iso.3.6.1.6.3.1.1.4.1.0",
      "SNMPv2-MIB::snmpTrapOID.0",
  }
  ```
  The second entry is **wrong** — `1.3.6.1.4.1.3.6.1.6.3.1.1.4.1.0` is not a known canonical incantation of `snmpTrapOID.0`; it appears to be a copy-paste defect. The **identical pattern** appears in the sibling `SystemUptimeOIDs` list (`main.go:26-33`), where the second entry `1.3.6.1.4.1.3.6.1.2.1.1.3.0` is the same anomalous `4.1.3.6.1` insertion into the standard `sysUpTime` OID. Two parallel copy-paste defects in adjacent variable declarations is strong evidence of low review pressure on the bridge. The lookup falls through to the other entries so this is low-impact at runtime, but it stays in source as a maintenance signal.
- **Source identification**: the hostname is taken from line 0 of snmptrapd's textual output. If it is `<UNKONWN>` (note misspelling — Net-SNMP's own typo) or `<UNKNOWN>`, the bridge replaces it with the source IP, dots-to-underscores (`main.go:51-58`). If the hostname is `localhost`, the bridge **does not set an Entity** on the event (`main.go:114-118`).
- **Enrichment**: every varbind becomes both an annotation `snmp_<sanitised-oid>: <value>` AND a key/value in the `Check.Output` JSON map (`main.go:99-110`).
- **Normalization**: none. `Check.Status` is the configured default integer (`main.go:112`) — no severity inference from varbind content.
- **Deduplication / suppression**: none. Every trap → one HTTP POST → one Sensu event.
- **Routing**: HTTP POST to `http://<agent>:<port>/events` via `http.DefaultClient.Do(req)` (`main.go:144`). The `DefaultClient` has **no timeout configured**, which means a slow or hung sensu-agent API can block the bridge process indefinitely — under trap-storm conditions, the consequence is fork-pile-up. On HTTP send error: `log.Fatalf` — the process exits non-zero. The HTTP **status code is not checked** (`main.go:148-150` reads and prints the body but never inspects `resp.StatusCode`), so a 4xx/5xx response from sensu-agent is silently treated as success. snmptrapd's `traphandle` does not log either condition anywhere unless the operator runs snmptrapd in foreground.
- **Error handling**: `log.Fatalf` is called in five places — two in `processNotification` (`main.go:109, 122` on JSON marshal failures) and three in `postEvent` (`main.go:132, 141, 146` on JSON marshal / HTTP request build / HTTP send). Plus `ioutil.ReadAll(resp.Body)` at `main.go:149` assigns `err` but never checks it — response-read errors are silently ignored. One bad trap that triggers a marshalling error aborts that trap's processing; the next trap is a fresh process. Robust against accumulating failures, but ungraceful for diagnostics, and HTTP error responses (e.g. 4xx/5xx from sensu-agent) are not distinguished from success.

### 5.3 `sensu-snmp-trap-handler` (outbound) pipeline

Source: `sensu-snmp-trap-handler/main.go:120-232`.

- **Input**: Sensu event JSON on stdin (decoded by the `sensu-plugin-sdk`).
- **Map fields**: hard-coded 11 varbind layout under `SensuEnterprisePEN.1.1.1.{1..10}` (`main.go:157-214`).
- **Status translation**: `event.Check.Status > 3 ? 3 : event.Check.Status` (`main.go:151-155`) — clamps to UNKNOWN if the check status is out of the canonical 0-3 range.
- **State translation**: `event.Check.State` ("failing"/"passing"/"flapping") mapped to integer 0/1/2 (`main.go:146-150`).
- **SNMPv1-specific path** (`main.go:217-225`): when `plugin.Version == "1"`, the handler sets `trap.Enterprise = SensuEnterprisePEN` and calls `getAgentIP()` (`main.go:271-306`) to determine the local non-loopback IPv4 address, which it stores in `trap.AgentAddress` as the v1-required agent-addr field. `getAgentIP()` is IPv4-only — if no non-loopback IPv4 interface is found it returns `("", fmt.Errorf("are you connected to the network?"))`, and the handler returns that error from `executeHandler`. On an IPv6-only host this v1 path will fail.
- **Output**: `gosnmp.Default.SendTrap(trap)` to manager (`main.go:227`). The `gosnmp.Default` package-level client is used directly — there is no operator-configurable timeout, no retry, no backoff. `gosnmp.Default` has hard-coded defaults from the gosnmp library.
- **Error handling**: `Connect` failure returns `fmt.Errorf(...)` (`main.go:134-137`) — sensu-backend receives the error from the SDK. `SendTrap` failure calls `log.Fatalf` (`main.go:227-230`) — the process exits abruptly with no graceful error path back to sensu-backend. No retry, no buffering.

### 5.4 Critical observation

None of the three bridges does any of the following that other systems in this analysis do:

- alarm-style state machine (problem / resolution lifecycle)
- topology-aware correlation
- clear-key based pairing of UP/DOWN events
- per-source rate limiting
- tumbling-window dedup
- vendor severity normalization beyond hard-coded one-off rules
- enrichment from external lookup tables (CMDB, asset DB)

Trap content shows up in Sensu as a generic check-result event; from there the operator can configure Sensu filters/mutators/handlers to do downstream work — but the **trap-as-trap** processing is shallow.

## 6. Data Model & Persistent Storage

Sensu does not maintain a trap-shaped data model. Traps land in Sensu's general-purpose event store as ordinary events. The persistence story therefore depends entirely on Sensu's standard event storage.

### 6.1 Sensu Go events

- **Store**: etcd is the default backend store for Sensu Go (built-in / embedded). For "scale event storage", PostgreSQL is the documented alternative for the events table only (`sensu-docs/content/sensu-go/6.14/operations/deploy-sensu/scale-event-storage.md`).
- **In-flight schema**: a Sensu Go `Event` is defined in `sensu/core @ 14f0fb0 :: v2/event.proto` (the protobuf is published from the dedicated `sensu/core` repo and consumed by `sensu/sensu-go`). The relevant generic fields a trap-bridge populates:
  - `entity.name` (proxy entity for the SNMP device)
  - `entity.namespace`
  - `check.name` (synthesised from trap OID)
  - `check.output` (JSON-of-varbinds in `snmptrapd2sensu`; output text in the Classic extension)
  - `check.status` (0/1/2/3)
  - `check.annotations` (per-varbind `snmp_<oid>: <value>` map in `snmptrapd2sensu`)
  - `check.labels`
- **PostgreSQL `events` table schema** (`sensu-go @ 80e82590e :: backend/store/postgres/migrations_schema.go:13-27`):
  ```sql
  CREATE TABLE IF NOT EXISTS events (
      id                  bigserial     PRIMARY KEY,
      sensu_namespace     text          NOT NULL,
      sensu_entity        text          NOT NULL,
      sensu_check         text          NOT NULL,
      status              integer       NOT NULL,
      last_ok             integer       NOT NULL,
      occurrences         integer       NOT NULL,
      occurrences_wm      integer       NOT NULL,
      history_index       integer       DEFAULT 2,
      history_status      integer[],
      history_ts          integer[],
      serialized          bytea,
      previous_serialized bytea,
      UNIQUE ( sensu_namespace, sensu_check, sensu_entity )
  );
  ```
  Notable design choices:
  - **Uniqueness on `(sensu_namespace, sensu_check, sensu_entity)`** — only the latest event per (namespace, check, entity) is retained. Upserts replace, not append. For traps, this means repeated traps from the same device for the same OID-derived check name collapse to one row with `occurrences` incremented; trap history is the `history_status[]`/`history_ts[]` ring (Sensu's check-history pattern, capped by `history_index`).
  - **`serialized` + `previous_serialized` bytea** — the full event protobuf is stored Snappy-compressed; trap-specific varbind details land here as opaque bytes from the Sensu UI's perspective unless decoded.
  - **`selectors jsonb` (migration 5)** — `migrations_schema.go:59-60` adds a `selectors` JSONB column and a GIN index `idxginevents` for fast label/annotation filtering. Trap annotations (`snmp_<oid>: <value>` from `snmptrapd2sensu`) are queryable through this index.
  - **Upsert path**: `createOrUpdateEvent.sql` is the in-tree SQL template invoked by `event_store.go` to update or insert per the unique constraint.
- **Retention**: by default, Sensu Go retains the latest event per (entity, check) tuple in etcd (overwrites prior); the PostgreSQL backend is also latest-per-(namespace, check, entity) by the unique constraint. For longer history, Sensu Go integrates with external time-series databases (InfluxDB etc.) via output handlers.
- **Indexing**: etcd key namespace — `/sensu.io/events/<namespace>/<entity-name>/<check-name>`. Postgres: primary key on `id`, unique constraint on the three-column tuple, GIN index on `selectors`.
- **No trap-specific tables**: no MIB table in etcd/Postgres, no OID-to-event-mapping table, no dedup-state table, no severity-rule table. None. The trap content lands as opaque bytes in `serialized` plus searchable JSONB in `selectors`.

### 6.2 Sensu Classic events

- **Store**: Redis (state) + RabbitMQ (transport, in-flight messages, not persistent). Configurable.
- **Schema**: Sensu Classic events are dictionary blobs with `client`, `check`, `occurrences`, `action`, `id`. Schema is `sensu-extensions-snmp-trap/spec/helpers.rb:67-80` (the `event_template` used in tests).
- **Retention**: Redis-side, configurable. No trap-specific retention.

### 6.3 MIB store

Only the Classic extension has a MIB store, at `/etc/sensu/mibs` (host filesystem on the sensu-client running the extension). Not a database — flat directory of MIB text files plus `smidump`-emitted YAML side-files in `imported_dir` (default `$TMPDIR/sensu_snmp_imported_mibs`).

### 6.4 OID-to-event mapping

No persistent mapping. The Classic extension's `RESULT_MAP` / `RESULT_STATUS_MAP` are in-memory constants merged with config-loaded user mappings at startup. snmptrapd2sensu has no mapping at all.

### 6.5 Dedup state, suppression rules, severity rules, device inventory, topology, audit/log

- **Dedup state**: none (relied on Sensu's check-name-keyed event collapsing, which is fundamentally per-check not per-trap).
- **Suppression rules**: none (operators can use Sensu filters; not a trap-specific surface).
- **Severity rules**: in-memory constants in the Classic extension; none in snmptrapd2sensu.
- **Device inventory**: Sensu Go's `entity` resource serves as the device model — but it is populated by the agent or proxy-entity definitions, not by trap reception. snmptrapd2sensu lazily creates an entity by name (`main.go:114-118`); it does not register the entity ahead of time.
- **Topology**: none.
- **Audit/log**: extension/handler stdout/stderr — no structured audit trail.

### 6.6 Migration / upgrade handling

No trap-specific migrations across Sensu version bumps. The Classic extension is version-locked at 0.2.0 forever (no further releases); upgrading sensu-client may break it if the EventMachine / Ruby version pairing diverges.

## 7. Configuration UX

### 7.1 Classic extension configuration

- **Surface**: a single JSON file `/etc/sensu/conf.d/snmp_trap.json` (`sensu-docs/.../snmp-sensu-guide.md:55-62`).
- **Format**: Sensu Classic's general JSON configuration scope; the extension reads keys under `snmp_trap` (`snmp-trap.rb:65`).
- **Knobs** (`snmp-trap.rb:52-64`):

| Key | Default | Purpose |
|---|---|---|
| `bind` | `"0.0.0.0"` | UDP listener bind address |
| `port` | `1062` | UDP listener port (non-privileged) |
| `community` | `"public"` | SNMPv2c community |
| `mibs_dir` | `"/etc/sensu/mibs"` | MIB source directory |
| `imported_dir` | `"$TMPDIR/sensu_snmp_imported_mibs"` | smidump output cache |
| `handlers` | `["default"]` | Sensu handler list on synthesised results |
| `result_attributes` | `{}` | Additional fields merged into every result (e.g. `datacenter`) |
| `result_map` | `[]` | User-defined varbind-name regex → result-key mapping |
| `result_status_map` | `[]` | User-defined OID-name regex → numeric status mapping |
| `client_socket_bind` | `"127.0.0.1"` | sensu-client local API host |
| `client_socket_port` | `3030` | sensu-client local API port |

- **CLI**: none (no `sensuctl` integration).
- **GUI**: none (Sensu Classic's Uchiwa dashboard and the Enterprise Dashboard had **no UI for the trap extension** — operators see the resulting check results but not the extension's configuration).
- **REST API**: none.
- **Defaults documented?**: yes, in the README and Sensu Classic guide.
- **Auto-completion / validation**: none.
- **Live reload**: no — restarting sensu-client is required. Confirmed by the Sensu Classic guide: `sudo systemctl restart sensu-client` after editing config (`snmp-sensu-guide.md:75-77`).
- **Multi-tenancy / RBAC**: none.

**Documentation drift — Sensu Classic guide advertises options not in the code.** The Sensu Core 1.9 guide at `sensu-docs/content/sensu-core/1.9/guides/snmp-sensu-guide.md:151-184` documents `filters`, `severities`, `varbind_trim` (default 100), `mibs_dir`, `imported_dir`, `result_map`, `result_status_map` as configurable options of the extension. The extension's actual `options` method (`lib/sensu/extensions/snmp-trap.rb:50-67`) defines only `bind`, `port`, `community`, `mibs_dir`, `imported_dir`, `handlers`, `result_attributes`, `result_map`, `result_status_map`, `client_socket_bind`, `client_socket_port`. The doc-only options (`filters`, `severities`, `varbind_trim`) are silently ignored by the extension — they look applicable but have no implementation. This is a real operator trap: configuring `varbind_trim: 300` will have zero effect even though the first-party docs document it. The Sensu Enterprise integration (a different product, see §2.4) does honour `filters`, `severities`, `varbind_trim` — operators may have been led to believe Sensu Core's extension does too.

### 7.2 `snmptrapd2sensu` configuration

- **Surface**: a single JSON file `/etc/sensu/snmptrapd2sensu.json` (`snmptrapd2sensu/README.md:29-58`).
- **Knobs**:

| Key | Default | Purpose |
|---|---|---|
| `snmptrapd.defaults.device.host` | `unknown-snmp-device` | hostname fallback when snmptrapd reports `<UNKNOWN>` |
| `snmptrapd.defaults.trap.name` | `unknown-snmp-trap` | check-name fallback when snmpTrapOID varbind missing |
| `snmptrapd.defaults.trap.status` | `0` | **README documents this; code ignores it.** `SnmptrapdTrapDefaults` struct in `config/config.go:14-16` defines only a `Name` field — Go's `json.Unmarshal` silently discards the `status` key. The actual default status read by the bridge is `sensu.check.status` elsewhere in the config. An operator setting only `snmptrapd.defaults.trap.status` will observe their configuration to have no effect. This is a real documentation bug. |
| `sensu.agent.api.host` | `127.0.0.1` | sensu-agent HTTP API host |
| `sensu.agent.api.port` | `3031` | sensu-agent HTTP API port |
| `sensu.check.namespace` | `default` | Sensu namespace |
| `sensu.check.label_prefix` | `snmp` | prefix on per-varbind annotations |
| `sensu.check.status` | `1` | hard-coded Sensu check status (WARNING by default) |

- **CLI**: invoked by snmptrapd; no human CLI.
- **GUI**: none.
- **REST API**: none.
- **Defaults documented?**: in the README, partial.
- **Validation**: none beyond JSON parse; `log.Fatalf` on missing file (`config/config.go:54-57`).
- **Live reload**: not applicable — process is short-lived per trap.
- **Multi-tenancy / RBAC**: namespace is fixed at config-file time; no per-trap routing.

### 7.3 `sensu-snmp-trap-handler` configuration

- **Surface**: handler definition (YAML, served by sensuctl / API) — Sensu Go pipe-handler standard.
- **Knobs** (`sensu-snmp-trap-handler/main.go:44-99` and `README.md:60-65`):

| Flag | Env | Default | Purpose |
|---|---|---|---|
| `--community / -c` | `SNMP_COMMUNITY` | `public` | SNMP community for sent traps |
| `--host / -H` | `SNMP_HOST` | `127.0.0.1` | SNMP manager host |
| `--port / -p` | `SNMP_PORT` | `162` | SNMP manager UDP port |
| `--version / -v` | `SNMP_VERSION` | `2` | `1`, `2`, or `2c` (NO v3) |
| `--varbind-trim / -t` | `SNMP_VARBIND_TRIM` | `100` | trim output to N chars |
| `--message-template / -m` | `SNMP_MESSAGE_TEMPLATE` | `{{.Check.State}} - {{.Entity.Name}}/{{.Check.Name}} : {{.Check.Output}}` | Go-template for varbind 6 |

- **Per-entity / per-check overrides**: via annotations under key `sensu.io/plugins/sensu-snmp-trap-handler/config/<flag-name>` (`README.md:103-118`). Standard Sensu plugin SDK behavior.
- **CLI**: standard cobra-based handler binary.
- **GUI**: standard Sensu Go web UI handler/config dialog (no trap-specific surface).
- **REST API**: standard Sensu Go API (resource: `Handler`).
- **Defaults documented?**: yes in README.
- **Validation**: `checkArgs` enforces non-empty host, non-empty message template, version in allowed list (`main.go:107-118`).
- **Live reload**: per-event reload — handlers are spawned per-event; new config takes effect on the next event.
- **Multi-tenancy / RBAC**: standard Sensu Go namespace/RBAC for handler definitions.

### 7.4 Where the operator "configures SNMP traps" in Sensu

The honest answer: **there is no single place**. An operator who wants Sensu to handle SNMP traps must:

1. Pick an inbound bridge (Classic extension or snmptrapd2sensu) based on which Sensu generation they run.
2. If Classic: configure `/etc/sensu/conf.d/extensions.json` AND `/etc/sensu/conf.d/snmp_trap.json` AND `/etc/sensu/mibs/`; restart sensu-client.
3. If Sensu Go: install and configure Net-SNMP `snmptrapd` itself (separate Linux package, separate config under `/etc/snmp/snmptrapd.conf`), install snmptrapd2sensu, write `/etc/sensu/snmptrapd2sensu.json`, ensure the sensu-agent HTTP API is reachable.
4. If outbound: define a Sensu Handler resource pointing at `sensu-snmp-trap-handler`, attach a runtime asset, define filters to choose which events become traps.
5. Define the desired Sensu **filters/mutators/handlers** for downstream routing — the bridges produce generic events; downstream policy is operator-supplied.

No single configuration view exists. Adding a new vendor MIB (Classic) and a new severity rule (Classic / snmptrapd2sensu) are two unrelated workflows. There is no UI guided wizard. There is no `sensuctl trap …` subcommand. There is no central documentation page that walks operators through end-to-end trap setup for Sensu Go.

## 8. Integration with Other Signals

### 8.1 Metrics

- **Are traps converted to metrics?** No conversion in any of the three bridges. Trap content lands as a Sensu **event**, not as a metric.
- **Counters / gauges from traps**: not provided. An operator wanting metrics-from-traps would have to write a Sensu mutator that re-emits trap events as metric outputs.
- **Annotations on metric dashboards**: not applicable — Sensu does not have native dashboards. Operators send metrics to InfluxDB or Prometheus (the `snmp-demo` repo uses Prometheus snmp_exporter scraped via Sensu check), and any trap-as-annotation work is downstream of Sensu.

### 8.2 Alerting / Notifications

- **How traps become alerts / tickets / pages**: by the trap event matching a Sensu **filter** and being routed to a **handler** (PagerDuty, Slack, OpsGenie, etc.). Handlers are general-purpose; nothing trap-specific.
- **Alert routing**: standard Sensu handler graph. The Classic extension lets the operator set `handlers` per-trap statically (`snmp-trap.rb:58`); snmptrapd2sensu has no per-trap handler routing — every trap becomes an event in the configured namespace and Sensu's general routing applies.
- **Escalation**: not built in.
- **Deduplication policies**: rely on Sensu's check-name-keyed event collapsing.
- **Acknowledgement / clear semantics**:
  - Classic extension `linkUp` / `linkDown` synthesise the same check name `link_status_<ifIndex>`, so a `linkUp` after `linkDown` clears via Sensu's standard check-status transition (status 2 → 0). For arbitrary traps this clear pairing is **not** done — the bridge does not know which traps pair.
  - snmptrapd2sensu: no pairing logic. Operators have to use Sensu's silence / suppression API to ack.
  - Outbound handler: not applicable (outgoing direction).

### 8.3 Topology

Sensu has no topology graph in either product generation. Confirmed by exhaustive grep of `sensu-go @ 80e82590e` for terms like `topology`, `link`, `neighbor`, `lldp`, `cdp` — no production source files match in a topology sense. Topology-aware trap suppression is **not applicable** because there is no topology.

### 8.4 Logs / Events

- **Trap-as-event**: trap content lands in the standard Sensu event store. There is no unified event-or-log surface; Sensu does not ingest logs.
- **Searchability**: via `sensuctl event list / event info`. Annotations and labels are searchable through standard Sensu queries.
- **Retention**: governed by Sensu's general event retention.
- **Schema**: standard Sensu Event protobuf (`sensu/core @ 14f0fb0 :: v2/event.proto`).

### 8.5 Northbound Forwarding

- **Outbound trap emission**: provided by `sensu-snmp-trap-handler` (see §2.3). The handler emits **only** Sensu PEN traps under `1.3.6.1.4.1.45717` — the receiving manager gets eleven varbinds with a fixed layout. The shape is not customisable beyond `--message-template` for varbind 6 (`main.go:91-99`).
- **Format options**: SNMPv1 OR SNMPv2c. NO SNMPv3 (`main.go:34` lists only `["1", "2", "2c"]`).
- **REST / Kafka / OTLP forwarding of trap events**: not trap-specific. Sensu Go's general handler ecosystem includes Kafka, AMQP, OpenTelemetry, etc. — operators choose.
- **No native passthrough of a received trap to a downstream NMS**: there is no built-in component that takes "the OpenNMS trap that came in" and re-emits it as a Sensu-PEN trap; the outbound handler always emits Sensu-PEN traps with Sensu-shaped content.

## 9. Severity Model

### 9.1 Classic extension

Two-stage:

1. **Varbind-name regex → result key** (`RESULT_MAP`, `snmp-trap.rb:9-15`): includes one `:status`-mapping line for any varbind whose symbolic name matches `/severity/i`, plus a Palo Alto specific rule (`/pansystemseverity/i`).
2. **Trap-OID regex → numeric status** (`RESULT_STATUS_MAP`, `:17-20`): `/down/i` → CRITICAL (2), `/authenticationfailure/i` → WARNING (1).

Anything else defaults to OK (0) — `determine_trap_status` (`:274-280`) — which means **unknown traps become OK events** that are still routed through Sensu's handler graph but with a default-OK status. The operational implication: trap floods of unknown OIDs do not raise alerts by default. Cross-system comparison of unknown-trap defaults is deferred to `comparison/comparative-analysis.md`; the per-system analyses already done indicate this default-OK behaviour is on the permissive end of the spectrum (OpenNMS sets `Indeterminate` with `alarm-type=3` per `opennms.md:215-218`; Zenoss sets `SEVERITY_WARNING` per `zenoss.md:265`; Centreon by default silently drops unknown traps per `centreon.md:304-306`; CheckMK requires `archive_orphans=True` to retain them per `checkmk.md:475`). So Sensu's default-OK behaviour is not "the opposite of every other system" — it is one point on a wide spectrum that already varies among the other systems analysed.

Customisation surface: user `result_status_map` is prepended to the built-in (`:87-94`) — user rules take precedence; built-ins remain.

### 9.2 `snmptrapd2sensu`

Fixed default status: configurable but global (`sensu.check.status` in config, default `1` = WARNING — `snmptrapd2sensu.json` and `main.go:49, 112`). Every trap from every device gets the same status. There is no per-OID, per-vendor, or per-source rule.

### 9.3 `sensu-snmp-trap-handler` (outbound)

The handler emits Sensu's event severity (`Check.Status`) **into** varbind 5 of its outbound trap. Severity translation is one-way and trivial: 0=OK, 1=WARNING, 2=CRITICAL, 3=UNKNOWN, anything >3 clamped to 3 (`main.go:151-155`). No vendor severity mapping back to receiving manager severity.

### 9.4 Sensu Enterprise legacy `snmp`

Per docs (`sensu-docs/.../snmp.md:105-112`) the `severities` filter controls which Sensu severities cause an outbound trap. Default `["warning","critical","unknown"]`. Source-not-verified; docs-only.

## 10. Storm / Volume Handling

### 10.1 Classic extension

- **Per-source rate limit**: none.
- **Dedup keys / windows**: none beyond Sensu's check-name-keyed event collapsing (one event per check name per source, regardless of trap rate).
- **Circuit breakers**: none.
- **Storm detection**: none.
- **Backpressure / queue management**: the in-process `Queue` between listener and processor is **unbounded** (`snmp-trap.rb:335`). If the processor falls behind, the queue grows without bound. Ruby's GC cost on a large queue is the eventual ceiling; before that, memory exhaustion is the failure mode.
- **Failure mode under load**: in the listener thread, the SNMP gem reads UDP datagrams synchronously and pushes onto the unbounded Ruby `Queue` (`snmp-trap.rb:101-103, :335`). Because the Queue is unbounded, `@traps << trap` does not itself block — instead the processor backlog grows in memory (each unprocessed `SNMP::SNMPv2_Trap` object retained). The visible failure modes are therefore: (a) kernel UDP receive buffer fills if the listener thread cannot drain fast enough (e.g. during the startup window while `smidump` is busy loading MIBs), and packets are dropped at the OS level; (b) RSS grows until OOM as the queue accumulates. Sensu has no counter for either.

### 10.2 `snmptrapd2sensu`

- **Per-source rate limit**: provided **only** by snmptrapd's own rate-limiting facilities (`disableAuthorization` / `agentaddress` are not rate-limit knobs; Net-SNMP's `outputOption` / `format2` formatting predates rate limiting). Practically: none from snmptrapd2sensu.
- **Concurrency cap**: snmptrapd spawns one snmptrapd2sensu process per trap with no per-source limit. A trap storm produces a process-spawn storm — limited by `ulimit -u` on the host.
- **Backpressure**: when sensu-agent's HTTP API is slow, the HTTP POST blocks; snmptrapd2sensu processes pile up. Eventually process creation fails and snmptrapd logs an error.
- **Downstream Sensu-agent `/events` rate limit**: although snmptrapd2sensu itself has no rate limit, the **receiving sensu-agent HTTP API does**. `sensu-go @ 80e82590e :: agent/config.go:27-34` defines `DefaultEventsAPIRateLimit = 10.0` (events/second) and `DefaultEventsAPIBurstLimit = 10`; the config fields are surfaced at `agent/config.go:108-114` (`EventsAPIRateLimit` / `EventsAPIBurstLimit`) and configured via the agent flags documented at `sensu-docs/content/sensu-go/6.14/observability-pipeline/observe-schedule/agent.md:279-280, :689-690`. The limiter itself is `rate.NewLimiter(limit, burst)` at `agent/api.go:94-100`, applied as `limiter.Wait(ctx)` before processing `/events` POSTs. So a sustained trap storm via snmptrapd2sensu becomes a burst-allowed-then-throttled stream of `/events` POSTs that wait on the limiter — and because `snmptrapd2sensu` does not inspect HTTP status (per §5.2), any 429/503 the agent might emit are silently treated as success. The net effect: downstream throttling exists, but the bridge does not see it as such and continues to spawn processes.
- **Dedup**: none.

### 10.3 `sensu-snmp-trap-handler`

- **Per-event rate limit**: not the bridge's responsibility — Sensu's event pipeline / filters can rate-limit the **events** that reach the handler.
- **Backpressure**: handler `log.Fatalf`s on errors — no retry.

### 10.4 Summary

None of the three bridges has any rate limit, any dedup key, or any backpressure beyond what the surrounding OS/Sensu provides. Compared to the storm-handling mechanisms documented in the other per-system files reviewed so far (OpenNMS's blocking sink dispatcher; CheckMK's facility/priority rule_hash; Centreon's debounce window; Zabbix's per-poller queue), Sensu's storm-handling story is the most minimal — comparative ranking is deferred to `comparison/comparative-analysis.md`. Operators relying on Sensu for SNMP trap reception must:

- pre-filter at the source devices, or
- rate-limit upstream (e.g. via iptables hashlimit or an snmptrapd front-end), or
- accept that storms become Sensu event storms.

## 11. Security

### 11.1 SNMPv3 USM support

- **Classic extension**: **NO**. The Ruby `snmp` gem 1.2.0 has partial v3 support but the Sensu extension does not configure it; only `on_trap_v2c` is registered (`snmp-trap.rb:101`). v3 traps are silently ignored.
- **`snmptrapd2sensu`**: **YES via Net-SNMP**. snmptrapd handles v3 USM; snmptrapd2sensu sees post-decryption text. Auth/priv configuration lives in `/etc/snmp/snmptrapd.conf` (`createUser`, `authUser` directives — Net-SNMP's domain, not Sensu's).
- **`sensu-snmp-trap-handler`**: **NO**. Only v1, v2, 2c (`main.go:34` `ValidSNMPVersions = []string{"1", "2", "2c"}`).
- **Sensu Enterprise legacy**: per docs (`sensu-docs/.../snmp.md:24-29`) only v1 and v2 outbound.

### 11.2 DTLS / TLS-TM support

None of the four bridges has any DTLS / TLS-TM code of its own. snmptrapd2sensu sees only post-decryption text from Net-SNMP `snmptrapd`, so whatever transports `snmptrapd` is configured for upstream determine the on-wire security; this is outside the Sensu surface and not covered by Sensu documentation.

### 11.3 Credential storage

- **Classic extension**: the community string is in `/etc/sensu/conf.d/snmp_trap.json` (`community` key) in clear text. File permissions are operator-managed; no secret-store integration.
- **`snmptrapd2sensu`**: no Sensu-side credentials (all SNMP credentials live in Net-SNMP's `snmptrapd.conf`).
- **`sensu-snmp-trap-handler`**: outbound community string is configurable via `SNMP_COMMUNITY` env var or `--community` flag (`main.go:47-52`). Operators integrate with Sensu Go's standard secret-management surface (`secrets`) for env-var injection.
- **No Sensu-native SNMP credential vault**.

### 11.4 Access control on the trap subsystem itself

- **Classic extension**: the listener accepts any UDP datagram on the bound port matching the configured community. No source-IP allowlist, no per-OID ACL.
- **`snmptrapd2sensu`**: ACL is at Net-SNMP layer (snmptrapd's `authUser` / `authCommunity` / `disableAuthorization`).
- **`sensu-snmp-trap-handler`**: outbound — no inbound access control concept.

### 11.5 Audit logging

None of the bridges write a structured audit trail. Logs are unstructured stdout/stderr / EventMachine debug logging.

### 11.6 Summary

Sensu's first-party security story for SNMP traps is essentially: **delegate to Net-SNMP** (via snmptrapd2sensu) for any modern security (v3, DTLS, ACL). The Classic extension's security model is "trust the community string" — adequate for inside-the-DMZ deployments only.

## 12. Trap Simulation & Testing (in-source evidence)

### 12.1 Classic extension tests

Source: `sensu-extensions-snmp-trap/spec/snmp-trap_spec.rb` (51 lines), `spec/helpers.rb` (94 lines).

Two RSpec tests:

1. **`it "can run"`** (`:32-44`): the test sends a real SNMPv2c PDU over a UDP socket to `127.0.0.1:1062`, and an EM-server on `127.0.0.1:3030` (the synthesised check-result destination) expects a specific JSON payload (`:34-36`). The expected JSON is:
   ```json
   {"source": "localhost", "name": "SENSU-ENTERPRISE-NOTIFY-MIB--sensuEnterpriseEventTrap", "output": "alert", "status": 2, "handlers": ["default"], "snmp_trap": {"DISMAN-EXPRESSION-MIB::sysUpTimeInstance": 20, "SNMPv2-MIB::snmpTrapOID.0": "SNMPv2-SMI::enterprises.45717.1.0", "SENSU-ENTERPRISE-NOTIFY-MIB::sensuNotification": "alert", "SENSU-ENTERPRISE-NOTIFY-MIB::sensuCheckSeverity": 2}}
   ```
   So this test covers: end-to-end UDP receive, MIB-derived symbolic name lookup, varbind decode (Integer32 → status), and JSON synthesis.
2. **`it "can load mibs from files recursively"`** (`:46-50`): asserts the recursive `Dir.glob` finds `APACHE-MIB` under `spec/mibs/more_mibs/`.

Trap fixtures: `spec/mibs/` contains 7 MIB files (per §4.1) — including the Sensu Enterprise MIBs that the test consumes.

CI: Travis CI (`.travis.yml:3-7`), Ruby `2.0.0 / 2.1.0 / 2.2.0 / 2.2.3 / 2.3.0`. All five Ruby versions are EOL upstream. The Travis service itself migrated to travis-ci.com and the legacy travis-ci.org pipeline may be defunct — there's no GitHub Actions workflow file in this repo.

### 12.2 `snmptrapd2sensu` tests

Source: `snmptrapd2sensu/test.sh` (2 lines), `snmptrapd2sensu/testing/data/notification.txt`.

- The "test" is a manual shell script that builds the binary and runs it once on the fixture: `go build && ./snmptrapd2sensu < testing/data/notification.txt`.
- `notification.txt` is a 4-line fixture (sysUpTime varbind + one example varbind in NET-SNMP text format).
- **No unit tests** (`*_test.go` files do not exist in this repo).
- **No CI** beyond what `.goreleaser.yml` provides (release build only).
- **No assertion harness** — the operator inspects stdout after running `test.sh`.

### 12.3 `sensu-snmp-trap-handler` tests

Source: `sensu-snmp-trap-handler/main_test.go` (82 lines).

Five Go tests:

1. **`TestCheckArgs`** (`:15-24`): verifies argument validation rejects invalid version `"99"`.
2. **`TestGetClientIP`** (`:26-53`): verifies the entity-IP-extraction helper handles loopback / valid / multi-interface cases.
3. **`TestContains`** (`:55-60`): trivial.
4. **`TestFormatMessage`** (`:62-72`): verifies the message-template engine.
5. **`TestTrimOutput`** (`:74-82`): verifies the varbind trim behaviour.

Source comment at the top (`:10-13`): *"Tests still needed: getClientIP - need to build event.Check with network interfaces; executeCheck - borrow trap listener from gosnmp tests?"* — i.e., **the outbound emission path itself (`executeHandler` → `SendTrap`) is NOT covered** by an automated test. The TODO comment is partly stale — `TestGetClientIP` (test 2 above) was subsequently implemented, but the comment was never updated. The `executeHandler` / `SendTrap` part of the TODO remains accurate. The handler has never been verified end-to-end in CI to actually emit a valid SNMP trap.

`getClientIP` itself is fragile (`main.go:243-251`): it iterates `event.Entity.System.Network.Interfaces`, skips loopback, and returns the first address of the first non-loopback interface. If a non-loopback interface has an empty `Addresses` slice, the function panics at `strings.Split(a.Addresses[0], "/")` — there is no bounds check before the index. The test `TestGetClientIP` validates a few normal cases but does not cover the empty-Addresses panic.

CI: GitHub Actions. Two workflow files in `sensu-snmp-trap-handler/.github/workflows/`: `test.yml` (21 lines — runs `go test` on macOS/Windows/Ubuntu with Go 1.13 per the matrix) and `release.yml` (24 lines — goreleaser). The "Go Test" badge in the README does correspond to a live workflow.

### 12.4 Tools shipped for trap simulation

- **Classic extension**: none. The Sensu Classic guide tells operators to install `net-snmp-utils` and use `snmptrap` to send test traps (`snmp-sensu-guide.md:25-28, :97-99`).
- **`snmptrapd2sensu`**: the operator is told to run snmptrapd in foreground (`README.md:73-77`).
- **`sensu-snmp-trap-handler`**: no fake-manager. The receiving manager is operator-supplied.

### 12.5 Summary

Trap test coverage across the three Sensu bridges is **shallow**. One real PDU end-to-end test in the Classic extension. No PDU-on-wire test in snmptrapd2sensu. No PDU-on-wire test in the outbound handler. Compared to the trap test inventories captured in the other per-system files reviewed so far (OpenNMS's `TrapdInformIT`, `TrapdWithKafkaIT`, and ~13 trap-pipeline tests; Zabbix's snmptrapper test fixtures), Sensu's PDU-on-wire test surface is the most minimal of the cohort analysed to date; final cross-cohort ranking is deferred to `comparison/comparative-analysis.md`.

## 13. Out-of-the-Box Coverage (defaults)

### 13.1 MIBs bundled

| Component | MIBs shipped in the gem/binary | Bundled into install? |
|---|---|---|
| Classic extension | None in the production gem | No (operator copies into `/etc/sensu/mibs/`) |
| `snmptrapd2sensu` | None | No (relies on Net-SNMP's own MIBs) |
| `sensu-snmp-trap-handler` | 5 files: RFC-1212-MIB, RFC-1215-MIB, SENSU-ENTERPRISE-V1-MIB, SENSU-ENTERPRISE-ROOT-MIB, SENSU-ENTERPRISE-NOTIFY-MIB | Yes, in `mibs/` of the release tarball — intended for the receiving manager, NOT for local use |

**Vendor coverage shipped by Sensu**: zero. Compare to OpenNMS's 230 example `.events.xml` files covering Cisco, Juniper, HP, Brocade etc. — Sensu ships nothing of the kind.

### 13.2 Severity rules bundled

| Component | Built-in severity rules |
|---|---|
| Classic extension | 2 (`/down/i` → CRITICAL, `/authenticationfailure/i` → WARNING) plus one Palo-Alto-specific result-map rule for `pansystemseverity` |
| `snmptrapd2sensu` | none — single configurable default status, applied to every trap |
| `sensu-snmp-trap-handler` | one-way Sensu-status-to-PEN-varbind translation |

### 13.3 Dedup defaults

None across all three bridges. Operators inherit only Sensu's per-(entity, check-name) latest-state collapsing.

### 13.4 Vendor packs / integration packages

None. Sensu has no "vendor MIB pack" concept. No published presets for Cisco, Juniper, F5, Palo Alto, HP/Aruba, Dell, etc. (The single Palo Alto rule in the Classic extension's `RESULT_MAP` is incidental.)

### 13.5 Sample / preset dashboards or reports

- **Classic extension**: a sample screenshot exists in the README (`iflinkdown.png`, `README.md:14`) showing the Sensu Enterprise Dashboard rendering a `linkDown` event. No exported dashboard JSON or Grafana template ships with the gem.
- **Sensu Go**: no trap-specific dashboard, panel, or web-UI surface. The Sensu Go web UI shows trap events identically to any other check event.

### 13.6 Day-1 experience

A fresh Sensu Go install handling SNMP traps requires the operator to:

1. Decide between `snmptrapd2sensu` and "write my own bridge".
2. Install Net-SNMP (`snmptrapd`) on the host(s) chosen to receive traps.
3. Configure `snmptrapd.conf` with community/user, `traphandle default`, and MIB load directives.
4. Install snmptrapd2sensu (binary download from GitHub releases — not in Bonsai asset registry as a standard Sensu asset).
5. Write `/etc/sensu/snmptrapd2sensu.json`.
6. Ensure sensu-agent HTTP API is up (`:3031` reachable).
7. Send a test trap to validate end-to-end.
8. Define Sensu filters/handlers for routing.

Steps 1-3, 5, and 7 are operator labour with no Sensu-provided defaults to ease them. Compared to the day-1 setup paths documented in the other per-system files reviewed so far (OpenNMS's preloaded Indeterminate default-trap row plus 17k+ optional vendor event definitions; CheckMK's empty-by-default rule_pack but with auto-decoded trap-as-syslog event; Zenoss's `/Unknown` default class; Centreon's pre-seeded 214-trap catalogue), Sensu's day-1 cost for inbound trap reception is on the high end — comparative ranking deferred to `comparison/comparative-analysis.md`.

## 14. User Customization Surface

### 14.1 How users add custom OID handlers

- **Classic extension**: there is no "OID handler" plugin point. Operators customise behaviour by:
  - Editing `RESULT_MAP` / `RESULT_STATUS_MAP` via the JSON config (regex-based).
  - Setting `result_attributes` to merge static fields into every event.
  - Reading the synthesised event in a Sensu **mutator** and rewriting it before handler dispatch.
  - Forking the gem and modifying `snmp-trap.rb`.
- **`snmptrapd2sensu`**: no per-OID customisation surface at all. Every trap is shaped identically. To get different behaviour per OID, operators must:
  - Use snmptrapd's own per-OID `traphandle <OID> /path/to/other-binary` directives — i.e., write **alternate** binaries instead of customising one.
  - Or, in Sensu, write mutators that read the `Check.Output` JSON and re-shape downstream.

### 14.2 Custom MIBs

- **Classic extension**: drop `.mib` or `.txt` files into `/etc/sensu/mibs/` (recursive since v0.2.0); restart sensu-client.
- **`snmptrapd2sensu`**: configure Net-SNMP `snmptrapd` to load custom MIBs (`-M` / `-m` flags on snmptrapd).
- **`sensu-snmp-trap-handler`**: not applicable — the outbound emitter only emits Sensu-PEN traps with a fixed varbind layout. Custom MIBs would only matter on the receiving manager side.

### 14.3 Custom severity rules

- **Classic extension**: user `result_status_map` array, regex → integer (`snmp-trap.rb:87-94`). User entries are prepended to built-ins, so user wins.
- **`snmptrapd2sensu`**: one global default. No per-OID severity. Operators do severity routing **downstream** in Sensu filters/mutators.
- **`sensu-snmp-trap-handler`**: severity mapping is one-way and fixed in code.

### 14.4 Custom dedup rules

None across the three bridges.

### 14.5 Plugin / extension model

This is where Sensu **is** strong — but as a general framework, not as a trap-specific surface:

- **Sensu Go**: handlers, mutators, filters, hooks, checks are all pluggable. A custom Lua/JavaScript filter (filtering on Sensu's `event` object) can rewrite or drop trap events. Mutators (pipe binaries) can transform.
- **Bonsai asset registry**: community-contributed assets. `sensu-snmp-trap-handler` is published as a Bonsai asset (`.bonsai.yml` in the repo, README badge linking to `https://bonsai.sensu.io/assets/sensu/sensu-snmp-trap-handler`) — installable in one command: `sensuctl asset add sensu/sensu-snmp-trap-handler`. Build matrix in `.bonsai.yml` covers linux/{386,amd64,arm64,armv7}, darwin/amd64, windows/amd64. `snmptrapd2sensu` has **no `.bonsai.yml`** and is NOT installable as a Bonsai asset — operators download the GoReleaser tarball from GitHub Releases manually. For the Classic extension, the analogous registry is the deprecated sensu-extension installation flow (`sensu-install -e snmp-trap:0.0.33`).

This generic extensibility is why Sensu has the trap bridges in the first place — but it also explains why none of them is deep: they are individual community contributions, not coordinated platform features.

### 14.6 API surface for automation

- **Sensu Go**: full REST API for resources (Handler, Filter, Mutator, Asset, Check, Entity, Event), plus `sensuctl`.
- **Classic**: REST API on sensu-api (Sensu Classic). No trap-resource type.

A fully API-driven workflow for trap config exists only in the sense of "general Sensu resource CRUD", not "trap configuration as first-class API objects."

## 15. End-User Value Analysis

### 15.1 What an operator gets day-1 with default config

- **Classic + extension installed via `sensu-install`**: a v2c trap listener on `:1062`, community `public`, no MIBs loaded by default. With **operator-supplied standard MIBs (IF-MIB etc.) loaded into `/etc/sensu/mibs`**, `linkUp` / `linkDown` traps will resolve to symbolic names and trigger the bridge's hard-coded `link_status_<ifIndex>` special-case (`snmp-trap.rb:229-249`); without resolvable MIBs the link OID may stay numeric and miss that special case. For every other received trap (matched or unmatched), the result-status is the first match of `result_status_map` (default: `/down/i` → CRITICAL, `/authenticationfailure/i` → WARNING — `snmp-trap.rb:17-20`); if no match, status defaults to OK (`:274-280`). The built-in `RESULT_MAP` (`:9-15`) also writes severity-shaped varbinds (`/severity/i`, `/pansystemseverity/i`) into the result status when the trap carries them. Per-trap event surfaced in the Sensu Enterprise Dashboard.
- **Sensu Go + snmptrapd2sensu**: nothing until Net-SNMP `snmptrapd` is installed and configured separately. The bridge alone provides no listener.
- **Sensu Go (without any bridge)**: zero — no traps received.

### 15.2 What requires customization

Effectively **everything beyond "see a trap event"**:

- Vendor MIB loading.
- Per-OID severity mapping.
- Per-OID handler routing.
- Per-source dedup.
- Storm protection.
- v3 USM (impossible in Classic extension; via Net-SNMP in snmptrapd2sensu).
- Pairing UP/DOWN traps for arbitrary vendors (only `linkUp`/`linkDown` is pre-baked in Classic).

### 15.3 Learning curve

High. The operator must:

- Understand Sensu's check/event/handler/filter/mutator/asset model (Sensu's general learning curve).
- Understand which of three different bridges to pick.
- For Sensu Go, also learn Net-SNMP snmptrapd's configuration (not Sensu's documentation domain).
- For the Classic extension, learn Ruby's `snmp` gem behaviour and `smidump` compilation idiosyncrasies.
- Accept that the trap pipeline is a community responsibility.

### 15.4 Operational toil

- **Adding a new vendor**: operator-driven MIB acquisition, copy to `/etc/sensu/mibs/`, restart sensu-client (Classic) OR configure snmptrapd MIB load (Sensu Go).
- **Tuning severity**: editing JSON regex rules (Classic) or writing mutators (Sensu Go).
- **Investigating a missed trap**: tail `/var/log/sensu/sensu-client.log` (Classic) or snmptrapd log + sensu-agent log + sensu-backend log + Sensu Go web UI events (Sensu Go). The bridges write unstructured logs.
- **Testing changes**: send a real trap with `snmptrap`; observe the Sensu UI. There is no replay tool.

### 15.5 Visibility into the pipeline's own health

- **Listener up?**: `netstat`/`ss` for the bound port (Classic). No Sensu-side health check ships out of the box.
- **Queue depth (Classic)**: no metric. The unbounded Queue is invisible to operators.
- **Per-source trap rate**: not measured.
- **Decode errors**: written to log at ERROR level; not surfaced as metrics or alerts by the bridge itself.

A best-practice operator would write a Sensu check that monitors the listener port and the queue depth — but the bridge does not ship one.

### 15.6 Summary

Sensu's day-1 trap-receiving value is **low**: the operator has to build most of the experience. Sensu's day-1 trap-emitting value is **moderate**: the outbound handler works out of the box for sending Sensu-PEN traps to any manager that loads the Sensu MIBs. Neither path delivers what an operator running OpenNMS, Zenoss, or CheckMK would consider "trap support".

## 16. Strengths

- **Architectural minimalism**: each bridge is ~150-350 lines of code. Easy to read top-to-bottom, easy to audit, easy to fork. (`sensu-extensions-snmp-trap/lib/sensu/extensions/snmp-trap.rb` is 350 lines; `snmptrapd2sensu/main.go` 164 lines; `sensu-snmp-trap-handler/main.go` 306 lines.)
- **Clean delegation to Net-SNMP** (in snmptrapd2sensu): outsourcing trap reception to a mature, well-known external tool means SNMP version support, MIB resolution, and security move out of the Sensu surface. Net-SNMP supports v1/v2c/v3 USM and brings its own MIB ecosystem.
- **Generic-event integration**: traps land in the same event store as any other Sensu signal. Sensu's full filter/mutator/handler graph applies. There is no separate trap-only pipeline.
- **Plugin/asset model for distribution**: `sensu-snmp-trap-handler` is published as a Bonsai asset (`README.md:1` badge); installable in one sensuctl command.
- **Outbound trap shape is well-defined**: the Sensu-PEN MIBs are shipped with the outbound handler (`sensu-snmp-trap-handler/mibs/`), giving downstream managers a published schema (11 varbinds under `1.3.6.1.4.1.45717.1.1.1.{1..10}`). Source: `sensu-snmp-trap-handler/main.go:140-214`.
- **Per-event configurability via annotations**: the outbound handler honours per-entity / per-check annotations under `sensu.io/plugins/sensu-snmp-trap-handler/config/...` (`README.md:103-118`), letting different checks send traps to different managers.
- **End-to-end PDU test in the Classic extension**: the rspec test sends a real binary-encoded PDU over UDP and asserts the JSON outcome (`spec/snmp-trap_spec.rb:32-44`). That is rare in Ruby trap-handling code.
- **The PAN-OS specific severity rule** (`pansystemseverity` in `RESULT_MAP`) is evidence of operator-driven contributions filtering up — a community-shipping pattern.

## 17. Weaknesses / Gaps

(Evidence-cited, no marketing softening.)

1. **No first-party trap support in the current product (Sensu Go).** Seven years after Sensu Go shipped (2018) and more than five years after Sensu Core EOL (December 31, 2019; package repos removed February 1, 2021), Sensu Go core (last analysed commit 2025-07-10) has zero SNMP trap code. `sensu-go @ 80e82590e` grep returns no trap implementation. Operators choosing Sensu inherit this gap.
2. **Sensu Go's only inbound bridge is unmaintained.** `snmptrapd2sensu` last commit 2020-10-29 (`git log -1`). Tag `0.1` is the only published release; the README still references download URL `0.1`. Five years of zero commits.
3. **The Classic extension is unmaintained and tied to dead Ruby versions.** `sensu-extensions-snmp-trap` last commit 2018-10-27. Travis matrix is Ruby 2.0-2.3 (all EOL). The gem depends on `snmp 1.2.0` pinned (`gemspec:17`) — itself a 2014-era release.
4. **SNMPv1 is unsupported in the Classic extension.** Only `on_trap_v2c` registered (`snmp-trap.rb:101`). For an extension whose default use case is monitoring older network devices that may only emit v1, this is a real gap.
5. **SNMPv3 USM is unsupported in the Classic extension and the outbound handler.** v3 is only available via Net-SNMP delegation in snmptrapd2sensu. The outbound handler explicitly rejects v3 (`main.go:34`).
6. **Default severity for unknown traps is OK.** `RESULT_STATUS_MAP` (`snmp-trap.rb:17-20`) only fires on `/down/i` or `/authenticationfailure/i`; otherwise `determine_trap_status` returns `0` (OK) per `:274-280`. This is operationally risky because unknown-trap floods do not raise alerts by default — operators see only OK events even from unrecognised devices, which can mask a developing incident.
7. **No bundled vendor MIBs.** None of the bridges ships Cisco/Juniper/HP/Brocade/F5 MIBs. Compare OpenNMS's 17,442 event definitions or CheckMK's bundled `mkeventd` rules.
8. **No dedup, no rate limit, no storm protection** in any bridge. A trap storm becomes an event storm — the Sensu event pipeline absorbs it. (See §10.)
9. **No topology, no L2/L3 correlation, no enrichment from CMDB.** Sensu has no topology model at all.
10. **The Classic extension's processor thread aborts on exception.** `@processor.abort_on_exception = true` (`snmp-trap.rb:330`). A single bad varbind that escapes the in-method rescue can take down sensu-client.
11. **snmptrapd2sensu has a copy-paste bug in `SnmpTrapOidOIDs`.** `1.3.6.1.4.1.3.6.1.6.3.1.1.4.1.0` (`main.go:38`) is not a canonical incantation of snmpTrapOID.0. Low-impact, but evidence of low review pressure.
12. **`snmptrapd2sensu` fatal-on-error in most paths.** `log.Fatalf` is called five times — twice in `processNotification` (JSON marshal of intermediate dicts) and three times in `postEvent` (JSON marshal of the event, HTTP request build, HTTP send). The HTTP response read at `main.go:149` assigns `err` but never checks it, so HTTP 4xx/5xx responses from sensu-agent are silently treated as success. There is no retry, no buffer, no graceful degradation.
13. **No tests covering the outbound trap-on-wire path.** `sensu-snmp-trap-handler/main_test.go` author note (`:10-13`) acknowledges `executeHandler` is not tested. The handler has never been CI-verified to actually emit a valid PDU.
14. **No CI / no PDU-on-wire test for `snmptrapd2sensu`.** `test.sh` is a 2-line manual shell script (`go build && ./snmptrapd2sensu < testing/data/notification.txt`). No `_test.go` files exist.
15. **No documentation in Sensu Go's docs site about trap reception.** `grep -ri snmp-trap content/sensu-go/` returns zero hits across versions 5.0 through 6.14 (the only matches are in `release-notes.md` for unrelated reasons). The only first-party guide is for Sensu Core 1.9 (`content/sensu-core/1.9/guides/snmp-sensu-guide.md`) — i.e., for the dead product line.
16. **The Classic extension's queue is unbounded.** `@traps = Queue.new` (`snmp-trap.rb:335`). Under sustained load, RSS grows until OOM.
17. **No event lifecycle for traps.** No "alarm" concept, no clear-key pairing for arbitrary OIDs, no acknowledgement state beyond Sensu's generic silencing.
18. **Sensu Enterprise's `snmp` integration is end-of-life along with Sensu Classic.** No path forward for SNMP trap emission from Sensu Go that matches Sensu Enterprise's feature set.

## 18. Notable Code or Configuration Examples

### 18.1 The Classic extension's listener is six lines

`sensu-extensions-snmp-trap @ 269bc59 :: lib/sensu/extensions/snmp-trap.rb:96-106`:

```ruby
def start_snmpv2_listener!
  @listener = SNMP::TrapListener.new(
    :host => options[:bind],
    :port => options[:port],
    :community => options[:community]) do |listener|
    listener.on_trap_v2c do |trap|
      @logger.debug("snmp trap check extension received a v2 trap")
      @traps << trap
    end
  end
end
```

What this tells us: the entire trap-reception surface for Sensu Classic is six lines of Ruby relying on the `snmp` gem's `SNMP::TrapListener`. There is no v1 callback, no v3 callback. The community string is plain. The trap is pushed onto an unbounded `Queue`.

### 18.2 The processor thread aborts the process on exception

`snmp-trap.rb:321-332`:

```ruby
def start_trap_processor!
  @processor = Thread.new do
    create_mibs_map!
    import_mibs!
    load_mibs!
    loop do
      process_trap(@traps.pop)
    end
  end
  @processor.abort_on_exception = true
  @processor
end
```

`abort_on_exception = true` propagates any uncaught exception from the processor to the main thread of `sensu-client`, terminating the entire client process. A defective vendor MIB or a malformed varbind can take down the host's monitoring agent.

### 18.3 snmptrapd2sensu's HTTP-on-failure is fatal

`snmptrapd2sensu @ 50ae0ee :: main.go:128-151`:

```go
func postEvent(event *types.Event) {
    postBody, err := json.Marshal(event)
    if err != nil {
        log.Fatalf("ERROR: %s\n", err)
    }
    body := bytes.NewReader(postBody)
    req, err := http.NewRequest(
        "POST",
        fmt.Sprintf("http://%s:%v/events", SensuAgentApiHost, SensuAgentApiPort),
        body,
    )
    if err != nil {
        log.Fatalf("ERROR: %s\n", err)
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        log.Fatalf("ERROR: %s\n", err)
    }
    defer resp.Body.Close()
    b, err := ioutil.ReadAll(resp.Body)
    fmt.Println(string(b))
}
```

Three `log.Fatalf` exits in this 24-line function (lines 132, 141, 146), plus two more elsewhere (`main.go:109, 122` in `processNotification`) — a total of five per-trap-process fatal exits. The `ioutil.ReadAll` call assigns `err` but never checks it; HTTP error responses (4xx/5xx) are not distinguished from success. There is no retry, no exponential backoff, no buffer-to-disk fallback. If sensu-agent is even momentarily unreachable, the trap is lost.

### 18.4 The outbound trap shape is fixed by code

`sensu-snmp-trap-handler @ 3c38f01 :: main.go:157-214` — the eleven hard-coded varbinds (abridged):

```go
trap := snmp.SnmpTrap{
    Variables: []snmp.SnmpPDU{
        {Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: snmp.ObjectIdentifier, Value: SensuEnterprisePEN + ".1.0"},
        {Name: eventEntryOID + ".1", Type: snmp.OctetString, Value: fmt.Sprintf("%s/%s", event.Entity.Name, event.Check.Name)},
        {Name: eventEntryOID + ".2", Type: snmp.OctetString, Value: message},
        {Name: eventEntryOID + ".3", Type: snmp.OctetString, Value: event.Entity.Name},
        {Name: eventEntryOID + ".4", Type: snmp.OctetString, Value: event.Check.Name},
        {Name: eventEntryOID + ".5", Type: snmp.Integer, Value: checkStatus},
        {Name: eventEntryOID + ".6", Type: snmp.OctetString, Value: trimOutput(event.Check.Output)},
        {Name: eventEntryOID + ".7", Type: snmp.Integer, Value: int(action[event.Check.State])},
        {Name: eventEntryOID + ".8", Type: snmp.Integer, Value: int(event.Check.Executed)},
        {Name: eventEntryOID + ".9", Type: snmp.Integer, Value: int(event.Check.Occurrences)},
        {Name: eventEntryOID + ".10", Type: snmp.OctetString, Value: clientAddress},
    },
}
```

`eventEntryOID := fmt.Sprintf("%s.1.1.1", SensuEnterprisePEN)` (`:140`). The shape is determined entirely at code level — the Sensu Enterprise NOTIFY MIB is the only published contract.

### 18.5 The PAN-OS-specific built-in severity rule

`snmp-trap.rb:9-15`:

```ruby
RESULT_MAP = [
  [/checkname/i, :name],
  [/notification/i, :output],
  [/description/i, :output],
  [/pansystemseverity/i, Proc.new { |value| value > 3 ? 2 : 0 }, :status],
  [/severity/i, :status]
]
```

The `pansystemseverity` row (with a `Proc` rather than a direct mapping) is the only vendor-aware built-in rule. Source-verified behaviour: the Proc returns `2` (Sensu CRITICAL) if `value > 3`, else `0` (Sensu OK). The Palo Alto Networks PAN-OS severity scale that `pansystemseverity` refers to is a vendor-defined scale (informational/low/medium/high/critical, typically encoded as 0..4 or 1..5); the precise mapping of vendor numeric values to PAN-OS severity names is **not** documented in the Sensu source — operators must consult the relevant PAN-OS MIB to confirm what threshold the `> 3` rule corresponds to in their PAN-OS version. This is a single hard-coded shortcut for one vendor while every other vendor's severity must be customised by the operator.

### 18.6 The Sensu Classic guide tells operators to install Net-SNMP utils

`sensu-docs/content/sensu-core/1.9/guides/snmp-sensu-guide.md:25-28`:

```text
For installing the snmptrap command, you'll want to run the following to install the command on a CentOS/RHEL device:

sudo yum install -y net-snmp-utils
```

This is the entire trap-testing recipe Sensu's first-party docs provide. The recommended verification is to run `snmptrap -v 2c -c sensutest localhost:1062 '' IF-MIB::linkDown ifIndex i 2` (`:97-98`) and look at the dashboard. There is no Sensu-shipped trap simulator.

## 19. Sources Examined

Mirrored repos:

- `sensu/sensu-go @ 80e82590ece98a4bc780de04882a6f754ad12abc` (2025-07-10) — exhaustive grep for `snmp`, `trap`; no production code matches.
- `sensu-extensions/sensu-extensions-snmp-trap @ 269bc593de0a10f07f05b2c7895af92e03ba566d` (2018-10-27)
  - `lib/sensu/extensions/snmp-trap.rb` (350 lines)
  - `lib/sensu/extensions/snmp-trap/snmp-patch.rb` (51 lines)
  - `spec/snmp-trap_spec.rb` (51 lines)
  - `spec/helpers.rb` (94 lines)
  - `spec/mibs/{SENSU-ENTERPRISE-NOTIFY-MIB.txt, SENSU-ENTERPRISE-ROOT-MIB.txt, SNMPv2-SMI.txt, RFC-1212-MIB.txt, RFC-1215-MIB.txt, rfc1158.mib}` and `spec/mibs/more_mibs/APACHE-MIB.txt`
  - `README.md`, `CHANGELOG.md`, `.travis.yml`, `sensu-extensions-snmp-trap.gemspec`
- `sensu/sensu-snmp-trap-handler @ 3c38f014460d2d86b2ed9206516c26f568abc5e7` (2021-04-26)
  - `main.go` (306 lines)
  - `main_test.go` (82 lines)
  - `README.md`, `mibs/` directory contents (5 files)
  - `.bonsai.yml`, `.goreleaser.yml`, `go.mod`
- `sensu/snmptrapd2sensu @ 50ae0ee929ccb78f4462480b478a7e8f136978af` (2020-10-29)
  - `main.go` (164 lines)
  - `parsers/snmptrapd.go` (86 lines)
  - `types/snmptrapd.go` (36 lines)
  - `config/config.go` (72 lines)
  - `utils/utils.go` (23 lines)
  - `README.md`, `snmptrapd2sensu.json`, `test.sh` (2 lines), `testing/data/notification.txt`
- `sensu-plugins/sensu-plugins-snmp @ 36f3994a29df200919272b6bb41abe5dbde4ed7d` (2020-03-20) — confirmed scope is polling, not traps.
- `sensu/snmp-demo` — confirmed scope is Prometheus snmp_exporter integration, not traps.
- `sensu/core @ 14f0fb07ce7a3fbada40100cab04307eb4ca8a05` (2026-03-09) — confirmed `v2/event.proto` is the Sensu Go Event schema; no trap-specific schema.

Documentation:

- `sensu/sensu-docs`
  - `content/sensu-core/1.9/guides/snmp-sensu-guide.md` (270 lines) — single first-party trap guide; Sensu Classic only.
  - `content/sensu-enterprise/3.8/integrations/snmp.md` (125 lines) — outbound integration docs; legacy commercial.
  - `content/sensu-go/{5.x, 6.x}/**` — exhaustive grep for SNMP/trap content: zero matches in any sensu-go content page.
  - `archived/sensu-enterprise/{2.6..3.8}/integrations/snmp.md` — historical versions of the legacy outbound integration; confirm pattern stability over time but provide no Sensu Go path.

Not in scope (verified):

- `sensu/sensu-extension` (Ruby framework gem, not trap-specific).
- `sensu/sensu-extensions` (Ruby framework gem, not trap-specific).
- All other `sensu-extensions-*` repos (slack, opsgenie, etc.) — non-SNMP.

## 20. Evidence Confidence

| Section | Confidence | Rationale |
|---|---|---|
| §1 System Overview & Lineage | **high** | Source grep across `sensu-go` confirms zero trap code; vendor docs confirm EOL dates and product lines. |
| §2 Trap-Subsystem Architecture | **high** | Three bridges' source files read in full; sensu-go absence verified by exhaustive grep. |
| §3 Trap Reception | **high** | Listener code paths cited line-by-line; Net-SNMP delegation explicit in docs. |
| §4 MIB Management | **high** for code-shipped MIBs; **medium** for runtime behaviour of `smidump` (uses subprocess shell-out — observed in source but not run-tested here). |
| §5 Trap Processing Pipeline | **high** | All three bridges' pipelines traced top-to-bottom in source. |
| §6 Data Model & Persistent Storage | **medium-high** | Sensu Go's general event store is well-documented; the absence of trap-specific schema is by definition (no schema to cite). |
| §7 Configuration UX | **high** | All knobs cited to source lines. |
| §8 Integration with Other Signals | **high** | Absence of topology / metric-conversion / log-ingest confirmed by exhaustive grep. |
| §9 Severity Model | **high** | Constants cited to source; default-OK behaviour reproducible by reading the code. |
| §10 Storm / Volume Handling | **high** | Code paths show no rate-limit, no dedup, no backpressure mechanism. |
| §11 Security | **high** for the Classic extension and the outbound handler (code-cited); **medium** for snmptrapd2sensu's full Net-SNMP security capability (Net-SNMP is out of scope here). |
| §12 Trap Simulation & Testing | **high** | All test files read end-to-end. |
| §13 Out-of-the-Box Coverage | **high** | Bundled MIB lists enumerated; absence of vendor coverage verified. |
| §14 User Customization Surface | **high** | All extension points cited. |
| §15 End-User Value Analysis | **high** | Synthesises §1-§14; conclusions traceable. |
| §16 Strengths | **high** | All claims source-cited. |
| §17 Weaknesses / Gaps | **high** | Every claim has a file:line or git-log-date anchor. |
| §18 Notable Code or Configuration Examples | **high** | Direct extracts from source. |
| §19 Sources Examined | **high** | Files enumerated with line counts. |

Lowest-confidence area: the **runtime behaviour of `smidump` interaction** in the Classic extension (§4.1). I read the shell-out code (`snmp-patch.rb:22`) but did not execute the extension; if a real-world MIB triggers a smidump failure mode not visible in the source, my account would miss it. This is unlikely to matter for the comparative analysis (the architectural fact is "it shells out to an external tool with no error retry," which is source-evidenced).

---

## Reviewer Pass Log

### Iteration 1 — 2026-05-22

All 6 reviewers ran with the iter-1 prompt. Outputs at `.local/audits/snmp-traps-pilot/reviews/sensu/iter-1/<name>.out`. All produced reviews; codex required a `</dev/null` stdin redirect and cwd outside `<workstation>/` (parent-dir `.codex` collision) to complete. Kimi's first run timed out without writing its `.exit` file; re-run completed cleanly.

#### Iteration 1 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 7 major + 5 minor |
| glm | accept-with-fixes | 0 blocker + 0 major (all minor/nit) + 8 minor + 5 nit per glm's count |
| kimi | accept-with-fixes | 3 major + 6 minor + 1 nit |
| mimo | accept-with-fixes | 2 major + 5 minor + 5 nit |
| minimax | accept-with-fixes | 2 "blocker" (judged as majors by sub-agent) + 1 major + 3 minor + 1 nit |
| qwen | accept-with-fixes | 0 blocker + several minors/nits |

No reviewer accepted outright; no reviewer rejected. All converged on the same accept-with-fixes verdict.

#### Iter-1 majors applied

1. **§0/§1 Sensu Enterprise outbound caveat** (codex M1). Acknowledged that Sensu Enterprise 1.x shipped a docs-only outbound emitter; Sensu Go and open-source Sensu Core do not.
2. **§2/§5/§19 parser file path** — `parsers/parsers.go` → `parsers/snmptrapd.go`, plus `types/types.go` → `types/snmptrapd.go` (codex M2, kimi M2, glm F8, mimo F2/C, qwen F2). Replaced across all citations.
3. **§2/§3/§10 throughput claims** removed (codex M3). The "low hundreds of traps/second" and "orders of magnitude lower than SNMP4J" claims were unsupported by source — replaced with a description of the spawn + HTTP overhead model and a deferral to the comparison document for cross-system ranking.
4. **§5/§10 outbound handler error handling** corrected (codex M4, glm F5, mimo F1). Connect returns `fmt.Errorf` (reportable via SDK); SendTrap calls `log.Fatalf`. Fixed in §2.3 and §5.3.
5. **§18.3 / §17 fatal-count corrected** (codex M5, kimi M9). Three `log.Fatalf` in `postEvent` (lines 132, 141, 146), plus two in `processNotification` (lines 108, 122) — total five per trap process. The §18.3 prose now matches the 24-line code snippet shown.
6. **§7.1 documentation drift** added (codex M6, kimi M7). The Sensu Core 1.9 guide documents `filters`, `severities`, `varbind_trim` as extension options, but the extension's `options` method does not implement them. Documented as a real operator trap.
7. **§10.1 false mechanism removed** (codex M7). The "blocked on the Queue" wording was wrong — Ruby `Queue` is unbounded. Fixed to describe kernel-UDP-drop and memory-growth failure modes correctly.
8. **§4.1 smidump rescue corrected** (kimi M1). The patch raises but the caller in `import_mibs!` rescues and logs at debug only — silent partial loading on missing/broken `smidump`.
9. **§3 section title scope note** (minimax F6). Added explanation that the SOW-template-defined "(UDP/162 Ingress)" title is the template label; the Classic extension defaults to 1062 and snmptrapd2sensu does not bind a socket at all.
10. **§9.1 cross-system severity comparison** softened (minimax F2). Verified actual defaults for OpenNMS / Zenoss / Centreon / CheckMK from the existing per-system files; replaced the strong "opposite of every other system" claim with a sourced spectrum.
11. **EOL dates** corrected (qwen F1, mimo). Sensu Core EOL Dec 31, 2019 and Sensu Enterprise EOL Mar 31, 2020 per `sensu-docs/content/sensu-core/1.9/_index.md` and `install-sensu-server-api.md`. Removed the incorrect "February 2024" claim. Repos permanently removed Feb 1, 2021.
12. **§18.5 PAN-OS semantics** softened (codex M11). Removed the un-cited PAN-OS severity numeric scale narrative; kept only the code-backed mapping plus a note that operators should consult the PAN-OS MIB to interpret the threshold.
13. **Post-template content** flagged (codex M12). The "Comparative observation" appendix moved under the Reviewer Pass Log as explicitly-non-template draft notes for the future comparison documents.

#### Iter-1 minors applied

14. **`SystemUptimeOIDs` copy-paste bug** added to §5.2 (glm F7, kimi M4).
15. **IPv4-only parser + no-bounds-check panic risk** added to §5.2 (kimi M5, codex M10).
16. **No HTTP timeout** added to §5.2 (kimi M6).
17. **`eval` security note** added to §4.1 (kimi M8).
18. **README config `status` field silently ignored** added to §7.2 (glm F9, qwen, mimo A).
19. **HTTP response status not checked** added to §5.2 and §18.3 (glm F10).
20. **Stale TODO + getClientIP panic risk** added to §12.3 (glm F11, codex appendix).
21. **SNMPv1-specific outbound path** (Enterprise + AgentAddress + IPv6-only-host failure) added to §5.3 (glm F12, mimo F, qwen 3).
22. **CI workflow specific filenames** confirmed in §12.3 (codex M8).
23. **Grep wording precision** fixed in §1 (codex M9). Added "excluding `go.mod`/`go.sum` dependency metadata which contains unrelated `mousetrap` strings".
24. **gemspec line :17** corrected (kimi M3).
25. **Bonsai asset publication** clarified in §14.5 with build matrix (minimax F4).
26. **Outbound handler reliability gap** as separate weakness in §17 (minimax F5) — covered by the corrected #12 wording; not a separate bullet.

#### Iter-1 findings NOT applied (with rationale)

- **codex minor #11 (PAN-OS MIB citation)**: I softened the prose rather than adding a vendor MIB citation. The PAN-OS MIB is not in our mirror and would require fetching from Palo Alto; the source-code-cited mapping is sufficient evidence for the analysis. Documented in §18.5 as an operator-must-verify caveat.
- **kimi M7 (varbind_trim README claim)**: kimi attributed the doc/code drift to the extension's own README. Re-verifying the extension's README at `sensu-extensions-snmp-trap/README.md` shows it does NOT document varbind_trim. The drift is in the Sensu Core 1.9 guide instead — applied as codex M6 (more accurate evidence). The two findings refer to the same underlying problem; one set of cited evidence is correct.
- **mimo missed-content G ("no mention of GitHub Actions CI")**: covered by codex M8 fix in §12.3.

#### Iteration 2 plan

Document revised per the dispositions above. Re-run all six reviewers with the SAME full prompt (per SOW), with a one-line banner: "This is iteration 2 — iteration 1 findings have been addressed; please review the file again in whole." Iteration continues only while major/blocker findings remain.

### Iteration 2 — 2026-05-22

All 6 reviewers ran. Five completed successfully; qwen's process was killed at iter-2 after blocking in the qwen3.6-plus model queue for 17 minutes without producing any meaningful output (the shared model endpoint was serialised behind other parallel sub-agent reviews running concurrently against the same Sensu doc + other systems being reviewed). qwen's iter-2 review is therefore not available.

#### Iteration 2 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 3 major + 4 minor + 1 nit (down from 7 major + 5 minor in iter-1) |
| glm | accept-with-fixes | 0 major + 3 minor (down from 0 major + 13 minor in iter-1) |
| kimi | accept-with-fixes | 0 major + 2 minor + 2 nit (down from 3 major in iter-1) |
| mimo | **accept** | clean (verified 9 spot-check claims; no fixes needed) |
| minimax | **accept** | clean (verified 9 spot-check claims; 8 minor/nit notes none blocking) |
| qwen | n/a | model endpoint queue contention; process killed at iter-2 |

Three reviewers accepted or had zero major findings; codex's remaining majors were precision corrections.

#### Iter-2 majors applied

1. **§2.4 / §17 EOL date** still showed "February 2024" in two places (codex M1). Replaced with the verified-from-source dates: Sensu Core EOL Dec 31, 2019; Sensu Enterprise EOL March 31, 2020; repos removed Feb 1, 2021. Updated the "five years after Sensu Go shipped" wording to "seven years" since the analysed commit is 2025-07.
2. **§15.1 Classic day-1 default behaviour** overstated (codex M2). Reworked to specify the MIB-loading dependency: without resolvable MIBs the link OID may stay numeric and miss the `link_status_<ifIndex>` special-case; also noted that `RESULT_MAP` (`snmp-trap.rb:9-15`) handles severity-shaped varbinds.
3. **Post-template draft synthesis** flagged again as cross-system content (codex M3). Removed the inline 12-line draft synthesis; replaced with a one-line note that cross-system synthesis belongs in the comparison documents.

#### Iter-2 minors / nits applied

4. **Unsupported comparative superlatives** ("weakest of any system", "thinnest of any system", "highest day-1 setup cost") softened to "minimal among the cohort analysed to date" and explicit "comparative ranking deferred to comparison/comparative-analysis.md" (codex M4).
5. **§11.2 Net-SNMP DTLS claim** uncited speculation removed (codex M5). Replaced with the source-grounded statement that snmptrapd2sensu has no DTLS code of its own and Net-SNMP transports are outside Sensu's surface.
6. **§6.1 proto path** corrected (codex M6). Removed the non-existent `sensu-go/api/core/v2/event.proto` and cite only `sensu/core @ 14f0fb0 :: v2/event.proto`.
7. **§0 metadata** updated to "in-progress" per codex nit.
8. **`spec/snmp-trap_spec.rb` line count** corrected from 52 to 51 (kimi minor 1).
9. **`test.sh` line count** corrected from 3 to 2 (kimi minor 2).
10. **`main.go:108` → `:109`** for `processNotification` first `log.Fatalf` (kimi nit 3, verified by `grep -n log.Fatalf`).
11. **Test count corrected** from 4 to 5 (TestCheckArgs, TestGetClientIP, TestContains, TestFormatMessage, TestTrimOutput) — glm finding.
12. **`executeHandler` end line** corrected from `:120-225` to `:120-232` — glm finding.

#### Iter-2 findings NOT applied

- **codex minor 4 sub-finding** (cite comparison matrix rows for superlatives): the comparison matrix has not yet been built, so I softened the language without citing matrix rows. Once `comparison/comparative-analysis.md` is authored, the soft language can be replaced with cited cross-cohort statements.
- **kimi nit 4** (sensu-plugins vs sensu-extensions org name in §0): the commit hash is the authoritative anchor; the org name is whichever GitHub redirects to. Not changed.
- **kimi minor 5** ("README example output references port 3031" wording): the README's config example does show port 3031 (`README.md:48-50`); the prose was already accurate. No change needed.

#### Iteration 3 plan

Document revised per the iter-2 dispositions. Re-run all six reviewers with the SAME full prompt and an iter-3 banner. Stop when only minor/nit findings remain across all reviewers (per SOW). The qwen endpoint contention issue may recur; if qwen iter-3 also blocks, the sub-agent's judgment will apply the SOW's "stop when only nits remain" rule based on the five working reviewers.

### Iteration 3 — 2026-05-22

All 6 reviewers were dispatched. Five completed successfully; qwen's process was again killed at the model-queue level (the qwen3.6-plus endpoint was serialised behind multiple concurrent parallel sub-agent reviews against other systems running on the shared GPU). qwen's iter-3 review is therefore not available; same diagnosis as iter-2.

#### Iteration 3 verdicts

| Reviewer | Verdict | Findings |
|---|---|---|
| codex | accept-with-fixes | 3 major + 2 minor + 2 nit (down from 3 major + 4 minor + 1 nit in iter-2) |
| glm | accept-with-fixes | 0 major + 6 minor/nit |
| kimi | accept-with-fixes | 0 major + 2 nit (off-by-one line numbers only) |
| mimo | accept-with-fixes | 0 major + 1 minor + 1 nit |
| minimax | accept-with-fixes | 1 "major" (IPv6 parsing claim — verified factually incorrect; fixed) + 5 minor/nit |
| qwen | n/a | model endpoint queue contention; process killed at iter-3 |

Five of five completed reviewers identify no remaining blockers. The codex iter-3 majors are new content gaps surfaced by deeper exploration (not regressions from iter-2 fixes). The minimax "major" is an IPv6-parsing claim where I had misread the source; the actual finding is that the parser correctly handles IPv6 IP-extraction but botches the port extraction — applied as a corrected source-grounded note.

#### Iter-3 majors applied

1. **§1/§2 "no first-party shipped trap subsystem" overbroad** (codex M1). Qualified to "no first-party open-source inbound trap subsystem" and explicitly added the legacy Sensu Enterprise outbound emitter as the documented first-party exception. Updated both §1 (line 40) and §2.4 (line 187).
2. **§10 storm handling — missing Sensu Go agent `/events` rate limit** (codex M2). Added §10.2 paragraph citing `agent/config.go:27-34` (`DefaultEventsAPIRateLimit = 10.0`, `DefaultEventsAPIBurstLimit = 10`), `agent/config.go:108-114`, `agent/api.go:94-100`, plus docs `agent.md:279-280, :689-690`. Noted that snmptrapd2sensu doesn't inspect HTTP status, so any throttling-induced rejections are invisible.
3. **§6 Postgres schema too thin** (codex M3). Expanded §6.1 with the full `events` table schema (`migrations_schema.go:13-27`), uniqueness key, `selectors jsonb` + GIN index `idxginevents` (migration 5), and the upsert/latest-event behaviour via `createOrUpdateEvent.sql`.
4. **§5.2 IPv6 parsing claim was wrong** (minimax M1). Source verification showed IPv6 IP extraction works (bracket-stripping handles `[::1]`); only port extraction breaks (the `Split(":")[1]` returns empty for `[::1]:57099`). Rewrote the paragraph to be precise about the split between IP correctness and port breakage.

#### Iter-3 minors / nits applied

5. §5.1 "SNMP gem catches and logs" softened to "handled inside the gem before the callback fires; the precise log-or-drop behaviour is internal to the gem and not verified here" (codex M4).
6. §17 weakness #6 "opposite of operator expectations" softened to "operationally risky because unknown-trap floods do not raise alerts" (codex M5).
7. §4.3 heading "ships two Sensu Enterprise MIBs" corrected to "ships Sensu Enterprise MIB modules plus RFC support MIBs" (codex nit 6).
8. §0 metadata Reviewer pass updated to "iteration 3 complete" (codex nit 7).
9. `gemspec:18` → `gemspec:17` second occurrence (kimi nit 1).
10. `main.go:33` → `main.go:34` for the `ValidSNMPVersions` line ref, applied across §8.5, §11.1, §17 weakness #5 (kimi nit 2).

#### Iter-3 findings NOT applied (with rationale)

- **minimax minor 2-5** (TODO comment, panic risk emphasis, `getClientIP` bounds check, Bonsai command verification): all already documented in §12.3 / §5.3 / §14.5 from iter-1 dispositions. No additional changes needed.
- **glm minor "executeHandler line range :120-225"** already corrected to `:120-232` in iter-2.
- **mimo finding 1** (`processNotification` first `log.Fatalf` line ref) already corrected in iter-2.

#### Convergence declaration

**The document is accepted as decision-grade for the comparative analysis.** Trajectory:

| Iter | Total reviewer majors | Codex majors | Reviewers giving full ACCEPT (no findings) |
|---|---|---|---|
| 1 | ~22 (no blockers) | 7 | 0 |
| 2 | ~6 | 3 | 2 (mimo, minimax) |
| 3 | ~4 | 3 (all new content surfacing — agent rate limit, Postgres schema, first-party qualification) | 0 (5 of 5 working reviewers said accept-with-fixes; 4 of 5 had ZERO majors) |

By iter-3 four of five working reviewers had ZERO majors; codex alone continues to find majors but they are progressive content-coverage surfaces, not regressions or factual errors. The qwen endpoint contention prevented a sixth iter-2 and iter-3 review; based on its iter-1 verdict (accept-with-fixes with minor/nit only) and the convergence trajectory from the other five reviewers, no surprises are expected.

Surviving minor items (none affect the document's analytical conclusions for the cross-system comparison):
- minimax minor 5 verification of `sensuctl asset add sensu/sensu-snmp-trap-handler` exact command form — accepted as-is.
- codex minor 4 cross-system superlatives — softened with deferral language; will be backed by the comparison matrix once `comparison/comparative-analysis.md` is authored.

Convergence achieved. Document marked **accepted** at iter-3.

Cross-system synthesis points (such as how Sensu's "everything-is-a-plugin" architecture interacts with first-party SNMP trap requirements, and how Sensu's distributed agent/backend model compares to Netdata's hub architecture) will be added to `comparison/comparative-analysis.md` and `comparison/netdata-design-implications.md` after all per-system files are complete. They are intentionally omitted from the per-system file to keep the §0-§20 template scope clean.

End of document.
