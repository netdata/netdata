# Define Per-Actor Highlight Paths

## Question

How should a `netdata.topology.v1` producer encode highlight paths when each
clicked actor has its own ordered path?

## Inputs

- A producer that needs `data.presentation.selection.actor_click.mode:
  highlight_path`.
- An actor detail table that stores path rows.
- Actor row references for the clicked actor and every member of its path.

## Schema Choices

Use one actor detail table with:

- `path_owner_column`: optional `actor_ref`; the actor whose click should use
  the row.
- `path_actor_column`: required `actor_ref`; the actor that appears in the
  highlighted path.
- `path_order_column`: required numeric column; the deterministic path order.

Do not point `path_actor_column` at the owner column unless every actor shares
one global path table. For producer-specific per-actor paths, owner and member
must be separate columns.

## Implementation Steps

1. Define the table type with `role: actor_detail`, `owner: actor`, and
   `aggregation: append`.
2. Add an owner actor-ref column such as `actor`.
3. Add a path-member actor-ref column such as `path_actor`.
4. Add a numeric order column such as `path_index`.
5. Set `data.presentation.selection.actor_click` to:

```json
{
  "mode": "highlight_path",
  "path_table": "stream_path",
  "path_owner_column": "actor",
  "path_actor_column": "path_actor",
  "path_order_column": "path_index"
}
```

## Validation

- Validate the payload with `FUNCTION_TOPOLOGY_SCHEMA.json`.
- Run the topology v1 validator. It checks that owner/member columns are
  `actor_ref` and the order column is numeric.
- Add a frontend fixture where two actors have different rows in the same path
  table, then verify each click resolves only that actor's path.

## Gotchas

- The owner column is intentionally optional for backward compatibility and for
  a single shared global path.
- Reusing the owner column as the path-member column causes each actor click to
  highlight only itself or direct graph neighbors, because the UI never receives
  the ordered path members.
- Keep path rows as actor-owned detail data, not graph links. Graph links remain
  the compact renderable topology; path rows drive selection behavior.
