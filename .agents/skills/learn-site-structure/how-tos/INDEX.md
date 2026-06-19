# How-tos -- INDEX

Live catalog of analysis-derived how-tos for the
learn-site-structure skill.

**Live-catalog rule** (also stated in `../SKILL.md` and
`../recipes/INDEX.md`): if an assistant is asked a concrete
question about how a Learn page is produced that requires
non-trivial analysis (multiple file reads, running ingest
locally, cross-referencing this repo's `map.yaml` with the
learn-repo output) AND the answer is not already documented
under one of the per-domain guides
(`../mapping.md`, `../pipeline.md`, `../sidebars.md`,
`../mdx-rules.md`, `../redirects.md`,
`../pitfalls-and-gotchas.md`, `../authoring-boundary.md`) or
the recipes (`../recipes/`), the assistant MUST author a new
`how-tos/<slug>.md` and add a one-line entry to this INDEX
BEFORE completing the task.

This is a durable rule. Skipping it means the next assistant
repeats the same analysis from scratch -- a framework
violation.

## Catalog

| Topic | Slug | Notes |
|---|---|---|
| Preview a documentation PR locally | `preview-documentation-pr-locally.md` | Isolated Learn ingest/build/browser inspection from PR source content before merge. |

## How to add a how-to

1. Create `how-tos/<slug>.md` with:
   - A one-line summary at the top (the question being
     answered).
   - The answer with file:line citations into this repo or
     the learn repo (`${NETDATA_REPOS_DIR}/learn/...`).
   - A "How I figured this out" footer naming the files
     read and the commands run, so the next assistant can
     verify or extend.
2. Add a row to the table above with topic, slug, and short
   notes.
3. Commit alongside the work that prompted the analysis.

## When NOT to add a how-to

- The question is already covered by an existing per-domain
  guide or recipe -- update that guide instead.
- The answer is a one-liner that doesn't require analysis.
- The answer is highly speculative or version-specific.
