#include "jsmn.h"
#include "../libnetdata.h"

#define JSON_TOKENS 1024

int json_tokens = JSON_TOKENS;

/**
 * Json Tokenise
 *
 * Map the string given inside tokens.
 *
 * @param js is the string used to create the tokens
 * @param len is the string length
 * @param count the number of tokens present in the string
 *
 * @return it returns the json parsed in tokens
 */
#ifdef ENABLE_JSONC
json_object *json_tokenise(char *js) {
    if(!js) {
        error("JSON: json string is empty.");
        return NULL;
    }

    json_object *token = json_tokener_parse(js);
    if(!token) {
        error("JSON: Invalid json string.");
        return NULL;
    }

    return token;
}
#else
jsmntok_t *json_tokenise(char *js, size_t len, size_t *count)
{
    int n = json_tokens;
    if(!js || !len) {
        error("JSON: json string is empty.");
        return NULL;
    }

    jsmn_parser parser;
    jsmn_init(&parser);

    jsmntok_t *tokens = mallocz(sizeof(jsmntok_t) * n);
    if(!tokens) return NULL;

    int ret = jsmn_parse(&parser, js, len, tokens, n);
    while (ret == JSMN_ERROR_NOMEM) {
        n *= 2;
        jsmntok_t *new = reallocz(tokens, sizeof(jsmntok_t) * n);
        if(!new) {
            freez(tokens);
            return NULL;
        }
        tokens = new;
        ret = jsmn_parse(&parser, js, len, tokens, n);
    }

    if (ret == JSMN_ERROR_INVAL) {
        error("JSON: Invalid json string.");
        freez(tokens);
        return NULL;
    }
    else if (ret == JSMN_ERROR_PART) {
        error("JSON: Truncated JSON string.");
        freez(tokens);
        return NULL;
    }

    if(count) *count = (size_t)ret;

    if(json_tokens < n) json_tokens = n;
    return tokens;
}
#endif

/**
 * Callback Print
 *
 * Set callback print case necesary and wrinte an information inside a buffer to write in the log.
 *
 * @param e a pointer for a structure that has the complete information about json structure.
 *
 * @return It always return 0
 */
int json_callback_print(JSON_ENTRY *e)
{
    BUFFER *wb=buffer_create(300);

    buffer_sprintf(wb,"%s = ", e->name);
    char txt[50];
    switch(e->type) {
        case JSON_OBJECT:
            e->callback_function = json_callback_print;
            buffer_strcat(wb,"OBJECT");
            break;

        case JSON_ARRAY:
            e->callback_function = json_callback_print;
            sprintf(txt,"ARRAY[%lu]", e->data.items);
            buffer_strcat(wb, txt);
            break;

        case JSON_STRING:
            buffer_strcat(wb, e->data.string);
            break;

        case JSON_NUMBER:
            sprintf(txt,"%Lf", e->data.number);
            buffer_strcat(wb,txt);

            break;

        case JSON_BOOLEAN:
            buffer_strcat(wb, e->data.boolean?"TRUE":"FALSE");
            break;

        case JSON_NULL:
            buffer_strcat(wb,"NULL");
            break;
    }
    info("JSON: %s", buffer_tostring(wb));
    buffer_free(wb);
    return 0;
}

/**
 * Walk string
 *
 * Set JSON_ENTRY to string and map the values from jsmntok_t.
 *
 * @param js the original string
 * @param t the tokens
 * @param start the first position
 * @param e the output structure.
 *
 * @return It always return 1
 */
size_t json_walk_string(char *js, jsmntok_t *t, size_t start, JSON_ENTRY *e)
{
    char old = js[t[start].end];
    js[t[start].end] = '\0';
    e->original_string = &js[t[start].start];

    e->type = JSON_STRING;
    e->data.string = e->original_string;
    if(e->callback_function) e->callback_function(e);
    js[t[start].end] = old;
    return 1;
}

/**
 * Walk Primitive
 *
 * Define the data type of the string
 *
 * @param js the original string
 * @param t the tokens
 * @param start the first position
 * @param e the output structure.
 *
 * @return It always return 1
 */
