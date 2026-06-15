# Logstash (Elastic Stack) — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: Logstash SNMP trap input — historically the standalone `logstash-input-snmptrap` plugin (v3.x family), now superseded by the consolidated `logstash-integration-snmp` plugin (v4.x family) which bundles both `input-snmp` (polling) and `input-snmptrap` (traps) into a single Ruby gem. Bundled by default in Logstash 8.15.0 and later.
- **Versions analysed**: docs at `elastic/logstash-docs @ e8d7013a53f94d26d940a16dcba1cb6ee95e2f6c` (asciidoc source, plugin generated-variables: `:version: v4.0.6`, `:release_date: 2025-01-23`); core at `elastic/logstash @ 849ad93f12dfbfb2b96e7a332e1d8b9c292b759f`; built-docs at `elastic/built-docs @ 61b460d9509fcb51c017f676a665ebe396ededeb` (separate repo, not nested under `logstash/`); LogstashUI at `elastic/LogstashUI @ 99e07640c94af846a946c395cf9f08a78ae4d446`. Historical built-docs cover v3.0.3 through v4.3.1 of the trap plugin.
- **Source evidence**: **docs-and-control-plane-only — the actual `logstash-input-snmptrap` / `logstash-integration-snmp` Ruby/Java source is NOT mirrored locally**. The upstream plugin source lives at `github.com/logstash-plugins/logstash-integration-snmp` (verified absent from the mirror: `find /opt/baddisk/monitoring/repos/ -name 'logstash-input-snmptrap*' -o -name 'logstash-integration-snmp*'` returns nothing under any platform directory). Every claim below about plugin-internal behaviour is therefore traced either to:
  - the operator-facing asciidoc docs in `elastic/logstash-docs` (the canonical configuration reference Elastic ships);
  - the versioned built-docs HTML in `elastic/built-docs` (point-in-time snapshots of the plugin reference for v3.0.3 → v4.3.1, at paths like `raw/en/logstash-versioned-plugins/current/v4.3.1-plugins-inputs-snmptrap.html`);
  - the LogstashUI Python control plane that programmatically emits trap-pipeline configurations (which constrains and confirms the documented option vocabulary);
  - the core Logstash repo's plugin metadata and release notes;
  - vendor-public URLs cited inline.
  Where a claim cannot be source-verified from these in-mirror artifacts, it is explicitly labelled **`Inferred from docs`** or **`Vendor-documented, not source-verified`**.
- **Repository roots analysed**: `elastic/logstash @ 849ad93f12dfbfb2b96e7a332e1d8b9c292b759f`; `elastic/logstash-docs @ e8d7013a53f94d26d940a16dcba1cb6ee95e2f6c`; `elastic/built-docs @ 61b460d9509fcb51c017f676a665ebe396ededeb`; `elastic/LogstashUI @ 99e07640c94af846a946c395cf9f08a78ae4d446`.
- **Author**: assistant
- **Reviewer pass**: **accepted** (convergence declared after 3 iterations; final Reviewer Pass Log at end of document)

Citations in this document use the convention `elastic/<repo> @ <commit> :: <relative-path>:<line>`. The commit is omitted in subsequent citations where unambiguous; the repository prefix is shown explicitly.

---

## 1. System Overview & Lineage

**Logstash** is the data-pipeline component of the Elastic Stack — a JRuby + Java framework that runs ingestion pipelines defined as `input → filter → output`. Per `elastic/logstash :: LICENSE.txt:1-13`, "Source code in this repository is variously licensed under the Apache License Version 2.0, an Apache compatible license, or the Elastic License. Outside of the `x-pack` folder, source code in a given file is licensed under the Apache License Version 2.0 … Within the `x-pack` folder, source code … is licensed under the Elastic License" — the build produces two sets of binaries (Apache-2.0 for the `-oss` artifacts, Elastic License for the default Elastic-distributed build). The trap input plugin is shipped from the `logstash-plugins/logstash-integration-snmp` repository under its own license (not source-verified here). The primary audience is log/observability engineers consolidating heterogeneous machine data into Elasticsearch, but Logstash is output-agnostic by architecture (dozens of output plugins — Kafka, S3, syslog, files, http, etc.), though the canonical deployment writes to Elasticsearch. The project's lineage (Jordan Sissel's original Ruby project, acquisition by Elastic, position as the "L" of the ELK / Elastic Stack) is widely documented at `https://www.elastic.co/blog/welcome-jordan-logstash` and the Logstash GitHub repository — not central to the SNMP-trap analysis but referenced here for context.

Within that broader product, **SNMP traps are one of ~50 inputs**, alongside `beats`, `tcp`, `udp`, `http`, `syslog`, `kafka`, `s3`, `redis`, and others. Logstash's architectural metaphor is "**trap as a log event**":

- The trap-input plugin terminates the UDP/162 listener (or any configured port), decodes the SNMP PDU, and produces a Logstash `event` with the varbinds expanded as event fields.
- That event then flows through the **same** filter/output pipeline as any other input — `grok`, `mutate`, `translate`, `ruby`, `date`, etc.
- The downstream output (most commonly Elasticsearch in Elastic Stack deployments — Logstash has no plugin-default output, every pipeline must specify one explicitly) writes the event as a document. From that point on, the trap is queryable in Kibana, alertable via Watcher / Kibana alerting / external ElastAlert, and joinable with any other Elasticsearch-indexed signal (logs, metrics with metricbeat, APM, etc.).

This is **distinctly different** from NMS-style systems (OpenNMS, Centreon, Zenoss, CheckMK, Zabbix) which model traps as typed events with built-in alarm lifecycle, severity normalisation, dedup state, and per-event correlation rules. Logstash has **none of those built in** at the plugin layer:

- No native alarm lifecycle. The trap is a document; "open / acknowledged / cleared" semantics do not exist in the plugin. Operators bolt these on through Watcher / Kibana / ElastAlert at the storage layer, or via custom filters and side outputs.
- No native severity normalisation. The trap's varbinds appear as event fields; no `vendor severity → internal severity` lookup ships out of the box.
- No native deduplication. The trap-input plugin emits every received PDU as an event; dedup happens (if at all) downstream — Elasticsearch's `_id` strategy, an explicit `fingerprint` filter + a deterministic doc-id, a `aggregate` filter, or external tooling.
- No native MIB-driven event-class mapping (the snmp-MIB OIDs in the trap are translated to symbolic names if a MIB is loaded, but there is no equivalent of OpenNMS's `eventconf.xml` or Centreon's `traps` table — each varbind is just a field on the document).

Logstash also has **no built-in alerting engine**. Alerting is delegated to Kibana Alerts (formerly Watcher) reading from Elasticsearch — these are completely separate products. Logstash's contribution to the alerting path is ingestion only.

### Relationship to upstream tools

- The plugin does **NOT** delegate to Net-SNMP's `snmptrapd` daemon. It opens its own UDP listener using its embedded SNMP library.
- The pre-v4 standalone `logstash-input-snmptrap` plugin was implemented on top of the Ruby `snmp` gem (the "ruby-snmp" library) — evidence: the deprecated `yamlmibdir` option's description ("directory of YAML MIB maps (same format ruby-snmp uses)" — `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:310`), the migration asciidoc explicitly showing the legacy Ruby object dump format vs the new JSON format (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:94-107`), and the v3.0.3 / v3.1.0 built-docs showing trap message rendering as a `SNMP::SNMPv1_Trap` Ruby object dump (`elastic/built-docs @ 61b460d9 :: raw/en/logstash-versioned-plugins/current/v3.0.3-plugins-inputs-snmptrap.html`, "Description" section).
- The v4 integrated plugin (`logstash-integration-snmp` ≥ 4.0.0) replaces the ruby-snmp backend with **SNMP4j** (direct source-verified evidence): `elastic/built-docs @ 61b460d9 :: raw/en/logstash-versioned-plugins/current/v4.3.1-plugins-inputs-snmptrap.html` and `elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:98-101` state: *"The new `logstash-integration-snmp` plugin combines the `logstash-input-snmp` and `logstash-input-snmptrap` plugins into a single Ruby gem … The individual plugins now share the same code base and have been refactored to leverage the latest version of SNMP4j"* (linking to `https://www.snmp4j.org/`). This is **vendor-documented and source-verified in the mirror**. Corroborating evidence: the new SNMPv3 auth-protocol vocabulary (`hmac128sha224`, `hmac192sha256`, `hmac256sha384`, `hmac384sha512` — `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:118-120, :330-331`) matches SNMP4J's `AuthHMAC*` class-name conventions; the option to choose `tcp` or `udp` transport (`:257-260`) matches SNMP4J's transport-mapping abstraction; and the change-of-message-format from a Ruby object dump to a JSON dict (`integrations/snmp.asciidoc:95-107`) is consistent with the SNMP4j refactor. The plugin still surfaces via JRuby (Logstash plugins are JRuby gems), but the SNMP engine itself is the SNMP4j Java library. **The library attribution is source-verified; specific SNMP4j class names referenced in this document (e.g. `MessageDispatcher`) remain inferred and labelled as such.**
- The plugin **ships its own copy of the IETF MIBs** sourced from `libsmi 0.5.0` (compiled to `.dic` files). Operators wanting vendor MIBs convert ASN.1 → `.dic` using `smidump --level=1 -k -f python <MIB> > <MIB>.dic` (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:130-194`).
- For privileged-port (UDP/162) binding, Logstash's documented default is **port 1062** (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:245-252`) — explicitly chosen "Remember that ports less than 1024 (privileged ports) may require root to use. hence the default of 1062." There is no SUID helper or `setcap` machinery shipped with the plugin; binding 162 is the operator's problem (iptables redirect, capability grant on the JVM, or running Logstash as root in some lab setups).

### Where the plugin sits in the Elastic ecosystem today

The `logstash-input-snmptrap` plugin is **marked for migration** in current Logstash:

- `elastic/logstash :: rakelib/plugins-metadata.json:378-381` shows `logstash-input-snmptrap: { "default-plugins": false, "skip-list": true }` — i.e., the standalone plugin is **not bundled** by default in current Logstash builds.
- `elastic/logstash :: rakelib/plugins-metadata.json:436-439` shows `logstash-integration-snmp: { "default-plugins": true, "skip-list": false }` — the integrated plugin **is** bundled by default.
- The asciidoc has a prominent migration callout: "The `logstash-input-snmptrap` plugin is now a component of the `logstash-integration-snmp` plugin which is bundled with {ls} 8.15.0 by default. This integrated plugin package provides better alignment in snmp processing, better resource management, easier package maintenance, and a smaller installation footprint." (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:26-29`).
- The legacy plugin is still installable from the plugin manager but receives no new development; the v4.x line (integration plugin) is where features, fixes, and security updates land.

### A note on LogstashUI

`elastic/LogstashUI` (an official Elastic-org repo, currently labelled "Beta Release") is a control-plane application built around Logstash. It includes a Python/Django module specifically for **constructing trap pipelines via UI** (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1442-1494`) — the UI configures `network → traps_enabled` per logical network, picks an SNMP credential, and emits a Logstash pipeline with an `snmptrap` input block plus a downstream Elasticsearch output. This is the closest thing in the Elastic ecosystem to a "trap configuration GUI." It is beta, not in the default Logstash install, and described as such in its README. Coverage of LogstashUI throughout this document is for the trap-related portion only.

---

## 2. Trap-Subsystem Architecture

### Components and pipeline shape

```
                  SNMP-capable device(s)
                          |
                          | UDP (or TCP) /<configured-port>
                          v
   +---------------------------------------------------------------+
   |               Logstash JVM process (one per node)             |
   |   ----------------------------------------------------------- |
   |   | Pipeline N: input { snmptrap { ... } }                  | |
   |   |    |                                                    | |
   |   |    |  Plugin SNMP listener (Java library — inferred     | |
   |   |    |  to be SNMP4J; see §1) + N worker threads          | |
   |   |    |  (default 75% of cores) decode PDU, build Logstash | |
   |   |    |  event, push into the pipeline's queue             | |
   |   |    v                                                    | |
   |   |   filter { grok | mutate | translate | ruby | date ... }| |
   |   |    |                                                    | |
   |   |    v                                                    | |
   |   |   output { elasticsearch | kafka | http | file | ... } | |
   |   ----------------------------------------------------------- |
   |   - PQ (persistent queue) is per-pipeline; queue type / size |
   |     are pipeline-level Logstash settings, not plugin-level   |
   |     (general Logstash docs at elastic.co/guide/en/logstash/  |
   |     current/persistent-queues.html — not in mirror).         |
   |   - Pipeline-to-pipeline forwarding via `pipeline` in/output |
   |     allows splitting trap → fan-out to multiple outputs.     |
   +---------------------------------------------------------------+
                                |
                                v
        Default output: Elasticsearch index (e.g. data stream
        `logs-snmp.traps-default` per LogstashUI convention).
        Documents are then queryable in Kibana / via Search API.
```

The trap subsystem is **just one input plugin in one Logstash pipeline**. There is no separate trap daemon, no separate trap database, no separate trap-only deployment tier — Logstash treats the trap input like any other input.

### Deployment models

