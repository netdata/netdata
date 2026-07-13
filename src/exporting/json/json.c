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

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    if (!instance->buffer) {
        netdata_log_error("EXPORTING: cannot create buffer for json exporting connector instance %s", instance->config.name);
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

    connector_specific_data->ssl = NETDATA_SSL_UNSET_CONNECTION;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        netdata_ssl_initialize_ctx(NETDATA_SSL_EXPORTING_CTX);
    }

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

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

    simple_connector_init(instance);

    if (netdata_mutex_init(&instance->mutex))
        return 1;
    if (netdata_cond_init(&instance->cond_var))
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
        instance->labels_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_exporters);

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    buffer_strcat(instance->labels_buffer, "\"labels\":{");
    rrdlabels_to_buffer(host->rrdlabels, instance->labels_buffer, "", ":", "\"", ",",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_json_string);
    buffer_strcat(instance->labels_buffer, "},");

    return 0;
}

static void format_dimension_json_plaintext_prefix(
    BUFFER *wb,
    const char *prefix,
    const char *hostname,
    const char *labels,
    const char *chart_id,
    const char *chart_name,
    const char *chart_family,
    const char *chart_context,
    const char *chart_type,
    const char *units,
    const char *dimension_id,
    const char *dimension_name,
    bool stored) {
    buffer_strcat(wb, "{\"prefix\":\"");
    buffer_json_strcat(wb, prefix);
    buffer_strcat(wb, "\",\"hostname\":\"");
    buffer_json_strcat(wb, hostname);
    buffer_strcat(wb, "\",");
    buffer_strcat(wb, labels);

    buffer_strcat(wb, "\"chart_id\":\"");
    buffer_json_strcat(wb, chart_id);
    buffer_strcat(wb, "\",\"chart_name\":\"");
    buffer_json_strcat(wb, chart_name);
    buffer_strcat(wb, "\",\"chart_family\":\"");
    buffer_json_strcat(wb, chart_family);
    buffer_strcat(wb, stored ? "\",\"chart_context\": \"" : "\",\"chart_context\":\"");
    buffer_json_strcat(wb, chart_context);
    buffer_strcat(wb, "\",\"chart_type\":\"");
    buffer_json_strcat(wb, chart_type);
    buffer_strcat(wb, stored ? "\",\"units\": \"" : "\",\"units\":\"");
    buffer_json_strcat(wb, units);

    buffer_strcat(wb, "\",\"id\":\"");
    buffer_json_strcat(wb, dimension_id);
    buffer_strcat(wb, "\",\"name\":\"");
    buffer_json_strcat(wb, dimension_name);
    buffer_strcat(wb, "\",\"value\":");
}

static void format_dimension_json_plaintext_value(BUFFER *wb, NETDATA_DOUBLE value) {
    buffer_print_netdata_double_fixed(wb, value);
}

