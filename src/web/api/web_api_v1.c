// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v1.h"
#include "v3/api_v3_calls.h"
#include "v1/api_v1_calls.h"

static struct {
    const char *name;
    uint32_t hash;
    CONTEXTS_V2_OPTIONS value;
} contexts_v2_options[] = {
          {"minify"           , 0    , CONTEXT_V2_OPTION_MINIFY}
        , {"debug"            , 0    , CONTEXT_V2_OPTION_DEBUG}
        , {"config"           , 0    , CONTEXT_V2_OPTION_ALERTS_WITH_CONFIGURATIONS}
        , {"instances"        , 0    , CONTEXT_V2_OPTION_ALERTS_WITH_INSTANCES}
        , {"values"           , 0    , CONTEXT_V2_OPTION_ALERTS_WITH_VALUES}
        , {"summary"          , 0    , CONTEXT_V2_OPTION_ALERTS_WITH_SUMMARY}
        , {NULL               , 0    , 0}
};

static struct {
    const char *name;
    uint32_t hash;
    CONTEXTS_V2_ALERT_STATUS value;
} contexts_v2_alert_status[] = {
          {"uninitialized"    , 0    , CONTEXT_V2_ALERT_UNINITIALIZED}
        , {"undefined"        , 0    , CONTEXT_V2_ALERT_UNDEFINED}
        , {"clear"            , 0    , CONTEXT_V2_ALERT_CLEAR}
        , {"raised"           , 0    , CONTEXT_V2_ALERT_RAISED}
        , {"active"           , 0    , CONTEXT_V2_ALERT_RAISED}
        , {"warning"          , 0    , CONTEXT_V2_ALERT_WARNING}
        , {"critical"         , 0    , CONTEXT_V2_ALERT_CRITICAL}
        , {NULL               , 0    , 0}
};

static struct {
    const char *name;
    uint32_t hash;
    DATASOURCE_FORMAT value;
} api_vX_data_formats[] = {
        {  DATASOURCE_FORMAT_DATATABLE_JSON , 0 , DATASOURCE_DATATABLE_JSON}
        , {DATASOURCE_FORMAT_DATATABLE_JSONP, 0 , DATASOURCE_DATATABLE_JSONP}
        , {DATASOURCE_FORMAT_JSON           , 0 , DATASOURCE_JSON}
        , {DATASOURCE_FORMAT_JSON2          , 0 , DATASOURCE_JSON2}
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

        // terminator
        , {NULL, 0, 0}
};

static struct {
    const char *name;
    uint32_t hash;
    DATASOURCE_FORMAT value;
} api_vX_data_google_formats[] = {
    // this is not an error - when Google requests json, it expects javascript
    // https://developers.google.com/chart/interactive/docs/dev/implementing_data_source#responseformat
      {"json",      0, DATASOURCE_DATATABLE_JSONP}
    , {"html",      0, DATASOURCE_HTML}
    , {"csv",       0, DATASOURCE_CSV}
    , {"tsv-excel", 0, DATASOURCE_TSV}

    // terminator
    , {NULL,        0, 0}
};

void nd_web_api_init(void) {
    int i;

    for(i = 0; contexts_v2_alert_status[i].name ; i++)
        contexts_v2_alert_status[i].hash = simple_hash(contexts_v2_alert_status[i].name);

    rrdr_option_init();

    for(i = 0; contexts_v2_options[i].name ; i++)
        contexts_v2_options[i].hash = simple_hash(contexts_v2_options[i].name);

    for(i = 0; api_vX_data_formats[i].name ; i++)
        api_vX_data_formats[i].hash = simple_hash(api_vX_data_formats[i].name);

    for(i = 0; api_vX_data_google_formats[i].name ; i++)
        api_vX_data_google_formats[i].hash = simple_hash(api_vX_data_google_formats[i].name);

    time_grouping_init();

    nd_uuid_t uuid;

	// generate
	uuid_generate(uuid);

	// unparse (to string)
	char uuid_str[37];
	uuid_unparse_lower(uuid, uuid_str);
}

