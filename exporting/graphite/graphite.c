// SPDX-License-Identifier: GPL-3.0-or-later

#include "graphite.h"

/**
 * Initialize connectors
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_collected_graphite_plaintext(struct instance *instance, RRDDIM *rd)
{
    struct engine *engine = instance->connector->engine;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (engine->config.options & BACKEND_OPTION_SEND_NAMES && st->name) ? st->name : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (engine->config.options & BACKEND_OPTION_SEND_NAMES && rd->name) ? rd->name : rd->id,
        RRD_ID_LENGTH_MAX);

    buffer_sprintf(
        instance->buffer,
        "%s.%s.%s.%s%s%s " COLLECTED_NUMBER_FORMAT " %llu\n",
        engine->config.prefix,
        engine->config.hostname,
        chart_name,
        dimension_name,
        (host->tags) ? ";" : "",
        (host->tags) ? host->tags : "",
        rd->last_collected_value,
        (unsigned long long)rd->last_collected_time.tv_sec);

    return 0;
}

/**
 * Initialize Grafite connector
 *
 * @param instance a connector data structure.
 * @return Always returns 0.
 */
int init_graphite_connector(struct connector *connector)
{
    connector->start_batch_formatting = NULL;
    connector->start_host_formatting = NULL;
    connector->start_chart_formatting = NULL;
    connector->metric_formatting = format_dimension_collected_graphite_plaintext;
    connector->end_chart_formatting = NULL;
    connector->end_host_formatting = NULL;
    connector->end_batch_formatting = NULL;

    connector->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = mallocz(sizeof(struct simple_connector_config));
    connector->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 2003;

    return 0;
}

/**
 * Initialize Grafite connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_graphite_instance(struct instance *instance)
{
    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for graphite exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    return 0;
}
