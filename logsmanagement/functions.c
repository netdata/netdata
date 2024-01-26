// SPDX-License-Identifier: GPL-3.0-or-later

/** @file functions.c
 *  
 *  @brief This is the file containing the implementation of the 
 *  logs management functions API.
 */

#include "functions.h"
#include "helper.h"
#include "query.h"

#define LOGS_MANAG_MAX_PARAMS       100
#define LOGS_MANAGEMENT_DEFAULT_QUERY_DURATION_IN_SEC  3600
#define LOGS_MANAGEMENT_DEFAULT_ITEMS_PER_QUERY 200

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
#define LOGS_MANAG_FUNC_PARAM_SLICE             "slice"
#define LOGS_MANAG_FUNC_PARAM_DELTA             "delta"
#define LOGS_MANAG_FUNC_PARAM_TAIL              "tail"

#define LOGS_MANAG_DEFAULT_DIRECTION            FACETS_ANCHOR_DIRECTION_BACKWARD

#define FACET_MAX_VALUE_LENGTH      8192

#define FUNCTION_LOGSMANAGEMENT_HELP_LONG \
    LOGS_MANAGEMENT_PLUGIN_STR " / " LOGS_MANAG_FUNC_NAME"\n" \
    "\n" \
    FUNCTION_LOGSMANAGEMENT_HELP_SHORT"\n" \
    "\n" \
    "The following parameters are supported::\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_HELP"\n" \
    "      Shows this help message\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_INFO"\n" \
    "      Request initial configuration information about the plugin.\n" \
    "      The key entity returned is the required_params array, which includes\n" \
    "      all the available "LOGS_MANAG_FUNC_NAME" sources.\n" \
    "      When `"LOGS_MANAG_FUNC_PARAM_INFO"` is requested, all other parameters are ignored.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_DATA_ONLY":true or "LOGS_MANAG_FUNC_PARAM_DATA_ONLY":false\n" \
    "      Quickly respond with data requested, without generating a\n" \
    "      `histogram`, `facets` counters and `items`.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_SOURCE":SOURCE\n" \
    "      Query only the specified "LOGS_MANAG_FUNC_NAME" sources.\n" \
    "      Do an `"LOGS_MANAG_FUNC_PARAM_INFO"` query to find the sources.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_BEFORE":TIMESTAMP_IN_SECONDS\n" \
    "      Absolute or relative (to now) timestamp in seconds, to start the query.\n" \
    "      The query is always executed from the most recent to the oldest log entry.\n" \
    "      If not given the default is: now.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_AFTER":TIMESTAMP_IN_SECONDS\n" \
    "      Absolute or relative (to `before`) timestamp in seconds, to end the query.\n" \
    "      If not given, the default is "LOGS_MANAG_STR(-LOGS_MANAGEMENT_DEFAULT_QUERY_DURATION_IN_SEC)".\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_LAST":ITEMS\n" \
    "      The number of items to return.\n" \
    "      The default is "LOGS_MANAG_STR(LOGS_MANAGEMENT_DEFAULT_ITEMS_PER_QUERY)".\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_ANCHOR":TIMESTAMP_IN_MICROSECONDS\n" \
    "      Return items relative to this timestamp.\n" \
    "      The exact items to be returned depend on the query `"LOGS_MANAG_FUNC_PARAM_DIRECTION"`.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_DIRECTION":forward or "LOGS_MANAG_FUNC_PARAM_DIRECTION":backward\n" \
    "      When set to `backward` (default) the items returned are the newest before the\n" \
    "      `"LOGS_MANAG_FUNC_PARAM_ANCHOR"`, (or `"LOGS_MANAG_FUNC_PARAM_BEFORE"` if `"LOGS_MANAG_FUNC_PARAM_ANCHOR"` is not set)\n" \
    "      When set to `forward` the items returned are the oldest after the\n" \
    "      `"LOGS_MANAG_FUNC_PARAM_ANCHOR"`, (or `"LOGS_MANAG_FUNC_PARAM_AFTER"` if `"LOGS_MANAG_FUNC_PARAM_ANCHOR"` is not set)\n" \
    "      The default is: backward\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_QUERY":SIMPLE_PATTERN\n" \
    "      Do a full text search to find the log entries matching the pattern given.\n" \
    "      The plugin is searching for matches on all fields of the database.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE":TIMESTAMP_IN_MICROSECONDS\n" \
    "      Each successful response, includes a `last_modified` field.\n" \
    "      By providing the timestamp to the `"LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE"` parameter,\n" \
    "      the plugin will return 200 with a successful response, or 304 if the source has not\n" \
    "      been modified since that timestamp.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_HISTOGRAM":facet_id\n" \
    "      Use the given `facet_id` for the histogram.\n" \
    "      This parameter is ignored in `"LOGS_MANAG_FUNC_PARAM_DATA_ONLY"` mode.\n" \
    "\n" \
    "   "LOGS_MANAG_FUNC_PARAM_FACETS":facet_id1,facet_id2,facet_id3,...\n" \
    "      Add the given facets to the list of fields for which analysis is required.\n" \
    "      The plugin will offer both a histogram and facet value counters for its values.\n" \
    "      This parameter is ignored in `"LOGS_MANAG_FUNC_PARAM_DATA_ONLY"` mode.\n" \
    "\n" \
    "   facet_id:value_id1,value_id2,value_id3,...\n" \
    "      Apply filters to the query, based on the facet IDs returned.\n" \
    "      Each `facet_id` can be given once, but multiple `facet_ids` can be given.\n" \
    "\n"


