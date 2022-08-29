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
        , {"showcustomvars"    , 0    , RRDR_OPTION_CUSTOM_VARS}
        , {"anomaly-bit"       , 0    , RRDR_OPTION_ANOMALY_BIT}
        , {"selected-tier"     , 0    , RRDR_OPTION_SELECTED_TIER}
        , {"raw"               , 0    , RRDR_OPTION_RETURN_RAW}
        , {"jw-anomaly-rates"  , 0    , RRDR_OPTION_RETURN_JWAR}
        , {"natural-points"    , 0    , RRDR_OPTION_NATURAL_POINTS}
        , {"virtual-points"    , 0    , RRDR_OPTION_VIRTUAL_POINTS}
        , {NULL                , 0    , 0}
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
        , {DATASOURCE_FORMAT_CSV_MARKDOWN   , 0 , DATASOURCE_CSV_MARKDOWN}
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

    for(i = 0; api_v1_data_options[i].name ; i++)
        api_v1_data_options[i].hash = simple_hash(api_v1_data_options[i].name);

    for(i = 0; api_v1_data_formats[i].name ; i++)
        api_v1_data_formats[i].hash = simple_hash(api_v1_data_formats[i].name);

    for(i = 0; api_v1_data_google_formats[i].name ; i++)
        api_v1_data_google_formats[i].hash = simple_hash(api_v1_data_google_formats[i].name);

    web_client_api_v1_init_grouping();

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

void web_client_api_request_v1_data_options_to_string(BUFFER *wb, RRDR_OPTIONS options) {
    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    int added = 0;
    for(int i = 0; api_v1_data_options[i].name ; i++) {
        if (unlikely((api_v1_data_options[i].value & options) && !(api_v1_data_options[i].value & used))) {
            if(added) buffer_strcat(wb, ",");
            buffer_strcat(wb, api_v1_data_options[i].name);
            used |= api_v1_data_options[i].value;
            added++;
        }
    }
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
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarms2json(host, w->response.data, all);
    buffer_no_cacheable(w->response.data);
    return HTTP_RESP_OK;
}

inline int web_client_api_request_v1_alarms_values(RRDHOST *host, struct web_client *w, char *url) {
    int all = web_client_api_request_v1_alarms_select(url);

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
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
            if(!contexts) contexts = buffer_create(255);
            buffer_strcat(contexts, "|");
            buffer_strcat(contexts, value);
        }
    }

    health_aggregate_alarms(host, w->response.data, contexts, status);

    buffer_sprintf(w->response.data, "]\n");
    w->response.data->contenttype = CT_APPLICATION_JSON;
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
    w->response.data->contenttype = CT_APPLICATION_JSON;
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

    w->response.data->contenttype = CT_APPLICATION_JSON;
    st->last_accessed_time = now_realtime_sec();
    callback(st, w->response.data);
    return HTTP_RESP_OK;

    cleanup:
    return ret;
}

inline int web_client_api_request_v1_alarm_variables(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_single_chart(host, w, url, health_api_v1_chart_variables2json);
}