inline CONTEXTS_V2_OPTIONS web_client_api_request_v2_context_options(char *o) {
    CONTEXTS_V2_OPTIONS ret = 0;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;

        uint32_t hash = simple_hash(tok);
        int i;
        for(i = 0; contexts_v2_options[i].name ; i++) {
            if (unlikely(hash == contexts_v2_options[i].hash && !strcmp(tok, contexts_v2_options[i].name))) {
                ret |= contexts_v2_options[i].value;
                break;
            }
        }
    }

    return ret;
}

CONTEXTS_V2_ALERT_STATUS web_client_api_request_v2_alert_status(char *o) {
    CONTEXTS_V2_ALERT_STATUS ret = 0;
    char *tok;

    while(o && *o && (tok = strsep_skip_consecutive_separators(&o, ", |"))) {
        if(!*tok) continue;

        uint32_t hash = simple_hash(tok);
        int i;
        for(i = 0; contexts_v2_alert_status[i].name ; i++) {
            if (unlikely(hash == contexts_v2_alert_status[i].hash && !strcmp(tok, contexts_v2_alert_status[i].name))) {
                ret |= contexts_v2_alert_status[i].value;
                break;
            }
        }
    }

    return ret;
}

void web_client_api_request_v2_contexts_alerts_status_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_V2_ALERT_STATUS options) {
    buffer_json_member_add_array(wb, key);

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; contexts_v2_alert_status[i].name ; i++) {
        if (unlikely((contexts_v2_alert_status[i].value & options) && !(contexts_v2_alert_status[i].value & used))) {
            const char *name = contexts_v2_alert_status[i].name;
            used |= contexts_v2_alert_status[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}

void web_client_api_request_v2_contexts_options_to_buffer_json_array(BUFFER *wb, const char *key, CONTEXTS_V2_OPTIONS options) {
    buffer_json_member_add_array(wb, key);

    RRDR_OPTIONS used = 0; // to prevent adding duplicates
    for(int i = 0; contexts_v2_options[i].name ; i++) {
        if (unlikely((contexts_v2_options[i].value & options) && !(contexts_v2_options[i].value & used))) {
            const char *name = contexts_v2_options[i].name;
            used |= contexts_v2_options[i].value;

            buffer_json_add_array_item_string(wb, name);
        }
    }

    buffer_json_array_close(wb);
}


inline uint32_t web_client_api_request_vX_data_format(char *name) {
    uint32_t hash = simple_hash(name);
    int i;

    for(i = 0; api_vX_data_formats[i].name ; i++) {
        if (unlikely(hash == api_vX_data_formats[i].hash && !strcmp(name, api_vX_data_formats[i].name))) {
            return api_vX_data_formats[i].value;
        }
    }

    return DATASOURCE_JSON;
}

inline uint32_t web_client_api_request_vX_data_google_format(char *name) {
    uint32_t hash = simple_hash(name);
    int i;

    for(i = 0; api_vX_data_google_formats[i].name ; i++) {
        if (unlikely(hash == api_vX_data_google_formats[i].hash && !strcmp(name, api_vX_data_google_formats[i].name))) {
            return api_vX_data_google_formats[i].value;
        }
    }

    return DATASOURCE_JSON;
}

void web_client_api_request_vX_source_to_buffer(struct web_client *w, BUFFER *source) {
    if(web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_CLOUD))
        buffer_sprintf(source, "method=NC");
    else if(web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_BEARER))
        buffer_sprintf(source, "method=api-bearer");
    else
        buffer_sprintf(source, "method=api");

    if(web_client_flag_check(w, WEB_CLIENT_FLAG_AUTH_GOD))
        buffer_strcat(source, ",role=god");
    else
        buffer_sprintf(source, ",role=%s", http_id2user_role(w->user_role));

    buffer_sprintf(source, ",permissions="HTTP_ACCESS_FORMAT, (HTTP_ACCESS_FORMAT_CAST)w->access);

    if(w->auth.client_name[0])
        buffer_sprintf(source, ",user=%s", w->auth.client_name);

    if(!uuid_is_null(w->auth.cloud_account_id)) {
        char uuid_str[UUID_COMPACT_STR_LEN];
        uuid_unparse_lower_compact(w->auth.cloud_account_id, uuid_str);
        buffer_sprintf(source, ",account=%s", uuid_str);
    }

    if(w->client_ip[0])
        buffer_sprintf(source, ",ip=%s", w->client_ip);

    if(w->forwarded_for)
        buffer_sprintf(source, ",forwarded_for=%s", w->forwarded_for);
}

