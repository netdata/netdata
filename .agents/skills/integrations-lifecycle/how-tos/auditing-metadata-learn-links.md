# Auditing `metadata.yaml` links to Learn

Use this when a generated integration page links to `learn.netdata.cloud` and a link may have drifted from the current
Learn route. The rule and its evidence live in the Learn skill:
`.agents/skills/docs-learn-site-structure/how-tos/integration-card-description-links.md` (absolute Learn URLs in
metadata bypass ingest's link rewriting; Learn routes come from `docs/.map/map.yaml` labels, not from source
filenames, so never infer a slug from a filename). This file keeps the audit commands.

## Audit command

Extract unique absolute Learn URLs from all metadata files and check their published response:

```bash
rg -No "https://learn\\.netdata\\.cloud/docs[^)\\]\\s,\"']+" --glob 'metadata.yaml' . \
  | sed 's/^.*https:/https:/' \
  | sort -u \
  | while read -r url; do
      printf '%s\t' "$url"
      curl -sL -o /dev/null -w '%{http_code}\t%{url_effective}\n' "$url"
    done
```

For URLs with fragments, also confirm the target anchor exists in the rendered HTML:

```bash
curl -A 'Mozilla/5.0' -sL 'https://learn.netdata.cloud/docs/netdata-agent/configuration' \
  | rg 'id="locate-your-config-directory"'
```

Validate repository-relative metadata links (`/...`, `./` and `../` targets) against the source tree. The script checks
that each file exists, not its anchors; ingest checks those
(`.agents/skills/docs-learn-site-structure/mapping.md#links-between-pages`):

```bash
python3 - <<'PY'
import pathlib, re, sys

root = pathlib.Path('.').resolve()
pat = re.compile(r'\[[^\]]+\]\(([^)]+)\)')
problems = []

for path in sorted(root.rglob('metadata.yaml')):
    text = path.read_text(errors='replace')
    for match in pat.finditer(text):
        target = match.group(1).strip()
        if target.startswith('/') and not target.startswith('//'):
            file = (root / target.split('#', 1)[0].lstrip('/')).resolve()
        elif target.startswith('../') or target.startswith('./'):
            file = (path.parent / target.split('#', 1)[0]).resolve()
        else:
            continue

        if not file.is_relative_to(root) or not file.is_file():
            line = text.count('\n', 0, match.start()) + 1
            problems.append((str(path.relative_to(root)), line, target))

if problems:
    for path, line, target in problems:
        print(f'{path}:{line}: linked file missing or outside the repository: {target}')
    sys.exit(1)

print('OK: all repository-relative metadata.yaml links resolve to source files')
PY
```

## Repair rule

Replace each absolute Learn URL with the repository-relative `.md` path of the target page's source file. Keep an
anchor only when it is the slug of the target heading's text: ingest checks repository-relative anchors differently
from absolute Learn URLs (`.agents/skills/docs-learn-site-structure/mapping.md#links-between-pages`). The rule, and
how the generator and ingest resolve that form, are in the Learn skill how-to named above.
