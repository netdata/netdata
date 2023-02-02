// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_logsmanagement.h"
#include "../../database/rrdfunctions.h"
#include "../../logsmanagement/query.h"

/* NETDATA_CHART_PRIO for Stats_chart_data */
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_TOT    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 1
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_UNC    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 2
#define NETDATA_CHART_PRIO_CIRC_BUFF_MEM_COM    NETDATA_CHART_PRIO_LOGS_STATS_BASE + 3
#define NETDATA_CHART_PRIO_COMPR_RATIO          NETDATA_CHART_PRIO_LOGS_STATS_BASE + 4
#define NETDATA_CHART_PRIO_DISK_USAGE           NETDATA_CHART_PRIO_LOGS_STATS_BASE + 5

#define NETDATA_CHART_PRIO_LOGS_INCR            100  /**< PRIO increment step from one log source to another **/

#define WORKER_JOB_COLLECT                      0
#define WORKER_JOB_UPDATE                       1

#define LOGS_MANAG_STR_HELPER(x) #x
#define LOGS_MANAG_STR(x) LOGS_MANAG_STR_HELPER(x)
#define FUNCTION_LOGSMANAGEMENT_HELP_SHORT      "Query of logs management engine running on this node"
#define FUNCTION_LOGSMANAGEMENT_HELP_LONG       \
    "logsmanagement\n\n" \
    "Function 'logsmanagement' enables querying of the logs management engine and retrieval of logs stored on this node. \n\n" \
    "Arguments:\n\n" \
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
    "All arguments except for either '"LOGS_QRY_KW_CHARTNAME"' or '"LOGS_QRY_KW_FILENAME"' are optional."


static struct Chart_meta chart_types[] = {
    {.type = GENERIC,       .init = generic_chart_init,   .collect = generic_chart_collect,   .update = generic_chart_update},
    {.type = FLB_GENERIC,   .init = generic_chart_init,   .collect = generic_chart_collect,   .update = generic_chart_update},
    {.type = WEB_LOG,       .init = web_log_chart_init,   .collect = web_log_chart_collect,   .update = web_log_chart_update},
    {.type = FLB_WEB_LOG,   .init = web_log_chart_init,   .collect = web_log_chart_collect,   .update = web_log_chart_update},
    {.type = FLB_SYSTEMD,   .init = systemd_chart_init,   .collect = systemd_chart_collect,   .update = systemd_chart_update},
    {.type = FLB_DOCKER_EV, .init = docker_ev_chart_init, .collect = docker_ev_chart_collect, .update = docker_ev_chart_update},
    {.type = FLB_SYSLOG,    .init = systemd_chart_init,   .collect = systemd_chart_collect,   .update = systemd_chart_update},
    {.type = FLB_SERIAL,    .init = generic_chart_init,   .collect = generic_chart_collect,   .update = generic_chart_update}
};

struct Stats_chart_data{
    char *rrd_type;

    RRDSET *st_circ_buff_mem_total;
    RRDDIM **dim_circ_buff_mem_total_arr;
    collected_number *num_circ_buff_mem_total_arr;

    RRDSET *st_circ_buff_mem_uncompressed;
    RRDDIM **dim_circ_buff_mem_uncompressed_arr;
    collected_number *num_circ_buff_mem_uncompressed_arr;

    RRDSET *st_circ_buff_mem_compressed;
    RRDDIM **dim_circ_buff_mem_compressed_arr;
    collected_number *num_circ_buff_mem_compressed_arr;

    RRDSET *st_compression_ratio;
    RRDDIM **dim_compression_ratio;
    collected_number *num_compression_ratio_arr;

    RRDSET *st_disk_usage;
    RRDDIM **dim_disk_usage;
    collected_number *num_disk_usage_arr;
};

static struct Stats_chart_data *stats_chart_data;
static struct Chart_meta **chart_data_arr;

