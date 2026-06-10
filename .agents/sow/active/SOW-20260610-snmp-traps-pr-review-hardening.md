# SOW-20260610-snmp-traps-pr-review-hardening - SNMP Traps PR Review Hardening

## Status

Status: in-progress

Sub-state: non-decision Function framework, secret handling, lazy PEN loading,
compressed profile installation, retry classification, writer shutdown,
listener observability hardening, the approved persistent-journal storage path,
the binary journal encoding metric rename, decode-error journal entries,
listener read-error logging, final Go 1.26 cleanup, DynCfg test job-name
injection, direct-journal directory setup, and user-profile automatic reload are
implemented locally. The public manual `snmp_traps:reload-profiles` Function has
been removed from source/docs in favor of an internal watcher for user-supplied
trap profiles only. Stock trap profiles MUST NOT be watched for live reload;
Netdata upgrade paths may update stock profile files together with code that
interprets them, so stock changes are picked up by process/job restart rather
than by a live profile watcher.

## Requirements

### Purpose

Make PR #22652 reviewable and merge-safe by resolving the integration risks that
could surprise maintainers, regress existing Netdata behavior, or affect users in
unexpected ways.

The purpose is fit for production Netdata Agent use:

- keep existing Function, DynCfg, SNMP, systemd-journal, packaging, and
  installer behavior stable for users that do not enable SNMP traps;
- make SNMP trap behavior explicit and fail early at job creation when
  configuration or environment preflight fails;
- reduce installed footprint and memory use for users that do not use SNMP
  traps;
- keep the PR easy to review by removing unrelated changes and documenting every
  touched existing framework surface.

### User Request

The user wants a new SOW for the review-fix batch, focused on:

- what can go wrong during review;
- how the PR can affect users in unexpected ways;
- whether existing Netdata behavior may have been broken;
- fixes found by two PR review passes on PR #22652.

### Assistant Understanding

Facts:

- PR #22652 adds a new `snmp_traps` go.d collector, trap profile generator,
  stock trap profiles, direct journal writing, optional OTLP export, Function
  API integration, DynCfg schema, packaging/install capability changes, and
  systemd-journal facets.
- The current branch touches existing framework surfaces outside the new
  collector:
  - Function controller/registry/CLI public-name dispatch;
  - DynCfg coded-error retry behavior;
  - SNMP device/topology registries;
  - systemd-journal facet allowlist;
  - packaging and installer capability handling;
  - `snmputils` IANA PEN loading;
  - two trap profile extraction/generation toolchains:
    `src/go/cmd/snmptrapprofilegen/` and `tools/snmp-traps-profile-gen/`;
  - health file naming and generated integration docs.
- Two review passes found several real risks:
  - public Function route table ignores job methods;
  - public Function name collisions are silent;
  - `AgentWide` currently hides `__job` but still dispatches through a job;
  - `TRAP_JSON`, dedup fields, and `TRAP_TAG_*` are missing from the C-side
    systemd-journal field allowlist;
  - stock trap profiles currently install uncompressed;
  - IANA PEN data is eagerly embedded and parsed into memory;
  - transient environment failures are currently classified like invalid config;
  - an unrelated `ibm.d/as400` refactor is present in the feature PR;
  - duplicate profile extraction/generation tooling is likely to receive
    maintainer pushback because it creates two code paths to understand, test,
    and keep compatible;
  - some tests do not prove the new public Function paths.
- User direction:
  - stock profile YAMLs remain uncompressed in the repository, but installed
    stock profiles should be `.gz`;
  - user-supplied profiles must always be loaded and validated at job creation;
  - PEN data should not be encoded in Go and should be lazy-loaded only when
    needed;
  - trap journals should move out of `/var/cache/netdata/traps/`;
  - `/var/log/journal/netdata/snmp-traps/` is attractive because the existing
    systemd-journal plugin scans `/var/log/journal` recursively;
  - the duplicate Python trap profile pipeline should not remain in the PR as a
    second maintained implementation; the Go helper is the authoritative path;
  - implementation should wait until this review-fix SOW is clear.

Inferences:

- The highest review risk is not the new collector itself; it is the existing
  framework surfaces touched to expose `snmp:traps`, add trap log facets, install
  capabilities, and route direct journal files.
- The duplicate extraction/generation toolchain is a real maintainability risk:
  reviewers can reasonably ask why the PR ships a Go generator while keeping a
  Python pipeline that appears to do overlapping work.
- `snmp:traps` should be treated as a public contract. A silent collision or
  stale Function advertisement would be a user-facing bug, not just internal
  cleanup.
- Moving journals under `/var/log/journal` can reduce new watcher work, but it
  increases the need to prove compatibility with the existing
  systemd-journal.plugin watcher, journalctl, permissions, and retention.
- Installed `.yaml.gz` profiles reduce disk/package footprint, but loader,
  packaging, docs, tests, and regeneration expectations must all agree.

Unknowns:

- Whether `/var/log/journal/netdata/snmp-traps/<job>/` is the final approved
  storage path or whether `/var/log/netdata/snmp-traps/<job>/` should be used
  with explicit watcher wiring instead.
- Whether the cleanest `AgentWide` fix should be a true framework-level
  agent-wide handler or a narrower SNMP-traps-specific registration rule.
- Whether dynamic `TRAP_TAG_*` should be registered in the C facets engine as
  selectable/filterable fields, filter-only fields, or only documented for the
  embedded Go journal Function path.

### Acceptance Criteria

- Every finding from the two review passes is classified as fixed, rejected with
  evidence, or deferred with explicit user approval and tracking.
- For real bugs, focused tests fail before the fix where practical, then pass
  after the fix.
- Function public-name routing is safe:
  - job-level public names and aliases are either supported and tested, or
    explicitly rejected at registration;
  - public-name collisions fail loudly;
  - CLI and controller public-name resolution have matching tests.
- `snmp:traps` availability is correct:
  - the Function is only exposed when direct journal storage is available for at
    least one trap job;
  - OTLP-only jobs do not expose the direct-journal logs Function;
  - no plugin protocol change for Function removal is introduced in this PR.
- Trap journal storage uses the user-approved log directory, not
  `/var/cache/netdata/traps/`.
- If `/var/log/journal/netdata/snmp-traps/` is selected, tests or manual
  validation prove:
  - the writer creates all directories and fails at job creation on permission or
    creation failure;
  - `journalctl --directory` can read the files;
  - systemd-journal.plugin discovers nested trap journals through its existing
    recursive scan/watch path;
  - retention remains per job.
- Stock trap profiles install compressed as `.yaml.gz`, while repository files
  stay uncompressed `.yaml`.
- The profile loader supports user and stock `.yaml`, `.yml`, `.yaml.gz`, and
  `.yml.gz` as needed, with user files always eagerly loaded and validated.
- User-supplied trap profile changes are picked up automatically while trap jobs
  are running:
  - only user profile directories are watched/fingerprinted;
  - stock profile directories and stock catalogue files are not watched;
  - automatic reloads rebuild only user-supplied profiles and carry over the
    existing stock route/store until all jobs stop or the process restarts;
  - if a user profile replaces a stock filename, the carried-over stock routes
    for that filename are filtered out;
  - successful automatic reloads atomically swap the shared profile index;
  - failed automatic reloads keep the last valid profile index active, increment
    profile-load failure metrics, and leave the cache dirty so subsequent DynCfg
    test/apply validates and fails at job creation until profiles are fixed.
- The public `snmp_traps:reload-profiles` Function is removed from the visible
  Function surface. The public logs Function remains `snmp:traps`.
- IANA PEN lookup is lazy and disk-backed; no package init builds a large
  enterprise-number map for users that do not need it.
- Invalid configuration and invalid profiles still fail with non-retryable
  DynCfg errors at job creation, while transient environment failures are
  reported at job creation and remain retryable according to existing go.d
  framework expectations.
- The unrelated `ibm.d/as400` change is removed from the SNMP traps PR unless a
  separate CI blocker requires it and the user approves keeping it.
- C-side systemd-journal facet support and docs agree on which `TRAP_*` fields
  can be searched, filtered, or faceted.
- Trap profile extraction/generation has one clear maintained path:
  - either the Go generator is the only supported path and duplicate Python
    tooling is removed or clearly parked as non-installed historical research;
  - or both paths have distinct documented ownership, coverage, and tests.
- The PR description/change summary is updated with the existing-code surfaces
  touched and why.
- Focused validation passes locally:
  - `go test` for `funcctl`, `jobmgr`, `godplugin`, `snmp_traps`, `ddsnmp`,
    and `snmputils` affected packages;
  - profile loader gzip tests;
  - packaging/install checks affected by compressed profiles and journal path;
  - `git diff --check`.

## Analysis

Sources checked:

- `src/go/plugin/agent/jobmgr/funcctl/registry.go`
- `src/go/plugin/agent/jobmgr/funcctl/dispatch.go`
- `src/go/plugin/agent/jobmgr/funcctl/controller.go`
- `src/go/cmd/godplugin/main.go`
- `src/go/pkg/funcapi/method.go`
- `src/go/pkg/funcapi/response.go`
- `src/go/plugin/go.d/collector/snmp_traps/reload.go`
- `src/go/plugin/go.d/collector/snmp_traps/func_logs.go`
- `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go`
- `src/go/plugin/go.d/collector/snmp_traps/serialize.go`
- `src/go/plugin/go.d/collector/snmp_traps/load.go`
- `src/go/plugin/go.d/collector/snmp/ddsnmp/device_registry.go`
- `src/go/plugin/go.d/pkg/snmputils/sysinfo.go`
- `src/go/cmd/snmptrapprofilegen/main.go`
- `tools/snmp-traps-profile-gen/README.md`
- `src/collectors/systemd-journal.plugin/systemd-journal.c`
- `src/collectors/systemd-journal.plugin/systemd-journal-files.c`
- `src/collectors/systemd-journal.plugin/systemd-journal-watcher.c`
- `CMakeLists.txt`
- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md`

Current state:

- `funcctl.registry.rebuildMethodRoutesLocked()` only iterates
  `module.methods`, so job-level public Function names are not in the route
  table.
- `funcctl.registry.rebuildMethodRoutesLocked()` overwrites
  `methodRoutes[functionName]` without collision detection.
- `funcctl.dispatch.makeMethodFuncHandler()` returns `422` when no jobs exist
  and documents that `AgentWide` still dispatches through the first running job.
- `snmp_traps` registers `snmp:traps` as `AgentWide: true`,
  `RawRequest: true`, and `RequireCloud: true`.
- `snmp_traps` previously exposed a public `snmp_traps:reload-profiles`
  maintenance Function, but the user rejected this UX because profile reload
  should not be a manual action surfaced next to the `snmp:traps` logs viewer.
- `src/go/go.mod:23` already depends on `github.com/fsnotify/fsnotify`, which
  supports Linux inotify, BSD/macOS kqueue, Windows ReadDirectoryChangesW, and
  illumos FEN. `src/go/plugin/agent/discovery/file/watch.go` is an accepted
  in-repo pattern for fsnotify plus periodic refresh and event-settle debounce.
- Runtime validation on an installed PR build showed:
  - direct Function calls for `snmp:traps` return `404`, proving the Function is
    not registered by `go.d.plugin`, not merely hidden by the UI;
  - DynCfg `test` for an SNMP traps job fails with
    `Job initialization failed: job name is empty`;
  - `runDyncfgCmdTest()` creates a module via `newConfigModule()` and calls
    `Init()` directly, while normal job creation uses `jobFactory.createV2()`
    which injects `SetJobName(cfg.Name())` before config application;
  - `snmpTrapsLogsMethodConfig()` uses an `Available` callback backed by the
    active direct-journal job counter, so a failed job does not publish
    `snmp:traps`. This matches the direct-journal-only Function requirement.
  - recent installed-agent logs show the configured trap job failing at
    initialization with `mkdir /var/log/journal/netdata: permission denied`.
    `/var/log/journal` exists and is owned by `root:systemd-journal`, while
    `/var/log/journal/netdata/snmp-traps` does not exist.
- `journal_writer.go` currently stores trap journals under
  `buildinfo.CacheDir/traps/<job>`.
- `systemd-journal-files.c` adds `/var/log/journal` as a default directory and
  recursively scans it; `systemd-journal-watcher.c` recursively watches
  subdirectories.
- `systemd-journal.c` currently includes core trap fields but not `TRAP_JSON`,
  dedup summary fields, or dynamic `TRAP_TAG_*`.
- `serialize.go` emits `TRAP_JSON`, `TRAP_SUPPRESSED_COUNT`,
  `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, and
  `TRAP_TAG_<KEY>`.
