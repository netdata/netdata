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
    freez((void *)config->prefix);

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
    buffer_free(instance->labels);

    uv_cond_destroy(&instance->cond_var);
    // uv_mutex_destroy(&instance->mutex);
}
