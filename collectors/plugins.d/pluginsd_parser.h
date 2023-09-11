// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_PARSER_H
#define NETDATA_PLUGINSD_PARSER_H

#include "daemon/common.h"

#define WORKER_PARSER_FIRST_JOB 3

// this has to be in-sync with the same at receiver.c
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION (WORKER_PARSER_FIRST_JOB - 3)

// this controls the max response size of a function
#define PLUGINSD_MAX_DEFERRED_SIZE (20 * 1024 * 1024)

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

typedef enum __attribute__ ((__packed__)) {
    PARSER_INIT_PLUGINSD        = (1 << 1),
    PARSER_INIT_STREAMING       = (1 << 2),
} PARSER_REPERTOIRE;

struct parser;
typedef PARSER_RC (*keyword_function)(char **words, size_t num_words, struct parser *parser);

typedef struct parser_keyword {
    char *keyword;
    size_t id;
    PARSER_REPERTOIRE repertoire;
    size_t worker_job_id;
} PARSER_KEYWORD;

typedef struct parser_user_object {
    RRDSET *st;
    RRDHOST *host;
    void    *opaque;
    struct plugind *cd;
    int trust_durations;
    RRDLABELS *new_host_labels;
    RRDLABELS *chart_rrdlabels_linked_temporarily;
    size_t data_collections_count;
    int enabled;

    STREAM_CAPABILITIES capabilities; // receiver capabilities

    struct {
        bool parsing_host;
        uuid_t machine_guid;
        char machine_guid_str[UUID_STR_LEN];
        STRING *hostname;
        RRDLABELS *rrdlabels;
    } host_define;

    struct parser_user_object_replay {
        time_t start_time;
        time_t end_time;

        usec_t start_time_ut;
        usec_t end_time_ut;

        time_t wall_clock_time;

        bool rset_enabled;
    } replay;

    struct parser_user_object_v2 {
        bool locked_data_collection;
        RRDSET_STREAM_BUFFER stream_buffer; // sender capabilities in this
        time_t update_every;
        time_t end_time;
        time_t wall_clock_time;
        bool ml_locked;
    } v2;
} PARSER_USER_OBJECT;

typedef struct parser {
    uint8_t version;                // Parser version
    PARSER_REPERTOIRE repertoire;
    uint32_t flags;
    int fd;                         // Socket
    size_t line;
    FILE *fp_input;                 // Input source e.g. stream
    FILE *fp_output;                // Stream to send commands to plugin

#ifdef ENABLE_HTTPS
    NETDATA_SSL *ssl_output;
#endif

    PARSER_USER_OBJECT user;        // User defined structure to hold extra state between calls

    struct buffered_reader reader;

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

    struct {
        SPINLOCK spinlock;
    } writer;

} PARSER;

PARSER *parser_init(struct parser_user_object *user, FILE *fp_input, FILE *fp_output, int fd, PARSER_INPUT_TYPE flags, void *ssl);
void parser_init_repertoire(PARSER *parser, PARSER_REPERTOIRE repertoire);
void parser_destroy(PARSER *working_parser);
void pluginsd_cleanup_v2(PARSER *parser);
void inflight_functions_init(PARSER *parser);
void pluginsd_keywords_init(PARSER *parser, PARSER_REPERTOIRE repertoire);
PARSER_RC parser_execute(PARSER *parser, PARSER_KEYWORD *keyword, char **words, size_t num_words);

static inline int find_first_keyword(const char *src, char *dst, int dst_size, bool *isspace_map) {
    const char *s = src, *keyword_start;

    while (unlikely(isspace_map[(uint8_t)*s])) s++;
    keyword_start = s;

    while (likely(*s && !isspace_map[(uint8_t)*s]) && dst_size > 1) {
        *dst++ = *s++;
        dst_size--;
    }
    *dst = '\0';
    return dst_size == 0 ? 0 : (int) (s - keyword_start);
}

PARSER_KEYWORD *gperf_lookup_keyword(register const char *str, register size_t len);

static inline PARSER_KEYWORD *parser_find_keyword(PARSER *parser, const char *command) {
    PARSER_KEYWORD *t = gperf_lookup_keyword(command, strlen(command));
    if(t && (t->repertoire & parser->repertoire))
        return t;

    return NULL;
}

static inline int parser_action(PARSER *parser, char *input) {
    parser->line++;

    if(unlikely(parser->flags & PARSER_DEFER_UNTIL_KEYWORD)) {
        char command[PLUGINSD_LINE_MAX + 1];
        bool has_keyword = find_first_keyword(input, command, PLUGINSD_LINE_MAX, isspace_map_pluginsd);

        if(!has_keyword || strcmp(command, parser->defer.end_keyword) != 0) {
            if(parser->defer.response) {
                buffer_strcat(parser->defer.response, input);
                if(buffer_strlen(parser->defer.response) > PLUGINSD_MAX_DEFERRED_SIZE) {
                    // more than PLUGINSD_MAX_DEFERRED_SIZE of data,
                    // or a bad plugin that did not send the end_keyword
                    internal_error(true, "PLUGINSD: deferred response is too big (%zu bytes). Stopping this plugin.", buffer_strlen(parser->defer.response));
                    return 1;
                }
            }
            return 0;
        }
        else {
            // call the action
            parser->defer.action(parser, parser->defer.action_data);

            // empty everything
            parser->defer.action = NULL;
            parser->defer.action_data = NULL;
            parser->defer.end_keyword = NULL;
            parser->defer.response = NULL;
            parser->flags &= ~PARSER_DEFER_UNTIL_KEYWORD;
        }
        return 0;
    }

    char *words[PLUGINSD_MAX_WORDS];
    size_t num_words = quoted_strings_splitter_pluginsd(input, words, PLUGINSD_MAX_WORDS);
    const char *command = get_word(words, num_words, 0);

    if(unlikely(!command))
        return 0;

    PARSER_RC rc;
    PARSER_KEYWORD *t = parser_find_keyword(parser, command);
    if(likely(t)) {
        worker_is_busy(t->worker_job_id);
        rc = parser_execute(parser, t, words, num_words);
        // rc = (*t->func)(words, num_words, parser);
        worker_is_idle();
    }
    else
        rc = PARSER_RC_ERROR;

    if(rc == PARSER_RC_ERROR) {
        BUFFER *wb = buffer_create(PLUGINSD_LINE_MAX, NULL);
        for(size_t i = 0; i < num_words ;i++) {
            if(i) buffer_fast_strcat(wb, " ", 1);

            buffer_fast_strcat(wb, "\"", 1);
            const char *s = get_word(words, num_words, i);
            buffer_strcat(wb, s?s:"");
            buffer_fast_strcat(wb, "\"", 1);
        }

        netdata_log_error("PLUGINSD: parser_action('%s') failed on line %zu: { %s } (quotes added to show parsing)",
                          command, parser->line, buffer_tostring(wb));

        buffer_free(wb);
    }

    return (rc == PARSER_RC_ERROR || rc == PARSER_RC_STOP);
}

#endif //NETDATA_PLUGINSD_PARSER_H
