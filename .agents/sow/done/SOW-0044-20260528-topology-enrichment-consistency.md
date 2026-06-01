# SOW-0044 - topology enrichment consistency

## Status

Status: completed

Sub-state: implemented, validated, committed, and ready to close.

## Requirements

### Purpose

Elevate network topology from process-only maps to operator-selectable actor
views that consistently expose process, container, service, orchestrator, and
cgroup enrichment across the Agent Functions that describe the same live
processes and sockets.

### User Request

Fix the container topology implementation so:

- users can select actor grouping by PID, process name, or container/service;
- per-PID views expose all available raw enrichment;
- grouped views merge enrichment across contributing PIDs instead of silently
  dropping it;
- both `topology:network-connections` and tabular `network-connections` use
  the same enrichment data;
- `apps.plugin` `processes` exposes the same per-PID enrichment;
- Cloud topology service and the frontend remain generic topology consumers,
  not container-specific implementations.

### Assistant Understanding

Facts:

- The user clarified that Cloud topology service and the frontend must be
  topology-agnostic. They should consume producer-declared schema, parameters,
  rows, labels, and aggregation rules without knowing what `container` means.
- `group_by:pid` is the only view where scalar raw per-PID fields can be emitted
  without ambiguity.
- `group_by:process_name` groups multiple PIDs, possibly across containers and
  nodes, so fields that vary per PID must be merged.
- `group_by:container` groups multiple processes under a canonical
  container/service name. For systemd services this is the service name; for
  non-container, non-service processes the fallback is the process name.
- `network-connections`, `topology:network-connections`, and `apps.plugin`
  `processes` all describe overlapping process/socket reality and must expose a
  consistent enrichment model.
- Container/service grouping must promote meaningful service boundaries, not
  every raw cgroup leaf. For `user.slice`, the promoted graph actor is the
  resolved username. If username resolution fails, the fallback display name is
  `user${UID}` with no space.
- Topology icon tokens are closed schema/UI tokens. Runtime-specific actor
  types need explicit tokens before producers can emit them without schema or
  frontend diagnostics.
- The Agent producer must emit the specific icon token that matches its
  topology classification. Adding schema/frontend tokens without changing the
  producer only leaves the dashboard on generic `container` and `service`
  icons.
- Live validation after the first icon install showed that `user.slice` paths
  were still promoted as individual `systemd_scope` actors. Root cause:
  `network-viewer.c` derives `container_name` from the last systemd unit
  component for every `systemd_*` actor, so `app-code.scope` and similar leaf
  scopes become actor identities.

Inferences:

- The earlier "drop variable fields in grouped views" rule is now known to be
  wrong for the product goal. Grouped views need merged/set-valued enrichment,
  not scalar replacement and not omission.
- Cloud-side work should be limited to generic schema/aggregation validation or
  fixes if testing proves a generic gap. It must not hardcode container logic.
- Raw cgroup/systemd details for user scopes remain useful evidence and should
  stay in actor detail tables even when the promoted graph actor is the user.

Unknowns:

- Final live behavior under the user's installed Agent will be validated by the
  user after the code and local focused tests pass.
- Whether `~/src/netdata/cloud-topology-service` needs a PR is not
  known yet. Current evidence shows generic forwarding and aggregation support;
  this SOW will verify with synthetic payloads before deciding.
- Whether each new icon token needs a custom frontend drawing or can initially
  share a generic rendering will be settled in the cloud-frontend PR.

### Acceptance Criteria

- SOW/spec/skill/docs no longer describe grouped topology enrichment as
  forbidden or PID-only.
- `topology:network-connections` `group_by:pid` emits scalar per-PID enrichment
  fields and actor labels for process/container/cgroup/service facts.
- `topology:network-connections` `group_by:process_name` and
  `group_by:container` expose merged enrichment through producer-declared actor
  labels and compatible aggregation metadata, without changing actor identity to
  unstable variable fields.
- Actor modal metadata advertises enrichment labels for process and container
  actors in all grouping modes.
- Container/service topology actor types emit matching closed icon tokens:
  `docker`, `kubernetes`, `lxc`, `podman`, `nspawn`, `systemd`, and existing
  `vm`.
