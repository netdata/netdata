# Redirects

One mechanism: Netlify `[[redirects]]` rules in `netlify.toml` of `netdata/learn`, a file ingest generates end to end
(`write_netlify_config` in `ingest/autogenerateRedirects.py`). There is no Docusaurus client-redirect plugin and no
`src/pages/` root component; the site root (`/` to `/docs/ask-nedi`) and the old `/docs/ask-netdata` route are
ordinary rules in `static.toml`; a frontmatter `redirect_from` is inert on this site. Owner of the inputs and the
gate: `netdata/learn` `README.md`, section "Redirects".
Verified against `netdata/learn @ c3a16edd5ee4dc819976ef162c9afaff4b9b968c`.

## Inputs

- `static.toml`: hand-written rules, copied verbatim into the static section of `netlify.toml`; also the `[build]`
  table Netlify reads. Count its rules with `grep -c '^\[\[redirects\]\]' static.toml`.
- The dynamic section of the tracked `netlify.toml`: rules from earlier runs, carried forward.
- `LegacyLearnCorrelateLinksWithGHURLs.json`, the catalogue: key = historical `learn.netdata.cloud` route, value =
  the GitHub blob URL of the source file that route must reach. Ingest re-resolves every value through the current
  source-to-page mapping (`UpdateGHLinksBasedOnMap`), so a redirect follows its page through any number of later moves
  while the source stays mapped.

## What one run does (`main`)

1. `addMovedRedirects` compares the current `custom_edit_url` to slug mapping with the previous run's
   `ingest/one_commit_back_file-dict.yaml` and creates an entry for every page whose slug changed: a `map.yaml` move,
   or a label change, which changes the slug (`./mapping.md#file-path-and-slug`).
2. `UpdateGHLinksBasedOnMap` turns every catalogue value that is the GitHub URL of a published source into that
   page's route. `gate_legacy_redirects` then classifies each entry: resolved (the value became a live route, or was
   already one), retained (the value is still a URL, but a `static.toml` rule or a tracked rule to a live page covers
   the route; reported), retired (listed under `legacy_catalogue_retirements` in `config/redirect-policy.json`), or
   failed. A failed entry raises `LegacyRedirectGateError`, ingest exits 3, and nothing below is written. A value
   that is any other absolute URL, a `learn.netdata.cloud` one included, is never resolved through the map: it is
   retained or retired when a rule or the policy covers it, and failed otherwise.
3. `append_entries_to_json` adds the moved entries to the catalogue; `combineDictsOverwrite` merges the resolved
   entries into the tracked rules and raises `ValueError` (an uncaught traceback, not exit 3) when a route would get
   a different target; `clean_redirects` drops rules whose source is a live route and collapses chains;
   `write_netlify_config` writes the file (a static wildcard conflict raises `ValueError` there).

`reconcile_generated_outputs` in `ingest.py` runs `clean_redirects` and `write_netlify_config` again at the end of
the run. Rules carry no explicit status code; Netlify's default applies.

## Author consequences

- Move or rename: automatic. Keep `edit_url` unchanged for a move; the catalogue is anchored to it. If the source file
  is also renamed, the old `edit_url` becomes an unresolvable catalogue value the next time the page moves, so prefer
  stable source paths.
- Delete: `docs/.map/README.md#unpublishing-files`. Repoint the catalogue value to the replacement page's GitHub blob
  URL, or remove the entry; an unresolvable entry fails the ingest at the gate (exit 3) rather than serving a silent
  404. Run ingest and commit the regenerated `netlify.toml` with the catalogue change; a resolved target that
  disagrees with the committed rule for the same route fails the merge in `combineDictsOverwrite`. Re-publishing a
  deleted page later needs no further surgery: the
  entry resolves again. A one-off rule, for example to an external domain, goes in `static.toml`.
- The catalogue only grows by moves; retirements are the pruning path. Netlify's per-site rule limit is a platform
  figure, not in this repository; compare it with the rule count from the command above rather than with line counts.