extern netdata_mutex_t stdout_mut;

static DICTIONARY *function_query_status_dict = NULL;

static DICTIONARY *used_hashes_registry = NULL;

typedef struct function_query_status {
    bool *cancelled; // a pointer to the cancelling boolean
    usec_t *stop_monotonic_ut;

    // request
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


#define LOGS_MANAG_KEYS_INCLUDED_IN_FACETS      \
    "log_source"                                \
    "|log_type"                                 \
    "|filename"                                 \
    "|basename"                                 \
    "|chartname"                                \
    "|message"                                  \
    ""

static void logsmanagement_function_facets(const char *transaction, char *function,
                                           usec_t *stop_monotonic_ut, bool *cancelled,
                                           BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
                                           const char *src __maybe_unused, void *data __maybe_unused){

    struct rusage start, end;
    getrusage(RUSAGE_THREAD, &start);

    const logs_qry_res_err_t *ret = &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_SERVER_ERR];

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    FUNCTION_QUERY_STATUS tmp_fqs = {
            .cancelled = cancelled,
            .stop_monotonic_ut = stop_monotonic_ut,
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
    facets_accepted_param(facets, LOGS_MANAG_FUNC_PARAM_DELTA);
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
            /* slice       = true, */
            delta       = false, 
            tail        = false;
    time_t after_s = 0, before_s = 0;
    usec_t anchor = 0;
    usec_t if_modified_since = 0;
    size_t last = 0;
    FACETS_ANCHOR_DIRECTION direction = LOGS_MANAG_DEFAULT_DIRECTION;
    const char *query = NULL;
    const char *chart = NULL;
    const char *source = NULL;
    // size_t filters = 0;

    buffer_json_member_add_object(wb, "_request");

    logs_query_params_t query_params = {0};
    unsigned long req_quota = 0;

    // unsigned int fn_off = 0, cn_off = 0;

    char *words[LOGS_MANAG_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, LOGS_MANAG_MAX_PARAMS);
    for(int i = 1; i < LOGS_MANAG_MAX_PARAMS ; i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        
        if(!strcmp(keyword, LOGS_MANAG_FUNC_PARAM_HELP)){
            BUFFER *tmp = buffer_create(0, NULL);
            buffer_sprintf(tmp, FUNCTION_LOGSMANAGEMENT_HELP_LONG);
            netdata_mutex_lock(&stdout_mut);
            pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, tmp);
            netdata_mutex_unlock(&stdout_mut);
            buffer_free(tmp);
            goto cleanup;
        }
        else if(!strcmp(keyword, LOGS_MANAG_FUNC_PARAM_INFO)){
            info = true;
        }
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_DELTA ":", sizeof(LOGS_MANAG_FUNC_PARAM_DELTA ":") - 1) == 0) {
            char *v = &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_DELTA ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                delta = false;
            else
                delta = true;
        }
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
        else if(strncmp(keyword, LOGS_MANAG_FUNC_PARAM_SOURCE ":", sizeof(LOGS_MANAG_FUNC_PARAM_SOURCE ":") - 1) == 0) {
            source = !strcmp("all", &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_SOURCE ":") - 1]) ? 
                NULL : &keyword[sizeof(LOGS_MANAG_FUNC_PARAM_SOURCE ":") - 1];
        }
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
    }

    fqs = &tmp_fqs;
    fqs_item = NULL;

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
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_DELTA, fqs->delta);
    buffer_json_member_add_boolean(wb, LOGS_MANAG_FUNC_PARAM_TAIL, fqs->tail);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_SOURCE, string2str(fqs->source));
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_AFTER, fqs->after_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_BEFORE, fqs->before_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_IF_MODIFIED_SINCE, fqs->if_modified_since);
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_ANCHOR, anchor);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_DIRECTION, 
        fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, LOGS_MANAG_FUNC_PARAM_LAST, fqs->entries);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_QUERY, fqs->query);
    buffer_json_member_add_string(wb, LOGS_MANAG_FUNC_PARAM_HISTOGRAM, fqs->histogram);
    buffer_json_object_close(wb); // request

    // buffer_json_journal_versions(wb);

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
                ret = fetch_log_sources(wb);
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
        goto output;
    }
    
    if(!req_quota)
        query_params.quota = LOGS_MANAG_QUERY_QUOTA_DEFAULT;
    else if(req_quota > LOGS_MANAG_QUERY_QUOTA_MAX) 
        query_params.quota = LOGS_MANAG_QUERY_QUOTA_MAX;
    else query_params.quota = req_quota;


    if(fqs->source)
        query_params.chartname[0] = (char *) string2str(fqs->source);

    query_params.order_by_asc = 0;

    
    // NOTE: Always perform descending timestamp query, req_from_ts >= req_to_ts.
    if(fqs->direction == FACETS_ANCHOR_DIRECTION_BACKWARD){
        query_params.req_from_ts = 
            (fqs->data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut / USEC_PER_MS : before_s * MSEC_PER_SEC;
        query_params.req_to_ts = 
            (fqs->data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut / USEC_PER_MS : after_s * MSEC_PER_SEC;
    }
    else{
        query_params.req_from_ts =
            (fqs->data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut / USEC_PER_MS : before_s * MSEC_PER_SEC;
        query_params.req_to_ts = 
            (fqs->data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut / USEC_PER_MS : after_s * MSEC_PER_SEC;
    }
    
    query_params.cancelled = cancelled;
    query_params.stop_monotonic_ut = stop_monotonic_ut;
    query_params.results_buff = buffer_create(query_params.quota, NULL);

    facets_rows_begin(facets);

    do{
        if(query_params.act_to_ts)
            query_params.req_from_ts = query_params.act_to_ts - 1000;

        ret = execute_logs_manag_query(&query_params);


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

                if(unlikely(!fqs->last_modified)) {
                    if(timestamp == if_modified_since){
                        ret = &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_UNMODIFIED];
                        goto output;
                    }
                    else                    
                        fqs->last_modified = timestamp;
                }

                facets_add_key_value(facets, "log_source", p_res_hdr->log_source[0] ? p_res_hdr->log_source : "-");

                facets_add_key_value(facets, "log_type", p_res_hdr->log_type[0] ? p_res_hdr->log_type : "-");

                facets_add_key_value(facets, "filename", p_res_hdr->filename[0] ? p_res_hdr->filename : "-");

                facets_add_key_value(facets, "basename", p_res_hdr->basename[0] ? p_res_hdr->basename : "-");

                facets_add_key_value(facets, "chartname", p_res_hdr->chartname[0] ? p_res_hdr->chartname : "-");

                size_t ls_len = strlen(ls + 2);
                facets_add_key_value_length(facets, "message", sizeof("message") - 1, 
                                            ls + 2, ls_len <= FACET_MAX_VALUE_LENGTH ? ls_len : FACET_MAX_VALUE_LENGTH);

                facets_row_finished(facets, timestamp);

            } while(remaining > 0);

            res_off += sizeof(*p_res_hdr) + p_res_hdr->text_size;

        }

        buffer_flush(query_params.results_buff);

    } while(query_params.act_to_ts > query_params.req_to_ts);

    m_assert(query_params.req_from_ts == query_params.act_from_ts, "query_params.req_from_ts != query_params.act_from_ts");
    m_assert(query_params.req_to_ts   == query_params.act_to_ts  , "query_params.req_to_ts != query_params.act_to_ts");
    

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
    buffer_json_member_add_uint64(wb, "error_code", (uint64_t) ret->err_code);
    buffer_json_member_add_string(wb, "error_string", ret->err_str);
    buffer_json_object_close(wb); // logs_management_meta

    buffer_json_member_add_uint64(wb, "status", ret->http_code);
    buffer_json_member_add_boolean(wb, "partial", ret->http_code != HTTP_RESP_OK || 
                                                  ret->err_code == LOGS_QRY_RES_ERR_CODE_TIMEOUT);
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

output:
    netdata_mutex_lock(&stdout_mut);
    if(ret->http_code != HTTP_RESP_OK)
        pluginsd_function_json_error_to_stdout(transaction, ret->http_code, ret->err_str);
    else
        pluginsd_function_result_to_stdout(transaction, ret->http_code, "application/json", expires, wb);
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

struct functions_evloop_globals *logsmanagement_func_facets_init(bool *p_logsmanagement_should_exit){

    function_query_status_dict = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
            NULL, sizeof(FUNCTION_QUERY_STATUS));

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    netdata_mutex_lock(&stdout_mut);
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"logs\" "HTTP_ACCESS_FORMAT" %d\n",
                    LOGS_MANAG_FUNC_NAME, 
                    LOGS_MANAG_QUERY_TIMEOUT_DEFAULT, 
                    FUNCTION_LOGSMANAGEMENT_HELP_SHORT,
                    (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
                    RRDFUNCTIONS_PRIORITY_DEFAULT + 1);
    netdata_mutex_unlock(&stdout_mut);

    struct functions_evloop_globals *wg = functions_evloop_init(1, "LGSMNGM", 
                                                                &stdout_mut, 
                                                                p_logsmanagement_should_exit);

    functions_evloop_add_function(  wg, LOGS_MANAG_FUNC_NAME, 
                                    logsmanagement_function_facets,
                                    LOGS_MANAG_QUERY_TIMEOUT_DEFAULT,
                                  NULL);
    
    return wg;
}