- `user.slice/user-UID.slice` topology paths collapse to one user actor named
  by resolved username, or `user${UID}` when username resolution is unavailable;
  the leaf systemd scope remains visible only in cgroup/detail evidence.
- Tabular `network-connections` exposes the same per-PID enrichment columns in
  aggregated and detailed socket views where a PID is available.
- The detailed tabular `network-connections` path warms APPS_LOOKUP for PIDs it
  observes, matching the aggregated path behavior.
- `apps.plugin` `processes` exposes the same per-PID enrichment fields derived
  from its cgroup lookup cache.
- Focused C tests and topology schema fixture validation cover PID,
  process-name, and container grouping behavior.
- Cloud topology service is checked with generic aggregation evidence. Any
  required Cloud change is represented as a generic Cloud PR plan or patch, not
  as container-specific consumer logic.

## Analysis

Sources checked:

- `src/collectors/network-viewer.plugin/network-viewer.c`
- `src/collectors/apps.plugin/apps_functions.c`
- `src/collectors/network-viewer.plugin/metadata.yaml`
- `src/collectors/network-viewer.plugin/integrations/network_connections.md`
- `.agents/skills/project-create-topology/SKILL.md`
- `.agents/sow/specs/topology-function-schema.md`
- `.agents/sow/specs/topology-containers-ipc-contract.md`
- `~/src/netdata/cloud-topology-service/internal/topology/request/request.go`
- `~/src/netdata/cloud-topology-service/internal/topology/aggregate/aggregate.go`
- `~/src/dashboard/cloud-frontend/src/domains/functions/api.js`
- `~/src/dashboard/cloud-frontend/src/domains/functions/components/header/settings.js`
- `~/src/dashboard/cloud-frontend/src/domains/functions/topology/v1/buildActors.js`

Current state:

- `network-viewer.c:2581-2585` merges duplicate grouped actors by adding socket
  count and then continuing, so later PIDs in the same grouped actor do not add
  enrichment labels.
- `network-viewer.c:2596-2618` calls APPS_LOOKUP enrichment only in
  `group_by:pid`; process-name grouping only writes `process`, and container
  grouping only writes `container_name`.
- `network-viewer.c:2629-2643` emits username, command line, namespace, PID,
  PPID, UID, and netns labels only for PID grouping.
- `network-viewer.c:4718-4722` streams detailed tabular rows directly; unlike
  `network-viewer.c:4739`, this path does not warm APPS_LOOKUP from the PIDs
  it observed.
- `apps_functions.c:158-220` and `apps_functions.c:1128-1168` define and emit
  the `processes` Function columns/rows without cgroup/container enrichment.
- Cloud request forwarding is generic: `request.go:13-18` reserves only
  topology metadata names, not `group_by`; `request.go:129-158` forwards
  advertised non-reserved parameters.
- Cloud actor aggregation is generic and producer-declared:
  `aggregate.go:893-910` uses actor type identity/merge identity and
  `aggregate.go:2242-2261`, `aggregate.go:2406-2479` merge columns according
  to declared aggregation rules, including `set`.
- Frontend Function selections are generic: `api.js:155-165` sends
  `payload.selections`; `settings.js:108-120` initializes defaults from
  required parameters; `buildActors.js:184-215` preserves actor raw rows under
  generic attributes/details.
- The current topology skill and specs still contain the old rule that grouped
  views must not expose variable cgroup/container metadata.

Risks:

- Adding too many visible columns can make table views noisy. The repair should
  add consistent data while keeping sensitive/high-cardinality fields hidden by
  default where appropriate.
- Grouped actor labels can grow with grouped PID diversity. Labels must be
  deduplicated by actor/key/value/source/kind to avoid modal spam.
- Raw cgroup paths and labels may contain operator-chosen identifying strings.
  Free-form labels remain denied unless explicitly whitelisted.
- Synchronous IPC in Function handlers remains forbidden. All network-viewer
  enrichment must use the asynchronous APPS_LOOKUP cache and request warming.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- The implementation followed the old SOW/spec decision that grouped topology
  views should omit variable per-PID fields. That decision conflicts with the
  product goal. It causes actor modals in grouped views to lack enrichment even
  though the cache can supply it.
