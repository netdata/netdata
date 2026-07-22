// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

int api_v1_single_chart_helper(RRDHOST *host, struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf)) {
    int ret = HTTP_RESP_BAD_REQUEST;
    char *chart = NULL;
    RRDSET_ACQUIRED *rsa = NULL;

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
        //else {
        /// buffer_sprintf(w->response.data, "Unknown parameter '%s' in request.", name);
        //  goto cleanup;
        //}
    }

    if(!chart || !*chart) {
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    rsa = rrdset_find_and_acquire(host, chart, false);
    if(!rsa) rsa = rrdset_find_byname_and_acquire(host, chart);

    RRDSET *st = rrdset_acquired_to_rrdset(rsa);
    if(!st) {
        buffer_strcat(w->response.data, "Chart is not found: ");
        buffer_strcat_htmlescape(w->response.data, chart);
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    w->response.data->content_type = CT_APPLICATION_JSON;
    rrdset_touch_last_accessed_time_s(st);
    callback(st, w->response.data);
    ret = HTTP_RESP_OK;

cleanup:
    rrdset_acquired_release(rsa);
    return ret;
}

int api_v1_charts(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    charts2json(host, w->response.data);
    return HTTP_RESP_OK;
}

int api_v1_chart(RRDHOST *host, struct web_client *w, char *url) {
    return api_v1_single_chart_helper(host, w, url, rrd_stats_api_v1_chart);
}

