# Go orchestration, metrics and UI experiment

The existing Go framework can own the Java-monitoring lifecycle and presentation. The experiment discovers ordinary
running JRE applications, attaches bundled Java instrumentation, accepts OTLP metrics in Go, and publishes curated
Netdata charts plus a Java application inventory. No shared framework changes or new Go dependencies were needed.

These are historical results for commit `072604a2a3`, when the Go command selected only owned fixtures in a private
container PID namespace. The code has since moved into the installed `java.plugin`; current setup and definitions
are documented in [the plugin guide](../../src/collectors/java.plugin/README.md). The recorded evidence below remains
unchanged and does not by itself establish the native installed plugin's behavior.

## What the user sees

- **Live → Java → Applications:** application, runtime, monitoring status, and separate JVM, HTTP and pool coverage.
  Process identity, PID, location and diagnostics are optional columns.
- **Java charts:** heap/non-heap memory, HTTP request rate by response status, HTTP latency heatmaps, pool active/idle
  connections, pending requests and connection limit. Labels identify application, process and pool or route.
- **Configuration schema:** excluded application names and display names. Java paths, exporters, ports, Java
  dependencies and MBean names are absent from the operator form.

The inventory and charts rendered in the standard local Netdata dashboard. The unclaimed test Agent rejects
authenticated DynCfg HTTP operations with 412. Schema/get/test/update/restart were exercised through the same running
plugin's existing plugins.d input pipe, with responses recorded by a lab-only stdout tee. The configuration protocol
and lifecycle are validated; authenticated form rendering and Cloud authorization are not established here.

## Where definitions live

| Responsibility | Source |
|---|---|
| JVM and HTTP measurement | Pinned stock OpenTelemetry Java agent |
| Existing Hikari pool measurement | `src/collectors/java.plugin/extension/src/main/java/org/netdata/spike/hikari/PoolObserver.java` |
| Accepted metric names, types, units, labels and source-time rules | `src/go/plugin/java/ingest/store.go` |
| Typed Netdata metric instruments | `src/go/plugin/java/collector/metrix.go` |
| Chart titles, groups, units, dimensions and instance labels | `src/go/plugin/java/collector/charts.yaml` |
| Operator fields and help | `src/go/plugin/java/collector/config_schema.json` |
| Application table and coverage | `collector/collector.go` and `javafunc/router.go` under `src/go/plugin/java` |
| Discovery, admission, attachment and attempt journal | `collector/discovery.go` under `src/go/plugin/java` |
| Process composition | `src/go/cmd/javaplugin/main.go` |

Extending coverage does not require replacing orchestration. A supported OTel metric needs a deliberate Go mapping,
instrument and chart definition. A measurement the stock agent cannot produce needs Java-side work, such as the
existing Hikari extension. This experiment does not establish a universal mapping for arbitrary OTLP metrics.

## Ownership and lifecycle

```text
Go framework job
  Run     → fixture discovery → same-UID Attach helper → bundled OTel agent in application
          ← admitted-instance OTLP/HTTP receiver ← JVM/HTTP/Hikari measurements
  Collect → freshness-filtered snapshot → metrix → chartengine → Netdata
  Function → discovered applications + observed coverage
  DynCfg  → replacement job after the previous Run has joined
```

`Init` and `Check` validate without binding a socket, writing attachment state or injecting instrumentation. `Run`
owns and joins its receiver and discovery loop. It loads the task-private journal after exclusive listener acquisition.
The journal is synced before helper launch. Successful, failed and ambiguous attempts are remembered; restarting a
job does not blindly reinject. Application identity comes from an ordinary Spring application name, with a fixture-UID
fallback. Process identity adds the run token, PID and start time. Display names are labels only. Exclusion stops
admission and publication; loaded instrumentation remains until the application restarts.

## Metric semantics

- The pinned agent emits JVM memory as a non-monotonic cumulative Sum. It becomes a gauge, not a rate.
- Pool gauges are snapshots. Missing data is not invented as zero.
- HTTP duration uses cumulative explicit histograms. OTLP per-bucket counts become cumulative metrix buckets;
  count becomes an incremental request-rate dimension and buckets become a Netdata heatmap. Heatmap intensity is
  `observations/s`; bucket boundaries carry the observed duration in seconds.
- HTTP source start times differ between response statuses. Raw chart instances include method, route, status and
  source epoch. A new epoch gets a fresh counter baseline. Dashboard context grouping can combine statuses.
- Duplicate, older, ambiguous and unadmitted known points are explicitly rejected, alongside the tested malformed
  shapes and regressions of histogram totals, sums or cumulative bucket prefixes. Unknown metric families are
  ignored. Histogram bounds are pinned for the store lifetime. This is not exhaustive malformed-input validation:
  a decreasing individual range bucket can be hidden by growth in earlier buckets. No such regression appeared
  in the pinned producer's recording; accepting arbitrary producers would require validating each range too.
- Freshness uses each point's source timestamp. Five seconds without advancing source data removes coverage;
  framework retention then retires charts. Fresh unchanged cumulative counts can represent a valid zero rate.
  Replaying old observations cannot keep metrics fresh.

## Validation and limits

The live harness checks two already-running monitored applications and a JVM with attachment disabled. Both monitored
applications provide six stored chart contexts. Each initial workload sends 1,500 requests: 1,350 HTTP 200 and 150 HTTP
503. Raw histogram totals and stored rates are checked separately. The blocked JVM has an explicit inventory row.
The restarted application also captured one HTTP 200 readiness request before the measured workload: six one-count
exports precede workload traffic. Its total is therefore 1,351 HTTP 200 plus 150 HTTP 503; the verifier accounts for
this separately and still requires exactly 1,350/150 workload responses and zero transport errors.

Rename preserves process and chart identity. Exclusion/re-enable and job restart do not increase attachment attempts.
Application restart creates a new identity and one new attempt (three initial attempts, four final). Pausing an owned
application removes its fresh coverage and active charts; resuming restores collection without reinjection.
Race tests cover pinned metric shapes, histogram conversion, duplicate/out-of-order/reset data, descriptor bounds,
freshness, admission, concurrency and receiver-to-chartengine publication.

Commands are in [README.md](README.md). Sanitized evidence is `go-evidence.json`; raw recordings, protocol output,
chart samples and screenshots stay in `.local/java-monitoring-spike/go-final-verified`. Every final chart dimension
has a stored sample within ten seconds of capture. Independent source review, race tests and vet found no blocker
for this fixed-fixture experiment; the corrected heatmap units were also verified in the live dashboard.

Production discovery, privilege separation, container identity, authenticated form rendering, packaging, concurrent
monitors, upgrades, recovery across host reboot and broad Java/library compatibility remain unproved. This receiver
records synthetic exports for evidence. The Go lab uses Docker's default capabilities plus SYS_PTRACE; the earlier
native-host three-capability measurement is separate. Earlier measurements found about 101 MiB additional RSS per
JVM for the selected instrumentation set; this phase does not solve that cost.

The related [Go/OTel orchestrator POC](https://github.com/netdata/netdata/pull/24166) manages an external OTel Collector
per job and feeds Rust. It does not provide this OTLP-to-metrix mapping. Shipping plugin boundaries and common
ingestion architecture still need a product/architecture decision.
