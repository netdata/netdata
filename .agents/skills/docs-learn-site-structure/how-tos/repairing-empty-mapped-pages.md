# How do I repair a mapped Learn page whose source body is empty?

Repair the source page and its `docs/.map/map.yaml` metadata in the repository that owns the content. Do not edit the generated
MDX in the Learn repository: `ingest/ingest.py` deletes and rebuilds that tree on every ingest.

## Source-first workflow

1. Find the route's `meta.edit_url` in `docs/.map/map.yaml`. That URL is the join key between the source file and Learn, regardless
   of the source file's directory.
2. Inspect the source Markdown and the generated Learn MDX. If both are empty, the ingest is preserving an empty source correctly;
   the fix belongs in the source repository, not in Learn.
3. Check whether the source is hand-authored or generated. A generated integration page carries the integration marker and must be
   repaired in its authoritative integration metadata or producer input; its generated map entry is not the description source. An
   ordinary mapped README or Markdown file is repaired directly.
4. For an ordinary mapped page, add an accurate `meta.description` to the existing map node. Learn injects that value as frontmatter
   during ingest. For a generated integration page, add the description to the owning metadata or producer input and regenerate.
5. Add source-level regression coverage for the objective defect: map ownership, description presence, semantic heading structure,
   and resolvable links. Do not use a word-count target as a completeness proxy.
6. Run the map schema check and a disposable full Learn ingest with the changed repository supplied through `--local-repo`. Inspect
   the generated MDX and built HTML for the final title, description, H1, body, and links.

## Why this works

- `mapping.md` documents that `map.yaml`, not the filesystem path, owns publication and frontmatter injection.
- `pipeline.md` shows that ingest copies and sanitizes the matched source before writing the Learn `docs/` tree.
- `authoring-boundary.md` documents that generated Learn MDX is wiped on the next ingest, while mapped source documents are edited
  in their source repository.

## How I figured this out

- Traced the Clocks and Socket entries in `docs/.map/map.yaml` to `src/libnetdata/clocks/README.md` and
  `src/libnetdata/socket/README.md`.
- Compared those sources with the generated Learn MDX and public HTML.
- Ran Learn's current `ingest/ingest.py` with the Agent worktree supplied through `--local-repo`, then inspected the regenerated
  output rather than editing it.
