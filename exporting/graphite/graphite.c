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

    // TODO: move to init_graphite_connector
    instance->connector->start_batch_formatting = NULL;
    instance->connector->start_host_formatting = NULL;
    instance->connector->start_chart_formatting = NULL;
    instance->connector->metric_formatting = format_dimension_collected_graphite_plaintext;
    instance->connector->end_chart_formatting = NULL;
    instance->connector->end_host_formatting = NULL;
    instance->connector->end_batch_formatting = NULL;

    return 0;
}
