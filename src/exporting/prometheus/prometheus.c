// SPDX-License-Identifier: GPL-3.0-or-later

#include "prometheus.h"

DEFINE_JUDYL_TYPED(PROM_CONTEXT_OPTIONS, PROMETHEUS_OUTPUT_OPTIONS);

static void PROM_CONTEXT_OPTIONS_free_cb(Word_t index, PROMETHEUS_OUTPUT_OPTIONS options __maybe_unused, void *data __maybe_unused) {
    STRING *context_id = (STRING *)index;
    string_freez(context_id);
}

// ----------------------------------------------------------------------------
// PROMETHEUS
// /api/v1/allmetrics?format=prometheus and /api/v1/allmetrics?format=prometheus_all_hosts

static int is_matches_rrdset(struct instance *instance, RRDSET *st, SIMPLE_PATTERN *filter) {
    if (instance->config.options & EXPORTING_OPTION_SEND_NAMES) {
        return simple_pattern_matches_string(filter, st->name);
    }
    return simple_pattern_matches_string(filter, st->id);
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
            netdata_log_debug(
                D_EXPORTING,
                "EXPORTING: not sending chart '%s' of host '%s', because it is disabled for exporting.",
                rrdset_id(st),
                rrdhost_hostname(host));
            return 0;
        }
    }

    if (unlikely(!rrdset_is_available_for_exporting_and_alarms(st))) {
        netdata_log_debug(
            D_EXPORTING,
            "EXPORTING: not sending chart '%s' of host '%s', because it is not available for exporting.",
            rrdset_id(st),
            rrdhost_hostname(host));
        return 0;
    }

    if (unlikely(
            st->rrd_memory_mode == RRD_DB_MODE_NONE &&
            !(EXPORTING_OPTIONS_DATA_SOURCE(instance->config.options) == EXPORTING_SOURCE_DATA_AS_COLLECTED))) {
        netdata_log_debug(
            D_EXPORTING,
            "EXPORTING: not sending chart '%s' of host '%s' because its memory mode is '%s' and the exporting connector requires database access.",
            rrdset_id(st),
            rrdhost_hostname(host),
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
    netdata_mutex_lock(&prometheus_server_root_mutex);
    if (prometheus_server_root) {
        struct prometheus_server *ps;
        for (ps = prometheus_server_root; ps; ) {
            struct prometheus_server *current = ps;
            ps = ps->next;
            if(current->server)
                freez((void *)current->server);

            freez(current);
        }
        prometheus_server_root = NULL;
    }
    netdata_mutex_unlock(&prometheus_server_root_mutex);
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
 * @param size the number of characters to copy.
 * @return Returns the length of the copied string.
 */
inline void prometheus_name_copy(char *d, const char *s, size_t size) {
    prometheus_rrdlabels_sanitize_name(d, s, size);
}

/**
 * Copy and sanitize label.
 *
 * @param d a destination string.
 * @param s a source string.
 * @param size the number of characters to copy.
 * @return Returns the length of the copied string.
 */
inline void prometheus_label_copy(char *d, const char *s, size_t size) {
    // our label values are already compatible with prometheus label values
    // so, just copy them
    strncpyz(d, s, size - 1);
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
                // netdata_log_info("matched extension for filename '%s': '%s'", filename, last_dot);
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

static int format_prometheus_label_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct format_prometheus_label_callback *d = (struct format_prometheus_label_callback *)data;

    if (!should_send_label(d->instance, ls)) return 0;

    char k[PROMETHEUS_ELEMENT_MAX + 1];
    char v[PROMETHEUS_ELEMENT_MAX + 1];

    prometheus_name_copy(k, name, sizeof(k));
    prometheus_label_copy(v, value, sizeof(v));

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
        instance->labels_buffer = buffer_create(1024, &netdata_buffers_statistics.buffers_exporters);

    struct format_prometheus_label_callback tmp = {
        .instance = instance,
        .count = 0
    };
    rrdlabels_walkthrough_read(host->rrdlabels, format_prometheus_label_callback, &tmp);
}

/**
 * Format host labels for the Prometheus exporter
 * We are using a structure instead a direct buffer to expand options quickly.
 *
 * @param data is the buffer used to add labels.
 */

static int format_prometheus_chart_label_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    BUFFER *wb = data;

    if (name[0] == '_' )
        return 1;

    char k[PROMETHEUS_ELEMENT_MAX + 1];
    char v[PROMETHEUS_ELEMENT_MAX + 1];

    prometheus_name_copy(k, name, sizeof(k));
    prometheus_label_copy(v, value, sizeof(v));

    if (*k && *v)
        buffer_sprintf(wb, ",%s=\"%s\"", k, v);

    return 1;
}

struct host_variables_callback_options {
    RRDHOST *host;
    BUFFER *wb;
    BUFFER *plabels_buffer;
    EXPORTING_OPTIONS exporting_options;
    PROMETHEUS_OUTPUT_OPTIONS output_options;
    const char *prefix;
    const char *labels;
    time_t now;
    int host_header_printed;
    char name[PROMETHEUS_VARIABLE_MAX + 1];
    SIMPLE_PATTERN *pattern;
    struct instance *instance;
    STRING *prometheus;
    PROM_CONTEXT_OPTIONS_JudyLSet *context_options;
};

/**
 * Print host variables.
 *
 * @param rv a variable.
 * @param data callback options.
 * @return Returns 1 if the chart can be sent, 0 otherwise.
 */
static int print_host_variables_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rv_ptr __maybe_unused, void *data) {
    const RRDVAR_ACQUIRED *rv = (const RRDVAR_ACQUIRED *)item;

    struct host_variables_callback_options *opts = data;

    if (!opts->host_header_printed) {
        opts->host_header_printed = 1;
    }

    NETDATA_DOUBLE value = rrdvar2number(rv);
    if (isnan(value) || isinf(value)) {
        return 0;
    }

    char *label_pre = "";
    char *label_post = "";
    if (opts->labels && *opts->labels) {
        label_pre = "{";
        label_post = "}";
    }

    prometheus_name_copy(opts->name, rrdvar_name(rv), sizeof(opts->name));

    if (opts->output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
        buffer_sprintf(
            opts->wb,
            "%s_%s%s%s%s " NETDATA_DOUBLE_FORMAT " %llu\n",
            opts->prefix,
            opts->name,
            label_pre,
            (opts->labels[0] == ',') ? &opts->labels[1] : opts->labels,
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
            (opts->labels[0] == ',') ? &opts->labels[1] : opts->labels,
            label_post,
            value);

    return 1;
}

struct gen_parameters {
    const char *prefix;
    const char *labels_prefix;
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
 * @param context context name we are using
 */
static inline void generate_as_collected_prom_help(BUFFER *wb,
                                                   const char *prefix,
                                                   char *context,
                                                   char *units,
                                                   char *suffix,
                                                   RRDSET *st)
{
    buffer_sprintf(wb, "# HELP %s_%s%s%s %s\n", prefix, context, units, suffix, rrdset_title(st));
}

/**
 * Write an as-collected help comment to a buffer.
 *
 * @param wb the buffer to write the comment to.
 * @param context context name we are using
 */
static inline void generate_as_collected_prom_type(BUFFER *wb,
                                                   const char *prefix,
                                                   char *context,
                                                   char *units,
                                                   char *suffix,
                                                   const char *type)
{
    buffer_sprintf(wb, "# TYPE %s_%s%s%s %s\n", prefix, context, units, suffix, type);
}

/**
 * Write an as-collected metric to a buffer.
 *
 * @param wb the buffer to write the metric to.
 * @param p parameters for generating the metric string.
 * @param homogeneous a flag for homogeneous charts.
 * @param prometheus_collector a flag for metrics from prometheus collector.
 * @param chart_labels the dictionary with chart labels
 */
static void generate_as_collected_from_metric(BUFFER *wb,
                                              struct gen_parameters *p,
                                              int homogeneous,
                                              int prometheus_collector,
                                              RRDLABELS *chart_labels)
{
    buffer_strcat(wb, p->prefix);
    buffer_putc(wb, '_');
    buffer_strcat(wb, p->context);

    if (!homogeneous) {
        buffer_putc(wb, '_');
        buffer_strcat(wb, p->dimension);
    }

    buffer_sprintf(wb, "%s{%schart=\"%s\"", p->suffix, p->labels_prefix, p->chart);

    if (homogeneous)
        buffer_sprintf(wb, ",%sdimension=\"%s\"", p->labels_prefix, p->dimension);

    buffer_sprintf(wb, ",%sfamily=\"%s\"", p->labels_prefix, p->family);

    rrdlabels_walkthrough_read(chart_labels, format_prometheus_chart_label_callback, wb);

    buffer_strcat(wb, p->labels);
    buffer_putc(wb, '}');
    buffer_putc(wb, ' ');

    if (prometheus_collector)
        buffer_print_netdata_double(wb,
            (NETDATA_DOUBLE)p->rd->collector.last_collected_value * (NETDATA_DOUBLE)p->rd->multiplier /
            (NETDATA_DOUBLE)p->rd->divisor);
    else
        buffer_print_int64(wb, p->rd->collector.last_collected_value);

    if (p->output_options & PROMETHEUS_OUTPUT_TIMESTAMPS) {
        buffer_putc(wb, ' ');
        buffer_print_uint64(wb, timeval_msec(&p->rd->collector.last_collected_time));
    }

    buffer_putc(wb, '\n');
}

static void prometheus_print_os_info(
    BUFFER *wb,
    RRDHOST *host,
    PROMETHEUS_OUTPUT_OPTIONS output_options)
{
    FILE *fp;
    char filename[FILENAME_MAX + 1];
    char buf[BUFSIZ + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/etc/os-release");
    fp = fopen(filename, "r");
    if (!fp) {
        /* Fallback to lsb-release */
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/etc/lsb-release");
        fp = fopen(filename, "r");
    }
    if (!fp) {
        return;
    }

    buffer_sprintf(wb, "netdata_os_info{instance=\"%s\"", rrdhost_hostname(host));

    while (fgets(buf, BUFSIZ, fp)) {
      char *in, *sanitized;
      char *key, *val;
      int in_val_part = 0;

      /* sanitize the line */
      sanitized = in = buf;
      in_val_part = 0;
      while (*in && *in != '\n') {
          if (!in_val_part) {
              /* Only accepts alphabetic characters and '_'
               * in key part */
              if (isalpha((uint8_t)*in) || *in == '_') {
                  *(sanitized++) = tolower((uint8_t)*in);
              } else if (*in == '=') {
                  in_val_part = 1;
                  *(sanitized++) = '=';
              }
          } else {
              /* Don't accept special characters in
               * value part */
              switch (*in) {
                  case '"':
                  case '\'':
                  case '\r':
                  case '\t':
                      break;
                  default:
                      if (isprint((uint8_t)*in)) {
                          *(sanitized++) = *in;
                      }
              }
          }
          in++;
      }
      /* Terminate the string */
      *(sanitized++) = '\0';

      /* Split key/val */
      key = buf;
      val = strchr(buf, '=');

      /* If we have a key/value pair, add it as a label */
      if (val) {
          *val = '\0';
          val++;
          buffer_sprintf(wb, ",%s=\"%s\"", key, val);
      }
    }

    /* Finish the line */
    if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
        buffer_sprintf(wb, "} 1 %llu\n", now_realtime_usec() / USEC_PER_MS);
    else
        buffer_sprintf(wb, "} 1\n");

    fclose(fp);
}

/**
 * RRDSET to JSON
 *
 * From RRDSET extract content necessary to write JSON output.
 *
 * @param st   netdata chart structure
 * @param data structure with necessary data and to build expected result.
 *
 * @return I returns 1 when content was used and 0 otherwise.
 */
static int prometheus_rrdset_to_json(RRDSET *st, void *data)
{
    struct host_variables_callback_options *opts = data;

    if (likely(can_send_rrdset(opts->instance, st, opts->pattern))) {
        PROMETHEUS_OUTPUT_OPTIONS output_options = opts->output_options;
        BUFFER *wb = opts->wb;
        const char *prefix = opts->prefix;

        BUFFER *plabels_buffer = opts->plabels_buffer;
        const char *plabels_prefix = opts->instance->config.label_prefix;

        STRING *prometheus = opts->prometheus;

        char chart[PROMETHEUS_ELEMENT_MAX + 1];
        char context[PROMETHEUS_ELEMENT_MAX + 1];
        char family[PROMETHEUS_ELEMENT_MAX + 1];
        char units[PROMETHEUS_ELEMENT_MAX + 1] = "";

        prometheus_label_copy(chart,
                              (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ?
                               rrdset_name(st) : rrdset_id(st), sizeof(chart));
        prometheus_label_copy(family, rrdset_family(st), sizeof(family));
        prometheus_name_copy(context, rrdset_context(st), sizeof(context));

        if(opts->output_options & PROMETHEUS_OUTPUT_HELP_TYPE) {
            // we do not want to print HELP and TYPE for the same context twice
            STRING *context_id = string_strdupz(context);
            PROMETHEUS_OUTPUT_OPTIONS ctx_opts = PROM_CONTEXT_OPTIONS_GET(opts->context_options, (Word_t)context_id);
            if (!(ctx_opts & PROMETHEUS_OUTPUT_HELP_TYPE)) {
                // it is not printed for this context yet
                ctx_opts = opts->output_options;
                PROM_CONTEXT_OPTIONS_SET(opts->context_options, (Word_t)context_id, ctx_opts);
            }
            else {
                // we have printed HELP and TYPE for this context already
                opts->output_options &= ~PROMETHEUS_OUTPUT_HELP_TYPE;
                string_freez(context_id);
            }
        }

        int as_collected = (EXPORTING_OPTIONS_DATA_SOURCE(opts->exporting_options)
                            == EXPORTING_SOURCE_DATA_AS_COLLECTED);
        int homogeneous = 1;
        int prometheus_collector = 0;
        RRDSET_FLAGS flags = rrdset_flag_get(st);
        if (as_collected) {
            if (flags & RRDSET_FLAG_HOMOGENEOUS_CHECK)
                rrdset_update_heterogeneous_flag(st);

            if (flags & RRDSET_FLAG_HETEROGENEOUS)
                homogeneous = 0;

            if (st->module_name == prometheus)
                prometheus_collector = 1;
        }
        else {
            if (EXPORTING_OPTIONS_DATA_SOURCE(opts->exporting_options) == EXPORTING_SOURCE_DATA_AVERAGE &&
                !(output_options & PROMETHEUS_OUTPUT_HIDEUNITS))
                prometheus_units_copy(units,
                                      rrdset_units(st),
                                      PROMETHEUS_ELEMENT_MAX,
                                      output_options & PROMETHEUS_OUTPUT_OLDUNITS);
        }

        // for each dimension
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {

            if (rd->collector.counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                char dimension[PROMETHEUS_ELEMENT_MAX + 1];
                char *suffix = "";

                struct gen_parameters p;
                p.prefix = prefix;
                p.labels_prefix = plabels_prefix;
                p.context = context;
                p.suffix = suffix;
                p.chart = chart;
                p.dimension = dimension;
                p.family = family;
                p.labels = (char *)opts->labels;
                p.output_options = output_options;
                p.st = st;
                p.rd = rd;

                if (as_collected) {
                    // we need as-collected / raw data

                    if (unlikely(rd->collector.last_collected_time.tv_sec < opts->instance->after))
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

                    if (opts->output_options & PROMETHEUS_OUTPUT_HELP_TYPE) {
                        generate_as_collected_prom_help(wb, prefix, context, units, p.suffix, st);
                        generate_as_collected_prom_type(wb, prefix, context, units, p.suffix, p.type);
                        opts->output_options &= ~PROMETHEUS_OUTPUT_HELP_TYPE;
                    }

                    if (homogeneous) {
                        // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                        // we add all dimensions as labels

                        prometheus_label_copy(
                            dimension,
                            (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                            sizeof(dimension));
                    }
                    else {
                        // the dimensions of the chart, do not have the same algorithm, multiplier or divisor
                        // we create a metric per dimension

                        prometheus_name_copy(
                            dimension,
                            (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                            sizeof(dimension));
                    }
                    generate_as_collected_from_metric(wb, &p, homogeneous, prometheus_collector, st->rrdlabels);
                }
                else {
                    // we need average or sum of the data

                    time_t last_time = opts->instance->before;
                    NETDATA_DOUBLE value = exporting_calculate_value_from_stored_data(opts->instance, rd, &last_time);

                    if (!isnan(value) && !isinf(value)) {
                        if (EXPORTING_OPTIONS_DATA_SOURCE(opts->exporting_options) == EXPORTING_SOURCE_DATA_AVERAGE)
                            suffix = "_average";
                        else if (EXPORTING_OPTIONS_DATA_SOURCE(opts->exporting_options)
                                 == EXPORTING_SOURCE_DATA_SUM)
                            suffix = "_sum";

                        prometheus_label_copy(
                            dimension,
                            (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rrddim_name(rd) : rrddim_id(rd),
                            sizeof(dimension));

                        if (opts->output_options & PROMETHEUS_OUTPUT_HELP_TYPE) {
                            generate_as_collected_prom_help(wb, prefix, context, units, suffix, st);
                            generate_as_collected_prom_type(wb, prefix, context, units, suffix, "gauge");
                            opts->output_options &= ~PROMETHEUS_OUTPUT_HELP_TYPE;
                        }

                        buffer_flush(plabels_buffer);
                        buffer_sprintf(plabels_buffer,
                                       "%1$schart=\"%2$s\",%1$sdimension=\"%3$s\",%1$sfamily=\"%4$s\"",
                                       plabels_prefix,
                                       chart,
                                       dimension,
                                       family);
                        rrdlabels_walkthrough_read(st->rrdlabels,
                                                   format_prometheus_chart_label_callback,
                                                   plabels_buffer);

                        if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                            buffer_sprintf(wb,
                                           "%s_%s%s%s{%s%s} " NETDATA_DOUBLE_FORMAT " %llu\n",
                                           prefix,
                                           context,
                                           units,
                                           suffix,
                                           buffer_tostring(plabels_buffer),
                                           opts->labels,
                                           value,
                                           last_time * MSEC_PER_SEC);
                        else
                            buffer_sprintf(wb, "%s_%s%s%s{%s%s} " NETDATA_DOUBLE_FORMAT "\n",
                                           prefix,
                                           context,
                                           units,
                                           suffix,
                                           buffer_tostring(plabels_buffer),
                                           opts->labels,
                                           value);
                    }
                }
            }
        }
        rrddim_foreach_done(rd);

        return 1;
    }

    return 0;
}

/**
 * RRDCONTEXT callback
 *
 * Callback used to parse dictionary
 *
 * @param item  the dictionary structure
 * @param value unused element
 * @param data  structure used to store data.
 *
 * @return It always returns HTTP_RESP_OK
 */
static inline int prometheus_rrdcontext_callback(const DICTIONARY_ITEM *item, void *value, void *data)
{
    const char *context_name = dictionary_acquired_item_name(item);
    struct host_variables_callback_options *opts = data;
    (void)value;

    opts->output_options |= PROMETHEUS_OUTPUT_HELP_TYPE;
    (void)rrdcontext_foreach_instance_with_rrdset_in_context(opts->host, context_name, prometheus_rrdset_to_json, data);

    return HTTP_RESP_OK;
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
    PROMETHEUS_OUTPUT_OPTIONS output_options,
    PROM_CONTEXT_OPTIONS_JudyLSet *context_options)
{
    SIMPLE_PATTERN *filter = simple_pattern_create(filter_string, NULL, SIMPLE_PATTERN_EXACT, true);

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, rrdhost_hostname(host), sizeof(hostname));

    format_host_labels_prometheus(instance, host);

    buffer_sprintf(
        wb,
        "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"",
        hostname,
        rrdhost_program_name(host),
        rrdhost_program_version(host));

    if (instance->labels_buffer && *buffer_tostring(instance->labels_buffer)) {
        buffer_sprintf(wb, ",%s", buffer_tostring(instance->labels_buffer));
    }

    if (output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
        buffer_sprintf(wb, "} 1 %llu\n", now_realtime_usec() / USEC_PER_MS);
    else
        buffer_sprintf(wb, "} 1\n");

    char labels[PROMETHEUS_LABELS_MAX + 1] = "";
    if (allhosts) {
        snprintfz(labels, PROMETHEUS_LABELS_MAX, ",%sinstance=\"%s\"", instance->config.label_prefix, hostname);
     }

    if (instance->labels_buffer)
        buffer_flush(instance->labels_buffer);

    if (instance->config.options & EXPORTING_OPTION_SEND_AUTOMATIC_LABELS)
        prometheus_print_os_info(wb, host, output_options);


    BUFFER *plabels_buffer = buffer_create(0, NULL);

    struct host_variables_callback_options opts = {
        .host = host,
        .wb = wb,
        .plabels_buffer = plabels_buffer,
        .labels = labels, // FIX: very misleading name and poor implementation of adding the "instance" label
        .exporting_options = exporting_options,
        .output_options = output_options,
        .prefix = prefix,
        .now = now_realtime_sec(),
        .host_header_printed = 0,
        .pattern = filter,
        .instance = instance,
        .prometheus = string_strdupz("prometheus"),
        .context_options = context_options,
    };

    // send custom variables set for the host
    if (output_options & PROMETHEUS_OUTPUT_VARIABLES) {
        rrdvar_walkthrough_read(host->rrdvars, print_host_variables_callback, &opts);
    }

    // for each context
    if (!host->rrdctx.contexts) {
        netdata_log_error("%s(): request for host '%s' that does not have rrdcontexts initialized.", __FUNCTION__, rrdhost_hostname(host));
        goto allmetrics_cleanup;
    }

    dictionary_walkthrough_read(host->rrdctx.contexts, prometheus_rrdcontext_callback, &opts);

allmetrics_cleanup:
    simple_pattern_free(filter);
    buffer_free(plabels_buffer);
    string_freez(opts.prometheus);
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
    const char *server,
    time_t now)
{
#ifndef UNIT_TESTING
    analytics_log_prometheus();
#endif
    if (!server || !*server)
        server = "default";

    time_t after = prometheus_server_last_access(server, host, now);

    if (!after) {
        after = now - instance->config.update_every;
    }

    if (after > now) {
        // oops! this should never happen
        after = now - instance->config.update_every;
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
        server,
        prometheus_exporter_instance->before);

    PROM_CONTEXT_OPTIONS_JudyLSet context_options;
    PROM_CONTEXT_OPTIONS_INIT(&context_options);

    rrd_stats_api_v1_charts_allmetrics_prometheus(
        prometheus_exporter_instance, host, filter_string, wb, prefix, exporting_options, 0, output_options, &context_options);

    PROM_CONTEXT_OPTIONS_FREE(&context_options, PROM_CONTEXT_OPTIONS_free_cb, NULL);
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
        server,
        prometheus_exporter_instance->before);

    PROM_CONTEXT_OPTIONS_JudyLSet context_options;
    PROM_CONTEXT_OPTIONS_INIT(&context_options);

    dfe_start_reentrant(rrdhost_root_index, host)
    {
        rrd_stats_api_v1_charts_allmetrics_prometheus(
            prometheus_exporter_instance, host, filter_string, wb, prefix, exporting_options, 1, output_options, &context_options);
    }
    dfe_done(host);

    PROM_CONTEXT_OPTIONS_FREE(&context_options, PROM_CONTEXT_OPTIONS_free_cb, NULL);
}
