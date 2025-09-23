# TODO – WebSphere Collectors Framework Migration

## Goals
- Port the three legacy WebSphere collectors (`websphere_pmi`, `websphere_mp`, `websphere_jmx`) onto the ibm.d framework so they follow the same structure as the new AS400/DB2 modules.
- Split low-level data acquisition into reusable protocols (PMI/XML, MicroProfile/OpenMetrics, and JMX helper) so collectors only orchestrate business logic and metric shaping.
- Preserve the current breadth of metrics, filtering, and cardinality controls while improving maintainability, chart lifecycle handling, and type safety.
- Lay groundwork for reusing the new protocols in future IBM or Java-centric collectors (e.g. other OpenMetrics endpoints, generic JMX targets).

## Current State Snapshot
- **PMI collector**: migrated to `modules/websphere/pmi` with framework contexts; legacy `collector/websphere_pmi` code has been removed.
- **MicroProfile collector**: under `collector/websphere_mp`, wraps HTTP fetching of MicroProfile metrics, applies regex-based classification (JVM vs REST), and manually manages dynamic charts. Uses go.d Prometheus helpers indirectly but does not expose a reusable protocol.
- **JMX collector**: under `collector/websphere_jmx`, tightly couples module lifecycle with the embedded Java helper process. Collector handles helper orchestration, command routing, delta calculations, and chart management.
- All three collectors still rely on go.d patterns (manual charts, `module.Base`) rather than the new framework’s type-safe contexts.
- Tests exist for PMI and MP parsing plus the JMX helper, but they assume the old structure.

## Target Architecture
```
modules/
  websphere/
    common/            # shared config structs, label builders, helpers
    pmi/
      collector.go     # framework Collector implementation
      collect_*.go
      config.go
      contexts/
      module.go / module_stub.go
    mp/
      ... (same pattern)
    jmx/
      ... (same pattern)
protocols/
  websphere/
    pmi/
      client.go        # HTTP + XML streaming + caching helpers
      parser.go        # typed table-like access into PMI XML
  jmxbridge/
    client.go        # generic helper process manager + command plumbing
    helper_stub.go   # !cgo stub emitting explanatory error
  websphere/
    jmx/
      adapter.go     # maps WebSphere domains onto the generic bridge
      types.go
  openmetrics/
    client.go          # generic OpenMetrics/Prometheus fetch + parse
    parser.go
```
- Collectors depend on their protocol client and the generated `contexts` package; they expose only orchestration/domain logic.
- Protocol packages expose typed data structures (e.g. PMI nodes/servers/stats, OpenMetrics samples grouped by scope, JMX responses by domain) and utility iterators so collectors can map them to contexts deterministically.

## Protocol Design Notes
### PMI (PerfServlet XML)
- Provide an API that abstracts the XML traversal into table-like collections: e.g. `Fetch(ctx)` returning a snapshot containing hierarchical lookups (`Servers`, `ThreadPools`, etc.) plus helpers to enumerate stats.
- Handle PMI refresh windows (`pmi_refresh_rate`) and caching inside the protocol so collectors simply request logical datasets and leave delta/integral calculations to reusable helpers.
- Reuse/extend existing delta caches (time stat, average stat, integral) but move them into protocol-level utility structs so multiple collectors (future) can leverage them.
- Consider streaming parsing with `encoding/xml` decoder to avoid holding the whole document when not needed, while still producing predictable structures.

### MicroProfile / OpenMetrics
- Build a generic `protocols/openmetrics` client that can:
  - Fetch metrics text over HTTP (respecting auth/TLS config) with context cancellation.
  - Parse Prometheus/OpenMetrics into a typed representation (families, samples, labels) using existing `go.d/pkg/prometheus` parsers under the hood.
  - Provide convenience filters (by scope: base, vendor, application) and helpers to coerce numeric values and units.
- The WebSphere MP collector consumes this protocol to classify metrics into framework contexts (JVM, thread pools, REST endpoints, generic “other”).
- Ensure the protocol is reusable for other collectors that need OpenMetrics ingestion.

### JMX Helper / Java Bridge
- Encapsulate the helper process lifecycle in a generic `protocols/jmxbridge` package:
  - Manage helper jar extraction, process start/stop, command marshaling, and response decoding.
  - Surface a domain-agnostic command API (INIT/SCRAPE/etc.) so adapters can plug in command builders and response decoders.
  - Implement resilience logic (restarts, circuit breaker state) within the bridge so collectors just surface health metrics and orchestrate data mapping.
  - Provide a `!cgo` stub module that registers an informative error when CGO is disabled, matching the framework contract.
- Layer a WebSphere-specific adapter on top of the bridge that translates between high-level fetch routines (`FetchJVM`, `FetchThreadPools`, `FetchJDBC`, …) and the underlying helper commands, returning strongly typed structs instead of `map[string]interface{}`.

