# SOW-20260609-snmp-traps-full-mib-corpus - SNMP Traps Full Mirrored MIB Corpus

## Status

Status: in-progress

Sub-state: full-corpus normalized extraction, LLM classification cache, enriched
JSONL output, restricted-template hardening, lazy stock-profile loading,
installed gzip support, and generated-pack replacement are implemented and
committed. Remaining work is external review and any fixes it finds.

## Requirements

### Purpose

Ensure the stock SNMP trap profile pack is generated from a defensible,
reviewable, and repeatable inventory of all available mirrored SMI MIB modules,
so Netdata ships broad out-of-the-box trap decoding coverage for network
operators without relying on an undocumented curated subset.

The purpose is fit for production network-team use:

- maximize trap profile coverage from the local mirrored corpus;
- preserve deterministic extraction and conflict handling;
- make omissions visible and reviewable;
- avoid sending incomplete or unstable input back through enrichment;
- keep generated output reproducible enough for code review.

### User Request

Create an SOW for all MIBs after evidence showed the current extraction roots do
not cover the full mirrored MIB corpus.

Relevant user direction:

- "The list of 50k traps does not have a lot, the most important is the standard
  snmpmibs list everyone is using."
- "Let's do this right this time."
- "We have the mirrored repos. Find all the mibs in it please and make sure the
  defacto mibs collected by the community as in them."
- "yes, create an SOW for all MIBs."

Regresses:

- SOW-0033 mechanical extraction corpus completeness gap.
- SOW-0034 enrichment quality because new extraction output must be sent through
  the classifier again.
- SOW-0042 Go extraction pipeline must become the source of truth for the new
  full-corpus workflow.

### Assistant Understanding

Facts:

- The legacy Python extractor has a hard-coded, priority-ordered
  `DEFAULT_SOURCE_DIRS` list in `tools/snmp-traps-profile-gen/extract.py`.
- The active Go helper accepts repeated `--source-dir` flags and fails if no
  source directory is supplied.
- The previous corpus expansion fixed one missed source, `pysnmp/mibs/src`, and
  raised the old extracted corpus to 50,198 traps.
- A fresh mirror scan found substantially more SMI modules than the current
  extraction source roots cover.
- Raw file count is not the right coverage metric by itself because community
  packs duplicate many standard and vendor MIBs. Unique MIB module name count is
  the first useful coverage metric; trap count must be measured by extraction.

Inferences:

- The current stock pack is likely missing real trap definitions from community
  collections not present in the current source roots.
- The extra 6,999 unique module names will not all contain traps. The extraction
  run, not the inventory scan, must determine how many additional trap OIDs are
  actually useful.
- A single unstructured "point generator at every directory" approach would
  make conflict selection harder to review and may let lower-quality community
  copies override canonical sources unless priority tiers are explicit.

Unknowns:

- How many of the missing 6,999 unique module names contain
  `NOTIFICATION-TYPE` or `TRAP-TYPE` definitions.
- How many additional OID conflicts appear when all community packs are included.
- Whether `gomib` can compile the full mirrored corpus in one staged run without
  memory, time, or parse-regression problems.
- Whether any newly extracted trap records expose profile-schema or classifier
  weaknesses beyond the known linkUp/linkDown varbind-overuse issue.

### Acceptance Criteria

- A checked-in or generated corpus manifest lists every selected MIB source root
  with tier, upstream repository, checked commit, relative path, raw SMI file
  count, unique module count, and reason for inclusion or exclusion.
- A full inventory artifact records all discovered SMI module definitions across
  the mirrored repos and identifies which modules are covered by the extraction
  manifest and which are not.
- The Go extraction pipeline can run against the approved manifest and produce
  deterministic `traps.jsonl`, `extraction-report.json`,
  `source-conflicts.json`, and `conflicts.json`.
- The extraction report states: raw SMI files scanned, unique modules
  discovered, modules compiled, modules failed, traps extracted, trap OID
  conflicts, and module source conflicts.
- Newly extracted traps are compared against the current 50,198-trap baseline by
  OID, MIB module, trap name, varbind count, and source tier.
- The classifier prompt and profile message template rules are updated before
  full reclassification if the template syntax SOW changes the trap profile
  format.
- A representative sample of newly added standard, vendor, and community-pack
  traps is manually reviewed for varbind metadata, category, severity,
  description, labels, and cardinality discipline.
- The final emitted profile pack has no dangling varbind references, no empty
  varbind table entries, no duplicate trap names within a profile file, no
  invalid category/severity values, and no unresolved message placeholders.
- The generated catalogue stays deterministic and matches the emitted profiles.
- The stock full-corpus pack is shippable without forcing active trap jobs to
  retain the whole profile database in memory: runtime keeps a compact
  OID-to-profile router and lazy-loads stock vendor profiles on first matching
  trap.
- User-supplied profile files under `/etc/netdata/go.d/snmp.trap-profiles/` are
  always parsed, validated, and loaded at job creation time so operator errors
  fail during configuration apply, not during trap runtime.
- Stock profiles remain uncompressed `.yaml` files in the repository for review,
  but installer/package output ships stock vendor profiles as `.yaml.gz`.
- Job creation validates the stock profile index/manifest and referenced stock
  files enough to detect missing, unreadable, corrupt, or stale generated stock
  artifacts before the listener is reported as started.

## Analysis

Sources checked:

- `tools/snmp-traps-profile-gen/extract.py`
- `src/go/cmd/snmptrapprofilegen/main.go`
- `tools/snmp-traps-profile-gen/README.md`
- `.agents/sow/done/SOW-0033-20260523-snmp-mib-mechanical-extraction.md`
- `.agents/sow/done/SOW-0042-20260525-snmp-traps-extraction-pipeline-v2.md`
- `.agents/sow/specs/snmp-traps/netdata.md`
- local mirrored repos under `${NETDATA_REPOS_DIR}/repos`
- local session snapshots were searched only to recover old extraction
  context; no private session text is used as durable evidence here.

Current state:

- Legacy extractor current source list exists in
  `tools/snmp-traps-profile-gen/extract.py:28`.
- Go helper has no built-in corpus list; stock regeneration must supply
  repeated `--source-dir` values. The flag is defined in
  `src/go/cmd/snmptrapprofilegen/main.go:381` and missing source dirs fail in
  `src/go/cmd/snmptrapprofilegen/main.go:412`.
- Current extracted baseline under `tools/snmp-traps-profile-gen/output/` is
  historical output, not a clean full-corpus baseline.
- The old baseline reports:
  - 14,324 MIB modules discovered;
  - 8,265 compiled successfully;
  - 7,039 failures;
  - 50,198 trap records;
  - 1,433 cross-MIB OID conflicts.

Fresh mirror inventory evidence before adding the 2026-06-09 extra mirrors:

- Full mirror scan, with `.git`, `node_modules`, and compiled Python MIB outputs
  excluded but real MIB `vendor/` directories included:
  - 169,241 raw files containing `DEFINITIONS ::= BEGIN`;
  - 167,566 module rows parsed from those files;
  - 16,347 unique MIB module names.
- Current source roots from the legacy extractor contain:
  - 29,092 raw SMI definition files;
  - 9,348 unique MIB module names.
- Gap:
  - 6,999 unique MIB module names exist in the mirror but are not covered by the
    current extraction roots.

