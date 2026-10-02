# Recipe: rename a doc page

The URL is computed from `label` (`../mapping.md#file-path-and-slug`), so a label change is a URL change, handled like
a move (`./move-doc-page.md`): the old route is redirected by the next ingest.

1. Change `meta.label` in `docs/.map/map.yaml`; leave `edit_url` alone unless the source file is renamed as well.
2. Predict the new slug and file name: `_sanitize_mdx_filename_source` replaces `'`, `:`, `/`, `(`, `)`, `,`, and
   the backtick with spaces, then the slug lowercases and joins with `-`. Check that no sibling produces the same
   name; a collision between non-integration pages aborts ingest.
3. `meta.path` does not rename the page's own segment; on a leaf it adds a directory segment above it
   (`../mapping.md#rows-ingest-reads-from-the-map`). The last segment always follows `label`.
4. Run `docs/.map/validate_map_schema.py` (`../mapping.md#what-is-checked-and-by-what`) and the local ingest
   (`docs/.map/README.md#2-test-the-changes`); the file under learn `docs/` is renamed and a redirect entry appears for
   the old route.
5. Update first-party links to the page in this repository (they are rewritten by ingest only if written as
   repository-relative `.md` paths, `../mapping.md#links-between-pages`); external links keep working through the
   redirect.
6. Open the PR here; after deploy, request the old URL and confirm the redirect.

There is no per-page slug override: a `slug:` in source frontmatter is discarded
(`../mapping.md#frontmatter-ingest-writes`), and a label change always moves the URL.
