// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTIONS_EVLOOP_H
#define NETDATA_FUNCTIONS_EVLOOP_H

#include "../libnetdata.h"

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
#define PLUGINSD_KEYWORD_FUNCTION               "FUNCTION"
#define PLUGINSD_KEYWORD_FUNCTION_CANCEL        "FUNCTION_CANCEL"
#define PLUGINSD_KEYWORD_FUNCTION_RESULT_BEGIN  "FUNCTION_RESULT_BEGIN"
#define PLUGINSD_KEYWORD_FUNCTION_RESULT_END    "FUNCTION_RESULT_END"

#define PLUGINSD_KEYWORD_REPLAY_CHART           "REPLAY_CHART"
#define PLUGINSD_KEYWORD_REPLAY_BEGIN           "RBEGIN"
#define PLUGINSD_KEYWORD_REPLAY_SET             "RSET"
#define PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE    "RDSTATE"
#define PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE    "RSSTATE"
#define PLUGINSD_KEYWORD_REPLAY_END             "REND"

#define PLUGINSD_KEYWORD_BEGIN_V2               "BEGIN2"
#define PLUGINSD_KEYWORD_SET_V2                 "SET2"
#define PLUGINSD_KEYWORD_END_V2                 "END2"

#define PLUGINSD_KEYWORD_HOST_DEFINE            "HOST_DEFINE"
#define PLUGINSD_KEYWORD_HOST_DEFINE_END        "HOST_DEFINE_END"
#define PLUGINSD_KEYWORD_HOST_LABEL             "HOST_LABEL"
#define PLUGINSD_KEYWORD_HOST                   "HOST"

#define PLUGINSD_KEYWORD_DYNCFG_ENABLE          "DYNCFG_ENABLE"
#define PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE "DYNCFG_REGISTER_MODULE"

#define PLUGINSD_KEYWORD_REPORT_JOB_STATUS      "REPORT_JOB_STATUS"

#define PLUGINSD_KEYWORD_EXIT                   "EXIT"

#define PLUGINSD_KEYWORD_SLOT                   "SLOT" // to change the length of this, update pluginsd_extract_chart_slot() too

#define PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT 10 // seconds

typedef void (*functions_evloop_worker_execute_t)(const char *transaction, char *function, int timeout, bool *cancelled);
struct functions_evloop_worker_job;
struct functions_evloop_globals *functions_evloop_init(size_t worker_threads, const char *tag, netdata_mutex_t *stdout_mutex, bool *plugin_should_exit);
void functions_evloop_add_function(struct functions_evloop_globals *wg, const char *function, functions_evloop_worker_execute_t cb, time_t default_timeout);


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

static inline void pluginsd_function_result_to_stdout(const char *transaction, int code, const char *content_type, time_t expires, BUFFER *result) {
    pluginsd_function_result_begin_to_stdout(transaction, code, content_type, expires);
    fwrite(buffer_tostring(result), buffer_strlen(result), 1, stdout);
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
}

#endif //NETDATA_FUNCTIONS_EVLOOP_H
