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

    connector_specific_data->ssl = NETDATA_SSL_UNSET_CONNECTION;
    if (instance->config.options & EXPORTING_OPTION_USE_TLS) {
        netdata_ssl_initialize_ctx(NETDATA_SSL_EXPORTING_CTX);
    }

    instance->start_batch_formatting = NULL;
    instance->start_host_formatting = format_host_labels_graphite_plaintext;
    instance->start_chart_formatting = format_chart_graphite_plaintext;

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

    instance->buffer = (void *)buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    if (!instance->buffer) {
        netdata_log_error("EXPORTING: cannot create buffer for graphite exporting connector instance %s", instance->config.name);
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
 * Copy a label value and substitute underscores in place of characters which can't be used in Graphite output
 *
 * @param dst a destination string.
 * @param src a source string.
 * @param len the maximum number of characters copied.
 */

void sanitize_graphite_label_value(char *dst, const char *src, size_t len)
{
    while (*src != '\0' && len) {
        if (isspace((uint8_t)*src) || *src == ';' || *src == '~')
            *dst++ = '_';
        else
            *dst++ = *src;
        src++;
        len--;
    }
    *dst = '\0';
}

static void sanitize_graphite_metric_path_value(char *dst, const char *src, size_t len) {
    while(*src && len) {
        uint32_t codepoint;
        size_t bytes = exporting_utf8_decode(src, len, &codepoint);

        if(codepoint == ';' || exporting_unicode_is_whitespace(codepoint))
            memset(dst, '_', bytes);
        else
            memcpy(dst, src, bytes);

        dst += bytes;
        src += bytes;
        len -= bytes;
    }

    *dst = '\0';
}

static void buffer_strcat_graphite_metric_path_value(BUFFER *wb, const char *src) {
    size_t len = strlen(src);

    buffer_need_bytes(wb, len + 1);
    sanitize_graphite_metric_path_value(&wb->buffer[wb->len], src, len);
    wb->len += len;
    buffer_overflow_check(wb);
}

// The part of a record that depends only on the host and the chart: "<prefix>.<hostname>.<chart>.".
static void format_graphite_chart_prefix(BUFFER *wb, BUFFER *metric_prefix, const char *chart_name) {
    buffer_memcat(wb, buffer_tostring(metric_prefix), buffer_strlen(metric_prefix));
    buffer_putc(wb, '.');
    buffer_strcat(wb, chart_name);
    buffer_putc(wb, '.');
}

// Everything of a record up to the value: the chart part, the dimension and the host labels.
static void format_graphite_record_start(struct instance *instance, const char *dimension_name, size_t dimension_len) {
    BUFFER *wb = instance->buffer;
    BUFFER *chart_prefix = instance->chart_prefix_buffer;

    buffer_memcat(wb, buffer_tostring(chart_prefix), buffer_strlen(chart_prefix));
    buffer_memcat(wb, dimension_name, dimension_len);
    if(instance->labels_buffer)
        buffer_memcat(wb, buffer_tostring(instance->labels_buffer), buffer_strlen(instance->labels_buffer));
    buffer_putc(wb, ' ');
}

static void format_graphite_record_end(BUFFER *wb, time_t timestamp) {
    buffer_putc(wb, ' ');
    buffer_print_uint64(wb, (uint64_t)timestamp);
    buffer_putc(wb, '\n');
}

static void format_graphite_record_double(
    struct instance *instance,
    const char *dimension_name,
    size_t dimension_len,
    NETDATA_DOUBLE value,
    time_t timestamp) {
    format_graphite_record_start(instance, dimension_name, dimension_len);
    buffer_sprintf(instance->buffer, NETDATA_DOUBLE_FORMAT, value);
    format_graphite_record_end(instance->buffer, timestamp);
}

static void format_graphite_record_integer(
    struct instance *instance,
    const char *dimension_name,
    size_t dimension_len,
    int64_t value,
    time_t timestamp) {
    format_graphite_record_start(instance, dimension_name, dimension_len);
    buffer_print_int64(instance->buffer, value);
    format_graphite_record_end(instance->buffer, timestamp);
}

/**
 * Format host identity and labels for Graphite connector
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @return Always returns 0.
 */

int format_host_labels_graphite_plaintext(struct instance *instance, RRDHOST *host)
{
    if (!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_exporters);

    if (!instance->metric_prefix_buffer)
        instance->metric_prefix_buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    else
        buffer_flush(instance->metric_prefix_buffer);

    buffer_strcat_graphite_metric_path_value(instance->metric_prefix_buffer, instance->config.prefix);
    buffer_putc(instance->metric_prefix_buffer, '.');

    if (host == localhost)
        buffer_strcat_graphite_metric_path_value(instance->metric_prefix_buffer, instance->config.hostname);
    else {
        RRDHOST_IDENTITY identity = rrdhost_identity_acquire(host);
        buffer_strcat_graphite_metric_path_value(instance->metric_prefix_buffer, string2str(identity.hostname));
        rrdhost_identity_release(&identity);
    }

    if (unlikely(!sending_labels_configured(instance)))
        return 0;

    rrdlabels_to_buffer(host->rrdlabels, instance->labels_buffer, ";", "=", "", "",
                        exporting_labels_filter_callback, instance,
                        NULL, sanitize_graphite_label_value);

    return 0;
}

/**
 * Start a chart for Graphite connector
 *
 * The chart part of the records is the same for all dimensions of a chart. It is built once, by the first record
 * the chart writes, and reused for the rest: a chart whose values are all skipped reads none of its metadata.
 *
 * @param instance an instance data structure.
 * @param st a chart.
 * @return Always returns 0.
 */
int format_chart_graphite_plaintext(struct instance *instance, RRDSET *st __maybe_unused)
{
    if (instance->chart_prefix_buffer)
        buffer_flush(instance->chart_prefix_buffer);

    return 0;
}

static void graphite_chart_prefix_build(struct instance *instance, RRDSET *st)
{
    if (!instance->chart_prefix_buffer)
        instance->chart_prefix_buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

    if (buffer_strlen(instance->chart_prefix_buffer))
        return;

    char chart_name[RRD_ID_LENGTH_MAX + 1];
    exporting_name_copy(
        chart_name,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && st->name) ? rrdset_name(st) : rrdset_id(st),
        RRD_ID_LENGTH_MAX);

    format_graphite_chart_prefix(instance->chart_prefix_buffer, instance->metric_prefix_buffer, chart_name);
}