- `load.go` currently reads only `.yaml` / `.yml` profile files with
  `os.ReadFile`.
- `load.go` eagerly loads user profile directories and builds a lazy stock
  route table from stock profiles/catalogue. This means the automatic watcher
  can be scoped to user profile directories without loading the full stock
  profile database into memory.
- `CMakeLists.txt` currently installs stock trap profile `.yaml` files raw.
- `snmputils/sysinfo.go` embeds `enterprise-numbers.txt` and builds the PEN map
  at package initialization.
- `snmptrapprofilegen` separately reads the PEN file from disk and can refresh
  it from the IANA URL.
- The project skill still says stock trap YAMLs "ship raw"; that conflicts with
  the current user decision that installed files should be compressed.
- The branch has unrelated diff in `src/go/plugin/ibm.d/modules/as400/helpers.go`
  compared with upstream.

Risks:

- **Function API regression:** changing dispatch can affect all go.d Functions,
  not only SNMP traps. Tests must cover existing module methods, job methods,
  aliases, public names, and CLI resolution.
- **DynCfg test regression:** collectors that require framework-injected job
  names fail the DynCfg test path even though the normal apply path can inject
  the same name correctly.
- **Packaging/service setup regression:** direct-journal trap jobs correctly
  fail at job creation when their journal directory cannot be created, but the
  installer/package/service layer must create the Netdata-owned subdirectory on
  systems that already have persistent journald enabled.
- **Stale Function UX:** if `snmp:traps` remains visible when no direct-journal
  job exists, users can click a logs Function that cannot return data.
- **Streaming/protocol risk:** Function removal is intentionally handled by a
  separate Netdata PR. This PR must not invent or extend plugin protocol removal
  behavior.
- **Filesystem/packaging risk:** moving journals into `/var/log/journal` may
  require ownership and permissions that differ by package/install path.
- **Retention risk:** trap journals must keep per-job retention and must not be
  accidentally managed as regular journald files by journald.
- **Footprint risk:** raw installed profiles and PEN data add large disk
  footprint; eager PEN parsing adds memory cost even for users that do not use
  the feature.
- **Review noise risk:** unrelated changes such as `ibm.d/as400` can distract
  maintainers or create ownership objections.
- **Duplicate tooling risk:** keeping both the Go generator and the Python
  extraction/classification/emission pipeline can be rejected as duplicated
  maintenance burden unless one is removed or clearly demoted to private
  historical tooling.
- **Docs/spec drift:** profile-format docs, skills, generated integration docs,
  and runtime behavior currently have several mismatches.

## External Review Intake

### Valid Findings

1. **SNMP community values are persisted in `TRAP_JSON`.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/decode.go:359` adds
       `snmpTrapCommunity.0` as a synthetic varbind for v1/v2c traps.
     - `src/go/plugin/go.d/collector/snmp_traps/serialize.go:172` and
       `src/go/plugin/go.d/collector/snmp_traps/serialize.go:497` serialize
       all varbinds into `TRAP_JSON`.
   - Impact:
     - v1/v2c community values are credentials. Persisting them in local journal
       files or forwarding them through OTLP would leak secrets to anyone with
       log access.
   - Required action:
     - Redact or omit the synthetic community varbind before journal/OTLP
       serialization.
     - Add a regression test proving `TRAP_JSON` does not contain community
       values while allowlist matching still works.

2. **Public Function route handling has incomplete collision and job-method
   semantics.**
   - Evidence:
     - `src/go/plugin/agent/jobmgr/funcctl/registry.go:319` rebuilds
       `methodRoutes` from module/static methods only.
     - `src/go/plugin/agent/jobmgr/funcctl/registry.go:327` overwrites
       `methodRoutes[functionName]` without collision detection.
     - `src/go/plugin/agent/jobmgr/funcctl/controller.go:216` documents that job
       methods ignore aliases and publish only canonical names.
     - `src/go/pkg/funcapi/response.go:8` documents `FunctionName` as a
       module/static method override, so job-method public-name support is a
       design choice, not an implied current contract.
   - Impact:
     - Future public-name collisions can route nondeterministically or silently
       replace a prior route.
     - If job methods are intended to support `FunctionName` or aliases, current
       dispatch and registration do not implement it.
   - Required action:
     - Add collision detection for public names.
     - Either explicitly validate/reject `FunctionName` and aliases on job
       methods, or implement and test full job-method support.
     - Add controller and CLI tests for the selected contract.

3. **The `snmp:traps` logs Function can be registered for OTLP-only jobs.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/init.go:34` registers
       `Methods: snmpTrapsMethods`.
     - `src/go/plugin/go.d/collector/snmp_traps/reload.go:30` always includes
       the `logs` method.
     - `src/go/plugin/go.d/collector/snmp_traps/init_test.go:448` proves an
       OTLP-only job creates no journal directory.
     - `src/go/plugin/go.d/collector/snmp_traps/func_logs.go:141` returns
       "direct journal output has no sources" when no direct-journal sources
       exist.
   - Impact:
     - This violates the requirement that the embedded logs Function should be
       available only when direct journal files exist.
   - Required action:
     - Make `snmp:traps` visibility depend on direct-journal jobs without adding
       new Function removal protocol semantics in this PR.
     - Add tests for direct-journal, mixed journal+OTLP, and OTLP-only jobs.

4. **Creation-time environment failures are all classified as non-retryable
   `422` errors.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/collector.go:98` through
       `src/go/plugin/go.d/collector/snmp_traps/collector.go:289` wraps bind,
       journal creation, engine-boots state, profile load, and OTLP preflight
       failures as `dyncfgCodedError{code: 422}`.
     - `src/go/plugin/agent/jobmgr/dyncfg_collector_callbacks.go:98` skips
       `scheduleRetryTask()` for `dyncfg.CodedError`.
     - `src/go/plugin/framework/jobruntime/job_v2.go:270` disables retry after
       init errors.
   - Impact:
     - Users correctly see failure at DynCfg apply time, but transient failures
       such as a temporarily unavailable OTLP endpoint or bind race become
       permanent until manual reconfiguration.
   - Required action:
     - Keep creation-time detection, but split invalid configuration/profile
       errors from retryable environment failures.
     - Add tests proving invalid config remains non-retryable and transient
       failures still schedule retry.

5. **Direct journal and SNMPv3 state paths are not aligned with final storage
   requirements.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/journal_writer.go:50` stores
       journal roots under `buildinfo.CacheDir/traps/<job>`.
     - `src/go/plugin/go.d/collector/snmp_traps/engineboots.go:17` hardcodes
       `/var/lib/netdata/snmp-trap`.
     - `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:158`
       documents the same hardcoded state path.
   - Impact:
     - Journal files live under cache, not log storage.
     - Custom install prefixes and non-default varlib paths can store engine
       state in the wrong place.
   - Required action:
     - Move journal files to the user-approved log path.
     - Move engine-boots and local-engine-id state to the configured Netdata
       state directory.
     - Validate directory creation and permissions at job creation.

6. **Installed stock profile and PEN footprint is paid by every installation.**
   - Evidence:
     - `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/` contains
       437 YAML files, 39 MB, and 1,214,657 lines.
     - `tools/snmp-traps-profile-gen/iana-enterprise-numbers.txt` is 4.9 MB.
     - `CMakeLists.txt:4278` installs stock profile YAML files raw.
     - `CMakeLists.txt:4285` installs the PEN snapshot.
   - Impact:
     - Lazy runtime loading avoids memory cost, but package/install disk
       footprint still grows for users that never enable traps.
   - Required action:
     - Keep repository profile YAMLs uncompressed.
     - Install stock profiles compressed as `.yaml.gz`.
     - Update loader, packaging, docs, and tests to match.

7. **IANA PEN data is duplicated and eagerly loaded in Go.**
   - Evidence:
     - `src/go/plugin/go.d/pkg/snmputils/sysinfo.go:140` embeds
       `enterprise-numbers.txt`.
     - `src/go/plugin/go.d/pkg/snmputils/sysinfo.go:144` builds the
       `enterpriseNumbers` map at package initialization.
     - `src/go/cmd/snmptrapprofilegen/main.go:2266` has a separate disk-backed
       PEN loader for the generator.
   - Impact:
     - All go.d users pay binary and memory cost for PEN lookup even when SNMP
       metadata enrichment is unused.
     - Runtime and generator have two PEN loading paths.
   - Required action:
     - Replace eager embed/map initialization with a lazy disk-backed loader.
     - Share parsing logic where practical.

8. **C-side systemd-journal facets are incomplete for fields emitted by the trap
   writer.**
   - Evidence:
     - `src/collectors/systemd-journal.plugin/systemd-journal.c:154` through
       `src/collectors/systemd-journal.plugin/systemd-journal.c:165` include
       core trap fields only.
     - `src/go/plugin/go.d/collector/snmp_traps/serialize.go:128` emits
       `TRAP_JSON`.
     - `src/go/plugin/go.d/collector/snmp_traps/serialize.go:132` through
       `src/go/plugin/go.d/collector/snmp_traps/serialize.go:134` emit dedup
       summary fields.
     - `src/go/plugin/go.d/collector/snmp_traps/serialize.go:144` emits dynamic
       `TRAP_TAG_<KEY>` fields.
   - Impact:
     - The embedded Go logs Function has its own SDK facet configuration, but
       the C systemd-journal Function will not expose the same trap fields if it
       scans the trap journals.
   - Required action:
     - Add the static dedup fields to the C-side included facets or document why
       they are not facets.
     - Investigate the established C facets pattern before deciding how dynamic
       `TRAP_TAG_*` should behave.
     - Treat `TRAP_JSON` carefully because it is large and high-cardinality.

