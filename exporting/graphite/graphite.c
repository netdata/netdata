// SPDX-License-Identifier: GPL-3.0-or-later

#include "graphite.h"

int format_dimension_collected_graphite_plaintext(struct engine *engine)
{
    (void)engine;

    return 0;
}

int init_graphite_instance(struct instance *instance)
{
    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for graphite exporting connector instance");
        return 1;
    }
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = NULL;
    instance->start_chart_formatting = NULL;
    instance->metric_formatting = format_dimension_collected_graphite_plaintext;
    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = NULL;
    instance->end_batch_formatting = NULL;

    return 0;
}