static RRDCONTEXT_TO_JSON_OPTIONS rrdcontext_to_json_parse_options(char *o) {
    RRDCONTEXT_TO_JSON_OPTIONS options = RRDCONTEXT_OPTION_NONE;
    char *tok;

    while(o && *o && (tok = mystrsep(&o, ", |"))) {
        if(!*tok) continue;

        if(!strcmp(tok, "full") || !strcmp(tok, "all"))
            options |= RRDCONTEXT_OPTIONS_ALL;
        else if(!strcmp(tok, "charts") || !strcmp(tok, "instances"))
            options |= RRDCONTEXT_OPTION_SHOW_INSTANCES;
        else if(!strcmp(tok, "dimensions") || !strcmp(tok, "metrics"))
            options |= RRDCONTEXT_OPTION_SHOW_METRICS;
        else if(!strcmp(tok, "queue"))
            options |= RRDCONTEXT_OPTION_SHOW_QUEUED;
        else if(!strcmp(tok, "flags"))
            options |= RRDCONTEXT_OPTION_SHOW_FLAGS;
        else if(!strcmp(tok, "uuids"))
            options |= RRDCONTEXT_OPTION_SHOW_UUIDS;
        else if(!strcmp(tok, "deleted"))
            options |= RRDCONTEXT_OPTION_SHOW_DELETED;
        else if(!strcmp(tok, "labels"))
            options |= RRDCONTEXT_OPTION_SHOW_LABELS;
        else if(!strcmp(tok, "deepscan"))
            options |= RRDCONTEXT_OPTION_DEEPSCAN;
        else if(!strcmp(tok, "hidden"))
            options |= RRDCONTEXT_OPTION_SHOW_HIDDEN;
    }

    return options;
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
            if(!dimensions) dimensions = buffer_create(100);
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
        chart_label_key_pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

    if(chart_labels_filter)
        chart_labels_filter_pattern = simple_pattern_create(chart_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

    if(dimensions) {
        chart_dimensions_pattern = simple_pattern_create(buffer_tostring(dimensions), ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);
        buffer_free(dimensions);
    }

    w->response.data->contenttype = CT_APPLICATION_JSON;
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
            if(!dimensions) dimensions = buffer_create(100);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
    }

    SIMPLE_PATTERN *chart_label_key_pattern = NULL;
    SIMPLE_PATTERN *chart_labels_filter_pattern = NULL;
    SIMPLE_PATTERN *chart_dimensions_pattern = NULL;

    if(chart_label_key)
        chart_label_key_pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

    if(chart_labels_filter)
        chart_labels_filter_pattern = simple_pattern_create(chart_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

    if(dimensions) {
        chart_dimensions_pattern = simple_pattern_create(buffer_tostring(dimensions), ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);
        buffer_free(dimensions);
    }

    w->response.data->contenttype = CT_APPLICATION_JSON;
    int ret = rrdcontexts_to_json(host, w->response.data, after, before, options, chart_label_key_pattern, chart_labels_filter_pattern, chart_dimensions_pattern);

    simple_pattern_free(chart_label_key_pattern);
    simple_pattern_free(chart_labels_filter_pattern);
    simple_pattern_free(chart_dimensions_pattern);

    return ret;
}

inline int web_client_api_request_v1_charts(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    charts2json(host, w->response.data, 0, 0);
    return HTTP_RESP_OK;
}

inline int web_client_api_request_v1_archivedcharts(RRDHOST *host __maybe_unused, struct web_client *w, char *url) {
    (void)url;

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
#ifdef ENABLE_DBENGINE
    if (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
        sql_rrdset2json(host, w->response.data);
#endif
    return HTTP_RESP_OK;
}

inline int web_client_api_request_v1_chart(RRDHOST *host, struct web_client *w, char *url) {
    return web_client_api_request_single_chart(host, w, url, rrd_stats_api_v1_chart);
}

void fix_google_param(char *s) {
    if(unlikely(!s)) return;

    for( ; *s ;s++) {
        if(!isalnum(*s) && *s != '.' && *s != '_' && *s != '-')
            *s = '_';
    }
}


// returns the HTTP code
inline int web_client_api_request_v1_data(RRDHOST *host, struct web_client *w, char *url) {
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
    char *max_anomaly_rates_str = NULL;
    char *context = NULL;
    char *chart_label_key = NULL;
    char *chart_labels_filter = NULL;
    char *group_options = NULL;
    int tier = 0;
    int group = RRDR_GROUPING_AVERAGE;
    int show_dimensions = 0;
    uint32_t format = DATASOURCE_JSON;
    uint32_t options = 0x00000000;

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
            if(!dimensions) dimensions = buffer_create(100);
            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "show_dimensions")) show_dimensions = 1;
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "timeout")) timeout_str = value;
        else if(!strcmp(name, "gtime")) group_time_str = value;
        else if(!strcmp(name, "group_options")) group_options = value;
        else if(!strcmp(name, "group")) {
            group = web_client_api_request_v1_data_group(value, RRDR_GROUPING_AVERAGE);
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
        else if(!strcmp(name, "max_anomaly_rates")) {
            max_anomaly_rates_str = value;
        }
        else if(!strcmp(name, "tier")) {
            tier = str2i(value);
            if(tier >= 0 && tier < storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
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

    if((!chart || !*chart) && (!context)) {
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    struct context_param  *context_param_list = NULL;

    if (context && !chart) {
        RRDSET *st1;

        SIMPLE_PATTERN *chart_label_key_pattern = NULL;
        if(chart_label_key)
            chart_label_key_pattern = simple_pattern_create(chart_label_key, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

        SIMPLE_PATTERN *chart_labels_filter_pattern = NULL;
        if(chart_labels_filter)
            chart_labels_filter_pattern = simple_pattern_create(chart_labels_filter, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT);

        STRING *context_string = string_strdupz(context);
        rrdhost_rdlock(host);
        rrdset_foreach_read(st1, host) {
            if (st1->context == context_string &&
                (!chart_label_key_pattern || rrdlabels_match_simple_pattern_parsed(st1->state->chart_labels, chart_label_key_pattern, ':')) &&
                (!chart_labels_filter_pattern || rrdlabels_match_simple_pattern_parsed(st1->state->chart_labels, chart_labels_filter_pattern, ':')))
                    build_context_param_list(owa, &context_param_list, st1);
        }
        rrdhost_unlock(host);
        string_freez(context_string);

        if (likely(context_param_list && context_param_list->rd))  // Just set the first one
            st = context_param_list->rd->rrdset;
        else {
            if (!chart_label_key && !chart_labels_filter)
                sql_build_context_param_list(owa, &context_param_list, host, context, NULL);
        }
    }
    else {
        st = rrdset_find(host, chart);
        if (!st)
            st = rrdset_find_byname(host, chart);
        if (likely(st))
            st->last_accessed_time = now_realtime_sec();
        else
            sql_build_context_param_list(owa, &context_param_list, host, NULL, chart);
    }

    if (!st) {
        if (likely(context_param_list && context_param_list->rd && context_param_list->rd->rrdset))
            st = context_param_list->rd->rrdset;
        else {
            free_context_param_list(owa, &context_param_list);
            context_param_list = NULL;
        }
    }

    if (!st && !context_param_list) {
        if (context && !chart) {
            if (!chart_label_key) {
                buffer_strcat(w->response.data, "Context is not found: ");
                buffer_strcat_htmlescape(w->response.data, context);
            } else {
                buffer_strcat(w->response.data, "Context: ");
                buffer_strcat_htmlescape(w->response.data, context);
                buffer_strcat(w->response.data, " or chart label key: ");
                buffer_strcat_htmlescape(w->response.data, chart_label_key);
                buffer_strcat(w->response.data, " not found");
            }
        }
        else {
            buffer_strcat(w->response.data, "Chart is not found: ");
            buffer_strcat_htmlescape(w->response.data, chart);
        }
        ret = HTTP_RESP_NOT_FOUND;
        goto cleanup;
    }

    long long before = (before_str && *before_str)?str2l(before_str):0;
    long long after  = (after_str  && *after_str) ?str2l(after_str):-600;
    int       points = (points_str && *points_str)?str2i(points_str):0;
    int       timeout = (timeout_str && *timeout_str)?str2i(timeout_str): 0;
    long      group_time = (group_time_str && *group_time_str)?str2l(group_time_str):0;
    int       max_anomaly_rates = (max_anomaly_rates_str && *max_anomaly_rates_str) ? str2i(max_anomaly_rates_str) : 0;

    if (timeout) {
        struct timeval now;
        now_realtime_timeval(&now);
        int inqueue = (int)dt_usec(&w->tv_in, &now) / 1000;
        timeout -= inqueue;
        if (timeout <= 0) {
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, "Query timeout exceeded");
            return HTTP_RESP_BACKEND_FETCH_FAILED;
        }
    }

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

    QUERY_PARAMS query_params = {
        .context_param_list = context_param_list,
        .timeout = timeout,
        .max_anomaly_rates = max_anomaly_rates,
        .show_dimensions = show_dimensions,
        .chart_label_key = chart_label_key,
        .wb = w->response.data};

    ret = rrdset2anything_api_v1(owa, st, &query_params, dimensions, format,
            points, after, before, group, group_options, group_time, options, &last_timestamp_in_data, tier);

    free_context_param_list(owa, &context_param_list);

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
    int alarm_normal = 0, alarm_warn = 0, alarm_crit = 0;
    RRDCALC *rc;
    rrdhost_rdlock(host);
    for(rc = host->alarms; rc ; rc = rc->next) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        switch(rc->status) {
            case RRDCALC_STATUS_WARNING:
                alarm_warn++;
                break;
            case RRDCALC_STATUS_CRITICAL:
                alarm_crit++;
                break;
            default:
                alarm_normal++;
        }
    }
    rrdhost_unlock(host);
    buffer_sprintf(wb, "\t\t\"normal\": %d,\n", alarm_normal);
    buffer_sprintf(wb, "\t\t\"warning\": %d,\n", alarm_warn);
    buffer_sprintf(wb, "\t\t\"critical\": %d\n", alarm_crit);
}

static inline void web_client_api_request_v1_info_mirrored_hosts(BUFFER *wb) {
    RRDHOST *host;
    int count = 0;

    buffer_strcat(wb, "\t\"mirrored_hosts\": [\n");
    rrd_rdlock();
    rrdhost_foreach_read(host) {
        if (count > 0)
            buffer_strcat(wb, ",\n");

        buffer_sprintf(wb, "\t\t\"%s\"", rrdhost_hostname(host));
        count++;
    }

    buffer_strcat(wb, "\n\t],\n\t\"mirrored_hosts_status\": [\n");
    count = 0;
    rrdhost_foreach_read(host)
    {
        if (count > 0)
            buffer_strcat(wb, ",\n");

        netdata_mutex_lock(&host->receiver_lock);
        buffer_sprintf(
            wb, "\t\t{ \"guid\": \"%s\", \"hostname\": \"%s\", \"reachable\": %s, \"hops\": %d"
            , host->machine_guid
            , rrdhost_hostname(host)
            , (host->receiver || host == localhost) ? "true" : "false"
            , host->system_info ? host->system_info->hops : (host == localhost) ? 0 : 1
            );
        netdata_mutex_unlock(&host->receiver_lock);

        rrdhost_aclk_state_lock(host);
        if (host->aclk_state.claimed_id)
            buffer_sprintf(wb, ", \"claim_id\": \"%s\"", host->aclk_state.claimed_id);
        else
            buffer_strcat(wb, ", \"claim_id\": null");
        rrdhost_aclk_state_unlock(host);

        if (host->node_id) {
            char node_id_str[GUID_LEN + 1];
            uuid_unparse_lower(*host->node_id, node_id_str);
            buffer_sprintf(wb, ", \"node_id\": \"%s\" }", node_id_str);
        } else
            buffer_strcat(wb, ", \"node_id\": null }");

        count++;
    }
    rrd_unlock();

    buffer_strcat(wb, "\n\t],\n");
}

inline void host_labels2json(RRDHOST *host, BUFFER *wb, size_t indentation) {
    char tabs[11];

    if (indentation > 10)
        indentation = 10;

    tabs[0] = '\0';
    while (indentation) {
        strcat(tabs, "\t");
        indentation--;
    }

    rrdlabels_to_buffer(host->host_labels, wb, tabs, ":", "\"", ",\n", NULL, NULL, NULL, NULL);
    buffer_strcat(wb, "\n");
}

extern int aclk_connected;
inline int web_client_api_request_v1_info_fill_buffer(RRDHOST *host, BUFFER *wb)
{
    buffer_strcat(wb, "{\n");
    buffer_sprintf(wb, "\t\"version\": \"%s\",\n", rrdhost_program_version(host));
    buffer_sprintf(wb, "\t\"uid\": \"%s\",\n", host->machine_guid);

    web_client_api_request_v1_info_mirrored_hosts(wb);

    buffer_strcat(wb, "\t\"alarms\": {\n");
    web_client_api_request_v1_info_summary_alarm_statuses(host, wb);
    buffer_strcat(wb, "\t},\n");

    buffer_sprintf(wb, "\t\"os_name\": \"%s\",\n", (host->system_info->host_os_name) ? host->system_info->host_os_name : "");
    buffer_sprintf(wb, "\t\"os_id\": \"%s\",\n", (host->system_info->host_os_id) ? host->system_info->host_os_id : "");
    buffer_sprintf(wb, "\t\"os_id_like\": \"%s\",\n", (host->system_info->host_os_id_like) ? host->system_info->host_os_id_like : "");
    buffer_sprintf(wb, "\t\"os_version\": \"%s\",\n", (host->system_info->host_os_version) ? host->system_info->host_os_version : "");
    buffer_sprintf(wb, "\t\"os_version_id\": \"%s\",\n", (host->system_info->host_os_version_id) ? host->system_info->host_os_version_id : "");
    buffer_sprintf(wb, "\t\"os_detection\": \"%s\",\n", (host->system_info->host_os_detection) ? host->system_info->host_os_detection : "");
    buffer_sprintf(wb, "\t\"cores_total\": \"%s\",\n", (host->system_info->host_cores) ? host->system_info->host_cores : "");
    buffer_sprintf(wb, "\t\"total_disk_space\": \"%s\",\n", (host->system_info->host_disk_space) ? host->system_info->host_disk_space : "");
    buffer_sprintf(wb, "\t\"cpu_freq\": \"%s\",\n", (host->system_info->host_cpu_freq) ? host->system_info->host_cpu_freq : "");
    buffer_sprintf(wb, "\t\"ram_total\": \"%s\",\n", (host->system_info->host_ram_total) ? host->system_info->host_ram_total : "");

    if (host->system_info->container_os_name)
        buffer_sprintf(wb, "\t\"container_os_name\": \"%s\",\n", host->system_info->container_os_name);
    if (host->system_info->container_os_id)
        buffer_sprintf(wb, "\t\"container_os_id\": \"%s\",\n", host->system_info->container_os_id);
    if (host->system_info->container_os_id_like)
        buffer_sprintf(wb, "\t\"container_os_id_like\": \"%s\",\n", host->system_info->container_os_id_like);
    if (host->system_info->container_os_version)
        buffer_sprintf(wb, "\t\"container_os_version\": \"%s\",\n", host->system_info->container_os_version);
    if (host->system_info->container_os_version_id)
        buffer_sprintf(wb, "\t\"container_os_version_id\": \"%s\",\n", host->system_info->container_os_version_id);
    if (host->system_info->container_os_detection)
        buffer_sprintf(wb, "\t\"container_os_detection\": \"%s\",\n", host->system_info->container_os_detection);
    if (host->system_info->is_k8s_node)
        buffer_sprintf(wb, "\t\"is_k8s_node\": \"%s\",\n", host->system_info->is_k8s_node);

    buffer_sprintf(wb, "\t\"kernel_name\": \"%s\",\n", (host->system_info->kernel_name) ? host->system_info->kernel_name : "");
    buffer_sprintf(wb, "\t\"kernel_version\": \"%s\",\n", (host->system_info->kernel_version) ? host->system_info->kernel_version : "");
    buffer_sprintf(wb, "\t\"architecture\": \"%s\",\n", (host->system_info->architecture) ? host->system_info->architecture : "");
    buffer_sprintf(wb, "\t\"virtualization\": \"%s\",\n", (host->system_info->virtualization) ? host->system_info->virtualization : "");
    buffer_sprintf(wb, "\t\"virt_detection\": \"%s\",\n", (host->system_info->virt_detection) ? host->system_info->virt_detection : "");
    buffer_sprintf(wb, "\t\"container\": \"%s\",\n", (host->system_info->container) ? host->system_info->container : "");
    buffer_sprintf(wb, "\t\"container_detection\": \"%s\",\n", (host->system_info->container_detection) ? host->system_info->container_detection : "");

    if (host->system_info->cloud_provider_type)
        buffer_sprintf(wb, "\t\"cloud_provider_type\": \"%s\",\n", host->system_info->cloud_provider_type);
    if (host->system_info->cloud_instance_type)
        buffer_sprintf(wb, "\t\"cloud_instance_type\": \"%s\",\n", host->system_info->cloud_instance_type);
    if (host->system_info->cloud_instance_region)
        buffer_sprintf(wb, "\t\"cloud_instance_region\": \"%s\",\n", host->system_info->cloud_instance_region);

    buffer_strcat(wb, "\t\"host_labels\": {\n");
    host_labels2json(host, wb, 2);
    buffer_strcat(wb, "\t},\n");

    buffer_strcat(wb, "\t\"collectors\": [");
    chartcollectors2json(host, wb);
    buffer_strcat(wb, "\n\t],\n");

#ifdef DISABLE_CLOUD
    buffer_strcat(wb, "\t\"cloud-enabled\": false,\n");
#else
    buffer_sprintf(wb, "\t\"cloud-enabled\": %s,\n",
                   appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", 1) ? "true" : "false");
#endif

#ifdef ENABLE_ACLK
    buffer_strcat(wb, "\t\"cloud-available\": true,\n");
#else
    buffer_strcat(wb, "\t\"cloud-available\": false,\n");
#endif
    char *agent_id = get_agent_claimid();
    if (agent_id == NULL)
        buffer_strcat(wb, "\t\"agent-claimed\": false,\n");
    else {
        buffer_strcat(wb, "\t\"agent-claimed\": true,\n");
        freez(agent_id);
    }
#ifdef ENABLE_ACLK
    if (aclk_connected) {
        buffer_strcat(wb, "\t\"aclk-available\": true,\n");
    }
    else
#endif
        buffer_strcat(wb, "\t\"aclk-available\": false,\n");     // Intentionally valid with/without #ifdef above

    buffer_strcat(wb, "\t\"memory-mode\": ");
    analytics_get_data(analytics_data.netdata_config_memory_mode, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"multidb-disk-quota\": ");
    analytics_get_data(analytics_data.netdata_config_multidb_disk_quota, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"page-cache-size\": ");
    analytics_get_data(analytics_data.netdata_config_page_cache_size, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"stream-enabled\": ");
    analytics_get_data(analytics_data.netdata_config_stream_enabled, wb);
    buffer_strcat(wb, ",\n");

#ifdef  ENABLE_COMPRESSION
    if(host->sender){
        buffer_strcat(wb, "\t\"stream-compression\": ");
        buffer_strcat(wb, (host->sender->rrdpush_compression ? "true" : "false"));
        buffer_strcat(wb, ",\n");
    }else{
        buffer_strcat(wb, "\t\"stream-compression\": null,\n");
    }
#else
    buffer_strcat(wb, "\t\"stream-compression\": null,\n");
#endif  //ENABLE_COMPRESSION   

    buffer_strcat(wb, "\t\"hosts-available\": ");
    analytics_get_data(analytics_data.netdata_config_hosts_available, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"https-enabled\": ");
    analytics_get_data(analytics_data.netdata_config_https_enabled, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"buildinfo\": ");
    analytics_get_data(analytics_data.netdata_buildinfo, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"release-channel\": ");
    analytics_get_data(analytics_data.netdata_config_release_channel, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"web-enabled\": ");
    analytics_get_data(analytics_data.netdata_config_web_enabled, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"notification-methods\": ");
    analytics_get_data(analytics_data.netdata_notification_methods, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"exporting-enabled\": ");
    analytics_get_data(analytics_data.netdata_config_exporting_enabled, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"exporting-connectors\": ");
    analytics_get_data(analytics_data.netdata_exporting_connectors, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"allmetrics-prometheus-used\": ");
    analytics_get_data(analytics_data.netdata_allmetrics_prometheus_used, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"allmetrics-shell-used\": ");
    analytics_get_data(analytics_data.netdata_allmetrics_shell_used, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"allmetrics-json-used\": ");
    analytics_get_data(analytics_data.netdata_allmetrics_json_used, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"dashboard-used\": ");
    analytics_get_data(analytics_data.netdata_dashboard_used, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"charts-count\": ");
    analytics_get_data(analytics_data.netdata_charts_count, wb);
    buffer_strcat(wb, ",\n");

    buffer_strcat(wb, "\t\"metrics-count\": ");
    analytics_get_data(analytics_data.netdata_metrics_count, wb);

#if defined(ENABLE_ML)
    buffer_strcat(wb, ",\n");
    char *ml_info = ml_get_host_info(host);

    buffer_strcat(wb, "\t\"ml-info\": ");
    buffer_strcat(wb, ml_info);

    free(ml_info);
#endif

    buffer_strcat(wb, "\n}");
    return 0;
}

#if defined(ENABLE_ML)
int web_client_api_request_v1_anomaly_events(RRDHOST *host, struct web_client *w, char *url) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    uint32_t after = 0, before = 0;

    while (url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value)
            continue;

        char *name = mystrsep(&value, "=");
        if (!name || !*name)
            continue;
        if (!value || !*value)
            continue;

        if (!strcmp(name, "after"))
            after = (uint32_t) (strtoul(value, NULL, 0) / 1000);
        else if (!strcmp(name, "before"))
            before = (uint32_t) (strtoul(value, NULL, 0) / 1000);
    }

    char *s;
    if (!before || !after)
        s = strdupz("{\"error\": \"missing after/before parameters\" }\n");
    else {
        s = ml_get_anomaly_events(host, "AD1", 1, after, before);
        if (!s)
            s = strdupz("{\"error\": \"json string is empty\" }\n");
    }

    BUFFER *wb = w->response.data;
    buffer_flush(wb);

    wb->contenttype = CT_APPLICATION_JSON;
    buffer_strcat(wb, s);
    buffer_no_cacheable(wb);

    freez(s);

    return HTTP_RESP_OK;
}

int web_client_api_request_v1_anomaly_event_info(RRDHOST *host, struct web_client *w, char *url) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    uint32_t after = 0, before = 0;

    while (url) {
        char *value = mystrsep(&url, "&");
        if (!value || !*value)
            continue;

        char *name = mystrsep(&value, "=");
        if (!name || !*name)
            continue;
        if (!value || !*value)
            continue;

        if (!strcmp(name, "after"))
            after = (uint32_t) strtoul(value, NULL, 0);
        else if (!strcmp(name, "before"))
            before = (uint32_t) strtoul(value, NULL, 0);
    }

    char *s;
    if (!before || !after)
        s = strdupz("{\"error\": \"missing after/before parameters\" }\n");
    else {
        s = ml_get_anomaly_event_info(host, "AD1", 1, after, before);
        if (!s)
            s = strdupz("{\"error\": \"json string is empty\" }\n");
    }

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_APPLICATION_JSON;
    buffer_strcat(wb, s);
    buffer_no_cacheable(wb);

    freez(s);
    return HTTP_RESP_OK;
}

