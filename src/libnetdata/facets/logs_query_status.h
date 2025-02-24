// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOGS_QUERY_STATUS_H
#define NETDATA_LOGS_QUERY_STATUS_H

#include "../libnetdata.h"

#define LQS_PARAMETER_HELP "help"
#define LQS_PARAMETER_AFTER "after"
#define LQS_PARAMETER_BEFORE "before"
#define LQS_PARAMETER_ANCHOR "anchor"
#define LQS_PARAMETER_LAST "last"
#define LQS_PARAMETER_QUERY "query"
#define LQS_PARAMETER_FACETS "facets"
#define LQS_PARAMETER_HISTOGRAM "histogram"
#define LQS_PARAMETER_DIRECTION "direction"
#define LQS_PARAMETER_IF_MODIFIED_SINCE "if_modified_since"
#define LQS_PARAMETER_DATA_ONLY "data_only"
#define LQS_PARAMETER_SOURCE "__logs_sources" // this must never conflict with user fields
#define LQS_PARAMETER_INFO "info"
#define LQS_PARAMETER_SLICE "slice"
#define LQS_PARAMETER_DELTA "delta"
#define LQS_PARAMETER_TAIL "tail"
#define LQS_PARAMETER_SAMPLING "sampling"

#define LQS_MAX_PARAMS 1000
#define LQS_DEFAULT_QUERY_DURATION (1 * 3600)

#undef LQS_SLICE_PARAMETER
#if LQS_DEFAULT_SLICE_MODE == 1
#define LQS_SLICE_PARAMETER 1
#endif

typedef struct {
    const char *transaction;

    FACET_KEY_OPTIONS default_facet; // the option to be used for internal fields.
                                     // when the requests set facets, we disable all default facets,
                                     // so that the UI has full control over them.

    bool fields_are_ids;             // POST works with field names, GET works with field hashes (IDs)
    bool info;                       // the request is an INFO request, do not execute a query.

    bool data_only;                  // return as fast as possible, with the requested amount of data,
                                     // without scanning the entire duration.

    bool slice;                      // apply native backend filters to slice the events database.
    bool delta;                      // return incremental data for the histogram (used with data_only)
    bool tail;                       // return NOT MODIFIED if no more data are available after the anchor given.

    time_t after_s;                  // the starting timestamp of the query
    time_t before_s;                 // the ending timestamp of the query
    usec_t after_ut;                 // in microseconds
    usec_t before_ut;                // in microseconds

    usec_t anchor;                   // the anchor to seek to
    FACETS_ANCHOR_DIRECTION direction; // the direction based on the anchor (or the query timeframe)

    usec_t if_modified_since;        // the timestamp to check with tail == true

    size_t entries;                  // the number of log events to return in a single response

    const char *query;               // full text search query string
    const char *histogram;           // the field to use for the histogram

    SIMPLE_PATTERN *sources;         // custom log sources to query
    LQS_SOURCE_TYPE source_type;     // pre-defined log sources to query

    size_t filters;                  // the number of filters (facets selected) in the query
    size_t sampling;                 // the number of log events to sample, when the query is too big

    time_t now_s;                    // the timestamp the query was received
    time_t expires_s;                // the timestamp the response expires
} LOGS_QUERY_REQUEST;

#define LOGS_QUERY_REQUEST_DEFAULTS(function_transaction, default_slice, default_direction) \
    (LOGS_QUERY_REQUEST) {                                              \
    .transaction = (function_transaction),                              \
    .default_facet = FACET_KEY_OPTION_FACET,                            \
    .info = false,                                                      \
    .data_only = false,                                                 \
    .slice = (default_slice),                                           \
    .delta = false,                                                     \
    .tail = false,                                                      \
    .after_s = 0,                                                       \
    .before_s = 0,                                                      \
    .anchor = 0,                                                        \
    .if_modified_since = 0,                                             \
    .entries = 0,                                                       \
    .direction = (default_direction),                                   \
    .query = NULL,                                                      \
    .histogram = NULL,                                                  \
    .sources = NULL,                                                    \
    .source_type = LQS_SOURCE_TYPE_ALL,                                 \
    .filters = 0,                                                       \
    .sampling = LQS_DEFAULT_ITEMS_SAMPLING,                             \
}