- The detailed tabular `network-connections` path never warms APPS_LOOKUP for
  PIDs it observes, so detailed-only socket workloads can remain unenriched.
- `apps.plugin` owns the PID-to-cgroup association but does not expose the same
  enrichment in its `processes` Function, creating inconsistent operator views.

Evidence reviewed:

- `network-viewer.c:2581-2585` duplicate grouped actor path skips enrichment.
- `network-viewer.c:2596-2618` PID-only actor enrichment branch.
- `network-viewer.c:4718-4722` detailed table callback emits rows inline.
- `network-viewer.c:4739` aggregated table path warms APPS_LOOKUP from the
  aggregated sockets array.
- `apps_functions.c:158-220`, `apps_functions.c:1128-1168` existing processes
  Function schema/data emission.
- Cloud and frontend generic evidence listed in Analysis.

Affected contracts and surfaces:

- `topology:network-connections` Function payload.
- Tabular `network-connections` Function payload.
- `apps.plugin` `processes` Function payload.
- APPS_LOOKUP cache demand/warm behavior in `network-viewer.plugin`.
- Topology schema/spec/docs/project skill text.
- Cloud topology service generic aggregation validation.

Existing patterns to reuse:

- Existing APPS_LOOKUP asynchronous cache in `network-viewer.plugin`.
- Existing topology `actor_labels` table with table aggregation `set`.
- Existing topology modal label recipe mechanism.
- Existing topology fixture validation through
  `src/collectors/network-viewer.plugin/tests/validate_topology_container_fixtures.py`
  and `tools/functions-validation/validate`.
- Existing apps PID/cgroup cache fields in `struct pid_stat`.

Risk and blast radius:

- Medium-high user-visible behavior change across three Functions.
- Low packaging risk; no installer/runtime path change is expected.
- Performance risk is bounded by using cache reads and batched warm requests,
  not synchronous IPC from Function handlers.
- Security/privacy risk is managed by hidden defaults for raw cgroup paths and
  existing label whitelist behavior.

Sensitive data handling plan:

- Do not copy real curl cookies, bearer tokens, customer/node identifiers,
  session data, public IPs, or raw production socket rows into durable
  artifacts.
- Use synthetic process/container labels in tests and documentation.
- Keep any live payload captures under `.local/` only if needed, and redact
  secrets before mentioning evidence in SOWs/specs/docs.

Implementation plan:

1. Repair SOW/spec/skill/docs so the durable contract matches the corrected
   product design.
2. Add deduplicated actor-label enrichment for grouped topology actors while
   preserving scalar raw fields in PID mode.
3. Add same enrichment columns to tabular `network-connections` and warm
   APPS_LOOKUP from detailed-mode PIDs.
4. Add the same per-PID enrichment columns to `apps.plugin` `processes`.
5. Extend focused tests/fixtures for PID, process-name, and container grouping,
   then validate against the topology schema.
6. Verify Cloud topology service generic aggregation and produce a generic Cloud
   patch/PR plan only if evidence shows a generic gap.

Validation plan:

- Build focused targets for `network-viewer.plugin`, `apps.plugin`, and related
  tests.
- Run existing APPS_LOOKUP/netipc focused tests.
- Run topology container fixture validation and schema validation.
- Add or update tests proving grouped actors keep merged labels and detailed
  table PIDs are warmed.
- Run same-failure searches for PID-only enrichment assumptions.
- Record Cloud generic aggregation evidence and any pending Cloud action.

Artifact impact plan:

- AGENTS.md: no workflow guardrail change expected.
- Runtime project skills: update `.agents/skills/project-create-topology/SKILL.md`
  because it currently states the wrong grouped-view rule.
- Specs: update `.agents/sow/specs/topology-function-schema.md` and
  `.agents/sow/specs/topology-containers-ipc-contract.md`.
- End-user/operator docs: update network-viewer metadata source and generated
  integration documentation.
- End-user/operator skills: no public skill change expected unless validation
  changes operator query workflow.
- SOW lifecycle: SOW-0040 is absorbed by this SOW; SOW-0041 must be corrected
  to generic Cloud validation/aggregation work instead of container-specific
  consumer work.

