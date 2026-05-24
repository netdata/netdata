# Splunk Connect for SNMP (SC4SNMP) — SNMP Trap Support: Complete Implementation Analysis

## 0. Document Metadata

- **System**: Splunk Connect for SNMP (SC4SNMP). A Kubernetes-native (or Docker-Compose) data pipeline that ingests SNMP polling results and SNMP trap notifications and forwards them to Splunk Enterprise / Splunk Cloud via HEC (HTTP Event Collector). SC4SNMP is itself **not** a monitoring or alerting product — it is an ingest connector. All "monitoring" semantics (events index, search, alerting, dashboards) live in Splunk.
- **Version analysed**: HEAD at commit `fdd4c74ef3cc8295675039be9f432b00e48b96d8` (chore release `1.16.0`, dated 2026-04-23). Helm chart `Chart.AppVersion` matches `1.16.0`.
- **Source evidence**: mirrored (full Python source, Helm chart, Docker-Compose layout, integration tests, asciidoc/markdown operator documentation).
- **Repository root analysed**: `splunk/splunk-connect-for-snmp @ fdd4c74e`
- **Mirrored at**: `/opt/baddisk/monitoring/repos/splunk/splunk-connect-for-snmp/`
- **License**: Apache-2.0 (see `LICENSE` and copyright headers e.g. `splunk/splunk-connect-for-snmp @ fdd4c74e :: splunk_connect_for_snmp/traps.py:1-15`).
- **Author**: assistant
- **Reviewer pass**: iteration 2 complete; see §21 for per-iteration reviewer history.

Citation format used below: `splunk/splunk-connect-for-snmp @ fdd4c74e :: <relative-path>:<line>`. After the first reference the commit prefix is dropped for readability; the repository prefix is shown explicitly when ambiguity could arise.

This analysis covers only the **trap** subsystem. Polling (poller, scheduler, walk) is referenced where it shares infrastructure (MIB server, Mongo, Redis, Celery, Sender) but is not the subject of the document.

---

## 1. System Overview & Lineage

**Splunk Connect for SNMP (SC4SNMP)** is the official, vendor-maintained way to get SNMP data into Splunk. The README puts it bluntly: *"Splunk Connect for SNMP Gets SNMP data in to Splunk Enterprise and Splunk Cloud Platform"* (`README.md:1-2`). The repository is owned by `splunk/`, released under Apache-2.0, and *"officially supported by Splunk. Customers who encounter any issues or require assistance can open a support ticket directly with Splunk Support"* (`README.md:20-21`). The primary audience is operators of Splunk Enterprise / Splunk Cloud who need to bring legacy SNMP-instrumented network and storage devices into a Splunk search/alert workflow without writing custom forwarders.

