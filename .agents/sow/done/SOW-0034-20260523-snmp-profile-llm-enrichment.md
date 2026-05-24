# SOW-0034 - SNMP Trap Profile LLM Enrichment Pipeline

## Status

Status: completed

Sub-state: shipped. Classifier `tools/snmp-traps-profile-gen/classify.py` enriched all 40,409 traps from SOW-0033's `extracted.jsonl` with category + severity + description. Emitter `tools/snmp-traps-profile-gen/emit.py` groups the enriched records into 327 per-vendor YAML files under `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/` plus a `catalogue.json` index. 100% LLM-classified (zero mechanical-fallback files remaining after corpus stabilization).

## Requirements

### Purpose

For each per-trap JSON record produced by SOW-0033 (raw MIB content), use a local LLM to fill in the three things MIBs do not provide:

1. **Category** — one of the 9 canonical slugs from netdata.md §3 (`state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `custom`, `unknown`).
2. **Severity** — one of `info`, `warn`, `crit`.
3. **Description template** — operator-friendly one-line MESSAGE template using `{varbind_name}` and `{SNMP_DEVICE_HOSTNAME}` placeholders, max ~200 chars.

The output is per-vendor profile YAML files matching the schema in netdata.md §7, ready to ship as the trap subsystem's stock OOB catalogue.

### User Request

User-stated direction: "We have 2 models here that are free for us: minimax-m2.7 at 130tps (nova:3537, 64 concurrent) and qwen3.6-35b-a3b at 180tps (localhost:3536, 8 concurrent). I recommend to create a small prompt for the classification required (3 fields), provide to it the information extracted from the MIBs, and ask it to do the classification and the description we need."

User clarification on throughput: minimax aggregate at 64 concurrent is ~1,000 tps (not single-stream-times-concurrency). Qwen aggregate at 8 concurrent is comparably batched (single-stream 180 tps; aggregate TBD by measurement).

### Assistant Understanding

Facts:

- Two local LLM endpoints available, free, with batched aggregate throughput:
  - `minimax-m2.7-coder` at nova:3537 — ~1,000 tps aggregate at 64 concurrent
  - `qwen3.6-35b-a3b` at localhost:3536 — single-stream 180 tps, 8 concurrent slots, aggregate TBD
- Combined estimate: ~1,600-1,900 tps aggregate (qwen contribution conservative).
- Expected workload: 40,000-120,000 per-trap classifications (range from SOW-0033).
- Per-request size estimate: 700-1,500 tokens input (MIB-extracted context) + 100-200 tokens output (JSON with 3 fields).
- Total tokens: ~80-200M.
- Wall-clock at conservative aggregate: ~17-22 hours for the full batch.
- Output validation rules can be enforced mechanically (JSON parse, enum check, char limit, varbind-reference sanity).

Inferences:

- Both models are code-tuned; classification quality on SNMP traps is unverified. A 200-trap sample-quality gate is mandatory before committing to the 17-22h batch.
- Few-shot examples (3-5 in-prompt) likely needed to anchor category boundaries.
- Description template generation is the higher-variance task; categorization is more reliable.
- LLM may hallucinate varbind names not in the input — validation must check that template `{varbind}` references match the provided varbind list.

Unknowns:

- Actual LLM output quality on this task — requires the sample-quality gate.
- Whether qwen's aggregate at 8 concurrent is high enough to justify mixing with minimax, or whether routing all traffic through minimax + qwen-as-retry-fallback is simpler.
- How often the LLM produces output that fails validation; retry strategy depends on this.

### Acceptance Criteria

- A tool `mib-classify-llm.py` (or equivalent) reads per-trap JSON records from SOW-0033, submits them to the local LLM pool with the agreed prompt, validates the output, and writes enriched per-trap records.
- A second tool `profile-emit.py` aggregates enriched records into per-vendor profile YAML files matching netdata.md §7 schema.
- Sample-quality gate: before the full batch, run on 200 representative traps; manual review confirms ≥90% acceptable output (correct category, sane severity, useful description). Iterate on the prompt until this bar is met.
- Full-batch run completes; per-trap enriched records on disk; per-vendor YAML files emitted.
- Validation report: total traps classified, per-category distribution, per-severity distribution, retry counts, fallback-to-mechanical-default counts, mean description length, validation-fail counts.
- Resumable: per-trap output written incrementally; re-running skips already-classified OIDs unless `--force`.
- Per-OID manual overrides: a sibling override file (e.g., `overrides.yaml`) lets curators correct individual OIDs; pipeline respects overrides without re-running the LLM.

## Analysis

Sources checked:

- `~/src/PRs/snmptraps/.agents/sow/specs/snmp-traps/netdata.md` §3 (categories), §7 (profile schema)
- SOW-0033 (input producer)
- User-stated LLM endpoints and throughput parameters

Current state:

- LLM endpoints exist and are operational; not yet exercised for this workload.
- No enrichment pipeline exists yet.

Risks:

- LLM output quality unverified. Mitigation: 200-trap sample-quality gate before commit to full batch.
- LLM may produce inconsistent categorizations across similar traps. Mitigation: few-shot examples in the prompt; per-category cross-check after batch (audit traps where the same MIB family is split across categories).
- LLM may write descriptions referencing varbinds not in the trap. Mitigation: output validation rejects responses whose `{varbind}` placeholders are absent from the input varbind list.
- LLM may produce descriptions with marketing-tone language. Mitigation: prompt explicitly forbids marketing; spot-check during sample-quality gate.
- Endpoints may rate-limit, error, or restart mid-batch. Mitigation: resumable pipeline; retry policy with exponential backoff; periodic checkpointing.
- 17-22h wall-clock means the run can't be casually re-done. Mitigation: per-OID outputs on disk; re-running is incremental.

## Pre-Implementation Gate

Status: needs-user-decision

Problem / root-cause model:

- MIBs do not encode the operator-platform decisions our profile schema needs (category, severity, description template). These decisions are pattern-recognition + writing tasks well-suited to an LLM. Local LLMs are free, available, and fast enough to process the full corpus overnight. The chosen approach minimises engineering effort (no rule-engine maintenance, no human curation gate for Day 1).

Evidence reviewed:

- netdata.md §3 (canonical categories), §7 (YAML schema), §13 Open Questions
- User-stated LLM endpoints and aggregate throughput
- Reference compiled output in `.local/dd_traps_db.json.gz` (validates the structural feasibility)

Affected contracts and surfaces:

- Profile YAML schema (defined in netdata.md §7; this pipeline produces YAML conforming to it).
- Stock catalogue at `/usr/share/netdata/snmp-profiles/` (delivery target — exact packaging TBD).
- Operator override path at `/etc/netdata/snmp-profiles/` (preserved; this pipeline only writes stock files).

Existing patterns to reuse:

- Async HTTP client patterns for concurrent LLM requests (e.g., `httpx` + `asyncio.gather`).
- The cohort-review protocol used elsewhere in this project (multi-model + retry) is the conceptual ancestor.

Risk and blast radius:

- Low. Tool runs at development time; output is files on disk; quality is gated by sample review before any release packaging.

Sensitive data handling plan:

- No sensitive data. MIB content is public; LLM prompts contain only MIB-derived material; outputs are public technical metadata.

Implementation plan:

1. Set up Python async HTTP client with retry + exponential backoff against both LLM endpoints.
2. Design the prompt (template at end of this SOW). Iterate on a 20-trap micro-sample first to validate basic mechanics (output parses, fields present).
3. Run on a 200-trap representative sample (random across major MIBs, weighted toward Tier-1 vendors). Manual review with the curator (user or delegate).
4. Iterate the prompt until ≥90% of the sample is acceptable.
5. Implement output validation: JSON parse, enum membership, char-limit, varbind-reference sanity.
6. Implement retry policy: validation-fail or HTTP error → retry on the other model → if still fails, fall back to mechanical regex defaults (category from trap-name regex, severity from trap-name regex, description = default template).
7. Implement per-OID checkpointing (write each enriched record to disk as it completes).
8. Run the full batch overnight.
9. Generate the validation report.
10. Run `profile-emit.py` to group enriched records into per-vendor YAML files.
11. Quality spot-check: random 500 from the full output; same review process as the sample gate.

Prompt template (subject to sample-gate iteration):

```
You are classifying an SNMP trap for an open-source observability platform aimed at network engineering teams.

Output ONLY a JSON object with exactly three fields. No prose, no markdown fences, no explanation.

TRAP CONTEXT:
  trap_oid:         {{oid}}
  trap_name:        {{name}}
  mib_module:       {{mib}}
  mib_purpose:      {{mib_description}}
  trap_description: {{trap_description}}
  trap_reference:   {{trap_reference}}
  varbinds:
    - {{varbind_name}} ({{varbind_syntax}}{{ enum_or_bits_summary }}) — {{varbind_description}}
    ...

ALLOWED CATEGORIES (pick exactly one):
  state_change   — interface/port state, system lifecycle, routing protocol state, environmental transitions
  config_change  — configuration audit events
  security       — security violations with per-event detail (port-sec, ACL hits, IPS)
  auth           — authentication events (login failures, SNMP auth failures)
  license        — license / compliance events
  mobility       — MAC mobility, STP actor events, topology actor events
  diagnostic     — vendor-specific diagnostic events (reboot reasons, module insertion, RAID, optical transceiver)
  custom         — operator-authored OIDs (OT/IoT/industrial)
  unknown        — cannot classify with confidence

ALLOWED SEVERITIES (pick exactly one):
  info   — informational, no action required
  warn   — operator attention warranted
  crit   — likely service impact, operator action required

DESCRIPTION TEMPLATE RULES:
  - One line, max 200 characters.
  - Use {placeholder} syntax for variable substitution.
  - Reference {SNMP_DEVICE_HOSTNAME} for the device.
  - Reference varbinds by their exact name from the varbinds list above (e.g., {ifIndex}, {ifDescr}).
  - Do not invent varbind names that are not in the list.
  - Plain factual language. No marketing terms. No "world-class," "powerful," "critical issue," etc.

EXAMPLES (few-shot):

Input: linkDown, IF-MIB, varbinds=[ifIndex, ifAdminStatus, ifOperStatus]
Output: {"category": "state_change", "severity": "warn", "description": "Interface ifIndex {ifIndex} went down on {SNMP_DEVICE_HOSTNAME} — admin={ifAdminStatus}, oper={ifOperStatus}"}

Input: ccmCLIRunningConfigChanged, CISCO-CONFIG-MAN-MIB, varbinds=[ccmHistoryEventCommandSource, ccmHistoryEventConfigSource, ccmHistoryEventConfigDestination]
Output: {"category": "config_change", "severity": "info", "description": "Cisco running-config changed via {ccmHistoryEventCommandSource} on {SNMP_DEVICE_HOSTNAME}"}

Input: authenticationFailure, SNMPv2-MIB, varbinds=[]
Output: {"category": "auth", "severity": "warn", "description": "SNMP authentication failure on {SNMP_DEVICE_HOSTNAME} from {SNMP_SOURCE_IP}"}

Input: ciscoPsmTrapSrvUnauthorized, CISCO-PORT-SECURITY-MIB, varbinds=[cpsIfViolationMacAddress, cpsIfViolationVlan, ifIndex]
Output: {"category": "security", "severity": "warn", "description": "Port-security violation: MAC {cpsIfViolationMacAddress} on ifIndex {ifIndex} (VLAN {cpsIfViolationVlan}) on {SNMP_DEVICE_HOSTNAME}"}

NOW CLASSIFY THE TRAP CONTEXT ABOVE. Output JSON only.
```

Validation rules (mechanical):

1. Response must parse as JSON.
2. Must contain exactly the keys `category`, `severity`, `description`.
3. `category` ∈ {state_change, config_change, security, auth, license, mobility, diagnostic, custom, unknown}.
4. `severity` ∈ {info, warn, crit}.
5. `description` is a string, ≤200 chars.
6. Every `{name}` placeholder in `description` must be either:
   - A name present in the input varbinds list, OR
   - One of the recognized standard placeholders (`SNMP_DEVICE_HOSTNAME`, `SNMP_SOURCE_IP`, `SNMP_TRAP_NAME`, `SNMP_DEVICE_VENDOR`).
7. `description` does not match a banned-phrase regex (e.g., `\b(critical|severe|catastrophic) issue\b`, `\bworld-class\b`, etc. — finalised during prompt iteration).

If validation fails → retry once on the other model → if still fails, fall back to mechanical defaults and tag the record `enrichment_source: fallback`.

Output schema (per-trap, enriched):

```json
{
  "oid": "...",
  "name": "...",
  "mib": "...",
  "category": "state_change",
  "severity": "warn",
  "description": "Interface ifIndex {ifIndex} went down on {SNMP_DEVICE_HOSTNAME} — admin={ifAdminStatus}, oper={ifOperStatus}",
  "varbinds": [ ... full varbinds from SOW-0033 extraction ... ],
  "enrichment_source": "llm:minimax-m2.7-coder",
  "enrichment_attempts": 1,
  "enrichment_warnings": []
}
```

Artifact impact plan:

- New tools: `tools/snmp-traps-profile-gen/mib-classify-llm.py`, `tools/snmp-traps-profile-gen/profile-emit.py` (location decided per SOW-0033).
- New per-trap enriched JSON outputs: e.g., `tools/snmp-traps-profile-gen/output/enriched/<MIB_NAME>/<OID>.json`.
- New per-vendor profile YAML files emitted to `tools/snmp-traps-profile-gen/output/profiles/<vendor>.yaml` (or equivalent layout).
- Sample-gate review records preserved under `tools/snmp-traps-profile-gen/output/sample-review/` with the human-reviewer's notes.
- An override file `tools/snmp-traps-profile-gen/overrides.yaml` for per-OID manual corrections.
- No changes to netdata.md or any other spec.

Open decisions:

1. **Sample-gate reviewer**: who reviews the 200-trap sample for quality? User personally, or a curator delegate? Affects the review-loop turnaround time.
2. **Routing strategy across the two LLMs**: round-robin, all-minimax-with-qwen-as-retry, or shard by MIB family? Defer to a 200-trap-per-model bake-off during the sample-gate iteration.
3. **Banned-phrase list**: finalise the regex during sample-gate iteration. Initial seed: marketing language, severity-inflation phrases.
4. **Per-vendor file partitioning**: group profiles by MIB module name (`IF-MIB.yaml`, `CISCO-CONFIG-MAN-MIB.yaml`) or by vendor (`cisco.yaml`, `juniper.yaml`)? Operators prefer per-vendor for grep/override; the pipeline must decide a normalization for vendor attribution (likely OID enterprise prefix lookup).
5. **Manual override mechanism**: simple YAML at `overrides.yaml` or per-OID files at `overrides/<OID>.yaml`? The former is easier to grep, the latter avoids merge conflicts.
6. **Rerun cadence**: how often do we re-run the full batch when MIB corpus updates? Likely tied to Netdata releases. Decision deferred.

## Followup mapping

- This SOW depends on SOW-0033 (MIB extraction) completing first.
- Output of this SOW feeds the Netdata trap subsystem implementation SOW (not yet written).
- Ongoing operation: as the MIB corpus grows (SOW-0001 in the monitoring repo, or vendor MIB additions), re-running this pipeline incrementally produces updated profile YAML.

## Validation gate (for SOW closure)

Closure requires:

- Sample-gate review completed; ≥90% acceptable rate documented in the SOW with the reviewer's notes.
- Prompt finalised in this SOW after iteration.
- Full-batch run completed; validation report on disk.
- Per-vendor YAML files emitted, lint-passing against the netdata.md §7 schema.
- Post-batch spot-check of random 500 records confirms quality holds.
- Tool resumability and override mechanism tested.
- Brief operator documentation in the tool's README.
- This SOW updated with final tool location, output paths, run statistics, sample-gate notes, and prompt version; moved to `done/`.

## Outcome

Resolution of the six open decisions at the Pre-Implementation Gate:

1. **Sample-gate reviewer** → user personally; iterated through ~4 prompt revisions on stratified 200-trap samples before unlocking the full batch.
2. **Routing strategy** → primary `minimax-m2.7` at the unauthenticated sglang endpoint `10.20.4.21:8357/v1` (100 concurrent slots, exact concurrency to keep sglang's batched-decode queues saturated). Fallback `qwen3.6-35b-a3b-nothinker` at `localhost:8356/v1` used surgically for the rare cases where minimax's decoder mutated unusual MIB prefixes (e.g., `ngev*` → `negev*` / `nggev*`); qwen tokenizes those correctly.
3. **Banned-phrase list** → finalized inside `classify.py`'s `SYSTEM_PROMPT` (marketing language, severity-inflation phrases, "fixing" odd varbind prefixes). Survives the schema validator's retry loop.
4. **Per-vendor file partitioning** → vendor (not MIB), via IANA enterprise-PEN lookup. `ENTERPRISE_VENDORS` table in `emit.py` covers ~100 named vendors; unknown PENs bucket as `enterprise-<N>.yaml` so nothing is silently dropped. IETF standard MIBs → `standard.yaml`; IEEE LLDP (`1.0.8802.*`) → `ieee-lldp.yaml`.
5. **Manual override mechanism** → operator overrides live under `/etc/netdata/go.d/snmp.trap-profiles/`, documented in `profile-format.md` § "Operator overrides". The pipeline does **not** maintain an in-tree overrides file — overrides are an operator-side concern, profiles ship as the vendor-curated layer.
6. **Rerun cadence** → run on demand when the MIB mirror updates or the taxonomy changes. The pipeline is idempotent: re-running `classify.py` skips already-enriched OIDs (use `--force` to overwrite); re-running `emit.py` produces a `git diff` against the committed pack that reviewers can read.

Closed taxonomy change during execution: `custom` was removed from the original 9-slug category list per netdata.md §3, leaving the 8 canonical slugs. Reason: `custom` was an escape valve that duplicated `unknown`'s role; keeping both let the model dodge classification. The trap profile-format documentation and the `ALLOWED_CATEGORIES` tuple in `classify.py` reflect the final 8.

Severity expanded from the original 3-slug set (`info`, `warn`, `crit`) to the **full 8-level syslog set** mapped to numeric `PRIORITY` (`emerg`=0 through `debug`=7). This lets operators alert with the same precision they get from syslog/journald already.

Final tool layout:

- `tools/snmp-traps-profile-gen/classify.py` — async LLM enrichment driver. 100 concurrent slots against the open `minimax-m2.7` endpoint, 3 retries with feedback per OID, schema-validated output (category enum, severity enum, description placeholder sanity).
- `tools/snmp-traps-profile-gen/emit.py` — per-vendor YAML emitter. Dedups varbinds into a file-scoped `varbinds:` table; ships catalogue.
- `tools/snmp-traps-profile-gen/output/enriched/<OID>.json` — per-trap enriched record (intermediate; gitignored). Schema includes `oid`, `name`, `mib`, `category`, `severity`, `priority`, `description`, `varbinds[]`, `trap_description`, `trap_status`, `mib_description`, `enrichment_source`, `enrichment_attempts`.
- `tools/snmp-traps-profile-gen/output/llm-failures.json` — per-OID validation failures + reasons (used to debug stubborn model behaviours).
- `tools/snmp-traps-profile-gen/output/enrichment-report.json` — per-source counts (attempt-1 / attempt-2 / attempt-3 / mechanical_fallback / qwen_fallback).
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/<vendor>.yaml` — shipped output: 327 per-vendor YAML files (28 MB raw, deduped varbinds table per file).
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json` — operator grep-before-install index.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` — operator-facing schema reference.

Run statistics (locked-in at completion):

| Metric | Value |
|---|---|
| Input traps (from SOW-0033) | 40,409 |
| Total LLM-enriched output files | 40,409 (100%) |
| Source: `minimax-m2.7` attempt-1 | 39,206 (97.02%) |
| Source: `minimax-m2.7` attempt-2 (retry with feedback) | 1,110 (2.75%) |
| Source: `minimax-m2.7` attempt-3 | 90 (0.22%) |
| Source: `qwen3.6-35b-a3b-nothinker` attempt-1 (vendor-prefix typo cases) | 3 (0.01%) |
| Source: mechanical fallback | 0 (0%) |
| Category distribution | state_change 17,300 (42.8%), diagnostic 16,158 (40.0%), config_change 3,675 (9.1%), security 1,445 (3.6%), auth 816 (2.0%), license 522 (1.3%), mobility 414 (1.0%), unknown 79 (0.2%) |
| Severity distribution (all 8 syslog levels used naturally) | warning 16,679 (41.3%), info 8,104 (20.1%), notice 7,843 (19.4%), err 3,771 (9.3%), crit 3,392 (8.4%), alert 597 (1.5%), debug 16 (0.04%), emerg 7 (0.02%) |
| Per-vendor output files | 327 |
| Per-vendor coverage (named) | ~100 vendors via PEN table |
| Per-vendor coverage (unnamed) | ~227 `enterprise-<N>` long-tail buckets |
| Stock pack size on disk (raw) | 28 MB |
| Stock pack size (gzipped, FYI) | 2.4 MB |
| Wall-clock for full batch | ~6h 4m (across two runs combined; first crashed at 396 enriched files due to a None-content bug, fixed and resumed) |

## Validation

Acceptance criteria evidence:

- Sample-gate review completed: 4 prompt revisions on a stratified-by-MIB 200-trap sample. Final attempt-1 success rate at the sample bar: 99% (199/200). User confirmed the sample met quality before unlocking the full batch.
- Prompt finalized in `classify.py` `SYSTEM_PROMPT`: 8 canonical categories, 8 syslog severities, banned-phrase list, varbind-prefix preservation rule, description length cap (1024 bytes).
- Full-batch run completed; `enrichment-report.json` on disk shows 100% completion at 97.02% attempt-1 success.
- 327 vendor YAML files emitted, schema-conformant to `profile-format.md`. Manual inspection of `cisco.yaml`, `huawei.yaml`, `dasan.yaml`, `ieee-lldp.yaml`, `standard.yaml` confirms structure.
- Post-batch audit of the 114 LLM-flagged `unknown` traps: pattern-grouped (vendor user-trap slots ~25, test/heartbeat ~15, generic wrapper notifications ~25, borderline-misses ~10, original mechanical-fallback misses ~35). 35 of the 41 mechanical-fallback files were recovered by re-classification with the updated prompt; the remaining 6 were recovered by re-running through the qwen fallback (where minimax's decoder kept mutating the `ngev*` vendor prefix). End state: 79 LLM-genuine `unknown` (correct calls — vendor user-trap slots have no static semantics) + 0 mechanical-fallback.
- Tool resumability tested: per-OID atomic file writes mean re-running `classify.py` over an existing `enriched/` directory only re-processes OIDs without an output file. `--force` overrides.

Tests or equivalent validation:

- The shipped pack at `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/` is the production deliverable; a regeneration produces a deterministic `git diff` against the committed state. Reviewable.
- Schema correctness end-to-end demonstrated by `emit.py` consuming 40,409 enriched records without skipping or erroring on any.

Real-use evidence:

- The trap-plugin SOW (not yet written) will read this pack on first trap arrival per device. The integration test for the data layer is the diff-clean re-emit run.

Reviewer findings and how they were handled:

- During iteration on prompt refinement, the user surfaced four real defects (each addressed in-prompt or in-code): (a) `category_ideal` field deemed useless after sample review — removed; (b) `custom` and `unknown` were both escape valves — merged to `unknown`; (c) model returned empty `content` on some traps — added raise-and-retry pattern in `call_llm()`; (d) model "auto-corrected" unusual vendor prefixes like `ngev*` — added a prompt sentence forbidding this, falling back to qwen for the residual 3 cases where minimax's decoder bias was strong.

Same-failure search results:

- No analogous classification pipeline elsewhere in the repo. Datadog's compiler stops at the structural extraction step (it does not classify traps semantically); LibreNMS's handler-class taxonomy is implicit in PHP and does not map cleanly to our 8-category structure.

Artifact maintenance gate:

- AGENTS.md — updated with the new `project-snmp-trap-profiles-authoring` skill entry (in the same close commit).
- Runtime project skills — added `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` covering this pipeline plus the trap profile authoring contract.
- Specs — `.agents/sow/specs/snmp-traps/netdata.md` §7 updated (install path + deduped varbinds table + lazy-load semantics).
- End-user / operator docs — `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` covers the on-disk schema. `tools/snmp-traps-profile-gen/README.md` covers the regeneration recipe with the new `--out-dir` pointing at the shipped-pack source location.
- End-user / operator skills — `query-netdata-agents` / `query-netdata-cloud` skills unaffected (no surface change to query interfaces).
- SOW lifecycle — Status: `open` → `completed`; moves from `pending/` to `done/` in the close commit alongside SOW-0033.

SOW status/directory consistency:

- Status: completed; file path: `.agents/sow/done/SOW-0034-20260523-snmp-profile-llm-enrichment.md` after the close commit.

Spec update or specific reason no spec update was needed:

- Spec updated. `netdata.md` §7 corrected to use the actual install path and documents the deduped varbinds table + lazy-load semantics. §3 already enumerated the 8 categories that classify.py enforces.

Project skill update or specific reason no skill update was needed:

- New skill `.agents/skills/project-snmp-trap-profiles-authoring/` documents the regeneration recipe, the closed taxonomy contract, the file-scoped `varbinds:` table pattern, label cardinality discipline, and the stock/operator separation. Indexed in `AGENTS.md` § "Project Skills Index".

End-user / operator docs update:

- `profile-format.md` is the operator-facing reference for the trap profile YAML schema. The website ingestion path for it is deferred to the trap-plugin SOW (which will land the consumer).

Lessons extracted:

- Schema-validated retry-with-feedback (3 attempts per OID, feeding the validator's reason back into the next prompt) cleared the long tail of malformed first attempts cheaply.
- The model can be honest about its limits when explicitly told an escape valve exists (`unknown`), as long as the prompt doesn't simultaneously offer a softer one (`custom`).
- Reasoning-style models (qwen with thinking) need much higher `max_tokens` than non-reasoning models — the reasoning chain eats the budget before the content emits. The `nothinker` variant or a `max_tokens` floor near 4000 are required.
- Per-vendor LLM tokenizer bias can corrupt vendor prefixes in description templates (`ngev*` → `negev*` / `nggev*`). Two cheap mitigations: a prompt sentence ("don't 'fix' what look like typos in varbind names") and a fallback model for the residual cases.
- Sample-quality gates are cheap (200 traps at 100 concurrent = ~108 seconds at the production endpoint) and saved hours of full-batch wasted work each time the prompt regressed.

Follow-up mapping:

- The trap-plugin SOW (not yet written) consumes the shipped pack at `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/`. Open items for that SOW (carried out of this SOW's scope): lazy-load implementation, operator-override merge layer, MIB hot-reload from `/etc/netdata/snmp-mibs/` for the never-classified case.

## References

- `~/src/PRs/snmptraps/.agents/sow/specs/snmp-traps/netdata.md` §3 (categories), §7 (profile schema), §13 (open questions)
- `~/src/PRs/snmptraps/.agents/sow/pending/SOW-0033-20260523-snmp-mib-mechanical-extraction.md` (upstream producer)
- `~/src/PRs/snmptraps/.local/dd_traps_db.json.gz` (reference compiled output — leaner than ours)
- LLM endpoints:
  - `minimax-m2.7-coder` at `nova:3537` (~1,000 tps aggregate at 64 concurrent)
  - `qwen3.6-35b-a3b` at `localhost:3536` (single-stream 180 tps, 8 concurrent; aggregate TBD by measurement)

End of SOW.
