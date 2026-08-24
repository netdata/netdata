# Relate stock Prometheus profiles to integration metadata

Stock Prometheus profile YAML and collector `metadata.yaml` are separate source
catalogs. Adding a profile does not create an integration page, so every stock
profile contribution must make a deliberate catalog disposition.

## What each catalog controls

- **Profile catalog:** `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/*.yaml`
  controls runtime profile matching, app identity, profile-owned fallback and
  relabel policy, scoped autogen policy, and chart templates. The strict
  profile envelope is decoded by
  `src/go/plugin/go.d/collector/prometheus/promprofiles/profile.go`.
- **Integration catalog:** `metadata.yaml` controls public integration entries
  and generated documentation. Its top-level `profile_coverage.modules` mapping
  associates authored module IDs with primary stock profiles. Discovery uses the
  `*/metadata.yaml` pattern and enumerated collector roots.
- **Documentation projection:**
  `integrations/prometheus_profile_docs.py` joins each mapped profile's strict
  `PROFILE-DESIGN.yaml` with its runtime profile YAML. Generation fails if a
  stock profile is unmapped or if a semantic view and runtime chart differ. The
  public chart payload is an allowlist of context, title, family hierarchy,
  units, dimensions, selectors, and entity scope. A profile-design view's
  `question` remains internal authoring rationale and is not projected.
- **Generic profile configuration:** the Prometheus collector metadata documents
  the `profiles` job option
  (`src/go/plugin/go.d/collector/prometheus/metadata.yaml:175-203`). That is
  configuration documentation, not an application catalog generated from stock
  profile files.
- **Taxonomy:** the generic Prometheus collector opts out because endpoint
  contexts are dynamic
  (`src/go/plugin/go.d/collector/prometheus/taxonomy.yaml:1-5`). Adding an
  application metadata module does not make those runtime contexts stable.

`integrations/gen_integrations.py` imports collector entries through
`load_collectors`, projects profile coverage, and then renders the metadata
modules. Website consumes the clean native-`details` form; Agent Markdown is
generated from the rich form and retains the same `data-prometheus-profile-*`
hooks after `clean_and_write()` conversion.

## Required catalog disposition

For every stock profile:

1. Search existing integration metadata and generated pages for an equivalent
   application/endpoint.
2. Choose and record exactly one public-page disposition:
   - **Add/update a Prometheus application module** when no equivalent entry
     exists.
   - **Update or cross-link the existing first-class integration** when it
     already documents the same endpoint. Do not create a duplicate catalog
     entry.
   - **Keep generic-only documentation** only when an application-specific
     entry would be misleading, and record the concrete product reason.
3. Map every stock profile exactly once as a primary profile under the owning
   metadata document's top-level `profile_coverage.modules`. A module key is its
   authored `meta.id`; its value is the list of primary stock profile IDs.
   Supporting profiles are derived from `PROFILE-DESIGN.yaml`
   `composition.supports` and MUST NOT be repeated in integration metadata.
4. Keep runtime profile YAML limited to collection behavior. Put the
   operator-facing profile `title` and `summary` in the strict
   `PROFILE-DESIGN.yaml` `documentation` block, and put the human activation
   explanation on every support dependency's `activation` field. Keep each
   view's `question` as internal authoring rationale; do not expose it through
   integration metadata or generated pages.
5. Omit job `profiles` from application examples so they exercise default
   automatic selection. Do not copy proof-descriptor candidate/support
   composition into deployment configuration; exact proof composition isolates
   declared evidence dependencies, while an exact job makes every named support
   namespace mandatory.
6. Omit job `app` when automatic selection provides one unambiguous application
   identity. Keep it only for an intentional override, genuine app conflict, or
   an endpoint for which no selected profile supplies one.
7. Put a short operator-model brief in
   `overview.data_collection.metrics_description`. Summarize the entities,
   capabilities, and processing stages; do not copy the complete chart/family
   ledger.
8. Run the normal integration generators locally to validate the source. Do
   not commit generated Markdown in the source PR; after merge,
   `generate-integrations.yml` opens the separate generated-artifact PR. Never
   hand-edit generated integration pages.
9. Preserve the Prometheus taxonomy opt-out unless the runtime collector gains
   a real stable-context contract.

## Validation

- Validate the edited `metadata.yaml` through `integrations/gen_integrations.py`.
- Run `python3 -m unittest integrations.tests.test_prometheus_profile_docs` to
  enforce stock-profile reachability, exact view/chart/family parity, support
  projection, the public chart-field allowlist, absence of internal questions,
  inherited-YAML isolation, and generated markup hooks.
- Regenerate the application integration page and, when the catalog gains or
  loses an entry, `src/collectors/COLLECTORS.md`.
- Run `python3 integrations/gen_taxonomy.py --check-only`.
- Run the source-complete collector regression through the metadata job and
  assert the exact automatically selected application/support set plus the
  profile-derived application context.
- Confirm the gitignored `integrations/integrations.js`,
  `integrations/integrations.json`, and `integrations/taxonomy.json` are not
  staged or committed.
- Review the generated catalog sentence and operator-model brief as public
  product copy.

## How I figured this out

Files read:

- `../SKILL.md`
- `../pipeline.md`
- `../schema-reference.md`
- `integrations/_common.py`
- `integrations/gen_integrations.py`
- `integrations/gen_docs_integrations.py`
- `integrations/gen_taxonomy.py`
- `src/go/plugin/go.d/collector/prometheus/metadata.yaml`
- `src/go/plugin/go.d/collector/prometheus/promprofiles/profile.go`
- `src/go/plugin/go.d/collector/prometheus/taxonomy.yaml`

Commands run:

- `rg 'metadata.yaml|prometheus.profiles|taxonomy.yaml' integrations src/go/plugin/go.d/collector/prometheus`
- `python3 integrations/gen_integrations.py`
- `python3 integrations/gen_docs_integrations.py -c go.d.plugin/prometheus`
- `python3 integrations/gen_taxonomy.py --check-only`
- `git diff --check`
