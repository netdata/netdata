# Java monitoring spike: first comparison

Measured on 2026-10-09. **The managed-agent direction is viable, but stock OTel late attachment does not meet the
first complete-coverage milestone.** JVM and HTTP monitoring work; an already-initialized Hikari pool remains invisible.
Startup attachment supplies all three. This is evidence for the next design decision, not a production-readiness claim.

The subsequent [extension experiment](EXTENSION_RESULTS.md) recovers the missing gauges for pools used after attachment.
This report preserves the original unmodified-agent comparison.

## What actually worked

The same Spring Boot 3.5.7 application ran with Tomcat 10.1.48, HikariCP 6.3.3 and H2 2.3.232. It had no Actuator,
exporter, monitoring code or JMX configuration. Each late-attachment target served traffic before attachment.
The agent was OTel 2.32.0; Netdata was `v2.11.0-305-nightly`. Images and agent bytes are pinned in the harness.

| Path | JVM memory, GC, threads, CPU | HTTP counts, errors, duration | Existing pool usage and waiters |
|---|---|---|---|
| Local JMX, no app setup | Available through JVM MBeans | Absent in this app | Absent in this app |
| OTel attached after warmup | Recorded and stored in Netdata | Recorded and stored in Netdata | Absent |
| Same OTel agent at startup | Recorded and stored in Netdata | Recorded and stored in Netdata | Recorded and stored in Netdata |

The result held on Temurin 17.0.20.1, 21.0.12.1 and 25.0.4.1, on Linux arm64 in Docker Desktop. OTel supplied heap
and non-heap metrics, GC duration histograms, thread/class counts and JVM CPU metrics. HTTP histograms had `/work`,
GET, 200 and 503 attributes. For all three late-attachment runs, cumulative HTTP counts exactly matched generated
200/503 responses. Netdata had non-null stored samples, not just chart metadata.

Startup attachment supplied active/idle connections, pending requests, limits, connection creation/use/wait durations.
Pending requests reached 9, 6 and 6 respectively during the three startup workloads, so pool data was not merely a set
of zero-valued instruments. The corresponding late runs produced no `db.client.connections.*` metrics.

JMX returned only the JVM/management domains in this fixture. This does not mean all Java applications lack useful
MBeans: it means JMX alone cannot promise application coverage for an ordinary application that does not register them.
The JMX probe's measurements were recorded directly; the lab did not implement a JMX-to-Netdata transport.

## Why late attachment misses the pool

The pinned OTel Hikari instrumentation wraps `HikariPool.setMetricsTrackerFactory`, normally invoked when constructing
the pool. Retransforming a loaded class does not repeat construction of an existing object. The warmed fixture does
not call that setter again. The startup control demonstrates that this agent supports the exact library version.
See the [pinned registration hook](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/1b8644f8eb11d90295b1c313391aa84edeeec6e2/instrumentation/hikaricp-3.0/javaagent/src/main/java/io/opentelemetry/javaagent/instrumentation/hikaricp/v3_0/HikariPoolInstrumentation.java).

