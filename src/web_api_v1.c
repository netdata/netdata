// SPDX-License-Identifier: GPL-3.0+
#include "common.h"

static struct {
    const char *name;
    uint32_t hash;
    int value;
} api_v1_data_groups[] = {
        {  "average"        , 0    , GROUP_AVERAGE}
        , {"min"            , 0    , GROUP_MIN}
        , {"max"            , 0    , GROUP_MAX}
        , {"sum"            , 0    , GROUP_SUM}
        , {"incremental_sum", 0    , GROUP_INCREMENTAL_SUM}
        , {"incremental-sum", 0    , GROUP_INCREMENTAL_SUM}
        , {                 NULL, 0, 0}
};

static struct {
    const char *name;
    uint32_t hash;
    uint32_t value;
} api_v1_data_options[] = {
        {  "nonzero"         , 0    , RRDR_OPTION_NONZERO}
        , {"flip"            , 0    , RRDR_OPTION_REVERSED}
        , {"reversed"        , 0    , RRDR_OPTION_REVERSED}
        , {"reverse"         , 0    , RRDR_OPTION_REVERSED}
        , {"jsonwrap"        , 0    , RRDR_OPTION_JSON_WRAP}
        , {"min2max"         , 0    , RRDR_OPTION_MIN2MAX}
        , {"ms"              , 0    , RRDR_OPTION_MILLISECONDS}
        , {"milliseconds"    , 0    , RRDR_OPTION_MILLISECONDS}
        , {"abs"             , 0    , RRDR_OPTION_ABSOLUTE}
        , {"absolute"        , 0    , RRDR_OPTION_ABSOLUTE}
        , {"absolute_sum"    , 0    , RRDR_OPTION_ABSOLUTE}
        , {"absolute-sum"    , 0    , RRDR_OPTION_ABSOLUTE}
        , {"display_absolute", 0    , RRDR_OPTION_DISPLAY_ABS}
        , {"display-absolute", 0    , RRDR_OPTION_DISPLAY_ABS}
        , {"seconds"         , 0    , RRDR_OPTION_SECONDS}
        , {"null2zero"       , 0    , RRDR_OPTION_NULL2ZERO}
        , {"objectrows"      , 0    , RRDR_OPTION_OBJECTSROWS}
        , {"google_json"     , 0    , RRDR_OPTION_GOOGLE_JSON}
        , {"google-json"     , 0    , RRDR_OPTION_GOOGLE_JSON}
        , {"percentage"      , 0    , RRDR_OPTION_PERCENTAGE}
        , {"unaligned"       , 0    , RRDR_OPTION_NOT_ALIGNED}
        , {"match_ids"       , 0    , RRDR_OPTION_MATCH_IDS}
        , {"match-ids"       , 0    , RRDR_OPTION_MATCH_IDS}
        , {"match_names"     , 0    , RRDR_OPTION_MATCH_NAMES}
        , {"match-names"     , 0    , RRDR_OPTION_MATCH_NAMES}
        , {                  NULL, 0, 0}
};

static struct {
    const char *name;
    uint32_t hash;
    uint32_t value;
} api_v1_data_formats[] = {
        {  DATASOURCE_FORMAT_DATATABLE_JSON , 0 , DATASOURCE_DATATABLE_JSON}
        , {DATASOURCE_FORMAT_DATATABLE_JSONP, 0 , DATASOURCE_DATATABLE_JSONP}
        , {DATASOURCE_FORMAT_JSON           , 0 , DATASOURCE_JSON}
        , {DATASOURCE_FORMAT_JSONP          , 0 , DATASOURCE_JSONP}
        , {DATASOURCE_FORMAT_SSV            , 0 , DATASOURCE_SSV}
        , {DATASOURCE_FORMAT_CSV            , 0 , DATASOURCE_CSV}
        , {DATASOURCE_FORMAT_TSV            , 0 , DATASOURCE_TSV}
        , {"tsv-excel"                      , 0 , DATASOURCE_TSV}
        , {DATASOURCE_FORMAT_HTML           , 0 , DATASOURCE_HTML}
        , {DATASOURCE_FORMAT_JS_ARRAY       , 0 , DATASOURCE_JS_ARRAY}
        , {DATASOURCE_FORMAT_SSV_COMMA      , 0 , DATASOURCE_SSV_COMMA}
        , {DATASOURCE_FORMAT_CSV_JSON_ARRAY , 0 , DATASOURCE_CSV_JSON_ARRAY}
        , {                                 NULL, 0, 0}
};

