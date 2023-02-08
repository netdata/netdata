// SPDX-License-Identifier: GPL-3.0-or-later

#include "parser.h"
#include "collectors/plugins.d/pluginsd_parser.h"

static inline int find_first_keyword(const char *src, char *dst, int dst_size, int (*custom_isspace)(char)) {
    const char *s = src, *keyword_start;

    while (unlikely(custom_isspace(*s))) s++;
    keyword_start = s;

    while (likely(*s && !custom_isspace(*s)) && dst_size > 1) {
        *dst++ = *s++;
        dst_size--;
    }
    *dst = '\0';
    return dst_size == 0 ? 0 : (int) (s - keyword_start);
}

/*
 * Initialize a parser 
 *     user   : as defined by the user, will be shared across calls
 *     input  : main input stream (auto detect stream -- file, socket, pipe)
 *     buffer : This is the buffer to be used (if null a buffer of size will be allocated)
 *     size   : buffer size either passed or will be allocated
 *              If the buffer is auto allocated, it will auto freed when the parser is destroyed
 *     
 * 
 */

PARSER *parser_init(void *user, parser_cleanup_t cleanup_cb, FILE *fp_input, FILE *fp_output, int fd,
                    PARSER_INPUT_TYPE flags, void *ssl __maybe_unused)
{
    PARSER *parser;

    parser = callocz(1, sizeof(*parser));
    parser->user = user;
    parser->user_cleanup_cb = cleanup_cb;
    parser->fd = fd;
    parser->fp_input = fp_input;
    parser->fp_output = fp_output;
#ifdef ENABLE_HTTPS
    parser->ssl_output = ssl;
#endif
    parser->flags = flags;
    parser->worker_job_next_id = WORKER_PARSER_FIRST_JOB;

    return parser;
}


static inline PARSER_KEYWORD *parser_find_keyword(PARSER *parser, const char *command) {
    uint32_t hash = djb2_hash(command);
    uint32_t slot = hash % PARSER_KEYWORDS_HASHTABLE_SIZE;
    PARSER_KEYWORD *t = parser->keywords.hashtable[slot];

    while(t) {
        if (hash == t->hash && (!strcmp(command, t->keyword)))
            return t;

        t = t->next;
    }

    return NULL;
}

/*
 * Add a keyword and the corresponding function that will be called
 * Multiple functions may be added
 * Input : keyword
 *       : callback function
 *       : flags
 * Output: > 0 registered function number
 *       : 0 Error
 */

int parser_add_keyword(PARSER *parser, char *keyword, keyword_function func) {
    if(unlikely(!keyword || !*keyword || !func))
        return 0;

    PARSER_KEYWORD *t = parser_find_keyword(parser, keyword);
    if(t) {
        for(int i = 0; i < t->func_no ;i++) {
            if (t->functions_array[i] == func) {
                error("PARSER: duplicate definition of the same function for keyword '%s'",
                      keyword);
                return i;
            }
        }

        if (t->func_no == PARSER_MAX_CALLBACKS) {
            error("PARSER: maximum number of callbacks reached on keyword '%s'",
                  keyword);
            return 0;
        }

        t->functions_array[t->func_no++] = (void *) func;
        return t->func_no;
    }

    t = callocz(1, sizeof(*t));
    t->worker_job_id = parser->worker_job_next_id++;
    t->keyword = strdupz(keyword);
    t->hash = djb2_hash(keyword);
    t->functions_array[t->func_no++] = func;

    uint32_t slot = t->hash % PARSER_KEYWORDS_HASHTABLE_SIZE;

    if(parser->keywords.hashtable[slot])
        fatal("PARSER: hashtable collision between keyword '%s' (%u) and '%s' (%u) on slot %u. "
              "Change the hashtable size.",
              parser->keywords.hashtable[slot]->keyword,
              parser->keywords.hashtable[slot]->hash,
              t->keyword,
              t->hash,
              slot
              );

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(parser->keywords.hashtable[slot], t, prev, next);

    worker_register_job_name(t->worker_job_id, t->keyword);

    return t->func_no;
}

/*
 * Cleanup a previously allocated parser
 */

void parser_destroy(PARSER *parser)
{
    if (unlikely(!parser))
        return;

    dictionary_destroy(parser->inflight.functions);

    // Remove keywords
    for(size_t i = 0 ; i < PARSER_KEYWORDS_HASHTABLE_SIZE; i++) {
        PARSER_KEYWORD  *t, *next;
        t = parser->keywords.hashtable[i];
        while (t) {
            next = t->next;
            freez(t->keyword);
            freez(t);
            t = next;
        }
    }
    
    freez(parser);
}


/*
 * Fetch the next line to process
 *
 */

int parser_next(PARSER *parser, char *buffer, size_t buffer_size)
{
    char *tmp = fgets(buffer, (int)buffer_size, (FILE *)parser->fp_input);

    if (unlikely(!tmp)) {
        if (feof((FILE *)parser->fp_input))
            error("PARSER: read failed: end of file");

        else if (ferror((FILE *)parser->fp_input))
            error("PARSER: read failed: input error");

        else
            error("PARSER: read failed: unknown error");

        return 1;
    }

    return 0;
}


/*
* Takes an initialized parser object that has an unprocessed entry (by calling parser_next)
* and if it contains a valid keyword, it will execute all the callbacks
*
*/

inline int parser_action(PARSER *parser, char *input)
{
    parser->line++;

    if(unlikely(parser->flags & PARSER_DEFER_UNTIL_KEYWORD)) {
        char command[PLUGINSD_LINE_MAX + 1];
        bool has_keyword = find_first_keyword(input, command, PLUGINSD_LINE_MAX, pluginsd_space);

        if(!has_keyword || strcmp(command, parser->defer.end_keyword) != 0) {
            if(parser->defer.response) {
                buffer_strcat(parser->defer.response, input);
                if(buffer_strlen(parser->defer.response) > 10 * 1024 * 1024) {
                    // more than 10MB of data
                    // a bad plugin that did not send the end_keyword
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
    size_t num_words = pluginsd_split_words(input, words, PLUGINSD_MAX_WORDS);
    const char *command = get_word(words, num_words, 0);

    if(unlikely(!command))
        return 0;

    PARSER_RC rc;
    PARSER_KEYWORD *t = parser_find_keyword(parser, command);
    if(likely(t)) {
        keyword_function *func = t->functions_array;

        worker_is_busy(t->worker_job_id);

        do {
            rc = (*func)(words, num_words, parser->user);
            if (unlikely(rc == PARSER_RC_ERROR || rc == PARSER_RC_STOP))
                break;
        } while(*(++func));

        worker_is_idle();
    }
    else
        rc = PARSER_RC_ERROR;

#ifdef NETDATA_INTERNAL_CHECKS
    if(rc == PARSER_RC_ERROR) {
        BUFFER *wb = buffer_create(PLUGINSD_LINE_MAX, NULL);
        for(size_t i = 0; i < num_words ;i++) {
            if(i) buffer_fast_strcat(wb, " ", 1);

            buffer_fast_strcat(wb, "\"", 1);
            const char *s = get_word(words, num_words, i);
            buffer_strcat(wb, s?s:"");
            buffer_fast_strcat(wb, "\"", 1);
        }

        internal_error(true, "PLUGINSD: parser_action('%s') failed on line %zu: { %s } (quotes added to show parsing)",
                       command, parser->line, buffer_tostring(wb));

        buffer_free(wb);
    }
#endif

    return (rc == PARSER_RC_ERROR || rc == PARSER_RC_STOP);
}
