# Tracing metadata sections missing from published integration pages

How do you identify where an integration section is lost when `metadata.yaml` contains it but Website or Learn does not
render it?

## Answer

Trace the section through each generated artifact before changing a downstream page template. Generated integration
pages have different delivery paths, even though both begin with Agent metadata.

### 1. Verify the Agent generator output

`integrations/gen_integrations.py` renders every section named in the type's `*_RENDER_KEYS` list (for collectors,
`COLLECTOR_RENDER_KEYS` includes `alerts`) through its Jinja template and stores the Markdown string on the integration
object (`render_collectors`). Run:

```bash
python3 integrations/gen_integrations.py
```

Inspect the matching object in `integrations/integrations.json` or `integrations/integrations.js`. The alerts template
(`integrations/templates/alerts.md`) produces either the alert table or the explicit empty-state sentence.

If the generated JSON has no alert table, investigate the Agent metadata, schema, filter, or template. Do not inspect
Website or Learn templates yet.

### 2. Verify the tracked Agent Markdown used by Learn

`integrations/gen_docs_integrations.py` (`build_readme_from_integration`) appends the rendered `alerts` section to the
collector page when it is present. Run:

```bash
python3 integrations/gen_docs_integrations.py
```

Inspect the collector's generated Markdown under its `integrations/` directory. The Agent workflow
`generate-integrations.yml` runs both generators and opens or updates the `integrations-regen` pull request.

At `netdata/learn @ 6bde65e850454be8778013a583ee6c96d1feb178`, Learn discovers files carrying the integration marker and
reads their generated metadata (`ingest/ingest.py`, the integration-marker discovery and `populate_integrations` path).
Published integration files are then copied and sanitized without reconstructing their body sections. Therefore:

- If the Agent Markdown is stale, merge or update the Agent regeneration pull request first.
- If the Agent Markdown is correct but Learn is stale, run the normal Learn ingest workflow.
- Never hand-edit the generated Agent or Learn Markdown.

### 3. Verify the Website data refresh

At `netdata/website @ db6ed7b907a8d9f833450bfbbc24fc10eb4c9e8e`, Website does not consume the tracked Agent Markdown.
Its `update-integrations.yml` workflow checks out Agent `master`, runs `gen_integrations.py`, and copies
`integrations.json` into Website data; `scripts/build_integrations_md_files.py` then creates integration page shells
from that data. The template `themes/tailwind/layouts/partials/integration-tabs.html` creates an Alerts tab whenever the
integration object contains a non-empty `alerts` value. Therefore:

- If Agent JSON is correct but Website data is stale, run the normal Website integration update workflow.
- If Website data contains the alert table but the rendered page omits it, inspect the Website template and rendered
  HTML.
- Never hand-edit generated Website integration data or page shells.

### 4. Validate the complete delivery chain

For an alerts omission, verify all of these independently:

1. The source `metadata.yaml` contains the intended operator-facing alerts.
2. Generated `integrations.json` contains the rendered alert table.
3. Generated Agent Markdown contains the same alert table.
4. The Agent regeneration pull request is based on the current source revision.
5. Website data regeneration and Learn ingestion ran after the Agent source and generated artifacts were merged.
6. The rendered Website and Learn pages contain a known alert name.

This sequence distinguishes a source or generator defect from a stale delivery artifact. A missing public section is not
evidence of a template defect until the downstream input has been shown to contain that section.

## How I figured this out

Files read: `integrations/gen_integrations.py`, `integrations/templates/alerts.md`,
`integrations/gen_docs_integrations.py`, `.github/workflows/generate-integrations.yml`; in `netdata/website @
db6ed7b907a8d9f833450bfbbc24fc10eb4c9e8e`: `.github/workflows/update-integrations.yml`,
`scripts/build_integrations_md_files.py`, `themes/tailwind/layouts/partials/integration-tabs.html`; in `netdata/learn @
6bde65e850454be8778013a583ee6c96d1feb178`: `ingest/ingest.py`.

```bash
rg -n "COLLECTOR_RENDER_KEYS|render_collectors|integration.get\(\"alerts\"\)" integrations
rg -n "gen_integrations.py|gen_docs_integrations.py|integrations-regen" .github/workflows/generate-integrations.yml
rg -n "populate_integrations|copy_doc|sanitize_page|INTEGRATION_MARKER" <learn-repo>/ingest/ingest.py
rg -n "gen_integrations.py|integration.alerts|render_integration" <website-repo>/.github <website-repo>/scripts <website-repo>/themes
```
