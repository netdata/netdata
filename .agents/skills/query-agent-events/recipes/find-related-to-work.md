# Recipe: find events related to current work

Use case: "We're working on dbengine cleanup. Are there
crashes in agent-events that match this area? What does the
distribution look like?"

This is the "is anyone hitting this?" template. Useful for
prioritizing work and validating fixes.

## Pattern

You have a candidate area defined by some combination of:

- a function name or symbol -> use `--function` or FTS
- a source file path -> use selections on `AE_FATAL_FILENAME`
- a code-path keyword that would appear in the stack trace
  -> use `--query` (FTS narrower)
- a thread name (e.g. `STREAM:N`, `CTXLOAD`, `APP-COLLECT`)
  -> use selections on `AE_FATAL_THREAD`

Combine the most-specific predicates first, then widen.

## Worked example

Working on dbengine page eviction:

```bash
# 1. Try by function -- the obvious symbols.
.agents/skills/query-agent-events/scripts/get-events.sh \
    --function 'rrdeng_page_descr_t,rrdeng_evict_pages,evict_main' \
    --since '14d ago' \
    --versions auto \
    --output /tmp/related-pass1.json

# 2. If pass 1 is sparse, widen to filename.
payload=$(jq -nc '{
  "after": -1209600, "before": 0, "last": 200,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_FATAL_FILENAME": [
      "src/database/engine/cache.c",
      "src/database/engine/pdc.c",
      "src/database/engine/rrdengine.c"
    ]
  }
}')
agentevents_query_function cloud "$payload" > /tmp/related-pass2.json

# 3. If still sparse, FTS over crash class.
.agents/skills/query-agent-events/scripts/get-events.sh \
    --health crash \
    --query 'page_descr OR cache_evict OR rrdeng' \
    --since '14d ago' \
    --versions auto \
    --output /tmp/related-pass3.json
```

Pass 1 (function) is the cheapest and most precise; pass 2
widens to files; pass 3 is the FTS fallback.

## Aggregation

After collecting candidate events, look at the distribution:

```bash
.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input /tmp/related-pass2.json \
    --by signal

.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input /tmp/related-pass2.json \
    --by version

.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input /tmp/related-pass2.json \
    --by fatal_function

.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input /tmp/related-pass2.json \
    --by db_mode
```

Together this tells you:
- how often we crash in this area;
- whether it's all one signal or mixed;
- which versions are affected;
- which functions in the area are the dominant crash sites;
- whether dbengine memory mode correlates.

## Sample one event

```bash
jq '.data[0] as $row | .columns as $cols
    | $cols | to_entries | sort_by(.value.index)
    | map({(.key): $row[.value.index]}) | add' /tmp/related-pass2.json
```

Examine a representative `AE_FATAL_STACK_TRACE`,
`AE_FATAL_MESSAGE`, `AE_FATAL_THREAD`. Cross-reference the
specific commit that introduced the path you suspect.

## Building a "before/after" comparison

If you've landed a fix:

```bash
# Before-fix nightly counts.
.agents/skills/query-agent-events/scripts/get-events.sh \
    --function 'fixed_function' \
    --versions 'v2.10.0,v2.10.0-100-nightly' \
    --since '14d ago' \
    --last 1 --facets 'AE_AGENT_VERSION' \
    --output /tmp/before-fix.json

# After-fix nightly counts.
.agents/skills/query-agent-events/scripts/get-events.sh \
    --function 'fixed_function' \
    --versions 'v2.10.0-130-nightly,v2.10.0-160-nightly' \
    --since '14d ago' \
    --last 1 --facets 'AE_AGENT_VERSION' \
    --output /tmp/after-fix.json
```

Compare counts. Significant drop -> fix is working.

## Pitfalls

- **Empty result for new code**: a function that just landed
  in a nightly may not have crashed in the wild yet. Wide the
  time window to 30 days OR wait for more agents to update.
- **Old fixes that haven't propagated**: if your fix is in
  v2.10.1 but most of the fleet runs v2.10.0, you'll still
  see the old crashes for weeks.
- **Recipe scoping**: don't forget to set
  `"__logs_sources": "agent-events"` when constructing
  payloads manually -- without it, the query targets
  all-local-logs (huge and unrelated).
