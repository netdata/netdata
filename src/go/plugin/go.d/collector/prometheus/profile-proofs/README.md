<!-- markdownlint-disable MD013 MD043 -->

# Stock Prometheus profile proofs

This directory contains the durable authoring proof for stock Prometheus profiles whose source surface cannot be established
from one live endpoint. Each profile directory records the operator model, the source-to-profile reconciliation, the evidence
boundary, the exact committed-input hashes, and the authoritative validation result.

## Files

- `OPERATOR-MODEL.md` defines entities, capabilities, processing stages, identities, populations, units, and exclusions, then
  reconciles those decisions with the emitted profile.
- `SOURCE-INVENTORY.tsv` is the binding source-family and exact-selector ledger. Each row records its source evidence and final
  chart, job-exclusion, or writer-ineligible disposition.
- `EVIDENCE.md` identifies upstream revisions, source and documentation locations, feature gates, observed versus synthetic
  evidence, fixture provenance, and reproducible validation commands.
- `VALIDATION-JOB.yaml` is the sanitized structured job-policy input used by the objective validator and mirrors the
  recommended metadata example without endpoint or authentication settings.
- `SHA256SUMS.tsv` fingerprints the final profile, sanitized fixture inputs, metadata job-policy source, validation job,
  operator model, and source inventory. It intentionally does not hash itself or transient validator reports.
- `VALIDATION.md` states the authoritative PASS result, counts, loss boundary, and evidence limitations.

## Evidence boundary

The committed fixtures are sanitized structural unions. Mutually exclusive releases, roles, features, and exporter modes may
coexist solely to exercise every source-proven shape; such a fixture is not claimed to represent one realizable endpoint.
Private observations can confirm transport and local feature enablement, but they neither narrow the supported source surface
nor enter these public proof artifacts.

Unknown future families remain eligible for generic fallback. A zero-fallback result for the declared source union proves
current-source completeness; it is not a reason to close the profile against future exporter additions.