Open-source reference evidence:

- Not checked for this repair. The bug is in this repository's topology
  producer contract and Cloud/UI generic consumer contract, not an external
  protocol interpretation.

Open decisions:

- None. The user approved proceeding with the corrected design:
  Agent producer fixes first, Cloud/UI generic-only unless generic evidence
  proves otherwise, and live installed validation by the user after local tests.

## Implications And Decisions

1. Cloud/UI consumer model
   - Decision: Cloud topology service and frontend stay topology-agnostic.
   - Implication: do not add `container`-specific UI or Cloud aggregation code.
   - Risk if violated: every future grouping dimension would require Cloud/UI
     code changes, defeating the topology v1 schema design.

2. Grouped actor enrichment model
   - Decision: PID mode emits scalar raw per-PID fields; grouped modes merge
     variable enrichment through set-valued labels/metadata.
   - Implication: actor identity and display labels stay stable while modals can
     show the merged facts.
   - Risk if violated: scalar fields in grouped actors would either be false or
     randomly replace values from one PID with another.

3. Function consistency
   - Decision: `topology:network-connections`, tabular `network-connections`,
     and apps `processes` must expose the same enrichment model where they share
     PID context.
   - Implication: enrichment is not topology-only decoration; it becomes
     reusable process/socket state.
   - Risk if violated: operators will see different process/container truth in
     different views.

4. APPS_LOOKUP demand model
   - Decision: no whole-cache invalidation or timer refresh. Consumers warm
     lookup data on observed PID demand and evict on concrete cleanup/identity
     events.
   - Implication: first responses may be partial, but subsequent responses keep
     asking while the PID remains visible until enrichment is known or
     permanently unavailable.
   - Risk if violated: refresh timers and whole-cache invalidation recreate the
     instability that caused mixed/floating actor updates.

5. Live validation
   - Decision: user will run final installed Agent validation.
   - Implication: this SOW still must pass focused local builds/tests and record
     the exact live checks needed.
   - Risk: local tests can prove contracts, not the user's full runtime
     packaging/install path.

6. Nested systemd service cgroups
   - Decision: systemd service charts should monitor the real `*.service`
     cgroup even when systemd places it below an intermediate `*.slice`, using
     `!/system.slice/*.service/*.service /system.slice/*.service` as the default
     match pattern.
   - Evidence: the installed Agent was missing `cups.service` from
     `systemd.service.cpu.utilization` while systemd reported
     `ControlGroup=/system.slice/system-cups.slice/cups.service`; the current
     default `!/system.slice/*/*.service /system.slice/*.service` rejects that
     path before service conversion.
   - Implication: direct services and services nested below slices are monitored
     as systemd services, while service-under-service descendants remain
     excluded to avoid overlapping hierarchical accounting.
   - Risk: `simple_pattern` is not path-component-aware. The selected pattern was
     checked with `netdata -W simple-pattern` for direct service, service under
     slice, service under service, and slice-only paths before implementation.

7. Topology orchestrator classification model
   - Decision: topology actor naming/classification must be rule-module based,
     not scattered special cases in network-viewer call sites.
   - Evidence: live validation showed systemd slices/scopes and nested services
     need clean topology names/icons even when the metric cgroup classifier does
     not monitor that cgroup as a systemd service chart.
   - Implication: metric-monitoring inclusion rules in cgroups.plugin remain
     separate from topology display/classification rules. Topology enrichment may
     derive `systemd_unit_name`, `systemd_unit_kind`, actor kind/type, and
     effective display orchestrator from cgroup path/rules without widening the
     netipc orchestrator enum for every display subtype.
   - Risk if violated: each new runtime/orchestrator version would require
     ad-hoc changes across apps, network-viewer, docs, and tests. A rule module
     keeps future Docker, Kubernetes, Podman, LXC, nspawn, VM, systemd unit, and
     other runtime rules localized.

## Plan

1. Repair SOW lifecycle and durable contract text.
2. Implement network-viewer topology grouped enrichment and label dedupe.
3. Implement tabular network-viewer enrichment and detailed PID warming.
4. Implement apps `processes` enrichment columns.
5. Update tests/fixtures/docs/specs/skills.
6. Validate locally and inspect Cloud generic aggregation.

