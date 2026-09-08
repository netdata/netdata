# Recipe: add a doc page

Procedure and fields: `docs/.map/README.md#steps-to-publish`. What this recipe adds is the order of operations and the
checks that catch the silent failures.

1. Write the `.md` anywhere sensible in the source repository; the path has no routing effect
   (`../mapping.md#the-join-key`). No frontmatter is needed; ingest replaces the leading HTML comment block, so a
   hand-written `slug:` or `sidebar_position:` has no effect.
2. Add the node to `docs/.map/map.yaml` under the intended parent with `label` and `edit_url`
   (`docs/.map/README.md#meta-fields`); `description` and `keywords` are optional; `path` changes the directory
   segment, never the page's own segment (`docs/.map/README.md#path-reconstruction`,
   `../mapping.md#rows-ingest-reads-from-the-map`). Its position among siblings is its sidebar position
   (`../sidebars.md`). The `edit_url` is the join key and has the exact shape
   `https://github.com/netdata/<repo>/edit/<branch>/<repository-relative path with extension>`, with `<repo>` the
   source repository (`netdata` for this one) and `<branch>` `master`, or `main` for `.github`.
3. Predict the URL: `/docs/<learn_rel_path>/<label>` lowercased, spaces as `-`, after the sanitizer
   (`../mapping.md#file-path-and-slug`). Check for a sibling that sanitizes to the same name.
4. Write cross-references as repository-relative `.md` paths (`../mapping.md#links-between-pages`); keep MDX-hostile
   text in inline code (`../mdx-rules.md`).
5. Run `docs/.map/validate_map_schema.py` (`../mapping.md#what-is-checked-and-by-what`), then the local ingest
   (`docs/.map/README.md#2-test-the-changes`), and confirm the page appears at the predicted path under learn `docs/`
   with the expected frontmatter.
6. Open one PR here with the page and the map change. `.github/workflows/check-markdown.yml` runs the real ingest
   against the PR (`../pipeline.md#verification-before-merging-here`).
7. After merge, `trigger-learn-update.yml` dispatches the learn ingest; a learn maintainer merges the ingest PR;
   Netlify deploys (`../pipeline.md#when-ingest-runs`, `docs/.map/README.md#4-merge-the-learn-ingest-pr`).
8. After deploy, request the predicted URL and confirm the page, its sidebar position, and its description.

Mistakes this catches: a missing node (no error, no page); an `edit_url` that does not match the schema pattern
(exit 2); an `edit_url` that matches the pattern but not the file (no error, no page); content from another source
repository whose node was forgotten here (`../authoring-boundary.md`).