9. **`Function.Context` is a new request-lifetime contract for all Function
   handlers.**
   - Evidence:
     - `src/go/plugin/framework/functions/parser.go:26` adds `Context` to
       `functions.Function`.
     - `src/go/plugin/framework/functions/manager_worker.go:37` sets it from
       the worker request context for every invocation.
     - `src/go/plugin/agent/jobmgr/funcctl/dispatch.go:55` uses it as the
       parent context for method execution.
   - Impact:
     - This is probably the correct behavior for cancellation, but it changes
       the implicit lifetime available to all existing handlers.
   - Required action:
     - Add a doc comment that the context is valid only for the handler call.
     - Add a test for cancellation propagation through a normal non-raw method.

10. **`CAP_NET_BIND_SERVICE` is a real security-review surface, even if it is
    required for UDP/162.**
    - Evidence:
      - `netdata-installer.sh:1004`,
        `packaging/cmake/pkg-files/deb/plugin-go/postinst:13`,
        `packaging/makeself/install-or-update.sh:200`,
        `netdata.spec.in:935`, and
        `system/systemd/netdata.service.in:56`.
    - Impact:
      - Every `go.d.plugin` process receives the capability, not only trap jobs.
      - This matches the single-binary capability pattern used by go.d for
        other capabilities, but reviewers will still ask for justification.
    - Required action:
      - Keep the capability if UDP/162 support remains mandatory.
      - Document the rationale and confirm no SELinux/package policy update is
        needed.

11. **The unrelated `ibm.d/as400` refactor is scope noise.**
    - Evidence:
      - `src/go/plugin/ibm.d/modules/as400/helpers.go:242` through
        `src/go/plugin/ibm.d/modules/as400/helpers.go:257` changes string
        parsing to `strings.Builder`.
    - Impact:
      - Maintainers can reasonably reject unrelated collector changes in this
        PR, even if the change was made to silence a quality gate.
    - Required action:
      - Remove it from this PR, or split it into a separate PR if still needed.

12. **The legacy Python trap-profile pipeline is duplicate maintained-looking
    tooling.**
    - Evidence:
      - The branch adds `tools/snmp-traps-profile-gen/classify.py`,
        `emit.py`, `extract.py`, `iana_pens.py`, `sample_review.py`,
        `requirements.txt`, and `README.md`.
      - The branch also adds the shipped Go helper under
        `src/go/cmd/snmptrapprofilegen/`.
    - Impact:
      - Two visible extraction/generation paths create review and maintenance
        burden.
    - Required action:
      - Follow the recorded user decision: remove the Python pipeline from the
        PR as maintained tooling.
      - Preserve durable lessons in specs/docs/tests and keep only private
        scratch outside the PR if needed.

13. **Active and legacy SOW working files are merge blockers.**
    - Evidence:
      - The branch adds `.agents/sow/active/SOW-*.md`,
        `.agents/sow/done/SOW-*.md`, and `.agents/sow/pending/SOW-*.md`.
      - The SOW workflow rejects active/pending/done working files before merge.
    - Impact:
      - This is acceptable during draft/handoff, but must be cleared before the
        merge head.
    - Required action:
      - Delete active/pending/done SOW working files before merge after durable
        knowledge is transferred.
      - Keep durable specs under `.agents/sow/specs/` as needed.

14. **PR description is stale and currently overclaims zero footprint.**
    - Evidence:
      - `gh pr view 22652` shows the PR body still says the journal SDK is
        `v0.5.1`.
      - `src/go/go.mod` pins `github.com/netdata/systemd-journal-sdk/go
        v0.6.3`.
      - The PR body says agents not using traps have "no added footprint"; the
        installed data footprint in finding 6 contradicts that wording.
    - Impact:
      - Maintainers will review against inaccurate claims.
    - Required action:
      - Update the PR description after the hardening batch.

15. **Listener read errors are not operator-visible.**
    - Evidence:
      - `src/go/plugin/go.d/collector/snmp_traps/listener.go:73` reads from the
        UDP socket.
      - `src/go/plugin/go.d/collector/snmp_traps/listener.go:76` sleeps and
        retries on read errors without an error counter.
    - Impact:
      - Persistent listener/socket errors can degrade trap ingestion with only
        logs, not metrics or health.
    - Required action:
      - This matches existing pending observability debt. Decide whether to fold
        it into this hardening batch or leave it tracked separately.

16. **Secondary OTLP close/flush errors are counted but not returned.**
    - Evidence:
      - `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:29`
        returns only the primary `Flush()` error.
      - `src/go/plugin/go.d/collector/snmp_traps/trapwriter_fanout.go:38`
        returns only the primary `Close()` error.
    - Impact:
      - This may be intentional because journal is the primary path, but it
        should be documented or aggregated if loss on OTLP shutdown matters.
    - Required action:
      - Add a short comment or aggregate errors if tests show the caller can use
        the secondary error safely.

17. **Runtime/package checks may now require `snmp-trap-profile-gen`
    unconditionally.**
    - Evidence:
      - `packaging/runtime-check.sh:61` lists `plugins.d/snmp-trap-profile-gen`
        in `NETDATA_LIBEXEC_PARTS`.
      - `CMakeLists.txt:3454` builds the helper when `ENABLE_PLUGIN_GO` is on.
    - Impact:
      - This is probably correct for plugin-go packages, but it is another
        install-surface assertion to validate in CI/static builds.
   - Required action:
     - Keep if all plugin-go packages install the helper; otherwise make the
       runtime check conditional.

18. **CWE-117 "sanitized" wording is misleading because the code counts
    binary-encoded fields; it does not sanitize their values.**
    - Evidence:
      - `src/go/plugin/go.d/collector/snmp_traps/cwe117.go:34` counts fields
        whose value needs binary journal encoding.
      - `src/go/plugin/go.d/collector/snmp_traps/serialize.go:445` increments
        the count when writing raw payload fields.
      - `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml:478`,
        `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml:597`,
        `src/health/health.d/snmp_trap.conf:232`, and
        `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md:491`
        describe these fields as sanitized.
    - Impact:
      - The current implementation may be safe because binary journal encoding
        prevents newline/control-byte log injection, but the public metric and
        docs claim a stronger transformation than the code performs.
    - Required action:
      - Rename the metric/dimension/docs before release to describe binary or
        unsafe field encoding, or implement real value sanitization.
      - Add a test that proves the final contract.

19. **UDP receive-buffer sizing and kernel-drop observability are missing.**
    - Evidence:
      - `src/go/plugin/go.d/collector/snmp_traps/listener.go:46` binds sockets
        with `net.ListenUDP()`.
      - No `SetReadBuffer()` call exists in the listener path.
      - `src/go/plugin/go.d/collector/snmp_traps/listener.go:73` through
        `src/go/plugin/go.d/collector/snmp_traps/listener.go:79` retries read
        errors without a counter.
    - Impact:
      - Trap bursts can be dropped by the kernel before Go sees them, and
        operators may have no metric explaining lost traps.
    - Required action:
      - Add a bounded, configurable receive-buffer setting if it fits existing
        go.d configuration patterns.
      - Add an operator-visible metric for listener read errors, and investigate
        whether kernel UDP drop counters can be exposed portably.

20. **The Go profile generator is installed as a runtime helper in
    `plugins.d`.**
    - Evidence:
      - `src/go/cmd/snmptrapprofilegen/main.go:405` defines the
        `snmp-trap-profile-gen` CLI.
      - `CMakeLists.txt:3459` builds the helper with `ENABLE_PLUGIN_GO`.
      - `netdata.spec.in:936`,
        `packaging/cmake/pkg-files/deb/plugin-go/postinst:9`,
        `packaging/makeself/install-or-update.sh:183`, and
        `packaging/runtime-check.sh:61` treat it as an installed plugin-go
        artifact.
    - Impact:
      - The helper is useful for operators with custom MIBs, but maintainers may
        reject adding an offline authoring tool to the runtime plugin directory
        and package checks.
    - Required action:
      - Decide whether the helper remains installed by default, moves outside
        `plugins.d`, is gated behind a build/package option, or is kept only as
        source/developer tooling.

21. **OTLP plaintext transport is allowed for non-loopback endpoints without a
    distinct insecure-transport switch.**
    - Evidence:
      - `src/go/plugin/go.d/collector/snmp_traps/otlp.go:30` defaults to
        `http://127.0.0.1:4317`.
      - `src/go/plugin/go.d/collector/snmp_traps/otlp.go:189` through
        `src/go/plugin/go.d/collector/snmp_traps/otlp.go:219` treats
        `http://host:port` and bare `host:port` as insecure transport.
      - `src/go/plugin/go.d/collector/snmp_traps/otlp.go:264` through
        `src/go/plugin/go.d/collector/snmp_traps/otlp.go:269` uses
        `insecure.NewCredentials()` for those endpoints.
      - `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:326`
        documents the plaintext behavior.
    - Impact:
      - The localhost default is reasonable, but a remote plaintext endpoint can
        leak trap contents and any non-redacted structured fields over the
        network.
    - Required action:
      - Decide whether non-loopback plaintext needs an explicit
        `insecure: true` option, a warning metric/log, or documentation-only
        treatment.

### Validated Review-Risk Classifications

1. **SELinux policy impact for UDP/162.**
   - Evidence:
     - No SELinux policy source files (`*.te`, `*.fc`, `*.if`, `*.cil`) were
       found in the repository.
     - `packaging/installer/netdata-updater.sh:324` is the only narrow
       SELinux-related packaging hit found in this branch-local search.
     - Current packaging/service changes add `CAP_NET_BIND_SERVICE`, not an
       SELinux module.
   - Classification:
     - Not a code blocker for this PR unless maintainers require shipped SELinux
       policy before merge.
   - Follow-up:
     - Track shipped SELinux policy as a separate packaging/security SOW or
       GitHub issue. It needs Fedora/RHEL-family packaging design, policy source,
       scriptlets, file contexts, port labeling, and enforcing-mode validation.

2. **Profile load latency and memory for the full stock pack.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/load.go:82` walks profile files
       only when the shared profile cache is built.
     - `src/go/plugin/go.d/collector/snmp_traps/profile.go:236` loads that cache
       on first active job, not process startup.
     - Benchmark on 2026-06-10:
       `BenchmarkBuildStockProfileStoreDefaultProfiles-24` reported
       `157494319 ns/op`, `81898957 B/op`, and `615192 allocs/op`.
   - Classification:
     - Not an always-on memory risk because the module has no default jobs and
       profiles are lazy/shared. First active job pays about 157 ms and transient
       allocation with the current stock pack.
   - Follow-up:
     - Keep watching this as the full generated profile corpus changes size.

3. **Topology enrichment specificity.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_topology/topology_trap_enrich.go:45`
       maps source IP to a specific `ifIndex` when possible.
     - `topology_trap_enrich.go:70` through `topology_trap_enrich.go:72` only
       loads neighbors for that interface.
     - `topology_trap_enrich.go:77` through `topology_trap_enrich.go:104`
       filters LLDP/CDP remote neighbors by local interface index.
   - Classification:
     - Reviewer concern rejected for current code. The enrichment does not dump
       all known neighbors when an interface can be identified.

