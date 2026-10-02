# Query agent Functions directly

This guide is part of the [`query-netdata-agents`](./SKILL.md) skill.
Read [SKILL.md](./SKILL.md#prerequisites) first for the
prerequisites (cloud token, network reachability, bearer flow).

For the response envelope (`status`, `v`, `type`, `help`,
`accepted_params`, `required_params`, `has_history`,
`update_every`, `data`), the four Function families, the canonical
protocol reference at
`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md`, and per-Function
body shapes, see
[../query-netdata-cloud/query-functions.md](../query-netdata-cloud/query-functions.md).
The agent and the Cloud proxy expose the same Function payload
shape -- the only difference is the URL and the auth header.

---

## Endpoint (agent v3)

`POST /api/v3/function?function={functionName}` on the agent at
port 19999. Path on the agent's HTTP API:

```
http://<agent>:19999/host/<node-uuid>/api/v3/function?function=<name>
```

`/api/v2/function` is also accepted on older agents -- prefer v3.

## Discover Functions on a single agent

Use `GET /api/v3/functions` for direct listing (`/api/v1/functions` is the older host catalog). These are listing
endpoints, not a generic Function named `info`. The v3 route is owned by `src/web/api/web_api_v3.c` and
`src/web/api/v2/api_v2_functions.c`. Inspect the chosen Function's advertised parameters; `{"info":true}` is a
Function-specific parameter convention, not universal discovery for arbitrary Functions.

For Cloud-side discovery across nodes, see
[List Functions on the nodes in a
room](../query-netdata-cloud/query-functions.md#list-functions-on-the-nodes-in-a-room).

## Invoke a Function via the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# processes uses command words for info mode, not a JSON info field.
agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=processes%20info' \
  | jq '{status, type, update_every, has_history, help}'
```

The `processes` handler in `src/collectors/apps.plugin/apps_functions.c` ignores JSON payload fields. For process
rows, use the `processes` command and its supported command-word filters; JSON `last` and `timeout` do not limit it.

The wrapper writes the response JSON to stdout; stderr shows the
curl invocation with `<AGENT_BEARER>` masked. Internal authentication is captured privately; endpoint responses are
not sanitized. Capture or project sensitive response fields before assistant-visible output, as the entry describes.

## When to prefer agent-direct over Cloud-proxied

- **Lower latency.** Direct skips the Cloud round-trip entirely.
- **Cloud unavailable.** This bearer wrapper needs a reusable cached token as well as Agent reachability. If it
  needs to mint or refresh, Cloud access is still required. It has no automatic transport fallback.
- **Batch fetches.** Direct calls avoid the Cloud proxy round trip; size concurrency and query windows for the
  Agent and Function. Direct transport does not imply unlimited capacity.

When the user only has Cloud access (the typical team-member case
on a remote agent), use the Cloud-proxied path documented in
[../query-netdata-cloud/query-functions.md](../query-netdata-cloud/query-functions.md)
instead.

## Limits and gotchas

- **Bearer protection:** a 412 indicates missing required authorization for that request. The wrapper always
  resolves a bearer before calling; it does not detect protection or retry on 412. See the
  [authentication reference](./authentication.md#protection-and-headers).
- **Function names:** after access checks, an unknown registered name returns 404. A missing or empty `function`
  parameter returns 400 (`src/nrpc/nrpc-registry.c`, `src/web/api/v1/api_v1_function.c`). Use the catalog spelling.
- **Response is not streamed.** Even on the agent, the Function
  response is buffered into a single JSON document.
- **The `cfg` field of an alert instance**, `claim_id`, `node_id`,
  and similar UUID values appear in responses. Treat as
  semi-sensitive; never paste raw responses into committed files.

## See also

- [../query-netdata-cloud/query-functions.md](../query-netdata-cloud/query-functions.md)
  -- canonical Function reference, response envelope, the four
  families, the `info` widget schema, developer references.
- [../query-netdata-cloud/query-logs.md](../query-netdata-cloud/query-logs.md),
  [query-topology.md](../query-netdata-cloud/query-topology.md),
  [query-flows.md](../query-netdata-cloud/query-flows.md) --
  per-family deep dives (the body shapes apply to direct-agent
  calls verbatim).
