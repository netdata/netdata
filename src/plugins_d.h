// SPDX-License-Identifier: GPL-3.0+

#ifndef NETDATA_PLUGINS_D_H
#define NETDATA_PLUGINS_D_H 1

#define PLUGINSD_FILE_SUFFIX ".plugin"
#define PLUGINSD_FILE_SUFFIX_LEN strlen(PLUGINSD_FILE_SUFFIX)
#define PLUGINSD_CMD_MAX (FILENAME_MAX*2)

#define PLUGINSD_KEYWORD_CHART "CHART"
#define PLUGINSD_KEYWORD_DIMENSION "DIMENSION"
#define PLUGINSD_KEYWORD_BEGIN "BEGIN"
#define PLUGINSD_KEYWORD_END "END"
#define PLUGINSD_KEYWORD_FLUSH "FLUSH"
#define PLUGINSD_KEYWORD_DISABLE "DISABLE"
#define PLUGINSD_KEYWORD_VARIABLE "VARIABLE"

#define PLUGINSD_LINE_MAX 1024
#define PLUGINSD_MAX_WORDS 20

#define PLUGINSD_MAX_DIRECTORIES 20
extern char *plugin_directories[PLUGINSD_MAX_DIRECTORIES];

struct plugind {
    char id[CONFIG_MAX_NAME+1];         // config node id

    char filename[FILENAME_MAX+1];      // just the filename
    char fullfilename[FILENAME_MAX+1];  // with path
    char cmd[PLUGINSD_CMD_MAX+1];       // the command that it executes

    volatile pid_t pid;
    netdata_thread_t thread;

    size_t successful_collections;      // the number of times we have seen
                                        // values collected from this plugin

    size_t serial_failures;             // the number of times the plugin started
                                        // without collecting values

    int update_every;                   // the plugin default data collection frequency
    volatile sig_atomic_t obsolete;     // do not touch this structure after setting this to 1
    volatile sig_atomic_t enabled;      // if this is enabled or not

    time_t started_t;

    struct plugind *next;
};

extern struct plugind *pluginsd_root;

extern void *pluginsd_main(void *ptr);

extern size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp, int trust_durations);
extern int pluginsd_split_words(char *str, char **words, int max_words);

extern int quoted_strings_splitter(char *str, char **words, int max_words, int (*custom_isspace)(char));
extern int config_isspace(char c);

#endif /* NETDATA_PLUGINS_D_H */
