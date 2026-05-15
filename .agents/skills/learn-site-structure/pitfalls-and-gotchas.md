# Pitfalls and gotchas

Every silent failure mode, dead artifact, undocumented
behavior, and edge case the Learn ingest pipeline carries
today. Read this BEFORE assuming the pipeline does the obvious
thing.

## Hard failures (exit non-zero)

- **`map.yaml` schema validation** -- exit code 2
  (`ingest.py:2807, 2819`).
- **`--fail-links` plus broken internal links** -- exit 1 at
  the very end of the run.
- **Kickstart checksum step in CI** -- exit 1 if `wget` or the
  placeholder substitution fails.

## Soft failures / silent breakage

- **Duplicate `(learn_rel_path, sidebar_label)`** -- only a
  printed warning at `ingest.py:2891-2908`. On case-insensitive
  filesystems (macOS/Windows), one file silently overwrites
  the other. On Linux they coexist but collide in URL space.

- **Missing `sidebar_label` / `learn_rel_path` in `map.yaml`
  row** -- file is skipped with a printed `KeyError`;
  downstream sidebar may have a hole and links that pointed to
  it become broken anchors.

- **`sidebar_label` containing characters that the slug
  sanitizer collapses** (`'`, `:`, `/`, `(`, `)`, `,`,
  backtick, repeated whitespace) -- destination filename can
  collide with a sibling that differs only by punctuation.

- **MDX 3 syntax bombs** -- bare `{word}` outside code is
  escaped, but a single backtick line, smart quotes, or
  non-paired `<...>` can still break the MDX parser. Symptoms:
  "Unexpected character `}`", "Could not parse expression", or
  a Docusaurus warning that becomes a blocking error in
  `production` builds. The ingest only escapes `{`, not `}`,
  not `<`. Bare `<word>` in body still breaks unless wrapped
  in code.

- **Broken anchor links** -- markdown header anchors are
  recomputed by `extract_headers_from_file`
  (`ingest.py:531-560`); custom anchors via `<a id="...">` may
  not match this slugger and trigger anchor-mismatch warnings.

- **Missing assets / images referenced by absolute path under
  `/img`** -- Docusaurus `onBrokenLinks: 'warn'`
  (`docusaurus.config.js:22`) and
  `markdown.hooks.onBrokenMarkdownLinks: 'warn'` -- broken
  links **warn but do not fail the build**, so they slip into
  production.

- **`<details><summary>` without newline** -- the source-side
  fix at `ingest.py:1787-1788` only handles the two literal
  forms it knows. Any other variant (e.g. `<details class="x">`)
  breaks MDX rendering silently.

- **`<= %< <->` not in code blocks** -- escaped by
  `sanitize_page` only as those exact substrings; near-variants
  like `< =`, `<---->`, or `< -` aren't covered.

- **`<word>` placeholders in prose break MDX silently
  through ingest** -- patterns like `<service-name>`,
  `<scope>`, `<app>`, `<aws-region>` look like JSX open
  tags to MDX 3 and demand a closing tag. Build fails with
  `Expected a closing tag for \`<word>\` ... before the end of
  \`paragraph\``. The escape battery does NOT touch these.
  Fix: wrap in backticks (`\`<service-name>\``) or rephrase.
  Hit 2026-05-07 in netflow-plugin `metadata.yaml`
  descriptions for AWS / GCP / phpIPAM IPAM sources.

- **`<` immediately followed by a digit** -- patterns like
  `<100 minutes`, `<5 seconds`, `<10ms` make MDX try to parse
  a JSX tag and fail with `Unexpected character '1' (U+0031)
  before name, expected a character that can start a name,
  such as a letter, $, or _`. Common in threshold descriptions.
  Fix: rephrase as `under 100 minutes` or `&lt;100 minutes`.
  Hit 2026-05-07 in
  `docs/network-flows/retention-querying.md`.

- **Generic-type syntax `Type<Param>`** -- Rust `Vec<u32>`,
  C++ `unique_ptr<T>`, Java `List<String>`, etc. parse as
  unbalanced JSX opens. Always wrap in backticks when
  documenting code types.

- **`meta_yaml: "<url>"` rewrite** -- silently rewrites
  `custom_edit_url` for any file containing `meta_yaml: "..."`,
  even if the author didn't intend it
  (`ingest.py:1801-1808`).

- **Frontmatter `slug:` overrides** -- frontmatter `slug:` is
  taken at face value (`ingest.py:1892`); a space in a slug
  breaks every link to that page. Not validated.

- **Auto-grid file overwrite risk**:
  `get_dir_make_file_and_recurse` will NOT overwrite an
  existing `<dir>/<dir>.mdx` (`ingest.py:2493`). However, if a
  directory contains exactly one published integration plus
  zero non-integrations, the script special-cases that as
  content and skips the grid (`ingest.py:2429-2503`).
  Adding/removing files can flip a directory between "grid"
  and "leaf" presentations, surprising the maintainer.

