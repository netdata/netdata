#include "libnetdata/libnetdata.h"
#include "json.h"

#ifndef ENABLE_JSONC
#include "jsmn.h"
#endif

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
        netdata_log_error("JSON: json string is empty.");
        return NULL;
    }

    json_object *token = json_tokener_parse(js);
    if(!token) {
        netdata_log_error("JSON: Invalid json string.");
        return NULL;
    }

    return token;
}
#else
jsmntok_t *json_tokenise(char *js, size_t len, size_t *count)
{
    int n = __atomic_load_n(&json_tokens, __ATOMIC_RELAXED);
    if(!js || !len) {
        netdata_log_error("JSON: json string is empty.");
        return NULL;
    }

    jsmn_parser parser;
    jsmn_init(&parser);

    jsmntok_t *tokens = mallocz(sizeof(jsmntok_t) * n);
    if(!tokens) return NULL;

    int ret = jsmn_parse(&parser, js, len, tokens, n);
    while (ret == JSMN_ERROR_NOMEM) {
        if(unlikely(n > INT_MAX / 2 || (size_t)n > SIZE_MAX / sizeof(*tokens) / 2)) {
            freez(tokens);
            return NULL;
        }

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
        netdata_log_error("JSON: Invalid json string.");
        freez(tokens);
        return NULL;
    }
    else if (ret == JSMN_ERROR_PART) {
        netdata_log_error("JSON: Truncated JSON string.");
        freez(tokens);
        return NULL;
    }

    if(count) *count = (size_t)ret;

    int cached = __atomic_load_n(&json_tokens, __ATOMIC_RELAXED);
    while(cached < n && !__atomic_compare_exchange_n(
              &json_tokens, &cached, n, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
        ;

    return tokens;
}
#endif

/**
 * Callback Print
 *
 * Set callback print case necessary and wrinte an information inside a buffer to write in the log.
 *
 * @param e a pointer for a structure that has the complete information about json structure.
 *
 * @return It always return 0
 */
int json_callback_print(JSON_ENTRY *e)
{
    BUFFER *wb=buffer_create(300, NULL);

    buffer_sprintf(wb,"%s = ", e->name);
    char txt[50];
    switch(e->type) {
        case JSON_OBJECT:
            e->callback_function = json_callback_print;
            buffer_strcat(wb,"OBJECT");
            break;

        case JSON_ARRAY:
            e->callback_function = json_callback_print;
            snprintfz(txt, sizeof(txt), "ARRAY[%lu]", (long unsigned int) e->data.items);
            buffer_strcat(wb, txt);
            break;

        case JSON_STRING:
            buffer_strcat(wb, e->data.string);
            break;

        case JSON_NUMBER:
            buffer_print_netdata_double(wb, e->data.number);
            break;

        case JSON_BOOLEAN:
            buffer_strcat(wb, e->data.boolean?"TRUE":"FALSE");
            break;

        case JSON_NULL:
            buffer_strcat(wb,"NULL");
            break;
    }
    netdata_log_info("JSON: %s", buffer_tostring(wb));
    buffer_free(wb);
    return 0;
}

/**
 * JSONC Set String
 *
 * Set the string value of the structure JSON_ENTRY.
 *
 * @param e the output structure
 */
static inline void json_jsonc_set_string(JSON_ENTRY *e,char *key,const char *value) {
    size_t len = strlen(key);
    if(len > JSON_NAME_LEN)
        len = JSON_NAME_LEN;
    e->type = JSON_STRING;
    memcpy(e->name,key,len);
    e->name[len] = 0x00;
    e->data.string = (char *) value;
}


#ifdef ENABLE_JSONC
/**
 * JSONC set Boolean
 *
 * Set the boolean value of the structure JSON_ENTRY
 *
 * @param e the output structure
 * @param value the input value
 */
static inline void json_jsonc_set_boolean(JSON_ENTRY *e,char *key,int value) {
    size_t len = key ? strnlen(key, JSON_NAME_LEN) : 0;
    e->type = JSON_BOOLEAN;
    if(len)
        memcpy(e->name,key,len);
    e->name[len] = 0x00;
    e->data.boolean = value;
}

static inline void json_jsonc_set_integer(JSON_ENTRY *e, char *key, int64_t value) {
    size_t len = strlen(key);
    if(len > JSON_NAME_LEN)
        len = JSON_NAME_LEN;
    e->type = JSON_NUMBER;
    memcpy(e->name, key, len);
    e->name[len] = 0;
    e->data.number = (NETDATA_DOUBLE)value;
}

/**
 * Parse Array
 *
 * Parse the array object.
 *
 * @param ptr the pointer for the object that we will parse.
 * @param callback_data additional data to be used together the callback function
 * @param callback_function function used to create a silencer.
 */
static inline void json_jsonc_parse_array(json_object *ptr, void *callback_data,int (*callback_function)(struct json_entry *)) {
    size_t end = json_object_array_length(ptr);

    if(end) {
        size_t i;
        i = 0;

        enum json_type type;
        do {
            json_object *jvalue =  json_object_array_get_idx(ptr, i);
            if(jvalue) {
                JSON_ENTRY e = { .type = JSON_OBJECT, .callback_data = callback_data };
                callback_function(&e);
                json_object_object_foreach(jvalue, key, val) {
                    type = json_object_get_type(val);
                    if (type == json_type_array) {
                        e.type = JSON_ARRAY;
                        json_jsonc_parse_array(val, callback_data, callback_function);
                    } else if (type == json_type_object) {
                        json_walk(val,callback_data,callback_function);
                    } else if (type == json_type_string) {
                        json_jsonc_set_string(&e,key,json_object_get_string(val));
                        callback_function(&e);
                    } else if (type == json_type_boolean) {
                        json_jsonc_set_boolean(&e,key,json_object_get_boolean(val));
                        callback_function(&e);
                    }
                }
            }

        } while (++i < end);
    }
}
#else

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

static size_t json_walk_token_span(jsmntok_t *t, size_t start)
{
    size_t span = 1;

    switch(t[start].type) {
        case JSMN_OBJECT:
        case JSMN_ARRAY:
            for(int i = 0; i < t[start].size; i++)
                span += json_walk_token_span(t, start + span);
            break;

        case JSMN_PRIMITIVE:
        case JSMN_STRING:
        default:
            break;
    }

    return span;
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
 * @param e the structure with values and callback to be used inside the function.
 *
 * @return It returns the array length
 */
size_t json_walk_array(char *js, jsmntok_t *t, size_t nest, size_t start, JSON_ENTRY *e)
{
    JSON_ENTRY ne;

    char old = js[t[start].end];
    js[t[start].end] = '\0';
    ne.original_string = &js[t[start].start];

    memcpy(&ne, e, sizeof(JSON_ENTRY));
    ne.type = JSON_ARRAY;
    ne.data.items = t[start].size;
    ne.callback_function = e->callback_function;
    ne.name[0]='\0';
    ne.fullname[0]='\0';
    if(e->callback_function) e->callback_function(&ne);
    js[t[start].end] = old;

    size_t i, init = start, size = t[start].size;

    start++;
    for(i = 0; i < size ; i++) {
        ne.pos = i;
        if (strlen(e->name) > JSON_NAME_LEN  - 24 || strlen(e->fullname) > JSON_FULLNAME_LEN -24) {
            netdata_log_info("JSON: JSON walk_array ignoring element with name:%s fullname:%s",e->name, e->fullname);
            start += json_walk_token_span(t, start);
            continue;
        }
        snprintfz(ne.name, JSON_NAME_LEN, "%s[%lu]", e->name, i);
        snprintfz(ne.fullname, JSON_FULLNAME_LEN, "%s[%lu]", e->fullname, i);

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
    ne.callback_function = e->callback_function;
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
                    ne.fullname[len] = '\0';
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
#endif

/**
 * Tree
 *
 * Call the correct walk function according its type.
 *
 * @param t the json object to work
 * @param callback_data additional data to be used together the callback function
 * @param callback_function function used to create a silencer.
 *
 * @return It always return 1
 */
#ifdef ENABLE_JSONC
size_t json_walk(json_object *t, void *callback_data, int (*callback_function)(struct json_entry *)) {
    if (!t || json_object_get_type(t) != json_type_object)
        return 0;

    JSON_ENTRY e = { 0 };
    e.callback_data = callback_data;
    enum json_type type;
    json_object_object_foreach(t, key, val) {
        type = json_object_get_type(val);
        if (type == json_type_array) {
            e.type = JSON_ARRAY;
            json_jsonc_parse_array(val,callback_data,callback_function);
        } else if (type == json_type_object) {
            e.type = JSON_OBJECT;
        } else if (type == json_type_string) {
            json_jsonc_set_string(&e,key,json_object_get_string(val));
            callback_function(&e);
        } else if (type == json_type_boolean) {
            json_jsonc_set_boolean(&e,key,json_object_get_boolean(val));
            callback_function(&e);
        } else if (type == json_type_int) {
            json_jsonc_set_integer(&e,key,json_object_get_int64(val));
            callback_function(&e);
        }
    }

    return 1;
}
#else
/**
 * Tree
 *
 * Call the correct walk function according its type.
 *
 * @param js the original string
 * @param t the tokens
 * @param callback_data additional data to be used together the callback function
 * @param callback_function function used to create a silencer.
 *
 * @return It always return 1
 */
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
 * @return JSON_OK  case everything happened as expected, JSON_CANNOT_PARSE case there were errors in the
 * parsing process and JSON_CANNOT_DOWNLOAD case the string given(js) is NULL.
 */
int json_parse(char *js, void *callback_data, int (*callback_function)(JSON_ENTRY *))
{
    if(js) {
#ifdef ENABLE_JSONC
        json_object *tokens = json_tokenise(js);
#else
        size_t count;
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


// ----------------------------------------------------------------------------
// strict RFC 8259 validation

#define JSON_STRICT_MAX_DEPTH 256

static inline const char *json_strict_skip_ws(const char *s, const char *e) {
    while(s < e && (*s == ' ' || *s == '\t' || *s == '\n' || *s == '\r'))
        s++;
    return s;
}

static inline bool json_strict_is_digit(char c) {
    return c >= '0' && c <= '9';
}

static inline bool json_strict_is_hex(char c) {
    return json_strict_is_digit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
}

// s points at the opening quote; returns the byte after the closing quote, or NULL
static const char *json_strict_string(const char *s, const char *e) {
    for(s++; s < e; ) {
        unsigned char c = (unsigned char)*s;

        if(c == '"')
            return s + 1;

        if(c < 0x20)
            return NULL;

        if(c != '\\') {
            s++;
            continue;
        }

        if(++s >= e)
            return NULL;

        switch(*s) {
            case '"': case '\\': case '/': case 'b': case 'f': case 'n': case 'r': case 't':
                s++;
                break;

            case 'u':
                if(e - s < 5)
                    return NULL;
                for(int i = 1; i <= 4; i++)
                    if(!json_strict_is_hex(s[i]))
                        return NULL;
                s += 5;
                break;

            default:
                return NULL;
        }
    }

    return NULL;
}

static const char *json_strict_number(const char *s, const char *e) {
    if(s < e && *s == '-')
        s++;

    if(s >= e)
        return NULL;

    if(*s == '0')
        s++;
    else if(*s >= '1' && *s <= '9') {
        while(s < e && json_strict_is_digit(*s))
            s++;
    }
    else
        return NULL;

    if(s < e && *s == '.') {
        if(++s >= e || !json_strict_is_digit(*s))
            return NULL;
        while(s < e && json_strict_is_digit(*s))
            s++;
    }

    if(s < e && (*s == 'e' || *s == 'E')) {
        if(++s < e && (*s == '+' || *s == '-'))
            s++;
        if(s >= e || !json_strict_is_digit(*s))
            return NULL;
        while(s < e && json_strict_is_digit(*s))
            s++;
    }

    return s;
}

static const char *json_strict_literal(const char *s, const char *e, const char *literal, size_t len) {
    if((size_t)(e - s) < len || memcmp(s, literal, len) != 0)
        return NULL;
    return s + len;
}

const char *json_strict_validate(const char *s, size_t len) {
    if(!s)
        return "invalid JSON: no JSON document";

    const char *e = s + len;
    char stack[JSON_STRICT_MAX_DEPTH];
    size_t depth = 0;

    enum { EXPECT_VALUE, EXPECT_MEMBER, AFTER_VALUE } state = EXPECT_VALUE;

    while(true) {
        s = json_strict_skip_ws(s, e);

        switch(state) {
            case EXPECT_MEMBER:
                if(s >= e || *s != '"')
                    return "invalid JSON: expected a double-quoted object key";
                if(!(s = json_strict_string(s, e)))
                    return "invalid JSON: invalid string";
                s = json_strict_skip_ws(s, e);
                if(s >= e || *s != ':')
                    return "invalid JSON: expected ':' after an object key";
                s++;
                state = EXPECT_VALUE;
                break;

            case EXPECT_VALUE:
                if(s >= e)
                    return "invalid JSON: expected a value";

                if(*s == '{' || *s == '[') {
                    if(depth >= JSON_STRICT_MAX_DEPTH)
                        return "invalid JSON: too deeply nested";
                    char open = *s++;
                    stack[depth++] = open;

                    const char *t = json_strict_skip_ws(s, e);
                    if(t < e && *t == (open == '{' ? '}' : ']')) {
                        s = t + 1;
                        depth--;
                        state = AFTER_VALUE;
                    }
                    else
                        state = (open == '{') ? EXPECT_MEMBER : EXPECT_VALUE;
                    break;
                }

                if(*s == '"')
                    s = json_strict_string(s, e);
                else if(*s == '-' || json_strict_is_digit(*s))
                    s = json_strict_number(s, e);
                else if(*s == 't')
                    s = json_strict_literal(s, e, "true", sizeof("true") - 1);
                else if(*s == 'f')
                    s = json_strict_literal(s, e, "false", sizeof("false") - 1);
                else if(*s == 'n')
                    s = json_strict_literal(s, e, "null", sizeof("null") - 1);
                else
                    return "invalid JSON: unexpected character";

                if(!s)
                    return "invalid JSON: invalid value";

                state = AFTER_VALUE;
                break;

            case AFTER_VALUE:
                if(!depth)
                    return (s == e) ? NULL : "invalid JSON: unexpected data after the document";

                if(s >= e)
                    return "invalid JSON: unterminated object or array";

                if(*s == ',') {
                    s++;
                    state = (stack[depth - 1] == '{') ? EXPECT_MEMBER : EXPECT_VALUE;
                }
                else if(*s == (stack[depth - 1] == '{' ? '}' : ']')) {
                    s++;
                    depth--;
                }
                else
                    return "invalid JSON: expected ',' or the end of the object or array";
                break;
        }
    }
}