static void logsmanagement_plugin_main_cleanup(void *ptr) {
    rrd_collector_finished();
    worker_unregister();
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

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

static int logsmanagement_function_execute_cb(  BUFFER *dest_wb, int timeout, 
                                                const char *function, void *collector_data, 
                                                void (*callback)(BUFFER *wb, int code, void *callback_data), 
                                                void *callback_data) {

    UNUSED(timeout);
    UNUSED(collector_data);
    UNUSED(callback);
    UNUSED(callback_data);

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
        .results_buff = buffer_create(query_params.quota),
        .keyword_matches = 0
    };

    unsigned int fn_off = 0, cn_off = 0;

    while(function){
        char *value = mystrsep((char **) &function, " ");
        if (!value || !*value) continue;
        else if(!strcmp(value, "help")){
            buffer_sprintf(dest_wb, FUNCTION_LOGSMANAGEMENT_HELP_LONG);
            dest_wb->contenttype = CT_TEXT_PLAIN;
            return HTTP_RESP_OK;
        }

        char *key = mystrsep(&value, ":");
        if(!key || !*key) continue;
        if(!value || !*value) continue;

        // Kludge to respect quotes with spaces in between
        // Not proper fix
        if(*value == '_'){
            value++;
            value[strlen(value)] = ' '; 
            value = mystrsep(&value, "_");
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
            query_params.quota = (size_t) strtoll(value, NULL, 10);
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
            error("functions: logsmanagement invalid parameter");
            return HTTP_RESP_BAD_REQUEST;
        }
    }

    const uint64_t  req_start_timestamp = query_params.start_timestamp,
                    req_end_timestamp = query_params.end_timestamp;
    struct rusage start, end;
    getrusage(RUSAGE_THREAD, &start);
    LOGS_QUERY_RESULT_TYPE err_code = execute_logs_manag_query(&query_params); // WARNING! query changes start_timestamp and end_timestamp
    getrusage(RUSAGE_THREAD, &end);

    int status;
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
                    "      \"requested_from\": %" PRIu64 ",\n"
                    "      \"requested_until\": %" PRIu64 ",\n"
                    "      \"requested_keyword\": \"%s\",\n",
                    status,
                    update_every,
                    QUERY_VERSION,
                    req_start_timestamp,
                    req_end_timestamp,
                    query_params.keyword ? query_params.keyword : ""
    );
     
    buffer_sprintf( dest_wb,
                    "      \"actual_from\": %" PRIu64 ",\n"
                    "      \"actual_until\": %" PRIu64 ",\n"
                    "      \"quota\": %zu,\n"
                    "      \"requested_filename\": [\n",
                    query_params.start_timestamp,
                    query_params.end_timestamp,
                    query_params.quota
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
                    "      \"keyword_matches\": %d, \n"
                    "      \"user_time\": %llu,\n"
                    "      \"system_time\": %llu,\n"
                    "      \"error_code\": %d,\n"
                    "      \"error\": \"",
                    query_params.keyword_matches,
                    end.ru_utime.tv_sec * 1000000ULL + end.ru_utime.tv_usec - 
                    start.ru_utime.tv_sec * 1000000ULL - start.ru_utime.tv_usec,
                    end.ru_stime.tv_sec * 1000000ULL + end.ru_stime.tv_usec - 
                    start.ru_stime.tv_sec * 1000000ULL - start.ru_stime.tv_usec,
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
                        "         %" PRIu64 ",\n", 
                        res_hdr.timestamp
        );

        if(likely(query_params.data_format == LOGS_QUERY_DATA_FORMAT_JSON_ARRAY)) 
            buffer_strcat(dest_wb, "         [\n   ");

        buffer_strcat(dest_wb, "         \"");

        /* Unfortunately '\n', '\\' and '"' need to be escaped, so we need to go 
         * through the result characters one by one. */
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
            else if(unlikely(*p == '\\')) {
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
    add_table_field(dest_wb, "Timestamp", "Timestamp in Milliseconds", true, "time", "milliseconds", NAN, "ascending", true, true, true, NULL, "max", false);
    add_table_field(dest_wb, "Logs", "Logs collected in last interval", true, "string", NULL, NAN, "ascending", true, true, true, NULL, "count_unique", false);
    add_table_field(dest_wb, "LogsTxtSz", "Logs text length", true, "string", NULL, NAN, "ascending", true, true, true, NULL, "count_unique", false);
    add_table_field(dest_wb, "MatchNo", "Keyword matches", true, "integer", NULL, NAN, "ascending", true, true, true, NULL, "sum", false);

    buffer_sprintf( dest_wb, 
                    "\n   },"
                    "\n   \"expires\": %lld"
                    "\n}",
                    (long long) now_realtime_sec() + update_every);

    buffer_free(query_params.results_buff);

    // buffer_no_cacheable(dest_wb);  

    return status;
}