static size_t graphite_dimension_name_copy(struct instance *instance, RRDDIM *rd, char *dst)
{
    return exporting_name_copy(
        dst,
        (instance->config.options & EXPORTING_OPTION_SEND_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
        RRD_ID_LENGTH_MAX);
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
    graphite_chart_prefix_build(instance, rd->rrdset);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    size_t dimension_len = graphite_dimension_name_copy(instance, rd, dimension_name);

    if(rrddim_is_float(rd))
        format_graphite_record_double(
            instance,
            dimension_name,
            dimension_len,
            rrddim_last_collected_as_double(rd),
            rd->collector.last_collected_time.tv_sec);
    else
        format_graphite_record_integer(
            instance,
            dimension_name,
            dimension_len,
            rrddim_last_collected_raw_int(rd),
            rd->collector.last_collected_time.tv_sec);

    return 0;
}

static void format_dimension_stored_graphite_record(
    struct instance *instance, RRDDIM *rd, NETDATA_DOUBLE value, time_t last_t)
{
    if(isnan(value))
        return;

    graphite_chart_prefix_build(instance, rd->rrdset);

    char dimension_name[RRD_ID_LENGTH_MAX + 1];
    size_t dimension_len = graphite_dimension_name_copy(instance, rd, dimension_name);

    format_graphite_record_double(instance, dimension_name, dimension_len, value, last_t);
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
    time_t last_t = 0;
    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_t);
    format_dimension_stored_graphite_record(instance, rd, value, last_t);
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
        (unsigned long int) buffer_strlen(simple_connector_data->last_buffer->buffer));

    return;
}

typedef enum {
    GRAPHITE_TEST_COLLECTED_FLOAT,
    GRAPHITE_TEST_COLLECTED_INTEGER,
    GRAPHITE_TEST_STORED,
} GRAPHITE_TEST_BRANCH;