Fresh mirror inventory evidence after adding `elastiflow/snmp`,
`rickicook/SNMP-MIBS`, `librenms/librenms-mibs`, and the Observium Community
Edition mirror:

- Full mirror scan:
  - 186,825 raw files containing `DEFINITIONS ::= BEGIN`;
  - 184,997 module rows parsed from those files;
  - 20,227 unique MIB module names.
- Current source roots are unchanged:
  - 9,348 unique MIB module names.
- Updated gap:
  - 10,879 unique MIB module names exist in the mirror but are not covered by
    the current extraction roots.
- Trap-definition prefilter:
  - 74,929 mirrored SMI files contain `NOTIFICATION-TYPE` or `TRAP-TYPE`;
  - 8,811 unique mirrored module names contain `NOTIFICATION-TYPE` or
    `TRAP-TYPE`;
  - current roots cover 4,050 such unique module names;
  - 4,761 trap-definition-bearing unique module names are outside current
    roots.

The trap-definition prefilter is only a rough mechanical signal. It is not the
number of traps the generator will emit; the generator must still parse and
compile modules to count real trap records.

Full-corpus mechanical extraction run on 2026-06-09:

- Scratch output: `/tmp/snmp-trap-full-corpus-extract-20260609-233649`.
- Source roots: 48.
- Started: `2026-06-09T20:36:50Z`.
- Finished: `2026-06-09T20:38:11Z`.
- Elapsed: 81.3 seconds.
- Source files scanned by the generator: 187,678.
- Source modules requested: 20,446.
- Module load count reported by the generator: 51,988.
- Trap records emitted: 176,683.
- Unique trap OIDs: 151,574.
- Trap forms:
  - `NOTIFICATION-TYPE`: 108,480.
  - `TRAP-TYPE`: 68,203.
- Failed-batch report entries: 73.
- Unique module names involved in failed-batch entries: 288.
- Duplicate module/source-conflict groups: 15,525.

Interpretation:

- The extraction itself completed and produced `traps.jsonl`,
  `extraction-report.json`, and `source-conflicts.json`.
- The failed-batch entries are mostly malformed, renamed, or archive-only module
  names that the loader could not resolve after fallback to individual-module
  retries.
- The large duplicate-module count is expected from broad community archives,
  but it makes source-priority review mandatory before profile emission.
- Some modules emitted suspiciously large trap counts, for example several
  `DPS-MIB-*` modules with 4,096 records each and multiple `CPQHSV*` modules
  with more than 1,000 records. These must be sampled before classification to
  distinguish valid vendor trap tables from extraction noise.

Mechanical deduplication / conflict-winner run on 2026-06-09:

- Scratch output: `/tmp/snmp-trap-full-corpus-dedup-20260609-234745`.
- Command path: `generate` without `--classify`, with output and temporary
  emitted profiles under `/tmp`.
- Raw trap records before OID conflict resolution: 176,683.
- Winner trap records after OID conflict resolution: 151,574.
- Unique classifier hashes after OID conflict resolution: 151,574.
- OID conflicts resolved: 12,479.
- Temporary vendor buckets emitted: 817.
- Existing stock profile overlap:
  - current stock unique OIDs: 71,787;
  - current stock OIDs present in the deduped corpus: 71,754;
  - deduped corpus OIDs not in current stock: 79,820;
  - current stock OIDs not in the deduped corpus: 33.
- The stock-overlap counts above are exact-string OID comparisons. The runtime
  profile lookup also tolerates the SMIv1 / SMIv2 trap-OID ambiguity by adding
  or removing one `.0.` segment immediately before the final OID arc.
- Recomputed with runtime-equivalent exact-or-`.0` matching:
  - current stock OIDs matched by the deduped corpus: 71,754;
  - current stock OIDs still unmatched: 33;
  - deduped corpus OIDs matched by current stock: 71,784;
  - deduped corpus OIDs not matched by current stock: 79,790.
- Recomputed with canonical `.0` collapse for logical trap identities:
  - current stock logical OIDs: 71,487;
  - deduped corpus logical OIDs: 150,769;
  - logical overlap: 71,454;
  - logical new-vs-current: 79,315;
  - logical current-not-in-new: 33.
- The deduped corpus contains 805 `.0`-equivalent OID pairs. All 805 pairs have
  different qualified names, and 668 pairs differ by form
  (`TRAP-TYPE` vs `NOTIFICATION-TYPE`), which is consistent with SMIv1/SMIv2
  converted definitions.
- No existing `classification-cache.jsonl` was found in the working tree or the
  scratch outputs.

Generator normalization implementation on 2026-06-10:

- `generate --classify` now classifies only the normalized winner set, not the
  raw extracted records.
- The standalone Go `classify` subcommand also normalizes its JSONL input
  before model calls. It accepts repeated `--source-dir` values so separate
  extract/classify workflows can preserve source-priority tie breaking.
- Normalization order:
  1. resolve exact duplicate trap OIDs using existing source-priority,
     standard-tree, last-updated, and qualified-name rules;
  2. resolve SMIv1/SMIv2 trap-OID aliases that differ only by one `.0.`
     segment immediately before the final arc;
  3. emit profiles and call the classifier only for the final winner set.
- Exact OID conflicts continue to be written to `conflicts.json`.
- SMIv1/SMIv2 `.0.` alias conflicts are written separately to
  `dot0-conflicts.json`.
- `extraction-report.json` now includes:
  - `dot0_conflict_oids`;
  - `logical_trap_oids`;
  - normalized output-side module/form counts;
  - optional baseline profile overlap summary.
- When `--baseline-profiles-dir` is provided, the helper writes
  `baseline-overlap.json` with exact, runtime-equivalent, and logical-OID
  overlap/missing/new lists.
- `emit` also normalizes before writing YAML, so standalone emission from a raw
  JSONL file does not reintroduce exact or `.0.` duplicates.

Full-corpus normalized verification run on 2026-06-10:

- Scratch output: `/tmp/snmp-trap-full-corpus-normalized-20260610-000911`.
- Raw trap records: 176,683.
- Exact OID conflicts: 12,479.
- `.0.` alias conflicts: 805.
- Final pre-LLM winner records: 150,769.
- Final logical trap OIDs: 150,769.
- Output forms:
  - `NOTIFICATION-TYPE`: 101,587.
  - `TRAP-TYPE`: 49,182.
- Baseline overlap using current stock profiles:
  - current stock exact OIDs: 71,787;
  - candidate exact OIDs: 150,769;
  - exact overlap: 71,451;
  - exact candidate-new: 79,318;
  - exact baseline-missing: 336;
  - runtime-equivalent baseline matched: 71,754;
  - runtime-equivalent baseline missing: 33;
  - runtime-equivalent candidate-new: 79,315;
  - logical overlap: 71,454;
  - logical candidate-new: 79,315;
  - logical baseline-missing: 33.

High-volume module triage:

- Top remaining modules are real MIB-defined trap sets, not parser-created
  artifacts. For example, `DPS-MIB-NMT2SB-V10A` literally defines 4,096
  `TRAP-TYPE` entries for TBOS point set/clear events, with descriptions such
  as "Generated when TBOS port 5 display 1 point 1 is set".
- This does not prove the final profile pack should include every such module,
  but it changes the decision from "extractor bug" to "generated/point-mapped
  MIB volume policy".
