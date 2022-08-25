// SPDX-License-Identifier: GPL-3.0-or-later

#include "remote_write.h"

static int as_collected;
static int homogeneous;
char context[PROMETHEUS_ELEMENT_MAX + 1];
char chart[PROMETHEUS_ELEMENT_MAX + 1];
char family[PROMETHEUS_ELEMENT_MAX + 1];
char units[PROMETHEUS_ELEMENT_MAX + 1] = "";

/**
 * Prepare HTTP header
 *
 * @param instance an instance data structure.
 */
void prometheus_remote_write_prepare_header(struct instance *instance)
{
    struct prometheus_remote_write_specific_config *connector_specific_config =
        instance->config.connector_specific_config;
    struct simple_connector_data *simple_connector_data = instance->connector_specific_data;

    buffer_sprintf(
        simple_connector_data->last_buffer->header,
        "POST %s HTTP/1.1\r\n"
        "Host: %s\r\n"
        "Accept: */*\r\n"
        "%s"
        "Content-Encoding: snappy\r\n"
        "Content-Type: application/x-protobuf\r\n"
        "X-Prometheus-Remote-Write-Version: 0.1.0\r\n"
        "Content-Length: %zu\r\n"
        "\r\n",
        connector_specific_config->remote_write_path,
        simple_connector_data->connected_to,
        simple_connector_data->auth_string ? simple_connector_data->auth_string : "",
        buffer_strlen(simple_connector_data->last_buffer->buffer));
}

/**
 * Process a response received after Prometheus remote write connector had sent data
 *
 * @param buffer a response from a remote service.
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int process_prometheus_remote_write_response(BUFFER *buffer, struct instance *instance)
{
    if (unlikely(!buffer))
        return 1;

    const char *s = buffer_tostring(buffer);
    int len = buffer_strlen(buffer);

    // do nothing with HTTP responses 200 or 204

    while (!isspace(*s) && len) {
        s++;
        len--;
    }
    s++;
    len--;

    if (likely(len > 4 && (!strncmp(s, "200 ", 4) || !strncmp(s, "204 ", 4))))
        return 0;
    else
        return exporting_discard_response(buffer, instance);
}

/**
 * Release specific data allocated.
 *
 * @param instance an instance data structure.
 */
void clean_prometheus_remote_write(struct instance *instance)
{
    struct simple_connector_data *simple_connector_data = instance->connector_specific_data;
    freez(simple_connector_data->connector_specific_data);

    struct prometheus_remote_write_specific_config *connector_specific_config =
        instance->config.connector_specific_config;
    freez(connector_specific_config->remote_write_path);
}

/**
 * Initialize Prometheus Remote Write connector instance
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_prometheus_remote_write_instance(struct instance *instance)
{
    instance->worker = simple_connector_worker;

    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_prometheus_remote_write;
    instance->start_chart_formatting = format_chart_prometheus_remote_write;
    instance->metric_formatting = format_dimension_prometheus_remote_write;
    instance->end_chart_formatting = NULL;
    instance->variables_formatting = format_variables_prometheus_remote_write;
    instance->end_host_formatting = NULL;
    instance->end_batch_formatting = format_batch_prometheus_remote_write;

    instance->prepare_header = prometheus_remote_write_prepare_header;
    instance->check_response = process_prometheus_remote_write_response;

    instance->buffer = (void *)buffer_create(0);

    if (uv_mutex_init(&instance->mutex))
        return 1;
    if (uv_cond_init(&instance->cond_var))
        return 1;

    struct simple_connector_data *simple_connector_data = callocz(1, sizeof(struct simple_connector_data));
    instance->connector_specific_data = simple_connector_data;

#ifdef ENABLE_HTTPS
    simple_connector_data->flags = NETDATA_SSL_START;
    simple_connector_data->conn = NULL;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        security_start_ssl(NETDATA_SSL_CONTEXT_EXPORTING);
    }
#endif

    struct prometheus_remote_write_specific_data *connector_specific_data =
        callocz(1, sizeof(struct prometheus_remote_write_specific_data));
    simple_connector_data->connector_specific_data = (void *)connector_specific_data;

    simple_connector_init(instance);

    connector_specific_data->write_request = init_write_request();

    instance->engine->protocol_buffers_initialized = 1;

    return 0;
}

struct format_remote_write_label_callback {
    struct instance *instance;
    void *write_request;
};

static int format_remote_write_label_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct format_remote_write_label_callback *d = (struct format_remote_write_label_callback *)data;

    if (!should_send_label(d->instance, ls)) return 0;

    char k[PROMETHEUS_ELEMENT_MAX + 1];
    char v[PROMETHEUS_ELEMENT_MAX + 1];

    prometheus_name_copy(k, name, PROMETHEUS_ELEMENT_MAX);
    prometheus_label_copy(v, value, PROMETHEUS_ELEMENT_MAX);
    add_label(d->write_request, k, v);
    return 1;
}

/**
 * Format host data for Prometheus Remote Write connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */
