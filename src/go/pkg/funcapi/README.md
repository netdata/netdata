# Function methods

`funcapi` defines collector and process Function declarations, handlers, selectors and responses.
Job Manager owns publication, job routing, execution and response framing. The
[process boundary](../../plugin/framework/functions/README.md) owns protocol input.

## Accepted selector forms

`ParamOption.Aliases` and `ParamConfig.ValueSeparator` serialize optional discovery metadata for alternate input
forms already accepted by a producer. They do not change `ResolveParam` or enable new parsing automatically.
Advertise only forms the request path actually accepts. Consumers normalizing values against this metadata must match
opaque canonical option IDs before considering aliases or joined-value splitting. Structured JSON `selections` preserve
literal IDs; legacy Go structured Function command arguments split commas before resolution, independently of this metadata.
Use JSON selections for IDs containing commas; see the
[Function contract](../../../plugins.d/FUNCTION_UI_DEVELOPER_GUIDE.md#accepted-option-forms).

## Topology focus metadata

`ParamConfig.Focus` declares optional roots/depth roles for aggregate-first topology focus. `RequiredParam` clones
the descriptor and nested slices so emitted metadata does not share mutable ownership with declarations.
`ValidateFocusParams` checks role uniqueness, canonical sentinel options, depth values and traversal declarations;
managed Function publication and responses reject invalid declarations. Data-bound references are checked by
`topologyv1.ValidateDecodedResponse`, not by metadata-only discovery. The descriptor changes neither parameter
resolution nor direct filtering. Wire semantics belong to the
[topology contract](../../../plugins.d/FUNCTION_TOPOLOGY_DEVELOPER_GUIDE.md#aggregate-first-focus).

## Process Function providers

Use `agent.Config.ProcessFunctions` (or `composition.Config.ProcessFunctions`) for
Functions whose data belongs to the plugin process and must remain queryable when
collection modules are disabled. Each `funcapi.ProcessFunctionProvider` has a stable
`ID`, a `Functions func() []FunctionConfig` callback and a `NewHandler func() MethodHandler`
factory, or a `NewHandlerWithContext func(ProcessFunctionContext) MethodHandler` factory. Exactly one factory is required. Providers require no collector, config schema, discovery provider or scheduled job.
The default public name is `ID:method`; `FunctionName` and `Aliases` retain their normal meaning.

For example, the command composition root can inject an existing history reader.
In this schematic example, `historyfunc` represents your handler package and
`historyReader` represents your process-owned history reader:

```go
agent.Config{
    ProcessFunctions: []funcapi.ProcessFunctionProvider{{
        ID: "history",
        Functions: func() []funcapi.FunctionConfig {
            return []funcapi.FunctionConfig{{ID: "events", RawRequest: true, ManagedInfo: true, HasHistory: true}}
        },
        NewHandler: func() funcapi.MethodHandler { return historyfunc.New(historyReader) },
    }},
}
```

Use the context-aware factory when a process Function needs framework-owned configured vnode identity.
`ProcessFunctionContext.LookupVnodeGUID(name)` resolves the exact configuration name to its canonical GUID,
including vnodes without running jobs or acquired metadata. It reads current committed configuration on every
call; it does not retain removed names or interpret display hostnames as names. The lookup is concurrency-safe,
read-only and owned by that run generation. Handlers MUST NOT close it or retain it beyond Cleanup.
Production composition supplies the lookup; direct controller construction without composition supplies an empty
context. These are selection capabilities, not trusted request host context or per-vnode authorization.

The command MUST provide a fresh handler and declarations for each run generation.
Callbacks run under the existing Function containment authority. Handler `Cleanup`
MUST release only generation-owned resources; it MUST NOT close injected process stores.
The command MUST wait for clean process termination before closing those stores.
A shutdown error means physical callbacks or cleanup may remain; callers MUST retain their
resources or exit the process rather than assume the error established a join barrier.
Restart withdraws old routes and waits for the provider's physical ownership before
constructing its replacement. Noncooperative work retains the existing quarantine and
process-restart-required behavior.

Providers share the existing catalog, dispatcher, publication, access flags, raw requests
and managed metadata. They have no framework `__job` parameter. Duplicate provider IDs
are invalid; providers and collectors MAY share an ID if their public names differ.
Provider primary names and aliases are reserved even while initially unavailable;
collisions with other providers, declared collector Functions or private routes fail
before publication. Later instance declarations are checked against the same reservations.

`Available` uses the existing process tick even without scheduled jobs. Like collector
`AgentFunctions`, it delays the first publication only: after publication, false results
neither withdraw the Function nor prevent dispatch. Handlers own current data availability.

An enabled plugin with providers remains running with zero enabled collectors and skips
collector discovery. Plugin `enabled: false` still disables everything. Omitting providers
preserves the existing empty-module shutdown and collector selection behavior.
`Creator.AgentFunctions` remains appropriate for intentionally module-bound APIs: it
requires its collector module to be selected, but does not require a running job.
`SharedFunctions` and `InstanceFunctions` remain job-backed.

## Input and response ownership

Raw input and raw output are independent choices:

| Declaration / response | Handler | Response owner |
|---|---|---|
| Default | `MethodParams` then `Handle` with resolved selectors | Framework wraps `FunctionResponse` |
| `RawRequest: true` | `HandleRaw` with the complete input | Framework wraps `FunctionResponse` |
| Any handler returning `RawResponse` | As declared | Handler or SDK supplies the complete envelope |

The framework MUST pass `RawResponse` through without adding selectors, history,
presentation or table fields. It ignores the other `FunctionResponse` fields in
that case. Use this for an SDK that already implements the Function protocol.

For managed responses, declaration `RequiredParams` are merged with handler
parameters by ID. Handler entries replace matching entries and append new ones.
Shared Functions on per-job collectors get a framework-owned `__job` selector;
handlers MUST NOT supply it. The framework keeps its own selector if one is supplied.
Agent Functions, instance Functions and single-instance shared Functions have no
framework job selector.

`HasHistory` advertises time-range support in managed info and data responses;
it defaults to false. `AcceptedParams` advertises additional input names, such as
`after`, `before` and `query`. It requires `RawRequest`: structured handlers receive
only resolved selector values and cannot access these extra inputs. The framework
combines those names with required selector IDs without duplicates. These declarations do not implement filtering,
storage, parsing or validation of extra inputs; the handler owns those semantics.

## Reading raw inputs

`ParseRawParams` reads the named inputs of a `RawMethodRequest`. A name takes its value from the first source that
has it: `key:value` or `key=value` arguments, then the payload `selections` object, then top-level payload fields.
A non-empty payload must be a JSON object. The returned `RawParams` provides:

- `String(name)`: one string, or a one-element string array (a single selection); `""` when absent.
- `TimeRange(now)`: `after` and `before` as Unix seconds, nil when absent. Values are integer seconds in years 1-9999.
  As in the [query API](../../../web/api/queries/README.md), `before <= 0` is relative to now and a negative `after`
  is relative to `before` (or to now without `before`).
  `after` greater than `before` is an error.
- `QueryMatcher()`: `query` compiled as a [simple pattern](../matcher/README.md), nil when absent or blank.

Errors are client input errors with messages suitable for a 400 response. Handlers keep their own selector
validation and choose the order of checks. Declare the inputs with `AcceptedParams` using `ParamAfter`,
`ParamBefore` and `ParamQuery`.

## Info for structured methods

For a shared Function with a job selector, structured `info` without an explicit `__job` returns declaration metadata
and the available jobs without calling `MethodParams` or `Handle`. This keeps job discovery available even when a
source cannot be queried. Agent, instance and single-instance structured Functions retain declaration-only info
by default.

With an explicit `__job`, the framework resolves that running job and calls `MethodParams` under the same cancellation,
availability and containment rules as data requests. It merges the result with declaration parameters and the
framework-owned job selector, then returns metadata only. It does not call `Handle` or validate secondary selector
values: a client may need this metadata to replace a selection that is unsupported by the new job.

A structured method MAY set `DynamicInfo: true` to use `MethodParams` for bound info as well, under the same
containment and cancellation rules. Job-backed bound methods fall back to declaration-only info when no running,
available job exists; an error from an active handler is returned, not hidden by that fallback. Unselected shared
multi-job info stays static even when this flag is enabled. `DynamicInfo` is not valid with `RawRequest`; raw methods
use `ManagedInfo` instead. Methods exposing cached choices should keep discovery free of collection or rendering;
for example, SNMP topology reads its cached managed-device targets for info without building the graph.

`MethodParams` may perform source I/O or use existing collector caches. A nil result preserves declared parameters;
an error fails the metadata request. This exposes the handler's existing discovery semantics, not an atomic snapshot
or a guarantee that the source is ready to return data. Data requests continue to discover and validate parameters
normally.

Clients SHOULD request scoped info when the selected job changes and reconcile dependent selections before querying
data. The JSON schema and transport are unchanged. Unselected multi-job bootstrap remains declaration-only;
opted-in bound info may include additional cached choices without a new client request. Older agents may return
only static declarations for scoped or bound info, so clients MUST still handle
data-request errors without presenting results from the previous job as current.

## Info for payload-aware methods

Existing raw-input methods receive `info` in `HandleRaw`.

A raw-input method MAY set `ManagedInfo: true` to use framework-managed bootstrap
and scoped metadata. `ManagedInfo` requires `RawRequest`.

1. For a shared Function with a job selector, `info` without a selected `__job`
   returns declaration metadata and the available jobs. It does not invoke a
   handler, so an unavailable source cannot prevent choosing another job.
2. The client sends `info` again with an explicit `__job`. The framework resolves
   that job and invokes `HandleRaw` with `Info: true`, preserving the original
   arguments and payload. The handler returns domain selectors, such as service
   choices, in `FunctionResponse.RequiredParams`.
3. The framework merges those selectors with the declaration and job selector,
   adds declared history and accepted inputs, and emits metadata only. Columns,
   rows and chart data are omitted from managed info; declared presentation is
   included. Help uses the handler override when supplied, otherwise the declaration
   or generated default. Errors and complete `RawResponse` envelopes retain their normal paths.

For bound methods without a job selector, opted-in `info` invokes their handler
directly. Data requests retain normal routing, including the default job when
`__job` is omitted. Selecting a job does not bypass cancellation, containment or
job availability checks.

For example, a payload-aware method can declare:

```go
funcapi.FunctionConfig{
    ID:             "events",
    Help:           "Query source events",
    RawRequest:     true,
    ManagedInfo:    true,
    HasHistory:     true,
    AcceptedParams: []string{"after", "before", "query"},
}
```

Its `HandleRaw` supplies source-specific metadata when `req.Info` is true and
query results otherwise. It SHOULD return ordinary `FunctionResponse` values
when using the managed contract; it need not construct framework metadata.

Response schemas are owned by
[`FUNCTION_UI_SCHEMA.json`](../../../plugins.d/FUNCTION_UI_SCHEMA.json).
