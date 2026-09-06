---
name: collectors-snmp-trap-profiles
description: Use when editing Netdata SNMP trap profile YAMLs under snmp.trap-profiles/, trap profile metric rules (metrics:/charts:), the trap profile-format documentation, the trap profile generator (src/go/cmd/snmptrapprofilegen, shipped as snmp-trap-profile-gen), regenerating or compressing the stock trap profile pack and its catalogue.json, or changing the trap category or severity sets. Enforces the closed 8-category / 8-severity taxonomy, MIB-qualified trap names, the file-scoped varbinds table, label cardinality discipline, stock/operator separation, and the generator's validation and determinism contracts.
---

# SNMP Trap Profile Authoring

Use this skill before editing files under:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/` (stock pack `default/`, `catalogue.json`, `profile-format.md`)
- `src/go/cmd/snmptrapprofilegen/` (the generator; installed as `snmp-trap-profile-gen`)
- `src/go/plugin/go.d/collector/snmp_traps/internal/catalog/` when the change is about what a profile may contain

## Authoritative sources

This skill holds only what the documents below lack. Point at them; do not restate them here.

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`: the profile format (file layout, the varbinds
  table, trap entries, `.0.` tolerance, description templates, `metrics:`/`charts:` rules and their validation list,
  categories, severities, cardinality, operator overrides, generated stock profiles). It ships with the pack under
  `usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/`, so it is operator-facing and must stay true to the loader.
- `src/go/plugin/go.d/collector/snmp_traps/ARCHITECTURE.md`: how the collector works (life of a trap, journal field
  contract, package map, "Where To Change Things", validation commands). Collector code changes start there, not here.
- `docs/npm/snmp-traps/` (published operator docs): `trap-profiles.md` (override versus new profile), `configuration.md`
  (every job option), `field-reference.md` (every `TRAP_*` field), `metrics.md` (built-in charts and dimensions).
- Sibling skills: `collectors-snmp-profiles` for polling profiles; `query-snmp-traps` for reading trap logs.
- Design history (rejected alternatives, the comparison with other trap systems, the phase plans) is not maintained
  in the tree; the pre-implementation design records that used to live in this directory are in git history.

## Required checks before changing a profile

1. **Trap `name:` is MIB-qualified** `<MIB-MODULE>::<symbol>` (`IF-MIB::linkDown`). Bare symbols are reused across
   vendor MIB modules and are not unique; the qualified form is what `snmptranslate` produces and what lands in
   `TRAP_NAME`. If the OID changes, the name changes. Different OIDs must have different names (`profile-format.md`,
   "Trap entries").

2. **Check `MAX-ACCESS` of the source MIB object for every varbind.** A `not-accessible` index object still belongs in
   the varbinds table, so that the rare device that does include it gets it named in `TRAP_JSON` and, when
   non-sensitive and non-redundant, as an indexed `TRAP_VAR_*` field. Never use it as a `description:` template
   variable on its own: an SNMP entity normally does not send it in the trap PDU, so the placeholder renders empty.
   No in-tree artifact can check this; it needs the MIB.

3. **Every varbind reference resolves.** A name in a trap's `varbinds:` list must exist in the file-scoped `varbinds:`
   table or be an inline `{name, oid, type}` dict on that trap. A dangling name renders empty in the description and
   produces a misleading journal message. Table entries need both `oid` and `type`; an empty `{}` entry fails profile
   load. The generator drops extractor records with an empty name, OID, or type from the table and from every trap's
   reference list (`buildProfile`, pinned by `TestBuildProfileDropsUnresolvedVarbinds`); do the same by hand.

4. **Keep the metadata the loader validates.** Trap `status:` takes only `current`, `deprecated`, `mandatory`,
   `obsolete`, `optional` (`validTrapStatuses` in the generator, `validStatuses` in `internal/catalog/profile.go`).
   Varbind `enum:` is what renders `{{value}}` symbolically and what `equals`/`in` predicates match; `constraints:`
   documents the range. File-scope `vendor:`, `mib_count:`, `trap_count:` are emitted on every stock file. Do not
   strip any of them when editing.