**Project shape**: a Python 3.10 codebase (`pyproject.toml:25`, `Dockerfile:1`) packaged into a single container image (`ghcr.io/splunk/splunk-connect-for-snmp/container`) that is invoked with a sub-command argument — `trap`, `celery worker-trap`, `celery worker-poller`, `celery worker-sender`, `celery beat`, `inventory`, `flower` — to choose which role the container plays inside the cluster (`entrypoint.sh:11-50`). The deployment unit is **a Helm chart** (`charts/splunk-connect-for-snmp/`) targeted at MicroK8s (Splunk's documented reference), or a `docker-compose.yaml` for single-host installations (`docker_compose/docker-compose.yaml`).

**Where SNMP traps fit in the broader product**: SC4SNMP has *"two main purposes. The first one is used to collect SNMP data from network devices according to planned schedules and the second one is responsible for listening to SNMP traps"* (`docs/architecture/design.md:14-15`). Traps are a **first-class but co-equal** signal — they share the same Celery framework, the same MongoDB, the same Redis broker, the same MIB server, and the same Splunk HEC sender as polling. The processing pipeline is the same `prepare → send` chain; only the producer (trap receiver vs poller worker) differs.

**Relationship to upstream tools**:
- The trap listener does **NOT** delegate to Net-SNMP's `snmptrapd`. SC4SNMP opens its own UDP socket using the pure-Python **pysnmp** library (specifically a fork: `pysnmplib = {git = "https://github.com/pysnmp/pysnmp.git", branch = "main"}` at `pyproject.toml:48`). The receiver is implemented as `pysnmp.entity.rfc3413.ntfrcv.NotificationReceiver` (`splunk_connect_for_snmp/traps.py:48, :445`).
- MIB resolution is delegated to a separate **MIB server** container (`ghcr.io/pysnmp/mibserver` — referenced by `MIB_SOURCES`/`MIB_INDEX`/`MIB_STANDARD` URLs in `charts/splunk-connect-for-snmp/templates/traps/deployment.yaml:53-58`; submission process documented at `docs/mib-request.md:13-17`).
- Asynchronous task processing is handled by **Celery 5.5.3** (`pyproject.toml:28`) with **Redis** as broker and result backend (Redis can run standalone or with Sentinel HA — see `splunk_connect_for_snmp/celery_config.py:40-65`). Scheduling for periodic polling uses **RedBeat** (`celery_config.py:75`).
- State (engine IDs, inventory, profiles, groups, schema versions) is persisted in **MongoDB** (`splunk_connect_for_snmp/common/collection_manager.py:224-262`; container envs at `traps/deployment.yaml:51`). MongoDB can run standalone or as a 3-node replica set (`charts/splunk-connect-for-snmp/values.yaml:546-565`).
- Output is to **Splunk HEC** (HTTP Event Collector) using the `requests` library — `splunk_connect_for_snmp/splunk/tasks.py:38-71`. Optionally also to an **OpenTelemetry metrics endpoint** for Splunk Observability Cloud / SignalFx (`splunk/tasks.py:87`; `values.yaml:108-134` `sim` section). **For traps, the OTel path is a no-op**: `prepare_trap_data` (`splunk/tasks.py:293-315`) emits an empty `data["metrics"]` list, and `send` (`splunk/tasks.py:182-184`) only POSTs `data["metrics"]` to `OTEL_METRICS_URL`. Traps reach HEC only.

**Distinctive lineage characteristic**: SC4SNMP **does not ship a single-process / single-binary install path** — the supported deployment surface is Helm-on-Kubernetes (MicroK8s is the documented baseline) or Docker-Compose (`docs/architecture/planning.md:9` lists *"A supported deployment of MicroK8s or Docker Compose"* as the requirement). The docs favor MicroK8s; Docker-Compose is offered as a lighter alternative. Whether SC4SNMP is *unique* among the cohort in this respect (vs e.g. Telegraf, snmptrapd, Zabbix's trapper) is a cross-system comparison that belongs in the comparison matrix; here we only assert that SC4SNMP's own product positioning is: if you have Splunk and want SNMP into it, you stand up a small Kubernetes cluster or a Docker-Compose stack — there is no `./snmp-trap-receiver --config foo.yaml` binary.

**License**: Apache-2.0 (`LICENSE`).

**Age & ownership**: copyright headers across the codebase are dated *2021* (e.g. `traps.py:1-2`, `celery_config.py:1-2`). The chart appVersion is `1.16.0` as of April 2026. Splunk owns the repository directly under `splunk/splunk-connect-for-snmp` — corporate-owned, not a community fork.

---

## 2. Trap-Subsystem Architecture

### Components (Kubernetes deployment surface)

```
                          SNMP devices (routers / switches / appliances)
                                          |
                                          | UDP/162 (or NodePort/LoadBalancer)
                                          v
   +---------------------------------------------------------------------------+
   |                            Kubernetes namespace                            |
   |                                                                            |
   |   +------------------+    +------------------+    +-------------------+    |
   |   | Service          |    | trap pod         |    | worker-trap pod   |    |
   |   | (LoadBalancer    |--->| (Deployment      |    | (Deployment       |    |
   |   | or NodePort)     |    |  N replicas)     |    |  N replicas)      |    |
   |   | UDP/162 -> 2162  |    |  pysnmp receiver |    |  Celery worker    |    |
   |   +------------------+    |  Celery producer |--->|  (queue: traps)   |    |
   |                           |  -> Redis queue  |    |  MIB resolve     |    |
   |                           +------------------+    |  -> Redis queue   |    |
   |                                                    +-------------------+    |
   |                                                              |              |
   |                                                              v              |
   |                                                    +-------------------+    |
   |   +---------------+   +---------------+            | worker-sender pod |    |
   |   | mibserver     |   | MongoDB       |            | (Deployment       |    |
   |   | (Deployment)  |   | (StatefulSet) |            |  N replicas)      |    |
   |   | pysnmp/mibs   |   | engine IDs,   |            |  Celery worker    |    |
   |   +---------------+   | inventory,    |            |  (queue: send)    |    |
   |          ^            | profiles,     |            |  HTTPS to HEC     |    |
   |          |            | schema, cache |            +-------------------+    |
   |          |            +---------------+                      |              |
   |          |                                                   |              |
   |   +---------------+   +---------------+                      |              |
   |   | Redis         |<--+ scheduler /   |                      |              |
   |   | (Deployment   |   | inventory job |                      |              |
   |   |  or HA Sentinel)  +---------------+                      |              |
   |   | broker queues |                                          |              |
   |   +---------------+                                          |              |
   +---------------------------------------------------------------------------+
                                                                  |
                                                                  v
                                                  +---------------------------+
                                                  |  Splunk HEC               |
                                                  |  index="netops"           |
                                                  |  sourcetype="sc4snmp:traps"|
                                                  +---------------------------+
                                                                  |
                                                                  v
                                                       Splunk Enterprise /
                                                       Splunk Cloud search,
                                                       dashboards, alerts.
```

The diagram resolves to the following Helm-defined Kubernetes objects (file paths under `charts/splunk-connect-for-snmp/templates/`):
- **`traps/deployment.yaml`** — the pysnmp NotificationReceiver pod (the "front door"). Default 2 replicas (`values.yaml:472`).
- **`traps/service.yaml`** — `LoadBalancer` (default) or `NodePort` UDP service that exposes 162 externally and targets pod port 2162.
- **`traps/hpa.yaml`** — HorizontalPodAutoscaler (CPU and optional memory averageValue) for the front-door pods (`charts/splunk-connect-for-snmp/templates/traps/hpa.yaml:1-32`).
- **`traps/pdb.yaml`** — PodDisruptionBudget.
- **`traps/networkpolicy.yaml`** — NetworkPolicy (only Ingress/Egress sections declared; pod selector includes the traps-labeled pods).
- **`worker/trap/deployment.yaml`** — the Celery worker pods consuming the `traps` Redis queue (`charts/splunk-connect-for-snmp/templates/worker/trap/deployment.yaml:47-50` — `args: [celery, worker-trap]`).
- **`worker/trap/hpa.yaml`** — separate HPA for the worker tier (`charts/splunk-connect-for-snmp/templates/worker/trap/hpa.yaml:1-25`).
- **`worker/.../sender`** templates — the worker consuming the `send` queue and POSTing to HEC. **Important**: the sender is a separate Celery worker; it is *shared* with the polling pipeline. There is no "trap sender" — every trap-derived event is queued onto the `send` queue alongside polled metrics (`splunk_connect_for_snmp/traps.py:289-294`).
- **`mibserver/`** — separate chart dependency (`pysnmp/mibs`) that serves ASN.1 MIB files over HTTP.
- **`mongodb/`** — StatefulSet (standalone or replication mode; `values.yaml:546-565`). As of 1.16 the chart provides its own MongoDB templates instead of the Bitnami chart (`CHANGELOG.md`: *"MongoDB Migration: Replaced Bitnami MongoDB chart dependency with custom Kubernetes manifests"*).
- **`redis/`** — Deployment (standalone) or StatefulSet (Sentinel HA; `values.yaml:594-643`).
- **`scheduler/`, `inventory/`, `ui/`** — polling-side; included for context only.

### Inter-component IPC

- **Device → trap pod**: UDP/162 (default) → pod port 2162. The 2162 internal port is fixed in `traps.py:372, :379` (pysnmp `openServerMode(("0.0.0.0", 2162))`); the Service maps external 162 → internal 2162.
- **Trap pod → worker-trap pod**: Celery task `trap` enqueued on Redis broker, queue `traps`. The chain enqueued is `trap → prepare → send` (`traps.py:289-294`), each step on its own queue with its own worker tier.
- **Worker-trap pod → MIB server**: HTTP GET (cached via `requests_cache` MongoDB-backed) for `.dic`/ASN.1 MIB lookup (`splunk_connect_for_snmp/snmp/manager.py:53-55, :278-289, :304-307`).
- **Worker-trap pod → MongoDB**: pymongo. Engine-ID lookup, inventory lookup, requests-cache backend.
- **Worker-sender pod → Splunk HEC**: HTTPS POST (mTLS optional; `splunk/tasks.py:42-44, :133-159`). Chunk size 50 events/POST (`splunk/tasks.py:126, :191`).
- **Worker-trap pod → Redis (enqueue `send`)**: Celery chain continuation.

### Deployment model

- **Kubernetes (MicroK8s baseline)**: the documented, vendor-recommended path. The Helm chart is in-repo. Resource recommendation: *"16 Core/32 threads x64 architecture server or virtual machine (single instance), 12 GB ram"* for single-instance, *"HA Requires 3 or more instances (odd numbers) 8 core/16 thread 16 GB ram"* (`docs/architecture/planning.md:9-14`).
- **Docker Compose**: a single-host fallback. `docker_compose/docker-compose.yaml` reproduces the same component graph (coredns, mibserver, mongo, redis, traps, worker-trap, worker-poller, worker-sender, scheduler) as containers on a single Docker host. The trap container binds 162 directly on the host network (`docker-compose.yaml:157-161` `ports: - mode: host, protocol: udp, published: ${TRAPS_PORT}, target: 2162`). There is no LoadBalancer abstraction in compose mode.
- **HA (k8s)**: documented via *"HA Requires 3 or more instances (odd numbers)"* (`docs/architecture/planning.md:13`). MongoDB replication mode (`values.yaml:548-555`) and Redis Sentinel mode (`values.yaml:594-643`) provide datastore HA. Trap-pod replicas + a `LoadBalancer` Service give UDP listener redundancy (modulo the usual UDP load-balancing limitations).
- **HA (compose)**: not a supported HA path; explicitly positioned as the lightweight option.

### Languages and key libraries

- **Python 3.10** (`pyproject.toml:25`, `Dockerfile:1`).
- **pysnmp** (forked, `pysnmplib` from `github.com/pysnmp/pysnmp.git`, branch `main`) — `pyproject.toml:48`. The full RFC 3414/3413/3411 stack: USM auth, MP v1/v2c/v3, MIB compiler, notification receiver.
- **pyasn1** for raw ASN.1 BER decoding when discovering engineIDs out-of-band (`traps.py:19-21, :119-149`).
- **Celery 5.5.3** with `tblib` extra; **Redis** as broker; **RedBeat** for scheduler (`pyproject.toml:28, 41`).
- **pymongo** ≥ 4 (`pyproject.toml:26`).
- **mongolock** for distributed locks across workers (`pyproject.toml:43`).
- **requests** with `requests-cache` (MongoDB backend) and `requests-ratelimiter` (`pyproject.toml:27, 36, 37`).
- **opentelemetry-api/sdk/instrumentation-celery/instrumentation-logging** for telemetry (`pyproject.toml:30-33`).
- **flower** for Celery admin UI (`pyproject.toml:51`).

### Container security context

The trap pod runs as **non-root** with all capabilities dropped and a read-only root filesystem (`charts/splunk-connect-for-snmp/templates/traps/deployment.yaml:33-41`):

```yaml
securityContext:
  capabilities:
    drop: [ALL]
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 10001
  runAsGroup: 10001
```

This means the pod cannot bind UDP/162 inside the container. SC4SNMP solves this by **binding 2162 inside the pod and using the Kubernetes Service to remap 162 → 2162 externally**. This is documented at `docs/configuration/traps.md:191-211`. The same pattern is used by other K8s-native receivers (and Telegraf's snmp_trap input by way of `setcap`/sidecars), and it is an explicit architectural decision that makes SC4SNMP friendlier to multi-tenant clusters but adds a forwarding hop.

---

## 3. Trap Reception (UDP/162 Ingress)

### Listener implementation

SC4SNMP runs **its own pysnmp-based NotificationReceiver** — there is no `snmptrapd` involved. The entry point is `traps.main()` (`splunk_connect_for_snmp/traps.py:337`) which:

1. Sets up MongoDB connection and engine-ID persistence (`traps.py:344-352`).
2. Creates a pysnmp `SnmpEngine` (`traps.py:356`).
3. Registers an "authentication failure observer" callback for visibility into v3 auth errors (`traps.py:360-365`).
4. Adds a custom UDP transport — `_EngineIDCaptureUdpTransport` or `_EngineIDCaptureUdp6Transport` — on `("0.0.0.0", 2162)` or `("::", 2162)` (`traps.py:368-380`).
5. Reads `/app/config/config.yaml` (mounted via ConfigMap) for v1/v2c communities and v3 `usernameSecrets` (`traps.py:382-385`).
6. For v3 users: reads SNMPv3 secrets from `/app/secrets/snmpv3/<secret>/{userName,authKey,privKey,authProtocol,privProtocol}` files (mounted via Kubernetes Secrets — `traps.py:387-422`; the Helm chart mounts them at `charts/.../traps/deployment.yaml:112-117, :158-178`).
7. For each (v3 user × each known security engine ID) tuple, calls `pysnmp.entity.config.addV3User(...)` (`traps.py:424-433`).
8. Registers `ntfrcv.NotificationReceiver(snmp_engine, cb_fun)` (`traps.py:445`) and enters `loop.run_forever()` (`traps.py:448`).

### Internal port and external port

- **Internal port (inside the pod)**: hardcoded `2162` (`traps.py:372, :379`). This is non-configurable in the Python code.
- **External port (Service)**: configurable via Helm value `traps.service.port` (default `162` — `values.yaml:489`).
- **NodePort variant**: `traps.service.type=NodePort` with `nodePort` (default 30000) for clusters without MetalLB / cloud LB (`docs/configuration/traps.md:159-169`; `values.yaml:491-492`).
- **In Docker-Compose**: bound directly to host network via `ports: - mode: host, protocol: udp, published: ${TRAPS_PORT}, target: 2162` (`docker-compose.yaml:157-161`). `TRAPS_PORT` defaults to 162 (per `.env.example` discussion in `docs/configuration/traps.md:208-211`).

> **Note on the `pysnmp.entity.config.addV1System` name.** SC4SNMP uses this function for both v1 and v2c community registration (see `traps.py:323-334`). The `V1System` in the name is a pysnmp API artifact (carried over from RFC 1157 internals); operationally it registers a *community string* that pysnmp then accepts under whichever message-processing model the sender selects. The `"1"` vs `"2c"` keys in `config_base["communities"]` only choose which community list is iterated; the underlying pysnmp call is the same.

### SNMP version support

| Version | Supported | Evidence |
|---|---|---|
| **v1** | yes | `traps.add_communities` registers v1 communities via `config.addV1System(snmp_engine, idx, community)` (`traps.py:330-334`); integration test `test_trap_v1` sends `CommunityData(community, mpModel=0)` (`integration_tests/tests/test_trap_integration.py:80-107`). |
| **v2c** | yes | Same community machinery; integration test `test_trap_v2` uses `mpModel=1` (`integration_tests/tests/test_trap_integration.py:109-136`). |
| **v3 (USM)** | yes | `traps.py:387-433` registers `usernameSecrets` against each known engineID; integration test `test_trap_v3` (`test_trap_integration.py:253-283`); docs `docs/configuration/traps.md:66-87`. |
| **v3 INFORM (notification-reception-with-ack)** | **undocumented, untested** | Neither the SC4SNMP code nor docs explicitly handle INFORM. pysnmp's `NotificationReceiver` does accept INFORM PDUs per its public API, but SC4SNMP's `cb_fun` (`traps.py:263-294`) treats the input as varbinds without differentiating PDU type, and the integration tests send only `"trap"` (`integration_tests/tests/test_trap_integration.py:40, :58`). The only `inform` artifact in the repo is the snmpsim fixture `integration_tests/snmpsim/data/variation/notification.snmprec:3` (used to simulate *sending* an inform from a fake agent, not to test SC4SNMP's *reception* of one). **Receiver-side INFORM behaviour in SC4SNMP is therefore unverified; we make no claim about whether SC4SNMP's NotificationReceiver acknowledges INFORMs or not.** |
| **TLSTM / DTLS** | not supported | `traps.py` only uses `pysnmp.carrier.asyncio.dgram.udp{,6}`. No TLS/DTLS transport configured anywhere. |

### SNMPv3 engineID handling — *the most distinctive feature of SC4SNMP*

Unlike most receivers in the cohort, SC4SNMP has two layered mechanisms for SNMPv3 engineIDs because **pysnmp pre-registers each (user × engineID) pair statically**, and unknown engineIDs would otherwise be rejected at decode time:

1. **Static list (default)**: `SNMP_V3_SECURITY_ENGINE_ID` environment variable (comma-separated). Default `80003a8c04` (`traps.py:60-62`, `values.yaml:475-477`).

2. **Bidirectional sync with MongoDB**: at startup, `_sync_engine_ids_with_mongo()` (`traps.py:152-171`) merges env-supplied engineIDs with the `engine_id_records` collection. Each engineID is then registered against every configured v3 user.

3. **Dynamic discovery (opt-in)**: `DISCOVER_ENGINE_ID=true` (`traps.py:66`) replaces the default UDP transport with `_EngineIDCaptureUdpTransport` (`traps.py:229-242`) which intercepts each datagram **before pysnmp parses it**, runs ASN.1 BER decode against the SNMPv3 message header to extract the `msgAuthoritativeEngineID` and the USM `msgUserName` (`traps.py:119-149`), and if the username matches a configured user, hot-registers the new engineID via `config.addV3User(..., securityEngineId=...)` so the **same** datagram succeeds at the next pysnmp decode pass. The new engineID is persisted to MongoDB so it survives pod restarts (`traps.py:201-226`).

   Docs warn: *"It is recommended to enable this feature only during the initial setup of the traps receiver. Once the engine IDs … have been collected, disable the feature to prevent unwanted engine ID registration and to improve trap processing efficiency by eliminating the overhead of extracting the engine ID from every incoming message."* (`docs/configuration/traps.md:115-116`).

4. **Context engine ID emission**: with `INCLUDE_SECURITY_CONTEXT_ID=true` (`traps.py:63-65`), the decoded engineID is also attached to the event under `fields.context_engine_id` (`traps.py:283-288`).

This is materially more sophisticated than the engineID handling in most peer systems (OpenNMS, Centreon, CheckMK, Zabbix all require manual pre-registration), and it is **forced on SC4SNMP** by pysnmp's eager-registration requirement. It is also a tacit acknowledgment that operators routinely cannot enumerate all device engineIDs ahead of time.

### Concurrency model

- The trap pod itself is **single-process, asyncio-based** (`traps.py:341-342, :448` — `loop.run_forever()`). pysnmp's asyncio dispatcher handles concurrent UDP datagrams in a single thread; CPU-bound work (decode, varbind extraction, MIB resolution) is offloaded by enqueueing a Celery task and returning immediately (`traps.py:289-294`).
- **Horizontal scaling**: `traps.replicaCount: 2` default (`values.yaml:472`). An `HorizontalPodAutoscaler` template ships with the chart (`templates/traps/hpa.yaml`) but `traps.autoscaling.enabled` is **false by default** (`values.yaml:510-515`); the documented `maxReplicas: 100` and `targetCPUUtilizationPercentage: 80` only take effect once the operator flips that flag. The default deployment therefore runs a fixed two trap pods. Multiple trap pods sit behind a UDP Service and consume from independent UDP sockets — UDP load-balancing across pods is **not coordinated** and is best-effort.
- The actual decode + MIB resolution work runs in **worker-trap pods** as Celery tasks; those scale independently (`values.yaml:312-339` `worker.trap`). Default `replicaCount: 2`, `concurrency: 4` (threads per pod), `prefetch: 30`, HPA up to 10 replicas at 80% CPU.
- The sender (HEC POST) is *not* trap-specific — it scales with `worker.sender.replicaCount` (default 1, HPA-capable) and processes the `send` queue from both traps and polling.

### Privileged-port handling

- **Pod cannot bind 162**: containers run as UID 10001, capabilities dropped (`traps/deployment.yaml:33-41`).
- **Kubernetes Service binds 162**: `LoadBalancer` (default, requires MetalLB or a cloud LB) or `NodePort` (no LB needed but port 30000+ externally).
- **Docker-Compose**: relies on the host kernel — Docker maps host 162 → container 2162 via host networking (root-owned port binding done by dockerd, not by the SC4SNMP container).

This is **cleaner than most other systems in the cohort**: there is no SUID helper, no `setcap`, no need to run a Python process as root.

### HA / clustering

- No native trap-listener clustering. Each trap pod independently runs an asyncio receiver. UDP load-balancing across pods is best-effort and depends on the LB (MetalLB layer-2 ARP, cloud LB session affinity, kube-proxy UDP `iptables` rules, etc.).
- The **state shared across trap pods** is in MongoDB (engine IDs) and Redis (the Celery queue). Two trap pods receiving the same trap from a misconfigured device that points at both Service endpoints would each enqueue the trap separately — no dedup on enqueue; the duplicate would appear twice in Splunk. Operators are expected to either point each device at a single Service endpoint or accept-and-dedupe-downstream (in Splunk SPL).

---

## 4. MIB Management

### Where MIBs live

MIBs do **not** live in the trap receiver pod or the worker-trap pod. They live in a **separate mibserver pod** that exposes them over HTTP. The trap and worker-trap pods pull them on-demand.

- Container image: `ghcr.io/pysnmp/mibs` (the chart pulls it as a sub-chart dependency; the repo at `github.com/pysnmp/mibs` hosts the actual MIB files).
- Default MIB index served at `http://<release>-mibserver/index.csv`, MIBs at `http://<release>-mibserver/asn1/@mib@` (`charts/splunk-connect-for-snmp/templates/traps/deployment.yaml:53-58`).
- Format: ASN.1 SMIv1/SMIv2 (the original MIB-file format); pysnmp compiles them lazily into `.py`/`.pyc` (the `/pysnmp-cache-volume` mount at `traps/deployment.yaml:106-108` retains compiled output for the pod lifetime).

### Compilation / load pipeline (worker-trap pod)

The Celery `Poller` task base class (used by both polling and trap tasks) initializes a pysnmp MIB builder and MIB view controller at construction (`splunk_connect_for_snmp/snmp/manager.py:271-316`):

```python
self.snmp_engine = SnmpEngine()
self.already_loaded_mibs = set()
self.builder = self.snmp_engine.getMibBuilder()
self.mib_view_controller = view.MibViewController(self.builder)
compiler.addMibCompiler(self.builder, sources=[MIB_SOURCES])

for mib in DEFAULT_STANDARD_MIBS:
    self.standard_mibs.append(mib)
    self.builder.loadModules(mib)

mib_response = self.session.get(f"{MIB_INDEX}")
# Builds an OID → MIB-name map from index.csv
```

The MIB index is a CSV of `oid,mib-name` pairs (`manager.py:304-312`). At trap-processing time the worker uses `is_mib_known(varbind_oid, oid_str, target)` (`manager.py:496-507`) which walks the OID tuple from longest-prefix to shortest and looks up the result in `self.mib_map`. If found, the MIB is lazily loaded via `self.builder.loadModules(mib)` (`manager.py:487-494`) and the varbind is re-resolved.

The trap-task implementation `splunk_connect_for_snmp/snmp/tasks.py:149-237` walks each varbind, tries to `resolveWithMib`, and on `SmiError` queues the OID for a deferred MIB-load + re-resolve pass.

### Bundled MIBs out-of-the-box

- **Inside the worker code**: `DEFAULT_STANDARD_MIBS = ["HOST-RESOURCES-MIB", "IF-MIB", "IP-MIB", "SNMPv2-MIB", "TCP-MIB", "UDP-MIB"]` (`manager.py:69-76`). These are pre-loaded at startup.
- **From the mibserver**: the `pysnmp/mibs` repo hosts a curated set; the current published catalogue is queryable at `https://pysnmp.github.io/mibs/index.csv` (referenced from `docs/mib-request.md:6-7`).
- **Vendor coverage**: whatever `pysnmp/mibs` includes. Operators with missing MIBs are told to *"create an issue in the [Mibserver](https://github.com/pysnmp/mibs) repository, attaching the files and information about the vendor"* (`docs/mib-request.md:13-17`).

### User workflow for adding/updating MIBs

Three documented paths (`docs/mib-request.md:13-156`):

1. **Submit upstream**: file an issue at `github.com/pysnmp/mibs` and wait for inclusion in the next mibserver image release.
2. **Pin a newer mibserver image**: bump `mibserver.image.tag` in `values.yaml` to grab MIBs already published but not yet in the SC4SNMP release.
3. **Local MIBs (since mibserver 1.15.0)**: mount a local directory containing `<VENDOR>/<MIB-FILE>` (where `<MIB-FILE>` matches the MIB module name and the file content starts with `<MODULE-NAME> DEFINITIONS ::= BEGIN`). In k8s: `mibserver.localMibs.pathToMibs: "/host/path"` (creates a PVC, mounts to mibserver pod). In Docker-Compose: `LOCAL_MIBS_PATH=./local_mibs` in `.env`.

After adding MIBs the user must **rollout-restart** the trap and worker-trap deployments (`docs/mib-request.md:51-56`) — there is no in-process MIB reload.

### Dependency resolution

The mibserver itself resolves `IMPORTS` between MIB modules during compilation. The MIB index CSV is `oid → MIB-name`. When the worker requests `/asn1/<MIB>`, the mibserver returns the file or a 404. Cross-MIB imports are pulled by pysnmp on demand and cached in `/pysnmp-cache-volume/` per pod.

### Fallback behaviour for unknown OIDs

`_process_remaining_oids` (`snmp/tasks.py:197-210`) attempts a second pass after loading newly-discovered MIBs. If still unresolved, the trap is still emitted to Splunk but the OID stays as a numeric `1.3.6.1.4.1.…` string with the field name set to the unresolved OID; this happens at `_resolve_remaining_oids` (`snmp/tasks.py:213-223`) which logs `"No translation found for {oid}"` at WARNING. **The trap is not dropped on translation failure** — this is consistent across the codebase. The Splunk event will have raw-OID fields where translation failed.

### MIB error handling policy

`docs/mib-request.md:19-33` states: *"In cases where errors are present within a MIB, such as incorrect MIB imports, missing type definitions, or improperly defined data types, responsibility for resolving these issues rests with the MIB vendor. We do not provide support for correcting the MIB files itself."* This is a Splunk-supportable boundary that punts MIB-quality issues to the vendor.

### Version management vs firmware

There is no in-product "MIB version per device firmware" notion. The MIB store is global; whichever MIB the mibserver serves is what gets used to resolve any varbind from any device. This is consistent with most pysnmp-based systems.

---

## 5. Trap Processing Pipeline

The pipeline is a three-stage Celery chain (`traps.py:289-294`):

```python
my_chain = chain(
    trap_task_signature(work).set(queue="traps").set(priority=5),
    prepare_task_signature().set(queue="send").set(priority=1),
    send_task_signature().set(queue="send").set(priority=0),
)
_ = my_chain.apply_async()
```

### Stage 0 — Reception (trap pod, pysnmp callback)

The pysnmp `cb_fun(snmp_engine, state_reference, context_engine_id, context_name, varbinds, cb_ctx)` (`traps.py:263-294`):
- Pulls the `transportAddress` (sender IP and port) from the pysnmp execution context.
- Builds a list of `(oid_prettyPrint, value_prettyPrint)` tuples from the varbinds.
- Optionally decodes the SNMPv3 engineID from the raw datagram (`decode_security_context`) and persists it to MongoDB.
- Optionally attaches `fields.context_engine_id` to the work dictionary.
- Enqueues the `trap → prepare → send` Celery chain on Redis.

This callback runs in the asyncio loop; it does **not** do MIB resolution. That's deferred to the worker.

### Stage 1 — `trap` Celery task (worker-trap pod)

`splunk_connect_for_snmp/snmp/tasks.py:149-169`:

```python
@shared_task(bind=True, base=Poller)
def trap(self, work):
    varbind_table, not_translated_oids, remaining_oids, remotemibs = [], [], [], set()
    metrics = {}
    work["host"] = format_ipv4_address(work["host"])

    _process_work_data(self, work, varbind_table, not_translated_oids)
    _process_remaining_oids(self, not_translated_oids, remotemibs,
                            remaining_oids, work["host"], varbind_table)
    _, _, result = self.process_snmp_data(varbind_table, metrics, work["host"])
    if human_bool(RESOLVE_TRAP_ADDRESS):
        work["host"] = resolve_address(work["host"])
    fields = work.get("fields", None)

    return _build_result(result, work["host"], fields)
```

Step by step:
1. **IPv4 normalization** (`format_ipv4_address` at `snmp/tasks.py:240-244`): if `IPv6_ENABLED` and the host contains a `.`, strip the `::ffff:` IPv4-mapped-IPv6 prefix. Otherwise pass through.
2. **First-pass varbind resolution** (`_process_work_data` at `snmp/tasks.py:172-185`):
   - For each (oid, value) tuple in `work["data"]`, validate the value as an OID and lazily load the responsible MIB if known.
   - Build a `pysnmp.smi.rfc1902.ObjectType(ObjectIdentity(oid), value).resolveWithMib(mib_view_controller)`.
   - On `SmiError`, append to `not_translated_oids`.
3. **Second-pass MIB load + re-resolve** (`_process_remaining_oids` at `snmp/tasks.py:197-210`): for each unresolved OID, walk to find a MIB to load, load it, retry resolution. Unresolved OIDs are logged at WARNING and the resolved varbinds are kept.
4. **Group / SNMP-wire-type extraction** (`process_snmp_data` at `snmp/manager.py:542-581`): walks the resolved varbind table, computes a `group_key` per varbind via `get_group_key(mib, oid, index)` (`manager.py:167-184`), and tags each varbind with its **SNMP wire type** — Counter32/64, TimeTicks, Gauge32/64, Integer, Unsigned32/64, CounterBasedGauge64, ZeroBasedCounter64, ObjectIdentifier, OctetString, etc. (`manager.py:187-194, :206-221`) — into a `metrics` dict keyed by group. This is a **wire-type classification, not a Splunk metric-store routing decision**: for the polling path the same dict downstream feeds Splunk's metrics index, but for the trap path the entire dict is serialized as JSON text inside the event body (see §8.1). A varbind classified as `Counter32` here does not become a Splunk counter for a trap.
5. **Optional reverse DNS** (`snmp/tasks.py:138-146, :165-166`): if `RESOLVE_TRAP_ADDRESS=true`, replace `work["host"]` with a TTL-cached `socket.gethostbyaddr(addr)[0]`. **Code-level defaults** (`snmp/tasks.py:53-54`): `MAX_DNS_CACHE_SIZE_TRAPS=100`, `TTL_DNS_CACHE_TRAPS=1800` (seconds). **Helm chart defaults** (`charts/splunk-connect-for-snmp/values.yaml:320-323`, applied via `templates/worker/_helpers.tpl:206-220`): `worker.trap.resolveAddress.cacheSize: 500`, `worker.trap.resolveAddress.cacheTTL: 1800`. **Effective default in Kubernetes deployments is therefore 500**, not 100; the Docker-Compose path uses the code-level default of 100 unless the operator overrides via `.env`.
6. **Result envelope** (`_build_result` at `snmp/tasks.py:226-237`):

```python
{
    "time": time.time(),
    "result": result,             # {group_key: {metrics: {...}, fields: {...}, indexes: [...]}}
    "address": host,
    "detectchange": False,
    "sourcetype": SPLUNK_SOURCETYPE_TRAPS,   # default "sc4snmp:traps"
}
```
(Optionally with `"fields": {"context_engine_id": "<hex>"}` when `INCLUDE_SECURITY_CONTEXT_ID=true`.)

### Stage 2 — `prepare` Celery task (worker-sender pod)

`splunk_connect_for_snmp/splunk/tasks.py:231-269` (the `prepare` task):

When `work["sourcetype"] == "sc4snmp:traps"`, the prepare task takes the `trap` task's output, applies optional custom translations (renames `<MIB>.<old-name>` to `<MIB>.<new-name>` per `scheduler.customTranslations` config — `splunk/tasks.py:318-337`), and calls `prepare_trap_data(work)` (`splunk/tasks.py:293-315`):

```python
def prepare_trap_data(work):
    events = []
    for key, data in work["result"].items():
        processed = {}
        if data["metrics"]:
            for k, v in data["metrics"].items():
                processed[k] = v
                processed[k]["value"] = value_as_best(v["value"])
        event = {
            "time": work["time"],
            "event": json.dumps({**data["fields"], **processed}),
            "source": "sc4snmp",
            "sourcetype": SPLUNK_SOURCETYPE_TRAPS,
            "host": work["address"],
            "index": SPLUNK_HEC_INDEX_EVENTS,  # default "netops"
        }
        if "fields" in work:
            event["fields"] = work["fields"]
        events.append(event)
    if SPLUNK_AGGREGATE_TRAPS_EVENTS:
        events = aggregate_traps(events)
    events = [json.dumps(e, indent=None) for e in events]
    return events
```

Each `group_key` becomes one Splunk HEC event. Optionally, all groups can be merged into a single event with `SPLUNK_AGGREGATE_TRAPS_EVENTS=true` (`aggregate_traps` at `splunk/tasks.py:340-347`).

> **Aggregation caveat (data-loss on key collision).** `aggregate_traps` (`splunk/tasks.py:340-347`) merges every per-group dict via Python `dict.update()`:
>
> ```python
> def aggregate_traps(events):
>     tmp_events = {}
>     for event in events:
>         e = json.loads(event["event"])
>         tmp_events.update(e)
>     events[0]["event"] = json.dumps(tmp_events, indent=None)
>     events = events[:1]
>     return events
> ```
>
> If two groups carry the same key (e.g. two interfaces emit the same MIB.object identifier under different indexes that collapse to the same string), the later one silently overwrites the earlier. Operators should leave aggregation off by default (chart default is `false` — `values.yaml:478-479`) unless the trap-event semantics guarantee unique keys per group.

The output of `prepare` is a dict `{"events": [...], "metrics": []}` ready for HTTP delivery (for traps, `"metrics"` is always empty — traps go to the event index only).

> **Trap-task tuning knobs.** Beyond the documented ones, two environment variables affect what the worker-trap pod does with each PDU:
>
> - `IGNORE_EMPTY_VARBINDS` (default `false` — `snmp/manager.py:61`): when set, traps that decode to an empty varbind list are silently dropped instead of producing an event.
> - `MAX_OID_TO_PROCESS` (default `70` — `snmp/manager.py:65`): caps the number of OIDs the worker chunks per pysnmp call. For traps this matters when a single PDU contains very many varbinds (e.g. some link-state-change traps from large chassis switches); the cap is per `getCmd`/`bulkCmd` chunk, not a hard truncation of the trap, but it does shape latency and MIB-resolution behaviour at the high end.

### Stage 3 — `send` Celery task (worker-sender pod)

`splunk_connect_for_snmp/splunk/tasks.py:169-216` (the `send` task):

```python
@shared_task(
    bind=True, base=HECTask,
    default_retry_delay=5, max_retries=60,
    retry_backoff=True, retry_backoff_max=1,
    autoretry_for=[ConnectionError, ConnectTimeout, ReadTimeout, Timeout],
    retry_jitter=True,
)
def send(self, data):
    if SPLUNK_HEC_TOKEN:
        do_send(data["events"], SPLUNK_HEC_URI, self)
        do_send(data["metrics"], SPLUNK_HEC_URI, self)
    if OTEL_METRICS_URL:
        do_send(data["metrics"], OTEL_METRICS_URL, self)
```

`do_send` posts up to 50 events per chunk (`SPLUNK_HEC_CHUNK_SIZE` default — `splunk/tasks.py:126, :191`), with explicit handling for HTTP 200/202 (success), 400/401/403 (no-retry, log at ERROR), 500/503 (retry with 5s countdown), and any other code (treated as fatal log).

### Source identification (IP → device mapping; agent-addr handling for v1)

- The `host` field in the Splunk event is the transport-layer source IP (extracted from pysnmp `transportAddress` — `traps.py:275`), optionally reverse-DNS-resolved. **The SNMPv1 `agent-addr` field in the trap PDU is NOT used as the source identity in SC4SNMP** — `cb_fun` derives `host` purely from `exec_context["transportAddress"][0]` (`traps.py:275`); if `agent-addr` is present it passes through the chain as just another varbind in `work["data"]`. This is a real operational choice in proxy-trap scenarios (where one device forwards traps on behalf of another and the v1 `agent-addr` carries the *original* sender's IP): SC4SNMP records the *proxy's* IP as `host`, not the originator's. Cross-system comparisons with CheckMK / OpenNMS / Zenoss on `agent-addr` handling belong in the per-system comparison matrix once those specs have settled; we make no claim here about how peers behave.
- No inventory join. The trap event does NOT carry the device's `walk_interval`, polling profile, group, or any inventory metadata. (Polling-derived metrics DO carry such metadata via `set_metrics_fields` — `splunk/tasks.py:272-290` — but traps go through `prepare_trap_data` which omits it.) Operators wanting to enrich trap events with inventory data must do so in Splunk SPL after ingestion.

### Enrichment

The `enrich` Celery task (`splunk_connect_for_snmp/enrich/tasks.py:89-160`) exists, but it operates on the **polling pipeline** — adding attributes from a `mongo.sc4snmp.targets` collection that polling has previously populated. Reviewing the trap chain (`traps.py:289-294`), there is **no `enrich` step in the trap chain**. Trap events go `trap → prepare → send`; they are NOT joined with polling-derived attributes.

This is consistently framed throughout this document as a **gap**, not a design strength: §15 ("operational toil"), §17 weakness #12 ("Trap and polling-derived data are not joined"), and the operator-facing summary all describe the absence as something that pushes work to Splunk SPL. Whether to consider this an intentional decoupling decision or a missing feature is, ultimately, an operator-judgement; this analysis treats it as a gap because operators consistently want enrichment-at-ingestion in practice.

### Normalization (vendor severity → internal severity)

There is **no severity normalization** in SC4SNMP. The trap event has whatever varbind values the device sent. If a device emits a vendor-specific severity (e.g. `cieEventSeverity`), it appears as a field on the Splunk event with its raw value. Mapping `linkDown.severity=critical` is **the operator's job in Splunk**, typically via a `lookup` or a saved search. SC4SNMP's stance is: "we forward, Splunk classifies." Cross-system comparisons with NMS-style products that ship severity-mapping rules out of the box (OpenNMS, Zenoss, CheckMK, Centreon) belong in the per-system comparison matrix; here we only assert that *SC4SNMP itself* has no such mechanism.

### Deduplication / suppression (keys, windows, rate limits)

There is **no deduplication inside SC4SNMP**. Every received trap → Celery chain → one (or, with `SPLUNK_AGGREGATE_TRAPS_EVENTS=true`, one-aggregated) Splunk event. Dedup is the operator's responsibility downstream, typically via Splunk SPL (`stats count by ...`, `dedup ...`). Splunk HEC's `events`-mode is also at-least-once; consult Splunk's own HEC docs for any built-in idempotency or event-key behaviour at the HEC boundary (this analysis does not claim Splunk-side dedup semantics).

### Routing

There is **no routing**. Every trap → `index=SPLUNK_HEC_INDEX_EVENTS` (default `netops`), `sourcetype=SPLUNK_SOURCETYPE_TRAPS` (default `sc4snmp:traps`). Multiple Splunk indexes per trap class is not supported by SC4SNMP; operators wanting per-vendor or per-severity indexes must do so via Splunk's `props.conf` / `transforms.conf` after ingestion.

### Error handling for malformed PDUs, unknown OIDs, decode failures

| Failure mode | Behaviour |
|---|---|
| **Malformed PDU** | pysnmp raises; the asyncio loop logs and continues; the datagram is dropped (no event emitted). |
| **SNMPv3 auth failure** | The observer callback `authentication_observer_cb_fun` (`traps.py:298-309`) logs `"Security Model failure for device {transport_address}: {errorIndication}"` at ERROR. With `DISCOVER_ENGINE_ID=true`, the captured engineID is included in the log line. No event emitted to Splunk. |
| **Unknown community** | Same pysnmp pathway as auth failure — logged via the observer. |
| **Unknown OID with no MIB** | Trap is still emitted; the OID stays numeric in the Splunk event field name. WARNING logged. |
| **Empty varbinds list** | Controlled by `IGNORE_EMPTY_VARBINDS` env var (default `false` — `snmp/manager.py:61, :140`). Default behaviour processes the trap with an empty varbind set; setting `IGNORE_EMPTY_VARBINDS=true` discards it before emission. |
| **Mongo unavailable at startup** | `wait_for_mongodb_replicaset(logger)` (`traps.py:83, :110`) blocks startup until Mongo is available. |
| **Redis unavailable** | `_ = my_chain.apply_async()` (`traps.py:294`) is **not** wrapped in `try/except`; the `_` only discards the returned `AsyncResult`, it does not catch exceptions. If Redis is unreachable when the enqueue runs, the exception propagates out of `cb_fun` into the pysnmp asyncio dispatcher, which logs and continues the next datagram. **The trap is lost** — there is no on-disk fallback queue inside the trap pod. |
| **HEC unavailable (TCP-level network error)** | `do_send` catches `ConnectionError` from the request and calls `self.retry(countdown=30)` (`splunk/tasks.py:199-202`). The task decorator separately autoretries `ConnectionError, ConnectTimeout, ReadTimeout, Timeout` with `default_retry_delay=5, retry_backoff=True, retry_backoff_max=1, max_retries=60` (`splunk/tasks.py:169-178`). The two paths combine to give a roughly minute-long retry window before the task fails and the trap is lost. |
| **HEC 4xx (400/401/403)** | Logged at ERROR; not retried; trap lost (`splunk/tasks.py:207-208`). |
| **HEC 5xx (500/503)** | Retried with `self.retry(countdown=5)` (`splunk/tasks.py:210-212`). |

The retry semantics are good (60 retries on transient connectivity errors) but there is no persistent disk-backed queue for cases where Redis itself becomes unavailable or fills up. Redis persistence (AOF) is enabled by default in the chart (`values.yaml:638-640` `persistence.aof.enabled: true`) which mitigates Redis restart loss, but cluster-level Redis failure during a trap storm is still a data-loss window.

---

## 6. Data Model & Persistent Storage

### MongoDB collections (database: `sc4snmp`)

| Collection | Purpose | Trap-related? | Evidence |
|---|---|---|---|
| `engine_id_records` | One document per known SNMPv3 security engine ID; schema `{host, security_engine_id}` with unique index on `security_engine_id`. | Yes — trap-specific. Populated by `EngineIdManager.save_engine_id(host, engine_id)` at trap-reception time (`traps.py:285-286`). | `splunk_connect_for_snmp/common/collection_manager.py:224-262`; index creation also in `splunk_connect_for_snmp/common/schema_migration.py:117-118`. |
| `inventory` | One document per polled device (`{address, port, version, community, secret, ...}`). | No (polling only). | `splunk_connect_for_snmp/snmp/tasks.py:80-84`. |
| `profiles` / `groups` | Polling profile and group definitions. | No. | `splunk_connect_for_snmp/common/collection_manager.py:97-218`. |
| `targets` / `attributes` | Polling-derived per-device attribute cache for enrichment. | No (traps are not enriched). | `splunk_connect_for_snmp/enrich/tasks.py:184-198`. |
| `cache_http` | `requests_cache` HTTP cache (TTL 1800s) for mibserver fetches. | Yes — shared with polling. | `splunk_connect_for_snmp/snmp/manager.py:279-287`. |
| `schema_version` | Tracks MongoDB schema migrations. | No (operational). | `splunk_connect_for_snmp/common/schema_migration.py`. |
| `mongolock` | Distributed lock collection for `mongolock`. | No (operational). | `pyproject.toml:43`. |

The trap subsystem touches exactly **one** persistent collection (`engine_id_records`) and one HTTP cache (`cache_http`). It does NOT maintain trap-specific state — no dedup table, no per-source rate-limit table, no alarm table.

### Redis (broker)

Redis is the Celery broker and result backend (via `redbeat` and `mongolock` — `pyproject.toml:42-43`, `splunk_connect_for_snmp/celery_config.py:67-69`). Keys:

- Three logical queues: `traps`, `poll`, `send` (`celery_config.py:94-98`).
- Result backend (per Celery — `task_ignore_result = True`, `result_persistent = False`, `result_expires = 60` at `celery_config.py:89-91`, so Redis is mainly a queue/broker, not a persistent store).
- Priority via `priority_steps = list(range(10))` (`celery_config.py:42, :62`).
- AOF persistence enabled by default in the chart (`values.yaml:638-640`).

### Splunk side (the real datastore)

The data ultimately lives in **Splunk indexes**. SC4SNMP does NOT operate a Splunk instance; it expects:
- `eventIndex: netops` (`values.yaml:101-102`) for trap events.
- `metricsIndex: netmetrics` (`values.yaml:103-104`) for polling metrics.
- `sourcetypeTraps: sc4snmp:traps` (`values.yaml:91-92`).
- `sourcetypePollingEvents: sc4snmp:event` (`values.yaml:94-95`).
- `sourcetypePollingMetrics: sc4snmp:metric` (`values.yaml:97-98`).

Retention, indexing strategy, and field extraction are governed by Splunk-side `indexes.conf`, `props.conf`, and `transforms.conf` — none of which ship with SC4SNMP. Operators must create these in Splunk before SC4SNMP starts sending.

### Migration / upgrade handling

`splunk_connect_for_snmp/common/schema_migration.py` handles MongoDB schema migrations across SC4SNMP versions, including the engineID index. Migrations run automatically at startup; the schema_version document tracks the applied version.

---

## 7. Configuration UX

### Configuration surfaces

| Surface | Path | Trap settings covered |
|---|---|---|
| Helm `values.yaml` | `charts/splunk-connect-for-snmp/values.yaml` | The entire trap deployment: replica counts, HPA, communities, v3 secrets, engine IDs, service type, port, MetalLB IP, autoscaling, log level, IPv6, networkPolicy. |
| ConfigMap → `/app/config/config.yaml` (in-pod) | Generated by chart from `values.yaml` keys `traps.communities` and `traps.usernameSecrets` and `scheduler.customTranslations` | Communities and v3 user list visible at trap startup (`traps.py:382-385`). |
| Kubernetes Secret | Helm `traps.usernameSecrets: [name1, name2, ...]` | Each secret stores SNMPv3 user fields (`userName, authKey, privKey, authProtocol, privProtocol, contextEngineId, contextName`) — `traps/deployment.yaml:158-178`. |
| Environment variables | Set by Helm into the trap pod env | `SNMP_V3_SECURITY_ENGINE_ID`, `DISCOVER_ENGINE_ID`, `INCLUDE_SECURITY_CONTEXT_ID`, `IPv6_ENABLED`, `LOG_LEVEL`, `DISABLE_MONGO_DEBUG_LOGGING`, `PYSNMP_DEBUG`, `SPLUNK_HEC_*` — `traps/deployment.yaml:48-92`. |
| Docker-Compose `.env` | `docker_compose/.env` (per docs) | `TRAPS_PORT`, `TRAP_LOG_LEVEL`, `SNMP_V3_SECURITY_ENGINE_ID`, `DISCOVER_ENGINE_ID`, `INCLUDE_SECURITY_CONTEXT_ID`, `WORKER_TRAP_REPLICAS`, `SPLUNK_AGGREGATE_TRAPS_EVENTS`, `RESOLVE_TRAP_ADDRESS`, `MAX_DNS_CACHE_SIZE_TRAPS`, `TTL_DNS_CACHE_TRAPS` — `docker_compose/docker-compose.yaml:143-166, :223-244`. |
| Docker-Compose `traps-config.yaml` | A separate file mounted at `/app/config/config.yaml` | Communities and v3 secret names — `docs/configuration/traps.md:44-57`. |
| SC4SNMP UI (optional) | Helm `UI.enable: true` (default off) | A web UI for poller/profile/inventory configuration. **Trap configuration is NOT covered by the UI**; operators must edit `values.yaml` for trap settings. Confirmed by inspecting `UI:` section of `values.yaml:20-50` and by absence of `traps` keys in the UI's documented surface. |
| Splunk Flower (Celery admin) | Helm `flower.enabled: true` | Read-only observability into the Celery queue; can revoke individual tasks but cannot reconfigure traps. (`docs/internal/flower.md`.) |

### What the operator sees by default

A fresh `helm install snmp -f traps_enabled_values.yaml ...` with the example values (`examples/traps_enabled_values.yaml:1-13`) gives:

```yaml
splunk:
  enabled: true
  protocol: https
  host: <splunk-host>
  token: 00000000-0000-0000-0000-000000000000
  insecureSSL: "false"
  port: "8088"
traps:
  communities:
    2c:
      - public
      - homelab
  loadBalancerIP: 10.202.6.213
```

This boots the entire stack — mongo, redis, mibserver, scheduler, inventory, 2 trap pods, 2 worker-trap pods, 1 worker-sender pod — and starts listening on UDP/162 (mapped from `LoadBalancerIP:162` to pod 2162). v1/v2c traps with community `public` or `homelab` will land in Splunk index `netops` with sourcetype `sc4snmp:traps`.

### Discoverability of options

- **Helm values** are commented inline in `values.yaml` (which is 700+ lines and serves as the de-facto config schema).
- **No formal JSON schema** (`config_schema.json` does not exist).
- **Documentation site** is generated from `docs/` via `mkdocs.yml` and published at `https://splunk.github.io/splunk-connect-for-snmp/`.
- **Validation**: most validation is implicit (the chart will fail Helm-render or the pod will crash-loop). The chart does not use `helm schema` or JSON-Schema for validation.

### Live reload vs restart

- Trap-pod-level changes (engine ID list, communities, v3 user list): **require a pod restart**. Documented at `docs/configuration/traps.md:30-33`:

```
microk8s kubectl rollout restart deployment snmp-splunk-connect-for-snmp-trap -n sc4snmp
```

- MIB changes: rollout-restart of `worker-trap` and `worker-poller` (`docs/mib-request.md:51-56`).
- Polling profile changes: `PROFILES_RELOAD_DELAY` (default 60s, `manager.py:63`) — worker reloads from MongoDB. Polling-only; not trap-relevant.

### Multi-tenancy / RBAC

- Trap-side: nothing native. There is no per-community tenant boundary, no per-source ACL, no per-v3-user routing. All accepted traps go to one index/sourcetype.
- Kubernetes RBAC governs *who can change the Helm values* and *who can read trap pod logs*; the chart creates a ServiceAccount (`charts/splunk-connect-for-snmp/templates/serviceaccount.yaml`) but otherwise relies on cluster-level RBAC.
- Splunk RBAC (post-ingestion) controls who can search/alert on the trap events — that is a Splunk concern.

---

## 8. Integration with Other Signals

### 8.1 Metrics

Traps in SC4SNMP **are not converted to metrics** in the time-series sense. The `prepare_trap_data` function (`splunk/tasks.py:293-315`) emits only `events` and returns an empty `"metrics": []` list. Even though the underlying `process_snmp_data` (`manager.py:542-581`) classifies varbinds with types `Counter32/64`, `Gauge*`, `TimeTicks` etc., these are folded into the event body as text fields:

```json
{ "IF-MIB.ifInErrors": { "type": "cc", "value": 0, "oid": "...", "time": ... }, ... }
```

This contrasts with polling, where the same `process_snmp_data` classification path emits these counters into Splunk's *metrics* index. Traps go to the *events* index. **A trap with a numerical varbind does not become a counter or gauge in Splunk's metrics store** unless the operator writes a saved search to extract and reingest it.

The OTel/SignalFx output (`splunk/tasks.py:182-184`) similarly only emits `data["metrics"]`, which is empty for traps.

### 8.2 Alerting / Notifications

SC4SNMP **has no alerting engine**. Alerting is the operator's job in Splunk:
- Splunk *saved search alerts* over `index=netops sourcetype=sc4snmp:traps`.
- Splunk ITSI (Splunk IT Service Intelligence) episode reviews.
- Forwarding to external systems (PagerDuty, Opsgenie, ServiceNow) is Splunk-side, via Splunk's alert action plugins.

There is no acknowledge / clear semantics in SC4SNMP itself. A trap is an immutable event in the events index; "ack" lives in whatever ITSM tool the customer integrates Splunk with.

This is consistent with SC4SNMP's positioning: it is an **ingest connector**, not a monitoring system.

### 8.3 Topology

No topology graph. SC4SNMP does **not** build an L2 graph, an L3 graph, or any device-to-device map. Each trap event has a `host` field (source IP or DNS); joining traps to a topology view is a post-ingestion Splunk concern, typically through:
- The community-maintained `SC4SNMP` dashboards in `github.com/splunk/observability-content-contrib/tree/main/dashboards-and-dashboard-groups/SC4SNMP` (referenced from `docs/dashboard.md:4-5`).
- Splunk ITSI service graphs (a separate Splunk premium app).

**No topology-aware suppression**; impossible because there is no topology.

### 8.4 Logs / Events

Each trap becomes one Splunk event (or one aggregated event when `SPLUNK_AGGREGATE_TRAPS_EVENTS=true`). The event schema is:

```json
{
  "time": <epoch-float>,
  "host": "<device-IP-or-DNS>",
  "source": "sc4snmp",
  "sourcetype": "sc4snmp:traps",
  "index": "netops",
  "event": "<json-string-with-all-fields-and-metrics>",
  "fields": { "context_engine_id": "<hex>" }   // optional
}
```

The `event` field is a JSON-serialized object containing every resolved varbind with its `name`, `oid`, `time`, `type`, and `value`. Splunk's `KV_MODE = json` then extracts these into searchable fields automatically.

Searchability: full Splunk SPL is available. Retention: governed by Splunk's `indexes.conf` for the target index.

### 8.5 Northbound Forwarding

SC4SNMP **does NOT re-emit traps**. There is no SNMP-trap output: the only outputs are HEC (HTTPS POST) and optionally OTel/SignalFx. To forward a trap to another NMS, the operator must either:
- Configure the device with two trap destinations (SC4SNMP and the other NMS); or
- Use a Splunk *alert action* triggered by a saved search over `index=netops` to forward to the other system; or
- Run a separate forwarder (`logstash`, `tcpdump|nc`, or another SC4SNMP) in parallel.

Compared to OpenNMS (which has a built-in `snmptrap-northbounder` that re-emits traps as SNMPv2c with vendor-specific OID structures), SC4SNMP is a **terminal sink** for the trap path — the data is now Splunk's.

---

## 9. Severity Model

**SC4SNMP has no internal severity model.** Severity, if any, is whatever varbind the device sent.

- The `process_snmp_data` path classifies varbinds by SNMP type (`cc` counter, `g` gauge, `r` ObjectIdentifier, `f` field, `te` type-error) — see `manager.py:206-221`. **None of these are "severity"** in the alerting sense; they are wire-type classifications used to route varbinds to Splunk's metrics vs events indexes.
- `docs/configuration/snmp-data-format.md:17-37` documents the field-vs-metric type classification but never mentions severity.
- The Splunk side can normalize severity via `props.conf` `EVAL-severity = ...` lookups, but that is out of SC4SNMP.

This is materially weaker than OpenNMS / Zenoss / CheckMK / Centreon and consistent with Logstash / Telegraf / Datadog-Agent — all of which treat trap severity as a downstream concern.

---

## 10. Storm / Volume Handling

### Per-source rate limits

**None at the receiver layer.** SC4SNMP does not implement per-source rate limiting on incoming traps. Every UDP datagram → asyncio handler → Celery enqueue.

### Dedup keys and windows

**None.** Every trap → unique Splunk event.

### Circuit breakers

**None on the trap path.** The sender has retry-with-backoff on HEC failures (60 retries, exponential — `splunk/tasks.py:169-178`), which is *the only* circuit-breaker-like behaviour and it's at the HEC boundary, not the trap receiver.

### Storm detection

**None.** A device emitting 10k traps/sec → 10k Celery tasks → 10k HEC events. The only protections are:
- **HPA autoscaling of the trap pod** — available but disabled by default (`values.yaml:510-515`, `traps.autoscaling.enabled: false`). When enabled, scales up to `maxReplicas: 100` at 80% CPU.
- **HPA autoscaling of the worker-trap pod** — available but disabled by default (`values.yaml:332-339`, `worker.trap.autoscaling.enabled: false`). When enabled, scales up to `maxReplicas: 10` at 80% CPU.
- **Redis queue depth** acts as a backpressure buffer (limited by Redis memory).

### Backpressure / queue management

- Celery `prefetch_count` (default `1` per worker — `celery_config.py:32`, `task_acks_late = True`) ensures workers don't grab more than they can process.
- `worker.trap.prefetch: 30` in the chart (`values.yaml:323`) gives trap workers more aggressive prefetch since trap tasks are smaller.
- `task_acks_late = True` (`celery_config.py:83`) re-queues tasks if a worker dies mid-task.
- `task_reject_on_worker_lost = True` (`celery_config.py:86`).
- `task_time_limit = CELERY_TASK_TIMEOUT` (default 2400s — `celery_config.py:31, :87`); tasks exceeding this are killed.
- `task_create_missing_queues = False` (`celery_config.py:88`) — only the three named queues exist; misconfigured tasks would fail rather than create a runaway queue.

### Capacity claim (vendor-published)

`docs/architecture/planning.md:23-26`:

> *A single installation of Splunk Connect for SNMP (SC4SNMP) on a machine with 16 Core/32 threads x64 and 64 GB RAM will be able to handle up to **1500 SNMP TRAPs per second**.*

This is a vendor-published number for a **single-host installation** (all pods scheduled onto one machine of the stated size); no source-code benchmark exists in-repo to corroborate. It is consistent with pysnmp's documented throughput for asyncio dispatchers but should be treated as a marketing-vetted ceiling, not a guaranteed floor. The number is per-single-host; whether throughput scales horizontally in a multi-host cluster with HPA enabled is plausible from the architecture (independent trap pods behind a UDP Service, independent worker pods consuming Redis queues, independent sender pods POSTing to HEC) but **is not benchmarked in-repo**; multi-host capacity planning requires the operator's own load test.

### Brutal honesty on storm handling

SC4SNMP is **heavily reliant on Splunk being able to keep up**. The trap pod will happily enqueue 10k traps/sec into Redis; the worker-trap pods will happily produce events; the sender will happily POST chunks of 50 to HEC. If Splunk's HEC backpressures or fails, the sender retries — and the exact paths are worth being precise about (the retry surface combines decorator behaviour with explicit in-function retries; we do not derive a single time budget):

- **Decorator-level autoretry** for `ConnectionError, ConnectTimeout, ReadTimeout, Timeout`: `default_retry_delay=5, retry_backoff=True, retry_backoff_max=1, max_retries=60` (`splunk/tasks.py:169-178`).
- **Explicit retry inside `do_send`** for a `ConnectionError` during the HTTPS POST: `self.retry(countdown=30)` (`splunk/tasks.py:199-202`).
- **Explicit retry inside `do_send`** for HTTP `500`/`503` responses: `self.retry(countdown=5)` (`splunk/tasks.py:210-212`).
- **No retry, log only** for HTTP `400/401/403` (`splunk/tasks.py:207-208`) and any other non-200/202 code (`splunk/tasks.py:214-215`).

Celery's behaviour under combined explicit + decorator retries (5.5.3) is non-trivial to derive precisely from the source; the practical effect is that *several* retries happen over *tens of seconds* before the task is declared failed, with the upper bound dominated by `max_retries=60` and the per-retry countdown chosen by whichever code path triggers the retry. An HEC outage longer than that window puts the trap at risk of loss. During a 10k traps/sec storm with HEC degraded, this is a meaningful loss surface. Operators should plan with Splunk's documented HEC ingest limits, ensure Redis has memory headroom, and not rely on SC4SNMP to absorb extended HEC outages cleanly.

> **Task time limit.** `CELERY_TASK_TIMEOUT` defaults to `2400` seconds (`splunk_connect_for_snmp/celery_config.py:31`) and is wired into `task_time_limit` (`celery_config.py:87`). A task that exceeds 40 minutes — which the 64-second retry window above cannot do, but a stuck HEC POST or stuck MIB resolution could — is killed and the trap is dropped.

---

## 11. Security

### SNMPv3 USM support

- **Auth protocols** (`splunk_connect_for_snmp/snmp/const.py:18-26`): MD5, SHA (SHA-1), SHA224, SHA256, SHA384, SHA512.
- **Privacy protocols** (`const.py:28-38`): DES, 3DES, AES (=AES128), AES128, AES192, AES192BLMT (Blumenthal), AES256, AES256BLMT.
- **Security level**: implied by which keys are present in the secret. `auth_key` + `priv_key` → `authPriv`. `auth_key` only → `authNoPriv`. Neither → `noAuthNoPriv`.
- Engine ID handling: see §3.

### DTLS / TLSTM support

**Not supported.** Only `pysnmp.carrier.asyncio.dgram.udp` and `udp6` are wired up (`traps.py:46`). RFC 5953 (TLSTM) and RFC 6353 (DTLS) require `pysnmp.carrier.asyncio.dgram.tls*` modules which are not used. This is consistent with most peers; only Net-SNMP and a few commercial systems implement TLSTM.

### Credential storage

- **v1/v2c communities**: stored as plaintext in the ConfigMap-mounted `/app/config/config.yaml` (`traps.py:382-385`). The ConfigMap itself is etcd-stored in plaintext (standard k8s caveat). **NOT** in a Secret. Operators concerned about community-string exposure must apply kubectl-encryption-at-rest or external secret managers.
- **v3 secrets**: stored in **Kubernetes Secrets** (`traps/deployment.yaml:158-178`). Each secret has keys `userName, authKey, privKey, authProtocol, privProtocol, contextEngineId, contextName`. The Helm chart mounts each as `/app/secrets/snmpv3/<secret>/<key>` and the trap pod reads them via `get_secret_value(...)` (`traps.py:391-413`, `snmp/auth.py:42-52`).
- **Splunk HEC token**: stored as a Kubernetes Secret. Three modes:
  - Plaintext in `splunk.token` (creates a Secret automatically — `values.yaml:71-74`).
  - `splunk.tokenSecretRef.{name,key}` referencing an existing Secret (`values.yaml:76-82`).
  - `splunk.tokenFilePath` reading from a file path (Vault Agent injector pattern — `values.yaml:83-87`, `splunk/tasks.py:93-108`).
- **mTLS for HEC** (since 1.x): client cert/key/CA paths mounted via Secret (`splunk/tasks.py:42-44, :150-159`; `docs/mtls.md`). Splunk 10's new HEC encryption is supported.
- **TLS server-cert verification for HEC defaults to OFF.** `splunk/tasks.py:82-85`:
  ```python
  if human_bool(os.getenv("SPLUNK_HEC_INSECURESSL", "yes"), default=True):
      SPLUNK_HEC_TLSVERIFY = False
  else:
      SPLUNK_HEC_TLSVERIFY = True
  ```
  The **Python-code default** (env var unset) is `SPLUNK_HEC_INSECURESSL=yes` → `SPLUNK_HEC_TLSVERIFY=False`, which **disables verification of the HEC server certificate** at the `requests.Session` level (`splunk/tasks.py:141, :145`). The **Helm chart default** is `splunk.insecureSSL: "false"` (`values.yaml:93`), so a stock chart install lands on TLS-verifying. The mismatch matters in two scenarios: (a) operators who explicitly flip `insecureSSL: "true"` to work around a Splunk Cloud cert issue and then leave it that way; (b) operators who run the Python directly (Docker-Compose without `.env` populating `SPLUNK_HEC_INSECURESSL`, or hand-managed pods bypassing Helm) — both paths silently disable TLS verification. `docs/security.md` does not flag this default.

### Access control on the trap subsystem itself

- **Receiver-side**: no IP allowlists, no per-source ACL. Whatever community/USM-user matches gets accepted.
- **Cluster-side**: a `NetworkPolicy` template exists (`charts/splunk-connect-for-snmp/templates/traps/networkpolicy.yaml`) and is gated by `traps.networkPolicy` (chart default: `false`). Important Kubernetes-semantics note: the template defines `policyTypes: [Ingress, Egress]` with **no `ingress:` or `egress:` rules** and a `podSelector` matching the traps-labeled pods. Per the NetworkPolicy spec, that is *not* "default-open" — when this NetworkPolicy is rendered (i.e. `traps.networkPolicy=true`), the selected trap pods receive **deny-all ingress and egress** unless another NetworkPolicy in the namespace explicitly allows traffic. With the chart default (`traps.networkPolicy=false`), no NetworkPolicy is created at all, and whatever the cluster's namespace-level baseline NetworkPolicies dictate is what applies to the trap pods (typically open if no baseline policy exists). There are no values-driven knobs in this template for declaring allowed peers or ports; operators who want a real restriction must author an additional NetworkPolicy themselves (or override `traps.networkPolicy` and pair it with explicit `allow-*` policies elsewhere).
- **Pod-level**: drop-all-caps, read-only root, non-root UID (see §2).

### Audit logging

- **Auth failures**: logged at ERROR via the pysnmp observer (`traps.py:298-309`). With `DISCOVER_ENGINE_ID=true` the captured engineID is included.
- **Trap reception (debug)**: trap-pod logs each notification at DEBUG (`traps.py:266-268, :277`).
- **No structured audit trail** — log lines, not JSON audit records. Splunk-side logging of the pod's stderr is documented at `docs/dashboard.md` (the trap dashboard reads the pod logs from a `_internal`-style Splunk index).

### Vendor security stance (`docs/security.md:1-14`)

Worth quoting because it is unusually candid:

> *SNMP is a protocol widely considered to be risky and requires threat mitigation at the network level.*
> *Do not expose SNMP endpoints to untrusted connections such as the internet or general LAN network of a typical enterprise.*
> *Do not allow SNMPv1 or SNMPv2 connections to cross a network zone where a man in the middle interception is possible.*
> *Many SNMPv3 devices rely on insecure cryptography including DES, MD5, and SHA. Do not assume that SNMPv3 devices and connections are secure by default.*

This is the **right** position and the docs say it plainly.

---

## 12. Trap Simulation & Testing (in-source evidence)

### Unit tests

| Path | Coverage |
|---|---|
| `test/test_traps.py` (49 lines) | `TestDecodeSecurityContext` — 4 tests of `decode_security_context()`: valid SNMPv3 message (a real binary PDU at `:26-29`, decoded to engineID `800000c1010a010fc4` and username `snmp-poller`), invalid version, ASN.1 error, None input. |
| `test/snmp/test_tasks.py` (lines 124-333) | 6 trap-related tests of `trap()` task: basic, with context_engine_id, two MIB-retry-translation paths (success and failure), reverse-DNS lookup, plus 3 `format_ipv4_address` tests (`:336-358`). |
| `test/splunk/test_prepare.py` | `test_prepare_trap` (`:16`), `test_prepare_aggregated_trap` (`:97`), plus a `TestPrepareTrapData` class (`:610-680`) covering `prepare_trap_data` with extra fields. |
| `test/splunk/test_send.py`, `test_value_as_best.py`, `test_read_hec_token.py` | HEC delivery (shared with polling). |
| `test/snmp/test_manager.py` | Tests for `process_snmp_data` — the varbind classification and group-key path that the `trap()` task calls at `snmp/tasks.py:164`. Indirectly trap-relevant: covers the SNMP-wire-type classification (`cc`, `g`, `r`, `f`, `te`) that determines how a trap's varbinds end up in the event body. |
| `test/snmp/test_*.py` (other shared) | MIB resolution, group key building — covers code paths used by traps but not specifically asserted under a trap-named test. |

Total trap-flavored unit-test coverage: roughly 13-15 test functions across 3 files, exercising the engine-ID decode, the trap task IO path, MIB retry, reverse DNS, prepare-trap, and aggregate-trap behaviour. **Reception itself (the pysnmp asyncio path) is mocked out** in unit tests — see `test/test_traps.py:5-17` where pysnmp, celery, and opentelemetry are wholesale `MagicMock()`-patched. **The asyncio receiver is not unit-tested.**

### Integration tests

`integration_tests/tests/test_trap_integration.py` (283 lines) — `pytest.mark.part6`:

| Test | What it does |
|---|---|
| `test_trap_v1` | Sends a v1 trap (`mpModel=0`) with community `publicv1`, two varbinds, then SPL searches `index="netops" sourcetype="sc4snmp:traps"` for one event. |
| `test_trap_v2` | Same with `mpModel=1` and community `homelab`. |
| `test_added_varbind` | Sends a v2c trap with `sysDescr=test_added_varbind`, searches Splunk for `"SNMPv2-MIB.sysDescr.value"="test_added_varbind"`. |
| `test_many_traps` | Sends 5 v2c traps in a loop, asserts result_count == 5. The most basic storm validation. |
| `test_more_than_one_varbind` | Tests multi-varbind in a single trap. |
| `test_loading_mibs` | Sends a trap with an enterprise OID (`1.3.6.1.4.1.15597.1.1.1.1`) that requires loading AVAMAR-MCS-MIB — validates the lazy-MIB-load path. |
| `test_trap_v3` | Tests SNMPv3 with USM secret `secretv4` (the test creates the secret via `create_v3_secrets_microk8s()` / `create_v3_secrets_compose()`, upgrades the chart, waits for pod init, sends a v3 trap with engineID `80003a8c04`, password-based SHA auth + DES priv, and searches for the result in Splunk). |

These integration tests are runnable against either a MicroK8s or Docker-Compose deployment — `request.config.getoption("sc4snmp_deployment")` switches branches (`test_trap_integration.py:255-263`). They require an actual Splunk Enterprise / Splunk Cloud instance available; the fixture is `setup_splunk` (defined elsewhere). This is **real end-to-end testing** of the entire pipeline.

### Sample trap fixtures included

The real binary SNMPv3 PDU at `test/test_traps.py:26` is the only **captured-packet binary fixture** in the repo, used for unit-testing the engineID-decode logic.

A second fixture file `integration_tests/snmpsim/data/variation/notification.snmprec` (3 rows: v1 trap, v2c trap, v3 inform) exists for the snmpsim notification simulator — but this is a *sending*-side fixture used to drive snmpsim to emit traps and informs at *fake agents*, not a reception-side fixture exercising SC4SNMP's NotificationReceiver against captured wire-format PDUs. The integration tests themselves (`integration_tests/tests/test_trap_integration.py:37-77`) generate traps via `pysnmp.hlapi.sendNotification(...)` (synthesised PDUs constructed in-process), not by replaying captured packets.

### Tools shipped for trap simulation

No simulator ships with SC4SNMP. Integration tests use `pysnmp.hlapi.sendNotification(...)` directly (`test_trap_integration.py:37-77`) to emit traps. Operators replicating this for manual testing typically use Net-SNMP's `snmptrap` CLI (`docs/configuration/traps.md:82-87` shows the canonical snmptrap incantation).

### CI workflow

- `.github/workflows/ci-main.yaml:17-31` — the main CI workflow triggers on `push` (to `main` / `develop`), `pull_request` (against `main` / `develop`), and `workflow_call`. There is **no `schedule:` trigger**. Unit tests run on every push and PR. The integration-test stages (`.github/workflows/ci-main.yaml:155-160, :194-199`) gate themselves with `if: "contains(needs.integration-tests-check.outputs.commit_message, '[run-int-tests]') || github.ref_name == 'develop'"` — i.e. integration tests run only when a contributor opts in via `[run-int-tests]` in the commit message, or when the change has landed on the `develop` branch.
- `.github/workflows/cd-dashboard-release.yaml:17-20` — separate workflow that ships `dashboard/dashboard.xml` as a release artifact.
- Code coverage tracked via `codecov.io/gh/splunk/splunk-connect-for-snmp` (badge in `README.md:5-6`).
- Static analysis via `sonar-project.properties` and `.fossa.yml`.

### Brutal assessment of test coverage

- **Unit tests cover the task functions but mock the asyncio receiver wholesale.** The actual pysnmp `NotificationReceiver` integration is not unit-tested. Issues like "the cb_fun is registered twice" or "the asyncio loop blocks on Mongo connection" would not be caught by unit tests.
- **Integration tests are excellent** — they exercise the full deployment (Helm upgrade, secret rotation, v3 auth, real Splunk searches) but they require a Splunk back-end and a real Kubernetes/Compose deployment, so they are not run on every commit.
- **No fuzz testing of trap PDU decoding.** A malformed PDU that crashes pysnmp would not be caught.
- **No load testing in-repo.** The "1500 traps/sec" capacity claim is not backed by an in-repo benchmark.

---

## 13. Out-of-the-Box Coverage (defaults)

### MIBs bundled

- **In the worker code**: `DEFAULT_STANDARD_MIBS = ["HOST-RESOURCES-MIB", "IF-MIB", "IP-MIB", "SNMPv2-MIB", "TCP-MIB", "UDP-MIB"]` (`manager.py:69-76`).
- **In the mibserver pod**: the full `pysnmp/mibs` catalogue at whatever tag the chart pulls. The current catalogue is queryable at `https://pysnmp.github.io/mibs/index.csv`.
- **Vendor coverage**: depends on the mibserver release. `docs/mib-request.md` does not enumerate a fixed vendor list; the operator must check `index.csv` for the MIBs they need.

### Severity rules bundled

**None.** See §9.

### Dedup defaults

**None.** See §10.

### Vendor packs / integration packages

**None at the SC4SNMP level.** There is no notion of a "Cisco pack" or "Juniper pack" inside SC4SNMP — at the trap layer.

However, on the **Splunk side** the operator typically also installs Splunk apps that supply field extractions, lookups, and dashboards for specific vendor MIBs (e.g. Splunk Add-on for Cisco, Splunk App for ITSI). These are out of SC4SNMP scope but are part of the practical deployment.

### Sample/preset dashboards or reports

- **In-repo, first-party dashboard**: `dashboard/dashboard.xml` ships in this repository, is released as a build artifact by `.github/workflows/cd-dashboard-release.yaml:17-20`, and `docs/dashboard.md:31-35` documents that operators download the XML from a release attachment and import it into Splunk. It contains trap-relevant panels — `"SNMP trap status"` at `dashboard.xml:148` and `"SNMP trap authorisation"` at `dashboard.xml:168` — that read directly from the trap pods' logs (the SPL is keyed on `sourcetype="*:container:splunk-connect-for-snmp-*"` and the `splunk_connect_for_snmp.snmp.tasks.trap` task name).
- **Trap-content dashboards (Splunk-side, separate repo)**: more elaborate operator dashboards live at `github.com/splunk/observability-content-contrib/tree/main/dashboards-and-dashboard-groups/SC4SNMP` (referenced from `docs/dashboard.md:4-5`). These include "Network Devices Dashboard" and "SNMP Agents Dashboard" and are intended for Splunk Classic Dashboards import.
- **SC4SNMP self-monitoring dashboards (separate repo)**: the same `observability-content-contrib` repo carries trap-pipeline-health sections (*"SNMP traps authorisation"*, *"SNMP trap status"* per `docs/dashboard.md:65-74`) powered by SC4SNMP pod logs shipped to a Splunk metric index via `splunk-logging` (Docker driver) or `splunk-otel-collector` (K8s) per `docs/dashboard.md:24-27`. Distinct from the first-party in-repo `dashboard.xml`: the in-repo dashboard is panel-level operator-visible and ships with the release; the `observability-content-contrib` dashboards are more elaborate but live outside the SC4SNMP release artifact set.
- **Default Helm flags for self-monitoring**: `flower.enabled: false`, `UI.enable: false` — both have to be opted into.

### Defaults summary

A **bare** `helm install snmp splunk-connect-for-snmp/splunk-connect-for-snmp` (no values overrides, no example file applied):
- Listens on UDP/162 via a LoadBalancer Service (no MetalLB IP allocated, so the Service stays `Pending` until the operator either sets `traps.loadBalancerIP` or flips to `NodePort`).
- **Trap communities are empty** (`values.yaml:477-478` `communities: {}`). v1/v2c traps are rejected at the pysnmp community-check stage until the operator adds a community. The wider `docs/configuration/traps.md:7` mentions `public` as a *documentation example*, not as a chart default.
- 2 trap pods, 2 worker-trap pods, 1 worker-sender pod (replica defaults; HPAs available but disabled — see §10).
- 6 standard MIBs preloaded in the worker; lazy resolution from `pysnmp/mibs` catalogue for the rest.
- All traps → Splunk index `netops`, sourcetype `sc4snmp:traps`.
- No dedup, no severity normalization, no rate limit, no UI, no Flower.
- HEC: `splunk.insecureSSL: "false"` per chart default (`values.yaml:93`) — TLS verification *enabled* under Helm. The underlying Python code default differs (env unset → TLS off; see §11).

The "documented quick-start" install in `examples/traps_enabled_values.yaml:1-13` layers on top of that bare default: it fills `2c: [public, homelab]` so v2c traps with either community are accepted, sets `splunk.insecureSSL: "false"` (enabling TLS verification), and assigns a `traps.loadBalancerIP`. Operators who run the bare chart without this example file get nothing accepted on UDP/162 until they configure a community.

---

## 14. User Customization Surface

### How users add custom OID handlers

**They don't, at the SC4SNMP level.** SC4SNMP does not expose a per-OID handler hook. All traps go through the same `trap → prepare → send` pipeline; there is no "if OID matches X, do Y" hook.

The only OID-level customization is **custom translations** (`scheduler.customTranslations` in `values.yaml`, processed by `splunk_connect_for_snmp/common/custom_translations.py:29-39` and applied at `splunk/tasks.py:318-337`):

```yaml
scheduler:
  customTranslations:
    IF-MIB:
      ifInDiscards: my_in_discards_renamed
```

This **renames** `IF-MIB.ifInDiscards` to `IF-MIB.my_in_discards_renamed` in the Splunk event. It is a textual rename only — it does NOT change behaviour, severity, dedup, or routing.

Per-OID action logic happens **in Splunk** via saved searches or `props.conf`/`transforms.conf`.

### Custom MIBs

Three paths (see §4): submit upstream, pin mibserver tag, local-MIBs mount. The third is the most flexible.

### Custom severity rules

Not at SC4SNMP. In Splunk via `EVAL-severity = case(...)` lookups.

### Custom dedup rules

Not at SC4SNMP. In Splunk via `| dedup ...` or `| stats latest(...) by ...` saved searches.

### Helm-level operational toggles

The trap subsystem is configured almost entirely through Helm values; the runtime env vars are derived from `values.yaml` by `charts/splunk-connect-for-snmp/templates/worker/_helpers.tpl:206-220` (the `environmental-variables-trap` template). The notable Helm-level structured sections include:

- `traps.communities`, `traps.usernameSecrets`, `traps.securityEngineId`, `traps.discoverEngineId`, `traps.includeSecurityContextId`, `traps.aggregateTrapsEvents` — the v1/v2c/v3 trap-acceptance surface.
- `traps.service.{type, port, nodePort, usemetallb, metallbsharingkey, annotations}` — the LoadBalancer/NodePort wiring.
- `traps.autoscaling.{enabled, minReplicas, maxReplicas, targetCPUUtilizationPercentage}` — HPA for the trap pod (disabled by default).
- `traps.networkPolicy` (boolean) — enable the deny-all NetworkPolicy template (see §11).
- `traps.logLevel`, `traps.disableMongoDebugLogging` — log verbosity for the receiver pod.
- `worker.trap.{replicaCount, concurrency, prefetch, autoscaling}` — sizing for the trap-task worker tier.
- `worker.trap.resolveAddress.{enabled, cacheSize, cacheTTL}` — the Helm-level interface for reverse-DNS lookup of trap source IPs. `enabled: false` by default; when enabled, `cacheSize: 500` and `cacheTTL: 1800` (seconds) — the Helm chart overrides the code-level `MAX_DNS_CACHE_SIZE_TRAPS=100` default (see §5 stage 1 step 5).

### Plugin / extension model

**No plugin or extension model exists at the trap layer.** SC4SNMP is a closed pipeline; the only extension points are:
- The MIB catalogue (operator-supplied MIBs).
- The Splunk HEC sourcetype / index (chosen via env vars).
- The custom-translations YAML (renames only).
- Replacing the entire HEC endpoint with another `requests`-compatible URL via env (`SPLUNK_HEC_HOST`, etc. — `splunk/tasks.py:38-71`).

For richer logic, operators are expected to fork.

### API surface for automation

There is **no REST API** for trap configuration. The only "API" is the Helm `values.yaml` (declarative config). The SC4SNMP UI (off by default) exposes a CRUD GUI for polling profiles/groups/inventory — **not for trap config**.

The Celery Flower UI (off by default) exposes the broker queues and is read-only for trap ops; tasks can be revoked but not authored.

---

## 15. End-User Value Analysis

### Day-1 value with default config

After `helm install snmp -f values.yaml ...` (with `splunk.host`, `splunk.token`, `traps.communities`, and `traps.loadBalancerIP` populated), an operator gets:
- A UDP/162 listener that accepts v1/v2c traps with the configured communities.
- Every accepted trap is enqueued onto Celery and forwarded to Splunk HEC as a JSON event in `index=netops sourcetype=sc4snmp:traps`. Latency is not asserted in source (integration tests use sleeps of 2-15 seconds before issuing the verification Splunk search — `integration_tests/tests/test_trap_integration.py:99-105, :128-135, :273-281` — and no test contains a latency-SLO assertion); operator expectation under no load is "a few seconds" but no in-repo benchmark establishes a tighter bound.
- Standard MIBs (IF-MIB, SNMPv2-MIB, TCP-MIB, UDP-MIB, IP-MIB, HOST-RESOURCES-MIB) are pre-loaded; enterprise MIBs are lazy-loaded from the mibserver if available.
- Fixed-replica scaling: `traps.replicaCount: 2` and `worker.trap.replicaCount: 2` (`values.yaml:472, :316`). **HPA templates exist but are disabled by default** (`traps.autoscaling.enabled: false` at `values.yaml:510-511`; `worker.trap.autoscaling.enabled: false` at `values.yaml:332-340`). Horizontal scaling on CPU only kicks in after the operator flips these flags.
- Splunk's full SPL is now available for searching the trap data.

That's it. There is no out-of-the-box alert, dashboard, or correlation in SC4SNMP itself. The dashboard and self-monitoring content is in a **separate repo** (`observability-content-contrib`) and requires manual import into Splunk.

### What requires customization

Almost every "monitoring" use case:
- Severity classification → Splunk-side.
- Alerting → Splunk-side (saved searches).
- Topology join → Splunk-side or Splunk ITSI.
- Dashboards → manual import from `observability-content-contrib`.
- Vendor MIBs not in `pysnmp/mibs` → submit upstream, pin a tag, or mount local MIBs.
- Multi-tenant routing (different indexes per source) → Splunk-side via `props.conf`/`transforms.conf` (SC4SNMP only writes one index).
- Northbound trap forwarding → not supported; configure two trap destinations on the device.
- Trap dedup → Splunk-side SPL.
- Per-source rate limiting → Splunk-side or upstream firewall.

### Learning curve

- **For a team already on Splunk + Kubernetes**: low. They know SPL, they know Helm, they know how to manage Secrets and Services. SC4SNMP slots into their existing operational model in an hour.
- **For a team new to either**: high. The operator must learn MicroK8s (or Docker-Compose), Helm values syntax, Kubernetes Secrets management, MetalLB or cloud-LB UDP support, the pysnmp/mibs ecosystem, Splunk HEC tokens, and Splunk SPL — before they can read their first trap. This is a **much heavier prerequisite stack** than any other system in the cohort.

### Operational toil

- **MIB management**: requires a separate workflow (PR to `pysnmp/mibs` or mount local MIBs and rollout-restart). Manageable but not zero-touch.
- **Engine ID management for v3**: well-mitigated by `DISCOVER_ENGINE_ID=true` + the MongoDB-backed persistence. **This is the strongest UX win in SC4SNMP's trap layer.**
- **Helm upgrades**: standard k8s discipline (validate values, dry-run, watch rollout, possibly schema_migration runs on the next pod start).
- **Cluster-level concerns**: MongoDB replica-set health, Redis Sentinel quorum, mibserver availability, MetalLB pool exhaustion — these are *new* operational concerns the operator inherits.
- **Splunk-side health**: HEC quota, index sizing, retention.

### Visibility into the pipeline's own health

- The trap pod logs at DEBUG/INFO/WARNING/ERROR depending on `LOG_LEVEL`.
- **Self-monitoring dashboards exist but require explicit installation** (`docs/dashboard.md`). They depend on shipping the pods' logs into a Splunk metric index via `splunk-logging` (Docker) or `splunk-otel-collector` (K8s).
- Celery Flower (off by default) exposes queue depth.
- MongoDB and Redis can be scraped via their standard exporters (mongodb-metrics, redis-exporter).
- No first-party Prometheus `/metrics` endpoint in SC4SNMP itself for trap rate, dedup rate, decode failures, MIB-resolution misses, or HEC retry rate. There is, however, **OpenTelemetry SDK initialization** at trap-pod startup — `traps.py:44-46` constructs a `TracerProvider`, and `pyproject.toml:30-33` brings in `opentelemetry-api`, `opentelemetry-sdk`, `opentelemetry-instrumentation-celery`, and `opentelemetry-instrumentation-logging`. The Celery instrumentation produces distributed traces with per-task duration, queue name, and status for `trap`, `prepare`, and `send`, and the logging instrumentation injects trace context into log lines. Custom trap-specific metrics (trap rate, decode-failure counters, MIB-miss counters, HEC-retry counters) are **not** emitted; only the framework-level Celery task spans are available, and operators must supply OTel exporter configuration to route the trace stream somewhere visible.

### Compared to a single-process trap receiver

The bar here is set by single-process systems (Telegraf, Zabbix-trapper, snmptrapd → snmptt). SC4SNMP is **dramatically heavier** in its operational surface and only justified if the team is already paying the Splunk + K8s tax. The trade-off it makes is: in exchange for that weight, you get Splunk's full search, retention, RBAC, and dashboarding for "free" once the data is there.

---

## 16. Strengths

1. **Kubernetes-native, container-first** — `splunk/splunk-connect-for-snmp @ fdd4c74e :: charts/splunk-connect-for-snmp/templates/`. Helm chart with HPA templates on both the listener and worker tiers (disabled by default but available), a NetworkPolicy template (also default-off; deny-all when enabled — see §11), PodDisruptionBudget, and a security context with drop-all-caps + read-only-root + non-root UID. This is a strong fit for Kubernetes-first shops and avoids the root+raw-sockets compromises that legacy SNMP-trap receivers commonly make.

2. **SNMPv3 engineID discovery reduces manual configuration** — `:: splunk_connect_for_snmp/traps.py:229-258`. SC4SNMP intercepts the raw datagram, decodes the engineID via pyasn1, dynamically registers a new (user, engineID) pair with pysnmp, and persists it to MongoDB so it survives restarts. Operators normally have to enumerate every device's engineID by hand for pysnmp-based receivers; this feature removes that step for the population of devices whose USM username already matches a configured user.

3. **Strict separation of receiver and worker scaling** — `:: charts/splunk-connect-for-snmp/templates/traps/hpa.yaml` and `:: charts/splunk-connect-for-snmp/templates/worker/trap/hpa.yaml`. Two independent HPAs let listener CPU (decode + asyncio + Celery enqueue) and worker CPU (MIB resolution + HEC formatting) scale separately. Most peers conflate the two.

4. **Real integration test coverage** — `:: integration_tests/tests/test_trap_integration.py:80-283`. Tests are run against actual Splunk instances and exercise the full Helm upgrade flow including v3 secret creation and rollout-restart. This is more rigorous than most peers and gives real-deployment confidence.

5. **Per-pod security posture is genuinely hardened** — `:: charts/splunk-connect-for-snmp/templates/traps/deployment.yaml:33-41`. drop-all-caps, read-only root, non-root UID, no privileged port binding inside the pod. Most other SNMP-trap receivers either run as root or rely on a SUID/setcap wrapper; SC4SNMP solves it cleanly via the Service abstraction.

6. **Strong retry semantics at the HEC boundary** — `:: splunk_connect_for_snmp/splunk/tasks.py:169-216`. 60 retries with exponential backoff on connection/timeout errors; explicit handling of 4xx vs 5xx; the Sender worker tier scales independently of the trap pipeline. This isolates trap-pipeline performance from HEC quirks.

7. **Decoupled from polling on the event path** — `:: splunk_connect_for_snmp/traps.py:289-294`. The trap chain is `trap → prepare → send`. Trap events do not pass through `enrich` (which is polling-only) and do not block on polling state. A trap is in Splunk in ~1-2s regardless of polling queue depth.

8. **Honest docs** — `:: docs/security.md:1-14`. The security page explicitly warns about SNMPv1/v2c risk, MD5/DES weakness, and SNMP-protocol exposure. The architecture docs are clear about what SC4SNMP does and does not do.

9. **Flexible HEC token sourcing** — `:: splunk_connect_for_snmp/splunk/tasks.py:93-108` and `:: charts/splunk-connect-for-snmp/values.yaml:71-87`. Three modes (plaintext, existing Secret reference, file-mounted from Vault Agent) cover the realistic spectrum of secret-management practices.

10. **Worker-loss tolerance at the Celery boundary** — `:: splunk_connect_for_snmp/celery_config.py:83-86`. `task_acks_late = True`, `task_reject_on_worker_lost = True`. A worker dying mid-task re-queues the task on a peer instead of dropping it. This gives at-least-once semantics for the SC4SNMP-internal pipeline (a single trap can be processed twice if a worker dies between HEC POST and ack); whether duplicates are then de-duplicated downstream in Splunk depends on Splunk-side configuration not controlled by SC4SNMP. We make no claim about Splunk's own duplicate-handling behaviour.

---

## 17. Weaknesses / Gaps

1. **Massive infrastructure footprint for "a UDP listener"** — Helm chart `:: charts/splunk-connect-for-snmp/values.yaml`. To accept a trap, SC4SNMP requires MongoDB (StatefulSet, 5Gi PVC, ChangeMe123 default password), Redis (Deployment or Sentinel cluster, 5Gi PVC, AOF persistence), mibserver (separate Deployment + HTTP cache), trap pods, worker-trap pods, worker-sender pods, and (optionally) UI front/back pods + Flower. Compare to Telegraf or snmptrapd — a single process with a config file. SC4SNMP's footprint is justified only for teams already paying the Splunk + K8s cost.

2. **No native severity model, dedup, rate limit, or storm protection** — see §9 and §10. SC4SNMP is a forwarder, not a monitor. A single misconfigured device emitting 10k traps/sec will fill Splunk's index unless rate-limited by Splunk-side licensing or upstream firewall. The vendor "1500 traps/sec" claim (`docs/architecture/planning.md:24-26`) is a hardware ceiling, not a graceful-degradation guarantee.

3. **Trap loss windows during dependency failures** — see §5 "Error handling" table. If Redis is unavailable when the trap arrives, the trap is lost (no on-disk fallback in the trap pod). If HEC stays down longer than ~60s of retry backoff per task, traps queued in Redis time out and are lost. Operators must build their own redundancy for these cases.

4. **No northbound trap forwarding** — SC4SNMP is a terminal sink for the SNMP protocol; traps cannot be re-emitted as SNMPv2c to a peer NMS. Compared to OpenNMS's `snmptrap-northbounder` (which does exactly that), SC4SNMP forces dual-trap-destination on the device side.

5. **No trap-side configuration UI** — `:: charts/splunk-connect-for-snmp/values.yaml:20-50` (UI section). The SC4SNMP UI covers polling profiles, groups, and inventory; it does not cover traps. Trap-side changes (communities, v3 users, engineIDs) are pure YAML + `kubectl rollout restart`.

6. **No first-party trap-pipeline metrics** — there is no Prometheus `/metrics` endpoint exporting trap-rate, decode-failure-rate, MIB-miss-rate, or HEC-retry-rate. OpenTelemetry SDK is initialised and the Celery instrumentation produces task-level spans (see §15), but no custom trap-domain counters are emitted. Self-monitoring relies on shipping pod logs into a Splunk metric index and importing dashboards (the in-repo `dashboard/dashboard.xml` or the separate `observability-content-contrib` set). Most modern systems would expose these as native Prometheus or OTel-metrics streams.

7. **MongoDB default credentials are static defaults** — `:: charts/splunk-connect-for-snmp/values.yaml:564` `rootPassword: "ChangeMe123"`. The chart trusts operators to override this; many will not. (Splunk's defence is that Mongo is internal-only by NetworkPolicy, but the chart's trap NetworkPolicy is disabled by default *and* would deny-all-traffic when enabled — see §11; cluster-wide Mongo isolation is not guaranteed out of the box.)

8. **Internal port `2162` is hard-coded** — `:: splunk_connect_for_snmp/traps.py:372, :379`. Cannot be changed without forking. Most operators won't care (Service port is configurable), but in shared-host scenarios (multiple SC4SNMP instances on one node, NodePort conflict) this is a constraint.

9. **INFORM reception is undocumented and untested** — see §3. SC4SNMP's docs do not mention INFORM and the integration tests send only `notifyType="trap"` (`integration_tests/tests/test_trap_integration.py:40, :58`). pysnmp's `NotificationReceiver` does generally accept INFORM PDUs but whether SC4SNMP correctly acknowledges them (so the sender stops retransmitting) is unverified in this analysis. Operators relying on INFORM should validate end-to-end before production rollout.

10. **Trap events get the same Splunk index/sourcetype regardless of source** — see §8.5 "no routing". A multi-tenant deployment (one SC4SNMP serving devices for multiple Splunk tenants) requires multiple SC4SNMP installations (one per index/sourcetype), or Splunk-side `props.conf` magic to re-route on ingestion.

11. **MIB-server is a separate upstream (`pysnmp/mibs`) with its own release cadence** — `:: docs/mib-request.md:13-17`. Adding a vendor MIB requires either an upstream PR (lead time = next pysnmp/mibs release) or a local-MIBs mount (operator-managed). There is no in-product MIB upload UI.

12. **Trap and polling-derived data are not joined** — see §5 enrichment. A trap event for `IF-MIB.linkDown` does NOT carry the device's `IF-MIB.ifAlias` from a recent walk. Operators must join in Splunk SPL across two sourcetypes.

13. **Vendor positioning conflates "trap throughput" with overall capacity** — `docs/architecture/planning.md:23-32`. The 1500-traps/sec and 2750-varbinds/sec numbers are for a single-instance deployment, not normalized to trap-pod replicas, MongoDB instance count, or HEC capacity. Capacity planning at scale requires re-deriving these numbers against actual deployment.

---

## 18. Notable Code or Configuration Examples

### 18.1 The Celery chain that defines the pipeline

`splunk/splunk-connect-for-snmp @ fdd4c74e :: splunk_connect_for_snmp/traps.py:289-294`

```python
my_chain = chain(
    trap_task_signature(work).set(queue="traps").set(priority=5),
    prepare_task_signature().set(queue="send").set(priority=1),
    send_task_signature().set(queue="send").set(priority=0),
)
_ = my_chain.apply_async()
```

This is the **entire** pipeline shape, expressed in seven lines. The trap pod is just a Celery producer.

### 18.2 The dynamic engineID discovery hook

`:: splunk_connect_for_snmp/traps.py:229-242`

```python
class _EngineIDCaptureUdpTransport(udp.UdpAsyncioTransport):
    """UDP transport that extracts SNMPv3 engineID from raw datagram and adds it as V3 user before pysnmp parses."""

    def datagram_received(self, datagram: bytes, transport_address: Any) -> None:
        if DISCOVER_ENGINE_ID:
            engine_id, username = decode_security_context(datagram)
            if engine_id:
                key = _normalize_transport_address(transport_address)
                _engine_id_from_raw_message[key] = engine_id
                if username and any(
                    uc["userName"] == username for uc in _v3_user_configs_for_discovery
                ):
                    _add_v3_user_for_new_engine_id(engine_id)
        super().datagram_received(datagram, transport_address)
```

Subclassing pysnmp's UDP transport to peek at the raw bytes *before* pysnmp parses them is the workaround for pysnmp's eager-registration constraint on (user × engineID) pairs. It is a notable implementation detail and the lever behind §3's dynamic-engineID-discovery feature.

### 18.3 The HEC sender's retry policy

`:: splunk_connect_for_snmp/splunk/tasks.py:169-216`

```python
@shared_task(
    bind=True,
    base=HECTask,
    default_retry_delay=5,
    max_retries=60,
    retry_backoff=True,
    retry_backoff_max=1,
    autoretry_for=[ConnectionError, ConnectTimeout, ReadTimeout, Timeout],
    retry_jitter=True,
)
def send(self, data):
    if SPLUNK_HEC_TOKEN:
        do_send(data["events"], SPLUNK_HEC_URI, self)
        do_send(data["metrics"], SPLUNK_HEC_URI, self)
    if OTEL_METRICS_URL:
        do_send(data["metrics"], OTEL_METRICS_URL, self)
```

Note that `retry_backoff_max=1` caps the inter-retry sleep at 1 second. Combined with `default_retry_delay=5` and the explicit per-path retries inside `do_send` (`countdown=30` for `ConnectionError`, `countdown=5` for HTTP 5xx — `splunk/tasks.py:199-212`), the task survives a moderate HEC outage but eventually fails after `max_retries=60` retries. See §10 for the operational implication during HEC outages.

### 18.4 The Helm chart's pod-security context

`:: charts/splunk-connect-for-snmp/templates/traps/deployment.yaml:33-41`

```yaml
- name: {{ .Chart.Name }}-traps
  securityContext:
    capabilities:
      drop:
        - ALL
    readOnlyRootFilesystem: true
    runAsNonRoot: true
    runAsUser: 10001
    runAsGroup: 10001
```

drop-all-caps + readOnlyRootFilesystem + non-root is the canonical hardened-pod posture and worth quoting because it's the explicit reason SC4SNMP cannot bind 162 inside the container (which forces the Service-port-remap pattern).

### 18.5 The trap event envelope written to HEC

`:: splunk_connect_for_snmp/splunk/tasks.py:293-315`

```python
def prepare_trap_data(work):
    events = []
    for key, data in work["result"].items():
        processed = {}
        if data["metrics"]:
            for k, v in data["metrics"].items():
                processed[k] = v
                processed[k]["value"] = value_as_best(v["value"])
        event = {
            "time": work["time"],
            "event": json.dumps({**data["fields"], **processed}),
            "source": "sc4snmp",
            "sourcetype": SPLUNK_SOURCETYPE_TRAPS,
            "host": work["address"],
            "index": SPLUNK_HEC_INDEX_EVENTS,
        }
        if "fields" in work:
            event["fields"] = work["fields"]
        events.append(event)
    if SPLUNK_AGGREGATE_TRAPS_EVENTS:
        events = aggregate_traps(events)
    events = [json.dumps(e, indent=None) for e in events]
    return events
```

This is the boundary between the SC4SNMP internal model and the Splunk wire format. Notice that all varbinds — counters, gauges, and textual fields — are flattened into a single JSON blob in the `event` field. There is no per-trap routing decision; every trap goes to the same `index` and `sourcetype`.

### 18.6 Example operator values file

`:: examples/polling_and_traps_v3.yaml:1-30` (combines polling + traps for the v3 case)

```yaml
splunk:
  enabled: true
  protocol: https
  host: i-0d903f60788be4c68.ec2.splunkit.io
  token: 00000000-0000-0000-0000-000000000000
  insecureSSL: "false"
  port: "8088"
traps:
  usernameSecrets:
    - sc4snmp-homesecure-sha-aes
    - sc4snmp-homesecure-sha-des
  securityEngineId:
    - "80003a8c04"
  loadBalancerIP: 10.202.4.202
```

### 18.7 Container entry-point routing

`:: entrypoint.sh:1-50` (verbatim, with the `case` arms relevant to the trap subsystem)

```sh
#!/usr/bin/env sh
set -e
. /app/.venv/bin/activate
. /app/construct-connection-strings.sh
LOG_LEVEL=${LOG_LEVEL:=INFO}
WORKER_CONCURRENCY=${WORKER_CONCURRENCY:=4}

wait-for-dep ${REDIS_DEPENDENCIES} "${MONGO_WAIT}" "${MIB_INDEX}"

ENABLE_TRAPS_SECRETS=${ENABLE_TRAPS_SECRETS:=false}
ENABLE_WORKER_POLLER_SECRETS=${ENABLE_WORKER_POLLER_SECRETS:=false}
wait-for-dep "${REDIS_DEPENDENCIES}" "${MONGO_URI}" "${MIB_INDEX}"
if [ "$ENABLE_TRAPS_SECRETS" = "true" ] || [ "$ENABLE_WORKER_POLLER_SECRETS" = "true" ]; then
    python /app/secrets/manage_secrets.py
fi
case $1 in
inventory)    inventory-loader ;;
celery)
    case $2 in
    beat)         celery -A splunk_connect_for_snmp.poller beat -l "$LOG_LEVEL" --max-interval=10 ;;
    worker-trap)  celery -A splunk_connect_for_snmp.poller worker -l "$LOG_LEVEL" -Q traps --autoscale=8,"$WORKER_CONCURRENCY" ;;
    worker-poller) celery -A splunk_connect_for_snmp.poller worker -l "$LOG_LEVEL"  -O fair -Q poll --autoscale=8,"$WORKER_CONCURRENCY" ;;
    worker-sender) celery -A splunk_connect_for_snmp.poller worker -l "$LOG_LEVEL" -Q send --autoscale=6,"$WORKER_CONCURRENCY" ;;
    flower)       celery -A splunk_connect_for_snmp.poller flower ;;
    *)            celery "$2" ;;
    esac ;;
trap)
    traps "$LOG_LEVEL" ;;
*) echo -n unknown cmd "$@" ;;
esac
```

The single container image is reused for every role; the Helm chart's per-Deployment `args:` field selects which arm of this `case` runs (`charts/.../worker/trap/deployment.yaml:47-50` for the worker-trap arm has `args: [celery, worker-trap]`; the trap-receiver Deployment has `args: [trap]` at `charts/.../traps/deployment.yaml:44-47`). Notes:

- The `trap` arm calls `traps "$LOG_LEVEL"` (`entrypoint.sh:44-46`). `traps` is the Poetry-installed console-script entry point declared at `pyproject.toml:10-11` (`traps = 'splunk_connect_for_snmp.traps:main'`).
- The `celery worker-trap` arm runs the Celery worker against the **`traps` Redis queue** with `-Q traps`. The `-A splunk_connect_for_snmp.poller` flag points Celery at the Celery `app` *instance* defined in `poller.py` (which itself imports the `celery_config` module to load the queue/broker config — `celery_config.py:1-98`). This is a Celery-app reference, **not** a queue name.
- Both `wait-for-dep` invocations (`entrypoint.sh:11, :15`) block startup until Redis, MongoDB, and the MIB-server HTTP index are reachable; `wait-for-dep` is a Splunk fork pulled via `pyproject.toml:42`.
- The optional `manage_secrets.py` step (`entrypoint.sh:17-19`) is only run when `ENABLE_TRAPS_SECRETS` or `ENABLE_WORKER_POLLER_SECRETS` is `true`, both default `false` (`entrypoint.sh:13-14`).

This dispatch lets all SC4SNMP pods share one image and one entrypoint shell.

---

## 19. Sources Examined

### Repository paths (relative to `splunk/splunk-connect-for-snmp @ fdd4c74e`)

Code:
- `splunk_connect_for_snmp/traps.py` — trap receiver (asyncio + pysnmp NotificationReceiver, dynamic engineID, Celery enqueue).
- `splunk_connect_for_snmp/celery_config.py` — Celery + Redis broker, queues, prefetch, retry, ack-late.
- `splunk_connect_for_snmp/snmp/tasks.py` — `trap` Celery task, MIB resolution, IPv4 normalization, reverse DNS.
- `splunk_connect_for_snmp/snmp/manager.py` — `Poller` base class (shared across walk/poll/trap), MIB compiler, varbind classification.
- `splunk_connect_for_snmp/snmp/auth.py` — v3 USM auth construction, engineID discovery for polling.
- `splunk_connect_for_snmp/snmp/const.py` — AuthProtocolMap, PrivProtocolMap.
- `splunk_connect_for_snmp/snmp/varbinds_resolver.py` — profile-to-varbind mapping (polling-side).
- `splunk_connect_for_snmp/splunk/tasks.py` — HEC sender, `prepare` task, `prepare_trap_data`, aggregate, mTLS, token file.
- `splunk_connect_for_snmp/common/collection_manager.py` — `EngineIdManager`, `ProfilesManager`, `GroupsManager`.
- `splunk_connect_for_snmp/common/schema_migration.py` — Mongo schema versioning.
- `splunk_connect_for_snmp/common/custom_translations.py` — varbind rename map.
- `splunk_connect_for_snmp/common/common.py` — `wait_for_mongodb_replicaset`, `disable_mongo_logging`, `human_bool`.
- `splunk_connect_for_snmp/enrich/tasks.py` — polling-only enrichment (referenced to confirm traps are not enriched).
- `splunk_connect_for_snmp/common/custom_cache.py` — TTL-LRU cache used by the trap worker for reverse-DNS lookups.
- `splunk_connect_for_snmp/common/collections_schemas.py` — JSON Schema for MongoDB document validation (includes `engine_id_records`).
- `entrypoint.sh` (repo top-level) and `construct-connection-strings.sh` — container entry-point routing (`case` on `$1` dispatching `trap` / `celery worker-trap` / `worker-poller` / `worker-sender` / `inventory` / `flower`) plus the helper that assembles Redis/MongoDB URIs from env vars.

Helm chart (under `charts/splunk-connect-for-snmp/`):
- `Chart.yaml`, `values.yaml`.
- `templates/traps/{deployment,service,hpa,deprecated_hpa,pdb,networkpolicy,_helpers.tpl}.yaml`.
- `templates/worker/trap/{deployment,hpa,deprecated_hpa}.yaml`.
- `templates/worker/_helpers.tpl` — defines `environmental-variables` and `environmental-variables-trap` template snippets (Helm-to-env wiring including `WORKER_CONCURRENCY`, `PREFETCH_COUNT`, `RESOLVE_TRAP_ADDRESS`, `MAX_DNS_CACHE_SIZE_TRAPS`, `TTL_DNS_CACHE_TRAPS`, `IPv6_ENABLED`).
- `templates/mongodb/`, `templates/redis/`, `templates/scheduler/`, `templates/inventory/`, `templates/sim/`, `templates/ui/`, `templates/serviceaccount.yaml`, `templates/NOTES.txt`, `templates/_helpers.tpl`.

Docker-Compose layout:
- `docker_compose/docker-compose.yaml`.
- `docker_compose/Corefile`, `docker_compose/manage_logs.py`.

Tests:
- `test/test_traps.py` — `decode_security_context` unit tests.
- `test/snmp/test_tasks.py` — `trap()` task unit tests + `format_ipv4_address` tests.
- `test/splunk/test_prepare.py` — `prepare_trap_data` tests.
- `test/splunk/test_send.py`, `test_value_as_best.py`, `test_read_hec_token.py` — HEC delivery tests.
- `integration_tests/tests/test_trap_integration.py` — full pipeline integration tests against a real Splunk.
- `integration_tests/snmpsim/data/variation/notification.snmprec` — snmpsim sending-side fixture (v1 trap, v2c trap, v3 inform).

CI / release:
- `.github/workflows/ci-main.yaml` — actual trigger semantics (push/PR/workflow_call; integration tests gated by `[run-int-tests]` or `develop`).
- `.github/workflows/cd-dashboard-release.yaml` — release-time upload of `dashboard/dashboard.xml`.

Dashboards (first-party):
- `dashboard/dashboard.xml` — in-repo Splunk Classic Dashboard XML; includes the *"SNMP trap status"* and *"SNMP trap authorisation"* panels.

Documentation:
- `docs/index.md`, `docs/ha.md`, `docs/security.md`, `docs/mib-request.md`, `docs/mtls.md`, `docs/dashboard.md`, `docs/small-environment.md`, `docs/releases.md`.
- `docs/architecture/{design,planning}.md`.
- `docs/configuration/{traps,snmpv3,snmp-data-format,profiles,poller-configuration,inventory,groups,splunk-configuration,workers,step-by-step-poll}.md`.
- `docs/troubleshooting/{traps-issues,polling-issues,docker-commands,k8s-commands,general-issues,configuring-logs}.md`.
- `docs/internal/{flower,pysnmp_debug}.md`.
- `docs/microk8s/` (Helm-installation guide; not exhaustively read).
- `docs/dockercompose/` (Docker-Compose installation guide; not exhaustively read).

Examples:
- `examples/traps_enabled_values.yaml`, `examples/polling_and_traps_v3.yaml`, `examples/polling_values.yaml`, `examples/polling_groups_values.yaml`, `examples/lightweight_installation.yaml`, `examples/splunk_token_from_secret.yaml`, `examples/splunk_token_from_vault.yaml`.

Misc:
- `Dockerfile`, `entrypoint.sh`, `construct-connection-strings.sh`, `pyproject.toml`, `poetry.lock`.
- `README.md`, `CHANGELOG.md`, `LICENSE`.
- `Makefile`, `mkdocs.yml`, `srs.yaml`, `sonar-project.properties`, `.fossa.yml`, `.releaserc`, `renovate.json`.

External (referenced but not in mirror):
- `https://splunk.github.io/splunk-connect-for-snmp/` (the rendered docs site).
- `https://pysnmp.github.io/mibs/index.csv` (MIB catalogue).
- `https://github.com/pysnmp/mibs` (mibserver source).
- `https://github.com/splunk/observability-content-contrib/tree/main/dashboards-and-dashboard-groups/SC4SNMP` (community dashboards).
- Splunk-side product docs for HEC, indexes, sourcetypes, props/transforms (not enumerated).

---

## 20. Evidence Confidence

| Section | Confidence | Notes |
|---|---|---|
| §1 — System Overview & Lineage | high | License, ownership, language, library list all source-verified in `pyproject.toml`, `Dockerfile`, `README.md`. |
| §2 — Trap-Subsystem Architecture | high | Helm templates, `docker-compose.yaml`, and `entrypoint.sh` directly exhibit the component graph. |
| §3 — Trap Reception | high | `traps.py:337-448` is the entire startup path; the `_EngineIDCaptureUdpTransport` subclass is unambiguous; v1/v2c/v3 paths are exercised by integration tests. INFORM handling is genuinely undocumented (`unverified` flagged). |
| §4 — MIB Management | high (process) / medium (vendor coverage) | Compilation & load pipeline source-verified in `manager.py:271-316, :487-507`. Vendor MIB coverage is an external dependency on `pysnmp/mibs`; the live catalogue at `pysnmp.github.io/mibs/index.csv` was not exhaustively enumerated here. |
| §5 — Trap Processing Pipeline | high | The Celery chain shape is one-line obvious (`traps.py:289-294`); each stage's task is fully readable in 50-100 lines. Edge cases (timeout window during HEC outage) are derived from explicit task decorators. |
| §6 — Data Model & Persistent Storage | high | `EngineIdManager` and the Mongo collection set are directly inspectable; Redis queue/key shapes are in `celery_config.py`. |
| §7 — Configuration UX | high | `values.yaml`, `traps/deployment.yaml`, and `docs/configuration/traps.md` together completely specify the operator surface. |
| §8 — Integration with Other Signals | high | `prepare_trap_data` (`splunk/tasks.py:293-315`) is the explicit boundary; the absence of metrics/topology/northbound paths is structural. |
| §9 — Severity Model | high (negative claim) | Direct evidence by absence: no severity field in `prepare_trap_data`, no severity mapping in `docs/configuration/`. |
| §10 — Storm / Volume Handling | high (mechanism) / medium (capacity claim) | All retry/backpressure mechanisms are source-verified. The 1500 traps/sec number is vendor-published in `docs/architecture/planning.md`; no in-repo benchmark to corroborate. |
| §11 — Security | high | Auth/Priv protocol map (`const.py`), Secret mounts (`traps/deployment.yaml:158-178`), security context (`traps/deployment.yaml:33-41`), security guidance (`docs/security.md`) all directly cited. |
| §12 — Trap Simulation & Testing | high | All test files inventoried and the unit-vs-integration split is explicit; the "asyncio receiver is not unit-tested" claim is by direct inspection of `test/test_traps.py:5-17`. |
| §13 — Out-of-the-Box Coverage | high | `DEFAULT_STANDARD_MIBS` is a code constant; dashboard repo location is documented in `docs/dashboard.md`. |
| §14 — User Customization Surface | high | Direct inspection of `custom_translations.py` and `values.yaml` UI section; absence of plugin hook is structural. |
| §15 — End-User Value Analysis | medium-high | Synthesis from §§1-14; learning-curve and operational-toil judgements are reasoned from the documented surfaces rather than a separate survey of operators. |
| §16 — Strengths | high | Each item file:line-anchored. |
| §17 — Weaknesses / Gaps | high | Each item file:line-anchored or supported by a documented absence. |
| §18 — Notable Code or Configuration Examples | high | Direct verbatim extracts. |
| §19 — Sources Examined | high | File list directly inspected during this analysis. |

Overall confidence: **high**. The trap subsystem of SC4SNMP is small enough to be read end-to-end (`traps.py:1-448`, `snmp/tasks.py:149-244`, `splunk/tasks.py:169-347`), and the Helm chart and Docker-Compose layout make the deployment surface concrete. The two areas of irreducible doc-only evidence are the vendor-published throughput numbers (§10) and the MIB-catalogue vendor coverage (§4); both are clearly labelled.

---

## 21. Reviewer History

### Iteration 1 — verdict per reviewer

| Reviewer | Verdict | Notes |
|---|---|---|
| codex (gpt-5.5) | accept-with-fixes | 5 majors, 2 minors, 1 nit — defaults, NetworkPolicy semantics, INFORM overstatement, CI trigger description, missing in-repo dashboard XML, Redis enqueue wording, fixture inventory, marketing language. All addressed in this document. |
| minimax (m2.7-coder) | accept-with-fixes | 1 blocker (CheckMK/OpenNMS comparison citation), 2 majors (metric-store conflation, INFORM table), 4 minors (HEC dedup, MongoDB line, 1500/sec context, integration-test fixture nature), 4 missed items (entrypoint.sh, CHANGELOG, CELERY_TASK_TIMEOUT, comparability framing). All addressed except CHANGELOG history excerpt (treated as out of scope for the per-system spec). |
| glm (glm-5.1) | accept-with-fixes | 8 minor / nit findings — DNS cache Helm override, HPA-disabled-by-default, vanilla install contradiction, resolveAddress Helm section, addV1System v1/v2c clarification, ConnectionError 30s retry, missing sources, OTel output qualification. All addressed. |
| qwen (qwen3.6-plus) | accept-with-fixes | 2 majors (TLS-verify default, OTel instrumentation presence), 4 minors (aggregate_traps key-collision, cb_fun exception path, retry_backoff_max mechanism, MongoDB line off-by-one), 4 nits (INFORM strengthening, missing test_manager.py, IGNORE_EMPTY_VARBINDS, entrypoint.sh). All addressed. |
| mimo (m2.5-pro) | infrastructure failure | Litellm fallback error after verification reads completed; review body not emitted. The reviewer did read all main source files (`traps.py`, `manager.py`, `tasks.py`, `values.yaml`, integration tests) before crashing, so partial cross-checking was completed but no findings were recorded. |
| kimi (k2.6) | infrastructure failure | "Timeout on reading data from socket" after verification reads completed. Same pattern as mimo — files read, no findings emitted. |

### Iteration 1 outcome

Four reviewers (codex, minimax, glm, qwen) returned `accept-with-fixes`. mimo and kimi failed mid-review with infrastructure errors (litellm fallback / socket timeout) after their verification source-reads had completed. Five distinct major-class findings were collected from codex (defaults, NetworkPolicy semantics, INFORM overstatement, CI trigger description, missing in-repo dashboard XML), plus the TLS-verify default and OTel-instrumentation gaps from qwen, plus one citation issue (CheckMK/OpenNMS `agent-addr` comparison without evidence) from minimax. All applied to the document. Iteration 2 was then run to verify convergence.

### Iteration 2 — verdict per reviewer

| Reviewer | Verdict | Notes |
|---|---|---|
| codex | accept-with-fixes | 4 majors (fabricated `entrypoint.sh` snippet in §18.7, "HPA scales by default" claim in §15, precise 64s retry-window math, HEC dedup-by-token sentence in §16 strength #10), 2 minors (latency claim, "only mirrored cohort" comparative overstatement), 2 housekeeping nits. All four majors addressed in this document. |
| qwen | **accept** | 8 minor / nit findings only. No blockers, no majors. *"Verdict: ACCEPT — the 8 findings above are all minor/nit level."* |
| minimax | accept-with-fixes | All findings at nit / minor severity; *"none are blockers; none contradict cited evidence."* |
| mimo | accept-with-fixes | All 7 findings nit-level (line-number nudges); *"no blockers, no major issues."* |
| glm | accept-with-fixes | 4 findings, all minor / nit (fabricated entrypoint quote, pysnmp-fork wording, missing `next` branch in CI, dangling §21 reference). Two overlap with codex. |
| kimi | accept-with-fixes | All findings at nit severity; *"fit for purpose after the §21 reference is resolved."* |

### Iteration 2 outcome — convergence

After fix-application, the picture stabilizes:
- **6 of 6** reviewers completed (mimo and kimi failed in iter-1; both completed cleanly in iter-2).
- **1 of 6** (qwen) returned outright `accept`.
- **5 of 6** returned `accept-with-fixes` whose findings, after this round, are uniformly minor / nit. Codex's 4 majors in iter-2 were all real factual issues and have been fixed in this revision (fabricated `entrypoint.sh` block replaced with the real source; §15 HPA-by-default claim corrected; §10/§18.3 retry math softened; §16 strength #10 HEC-dedup sentence removed).

Per the SOW stop rule (≥3 reviewers accept-with-fixes and only minor/nit findings remain), **iteration 2 is declared converged.**

Future regression risk: if SC4SNMP merges a release that changes the trap subsystem materially (new SNMP version, INFORM ACK semantics, metrics emission, schema migration), this spec must be re-run through the reviewer set against the new commit.

### Surviving minor / nit findings (from iter-2, not requiring further iteration)

- §13 "bare install" could be even more explicit that with `communities: {}` the trap path silently accepts nothing. The current §13 says this; an extra sentence in §7 ("What the operator sees by default") would help discoverability.
- §3 mentions `containerPort: 2163` for IPv6 in the trap Deployment template (`traps/deployment.yaml:97-100`) while the Python code binds `("::", 2162)` and the Service targets `2162`. This is harmless metadata under standard Helm rendering but is a small inconsistency worth a future cleanup PR upstream.
- §12 could enumerate `test/snmp/test_group_key.py`, `test/snmp/test_utils.py`, `test/snmp/test_process_snmp_data.py`, `test/snmp/test_do_work.py` as indirect trap coverage; they exercise shared `process_snmp_data` / `get_group_key` / `map_metric_type` code paths that the trap pipeline uses.
- §15 could reference `docs/small-environment.md` for resource-constrained deployments.
- §6 could mention `splunk_connect_for_snmp/common/collections_schemas.py` JSON Schema validation for `engine_id_records` documents.
- `wait-for-dep`, `manage_secrets.py`, and the worker `-O fair` flag (poller-only) are operational details that did not make it into the architectural narrative.

These are accepted as deferred polish items rather than blockers; the document remains source-faithful and template-complete.
