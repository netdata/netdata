// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v1.h"

char *api_secret;

static struct {
    const char *name;
    uint32_t hash;
    RRDR_OPTIONS value;
} api_v1_data_options[] = {
        {  "nonzero"           , 0    , RRDR_OPTION_NONZERO}
        , {"flip"              , 0    , RRDR_OPTION_REVERSED}
        , {"reversed"          , 0    , RRDR_OPTION_REVERSED}
        , {"reverse"           , 0    , RRDR_OPTION_REVERSED}
        , {"jsonwrap"          , 0    , RRDR_OPTION_JSON_WRAP}
        , {"min2max"           , 0    , RRDR_OPTION_MIN2MAX}
        , {"ms"                , 0    , RRDR_OPTION_MILLISECONDS}
        , {"milliseconds"      , 0    , RRDR_OPTION_MILLISECONDS}
        , {"abs"               , 0    , RRDR_OPTION_ABSOLUTE}
        , {"absolute"          , 0    , RRDR_OPTION_ABSOLUTE}
        , {"absolute_sum"      , 0    , RRDR_OPTION_ABSOLUTE}
        , {"absolute-sum"      , 0    , RRDR_OPTION_ABSOLUTE}
        , {"display_absolute"  , 0    , RRDR_OPTION_DISPLAY_ABS}
        , {"display-absolute"  , 0    , RRDR_OPTION_DISPLAY_ABS}
        , {"seconds"           , 0    , RRDR_OPTION_SECONDS}
        , {"null2zero"         , 0    , RRDR_OPTION_NULL2ZERO}
        , {"objectrows"        , 0    , RRDR_OPTION_OBJECTSROWS}
        , {"google_json"       , 0    , RRDR_OPTION_GOOGLE_JSON}
        , {"google-json"       , 0    , RRDR_OPTION_GOOGLE_JSON}
        , {"percentage"        , 0    , RRDR_OPTION_PERCENTAGE}
        , {"unaligned"         , 0    , RRDR_OPTION_NOT_ALIGNED}
        , {"match_ids"         , 0    , RRDR_OPTION_MATCH_IDS}
        , {"match-ids"         , 0    , RRDR_OPTION_MATCH_IDS}
        , {"match_names"       , 0    , RRDR_OPTION_MATCH_NAMES}
        , {"match-names"       , 0    , RRDR_OPTION_MATCH_NAMES}
        , {"anomaly-bit"       , 0    , RRDR_OPTION_ANOMALY_BIT}
        , {"selected-tier"     , 0    , RRDR_OPTION_SELECTED_TIER}
        , {"raw"               , 0    , RRDR_OPTION_RETURN_RAW}
        , {"jw-anomaly-rates"  , 0    , RRDR_OPTION_RETURN_JWAR}
        , {"natural-points"    , 0    , RRDR_OPTION_NATURAL_POINTS}
        , {"virtual-points"    , 0    , RRDR_OPTION_VIRTUAL_POINTS}
        , {"all-dimensions"    , 0    , RRDR_OPTION_ALL_DIMENSIONS}
        , {"plan"              , 0    , RRDR_OPTION_SHOW_PLAN}
        , {"details"           , 0    , RRDR_OPTION_SHOW_DETAILS}
        , {"debug"             , 0    , RRDR_OPTION_DEBUG}
        , {"minify"            , 0    , RRDR_OPTION_MINIFY}
        , {"annotations"       , 0    , RRDR_OPTION_JW_ANNOTATIONS}
        , {NULL                , 0    , 0}
};

static struct {
    const char *name;
    uint32_t hash;
    DATASOURCE_FORMAT value;
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
        , {DATASOURCE_FORMAT_CSV_MARKDOWN   , 0 , DATASOURCE_CSV_MARKDOWN}
        , {                                 NULL, 0, 0}
};

static struct {
    const char *name;
    uint32_t hash;
    DATASOURCE_FORMAT value;
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

    for(i = 0; api_v1_data_options[i].name ; i++)
        api_v1_data_options[i].hash = simple_hash(api_v1_data_options[i].name);

    for(i = 0; api_v1_data_formats[i].name ; i++)
        api_v1_data_formats[i].hash = simple_hash(api_v1_data_formats[i].name);

    for(i = 0; api_v1_data_google_formats[i].name ; i++)
        api_v1_data_google_formats[i].hash = simple_hash(api_v1_data_google_formats[i].name);

    time_grouping_init();

	uuid_t uuid;

	// generate
	uuid_generate(uuid);

	// unparse (to string)
	char uuid_str[37];
	uuid_unparse_lower(uuid, uuid_str);
}

char *get_mgmt_api_key(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/netdata.api.key", netdata_configured_varlib_dir);
    char *api_key_filename=config_get(CONFIG_SECTION_REGISTRY, "netdata management api key file", filename);
    static char guid[GUID_LEN + 1] = "";

    if(likely(guid[0]))
        return guid;

    // read it from disk
    int fd = open(api_key_filename, O_RDONLY);
    if(fd != -1) {
        char buf[GUID_LEN + 1];
        if(read(fd, buf, GUID_LEN) != GUID_LEN)
            error("Failed to read management API key from '%s'", api_key_filename);
        else {
            buf[GUID_LEN] = '\0';
            if(regenerate_guid(buf, guid) == -1) {
                error("Failed to validate management API key '%s' from '%s'.",
                      buf, api_key_filename);

                guid[0] = '\0';
            }
        }
        close(fd);
    }

    // generate a new one?
    if(!guid[0]) {
        uuid_t uuid;

        uuid_generate_time(uuid);
        uuid_unparse_lower(uuid, guid);
        guid[GUID_LEN] = '\0';

        // save it
        fd = open(api_key_filename, O_WRONLY|O_CREAT|O_TRUNC, 444);
        if(fd == -1) {
            error("Cannot create unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file.", api_key_filename);
            goto temp_key;
        }

        if(write(fd, guid, GUID_LEN) != GUID_LEN) {
            error("Cannot write the unique management API key file '%s'. Please adjust config parameter 'netdata management api key file' to a proper path and file with enough space left.", api_key_filename);
            close(fd);
            goto temp_key;
        }

        close(fd);
    }

    return guid;

temp_key:
    info("You can still continue to use the alarm management API using the authorization token %s during this Netdata session only.", guid);
    return guid;
}

