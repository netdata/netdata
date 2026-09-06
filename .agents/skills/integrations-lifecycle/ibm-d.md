# ibm.d generation chain

ibm.d modules generate their own `metadata.yaml`, `README.md`, `config_schema.json`, and `zz_generated_contexts.go`
from a small set of authoritative inputs. This differs from every other collector family, where `metadata.yaml` is
hand-edited.

**Maintainer rule**: for any ibm.d module, NEVER edit `metadata.yaml`, `README.md`, or `config_schema.json` directly.
Edit `contexts/contexts.yaml`, `config.go`, or `module.yaml`, then run `go generate`. Hand edits to generated files are
silently overwritten on the next `go generate`; the DO-NOT-EDIT banner is the only signal.

## Layout per module

```
src/go/plugin/ibm.d/modules/<module-dir>/
├── module.yaml                # display name, descriptions, icon, categories, link
├── config.go                  # Config struct, parsed via Go AST
├── contexts/
│   ├── contexts.yaml          # metric definitions: classes -> contexts -> dimensions
│   ├── doc.go                 # //go:generate go run ../../../metricgen/main.go ...
│   └── zz_generated_contexts.go  # GENERATED (package doc says DO NOT EDIT)
├── generate.go                # //go:generate go run <path-to-docgen> ...
├── metadata.yaml              # GENERATED (first line: `# Generated metadata.yaml for <module> module`)
├── README.md                  # GENERATED
├── config_schema.json         # GENERATED (pure JSON, no banner)
└── <module-source>.go ...     # the collector implementation (hand-written)
```

`websphere/` is a special parent: its sub-modules `websphere/{jmx,mp,pmi}/` each have their own `metadata.yaml`,
`module.yaml`, and contexts. `COLLECTOR_SOURCES` in `integrations/_common.py` lists
`src/go/plugin/ibm.d/modules/websphere` separately because the module glob is one level deep.

The guide uses two distinct identifiers:

- `<module-dir>` is the path relative to `src/go/plugin/ibm.d/modules/`, such as `db2` or `websphere/pmi`. Use it in
  filesystem paths and `go generate`.
- `<module-name>` is the exact `name` value in that directory's `module.yaml`, such as `db2` or `websphere_pmi`. Use
  it for docgen's `-module` argument and the integrations selector `ibm.d.plugin/<module-name>`.

The values match for top-level modules and differ for all three nested WebSphere modules.

## The two generators

### `metricgen`: `contexts.yaml` to `zz_generated_contexts.go`

`src/go/plugin/ibm.d/metricgen/main.go` reads `contexts/contexts.yaml` (classes, contexts, dimensions) and writes the
Go file that registers those contexts with the ibm.d framework. The directive is in `contexts/doc.go`.

### `docgen`: `contexts.yaml` + `config.go` + `module.yaml` to `metadata.yaml` + `README.md` + `config_schema.json`

`src/go/plugin/ibm.d/docgen/main.go`. Inputs per module:

- `contexts/contexts.yaml`, the same structure `metricgen` reads;
- `config.go`, parsed via Go AST (`docgen/config_parser.go`) into config field records;
- `module.yaml`: name, display name, overview `description`, frontmatter `page_description`, icon, categories, link.
  `page_description` becomes `meta.monitored_instance.description`, the explicit page meta description
  (`description-authoring.md`).

Outputs per module, each from a template inside `docgen/main.go` (`metadataTemplate`, `generateConfigSchema`,
`generateReadme`). The metadata template hardcodes scaffolding that the module does not control: the default
`update_every: 1` option, `endpoint: dummy://localhost`, and a fixed "Enable monitoring interface" prerequisite. Richer
metadata content means extending the template or `module.yaml`, never editing the generated file; `module.yaml` is the
right place for static prose because it survives regeneration.

The docgen directive lives in `generate.go`:

```go
//go:generate go run <relative-path-to-docgen> -module=<module-name> -contexts=contexts/contexts.yaml -config=config.go -module-info=module.yaml
```

The relative path is `../../docgen` for top-level modules and `../../../docgen` for the nested WebSphere modules.

## End-to-end edit recipe

1. Edit one of `contexts/contexts.yaml` (metric class, context, dimension), `config.go` (config field), or
   `module.yaml` (display name, descriptions, categories, icon).
2. From the repo root:

   ```bash
   go generate ./src/go/plugin/ibm.d/modules/<module-dir>/...
   ```

   This runs BOTH `metricgen` and `docgen`. `go generate ./src/go/plugin/ibm.d/modules/websphere/...` hits all three
   WebSphere sub-modules.
3. Inspect all four generated files. Delivery follows `consistency.md`, "Delivery boundary": the runtime outputs
   `contexts/zz_generated_contexts.go` and `config_schema.json` ship in the source PR with the change that produced them
   (both workflows fail on drift there); `metadata.yaml`, `README.md`, and the integration pages go through the
   post-merge regeneration PR.
4. `go generate` does not run the integrations pipeline. Validate the derived metadata locally:

   ```bash
   python3 integrations/gen_integrations.py
   python3 integrations/gen_docs_integrations.py -c ibm.d.plugin/<module-name>
   python3 integrations/gen_doc_collector_page.py
   ```

5. Confirm the generated diff contains only the expected derived changes, then leave the documentation outputs
   unstaged.

## What generation does and does not cover

Generation keeps runtime metric registration, integration metadata, the DynCfg schema, and the README consistent by
construction. It does NOT cover the stock `.conf` or `health.d/<...>.conf`; those still need manual sync under the
consistency rule.
