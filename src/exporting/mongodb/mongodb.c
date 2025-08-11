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
        netdata_log_error("EXPORTING: collection name is a mandatory MongoDB parameter, but it is not configured");
        return 1;
    }

    uri = mongoc_uri_new_with_error(instance->config.destination, &bson_error);
    if (unlikely(!uri)) {
        netdata_log_error("EXPORTING: failed to parse URI: %s. Error message: %s",
                          instance->config.destination,
                          bson_error.message);
        return 1;
    }

    int32_t socket_timeout =
        mongoc_uri_get_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, instance->config.timeoutms);
    if (!mongoc_uri_set_option_as_int32(uri, MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout)) {
        netdata_log_error("EXPORTING: failed to set %s to the value %d", MONGOC_URI_SOCKETTIMEOUTMS, socket_timeout);
        return 1;
    };

    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;

    connector_specific_data->client = mongoc_client_new_from_uri(uri);
    if (unlikely(!connector_specific_data->client)) {
        netdata_log_error("EXPORTING: failed to create a new client");
        return 1;
    }

    if (!mongoc_client_set_appname(connector_specific_data->client, "netdata")) {
        netdata_log_error("EXPORTING: failed to set client appname");
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
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = format_batch_mongodb;

    instance->prepare_header = NULL;
    instance->check_response = NULL;

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    if (!instance->buffer) {
        netdata_log_error("EXPORTING: cannot create buffer for MongoDB exporting connector instance %s",
                          instance->config.name);
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
        netdata_log_error("EXPORTING: cannot initialize MongoDB exporting connector");
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
        connector_specific_data->total_documents_inserted -= connector_specific_data->last_buffer->documents_inserted;
        stats->buffered_bytes -= connector_specific_data->last_buffer->buffered_bytes;
    }
    insert = callocz((size_t)stats->buffered_metrics, sizeof(bson_t *));
    connector_specific_data->last_buffer->insert = insert;

    BUFFER *buffer = (BUFFER *)instance->buffer;
    char *start = (char *)buffer_tostring(buffer);
    char *end = start;

    size_t documents_inserted = 0;

    while (*end && documents_inserted <= (size_t)stats->buffered_metrics) {
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
            netdata_log_error(
                "EXPORTING: Failed creating a BSON document from a JSON string \"%s\" : %s", start, bson_error.message);
            free_bson(insert, documents_inserted);
            return 1;
        }

        start = end;

        documents_inserted++;
    }

    stats->buffered_bytes += connector_specific_data->last_buffer->buffered_bytes = buffer_strlen(buffer);

    buffer_flush(buffer);

    // The stats->buffered_metrics is used in the MongoDB batch formatting as a variable for the number
    // of metrics, added in the current iteration, so we are clearing it here. We will use the
    // connector_specific_data->total_documents_inserted in the worker to show the statistics.
    stats->buffered_metrics = 0;
    connector_specific_data->total_documents_inserted += documents_inserted;

    connector_specific_data->last_buffer->documents_inserted = documents_inserted;
    connector_specific_data->last_buffer = connector_specific_data->last_buffer->next;

    return 0;
}

/**
 * Clean a MongoDB connector instance up
 *
 * @param instance an instance data structure.
 */
