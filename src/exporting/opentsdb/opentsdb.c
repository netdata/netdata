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
    instance->start_chart_formatting = format_chart_opentsdb;

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
    instance->start_chart_formatting = format_chart_opentsdb;

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
 * @param len the maximum number of bytes copied.
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

/**
 * Copy a metric prefix or hostname, neutralizing only what must not reach an OpenTSDB telnet record
 *
 * A telnet record is `put <metric> <timestamp> <value> <tagk>=<tagv> ...\n`: whitespace-delimited
 * fields, newline-terminated, with no quoting or escaping. Only whitespace and the record terminator
 * can corrupt or inject a record, so those are replaced, along with the remaining control codepoints
 * -- C0, DEL, and C1 -- which cannot break our framing but are never legitimate in a hostname or
 * prefix: U+009B is the single-character CSI, the same escape hazard as U+001B.
 *
 * Everything else, punctuation included, is passed through byte for byte: none of `:`, `=` or `;`
 * affects this record's framing, and relabelling metadata the user configured would store the point
 * under a different host identity and split the host's history -- which is what netdata/netdata#23684
 * reported. A destination with a stricter tag grammar (stock OpenTSDB has one) may reject such a point
 * instead; that is the destination's rule and the operator's concern. This is therefore neither the Graphite
 * metric-path policy (which passes the non-whitespace controls through, and replaces `;` for Carbon's
 * tag syntax) nor the OpenTSDB label-value policy of sanitize_opentsdb_label_value().
 *
 * The replacement is byte-length preserving, which the callers below depend on for buffer sizing.
 *
 * @param dst a destination string.
 * @param src a source string.
 * @param len the maximum number of bytes copied.
 */
static void sanitize_opentsdb_telnet_metadata_value(char *dst, const char *src, size_t len) {
    while(*src && len) {
        uint32_t codepoint;
        size_t bytes = exporting_utf8_decode(src, len, &codepoint);

        if(codepoint < 0x20 || (codepoint >= 0x7F && codepoint <= 0x9F) ||
           exporting_unicode_is_whitespace(codepoint))
            memset(dst, '_', bytes);
        else
            memcpy(dst, src, bytes);

        dst += bytes;
        src += bytes;
        len -= bytes;
    }

    *dst = '\0';
}

static void buffer_strcat_opentsdb_telnet_value(BUFFER *wb, const char *src) {
    size_t len = strlen(src);

    buffer_need_bytes(wb, len + 1);
    sanitize_opentsdb_telnet_metadata_value(&wb->buffer[wb->len], src, len);
    wb->len += len;
    buffer_overflow_check(wb);
}

// The part of a record that depends only on the chart: "put <prefix>.<chart>.".
static void format_opentsdb_telnet_chart_prefix(BUFFER *wb, const char *prefix, const char *chart_name) {
    buffer_strcat(wb, "put ");
    buffer_strcat_opentsdb_telnet_value(wb, prefix);
    buffer_putc(wb, '.');
    buffer_strcat(wb, chart_name);
    buffer_putc(wb, '.');
}

// Everything of a record up to the value: the chart part, the dimension and the timestamp.
static void format_opentsdb_telnet_record_start(
    BUFFER *wb,
    BUFFER *chart_prefix,
    const char *dimension_name,
    size_t dimension_len,
    time_t timestamp) {
    buffer_memcat(wb, buffer_tostring(chart_prefix), buffer_strlen(chart_prefix));
    buffer_memcat(wb, dimension_name, dimension_len);
    buffer_putc(wb, ' ');
    buffer_print_uint64(wb, (uint64_t)timestamp);
    buffer_putc(wb, ' ');
}

static void format_opentsdb_telnet_host(BUFFER *wb, const char *hostname) {
    buffer_strcat(wb, " host=");
    buffer_strcat_opentsdb_telnet_value(wb, hostname);
}

static void format_opentsdb_telnet_suffix(BUFFER *wb, BUFFER *host_and_labels) {
    buffer_memcat(wb, buffer_tostring(host_and_labels), buffer_strlen(host_and_labels));
    buffer_putc(wb, '\n');
}

static bool opentsdb_value_is_exportable(NETDATA_DOUBLE value) {
    return netdata_double_isnumber(value);
}

static void opentsdb_value_print(BUFFER *wb, bool value_is_float, NETDATA_DOUBLE value, int64_t integer_value) {
    if(value_is_float)
        buffer_sprintf(wb, NETDATA_DOUBLE_FORMAT, value);
    else
        buffer_print_int64(wb, integer_value);
}

/**
 * Start a chart for OpenTSDB connectors
 *
 * The chart part of the records is the same for all dimensions of a chart. It is built once, by the first record
 * the chart writes, and reused for the rest: a chart whose values are all skipped reads none of its metadata.
 *
 * @param instance an instance data structure.
 * @param st a chart.
 * @return Always returns 0.
 */
int format_chart_opentsdb(struct instance *instance, RRDSET *st __maybe_unused)
{
    if (instance->chart_prefix_buffer)
        buffer_flush(instance->chart_prefix_buffer);

    return 0;
}

