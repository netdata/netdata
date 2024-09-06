// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "windows-events.h"

#define WINDOWS_EVENTS_WORKER_THREADS 5
#define WINDOWS_EVENTS_DEFAULT_TIMEOUT 600

netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

typedef enum {
    WEVTS_NONE               = 0,
    WEVTS_ALL                = (1 << 0),
} WEVT_SOURCE_TYPE;

#define WEVT_FUNCTION_DESCRIPTION    "View, search and analyze Microsoft Windows events."
#define WEVT_FUNCTION_NAME           "windows-events"

// functions needed by LQS
static WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value);

// structures needed by LQS
struct lqs_extension {};

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

static WEVT_SOURCE_TYPE wevt_internal_source_type(const char *value) {
    if(strcmp(value, "all") == 0)
        return WEVTS_ALL;

    return WEVTS_NONE;
}

static void buffer_json_wevt_versions(BUFFER *wb __maybe_unused) {
    ;
}

static void wevt_register_transformations(LOGS_QUERY_STATUS *lqs __maybe_unused) {
    ;
}

static int wevt_master_query(BUFFER *wb __maybe_unused, LOGS_QUERY_STATUS *lqs __maybe_unused) {
    return HTTP_RESP_NOT_IMPLEMENTED;
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


    // ------------------------------------------------------------------------
    // debug

    if(argc >= 2 && strcmp(argv[argc - 1], "debug") == 0) {
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

    usec_t step_ut = 100 * USEC_PER_MS;
    usec_t send_newline_ut = 0;
    bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {

        usec_t dt_ut = heartbeat_next(&hb, step_ut);
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    exit(0);
}
