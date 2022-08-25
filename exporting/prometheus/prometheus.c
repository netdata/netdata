// SPDX-License-Identifier: GPL-3.0-or-later

#define EXPORTINGS_INTERNALS
#include "prometheus.h"

// ----------------------------------------------------------------------------
// PROMETHEUS
// /api/v1/allmetrics?format=prometheus and /api/v1/allmetrics?format=prometheus_all_hosts

static int is_matches_rrdset(struct instance *instance, RRDSET *st, SIMPLE_PATTERN *filter) {
    if (instance->config.options & EXPORTING_OPTION_SEND_NAMES) {
        return simple_pattern_matches(filter, st->name);
    }
    return simple_pattern_matches(filter, st->id);
}

/**
 * Check if a chart can be sent to Prometheus
 *
 * @param instance an instance data structure.
 * @param st a chart.
 * @param filter a simple pattern to match against.
 * @return Returns 1 if the chart can be sent, 0 otherwise.
 */
inline int can_send_rrdset(struct instance *instance, RRDSET *st, SIMPLE_PATTERN *filter)
{
#ifdef NETDATA_INTERNAL_CHECKS
    RRDHOST *host = st->rrdhost;
#endif

    // Do not send anomaly rates charts.
    if (st->state && st->state->is_ar_chart)
        return 0;

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_EXPORTING_IGNORE)))
        return 0;

    if (filter) {
        if (!is_matches_rrdset(instance, st, filter)) {
            return 0;
        }
    } else if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_EXPORTING_SEND))) {
        // we have not checked this chart
        if (is_matches_rrdset(instance, st, instance->config.charts_pattern)) {
            rrdset_flag_set(st, RRDSET_FLAG_EXPORTING_SEND);
        } else {
            rrdset_flag_set(st, RRDSET_FLAG_EXPORTING_IGNORE);
            debug(
                D_EXPORTING,
                "EXPORTING: not sending chart '%s' of host '%s', because it is disabled for exporting.",
                st->id,
                host->hostname);
            return 0;
        }
    }

    if (unlikely(!rrdset_is_available_for_exporting_and_alarms(st))) {
        debug(
            D_EXPORTING,
            "EXPORTING: not sending chart '%s' of host '%s', because it is not available for exporting.",
            st->id,
            host->hostname);
        return 0;
    }

    if (unlikely(
            st->rrd_memory_mode == RRD_MEMORY_MODE_NONE &&
            !(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED))) {
        debug(
            D_EXPORTING,
            "EXPORTING: not sending chart '%s' of host '%s' because its memory mode is '%s' and the exporting connector requires database access.",
            st->id,
            host->hostname,
            rrd_memory_mode_name(host->rrd_memory_mode));
        return 0;
    }

    return 1;
}

static struct prometheus_server {
    const char *server;
    uint32_t hash;
    RRDHOST *host;
    time_t last_access;
    struct prometheus_server *next;
} *prometheus_server_root = NULL;

static netdata_mutex_t prometheus_server_root_mutex = NETDATA_MUTEX_INITIALIZER;

/**
 * Clean server root local structure
 */
void prometheus_clean_server_root()
{
    if (prometheus_server_root) {
        netdata_mutex_lock(&prometheus_server_root_mutex);

        struct prometheus_server *ps;
        for (ps = prometheus_server_root; ps; ) {
            struct prometheus_server *current = ps;
            ps = ps->next;
            if(current->server)
                freez((void *)current->server);

            freez(current);
        }
        prometheus_server_root = NULL;
        netdata_mutex_unlock(&prometheus_server_root_mutex);
    }
}

/**
 * Get the last time when a Prometheus server scraped the Netdata Prometheus exporter.
 *
 * @param server the name of the Prometheus server.
 * @param host a data collecting host.
 * @param now actual time.
 * @return Returns the last time when the server accessed Netdata, or 0 if it is the first occurrence.
 */
static inline time_t prometheus_server_last_access(const char *server, RRDHOST *host, time_t now)
{
#ifdef UNIT_TESTING
    return 0;
#endif
    uint32_t hash = simple_hash(server);

    netdata_mutex_lock(&prometheus_server_root_mutex);

    struct prometheus_server *ps;
    for (ps = prometheus_server_root; ps; ps = ps->next) {
        if (host == ps->host && hash == ps->hash && !strcmp(server, ps->server)) {
            time_t last = ps->last_access;
            ps->last_access = now;
            netdata_mutex_unlock(&prometheus_server_root_mutex);
            return last;
        }
    }

    ps = callocz(1, sizeof(struct prometheus_server));
    ps->server = strdupz(server);
    ps->hash = hash;
    ps->host = host;
    ps->last_access = now;
    ps->next = prometheus_server_root;
    prometheus_server_root = ps;

    netdata_mutex_unlock(&prometheus_server_root_mutex);
    return 0;
}