size_t json_walk_primitive(char *js, jsmntok_t *t, size_t start, JSON_ENTRY *e)
{
    char old = js[t[start].end];
    js[t[start].end] = '\0';
    e->original_string = &js[t[start].start];

    switch(e->original_string[0]) {
        case '0': case '1': case '2': case '3': case '4': case '5': case '6': case '7':
        case '8': case '9': case '-': case '.':
            e->type = JSON_NUMBER;
            e->data.number = strtold(e->original_string, NULL);
            break;

        case 't': case 'T':
            e->type = JSON_BOOLEAN;
            e->data.boolean = 1;
            break;

        case 'f': case 'F':
            e->type = JSON_BOOLEAN;
            e->data.boolean = 0;
            break;

        case 'n': case 'N':
        default:
            e->type = JSON_NULL;
            break;
    }
    if(e->callback_function) e->callback_function(e);
    js[t[start].end] = old;
    return 1;
}

/**
 * Array
 *
 * Measure the array length
 *
 * @param js the original string
 * @param t the tokens
 * @param nest the length of structure t
 * @param start the first position
 * @param e the output structure.
 *
 * @return It returns the array length
 */
size_t json_walk_array(char *js, jsmntok_t *t, size_t nest, size_t start, JSON_ENTRY *e)
{
    JSON_ENTRY ne = {
            .name = "",
            .fullname = "",
            .callback_data = NULL,
            .callback_function = NULL
    };

    char old = js[t[start].end];
    js[t[start].end] = '\0';
    ne.original_string = &js[t[start].start];

    memcpy(&ne, e, sizeof(JSON_ENTRY));
    ne.type = JSON_ARRAY;
    ne.data.items = t[start].size;
    ne.callback_function = NULL;
    ne.name[0]='\0';
    ne.fullname[0]='\0';
    if(e->callback_function) e->callback_function(&ne);
    js[t[start].end] = old;

    size_t i, init = start, size = t[start].size;

    start++;
    for(i = 0; i < size ; i++) {
        ne.pos = i;
        if (!e->name || !e->fullname || strlen(e->name) > JSON_NAME_LEN  - 24 || strlen(e->fullname) > JSON_FULLNAME_LEN -24) {
            info("JSON: JSON walk_array ignoring element with name:%s fullname:%s",e->name, e->fullname);
            continue;
        }
        sprintf(ne.name, "%s[%lu]", e->name, i);
        sprintf(ne.fullname, "%s[%lu]", e->fullname, i);

        switch(t[start].type) {
            case JSMN_PRIMITIVE:
                start += json_walk_primitive(js, t, start, &ne);
                break;

            case JSMN_OBJECT:
                start += json_walk_object(js, t, nest + 1, start, &ne);
                break;

            case JSMN_ARRAY:
                start += json_walk_array(js, t, nest + 1, start, &ne);
                break;

            case JSMN_STRING:
                start += json_walk_string(js, t, start, &ne);
                break;
        }
    }
    return start - init;
}

/**
 * Object
 *
 * Measure the Object length
 *
 * @param js the original string
 * @param t the tokens
 * @param nest the length of structure t
 * @param start the first position
 * @param e the output structure.
 *
 * @return It returns the Object length
 */
