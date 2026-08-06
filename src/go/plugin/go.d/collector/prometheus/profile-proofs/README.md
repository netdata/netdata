<!-- markdownlint-disable MD013 MD043 -->

# Stock Prometheus profile proofs

This directory contains compact proofs for stock Prometheus profiles whose supported source surface cannot be established
from one live endpoint. Bulky machine-readable evidence lives in [`netdata/testdata`](https://github.com/netdata/testdata)
under `prometheus/profiles/<revision>/`.

## Single-owner artifact contract

- `EVIDENCE.md` owns upstream revisions, source/documentation provenance, feature and configuration gates, evidence
  construction, and limitations. It MUST NOT duplicate replay commands, paths derived by the descriptor, or current results.
- `OPERATOR-MODEL.md` owns human semantic decisions: operator domains, entity identity, causal order, observation populations,
  unit algebra, and exclusion reasons. It MUST NOT copy the exact per-family inventory.
- `VALIDATION.md` explains how to interpret source-complete, supplemental, and job-policy cases. It MUST NOT duplicate their
  fixtures, verdicts, counts, findings, or per-profile commands.
- `VALIDATION-JOB.yaml` is the sanitized structured job-policy input. Its deployable fields mirror the recommended metadata
  example without endpoint, authentication, or profile-selection settings. Optional `future_inputs` are validation-only.
- `proof.yaml` is the machine assertion oracle. It owns the external revision and manifest digest, source-inventory
  expectations, metadata example/job identity, named replay cases, expected verdicts/counts/findings, and local integrity
  digests. Fixed paths are derived from profile/revision and are not repeated in the descriptor.
- External `SOURCE-INVENTORY.tsv` is the binding exact source-family/selector ledger. Each row records provenance, semantic
  classification, and its chart, job-exclusion, or writer-ineligible disposition.
- External `fixtures/*.prom` are sanitized replay inputs. External `manifest.yaml` authenticates the complete immutable
  evidence directory.

Tests discover `proof.yaml`; there is no separate registry. Replay checks every declared case, compares all expected machine
facts and finding codes, verifies inventory totals, and reconciles exact raw-family and authored-selector sets for the single
source-complete case.

## Evidence boundary

Structural-union fixtures may combine mutually exclusive releases, roles, features, and exporter modes solely to exercise
every source-proven shape. They are not claimed to represent one realizable endpoint.

“Sanitized” means fixtures are constructed from public source registrations with invented identities and values. Private
endpoints, credentials, deployment labels, and operating values are absent rather than copied through a universal token
replacement scheme.

Unknown future families remain eligible for generic fallback. Current source completeness does not close a profile against
future exporter additions.

## External testdata contract

- Tests use the latest `netdata/testdata` `master`, cloned into the ignored `src/go/testdata` directory. Netdata stores no
  testdata commit lock.
- A referenced external path is immutable. Evidence changes MUST add a new revision directory and update `proof.yaml`; they
  MUST NOT rewrite or delete a path referenced by a merged Netdata commit.
- Latest `master` is the transport. The pinned manifest digest, immutable paths, and per-file digests are the reproducibility
  boundary.
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
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py evidence-dirs
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py verify
```

From `src/go`, replay every declared case and the exact inventory reconciliation:

```bash
NETDATA_PROMETHEUS_TESTDATA_REQUIRED=1 go test -count=1 \
  ./internal/promprofilevalidation -run TestStockProfileProofsReplay
```

After deliberate changes to proof artifacts, refresh integrity metadata:

```bash
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py refresh
```

`refresh` verifies the external manifest and source-inventory expectations, then rewrites only descriptor integrity metadata.
It does not run the validator or accept changed expected behavior.
