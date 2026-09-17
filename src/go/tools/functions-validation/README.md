# Functions Validation (CLI + Containers)

**Place in the documentation set.** The project skill `.agents/skills/topology-authoring/SKILL.md` cites sections
of this document by heading anchor, and `.agents/sow/audit.sh` fails when a cited heading no longer exists, so
renaming or removing a heading here updates the skill in the same change.

## TL;DR
- Bring up databases with Docker Compose.
- Start `go.d.plugin` normally and send Function requests through its stdin protocol.
- The current CLI has no `--function` or `--function-args` flags. `--config-dir` remains available as `-c`.
- Config files live under `./config/go.d`.
- Extract each Function JSON result and validate it against the canonical schema.
- The container E2E scripts still use the removed CLI flags and need migration before they can validate current builds.

## Start containers
```
docker compose up -d
```

## Example CLI run (Postgres)
Run from the repository root:

```bash
(cd src/go && go build -o /tmp/go.d.plugin ./cmd/godplugin)
/tmp/go.d.plugin -c src/go/tools/functions-validation/config -m postgres
```

For an automated pipe-based harness, keep stdin open and consume stdout continuously. In non-terminal mode,
wait for the job's `CONFIG ... create ... job` announcement, then enable that exact job through DynCfg. For a
job announced as `go.d:collector:postgres:local`, send:

```text
FUNCTION enable-local 30 "config go.d:collector:postgres:local enable" 0xFFFF "method=api,role=test"
```

After `FUNCTION GLOBAL "postgres:top-queries"` is announced, send requests on the same stdin:

```text
FUNCTION query-info 30 "postgres:top-queries info" 0xFFFF "method=api,role=test"
FUNCTION query-data 30 "postgres:top-queries __job:local" 0xFFFF "method=api,role=test"
```

Match each response by its request ID in `FUNCTION_RESULT_BEGIN`, and save only the JSON body up to
`FUNCTION_RESULT_END`; stdout also carries charts and configuration messages. `info` describes the method and
is not a data sample. A collector that publishes collected snapshots may return unavailable until its first
collection. Send `QUIT` and wait for this process when the harness finishes.

The framing is shared by all job-backed Functions; use the module, method and announced job from the tested
collector. Single-instance collectors do not expose `__job`. Sources: `pkg/cli`, `plugin/agent/policy/runmode.go`
and the process-level examples in `plugin/agent/jobmgr/internal/jobmgrtest/agent_predicate_helpers.go`.

## Validate output

```
echo '{"status":200,"type":"table","columns":{},"data":[]}' | \
  (cd src/go && go run ./tools/functions-validation/validate)
```

## Validate topology v1 fixtures

```
(cd src/go && \
  go run ./tools/functions-validation/validate \
    --schema ../plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json \
    --input tools/functions-validation/fixtures/topology-v1/network-connections.json)
```

Topology v1 validation uses the JSON Schema and additional compact-table
semantic checks: decoded column lengths must match `rows`, dictionary indexes
must be in range, actor/link references must point to existing rows, and
correlation rules must reference existing actor/link types and point/claim key
columns.

## Validate output (require rows)
Save the JSON body of a successful data response as `/tmp/pg.json`, then run from the repository root:

```bash
(cd src/go && go run ./tools/functions-validation/validate --input /tmp/pg.json --min-rows 1)
```

## E2E runner (legacy)

These commands document the existing container scripts. They currently invoke removed CLI flags and do not
validate current `go.d.plugin` builds; migrate their request transport before using them. The standalone JSON
validator and topology fixture validation above remain usable.
```
./e2e.sh
```
```
./e2e.sh --jobs 4
```
```
./e2e.sh --only postgres,mysql
```
```
./e2e.sh --list
```

## Per-DB E2E (single DB)
```
./e2e/postgres.sh
```

### Behavior
- Each DB script creates a workspace under `/tmp` and runs Docker Compose there.
- Ports are auto-selected per run to avoid collisions.
- Builds `go.d.plugin` into the `/tmp` workspace.
- Validates schema **and** that data rows are returned for top-queries.
- Cleans up the `/tmp` workspace on success; keeps it on failure for debugging.

## Notes
- The compose file sets credentials that match the sample configs in `./config`.
- MSSQL uses an init container to enable Query Store and seed data.
- MongoDB enables the profiler to populate `system.profile`.
- The validator reads the canonical schema at `src/plugins.d/FUNCTION_UI_SCHEMA.json`.
- Topology v1 fixtures live under `src/go/tools/functions-validation/fixtures/topology-v1/`.
