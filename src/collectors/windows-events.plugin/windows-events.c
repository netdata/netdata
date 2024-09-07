// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "windows-events.h"

#define WINDOWS_EVENTS_WORKER_THREADS 5
#define WINDOWS_EVENTS_DEFAULT_TIMEOUT 600
#define WINDOWS_EVENTS_SCAN_EVERY_USEC (5 * 60 * USEC_PER_SEC)
#define WINDOWS_EVENTS_PROGRESS_EVERY_UT (250 * USEC_PER_MS)

netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

#define WEVT_FUNCTION_DESCRIPTION    "View, search and analyze Microsoft Windows events."
#define WEVT_FUNCTION_NAME           "windows-events"

// functions needed by LQS

// structures needed by LQS
struct lqs_extension {
    struct {
        usec_t start_ut;
        usec_t stop_ut;
        usec_t first_msg_ut;

        uint64_t first_msg_seqnum;
    } query_file;

    // struct {
    //     uint32_t enable_after_samples;
    //     uint32_t slots;
    //     uint32_t sampled;
    //     uint32_t unsampled;
    //     uint32_t estimated;
    // } samples;

    // struct {
    //     uint32_t enable_after_samples;
    //     uint32_t every;
    //     uint32_t skipped;
    //     uint32_t recalibrate;
    //     uint32_t sampled;
    //     uint32_t unsampled;
    //     uint32_t estimated;
    // } samples_per_file;

    // struct {
    //     usec_t start_ut;
    //     usec_t end_ut;
    //     usec_t step_ut;
    //     uint32_t enable_after_samples;
    //     uint32_t sampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
    //     uint32_t unsampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
    // } samples_per_time_slot;

    // per file progress info
    // size_t cached_count;

    // progress statistics
    usec_t matches_setup_ut;
    size_t rows_useful;
    size_t rows_read;
    size_t bytes_read;
    size_t files_matched;
    size_t file_working;
};

// prepare LQS
#define LQS_DEFAULT_SLICE_MODE      0
#define LQS_FUNCTION_NAME           WEVT_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION    WEVT_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING  1000000
#define LQS_SOURCE_TYPE             WEVT_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL         WEVTS_ALL
#define LQS_SOURCE_TYPE_NONE        WEVTS_NONE
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) wevt_internal_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) wevt_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

#define WEVT_ALWAYS_VISIBLE_KEYS                NULL

#define WEVT_KEYS_EXCLUDED_FROM_FACETS          \
    "|*Message*"                                \
    "|*ID"                                      \
    ""

#define WEVT_KEYS_INCLUDED_IN_FACETS            \
    "|Provider"                                 \
    "|Source"                                   \
    "|Level"                                    \
    "|Computer"                                 \
    ""

static void wevt_register_transformations(LOGS_QUERY_STATUS *lqs __maybe_unused) {
    ;
}

WEVT_QUERY_STATUS wevt_query_forward(
        WEVT_LOG *j, BUFFER *wb __maybe_unused, FACETS *facets,
        LOGS_QUERY_SOURCE *jf,
    LOGS_QUERY_STATUS *fqs)
{
    return WEVT_FAILED_TO_OPEN;
}

WEVT_QUERY_STATUS wevt_query_backward(
        WEVT_LOG *j, BUFFER *wb __maybe_unused, FACETS *facets,
        LOGS_QUERY_SOURCE *jf,
    LOGS_QUERY_STATUS *fqs)
{
    return WEVT_FAILED_TO_OPEN;
}

static WEVT_QUERY_STATUS wevt_query_one_channel(
        const char *fullname, BUFFER *wb, FACETS *facets,
        LOGS_QUERY_SOURCE *jf,
    LOGS_QUERY_STATUS *fqs) {

    errno_clear();

    WEVT_LOG *log = wevt_openlog6(channel2unicode(fullname), false);
    if(!log) {
        netdata_log_error("WEVT: cannot open channel '%s' for query", fullname);
        return WEVT_FAILED_TO_OPEN;
    }

    WEVT_QUERY_STATUS status;
    bool matches_filters = true;

    // if(fqs->rq.slice) {
    //     usec_t started = now_monotonic_usec();
    //
    //     matches_filters = netdata_systemd_filtering_by_journal(j, facets, fqs) || !fqs->rq.filters;
    //     usec_t ended = now_monotonic_usec();
    //
    //     fqs->c.matches_setup_ut += (ended - started);
    // }

    if(matches_filters) {
        if(fqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD)
            status = wevt_query_forward(log, wb, facets, jf, fqs);
        else
            status = wevt_query_backward(log, wb, facets, jf, fqs);
    }
    else
        status = WEVT_NO_CHANNEL_MATCHED;

    wevt_closelog6(log);

    return status;
}