typedef struct {
    FACETS *facets;

    LOGS_QUERY_REQUEST rq;

    bool *cancelled; // a pointer to the cancelling boolean
    usec_t *stop_monotonic_ut;

    struct {
        usec_t start_ut;
        usec_t stop_ut;
        usec_t delta_ut;
    } anchor;

    struct {
        usec_t start_ut;
        usec_t stop_ut;
        bool stop_when_full;
    } query;

    usec_t last_modified;

    struct lqs_extension c;
} LOGS_QUERY_STATUS;

struct logs_query_data {
    const char *transaction;
    FACETS *facets;
    LOGS_QUERY_REQUEST *rq;
    BUFFER *wb;
};

static inline FACETS_ANCHOR_DIRECTION lgs_get_direction(const char *value) {
    return strcasecmp(value, "forward") == 0 ? FACETS_ANCHOR_DIRECTION_FORWARD : FACETS_ANCHOR_DIRECTION_BACKWARD;
}

static inline void lqs_log_error(LOGS_QUERY_STATUS *lqs, const char *msg) {
    nd_log(NDLS_COLLECTORS, NDLP_ERR,
           "LOGS QUERY ERROR: %s, on query "
           "timeframe [%"PRIu64" - %"PRIu64"], "
           "anchor [%"PRIu64" - %"PRIu64"], "
           "if_modified_since %"PRIu64", "
           "data_only:%s, delta:%s, tail:%s, direction:%s"
           , msg
           , lqs->rq.after_ut
           , lqs->rq.before_ut
           , lqs->anchor.start_ut
           , lqs->anchor.stop_ut
           , lqs->rq.if_modified_since
           , lqs->rq.data_only ? "true" : "false"
           , lqs->rq.delta ? "true" : "false"
           , lqs->rq.tail ? "tail" : "false"
           , lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
}

static inline void lqs_query_timeframe(LOGS_QUERY_STATUS *lqs, usec_t anchor_delta_ut) {
    lqs->anchor.delta_ut = anchor_delta_ut;

    if(lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD) {
        lqs->query.start_ut = (lqs->rq.data_only && lqs->anchor.start_ut) ? lqs->anchor.start_ut : lqs->rq.after_ut;
        lqs->query.stop_ut = ((lqs->rq.data_only && lqs->anchor.stop_ut) ? lqs->anchor.stop_ut : lqs->rq.before_ut) + lqs->anchor.delta_ut;
    }
    else {
        lqs->query.start_ut = ((lqs->rq.data_only && lqs->anchor.start_ut) ? lqs->anchor.start_ut : lqs->rq.before_ut) + lqs->anchor.delta_ut;
        lqs->query.stop_ut = (lqs->rq.data_only && lqs->anchor.stop_ut) ? lqs->anchor.stop_ut : lqs->rq.after_ut;
    }

    lqs->query.stop_when_full = (lqs->rq.data_only && !lqs->anchor.stop_ut);
}

static inline void lqs_function_help(LOGS_QUERY_STATUS *lqs, BUFFER *wb) {
    buffer_reset(wb);
    wb->content_type = CT_TEXT_PLAIN;
    wb->response_code = HTTP_RESP_OK;

    buffer_sprintf(wb,
                   "%s / %s\n"
                   "\n"
                   "%s\n"
                   "\n"
                   "The following parameters are supported:\n"
                   "\n"
                   , program_name
                   , LQS_FUNCTION_NAME
                   , LQS_FUNCTION_DESCRIPTION
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_HELP "\n"
                   "      Shows this help message.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_INFO "\n"
                   "      Request initial configuration information about the plugin.\n"
                   "      The key entity returned is the required_params array, which includes\n"
                   "      all the available log sources.\n"
                   "      When `" LQS_PARAMETER_INFO "` is requested, all other parameters are ignored.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_DATA_ONLY ":true or " LQS_PARAMETER_DATA_ONLY ":false\n"
                   "      Quickly respond with data requested, without generating a\n"
                   "      `histogram`, `facets` counters and `items`.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_DELTA ":true or " LQS_PARAMETER_DELTA ":false\n"
                   "      When doing data only queries, include deltas for histogram, facets and items.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_TAIL ":true or " LQS_PARAMETER_TAIL ":false\n"
                   "      When doing data only queries, respond with the newest messages,\n"
                   "      and up to the anchor, but calculate deltas (if requested) for\n"
                   "      the duration [anchor - before].\n"
                   "\n"
                   );

#ifdef LQS_SLICE_PARAMETER
    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_SLICE ":true or " LQS_PARAMETER_SLICE ":false\n"
                   "      When it is turned on, the plugin is is slicing the logs database,\n"
                   "      utilizing the underlying available indexes.\n"
                   "      When it is off, all filtering is done by the plugin.\n"
                   "      The default is: %s\n"
                   "\n"
                   , lqs->rq.slice ? "true" : "false"
                   );
#endif
    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_SOURCE ":SOURCE\n"
                   "      Query only the specified log sources.\n"
                   "      Do an `" LQS_PARAMETER_INFO "` query to find the sources.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_BEFORE ":TIMESTAMP_IN_SECONDS\n"
                   "      Absolute or relative (to now) timestamp in seconds, to start the query.\n"
                   "      The query is always executed from the most recent to the oldest log entry.\n"
                   "      If not given the default is: now.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_AFTER ":TIMESTAMP_IN_SECONDS\n"
                   "      Absolute or relative (to `before`) timestamp in seconds, to end the query.\n"
                   "      If not given, the default is %d.\n"
                   "\n"
                   , -LQS_DEFAULT_QUERY_DURATION
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_LAST ":ITEMS\n"
                   "      The number of items to return.\n"
                   "      The default is %zu.\n"
                   "\n"
                   , lqs->rq.entries
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_SAMPLING ":ITEMS\n"
                   "      The number of log entries to sample to estimate facets counters and histogram.\n"
                   "      The default is %zu.\n"
                   "\n"
                   , lqs->rq.sampling
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_ANCHOR ":TIMESTAMP_IN_MICROSECONDS\n"
                   "      Return items relative to this timestamp.\n"
                   "      The exact items to be returned depend on the query `" LQS_PARAMETER_DIRECTION "`.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_DIRECTION ":forward or " LQS_PARAMETER_DIRECTION ":backward\n"
                   "      When set to `backward` (default) the items returned are the newest before the\n"
                   "      `" LQS_PARAMETER_ANCHOR "`, (or `" LQS_PARAMETER_BEFORE "` if `" LQS_PARAMETER_ANCHOR "` is not set)\n"
                   "      When set to `forward` the items returned are the oldest after the\n"
                   "      `" LQS_PARAMETER_ANCHOR "`, (or `" LQS_PARAMETER_AFTER "` if `" LQS_PARAMETER_ANCHOR "` is not set)\n"
                   "      The default is: %s\n"
                   "\n"
                   , lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_QUERY ":SIMPLE_PATTERN\n"
                   "      Do a full text search to find the log entries matching the pattern given.\n"
                   "      The plugin is searching for matches on all fields of the database.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_IF_MODIFIED_SINCE ":TIMESTAMP_IN_MICROSECONDS\n"
                   "      Each successful response, includes a `last_modified` field.\n"
                   "      By providing the timestamp to the `" LQS_PARAMETER_IF_MODIFIED_SINCE "` parameter,\n"
                   "      the plugin will return 200 with a successful response, or 304 if the source has not\n"
                   "      been modified since that timestamp.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_HISTOGRAM ":facet_id\n"
                   "      Use the given `facet_id` for the histogram.\n"
                   "      This parameter is ignored in `" LQS_PARAMETER_DATA_ONLY "` mode.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   " LQS_PARAMETER_FACETS ":facet_id1,facet_id2,facet_id3,...\n"
                   "      Add the given facets to the list of fields for which analysis is required.\n"
                   "      The plugin will offer both a histogram and facet value counters for its values.\n"
                   "      This parameter is ignored in `" LQS_PARAMETER_DATA_ONLY "` mode.\n"
                   "\n"
                   );

    buffer_sprintf(wb,
                   "   facet_id:value_id1,value_id2,value_id3,...\n"
                   "      Apply filters to the query, based on the facet IDs returned.\n"
                   "      Each `facet_id` can be given once, but multiple `facet_ids` can be given.\n"
                   "\n"
                   );
}