4. **Raw logs Function scope/context propagation.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/func_logs.go:52` through
       `func_logs.go:69` passes the raw SDK request payload into the journal SDK.
     - `src/go/plugin/go.d/collector/snmp_traps/func_logs_test.go:42` through
       `func_logs_test.go:94` writes two job journals and proves
       `selections.__logs_sources=["local"]` returns only the selected job.
   - Classification:
     - Reviewer concern rejected for current code. Logs-source selection is
       carried by the raw request payload and is covered by a focused test.

5. **INFORM response varbind echo and response-size behavior.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/inform.go:19` through
       `inform.go:24` copies request varbinds into the GetResponse.
     - `src/go/plugin/go.d/collector/snmp_traps/listener.go:14` caps incoming
       datagrams at 8192 bytes.
     - `src/go/plugin/go.d/collector/snmp_traps/inform_test.go:15` through
       `inform_test.go:50` verifies v2c INFORM response varbind echo and that
       the response is not larger than the request for the covered case.
   - Classification:
     - Not a proven bug. Existing behavior matches the normal INFORM
       acknowledgement shape and has focused test coverage.
   - Follow-up:
     - Add a larger-payload test only if reviewers require stronger
       amplification proof.

6. **DynCfg default template binds all IPv4 interfaces on UDP/162 and accepts
   all source IPs.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/config_schema.json:24` through
       `config_schema.json:30` set the new-job default endpoint to
       `0.0.0.0:162`.
     - `config_schema.json:192` through `config_schema.json:205` default
       `allowlist.source_cidrs` to `0.0.0.0/0` and `::/0`.
     - There is no stock job, so no listener is started unless a user creates a
       job through IaC or DynCfg.
   - Classification:
     - Product decision accepted for this PR. UDP/162 is required for devices
       that cannot target another port, and the permissive allowlist is a
       template for explicit job creation, not default runtime behavior.

7. **Dedup fingerprint allocation cost.**
   - Evidence:
     - Benchmark on 2026-06-10:
       `BenchmarkDedupFingerprint-24` reported `290.1 ns/op`, `0 B/op`,
       `0 allocs/op`.
     - Benchmark on 2026-06-10:
       `BenchmarkDedupAdmitDuplicate-24` reported `378.9 ns/op`, `0 B/op`,
       `0 allocs/op`.
   - Classification:
     - Reviewer concern rejected for current code. Enabled dedup fingerprinting
       is allocation-free in the focused benchmark.

8. **Reverse-DNS lookup concurrency.**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/enrich.go:44` sets
       `defaultReverseDNSConcurrent = 32`.
     - `enrich.go:58` initializes `lookupSem` with that capacity.
     - `enrich.go:145` through `enrich.go:152` use a non-blocking semaphore
       acquire and skip async lookup when the limit is full.
   - Classification:
     - Reviewer concern rejected for current code. Reverse DNS is disabled by
       default and bounded when enabled.

9. **Canonical module-method Function names when `FunctionName` is set.**
   - Evidence:
     - `src/go/pkg/funcapi/method.go:8` through `method.go:12` intentionally
       returns the override name when `FunctionName` is set.
     - `src/go/plugin/go.d/collector/snmp_traps/reload.go:16` sets the public
       name to `snmp:traps`, matching the user-approved Function hierarchy.
     - `src/go/cmd/godplugin/main_test.go:30` through `main_test.go:103` covers
       CLI resolution for canonical names, aliases, and explicit public names.
   - Classification:
     - Not a bug for this PR. `snmp_traps:logs` remains an implementation
       method name; `snmp:traps` is the public contract.

10. **Function `tags` and public names escaping.**
    - Evidence:
      - `src/go/pkg/funcapi/response.go:8` through `response.go:18` exposes
        public names and tags only through module-provided `MethodConfig`.
      - `src/go/plugin/agent/jobmgr/funcctl/controller.go:98` through
        `controller.go:115` checks public-name collisions before module
        registration.
      - `src/go/plugin/agent/jobmgr/funcctl/controller_test.go:475` through
        `controller_test.go:495` covers colliding public names.
    - Classification:
      - Not a proven injection bug because these fields are internal constants,
        not user-controlled configuration. Collision handling is covered.
    - Follow-up:
      - Broader plugin-protocol string validation can be handled as a framework
        hardening task if maintainers want it across all Function registrations.

### Rejected Review Findings

1. **"Unrelated DB engine / health changes are bundled."**
   - Evidence:
     - `git diff --stat upstream/master...HEAD -- src/database/engine
       src/libnetdata/spawn_server src/health` returned no changed files in the
       local branch state checked for this SOW.
   - Disposition:
     - Rejected as stale or reviewer-context error for the current branch.

2. **"The Function CLI always regressed to a full module scan before direct
   lookup."**
   - Evidence:
     - `src/go/cmd/godplugin/main.go:260` currently calls
       `resolveFunctionCLIRequestByPublicName()` before `registry.Lookup()`, so
       canonical names do scan first.
     - The practical severity is lower than claimed because the scan happens in
       CLI direct-execution mode, not on every daemon Function request. The
       daemon path uses `funcctl` route lookup first at
       `src/go/plugin/agent/jobmgr/funcctl/dispatch.go:40`.
   - Disposition:
     - Keep a CLI parity/performance test, but do not classify as a broad daemon
       hot-path regression.

3. **"`FUNCTION_REMOVE` protocol was introduced by this branch."**
   - Evidence:
     - `src/go/pkg/netdataapi/api.go:212` and
       `src/go/plugin/framework/dyncfg/responder.go:148` are unchanged from
       `upstream/master`.
     - The current implementation is a no-op placeholder.
   - Disposition:
     - Rejected as a branch-introduced protocol change. The constraint remains:
       this PR must not implement Function removal protocol semantics.

4. **"Dynamic engine-ID retry bypasses the source-IP allowlist."**
   - Evidence:
     - `src/go/plugin/go.d/collector/snmp_traps/collector.go:439` through
       `src/go/plugin/go.d/collector/snmp_traps/collector.go:447` apply the
       source allowlist before decode/retry for real UDP packets with a peer.
     - `src/go/plugin/go.d/collector/snmp_traps/listener.go:73` receives the
       UDP peer and `src/go/plugin/go.d/collector/snmp_traps/listener.go:85`
       passes it to the packet handler.
     - `src/go/plugin/go.d/collector/snmp_traps/dynamic.go:118` does not repeat
       the allowlist check, but it is reached after the outer check on the real
       UDP path.
   - Disposition:
     - Rejected for real network ingestion. Keep tests for nil-peer internal
       calls if they are exposed to test helpers or future transports.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- The PR introduces SNMP traps by touching shared framework and packaging
  surfaces. Several review findings are real because these shared surfaces were
  extended only enough for the first `snmp:traps` path, not for the full public
  contract they imply.
- The largest user-facing risks are:
  - direct journal logs stored under cache instead of log storage;
  - a public logs Function visible or routed when no direct-journal source can
    serve it;
  - profile and PEN footprint paid by all installations;
  - public Function name routing/collision issues in shared go.d framework code.
- The largest maintainer-facing risks are:
  - duplicate trap profile generation code paths;
  - broad existing-framework touches without enough tests;
  - unrelated changes in files outside the SNMP traps scope.

Evidence reviewed:

- Review pass 1 and review pass 2 pasted by the user.
- Code evidence listed in `## Analysis`.
- vLLM documentation for automatic prefix caching: vLLM reuses KV cache only
  when requests share the same token prefix; cache metrics measure queried and
  hit tokens over recent requests.

Affected contracts and surfaces:

- go.d collector module registration and Function public names.
- Function controller, registry, CLI, and raw response handling.
- DynCfg job creation error classes and retry behavior.
- SNMP collector registry lookup API used by trap enrichment.
- systemd-journal C plugin field allowlist and defaults.
- Direct journal filesystem layout, permissions, and retention.
- CMake/package/install layout for stock profiles and PEN snapshot.
- Trap profile loader file formats.
- SNMP trap generated docs, project skill, and PR description.

Clean-end-state target:

- The PR reaches a reviewable state where every existing framework touch has a
  clear reason, tests, and no unintended behavior change for non-SNMP-trap users.
- Trap logs are stored under the approved log directory and are queryable through
  the intended Functions/UI without extra operator surprises.
- Installed stock data is compressed and lazy-loaded enough that users who do
  not enable trap jobs do not pay avoidable disk or memory cost.
- Function public-name routing is either fully supported for module and job
  methods or explicitly constrained with validation and tests.
- The PR contains no unrelated source changes.
- Trap profile generation has one authoritative maintained implementation for
  production/operator use.

Removed as redundant (i):

- Raw installed stock trap profile YAMLs, if compressed install output is
  selected.
- Eager `snmputils` embedded PEN map initialization.
- Unrelated `ibm.d/as400` source diff, unless separately approved.
- Duplicate legacy profile extraction/generation tooling, unless explicitly
  retained as non-installed research/history with clear documentation and no
  operator-facing support promise.
- Any stale docs/skills that state stock trap profiles install raw.

Excluded coupled items (ii):

- Full Function removal protocol semantics are excluded because the user stated
  the separate Function deletion PR should handle this sensitive cross-Netdata
  behavior.
- Full systemd-journal virtual Function refactor is excluded; this SOW only
  hardens the SNMP traps PR path and field visibility.
- Full regenerated trap profile pack review is excluded; it is tracked by the
  full-MIB corpus SOW and background LLM run.

Reference search:

- Required before implementation for:
  - `FunctionName`, `Aliases`, `AgentWide`, and public Function route handling;
  - `/var/cache/netdata/traps`, `journalRoot`, and `journalBaseRoot`;
  - `snmp.trap-profiles/default/*.yaml` packaging/install references;
  - `enterprise-numbers.txt`, `lookupEnterpriseNumber`, and PEN file paths;
  - `TRAP_JSON`, `TRAP_SUPPRESSED_*`, `TRAP_REPORT_PERIOD_SEC`, and
    `TRAP_TAG_`.
  - `tools/snmp-traps-profile-gen` references from docs, skills, packaging, and
    SOW specs.

Existing patterns to reuse:

- Existing `funcctl` collision checks for job method IDs.
- Existing `funcapi.MethodFunctionNames()` helper for public names and aliases.
- Existing `pluginconfig.CollectorsUserDirs()` and
  `pluginconfig.CollectorsStockDir()` profile path conventions.
- Existing systemd-journal recursive scan/watch logic for `/var/log/journal`.
- Existing CMake install patterns for go.d profile directories.
- Existing test style in `funcctl`, `godplugin`, `ddsnmp`, and `snmp_traps`.

Risk and blast radius:

- Medium-high because the batch touches shared framework behavior, packaging,
  and default log discovery.
- Risk is manageable if changes are separated into focused commits/tests:
  Function routing, journal path, compressed profiles, PEN lazy loading,
  DynCfg errors, and cleanup/docs.

Sensitive data handling plan:

- Use only file paths, line numbers, field names, command names, synthetic IPs,
  and sanitized summaries in this SOW and follow-up docs.
