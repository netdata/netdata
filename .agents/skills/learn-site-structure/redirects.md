# Redirects

Learn supports four redirect mechanisms layered in precedence
order. Most page moves and renames produce **automatic**
redirects. Page deletions require **manual** surgery.

## The four mechanisms

| # | Mechanism | Precedence | Authoritative? | Where configured |
|---|---|---|---|---|
| 1 | Netlify `[[redirects]]` (in `netlify.toml`) | edge -- before SPA loads | YES | Generated end-to-end by `ingest/autogenerateRedirects.main` each ingest |
| 2 | Docusaurus `@docusaurus/plugin-client-redirects` | client-side, after SPA loads | client-only | `docusaurus.config.js:167-178` |
| 3 | React root redirect | client-side fallback | hand-coded | `${NETDATA_REPOS_DIR}/learn/src/pages/index.js:1-6` (`/` -> `/docs/ask-nedi`) |
| 4 | Frontmatter `redirect_from` | client-side, Docusaurus-native | NOT used | grep yields zero hits in `docs/` |

So mechanism 1 is the workhorse. Mechanisms 2 and 3 are
limited and supplemental. Mechanism 4 is theoretically
supported but not used here.

## Mechanism 1: Netlify edge redirects (the workhorse)

`${NETDATA_REPOS_DIR}/learn/netlify.toml` is REGENERATED end
to end by `ingest/autogenerateRedirects.main`
(`autogenerateRedirects.py:172-213`) on every ingest run. It
has two sections; both are regenerated each ingest:

1. **Static section** (`# section: static << START / END`) --
   copied verbatim from
   `${NETDATA_REPOS_DIR}/learn/static.toml` (hand-curated).
   Currently 28 rules covering legacy `/docs/agent/...` and
   `/docs/nightly/...` paths, the `/guides -> /docs`
   migration, and a handful of category remaps (e.g.
   `kubernetes-k8s-netdata -> /docs/collecting-metrics/kubernetes`).

2. **Dynamic section** (`# section: dynamic << START / END`)
   -- built from
   `${NETDATA_REPOS_DIR}/learn/LegacyLearnCorrelateLinksWithGHURLs.json`
   joined with the just-finished ingest's
   `(GH URL -> Learn URL)` map. Currently many hundreds of
   rules; the file is ~12,700 lines.

Both `netlify.toml` (the live published file) and
`LegacyLearnCorrelateLinksWithGHURLs.json` are committed to
git on each ingest PR.

### Auto-redirects on move/rename

When a node moves in `<repo>/docs/.map/map.yaml` (or its
`meta.label` changes -- which changes the slug), the diff-based
`addMovedRedirects` (`autogenerateRedirects.py:124-155`):

1. Builds the current `(custom_edit_url -> new_learn_path)`
   mapping.
2. Compares with the previous-run snapshot at
   `ingest/one_commit_back_file-dict.yaml`.
3. For every `custom_edit_url` whose target moved, adds
   `https://learn.netdata.cloud<old_path> -> <github_blob_url>`
   to the redirect set.
4. `append_entries_to_json` writes the new entries to
   `LegacyLearnCorrelateLinksWithGHURLs.json`.
5. `UpdateGHLinksBasedOnMap` reads ALL entries from
   `LegacyLearnCorrelateLinksWithGHURLs.json` (legacy + just
   appended), looks up their current Learn target via the
   `mapping`, and folds them into the dynamic-redirect
   section.

### Indirection: GH-URL anchored

The redirect store is anchored to the GitHub blob URL of the
source file, NOT to the old Learn URL directly. So a moved
page's old URL keeps redirecting forever, even after several
subsequent moves -- as long as the source file still exists
somewhere in `map.yaml`. Each ingest re-resolves
`GH URL -> current Learn URL` via the live mapping.

If the source file is deleted (the GH URL no longer points to
real content), the redirect resolves to a missing URL and
serves a 404 unless someone manually edits the JSON. See the
delete recipe.

## Mechanism 2: Docusaurus client redirects

`${NETDATA_REPOS_DIR}/learn/docusaurus.config.js:167-178`:

```js
redirects: [{ from: '/docs/ask-netdata', to: '/docs/ask-nedi' }]
```

Currently one entry. These are client-side redirects rendered
as static HTML stubs by the Docusaurus build. Used for in-app
links the SPA might encounter.

