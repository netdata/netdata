<!-- markdownlint-disable-file MD043 -->

# Prometheus profile proof descriptors

This developer tool discovers every
`src/go/plugin/go.d/collector/prometheus/profile-proofs/*/proof.yaml` descriptor. It is the command-line adapter over
`internal/promprofileproof`; it contains no independent descriptor, inventory, or integrity logic.

From `src/go`:

```text
go run ./tools/prometheus-profile-proof evidence-dirs --repo-root ../..
go run ./tools/prometheus-profile-proof verify --repo-root ../..
go run ./tools/prometheus-profile-proof refresh --repo-root ../..
```

- `evidence-dirs` prints the sorted external directories needed for sparse checkout.
- `verify` checks strict descriptor decoding, derived canonical paths, local integrity, exact external directory contents,
  strict source-inventory structure, and declared inventory totals against latest testdata `master`.
- `refresh` verifies the same external layout and inventory contract, then rewrites local integrity digests and byte counts
  using deterministic two-space YAML. It does not run validation or change replay expectations.

The authoring-skill launcher `.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py` resolves repository and Go
tool paths, so it can be called from any directory.

## Descriptor contract

`proof.yaml` version 3 contains:

- `profile`: stock profile name; the proof directory and stock profile path are derived from it;
- optional `supporting_profiles`: the explicit stock profiles composed with the candidate, including their local integrity
  digests; the external directory is always derived as `prometheus/profiles/<profile>`;
- `source_inventory`: expected row, unique source-family, unique authored-selector, and disposition totals;
- `validation.metadata_example`: the collector metadata example and job identity that `VALIDATION-JOB.yaml` must mirror;
- `validation.cases`: one named source-complete case plus optional supplemental cases, each with a derived fixture path, job
  mode, optional composition mode, expected verdict, complete count record, and error/warning finding-code counts; and
- `integrity`: digests and byte counts for the fixed evidence, operator-model, validation-job, validation-summary, and stock
  profile artifacts.

Every proof directory must contain exactly the fixed artifact set and one descriptor. Profile names and derived profile paths
must be unique. Each descriptor has exactly one `source_complete` case and that case must expect `PASS`. Supplemental cases
may intentionally expect `FAIL` when they prove partial-source or policy boundaries.

Case composition defaults to `declared`: the candidate plus every descriptor-declared supporting profile. A source-complete
case MUST use that full composition. A supplemental case whose partial fixture intentionally omits every supporting-profile
namespace MAY set `composition: candidate_only`; this keeps candidate partial-surface diagnostics executable without adding
unrelated synthetic evidence. Candidate-only mode is invalid when the descriptor declares no supporting profile.

The descriptor intentionally does not hash itself or transient validator output. Replay tests are the execution oracle: they
compare all declared counts/findings and reconcile the source-complete report's exact raw-family and authored-selector sets
against the inventory.

External evidence is latest-only working data, not a historical content pin. Update the stable testdata profile directory
and its Netdata proof expectations together. Run the exact replay and review changed facts before editing a validation case;
`refresh` is local-integrity generation only and cannot accept changed behavior.