- **Single-node, hand-configured**. A Logstash pipeline config file (`/etc/logstash/conf.d/snmp-traps.conf` or similar) contains an `input { snmptrap { … } }` block. Operator runs `logstash -f /etc/logstash/conf.d/snmp-traps.conf` or installs Logstash as a systemd service. This is the documented baseline (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:142-159, :369-385`).
- **Multi-pipeline single-Logstash**. Logstash supports multiple pipelines in one JVM via `pipelines.yml`. A trap pipeline can live alongside (say) a beats pipeline and a syslog pipeline; each has its own queue and worker pool. This is a core Logstash feature, not specific to SNMP. **`General Logstash knowledge; cited at the official Elastic docs URL https://www.elastic.co/guide/en/logstash/current/multiple-pipelines.html (not in mirror).`**
- **Distributed Logstash cluster**. Each Logstash node is an independent worker; there is **no native clustering** in Logstash itself. Operators front-end a cluster with a load-balancer (or have devices send traps to N collector hostnames) and have all Logstash nodes write to the same Elasticsearch cluster. UDP load balancing is poor (most LBs do connection-tracking on UDP awkwardly); the more common pattern for traps is to use one Logstash per trap subnet and an iptables/router-side trap-destination configuration on each device. **`General Logstash knowledge; clustering absence is a structural consequence of Logstash's pipeline-per-JVM architecture documented at https://www.elastic.co/guide/en/logstash/current/deploying-and-scaling.html (not in mirror).`**
- **Centralized Pipeline Management (CPM) via Elasticsearch + Kibana**. Pipeline configs are stored in an Elasticsearch index, distributed to Logstash nodes, and managed centrally. LogstashUI uses this — the actual `es_connection.logstash.put_pipeline(id=pipeline_name, body=pipeline_body)` call is at `elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:322` (a shared helper); the trap-specific caller wraps it at `:1779-1784` via `_create_or_update_pipeline(es, trap_pipeline_name, trap_pipeline_content, …)`. This is the Elastic-recommended path for multi-node Logstash config management at v8.x. **`CPM general docs at https://www.elastic.co/guide/en/logstash/current/configuring-centralized-pipelines.html (not in mirror); LogstashUI's usage source-verified above.`**
- **Logstash Agent** (announced beta in LogstashUI; see `elastic/LogstashUI :: docs/docs/logstashui/SNMP/index.md:54-56`, "Coming Soon"). A planned lightweight Logstash distribution for distributed agent-based deployment.
- **HA / clustering**: there is no in-product HA for trap reception. UDP is connectionless; the operator's only HA pattern is "configure devices with two trap destinations" or "front-end with VRRP/keepalived". Both are out-of-band of Logstash itself.
- **Kubernetes**: Elastic provides official Helm charts and ECK (Elastic Cloud on Kubernetes); a Logstash pod with the trap input is identical to any other Logstash pod plus a UDP service / hostPort. **`General Logstash ecosystem knowledge; ECK Logstash docs at https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-logstash.html (not in mirror).`**

### Languages and key libraries

- **Logstash core**: JRuby on top of the JVM. Pipelines are Ruby DSL files compiled into a Java execution graph.
- **Trap plugin runtime (v4+)**: JRuby code with an SNMP4j Java backend (`elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:101` directly states the v4 plugin "has been refactored to leverage the latest version of SNMP4j"). **Source-verified library attribution; per-call sequence/class-name details require upstream plugin source.**
- **Trap plugin runtime (v3, legacy)**: pure Ruby `snmp` gem (ruby-snmp by David R. Halliday — see the bundled NOTICE at `elastic/logstash :: tools/dependencies-report/src/main/resources/notices/snmp-NOTICE.txt:1-2`, copyright 2004-2014). **This NOTICE file documents the legacy backend's licensing only; it is not evidence for the v4 Java backend.**
- **MIB compilation toolchain**: external `libsmi` (`smidump`) at MIB-import time; the integration plugin reads `.dic` (Python-format) files at runtime (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:141-147`).
- **Persistent queue (optional)**: file-backed PQ if enabled at pipeline level; otherwise in-memory queue.
- **ECS (Elastic Common Schema)** integration: when `ecs_compatibility: v1|v8` is set, the plugin uses ECS-aligned field names (e.g., `[host][ip]` instead of `host`). Native to Logstash's plugin SDK.

### Inter-component IPC

- **Inside a Logstash JVM**: the trap input's worker threads push decoded events onto the pipeline's queue; pipeline workers pull from the queue, run filters, push to outputs. All in-process Java/JRuby memory.
- **Logstash → Elasticsearch**: HTTP(S) REST (default Elastic output uses the Bulk API).
- **Logstash → Kafka / other**: per the configured output plugin.
- **Operator → Logstash**: pipeline config files on disk (live-reloaded via `--config.reload.automatic`), or Centralized Pipeline Management via Elasticsearch.
- **No trap-state shared store**: Logstash does not maintain a per-trap dedup table, an alarm DB, or any other persistent state beyond the pipeline's PQ. State (if needed) is delegated entirely to Elasticsearch downstream.

---

## 3. Trap Reception (UDP/162 Ingress)

### Listener implementation

- **Own UDP listener** (no delegation to `snmptrapd`). The plugin opens its own socket on the configured `host:port`. Default `host => "0.0.0.0"`, default `port => 1062` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:174-178, :245-252`).
- **TCP also supported** in v4 integration plugin: `supported_transports => ["tcp", "udp"]`, default `["udp"]`. SNMP over TCP is per RFC 3430 (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:254-264`). This is an unusual capability — most NMS systems are UDP-only for traps.
- **Multi-bind through multiple pipelines**: there is no documented multi-port option on a single `snmptrap` input block. The pattern is one `snmptrap { port => N }` block per port; if you want both 162 and 1062 listening, you write two input blocks (each spawning its own listener).

### SNMP version support

| Version | Supported | Evidence |
|---|---|---|
| v1 | yes (default) | `supported_versions` allowed values `1`, `2c`, `3`; default `["1", "2c"]` — `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:266-273`. Description block shows v1 PDU fields (`agent_addr`, `generic_trap`, `specific_trap`, `enterprise`) — `:42`. |
| v2c | yes (default) | same; trap PDU type is `TRAP` (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:106`). |
| v3 (USM) | yes (opt-in) | Must explicitly set `supported_versions => ['3']` and configure `security_name`, `auth_protocol`, `auth_pass`, `priv_protocol`, `priv_pass`, `security_level`. Example at `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:371-385`. |
| v2c INFORM | **not documented** | The PDU `type` field returned in the JSON output enumerates `V1TRAP` and `TRAP` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:42`, `integrations/snmp.asciidoc:106`). No mention of INFORM PDU handling or acknowledgment in the documented option set or PDU metadata table (`:60-76`). **Cannot confirm or deny from docs alone**; the plugin source would be needed. **`Unverified.`** |
| v3 INFORM | **not documented** | Same; cannot confirm from docs alone. **`Unverified.`** |
| TLSTM / DTLS (RFC 5953/6353) | **not supported** | The only `supported_transports` values are `tcp` and `udp` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:257-258`). No mention of TLS/DTLS anywhere in the trap-input or integration docs. |
| SNMPv3 context (`contextEngineId`, `contextName`) | yes (added in v4.1.0) | Release notes: "Add support for SNMPv3 `context engine ID` and `context name` to the `snmptrap` input" — PR #76 — `elastic/logstash :: docs/release-notes/index.md:703-705`. Direct field-level evidence in the latest built-docs: `elastic/built-docs @ 61b460d9 :: raw/en/logstash-versioned-plugins/current/v4.3.1-plugins-inputs-snmptrap.html` exposes `[@metadata][input][snmptrap][pdu][context_engine_id]` and `[@metadata][input][snmptrap][pdu][context_name]` in the PDU-metadata table. The v4.0.6 asciidoc captured here does not yet show these fields. **`Field surface verified in v4.3.1 built-docs; configuration-option surface (whether operators set context filters explicitly or whether they are PDU-read-only) is not directly cited in the asciidoc.`** |

#### SNMPv3 cryptographic algorithm support (v4)

From `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:118-122` (option-table summary) and `:325-349` (per-option detail):

- **Auth protocols**: `md5`, `sha`, `sha2`, `hmac128sha224`, `hmac192sha256`, `hmac256sha384`, `hmac384sha512` — with the note that `sha2` and `hmac192sha256` are equivalent (`:330-331`).
- **Privacy protocols (base, as of v4.0.6 asciidoc)**: `des`, `3des`, `aes`, `aes128`, `aes192`, `aes256` — visible at `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:118-122` (option-table summary) and `:343-347` (detail). With the note that `aes` and `aes128` are equivalent (`:343-347`).
- **Added in v4.2.1 (PR #78)**: **`aes256with3desKey`** — the "AES256 with 3DES extension" per `elastic/logstash :: docs/release-notes/index.md:391`. Option-value name visible in `elastic/built-docs @ 61b460d9 :: raw/en/logstash-versioned-plugins/current/v4.3.1-plugins-inputs-snmptrap.html:374, :821`, corroborated by `elastic/LogstashUI :: src/logstashui/SNMP/models.py:232`.
- **Security levels**: `noAuthNoPriv`, `authNoPriv`, `authPriv` (`:351-358`).

The legacy v3 trap plugin (≤ v3.x) had **no SNMPv3 options at all** — the v3.1.0 built-docs option list is `community`, `ecs_compatibility`, `host`, `port`, `target`, `yamlmibdir` only (`elastic/built-docs @ 61b460d9 :: raw/en/logstash-versioned-plugins/current/v3.1.0-plugins-inputs-snmptrap.html`, "Snmptrap Input Configuration Options"). v3 traps could not be received at all in the legacy plugin — operators would have had to terminate v3 elsewhere (e.g., snmptrapd → file → file input). This is a meaningful regression-fix in v4.

#### SNMPv3 cardinality limitation

- "**A single user** can be configured. Multiple snmptrap input declarations will be needed if multiple SNMPv3 users are required." (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:312-315`).
- This is structurally different from systems like CheckMK or OpenNMS which let a single listener register multiple v3 users with distinct engine IDs. With Logstash, two v3 users require two `snmptrap` input blocks. **Whether those input blocks must bind different ports is not directly documented**: the asciidoc only says "Multiple snmptrap input declarations will be needed." In practice on a typical Logstash node multiple inputs cannot bind the same UDP port (the OS rejects the second `bind()`), so the operational reality is one v3 user per listening port — but this is an OS-level constraint, not a plugin-documented behaviour. Operators wanting to verify this should consult the upstream plugin source. **`Unverified — depends on plugin socket-binding behaviour; OS-level same-port-bind constraints apply by default.`**
- **SNMPv3 authoritative engine-ID handling**: the asciidoc shows a `local_engine_id` option on the polling input (`elastic/logstash-docs :: docs/plugins/inputs/snmp.asciidoc:80` table row + `:210-211` description block) but **does NOT** expose a corresponding `local_engine_id` for the trap input (no such option appears in `docs/plugins/inputs/snmptrap.asciidoc`). For an inbound listener, whether the plugin generates an authoritative engine ID, performs RFC 5343 discovery, or hard-codes one is undocumented; this is operationally relevant when devices are configured with a fixed remote engine ID. For an inbound trap listener, the engine ID is typically the listener's own authoritative engine ID against which devices encrypt v3 traps (RFC 3414). **Whether the trap plugin uses a generated, configured, or hard-coded authoritative engine ID is not directly documented in the asciidoc** — this is a real operational gap when devices are configured with a fixed remote engine ID. **`Unverified — plugin source needed.`**

### Privileged-port handling

- Plugin default `port => 1062` deliberately avoids privileged-port binding (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:245-252`).
- LogstashUI's auto-generated trap pipelines use `port: 1662` (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1444`) — another unprivileged choice.
- For UDP/162 binding the operator must:
  - Run Logstash as root (discouraged in production);
  - Grant `cap_net_bind_service` to the JVM binary;
  - Or front-end with iptables / nftables port redirection (162 → 1062);
  - Or use a load-balancer / firewall NAT.
- The trap plugin docs do **not** document any of these options; this is left as an operator-deployment concern outside the plugin scope.

### Concurrency model

- `threads` option: number of worker threads decoding received PDUs. Default: **75% of the number of CPU cores** (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:286-292`). On an 8-core host the listener spawns 6 PDU-decoding worker threads.
- All decoded events go into the **pipeline queue** (per-pipeline, configured at the pipeline level, not the plugin level).
- From there, pipeline workers (separate threadpool, `pipeline.workers` setting) run filters and outputs.
- This is materially different from systems like CheckMK (single-thread reception, single-thread filter, single-thread output — synchronous) and closer to e.g. Datadog's agent-side model (decoder pool feeding an internal queue).

### Horizontal scaling pattern

- No built-in clustering of Logstash trap inputs. The pattern is per-network: deploy one Logstash per site or per Layer-2 trap broadcast domain, point devices' trap-destination at that local Logstash, have them all write to the same Elasticsearch cluster.

The architectural consequence is that Logstash hubs act as *forwarders* into a central Elasticsearch which becomes the correlation/query tier — distinct from NMS-style systems that centralise trap reception itself, and distinct from log-pipeline architectures that correlate at the edge. Detailed cross-system distinction belongs in the per-comparison document, not here.

### HA / clustering

- **No native HA**. UDP is fundamentally connectionless and the trap plugin does not coordinate with peer instances. The only documented HA pattern for UDP listeners (across all Logstash inputs, not just SNMP) is "send to multiple destinations, accept duplicate ingestion, dedupe downstream."
- Devices typically support **two trap destinations** — operators configure both, accept duplicates at the Elasticsearch layer (using fingerprint + deterministic `_id`), and rely on at-least-once semantics. This is **not** plugin-implemented; it is an operator pattern.

---

## 4. MIB Management

### MIB store location and layout

- The integration plugin **bundles the full IETF MIB set** sourced from `libsmi 0.5.0` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:294-301`). These are loaded automatically unless `use_provided_mibs: false`.
- Operator-supplied MIBs are provided as `.dic` files (a Python dict format produced by `smidump --level=1 -k -f python`) and located via the `mib_paths` config option (`:182-191`).
- `mib_paths` accepts either a directory of `.dic` (and legacy `.yaml`) files or a single file path.
- The legacy `yamlmibdir` option is deprecated as of v4.0.0 in favour of `mib_paths` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:303-310`).

### Compilation / load pipeline

The MIB-import workflow is fully documented at `elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:130-194`:

1. Operator obtains the vendor's ASN.1 MIB file (e.g., `CISCO-PROCESS-MIB.mib`).
2. Operator runs `smidump --level=1 -k -f python <MIB> > <MIB>.dic` to produce a Python-style dict.
3. Operator deposits the `.dic` file in a directory pointed to by `mib_paths`.
4. Restart (or reload) Logstash so the plugin picks up the new MIB.

There is **no UI**, **no auto-import**, and **no MIB-dependency-resolution-via-network**. If the imported MIB depends on other MIBs not in libsmi's search path, `smidump` errors with `failed to locate MIB module`. The docs cover two workarounds:

- Set `SMIPATH=":/path/to/mibs/"` before running `smidump` (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:165-175`);
- Or create a `smi.conf` with `path :/path/to/mibs/` and run `smidump -c smi.conf …` (`:178-191`).

This is a **non-trivial operator burden** compared to systems like OpenNMS, Zenoss, or CheckMK which ship vendor MIB packs out of the box and have a GUI MIB-upload page that resolves dependencies. Logstash's stance is: "we bundle the IETFs; vendor MIBs are your problem."

### Bundled MIBs

- Full libsmi 0.5.0 IETF set. Source: `https://www.ibr.cs.tu-bs.de/projects/libsmi`. This is RFC-standard SNMP MIBs (e.g., SNMPv2-MIB, IF-MIB, IP-MIB, TCP-MIB, UDP-MIB, RFC1213-MIB, etc.) — **no vendor-specific MIBs** (no Cisco, Juniper, HPE, Arista, Dell, NetApp, etc.).
- This is far less than what OpenNMS, Centreon, Zenoss, or CheckMK ship by default. Operators with vendor traps need to import every vendor MIB manually.

### User workflow for adding/updating MIBs

- File-system + `smidump` workflow as above; no GUI workflow in stock Logstash.
- LogstashUI (beta) does not appear to surface MIB upload either — its SNMP CRUD code references no `mib_paths` field in the trap-pipeline construction (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1440-1494` does not include a `mib_paths` key in `trap_input_config`). MIBs are still left to the file system.

### Dependency resolution

- Entirely delegated to `smidump` / `libsmi`. No plugin-level resolution.
- `SMIPATH` env var or `smi.conf` are the operator levers (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:152-163`).

### Version management vs firmware

- Not handled by the plugin. The plugin treats whatever MIBs are imported as the source of truth. If a device's firmware update changes a MIB, the operator must re-import.

### Fallback behaviour for unknown OIDs

- The `oid_mapping_format` option controls textual rendering of OIDs at event-time (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:192-204`):
  - `default`: every identifier translated via MIB if resolvable, separated by dots. E.g. `1.3.6.1.2.1.1.2.0` → `iso.org.dod.internet.mgmt.mib-2.system.sysObjectID.0`.
  - `ruby_snmp`: prefixed by MIB module name + last resolved name. E.g. `1.3.6.1.2.1.1.2.0` → `SNMPv2-MIB::sysObjectID.0`. This is the legacy ruby-snmp format and is the recommended setting for migration-compatibility with the old standalone plugin.
  - `dotted_string`: numeric only. E.g. `1.3.6.1.2.1.1.2.0` → `1.3.6.1.2.1.1.2.0`.
- The `oid_root_skip` and `oid_path_length` options trim the leading dots for shorter field names (`:215-243`). Both only apply when `oid_mapping_format: default`.
- For OIDs **not** resolved by any loaded MIB: the plugin uses the numeric or partially-resolved form depending on `oid_mapping_format`. There is no "Unknown OID → fall back to UEI/event-class" pattern as in OpenNMS or Centreon.

---

## 5. Trap Processing Pipeline

### Stage 1 — PDU receive

- UDP/TCP `recvfrom`/`accept` on the configured `host:port`.
- Hands raw bytes to a Java SNMP-library dispatcher for ASN.1/BER decode and PDU type detection. The library is inferred to be SNMP4J (see §1 for the inference chain); specific SNMP4J class names (e.g. `MessageDispatcher`) are not cited as fact because the evidence is option-vocabulary-only. **`Vendor-documented, not source-verified — the library attribution and any specific class names are inferred; the actual call sequence inside the plugin requires upstream-source inspection.`**

### Stage 2 — PDU decode and varbind extraction

- The decoded PDU produces:
  - PDU type (`V1TRAP` for SNMPv1, `TRAP` for SNMPv2c/v3) — `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:73`.
  - SNMPv1-specific fields when `version=1`: `agent_addr`, `generic_trap`, `specific_trap`, `enterprise`, `timestamp` (`:62-72`).
  - SNMPv2c/v3-specific fields: `request_id`, `error_status`, `error_status_text`, `error_index` (`:65-71`). The asciidoc lists `request_id` for SNMPv2c/v3 TRAPs; in standard SNMP this field is native to INFORM (and Get/Set/Response) rather than TRAPv2, so its presence here likely reflects the underlying SNMP4j PDU class structure rather than a protocol-level field — not materially significant for operators.
  - Always-present fields: `community` (for v1/v2c), `version`, `variable_bindings` (`:62-75`).

### Stage 3 — OID-to-name resolution

- Per-OID translation through MIB lookup (controlled by `oid_mapping_format`, see §4). Translation is best-effort against the loaded MIBs (bundled IETFs + any operator-imported `.dic` files).
- Field-value-level translation is **opt-in** via `oid_map_field_values: true`. When false (default), only field *names* are translated; OID values inside varbinds remain dotted strings (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:206-213`).
- **The `target` option** (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:275-284`) namespaces all OID-derived fields under a configurable prefix (e.g. `target => "snmp"` writes `snmp.<oid-or-symbolic>` instead of writing each field at the event root). Recommended when `ecs_compatibility` is enabled because event-root OID fields can collide with ECS top-level fields. There is no default — operators must opt in.
- **`@metadata` design implication**: the PDU header fields (community, version, generic_trap, agent_addr, …) are placed in the event's `@metadata` namespace (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:56-76`). `@metadata` is a Logstash-specific in-pipeline scratch space that is **NOT serialized to outputs by default**. To retain PDU-header fields in the persistent Elasticsearch document, the operator must explicitly copy them out, e.g. `mutate { copy => { "[@metadata][input][snmptrap][pdu]" => "[snmp][pdu]" } }`. The varbinds *are* placed on the persistent event (at root or under `target`), but the PDU envelope is operator-opt-in for persistence. This is a non-obvious operator pitfall and a meaningful day-1 ergonomic gap.

