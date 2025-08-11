// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTIONS_EVLOOP_H
#define NETDATA_FUNCTIONS_EVLOOP_H

#include "../libnetdata.h"

#define MAX_FUNCTION_PARAMETERS 1024
#define PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT 10 // seconds

// plugins.d 1st version of the external plugins and streaming protocol
#define PLUGINSD_KEYWORD_CHART                  "CHART"
#define PLUGINSD_KEYWORD_CHART_DEFINITION_END   "CHART_DEFINITION_END"
#define PLUGINSD_KEYWORD_DIMENSION              "DIMENSION"
#define PLUGINSD_KEYWORD_BEGIN                  "BEGIN"
#define PLUGINSD_KEYWORD_SET                    "SET"
#define PLUGINSD_KEYWORD_END                    "END"
#define PLUGINSD_KEYWORD_FLUSH                  "FLUSH"
#define PLUGINSD_KEYWORD_DISABLE                "DISABLE"
#define PLUGINSD_KEYWORD_VARIABLE               "VARIABLE"
#define PLUGINSD_KEYWORD_LABEL                  "LABEL"
#define PLUGINSD_KEYWORD_OVERWRITE              "OVERWRITE"
#define PLUGINSD_KEYWORD_CLABEL                 "CLABEL"
#define PLUGINSD_KEYWORD_CLABEL_COMMIT          "CLABEL_COMMIT"
#define PLUGINSD_KEYWORD_EXIT                   "EXIT"

// high-speed versions of BEGIN, SET, END
#define PLUGINSD_KEYWORD_BEGIN_V2               "BEGIN2"
#define PLUGINSD_KEYWORD_SET_V2                 "SET2"
#define PLUGINSD_KEYWORD_END_V2                 "END2"

// super high-speed versions of BEGIN, SET, END have this as first parameter
// enabled with the streaming capability STREAM_CAP_SLOTS
#define PLUGINSD_KEYWORD_SLOT                   "SLOT" // to change the length of this, update pluginsd_extract_chart_slot() too

// virtual hosts (only for external plugins - for streaming virtual hosts are like all other hosts)
#define PLUGINSD_KEYWORD_HOST_DEFINE            "HOST_DEFINE"
#define PLUGINSD_KEYWORD_HOST_DEFINE_END        "HOST_DEFINE_END"
#define PLUGINSD_KEYWORD_HOST_LABEL             "HOST_LABEL"
#define PLUGINSD_KEYWORD_HOST                   "HOST"

// replication
// enabled with STREAM_CAP_REPLICATION
#define PLUGINSD_KEYWORD_REPLAY_CHART           "REPLAY_CHART"
#define PLUGINSD_KEYWORD_REPLAY_BEGIN           "RBEGIN"
#define PLUGINSD_KEYWORD_REPLAY_SET             "RSET"
#define PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE    "RDSTATE"
#define PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE    "RSSTATE"
#define PLUGINSD_KEYWORD_REPLAY_END             "REND"

// plugins.d accepts these for functions (from external plugins or streaming children)
// related to STREAM_CAP_FUNCTIONS, STREAM_CAP_PROGRESS
#define PLUGINSD_KEYWORD_FUNCTION               "FUNCTION"                  // define a function
#define PLUGINSD_KEYWORD_FUNCTION_PROGRESS      "FUNCTION_PROGRESS"         // send updates about function progress
#define PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN  "FUNCTION_RESULT_BEGIN"     // the result of a function transaction
#define PLUGINSD_KEYWORD_FUNCTION_RESULT_END    "FUNCTION_RESULT_END"       // the end of the result of a func. trans.

// plugins.d sends these for functions (to external plugins or streaming children)
// related to STREAM_CAP_FUNCTIONS, STREAM_CAP_PROGRESS
#define PLUGINSD_CALL_FUNCTION                  "FUNCTION"                  // call a function to a plugin or remote host
#define PLUGINSD_CALL_FUNCTION_PAYLOAD_BEGIN    "FUNCTION_PAYLOAD"          // call a function with a payload
#define PLUGINSD_CALL_FUNCTION_PAYLOAD_END      "FUNCTION_PAYLOAD_END"      // function payload ends
#define PLUGINSD_CALL_FUNCTION_CANCEL           "FUNCTION_CANCEL"           // cancel a running function transaction
#define PLUGINSD_CALL_FUNCTION_PROGRESS         "FUNCTION_PROGRESS"         // let the function know the user is waiting

#define PLUGINSD_CALL_QUIT                      "QUIT"                      // ask the plugin to quit

// dyncfg
// enabled with STREAM_CAP_DYNCFG
#define PLUGINSD_KEYWORD_CONFIG                 "CONFIG"
#define PLUGINSD_KEYWORD_CONFIG_ACTION_CREATE   "create"
#define PLUGINSD_KEYWORD_CONFIG_ACTION_DELETE   "delete"
#define PLUGINSD_KEYWORD_CONFIG_ACTION_STATUS   "status"
#define PLUGINSD_FUNCTION_CONFIG                "config"

