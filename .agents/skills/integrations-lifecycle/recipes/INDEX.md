# Recipes -- INDEX

Step-by-step recipes for the most common integration-lifecycle
tasks a maintainer (or AI assistant) performs.

| Recipe | When to use |
|---|---|
| `add-go-collector.md` | Adding a new go.d module from scratch (most common case). |
| `update-collector.md` | Modifying an existing collector's metrics, config, alerts, or docs. |
| `add-ibm-d-module.md` | Adding a new ibm.d module (where `metadata.yaml` is generated). |
| `add-cloud-notification.md` | Adding a new cloud-notification destination. |
| `add-secretstore-backend.md` | Adding a new secretstore backend. |
| `add-service-discovery.md` | Adding a new service-discovery discoverer. |
| `regenerate-locally.md` | Running the full pipeline locally to verify your changes before pushing. |

## Common preamble for all recipes

Every recipe assumes you are in the agent repo root. Install
the Python deps once per machine:

```bash
./integrations/pip.sh
```

That installs `jsonschema`, `referencing`, `jinja2`,
`ruamel.yaml` -- the four packages `gen_integrations.py`
needs.

For fast iteration during development, prefer `-c plugin/module`
scoping on `gen_docs_integrations.py` to skip cleaning/
regenerating other directories:

```bash
python3 integrations/gen_docs_integrations.py -c go.d/<your-module>
```

## When in doubt

1. Read `pipeline.md` for the end-to-end flow.
2. Read `schema-reference.md` for the exact field your
   `metadata.yaml` change needs.
3. Read `consistency.md` for the collector consistency rule.
4. Read `gotchas.md` BEFORE assuming the pipeline does the
   obvious thing.
5. If you encountered a question that this catalog doesn't
   cover and you had to investigate to answer it, AUTHOR a
   how-to under `../how-tos/<slug>.md` and add it to
   `../how-tos/INDEX.md`. This rule is mandatory; see
   `../SKILL.md` "Live how-to rule".
