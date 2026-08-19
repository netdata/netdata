# ibm.d generation chain

ibm.d is a Go collector framework whose modules generate their
own `metadata.yaml` (and `README.md`, `config_schema.json`,
`zz_generated_contexts.go`) from a small set of authoritative
inputs. This is fundamentally different from go.d / python.d /
charts.d collectors where `metadata.yaml` is hand-edited.

**Maintainer rule**: for any ibm.d module, NEVER edit
`metadata.yaml`, `README.md`, or `config_schema.json` directly.
Edit `contexts.yaml`, `config.go`, or `module.yaml`, then run
`go generate ./...`.

## Layout per module

```
src/go/plugin/ibm.d/modules/<module-dir>/
├── module.yaml                # display name, descriptions, icon, categories, link
├── config.go                  # Config struct -- parsed via Go AST
├── contexts/
│   ├── contexts.yaml          # metric definitions: classes -> contexts -> dimensions
│   ├── doc.go                 # //go:generate go run ../../../metricgen/main.go ...
│   └── zz_generated_contexts.go  # GENERATED -- DO NOT EDIT
├── generate.go                # //go:generate go run ../../docgen ...
├── metadata.yaml              # GENERATED -- DO NOT EDIT (consumed by gen_integrations.py)
├── README.md                  # GENERATED -- DO NOT EDIT
├── config_schema.json         # GENERATED -- DO NOT EDIT
└── <module-source>.go ...     # the collector implementation (hand-written)
```

`websphere/` is a special parent: its sub-modules
`websphere/{jmx,mp,pmi}/` each have their own
`metadata.yaml`, `module.yaml`, etc. `gen_integrations.py:35`
adds `src/go/plugin/ibm.d/modules/websphere` separately to
`COLLECTOR_SOURCES` so these one-level-deeper paths get picked
up.

The guide uses two distinct identifiers:

- `<module-dir>` is the path relative to
  `src/go/plugin/ibm.d/modules/`, such as `db2` or
  `websphere/pmi`. Use it in filesystem paths and `go generate`.
- `<module-name>` is the exact `name` value in that directory's
  `module.yaml`, such as `db2` or `websphere_pmi`. Use it for
  docgen's `-module` argument and the integrations selector
  `ibm.d.plugin/<module-name>`.

The values happen to match for top-level modules. They differ for
all three nested WebSphere modules.

## The two generators

### `metricgen` -- contexts.yaml -> zz_generated_contexts.go

Repo path: `src/go/plugin/ibm.d/metricgen/main.go`.

Reads a module's `contexts/contexts.yaml`. The file declares
classes -> contexts -> dimensions in a structured form:

```yaml
classes:
  - name: connection
    contexts:
      - name: connection_count
        title: Connection count
        units: connections
        family: connections
        type: line
        dimensions:
          - name: total
          - name: active
```

Writes `contexts/zz_generated_contexts.go` -- a Go source file
that registers these contexts with the ibm.d framework so the
collector can emit metrics by name. The generated file is
committed.

Triggered by:

```go
//go:generate go run ../../../metricgen/main.go ...
```

at `src/go/plugin/ibm.d/modules/<module-dir>/contexts/doc.go:5`.

### `docgen` -- contexts.yaml + config.go + module.yaml -> metadata.yaml + README.md + config_schema.json

Repo path: `src/go/plugin/ibm.d/docgen/main.go`.

Inputs (per module):

- `contexts/contexts.yaml` -- the same metric structure
  metricgen reads. Parsed as `Config` with `Class` entries
  (`docgen/main.go:28-55`).
- `config.go` -- the Go `Config` struct. Parsed via Go AST
  (`docgen/config_parser.go`) to extract `ConfigField` records
  (`docgen/main.go:57-78`).
- `module.yaml` -- module-level metadata: name, display name,
  overview `description`, frontmatter `page_description`, icon,
  categories, and link. `page_description` is the authoritative
  source for an explicit integration-page description.

Outputs (per module):

- `metadata.yaml` -- written from `metadataTemplate`
  (`docgen/main.go:562`). The generated file opens with the
  banner: `# Generated metadata.yaml for <module> module`.
  When `module.yaml` sets `page_description`, docgen emits it as
  `meta.monitored_instance.description`; the repository-wide
  documentation generator then uses that value for page
  frontmatter. The file also carries hardcoded scaffolding
  (default `update_every: 1` option,
  `endpoint: dummy://localhost`, and a fixed prerequisite
  "Enable monitoring interface") plus dynamic content extracted
  from `contexts.yaml` and `config.go`. Authors who want richer
  metadata content must extend the template or `module.yaml`,
  not edit the generated file.