## Migration Phases & Tasks
### Phase 1 – Protocol Foundations & Shared Utilities
- [ ] Stand up `protocols/websphere/pmi` with HTTP client setup, PMI XML parsing, caching primitives, and unit tests using existing sample payloads from `collect_test.go`.
- [ ] Extract a reusable OpenMetrics client (`protocols/openmetrics`) leveraging the existing Prometheus parser; cover fetch + parse with fixtures from current MP tests.
- [ ] Stand up `protocols/jmxbridge` to host the helper lifecycle + command plumbing, then add a WebSphere adapter that ports logic from `jmx.go`/`resilience.go`; include lifecycle tests using stub responses.
- [ ] Create `modules/websphere/common` with shared config structs (cluster labels, cardinality defaults), label builders, and helpers for consistent context labeling across the three modules.
- [ ] Provide CGO stubs and ensure go vet/build succeed when CGO is disabled.

### Phase 2 – WebSphere PMI Module Migration
- [ ] Define `modules/websphere/pmi/contexts/contexts.yaml` covering existing metric families (JVM, thread pools, JDBC/JCA, JMS, web apps, APM, cluster, etc.) and regenerate code via `go generate`.
- [ ] Port configuration schema/validation into `config.go` and `module.yaml`, preserving feature flags, selectors, and cardinality controls from the legacy collector.
- [ ] Re-implement `CollectOnce` using the new PMI protocol: iterate typed datasets, apply selector filters, and populate contexts with type-safe setters.
- [ ] Move delta/time-average logic onto protocol utilities or framework state helpers to keep collector loops minimal.
- [ ] Recreate tests (unit + integration) using the new module structure; adapt existing fixtures to assert context output rather than raw chart IDs.
- [ ] Update documentation (`README.md`, stock config) to reference the new module path.
- [x] Delete the legacy `collector/websphere_pmi` package once parity tests pass.

### Phase 3 – WebSphere MicroProfile Module Migration
- [ ] Author contexts for JVM, vendor/thread-pool, REST endpoint metrics, and fallback “other” metrics with sensible families/priorities mirroring current dashboards.
- [ ] Implement the framework collector (`modules/websphere/mp`) that:
    * Uses the OpenMetrics protocol to fetch samples.
    * Classifies metrics via regex/prefix helpers (moved from legacy code) housed in `modules/websphere/common`.
    * Applies REST endpoint filtering/cardinality limits before exporting contexts.
- [ ] Port configuration (URL rules, TLS options, selectors) and regenerate schema.
- [ ] Add tests verifying metrics classification and context emission using sample metric payloads.
- [ ] Remove legacy `collector/websphere_mp` after verification.

### Phase 4 – WebSphere JMX Module Migration
- [ ] Build contexts for JVM, thread pools, JDBC/JCA, JMS, applications, clusters, and APM metrics with appropriate instance labels (server, cluster, servlet/EJB identifiers).
- [ ] Implement the framework collector using the new bridge + adapter:
    * Manage helper health metrics via protocol signals (connection state, circuit breaker).
    * Keep cardinality management and selectors (applications, pools, servlets, ejbs) but leverage framework state for instance lifecycle.
    * Translate protocol structs into context setters, handling precision scaling consistently.
- [ ] Port configuration handling (JMX URL, auth, classpath, feature toggles) and regenerate schema/stock config.
- [ ] Ensure helper jar embedding + extraction still works under the new layout (update build scripts if necessary).
- [ ] Rework unit/integration tests to exercise protocol mocks and verify context output; include helper process restart scenarios.
- [ ] Drop `collector/websphere_jmx` once migration is validated.

### Phase 5 – Integration, QA, and Cleanup
- [ ] Update CI workflows (fmt/vet/build/test) if new packages or go:generate steps require adjustments.
- [ ] Run `./build-ibm.sh` and smoke-test all three modules with existing configs (`/etc/netdata/ibm.d/websphere_*.conf`) to verify runtime behavior and dashboards.
- [ ] Refresh documentation index (e.g., `WEBSPHERE-MONITORING.md`) to point to new modules and protocols.
- [ ] Confirm framework dashboards render with the reorganized families and priorities; align with Netdata Cloud expectations.
- [ ] Archive/remove leftover fixtures or helper scripts from legacy collectors.

## Open Questions / Research Items
- Should PMI parsing expose a generic “table” abstraction reusable by future XML-based collectors? Investigate whether building a lightweight typed traversal helper is worth the effort vs. bespoke structs.
- For OpenMetrics, can we upstream the protocol into a shared location (`shared/prom`) for reuse beyond WebSphere? Evaluate scope before locking in API.
- The JMX helper currently embeds a WebSphere-focused JAR; consider generalizing it (or supporting multiple helper jars) to serve other Java platforms once the bridge is in place, and track helper versions during build.
- Determine how much of the existing resilience logic (circuit breaker, cached metrics) belongs in the protocol vs. collector (especially for surfacing health charts).
- Verify whether PMI and JMX modules can share cluster labeling/state helpers to avoid divergence in future features.