- Do not write real device IPs, SNMP communities, private endpoints, customer
  identifiers, bearer tokens, or raw trap payloads to durable artifacts.

Implementation plan:

1. Add failing tests for real framework issues:
   - job-level public Function names;
   - public-name collisions;
   - CLI public-name resolution parity;
   - `snmp:traps` direct-journal availability behavior.
2. Fix Function routing/registration with minimal shared-framework change.
3. Fix `snmp:traps` availability so direct-journal logs are exposed only when
   direct journals exist, without adding plugin protocol changes.
4. Move trap journal root to the approved log path and validate directory
   creation/readability at job creation.
5. Compress installed stock trap profiles and update the loader for `.gz`
   support while keeping repository files raw.
6. Replace eager embedded PEN loading with a lazy disk-backed loader shared by
   runtime and generator where practical.
7. Split invalid configuration errors from transient retryable environment
   errors while preserving job-creation detection.
8. Complete cleanup:
   - remove unrelated `ibm.d/as400` diff or record explicit reason to keep it;
   - remove or explicitly demote duplicate Python trap profile tooling;
   - add comments for context lifetime and async per-job metrics globals;
   - rename health file if approved;
   - update systemd-journal field allowlist/comments;
   - update docs, skills, specs, and PR description.
9. Run focused validation and re-check PR CI.

Validation plan:

- Add tests before fixes where practical, then make them pass.
- Run:
  - `GOTOOLCHAIN=go1.26.0 go test ./plugin/agent/jobmgr/funcctl ./plugin/agent/jobmgr ./cmd/godplugin`
  - `GOTOOLCHAIN=go1.26.0 go test ./plugin/go.d/collector/snmp_traps ./plugin/go.d/collector/snmp/ddsnmp ./plugin/go.d/pkg/snmputils`
  - focused `snmptrapprofilegen` tests if PEN or prompt code changes;
  - `git diff --check`;
  - packaging/runtime checks affected by compressed profiles and journal path.
- Manually verify `journalctl --directory` on a synthetic trap journal under the
  selected log path.

Artifact impact plan:

- AGENTS.md: no update expected.
- Runtime project skills: update `project-snmp-trap-profiles-authoring` for
  compressed installed stock profiles and the final single-generator tooling
  contract.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` for journal path,
  profile compression, PEN loading, and Function availability.
- End-user/operator docs: update SNMP traps integration docs and profile format.
- End-user/operator skills: update query-snmp-traps if Function availability or
  log source selection changes.
- SOW lifecycle: keep this SOW branch-local during PR hardening; delete before
  merge after durable knowledge is transferred.

Open-source reference evidence:

- No additional external open-source repositories are needed for this SOW. The
  issues are integration contracts inside Netdata and the journal SDK path.

User-approved decisions:

1. Trap journal storage path:
   - Option A: `/var/log/journal/netdata/snmp-traps/<job>/`.
     - Pros: existing systemd-journal.plugin recursive scan/watch should pick it
       up without new watcher configuration; aligns with "show in Logs"
       behavior.
     - Cons: stores SDK-owned files under the journald tree; requires careful
       permission and retention validation.
     - Risk: journald or distro policy may treat the tree as journald-owned.
   - Option B: `/var/log/netdata/snmp-traps/<job>/`.
     - Pros: cleaner Netdata-owned application log path.
     - Cons: systemd-journal.plugin will not see it by default unless this PR
       also wires a new watched source or relies only on the embedded
       `snmp:traps` Function.
     - Risk: more code and review surface to get the same UI visibility.
   - Recommendation: Option A, if validation proves journalctl and
     systemd-journal.plugin discovery work reliably.
   - Decision: **Option A selected by user on 2026-06-10**. Use
     `/var/log/journal/netdata/snmp-traps/<job>/` if validation proves
     `journalctl` and systemd-journal.plugin discovery work reliably.
2. Transient environment failure retry semantics:
   - Option A: keep all job-creation failures non-retryable.
     - Pros: strongest fail-at-creation behavior.
     - Cons: transient startup failures can leave file-configured jobs down
       until manual intervention.
     - Risk: differs from normal collector retry behavior for temporary
       environment failures.
   - Option B: make transient environment failures retryable everywhere.
     - Pros: better unattended recovery.
     - Cons: can weaken DynCfg apply-time failure reporting if not separated
       from explicit apply.
     - Risk: users may see an apply succeed even though the listener is not
       actually ready.
   - Option C: keep DynCfg apply failures immediate and non-ambiguous, but make
     file-config startup transient environment failures retryable.
     - Pros: preserves the user requirement for DynCfg while improving restart
       recovery for file-config jobs.
     - Cons: requires careful framework-level distinction and tests.
     - Risk: incorrect classification can either hide real configuration errors
       or over-retry invalid jobs.
   - Decision: **Option C selected by user on 2026-06-10**.
3. `AgentWide` implementation:
   - Option A: implement true framework-level agent-wide handlers.
     - Pros: cleans up existing FIXME for all modules.
     - Cons: broad Function framework change with more regression risk.
   - Option B: constrain this PR to module/static public names and make
     `snmp:traps` availability depend on direct-journal jobs.
     - Pros: smaller shared-framework change for this PR.
     - Cons: leaves true agent-wide semantics for later.
   - Recommendation: Option B for this PR unless tests prove true agent-wide is
     simple and low-risk.
   - Decision: **Option B selected by user on 2026-06-10**. Keep this PR
     surgical and leave true framework-level agent-wide behavior for later.
4. Dynamic `TRAP_TAG_*` in C-side systemd-journal facets:
   - Option A: register dynamic `TRAP_TAG_*` as filterable/selectable.
   - Option B: keep labels visible/searchable only through the embedded
     `snmp:traps` Go Function and document the C plugin limitation.
   - Option C: investigate the C facets engine first, then decide the final
     behavior from evidence.
   - Recommendation: investigate the facets engine first; do not add a glob-like
     contract unless the C plugin has an established dynamic-key pattern.
   - Decision: **Option C selected by user on 2026-06-10**.
5. Duplicate profile extraction/generation tooling:
   - Option A: keep only the Go generator as the maintained tool and remove the
     legacy Python pipeline from the PR.
     - Pros: strongest review posture; one supported implementation; lower
       maintenance burden.
     - Cons: may lose scratch/research scripts unless preserved outside the
       repo.
     - Risk: if the Go generator does not fully replace the Python pipeline yet,
       removing it may slow regeneration debugging.
   - Option B: keep the Python tooling but mark it clearly as non-installed,
     historical/research-only, and not part of the operator-supported path.
     - Pros: preserves useful development artifacts.
     - Cons: still creates visible duplicate code in the PR and may not satisfy
       maintainers.
     - Risk: future assistants or contributors may keep editing the wrong path.
   - Option C: move the Python tooling to local/private SOW scratch and keep no
     duplicate in the PR.
     - Pros: same review benefit as removal while preserving local context.
     - Cons: not visible to future contributors unless durable lessons are moved
       into specs/docs/tests.
     - Risk: local-only scripts are easy to lose across machines.
   - Decision: remove the Python pipeline from the PR as maintained tooling.
     Preserve durable lessons in specs/docs/tests, and move any private scratch
     artifacts outside the PR if needed.
   - Rationale: the Go helper is the shipped, tested, packaging-integrated path.
     Keeping the Python pipeline creates duplicate code to maintain and is
     likely to receive maintainer pushback.

## Implications And Decisions

Approved decisions:

1. Storage path decision: **Option A selected by user on 2026-06-10**.
2. Transient retry semantics: **Option C selected by user on 2026-06-10**.
3. `AgentWide` scope decision: **Option B selected by user on 2026-06-10**.
4. `TRAP_TAG_*` C-facets decision: **Option C selected by user on
   2026-06-10**.
5. Duplicate tooling decision: decided. Remove the Python pipeline from the PR
   as maintained tooling; keep the Go helper as authoritative.

4C research result:

- `facets_register_dynamic_key_name()` is not wildcard field registration. It
  registers one named key and attaches a row-rendering callback:
  `src/libnetdata/facets/facets.c:1772` and
  `src/libnetdata/facets/facets.c:2783`.
- The only current C-side systemd-journal dynamic key is
  `ND_JOURNAL_PROCESS`, which computes one output column from other row fields:
  `src/collectors/systemd-journal.plugin/systemd-journal.c:186` and
  `src/collectors/systemd-journal.plugin/systemd-journal-annotations.c:491`.
- The default facet allowlist is a `simple_pattern` list:
  `src/libnetdata/facets/facets.c:1705`. `simple_pattern` treats a trailing
  `*` as prefix matching even when the default mode is exact:
  `src/libnetdata/simple_pattern/simple_pattern.c:53`.
- The systemd-journal Function passes `SYSTEMD_KEYS_INCLUDED_IN_FACETS` into
  that allowlist:
  `src/collectors/systemd-journal.plugin/systemd-journal.c:296`.
- Every journal field encountered is registered by exact field name while rows
  are processed:
  `src/collectors/systemd-journal.plugin/systemd-journal-execute.h:80`.
- Conclusion: supporting C-side `TRAP_TAG_*` facets does not require a new
  dynamic-facet API. Adding `|TRAP_TAG_*` to
  `SYSTEMD_KEYS_INCLUDED_IN_FACETS` should make encountered trap tag fields
  selectable/filterable as normal facets.
- Risk: adding `|TRAP_TAG_*` makes every encountered tag key a default facet and
  output column when the systemd-journal plugin reads trap journals. This is
  bounded by the existing facets limits, but can add UI noise and value-indexing
  cost if profiles or operators create many distinct tag keys.
- Recommendation: add `|TRAP_TAG_*` only after validating with a focused
  systemd-journal query fixture that tag fields become facets, exact tag
  selections work, and non-trap systemd-journal behavior is unchanged. Do not
  add new wildcard/prefix APIs to the facets engine in this PR.
- Decision: **Option A selected by user on 2026-06-10** after the investigation.
  Add `|TRAP_TAG_*` to the C-side systemd-journal facet allowlist with focused
  validation, and do not add new wildcard/prefix APIs to the facets engine.

## Plan

1. Write failing tests for the real framework/public-contract issues.
2. Implement the fixes in coherent batches:
   - Function routing and availability;
   - journal path and job-creation preflight;
   - compressed profile install and loader;
   - PEN lazy loading;
   - DynCfg retry/error classification;
   - duplicate tooling removal or demotion;
   - docs/spec/skill cleanup and unrelated diff removal.
3. Validate locally.
4. Update PR description and push.
5. Run a full review pass after the batch is complete.

## Pre-Implementation Gate

Status: in-progress

Problem/root-cause model:

- Public Function names are a shared namespace. The branch added
  `FunctionName` but only routed module/static methods, so job-method public
  names would appear valid while dispatching incorrectly.
- Public-name collisions were stored in a map with last-writer-wins behavior,
  so a later module could silently steal a Function name.
- The PR contained an unrelated `ibm.d/as400` helper refactor.

Evidence reviewed:

- `src/go/plugin/agent/jobmgr/funcctl/registry.go` route table construction.
- `src/go/plugin/agent/jobmgr/funcctl/controller.go` module/job method
  registration and unregister paths.
- `src/go/plugin/agent/jobmgr/funcctl/controller_test.go` existing lifecycle
  and job-method registration tests.
- `src/go/plugin/agent/jobmgr/funcdispatch_test.go` public module Function
  dispatch coverage.
- `src/go/plugin/framework/functions/parser.go` Function request context field.
- `src/go/plugin/ibm.d/modules/as400/helpers.go` unrelated diff against
  `upstream/master`.

Affected contracts and surfaces:

- Function public-name registration and dispatch.
- Job-method registration validation.
- Function request context lifetime documentation.
- Existing `ibm.d/as400` collector source diff scope.

Clean-end-state target:

- Module/static methods may use `FunctionName` and aliases.
- Job methods MUST NOT use `FunctionName` or aliases until job-method public
  alias registration/unregistration/dispatch is implemented end-to-end.
- Public Function name collisions MUST NOT silently overwrite an existing route.
- The unrelated `ibm.d/as400` change MUST be removed from the feature PR.

Removed redundant / unrelated items:

- Remove the `ibm.d/as400` string-builder refactor from this PR.

Excluded coupled items:

- True agent-wide dispatch remains excluded from this batch because it changes
  the Function execution contract and is tracked as an open decision in this
  SOW.
- Job-method public aliases remain excluded from this batch because supporting
  them requires coordinated publish, route, unregister, and API behavior; this
  batch rejects them explicitly instead.

Existing patterns to reuse:

- Existing job-method collision validation aborts the full job-method batch
  before publishing handlers.
- Existing module/static public names are registered only after first job start.

Risk and blast radius:

- Shared Function controller behavior can affect every go.d module that exposes
  Functions.
- Sorting module registration changes nondeterministic map iteration into a
  deterministic first-owner-wins policy for public-name collisions.

Sensitive data handling:

- Record only file paths, code behavior, and validation command names. Do not
  record secrets, SNMP communities, private endpoints, or live customer data.

Implementation plan for this batch:

1. Validate module/static public Function names before registry insertion.
2. Reject `FunctionName` and aliases on job methods.
3. Add tests for both behaviors.
4. Document `functions.Function.Context` lifetime.
5. Remove the unrelated `ibm.d/as400` diff.

Validation plan:

- Run focused Function controller/dispatch tests.
- Run affected package tests.
- Run `go vet` on affected Go packages.
- Run `git diff --check`.

Artifact impact:

- No end-user docs change in this batch.
- No generated profile or packaging artifact change in this batch.

Open decisions:

- None for the approved implementation batch.
- Dynamic `TRAP_TAG_*` behavior was investigated and approved separately in the
  `## Implications And Decisions` section.