// Returns the chart part of the records, building it if this is the first record the chart writes.
static BUFFER *opentsdb_chart_prefix(
    struct instance *instance, RRDSET *st, void (*format)(BUFFER *wb, const char *prefix, const char *chart_name))
{
    if (!instance->chart_prefix_buffer)
        instance->chart_prefix_buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

    BUFFER *chart_prefix = instance->chart_prefix_buffer;
    if (!buffer_strlen(chart_prefix)) {
        char chart_name[RRD_ID_LENGTH_MAX + 1];
        exporting_name_copy(
            chart_name,
            (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : rrdset_id(st),
            RRD_ID_LENGTH_MAX);

        format(chart_prefix, instance->config.prefix, chart_name);
    }

    return chart_prefix;
}

static size_t opentsdb_dimension_name_copy(struct instance *instance, RRDDIM *rd, char *dst)
{
    return exporting_name_copy(
        dst,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);
}

static bool format_opentsdb_telnet_metric(
    struct instance *instance,
    RRDDIM *rd,
    time_t timestamp,
    bool value_is_float,
    NETDATA_DOUBLE value,
    int64_t integer_value) {
    if(value_is_float && !opentsdb_value_is_exportable(value))
        return false;

    BUFFER *chart_prefix = opentsdb_chart_prefix(instance, rd->rrdset, format_opentsdb_telnet_chart_prefix);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    size_t dimension_len = opentsdb_dimension_name_copy(instance, rd, dimension_name);

    BUFFER *wb = instance->buffer;
    format_opentsdb_telnet_record_start(wb, chart_prefix, dimension_name, dimension_len, timestamp);
    opentsdb_value_print(wb, value_is_float, value, integer_value);
    format_opentsdb_telnet_suffix(wb, instance->labels_buffer);
    return true;
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
    if(rrddim_is_float(rd))
        format_opentsdb_telnet_metric(
            instance, rd, rd->collector.last_collected_time.tv_sec, true, rrddim_last_collected_as_double(rd), 0);
    else
        format_opentsdb_telnet_metric(
            instance, rd, rd->collector.last_collected_time.tv_sec, false, 0.0, rrddim_last_collected_raw_int(rd));

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
    time_t last_t = 0;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);
    format_opentsdb_telnet_metric(instance, rd, last_t, true, value, 0);
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

// The tags of a record are the same for all records of a host: `,"tags":{"host":"<hostname>"<labels>}}`.
static void format_opentsdb_http_tags_open(BUFFER *wb, const char *hostname) {
    buffer_strcat(wb, ",\"tags\":{\"host\":\"");
    buffer_json_strcat(wb, hostname);
    buffer_strcat(wb, "\"");
}

static void format_opentsdb_http_tags_close(BUFFER *wb) {
    buffer_strcat(wb, "}}");
}

/**
 * Format the record tags (host and labels) for OpenTSDB HTTP connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */

int format_host_labels_opentsdb_http(struct instance *instance, RRDHOST *host) {
    if (!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_exporters);

    if (host == localhost)
        format_opentsdb_http_tags_open(instance->labels_buffer, instance->config.hostname);
    else {
        RRDHOST_IDENTITY identity = rrdhost_identity_acquire(host);
        format_opentsdb_http_tags_open(instance->labels_buffer, string2str(identity.hostname));
        rrdhost_identity_release(&identity);
    }

    if (sending_labels_configured(instance))
        rrdlabels_to_buffer(host->rrdlabels, instance->labels_buffer, ",", ":", "\"", "",
                            exporting_labels_filter_callback, instance,
                            NULL, sanitize_opentsdb_label_value);

    format_opentsdb_http_tags_close(instance->labels_buffer);
    return 0;
}

// The part of a record that depends only on the chart: `{"metric":"<prefix>.<chart>.`.
static void format_opentsdb_http_chart_prefix(BUFFER *wb, const char *prefix, const char *chart_name) {
    buffer_strcat(wb, "{\"metric\":\"");
    buffer_json_strcat(wb, prefix);
    buffer_putc(wb, '.');
    buffer_json_strcat(wb, chart_name);
    buffer_putc(wb, '.');
}

// Everything of a record up to the value: the chart part, the dimension and the timestamp.
static void format_opentsdb_http_record_start(
    BUFFER *wb,
    BUFFER *chart_prefix,
    const char *dimension_name,
    time_t timestamp) {
    buffer_memcat(wb, buffer_tostring(chart_prefix), buffer_strlen(chart_prefix));
    buffer_json_strcat(wb, dimension_name);
    buffer_strcat(wb, "\",\"timestamp\":");
    buffer_print_uint64(wb, (uint64_t)timestamp);
    buffer_strcat(wb, ",\"value\":");
}

static bool format_opentsdb_http_metric(
    struct instance *instance,
    RRDDIM *rd,
    time_t timestamp,
    bool value_is_float,
    NETDATA_DOUBLE value,
    int64_t integer_value) {
    if(value_is_float && !opentsdb_value_is_exportable(value))
        return false;

    BUFFER *wb = instance->buffer;
    if(buffer_strlen(wb) > 2)
        buffer_strcat(wb, ",\n");

    BUFFER *chart_prefix = opentsdb_chart_prefix(instance, rd->rrdset, format_opentsdb_http_chart_prefix);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    opentsdb_dimension_name_copy(instance, rd, dimension_name);

    format_opentsdb_http_record_start(wb, chart_prefix, dimension_name, timestamp);
    opentsdb_value_print(wb, value_is_float, value, integer_value);
    if(instance->labels_buffer)
        buffer_memcat(wb, buffer_tostring(instance->labels_buffer), buffer_strlen(instance->labels_buffer));
    return true;
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
    if(rrddim_is_float(rd))
        format_opentsdb_http_metric(
            instance, rd, rd->collector.last_collected_time.tv_sec, true, rrddim_last_collected_as_double(rd), 0);
    else
        format_opentsdb_http_metric(
            instance, rd, rd->collector.last_collected_time.tv_sec, false, 0.0, rrddim_last_collected_raw_int(rd));

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
    time_t last_t = 0;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);
    format_opentsdb_http_metric(instance, rd, last_t, true, value, 0);
    return 0;
}

