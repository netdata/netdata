// SPDX-License-Identifier: GPL-3.0-or-later

/** @file functions.c
 *  
 *  @brief This is the file containing the implementation of the 
 *  logs management functions API.
 */

#include "functions.h"
#include "helper.h"
#include "query.h"

#define FUNCTION_LOGSMANAGEMENT_HELP_LONG \
    LOGS_MANAG_FUNC_NAME"\n\n" \
    "Function 'logsmanagement' enables querying of the logs management engine and retrieval of logs stored on this node. \n\n" \
    "Arguments:\n\n" \
    "   help\n" \
    "      prints this help message and returns\n\n" \
    "   sources\n" \
    "      returns a list of available log sources to be queried\n\n" \
    "   "LOGS_MANAG_FUNC_PARAM_AFTER":NUMBER\n" \
    "      start timestamp in ms to search from (inclusive)\n\n" \
    "   "LOGS_MANAG_FUNC_PARAM_BEFORE":NUMBER\n" \
    "      end timestamp in ms to search until (inclusive), if < '"LOGS_MANAG_FUNC_PARAM_AFTER"', query will search timestamps in reverse order\n\n" \
    "   "LOGS_QRY_KW_QUOTA":NUMBER\n" \
    "      max size of logs to return (in MiB), default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_QUOTA_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_CHARTNAME":STRING\n" \
    "      Chart name (or names if provided multiple times) to be queried for logs, max No. of sources: " \
            LOGS_MANAG_STR(LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES) "\n\n" \
    "   "LOGS_QRY_KW_FILENAME":STRING\n" \
    "      If no 'chartname' is provided, file name (or names if provided multiple times) to be queried for logs, max No. of sources: " \
            LOGS_MANAG_STR(LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES) "\n\n" \
    "   "LOGS_QRY_KW_KEYWORD":STRING\n" \
    "      Keyword to be searched in the queried logs\n\n" \
    "   "LOGS_QRY_KW_IGNORE_CASE":BOOL\n" \
    "      Case-sensitive keyword search if set to 0, default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_IGNORE_CASE_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_SANITIZE_KW":BOOL\n" \
    "      If non-zero, the keyword will be sanitized before used by the regex engine (it will *not* be interpreted as a regex), default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_SANITIZE_KEYWORD_DEFAULT) "\n\n" \
    "All arguments except for either '"LOGS_QRY_KW_CHARTNAME"' or '"LOGS_QRY_KW_FILENAME"' are optional.\n" \
    "If 'help' or 'sources' is passed on, all other arguments will be ignored."



#define add_table_field(wb, key, name, visible, type, units, max, sort, sortable, sticky, unique_key, pointer_to, summary, range) do { \
    if(fields_added) buffer_strcat(wb, ",");                                                                    \
    buffer_sprintf(wb, "\n      \"%s\": {", key);                                                               \
    buffer_sprintf(wb, "\n         \"index\":%d,", fields_added);                                               \
    buffer_sprintf(wb, "\n         \"unique_key\":%s,", (unique_key)?"true":"false");                           \
    buffer_sprintf(wb, "\n         \"name\":\"%s\",", name);                                                    \
    buffer_sprintf(wb, "\n         \"visible\":%s,", (visible)?"true":"false");                                 \
    buffer_sprintf(wb, "\n         \"type\":\"%s\",", type);                                                    \
    if(units)                                                                                                   \
       buffer_sprintf(wb, "\n         \"units\":\"%s\",", (char*)(units));                                      \
    if(!isnan((NETDATA_DOUBLE)(max)))                                                                           \
       buffer_sprintf(wb, "\n         \"max\":%f,", (NETDATA_DOUBLE)(max));                                     \
    if(pointer_to)                                                                                              \
        buffer_sprintf(wb, "\n         \"pointer_to\":\"%s\",", (char *)(pointer_to));                          \
    buffer_sprintf(wb, "\n         \"sort\":\"%s\",", sort);                                                    \
    buffer_sprintf(wb, "\n         \"sortable\":%s,", (sortable)?"true":"false");                               \
    buffer_sprintf(wb, "\n         \"sticky\":%s,", (sticky)?"true":"false");                                   \
    buffer_sprintf(wb, "\n         \"summary\":\"%s\",", summary);                                              \
    buffer_sprintf(wb, "\n         \"filter\":\"%s\"", (range)?"range":"multiselect");                          \
    buffer_sprintf(wb, "\n      }");                                                                            \
    fields_added++;                                                                                             \
} while(0)