static bool format_dimension_stored_json_plaintext_value_is_exportable(NETDATA_DOUBLE value) {
    return !isnan(value);
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

    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_JSON_HTTP) {
        if (buffer_strlen((BUFFER *)instance->buffer) > 2)
        buffer_strcat(instance->buffer, ",\n");
    }

    format_dimension_json_plaintext_prefix(
        instance->buffer,
        instance->config.prefix,
        (host == localhost) ? instance->config.hostname : rrdhost_hostname(host),
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "",
        rrdset_id(st),
        rrdset_name(st),
        rrdset_family(st),
        rrdset_context(st),
        rrdset_parts_type(st),
        rrdset_units(st),
        rrddim_id(rd),
        rrddim_name(rd),
        false);

    if(rrddim_is_float(rd))
        format_dimension_json_plaintext_value(instance->buffer, rrddim_last_collected_as_double(rd));
    else
        buffer_sprintf(instance->buffer, COLLECTED_NUMBER_FORMAT, (collected_number)rrddim_last_collected_raw_int(rd));

    buffer_sprintf(instance->buffer, ",\"timestamp\":%llu}",
        (unsigned long long)rd->collector.last_collected_time.tv_sec);

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

    if(!format_dimension_stored_json_plaintext_value_is_exportable(value))
        return 0;

    if (instance->config.type == EXPORTING_CONNECTOR_TYPE_JSON_HTTP) {
        if (buffer_strlen((BUFFER *)instance->buffer) > 2)
            buffer_strcat(instance->buffer, ",\n");
    }

    format_dimension_json_plaintext_prefix(
        instance->buffer,
        instance->config.prefix,
        (host == localhost) ? instance->config.hostname : rrdhost_hostname(host),
        instance->labels_buffer ? buffer_tostring(instance->labels_buffer) : "",
        rrdset_id(st),
        rrdset_name(st),
        rrdset_family(st),
        rrdset_context(st),
        rrdset_parts_type(st),
        rrdset_units(st),
        rrddim_id(rd),
        rrddim_name(rd),
        true);

    format_dimension_json_plaintext_value(instance->buffer, value);
    buffer_sprintf(instance->buffer, ",\"timestamp\": %llu}", (unsigned long long)last_t);

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
        (unsigned long int) buffer_strlen(simple_connector_data->last_buffer->buffer));

    return;
}

static int json_plaintext_unittest_case(
    const char *description,
    bool stored,
    const char *prefix,
    const char *hostname,
    const char *chart_id,
    const char *chart_name,
    const char *chart_family,
    const char *chart_context,
    const char *chart_type,
    const char *units,
    const char *dimension_id,
    const char *dimension_name,
    const char *expected) {
    BUFFER *wb = buffer_create(0, NULL);
    format_dimension_json_plaintext_prefix(
        wb,
        prefix,
        hostname,
        "\"labels\":{\"label\":\"value\"},",
        chart_id,
        chart_name,
        chart_family,
        chart_context,
        chart_type,
        units,
        dimension_id,
        dimension_name,
        stored);
    format_dimension_json_plaintext_value(wb, 1.5);
    buffer_strcat(wb, stored ? ",\"timestamp\": 42}" : ",\"timestamp\":42}");

    int errors = 0;
    if(strcmp(buffer_tostring(wb), expected) != 0) {
        fprintf(
            stderr,
            "exporting JSON %s output mismatch\nexpected: %s\nactual:   %s\n",
            description,
            expected,
            buffer_tostring(wb));
        errors++;
    }

    json_object *root = json_tokener_parse(buffer_tostring(wb));
    json_object *member = NULL, *labels = NULL;
#define CHECK_JSON_STRING(key, value)                                                                                   \
    (!json_object_object_get_ex(root, key, &member) || strcmp(json_object_get_string(member), value) != 0)
    if(!root || !json_object_is_type(root, json_type_object) || json_object_object_length(root) != 13 ||
       CHECK_JSON_STRING("prefix", prefix) || CHECK_JSON_STRING("hostname", hostname) ||
       CHECK_JSON_STRING("chart_id", chart_id) || CHECK_JSON_STRING("chart_name", chart_name) ||
       CHECK_JSON_STRING("chart_family", chart_family) || CHECK_JSON_STRING("chart_context", chart_context) ||
       CHECK_JSON_STRING("chart_type", chart_type) || CHECK_JSON_STRING("units", units) ||
       CHECK_JSON_STRING("id", dimension_id) || CHECK_JSON_STRING("name", dimension_name) ||
       !json_object_object_get_ex(root, "labels", &labels) || !json_object_is_type(labels, json_type_object) ||
       !json_object_object_get_ex(labels, "label", &member) || strcmp(json_object_get_string(member), "value") != 0 ||
       !json_object_object_get_ex(root, "value", &member) || json_object_get_double(member) != 1.5 ||
       !json_object_object_get_ex(root, "timestamp", &member) || json_object_get_int64(member) != 42) {
        fprintf(stderr, "exporting JSON %s schema or decoded value mismatch\n", description);
        errors++;
    }
#undef CHECK_JSON_STRING

    if(root)
        json_object_put(root);
    buffer_free(wb);
    return errors;
}

