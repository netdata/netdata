# query-netdata-cloud -- How-tos index

This directory holds **operational how-tos**: short, focused
recipes that combine the per-domain guides into answers for
specific questions. Each how-to documents the question, the steps
taken, the wrappers used, and the expected output shape.

## The "if you analyze, you author a how-to" rule

The how-tos catalog is meant to be **live**. Every time an AI
assistant (or human) is asked a question that:

1. The user expects a concrete answer to, AND
2. Is not already documented in this index, AND
3. Forces analysis (multiple wrapper calls, jq pipelines, or
   cross-referencing more than one per-domain guide)

the assistant MUST author a new how-to in this directory and add
it to the index BELOW before completing the task.

This is mandatory. Skipping it means the next assistant repeats
the same analysis from scratch.

## How-to authoring template

Filename: `<slug>.md` (e.g. `find-node-id-by-hostname.md`).

Sections:

1. **Question** -- the user-visible question, verbatim or
   paraphrased.
2. **Inputs** -- what the user must supply (space, hostname,
   time range, etc.).
3. **Steps** -- numbered, each calling exactly one wrapper from
   `query-netdata-agents/scripts/_lib.sh`.
4. **Output** -- what the assistant returns to the user.
5. **Notes / gotchas** -- edge cases, follow-ups, related
   how-tos.
6. **Source guides** -- cross-links to the per-domain guides
   used.

Every code example must use the token-safe wrappers
(`agents_query_cloud`, `agents_query_agent`,
`agents_call_function`). No raw curl with `-H "Authorization:
Bearer $TOKEN"` -- that defeats the no-token-leak guarantee.

## Index

(Populate as how-tos are authored. Stubs below correspond to the
seed verification questions in `../verify/questions.md`; replace
each `(stub -- not yet authored)` with a real link as soon as a
how-to is written.)

### Identity / hardware / OS

- `find-node-id-by-hostname.md` (stub -- not yet authored)
- `find-node-hardware-specs.md` (stub -- not yet authored)
- `find-node-os.md` (stub -- not yet authored)

### Streaming / parents / vnodes

- `is-node-a-parent-and-children.md` (stub -- not yet authored)
- `is-node-a-child-and-parent-target.md` (stub -- not yet authored)
- `list-vnodes-on-node.md` (stub -- not yet authored)

### Collectors / jobs

- `find-failed-collection-jobs.md` (stub -- not yet authored)
- `is-collector-monitoring-X-and-frequency.md` (stub -- not yet authored)

### Top processes

- `pid-with-biggest-memory-and-app-group.md` (stub -- not yet authored)

### Alerts

- `currently-firing-alerts-in-room.md` (stub -- not yet authored)
- `alert-config-by-cfg-hash.md` (stub -- not yet authored)
- `silenced-alerts.md` (stub -- not yet authored)

### Logs / status file

- `last-netdata-status-file-log.md` (stub -- not yet authored)
- `recent-error-logs-in-namespace.md` (stub -- not yet authored)

### Topology / flows

- `local-l2-topology-summary.md` (stub -- not yet authored)
- `top-flow-talkers-last-hour.md` (stub -- not yet authored)

### Members / rooms / feed

- `members-by-role-in-space.md` (stub -- not yet authored)
- `rooms-with-most-nodes.md` (stub -- not yet authored)
- `node-state-changes-last-hour.md` (stub -- not yet authored)

## Cross-skill how-tos

When the answer needs both Cloud-side and direct-agent-side calls
(e.g. "find the parent of a stale node, then read its
streaming-state directly"), author the how-to under the skill
that owns the FIRST wrapper call and cross-link to the other.