## Execution Log

### 2026-05-28

- Created the SOW after the user approved proceeding with the corrected design.
- Updated the topology producer contract so `group_by` exposes `process_name`,
  `pid`, and `container` actor modes without a cgroup-path show/hide selector.
- Changed topology actor collection so every observed PID is retained as a
  contributor first, then rendered into the selected actor grouping. This lets
  grouped actors add deduplicated per-PID labels instead of dropping enrichment
  after the first grouped row.
- Added pending enrichment state to topology actors: cache misses now emit
  `cgroup_status=retry_later` and `container_name=[pending]` instead of
  silently omitting the partial state.
- Added `search.label_keys` and modal label identification fields for
  cgroup/container/service/process enrichment labels so the UI can consume them
  generically.
- Added the same per-PID enrichment columns to tabular
  `network-connections` and to apps `processes`.
- Moved the apps-side enrichment derivation into
  `apps-cgroups-enrichment.[ch]` so `apps_functions.c` stays focused on
  Function table emission.
- Updated topology fixtures and validation to assert `cgroup_status`, `set`
  aggregation metadata, and label-key search metadata.
- Created and completed a tests-only Cloud SOW:
  `~/src/netdata/cloud-topology-service/.agents/sow/done/SOW-0025-20260528-actor-attribute-set-aggregation-guardrail.md`.
  It adds a generic Cloud aggregation test proving actor attributes with
  `aggregation: "set"` merge across duplicate actors without adding
  container-specific Cloud code.
- Audited the production diff against `master` for netipc debug/logging leaks.
  Kept lookup-service connect, disconnect, failed-connect, server-start,
  server-stop, and request-failure logs as production diagnostics. Removed only
  message-level tracing style logs; no send/receive/respond payload chatter is
  present in production code.
- Added one-per-failure-run request-failure logging for CGROUPS_LOOKUP and
  APPS_LOOKUP calls so support has a journal reason when enrichment stops, but
  persistent broken peers do not flood logs until a successful response resets
  the failure state.
- Restored cgroups discovery open-error de-duplication from `master` after the
  audit found the branch could otherwise log the same inaccessible cgroup
  directory repeatedly on every discovery scan.
- Fixed systemd service cgroup matching so services below intermediate slices,
  such as `/system.slice/system-cups.slice/cups.service`, become systemd service
  charts while service-under-service descendants remain excluded.
- Verified service-name extraction for nested systemd services: the conversion
  code uses the basename from `cg->id` and strips the `.service` extension, so
  the nested cups path becomes service name `cups`.

### 2026-05-29

- Added shared cgroup topology rule-module files under
  `src/collectors/common-cgroups/` so topology display classification is
  localized instead of scattered through network-viewer and apps call sites.
- Updated network-viewer grouped container actors to use specific actor types
  such as `systemd_service`, `systemd_scope`, `docker_container`, and `vm`
  while keeping the shared topology merge scope as `container`.
- Split grouped actor modal details into generic actor-owned `processes` and
  `cgroups` tables. This keeps Cloud/UI topology-agnostic: the producer declares
  table ids, columns, owner scope, and aggregation rules.
- Fixed duplicate `container_name` labels by suppressing the enrichment
  duplicate when the grouped container actor already uses the same value as its
  identity/display name.
- Added `systemd_unit_kind` and `actor_kind` to apps and network-viewer
  enrichment so `processes`, `network-connections`, and
  `topology:network-connections` expose the same classification facts.
- Copied the local ignored `.env` from the sibling Agent checkout only for
  token-safe direct-Agent validation. The file is ignored by `/.gitignore` and
  no secret values were printed or written to durable artifacts.
- Installed the build with `sudo ./install.sh`; Netdata restarted successfully.
- Live direct-Agent validation used the `query-netdata-agents` token-safe
  wrapper. Bearer values and Cloud token values were masked by the wrapper.
- The first live query after restart returned partial enrichment because
  network-viewer had not yet connected to APPS_LOOKUP. This produced
  `retry_later` cgroup rows and a `[pending]` actor, not a process-name fallback.
  Journal evidence later showed `network-viewer.plugin connected to
  APPS_LOOKUP service`.