- **`safe_cleanup_learn_folders` deletes ALL `.json` files
  unconditionally** (`ingest.py:1061-1063`) without checking
  `part_of_learn`. Hand-authored `_category_.json` in
  `${NETDATA_REPOS_DIR}/learn/docs/` is wiped each run unless
  ingest itself re-creates it via
  `ensure_category_json_for_dirs`.

## Dead code / stale artifacts

### `ingest.js` is LEGACY

`${NETDATA_REPOS_DIR}/learn/ingest.js` is the original Node
orchestrator. It is no longer the live pipeline. The README,
the active workflow, and `<repo>/docs/.map/README.md` all run
`ingest/ingest.py`. **Do not edit `ingest.js`**; do not trust
its behavior as a description of the current pipeline.

### `ingest.md` is stale

`${NETDATA_REPOS_DIR}/learn/ingest.md` documents `ingest.js`,
not `ingest.py`. The `README.md` describes the Python pipeline
(at lines 52-172). When in conflict, README + `ingest.py` win
over `ingest.md`.

### `ingest/create_grid_integration_pages.py` is empty

`${NETDATA_REPOS_DIR}/learn/ingest/create_grid_integration_pages.py`
is 0 bytes. The README at lines 127-128 still tells users to
run it; the actual grid generation moved into
`ingest.py:get_dir_make_file_and_recurse` (`ingest.py:2333-2510`).
Running the empty script does nothing and produces no error.

### Duplicate link-checker

`${NETDATA_REPOS_DIR}/learn/scripts/check_learn_links.py`
duplicates
`${NETDATA_REPOS_DIR}/learn/ingest/check_learn_links.py`
verbatim. Pick one as canonical; today both exist.

### `search-icons.js` is dead code

`${NETDATA_REPOS_DIR}/learn/search-icons.js` is a
developer-only icon-picker shim that's never imported. Safe to
ignore.

### `.bak` workflows

Three legacy workflows kept as `.bak`:
- `${NETDATA_REPOS_DIR}/learn/.github/workflows/old_ingest.yml.bak`
  (Node `ingest.js` cron),
- `old_check-broken-links.yml.bak`,
- `old_check-broken-links-external.yml.bak`,
- `check-internal-links.yml.bak`.

Not active. Useful as historical reference only.

### `produce_gh_edit_link_for_repo` typo

`ingest.py:1027-1035`: the format string is
**single-quoted** (`"https://github.com/netdata/{repo}/edit/master/{file_path}"`)
-- the `f` prefix is missing -- so the function returns the
literal string with `{repo}` and `{file_path}` unsubstituted
for non-`.github` repos. Not currently called in the live
pipeline, but a real bug that would surface if the function
ever gets invoked.

## Versioning is effectively unused

- `${NETDATA_REPOS_DIR}/learn/versioning/remove_edit_links.py`
  is the only versioning helper. It is a manual prep step for
  snapshotting a version: rewrite `custom_edit_url:` to
  `null`. Not automated.
- No `versioned_docs/`, `versioned_sidebars/`, or
  `versions.json` files exist in
  `${NETDATA_REPOS_DIR}/learn/`.
- `package.json` has standard `docusaurus`, `start`, `build`,
  `swizzle`, `deploy`, `clear`, `serve` scripts but no
  version-tagging script.
- Conclusion: there is one live version. Freezing one would
  require manual `versioning/remove_edit_links.py` on a
  snapshot, then `docusaurus docs:version <name>` (no
  automation exists for this).

## Netlify redirect-rule ceiling

The dynamic redirect section keeps growing -- already ~12,700
lines in `${NETDATA_REPOS_DIR}/learn/netlify.toml`. The
`LegacyLearnCorrelateLinksWithGHURLs.json` already has ~3,490
entries.

**Netlify's redirect-rule limit is ~10,000 rules per site.**
This repo is approaching/past that ceiling. Not noted anywhere
in the live code or docs.

When the limit is exceeded, Netlify deploys still succeed but
some redirects stop working. Followup: either prune the
catalog (drop redirects older than N months) or move the
mechanism to a different layer.

## Inferring repo from a local `--repos /path` argument

`ingest.py:2701-2738`: `ingest.py` first matches the basename
to a known repo key; if no exact match, it does a SUBSTRING
match in either direction (`netdata` vs `mynetdata-fork`).
This can pick the wrong repo silently.

When testing locally with a fork that has an unusual name, use
`--local-repo netdata:/path/to/your/fork` to force the right
mapping.

## Schemas accept extras silently