void mongodb_cleanup(struct instance *instance)
{
    netdata_log_info("EXPORTING: cleaning up instance %s ...", instance->config.name);

    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;

    mongoc_collection_destroy(connector_specific_data->collection);
    mongoc_client_destroy(connector_specific_data->client);
    if (instance->engine->mongoc_initialized) {
        mongoc_cleanup();
        instance->engine->mongoc_initialized = 0;
    }

    buffer_free(instance->buffer);

    struct bson_buffer *next_buffer = connector_specific_data->first_buffer;
    for (int i = 0; i < instance->config.buffer_on_failures; i++) {
        struct bson_buffer *current_buffer = next_buffer;
        next_buffer = next_buffer->next;

        if (current_buffer->insert)
            free_bson(current_buffer->insert, current_buffer->documents_inserted);
        freez(current_buffer);
    }

    freez(connector_specific_data);

    struct mongodb_specific_config *connector_specific_config =
        (struct mongodb_specific_config *)instance->config.connector_specific_config;
    freez(connector_specific_config->database);
    freez(connector_specific_config->collection);
    freez(connector_specific_config);

    netdata_log_info("EXPORTING: instance %s exited", instance->config.name);
    instance->exited = 1;

    return;
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
#ifdef NETDATA_INTERNAL_CHECKS
    struct mongodb_specific_config *connector_specific_config = instance->config.connector_specific_config;
#endif
    struct mongodb_specific_data *connector_specific_data =
        (struct mongodb_specific_data *)instance->connector_specific_data;

    while (!instance->engine->exit) {
        struct stats *stats = &instance->stats;

        uv_mutex_lock(&instance->mutex);
        if (!connector_specific_data->first_buffer->insert ||
            !connector_specific_data->first_buffer->documents_inserted) {
            while (!instance->data_is_ready)
                uv_cond_wait(&instance->cond_var, &instance->mutex);
            instance->data_is_ready = 0;
        }

        if (unlikely(instance->engine->exit)) {
            uv_mutex_unlock(&instance->mutex);
            break;
        }

        // reset the monitoring chart counters
        stats->received_bytes =
        stats->sent_bytes =
        stats->sent_metrics =
        stats->lost_metrics =
        stats->receptions =
        stats->transmission_successes =
        stats->transmission_failures =
        stats->data_lost_events =
        stats->lost_bytes =
        stats->reconnects = 0;

        bson_t **insert = connector_specific_data->first_buffer->insert;
        size_t documents_inserted = connector_specific_data->first_buffer->documents_inserted;
        size_t buffered_bytes = connector_specific_data->first_buffer->buffered_bytes;

        connector_specific_data->first_buffer->insert = NULL;
        connector_specific_data->first_buffer->documents_inserted = 0;
        connector_specific_data->first_buffer->buffered_bytes = 0;
        connector_specific_data->first_buffer = connector_specific_data->first_buffer->next;

        uv_mutex_unlock(&instance->mutex);

        size_t data_size = 0;
        for (size_t i = 0; i < documents_inserted; i++) {
            data_size += insert[i]->len;
        }

        netdata_log_debug(
            D_EXPORTING,
            "EXPORTING: mongodb_insert(): destination = %s, database = %s, collection = %s, data size = %zu",
            instance->config.destination,
            connector_specific_config->database,
            connector_specific_config->collection,
            data_size);

        if (likely(documents_inserted != 0)) {
            bson_error_t bson_error;
            if (likely(mongoc_collection_insert_many(
                    connector_specific_data->collection,
                    (const bson_t **)insert,
                    documents_inserted,
                    NULL,
                    NULL,
                    &bson_error))) {
                stats->sent_metrics = documents_inserted;
                stats->sent_bytes += data_size;
                stats->transmission_successes++;
                stats->receptions++;
            } else {
                // oops! we couldn't send (all or some of the) data
                netdata_log_error("EXPORTING: %s", bson_error.message);
                netdata_log_error(
                    "EXPORTING: failed to write data to the database '%s'. "
                    "Willing to write %zu bytes, wrote %zu bytes.",
                    instance->config.destination, data_size, 0UL);

                stats->transmission_failures++;
                stats->data_lost_events++;
                stats->lost_bytes += buffered_bytes;
                stats->lost_metrics += documents_inserted;
            }
        }

        free_bson(insert, documents_inserted);

        if (unlikely(instance->engine->exit))
            break;

        uv_mutex_lock(&instance->mutex);

        stats->buffered_metrics = connector_specific_data->total_documents_inserted;

        send_internal_metrics(instance);

        connector_specific_data->total_documents_inserted -= documents_inserted;

        stats->buffered_metrics = 0;
        stats->buffered_bytes -= buffered_bytes;

        uv_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        return;
#endif
    }

    mongodb_cleanup(instance);
}