- `config_schema.json` -- written from a separate template
  (`docgen/main.go:528`). Used by the dashboard's DYNCFG
  editor.
- `README.md` -- written from a readme template
  (`docgen/main.go:552`). Includes module info, metric tables,
  config tables. Banner depends on the template.

Triggered by:

```go
//go:generate go run <relative-path-to-docgen> -module=<module-name> -contexts=contexts/contexts.yaml -config=config.go -module-info=module.yaml
```

at `src/go/plugin/ibm.d/modules/<module-dir>/generate.go:3`. The
relative path to docgen is `../../docgen` for top-level modules and
`../../../docgen` for nested WebSphere modules.

## End-to-end edit recipe (ibm.d module)

1. Edit one of:
   - `contexts/contexts.yaml` to add/change/remove a metric
     class, context, or dimension;
   - `config.go` to add/change/remove a config field;
   - `module.yaml` to change the display name, overview or page
     description, categories, icon, etc.
2. Run from the repo root:
   ```bash
   go generate ./src/go/plugin/ibm.d/modules/<module-dir>/...
   ```
   This invokes BOTH `metricgen` (on `contexts.yaml`) and
   `docgen` (on the module).
3. Inspect and validate ALL generated files:
   - `metadata.yaml`
   - `README.md`
   - `config_schema.json`
   - `contexts/zz_generated_contexts.go`
   For a documentation-only `module.yaml` change, leave the derived metadata,
   README, and integration pages to the post-merge generated-artifact PR. If
   `contexts.yaml` or `config.go` changes runtime behavior, commit the required
   runtime outputs (`contexts/zz_generated_contexts.go` and/or
   `config_schema.json`) in the source PR so compiled code and configuration
   stay synchronized. The post-merge route is not a substitute for runtime
   correctness.
4. Run the integrations regen locally to update the
   per-integration `.md` and the umbrella pages:
   ```bash
   ./integrations/pip.sh
   python3 integrations/gen_integrations.py
   python3 integrations/gen_docs_integrations.py -c ibm.d.plugin/<module-name>
   python3 integrations/gen_doc_collector_page.py
   python3 integrations/gen_doc_secrets_page.py
   ```
5. Confirm the generated diff contains only the expected derived changes.
   After the source PR merges, `generate-integrations.yml` runs this producer
   chain again and opens the separate generated-artifact PR containing the
   generated documentation files, `<plugin-dir>/integrations/<slug>.md`, and
   umbrella pages. Runtime outputs required by step 3 have already shipped in
   the source PR.

## Why ibm.d is generated this way

ibm.d collectors are typically heavy: many metrics, many
config fields, dense documentation. Generating ensures
consistency between:
- the runtime metric registration
  (`zz_generated_contexts.go`),
- the integration metadata (`metadata.yaml`),
- the dashboard schema (`config_schema.json`),
- the user-facing documentation (`README.md`).

It is the closest thing this repo has to enforcement of the
collector consistency rule for the integration-page side
(metadata + README + config_schema), but it does NOT cover
taxonomy.yaml, the stock `.conf`, or `health.d/<...>.conf` --
those still need manual sync unless a module-specific generator
adds coverage.

## Risks and gotchas

- **Hand edits to generated files are silently overwritten on
  next `go generate`.** No warning. The DO-NOT-EDIT banner is
  the only signal.
- **`module.yaml` is the right place for static prose** (e.g.
  description text) that the metadata template inlines. Edits
  to that file survive regeneration; edits to the generated
  `metadata.yaml` do not.
- **The metadata template hardcodes some scaffolding** (e.g.
  `endpoint: dummy://localhost`). Modules that need different
  scaffolding must extend the template at
  `docgen/main.go:562+` -- editing the generated `metadata.yaml`
  is not a fix.
- **`go generate` does not auto-run `gen_integrations.py`**.
  After regenerating ibm.d files, you still need to run the
  integrations pipeline to refresh the per-integration `.md`
  and umbrella pages.
- **`websphere/` sub-modules each have their own generation
  cycle**. Running `go generate ./src/go/plugin/ibm.d/modules/websphere/...`
  hits all three (`jmx`, `mp`, `pmi`).