// The record writers composed the way the metric callbacks compose them, for names given as strings.
static void opentsdb_http_unittest_prefix(
    BUFFER *wb, const char *prefix, const char *chart_name, const char *dimension_name, time_t timestamp) {
    BUFFER *chart_prefix = buffer_create(0, NULL);
    format_opentsdb_http_chart_prefix(chart_prefix, prefix, chart_name);
    format_opentsdb_http_record_start(wb, chart_prefix, dimension_name, timestamp);
    buffer_free(chart_prefix);
}

static void opentsdb_http_unittest_suffix(BUFFER *wb, const char *hostname, const char *labels) {
    format_opentsdb_http_tags_open(wb, hostname);
    buffer_strcat(wb, labels);
    format_opentsdb_http_tags_close(wb);
}

static void opentsdb_telnet_unittest_prefix(
    BUFFER *wb, const char *prefix, const char *chart_name, const char *dimension_name, time_t timestamp) {
    BUFFER *chart_prefix = buffer_create(0, NULL);
    format_opentsdb_telnet_chart_prefix(chart_prefix, prefix, chart_name);
    format_opentsdb_telnet_record_start(wb, chart_prefix, dimension_name, strlen(dimension_name), timestamp);
    buffer_free(chart_prefix);
}

static void opentsdb_telnet_unittest_suffix(BUFFER *wb, const char *host_and_labels) {
    buffer_strcat(wb, host_and_labels);
    buffer_putc(wb, '\n');
}

// A chart and a dimension for driving the record writers. A NULL name leaves it unset, which makes the connector
// fall back to the id.
static RRDSET *opentsdb_unittest_chart(const char *id, const char *name) {
    RRDSET *st = callocz(1, sizeof(*st));
    st->id = string_strdupz(id);
    st->name = name ? string_strdupz(name) : NULL;
    return st;
}

static void opentsdb_unittest_chart_free(RRDSET *st) {
    string_freez(st->id);
    string_freez(st->name);
    freez(st);
}

static RRDDIM *opentsdb_unittest_dimension(RRDSET *st, const char *id, const char *name) {
    RRDDIM *rd = callocz(1, sizeof(*rd));
    rd->rrdset = st;
    rd->id = string_strdupz(id);
    rd->name = name ? string_strdupz(name) : NULL;
    return rd;
}

static void opentsdb_unittest_dimension_free(RRDDIM *rd) {
    string_freez(rd->id);
    string_freez(rd->name);
    freez(rd);
}

static bool opentsdb_unittest_metric(
    struct instance *instance, bool http, const char *chart_id, const char *dimension_id, time_t timestamp,
    bool value_is_float, NETDATA_DOUBLE value, int64_t integer_value) {
    RRDSET *st = opentsdb_unittest_chart(chart_id, NULL);
    RRDDIM *rd = opentsdb_unittest_dimension(st, dimension_id, NULL);
    format_chart_opentsdb(instance, st);
    bool written = http ?
        format_opentsdb_http_metric(instance, rd, timestamp, value_is_float, value, integer_value) :
        format_opentsdb_telnet_metric(instance, rd, timestamp, value_is_float, value, integer_value);
    opentsdb_unittest_dimension_free(rd);
    opentsdb_unittest_chart_free(st);
    return written;
}

