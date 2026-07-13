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

    connector_specific_data->ssl = NETDATA_SSL_UNSET_CONNECTION;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        netdata_ssl_initialize_ctx(NETDATA_SSL_EXPORTING_CTX);
    }

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

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    if (!instance->buffer) {
        netdata_log_error("EXPORTING: cannot create buffer for opentsdb telnet exporting connector instance %s", instance->config.name);
        return 1;
    }

    simple_connector_init(instance);

    if (netdata_mutex_init(&instance->mutex))
        return 1;
    if (netdata_cond_init(&instance->cond_var))
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
    connector_specific_data->ssl = NETDATA_SSL_UNSET_CONNECTION;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        netdata_ssl_initialize_ctx(NETDATA_SSL_EXPORTING_CTX);
    }
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

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    if (!instance->buffer) {
        netdata_log_error("EXPORTING: cannot create buffer for opentsdb HTTP exporting connector instance %s", instance->config.name);
        return 1;
    }

    simple_connector_init(instance);

    if (netdata_mutex_init(&instance->mutex))
        return 1;
    if (netdata_cond_init(&instance->cond_var))
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
        if (isalpha((uint8_t)*src) || isdigit((uint8_t)*src) || *src == '-' || *src == '.' || *src == '/' || IS_UTF8_BYTE(*src))
            *dst++ = *src;
        else
            *dst++ = '_';

        src++;
        len--;
    }
    *dst = '\0';
}

static void buffer_strcat_opentsdb_telnet_value(BUFFER *wb, const char *src) {
    size_t len = strlen(src);

    buffer_need_bytes(wb, len + 1);
    sanitize_opentsdb_label_value(&wb->buffer[wb->len], src, len);
    wb->len += len;
    buffer_overflow_check(wb);
}

static void format_opentsdb_telnet_prefix(
    BUFFER *wb,
    const char *prefix,
    const char *chart_name,
    const char *dimension_name,
    unsigned long long timestamp) {
    buffer_strcat(wb, "put ");
    buffer_strcat_opentsdb_telnet_value(wb, prefix);
    buffer_sprintf(wb, ".%s.%s %llu ", chart_name, dimension_name, timestamp);
}

static void format_opentsdb_telnet_host(BUFFER *wb, const char *hostname) {
    buffer_strcat(wb, " host=");
    buffer_strcat_opentsdb_telnet_value(wb, hostname);
}

static void format_opentsdb_telnet_suffix(BUFFER *wb, const char *host_and_labels) {
    buffer_strcat(wb, host_and_labels);
    buffer_putc(wb, '\n');
}

/**
 * Format host identity and labels for OpenTSDB Telnet connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */

int format_host_labels_opentsdb_telnet(struct instance *instance, RRDHOST *host) {
    if(!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_exporters);

    if(host == localhost)
        format_opentsdb_telnet_host(instance->labels_buffer, instance->config.hostname);
    else {
        RRDHOST_IDENTITY identity = rrdhost_identity_acquire(host);
        format_opentsdb_telnet_host(instance->labels_buffer, string2str(identity.hostname));
        rrdhost_identity_release(&identity);
    }

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    buffer_strcat(instance->labels_buffer, " ");
    rrdlabels_to_buffer(host->rrdlabels, instance->labels_buffer, "", "=", "", " ",
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

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : rrdset_id(st),
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    format_opentsdb_telnet_prefix(
        instance->buffer,
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)rd->collector.last_collected_time.tv_sec);

    if(rrddim_is_float(rd))
        buffer_sprintf(instance->buffer, NETDATA_DOUBLE_FORMAT, rrddim_last_collected_as_double(rd));
    else
        buffer_sprintf(instance->buffer, COLLECTED_NUMBER_FORMAT, (collected_number)rrddim_last_collected_raw_int(rd));

    format_opentsdb_telnet_suffix(instance->buffer, buffer_tostring(instance->labels_buffer));

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

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : rrdset_id(st),
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

    format_opentsdb_telnet_prefix(
        instance->buffer,
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)last_t);

    buffer_sprintf(instance->buffer, NETDATA_DOUBLE_FORMAT, value);

    format_opentsdb_telnet_suffix(instance->buffer, buffer_tostring(instance->labels_buffer));

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
        (unsigned long int) buffer_strlen(simple_connector_data->last_buffer->buffer));

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
        instance->labels_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_exporters);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    rrdlabels_to_buffer(host->rrdlabels, instance->labels_buffer, ",", ":", "\"", "",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_opentsdb_label_value);
    return 0;
}

static void format_opentsdb_http_prefix(
    BUFFER *wb,
    const char *prefix,
    const char *chart_name,
    const char *dimension_name,
    unsigned long long timestamp) {
    buffer_strcat(wb, "{\"metric\":\"");
    buffer_json_strcat(wb, prefix);
    buffer_putc(wb, '.');
    buffer_json_strcat(wb, chart_name);
    buffer_putc(wb, '.');
    buffer_json_strcat(wb, dimension_name);
    buffer_sprintf(wb, "\",\"timestamp\":%llu,\"value\":", timestamp);
}

