# Topology Parity Runbook

## Scope
- Validate topology engine parity against imported Enlinkd fixtures and assertion inventories.
- Evidence files live in `src/go/pkg/topology/engine/parity/evidence`.

## Prerequisites
- Upstream checkout available at `/tmp/topology-library-repos/enlinkd`.
- Run commands from `src/go`.

## Refresh Evidence
```bash
go run ./tools/topology-parity-evidence --mode sync
```

What it does:
- Verifies fixture source exists.
- Refreshes local fixture mirror.
- Regenerates:
  - `enlinkd-fixture-inventory.csv`
  - `enlinkd-test-method-inventory.csv`
  - `enlinkd-assertion-inventory.csv`

## Verify Mirror Integrity
```bash
go run ./tools/topology-parity-evidence --mode verify
```

Expected:
- `verify complete`
- Fixture counts match upstream mirror and evidence inventory.

## Run Full Parity Suite
```bash
go run ./tools/topology-parity-evidence --mode suite
```

Outputs:
- `parity-summary.json`
- Scenario pass/fail totals.
- Mapped test/assertion coverage totals.
- Determinism check (`runs=2`, `byte_identical=true` expected).

## Run Behavior Oracle Diff (Go vs Enlinkd Golden)
```bash
go run ./tools/topology-parity-evidence --mode oracle-diff
```

Outputs:
- `behavior-oracle-diff.json` (machine-readable per-scenario diff report).
- `behavior-oracle-diff.md` (human-readable summary).
- Fixture-input evidence per scenario (walk file path + sha256 + size).
- In-scope pass criteria: zero diffs for device identity set, hostname identity, adjacency set, and metadata counts.

## Run Go Test Gates
```bash
go test ./pkg/topology/engine/... ./plugin/go.d/collector/snmp ./tools/topology-parity-evidence -count=1
```

Expected:
- All packages pass.

## Failure Triage
1. `verify` fails:
   - Confirm upstream checkout path exists.
   - Re-run `--mode sync`, then `--mode verify`.
2. `suite` has failed scenarios:
   - Open `parity-summary.json`.
   - Inspect failing scenario IDs and manifests.
   - Re-run targeted tests in `./pkg/topology/engine/parity`.
3. Coverage totals regress:
   - Check `assertion-mapping.csv` uniqueness and status values.
   - Reconcile `not-applicable-approved.csv` with mapping rows using `not-applicable-approved`.
4. Determinism fails:
   - Re-run `suite` and compare generated summary files.
   - Check sort/order logic in parity result builders and adapters.
