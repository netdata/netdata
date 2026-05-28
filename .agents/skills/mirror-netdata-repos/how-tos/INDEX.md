# How-tos -- INDEX

Live catalog of analysis-derived how-tos for the
mirror-netdata-repos skill.

**Live-catalog rule** (also stated in `../SKILL.md`): if an
assistant is asked a concrete question about the mirror that
required non-trivial analysis (multiple file reads, running
the script with custom flags, debugging a failed sync) AND
the answer is not already documented in `../SKILL.md`, the
assistant MUST author a new `how-tos/<slug>.md` and add a row
to this INDEX BEFORE completing the task.

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
   - The answer with file/line citations into the script if
     relevant.
   - A "How I figured this out" footer naming the files read
     and the commands run.
2. Add a row to the table above with topic, slug, short notes.
3. Commit alongside the work that prompted the analysis.

## When NOT to add a how-to

- The question is already covered by SKILL.md.
- The answer is a one-liner.
- The answer is highly speculative or version-specific.
