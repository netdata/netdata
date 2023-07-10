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
    "logsmanagement\n\n" \
    "Function 'logsmanagement' enables querying of the logs management engine and retrieval of logs stored on this node. \n\n" \
    "Arguments:\n\n" \
    "   help\n" \
    "      prints this help message and returns\n\n" \
    "   sources\n" \
    "      returns a list of available log sources to be queried\n\n" \
    "   "LOGS_QRY_KW_START_TIME":NUMBER\n" \
    "      start timestamp in ms to search from, default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_START_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_END_TIME":NUMBER\n" \
    "      end timestamp in ms to search until, default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_END_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_QUOTA":NUMBER\n" \
    "      max size of logs to return (in MiB), default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_QUOTA_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_CHARTNAME":STRING\n" \
    "      Chart name (or names if provided multiple times) to be queried for logs, max No. of sources: " \
            LOGS_MANAG_STR(LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES) "\n\n" \
    "   "LOGS_QRY_KW_FILENAME":STRING\n" \
    "      If no 'chart_name' is provided, file name (or names if provided multiple times) to be queried for logs, max No. of sources: " \
            LOGS_MANAG_STR(LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES) "\n\n" \
    "   "LOGS_QRY_KW_KEYWORD":STRING\n" \
    "      Keyword to be searched in the queried logs\n\n" \
    "   "LOGS_QRY_KW_IGNORE_CASE":BOOL\n" \
    "      Case-sensitive keyword search if set to 0, default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_IGNORE_CASE_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_SANITIZE_KW":BOOL\n" \
    "      If non-zero, the keyword will be sanitized before used by the regex engine (it will *not* be interpreted as a regex), default: " \
            LOGS_MANAG_STR(LOGS_MANAG_QUERY_SANITIZE_KEYWORD_DEFAULT) "\n\n" \
    "   "LOGS_QRY_KW_DATA_FORMAT":STRING\n" \
    "      Grouping of results per collection interval, options: '"LOGS_QRY_KW_JSON_ARRAY"' (default), '"LOGS_QRY_KW_NEWLINE"'\n\n" \
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

    UNUSED(timeout);
    UNUSED(collector_data);
    UNUSED(callback);
    UNUSED(callback_data);

    int status;

    logs_query_params_t query_params = {  
        .start_timestamp = LOGS_MANAG_QUERY_START_DEFAULT,
        .end_timestamp = LOGS_MANAG_QUERY_END_DEFAULT, /* default from / until to return all timestamps */
        .quota = LOGS_MANAG_QUERY_QUOTA_DEFAULT,
        .chart_name = {0},
        .filename = {0},
        .keyword = NULL,
        .ignore_case = 0,
        .sanitize_keyword = 0,
        .data_format = LOGS_QUERY_DATA_FORMAT_JSON_ARRAY,
        .results_buff = buffer_create(query_params.quota, &netdata_buffers_statistics.buffers_functions),
        .num_lines = 0
    };

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
                            QUERY_VERSION);
            LOGS_QUERY_RESULT_TYPE err_code = fetch_log_sources(dest_wb);
            buffer_sprintf( dest_wb, 
                            "\n"
                            "   },\n"
                            "   \"error code\": %d,\n"
                            "   \"error\": \"",
                            err_code);
            
            switch(err_code){
                case GENERIC_ERROR:
                    buffer_strcat(dest_wb, "query generic error");
                    status = HTTP_RESP_BACKEND_FETCH_FAILED;
                    break;
                case NO_RESULTS_FOUND:
                    buffer_strcat(dest_wb, "no results found");
                    status = HTTP_RESP_OK;
                    break;
                default:
                    buffer_strcat(dest_wb, "no error");
                    status = HTTP_RESP_OK;
                    break;
            } 
            buffer_strcat(dest_wb, "\"\n}");

            return status;
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

        if(!strcmp(key, LOGS_QRY_KW_START_TIME)){
            query_params.start_timestamp = strtoll(value, NULL, 10);
        }
        else if(!strcmp(key, LOGS_QRY_KW_END_TIME)){
            query_params.end_timestamp = strtoll(value, NULL, 10);
        }
        else if(!strcmp(key, LOGS_QRY_KW_QUOTA)){
            query_params.quota = strtoll(value, NULL, 10);
        }
        else if(!strcmp(key, LOGS_QRY_KW_FILENAME) && 
                fn_off < LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES){
            query_params.filename[fn_off++] = value;
        }
        else if(!strcmp(key, LOGS_QRY_KW_CHARTNAME) && 
                cn_off < LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES){
            query_params.chart_name[cn_off++] = value;
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
        else if(unlikely(!strcmp(key, LOGS_QRY_KW_DATA_FORMAT) && !strcmp(value, LOGS_QRY_KW_NEWLINE))) {
            query_params.data_format = LOGS_QUERY_DATA_FORMAT_NEW_LINE;
        }
        else {
            collector_error("functions: logsmanagement invalid parameter");
            return HTTP_RESP_BAD_REQUEST;
        }
    }

    const msec_t req_start_timestamp = query_params.start_timestamp,
                 req_end_timestamp = query_params.end_timestamp;
    const unsigned long req_quota = query_params.quota;
    struct rusage start, end;
    getrusage(RUSAGE_THREAD, &start);
    LOGS_QUERY_RESULT_TYPE err_code = execute_logs_manag_query(&query_params); // WARNING! query may change start_timestamp, end_timestamp and quota
    getrusage(RUSAGE_THREAD, &end);

    switch(err_code){
        case INVALID_REQUEST_ERROR:
        case NO_MATCHING_CHART_OR_FILENAME_ERROR:
            status = HTTP_RESP_BAD_REQUEST;
            break;
        case GENERIC_ERROR:
            status = HTTP_RESP_BACKEND_FETCH_FAILED;
            break;
        default:
            status = HTTP_RESP_OK;
    }

    fn_off = cn_off = 0;

    int update_every = 1;

    buffer_sprintf( dest_wb,
                    "{\n"
                    "   \"status\": %d,\n"
                    "   \"type\": \"table\",\n"
                    "   \"update_every\": %d,\n"
                    "   \"logs_management_meta\": {\n"
                    "      \"api_version\": %s,\n"
                    "      \"requested_from\": %llu,\n"
                    "      \"requested_until\": %llu,\n"
                    "      \"requested_quota\": %llu,\n"
                    "      \"requested_keyword\": \"%s\",\n",
                    status,
                    update_every,
                    QUERY_VERSION,
                    req_start_timestamp,
                    req_end_timestamp,
                    req_quota / (1 KiB),
                    query_params.keyword ? query_params.keyword : ""
    );
     
    buffer_sprintf( dest_wb,
                    "      \"actual_from\": %llu,\n"
                    "      \"actual_until\": %llu,\n"
                    "      \"actual_quota\": %llu,\n"
                    "      \"requested_filename\": [\n",
                    query_params.start_timestamp,
                    query_params.end_timestamp,
                    query_params.quota / (1 KiB)
    );
    while(query_params.filename[fn_off]) buffer_sprintf(dest_wb, "         \"%s\",\n", query_params.filename[fn_off++]);
    if(query_params.filename[0])  dest_wb->len -= 2;

    buffer_strcat(  dest_wb, 
                    "\n      ],\n"
                    "      \"requested_chart_name\": [\n"
    );
    while(query_params.chart_name[cn_off]) buffer_sprintf(dest_wb, "         \"%s\",\n", query_params.chart_name[cn_off++]);
    if(query_params.chart_name[0])  dest_wb->len -= 2;
    buffer_strcat(dest_wb, "\n      ],\n");

    buffer_sprintf( dest_wb, 
                    "      \"num_lines\": %lu, \n"
                    "      \"user_time\": %llu,\n"
                    "      \"system_time\": %llu,\n"
                    "      \"error_code\": %d,\n"
                    "      \"error\": \"",
                    query_params.num_lines,
                    end.ru_utime.tv_sec * USEC_PER_SEC + end.ru_utime.tv_usec - 
                    start.ru_utime.tv_sec * USEC_PER_SEC - start.ru_utime.tv_usec,
                    end.ru_stime.tv_sec * USEC_PER_SEC + end.ru_stime.tv_usec - 
                    start.ru_stime.tv_sec * USEC_PER_SEC - start.ru_stime.tv_usec,
                    err_code
    );
    switch(err_code){
        case GENERIC_ERROR:
            buffer_strcat(dest_wb, "query generic error");
            break;
        case INVALID_REQUEST_ERROR:
            buffer_strcat(dest_wb, "invalid request");
            break;
        case NO_MATCHING_CHART_OR_FILENAME_ERROR:
            buffer_strcat(dest_wb, "no matching chart or filename found");
            break;
        case NO_RESULTS_FOUND:
            buffer_strcat(dest_wb, "no results found");
            break;
        default:
            buffer_strcat(dest_wb, "success");
            break;
    } 
    buffer_strcat(  dest_wb, 
                    "\"\n"
                    "   },\n"
                    "   \"data\":[\n"
    );

    size_t res_off = 0;
    logs_query_res_hdr_t res_hdr;
    while(query_params.results_buff->len - res_off > 0){
        memcpy(&res_hdr, &query_params.results_buff->buffer[res_off], sizeof(res_hdr));
        
        buffer_sprintf( dest_wb, 
                        "      [\n"
                        "         %llu,\n", 
                        res_hdr.timestamp
        );

        if(likely(query_params.data_format == LOGS_QUERY_DATA_FORMAT_JSON_ARRAY)) 
            buffer_strcat(dest_wb, "         [\n   ");

        buffer_strcat(dest_wb, "         \"");

        /* Unfortunately '\n', '\\' (except for "\\n") and '"' need to be 
         * escaped, so we need to go through the result characters one by one.*/
        char *p = &query_params.results_buff->buffer[res_off] + sizeof(res_hdr);
        size_t remaining = res_hdr.text_size;
        while (remaining--){
            if(unlikely(*p == '\n')){
                if(likely(query_params.data_format == LOGS_QUERY_DATA_FORMAT_JSON_ARRAY)){
                    buffer_strcat(dest_wb, "\",\n            \"");
                } else {
                    buffer_need_bytes(dest_wb, 2);
                    dest_wb->buffer[dest_wb->len++] = '\\';
                    dest_wb->buffer[dest_wb->len++] = 'n';
                }
                
            } 
            else if(unlikely(*p == '\\' && *(p+1) != 'n')) {
                buffer_need_bytes(dest_wb, 2);
                dest_wb->buffer[dest_wb->len++] = '\\';
                dest_wb->buffer[dest_wb->len++] = '\\';
            }
            else if(unlikely(*p == '"')) {
                buffer_need_bytes(dest_wb, 2);
                dest_wb->buffer[dest_wb->len++] = '\\';
                dest_wb->buffer[dest_wb->len++] = '"';
            }
            else {
                // Escape control characters like [90m
                if(unlikely(iscntrl(*p) && *(p+1) == '[')) {
                    while(*p != 'm'){
                        buffer_need_bytes(dest_wb, 1);
                        p++;
                        remaining--;
                    }
                }
                else{
                    buffer_need_bytes(dest_wb, 1);
                    dest_wb->buffer[dest_wb->len++] = *p;
                }
            }
            p++;
        }
        buffer_strcat(dest_wb, "\"");

        if(likely(query_params.data_format == LOGS_QUERY_DATA_FORMAT_JSON_ARRAY)) 
            buffer_strcat(dest_wb, "\n         ]");
        
        buffer_sprintf( dest_wb, 
                        ",\n"
                        "         %zu,\n"
                        "         %d\n"
                        "      ]" , 
                        res_hdr.text_size,
                        res_hdr.matches);

        res_off += sizeof(res_hdr) + res_hdr.text_size;

        // Add comma and new line if there are more data to be printed
        if(query_params.results_buff->len - res_off > 0) buffer_strcat(dest_wb, ",\n");
    }

    buffer_fast_strcat(dest_wb, "\n   ],\n   \"columns\": {", sizeof("\n   ],\n   \"columns\": {") - 1);
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

    // buffer_no_cacheable(dest_wb);  

    return status;
}
