#ifndef CHECKIN_JSON_H
#define CHECKIN_JSON_H 1


#include "jsmn.h"

#define JSON_NAME_LEN 200
#define JSON_FULLNAME_LEN 1024

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
    char fullname[JSON_FULLNAME_LEN + 1];
    union {
        char *string;			// type == JSON_STRING
        long double number;		// type == JSON_NUMBER
        int boolean;			// type == JSON_BOOLEAN
        size_t items;			// type == JSON_ARRAY
    } data;
    size_t pos;					// the position of this item in its parent

    char *original_string;

    // callbacks are only used when type is
    // JSON_ARRAY to define the callback for each item in the array
    // JSON_OBJECT to define the callback for each item in the object
    void *callback_data;
    int (*callback_function)(struct json_entry *);
} JSON_ENTRY;

// ----------------------------------------------------------------------------
// public functions

#define JSON_OK 				0
#define JSON_CANNOT_DOWNLOAD 	1
#define JSON_CANNOT_PARSE		2

int json_parse(char *js, void *callback_data, int (*callback_function)(JSON_ENTRY *));


// ----------------------------------------------------------------------------
// private functions

jsmntok_t *json_tokenise(char *js, size_t len, size_t *count);

size_t json_walk_tree(char *js, jsmntok_t *t, void *callback_data, int (*callback_function)(struct json_entry *));
size_t json_walk_object(char *js, jsmntok_t *t, size_t nest, size_t start, JSON_ENTRY *e);
size_t json_walk_array(char *js, jsmntok_t *t, size_t nest, size_t start, JSON_ENTRY *e);
size_t json_walk_string(char *js, jsmntok_t *t, size_t start, JSON_ENTRY *e);
size_t json_walk_primitive(char *js, jsmntok_t *t, size_t start, JSON_ENTRY *e);

int json_callback_print(JSON_ENTRY *e);
int json_test(char *str);

#endif