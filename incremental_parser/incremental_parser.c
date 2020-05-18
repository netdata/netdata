// SPDX-License-Identifier: GPL-3.0-or-later

#include "incremental_parser.h"

static inline int find_keyword(char *str, char *keyword, int max_size, int (*custom_isspace)(char))
{
    char *s = str, *keyword_start;

    while (unlikely(custom_isspace(*s))) s++;
    keyword_start = s;

    while (likely(*s && !custom_isspace(*s)) && max_size > 0) {
        *keyword++ = *s++;
        max_size--;
    }
    *keyword = '\0';
    return max_size == 0 ? 0 : (int) (s - keyword_start);
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

INCREMENTAL_PARSER *parser_init(RRDHOST *host, void *user, void *input, PARSER_INPUT_TYPE flags)
{
    INCREMENTAL_PARSER *working_parser;

    // If no parsing input flags, assume SPLIT only
    if (unlikely(!(flags & (PARSER_INPUT_SPLIT | PARSER_INPUT_ORIGINAL))))
        flags |= PARSER_INPUT_SPLIT;

    working_parser = callocz(1, sizeof(*working_parser));

    if (unlikely(!working_parser))
        return NULL;

    working_parser->user  = user;
    working_parser->input = input;
    working_parser->flags = flags;
    working_parser->host  = host;
    //working_parser->unknown_function = NULL;
#ifdef ENABLE_HTTPS
    working_parser->bytesleft = 0;
    working_parser->readfrom  = NULL;
#endif

    return working_parser;
}


/*
 * Push a new line into the parsing stream
 *
 * This line will be the next one to process ie the next fetch will get this one
 *
 */

int parser_push(INCREMENTAL_PARSER *working_parser, char *line)
{
    PARSER_DATA    *tmp_parser_data;
    
    if (unlikely(!working_parser))
        return 1;

    if (unlikely(!line))
        return 0;

    tmp_parser_data = callocz(1, sizeof(*tmp_parser_data));
    tmp_parser_data->line = strdupz(line);
    tmp_parser_data->next = working_parser->data;
    working_parser->data = tmp_parser_data;

    return 0;
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

int parser_add_keyword(INCREMENTAL_PARSER *working_parser, char *keyword, keyword_function func)
{
    PARSER_KEYWORD  *tmp_keyword;

    if (strcmp(keyword, "_read") == 0) {
        working_parser->read_function = (void *) func;
        return 0;
    }

    if (strcmp(keyword, "_eof") == 0) {
        working_parser->eof_function = (void *) func;
        return 0;
    }

    if (strcmp(keyword, "_unknown") == 0) {
        working_parser->unknown_function = (void *) func;
        return 0;
    }

    uint32_t    keyword_hash = simple_hash(keyword);

    tmp_keyword = working_parser->keyword;

    while (tmp_keyword) {
        if (tmp_keyword->keyword_hash == keyword_hash && (!strcmp(tmp_keyword->keyword, keyword))) {
                if (tmp_keyword->func_no == PARSER_MAX_CALLBACKS)
                    return 0;
                tmp_keyword->func[tmp_keyword->func_no++] = (void *) func;
                return tmp_keyword->func_no;
        }
        tmp_keyword = tmp_keyword->next;
    }

    tmp_keyword = callocz(1, sizeof(*tmp_keyword));

    tmp_keyword->keyword = strdupz(keyword);
    tmp_keyword->keyword_hash = keyword_hash;
    tmp_keyword->func[tmp_keyword->func_no++] = (void *) func;

    tmp_keyword->next = working_parser->keyword;
    working_parser->keyword = tmp_keyword;
    return tmp_keyword->func_no;
}

/*
 * Cleanup a previously allocated parser
 */

void parser_destroy(INCREMENTAL_PARSER *working_parser)
{
    if (unlikely(!working_parser))
        return;

    PARSER_KEYWORD  *tmp_keyword, *tmp_keyword_next;
    PARSER_DATA     *tmp_parser_data, *tmp_parser_data_next;
    
    // Remove keywords
    tmp_keyword = working_parser->keyword;
    while (tmp_keyword) {
        tmp_keyword_next = tmp_keyword->next;
        freez(tmp_keyword->keyword);
        freez(tmp_keyword);
        tmp_keyword =  tmp_keyword_next;
    }
    
    // Remove pushed data if any
    tmp_parser_data = working_parser->data;
    while (tmp_parser_data) {
        tmp_parser_data_next = tmp_parser_data->next;
        freez(tmp_parser_data->line);
        freez(tmp_parser_data);
        tmp_parser_data =  tmp_parser_data_next;
    }

    freez(working_parser);
    return;
}


/*
 * Fetch the next line to process
 *
 */

int parser_next(INCREMENTAL_PARSER *working_parser)
{
    char    *tmp = NULL;

    if (unlikely(!working_parser))
        return 1;

    working_parser->flags &= ~(PARSER_INPUT_PROCESSED);

    PARSER_DATA  *tmp_parser_data = working_parser->data;

    if (unlikely(tmp_parser_data)) {
        strncpyz(working_parser->buffer, tmp_parser_data->line, PLUGINSD_LINE_MAX);
        working_parser->data = tmp_parser_data->next;
        freez(tmp_parser_data->line);
        freez(tmp_parser_data);
        return 0;
    }

#ifdef ENABLE_HTTPS
    int normalread = 1;
    if (netdata_srv_ctx) {
        if (working_parser->host->stream_ssl.conn && !working_parser->host->stream_ssl.flags) {
            tmp = working_parser->buffer;
            if (!working_parser->bytesleft) {
                working_parser->readfrom = working_parser->tmpbuffer;
                working_parser->bytesleft =
                    pluginsd_update_buffer(working_parser->readfrom, working_parser->host->stream_ssl.conn);
                if (working_parser->bytesleft <= 0) {
                    return 1;
                }
            }

            working_parser->readfrom = pluginsd_get_from_buffer(
                working_parser->buffer, &working_parser->bytesleft, working_parser->readfrom,
                working_parser->host->stream_ssl.conn, working_parser->tmpbuffer);

            if (!working_parser->readfrom)
                tmp = NULL;

            normalread = 0;
        }
    }
    if (normalread) {
        if (unlikely(working_parser->read_function))
            tmp = working_parser->read_function(working_parser->buffer, PLUGINSD_LINE_MAX, working_parser->input);
        else
            tmp = fgets(working_parser->buffer, PLUGINSD_LINE_MAX, (FILE *)working_parser->input);
    }
#else
    if (unlikely(working_parser->read_function))
        tmp = working_parser->read_function(working_parser->buffer, PLUGINSD_LINE_MAX, working_parser->input);
    else
        tmp = fgets(working_parser->buffer, PLUGINSD_LINE_MAX, (FILE *)working_parser->input);
#endif

    if (unlikely(!tmp)) {
        if (unlikely(working_parser->eof_function)) {
            int rc = working_parser->eof_function(working_parser->input);
            error("read failed: user defined function returned %d", rc);
        }
        else {
            if (feof((FILE *)working_parser->input))
                error("read failed: end of file");
            else if (ferror((FILE *)working_parser->input))
                error("read failed: input error");
            else
                error("read failed: unknown error");
        }
        return 1;
    }
    return 0;
}


/*
* Takes an initialized parser object that has an unprocessed entry (by calling parser_next)
* and if it contains a valid keyword, it will execute all the callbacks
*
*/

inline int parser_action(INCREMENTAL_PARSER *working_parser)
{
    PARSER_RC   rc = PARSER_RC_OK;
    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    char command[PLUGINSD_LINE_MAX];
    keyword_function action_function;
    keyword_function *action_function_list = NULL;
    char *local_copy = NULL; 

    if (unlikely(!working_parser))
        return 1;

    if (unlikely(working_parser->flags & PARSER_INPUT_PROCESSED))
        return 0;

    PARSER_KEYWORD  *tmp_keyword = working_parser->keyword;
    if (unlikely(!tmp_keyword)) {
        return 1;
    }

    int w = find_keyword(working_parser->buffer, command, PLUGINSD_LINE_MAX, pluginsd_space);
    if (w == 0) {
        return 1;
    }

    if ((working_parser->flags & PARSER_INPUT_FULL) == PARSER_INPUT_FULL) {
        local_copy = strdupz(working_parser->buffer);
        pluginsd_split_words(working_parser->buffer, words, PLUGINSD_MAX_WORDS);
        words[0] = local_copy;
    }
    else {
        local_copy = NULL;
        if (working_parser->flags & PARSER_INPUT_SPLIT) {
            pluginsd_split_words(working_parser->buffer, words, PLUGINSD_MAX_WORDS);
        }
        else {
            words[0] = working_parser->buffer;
            words[1] = NULL;
        }
    }

    uint32_t command_hash = simple_hash(command);

    while(tmp_keyword) {
        if (command_hash == tmp_keyword->keyword_hash &&
                (!strcmp(command, tmp_keyword->keyword))) {
                    action_function_list = &tmp_keyword->func[0];
                    break;
        }
        tmp_keyword = tmp_keyword->next;
    }

    if (unlikely(!action_function_list)) {
        if (unlikely(working_parser->unknown_function))
            rc = working_parser->unknown_function(words, working_parser->user);
        else
            rc = PARSER_RC_ERROR;
#ifdef NETDATA_INTERNAL_CHECKS
        error("Unknown keyword [%s]", words[0]);
#endif
    }
    else {
        while ((action_function = *action_function_list) != NULL) {
                rc = action_function(words, working_parser->user);
                if (unlikely(rc == PARSER_RC_ERROR || rc == PARSER_RC_STOP))
                    break;                
                action_function_list++;
        }
    }
    if (local_copy)
        freez(local_copy);

    working_parser->flags |= PARSER_INPUT_PROCESSED;

    return (rc == PARSER_RC_ERROR);
}
