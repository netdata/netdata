# snmp-traps-profile-gen

Three-stage pipeline that turns the locally mirrored MIB corpus into
per-vendor SNMP trap profile YAML files, ready to ship as the stock
out-of-the-box catalogue for the Netdata SNMP trap subsystem.

The pipeline is driven by the two SOWs at
`.agents/sow/done/SOW-0033-*` (mechanical MIB extraction) and
`.agents/sow/done/SOW-0034-*` (LLM classification + description
generation).  Read those first for the design rationale.

The schema authority for the YAML it produces is
`src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.
The trap subsystem design referencing it is
`.agents/sow/specs/snmp-traps/netdata.md`.

## Layout

```
tools/snmp-traps-profile-gen/
  extract.py     # Phase 1 - SOW-0033, mechanical MIB extraction
  classify.py    # Phase 2 - SOW-0034, LLM enrichment
  emit.py        # Phase 3 - per-vendor YAML output
  requirements.txt
  output/        # all generated artefacts (gitignored)
```

The expected output sub-tree under `output/`:

```
output/
  extracted.jsonl           # one line per trap (extract.py)
  extraction-report.json    # counts + dirs scanned
  failed-mibs.json          # per-MIB compile failures
  dedup-conflicts.json      # OIDs found in more than one MIB module
  enriched/<OID>.json       # one file per trap (classify.py)
  llm-failures.json         # validation failures + reasons
  enrichment-report.json    # counts of primary/fallback model/mech
  sample-review.md          # human review document for the 200-trap gate
  profiles/<vendor>.yaml    # final per-vendor profile YAML (emit.py)
  profile-emit-report.json  # vendor -> entry-count map
```

## Setup

Use the existing trial venv at
`/home/costa/src/PRs/snmptraps/.local/snmp-extract-trial/venv/` (already
has `pysmi 2.0.0`, `httpx`, `PyYAML`).  Or create a new venv from
`requirements.txt`.

The classifier reads the LLM API key from
`~/.local/share/opencode/auth.json` (JSON key `llm-netdata-cloud.key`).
The key is **never** written to logs or output files.

## Phase 1 - extract MIBs

```sh
# Test on a few MIBs first.
./extract.py --mibs IF-MIB SNMPv2-MIB BGP4-MIB CISCO-CONFIG-MAN-MIB \
             --out-dir output

# Full corpus.
./extract.py --all --out-dir output --log-level INFO --progress-every 200
```

`extract.py` walks every configured source directory in priority order,
deduplicates MIB module names (first occurrence wins), then compiles each
module via pysmi.  For every successfully-compiled module it iterates
symbols of class `notificationtype` / `traptype` and resolves their
`OBJECTS` lists against the global symbol table built from **all**
compiled MIBs (cross-module varbind resolution).

Resumability: re-run with `--resume` to append-only; already-extracted
MIB names are skipped.

## Phase 2 - classify (sample gate first)

```sh
# 200-trap stratified sample.  Stop and review output/sample-review.md
# before running the full batch.
./classify.py --sample 200 --out-dir output

# After review, full batch.
./classify.py --out-dir output
```

`classify.py` is async + concurrent.  Defaults: 48 in-flight against the
primary model (`minimax-m2.7-coder`), 6 against the fallback
(`qwen3.6-35b-a3b-nothinker`).  Each trap is written atomically to
`output/enriched/<OID>.json` as soon as it completes.  Re-running skips
OIDs that already have output files; `--force` overrides.

If the primary model returns an invalid response (JSON parse failure,
banned phrase, unknown placeholder, …) the fallback model is tried once;
if it also fails, the trap is filled in via a deterministic regex-based
fallback (`enrichment_source: fallback:mechanical`).

## Phase 3 - emit per-vendor YAML

```sh
./emit.py \
  --in-dir output/enriched \
  --out-dir ../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default \
  --catalogue ../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json
```

`emit.py` groups enriched records by vendor (OID enterprise-prefix
lookup) and emits one YAML file per vendor matching the schema in
`src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md`.
Standard IETF/IANA OIDs go to `standard.yaml`.  IEEE 802.1AB (LLDP)
OIDs go to `ieee-lldp.yaml`.  Enterprise OIDs whose enterprise number
is in `ENTERPRISE_VENDORS` get the named slug; unknown enterprise
numbers are bucketed under `enterprise-<N>.yaml` so nothing is silently
dropped.

Each vendor file has a file-scoped `varbinds:` table (name-keyed,
deduped across the vendor's traps); each trap entry references
varbinds by name.  This keeps the on-disk and loaded-memory footprint
linear in the number of distinct varbinds per vendor rather than in the
number of traps that use them.

The catalogue (`catalogue.json`) is the operator's grep-before-install
index: `{vendor: {file, trap_count, varbind_count, mib_count, mibs[],
sample_traps[]}}`.  Ships alongside the YAML files.

The shipped pack lands directly under
`src/go/plugin/go.d/config/go.d/snmp.trap-profiles/` — not under
`output/` — so a regeneration produces a `git diff` against the
previously committed pack that reviewers can read.  The plugin loads
these profiles only when the trap subsystem is enabled.

## Sample-quality gate

Before committing to the multi-hour full batch:

1. Run `classify.py --sample 200`.
2. Open `output/sample-review.md`.  Each section shows the input
   description, varbind names, and the LLM-produced category / severity /
   description template.
3. Eyeball ~20-30% of the sample.  Look for:
   - Wrong category (e.g., a config change tagged as `state_change`).
   - Description templates referencing varbinds not in the input.
   - Marketing-tone language ("powerful", "world-class", "critical
     issue").
4. If the sample looks acceptable, run the full batch.
