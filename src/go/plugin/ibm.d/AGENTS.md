# IBM.d Plugin Developer Guide

This guide is for developers contributing to the IBM.d plugin. For end-user documentation, see [README.md](./README.md).

## Architecture Overview

`ibm.d.plugin` is Netdata's CGO-enabled plugin for IBM workloads. It ships with collectors for DB2, IBM i (AS/400), IBM MQ, and WebSphere, all implemented with the **IBM.D framework** – a type-safe layer built on top of go.d designed to be AI-assistant friendly.

### Why a Dedicated Plugin?

- **Native libraries** – DB2 connectivity and several IBM APIs require IBM's C client libraries, so the plugin is compiled with `CGO_ENABLED=1`.
- **Predictable code generation** – collectors describe their metrics in declarative YAML; code-gen keeps the runtime, schema, metadata, and docs in sync.
- **Modular architecture** – reusable protocols (OpenMetrics, PMI XML, JMX bridge, MQ interfaces) make it easy to add new IBM collectors without duplicating plumbing.

## Repository Layout

| Path | Purpose |
|------|---------|
| `framework/` | IBM.D collector SDK: base collector, context helpers, generator tooling. See [`framework/README.md`](framework/README.md). |
| `modules/` | All IBM collectors (AS400, DB2, MQ, WebSphere). Each module is self-contained and backed by the framework. |
| `protocols/` | Reusable protocol clients (e.g. PMI XML parser, OpenMetrics client, JMX helper bridge, MQ PCF client). |
| `pkg/` | Shared CGO shims (DB2 ODBC bridge, ODBC helpers) used by multiple protocols/modules. |
| `docgen/` | Tooling to generate docs/config metadata straight from module sources. |
| `metricgen/` | Experimental helper for generating boilerplate metric exports. |

## Auto-Generated Files

The IBM.D plugin uses code generation to keep contexts, documentation, and metadata in sync. Understanding which files are generated vs. editable is crucial for development.

### Generated Files (DO NOT EDIT)

Each module generates these files automatically:

| File | Generator | Source | Purpose |
|------|-----------|--------|---------|
| `zz_generated_contexts.go` | `metricgen` | `contexts.yaml` | Type-safe Go structs for metric contexts |
| `README.md` | `docgen` | `contexts.yaml` + `config.go` + `module.yaml` | Module documentation |
| `metadata.yaml` | `docgen` | `contexts.yaml` + `config.go` + `module.yaml` | Netdata integrations metadata |

**⚠️ Warning:** Direct edits to these files will be overwritten on the next `go generate` run.

### Source Files (EDITABLE)

| File | Purpose |
|------|---------|
| `contexts/contexts.yaml` | **Source of truth** for all metrics, charts, dimensions, families, priorities |
| `config.go` | Collector configuration structure (exported to JSON schema by docgen) |
| `module.yaml` | Module metadata (name, description, categories) |
| All other `.go` files | Module implementation code |

### Regenerating Code

#### Regenerate a Single Module

From the module directory:
```bash
cd modules/as400
go generate ./...
```

This runs both generators:
1. **metricgen** (via `contexts/doc.go`) → regenerates `zz_generated_contexts.go`
2. **docgen** (via `generate.go`) → regenerates `README.md` and `metadata.yaml`

#### Regenerate All Modules

From the plugin root:
```bash
cd src/go/plugin/ibm.d
go generate ./modules/...
```

#### After Regeneration

Always run `gofmt` on generated Go code:
```bash
gofmt -w modules/*/contexts/zz_generated_contexts.go
```

### When to Regenerate

Regenerate after modifying:
- ✅ `contexts/contexts.yaml` (metrics definitions)
- ✅ `config.go` (configuration structure)
- ✅ `module.yaml` (module metadata)
- ❌ Implementation `.go` files (no regeneration needed)

### Verifying Generated Code

After regeneration, verify the module works:
```bash
sudo script -c '/usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m MODULE --dump=3s --dump-summary 2>&1' /dev/null
```

## Building the Plugin

The plugin is built automatically by Netdata's CMake tree when `ENABLE_PLUGIN_IBM=On` and the IBM CLI driver is available:

```bash
mkdir build-ibm && cd build-ibm
cmake -DENABLE_PLUGIN_IBM=On ..
make ibm-plugin
```

The build target downloads the driver if it is not already present; see the packaging scripts for distro-specific logic. The resulting binary is placed under `build-ibm/ibm.d.plugin` and must remain in `usr/libexec/netdata/plugins.d/` for Netdata to load it.

## Module Development Workflow

1. Update `contexts/contexts.yaml` and `config.go` (see [Source Files](#source-files-editable)).
2. Run `go generate ./...` in the module directory (see [Regenerating Code](#regenerating-code)).
3. Run `gofmt -w contexts/zz_generated_contexts.go` to format generated code.
4. Validate with `script -c 'sudo /usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m MODULE --dump=3s --dump-summary 2>&1' /dev/null`.
5. Commit **both** source files and generated files together.

## Testing & Debugging

### Command-line dump mode
Works exactly like go.d:
```bash
script -c 'sudo /usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m MODULE --dump=2s --dump-summary 2>&1' /dev/null
```

### Structured fixture dumps
Generate JSON/SQL artifacts for automated tests:
```bash
ibm.d.plugin --module MODULE --dump-data ./testdata/MODULE
```
The flag implicitly enables dump mode and exits once every job has produced at least one collection.

## Contributing Guidelines

1. Review [`framework/README.md`](framework/README.md) for IBM.D framework details.
2. Follow [General collector best practices](../BEST-PRACTICES.md).
3. **Never edit auto-generated files** – see [Auto-Generated Files](#auto-generated-files) section.
4. Always regenerate code after modifying `contexts.yaml`, `config.go`, or `module.yaml`.
5. Run `gofmt` on generated Go files before committing.
6. Commit **both** source and generated files together to keep them in sync.
7. Each module directory (`modules/<name>/`) contains its own README with module-specific notes.

## Runtime Internals

- The plugin reads `/etc/netdata/ibm.d.conf` for global settings and discovers per-collector jobs under `/etc/netdata/ibm.d/*.conf`.
- Each module provides safe stock health alarms in `src/health/health.d/`.
- The plugin supports dynamic configuration through the Netdata Agent.

For questions or suggestions, open a GitHub issue or reach out on Netdata's community channels.