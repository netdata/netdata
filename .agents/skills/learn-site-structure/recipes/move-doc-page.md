# Recipe: move a doc page (different sidebar location)

Move a page from one sidebar location to another. The source
file stays where it is; only its position in `map.yaml`
changes. Ingest auto-generates a redirect from the old URL to
the new one.

## 0. Pre-check

Confirm the page is a regular doc (`.md`), not an integration
page. Integration pages flow through the integrations
pipeline; their position is driven by their `meta.categories`,
not by `map.yaml`. To move an integration page, you change
the integration's category in its `metadata.yaml` (see the
`integrations-lifecycle` skill).

## 1. Edit map.yaml

Open `<repo>/docs/.map/map.yaml`.

1. Find the existing node for the page.
2. Cut it from the old location and paste it under the new
   parent.
3. Keep `meta.edit_url` UNCHANGED -- it still points to the
   same source file.
4. Optionally adjust `meta.label` if the new location calls
   for a different display name (this changes the URL slug
   too -- in that case it's effectively also a rename).

## 2. Test locally

```bash
cd ${NETDATA_REPOS_DIR}/learn
. venv/bin/activate
python3 ingest/ingest.py --local-repo netdata:<repo> \
    --ignore-on-prem-repo --fail-links-netdata
```

Check:
- The page appears under the new parent in the sidebar.
- A redirect entry was added to `netlify.toml` and
  `LegacyLearnCorrelateLinksWithGHURLs.json` from the old URL
  to the new target.

```bash
grep -A1 "<old-url>" netlify.toml LegacyLearnCorrelateLinksWithGHURLs.json
```

## 3. Open the docs PR

Single map.yaml change. The source `.md` file is unchanged.

## 4. After merge

Same as add-doc-page: 0-3 hour cron + ingest PR + manual
merge + Netlify deploy. The auto-generated redirect ships
with the ingest PR.

## 5. Verify

After deploy:
- The page is at the new URL.
- The OLD URL redirects to the new one (HTTP 301 from
  Netlify edge).

```bash
curl -sI https://learn.netdata.cloud<old-path>
# Expect: HTTP/2 301
# Location: https://learn.netdata.cloud<new-path>
```

## Notes

- **Auto-redirect is via the GitHub blob URL**, not via the
  old Learn URL directly. The catalog is anchored to
  `github.com/netdata/netdata/blob/master/<source-path>` so
  multiple consecutive moves keep working as long as the
  source file still exists.

- **If you also rename the file in source**, treat that as a
  separate move + you must update `meta.edit_url`. The diff
  mechanism still works because the OLD `meta.edit_url` value
  is what gets stored as the redirect's GH URL key.

- **`slug:` overrides bypass the diff mechanism.** If the
  page has a frontmatter `slug:`, moving the map.yaml row
  does NOT change the URL, so no redirect is generated. To
  move a slug-override page, edit the slug AND ensure a
  manual entry lands in
  `LegacyLearnCorrelateLinksWithGHURLs.json` (similar to the
  delete recipe).

## Common mistakes

- **Editing `meta.edit_url` when only moving.** The edit_url
  should NOT change for a move (the source file stays put).
- **Editing the source file path AND the map.yaml row in the
  same PR without updating `meta.edit_url`.** Will produce
  validation failures because the schema regex requires the
  edit_url to match an existing source file.
- **Expecting the OLD URL to disappear.** It auto-redirects
  forever. Don't break links pointing to it.
