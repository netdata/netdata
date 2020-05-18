// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

/**
 * Clean the allocated variables
 *
 * @param ptr a pointer to the structure with variables to clean.
 */
void instance_clean(struct instance *ptr)
{
    if (ptr->buffer)
        freez(ptr->buffer);

    if (ptr->connector_specific_data)
        freez(ptr->connector_specific_data);

    if (ptr->labels)
        freez(ptr->labels);

    uv_mutex_destroy(&ptr->mutex);
    uv_cond_destroy(&ptr->cond_var);
}
