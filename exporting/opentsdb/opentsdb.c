// SPDX-License-Identifier: GPL-3.0-or-later

#include "opentsdb.h"

/**
 * Initialize OpenTSDB connector
 *
 * @param instance a connector data structure.
 * @return Always returns 0.
 */
int init_opentsdb_connector(struct connector *connector)
{
    connector->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = mallocz(sizeof(struct simple_connector_config));
    connector->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 4242;

    return 0;
}

/**
 * Initialize OpenTSDB telnet connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_opentsdb_telnet_instance(struct instance *instance)
{
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_opentsdb_telnet;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_opentsdb_telnet;
    else
        instance->metric_formatting = format_dimension_stored_opentsdb_telnet;

    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = NULL;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for opentsdb telnet exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    return 0;
}

/**
 * Initialize OpenTSDB HTTP connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_opentsdb_http_instance(struct instance *instance)
{
    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_opentsdb_http;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_opentsdb_http;
    else
        instance->metric_formatting = format_dimension_stored_opentsdb_http;

    instance->end_chart_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = NULL;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for opentsdb HTTP exporting connector instance %s", instance->config.name);
        return 1;
    }
    uv_mutex_init(&instance->mutex);
    uv_cond_init(&instance->cond_var);

    return 0;
}

/**
 * Copy a label value and substitute underscores in place of charachters which can't be used in OpenTSDB output
 *
 * @param dst a destination string.
 * @param src a source string.
 * @param len the maximum number of characters copied.
 */

void sanitize_opentsdb_label_value(char *dst, char *src, size_t len)
{
    while (*src != '\0' && len) {
        if (isalpha(*src) || isdigit(*src) || *src == '-' || *src == '_' || *src == '.' || *src == '/' || IS_UTF8_BYTE(*src))
            *dst++ = *src;
        else
            *dst++ = '_';
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
int format_host_labels_opentsdb_telnet(struct instance *instance, RRDHOST *host)
{
    if (!instance->labels)
        instance->labels = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    netdata_rwlock_rdlock(&host->labels_rwlock);
    for (struct label *label = host->labels; label; label = label->next) {
        if (!should_send_label(instance, label))
            continue;

        char value[CONFIG_MAX_VALUE + 1];
        sanitize_opentsdb_label_value(value, label->value, CONFIG_MAX_VALUE);

        if (*value)
            buffer_sprintf(instance->labels, " %s=%s", label->key, value);
    }
    netdata_rwlock_unlock(&host->labels_rwlock);

    return 0;
}

/**
 * Format dimension using collected data for OpenTSDB telnet connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_collected_opentsdb_telnet(struct instance *instance, RRDDIM *rd)
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
        "put %s.%s.%s %llu " COLLECTED_NUMBER_FORMAT " host=%s%s%s%s\n",
        engine->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)rd->last_collected_time.tv_sec,
        rd->last_collected_value,
        engine->config.hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        (instance->labels) ? buffer_tostring(instance->labels) : "");

    return 0;
}

/**
 * Format dimension using a calculated value from stored data for OpenTSDB telnet connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_stored_opentsdb_telnet(struct instance *instance, RRDDIM *rd)
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
        "put %s.%s.%s %llu " CALCULATED_NUMBER_FORMAT " host=%s%s%s%s\n",
        engine->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)last_t,
        value,
        engine->config.hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        (instance->labels) ? buffer_tostring(instance->labels) : "");

    return 0;
}

/**
 * Prepare an HTTP message for OpenTSDB HTTP connector
 *
 * @param buffer a buffer to write the message to.
 * @param message the body of the message.
 * @param hostname the name of the host that sends the message.
 * @param length the length of the message body.
 */
static inline void opentsdb_build_message(BUFFER *buffer, char *message, const char *hostname, int length)
{
    buffer_sprintf(
        buffer,
        "POST /api/put HTTP/1.1\r\n"
        "Host: %s\r\n"
        "Content-Type: application/json\r\n"
        "Content-Length: %d\r\n"
        "\r\n"
        "%s",
        hostname,
        length,
        message);
}

/**
 * Format host labels for OpenTSDB HTTP connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */
int format_host_labels_opentsdb_http(struct instance *instance, RRDHOST *host)
{
    if (!instance->labels)
        instance->labels = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    netdata_rwlock_rdlock(&host->labels_rwlock);
    for (struct label *label = host->labels; label; label = label->next) {
        if (!should_send_label(instance, label))
            continue;

        char escaped_value[CONFIG_MAX_VALUE * 2 + 1];
        sanitize_json_string(escaped_value, label->value, CONFIG_MAX_VALUE);

        char value[CONFIG_MAX_VALUE + 1];
        sanitize_opentsdb_label_value(value, escaped_value, CONFIG_MAX_VALUE);

        if (*value) {
            buffer_strcat(instance->labels, ",");
            buffer_sprintf(instance->labels, "\"%s\":\"%s\"", label->key, value);
        }
    }
    netdata_rwlock_unlock(&host->labels_rwlock);

    return 0;
}

/**
 * Format dimension using collected data for OpenTSDB HTTP connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_collected_opentsdb_http(struct instance *instance, RRDDIM *rd)
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

    char message[1024];
    int length = snprintfz(
        message,
        sizeof(message),
        "{"
        "  \"metric\": \"%s.%s.%s\","
        "  \"timestamp\": %llu,"
        "  \"value\": " COLLECTED_NUMBER_FORMAT ","
        "  \"tags\": {"
        "    \"host\": \"%s%s%s\"%s"
        "  }"
        "}",
        engine->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)rd->last_collected_time.tv_sec,
        rd->last_collected_value,
        engine->config.hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        instance->labels ? buffer_tostring(instance->labels) : "");

    if (length > 0) {
        opentsdb_build_message(instance->buffer, message, engine->config.hostname, length);
    }

    return 0;
}

/**
 * Format dimension using a calculated value from stored data for OpenTSDB HTTP connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_stored_opentsdb_http(struct instance *instance, RRDDIM *rd)
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

    char message[1024];
    int length = snprintfz(
        message,
        sizeof(message),
        "{"
        "  \"metric\": \"%s.%s.%s\","
        "  \"timestamp\": %llu,"
        "  \"value\": " CALCULATED_NUMBER_FORMAT ","
        "  \"tags\": {"
        "    \"host\": \"%s%s%s\"%s"
        "  }"
        "}",
        engine->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)last_t,
        value,
        engine->config.hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        instance->labels ? buffer_tostring(instance->labels) : "");

    if (length > 0) {
        opentsdb_build_message(instance->buffer, message, engine->config.hostname, length);
    }

    return 0;
}