int web_client_api_request_v1_ml_info(RRDHOST *host, struct web_client *w, char *url) {
    (void) url;

    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    char *s = ml_get_host_runtime_info(host);
    if (!s)
        s = strdupz("{\"error\": \"json string is empty\" }\n");

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_APPLICATION_JSON;
    buffer_strcat(wb, s);
    buffer_no_cacheable(wb);

    freez(s);
    return HTTP_RESP_OK;
}

#endif // defined(ENABLE_ML)

inline int web_client_api_request_v1_info(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;
    if (!netdata_ready) return HTTP_RESP_BACKEND_FETCH_FAILED;
    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_APPLICATION_JSON;

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

    wb->contenttype = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
    return HTTP_RESP_OK;
}

static int web_client_api_request_v1_weights_internal(RRDHOST *host, struct web_client *w, char *url, WEIGHTS_METHOD method, WEIGHTS_FORMAT format) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    long long baseline_after = 0, baseline_before = 0, after = 0, before = 0, points = 0;
    RRDR_OPTIONS options = RRDR_OPTION_NOT_ALIGNED | RRDR_OPTION_NONZERO | RRDR_OPTION_NULL2ZERO;
    int options_count = 0;
    RRDR_GROUPING group = RRDR_GROUPING_AVERAGE;
    int timeout = 0;
    int tier = 0;
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
            group = web_client_api_request_v1_data_group(value, RRDR_GROUPING_AVERAGE);

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
            tier = str2i(value);
            if(tier >= 0 && tier < storage_tiers)
                options |= RRDR_OPTION_SELECTED_TIER;
        }
    }

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_APPLICATION_JSON;

    SIMPLE_PATTERN *contexts = (contexts_str) ? simple_pattern_create(contexts_str, ",|\t\r\n\f\v", SIMPLE_PATTERN_EXACT) : NULL;

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