static void format_opentsdb_http_suffix(BUFFER *wb, const char *hostname, const char *labels) {
    buffer_strcat(wb, ",\"tags\":{\"host\":\"");
    buffer_json_strcat(wb, hostname);
    buffer_strcat(wb, "\"");
    buffer_strcat(wb, labels);
    buffer_strcat(wb, "}}");
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
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : rrdset_id(st),
        RRD_ID_LENGTH_MAX);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        dimension_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);

    if (buffer_strlen((BUFFER *)instance->buffer) > 2)
        buffer_strcat(instance->buffer, ",\n");

    format_opentsdb_http_prefix(
        instance->buffer,
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)rd->collector.last_collected_time.tv_sec);

    if(rrddim_is_float(rd))
        buffer_sprintf(instance->buffer, NETDATA_DOUBLE_FORMAT, rrddim_last_collected_as_double(rd));
    else
        buffer_sprintf(instance->buffer, COLLECTED_NUMBER_FORMAT, (collected_number)rrddim_last_collected_raw_int(rd));

    format_opentsdb_http_suffix(
        instance->buffer,
        (host == localhost) ? instance->config.hostname : rrdhost_hostname(host),
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
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : rrdset_id(st),
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

    format_opentsdb_http_prefix(
        instance->buffer,
        instance->config.prefix,
        chart_name,
        dimension_name,
        (unsigned long long)last_t);

    buffer_sprintf(instance->buffer, NETDATA_DOUBLE_FORMAT, value);

    format_opentsdb_http_suffix(
        instance->buffer,
        (host == localhost) ? instance->config.hostname : rrdhost_hostname(host),
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "");

    return 0;
}

static int opentsdb_http_unittest_case(
    const char *description,
    const char *prefix,
    const char *chart_name,
    const char *dimension_name,
    const char *hostname,
    const char *expected) {
    BUFFER *wb = buffer_create(0, NULL);
    format_opentsdb_http_prefix(wb, prefix, chart_name, dimension_name, 42);
    buffer_strcat(wb, "1.5");
    format_opentsdb_http_suffix(wb, hostname, ",\"label\":\"value\"");

    int errors = 0;
    if(strcmp(buffer_tostring(wb), expected) != 0) {
        fprintf(
            stderr,
            "OpenTSDB HTTP JSON %s output mismatch\nexpected: %s\nactual:   %s\n",
            description,
            expected,
            buffer_tostring(wb));
        errors++;
    }

    char metric[1024];
    snprintfz(metric, sizeof(metric), "%s.%s.%s", prefix, chart_name, dimension_name);

    json_object *root = json_tokener_parse(buffer_tostring(wb));
    json_object *member = NULL, *tags = NULL;
    if(!root || !json_object_is_type(root, json_type_object) || json_object_object_length(root) != 4 ||
       !json_object_object_get_ex(root, "metric", &member) || strcmp(json_object_get_string(member), metric) != 0 ||
       !json_object_object_get_ex(root, "timestamp", &member) || json_object_get_int64(member) != 42 ||
       !json_object_object_get_ex(root, "value", &member) || json_object_get_double(member) != 1.5 ||
       !json_object_object_get_ex(root, "tags", &tags) || !json_object_is_type(tags, json_type_object) ||
       json_object_object_length(tags) != 2 || !json_object_object_get_ex(tags, "host", &member) ||
       strcmp(json_object_get_string(member), hostname) != 0 ||
       !json_object_object_get_ex(tags, "label", &member) || strcmp(json_object_get_string(member), "value") != 0) {
        fprintf(stderr, "OpenTSDB HTTP JSON %s schema or decoded value mismatch\n", description);
        errors++;
    }

    if(root)
        json_object_put(root);
    buffer_free(wb);
    return errors;
}

int exporting_opentsdb_http_unittest(void) {
    int errors = 0;

    errors += opentsdb_http_unittest_case(
        "ordinary metadata",
        "netdata",
        "chart.name",
        "dimension.name",
        "localhost",
        "{\"metric\":\"netdata.chart.name.dimension.name\",\"timestamp\":42,\"value\":1.5,"
        "\"tags\":{\"host\":\"localhost\",\"label\":\"value\"}}");

    errors += opentsdb_http_unittest_case(
        "hostile metadata",
        "pre\"\\\x01",
        "chart\nname",
        "dimension\tname",
        "host\r\"\\name",
        "{\"metric\":\"pre\\\"\\\\\\u0001.chart\\nname.dimension\\tname\",\"timestamp\":42,"
        "\"value\":1.5,\"tags\":{\"host\":\"host\\r\\\"\\\\name\",\"label\":\"value\"}}");

    return errors;
}