/**
 * Copy and sanitize name.
 *
 * @param d a destination string.
 * @param s a source string.
 * @param usable the number of characters to copy.
 * @return Returns the length of the copied string.
 */
inline size_t prometheus_name_copy(char *d, const char *s, size_t usable)
{
    size_t n;

    for (n = 0; *s && n < usable; d++, s++, n++) {
        register char c = *s;

        if (!isalnum(c))
            *d = '_';
        else
            *d = c;
    }
    *d = '\0';

    return n;
}

/**
 * Copy and sanitize label.
 *
 * @param d a destination string.
 * @param s a source string.
 * @param usable the number of characters to copy.
 * @return Returns the length of the copied string.
 */
inline size_t prometheus_label_copy(char *d, const char *s, size_t usable)
{
    size_t n;

    // make sure we can escape one character without overflowing the buffer
    usable--;

    for (n = 0; *s && n < usable; d++, s++, n++) {
        register char c = *s;

        if (unlikely(c == '"' || c == '\\' || c == '\n')) {
            *d++ = '\\';
            n++;
        }
        *d = c;
    }
    *d = '\0';

    return n;
}

/**
 * Copy and sanitize units.
 *
 * @param d a destination string.
 * @param s a source string.
 * @param usable the number of characters to copy.
 * @param showoldunits set this flag to 1 to show old (before v1.12) units.
 * @return Returns the destination string.
 */
inline char *prometheus_units_copy(char *d, const char *s, size_t usable, int showoldunits)
{
    const char *sorig = s;
    char *ret = d;
    size_t n;

    // Fix for issue 5227
    if (unlikely(showoldunits)) {
        static struct {
            const char *newunit;
            uint32_t hash;
            const char *oldunit;
        } units[] = { { "KiB/s", 0, "kilobytes/s" },
                      { "MiB/s", 0, "MB/s" },
                      { "GiB/s", 0, "GB/s" },
                      { "KiB", 0, "KB" },
                      { "MiB", 0, "MB" },
                      { "GiB", 0, "GB" },
                      { "inodes", 0, "Inodes" },
                      { "percentage", 0, "percent" },
                      { "faults/s", 0, "page faults/s" },
                      { "KiB/operation", 0, "kilobytes per operation" },
                      { "milliseconds/operation", 0, "ms per operation" },
                      { NULL, 0, NULL } };
        static int initialized = 0;
        int i;

        if (unlikely(!initialized)) {
            for (i = 0; units[i].newunit; i++)
                units[i].hash = simple_hash(units[i].newunit);
            initialized = 1;
        }

        uint32_t hash = simple_hash(s);
        for (i = 0; units[i].newunit; i++) {
            if (unlikely(hash == units[i].hash && !strcmp(s, units[i].newunit))) {
                // info("matched extension for filename '%s': '%s'", filename, last_dot);
                s = units[i].oldunit;
                sorig = s;
                break;
            }
        }
    }
    *d++ = '_';
    for (n = 1; *s && n < usable; d++, s++, n++) {
        register char c = *s;

        if (!isalnum(c))
            *d = '_';
        else
            *d = c;
    }

    if (n == 2 && sorig[0] == '%') {
        n = 0;
        d = ret;
        s = "_percent";
        for (; *s && n < usable; n++)
            *d++ = *s++;
    } else if (n > 3 && sorig[n - 3] == '/' && sorig[n - 2] == 's') {
        n = n - 2;
        d -= 2;
        s = "_persec";
        for (; *s && n < usable; n++)
            *d++ = *s++;
    }

    *d = '\0';

    return ret;
}

/**
 * Format host labels for the Prometheus exporter
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 */

struct format_prometheus_label_callback {
    struct instance *instance;
    size_t count;
};

