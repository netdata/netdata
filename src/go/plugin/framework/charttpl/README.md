# charttpl

`charttpl` defines the chart-template DSL used by `ModuleV2` collectors.
It is the input schema consumed by `chartengine`.

**Audience**: `ModuleV2` collector authors and framework contributors.

**See also**: [metrix](/src/go/pkg/metrix/README.md) (metrics storage and read API),
[chartengine](/src/go/plugin/framework/chartengine/README.md) (compile + plan).

## Purpose

| Package       | Role                                                                         |
|---------------|------------------------------------------------------------------------------|
| `charttpl`    | Parse + default + semantic validation of chart-template YAML                 |
| `chartengine` | Compile template into immutable program and build create/update/remove plans |

## Processing Pipeline

1. **Decode** — Strict YAML decode (`yaml.UnmarshalStrict`). Unknown fields fail decoding.
2. **Defaults** — `version` defaults to `v1` (top-level only); chart `type` defaults to `line` (applied recursively across groups).
3. **Semantic validation** — Field-level checks with path-aware errors (metric scoping, selectors, dimension rules, lifecycle bounds).

## Syntax (v1)

### Top-level fields

| Field               | Type   | Required | Description                                                   |
|---------------------|--------|----------|---------------------------------------------------------------|
| `version`           | string | no       | Must be `v1` (defaults to `v1`)                               |
| `context_namespace` | string | no       | Prefix for chart context path (see context composition below) |
| `engine`            | object | no       | Template-level chartengine policy                             |
| `groups`            | array  | yes      | Recursive chart groups                                        |

### `groups[]`

| Field               | Type          | Required | Description                                                                          |
|---------------------|---------------|----------|--------------------------------------------------------------------------------------|
| `family`            | string        | yes      | Family segment used for chart family composition                                     |
| `context_namespace` | string        | no       | Context segment appended to inherited context namespace                              |
| `metrics`           | array[string] | no       | Metrics available for dimension selectors in this group (inherited by nested groups) |
| `charts`            | array         | no       | Chart definitions                                                                    |
| `groups`            | array         | no       | Nested groups                                                                        |

**Context composition**: The final chart context is built by joining all context parts with `.`:
`<top context_namespace>.<group context_namespace>...<chart.context>`.
For example, with top-level `context_namespace: netdata.go.plugin`, group `context_namespace: mysql`,
and chart `context: queries`, the resulting context is `netdata.go.plugin.mysql.queries`.

### `charts[]`

| Field             | Type          | Required | Description                                                                      |
|-------------------|---------------|----------|----------------------------------------------------------------------------------|
| `id`              | string        | no       | Base chart ID template (if omitted, derived from `context`)                      |
| `title`           | string        | yes      | Chart title                                                                      |
| `family`          | string        | no       | Optional chart-level family leaf                                                 |
| `context`         | string        | yes      | Chart context leaf                                                               |
| `units`           | string        | yes      | Chart units                                                                      |
| `algorithm`       | string        | no       | `absolute` or `incremental`                                                      |
| `type`            | string        | no       | `line`, `area`, `stacked`, `heatmap` (defaults to `line`)                        |
| `priority`        | int           | no       | Chart priority                                                                   |
| `label_promotion` | array[string] | no       | Labels to promote as chart labels (visible in chart metadata, used for grouping) |
| `instances`       | object        | no       | Instance identity policy                                                         |
| `lifecycle`       | object        | no       | Instance/dimension cap and expiry policy                                         |
| `dimensions`      | array         | yes      | Dimension selectors and naming                                                   |

### `instances.by_labels`

Instance identity determines how series are grouped into chart instances.
When multiple series share the same instance identity labels, they appear as dimensions on the same chart.

| Token        | Meaning                                         |
|--------------|-------------------------------------------------|
| `label_key`  | Include explicit label key in instance identity |
| `*`          | Include all labels                              |
| `!label_key` | Exclude label key                               |

### `lifecycle`

| Field                            | Type | Default        | Description                                             |
|----------------------------------|------|----------------|---------------------------------------------------------|
| `max_instances`                  | int  | `0`            | Best-effort cap (`0` = disabled)                        |
| `expire_after_cycles`            | int  | engine default | Expire chart instances not seen for N successful cycles |
| `dimensions.max_dims`            | int  | `0`            | Best-effort dimension cap (`0` = disabled)              |
| `dimensions.expire_after_cycles` | int  | `0`            | Expire dimensions not seen for N successful cycles      |

