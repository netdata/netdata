// SPDX-License-Identifier: GPL-3.0-or-later

#define EXPORTING_INTERNALS
#include "mongodb.h"

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

/**
 * Initialize MongoDB connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_mongodb_instance(struct instance *instance)
{
    instance->worker = mongodb_connector_worker;

    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_json_plaintext;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_json_plaintext;
    else
        instance->metric_formatting = format_dimension_stored_json_plaintext;

    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = NULL;

    instance->send_header = NULL;
    instance->check_response = NULL;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for MongoDB exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    struct mongodb_specific_config *connector_specific_config = instance->config.connector_specific_config;
    struct mongodb_specific_data *connector_specific_data = callocz(1, sizeof(struct mongodb_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    instance->config.timeoutms =
        (instance->config.update_every >= 2) ? (instance->engine->config.update_every * MSEC_PER_SEC - 500) : 1000;

    if (!instance->engine->mongoc_initialized) {
        if (unlikely(mongodb_init(
                instance->config.destination,
                connector_specific_config->database,
                connector_specific_config->collection,
                instance->config.timeoutms))) {
            error("EXPORTING: cannot initialize MongoDB exporting connector");
            return 1;
        }
        instance->engine->mongoc_initialized = 1;
    }

    mongodb_init(
        instance->config.destination,
        connector_specific_config->database,
        connector_specific_config->collection,
        instance->config.timeoutms);

    return 0;
}

/**
 * MongoDB connector worker
 *
 * Runs in a separate thread for every instance.
 *
 * @param instance_p an instance data structure.
 */
void mongodb_connector_worker(void *instance_p)
{
    struct instance *instance = (struct instance *)instance_p;
    struct mongodb_specific_config *connector_specific_config = instance->config.connector_specific_config;

    while (!netdata_exit) {
        struct stats *stats = &instance->stats;

        uv_mutex_lock(&instance->mutex);
        uv_cond_wait(&instance->cond_var, &instance->mutex);

        BUFFER *buffer = (BUFFER *)instance->buffer;
        size_t buffer_len = buffer_strlen(buffer);

        size_t sent = 0;

        while (sent < buffer_len) {
            const char *first_char = buffer_tostring(buffer) + sent;

            debug(
                D_BACKEND,
                "EXPORTING: mongodb_insert(): destination = %s, database = %s, collection = %s, buffer = %zu",
                instance->config.destination,
                connector_specific_config->database,
                connector_specific_config->collection,
                buffer_len);

            if (likely(!mongodb_insert((char *)first_char, (size_t)stats->chart_buffered_metrics))) {
                sent += buffer_len;
                stats->chart_transmission_successes++;
                stats->chart_receptions++;
            } else {
                // oops! we couldn't send (all or some of the) data
                error(
                    "EXPORTING: failed to write data to the database '%s'. "
                    "Willing to write %zu bytes, wrote %zu bytes.",
                    instance->config.destination, buffer_len, 0UL);

                stats->chart_transmission_failures++;
                stats->chart_data_lost_events++;
                stats->chart_lost_bytes += buffer_len;
                stats->chart_lost_metrics += stats->chart_buffered_metrics;

                break;
            }

            if (unlikely(netdata_exit))
                break;
        }

        stats->chart_sent_bytes += sent;
        if (likely(sent == buffer_len))
            stats->chart_sent_metrics = stats->chart_buffered_metrics;

        buffer_flush(buffer);

        uv_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        break;
#endif
    }
}
