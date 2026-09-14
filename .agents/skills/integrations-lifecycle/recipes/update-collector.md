# Recipe: update an existing collector integration

Use this when a collector's metrics, chart contexts, configuration, alerts, or documentation change. Read
`../consistency.md` first: it owns which artifacts move together and the delivery boundary.

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
(`../consistency.md`, "What reviewers should check"). An existing sibling `taxonomy.yaml` is dormant; leave it alone.

## 4. Validate locally

Interpreter and dependencies: `integrations/README.md` (look for `<repo>/.venv/` first).

```bash
python3 integrations/gen_integrations.py
python3 integrations/gen_docs_integrations.py --check
python3 -m unittest integrations.tests.test_descriptions integrations.tests.test_collector_metadata
```

Preview rendered prose with `../how-tos/preview-collector-page.md`. It keeps checkout pages and README files
untouched; generated documentation still arrives through the post-merge PR (`../consistency.md`).

## 5. Before opening the PR

Stage authoritative inputs and any required generated runtime outputs under `../consistency.md`. Its staged-artifact
check MUST print nothing; unrelated untracked reports do not need cleanup. Name
the post-merge regeneration PR as the delivery route for the generated pages in the PR description, and enumerate the
consistency artifacts you left unchanged with the reason.