static inline bool lqs_request_parse_json_payload(json_object *jobj, void *data, BUFFER *error) {
    const char *path = "";
    struct logs_query_data *qd = data;
    LOGS_QUERY_REQUEST *rq = qd->rq;
    BUFFER *wb = qd->wb;
    FACETS *facets = qd->facets;
    // const char *transaction = qd->transaction;

    buffer_flush(error);

    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_INFO, rq->info, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_DELTA, rq->delta, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_TAIL, rq->tail, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_SLICE, rq->slice, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_DATA_ONLY, rq->data_only, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_SAMPLING, rq->sampling, error, false);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_AFTER, rq->after_s, error, false);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_BEFORE, rq->before_s, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_IF_MODIFIED_SINCE, rq->if_modified_since, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_ANCHOR, rq->anchor, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_LAST, rq->entries, error, false);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_DIRECTION, lgs_get_direction, rq->direction, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_QUERY, rq->query, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, LQS_PARAMETER_HISTOGRAM, rq->histogram, error, false);

    json_object *fcts;
    if (json_object_object_get_ex(jobj, LQS_PARAMETER_FACETS, &fcts)) {
        if (json_object_get_type(fcts) != json_type_array) {
            buffer_sprintf(error, "member '%s' is not an array.", LQS_PARAMETER_FACETS);
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "POST payload: '%s' is not an array", LQS_PARAMETER_FACETS);
            return false;
        }

        rq->default_facet = FACET_KEY_OPTION_NONE;
        facets_reset_and_disable_all_facets(facets);

        buffer_json_member_add_array(wb, LQS_PARAMETER_FACETS);

        size_t facets_len = json_object_array_length(fcts);
        for (size_t i = 0; i < facets_len; i++) {
            json_object *fct = json_object_array_get_idx(fcts, i);

            if (json_object_get_type(fct) != json_type_string) {
                buffer_sprintf(error, "facets array item %zu is not a string", i);
                // nd_log(NDLS_COLLECTORS, NDLP_ERR, "POST payload: facets array item %zu is not a string", i);
                return false;
            }

            const char *value = json_object_get_string(fct);
            facets_register_facet(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
            buffer_json_add_array_item_string(wb, value);
        }

        buffer_json_array_close(wb); // facets
    }

    json_object *selections;
    if (json_object_object_get_ex(jobj, "selections", &selections)) {
        if (json_object_get_type(selections) != json_type_object) {
            buffer_sprintf(error, "member 'selections' is not an object");
            // nd_log(NDLS_COLLECTORS, NDLP_ERR, "POST payload: '%s' is not an object", "selections");
            return false;
        }

        buffer_json_member_add_object(wb, "selections");

        CLEAN_BUFFER *sources_list = buffer_create(0, NULL);

        json_object_object_foreach(selections, key, val) {
            if(strcmp(key, "query") == 0) continue;

            if (json_object_get_type(val) != json_type_array) {
                buffer_sprintf(error, "selection '%s' is not an array", key);
                // nd_log(NDLS_COLLECTORS, NDLP_ERR, "POST payload: selection '%s' is not an array", key);
                return false;
            }

            bool is_source = false;
            if(strcmp(key, LQS_PARAMETER_SOURCE) == 0) {
                // reset the sources, so that only what the user selects will be shown
                is_source = true;
                rq->source_type = LQS_SOURCE_TYPE_NONE;
            }

            buffer_json_member_add_array(wb, key);

            size_t values_len = json_object_array_length(val);
            for (size_t i = 0; i < values_len; i++) {
                json_object *value_obj = json_object_array_get_idx(val, i);

                if (json_object_get_type(value_obj) != json_type_string) {
                    buffer_sprintf(error, "selection '%s' array item %zu is not a string", key, i);
                    // nd_log(NDLS_COLLECTORS, NDLP_ERR, "POST payload: selection '%s' array item %zu is not a string", key, i);
                    return false;
                }

                const char *value = json_object_get_string(value_obj);

                if(is_source) {
                    // processing sources
                    LQS_SOURCE_TYPE t = LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value);
                    if(t != LQS_SOURCE_TYPE_NONE) {
                        rq->source_type |= t;
                        value = NULL;
                    }
                    else {
                        // else, match the source, whatever it is
                        if(buffer_strlen(sources_list))
                            buffer_putc(sources_list, '|');

                        buffer_strcat(sources_list, value);
                    }
                }
                else {
                    // Call facets_register_facet_id_filter for each value
                    facets_register_facet_filter(
                        facets, key, value, FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_REORDER);

                    rq->filters++;
                }

                buffer_json_add_array_item_string(wb, value);
            }

            buffer_json_array_close(wb); // key
        }

        if(buffer_strlen(sources_list)) {
            simple_pattern_free(rq->sources);
            rq->sources = simple_pattern_create(buffer_tostring(sources_list), "|", SIMPLE_PATTERN_EXACT, false);
        }

        buffer_json_object_close(wb); // selections
    }

    facets_use_hashes_for_ids(facets, false);
    rq->fields_are_ids = false;
    return true;
}

