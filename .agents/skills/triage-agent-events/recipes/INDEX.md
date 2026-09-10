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
source .agents/skills/triage-agent-events/scripts/_lib.sh
agentevents_load_env
```

## Live how-to rule

Capture timing and authorization follow `AGENTS.md#knowledge-capture`. Additional authorized how-tos belong under
`../how-tos/` and are listed in `../how-tos/INDEX.md`.