static int format_prometheus_label_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct format_prometheus_label_callback *d = (struct format_prometheus_label_callback *)data;

    if (!should_send_label(d->instance, ls)) return 0;

    char k[PROMETHEUS_ELEMENT_MAX + 1];
    char v[PROMETHEUS_ELEMENT_MAX + 1];

    prometheus_name_copy(k, name, PROMETHEUS_ELEMENT_MAX);
    prometheus_label_copy(v, value, PROMETHEUS_ELEMENT_MAX);

    if (*k && *v) {
        if (d->count > 0) buffer_strcat(d->instance->labels_buffer, ",");
        buffer_sprintf(d->instance->labels_buffer, "%s=\"%s\"", k, v);
        d->count++;
    }
    return 1;
}

void format_host_labels_prometheus(struct instance *instance, RRDHOST *host)
{
    if (unlikely(!sending_labels_configured(instance)))
        return;

    if (!instance->labels_buffer)
        instance->labels_buffer = buffer_create(1024);

    struct format_prometheus_label_callback tmp = {
        .instance = instance,
        .count = 0
    };
    rrdlabels_walkthrough_read(host->host_labels, format_prometheus_label_callback, &tmp);
}

struct host_variables_callback_options {
    RRDHOST *host;
    BUFFER *wb;
    EXPORTING_OPTIONS exporting_options;
    PROMETHEUS_OUTPUT_OPTIONS output_options;
    const char *prefix;
    const char *labels;
    time_t now;
    int host_header_printed;
    char name[PROMETHEUS_VARIABLE_MAX + 1];
};

/**
 * Print host variables.
 *
 * @param rv a variable.
 * @param data callback options.
 * @return Returns 1 if the chart can be sent, 0 otherwise.
 */
