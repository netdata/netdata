# query-netdata-cloud -- How-tos index

This directory holds **operational how-tos**: short, focused
recipes that combine the per-domain guides into answers for
specific questions. Each how-to documents the question, the steps
taken, the wrappers used, and the expected output shape.

## Knowledge Capture

Capture timing, authorization, and audience boundaries follow
[the skill's Knowledge Capture section](../SKILL.md#knowledge-capture).

## How-to authoring template

Filename: `<slug>.md` (e.g. `find-node-id-by-hostname.md`).

Sections:

1. **Question** -- the user-visible question, verbatim or
   paraphrased.
2. **Inputs** -- what the user must supply (space, hostname,
   time range, etc.).
3. **Steps** -- numbered, with the commands and local processing needed for the question.
4. **Output** -- what the assistant returns to the user.
5. **Notes / gotchas** -- edge cases, follow-ups, related
   how-tos.
6. **Source guides** -- cross-links to the per-domain guides
   used.

Credential-bearing requests MUST use the shared query wrappers. Ordinary local processing and unauthenticated
probes MAY use ordinary commands under [Safe Execution](../SKILL.md#safe-execution). Masked request logs do not
sanitize response bodies or arbitrary body fields; choose private capture or an appropriate output projection.

## Index

Only existing recipes are listed. Add a link when the recipe is written.

### Metrics / fleet SLOs

- [`compare-explicit-and-room-wide-node-scope.md`](./compare-explicit-and-room-wide-node-scope.md) -- compare a fixed UUID scope with the current room-wide node scope; explains why all-room queries omit `scope.nodes` and use `selectors.nodes: ["*"]`, why a large UUID selector is redundant and expensive, and how to pass large payloads through the token-safe wrapper with `@file`.
- [`fleet-connectivity-slo-queries.md`](./fleet-connectivity-slo-queries.md) -- single-dimension fleet percentages (percent of devices connected/streaming, percent of devices with a boolean dimension at 1 or 0) and ranking devices by percent of time a boolean dimension was 0; includes the average-of-boolean trick and the countif-through-Cloud caveat.

### Streaming / parents / vnodes

- [`diagnose-no-data-on-zoom-parent-retention-gaps.md`](./diagnose-no-data-on-zoom-parent-retention-gaps.md) -- why a node shows "No data" when zooming in while wider zoom renders fine: identify the serving agent from jsonwrap `.agents`, compare forced-tier queries (tier 0 vs 1 vs 2), reduce all-null rows to gap runs, run the decisive control test (does the PARENT's own local data have the same tier0 hole?), read the parent's daemon log via the `windows-events`/`systemd-journal` Function for `DBENGINE` write errors, and quantify child streaming flapping via `netdata.streaming_outbound` `replicating` buckets.

### Topology / flows

- [`group-network-topology-by-kubernetes-pod.md`](./group-network-topology-by-kubernetes-pod.md) -- summarize `topology:network-connections` process actors by Kubernetes pod and namespace through Cloud.
- [`find-containers-for-topology-port.md`](./find-containers-for-topology-port.md) -- find containers or pods exposing a specific TCP port from the Cloud topology Function payload.
- [Check an installed Cloud-connected flow Function](./validate-local-netflow-function.md) -- operator diagnosis of
  Function availability and a real query; producer/schema contract validation belongs in the project developer skill.

## Cross-skill how-tos

When the answer needs both Cloud-side and direct-agent-side calls
(e.g. "find the parent of a stale node, then read its
streaming-state directly"), author the how-to under the skill
that owns the FIRST wrapper call and cross-link to the other.
