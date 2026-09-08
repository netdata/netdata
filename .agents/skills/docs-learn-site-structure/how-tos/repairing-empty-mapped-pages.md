# How do I repair a mapped Learn page whose body is empty?

Repair the source page and its `docs/.map/map.yaml` node in the repository that owns the content. The generated MDX
under learn `docs/` is deleted and rebuilt on every ingest (`../authoring-boundary.md`).

1. Find the route's node in `docs/.map/map.yaml`; its `edit_url` is the join key to the source file regardless of the
   file's directory (`../mapping.md#the-join-key`).
2. Compare the source Markdown with the generated MDX. If both are empty, ingest preserved an empty source correctly
   and the fix belongs in the source repository.
3. Decide whether the source is hand-authored or generated. A generated integration page carries
   `INTEGRATION_MARKER` and is repaired in its `metadata.yaml` or producer input, then regenerated
   (`integrations-lifecycle`); an ordinary mapped page is edited directly.
4. For an ordinary page, add an accurate `meta.description` to the node; ingest injects it as frontmatter
   (`../mapping.md#frontmatter-ingest-writes`). `integrations/tests/test_descriptions.py` requires a valid description
   on the pages listed in its `MAP_DESCRIPTION_TARGETS` and case-folded uniqueness across every description in the
   map, and runs in `.github/workflows/check-markdown.yml`; add the page to the targets to pin it.
5. Add source-level regression coverage for the objective defect (map ownership, description presence, heading
   structure, resolvable links); a word count is not a completeness measure.
6. Run `docs/.map/validate_map_schema.py` (`../mapping.md#what-is-checked-and-by-what`) and a disposable ingest with
   `--local-repo` (`docs/.map/README.md#2-test-the-changes`); inspect the generated MDX for title, description, H1,
   body, and links.

Origin: the `Clocks` and `Socket` nodes, whose sources are `src/libnetdata/clocks/README.md` and
`src/libnetdata/socket/README.md`, rendered empty; the sources were the fix and `test_descriptions.py` now covers them.
