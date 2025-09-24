# IBM.D Collector Framework

The IBM.D framework is a thin layer on top of Netdata’s go.d engine that makes it easy – and predictable – to implement complex collectors. It focuses on:

- **Type-safe metric exports** generated from declarative context definitions
- **Consistent configuration** driven by Go structs and JSON Schema
- **Automation** so documentation, metadata, and stock alerts stay in sync

The framework is used by every collector inside `modules/` and is designed so AI assistants can safely extend it.

## Core Concepts

### 1. Contexts and Families

Module metrics are described in `contexts/contexts.yaml`. Each entry declares the Netdata context name, family hierarchy, units, chart priority, and dimensions. Example:

```yaml
System:
  labels: []
  contexts:
    - name: CPUUtilization
      context: as400.cpu_utilization
      family: compute/cpu
      units: percentage
      type: line
      priority: 101
      dimensions:
        - { name: utilization, algo: absolute, div: 1000 }
```

The generator produces strongly typed Go setters at `contexts/zz_generated_contexts.go`, so collectors can export metrics without stringly-typed code.

### 2. Collector Skeleton

Each module embeds `framework.Collector` for lifecycle management:

```go
type Collector struct {
    framework.Collector

    Config `yaml:",inline" json:",inline"`
    client *protocol.Client
    mx     *metricsData
}
```

Implement these hooks:

- `InitOnce()` to allocate caches and parse configuration defaults
- `Collect(ctx)` to call protocol clients and populate the typed context setters
- Optional `Cleanup()` for protocol tear-down

The base struct provides logging, state, and convenience helpers for instance tracking.

### 3. Configuration

Configuration structs embed `framework.Config` and define module-specific options:

```go
type Config struct {
    framework.Config `yaml:",inline" json:",inline"`

    Endpoint string           `yaml:"endpoint" json:"endpoint"`
    Timeout  confopt.Duration `yaml:"timeout"  json:"timeout"`

    MaxEntities int              `yaml:"max_entities" json:"max_entities"`
    MatchEntities matcher.Simple `yaml:"match_entities" json:"match_entities"`
}
```

Run `go generate` (see below) and docgen will emit `config_schema.json` and README tables automatically.

### 4. Protocols and Shared Packages

Framework collectors focus on orchestration. Low-level APIs live under:

- `protocols/` – HTTP/OpenMetrics, PMI XML, JMX helper, MQ PCF, etc.
- `pkg/` – CGO shims (ODBC bridge, DB2 helper libraries).

Protocols return typed data structures so collectors can be implemented with straightforward loops.

## Tooling

The following generators keep modules aligned:

| Tool | Location | Purpose |
|------|----------|---------|
| `docgen` | `../docgen` | Generates README, metadata.yaml, and config_schema.json |
| `metricgen` | `../metricgen` | Optional helper to scaffold metric exports |
| `go generate` directives | module directories | Invoke docgen + context generation |

Typical `go:generate` directives for a module:

```go
//go:generate go run ../../docgen -module=as400 -contexts=contexts/contexts.yaml -config=config.go -module-info=module.yaml
//go:generate go run ../../metricgen -module=as400 -contexts=contexts/contexts.yaml -out=contexts/zz_generated_contexts.go
```

After editing `contexts.yaml`, `config.go`, or `module.yaml` run:

```bash
cd src/go/plugin/ibm.d/modules/<module>
go generate ./...
```

## Writing a New Collector

1. **Create module scaffold** under `modules/<name>/` using an existing module as a template.
2. **Define contexts** in `contexts/contexts.yaml` and labels that describe your metrics.
3. **Model configuration** in `config.go` – include sensible defaults and cardinality controls (`MaxX`, `MatchX`).
4. **Implement protocol client(s)** if one doesn’t exist yet (place them in `protocols/<domain>/`).
5. **Implement collector.go**:
   - Parse config in `InitOnce`
   - Call protocols in `Collect`
   - Export metrics via the generated context setters
6. **Run generators** (`go generate`) and review README, metadata, schema output.
7. **Add stock alerts** under `src/health/health.d/` targeting contexts with safe thresholds.
8. **Document** any runtime prerequisites (CGO libraries, environment variables) in the module README and metadata.

## Runtime Integration

- Modules register themselves in `init()` using `framework.RegisterModule` (see existing modules for examples).
- The IBM plugin loads `/etc/netdata/ibm.d/<module>.conf` and constructs jobs according to the schema.
- Health alerts and dashboards automatically pick up the generated contexts; keep names stable.

## Debugging Tips

- Run the plugin in dump mode: `script -c 'sudo /usr/libexec/netdata/plugins.d/ibm.d.plugin -d -m MODULE --dump=3s --dump-summary 2>&1' /dev/null`
- The summary tree should match what is declared in `contexts.yaml`.
- Use the framework logger (`c.Infof`, `c.Warningf`, etc.) for human-friendly messages when a feature is unavailable.

## Additional Resources

- [`../README.md`](../README.md) – project overview, build instructions, and directory map
- [`../AGENTS.md`](../AGENTS.md) – authoring checklist and best practices for AI assistants
- [`../BEST-PRACTICES.md`](../BEST-PRACTICES.md) – in-depth guidance on go.d/ibm.d collector development

Contributions are welcome! Keep documentation, schemas, metadata, and health alerts synchronized to guarantee a smooth user experience.