size_t json_walk_object(char *js, jsmntok_t *t, size_t nest, size_t start, JSON_ENTRY *e)
{
    JSON_ENTRY ne = {
            .name = "",
            .fullname = "",
            .callback_data = NULL,
            .callback_function = NULL
    };

    char old = js[t[start].end];
    js[t[start].end] = '\0';
    ne.original_string = &js[t[start].start];
    memcpy(&ne, e, sizeof(JSON_ENTRY));
    ne.type = JSON_OBJECT;
    ne.callback_function = NULL;
    if(e->callback_function) e->callback_function(&ne);
    js[t[start].end] = old;

    int key = 1;
    size_t i, init = start, size = t[start].size;

    start++;
    for(i = 0; i < size ; i++) {
        switch(t[start].type) {
            case JSMN_PRIMITIVE:
                start += json_walk_primitive(js, t, start, &ne);
                key = 1;
                break;

            case JSMN_OBJECT:
                start += json_walk_object(js, t, nest + 1, start, &ne);
                key = 1;
                break;

            case JSMN_ARRAY:
                start += json_walk_array(js, t, nest + 1, start, &ne);
                key = 1;
                break;

            case JSMN_STRING:
            default:
                if(key) {
                    int len = t[start].end - t[start].start;
                    if (unlikely(len>JSON_NAME_LEN)) len=JSON_NAME_LEN;
                    strncpy(ne.name, &js[t[start].start], len);
                    ne.name[len] = '\0';
                    len=strlen(e->fullname) + strlen(e->fullname[0]?".":"") + strlen(ne.name);
                    char *c = mallocz((len+1)*sizeof(char));
                    sprintf(c,"%s%s%s", e->fullname, e->fullname[0]?".":"", ne.name);
                    if (unlikely(len>JSON_FULLNAME_LEN)) len=JSON_FULLNAME_LEN;
                    strncpy(ne.fullname, c, len);
                    freez(c);
                    start++;
                    key = 0;
                }
                else {
                    start += json_walk_string(js, t, start, &ne);
                    key = 1;
                }
                break;
        }
    }
    return start - init;
}

/**
 * Tree
 *
 * Call the correct walk function according its type.
 *
 * @param js the original string
 * @param t the tokens
 * @param callback_data
 * @param callback_function
 *
 * @return It always return 1
 */
#ifdef ENABLE_JSONC
size_t json_walk(json_object *t, void *callback_data, int (*callback_function)(struct json_entry *)) {
    JSON_ENTRY e = {
            .name = "",
            .fullname = "",
            .callback_data = callback_data,
            .callback_function = callback_function
    };

    enum json_type type;
    json_object_object_foreach(t, key, val) {
        type = json_object_get_type(val);
        if (type == json_type_array) {
            e.type = JSON_ARRAY;
        } else if (type == json_type_object) {
            e.type = JSON_OBJECT;
        }
    }

    return 1;
}
#else
size_t json_walk_tree(char *js, jsmntok_t *t, void *callback_data, int (*callback_function)(struct json_entry *))
{
    JSON_ENTRY e = {
            .name = "",
            .fullname = "",
            .callback_data = callback_data,
            .callback_function = callback_function
    };

    switch (t[0].type) {
        case JSMN_OBJECT:
            e.type = JSON_OBJECT;
            json_walk_object(js, t, 0, 0, &e);
            break;

        case JSMN_ARRAY:
            e.type = JSON_ARRAY;
            json_walk_array(js, t, 0, 0, &e);
            break;

        case JSMN_PRIMITIVE:
        case JSMN_STRING:
            break;
    }

    return 1;
}
#endif

/**
 * JSON Parse
 *
 * Parse the json message with the callback function
 *
 * @param js the string that the callback function will parse
 * @param callback_data additional data to be used together the callback function
 * @param callback_function function used to create a silencer.
 *
 * @return JSON_OK  case everything happend as expected, JSON_CANNOT_PARSE case there were errors in the
 * parsing procces and JSON_CANNOT_DOWNLOAD case the string given(js) is NULL.
 */
int json_parse(char *js, void *callback_data, int (*callback_function)(JSON_ENTRY *))
{
    size_t count;
    if(js) {
#ifdef ENABLE_JSONC
        json_object *tokens = json_tokenise(js);
#else
        jsmntok_t *tokens = json_tokenise(js, strlen(js), &count);
#endif
        if(tokens) {
#ifdef ENABLE_JSONC
            json_walk(tokens, callback_data, callback_function);
            json_object_put(tokens);
#else
            json_walk_tree(js, tokens, callback_data, callback_function);
            freez(tokens);
#endif
            return JSON_OK;
        }

        return JSON_CANNOT_PARSE;
    }

    return JSON_CANNOT_DOWNLOAD;
}

/*
int json_test(char *str)
{
    return json_parse(str, NULL, json_callback_print);
}
 */