void web_client_api_v1_management_init(void) {
	api_secret = get_mgmt_api_key();
}

inline RRDR_OPTIONS web_client_api_request_v1_data_options(char *o) {
    RRDR_OPTIONS ret = 0x00000000;
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

void web_client_api_request_v1_data_options_to_buffer_json_array(BUFFER *wb, const char *key, RRDR_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; api_v1_data_options[i].name ; i++) {
        if (unlikely((api_v1_data_options[i].value & options) && !(api_v1_data_options[i].value & used))) {
            const char *name = api_v1_data_options[i].name;
            used |= api_v1_data_options[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void web_client_api_request_v1_data_options_to_string(char *buf, size_t size, RRDR_OPTIONS options) {
    char *write = buf;
    char *end = &buf[size - 1];

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    int added = 0;
    for(int i = 0; api_v1_data_options[i].name ; i++) {
        if (unlikely((api_v1_data_options[i].value & options) && !(api_v1_data_options[i].value & used))) {
            const char *name = api_v1_data_options[i].name;
            used |= api_v1_data_options[i].value;

            if(added && write < end)
                *write++ = ',';

            while(*name && write < end)
                *write++ = *name++;

            added++;
        }
    }
    *write = *end = '\0';
}

inline DATASOURCE_FORMAT web_client_api_request_v1_data_format(char *name) {
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

int web_client_api_request_v1_alarms_select (char *url) {
    int all = 0;
    while(url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value) continue;

        if(!strcmp(value, "all")) all = 1;
        else if(!strcmp(value, "active")) all = 0;
    }

    return all;
}

inline int web_client_api_request_v1_alarms(RRDHOST *host, struct web_client *w, char *url) {
    int all = web_client_api_request_v1_alarms_select(url);

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    health_alarms2json(host, w->response.data, all);
    buffer_no_cacheable(w->response.data);
    return HTTP_RESP_OK;
}

inline int web_client_api_request_v1_alarms_values(RRDHOST *host, struct web_client *w, char *url) {
    int all = web_client_api_request_v1_alarms_select(url);

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    health_alarms_values2json(host, w->response.data, all);
    buffer_no_cacheable(w->response.data);
    return HTTP_RESP_OK;
}

inline int web_client_api_request_v1_alarm_count(RRDHOST *host, struct web_client *w, char *url) {
    RRDCALC_STATUS status = RRDCALC_STATUS_RAISED;
    BUFFER *contexts = NULL;

    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "[");

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 alarm_count query param '%s' with value '%s'", w->id, name, value);

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

inline int web_client_api_request_v1_alarm_log(RRDHOST *host, struct web_client *w, char *url) {
    uint32_t after = 0;
    char *chart = NULL;

    while(url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if (!strcmp(name, "after")) after = (uint32_t)strtoul(value, NULL, 0);
        else if (!strcmp(name, "chart")) chart = value;
    }

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    health_alarm_log2json(host, w->response.data, after, chart);
    return HTTP_RESP_OK;
}

inline int web_client_api_request_single_chart(RRDHOST *host, struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf)) {
    int ret = HTTP_RESP_BAD_REQUEST;
    char *chart = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = mystrsep(&url, "&");
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
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    w->response.data->content_type = CT_APPLICATION_JSON;
    st->last_accessed_time_s = now_realtime_sec();
    callback(st, w->response.data);
    return HTTP_RESP_OK;

    cleanup:
    return ret;
}

inline int web_client_api_request_v1_alarm_variables(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_single_chart(host, w, url, health_api_v1_chart_variables2json);
}

static int web_client_api_request_v1_context(RRDHOST *host, struct web_client *w, char *url) {
    char *context = NULL;
    RRDCONTEXT_TO_JSON_OPTIONS options = RRDCONTEXT_OPTION_NONE;
    time_t after = 0, before = 0;
    const char *chart_label_key = NULL, *chart_labels_filter = NULL;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "context") || !strcmp(name, "ctx")) context = value;
        else if(!strcmp(name, "after")) after = str2l(value);
        else if(!strcmp(name, "before")) before = str2l(value);
        else if(!strcmp(name, "options")) options = rrdcontext_to_json_parse_options(value);
        else if(!strcmp(name, "chart_label_key")) chart_label_key = value;
        else if(!strcmp(name, "chart_labels_filter")) chart_labels_filter = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100, &netdata_buffers_statistics.buffers_api);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
    }

    if(!context || !*context) {
        buffer_sprintf(w->response.data, "No context is given at the request.");
        return HTTP_RESP_BAD_REQUEST;
    }

    SIMPLE_PATTERN *chart_label_key_pattern = NULL;
    SIMPLE_PATTERN *chart_labels_filter_pattern = NULL;
    SIMPLE_PATTERN *chart_dimensions_pattern = NULL;

    if(chart_label_key)
        chart_label_key_pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT, true);

    if(chart_labels_filter)
        chart_labels_filter_pattern = simple_pattern_create(chart_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT,
                                                            true);

    if(dimensions) {
        chart_dimensions_pattern = simple_pattern_create(buffer_tostring(dimensions), ",|\t\r\n\f\v",
                                                         SIMPLE_PATTERN_EXACT, true);
        buffer_free(dimensions);
    }

    w->response.data->content_type = CT_APPLICATION_JSON;
    int ret = rrdcontext_to_json(host, w->response.data, after, before, options, context, chart_label_key_pattern, chart_labels_filter_pattern, chart_dimensions_pattern);

    simple_pattern_free(chart_label_key_pattern);
    simple_pattern_free(chart_labels_filter_pattern);
    simple_pattern_free(chart_dimensions_pattern);

    return ret;
}

