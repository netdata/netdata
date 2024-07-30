// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

static int web_client_api_request_v1_alarms_select(char *url) {
    int all = 0;
    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        if(!strcmp(value, "all") || !strcmp(value, "all=true")) all = 1;
        else if(!strcmp(value, "active") || !strcmp(value, "active=true")) all = 0;
    }

    return all;
}

int api_v1_alarms(RRDHOST *host, struct web_client *w, char *url) {
    int all = web_client_api_request_v1_alarms_select(url);

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    health_alarms2json(host, w->response.data, all);
    buffer_no_cacheable(w->response.data);
    return HTTP_RESP_OK;
}

int api_v1_alarms_values(RRDHOST *host, struct web_client *w, char *url) {
    int all = web_client_api_request_v1_alarms_select(url);

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    health_alarms_values2json(host, w->response.data, all);
    buffer_no_cacheable(w->response.data);
    return HTTP_RESP_OK;
}

int api_v1_alarm_count(RRDHOST *host, struct web_client *w, char *url) {
    RRDCALC_STATUS status = RRDCALC_STATUS_RAISED;
    BUFFER *contexts = NULL;

    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "[");

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        netdata_log_debug(D_WEB_CLIENT, "%llu: API v1 alarm_count query param '%s' with value '%s'", w->id, name, value);

        char* p = value;
        if(!strcmp(name, "status")) {
            while ((*p = toupper(*p))) p++;
            if (!strcmp("CRITICAL", value)) status = RRDCALC_STATUS_CRITICAL;
            else if (!strcmp("WARNING", value)) status = RRDCALC_STATUS_WARNING;
            else if (!strcmp("UNINITIALIZED", value)) status = RRDCALC_STATUS_UNINITIALIZED;
            else if (!strcmp("UNDEFINED", value)) status = RRDCALC_STATUS_UNDEFINED;
            else if (!strcmp("REMOVED", value)) status = RRDCALC_STATUS_REMOVED;
            else if (!strcmp("CLEAR", value)) status = RRDCALC_STATUS_CLEAR;
        }
        else if(!strcmp(name, "context") || !strcmp(name, "ctx")) {
            if(!contexts) contexts = buffer_create(255, &netdata_buffers_statistics.buffers_api);
            buffer_strcat(contexts, "|");
            buffer_strcat(contexts, value);
        }
    }

    health_aggregate_alarms(host, w->response.data, contexts, status);

    buffer_sprintf(w->response.data, "]\n");
    w->response.data->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(w->response.data);

    buffer_free(contexts);
    return 200;
}

int api_v1_alarm_log(RRDHOST *host, struct web_client *w, char *url) {
    time_t after = 0;
    char *chart = NULL;

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if (!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if (!strcmp(name, "after")) after = (time_t) strtoul(value, NULL, 0);
        else if (!strcmp(name, "chart")) chart = value;
    }

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    sql_health_alarm_log2json(host, w->response.data, after, chart);
    return HTTP_RESP_OK;
}

int api_v1_variable(RRDHOST *host, struct web_client *w, char *url) {
    int ret = HTTP_RESP_BAD_REQUEST;
    char *chart = NULL;
    char *variable = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = strsep_skip_consecutive_separators(&url, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "variable")) variable = value;
    }

    if(!chart || !*chart || !variable || !*variable) {
        buffer_sprintf(w->response.data, "A chart= and a variable= are required.");
        goto cleanup;
    }

    RRDSET *st = rrdset_find(host, chart);
    if(!st) st = rrdset_find_byname(host, chart);
    if(!st) {
        buffer_strcat(w->response.data, "Chart is not found: ");
        buffer_strcat_htmlescape(w->response.data, chart);
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    w->response.data->content_type = CT_APPLICATION_JSON;
    st->last_accessed_time_s = now_realtime_sec();
    alert_variable_lookup_trace(host, st, variable, w->response.data);

    return HTTP_RESP_OK;

cleanup:
    return ret;
}

int api_v1_alarm_variables(RRDHOST *host, struct web_client *w, char *url) {
    return api_v1_single_chart_helper(host, w, url, health_api_v1_chart_variables2json);
}