static bool source_is_mine(LOGS_QUERY_SOURCE *src, LOGS_QUERY_STATUS *lqs) {
    if((lqs->rq.source_type == WEVTS_NONE && !lqs->rq.sources) || (src->source_type & lqs->rq.source_type) ||
       (lqs->rq.sources && simple_pattern_matches(lqs->rq.sources, string2str(src->source)))) {

        if(!src->msg_last_ut)
            // the file is not scanned yet, or the timestamps have not been updated,
            // so we don't know if it can contribute or not - let's add it.
            return true;

        usec_t anchor_delta = 0;
        usec_t first_ut = src->msg_first_ut - anchor_delta;
        usec_t last_ut = src->msg_last_ut + anchor_delta;

        if(last_ut >= lqs->rq.after_ut && first_ut <= lqs->rq.before_ut)
            return true;
    }

    return false;
}

static int wevt_master_query(BUFFER *wb __maybe_unused, LOGS_QUERY_STATUS *lqs __maybe_unused) {
    // make sure the sources list is updated
    wevt_sources_scan();

    FACETS *facets = lqs->facets;

    WEVT_QUERY_STATUS status = WEVT_NO_CHANNEL_MATCHED;

    lqs->c.files_matched = 0;
    lqs->c.file_working = 0;
    lqs->c.rows_useful = 0;
    lqs->c.rows_read = 0;
    lqs->c.bytes_read = 0;

    size_t files_used = 0;
    size_t files_max = dictionary_entries(wevt_sources);
    const DICTIONARY_ITEM *file_items[files_max];

    // count the files
    bool files_are_newer = false;
    LOGS_QUERY_SOURCE *src;
    dfe_start_read(wevt_sources, src) {
        if(!source_is_mine(src, lqs))
            continue;

        file_items[files_used++] = dictionary_acquired_item_dup(wevt_sources, src_dfe.item);

        if(src->msg_last_ut > lqs->rq.if_modified_since)
            files_are_newer = true;
    }
    dfe_done(jf);

    lqs->c.files_matched = files_used;

    if(lqs->rq.if_modified_since && !files_are_newer)
        return rrd_call_function_error(wb, "not modified", HTTP_RESP_NOT_MODIFIED);

    // sort the files, so that they are optimal for facets
    if(files_used >= 2) {
        if (lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  wevt_sources_dict_items_backward_compar);
        else
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  wevt_sources_dict_items_forward_compar);
    }

    bool partial = false;
    usec_t query_started_ut = now_monotonic_usec();
    usec_t started_ut = query_started_ut;
    usec_t ended_ut = started_ut;
    usec_t duration_ut = 0, max_duration_ut = 0;
    usec_t progress_duration_ut = 0;

    // sampling_query_init(lqs, facets);

    buffer_json_member_add_array(wb, "_channels");
    for(size_t f = 0; f < files_used ;f++) {
        const char *filename = dictionary_acquired_item_name(file_items[f]);
        src = dictionary_acquired_item_value(file_items[f]);

        if(!source_is_mine(src, lqs))
            continue;

        started_ut = ended_ut;

        // do not even try to do the query if we expect it to pass the timeout
        if(ended_ut + max_duration_ut * 3 >= *lqs->stop_monotonic_ut) {
            partial = true;
            status = WEVT_TIMED_OUT;
            break;
        }

        lqs->c.file_working++;

        size_t rows_useful = lqs->c.rows_useful;
        size_t rows_read = lqs->c.rows_read;
        size_t bytes_read = lqs->c.bytes_read;
        size_t matches_setup_ut = lqs->c.matches_setup_ut;

        // sampling_file_init(lqs, src);

        WEVT_QUERY_STATUS tmp_status = wevt_query_one_channel(filename, wb, facets, src, lqs);

        rows_useful = lqs->c.rows_useful - rows_useful;
        rows_read = lqs->c.rows_read - rows_read;
        bytes_read = lqs->c.bytes_read - bytes_read;
        matches_setup_ut = lqs->c.matches_setup_ut - matches_setup_ut;

        ended_ut = now_monotonic_usec();
        duration_ut = ended_ut - started_ut;

        if(duration_ut > max_duration_ut)
            max_duration_ut = duration_ut;

        progress_duration_ut += duration_ut;
        if(progress_duration_ut >= WINDOWS_EVENTS_PROGRESS_EVERY_UT) {
            progress_duration_ut = 0;
            netdata_mutex_lock(&stdout_mutex);
            pluginsd_function_progress_to_stdout(lqs->rq.transaction, f + 1, files_used);
            netdata_mutex_unlock(&stdout_mutex);
        }

        buffer_json_add_array_item_object(wb); // channel source
        {
            // information about the file
            buffer_json_member_add_string(wb, "_filename", filename);
            buffer_json_member_add_uint64(wb, "_source_type", src->source_type);
            buffer_json_member_add_string(wb, "_source", string2str(src->source));
            buffer_json_member_add_uint64(wb, "_msg_first_ut", src->msg_first_ut);
            buffer_json_member_add_uint64(wb, "_msg_last_ut", src->msg_last_ut);

            // information about the current use of the file
            buffer_json_member_add_uint64(wb, "duration_ut", ended_ut - started_ut);
            buffer_json_member_add_uint64(wb, "rows_read", rows_read);
            buffer_json_member_add_uint64(wb, "rows_useful", rows_useful);
            buffer_json_member_add_double(wb, "rows_per_second", (double) rows_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "bytes_read", bytes_read);
            buffer_json_member_add_double(wb, "bytes_per_second", (double) bytes_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "duration_matches_ut", matches_setup_ut);

            // if(lqs->rq.sampling) {
            //     buffer_json_member_add_object(wb, "_sampling");
            //     {
            //         buffer_json_member_add_uint64(wb, "sampled", lqs->c.samples_per_file.sampled);
            //         buffer_json_member_add_uint64(wb, "unsampled", lqs->c.samples_per_file.unsampled);
            //         buffer_json_member_add_uint64(wb, "estimated", lqs->c.samples_per_file.estimated);
            //     }
            //     buffer_json_object_close(wb); // _sampling
            // }
        }
        buffer_json_object_close(wb); // channel source

        bool stop = false;
        switch(tmp_status) {
            case WEVT_OK:
            case WEVT_NO_CHANNEL_MATCHED:
                status = (status == WEVT_OK) ? WEVT_OK : tmp_status;
                break;

            case WEVT_FAILED_TO_OPEN:
            case WEVT_FAILED_TO_SEEK:
                partial = true;
                if(status == WEVT_NO_CHANNEL_MATCHED)
                    status = tmp_status;
                break;

            case WEVT_CANCELLED:
            case WEVT_TIMED_OUT:
                partial = true;
                stop = true;
                status = tmp_status;
                break;

            case WEVT_NOT_MODIFIED:
                internal_fatal(true, "this should never be returned here");
                break;
        }

        if(stop)
            break;
    }
    buffer_json_array_close(wb); // _channels

    // release the files
    for(size_t f = 0; f < files_used ;f++)
        dictionary_acquired_item_release(wevt_sources, file_items[f]);

    switch (status) {
        case WEVT_OK:
            if(lqs->rq.if_modified_since && !lqs->c.rows_useful)
                return rrd_call_function_error(wb, "no useful logs, not modified", HTTP_RESP_NOT_MODIFIED);
            break;

        case WEVT_TIMED_OUT:
        case WEVT_NO_CHANNEL_MATCHED:
            break;

        case WEVT_CANCELLED:
            return rrd_call_function_error(wb, "client closed connection", HTTP_RESP_CLIENT_CLOSED_REQUEST);

        case WEVT_NOT_MODIFIED:
            return rrd_call_function_error(wb, "not modified", HTTP_RESP_NOT_MODIFIED);

        case WEVT_FAILED_TO_OPEN:
            return rrd_call_function_error(wb, "failed to open journal", HTTP_RESP_INTERNAL_SERVER_ERROR);

        case WEVT_FAILED_TO_SEEK:
            return rrd_call_function_error(wb, "failed to seek in journal", HTTP_RESP_INTERNAL_SERVER_ERROR);

        default:
            return rrd_call_function_error(wb, "unknown status", HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", partial);
    buffer_json_member_add_string(wb, "type", "table");

    // build a message for the query
    if(!lqs->rq.data_only) {
        CLEAN_BUFFER *msg = buffer_create(0, NULL);
        CLEAN_BUFFER *msg_description = buffer_create(0, NULL);
        ND_LOG_FIELD_PRIORITY msg_priority = NDLP_INFO;

        // if(!journal_files_completed_once()) {
        //     buffer_strcat(msg, "Journals are still being scanned. ");
        //     buffer_strcat(msg_description
        //                   , "LIBRARY SCAN: The journal files are still being scanned, you are probably viewing incomplete data. ");
        //     msg_priority = NDLP_WARNING;
        // }

        if(partial) {
            buffer_strcat(msg, "Query timed-out, incomplete data. ");
            buffer_strcat(msg_description
                          , "QUERY TIMEOUT: The query timed out and may not include all the data of the selected window. ");
            msg_priority = NDLP_WARNING;
        }

        // if(lqs->c.samples.estimated || lqs->c.samples.unsampled) {
        //     double percent = (double) (lqs->c.samples.sampled * 100.0 /
        //                                (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
        //     buffer_sprintf(msg, "%.2f%% real data", percent);
        //     buffer_sprintf(msg_description, "ACTUAL DATA: The filters counters reflect %0.2f%% of the data. ", percent);
        //     msg_priority = MIN(msg_priority, NDLP_NOTICE);
        // }
        //
        // if(lqs->c.samples.unsampled) {
        //     double percent = (double) (lqs->c.samples.unsampled * 100.0 /
        //                                (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
        //     buffer_sprintf(msg, ", %.2f%% unsampled", percent);
        //     buffer_sprintf(msg_description
        //                    , "UNSAMPLED DATA: %0.2f%% of the events exist and have been counted, but their values have not been evaluated, so they are not included in the filters counters. "
        //                    , percent);
        //     msg_priority = MIN(msg_priority, NDLP_NOTICE);
        // }
        //
        // if(lqs->c.samples.estimated) {
        //     double percent = (double) (lqs->c.samples.estimated * 100.0 /
        //                                (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
        //     buffer_sprintf(msg, ", %.2f%% estimated", percent);
        //     buffer_sprintf(msg_description
        //                    , "ESTIMATED DATA: The query selected a large amount of data, so to avoid delaying too much, the presented data are estimated by %0.2f%%. "
        //                    , percent);
        //     msg_priority = MIN(msg_priority, NDLP_NOTICE);
        // }

        buffer_json_member_add_object(wb, "message");
        if(buffer_tostring(msg)) {
            buffer_json_member_add_string(wb, "title", buffer_tostring(msg));
            buffer_json_member_add_string(wb, "description", buffer_tostring(msg_description));
            buffer_json_member_add_string(wb, "status", nd_log_id2priority(msg_priority));
        }
        // else send an empty object if there is nothing to tell
        buffer_json_object_close(wb); // message
    }

    if(!lqs->rq.data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", WEVT_FUNCTION_DESCRIPTION);
    }

    if(!lqs->rq.data_only || lqs->rq.tail)
        buffer_json_member_add_uint64(wb, "last_modified", lqs->last_modified);

    facets_sort_and_reorder_keys(facets);
    facets_report(facets, wb, used_hashes_registry);

    wb->expires = now_realtime_sec() + (lqs->rq.data_only ? 3600 : 0);
    buffer_json_member_add_time_t(wb, "expires", wb->expires);

    // if(lqs->rq.sampling) {
    //     buffer_json_member_add_object(wb, "_sampling");
    //     {
    //         buffer_json_member_add_uint64(wb, "sampled", lqs->c.samples.sampled);
    //         buffer_json_member_add_uint64(wb, "unsampled", lqs->c.samples.unsampled);
    //         buffer_json_member_add_uint64(wb, "estimated", lqs->c.samples.estimated);
    //     }
    //     buffer_json_object_close(wb); // _sampling
    // }

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
    return wb->response_code;
}

void function_windows_events(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                             BUFFER *payload, HTTP_ACCESS access __maybe_unused,
                             const char *source __maybe_unused, void *data __maybe_unused) {
    bool have_slice = LQS_DEFAULT_SLICE_MODE;

    LOGS_QUERY_STATUS tmp_fqs = {
            .facets = lqs_facets_create(
                    LQS_DEFAULT_ITEMS_PER_QUERY,
                    FACETS_OPTION_ALL_KEYS_FTS,
                    WEVT_ALWAYS_VISIBLE_KEYS,
                    WEVT_KEYS_INCLUDED_IN_FACETS,
                    WEVT_KEYS_EXCLUDED_FROM_FACETS,
                    have_slice),

            .rq = LOGS_QUERY_REQUEST_DEFAULTS(transaction, have_slice, FACETS_ANCHOR_DIRECTION_BACKWARD),

            .cancelled = cancelled,
            .stop_monotonic_ut = stop_monotonic_ut,
    };
    LOGS_QUERY_STATUS *lqs = &tmp_fqs;

    CLEAN_BUFFER *wb = lqs_create_output_buffer();

    // ------------------------------------------------------------------------
    // parse the parameters

    if(lqs_request_parse_and_validate(lqs, wb, function, payload, have_slice, "PRIORITY")) {
        wevt_register_transformations(lqs);

        // ------------------------------------------------------------------------
        // add versions to the response

        buffer_json_wevt_versions(wb);

        // ------------------------------------------------------------------------
        // run the request

        if (lqs->rq.info)
            lqs_info_response(wb, lqs->facets);
        else {
            wevt_master_query(wb, lqs);
            if (wb->response_code == HTTP_RESP_OK)
                buffer_json_finalize(wb);
        }
    }

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    lqs_cleanup(lqs);
}

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    clocks_init();
    nd_thread_tag_set("wevt.plugin");
    nd_log_initialize_for_external_plugins("windows-events.plugin");

    // ------------------------------------------------------------------------
    // initialization

    wevt_sources_init();

    // ------------------------------------------------------------------------
    // debug

    if(argc >= 2 && strcmp(argv[argc - 1], "debug") == 0) {
        wevt_sources_scan();

        bool cancelled = false;
        usec_t stop_monotonic_ut = now_monotonic_usec() + 600 * USEC_PER_SEC;
        char buf[] = "windows-events info after:-8640000 before:0 direction:backward last:200 data_only:false slice:true facets: source:all";
        function_windows_events("123", buf, &stop_monotonic_ut, &cancelled,
                                 NULL, HTTP_ACCESS_ALL, NULL, NULL);
        exit(1);
    }

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(WINDOWS_EVENTS_WORKER_THREADS, "WEVT", &stdout_mutex, &plugin_should_exit);

    functions_evloop_add_function(wg,
                                  WEVT_FUNCTION_NAME,
                                  function_windows_events,
                                  WINDOWS_EVENTS_DEFAULT_TIMEOUT,
                                  NULL);

    // ------------------------------------------------------------------------
    // register functions to netdata

    netdata_mutex_lock(&stdout_mutex);

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"logs\" "HTTP_ACCESS_FORMAT" %d\n",
            WEVT_FUNCTION_NAME, WINDOWS_EVENTS_DEFAULT_TIMEOUT, WEVT_FUNCTION_DESCRIPTION,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    // ------------------------------------------------------------------------

    const usec_t step_ut = 100 * USEC_PER_MS;
    usec_t send_newline_ut = 0;
    usec_t since_last_scan_ut = WINDOWS_EVENTS_SCAN_EVERY_USEC * 2; // something big to trigger scanning at start
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {

        if(since_last_scan_ut > WINDOWS_EVENTS_SCAN_EVERY_USEC) {
            wevt_sources_scan();
            since_last_scan_ut = 0;
        }

        usec_t dt_ut = heartbeat_next(&hb, step_ut);
        since_last_scan_ut += dt_ut;
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    exit(0);
}