static int web_client_api_request_v1_contexts(RRDHOST *host, struct web_client *w, char *url) {
    RRDCONTEXT_TO_JSON_OPTIONS options = RRDCONTEXT_OPTION_NONE;
    time_t after = 0, before = 0;
    const char *chart_label_key = NULL, *chart_labels_filter = NULL;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "after")) after = str2l(value);
        else if(!strcmp(name, "before")) before = str2l(value);
        else if(!strcmp(name, "options")) options = rrdcontext_to_json_parse_options(value);
        else if(!strcmp(name, "chart_label_key")) chart_label_key = value;
        else if(!strcmp(name, "chart_labels_filter")) chart_labels_filter = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100, &netdata_buffers_statistics.buffers_api);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
    }

    SIMPLE_PATTERN *chart_label_key_pattern = NULL;
    SIMPLE_PATTERN *chart_labels_filter_pattern = NULL;
    SIMPLE_PATTERN *chart_dimensions_pattern = NULL;

    if(chart_label_key)
        chart_label_key_pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT, true);

    if(chart_labels_filter)
        chart_labels_filter_pattern = simple_pattern_create(chart_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT,
                                                            true);

    if(dimensions) {
        chart_dimensions_pattern = simple_pattern_create(buffer_tostring(dimensions), ",|\t\r\n\f\v",
                                                         SIMPLE_PATTERN_EXACT, true);
        buffer_free(dimensions);
    }

    w->response.data->content_type = CT_APPLICATION_JSON;
    int ret = rrdcontexts_to_json(host, w->response.data, after, before, options, chart_label_key_pattern, chart_labels_filter_pattern, chart_dimensions_pattern);

    simple_pattern_free(chart_label_key_pattern);
    simple_pattern_free(chart_labels_filter_pattern);
    simple_pattern_free(chart_dimensions_pattern);

    return ret;
}

inline int web_client_api_request_v1_charts(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;

    buffer_flush(w->response.data);
    w->response.data->content_type = CT_APPLICATION_JSON;
    charts2json(host, w->response.data, 0, 0);
    return HTTP_RESP_OK;
}

inline int web_client_api_request_v1_chart(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_single_chart(host, w, url, rrd_stats_api_v1_chart);
}

