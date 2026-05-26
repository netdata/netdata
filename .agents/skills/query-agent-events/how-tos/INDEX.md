# How-tos -- INDEX

Live catalog of analysis-derived how-tos for the
query-agent-events skill.

**Live-catalog rule** (also stated in `../SKILL.md`): if an
assistant is asked a concrete question about agent-events
that requires non-trivial analysis (multiple file reads,
multiple queries, cross-referencing producer source) AND the
answer is not already documented in the per-domain guides
(`../AE_FIELDS.md`, `../transports.md`, `../update-cadence.md`,
`../query-discipline.md`, `../finding-crashes.md`,
`../finding-fatals.md`) or the recipes (`../recipes/`), the
assistant MUST author a new `how-tos/<slug>.md` and add a
one-line entry to this INDEX BEFORE completing the task.

This is durable. Skipping it means the next assistant repeats
the same analysis from scratch.

## Catalog

(empty -- entries grow as assistants encounter
not-yet-documented questions)

| Topic | Slug | Notes |
|---|---|---|
| -- | -- | -- |

## How to add a how-to

1. Create `how-tos/<slug>.md` with:
   - One-line summary at the top (the question being answered).
   - The answer with `path:line` citations into producer source
     where appropriate.
   - A "How I figured this out" footer naming the files read,
     the queries run (with payloads), and the helpers used.
2. Add a row to the table above with topic, slug, short notes.
3. Commit alongside the work that prompted the analysis.

## When NOT to add a how-to

- The question is already covered by an existing per-domain
  guide or recipe -- update that guide instead.
- The answer is a one-liner.
- The answer is highly speculative or version-specific.
