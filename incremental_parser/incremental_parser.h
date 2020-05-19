// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INCREMENTAL_PARSER_H
#define NETDATA_INCREMENTAL_PARSER_H 1

#include "../daemon/common.h"

#define PARSER_MAX_CALLBACKS 20

// PARSER return codes
typedef enum parser_rc {
    PARSER_RC_OK,       // Callback was successful, go on
    PARSER_RC_STOP,     // Callback says STOP
    PARSER_RC_ERROR     // Callback failed (abort rest of callbacks)
} PARSER_RC;

typedef struct pluginsd_action {
    PARSER_RC (*set_action)(void *user, char *variable, char *value);
    PARSER_RC (*begin_action)(void *user, char *chart_id, usec_t microseconds);
    PARSER_RC (*end_action)(void *user);
    PARSER_RC (*chart_action)(void *user, char *type, char *id, char *title,
     char *units, char *family, char *context, RRDSET_TYPE chart_type, int priority, int update_every,
     char *options, char *plugin, char *module);
    PARSER_RC (*dimension_action)
    (void *user, char *id, char *name, char *algorithm, long multiplier, long divisor, RRD_ALGORITHM algorithm_type);
    PARSER_RC (*flush_action)(void *user);
    PARSER_RC (*disable_action)(void *user);
    PARSER_RC (*variable_action)(void *user, int global, char *name, calculated_number value);
    PARSER_RC (*label_action)(void *user, char *labels);
    PARSER_RC (*overwrite_action)(void *user);
} PLUGINSD_ACTION;

typedef enum parser_input_type {
    PARSER_INPUT_SPLIT       = 1 << 1,
    PARSER_INPUT_ORIGINAL    = 1 << 2,
    PARSER_INPUT_PROCESSED   = 1 << 3,
} PARSER_INPUT_TYPE;

#define PARSER_INPUT_FULL   (PARSER_INPUT_SPLIT|PARSER_INPUT_ORIGINAL)

typedef PARSER_RC (*keyword_function)(char **, void *);

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

typedef struct incremental_parser {
    uint8_t version;                // Parser version
    RRDHOST *host;
    void *input;                    // Input source e.g. stream
    PARSER_DATA    *data;           // extra input
    PARSER_KEYWORD  *keyword;       // List of parse keywords and functions
    void    *user;                  // User defined structure to hold extra state between calls
    uint32_t flags;

    char *(*read_function)(char *buffer, long unsigned int, void *input);
    int (*eof_function)(void *input);
    keyword_function unknown_function;
    char buffer[PLUGINSD_LINE_MAX];
#ifdef ENABLE_HTTPS
    int bytesleft;
    char tmpbuffer[PLUGINSD_LINE_MAX];
    char *readfrom;
#endif
} INCREMENTAL_PARSER;

INCREMENTAL_PARSER *parser_init(RRDHOST *host, void *user, void *input, PARSER_INPUT_TYPE flags);
int parser_add_keyword(INCREMENTAL_PARSER *working_parser, char *keyword, keyword_function func);
int parser_next(INCREMENTAL_PARSER *working_parser);
int parser_action(INCREMENTAL_PARSER *working_parser);
int parser_push(INCREMENTAL_PARSER *working_parser, char *line);
void parser_destroy(INCREMENTAL_PARSER *working_parser);

extern size_t incremental_pluginsd_process(RRDHOST *host, struct plugind *cd, FILE *fp, int trust_durations);

#endif