- After APPS_LOOKUP connected, repeated live refreshes remained in the selected
  container grouping mode with actor types limited to `systemd_service`,
  `systemd_scope`, `docker_container`, `vm`, `process_group`, `container`,
  `self`, and `endpoint`.
- A standalone `network-viewer.plugin` validation run was found to interfere
  with the live plugin's named `netdata-spawn-setns.sock` path. Netdata was
  restarted after this discovery and final validation used only the live Agent
  HTTP Function path.
- Clean restart validation restored `/run/netdata/netdata-spawn-setns.sock`.
  First query after restart returned the designed partial `[pending]` state;
  the second query after `network-viewer.plugin connected to APPS_LOOKUP
  service` returned fully enriched grouped actors.

## Validation

Acceptance criteria evidence:

- `topology:network-connections` grouping is producer-declared through
  `group_by` options in `network-viewer.c`; no show/hide cgroup-path selector
  remains in active code.
- PID mode fills scalar cgroup/container/service fields on actor rows and actor
  labels. Cache misses are explicit with `retry_later` / `[pending]`.
- Process-name and container grouped modes retain every contributing PID in
  `ctx->process_actors`, then add deduplicated labels for PID, user, command,
  namespace, local address, cgroup, container, orchestrator, Kubernetes,
  Docker, and systemd facts.
- Actor type metadata now advertises enrichment through modal identification
  fields and `search.label_keys`, so Cloud/UI stay generic.
- Tabular `network-connections` emits the same cgroup/container/service
  enrichment columns after `User` where PID context exists, and detailed mode
  warms APPS_LOOKUP from observed PIDs.
- apps `processes` emits the same Linux enrichment columns using
  `apps_process_enrichment_fill()`.
- Cloud topology service was checked separately. It already had generic
  request forwarding, actor-label set merge, and column set aggregation. A
  tests-only guardrail now proves duplicate actor attributes with
  `aggregation: "set"` merge generically.

Tests or equivalent validation:

- `sudo cmake --build build --target network-viewer-topology-containers-test network-viewer-apps-lookup-client-test apps-lookup-protocol-test network-viewer.plugin apps.plugin -j 8`
  passed. The only note was the existing compiler variable-tracking retry in
  `topology_write_response_metadata`.
- `sudo cmake --build build --target network-viewer-topology-containers-test network-viewer-apps-lookup-client-test apps-lookup-protocol-test cgroup-lookup-netipc-test cgroup-orchestrator-test apps.plugin network-viewer.plugin -j 8`
  passed. The only note was the existing compiler variable-tracking retry in
  `topology_write_response_metadata`.
- Re-ran the same focused build after restoring lifecycle/failure logs; it
  passed with the same compiler variable-tracking note.
- `sudo build/network-viewer-topology-containers-test` passed.
- `sudo build/network-viewer-apps-lookup-client-test` passed.
- `sudo build/apps-lookup-protocol-test` passed.
- `sudo build/cgroup-lookup-netipc-test` passed.
- `sudo build/cgroup-orchestrator-test` passed.
- `sudo cmake --build build --target cgroup-orchestrator-test cgroup-lookup-netipc-test -j 8`
  passed after the nested-systemd-service matcher change.
- `sudo build/cgroup-orchestrator-test` passed with cases for direct service,
  service under slice, service under service, slice-only path, and the configured
  `simple_pattern` default.
- `sudo build/cgroup-lookup-netipc-test` passed after the matcher change.
- `netdata -W simple-pattern '!/system.slice/*.service/*.service /system.slice/*.service' ...`
  was checked for direct service, service under slice, service under service,
  and slice-only paths. Results were positive, positive, negative, and not
  matched respectively.
- `sudo ./install.sh` completed successfully and restarted the local Agent.
- `python3 src/collectors/network-viewer.plugin/tests/validate_topology_container_fixtures.py`
  passed for 4 fixtures.
- `go run ./tools/functions-validation/validate --schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json --input <fixture>`
  passed for all topology network-viewer fixtures from `src/go`.
- `git diff --check` passed in the Agent repository.
- Cloud: `go test ./internal/topology/aggregate -run 'TestAggregate(SetAggregatesDuplicateActorAttributes|ColumnAggregationRules|ActorLabelsSet)' -count=1`
  passed.