static int opentsdb_telnet_unittest_case(
    const char *description,
    const char *prefix,
    const char *hostname,
    const char *labels,
    const char *expected) {
    BUFFER *host_and_labels = buffer_create(0, NULL);
    format_opentsdb_telnet_host(host_and_labels, hostname);
    buffer_strcat(host_and_labels, labels);

    BUFFER *wb = buffer_create(0, NULL);
    format_opentsdb_telnet_prefix(wb, prefix, "chart.name", "dimension.name", 42);
    buffer_strcat(wb, "1.5");
    format_opentsdb_telnet_suffix(wb, buffer_tostring(host_and_labels));

    int errors = 0;
    if(strcmp(buffer_tostring(wb), expected) != 0) {
        fprintf(
            stderr,
            "OpenTSDB Telnet %s output mismatch\nexpected: %s\nactual:   %s\n",
            description,
            expected,
            buffer_tostring(wb));
        errors++;
    }

    size_t newline_count = 0;
    for(const char *s = buffer_tostring(wb); *s; s++) {
        if(*s == '\n')
            newline_count++;
        else if(*s == '\r' || *s == '\t') {
            fprintf(stderr, "OpenTSDB Telnet %s output retained a protocol delimiter\n", description);
            errors++;
            break;
        }
    }

    if(newline_count != 1 || !buffer_strlen(wb) || wb->buffer[wb->len - 1] != '\n') {
        fprintf(stderr, "OpenTSDB Telnet %s output is not exactly one newline-terminated command\n", description);
        errors++;
    }

    buffer_free(wb);
    buffer_free(host_and_labels);
    return errors;
}

int exporting_opentsdb_telnet_unittest(void) {
    int errors = 0;

    errors += opentsdb_telnet_unittest_case(
        "ordinary metadata",
        "netdata",
        "localhost",
        " label=value",
        "put netdata.chart.name.dimension.name 42 1.5 host=localhost label=value\n");

    errors += opentsdb_telnet_unittest_case(
        "allowed punctuation",
        "Netdata-A_9/part.",
        "Host-A_9/path.example",
        " label=value",
        "put Netdata-A_9/part..chart.name.dimension.name 42 1.5 host=Host-A_9/path.example label=value\n");

    errors += opentsdb_telnet_unittest_case(
        "hostile metadata",
        "pre fix\tbad\r\nput",
        "host name\tbad\r\nput",
        " label=value",
        "put pre_fix_bad__put.chart.name.dimension.name 42 1.5 host=host_name_bad__put label=value\n");

    errors += opentsdb_telnet_unittest_case(
        "all-invalid metadata",
        " \t\r\n",
        " \t\r\n",
        " label=value",
        "put ____.chart.name.dimension.name 42 1.5 host=____ label=value\n");

    errors += opentsdb_telnet_unittest_case(
        "empty metadata",
        "",
        "",
        "",
        "put .chart.name.dimension.name 42 1.5 host=\n");

    errors += opentsdb_telnet_unittest_case(
        "colliding invalid metadata",
        "pre fix",
        "host name",
        "",
        "put pre_fix.chart.name.dimension.name 42 1.5 host=host_name\n");

    errors += opentsdb_telnet_unittest_case(
        "colliding valid metadata",
        "pre_fix",
        "host_name",
        "",
        "put pre_fix.chart.name.dimension.name 42 1.5 host=host_name\n");

    char long_prefix[4097];
    char long_hostname[4097];
    memset(long_prefix, 'p', sizeof(long_prefix) - 1);
    memset(long_hostname, 'h', sizeof(long_hostname) - 1);
    long_prefix[1024] = '\n';
    long_hostname[3072] = '\t';
    long_prefix[sizeof(long_prefix) - 1] = '\0';
    long_hostname[sizeof(long_hostname) - 1] = '\0';

    BUFFER *host_and_labels = buffer_create(0, NULL);
    format_opentsdb_telnet_host(host_and_labels, long_hostname);

    BUFFER *wb = buffer_create(0, NULL);
    format_opentsdb_telnet_prefix(wb, long_prefix, "chart.name", "dimension.name", 42);
    buffer_strcat(wb, "1.5");
    format_opentsdb_telnet_suffix(wb, buffer_tostring(host_and_labels));

    const size_t expected_length = strlen("put ") + strlen(long_prefix) + strlen(".chart.name.dimension.name 42 1.5 host=") +
                                   strlen(long_hostname) + 1;
    if(buffer_strlen(wb) != expected_length || wb->buffer[sizeof("put ") - 1 + 1024] != '_' ||
       strchr(buffer_tostring(wb), '\t') || strchr(buffer_tostring(wb), '\r') ||
       strchr(buffer_tostring(wb), '\n') != &wb->buffer[wb->len - 1]) {
        fprintf(stderr, "OpenTSDB Telnet long metadata was truncated or retained a protocol delimiter\n");
        errors++;
    }
    buffer_free(wb);
    buffer_free(host_and_labels);

    return errors;
}