- The top normalized output modules in the verification run are:
  - `DPS-MIB-NMT2SB-V10A-V2`: 4,096;
  - `DPS-MIB-NMT2SB-V10A`: 4,096;
  - `DPS-MIB-NMT2SA-V10A`: 4,096;
  - `DPS-MIB-NMT2SA-V10A-V2`: 3,456;
  - `TROPIC-NOTIFICATION-MIB`: 2,919;
  - `ISG-NSD-COMMON-TRAPS`: 1,990;
  - `ALBALA-MIB`: 1,962;
  - `HUAWEI-STORAGE-OID-BASED-TRAP-MIB`: 1,437;
  - `ENVIROMUX16D`: 1,418;
  - `HUAWEI-SERVER-IBMC-MIB`: 1,394.

Disk and memory footprint evidence:

- Current stock pack:
  - raw YAML directory size: 39,463,888 bytes;
  - gzip tar stream size: 3,417,901 bytes;
  - vendor YAML files: 437;
  - trap entries: 71,787.
- Normalized 150,769-trap scratch pack:
  - raw YAML directory size: 77,092,088 bytes;
  - gzip tar stream size: 4,855,958 bytes;
  - vendor YAML files: 817;
  - trap entries: 150,769.
- Temporary load-memory probe using `loadProfileCache()` and forced GC:
  - current stock retained heap after GC: 141,017,960 bytes;
  - normalized scratch retained heap after GC: 295,727,216 bytes;
  - current stock load total allocation delta: 721,110,024 bytes;
  - normalized scratch load total allocation delta: 1,446,069,144 bytes.
- Interpretation:
  - package/download footprint increase is modest when compressed;
  - raw installed profile footprint roughly doubles;
  - active trap-job retained heap roughly doubles and is the primary product
    risk;
  - profile memory remains lazy and shared across trap jobs, so users with no
    active SNMP trap job still do not pay the memory cost.

Interpretation:

- The immediate LLM-call upper bound after current mechanical deduplication is
  151,574 calls.
- If the `.0` trap-OID equivalence is accepted as a pre-LLM conflict class, the
  immediate upper bound drops to 150,769 logical trap identities.
- Under the user decision to re-run the full profile corpus after changing the
  prompt/template rules, the existing stock YAML should not be counted as a
  reusable classification cache.
- Further call reduction must come from pre-LLM structural filtering, such as
  source-manifest pruning, malformed/noisy-module exclusion, or explicit
  generated-pack scope decisions. It should not come from asking the model to
  resolve duplicate or low-quality extraction artifacts.

Major community packs contributing module names outside current roots:

- `MizaruIT/MIBS`: 5,849 unique module names outside current roots.
- `cisco-kusanagi/mibs.snmplabs.com`: 5,118 outside current roots.
- `splunk/splunk-connect-for-snmp-mib-server`: 5,093 outside current roots.
- `zakiharis/snmp_mibs_files`: 4,874 outside current roots.
- `trevoro/snmp-mibs`: 4,418 outside current roots.
- `Marll22/SNMP-MIB-Files`: 1,314 outside current roots.
- `RenJiangZhou2163/MIBs`: 822 outside current roots.
- `nielsb/mibdepot`: 199 outside current roots.

Additional mirrors added on 2026-06-09:

| Repository | Checked commit | Raw SMI files | Unique modules | Outside current roots | Trap-ish files | Notes |
|---|---:|---:|---:|---:|---:|---|
| `rickicook/SNMP-MIBS` | `e7a12bc5a90f` | 1,606 | 1,564 | 361 | 473 | Raw `.mib` / `.txt` vendor files. |
| `librenms/librenms-mibs` | `ee6e5612efec` | 1,977 | 1,724 | 47 | 696 | Archived flat raw-MIB repo; mostly overlaps current LibreNMS tree but still useful for inventory proof. |
| `DanielleHuisman/observium-community-edition` | `e73f0af9ed24` | 14,001 | 13,562 | 6,436 | 6,243 | Observium `mibs/` tree; contributed 3,880 unique module names not present in the previous full mirror inventory. |
| `elastiflow/snmp` | `e468f2404300` | 0 | 0 | 0 | 0 | Not a raw ASN.1 MIB corpus; structured device/object/enum definitions under `autodiscover/`, `device_groups/`, `object_groups/`, and `enums/`. |

Interpretation:

- Observium materially changes the full-corpus baseline and must be included in
  the new manifest policy discussion.
- `rickicook/SNMP-MIBS` and archived `librenms/librenms-mibs` add little unique
  module coverage beyond what the enlarged mirror already had, but they help
  prove overlap and may contain alternate copies worth conflict reporting.
- `elastiflow/snmp` should not be fed to ASN.1 extraction. It is a separate
  structured-profile evidence source that may help validate device identity,
  object-group coverage, enum mappings, and possibly future polling/trap
  cross-links.

Popular enterprise vendor spot-check:

| Vendor family | Mirror modules | Current roots | Outside current roots |
|---|---:|---:|---:|
| Cisco | 1,706 | 1,680 | 26 |
| Juniper | 318 | 312 | 6 |
| Arista | 40 | 31 | 9 |
| HPE / Aruba / H3C | 902 | 829 | 73 |
| Fortinet | 22 | 17 | 5 |
| Palo Alto | 7 | 7 | 0 |
| F5 | 12 | 11 | 1 |
| Check Point | 5 | 3 | 2 |
| Dell | 179 | 164 | 15 |
| Extreme / Brocade / Ruckus | 167 | 162 | 5 |
| Huawei | 512 | 304 | 208 |
| Nokia / Alcatel / Timetra | 282 | 234 | 48 |
| D-Link | 409 | 163 | 246 |
| MikroTik | 1 | 1 | 0 |
| Ubiquiti | 8 | 7 | 1 |
| NetApp | 1 | 1 | 0 |
| APC / Schneider | 4 | 3 | 1 |
| Eaton | 12 | 5 | 7 |
| Vertiv / Emerson / Liebert | 23 | 14 | 9 |
| VMware | 31 | 30 | 1 |
| Synology / QNAP | 12 | 12 | 0 |

Interpretation:

- The mirrored corpus already includes the common enterprise vendor families.
- The current extraction roots already include many representative modules from
  those vendors, but they miss newer or less common variants from broad
  community packs.
- Large current-root misses are concentrated in Huawei, D-Link, HPE/Aruba/H3C,
  Nokia/Alcatel/Timetra, and smaller product-line variants for Cisco, Arista,
  Dell, Eaton, and Vertiv/Liebert.

Representative modules present in the mirror but outside current roots:

- `CISCO-FIREPOWER-AP-BMC-MIB`
- `ARISTA-CLB-MIB`
- `HUAWEI-BRAS-RUI-TRAP-MIB`
- `EATON-EPDU-MIB` variants beyond the current selected copy
- `LIEBERT-GP-AGENT-MIB` variants beyond the current selected copy

Potential additional mirrors to evaluate:

- The four previously identified candidates are now mirrored and inventoried.
- Vendor official download surfaces should be treated separately from GitHub
  mirroring. Cisco has an official GitHub MIB repo and is already mirrored.
  Arista and Palo Alto publish direct MIB download pages. Juniper, Fortinet,
  and F5 expose MIB packages or explorers through vendor support/download
  surfaces; do not scrape gated portals without explicit approval and license
  review.

