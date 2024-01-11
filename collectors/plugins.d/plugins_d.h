// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINS_D_H
#define NETDATA_PLUGINS_D_H 1

#include "daemon/common.h"

#define PLUGINSD_FILE_SUFFIX ".plugin"
#define PLUGINSD_FILE_SUFFIX_LEN strlen(PLUGINSD_FILE_SUFFIX)
#define PLUGINSD_CMD_MAX (FILENAME_MAX*2)
#define PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH 0

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

#endif /* NETDATA_PLUGINS_D_H */