## Execution Log

### 2026-06-10

- Created SOW from user request after two PR review passes identified
  merge-readiness and integration-risk findings.
- Committed the parser/tooling changes separately before running additional
  external review passes.
- Consolidated the user-provided and external review findings into
  `## External Review Intake`, separating valid findings, needs-validation
  risks, and rejected/stale review claims.
- Implemented the no-product-decision Function framework hardening batch:
  - sorted module registration for deterministic public-name collision
    handling;
  - skipped a module that attempts to claim a public Function name already
    owned by another module/method;
  - rejected `FunctionName` and `Aliases` on job methods until job-method
    public alias support exists end-to-end;
  - documented `functions.Function.Context` as handler-lifetime only;
  - removed the unrelated `ibm.d/as400` refactor from the feature PR.
- Implemented the low-risk C-side systemd-journal facet allowlist correction:
  - added dedup scalar fields `TRAP_REPORT_PERIOD_SEC`,
    `TRAP_SUPPRESSED_COUNT`, and `TRAP_SUPPRESSED_FINGERPRINTS`;
  - kept `TRAP_JSON` out of default C facets because the facets engine already
    has all-key full-text search enabled, while `TRAP_JSON` is high-cardinality
    payload data and is documented as searchable/viewable rather than a default
    facet;
  - kept dynamic `TRAP_TAG_*` out of this batch pending validation of the C
    dynamic-key/facet model.
- Removed the duplicate Python trap-profile generation pipeline from the PR:
  - deleted the tracked `tools/snmp-traps-profile-gen/` Python scripts and
    README;
  - moved the bundled IANA PEN snapshot into
    `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/`;
  - updated CMake, Go helper fallback paths, operator docs, project skills, and
    specs to use the Go helper as the only maintained profile-generation path;
  - removed negative user-facing wording that described implementation details
    users do not need to care about.
- Implemented the approved C-side `TRAP_TAG_*` facet path:
  - added `TRAP_TAG_*` to the systemd-journal default facet allowlist;
  - did not add a new facets-engine wildcard API because the existing
    `simple_pattern` allowlist already supports this prefix pattern;
  - validated with a synthetic SDK journal and the real
    `systemd-journal.plugin --test` Function query path.

## Validation

Acceptance criteria evidence:

- `git diff --check --no-index /dev/null
  .agents/sow/active/SOW-20260610-snmp-traps-pr-review-hardening.md`
  produced no whitespace warnings after the intake update.
- Focused Function controller/dispatch validation passes:
  - `go test ./plugin/agent/jobmgr/funcctl ./plugin/agent/jobmgr -run 'TestControllerLifecycleHooks|TestControllerRegisterJobMethods|TestExecuteFunction_ModuleMethodPublicFunctionName|TestExecuteFunction_ContextBehavior' -count=1`.
- Affected package validation passes:
  - `go test ./pkg/funcapi ./plugin/framework/functions ./plugin/agent/jobmgr/funcctl ./plugin/agent/jobmgr ./plugin/go.d/collector/snmp_traps ./cmd/snmptrapprofilegen -count=1`;
  - `go vet ./pkg/funcapi ./plugin/framework/functions ./plugin/agent/jobmgr/funcctl ./plugin/agent/jobmgr ./plugin/go.d/collector/snmp_traps ./cmd/snmptrapprofilegen`;
  - `git diff --check`.
- Targeted C build validation passes:
  - `ninja -C /tmp/netdata-snmptraps-cmake-yml-check-no-xen systemd-journal.plugin`.
- C-side dynamic trap tag validation passes:
  - generated a temporary compact, uncompressed SDK journal under
    `/tmp/snmp-trap-tags-journal.yGNs3h` with fields `TRAP_TAG_TENANT=edge` and
    `TRAP_TAG_REGION=lab`;
  - `systemd-journal.plugin --test systemd-journal --dir
    /tmp/snmp-trap-tags-journal.yGNs3h` returned status `200`, one row, and
    default facets for both `TRAP_TAG_TENANT` and `TRAP_TAG_REGION`;
  - the same Function query with
    `selections: {"TRAP_TAG_TENANT": ["edge"]}` returned one row with value
    `edge`;
  - the same Function query with
    `selections: {"TRAP_TAG_TENANT": ["missing"]}` returned zero rows.
- Python pipeline removal validation passes:
  - `rg` over current code, docs, specs, and skills finds no live references to
    `tools/snmp-traps-profile-gen`, `extract.py`, `classify.py`, `emit.py`,
    `iana_pens.py`, or `sample_review.py`;
  - `go test ./cmd/snmptrapprofilegen -count=1`;
  - `cmake -S . -B /tmp/netdata-snmptraps-cmake-yml-check-no-xen -G Ninja -DENABLE_PLUGIN_GO=ON -DENABLE_PLUGIN_XENSTAT=OFF`;
  - generated `cmake_install.cmake` installs
    `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/iana-enterprise-numbers.txt`;
  - `git diff --check` and `git diff --cached --check`.

### 2026-06-10 Review-Hardening Follow-Up Batch

Implemented locally:

- SNMP community values are omitted from serialized journal and OTLP varbind
  payloads while still being available for allowlist checks.
- The `snmp:traps` logs Function is advertised only when at least one active job
  writes direct journal files; OTLP-only jobs do not expose a local direct
  journal logs source.
- IANA PEN lookup no longer embeds or parses the 249k-line registry at package
  initialization. The runtime loads the installed `iana-enterprise-numbers.txt`
  lazily on first enterprise-OID lookup.
- DynCfg/job-runtime failures now distinguish invalid configuration/profile
  errors from retryable environment/startup failures:
  - invalid config/profile remains non-retryable `422`;
  - bind, journal directory/writer, SNMPv3 state persistence, and OTLP preflight
    failures are reported at job creation and remain retryable.
- Fanout writer `Flush()` and `Close()` now aggregate primary and secondary
  backend errors with `errors.Join()` so secondary OTLP shutdown/export errors
  are not hidden.
- Listener socket read errors after a successful bind are counted as
  `listener_read_failed`, exported in the `snmp.trap.errors` chart, documented
  in metadata/generated integration docs, and covered by a health alert.
- The duplicate OTLP export failure metric emission found while adding listener
  observability was removed.
- Added a normal-method Function context cancellation test, complementing the
  existing raw request context coverage.

Validation evidence:

- `go test ./plugin/go.d/collector/snmp_traps ./plugin/go.d/pkg/snmputils`
  passed.
- `go test ./plugin/framework/jobruntime ./plugin/agent/jobmgr
  ./plugin/agent/jobmgr/funcctl` passed.
- `integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps` completed
  without errors.
- `git diff --check` completed without warnings.

### 2026-06-10 Persistent-Journal Storage Batch

Implemented locally:

- Direct journal files now use `/var/log/journal/netdata/snmp-traps/<job>/`
  instead of `/var/cache/netdata/traps/<job>/`.
- Direct-journal jobs validate that `/var/log/journal` already exists before
  creating any Netdata-owned child directories.
- The plugin does not create `/var/log/journal`, avoiding an accidental change
  to host journald persistence behavior on systems where that directory is
  intentionally absent.
- Missing or invalid persistent journal parent paths fail at job creation with a
  retryable `503` coded error.
- SNMPv3 receiver-local state now uses the configured Netdata lib directory
  (`NETDATA_LIB_DIR`) when available, with `/var/lib/netdata/snmp-trap` only as
  the fallback.
- Metadata, generated integration docs, schema descriptions, and durable specs
  now describe the persistent-journal path and the parent-directory
  prerequisite.

Validation evidence:

- `go test -count=1 ./plugin/go.d/collector/snmp_traps
  ./plugin/go.d/pkg/snmputils` passed.
- `python3 integrations/gen_integrations.py` completed without errors.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
  completed without errors.
- `git diff --check` completed without warnings.
- `rg` found no remaining `/var/cache/netdata/traps` references in the SNMP
  traps specs, collector docs, or SNMP traps integration surfaces.

### 2026-06-10 Binary Journal Encoding Metric Rename

Implemented locally:

- Renamed the pre-release `sanitized` error dimension and
  `snmp_trap_errors_sanitized` metric to `binary_encoded` and
  `snmp_trap_errors_binary_encoded`.
- Renamed the health alert from `snmp_trap_sanitized_fields` to
  `snmp_trap_binary_encoded_fields`.
- Renamed internal writer/test APIs from `SanitizedFields()` to
  `BinaryEncodedFields()` so the code matches what actually happens.