static struct {
    const char *name;
    uint32_t hash;
    uint32_t value;
} api_v1_data_google_formats[] = {
        // this is not error - when google requests json, it expects javascript
        // https://developers.google.com/chart/interactive/docs/dev/implementing_data_source#responseformat
        {  "json"     , 0    , DATASOURCE_DATATABLE_JSONP}
        , {"html"     , 0    , DATASOURCE_HTML}
        , {"csv"      , 0    , DATASOURCE_CSV}
        , {"tsv-excel", 0    , DATASOURCE_TSV}
        , {           NULL, 0, 0}
};

void web_client_api_v1_init(void) {
    int i;

    for(i = 0; api_v1_data_groups[i].name ; i++)
        api_v1_data_groups[i].hash = simple_hash(api_v1_data_groups[i].name);

    for(i = 0; api_v1_data_options[i].name ; i++)
        api_v1_data_options[i].hash = simple_hash(api_v1_data_options[i].name);

    for(i = 0; api_v1_data_formats[i].name ; i++)
        api_v1_data_formats[i].hash = simple_hash(api_v1_data_formats[i].name);

    for(i = 0; api_v1_data_google_formats[i].name ; i++)
        api_v1_data_google_formats[i].hash = simple_hash(api_v1_data_google_formats[i].name);
}

inline int web_client_api_request_v1_data_group(char *name, int def) {
    int i;

    uint32_t hash = simple_hash(name);
    for(i = 0; api_v1_data_groups[i].name ; i++)
        if(unlikely(hash == api_v1_data_groups[i].hash && !strcmp(name, api_v1_data_groups[i].name)))
            return api_v1_data_groups[i].value;

    return def;
}

inline uint32_t web_client_api_request_v1_data_options(char *o) {
    uint32_t ret = 0x00000000;
    char *tok;

    while(o && *o && (tok = mystrsep(&o, ", |"))) {
        if(!*tok) continue;

        uint32_t hash = simple_hash(tok);
        int i;
        for(i = 0; api_v1_data_options[i].name ; i++) {
            if (unlikely(hash == api_v1_data_options[i].hash && !strcmp(tok, api_v1_data_options[i].name))) {
                ret |= api_v1_data_options[i].value;
                break;
            }
        }
    }

    return ret;
}

inline uint32_t web_client_api_request_v1_data_format(char *name) {
    uint32_t hash = simple_hash(name);
    int i;

    for(i = 0; api_v1_data_formats[i].name ; i++) {
        if (unlikely(hash == api_v1_data_formats[i].hash && !strcmp(name, api_v1_data_formats[i].name))) {
            return api_v1_data_formats[i].value;
        }
    }

    return DATASOURCE_JSON;
}

inline uint32_t web_client_api_request_v1_data_google_format(char *name) {
    uint32_t hash = simple_hash(name);
    int i;

    for(i = 0; api_v1_data_google_formats[i].name ; i++) {
        if (unlikely(hash == api_v1_data_google_formats[i].hash && !strcmp(name, api_v1_data_google_formats[i].name))) {
            return api_v1_data_google_formats[i].value;
        }
    }

    return DATASOURCE_JSON;
}


inline int web_client_api_request_v1_alarms(RRDHOST *host, struct web_client *w, char *url) {
    int all = 0;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        if(!strcmp(value, "all")) all = 1;
        else if(!strcmp(value, "active")) all = 0;
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarms2json(host, w->response.data, all);
    return 200;
}

inline int web_client_api_request_v1_alarm_log(RRDHOST *host, struct web_client *w, char *url) {
    uint32_t after = 0;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "after")) after = (uint32_t)strtoul(value, NULL, 0);
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarm_log2json(host, w->response.data, after);
    return 200;
}