5. **Categories: closed set of 8.** `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`,
   `diagnostic`, `unknown`. Cross-cutting concerns (compliance scope, tenant, datacenter, change window) go in
   `labels:`, not new slugs. Every profile entry declares `category` and `severity` (the loader rejects a missing
   or unknown value); only a trap with no profile match is logged with `unknown`/`notice` at runtime, and operators
   reclassify it through the job's `overrides:`. There is no "custom" category. Changing the set is taxonomy work.

6. **Severities: closed set of 8 full syslog names** mapped to `PRIORITY=0..7`: `emerg`, `alert`, `crit`, `err`,
   `warning`, `notice`, `info`, `debug` (never `warn`). `emerg` is for true vendor catastrophe; routine events are
   `warning`/`notice`/`info`; `debug` only when the MIB itself marks the notification debug-level.

7. **Label cardinality.** Label templates reference bounded varbinds only. Reject labels built from MAC addresses,
   source IPs, usernames, packet contents, RAID slot IDs, or any per-event identifier; the loader rejects unbounded
   label templates at profile load. High-cardinality content belongs in `description:` (rendered into `MESSAGE`),
   the indexed `TRAP_VAR_*` fields, and `TRAP_JSON`, none of which propagate to metrics.

8. **Label keys** match `[a-z][a-z0-9_]*` and emit as `TRAP_TAG_<KEY_UPPERCASE>`, whether they come from a profile's
   `labels:` or from a listener job's per-OID `overrides[].labels` (there is no job-wide `labels:` option). The
   dedicated prefix makes collision with the plugin-owned `TRAP_*` fields impossible, so the key syntax is the only
   check. See `docs/npm/snmp-traps/field-reference.md` for the shipped `TRAP_*` set (it is not closed at
   the profile level; new fields arrive with collector releases).

9. **Trap OID form and `.0.` tolerance.** Use the OID form the source MIB tooling produces. Lookup is exact-match
   first, then retries the alternate spelling with one `.0.` segment added or removed before the final arc
   (`model.AlternateTrapOID`, called from `Epoch.lookupLoaded`). A catalogue that defines both spellings of one trap
   fails to load ("alternate form already defined"). The tolerance is trap-OID only: never normalize or
   alternate-match varbind OIDs.

10. **Stock versus operator.** Files under `default/` are generated and overwritten on regeneration (their header
    comment says so); never hand-edit them for site concerns. Operator profiles live in the user config directory
    `go.d/snmp.trap-profiles/` (`catalog_paths.go`) in one of three forms: a complete same-identity replacement of a
    stock file, an independent different-identity addition, or a metric-only profile whose rules reference stock
    traps. Partial inheritance does not exist; an `extends:` key is rejected as unknown at profile load. Per-OID
    category, severity, and label overrides belong in the listener job's `overrides:`
    (`docs/npm/snmp-traps/trap-profiles.md` has the decision table).

11. **No hand-authored journal fields.** There is no `journal_fields:` key: `TRAP_VAR_*` fields are derived from the
    received non-sensitive, non-redundant varbinds and `TRAP_JSON` keeps the audit copy. `display_hint` is documented
    as a future varbind field but is not part of the loaded schema: `varbinds:` entries accept unknown keys
    (`profileYAMLSpec` in `internal/catalog/load.go`), so a hand-added `display_hint` is silently ignored and then
    overwritten on regeneration. When the renderer starts consuming DISPLAY-HINT metadata, the extractor,
    `profile-format.md`, and `model.TrapEntry` change in the same cycle.

