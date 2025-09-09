// SPDX-License-Identifier: GPL-3.0-or-later

#include "aws_kinesis.h"

/**
 * Clean AWS Kinesis *
 */
void aws_kinesis_cleanup(struct instance *instance)
{
    netdata_log_info("EXPORTING: cleaning up instance %s ...", instance->config.name);
    kinesis_shutdown(instance->connector_specific_data);

    freez(instance->connector_specific_data);

    struct aws_kinesis_specific_config *connector_specific_config = instance->config.connector_specific_config;
    if (connector_specific_config) {
        freez(connector_specific_config->auth_key_id);
        freez(connector_specific_config->secure_key);
        freez(connector_specific_config->stream_name);

        freez(connector_specific_config);
    }

    netdata_log_info("EXPORTING: instance %s exited", instance->config.name);
    instance->exited = 1;
}

/**
 * Initialize AWS Kinesis connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_aws_kinesis_instance(struct instance *instance)
{
    instance->worker = aws_kinesis_connector_worker;

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
        netdata_log_error("EXPORTING: cannot create buffer for AWS Kinesis exporting connector instance %s",
                          instance->config.name);
        return 1;
    }
    if (netdata_mutex_init(&instance->mutex))
        return 1;
    if (netdata_cond_init(&instance->cond_var))
        return 1;

    if (!instance->engine->aws_sdk_initialized) {
        aws_sdk_init();
        instance->engine->aws_sdk_initialized = 1;
    }

    struct aws_kinesis_specific_config *connector_specific_config = instance->config.connector_specific_config;
    struct aws_kinesis_specific_data *connector_specific_data = callocz(1, sizeof(struct aws_kinesis_specific_data));
    instance->connector_specific_data = (void *)connector_specific_data;

    if (!strcmp(connector_specific_config->stream_name, "")) {
        netdata_log_error("stream name is a mandatory Kinesis parameter but it is not configured");
        return 1;
    }

    kinesis_init(
        (void *)connector_specific_data,
        instance->config.destination,
        connector_specific_config->auth_key_id,
        connector_specific_config->secure_key,
        instance->config.timeoutms);

    return 0;
}

/**
 * AWS Kinesis connector worker
 *
 * Runs in a separate thread for every instance.
 *
 * @param instance_p an instance data structure.
 */
void *aws_kinesis_connector_worker(void *instance_p)
{
    struct instance *instance = (struct instance *)instance_p;
    struct aws_kinesis_specific_config *connector_specific_config = instance->config.connector_specific_config;
    struct aws_kinesis_specific_data *connector_specific_data = instance->connector_specific_data;

    while (!instance->engine->exit) {
        unsigned long long partition_key_seq = 0;
        struct stats *stats = &instance->stats;

        netdata_mutex_lock(&instance->mutex);
        while (!instance->data_is_ready)
            netdata_cond_wait(&instance->cond_var, &instance->mutex);
        instance->data_is_ready = 0;

        if (unlikely(instance->engine->exit)) {
            netdata_mutex_unlock(&instance->mutex);
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

        size_t sent = 0;

        while (sent < buffer_len) {
            char partition_key[KINESIS_PARTITION_KEY_MAX + 1];
            snprintf(partition_key, KINESIS_PARTITION_KEY_MAX, "netdata_%llu", partition_key_seq++);
            size_t partition_key_len = strnlen(partition_key, KINESIS_PARTITION_KEY_MAX);

            const char *first_char = buffer_tostring(buffer) + sent;

            size_t record_len = 0;

            // split buffer into chunks of maximum allowed size
            if (buffer_len - sent < KINESIS_RECORD_MAX - partition_key_len) {
                record_len = buffer_len - sent;
            } else {
                record_len = KINESIS_RECORD_MAX - partition_key_len;
                while (record_len && *(first_char + record_len - 1) != '\n')
                    record_len--;
            }
            char error_message[ERROR_LINE_MAX + 1] = "";

            netdata_log_debug(D_EXPORTING,
                              "EXPORTING: kinesis_put_record(): dest = %s, id = %s, key = %s, stream = %s, partition_key = %s, "
                              "buffer = %zu, record = %zu",
                              instance->config.destination,
                              connector_specific_config->auth_key_id,
                              connector_specific_config->secure_key,
                              connector_specific_config->stream_name,
                              partition_key,
                              buffer_len,
                              record_len);

            kinesis_put_record(
                connector_specific_data, connector_specific_config->stream_name, partition_key, first_char, record_len);

            sent += record_len;
            stats->transmission_successes++;

            size_t sent_bytes = 0, lost_bytes = 0;

            if (unlikely(kinesis_get_result(
                    connector_specific_data->request_outcomes, error_message, &sent_bytes, &lost_bytes))) {
                // oops! we couldn't send (all or some of the) data
                netdata_log_error("EXPORTING: %s", error_message);
                netdata_log_error("EXPORTING: failed to write data to external database '%s'. Willing to write %zu bytes, wrote %zu bytes.",
                                  instance->config.destination,
                                  sent_bytes,
                                  sent_bytes - lost_bytes);

                stats->transmission_failures++;
                stats->data_lost_events++;
                stats->lost_bytes += lost_bytes;

                // estimate the number of lost metrics
                stats->lost_metrics += (collected_number)(
                    stats->buffered_metrics *
                    (buffer_len && (lost_bytes > buffer_len) ? (double)lost_bytes / buffer_len : 1));

                break;
            } else {
                stats->receptions++;
            }

            if (unlikely(instance->engine->exit))
                break;
        }

        stats->sent_bytes += sent;
        if (likely(sent == buffer_len))
            stats->sent_metrics = stats->buffered_metrics;

        buffer_flush(buffer);

        send_internal_metrics(instance);

        stats->buffered_metrics = 0;

        netdata_mutex_unlock(&instance->mutex);

#ifdef UNIT_TESTING
        return;
#endif
    }

    aws_kinesis_cleanup(instance);
    return NULL;
}
