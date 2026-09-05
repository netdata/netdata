# MDX rules

Docusaurus uses MDX 3 for `.mdx` files. MDX is markdown plus
JSX, which means certain markdown text that is harmless in
plain GitHub-rendered markdown will break MDX parsing. Ingest
runs a battery of escape transformations to make source `.md`
content survive the conversion to `.mdx`.

Live in `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py:1721-1799`
(the `_escape_mdx_braces` function and adjacent transforms),
exhaustively tested in
`${NETDATA_REPOS_DIR}/learn/test_escape_mdx_braces.py`. Every
transform is applied to every published file via
`sanitize_page` (`ingest.py:1765-1829`), in this order:

## 1. Frontmatter conversion

- First `<!--` -> `---`, first `-->` -> `---` (`ingest.py:1779-1780`).
- Turns the injected hidden metadata block into real YAML
  frontmatter.

## 2. Strip unhideme markers

`<!--unhideme` and `unhideme-->` markers are stripped
(`ingest.py:1783-1784`).

## 3. `<details><summary>` newline fix

`<details><summary>` -> `<details>\n<summary>` (and the
`<details open>` variant) for MDX 3 compatibility
(`ingest.py:1787-1788`). Without this fix, the inline form
breaks the parser.

**Caveat**: only the two literal forms it knows are fixed. Any
other variant (e.g. `<details class="x"><summary>`) breaks
MDX rendering silently.

## 4. `_escape_mdx_braces`

Escape every bare `{` outside fenced/inline code:

