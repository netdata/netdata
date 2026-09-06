# Gotchas

Traps the code does not announce. Mechanics that merely surprise are in `pipeline.md`; this file keeps the facts that
cost someone a debugging session. Citations name symbols; open the file to find them.

## Dead or unenforced code

- `integrations/check_collector_metadata.py` is dead. It imports `SINGLE_PATTERN`, `MULTI_PATTERN`, `SINGLE_VALIDATOR`,
  `MULTI_VALIDATOR` from `gen_integrations`, names that no longer exist (the live ones are `METADATA_PATTERN` and
  `COLLECTOR_VALIDATOR` in `_common.py`), so it exits with `ImportError`; its success message also lacks the `f` prefix.
  No workflow or CMake module references it; only `integrations/README.md` mentions it as unused. Do not rely on it, and
  track any revival as its own issue first.
- `integrations/schemas/distros.json` is declared but never consulted: `main()` loads `.github/data/distros.yml` with
  `load_yaml` and hands it to `render_deploy` unvalidated. Garbage in that file produces broken `platform_info` tables
  with no error. Track any wiring-up as its own issue first.
- The `cid == 'flows'` branch in `gen_doc_collector_page.py` refers to a top-level category that no longer exists
  (`network-performance-monitoring` replaced it); it is dead. Earlier guidance to keep it was written before the
  category moved; removing it is optional cleanup, not required by any current change.

## Silent behaviors

- `build_path` in `gen_docs_integrations.py` derives the local directory from the `meta_yaml` GitHub URL and must strip
  both `edit/master/` and `blob/master/` before removing `/metadata.yaml`. When it does not, scoped generation (`-c
  go.d.plugin/<module>`) finds the collector in `integrations.js` and writes nothing, because the derived path does not
  exist. It also assumes `https://github.com/netdata/...` (`AGENT_REPO` in `_common.py`); a fork path breaks it.
- `resolve_related_links` replaces a `{% relatedResource %}` marker whose id is unknown with the bare name and no
  warning.
- `make_id` keeps the display name's case (`go.d.plugin-pulsar-Apache_Pulsar`) while `clean_string` lowercases the slug,
  so an id and its page slug never match byte for byte. Slug collisions: `pipeline.md`, Stage 2.
- The schemas are not strict, so an unknown key such as `alternative_monitored_instances`
  (`src/go/plugin/go.d/collector/postgres/metadata.yaml`) validates, reaches `integrations.js`, and is rendered by
  nothing.
- The `collector_default` fallback (`data-collection.applications`) fires only for a module whose `categories` list is
  empty. A module whose declared ids are all invalid gets a fatal warning and an empty category list; it is not parked
  anywhere.
- The community badge is chosen by key presence (`"community" in integration["meta"]`), not by value: a `community`
  key set to `false` still renders the Community badge. Every current use is `true`.
- `PRESERVE_FILES` in `gen_docs_integrations.py` and the dcstat removal step in `check-markdown.yml` are a coupled pair
  around one Learn redirect migration (`pipeline.md`, Stage 2). A local full regeneration therefore keeps a page whose
  source directory no longer produces it; that is intended until the Learn catalog is republished.

## Validation traps

- A generic Draft-7 validator ignores the custom `netdata-balanced-parentheses` format and reports a file valid that the
  generator rejects; validate through `make_validator()` only (`pipeline.md`, Stage 1).
- `fail_on_warnings()` fails the run on any warning at all, deduplicated by file path. Cosmetic issues block the
  regeneration PR.

## Source layout

- `integrations/cloud-notifications/metadata.yaml` and `integrations/cloud-authentication/metadata.yaml` are single
  files holding arrays; the loaders branch on `if 'id' in data` to accept either one entry or an array. Most other types
  have one file per integration.
- `COLLECTOR_SOURCES` lists `src/go/plugin/ibm.d/modules/websphere` separately from `.../modules`, and
  `TAXONOMY_SOURCES` lists each ebpfgo `taxonomy.yaml` individually, because the recursive glob is one level deep. A new
  nested module directory needs the same treatment or it is silently skipped (`pipeline.md`, sources table).
- `integrations/pip.sh` and `packaging/cmake/Modules/NetdataRenderDocs.cmake` list the same Python packages and MUST be
  changed together (`pip.sh` says so in a comment). `markdown-it-py` is a runtime dependency of generation, not a
  test-only one: `gen_integrations.py` imports `_common`, which imports `descriptions`, which imports `markdown_it`.
- `integrations/templates/README.md` predates `setup-service_discovery.md` and does not list it.

## Rendering into MDX

Every free-text `metadata.yaml` field flows through the generator, the tracked page, Learn ingest, and the MDX 3 build
on Netlify. Learn's ingest escapes only a few patterns (bare `{`, the operators `<=`, `%<`, `<->`, and
`<details><summary>`); everything else passes this repository's CI and fails the next Learn deploy preview. The
author-side rules are in `.agents/skills/collectors-metadata-yaml/SKILL.md` ("Safety Of The Markdown") and
`integrations/tests/test_collector_metadata.py` checks collector metadata for them in both workflows; the MDX side is
`docs-learn-site-structure/mdx-rules.md` and `pitfalls-and-gotchas.md`.

The incident that produced the rule (2026-05-07): the netflow-plugin metadata carried `description: Sets tenant=amazon,
region=<aws-region>, role=<service-name>.` for the AWS IP Ranges card. Netdata CI, `gen_integrations.py`, and Learn
ingest all passed; the Netlify preview failed with `Expected a closing tag for <service-name>`. Fix: backticks around
the placeholders at the source.
