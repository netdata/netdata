# query-netdata-agents -- How-tos index

This directory holds **operational how-tos** for direct-agent
calls: short, focused recipes that combine the per-domain guides
into answers for specific questions. Each how-to documents the
question, the steps taken, the wrappers used, and the expected
output shape.

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

Filename: `<slug>.md`. Sections: Question, Inputs, Steps (each
calling one wrapper), Output, Notes / gotchas, Source guides.

Every code example must use the token-safe wrappers from
`scripts/_lib.sh` (`agents_query_cloud`, `agents_query_agent`,
`agents_call_function`). No raw curl with `Authorization: Bearer
$TOKEN` or `X-Netdata-Auth: Bearer <uuid>` literals -- that
defeats the no-token-leak guarantee.

## Index

(Populate as how-tos are authored. Stubs below correspond to the
seed verification questions in `../verify/questions.md`; replace
each `(stub -- not yet authored)` with a real link as soon as a
how-to is written.)

### Identity / hardware / OS

- `agent-info-summary.md` (stub -- not yet authored)
- `read-claim-id-direct.md` (stub -- not yet authored)
- `chart-labels-via-metrics-summary.md` (stub -- not yet authored)

### Streaming

- `incoming-children-list.md` (stub -- not yet authored)
- `outgoing-parent-target.md` (stub -- not yet authored)
- `replication-progress-per-peer.md` (stub -- not yet authored)

### Collectors / jobs / vnodes (DynCfg)

- `list-go.d-jobs-and-status.md` (stub -- not yet authored)
- `list-vnodes.md` (stub -- not yet authored)
- `read-job-config.md` (stub -- not yet authored)
- `add-go.d-job.md` (stub -- not yet authored)

### Functions

- `discover-registered-functions.md` (stub -- not yet authored)
- `call-function-info.md` (stub -- not yet authored)

### Logs

- `tail-namespace-direct.md` (stub -- not yet authored)
- `last-status-file-log-direct.md` (stub -- not yet authored)
- `histogram-by-priority.md` (stub -- not yet authored)

### Alerts

- `currently-firing-alerts-direct.md` (stub -- not yet authored)
- `alert-config-direct.md` (stub -- not yet authored)
- `transitions-last-hour-direct.md` (stub -- not yet authored)

### Topology / flows

- `topology-summary-direct.md` (stub -- not yet authored)
- `flows-top-talkers-direct.md` (stub -- not yet authored)

### Metrics

- `current-cpu-direct.md` (stub -- not yet authored)
- `peak-memory-last-hour-direct.md` (stub -- not yet authored)

## Cross-skill how-tos

When the answer needs both direct-agent and Cloud-side calls
(e.g. "discover the bearer-protected agent's claim_id from Cloud
first, then call it directly"), author the how-to under the
skill that owns the FIRST wrapper call and cross-link to the
other.
