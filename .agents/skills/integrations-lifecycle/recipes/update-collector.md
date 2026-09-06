# Recipe: update an existing collector integration

Use this when a collector's metrics, chart contexts, configuration, alerts, or documentation change. Read
`../consistency.md` first: it owns which artifacts move together, the delivery boundary, and the taxonomy gate.

## 1. Identify what changed

From the collector directory, list the changed surfaces: runtime code; `metadata.yaml` metric contexts, units,
dimensions, setup, alerts; `config_schema.json`; the stock `.conf`; `health.d/*.conf`. The generated
`integrations/<slug>.md` and `README.md` symlink are outputs and follow automatically.

## 2. Update `metadata.yaml`

Metric rows mirror the code (context, chart title, unit, chart type, dimensions); option rows mirror the schema; alert
rows mirror the health conf. The content rules for every field are in `.agents/skills/collectors-metadata-yaml/`
(`metrics.md`, `setup.md`, `alerts-and-meta.md`); the catalog sentence and page description in
`../description-authoring.md`.

## 3. Update the remaining artifacts

Keep `config_schema.json`, the stock `.conf`, and `health.d/*.conf` synchronized with the behavior change
(`../consistency.md`, "What reviewers should check"). If you added, removed, or renamed metric contexts, reseed
`taxonomy.yaml` so the taxonomy gate passes (`../consistency.md`, "The collector taxonomy gate").

## 4. Validate locally

Dependencies and the complete command list: `integrations/README.md`. The scoped form:

```bash
python3 integrations/gen_integrations.py
python3 integrations/gen_taxonomy.py --check-only
python3 integrations/check_collector_taxonomy.py --pr-diff master...HEAD   # adjust the range to the PR base
python3 -m unittest integrations.tests.test_descriptions integrations.tests.test_collector_metadata
python3 integrations/gen_docs_integrations.py -c go.d.plugin/<module>
python3 integrations/gen_doc_collector_page.py
```

Do not skip the page generators: they validate the source locally even though their output stays unstaged. Run
`gen_doc_secrets_page.py` or `gen_doc_service_discovery_page.py` too when a secret store or discoverer changed.

## 5. Before opening the PR

Stage the source artifacts only. Run the gitignored-catalog check from `../consistency.md`; it MUST print nothing.
Name the post-merge regeneration PR as the delivery route for the generated pages in the PR description, and
enumerate the consistency artifacts you left unchanged with the reason.
