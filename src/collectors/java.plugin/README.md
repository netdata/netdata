# Experimental Java application monitoring

`java.plugin` discovers running Java applications on the local Linux host and loads Netdata's bundled instrumentation.
The Go collector owns process discovery, attachment state, metrics, charts and the Java Applications table. Application
owners do not need an exporter, a JMX port, an additional Java installation or monitoring flags.

## Install and try it

From the Netdata source tree, use the normal installer with an explicit opt-in:

```sh
sudo ./netdata-installer.sh --enable-plugin-java
```

The option defaults to OFF, independently of other build defaults. Normal builds do not download or install the Java
bundle. `--disable-plugin-java` disables subsequent builds; it does not uninstall a previous opt-in installation.
Direct CMake builds use `-DENABLE_PLUGIN_JAVA=ON`, target `java-plugin`, and install component `plugin-java`. The
Agent's `ndsudo` must be built from the same enabled configuration and installed with its normal root:netdata 4750
permissions. The source installer handles those permissions, including executable permissions for `java-helper`.

The enabled build downloads checksum-pinned Temurin JDK 21.0.12.1+1, OpenTelemetry Java agent 2.32.0 and Maven 3.9.11.
Maven builds the included Hikari extension using pinned dependencies. Tools and Maven caches stay in the build
directory. The installed private runtime and agent live under `usr/share/netdata/java`, relative to the installation
prefix. The first build needs internet access and several hundred MB of download/build space. No system Java or
Maven package is installed. Runtime collection needs no internet connection.

The stock configuration enables discovery automatically. Start Netdata and open **Live → Java → Applications** for
application status and coverage, or **Metrics → java** for charts. The table's optional Details column explains blocked
or unsupported applications. Configuration accepts exact discovered application names:

```yaml
# /etc/netdata/java/java.conf (under the chosen installation prefix)
jobs:
  - name: java
    exclude_applications:
      - maintenance.jar
    application_names:
      checkout: Checkout API
```

The same fields are exposed through the Go framework's dynamic configuration schema. Display names do not change
process identity. Exclusion prevents attachment and metric publication; instrumentation already loaded into a JVM
remains until that application restarts.

## First-version support

- Native non-root Linux JVMs on glibc amd64/arm64. The spike exercised HotSpot JDK/JRE 17, 21 and 25; the helper
  requires Java 17 or newer. The bundled Attach runtime is Java 21. Other JVM implementations are unverified.
- Applications with ordinary permissions, including systemd `PrivateTmp` when the installed bundle remains visible.
  The bundle and its parent directories must be root-owned and not writable by other users.
- Root-owned JVMs and different PID, user or network namespaces/root filesystems are shown as unsupported.
  Container monitoring is a later capability; mounting host procfs into a container is not sufficient.
- A disabled Attach mechanism, hidden bundle paths, mixed process credentials, recognizable existing OpenTelemetry
  instrumentation or conflicting instrumentation settings prevents attachment. An application does not need a restart
  for initial supported attachment. Extremely long installation paths can exceed HotSpot's Attach argument limit and
  are rejected before loading instrumentation.
- Existing Hikari pools become visible after their next connection borrow. A pool with no subsequent borrow is not
  discovered by this extension. HTTP instrumentation depends on the supported OTel servlet/Tomcat/Spring integrations.

Current measurements are JVM heap/non-heap memory, HTTP request rates by method/route/status, HTTP duration
histograms, and Hikari active/idle connections, waiting requests and configured connection limits. **Not observed**
means no fresh measurement of that family; it does not mean zero activity or prove that a library is unsupported.
The first version does not promise arbitrary MBean coverage, traces, logs or universal Java-library instrumentation.

## Ownership and recovery

The plugin runs as the ordinary Netdata account. Only fixed discovery and attach operations go through `ndsudo` to
`java-helper`. The helper selects installed artifacts, verifies process identity and eligibility, then launches its
Attach child with the application's UID/GID, no supplementary groups or capabilities, and `no_new_privs`. It owns the
child deadline and reaping. Neither JVM options nor executable/JAR paths can be supplied through the collector form
or privileged command arguments. Secrets travel through stdin, never command-line arguments.

The receiver listens only on loopback. Each admitted process gets a random credential bound to its exact application
and boot/PID/start-time identity. A monitored application cannot use its credential to publish another application's
metrics. The loopback port, credentials and attachment outcomes persist in `var/lib/netdata/java/state.json` under an
exclusive lifetime lock. Treat this file as private recovery state: deleting it while instrumented JVMs remain alive
loses their receiver configuration. A busy saved port or damaged state stops attachment instead of choosing a new
port or guessing. Root or same-account modification of private state is outside the supported recovery model.

Each attempt is persisted before launch. Successful, blocked and ambiguous outcomes are never automatically retried
for the same process. Restarting Netdata preserves the endpoint and credentials; restarting an application creates a
new eligible identity. If an attachment is ambiguous, inspect Details and restart that application when appropriate.
Stopping the plugin stops its receiver and workers; it does not stop applications or unload their agent. Interactive
terminal runs refuse attachment so they cannot compete with the service's persisted state.

Source timestamps control freshness. Series disappear after five seconds without a fresh export; chart retirement
then follows the normal Go chart-engine lifecycle. Cumulative HTTP counter epochs remain separate across resets.
Expired gauge state is released. HTTP keeps its latest cumulative comparison per producer series until the process
retires, so a pause cannot make an older or regressed counter valid again.

## Where metrics and UI are defined

| Responsibility | Source |
|---|---|
| JVM/HTTP instrumentation | Pinned stock OpenTelemetry Java agent |
| Hikari instrumentation | `extension/src/main/java/org/netdata/spike/hikari/` |
| Privileged discovery and Attach supervision | `src/go/plugin/java/privileged/` |
| Fixed Java Attach options and instrumentation selection | `src/collectors/java.plugin/NetdataAttach.java`, `otel.properties` |
| Accepted metrics, units, labels and source semantics | `src/go/plugin/java/ingest/store.go` |
| Typed Netdata instruments | `src/go/plugin/java/collector/metrix.go` |
| Chart titles, groups, dimensions and labels | `src/go/plugin/java/collector/charts.yaml` |
| Operator form | `src/go/plugin/java/collector/config_schema.json` |
| Application table | `src/go/plugin/java/javafunc/router.go` |
| Job lifecycle, persistence and authenticated receiver | `src/go/plugin/java/collector/` |

An additional metric supported by the agent needs an explicit Go mapping, instrument and chart definition. A missing
Java-side measurement can be added to the bundled extension without replacing the Go orchestration approach. The
private Hikari metric/package names retain their spike prefix for compatibility with captured regression fixtures;
user-facing chart contexts use `java.*`.
