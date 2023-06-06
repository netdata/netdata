// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINS_D_H
#define NETDATA_PLUGINS_D_H 1

#include "daemon/common.h"

#define PLUGINSD_FILE_SUFFIX ".plugin"
#define PLUGINSD_FILE_SUFFIX_LEN strlen(PLUGINSD_FILE_SUFFIX)
#define PLUGINSD_CMD_MAX (FILENAME_MAX*2)
#define PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH 0

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

#define PLUGINSD_KEYWORD_EXIT                   "EXIT"

#define PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT 10 // seconds

#define PLUGINSD_LINE_MAX_SSL_READ 512

#define PLUGINSD_MAX_WORDS 20

#define PLUGINSD_MAX_DIRECTORIES 20
extern char *plugin_directories[PLUGINSD_MAX_DIRECTORIES];

struct plugind {
    char id[CONFIG_MAX_NAME+1];         // config node id

    char filename[FILENAME_MAX+1];      // just the filename
    char fullfilename[FILENAME_MAX+1];  // with path
    char cmd[PLUGINSD_CMD_MAX+1];       // the command that it executes

    size_t successful_collections;      // the number of times we have seen
                                        // values collected from this plugin

    size_t serial_failures;             // the number of times the plugin started
                                        // without collecting values

    RRDHOST *host;                      // the host the plugin collects data for
    int update_every;                   // the plugin default data collection frequency

    struct {
        SPINLOCK spinlock;
        bool running;                  // do not touch this structure after setting this to 1
        bool enabled;                   // if this is enabled or not
        netdata_thread_t thread;
        pid_t pid;
    } unsafe;

    time_t started_t;

    struct plugind *prev;
    struct plugind *next;
};

extern struct plugind *pluginsd_root;

size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp_plugin_input, FILE *fp_plugin_output, int trust_durations);
void pluginsd_process_thread_cleanup(void *ptr);

size_t pluginsd_initialize_plugin_directories();



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

#endif /* NETDATA_PLUGINS_D_H */