Risks:

- **Conflict risk:** larger community packs carry duplicate and stale copies.
  If priority is not explicit, lower-quality copies can override canonical MIBs.
- **Compile-time risk:** the full mirror may create many compile failures; these
  must be reported, not hidden.
- **Reviewability risk:** full profile regeneration may produce a very large
  diff. The manifest and inventory must allow reviewers to understand coverage
  changes before reviewing generated YAML.
- **Prompt-quality risk:** existing descriptions can overuse optional varbinds.
  Reclassification must wait until the trap message templating and prompt rules
  are settled.
- **Performance risk:** extracting and classifying a larger corpus can take
  hours and high memory. Runs must use `/tmp` or another scratch output path,
  never production directories.
- **Security/privacy risk:** MIB corpora are public OSS mirrors, but generated
  logs and SOW evidence must not include secrets, SNMP communities, customer
  names, private endpoints, or real network-device data.

Classifier prompt and template preparation:

- Current state:
  - The classifier prompt now emits restricted Go `text/template`
    descriptions instead of direct `{var}` / `{var.raw}` placeholders.
  - Expose a small validated function API to the model:
    - `{{hostname}}`
    - `{{source_ip}}`
    - `{{trap_name}}`
    - `{{vendor}}`
    - `{{value "varbindName"}}`
    - `{{raw "varbindName"}}`
    - `{{first ...}}`
  - Allow only the small control subset needed for optional text: `with`,
    `else`, and `end`.
  - Reject unknown functions, unknown varbind names, parse errors, and forbidden
    template actions at profile load / job creation time.
  - At runtime, a known but absent varbind renders as empty or activates a
    fallback; it must not render as `<missing>`.
- Prompt rule:
  - The classifier must treat every trap varbind as potentially absent from
    real devices unless the MIB and SNMP standard prove otherwise.
  - The classifier must use varbinds only when the message cannot be useful
    without them.
  - If the trap identity itself provides the state, the classifier must derive
    the message from the trap, not from optional varbinds.
  - Link up/down traps must say the interface went up/down based on the trap
    type. They must not say "changed to {{value \"ifOperStatus\"}}" unless the
    MIB explicitly proves that varbind is mandatory and is the new state.
- Completed checks:
  - The parser rejects legacy single-brace placeholders.
  - The parser rejects unknown functions, unknown varbind names, parse errors,
    variables, assignments, pipelines, comparisons, arithmetic, templates,
    blocks, arbitrary functions, and forbidden actions such as `range`.
  - Restricted `if` blocks were initially accepted, but quality sampling found
    this unsafe with the current function set: `value` returns a string, and
    Go template `if` treats any non-empty string as truthy. This can turn
    boolean-looking varbinds such as `false` / `disconnected` into the true
    branch. The corrected target is to reject `if` in both generator and runtime
    profile validation; `with` / `else` remains the supported presence-based
    conditional.
  - The parser rejects descriptions that reference `{{hostname}}` more than
    once; hostname should appear only in the final ` on {{hostname}}.` suffix.
  - The parser repairs deterministic model mistakes before schema/template
    validation:
    - severity words used as `category` are mapped through the mechanical
      category classifier;
    - common invalid categories such as `threshold` and `redundancy` are mapped
      to the closed taxonomy;
    - `{{value "trap_interface"}}`-style built-in calls are rewritten to the
      built-in action;
    - bare `{{varbindName}}` actions are rewritten to
      `{{value "varbindName"}}` when the varbind is in the allowed set;
    - unique varbind suffix repair accepts six-character suffixes, which fixed
      cases like `hwAsmngTrapSlotId` → `hwAsmngTrapAsSlotId`.
  - Final accepted cache rows have valid JSONL, no accepted
    `range` / template / define / block actions, no legacy `{_HOSTNAME}`, no
    `<missing>`, no duplicate hostname references, and all end with
    ` on {{hostname}}.`.
- Remaining blocker:
  - Do not commit regenerated stock profiles until the accepted cache passes
    targeted quality sampling and the stock-pack lazy-loading / installed gzip
    work is ready.
  - Revalidate the completed cache after rejecting `if`; the expected invalid
    set is only the three rows found by `/tmp/snmp-trap-full-corpus-qa-20260610-084554/if-template-details.tsv`.

Full-corpus LLM classification run on 2026-06-10:

- Scratch output: `/tmp/snmp-trap-full-corpus-llm-20260610-010631`.
- Input: `/tmp/snmp-trap-full-corpus-normalized-20260610-000911/traps.jsonl`.
- Model: `qwen3.6-35b-a3b-nothinker`.
- Concurrency: 24.
- Started: `2026-06-10T01:06:31+03:00`.
- Initial run result:
  - last cache/log write: `2026-06-10T04:59:43+03:00`;
  - exit status: non-zero;
  - cached classifications: 150,721 / 150,769;
  - missing classifications: 48;
  - cause: `CISCO-ITP-GRT-MIB::ciscoGrtDestStateChange` failed after five
    retries because the model kept producing forbidden `if` template actions.
- Resume runs:
  - after stricter duplicate-hostname validation and better repairs, resume
    attempts reduced missing hashes from 48 → 24 → 6;
  - after allowing restricted `if` in both generator and runtime validation,
    and after suffix/bare-varbind/category repairs, the final resume completed
    all remaining hashes;
  - quality sampling then proved `if value` unsafe because `value` returns a
    string and Go template `if` treats any non-empty string as true. The
    generator/runtime validators were changed to reject `if` and the three
    accepted `if` rows were reclassified with `with`-based wording.
- Final cache/output status:
  - input normalized traps: 150,769;
  - cached classifications: 150,769;
  - enriched output rows: 150,769 in
    `/tmp/snmp-trap-full-corpus-llm-20260610-010631/enriched-no-if.jsonl`;
  - missing classifications: 0;
  - cache JSONL validation: pass;
  - enriched JSONL validation: pass.
- Final retry distribution in the accepted cache:
  - attempt 1: 149,240;
  - attempt 2: 1,411;
  - attempt 3: 79;
  - attempt 4: 24;
  - attempt 5: 15.
- Validation pressure observed in the log:
  - validation failures: 1,903;
  - forbidden `if` template action attempts: 68;
  - descriptions missing the required suffix before retry: 145;
  - over-500-character descriptions before retry: 3;
  - invalid category values before retry were mostly severity words such as
    `info`, `warning`, and `notice`.
- Final accepted-cache category distribution:
  - diagnostic: 85,276;
  - state_change: 42,386;
  - config_change: 13,956;
  - security: 5,167;
  - auth: 1,816;
  - license: 1,061;
  - mobility: 727;
  - unknown: 380.
- Final accepted-cache severity distribution:
  - warning: 60,805;
  - notice: 60,380;
  - crit: 16,212;
  - alert: 6,133;
  - err: 3,830;
  - info: 3,070;
  - emerg: 267;
  - debug: 72.
