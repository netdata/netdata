# query-netdata-agents -- verification questions (seed list)

This file is the **seed input** consumed by the verification
harness in SOW-0006 for direct-agent queries. The harness spawns
a Sonnet-class assistant with `../SKILL.md` + `../how-tos/INDEX.md`
+ the canonical reference docs as context, asks each question
below, captures the transcript, and grades against `grader.md`.

When the assistant cannot answer or has to perform analysis not
already documented under `../how-tos/`, the assistant must author
a new how-to and add it to the index before completing.

## Anchor: target nodes

Two targets:

- **Local desktop**: the agent reachable at `http://localhost:19999`
  (typically the user's `costa-desktop`).
- **Remote agent-events node**: the agent reachable at
  `http://${AGENT_EVENTS_HOSTNAME}:19999` with node UUID
  `${AGENT_EVENTS_NODE_ID}` and machine_guid
  `${AGENT_EVENTS_MACHINE_GUID}`.

Both use the bearer-mint flow. The harness verifies the wrapper
mints / caches / refreshes correctly for both.

## Identity (direct)

- **Q01** -- Read the agent's `/api/v3/info` directly. What is the
  node UUID, machine_guid, claim_id, agent version, and
  hostname?
- **Q02** -- What is the install prefix detected by
  `agents_netdata_prefix` on the local desktop?

## Streaming (agent-only -- Cloud has no equivalent)

- **Q03** -- Run the `netdata-streaming` Function on the agent.
  Is it acting as a parent (any incoming-direction rows)? If so,
  how many children, and what's the replication progress per
  child?
- **Q04** -- Is the agent acting as a child (any outgoing-
  direction row)? If so, what is the upstream parent host /
  endpoint?

## DynCfg (direct)

- **Q05** -- Use `GET /api/v3/config?action=tree&path=/` to list
  every configuration object on the agent. Group them by the
  top-level path (e.g. `/collectors/go.d/Jobs`,
  `/health/alerts/prototypes`, etc.) and show the count per
  group.
- **Q06** -- For one collector job (your choice), get its JSON
  Schema via `action=schema` and its current value via
  `action=get`.
- **Q07** -- Are there any vnodes? Use
  `path=/collectors/go.d/Vnodes` (and `ibm.d/Vnodes`).

## Functions (direct)

- **Q08** -- Discover every Function registered on the agent
  (use the listing endpoint or info-walk pattern). Group by
  family (table snapshot vs log explorer vs topology vs flows
  vs other).
- **Q09** -- For each of `processes`, `network-connections`,
  `mount-points`, call with `{"info":true}` and report the
  parameter set.

## Logs (direct)

- **Q10** -- Tail the last 10 entries of the system journal on
  the local desktop.
- **Q11** -- Find the last error-priority entry written to the
  systemd journal in the last hour.

## Alerts (direct)

- **Q12** -- Use `POST /api/v3/alerts` with
  `{"options":["instances"]}` to list currently-firing alerts
  on the agent. Pick one with status CRITICAL or WARNING and
  fetch its full config via `GET /api/v3/alert_config?config=...`.
- **Q13** -- Use `POST /api/v3/alert_transitions` to find every
  CLEAR -> CRITICAL transition in the last hour.

## Metrics (direct)

- **Q14** -- Use `POST /api/v3/data` to find the maximum
  `system.cpu` user dimension over the last hour, points=60.
- **Q15** -- Use `GET /api/v3/contexts` to list every metric
  context the agent currently collects, sorted alphabetically.

## Topology (direct)

- **Q16** -- Run `topology:snmp` against the local desktop with
  `{"info":true}` and report `accepted_params`. (If `topology:
  snmp` is not registered on the local desktop because no SNMP
  collector is configured, say so explicitly.)

## Flows (direct)

- **Q17** -- Run `flows:netflow` against the local desktop with
  `{"info":true}` and report `accepted_params`. (If
  `flows:netflow` is not registered, say so explicitly.)

## Token-safety self-test

- **Q18** -- Run `agents_selftest_no_token_leak`. It must print
  `[PASS]` to stderr. The captured stdout of every wrapper
  invocation in this session must not contain
  `NETDATA_CLOUD_TOKEN` bytes, `X-Netdata-Auth: Bearer
  <real-uuid>`, or any cached-bearer UUID from
  `<repo>/.local/audits/query-netdata-agents/bearers/`.

## Cross-skill (depends on the cloud skill)

- **Q19** -- Pick a node UUID from the Cloud `/nodes` listing
  (uses the cloud skill's `query-nodes.md`), then call
  `agents_query_agent` directly against it (this skill).
  Confirm both transports return the same `host[0].nm`.
