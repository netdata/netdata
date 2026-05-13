// SPDX-License-Identifier: GPL-3.0-or-later
//
// Phase 2 of SOW-0018: discovery endpoints that Grafana's Prometheus
// datasource probes for UI features (metric browser, label autocomplete,
// datasource health-check).
//
// Five Prometheus-shaped endpoints land here:
//   /api/v1/labels                     -- union of label names
//   /api/v1/label/<name>/values        -- distinct values of one label
//   /api/v1/series                     -- full label maps per series
//   /api/v1/metadata                   -- per-metric TYPE/HELP/unit
//   /api/v1/status/buildinfo           -- static version string for heuristics
//
// All five route through the same callback registered in `web_api_v1.c`;
// this file inspects `w->url_path_decoded` to dispatch. The first four call
// into the Rust crate for parsing+resolution+serialization; buildinfo is a
// static JSON literal.
//
// Parameter parsing details that matter for Grafana compatibility:
//   * `match[]` repeats. Each occurrence carries one PromQL selector; the
//     handler collects them into an array and passes that array through
//     the Rust FFI. The Rust side OR-unions the resolved series by
//     signature.
//   * `limit` is forwarded; the Rust side caps it at `max_series=10000`
//     and emits a Prometheus-style `warnings` envelope field on truncation.
//   * `start`/`end` are accepted but currently ignored (Phase 1 sample
//     iteration already runs over a default lookback window). Phase 2
//     keeps the API surface complete so Grafana sees the expected param
//     set.
//   * POST is accepted on `/labels` and `/series` with
//     `application/x-www-form-urlencoded` bodies. This mirrors the Phase 1
//     POST shim and is required because `httpMethod=POST` is Grafana's
//     modern default for the discovery endpoints.

#include "api_v3_calls.h"
#include "crates/netdata_promql/nd_promql.h"
#include <stdbool.h>

#define MAX_MATCHERS 32

// ---------------------------------------------------------------------------
// Parameter parsing
// ---------------------------------------------------------------------------

// Decode one URL-encoded query parameter value in-place. We do this lazily
// per-value rather than once per URL because Phase 1 already URL-decodes
// the request as a whole; only the form-encoded POST body needs additional
// decoding, and we handle that on entry.

typedef struct discovery_params {
    const char *matchers[MAX_MATCHERS];
    size_t matchers_len;
    const char *label_name;       // for /api/v1/label/<name>/values
    const char *metric_filter;    // for /api/v1/metadata
    const char *host;             // ?host=... (NULL = localhost, "*" = all)
    int64_t start_ms;
    int64_t end_ms;
    size_t limit;
    bool overflow;                // too many match[] values; drop extras
} discovery_params;

static int64_t parse_unix_seconds_to_ms(const char *s, int64_t fallback_ms) {
    if (!s || !*s)
        return fallback_ms;
    char *end = NULL;
    double secs = strtod(s, &end);
    if (end == s)
        return fallback_ms;
    return (int64_t)(secs * 1000.0);
}

// Parse a `name=value&name=value&...` string into the params struct, in
// place. The input buffer is mutated; tokens are returned as pointers into
// it. Multiple occurrences of `match[]` are appended to `matchers`. Other
// keys take the last value (Grafana never sends repeats for them).
static void parse_query_string(char *url, discovery_params *out) {
    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value)
            continue;
        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name)
            continue;
        if (!value)
            value = "";

        if (!strcmp(name, "match[]")) {
            if (out->matchers_len >= MAX_MATCHERS) {
                out->overflow = true;
                continue;
            }
            out->matchers[out->matchers_len++] = value;
        } else if (!strcmp(name, "match")) {
            // Some clients send `match=` without the brackets; accept both.
            if (out->matchers_len >= MAX_MATCHERS) {
                out->overflow = true;
                continue;
            }
            out->matchers[out->matchers_len++] = value;
        } else if (!strcmp(name, "metric"))
            out->metric_filter = value;
        else if (!strcmp(name, "host"))
            out->host = value;
        else if (!strcmp(name, "start"))
            out->start_ms = parse_unix_seconds_to_ms(value, 0);
        else if (!strcmp(name, "end"))
            out->end_ms = parse_unix_seconds_to_ms(value, 0);
        else if (!strcmp(name, "limit")) {
            long long v = strtoll(value, NULL, 10);
            if (v > 0)
                out->limit = (size_t)v;
        }
        // Unknown keys silently ignored.
    }
}

