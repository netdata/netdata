# Add Graph Presentation To A Topology

## Question

How does a producer add backend-controlled graph presentation (type visuals, port bullets, legend, per-actor
highlight paths) to a `netdata.topology.v1` payload without making the UI domain-specific?

## Owners

- Every token and field: `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#presentation-plane` and
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#closed-token-vocabulary`; the closed enums themselves are in
  `src/plugins.d/FUNCTION_TOPOLOGY_SCHEMA.json`.
- Modal recipes and label identification:
  `src/plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#actor-labels-and-modal-composition`.

## Steps

1. Put actor visuals in `data.types.actor_types.<id>.presentation`, link visuals in
   `data.types.link_types.<id>.presentation`, port-bullet visuals in `data.types.port_types.<id>.presentation`, and
   cross-type behavior (legend, `port_fields`, `scale_keys`, `selection.actor_click`) in `data.presentation`.
2. Define `port_types` and `ports.sources[]` together whenever `ports.show_bullets` is true. A source reads from
   `links`, a named `evidence` type, or a named `actor_table`; its `name_column` is a scalar display column, never a
   ref, array, or JSON column.
3. For highlight paths, set `selection.actor_click.mode: highlight_path` with `path_table`, `path_actor_column`
   (`actor_ref`, the path member), and `path_order_column` (numeric). When the same table stores a different path per
   clicked actor, add `path_owner_column` (`actor_ref`, the clicked actor) and keep it a separate column from the
   member column. The owner column is optional only for one shared global path.

   ```json
   {
     "mode": "highlight_path",
     "path_table": "stream_path",
     "path_owner_column": "actor",
     "path_actor_column": "path_actor",
     "path_order_column": "path_index"
   }
   ```

4. Keep path rows as actor-owned detail data (`role: actor_detail`, `owner: actor`), not graph links.
5. Update producer tests or fixtures so the new presentation path is exercised.

## Validation

- Validate against `FUNCTION_TOPOLOGY_SCHEMA.json` and run `topologyv1.ValidateDecodedData` on `data` (see
  `../SKILL.md#what-the-code-enforces` for what each layer catches).
- Add negative tests: missing label-policy columns, non-display label columns, a missing port-bullet source table, bad
  highlight-path columns, an invalid token value.
- Add a frontend fixture in which two actors have different rows in the same path table and verify each click
  resolves only that actor's path.

## Gotchas

- Reusing the owner column as the path-member column makes each click highlight only the clicked actor or its direct
  graph neighbors, because the UI never receives the ordered members. The schema allows it; only this test catches it.
- Presentation is production payload data, not compatibility reconstruction data.
- Type ids are producer-local until Cloud aggregation namespaces and canonicalizes them.
- `profile_version` is diagnostic; never use it to drop facts or rows.
- If a presentation source depends on optional runtime data, still declare a stable table or evidence type so
  validators can catch typos.
- Sensitive identifiers may exist in detail tables. Do not reference them in `label_policy`, graph hover, port-bullet
  labels, logs, docs, SOWs, or durable review artifacts.
