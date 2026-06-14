---
name: project-snmp-trap-profiles-authoring
description: Use when editing Netdata SNMP trap profile YAMLs, trap profile metric rules, the trap profile-format documentation, the snmp-trap-profile-gen Go helper, or running a regeneration of the OOB trap profile pack. Enforces the closed 8-category / 8-severity taxonomy, the file-scoped varbinds-table pattern, metric cardinality discipline, and stock/operator separation.
---

# SNMP Trap Profile Authoring

Use this skill before editing files under:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/`
- `src/go/cmd/snmptrapprofilegen/` (shipped helper source)
- `.agents/sow/specs/snmp-traps/netdata.md` (when the change touches profile schema or trap subsystem decisions that the profiles encode)

The authoritative schema reference is
`src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`. The
authoritative subsystem design is
`.agents/sow/specs/snmp-traps/netdata.md`. This skill is the working checklist
that keeps repository edits aligned with both.

## SNMP Trap SOW Spec Organization

The SNMP trap SOW spec directory intentionally separates Netdata product
contracts from research evidence.

- Netdata-owned specs and decisions live directly under
  `.agents/sow/specs/snmp-traps/`:
  - `netdata.md`
  - `trap-metrics-profiles.md`
  - `netdata-snmp-hub-architecture.md`
  - `decisions/`
- Research evidence lives under `.agents/sow/specs/snmp-traps/research/`:
  - `domain/` for general SNMP trap observability research;
  - `playbooks/` for operational playbooks and skill-distillation source
    material;
  - `netdata-existing/` for inventories of existing Netdata subsystems used as
    design inputs;
  - `external-systems/` for per-product studies of other trap implementations;
  - `comparison/` for cross-system matrices, stress tests, and synthesis.

Do not put new research files beside the Netdata specs. Research can inform
specs, but it is not itself a Netdata product contract. When a research finding
becomes a product rule, copy the accepted rule into a top-level spec or
`decisions/` entry and cite the research path as evidence.

## Trap OID `.0.` tolerance

Profile authors should use the canonical trap OID form produced by the source
MIB/tooling. The receiver lookup is exact-match-first and then tolerates the
SMIv1 / SMIv2 trap-OID ambiguity by adding or removing a single `.0.` segment
immediately before the final OID arc on primary miss. This tolerance is
trap-OID-only: do not normalize or alternate-match varbind OIDs.

## Required checks before changing a profile

1. **Trap `name:` must be MIB-qualified.** Every trap entry's `name:`
   field uses the canonical SMI form `<MIB-MODULE>::<symbol>` (e.g.
   `IF-MIB::linkDown`, `CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged`).
   Vendors reuse bare symbolic names across product-line MIB modules; the
   bare symbol is NOT globally unique. The qualified form matches what
   `snmptranslate` / `snmptrapd` / MIB browsers produce and is what the
   plugin writes to the `TRAP_NAME` journal field. Rule: if the OID
   changes, the `name:` slug MUST change.

2. **Resolve every varbind reference.** A name in a trap entry's `varbinds:`
   list MUST exist in the file-scoped `varbinds:` table or be an inline dict
   on the trap entry. Dangling name references are a bug — they render as empty
   values in restricted templates and produce misleading journal messages.

3. **Identify the source MIB object for every varbind.** Check the object's
   `MAX-ACCESS`. `not-accessible` index objects must still be declared in the
   table (they ship inside `TRAP_JSON` and, when non-sensitive/non-redundant,
   indexed `TRAP_VAR_*` fields), but never as a `description:` template
   variable on its own — varbinds an SNMP entity will not send in a trap PDU
   never resolve at runtime.

4. **File-scoped `varbinds:` table entries require both `oid` and `type`.**
   Varbind records with `resolved: false` (the MIB-extractor couldn't
   resolve the OBJECT-TYPE through the IMPORTS chain) MUST be dropped
   from both the table and the per-trap reference list. An empty `{}`
   entry under `varbinds:` violates the schema and is rejected by the
   plugin at profile load.

5. **Categories: closed set of 8.** `category` must be one of
   `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`,
   `diagnostic`, `unknown`. Do not introduce new categories. Cross-cutting
   concerns (compliance scope, tenant, datacenter, change window…) belong in
   `labels:`, not as new category slugs. Operator-authored OIDs default to
   `unknown` and the operator overrides category/severity/labels in plugin
   config — there is no separate "custom" category slug.

6. **Severities: closed set of 8, mapped to syslog PRIORITY.** `severity`
   must be one of `emerg`, `alert`, `crit`, `err`, `warning`, `notice`,
   `info`, `debug` (full names — not `warn`). The plugin maps these to
   `PRIORITY=0..7` on the journal entry. `emerg` is reserved for true vendor
   catastrophe; default to `warning`/`notice`/`info` for routine events.
   `debug` is rare and only for traps the MIB itself marks as debug-level.

7. **Cardinality discipline on `labels:`.** Label templates must reference
   bounded-cardinality varbinds only. Reject (do not commit) labels that
   reference MAC addresses, source IPs, usernames, packet contents, RAID
   slot IDs, or any per-event identifier. High-cardinality content belongs
   in `description:` (rendered into MESSAGE), indexed `TRAP_VAR_*` journal
   fields, and `TRAP_JSON`, not in metric-propagating labels.

8. **Label keys use a structurally-safe namespace.** All labels (from
   profile `labels:` AND operator config `labels:`) emit as
   `TRAP_TAG_<KEY_UPPERCASE>` journal fields. The dedicated `TRAP_TAG_*`
   namespace removes any risk of collision with the plugin-controlled
   `TRAP_*` field set (`TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, etc. — see
   spec §11). The only remaining validation is the lowercase-key syntax
   rule (`[a-z][a-z0-9_]*`). Pick label keys that read clearly.

