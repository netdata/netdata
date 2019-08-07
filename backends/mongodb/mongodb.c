// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "mongodb.h"
#include <mongoc.h>

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

mongoc_client_pool_t *mongodb_client_pool;
const char *mongodb_database_string;
const char *mongodb_collection_string;

int mongodb_init(const char *uri_string,
                 const char *database_string,
                 const char *collection_string,
                 int32_t default_socket_timeout) {
    mongoc_uri_t *uri;
    bson_error_t error;

    mongodb_database_string = database_string;
    mongodb_collection_string = collection_string;

    mongoc_init();

    uri = mongoc_uri_new_with_error(uri_string, &error);
    if(unlikely(!uri)) {
        error("BACKEND: failed to parse URI: %s. Error message: %s", uri_string, error.message);
        return 1;
    }

    int32_t socket_timeout = mongoc_uri_get_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, default_socket_timeout);
    if(!mongoc_uri_set_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout)) {
        error("BACKEND: failed to set %s to the value %d", MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout);
        return 1;
    };

    mongodb_client_pool = mongoc_client_pool_new(uri);
    if(unlikely(!mongodb_client_pool)) {
        error("BACKEND: failed to create a new client pool");
        return 1;
    }

    mongoc_client_pool_set_error_api(mongodb_client_pool, MONGOC_ERROR_API_VERSION_2);

    const char *appname = mongoc_uri_get_option_as_utf8(uri, MONGOC_URI_APPNAME, "netdata");
    if(!mongoc_client_pool_set_appname(mongodb_client_pool, appname)) {
        error("BACKEND: failed to set client appname");
    };

    mongoc_uri_destroy(uri);

    return 0;
}

void free_bson(bson_t **insert, size_t n_documents) {
    size_t i;

    for(i = 0; i < n_documents; i++)
        bson_destroy(insert[i]);

    free(insert);
}

struct objects_to_clean {
    struct mongodb_thread **thread_data;

    mongoc_client_t **client;
    mongoc_collection_t **collection;

    bson_t ***insert;
    size_t *n_documents;
};

static void mongodb_insert_cleanup(void *objects_to_clean) {
    struct objects_to_clean *objects = (struct objects_to_clean *)objects_to_clean;

    free_bson(*objects->insert, *objects->n_documents);
    mongoc_collection_destroy(*objects->collection);
    mongoc_client_pool_push(mongodb_client_pool, *objects->client);

    buffer_flush((*objects->thread_data)->buffer);

    netdata_mutex_lock(&(*objects->thread_data)->mutex);
    (*objects->thread_data)->finished = 1;
    netdata_mutex_unlock(&(*objects->thread_data)->mutex);
}

void *mongodb_insert(void *mongodb_thread) {
    struct objects_to_clean objects_to_clean;
    netdata_thread_cleanup_push(mongodb_insert_cleanup, (void *)&objects_to_clean);

    struct mongodb_thread *thread_data = (struct mongodb_thread *)mongodb_thread;
    mongoc_client_t *client;
    mongoc_collection_t *collection;
    bson_t **insert = calloc(thread_data->n_metrics, sizeof(bson_t *));
    bson_error_t error;
    char *start = (char *)buffer_tostring(thread_data->buffer);
    char *end = start;
    size_t n_documents = 0;

    objects_to_clean.thread_data = &thread_data;
    objects_to_clean.client = &client;
    objects_to_clean.collection = &collection;
    objects_to_clean.insert = &insert;
    objects_to_clean.n_documents = &n_documents;


    client = mongoc_client_pool_pop(mongodb_client_pool);
    if(unlikely(!client)) {
        error("BACKEND: failed to create a new client");
        thread_data->error = 1;
        free_bson(insert, n_documents);
        buffer_flush(thread_data->buffer);
        return NULL;
    }

    collection = mongoc_client_get_collection(client, mongodb_database_string, mongodb_collection_string);

    while(*end && n_documents <= thread_data->n_metrics) {
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
            error("BACKEND: %s", error.message);
            thread_data->error = 1;
            goto cleanup;
        }

        start = end;

        n_documents++;
    }

    if(unlikely(!mongoc_collection_insert_many(collection, (const bson_t **)insert, n_documents, NULL, NULL, &error))) {
        error("BACKEND: %s", error.message);
        thread_data->error = 1;
        goto cleanup;
    }

cleanup:
    netdata_thread_cleanup_pop(1);

    return NULL;
}

void mongodb_cleanup() {
    mongoc_client_pool_destroy(mongodb_client_pool);
    mongoc_cleanup();

    return;
}

int read_mongodb_conf(const char *path, char **uri_p, char **database_p, char **collection_p) {
    char *uri = *uri_p;
    char *database = *database_p;
    char *collection = *collection_p;

    if(unlikely(uri)) freez(uri);
    if(unlikely(database)) freez(database);
    if(unlikely(collection)) freez(collection);
    uri = NULL;
    database = NULL;
    collection = NULL;

    int line = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/mongodb.conf", path);

    char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

    debug(D_BACKEND, "BACKEND: opening config file '%s'", filename);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        return 1;
    }

    while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
        buffer[CONFIG_FILE_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if(!s || *s == '#') {
            debug(D_BACKEND, "BACKEND: ignoring line %d of file '%s', it is empty.", line, filename);
            continue;
        }

        char *name = s;
        char *value = strchr(s, '=');
        if(unlikely(!value)) {
            error("BACKEND: ignoring line %d ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(unlikely(!name || *name == '#')) {
            error("BACKEND: ignoring line %d of file '%s', name is empty.", line, filename);
            continue;
        }

        if(!value)
            value = "";
        else
            value = strip_quotes(value);

        if(name[0] == 'u' && !strcmp(name, "uri")) {
            uri = strdupz(value);
        }
        else if(name[0] == 'd' && !strcmp(name, "database")) {
            database = strdupz(value);
        }
        else if(name[0] == 'c' && !strcmp(name, "collection")) {
            collection = strdupz(value);
        }
    }

    fclose(fp);

    if(unlikely(!collection || !*collection)) {
        error("BACKEND: collection name is a mandatory MongoDB parameter, but it is not configured");
        return 1;
    }

    *uri_p = uri;
    *database_p = database;
    *collection_p = collection;

    return 0;
}
