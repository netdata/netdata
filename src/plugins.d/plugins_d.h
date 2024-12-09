// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINS_D_H
#define NETDATA_PLUGINS_D_H 1

#include "libnetdata/libnetdata.h"

struct rrdhost;

 #define PLUGINSD_CMD_MAX (FILENAME_MAX*2)
#define PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH 0

#define PLUGINSD_MAX_DIRECTORIES 20
extern char *plugin_directories[PLUGINSD_MAX_DIRECTORIES];

struct plugind {
    STRING *id;                         // config node id
    STRING *filename;                   // just the filename
    STRING *fullfilename;               // with path
    STRING *cmd;                        // the command that it executes

    size_t successful_collections;      // the number of times we have seen
                                        // values collected from this plugin

    size_t serial_failures;             // the number of times the plugin started
                                        // without collecting values

    struct rrdhost *host;               // the host the plugin collects data for
    int update_every;                   // the plugin default data collection frequency

    struct {
        SPINLOCK spinlock;
        bool running;                  // do not touch this structure after setting this to 1
        bool enabled;                   // if this is enabled or not
        ND_THREAD *thread;
        POPEN_INSTANCE *pi;
        pid_t pid;
    } unsafe;

    time_t started_t;

    struct plugind *prev;
    struct plugind *next;
};

extern struct plugind *pluginsd_root;

size_t pluginsd_process(struct rrdhost *host, struct plugind *cd, int fd_input, int fd_output, int trust_durations);

struct parser;
void pluginsd_process_cleanup(struct parser *parser);
void pluginsd_process_thread_cleanup(void *pptr);

size_t pluginsd_initialize_plugin_directories();

#endif /* NETDATA_PLUGINS_D_H */
