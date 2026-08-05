<!-- markdownlint-disable MD013 MD043 -->

# Stock Prometheus profile proofs

This directory contains the compact authoring proof for stock Prometheus profiles whose source surface cannot be established
from one live endpoint. Bulky machine-readable evidence lives in [`netdata/testdata`](https://github.com/netdata/testdata)
under `prometheus/profiles/<profile-revision>/`; each local proof pins that external evidence by manifest digest and byte
size.

## Files

- `OPERATOR-MODEL.md` defines entities, capabilities, processing stages, identities, populations, units, and exclusions, then
  reconciles those decisions with the emitted profile.
- `src/go/testdata/prometheus/profiles/<profile-revision>/SOURCE-INVENTORY.tsv` is the binding source-family and exact-selector ledger.
  Each row records its source evidence and final chart, job-exclusion, or writer-ineligible disposition.
- `EVIDENCE.md` identifies upstream revisions, source and documentation locations, feature gates, observed versus synthetic
  evidence, fixture provenance, and reproducible validation commands.
- `VALIDATION-JOB.yaml` is the sanitized structured job-policy input used by the objective validator. Its deployable fields
  mirror the recommended metadata example without endpoint, authentication, or profile-selection settings; optional
  `future_inputs` are validation-only raw probes and are not copied to collector metadata.
- `proof.yaml` is the one machine-readable descriptor. It identifies the profile, compact proof documents, metadata example
  and job, external revision/manifest/inventory/fixture, expected validator verdict/counts, and integrity digests. Tests and
  CI discover these files; no separate proof registry exists.
- The external manifest records the path, kind, byte size, and SHA-256 digest of every fixture and source inventory.
  `proof.yaml` pins that manifest and every compact proof input, but intentionally does not hash itself or transient reports.
- `VALIDATION.md` explains the result, loss boundary, producer-specific checks, and evidence limitations. Expected machine
  facts live only in `proof.yaml`; tests do not parse prose.

## Evidence boundary

The committed fixtures are sanitized structural unions. Mutually exclusive releases, roles, features, and exporter modes may
coexist solely to exercise every source-proven shape; such a fixture is not claimed to represent one realizable endpoint.
Private observations can confirm transport and local feature enablement, but they neither narrow the supported source surface
nor enter these public proof artifacts.

“Sanitized” does not mean that a live exposition was copied and replaced through a universal redaction-token mapping. The
fixtures are constructed from public source registrations and use invented, non-production identities and values; endpoint,
authentication, and live deployment-label values are absent rather than substituted. Each profile's `EVIDENCE.md` records
its source-versus-observation boundary and the synthetic placeholder convention used by its fixtures.

Unknown future families remain eligible for generic fallback. A zero-fallback result for the declared source union proves
current-source completeness; it is not a reason to close the profile against future exporter additions.

## External testdata contract

- Tests use the latest `netdata/testdata` `master`, cloned into the ignored `src/go/testdata` directory. No commit lock is
  stored in Netdata.
- A referenced external path is immutable. Evidence changes MUST add a new profile revision/directory and update the Netdata
  proof to reference its new manifest; they MUST NOT rewrite or delete a path used by a merged Netdata commit.
- Netdata verifies the pinned external manifest and every file declared by it before replaying a proof. Latest master is the
  transport; immutable paths and digests are the reproducibility boundary.
- Ordinary `go test` does not access the network. External-dependent tests skip when the checkout is absent; the dedicated
  Prometheus profile workflow sets `NETDATA_PROMETHEUS_TESTDATA_REQUIRED=1`, so missing or invalid evidence fails CI.

Local setup from the repository root:

```bash
git clone --depth=1 --branch master https://github.com/netdata/testdata.git src/go/testdata
```

For an existing checkout, fetch and detach at the latest remote `master` before replay. The local feature branches remain
unchanged:

```bash
git -C src/go/testdata fetch --depth=1 origin master
git -C src/go/testdata switch --detach FETCH_HEAD
```

Set `NETDATA_TESTDATA_DIR` to use a checkout elsewhere. Ordinary tests skip only when the checkout root is absent; a present
but incomplete or unreadable checkout fails.

From the repository root, use the descriptor-backed authoring command to inspect or refresh the integrity chain:

```bash
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py evidence-dirs
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py verify
.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py refresh
```

`refresh` rewrites only descriptor integrity metadata. Run the validator first and update the descriptor's expected facts
deliberately; integrity refresh is not validation and does not accept changed behavior by itself.
