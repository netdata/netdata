---
name: query-snmp-traps
description: Query, explain or review SNMP trap logs and recipes through Cloud or an Agent, including severity, senders, dedup, decode errors and TRAP_* fields. Guide installed custom-MIB conversion; profile development belongs to collectors-snmp-trap-profiles.
---

# Query SNMP Traps

Use this operator skill for the `snmp:traps` Function provided by the `snmp_traps` collector. Direct-journal jobs
appear in `__logs_sources`, normally by listener job name. OTLP-only jobs (`journal.enabled: false`) create no local
journal files and do not appear as local log sources.

## Choose The Task

- **Explain or review:** read the selected recipe, field reference or helper as reference material. Do not source
  helpers, load credentials or query live nodes just because this skill was loaded.
- **Run a requested trap query:** follow [Safe Execution](#safe-execution), [Standard Setup](#standard-setup) and the
  selected recipe. Existing authorization covers ordinary query steps.
- **Convert custom MIBs for an installed Agent:** follow the operator conversion recipe. Conversion/review does not
  authorize installing profiles, restarting jobs or sending MIB content to a classifier endpoint; those operations
  must be within the user's requested scope.
- **Develop collector code:** use the project `collectors-authoring` skill and the collector's developer owners.
  Profile/schema work and stock-profile regeneration use `collectors-snmp-trap-profiles`; query recipes remain
  available for authorized operator checks.

## Guides

| Task | How-to |
|---|---|
| Recent security traps from one device | [how-tos/recent-security-traps-from-device.md](./how-tos/recent-security-traps-from-device.md) |
| Critical and emergency traps across a room | [how-tos/filter-by-severity-across-fleet.md](./how-tos/filter-by-severity-across-fleet.md) |
| Top trap senders in the last hour | [how-tos/top-trap-senders-last-hour.md](./how-tos/top-trap-senders-last-hour.md) |
| Dedup summaries during a flap storm | [how-tos/inspect-dedup-summary-entries.md](./how-tos/inspect-dedup-summary-entries.md) |
| Filter by indexed varbind fields; inspect `TRAP_JSON` when needed | [how-tos/search-varbind-value-in-trap-json.md](./how-tos/search-varbind-value-in-trap-json.md) |
| Convert custom MIBs into trap profiles | [how-tos/convert-custom-mibs-to-trap-profiles.md](./how-tos/convert-custom-mibs-to-trap-profiles.md) |
| Operational how-tos catalog | [how-tos/INDEX.md](./how-tos/INDEX.md) |

## Safe Execution

Credential-bearing calls MUST use the [shared Agent query helpers](../query-netdata-agents/SKILL.md#safe-execution).
Their masked command logging does not sanitize responses or arbitrary body fields. Do not paste Cloud tokens,
Agent bearers or token-bearing curl commands. Use the
[Cloud logs request contract](../query-netdata-cloud/query-logs.md)
for log query parameters; trap-specific selection and interpretation belong here.

- Use structured selections first. The Function already scopes to SNMP trap journals; `selections.__logs_sources`
  selects a listener job. Usually narrow `TRAP_REPORT_TYPE` to `trap`, `deduplication_summary` or `decode_error`.
  Use full-text `query` only as residual search over the narrowed results.
- Cloud-proxied calls target one node. For a room/fleet, list nodes, call each node's Function and aggregate locally;
  the fleet recipe handles current-run isolation, failures and severity-facet filtering.
- Trap rows, communities, USM secrets, MACs, usernames, public device IPs, customer hostnames and full `TRAP_JSON`
  MUST NOT enter durable artifacts. Return summaries; raw output MAY be inspected privately when the user explicitly
  needs it locally. A hostname/message projection is not automatically sanitized.
- Capture raw responses in each recipe's private run directory. Local processing MAY use ordinary jq/shell commands;
  wrapper use is required for credential-bearing requests, not every command in a recipe.

## Trap Field Reference

[fields.md](./fields.md) covers report types, severities, source identity, enrichment, indexed `TRAP_VAR_*`,
`TRAP_JSON`, suppression and decode-error fields. Read it when interpreting fields; ordinary setup does not need the
full field table.

## Standard Setup

For an authorized live query, run from the repository checkout with Bash, Git, jq and curl. Follow the shared
[configuration instructions](../query-netdata-agents/authentication.md#configuration) for local credentials:

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
```

Cloud-proxied query, preferred by default; capture discovery privately:

```bash
NODE_UUID="YOUR_NODE_UUID"
SNMP_TRAPS_FUNCTION="snmp:traps"

TRAPS_INFO_JSON="$(agents_call_function \
  --via cloud \
  --node "$NODE_UUID" \
  --function "$SNMP_TRAPS_FUNCTION" \
  --body '{"info":true}')"
```

Direct-agent query when that route is requested. During a Cloud outage it needs a bearer accepted by the cache
policy; otherwise the helper still needs Cloud access to mint. There is no automatic transport fallback. See the
[Agent transport contract](../query-netdata-agents/SKILL.md#choose-the-task).

```bash
NODE_UUID="YOUR_NODE_UUID"
AGENT_HOST="agent.example.invalid:19999"
MACHINE_GUID="YOUR_MACHINE_GUID"
SNMP_TRAPS_FUNCTION="snmp:traps"

TRAPS_INFO_JSON="$(agents_call_function \
  --via agent \
  --node "$NODE_UUID" \
  --host "$AGENT_HOST" \
  --machine-guid "$MACHINE_GUID" \
  --function "$SNMP_TRAPS_FUNCTION" \
  --body '{"info":true}')"
```

## Row Decoding Helper

Use [Row Decoding](./fields.md#row-decoding) for column-indexed log arrays and private decoded rows.

## Source Selection

Start with `{"info":true}` for `snmp:traps` and inspect the
`__logs_sources` required parameter. By default, the SDK selects all
direct-journal sources. To target one listener job, add:

```json
{
  "selections": {
    "__logs_sources": ["local"]
  }
}
```

If a job is missing from `__logs_sources`, verify it exists and `journal.enabled` is not `false`. Check the running
Function response and availability: with no direct-journal trap sources, it can return no sources or an unavailable
response. A visible Function name alone does not prove sources exist. The collector handler
`src/go/plugin/go.d/collector/snmp_traps/internal/snmptrapsfunc/func_logs.go` owns that availability check.

## Knowledge Capture

- For answer-only questions, deliver the requested answer. If the work reveals a reusable, evidence-backed recipe
  not already documented, you MUST preserve a sanitized note with the finding, supporting evidence, and proposed
  owning guide. In a repository checkout, use `<repo-root>/.local/audits/<subject>/followups.md`, reusing this skill's
  audit directory when available. Outside a checkout, use an appropriate local workspace. If no writable workspace
  is available, include the sanitized follow-up in the response instead.
- Briefly report reusable documentation discoveries and proposed updates in the answer-only final response, even
  when recorded locally. Obtain authorization before those guide edits; do not delay the answer while awaiting it.
- During authorized implementation, you MUST update this skill or its guides for reusable, evidence-backed
  discoveries made while doing the work, even when the documentation is not required for the code change. This
  needs no separate authorization. Keep [`how-tos/INDEX.md`](./how-tos/INDEX.md) consistent and report the updates.
- Guide edits arising from answer-only questions require separate authorization. Documentation capture records
  observed behavior; it does not authorize additional implementation or new product contracts. Commit and
  publication require authorization too.
- Prefer updating an existing guide over duplicating it. Keep recipes operator-facing: querying and interpreting
  SNMP traps. Developer validation, schema work, collector implementation, fixtures, and project handoff notes
  belong in project developer documentation, not in this public skill.

## See Also

- [Cloud log Function guide](../query-netdata-cloud/query-logs.md)
- [Direct-agent log Function guide](../query-netdata-agents/query-logs.md)
- [Generic Function invocation through Cloud](../query-netdata-cloud/query-functions.md)
- [Generic direct-agent Function invocation](../query-netdata-agents/query-functions.md)
- [SNMP trap profile format](../../../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md)