// Compose the record writers the way the callbacks do, for a chart named "chart.name" and a dimension named
// "dimension.name".
static void graphite_unittest_record(struct instance *instance, GRAPHITE_TEST_BRANCH branch) {
    if(!instance->chart_prefix_buffer)
        instance->chart_prefix_buffer = buffer_create(0, NULL);
    buffer_flush(instance->chart_prefix_buffer);
    format_graphite_chart_prefix(instance->chart_prefix_buffer, instance->metric_prefix_buffer, "chart.name");

    const char *dimension = "dimension.name";
    switch(branch) {
        case GRAPHITE_TEST_COLLECTED_FLOAT:
            format_graphite_record_double(instance, dimension, strlen(dimension), 1.5, 42);
            break;
        case GRAPHITE_TEST_COLLECTED_INTEGER:
            format_graphite_record_integer(instance, dimension, strlen(dimension), 15, 42);
            break;
        case GRAPHITE_TEST_STORED:
            format_graphite_record_double(instance, dimension, strlen(dimension), -2.5, 42);
            break;
    }
}

static int graphite_unittest_case(
    const char *description,
    GRAPHITE_TEST_BRANCH branch,
    const char *prefix,
    const char *hostname,
    const char *labels,
    const char *expected) {
    struct instance instance = { 0 };
    instance.config.prefix = prefix;
    instance.buffer = buffer_create(0, NULL);
    instance.metric_prefix_buffer = buffer_create(0, NULL);
    instance.labels_buffer = buffer_create(0, NULL);
    BUFFER *wb = instance.buffer;

    buffer_strcat_graphite_metric_path_value(instance.metric_prefix_buffer, prefix);
    buffer_putc(instance.metric_prefix_buffer, '.');
    buffer_strcat_graphite_metric_path_value(instance.metric_prefix_buffer, hostname);
    buffer_strcat(instance.labels_buffer, labels);

    graphite_unittest_record(&instance, branch);

    int errors = 0;
    if(strcmp(buffer_tostring(instance.buffer), expected) != 0) {
        fprintf(
            stderr,
            "Graphite %s output mismatch\nexpected: %s\nactual:   %s\n",
            description,
            expected,
            buffer_tostring(instance.buffer));
        errors++;
    }

    size_t newline_count = 0;
    for(const char *s = buffer_tostring(instance.buffer); *s; s++) {
        if(*s == '\n')
            newline_count++;
        else if(*s == '\r' || *s == '\t' || *s == '\v' || *s == '\f') {
            fprintf(stderr, "Graphite %s output retained a protocol delimiter\n", description);
            errors++;
            break;
        }
    }

    if(newline_count != 1 || !buffer_strlen(wb) || wb->buffer[wb->len - 1] != '\n') {
        fprintf(stderr, "Graphite %s output is not exactly one newline-terminated record\n", description);
        errors++;
    }

    buffer_free(instance.chart_prefix_buffer);
    buffer_free(instance.labels_buffer);
    buffer_free(instance.metric_prefix_buffer);
    buffer_free(instance.buffer);
    return errors;
}

static int graphite_unicode_whitespace_unittest(void) {
    static const char whitespace[] =
        "\x09\x0a\x0b\x0c\x0d\x1c\x1d\x1e\x1f\x20"
        "\xc2\x85\xc2\xa0\xe1\x9a\x80"
        "\xe2\x80\x80\xe2\x80\x81\xe2\x80\x82\xe2\x80\x83\xe2\x80\x84\xe2\x80\x85"
        "\xe2\x80\x86\xe2\x80\x87\xe2\x80\x88\xe2\x80\x89\xe2\x80\x8a"
        "\xe2\x80\xa8\xe2\x80\xa9\xe2\x80\xaf\xe2\x81\x9f\xe3\x80\x80";

    const size_t whitespace_bytes = sizeof(whitespace) - 1;
    char prefix[sizeof(whitespace) + 1];
    char hostname[sizeof(whitespace) + 1];
    char normalized[sizeof(whitespace)];

    prefix[0] = 'p';
    hostname[0] = 'h';
    memcpy(&prefix[1], whitespace, sizeof(whitespace));
    memcpy(&hostname[1], whitespace, sizeof(whitespace));
    memset(normalized, '_', whitespace_bytes);
    normalized[whitespace_bytes] = '\0';

    int errors = 0;
    char expected[512];

    snprintf(
        expected,
        sizeof(expected),
        "p%s.h%s.chart.name.dimension.name 1.5000000 42\n",
        normalized,
        normalized);
    errors += graphite_unittest_case(
        "collected floating Unicode whitespace", GRAPHITE_TEST_COLLECTED_FLOAT, prefix, hostname, "", expected);

    snprintf(expected, sizeof(expected), "p%s.h%s.chart.name.dimension.name 15 42\n", normalized, normalized);
    errors += graphite_unittest_case(
        "collected integer Unicode whitespace", GRAPHITE_TEST_COLLECTED_INTEGER, prefix, hostname, "", expected);

    snprintf(
        expected,
        sizeof(expected),
        "p%s.h%s.chart.name.dimension.name -2.5000000 42\n",
        normalized,
        normalized);
    errors += graphite_unittest_case(
        "stored Unicode whitespace", GRAPHITE_TEST_STORED, prefix, hostname, "", expected);

    return errors;
}

