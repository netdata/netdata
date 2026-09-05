<!-- markdownlint-disable MD013 MD043 -->

# Stock Prometheus profile proofs

This directory contains compact proofs for stock Prometheus profiles whose supported source surface cannot be established
from one live endpoint. Bulky machine-readable evidence lives in [`netdata/testdata`](https://github.com/netdata/testdata)
under `prometheus/profiles/<profile>/`.

The cross-repository authority model, package boundaries, and end-to-end compile/replay flow are defined in the
[canonical framework architecture](../../../../../internal/promprofile/README.md).

## Artifact contract

- `OPERATOR-MODEL.md` explains human rationale: operator domains, entity grain, causal order, reduction choices, and
  exclusion reasons. It MUST NOT duplicate exact source declarations, routes, or replay results.
- `PROFILE-DESIGN.yaml` owns the strict operator-facing semantic contract: composition, entities, label treatment,
  normalizations, exclusions, limitations, views, reduction, units, and presentation intent.
- `proof.yaml` owns realizable replay cases, explicit source environments, independently authored expected outcomes,
  metadata-example identity, future inputs, and coverage participation. It does not store generated counts or digests.
  Its initial strict schema is identified by `version: v1`.
  Expected `FAIL` belongs to a standalone fixture case with `coverage: false`; ordered lifecycle steps expect `PASS`.
- External `SOURCE-SEMANTICS.yaml` owns pinned upstream evidence, source environments, registrations/signals, components,
  labels, contributor models, relationships, state encodings, and source exclusions.
- External `SOURCE-REGISTRY.yaml` and `SOURCE-REGISTRY.generator.yaml`, when present, are an inseparable pair. The committed
  generator inputs and output own a mechanically derived registration surface without making generated groupings semantic.
- External `fixtures/*.prom` are sanitized realizable replay inputs. Every consumed fixture is named by `proof.yaml`.

Each local proof directory contains exactly those three files. Each external directory contains `SOURCE-SEMANTICS.yaml`,
the descriptor-referenced fixtures, and either both source-registry files plus their generator directory or none of them.
Unexpected and unreferenced artifacts fail verification.

Tests discover `proof.yaml`; there is no separate registry. The compiler resolves the candidate's declared support closure,
selects profiles automatically through production matching, reconciles every source/normalization/route/plan/wire fact, and
aggregates declaration-bounded semantic coverage across the cases that explicitly participate.

## Evidence boundary

Fixtures MUST represent a realizable producer mode. Mutually exclusive modes use separate cases; optional
capabilities MAY be combined only when the source contract permits them to coexist.

“Sanitized” means fixtures are constructed from public source registrations with invented identities and values. Private
endpoints, credentials, deployment labels, and operating values are absent rather than copied through a universal token
replacement scheme.

Unknown future families remain eligible for generic fallback. Current source completeness does not close a profile against
future exporter additions.

## External testdata contract

- Tests use the latest `netdata/testdata` `master`, cloned into the ignored `src/go/testdata` directory. Netdata stores no
  testdata commit or content lock.
- Each profile uses one stable `prometheus/profiles/<profile>/` directory. Update that directory and the corresponding
  Netdata proof expectations together when exporter coverage or validation behavior changes.
- Historical Netdata checkouts are not guaranteed to validate against later testdata master content. Exact replay and
  source-contract reconciliation are the drift boundary for the current validator.
- Ordinary tests do not access the network. External-dependent tests skip when the checkout is absent; required CI sets
  `NETDATA_PROMETHEUS_TESTDATA_REQUIRED=1`, so missing or invalid evidence fails.

Local setup from the repository root:

```bash
git clone --depth=1 --branch master https://github.com/netdata/testdata.git src/go/testdata
```

Update an existing checkout without changing its local branches:

```bash
git -C src/go/testdata fetch --depth=1 origin master
git -C src/go/testdata switch --detach FETCH_HEAD
```

`NETDATA_TESTDATA_DIR` may point to a checkout elsewhere.

## Verification workflow

From the repository root:

```bash
.agents/skills/collectors-prometheus-profiles/scripts/proof-bundle.py evidence-dirs
.agents/skills/collectors-prometheus-profiles/scripts/proof-bundle.py verify
```

From `src/go`, replay every declared case and its semantic reconciliation:

```bash
NETDATA_PROMETHEUS_TESTDATA_REQUIRED=1 go test -count=1 \
  ./internal/promprofile/validation -run TestStockProfileProofsReplay
```
