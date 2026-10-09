# Automatic Java attachment and instrumentation cost

The separate monitor discovered running fixture JVMs, delivered the monitoring jars into their container filesystems,
and attached without application-owner configuration. JVM, HTTP and Hikari pool metrics reached Netdata. Discovery
and attachment recovered after an application process restarted; restarting the monitor did not duplicate attachment.

This establishes a useful part of the desired workflow on the tested Linux containers. Docker supplied a shared PID
namespace, and the discovery rule deliberately recognizes only this fixture. Native-host discovery into ordinary,
independent container PID namespaces and safe production injection remain unproven.

## What was tested

The unchanged Spring Boot 3.5.7 / Tomcat 10.1.48 / HikariCP 6.3.3 application runs on the pinned Temurin 21 JRE image.
Its image contains the JRE and application jar only. A separate monitor image owns the JDK, unmodified OTel 2.32.0 jar
and existing Netdata experiment extension. Four application containers have separate filesystems and networks but
share the first container's task-owned PID namespace. Neither a Docker socket nor the host PID namespace is exposed.

The scanner finds the fixture's Java command through `/proc`; no PID list is passed to it. It records PID/start time,
reads the target UID/GID, and starts a bounded helper under those credentials. The helper creates a fresh target `/tmp`
directory, copies the jars through `/proc/<pid>/root`, then uses the bundled JDK's Attach API. The scanner uses root
plus `CAP_SYS_PTRACE`; the attachment child drops to the application user. No installed Netdata helper was changed.

| Case | Observed result |
|---|---|
| Already-running app, UID 10001 | Automatic delivery/attach; exact 5,400 HTTP 200 + 600 HTTP 503 requests; JVM/HTTP/pool samples stored |
| Already-running app, UID 10002 | Same successful coverage under a different Unix user |
| `-XX:-EnableDynamicAgentLoading` | Explicit `AgentLoadException`; application still serves requests; no agent telemetry |
| Read-only target `/tmp` | Explicit file-delivery failure; application still serves requests using a separate writable application temp directory |
| Monitor process/container restart | Four existing attempts remain four; no duplicate attempt |
| Application child restart | New PID/start identity discovered and attached; all six required stored contexts have fresh samples for the new identity |

There are three successful attachments across two UIDs, two expected failures and five distinct attempted process
identities. The monitor's final Docker memory sample is 72.2 MiB; this is a single idle sample after helpers exited,
not a peak or fleet-scale measurement. Copied artifacts live in the disposable targets until cleanup.

