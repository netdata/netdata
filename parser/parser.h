// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INCREMENTAL_PARSER_H
#define NETDATA_INCREMENTAL_PARSER_H 1

#include "daemon/common.h"

#define PARSER_MAX_CALLBACKS 20
#define PARSER_MAX_RECOVER_KEYWORDS 128
#define WORKER_PARSER_FIRST_JOB 3

// this has to be in-sync with the same at receiver.c
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION (WORKER_PARSER_FIRST_JOB - 3)

// PARSER return codes
typedef enum parser_rc {
    PARSER_RC_OK,       // Callback was successful, go on
    PARSER_RC_STOP,     // Callback says STOP
    PARSER_RC_ERROR     // Callback failed (abort rest of callbacks)
} PARSER_RC;

typedef enum parser_input_type {
    PARSER_INPUT_SPLIT          = (1 << 1),
    PARSER_INPUT_KEEP_ORIGINAL  = (1 << 2),
    PARSER_INPUT_PROCESSED      = (1 << 3),
    PARSER_NO_PARSE_INIT        = (1 << 4),
    PARSER_NO_ACTION_INIT       = (1 << 5),
    PARSER_DEFER_UNTIL_KEYWORD  = (1 << 6),
} PARSER_INPUT_TYPE;

#define PARSER_INPUT_FULL   (PARSER_INPUT_SPLIT|PARSER_INPUT_ORIGINAL)

typedef PARSER_RC (*keyword_function)(char **words, size_t num_words, void *user_data);

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
    int fd;                         // Socket
    FILE *fp_input;                 // Input source e.g. stream
    FILE *fp_output;                // Stream to send commands to plugin
#ifdef ENABLE_HTTPS
    struct netdata_ssl *ssl_output;
#endif
    PARSER_DATA    *data;           // extra input
    PARSER_KEYWORD  *keyword;       // List of parse keywords and functions
    void    *user;                  // User defined structure to hold extra state between calls
    uint32_t flags;
    size_t line;

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

PARSER *parser_init(RRDHOST *host, void *user, FILE *fp_input, FILE *fp_output, int fd, PARSER_INPUT_TYPE flags, void *ssl);
int parser_add_keyword(PARSER *working_parser, char *keyword, keyword_function func);
int parser_next(PARSER *working_parser);
int parser_action(PARSER *working_parser, char *input);
int parser_push(PARSER *working_parser, char *line);
void parser_destroy(PARSER *working_parser);
int parser_recover_input(PARSER *working_parser);

size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp_plugin_input, FILE *fp_plugin_output, int trust_durations);

PARSER_RC pluginsd_set(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_begin(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_end(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_chart(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_chart_definition_end(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_dimension(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_variable(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_flush(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_disable(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_label(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_overwrite(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_clabel_commit(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_clabel(char **words, size_t num_words, void *user);

PARSER_RC pluginsd_replay_rrdset_begin(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_set(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_end(char **words, size_t num_words, void *user);

#endif
