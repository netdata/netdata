// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "mongodb.h"
#include <mongoc.h>

mongoc_client_t *client;
mongoc_collection_t *collection;

int mongodb_init(const char *uri_string, const char *database_string, const char *collection_string) {
    mongoc_uri_t *uri;
    bson_error_t error;

    mongoc_init ();

    uri = mongoc_uri_new_with_error (uri_string, &error);
    if (!uri) {
       error("BACKEND: failed to parse URI: %s. Error message: %s", uri_string, error.message);
       return 1;
    }

    mongoc_uri_set_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, 9000); // TODO: use variable for timeout

    client = mongoc_client_new_from_uri (uri);
    if (!client) {
       return 1;
    }

    mongoc_client_set_appname (client, "netdata");

    collection = mongoc_client_get_collection (client, database_string, collection_string);

    mongoc_uri_destroy (uri);

    return 0;
}

int mongodb_insert(char *data) {
    char *start = data, *end = data;

    while(*end) {
        bson_t *insert;
        bson_error_t error;

        while(*end && *end != '\n') end++;

        if(*end) {
            *end = '\0';
            end++;
        }
        else {
            break;
        }

        insert = bson_new_from_json((const uint8_t *)start, -1, &error);

        if (!insert) {
           fprintf (stderr, "%s\n", error.message);
           return 1;
        }

        start = end;

        if (!mongoc_collection_insert_one(collection, insert, NULL, NULL, &error)) {
           fprintf (stderr, "%s\n", error.message);
        }

        bson_destroy (insert);
    }

    return 0;
}

void mongodb_cleanup() {
    mongoc_collection_destroy (collection);
    mongoc_client_destroy (client);
    mongoc_cleanup ();

    return;
}