12. **Profile metrics only through the validated schema.** `profile-format.md`, "Optional `metrics:` rules and
    `charts:`", owns the syntax, defaults, numeric source types, and the load-time rejection list; a listener job
    enables rules explicitly with `profile_metrics.include` (rule names, never trap names or filenames). Checks the
    loader enforces (`internal/catalog/metric_validate.go`) that authors most often get wrong:
    - Chart IDs and contexts must not reuse the six built-in charts `events`, `severity`, `errors`,
      `dedup_suppressed`, `pipeline`, `profile_metric_diagnostics` or their `snmp.trap.*` contexts; a chart context
      defaults to `snmp.trap.<chart id>` and must start with `snmp.trap.`; metric names must not start with a
      reserved prefix (`builtInProfileMetricChartIDs`, `reservedProfileMetricPrefixes`).
    - Every `where:` predicate selects exactly one string-valued source: `varbind`, or `field` from the closed set
      `category`, `severity`, `trap_name`, `trap_oid`; predicates AND; use `absent`, not `not` plus `exists`.
    - `identity.resource.key_from_varbind` must be an integer-like bounded varbind (`INTEGER`, `Integer32`,
      `Unsigned32`, `Gauge32`); `Counter32`, `Counter64`, `TimeTicks` are `sample` values, not identity keys.
    - `missing:` is one of `drop`, `error`, `zero`, `unknown_dimension` (`zero` is invalid for `counter` and
      `state`; `unknown_dimension` needs resource identity).
    Authoring rules the loader does NOT check, so review them by hand:
    - Every rule sharing a chart has the same label shape: do not mix resource and non-resource rules, or several
      resource classes, in one chart.
    - Declare `lifecycle` explicitly on every chart that creates per-source or per-resource instances instead of
      relying on the loader's defaults; expired instances are removed and a returning identity starts a fresh series.
    - On a chart that carries a `state` rule, `lifecycle.expire_after_cycles` must outlast `state.ttl`: the TTL
      publishes the clear value once and only then expires the series, so a chart that expires first leaves the
      problem state as the last value. The loader validates the two fields independently and never compares them.
      A rule's `where:` is evaluated before its `state.set_when`/`clear_when` predicates.
    - Never use the community varbind or another sensitive varbind (`model.IsSensitiveVarbind`) as a predicate or
      sample source; redaction happens downstream, not in rule validation.
    - Profile rules describe vendor or site semantics, never receiver health. Profile metrics update only after the
      trap is committed to the configured backend; dedup-suppressed and write-failed traps do not count.
    - No stock profile ships `metrics:` today (0 of 803). Stock rules would be a curation layer that the generator
      must preserve through a tested read-modify-write path from a reviewable, committed source recording rule name,
      trap, varbinds, type, chart, and cardinality evidence, and must validate against the `varbinds:` and `traps:`
      it just emitted before writing the file. That path does not exist; build it before adding stock rules, and
      check pack size and lazy-load memory when you do.

## Required checks when editing the generator (`src/go/cmd/snmptrapprofilegen/`)

1. **One Go binary, no runtime dependencies.** CMake target `snmp_trap_profile_gen` builds `snmp-trap-profile-gen`
   and installs it under `usr/libexec/netdata/plugins.d/` in the `plugin-go` component; the pack build runs it with
   `CGO_ENABLED=0`. Do not add Python, CGO, SQLite, or a runtime MIB compiler to the shipped path.

2. **Subcommands** are `extract`, `classify`, `emit`, `generate`, and `compress-zstd` (`usage`). `generate` is
   extract plus optional classify plus emit; the three stages exist separately for reruns on saved artifacts.

3. **Extraction stays incremental and memory-bounded.** The corpus is too large for one MIB universe: keep
   batch-based gomib loading (`--batch-size`, default 32), deterministic source priority, and the review artifacts
   under `--out-dir`:
   `traps.jsonl`, `extraction-report.json`, `conflicts.json` (duplicate trap OIDs), `dot0-conflicts.json` (both
   `.0.` spellings present), `source-conflicts.json` (one module name in several files). `--baseline-profiles-dir`
   adds a stock-overlap report. If source discovery changes, rerun a representative multi-vendor corpus before
   touching the stock pack.

