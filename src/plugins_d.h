#ifndef NETDATA_PLUGINS_D_H
#define NETDATA_PLUGINS_D_H 1

/**
 * @file plugins_d.h
 * @brief Thread maintains external plugins.
 */

/// File suffix a plugin must have
#define PLUGINSD_FILE_SUFFIX ".plugin"
/// Length of `PLUGINSD_FILE_SUFFIX`          
#define PLUGINSD_FILE_SUFFIX_LEN strlen(PLUGINSD_FILE_SUFFIX)
/// Max length of command starting a plugin.
#define PLUGINSD_CMD_MAX (FILENAME_MAX*2)
/// Maximum line of a line.
#define PLUGINSD_LINE_MAX 1024

/** List of plugin daemons */
struct plugind {
    char id[CONFIG_MAX_NAME+1];         ///< config node id

    char filename[FILENAME_MAX+1];      ///< just the filename
    char fullfilename[FILENAME_MAX+1];  ///< with path
    char cmd[PLUGINSD_CMD_MAX+1];       ///< the command that is executes

    pid_t pid;                          ///< process id
    pthread_t thread;                   ///< the thread

    size_t successful_collections;      ///< the number of times we have seen
                                        ///< values collected from this plugin

    size_t serial_failures;             ///< the number of times the plugin started
                                        ///< without collecting values

    int update_every;                   ///< the plugin default data collection frequency
    volatile int obsolete;              ///< do not touch this structure after setting this to 1
    volatile int enabled;               ///< if this is enabled or not

    time_t started_t;                   ///< time thread started

    struct plugind *next;               ///< the next daemon in the list
};

/// Global struct plugind
extern struct plugind *pluginsd_root;

/**
 * Main method of thread pluginsd.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *pluginsd_main(void *ptr);

#endif /* NETDATA_PLUGINS_D_H */