// returns the HTTP code
static inline int web_client_api_request_v1_data(RRDHOST *host, struct web_client *w, char *url) {
    debug(D_WEB_CLIENT, "%llu: API v1 data with URL '%s'", w->id, url);

    int ret = HTTP_RESP_BAD_REQUEST;
    BUFFER *dimensions = NULL;

    buffer_flush(w->response.data);

    char    *google_version = "0.6",
            *google_reqId = "0",
            *google_sig = "0",
            *google_out = "json",
            *responseHandler = NULL,
            *outFileName = NULL;

    time_t last_timestamp_in_data = 0, google_timestamp = 0;

    char *chart = NULL;
    char *before_str = NULL;
    char *after_str = NULL;
    char *group_time_str = NULL;
    char *points_str = NULL;
    char *timeout_str = NULL;
    char *context = NULL;
    char *chart_label_key = NULL;
    char *chart_labels_filter = NULL;
    char *group_options = NULL;
    size_t tier = 0;
    RRDR_TIME_GROUPING group = RRDR_GROUPING_AVERAGE;
    DATASOURCE_FORMAT format = DATASOURCE_JSON;
    RRDR_OPTIONS options = 0;

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 data query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "context")) context = value;
        else if(!strcmp(name, "chart_label_key")) chart_label_key = value;
        else if(!strcmp(name, "chart_labels_filter")) chart_labels_filter = value;
        else if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions) dimensions = buffer_create(100, &netdata_buffers_statistics.buffers_api);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "show_dimensions")) options |= RRDR_OPTION_ALL_DIMENSIONS;
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "timeout")) timeout_str = value;
        else if(!strcmp(name, "gtime")) group_time_str = value;
        else if(!strcmp(name, "group_options")) group_options = value;
        else if(!strcmp(name, "group")) {
            group = time_grouping_parse(value, RRDR_GROUPING_AVERAGE);
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
        else if(!strcmp(name, "tier")) {
            tier = str2ul(value);
            if(tier < storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
            else
                tier = 0;
        }
    }

    // validate the google parameters given
    fix_google_param(google_out);
    fix_google_param(google_sig);
    fix_google_param(google_reqId);
    fix_google_param(google_version);
    fix_google_param(responseHandler);
    fix_google_param(outFileName);

    RRDSET *st = NULL;
    ONEWAYALLOC *owa = onewayalloc_create(0);
    QUERY_TARGET *qt = NULL;

    if(!is_valid_sp(chart) && !is_valid_sp(context)) {
        buffer_sprintf(w->response.data, "No chart or context is given.");
        goto cleanup;
    }

    if(chart && !context) {
        // check if this is a specific chart
        st = rrdset_find(host, chart);
        if (!st) st = rrdset_find_byname(host, chart);
    }

    long long before = (before_str && *before_str)?str2l(before_str):0;
    long long after  = (after_str  && *after_str) ?str2l(after_str):-600;
    int       points = (points_str && *points_str)?str2i(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    long      group_time = (group_time_str && *group_time_str)?str2l(group_time_str):0;

    QUERY_TARGET_REQUEST qtr = {
            .version = 1,
            .after = after,
            .before = before,
            .host = host,
            .st = st,
            .nodes = NULL,
            .contexts = context,
            .instances = chart,
            .dimensions = (dimensions)?buffer_tostring(dimensions):NULL,
            .timeout = timeout,
            .points = points,
            .format = format,
            .options = options,
            .time_group_method = group,
            .time_group_options = group_options,
            .resampling_time = group_time,
            .tier = tier,
            .chart_label_key = chart_label_key,
            .labels = chart_labels_filter,
            .query_source = QUERY_SOURCE_API_DATA,
            .priority = STORAGE_PRIORITY_NORMAL,
    };
    qt = query_target_create(&qtr);

    if(!qt || !qt->query.used) {
        buffer_sprintf(w->response.data, "No metrics where matched to query.");
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    if (timeout) {
        struct timeval now;
        now_realtime_timeval(&now);
        int inqueue = (int)dt_usec(&w->tv_in, &now) / 1000;
        timeout -= inqueue;
        if (timeout <= 0) {
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Query timeout exceeded");
            ret = HTTP_RESP_BACKEND_FETCH_FAILED;
            goto cleanup;
        }
    }

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

        buffer_sprintf(
            w->response.data,
            "%s({version:'%s',reqId:'%s',status:'ok',sig:'%"PRId64"',table:",
            responseHandler,
            google_version,
            google_reqId,
            (int64_t)st->last_updated.tv_sec);
    }
    else if(format == DATASOURCE_JSONP) {
        if(responseHandler == NULL)
            responseHandler = "callback";

        buffer_strcat(w->response.data, responseHandler);
        buffer_strcat(w->response.data, "(");
    }

    ret = data_query_execute(owa, w->response.data, qt, &last_timestamp_in_data);

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
    query_target_release(qt);
    onewayalloc_destroy(owa);
    buffer_free(dimensions);
    return ret;
}

// Pings a netdata server:
// /api/v1/registry?action=hello
//
// Access to a netdata registry:
// /api/v1/registry?action=access&machine=${machine_guid}&name=${hostname}&url=${url}
//
// Delete from a netdata registry:
// /api/v1/registry?action=delete&machine=${machine_guid}&name=${hostname}&url=${url}&delete_url=${delete_url}
//
// Search for the URLs of a machine:
// /api/v1/registry?action=search&machine=${machine_guid}&name=${hostname}&url=${url}&for=${machine_guid}
//
// Impersonate:
// /api/v1/registry?action=switch&machine=${machine_guid}&name=${hostname}&url=${url}&to=${new_person_guid}
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

    // TODO
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

	// Don't cache registry responses
    buffer_no_cacheable(w->response.data);

    while(url) {
        char *value = mystrsep(&url, "&");
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
        return HTTP_RESP_BAD_REQUEST;
    }

    if(unlikely(action == 'H')) {
        // HELLO request, dashboard ACL
        analytics_log_dashboard();
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
                return HTTP_RESP_BAD_REQUEST;
            }

            web_client_enable_tracking_required(w);
            return registry_request_access_json(host, w, person_guid, machine_guid, machine_url, url_name, now_realtime_sec());

        case 'D':
            if(unlikely(!machine_guid || !machine_url || !delete_url)) {
                error("Invalid registry request - delete requires these parameters: machine ('%s'), url ('%s'), delete_url ('%s')", machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", delete_url?delete_url:"UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Delete request.");
                return HTTP_RESP_BAD_REQUEST;
            }

            web_client_enable_tracking_required(w);
            return registry_request_delete_json(host, w, person_guid, machine_guid, machine_url, delete_url, now_realtime_sec());

        case 'S':
            if(unlikely(!machine_guid || !machine_url || !search_machine_guid)) {
                error("Invalid registry request - search requires these parameters: machine ('%s'), url ('%s'), for ('%s')", machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", search_machine_guid?search_machine_guid:"UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Search request.");
                return HTTP_RESP_BAD_REQUEST;
            }

            web_client_enable_tracking_required(w);
            return registry_request_search_json(host, w, person_guid, machine_guid, machine_url, search_machine_guid, now_realtime_sec());

        case 'W':
            if(unlikely(!machine_guid || !machine_url || !to_person_guid)) {
                error("Invalid registry request - switching identity requires these parameters: machine ('%s'), url ('%s'), to ('%s')", machine_guid?machine_guid:"UNSET", machine_url?machine_url:"UNSET", to_person_guid?to_person_guid:"UNSET");
                buffer_flush(w->response.data);
                buffer_strcat(w->response.data, "Invalid registry Switch request.");
                return HTTP_RESP_BAD_REQUEST;
            }

            web_client_enable_tracking_required(w);
            return registry_request_switch_json(host, w, person_guid, machine_guid, machine_url, to_person_guid, now_realtime_sec());

        case 'H':
            return registry_request_hello_json(host, w);

        default:
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Invalid registry request - you need to set an action: hello, access, delete, search");
            return HTTP_RESP_BAD_REQUEST;
    }
}

static inline void web_client_api_request_v1_info_summary_alarm_statuses(RRDHOST *host, BUFFER *wb) {
    buffer_json_member_add_object(wb, "alarms");

    size_t normal = 0, warning = 0, critical = 0;
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        switch(rc->status) {
            case RRDCALC_STATUS_WARNING:
                warning++;
                break;
            case RRDCALC_STATUS_CRITICAL:
                critical++;
                break;
            default:
                normal++;
        }
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    buffer_json_member_add_uint64(wb, "normal", normal);
    buffer_json_member_add_uint64(wb, "warning", warning);
    buffer_json_member_add_uint64(wb, "critical", critical);

    buffer_json_object_close(wb);
}

static inline void web_client_api_request_v1_info_mirrored_hosts_status(BUFFER *wb, RRDHOST *host) {
    buffer_json_add_array_item_object(wb);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
    buffer_json_member_add_uint64(wb, "hops", host->system_info ? host->system_info->hops : (host == localhost) ? 0 : 1);
    buffer_json_member_add_boolean(wb, "reachable", (host == localhost || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)));

    buffer_json_member_add_string(wb, "guid", host->machine_guid);
    buffer_json_member_add_uuid(wb, "node_id", host->node_id);
    rrdhost_aclk_state_lock(host);
    buffer_json_member_add_string(wb, "claim_id", host->aclk_state.claimed_id);
    rrdhost_aclk_state_unlock(host);

    buffer_json_object_close(wb);
}

