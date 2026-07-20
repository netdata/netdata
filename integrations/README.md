To generate a copy of `integrations.js` and validate collector
taxonomy locally, you will need:

- Python 3.12 or newer.
- The following third-party Python modules:
    - `jsonschema`
    - `referencing`
    - `jinja2`
    - `ruamel.yaml`
    - `pydantic`
- A local checkout of https://github.com/netdata/netdata

The third-party dependencies can be covered by creating a Python virtual
environment and running the `pip.sh` script from this directory inside
that virtual environment.

Once the environment is set up, run the documentation generators from
the Agent repo root:

- `integrations/gen_integrations.py`
- `integrations/gen_taxonomy.py --check-only`
- `integrations/check_collector_taxonomy.py`
- `integrations/gen_docs_integrations.py`
- `integrations/gen_doc_collector_page.py`
- `integrations/gen_doc_secrets_page.py`

These scripts must be run _from this specific location_, as they use
their own path to figure out where all the files they need are.

Collector dashboard taxonomy is authored in sibling `taxonomy.yaml`
files next to collector `metadata.yaml` files. Static collectors use
ordered `items:` trees; a plain context string in `items:` owns that
chart context and normalizes to `type: owned_context`. Display widgets
use `type: context` with `contexts:` and `chart_library`, and every
referenced literal context must be owned somewhere in the structural
tree. Dynamic collectors use `type: selector` with `context_prefix:`
or `collect_plugin:` and must opt in from `metadata.yaml` with
`metrics.dynamic_context_prefixes:` or
`metrics.dynamic_collect_plugins:`; a taxonomy `context_prefix:` may
narrow a declared metadata namespace. The generated
`integrations/taxonomy.json` artifact is gitignored like
`integrations/integrations.js`.

To seed a static collector taxonomy from existing metadata contexts:

```bash
python3 integrations/gen_taxonomy_seed.py src/go/plugin/go.d/collector/apache/metadata.yaml --module-name apache --section-id applications.apache --placement-id apache --icon apache
```

Pull requests run `integrations/check_collector_taxonomy.py` from
`.github/workflows/check-markdown.yml`. The gate validates committed
taxonomy files and fails when a collector `metadata.yaml` metrics block
or `taxonomy.yaml` changes without matching taxonomy coverage.
