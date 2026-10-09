# Java monitoring feasibility lab

This experiment compares local JMX with late and startup attachment of the same OpenTelemetry Java agent.
It tests whether an ordinary, already-running Java application can acquire useful Netdata monitoring without
application-owner configuration. It is an experimental harness, not a shipped collector or an installation procedure.

## Run

Requires Python 3.10+ and a running Linux Docker engine. The recorded experiment used Linux arm64 in Docker Desktop;
native Linux hosts, Kubernetes, Windows and OpenJ9 are not covered by that evidence.

From the repository root:

```sh
python3 tests/java-monitoring-spike/run.py --self-test
python3 tests/java-monitoring-spike/run.py --build \
  --images netdata-java-spike:17 netdata-java-spike:21 netdata-java-spike:25 \
  --boundaries --coexistence
```

The build downloads pinned public images and Maven dependencies. The default run compares JDK 21 only; `--build`
builds all four target images, including the JRE-only target used by `--boundaries`. Subsequent runs can omit it.
Use a new `--output` directory to select an evidence location; existing directories are refused.

The runner creates a separate Docker network and containers with a unique ownership label. It only publishes
ephemeral loopback ports. It does not use host PID/network namespaces, mount the Docker socket, or attach to existing
applications. The only host mount is its new synthetic telemetry recording directory. A `finally` block collects
logs and removes the exact owned containers and network; downloaded images and evidence remain for reuse.
`ownership.json` records resource IDs if an external interruption prevents cleanup. Never use a global Docker prune
to clean up this experiment.

## Fixture and comparison

- Spring Boot 3.5.7, embedded Tomcat, HikariCP and an in-memory H2 database. No Actuator, Micrometer, exporter, JMX
  configuration or monitoring code is present in the application.
- `/work` borrows a connection, runs `SELECT 42`, holds it for 5 ms and returns either HTTP 200 or synthetic HTTP 503.
  Sixteen clients generate traffic, with approximately one request in ten returning 503.
- Readiness requests and ten seconds of warmup establish that the pool already exists and has served requests before
  the local JMX probe or late agent attachment runs. Startup attachment is a separate positive control.
- All applications run as UID 10001 with a 128–256 MiB Java heap. The normal probe runs as that same UID.
- The JMX probe starts the JVM's local management agent, inventories all registered MBeans and reads JVM attributes.
  It does not enable Spring/Hikari MBeans or set up a remote JMX port. JMX data is recorded, not forwarded to Netdata.
- The OTel probe calls `VirtualMachine.loadAgent` with monitoring configuration supplied at attachment time. A
  successful return is recorded separately from observed metrics. Startup and late attachment use identical options.
- OTel exports metrics every second; traces and logs exporters are disabled. This does not disable all instrumentation
  that can also create spans. The experiment does not claim an optimized metrics-only agent configuration.
- A disposable OTel Collector records raw OTLP JSON and forwards metrics to Netdata over gRPC on the dedicated Docker
  network. Its receiver and Netdata's OTLP receiver are not published to the host. Independent network namespaces allow
  Netdata to restart without replacing the recorder's network stack. The recorder is lab apparatus, not a proposed
  product dependency.

## Evidence and validation

The default output is under `.local/java-monitoring-spike/<timestamp>/`:

| File | Evidence |
|---|---|
| `results.json` | JMX inventory, attachment status/time, Java version, workload measurements and resource snapshots |
| `*-metrics.json` | Last observed metric payload per name and all observed resource identities for that service |
| `recording/metrics.json` | Complete OTLP recording across all services, including earlier samples |
| `*-charts.json` | Netdata chart metadata selected by the synthetic service name |
| `*-data.json` | Actual Netdata time-series samples for selected JVM, HTTP and pool charts |
| `boundaries.json` | Disabled-attach, separate-helper/JRE, duplicate-agent and restart observations |
| `coexistence.json` | Late OTel attachment with a pre-existing Datadog agent and a concurrent second JVM |
| `images.json` | Netdata/recorder/default fixture image identities |
| `*.log` | Logs from task-owned containers |

The startup positive control must provide JVM memory, HTTP duration, pool usage and pending requests, plus stored
Netdata samples. Missing raw telemetry when Netdata has charts fails the harness. Late attachment is not asserted to
provide every metric: its gaps are the question under investigation. Raw file reads exclude an unfinished final line
while the recorder is writing. No absent metric is replaced with zero.

The 20-second workload measurements are exploratory. The baseline measurement already has the built-in JMX
management agent running. There is no continuous external JMX polling; the one-shot probe's CPU and duration are
included around attachment, not a steady-state collector cost. Requests originate from the workstation, cross
Docker's loopback forwarding, and share resources with monitoring. Ordering, warmup and JIT effects are not controlled
well enough to claim a production overhead percentage or latency bound.

Boundary checks use a separate JDK helper with the JRE target's PID and network namespaces but a separate filesystem.
This tests that the target need not contain JDK client tools. The agent JAR is already staged at `/lab/otel.jar` in
the target image: secure delivery into an arbitrary existing container is not implemented. Application restarts use
the startup-agent positive control; discovery and automatic reattachment after a late-attached JVM restarts are not
implemented. Reattaching the same agent is not evidence of compatibility with third-party APM agents. `--coexistence`
downloads Datadog Java agent 1.67.1, verifies its release SHA256, starts the fixture with it, then attaches OTel after
warmup. Datadog tracing stays enabled but its exporter targets an unused loopback port; remote configuration, telemetry,
profiling, AppSec, data streams and JMXFetch are disabled. This bounded smoke check cannot establish full Datadog
compatibility or trace correctness. A second instrumented JVM runs concurrently to check metric identity separation.

## Pinned sources

- OTel Java agent 2.32.0, SHA256
  `f787eb6c7f3d18e69a431e108a15278d25ee37f83d68b678f621e063f3988f82`.
- `open-telemetry/opentelemetry-java-instrumentation @ 1b8644f8eb11d90295b1c313391aa84edeeec6e2`:
  [agent argument handling](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/1b8644f8eb11d90295b1c313391aa84edeeec6e2/javaagent-bootstrap/src/main/java/io/opentelemetry/javaagent/bootstrap/AgentInitializer.java),
  [Hikari metric registration hook](https://github.com/open-telemetry/opentelemetry-java-instrumentation/blob/1b8644f8eb11d90295b1c313391aa84edeeec6e2/instrumentation/hikaricp-3.0/javaagent/src/main/java/io/opentelemetry/javaagent/instrumentation/hikaricp/v3_0/HikariPoolInstrumentation.java).
- Runtime, build, recorder and Netdata image digests are pinned in `Dockerfile` and `run.py`.
- [Oracle Attach API](https://docs.oracle.com/en/java/javase/25/docs/api/jdk.attach/com/sun/tools/attach/VirtualMachine.html),
  [JEP 451](https://openjdk.org/jeps/451),
  [Spring JMX defaults](https://docs.spring.io/spring-boot/reference/actuator/jmx.html),
  [Hikari MBean setup](https://github.com/brettwooldridge/HikariCP/wiki/MBean-(JMX)-Monitoring-and-Management).

See [RESULTS.md](RESULTS.md) for the measured outcome, limits and next design decision. To verify a saved comparison:

```sh
python3 tests/java-monitoring-spike/summarize.py .local/java-monitoring-spike/<run-directory>
```

The verifier checks exact generated-versus-observed HTTP response counts in late-attachment runs, histogram bucket
counts, startup pool controls and non-null Netdata samples. The runner's lifecycle checks additionally require stored
sample timestamps later than each restart; retained chart metadata alone cannot pass them.