inline int web_client_api_request_single_chart(RRDHOST *host, struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf)) {
    int ret = 400;
    char *chart = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = mystrsep(&url, "?&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
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

    RRDSET *st = rrdset_find(host, chart);
    if(!st) st = rrdset_find_byname(host, chart);
    if(!st) {
        buffer_strcat(w->response.data, "Chart is not found: ");
        buffer_strcat_htmlescape(w->response.data, chart);
        ret = 404;
        goto cleanup;
    }

    w->response.data->contenttype = CT_APPLICATION_JSON;
    st->last_accessed_time = now_realtime_sec();
    callback(st, w->response.data);
    return 200;

    cleanup:
    return ret;
}

inline int web_client_api_request_v1_alarm_variables(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_single_chart(host, w, url, health_api_v1_chart_variables2json);
}

inline int web_client_api_request_v1_charts(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    rrd_stats_api_v1_charts(host, w->response.data);
    return 200;
}

struct prometheus_output_options {
    char *name;
    PROMETHEUS_OUTPUT_OPTIONS flag;
} prometheus_output_flags_root[] = {
        { "help",       PROMETHEUS_OUTPUT_HELP       },
        { "types",      PROMETHEUS_OUTPUT_TYPES      },
        { "names",      PROMETHEUS_OUTPUT_NAMES      },
        { "timestamps", PROMETHEUS_OUTPUT_TIMESTAMPS },
        { "variables",  PROMETHEUS_OUTPUT_VARIABLES  },

        // terminator
        { NULL, PROMETHEUS_OUTPUT_NONE },
};

inline int web_client_api_request_v1_allmetrics(RRDHOST *host, struct web_client *w, char *url) {
    int format = ALLMETRICS_SHELL;
    const char *prometheus_server = w->client_ip;
    uint32_t prometheus_backend_options = global_backend_options;
    PROMETHEUS_OUTPUT_OPTIONS prometheus_output_options = PROMETHEUS_OUTPUT_TIMESTAMPS | ((global_backend_options & BACKEND_OPTION_SEND_NAMES)?PROMETHEUS_OUTPUT_NAMES:0);
    const char *prometheus_prefix = global_backend_prefix;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "format")) {
            if(!strcmp(value, ALLMETRICS_FORMAT_SHELL))
                format = ALLMETRICS_SHELL;
            else if(!strcmp(value, ALLMETRICS_FORMAT_PROMETHEUS))
                format = ALLMETRICS_PROMETHEUS;
            else if(!strcmp(value, ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS))
                format = ALLMETRICS_PROMETHEUS_ALL_HOSTS;
            else if(!strcmp(value, ALLMETRICS_FORMAT_JSON))
                format = ALLMETRICS_JSON;
            else
                format = 0;
        }
        else if(!strcmp(name, "server")) {
            prometheus_server = value;
        }
        else if(!strcmp(name, "prefix")) {
            prometheus_prefix = value;
        }
        else if(!strcmp(name, "data") || !strcmp(name, "source") || !strcmp(name, "data source") || !strcmp(name, "data-source") || !strcmp(name, "data_source") || !strcmp(name, "datasource")) {
            prometheus_backend_options = backend_parse_data_source(value, prometheus_backend_options);
        }
        else {
            int i;
            for(i = 0; prometheus_output_flags_root[i].name ; i++) {
                if(!strcmp(name, prometheus_output_flags_root[i].name)) {
                    if(!strcmp(value, "yes") || !strcmp(value, "1") || !strcmp(value, "true"))
                        prometheus_output_options |= prometheus_output_flags_root[i].flag;
                    else
                        prometheus_output_options &= ~prometheus_output_flags_root[i].flag;

                    break;
                }
            }
        }
    }

    buffer_flush(w->response.data);
    buffer_no_cacheable(w->response.data);

    switch(format) {
        case ALLMETRICS_JSON:
            w->response.data->contenttype = CT_APPLICATION_JSON;
            rrd_stats_api_v1_charts_allmetrics_json(host, w->response.data);
            return 200;

        case ALLMETRICS_SHELL:
            w->response.data->contenttype = CT_TEXT_PLAIN;
            rrd_stats_api_v1_charts_allmetrics_shell(host, w->response.data);
            return 200;

        case ALLMETRICS_PROMETHEUS:
            w->response.data->contenttype = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(host, w->response.data, prometheus_server, prometheus_prefix, prometheus_backend_options, prometheus_output_options);
            return 200;

        case ALLMETRICS_PROMETHEUS_ALL_HOSTS:
            w->response.data->contenttype = CT_PROMETHEUS;
            rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(host, w->response.data, prometheus_server, prometheus_prefix, prometheus_backend_options, prometheus_backend_options);
            return 200;

        default:
            w->response.data->contenttype = CT_TEXT_PLAIN;
            buffer_strcat(w->response.data, "Which format? '" ALLMETRICS_FORMAT_SHELL "', '" ALLMETRICS_FORMAT_PROMETHEUS "', '" ALLMETRICS_FORMAT_PROMETHEUS_ALL_HOSTS "' and '" ALLMETRICS_FORMAT_JSON "' are currently supported.");
            return 400;
    }
}