int logsmanagement_function_execute_cb( BUFFER *dest_wb, int timeout, 
                                        const char *function, void *collector_data, 
                                        void (*callback)(BUFFER *wb, int code, void *callback_data), 
                                        void *callback_data) {

    UNUSED(collector_data);
    UNUSED(callback);
    UNUSED(callback_data);

    struct rusage start, end;
    getrusage(RUSAGE_THREAD, &start);

    logs_query_params_t query_params = {0};
    unsigned long req_quota = 0;

    unsigned int fn_off = 0, cn_off = 0;

    while(function){
        char *value = strsep_skip_consecutive_separators((char **) &function, " ");
        if (!value || !*value) continue;
        else if(!strcmp(value, "help")){
            buffer_sprintf(dest_wb, FUNCTION_LOGSMANAGEMENT_HELP_LONG);
            dest_wb->content_type = CT_TEXT_PLAIN;
            return HTTP_RESP_OK;
        }
        else if (!strcmp(value, "sources")){
            buffer_sprintf( dest_wb, 
                            "{\n"
                            "   \"api version\": %s,\n" 
                            "   \"log sources\": {\n",
                            LOGS_QRY_VERSION);
            const logs_qry_res_err_t *const res_err = fetch_log_sources(dest_wb);
            buffer_sprintf( dest_wb, 
                            "\n"
                            "   },\n"
                            "   \"error code\": %d,\n"
                            "   \"error\": \"%s\"\n}",
                            (int) res_err->err_code,
                            res_err->err_str);

            return res_err->http_code;
        }

        char *key = strsep_skip_consecutive_separators(&value, ":");
        if(!key || !*key) continue;
        if(!value || !*value) continue;

        // Kludge to respect quotes with spaces in between
        // Not proper fix
        if(*value == '_'){
            value++;
            value[strlen(value)] = ' '; 
            value = strsep_skip_consecutive_separators(&value, "_");
            function = strrchr(value, 0);
            function++;
        }

        if(!strcmp(key, LOGS_MANAG_FUNC_PARAM_AFTER)){
            query_params.req_from_ts = strtoll(value, NULL, 10);
        }
        else if(!strcmp(key, LOGS_MANAG_FUNC_PARAM_BEFORE)){
            query_params.req_to_ts = strtoll(value, NULL, 10);
        }
        else if(!strcmp(key, LOGS_QRY_KW_QUOTA)){
            req_quota = strtoll(value, NULL, 10);
        }
        else if(!strcmp(key, LOGS_QRY_KW_FILENAME) && fn_off < LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES){
            query_params.filename[fn_off++] = value;
        }
        else if(!strcmp(key, LOGS_QRY_KW_CHARTNAME) && cn_off < LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES){
            query_params.chartname[cn_off++] = value;
        }
        else if(!strcmp(key, LOGS_QRY_KW_KEYWORD)){
            query_params.keyword = value;
        }
        else if(!strcmp(key, LOGS_QRY_KW_IGNORE_CASE)){
            query_params.ignore_case = strtol(value, NULL, 10) ? 1 : 0;
        }
        else if(!strcmp(key, LOGS_QRY_KW_SANITIZE_KW)){
            query_params.sanitize_keyword = strtol(value, NULL, 10) ? 1 : 0;
        }
        else {
            collector_error("functions: logsmanagement invalid parameter");
            return HTTP_RESP_BAD_REQUEST;
        }
    }

    query_params.order_by_asc = query_params.req_from_ts <= query_params.req_to_ts ? 1 : 0;

    fn_off = cn_off = 0;

    if(!req_quota) query_params.quota = LOGS_MANAG_QUERY_QUOTA_DEFAULT;
    else if(req_quota > LOGS_MANAG_QUERY_QUOTA_MAX) query_params.quota = LOGS_MANAG_QUERY_QUOTA_MAX;
    else query_params.quota = req_quota;

    query_params.stop_monotonic_ut = now_monotonic_usec() + (timeout - 1) * USEC_PER_SEC;

    query_params.results_buff = buffer_create(query_params.quota, NULL);

    const logs_qry_res_err_t *const res_err = execute_logs_manag_query(&query_params);

    const int update_every = 1;

    buffer_sprintf( dest_wb,
                    "{\n"
                    "   \"status\": %d,\n"
                    "   \"type\": \"table\",\n"
                    "   \"update_every\": %d,\n"
                    "   \"data\":[\n",
                    res_err->http_code,
                    update_every
    );

    size_t res_off = 0;
    logs_query_res_hdr_t *p_res_hdr;
    while(query_params.results_buff->len - res_off > 0){
        p_res_hdr = (logs_query_res_hdr_t *) &query_params.results_buff->buffer[res_off];
        
        buffer_sprintf( dest_wb, 
                        "      [\n"
                        "         %" PRIu64 ",\n"
                        "         [\n   "
                        "         \"", 
                        p_res_hdr->timestamp
        );

        buffer_need_bytes(dest_wb, p_res_hdr->text_size);

        /* Unfortunately '\n', '\\' (except for "\\n") and '"' need to be 
         * escaped, so we need to go through the result characters one by one.
         * The first part below is needed only for descending timestamp order 
         * queries. */

        int order_desc_rem = p_res_hdr->text_size - 1;
        char *line_s = &query_params.results_buff->buffer[res_off] + 
                        sizeof(*p_res_hdr) + p_res_hdr->text_size - 2;
        if(!query_params.order_by_asc){
            while(order_desc_rem > 0 && *line_s != '\n') {
                line_s--;
                order_desc_rem--;
            }
        }

        char *p = query_params.order_by_asc ? 
            &query_params.results_buff->buffer[res_off] + sizeof(*p_res_hdr) : line_s + 1;

        size_t remaining = p_res_hdr->text_size;
        while (--remaining){ /* --remaining instead of remaing-- to exclude the last '\n' */
            if(unlikely(*p == '\n')){
                buffer_strcat(dest_wb, "\",\n\t\t\t\t\"");
                if(!query_params.order_by_asc){
                    do{
                        order_desc_rem--;
                        line_s--;
                    } while(order_desc_rem > 0 && *line_s != '\n');
                    p = line_s;
                }
            } 
            else if(unlikely(*p == '\\' && *(p+1) != 'n')) {
                buffer_need_bytes(dest_wb, 1);
                dest_wb->buffer[dest_wb->len++] = '\\';
                dest_wb->buffer[dest_wb->len++] = '\\';
            }
            else if(unlikely(*p == '"')) {
                buffer_need_bytes(dest_wb, 1);
                dest_wb->buffer[dest_wb->len++] = '\\';
                dest_wb->buffer[dest_wb->len++] = '"';
            }
            else{
                dest_wb->buffer[dest_wb->len++] = *p;
            }
            p++;
        }
        
        buffer_sprintf( dest_wb,    "\"\n"
                                    "         ],\n"
                                    "         %zu,\n"
                                    "         %d\n"
                                    "      ]" , 
                                    p_res_hdr->text_size,
                                    p_res_hdr->matches);

        res_off += sizeof(*p_res_hdr) + p_res_hdr->text_size;

        // Add comma and new line if there are more data to be printed
        if(query_params.results_buff->len - res_off > 0) buffer_strcat(dest_wb, ",\n");
    }

    getrusage(RUSAGE_THREAD, &end);

    buffer_sprintf( dest_wb,
                    "\n   ],\n"
                    "   \"logs_management_meta\": {\n"
                    "      \"api_version\": %s,\n"
                    "      \"requested_from\": %" PRIu64 ",\n"
                    "      \"requested_to\": %" PRIu64 ",\n"
                    "      \"requested_quota\": %llu,\n"
                    "      \"requested_keyword\": \"%s\",\n"
                    "      \"actual_from\": %" PRIu64",\n"
                    "      \"actual_to\": %" PRIu64",\n"
                    "      \"actual_quota\": %llu,\n"
                    "      \"requested_filename\": [\n",
                    LOGS_QRY_VERSION,
                    query_params.req_from_ts,
                    query_params.req_to_ts,
                    req_quota / (1 KiB),
                    query_params.keyword ? query_params.keyword : "",
                    query_params.act_from_ts,
                    query_params.act_to_ts,
                    query_params.quota / (1 KiB)
    );

    while(query_params.filename[fn_off]) 
        buffer_sprintf(dest_wb, "         \"%s\",\n", query_params.filename[fn_off++]);
    if(query_params.filename[0])  dest_wb->len -= 2;
    buffer_strcat(  dest_wb, 
                    "\n      ],\n"
                    "      \"requested_chartname\": [\n"
    );

    while(query_params.chartname[cn_off]) 
        buffer_sprintf(dest_wb, "         \"%s\",\n", query_params.chartname[cn_off++]);
    if(query_params.chartname[0])  dest_wb->len -= 2;

    buffer_sprintf( dest_wb, 
                    "\n      ],\n"
                    "      \"num_lines\": %lu, \n"
                    "      \"user_time\": %llu,\n"
                    "      \"system_time\": %llu,\n"
                    "      \"error_code\": %d,\n"
                    "      \"error\": \"%s\"\n"
                    "   },\n",
                    query_params.num_lines,
                    end.ru_utime.tv_sec * USEC_PER_SEC + end.ru_utime.tv_usec - 
                    start.ru_utime.tv_sec * USEC_PER_SEC - start.ru_utime.tv_usec,
                    end.ru_stime.tv_sec * USEC_PER_SEC + end.ru_stime.tv_usec - 
                    start.ru_stime.tv_sec * USEC_PER_SEC - start.ru_stime.tv_usec,
                    (int) res_err->err_code,
                    res_err->err_str
    );


    buffer_strcat(dest_wb, "   \"columns\": {");
    int fields_added = 0;
    add_table_field(dest_wb, "Timestamp", "Timestamp in Milliseconds", true, "time", "milliseconds", NAN, "ascending", true, true, false, NULL, "average", false);
    add_table_field(dest_wb, "Logs", "Logs collected in last interval", true, "string", NULL, NAN, "ascending", false, false, false, NULL, "N/A", false);
    add_table_field(dest_wb, "LogsTxtSz", "Logs text length", false, "integer", NULL, NAN, "ascending", true, false, false, NULL, "sum", false);
    add_table_field(dest_wb, "MatchNo", "Keyword matches", true, "integer", NULL, NAN, "ascending", true, false, false, NULL, "sum", false);

    buffer_sprintf( dest_wb, 
                    "\n   },"
                    "\n   \"expires\": %lld"
                    "\n}",
                    (long long) now_realtime_sec() + update_every);

    buffer_free(query_params.results_buff);

    buffer_no_cacheable(dest_wb);  

    return res_err->http_code;
}


