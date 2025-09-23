# AS400 & DB2 Framework Migration Plan

## Context
- The `ibm.d` plugin now has a YAML-driven framework (`src/go/plugin/ibm.d/framework`) that auto-generates type-safe chart code, handles dynamic instances, and separates protocol logic from collector orchestration (see `src/go/plugin/ibm.d/modules/mq/*` and `src/go/plugin/ibm.d/CLAUDE.md`).
- Existing `as400` and `db2` collectors under `src/go/plugin/ibm.d/collector/` are pre-framework. They work today and are in production on this host (configs under `/etc/netdata/ibm.d/as400.conf` and `/etc/netdata/ibm.d/db2.conf`).
- We can refactor freely: the plugin has not shipped, but we must preserve the current module names (`as400`, `db2`) and remain 100% compatible with the existing configuration surface.
- When done, the legacy code can be removed (git history is our backup).

## High-Level Goals
1. Rebuild `as400` and `db2` collectors using the new framework structure (protocol + module directories, YAML-generated contexts, framework collector).
2. Maintain full feature parity: every metric currently exposed must map to a generated context/dimension.
3. Preserve configuration fields and semantics so existing jobs continue to work unchanged.
4. Reuse or refactor shared utilities (e.g. `pkg/dbdriver`, `pkg/odbcbridge`) to fit the protocol abstraction.
5. Provide documentation, schemas, and generation hooks identical to the MQ module pattern (README metadata, `config_schema.json`, etc.).
6. Ensure stub implementations exist for unsupported build targets (e.g. `!cgo`).
7. Validate against live configs (`/etc/netdata/ibm.d/*.conf`) before the rewrite lands.

## Shared Prerequisites
- [x] Review current collectors to catalogue all metric classes, label sets, and chart families (`src/go/plugin/ibm.d/collector/{as400,db2}/charts*.go`, `metrics.go`, `collect_*.go`).
- [x] Identify all configuration fields and defaults that must be preserved (match `/etc/netdata/ibm.d/as400.conf` and `/etc/netdata/ibm.d/db2.conf`).
- [x] Inventory SQL/ODBC access patterns and helper packages (e.g. `pkg/dbdriver`, `pkg/odbcbridge`, `sql_queries.go`). Determine what moves into protocols vs. collectors.
- [x] Audit stub files (`*_stub.go`) and ensure equivalent coverage post-refactor.
- [x] Decide on directory layout mirroring `modules/mq` (e.g. `src/go/plugin/ibm.d/modules/as400` & `.../modules/db2`) and matching protocol packages (e.g. `src/go/plugin/ibm.d/protocols/as400` & `.../protocols/db2`).

## Framework Migration Tasks
1. **Protocols**
   - [x] Design protocol clients that encapsulate all connection logic, retries, and raw data retrieval (AS400 complete; repeat for DB2 in Phase 2).
   - [x] Ensure protocols respect CGO requirements (AS400 uses ODBC/CLI). Document prerequisites.
   - [x] Provide stub implementations (`*_stub.go`) for non-CGO builds.

2. **Contexts & Metrics Definition**
   - [x] Derive YAML definitions for every chart/metric from the legacy collectors (e.g. disk, subsystem, job queues for AS400; database, buffer pool, tablespace, locks, wait times for DB2).
   - [x] Structure `contexts.yaml` into logical classes with label ordering that matches current dimension naming.
   - [x] Handle unit conversions via YAML `mul/div/precision` to avoid manual scaling (see `CLAUDE.md` guidance).
   - [x] Run `go generate` (metricgen + docgen) to create `zz_generated_contexts.go`, `metadata.yaml`, `config_schema.json`, and `README.md`.

3. **Collector Modules**
   - [x] Create `modules/as400/{collector.go,collect_*.go,config.go,init.go,module.go}` mirroring MQ structure.
   - [x] Embed `framework.Collector`, register generated contexts, and implement `CollectOnce()` using the new protocol client.
   - [x] Translate legacy data processing (e.g. caching, deduping, cardinality limits) into framework patterns (`CollectorState`, obsoletion, label instance IDs).
   - [x] Preserve configuration behaviour (defaults from `defaultConfig()`, per-job overrides, selectors/patterns, max limits).
   - [x] Recreate per-class collection guards (enable/disable blocks like `collect_disk_metrics`, etc.).
   - [x] Replace hand-crafted chart/metric updates with generated `contexts.<Class>.<Metric>.Set(...)` calls.

4. **Configuration & Documentation**
   - [x] Ensure `config.go` uses embedded `framework.Config` plus existing fields, maintaining YAML keys.
   - [x] Confirm `config_schema.json` exposes current options exactly (checkbox names, descriptions, defaults).
   - [x] Update module README to match generated format (feature overview, metric tables grouped by scope) and mention prerequisites (ODBC drivers, SSL, etc.).
- [ ] Update `CLAUDE.md` if new framework conventions or caveats arise. *(Pending any doc refresh once we freeze behaviour.)*

5. **Shared Dependencies**
- [x] Validate `pkg/dbdriver`, `pkg/odbcbridge`, and related helpers still work in the protocol abstraction; factor out duplicate logic if both collectors share the same DB access patterns.
- [x] Review licensing / build tags for CGO (ensure `//go:build cgo` blocks remain accurate).
   - [ ] Check if any cross-module utilities should move into a shared directory (`shared/`?).

## Validation & QA
- [ ] Implement unit tests where practical (e.g. config validation, label sanitisation) and adjust/inherit any existing ones.
- [x] Run `go test ./...` under `src/go` with CGO enabled to ensure new code compiles cleanly alongside MQ.
- [x] Use `./build-ibm.sh` and the documented dump mode to exercise AS400/DB2 modules in isolation.
- [x] Test with local configs from `/etc/netdata/ibm.d/` to confirm backwards compatibility (multiple DB2 instances and remote AS400 system).
- [x] Validate runtime on this host (or staging) before removing legacy collectors.
- [ ] Document migration notes (if any behavioural changes are unavoidable) in commit message / release notes.

## Decommission Legacy Code
- [x] After validation, remove old files under `src/go/plugin/ibm.d/collector/{as400,db2}` and their chart definitions. (AS400 done; DB2 pending.)
- [x] Delete legacy tests or helpers that become unused.
- [x] Update `src/go/go.mod` references if packages move. *(No changes required after deletions.)*
- [x] Ensure clean build/test pipeline (no stale references to removed files).

## Tracking & Progress
We will ship the migration in two phases so we can validate each module before touching the next.

**Phase 1 – AS400**
1. Shared analysis & protocol design (only work needed to unblock AS400). ✅ Done.
2. AS400 context YAML + generated code. ✅ Done.
3. AS400 protocol + collector implementation. ✅ Protocol client and framework collector in place.
4. AS400 validation with existing configs. ✅ Completed via local dump runs.
5. Remove legacy AS400 code once confidence is high. ✅ Legacy collector removed.

**Phase 2 – DB2**
6. Extend shared analysis for any DB2-specific pieces still pending. ✅ Done.
7. DB2 context YAML + generated code. ✅ Done.
8. DB2 protocol + collector implementation. ✅ Collector migrated to framework with protocol client.
9. DB2 validation with the four local instances. ✅ Confirmed across ports 21017–24017.
10. Remove legacy DB2 code. ✅ Legacy collector directory removed; modules now source of truth.

**Final wrap-up**
11. Cleanup & final QA (shared utilities, docs, go test, build-ibm.sh, etc.).

Keep the file updated as tasks complete to maintain transparency on migration status.
