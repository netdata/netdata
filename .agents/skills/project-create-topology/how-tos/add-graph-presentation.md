# Add Graph Presentation To A Topology

## Question

How should a topology producer add polished graph presentation to
`netdata.topology.v1` without making the UI domain-specific?

## Inputs

- A topology producer that emits `netdata.topology.v1`.
- Actor and link compact tables with required `type` columns.
- Type registry entries under `data.types`.
- Optional evidence and actor-detail tables used by port bullets or
  highlight-path behavior.

## Schema Choices

- Put actor visuals in `data.types.actor_types.<id>.presentation`.
- Put link visuals in `data.types.link_types.<id>.presentation`.
- Put port bullet visuals in `data.types.port_types.<id>.presentation`.
- Put cross-type graph behavior in `data.presentation`.
- Use only schema-defined tokens for colors, icons, opacity, width, line style,
  curves, and arrows.
- Use `label_policy.columns` for actor labels. Do not use canonical identity
  arrays as display text.
- Use `ports.sources[]` when `ports.show_bullets` is true.
- Use `selection.actor_click.mode: highlight_path` only with path table,
  path-member actor-column, and order-column references. Add an owner actor
  column when the same table stores different paths for different clicked
  actors.

## Implementation Steps

1. Define type-level presentation for every actor type that should have a
   domain-specific visual profile.
2. Define link presentation for every renderable link type, including direction
   arrow and curve tokens.
3. Define `port_types` and `ports.sources[]` together:
   - `source: links` reads bullets from the graph links table;
   - `source: evidence` reads bullets from a named evidence type;
   - `source: actor_table` reads bullets from a named actor detail table.
4. Make `name_column` a scalar display column. Do not use `actor_ref`,
   `link_ref`, `evidence_ref`, `array`, or `json` as bullet labels.
5. Add graph-level `data.presentation.legend`, `port_fields`, `scale_keys`, and
   `selection.actor_click`.
6. Update producer tests or fixtures so the new presentation path is exercised.

## Validation

- Validate JSON against `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Run `topologyv1.ValidateDecodedResponse()` or the function-validation tool so
  semantic references are checked.
- Add negative tests for:
  - missing label-policy columns;
  - non-display label columns;
  - missing port-bullet source tables;
  - bad highlight-path columns;
  - invalid token values.
- Check C producers with the compile command from `build/compile_commands.json`
  using `-fsyntax-only` when a full local build is blocked.

## Gotchas

- Presentation is production payload data, not compatibility reconstruction
  data.
- Type ids are producer-local until Cloud aggregation namespaces and
  canonicalizes them.
- `profile_version` is diagnostic. Do not use it to drop facts or rows.
- If a presentation source depends on optional runtime data, still declare a
  stable table/evidence type so validators can catch typos.
- Sensitive identifiers may exist in topology detail tables. Do not reference
  them in `label_policy`, graph hover, port bullet labels, logs, docs, SOWs, or
  durable review artifacts.
