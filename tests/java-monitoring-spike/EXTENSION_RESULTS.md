# Java monitoring spike: existing-pool extension

Measured on 2026-10-09. **A separate OTel extension recovered active, idle and pending-connection metrics from the
already-running Spring application on JDK 17, 21 and 25.** The application and upstream agent were unchanged. This
closes the first experiment's Hikari gauge gap for pools that receive a connection request after attachment.

## What changed

Two Java source files implement an OTel instrumentation module and a pool observer. The module observes
`HikariDataSource.getConnection()` and `close()`. It reads public Hikari accessors, uses weak target references, bounds
live registrations, and releases metric callbacks on close. An extension JAR is passed to the unmodified OTel agent
using its supported extension mechanism. No Spring/Actuator/JMX configuration was added to the application.

For these runs, upstream Hikari instrumentation is disabled so that the extension owns pool gauges. This experiment
does not implement pool acquisition/use/creation duration metrics. Other upstream instrumentation supplies JVM and
HTTP metrics. Experimental metric names begin with `netdata.spike.hikari`; they are not a proposed public contract.

## Coverage and cost

All nine baseline/stock-late/extension-late cases used the same ordinary Spring Boot 3.5.7 application, HikariCP 6.3.3,
Tomcat 10.1.48 and H2 2.3.232 on Linux arm64 in Docker Desktop. OTel was 2.32.0, with SDK 1.66.0. Ten seconds of traffic
preceded attachment. Both late modes' recorded HTTP 200/503 counts exactly matched the generated twenty-second
workload. Histogram counts were consistent, and Netdata stored actual JVM and HTTP samples.

Stock late attachment again produced no pool metrics. Every extension run produced one active, one idle, one pending
and one limit series for the existing pool, with actual stored Netdata samples. It required no application restart.

| JDK | Baseline RSS | Stock late RSS | Extension late RSS | Extension attach time |
|---|---:|---:|---:|---:|
| 17 | 389 MiB | 518 MiB | 525 MiB | 1.73 s |
| 21 | 409 MiB | 512 MiB | 518 MiB | 1.52 s |
| 25 | 403 MiB | 519 MiB | 524 MiB | 1.64 s |

Across these runs, throughput was 1,795–1,853 requests/s and p95 latency was 10.83–11.08 ms. Extension-run RSS was
4–7 MiB above the corresponding stock run, but these are single-run observations, not an isolated extension-overhead
estimate. Upstream Hikari instrumentation also differs between configurations. The full agent still accounts for a
substantial footprint: extension runs were approximately 109–136 MiB above the JMX baseline in this rerun. Ordering,
JIT effects, the pool-limited workload and host forwarding prevent a production CPU/latency claim. The registry adds
a synchronized lookup on connection borrowing; its cost at higher borrow rates has not been established.

## Lifecycle and existing tracker

A separate controlled Hikari fixture passed nine phases on each JDK, for 27 verified phases. Both pools existed and
had served connections before attachment. The fixture supplies an independent source oracle and a pre-existing
counting metrics tracker; none of these test controls were added to the ordinary Spring application.

- Before the next borrow, neither existing pool was discovered. Sixteen concurrent borrows registered one pool once;
  the second pool remained invisible until it too received post-attachment traffic.
- With two connections held and one caller blocked, raw telemetry and Netdata both reported active=2, idle=0,
  pending=1 and limit=2. After release, raw telemetry returned to active=0, idle=2 and pending=0.
- Closing one pool stopped its observations while the other continued. Closing both left no pool points in fresh
  export windows while JVM telemetry continued; missing pools were not represented by fabricated zeroes.
- Recreating a pool with the same name and a different limit produced a new pool identity and correct values. Lazy
  initialization and five further close/recreate cycles produced no duplicate series in the observed windows.
- The original metrics-tracker factory remained installed. Each run ended with exactly 43 acquisitions, 43 usage
  callbacks and eight tracker closes for eight created trackers. Hikari registered no MBeans.

The first lifecycle attempt exposed a harness assumption: Netdata's pinned image stores these charts at ten-second
intervals, although OTel exports every second. A five-second saturated state did not guarantee a stored sample. The
final test holds that state for 25 seconds and checks actual values within its time window; all three runs pass.
Early HTTP 404 data-query responses are retained as errors rather than parsed as JSON or treated as zero-valued data.

## Limits and implication

- A pool that remains unused after attachment is invisible. The experiment discovers it on the next borrow, rather
  than enumerating existing heap objects. It cannot reconstruct activity before attachment.
- The extension is compiled for Java 17+. Evidence covers HikariCP 6.3.3 and one Hikari class loader per JVM. The SPI
  is version-pinned; this prototype has no
  generated compatibility checks for other Hikari versions. Multiple independent class loaders need additional identity
  and lifetime design. Cap exhaustion and garbage-collection cleanup have not been tested.
- The agent and extension were pre-staged in the target image. Automatic JVM discovery, artifact delivery, attach
  permissions, service naming, reattachment and native-host/Kubernetes deployment remain separate work.
- JVM policy restrictions from the first experiment still apply. This extension does not bypass disabled attachment.
- Stopping source observations does not prove Netdata chart retirement. Production chart identity, retirement, labels
  and alerts remain undesigned.

The result supports a Netdata-managed upstream agent plus focused extensions as a candidate worth pursuing. It shows
that the first coverage gap can be addressed without a fork or a different collection architecture. It does not yet
establish a comprehensive zero-setup product. The next useful spike would test Netdata-owned discovery and attachment
on a native Linux host and ordinary containers, alongside a narrower agent configuration and controlled cost study.

## Reproduce and inspect

See [README.md](README.md#existing-pool-extension-experiment) for commands and verifier behavior. The original comparison
remains in [RESULTS.md](RESULTS.md). Local raw evidence lives in `.local/java-monitoring-spike/extension-matrix` and
`.local/java-monitoring-spike/pool-verified`; [extension-evidence.json](extension-evidence.json) preserves sanitized
measurements. Earlier `extension-first`, `pool-first` and `pool-matrix` runs are exploratory or diagnostic evidence;
the two named final runs establish the reported outcome.

Source contracts were checked against the pinned
[OTel extension example](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/1b8644f8eb11d90295b1c313391aa84edeeec6e2/examples/extension/build.gradle.kts),
[instrumentation SPI](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/1b8644f8eb11d90295b1c313391aa84edeeec6e2/javaagent-extension-api/src/main/java/io/opentelemetry/javaagent/extension/instrumentation/InstrumentationModule.java),
[Hikari public accessors and close behavior](https://github.com/brettwooldridge/HikariCP/blob/ea81bfb5852216dbfcb1f219742f91b5abceb81b/src/main/java/com/zaxxer/hikari/HikariDataSource.java),
and [SDK callback collection/removal](https://github.com/open-telemetry/opentelemetry-java/blob/v1.66.0/sdk/metrics/src/main/java/io/opentelemetry/sdk/metrics/SdkMeter.java).