- Updated metadata, generated integration docs, health text, and the durable
  Netdata SNMP traps spec to describe binary journal encoding, not value
  sanitization.
- Left the health file named `snmp_trap.conf` for now because the filename
  matches the chart and alert prefix (`snmp.trap.*` / `snmp_trap_*`). The module
  name match (`snmp_traps.conf`) is a review-style preference, not proven broken
  behavior.

Validation evidence:

- `python3 integrations/gen_integrations.py` completed without errors.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
  completed without errors.
- `go test -count=1 ./plugin/go.d/collector/snmp_traps` passed.
- `git diff --check` completed without warnings.
- `rg` found no remaining `sanitized` metric/API names in the SNMP traps code,
  health, integration docs, or Netdata SNMP traps spec surfaces.

### 2026-06-10 Profile Catalogue And Ingestion Hot-Path Batch

Implemented locally:

- Added route metadata (`trap_oids`) to the stock profile `catalogue.json`.
- Changed first-job stock profile routing to build from the catalogue when it
  is available, falling back to full YAML parsing only for older/missing
  catalogues.
- Kept user/operator profiles eagerly loaded and validated at job creation.
- Added support for `catalogue.json.gz` alongside `catalogue.json`, matching the
  install-time compression requirement.
- Added install-time gzip handling for the stock catalogue so packages install
  `catalogue.json.gz` instead of raw JSON.
- Tightened topology trap enrichment so `TRAP_NEIGHBORS` includes only LLDP/CDP
  neighbors on the interface matched by the trap source IP. Management IP
  matches still provide device identity without guessing all device neighbors.
- Strengthened INFORM response tests to prove the response is a GetResponse,
  keeps the request ID/community, echoes varbinds exactly, and is not larger
  than the received request packet.
- Reworked dedup fingerprint construction to avoid per-trap heap allocations.
- Added a reverse-DNS lookup concurrency limit of 32 outstanding lookups when
  reverse DNS is explicitly enabled.
- Added `listen.receive_buffer` with a 4 MiB default applied during listener
  creation. Invalid values are configuration errors; socket-level buffer setup
  failures are startup errors surfaced at job creation.
- Fixed config/spec drift by changing sample and durable spec
  `retention.rotation_duration` defaults from `1h` to `null`.

Validation evidence:

- `go test -count=1 ./cmd/snmptrapprofilegen
  ./plugin/go.d/collector/snmp_traps ./plugin/go.d/collector/snmp_topology`
  passed.
- `go test -run '^$' -bench
  'BenchmarkBuildStockProfileStoreDefaultProfiles|BenchmarkDedupFingerprint|BenchmarkDedupAdmitDuplicate'
  -benchmem ./plugin/go.d/collector/snmp_traps` passed with:
  - stock profile route build: `171368429 ns/op`, `81903810 B/op`,
    `615198 allocs/op`;
  - dedup fingerprint: `299.5 ns/op`, `0 B/op`, `0 allocs/op`;
  - dedup admit duplicate: `366.8 ns/op`, `0 B/op`, `0 allocs/op`.
- `cmake -S . -B /tmp/netdata-snmptraps-cmake-yml-check-no-xen -G Ninja
  -DENABLE_PLUGIN_GO=ON -DENABLE_PLUGIN_XENSTAT=OFF` passed.
- `ninja -C /tmp/netdata-snmptraps-cmake-yml-check-no-xen go.d.plugin
  snmp-trap-profile-gen nd-mcp` passed.
- `cmake --install /tmp/netdata-snmptraps-cmake-yml-check-no-xen --component
  plugin-go --prefix /tmp/netdata-snmptraps-install.[redacted]` passed.
- The temporary component install produced:
  - `usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/catalogue.json.gz`;
  - no raw `catalogue.json`;
  - no raw stock trap profile `.yaml` files under `default/`;
  - 817 installed stock profile `.yaml.gz` files;
  - `catalogue.json.gz` size `541359` bytes.
- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
  passed.
- `git diff --check` completed without warnings.

### 2026-06-10 Function Route Defensive Guard

Implemented locally:

- Rechecked the Function public-name route code after the profile-loading
  batch and found the low-level registry rebuild still used last-writer-wins
  behavior for duplicate public names.
- Changed the registry fallback to deterministic first-owner-wins order by
  sorting module names during route rebuild and preserving an existing route if
  another method claims the same public name.
- Added a focused registry test. The controller-level behavior still rejects
  and logs colliding modules before the fallback matters.

Validation evidence:

- `go test -count=1 ./plugin/agent/jobmgr/funcctl ./plugin/agent/jobmgr`
  passed.
- `go test -count=1 ./cmd/godplugin` passed.
- `git diff --check` completed without warnings.

### 2026-06-10 PR Description And Review Polish

Implemented locally:

- Updated PR #22652 description to remove stale generated text, stale SDK
  version information, and the incorrect "no added footprint" claim.
- Replaced the PR description with current implementation, validation,
  installed-footprint, and existing-code touch-point summaries.
- Corrected user-facing SNMP traps documentation from "OTEL-only" to
  "OTLP-only" where it refers to the configured OTLP exporter backend.
- Added a schema/docs warning that plaintext `http://` / bare OTLP endpoints
  should be limited to local receivers unless trap contents can be sent without
  transport encryption.
- Installed `profile-format.md` with the trap profile data so the shipped
  profile helper has an offline schema reference beside the installed profiles.

Validation evidence:

- `go test -count=1 ./plugin/go.d/collector/snmp_traps` passed after the
  documentation/schema cleanup.
- `python3 integrations/gen_integrations.py` passed.
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/snmp_traps`
  passed.
- `cmake -S . -B /tmp/netdata-snmptraps-cmake-yml-check-no-xen -G Ninja
  -DENABLE_PLUGIN_GO=ON -DENABLE_PLUGIN_XENSTAT=OFF` passed.
- `cmake --install /tmp/netdata-snmptraps-cmake-yml-check-no-xen --component
  plugin-go --prefix /tmp/netdata-snmptraps-install.[redacted]` passed and
  verified:
  - `profile-format.md` is installed;
  - `catalogue.json.gz` is installed;
  - raw `catalogue.json` is not installed;
  - no raw stock `.yaml` files are installed under `default/`.
- `git diff --check` completed without warnings.

### 2026-06-10 Decode/Listener Observability And Final Readiness Pass

Implemented locally:

- Decode failures for received UDP frames now create structured direct-journal
  entries with decode-error fields, packet size, packet hash, listener endpoint,
  source IP, source UDP peer, and source UDP port.
- Decode-error entries omit SNMP communities and packet bytes.
- Listener socket read errors after successful listener creation are counted
  and rate-limited in plugin logs so operators can find receive-loop failures
  without log floods.
- Go 1.26 `go fix ./...` output was applied to the trap profile generator and
  trap profile tests.
- The earlier unrelated `ibm.d/as400` Sonar cleanup was removed from the
  current branch to keep this PR scoped to SNMP traps.

Validation evidence:

- `go test -count=1 ./plugin/go.d/collector/snmp_traps
  ./plugin/go.d/collector/snmp_topology ./plugin/agent/jobmgr/funcctl
  ./cmd/godplugin ./pkg/funcapi` passed.
- `go test -count=1 ./cmd/snmptrapprofilegen` passed.
- `go fix ./...` in `src/go` passed after applying the toolchain diff.
- `git diff --check` completed without warnings.
- Review-risk benchmarks on 2026-06-10:
  - `BenchmarkBuildStockProfileStoreDefaultProfiles-24`:
    `157494319 ns/op`, `81898957 B/op`, `615192 allocs/op`;
  - `BenchmarkDedupFingerprint-24`: `290.1 ns/op`, `0 B/op`,
    `0 allocs/op`;
  - `BenchmarkDedupAdmitDuplicate-24`: `378.9 ns/op`, `0 B/op`,
    `0 allocs/op`.

### 2026-06-10 Installed Runtime Function Availability Fix

Implemented locally:

- DynCfg `test` now injects `SetJobName()` into V2 collectors before applying
  config and calling `Init()`, matching normal job creation.
- Installer/service setup now conditionally creates
  `/var/log/journal/netdata/snmp-traps` for the Netdata plugin user when
  `/var/log/journal` already exists.
- The installer does not create `/var/log/journal` itself, so it does not
  silently enable persistent journald storage on systems that have not enabled
  it.
- `snmp:traps` remains gated on running direct-journal trap jobs; OTLP-only or
  failed jobs do not publish the direct-journal logs Function.

Runtime evidence:

- Installed-agent logs showed `snmp_traps/local` failing at job initialization
  with a permission error while creating `/var/log/journal/netdata`.
- After creating the directory with the running plugin user and restarting
  Netdata, installed-agent logs showed `snmp_traps/local` check success and
  started.
- At that point in the branch, `/api/v1/functions` listed both
  `snmp_traps:reload-profiles` and `snmp:traps`; the reload Function was later
  removed from the visible surface in favor of automatic user-profile reload.
- Direct local execution of `snmp:traps` returned `412` Cloud SSO required
  rather than `404`, proving the Function is registered while preserving the
  Cloud-only access contract.

Validation evidence:

- `go test -count=1 ./plugin/agent/jobmgr ./plugin/agent/jobmgr/funcctl
  ./cmd/godplugin ./plugin/go.d/collector/snmp_traps` passed.
- `sh -n packaging/installer/functions.sh
  packaging/cmake/pkg-files/deb/plugin-go/postinst
  packaging/makeself/install-or-update.sh` passed.
- `git diff --check` completed without warnings.

### 2026-06-10 User Profile Auto-Reload And Function Surface Cleanup

Implemented locally:

- Removed the visible `snmp_traps:reload-profiles` Function from the published
  `snmp_traps` method list. The only public SNMP traps Function in this area is
  the direct-journal logs Function `snmp:traps`.
- Added an internal user-profile watcher that starts when the first trap job
  acquires the shared profile cache and stops when the last job releases it.
- The watcher fingerprints only user-supplied profile directories. Stock
  profile directories and stock catalogue files are not watched, per user
  decision, because Netdata upgrades may update stock profiles together with the
  code that interprets them.
- Automatic watcher reloads use a user-only reload path that preserves the
  existing stock route/store until all trap jobs stop or the process restarts.
  User profiles that replace a stock filename still remove the corresponding
  carried-over stock routes.
- The watcher uses fsnotify plus a periodic fingerprint fallback. Fingerprints
  include profile filename, size, mtime, and mode, and include underscore-prefixed
  profile files because they can be used by `extends`.
- On user-profile changes, the watcher marks the cache dirty, attempts a
  background reload, and atomically swaps in the new profile index only on
  success.
- If reload fails, existing jobs keep the previous valid index and
  profile-load-failure metrics increment. The dirty flag remains set so the next
  DynCfg test/apply or job creation synchronously validates the changed user
  profiles and fails until the profile files are fixed.
- If user-profile fingerprinting itself fails, the watcher marks the cache dirty
  and attempts a reload so readability/permission failures are surfaced on the
  next DynCfg test/apply instead of leaving the current cache silently valid.
- Updated operator docs and the SNMP traps query skill how-to to describe
  automatic user-profile reload instead of a manual Function call.

Validation evidence:

- `go test -count=1 ./plugin/go.d/collector/snmp_traps
  ./plugin/agent/jobmgr/funcctl ./cmd/godplugin ./plugin/agent/jobmgr` passed.

### 2026-06-10 External Review Hardening Of User Profile Auto-Reload

Implemented locally after the post-commit external review pass:

- `profileWatcher.Start()` now keeps the periodic fingerprint loop running even
  when `fsnotify.NewWatcher()` fails. The failure is logged, but automatic
  operator-profile reload degrades to periodic scans instead of silently
  disappearing.
- The watcher now advances `lastFingerprint` only after a successful reload.
  Invalid changed profiles therefore remain retryable on the next periodic scan
  or event, and the dirty flag continues to force job-creation validation.
- The watcher no longer watches the parent go.d config directory. It watches
  only existing operator profile directories and relies on the periodic
  fingerprint fallback to notice a newly-created operator profile directory.
- Remove/rename events now clear matching entries from the internal watch map so
  deleted-and-recreated profile directories can be watched again.
- The dead `shouldRefreshForEvent()` parent/base comparison was removed because
  it was equivalent to `path == dir` and added maintenance risk without
  behavior.
- Fingerprinting now uses `fs.DirEntry.Info()` from the `WalkDir` entry instead
  of a separate `os.Stat()` per profile file.
- Added tests that start the real watcher run loop, exercise the periodic
  reload path, prove fallback behavior when fsnotify initialization fails,
  prove failed reloads do not advance the fingerprint, and cover watch-map
  cleanup for removed directories.
- Updated durable specs:
  - `.agents/sow/specs/snmp-traps/netdata.md`
  - `.agents/sow/specs/snmp-traps/comparison/comparative-analysis.md`

Rejected or deferred findings:

- User profile duplicate-OID override by a different filename was not changed.
  Current documented contract is same-filename replacement or explicit
  `extends:` merge. Different filename additions that duplicate existing stock
  OIDs remain invalid, matching initial-load behavior.
- Rebuilding stock enterprise-prefix routes after same-filename replacement was
  deferred because the current generated stock catalogue routes exact OIDs, the
  replacement contract is same-filename replacement, and no failing branch test
  proved a shipped catalogue route loss. Revisit only if a real catalogue
  contains multi-file enterprise-prefix routes that survive replacement.
- A fsnotify close-channel drain helper was not added. In this implementation
  the run goroutine owns `Close()`, `Stop()` cancels and waits for `done`, and
  fsnotify v1.10.1 closes its event/error channels internally from the backend
  reader. The existing code avoids concurrent `Close()` from another goroutine.

Validation evidence:

- `go test -count=1 ./plugin/go.d/collector/snmp_traps` passed.
- `go test -count=1 ./plugin/go.d/collector/snmp_traps
  ./plugin/agent/jobmgr/funcctl ./cmd/godplugin ./plugin/agent/jobmgr` passed.
- `rg -n "reload-profiles|snmp_traps:reload" .agents/sow/specs src/go docs
  integrations` now returns only the explicit superseded/rejected manual
  Function note in the durable spec.

### 2026-06-10 External Review Rerun Fixes

Implemented locally after the full-scope reviewer rerun:

- Removed `TRAP_JSON` from `SYSTEMD_KEYS_INCLUDED_IN_FACETS` in
  `src/collectors/systemd-journal.plugin/systemd-journal.c`, matching the
  earlier recorded decision that `TRAP_JSON` is high-cardinality payload data
  and should not be a default C-side facet.
- Made the direct `godplugin` Function CLI public-name resolver deterministic
  by sorting module names before scanning. This matches the Function controller
  collision behavior and prevents CLI/daemon disagreement if a future module
  claims the same public Function name.
- Added a CLI collision regression test proving sorted first-owner behavior.
- Removed the lingering unrelated `src/go/plugin/ibm.d/modules/as400` diff from
  the branch.

Rejected or stale rerun findings:

- The persistent-journal-root startup error classification is already
  retryable `503` in current code:
  `src/go/plugin/go.d/collector/snmp_traps/collector.go` calls
  `dyncfgStartupError()` for `validatePersistentJournalRoot()`.
- The systemd v235 service template does not define any `CapabilityBoundingSet`
  entries and runs the daemon as root, so the specific "missing
  CAP_NET_BIND_SERVICE from v235 bounding set" finding does not apply to that
  template. The modern service template does include the capability.
- RPM-native postinstall journal-directory creation is outside this repository's
  packaging tree. This branch still creates the directory in the native
  installer, Debian plugin package postinst, makeself path, and systemd service
  `ExecStartPre`; any separate RPM spec follow-up belongs in the packaging
  repository if maintainers require package-install-time creation before service
  start.
- The claimed fsnotify add-watch leak on `os.Stat` failure is not reachable in
  the cited code path because `watcher.Add()` is called only after successful
  `os.Stat()` and directory validation.

Validation evidence:

- `go test -count=1 ./cmd/godplugin ./plugin/agent/jobmgr/funcctl
  ./plugin/agent/jobmgr ./plugin/go.d/collector/snmp_traps ./pkg/funcapi
  ./plugin/ibm.d/modules/as400` passed.
- `git diff --check` completed without warnings.
- `git diff upstream/master...HEAD --
  src/go/plugin/ibm.d/modules/as400/helpers.go` is expected to be empty after
  this fix is committed.

Closed review nits:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/metadata/.keep` is tracked
  but the trap profile loader and CMake install rules do not use the metadata
  directory. Cleanest disposition is to delete it, but file deletion requires
  explicit approval.
  - Decision: user approved removal on 2026-06-10.
  - Status: closed. The placeholder is no longer present in the current branch.