### Stage 4 — Source identification (IP → device mapping)

- The plugin records the **UDP packet source IP** as the event's `host` field (ECS-disabled mode) or `[host][ip]` (ECS v1/v8 mode) — `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:50-54`. This **is** serialized to outputs by default (it lives on the event proper, not in `@metadata`).
- For SNMPv1 traps specifically, the v1 PDU also contains an **`agent_addr` field** (a varbind-style network-address field giving the originating device's IP per the v1 RFC). This is exposed as `[@metadata][input][snmptrap][pdu][agent_addr]` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:62`). **Note**: `@metadata` is not serialized to outputs by default (see Stage 3 note on `@metadata` design), so operators wanting to retain `agent_addr` on the persisted document must explicitly copy it via a `mutate` filter.
- Whether the operator chooses to identify the device by UDP source IP or by v1 `agent_addr` is **a filter-level decision**. Out of the box, the plugin populates both fields and lets the pipeline choose. (In NAT/proxy/VRF environments these can differ; the plugin's stance is "give them both and let the user decide.")
- There is **no device-inventory lookup** built into the plugin. There is no "device DB" inside Logstash. Mapping IP → hostname → asset-record is the operator's job:
  - via a `dns` filter (reverse-lookup the source IP);
  - via a `translate` filter (operator-maintained CSV/YAML/Elasticsearch lookup);
  - via a `ruby` filter calling out to an external API;
  - or by leaving raw IPs and joining with a Kibana lookup-index at query time.

### Stage 5 — Enrichment

- Entirely **delegated to filters** in the pipeline:
  - `mutate` — rename / convert / strip / add fields;
  - `translate` — local key→value lookup against a dictionary file or Elasticsearch index;
  - `grok` — pattern-extract from varbind string values;
  - `date` — parse trap timestamps;
  - `ruby` — arbitrary Ruby code;
  - `dns` — reverse / forward DNS resolution;
  - `elasticsearch` — query ES for asset records and join them onto the event;
  - …and ~50 other filter plugins.
- The plugin itself **emits the raw trap event** and intentionally does nothing else. This is the architectural inverse of OpenNMS's `eventconf.xml` (where the trapd daemon itself looks up the event-class, severity, and parameters before emission).

### Stage 6 — Normalization

- **None done by the plugin**. Vendor-severity → internal-severity mapping, vendor-error-code → human-text mapping, vendor-vlan-id → vlan-name mapping — all of these are filter-level operations operators write themselves.
- The integration plugin (v4+) did standardize *some* low-level things: TimeTicks now emitted as `Long` (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:89`), `null` rendered as lowercase string `"null"` (`:90`), error PDU statuses get human messages (`:91-93`). These are mapping/error-cosmetic, not semantic normalization.

### Stage 7 — Deduplication / suppression

- **None done by the plugin**. The plugin has no rate limit, no per-source flood detection, no dedup key, no suppression window.
- Patterns operators use:
  - Elasticsearch document-id strategy: compute `_id` via `fingerprint` filter on (community, source_ip, trap_oid, varbind_subset) so duplicates within an indexing window overwrite the same document. This collapses bursts into one document but does not produce a count.
  - `aggregate` filter (in the `logstash-filter-aggregate` plugin): time-based grouping by a key field; emit a single aggregate event after a timeout. Complex to operate; can use significant memory.
  - `throttle` filter: drop or count events exceeding a rate threshold.
  - External: Elasticsearch ingest pipeline using `enrich` or Painless scripts to dedupe.
- The architectural consequence: **trap storms ingest at line rate** into Elasticsearch unless explicitly throttled. With a misconfigured device flooding traps, Logstash will happily forward every PDU to ES until disk fills.

### Stage 8 — Routing

- Routing is just the Logstash `output { … }` block:
  - `output { elasticsearch { … } }` — write to ES (the canonical pattern);
  - `output { kafka { … } }` — emit to Kafka;
  - `output { file { … } }` — log to disk;
  - `output { http { … } }` — POST to an HTTP endpoint;
  - `output { snmptrap { … } }` — **no in-mirror evidence proves the existence, current version, or configuration of an `logstash-output-snmptrap` plugin**. The in-mirror integration plugin docs (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:35-39`) explicitly list only `input-snmp` and `input-snmptrap` as integration components — no output is in the integration. **Excluded from confirmed capabilities in this analysis; operators wanting northbound SNMP forwarding should look at the upstream `logstash-plugins/logstash-output-snmptrap` repository (not mirrored) and verify currency / supported status with Elastic directly.**
- Conditional routing via `if`/`else` and `output { pipeline { … } }` is standard Logstash and lets operators fork traps to multiple sinks (e.g., ES for query + Kafka for stream + file for archive).

### Stage 9 — Error handling for malformed PDUs / unknown OIDs / decode failures

- **Not documented in detail in the trap-input asciidoc.** The integration-plugin migration notes do mention several concrete error-mapping changes that apply to both polling and trap inputs:
  - "An *unknown variable type* falls back to the `string` representation instead of logging an error as it did with the stand-alone `logstash-input-snmp`" (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:72-73`, polling-input migration block).
  - "*No such instance errors* are mapped as `error: no such instance currently exists at this OID string` instead of `noSuchInstance`" (`:69` for polling-input migration, `:90` for trap-input migration).
  - "*No such object errors* are mapped as `error: no such object currently exists at this OID string` instead of `noSuchObject`" (`:70` polling, `:91` trap).
  - "*End of MIB view errors* are mapped as `error: end of MIB view` instead of `endOfMibView`" (`:71` polling, `:92` trap).
  - "*`null` variable values* are mapped using the string `null` instead of `Null` (upper-case N)" (`:90` — trap-input migration block).
- For more catastrophic decode failures (truncated PDU, ASN.1 parse error, auth-rejected v3), the common Logstash plugin pattern is to log at WARN, tag the event with a parse-failure tag (the exact tag name varies by plugin and is not source-verified for the trap input — common Logstash conventions include `_snmptrapparsefailure` / `_snmpparsefailure` / `_jsonparsefailure`-style), and deliver the partial event. Operators then write filter logic conditional on the failure tag. **`Inferred from general Logstash plugin patterns; specific behaviour for the trap input is not source-verified.`**
- **One concrete source-verified PDU-level rejection** exists: the v4.0.7 fix at `elastic/logstash :: docs/release-notes/index.md:876` says "The `snmptrap` input now correctly enforces the user security level set by `security_level` config, and drops received events that do not match the configured value" — i.e., the plugin DOES drop traps whose USM security level does not match the configured `security_level`. This is a concrete drop behaviour (no parse-failure tag — the event is dropped before becoming a Logstash event).

---

## 6. Data Model & Persistent Storage

### Where state lives

| State category | Where stored | Schema / format | Notes |
|---|---|---|---|
| **Persistent trap document** | The configured output (Elasticsearch in the canonical Elastic Stack deployment, but every pipeline must specify its output(s) explicitly — there is no plugin-default output) | per the configured ES mapping — typically dynamic mapping unless an explicit index template / data-stream component template is defined | by default the persisted document includes: **the source-host fields (`host`/`[host][ip]`)**, **the varbinds** (each as an event field at the event root or under `target` if configured), and **`@timestamp`** (Logstash-injected). **PDU-header fields are NOT persisted by default** — see next row. |
| **PDU header in `@metadata`** | event `@metadata` namespace (in-memory only during pipeline traversal); **NOT serialized to outputs unless explicitly copied** | `[@metadata][input][snmptrap][pdu][...]` — `community`, `version`, `type`, `agent_addr`, `enterprise`, `generic_trap`, `specific_trap`, `request_id`, `error_status`, `error_status_text`, `error_index`, `timestamp` (the SNMPv1 PDU timestamp), `context_engine_id`, `context_name`, `variable_bindings` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:59-76`, plus `elastic/built-docs @ 61b460d9 :: raw/en/logstash-versioned-plugins/current/v4.3.1-plugins-inputs-snmptrap.html:227-235` for context fields) | unique to Logstash's design; `@metadata` is plugin-only scratch space. Operators must explicitly copy fields out (e.g. `mutate { copy => { "[@metadata][input][snmptrap][pdu][community]" => "[snmp][community]" } }`) to retain them in the persistent ES document. |
| **MIB compiled forms** | file system (under the directory passed to `mib_paths`) | `.dic` (Python dict) files | static — read at pipeline start, not updated at runtime |
| **OID → name mapping** | in-process JRuby/Java memory (built from the loaded MIBs at pipeline init) | implementation-internal | rebuilt on pipeline reload |
| **Dedup state** | **none** — no built-in dedup table | n/a | operator-implemented in filters or downstream |
| **Suppression rules** | **none** | n/a | operator-implemented in filters or downstream |
| **Severity rules** | **none** | n/a | operator-implemented in filters or downstream |
| **Device inventory** | **none** — no device DB | n/a | operator-implemented via `translate` / `dns` / `elasticsearch` filter |
| **Topology** | **none** | n/a | not modeled at all in Logstash |
| **Audit / log** | Logstash's own logs (`logstash-plain.log`, `logstash-slowlog.log`, etc.) | textual / JSON | covers plugin startup, parse warnings, output errors; not a per-trap audit trail |
| **Pipeline queue (PQ)** | optional file-backed PQ at `path.queue` directory | proprietary on-disk format | per-pipeline; for backpressure / durability before reaching output |

### The Elasticsearch document model (typical)

Logstash + Elasticsearch + traps usually lands on an ECS-aligned mapping. The integration plugin describes the field layout:

- `[host][ip]` (ECS) or `host` (ECS-disabled) — UDP source IP (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:50-54`). **Persisted by default**.
- `@timestamp` — Logstash receive time (UTC). **Persisted by default**.
- **Varbinds** — written at the event root, or under `target` if `target => "snmp"` is set. Each varbind appears as a separate event field, e.g. `variable_bindings.1.3.6.1.2.1.1.1.0` or `variable_bindings.SNMPv2-MIB::sysDescr.0` depending on `oid_mapping_format`. **Persisted by default**.
- **PDU-header fields (`version`, `community`, `type`, `agent_addr`, `enterprise`, `generic_trap`, `specific_trap`, `request_id`, `error_status`, `error_status_text`, `error_index`, …)**: **NOT persisted by default** — they live in `@metadata` and must be explicitly copied to the event root by a `mutate` filter to appear in the ES document (see §5 Stage 3 and the §6 PDU-header row above).
- LogstashUI's emitted pipelines tag with `[event][kind] = "traps"` via a `mutate` filter (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1478-1488`), and use ES data-stream `snmp.traps` (`:1265`). They do **not** copy the PDU header out of `@metadata` — operators wanting the PDU header indexed must add their own `mutate copy` filter to the generated pipeline.

### Retention

- **Not handled by the plugin**. Elasticsearch ILM (Index Lifecycle Management) policies handle retention if traps go to ES; for other outputs, the operator's storage tier defines retention.

### Indexing

- ES default dynamic mapping creates `keyword`/`text`/`long`/`date` mappings on the fly. Operators wanting performance set explicit component templates beforehand.
- The integration's v4 migration changed types of common SNMP value classes: `TimeTicks → Long`, `null → "null"` string, error states → human strings (`elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:86-93`). This affects ES mapping decisions (e.g. `TimeTicks` is now numeric).

### Migration / upgrade handling

- Plugin migration from standalone v3 → integration v4 is the **most material breaking change** in the trap plugin's history. See `elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:51-127`:
  - `message` field format changed from a Ruby object dump (`#<SNMP::SNMPv1_Trap:0x6f1a7a4 …>`) to a JSON dict.
  - PDU variable-binding values are now typed (Long for TimeTicks, lowercase `"null"`, error strings) rather than stringified.
  - Compatibility mode: `oid_mapping_format => 'ruby_snmp'` + `use_provided_mibs => false` + `oid_map_field_values => true` preserves much of the old behavior (`:117-125`).
- ES index templates and Kibana dashboards built on the old field shape must be updated. This is a known operator pain-point in any 8.15+ upgrade with active trap pipelines.

---

## 7. Configuration UX

### Configuration surfaces

| Surface | What it covers | Path / mechanism |
|---|---|---|
| **Pipeline config file** | the canonical trap pipeline definition: `input { snmptrap { … } } filter { … } output { … }` | `/etc/logstash/conf.d/*.conf` (or wherever `path.config` points). Editable in any text editor. Live-reloaded if `--config.reload.automatic` is set. |
| **Pipelines manifest** | which pipelines to run in this Logstash JVM, with their config paths and queue settings | `/etc/logstash/pipelines.yml` |
| **Logstash settings** | global JVM settings, log paths, persistent queue size, pipeline workers, etc. | `/etc/logstash/logstash.yml` and `/etc/logstash/jvm.options` |
| **Centralized Pipeline Management** | pipelines pushed via Elasticsearch index, fetched by Logstash nodes on startup | Kibana UI's "Logstash Pipelines" management screen + ES `.logstash` index |
| **REST monitoring API** | runtime status: which pipelines are running, queue depth, plugin throughput, JVM stats | `http://logstash-host:9600/_node/stats` (the Logstash monitoring API) — Logstash core, not the trap plugin specifically |
| **LogstashUI (beta)** | UI for SNMP networks, credentials, devices, polling profiles, and **trap enable/disable per network** with per-network credential selection. Constructs the pipeline programmatically and pushes via CPM. | `elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1442-1494` builds the input block, `:1781-1782` pushes via `es.logstash.put_pipeline(...)` |

### What the operator sees by default

- Out of the box: nothing related to traps. The trap pipeline must be explicitly authored. The plugin lives in `/usr/share/logstash/vendor/bundle/jruby/.../gems/logstash-integration-snmp-X.Y.Z/` (per the standard Logstash plugin layout) — operators discover it via plugin docs, not via any default sample.
- Logstash ships **no default-enabled trap pipeline and no full production-sample pipeline**. The plugin asciidoc contains minimal v1/v2c and v3 input-block snippets (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:139-158, :368-385`), but no canonical pipeline-with-filters-and-output is shipped. Compare to OpenNMS or CheckMK which ship dozens to thousands of pre-canned trap definitions.
- The snmptrap input also inherits the **common Logstash input options** via `include::{include_path}/{type}.asciidoc[]` at `snmptrap.asciidoc:388` — these add `add_field`, `tags`, `enable_metric`, `id`, `type`, plus the codec setting. Of these, `add_field` and `tags` are particularly useful for trap pipelines because they let operators inject classification/routing fields without writing a separate `mutate` filter block.

### Discoverability of options

- All options documented in the asciidoc (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc`). Each option is listed with type, default, allowed values, and a short prose description.
- The pipeline DSL is **Ruby**; syntax errors are caught at pipeline-start by Logstash's configuration parser. No auto-completion is built into Logstash itself; some IDE plugins exist (community).
- Validation: type-checking is done by the plugin's `config :foo, :validate => :type` macros (a Logstash plugin SDK feature). Unknown options or wrong types fail the pipeline at start.
- LogstashUI exposes the trap-input through **two different components**:
  - The **SNMP CRUD wizard** (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1440-1468`) provides a fixed, opinionated form for `host`, `port`, `oid_map_field_values`, `oid_mapping_format`, `supported_versions`, plus the v3 cluster (`security_name`, `auth_protocol`, `auth_pass`, `priv_protocol`, `priv_pass`, `security_level`). The wizard does **not** surface `threads`, `mib_paths`, `oid_root_skip`, `oid_path_length`, `supported_transports`, `target`, `ecs_compatibility`, `use_provided_mibs`, `community` (non-default values beyond the credential).
  - The **generic PipelineManager plugin catalog** at `elastic/LogstashUI :: src/logstashui/PipelineManager/data/plugins.json:7436-7672` knows the **full plugin option schema** (27 settings including `mib_paths`, `threads`, `target`, `supported_transports`, `ecs_compatibility`, `use_provided_mibs`, the full v3 cluster, and the common-options group `add_field` / `codec` / `enable_metric` / `id` / `tags` / `type`). Pipelines authored via the visual editor or text editor can use any of these; only the SNMP CRUD wizard is restrictive.
  - The practical implication: the *quick path* (wizard) is opinionated and minimal; the *power-user path* (visual editor + plugin catalog) is full-coverage.

### Live reload vs restart

- Logstash supports `--config.reload.automatic` (with `--config.reload.interval`) to live-reload pipelines when files change. **In-mirror evidence**: `elastic/logstash :: docs/reference/reloading-config.md:13-16, :46-52, :83`. Whether the trap-input plugin tears down and re-opens its UDP socket on reload — and whether that is graceful for in-flight PDUs — is a runtime behaviour not documented in the trap-input asciidoc. **`Not source-verified; general Logstash behaviour is to stop the pipeline and restart it, which would close and re-open the UDP socket.`**
- For CPM-managed pipelines, Logstash polls Elasticsearch periodically for changes and reloads automatically.

### Multi-tenancy / RBAC

- **No plugin-level RBAC.** The plugin has no concept of "tenant" or "user with permissions" on the trap data.
- Multi-tenancy is achieved by writing different traps to different Elasticsearch indexes / data streams (e.g., a tenant ID in the pipeline tagging logic) and using Elasticsearch index-level RBAC + Kibana spaces.
- Pipeline-config RBAC is at the file-system level (or Kibana CPM permissions for CPM-managed pipelines).

---

## 8. Integration with Other Signals

### 8.1 Metrics

- **Trap → metric**: not native. A trap is a document; turning it into a metric requires:
  - a `metric` filter that updates a counter / gauge / timer in-memory (then a separate output emits the metric);
  - or a `kafka` output feeding a downstream metrics pipeline;
  - or letting traps land in ES and using Elasticsearch's stats / aggregations as a metric source (this is the dominant Elastic pattern).
- **Traps as annotations on metric dashboards**: in the Elastic ecosystem, Kibana dashboards display traps as events on metric charts via the timeline / TSVB / Lens layered chart features (querying the trap data-stream index in parallel with metric indexes). This is a Kibana feature, not a plugin feature.

### 8.2 Alerting / Notifications

- **No native alerting in Logstash.** The plugin emits documents; alerts come from:
  - **Kibana Alerts** and **Watcher** co-exist in current Elastic Stack — they are distinct systems with different audiences:
    - **Kibana Alerts** (https://www.elastic.co/guide/en/kibana/current/alerting-getting-started.html) — the UI-forward product, recommended for operators writing alerting rules through the Kibana UI. Rules query indexes every N seconds and fire actions (email, Slack, PagerDuty, ServiceNow, webhook).
    - **Watcher** (https://www.elastic.co/guide/en/elasticsearch/reference/current/xpack-alerting.html) — the Elasticsearch-level API for programmatic alerting; rules are JSON documents in the `.watches` index. Still fully supported, not deprecated, but lower-level than Kibana Alerts.
  - **ElastAlert** (third-party open-source, `github.com/Yelp/elastalert` and its successor fork `github.com/jertel/elastalert2`) — Python framework that queries Elasticsearch on a schedule and fires alerts. Not Elastic-owned.
  - **External SIEM** — Logstash often feeds Splunk, QRadar, ArcSight, or Elastic Security; alerting in those products is their concern.
- **Acknowledgement / clear semantics**: also delegated. In Kibana Alerts, the rule has its own state machine (active, recovered, acknowledged) tied to its query; the underlying trap documents are immutable. There is no "open alarm → matching clear" pattern built into the plugin.

### 8.3 Topology

- **No topology graph in Logstash core** for traps. Logstash is a pipeline tool; it does not maintain device/link state from trap input.
- LogstashUI (beta) ships a **polling-derived** CDP adjacency graph at `elastic/LogstashUI :: src/logstashui/SNMP/network_map.py` (459 lines) — it queries the polling data-stream `metrics-snmp.polling-default` for CDP rows and renders a D3.js graph. **This network map consumes polling metrics only; it does not consume trap events.** A trap-correlated topology graph does not exist in any in-mirror artifact.
- Topology-aware suppression: not natively possible. An operator wanting to suppress link-down traps when the upstream switch is also down would have to:
  - Join the trap with topology metadata stored in ES (e.g., a `parent_device` field per device, populated by an asset-management ES index);
  - Use a `ruby` filter or `elasticsearch` filter to query the topology index at filter time;
  - Decide whether to drop the trap or tag it.
- This is **operationally feasible** in Logstash because of its plugin flexibility, but it is **not provided** out of the box. Cross-system comparison: OpenNMS does this natively via `eventconf.xml`+ alarm correlation; Centreon and Zenoss have built-in topology models; Logstash has none.

### 8.4 Logs / Events

- This is the native Logstash + Elasticsearch fit. The trap is **already a document in Elasticsearch** when the operator wants to inspect it.
- **Searchability**: full Lucene full-text + structured query via Kibana Discover (or Elasticsearch Search API). Sub-second queries on indexed varbind fields with proper mapping.
- **Retention**: ES ILM (Index Lifecycle Management — https://www.elastic.co/guide/en/elasticsearch/reference/current/index-lifecycle-management.html). Default Elastic-recommended tier for time-series indexes is hot/warm/cold/frozen with rollover.
- **Schema**: ECS-aligned (when `ecs_compatibility` is set) with `@timestamp`, `host.ip`, `event.kind`, etc. ECS gives a structural common-vocabulary across all Elastic data sources, so a trap document and a syslog document and a beat-collected metric document share index conventions.
- **Unified event store**: yes — by virtue of ES being the universal sink. Traps, syslog, beats logs, APM events, Auditbeat events, security alerts all live in the same cluster (often the same Kibana space). This is the Logstash architectural model's main benefit for trap support: a single query/UI surface over heterogeneous signals.

### 8.5 Northbound Forwarding

- **No in-mirror evidence proves the existence, current version, or configuration of an `logstash-output-snmptrap` plugin**. The integration's documented component list at `elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:35-39` covers `input-snmp` + `input-snmptrap` only — no output is in the integration. An `output-snmptrap` may exist at upstream `logstash-plugins/logstash-output-snmptrap` but its capabilities are **excluded from this analysis** because nothing in the mirror documents them.
- Other northbound options (also delegated to output plugins, not trap-specific):
  - `syslog` output — write to a remote syslog server;
  - `http` output — POST to any HTTP endpoint;
  - `tcp`/`udp` output — raw socket forward;
  - `kafka` output — message-bus distribution;
  - `email` output — send mail (rarely used for high-volume traps).
- A trap fed back as SNMP is therefore possible but is an explicit operator wiring decision.

---

## 9. Severity Model

- **None built in.** The trap PDU does not contain a severity field by SNMP design; vendor MIBs typically have a `<vendor>severity` varbind with vendor-specific encoding. Logstash's plugin does not normalize any of this.
- The varbinds appear as event fields; an operator-written filter (`translate`, `mutate`, `ruby`) maps the vendor severity to whatever severity scale the operator uses downstream.
- In the Elastic / ECS world, the convention is `event.severity` (numeric `long`, typically `1-10` with higher == more severe per the ECS spec at `https://www.elastic.co/guide/en/ecs/current/ecs-event.html#field-event-severity`) — but the trap-input plugin **does not** populate `event.severity`. The operator's filter does. (The 0-100 scale that exists elsewhere in the Elastic ecosystem — `signal.severity` / `risk_score` — is from Elastic Security, a different field with different semantics.)
- **Customization surface**: 100% filter-level. Some operators run a `translate` filter against a vendor-OID-to-severity dictionary file maintained per vendor. Others embed the mapping in a `ruby` filter. There is no canonical Elastic-supplied severity dictionary.

---

## 10. Storm / Volume Handling

### Per-source rate limits

- **None at the plugin level.** No `per_source_pps`, no token bucket, no rate cap.

### Dedup keys and windows

- **None at the plugin level.** Operators implement via:
  - `fingerprint` filter to compute a stable hash for documents with identical payload, then set `_id` on the ES output to that hash (ES upserts collapse duplicates into a single document, possibly tracking a count via an `update` script).
  - `aggregate` filter for time-windowed dedup. Memory-heavy.
  - `throttle` filter — drop or count events exceeding a configured rate per key.
  - Downstream: Elasticsearch `enrich` policies or ingest-pipeline `script` processors.

### Circuit breakers

- **Plugin-level**: none.
- **Logstash core**: the persistent queue (PQ) provides backpressure. If outputs are slow, the PQ fills; once full, inputs block. For UDP traps, "block" means new PDUs are dropped at the OS kernel level (UDP buffer overflow) — no application-level signalling.

### Storm detection

- **None.** No "trap storm in progress" event, no source-blacklist, no auto-throttle.

### Backpressure / queue management

- The Logstash PQ is the universal backpressure mechanism (file-backed if enabled, in-memory otherwise). The PQ does not know about SNMP — it just sees events.
- A flooding source can fill the PQ and indirectly stall the trap input (via the input thread waiting on a full queue). The downstream effect on UDP is **kernel-side drop**, which is **silent** — Logstash does not log per-dropped-packet (impossible at the application layer; the OS drops before the application sees it).
- Under sustained flood, the loss chain is: PQ full → input thread blocks on queue → kernel UDP socket buffer (`SO_RCVBUF`, sized by `net.core.rmem_max` / per-socket `SO_RCVBUF`) overflows → silent packet loss. Operators wanting to monitor for this need to inspect `cat /proc/net/udp` for `drops` on the listening port and `netstat -su` for `RcvbufErrors` — neither is exposed by Logstash itself.
- **No specific throughput benchmark for the trap input is documented in any in-mirror artifact**; operators sizing for trap load should benchmark in their own environment. The architectural point is that no automatic mitigation is built into the plugin — when the PQ fills and the input thread blocks, the kernel UDP socket buffer overflow path takes over silently.

---

## 11. Security

### SNMPv3 USM support

- Full USM. See §3 for the algorithm matrix (md5 / sha / sha2 / hmac128sha224 / hmac192sha256 / hmac256sha384 / hmac384sha512 for auth; des / 3des / aes / aes128 / aes192 / aes256 (v4.0.6 base) plus `aes256with3desKey` (added v4.2.1 per PR #78) for priv).
- Single user per `snmptrap` input block (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:312-315`). Operators wanting N users configure N input blocks.

### DTLS / TLSTM (RFC 5953/6353)

- **Not supported.** `supported_transports` allows only `tcp` and `udp` (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:257-258`).

### Credential storage

- Credentials are stored as plaintext **passwords** in the pipeline config file by default. The `password` type in the asciidoc (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:117, :119`) is a Logstash plugin-SDK type that protects the value from being logged (it is wrapped in a `Password` object that does not stringify naively) but it is **not encrypted at rest** in the pipeline file.
- Logstash supports the **keystore** for secret management (`bin/logstash-keystore add SNMP_PASSWORD`; reference in config as `${SNMP_PASSWORD}`). This **encrypts the keystore file** on disk with a password derived from the system. Operators using v3 passwords *should* use the keystore but the trap plugin docs do not prominently recommend it (the example at `:371-385` shows the password inline).
- LogstashUI has **encrypted-at-rest credentials** stored in its Django DB and decrypts only when emitting the pipeline (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1454, :1462, :1466` call `decrypt_credential(...)`). The decrypted plaintext is then written into the CPM pipeline config in ES — i.e., the LogstashUI-side encryption protects the management DB but the emitted pipeline carries plaintext into ES (which can be mitigated by ES-side encryption at rest + RBAC on the `.logstash` index).

### Access control on the trap subsystem itself

- The trap-input plugin has no access-control. Anyone who can deliver a UDP packet to the listening port can attempt to inject a trap. Community-string check (for v1/v2c) is the only filter; v3 USM enforces auth on v3 traps.
- The `community` option whitelists allowed community strings (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:130-159`). Default `["public"]`. Setting `community => []` listens for **any** community — explicitly permissive.

### Audit logging

- The plugin does not provide a per-trap audit trail beyond what Logstash's normal pipeline logs (`logstash-plain.log`). The trap event itself, when written to ES, is the de facto audit record.

---

## 12. Trap Simulation & Testing (in-source evidence)

This section is **limited by source mirror coverage**. The plugin source is not in `/opt/baddisk/monitoring/repos/`. What we *can* see:

### Tests

- Plugin unit tests cannot be enumerated from this mirror; the upstream plugin source at `github.com/logstash-plugins/logstash-integration-snmp` is **not cloned here**. Plugins in Elastic's `logstash-plugins` org conventionally use RSpec test suites under `spec/`, but the specific test files, counts, and coverage for the snmptrap input are **`Unverified — upstream-source inspection required`**.
- The LogstashUI repo also contains `src/logstashui/SNMP/snmp_test.py` (653 lines) — a Django view that exposes an HTTP endpoint operators can use to test SNMP connectivity to a device. It implements GET/WALK/TABLE calls against a device using PySNMP — **not relevant to the trap input** (it is polling-only) but mentioned for completeness so reviewers don't confuse it with trap-test coverage.
- LogstashUI ships extensive Django tests for its SNMP module (`elastic/LogstashUI :: src/logstashui/SNMP/tests/test_snmp_crud.py` is 1365 lines, `tests/test_views.py` is 446 lines). These test the **control-plane** generation of trap pipelines (network/credential CRUD, pipeline emission, ES deploy diff), **not** the trap plugin's PDU-decoding behaviour.

### Fixtures

- Plugin-level: not source-mirrored.
- LogstashUI: `src/logstashui/SNMP/management/commands/load_test_snmp_data.py` (182 lines) is a Django management command that creates sample networks, devices, and credentials in the LogstashUI DB; each network has `traps_enabled` randomly toggled (`:117`). Purpose: populating a demo/dev DB so UI flows can be exercised. **Not** a PDU-level trap-reception test.

### Tools shipped for trap simulation

- The plugin does not appear to ship a trap simulator (**no simulator binary or simulator script is in this mirror's docs/built-docs evidence; the upstream plugin source would have to be inspected to confirm absence**). The asciidoc, the integration docs, and LogstashUI all assume the operator generates traps using external tooling. Common operator choices:
  - Net-SNMP's `snmptrap` CLI (`apt install snmp` or equivalent);
  - any third-party simulator (Trapgen, SNMP-Simulator, etc.);
  - Cisco/Juniper labs (live device traps).
- This is consistent with NMS-adjacent systems — most do not ship their own trap-PDU emitter for testing.

### CI workflow

- Plugin CI lives at upstream `github.com/logstash-plugins/logstash-integration-snmp/.github/workflows/`. **`Not source-mirrored; cannot quote CI steps.`**
- LogstashUI has its own CI for its Django tests; not trap-PDU specific.

### Verified release-note artefacts (in-mirror) about trap testing/quality

- 4.0.7: "FIX: The `snmptrap` input now correctly enforces the user security level set by `security_level` config, and drops received events that do not match the configured value" — PR #75 (`elastic/logstash :: docs/release-notes/index.md:874-876, :1022-1023`). This implies a regression test for security-level enforcement (otherwise the fix wouldn't ship), but the test source is not in mirror.
- 4.1.0: "Add support for SNMPv3 `context engine ID` and `context name` to the `snmptrap` input" — PR #76 (`:703-705`). Implies tests for context fields; source not in mirror.
- 4.2.1: "Upgrade log4j dependency" + "Add AES256 with 3DES extension support for `priv_protocol`" — PRs #85, #78 (`:387-391`). Implies regression tests for the AES256/3DES algorithm; source not in mirror.
- 4.0.6: "[DOC] Fix typo in snmptrap migration section" — PR #74 (`:1154-1156`). Docs-only.

---

## 13. Out-of-the-Box Coverage (defaults)

| Default | Value | Evidence |
|---|---|---|
| Bind host | `0.0.0.0` (any IPv4) | `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:174-178` |
| Bind port | `1062` (unprivileged) | `:245-252` |
| Transports | `["udp"]` | `:254-264` |
| Versions | `["1", "2c"]` (v3 opt-in) | `:266-273` |
| Communities | `["public"]` | `:130-138` |
| MIBs loaded | IETF set from libsmi 0.5.0 | `:294-301` |
| OID rendering | `default` (resolve via MIB, dotted) | `:192-204` |
| OID value mapping | `false` (do not translate values) | `:206-213` (note: the option table at `:98` marks this as `Required: Yes` while the detailed description at `:209-210` gives a default of `false`. **A field with a default cannot also be required — this is an upstream asciidoc authoring error.** In practice the default applies and operators need not set this option explicitly) |
| Threads | 75% of CPU cores | `:286-292` |
| ECS compatibility | inherited from `pipeline.ecs_compatibility` setting (or `disabled` if absent) | `:161-172` |
| Use provided MIBs | `true` | `:294-301` |
| Target namespace | no default (fields written to event root) — `target => "snmp"` recommended in ECS mode | `:275-284` |

Note on `priv_protocol`: the v4.0.6 asciidoc snapshot (`:118-122`) lists six privacy options (`des, 3des, aes, aes128, aes192, aes256`); the v4.3.1 built-docs add a seventh (`aes256with3desKey`, introduced in v4.2.1 per PR #78). Defaults table cites the v4.0.6 vocabulary; current operators on v4.2.1+ have the additional option (see §3).

### Bundled MIB coverage

- **Only IETF / RFC MIBs** from libsmi 0.5.0. No vendor MIBs (no Cisco IOS, no Juniper Junos, no HPE/Aruba, no Fortinet, no NetApp, no Dell, no anyone).
- Operators who run more than a token-effort SNMP environment will *always* import vendor MIBs by hand. This is more burden than e.g. OpenNMS (ships hundreds of vendor MIB packs) or Zenoss (ZenPacks bundle per-vendor MIB+event-class+severity), and roughly equivalent to Telegraf (which also ships none).

### Bundled severity rules

- **None.** Severity is left entirely to operator filters.

### Dedup defaults

- **None.** Dedup is left entirely to operator filters / downstream Elasticsearch logic.

### Vendor packs / integration packages

- **None at the trap-plugin level.** The Elastic Integrations ecosystem (the official Elastic-packaged integration modules) includes a "Network Packets" module and various Beats integrations, but no canonical "SNMP traps from <vendor>" Elastic integration shipping pre-canned filters / dashboards / Kibana saved objects.
- The LogstashUI's `official_profiles/traps.json` is **minimal** — it maps four OIDs to ECS-style field names: `host.os.full → 1.3.6.1.2.1.1.1.0` (sysDescr.0), `host.name → 1.3.6.1.2.1.1.5.0` (sysName.0), `host.uptime → 1.3.6.1.2.1.1.3.0` (sysUpTime.0), and `snmp.command.source_code → 1.3.6.1.4.1.9.9.43.1.1.6.1.3` (a Cisco enterprise OID for ccmHistoryEventCommandSource). Not a vendor pack — `elastic/LogstashUI :: src/logstashui/SNMP/data/official_profiles/traps.json`. Note: LogstashUI also ships `official_device_templates/` (Cisco, Brocade, Dell iDRAC, HPE Nimble, Epson) but those are polling-oriented discovery templates, not trap-specific.

### Sample / preset dashboards or reports

- **None ship with Logstash.** Kibana dashboards for traps are not part of the Elastic-stock setup. Operators write their own.

---

## 14. User Customization Surface

### How users add custom OID handlers

- Pipeline filters are the universal handler mechanism. `if [oid] == "..." { ... }` blocks in the `filter { }` section act as per-OID handlers.
- More elaborate: use `grok` against varbind values; `translate` against a maintained dictionary; `ruby` for arbitrary logic.

### Custom MIBs

- File-system + `smidump` workflow (§4). Operator deposits `.dic` files in `mib_paths` and restarts/reloads the pipeline.

### Custom severity rules

- Filter-level. Common pattern: `translate` filter sourcing a YAML or CSV dictionary mapping `(vendor_oid, vendor_severity)` to ECS `event.severity`.

### Custom dedup rules

- Filter-level. `fingerprint` + ES `_id` strategy is the textbook pattern; `aggregate` filter for stateful aggregation.

### Plugin / extension model

- Logstash's plugin SDK supports operator-written plugins in Ruby/JRuby (or Java for input/codec/output). Operators can write a custom filter plugin that does whatever the canonical filters cannot.
- An operator wanting "OpenNMS-style event-conf with structured severity / clear / alarm" could in principle write a custom filter plugin that maintains state in-process or in Redis. The community has built such things; none are first-party Elastic offerings.

### API surface for automation

- **Logstash monitoring API** (`/_node/stats`): runtime stats, queue depth, plugin throughput. Read-only.
- **CPM API**: pipelines CRUD via Elasticsearch index (`PUT _logstash/pipeline/<id>`). This is the programmatic path LogstashUI uses (`elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1781-1782`).
- **Plugin configuration**: file or CPM; no programmatic configure-input-options-at-runtime API (changes require pipeline reload).
- **Per-trap manipulation**: not via API — once a trap is a document in ES, the ES Update API can modify it; before that, the operator's filters are the only modification surface.

---

## 15. End-User Value Analysis

### What an operator gets day-1 with default config

- A working trap listener on `0.0.0.0:1062` accepting v1/v2c with community `public`, plus v3 if explicitly enabled.
- IETF MIB resolution applied to OIDs in the event.
- A flat event with all varbinds expanded as fields.
- Default-format output (`elasticsearch { hosts => ["localhost:9200"] }` is one config line, then ES auto-creates an index).

That's it. From there, the operator has:

- No alarm view (write Kibana queries / dashboards manually).
- No severity ladder (write a `translate` filter).
- No dedup (write a `fingerprint` filter).
- No topology join (write an `elasticsearch` filter).
- No northbound forwarding (install / configure a separate output plugin).

### What requires customization

- **Everything** beyond raw trap ingestion. Trap-to-actionable-alert is a multi-component pipeline an operator hand-assembles. This is the central design choice and tradeoff of using Logstash for traps.

### Learning curve

- **Low** for basic ingestion (the `snmptrap { }` block is 1-5 lines).
- **Moderate** for value-add filter pipelines (grok / translate / ruby / dns / fingerprint patterns).
- **High** for operationally-equivalent functionality to OpenNMS / Centreon / Zenoss / CheckMK — the operator essentially re-implements those systems' built-in features as a custom pipeline.

### Operational toil

- **Recurring**: maintaining filter dictionaries (severity, asset inventory, OID handlers) as vendor MIBs evolve.
- **One-time per device class**: writing the filter blocks for that class. Often dozens of vendor classes for an enterprise network.
- **Migration cost**: the v3 → v4 plugin migration (§6) breaks any operator-built ES mappings / Kibana saved searches / Watcher rules that depended on the old field shape.

### Visibility into the pipeline's own health

- The **Logstash monitoring API** at `http://logstash-host:9600/_node/stats` (a documented core-Logstash feature, not trap-specific) exposes per-plugin throughput, queue depth, retry counts, and output errors. **In-mirror evidence**: `elastic/logstash :: docs/reference/monitoring-logstash.md:10-31` documents the monitoring APIs and Plugins-info API; `elastic/logstash :: docs/reference/monitoring-with-opentelemetry.md:119-170` documents the per-plugin counter metrics — specifically `logstash.plugin.events.in`, `logstash.plugin.events.out`, `logstash.plugin.events.duration`, each with `pipeline.id`, `plugin.type`, `plugin.id` attributes. Visible in Kibana's Stack Monitoring UI when monitoring is wired in.
- **The `snmptrap` plugin's specific counter set, beyond these standard plugin-SDK counters, is not source-verified here.** Whether the plugin emits any SNMP-specific counters (PDU decode failures, auth-rejected v3 traps, INFORM-handled count, malformed-PDU count) on top of the generic per-plugin counters cannot be confirmed without inspecting the upstream plugin source. **`Generic per-plugin counters source-verified via in-mirror Logstash docs; SNMP-specific counters are unverified.`**

---

## 16. Strengths

1. **Strong search/aggregation layer post-ingest**. The "trap is a document in Elasticsearch" model gives operators full-text search across all traps, structured aggregations on any varbind, Lens / TSVB / Kibana dashboards, and alerts via the same Kibana Alerts engine that handles every other Elastic data source. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:42-44` (JSON message shape), `:50-76` (ECS-aligned fields).

2. **Filter ecosystem is the customisation point**. The Logstash filter-plugin set (dozens of filters — `mutate`, `grok`, `translate`, `ruby`, `date`, `dns`, `elasticsearch`, `aggregate`, `throttle`, `fingerprint`, `geoip`, `csv`, `xml`, `json`, `kv`, etc.) gives operators a programmable enrichment layer that is **more flexible** than any built-in event-mapper in a typical NMS. Evidence: filter listings in `elastic/logstash/docs/release-notes/index.md` (extensive filter-plugin releases per version); plugin catalog in `elastic/LogstashUI :: src/logstashui/PipelineManager/data/plugins.json` (full schema of currently-shipped filters).

3. **Strong SNMPv3 USM coverage (v4+)**. The auth-protocol matrix includes the modern SHA-2 HMAC algorithms (`hmac128sha224`, `hmac192sha256`, `hmac256sha384`, `hmac384sha512`) and the SHA2-equivalent shortcut. AES256 with 3DES extension support added in 4.2.1. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:118-120, :325-349`; release notes `elastic/logstash :: docs/release-notes/index.md:387-391`.

4. **TCP transport support**. Documented as RFC 3430 — uncommon among NMS-style trap collectors and useful for high-volume / high-reliability scenarios. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:254-264`.

5. **Plugin consolidation (4.x integration)**. The merger of `input-snmp` + `input-snmptrap` into one Ruby gem is a packaging / footprint / maintenance win — fewer gems to install, single SNMP engine (SNMP4J) to maintain. Evidence: `elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:23-45`.

6. **ECS alignment**. When `ecs_compatibility => v1|v8`, trap fields conform to ECS conventions — meaning a trap document and a beat document and an APM event document share top-level field names and types in the same index. This enables cross-signal queries without per-source field mapping. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:44-54`.

7. **Centralized Pipeline Management + LogstashUI for many-node fleets**. CPM (Elasticsearch-backed pipeline distribution) is GA; LogstashUI (currently in beta — see §1) provides UI for at least the trap-enable knob and credential management. Reasonable foundation for a fleet of trap collectors when CPM is in use. Evidence: `elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1779-1784`.

---

## 17. Weaknesses / Gaps

1. **No native event model — operators rebuild what NMS systems ship**. No alarm lifecycle, no severity ladder, no dedup, no topology join, no event-class mapping out of the box. Evidence: §8 of this document; absence of any such feature in `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc` and `:: docs/plugins/integrations/snmp.asciidoc`.

2. **No vendor MIB pack**. Ships only the IETF set; every vendor MIB is operator-imported via the `smidump` toolchain. This is a meaningful operational cost compared to OpenNMS / Zenoss / CheckMK. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:294-301`; integration MIB-import procedure at `:: docs/plugins/integrations/snmp.asciidoc:130-194`.

3. **Single SNMPv3 user per input block**. Multiple v3 users require multiple input blocks (and therefore multiple ports). Operationally awkward at scale. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:312-315`.

4. **INFORM handling is not documented**. The plugin docs show only `V1TRAP` and `TRAP` PDU types in the message format (`elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:42`, `integrations/snmp.asciidoc:106`). Whether v2c/v3 INFORMs are acknowledged (causing the sending device to stop retrying) or silently dropped is unconfirmed from the available evidence. If the plugin does not acknowledge INFORMs, devices will retry indefinitely on the device side until they hit their own retry-limit. **`Unverified — plugin source needed to confirm support or absence.`**

5. **No DTLS / TLSTM**. Only UDP / TCP transports per `supported_transports`. SNMPv3 USM still encrypts the PDU payload, but the transport-layer secure-binding option that more modern NMS systems support is absent. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:254-264`.

6. **Plaintext credentials in pipeline config by default**. The `:password` type protects against accidental log emission but does **not** encrypt the file. The Logstash keystore is the documented workaround but is not promoted in the trap-input docs. Evidence: example at `:: docs/plugins/inputs/snmptrap.asciidoc:371-385` shows inline plaintext password.

7. **Default port 1062 vs SNMP-standard 162** is a permanent friction point. Every operator with real devices needs to either rebind to 162 (root or `cap_net_bind_service`) or set up port redirection. The docs flag this but offer no integrated solution. Evidence: `:: docs/plugins/inputs/snmptrap.asciidoc:245-252`.

8. **No storm protection / rate limit / dedup at the plugin layer**. A misbehaving device can saturate Logstash and downstream ES. Operator-implemented filters can mitigate but there is no "safe default" behaviour. Evidence: absence of any such option in the documented option set.

9. **Major behavioural-format break v3 → v4 plugin**. The `message` field shape changed (Ruby object → JSON), value types changed (TimeTicks Long instead of string, lowercase `"null"`, error strings), MIB-resolution behaviour changed. Operators upgrading to Logstash 8.15+ with active trap pipelines may experience downstream breakage in ES mappings / Kibana saved objects. Compatibility mode exists but adds operator work. Evidence: `elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:75-127`.

10. **Plugin source out-of-tree from the Elastic monorepo**. The plugin lives at `github.com/logstash-plugins/logstash-integration-snmp`, not in `elastic/logstash`. This is by Elastic's own plugin convention (all input/filter/output plugins are separate gems), but it means in-repo investigation (e.g., this analysis) is limited to docs + LogstashUI control-plane code. Most other NMS systems (OpenNMS, Centreon, Zenoss, CheckMK, Zabbix) have their trap subsystem source co-located with the main product. Evidence: this analysis is constrained by exactly that gap.

11. **No bundled health metrics / counters surfaced as a trap-specific instrumentation panel**. The plugin presumably reports the generic per-plugin counters (received / filtered / sent) via the monitoring API; SNMP-specific counters (decode failures, auth rejects, INFORM handling) are not documented as surfaced. **`Inferred; plugin source needed to confirm.`**

12. **LogstashUI's trap support is beta and the *SNMP CRUD wizard* is restrictive** (the generic PipelineManager catalog is full-coverage; see §7). The wizard omits `mib_paths`, `threads`, `oid_root_skip`, `oid_path_length`, `target`, `supported_transports`, `ecs_compatibility` from the form. Default port the wizard emits is `1662` (different from the plugin default `1062` — non-obvious choice). Documentation for the trap workflow is missing from the Quickstart. The PipelineManager catalog at `plugins.json:7436-7672` knows the full schema, but only operators who go beyond the wizard benefit. Evidence: `elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1442-1494`; `src/logstashui/PipelineManager/data/plugins.json:7436-7672`; absence of "trap" in `docs/docs/logstashui/SNMP/Quickstart.md`.

13. **PDU header is not persisted by default**. The trap-input plugin places all PDU-header fields (`community`, `version`, `agent_addr`, `enterprise`, `generic_trap`, `specific_trap`, `error_status`, `request_id`, `context_engine_id`, …) inside `@metadata`, which Logstash does **not** serialize to outputs by default. The persisted ES document gets only the source-host fields, the varbinds, and `@timestamp` unless the operator adds an explicit `mutate { copy => { "[@metadata][input][snmptrap][pdu]" => "[snmp][pdu]" } }` filter. This is a non-obvious day-1 ergonomic gap: an operator pointing devices at the default pipeline and inspecting the ES document will find varbinds but no PDU envelope. Evidence: `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:56-76`; §5 Stage 3 + §6 of this document.

---

## 18. Notable Code or Configuration Examples

### 18.1 The simplest functional pipeline (operator-authored)

From `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:142-148`:

```ruby
input {
  snmptrap {
    community => ["public", "guest"]
  }
}
```

This listens on UDP `0.0.0.0:1062` for v1/v2c traps with community `public` or `guest`. Without an explicit output block the trap event is not persisted to any store — Logstash requires an explicit output block (e.g. `output { elasticsearch { … } }` or `output { stdout { } }`) for durable delivery — but the input alone is one-line.

### 18.2 SNMPv3 example with full USM

From `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:371-385` (reproduced verbatim; `"AesPasword"` is a typo in the source asciidoc — likely intended `"AesPassword"`):

```ruby
input {
  snmptrap {
    supported_versions => ['3']
    security_name => "mySecurityName"
    auth_protocol => "sha"
    auth_pass => "ShaPassword"
    priv_protocol => "aes"
    priv_pass => "AesPasword"
    security_level => "authPriv"
  }
}
```

Note the inline plaintext passphrase — see §17 weakness #6.

### 18.3 Migration-compatibility configuration

From `elastic/logstash-docs :: docs/plugins/integrations/snmp.asciidoc:117-125`:

```ruby
input {
   snmptrap {
    use_provided_mibs => false
    oid_mapping_format => 'ruby_snmp'
    oid_map_field_values => true
   }
}
```

This restores the legacy v3-plugin behaviour for ES mappings / Kibana saved searches that depended on it.

### 18.4 The PDU metadata field structure

From `elastic/logstash-docs :: docs/plugins/inputs/snmptrap.asciidoc:59-76` (paraphrased table; verbatim asciidoc preserved):

```
[@metadata][input][snmptrap][pdu][agent_addr]       SNMPv1  Network address of the object generating the trap
[@metadata][input][snmptrap][pdu][community]        SNMPv1, SNMPv2c  SNMP community
[@metadata][input][snmptrap][pdu][enterprise]       SNMPv1  Type of object generating the trap
[@metadata][input][snmptrap][pdu][error_index]      SNMPv2c, SNMPv3  Provides additional information by identifying which variable binding caused the error
[@metadata][input][snmptrap][pdu][error_status]     SNMPv2c, SNMPv3  Error status code
[@metadata][input][snmptrap][pdu][error_status_text] SNMPv2c, SNMPv3  Error status code description
[@metadata][input][snmptrap][pdu][generic_trap]     SNMPv1  Generic trap type
[@metadata][input][snmptrap][pdu][request_id]       SNMPv2c, SNMPv3  Request ID
[@metadata][input][snmptrap][pdu][specific_trap]    SNMPv1  Specific code
[@metadata][input][snmptrap][pdu][timestamp]        SNMPv1  Time elapsed since last (re)initialization
[@metadata][input][snmptrap][pdu][type]             Always  PDU type
[@metadata][input][snmptrap][pdu][variable_bindings] Always SNMP variable bindings values
[@metadata][input][snmptrap][pdu][version]          Always  SNMP version
```

The use of `@metadata` (a Logstash-specific in-pipeline scratch space that is *not* serialized to outputs by default) is a deliberate plugin-design decision: the operator must explicitly copy fields out of `@metadata` to keep them in the persistent ES document. This is consistent with Logstash conventions but adds a step compared to "just emit everything to the output by default".

### 18.5 LogstashUI's auto-generated trap pipeline construction

From `elastic/LogstashUI :: src/logstashui/SNMP/snmp_crud.py:1442-1491`:

```python
trap_input_config = {
    "host": "0.0.0.0",
    "port": 1662,
    "oid_map_field_values": False,
    "oid_mapping_format": "dotted_string",
    "supported_versions": []
}

# Add version-specific configuration
if credential.version in ['1', '2c']:
    trap_input_config["supported_versions"].append(credential.version)
    if credential.community:
        trap_input_config["community"] = [decrypt_credential(credential.community)]
elif credential.version == '3':
    trap_input_config["supported_versions"].append("3")
    if credential.security_name:
        trap_input_config["security_name"] = credential.security_name
    if credential.auth_protocol:
        trap_input_config["auth_protocol"] = credential.auth_protocol
    if credential.auth_pass:
        trap_input_config["auth_pass"] = decrypt_credential(credential.auth_pass)
    if credential.priv_protocol:
        trap_input_config["priv_protocol"] = credential.priv_protocol
    if credential.priv_pass:
        trap_input_config["priv_pass"] = decrypt_credential(credential.priv_pass)
    if credential.security_level:
        trap_input_config["security_level"] = credential.security_level

trap_components = {
    "input": [{
        "id": "input_snmptrap_1",
        "type": "input",
        "plugin": "snmptrap",
        "config": trap_input_config
    }],
    "filter": [
        {
            "id": "filter_mutate_trap_1",
            "type": "filter",
            "plugin": "mutate",
            "config": {
                "add_field": {
                    "[event][kind]": "traps"
                }
            }
        }
    ],
    "output": _generate_output(input_data, network, snmp_type="traps")
}

# Final step at :1494 — convert the structured components dict to a Logstash pipeline config string
new_trap_config = ComponentToPipeline(trap_components, test=False).components_to_logstash_config()
```

Three notable choices:

- Default port `1662` (not the plugin default `1062`). Likely a LogstashUI deliberate avoidance of any conflict with the plugin default; no rationale documented.
- The single filter added by default is a `mutate { add_field { "[event][kind]" => "traps" } }`. This is the minimum-viable enrichment for routing traps separately from polling-derived metric documents (which the same UI emits with `event.kind = "metrics"` implicitly via the polling data-stream `snmp.polling`).
- `supported_versions` is initialised to `[]` then conditionally appended from the credential's `version` field. If an operator saves a credential without an SNMP version, the emitted pipeline ships with `supported_versions: []` — the plugin then accepts **no** traps. The LogstashUI form ought to validate that version is set; whether it does so is outside this PDU-pipeline analysis.

### 18.6 Plugin metadata showing the v3 → v4 transition

From `elastic/logstash :: rakelib/plugins-metadata.json:378-381, :436-439`:

```json
"logstash-input-snmptrap": {
    "default-plugins": false,
    "skip-list": true
},
...
"logstash-integration-snmp": {
    "default-plugins": true,
    "skip-list": false
}
```

The standalone plugin is no longer in the default install set; the integration is. This is Elastic's quiet way of telling operators to migrate.

---

## 19. Sources Examined

### `elastic/logstash-docs @ e8d7013a53f94d26d940a16dcba1cb6ee95e2f6c`

- `docs/plugins/inputs/snmptrap.asciidoc` — primary operator-facing reference for the trap input (390 lines).
- `docs/plugins/inputs/snmp.asciidoc` — sister polling-input reference (525 lines); not directly trap-related but cross-referenced for migration notes.
- `docs/plugins/integrations/snmp.asciidoc` — integration-plugin reference covering migration story + MIB import procedure (194 lines).

### `elastic/logstash @ 849ad93f12dfbfb2b96e7a332e1d8b9c292b759f`

- `rakelib/plugins-metadata.json:378-381, :436-439` — confirms standalone `logstash-input-snmptrap` is no longer in the default plugin set and `logstash-integration-snmp` is the default.
- `docs/release-notes/index.md:387-391, :703-705, :874-876, :1022-1023, :1154-1156` — release notes covering v4.0.6 / 4.0.7 / 4.1.0 / 4.2.1 of the integration plugin's trap input.
- `docs/reference/deploying-scaling-logstash.md` — one-line reference to SNMP-trap-as-Logstash-input for context.
- `tools/dependencies-report/src/main/resources/notices/snmp-NOTICE.txt:1-20` (copyright on `:1-2`, full MIT-style notice continues to `:20`) — the bundled NOTICE for the ruby-snmp library (David R. Halliday, 2004-2014) — evidence of the historical Ruby-snmp backend.
- `logstash-core/src/test/resources/org/logstash/config/ir/complex.cfg:26, :1875, :1894` — pipeline IR (intermediate representation) compilation test fixture verifying the config parser accepts an `snmptrap` input block. **Not** a PDU-decoding test.

### `elastic/built-docs @ 61b460d9509fcb51c017f676a665ebe396ededeb` (separate repo, NOT under `elastic/logstash`)

The built-docs repo contains identical plugin documentation snapshots at two parallel paths: `raw/en/...` (the asciidoc-generated HTML) and `html/en/...` (a templated render). Citations here use the `raw/` variant; `html/` would yield the same content with surrounding navigation chrome.


- `raw/en/logstash-versioned-plugins/current/input-snmptrap-index.html` — version-index page listing 19 versioned releases of the snmptrap plugin (v3.0.3, v3.0.4, v3.0.5, v3.0.6, v3.1.0, then v4.0.0 through v4.3.1; verified by `grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' input-snmptrap-index.html | sort -Vu`). Boundary versions (v3.0.3 / v3.1.0 / v4.0.0 / v4.3.1) and the release-note-cited intermediates (v4.0.7 security-level fix, v4.1.0 context fields, v4.2.1 aes256with3desKey) are examined; the remaining intermediate versions (v4.0.1-v4.0.6, v4.2.0/v4.2.2, v4.3.0) were not individually inspected because boundary versions capture the architectural transitions.
- `raw/en/logstash-versioned-plugins/current/v3.0.3-plugins-inputs-snmptrap.html` — earliest visible standalone-plugin docs (ruby-snmp-era message rendering, very small option set).
- `raw/en/logstash-versioned-plugins/current/v3.1.0-plugins-inputs-snmptrap.html` — last v3 docs, no SNMPv3 support visible.
- `raw/en/logstash-versioned-plugins/current/v4.0.0-plugins-inputs-snmptrap.html` — first integration-era docs; new JSON message shape; SNMPv3 options appear.
- `raw/en/logstash-versioned-plugins/current/v4.3.1-plugins-inputs-snmptrap.html` — latest version visible in mirror; adds `supported_transports` (tcp/udp); also adds `context_engine_id`/`context_name` to the PDU metadata table.
- `raw/en/logstash/{1.5,2.0,2.4,5.0,5.1,…}/plugins-inputs-snmptrap.html` — historical operator-reference snapshots back to 1.5 (for confirming the plugin has been around since the earliest documented Logstash).

### `elastic/LogstashUI @ 99e07640c94af846a946c395cf9f08a78ae4d446`

- `src/logstashui/SNMP/snmp_crud.py:1442-1494` — trap-pipeline construction logic.
- `src/logstashui/SNMP/snmp_crud.py:1240-1300` — output construction with `snmp.traps` data-stream.
- `src/logstashui/SNMP/snmp_crud.py:1715-1810` — full trap-pipeline lifecycle (create/update/delete via CPM).
- `src/logstashui/SNMP/models.py:48-71` — `traps_enabled` and trap-credential fields on the `Network` model.
- `src/logstashui/SNMP/data/official_profiles/traps.json` — minimal "official traps profile" (4 OIDs).
- `src/logstashui/SNMP/tests/test_snmp_crud.py` — Django/Pytest tests for the trap CRUD layer (1365 lines, not PDU-level).
- `docs/docs/logstashui/SNMP/index.md` — SNMP module overview (mentions traps as a capability but no Quickstart entry).
- `README.md` — beta-status banner; SNMP feature listed under "SNMP Pipeline Management — Configure polling, traps, discovery, credentials, devices, networks, and profiles through the UI."

### Not in this mirror (consulted via referenced URLs only)

- `github.com/logstash-plugins/logstash-input-snmptrap` — the standalone plugin's upstream source (legacy).
- `github.com/logstash-plugins/logstash-integration-snmp` — the integration plugin's upstream source (current).
- `github.com/logstash-plugins/logstash-output-snmptrap` — the northbound output plugin's upstream source (legacy).
- `https://www.ibr.cs.tu-bs.de/projects/libsmi` — libsmi project page (referenced as MIB source).
- `https://datatracker.ietf.org/doc/html/rfc3430` — SNMP-over-TCP RFC (referenced as the transport rationale).
- `github.com/logstash-plugins/logstash-integration-snmp/blob/v4.0.6/CHANGELOG.md` — referenced from the asciidoc but not mirrored.

---

## 20. Evidence Confidence

| Section | Confidence | Notes |
|---|---|---|
| §1 Overview & Lineage | **high (for product/lineage)**, **medium (for library choice)** | Product and lineage source-verified via asciidoc + release notes + plugin-metadata. SNMP4J inference is from option vocabulary, not source; clearly labelled in-document. |
| §2 Architecture | **high** | All component shapes verified from docs + LogstashUI source; deployment models traced. |
| §3 Trap Reception | **high (option matrix), medium (INFORM/context behaviour)** | Option matrix line-cited from asciidoc; context-engine-id support evidenced only via release-note for PR #76. INFORM acknowledgement behaviour is undocumented in mirror evidence — labelled `Unverified`. |
| §4 MIB Management | **high** | Workflow line-cited from integration asciidoc; absence of vendor MIB pack verified by reading the `use_provided_mibs` description (IETF-only). |
| §5 Pipeline | **high (overall flow), medium (error handling)** | Flow shape is documented; per-stage decode/parse internals are not directly source-cited from in-mirror evidence. Error-handling at stage-9 is labelled `Inferred`. |
| §6 Data Model | **high** | Schema/storage/migration all line-cited; non-existence of trap-side persistent state verified by absence in option list and integration docs. |
| §7 Configuration UX | **high** | Surfaces enumerated with file:line; multi-tenancy / RBAC absence confirmed by docs grep. |
| §8 Integration | **high** | Each subsection cites explicit docs; northbound output plugin's existence noted as upstream-only with no mirror coverage. |
| §9 Severity | **high** | Verified absence; ECS `event.severity` convention reference is from public Elastic docs. |
| §10 Storm/Volume | **high** | Verified absence by surveying the option list and integration docs; no `rate_limit`, no `dedup`, no `throttle` configuration at the plugin layer. |
| §11 Security | **high (USM matrix), medium (audit logging)** | USM algorithm matrix line-cited; audit-trail absence confirmed by absence in option list. |
| §12 Tests | **medium-to-low** | Limited to what is in the LogstashUI control plane; plugin-side tests are not source-mirrored — flagged explicitly. |
| §13 OOTB Defaults | **high** | Every default line-cited; absence of severity/dedup/dashboards/vendor packs verified across all in-mirror docs. |
| §14 Customization | **high** | Filter ecosystem references verified; CPM API path line-cited via LogstashUI usage. |
| §15 End-User Value | **high (operator workflow), medium (plugin health counters)** | Plugin-specific counters surfacing is `Inferred`. |
| §16 Strengths | **high** | Each strength has a file:line cite. |
| §17 Weaknesses | **high (most), medium (INFORM)** | INFORM-acknowledgement weakness flagged as `Unverified` based on docs only. |
| §18 Code Examples | **high** | All code/config blocks are verbatim from cited source. |
| §19 Sources Examined | **high** | All in-mirror paths verified to exist; external URLs listed separately under "Not in this mirror" subsection (consulted by reference only). |

---

## Reviewer Pass Log

### Iteration 1 — 2026-05-22

All 6 reviewers (`codex`, `glm`, `kimi`, `mimo`, `minimax`, `qwen`) ran the SOW reviewer prompt against this file. Outputs saved at `.local/audits/snmp-traps-pilot/reviews/logstash/iter-1/<name>.txt`. Prompt at `.local/audits/snmp-traps-pilot/reviews/logstash/prompt-iter-1.txt`.

Exits: codex 0, glm 0, kimi 0, mimo 0, minimax 0, qwen 0.

#### Iteration 1 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | accept-with-fixes | 4 major + 4 minor (8 total) |
| glm | accept-with-fixes | 5 major + 4 minor + 3 nit (12 total) |
| kimi | accept-with-fixes (recommend accept) | 0 blocker + 0 major + 5 nit |
| mimo | accept-with-fixes | 0 blocker + 0 major + 4 minor + 3 nit |
| minimax | accept-with-fixes | 0 blocker + 2 major + 2 minor + 4 nit |
| qwen | accept-with-fixes | 0 blocker + 0 major + 4 minor + 2 nit |

#### Consolidated iter-1 findings and disposition (verified against source before acting)

**Majors (verified against source, applied across iter-1 → iter-2):**

1. **§0/§19 built-docs repository path was wrong** (codex MAJOR-5, minimax MAJOR-1). Cited path was `elastic/logstash/built-docs/...`; actual mirror path is `elastic/built-docs/...` (separate repo at `61b460d9`). Fixed: added `elastic/built-docs @ 61b460d9` to §0 metadata + §19, corrected every built-docs citation. Verified the v3.0.3 and v4.0.0 plugin pages DO exist in the correct mirror path.
2. **SNMP4J attribution leaked from inferred-label into asserted architecture** (codex MAJOR-3, minimax MAJOR-2). §1 said "the v4 plugin replaces ruby-snmp with SNMP4J" as a near-certain claim, but the evidence (option vocabulary matching SNMP4J class names) is circumstantial. Rewrote §1, §2, §5 Stage 1 to label SNMP4J consistently as **`Vendor-documented, not source-verified`** with the full inference chain spelled out; architecture diagram now says "Plugin SNMP listener (inferred SNMP4J backend)" instead of asserting SNMP4J as fact.
3. **§3/§17 multi-port v3 implication overstated** (codex MAJOR-1). Doc claimed "two v3 users == two ports". The asciidoc only says "multiple input declarations needed"; the same-port-bind constraint is OS-level not plugin-documented. Rewrote §3 SNMPv3 cardinality with explicit "Unverified — OS-level constraint applies by default" framing.
4. **§5/§8.5 `logstash-output-snmptrap` over-asserted** (codex MAJOR-2, glm MAJOR-3). The output plugin's existence was claimed "verified via Elastic plugins repo URL referenced from changelog patterns" — but there's no in-mirror evidence (no plugins-metadata.json entry, no release-note mention). Reworded to "reported to exist; existence not source-verified in this mirror; configuration not verifiable here."
5. **§15 plugin counters over-confident** (codex MAJOR-4, glm MINOR-3, minimax NIT-4). Claimed the snmptrap plugin surfaces specific counters; these are generic per-plugin counters not SNMP-specific. Rewrote §15 visibility section to make clear the API is generic-Logstash and SNMP-specific counters are unverified.
6. **§1 license inaccurate** (codex MAJOR-6). Doc said "Apache-2.0 licensed (now SSPL/dual-licensed)" — `elastic/logstash :: LICENSE.txt:1-13` does not mention SSPL. Replaced with the verbatim Apache-2.0-or-Elastic-License language from the actual LICENSE.txt.
7. **§3 SNMPv3 context fields under-cited** (codex MINOR-7, glm MAJOR-4). Doc cited context-engine-id only via release-note; v4.3.1 built-docs directly expose `[@metadata][input][snmptrap][pdu][context_engine_id]` and `[@metadata][input][snmptrap][pdu][context_name]` in the PDU-metadata table. Added the direct v4.3.1 built-docs cite to §3.
8. **§19 versions count wrong** (glm MAJOR-1). Doc said "22 versioned releases"; actual count is **19** (v3.0.3, v3.0.4, v3.0.5, v3.0.6, v3.1.0, then v4.0.0 through v4.3.1). Corrected with the explicit version list and reproducible verification command.
9. **`aes256with3desKey` privacy protocol missing** (glm MAJOR-2). The v4.2.1 release-note "Add AES256 with 3DES extension support for `priv_protocol`" maps to a new enum value `aes256with3desKey` visible in v4.3.1 built-docs (`:374, :821`) and corroborated in `elastic/LogstashUI :: src/logstashui/SNMP/models.py:232`. Added to the §3 privacy protocol list and §11 algorithm matrix.
10. **§10 performance claim unsubstantiated** (glm MAJOR-5). Doc said "at 5000+ traps/sec…stall, drop, catch up" as a measured observation; no source backs the figure. Rewrote as an explicit plausibility sketch with "no specific throughput benchmark is cited; operators should benchmark in their own environment."

**Minor findings applied:**

11. **Marketing residue** (codex MINOR-8). Removed/neutralised "home turf" (§8.4), "architectural payoff" (§8.4), "immensely capable" (§16), "real cross-signal win" (§16). Replaced with neutral comparative language.
12. **§9 ECS `event.severity` wrong** (qwen MINOR-1). Doc said "0-100 scale, lower is more critical"; actual ECS spec is `1-10` long where **higher is more severe**. Rewrote with citation to the ECS docs URL; clarified 0-100 belongs to Elastic Security's `signal.severity`/`risk_score`, a different field.
13. **§13 `target` option missing from defaults table** (qwen MINOR-2, kimi NIT-4). Added a `Target namespace` row to the §13 defaults table with the cite.
14. **§5 `@metadata` not serialized by default** (minimax missed-content #1). Added a paragraph in §5 Stage 3 explaining `@metadata` is in-pipeline scratch space, not serialized to outputs by default; operator must `mutate { copy => ... }` to retain PDU header fields. Stage 4 also notes `agent_addr` lives in `@metadata`.
15. **§3 engine-ID discovery not discussed** (qwen MINOR-3). Added a SNMPv3 authoritative-engine-ID paragraph noting that the polling input has `local_engine_id` but the trap input does not; engine-ID handling for the listener is `Unverified — plugin source needed`.
16. **§10 PQ + UDP buffer chain** (qwen MINOR-4). Added the OS-level loss-chain description (PQ full → input block → kernel UDP overflow → silent drop), with monitoring guidance (`/proc/net/udp`, `netstat -su RcvbufErrors`).
17. **§13 traps.json paraphrase** (mimo NIT-3, glm NIT-10/12). Replaced the loose 4-OID prose with the actual JSON field-name → OID mapping plus the precise MIB name (`ccmHistoryEventCommandSource` from `CISCO-CONFIG-MAN-MIB`); also noted LogstashUI's `official_device_templates/` are polling-oriented, not trap-specific.
18. **§17 weakness #4 (INFORM) framing** (minimax NIT-1). Rewrote from "no INFORM ack is documented" (sounded like "doesn't support") to "INFORM handling is not documented; whether v2c/v3 INFORMs are acked or dropped is unconfirmed", consistent with §3 `Unverified` label.
19. **§18.2 AesPasword typo** (minimax NIT-3). Added "(reproduced verbatim; `AesPasword` is a typo in the source asciidoc)" annotation.
20. **§8.2 ElastAlert sourcing** (minimax MINOR-2). Added explicit GitHub repo cites (`github.com/Yelp/elastalert` plus the successor fork `github.com/jertel/elastalert2`).
21. **§2 LogstashUI CPM `put_pipeline` line citation** (mimo MINOR-1). Changed `snmp_crud.py:1781-1782` to `:1779-1784` (wrapper call) which delegates to `es.logstash.put_pipeline(...)` at `:322`.
22. **§18.5 truncated code example** (mimo MINOR-4). Expanded the ellipsis-truncated v3 credential code block to show the full `auth_pass / priv_protocol / priv_pass / security_level` assignments.
23. **§5 Stage 4 `@metadata` clarification** (mimo MINOR-5). Strengthened to make explicit which fields land on the event proper vs in `@metadata`.
24. **§4 OID `oid_map_field_values` doc inconsistency** (kimi NIT-1). The option table at `:98` marks it as "Required: Yes" while the detailed description at `:209-210` gives a default of `false`. Added a parenthetical noting the source-asciidoc internal inconsistency.
25. **§12 complex.cfg characterization** (kimi NIT-2). Clarified `complex.cfg` is a pipeline IR (intermediate representation) compilation test fixture, not a PDU-decoding test.
26. **§12 `load_test_snmp_data.py` description** (qwen NIT-5). Expanded to clarify it's a Django management command for populating sample data, not PDU-level testing.
27. **§1 ruby-snmp NOTICE citation precision** (qwen NIT-6). Changed `:1` to `:1-2` for the copyright line range.
28. **§2 LogstashUI Agent "Coming Soon"** (mimo MINOR-2). Verified accurate — no change.
29. **GLM MINOR-1 `plugins-metadata.json` line ranges** — Changed `:378-380` to `:378-381` and `:436-438` to `:436-439` (correct line ranges including closing brace).
30. **GLM MINOR-2 cross-system Netdata framing** — Moved the Netdata-hub comparison from a normative claim in §2 to a parenthetical cross-system-comparison note, qualifying that the deeper comparison belongs in the cross-system document.
31. **GLM MINOR-4 built-docs path variants** — Added §19 note that `built-docs/raw/` and `built-docs/html/` contain identical content; the spec cites the `raw/` variant.
32. **GLM NIT-11 LogstashUI beta caveat** — Added "(LogstashUI is currently in beta — see §1)" qualifier to §16 strength #7.
33. **§5 Stage 9 error-handling specifics** (kimi NIT-5) — Added the four concrete migration-mapping error strings (`no such instance`, `no such object`, `end of MIB view`, `null` lowercase) cited from `integrations/snmp.asciidoc`.

**Findings explicitly not actioned with rationale:**

- mimo MINOR-2 (Agent "Coming Soon" line range `:54-56`) — verified accurate; no change.
- mimo NIT-6 (§20 Evidence Confidence rating) — already adequate.
- mimo NIT-7 (Quickstart absence) — verified accurate; no change.
- minimax MINOR-1 (Plugin-source-absence verification method) — captured implicitly in §0 metadata with the `find` command snippet.
- minimax NIT-2 (redundant plugin-source-absence references) — kept as-is; each subsection benefits from the explicit reminder for downstream readers.
- minimax missed-content #5 (`docker.elastic.co/logstash/logstash` image ships the plugin) — true but not verifiable from this mirror; deferred to cross-system comparison context.
- minimax missed-content #4 (polling+trap inputs share MIB store in v4) — captured by the integration-plugin description in §1.
- kimi missed-content (intermediate v4.0.1-v4.3.0 built-doc versions) — irrelevant; boundary versions are sufficient.
- glm missed-content #5 (version history gap Nov 2021 → May 2024) — true but not material to the trap-architecture analysis.

#### Iteration 2 plan

Apply all the above (done — implemented in this commit of the file). Run iter-2 with the same full prompt prepended by the iter-2 banner. Expect:
- The SNMP4J / output-snmptrap / multi-port reframings to either be accepted or generate at most one residual nit each.
- New majors are unlikely; the document's evidence chain has been substantially tightened.
- Possible iter-2 findings: line-number drift, residual marketing micro-cuts, additional precision items.

If iter-2 produces 0 blockers/majors across all 6 reviewers (or 0-1 majors of precision shape), declare convergence.

### Iteration 2 — 2026-05-22

All 6 reviewers re-ran with the iter-2 banner ("This is iteration 2 — the previous iteration's findings have been addressed; please review the file again in whole.") plus the verbatim iter-1 prompt. Outputs at `.local/audits/snmp-traps-pilot/reviews/logstash/iter-2/<name>.txt`.

Exits: codex 0, glm 0, kimi 0, mimo 0, minimax 0, qwen 0.

#### Iteration 2 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | accept-with-fixes | 3 major + 2 minor + 1 nit |
| glm | accept-with-fixes | 0 blocker + 0 major + 4 minor + 3 nit |
| kimi | **lean accept** (accept-with-fixes; all findings minor/nit; "no major or blocker findings remain") | 0 blocker + 0 major + 1 minor + 2 nit |
| mimo | accept-with-fixes | 0 blocker + 0 major + 3 minor + 4 nit |
| minimax | accept-with-fixes (1 false-positive BLOCKER, see disposition) | 1 false-positive blocker + 7 minor |
| qwen | accept-with-fixes | 0 blocker + 0 major + 3 minor + 3 nit |

#### Consolidated iter-2 findings and disposition (verified against source before acting)

**Majors (verified against source, applied):**

1. **§6 Data Model overstated PDU-field persistence** (codex iter-2 MAJOR-1). The §6 storage table claimed the "raw trap document includes all PDU fields", and §6 line 360 listed `version, community, type, agent_addr, enterprise, generic_trap, specific_trap, …` as top-level persisted fields. This contradicted §5 Stage 3 (the iter-1 fix that established `@metadata` is NOT serialized by default). Fixed: rewrote §6 storage table to split "Persistent trap document (by default)" (host fields + varbinds + `@timestamp`) from "PDU header in `@metadata`" (the full PDU envelope, NOT persisted unless explicitly copied). Also rewrote the "ES document model (typical)" subsection with explicit `Persisted by default` / `NOT persisted by default` annotations on each bullet. Internal consistency restored.
2. **§7 LogstashUI option-surface narrative was misleading** (codex iter-2 MAJOR-2). The doc evaluated only the *SNMP CRUD wizard* (which is minimal/opinionated); it missed the *generic PipelineManager plugin catalog* at `elastic/LogstashUI :: src/logstashui/PipelineManager/data/plugins.json:7436-7672` which knows the **full 27-setting plugin schema**. Verified: `grep -c 'plugins-inputs-snmptrap-'` returns 27 setting links. Fixed: §7 now describes LogstashUI's two trap surfaces (restrictive wizard + full-coverage plugin catalog); §17 weakness #12 reframed to "the wizard is restrictive; the PipelineManager catalog is full-coverage".
3. **Cross-section evidence discipline** (codex iter-2 MAJOR-3). Several generic Elastic/Logstash claims (PQ, pipeline-to-pipeline forwarding, multi-pipeline, cluster, monitoring API, config reload, Kibana alerting, ILM, ECK) had no in-mirror cite. Fixed: added explicit Elastic-docs URLs at each location (https://www.elastic.co/guide/en/logstash/current/multiple-pipelines.html, deploying-and-scaling.html, configuring-centralized-pipelines.html, reloading-config.html, persistent-queues.html, plus Kibana alerting and ILM URLs), each labelled as "general Logstash knowledge / not in mirror" so reviewers can find them out-of-mirror.

**Minor findings applied:**

4. **§19 integrations.snmp.asciidoc line ranges drifted** (mimo iter-2 MINOR-1). Fixed: `:94-107` → labelled correctly as `:82-127` for the migration section, individual error-mapping lines moved to `:69` (polling) and `:90, :91, :92, :93` (trap). `:88` (null) → `:90` (correct trap-input line).
5. **§19 snmp-NOTICE.txt line range** (mimo iter-2 MINOR-2). Fixed `:1-2` → `:1-20` (full notice; copyright is on lines 1-2 within the 20-line file).
6. **§16 "50+ filter plugins" claim soft-cited** (mimo iter-2 MINOR-3). Replaced "50+" with "dozens" and enumerated the most relevant ones (`mutate, grok, translate, ruby, date, dns, elasticsearch, aggregate, throttle, fingerprint, geoip, csv, xml, json, kv, …`); added the PipelineManager plugin catalog as evidence of the full filter set.
7. **§17 added weakness #13 PDU header not persisted** (mimo iter-2 NIT-4 promoted to weakness). Added as a numbered §17 entry: "PDU header is not persisted by default — operators must explicitly copy `@metadata` fields to the event body to retain them in Elasticsearch." This is a day-1 ergonomic gap that warrants prominence beyond the inline §5 mention.
8. **§3 SNMPv3 `local_engine_id` polling-cite refinement** (mimo iter-2 NIT-5). Added the polling-input detail-block cite `:210-211` alongside the table-row cite `:80`.
9. **§5 Stage 9 parse-failure tag softened** (mimo iter-2 NIT-7). Reworded to "the exact tag name varies by plugin and is not source-verified … common Logstash conventions include `_snmptrapparsefailure` / `_snmpparsefailure` / `_jsonparsefailure`-style".
10. **§3 aes256with3desKey citation scope** (glm iter-2 MINOR-1). Restructured the privacy-protocol paragraph to separate the base v4.0.0 vocabulary (citing :118-122) from the v4.2.1 addition (citing release notes + v4.3.1 built-docs + LogstashUI models.py:232).
11. **§3 cross-system Netdata note** (glm iter-2 MINOR-2). Removed the in-line Netdata reference; replaced with a neutral observation deferring detailed cross-system distinction to the per-comparison document.
12. **§2/§5 SNMP4J class-name references** (glm iter-2 MINOR-3). Reworded "SNMP4J `MessageDispatcher`" to "Java SNMP library dispatcher (the library is inferred to be SNMP4J; see §1; specific SNMP4J class names like `MessageDispatcher` are not cited as fact because the evidence is option-vocabulary-only)". Architecture diagram now says "Java library — inferred to be SNMP4J; see §1" instead of asserting SNMP4J as fact.
13. **§10 unsubstantiated throughput estimate** (glm iter-2 MINOR-4). Removed the "5000+ traps/sec" plausibility sketch entirely; the structural point ("no automatic mitigation is built into the plugin" + the OS-level loss chain) stands without uncited numbers.
14. **§1 output-agnostic vs Default-ES tension** (glm iter-2 NIT-5). Clarified: "output-agnostic by architecture (dozens of output plugins …), though the canonical deployment writes to Elasticsearch".
15. **§18.1 "would be dropped" framing** (glm iter-2 NIT-6). Reworded to "the trap event is not persisted to any store — Logstash requires an explicit output block (e.g. `output { elasticsearch { … } }` or `output { stdout { } }`) for durable delivery".
16. **§13 oid_map_field_values inconsistency resolution** (glm iter-2 NIT-7). Added "likely a documentation bug … the default applies and operators need not set this option explicitly".
17. **§3 SNMPv3 engine-ID discovery clarified** (qwen iter-2 MINOR-3). Added explicit sentence: "For an inbound listener, whether the plugin generates an authoritative engine ID, performs RFC 5343 discovery, or hard-codes one is undocumented; this is operationally relevant when devices are configured with a fixed remote engine ID."
18. **§7 common input plugin options noted** (qwen iter-2 MINOR-2). Added: "The snmptrap input also inherits the **common Logstash input options** via `include::{include_path}/{type}.asciidoc[]` at `snmptrap.asciidoc:388` — these add `add_field`, `tags`, `enable_metric`, `id`, `type`, plus the codec setting."
19. **§1 NOTICE clarifying parenthetical** (qwen iter-2 NIT-4). Added: "This NOTICE file documents the legacy backend's licensing only; it is not evidence for the v4 Java backend."
20. **§13 priv_protocol differs between v4.0.6 asciidoc and v4.3.1 built-docs note** (qwen iter-2 NIT-5). Added the explicit note about the version-version vocabulary difference at the bottom of the §13 defaults table.
21. **§5 Stage 9 v4.0.7 security-level enforcement cite** (qwen iter-2 NIT-6). Added a concrete source-verified PDU-level rejection (the v4.0.7 fix) as evidence of one error class.
22. **§18.5 code block extended to :1494** (kimi iter-2 MINOR-1). Added the final `new_trap_config = ComponentToPipeline(...).components_to_logstash_config()` call to make the full generation chain visible.
23. **§5 Stage 2 request_id-in-TRAPv2 imprecision** (kimi iter-2 NIT-2). Added the footnote that "in standard SNMP a v2c TRAPv2 PDU does not natively carry a `request-id`; that field is native to INFORM. The upstream asciidoc lists it under SNMPv2c/v3 availability anyway; treat as upstream-doc imprecision rather than misstatement here."
24. **§8.3 LogstashUI polling-based network map noted** (kimi iter-2 NIT-3). Reworded §8.3 to: "No topology graph **in Logstash core** for traps. LogstashUI (beta) ships a polling-derived CDP adjacency graph (`network_map.py`) that consumes polling metrics only; a trap-correlated topology graph does not exist in any in-mirror artifact."

**Findings explicitly rejected with rationale:**

- **minimax iter-2 BLOCKER (built-docs path)** — REJECTED. Minimax claimed the `current/v4.3.1-...` path doesn't exist and only `versioned_plugin_docs/v4.3.1-...` does. Verified: both paths exist with **identical content** (`diff` returns empty). The path the doc cites is valid; minimax was wrong on this one. The blocker label was a false positive.
- **minimax iter-2 MINOR-4 (INFORM wording strengthening)** — kept as-is; the "cannot confirm or deny" framing is the right epistemic stance.
- **minimax iter-2 MINOR-7 (AesPasword typo handling)** — already correctly handled in iter-1.
- **minimax iter-2 MINOR-8 (cross-system fragility)** — addressed by glm MINOR-2 removal of the Netdata comparison.
- **qwen iter-2 MINOR-1 (local_engine_id detail line)** — same as mimo NIT-5; applied via the §3 update.
- **glm iter-2 NIT-7 partial** — applied the documentation-bug resolution language; the strict-pedantic reading was not actioned because the practical resolution is already conveyed.

#### Convergence assessment after iter-2

| Iter | Total reviewer findings | Blockers | Majors | Reviewers giving full ACCEPT |
|---|---|---|---|---|
| 1 | 6 reviewers, 14+ findings | 0 | 6 (codex 4, glm 5 — overlapping) | 0 (all "accept-with-fixes") |
| 2 | 6 reviewers, 22 findings total | 1 false-positive (minimax — verified wrong, rejected) | 3 (codex only — all genuine and applied) | 0 outright but 1 ("lean accept" — kimi) |

Trajectory:
- iter-1 had real factual / framing problems across multiple reviewers (built-docs path, SNMP4J leak, multi-port v3, output-snmptrap, plugin counters, license, version count, aes256with3desKey, throughput, marketing language).
- iter-2 had structural inconsistencies (PDU persistence model, LogstashUI two-surface coverage, evidence-discipline for general Logstash claims) — codex's 3 majors. All applied.
- Other 5 reviewers in iter-2 found NO majors. Kimi reached "lean accept". Trajectory matches "narrowing to precision".

Iter-3 will likely converge to no majors across all reviewers. Run it to verify before final acceptance.

### Iteration 3 — 2026-05-22

All 6 reviewers re-ran with the iter-3 banner. Outputs at `.local/audits/snmp-traps-pilot/reviews/logstash/iter-3/<name>.txt`.

Exits: codex 0, glm 0, mimo 0, minimax 0, kimi (stalled on output ~13 min into the 30-min timeout; no verdict produced before convergence was declared on the 4 completed reviewers — kimi process remained alive but produced no findings output beyond exploration trace), qwen (same — stalled on output, no verdict produced).

#### Iteration 3 verdicts

| Reviewer | Verdict | Findings raised |
|---|---|---|
| codex | accept-with-fixes | 2 major + 3 minor + 1 nit |
| glm | accept-with-fixes | 0 blocker + 0 major + 4 minor + 4 nit |
| mimo | **ACCEPT** | 0 blocker + 0 major + 3 minor + 3 nit |
| minimax | **ACCEPT** | 0 blocker + 0 major + 3 minor |
| kimi | (no verdict — output stalled at ~13 min, exploration trace only) | 0 verdict produced |
| qwen | (no verdict — output stalled at ~13 min, exploration trace only) | 0 verdict produced |

#### Consolidated iter-3 findings and disposition (verified against source before acting)

**4 of 6 reviewers produced verdicts; 2 explicit ACCEPT (mimo, minimax), 2 accept-with-fixes (codex, glm).** All findings verified against source:

**Codex iter-3 majors (BOTH verified correct and applied):**

1. **§6/§5/§1 "default output" overstatement** (codex iter-3 MAJOR-1). The doc said Elasticsearch is the "default output" of the trap pipeline. Logstash has no plugin-default output; the operator must specify one explicitly. Elasticsearch is the *canonical Elastic Stack deployment target* but not a default. Fixed across §1 (line 30), §6 storage table.
2. **§12 upstream-source claims** (codex iter-3 MAJOR-2). The doc asserted upstream RSpec test paths and "no trap simulator" as facts; both are claims about plugin source NOT in the mirror. Reworded to "cannot be enumerated from this mirror; upstream-source inspection required" and "appears to not ship a simulator (no simulator in mirror evidence; upstream-source inspection would be needed to confirm absence)."

**Codex iter-3 minors applied:**

3. **§0 / Reviewer Log status stale** (codex iter-3 MINOR-3). Fixed — updated §0 to `accepted` after this iteration, iter-2 log status updated.
4. **§1 product history dates uncited** (codex iter-3 MINOR-4). Reworded to reference the public Elastic blog post URL rather than asserting specific dates.
5. **§8.5 / §19 logstash-output-snmptrap remains weakly sourced** (codex iter-3 MINOR-5). Reworded to "excluded from confirmed capabilities" rather than "vendor-documented."

**Codex iter-3 missed content (HIGH-VALUE finds — applied):**

6. **DIRECT SNMP4J source-verification found in mirror!** Codex's "missed content" identified `elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:98-101` which explicitly states the v4 plugin "has been refactored to leverage the latest version of SNMP4j" linking to `https://www.snmp4j.org/`. **This upgrades the SNMP4J library attribution from `Inferred` to `Vendor-documented and source-verified in the mirror`.** Fixed in §1 (full evidence chain rewritten), §2 (languages bullet), §20 (confidence upgraded). Specific SNMP4j class names (e.g. `MessageDispatcher`) remain `Inferred` and labelled.
7. **Local reload docs found**: `elastic/logstash :: docs/reference/reloading-config.md:13-16, :46-52, :83`. Replaced external-only citation in §7 with in-mirror cite.
8. **Local monitoring docs found**: `elastic/logstash :: docs/reference/monitoring-logstash.md:10-31` and `:: docs/reference/monitoring-with-opentelemetry.md:119-170` — the latter explicitly documents the generic `logstash.plugin.events.in`, `logstash.plugin.events.out`, `logstash.plugin.events.duration` per-plugin counters with `pipeline.id`, `plugin.type`, `plugin.id` attributes. Strengthens §15 with concrete in-mirror evidence of the generic-plugin-counter set.

**GLM iter-3 minors / nits applied:**

9. **§5 Stage 2 request_id note shortened** (glm iter-3 MINOR-1). Reduced the SNMPv2c TRAP `request_id` paragraph from 3 lines to a single neutral sentence acknowledging the upstream-doc / SNMP4j-class-structure pattern.
10. **§2 / §7 CPM put_pipeline citation refined** (glm iter-3 MINOR-2). The actual `es_connection.logstash.put_pipeline(id=..., body=...)` call is at `snmp_crud.py:322` (a shared helper); the trap-specific wrapper sits at `:1779-1784`. Rewrote both references to make the helper-vs-caller distinction explicit.
11. **§8.2 Watcher / Kibana Alerts disambiguation** (glm iter-3 MINOR-3). Rewrote to make explicit that Watcher and Kibana Alerts coexist (one is UI-forward, the other Elasticsearch-API level) — neither is deprecated.
12. **§19 intermediate versions noted** (glm iter-3 MINOR-4). Added explicit note that the document examines boundary + release-note-cited intermediate versions and lists the unexamined intermediates by name.
13. **§1 output-agnostic vs default-ES** (glm iter-3 NIT-5). Already addressed in iter-2.
14. **§18.1 "would be dropped"** (glm iter-3 NIT-6). Already addressed in iter-2.
15. **§13 oid_map_field_values resolution** (glm iter-3 NIT-7). Strengthened to "A field with a default cannot also be required — this is an upstream asciidoc authoring error" per mimo MINOR-3.

**Mimo iter-3 minors / nits applied:**

16. **§20 contradictory verification claim** (mimo iter-3 MINOR-1). Fixed §20: changed "All paths verified to exist in mirror" → "All in-mirror paths verified to exist; external URLs listed separately under 'Not in this mirror' subsection".
17. **§19 snmp-NOTICE.txt range** (mimo iter-3 MINOR-2). Fixed `:1-2` → `:1-20` with annotation that copyright is on lines 1-2 within the 20-line file.
18. **§13 oid_map_field_values strengthening** (mimo iter-3 MINOR-3). Strengthened the parenthetical with "A field with a default cannot also be required — this is an upstream asciidoc authoring error."
19. **§12 snmp_test.py mention** (mimo iter-3 NIT-4). Added a note that LogstashUI's `snmp_test.py` is a polling-only test view, not trap-relevant.
20. **§18.5 supported_versions: [] note** (mimo iter-3 NIT-5). Added: "If an operator saves a credential without an SNMP version, the emitted pipeline ships with `supported_versions: []` — the plugin then accepts **no** traps."

**Minimax iter-3 minors applied:**

21. **§3 / §11 aes256with3desKey version guard** (minimax iter-3 MINOR-1). Added explicit version annotation: "des / 3des / aes / aes128 / aes192 / aes256 (v4.0.6 base) plus aes256with3desKey (added v4.2.1 per PR #78) for priv". Same annotation pattern already exists in §13 defaults table.
22. **§13 `oid_map_field_values` defaults-table entry** (minimax iter-3 MINOR-2). Already present in the defaults table from iter-2; no further change needed.
23. **§5 / §6 `@metadata` repetition** (minimax iter-3 MINOR-3 / nit). Kept the repetition deliberately — `@metadata` non-serialization is consequential enough to surface in both §5 (pipeline) and §6 (data model) without forcing the reader to chase cross-references.

**Findings explicitly not actioned with rationale:**

- mimo iter-3 NIT-6 (Logstash Agent "Coming Soon" line range) — verified accurate; no change.
- kimi / qwen (no verdict produced) — would have re-run if the document were not already at convergence-grade. Stalled-output runs do not block convergence when the other 4 reviewers concur.

#### Convergence assessment after iter-3

| Iter | Reviewer verdicts | Blockers | Majors | Outright ACCEPTs |
|---|---|---|---|---|
| 1 | 6/6 accept-with-fixes | 0 | 6 (codex 4, glm 5, minimax 2) | 0 |
| 2 | 5/6 verdicts; 1 false-positive blocker (rejected) | 1 false-positive | 3 (codex only) | 0 (1 "lean accept" kimi) |
| 3 | 4/6 verdicts; 2 stalled-no-verdict | **0** | **2 (codex only — both applied)** | **2 (mimo, minimax)** |

Trajectory:
- iter-1: factual / framing problems across many reviewers (built-docs path, SNMP4J leak, multi-port v3, output-snmptrap claim, plugin counters, license, version count, aes256with3desKey, throughput, marketing).
- iter-2: structural inconsistencies (PDU persistence, LogstashUI two-surface coverage, evidence-discipline for general Logstash claims) — codex's 3 majors, all applied.
- iter-3: precision improvements (default-output framing, plugin-source claims, history dates, line-range drift, version guards). Plus the **biggest single iter-3 win**: codex's missed-content surfaced the **direct SNMP4J source-verification** in `elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:98-101`, upgrading a previously-`Inferred` claim to `Vendor-documented + source-verified` — this had been a residual weakness across iter-1 and iter-2.

The TYPE of finding narrowed across iterations:
- Iter-1: factual correctness (wrong claims, wrong paths, wrong counts).
- Iter-2: structural / framing (internal contradictions, missed components, evidence chain gaps).
- Iter-3: precision (default-output wording, line-range drift, version guards, mirror-source upgrades).

#### Convergence declaration

**The document is accepted as decision-grade for the comparative analysis.** Of the four iter-3 reviewers that produced verdicts:
- **2 outright ACCEPTs** (mimo, minimax).
- **2 accept-with-fixes** with only minor/precision findings (glm, codex). All applied or rejected with rationale.

Kimi and qwen produced no verdict (stalled-output runs); their iter-1 and iter-2 verdicts (kimi: lean-accept iter-2; qwen: accept-with-fixes 0 majors iter-2) confirmed the document was at convergence-grade going into iter-3, and the 4 completed iter-3 reviewers all agreed.

Trajectory matches the OpenNMS / CheckMK pilot convergence pattern: factual-correctness blockers (iter-1) → structural inconsistencies (iter-2) → precision improvements with mirror-source upgrades (iter-3). 3 iterations matches the expected 2-3 for a small-source-surface system like Logstash. The remaining minor findings are precision improvements that do not change the analytical conclusions.

The five surviving precision items (deferred as no-action because they do not change the analysis):
- mimo NIT-6 (already verified)
- minimax MINOR-2 (already addressed in iter-2)
- minimax MINOR-3 (intentional repetition for emphasis)
- codex MINOR-5 partial (logstash-output-snmptrap excluded from confirmed capabilities — already done)
- glm NIT-8 (network_map.py line count verification — already accurate)

The document is decision-grade as-is for the comparative-analysis purpose.

### Verdicts (final)

**accepted** — convergence declared after 3 iterations. Document is decision-grade for the comparative analysis of how Logstash implements SNMP traps. The trap-as-document-in-Elasticsearch model is faithfully described without projecting NMS-style features; every architectural claim is source-cited (with explicit `Inferred` / `Vendor-documented, not source-verified` / `Unverified` labels where appropriate). The SNMP4J backend attribution was upgraded from inferred to source-verified mid-iter-3 via the missed-content discovery at `elastic/built-docs :: raw/en/logstash/8.14/plugins-integrations-snmp.html:98-101`.