static inline void web_client_api_request_v1_info_mirrored_hosts(BUFFER *wb) {
    RRDHOST *host;

    rrd_rdlock();

    buffer_json_member_add_array(wb, "mirrored_hosts");
    rrdhost_foreach_read(host)
        buffer_json_add_array_item_string(wb, rrdhost_hostname(host));
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "mirrored_hosts_status");
    rrdhost_foreach_read(host) {
        if ((host == localhost || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) {
            web_client_api_request_v1_info_mirrored_hosts_status(wb, host);
        }
    }
    rrdhost_foreach_read(host) {
        if ((host != localhost && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) {
            web_client_api_request_v1_info_mirrored_hosts_status(wb, host);
        }
    }
    buffer_json_array_close(wb);

    rrd_unlock();
}

static inline void host_labels2json(RRDHOST *host, BUFFER *wb) {
    buffer_json_member_add_object(wb, "host_labels");
    rrdlabels_to_buffer_json_members(host->rrdlabels, wb);
    buffer_json_object_close(wb);
}

static void host_collectors(RRDHOST *host, BUFFER *wb) {
    buffer_json_member_add_array(wb, "collectors");

    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
    RRDSET *st;
    char name[500];

    time_t now = now_realtime_sec();

    rrdset_foreach_read(st, host) {
        if (!rrdset_is_available_for_viewers(st))
            continue;

        sprintf(name, "%s:%s", rrdset_plugin_name(st), rrdset_module_name(st));

        bool old = 0;
        bool *set = dictionary_set(dict, name, &old, sizeof(bool));
        if(!*set) {
            *set = true;
            st->last_accessed_time_s = now;
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "plugin", rrdset_plugin_name(st));
            buffer_json_member_add_string(wb, "module", rrdset_module_name(st));
            buffer_json_object_close(wb);
        }
    }
    rrdset_foreach_done(st);
    dictionary_destroy(dict);

    buffer_json_array_close(wb);
}