extern netdata_mutex_t stdout_mut;

DICTIONARY *function_query_status_dict = NULL;

static DICTIONARY *used_hashes_registry = NULL;

typedef struct function_query_status {
    bool *cancelled; // a pointer to the cancelling boolean
    usec_t stop_monotonic_ut;

    usec_t started_monotonic_ut;

    // request
    // SD_JOURNAL_FILE_SOURCE_TYPE source_type;
    STRING *source;
    usec_t after_ut;
    usec_t before_ut;

    struct {
        usec_t start_ut;
        usec_t stop_ut;
    } anchor;

    FACETS_ANCHOR_DIRECTION direction;
    size_t entries;
    usec_t if_modified_since;
    bool delta;
    bool tail;
    bool data_only;
    bool slice;
    size_t filters;
    usec_t last_modified;
    const char *query;
    const char *histogram;

    // per file progress info
    size_t cached_count;

    // progress statistics
    usec_t matches_setup_ut;
    size_t rows_useful;
    size_t rows_read;
    size_t bytes_read;
    size_t files_matched;
    size_t file_working;
} FUNCTION_QUERY_STATUS;

static void function_systemd_journal_progress(BUFFER *wb, const char *transaction, const char *progress_id) {
    if(!progress_id || !(*progress_id)) {
        netdata_mutex_lock(&stdout_mut);
        pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_BAD_REQUEST, "missing progress id");
        netdata_mutex_unlock(&stdout_mut);
        return;
    }

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(function_query_status_dict, progress_id);

    if(!item) {
        netdata_mutex_lock(&stdout_mut);
        pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_NOT_FOUND, "progress id is not found here");
        netdata_mutex_unlock(&stdout_mut);
        return;
    }

    FUNCTION_QUERY_STATUS *fqs = dictionary_acquired_item_value(item);

    usec_t now_monotonic_ut = now_monotonic_usec();
    if(now_monotonic_ut + 10 * USEC_PER_SEC > fqs->stop_monotonic_ut)
        fqs->stop_monotonic_ut = now_monotonic_ut + 10 * USEC_PER_SEC;

    usec_t duration_ut = now_monotonic_ut - fqs->started_monotonic_ut;

    size_t files_matched = fqs->files_matched;
    size_t file_working = fqs->file_working;
    if(file_working > files_matched)
        files_matched = file_working;

    size_t rows_read = __atomic_load_n(&fqs->rows_read, __ATOMIC_RELAXED);
    size_t bytes_read = __atomic_load_n(&fqs->bytes_read, __ATOMIC_RELAXED);

    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_uint64(wb, "running_duration_usec", duration_ut);
    buffer_json_member_add_double(wb, "progress", (double)file_working * 100.0 / (double)files_matched);
    char msg[1024 + 1];
    snprintfz(msg, 1024,
              "Read %zu rows (%0.0f rows/s), "
              "data %0.1f MB (%0.1f MB/s), "
              "file %zu of %zu",
              rows_read, (double)rows_read / (double)duration_ut * (double)USEC_PER_SEC,
              (double)bytes_read / 1024.0 / 1024.0, ((double)bytes_read / (double)duration_ut * (double)USEC_PER_SEC) / 1024.0 / 1024.0,
              file_working, files_matched
              );
    buffer_json_member_add_string(wb, "message", msg);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mut);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_realtime_sec() + 1, wb);
    netdata_mutex_unlock(&stdout_mut);

    dictionary_acquired_item_release(function_query_status_dict, item);
}