- **Preserve** fenced ```` ``` ... ``` ``` blocks (DOTALL
  match).
- **Preserve** inline `` `...` `` (no newlines, no nested
  backticks).
- **Preserve** `^import ...$` lines (MDX ESM destructuring
  syntax).
- **Preserve** `^export (default|function|const|let|var|{) ...$`
  lines (regex
  `export\s+(?:default|function|const|let|var|\{)`).
- Replace every bare `{` (not already preceded by `\`) with
  `\{`.
- Restore `style=\{\{` back to `style={{` because that's valid
  JSX.

**Bash `export VAR=...` lines are NOT preserved as ESM**. The
`export` regex requires `default`, `function`, `const`, `let`,
`var`, or `{` after `export`. Bash-style `export NETDATA_FOO=bar`
passes through escape unchanged because it has no `{`. If a
bash export contained `{`, it would get escaped (probably what
you want).

## 5. Specific operator escapes

- `<=` -> `\<=`
- `%<` -> `%\<`
- `<->` -> `\<->`

(`ingest.py:1792-1794`). MDX would otherwise try to parse
these as JSX tags. Note these are **exact-substring** rules;
near-variants like `< =` (with space) or `<---->` (multi-dash)
are NOT covered. See `pitfalls-and-gotchas.md`.

### Patterns that the escape battery does NOT cover

- **`<word>` placeholders in prose** (e.g. `<service-name>`,
  `<scope>`, `<app>`). MDX 3 parses anything that lexically
  looks like an open tag and then expects a matching close tag.
  Without one, the build fails with
  `Expected a closing tag for \`<word>\` ... before the end of \`paragraph\``.
- **`<` followed by a digit** (e.g. `<100 minutes`,
  `<5 seconds`). MDX rejects this with
  `Unexpected character '1' (U+0031) before name, expected a
  character that can start a name, such as a letter, $, or _`.
  This pattern is common in lists describing thresholds.
- **`Type<param>` Rust/C++/Java generic syntax** (e.g.
  `Vec<u32>`, `HashMap<String, Vec<u8>>`, `unique_ptr<T>`).
  Same JSX-tag issue.

The safe options, in order of preference:

1. **Wrap in inline code with backticks** -- rule 4
   preserves inline code, so `\`<service-name>\``,
   `\`Vec<u32>\``, `\`<APP>\`` all survive intact. This is
   the standard fix for our Netdata integration content.
2. **Rephrase the sentence** -- e.g. `< 100 minutes`
   becomes `under 100 minutes`. Often clearer than the
   original anyway.
3. **Backslash-escape the `<`** -- `\<word>`. Works but
   uglier than backticks. Use only when the `<` must remain
   visibly a less-than operator, not a placeholder.

These three were exercised in the netflow-plugin docs
(2026-05-07 ingest preview deploy failure):
`docs/network-flows/retention-querying.md` had `<100 minutes`,
fixed by rephrasing; `aws_ip_ranges.md`, `gcp_ip_ranges.md`,
and `generic_json-over-http_ipam.md` (generated from
`metadata.yaml`) had `<service-name>`, `<scope>`, `<app>`,
fixed by wrapping the placeholders in backticks at the
`metadata.yaml` source.

## 6. Bare URL angle-bracket links

Converted to markdown links (`ingest.py:1797-1799`):

- `<https://...>` -> `[https://...](https://...)`
- `<http://...>` -> `[http://...](http://...)`
- `<email@x.y>` -> `[email@x.y](mailto:email@x.y)`

So `<...>` used as a "I want this rendered as a link" markdown
shortcut works correctly through ingest.

## 7. `meta_yaml` rewrite

If `meta_yaml: "<url>"` is present in the file, the file is
treated as an integration. `meta_yaml:` is removed and
`custom_edit_url` is rewritten to that URL
(`ingest.py:1801-1808`). Silent rewrite -- any file with that
key triggers it.

## 8. Integration logo annotation

Integration files (`INTEGRATION_MARKER` present) get
`<img src="https://(www.)?netdata.cloud/img/...">` annotated
with `data-integration-logo`, `data-logo-contrast-light`,
`data-logo-contrast-dark`, `data-logo-contrast-confidence`
after a fetch+luminance analysis
(`_annotate_integration_logo_tags` and `_analyze_remote_logo`,
`ingest.py:1647-1718`). Used by `Grid_integrations` and the
dashboard theme to add a subtle glow on low-contrast logos.

## 9. Drop analytics pixel lines

Lines starting with `[![analytics]` are dropped
(`ingest.py:1816-1817`). These are leftover GitHub
README-style tracking pixels that don't make sense on Learn.

## What survives, what doesn't

The escape rules cover the most common breakage patterns:

| Pattern | Survives? | Why |
|---|---|---|
| `{word}` outside code | escaped to `\{word}` | rule 4 |
| `${expr}` | escaped to `\${expr}` | rule 4 |
| `style={{ }}` JSX | yes (un-escaped) | rule 4 restoration |
| Already-escaped braces | unchanged | rule 4 idempotent |
| Fenced or inline code | unchanged | rule 4 preservation |
| MDX `import`/`export` ESM | unchanged | rule 4 preservation |
| Tables with `{` cells | escaped per cell | rule 4 |
| `<=`, `%<`, `<->` exact | escaped | rule 5 |
| `< =` (with space) | NOT covered | rule 5 is exact-substring |
| `<---->` (long arrow) | NOT covered | rule 5 is exact-substring |
| `<htmltag>` body content | NOT escaped | breaks MDX unless wrapped in code |
| `<word>` placeholders in prose | NOT escaped | breaks MDX; wrap in backticks or rephrase |
| `<` + digit (`<100`, `<5s`) | NOT escaped | breaks MDX; rephrase as "under N" or escape |
| `Vec<u32>`, `HashMap<K,V>` generics | NOT escaped | breaks MDX; wrap in backticks |
| `<details>` inline summary | fixed | rule 3 |
| `<details class="x">` | NOT fixed | rule 3 only knows two forms |
| `}` closing brace alone | NOT escaped | rule 4 only escapes `{` |
| Bare URL in `<...>` | converted to MD link | rule 6 |

## Test suite

`${NETDATA_REPOS_DIR}/learn/test_escape_mdx_braces.py:74-377`
exercises:

- simple `{word}`, `${expr}` templates;
- `{{double}}` braces;
- `style={{ }}` JSX (preserved);
- already-escaped braces (idempotent);
- fenced and inline code preservation (multiple code blocks
  per file, mixed inline and fenced);
- table rows with mixed bare and code-fenced braces;
- real Zabbix and Nagios integration content;
- MDX `import`/`export` ESM lines;
- bash `export` lines (currently pass through unchanged
  because they have no braces);
- empty input;
- only-`{}` content;
- nested `{outer{inner}}`;
- unclosed `{`;
- lone `}`;
- newline after `{`;
- the original hardcoded `{attribute_name}` patterns;
- a full-document simulation.

## Mermaid diagrams

Mermaid diagrams are enabled at the markdown level in
`docusaurus.config.js:26-30`. The escape rules preserve
fenced code blocks, so ` ```mermaid ... ``` ` blocks survive
intact.

## What you can put in source `.md` files safely

- Any plain markdown.
- Code blocks (fenced and inline) -- whatever's inside them is
  preserved.
- MDX import/export ESM at the top of file (works in `.mdx`,
  not in `.md`; ingest converts to `.mdx`).
- `style={{ ... }}` JSX (preserved).
- Mermaid in fenced ```` ```mermaid ``` ```.
- `<details>` and `<summary>` with newlines between them.

## What you should escape yourself in source

- `<htmltag>` in body content -- always wrap in code (fenced
  or inline).
- Multi-character operators like `<->` with extra dashes
  (`<-->`, `<--->`).
- `< ` or ` <` with spaces around the `<`.
- Closing-brace-only sequences if they're standalone.

## Onbroken-links policy

`docusaurus.config.js:22`: `onBrokenLinks: 'warn'`. Broken
links never fail the Docusaurus build at the parse level; they
only get caught by the ingest's pre-build link checker
(`--fail-links`) and the daily 404 sweep
(`daily-learn-link-check.yml`). So a broken link in your `.md`
will silently make it to production unless caught by one of
those two gates.