## Mechanism 3: React root redirect

`${NETDATA_REPOS_DIR}/learn/src/pages/index.js:1-6` -- the home
page is a React component that redirects `/` -> `/docs/ask-nedi`.

This is the LAST FALLBACK for the homepage. If you remove or
rename `ask-nedi.mdx` (which is `part_of_learn: True` and
otherwise survives ingest), you break the site root.

## Mechanism 4: frontmatter `redirect_from` -- not used

Docusaurus supports `redirect_from` in frontmatter. A grep
across `${NETDATA_REPOS_DIR}/learn/docs/` returns zero hits.
Don't use this mechanism here -- it's not part of the
established conventions.

## Order of precedence

Server-then-client:

1. Netlify edge rules (mechanism 1) catch the request before
   anything else.
2. If the URL passes through, Docusaurus client redirects
   (mechanism 2) handle in-app navigation.
3. The React root redirect (mechanism 3) is the last fallback
   for the homepage.

A redirect from one mechanism cannot override a redirect in a
higher-priority mechanism. So an `/docs/old-page` URL caught by
the dynamic Netlify section never reaches the Docusaurus
client redirect plugin.

## What happens when a page moves

The diff-based mechanism handles it automatically:

1. Author updates `<repo>/docs/.map/map.yaml`.
2. Docs PR merges to `master`.
3. The next ingest cycle (within 3 hours) detects the diff and
   adds a redirect to `LegacyLearnCorrelateLinksWithGHURLs.json`
   and `netlify.toml`.
4. Old URL keeps working forever.

## What happens when a page is renamed

Same as a move (the URL slug is derived from `sidebar_label`,
so changing `meta.label` is functionally a move).

## What happens when a page is deleted

The author MUST manually update
`${NETDATA_REPOS_DIR}/learn/LegacyLearnCorrelateLinksWithGHURLs.json`
per `<repo>/docs/.map/README.md:96-104`. The recipe:

1. Delete the source `.md` and remove the matching node from
   `map.yaml`. Open the docs PR.
2. Open `LegacyLearnCorrelateLinksWithGHURLs.json` in the
   learn repo.
3. Search for the GitHub blob/edit URL of the deleted file.
4. Either:
   - Update its value to the closest live replacement on Learn
     (so old links redirect somewhere useful).
   - Or delete the entry (resulting in a 404 for the old URL).
5. Optionally add a manual entry in `static.toml` under
   `# section: static` for hard-coded one-off redirects.

Without this manual step, the redirect resolves to a missing
URL and serves a 404. See `recipes/delete-doc-page.md`.

## How redirects are committed

Each ingest run commits two files to the learn-repo PR:

- `netlify.toml` (always regenerated end-to-end).
- `LegacyLearnCorrelateLinksWithGHURLs.json` (appended to;
  preserves legacy entries, adds new diff entries).

Both are inspected during the manual review of the
`Ingest New Documentation` PR.

## Static `static.toml` redirects

The hand-curated static section of `static.toml` is for
one-off redirects that the diff mechanism wouldn't catch:

- Cross-domain migrations (e.g. legacy `/docs/agent/...` ->
  current paths).
- Category remaps (e.g.
  `/docs/collecting-metrics/kubernetes-k8s-netdata` ->
  `/docs/collecting-metrics/kubernetes`).
- The `/guides` -> `/docs` migration.

Edit `static.toml` directly to add a static redirect; it gets
copied into `netlify.toml` on the next ingest run.

## Risks

- **Unbounded redirect growth.** Every ingest appends new
  entries to `LegacyLearnCorrelateLinksWithGHURLs.json`. The
  dynamic redirect section keeps growing -- already ~12,700
  lines. Netlify's redirect-rule limit is ~10,000 rules per
  site. The repo is approaching/past that ceiling. Not noted
  anywhere in the live code or docs. Followup item.
- **Forgotten deletions.** Deleting a source file without
  manual JSON surgery results in a 404 on the old URL with no
  warning. The daily link checker
  (`daily-learn-link-check.yml`) will flag it, but only after
  publication.
- **Slug overrides bypass the diff mechanism.** If a file has
  `slug:` in frontmatter, that value wins. Renaming the file
  or moving its `map.yaml` row does NOT change the URL, so no
  redirect is generated. To rename a slug-override page, edit
  the slug AND ensure the redirect catalog gets a manual
  entry.
