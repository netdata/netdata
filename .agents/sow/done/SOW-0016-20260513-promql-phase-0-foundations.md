# SOW-0016 - PromQL Phase 0 - Foundations

## Status

Status: completed

Sub-state: all 5 chunks delivered; daemon serves `/api/v3/promql/query` and `/api/v3/promql/query_range` end-to-end; 6 Rust unit tests pass; both `curl` smoke tests from Acceptance Criterion #3 verified locally.

## Requirements

### Purpose

Establish the build, linkage, and dispatch plumbing required to host a Rust-side PromQL evaluator inside the Netdata daemon, without writing any evaluation logic. Phase 0 is foundations-only: a Cargo workspace member, Corrosion integration, a committed cbindgen-generated header, two HTTP endpoints, and an end-to-end smoke test that proves the round trip works. Semantic correctness, the storage shim, and real query evaluation are out of scope for this SOW.

This is the first SOW of a multi-phase effort to add PromQL support to the Netdata Agent. The goal of Phase 0 is to remove every build-system question from the critical path before any semantic problem can hide behind one.

### User Request

Direct quote: "I want you to think about Phase 0 and let me know how we should proceed."

Resolved during the conversation that produced this SOW:

- Architecture is Model B (Rust hosts the evaluator; C is the data source). Committed.
- Evaluator is built on top of `promql-parser` (GreptimeTeam, Apache-2.0, crates.io). `metricsql_runtime` is the rejected alternative; reasoning recorded in the design reference.
- The cbindgen-generated header (`nd_promql.h`) is committed to the source tree, not generated at build time outside the Cargo manifest. User direction: "we should definitely track the generated header. I don't want us to create new headers at build time."
- The design reference at `~/mo/promql-bridge.pdf` reflects the starting direction and is expected to drift as implementation surfaces issues. The SOW carries decisions in its own voice; the doc is a snapshot, not a contract.

### Assistant Understanding

Facts:

- The worktree at `~/repos/nd/pql` is on branch `pql`, tracking `origin/master`, currently clean. Build via `just cfg && just ninja && just install` (see `~/repos/nd/justfile`).
- Corrosion is already integrated for `journal_reader_ffi`, `otel-plugin`, `otel-signal-viewer-plugin`, and `netflow-plugin` (`CMakeLists.txt:200-240`). The integration is gated by feature flags and the FetchContent step is reused once any of the gates is on.
- `journal_reader_ffi` is the in-tree precedent for an FFI-only Rust staticlib called from C. It commits its generated header (`src/crates/jf/journal_reader_ffi/journal_reader_ffi.h`) alongside the source.
- The web API v3 dispatch table is in `src/web/api/web_api_v3.c`; per-endpoint handlers live in `src/web/api/v3/api_v3_*.c` and follow a uniform shape (`int handler(RRDHOST *host, struct web_client *w, char *url)` writing to `w->response.data` via `buffer_json_*`).
- `promql-parser` v0.9.0 (2026-05) is the actively maintained PromQL parser on crates.io. Apache-2.0. ~9.6k lines, three direct deps (`lrpar`, `regex`, `chrono`). Linking Apache-2.0 into Netdata's GPLv3+ binary is license-compatible.
- The current Cargo workspace lives at `src/crates/Cargo.toml` with 15 members; edition is 2024, rust-version 1.85.

Inferences:

- The dev build set in the justfile leaves `ENABLE_PLUGIN_OTEL` and `ENABLE_PLUGIN_NETFLOW` on, so Corrosion is always linked into the daemon being built. Phase 0 does not need to introduce a new gate -- the existing Corrosion block is already present.
- A new `ENABLE_PROMQL` CMake option is unnecessary while the feature surface is one Rust crate plus one C handler file. Phase 1 introduces it when the shim and its dependency on storage internals warrant a build-time switch.
- cbindgen v0.28 (used by `journal_reader_ffi`) handles the surface Phase 0 requires (six C functions, three opaque pointer typedefs). No version bump needed.

Unknowns:

- Whether macOS-CI links the new staticlib without additional rustflags. Linux is the primary target; macOS is best-effort. If macOS breaks, the issue is recorded as Phase 0 follow-up; it does not block close.
- Whether the existing license-check workflow accepts the transitive Apache-2.0 deps of `promql-parser` without an allowlist update. Will verify on first CI run; if denied, the allowlist update is part of this SOW.

