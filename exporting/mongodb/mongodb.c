// SPDX-License-Identifier: GPL-3.0-or-later

#define EXPORTING_INTERNALS
#include "mongodb.h"
#include <mongoc.h>

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

mongoc_client_t *mongodb_client;
mongoc_collection_t *mongodb_collection;

int mongodb_init(const char *uri_string,
                 const char *database_string,
                 const char *collection_string,
                 int32_t default_socket_timeout) {
    mongoc_uri_t *uri;
    bson_error_t error;

    mongoc_init();

    uri = mongoc_uri_new_with_error(uri_string, &error);
    if(unlikely(!uri)) {
        error("EXPORTING: failed to parse URI: %s. Error message: %s", uri_string, error.message);
        return 1;
    }

    int32_t socket_timeout = mongoc_uri_get_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, default_socket_timeout);
    if(!mongoc_uri_set_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout)) {
        error("EXPORTING: failed to set %s to the value %d", MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout);
        return 1;
    };

    mongodb_client = mongoc_client_new_from_uri(uri);
    if(unlikely(!mongodb_client)) {
        error("EXPORTING: failed to create a new client");
        return 1;
    }

    if(!mongoc_client_set_appname(mongodb_client, "netdata")) {
        error("EXPORTING: failed to set client appname");
    };

    mongodb_collection = mongoc_client_get_collection(mongodb_client, database_string, collection_string);

    mongoc_uri_destroy(uri);

    return 0;
}

void free_bson(bson_t **insert, size_t n_documents) {
    size_t i;

    for(i = 0; i < n_documents; i++)
        bson_destroy(insert[i]);

    free(insert);
}

int mongodb_insert(char *data, size_t n_metrics) {
    bson_t **insert = calloc(n_metrics, sizeof(bson_t *));
    bson_error_t error;
    char *start = data, *end = data;
    size_t n_documents = 0;

    while(*end && n_documents <= n_metrics) {
        while(*end && *end != '\n') end++;

        if(likely(*end)) {
            *end = '\0';
            end++;
        }
        else {
            break;
        }

        insert[n_documents] = bson_new_from_json((const uint8_t *)start, -1, &error);

        if(unlikely(!insert[n_documents])) {
           error("EXPORTING: %s", error.message);
           free_bson(insert, n_documents);
           return 1;
        }

        start = end;

        n_documents++;
    }

    if(unlikely(!mongoc_collection_insert_many(mongodb_collection, (const bson_t **)insert, n_documents, NULL, NULL, &error))) {
       error("EXPORTING: %s", error.message);
       free_bson(insert, n_documents);
       return 1;
    }

    free_bson(insert, n_documents);

    return 0;
}

void mongodb_cleanup() {
    mongoc_collection_destroy(mongodb_collection);
    mongoc_client_destroy(mongodb_client);
    mongoc_cleanup();

    return;
}