- Final quick quality flags in accepted rows:
  - 0 descriptions contain `{{hostname}}` more than once;
  - 0 descriptions contain `{{if`;
  - 0 descriptions contain legacy single-brace placeholders after stripping
    valid double-brace actions;
  - 0 descriptions contain `<missing>`, `<unresolved>`, `{{range`,
    `{{template`, `{{define`, or `{{block`;
  - 3 descriptions exceed 220 characters;
  - 100 descriptions contain SNMP/MIB/OID jargon;
  - 9,297 descriptions match `changed to {{value ...}}`, which may be valid
    for some state traps but must be sampled because it can also indicate
    optional-varbind overuse.
- Scratch profile emission:
  - output: `/tmp/snmp-trap-full-corpus-emit-no-if-20260610-085446`;
  - emitted YAML files: 817;
  - emitted trap entries: 150,769;
  - raw emitted profiles size: 80 MB;
  - catalogue size: 476 KB;
  - gzip tar stream for profiles plus catalogue:
    `/tmp/snmp-trap-full-corpus-emit-no-if-20260610-085446-profiles.tar.gz`,
    5.8 MB;
  - runtime loader validation: pass via temporary Go program that set
    `executable.Directory` to a synthetic stock config tree, called
    `AcquireProfileCache()`, and resolved standard `linkDown` / `linkUp`
    profile lookups.

## Pre-Implementation Gate

Status: in-progress

Problem / root-cause model:

- The stock trap profile pack was generated from a curated source list, not from
  a complete mirrored-MIB inventory.
- This happened because the first extraction SOW optimized for known community
  packs and then repaired one missed canonical source, but did not leave behind
  a full-mirror inventory gate proving all mirrored MIB modules were either
  included or intentionally excluded.
- Evidence: the fresh inventory found 16,347 unique module names across the
  mirror, while the current source roots cover 9,348.

Evidence reviewed:

- `tools/snmp-traps-profile-gen/extract.py:28`: legacy
  `DEFAULT_SOURCE_DIRS`.
- `src/go/cmd/snmptrapprofilegen/main.go:381`: `--source-dir` is repeatable.
- `src/go/cmd/snmptrapprofilegen/main.go:412`: missing source dirs fail.
- `tools/snmp-traps-profile-gen/README.md:109`: current stock regeneration
  workflow uses explicit `--source-dir`.
- SOW-0033 records the prior `pysnmp/mibs/src` omission and repair.
- SOW-0042 records that the Go generator is the active extraction pipeline.
- Open-source mirror evidence is listed below using upstream repository and
  checked commit.

Affected contracts and surfaces:

