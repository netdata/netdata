# Functions Validation (Agent Protocol + Containers)

## TL;DR
- Bring up databases with Docker Compose.
- Run `go.d.plugin` as an Agent and invoke Functions through its stdin/result
  protocol with the configs in `./config`.
- Config files live under `./config/go.d`.
- Validate output against the embedded schema.
- Use `./e2e.sh` for automated end-to-end checks in `/tmp` (runs per-DB scripts).

## Start containers
```
docker compose up -d
```

## Example Agent-protocol run (Postgres)
```
cd ../../../src/go
go build -o /tmp/go.d.plugin ./cmd/godplugin
go run ./tools/functions-validation/call \
  --plugin /tmp/go.d.plugin \
  --config-dir ./tools/functions-validation/config \
  --module postgres \
  --function postgres:top-queries \
  --arg info
```

Each `--arg` supplies one whitespace-free Function argument token. Repeat the
flag when a Function takes multiple arguments.

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
```
go run ./tools/functions-validation/call \
  --plugin /tmp/go.d.plugin \
  --config-dir ./tools/functions-validation/config \
  --module postgres \
  --function postgres:top-queries \
  --arg __job:local \
  > /tmp/pg.json

go run ./tools/functions-validation/validate --input /tmp/pg.json --min-rows 1
```

## E2E runner (recommended)
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
- Builds `go.d.plugin` and the Agent-protocol Function caller into the `/tmp`
  workspace.
- Waits for the requested Function publication, writes a real `FUNCTION` record
  to Agent stdin, and validates the matching `FUNCTION_RESULT_BEGIN/END` frame.
- Validates schema **and** that data rows are returned for top-queries.
- Cleans up the `/tmp` workspace on success; keeps it on failure for debugging.

## Notes
- The compose file sets credentials that match the sample configs in `./config`.
- MSSQL uses an init container to enable Query Store and seed data.
- MongoDB enables the profiler to populate `system.profile`.
- The validator reads the canonical schema at `src/plugins.d/FUNCTION_UI_SCHEMA.json`.
- Topology v1 fixtures live under `src/go/tools/functions-validation/fixtures/topology-v1/`.
