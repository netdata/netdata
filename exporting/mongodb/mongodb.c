// SPDX-License-Identifier: GPL-3.0-or-later

#define EXPORTING_INTERNALS
#include "mongodb.h"

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

/**
 * Initialize MongoDB connector specific data, including a ring buffer
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int mongodb_init(struct instance *instance)
{
    struct mongodb_specific_config *connector_specific_config = instance->config.connector_specific_config;
    mongoc_uri_t *uri;
    bson_error_t bson_error;

    if (unlikely(!connector_specific_config->collection || !*connector_specific_config->collection)) {
        error("EXPORTING: collection name is a mandatory MongoDB parameter, but it is not configured");
        return 1;
    }

    uri = mongoc_uri_new_with_error(instance->config.destination, &bson_error);
    if (unlikely(!uri)) {
        error(
            "EXPORTING: failed to parse URI: %s. Error message: %s", instance->config.destination, bson_error.message);
        return 1;
    }

    int32_t socket_timeout =
        mongoc_uri_get_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, instance->config.timeoutms);
    if (!mongoc_uri_set_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout)) {
        error("EXPORTING: failed to set %s to the value %d", MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout);
        return 1;
    };

    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;

    connector_specific_data->client = mongoc_client_new_from_uri(uri);
    if (unlikely(!connector_specific_data->client)) {
        error("EXPORTING: failed to create a new client");
        return 1;
    }

    if (!mongoc_client_set_appname(connector_specific_data->client, "netdata")) {
        error("EXPORTING: failed to set client appname");
    };

    connector_specific_data->collection = mongoc_client_get_collection(
        connector_specific_data->client, connector_specific_config->database, connector_specific_config->collection);

    mongoc_uri_destroy(uri);

    // create a ring buffer
    struct bson_buffer *first_buffer = NULL;

    if (instance->config.buffer_on_failures < 2)
        instance->config.buffer_on_failures = 1;
    else
        instance->config.buffer_on_failures -= 1;

    for (int i = 0; i < instance->config.buffer_on_failures; i++) {
        struct bson_buffer *current_buffer = callocz(1, sizeof(struct bson_buffer));

        if (!connector_specific_data->first_buffer)
            first_buffer = current_buffer;
        else
            current_buffer->next = connector_specific_data->first_buffer;

        connector_specific_data->first_buffer = current_buffer;
    }

    first_buffer->next = connector_specific_data->first_buffer;
    connector_specific_data->last_buffer = connector_specific_data->first_buffer;

    return 0;
}

/**
 * Clean a MongoDB connector instance up
 *
 * @param instance an instance data structure.
 */
void mongodb_cleanup(struct instance *instance)
{
    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;

    mongoc_collection_destroy(connector_specific_data->collection);
    mongoc_client_destroy(connector_specific_data->client);

    return;
}

/**
 * Initialize a MongoDB connector instance
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
    instance->end_batch_formatting = format_batch_mongodb;

    instance->send_header = NULL;
    instance->check_response = NULL;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for MongoDB exporting connector instance %s", instance->config.name);
        return 1;
    }
    if (uv_mutex_init(&instance->mutex))
        return 1;
    if (uv_cond_init(&instance->cond_var))
        return 1;

    struct mongodb_specific_data *connector_specific_data = callocz(1, sizeof(struct mongodb_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    instance->config.timeoutms =
        (instance->config.update_every >= 2) ? (instance->engine->config.update_every * MSEC_PER_SEC - 500) : 1000;

    if (!instance->engine->mongoc_initialized) {
        mongoc_init();
        instance->engine->mongoc_initialized = 1;
    }

    if (unlikely(mongodb_init(instance))) {
        error("EXPORTING: cannot initialize MongoDB exporting connector");
        return 1;
    }

    return 0;
}

/**
 * Free an array of BSON structures
 *
 * @param insert an array of documents.
 * @param documents_inserted the number of documents inserted.
 */
