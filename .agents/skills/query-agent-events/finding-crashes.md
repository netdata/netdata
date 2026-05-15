# Finding crashes (signal)

A "crash" = the kernel delivered a fatal signal to the agent
process. The signature is `AE_FATAL_SIGNAL_CODE` non-empty.

## Quick recipe

Get recent signal crashes on stable + recent nightlies:

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --health crash \
    --since '24h ago' \
    --versions auto
```

Output is a JSON dump under
`<repo>/.local/audits/query-agent-events/<timestamp>.json`.

Then aggregate:

```bash
.agents/skills/query-agent-events/scripts/analyze-events.sh \
    --input <bundle.json> \
    --by signal
```

## What "signal crash" means

The agent received a fatal signal (SIGSEGV, SIGBUS, SIGFPE,
SIGABRT, SIGILL, etc.) and could not gracefully recover. The
deadly-signal handler tried to capture context (signal_code,
fault_address, stack_trace) before exiting.

Distinguishing predicates:

- `AE_FATAL_SIGNAL_CODE` non-empty -- definitive marker.
- `AE_AGENT_HEALTH` IN crash-first / crash-loop / crash-repeated
  / crash-entered -- the agent classifies the result.
- `AE_AGENT_EXIT_REASON_*` typically contains
  `signal-segmentation-fault`, `signal-bus-error`,
  `signal-floating-point-exception`, `signal-illegal-instruction`,
  `signal-abort`, etc.

## Index-friendly query for crashes

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
  "facets": ["AE_FATAL_SIGNAL_CODE", "AE_FATAL_FUNCTION", "AE_HOST_ARCHITECTURE", "AE_OS_FAMILY"]
}
```

The auto version filter computes the version list dynamically
(see `update-cadence.md`).

## Triage flow

1. **Get the dump** -- `get-events.sh --health crash`.
2. **Group by signal** -- `analyze-events.sh --by signal` to
   see SIGSEGV vs SIGBUS vs SIGABRT distribution.
3. **Group by function** for the dominant signal --
   `analyze-events.sh --by fatal_function --filter "signal=SIGSEGV/SEGV_MAPERR"`
   (or pre-filter the dump with `jq`).
4. **Pick the dominant function**, look at one
   representative event's stack trace
   (`AE_FATAL_STACK_TRACE`), correlate with source.
5. **Cross-check by version** -- is this on stable only? Just
   nightlies? When did it appear?
6. **Cross-check by environment** -- arch? distro? kubernetes?
   parent vs child? Is the crash environment-specific?
7. **Read source, fix bug.**

## Common signal-code values

(from `<repo>/src/libnetdata/signals/signal-code.c:97-184`,
see `AE_FIELDS.md` for the full table)

| Signal code | What it means |
|---|---|
| `SIGSEGV/SEGV_MAPERR` | NULL pointer / freed memory / unmapped page. |
| `SIGSEGV/SEGV_ACCERR` | Write to read-only / executable page. |
| `SIGBUS/BUS_ADRALN` | Misaligned access (mostly ARM / mmap). |
| `SIGBUS/BUS_OBJERR` | Object-level fault (often disk I/O). |
| `SIGFPE/FPE_INTDIV` | Integer divide by zero. |
| `SIGABRT/SI_TKILL` | abort() / assertion failure. |
| `SIGILL/ILL_ILLOPC` | Illegal instruction (often binary corruption). |
| `SIGTRAP/TRAP_BRKPT` | Breakpoint trap. Usually a debugger; sometimes a deliberate `__builtin_trap()`. |

## Pitfalls

- **Empty stack trace**: `AE_FATAL_STACK_TRACE` may be the
  string `info: will now attempt to get stack trace` or `info: stack trace is not available, libbacktrace reports no frames`
  when the fault was instantaneous (e.g. NULL deref at low
  address). Without the stack, fall back to
  `AE_FATAL_FUNCTION` + `AE_FATAL_FILENAME` + `AE_FATAL_LINE`
  (these are populated from `__FILE__` / `__LINE__` of the
  most recent `fatal()` call -- not the crash site, but
  often nearby).

- **Aborted dumps**: events where `AE_FATAL_SIGNAL_CODE` is
  set but other `AE_FATAL_*` fields are empty -- the signal
  arrived before context capture completed.

- **Shutdown races**: a crash during shutdown reports
  `AE_EXIT_CAUSE = 'killed hard on exit'` or `'killed hard on shutdown'`
  with a stack trace that may show shutdown timing rather
  than the actual crash site. Interpret with caution.

- **Sentry-suppressed**: when `AE_FATAL_SENTRY = true`, the
  agent attempted a Sentry submission. Sentry breakdowns have
  more information; cross-reference if available.

## Filtering out noise

Many crashes from old / unsupported versions are already
fixed. The default `--versions auto` filter handles this.
For wider investigations, scope to stable releases only:

```bash
get-events.sh --health crash --versions '^v2\.\d+\.\d+$'
```

## Related recipes

- `recipes/find-by-function.md` -- when you have a function
  name in mind.
- `recipes/find-by-version.md` -- regression spotter.
- `finding-fatals.md` -- the OTHER class (deliberate exits,
  not signal crashes).