#define FACET_MAX_VALUE_LENGTH      8192

#define LOGS_MANAG_MAX_PARAMS       100

#define LOGS_MANAG_FUNC_PARAM_HELP              "help"
#define LOGS_MANAG_FUNC_PARAM_ANCHOR            "anchor"
#define LOGS_MANAG_FUNC_PARAM_LAST              "last"
#define LOGS_MANAG_FUNC_PARAM_QUERY             "query"
#define LOGS_MANAG_FUNC_PARAM_FACETS            "facets"
#define LOGS_MANAG_FUNC_PARAM_HISTOGRAM         "histogram"
#define LOGS_MANAG_FUNC_PARAM_DIRECTION         "direction"
#define LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE "if_modified_since"
#define LOGS_MANAG_FUNC_PARAM_DATA_ONLY         "data_only"
#define LOGS_MANAG_FUNC_PARAM_SOURCE            "source"
#define LOGS_MANAG_FUNC_PARAM_INFO              "info"
#define LOGS_MANAG_FUNC_PARAM_ID                "id"
#define LOGS_MANAG_FUNC_PARAM_PROGRESS          "progress"
#define LOGS_MANAG_FUNC_PARAM_SLICE             "slice"
#define LOGS_MANAG_FUNC_PARAM_DELTA             "delta"
#define LOGS_MANAG_FUNC_PARAM_TAIL              "tail"

// #define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
// #define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
// #define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         30
// #define SYSTEMD_JOURNAL_MAX_PARAMS              100
#define LOGS_MANAGEMENT_DEFAULT_QUERY_DURATION_IN_SEC  (3 * 3600)
#define LOGS_MANAGEMENT_DEFAULT_ITEMS_PER_QUERY 200

#define LOGS_MANAG_KEYS_INCLUDED_IN_FACETS      \
    "log_source"                                \
    "|log_type"                                 \
    "|filename"                                 \
    "|basename"                                 \
    "|chartname"                                \
    "|message"                                  \
    ""

static void logsmanagement_function_facets(const char *transaction, char *function, int timeout, bool *cancelled){

    struct rusage start, end;
    getrusage(RUSAGE_THREAD, &start);

    int ret = HTTP_RESP_INTERNAL_SERVER_ERROR;

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    usec_t now_monotonic_ut = now_monotonic_usec();
    FUNCTION_QUERY_STATUS tmp_fqs = {
            .cancelled = cancelled,
            .started_monotonic_ut = now_monotonic_ut,
            .stop_monotonic_ut = now_monotonic_ut + (timeout * USEC_PER_SEC),
    };
    FUNCTION_QUERY_STATUS *fqs = NULL;
    const DICTIONARY_ITEM *fqs_item = NULL;

    FACETS *facets = facets_create(50, FACETS_OPTION_ALL_KEYS_FTS,
                                   NULL,
                                   LOGS_MANAG_KEYS_INCLUDED_IN_FACETS,
                                   NULL);
    
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_INFO);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_SOURCE);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_AFTER);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_BEFORE);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_ANCHOR);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_DIRECTION);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_LAST);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_QUERY);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_FACETS);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_HISTOGRAM);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE);
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_DATA_ONLY);
    // facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_ID);
    // facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_PROGRESS);
    // facets_accepted_param(facets, JOURNAL_PARAMETER_DELTA);
    // facets_accepted_param(facets, JOURNAL_PARAMETER_TAIL);