// Concatenate URL params + form body so both contribute. URL takes
// precedence on conflicts because that's the Phase 1 convention.
static char *combined_params(struct web_client *w, char *url, char *buf, size_t buf_size) {
    bool have_url = url && *url;
    bool have_body = w->payload && buffer_strlen(w->payload) > 0;
    if (have_url && !have_body)
        return url;
    if (!have_url && !have_body)
        return NULL;
    if (!have_url) {
        url_decode_r(buf, buffer_tostring(w->payload), buf_size);
        buf[buf_size - 1] = '\0';
        return buf;
    }
    // Both: decode body first, then append URL params (URL wins on
    // duplicate-key lookups via "last write wins" in parse, so we put it
    // last).
    url_decode_r(buf, buffer_tostring(w->payload), buf_size);
    buf[buf_size - 1] = '\0';
    size_t body_len = strlen(buf);
    if (body_len + 1 < buf_size) {
        buf[body_len] = '&';
        size_t url_len = strlen(url);
        if (body_len + 1 + url_len < buf_size) {
            memcpy(buf + body_len + 1, url, url_len + 1);
        }
    }
    return buf;
}

// ---------------------------------------------------------------------------
// Response writers
// ---------------------------------------------------------------------------

static int write_response_to_buffer(struct web_client *w, struct NdPromqlResponse *resp) {
    buffer_flush(w->response.data);
    if (!resp) {
        buffer_strcat(w->response.data,
                      "{\"status\":\"error\",\"errorType\":\"internal\",\"error\":\"null response\"}");
        w->response.data->content_type = CT_APPLICATION_JSON;
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }
    const char *body = nd_promql_response_body(resp);
    if (body)
        buffer_strcat(w->response.data, body);
    w->response.data->content_type = CT_APPLICATION_JSON;
    int status = nd_promql_response_http_status(resp);
    nd_promql_response_free(resp);
    return status;
}

// ---------------------------------------------------------------------------
// /api/v1/status/buildinfo (static JSON)
// ---------------------------------------------------------------------------
//
// Grafana's Go backend hits this endpoint to classify the datasource as
// Prometheus vs Mimir. The version string is not used for frontend feature
// gating (the frontend reads `jsonData.prometheusVersion` set in the
// datasource UI), but it is logged and shown to operators. We return
// Prometheus `2.45.0` as a familiar reference value; what controls
// classification is the *empty* `features` map (Mimir reports a non-empty
// map of feature flags).

static int handle_buildinfo(struct web_client *w) {
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data,
        "{\"status\":\"success\",\"data\":{"
        "\"version\":\"2.45.0\","
        "\"revision\":\"netdata\","
        "\"branch\":\"netdata\","
        "\"buildUser\":\"netdata\","
        "\"buildDate\":\"\","
        "\"goVersion\":\"\","
        "\"features\":{}"
        "}}");
    w->response.data->content_type = CT_APPLICATION_JSON;
    return HTTP_RESP_OK;
}

// ---------------------------------------------------------------------------
// Endpoint handlers
// ---------------------------------------------------------------------------

static int handle_labels(struct web_client *w, char *params) {
    discovery_params p = {0};
    parse_query_string(params, &p);
    struct NdPromqlResponse *resp =
        nd_promql_labels(p.host,
                         p.matchers_len ? p.matchers : NULL,
                         p.matchers_len,
                         p.start_ms,
                         p.end_ms,
                         p.limit);
    return write_response_to_buffer(w, resp);
}

