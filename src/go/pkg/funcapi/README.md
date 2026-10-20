# Collector Function methods

`funcapi` defines collector Function declarations, handlers, selectors and responses.
Job Manager owns publication, job routing, execution and response framing. The
[process boundary](../../plugin/framework/functions/README.md) owns protocol input.

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

## Info for structured methods

For a shared Function with a job selector, structured `info` without an explicit `__job` returns declaration metadata
and the available jobs without calling `MethodParams` or `Handle`. This keeps job discovery available even when a
source cannot be queried. Agent, instance and single-instance structured Functions also retain declaration-only info.

With an explicit `__job`, the framework resolves that running job and calls `MethodParams` under the same cancellation,
availability and containment rules as data requests. It merges the result with declaration parameters and the
framework-owned job selector, then returns metadata only. It does not call `Handle` or validate secondary selector
values: a client may need this metadata to replace a selection that is unsupported by the new job.

`MethodParams` may perform source I/O or use existing collector caches. A nil result preserves declared parameters;
an error fails the metadata request. This exposes the handler's existing discovery semantics, not an atomic snapshot
or a guarantee that the source is ready to return data. Data requests continue to discover and validate parameters
normally.

Clients SHOULD request scoped info when the selected job changes and reconcile dependent selections before querying
data. The JSON schema and transport are unchanged. Existing clients that only request unscoped info keep their
current behavior. Older agents may return only static declarations for scoped info, so clients MUST still handle
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
