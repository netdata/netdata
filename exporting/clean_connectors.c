// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Clean the instance config.
 * @param ptr
 */
static void clean_instance_config(struct instance_config *ptr)
{
    if (ptr->name)
        freez((void *)ptr->name);

    if (ptr->destination)
        freez((void *)ptr->destination);

    if (ptr->charts_pattern)
        simple_pattern_free(ptr->charts_pattern);

    if (ptr->hosts_pattern)
        simple_pattern_free(ptr->hosts_pattern);
}

/**
 * Clean the allocated variables
 *
 * @param ptr a pointer to the structure with variables to clean.
 */
void clean_instance(struct instance *ptr)
{
    clean_instance_config(&ptr->config);
    if (ptr->labels)
        buffer_free(ptr->labels);

    uv_mutex_destroy(&ptr->mutex);
    uv_cond_destroy(&ptr->cond_var);
}
