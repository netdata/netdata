# Recipe: find events by fatal function

Use case: "Is anyone hitting a crash in `function_name` /
`source/file.c`?"

## Quick path

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --function 'rrdcontext_release,rrdcontext_dispatch_updates_to_main' \
    --since '7d ago' \
    --versions auto \
    --last 200 \
    --output /tmp/by-function.json
```

`--function` is a comma-separated list -- multiple values are
OR'd via `selections.AE_FATAL_FUNCTION`. The query is fully
indexed (no FTS), so 7d windows are cheap.

Then:

```bash
.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input /tmp/by-function.json \
    --by version
# ...and...
.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input /tmp/by-function.json \
    --by signal
```

## By filename instead of function

If you know the file but not the exact function:

```bash
# AE_FATAL_FILENAME is a `selections` field too.
# get-events.sh doesn't have a --filename flag; use jq to
# filter, OR construct the payload manually:

payload=$(jq -nc '{
  "after":  -604800,
  "before": 0,
  "last":   500,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_FATAL_FILENAME": ["src/database/rrdcontext/rrdcontext-cleanup.c"]
  }
}')
agentevents_query_function cloud "$payload" > /tmp/by-filename.json
```

## By symbol via FTS narrower

When the symbol may be in the stack trace but not directly
matched by `AE_FATAL_FUNCTION` (i.e. an inlined or downstream
callee):

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --health crash \
    --query 'inlined_callee_name' \
    --since '7d ago' \
    --versions auto \
    --output /tmp/by-symbol.json
```

`--health crash` is the structured slice (index-friendly);
`--query` is the FTS narrower over the resulting subset. This
is the right composition.

## Triage flow

1. **Get the dump** (as above).
2. **Group by version** -- is this on stable, nightlies, or
   both? Was it new in a recent version?
3. **Group by signal** -- is it always SIGSEGV, or mixed?
4. **Open one event** -- look at `AE_FATAL_STACK_TRACE`,
   `AE_FATAL_LINE`, `AE_FATAL_MESSAGE`, `AE_FATAL_THREAD`.
5. **Cross-reference source** -- read the function in this
   repo at the cited line.
6. **Group by environment** -- arch / os_family / kubernetes /
   profile -- is it environment-specific?
7. **Hypothesize, fix, ship.**

## Common patterns

- Multiple distinct `fatal_function` values that all crash in
  the same source file -> the file has a structural issue
  (state corruption, race condition, invariant violation).
- One `fatal_function` value but mixed signals (SIGSEGV +
  SIGBUS + SIGABRT) -> the function is a chokepoint hit by
  many upstream paths.
- Crashes in one function on parent profile only
  (`AE_AGENT_PROFILE_0=parent`) -> the function is in the
  streaming-receiver path.

## Pitfalls

- **`AE_FATAL_FUNCTION` is the function where `fatal()` was
  called**, not necessarily the function where the crash
  occurred. For signal crashes, it's the function name
  recorded by the deadly-signal handler.
- **Stack trace empty** for instantaneous crashes -- use line
  / filename / function instead.
- **Demangled symbols** -- the function name may have C++
  decorations (e.g. `MyClass::method`). Try the demangled
  form in `--function`.
