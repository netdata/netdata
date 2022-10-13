// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INCREMENTAL_PARSER_H
#define NETDATA_INCREMENTAL_PARSER_H 1

#include "daemon/common.h"

#define PARSER_MAX_CALLBACKS 20
#define PARSER_MAX_RECOVER_KEYWORDS 128
#define WORKER_PARSER_FIRST_JOB 1

// PARSER return codes
typedef enum parser_rc {
    PARSER_RC_OK,       // Callback was successful, go on
    PARSER_RC_STOP,     // Callback says STOP
    PARSER_RC_ERROR     // Callback failed (abort rest of callbacks)
} PARSER_RC;

typedef struct pluginsd_action {
    PARSER_RC (*guid_action)(void *user, uuid_t *uuid);
    PARSER_RC (*context_action)(void *user, uuid_t *uuid);
    PARSER_RC (*tombstone_action)(void *user, uuid_t *uuid);
    PARSER_RC (*host_action)(void *user, char *machine_guid, char *hostname, char *registry_hostname, int update_every, char *os,
        char *timezone, char *tags);
} PLUGINSD_ACTION;

typedef enum parser_input_type {
    PARSER_INPUT_SPLIT          = (1 << 1),
    PARSER_INPUT_KEEP_ORIGINAL  = (1 << 2),
    PARSER_INPUT_PROCESSED      = (1 << 3),
    PARSER_NO_PARSE_INIT        = (1 << 4),
    PARSER_NO_ACTION_INIT       = (1 << 5),
    PARSER_DEFER_UNTIL_KEYWORD  = (1 << 6),
} PARSER_INPUT_TYPE;

#define PARSER_INPUT_FULL   (PARSER_INPUT_SPLIT|PARSER_INPUT_ORIGINAL)

typedef PARSER_RC (*keyword_function)(char **, void *, PLUGINSD_ACTION  *plugins_action);

typedef struct parser_keyword {
    size_t      worker_job_id;
    char        *keyword;
    uint32_t    keyword_hash;
    int         func_no;
    keyword_function    func[PARSER_MAX_CALLBACKS+1];
    struct      parser_keyword *next;
} PARSER_KEYWORD;

typedef struct parser_data {
    char  *line;
    struct parser_data *next;
} PARSER_DATA;

typedef struct parser {
    size_t worker_job_next_id;
    uint8_t version;                // Parser version
    RRDHOST *host;
    void *input;                    // Input source e.g. stream
    void *output;                   // Stream to send commands to plugin
    PARSER_DATA    *data;           // extra input
    PARSER_KEYWORD  *keyword;       // List of parse keywords and functions
    PLUGINSD_ACTION *plugins_action;
    void    *user;                  // User defined structure to hold extra state between calls
    uint32_t flags;

    char *(*read_function)(char *buffer, long unsigned int, void *input);
    int (*eof_function)(void *input);
    keyword_function unknown_function;
    char buffer[PLUGINSD_LINE_MAX];
    char *recover_location[PARSER_MAX_RECOVER_KEYWORDS+1];
    char recover_input[PARSER_MAX_RECOVER_KEYWORDS];
#ifdef ENABLE_HTTPS
    int bytesleft;
    char tmpbuffer[PLUGINSD_LINE_MAX];
    char *readfrom;
#endif

    struct {
        const char *end_keyword;
        BUFFER *response;
        void (*action)(struct parser *parser, void *action_data);
        void *action_data;
    } defer;

    struct {
        DICTIONARY *functions;
        usec_t smaller_timeout;
    } inflight;

} PARSER;

int find_first_keyword(const char *str, char *keyword, int max_size, int (*custom_isspace)(char));

PARSER *parser_init(RRDHOST *host, void *user, void *input, void *output, PARSER_INPUT_TYPE flags);
int parser_add_keyword(PARSER *working_parser, char *keyword, keyword_function func);
int parser_next(PARSER *working_parser);
int parser_action(PARSER *working_parser, char *input);
int parser_push(PARSER *working_parser, char *line);
void parser_destroy(PARSER *working_parser);
int parser_recover_input(PARSER *working_parser);

size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp_plugin_input, FILE *fp_plugin_output, int trust_durations);

PARSER_RC pluginsd_set(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_begin(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_end(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_chart(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_dimension(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_variable(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_flush(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_disable(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_label(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_overwrite(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_guid(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_context(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_tombstone(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_clabel_commit(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
PARSER_RC pluginsd_clabel(char **words, void *user, PLUGINSD_ACTION  *plugins_action);

#endif
