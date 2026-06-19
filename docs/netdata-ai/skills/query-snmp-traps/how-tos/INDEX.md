# query-snmp-traps -- How-tos index

This directory holds operational SNMP trap query recipes. Each
how-to uses the token-safe wrappers from
[`query-netdata-agents`](../../query-netdata-agents/SKILL.md) and
the `snmp:traps` Function with optional `__logs_sources`
selection.

## The "if you analyze, you author a how-to" rule

The catalog is live. Every time an AI assistant or operator answers a
new concrete SNMP trap query that needs multiple wrapper calls, jq
pipelines, or cross-referencing more than one guide, add the recipe
here before completing the task.

Keep these how-tos operator-facing. Developer validation, schema
work, collector implementation, test fixtures, or SOW handoff notes
belong in `.agents/skills/` and SOW files, not in this public skill.

## How-to authoring template

Filename: `<slug>.md`.

Sections:

1. **Question** -- the operator question.
2. **Inputs** -- placeholders the operator must provide.
3. **Steps** -- runnable commands using token-safe wrappers.
4. **Output** -- what to return or inspect.
5. **Notes / gotchas** -- privacy, scale, and query caveats.
6. **Source guides** -- links to the guides used.

Do not include raw Cloud tokens, agent bearers, SNMP communities, USM
secrets, public device IPs, raw MAC addresses, customer hostnames, or
full trap payloads in durable artifacts.

## Index

- [Recent security traps from one device](./recent-security-traps-from-device.md)
- [Filter by severity across a room](./filter-by-severity-across-fleet.md)
- [Top trap senders in the last hour](./top-trap-senders-last-hour.md)
- [Inspect dedup summary entries during a flap storm](./inspect-dedup-summary-entries.md)
- [Filter an indexed varbind field and inspect `TRAP_JSON`](./search-varbind-value-in-trap-json.md)
- [Convert custom MIBs into trap profiles](./convert-custom-mibs-to-trap-profiles.md)
