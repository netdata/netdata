// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INCREMENTAL_PARSER_H
#define NETDATA_INCREMENTAL_PARSER_H 1

#include "daemon/common.h"

#define PARSER_MAX_CALLBACKS 20
#define PARSER_MAX_RECOVER_KEYWORDS 128

// PARSER return codes
typedef enum parser_rc {
    PARSER_RC_OK,       // Callback was successful, go on
    PARSER_RC_STOP,     // Callback says STOP
    PARSER_RC_ERROR     // Callback failed (abort rest of callbacks)
} PARSER_RC;

typedef struct pluginsd_action {
    PARSER_RC (*set_action)(void *user, RRDSET *st, RRDDIM *rd, long long int value);
    PARSER_RC (*begin_action)(void *user, RRDSET *st, usec_t microseconds, int trust_durations);
    PARSER_RC (*end_action)(void *user, RRDSET *st);
    PARSER_RC (*chart_action)
    (void *user, char *type, char *id, char *name, char *family, char *context, char *title, char *units, char *plugin,
     char *module, int priority, int update_every, RRDSET_TYPE chart_type, char *options);
    PARSER_RC (*dimension_action)
    (void *user, RRDSET *st, char *id, char *name, char *algorithm, long multiplier, long divisor, char *options,
     RRD_ALGORITHM algorithm_type);

    PARSER_RC (*flush_action)(void *user, RRDSET *st);
    PARSER_RC (*disable_action)(void *user);
    PARSER_RC (*variable_action)(void *user, RRDHOST *host, RRDSET *st, char *name, int global, calculated_number value);
    PARSER_RC (*label_action)(void *user, char *key, char *value, LABEL_SOURCE source);
    PARSER_RC (*overwrite_action)(void *user, RRDHOST *host, struct label *new_labels);
    PARSER_RC (*clabel_action)(void *user, char *key, char *value, LABEL_SOURCE source);
    PARSER_RC (*clabel_commit_action)(void *user, RRDHOST *host, struct label *new_labels);

    PARSER_RC (*guid_action)(void *user, uuid_t *uuid);
    PARSER_RC (*context_action)(void *user, uuid_t *uuid);
    PARSER_RC (*tombstone_action)(void *user, uuid_t *uuid);
    PARSER_RC (*host_action)(void *user, char *machine_guid, char *hostname, char *registry_hostname, int update_every, char *os,
        char *timezone, char *tags);
} PLUGINSD_ACTION;

typedef enum parser_input_type {
    PARSER_INPUT_SPLIT       = 1 << 1,
    PARSER_INPUT_ORIGINAL    = 1 << 2,
    PARSER_INPUT_PROCESSED   = 1 << 3,
    PARSER_NO_PARSE_INIT     = 1 << 4,
    PARSER_NO_ACTION_INIT    = 1 << 5,
} PARSER_INPUT_TYPE;

#define PARSER_INPUT_FULL   (PARSER_INPUT_SPLIT|PARSER_INPUT_ORIGINAL)

typedef PARSER_RC (*keyword_function)(char **, void *, PLUGINSD_ACTION  *plugins_action);

typedef struct parser_keyword {
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
    uint8_t version;                // Parser version
    RRDHOST *host;
    void *input;                    // Input source e.g. stream
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
} PARSER;

PARSER *parser_init(RRDHOST *host, void *user, void *input, PARSER_INPUT_TYPE flags);
int parser_add_keyword(PARSER *working_parser, char *keyword, keyword_function func);
int parser_next(PARSER *working_parser);
int parser_action(PARSER *working_parser, char *input);
int parser_push(PARSER *working_parser, char *line);
void parser_destroy(PARSER *working_parser);
int parser_recover_input(PARSER *working_parser);

extern size_t pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp, int trust_durations);

extern PARSER_RC pluginsd_set(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_begin(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_end(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_chart(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_dimension(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_variable(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_flush(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_disable(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_label(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_overwrite(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_guid(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_context(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_tombstone(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_clabel_commit(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC pluginsd_clabel(char **words, void *user, PLUGINSD_ACTION  *plugins_action);

#endif
