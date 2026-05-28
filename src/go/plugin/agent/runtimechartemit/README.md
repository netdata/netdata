# runtimechartemit

This package is the runtime/internal metrics bridge between component-owned
`metrix.RuntimeStore` writers and Netdata chart protocol output.

## Registration flow

1. Component code creates/owns a `metrix.RuntimeStore` (writer side).
2. Component registers itself via `runtimecomp.Service.RegisterComponent(...)` with:
   - stable `Name`
   - `Store`
   - optional `TemplateYAML` (or `Autogen.Enabled=true` for fallback template)
   - cadence metadata (`UpdateEvery`) and emit env metadata (`TypeID`, `Plugin`, `Module`, `JobName`, `JobLabels`).
3. Service normalizes config and upserts it into the internal component registry.
4. Runtime metrics job snapshots registry entries on each tick.
5. For each component due on this tick:
   - read component store via `Read(metrix.ReadRaw(), metrix.ReadFlatten())`
   - build plan with chartengine
   - emit plan through `chartemit.ApplyPlan(...)`.
6. When component is unregistered, runtime job emits obsolete/remove actions for previously known charts.

## Lifecycle ownership

- `Service.Start(pluginName, out)` starts runtime metrics job and cadence ticker.
- `Service.Stop()` stops ticker and runtime job.
- Components should register on start and unregister on stop to avoid stale runtime charts.

## Producers vs components

- Components: register a runtime store to be charted.
- Producers: register a `tickFn` callback via `RegisterProducer` when a runtime source has no independent owner loop and must be advanced by runtime service cadence.

## Operational notes

- Runtime metrics job is intentionally single-flight; overlapping ticks are skipped and logged.
- Observer chartengine runs with `WithRuntimeStore(nil)` (no self-instrumentation loop).
- `Name` is the registry identity; re-registering the same name replaces generation and reinitializes runtime chart state.
