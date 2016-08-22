#ifndef NETDATA_PLUGINS_D_H
#define NETDATA_PLUGINS_D_H 1

#define PLUGINSD_FILE_SUFFIX ".plugin"
#define PLUGINSD_FILE_SUFFIX_LEN strlen(PLUGINSD_FILE_SUFFIX)
#define PLUGINSD_CMD_MAX (FILENAME_MAX*2)
#define PLUGINSD_LINE_MAX 1024

struct plugind {
    char id[CONFIG_MAX_NAME+1];         // config node id

    char filename[FILENAME_MAX+1];      // just the filename
    char fullfilename[FILENAME_MAX+1];  // with path
    char cmd[PLUGINSD_CMD_MAX+1];       // the command that is executes

    pid_t pid;
    pthread_t thread;

    size_t successful_collections;      // the number of times we have seen
                                        // values collected from this plugin

    size_t serial_failures;             // the number of times the plugin started
                                        // without collecting values

    int update_every;                   // the plugin default data collection frequency
    int obsolete;                       // do not touch this structure after setting this to 1
    int enabled;                        // if this is enabled or not

    time_t started_t;

    struct plugind *next;
};

extern struct plugind *pluginsd_root;

extern void *pluginsd_main(void *ptr);

#endif /* NETDATA_PLUGINS_D_H */
