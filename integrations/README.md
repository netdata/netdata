# Integrations pipeline

The scripts in this directory turn every `metadata.yaml` in the repository into the integrations catalog
(`integrations.js`, `integrations.json`), one documentation page per integration, and the umbrella pages under
`src/collectors/`. How the pipeline works, what is generated, and how changes are delivered is documented for
maintainers in `.agents/skills/integrations-lifecycle/`; this file is the local-run reference.

## Requirements

- Python (`check-markdown.yml` pins 3.13; the regeneration workflow uses the runner default), run from the root of this
  repository: the page and umbrella generators open `integrations/integrations.js` and write their outputs by paths
  relative to the current directory. Look for `<repo>/.venv/` first: when it exists it holds these dependencies, so run
  every command below with `.venv/bin/python3`. Only without it install the packages yourself.
- The Python packages installed by `./integrations/pip.sh`: `jsonschema`, `referencing`, `jinja2`, `ruamel.yaml`, and
  `markdown-it-py`. All five are needed for generation (the description validator imports `markdown-it-py`); the same
  list is pinned in `packaging/cmake/Modules/NetdataRenderDocs.cmake` and the two must change together. Distribution
  packages work too: `apt-get install python3-jsonschema python3-referencing python3-jinja2 python3-ruamel.yaml`
  (Debian, Ubuntu), `apk add py3-jsonschema py3-referencing py3-jinja2 py3-ruamel.yaml` (Alpine), or `dnf install
  python3-jsonschema python3-referencing python3-jinja2 python3-ruamel-yaml` (Fedora, RHEL with EPEL), plus
  `markdown-it-py` from pip.
- Go (the version in `src/go/go.mod`) only when ibm.d module inputs changed, for `go generate`.

## Commands

Validation is what a source change needs before its PR; regeneration of the tracked pages is optional and its output is
never committed (see "What to commit"). Run from the repository root. The producers (first two lines) matter only when
their inputs or generators changed; run that producer chain and its validation in an isolated source copy containing
current modified and new inputs, as described below. With current producer outputs, catalog validation alone writes
only the ignored catalogs in the checkout.

```bash
python3 integrations/gen_npm_catalog.py                       # SNMP profiles -> npm-catalog/metadata.yaml
(cd src/go && go generate ./plugin/ibm.d/modules/...)         # ibm.d inputs -> metadata.yaml, README.md, config_schema.json
python3 integrations/gen_integrations.py                      # validation: every metadata.yaml -> integrations.js / .json (gitignored)
python3 integrations/gen_docs_integrations.py --check         # validation: page descriptions, writes nothing
python3 -m unittest integrations.tests.test_descriptions integrations.tests.test_prometheus_profile_docs \
    integrations.tests.test_collector_metadata                 # what CI runs; test_collector_page_navigation is manual
```

For a selected collector page, use the isolated recipe in
[preview-collector-page](../.agents/skills/integrations-lifecycle/how-tos/preview-collector-page.md). It uses the current
catalog and writes pages under a fresh scratch directory, preserving checkout pages and READMEs. Full page/umbrella
regeneration and producer chains should run in an isolated source copy containing current modified and new inputs;
see `.agents/skills/integrations-lifecycle/pipeline.md` and `.agents/skills/integrations-lifecycle/ibm-d.md`.
Keep unrelated generated-file changes intact and unstaged; validation is not a reason to restore or delete them.

`integrations/check_collector_metadata.py` is a legacy script that no longer runs. `gen_taxonomy.py`,
`gen_taxonomy_seed.py`, `check_collector_taxonomy.py`, the `taxonomy.yaml` files, and `integrations/taxonomy/` are a
dormant collector-taxonomy prototype kept for later work; nothing runs them.

## What to commit

A source pull request includes authoritative inputs and the required generated runtime outputs: ibm.d
`contexts/zz_generated_contexts.go` and `config_schema.json` accompany the inputs that produced them. Both integration
workflows verify these runtime outputs after `go generate`.

Generated documentation follows the post-merge `integrations-regen` pull request: integration pages, generated README
files, umbrella pages, and producer-generated metadata (ibm.d and the NPM catalog). Preserve local generated-file
changes without staging them. The complete boundary is owned by
[delivery](../.agents/skills/integrations-lifecycle/consistency.md#delivery-boundary-source-pr-versus-post-merge-pr).
The gitignored catalogs (`integrations.js`, `integrations.json`, `taxonomy.json`) and the untracked
`src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt` report are never committed; they may remain
locally for inspection.

Pull requests run `.github/workflows/check-markdown.yml`, which regenerates everything, runs the tests above, and
validates the generated links through the Learn ingest.
