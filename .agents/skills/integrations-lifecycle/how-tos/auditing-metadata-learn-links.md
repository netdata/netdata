# Auditing `metadata.yaml` links to Learn

Use this when a generated integration page links to `learn.netdata.cloud` and a link may have drifted from the current
Learn route. The rule and its evidence live in the Learn skill:
`.agents/skills/docs-learn-site-structure/how-tos/integration-card-description-links.md` (absolute Learn URLs in
metadata bypass ingest's link rewriting and anchor validation; Learn routes come from `docs/.map/map.yaml` labels, not
from source filenames, so never infer a slug from a filename). This file keeps the audit commands.

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

Validate source-relative metadata links (`/docs/...` and `../` targets) against the source tree:

```bash
python3 - <<'PY'
import pathlib, re, sys

root = pathlib.Path('.')
pat = re.compile(r'\[[^\]]+\]\(([^)]+)\)')
problems = []

for path in sorted(root.rglob('metadata.yaml')):
    text = path.read_text(errors='replace')
    for match in pat.finditer(text):
        target = match.group(1).strip()
        if target.startswith('/docs/'):
            file = root / target.split('#', 1)[0].lstrip('/')
        elif target.startswith('../') or target.startswith('./'):
            file = (path.parent / target.split('#', 1)[0]).resolve()
        else:
            continue

        if not file.is_file():
            line = text.count('\n', 0, match.start()) + 1
            problems.append((str(path), line, target))

if problems:
    for path, line, target in problems:
        print(f'{path}:{line}: missing linked source file: {target}')
    sys.exit(1)

print('OK: all metadata.yaml /docs and relative markdown links resolve to source files')
PY
```

## Repair rule

Prefer the source-relative `/docs/... .md` form when the consuming surface supports Learn ingest rewriting. Keep an
absolute URL only for surfaces that do not rewrite, derive its slug from the `map.yaml` label, and verify it with `curl`
as above.