### `dimensions[]`

| Field                | Type   | Required | Description                                                    |
|----------------------|--------|----------|----------------------------------------------------------------|
| `selector`           | string | yes      | Metric selector expression (must include explicit metric name) |
| `name`               | string | no       | Static dimension name                                          |
| `name_from_label`    | string | no       | Dynamic dimension name sourced from one label                  |
| `options.multiplier` | int    | no       | DIM multiplier (`0` means default `1`)                         |
| `options.divisor`    | int    | no       | DIM divisor (`0` means default `1`)                            |
| `options.hidden`     | bool   | no       | Mark dimension hidden                                          |
| `options.float`      | bool   | no       | Emit `type=float` and use `SETFLOAT` updates                   |

**Selector syntax**: A selector takes the form `metric_name` or `metric_name{label=value, ...}`.
The metric name prefix is required; label-only selectors like `{label=value}` are rejected.

### `engine` (template-level policy)

| Field                                 | Type          | Description                                                               |
|---------------------------------------|---------------|---------------------------------------------------------------------------|
| `selector.allow`                      | array[string] | Global include selectors                                                  |
| `selector.deny`                       | array[string] | Global exclude selectors                                                  |
| `autogen.enabled`                     | bool          | Enable unmatched-series autogen fallback                                  |
| `autogen.max_type_id_len`             | int           | Max full `type.id` length (`0` = default; must be `0` or `>= 4` when set) |
| `autogen.expire_after_success_cycles` | uint64        | Autogen lifecycle expiry                                                  |

## Minimal Example

```yaml
version: v1
context_namespace: netdata.go.plugin.example
groups:
  - family: HTTP
    metrics:
      - app.requests_total
      - app.latency_seconds_bucket
    charts:
      - id: requests
        title: Requests
        context: requests
        units: requests/s
        dimensions:
          - selector: app.requests_total
            name: requests
      - id: latency
        title: Request latency
        context: latency
        units: seconds
        dimensions:
          - selector: app.latency_seconds_bucket{le="0.5"}
            name: le_0_5
```

## Validation Rules

All rules below produce semantic validation errors unless noted:

- `version` must be `v1`
- `groups[]` must be non-empty
- `group.family` must not be empty or whitespace-only
- `group.metrics[]` entries must not be empty or whitespace-only; no duplicates within same group
- `chart.title`, `chart.context`, `chart.units` must be non-empty
- `dimension.selector` must include explicit metric name (prefix before `{`)
- Selector metric must be visible in current group metric scope
- `name` and `name_from_label` are mutually exclusive
- `name` and `name_from_label` must not be whitespace-only
- Duplicate dimension `name` values within the same chart are rejected
- `instances.by_labels` must contain at least one token when `instances` is set
- `instances.by_labels` exclude token must include label key (e.g., `!key`, not bare `!`)
- `instances.by_labels` tokens must not be duplicated
- Lifecycle numeric fields must be `>= 0`
- `engine.autogen.max_type_id_len` must be `0` or `>= 4`
- Unknown YAML field — decode error (strict unmarshal)

## Current Phase-1 Constraints

- **Placeholders in `chart.id` and `dimension.name`** — Parsed by template parser but intentionally rejected in phase-1 syntax.
- **Runtime inferred dimension naming** — Allowed only for inferable selectors (histogram bucket / summary quantile / stateset-like sources) when both `name` and `name_from_label` are omitted.
- **Selector parse stage** — Full selector parsing happens in the chartengine compile stage, not here.

## Compiler-Derived Behavior

The following behaviors are applied by `chartengine` during compilation, not by `charttpl`.
They affect how you write templates:

| Input                                   | Derived behavior                                                                          |
|-----------------------------------------|-------------------------------------------------------------------------------------------|
| Missing `chart.id`                      | `id` derived from `context` (`.` replaced with `_`)                                       |
| Missing `chart.algorithm`               | Inferred from metric suffixes (`*_total`, `*_count`, `*_sum`, `*_bucket` => counter-like) |
| Group family hierarchy + `chart.family` | Composed into slash-separated chart family                                                |
