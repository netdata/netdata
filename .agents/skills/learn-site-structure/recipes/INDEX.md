# Recipes -- INDEX

Step-by-step recipes for the most common tasks an author /
maintainer performs against the Learn site.

| Recipe | When to use |
|---|---|
| `add-doc-page.md` | Adding a brand-new page to Learn. |
| `move-doc-page.md` | Moving a page to a different sidebar location (different parent). |
| `rename-doc-page.md` | Renaming a page (changes URL slug). |
| `delete-doc-page.md` | Unpublishing a page. The only recipe that requires manual surgery. |

## Common preamble for all recipes

Every recipe assumes the source repo (almost always
`netdata/netdata`, this repo) is your edit target. The map
file `<repo>/docs/.map/map.yaml` is the lever.

### Quick mental model

1. **What changes appears on Learn:** anything you add to /
   modify in / remove from `<repo>/docs/.map/map.yaml`,
   plus the corresponding `.md` files in source repos.
2. **When changes appear:** ingest CI runs every 3 hours
   (cron) plus on push to learn-repo master plus on manual
   dispatch. So after merging your docs PR to
   `netdata/netdata` master, expect a 0-3-hour delay before
   the ingest PR opens in the learn repo.
3. **Maintainer step in learn repo:** review and merge the
   auto-opened "Ingest New Documentation" PR. Netlify
   deploys within minutes.

### Test locally before pushing

The cheapest way to verify your `map.yaml` change works:

```bash
cd ${NETDATA_REPOS_DIR}/learn

# Set up venv once.
python3 -m venv venv && . venv/bin/activate
pip install -r .learn_environment/ingest-requirements.txt

# Test against your local netdata clone.
python3 ingest/ingest.py --local-repo netdata:<repo> \
    --ignore-on-prem-repo --fail-links-netdata
```

After ingest produces output in
`${NETDATA_REPOS_DIR}/learn/docs/`, browse it with the dev
server:

```bash
cd ${NETDATA_REPOS_DIR}/learn
yarn start    # runs Docusaurus dev server, opens browser
```

## When in doubt

1. Read `../mapping.md` for the source-to-URL computation.
2. Read `../pipeline.md` for what runs in CI.
3. Read `../authoring-boundary.md` to confirm where to edit.
4. Read `../pitfalls-and-gotchas.md` BEFORE assuming the
   pipeline does the obvious thing.
5. If you encountered a question that this catalog doesn't
   cover and you had to investigate to answer it, AUTHOR a
   how-to under `../how-tos/<slug>.md` and add it to
   `../how-tos/INDEX.md`. This rule is mandatory; see
   `../SKILL.md` "Live how-to rule".
