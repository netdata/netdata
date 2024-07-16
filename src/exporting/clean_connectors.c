// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Clean the instance config.
 *
 * @param config an instance config structure.
 */
static void clean_instance_config(struct instance_config *config)
{
    if(!config)
        return;

    freez((void *)config->type_name);
    freez((void *)config->name);
    freez((void *)config->destination);
    freez((void *)config->username);
    freez((void *)config->password);
    freez((void *)config->prefix);
    freez((void *)config->hostname);

    simple_pattern_free(config->charts_pattern);

    simple_pattern_free(config->hosts_pattern);
}

/**
 * Clean the allocated variables
 *
 * @param instance an instance data structure.
 */
void clean_instance(struct instance *instance)
{
    clean_instance_config(&instance->config);
    buffer_free(instance->labels_buffer);

    uv_cond_destroy(&instance->cond_var);
    // uv_mutex_destroy(&instance->mutex);
}

/**
 * Clean up a simple connector instance on Netdata exit
 *
 * @param instance an instance data structure.
 */
void simple_connector_cleanup(struct instance *instance)
{
    netdata_log_info("EXPORTING: cleaning up instance %s ...", instance->config.name);

    struct simple_connector_data *simple_connector_data =
        (struct simple_connector_data *)instance->connector_specific_data;

    freez(simple_connector_data->auth_string);

    buffer_free(instance->buffer);
    buffer_free(simple_connector_data->buffer);
    buffer_free(simple_connector_data->header);

    struct simple_connector_buffer *next_buffer = simple_connector_data->first_buffer;
    for (int i = 0; i < instance->config.buffer_on_failures; i++) {
        struct simple_connector_buffer *current_buffer = next_buffer;
        next_buffer = next_buffer->next;

        buffer_free(current_buffer->header);
        buffer_free(current_buffer->buffer);
        freez(current_buffer);
    }

    netdata_ssl_close(&simple_connector_data->ssl);

    freez(simple_connector_data);

    struct simple_connector_config *simple_connector_config =
        (struct simple_connector_config *)instance->config.connector_specific_config;
    freez(simple_connector_config);

    netdata_log_info("EXPORTING: instance %s exited", instance->config.name);
    instance->exited = 1;
}
