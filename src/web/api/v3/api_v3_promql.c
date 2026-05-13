// SPDX-License-Identifier: GPL-3.0-or-later
//
// /api/v3/promql/* dispatcher.
//
// Phase 0 (SOW-0016): glue between the v3 dispatch table and the Rust
// netdata_promql crate. Parses Prometheus-style query parameters, calls into
// Rust, and writes the JSON response. No real evaluation happens here -- see
// the Rust crate for the placeholder body.
//
// The v3 dispatch table splits paths at the first '/', so this single entry
// handles both `/api/v3/promql/query` and `/api/v3/promql/query_range` by
// inspecting `w->url_path_decoded`. Same pattern as `api_v1_manage`.

#include "api_v3_calls.h"
#include "crates/netdata_promql/nd_promql.h"
#include <stdbool.h>

// Prometheus convention: timestamps are float seconds; we pass int64
// milliseconds to Rust.
static int64_t parse_unix_seconds_to_ms(const char *s, int64_t fallback_ms) {
    if (!s || !*s)
        return fallback_ms;
    char *end = NULL;
    double secs = strtod(s, &end);
    if (end == s)
        return fallback_ms;
    return (int64_t)(secs * 1000.0);
}

// Duration in either Prometheus shorthand (`30s`) or plain seconds. Phase 0
// supports only optional `s` suffix; richer parsing arrives with Phase 1.
static int64_t parse_duration_to_ms(const char *s, int64_t fallback_ms) {
    if (!s || !*s)
        return fallback_ms;
    char *end = NULL;
    double secs = strtod(s, &end);
    if (end == s)
        return fallback_ms;
    if (*end && !(*end == 's' && end[1] == '\0'))
        return fallback_ms;
    return (int64_t)(secs * 1000.0);
}

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

static int handle_instant(struct web_client *w, char *url) {
    char *query = NULL;
    char *time_str = NULL;
    char *timeout_str = NULL;

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value)
            continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name)
            continue;
        if (!value)
            value = "";

        if (!strcmp(name, "query"))
            query = value;
        else if (!strcmp(name, "time"))
            time_str = value;
        else if (!strcmp(name, "timeout"))
            timeout_str = value;
    }

    int64_t at_ms = parse_unix_seconds_to_ms(time_str, (int64_t)(now_realtime_usec() / USEC_PER_MS));
    int64_t timeout_ms = parse_duration_to_ms(timeout_str, 30000);

    struct NdPromqlResponse *resp =
        nd_promql_query_instant(NULL, query, at_ms, timeout_ms);
    return write_response_to_buffer(w, resp);
}

static int handle_range(struct web_client *w, char *url) {
    char *query = NULL;
    char *start_str = NULL;
    char *end_str = NULL;
    char *step_str = NULL;
    char *timeout_str = NULL;

    while (url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value)
            continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if (!name || !*name)
            continue;
        if (!value)
            value = "";

        if (!strcmp(name, "query"))
            query = value;
        else if (!strcmp(name, "start"))
            start_str = value;
        else if (!strcmp(name, "end"))
            end_str = value;
        else if (!strcmp(name, "step"))
            step_str = value;
        else if (!strcmp(name, "timeout"))
            timeout_str = value;
    }

    int64_t now_ms = (int64_t)(now_realtime_usec() / USEC_PER_MS);
    int64_t start_ms = parse_unix_seconds_to_ms(start_str, now_ms - 300000);
    int64_t end_ms = parse_unix_seconds_to_ms(end_str, now_ms);
    int64_t step_ms = parse_duration_to_ms(step_str, 15000);
    int64_t timeout_ms = parse_duration_to_ms(timeout_str, 30000);

    struct NdPromqlResponse *resp =
        nd_promql_query_range(NULL, query, start_ms, end_ms, step_ms, timeout_ms);
    return write_response_to_buffer(w, resp);
}

int api_v3_promql(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    // Distinguish the two endpoints by inspecting the decoded path. The
    // same handler serves both /api/v3/promql/* (the Netdata-namespaced
    // surface) and the Prometheus mirror paths /api/v1/query{,_range}.
    const char *path = buffer_tostring(w->url_path_decoded);
    bool is_range = strstr(path, "promql/query_range") || strstr(path, "/v1/query_range");
    bool is_instant = strstr(path, "promql/query") || strstr(path, "/v1/query");

    if (is_range)
        return handle_range(w, url);
    if (is_instant)
        return handle_instant(w, url);

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data,
                  "{\"status\":\"error\",\"errorType\":\"not_found\","
                  "\"error\":\"unknown promql endpoint; supported: query, query_range\"}");
    w->response.data->content_type = CT_APPLICATION_JSON;
    return HTTP_RESP_NOT_FOUND;
}
