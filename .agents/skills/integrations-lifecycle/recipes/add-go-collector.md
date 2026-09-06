# Recipe: add a new go.d collector integration

This recipe covers the integration-artifact side of a brand-new go.d module called `<name>`. For modifying an existing
collector, see `update-collector.md`.

## 0. Read first

- `.agents/skills/collectors-authoring/SKILL.md`: the broader "how to write a collector" context.
- `src/go/plugin/go.d/docs/how-to-write-a-collector.md`: the framework V2 code and layout guide (new go.d collectors
  MUST use framework V2).
- `../consistency.md`: which artifacts move together and the delivery boundary.
- `.agents/skills/collectors-metadata-yaml/SKILL.md`: what every `metadata.yaml` field says. Write the file with it
  open.

## 1. The files a new collector adds

```
src/go/plugin/go.d/collector/<name>/
|-- collector.go, config.go, collect.go, metrix.go, write_metrics.go, charts.yaml, testdata/   # code
|-- config_schema.json   # DynCfg schema (collectors-go-design/config-schema.md)
|-- metadata.yaml        # integration metadata (this recipe)
`-- README.md            # generated later as a symlink; do not create it in the source PR
src/go/plugin/go.d/config/go.d/<name>.conf      # stock config
src/health/health.d/<name>.conf                 # alerts, if any
```

Also `src/go/plugin/go.d/collector/init.go` (registration import), `src/go/plugin/go.d/config/go.d.conf` (module
toggle), and `src/go/plugin/go.d/README.md` (collector list).

## 2. Author `metadata.yaml`

Copy a rich, current collector as the template (`src/go/plugin/go.d/collector/postgres/metadata.yaml`) and strip it. The
validator is `integrations/schemas/collector.json`; run `python3 integrations/gen_integrations.py` and read its output
rather than guessing which keys are required (every warning is fatal). `../schema-reference.md` lists the behavior the
schema file does not show; the facts an author hits most:

- `meta.monitored_instance.categories` must name ids from `integrations/categories.yaml`. An invalid id is dropped with
  a warning that fails the run; an empty list falls back to `data-collection.applications` (the only `collector_default:
  true` category). Add a new category under `data-collection` only when none fits.
- `meta.monitored_instance.name` drives the page slug (`clean_string`: lowercase, spaces to `_`, `/` to `-`, drop `(`,
  `)`, `:`), the sidebar label, and the integration id (`make_id`, which keeps case). Two names that clean to the same
  slug in one directory overwrite each other silently.
- A `metrics.scopes[].name` of `global` is rewritten to `<Display Name> instance` at render time; keep `global` in the
  source.
- The first sentence of `overview.data_collection.metrics_description` is the Monitor Anything catalog row
  (`../description-authoring.md`).

## 3. The other consistency artifacts

- Stock `.conf`: minimal but representative; show every common option with a comment.
- `config_schema.json`: each stock-conf option SHOULD have an entry with the same default; form rules in
  `.agents/skills/collectors-go-design/config-schema.md`.
- `health.d/<name>.conf`: each alert SHOULD have a matching row under `metadata.yaml` `alerts[]`.

## 4. Run the pipeline locally

Dependencies and the full command list: `integrations/README.md`. The scoped form for one collector:

```bash
python3 integrations/gen_integrations.py
python3 -m unittest integrations.tests.test_descriptions integrations.tests.test_collector_metadata
python3 integrations/gen_docs_integrations.py -c go.d.plugin/<name>
python3 integrations/gen_doc_collector_page.py
```

Expected: `integrations/integrations.js` and `integrations.json` regenerated (gitignored);
`src/go/plugin/go.d/collector/<name>/integrations/<slug>.md` created with the `<!--startmeta` banner, your
`sidebar_label`, and a `learn_rel_path` under `Collecting Metrics/Collectors/<category>`; `README.md` becomes a symlink
to it (the directory holds exactly one integration); `src/collectors/COLLECTORS.md` lists the collector in its section.
If `gen_integrations.py` exits non-zero, the warning names the file and the schema violation.

## 5. Verify

- Read the generated page and the `COLLECTORS.md` row; both are public copy.
- From `src/go`, run `timeout 15s go run ./cmd/godplugin -m <name> -d`. Success: the module registers, a job starts, and
  the command runs until the timeout stops it. `unknown module`, `no jobs started`, config-load errors, or an immediate
  exit are failures. Use `-c <config-dir>` for a non-standard config path.
- `git diff` touches only: the module directory, `init.go`, `go.d.conf`, the stock conf, the go.d README, the health
  conf, and possibly `integrations/categories.yaml`. No generated page, README symlink, umbrella page, or gitignored
  catalog (the `git status` check in `../consistency.md` MUST print nothing).

## 6. After merge

`generate-integrations.yml` opens the `integrations-regen` PR with the generated page, the README symlink, and the
umbrella pages (`../consistency.md`, "Delivery boundary"). The Learn page appears after Learn's next ingest run (the
`docs-learn-site-structure` skill); the in-app Integrations catalog rebuilds when cloud-frontend's own CI reruns
`gen_integrations.py` (`../in-app-contract.md`).

## Common mistakes

- Forgetting one consistency artifact: the most common review finding. Enumerate them in the PR description.
- Hand-editing `integrations/<slug>.md` after generation. Never. Edit `metadata.yaml` and regenerate.
- Skipping `gen_doc_collector_page.py` locally, so the collector's absence from `COLLECTORS.md` goes unnoticed until the
  regen PR.
- A category typo, or a display name whose slug collides with an existing collector (section 2).