#ifndef ENABLE_DBENGINE
int web_client_api_request_v1_dbengine_stats(RRDHOST *host, struct web_client *w, char *url) {
    return HTTP_RESP_NOT_FOUND;
}
#else
static void web_client_api_v1_dbengine_stats_for_tier(BUFFER *wb, int tier) {
    RRDENG_SIZE_STATS stats = rrdeng_size_statistics(multidb_ctx[tier]);

    buffer_sprintf(wb,
                   "\n\t\t\"default_granularity_secs\":%zu"
                   ",\n\t\t\"sizeof_metric\":%zu"
                   ",\n\t\t\"sizeof_metric_in_index\":%zu"
                   ",\n\t\t\"sizeof_page\":%zu"
                   ",\n\t\t\"sizeof_page_in_index\":%zu"
                   ",\n\t\t\"sizeof_extent\":%zu"
                   ",\n\t\t\"sizeof_page_in_extent\":%zu"
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
                   ",\n\t\t\"pages_duration_secs\":%ld"
                   ",\n\t\t\"single_point_pages\":%zu"
                   ",\n\t\t\"first_t\":%llu"
                   ",\n\t\t\"last_t\":%llu"
                   ",\n\t\t\"database_retention_secs\":%ld"
                   ",\n\t\t\"average_compression_savings\":%0.2f"
                   ",\n\t\t\"average_point_duration_secs\":%0.2f"
                   ",\n\t\t\"average_metric_retention_secs\":%0.2f"
                   ",\n\t\t\"ephemeral_metrics_per_day_percent\":%0.2f"
                   ",\n\t\t\"average_page_size_bytes\":%0.2f"
                   ",\n\t\t\"estimated_concurrently_collected_metrics\":%zu"
                   ",\n\t\t\"currently_collected_metrics\":%zu"
                   ",\n\t\t\"max_concurrently_collected_metrics\":%zu"
                   ",\n\t\t\"disk_space\":%zu"
                   ",\n\t\t\"max_disk_space\":%zu"
                   , stats.default_granularity_secs
                   , stats.sizeof_metric
                   , stats.sizeof_metric_in_index
                   , stats.sizeof_page
                   , stats.sizeof_page_in_index
                   , stats.sizeof_extent
                   , stats.sizeof_page_in_extent
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
                   , stats.pages_duration_secs
                   , stats.single_point_pages
                   , stats.first_t
                   , stats.last_t
                   , stats.database_retention_secs
                   , stats.average_compression_savings
                   , stats.average_point_duration_secs
                   , stats.average_metric_retention_secs
                   , stats.ephemeral_metrics_per_day_percent
                   , stats.average_page_size_bytes
                   , stats.estimated_concurrently_collected_metrics
                   , stats.currently_collected_metrics
                   , stats.max_concurrently_collected_metrics
                   , stats.disk_space
                   , stats.max_disk_space
                   );
}
int web_client_api_request_v1_dbengine_stats(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused) {
    if (!netdata_ready)
        return HTTP_RESP_BACKEND_FETCH_FAILED;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);

    buffer_strcat(wb, "{");
    for(int tier = 0; tier < storage_tiers ;tier++) {
        buffer_sprintf(wb, "%s\n\t\"tier%d\": {", tier?",":"", tier);
        web_client_api_v1_dbengine_stats_for_tier(wb, tier);
        buffer_strcat(wb, "\n\t}");
    }
    buffer_strcat(wb, "\n}");

    return HTTP_RESP_OK;
}
#endif

