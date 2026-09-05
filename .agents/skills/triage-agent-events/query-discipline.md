# Query discipline

How to compose efficient agent-events queries. Hard rule:
**structured filters first, FTS only as residual.**

For the JSON shape of the Function payload (including the
`selections` multi-value filter mechanism), see:

- `<repo>/docs/netdata-ai/skills/query-netdata-cloud/query-logs.md`
  ("Multi-value field selections" section)

This doc covers agent-events-specific guidance: which AE_*
fields to put in `selections`, how to compose them, and the
anti-patterns to avoid.

## The hard rule

```
selections (structured, indexable)  -->  query (FTS, residual narrower)
```

NEVER FTS-first on agent-events. The namespace can hold 40k-200k
records / day; a bare `query` over a wide window full-scans the
dataset.

## What to put in `selections`

Always include at least one of these fields (always present on
every record per `<repo>/src/daemon/status-file.c:967-976`):

- `AE_AGENT_HEALTH` -- crash class. Filter to `crash-*` values
  to drop healthy records.
- `AE_EXIT_CAUSE` -- exit reason (26 distinct values; pick the
  ones you want).
- `AE_AGENT_VERSION` -- producing agent version (regression
  slicing; pair with the auto-version filter).
- `AE_FATAL_SIGNAL_CODE` -- empty / non-empty discriminates
  signal crashes from deliberate fatals / graceful exits.

For more specific triage:

- `AE_FATAL_FUNCTION` / `AE_FATAL_FILENAME` -- localize to a
  function or source file.
- `AE_HOST_ARCHITECTURE` -- arch-specific bugs.
- `AE_OS_FAMILY` / `AE_OS_TYPE` -- distro / OS-specific.
- `AE_AGENT_PROFILE_0` -- standalone / parent / child / iot.
- `AE_AGENT_KUBERNETES` -- k8s-specific.
- `AE_AGENT_INSTALL_TYPE` -- packaging-specific.
- `AE_AGENT_ACLK` -- cloud-claimed vs not.

See `AE_FIELDS.md` for the full field map and which values
each enum supports.

## Worked examples

### Example 1: index-friendly crash slice

Find recent signal crashes on stable v2.10.x and the latest
2 nightlies:

```json
{
  "after":  -86400,
  "before": 0,
  "last":   500,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_AGENT_HEALTH":  ["crash-first", "crash-loop", "crash-repeated", "crash-entered"],
    "AE_AGENT_VERSION": ["v2.10.0", "v2.10.0-135-nightly", "v2.10.0-130-nightly"]
  },
  "facets": ["AE_FATAL_SIGNAL_CODE", "AE_FATAL_FUNCTION", "AE_HOST_ARCHITECTURE"]
}
```

The `selections` cuts to ~hundreds-to-low-thousands of records
via the facet index; `facets` on top groups by the dimensions
of interest. No FTS needed.

### Example 2: index-friendly + FTS narrower

Find recent crashes whose stack trace mentions a specific
function:

```json
{
  "after":  -86400,
  "before": 0,
  "last":   200,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_AGENT_HEALTH":  ["crash-first", "crash-loop", "crash-repeated", "crash-entered"]
  },
  "query": "rrdcontext_release"
}
```

`selections` cuts to crashes only; `query` does FTS over the
already-sliced subset for the substring match. This is the
right composition: structured first, FTS narrows.

If you can express the function name as a `selections` value
on `AE_FATAL_FUNCTION`, prefer that:

```json
{
  "after":  -86400,
  "before": 0,
  "last":   200,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_FATAL_FUNCTION": ["rrdcontext_release"]
  }
}
```

This is even faster (pure indexed, no FTS).

### Example 3: regression spotter

Compare crash counts across versions:

```json
{
  "after":  -86400,
  "before": 0,
  "last":   1,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_AGENT_HEALTH": ["crash-first", "crash-loop", "crash-repeated", "crash-entered"]
  },
  "facets": ["AE_AGENT_VERSION"],
  "histogram": "AE_AGENT_VERSION"
}
```

`last: 1` because we only need the facet counts, not the rows.
The histogram + facets give the per-version distribution.

### Example 4: rare-crash investigation (wider window)

Looking for a known rare crash signature (1-per-few-days):

```json
{
  "after":  -604800,
  "before": 0,
  "last":   500,
  "__logs_sources": "agent-events",
  "selections": {
    "AE_FATAL_SIGNAL_CODE": ["SIGSEGV/SEGV_MAPERR"],
    "AE_FATAL_FUNCTION":    ["specific_function"]
  }
}
```

7-day window is acceptable here because the `selections`
cuts to specific signal+function -- a sharp index-resolved
slice that returns small results regardless of window width.

## Anti-patterns

### Bare FTS over wide window (BAD)

```json
{
  "after":  -604800,
  "before": 0,
  "__logs_sources": "agent-events",
  "query": "SIGSEGV"
}
```

A 7-day FTS over the full namespace. Slow. ALWAYS pair with
at least one structured `selections` field.

### FTS for things that should be `selections` (BAD)

```json
{
  "query": "SIGSEGV"
}
```

`SIGSEGV/...` is a value in the `AE_FATAL_SIGNAL_CODE` field.
Use:

```json
{
  "selections": {
    "AE_FATAL_SIGNAL_CODE": ["SIGSEGV/SEGV_MAPERR", "SIGSEGV/SEGV_ACCERR", "SIGSEGV/SEGV_BNDERR", "SIGSEGV/SEGV_PKUERR"]
  }
}
```

### Naive equality on `=` only (BAD when you want OR)

The `selections` mechanism gives you OR-of-values for free.
Don't run multiple queries to OR results client-side; put the
values in the array.

### Wide window without version filter (BAD when noisy)

If 40-200k events / day are reported on stable releases, a
30-day window without `AE_AGENT_VERSION` filter returns
millions of records, most from old versions whose bugs are
already fixed. Always pair wide windows with a version slice.

## Discovery first, then query

Use `info=true` to discover what fields and values the agent
currently exposes:

```json
{ "info": true, "__logs_sources": "agent-events" }
```

Or use `last: 1` + a facet to enumerate values:

```json
{
  "after": -86400, "before": 0, "last": 1,
  "__logs_sources": "agent-events",
  "facets": ["AE_AGENT_VERSION"]
}
```

Then build the real query using the discovered values in
`selections`.

## Combining facets + selections

`facets` (the array of field names you want grouped in the
response) and `selections` (the structured filter) are
independent. Common pattern: filter narrowly with
`selections`, then `facets` over the narrow result to see
per-field breakdowns.

```json
{
  "selections": {
    "AE_AGENT_HEALTH":  ["crash-loop"],
    "AE_AGENT_VERSION": ["v2.10.0"]
  },
  "facets": ["AE_FATAL_FUNCTION", "AE_HOST_ARCHITECTURE", "AE_OS_FAMILY"]
}
```

This filters to "crash-loop on v2.10.0", then shows the
distribution by function, architecture, and OS in the result
facets.

## Quick checklist

Before sending any query, verify:

1. `__logs_sources` is set to `"agent-events"` (the journal namespace -- hardcoded constant, NOT the value of `${AGENT_EVENTS_HOSTNAME}`).
2. `selections` contains at least ONE always-present field
   (`AE_AGENT_HEALTH`, `AE_EXIT_CAUSE`, `AE_AGENT_VERSION`).
3. Time window matches the use case (24h default, 7d for rare
   crashes).
4. If you used `query` for FTS, you have a structured
   `selections` slice in front of it.
5. `last` is set sensibly (200-500 for triage; 1 if you only
   want facet counts).
