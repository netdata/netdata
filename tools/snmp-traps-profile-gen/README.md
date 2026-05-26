# snmp-traps-profile-gen

Legacy Python pipeline that turned the locally mirrored MIB corpus into
per-vendor SNMP trap profile YAML files for the stock out-of-the-box catalogue
for the Netdata SNMP trap subsystem.

The current shipped helper is the Go command in
`src/go/cmd/snmptrapprofilegen/`. CMake installs it as
`/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen` together with Netdata.
Use that helper for new stock regenerations and for operator MIB conversion.
The Python scripts in this directory are retained as historical/reference
tooling and for the bundled IANA PEN snapshot.

The legacy pipeline was driven by the two SOWs at
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
  extract.py                    # Phase 1 - SOW-0033, mechanical MIB extraction
  classify.py                   # Phase 2 - SOW-0034, LLM enrichment (dual-endpoint pool)
  emit.py                       # Phase 3 - per-vendor YAML output
  iana_pens.py                  # IANA PEN registry parser used by emit.py
  iana-enterprise-numbers.txt   # bundled IANA registry snapshot (~5 MB)
  sample_review.py              # builds output/sample-review.md from a 200-trap sample
  README.md                     # this file
  requirements.txt              # pysmi, httpx, PyYAML
  .gitignore                    # excludes output/, __pycache__/
  output/                       # all generated artefacts (gitignored)
```

The active helper lives outside this directory:

```
src/go/cmd/snmptrapprofilegen/
  main.go                       # extract, classify, emit, generate
  main_test.go                  # focused helper tests
```

## Current shipped helper

Operator conversion of local MIBs:

```sh
/usr/libexec/netdata/plugins.d/snmp-trap-profile-gen generate \
  --source-dir ./mibs \
  --all \
  --out-dir ./snmp-trap-profile-gen-output
```

The generated per-vendor YAML files land under
`./snmp-trap-profile-gen-output/profiles/`. Copy the needed files into
`/etc/netdata/go.d/snmp.trap-profiles/`.
The helper also writes review artifacts under `--out-dir`: `traps.jsonl`,
`extraction-report.json`, `conflicts.json` for duplicate trap OIDs, and
`source-conflicts.json` when multiple MIB files define the same module name.

Stock regeneration from the source tree:

```sh
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

The helper uses the bundled IANA PEN snapshot by default. In installed Netdata
that file is
`/usr/lib/netdata/conf.d/go.d/snmp.trap-profiles/iana-enterprise-numbers.txt`.
Pass `--refresh-pen` when the run should fetch the current IANA registry before
emission.

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

## Legacy Python setup

Create a local virtual environment from `requirements.txt`:

```sh
python3 -m venv .local/snmp-traps-profile-gen-venv
. .local/snmp-traps-profile-gen-venv/bin/activate
python3 -m pip install -r tools/snmp-traps-profile-gen/requirements.txt
```

The classifier talks to OpenAI-compatible chat-completions endpoints. Pass
endpoints explicitly with `--endpoint`, or set the neutral single-endpoint
defaults with `SNMP_TRAP_PROFILE_GEN_LLM_BASE_URL`,
`SNMP_TRAP_PROFILE_GEN_MODEL`, and `SNMP_TRAP_PROFILE_GEN_CONCURRENCY`.
The helper does not write API keys, endpoint credentials, or response headers
to output files.

## Legacy Phase 1 - extract MIBs

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

## Legacy Phase 2 - classify (sample gate first)

```sh
# 200-trap stratified sample.  Stop and review output/sample-review.md
# before running the full batch.
./classify.py --sample 200 --out-dir output \
  --endpoint 'http://127.0.0.1:8000/v1|local-model|8|800'

# After review, full batch (same --endpoint flags).
./classify.py --out-dir output \
  --endpoint 'http://127.0.0.1:8000/v1|local-model|8|800'
```

`classify.py` is async + concurrent. Dispatches across one or more LLM
endpoints in parallel; workers per endpoint pull from a shared queue so
the faster endpoint absorbs more work. Spec format per `--endpoint`:
`base_url|model|concurrency|max_tokens` (last field optional, defaults
800). Without `--endpoint` it falls back to single-endpoint defaults
(`--base-url`, `--model`, `--concurrency`).

For Qwen "thinker" variants, the helper auto-injects
`chat_template_kwargs.enable_thinking=false` so per-request wall-clock
stays low (without this, the reasoning chain blows the budget).

Each trap is written atomically to `output/enriched/<OID>.json` as soon
as it completes. Re-running skips OIDs that already have output files;
`--force` overrides.

Up to 3 LLM attempts per trap with the validator's reason fed back into
the retry prompt. If all 3 attempts fail validation the trap is filled
in via a deterministic regex-based fallback (`enrichment_source:
fallback:mechanical`).

## Legacy Phase 3 - emit per-vendor YAML

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