static int opentsdb_http_unittest_case(
    const char *description,
    const char *prefix,
    const char *chart_name,
    const char *dimension_name,
    const char *hostname,
    const char *expected) {
    BUFFER *wb = buffer_create(0, NULL);
    opentsdb_http_unittest_prefix(wb, prefix, chart_name, dimension_name, 42);
    buffer_strcat(wb, "1.5");
    opentsdb_http_unittest_suffix(wb, hostname, ",\"label\":\"value\"");

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

static int opentsdb_number_unittest(void) {
    int errors = 0;

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
        bool expect_record;
    } number_cases[] = {
        { "positive zero", 0.0, true },
        { "negative zero", copysignndd(0.0, -1.0), true },
        { "maximum finite", NETDATA_DOUBLE_MAX, true },
        { "minimum finite", -NETDATA_DOUBLE_MAX, true },
        { "minimum positive normal", minimum_normal, true },
        { "minimum negative normal", -minimum_normal, true },
        { "minimum positive subnormal", minimum_subnormal, true },
        { "minimum negative subnormal", -minimum_subnormal, true },
        { "NaN", NAN, false },
        { "positive infinity", INFINITY, false },
        { "negative infinity", -INFINITY, false },
    };

    for(size_t i = 0; i < _countof(number_cases); i++) {
        BUFFER *http = buffer_create(0, NULL);
        BUFFER *expected_http = buffer_create(0, NULL);
        buffer_strcat(http, "[\n");
        buffer_strcat(expected_http, "[\n");

        BUFFER *http_tags = buffer_create(0, NULL);
        opentsdb_http_unittest_suffix(http_tags, "localhost", "");
        struct instance http_instance = {
            .config.prefix = "netdata",
            .buffer = http,
            .labels_buffer = http_tags,
        };
        bool http_record = opentsdb_unittest_metric(
            &http_instance, true, "chart.name", "dimension.name", 42, true, number_cases[i].value, 0);
        if(number_cases[i].expect_record) {
            opentsdb_http_unittest_prefix(expected_http, "netdata", "chart.name", "dimension.name", 42);
            buffer_sprintf(expected_http, NETDATA_DOUBLE_FORMAT, number_cases[i].value);
            opentsdb_http_unittest_suffix(expected_http, "localhost", "");
        }

        if(http_record != number_cases[i].expect_record ||
           strcmp(buffer_tostring(http), buffer_tostring(expected_http)) != 0) {
            fprintf(
                stderr,
                "OpenTSDB HTTP %s omission or numeric output mismatch\nexpected: %s\nactual:   %s\n",
                number_cases[i].description,
                buffer_tostring(expected_http),
                buffer_tostring(http));
            errors++;
        }

        buffer_free(http_instance.chart_prefix_buffer);
        buffer_free(http_tags);
        buffer_free(expected_http);
        buffer_free(http);

        BUFFER *telnet = buffer_create(0, NULL);
        BUFFER *expected_telnet = buffer_create(0, NULL);
        buffer_strcat(telnet, "existing\n");
        buffer_strcat(expected_telnet, "existing\n");

        BUFFER *telnet_labels = buffer_create(0, NULL);
        buffer_strcat(telnet_labels, " host=localhost label=value");
        struct instance telnet_instance = {
            .config.prefix = "netdata",
            .buffer = telnet,
            .labels_buffer = telnet_labels,
        };
        bool telnet_record = opentsdb_unittest_metric(
            &telnet_instance, false, "chart.name", "dimension.name", 42, true, number_cases[i].value, 0);
        if(number_cases[i].expect_record) {
            opentsdb_telnet_unittest_prefix(expected_telnet, "netdata", "chart.name", "dimension.name", 42);
            buffer_sprintf(expected_telnet, NETDATA_DOUBLE_FORMAT, number_cases[i].value);
            opentsdb_telnet_unittest_suffix(expected_telnet, " host=localhost label=value");
        }

        if(telnet_record != number_cases[i].expect_record ||
           strcmp(buffer_tostring(telnet), buffer_tostring(expected_telnet)) != 0) {
            fprintf(
                stderr,
                "OpenTSDB Telnet %s omission or numeric output mismatch\nexpected: %s\nactual:   %s\n",
                number_cases[i].description,
                buffer_tostring(expected_telnet),
                buffer_tostring(telnet));
            errors++;
        }

        buffer_free(telnet_instance.chart_prefix_buffer);
        buffer_free(expected_telnet);
        buffer_free(telnet_labels);
        buffer_free(telnet);
    }

    BUFFER *http_batch = buffer_create(0, NULL);
    buffer_strcat(http_batch, "[\n");
    BUFFER *http_tags = buffer_create(0, NULL);
    opentsdb_http_unittest_suffix(http_tags, "localhost", "");
    struct instance http_instance = {
        .config.prefix = "netdata",
        .buffer = http_batch,
        .labels_buffer = http_tags,
    };
    bool first_record = opentsdb_unittest_metric(&http_instance, true, "chart", "first", 42, true, 1.5, 0);
    size_t before_omission = buffer_strlen(http_batch);
    bool omitted_record = opentsdb_unittest_metric(&http_instance, true, "chart", "omitted", 42, true, INFINITY, 0);
    size_t after_omission = buffer_strlen(http_batch);
    bool second_record = opentsdb_unittest_metric(&http_instance, true, "chart", "second", 42, true, -2.5, 0);
    buffer_strcat(http_batch, "\n]\n");

    BUFFER *expected_batch = buffer_create(0, NULL);
    buffer_strcat(expected_batch, "[\n");
    opentsdb_http_unittest_prefix(expected_batch, "netdata", "chart", "first", 42);
    buffer_sprintf(expected_batch, NETDATA_DOUBLE_FORMAT, (NETDATA_DOUBLE)1.5);
    opentsdb_http_unittest_suffix(expected_batch, "localhost", "");
    buffer_strcat(expected_batch, ",\n");
    opentsdb_http_unittest_prefix(expected_batch, "netdata", "chart", "second", 42);
    buffer_sprintf(expected_batch, NETDATA_DOUBLE_FORMAT, (NETDATA_DOUBLE)-2.5);
    opentsdb_http_unittest_suffix(expected_batch, "localhost", "");
    buffer_strcat(expected_batch, "\n]\n");

    if(!first_record || omitted_record || !second_record || after_omission != before_omission ||
       strcmp(buffer_tostring(http_batch), buffer_tostring(expected_batch)) != 0) {
        fprintf(
            stderr,
            "OpenTSDB HTTP omitted value changed array framing\nexpected: %s\nactual:   %s\n",
            buffer_tostring(expected_batch),
            buffer_tostring(http_batch));
        errors++;
    }

    buffer_free(http_instance.chart_prefix_buffer);
    buffer_free(expected_batch);
    buffer_free(http_batch);

    BUFFER *integers = buffer_create(0, NULL);
    buffer_strcat(integers, "[\n");
    struct instance integer_instance = {
        .config.prefix = "netdata",
        .buffer = integers,
        .labels_buffer = http_tags,
    };
    bool integer_record = opentsdb_unittest_metric(
        &integer_instance, true, "chart", "integer", 42, false, NAN, INT64_MIN);
    BUFFER *expected_integers = buffer_create(0, NULL);
    buffer_strcat(expected_integers, "[\n");
    opentsdb_http_unittest_prefix(expected_integers, "netdata", "chart", "integer", 42);
    buffer_sprintf(expected_integers, COLLECTED_NUMBER_FORMAT, (collected_number)INT64_MIN);
    opentsdb_http_unittest_suffix(expected_integers, "localhost", "");
    if(!integer_record || strcmp(buffer_tostring(integers), buffer_tostring(expected_integers)) != 0) {
        fprintf(stderr, "OpenTSDB collected integer output changed\n");
        errors++;
    }
    buffer_free(integer_instance.chart_prefix_buffer);
    buffer_free(expected_integers);
    buffer_free(integers);
    buffer_free(http_tags);

    return errors;
}