// claiming
#define PLUGINSD_KEYWORD_NODE_ID                "NODE_ID"
#define PLUGINSD_KEYWORD_CLAIMED_ID             "CLAIMED_ID"

#define PLUGINSD_KEYWORD_JSON                   "JSON"
#define PLUGINSD_KEYWORD_JSON_END               "JSON_PAYLOAD_END"
#define PLUGINSD_KEYWORD_JSON_CMD_STREAM_PATH   "STREAM_PATH"
#define PLUGINSD_KEYWORD_JSON_CMD_ML_MODEL      "ML_MODEL"

typedef void (*functions_evloop_worker_execute_t)(const char *transaction, char *function, usec_t *stop_monotonic_ut,
                                                  bool *cancelled, BUFFER *payload, HTTP_ACCESS access,
                                                  const char *source, void *data);

struct functions_evloop_worker_job;
struct functions_evloop_globals *functions_evloop_init(size_t worker_threads, const char *tag, netdata_mutex_t *stdout_mutex, bool *plugin_should_exit);
void functions_evloop_add_function(struct functions_evloop_globals *wg, const char *function, functions_evloop_worker_execute_t cb, time_t default_timeout, void *data);
void functions_evloop_cancel_threads(struct functions_evloop_globals *wg);

#define FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT (10 * USEC_PER_SEC)
static inline void functions_stop_monotonic_update_on_progress(usec_t *stop_monotonic_ut) {
    usec_t now_ut = now_monotonic_usec();
    if(now_ut + FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT > *stop_monotonic_ut) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "Extending function timeout due to PROGRESS update...");
        __atomic_store_n(stop_monotonic_ut, now_ut + FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT, __ATOMIC_RELAXED);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_DEBUG, "Received PROGRESS update...");
}

#define pluginsd_function_result_begin_to_buffer(wb, transaction, code, content_type, expires)      \
    buffer_sprintf(wb                                                                               \
                    , PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " \"%s\" %d \"%s\" %ld\n"              \
                    , (transaction) ? (transaction) : ""                                            \
                    , (int)(code)                                                                   \
                    , (content_type) ? (content_type) : ""                                          \
                    , (long int)(expires)                                                           \
    )

#define pluginsd_function_result_end_to_buffer(wb) \
    buffer_strcat(wb, "\n" PLUGINSD_KEYWORD_FUNCTION_RESULT_END "\n")

#define pluginsd_function_result_begin_to_stdout(transaction, code, content_type, expires)          \
    fprintf(stdout                                                                                  \
                    , PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN " \"%s\" %d \"%s\" %ld\n"              \
                    , (transaction) ? (transaction) : ""                                            \
                    , (int)(code)                                                                   \
                    , (content_type) ? (content_type) : ""                                          \
                    , (long int)(expires)                                                           \
    )

#define pluginsd_function_result_end_to_stdout() \
    fprintf(stdout, "\n" PLUGINSD_KEYWORD_FUNCTION_RESULT_END "\n")

static inline void pluginsd_function_json_error_to_stdout(const char *transaction, int code, const char *msg) {
    char buffer[PLUGINSD_LINE_MAX + 1];
    json_escape_string(buffer, msg, PLUGINSD_LINE_MAX);

    pluginsd_function_result_begin_to_stdout(transaction, code, "application/json", now_realtime_sec());
    fprintf(stdout, "{\"status\":%d,\"error_message\":\"%s\"}", code, buffer);
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
}

static inline void pluginsd_function_result_to_stdout(const char *transaction, BUFFER *result) {
    pluginsd_function_result_begin_to_stdout(transaction, result->response_code,
                                             content_type_id2string(result->content_type),
                                             result->expires);

    fwrite(buffer_tostring(result), buffer_strlen(result), 1, stdout);

    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
}

static inline void pluginsd_function_progress_to_stdout(const char *transaction, size_t done, size_t all) {
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION_PROGRESS " '%s' %zu %zu\n",
            transaction, done, all);
    fflush(stdout);
}

static inline void send_newline_and_flush(netdata_mutex_t *mutex) {
    netdata_mutex_lock(mutex);
    fprintf(stdout, "\n");
    fflush(stdout);
    netdata_mutex_unlock(mutex);
}

void functions_evloop_dyncfg_add(struct functions_evloop_globals *wg, const char *id, const char *path,
                                 DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type, const char *source, DYNCFG_CMDS cmds,
                                 HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                                 dyncfg_cb_t cb, void *data);

void functions_evloop_dyncfg_del(struct functions_evloop_globals *wg, const char *id);
void functions_evloop_dyncfg_status(struct functions_evloop_globals *wg, const char *id, DYNCFG_STATUS status);

#endif //NETDATA_FUNCTIONS_EVLOOP_H
