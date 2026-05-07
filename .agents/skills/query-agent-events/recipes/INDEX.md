# Recipes -- INDEX

Task-oriented recipes that combine `get-events.sh` +
`analyze-events.sh` for common bug-investigation flows.

| Recipe | Use case |
|---|---|
| `find-by-function.md` | "Is anyone hitting a crash in this specific function?" |
| `find-by-version.md` | "Did crash X start in v2.10? Was it fixed in nightlies?" |
| `find-related-to-work.md` | "We just fixed Y. Is anyone hitting Y in agent-events?" |

For top-level "find all crashes" / "find all fatals" flows,
see `../finding-crashes.md` and `../finding-fatals.md`.

## Common preamble

```bash
cd <repo>
source .agents/skills/query-agent-events/scripts/_lib.sh
agentevents_load_env
```

## Live how-to rule

If you investigate a question that isn't covered by the
existing per-domain guides or these recipes AND your work
involved non-trivial analysis, AUTHOR a how-to under
`../how-tos/<slug>.md` and add a row to `../how-tos/INDEX.md`.
See SKILL.md for the rule.
