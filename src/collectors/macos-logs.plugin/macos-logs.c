// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "macos-logs.h"

netdata_mutex_t stdout_mutex;
DICTIONARY *used_hashes_registry = NULL;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}

static bool plugin_should_exit = false;

#define MACOS_LOGS_ALWAYS_VISIBLE_KEYS NULL

#define MACOS_LOGS_KEYS_EXCLUDED_FROM_FACETS \
    "|" MACOS_LOGS_FIELD_MESSAGE             \
    ""

#define MACOS_LOGS_KEYS_INCLUDED_IN_FACETS  \
    "|" MACOS_LOGS_FIELD_LEVEL              \
    "|" MACOS_LOGS_FIELD_PROCESS            \
    "|" MACOS_LOGS_FIELD_SENDER             \
    "|" MACOS_LOGS_FIELD_SUBSYSTEM          \
    "|" MACOS_LOGS_FIELD_CATEGORY           \
    "|" MACOS_LOGS_FIELD_ENTRY_TYPE         \
    "|" MACOS_LOGS_FIELD_STORE_CATEGORY     \
    ""

static FACET_ROW_SEVERITY macos_logs_level_to_facet_severity(
    FACETS *facets __maybe_unused, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *level_rkv = dictionary_get(row->dict, MACOS_LOGS_FIELD_LEVEL_ID);
    if(!level_rkv || level_rkv->empty)
        return FACET_ROW_SEVERITY_NORMAL;

    int level = str2i(buffer_tostring(level_rkv->wb));
    switch(level) {
        case 1: // debug
            return FACET_ROW_SEVERITY_DEBUG;
        case 3: // notice
            return FACET_ROW_SEVERITY_NOTICE;
        case 4: // error
        case 5: // fault
            return FACET_ROW_SEVERITY_CRITICAL;
        case 0: // undefined
        case 2: // info
        default:
            return FACET_ROW_SEVERITY_NORMAL;
    }
}

static void macos_logs_register_fields(LOGS_QUERY_STATUS *lqs) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    facets_register_row_severity(facets, macos_logs_level_to_facet_severity, NULL);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_MESSAGE,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_LEVEL,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_EXPANDED_FILTER | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(facets, MACOS_LOGS_FIELD_LEVEL_ID, FACET_KEY_OPTION_HIDDEN);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_PROCESS,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(facets, MACOS_LOGS_FIELD_PID, FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_SENDER,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_SUBSYSTEM,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_CATEGORY,
        rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_ENTRY_TYPE,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets,
        MACOS_LOGS_FIELD_STORE_CATEGORY,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(facets, MACOS_LOGS_FIELD_THREAD_ID, FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, MACOS_LOGS_FIELD_ACTIVITY_ID, FACET_KEY_OPTION_FTS);
}

static void buffer_json_macos_logs_versions(BUFFER *wb) {
    buffer_json_member_add_object(wb, "versions");
    {
        buffer_json_member_add_uint64(wb, "sources", 1);
    }
    buffer_json_object_close(wb);
}

