# Recipe: delete (unpublish) a doc page

The procedure is `docs/.map/README.md#unpublishing-files`. This recipe adds what the redirect gate expects
(`../redirects.md`) and the order that keeps both repositories consistent.

1. Decide where the old URL should go: a replacement page, or nowhere (a retirement).
2. In this repository, remove the node from `docs/.map/map.yaml` and delete or keep the source file as appropriate.
   Search this repository's docs for links to the page and fix them; ingest reports the rest as broken links, and
   `.github/workflows/check-markdown.yml` fails the PR on them.
3. In `netdata/learn`, find the page's GitHub blob URL among the values of `LegacyLearnCorrelateLinksWithGHURLs.json`
   and every key that points at it:
   - replacement: set the value to the replacement page's GitHub blob URL (the URL of its source file). An absolute
     `learn.netdata.cloud` URL is treated as an external target and is not resolved through the map;
   - retirement: remove the entry, or record it under `legacy_catalogue_retirements` in `config/redirect-policy.json`
     when the route must stay documented as retired;
   - a one-off rule to an external destination goes in `static.toml`.
   Run the learn ingest with `--local-repo netdata:<this checkout>` and commit the regenerated `netlify.toml` with the
   catalogue change; the gate rejects a catalogue target that conflicts with the committed rule.
4. Order the two PRs by what CI checks: this repository's `check-markdown.yml` ingests with the still-unpatched
   catalogue, so an unresolvable entry fails here with exit 3 until the learn change lands or the entry is covered.
   Land the learn catalogue change first, or in the same window, when the deleted page has catalogue entries.
5. After both deploy, request the old URL: the replacement, or the retirement's 404.

Do not delete `docs/ask-nedi.mdx` from the learn repository through this recipe; it is a `part_of_learn` page and the
site root redirects to it (`../authoring-boundary.md`). Do not delete a generated integration page; retire it through
its `metadata.yaml` and the integrations pipeline (`integrations-lifecycle`).
