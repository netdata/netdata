# Recipe: add a new go.d collector integration

This recipe assumes you are adding a brand-new go.d module
called `<name>`. For modifying an existing collector, see
`update-collector.md`.

## 0. Read first

- `<repo>/.agents/skills/project-writing-collectors/SKILL.md`
  -- the broader "how to write a collector" context (NIDL
  contexts, dashboard shaping, plugin landscape).
- `../SKILL.md` -- this skill's overview.
- `../schema-reference.md` -- the `collector.json` schema
  fields you will be filling in.

## 1. Create the module skeleton

Standard go.d layout:

```
src/go/plugin/go.d/collector/<name>/
├── <name>.go          # Module entrypoint, Init/Check/Collect
├── config.go          # Config struct
├── config_schema.json # DYNCFG schema
├── metadata.yaml      # Integration metadata (this skill's territory)
├── README.md          # Will become a symlink to integrations/<slug>.md once gen runs
├── testdata/          # Fixtures
└── ...other .go files
```

Plus stock conf:

```
src/go/plugin/go.d/config/go.d/<name>.conf
```

Plus alerts (if any):

```
src/health/health.d/<name>.conf
```

## 2. Author `metadata.yaml`

Use an existing rich collector as a template:
`src/go/plugin/go.d/collector/postgres/metadata.yaml`.

Required top-level fields per `collector.json`:

```yaml
plugin_name: go.d.plugin
modules:
  - meta:
      plugin_name: go.d.plugin
      module_name: <name>
      monitored_instance:
        name: "<Display Name>"
        link: "https://upstream-site.example/"
        categories:
          - data-collection.<category>     # see categories.yaml for valid ids
        icon_filename: "<name>.svg"
      keywords: [<keywords>]
      related_resources:
        integrations:
          list: []
      info_provided_to_referring_integrations:
        description: ""
    overview:
      data_collection:
        metrics_description: |
          First sentence: Monitor <thing> or collect <data> from <thing>.
          Add more detail after that only if it is useful on the full page.
        method_description: |
          One paragraph: how we collect it.
      supported_platforms:
        include: []
        exclude: []
      multi_instance: true
      additional_permissions:
        description: ""
      default_behavior:
        auto_detection:
          description: ""
        limits:
          description: ""
        performance_impact:
          description: ""
    setup:
      prerequisites:
        list: []
      configuration:
        file:
          name: "go.d/<name>.conf"
        options:
          description: ""
          folding:
            title: "Config options"
            enabled: true
          list: []
        examples:
          folding:
            title: "Config"
            enabled: true
          list:
            - name: "Basic"
              description: "Basic configuration."
              config: |
                jobs:
                  - name: local
                    url: http://localhost:1234
    troubleshooting:
      problems:
        list: []
    alerts: []
    metrics:
      folding:
        title: "Metrics"
        enabled: false
      description: ""
      availability: []
      scopes:
        - name: global   # will be auto-rewritten to "<Display Name> instance"
          description: ""
          labels: []
          metrics:
            - name: <name>.<context>
              description: <Chart title>
              unit: <unit>
              chart_type: line   # one of: line, area, stacked, heatmap
              dimensions:
                - name: <dim>
```

The first sentence of `metrics_description` is also used as the
description in generated catalog-style pages such as
`src/collectors/COLLECTORS.md`. Keep it product-facing and stable:
start with an action phrase, describe the integration, and do not
describe configuration variables, defaults, limits, or setup steps.
Put those details in the setup, default-behavior, examples, or
troubleshooting fields.

Hit every required field. The validator is strict (fatal on
warnings). Refer to `../schema-reference.md` for the
exhaustive field list.

## 3. Make sure `categories.yaml` has your category

If your `monitored_instance.categories` references a category
that doesn't exist in `integrations/categories.yaml`, the
validator will warn (fatal). Either pick an existing category
or add a new one under the appropriate parent (typically
`data-collection`).

## 4. Taxonomy, stock `.conf`, `config_schema.json`, alerts, README

These files are the rest of the collector consistency rule:

- `src/go/plugin/go.d/collector/<name>/taxonomy.yaml` --
  dashboard TOC placement for chart contexts. Static collectors
  use ordered `items:` trees; plain strings in structural `items:`
  own chart contexts. Dynamic collectors use `type: selector` with
  `context_prefix:` or `collect_plugin:` and matching
	  `metadata.yaml.metrics.dynamic_*` declarations. Display widgets
	  use `type: context` with `contexts:` and `chart_library`; those
	  referenced contexts must also be owned by structural items.
	  Pick `--section-id` from
	  `integrations/taxonomy/sections.yaml`; `section_id` is a stable
	  registry ID, not a path to invent in the collector file.
	  Seed the initial explicit context list with:
	  ```bash
	  python3 integrations/gen_taxonomy_seed.py src/go/plugin/go.d/collector/<name>/metadata.yaml --module-name <name> --section-id <section.id> --placement-id <name> --icon <icon>
	  ```
	  For a rich example with summary grids, table widgets, nested
	  groups, and ownership leaves, read
	  `src/go/plugin/go.d/collector/mysql/taxonomy.yaml`.
- `src/go/plugin/go.d/config/go.d/<name>.conf` -- the stock
  config users will see at
  `/etc/netdata/go.d/<name>.conf`. Keep it minimal but
  representative. Show every common option with a comment.
