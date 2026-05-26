# Auditing `metadata.yaml` links to Learn

Use this when a generated integration page contains links to
`learn.netdata.cloud` and one of them drifts from the current Learn route.

## Why this matters

`metadata.yaml` is rendered into integration pages on Learn, the website, and
the in-app integrations catalog (`../SKILL.md:37`). A broken Learn URL in
metadata therefore becomes a user-visible broken link on multiple surfaces.

Learn routes are not derived from source filenames. They are derived from
`docs/.map/map.yaml` labels and hierarchy, while source-relative `/docs/... .md`
links can be rewritten by Learn ingest (`../../learn-site-structure/mapping.md:219`).
For example, `docs/network-flows/visualization/summary-sankey.md` is published
as `/docs/network-flows/visualization/sankey-and-table` because the map label is
`Sankey and Table` (`docs/.map/map.yaml:515`).

## Audit command

Extract unique absolute Learn URLs from all metadata files and check their
published response:

```bash
rg -No "https://learn\\.netdata\\.cloud/docs[^)\\]\\s,\"']+" --glob 'metadata.yaml' . \
  | sed 's/^.*https:/https:/' \
  | sort -u \
  | while read -r url; do
      printf '%s\t' "$url"
      curl -sL -o /dev/null -w '%{http_code}\t%{url_effective}\n' "$url"
    done
```

For URLs with fragments, also confirm the target anchor exists in the rendered
HTML:

```bash
curl -A 'Mozilla/5.0' -sL 'https://learn.netdata.cloud/docs/netdata-agent/configuration' \
  | rg 'id="locate-your-config-directory"'
```

Validate source-relative metadata links locally:

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

- If the link is Markdown text and points to a Netdata source doc, prefer the
  source-relative `/docs/... .md` form when the consuming surface supports Learn
  ingest rewriting.
- If the same metadata is consumed by non-Learn surfaces that do not rewrite
  source-relative links, keep an absolute `https://learn.netdata.cloud/docs/...`
  URL, but derive the slug from `docs/.map/map.yaml` labels and verify it with
  `curl`.
- Do not infer slugs from filenames. Check the map label first, then validate
  the published URL.

## How I figured this out

Files read:

- `../SKILL.md`
- `../../learn-site-structure/mapping.md`
- `docs/.map/map.yaml`

Commands run:

- `rg -No 'https://learn\.netdata\.cloud/docs...' --glob 'metadata.yaml' .`
- `curl -sL -o /dev/null -w '%{http_code}\t%{url_effective}\n' <url>`
- local source-relative metadata link validation script above.