extern int aclk_connected;
inline int web_client_api_request_v1_info_fill_buffer(RRDHOST *host, BUFFER *wb) {
    buffer_json_initialize(wb, "\"", "\"", 0, true, false);

    buffer_json_member_add_string(wb, "version", rrdhost_program_version(host));
    buffer_json_member_add_string(wb, "uid", host->machine_guid);

    buffer_json_member_add_uint64(wb, "hosts-available", rrdhost_hosts_available());
    web_client_api_request_v1_info_mirrored_hosts(wb);

    web_client_api_request_v1_info_summary_alarm_statuses(host, wb);

    buffer_json_member_add_string_or_empty(wb, "os_name", host->system_info->host_os_name);
    buffer_json_member_add_string_or_empty(wb, "os_id", host->system_info->host_os_id);
    buffer_json_member_add_string_or_empty(wb, "os_id_like", host->system_info->host_os_id_like);
    buffer_json_member_add_string_or_empty(wb, "os_version", host->system_info->host_os_version);
    buffer_json_member_add_string_or_empty(wb, "os_version_id", host->system_info->host_os_version_id);
    buffer_json_member_add_string_or_empty(wb, "os_detection", host->system_info->host_os_detection);
    buffer_json_member_add_string_or_empty(wb, "cores_total", host->system_info->host_cores);
    buffer_json_member_add_string_or_empty(wb, "total_disk_space", host->system_info->host_disk_space);
    buffer_json_member_add_string_or_empty(wb, "cpu_freq", host->system_info->host_cpu_freq);
    buffer_json_member_add_string_or_empty(wb, "ram_total", host->system_info->host_ram_total);

    buffer_json_member_add_string_or_omit(wb, "container_os_name", host->system_info->container_os_name);
    buffer_json_member_add_string_or_omit(wb, "container_os_id", host->system_info->container_os_id);
    buffer_json_member_add_string_or_omit(wb, "container_os_id_like", host->system_info->container_os_id_like);
    buffer_json_member_add_string_or_omit(wb, "container_os_version", host->system_info->container_os_version);
    buffer_json_member_add_string_or_omit(wb, "container_os_version_id", host->system_info->container_os_version_id);
    buffer_json_member_add_string_or_omit(wb, "container_os_detection", host->system_info->container_os_detection);
    buffer_json_member_add_string_or_omit(wb, "is_k8s_node", host->system_info->is_k8s_node);

    buffer_json_member_add_string_or_empty(wb, "kernel_name", host->system_info->kernel_name);
    buffer_json_member_add_string_or_empty(wb, "kernel_version", host->system_info->kernel_version);
    buffer_json_member_add_string_or_empty(wb, "architecture", host->system_info->architecture);
    buffer_json_member_add_string_or_empty(wb, "virtualization", host->system_info->virtualization);
    buffer_json_member_add_string_or_empty(wb, "virt_detection", host->system_info->virt_detection);
    buffer_json_member_add_string_or_empty(wb, "container", host->system_info->container);
    buffer_json_member_add_string_or_empty(wb, "container_detection", host->system_info->container_detection);

    buffer_json_member_add_string_or_omit(wb, "cloud_provider_type", host->system_info->cloud_provider_type);
    buffer_json_member_add_string_or_omit(wb, "cloud_instance_type", host->system_info->cloud_instance_type);
    buffer_json_member_add_string_or_omit(wb, "cloud_instance_region", host->system_info->cloud_instance_region);

    host_labels2json(host, wb);
    host_functions2json(host, wb);
    host_collectors(host, wb);

#ifdef DISABLE_CLOUD
    buffer_json_member_add_boolean(wb, "cloud-enabled", false);
#else
    buffer_json_member_add_boolean(wb, "cloud-enabled",
                   appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", true));
#endif

#ifdef ENABLE_ACLK
    buffer_json_member_add_boolean(wb, "cloud-available", true);
#else
    buffer_json_member_add_boolean(wb, "cloud-available", false);
#endif

    char *agent_id = get_agent_claimid();
    buffer_json_member_add_boolean(wb, "agent-claimed", agent_id != NULL);
    freez(agent_id);

#ifdef ENABLE_ACLK
    buffer_json_member_add_boolean(wb, "aclk-available", aclk_connected);
#else
    buffer_json_member_add_boolean(wb, "aclk-available", false);
#endif

    buffer_json_member_add_string(wb, "memory-mode", rrd_memory_mode_name(host->rrd_memory_mode));
#ifdef ENABLE_DBENGINE
    buffer_json_member_add_uint64(wb, "multidb-disk-quota", default_multidb_disk_quota_mb);
    buffer_json_member_add_uint64(wb, "page-cache-size", default_rrdeng_page_cache_mb);
#endif // ENABLE_DBENGINE
    buffer_json_member_add_boolean(wb, "web-enabled", web_server_mode != WEB_SERVER_MODE_NONE);
    buffer_json_member_add_boolean(wb, "stream-enabled", default_rrdpush_enabled);

#ifdef  ENABLE_COMPRESSION
    buffer_json_member_add_boolean(wb, "stream-compression",
                                   host->sender && stream_has_capability(host->sender, STREAM_CAP_COMPRESSION));
#else
    buffer_json_member_add_boolean(wb, "stream-compression", false);
#endif  //ENABLE_COMPRESSION

#ifdef ENABLE_HTTPS
    buffer_json_member_add_boolean(wb, "https-enabled", true);
#else
    buffer_json_member_add_boolean(wb, "https-enabled", false);
#endif

    buffer_json_member_add_quoted_string(wb, "buildinfo", analytics_data.netdata_buildinfo);
    buffer_json_member_add_quoted_string(wb, "release-channel", analytics_data.netdata_config_release_channel);
    buffer_json_member_add_quoted_string(wb, "notification-methods", analytics_data.netdata_notification_methods);

    buffer_json_member_add_boolean(wb, "exporting-enabled", analytics_data.exporting_enabled);
    buffer_json_member_add_quoted_string(wb, "exporting-connectors", analytics_data.netdata_exporting_connectors);

    buffer_json_member_add_uint64(wb, "allmetrics-prometheus-used", analytics_data.prometheus_hits);
    buffer_json_member_add_uint64(wb, "allmetrics-shell-used", analytics_data.shell_hits);
    buffer_json_member_add_uint64(wb, "allmetrics-json-used", analytics_data.json_hits);
    buffer_json_member_add_uint64(wb, "dashboard-used", analytics_data.dashboard_hits);

    buffer_json_member_add_uint64(wb, "charts-count", analytics_data.charts_count);
    buffer_json_member_add_uint64(wb, "metrics-count", analytics_data.metrics_count);

#if defined(ENABLE_ML)
    buffer_json_member_add_object(wb, "ml-info");
    ml_get_host_info(host, wb);
    buffer_json_object_close(wb);
#endif

    buffer_json_finalize(wb);
    return 0;
}

#if defined(ENABLE_ML)
int web_client_api_request_v1_ml_info(RRDHOST *host, struct web_client *w, char *url) {
    (void) url;

    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    char *s = ml_get_host_runtime_info(host);
    if (!s)
        s = strdupz("{\"error\": \"json string is empty\" }\n");

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_strcat(wb, s);
    buffer_no_cacheable(wb);

    freez(s);
    return HTTP_RESP_OK;
}

int web_client_api_request_v1_ml_models(RRDHOST *host, struct web_client *w, char *url) {
    (void) url;

    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    char *s = ml_get_host_models(host);
    if (!s)
        s = strdupz("{\"error\": \"json string is empty\" }\n");

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_strcat(wb, s);
    buffer_no_cacheable(wb);

    freez(s);
    return HTTP_RESP_OK;
}
#endif

inline int web_client_api_request_v1_info(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;
    if (!netdata_ready) return HTTP_RESP_BACKEND_FETCH_FAILED;
    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;

    web_client_api_request_v1_info_fill_buffer(host, wb);

    buffer_no_cacheable(wb);
    return HTTP_RESP_OK;
}

static int web_client_api_request_v1_aclk_state(RRDHOST *host, struct web_client *w, char *url) {
    UNUSED(url);
    UNUSED(host);
    if (!netdata_ready) return HTTP_RESP_BACKEND_FETCH_FAILED;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);

    char *str = aclk_state_json();
    buffer_strcat(wb, str);
    freez(str);

    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
    return HTTP_RESP_OK;
}