int format_host_prometheus_remote_write(struct instance *instance, RRDHOST *host)
{
    struct simple_connector_data *simple_connector_data =
        (struct simple_connector_data *)instance->connector_specific_data;
    struct prometheus_remote_write_specific_data *connector_specific_data =
        (struct prometheus_remote_write_specific_data *)simple_connector_data->connector_specific_data;

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(
        hostname,
        (host == localhost) ? instance->config.hostname : host->hostname,
        PROMETHEUS_ELEMENT_MAX);

    add_host_info(
        connector_specific_data->write_request,
        "netdata_info", hostname, host->program_name, host->program_version, now_realtime_usec() / USEC_PER_MS);
    
    if (unlikely(sending_labels_configured(instance))) {
        struct format_remote_write_label_callback tmp = {
            .write_request = connector_specific_data->write_request,
            .instance = instance
        };
        rrdlabels_walkthrough_read(host->host_labels, format_remote_write_label_callback, &tmp);
    }

    return 0;
}

/**
 * Format chart data for Prometheus Remote Write connector
 *
 * @param instance an instance data structure.
 * @param st a chart.
 * @return Always returns 0.
 */
int format_chart_prometheus_remote_write(struct instance *instance, RRDSET *st)
{
    prometheus_label_copy(
        chart,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? st->name : st->id,
        PROMETHEUS_ELEMENT_MAX);
    prometheus_label_copy(family, rrdset_family(st), PROMETHEUS_ELEMENT_MAX);
    prometheus_name_copy(context, st->context, PROMETHEUS_ELEMENT_MAX);

    as_collected = (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED);
    homogeneous = 1;
    if (as_collected) {
        if (rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK))
            rrdset_update_heterogeneous_flag(st);

        if (rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
            homogeneous = 0;
    } else {
        if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AVERAGE)
            prometheus_units_copy(units, rrdset_units(st), PROMETHEUS_ELEMENT_MAX, 0);
    }

    return 0;
}

/**
 * Format dimension data for Prometheus Remote Write connector
 *
 * @param instance an instance data structure.
 * @param rd a dimension.
 * @return Always returns 0.
 */