static int json_plaintext_number_unittest_case(
    const char *description, NETDATA_DOUBLE value, bool stored, bool expect_record, bool expect_null) {
    int errors = 0;
    BUFFER *actual_value = buffer_create(0, NULL);
    BUFFER *expected_value = buffer_create(0, NULL);
    BUFFER *record = buffer_create(0, NULL);

    if(expect_record) {
        format_dimension_json_plaintext_value(actual_value, value);
        if(expect_null)
            buffer_strcat(expected_value, "null");
        else
            buffer_sprintf(expected_value, NETDATA_DOUBLE_FORMAT, value);

        if(strcmp(buffer_tostring(actual_value), buffer_tostring(expected_value)) != 0) {
            fprintf(
                stderr,
                "exporting JSON %s %s numeric output mismatch\nexpected: %s\nactual:   %s\n",
                stored ? "stored" : "collected",
                description,
                buffer_tostring(expected_value),
                buffer_tostring(actual_value));
            errors++;
        }

        format_dimension_json_plaintext_prefix(
            record,
            "netdata",
            "localhost",
            "\"labels\":{},",
            "chart.id",
            "chart.name",
            "family",
            "context",
            "line",
            "units",
            "dimension.id",
            "dimension.name",
            stored);
        format_dimension_json_plaintext_value(record, value);
        buffer_strcat(record, stored ? ",\"timestamp\": 42}" : ",\"timestamp\":42}");

        json_object *root = json_tokener_parse(buffer_tostring(record));
        json_object *member = NULL;
        if(!root || !json_object_is_type(root, json_type_object) ||
           !json_object_object_get_ex(root, "value", &member)) {
            fprintf(stderr, "exporting JSON %s %s record is not valid JSON\n",
                    stored ? "stored" : "collected", description);
            errors++;
        }

        if(root)
            json_object_put(root);
    }
    else if(format_dimension_stored_json_plaintext_value_is_exportable(value)) {
        fprintf(stderr, "exporting JSON stored %s should remain an omitted gap\n", description);
        errors++;
    }

    buffer_free(record);
    buffer_free(expected_value);
    buffer_free(actual_value);
    return errors;
}

static int json_plaintext_transport_unittest(void) {
    int errors = 0;

    BUFFER *plaintext = buffer_create(0, NULL);
    buffer_strcat(plaintext, "{\"value\":");
    format_dimension_json_plaintext_value(plaintext, INFINITY);
    buffer_strcat(plaintext, ",\"timestamp\":42}\n");
    if(strcmp(buffer_tostring(plaintext), "{\"value\":null,\"timestamp\":42}\n") != 0) {
        fprintf(stderr, "exporting JSON plaintext transport framing mismatch\n");
        errors++;
    }
    buffer_free(plaintext);

    struct instance instance = { 0 };
    instance.config.name = "JSON unit test";
    instance.connector_specific_data = callocz(1, sizeof(struct simple_connector_data));
    instance.buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

    struct simple_connector_data *connector = instance.connector_specific_data;
    struct simple_connector_buffer *queued = callocz(1, sizeof(*queued));
    queued->header = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    queued->buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    queued->next = queued;
    connector->first_buffer = connector->last_buffer = queued;

    open_batch_json_http(&instance);
    buffer_strcat(instance.buffer, "{\"value\":");
    format_dimension_json_plaintext_value(instance.buffer, 1.5);
    buffer_strcat(instance.buffer, "},\n{\"value\":");
    format_dimension_json_plaintext_value(instance.buffer, -INFINITY);
    buffer_strcat(instance.buffer, "}");
    instance.stats.buffered_metrics = 2;
    close_batch_json_http(&instance);

    const char *expected = "[\n{\"value\":1.5000000},\n{\"value\":null}\n]\n";
    const size_t expected_size = strlen(expected);
    if(strcmp(buffer_tostring(queued->buffer), expected) != 0 || queued->buffered_metrics != 2 ||
       queued->buffered_bytes != expected_size || queued->used != 1 ||
       connector->total_buffered_metrics != 2 || instance.stats.buffered_metrics != 0 ||
       (size_t)instance.stats.buffered_bytes != expected_size || buffer_strlen((BUFFER *)instance.buffer) != 0) {
        fprintf(stderr, "exporting JSON HTTP batch, counter, or retry-buffer mismatch\n");
        errors++;
    }

    buffer_free(instance.buffer);
    buffer_free(queued->header);
    buffer_free(queued->buffer);
    freez(queued);
    freez(connector);
    return errors;
}