- `src/go/plugin/go.d/collector/<name>/config_schema.json` --
  the DYNCFG schema. Each option in the stock `.conf` should
  have a corresponding entry here, with the same default.
- `src/health/health.d/<name>.conf` -- alerts on the metrics
  declared in `metadata.yaml`. Each alert in this file should
  have a matching entry under `metadata.yaml.modules[0].alerts[]`.
- `src/go/plugin/go.d/collector/<name>/README.md` -- this is the
  USER-FACING documentation. After step 5, this file will be
  REPLACED with a symlink to
  `integrations/<slug>.md`. So you do NOT hand-write the
  README; the generator does. Stub it as empty initially.

## 5. Run the pipeline locally

From the repo root:

```bash
./integrations/pip.sh   # once
python3 integrations/gen_integrations.py
python3 integrations/gen_taxonomy.py --check-only
python3 integrations/gen_docs_integrations.py -c go.d/<name>
python3 integrations/gen_doc_collector_page.py
python3 integrations/gen_doc_secrets_page.py
```

Expected outputs:

- `integrations/integrations.js` and `integrations/integrations.json`
  regenerated (gitignored, do NOT commit them).
- Collector taxonomy validated. If `gen_taxonomy.py` fails, fix
  `taxonomy.yaml` or the matching `metadata.yaml.metrics.dynamic_*`
  declaration before continuing.
- `src/go/plugin/go.d/collector/<name>/integrations/<slug>.md`
  CREATED. Inspect: it should contain the `<!--startmeta`
  banner with your `sidebar_label` and `learn_rel_path`, then
  the rendered overview / setup / metrics / alerts /
  troubleshooting sections.
- `src/go/plugin/go.d/collector/<name>/README.md` becomes a
  symlink to `integrations/<slug>.md` (because there is
  exactly one integration in this directory).
- `src/collectors/COLLECTORS.md` updated to include your new
  collector in its category section.

If `gen_integrations.py` exits non-zero, read the warning
output -- a schema validation failed. Fix `metadata.yaml` and
re-run.

## 6. Verify locally

- Open the generated `integrations/<slug>.md` and make sure
  every section reads correctly.
- Open `src/collectors/COLLECTORS.md` and find your collector
  in the table.
- Run `python3 integrations/check_collector_taxonomy.py` before
  opening the PR. In CI this also runs with `--pr-diff` to enforce
  touched-collector taxonomy coverage.
- Run `git diff` and confirm the only changes are in:
  - `src/go/plugin/go.d/collector/<name>/...` (your new module
    files).
  - `src/go/plugin/go.d/collector/<name>/integrations/<slug>.md`
    (the generated integration page).
  - `src/go/plugin/go.d/collector/<name>/README.md` (now a
    symlink).
  - `src/collectors/COLLECTORS.md` (umbrella page updated).
  - `src/health/health.d/<name>.conf` (alerts file).
  - Possibly `integrations/categories.yaml` if you added a
    category.
  - NOT `integrations/integrations.js` or
    `integrations.json` (gitignored).
  - NOT `integrations/taxonomy.json` (gitignored).

## 7. Commit and push

Single PR, single commit (or a few logical commits) covering
the collector consistency rule plus the generated integration
page and umbrella update. Reviewers will check that affected
artifacts were updated together.

## 8. CI

- `check-markdown.yml` will run on the PR. It runs the same
  pipeline scripts, validates taxonomy, and validates Learn ingest. If your
  committed integration page diverges from CI's regen, the
  workflow fails -- fix locally and re-push.
- After merge, `generate-integrations.yml` triggers on master.
  Since you already committed the regen, this should not
  produce changes. If it does, the auto-PR
  (`Regenerate integrations docs`) catches the drift -- merge
  it.

## 9. Surface arrival timing

- `src/collectors/COLLECTORS.md` is live in the repo
  immediately after merge.
- The cloud-frontend dashboard's Integrations page rebuilds
  on its own schedule (when the cloud-frontend CI re-runs
  `gen_integrations.py` against master). Coordinate with
  the dashboard team if you need to know the exact next
  build.
- The Learn site's per-integration page lands within a few
  hours -- Learn's `ingest.yml` workflow runs every 3 hours
  (see the `learn-site-structure` skill for details).

## Common mistakes

- **Forgetting one collector-consistency artifact.** The most common
  cause of review feedback. Use `git status` after step 5 to
  confirm every affected source/generated artifact is staged.
- **Hand-editing `integrations/<slug>.md` after generation.**
  Never. It is regenerated each time. Edit `metadata.yaml`
  and re-run.
- **Skipping `gen_doc_collector_page.py`.** This forgets to
  update `src/collectors/COLLECTORS.md`, leaving your
  collector invisible in the umbrella table even though the
  per-integration page exists.
- **Categories typo.** A category id that doesn't match
  `categories.yaml` causes validation to fail (warnings are
  fatal). The renderer would silently fall back to
  `data-collection.applications` if your only declared
  category is bogus -- that fallback is itself the symptom
  of a typo, not the desired outcome.
- **Slug collision.** If `clean_string(meta.name)` produces
  the same slug as an existing collector in the same
  directory, one overwrites the other silently. Pick a
  unique enough display name.
