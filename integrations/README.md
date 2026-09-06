# Integrations pipeline

The scripts in this directory turn every `metadata.yaml` in the repository into the integrations catalog
(`integrations.js`, `integrations.json`), one documentation page per integration, and the umbrella pages under
`src/collectors/`. How the pipeline works, what is generated, and how changes are delivered is documented for
maintainers in `.agents/skills/integrations-lifecycle/`; this file is the local-run reference.

## Requirements

- Python (CI uses 3.13), run from the root of this repository: the page and umbrella generators open
  `integrations/integrations.js` and write their outputs by paths relative to the current directory. When the checkout
  has a `.venv/`, use `.venv/bin/python3`; the system interpreter usually lacks the packages below.
- The Python packages installed by `./integrations/pip.sh`: `jsonschema`, `referencing`, `jinja2`, `ruamel.yaml`, and
  `markdown-it-py`. All five are needed for generation (the description validator imports `markdown-it-py`); the same
  list is pinned in `packaging/cmake/Modules/NetdataRenderDocs.cmake` and the two must change together. Distribution
  packages work too: `apt-get install python3-jsonschema python3-referencing python3-jinja2 python3-ruamel.yaml`
  (Debian, Ubuntu), `apk add py3-jsonschema py3-referencing py3-jinja2 py3-ruamel.yaml` (Alpine), or
  `dnf install python3-jsonschema python3-referencing python3-jinja2 python3-ruamel-yaml` (Fedora, RHEL with EPEL), plus
  `markdown-it-py` from pip.
- Go (the version in `src/go/go.mod`) only when ibm.d module inputs changed, for `go generate`.

## Commands

Run in this order from the repository root. The producers (first two lines) matter only when their inputs or generators
changed; everything after them reads `integrations/integrations.js`, which is gitignored and produced by
`gen_integrations.py`.

```bash
python3 integrations/gen_npm_catalog.py                       # SNMP profiles -> npm-catalog/metadata.yaml
(cd src/go && go generate ./plugin/ibm.d/modules/...)         # ibm.d inputs -> metadata.yaml, README.md, config_schema.json
python3 integrations/gen_integrations.py                      # every metadata.yaml -> integrations.js / integrations.json
python3 integrations/gen_taxonomy.py --check-only             # validates taxonomy.yaml files (the taxonomy CI gate)
python3 integrations/check_collector_taxonomy.py --pr-diff master...HEAD   # what the PR gate will say; adjust the range
python3 -m unittest integrations.tests.test_taxonomy integrations.tests.test_descriptions \
    integrations.tests.test_prometheus_profile_docs integrations.tests.test_collector_metadata
python3 integrations/gen_docs_integrations.py                 # integrations.js -> one page per integration
python3 integrations/gen_doc_collector_page.py                # -> src/collectors/COLLECTORS.md
python3 integrations/gen_doc_secrets_page.py                  # -> src/collectors/SECRETS.md
python3 integrations/gen_doc_service_discovery_page.py        # -> src/collectors/SERVICE-DISCOVERY.md
```

`gen_docs_integrations.py -c <plugin>/<module>` regenerates one collector's page; `--check` validates the generated
page descriptions without writing. `integrations/check_collector_metadata.py` is a legacy script that no longer runs.

## What to commit

A source pull request commits the authoritative inputs only. Generated pages, generated README files, the umbrella
pages,
and generated metadata (ibm.d, NPM catalog) are validated locally and left unstaged; the post-merge workflow
`.github/workflows/generate-integrations.yml` regenerates them and opens the `integrations-regen` pull request. The
gitignored catalogs (`integrations.js`, `integrations.json`, `taxonomy.json`) and the untracked
`src/go/plugin/go.d/collector/snmp/npm-catalog/metrics-metadata-gaps.txt` report are never committed.

Pull requests run `.github/workflows/check-markdown.yml`, which regenerates everything, runs the tests above, runs the
taxonomy gate (a collector `metadata.yaml` that is added or whose metric contexts change must have a sibling
`taxonomy.yaml`; seed one with `integrations/gen_taxonomy_seed.py`), and validates the generated links through the
Learn ingest.
