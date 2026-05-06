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
src/go/plugin/ibm.d/modules/<m>/
├── module.yaml                # display name, description, icon, categories, link, keywords
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

at `src/go/plugin/ibm.d/modules/<m>/contexts/doc.go:5`.

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
  description, icon, categories, link, keywords.

Outputs (per module):

- `metadata.yaml` -- written from `metadataTemplate`
  (`docgen/main.go:562`). The generated file opens with the
  banner: `# Generated metadata.yaml for <module> module`. It
  carries hardcoded scaffolding (`most_popular: false`,
  default `update_every: 1` option, `endpoint: dummy://localhost`,
  and a fixed prerequisite "Enable monitoring interface")
  PLUS the dynamic content extracted from `contexts.yaml` and
  `config.go`. Authors who want richer metadata.yaml content
  must extend the template or `module.yaml`, NOT edit the
  generated file.
- `config_schema.json` -- written from a separate template
  (`docgen/main.go:528`). Used by the dashboard's DYNCFG
  editor.
- `README.md` -- written from a readme template
  (`docgen/main.go:552`). Includes module info, metric tables,
  config tables. Banner depends on the template.

Triggered by:

```go
//go:generate go run ../../docgen -module=<m> -contexts=contexts/contexts.yaml -config=config.go -module-info=module.yaml
```

at `src/go/plugin/ibm.d/modules/<m>/generate.go:3`.

## End-to-end edit recipe (ibm.d module)

1. Edit one of:
   - `contexts/contexts.yaml` to add/change/remove a metric
     class, context, or dimension;
   - `config.go` to add/change/remove a config field;
   - `module.yaml` to change the display name, description,
     categories, icon, etc.
2. Run from the repo root:
   ```bash
   go generate ./src/go/plugin/ibm.d/modules/<m>/...
   ```
   This invokes BOTH `metricgen` (on `contexts.yaml`) and
   `docgen` (on the module).
3. Commit ALL generated files together with the source change:
   - `metadata.yaml`
   - `README.md`
   - `config_schema.json`
   - `contexts/zz_generated_contexts.go`
4. Run the integrations regen locally to update the
   per-integration `.md` and the umbrella pages:
   ```bash
   ./integrations/pip.sh
   python3 integrations/gen_integrations.py
   python3 integrations/gen_docs_integrations.py -c ibm.d/<m>
   python3 integrations/gen_doc_collector_page.py
   python3 integrations/gen_doc_secrets_page.py
   ```
5. Commit the regenerated `<plugin-dir>/integrations/<slug>.md`
   and umbrella pages too, in the same PR.

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
five-file consistency rule for the integration-page side
(metadata + README + config_schema), but it does NOT cover
the stock `.conf` or `health.d/<...>.conf` -- those still
need manual sync.

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
