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

int mongodb_insert(const char *data) {
    bson_t *insert;
    bson_error_t error;

    insert = bson_new_from_json((const uint8_t *)data, -1, &error);

    if (!insert) {
       fprintf (stderr, "%s\n", error.message);
       return 1;
    }

    if (!mongoc_collection_insert_one(collection, insert, NULL, NULL, &error)) {
       fprintf (stderr, "%s\n", error.message);
    }

    bson_destroy (insert);

    return 0;
}

void mongodb_cleanup() {
    mongoc_collection_destroy (collection);
    mongoc_client_destroy (client);
    mongoc_cleanup ();

    return;
}

int format_dimension_collected_mongodb_plaintext(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;
    (void)backend_options;

    const char *tags_pre = "", *tags_post = "", *tags = host->tags;
    if(!tags) tags = "";

    if(*tags) {
        if(*tags == '{' || *tags == '[' || *tags == '"') {
            tags_pre = "\"host_tags\":";
            tags_post = ",";
        }
        else {
            tags_pre = "\"host_tags\":\"";
            tags_post = "\",";
        }
    }

    buffer_sprintf(b,

                   ",\n{"
                   "\"prefix\":\"%s\","
                   "\"hostname\":\"%s\","
                   "%s%s%s"

                   "\"chart_id\":\"%s\","
                   "\"chart_name\":\"%s\","
                   "\"chart_family\":\"%s\","
                   "\"chart_context\": \"%s\","
                   "\"chart_type\":\"%s\","
                   "\"units\": \"%s\","

                   "\"id\":\"%s\","
                   "\"name\":\"%s\","
                   "\"value\":" COLLECTED_NUMBER_FORMAT ","

                   "\"timestamp\": %llu}",

                   prefix,
                   hostname,
                   tags_pre, tags, tags_post,

                   st->id,
                   st->name,
                   st->family,
                   st->context,
                   st->type,
                   st->units,

                   rd->id,
                   rd->name,
                   rd->last_collected_value,

                   (unsigned long long) rd->last_collected_time.tv_sec
    );

    return 1;
}

int format_dimension_stored_mongodb_plaintext(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

    if(!isnan(value)) {
        const char *tags_pre = "", *tags_post = "", *tags = host->tags;
        if(!tags) tags = "";

        if(*tags) {
            if(*tags == '{' || *tags == '[' || *tags == '"') {
                tags_pre = "\"host_tags\":";
                tags_post = ",";
            }
            else {
                tags_pre = "\"host_tags\":\"";
                tags_post = "\",";
            }
        }

        buffer_sprintf(b,

                       ",\n{"
                       "\"prefix\":\"%s\","
                       "\"hostname\":\"%s\","
                       "%s%s%s"

                       "\"chart_id\":\"%s\","
                       "\"chart_name\":\"%s\","
                       "\"chart_family\":\"%s\","
                       "\"chart_context\": \"%s\","
                       "\"chart_type\":\"%s\","
                       "\"units\": \"%s\","

                       "\"id\":\"%s\","
                       "\"name\":\"%s\","
                       "\"value\":" CALCULATED_NUMBER_FORMAT ","

                       "\"timestamp\": %llu}",

                       prefix,
                       hostname,
                       tags_pre, tags, tags_post,

                       st->id,
                       st->name,
                       st->family,
                       st->context,
                       st->type,
                       st->units,

                       rd->id,
                       rd->name,
                       value,

                       (unsigned long long) last_t
        );

        return 1;
    }
    return 0;
}

int process_mongodb_response(BUFFER *b) {
    return discard_response(b, "mongodb");
}
