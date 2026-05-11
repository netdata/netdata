# Keep Network Flows on Monitor Anything

**One-line summary:** `src/collectors/COLLECTORS.md` is the generated Learn "Monitor anything with Netdata" page; Network Flows entries appear there only when `integrations/gen_doc_collector_page.py` treats the top-level `flows` category as a section.

## What Updates The Page

`src/collectors/COLLECTORS.md` is generated, not hand-authored. The local and CI command is:

```bash
python3 integrations/gen_integrations.py
python3 integrations/gen_doc_collector_page.py
```

`gen_integrations.py` produces the runtime
`integrations/integrations.js` catalog. `gen_doc_collector_page.py`
then reads that catalog and atomically replaces
`src/collectors/COLLECTORS.md`. The file is committed because it is
the source document that Learn ingests for the "Monitor anything with
Netdata" page.

The CI workflow that checks documentation PRs also runs this path before Learn ingest:

- `.github/workflows/check-markdown.yml` runs `gen_integrations.py`, `gen_docs_integrations.py`, `gen_doc_collector_page.py`, and `gen_doc_secrets_page.py` before `learn/ingest/ingest.py`.
- `.github/workflows/generate-integrations.yml` runs the same generator family after metadata changes land on `master` and opens a regeneration PR.

## Why Flows Need Explicit Handling

Most Monitor Anything sections are children of `data-collection` in `integrations/categories.yaml`. Network Flows is different:

- `integrations/categories.yaml` defines top-level `flows` with children `flows.sources` and `flows.enrichment-methods`.
- `src/crates/netflow-plugin/metadata.yaml` uses those categories for NetFlow / IPFIX / sFlow and flow enrichment entries.
- `integrations/gen_doc_collector_page.py` therefore must treat top-level `flows` as a section, otherwise those entries are not grouped as `Network Flows` on Monitor Anything.

## Validation

After changing flow metadata or category handling, run:

```bash
python3 integrations/gen_integrations.py
python3 integrations/gen_docs_integrations.py
python3 integrations/gen_doc_collector_page.py
rg -n '^### Network Flows|\\[NetFlow\\]|\\[Static Metadata\\]|\\[Decapsulation\\]' src/collectors/COLLECTORS.md
```

Expected result: `src/collectors/COLLECTORS.md` contains a `### Network Flows` section listing NetFlow, IPFIX, sFlow, and the enrichment integrations.

## How I Figured This Out

Read `integrations/categories.yaml`, `src/crates/netflow-plugin/metadata.yaml`, `integrations/gen_doc_collector_page.py`, `src/collectors/COLLECTORS.md`, `.github/workflows/check-markdown.yml`, and `.github/workflows/generate-integrations.yml`; regenerated `COLLECTORS.md` and checked for the `Network Flows` section.
