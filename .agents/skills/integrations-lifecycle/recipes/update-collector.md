# Recipe: update an existing collector integration

Use this when a collector's metrics, chart contexts, configuration,
alerts, or generated docs change. The goal is to keep runtime behavior,
metadata, taxonomy, docs, and CI validation in one coherent PR.

## 0. Read first

- `../SKILL.md` -- integrations lifecycle overview.
- `../consistency.md` -- what the collector consistency rule requires
  and what CI enforces.
- `../schema-reference.md` -- exact `metadata.yaml` and
  `taxonomy.yaml` fields.

## 1. Identify what changed

From the collector directory, list the changed surfaces:

- runtime `.go` / script code;
- `metadata.yaml` metric contexts, units, dimensions, setup, alerts;
- `taxonomy.yaml` dashboard TOC placement;
- `config_schema.json`;
- stock `.conf`;
- `health.d/*.conf`;
- generated `integrations/<slug>.md` and `README.md` symlink.

If chart contexts are added, removed, renamed, or moved between dynamic
and static emission, update `taxonomy.yaml` in the same PR.

## 2. Update `metadata.yaml`

Keep `metrics.scopes[].metrics[].name` aligned with the collector's
actual emitted chart contexts. Keep units and descriptions aligned with
the code. If a collector emits runtime-only dynamic contexts, declare
the guardrail in metadata:

```yaml
metrics:
  dynamic_context_prefixes:
    - prefix: snmp.
      reason: SNMP profiles emit device-specific contexts at runtime.
```

Use `dynamic_collect_plugins` only when a stable context-name prefix is
not available.

## 3. Update `taxonomy.yaml`

Check whether the existing taxonomy still owns every static context
exactly once:

```bash
python3 integrations/gen_taxonomy.py --check-only
```

Rules of thumb:

- plain strings in structural `items:` own contexts;
- `type: context` widgets reference contexts but do not own them;
- every literal widget reference must be owned elsewhere or carry an
  explicit `unresolved` escape hatch;
- dynamic collectors use `type: selector` with declared
  `context_prefix:` or `collect_plugin:`;
- pick section IDs from `integrations/taxonomy/sections.yaml`.

For a rich reference, compare against
`src/go/plugin/go.d/collector/mysql/taxonomy.yaml`.

## 4. Update the remaining collector artifacts

Keep these synchronized when the corresponding behavior changes:

- `config_schema.json` for dynamic configuration;
- stock `.conf` for user-visible defaults;
- `health.d/*.conf` and `metadata.yaml.modules[].alerts[]`;
- generated docs via the integrations pipeline.

Do not hand-edit generated `integrations/<slug>.md` files.

## 5. Run local validation

From the repo root:

```bash
python3 integrations/gen_integrations.py
python3 integrations/gen_taxonomy.py --check-only
python3 integrations/check_collector_taxonomy.py
python3 -m unittest integrations.tests.test_taxonomy
python3 integrations/gen_docs_integrations.py -c go.d.plugin/<module>
```

Use the repo-local `.venv/bin/python` when one exists for the current
worktree.

## 6. Before opening the PR

Run:

```bash
git status --short
```

Commit source changes, generated docs, and taxonomy updates together.
Do not commit gitignored runtime artifacts such as
`integrations/integrations.js`, `integrations/integrations.json`, or
`integrations/taxonomy.json`.
