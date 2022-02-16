// SPDX-License-Identifier: GPL-3.0-or-later

#include "parser.h"

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

PARSER *parser_init(RRDHOST *host, void *user, void *input, PARSER_INPUT_TYPE flags)
{
    PARSER *parser;

    parser = callocz(1, sizeof(*parser));

    if (unlikely(!parser))
        return NULL;

    parser->plugins_action = callocz(1, sizeof(PLUGINSD_ACTION));
    if (unlikely(!parser->plugins_action)) {
        freez(parser);
        return NULL;
    }

    parser->user = user;
    parser->input = input;
    parser->flags = flags;
    parser->host = host;
#ifdef ENABLE_HTTPS
    parser->bytesleft = 0;
    parser->readfrom = NULL;
#endif

    if (unlikely(!(flags & PARSER_NO_PARSE_INIT))) {
        int rc = parser_add_keyword(parser, PLUGINSD_KEYWORD_FLUSH, pluginsd_flush);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_CHART, pluginsd_chart);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_DIMENSION, pluginsd_dimension);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_DISABLE, pluginsd_disable);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_VARIABLE, pluginsd_variable);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_LABEL, pluginsd_label);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_OVERWRITE, pluginsd_overwrite);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_END, pluginsd_end);
        rc += parser_add_keyword(parser, "CLABEL_COMMIT", pluginsd_clabel_commit);
        rc += parser_add_keyword(parser, "CLABEL", pluginsd_clabel);
        rc += parser_add_keyword(parser, PLUGINSD_KEYWORD_BEGIN, pluginsd_begin);
        rc += parser_add_keyword(parser, "SET", pluginsd_set);
    }

    return parser;
}


/*
 * Push a new line into the parsing stream
 *
 * This line will be the next one to process ie the next fetch will get this one
 *
 */

int parser_push(PARSER *parser, char *line)
{
    PARSER_DATA    *tmp_parser_data;
    
    if (unlikely(!parser))
        return 1;

    if (unlikely(!line))
        return 0;

    tmp_parser_data = callocz(1, sizeof(*tmp_parser_data));
    tmp_parser_data->line = strdupz(line);
    tmp_parser_data->next = parser->data;
    parser->data = tmp_parser_data;

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

int parser_add_keyword(PARSER *parser, char *keyword, keyword_function func)
{
    PARSER_KEYWORD  *tmp_keyword;

    if (strcmp(keyword, "_read") == 0) {
        parser->read_function = (void *) func;
        return 0;
    }

    if (strcmp(keyword, "_eof") == 0) {
        parser->eof_function = (void *) func;
        return 0;
    }

    if (strcmp(keyword, "_unknown") == 0) {
        parser->unknown_function = (void *) func;
        return 0;
    }

    uint32_t    keyword_hash = simple_hash(keyword);

    tmp_keyword = parser->keyword;

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

    tmp_keyword->next = parser->keyword;
    parser->keyword = tmp_keyword;
    return tmp_keyword->func_no;
}

/*
 * Cleanup a previously allocated parser
 */

void parser_destroy(PARSER *parser)
{
    if (unlikely(!parser))
        return;

    PARSER_KEYWORD  *tmp_keyword, *tmp_keyword_next;
    PARSER_DATA     *tmp_parser_data, *tmp_parser_data_next;
    
    // Remove keywords
    tmp_keyword = parser->keyword;
    while (tmp_keyword) {
        tmp_keyword_next = tmp_keyword->next;
        freez(tmp_keyword->keyword);
        freez(tmp_keyword);
        tmp_keyword =  tmp_keyword_next;
    }
    
    // Remove pushed data if any
    tmp_parser_data = parser->data;
    while (tmp_parser_data) {
        tmp_parser_data_next = tmp_parser_data->next;
        freez(tmp_parser_data->line);
        freez(tmp_parser_data);
        tmp_parser_data =  tmp_parser_data_next;
    }

    freez(parser->plugins_action);

    freez(parser);
    return;
}


/*
 * Fetch the next line to process
 *
 */

int parser_next(PARSER *parser)
{
    char    *tmp = NULL;

    if (unlikely(!parser))
        return 1;

    parser->flags &= ~(PARSER_INPUT_PROCESSED);

    PARSER_DATA  *tmp_parser_data = parser->data;

    if (unlikely(tmp_parser_data)) {
        strncpyz(parser->buffer, tmp_parser_data->line, PLUGINSD_LINE_MAX);
        parser->data = tmp_parser_data->next;
        freez(tmp_parser_data->line);
        freez(tmp_parser_data);
        return 0;
    }

    if (unlikely(parser->read_function))
        tmp = parser->read_function(parser->buffer, PLUGINSD_LINE_MAX, parser->input);
    else
        tmp = fgets(parser->buffer, PLUGINSD_LINE_MAX, (FILE *)parser->input);

    if (unlikely(!tmp)) {
        if (unlikely(parser->eof_function)) {
            int rc = parser->eof_function(parser->input);
            error("read failed: user defined function returned %d", rc);
        }
        else {
            if (feof((FILE *)parser->input))
                error("read failed: end of file");
            else if (ferror((FILE *)parser->input))
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

inline int parser_action(PARSER *parser, char *input)
{
    PARSER_RC   rc = PARSER_RC_OK;
    char *words[PLUGINSD_MAX_WORDS] = { NULL };
    char command[PLUGINSD_LINE_MAX];
    keyword_function action_function;
    keyword_function *action_function_list = NULL;

    if (unlikely(!parser))
        return 1;
    parser->recover_location[0] = 0x0;

    // if not direct input check if we have reprocessed this
    if (unlikely(!input && parser->flags & PARSER_INPUT_PROCESSED))
        return 0;

    PARSER_KEYWORD  *tmp_keyword = parser->keyword;
    if (unlikely(!tmp_keyword)) {
        return 1;
    }

    if (unlikely(!input))
        input = parser->buffer;

    if (unlikely(!find_keyword(input, command, PLUGINSD_LINE_MAX, pluginsd_space)))
        return 0;

    if ((parser->flags & PARSER_INPUT_ORIGINAL) == PARSER_INPUT_ORIGINAL)
        pluginsd_split_words(input, words, PLUGINSD_MAX_WORDS, parser->recover_input, parser->recover_location, PARSER_MAX_RECOVER_KEYWORDS);
    else
        pluginsd_split_words(input, words, PLUGINSD_MAX_WORDS, NULL, NULL, 0);

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
        if (unlikely(parser->unknown_function))
            rc = parser->unknown_function(words, parser->user, NULL);
        else
            rc = PARSER_RC_ERROR;
#ifdef NETDATA_INTERNAL_CHECKS
        error("Unknown keyword [%s]", input);
#endif
    }
    else {
        while ((action_function = *action_function_list) != NULL) {
                rc = action_function(words, parser->user, parser->plugins_action);
                if (unlikely(rc == PARSER_RC_ERROR || rc == PARSER_RC_STOP))
                    break;                
                action_function_list++;
        }
    }

    if (likely(input == parser->buffer))
        parser->flags |= PARSER_INPUT_PROCESSED;

    return (rc == PARSER_RC_ERROR);
}

inline int parser_recover_input(PARSER *parser)
{
    if (unlikely(!parser))
        return 1;

    for(int i=0; i < PARSER_MAX_RECOVER_KEYWORDS && parser->recover_location[i]; i++)
        *(parser->recover_location[i]) = parser->recover_input[i];

    parser->recover_location[0] = 0x0;

    return 0;
}
