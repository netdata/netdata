#include "libnetdata/libnetdata.h"
#include "json.h"

/**
 * Json Tokenise
 *
 * Parse the string into a json-c object.
 *
 * @param js is the string to parse
 *
 * @return the parsed json object, or NULL on error
 */
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
 * Walk the objects of an array, also inside nested arrays. Scalar and null elements have no key, so they are skipped.
 *
 * @param ptr the pointer for the object that we will parse.
 * @param callback_data additional data to be used together the callback function
 * @param callback_function function used to create a silencer.
 */
static inline void json_jsonc_parse_array(json_object *ptr, void *callback_data,int (*callback_function)(struct json_entry *)) {
    size_t end = json_object_array_length(ptr);

    for(size_t i = 0; i < end; i++) {
        json_object *jvalue = json_object_array_get_idx(ptr, i);
        enum json_type type = json_object_get_type(jvalue);

        if(type == json_type_array) {
            json_jsonc_parse_array(jvalue, callback_data, callback_function);
            continue;
        }

        if(type != json_type_object)
            continue;

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
}

/**
 * Tree
 *
 * Call the correct walk function according its type.
 *
 * @param t the json object to work
 * @param callback_data additional data to be used together the callback function
 * @param callback_function function used to create a silencer.
 *
 * @return It returns 1, or 0 when t is not an object
 */
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
        json_object *tokens = json_tokenise(js);

        if(tokens) {
            json_walk(tokens, callback_data, callback_function);
            json_object_put(tokens);
            return JSON_OK;
        }

        return JSON_CANNOT_PARSE;
    }

    return JSON_CANNOT_DOWNLOAD;
}

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
