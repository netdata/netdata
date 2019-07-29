// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "mongodb.h"
#include <mongoc.h>

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

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

        if(!value) value = "";

        // strip quotes
        if(*value == '"' || *value == '\'') {
            value++;

            s = value;
            while(*s) s++;
            if(s != value) s--;

            if(*s == '"' || *s == '\'') *s = '\0';
        }
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
        error("BACKEND: collection name is a mandatory MongoDB parameter but it is not configured");
        return 1;
    }

    *uri_p = uri;
    *database_p = database;
    *collection_p = collection;

    return 0;
}
