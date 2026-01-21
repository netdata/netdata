# Functions Validation (CLI + Containers)

## TL;DR
- Bring up databases with Docker Compose.
- Use `go.d.plugin --function` with the configs in `./config`.
- Config files live under `./config/go.d`.
- Validate output against the embedded schema.
- Use `./e2e.sh` for automated end-to-end checks in `/tmp`.

## Start containers
```
docker compose up -d
```

## Example CLI run (Postgres)
```
cd ../../../
src/go/go.d.plugin \
  --config-dir src/go/tools/functions-validation/config \
  --function postgres:top-queries \
  --function-args info
```

## Validate output
```
echo '{"status":200,"type":"table","columns":{},"data":[]}' | \
  (cd src/go && go run ./tools/functions-validation/validate)
```

## Validate output (require rows)
```
src/go/go.d.plugin \
  --config-dir src/go/tools/functions-validation/config \
  --function postgres:top-queries \
  --function-args __job:local \
  > /tmp/pg.json

(cd src/go && go run ./tools/functions-validation/validate --input /tmp/pg.json --min-rows 1)
```

## E2E runner (recommended)
```
./e2e.sh
```

### Behavior
- Creates a workspace under `/tmp` and runs Docker Compose there.
- Builds `go.d.plugin` into the `/tmp` workspace.
- Validates schema **and** that data rows are returned for top-queries.
- Cleans up the `/tmp` workspace on success; keeps it on failure for debugging.

## Notes
- The compose file sets credentials that match the sample configs in `./config`.
- MSSQL uses an init container to enable Query Store and seed data.
- MongoDB enables the profiler to populate `system.profile`.
- The validator reads the canonical schema at `src/plugins.d/FUNCTION_UI_SCHEMA.json`.
