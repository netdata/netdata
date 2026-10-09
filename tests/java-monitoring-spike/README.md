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
- The original comparison applications run as UID 10001 with a 128–256 MiB Java heap. The normal probe uses that UID.
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
implemented by `run.py`; the separate automatic-attachment experiment below tests a bounded implementation.
Reattaching the same agent is not evidence of compatibility with third-party APM agents. `--coexistence`
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
After application restart, the samples must also belong to a new service-instance identity. Late-arriving samples
from the previous process cannot establish recovery.

## Existing-pool extension experiment

See [EXTENSION_RESULTS.md](EXTENSION_RESULTS.md) for measured results and limitations.

The separate `extension/` JAR uses the upstream OTel 2.32.0 instrumentation SPI; the upstream agent and ordinary Spring
application are unchanged. It observes `HikariDataSource.getConnection()` and reads public pool accessors. Entry advice
discovers existing pools before a borrow can block; exit advice handles pools initialized by that call. Close advice
removes observation. It never replaces a metrics tracker or enables JMX registration.

```sh
python3 tests/java-monitoring-spike/run.py --build \
  --images netdata-java-spike:17 netdata-java-spike:21 netdata-java-spike:25 \
  --modes baseline late extension --output .local/java-monitoring-spike/extension-comparison
python3 tests/java-monitoring-spike/summarize.py .local/java-monitoring-spike/extension-comparison
python3 tests/java-monitoring-spike/pool_checks.py \
  --images netdata-java-spike:17 netdata-java-spike:21 netdata-java-spike:25 \
  --output .local/java-monitoring-spike/pool-lifecycle
python3 tests/java-monitoring-spike/pool_checks.py --verify .local/java-monitoring-spike/pool-lifecycle
```

`extension` mode supplies `otel.javaagent.extensions` and disables upstream Hikari instrumentation, making the extension
the sole pool-metric producer. Other upstream instrumentation stays enabled. Its experimental gauges are
`netdata.spike.hikari.connections` (active/idle), `netdata.spike.hikari.pending_requests` and
`netdata.spike.hikari.limit`. This deliberately bounded test does not implement acquisition/use/creation durations.
Names and attributes are experimental, not a proposed public Netdata metric contract.

The separate `lifecycle/` fixture provides control endpoints and an independent source oracle. It creates and uses two
pools before attachment, with a counting metrics tracker already installed. The verifier checks idle discovery limits,
sixteen concurrent discovery calls, exact held-connection/waiter values, release, close, lazy recreation and repeated
recreation. It verifies tracker counts, absent JMX registrations, fresh OTLP observations and actual Netdata samples.
These control endpoints are lab apparatus; the ordinary Spring coverage case needs none of them.

`pool-checks.json` records operations' stable observation windows and source readings; `pool-verification.json` records
the verified result. Replaying the verifier requires the raw recording and saved chart/data captures. HTTP data-query
failures are retained as errors, never converted into samples. `summarize.py` also verifies exact HTTP counts and stored
pool data for extension runs.

The prototype bounds live registrations to 128 per instrumented Hikari class loader and uses weak source references.
Discovery requires a connection request after attachment; a pool that remains unused is invisible. Concurrent
independent Hikari class loaders, cap exhaustion, garbage-collection cleanup and broad library-version compatibility
are not established by this experiment. Close/removal means that source observations cease; Netdata chart retirement
is a separate lifecycle question. The synchronized registry adds work to connection borrowing; the short HTTP runs
cannot establish its production cost.

## Automatic attachment and narrower instrumentation

After building `netdata-java-spike:21` with the original Dockerfile, run:

```sh
python3 tests/java-monitoring-spike/automation_checks.py --build --automatic \
  --output .local/java-monitoring-spike/automatic
python3 tests/java-monitoring-spike/automation_checks.py --cost-rounds 3 \
  --output .local/java-monitoring-spike/cost
python3 tests/java-monitoring-spike/summarize_automation.py \
  --automatic .local/java-monitoring-spike/automatic --cost .local/java-monitoring-spike/cost
python3 tests/java-monitoring-spike/summarize_automation.py --self-test
```

`Dockerfile.automation` builds a plain JRE image containing only the unchanged application, plus a separate monitor
image with the JDK, pinned agent, extension and experimental discovery helper. Four synthetic applications share one
**task-owned PID namespace** while retaining separate filesystems and network namespaces. The monitor also joins that
PID namespace; Docker orchestration supplies this visibility. It never uses the host PID namespace or Docker socket.

`Scout` scans `/proc` for the exact fixture command `-jar /app/app.jar`, without receiving target PIDs. Root plus
`CAP_SYS_PTRACE` gives the scanner visibility across the fixture UIDs. Each attachment child drops to the target's
UID/GID, clears supplementary groups and enables `no_new_privs`. It creates a fresh directory in target `/tmp` through
`/proc/<pid>/root`, copies the jars, and calls the bundled JDK's Attach API. The target needs no monitoring arguments,
remote JMX port, exporter or JDK. The scanner itself is a lab program, not a production privilege boundary: **never run
it in a host PID namespace or against existing applications**. Its environment guard is an accidental-use check.

Attempts are keyed by PID/start time and journaled before launch. The journal survives restarting this monitor container;
it does not implement recovery after monitor-container replacement, uncertain attach reconciliation or safe upgrades.
The cap is 64 lifetime attempts and each helper has a 30-second deadline. Failed/uncertain attempts are not retried for
the same process identity. Artifacts persist until the target containers are removed. Service names derived from UID
are fixture identities, not a production service-discovery design.

The automatic run checks two UIDs, explicit dynamic-agent-loading denial, read-only target `/tmp`, duplicate suppression
after monitor restart, and automatic reattachment after an exact identified application child exits. Both successful
applications have served requests before initial attachment. The restart may attach during application startup. The
replay requires fresh stored JVM, HTTP and pool contexts belonging to the new process identity, beyond the restart time.
See [the results and limitations](AUTOMATION_RESULTS.md).

The cost study uses the original JDK 21 fixture for all modes. `baseline` has no agent or JMX probe. `full` uses stock
instrumentation plus the Hikari extension, with stock Hikari instrumentation disabled. `narrow` disables default
instrumentation and enables only `runtime-telemetry`, `servlet`, `tomcat`, `spring-webmvc` and `netdata-spike-hikari`.
These options are monitor-owned; they are not proposed app-owner configuration. Traces/log exporters are disabled in
both instrumented modes; that does not disable all internal tracing work.

Three blocks rotate baseline/full/narrow ordering. Each fresh application gets a two-CPU quota, 128–256 MiB heap,
10 seconds before attachment and 20 seconds after attachment of warmup, then 30 seconds of measurement. A separate
Java HTTP client shares only its target's network namespace, sends 300 requests/second with connection reuse and
`holdMs=0`, and records dispatch lag, response latency and exact status counts. This avoids the earlier host forwarding
and intentional pool-wait bottlenecks. CPU comes from target `/proc/1/stat` deltas around the measurement command;
RSS is sampled at the end. These are repeated synthetic observations, not production overhead bounds or long-run
memory estimates. The load generator restarts per phase, so client warmup remains a limitation.

`automatic-results.json`, `cost-results.json`, raw OTLP, chart/data captures and logs support replay. The cost verifier
requires all nine balanced cases; one `--cost-rounds` block is a diagnostic run and cannot satisfy the full verifier.