inline int web_client_api_request_v1_chart(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_single_chart(host, w, url, rrd_stats_api_v1_chart);
}

int web_client_api_request_v1_badge(RRDHOST *host, struct web_client *w, char *url) {
    int ret = 400;
    buffer_flush(w->response.data);

    BUFFER *dimensions = NULL;

    const char *chart = NULL
    , *before_str = NULL
    , *after_str = NULL
    , *points_str = NULL
    , *multiply_str = NULL
    , *divide_str = NULL
    , *label = NULL
    , *units = NULL
    , *label_color = NULL
    , *value_color = NULL
    , *refresh_str = NULL
    , *precision_str = NULL
    , *scale_str = NULL
    , *alarm = NULL;

    int group = GROUP_AVERAGE;
    uint32_t options = 0x00000000;

    while(url) {
        char *value = mystrsep(&url, "/?&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 badge.svg query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions)
                dimensions = buffer_create(100);

            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "group")) {
            group = web_client_api_request_v1_data_group(value, GROUP_AVERAGE);
        }
        else if(!strcmp(name, "options")) {
            options |= web_client_api_request_v1_data_options(value);
        }
        else if(!strcmp(name, "label")) label = value;
        else if(!strcmp(name, "units")) units = value;
        else if(!strcmp(name, "label_color")) label_color = value;
        else if(!strcmp(name, "value_color")) value_color = value;
        else if(!strcmp(name, "multiply")) multiply_str = value;
        else if(!strcmp(name, "divide")) divide_str = value;
        else if(!strcmp(name, "refresh")) refresh_str = value;
        else if(!strcmp(name, "precision")) precision_str = value;
        else if(!strcmp(name, "scale")) scale_str = value;
        else if(!strcmp(name, "alarm")) alarm = value;
    }

    if(!chart || !*chart) {
        buffer_no_cacheable(w->response.data);
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    int scale = (scale_str && *scale_str)?str2i(scale_str):100;

    RRDSET *st = rrdset_find(host, chart);
    if(!st) st = rrdset_find_byname(host, chart);
    if(!st) {
        buffer_no_cacheable(w->response.data);
        buffer_svg(w->response.data, "chart not found", NAN, "", NULL, NULL, -1, scale, 0);
        ret = 200;
        goto cleanup;
    }
    st->last_accessed_time = now_realtime_sec();

    RRDCALC *rc = NULL;
    if(alarm) {
        rc = rrdcalc_find(st, alarm);
        if (!rc) {
            buffer_no_cacheable(w->response.data);
            buffer_svg(w->response.data, "alarm not found", NAN, "", NULL, NULL, -1, scale, 0);
            ret = 200;
            goto cleanup;
        }
    }

    long long multiply  = (multiply_str  && *multiply_str )?str2l(multiply_str):1;
    long long divide    = (divide_str    && *divide_str   )?str2l(divide_str):1;
    long long before    = (before_str    && *before_str   )?str2l(before_str):0;
    long long after     = (after_str     && *after_str    )?str2l(after_str):-st->update_every;
    int       points    = (points_str    && *points_str   )?str2i(points_str):1;
    int       precision = (precision_str && *precision_str)?str2i(precision_str):-1;

    if(!multiply) multiply = 1;
    if(!divide) divide = 1;

    int refresh = 0;
    if(refresh_str && *refresh_str) {
        if(!strcmp(refresh_str, "auto")) {
            if(rc) refresh = rc->update_every;
            else if(options & RRDR_OPTION_NOT_ALIGNED)
                refresh = st->update_every;
            else {
                refresh = (int)(before - after);
                if(refresh < 0) refresh = -refresh;
            }
        }
        else {
            refresh = str2i(refresh_str);
            if(refresh < 0) refresh = -refresh;
        }
    }

    if(!label) {
        if(alarm) {
            char *s = (char *)alarm;
            while(*s) {
                if(*s == '_') *s = ' ';
                s++;
            }
            label = alarm;
        }
        else if(dimensions) {
            const char *dim = buffer_tostring(dimensions);
            if(*dim == '|') dim++;
            label = dim;
        }
        else
            label = st->name;
    }
    if(!units) {
        if(alarm) {
            if(rc->units)
                units = rc->units;
            else
                units = "";
        }
        else if(options & RRDR_OPTION_PERCENTAGE)
            units = "%";
        else
            units = st->units;
    }

    debug(D_WEB_CLIENT, "%llu: API command 'badge.svg' for chart '%s', alarm '%s', dimensions '%s', after '%lld', before '%lld', points '%d', group '%d', options '0x%08x'"
          , w->id
          , chart
          , alarm?alarm:""
          , (dimensions)?buffer_tostring(dimensions):""
          , after
          , before
          , points
          , group
          , options
    );

    if(rc) {
        if (refresh > 0) {
            buffer_sprintf(w->response.header, "Refresh: %d\r\n", refresh);
            w->response.data->expires = now_realtime_sec() + refresh;
        }
        else buffer_no_cacheable(w->response.data);

        if(!value_color) {
            switch(rc->status) {
                case RRDCALC_STATUS_CRITICAL:
                    value_color = "red";
                    break;

                case RRDCALC_STATUS_WARNING:
                    value_color = "orange";
                    break;

                case RRDCALC_STATUS_CLEAR:
                    value_color = "brightgreen";
                    break;

                case RRDCALC_STATUS_UNDEFINED:
                    value_color = "lightgrey";
                    break;

                case RRDCALC_STATUS_UNINITIALIZED:
                    value_color = "#000";
                    break;

                default:
                    value_color = "grey";
                    break;
            }
        }

        buffer_svg(w->response.data,
                label,
                (isnan(rc->value)||isinf(rc->value)) ? rc->value : rc->value * multiply / divide,
                units,
                label_color,
                value_color,
                precision,
                scale,
                options
        );
        ret = 200;
    }
    else {
        time_t latest_timestamp = 0;
        int value_is_null = 1;
        calculated_number n = NAN;
        ret = 500;

        // if the collected value is too old, don't calculate its value
        if (rrdset_last_entry_t(st) >= (now_realtime_sec() - (st->update_every * st->gap_when_lost_iterations_above)))
            ret = rrdset2value_api_v1(st, w->response.data, &n, (dimensions) ? buffer_tostring(dimensions) : NULL
                                      , points, after, before, group, 0, options, NULL, &latest_timestamp, &value_is_null);

        // if the value cannot be calculated, show empty badge
        if (ret != 200) {
            buffer_no_cacheable(w->response.data);
            value_is_null = 1;
            n = 0;
            ret = 200;
        }
        else if (refresh > 0) {
            buffer_sprintf(w->response.header, "Refresh: %d\r\n", refresh);
            w->response.data->expires = now_realtime_sec() + refresh;
        }
        else buffer_no_cacheable(w->response.data);

        // render the badge
        buffer_svg(w->response.data,
                label,
                (value_is_null)?NAN:(n * multiply / divide),
                units,
                label_color,
                value_color,
                precision,
                scale,
                options
        );
    }

    cleanup:
    buffer_free(dimensions);
    return ret;
}

