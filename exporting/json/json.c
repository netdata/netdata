// SPDX-License-Identifier: GPL-3.0-or-later

#include "json.h"

/**
 * Initialize JSON connector
 *
 * @param instance a connector data structure.
 * @return Always returns 0.
 */
int init_json_connector(struct connector *connector)
{
    connector->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = mallocz(sizeof(struct simple_connector_config));
    connector->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 5448;

    return 0;
}

/**
 * Initialize JSON connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_json_instance(struct instance *instance)
{
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_json_plaintext;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_json_plaintext;
    else
        instance->metric_formatting = format_dimension_stored_json_plaintext;

    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = NULL;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for json exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    return 0;
}

/**
 * Format host labels for JSON connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */
int format_host_labels_json_plaintext(struct instance *instance, RRDHOST *host)
{
    if (!instance->labels)
        instance->labels = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    buffer_strcat(instance->labels, "\"labels\":{");

    int count = 0;
    netdata_rwlock_rdlock(&host->labels_rwlock);
    for (struct label *label = host->labels; label; label = label->next) {
        if (!should_send_label(instance, label))
            continue;

        char value[CONFIG_MAX_VALUE * 2 + 1];
        sanitize_json_string(value, label->value, CONFIG_MAX_VALUE);
        if (count > 0)
            buffer_strcat(instance->labels, ",");
        buffer_sprintf(instance->labels, "\"%s\":\"%s\"", label->key, value);

        count++;
    }
    netdata_rwlock_unlock(&host->labels_rwlock);

    buffer_strcat(instance->labels, "},");

    return 0;
}

/**
 * Format dimension using collected data for JSON connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_collected_json_plaintext(struct instance *instance, RRDDIM *rd)
{
    struct engine *engine = instance->connector->engine;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    const char *tags_pre = "", *tags_post = "", *tags = host->tags;
    if (!tags)
        tags = "";

    if (*tags) {
        if (*tags == '{' || *tags == '[' || *tags == '"') {
            tags_pre = "\"host_tags\":";
            tags_post = ",";
        } else {
            tags_pre = "\"host_tags\":\"";
            tags_post = "\",";
        }
    }

    buffer_sprintf(
        instance->buffer,

        "{"
        "\"prefix\":\"%s\","
        "\"hostname\":\"%s\","
        "%s%s%s"
        "%s"

        "\"chart_id\":\"%s\","
        "\"chart_name\":\"%s\","
        "\"chart_family\":\"%s\","
        "\"chart_context\":\"%s\","
        "\"chart_type\":\"%s\","
        "\"units\":\"%s\","

        "\"id\":\"%s\","
        "\"name\":\"%s\","
        "\"value\":" COLLECTED_NUMBER_FORMAT ","

        "\"timestamp\":%llu}\n",

        engine->config.prefix,
        engine->config.hostname,
        tags_pre,
        tags,
        tags_post,
        instance->labels ? buffer_tostring(instance->labels) : "",

        st->id,
        st->name,
        st->family,
        st->context,
        st->type,
        st->units,

        rd->id,
        rd->name,
        rd->last_collected_value,

        (unsigned long long)rd->last_collected_time.tv_sec);

    return 0;
}

/**
 * Format dimension using a calculated value from stored data for JSON connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_stored_json_plaintext(struct instance *instance, RRDDIM *rd)
{
    struct engine *engine = instance->connector->engine;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    time_t last_t;
    calculated_number value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

    if(isnan(value))
        return 0;

    const char *tags_pre = "", *tags_post = "", *tags = host->tags;
    if (!tags)
        tags = "";

    if (*tags) {
        if (*tags == '{' || *tags == '[' || *tags == '"') {
            tags_pre = "\"host_tags\":";
            tags_post = ",";
        } else {
            tags_pre = "\"host_tags\":\"";
            tags_post = "\",";
        }
    }

    buffer_sprintf(
        instance->buffer,
        "{"
        "\"prefix\":\"%s\","
        "\"hostname\":\"%s\","
        "%s%s%s"
        "%s"

        "\"chart_id\":\"%s\","
        "\"chart_name\":\"%s\","
        "\"chart_family\":\"%s\","
        "\"chart_context\": \"%s\","
        "\"chart_type\":\"%s\","
        "\"units\": \"%s\","

        "\"id\":\"%s\","
        "\"name\":\"%s\","
        "\"value\":" CALCULATED_NUMBER_FORMAT ","

        "\"timestamp\": %llu}\n",

        engine->config.prefix,
        engine->config.hostname,
        tags_pre,
        tags,
        tags_post,
        instance->labels ? buffer_tostring(instance->labels) : "",

        st->id,
        st->name,
        st->family,
        st->context,
        st->type,
        st->units,

        rd->id,
        rd->name,
        value,

        (unsigned long long)last_t);

    return 0;
}