void free_bson(bson_t **insert, size_t documents_inserted)
{
    size_t i;

    for (i = 0; i < documents_inserted; i++)
        bson_destroy(insert[i]);

    freez(insert);
}

/**
 * Format a batch for the MongoDB connector
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int format_batch_mongodb(struct instance *instance)
{
    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;
    struct stats *stats = &instance->stats;

    bson_t **insert = connector_specific_data->last_buffer->insert;
    if (insert) {
        // ring buffer is full, reuse the oldest element
        connector_specific_data->first_buffer = connector_specific_data->first_buffer->next;
        free_bson(insert, connector_specific_data->last_buffer->documents_inserted);
    }
    insert = callocz((size_t)stats->chart_buffered_metrics, sizeof(bson_t *));
    connector_specific_data->last_buffer->insert = insert;

    BUFFER *buffer = (BUFFER *)instance->buffer;
    char *start = (char *)buffer_tostring(buffer);
    char *end = start;

    size_t documents_inserted = 0;

    while (*end && documents_inserted <= (size_t)stats->chart_buffered_metrics) {
        while (*end && *end != '\n')
            end++;

        if (likely(*end)) {
            *end = '\0';
            end++;
        } else {
            break;
        }

        bson_error_t bson_error;
        insert[documents_inserted] = bson_new_from_json((const uint8_t *)start, -1, &bson_error);

        if (unlikely(!insert[documents_inserted])) {
            error("EXPORTING: %s", bson_error.message);
            free_bson(insert, documents_inserted);
            return 1;
        }

        start = end;

        documents_inserted++;
    }

    buffer_flush(buffer);

    connector_specific_data->last_buffer->documents_inserted = documents_inserted;
    connector_specific_data->last_buffer = connector_specific_data->last_buffer->next;

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
    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;

    while (!netdata_exit) {
        struct stats *stats = &instance->stats;

        uv_mutex_lock(&instance->mutex);
        uv_cond_wait(&instance->cond_var, &instance->mutex);

        bson_t **insert = connector_specific_data->first_buffer->insert;
        size_t documents_inserted = connector_specific_data->first_buffer->documents_inserted;

        connector_specific_data->first_buffer->insert = NULL;
        connector_specific_data->first_buffer->documents_inserted = 0;
        connector_specific_data->first_buffer = connector_specific_data->first_buffer->next;

        uv_mutex_unlock(&instance->mutex);

        size_t data_size = 0;
        for (size_t i = 0; i < documents_inserted; i++) {
            data_size += insert[i]->len;
        }

        debug(
            D_BACKEND,
            "EXPORTING: mongodb_insert(): destination = %s, database = %s, collection = %s, data size = %zu",
            instance->config.destination,
            connector_specific_config->database,
            connector_specific_config->collection,
            data_size);

        if (unlikely(documents_inserted == 0))
            continue;

        bson_error_t bson_error;
        if (likely(mongoc_collection_insert_many(
                connector_specific_data->collection,
                (const bson_t **)insert,
                documents_inserted,
                NULL,
                NULL,
                &bson_error))) {
            stats->chart_sent_bytes += data_size;
            stats->chart_transmission_successes++;
            stats->chart_receptions++;
        } else {
            // oops! we couldn't send (all or some of the) data
            error("EXPORTING: %s", bson_error.message);
            error(
                "EXPORTING: failed to write data to the database '%s'. "
                "Willing to write %zu bytes, wrote %zu bytes.",
                instance->config.destination, data_size, 0UL);

            stats->chart_transmission_failures++;
            stats->chart_data_lost_events++;
            stats->chart_lost_bytes += data_size;
            stats->chart_lost_metrics += stats->chart_buffered_metrics;
        }

        free_bson(insert, documents_inserted);

        if (unlikely(netdata_exit))
            break;

        stats->chart_sent_bytes += data_size;
        stats->chart_sent_metrics = stats->chart_buffered_metrics;

#ifdef UNIT_TESTING
        break;
#endif
    }

    mongodb_cleanup(instance);
}
