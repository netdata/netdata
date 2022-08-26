// SPDX-License-Identifier: GPL-3.0-or-later

#include "opentsdb.h"
#include "../json/json.h"

/**
 * Initialize OpenTSDB telnet connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_opentsdb_telnet_instance(struct instance *instance)
{
    instance->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = callocz(1, sizeof(struct simple_connector_config));
    instance->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 4242;

    struct simple_connector_data *connector_specific_data = callocz(1, sizeof(struct simple_connector_data));
    instance->connector_specific_data = connector_specific_data;

#ifdef ENABLE_HTTPS
    connector_specific_data->flags = NETDATA_SSL_START;
    connector_specific_data->conn = NULL;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        security_start_ssl(NETDATA_SSL_CONTEXT_EXPORTING);
    }
#endif

    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_opentsdb_telnet;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_opentsdb_telnet;
    else
        instance->metric_formatting = format_dimension_stored_opentsdb_telnet;

    instance->end_chart_formatting = NULL;
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = simple_connector_end_batch;

    instance->prepare_header = NULL;
    instance->check_response = exporting_discard_response;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for opentsdb telnet exporting connector instance %s", instance->config.name);
        return 1;
    }

    simple_connector_init(instance);

    if (uv_mutex_init(&instance->mutex))
        return 1;
    if (uv_cond_init(&instance->cond_var))
        return 1;

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
    instance->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = callocz(1, sizeof(struct simple_connector_config));
    instance->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 4242;

    struct simple_connector_data *connector_specific_data = callocz(1, sizeof(struct simple_connector_data));
#ifdef ENABLE_HTTPS
    connector_specific_data->flags = NETDATA_SSL_START;
    connector_specific_data->conn = NULL;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        security_start_ssl(NETDATA_SSL_CONTEXT_EXPORTING);
    }
#endif
    instance->connector_specific_data = connector_specific_data;

    instance->start_batch_formatting = open_batch_json_http;
    instance->start_host_formatting = format_host_labels_opentsdb_http;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_opentsdb_http;
    else
        instance->metric_formatting = format_dimension_stored_opentsdb_http;

    instance->end_chart_formatting = NULL;
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = close_batch_json_http;

    instance->prepare_header = opentsdb_http_prepare_header;
    instance->check_response = exporting_discard_response;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for opentsdb HTTP exporting connector instance %s", instance->config.name);
        return 1;
    }

    simple_connector_init(instance);

    if (uv_mutex_init(&instance->mutex))
        return 1;
    if (uv_cond_init(&instance->cond_var))
        return 1;

    return 0;
}

/**
 * Copy a label value and substitute underscores in place of characters which can't be used in OpenTSDB output
 *
 * @param dst a destination string.
 * @param src a source string.
 * @param len the maximum number of characters copied.
 */

void sanitize_opentsdb_label_value(char *dst, const char *src, size_t len)
{
    while (*src != '\0' && len) {
        if (isalpha(*src) || isdigit(*src) || *src == '-' || *src == '.' || *src == '/' || IS_UTF8_BYTE(*src))
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

int format_host_labels_opentsdb_telnet(struct instance *instance, RRDHOST *host) {
    if(!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    buffer_strcat(instance->labels_buffer, " ");
    rrdlabels_to_buffer(host->host_labels, instance->labels_buffer, "", "=", "", " ",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_opentsdb_label_value);
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
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    buffer_sprintf(
        instance->buffer,
        "put %s.%s.%s %llu " COLLECTED_NUMBER_FORMAT " host=%s%s%s%s\n",
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)rd->last_collected_time.tv_sec,
        rd->last_collected_value,
        (host == localhost) ? instance->config.hostname : host->hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        (instance->labels_buffer) ? buffer_tostring(instance->labels_buffer) : "");

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
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    time_t last_t;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

    if(isnan(value))
        return 0;

    buffer_sprintf(
        instance->buffer,
        "put %s.%s.%s %llu " NETDATA_DOUBLE_FORMAT " host=%s%s%s%s\n",
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)last_t,
        value,
        (host == localhost) ? instance->config.hostname : host->hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        (instance->labels_buffer) ? buffer_tostring(instance->labels_buffer) : "");

    return 0;
}

/**
 * Prepare HTTP header
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
void opentsdb_http_prepare_header(struct instance *instance)
{
    struct simple_connector_data *simple_connector_data = instance->connector_specific_data;

    buffer_sprintf(
        simple_connector_data->last_buffer->header,
        "POST /api/put HTTP/1.1\r\n"
        "Host: %s\r\n"
        "%s"
        "Content-Type: application/json\r\n"
        "Content-Length: %lu\r\n"
        "\r\n",
        instance->config.destination,
        simple_connector_data->auth_string ? simple_connector_data->auth_string : "",
        buffer_strlen(simple_connector_data->last_buffer->buffer));

    return;
}

/**
 * Format host labels for OpenTSDB HTTP connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */

int format_host_labels_opentsdb_http(struct instance *instance, RRDHOST *host) {
    if (!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    rrdlabels_to_buffer(host->host_labels, instance->labels_buffer, ",", ":", "\"", "",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_opentsdb_label_value);
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
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    if (buffer_strlen((BUFFER *)instance->buffer) > 2)
        buffer_strcat(instance->buffer, ",\n");

    buffer_sprintf(
        instance->buffer,
        "{"
        "\"metric\":\"%s.%s.%s\","
        "\"timestamp\":%llu,"
        "\"value\":"COLLECTED_NUMBER_FORMAT","
        "\"tags\":{"
        "\"host\":\"%s%s%s\"%s"
        "}"
        "}",
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)rd->last_collected_time.tv_sec,
        rd->last_collected_value,
        (host == localhost) ? instance->config.hostname : host->hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "");

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
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : st->id,
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    time_t last_t;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

    if(isnan(value))
        return 0;

    if (buffer_strlen((BUFFER *)instance->buffer) > 2)
        buffer_strcat(instance->buffer, ",\n");

    buffer_sprintf(
        instance->buffer,
        "{"
        "\"metric\":\"%s.%s.%s\","
        "\"timestamp\":%llu,"
        "\"value\":" NETDATA_DOUBLE_FORMAT ","
        "\"tags\":{"
        "\"host\":\"%s%s%s\"%s"
        "}"
        "}",
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)last_t,
        value,
        (host == localhost) ? instance->config.hostname : host->hostname,
        (host->tags) ? " " : "",
        (host->tags) ? host->tags : "",
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "");

    return 0;
}
