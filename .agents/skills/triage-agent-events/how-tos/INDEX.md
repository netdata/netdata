# How-tos -- INDEX

Live catalog of analysis-derived how-tos for the
triage-agent-events skill.

Capture timing and authorization follow `AGENTS.md#knowledge-capture`. Check the guides and recipes routed from
`../SKILL.md` before adding an authorized how-to.

## Catalog

| Topic | Slug | Notes |
|---|---|---|
| Trace a stack-symbol regression to a concurrent mutator | `trace-stack-symbol-regression-to-mutator.md` | Separates direct vs stack-only matches, finds the first affected build, maps the faulting call, and validates shared-state lifetime evidence. |

## How to add a how-to

1. Create `how-tos/<slug>.md` with:
   - One-line summary at the top (the question being answered).
   - The answer with `path:line` citations into producer source
     where appropriate.
   - A "How I figured this out" footer naming the files read,
     the queries run (with payloads), and the helpers used.
2. Add a row to the table above with topic, slug, short notes.
3. Git operations follow `AGENTS.md#git-and-pr-workflow`.

## When NOT to add a how-to

- The question is already covered by an existing per-domain
  guide or recipe -- update that guide instead.
- The answer is a one-liner.
- The answer is highly speculative or version-specific.
