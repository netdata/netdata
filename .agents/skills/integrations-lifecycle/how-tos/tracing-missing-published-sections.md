# Tracing metadata sections missing from published integration pages

How do you identify where an integration section is lost when `metadata.yaml` contains it but Website or Learn does not render it?

## Answer

Trace the section through each generated artifact before changing a downstream page template. Generated integration pages have different delivery paths, even though both begin with Agent metadata.

### 1. Verify the Agent generator output

The integration generator explicitly includes `alerts` among the rendered collector sections (`integrations/gen_integrations.py:69-77`). For every collector, it renders each supported section through its Jinja template and writes the resulting Markdown string into the integration object (`integrations/gen_integrations.py:859-875`).

Run:

```bash
python3 integrations/gen_integrations.py
```

Inspect the matching object in `integrations/integrations.json` or `integrations/integrations.js`. The alerts template produces either the operator-facing alert table or the explicit empty-state sentence (`integrations/templates/alerts.md:1-14`).

If the generated JSON has no alert table, investigate the Agent metadata, schema, filter, or template. Do not inspect Website or Learn templates yet.

### 2. Verify the tracked Agent Markdown used by Learn

The documentation generator appends the rendered `alerts` section to collector Markdown when it is present (`integrations/gen_docs_integrations.py:275-312`). Run:

```bash
python3 integrations/gen_docs_integrations.py
```

Inspect the collector's generated Markdown under its `integrations/` directory. The Agent workflow runs both generators and opens or updates the `integrations-regen` pull request (`.github/workflows/generate-integrations.yml:96-153`).

At `netdata/learn @ 6bde65e850454be8778013a583ee6c96d1feb178`, Learn discovers files carrying the integration marker and reads their generated metadata (`ingest/ingest.py:1249-1298`). Published integration files are then copied and sanitized without reconstructing their body sections (`ingest/ingest.py:3977-3986`). Therefore:

- If the Agent Markdown is stale, merge or update the Agent regeneration pull request first.
- If the Agent Markdown is correct but Learn is stale, run the normal Learn ingest workflow.
- Never hand-edit the generated Agent or Learn Markdown.

### 3. Verify the Website data refresh

At `netdata/website @ db6ed7b907a8d9f833450bfbbc24fc10eb4c9e8e`, Website does not consume the tracked Agent Markdown. Its update workflow checks out Agent `master`, runs `gen_integrations.py`, and copies `integrations.json` into Website data (`.github/workflows/update-integrations.yml:27-64`). Website then creates integration page shells from that data (`scripts/build_integrations_md_files.py:889-927`).

The Website integration template already creates an Alerts tab whenever the integration object contains a non-empty `alerts` value (`themes/tailwind/layouts/partials/integration-tabs.html:16-20`). Therefore:

- If Agent JSON is correct but Website data is stale, run the normal Website integration update workflow.
- If Website data contains the alert table but the rendered page omits it, then inspect the Website template and rendered HTML.
- Never hand-edit generated Website integration data or page shells.

### 4. Validate the complete delivery chain

For an alerts omission, verify all of these independently:

1. The source `metadata.yaml` contains the intended operator-facing alerts.
2. Generated `integrations.json` contains the rendered alert table.
3. Generated Agent Markdown contains the same alert table.
4. The Agent regeneration pull request is based on the current source revision.
5. Website data regeneration and Learn ingestion ran after the Agent source and generated artifacts were merged.
6. The rendered Website and Learn pages contain a known alert name.

This sequence distinguishes a source or generator defect from a stale delivery artifact. A missing public section is not evidence of a template defect until the downstream input has been shown to contain that section.

## How I figured this out

Files read:

- `integrations/gen_integrations.py`
- `integrations/templates/alerts.md`
- `integrations/gen_docs_integrations.py`
- `.github/workflows/generate-integrations.yml`
- `netdata/website @ db6ed7b907a8d9f833450bfbbc24fc10eb4c9e8e`: `.github/workflows/update-integrations.yml`, `scripts/build_integrations_md_files.py`, and `themes/tailwind/layouts/partials/integration-tabs.html`
- `netdata/learn @ 6bde65e850454be8778013a583ee6c96d1feb178`: `ingest/ingest.py`

Commands used:

```bash
rg -n "COLLECTOR_RENDER_KEYS|render_collectors|integration.get\(\"alerts\"\)" integrations
rg -n "gen_integrations.py|gen_docs_integrations.py|integrations-regen" .github/workflows/generate-integrations.yml
rg -n "populate_integrations|copy_doc|sanitize_page|INTEGRATION_MARKER" <learn-repo>/ingest/ingest.py
rg -n "gen_integrations.py|integration.alerts|render_integration" <website-repo>/.github <website-repo>/scripts <website-repo>/themes
```