There is a concrete next experiment: observe an existing `HikariDataSource` on a subsequent `getConnection` call and
register pool observation once, with proper close/unregister behavior. Hikari's public
[`getHikariPoolMXBean()`](https://github.com/brettwooldridge/HikariCP/blob/ea81bfb5852216dbfcb1f219742f91b5abceb81b/src/main/java/com/zaxxer/hikari/HikariDataSource.java#L299)
returns its pool without requiring MBean-server registration. That makes a targeted OTel extension plausible.
It has not been implemented or validated; metrics-tracker coexistence, object lifetime, idle pools and other Hikari
versions remain questions. This would test extensibility within the candidate architecture.

## Permissions, containers and policy

- Same-UID local attachment worked without a configured remote JMX endpoint or an application restart.
- A separate JDK helper attached to a JRE-only target while sharing the target's PID and network namespaces and using
  its UID. Filesystems remained separate. A different-UID helper failed. The target did not need JDK client tools.
- The agent JAR was pre-staged in the target image. Artifact delivery into arbitrary containers, namespace discovery,
  user-namespace mapping and a production privileged helper remain unimplemented.
- `-XX:-EnableDynamicAgentLoading` blocked OTel loading while local JMX still worked.
- `-XX:+DisableAttachMechanism` blocked both routes. Both applications continued serving requests after failed probes.
- A Datadog 1.67.1 tracing-agent smoke test allowed later OTel attachment and delivered JVM/HTTP metrics. Datadog's
  optional services were disabled and its trace destination was an unused loopback port. This proves a narrow
  coexistence case, not trace correctness or general compatibility with other monitoring agents.
- Two concurrently monitored JVMs, both PID 1 in separate containers, had separate container/service-instance identities
  and Netdata charts. Service names were supplied by the harness, so automatic service naming is not yet proven.
- Reattaching the same OTel agent returned successfully without creating a second service-instance identity. With the
  corrected lab topology, Netdata stored fresh samples about 15 seconds after both a Netdata restart and a startup-agent
  application restart. The latter created a new service-instance identity; old charts were still present during the
  observation window. Chart retirement and automatic reattachment to a restarted, late-attached application are not proven.

These observations do not establish native-host installation, Kubernetes injection, OpenJ9, GraalVM native images,
custom jlink runtimes, hardened filesystems or production UID/capability policy. Root privileges do not restore JVM
modules that are absent or remove the need to respect the target's agent-loading policy.

## Exploratory resource measurements

| JDK | JMX baseline RSS | Late-agent RSS | Startup-agent RSS | Late attach time |
|---|---:|---:|---:|---:|
| 17 | 393 MiB | 491 MiB | 486 MiB | 1.79 s |
| 21 | 403 MiB | 502 MiB | 513 MiB | 1.58 s |
| 25 | 404 MiB | 493 MiB | 512 MiB | 1.56 s |

The tested distribution added roughly 90–110 MiB RSS per JVM in this fixture. Across these short runs throughput was
1,800–1,844 requests/s and p95 latency was 10.95–11.26 ms. These are observations, not overhead guarantees: the workload
is bounded by a small connection pool, the baseline includes local JMX management, and ordering/JIT effects were not
controlled. Process CPU snapshots and monitor-container statistics are retained with the local evidence. A controlled
CPU/latency study and a narrower instrumentation configuration are still needed before a production cost claim.

## Architecture implication and next decision

Local JMX remains useful for exposed MBeans and runtime fallback. It cannot be the entire application-monitoring
foundation if the user should not configure their application. A managed in-process agent offers broader coverage;
the experiment supports continuing that investigation, not selecting the full OTel distribution unchanged.

A candidate division of responsibility is:

1. Netdata discovers JVMs, identifies the application/container and owns bounded helper execution, artifact delivery,
   attachment and capability reporting. C/Go/Rust remain appropriate for this orchestration.
2. Netdata-managed Java code observes runtime MBeans and instruments supported libraries inside the target JVM.
   Library support can grow through tested instrumentation/extensions without replacing the collection architecture.
3. Existing OTLP ingestion receives the data. A product implementation would need curated chart identities, bounded
   labels, stable service identity and alerts. The generic charts in this lab are transport proof, not the final UX.

**Recommended next experiment:** close the already-created-pool gap with a small OTel extension, then repeat this exact
late-attachment test and measure its cost. This preserves the no-application-setup target while testing how much custom
instrumentation Netdata would own. The alternative is to prioritize Netdata-managed startup injection: this fixture
shows fuller coverage, but deployment integration and an application restart become requirements. Neither route is
selected as a production architecture by this report.

## Evidence and limits

`measured-evidence.json` preserves sanitized comparison results. `summarize.py` checks JVM payloads, HTTP counts and
histogram consistency, pool positive controls and actual stored Netdata samples against the original run artifacts.
The reproduction and per-file evidence map are in [README.md](README.md).

Initial recorder-file and shared-network-namespace problems were harness defects. The first restart attempt replaced
Netdata's network namespace while the recorder retained the old one; connection-refused errors invalidated that
attempt as a product lifecycle test. The harness now gives the recorder and Netdata independent network namespaces.
The pinned Netdata image also predates the current source tree's receiver configuration schema; the harness uses its
verified `NETDATA_OTEL_CFG_ENDPOINT_PATH` option. Fresh-sample restart verification is recorded separately from the
coverage matrix and passed after both harness corrections.