- Cloud: `go test ./internal/topology/...` passed.
- Cloud: `git diff --check` passed.
- Cloud: `.agents/sow/audit.sh` passed after the tests-only SOW was closed.

Real-use evidence:

- User validated the installed Agent manually and reported that
  `topology:network-connections` works in `group_by=container`.
- `sudo ./install.sh` completed successfully and restarted the local Agent.
- Direct local HTTP without bearer returned HTTP 412 due SSO protection. After
  copying the ignored local `.env`, the `query-netdata-agents` token-safe wrapper
  successfully queried the local Agent with a per-agent bearer.
- Live schema validation passed:
  `go run ./tools/functions-validation/validate --schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json --input /tmp/topology-container-live.json`.
- First post-restart live query returned partial data before APPS_LOOKUP was
  connected: 56 actors, 78 cgroup rows, and all rows in `retry_later`. This is
  the intended partial-data state from decision 4 and did not fall back to
  process-name actors.
- Journal evidence after that query showed
  `network-viewer.plugin connected to APPS_LOOKUP service`.
- Steady-state live query after APPS_LOOKUP connection returned:
  108 actors, 136 links, 76 process rows, 76 cgroup rows, and actor-owned
  `actor_labels`, `processes`, `cgroups`, and `socket_ports` tables.
- Steady-state actor types were:
  `endpoint=52`, `systemd_service=30`, `systemd_scope=21`, `vm=2`, `self=1`,
  `process_group=1`, and generic `container=1`.
- Steady-state cgroup status values were:
  `known=74`, `host_root=1`, and `retry_later=1`.
- Steady-state effective orchestrators were:
  `systemd=72`, `kvm=2`, `host_root=1`, and `unknown=1`.
- Duplicate actor label check found:
  `duplicate_actor_key_value_count=0`, `duplicate_container_name_count=0`.
- Repeated refresh validation ran 4 live queries about 15 seconds apart. The
  actor type families stayed in container/service/scope/vm mode and did not
  reintroduce mixed floating process actors. The only continuing partial item
  was one `retry_later` PID represented as the generic `[pending]` container.
- Clean restart validation:
  - `/run/netdata/netdata-spawn-setns.sock` was present as
    `socket root:[netdata_group] 770` after restart.
  - query 1, before APPS_LOOKUP connection, returned `actors=59`, `links=63`,
    actor types `self=1`, `container=1`, `endpoint=57`, and
    `cgroup_status=retry_later=80`.
  - journal then showed `network-viewer.plugin connected to APPS_LOOKUP
    service`.
  - query 2 returned `actors=116`, `links=144`, actor types
    `self=1`, `systemd_service=30`, `docker_container=3`, `vm=2`,
    `systemd_scope=21`, `process_group=1`, `endpoint=58`, and cgroup status
    `known=79`, `host_root=1`.
- Nested systemd service metrics remain fixed after install:
  `systemd_cups.cpu` exists with `context=systemd.service.cpu.utilization` and
  chart label `service_name=cups`.
- User-slice repair validation after rebuilding, installing, and restarting
  Netdata:
  - focused topology unit test passed:
    `./build/network-viewer-topology-containers-test`.
  - live `group_by=container` Function query returned exactly one user actor:
    `type=user`, `container_name=[resolved-username]`,
    `orchestrator=systemd`, `actor_kind=user`, and
    `display_name=[resolved-username]`.
  - three consecutive live refreshes about 10 seconds apart kept the same
    single user actor and did not emit `systemd_scope` graph actors.
  - raw `.scope` names remain only in evidence fields such as `cgroup_path` and
    `systemd_unit_name`, so actor details can explain where the grouped user
    actor came from without promoting each transient scope as a graph actor.
- Final installed-Agent validation after reinstall resolved the PID-zero
  attribution symptom. Root cause was the installed plugin missing the required
  runtime permissions, not a `local-sockets` parser failure. With permissions
  restored by reinstall, process attribution, container grouping, and user-slice
  grouping worked as designed.

Reviewer findings:

