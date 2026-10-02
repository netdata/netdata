# Preview a collector page without replacing checkout pages

Use this for collector prose inspection during authorized metadata changes or an explicitly requested preview.
For flows and other integration types, use [Non-collector pages](#non-collector-pages) below.
Read-only review uses the supplied artifacts and source; loading a content skill does not authorize regeneration.
This recipe refreshes the two ignored catalogs, then runs the actual page generator in a fresh private scratch
working directory. It never links source directories into scratch or restores checkout files.

## Current inputs and dependencies

Use the Python environment from `integrations/README.md` and run from the source checkout root. The catalog generator
reads current filesystem metadata, including modified and new files; it writes `integrations/integrations.js` and
`integrations/integrations.json`. Require success before using either: the generator can write them before reporting
fatal warnings. Existing ignored catalog contents will be replaced.

For producer-generated metadata (ibm.d or the NPM catalog), first ensure the producer outputs represent the current
inputs. Follow `../ibm-d.md` or `npm-catalog-generation.md`. To preserve a checkout during the whole producer chain,
use a fresh regular-file source copy containing the current modified and new inputs, with no symlinks escaping it,
and run producers, catalog generation, and this preview there. A copy of HEAD alone may miss the change. Sending
ibm.d docgen output to scratch does not make the source-checkout catalog generator read it automatically.

## Render selected prose

Replace `go.d.plugin/nginx` with the collector key (`plugin_name/module_name`, not necessarily its directory name).
Run with the dependency-equipped interpreter. Each invocation keeps its scratch output for inspection and prints its
location; no cleanup or restore is required.

```bash
python3 - go.d.plugin/nginx <<'PY'
from pathlib import Path
import json
import shlex
import shutil
import subprocess
import sys
import tempfile

repo = Path.cwd().resolve()
key = sys.argv[1]
sys.path.insert(0, str(repo / "integrations"))
import gen_docs_integrations as docs
from gen_doc_collector_page import get_integration_description

def run(args, cwd):
    print(f"Running in {cwd}: {shlex.join(map(str, args))}", file=sys.stderr)
    result = subprocess.run(args, cwd=cwd)
    if result.returncode:
        print(f"Command failed in {cwd}: status {result.returncode}", file=sys.stderr)
        raise SystemExit(result.returncode)

run([sys.executable, repo / "integrations/gen_integrations.py"], repo)
catalog = repo / "integrations/integrations.js"
_, records = docs.read_integrations_js(str(catalog))
bases = docs._base_paths_for_collector(records, key)
if not bases:
    raise SystemExit(f"No collector records match {key!r}")

scratch = Path(tempfile.mkdtemp(prefix="netdata-page-preview-"))
print(f"Preview directory: {scratch}", flush=True)
(scratch / "integrations").mkdir()
shutil.copyfile(catalog, scratch / "integrations/integrations.js")
for base in set(bases):
    relative = Path(base)
    if relative.is_absolute() or ".." in relative.parts or not relative.parts:
        raise SystemExit(f"Invalid collector base path: {base!r}")
    (scratch / relative).mkdir(parents=True, exist_ok=True)
run([sys.executable, repo / "integrations/gen_docs_integrations.py", "-c", key], scratch)
pages = sorted(scratch.glob("**/integrations/*.md"))
if not pages:
    raise SystemExit("Generator returned without producing any collector pages")
for page in pages:
    print(page)
sentences = [{"id": record["id"], "description": get_integration_description(record)}
             for record in docs._select_integrations(records, key)]
summary = scratch / "catalog-descriptions.json"
summary.write_text(json.dumps(sentences, indent=2) + "\n", encoding="utf-8")
print(summary)
PY
```

Read every selected page as an operator, including its frontmatter. Inspect the separate `catalog-descriptions.json`
produced by the catalog sentence extractor, using
`../description-authoring.md`: catalog and page meta descriptions have different precedence. Generated pages and
README files remain outputs; fix their authoritative inputs and run a fresh preview. Delivery follows
`../consistency.md`.

## Non-collector pages

The `-c plugin/module` selector matches only `integration_type: collector`; flows and other integration types do
not match it even when their metadata has plugin and module names. Prepare a fresh regular-file source copy with
current modified and intentional new inputs, and no symlinks escaping it, as described under
[Current inputs and dependencies](#current-inputs-and-dependencies). Run applicable producers there first.

From that isolated repository root, use the dependency-equipped Python from `integrations/README.md`:

```bash
# Run both commands in the prepared isolated source copy.
python3 integrations/gen_integrations.py
python3 integrations/gen_docs_integrations.py
```

This generates the full page corpus in the copy; inspect the relevant page and its catalog sentence there.
`--check` validates descriptions without rendering pages. Full umbrella rendering also needs the later stages in
`../pipeline.md`. Generated pages and README symlinks remain local preview outputs under `../consistency.md`.

## What this validates

The existing CLI validates descriptions for the full catalog, then renders selected records with the normal banner,
Markdown cleanup, related-link pass, and README handling. An unrelated invalid description can therefore block it.
Creating every selected base directory matters: the generator silently skips missing bases.

A scoped render omits links to unselected integration records and may choose a README symlink differently from a full
render of a shared collector directory. It proves selected generated prose, not the complete link graph, final README
ownership, umbrella-table rendering, Learn ingestion, or browser layout. For those requirements, use the complete
pipeline in an isolated source copy (`../pipeline.md`), or the explicitly requested Learn preview workflow
(`.agents/skills/docs-learn-pr-preview/SKILL.md`).

The confinement depends on the current generator's paths: `read_integrations_js`, `cleanup`, `write_to_file`, and
`make_symlinks` in `integrations/gen_docs_integrations.py` operate relative to its working directory. Re-check that
contract if changing the generator. `gen_integrations.py` instead anchors its catalog outputs to the source checkout;
merely changing its working directory does not isolate those writes.