static void opentsdb_unittest_collected(
    struct instance *instance, RRDSET *st, const char *id, const char *name,
    bool is_float, int64_t int_value, NETDATA_DOUBLE float_value, time_t timestamp) {
    RRDDIM *rd = opentsdb_unittest_dimension(st, id, name);
    if(is_float) {
        rd->collector.options |= RRDDIM_OPTION_VALUE_FLOAT;
        rd->collector.collected.f.last_collected_value = float_value;
    }
    else
        rd->collector.collected.i.last_collected_value = (collected_number)int_value;
    rd->collector.last_collected_time.tv_sec = timestamp;

    instance->metric_formatting(instance, rd);
    opentsdb_unittest_dimension_free(rd);
}

static RRDHOST *opentsdb_unittest_host(const char *hostname, const char *label, RRDLABEL_SRC label_source) {
    RRDHOST *host = callocz(1, sizeof(*host));
    spinlock_init(&host->rrdhost_update_lock);
    host->hostname = string_strdupz(hostname);
    host->program_name = string_strdupz("netdata");
    host->program_version = string_strdupz("1");
    host->rrdlabels = rrdlabels_create();
    rrdlabels_add(host->rrdlabels, "label", label, label_source);
    return host;
}

static void opentsdb_unittest_host_free(RRDHOST *host) {
    rrdlabels_destroy(host->rrdlabels);
    string_freez(host->hostname);
    string_freez(host->program_name);
    string_freez(host->program_version);
    freez(host);
}