static RRDSET *graphite_unittest_chart(const char *id, const char *name) {
    RRDSET *st = callocz(1, sizeof(*st));
    st->id = string_strdupz(id);
    st->name = string_strdupz(name);
    return st;
}

static void graphite_unittest_chart_free(RRDSET *st) {
    string_freez(st->id);
    string_freez(st->name);
    freez(st);
}

// a NULL name leaves rd->name unset, which makes the connector fall back to the id
static void graphite_unittest_dimension(
    struct instance *instance, RRDSET *st, const char *id, const char *name,
    bool is_float, int64_t int_value, NETDATA_DOUBLE float_value, time_t timestamp) {
    RRDDIM rd = { 0 };
    rd.rrdset = st;
    rd.id = string_strdupz(id);
    rd.name = name ? string_strdupz(name) : NULL;
    if(is_float) {
        rd.collector.options |= RRDDIM_OPTION_VALUE_FLOAT;
        rd.collector.collected.f.last_collected_value = float_value;
    }
    else
        rd.collector.collected.i.last_collected_value = (collected_number)int_value;
    rd.collector.last_collected_time.tv_sec = timestamp;

    instance->metric_formatting(instance, &rd);

    string_freez(rd.id);
    string_freez(rd.name);
}

static void graphite_unittest_stored(
    struct instance *instance, RRDSET *st, const char *id, NETDATA_DOUBLE value, time_t last_t) {
    RRDDIM rd = { 0 };
    rd.rrdset = st;
    rd.id = string_strdupz(id);
    format_dimension_stored_graphite_record(instance, &rd, value, last_t);
    string_freez(rd.id);
}

// Drive the real chart and metric callbacks across two hosts and four charts, on the collected and the stored paths,
// and compare the bytes with the record format the connector has always produced.
static int graphite_callbacks_unittest(void) {
    int errors = 0;

    struct instance instance = { 0 };
    instance.config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED | EXPORTING_OPTION_SEND_NAMES;
    instance.start_chart_formatting = format_chart_graphite_plaintext;
    instance.metric_formatting = format_dimension_collected_graphite_plaintext;
    instance.buffer = buffer_create(0, NULL);
    instance.metric_prefix_buffer = buffer_create(0, NULL);
    instance.labels_buffer = buffer_create(0, NULL);

    // first host: labels and names on
    buffer_strcat(instance.metric_prefix_buffer, "netdata.host1");
    buffer_strcat(instance.labels_buffer, ";k=v");
    RRDSET *st1 = graphite_unittest_chart("a.b", "a name");
    instance.start_chart_formatting(&instance, st1);
    graphite_unittest_dimension(&instance, st1, "d1", "n 1", false, INT64_MIN, 0.0, 1700000000);
    graphite_unittest_dimension(&instance, st1, "d2", "n2", true, 0, 2.5, 1700000001);
    graphite_unittest_dimension(&instance, st1, "d3", NULL, false, 42, 0.0, -1);

    // second host: labels and names off
    instance.config.options = EXPORTING_SOURCE_DATA_AS_COLLECTED;
    buffer_flush(instance.metric_prefix_buffer);
    buffer_strcat(instance.metric_prefix_buffer, "netdata.host2");
    buffer_flush(instance.labels_buffer);
    RRDSET *st2 = graphite_unittest_chart("c.d", "c name");
    instance.start_chart_formatting(&instance, st2);
    graphite_unittest_dimension(&instance, st2, "e1", "m1", false, INT64_MAX, 0.0, 7);

    const char *expected =
        "netdata.host1.a_name.n_1;k=v -9223372036854775808 1700000000\n"
        "netdata.host1.a_name.n2;k=v 2.5000000 1700000001\n"
        "netdata.host1.a_name.d3;k=v 42 18446744073709551615\n"
        "netdata.host2.c.d.e1 9223372036854775807 7\n";
    if(strcmp(buffer_tostring(instance.buffer), expected) != 0) {
        fprintf(stderr, "Graphite callbacks output mismatch\nexpected: %s\nactual:   %s\n",
                expected, buffer_tostring(instance.buffer));
        errors++;
    }

    // stored data source: a skipped (NaN) value writes nothing and does not build the chart part
    buffer_flush(instance.buffer);
    instance.config.options = EXPORTING_SOURCE_DATA_AVERAGE;
    RRDSET *st3 = graphite_unittest_chart("e.f", "e name");
    instance.start_chart_formatting(&instance, st3);
    graphite_unittest_stored(&instance, st3, "g1", NAN, 10);
    if(buffer_strlen(instance.chart_prefix_buffer) || buffer_strlen((BUFFER *)instance.buffer)) {
        fprintf(stderr, "Graphite stored: a skipped value wrote bytes or built the chart part\n");
        errors++;
    }
    graphite_unittest_stored(&instance, st3, "g2", -1.25, 0);
    graphite_unittest_stored(&instance, st3, "g3", NAN, 12);
    graphite_unittest_stored(&instance, st3, "g4", 3.0, -1);

    // a chart whose values are all skipped writes nothing
    instance.start_chart_formatting(&instance, st2);
    graphite_unittest_stored(&instance, st2, "e1", NAN, 14);

    expected =
        "netdata.host2.e.f.g2 -1.2500000 0\n"
        "netdata.host2.e.f.g4 3.0000000 18446744073709551615\n";
    if(strcmp(buffer_tostring(instance.buffer), expected) != 0 || buffer_strlen(instance.chart_prefix_buffer)) {
        fprintf(stderr, "Graphite stored records mismatch\nexpected: %s\nactual:   %s\n",
                expected, buffer_tostring(instance.buffer));
        errors++;
    }

    graphite_unittest_chart_free(st1);
    graphite_unittest_chart_free(st2);
    graphite_unittest_chart_free(st3);
    buffer_free(instance.buffer);
    buffer_free(instance.metric_prefix_buffer);
    buffer_free(instance.labels_buffer);
    buffer_free(instance.chart_prefix_buffer);
    return errors;
}

