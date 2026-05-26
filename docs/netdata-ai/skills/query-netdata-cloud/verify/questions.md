# query-netdata-cloud -- verification questions (seed list)

This file is the **seed input** consumed by the verification
harness in SOW-0006. The harness spawns a Sonnet-class assistant
with `../SKILL.md` + `../how-tos/INDEX.md` + the canonical
reference docs as context, asks each question below, captures the
transcript, and grades against `grader.md` (delivered by SOW-0006).

When the assistant cannot answer or has to perform analysis not
already documented under `../how-tos/`, the assistant must author
a new how-to and add it to the index before completing.

## Anchor: target node

Every question is asked against the same target node:

> Node `costa-desktop` -- a node visible to the user in their
> Netdata Cloud (any space the cloud token can read; the
> harness must locate it).

Resolving `costa-desktop` from a hostname to a `node UUID` is
itself the first verification (see Q01).

## Identity / hardware / OS

- **Q01** -- Find the node UUID and machine GUID of the node
  whose hostname is `costa-desktop`. Which space and which
  rooms is it in?
- **Q02** -- What are the hardware specs of `costa-desktop`?
  CPU architecture, core count, total RAM, total disk space.
- **Q03** -- What operating system does `costa-desktop` run?
  Distribution name, version, kernel version.
- **Q04** -- Which cloud provider, region, and instance type is
  `costa-desktop` running on (if any)?
- **Q05** -- What is the agent version on `costa-desktop`, and
  what is its claim_id?

## Streaming / parent / child / vnodes

- **Q06** -- Is `costa-desktop` acting as a parent? If so, how
  many children stream to it, and what are their hostnames /
  node UUIDs?
- **Q07** -- Is `costa-desktop` a child? If so, which parent
  does it stream to (host or endpoint)?
- **Q08** -- Does `costa-desktop` have any virtual nodes
  configured? Which? Are they running?

## Collection / jobs / DynCfg

- **Q09** -- Are there any failed data-collection jobs on
  `costa-desktop`? Which?
- **Q10** -- Does `costa-desktop` monitor nvidia DCGM (the
  go.d.plugin `nvidia_smi` collector or equivalent)? At what
  data-collection frequency (`update_every`)?
- **Q11** -- List every `go.d.plugin` collector job currently
  RUNNING on `costa-desktop`, sorted alphabetically.

## Processes / top

- **Q12** -- Which PID is currently consuming the most resident
  memory on `costa-desktop`? What is its command line, and which
  app-group / dashboard category does the dashboard show it
  under?

## Alerts

- **Q13** -- What alerts are currently firing (not CLEAR) in
  the room that contains `costa-desktop`? List by status
  (CRITICAL / WARNING) with alert name and instance.
- **Q14** -- Get the full configuration definition of one
  currently-firing alert from Q13 (selectors, thresholds,
  notification settings).
- **Q15** -- Are there any active or scheduled silencing rules
  in the space?

## Logs / status file

- **Q16** -- What was the last record written to the
  `agent-events` journal namespace on the `agent-events` node?
- **Q17** -- Find the last 5 entries with PRIORITY <= warning
  (i.e. err / crit / alert / emerg) in the system journal of
  `costa-desktop` over the last 24 hours.

## Topology

- **Q18** -- Run the L2 topology Function on `costa-desktop`
  (`topology:snmp`). How many actors and links are reported,
  and what discovery protocols are used (LLDP / CDP / FDB /
  STP)?

## Flows

- **Q19** -- Are network flows being collected on
  `costa-desktop`? If yes, list the top-10 source-AS / dest-AS
  pairs by bytes over the last hour.

## Rooms / members / feed

- **Q20** -- How many rooms are in the user's space, and which
  is the largest by `node_count`?
- **Q21** -- List the active admins of the space.
- **Q22** -- List all `node-state-stale` and `node-state-offline`
  events in the last hour from the audit feed.

## Self-test invariants

- **Q23** -- After answering all questions above, confirm that
  no NETDATA_CLOUD_TOKEN bytes, no agent-bearer UUID values,
  and no claim_id values appeared in any printed output during
  this session.
