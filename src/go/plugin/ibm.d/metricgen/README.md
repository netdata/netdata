# metricgen

`metricgen` is a lightweight helper that emits typed Go boilerplate for IBM.D modules. It reads the same `contexts/contexts.yaml` file used by docgen and produces strongly typed setters or utility structs that make it easier to export metrics without hand-writing repetitive code.

Usage example (from a module directory):

```go
//go:generate go run ../../metricgen \
//          -module=db2 \
//          -contexts=contexts/contexts.yaml \
//          -out=contexts/zz_generated_contexts.go
```

The generated file is committed to source control and used by the collector:

- Adds typed `Set` helpers for every context/dimension
- Exposes label structs for dynamic charts
- Keeps naming synchronized with `contexts.yaml`

You can extend the tool if a module needs additional scaffolding, but in most cases the default output is sufficient.
