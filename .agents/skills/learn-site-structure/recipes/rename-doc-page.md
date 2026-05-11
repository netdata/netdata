# Recipe: rename a doc page

Rename a page (change its display name in the sidebar / URL).
Functionally identical to a move because the URL slug is
derived from `meta.label`. Auto-redirected.

## 1. Edit map.yaml

Open `<repo>/docs/.map/map.yaml`. Find the node and change
`meta.label`:

```yaml
# Before
- meta:
    label: Old Name
    edit_url: https://github.com/netdata/netdata/edit/master/docs/x/page.md

# After
- meta:
    label: New Name
    edit_url: https://github.com/netdata/netdata/edit/master/docs/x/page.md
```

`edit_url` stays unchanged (source file is still the same
file).

## 2. Optionally rename the source file too

If you want the source file to also have a new name (cleaner),
rename it in this repo AND update `meta.edit_url` to match.
Two scenarios:

- **Just label change**: edit_url unchanged. Auto-redirect
  works; old URL maps to old GH URL maps (via the now-current
  map) to new URL.
- **Label + source filename change**: edit_url changes. The
  diff mechanism keys on edit_url, so the OLD edit_url is
  what stores the redirect key. After the rename, the OLD
  edit_url no longer points to a real source file in the
  current commit -- but
  `LegacyLearnCorrelateLinksWithGHURLs.json` still has it
  pointing at the new URL via the previous-run snapshot.
  This works for ONE rename cycle. If you rename again later,
  the chain may break -- so prefer to keep edit_url stable.

## 3. Test locally

```bash
cd ${NETDATA_REPOS_DIR}/learn
. venv/bin/activate
python3 ingest/ingest.py --local-repo netdata:<repo> \
    --ignore-on-prem-repo --fail-links-netdata
```

Check that the auto-redirect is generated. See
`../redirects.md`.

## 4. Open the docs PR

map.yaml change (and source file rename if applicable).
Reviewers check the `edit_url` regex and that the rename
makes sense.

## 5. After merge

Same flow as move/add: 0-3 hour cron + ingest PR + manual
merge + Netlify deploy.

## 6. Verify

```bash
# Old URL redirects to new
curl -sI https://learn.netdata.cloud<old-path>   # expect 301
# New URL works
curl -sI https://learn.netdata.cloud<new-path>   # expect 200
```

## Notes

- **Slug computation refresher**: the slug = lowercase
  `<learn_rel_path>/<sanitized-label>`, with spaces -> `-`,
  `//` -> `/`. So `Old Name` -> `old-name`, `New Name` ->
  `new-name`.
- **The filename in the learn repo also changes** because the
  destination filename is derived from `sidebar_label` (with
  case preserved + only certain chars stripped). So
  `${NETDATA_REPOS_DIR}/learn/docs/x/Old Name.mdx` becomes
  `New Name.mdx`. This is normal and ingest handles it.
- **External links to the OLD URL keep working** thanks to
  the auto-redirect. But it's still good practice to update
  any first-party references (within
  `<repo>/docs/...`).

## Common mistakes

- **Renaming `meta.label` to something with `/`, `(`, `)`,
  `,`, `'`, backtick, or `:`** -- those characters get
  stripped by the slug sanitizer. Pick a label that survives
  cleanup.
- **Forgetting that the URL changes.** A rename IS a URL
  change. The auto-redirect mitigates external link breakage,
  but the URL surface itself moves.
- **Renaming a page with `slug:` override in source
  frontmatter.** The auto-redirect mechanism is bypassed
  (slug override wins). You'll need manual JSON surgery
  similar to the delete recipe.