// returns the HTTP code
inline int web_client_api_request_v1_data(RRDHOST *host, struct web_client *w, char *url) {
    debug(D_WEB_CLIENT, "%llu: API v1 data with URL '%s'", w->id, url);

    int ret = 400;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    char    *google_version = "0.6",
            *google_reqId = "0",
            *google_sig = "0",
            *google_out = "json",
            *responseHandler = NULL,
            *outFileName = NULL;

    time_t last_timestamp_in_data = 0, google_timestamp = 0;

    char *chart = NULL
    , *before_str = NULL
    , *after_str = NULL
    , *group_time_str = NULL
    , *points_str = NULL;

    int group = GROUP_AVERAGE;
    uint32_t format = DATASOURCE_JSON;
    uint32_t options = 0x00000000;

    while(url) {
        char *value = mystrsep(&url, "?&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 data query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "gtime")) group_time_str = value;
        else if(!strcmp(name, "group")) {
            group = web_client_api_request_v1_data_group(value, GROUP_AVERAGE);
        }
        else if(!strcmp(name, "format")) {
            format = web_client_api_request_v1_data_format(value);
        }
        else if(!strcmp(name, "options")) {
            options |= web_client_api_request_v1_data_options(value);
        }
        else if(!strcmp(name, "callback")) {
            responseHandler = value;
        }
        else if(!strcmp(name, "filename")) {
            outFileName = value;
        }
        else if(!strcmp(name, "tqx")) {
            // parse Google Visualization API options
            // https://developers.google.com/chart/interactive/docs/dev/implementing_data_source
            char *tqx_name, *tqx_value;

            while(value) {
                tqx_value = mystrsep(&value, ";");
                if(!tqx_value || !*tqx_value) continue;

                tqx_name = mystrsep(&tqx_value, ":");
                if(!tqx_name || !*tqx_name) continue;
                if(!tqx_value || !*tqx_value) continue;

                if(!strcmp(tqx_name, "version"))
                    google_version = tqx_value;
                else if(!strcmp(tqx_name, "reqId"))
                    google_reqId = tqx_value;
                else if(!strcmp(tqx_name, "sig")) {
                    google_sig = tqx_value;
                    google_timestamp = strtoul(google_sig, NULL, 0);
                }
                else if(!strcmp(tqx_name, "out")) {
                    google_out = tqx_value;
                    format = web_client_api_request_v1_data_google_format(google_out);
                }
                else if(!strcmp(tqx_name, "responseHandler"))
                    responseHandler = tqx_value;
                else if(!strcmp(tqx_name, "outFileName"))
                    outFileName = tqx_value;
            }
        }
    }

    if(!chart || !*chart) {
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    RRDSET *st = rrdset_find(host, chart);
    if(!st) st = rrdset_find_byname(host, chart);
    if(!st) {
        buffer_strcat(w->response.data, "Chart is not found: ");
        buffer_strcat_htmlescape(w->response.data, chart);
        ret = 404;
        goto cleanup;
    }
    st->last_accessed_time = now_realtime_sec();

    long long before = (before_str && *before_str)?str2l(before_str):0;
    long long after  = (after_str  && *after_str) ?str2l(after_str):0;
    int       points = (points_str && *points_str)?str2i(points_str):0;
    long      group_time = (group_time_str && *group_time_str)?str2l(group_time_str):0;

    debug(D_WEB_CLIENT, "%llu: API command 'data' for chart '%s', dimensions '%s', after '%lld', before '%lld', points '%d', group '%d', format '%u', options '0x%08x'"
          , w->id
          , chart
          , (dimensions)?buffer_tostring(dimensions):""
          , after
          , before
          , points
          , group
          , format
          , options
    );

    if(outFileName && *outFileName) {
        buffer_sprintf(w->response.header, "Content-Disposition: attachment; filename=\"%s\"\r\n", outFileName);
        debug(D_WEB_CLIENT, "%llu: generating outfilename header: '%s'", w->id, outFileName);
    }

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "google.visualization.Query.setResponse";

        debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSON/JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
                w->id, google_version, google_reqId, google_sig, google_out, responseHandler, outFileName
        );

        buffer_sprintf(w->response.data,
                "%s({version:'%s',reqId:'%s',status:'ok',sig:'%ld',table:",
                responseHandler, google_version, google_reqId, st->last_updated.tv_sec);
    }
    else if(format == DATASOURCE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "callback";

        buffer_strcat(w->response.data, responseHandler);
        buffer_strcat(w->response.data, "(");
    }

    ret = rrdset2anything_api_v1(st, w->response.data, dimensions, format, points, after, before, group, group_time
                                 , options, &last_timestamp_in_data);

    if(format == DATASOURCE_DATATABLE_JSONP) {
        if(google_timestamp < last_timestamp_in_data)
            buffer_strcat(w->response.data, "});");

        else {
            // the client already has the latest data
            buffer_flush(w->response.data);
            buffer_sprintf(w->response.data,
                    "%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
                    responseHandler, google_version, google_reqId);
        }
    }
    else if(format == DATASOURCE_JSONP)
        buffer_strcat(w->response.data, ");");

    cleanup:
    buffer_free(dimensions);
    return ret;
}