- `src/health/health.d/snmp_trap.conf` is singular while the module is
  `snmp_traps`. Current rationale for keeping it is that chart contexts and
  alert names are `snmp.trap.*` / `snmp_trap_*`; renaming would require updating
  generated health links and is not proven to fix behavior. Rename requires
  explicit approval.
  - Decision: user approved renaming to `snmp_traps.conf` on 2026-06-10,
    because this PR has not merged yet and no released file path is affected.
  - Status: closed. The current branch contains `snmp_traps.conf`.
- SELinux behavior for UDP/162 remains a deployment-environment validation
  item. No SELinux policy files were found in the changed packaging/service
  paths; this needs validation on an enforcing SELinux distro or review by the
  packaging maintainers.
  - Decision: user requested an implementation analysis for what would be
    required to ship a Netdata SELinux policy.
  - Repo evidence:
    - No committed SELinux policy source files (`*.te`, `*.fc`, `*.if`, `*.cil`)
      were found in this repository.
    - The only package-side SELinux action found is the updater's narrow
      `restorecon /dev/null` repair.
    - The eBPF collector currently documents an operator-created local SELinux
      module instead of shipping a Netdata policy package.
  - External packaging evidence:
    - Fedora's SELinux policy module packaging guidance describes shipping a
      loadable policy either in the main package or in a separate `-selinux`
      subpackage, with build/runtime dependencies, installation scriptlets, and
      context repair through `fixfiles`/`restorecon`.
    - Red Hat's custom policy guidance requires a policy development
      environment such as `selinux-policy-devel` and validates process domains,
      file contexts, AVC processing, deployment, and packaging/distribution.
    - Red Hat's SELinux docs show that service ports are governed by SELinux
      port labels; an unlabeled or wrong-type port can fail with `name_bind`,
      and `semanage port -a -t <type> -p <protocol> <port>` is the normal local
      administration path.
  - Implementation required to ship policy:
    - Define a Netdata SELinux policy module source tree, including process
      domain assumptions for `netdata` and `go.d.plugin`.
    - Define file contexts for Netdata binaries, plugin paths, trap journal
      directories, trap state directories, stock profile data, and user-supplied
      profile/config directories.
    - Define or reuse the correct UDP/162 port type and install the required
      port mapping on SELinux systems.
    - Add RPM packaging hooks or a `netdata-selinux` subpackage to build,
      install, upgrade, remove, and relabel policy safely.
    - Decide what Debian/static installers should do, because SELinux policy
      packaging is distribution-specific.
    - Validate on enforcing Fedora/RHEL-family systems with UDP/162 enabled,
      journal writes active, profile loading active, and no relevant AVC
      denials.
  - Recommendation:
    - Treat shipped SELinux policy as a separate packaging/security SOW and PR
      unless maintainers require it as a blocker for this PR. This SNMP traps
      PR already updates capability handling for UDP/162; policy packaging would
      introduce a new cross-distro security surface outside the collector.

Still pending:

- Expected SOW merge guard remains until active SOW working files are deleted
  before merge.
- Real installation/lab validation remains a product validation step outside
  this code-readiness pass.

External reviewer final pass findings accepted for this patch:

- Qwen found `readMaybeGzipFile()` used unbounded `io.ReadAll()` for profile
  YAML and stock catalogue files. This is a local-file write/access risk rather
  than a network-triggered issue, but a malformed `.yaml.gz` or
  `catalogue.json.gz` should not be able to expand without a hard cap inside the
  agent. Plan: cap decompressed reads and return an explicit error when the cap
  is exceeded.
- Qwen and Minimax both found a profile-template side path where a custom
  profile could explicitly render the synthetic SNMP community varbind into
  `MESSAGE`, labels/`TRAP_TAG_*`, or OTLP body fields. Existing TRAP_JSON and
  OTLP varbind serialization redaction is correct, and stock profiles do not
  reference the community in templates, but profile rendering should also
  redact sensitive varbinds. Plan: redact sensitive varbinds in both legacy
  `{var}` templates and Go `{{value "var"}}` / `{{raw "var"}}` templates.
- Qwen found the DynCfg `test` command path hardcoded `422` for all `Init()` and
  `Check()` failures. Runtime apply/update already preserves coded errors.
  Plan: preserve `dyncfg.CodedError.Code()` in `test` responses too, falling
  back to `422` for plain errors.
- GLM found that `dedup_key_varbinds` could explicitly reference the synthetic
  SNMP community varbind. This did not expose the value in plaintext, but the
  value could influence an emitted dedup fingerprint. Plan: preserve the
  varbind's presence/OID/type in the fingerprint shape while replacing the
  sensitive value with the shared redaction token before hashing.

Rejected or deferred final-pass findings:

- Active SOW files remain an expected draft-branch merge guard and will be
  removed during final merge preparation after durable memory transfer.
- Health/chart naming singular-vs-plural is a product naming consistency item,
  not a runtime correctness bug. The user already approved the file rename to
  `snmp_traps.conf`; chart contexts remain `snmp.trap.*`.
- Engine state path `snmp-trap` vs journal path `snmp-traps` is pre-release and
  harmless, but changing it now would touch state-path semantics. Leave unless a
  maintainer explicitly requests a rename.
- C-side `TRAP_TAG_*` unit coverage and SELinux policy packaging remain
  follow-up candidates unless maintainers make them merge blockers.

## Artifact Maintenance Gate

- Specs, generated integration docs, and the operator skill have already been
  updated in the changed branch files.
- Branch-local SOW files remain active takeover memory and must be removed only
  during final merge preparation.