9. **Stock vs operator separation.** Files under
   `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/` are stock
   vendor-curated profiles, regenerated by `src/go/cmd/snmptrapprofilegen/`
   and shipped from Netdata as
   `/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen`. Do not hand-edit
   them for site-specific concerns; site overrides belong under
   `/etc/netdata/go.d/snmp.trap-profiles/` and are documented in
   `profile-format.md` § "Operator overrides".

10. **Profile metrics use the validated `metrics:` / `charts:` schema.** Trap
    profiles may define optional trap-to-metric rules only through the schema in
    `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.
    Listener jobs decide enablement with `profile_metrics`. Do not add ad hoc
    metric fields, unbounded labels, or site-specific metric choices to stock
    profiles. Site-specific metric rules belong in operator profile files under
    `/etc/netdata/go.d/snmp.trap-profiles/`.

    Required profile-metric authoring checks:
    - Rule types are only `counter`, `sample`, and `state`; use canonical
      fields for stock/generated profiles and keep compact aliases for
      operator-authored examples.
    - `where:` predicates are ANDed and may use `equals`, `in`, `exists`,
      `absent`, `greater_than`, `less_than`, `range`, and `not`; never combine
      `not` with `exists` or `absent`, and never define a predicate without a
      condition operator.
    - `sample` rules may read only numeric varbind types documented in
      `profile-format.md`; `TimeTicks` is converted to seconds before `scale`.
      `Counter32`, `Counter64`, and `TimeTicks` are valid for sample rules,
      not resource identity keys.
    - `state` rules use either separate `problem_trap` / `clear_trap` OIDs or
      same-OID `state.set_when` / `state.clear_when` predicates. `state.ttl`
      must be a valid Go duration string, and `state.ttl_behavior` currently
      supports only `clear_and_expire`.
    - `identity.resource.key_from_varbind` MUST reference an integer-like
      bounded varbind (`INTEGER`, `Integer32`, `Unsigned32`, or `Gauge32`).
      Never use strings, MACs, usernames, addresses, payloads, or event IDs as
      metric resource keys.
    - All rules sharing one chart MUST have the same label shape. Do not mix
      resource and non-resource rules in one chart, and do not mix multiple
      resource classes in one chart.
    - Charts that can create source or resource instances MUST have bounded
      lifecycle settings. Expired series are removed; if the same identity
      appears again, the next committed trap creates a fresh series.
    - `missing: unknown_dimension` is allowed only with resource identity.
      `missing: drop` increments rule-miss diagnostics; `missing: error`
      increments extraction-failure diagnostics.
    - Metric names, chart IDs, and chart contexts MUST NOT collide with
      built-in receiver charts, the built-in `profile_metric_diagnostics`
      chart, or any other loaded profile rule/chart.
      Reserved metric prefixes include `snmp_trap_events_`,
      `snmp_trap_severity_`, `snmp_trap_errors_`, `snmp_trap_dedup_`,
      `snmp_trap_pipeline_`, `snmp_trap_source_`, `snmp_trap_sources_`,
      `snmp_trap_metric_`, and `snmp_trap_profile_metrics_`.
      Built-in source receiver metrics are automatic; profile rules should
      describe vendor or site semantics, not duplicate receiver pipeline health.
    - `auto_safe: true` means the rule is safe for broad trap hubs: bounded
      labels, bounded resource identity, no sensitive values, and no surprising
      high cardinality. Stock rules need review evidence before enabling it.
    - Profile metrics update only after the trap is successfully committed to
      the configured journal and/or OTLP backend. Dedup-suppressed and
      write-failed traps do not update profile metrics.

11. **No `journal_fields:` list in profiles.** The plugin derives indexed
    `TRAP_VAR_*` journal fields automatically from received non-sensitive,
    non-redundant event varbinds, and keeps the structured audit copy in
    `TRAP_JSON`. There is no profile knob to hand-author per-OID journal
    field names.

12. **`display_hint` is reserved, not yet emitted.** `profile-format.md`
    documents `display_hint` (e.g. `1x:` for MAC, `1d.1d.1d.1d` for IPv4)
    as a future varbind field. The extractor keeps display hints in
    intermediate JSONL when `gomib` exposes them, but the stock profile
    emitter does not write `display_hint` today. Do not add `display_hint`
    keys by hand to stock profiles — they would be silently overwritten on
    regeneration. When the plugin's renderer needs display-hint formatting,
    the emitter and loader will be updated in the same regeneration cycle.

## Required checks when editing the generator (`src/go/cmd/snmptrapprofilegen/`)

1. **The shipped helper must remain a single Go binary.** It is built by CMake
   as `snmp-trap-profile-gen`, installed under
   `usr/libexec/netdata/plugins.d/`, and packaged in the `plugin-go`
   component. Do not add Python, CGO, SQLite, or runtime MIB compiler
   dependencies to the shipped operator path.

2. **Extraction must remain incremental.** The full corpus is too large to
   load as one global MIB universe. Keep the batch-based gomib loading path,
   deterministic source priority, duplicate-module `source-conflicts.json`,
   OID-level `conflicts.json`, and bounded memory validation. If source
   discovery changes, rerun at least a representative multi-vendor corpus
   before touching the stock pack.

3. **Classification cache stays reviewable text.** The cache is deterministic
   JSONL keyed by the classifier input hash. Do not switch to SQLite or another
   opaque cache for committed or CI-reviewed state.

4. **LLM output validation is mandatory.** Model responses must validate
   against the JSON Schema and the semantic validators: closed category,
   closed severity, exact template helper allowlist, and uniform description
   style ending with ` on {{hostname}}.`. Retry invalid responses up to five total
   attempts before mechanical fallback, or hard failure under `--require-llm`.

5. **YAML emission** is the producer of files in `default/`. It must:
   - MIB-qualify every trap `name:` as `<MIB-MODULE>::<symbol>` so the
     slug is globally unique;
   - dedup varbinds into the file-scoped table (do not regress to per-trap
     inline varbinds — see netdata.md §7 and `profile-format.md`);
   - drop varbind records with no resolvable `oid` (extractor's
     `resolved: false` cases) from both the table and the per-trap
     reference list — never emit empty `{}` table entries;
   - drop internal pipeline metadata (`enrichment_source`,
     `enrichment_attempts`) from the YAML output;
   - keep `catalogue.json` in sync (operator grep-before-install tool);
   - emit deterministic output (sorted varbind names, traps sorted by
     OID then name) so regenerations produce reviewable diffs.

6. **PEN registry handling** must use the bundled snapshot by default. CMake
   installs
   `src/go/plugin/go.d/config/go.d/snmp.profiles/metadata/iana-enterprise-numbers.txt`
   under `usr/lib/netdata/conf.d/go.d/snmp.profiles/metadata/`. `--refresh-pen`
   may fetch the current IANA registry when explicitly requested.

7. **Regenerating the stock pack** uses the Go helper:

   ```bash
   cd src/go
   go run ./cmd/snmptrapprofilegen generate \
     --source-dir /path/to/mibs \
     --all \
     --classify \
     --require-llm \
     --concurrency 20 \
     --out-dir /tmp/snmp-trap-profile-gen-output \
     --profiles-out-dir ../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default \
     --catalogue ../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json
   ```

   The installed operator equivalent is:

   ```bash
   /usr/libexec/netdata/plugins.d/snmp-trap-profile-gen generate \
     --source-dir ./mibs \
     --all \
     --out-dir ./snmp-trap-profile-gen-output
   ```

   The shipped pack lives under
   `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/`. Operator
   output should be copied from `snmp-trap-profile-gen-output/profiles/` into
   `/etc/netdata/go.d/snmp.trap-profiles/`.

## Required checks when changing categories or severities (taxonomy work)

These are closed sets enforced in three places that must stay in sync:

1. `src/go/cmd/snmptrapprofilegen/main.go` — `validCategories`,
   `validSeverities`, `severityPriority`, JSON Schema, and classifier prompt text.
2. `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`
   — the operator-facing category and severity tables.
3. `.agents/sow/specs/snmp-traps/netdata.md` — §3 (category taxonomy) and
   §11 (PRIORITY mapping).

A taxonomy change without all three updates is incomplete and will be
rejected at review. Any taxonomy change also requires a re-run of
the Go helper's classification path against the full corpus (the existing
classifications were done under the prior taxonomy and are now stale).

## File size discipline

Stock profile YAMLs stay raw in the repository so changes are reviewable in
`git diff`. Installed/package stock vendor profiles MUST be compressed as
`.yaml.zst`; the runtime loader supports raw `.yaml`, compressed `.yaml.zst`,
and draft-era `.yaml.gz` compatibility. Operator/user profiles under
`/etc/netdata/go.d/snmp.trap-profiles/` SHOULD stay uncompressed `.yaml` for
editability. If a single vendor file grows past ~10 MB in the repository,
revisit description verbosity rather than hiding unreviewable generated bloat
behind compression.