4. **Classification cache stays reviewable JSONL.** One `Classification` record per trap keyed by `hashTrap`, with
   `schema_version` and `prompt_version`; a record whose versions differ from `defaultSchemaVer` or
   `defaultPromptVer` is rejected, so bump `defaultPromptVer` whenever the prompt or the taxonomy changes or the
   cache silently replays stale answers. `--force-llm` ignores the cache. The cache path is derived from the default
   out-dir unless `--cache` is passed explicitly, even when `--out-dir` differs. Never switch to SQLite or another
   opaque store.

5. **LLM output validation is mandatory.** Every response passes the classifier response JSON Schema
   (`classifierResponseSchemaJSON`, checked by `validateClassifierResponseSchema`), the template check
   (`validateDescriptionTemplate`: only the helpers in `classifierTemplateFuncMap`, references checked against the
   trap record), and the style check (`validateDescriptionStyle`: ends with ` on {{hostname}}.`, `{{hostname}}`
   exactly once). Off-taxonomy categories are remapped first by `repairInvalidCategory`. Up to
   `maxLLMAttempts` (5) tries, then `mechanicalClassification`, or a hard failure under `--require-llm`. MIB text
   reaches the model wrapped as untrusted input (`sanitizePromptText`); keep that wrapping.

6. **Emission is deterministic and produces the file-scoped table** (`buildProfile`, `writeProfileYAML`):
   - trap `name:` is MIB-qualified; varbind table names are bare symbols;
   - one table entry per varbind name; a name that recurs with a different OID or type falls back to an inline
     `{name, oid, type}` dict on that trap (intended; do not "fix" it, and do not regress to inline everywhere);
   - records with an empty name, OID, or type are dropped from table and references; no `{}` entries;
   - traps sort by OID (`compareOIDString`) and the table keys and rule names are sorted, so regenerations diff
     cleanly;
   - the three-line header comment is part of the file and of its digest;
   - the vendor slug (`vendorForOID`: `standard`, `ieee-lldp`, `ieee-802`, the PEN slug or `enterprise-<pen>`, else
     `oid-<first arc>` or `unknown`) is the output filename and therefore the identity an operator override replaces.

7. **`catalogue.json` stays in sync.** Each entry (`profileCatalogueEntry`) records `file`, `mib_count`, `mibs`,
   `sample_traps`, `trap_count`, `trap_oids`, `varbind_count`, `sha256`, and `metric_rule_names` when the profile
   has rules (omitted otherwise, which is every stock file today; emitting stock rules also needs the curation path
   described in profile check 12). `sha256` is 64 lowercase hex over the exact bytes
   written, comments and final newline included (format pinned by `TestStockProfileCatalogueRequiresValidSHA256`);
   lazy hydration verifies it (`TestStockProfileEpochBindsLazyHydrationToManifestContent`). Catalog tests load all
   shipped profiles and require the manifest and the files to agree in both directions
   (`TestStockProfileCatalogueMatchesDefaultFiles`, `TestStockCatalogueReconcilesPhysicalInventory`,
   `TestStockProfileDefaultFilesParse`): regenerating profiles without the catalogue fails tests.

8. **PEN registry.** The default is the bundled snapshot (`defaultPENFilePath`; installed at
   `usr/lib/netdata/conf.d/go.d/snmp.profiles/metadata/iana-enterprise-numbers.txt`). With `--refresh-pen`, or when
   the file is missing or empty, `loadPENs` fetches `--pen-url` and a failed fetch aborts the run
   (`TestLoadPENsRefreshFailureIsFatal`). An air-gapped run needs the snapshot present.

## Regenerating the stock pack

```bash
cd src/go
go run ./cmd/snmptrapprofilegen generate \
  --source-dir /path/to/mibs \
  --all \
  --classify \
  --require-llm \
  --concurrency 20 \
  --out-dir /tmp/snmp-trap-profile-gen-output \
  --cache /tmp/snmp-trap-profile-gen-output/classification-cache.jsonl \
  --profiles-out-dir ./plugin/go.d/config/go.d/snmp.trap-profiles/default \
  --catalogue ./plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json
```