inline int web_client_api_request_v1_registry(RRDHOST *host, struct web_client *w, char *url) {
    static uint32_t hash_action = 0, hash_access = 0, hash_hello = 0, hash_delete = 0, hash_search = 0,
            hash_switch = 0, hash_machine = 0, hash_url = 0, hash_name = 0, hash_delete_url = 0, hash_for = 0,
            hash_to = 0 /*, hash_redirects = 0 */;

    if(unlikely(!hash_action)) {
        hash_action = simple_hash("action");
        hash_access = simple_hash("access");
        hash_hello = simple_hash("hello");
        hash_delete = simple_hash("delete");
        hash_search = simple_hash("search");
        hash_switch = simple_hash("switch");
        hash_machine = simple_hash("machine");
        hash_url = simple_hash("url");
        hash_name = simple_hash("name");
        hash_delete_url = simple_hash("delete_url");
        hash_for = simple_hash("for");
        hash_to = simple_hash("to");
/*
        hash_redirects = simple_hash("redirects");
*/
    }

    char person_guid[GUID_LEN + 1] = "";

    debug(D_WEB_CLIENT, "%llu: API v1 registry with URL '%s'", w->id, url);

    // FIXME
    // The browser may send multiple cookies with our id

    char *cookie = strstr(w->response.data->buffer, NETDATA_REGISTRY_COOKIE_NAME "=");
    if(cookie)
        strncpyz(person_guid, &cookie[sizeof(NETDATA_REGISTRY_COOKIE_NAME)], 36);

    char action = '\0';
    char *machine_guid = NULL,
            *machine_url = NULL,
            *url_name = NULL,
            *search_machine_guid = NULL,
            *delete_url = NULL,
            *to_person_guid = NULL;
/*
    int redirects = 0;
*/

    while(url) {
        char *value = mystrsep(&url, "?&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if (!name || !*name) continue;
        if (!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 registry query param '%s' with value '%s'", w->id, name, value);

        uint32_t hash = simple_hash(name);

        if(hash == hash_action && !strcmp(name, "action")) {
            uint32_t vhash = simple_hash(value);

            if(vhash == hash_access && !strcmp(value, "access")) action = 'A';
            else if(vhash == hash_hello && !strcmp(value, "hello")) action = 'H';
            else if(vhash == hash_delete && !strcmp(value, "delete")) action = 'D';
            else if(vhash == hash_search && !strcmp(value, "search")) action = 'S';
            else if(vhash == hash_switch && !strcmp(value, "switch")) action = 'W';
#ifdef NETDATA_INTERNAL_CHECKS
            else error("unknown registry action '%s'", value);
#endif /* NETDATA_INTERNAL_CHECKS */
        }
/*
        else if(hash == hash_redirects && !strcmp(name, "redirects"))
            redirects = atoi(value);
*/
        else if(hash == hash_machine && !strcmp(name, "machine"))
            machine_guid = value;

        else if(hash == hash_url && !strcmp(name, "url"))
            machine_url = value;

        else if(action == 'A') {
            if(hash == hash_name && !strcmp(name, "name"))
                url_name = value;
        }
        else if(action == 'D') {
            if(hash == hash_delete_url && !strcmp(name, "delete_url"))
                delete_url = value;
        }
        else if(action == 'S') {
            if(hash == hash_for && !strcmp(name, "for"))
                search_machine_guid = value;
        }
        else if(action == 'W') {
            if(hash == hash_to && !strcmp(name, "to"))
                to_person_guid = value;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        else error("unused registry URL parameter '%s' with value '%s'", name, value);
#endif /* NETDATA_INTERNAL_CHECKS */
    }

    if(unlikely(respect_web_browser_do_not_track_policy && web_client_has_donottrack(w))) {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Your web browser is sending 'DNT: 1' (Do Not Track). The registry requires persistent cookies on your browser to work.");
        return 400;
    }

    if(unlikely(action == 'H')) {
        // HELLO request, dashboard ACL
        if(unlikely(!web_client_can_access_dashboard(w)))
            return web_client_permission_denied(w);
    }
    else {
        // everything else, registry ACL
        if(unlikely(!web_client_can_access_registry(w)))
            return web_client_permission_denied(w);
    }

    switch(action) {
        case 'A':
            if(unlikely(!machine_guid || !machine_url || !url_name)) {
                error("Invalid registry request - access requires these parameters: machine ('%s'), url ('%s'), name ('%s')", machine_guid ? machine_guid : "UNSET", machine_url ? machine_url : "UNSET", url_name ? url_name : "UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Access request.");
                return 400;
            }

            web_client_enable_tracking_required(w);
            return registry_request_access_json(host, w, person_guid, machine_guid, machine_url, url_name, now_realtime_sec());

        case 'D':
            if(unlikely(!machine_guid || !machine_url || !delete_url)) {
                error("Invalid registry request - delete requires these parameters: machine ('%s'), url ('%s'), delete_url ('%s')", machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", delete_url?delete_url:"UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Delete request.");
                return 400;
            }

            web_client_enable_tracking_required(w);
            return registry_request_delete_json(host, w, person_guid, machine_guid, machine_url, delete_url, now_realtime_sec());

        case 'S':
            if(unlikely(!machine_guid || !machine_url || !search_machine_guid)) {
                error("Invalid registry request - search requires these parameters: machine ('%s'), url ('%s'), for ('%s')", machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", search_machine_guid?search_machine_guid:"UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Search request.");
                return 400;
            }

            web_client_enable_tracking_required(w);
            return registry_request_search_json(host, w, person_guid, machine_guid, machine_url, search_machine_guid, now_realtime_sec());

        case 'W':
            if(unlikely(!machine_guid || !machine_url || !to_person_guid)) {
                error("Invalid registry request - switching identity requires these parameters: machine ('%s'), url ('%s'), to ('%s')", machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", to_person_guid?to_person_guid:"UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Switch request.");
                return 400;
            }

            web_client_enable_tracking_required(w);
            return registry_request_switch_json(host, w, person_guid, machine_guid, machine_url, to_person_guid, now_realtime_sec());

        case 'H':
            return registry_request_hello_json(host, w);

        default:
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Invalid registry request - you need to set an action: hello, access, delete, search");
            return 400;
    }
}

static struct api_command {
    const char *command;
    uint32_t hash;
    WEB_CLIENT_ACL acl;
    int (*callback)(RRDHOST *host, struct web_client *w, char *url);
} api_commands[] = {
        { "data",            0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_data            },
        { "chart",           0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_chart           },
        { "charts",          0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_charts          },

        // registry checks the ACL by itself, so we allow everything
        { "registry",        0, WEB_CLIENT_ACL_NOCHECK,   web_client_api_request_v1_registry        },

        // badges can be fetched with both dashboard and badge permissions
        { "badge.svg",       0, WEB_CLIENT_ACL_DASHBOARD|WEB_CLIENT_ACL_BADGE, web_client_api_request_v1_badge },

        { "alarms",          0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarms          },
        { "alarm_log",       0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_log       },
        { "alarm_variables", 0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_variables },
        { "allmetrics",      0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_allmetrics      },

        // terminator
        { NULL,              0, WEB_CLIENT_ACL_NONE,      NULL                                      },
};

inline int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url) {
    static int initialized = 0;
    int i;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(i = 0; api_commands[i].command ; i++)
            api_commands[i].hash = simple_hash(api_commands[i].command);
    }

    // get the command
    char *tok = mystrsep(&url, "/?&");
    if(tok && *tok) {
        debug(D_WEB_CLIENT, "%llu: Searching for API v1 command '%s'.", w->id, tok);
        uint32_t hash = simple_hash(tok);

        for(i = 0; api_commands[i].command ;i++) {
            if(unlikely(hash == api_commands[i].hash && !strcmp(tok, api_commands[i].command))) {
                if(unlikely(api_commands[i].acl != WEB_CLIENT_ACL_NOCHECK) && !(w->acl & api_commands[i].acl))
                    return web_client_permission_denied(w);

                return api_commands[i].callback(host, w, url);
            }
        }

        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Unsupported v1 API command: ");
        buffer_strcat_htmlescape(w->response.data, tok);
        return 404;
    }
    else {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API v1 command?");
        return 400;
    }
}
