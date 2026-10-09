# Installed native Java plugin results

The experimental `java.plugin` works on native non-root Linux JVMs using Go orchestration, fixed privileged Attach
operations and a bundled Java runtime/agent. It is opt-in through `--enable-plugin-java`; normal builds leave it OFF.
See the [plugin guide](../../src/collectors/java.plugin/README.md) for installation, configuration and metric definitions.

## Verified environment and behavior

Validation used Debian 13 amd64, the actual CMake-built Netdata daemon, `java.plugin`, `java-helper` and `ndsudo`, and
the actual `plugin-java` installation component in an isolated root-owned prefix. No system Java/Maven package or
existing service was changed. The application fixture was Spring Boot 3.5.7 with Hikari/H2 on bundled Temurin 21.0.12.1+1.
OTel remained the pinned stock 2.32.0 distribution with the existing Netdata Hikari extension.
The final run used implementation commit `ed8a0be2da`. After evidence export, all 30 transient units from the six
installation attempts were inactive, and the exact temporary build/installation directories were removed. The
existing Netdata service remained active and the three pre-existing Docker containers remained running.

Two applications ran under distinct non-root numeric UIDs; one used systemd `PrivateTmp`. Their commands had no
monitoring flags. Both supplied fresh JVM memory, HTTP rates/duration and Hikari connections/pending/limit measurements.
A JVM with Attach disabled appeared as **Blocked**, and the root-owned fixture appeared as **Unsupported**.

| Phase | Active Java charts | Result |
|---|---:|---|
| Initial discovery/attachment | 16 | All six contexts for both applications |
| Netdata restart | 16 | Saved port, credential fingerprints and attachment state unchanged |
| Display-name change | 16 | Chart IDs unchanged; display label updated |
| Exclude one application | 8 | Excluded application's charts absent |
| Application restart | 16 | New process identity with all six contexts and fresh samples |

The harness requires recent finite, nonnegative values for every selected chart dimension, positive HTTP 200/503
rates after traffic, and the duration heatmap's `observations/s` units. Each workload sends 1,500 requests: 1,350 HTTP 200,
150 HTTP 503, with zero client transport errors. These client counts are not an exact cumulative-counter equivalence
proof for this run. Earlier source/ingestion evidence and current regression fixtures cover histogram/count mapping.

The dashboard's Java navigation and pool/HTTP chart metadata were inspected in an anonymous browser session.
Application inventory and configuration changes were verified through the real Agent API and configuration files.
Authenticated DynCfg form rendering was not verified. The schema is included for the normal framework UI.

## Failures corrected during installation testing

- Helper mode `0750` prevented re-execution after dropping to an unrelated application UID. The same Linux worker
  regression failed at `0750` and passed at `0755`; the helper is not setuid and rejects root-only operations from
  ordinary users. Only `ndsudo` supplies elevation.
- Preserving the build user's umask made generated installed JARs/directories group-writable. The helper refused them.
  CMake now installs generated bundle files as `0644` and directories as `0755`, preserving pinned JDK executables.
- Installed paths pushed the Attach load argument beyond HotSpot's 1,024-byte limit. Fixed instrumentation selection
  now lives in trusted `otel.properties`; required exporter settings and process credentials remain explicit options.
  Oversized requests are rejected before loading. [HotSpot Attach limit](https://github.com/openjdk/jdk21u/blob/master/src/hotspot/share/services/attachListener.hpp)
- Configuration conflict checks now account for OTel's normalized property/environment aliases and untrimmed boolean
  parsing. Conflicting aliases cannot override the fixed exporter destination or instrumentation policy.
  [Pinned configuration reader](https://github.com/open-telemetry/opentelemetry-java/blob/v1.66.0/sdk-extensions/autoconfigure-spi/src/main/java/io/opentelemetry/sdk/autoconfigure/spi/internal/DefaultConfigProperties.java)
- Dynamic metric labels avoid permanent Vec caches. Expired gauges are removed; cumulative HTTP comparison state
  survives source silence and is released when its process retires. This keeps chart identity and counter validation.

## Validation and limits

Full plugin Go race tests and vet pass. Linux helper tests cover identity/namespace policy, dropped credentials and
capabilities, worker reaping/deadlines without CAP_KILL, and bounded input. The standalone Java test covers argument
byte limits, exporter option precedence, configuration aliases and environment size. Python harness regression and
command-status/redaction/ownership self-tests pass. Independent review covered privilege, restart, admission,
packaging and metric lifecycle; verified findings were fixed and checked again.

The complete source installer was not executed on the non-disposable VM. Its flag wiring, shell syntax and permission
steps were checked; the actual CMake build and install component supplied the native runtime evidence. The revised
Docker regression harness was not rerun in this phase. Linux arm64 compiled; this native live run covered amd64 only.
The earlier JDK/JRE 17/21/25 and container spike results remain historical evidence, not a claim that this first plugin
supports containers or every Java runtime/library. This run is not a controlled overhead benchmark.

`native_plugin_checks.py` reproduces the native lifecycle checks and saves raw samples, charts and sanitized state
fingerprints. Its output stays private because Agent responses/logs contain host/process identifiers. The committed
`native-plugin-evidence.json` contains only the resulting checks and synthetic workload totals. The helper never
records receiver credentials in these artifacts. See the [test instructions](README.md#native-linux-acceptance).