static int handle_label_values(struct web_client *w, char *params, const char *label_name) {
    discovery_params p = {0};
    parse_query_string(params, &p);
    if (!label_name || !*label_name) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data,
            "{\"status\":\"error\",\"errorType\":\"bad_data\","
            "\"error\":\"label name is required in path: /api/v1/label/<name>/values\"}");
        w->response.data->content_type = CT_APPLICATION_JSON;
        return HTTP_RESP_BAD_REQUEST;
    }
    struct NdPromqlResponse *resp =
        nd_promql_label_values(p.host,
                               label_name,
                               p.matchers_len ? p.matchers : NULL,
                               p.matchers_len,
                               p.start_ms,
                               p.end_ms,
                               p.limit);
    return write_response_to_buffer(w, resp);
}

static int handle_series(struct web_client *w, char *params) {
    discovery_params p = {0};
    parse_query_string(params, &p);
    struct NdPromqlResponse *resp =
        nd_promql_series(p.host,
                         p.matchers_len ? p.matchers : NULL,
                         p.matchers_len,
                         p.start_ms,
                         p.end_ms,
                         p.limit);
    return write_response_to_buffer(w, resp);
}

static int handle_metadata(struct web_client *w, char *params) {
    discovery_params p = {0};
    parse_query_string(params, &p);
    struct NdPromqlResponse *resp =
        nd_promql_metadata(p.host,
                           p.metric_filter,
                           p.limit);
    return write_response_to_buffer(w, resp);
}

// ---------------------------------------------------------------------------
// Path routing
// ---------------------------------------------------------------------------

// Extract `<name>` from `/api/v1/label/<name>/values`. Returns NULL on a
// malformed path. The string returned points into the decoded URL buffer
// that the dispatcher owns.
static const char *extract_label_name(const char *path) {
    if (!path)
        return NULL;
    const char *p = strstr(path, "/label/");
    if (!p)
        return NULL;
    p += strlen("/label/");
    // The dispatcher already split off the leading `/api/v1`, but tolerate
    // either shape.
    if (!*p)
        return NULL;
    // Caller will tokenize; we return the raw pointer here and check the
    // separator at dispatch time.
    return p;
}

int api_v1_promql_discovery(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    const char *path = buffer_tostring(w->url_path_decoded);

    char params_buf[NETDATA_WEB_REQUEST_URL_SIZE + 2];
    char *params = combined_params(w, url, params_buf, sizeof(params_buf));
    if (!params)
        params = "";

    // Order matters: longer prefixes first. `label/` vs `labels` would
    // collide otherwise.
    if (strstr(path, "/v1/status/buildinfo"))
        return handle_buildinfo(w);

    if (strstr(path, "/v1/label/")) {
        // /api/v1/label/<name>/values
        const char *p = extract_label_name(path);
        if (!p)
            goto not_found;
        // Slice <name> at the next '/' on a stack buffer so we don't mutate
        // the decoded URL.
        char namebuf[256];
        size_t i = 0;
        while (*p && *p != '/' && i + 1 < sizeof(namebuf))
            namebuf[i++] = *p++;
        namebuf[i] = '\0';
        if (i == 0 || strcmp(p, "/values") != 0)
            goto not_found;
        return handle_label_values(w, params, namebuf);
    }

    if (strstr(path, "/v1/labels"))
        return handle_labels(w, params);

    if (strstr(path, "/v1/series"))
        return handle_series(w, params);

    if (strstr(path, "/v1/metadata"))
        return handle_metadata(w, params);

not_found:
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data,
        "{\"status\":\"error\",\"errorType\":\"not_found\","
        "\"error\":\"unknown discovery endpoint\"}");
    w->response.data->content_type = CT_APPLICATION_JSON;
    return HTTP_RESP_NOT_FOUND;
}