int exporting_graphite_unittest(void) {
    int errors = 0;

    errors += graphite_unittest_case(
        "collected floating ordinary metadata",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "netdata",
        "localhost",
        ";label=value",
        "netdata.localhost.chart.name.dimension.name;label=value 1.5000000 42\n");

    errors += graphite_unittest_case(
        "collected integer ordinary metadata",
        GRAPHITE_TEST_COLLECTED_INTEGER,
        "netdata",
        "localhost",
        "",
        "netdata.localhost.chart.name.dimension.name 15 42\n");

    errors += graphite_unittest_case(
        "stored ordinary metadata",
        GRAPHITE_TEST_STORED,
        "netdata",
        "localhost",
        "",
        "netdata.localhost.chart.name.dimension.name -2.5000000 42\n");


    errors += graphite_unittest_case(
        "accepted punctuation and UTF-8",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "Netdata-A_9/part.:@[]+()Ε",
        "Host-A_9/path.example:@[]+()λλάδα",
        "",
        "Netdata-A_9/part.:@[]+()Ε.Host-A_9/path.example:@[]+()λλάδα.chart.name.dimension.name 1.5000000 42\n");

    errors += graphite_unicode_whitespace_unittest();
    errors += graphite_callbacks_unittest();

    errors += graphite_unittest_case(
        "hostile metadata",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "pre fix;bad~tag\tbad\r\nmetric",
        "host name;bad~tag\tbad\r\nmetric",
        "",
        "pre_fix_bad~tag_bad__metric.host_name_bad~tag_bad__metric.chart.name.dimension.name 1.5000000 42\n");

    errors += graphite_unittest_case(
        "all-invalid metadata",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        " ;\t\r\n",
        " ;\t\r\n",
        "",
        "_____._____.chart.name.dimension.name 1.5000000 42\n");

    errors += graphite_unittest_case(
        "tilde compatibility",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "~pre~fix~",
        "~host~name~",
        "",
        "~pre~fix~.~host~name~.chart.name.dimension.name 1.5000000 42\n");

    errors += graphite_unittest_case(
        "empty metadata",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "",
        "",
        "",
        "..chart.name.dimension.name 1.5000000 42\n");

    errors += graphite_unittest_case(
        "colliding invalid metadata",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "pre fix",
        "host;name",
        "",
        "pre_fix.host_name.chart.name.dimension.name 1.5000000 42\n");

    errors += graphite_unittest_case(
        "colliding valid metadata",
        GRAPHITE_TEST_COLLECTED_FLOAT,
        "pre_fix",
        "host_name",
        "",
        "pre_fix.host_name.chart.name.dimension.name 1.5000000 42\n");

    char long_prefix[4097];
    char long_hostname[4097];
    memset(long_prefix, 'p', sizeof(long_prefix) - 1);
    memset(long_hostname, 'h', sizeof(long_hostname) - 1);
    long_prefix[1024] = '\n';
    long_hostname[3072] = '\t';
    long_prefix[sizeof(long_prefix) - 1] = '\0';
    long_hostname[sizeof(long_hostname) - 1] = '\0';

    struct instance instance = { 0 };
    instance.config.prefix = long_prefix;
    instance.buffer = buffer_create(0, NULL);
    instance.metric_prefix_buffer = buffer_create(0, NULL);
    BUFFER *wb = instance.buffer;
    buffer_strcat_graphite_metric_path_value(instance.metric_prefix_buffer, long_prefix);
    buffer_putc(instance.metric_prefix_buffer, '.');
    buffer_strcat_graphite_metric_path_value(instance.metric_prefix_buffer, long_hostname);
    graphite_unittest_record(&instance, GRAPHITE_TEST_COLLECTED_FLOAT);

    const size_t expected_length = strlen(long_prefix) + 1 + strlen(long_hostname) +
                                   strlen(".chart.name.dimension.name 1.5000000 42\n");
    if(buffer_strlen(wb) != expected_length || wb->buffer[1024] != '_' ||
       wb->buffer[strlen(long_prefix) + 1 + 3072] != '_' ||
       strchr(buffer_tostring(wb), '\t') || strchr(buffer_tostring(wb), '\r') ||
       strchr(buffer_tostring(wb), '\n') != &wb->buffer[wb->len - 1]) {
        fprintf(stderr, "Graphite long metadata was truncated or retained a protocol delimiter\n");
        errors++;
    }
    buffer_free(instance.chart_prefix_buffer);
    buffer_free(instance.metric_prefix_buffer);
    buffer_free(instance.buffer);

    RRDLABELS *labels = rrdlabels_create();
    rrdlabels_add(labels, "configured", "label value~bad", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "automatic", "excluded", RRDLABEL_SRC_AUTO);
    BUFFER *labels_buffer = buffer_create(0, NULL);
    struct instance labels_instance = { 0 };
    labels_instance.config.options = EXPORTING_OPTION_SEND_CONFIGURED_LABELS;
    rrdlabels_to_buffer(
        labels,
        labels_buffer,
        ";",
        "=",
        "",
        "",
        exporting_labels_filter_callback,
        &labels_instance,
        NULL,
        sanitize_graphite_label_value);
    if(strcmp(buffer_tostring(labels_buffer), ";configured=label_value_bad") != 0) {
        fprintf(stderr, "Graphite label filtering or sanitization changed\n");
        errors++;
    }
    buffer_free(labels_buffer);
    rrdlabels_destroy(labels);

    RRDHOST child = { 0 };
    spinlock_init(&child.rrdhost_update_lock);
    child.hostname = string_strdupz("child host");
    child.program_name = string_strdupz("netdata");
    child.program_version = string_strdupz("1");

    struct instance child_instance = { 0 };
    child_instance.config.prefix = "netdata";
    child_instance.buffer = buffer_create(0, NULL);
    format_host_labels_graphite_plaintext(&child_instance, &child);

    STRING *old_hostname = child.hostname;
    child.hostname = string_strdupz("renamed child");
    string_freez(old_hostname);

    graphite_unittest_record(&child_instance, GRAPHITE_TEST_COLLECTED_FLOAT);
    if(strcmp(
           buffer_tostring(child_instance.buffer),
           "netdata.child_host.chart.name.dimension.name 1.5000000 42\n") != 0) {
        fprintf(stderr, "Graphite child hostname snapshot did not survive identity replacement\n");
        errors++;
    }

    buffer_free(child_instance.chart_prefix_buffer);
    buffer_free(child_instance.labels_buffer);
    buffer_free(child_instance.metric_prefix_buffer);
    buffer_free(child_instance.buffer);
    string_freez(child.hostname);
    string_freez(child.program_name);
    string_freez(child.program_version);

    return errors;
}
