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
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = NULL;

    instance->prepare_header = NULL;
    instance->check_response = NULL;

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    if (!instance->buffer) {
        netdata_log_error("EXPORTING: cannot create buffer for Pub/Sub exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    struct pubsub_specific_data *connector_specific_data = callocz(1, sizeof(struct pubsub_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    struct pubsub_specific_config *connector_specific_config =
        (struct pubsub_specific_config *)instance->config.connector_specific_config;
    char error_message[ERROR_LINE_MAX + 1] = "";
    if (pubsub_init(
            (void *)connector_specific_data, error_message, instance->config.destination,
            connector_specific_config->credentials_file, connector_specific_config->project_id,
            connector_specific_config->topic_id)) {
        netdata_log_error(
            "EXPORTING: Cannot initialize a Pub/Sub publisher for instance %s: %s",
            instance->config.name, error_message);
        return 1;
    }

    return 0;
}

/**
 * Clean a PubSub connector instance
 *
 * @param instance an instance data structure.
 */
void clean_pubsub_instance(struct instance *instance)
{
    netdata_log_info("EXPORTING: cleaning up instance %s ...", instance->config.name);

    struct pubsub_specific_data *connector_specific_data =
        (struct pubsub_specific_data *)instance->connector_specific_data;
    pubsub_cleanup(connector_specific_data);
    freez(connector_specific_data);

    buffer_free(instance->buffer);

    struct pubsub_specific_config *connector_specific_config =
        (struct pubsub_specific_config *)instance->config.connector_specific_config;
    freez(connector_specific_config->credentials_file);
    freez(connector_specific_config->project_id);
    freez(connector_specific_config->topic_id);
    freez(connector_specific_config);

    netdata_log_info("EXPORTING: instance %s exited", instance->config.name);
    instance->exited = 1;

    return;
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

    char threadname[ND_THREAD_TAG_MAX + 1];
    snprintfz(threadname, ND_THREAD_TAG_MAX, "EXPPBSB[%zu]", instance->index);
    uv_thread_set_name_np(threadname);

    while (!instance->engine->exit) {
        struct stats *stats = &instance->stats;
        char error_message[ERROR_LINE_MAX + 1] = "";

        uv_mutex_lock(&instance->mutex);
        while (!instance->data_is_ready)
            uv_cond_wait(&instance->cond_var, &instance->mutex);
        instance->data_is_ready = 0;


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

        BUFFER *buffer = (BUFFER *)instance->buffer;
        size_t buffer_len = buffer_strlen(buffer);

        stats->buffered_bytes = buffer_len;

        if (pubsub_add_message(instance->connector_specific_data, (char *)buffer_tostring(buffer))) {
            netdata_log_error("EXPORTING: Instance %s: Cannot add data to a message", instance->config.name);

            stats->data_lost_events++;
            stats->lost_metrics += stats->buffered_metrics;
            stats->lost_bytes += buffer_len;

            goto cleanup;
        }

        netdata_log_debug(
            D_EXPORTING, "EXPORTING: pubsub_publish(): project = %s, topic = %s, buffer = %zu",
            connector_specific_config->project_id, connector_specific_config->topic_id, buffer_len);

        if (pubsub_publish((void *)connector_specific_data, error_message, stats->buffered_metrics, buffer_len)) {
            netdata_log_error("EXPORTING: Instance: %s: Cannot publish a message: %s", instance->config.name, error_message);

            stats->transmission_failures++;
            stats->data_lost_events++;
            stats->lost_metrics += stats->buffered_metrics;
            stats->lost_bytes += buffer_len;

            goto cleanup;
        }

        stats->sent_bytes = buffer_len;
        stats->transmission_successes++;

        size_t sent_metrics = 0, lost_metrics = 0, sent_bytes = 0, lost_bytes = 0;

        if (unlikely(pubsub_get_result(
                connector_specific_data, error_message, &sent_metrics, &sent_bytes, &lost_metrics, &lost_bytes))) {
            // oops! we couldn't send (all or some of the) data
            netdata_log_error("EXPORTING: %s", error_message);
            netdata_log_error(
                "EXPORTING: failed to write data to service '%s'. Willing to write %zu bytes, wrote %zu bytes.",
                instance->config.destination, lost_bytes, sent_bytes);

            stats->transmission_failures++;
            stats->data_lost_events++;
            stats->lost_metrics += lost_metrics;
            stats->lost_bytes += lost_bytes;
        } else {
            stats->receptions++;
            stats->sent_metrics = sent_metrics;
        }

    cleanup:
        send_internal_metrics(instance);

        buffer_flush(buffer);
        stats->buffered_metrics = 0;

        uv_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        return;
#endif
    }

    clean_pubsub_instance(instance);
}