static int web_client_api_request_v1_weights_internal(RRDHOST *host, struct web_client *w, char *url, WEIGHTS_METHOD method, WEIGHTS_FORMAT format) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    long long baseline_after = 0, baseline_before = 0, after = 0, before = 0, points = 0;
    RRDR_OPTIONS options = RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NONZERO | RRDR_OPTION_NULL2ZERO;
    int options_count = 0;
    RRDR_TIME_GROUPING group = RRDR_GROUPING_AVERAGE;
    int timeout = 0;
    size_t tier = 0;
    const char *group_options = NULL, *contexts_str = NULL;

    while (url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value)
            continue;

        char *name = mystrsep(&value, "=");
        if (!name || !*name)
            continue;
        if (!value || !*value)
            continue;

        if (!strcmp(name, "baseline_after"))
            baseline_after = (long long) strtoul(value, NULL, 0);

        else if (!strcmp(name, "baseline_before"))
            baseline_before = (long long) strtoul(value, NULL, 0);

        else if (!strcmp(name, "after") || !strcmp(name, "highlight_after"))
            after = (long long) strtoul(value, NULL, 0);

        else if (!strcmp(name, "before") || !strcmp(name, "highlight_before"))
            before = (long long) strtoul(value, NULL, 0);

        else if (!strcmp(name, "points") || !strcmp(name, "max_points"))
            points = (long long) strtoul(value, NULL, 0);

        else if (!strcmp(name, "timeout"))
            timeout = (int) strtoul(value, NULL, 0);

        else if(!strcmp(name, "group"))
            group = time_grouping_parse(value, RRDR_GROUPING_AVERAGE);

        else if(!strcmp(name, "options")) {
            if(!options_count) options = RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NULL2ZERO;
            options |= web_client_api_request_v1_data_options(value);
            options_count++;
        }

        else if(!strcmp(name, "method"))
            method = weights_string_to_method(value);

        else if(!strcmp(name, "context") || !strcmp(name, "contexts"))
            contexts_str = value;

        else if(!strcmp(name, "tier")) {
            tier = str2ul(value);
            if(tier < storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
            else
                tier = 0;
        }
    }

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;

    SIMPLE_PATTERN *contexts = (contexts_str) ? simple_pattern_create(contexts_str, ",|\t\r\n\f\v",
                                                                      SIMPLE_PATTERN_EXACT, true) : NULL;

    int ret = web_api_v1_weights(host, wb, method, format, group, group_options, baseline_after, baseline_before, after, before, points, options, contexts, tier, timeout);

    simple_pattern_free(contexts);
    return ret;
}

int web_client_api_request_v1_metric_correlations(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_v1_weights_internal(host, w, url, default_metric_correlations_method, WEIGHTS_FORMAT_CHARTS);
}

int web_client_api_request_v1_weights(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_v1_weights_internal(host, w, url, WEIGHTS_METHOD_ANOMALY_RATE, WEIGHTS_FORMAT_CONTEXTS);
}

int web_client_api_request_v1_function(RRDHOST *host, struct web_client *w, char *url) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    int timeout = 0;
    const char *function = NULL;

    while (url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value)
            continue;

        char *name = mystrsep(&value, "=");
        if (!name || !*name)
            continue;

        if (!strcmp(name, "function"))
            function = value;

        else if (!strcmp(name, "timeout"))
            timeout = (int) strtoul(value, NULL, 0);
    }

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);

    return rrd_call_function_and_wait(host, wb, timeout, function);
}

int web_client_api_request_v1_functions(RRDHOST *host, struct web_client *w, char *url __maybe_unused) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);

    buffer_json_initialize(wb, "\"", "\"", 0, true, false);
    host_functions2json(host, wb);
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

