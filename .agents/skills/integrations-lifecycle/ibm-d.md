# ibm.d generation chain

ibm.d modules generate their own `metadata.yaml`, `config_schema.json`, and `zz_generated_contexts.go` from a small set
of authoritative inputs. This differs from every other collector family, where `metadata.yaml` is hand-edited.

**Maintainer rule**: for any ibm.d module, NEVER edit `metadata.yaml`, `config_schema.json`, or the module's `README.md`
directly. Edit `contexts/contexts.yaml`, `config.go`, or `module.yaml`, then run `go generate`. Hand edits to generated
files are silently overwritten on the next `go generate`; the DO-NOT-EDIT banner is the only signal.

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
├── README.md                  # a tracked SYMLINK to integrations/<slug>.md, created by the pipeline (see the trap below)
├── config_schema.json         # GENERATED (pure JSON, no banner)
└── <module-source>.go ...     # the collector implementation (hand-written)
```

`websphere/` is a special parent: its sub-modules `websphere/{jmx,mp,pmi}/` each have their own `metadata.yaml`,
`module.yaml`, and contexts. `COLLECTOR_SOURCES` in `integrations/_common.py` lists
`src/go/plugin/ibm.d/modules/websphere` separately because the module glob is one level deep.

The guide uses two distinct identifiers:

- `<module-dir>` is the path relative to `src/go/plugin/ibm.d/modules/`, such as `db2` or `websphere/pmi`. Use it in
  filesystem paths and `go generate`.
- `<module-name>` is the exact `name` value in that directory's `module.yaml`, such as `db2` or `websphere_pmi`. Use it
  for docgen's `-module` argument and the integrations selector `ibm.d.plugin/<module-name>`.

The values match for top-level modules and differ for all three nested WebSphere modules.

## The two generators

### `metricgen`: `contexts.yaml` to `zz_generated_contexts.go`

`src/go/plugin/ibm.d/metricgen/main.go` reads `contexts/contexts.yaml` (classes, contexts, dimensions) and writes the Go
file that registers those contexts with the ibm.d framework. The directive is in `contexts/doc.go`.

### `docgen`: `contexts.yaml` + `config.go` + `module.yaml` to `metadata.yaml` + `README.md` + `config_schema.json`

`src/go/plugin/ibm.d/docgen/main.go`. Inputs per module:

- `contexts/contexts.yaml`, the same structure `metricgen` reads;
- `config.go`, parsed via Go AST (`docgen/config_parser.go`) into config field records;
- `module.yaml`: name, display name, overview `description`, frontmatter `page_description`, icon, categories, keywords,
  link. `page_description` becomes `meta.monitored_instance.description`, the explicit page meta description
  (`description-authoring.md`).

Outputs per module: `metadata.yaml` from the `metadataTemplate` constant in `docgen/main.go`, `config_schema.json` built
programmatically by `generateConfigSchema`, and a README from `readmeTemplate` (`generateReadme`). The metadata template
hardcodes scaffolding that the module does not control: the default `update_every: 1` option, `endpoint:
dummy://localhost`, and a fixed "Enable monitoring interface" prerequisite. Richer metadata content means extending the
template or `module.yaml`, never editing the generated file; `module.yaml` is the right place for static prose because
it survives regeneration.

**The README trap.** Every ibm.d module's `README.md` is a tracked symlink to `integrations/<slug>.md`, made by the
integrations pipeline like any single-integration directory. `generateReadme` opens `README.md` with `os.Create`, which
follows the symlink, so a local `go generate` overwrites the tracked generated integration page with docgen's README
content and leaves it modified in `git status`. In CI this is masked because `gen_docs_integrations.py` runs afterwards
and rewrites the page. Locally, never stage that page; regenerate it with the pipeline or leave it to the post-merge
regeneration PR.

The docgen directive lives in `generate.go`:

```go
//go:generate go run <relative-path-to-docgen> -module=<module-name> -contexts=contexts/contexts.yaml -config=config.go -module-info=module.yaml
```

The relative path is `../../docgen` for top-level modules and `../../../docgen` for the nested WebSphere modules.

## End-to-end edit recipe

1. Edit one of `contexts/contexts.yaml` (metric class, context, dimension), `config.go` (config field), or `module.yaml`
   (display name, descriptions, categories, icon).
2. From the Go module root (there is no `go.mod` at the repository root, so the repo-root form fails with "does not
   contain main module"):

   ```bash
   cd src/go && go generate ./plugin/ibm.d/modules/<module-dir>/...
   ```

   This runs BOTH `metricgen` and `docgen`. `./plugin/ibm.d/modules/websphere/...` hits all three WebSphere sub-modules;
   both workflows run `./plugin/ibm.d/modules/...` for every module.
3. Inspect the generated files. Delivery follows `consistency.md`, "Delivery boundary": the runtime outputs
   `contexts/zz_generated_contexts.go` and `config_schema.json` ship in the source PR with the change that produced them
   (both workflows fail on drift there); `metadata.yaml` and the integration page (including the one docgen just wrote
   through the README symlink) go through the post-merge regeneration PR.
4. `go generate` does not run the integrations pipeline. Validate the derived metadata locally:

   ```bash
   python3 integrations/gen_integrations.py
   python3 integrations/gen_docs_integrations.py --check
   ```

   Regenerating the page (`python3 integrations/gen_docs_integrations.py -c ibm.d.plugin/<module-name>`) is optional; it
   also repairs the page `go generate` overwrote through the README symlink.
5. Confirm the diff contains only the expected changes, then undo every change to `metadata.yaml`, `README.md`, and
   `integrations/<slug>.md` before committing; they arrive through the post-merge PR.

## What generation does and does not cover

Generation keeps runtime metric registration, integration metadata, and the DynCfg schema consistent by construction. It
does NOT cover the stock `.conf` or `health.d/<...>.conf`; those still need manual sync under the consistency rule.
