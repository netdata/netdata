<!-- markdownlint-disable-file MD043 -->

# Prometheus profile proof descriptors

This developer tool discovers every
`src/go/plugin/go.d/collector/prometheus/profile-proofs/*/proof.yaml` descriptor.
It is the command-line adapter over `internal/promprofileproof`; it contains no
independent descriptor or integrity logic.

From `src/go`:

```text
go run ./tools/prometheus-profile-proof evidence-dirs --repo-root ../..
go run ./tools/prometheus-profile-proof verify --repo-root ../..
go run ./tools/prometheus-profile-proof refresh --repo-root ../..
```

- `evidence-dirs` prints the sorted external directories needed for sparse
  checkout. CI consumes this output directly.
- `verify` checks strict descriptor decoding, canonical paths, local artifact
  digests, the pinned external manifest, every file declared by that manifest,
  and directory/manifest completeness.
- `refresh` first validates every external manifest/file chain, then rewrites
  descriptor SHA-256 digests and byte counts using deterministic two-space YAML.
  It does not run the profile validator or change `validation.expected`.

The authoring-skill launcher
`.agents/skills/project-prometheus-profiles/scripts/proof-bundle.py` resolves the
repository and Go tool paths, so it can be called from any directory.

## Descriptor contract

`proof.yaml` version 1 contains:

- `profile`: stock profile name and repository-relative path;
- `proof`: repository-relative evidence, operator-model, and validation-summary
  documents in the descriptor directory;
- `external_evidence`: immutable testdata revision, manifest, source inventory,
  and authoritative replay fixture, with the manifest digest and byte count;
- `validation`: repository-relative job, matching metadata example/job identity,
  and the complete expected PASS/count record; and
- `integrity`: the sorted, exact set of profile/job/proof-document paths with
  their SHA-256 digests and byte counts.

Every proof directory must contain exactly one discovered descriptor. Profile
names/profile paths must be unique; all paths and integrity entries must be
canonical. External revisions may be shared when multiple profiles genuinely
consume the same immutable evidence directory. The descriptor intentionally
does not hash itself or transient validator output.

Run the exact validator replay and review any changed facts before editing
`validation.expected`. `refresh` is only integrity generation; using it cannot
turn an unreviewed behavior change into an accepted proof.
