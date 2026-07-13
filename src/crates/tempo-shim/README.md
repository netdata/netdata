# Grafana for Netdata traces (Tempo API shim)

The Netdata Agent can serve its stored OpenTelemetry traces to Grafana
through a **Tempo-compatible HTTP API**: point a stock Grafana *Tempo*
datasource at the Agent and use the Search tab (filter builder + tag
autocomplete) and the trace waterfall as usual.

> **Temporary scaffolding.** This surface exists to make stored traces
> visible while Netdata's own trace views are being designed; expect it
> to be replaced. It implements the Grafana search form's filter
> grammar — not the full TraceQL language (pipelines, structural
> operators, aggregates, and the `/api/metrics/*` endpoints return
> clear errors).

## Enable

In `otel.yaml`, under the `traces:` section:

```yaml
traces:
  tempo:
    enabled: true
    bind: "127.0.0.1:3200"
```

The listener starts with the otel plugin. **There is no
authentication** — the default bind is localhost; only expose it on a
network path you trust (an SSH tunnel or a reverse proxy with auth are
the usual choices).

## Point Grafana at it

1. Grafana → Connections → Data sources → Add → **Tempo**.
2. URL: `http://<agent-host>:3200`.
3. Leave **gRPC streaming off** (it is off by default; the shim serves
   the plain HTTP API only).
4. Save & test.

Verified end-to-end against Grafana **11.6**. Grafana 12.x's redesigned
Explore traces table was observed crashing while rendering its own
backend's search frames (a Grafana-side regression, not specific to
this shim); if the Search results table shows "An unexpected error
happened", use a Grafana 11.x until upstream fixes it.

The Search tab's filter builder, tag/value autocomplete, duration and
multi-value filters, and the waterfall view (click a search result) all
work. Anything typed into the raw TraceQL editor beyond the form's
filter grammar returns HTTP 400; Grafana shows the failed status and
the reason is in the response body (visible with `curl`).

## Endpoints served

| Endpoint | Purpose |
|---|---|
| `GET /api/echo` | health ("Save & test") |
| `GET /api/v2/traces/{id}` / `GET /api/traces/{id}` | trace by id (protobuf) |
| `GET /api/search` | trace search (strict-jsonpb JSON) |
| `GET /api/v2/search/tags` | tag autocomplete |
| `GET /api/v2/search/tag/{tag}/values` | tag-value autocomplete |

Notes:

- Results that are incomplete (a work ceiling, a source failure, a
  size cap) are still served; the response carries an
  `X-Netdata-Partial: <reasons>` header and the Agent logs a line —
  Tempo's search/tags wire formats have no status field to carry it.
- Trace-by-id ignores the optional `start`/`end` hints and always
  answers from full local retention (per-file bloom filters keep that
  cheap); honoring the hints could silently drop spans of the
  requested trace.
- Queries serve the default tenant.
