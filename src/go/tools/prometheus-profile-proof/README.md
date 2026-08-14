<!-- markdownlint-disable-file MD043 -->

# Prometheus profile proofs

This developer tool discovers every
`src/go/plugin/go.d/collector/prometheus/profile-proofs/*/proof.yaml` descriptor. It is the command-line adapter over
`internal/promprofile/proof` and `internal/promprofile/validation`; it contains no independent proof contract or validation
logic.

See the [canonical framework architecture](../../internal/promprofile/README.md) for repository ownership, package
dependencies, compilation, production replay, and semantic reconciliation. This document owns only the CLI and its
verification contract.

From `src/go`:

```text
go run ./tools/prometheus-profile-proof evidence-dirs --repo-root ../..
go run ./tools/prometheus-profile-proof verify --repo-root ../..
```

- `evidence-dirs` prints the sorted external directories needed for sparse checkout across the proof catalog.
- `verify` strictly loads the complete proof catalog and executes it against the latest testdata `master` checkout.

All commands accept `--profile <name>`. A targeted verification still compiles the complete proof catalog because support
composition is resolved from that catalog, but it replays and checks coverage only for the requested candidate profile.

The authoring-skill launcher `.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py` resolves repository and Go
tool paths, so it can be called from any directory.

## Verification contract

Verification joins four independently owned artifacts:

- local `OPERATOR-MODEL.md` for human rationale;
- local `PROFILE-DESIGN.yaml` for operator-facing semantics;
- local `proof.yaml` for realizable replay cases and independently authored expectations; and
- external `SOURCE-SEMANTICS.yaml`, with an optional generated source-registry pair, for upstream source semantics.

`verify` compiles every source and profile contract, resolves support composition, then replays each selected case
through the production Prometheus collector, relabeling, writer, profile selection, chart engine, and wire-normalization
paths. The compiled contract reconciles source, normalization, route, plan, observation, and wire facts and checks aggregate
declaration-bounded coverage after the last participating case. Full per-case snapshots are consumed sequentially rather
than retained for the whole catalog.

Expected validator or semantic failures use standalone fixture cases with `coverage: false`. Ordered `steps` are reserved
for persistent lifecycle evidence and every step expects `PASS`; an unexpected step failure aborts replay explicitly.

Fixtures are derived canonical paths under `prometheus/profiles/<profile>/fixtures/`. The descriptor does not pin
testdata content: the replay expectations and semantic coverage are the drift boundary against latest testdata `master`.

Every local proof directory must contain exactly `OPERATOR-MODEL.md`, `PROFILE-DESIGN.yaml`, and `proof.yaml`. Every external
directory must contain exactly `SOURCE-SEMANTICS.yaml`, descriptor-referenced fixtures, and either the complete generated
source-registry pair plus generator directory or no registry artifacts. Verification rejects stale, unexpected, and
unreferenced files instead of maintaining generated integrity metadata.