static int macos_logs_response(BUFFER *wb, LOGS_QUERY_STATUS *lqs, MACOS_LOGS_QUERY_STATUS status) {
    bool partial = status == MACOS_LOGS_QUERY_TIMED_OUT || status == MACOS_LOGS_QUERY_SCAN_LIMIT_REACHED;

    switch(status) {
        case MACOS_LOGS_QUERY_OK:
            if(lqs->rq.if_modified_since && !lqs->c.rows_useful)
                return rrd_call_function_error(wb, "no useful logs, not modified", HTTP_RESP_NOT_MODIFIED);
            break;

        case MACOS_LOGS_QUERY_TIMED_OUT:
        case MACOS_LOGS_QUERY_SCAN_LIMIT_REACHED:
            break;

        case MACOS_LOGS_QUERY_CANCELLED:
            return rrd_call_function_error(wb, "client closed connection", HTTP_RESP_CLIENT_CLOSED_REQUEST);

        case MACOS_LOGS_QUERY_OPEN_FAILED:
            return rrd_call_function_error(wb, "failed to open macOS unified log store", HTTP_RESP_INTERNAL_SERVER_ERROR);

        case MACOS_LOGS_QUERY_ENUMERATOR_FAILED:
            return rrd_call_function_error(wb, "failed to enumerate macOS unified logs", HTTP_RESP_INTERNAL_SERVER_ERROR);

        default:
            return rrd_call_function_error(wb, "unknown macOS unified log query status", HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", partial);
    buffer_json_member_add_string(wb, "type", "table");

    if(!lqs->rq.data_only) {
        CLEAN_BUFFER *msg = buffer_create(0, NULL);
        CLEAN_BUFFER *msg_description = buffer_create(0, NULL);
        ND_LOG_FIELD_PRIORITY msg_priority = NDLP_INFO;

        if(status == MACOS_LOGS_QUERY_TIMED_OUT) {
            buffer_strcat(msg, "Query timed out, incomplete data. ");
            buffer_strcat(msg_description, "QUERY TIMEOUT: The query timed out and may not include all data in the selected window. ");
            msg_priority = NDLP_WARNING;
        }
        else if(status == MACOS_LOGS_QUERY_SCAN_LIMIT_REACHED) {
            buffer_strcat(msg, "Query scan limit reached, incomplete data. ");
            buffer_sprintf(
                msg_description,
                "SCAN LIMIT: The query reached the safety scan limit of %zu log entries and may not include all data in the selected window. ",
                lqs->c.rows_scanned_limit);
            msg_priority = NDLP_WARNING;
        }

        buffer_json_member_add_object(wb, "message");
        if(buffer_tostring(msg)) {
            buffer_json_member_add_string(wb, "title", buffer_tostring(msg));
            buffer_json_member_add_string(wb, "description", buffer_tostring(msg_description));
            buffer_json_member_add_string(wb, "status", nd_log_id2priority(msg_priority));
        }
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_array(wb, "_sources");
    {
        buffer_json_add_array_item_object(wb);
        {
            usec_t duration_ut = lqs->c.query_finished_ut > lqs->c.query_started_ut ?
                                     lqs->c.query_finished_ut - lqs->c.query_started_ut : 0;

            buffer_json_member_add_string(wb, "_name", "macOS unified log");
            buffer_json_member_add_uint64(wb, "_source_type", MACOS_LOGS_SOURCE_ALL);
            buffer_json_member_add_string(wb, "_source", "all");
            buffer_json_member_add_uint64(wb, "duration_ut", duration_ut);
            buffer_json_member_add_uint64(wb, "rows_read", lqs->c.rows_read);
            buffer_json_member_add_uint64(wb, "rows_useful", lqs->c.rows_useful);
            buffer_json_member_add_double(
                wb, "rows_per_second",
                duration_ut ? (double)lqs->c.rows_read / (double)duration_ut * (double)USEC_PER_SEC : 0.0);
            buffer_json_member_add_uint64(wb, "bytes_read", lqs->c.bytes_read);
            buffer_json_member_add_double(
                wb, "bytes_per_second",
                duration_ut ? (double)lqs->c.bytes_read / (double)duration_ut * (double)USEC_PER_SEC : 0.0);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb);

    if(!lqs->rq.data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", MACOS_LOGS_FUNCTION_DESCRIPTION);
    }

    if(!lqs->rq.data_only || lqs->rq.tail)
        buffer_json_member_add_uint64(wb, "last_modified", lqs->last_modified);

    facets_sort_and_reorder_keys(lqs->facets);
    facets_report(lqs->facets, wb, used_hashes_registry);

    wb->expires = now_realtime_sec() + (lqs->rq.data_only ? 3600 : 0);
    buffer_json_member_add_time_t(wb, "expires", wb->expires);

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
    return wb->response_code;
}

void function_macos_logs(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                         BUFFER *payload, HTTP_ACCESS access __maybe_unused,
                         const char *source __maybe_unused, void *data __maybe_unused) {
    LOGS_QUERY_STATUS tmp_lqs = {
        .facets = lqs_facets_create(
            LQS_DEFAULT_ITEMS_PER_QUERY,
            FACETS_OPTION_ALL_KEYS_FTS | FACETS_OPTION_HASH_IDS,
            MACOS_LOGS_ALWAYS_VISIBLE_KEYS,
            MACOS_LOGS_KEYS_INCLUDED_IN_FACETS,
            MACOS_LOGS_KEYS_EXCLUDED_FROM_FACETS,
            LQS_DEFAULT_SLICE_MODE),

        .rq = LOGS_QUERY_REQUEST_DEFAULTS(transaction, LQS_DEFAULT_SLICE_MODE, FACETS_ANCHOR_DIRECTION_BACKWARD),

        .cancelled = cancelled,
        .stop_monotonic_ut = stop_monotonic_ut,
    };
    LOGS_QUERY_STATUS *lqs = &tmp_lqs;

    CLEAN_BUFFER *wb = lqs_create_output_buffer();

    if(lqs_request_parse_and_validate(lqs, wb, function, payload, LQS_DEFAULT_SLICE_MODE, MACOS_LOGS_FIELD_LEVEL)) {
        macos_logs_register_fields(lqs);
        buffer_json_macos_logs_versions(wb);

        if(lqs->rq.info)
            lqs_info_response(wb, lqs->facets);
        else {
            lqs_query_timeframe(lqs, MACOS_LOGS_ANCHOR_DELTA_UT);
            MACOS_LOGS_QUERY_STATUS status = macos_logs_query_oslog(lqs);
            if(macos_logs_response(wb, lqs, status) == HTTP_RESP_OK)
                buffer_json_finalize(wb);
        }
    }

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    lqs_cleanup(lqs);
}

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    nd_thread_tag_set("macos-logs.plugin");
    nd_log_initialize_for_external_plugins("macos-logs.plugin");
    netdata_threads_init_for_external_plugins(0);

    used_hashes_registry = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);

    if(argc >= 2 && strcmp(argv[argc - 1], "debug") == 0) {
        bool cancelled = false;
        usec_t stop_monotonic_ut = now_monotonic_usec() + MACOS_LOGS_DEFAULT_TIMEOUT * USEC_PER_SEC;
        char buf[] = MACOS_LOGS_FUNCTION_NAME " info";
        function_macos_logs("debug-transaction", buf, &stop_monotonic_ut, &cancelled, NULL, HTTP_ACCESS_ALL, NULL, NULL);
        dictionary_destroy(used_hashes_registry);
        return 0;
    }

    struct functions_evloop_globals *wg =
        functions_evloop_init(MACOS_LOGS_WORKER_THREADS, "MLOG", &stdout_mutex, &plugin_should_exit, NULL);

    functions_evloop_add_function(wg, MACOS_LOGS_FUNCTION_NAME, function_macos_logs, MACOS_LOGS_DEFAULT_TIMEOUT, NULL);

    netdata_mutex_lock(&stdout_mutex);
    fprintf(stdout,
            PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"logs\" " HTTP_ACCESS_FORMAT " %d\n",
            MACOS_LOGS_FUNCTION_NAME,
            MACOS_LOGS_DEFAULT_TIMEOUT,
            MACOS_LOGS_FUNCTION_DESCRIPTION,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);
    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    usec_t send_newline_ut = 0;
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while(!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {
        usec_t dt_ut = heartbeat_next(&hb);

        send_newline_ut += dt_ut;
        if(!tty && send_newline_ut >= USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    functions_evloop_cancel_threads(wg);
    dictionary_destroy(used_hashes_registry);
    return 0;
}
