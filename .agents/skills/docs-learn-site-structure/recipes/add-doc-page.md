# Recipe: add a new doc page to Learn

Add a new published page on `learn.netdata.cloud` from the
agent repo (this repo) or any of the 6 source repos.

## 0. Read first

- `../SKILL.md` -- skill overview.
- `../mapping.md` -- the map.yaml schema and URL computation.

## 1. Create the source markdown

Place the file under a reasonable location in the source repo.
Path is largely cosmetic; what matters is the `map.yaml` row.

For this repo, suggested locations:
- `<repo>/docs/<existing-section>/<your-page>.md` for general
  user-facing docs;
- `<repo>/docs/developer-and-contributor-corner/<page>.md` for
  developer notes;
- `<repo>/docs/netdata-agent/<page>.md` for agent docs;
- `<repo>/docs/dashboards-and-charts/<page>.md` for dashboard
  docs.

Author the markdown content normally. No special frontmatter
required; ingest will inject what it needs.

If you absolutely must pin a stable URL across renames, add
`slug: /your/stable/path/here` at the very top of the file
(before any HTML-comment metadata block) -- but this is rare
and usually unnecessary because the auto-redirect mechanism
handles renames.

## 2. Add a node to map.yaml

Open `<repo>/docs/.map/map.yaml` and add a row under the
appropriate parent:

```yaml
- meta:
    label: My New Page
    edit_url: https://github.com/netdata/netdata/edit/master/docs/<section>/<your-page>.md
    description: Optional one-line description for SEO.
    keywords: [keyword1, keyword2]
```

The position of the row in the YAML determines the sidebar
position within the parent (sibling-relative; ingest assigns
`sidebar_position` in steps of 10 in traversal order).

If you want a single-segment override of the URL slug
(different from the label), add `path: <slug>` under the
`meta` block.

## 3. Test locally

From the learn repo:

```bash
cd ${NETDATA_REPOS_DIR}/learn
. venv/bin/activate    # if not already
python3 ingest/ingest.py --local-repo netdata:<repo> \
    --ignore-on-prem-repo --fail-links-netdata
yarn start             # browse at http://localhost:3000
```

Check:
- Your new page appears in the sidebar under the expected
  parent.
- The URL is what you expected
  (`/docs/<learn_rel_path>/<sidebar_label>` lowercased).
- Links from / to your page resolve.

## 4. Open the docs PR in this repo

Single PR with two commits (or one):
1. The new `.md` file under `<repo>/docs/`.
2. The map.yaml change adding the node.

Reviewers check:
- The map.yaml row has all required fields (`label`,
  `edit_url`).
- The `edit_url` matches the schema regex
  (`^https://github\.com/netdata/<repo>/edit/<branch>/.+\.(md|mdx)$`).
- The page is in a sensible parent in the tree.

## 5. After merge

`${NETDATA_REPOS_DIR}/learn/.github/workflows/ingest.yml`
fires within 3 hours (or you can `workflow_dispatch` it
immediately). Ingest opens an "Ingest New Documentation" PR
in the learn repo. A learn-repo maintainer reviews and merges
it. Netlify deploys within minutes.

End-to-end timing: 0-3 hours of cron + manual review/merge +
minutes of Netlify deploy.

## 6. Verify on production

After deploy, check:
- `https://learn.netdata.cloud/docs/<your-rel-path>/<your-label>`
  resolves.
- The page renders correctly (no MDX errors).
- Sidebar position is what you expected.
- Page metadata (title, description, keywords) is populated
  from your `meta` block.

## Common mistakes

- **Forgetting the map.yaml row.** The page won't be
  published. There's no warning -- ingest silently skips
  source files not in the map.
- **Wrong `edit_url` format.** Schema validation fails the
  whole ingest run with exit code 2. Check the regex.
- **Source path implies the URL.** It does NOT. The URL is
  computed from `meta.label` + `learn_rel_path`. Don't expect
  the source filesystem path to influence routing.
- **Hand-editing `${NETDATA_REPOS_DIR}/learn/docs/<page>.mdx`
  directly.** Wiped on next ingest. Use `part_of_learn: True`
  if you really need a hand-authored exception.
- **Wrong source repo for the content.** If your content is
  about Netdata Cloud On-Prem, edit
  `${NETDATA_REPOS_DIR}/netdata-cloud-onprem/`, not this repo.
  But the map.yaml row goes in THIS repo.