void *logsmanagement_plugin_main(void *ptr){
    worker_register("LOGSMANAGPLG");
    worker_register_job_name(WORKER_JOB_COLLECT, "collection");
    worker_register_job_name(WORKER_JOB_UPDATE, "update");

    rrd_collector_started();

	netdata_thread_cleanup_push(logsmanagement_plugin_main_cleanup, ptr);

    /* wait for p_file_infos_arr initialisation */
    for(int retries = 20; !p_file_infos_arr_ready; retries--){
        sleep_usec(500 * USEC_PER_MS);
        if(retries == 0) goto cleanup;
    }

    stats_chart_data = callocz(1, sizeof(struct Stats_chart_data));
    stats_chart_data->rrd_type = "netdata";

    /* Circular buffer total memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_total = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular buffer memory total"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Circular buffers total memory"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_TOT 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_circ_buff_mem_total_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_total_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Circular buffer uncompressed buffered items memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_uncompressed = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular buffer uncompressed buffered total"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Circular buffers uncompressed buffered total"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_UNC 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_circ_buff_mem_uncompressed_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_uncompressed_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Circular buffer compressed buffered items memory stats - initialise */
    stats_chart_data->st_circ_buff_mem_compressed = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "circular buffer compressed buffered total"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Circular buffers compressed buffered total"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_CIRC_BUFF_MEM_COM 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_circ_buff_mem_compressed_arr = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_circ_buff_mem_compressed_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* Compression stats - initialise */
    stats_chart_data->st_compression_ratio = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "average compression ratio"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Average compression ratio"
            , "uncompressed / compressed ratio"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_COMPR_RATIO 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_compression_ratio = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_compression_ratio_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));

    /* DB disk usage stats - initialise */
    stats_chart_data->st_disk_usage = rrdset_create_localhost(
            stats_chart_data->rrd_type
            , "database disk usage"
            , NULL
            , "logsmanagement.plugin"
            , NULL
            , "Database disk usage"
            , "bytes"
            , "logsmanagement.plugin"
            , NULL
            , NETDATA_CHART_PRIO_DISK_USAGE 
            , g_logs_manag_update_every
            , RRDSET_TYPE_AREA
    );
    stats_chart_data->dim_disk_usage = callocz(p_file_infos_arr->count, sizeof(RRDDIM));
    stats_chart_data->num_disk_usage_arr = callocz(p_file_infos_arr->count, sizeof(collected_number));


    chart_data_arr = callocz(p_file_infos_arr->count, sizeof(struct Chart_meta *));

    for(int i = 0; i < p_file_infos_arr->count; i++){

        struct File_info *p_file_info = p_file_infos_arr->data[i];
        if(!p_file_info->parser_config){ // Check if there is parser configuration to be used for chart generation
            chart_data_arr[i] = NULL;
            continue; 
        } 

        /* Circular buffer memory stats - add dimensions */
        stats_chart_data->dim_circ_buff_mem_total_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_mem_total, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats_chart_data->dim_circ_buff_mem_uncompressed_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_mem_uncompressed, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        stats_chart_data->dim_circ_buff_mem_compressed_arr[i] = 
            rrddim_add( stats_chart_data->st_circ_buff_mem_compressed, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* Compression stats - add dimensions */
        stats_chart_data->dim_compression_ratio[i] = 
            rrddim_add( stats_chart_data->st_compression_ratio, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        /* DB disk usage stats - add dimensions */
        stats_chart_data->dim_disk_usage[i] = 
            rrddim_add( stats_chart_data->st_disk_usage, 
                        p_file_info->chart_name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        chart_data_arr[i] = callocz(1, sizeof(struct Chart_meta));
        memcpy(chart_data_arr[i], &chart_types[p_file_info->log_type], sizeof(struct Chart_meta));
        chart_data_arr[i]->base_prio = NETDATA_CHART_PRIO_LOGS_BASE + (i + 1) * NETDATA_CHART_PRIO_LOGS_INCR;
        chart_data_arr[i]->init(p_file_info, chart_data_arr[i]);

        /* Custom charts - initialise */
        for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
            chart_data_arr[i]->chart_data_cus_arr = reallocz( chart_data_arr[i]->chart_data_cus_arr, 
                                                              (cus_off + 1) * sizeof(Chart_data_cus_t *));
            chart_data_arr[i]->chart_data_cus_arr[cus_off] = callocz(1, sizeof(Chart_data_cus_t));

            RRDSET *st_cus = rrdset_find_active_bytype_localhost( p_file_info->chart_name, 
                                                                  p_file_info->parser_cus_config[cus_off]->chart_name);
            if(st_cus) chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus = st_cus;
            else {
                chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus = rrdset_create_localhost(
                        (char *) p_file_info->chart_name
                        , p_file_info->parser_cus_config[cus_off]->chart_name
                        , NULL
                        , "regex charts"
                        , NULL
                        , p_file_info->parser_cus_config[cus_off]->chart_name
                        , "matches/s"
                        , "logsmanagement.plugin"
                        , NULL
                        , chart_data_arr[i]->base_prio + 1000 + cus_off
                        , p_file_info->update_every
                        , RRDSET_TYPE_AREA
                );
                // rrdset_done() need to be run for this
                chart_data_arr[i]->chart_data_cus_arr[cus_off]->need_rrdset_done = 1; 
            }  
            chart_data_arr[i]->chart_data_cus_arr[cus_off]->dim_cus_count = 
                rrddim_add( chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus, 
                            p_file_info->parser_cus_config[cus_off]->regex_name, NULL, 
                            1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

    }


    usec_t step = g_logs_manag_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    rrd_collector_add_function( localhost, NULL, "logsmanagement", 10, FUNCTION_LOGSMANAGEMENT_HELP_SHORT, true, 
                                logsmanagement_function_execute_cb, NULL);

	while(!netdata_exit){

        worker_is_idle();
        (void) heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        for(int i = 0; i < p_file_infos_arr->count; i++){
            struct File_info *p_file_info = p_file_infos_arr->data[i];
            
            // Check if there is parser configuration to be used for chart generation
            if(!p_file_info->parser_config) continue; 

            worker_is_busy(WORKER_JOB_COLLECT);

            /* Circular buffer total memory stats - collect (no need to be within p_file_info->parser_metrics_mut lock) */
            stats_chart_data->num_circ_buff_mem_total_arr[i] = __atomic_load_n(&p_file_info->circ_buff->total_cached_mem, __ATOMIC_RELAXED);

            /* Circular buffer buffered uncompressed & compressed memory stats - collect */
            stats_chart_data->num_circ_buff_mem_uncompressed_arr[i] = __atomic_load_n(&p_file_info->circ_buff->text_size_total, __ATOMIC_RELAXED);
            stats_chart_data->num_circ_buff_mem_compressed_arr[i] = __atomic_load_n(&p_file_info->circ_buff->text_compressed_size_total, __ATOMIC_RELAXED);

            /* Compression stats - collect */
            stats_chart_data->num_compression_ratio_arr[i] = __atomic_load_n(&p_file_info->circ_buff->compression_ratio, __ATOMIC_RELAXED);

            /* DB disk usage stats - collect */
            stats_chart_data->num_disk_usage_arr[i] = __atomic_load_n(&p_file_info->blob_total_size, __ATOMIC_RELAXED);


            uv_mutex_lock(p_file_info->parser_metrics_mut);

            chart_data_arr[i]->collect(p_file_info, chart_data_arr[i]);

            /* Custom charts - collect */
            for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
                chart_data_arr[i]->chart_data_cus_arr[cus_off]->num_cus_count += 
                    p_file_info->parser_metrics->parser_cus[cus_off]->count;
                p_file_info->parser_metrics->parser_cus[cus_off]->count = 0;
            }

            uv_mutex_unlock(p_file_info->parser_metrics_mut);


            if(unlikely(netdata_exit)) break;

            worker_is_busy(WORKER_JOB_UPDATE);

            /* Circular buffer total memory stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_total, 
                                  stats_chart_data->dim_circ_buff_mem_total_arr[i], 
                                  stats_chart_data->num_circ_buff_mem_total_arr[i]);
            
            /* Circular buffer buffered compressed & uncompressed memory stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_uncompressed, 
                                  stats_chart_data->dim_circ_buff_mem_uncompressed_arr[i], 
                                  stats_chart_data->num_circ_buff_mem_uncompressed_arr[i]);
            rrddim_set_by_pointer(stats_chart_data->st_circ_buff_mem_compressed, 
                                  stats_chart_data->dim_circ_buff_mem_compressed_arr[i], 
                                  stats_chart_data->num_circ_buff_mem_compressed_arr[i]);

            /* Compression stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_compression_ratio, 
                                  stats_chart_data->dim_compression_ratio[i], 
                                  stats_chart_data->num_compression_ratio_arr[i]);

            /* DB disk usage stats - update chart */
            rrddim_set_by_pointer(stats_chart_data->st_disk_usage, 
                                  stats_chart_data->dim_disk_usage[i], 
                                  stats_chart_data->num_disk_usage_arr[i]);

            chart_data_arr[i]->update(p_file_info, chart_data_arr[i]);

            /* Custom charts - update chart */
            for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
                rrddim_set_by_pointer(  chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus,
                                        chart_data_arr[i]->chart_data_cus_arr[cus_off]->dim_cus_count,
                                        chart_data_arr[i]->chart_data_cus_arr[cus_off]->num_cus_count);
            }
            for(int cus_off = 0; p_file_info->parser_cus_config[cus_off]; cus_off++){
                if(chart_data_arr[i]->chart_data_cus_arr[cus_off]->need_rrdset_done){
                    rrdset_done(chart_data_arr[i]->chart_data_cus_arr[cus_off]->st_cus);
                }
            }

        }

        // outside for loop as dimensions updated across different loop iterations, unlike chart_data_arr metrics.
        rrdset_done(stats_chart_data->st_circ_buff_mem_total); 
        rrdset_done(stats_chart_data->st_circ_buff_mem_uncompressed);
        rrdset_done(stats_chart_data->st_circ_buff_mem_compressed);
        rrdset_done(stats_chart_data->st_compression_ratio);
        rrdset_done(stats_chart_data->st_disk_usage);
    }

cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