// #ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
//     facets_accepted_param(facets, JOURNAL_PARAMETER_SLICE);
// #endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    // register the fields in the order you want them on the dashboard

    facets_register_key_name(facets, "log_source",  FACET_KEY_OPTION_FACET | 
                                                    FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, "log_type",    FACET_KEY_OPTION_FACET | 
                                                    FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, "filename",    FACET_KEY_OPTION_FACET | 
                                                    FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, "basename",    FACET_KEY_OPTION_FACET | 
                                                    FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, "chartname",   FACET_KEY_OPTION_VISIBLE | 
                                                    FACET_KEY_OPTION_FACET | 
                                                    FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, "message",     FACET_KEY_OPTION_NEVER_FACET | 
                                                    FACET_KEY_OPTION_MAIN_TEXT | 
                                                    FACET_KEY_OPTION_VISIBLE |
                                                    FACET_KEY_OPTION_FTS);

    bool    info        = false, 
            data_only   = false, 
            progress    = false, 
            /* slice       = true, */
            delta       = false, 
            tail        = false;
    time_t after_s = 0, before_s = 0;
    usec_t anchor = 0;
    usec_t if_modified_since = 0;
    size_t last = 0;
    FACETS_ANCHOR_DIRECTION direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    const char *query = NULL;
    const char *chart = NULL;
    const char *source = NULL;
    const char *progress_id = NULL;
    // SD_JOURNAL_FILE_SOURCE_TYPE source_type = SDJF_ALL;
    // size_t filters = 0;  

    buffer_json_member_add_object(wb, "_request");

    logs_query_params_t query_params = {0};
    unsigned long req_quota = 0;

    unsigned int fn_off = 0, cn_off = 0;

    char *words[LOGS_MANAG_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, LOGS_MANAG_MAX_PARAMS);
    for(int i = 1; i < LOGS_MANAG_MAX_PARAMS ; i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        
        if(!strcmp(keyword, LOGS_MANAG_FUNC_PARAM_HELP)){
            BUFFER *wb = buffer_create(0, NULL);
            buffer_sprintf(wb, FUNCTION_LOGSMANAGEMENT_HELP_LONG);
            netdata_mutex_lock(&stdout_mut);
            pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
            netdata_mutex_unlock(&stdout_mut);
            buffer_free(wb);
            ret = HTTP_RESP_OK;
            goto cleanup;
        }
        else if(!strcmp(keyword, LOGS_MANAG_FUNC_PARAM_INFO)){
            info = true;
        }
        else if(!strcmp(keyword, LOGS_MANAG_FUNC_PARAM_PROGRESS)){
            progress = true;
        }
        // else if(strncmp(keyword, JOURNAL_PARAMETER_DELTA ":", sizeof(JOURNAL_PARAMETER_DELTA ":") - 1) == 0) {
        //     char *v = &keyword[sizeof(JOURNAL_PARAMETER_DELTA ":") - 1];

        //     if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
        //         delta = false;
        //     else
        //         delta = true;
        // }
        // else if(strncmp(keyword, JOURNAL_PARAMETER_TAIL ":", sizeof(JOURNAL_PARAMETER_TAIL ":") - 1) == 0) {
        //     char *v = &keyword[sizeof(JOURNAL_PARAMETER_TAIL ":") - 1];

        //     if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
        //         tail = false;
        //     else
        //         tail = true;
        // }
        else if(!strncmp(   keyword, 
                            LOGS_MANAG_FUNC_PARAM_DATA_ONLY ":", 
                            sizeof(LOGS_MANAG_FUNC_PARAM_DATA_ONLY ":") - 1)) {

            char *v = &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_DATA_ONLY ":") - 1];

            if(!strcmp(v, "false") || !strcmp(v, "no") || !strcmp(v, "0"))
                data_only = false;
            else
                data_only = true;
        }
        // else if(strncmp(keyword, JOURNAL_PARAMETER_SLICE ":", sizeof(JOURNAL_PARAMETER_SLICE ":") - 1) == 0) {
        //     char *v = &keyword[sizeof(JOURNAL_PARAMETER_SLICE ":") - 1];

        //     if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
        //         slice = false;
        //     else
        //         slice = true;
        // }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_ID ":", sizeof(LOGS_MANAG_FUNC_PARAM_ID ":") - 1) == 0) {
            char *id = &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_ID ":") - 1];

            if(*id)
                progress_id = id;
        }

        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_SOURCE ":", sizeof(LOGS_MANAG_FUNC_PARAM_SOURCE ":") - 1) == 0) {
            source = !strcmp("all", &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_SOURCE ":") - 1]) ? 
                NULL : &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_SOURCE ":") - 1];
        }



        // char *key = strsep_skip_consecutive_separators(&value, ":");
        // if(!key || !*key) continue;
        // if(!value || !*value) continue;

        // Kludge to respect quotes with spaces in between
        // Not proper fix
        // if(*value == '_'){
        //     value++;
        //     value[strlen(value)] = ' '; 
        //     value = strsep_skip_consecutive_separators(&value, "_");
        //     function = strrchr(value, 0);
        //     function++;
        // }

        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_AFTER ":", sizeof(LOGS_MANAG_FUNC_PARAM_AFTER ":") - 1) == 0) {
            after_s = str2l(&keyword[sizeof(LOGS_MANAG_FUNC_PARAM_AFTER ":") - 1]);
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_BEFORE ":", sizeof(LOGS_MANAG_FUNC_PARAM_BEFORE ":") - 1) == 0) {
            before_s = str2l(&keyword[sizeof(LOGS_MANAG_FUNC_PARAM_BEFORE ":") - 1]);
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE ":", sizeof(LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE ":") - 1) == 0) {
            if_modified_since = str2ull(&keyword[sizeof(LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE ":") - 1], NULL);
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_ANCHOR ":", sizeof(LOGS_MANAG_FUNC_PARAM_ANCHOR ":") - 1) == 0) {
            anchor = str2ull(&keyword[sizeof(LOGS_MANAG_FUNC_PARAM_ANCHOR ":") - 1], NULL);
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_DIRECTION ":", sizeof(LOGS_MANAG_FUNC_PARAM_DIRECTION ":") - 1) == 0) {
            direction = !strcasecmp(&keyword[sizeof(LOGS_MANAG_FUNC_PARAM_DIRECTION ":") - 1], "forward") ? 
                FACETS_ANCHOR_DIRECTION_FORWARD : FACETS_ANCHOR_DIRECTION_BACKWARD;
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_LAST ":", sizeof(LOGS_MANAG_FUNC_PARAM_LAST ":") - 1) == 0) {
            last = str2ul(&keyword[sizeof(LOGS_MANAG_FUNC_PARAM_LAST ":") - 1]);
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_QUERY ":", sizeof(LOGS_MANAG_FUNC_PARAM_QUERY ":") - 1) == 0) {
            query= &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_QUERY ":") - 1];
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_HISTOGRAM ":", sizeof(LOGS_MANAG_FUNC_PARAM_HISTOGRAM ":") - 1) == 0) {
            chart = &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_HISTOGRAM ":") - 1];
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_FACETS ":", sizeof(LOGS_MANAG_FUNC_PARAM_FACETS ":") - 1) == 0) {
            char *value = &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_FACETS ":") - 1];
            if(*value) {
                buffer_json_member_add_array(wb, LOGS_MANAG_FUNC_PARAM_FACETS);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // LOGS_MANAG_FUNC_PARAM_FACETS
            }
        }
        else {
            char *value = strchr(keyword, ':');
            if(value) {
                *value++ = '\0';

                buffer_json_member_add_array(wb, keyword);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id_filter(facets, keyword, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);
                    // filters++;

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
        


        // else if(!strcmp(key, LOGS_QRY_KW_QUOTA)){
        //     req_quota = strtoll(value, NULL, 10);
        // }
        // else if(!strcmp(key, LOGS_QRY_KW_FILENAME) && fn_off < LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES){
        //     query_params.filename[fn_off++] = value;
        // }
        // else if(!strcmp(key, LOGS_QRY_KW_CHARTNAME) && cn_off < LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES){
        //     query_params.chartname[cn_off++] = value;
        // }
        // else if(!strcmp(key, LOGS_QRY_KW_IGNORE_CASE)){
        //     query_params.ignore_case = strtol(value, NULL, 10) ? 1 : 0;
        // }
        // else if(!strcmp(key, LOGS_QRY_KW_SANITIZE_KW)){
        //     query_params.sanitize_keyword = strtol(value, NULL, 10) ? 1 : 0;
        // }
    }

    // ------------------------------------------------------------------------
    // put this request into the progress db

    if(progress_id && *progress_id) {
        fqs_item = dictionary_set_and_acquire_item(function_query_status_dict, progress_id, &tmp_fqs, sizeof(tmp_fqs));
        fqs = dictionary_acquired_item_value(fqs_item);
    }
    else {
        // no progress id given, proceed without registering our progress in the dictionary
        fqs = &tmp_fqs;
        fqs_item = NULL;
    }

    // ------------------------------------------------------------------------
    // validate parameters

    time_t now_s = now_realtime_sec();
    time_t expires = now_s + 1;

    if(!after_s && !before_s) {
        before_s = now_s;
        after_s = before_s - LOGS_MANAGEMENT_DEFAULT_QUERY_DURATION_IN_SEC;
    }
    else
        rrdr_relative_window_to_absolute(&after_s, &before_s, now_s);

    if(after_s > before_s) {
        time_t tmp = after_s;
        after_s = before_s;
        before_s = tmp;
    }

    if(after_s == before_s)
        after_s = before_s - LOGS_MANAGEMENT_DEFAULT_QUERY_DURATION_IN_SEC;

    if(!last)
        last = LOGS_MANAGEMENT_DEFAULT_ITEMS_PER_QUERY;


    // ------------------------------------------------------------------------
    // set query time-frame, anchors and direction

    fqs->after_ut = after_s * USEC_PER_SEC;
    fqs->before_ut = (before_s * USEC_PER_SEC) + USEC_PER_SEC - 1;
    fqs->if_modified_since = if_modified_since;
    fqs->data_only = data_only;
    fqs->delta = (fqs->data_only) ? delta : false;
    fqs->tail = (fqs->data_only && fqs->if_modified_since) ? tail : false;
    fqs->source = string_strdupz(source);
    // fqs->source_type = source_type;
    fqs->entries = last;
    fqs->last_modified = 0;
    // fqs->filters = filters;
    fqs->query = (query && *query) ? query : NULL;
    fqs->histogram = (chart && *chart) ? chart : NULL;
    fqs->direction = direction;
    fqs->anchor.start_ut = anchor;
    fqs->anchor.stop_ut = 0;

    if(fqs->anchor.start_ut && fqs->tail) {
        // a tail request
        // we need the top X entries from BEFORE
        // but, we need to calculate the facets and the
        // histogram up to the anchor
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = anchor;
    }

    if(anchor && anchor < fqs->after_ut) {
        // log_fqs(fqs, "received anchor is too small for query timeframe, ignoring anchor");
        anchor = 0;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = 0;
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }
    else if(anchor > fqs->before_ut) {
        // log_fqs(fqs, "received anchor is too big for query timeframe, ignoring anchor");
        anchor = 0;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = 0;
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }

    facets_set_anchor(facets, fqs->anchor.start_ut, fqs->anchor.stop_ut, fqs->direction);

    facets_set_additional_options(facets,
                                  ((fqs->data_only) ? FACETS_OPTION_DATA_ONLY : 0) |
                                  ((fqs->delta) ? FACETS_OPTION_SHOW_DELTAS : 0));

    // ------------------------------------------------------------------------
    // set the rest of the query parameters

    facets_set_items(facets, fqs->entries);
    facets_set_query(facets, fqs->query);

// #ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
//     fqs->slice = slice;
//     if(slice)
//         facets_enable_slice_mode(facets);
// #else
//     fqs->slice = false;
// #endif

    if(fqs->histogram)
        facets_set_timeframe_and_histogram_by_id(facets, fqs->histogram, fqs->after_ut, fqs->before_ut);
    else
        facets_set_timeframe_and_histogram_by_name(facets, chart ? chart : "chartname", fqs->after_ut, fqs->before_ut);


    // ------------------------------------------------------------------------
    // complete the request object

    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_INFO, false);
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_SLICE, fqs->slice);
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_DATA_ONLY, fqs->data_only);
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_PROGRESS, false);
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_DELTA, fqs->delta);
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_TAIL, fqs->tail);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_ID, progress_id);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_SOURCE, string2str(fqs->source));
    // buffer_json_member_add_uint64(wb, "source_type", fqs->source_type);
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_AFTER, fqs->after_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_BEFORE, fqs->before_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, "if_modified_since", fqs->if_modified_since);
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_ANCHOR, anchor);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_DIRECTION, fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_LAST, fqs->entries);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_QUERY, fqs->query);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_HISTOGRAM, fqs->histogram);
    buffer_json_object_close(wb); // request

    // buffer_json_journal_versions(wb);

    // buffer_json_member_add_string(wb, "source", source ? source : "default");
    // buffer_json_member_add_time_t(wb, "after", after_s);
    // buffer_json_member_add_time_t(wb, "before", before_s);
    // buffer_json_member_add_uint64(wb, "if_modified_since", if_modified_since);
    // buffer_json_member_add_uint64(wb, "anchor", anchor);
    // buffer_json_member_add_string(wb, "direction", direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    // buffer_json_member_add_uint64(wb, "last", last);
    // buffer_json_member_add_string(wb, "query", query);
    // buffer_json_member_add_string(wb, "chart", chart);
    // buffer_json_member_add_time_t(wb, "timeout", timeout);
    // buffer_json_object_close(wb); // request


    // ------------------------------------------------------------------------
    // run the request

    if(info) {
        facets_accepted_parameters_to_json_array(facets, wb, false);
        buffer_json_member_add_array(wb, "required_params");
        {
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "source");
                buffer_json_member_add_string(wb, "name", "source");
                buffer_json_member_add_string(wb, "help", "Select the Logs Management source to query");
                buffer_json_member_add_string(wb, "type", "select");
                buffer_json_member_add_array(wb, "options");
                const logs_qry_res_err_t *const res_err = fetch_log_sources(wb);
                ret = res_err->http_code;
                buffer_json_array_close(wb); // options array
            }
            buffer_json_object_close(wb); // required params object
        }
        buffer_json_array_close(wb); // required_params array

        facets_table_config(wb);

        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
        buffer_json_member_add_string(wb, "type", "table");
        buffer_json_member_add_string(wb, "help", FUNCTION_LOGSMANAGEMENT_HELP_SHORT);
        buffer_json_finalize(wb);
        ret = HTTP_RESP_OK;
        goto output;
    }
    
    if(progress) {
        function_systemd_journal_progress(wb, transaction, progress_id);
        goto cleanup;
    }

    // if(sources){
    //     buffer_sprintf( wb, 
    //                     ",\n"
    //                     "   \"api version\": %s,\n" 
    //                     "   \"log sources\": {\n",
    //                     LOGS_QRY_VERSION);
    //     const logs_qry_res_err_t *const res_err = fetch_log_sources(wb);
    //     buffer_sprintf( wb, 
    //                     "\n"
    //                     "   },\n"
    //                     "   \"error code\": %d,\n"
    //                     "   \"error\": \"%s\"\n}",
    //                     (int) res_err->err_code,
    //                     res_err->err_str);

    //     ret = res_err->http_code;
    //     goto cleanup;
    // }

    
    // For now, always perform descending timestamp query
    // TODO: FIXME
    query_params.req_from_ts = before_s * MSEC_PER_SEC;
    query_params.req_to_ts = after_s * MSEC_PER_SEC;

    if(fqs->source)
        query_params.chartname[0] = (char *) string2str(fqs->source);

    query_params.order_by_asc = query_params.req_from_ts <= query_params.req_to_ts ? 1 : 0;

    fn_off = cn_off = 0;

    if(!req_quota) query_params.quota = LOGS_MANAG_QUERY_QUOTA_DEFAULT;
    else if(req_quota > LOGS_MANAG_QUERY_QUOTA_MAX) query_params.quota = LOGS_MANAG_QUERY_QUOTA_MAX;
    else query_params.quota = req_quota;

    query_params.stop_monotonic_ut = now_monotonic_usec() + (timeout - 1) * USEC_PER_SEC;

    query_params.results_buff = buffer_create(query_params.quota, NULL);

    const logs_qry_res_err_t *const res_err = execute_logs_manag_query(&query_params);
    ret = res_err->http_code;

    // if(data_only)
    //     facets_data_only_mode(facets);

    facets_rows_begin(facets);


    usec_t last_modified = 0;

    size_t res_off = 0;
    logs_query_res_hdr_t *p_res_hdr;
    while(query_params.results_buff->len - res_off > 0){
        p_res_hdr = (logs_query_res_hdr_t *) &query_params.results_buff->buffer[res_off];



        ssize_t remaining = p_res_hdr->text_size;
        char *ls = &query_params.results_buff->buffer[res_off] + sizeof(*p_res_hdr) + p_res_hdr->text_size - 1;
        *ls = '\0';
        int timestamp_off = p_res_hdr->matches;
        do{
            do{
                --remaining;
                --ls;
            } while(remaining > 0 && *ls != '\n');
            *ls = '\0';
            --remaining;
            --ls;

            usec_t timestamp = p_res_hdr->timestamp * USEC_PER_MS + --timestamp_off;

            if(unlikely(!last_modified)) {
                if(timestamp == if_modified_since){
                    ret = HTTP_RESP_NOT_MODIFIED;
                    goto cleanup;
                }
                
                last_modified = timestamp;
            }

            facets_add_key_value(facets, "log_source", p_res_hdr->log_source[0] ? p_res_hdr->log_source : "-");

            facets_add_key_value(facets, "log_type", p_res_hdr->log_type[0] ? p_res_hdr->log_type : "-");

            facets_add_key_value(facets, "filename", p_res_hdr->filename[0] ? p_res_hdr->filename : "-");

            facets_add_key_value(facets, "basename", p_res_hdr->basename[0] ? p_res_hdr->basename : "-");

            facets_add_key_value(facets, "chartname", p_res_hdr->chartname[0] ? p_res_hdr->chartname : "-");

            facets_add_key_value(facets, "message", ls + 2);

            facets_row_finished(facets, timestamp);

        } while(remaining > 0);

        res_off += sizeof(*p_res_hdr) + p_res_hdr->text_size;

    }

    getrusage(RUSAGE_THREAD, &end);
    time_t user_time =  end.ru_utime.tv_sec * USEC_PER_SEC + end.ru_utime.tv_usec - 
                        start.ru_utime.tv_sec * USEC_PER_SEC - start.ru_utime.tv_usec;
    time_t sys_time =   end.ru_stime.tv_sec * USEC_PER_SEC + end.ru_stime.tv_usec - 
                        start.ru_stime.tv_sec * USEC_PER_SEC - start.ru_stime.tv_usec;

    buffer_json_member_add_object(wb, "logs_management_meta");
    buffer_json_member_add_string(wb, "api_version", LOGS_QRY_VERSION);
    buffer_json_member_add_uint64(wb, "num_lines", query_params.num_lines);
    buffer_json_member_add_uint64(wb, "user_time", user_time);
    buffer_json_member_add_uint64(wb, "system_time", sys_time);
    buffer_json_member_add_uint64(wb, "total_time", user_time + sys_time);
    buffer_json_member_add_uint64(wb, "error_code", (uint64_t) res_err->err_code);
    buffer_json_member_add_string(wb, "error_string", res_err->err_str);
    buffer_json_object_close(wb); // logs_management_meta

    buffer_json_member_add_uint64(wb, "status", ret);
    buffer_json_member_add_boolean(wb, "partial", ret != HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");


    if(!fqs->data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", FUNCTION_LOGSMANAGEMENT_HELP_SHORT);
    }

    if(!fqs->data_only || fqs->tail)
        buffer_json_member_add_uint64(wb, "last_modified", fqs->last_modified);

    facets_sort_and_reorder_keys(facets);
    facets_report(facets, wb, used_hashes_registry);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + (fqs->data_only ? 3600 : 0));
    buffer_json_finalize(wb); // logs_management_meta


    // ------------------------------------------------------------------------
    // cleanup query params

    string_freez(fqs->source);
    fqs->source = NULL;

    // ------------------------------------------------------------------------
    // handle error response

    if(ret != HTTP_RESP_OK) {
        netdata_mutex_lock(&stdout_mut);
        pluginsd_function_json_error_to_stdout(transaction, ret, "failed");
        netdata_mutex_unlock(&stdout_mut);
        goto cleanup;
    }