- `src/go/cmd/snmptrapprofilegen/` extraction, classification, emission, tests.
- `tools/snmp-traps-profile-gen/README.md` regeneration documentation.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/*.yaml`
  generated stock profiles.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json`
  generated catalogue.
- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md` if the
  template syntax changes before regeneration.
- `src/go/plugin/go.d/collector/snmp_traps/` profile loading, lookup, reload,
  and tests.
- `CMakeLists.txt` and package/install surfaces that currently install stock
  trap profiles as plain `.yaml`.
- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` if the
  regeneration workflow or manifest becomes mandatory.
- `.agents/sow/specs/snmp-traps/netdata.md` if profile-generation guarantees or
  corpus policy change.
- PR review surface: generated profile pack size and generated data diff.

Clean-end-state target:

- The repo has a deterministic full-corpus generation workflow driven by a
  reviewable MIB corpus manifest, not by an implicit local hard-coded list.
- The manifest tiers sources so canonical standards and vendor-owned repos win
  over broad community archives during duplicate module and duplicate OID
  conflicts.
- The extraction artifacts prove exactly which mirrored MIB modules were
  included, excluded, compiled, failed, and emitted as trap profiles.
- The stock trap profile pack is regenerated from the approved manifest after
  the message-template/profile-format work is settled.
- Removed as redundant (i):
  - any undocumented stock-regeneration command that relies on remembered local
    paths;
  - any stale statement that the 50,198-trap extracted corpus is the broadest
    available mirrored corpus.
- Excluded coupled items (ii):
  - shipping the regenerated profile pack without first resolving the trap
    message-template syntax and classifier prompt rules; reason: the current
    linkUp/linkDown issue shows classification guidance affects generated
    profile correctness.
  - making the installed operator helper automatically scan the workstation
    mirror; reason: installed Netdata users do not have this mirror, and
    operator conversion must remain explicit via `--source-dir`.
- Reference search:
  - No public path or schema is replaced yet. Before implementation, search for
    `DEFAULT_SOURCE_DIRS`, `--source-dir`, `traps.jsonl`,
    `extraction-report.json`, `source-conflicts.json`, and stock regeneration
    command examples, then update all surviving references or record exclusions.

Existing patterns to reuse:

- Go helper subcommands: `extract`, `classify`, `emit`, `generate`.
- Existing extraction artifacts: `traps.jsonl`, `extraction-report.json`,
  `source-conflicts.json`, `conflicts.json`, `classification-cache.jsonl`.
- Existing deterministic JSONL/YAML emission and profile validators.
- Existing profile authoring skill rules: MIB-qualified trap names, file-scoped
  varbind tables, no dangling varbinds, closed categories/severities, label
  cardinality discipline.
- Existing SOW-0042 design: source priority and conflict reports instead of
  silent first-wins behavior.

Risk and blast radius:

- Generated YAML pack may grow substantially and make PR review harder.
- A bigger profile pack may affect repository size and install package size.
- More profile files can increase profile-load time and memory for enabled trap
  jobs, although profiles are only loaded on first trap job creation.
- Broad community packs can contain broken, obsolete, or vendor-renamed MIBs.
- Reclassification can be expensive and must be reproducible through a stable
  cache and validation report.
- The final pack must not regress runtime hot path lookup performance.

Sensitive data handling plan:

- Use only public mirrored MIB repositories and sanitized local command summaries
  in durable artifacts.
- Do not write raw SNMP communities, device IPs, customer names, private
  endpoints, API keys, bearer tokens, response headers, or real network trap
  payloads into the SOW, specs, docs, skills, or comments.
- Scratch extraction outputs must be written under `/tmp` or another explicit
  scratch path, not `/var/cache/netdata/traps/` or production Netdata
  directories.
- If any generated artifact unexpectedly contains sensitive local data, stop,
  sanitize, and record only redacted evidence.

Implementation plan:

1. Build a corpus inventory mode or standalone script around the Go generator:
   scan approved roots for SMI module definitions, parse module names, and write
   `mib-inventory.jsonl` plus summary JSON.
2. Add a tiered corpus manifest file for stock regeneration:
   canonical standards first, vendor-owned repos next, broad community packs
   later, and tiny incidental MIB roots last or excluded with reasons.
3. Update extraction to consume the manifest or a manifest-expanded source list
   while preserving deterministic source priority and conflict reporting.
4. Run inventory and extraction into a scratch output directory.
5. Compare new extraction output against the current 50,198-trap baseline.
6. Fix extractor correctness issues found by the expanded corpus before
   classification.
7. After the template/prompt SOW is settled, reclassify all cache misses and
   regenerate the stock pack.
8. Validate generated profiles, catalogue, docs, and skill updates.
9. Add a generated stock routing index so the runtime does not need to retain
   the whole stock profile database:
   - enterprise trap OIDs route by enterprise prefix to one stock vendor file;
   - non-enterprise trap OIDs use a small exact-OID route table;
   - user profile files are still loaded and validated eagerly at job creation;
   - a user file with the same basename as a stock file replaces that stock file,
     preserving the documented multipath override semantics.
10. Add runtime `.yaml.gz` support for stock profile files:
    - repository keeps stock YAML uncompressed for review;
    - installer/package output may ship stock vendor profiles compressed;
    - loader reads both `.yaml` and `.yaml.gz`;
    - first trap job creation validates the stock index and referenced stock
      files, but retains only the small routing index plus eagerly loaded user
      profiles.
11. Lazy-load stock vendor files on first matching trap:
    - load exactly the routed stock file;
    - merge loaded traps into the shared index once;
    - keep the loaded vendor file shared across all listener jobs;
    - propagate lazy-load errors into self-metrics/logs while startup validation
      keeps expected package/file failures at job creation.

Validation plan:

- Unit tests for manifest parsing, source priority, duplicate module reporting,
  inventory output, and path-boundary handling.
- Determinism test: run inventory/extraction twice on the same manifest and diff
  JSONL/JSON artifacts.
- Full-corpus extraction run with summary counts and failure report.
- Baseline comparison against old `extracted.jsonl` by OID, module, trap name,
  varbind count, and profile vendor bucket.
- Generated profile validation:
  - no unresolved varbind references;
  - no empty varbind entries;
  - all categories/severities valid;
  - all trap names MIB-qualified;
  - catalogue matches emitted files.
- Lazy stock-profile validation:
  - generated stock route index covers all emitted stock trap OIDs;
  - user profiles are loaded immediately at job creation;
  - stock route index and referenced stock files are validated at job creation;
  - `.yaml` and `.yaml.gz` stock files both load;
  - user same-basename replacement prevents loading the replaced stock file;
  - first lookup for a stock trap loads only that routed stock file and then
    reuses it for later lookups;
  - existing duplicate-OID/name failure semantics are preserved for loaded
    profiles.
- Manual sample review of newly added traps from:
  - standards;
  - Cisco;
  - D-Link or equivalent switch vendor;
  - firewall/load-balancer vendors;
  - storage/UPS/environmental vendors;
  - at least one broad community-only source.
- Runtime smoke test with a small generated profile subset and synthetic traps.
- External reviewer pass only after the SOW implementation is complete as one
  meaningful batch.

Artifact impact plan:

- AGENTS.md: likely unaffected unless SOW workflow changes.
- Runtime project skills: update
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` if the corpus
  manifest becomes mandatory for stock regeneration.
- Specs: update `.agents/sow/specs/snmp-traps/netdata.md` if stock profile
  coverage guarantees or generation workflow are clarified.
- End-user/operator docs: likely unaffected for internal stock regeneration;
  update only if operator helper behavior changes.
- End-user/operator skills: likely unaffected unless profile authoring guidance
  changes.
- SOW lifecycle: branch-local working file; durable knowledge must move into
  specs/skills/docs/code/tests before merge; delete SOW before final merge.

Open-source reference evidence:

- `pysnmp/mibs @ c6259b44b1ba`: `src/standard/`, `src/vendor/`.
- `netdisco/netdisco-mibs @ e981548ffb72`: `rfc/`, vendor directories.
- `librenms/librenms @ c6834877795c`: `mibs/`.
- `cisco/cisco-mibs @ e72ac6cda222`: `v1/`, `v2/`, product MIB dirs.
- `Poil/MIBs @ 141ba7f350a7`: root MIB archive and `cisco_v2/`.
- `kcsinclair/mibs @ 98fccc8acc58`: root MIB archive.
- `kmalinich/snmp-mibs @ 4ad06ab6c6d2`: root MIB archive.
- `hsnodgrass/snmp_mib_archive @ a96880991cc5`:
  `snmp_mib_archive/`.
- `Marll22/SNMP-MIB-Files @ 53b95dffd4a2`: vendor letter directories.
- `MizaruIT/MIBS @ 38e88f697c60`: `ASN1-FORMAT/`.
- `zakiharis/snmp_mibs_files @ 10714807fd3d`: `MIBS/`.
- `splunk/splunk-connect-for-snmp-mib-server @ 93da55de9c41`: `mibs/`.
- `cisco-kusanagi/mibs.snmplabs.com @ 48bb18954e5d`: `asn1/`.
- `nielsb/mibdepot @ cf3acf90317a`: `mibs/`.
- `trevoro/snmp-mibs @ 23f61f6ff977`: `mibs/`.
- `rickicook/SNMP-MIBS @ e7a12bc5a90f`: raw MIB file tree.
- `librenms/librenms-mibs @ ee6e5612efec`: archived flat raw-MIB tree.
- `DanielleHuisman/observium-community-edition @ e73f0af9ed24`: `mibs/`.
- `elastiflow/snmp @ e468f2404300`: `autodiscover/`,
  `device_groups/`, `object_groups/`, and `enums/`.

Open decisions:

1. Corpus policy:
   - A. Full mirrored corpus manifest with tiered priority.
   - B. Current roots plus only the six largest missing community packs.
   - C. Current roots plus hand-picked packs after manual review.
2. Generated profile delivery:
   - A. Regenerate and ship full stock pack in this PR after validation.
   - B. Produce extraction and classification artifacts first, then split the
     generated pack into a separate PR.
3. Timing relative to message-template SOW:
   - A. Finish trap message-template/profile-format change first, then
     classify/regenerate all profiles once.
   - B. Extract now, but block classification/emission until template work is
     settled.

## Implications And Decisions

### Decision 1 - Corpus Policy

Evidence:

- Current source roots cover 9,348 unique module names.
- The mirror contains 16,347 unique module names.
- About 6,999 unique module names are outside current roots.

Options:

- **A. Full mirrored corpus manifest with tiered priority.**
  - Pros: best coverage, explicit exclusions, repeatable, reviewer-friendly.
  - Cons: highest extraction/classification cost; largest generated diff.
  - Implications: manifest and inventory become mandatory stock-regeneration
    artifacts.
  - Risks: more broken MIBs and conflicts, but they become visible.
- **B. Current roots plus six largest missing community packs.**
  - Pros: captures most missing modules with less scope.
  - Cons: still leaves undocumented omissions.
  - Implications: future reviewers can still ask why smaller sources were
    excluded.
  - Risks: repeats the original class of bug, just with a larger curated list.
- **C. Hand-picked packs after manual review.**
  - Pros: smallest diff and easiest to explain in a short PR.
  - Cons: not "all MIBs"; subjective and incomplete.
  - Implications: requires follow-up work for the rest.
  - Risks: fails the user-stated purpose.

Recommendation: **A, long-term-best**. The core problem is lack of a complete
inventory gate. A full tiered manifest fixes the class of problem, while B/C
only reduce its size.

Decision: **A selected by user on 2026-06-09**. Run against the full mirrored
corpus with source priority tiers; extraction artifacts will prove actual
compiled and emitted trap coverage.

### Decision 2 - Generated Profile Delivery

Options:

- **A. Regenerate and ship full stock pack in this PR.**
  - Pros: one branch contains the complete feature state.
  - Cons: PR may become very large and hard to review.
  - Implications: reviewers must review generator, manifest, and generated
    profile diff together.
  - Risks: generated-data volume may obscure code review.
- **B. Produce extraction/classification artifacts and split generated pack into
  a separate PR.**
  - Pros: code/manifest review is easier; generated data can be reviewed with
    clear provenance later.
  - Cons: requires PR sequencing.
  - Implications: current PR cannot claim final stock-pack coverage until the
    generated-pack PR lands.
  - Risks: if not tracked tightly, generated data can lag the generator.

Recommendation: **B, surgical for PR review**, unless maintainers explicitly
prefer one huge generated-data PR. This does not lower the final target; it
only stages review around generated artifacts.

Decision: **A selected by user direction to proceed with the regenerated stock
pack on 2026-06-10**. The branch now contains the full generated pack. If
maintainers later require split PRs, this generated-pack commit can be moved to
its own PR without changing the generator/runtime work.

### Decision 3 - Timing Relative To Message Templates

Options:

- **A. Finish the message-template/profile-format SOW first, then classify and
  regenerate once.**
  - Pros: avoids paying full classification cost twice; prevents regenerating
    profiles with a known weak message syntax.
  - Cons: full-corpus profile pack waits on template work.
  - Implications: this SOW can still do inventory/extraction work first.
  - Risks: low, if extraction artifacts are kept separate from emitted stock
    profiles until template work is done.
- **B. Extract now and block classification/emission until template work is
  settled.**
  - Pros: starts mechanical work immediately.
  - Cons: SOW stays partially complete for longer.
  - Implications: classification cache remains untouched until the prompt is
    final.
  - Risks: low, as long as no generated stock profiles are emitted from stale
    prompt rules.

Recommendation: **B now, then A for classification/emission**. Inventory and
mechanical extraction are independent enough to proceed; classification and
profile emission must wait for the template/prompt decision.

Decision: **B selected by user on 2026-06-09**. Run mechanical extraction in a
scratch directory now. Do not run LLM classification or emit stock profiles
until the classifier prompt and profile template rules are settled.

## Plan

1. Confirm active-SOW sequencing and mark this SOW `ready` only when it is the
   active implementation target.
2. Add a reviewable full-corpus manifest format and fixtures.
3. Add inventory output for all discovered SMI module definitions.
4. Wire the Go generator to consume the manifest or generated source list.
5. Run full inventory and extraction in scratch output.
6. Compare against the old 50,198-trap baseline.
7. Fix extractor bugs found by the expanded corpus.
8. Pause before classification/emission if message-template/profile-format work
   is not complete.
9. Reclassify, emit, validate, and review as one meaningful batch.

## Execution Log

### 2026-06-09

- Created planning SOW from current mirror inventory evidence.
- No implementation files changed.
- User approved extraction-only execution for the full mirrored corpus.
- Updated SOW to `in-progress` for mechanical extraction. LLM classification and
  profile emission remain blocked until prompt/template rules are ready.
- Full-corpus extraction completed in scratch:
  - `traps.jsonl`: 176,683 trap records.
  - `extraction-report.json`: 151,574 unique trap OIDs.
  - `source-conflicts.json`: 15,525 duplicate module/source-conflict groups.
- Recorded prompt/template target for the next implementation step. No
  classification or stock-profile emission has been run from the new corpus.
- Ran scratch `generate` without `--classify` to apply the generator's current
  OID conflict-winner logic before any LLM calls:
  - winner records: 151,574;
  - OID conflicts resolved: 12,479;
  - unique classifier hashes: 151,574;
  - current-stock overlap: 71,754 OIDs;
  - new-vs-current OIDs: 79,820.
- Implemented generator-side pre-LLM normalization:
  - exact OID conflicts are resolved first;
  - `.0.` SMIv1/SMIv2 aliases are resolved second;
  - `generate --classify` now classifies only final winners;
  - standalone `classify` also normalizes its input before model calls;
  - `emit` also normalizes before writing YAML;
  - optional baseline overlap reporting is available through
    `--baseline-profiles-dir`.
- Updated regeneration README to document the normalized classifier input set
  and new report files.
- Full scratch verification after implementation produced 150,769 final
  pre-LLM records and 805 `.0.` alias conflicts.

### 2026-06-10

- Ran the full-corpus LLM classification in scratch with 24 client workers
  against the local 16-concurrent model server:
  - input: `/tmp/snmp-trap-full-corpus-normalized-20260610-000911/traps.jsonl`;
  - records: 150,769;
  - initial cache/output completed with 150,769 records.
- QA found three generated descriptions using `{{if ...}}`. This was unsafe
  because runtime `value` returns a string and Go templates treat any non-empty
  string as true, so comparisons expressed through `if value` would not mean
  what profile authors intended.
- Hardened the profile template contract:
  - generator prompt now allows `with/else/end`, not `if`;
  - generator semantic validation rejects `parse.IfNode`;
  - runtime profile validation rejects `parse.IfNode`;
  - profile-format docs and SNMP traps spec document `with`/`first` only.
- Reclassified the three affected cached rows and wrote repaired output:
  - `/tmp/snmp-trap-full-corpus-llm-20260610-010631/enriched-no-if.jsonl`;
  - 150,769 rows;
  - 0 `{{if`;
  - 0 legacy single-brace placeholders after stripping valid double-brace
    actions;
  - 0 `<missing>` / `<unresolved>`.
- Emitted a scratch stock profile pack from the repaired output:
  - output: `/tmp/snmp-trap-full-corpus-emit-no-if-20260610-085446`;
  - profile files: 817;
  - traps: 150,769;
  - raw profile size: about 80 MB;
  - gzip tar stream for profiles plus catalogue: about 5.8 MB.
- Implemented lazy stock-profile loading:
  - operator/user profiles load eagerly at job creation;
  - stock files are parsed and validated at job creation, but only a compact
    OID-to-stock-file route table is retained;
  - enterprise trap OIDs route by `1.3.6.1.4.1.<PEN>`;
  - non-enterprise OIDs and ambiguous enterprise-prefix cases use exact routes;
  - first matching trap loads the routed stock file into the shared profile
    index;
  - user same-basename replacement treats `vendor.yaml` and installed
    `vendor.yaml.gz` as the same logical stock file.
- Implemented gzip-aware profile loading and packaging:
  - runtime accepts `.yaml`, `.yml`, `.yaml.gz`, and `.yml.gz`;
  - `extends: base.yaml` resolves installed `base.yaml.gz` when needed;
  - CMake install compresses stock trap profile YAMLs with `gzip -f -9`;
  - CMake configure now fails if `gzip` is unavailable while `plugin-go` is
    enabled, so packages cannot silently ship raw stock profiles.
- Added a lookup error path for post-start stock lazy-load failures:
  - packet handling increments `profile_load_failed` and logs the failure;
  - the trap is still journaled with raw OID/varbind data;
  - genuine unknown OIDs still increment `unknown_oid`.
- Replaced the repository stock profile pack from the repaired scratch emission:
  - source: `/tmp/snmp-trap-full-corpus-emit-no-if-20260610-085446/profiles/`;
  - committed profile files: 817;
  - committed trap entries: 150,769;
  - committed raw profile directory size: about 80 MB;
  - committed catalogue size: 476 KB;
  - largest generated files remain under the skill's 10 MB reviewability
    threshold.
- Committed implementation and generated-pack changes:
  - `77b6d745ef Lazy-load SNMP trap stock profiles`;
  - `27434c72fa Regenerate SNMP trap stock profiles`.
- Addressed the valid full-corpus review findings that did not require a new
  product decision:
  - CMake now installs and compresses stock `.yml` trap profiles in addition to
    `.yaml`, matching the runtime loader's accepted extensions.
  - Added a repository-pack guard test that verifies `catalogue.json` entries,
    profile file names, and per-file trap counts stay in sync.
  - Added a lazy-stock override test to prove operator overrides apply after a
    stock vendor profile is loaded on demand and do not mutate the shared stock
    trap definition.
  - Strengthened the IF-MIB `linkDown`/`linkUp` stock-profile test so it covers
    traps without `ifIndex`, rejects `ifOperStatus`/`ifAdminStatus` wording, and
    rejects the old `changed to <missing>` failure class.
- Recorded the deliberate job-creation validation trade-off:
  - stock route building parses and validates the stock profile files at job
    creation so missing, corrupt, or invalid packaged profiles fail during
    DynCfg apply/startup instead of after traps arrive;
  - retained runtime memory stays lazy because loaded stock `TrapDef` entries
    are retained only after first matching trap lookup;
  - focused stock-profile test timing on the workstation was about 3.6 seconds,
    while the full `snmp_traps` package test took about 7.6 seconds.

## Validation

Acceptance criteria evidence:

- Mechanical extraction artifact evidence exists under
  `/tmp/snmp-trap-full-corpus-extract-20260609-233649`.
- Repaired classification artifact exists at
  `/tmp/snmp-trap-full-corpus-llm-20260610-010631/enriched-no-if.jsonl`.
- Scratch generated profile pack exists at
  `/tmp/snmp-trap-full-corpus-emit-no-if-20260610-085446`.

Tests or equivalent validation:

- `extraction-report.json` confirms extraction completion, emitted trap count,
  unique OID count, failed-batch count, and source-conflict count.
- Scratch post-conflict `generate` run confirms the current pre-LLM winner set
  and LLM-call upper bound.
- `go test ./cmd/snmptrapprofilegen` passes.
- Full scratch `generate` without `--classify` passes against the 48-root
  mirrored corpus and writes `conflicts.json`, `dot0-conflicts.json`,
  `source-conflicts.json`, `baseline-overlap.json`, and normalized
  `traps.jsonl`.
- Repaired enriched output QA:
  - 150,769 rows;
  - 0 `{{if`;
  - 0 `<missing>`;
  - 0 `<unresolved>`;
  - 0 unsupported `range`/`template`/`define`/`block` actions.
- Scratch emitted profile QA:
  - 817 YAML files;
  - 150,769 emitted trap entries;
  - 0 `{{if`;
  - 0 `<missing>`;
  - 0 `<unresolved>`.
- Committed profile-pack QA:
  - 817 YAML files;
  - 150,769 emitted trap entries;
  - raw profile directory size: 80 MB;
  - `catalogue.json` size: 476 KB;
  - 0 `{{if`;
  - 0 `<missing>`;
  - 0 `<unresolved>`.
- Targeted runtime loader validation passed against the scratch emitted pack
  through `AcquireProfileCache()` with a synthetic stock config tree and
  standard `linkDown`/`linkUp` lookups.
- `go test ./cmd/snmptrapprofilegen ./plugin/go.d/collector/snmp_traps`
  passes after template, lazy-load, gzip, and generated-pack changes.
- Focused post-review tests pass:
  - `go test ./plugin/go.d/collector/snmp_traps -run 'TestOverridesApplyToLazyLoadedStockProfiles|TestStockProfileCatalogueMatchesDefaultFiles|TestStockIFMIBLinkMessagesDoNotDependOnIfOperStatus|TestStockProfileStoreLazyLoadsRoutedGzip' -count=1 -v`;
  - `TestStockProfileCatalogueMatchesDefaultFiles` confirms the committed
    `catalogue.json` matches the stock profile files and their trap counts.
- Full post-review package validation passes:
  - `go test ./plugin/go.d/collector/snmp_traps -count=1`;
  - `go test ./cmd/snmptrapprofilegen -count=1`;
  - `go vet ./plugin/go.d/collector/snmp_traps/... ./cmd/snmptrapprofilegen`.
- `git diff --check` passes.
- Fresh `/tmp` CMake configure passes with `ENABLE_PLUGIN_GO=On`.
- Fresh `/tmp` CMake configure passes with `ENABLE_PLUGIN_GO=ON` and
  `ENABLE_PLUGIN_XENSTAT=OFF`; the first configure attempt with default local
  options stopped on a pre-existing missing `xenstat` dependency before
  generation.
- Generated `cmake_install.cmake` contains compression globs for both
  `*.yaml` and `*.yml` under
  `usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/default/`.
- Targeted CMake install-code simulation confirms staged stock `standard.yaml`
  is compressed to `standard.yaml.gz` and the raw staged file is removed.
- Pending: external review of the complete SOW batch after this post-review
  hardening commit.

Real-use evidence:

- Pending.

Reviewer findings:

- Three external review passes were checked for the full-corpus/lazy-load
  changes.
- Accepted and fixed:
  - runtime accepted `.yml`/`.yml.gz` but CMake only installed/compressed
    `.yaml`;
  - stock catalogue/file drift lacked a direct test;
  - lazy-loaded stock profile overrides lacked a direct test;
  - IF-MIB link trap test did not prove the missing-varbind fallback case.
- Accepted as deliberate requirements trade-off:
  - route-table construction validates stock files at job creation, even though
    this creates startup CPU and temporary allocation cost, because trap-profile
    package failures must be visible before the job is reported as started.
- Not changed in this SOW:
  - large-vendor first-lookup latency is a known consequence of vendor-level
    lazy loading and remains a performance measurement item unless live testing
    shows it causes packet loss;
  - gzip being required by CMake is consistent with the accepted requirement
    that installed stock profiles are compressed.

Same-failure scan:

- Pending.

Sensitive data gate:

- Current SOW uses public repository names, commits, and sanitized count
  summaries only. No secrets, SNMP communities, customer data, private
  endpoints, or real trap payloads are recorded.

## Artifact Maintenance Gate

- AGENTS.md: pending; likely no update needed.
- Runtime project skills: updated
  `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` for the
  installed gzip requirement and runtime `.yaml.gz` support.
- Specs: updated `.agents/sow/specs/snmp-traps/netdata.md` for restricted
  template syntax, lazy stock-profile loading, installed gzip files, and
  `profile_load_failed` semantics.
- End-user/operator docs: updated `metadata.yaml`, generated integration
  markdown, and `profile-format.md` for installed gzip files and lazy stock
  loading.
- End-user/operator skills: pending; likely no update.
- SOW lifecycle: active branch-local SOW; transfer durable knowledge before
  merge and delete this SOW before final merge.

Specs update:

- `.agents/sow/specs/snmp-traps/netdata.md` updated.

Project skills update:

- `.agents/skills/project-snmp-trap-profiles-authoring/SKILL.md` updated.

End-user/operator docs update:

- `src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`
  updated.
- `src/go/plugin/go.d/collector/snmp_traps/metadata.yaml` updated.
- `src/go/plugin/go.d/collector/snmp_traps/integrations/snmp_trap_listener.md`
  updated.

End-user/operator skills update:

- Pending.

Lessons:

- Pending.

Follow-up mapping:

- Pending.

## Outcome

Pending.

## Lessons Extracted

Pending.

## Follow-up Issues

None yet.
