// SPDX-License-Identifier: GPL-3.0-or-later

#include "graphite.h"

int format_dimension_collected_graphite_plaintext(struct engine *engine)
{
    (void)engine;

    return 0;
}

int init_graphite_connector(struct connector *connector)
{
    connector->start_batch_formatting = NULL;
    connector->start_host_formatting = NULL;
    connector->start_chart_formatting = NULL;
    connector->metric_formatting = format_dimension_collected_graphite_plaintext;
    connector->end_chart_formatting = NULL;
    connector->end_host_formatting = NULL;
    connector->end_batch_formatting = NULL;

    return 0;
}

int init_graphite_instance(struct instance *instance)
{
    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for graphite exporting connector instance");
        return 1;
    }

    return 0;
}
