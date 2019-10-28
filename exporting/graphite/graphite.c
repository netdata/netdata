// SPDX-License-Identifier: GPL-3.0-or-later

#include "graphite.h"

int format_dimension_collected_graphite_plaintext(struct instance *instance, RRDDIM *rd)
{
    (void)instance;
    (void)rd;

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
        error("EXPORTING: cannot create buffer for graphite exporting connector instance %s", instance->config.name);
        return 1;
    }

    return 0;
}