output:
    netdata_mutex_lock(&stdout_mut);
    pluginsd_function_result_to_stdout(transaction, ret, "application/json", expires, wb);
    netdata_mutex_unlock(&stdout_mut);

cleanup:
    facets_destroy(facets);
    buffer_free(query_params.results_buff);
    buffer_free(wb);

    if(fqs_item) {
        dictionary_del(function_query_status_dict, dictionary_acquired_item_name(fqs_item));
        dictionary_acquired_item_release(function_query_status_dict, fqs_item);
        dictionary_garbage_collect(function_query_status_dict);
    }
}

void logsmanagement_func_facets_init(bool *p_logsmanagement_should_exit){

    function_query_status_dict = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(FUNCTION_QUERY_STATUS));

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    netdata_mutex_lock(&stdout_mut);
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\"\n",
                    LOGS_MANAG_FUNC_NAME, 
                    LOGS_MANAG_QUERY_TIMEOUT_DEFAULT, 
                    FUNCTION_LOGSMANAGEMENT_HELP_SHORT);
    netdata_mutex_unlock(&stdout_mut);

    struct functions_evloop_globals *wg = functions_evloop_init(1, "LGSMNGM", 
                                                                &stdout_mut, 
                                                                p_logsmanagement_should_exit);

    functions_evloop_add_function(  wg, LOGS_MANAG_FUNC_NAME, 
                                    logsmanagement_function_facets,
                                    LOGS_MANAG_QUERY_TIMEOUT_DEFAULT);
}
