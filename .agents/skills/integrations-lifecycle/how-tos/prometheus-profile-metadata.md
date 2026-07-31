# Relate stock Prometheus profiles to integration metadata

Stock Prometheus profile YAML and collector `metadata.yaml` are separate source
catalogs. Adding a profile does not create an integration page, so every stock
profile contribution must make a deliberate catalog disposition.

## What each catalog controls

- **Profile catalog:** `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/*.yaml`
  controls runtime profile matching, scoped autogen policy, and chart templates.
  The strict profile header accepts only `match`, `app`, `autogen`, and
  `template` (`src/go/plugin/go.d/collector/prometheus/promprofiles/profile.go:19-29`,
  `:133-161`).
- **Integration catalog:** `metadata.yaml` controls public integration entries
  and generated documentation. Discovery uses the `*/metadata.yaml` pattern and
  enumerated collector roots (`integrations/_common.py:16-29`, `:96-111`,
  `:136-162`).
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
`load_collectors` (`integrations/gen_integrations.py:10-23`) and renders the
metadata modules (`:740-756`). It never scans the profile directory.

## Required catalog disposition

For every stock profile:

1. Search existing integration metadata and generated pages for an equivalent
   application/endpoint.
2. Choose and record exactly one disposition:
   - **Add/update a Prometheus application module** when no equivalent entry
     exists.
   - **Update or cross-link the existing first-class integration** when it
     already documents the same endpoint. Do not create a duplicate catalog
     entry.
   - **Keep generic-only documentation** only when an application-specific
     entry would be misleading, and record the concrete product reason.
3. Keep collector `metadata.yaml` authoritative. Do not add documentation keys
   to the strict runtime profile envelope.
4. Put a short operator-model brief in
   `overview.data_collection.metrics_description`. Summarize the entities,
   capabilities, and processing stages; do not copy the complete chart/family
   ledger.
5. Run the normal integration generators and commit the required generated
   Markdown. Do not hand-edit generated integration pages.
6. Preserve the Prometheus taxonomy opt-out unless the runtime collector gains
   a real stable-context contract.

## Validation

- Validate the edited `metadata.yaml` through `integrations/gen_integrations.py`.
- Regenerate the application integration page and, when the catalog gains or
  loses an entry, `src/collectors/COLLECTORS.md`.
- Run `python3 integrations/gen_taxonomy.py --check-only`.
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
