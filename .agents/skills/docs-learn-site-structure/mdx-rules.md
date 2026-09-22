# MDX rules

Learn renders `.mdx` with MDX 3, so markdown that is harmless on GitHub can fail the Docusaurus build. `sanitize_page`
in `ingest/ingest.py` rewrites every published file; what it does not cover is the author's job. Author-side rules for
collector metadata: `.agents/skills/collectors-metadata-yaml/SKILL.md#safety-of-the-markdown`. Verified against
`netdata/learn @ c3a16edd5ee4dc819976ef162c9afaff4b9b968c`.

## What `sanitize_page` does, in its order

1. The first `<!--` and the first `-->` become `---` (the injected metadata block becomes frontmatter).
2. `<!--unhideme` and `unhideme-->` markers are removed.
3. `<details><summary>` and `<details open><summary>` get a newline between the tags; no other variant is touched.
4. `_escape_mdx_braces` escapes every `{` not already preceded by a backslash, after setting aside fenced code
   blocks, inline code, `import` lines, and `export` lines that start an ESM form (`default`, `function`, `const`,
   `let`, `var`, `{`); `style={{` is restored afterwards. A shell `export VAR=...` is not an ESM form, so it is only
   safe while it contains no `{`. Nothing escapes `}` or `<`.
5. Integration pages (`INTEGRATION_MARKER`) get their `netdata.cloud/img` logo tags annotated with contrast data
   attributes (`_annotate_integration_logo_tags`, one HTTP fetch per logo URL per run, `LOGO_ANALYSIS_TIMEOUT`); a
   failed fetch still writes the attributes, with `unknown` contrast and `low` confidence.
6. The exact substrings `<=`, `%<`, and `<->` are backslash-escaped. `< =` or `<-->` are not.
7. `<https://...>`, `<http://...>`, and `<user@host>` become markdown links.
8. A `meta_yaml: "<url>"` line anywhere in the file is removed and `custom_edit_url` is rewritten to that URL.
9. Lines starting with `[![analytics]` are dropped, and blank lines around the frontmatter are normalized.

## What breaks and how to write it

Not covered by the transforms, each fails the MDX build:

- `<word>` placeholders in prose (`<service-name>`, `<scope>`), which MDX reads as an unclosed tag (the build reports
  `Expected a closing tag for <word>`);
- `<` directly followed by a digit (`<100 minutes`; reported as `Unexpected character ... before name`);
- generic type syntax (`Vec<u32>`, `List<String>`);
- any HTML tag in body text that is not meant as JSX;
- a standalone `}` outside code (nothing escapes it), and operator spellings other than the three exact substrings of
  step 6, such as `<-->` or `< =`.

Fixes, in order of preference: wrap the token in inline code (step 4 preserves it); rephrase (`under 100 minutes`);
escape as `\<` only when the character must read as a less-than sign. Fenced and inline code, MDX `import`/`export`
at the top of the file, `style={{ }}`, and fenced Mermaid blocks (`markdown.mermaid` is on in
`docusaurus.config.js`; `fix_mermaid_diagram_contrast` rewrites low-contrast fills) survive as written.

## Tests and gates

- `test_escape_mdx_braces.py` at the learn root defines its own copy of `_escape_mdx_braces` and is run by nothing;
  it does not protect the live function. The ingest test suite is `ingest/test_*.py`.
- `docusaurus.config.js` sets `onBrokenLinks: 'warn'` and `markdown.hooks.onBrokenMarkdownLinks: 'warn'`, so the
  build does not fail on a broken link. The gates that do are ingest's own link and anchor validation under a
  `--fail-links*` flag (this repository's `.github/workflows/check-markdown.yml`), the daily `learn_link` sweep, and
  the learn `rendered-link-integrity.yml` job (`./pipeline.md#when-ingest-runs`).