// Drive the real host, chart and metric callbacks across two hosts and four charts, on the collected and the stored
// paths, and compare the bytes with the record format the connector has always produced.
static int opentsdb_callbacks_unittest(bool http) {
    int errors = 0;
    const char *type = http ? "HTTP" : "Telnet";

    struct instance instance = { 0 };
    instance.config.type = http ? EXPORTING_CONNECTOR_TYPE_OPENTSDB_HTTP : EXPORTING_CONNECTOR_TYPE_OPENTSDB;
    instance.config.prefix = "netdata";
    instance.config.options =
        EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES | EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance.start_host_formatting = http ? format_host_labels_opentsdb_http : format_host_labels_opentsdb_telnet;
    instance.start_chart_formatting = format_chart_opentsdb;
    instance.metric_formatting =
        http ? format_dimension_collected_opentsdb_http : format_dimension_collected_opentsdb_telnet;
    instance.end_host_formatting = flush_host_labels;
    instance.buffer = buffer_create(0, NULL);
    if(http)
        open_batch_json_http(&instance);

    // first host: one configured label, names on
    RRDHOST *host1 = opentsdb_unittest_host("host \"1\"", "v a", RRDLABEL_SRC_CONFIG);
    instance.start_host_formatting(&instance, host1);
    RRDSET *st1 = opentsdb_unittest_chart("a.b", "a name");
    instance.start_chart_formatting(&instance, st1);
    opentsdb_unittest_collected(&instance, st1, "d1", "n 1", false, INT64_MIN, 0.0, 1700000000);
    opentsdb_unittest_collected(&instance, st1, "d2", "n2", true, 0, 2.5, 1700000001);
    opentsdb_unittest_collected(&instance, st1, "d3", NULL, false, 42, 0.0, -1);
    opentsdb_unittest_collected(&instance, st1, "d4", "n4", true, 0, NAN, 1700000003);
    instance.end_host_formatting(&instance, host1);

    // second host: its only label is filtered out, names off
    instance.config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    RRDHOST *host2 = opentsdb_unittest_host("host2", "auto", RRDLABEL_SRC_AUTO);
    instance.start_host_formatting(&instance, host2);
    RRDSET *st2 = opentsdb_unittest_chart("c.d", "c name");
    instance.start_chart_formatting(&instance, st2);
    opentsdb_unittest_collected(&instance, st2, "e1", "m1", false, INT64_MAX, 0.0, 7);

    const char *expected = http ?
        "[\n"
        "{\"metric\":\"netdata.a_name.n_1\",\"timestamp\":1700000000,\"value\":-9223372036854775808,"
        "\"tags\":{\"host\":\"host \\\"1\\\"\",\"label\":\"v_a\"}},\n"
        "{\"metric\":\"netdata.a_name.n2\",\"timestamp\":1700000001,\"value\":2.5000000,"
        "\"tags\":{\"host\":\"host \\\"1\\\"\",\"label\":\"v_a\"}},\n"
        "{\"metric\":\"netdata.a_name.d3\",\"timestamp\":18446744073709551615,\"value\":42,"
        "\"tags\":{\"host\":\"host \\\"1\\\"\",\"label\":\"v_a\"}},\n"
        "{\"metric\":\"netdata.c.d.e1\",\"timestamp\":7,\"value\":9223372036854775807,"
        "\"tags\":{\"host\":\"host2\"}}" :
        "put netdata.a_name.n_1 1700000000 -9223372036854775808 host=host_\"1\" label=v_a\n"
        "put netdata.a_name.n2 1700000001 2.5000000 host=host_\"1\" label=v_a\n"
        "put netdata.a_name.d3 18446744073709551615 42 host=host_\"1\" label=v_a\n"
        "put netdata.c.d.e1 7 9223372036854775807 host=host2 \n";
    if(strcmp(buffer_tostring(instance.buffer), expected) != 0) {
        fprintf(stderr, "OpenTSDB %s callbacks output mismatch\nexpected: %s\nactual:   %s\n",
                type, expected, buffer_tostring(instance.buffer));
        errors++;
    }

    // stored data source: a skipped (NaN or infinite) value writes nothing and does not build the chart part
    buffer_flush(instance.buffer);
    if(http)
        open_batch_json_http(&instance);
    instance.config.options = EXPORTING_SOURCE_DATA_AVERAGE | EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    instance.metric_formatting = http ? format_dimension_stored_opentsdb_http : format_dimension_stored_opentsdb_telnet;
    size_t empty = buffer_strlen((BUFFER *)instance.buffer);

    RRDSET *st3 = opentsdb_unittest_chart("e.f", "e name");
    instance.start_chart_formatting(&instance, st3);
    RRDDIM *rd = opentsdb_unittest_dimension(st3, "g1", NULL);
    if(http)
        format_opentsdb_http_metric(&instance, rd, 10, true, NAN, 0);
    else
        format_opentsdb_telnet_metric(&instance, rd, 10, true, -INFINITY, 0);
    opentsdb_unittest_dimension_free(rd);
    if(buffer_strlen(instance.chart_prefix_buffer) || buffer_strlen((BUFFER *)instance.buffer) != empty) {
        fprintf(stderr, "OpenTSDB %s stored: a skipped value wrote bytes or built the chart part\n", type);
        errors++;
    }

    const struct {
        const char *id;
        NETDATA_DOUBLE value;
        time_t last_t;
    } stored[] = {
        { "g2", -1.25, 0 },
        { "g3", NAN, 12 },
        { "g4", 3.0, -1 },
    };
    for(size_t i = 0; i < _countof(stored); i++) {
        rd = opentsdb_unittest_dimension(st3, stored[i].id, NULL);
        if(http)
            format_opentsdb_http_metric(&instance, rd, stored[i].last_t, true, stored[i].value, 0);
        else
            format_opentsdb_telnet_metric(&instance, rd, stored[i].last_t, true, stored[i].value, 0);
        opentsdb_unittest_dimension_free(rd);
    }

    // a chart whose values are all skipped writes nothing
    instance.start_chart_formatting(&instance, st2);
    rd = opentsdb_unittest_dimension(st2, "e1", NULL);
    if(http)
        format_opentsdb_http_metric(&instance, rd, 14, true, INFINITY, 0);
    else
        format_opentsdb_telnet_metric(&instance, rd, 14, true, NAN, 0);
    opentsdb_unittest_dimension_free(rd);

    expected = http ?
        "[\n"
        "{\"metric\":\"netdata.e.f.g2\",\"timestamp\":0,\"value\":-1.2500000,\"tags\":{\"host\":\"host2\"}},\n"
        "{\"metric\":\"netdata.e.f.g4\",\"timestamp\":18446744073709551615,\"value\":3.0000000,"
        "\"tags\":{\"host\":\"host2\"}}" :
        "put netdata.e.f.g2 0 -1.2500000 host=host2 \n"
        "put netdata.e.f.g4 18446744073709551615 3.0000000 host=host2 \n";
    if(strcmp(buffer_tostring(instance.buffer), expected) != 0 || buffer_strlen(instance.chart_prefix_buffer)) {
        fprintf(stderr, "OpenTSDB %s stored records mismatch\nexpected: %s\nactual:   %s\n",
                type, expected, buffer_tostring(instance.buffer));
        errors++;
    }

    instance.end_host_formatting(&instance, host2);
    if(buffer_strlen(instance.labels_buffer)) {
        fprintf(stderr, "OpenTSDB %s host tags survived the end of the host\n", type);
        errors++;
    }

    opentsdb_unittest_chart_free(st1);
    opentsdb_unittest_chart_free(st2);
    opentsdb_unittest_chart_free(st3);
    opentsdb_unittest_host_free(host1);
    opentsdb_unittest_host_free(host2);
    buffer_free(instance.buffer);
    buffer_free(instance.labels_buffer);
    buffer_free(instance.chart_prefix_buffer);
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

    errors += opentsdb_number_unittest();
    errors += opentsdb_callbacks_unittest(true);
    errors += opentsdb_callbacks_unittest(false);

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
    opentsdb_telnet_unittest_prefix(wb, prefix, "chart.name", "dimension.name", 42);
    buffer_strcat(wb, "1.5");
    opentsdb_telnet_unittest_suffix(wb, buffer_tostring(host_and_labels));

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

static const struct {
    const char *description;
    const char *prefix;
    const char *hostname;
    const char *labels;
    const char *expected;
} opentsdb_telnet_cases[] = {
    { "ordinary metadata", "netdata", "localhost", " label=value",
      "put netdata.chart.name.dimension.name 42 1.5 host=localhost label=value\n" },
    { "allowed punctuation", "Netdata-A_9/part.", "Host-A_9/path.example", " label=value",
      "put Netdata-A_9/part..chart.name.dimension.name 42 1.5 host=Host-A_9/path.example label=value\n" },
    { "hostile metadata", "pre fix\tbad\r\nput", "host name\tbad\r\nput", " label=value",
      "put pre_fix_bad__put.chart.name.dimension.name 42 1.5 host=host_name_bad__put label=value\n" },
    { "all-invalid metadata", " \t\r\n", " \t\r\n", " label=value",
      "put ____.chart.name.dimension.name 42 1.5 host=____ label=value\n" },
    { "empty metadata", "", "", "",
      "put .chart.name.dimension.name 42 1.5 host=\n" },
    { "colliding invalid metadata", "pre fix", "host name", "",
      "put pre_fix.chart.name.dimension.name 42 1.5 host=host_name\n" },
    { "colliding valid metadata", "pre_fix", "host_name", "",
      "put pre_fix.chart.name.dimension.name 42 1.5 host=host_name\n" },
    // netdata/netdata#23684: the configured hostname must survive intact. Only what can corrupt the
    // record is replaced, so every printable byte the user configured reaches the destination.
    { "colon in metadata", "net:data", "Dev::MyHost", " label=value",
      "put net:data.chart.name.dimension.name 42 1.5 host=Dev::MyHost label=value\n" },
    { "preserved punctuation", "pre=fix;a,b+c%d@e#f",
      "host=name;a,b+c(d)e[f]!g~h*i?j&k$l|m<n>o^p{q}r'\"s\\t", " label=value",
      "put pre=fix;a,b+c%d@e#f.chart.name.dimension.name 42 1.5 "
      "host=host=name;a,b+c(d)e[f]!g~h*i?j&k$l|m<n>o^p{q}r'\"s\\t label=value\n" },
    { "utf8 metadata", "pr\xc3\xa9""fix", "Dev::\xc3\x9c""n\xc3\xaf""c\xc3\xb8""de", " label=value",
      "put pr\xc3\xa9""fix.chart.name.dimension.name 42 1.5 "
      "host=Dev::\xc3\x9c""n\xc3\xaf""c\xc3\xb8""de label=value\n" },
    // an invalid sequence is copied byte for byte, so metadata we cannot interpret is not mangled,
    // except where the lone byte value is itself a control or whitespace codepoint (0x80-0xA0)
    { "malformed utf8 metadata", "pre\xc3""fix", "host\xff""name", " label=value",
      "put pre\xc3""fix.chart.name.dimension.name 42 1.5 host=host\xff""name label=value\n" },
    // non-breaking space: whitespace at the destination's splitter, replaced byte-length preserving
    { "unicode whitespace metadata", "pre\xc2\xa0""fix", "host\xe2\x80\x83name", " label=value",
      "put pre__fix.chart.name.dimension.name 42 1.5 host=host___name label=value\n" },
    // C1 controls are the same hazard as C0 in their multi-byte form: U+009B is the
    // single-character CSI. Two source bytes become two underscores, keeping the length.
    { "c1 control in metadata", "pre\xc2\x9b""fix", "host\xc2\x80""name", " label=value",
      "put pre__fix.chart.name.dimension.name 42 1.5 host=host__name label=value\n" },
    // a lead byte with no continuation left in the buffer is consumed one byte at a time, so a
    // truncated sequence is not silently rewritten. The orphan continuation byte in the hostname is
    // the documented exception: 0x82 decodes to its own value, which is a C1 control.
    { "truncated utf8 at end of metadata", "prefix\xc3", "hostname\xe2\x82", " label=value",
      "put prefix\xc3.chart.name.dimension.name 42 1.5 host=hostname\xe2_ label=value\n" },
    // the same exception, in the form most likely to occur: a lone 0x85 or 0xA0 decodes to its own
    // byte value, which is the NEL and the NBSP codepoint
    { "lone whitespace-valued bytes", "pre\x85""fix", "host\xa0""name", " label=value",
      "put pre_fix.chart.name.dimension.name 42 1.5 host=host_name label=value\n" },
    // control bytes cannot break our framing, but are never legitimate metadata and would be
    // interpreted by whatever consumes the record downstream
    { "control bytes in metadata", "pre\x01\x1b""fix", "host\x0b\x7f""name", " label=value",
      "put pre__fix.chart.name.dimension.name 42 1.5 host=host__name label=value\n" },
};

int exporting_opentsdb_telnet_unittest(void) {
    int errors = 0;

    for(size_t i = 0; i < sizeof(opentsdb_telnet_cases) / sizeof(opentsdb_telnet_cases[0]); i++)
        errors += opentsdb_telnet_unittest_case(
            opentsdb_telnet_cases[i].description,
            opentsdb_telnet_cases[i].prefix,
            opentsdb_telnet_cases[i].hostname,
            opentsdb_telnet_cases[i].labels,
            opentsdb_telnet_cases[i].expected);

    char long_prefix[4097];
    char long_hostname[4097];
    memset(long_prefix, 'p', sizeof(long_prefix) - 1);
    memset(long_hostname, 'h', sizeof(long_hostname) - 1);
    long_prefix[1024] = '\n';
    long_prefix[2048] = ':';
    long_hostname[3072] = '\t';
    long_hostname[2048] = ':';
    long_prefix[sizeof(long_prefix) - 1] = '\0';
    long_hostname[sizeof(long_hostname) - 1] = '\0';

    BUFFER *host_and_labels = buffer_create(0, NULL);
    format_opentsdb_telnet_host(host_and_labels, long_hostname);

    BUFFER *wb = buffer_create(0, NULL);
    opentsdb_telnet_unittest_prefix(wb, long_prefix, "chart.name", "dimension.name", 42);
    buffer_strcat(wb, "1.5");
    opentsdb_telnet_unittest_suffix(wb, buffer_tostring(host_and_labels));

    const size_t expected_length = strlen("put ") + strlen(long_prefix) + strlen(".chart.name.dimension.name 42 1.5 host=") +
                                   strlen(long_hostname) + 1;
    const size_t prefix_offset = sizeof("put ") - 1;
    const size_t hostname_offset =
        prefix_offset + strlen(long_prefix) + strlen(".chart.name.dimension.name 42 1.5 host=");
    if(buffer_strlen(wb) != expected_length || wb->buffer[prefix_offset + 1024] != '_' ||
       strchr(buffer_tostring(wb), '\t') || strchr(buffer_tostring(wb), '\r') ||
       strchr(buffer_tostring(wb), '\n') != &wb->buffer[wb->len - 1]) {
        fprintf(stderr, "OpenTSDB Telnet long metadata was truncated or retained a protocol delimiter\n");
        errors++;
    }

    // the same length-preserving pass must leave configured punctuation alone at scale, not only in
    // the short cases above
    if(wb->buffer[prefix_offset + 2048] != ':' || wb->buffer[hostname_offset + 2048] != ':' ||
       wb->buffer[hostname_offset + 3072] != '_') {
        fprintf(stderr, "OpenTSDB Telnet long metadata lost a valid character or kept a delimiter\n");
        errors++;
    }
    buffer_free(wb);
    buffer_free(host_and_labels);

    return errors;
}
