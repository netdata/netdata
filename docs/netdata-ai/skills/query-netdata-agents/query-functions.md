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

Most agents expose a function-listing surface through the same
generic Function call with `function=info`-like discovery. To
enumerate by name, query each Function with `{"info":true}`. For
a top-level list, use the Cloud-side functions endpoint via
[../query-netdata-cloud/query-functions.md#list-available-functions](../query-netdata-cloud/query-functions.md#list-available-functions);
the Cloud listing is authoritative even when you ultimately call
the agent directly.

## Invoke a Function via the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# Discover the parameter set (info=true is the safe first call).
agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=processes' '{"info":true}'

# Real query (after `info` told you the parameters).
agents_query_agent \
    --node "$AGENT_EVENTS_NODE_ID" \
    --host "$AGENT_EVENTS_HOSTNAME:19999" \
    --machine-guid "$AGENT_EVENTS_MACHINE_GUID" \
    POST '/api/v3/function?function=processes' '{"last":50,"timeout":30000}'
```

The wrapper writes the response JSON to stdout; stderr shows the
curl invocation with `<AGENT_BEARER>` masked (the bearer is
minted/cached/refreshed internally and never reaches stdout).

## When to prefer agent-direct over Cloud-proxied

- **Lower latency.** Direct skips the Cloud round-trip entirely.
- **Cloud unavailable.** The agent answers as long as port 19999
  is reachable from your workstation.
- **High-frequency batch fetches.** The Cloud may rate-limit
  function calls; the agent does not.

When the user only has Cloud access (the typical team-member case
on a remote agent), use the Cloud-proxied path documented in
[../query-netdata-cloud/query-functions.md](../query-netdata-cloud/query-functions.md)
instead.

## Limits and gotchas

- **Bearer protection**: a 412 response from the agent means the
  agent is bearer-protected. The `agents_query_agent` wrapper
  handles this transparently (mint via Cloud, cache, refresh).
  Direct curl fails until you mint a bearer.
- **Function name is case-sensitive.** Wrong casing returns 400.
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
