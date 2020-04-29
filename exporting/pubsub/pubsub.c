// SPDX-License-Identifier: GPL-3.0-or-later

#include "pubsub.h"

/**
 * Initialize Pub/Sub connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_pubsub_instance(struct instance *instance)
{
    instance->worker = pubsub_connector_worker;

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
        error("EXPORTING: cannot create buffer for Pub/Sub exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    struct pubsub_specific_data *connector_specific_data = callocz(1, sizeof(struct pubsub_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    info("EXPORTING: Connector specific data pointer = %p", instance->connector_specific_data);

    struct pubsub_specific_config *pubsub_specific_config =
        (struct pubsub_specific_config *)instance->config.connector_specific_config;
    pubsub_init(
        (void *)connector_specific_data, pubsub_specific_config->project_id, pubsub_specific_config->topic_id);

    return 0;
}

/**
 * Format dimension using collected data for Pub/Sub connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_collected_pubsub(struct instance *instance, RRDDIM *rd)
{
    BUFFER *buffer = (BUFFER *)instance->buffer;

    format_dimension_collected_json_plaintext(instance, rd);
    pubsub_add_message(instance->connector_specific_data, (char *)buffer_tostring(buffer));
    buffer_flush(buffer);

    return 0;
}

/**
 * Format dimension using a calculated value from stored data for Pub/Sub connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_stored_pubsub(struct instance *instance, RRDDIM *rd)
{
    BUFFER *buffer = (BUFFER *)instance->buffer;

    format_dimension_stored_json_plaintext(instance, rd);
    pubsub_add_message(instance->connector_specific_data, (char *)buffer_tostring(buffer));
    buffer_flush(buffer);

    return 0;
}

/**
 * Pub/Sub connector worker
 *
 * Runs in a separate thread for every instance.
 *
 * @param instance_p an instance data structure.
 */
void pubsub_connector_worker(void *instance_p)
{
    struct instance *instance = (struct instance *)instance_p;
    struct pubsub_specific_config *connector_specific_config = instance->config.connector_specific_config;
    struct pubsub_specific_data *connector_specific_data = instance->connector_specific_data;
    info("EXPORTING: Connector specific data pointer = %p", connector_specific_data);

    while (!netdata_exit) {
        struct stats *stats = &instance->stats;

        uv_mutex_lock(&instance->mutex);
        uv_cond_wait(&instance->cond_var, &instance->mutex);

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

        BUFFER *buffer = (BUFFER *)instance->buffer;
        size_t buffer_len = buffer_strlen(buffer);

        stats->buffered_bytes = buffer_len;

        size_t sent = 0;

        char error_message[ERROR_LINE_MAX + 1] = "";

        debug(
            D_BACKEND,
            "EXPORTING: pubsub_publish(): project = %s, topic = %s, buffer = %zu",
            connector_specific_config->project_id,
            connector_specific_config->topic_id,
            buffer_len);

        pubsub_publish((void *)connector_specific_data);

        sent += buffer_len;
        stats->transmission_successes++;

        size_t sent_metrics = 0, lost_metrics = 0, sent_bytes = 0, lost_bytes = 0;

        if (unlikely(pubsub_get_result(
                connector_specific_data, error_message, &sent_metrics, &sent_bytes, &lost_metrics, &lost_bytes))) {
            // oops! we couldn't send (all or some of the) data
            error("EXPORTING: %s", error_message);
            error(
                "EXPORTING: failed to write data to database '%s'. Willing to write %zu bytes, wrote %zu bytes.",
                instance->config.destination, sent_bytes, sent_bytes - lost_bytes);

            stats->transmission_failures++;
            stats->data_lost_events++;
            stats->lost_metrics += lost_metrics;
            stats->lost_bytes += lost_bytes;

            break;
        } else {
            stats->receptions++;
        }

            error("EXPORTING: An iteration of the pubsub worker");

            if (unlikely(netdata_exit))
                break;

            stats->sent_bytes += sent;
            if (likely(sent == buffer_len))
                stats->sent_metrics = sent_metrics;

            buffer_flush(buffer);

            send_internal_metrics(instance);

            stats->buffered_metrics = 0;

            uv_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        break;
#endif
        }
}
