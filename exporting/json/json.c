// SPDX-License-Identifier: GPL-3.0-or-later

#include "json.h"

/**
 * Initialize JSON connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_json_instance(struct instance *instance)
{
    instance->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = callocz(1, sizeof(struct simple_connector_config));
    instance->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 5448;

    struct simple_connector_data *connector_specific_data = callocz(1, sizeof(struct simple_connector_data));
    instance->connector_specific_data = connector_specific_data;

    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_json_plaintext;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_json_plaintext;
    else
        instance->metric_formatting = format_dimension_stored_json_plaintext;

    instance->end_chart_formatting = NULL;
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = simple_connector_end_batch;

    instance->prepare_header = NULL;

    instance->check_response = exporting_discard_response;

    instance->buffer = (void *)buffer_create(0);
    if (!instance->buffer) {
        error("EXPORTING: cannot create buffer for json exporting connector instance %s", instance->config.name);
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
 * Initialize JSON connector instance for HTTP protocol
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_json_http_instance(struct instance *instance)
{
    instance->worker = simple_connector_worker;

    struct simple_connector_config *connector_specific_config = callocz(1, sizeof(struct simple_connector_config));
    instance->config.connector_specific_config = (void *)connector_specific_config;
    connector_specific_config->default_port = 5448;

    struct simple_connector_data *connector_specific_data = callocz(1, sizeof(struct simple_connector_data));
    instance->connector_specific_data = connector_specific_data;

#ifdef ENABLE_HTTPS
    connector_specific_data->flags = NETDATA_SSL_START;
    connector_specific_data->conn = NULL;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        security_start_ssl(NETDATA_SSL_CONTEXT_EXPORTING);
    }
#endif

    instance->start_batch_formatting = open_batch_json_http;
    instance->start_host_formatting = format_host_labels_json_plaintext;
    instance->start_chart_formatting = NULL;

    if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
        instance->metric_formatting = format_dimension_collected_json_plaintext;
    else
        instance->metric_formatting = format_dimension_stored_json_plaintext;

    instance->end_chart_formatting = NULL;
    instance->variables_formatting = NULL;
    instance->end_host_formatting = flush_host_labels;
    instance->end_batch_formatting = close_batch_json_http;

    instance->prepare_header = json_http_prepare_header;

    instance->check_response = exporting_discard_response;

    instance->buffer = (void *)buffer_create(0);

    simple_connector_init(instance);

    if (uv_mutex_init(&instance->mutex))
        return 1;
    if (uv_cond_init(&instance->cond_var))
        return 1;

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
    if (!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    buffer_strcat(instance->labels_buffer, "\"labels\":{");
    rrdlabels_to_buffer(host->host_labels, instance->labels_buffer, "", ":", "\"", ",",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_json_string);
    buffer_strcat(instance->labels_buffer, "},");

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

    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_JSON_HTTP) {
        if (buffer_strlen((BUFFER *)instance->buffer) > 2)
        buffer_strcat(instance->buffer, ",\n");
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

        "\"timestamp\":%llu}",

        instance->config.prefix,
        (host == localhost) ? instance->config.hostname : host->hostname,
        tags_pre,
        tags,
        tags_post,
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "",

        st->id,
        st->name,
        st->family,
        st->context,
        rrdset_type(st),
        rrdset_units(st),
        rrddim_id(rd),
        rrddim_name(rd),
        rd->last_collected_value,

        (unsigned long long)rd->last_collected_time.tv_sec);

    if (instance->config.type != EXPORTING_CONNECTOR_TYPE_JSON_HTTP) {
        buffer_strcat(instance->buffer, "\n");
    }

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
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    time_t last_t;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

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

    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_JSON_HTTP) {
        if (buffer_strlen((BUFFER *)instance->buffer) > 2)
            buffer_strcat(instance->buffer, ",\n");
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
        "\"value\":" NETDATA_DOUBLE_FORMAT ","

        "\"timestamp\": %llu}",

        instance->config.prefix,
        (host == localhost) ? instance->config.hostname : host->hostname,
        tags_pre,
        tags,
        tags_post,
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "",

        st->id,
        st->name,
        st->family,
        st->context,
        rrdset_type(st),
        rrdset_units(st),
        rrddim_id(rd),
        rrddim_name(rd),
        value,

        (unsigned long long)last_t);

    if (instance->config.type != EXPORTING_CONNECTOR_TYPE_JSON_HTTP) {
        buffer_strcat(instance->buffer, "\n");
    }

    return 0;
}

/**
 * Open a JSON list for a bach
 *
 * @param instance an instance data structure.
 * @return Always returns 0.
 */
int open_batch_json_http(struct instance *instance)
{
    buffer_strcat(instance->buffer, "[\n");

    return 0;
}

/**
 * Close a JSON list for a bach and update buffered bytes counter
 *
 * @param instance an instance data structure.
 * @return Always returns 0.
 */
int close_batch_json_http(struct instance *instance)
{
    buffer_strcat(instance->buffer, "\n]\n");

    simple_connector_end_batch(instance);

    return 0;
}

/**
 * Prepare HTTP header
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
void json_http_prepare_header(struct instance *instance)
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