static struct web_api_command api_commands_v1[] = {
    // time-series data APIs
    {
        .api = "data",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_data,
        .allow_subpaths = 0
    },
    {
        .api = "weights",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_weights,
        .allow_subpaths = 0
    },
    {
        // deprecated - do not use anymore - use "weights"
        .api = "metric_correlations",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_metric_correlations,
        .allow_subpaths = 0
    },
    {
        // exporting API
        .api = "allmetrics",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_allmetrics,
        .allow_subpaths = 0
    },
    {
        // badges can be fetched with both dashboard and badge ACL
        .api = "badge.svg",
        .hash = 0,
        .acl = HTTP_ACL_BADGES,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_badge,
        .allow_subpaths = 0
    },

    // alerts APIs
    {
        .api = "alarms",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_alarms,
        .allow_subpaths = 0
    },
    {
        .api = "alarms_values",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_alarms_values,
        .allow_subpaths = 0
    },
    {
        .api = "alarm_log",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_alarm_log,
        .allow_subpaths = 0
    },
    {
        .api = "alarm_variables",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_alarm_variables,
        .allow_subpaths = 0
    },
    {
        .api = "variable",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_variable,
        .allow_subpaths = 0
    },
    {
        .api = "alarm_count",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_alarm_count,
        .allow_subpaths = 0
    },

    // functions APIs - they check permissions per function call
    {
        .api = "function",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_function,
        .allow_subpaths = 0
    },
    {
        .api = "functions",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_functions,
        .allow_subpaths = 0
    },

    // time-series metadata APIs
    {
        .api = "chart",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_chart,
        .allow_subpaths = 0
    },
    {
        .api = "charts",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_charts,
        .allow_subpaths = 0
    },
    {
        .api = "context",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_context,
        .allow_subpaths = 0
    },
    {
        .api = "contexts",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_contexts,
        .allow_subpaths = 0
    },

    // registry APIs
    {
        // registry checks the ACL by itself, so we allow everything
        .api = "registry",
        .hash = 0,
        .acl = HTTP_ACL_NONE, // it manages acl by itself
        .access = HTTP_ACCESS_NONE, // it manages access by itself
        .callback = web_client_api_request_v1_registry,
        .allow_subpaths = 0
    },

    // agent information APIs
    {
        .api = "info",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_info,
        .allow_subpaths = 0
    },
    {
        .api = "aclk",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_aclk,
        .allow_subpaths = 0
    },
    {
        // deprecated - use /api/v2/info
        .api = "dbengine_stats",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_dbengine_stats,
        .allow_subpaths = 0
    },

    // dyncfg APIs
    {
        .api = "config",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v3_config,
        .allow_subpaths = 0
    },

    {
        .api = "ml_info",
        .hash = 0,
        .acl = HTTP_ACL_DASHBOARD,
        .access = HTTP_ACCESS_ANONYMOUS_DATA,
        .callback = web_client_api_request_v1_ml_info,
        .allow_subpaths = 0
    },

    {
        // deprecated
        .api = "manage",
        .hash = 0,
        .acl = HTTP_ACL_MANAGEMENT,
        .access = HTTP_ACCESS_NONE, // it manages access by itself
        .callback = web_client_api_request_v1_manage,
        .allow_subpaths = 1
    },

    {
        // terminator - keep this last on this list
        .api = NULL,
        .hash = 0,
        .acl = HTTP_ACL_NONE,
        .access = HTTP_ACCESS_NONE,
        .callback = NULL,
        .allow_subpaths = 0
    },
};

inline int web_client_api_request_v1(RRDHOST *host, struct web_client *w, char *url_path_endpoint) {
    static int initialized = 0;

    if(unlikely(initialized == 0)) {
        initialized = 1;

        for(int i = 0; api_commands_v1[i].api ; i++)
            api_commands_v1[i].hash = simple_hash(api_commands_v1[i].api);
    }

    return web_client_api_request_vX(host, w, url_path_endpoint, api_commands_v1);
}