int exporting_json_connector_unittest(void) {
    int errors = 0;

    errors += json_plaintext_unittest_case(
        "ordinary collected metadata",
        false,
        "netdata",
        "localhost",
        "chart.id",
        "chart.name",
        "family",
        "context",
        "line",
        "units",
        "dimension.id",
        "dimension.name",
        "{\"prefix\":\"netdata\",\"hostname\":\"localhost\",\"labels\":{\"label\":\"value\"},"
        "\"chart_id\":\"chart.id\",\"chart_name\":\"chart.name\",\"chart_family\":\"family\","
        "\"chart_context\":\"context\",\"chart_type\":\"line\",\"units\":\"units\","
        "\"id\":\"dimension.id\",\"name\":\"dimension.name\",\"value\":1.5000000,\"timestamp\":42}");

    errors += json_plaintext_unittest_case(
        "hostile stored metadata",
        true,
        "pre\"\\\x01",
        "host\nname",
        "chart\tid",
        "chart\rname",
        "family\bname",
        "context\fname",
        "type\"name",
        "units\\name",
        "dimension\"id",
        "dimension\nname",
        "{\"prefix\":\"pre\\\"\\\\\\u0001\",\"hostname\":\"host\\nname\","
        "\"labels\":{\"label\":\"value\"},\"chart_id\":\"chart\\tid\","
        "\"chart_name\":\"chart\\rname\",\"chart_family\":\"family\\bname\","
        "\"chart_context\": \"context\\fname\",\"chart_type\":\"type\\\"name\","
        "\"units\": \"units\\\\name\",\"id\":\"dimension\\\"id\","
        "\"name\":\"dimension\\nname\",\"value\":1.5000000,\"timestamp\": 42}");

#ifdef NETDATA_WITH_LONG_DOUBLE
    const NETDATA_DOUBLE minimum_normal = LDBL_MIN;
    const NETDATA_DOUBLE minimum_subnormal = nextafterl(0.0L, 1.0L);
#else
    const NETDATA_DOUBLE minimum_normal = DBL_MIN;
    const NETDATA_DOUBLE minimum_subnormal = nextafter(0.0, 1.0);
#endif
    const struct {
        const char *description;
        NETDATA_DOUBLE value;
        bool expect_null;
    } number_cases[] = {
        { "positive zero", 0.0, false },
        { "negative zero", copysignndd(0.0, -1.0), false },
        { "maximum finite", NETDATA_DOUBLE_MAX, false },
        { "minimum finite", -NETDATA_DOUBLE_MAX, false },
        { "minimum positive normal", minimum_normal, false },
        { "minimum negative normal", -minimum_normal, false },
        { "minimum positive subnormal", minimum_subnormal, false },
        { "minimum negative subnormal", -minimum_subnormal, false },
        { "NaN", NAN, true },
        { "positive infinity", INFINITY, true },
        { "negative infinity", -INFINITY, true },
    };

    for(size_t i = 0; i < _countof(number_cases); i++) {
        const bool stored_record = !isnan(number_cases[i].value);
        errors += json_plaintext_number_unittest_case(
            number_cases[i].description, number_cases[i].value, false, true, number_cases[i].expect_null);
        errors += json_plaintext_number_unittest_case(
            number_cases[i].description,
            number_cases[i].value,
            true,
            stored_record,
            number_cases[i].expect_null);
    }

    errors += json_plaintext_transport_unittest();

    return errors;
}