static inline bool lqs_request_parse_POST(LOGS_QUERY_STATUS *lqs, BUFFER *wb, BUFFER *payload, const char *transaction) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    buffer_json_member_add_object(wb, "_request");

    struct logs_query_data qd = {
        .transaction = transaction,
        .facets = facets,
        .rq = rq,
        .wb = wb,
    };

    int code;
    CLEAN_JSON_OBJECT *jobj =
        json_parse_function_payload_or_error(wb, payload, &code, lqs_request_parse_json_payload, &qd);
    wb->response_code = code;

    return (jobj && code == HTTP_RESP_OK);
}

static inline bool lqs_request_parse_GET(LOGS_QUERY_STATUS *lqs, BUFFER *wb, char *function) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    buffer_json_member_add_object(wb, "_request");

    char func_copy[strlen(function) + 1];
    memcpy(func_copy, function, sizeof(func_copy));

    char *words[LQS_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_whitespace(func_copy, words, LQS_MAX_PARAMS);
    for(int i = 1; i < LQS_MAX_PARAMS;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, LQS_PARAMETER_HELP) == 0) {
            lqs_function_help(lqs, wb);
            return false;
        }
        else if(strcmp(keyword, LQS_PARAMETER_INFO) == 0) {
            rq->info = true;
        }
        else if(strncmp(keyword, LQS_PARAMETER_DELTA ":", sizeof(LQS_PARAMETER_DELTA ":") - 1) == 0) {
            char *v = &keyword[sizeof(LQS_PARAMETER_DELTA ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->delta = false;
            else
                rq->delta = true;
        }
        else if(strncmp(keyword, LQS_PARAMETER_TAIL ":", sizeof(LQS_PARAMETER_TAIL ":") - 1) == 0) {
            char *v = &keyword[sizeof(LQS_PARAMETER_TAIL ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->tail = false;
            else
                rq->tail = true;
        }
        else if(strncmp(keyword, LQS_PARAMETER_SAMPLING ":", sizeof(LQS_PARAMETER_SAMPLING ":") - 1) == 0) {
            rq->sampling = str2ul(&keyword[sizeof(LQS_PARAMETER_SAMPLING ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_DATA_ONLY ":", sizeof(LQS_PARAMETER_DATA_ONLY ":") - 1) == 0) {
            char *v = &keyword[sizeof(LQS_PARAMETER_DATA_ONLY ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->data_only = false;
            else
                rq->data_only = true;
        }
        else if(strncmp(keyword, LQS_PARAMETER_SLICE ":", sizeof(LQS_PARAMETER_SLICE ":") - 1) == 0) {
            char *v = &keyword[sizeof(LQS_PARAMETER_SLICE ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->slice = false;
            else
                rq->slice = true;
        }
        else if(strncmp(keyword, LQS_PARAMETER_SOURCE ":", sizeof(LQS_PARAMETER_SOURCE ":") - 1) == 0) {
            const char *value = &keyword[sizeof(LQS_PARAMETER_SOURCE ":") - 1];

            buffer_json_member_add_array(wb, LQS_PARAMETER_SOURCE);

            CLEAN_BUFFER *sources_list = buffer_create(0, NULL);

            rq->source_type = LQS_SOURCE_TYPE_NONE;
            while(value) {
                char *sep = strchr(value, ',');
                if(sep)
                    *sep++ = '\0';

                buffer_json_add_array_item_string(wb, value);

                LQS_SOURCE_TYPE t = LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value);
                if(t != LQS_SOURCE_TYPE_NONE) {
                    rq->source_type |= t;
                }
                else {
                    // else, match the source, whatever it is
                    if(buffer_strlen(sources_list))
                        buffer_putc(sources_list, '|');

                    buffer_strcat(sources_list, value);
                }

                value = sep;
            }

            if(buffer_strlen(sources_list)) {
                simple_pattern_free(rq->sources);
                rq->sources = simple_pattern_create(buffer_tostring(sources_list), "|", SIMPLE_PATTERN_EXACT, false);
            }

            buffer_json_array_close(wb); // source
        }
        else if(strncmp(keyword, LQS_PARAMETER_AFTER ":", sizeof(LQS_PARAMETER_AFTER ":") - 1) == 0) {
            rq->after_s = str2l(&keyword[sizeof(LQS_PARAMETER_AFTER ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_BEFORE ":", sizeof(LQS_PARAMETER_BEFORE ":") - 1) == 0) {
            rq->before_s = str2l(&keyword[sizeof(LQS_PARAMETER_BEFORE ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_IF_MODIFIED_SINCE ":", sizeof(LQS_PARAMETER_IF_MODIFIED_SINCE ":") - 1) == 0) {
            rq->if_modified_since = str2ull(&keyword[sizeof(LQS_PARAMETER_IF_MODIFIED_SINCE ":") - 1], NULL);
        }
        else if(strncmp(keyword, LQS_PARAMETER_ANCHOR ":", sizeof(LQS_PARAMETER_ANCHOR ":") - 1) == 0) {
            rq->anchor = str2ull(&keyword[sizeof(LQS_PARAMETER_ANCHOR ":") - 1], NULL);
        }
        else if(strncmp(keyword, LQS_PARAMETER_DIRECTION ":", sizeof(LQS_PARAMETER_DIRECTION ":") - 1) == 0) {
            rq->direction = lgs_get_direction(&keyword[sizeof(LQS_PARAMETER_DIRECTION ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_LAST ":", sizeof(LQS_PARAMETER_LAST ":") - 1) == 0) {
            rq->entries = str2ul(&keyword[sizeof(LQS_PARAMETER_LAST ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_QUERY ":", sizeof(LQS_PARAMETER_QUERY ":") - 1) == 0) {
            freez((void *)rq->query);
            rq->query= strdupz(&keyword[sizeof(LQS_PARAMETER_QUERY ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_HISTOGRAM ":", sizeof(LQS_PARAMETER_HISTOGRAM ":") - 1) == 0) {
            freez((void *)rq->histogram);
            rq->histogram = strdupz(&keyword[sizeof(LQS_PARAMETER_HISTOGRAM ":") - 1]);
        }
        else if(strncmp(keyword, LQS_PARAMETER_FACETS ":", sizeof(LQS_PARAMETER_FACETS ":") - 1) == 0) {
            rq->default_facet = FACET_KEY_OPTION_NONE;
            facets_reset_and_disable_all_facets(facets);

            char *value = &keyword[sizeof(LQS_PARAMETER_FACETS ":") - 1];
            if(*value) {
                buffer_json_member_add_array(wb, LQS_PARAMETER_FACETS);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // facets
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

                    facets_register_facet_filter_id(
                        facets, keyword, value,
                        FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_REORDER);

                    buffer_json_add_array_item_string(wb, value);
                    rq->filters++;

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
    }

    facets_use_hashes_for_ids(facets, true);
    rq->fields_are_ids = true;
    return true;
}

static inline void lqs_info_response(BUFFER *wb, FACETS *facets) {
    // the buffer already has the request in it
    // DO NOT FLUSH IT

    buffer_json_member_add_uint64(wb, "v", 3);
    facets_accepted_parameters_to_json_array(facets, wb, false);
    buffer_json_member_add_array(wb, "required_params");
    {
        buffer_json_add_array_item_object(wb);
        {
            buffer_json_member_add_string(wb, "id", LQS_PARAMETER_SOURCE);
            buffer_json_member_add_string(wb, "name", LQS_PARAMETER_SOURCE_NAME);
            buffer_json_member_add_string(wb, "help", "Select the logs source to query");
            buffer_json_member_add_string(wb, "type", "multiselect");
            buffer_json_member_add_array(wb, "options");
            {
                LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb);
            }
            buffer_json_array_close(wb); // options array
        }
        buffer_json_object_close(wb); // required params object
    }
    buffer_json_array_close(wb); // required_params array

    facets_table_config(facets, wb);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_string(wb, "help", LQS_FUNCTION_DESCRIPTION);
    buffer_json_finalize(wb);

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
}

static inline BUFFER *lqs_create_output_buffer(void) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_reset(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    return wb;
}

static inline FACETS *lqs_facets_create(uint32_t items_to_return, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys, bool have_slice) {
    FACETS *facets = facets_create(items_to_return, options,
                                   visible_keys, facet_keys, non_facet_keys);

    facets_accepted_param(facets, LQS_PARAMETER_INFO);
    facets_accepted_param(facets, LQS_PARAMETER_SOURCE);
    facets_accepted_param(facets, LQS_PARAMETER_AFTER);
    facets_accepted_param(facets, LQS_PARAMETER_BEFORE);
    facets_accepted_param(facets, LQS_PARAMETER_ANCHOR);
    facets_accepted_param(facets, LQS_PARAMETER_DIRECTION);
    facets_accepted_param(facets, LQS_PARAMETER_LAST);
    facets_accepted_param(facets, LQS_PARAMETER_QUERY);
    facets_accepted_param(facets, LQS_PARAMETER_FACETS);
    facets_accepted_param(facets, LQS_PARAMETER_HISTOGRAM);
    facets_accepted_param(facets, LQS_PARAMETER_IF_MODIFIED_SINCE);
    facets_accepted_param(facets, LQS_PARAMETER_DATA_ONLY);
    facets_accepted_param(facets, LQS_PARAMETER_DELTA);
    facets_accepted_param(facets, LQS_PARAMETER_TAIL);
    facets_accepted_param(facets, LQS_PARAMETER_SAMPLING);

    if(have_slice)
        facets_accepted_param(facets, LQS_PARAMETER_SLICE);

    return facets;
}

static inline bool lqs_request_parse_and_validate(LOGS_QUERY_STATUS *lqs, BUFFER *wb, char *function, BUFFER *payload, bool have_slice, const char *default_histogram) {
    LOGS_QUERY_REQUEST *rq = &lqs->rq;
    FACETS *facets = lqs->facets;

    if( (payload && !lqs_request_parse_POST(lqs, wb, payload, rq->transaction)) ||
        (!payload && !lqs_request_parse_GET(lqs, wb, function)) )
        return false;

    // ----------------------------------------------------------------------------------------------------------------
    // validate parameters

    if(rq->query && !*rq->query) {
        freez((void *)rq->query);
        rq->query = NULL;
    }

    if(rq->histogram && !*rq->histogram) {
        freez((void *)rq->histogram);
        rq->histogram = NULL;
    }

    if(!rq->data_only)
        rq->delta = false;

    if(!rq->data_only || !rq->if_modified_since)
        rq->tail = false;

    rq->now_s = now_realtime_sec();
    rq->expires_s = rq->now_s + 1;
    wb->expires = rq->expires_s;

    if(!rq->after_s && !rq->before_s) {
        rq->before_s = rq->now_s;
        rq->after_s = rq->before_s - LQS_DEFAULT_QUERY_DURATION;
    }
    else
        rrdr_relative_window_to_absolute(&rq->after_s, &rq->before_s, rq->now_s);

    if(rq->after_s > rq->before_s) {
        time_t tmp = rq->after_s;
        rq->after_s = rq->before_s;
        rq->before_s = tmp;
    }

    if(rq->after_s == rq->before_s)
        rq->after_s = rq->before_s - LQS_DEFAULT_QUERY_DURATION;

    rq->after_ut = rq->after_s * USEC_PER_SEC;
    rq->before_ut = (rq->before_s * USEC_PER_SEC) + USEC_PER_SEC - 1;

    if(!rq->entries)
        rq->entries = LQS_DEFAULT_ITEMS_PER_QUERY;

    // ----------------------------------------------------------------------------------------------------------------
    // validate the anchor

    lqs->last_modified = 0;
    lqs->anchor.start_ut = lqs->rq.anchor;
    lqs->anchor.stop_ut = 0;

    if(lqs->anchor.start_ut && lqs->rq.tail) {
        // a tail request
        // we need the top X entries from BEFORE
        // but, we need to calculate the facets and the
        // histogram up to the anchor
        lqs->rq.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
        lqs->anchor.start_ut = 0;
        lqs->anchor.stop_ut = lqs->rq.anchor;
    }

    if(lqs->rq.anchor && lqs->rq.anchor < lqs->rq.after_ut) {
        lqs_log_error(lqs, "received anchor is too small for query timeframe, ignoring anchor");
        lqs->rq.anchor = 0;
        lqs->anchor.start_ut = 0;
        lqs->anchor.stop_ut = 0;
        lqs->rq.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }
    else if(lqs->rq.anchor > lqs->rq.before_ut) {
        lqs_log_error(lqs, "received anchor is too big for query timeframe, ignoring anchor");
        lqs->rq.anchor = 0;
        lqs->anchor.start_ut = 0;
        lqs->anchor.stop_ut = 0;
        lqs->rq.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }

    facets_set_anchor(facets, lqs->anchor.start_ut, lqs->anchor.stop_ut, lqs->rq.direction);

    facets_set_additional_options(facets,
                                  ((lqs->rq.data_only) ? FACETS_OPTION_DATA_ONLY : 0) |
                                      ((lqs->rq.delta) ? FACETS_OPTION_SHOW_DELTAS : 0));

    facets_set_items(facets, lqs->rq.entries);
    facets_set_query(facets, lqs->rq.query);

    if(lqs->rq.slice && have_slice)
        facets_enable_slice_mode(facets);
    else
        lqs->rq.slice = false;

    if(lqs->rq.histogram) {
        if(lqs->rq.fields_are_ids)
            facets_set_timeframe_and_histogram_by_id(facets, lqs->rq.histogram, lqs->rq.after_ut, lqs->rq.before_ut);
        else
            facets_set_timeframe_and_histogram_by_name(facets, lqs->rq.histogram, lqs->rq.after_ut, lqs->rq.before_ut);
    }
    else if(default_histogram)
        facets_set_timeframe_and_histogram_by_name(facets, default_histogram, lqs->rq.after_ut, lqs->rq.before_ut);

    // complete the request object
    buffer_json_member_add_boolean(wb, LQS_PARAMETER_INFO, lqs->rq.info);
    buffer_json_member_add_boolean(wb, LQS_PARAMETER_SLICE, lqs->rq.slice);
    buffer_json_member_add_boolean(wb, LQS_PARAMETER_DATA_ONLY, lqs->rq.data_only);
    buffer_json_member_add_boolean(wb, LQS_PARAMETER_DELTA, lqs->rq.delta);
    buffer_json_member_add_boolean(wb, LQS_PARAMETER_TAIL, lqs->rq.tail);
    buffer_json_member_add_uint64(wb, LQS_PARAMETER_SAMPLING, lqs->rq.sampling);
    buffer_json_member_add_uint64(wb, "source_type", lqs->rq.source_type);
    buffer_json_member_add_uint64(wb, LQS_PARAMETER_AFTER, lqs->rq.after_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, LQS_PARAMETER_BEFORE, lqs->rq.before_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, "if_modified_since", lqs->rq.if_modified_since);
    buffer_json_member_add_uint64(wb, LQS_PARAMETER_ANCHOR, lqs->rq.anchor);
    buffer_json_member_add_string(wb, LQS_PARAMETER_DIRECTION, lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, LQS_PARAMETER_LAST, lqs->rq.entries);
    buffer_json_member_add_string(wb, LQS_PARAMETER_QUERY, lqs->rq.query);
    buffer_json_member_add_string(wb, LQS_PARAMETER_HISTOGRAM, lqs->rq.histogram);
    buffer_json_object_close(wb); // request

    return true;
}

static inline void lqs_cleanup(LOGS_QUERY_STATUS *lqs) {
    freez((void *)lqs->rq.query);
    freez((void *)lqs->rq.histogram);
    simple_pattern_free(lqs->rq.sources);
    facets_destroy(lqs->facets);
}

#endif //NETDATA_LOGS_QUERY_STATUS_H