int format_dimension_prometheus_remote_write(struct instance *instance, RRDDIM *rd)
{
    struct simple_connector_data *simple_connector_data =
        (struct simple_connector_data *)instance->connector_specific_data;
    struct prometheus_remote_write_specific_data *connector_specific_data =
        (struct prometheus_remote_write_specific_data *)simple_connector_data->connector_specific_data;

    if (rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
        char name[PROMETHEUS_LABELS_MAX + 1];
        char dimension[PROMETHEUS_ELEMENT_MAX + 1];
        char *suffix = "";
        RRDHOST *host = rd->rrdset->rrdhost;

        if (as_collected) {
            // we need as-collected / raw data

            if (unlikely(rd->last_collected_time.tv_sec < instance->after)) {
                debug(
                    D_EXPORTING,
                    "EXPORTING: not sending dimension '%s' of chart '%s' from host '%s', "
                    "its last data collection (%lu) is not within our timeframe (%lu to %lu)",
                    rrddim_id(rd), rd->rrdset->id,
                    (host == localhost) ? instance->config.hostname : host->hostname,
                    (unsigned long)rd->last_collected_time.tv_sec,
                    (unsigned long)instance->after,
                    (unsigned long)instance->before);
                return 0;
            }

            if (homogeneous) {
                // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                // we add all dimensions as labels

                prometheus_label_copy(
                    dimension,
                    (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                    PROMETHEUS_ELEMENT_MAX);
                snprintf(name, PROMETHEUS_LABELS_MAX, "%s_%s%s", instance->config.prefix, context, suffix);

                add_metric(
                    connector_specific_data->write_request,
                    name, chart, family, dimension,
                    (host == localhost) ? instance->config.hostname : host->hostname,
                    rd->last_collected_value, timeval_msec(&rd->last_collected_time));
            } else {
                // the dimensions of the chart, do not have the same algorithm, multiplier or divisor
                // we create a metric per dimension

                prometheus_name_copy(
                    dimension,
                    (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                    PROMETHEUS_ELEMENT_MAX);
                snprintf(
                    name, PROMETHEUS_LABELS_MAX, "%s_%s_%s%s", instance->config.prefix, context, dimension,
                    suffix);

                add_metric(
                    connector_specific_data->write_request,
                    name, chart, family, NULL,
                    (host == localhost) ? instance->config.hostname : host->hostname,
                    rd->last_collected_value, timeval_msec(&rd->last_collected_time));
            }
        } else {
            // we need average or sum of the data

            time_t last_t = instance->before;
            NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);

            if (!isnan(value) && !isinf(value)) {
                if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AVERAGE)
                    suffix = "_average";
                else if (EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_SUM)
                    suffix = "_sum";

                prometheus_label_copy(
                    dimension,
                    (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                    PROMETHEUS_ELEMENT_MAX);
                snprintf(
                    name, PROMETHEUS_LABELS_MAX, "%s_%s%s%s", instance->config.prefix, context, units, suffix);

                add_metric(
                    connector_specific_data->write_request,
                    name, chart, family, dimension,
                    (host == localhost) ? instance->config.hostname : host->hostname,
                    value, last_t * MSEC_PER_SEC);
            }
        }
    }

    return 0;
}

int format_variable_prometheus_remote_write_callback(RRDVAR *rv, void *data) {
    struct prometheus_remote_write_variables_callback_options *opts = data;

    if (rv->options & (RRDVAR_OPTION_CUSTOM_HOST_VAR | RRDVAR_OPTION_CUSTOM_CHART_VAR)) {        
        RRDHOST *host = opts->host;
        struct instance *instance = opts->instance;
        struct simple_connector_data *simple_connector_data =
            (struct simple_connector_data *)instance->connector_specific_data;
        struct prometheus_remote_write_specific_data *connector_specific_data =
            (struct prometheus_remote_write_specific_data *)simple_connector_data->connector_specific_data;

        char name[PROMETHEUS_LABELS_MAX + 1];
        char *suffix = "";

        prometheus_name_copy(context, rv->name, PROMETHEUS_ELEMENT_MAX);
        snprintf(name, PROMETHEUS_LABELS_MAX, "%s_%s%s", instance->config.prefix, context, suffix);

        NETDATA_DOUBLE value = rrdvar2number(rv);
        add_variable(connector_specific_data->write_request, name,
            (host == localhost) ? instance->config.hostname : host->hostname, value, opts->now / USEC_PER_MS);
    }

    return 0;
}

/**
 * Format a variable for Prometheus Remote Write connector
 * 
 * @param rv a variable.
 * @param instance an instance data structure.
 * @return Always returns 0.
 */ 
int format_variables_prometheus_remote_write(struct instance *instance, RRDHOST *host)
{
    struct prometheus_remote_write_variables_callback_options opt = {
        .host = host,
        .instance = instance,
        .now = now_realtime_usec(),
    };

    return foreach_host_variable_callback(host, format_variable_prometheus_remote_write_callback, &opt);
}

/**
 * Format a batch for Prometheus Remote Write connector
 *
 * @param instance an instance data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int format_batch_prometheus_remote_write(struct instance *instance)
{
    struct simple_connector_data *simple_connector_data =
        (struct simple_connector_data *)instance->connector_specific_data;
    struct prometheus_remote_write_specific_data *connector_specific_data =
        (struct prometheus_remote_write_specific_data *)simple_connector_data->connector_specific_data;

    size_t data_size = get_write_request_size(connector_specific_data->write_request);

    if (unlikely(!data_size)) {
        error("EXPORTING: write request size is out of range");
        return 1;
    }

    BUFFER *buffer = instance->buffer;

    buffer_need_bytes(buffer, data_size);
    if (unlikely(pack_and_clear_write_request(connector_specific_data->write_request, buffer->buffer, &data_size))) {
        error("EXPORTING: cannot pack write request");
        return 1;
    }
    buffer->len = data_size;

    simple_connector_end_batch(instance);

    return 0;
}