The tested helper is Temurin **21.0.12.1+1**, source `adoptium/jdk21u @ 1c417fbfc2f7`. Its
[Linux Attach implementation](https://github.com/adoptium/jdk21u/blob/1c417fbfc2f7/src/jdk.attach/linux/classes/sun/tools/attach/VirtualMachineImpl.java)
tries the target's writable `/proc/<pid>/root/tmp`, including when the two processes share a PID namespace but have
different filesystems. Older JDK helper behavior differs; this result does not establish that every JDK 21 build works
the same way. The monitor can bundle the tested helper rather than depend on a target application shipping JDK tools.

## Narrower configuration and cost

The same unmodified upstream agent supports
[selective instrumentation](https://opentelemetry.io/docs/zero-code/java/agent/disable/). This test disables default
instrumentation and enables JVM runtime telemetry, Servlet, Tomcat, Spring MVC and the Hikari extension. Dependencies
and broader application coverage need verification per profile. The application owner supplies none of these options.

The narrowed configuration retained JVM memory, HTTP status counts, `/work` route labels, duration histogram counts
and buckets, and pool active/idle/pending/limit gauges in both raw OTLP and actual Netdata samples. The scope is this
Spring/Tomcat/Hikari stack; this configuration does not promise outgoing calls, async frameworks or other app metrics.

Nine runs used three rotated blocks: baseline/full/narrow, narrow/baseline/full, full/narrow/baseline. Each fresh JDK 21
application had a two-CPU quota, 128–256 MiB heap, 10 seconds of pre-attach traffic, 20 seconds of post-attach warmup and
30 seconds of measurement. A separate client inside the target network namespace sent 300 requests/second with a
zero hold time. Baseline had neither agent nor JMX probe. Both agent modes included the Hikari extension and disabled
the stock Hikari instrumentation; only the selected instrumentation modules differed.

Values are medians of three runs, with observed ranges in parentheses:

| Measurement | No agent | Full instrumentation + extension | Narrow instrumentation + extension |
|---|---:|---:|---:|
| End-of-run application RSS, MiB | 322.5 (318.4–322.8) | 448.3 (444.3–449.9) | 423.4 (421.3–424.2) |
| Application CPU, ms/request | 0.420 (0.380–0.481) | 0.522 (0.444–0.524) | 0.439 (0.431–0.492) |
| Client-observed p95 response time, ms | 1.872 (1.478–2.013) | 1.595 (1.427–1.673) | 1.711 (1.417–1.959) |
| Attachment wall time, seconds | — | 1.871 (1.706–1.909) | 1.022 (0.993–1.138) |

All 81,000 measured requests completed with the expected 90% HTTP 200 / 10% synthetic HTTP 503 split and no transport
failures. All six instrumented cases also match the exact cumulative post-attach request counts (15,000 each, including
warmup), preserve route/status/histogram semantics, and have stored JVM/HTTP/pool samples. Client p95 dispatch lag was
0.803–0.851 ms across the nine runs.

The narrowed profile saved approximately **25 MiB per JVM** relative to full instrumentation and shortened attachment.
It still added approximately **101 MiB** over baseline, using differences of median RSS. The CPU medians are encouraging
but ranges overlap; the latency variation does not establish a benefit or regression. Three short runs on Docker
Desktop cannot establish steady-state production overhead. CPU excludes the separate monitor, exporter/receiver and
load client; its interval includes client launch/drain around the measured requests. RSS is an end sample, not a peak,
and no forced GC or long soak was performed. The freshly started client adds warmup variability to every phase.

## Interpretation and remaining boundary

The evidence supports continuing with a Netdata-managed discovery/attachment layer around the upstream Java agent,
with a small extension where existing objects need coverage. It does not yet select or validate a production helper.

- Application setup was eliminated in the successful lab cases. Netdata still owns artifact packaging, process
  discovery, permissions, transport configuration and the collection lifecycle.
- The monitor's lab journal prevents blind repeat attempts across its own restart. It does not reconcile uncertain
  outcomes, replacement monitors, multiple owners, existing agent versions, upgrades or reboot/PID-namespace changes.
- The fixture allowlist and UID-based service names are deliberate test boundaries. Production identity needs container,
  process and service metadata, including multiple JVMs for the same user and container churn.
- A production privileged boundary must validate process identity across races, handle untrusted target paths and
  namespaces, authenticate telemetry ownership and constrain payloads/operations. This lab helper is not suitable for
  installation as setuid or as a general root command.
- Disabled JVM attachment/loading and unavailable target storage require explicit user-visible states. This experiment
  does not bypass application policy or implement an alternative path for locked-down workloads.
- Native Linux hosts, cross-PID-namespace discovery, Kubernetes, rootless/user namespaces, read-only/distroless images
  with different temp layouts, SELinux/AppArmor, other JVM vendors, older helper builds and broad framework coverage
  are not established. Earlier JDK 17/21/25 coverage belongs to the earlier experiments, not this new configuration.

The next useful validation is the same workflow from a host-installed monitor into ordinary Linux container PID
namespaces, followed by a concrete design for the smallest privileged helper and its lifecycle. The cost evidence must
also be checked on longer representative applications before choosing a production resource budget.

## Evidence

- `automation_checks.py` and `Scout.java`: real discovery, delivery, failure and restart experiment.
- `HttpLoad.java`: internal paced workload used by the cost comparison.
- `summarize_automation.py`: replay of exact coverage, fresh per-context restart samples and complete balanced cases.
- `automation-evidence.json`: sanitized checked results; raw local evidence under
  `.local/java-monitoring-spike/automatic-first` and `.local/java-monitoring-spike/cost-balanced`.
- The original [comparison](RESULTS.md) and [extension study](EXTENSION_RESULTS.md) remain separately documented.