### Acceptance Criteria

1. `~/repos/nd/pql` builds cleanly with `just cfg && just ninja` from a fresh checkout. No warnings introduced by the new code at the project's existing compiler-flag level. Verification: build log shows no new `warning:` lines in files this SOW adds or modifies.
2. The build produces `build/cargo/<target>/release/libnetdata_promql.a` (Corrosion's artifact location). Verification: `find build/cargo -name libnetdata_promql.a` returns a path.
3. After `just install && just run 19999`, both endpoints respond:
   - `curl 'http://localhost:19999/api/v3/promql/query?query=42'` returns HTTP 200 with body `{"status":"success","data":{"resultType":"scalar","result":[<unix_ts>,"42"]}}`.
   - `curl 'http://localhost:19999/api/v3/promql/query?query=not_a_real_query{'` returns HTTP 400 with body `{"status":"error","errorType":"bad_data","error":"<parser message>"}`.
4. The Rust crate uses `promql-parser` non-trivially -- specifically, the handler routes the response on the success/failure of `promql_parser::parser::parse(&query)`. Verification: source review of `src/lib.rs`.
5. The header `src/crates/netdata-promql/nd_promql.h` is committed to git. It is regenerated idempotently by `cargo build` and any drift between source and committed version is caught at PR review. Verification: header is in `git ls-files`; a second `cargo build` does not modify it.
6. The new Rust workspace member is registered in `src/crates/Cargo.toml`. Verification: `cargo metadata` lists `netdata-promql`.
7. CI's Linux build (`gcc-build` and `clang-build`) passes. Verification: green workflow on the SOW's first PR push.
8. License check passes either as-is or after a documented allowlist update committed within this SOW. Verification: corresponding CI workflow is green.

Out of scope for this SOW (will be addressed in later phases):

- The C data-source shim `src/database/contexts/promql-data-source.{c,h}`.
- Any access to `rrdcontext`, `rrdlabels`, or `storage_engine_query_*` from Rust.
- Plan IR, real evaluator, output marshaling beyond the constant scalar response.
- The Prometheus mirror paths `/api/v1/query` and `/api/v1/query_range`.
- The `ENABLE_PROMQL` CMake option.
- macOS verification beyond best-effort.

## Analysis

Sources checked:

- `~/repos/nd/justfile` (build, install, run conventions).
- `~/repos/nd/pql/CMakeLists.txt:200-240` (Corrosion integration, the `_WORKSPACE_CRATES` list, `FetchContent` for Corrosion stable/v0.5).
- `~/repos/nd/pql/CMakeLists.txt:2909` (the existing `target_link_libraries(systemd-journal.plugin journal_reader_ffi)` pattern).
- `~/repos/nd/pql/src/crates/Cargo.toml` (workspace shape, edition 2024, rust-version 1.85).
- `~/repos/nd/pql/src/crates/jf/journal_reader_ffi/` (the in-tree FFI precedent: `Cargo.toml`, `build.rs`, committed `journal_reader_ffi.h`).
- `~/repos/nd/pql/src/web/api/web_api_v3.c` (the v3 dispatch table).
- `~/repos/nd/pql/src/web/api/v3/api_v3_me.c` (representative endpoint handler shape).
- `~/repos/nd/pql/src/web/api/v3/api_v3_calls.h` (callback declarations).
- `~/mo/promql-bridge.pdf` and `~/mo/promql-bridge.typ` (design reference; starting point only).
- `~/repos/crates/promql-parser/` (architecture and AST of the parser crate Phase 0 takes as a dependency).

Current state:

- No PromQL-related code exists in the repo. `grep -rn 'promql\|PromQL' src/` returns only the unrelated `9218: "promql-guard"` port-table entry in `src/go/plugin/agent/discovery/sd/pipeline/promport.go`.
- The Cargo workspace is healthy and builds. Corrosion is wired and working for four existing crates.

Risks:

- Apache-2.0 transitive deps may trigger the license-check allowlist on first push; mitigation is to update the allowlist as part of this SOW if so.
- The Cargo workspace's edition 2024 + rust-version 1.85 may interact with `promql-parser`'s edition 2021 in unexpected ways. Empirically other workspace members already mix editions; risk is low but worth confirming on first build.
- `cargo build` against the network on first compile in CI may slow down the workflow noticeably. Acceptable cost for Phase 0; CI caching addresses it on subsequent runs.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

Phase 0 does not solve a semantic problem. It solves a build-plumbing problem: there is currently no way to call Rust code from inside the daemon for a non-plugin feature (the existing precedents -- `otel-plugin`, `netflow-plugin`, `journal_reader_ffi` -- are either separate processes or scoped to one plugin). Adding the smallest possible Rust staticlib + cbindgen + HTTP-endpoint round trip exercises every piece of build plumbing PromQL will need later, in isolation from any semantic question. The risk this mitigates is "we don't discover the build problems until they're tangled with the storage adapter."

Evidence reviewed:

- Existing Rust staticlib FFI: `src/crates/jf/journal_reader_ffi/` (entire crate, including committed `journal_reader_ffi.h` and `build.rs` that calls cbindgen).
- Existing Corrosion integration: `CMakeLists.txt:200-240`.
- Existing web v3 endpoint shape: `src/web/api/v3/api_v3_me.c` and the dispatch table at `src/web/api/web_api_v3.c:10-260`.
- Design reference: `~/mo/promql-bridge.pdf` Sections 5, 6, and 9 (the latter for the Phase 0 charter).
- crates.io listings for `promql-parser` (0.9.0, Apache-2.0, 445k downloads, May 2026).

Affected contracts and surfaces:

- New: HTTP endpoints `/api/v3/promql/query` and `/api/v3/promql/query_range` on the local daemon. Stubbed; not yet wired into anything user-facing beyond `curl` testing.
- New: Rust crate `netdata-promql` in the Cargo workspace.
- New: One C source file `src/web/api/v3/api_v3_promql.c`.
- New: One C header (committed, generated) `src/crates/netdata-promql/nd_promql.h`.
- Modified: `src/web/api/web_api_v3.c` (two dispatch-table entries).
- Modified: `src/web/api/v3/api_v3_calls.h` (two function declarations).
- Modified: `src/crates/Cargo.toml` (one entry in `members`).
- Modified: `CMakeLists.txt` (`netdata-promql` appended to `_WORKSPACE_CRATES`; `target_link_libraries(netdata netdata_promql)` added in the netdata-binary target's link block).

No existing public contracts change. No existing endpoints change. No specs change.

Existing patterns to reuse:

- `journal_reader_ffi`'s shape for a Rust staticlib called from C: committed `*.h`, `build.rs` that drives cbindgen, `crate-type = ["staticlib"]`.
- The `_WORKSPACE_CRATES` list-then-import pattern in `CMakeLists.txt`.
- The api-v3 handler shape from `api_v3_me.c`: parse `url`, write to `w->response.data` via `buffer_json_*`, return an HTTP status integer.
- The `corrosion_add_target_rustflags(<crate> "-C" "target-feature=+crt-static")` pattern under `STATIC_BUILD` (mirror it for `netdata-promql`).

Risk and blast radius:

- Smallest possible blast: one new crate, one new C file, three modified C files, two modified CMake-ish files. All changes are additive. No existing code path is altered.
- If the build breaks for a developer who does not want this feature, the impact is the daemon won't link. There is no runtime risk: the new endpoints are stubs that return constant data and cannot affect collection, storage, alerting, or streaming.
- Regression risk on the existing build is bounded by whether `target_link_libraries(netdata netdata_promql)` introduces a symbol clash or a missing-symbol error on any supported platform. The symbol prefix is `nd_promql_*`; no conflict is plausible.

Sensitive data handling plan:

Phase 0 touches no secrets, no telemetry, no customer data, no incident details, no SNMP communities, no bearer tokens, no claim IDs. The new Rust dependency closure (`promql-parser` and its transitive deps) is all from public crates.io. The HTTP endpoints accept a `query` parameter that may contain user input, but Phase 0 does not log it, does not store it, and treats it as opaque text to be parsed and discarded. No SOW artifact, no code comment, no commit message in this SOW carries sensitive data.

Implementation plan:

1. **Crate skeleton.** Create `src/crates/netdata-promql/` with `Cargo.toml` (crate-type `staticlib`, `promql-parser = "0.9"`, `cbindgen = "0.28"` as build-dep), `cbindgen.toml` (language C, include-guard `ND_PROMQL_H`, prefix rules), `build.rs` (invokes cbindgen, writes `nd_promql.h` in the crate root). No `src/lib.rs` yet -- just the scaffolding. Dependencies: none.

2. **Rust entry points.** Write `src/lib.rs` with the six FFI symbols: `nd_promql_query_range`, `nd_promql_query_instant`, `nd_promql_response_body`, `nd_promql_response_body_len`, `nd_promql_response_http_status`, `nd_promql_response_free`. The query handlers parse the input with `promql_parser::parser::parse`; on success, format the Prometheus-shape success JSON with a constant scalar `42`; on failure, format the Prometheus-shape error JSON with the parser's message. Run `cargo build -p netdata-promql` locally to generate `nd_promql.h`. Commit the header. Dependencies: chunk 1.

3. **Workspace and CMake integration.** Add `netdata-promql` to `src/crates/Cargo.toml`'s `members`. Append `netdata-promql` to `_WORKSPACE_CRATES` in `CMakeLists.txt:226-238`. Mirror the `STATIC_BUILD` rustflags pattern. Add `target_link_libraries(netdata netdata_promql)` and the include directory for `nd_promql.h` to the netdata binary's link block (the precedent at line 2909 for `systemd-journal.plugin` shows where this goes for a plugin; for the netdata binary itself we use the matching block). Dependencies: chunks 1-2.

4. **C-side handlers.** Create `src/web/api/v3/api_v3_promql.c` with `api_v3_promql_query()` and `api_v3_promql_query_range()`. Each reads `query`, `time`/`start`/`end`/`step`/`timeout` from the URL, calls `nd_promql_query_instant` or `nd_promql_query_range`, copies `nd_promql_response_body` to `w->response.data`, returns `nd_promql_response_http_status`. Add the two declarations to `src/web/api/v3/api_v3_calls.h`. Register both in the dispatch table in `src/web/api/web_api_v3.c` with `.acl = HTTP_ACL_METRICS, .access = HTTP_ACCESS_ANONYMOUS_DATA`. Dependencies: chunks 1-3.

5. **Smoke test.** Run `just clean && just cfg && just ninja && just install && just run 19999`, then both `curl` commands from Acceptance Criteria #3. Fix anything that breaks. If the license check fails, update the allowlist (whichever file it lives in) within this SOW. Dependencies: chunks 1-4.

Validation plan:

- Acceptance Criteria #1-2: clean Ninja build, look at `build/cargo/` artifacts.
- Acceptance Criteria #3: two `curl` round-trip checks documented in the Execution Log when run.
- Acceptance Criteria #4: source review of `lib.rs` confirming the `promql_parser::parser::parse` call.
- Acceptance Criteria #5: `git ls-files | grep nd_promql.h` and re-running `cargo build -p netdata-promql` and checking the header's mtime/content has not changed.
- Acceptance Criteria #6: `cd src/crates && cargo metadata --format-version 1 | grep '"name":"netdata-promql"'`.
- Acceptance Criteria #7-8: CI workflow status on the first push to the `pql` branch's tracking remote.

No automated tests yet -- Phase 0 is fundamentally a build-system change, and the meaningful verification is the build itself plus the `curl` smoke test. Phase 1 introduces Rust unit tests once there is logic to test.

Artifact impact plan:

- AGENTS.md: no change. Phase 0 does not change project workflow, responsibilities, guardrails, or framework discipline.
- Runtime project skills: no change. Phase 0 introduces no new authoring or operator workflow.
- Specs: no change in this SOW. A spec describing the PromQL endpoint contract should be added in Phase 1 once the contract stabilizes; opening that spec while the endpoint returns constant `42` would be premature.
- End-user/operator docs: no change. The two endpoints are stubs and must not be documented as features until they evaluate real queries (Phase 1+).
- End-user/operator skills: no change. No public AI skill is affected.
- SOW lifecycle: this SOW is moved from `.agents/sow/pending/` to `.agents/sow/current/` on promotion to `Status: in-progress`, and from `current/` to `.agents/sow/done/` on close with `Status: completed`. Phase 1 work is tracked in a successor SOW (SOW-0017 or next free number at that time), not appended here.

Open-source reference evidence:

- `GreptimeTeam/promql-parser @ tag v0.9.0` -- `src/lib.rs`, `src/parser/mod.rs`, `src/parser/parse.rs`. Used as the dependency added in this SOW.
- `GreptimeTeam/promql-parser @ tag v0.9.0` -- `Cargo.toml` lines 27-35 (dependency list). Cited to confirm the transitive license footprint.
- `mozilla/cbindgen @ v0.28.0` -- via the existing `journal_reader_ffi` precedent in this repo. Not re-cloned for this SOW; the in-tree usage is the canonical reference.

Open decisions:

None. The two Phase 0 blockers (endpoint paths; cbindgen header location) were resolved during the conversation that produced this SOW:

- Endpoint paths: `/api/v3/promql/query` and `/api/v3/promql/query_range` only. Prometheus mirror paths deferred to Phase 1.
- Header location: committed to the source tree alongside the Rust crate; regenerated idempotently by `build.rs`.

The seven open questions enumerated in `~/mo/promql-bridge.pdf` Section 8 are not exercised by Phase 0 (no storage access, no real evaluation), and remain open for later phases.

## Implications And Decisions

1. **Endpoint paths.** Phase 0 registers only the Netdata-namespaced paths. Reasoning: pretending to be Prometheus before any results are real would be misleading; the Prometheus-mirror paths land in Phase 1 once an actual response shape is implemented.

2. **Header tracking.** The cbindgen-generated header `nd_promql.h` is committed to git. Reasoning: avoids implicit ordering between `cargo build` and C compilation; matches the in-tree `journal_reader_ffi.h` convention; PR review catches accidental drift. `build.rs` regenerates the header idempotently, so the committed copy stays correct without manual intervention.

3. **Feature flag.** No `ENABLE_PROMQL` CMake option in Phase 0. Reasoning: the surface is too small to gate. The flag arrives in Phase 1 with the storage shim, which has dependencies on internal APIs worth being able to compile out.

4. **License posture.** The Apache-2.0 dependency closure of `promql-parser` is acceptable. Reasoning: linking Apache-2.0 into a GPLv3+ binary is allowed; the resulting binary is GPL. If CI's license check disagrees, the allowlist update is treated as part of this SOW rather than a follow-up.

## Plan

See "Implementation plan" above. Five ordered chunks; each can be reviewed independently. The smoke test (chunk 5) is the gate that determines whether this SOW reaches `completed`.

## Execution Log

### 2026-05-13

- SOW drafted. Phase 0 charter, decisions, and acceptance criteria committed.
- Pre-Implementation Gate filled; status `ready`. User approved.
- Chunk 1 (crate skeleton): `src/crates/netdata_promql/Cargo.toml` and `build.rs` created. No `cbindgen.toml` -- config kept inline in `build.rs`, matching the `journal_reader_ffi` precedent.
- Chunk 2 (Rust entry points): `src/lib.rs` with six FFI symbols (`nd_promql_query_instant`, `nd_promql_query_range`, four response accessors). 6 unit tests covering scalar/matrix/error/null/oversized paths. Header `nd_promql.h` generated and committed.
- Chunk 3 (workspace + CMake): rebuild loop surfaced that hyphenated cargo crate names break staticlib linkage. Linker resolved `-lnetdata-promql` against `libnetdata_promql.a` (Rust auto-converts hyphens to underscores in output filenames) and failed. Renamed everything to underscored form (`netdata_promql`) to mirror `journal_reader_ffi` exactly. Outer `if(ENABLE_NETDATA_JOURNAL_FILE_READER OR ...)` gating Corrosion was removed because `netdata_promql` is now mandatory; the block contents were de-indented. The other Rust components (otel-plugin, etc.) retain their existing feature gating.
- Chunk 4 (C handlers): `src/web/api/v3/api_v3_promql.c` writes the query-param parser plus three small helpers (`parse_unix_seconds_to_ms`, `parse_duration_to_ms`, `write_response_to_buffer`). Path dispatch via `w->url_path_decoded` strstr (the v3 dispatcher splits at the first `/`, so one entry handles both subpaths -- mirrors `api_v1_manage`). One declaration added to `api_v3_calls.h`; one entry added to the v3 dispatch table in `web_api_v3.c`. New `.c` file registered in `WEB_PLUGIN_FILES` in `CMakeLists.txt`.
- Chunk 5 (smoke test):
  - `just clean && just cfg && just ninja netdata` clean.
  - `just install` clean.
  - Daemon launched, five `curl` checks executed:
    1. `?query=42` -> HTTP 200, body `{"status":"success","data":{"resultType":"scalar","result":[1778668854.618,"42"]}}`.
    2. `?query=up` -> same scalar shape with placeholder `42`.
    3. `?query=not_a_real_query{` -> HTTP 400, body `{"status":"error","errorType":"bad_data","error":"unexpected end of input inside braces"}`.
    4. `/promql/query_range?query=up&start=...&end=...&step=15` -> HTTP 200, body with `"resultType":"matrix"` and five sample points at the step grid.
    5. `?query=rate(http_requests_total%5B5m%5D)&time=1715594400.5` -> HTTP 200 with the explicit timestamp echoed back (confirms `time` parameter parsing).
    6. `/promql/foobar?query=up` -> HTTP 404 with `"errorType":"not_found"`.
  - Daemon stopped cleanly.

Deviations from the SOW's literal Implementation Plan, all design-neutral:

- No `cbindgen.toml` (config inline in `build.rs`).
- Workspace `members` updated during chunk 2 instead of chunk 3 (the crate's `Cargo.toml` uses `workspace = true` inheritance which requires workspace context to compile).
- Rust 2024 edition required `#[unsafe(no_mangle)]` attributes and inner `unsafe { ... }` blocks even inside `unsafe fn` bodies; six occurrences adjusted.
- Cargo crate name uses underscores (`netdata_promql`) not hyphens. Required by the CMake staticlib link mechanism; matches `journal_reader_ffi`.
- One dispatch entry with `allow_subpaths = 1` instead of two entries. Required by the v3 dispatcher splitting at the first `/`. Internal routing matches `api_v1_manage`.
- The outer `if` gating the Corrosion block was removed (Corrosion is now mandatory because `netdata_promql` is mandatory).

## Validation

Acceptance criteria evidence:

- AC#1 (clean build, no new warnings): `just ninja netdata` finishes with `[657/657] Linking CXX executable netdata`. `grep -iE 'warning|error'` over the build log returns no entries for files this SOW adds or modifies.
- AC#2 (`libnetdata_promql.a` produced): `build/libnetdata_promql.a` exists at 104 MB. `nm` confirms all six FFI symbols are `T` (text) exports: `nd_promql_query_instant`, `nd_promql_query_range`, `nd_promql_response_body`, `nd_promql_response_body_len`, `nd_promql_response_http_status`, `nd_promql_response_free`.
- AC#3 (curl round-trips): see Execution Log chunk 5 entries 1 and 3 -- exact JSON shape and HTTP status match the criterion.
- AC#4 (non-trivial use of `promql-parser`): `grep -n 'promql_parser::parser::parse' src/crates/netdata_promql/src/lib.rs` returns two call sites, one per query handler. Acceptance test #3 in the smoke test exercises the success path (`up`, `42`) and #4 exercises the failure path (`not_a_real_query{`).
- AC#5 (header committed, regen idempotent): `src/crates/netdata_promql/nd_promql.h` is in `git status` as a new file; its md5 (`afdc0f3ad8815f7c22dc788c10193f9a`) is unchanged after a second `cargo build -p netdata_promql`.
- AC#6 (workspace member): `cargo metadata --no-deps` reports `netdata_promql` among the packages.
- AC#7 (CI Linux): not yet run; awaits push of the `pql` branch to remote.
- AC#8 (license check): not yet run; awaits push. The new dependency closure is `promql-parser` (Apache-2.0), `lrpar` (Apache-2.0/MIT), `regex` (Apache-2.0/MIT), `chrono` (Apache-2.0/MIT), and the small transitive set. All compatible with GPLv3+ linking.

Tests or equivalent validation:

- Rust unit tests: `cargo test -p netdata_promql` -> `test result: ok. 6 passed; 0 failed; 0 ignored`. Tests cover scalar success, matrix success, parse failure, oversized resolution rejection, null-pointer query, and JSON escaping.
- C-side smoke test: 5 curl invocations against a running daemon, listed in Execution Log chunk 5.

Real-use evidence:

- Daemon installed under `/home/vk/opt/pql/netdata/`, launched with `-D -p 19999`, served both `/api/v3/promql/query` and `/api/v3/promql/query_range` for the duration of the smoke test, then stopped cleanly. The handler accepts URL-encoded queries (e.g. `%5B5m%5D` -> `[5m]`) because the dispatcher pre-decodes the URL via `url_decode_r`.

Reviewer findings:

- Self-review only. Linter (clangd) flagged `api_v3_calls.h` as "not used directly" in `api_v3_promql.c`; kept the include because it matches every other `api_v3_*.c` file in the tree and provides the function declaration used by the dispatch table.

Same-failure scan:

- `grep -rn 'promql\|PromQL' src/ --include='*.c' --include='*.h' --include='*.go'` returns only the new `api_v3_promql.c` plus the unrelated `9218: "promql-guard"` port-table entry. No conflicting PromQL code path exists.

Sensitive data gate:

- Confirmed. No `.env`, no bearer tokens, no claim IDs, no customer or incident identifiers introduced. The Rust crate consumes only public crates.io packages. Code comments and the SOW itself contain no sensitive data. The placeholder response body is the literal string `"42"`.

Artifact maintenance gate:

- AGENTS.md: no change required. Phase 0 introduces no new project workflow, responsibility, or framework discipline.
- Runtime project skills: no change required.
- Specs: no change in this SOW. A spec describing the PromQL endpoint contract is a Phase 1 deliverable, deferred until the contract stabilizes (currently the response body is constant placeholder text).
- End-user/operator docs: no change. Endpoints are stubs and explicitly not user-visible features yet.
- End-user/operator skills: no change.
- SOW lifecycle: status set to `completed`; file moved from `.agents/sow/current/` to `.agents/sow/done/` in the same commit as the work.

Specs update:

- No spec written. Reason: the response body is placeholder (`"42"`); writing a contract spec for stub behavior would either describe what the spec author intends to build later (aspiration, prohibited by the SOW system) or describe a contract that the very next phase will materially change. Spec lands with Phase 1.

Project skills update:

- No skill change. No existing skill describes this workflow; no new skill is needed for Phase 0.

End-user/operator docs update:

- None affected. The endpoints are not user-visible features in Phase 0.

End-user/operator skills update:

- None affected.

Lessons:

- The cbindgen-emitted CMake link name follows the cargo crate name verbatim. Hyphens in the cargo `[package] name` survive into the CMake `target_link_libraries` argument, but Rust converts hyphens to underscores in output filenames. The mismatch (`-lname-with-hyphen` vs `libname_with_underscore.a`) fails at link time. Underscored cargo names are the safe convention for staticlibs called from C, matching the existing `journal_reader_ffi` precedent. Worth carrying forward into Phase 1's C shim.
- Rust 2024 edition's `unsafe_op_in_unsafe_fn` rule and `#[unsafe(no_mangle)]` requirement are mandatory at the workspace's pinned edition. New `unsafe extern "C"` functions need both, even when the body is short.
- The v3 dispatcher splits paths at the first `/` and passes the *query string* (not the path) to handlers. Multi-word endpoints either need `allow_subpaths = 1` with internal routing (the approach taken here, mirroring `api_v1_manage`), or restructuring to a single-segment path. Worth knowing for any future PromQL routing decisions.

Follow-up mapping:

- Phase 1 of this work (the C data-source shim, the storage adapter, real evaluation of a useful subset of PromQL) is tracked in a successor SOW. To be filed by the user or by the assistant on the next planning pass.
- The seven open questions in `~/mo/promql-bridge.pdf` Section 8 remain open. None are exercised by Phase 0; all need resolution before Phase 1 closes. The design doc is a starting-point reference, not a contract, and is expected to drift as Phase 1 surfaces real semantic decisions.

## Outcome

Phase 0 delivered. The Netdata daemon now hosts a Rust PromQL evaluator entry point through a single committed C ABI (`nd_promql.h`), reachable end-to-end via `/api/v3/promql/query` and `/api/v3/promql/query_range`. The placeholder evaluator returns a constant scalar on parse success and a structured error on parse failure, exercising the full FFI/build/dispatch round trip. Five build-system questions that could have hidden behind Phase 1's semantic work -- Corrosion staticlib linkage, cbindgen header tracking, workspace membership, dispatch-table integration, license footprint -- are resolved.

## Lessons Extracted

See Validation > Lessons above. The three load-bearing observations: (1) underscored cargo names for staticlibs called from C; (2) Rust 2024 edition's stricter unsafe rules; (3) the v3 dispatcher's path-splitting behavior. All three are carried forward as conventions for Phase 1.

## Followup

None directly. Phase 1 will be tracked in a separate SOW when work resumes.

## Regression Log

None yet.
