// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_api_v1.h"

static struct {
    const char *name;
    uint32_t hash;
    RRDR_OPTIONS value;
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
        if(fd == -1)
            fatal("Cannot create unique management API key file '%s'. Please fix this.", api_key_filename);

        if(write(fd, guid, GUID_LEN) != GUID_LEN)
            fatal("Cannot write the unique management API key file '%s'. Please fix this.", api_key_filename);

        close(fd);
    }

    return guid;
}

void web_client_api_v1_management_init(void) {
	api_secret = get_mgmt_api_key();
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
    (void)url;
    int all = 0;

    uint32_t end = w->total_params;
    if(end) {
        uint32_t  i = 0;
        do {
            char *value = w->param_values[i].body;

            if(!strncmp(value, "all",3)) all = 1;
            else if(!strncmp(value, "active",6)) all = 0;
        } while(++i < end);
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarms2json(host, w->response.data, all);
    buffer_no_cacheable(w->response.data);
    return 200;
}

inline int web_client_api_request_v1_alarm_log(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;
    uint32_t after = 0;

    uint32_t end = w->total_params;
    if(end) {
        uint32_t  i = 0;
        do {
            char *value = w->param_values[i].body;
            size_t lvalue = w->param_values[i].length;
            char save = value[lvalue];
            value[lvalue] = 0x00;

            char *name = w->param_name[i].body;
            size_t lname = w->param_name[i].length;

            if(!strncmp(name, "after",lname)) after = (uint32_t)strtoul(value, NULL, 0);
            value[lvalue] = save;
        } while (++i < end);
    }

    buffer_flush(w->response.data);
    w->response.data->contenttype = CT_APPLICATION_JSON;
    health_alarm_log2json(host, w->response.data, after);
    return 200;
}

inline int web_client_api_request_single_chart(RRDHOST *host, struct web_client *w, char *url, void callback(RRDSET *st, BUFFER *buf)) {
    (void)url;
    int ret = 400;
    char *chart = NULL;

    buffer_flush(w->response.data);

    uint32_t  i = 0;
    uint32_t end = w->total_params;
    if(end) {
        do {
            char *name = w->param_name[i].body;
            size_t nlength  = w->param_name[i].length;
            char *value = w->param_values[i].body;

            // name and value are now the parameters
            // they are not null and not empty

            if(!strncmp(name, "chart",nlength)) chart = value;
            //else {
            /// buffer_sprintf(w->response.data, "Unknown parameter '%s' in request.", name);
            //  goto cleanup;
            //}
        } while (++i < end);
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
    charts2json(host, w->response.data);
    return 200;
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
    (void)url;
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

    int group = RRDR_GROUPING_AVERAGE;
    uint32_t format = DATASOURCE_JSON;
    uint32_t options = 0x00000000;

    uint32_t end = w->total_params;
    char save[WEB_FIELDS_MAX];
    char *value ;
    size_t lvalue;
    if(end) {
        uint32_t i = 0;
        do {
            char *name = w->param_name[i].body;
            size_t lname = w->param_name[i].length;
            value = w->param_values[i].body;
            lvalue = w->param_values[i].length;
            save[i] = value[lvalue];
            value[lvalue] = 0x00;

            debug(D_WEB_CLIENT, "%llu: API v1 data query param '%s' with value '%s'", w->id, name, value);

            // name and value are now the parameters
            // they are not null and not empty

            if(!strncmp(name, "chart",lname)) chart = value;
            else if(!strncmp(name, "dimension",lname) || !strncmp(name, "dim",lname) || !strncmp(name, "dimensions",lname) || !strncmp(name, "dims",lname)) {
                if(!dimensions) dimensions = buffer_create(100);
                buffer_strcat(dimensions, "|");
                buffer_strcat(dimensions, value);
            }
            else if(!strncmp(name, "after",lname)) after_str = value;
            else if(!strncmp(name, "before",lname)) before_str = value;
            else if(!strncmp(name, "points",lname)) points_str = value;
            else if(!strncmp(name, "gtime",lname)) group_time_str = value;
            else if(!strncmp(name, "group",lname)) {
                group = web_client_api_request_v1_data_group(value, RRDR_GROUPING_AVERAGE);
            }
            else if(!strncmp(name, "format",lname)) {
                format = web_client_api_request_v1_data_format(value);
            }
            else if(!strncmp(name, "options",lname)) {
                options |= web_client_api_request_v1_data_options(value);
            }
            else if(!strncmp(name, "callback",lname)) {
                responseHandler = value;
            }
            else if(!strncmp(name, "filename",lname)) {
                outFileName = value;
            }
            else if(!strncmp(name, "tqx",lname)) {
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
        } while (++i < end);
    }

    // validate the google parameters given
    fix_google_param(google_out);
    fix_google_param(google_sig);
    fix_google_param(google_reqId);
    fix_google_param(google_version);
    fix_google_param(responseHandler);
    fix_google_param(outFileName);

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
    if(end) {
        uint32_t i = 0;
        do {
            value = w->param_values[i].body;
            lvalue = w->param_values[i].length;
            value[lvalue] = save[i];
        } while ( ++i < end );
    }
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

    uint32_t i = 0;
    uint32_t end = w->total_params;
    if (!end) {
        goto nothing;
    }

    do {
        char *name = w->param_name[i].body;
        size_t nlength = w->param_name[i].length;
        char *value = w->param_values[i].body;
        size_t vlength = w->param_values[i].length;

        debug(D_WEB_CLIENT, "%llu: API v1 registry query param '%s' with value '%s'", w->id, name, value);

        //uint32_t hash = simple_hash(name);
        uint32_t hash = simple_nhash(name,nlength);

        //if(hash == hash_action && !strcmp(name, "action")) {
        if(hash == hash_action && !strncmp(name, "action",nlength)) {
            //uint32_t vhash = simple_hash(value);
            uint32_t vhash = simple_nhash(value,vlength);

            if(vhash == hash_access && !strncmp(value, "access",vlength)) action = 'A';
            else if(vhash == hash_hello && !strncmp(value, "hello",vlength)) action = 'H';
            else if(vhash == hash_delete && !strncmp(value, "delete",vlength)) action = 'D';
            else if(vhash == hash_search && !strncmp(value, "search",vlength)) action = 'S';
            else if(vhash == hash_switch && !strncmp(value, "switch",vlength)) action = 'W';
#ifdef NETDATA_INTERNAL_CHECKS
            else error("unknown registry action '%s'", value);
#endif /* NETDATA_INTERNAL_CHECKS */
        }
/*
        else if(hash == hash_redirects && !strcmp(name, "redirects"))
            redirects = atoi(value);
*/
        else if(hash == hash_machine && !strncmp(name, "machine",nlength))
            machine_guid = value;

        else if(hash == hash_url && !strncmp(name, "url",nlength))
            machine_url = value;

        else if(action == 'A') {
            if(hash == hash_name && !strncmp(name, "name",nlength))
                url_name = value;
        }
        else if(action == 'D') {
            if(hash == hash_delete_url && !strncmp(name, "delete_url",nlength))
                delete_url = value;
        }
        else if(action == 'S') {
            if(hash == hash_for && !strncmp(name, "for",nlength))
                search_machine_guid = value;
        }
        else if(action == 'W') {
            if(hash == hash_to && !strncmp(name, "to",nlength))
                to_person_guid = value;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        else error("unused registry URL parameter '%s' with value '%s'", name, value);
#endif /* NETDATA_INTERNAL_CHECKS */
    } while (++i < end );

nothing:
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
    RRDHOST *rc;
    int count = 0;
    rrd_rdlock();
    rrdhost_foreach_read(rc) {
        if(count > 0) buffer_strcat(wb, ",\n");
        buffer_sprintf(wb, "\t\t\"%s\"", rc->hostname);
        count++;
    }
    buffer_strcat(wb, "\n");
    rrd_unlock();
}

inline int web_client_api_request_v1_info(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;
    if (!netdata_ready) return 503;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->contenttype = CT_APPLICATION_JSON;

    buffer_strcat(wb, "{\n");
    buffer_sprintf(wb, "\t\"version\": \"%s\",\n", host->program_version);
    buffer_sprintf(wb, "\t\"uid\": \"%s\",\n", host->machine_guid);

    buffer_strcat(wb, "\t\"mirrored_hosts\": [\n");
    web_client_api_request_v1_info_mirrored_hosts(wb);
    buffer_strcat(wb, "\t],\n");

    buffer_strcat(wb, "\t\"alarms\": {\n");
    web_client_api_request_v1_info_summary_alarm_statuses(host, wb);
    buffer_strcat(wb, "\t},\n");

    buffer_sprintf(wb, "\t\"os_name\": %s,\n", (host->system_info->os_name) ? host->system_info->os_name : "\"\"");
    buffer_sprintf(wb, "\t\"os_id\": \"%s\",\n", (host->system_info->os_id) ? host->system_info->os_id : "");
    buffer_sprintf(wb, "\t\"os_id_like\": \"%s\",\n", (host->system_info->os_id_like) ? host->system_info->os_id_like : "");
    buffer_sprintf(wb, "\t\"os_version\": \"%s\",\n", (host->system_info->os_version) ? host->system_info->os_version : "");
    buffer_sprintf(wb, "\t\"os_version_id\": \"%s\",\n", (host->system_info->os_version_id) ? host->system_info->os_version_id : "");
    buffer_sprintf(wb, "\t\"os_detection\": \"%s\",\n", (host->system_info->os_detection) ? host->system_info->os_detection : "");
    buffer_sprintf(wb, "\t\"kernel_name\": \"%s\",\n", (host->system_info->kernel_name) ? host->system_info->kernel_name : "");
    buffer_sprintf(wb, "\t\"kernel_version\": \"%s\",\n", (host->system_info->kernel_version) ? host->system_info->kernel_version : "");
    buffer_sprintf(wb, "\t\"architecture\": \"%s\",\n", (host->system_info->architecture) ? host->system_info->architecture : "");
    buffer_sprintf(wb, "\t\"virtualization\": \"%s\",\n", (host->system_info->virtualization) ? host->system_info->virtualization : "");
    buffer_sprintf(wb, "\t\"virt_detection\": \"%s\",\n", (host->system_info->virt_detection) ? host->system_info->virt_detection : "");
    buffer_sprintf(wb, "\t\"container\": \"%s\",\n", (host->system_info->container) ? host->system_info->container : "");
    buffer_sprintf(wb, "\t\"container_detection\": \"%s\",\n", (host->system_info->container_detection) ? host->system_info->container_detection : "");

    buffer_strcat(wb, "\t\"collectors\": [");
    chartcollectors2json(host, wb);
    buffer_strcat(wb, "\n\t]\n");

    buffer_strcat(wb, "}");
    buffer_no_cacheable(wb);
    return 200;
}

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

        // registry checks the ACL by itself, so we allow everything
        { "registry",        0, WEB_CLIENT_ACL_NOCHECK,   web_client_api_request_v1_registry        },

        // badges can be fetched with both dashboard and badge permissions
        { "badge.svg",       0, WEB_CLIENT_ACL_DASHBOARD|WEB_CLIENT_ACL_BADGE, web_client_api_request_v1_badge },

        { "alarms",          0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarms          },
        { "alarm_log",       0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_log       },
        { "alarm_variables", 0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_alarm_variables },
        { "allmetrics",      0, WEB_CLIENT_ACL_DASHBOARD, web_client_api_request_v1_allmetrics      },
        { "manage/health",   0, WEB_CLIENT_ACL_MGMT,      web_client_api_request_v1_mgmt_health     },
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

    char *cmd = w->command.body;
    size_t length = w->command.length;
    uint32_t hash = simple_nhash(cmd,length);

    for(i = 0; api_commands[i].command ;i++) {
        if(unlikely(hash == api_commands[i].hash && !strncmp(cmd, api_commands[i].command,length))) {
            if(unlikely(api_commands[i].acl != WEB_CLIENT_ACL_NOCHECK) &&  !(w->acl & api_commands[i].acl))
                return web_client_permission_denied(w);

            return api_commands[i].callback(host, w, url);
        }
    }

    char copyme[256];
    length = w->path.length;
    memcpy(copyme,w->path.body,length);
    copyme[length] = 0x00;

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, "Unsupported v1 API command: ");
    buffer_strcat_htmlescape(w->response.data, copyme);
    return 404;
}