- Classification talks to an OpenAI-compatible endpoint (`--base-url`, default a local server; `--model`); without
  `--classify` every trap keeps the extractor defaults: category `unknown`, severity `notice`, description
  `<qualified name> on {{hostname}}.`.
- Review the diff: ordering is deterministic, so unexpected churn means an extractor or prompt change.
- Counts quoted in docs (`ARCHITECTURE.md`, `docs/npm/snmp-traps/trap-profiles.md`) describe the pack; recompute
  them from `catalogue.json` after a regeneration rather than carrying old numbers.

The installed operator form converts site MIBs offline; the output under `snmp-trap-profile-gen-output/profiles/` is
copied into the operator profile directory (`profile-format.md`, "Generated stock profiles").

## Changing categories or severities (taxonomy work)

The sets are duplicated in code, alerts, and docs. Only the telemetry series are test-pinned
(`TestJobCollectsExactRetainedMetricSet` in `internal/telemetry/job_test.go` enumerates the per-category and
per-severity counters); nothing pins the generator's or the loader's sets. A change must touch every site:

1. Generator `main.go`: `validCategories`, `validSeverities`, `severityPriority`, `repairInvalidCategory`,
   `mechanicalClassification` (the no-LLM fallback hard-codes both sets), the classifier response JSON Schema
   (`classifierResponseSchemaJSON`), and the prompt text; bump `defaultPromptVer`.
2. Collector `internal/catalog/profile.go`: `validCategories`, `validSeverities`, `categoryList`, `severityList`
   (the loader rejects profiles the generator would otherwise emit).
3. Per-slug surfaces in the collector: the `overrides[].category` and `overrides[].severity` enums in
   `config_schema.json`, the `internal/telemetry` counters and their test, the `events` and `severity` chart
   dimensions in `charts.yaml` and `metadata.yaml`, the OTLP severity mapping (`otlpSeverity` in
   `internal/output/otlp`), and the `PRIORITY` mapping in `internal/output/journal` (`grep -rn` the slug across
   `snmp_traps/`).
4. Health: `src/health/health.d/snmp_traps.conf` has severity-rate alert templates for `emerg`, `alert`, `crit`,
   `err`, and `warning` (`notice`, `info`, `debug` deliberately do not alert), mirrored in the `alerts:` list of
   `metadata.yaml`; a renamed or removed slug silently breaks them. `grep -rn` the slug across
   `src/health/health.d/` too.
5. Docs: `profile-format.md` category and severity tables, and every operator page that enumerates the sets
   (`grep -rln` the slug across `docs/npm/snmp-traps/`: today `README.md`, `trap-profiles.md`, `configuration.md`,
   `field-reference.md`, `metrics.md`, `alerts.md`, `usage-and-output.md`, `journal-and-querying.md`,
   `investigation-playbooks.md`).
6. Re-run classification for the full corpus: existing cache records were produced under the old taxonomy.

Add tests that pin the generator's and the loader's sets when you touch them.

## File size and compression

- Stock profile YAMLs stay raw in the repository so `git diff` reviews them.
- The pack build (CMake) runs the generator's `compress-zstd --rm` subcommand via `go run` on a copy of the pack and
  installs `*.yaml.zst` plus `catalogue.json.zst`; the installed pack must stay compressed. The loader accepts
  profiles as `.yaml.zst`, `.yml.zst`, `.yaml`, or `.yml` (`internal/catalog/load.go`) and the manifest as
  `catalogue.json` or `catalogue.json.zst`, never gzip (`internal/catalog/stock.go`).
- Operator profiles stay uncompressed `.yaml` for editability.
- If one vendor file passes about 10 MB in the repository, cut description verbosity rather than hide generated bloat
  behind compression.

## Validation

```bash
cd src/go
go test -count=1 ./cmd/snmptrapprofilegen/
go test -count=1 ./plugin/go.d/collector/snmp_traps/internal/catalog/...
```

The catalog tests load all shipped profiles and verify the manifest. For collector code changes run the full
`snmp_traps` suite with `-race` as `ARCHITECTURE.md`, "Validation", describes.