static struct api_command {
    const char *command;
    uint32_t hash;
    WEB_CLIENT_ACL acl;
    int (*callback)(RRDHOST *host, struct web_client *w, char *url);
} api_commands[] = {
        { "info",            0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_info            },
        { "data",            0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_data            },
        { "chart",           0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_chart           },
        { "charts",          0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_charts          },
        { "context",         0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_context         },
        { "contexts",        0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_contexts        },
        { "archivedcharts",  0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_archivedcharts  },

        // registry checks the ACL by itself, so we allow everything
        { "registry",        0, WEB_CLIENT_ACL_NOCHECK,   web_client_api_request_v1_registry        },

        // badges can be fetched with both dashboard and badge permissions
        { "badge.svg",       0, WEB_CLIENT_ACL_DASHBOARD|WEB_CLIENT_ACL_BADGE, web_client_api_request_v1_badge },

        { "alarms",          0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarms          },
        { "alarms_values",   0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarms_values   },
        { "alarm_log",       0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_log       },
        { "alarm_variables", 0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_variables },
        { "alarm_count",     0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_count     },
        { "allmetrics",      0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_allmetrics      },

#if defined(ENABLE_ML)
        { "anomaly_events",     0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_anomaly_events     },
        { "anomaly_event_info", 0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_anomaly_event_info },
        { "ml_info",            0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_ml_info            },
#endif

        { "manage/health",       0, WEB_CLIENT_ACL_MGMT,      web_client_api_request_v1_mgmt_health         },
        { "aclk",                0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_aclk_state          },
        { "metric_correlations", 0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_metric_correlations },
        { "weights",             0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_weights },

        { "dbengine_stats",      0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_dbengine_stats },

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
    if(url) {
        debug(D_WEB_CLIENT, "%llu: Searching for API v1 command '%s'.", w->id, url);
        uint32_t hash = simple_hash(url);

        for(i = 0; api_commands[i].command ;i++) {
            if(unlikely(hash == api_commands[i].hash && !strcmp(url, api_commands[i].command))) {
                if(unlikely(api_commands[i].acl != WEB_CLIENT_ACL_NOCHECK) &&  !(w->acl & api_commands[i].acl))
                    return web_client_permission_denied(w);

                //return api_commands[i].callback(host, w, url);
                return api_commands[i].callback(host, w, (w->decoded_query_string + 1));
            }
        }

        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Unsupported v1 API command: ");
        buffer_strcat_htmlescape(w->response.data, url);
        return HTTP_RESP_NOT_FOUND;
    }
    else {
        buffer_flush(w->response.data);
        buffer_sprintf(w->response.data, "Which API v1 command?");
        return HTTP_RESP_BAD_REQUEST;
    }
}
