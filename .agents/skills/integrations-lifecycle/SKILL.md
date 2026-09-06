---
name: integrations-lifecycle
description: Netdata integrations pipeline reference. Use when editing metadata.yaml and needing to validate, regenerate, or deliver it; modifying integrations generators, schemas, templates, generated integration docs, integrations.js, integrations.json, COLLECTORS.md, SECRETS.md, or SERVICE-DISCOVERY.md; changing collector consistency artifacts; working with ibm.d contexts.yaml to metadata.yaml generation or the NPM catalog generator; or asked what the collector taxonomy.yaml files are.
---

# integrations-lifecycle

The pipeline that turns source metadata into the integration pages, the umbrella pages, and the in-app catalog. This
skill owns the mechanics and the delivery rules; what a collector `metadata.yaml` field should say is
`.agents/skills/collectors-metadata-yaml/`, and `config_schema.json` is
`.agents/skills/collectors-go-design/config-schema.md`.

Facts every reader needs:

- `metadata.yaml` is the input. Hand-authored for most integrations; generated for ibm.d modules (from `contexts.yaml`,
  `config.go`, `module.yaml`) and for the NPM catalog (from SNMP profiles by `integrations/gen_npm_catalog.py`). Edit
  the authoritative source, never a generated file.
- Every `<dir>/integrations/<slug>.md`, every README that is a symlink or whose `<!--startmeta` block carries a
  `message: "DO NOT EDIT..."` line (a `<!--startmeta` block alone is not the marker; hand-authored Learn pages have one
  too), and the umbrella pages `src/collectors/COLLECTORS.md`, `SECRETS.md`, `SERVICE-DISCOVERY.md` (no banner) are
  generated. Do not edit them.
- Source PRs carry sources; generated documentation arrives through the post-merge `integrations-regen` PR. Validate
  with `gen_integrations.py` and the tests; do not regenerate pages for the PR, and undo generated changes if you did.
  The one place this boundary is written out, with its ibm.d runtime-output exception, is `consistency.md`.
- The collector taxonomy (`taxonomy.yaml`, `gen_taxonomy.py`) is a dormant early implementation kept for later work;
  nothing runs it. Do not author, extend, or seed taxonomy content (`consistency.md`, "The dormant collector taxonomy").

## You edit metadata or a collector and need to validate and deliver

| Read | For |
|---|---|
| `consistency.md` | which artifacts move together, the delivery boundary, what CI enforces, the review checklist, the dormant taxonomy |
| `integrations/README.md` | dependencies and the command list for a local run |
| `description-authoring.md` | the catalog sentence and the generated page meta description (the two cross-type description contracts) |
| `recipes/add-go-collector.md`, `recipes/update-collector.md` | step-by-step for a new or changed go.d collector |
| `ibm-d.md` | the ibm.d generation chain |
| `how-tos/INDEX.md` | retiring a collector, tracing a missing published section, auditing Learn links, Prometheus profiles and the catalog |

## You change the generators, schemas, templates, or CI

| Read | For |
|---|---|
| `pipeline.md` | stages, sources per integration type, validation and rendering, outputs and banners, page locations and `learn_rel_path`, umbrella pages, both workflows |
| `schema-reference.md` | what each schema validates and the behavior the schema file does not show |
| `gotchas.md` | dead code, silent behaviors, validation traps, the MDX incident |
| `in-app-contract.md` | how cloud-frontend consumes `integrations.js` and what it treats as a contract |
| `how-tos/INDEX.md` | adding an integration type, the NPM catalog generator |

Citations in this skill name files and symbols, never line numbers; open the file and search for the symbol.

## Live how-to rule

When a concrete question about the pipeline needs non-trivial analysis (reading several scripts, running the pipeline,
cross-referencing schemas) and no file above answers it, the assistant MUST write `how-tos/<slug>.md` and add its row to
`how-tos/INDEX.md` before completing the task, so the next assistant does not repeat the analysis. When an existing
guide almost answers it, update that guide instead.

## Path discipline

`.agents/sensitive-data-discipline.md` applies: repository files repo-relative, sibling Netdata-org repositories as
`${NETDATA_REPOS_DIR}/<repo>/...`, no workstation paths.

## Related skills

- `collectors-authoring`: read first when authoring a brand-new collector; this skill covers the integration side.
- `collectors-metadata-yaml`: the words on the page, field by field.
- `collectors-go-design/config-schema.md`: the DynCfg form.
- `docs-learn-site-structure`: how the generated pages reach `learn.netdata.cloud` (`docs/.map/map.yaml`, ingest,
  redirects).
- `collectors-prometheus-profiles`: the profile side of the Prometheus catalog.
