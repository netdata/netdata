#ifndef CHECKIN_JSON_H
#define CHECKIN_JSON_H 1

#include <json-c/json.h>
// fix an older json-c bug
// https://github.com/json-c/json-c/issues/135
#ifdef error_description
#undef error_description
#endif // error_description

//https://www.ibm.com/support/knowledgecenter/en/SS9H2Y_7.6.0/com.ibm.dp.doc/json_parserlimits.html
#define JSON_NAME_LEN 256

typedef enum {
    JSON_OBJECT = 0,
    JSON_ARRAY = 1,
    JSON_STRING = 2,
    JSON_NUMBER = 3,
    JSON_BOOLEAN = 4,
    JSON_NULL = 5,
} JSON_ENTRY_TYPE;

typedef struct json_entry {
    JSON_ENTRY_TYPE type;
    char name[JSON_NAME_LEN + 1];
    union {
        char *string;           // type == JSON_STRING
        NETDATA_DOUBLE number;  // type == JSON_NUMBER
        int boolean;            // type == JSON_BOOLEAN
    } data;

    void *callback_data;
} JSON_ENTRY;

// ----------------------------------------------------------------------------
// public functions

#define JSON_OK 				0
#define JSON_CANNOT_DOWNLOAD 	1
#define JSON_CANNOT_PARSE		2

int json_parse(char *js, void *callback_data, int (*callback_function)(JSON_ENTRY *));

// Strict RFC 8259 validation of one complete document, with no extensions: double-quoted
// strings without raw control characters, no comments, nothing but whitespace after the value.
// json-c cannot be used for this - even its strict mode accepts raw newlines in strings and
// single-quoted strings. UTF-8 well-formedness of string contents is not checked.
// Returns NULL when valid, or a static reason starting with "invalid JSON: ".
const char *json_strict_validate(const char *s, size_t len);


// ----------------------------------------------------------------------------
// private functions

json_object *json_tokenise(char *js);
size_t json_walk(json_object *t, void *callback_data, int (*callback_function)(struct json_entry *));

static inline void cleanup_json_object_pp(struct json_object **jobj) {
    if(*jobj)
        json_object_put(*jobj);
}
#define CLEAN_JSON_OBJECT _cleanup_(cleanup_json_object_pp) struct json_object

#endif // CHECKIN_JSON_H
