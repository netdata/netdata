// SPDX-License-Identifier: GPL-3.0-or-later

#include "graphite.h"

/**
 * Initialize Graphite connector
 *
 * @param instance a connector data structure.
 * @return Always returns 0.
 */
int init_graphite_connector(struct connector *connector)
{
    connector->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = mallocz(sizeof(struct simple_connector_config));
    connector->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 2003;

    return 0;
}

/**
 * Initialize Graphite connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_graphite_instance(struct instance *instance)
{
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_graphite_plaintext;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_graphite_plaintext;
    else
        instance->metric_formatting = format_dimension_stored_graphite_plaintext;

    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = NULL;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for graphite exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    return 0;
}

/**
 * Copy a label value and substitute underscores in place of charachters which can't be used in Graphite output
 *
 * @param dst a destination string.
 * @param src a source string.
 * @param len the maximum number of characters copied.
 */

void sanitize_graphite_label_value(char *dst, char *src, size_t len)
{
    while (*src != '\0' && len) {
        if (isspace(*src) || *src == ';' || *src == '~')
            *dst++ = '_';
        else
            *dst++ = *src;
        src++;
        len--;
    }
    *dst = '\0';
}

/**
 * Format host labels for JSON connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */
int format_host_labels_graphite_plaintext(struct instance *instance, RRDHOST *host)
{
    if (!instance->labels)
        instance->labels = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    rrdhost_check_rdlock(host);
    netdata_rwlock_rdlock(&host->labels_rwlock);
    for (struct label *label = host->labels; label; label = label->next) {
        if (!should_send_label(instance, label))
            continue;

        char value[CONFIG_MAX_VALUE + 1];
        sanitize_graphite_label_value(value, label->value, CONFIG_MAX_VALUE);

        if (*value) {
            buffer_strcat(instance->labels, ";");
            buffer_sprintf(instance->labels, "%s=%s", label->key, value);
        }
    }
    netdata_rwlock_unlock(&host->labels_rwlock);

    return 0;
}

/**
 * Format dimension using collected data for Graphite connector
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
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? st->name : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rd->name : rd->id,
        RRD_ID_LENGTH_MAX);

    buffer_sprintf(
        instance->buffer,
        "%s.%s.%s.%s%s%s%s " COLLECTED_NUMBER_FORMAT " %llu\n",
        engine->config.prefix,
        engine->config.hostname,
        chart_name,
        dimension_name,
        (host->tags) ? ";" : "",
        (host->tags) ? host->tags : "",
        (instance->labels) ? buffer_tostring(instance->labels) : "",
        rd->last_collected_value,
        (unsigned long long)rd->last_collected_time.tv_sec);

    return 0;
}

/**
 * Format dimension using a calculated value from stored data for Graphite connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_stored_graphite_plaintext(struct instance *instance, RRDDIM *rd)
{
    struct engine *engine = instance->connector->engine;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? st->name : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rd->name : rd->id,
        RRD_ID_LENGTH_MAX);

    time_t last_t;
    calculated_number value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

    if(isnan(value))
        return 0;

    buffer_sprintf(
        instance->buffer,
        "%s.%s.%s.%s%s%s%s " CALCULATED_NUMBER_FORMAT " %llu\n",
        engine->config.prefix,
        engine->config.hostname,
        chart_name,
        dimension_name,
        (host->tags) ? ";" : "",
        (host->tags) ? host->tags : "",
        (instance->labels) ? buffer_tostring(instance->labels) : "",
        value,
        (unsigned long long)last_t);

    return 0;
}