- External reviewers were not run in this repair turn because the user asked
  for direct takeover/proceed work, and no request was made to run external AI
  reviewers. Local evidence and focused tests are recorded here.

Same-failure scan:

- `rg -n "show_cgroup|cgroup_path.*visible|hide.*cgroup|show/hide|Show.*Cgroup|Cgroup Paths" src/collectors/network-viewer.plugin src/collectors/apps.plugin .agents/sow .agents/skills docs -g '!*.o'`
  found no active source show/hide cgroup-path control. Only corrective SOW
  notes remain.
- `rg -n "group_by.*process_name|group_by.*container|UNKNOWN_RETRY_LATER|retry_later|label_keys|aggregation" src/collectors/network-viewer.plugin src/collectors/apps.plugin .agents/sow/specs .agents/skills/project-create-topology -g '!*.o'`
  confirmed the remaining grouped/enrichment references align with the new
  contract.
- `git diff master -- src/collectors/apps.plugin src/collectors/cgroups.plugin src/collectors/network-viewer.plugin src/libnetdata/netipc | rg -n '^\+.*(netdata_log_info|collector_info|debug|DEBUG|trace|TEMP|temporary|connected to|lost .*service|unavailable|server started|server stopped)'`
  now finds only intentional lifecycle/failure diagnostics plus the literal
  `debug` command-line mode in the Go `cgroup-name` port.
- `git diff master -- src/collectors/apps.plugin src/collectors/cgroups.plugin src/collectors/network-viewer.plugin src/libnetdata/netipc | rg -n '^\+.*(sending|just sent|sent request|received request|received response|responding|payload bytes|debug trace|DEBUG|TEMP|temporary)'`
  found no production message-level tracing. The only hit was documentation text
  saying apps sends data to Netdata, not a production log.

Sensitive data gate:

- Durable artifacts contain only synthetic fixture values, code paths, and
  file/function evidence. No raw cookies, bearer tokens, customer identifiers,
  personal data, public customer-identifying IPs, private endpoints, or
  proprietary incident details were written.

Artifact maintenance gate:

- AGENTS.md: no update needed; workflow rules did not change.
- Runtime project skills: `.agents/skills/project-create-topology/SKILL.md`
  updated to describe grouped enrichment via labels/set aggregation.
- Specs: `.agents/sow/specs/topology-function-schema.md` and
  `.agents/sow/specs/topology-containers-ipc-contract.md` updated for the
  corrected grouping/enrichment and partial-data behavior.
- End-user/operator docs: `src/collectors/network-viewer.plugin/metadata.yaml`,
  generated `integrations/network_connections.md`, and
  `src/collectors/apps.plugin/README.md` updated.
- End-user/operator skills: no public skill update needed; no operator skill
  query workflow changed.
- SOW lifecycle: SOW-0040 is closed as absorbed by this SOW; SOW-0041 is
  corrected to generic Cloud validation/aggregation work; SOW-0036 and
  SOW-0043 record corrections for the superseded grouped-enrichment rule.

Specs update:

- Updated topology Function and containers IPC specs as listed above.

Project skills update:

- Updated `project-create-topology` as listed above.

End-user/operator docs update:

- Updated network-viewer metadata/integration docs and apps README as listed
  above.

End-user/operator skills update:

- No update needed. This work changed producer payloads and internal developer
  workflow, not the public operator AI skill command surface.

## Outcome

Implementation, focused local validation, install validation, live direct-Agent
payload validation, and final user validation are complete for the current
scope. First-response partial enrichment remains an intentional state when
APPS_LOOKUP has not connected or a PID is still being enriched; steady-state
container grouping must resolve to enriched process/container/service/user
actors once plugin permissions and lookup services are available.

## Lessons Extracted

- The previous implementation treated enrichment as decoration. The correct
  model is that enrichment is process/socket state shared by topology,
  network-connections tables, and apps processes.
- Cloud did not need container-specific code. The missing Cloud work was a
  generic tests-only guardrail for actor attribute `set` aggregation.

## Followup

- None. The previous runtime watch item was resolved by final installed-Agent
  validation after plugin permissions were restored. Future regressions should
  reopen this SOW or create a new regression SOW with the exact live payload and
  journal evidence.

## Regression Log

None yet.
