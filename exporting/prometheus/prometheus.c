// SPDX-License-Identifier: GPL-3.0-or-later

#define EXPORTINGS_INTERNALS
#include "prometheus.h"

// ----------------------------------------------------------------------------
// PROMETHEUS
// /api/v1/allmetrics?format=prometheus and /api/v1/allmetrics?format=prometheus_all_hosts

/**
 * Check if a chart can be sent to an external databese
 *
 * @param instance an instance data structure.
 * @param st a chart.
 * @return Returns 1 if the chart can be sent, 0 otherwise.
 */
inline int can_send_rrdset(struct instance *instance, RRDSET *st)
{
    RRDHOST *host = st->rrdhost;

    if (unlikely(rrdset_flag_check(st, RRDSET_FLAG_BACKEND_IGNORE)))
        return 0;

    if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_BACKEND_SEND))) {
        // we have not checked this chart
        if (simple_pattern_matches(instance->config.charts_pattern, st->id) ||
            simple_pattern_matches(instance->config.charts_pattern, st->name))
            rrdset_flag_set(st, RRDSET_FLAG_BACKEND_SEND);
        else {
            rrdset_flag_set(st, RRDSET_FLAG_BACKEND_IGNORE);
            debug(
                D_BACKEND,
                "EXPORTING: not sending chart '%s' of host '%s', because it is disabled for exporting.",
                st->id,
                host->hostname);
            return 0;
        }
    }

    if (unlikely(!rrdset_is_available_for_backends(st))) {
        debug(
            D_BACKEND,
            "EXPORTING: not sending chart '%s' of host '%s', because it is not available for exporting.",
            st->id,
            host->hostname);
        return 0;
    }

    if (unlikely(
            st->rrd_memory_mode == RRD_MEMORY_MODE_NONE &&
            !(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED))) {
        debug(
            D_BACKEND,
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
 * @param s a source sting.
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
 * @param s a source sting.
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
 * @param s a source sting.
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
void format_host_labels_prometheus(struct instance *instance, RRDHOST *host)
{
    if (unlikely(!sending_labels_configured(instance)))
        return;

    if (!instance->labels)
        instance->labels = buffer_create(1024);

    int count = 0;
    rrdhost_check_rdlock(host);
    netdata_rwlock_rdlock(&host->labels_rwlock);
    for (struct label *label = host->labels; label; label = label->next) {
        if (!should_send_label(instance, label))
            continue;

        char key[PROMETHEUS_ELEMENT_MAX + 1];
        char value[PROMETHEUS_ELEMENT_MAX + 1];

        prometheus_name_copy(key, label->key, PROMETHEUS_ELEMENT_MAX);
        prometheus_label_copy(value, label->value, PROMETHEUS_ELEMENT_MAX);

        if (*key && *value) {
            if (count > 0)
                buffer_strcat(instance->labels, ",");
            buffer_sprintf(instance->labels, "%s=\"%s\"", key, value);
            count++;
        }
    }
    netdata_rwlock_unlock(&host->labels_rwlock);
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

        calculated_number value = rrdvar2number(rv);
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
                "%s_%s%s%s%s " CALCULATED_NUMBER_FORMAT " %llu\n",
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
                "%s_%s%s%s%s " CALCULATED_NUMBER_FORMAT "\n",
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

/**
 * Write metrics in Prometheus format to a buffer.
 *
 * @param instance an instance data structure.
 * @param host a data collecting host.
 * @param wb the buffer to fill with metrics.
 * @param prefix a prefix for every metric.
 * @param exporting_options options to configure what data is exported.
 * @param allhosts set to 1 if host instance should be in the output for tags.
 * @param output_options options to configure the format of the output.
 */
static void rrd_stats_api_v1_charts_allmetrics_prometheus(
    struct instance *instance,
    RRDHOST *host,
    BUFFER *wb,
    const char *prefix,
    EXPORTING_OPTIONS exporting_options,
    int allhosts,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    rrdhost_rdlock(host);

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, host->hostname, PROMETHEUS_ELEMENT_MAX);

    format_host_labels_prometheus(instance, host);

    if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
        buffer_sprintf(
            wb,
            "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1 %llu\n",
            hostname,
            host->program_name,
            host->program_version,
            now_realtime_usec() / USEC_PER_MS);
    else
        buffer_sprintf(
            wb,
            "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1\n",
            hostname,
            host->program_name,
            host->program_version);

    char labels[PROMETHEUS_LABELS_MAX + 1] = "";
    if (allhosts) {
        if (instance->labels && buffer_tostring(instance->labels)) {
            if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS) {
                buffer_sprintf(
                    wb,
                    "netdata_host_tags_info{instance=\"%s\",%s} 1 %llu\n",
                    hostname,
                    buffer_tostring(instance->labels),
                    now_realtime_usec() / USEC_PER_MS);

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(
                    wb,
                    "netdata_host_tags{instance=\"%s\",%s} 1 %llu\n",
                    hostname,
                    buffer_tostring(instance->labels),
                    now_realtime_usec() / USEC_PER_MS);
            } else {
                buffer_sprintf(
                    wb, "netdata_host_tags_info{instance=\"%s\",%s} 1\n", hostname, buffer_tostring(instance->labels));

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(
                    wb, "netdata_host_tags{instance=\"%s\",%s} 1\n", hostname, buffer_tostring(instance->labels));
            }
        }

        snprintfz(labels, PROMETHEUS_LABELS_MAX, ",instance=\"%s\"", hostname);
    } else {
        if (instance->labels && buffer_tostring(instance->labels)) {
            if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS) {
                buffer_sprintf(
                    wb,
                    "netdata_host_tags_info{%s} 1 %llu\n",
                    buffer_tostring(instance->labels),
                    now_realtime_usec() / USEC_PER_MS);

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(
                    wb,
                    "netdata_host_tags{%s} 1 %llu\n",
                    buffer_tostring(instance->labels),
                    now_realtime_usec() / USEC_PER_MS);
            } else {
                buffer_sprintf(wb, "netdata_host_tags_info{%s} 1\n", buffer_tostring(instance->labels));

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(wb, "netdata_host_tags{%s} 1\n", buffer_tostring(instance->labels));
            }
        }
    }

    if (instance->labels)
        buffer_flush(instance->labels);

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

        if (likely(can_send_rrdset(instance, st))) {
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
            if (as_collected) {
                if (rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK))
                    rrdset_update_heterogeneous_flag(st);

                if (rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
                    homogeneous = 0;
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

                        if (unlikely(rd->last_collected_time.tv_sec < instance->after))
                            continue;

                        const char *t = "gauge", *h = "gives";
                        if (rd->algorithm == RRD_ALGORITHM_INCREMENTAL ||
                            rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL) {
                            t = "counter";
                            h = "delta gives";
                            suffix = "_total";
                        }

                        if (homogeneous) {
                            // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                            // we add all dimensions as labels

                            prometheus_label_copy(
                                dimension,
                                (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id,
                                PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(
                                    wb,
                                    "# COMMENT %s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT
                                    " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n",
                                    prefix,
                                    context,
                                    suffix,
                                    (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id,
                                    st->context,
                                    st->family,
                                    (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id,
                                    rd->multiplier,
                                    rd->divisor,
                                    h,
                                    st->units,
                                    t);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# TYPE %s_%s%s %s\n", prefix, context, suffix, t);

                            if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(
                                    wb,
                                    "%s_%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " COLLECTED_NUMBER_FORMAT
                                    " %llu\n",
                                    prefix,
                                    context,
                                    suffix,
                                    chart,
                                    family,
                                    dimension,
                                    labels,
                                    rd->last_collected_value,
                                    timeval_msec(&rd->last_collected_time));
                            else
                                buffer_sprintf(
                                    wb,
                                    "%s_%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " COLLECTED_NUMBER_FORMAT
                                    "\n",
                                    prefix,
                                    context,
                                    suffix,
                                    chart,
                                    family,
                                    dimension,
                                    labels,
                                    rd->last_collected_value);
                        } else {
                            // the dimensions of the chart, do not have the same algorithm, multiplier or divisor
                            // we create a metric per dimension

                            prometheus_name_copy(
                                dimension,
                                (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id,
                                PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(
                                    wb,
                                    "# COMMENT %s_%s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT
                                    " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n",
                                    prefix,
                                    context,
                                    dimension,
                                    suffix,
                                    (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id,
                                    st->context,
                                    st->family,
                                    (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id,
                                    rd->multiplier,
                                    rd->divisor,
                                    h,
                                    st->units,
                                    t);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(
                                    wb, "# TYPE %s_%s_%s%s %s\n", prefix, context, dimension, suffix, t);

                            if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(
                                    wb,
                                    "%s_%s_%s%s{chart=\"%s\",family=\"%s\"%s} " COLLECTED_NUMBER_FORMAT " %llu\n",
                                    prefix,
                                    context,
                                    dimension,
                                    suffix,
                                    chart,
                                    family,
                                    labels,
                                    rd->last_collected_value,
                                    timeval_msec(&rd->last_collected_time));
                            else
                                buffer_sprintf(
                                    wb,
                                    "%s_%s_%s%s{chart=\"%s\",family=\"%s\"%s} " COLLECTED_NUMBER_FORMAT "\n",
                                    prefix,
                                    context,
                                    dimension,
                                    suffix,
                                    chart,
                                    family,
                                    labels,
                                    rd->last_collected_value);
                        }
                    } else {
                        // we need average or sum of the data

                        time_t first_time = instance->after;
                        time_t last_time = instance->before;
                        calculated_number value = exporting_calculate_value_from_stored_data(instance, rd, &last_time);

                        if (!isnan(value) && !isinf(value)) {
                            if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_AVERAGE)
                                suffix = "_average";
                            else if (EXPORTING_OPTIONS_DATA_SOURCE(exporting_options) == EXPORTING_SOURCE_DATA_SUM)
                                suffix = "_sum";

                            prometheus_label_copy(
                                dimension,
                                (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id,
                                PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(
                                    wb,
                                    "# COMMENT %s_%s%s%s: dimension \"%s\", value is %s, gauge, dt %llu to %llu inclusive\n",
                                    prefix,
                                    context,
                                    units,
                                    suffix,
                                    (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id,
                                    st->units,
                                    (unsigned long long)first_time,
                                    (unsigned long long)last_time);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# TYPE %s_%s%s%s gauge\n", prefix, context, units, suffix);

                            if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(
                                    wb,
                                    "%s_%s%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " CALCULATED_NUMBER_FORMAT
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
                                    "%s_%s%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " CALCULATED_NUMBER_FORMAT
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
 * @param wb the buffer to write to.
 * @param server the name of a Prometheus server.
 * @param prefix a prefix for every metric.
 * @param exporting_options options to configure what data is exported.
 * @param output_options options to configure the format of the output.
 */
void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(
    RRDHOST *host,
    BUFFER *wb,
    const char *server,
    const char *prefix,
    EXPORTING_OPTIONS exporting_options,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    if (unlikely(!prometheus_exporter_instance))
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
        prometheus_exporter_instance, host, wb, prefix, exporting_options, 0, output_options);
}

/**
 * Write metrics and auxiliary information for all hosts to a buffer.
 *
 * @param host a data collecting host.
 * @param wb the buffer to write to.
 * @param server the name of a Prometheus server.
 * @param prefix a prefix for every metric.
 * @param exporting_options options to configure what data is exported.
 * @param output_options options to configure the format of the output.
 */
void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(
    RRDHOST *host,
    BUFFER *wb,
    const char *server,
    const char *prefix,
    EXPORTING_OPTIONS exporting_options,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    if (unlikely(!prometheus_exporter_instance))
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
            prometheus_exporter_instance, host, wb, prefix, exporting_options, 1, output_options);
    }
    rrd_unlock();
}
