# Build a source-derived synthetic Prometheus fixture

A source-derived fixture proves the structure and routing of metric surfaces
that one available deployment does not expose. It does not prove that those
surfaces are enabled, emitted, populated, or bounded the same way in a real
deployment.

## Define the supported surface

Record the target before generating samples:

- the observed application/exporter version and configuration;
- the exact source revision that produced it, or an explicit mismatch/unknown;
- the current upstream revision included in the profile's support scope;
- optional feature gates, connectors, modes, and mutually exclusive implementations;
- version aliases, replacements, and removed families.

“All metrics” means all source-defined writer-capable families in this declared
scope. It does not mean unknown future releases, downstream forks, or arbitrary
runtime-generated label values.

## Build the source contract

Use primary evidence first:

1. Find every metric registration in the pinned source revisions.
2. Follow update/observation callsites to establish population, labels, units, temporal behavior, and feature gates.
3. Read the matching documentation, release notes, and upstream tests/fixtures.
4. Search mirrored repositories and other monitoring integrations for missing
   fixtures, feature terminology, operator questions, and version differences.
5. Verify every borrowed name, type, label, and semantic claim against the target source or documentation.

Third-party dashboards and fixtures are recommended discovery evidence, not
exporter-contract authority. They can be stale, collapse labels, or compute a
different quantity from the title they display.

## Preserve fixture fidelity

Start from a sanitized transformation of an observed dump when one exists, then
add source-only families. A committed fixture MUST:

- preserve `HELP`, `TYPE`, exact family/sample names, and structural suffixes;
- preserve label keys and collision-relevant distinct label combinations;
- use stable synthetic aliases instead of operational label values;
- preserve histogram buckets, summary quantiles, state labels, and bounded enumerations from source;
- use valid Prometheus exposition and internally consistent distribution samples;
- include enough distinct identities/dimensions to exercise instance construction and normalization;
- identify source-only optional families and their gates in `SOURCE-SEMANTICS.yaml`;
- contain no credentials, private endpoints, customer/user identifiers, prompts,
  request content, or copied operational values.

Synthetic values MAY be zero when only structure is under test. Do not assert
those values as behavioral expectations. When non-zero histogram values are
needed, cumulative buckets MUST be monotonic, `_count` MUST agree with the
terminal bucket, and `_sum` MUST remain plausible for the observations.

## Represent incompatible surfaces honestly

One fixture MAY combine optional capabilities only when the source contract
proves they can coexist in one producer mode. Use separate realizable fixtures
and proof cases when combining surfaces would:

- double-count deprecated aliases and their replacements;
- merge incompatible label/type contracts;
- hide version-specific routing behavior;
- create an impossible identity collision that changes the planner result.

## Validate both proof classes

Keep observed and synthetic evidence separate:

- Use the private observed dump to prove actual names, label
  values/cardinality, and enabled-state behavior for that deployment.
- Use the union of source-derived realizable proof cases to prove every authored
  chart/dimension can materialize and every source-defined series is routed.
- Use focused collector regressions on representative observed shapes to prove
  the complete profile still initializes, matches, and collects when optional
  families are absent.
- When a feature-enabled real scrape is available, validate it as additional runtime evidence rather than replacing the
  source-complete structural fixture.

A partial real dump may report dead optional charts in the strict authoring
validator after the complete profile is added. That is an evidence limitation,
not a runtime collector failure. Never relabel a synthetic `PASS` as full
production validation.

## Record provenance

The proof contracts MUST record provenance without duplicating it:

- `SOURCE-SEMANTICS.yaml` records application/exporter revisions, authoritative public source locations, source
  environments, registrations, and update semantics. It is the source-backed contract; it does not classify a private
  scrape as observed versus synthetic.
- The optional generated source registry records mechanically extracted registrations and the checked upstream revision.
- `proof.yaml` names each sanitized replay fixture, declares its realizable environment, and records the independently
  authored expected replay outcome. Keep private observed dumps outside the proof bundle.

Coverage across those cases is source-complete; no individual case is relabeled
as a complete runtime deployment when the supported surface is mutually
exclusive.
