# Relate stock Prometheus profiles to integration metadata

Stock Prometheus profile YAML and collector `metadata.yaml` are separate source catalogs. Adding a profile does not
create an integration page, so every stock profile contribution must make a deliberate catalog disposition. The
profile-side rules (the `profile_coverage.modules` mapping, `PROFILE-DESIGN.yaml` ownership of `title`/`summary`,
omitting `profiles` and `app` from stock examples) are owned by
`.agents/skills/collectors-prometheus-profiles/SKILL.md`; this file covers the integration-catalog side only.

## What each catalog controls

- **Profile catalog:** `src/go/plugin/go.d/config/go.d/prometheus.profiles/default/*.yaml` controls runtime profile
  matching, app identity, fallback and relabel policy, and chart templates.
- **Integration catalog:** `metadata.yaml` controls public integration entries and generated documentation. Its
  top-level `profile_coverage.modules` mapping (allowed only for the `go.d.plugin/prometheus` metadata file) associates
  authored module ids with primary stock profiles; `_common.py` warns on unknown module ids.
- **Documentation projection:** `integrations/prometheus_profile_docs.py` joins each mapped profile's
  `PROFILE-DESIGN.yaml` with its runtime YAML and attaches a detached `metrics.profile_coverage` projection to the
  module after schema validation. Generation fails if a stock profile is unmapped or a semantic view and runtime chart
  differ. The rows are grouped by the profile's top-level family (Prometheus metric, Netdata family and chart title,
  dimension, unit, entity scope); a view's `question` stays internal. Do not author `metrics.profile_coverage` inside a
  module: YAML merge anchors would share the nested mapping between inherited modules.
- **Generic profile configuration:** the Prometheus collector metadata documents the `profiles` job option; that is
  configuration documentation, not an application catalog.

## Required catalog disposition

For every stock profile:

1. Search existing integration metadata and generated pages for an equivalent application or endpoint.
2. Choose and record exactly one public-page disposition: add or update a Prometheus application module when no
   equivalent entry exists; update or cross-link the existing first-class integration when it already documents the same
   endpoint (never a duplicate catalog entry); or keep generic-only documentation, recording the concrete product
   reason.
3. Put a short operator-model brief in `overview.data_collection.metrics_description`: entities, capabilities,
   processing stages. Do not copy the chart and family ledger; the projection renders it.
4. Run the generators locally and deliver per `../consistency.md`.

## Validation

- `python3 integrations/gen_integrations.py` validates the edited `metadata.yaml` and runs the projection.
- `python3 -m unittest integrations.tests.test_prometheus_profile_docs` enforces stock-profile reachability, view/chart/
  family parity, support projection, the public chart-field allowlist, absence of internal questions, inherited-YAML
  isolation, complete metric mappings, and grouped table output. Both integration workflows run it.
- Review the catalog sentence and operator-model brief as public product copy; if you regenerate the page or
  `src/collectors/COLLECTORS.md` to read them, undo those tracked changes before committing.