`<repo>/docs/.map/map.schema.json` sets
`additionalProperties: false`. Validation IS strict; unknown
keys fail the run.

But the per-file metadata block format is NOT schema-validated
end-to-end; the script tolerates extra keys in the
`<!--startmeta...endmeta-->` block. Stray keys pass through to
the rendered frontmatter and may either be ignored by
Docusaurus or, in rare cases, trigger build warnings.

## `learn_link` is informational only

`learn_link:` in frontmatter is NOT used by Docusaurus for
routing. It is checked daily by `check_learn_links.py` (HEAD
requests against `learn.netdata.cloud`) -- its purpose is to
catch the case where a slug in the source-controlled file no
longer matches what's actually deployed.

## Logo contrast analysis makes outbound HTTP calls

`ingest.py:1647-1681` makes outbound HTTP calls to
`netdata.cloud/img/...` for every integration during ingest.
Result is cached per URL within a run. CI runs over hundreds
of logos; total network time is bounded by
`LOGO_ANALYSIS_TIMEOUT = 8 s` per request. Transient network
errors during ingest can cause subtle visual inconsistencies
(missing data attributes on logos).

## Sidebar position 0 reservation

`sidebar_position: 0` is reserved for "Ask Nedi"
(`ingest.py:493-512`). Any other top-level entry assigned
position 0 gets re-stamped to >= 10. So you can't pin a
non-`docs/ask-nedi.mdx` page to the very top of the sidebar.

## Empty/unmapped directories vanish

`ensure_category_json_for_dirs` (`ingest.py:357-360`) does NOT
create `_category_.json` when a directory has no `.mdx` files.
So if you remove all docs from a category but leave the empty
dir, it disappears from the sidebar (Docusaurus drops empty
dirs). To intentionally keep an empty section, you need at
least one `learn_status: Published` page in it.

## Slugs in source frontmatter are sticky

Ingest writes `slug:` in the frontmatter for every published
file. Authors who want a stable URL across renames must set
`slug:` in the source `.md` (which becomes the override path
at `ingest.py:1890-1895`).

But the source file's frontmatter is rewritten by ingest, so
`slug:` must be set BEFORE the metadata block, where it
survives the `<!-- ... -->` rewrite. In practice authors
rarely do this; they rely on `meta.label` and the redirect
machinery instead.

## Auto-redirect chain depth is unbounded

Every ingest appends new entries to
`LegacyLearnCorrelateLinksWithGHURLs.json`. Old redirects are
re-resolved each run via `UpdateGHLinksBasedOnMap`, so a
multiple-times-moved page keeps working. But the catalog grows
indefinitely.

## Home page depends on `ask-nedi.mdx`

`${NETDATA_REPOS_DIR}/learn/src/pages/index.js:1-6` redirects
`/` -> `/docs/ask-nedi`. Anyone removing or renaming
`ask-nedi.mdx` (which is `part_of_learn: True` and otherwise
survives ingest) breaks the site root.

## Search is local, not Algolia

`docusaurus.config.js:34-39` uses
`@easyops-cn/docusaurus-search-local` -- client-side, hashed
indexes built at `yarn build` time. There is NO Algolia
DocSearch integration. Search results are not as polished, and
indexing is build-time (changes propagate only on next deploy).

## Posthog, GTM, gtag, Reo.dev, Nedi UI

`docusaurus.config.js:179-264` loads Posthog, Google Tag
Manager, Google gtag, Reo.dev, and the Nedi UI as
plugins/scripts. The Nedi embed depends on
`nedi.netdata.cloud/ai-agent-public.js`; if that origin is
down, the chat widget fails silently but the rest of the site
loads.

## Custom anchor IDs

`ingest.py:531-560` extracts header anchors via a slugger.
Custom anchors via `<a id="..."> </a>` may not match the
slugger and trigger anchor-mismatch warnings during link
resolution (step 12 of the pipeline). Use `## Heading` text
that produces the desired slug instead of injecting custom
anchors.

## OpenAPI / Swagger

`${NETDATA_REPOS_DIR}/learn/static/api/` has Swagger UI files.
Not part of the docs/ ingest tree. Updated separately when the
agent's OpenAPI surface changes.

## Forks / non-`netdata/netdata` sources

The pipeline assumes `AGENT_REPO = 'netdata/netdata'`
everywhere (`ingest.py:15`). Running `ingest.py` against a
fork (e.g. `--repos ktsaou/netdata:master`) works, but
`build_path` (`gen_docs_integrations.py:81-90`) assumes
`meta_yaml` URLs start with `https://github.com/netdata/...`,
so integration page URL rewriting may break. For local
testing use `--local-repo netdata:/path/to/your/fork` so the
edit URLs still reference `netdata/netdata` upstream.
