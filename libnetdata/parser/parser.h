// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_INCREMENTAL_PARSER_H
#define NETDATA_INCREMENTAL_PARSER_H 1

#include "../libnetdata.h"

#define WORKER_PARSER_FIRST_JOB 3

// this has to be in-sync with the same at receiver.c
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION (WORKER_PARSER_FIRST_JOB - 3)

#define PARSER_KEYWORDS_HASHTABLE_SIZE 73 // unittest finds this magic number
//#define parser_hash_function(s) djb2_hash32(s)
//#define parser_hash_function(s) fnv1_hash32(s)
//#define parser_hash_function(s) fnv1a_hash32(s)
//#define parser_hash_function(s) larson_hash32(s)
#define parser_hash_function(s) pluginsd_parser_hash32(s)

// PARSER return codes
typedef enum __attribute__ ((__packed__)) parser_rc {
    PARSER_RC_OK,       // Callback was successful, go on
    PARSER_RC_STOP,     // Callback says STOP
    PARSER_RC_ERROR     // Callback failed (abort rest of callbacks)
} PARSER_RC;

typedef enum __attribute__ ((__packed__)) parser_input_type {
    PARSER_INPUT_SPLIT          = (1 << 1),
    PARSER_DEFER_UNTIL_KEYWORD  = (1 << 2),
} PARSER_INPUT_TYPE;

typedef PARSER_RC (*keyword_function)(char **words, size_t num_words, void *user_data);

typedef struct parser_keyword {
    size_t worker_job_id;
    char *keyword;
    keyword_function func;
} PARSER_KEYWORD;

typedef struct parser {
    size_t worker_job_next_id;
    uint8_t version;                // Parser version
    int fd;                         // Socket
    FILE *fp_input;                 // Input source e.g. stream
    FILE *fp_output;                // Stream to send commands to plugin
#ifdef ENABLE_HTTPS
    NETDATA_SSL *ssl_output;
#endif
    void *user;                     // User defined structure to hold extra state between calls
    uint32_t flags;
    size_t line;

    struct {
        PARSER_KEYWORD *hashtable[PARSER_KEYWORDS_HASHTABLE_SIZE];
    } keywords;

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

PARSER *parser_init(void *user, FILE *fp_input, FILE *fp_output, int fd, PARSER_INPUT_TYPE flags, void *ssl);
void parser_add_keyword(PARSER *working_parser, char *keyword, keyword_function func);
int parser_next(PARSER *working_parser, char *buffer, size_t buffer_size);
int parser_action(PARSER *working_parser, char *input);
void parser_destroy(PARSER *working_parser);

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

PARSER_RC pluginsd_replay_begin(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_rrddim_collection_state(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_rrdset_collection_state(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_set(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_replay_end(char **words, size_t num_words, void *user);

PARSER_RC pluginsd_begin_v2(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_set_v2(char **words, size_t num_words, void *user);
PARSER_RC pluginsd_end_v2(char **words, size_t num_words, void *user);
void pluginsd_cleanup_v2(void *user);

#endif