static int print_host_variables(RRDVAR *rv, void *data)
{
    struct host_variables_callback_options *opts = data;

    if (rv->options & (RRDVAR_OPTION_CUSTOM_HOST_VAR | RRDVAR_OPTION_CUSTOM_CHART_VAR)) {
        if (!opts->host_header_printed) {
            opts->host_header_printed = 1;

            if (opts->output_options & PROMETHEUS_OUTPUT_HELP) {
                buffer_sprintf(opts->wb, "\n# COMMENT global host and chart variables\n");
            }
        }

        NETDATA_DOUBLE value = rrdvar2number(rv);
        if (isnan(value) || isinf(value)) {
            if (opts->output_options & PROMETHEUS_OUTPUT_HELP)
                buffer_sprintf(
                    opts->wb, "# COMMENT variable \"%s\" is %s. Skipped.\n", rv->name, (isnan(value)) ? "NAN" : "INF");

            return 0;
        }

        char *label_pre = "";
        char *label_post = "";
        if (opts->labels && *opts->labels) {
            label_pre = "{";
            label_post = "}";
        }

        prometheus_name_copy(opts->name, rv->name, sizeof(opts->name));

        if (opts->output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
            buffer_sprintf(
                opts->wb,
                "%s_%s%s%s%s " NETDATA_DOUBLE_FORMAT " %llu\n",
                opts->prefix,
                opts->name,
                label_pre,
                opts->labels,
                label_post,
                value,
                opts->now * 1000ULL);
        else
            buffer_sprintf(
                opts->wb,
                "%s_%s%s%s%s " NETDATA_DOUBLE_FORMAT "\n",
                opts->prefix,
                opts->name,
                label_pre,
                opts->labels,
                label_post,
                value);

        return 1;
    }

    return 0;
}

struct gen_parameters {
    const char *prefix;
    char *context;
    char *suffix;

    char *chart;
    char *dimension;
    char *family;
    char *labels;

    PROMETHEUS_OUTPUT_OPTIONS output_options;
    RRDSET *st;
    RRDDIM *rd;

    const char *relation;
    const char *type;
};

/**
 * Write an as-collected help comment to a buffer.
 *
 * @param wb the buffer to write the comment to.
 * @param p parameters for generating the comment string.
 * @param homogeneous a flag for homogeneous charts.
 * @param prometheus_collector a flag for metrics from prometheus collector.
 */
static void generate_as_collected_prom_help(BUFFER *wb, struct gen_parameters *p, int homogeneous, int prometheus_collector)
{
    buffer_sprintf(wb, "# COMMENT %s_%s", p->prefix, p->context);

    if (!homogeneous)
        buffer_sprintf(wb, "_%s", p->dimension);

    buffer_sprintf(
        wb,
        "%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * ",
        p->suffix,
        (p->output_options & PROMETHEUS_OUTPUT_NAMES && p->st->name) ? p->st->name : p->st->id,
        p->st->context,
        p->st->family,
        (p->output_options & PROMETHEUS_OUTPUT_NAMES && p->rd->name) ? rrddim_name(p->rd) : rrddim_id(p->rd));

    if (prometheus_collector)
        buffer_sprintf(wb, "1 / 1");
    else
        buffer_sprintf(wb, COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT, p->rd->multiplier, p->rd->divisor);

    buffer_sprintf(wb, " %s %s (%s)\n", p->relation, p->st->units, p->type);
}

/**
 * Write an as-collected metric to a buffer.
 *
 * @param wb the buffer to write the metric to.
 * @param p parameters for generating the metric string.
 * @param homogeneous a flag for homogeneous charts.
 * @param prometheus_collector a flag for metrics from prometheus collector.
 */
static void generate_as_collected_prom_metric(BUFFER *wb, struct gen_parameters *p, int homogeneous, int prometheus_collector)
{
    buffer_sprintf(wb, "%s_%s", p->prefix, p->context);

    if (!homogeneous)
        buffer_sprintf(wb, "_%s", p->dimension);

    buffer_sprintf(wb, "%s{chart=\"%s\",family=\"%s\"", p->suffix, p->chart, p->family);

    if (homogeneous)
        buffer_sprintf(wb, ",dimension=\"%s\"", p->dimension);

    buffer_sprintf(wb, "%s} ", p->labels);

    if (prometheus_collector)
        buffer_sprintf(
            wb,
            NETDATA_DOUBLE_FORMAT,
            (NETDATA_DOUBLE)p->rd->last_collected_value * (NETDATA_DOUBLE)p->rd->multiplier /
                (NETDATA_DOUBLE)p->rd->divisor);
    else
        buffer_sprintf(wb, COLLECTED_NUMBER_FORMAT, p->rd->last_collected_value);

    if (p->output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
        buffer_sprintf(wb, " %llu\n", timeval_msec(&p->rd->last_collected_time));
    else
        buffer_sprintf(wb, "\n");
}

/**
 * Write metrics in Prometheus format to a buffer.
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @param filter_string a simple pattern filter.
 * @param wb the buffer to fill with metrics.
 * @param prefix a prefix for every metric.
 * @param exporting_options options to configure what data is exported.
 * @param allhosts set to 1 if host instance should be in the output for tags.
 * @param output_options options to configure the format of the output.
 */
static void rrd_stats_api_v1_charts_allmetrics_prometheus(
    struct instance *instance,
    RRDHOST *host,
    const char *filter_string,
    BUFFER *wb,
    const char *prefix,
    EXPORTING_OPTIONS exporting_options,
    int allhosts,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    SIMPLE_PATTERN *filter = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT);
    rrdhost_rdlock(host);

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, host->hostname, PROMETHEUS_ELEMENT_MAX);

    format_host_labels_prometheus(instance, host);

    buffer_sprintf(
        wb,
        "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"",
        hostname,
        host->program_name,
        host->program_version);

    if (instance->labels_buffer && *buffer_tostring(instance->labels_buffer)) {
        buffer_sprintf(wb, ",%s", buffer_tostring(instance->labels_buffer));
    }

    if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
        buffer_sprintf(wb, "} 1 %llu\n", now_realtime_usec() / USEC_PER_MS);
    else
        buffer_sprintf(wb, "} 1\n");

    char labels[PROMETHEUS_LABELS_MAX + 1] = "";
    if (allhosts) {
        snprintfz(labels, PROMETHEUS_LABELS_MAX, ",instance=\"%s\"", hostname);
     }

    if (instance->labels_buffer)
        buffer_flush(instance->labels_buffer);

    // send custom variables set for the host
    if (output_options & PROMETHEUS_OUTPUT_VARIABLES) {
        struct host_variables_callback_options opts = { .host = host,
                                                        .wb = wb,
                                                        .labels = (labels[0] == ',') ? &labels[1] : labels,
                                                        .exporting_options = exporting_options,
                                                        .output_options = output_options,
                                                        .prefix = prefix,
                                                        .now = now_realtime_sec(),
                                                        .host_header_printed = 0 };
        foreach_host_variable_callback(host, print_host_variables, &opts);
    }

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host)
    {

        if (likely(can_send_rrdset(instance, st, filter))) {
            rrdset_rdlock(st);

            char chart[PROMETHEUS_ELEMENT_MAX + 1];
            char context[PROMETHEUS_ELEMENT_MAX + 1];
            char family[PROMETHEUS_ELEMENT_MAX + 1];
            char units[PROMETHEUS_ELEMENT_MAX + 1] = "";

            prometheus_label_copy(
                chart, (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id, PROMETHEUS_ELEMENT_MAX);
            prometheus_label_copy(family, st->family, PROMETHEUS_ELEMENT_MAX);
            prometheus_name_copy(context, st->context, PROMETHEUS_ELEMENT_MAX);

            int as_collected = (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_AS_COLLECTED);
            int homogeneous = 1;
            int prometheus_collector = 0;
            if (as_collected) {
                if (rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK))
                    rrdset_update_heterogeneous_flag(st);

                if (rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
                    homogeneous = 0;

                if (st->module_name && !strcmp(st->module_name, "prometheus"))
                    prometheus_collector = 1;
            } else {
                if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_AVERAGE &&
                    !(output_options & PROMETHEUS_OUTPUT_HIDEUNITS))
                    prometheus_units_copy(
                        units, st->units, PROMETHEUS_ELEMENT_MAX, output_options & PROMETHEUS_OUTPUT_OLDUNITS);
            }

            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                buffer_sprintf(
                    wb,
                    "\n# COMMENT %s chart \"%s\", context \"%s\", family \"%s\", units \"%s\"\n",
                    (homogeneous) ? "homogeneous" : "heterogeneous",
                    (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id,
                    st->context,
                    st->family,
                    st->units);

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st)
            {
                if (rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    char dimension[PROMETHEUS_ELEMENT_MAX + 1];
                    char *suffix = "";

                    if (as_collected) {
                        // we need as-collected / raw data

                        struct gen_parameters p;
                        p.prefix = prefix;
                        p.context = context;
                        p.suffix = suffix;
                        p.chart = chart;
                        p.dimension = dimension;
                        p.family = family;
                        p.labels = labels;
                        p.output_options = output_options;
                        p.st = st;
                        p.rd = rd;

                        if (unlikely(rd->last_collected_time.tv_sec < instance->after))
                            continue;

                        p.type = "gauge";
                        p.relation = "gives";
                        if (rd->algorithm == RRD_ALGORITHM_INCREMENTAL ||
                            rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL) {
                            p.type = "counter";
                            p.relation = "delta gives";
                            if (!prometheus_collector)
                                p.suffix = "_total";
                        }

                        if (homogeneous) {
                            // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                            // we add all dimensions as labels

                            prometheus_label_copy(
                                dimension,
                                (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                                PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                generate_as_collected_prom_help(wb, &p, homogeneous, prometheus_collector);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# TYPE %s_%s%s %s\n", prefix, context, suffix, p.type);

                            generate_as_collected_prom_metric(wb, &p, homogeneous, prometheus_collector);
                        } else {
                            // the dimensions of the chart, do not have the same algorithm, multiplier or divisor
                            // we create a metric per dimension

                            prometheus_name_copy(
                                dimension,
                                (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                                PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                generate_as_collected_prom_help(wb, &p, homogeneous, prometheus_collector);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(
                                    wb, "# TYPE %s_%s_%s%s %s\n", prefix, context, dimension, suffix, p.type);

                            generate_as_collected_prom_metric(wb, &p, homogeneous, prometheus_collector);
                        }
                    } else {
                        // we need average or sum of the data

                        time_t first_time = instance->after;
                        time_t last_time = instance->before;
                        NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(instance, rd, &last_time);

                        if (!isnan(value) && !isinf(value)) {
                            if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_AVERAGE)
                                suffix = "_average";
                            else if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_SUM)
                                suffix = "_sum";

                            prometheus_label_copy(
                                dimension,
                                (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                                PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(
                                    wb,
                                    "# COMMENT %s_%s%s%s: dimension \"%s\", value is %s, gauge, dt %llu to %llu inclusive\n",
                                    prefix,
                                    context,
                                    units,
                                    suffix,
                                    (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                                    st->units,
                                    (unsigned long long)first_time,
                                    (unsigned long long)last_time);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# TYPE %s_%s%s%s gauge\n", prefix, context, units, suffix);

                            if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(
                                    wb,
                                    "%s_%s%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " NETDATA_DOUBLE_FORMAT
                                    " %llu\n",
                                    prefix,
                                    context,
                                    units,
                                    suffix,
                                    chart,
                                    family,
                                    dimension,
                                    labels,
                                    value,
                                    last_time * MSEC_PER_SEC);
                            else
                                buffer_sprintf(
                                    wb,
                                    "%s_%s%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " NETDATA_DOUBLE_FORMAT
                                    "\n",
                                    prefix,
                                    context,
                                    units,
                                    suffix,
                                    chart,
                                    family,
                                    dimension,
                                    labels,
                                    value);
                        }
                    }
                }
            }

            rrdset_unlock(st);
        }
    }

    rrdhost_unlock(host);
    simple_pattern_free(filter);
}

/**
 * Get the last time time when a server accessed Netdata. Write information about an API request to a buffer.
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @param wb the buffer to write to.
 * @param exporting_options options to configure what data is exported.
 * @param server the name of a Prometheus server..
 * @param now actual time.
 * @param output_options options to configure the format of the output.
 * @return Returns the last time when the server accessed Netdata.
 */
static inline time_t prometheus_preparation(
    struct instance *instance,
    RRDHOST *host,
    BUFFER *wb,
    EXPORTING_OPTIONS exporting_options,
    const char *server,
    time_t now,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
#ifndef UNIT_TESTING
    analytics_log_prometheus();
#endif
    if (!server || !*server)
        server = "default";

    time_t after = prometheus_server_last_access(server, host, now);

    int first_seen = 0;
    if (!after) {
        after = now - instance->config.update_every;
        first_seen = 1;
    }

    if (after > now) {
        // oops! this should never happen
        after = now - instance->config.update_every;
    }

    if (output_options & PROMETHEUS_OUTPUT_HELP) {
        char *mode;
        if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_AS_COLLECTED)
            mode = "as collected";
        else if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_AVERAGE)
            mode = "average";
        else if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_SUM)
            mode = "sum";
        else
            mode = "unknown";

        buffer_sprintf(
            wb,
            "# COMMENT netdata \"%s\" to %sprometheus \"%s\", source \"%s\", last seen %lu %s, time range %lu to %lu\n\n",
            host->hostname,
            (first_seen) ? "FIRST SEEN " : "",
            server,
            mode,
            (unsigned long)((first_seen) ? 0 : (now - after)),
            (first_seen) ? "never" : "seconds ago",
            (unsigned long)after,
            (unsigned long)now);
    }

    return after;
}

/**
 * Write metrics and auxiliary information for one host to a buffer.
 *
 * @param host a data collecting host.
 * @param filter_string a simple pattern filter.
 * @param wb the buffer to write to.
 * @param server the name of a Prometheus server.
 * @param prefix a prefix for every metric.
 * @param exporting_options options to configure what data is exported.
 * @param output_options options to configure the format of the output.
 */
void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
    RRDHOST *host,
    const char *filter_string,
    BUFFER *wb,
    const char *server,
    const char *prefix,
    EXPORTING_OPTIONS exporting_options,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    if (unlikely(!prometheus_exporter_instance || !prometheus_exporter_instance->config.initialized))
        return;

    prometheus_exporter_instance->before = now_realtime_sec();

    // we start at the point we had stopped before
    prometheus_exporter_instance->after = prometheus_preparation(
        prometheus_exporter_instance,
        host,
        wb,
        exporting_options,
        server,
        prometheus_exporter_instance->before,
        output_options);

    rrd_stats_api_v1_charts_allmetrics_prometheus(
        prometheus_exporter_instance, host, filter_string, wb, prefix, exporting_options, 0, output_options);
}

/**
 * Write metrics and auxiliary information for all hosts to a buffer.
 *
 * @param host a data collecting host.
 * @param filter_string a simple pattern filter.
 * @param wb the buffer to write to.
 * @param server the name of a Prometheus server.
 * @param prefix a prefix for every metric.
 * @param exporting_options options to configure what data is exported.
 * @param output_options options to configure the format of the output.
 */
void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(
    RRDHOST *host,
    const char *filter_string,
    BUFFER *wb,
    const char *server,
    const char *prefix,
    EXPORTING_OPTIONS exporting_options,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    if (unlikely(!prometheus_exporter_instance || !prometheus_exporter_instance->config.initialized))
        return;

    prometheus_exporter_instance->before = now_realtime_sec();

    // we start at the point we had stopped before
    prometheus_exporter_instance->after = prometheus_preparation(
        prometheus_exporter_instance,
        host,
        wb,
        exporting_options,
        server,
        prometheus_exporter_instance->before,
        output_options);

    rrd_rdlock();
    rrdhost_foreach_read(host)
    {
        rrd_stats_api_v1_charts_allmetrics_prometheus(
            prometheus_exporter_instance, host, filter_string, wb, prefix, exporting_options, 1, output_options);
    }
    rrd_unlock();
}
