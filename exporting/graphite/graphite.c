// SPDX-License-Identifier: GPL-3.0-or-later

#include "graphite.h"

/**
 * Initialize Graphite connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_graphite_instance(struct instance *instance)
{
    instance->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = callocz(1, sizeof(struct simple_connector_config));
    instance->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 2003;

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
    instance->start_host_formatting = format_host_labels_graphite_plaintext;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_graphite_plaintext;
    else
        instance->metric_formatting = format_dimension_stored_graphite_plaintext;

    instance->end_chart_formatting = NULL;
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = simple_connector_end_batch;

    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_GRAPHITE_HTTP)
        instance->prepare_header = graphite_http_prepare_header;
    else
        instance->prepare_header = NULL;

    instance->check_response = exporting_discard_response;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for graphite exporting connector instance %s", instance->config.name);
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
 * Copy a label value and substitute underscores in place of characters which can't be used in Graphite output
 *
 * @param dst a destination string.
 * @param src a source string.
 * @param len the maximum number of characters copied.
 */

void sanitize_graphite_label_value(char *dst, const char *src, size_t len)
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
    if (!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    rrdlabels_to_buffer(host->host_labels, instance->labels_buffer, ";", "=", "", "",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_graphite_label_value);

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
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    buffer_sprintf(
        instance->buffer,
        "%s.%s.%s.%s%s%s%s " COLLECTED_NUMBER_FORMAT " %llu\n",
        instance->config.prefix,
        (host == localhost) ? instance->config.hostname : host->hostname,
        chart_name,
        dimension_name,
        (host->tags) ? ";" : "",
        (host->tags) ? host->tags : "",
        (instance->labels_buffer) ? buffer_tostring(instance->labels_buffer) : "",
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
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    time_t last_t;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

    if(isnan(value))
        return 0;

    buffer_sprintf(
        instance->buffer,
        "%s.%s.%s.%s%s%s%s " NETDATA_DOUBLE_FORMAT " %llu\n",
        instance->config.prefix,
        (host == localhost) ? instance->config.hostname : host->hostname,
        chart_name,
        dimension_name,
        (host->tags) ? ";" : "",
        (host->tags) ? host->tags : "",
        (instance->labels_buffer) ? buffer_tostring(instance->labels_buffer) : "",
        value,
        (unsigned long long)last_t);

    return 0;
}

/**
 * Prepare HTTP header
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
void graphite_http_prepare_header(struct instance *instance)
{
    struct simple_connector_data *simple_connector_data = instance->connector_specific_data;

    buffer_sprintf(
        simple_connector_data->last_buffer->header,
        "POST /api/put HTTP/1.1\r\n"
        "Host: %s\r\n"
        "%s"
        "Content-Type: application/graphite\r\n"
        "Content-Length: %lu\r\n"
        "\r\n",
        instance->config.destination,
        simple_connector_data->auth_string ? simple_connector_data->auth_string : "",
        buffer_strlen(simple_connector_data->last_buffer->buffer));

    return;
}