#ifndef ENABLE_DBENGINE
int web_client_api_request_v1_dbengine_stats(RRDHOST *host __maybe_unused, struct web_client *w __maybe_unused, char *url __maybe_unused) {
    return HTTP_RESP_NOT_FOUND;
}
#else
static void web_client_api_v1_dbengine_stats_for_tier(BUFFER *wb, size_t tier) {
    RRDENG_SIZE_STATS stats = rrdeng_size_statistics(multidb_ctx[tier]);

    buffer_sprintf(wb,
                   "\n\t\t\"default_granularity_secs\":%zu"
                   ",\n\t\t\"sizeof_datafile\":%zu"
                   ",\n\t\t\"sizeof_page_in_cache\":%zu"
                   ",\n\t\t\"sizeof_point_data\":%zu"
                   ",\n\t\t\"sizeof_page_data\":%zu"
                   ",\n\t\t\"pages_per_extent\":%zu"
                   ",\n\t\t\"datafiles\":%zu"
                   ",\n\t\t\"extents\":%zu"
                   ",\n\t\t\"extents_pages\":%zu"
                   ",\n\t\t\"points\":%zu"
                   ",\n\t\t\"metrics\":%zu"
                   ",\n\t\t\"metrics_pages\":%zu"
                   ",\n\t\t\"extents_compressed_bytes\":%zu"
                   ",\n\t\t\"pages_uncompressed_bytes\":%zu"
                   ",\n\t\t\"pages_duration_secs\":%lld"
                   ",\n\t\t\"single_point_pages\":%zu"
                   ",\n\t\t\"first_t\":%ld"
                   ",\n\t\t\"last_t\":%ld"
                   ",\n\t\t\"database_retention_secs\":%lld"
                   ",\n\t\t\"average_compression_savings\":%0.2f"
                   ",\n\t\t\"average_point_duration_secs\":%0.2f"
                   ",\n\t\t\"average_metric_retention_secs\":%0.2f"
                   ",\n\t\t\"ephemeral_metrics_per_day_percent\":%0.2f"
                   ",\n\t\t\"average_page_size_bytes\":%0.2f"
                   ",\n\t\t\"estimated_concurrently_collected_metrics\":%zu"
                   ",\n\t\t\"currently_collected_metrics\":%zu"
                   ",\n\t\t\"disk_space\":%zu"
                   ",\n\t\t\"max_disk_space\":%zu"
                   , stats.default_granularity_secs
                   , stats.sizeof_datafile
                   , stats.sizeof_page_in_cache
                   , stats.sizeof_point_data
                   , stats.sizeof_page_data
                   , stats.pages_per_extent
                   , stats.datafiles
                   , stats.extents
                   , stats.extents_pages
                   , stats.points
                   , stats.metrics
                   , stats.metrics_pages
                   , stats.extents_compressed_bytes
                   , stats.pages_uncompressed_bytes
                   , (long long)stats.pages_duration_secs
                   , stats.single_point_pages
                   , stats.first_time_s
                   , stats.last_time_s
                   , (long long)stats.database_retention_secs
                   , stats.average_compression_savings
                   , stats.average_point_duration_secs
                   , stats.average_metric_retention_secs
                   , stats.ephemeral_metrics_per_day_percent
                   , stats.average_page_size_bytes
                   , stats.estimated_concurrently_collected_metrics
                   , stats.currently_collected_metrics
                   , stats.disk_space
                   , stats.max_disk_space
                   );
}
int web_client_api_request_v1_dbengine_stats(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);

    if(!dbengine_enabled) {
        buffer_strcat(wb, "dbengine is not enabled");
        return HTTP_RESP_NOT_FOUND;
    }

    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
    buffer_strcat(wb, "{");
    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        buffer_sprintf(wb, "%s\n\t\"tier%zu\": {", tier?",":"", tier);
        web_client_api_v1_dbengine_stats_for_tier(wb, tier);
        buffer_strcat(wb, "\n\t}");
    }
    buffer_strcat(wb, "\n}");

    return HTTP_RESP_OK;
}
#endif

#ifdef NETDATA_DEV_MODE
#define ACL_DEV_OPEN_ACCESS WEB_CLIENT_ACL_DASHBOARD
#else
#define ACL_DEV_OPEN_ACCESS 0
#endif

static struct web_api_command api_commands_v1[] = {
        { "info",            0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_info                       },
        { "data",            0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_data                       },
        { "chart",           0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_chart                      },
        { "charts",          0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_charts                     },
        { "context",         0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_context                    },
        { "contexts",        0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_contexts                   },

        // registry checks the ACL by itself, so we allow everything
        { "registry",        0, WEB_CLIENT_ACL_NOCHECK,                         web_client_api_request_v1_registry                   },

        // badges can be fetched with both dashboard and badge permissions
        { "badge.svg",       0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_BADGE | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_badge },

        { "alarms",          0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_alarms                     },
        { "alarms_values",   0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_alarms_values             },
        { "alarm_log",       0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_alarm_log                 },
        { "alarm_variables", 0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_alarm_variables           },
        { "alarm_count",     0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_alarm_count               },
        { "allmetrics",      0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_allmetrics                },

#if defined(ENABLE_ML)
        { "ml_info",         0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_ml_info            },
        { "ml_models",       0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_ml_models          },
#endif

        { "manage/health",       0, WEB_CLIENT_ACL_MGMT | WEB_CLIENT_ACL_ACLK,      web_client_api_request_v1_mgmt_health           },
        { "aclk",                0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_aclk_state            },
        { "metric_correlations", 0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_metric_correlations   },
        { "weights",             0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_weights               },

        { "function",            0, WEB_CLIENT_ACL_ACLK | ACL_DEV_OPEN_ACCESS, web_client_api_request_v1_function },
        { "functions",            0, WEB_CLIENT_ACL_ACLK | ACL_DEV_OPEN_ACCESS, web_client_api_request_v1_functions },

        { "dbengine_stats",      0, WEB_CLIENT_ACL_DASHBOARD | WEB_CLIENT_ACL_ACLK, web_client_api_request_v1_dbengine_stats },

        // terminator
        { NULL,              0, WEB_CLIENT_ACL_NONE,      NULL },
};

inline int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v1[i].command ; i++)
            api_commands_v1[i].hash = simple_hash(api_commands_v1[i].command);
    }

    return web_client_api_request_vX(host, w, url, api_commands_v1);
}
