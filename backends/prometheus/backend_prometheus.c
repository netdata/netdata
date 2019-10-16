// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "backend_prometheus.h"

// ----------------------------------------------------------------------------
// PROMETHEUS
// /api/v1/allmetrics?format=prometheus and /api/v1/allmetrics?format=prometheus_all_hosts

static struct prometheus_server {
    const char *server;
    uint32_t hash;
    RRDHOST *host;
    time_t last_access;
    struct prometheus_server *next;
} *prometheus_server_root = NULL;

static inline time_t prometheus_server_last_access(const char *server, RRDHOST *host, time_t now) {
    static netdata_mutex_t prometheus_server_root_mutex = NETDATA_MUTEX_INITIALIZER;

    uint32_t hash = simple_hash(server);

    netdata_mutex_lock(&prometheus_server_root_mutex);

    struct prometheus_server *ps;
    for(ps = prometheus_server_root; ps ;ps = ps->next) {
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

static inline size_t prometheus_name_copy(char *d, const char *s, size_t usable) {
    size_t n;

    for(n = 0; *s && n < usable ; d++, s++, n++) {
        register char c = *s;

        if(!isalnum(c)) *d = '_';
        else *d = c;
    }
    *d = '\0';

    return n;
}

static inline size_t prometheus_label_copy(char *d, const char *s, size_t usable) {
    size_t n;

    // make sure we can escape one character without overflowing the buffer
    usable--;

    for(n = 0; *s && n < usable ; d++, s++, n++) {
        register char c = *s;

        if(unlikely(c == '"' || c == '\\' || c == '\n')) {
            *d++ = '\\';
            n++;
        }
        *d = c;
    }
    *d = '\0';

    return n;
}

static inline char *prometheus_units_copy(char *d, const char *s, size_t usable, int showoldunits) {
    const char *sorig = s;
    char *ret = d;
    size_t n;

    // Fix for issue 5227
    if (unlikely(showoldunits)) {
		static struct {
			const char *newunit;
			uint32_t hash;
			const char *oldunit;
		} units[] = {
				  {"KiB/s", 0, "kilobytes/s"}
				, {"MiB/s", 0, "MB/s"}
				, {"GiB/s", 0, "GB/s"}
				, {"KiB"  , 0, "KB"}
				, {"MiB"  , 0, "MB"}
				, {"GiB"  , 0, "GB"}
				, {"inodes"       , 0, "Inodes"}
				, {"percentage"   , 0, "percent"}
				, {"faults/s"     , 0, "page faults/s"}
				, {"KiB/operation", 0, "kilobytes per operation"}
				, {"milliseconds/operation", 0, "ms per operation"}
				, {NULL, 0, NULL}
		};
		static int initialized = 0;
		int i;

		if(unlikely(!initialized)) {
			for (i = 0; units[i].newunit; i++)
				units[i].hash = simple_hash(units[i].newunit);
			initialized = 1;
		}

		uint32_t hash = simple_hash(s);
		for(i = 0; units[i].newunit ; i++) {
			if(unlikely(hash == units[i].hash && !strcmp(s, units[i].newunit))) {
				// info("matched extension for filename '%s': '%s'", filename, last_dot);
				s=units[i].oldunit;
				sorig = s;
				break;
			}
		}
    }
    *d++ = '_';
    for(n = 1; *s && n < usable ; d++, s++, n++) {
        register char c = *s;

        if(!isalnum(c)) *d = '_';
        else *d = c;
    }

    if(n == 2 && sorig[0] == '%') {
        n = 0;
        d = ret;
        s = "_percent";
        for( ; *s && n < usable ; n++) *d++ = *s++;
    }
    else if(n > 3 && sorig[n-3] == '/' && sorig[n-2] == 's') {
        n = n - 2;
        d -= 2;
        s = "_persec";
        for( ; *s && n < usable ; n++) *d++ = *s++;
    }

    *d = '\0';

    return ret;
}


#define PROMETHEUS_ELEMENT_MAX 256
#define PROMETHEUS_LABELS_MAX 1024
#define PROMETHEUS_VARIABLE_MAX 256

#define PROMETHEUS_LABELS_MAX_NUMBER 128

struct host_variables_callback_options {
    RRDHOST *host;
    BUFFER *wb;
    BACKEND_OPTIONS backend_options;
    PROMETHEUS_OUTPUT_OPTIONS output_options;
    const char *prefix;
    const char *labels;
    time_t now;
    int host_header_printed;
    char name[PROMETHEUS_VARIABLE_MAX+1];
};

static int print_host_variables(RRDVAR *rv, void *data) {
    struct host_variables_callback_options *opts = data;

    if(rv->options & (RRDVAR_OPTION_CUSTOM_HOST_VAR|RRDVAR_OPTION_CUSTOM_CHART_VAR)) {
        if(!opts->host_header_printed) {
            opts->host_header_printed = 1;

            if(opts->output_options & PROMETHEUS_OUTPUT_HELP) {
                buffer_sprintf(opts->wb, "\n# COMMENT global host and chart variables\n");
            }
        }

        calculated_number value = rrdvar2number(rv);
        if(isnan(value) || isinf(value)) {
            if(opts->output_options & PROMETHEUS_OUTPUT_HELP)
                buffer_sprintf(opts->wb, "# COMMENT variable \"%s\" is %s. Skipped.\n", rv->name, (isnan(value))?"NAN":"INF");

            return 0;
        }

        char *label_pre = "";
        char *label_post = "";
        if(opts->labels && *opts->labels) {
            label_pre = "{";
            label_post = "}";
        }

        prometheus_name_copy(opts->name, rv->name, sizeof(opts->name));

        if(opts->output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
            buffer_sprintf(opts->wb
                           , "%s_%s%s%s%s " CALCULATED_NUMBER_FORMAT " %llu\n"
                           , opts->prefix
                           , opts->name
                           , label_pre
                           , opts->labels
                           , label_post
                           , value
                           , ((rv->last_updated) ? rv->last_updated : opts->now) * 1000ULL
            );
        else
            buffer_sprintf(opts->wb, "%s_%s%s%s%s " CALCULATED_NUMBER_FORMAT "\n"
                           , opts->prefix
                           , opts->name
                           , label_pre
                           , opts->labels
                           , label_post
                           , value
            );

        return 1;
    }

    return 0;
}

static void rrd_stats_api_v1_charts_allmetrics_prometheus(RRDHOST *host, BUFFER *wb, const char *prefix, BACKEND_OPTIONS backend_options, time_t after, time_t before, int allhosts, PROMETHEUS_OUTPUT_OPTIONS output_options) {
    rrdhost_rdlock(host);

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, host->hostname, PROMETHEUS_ELEMENT_MAX);

    char labels[PROMETHEUS_LABELS_MAX + 1] = "";
    if(allhosts) {
        if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1 %llu\n", hostname, host->program_name, host->program_version, now_realtime_usec() / USEC_PER_MS);
        else
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1\n", hostname, host->program_name, host->program_version);

        if(host->tags && *(host->tags)) {
            if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS) {
                buffer_sprintf(wb, "netdata_host_tags_info{instance=\"%s\",%s} 1 %llu\n", hostname, host->tags, now_realtime_usec() / USEC_PER_MS);

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(wb, "netdata_host_tags{instance=\"%s\",%s} 1 %llu\n", hostname, host->tags, now_realtime_usec() / USEC_PER_MS);
            }
            else {
                buffer_sprintf(wb, "netdata_host_tags_info{instance=\"%s\",%s} 1\n", hostname, host->tags);

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(wb, "netdata_host_tags{instance=\"%s\",%s} 1\n", hostname, host->tags);
            }

        }

        snprintfz(labels, PROMETHEUS_LABELS_MAX, ",instance=\"%s\"", hostname);
    }
    else {
        if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1 %llu\n", hostname, host->program_name, host->program_version, now_realtime_usec() / USEC_PER_MS);
        else
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1\n", hostname, host->program_name, host->program_version);

        if(host->tags && *(host->tags)) {
            if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS) {
                buffer_sprintf(wb, "netdata_host_tags_info{%s} 1 %llu\n", host->tags, now_realtime_usec() / USEC_PER_MS);

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(wb, "netdata_host_tags{%s} 1 %llu\n", host->tags, now_realtime_usec() / USEC_PER_MS);
            }
            else {
                buffer_sprintf(wb, "netdata_host_tags_info{%s} 1\n", host->tags);

                // deprecated, exists only for compatibility with older queries
                buffer_sprintf(wb, "netdata_host_tags{%s} 1\n", host->tags);
            }
        }
    }

    // send custom variables set for the host
    if(output_options & PROMETHEUS_OUTPUT_VARIABLES){
        struct host_variables_callback_options opts = {
                .host = host,
                .wb = wb,
                .labels = (labels[0] == ',')?&labels[1]:labels,
                .backend_options = backend_options,
                .output_options = output_options,
                .prefix = prefix,
                .now = now_realtime_sec(),
                .host_header_printed = 0
        };
        foreach_host_variable_callback(host, print_host_variables, &opts);
    }

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        char chart[PROMETHEUS_ELEMENT_MAX + 1];
        char context[PROMETHEUS_ELEMENT_MAX + 1];
        char family[PROMETHEUS_ELEMENT_MAX + 1];
        char units[PROMETHEUS_ELEMENT_MAX + 1] = "";

        prometheus_label_copy(chart, (output_options & PROMETHEUS_OUTPUT_NAMES && st->name)?st->name:st->id, PROMETHEUS_ELEMENT_MAX);
        prometheus_label_copy(family, st->family, PROMETHEUS_ELEMENT_MAX);
        prometheus_name_copy(context, st->context, PROMETHEUS_ELEMENT_MAX);

        if(likely(backends_can_send_rrdset(backend_options, st))) {
            rrdset_rdlock(st);

            int as_collected = (BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED);
            int homogeneous = 1;
            if(as_collected) {
                if(rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK))
                    rrdset_update_heterogeneous_flag(st);

                if(rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
                    homogeneous = 0;
            }
            else {
                if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AVERAGE && !(output_options & PROMETHEUS_OUTPUT_HIDEUNITS))
                    prometheus_units_copy(units, st->units, PROMETHEUS_ELEMENT_MAX, output_options & PROMETHEUS_OUTPUT_OLDUNITS);
            }

            if(unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                buffer_sprintf(wb, "\n# COMMENT %s chart \"%s\", context \"%s\", family \"%s\", units \"%s\"\n"
                               , (homogeneous)?"homogeneous":"heterogeneous"
                               , (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id
                               , st->context
                               , st->family
                               , st->units
                );

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    char dimension[PROMETHEUS_ELEMENT_MAX + 1];
                    char *suffix = "";

                    if (as_collected) {
                        // we need as-collected / raw data

                        if(unlikely(rd->last_collected_time.tv_sec < after))
                            continue;

                        const char *t = "gauge", *h = "gives";
                        if(rd->algorithm == RRD_ALGORITHM_INCREMENTAL ||
                           rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL) {
                            t = "counter";
                            h = "delta gives";
                            suffix = "_total";
                        }

                        if(homogeneous) {
                            // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                            // we add all dimensions as labels

                            prometheus_label_copy(dimension, (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if(unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(wb
                                               , "# COMMENT %s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n"
                                               , prefix
                                               , context
                                               , suffix
                                               , (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id
                                               , st->context
                                               , st->family
                                               , (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id
                                               , rd->multiplier
                                               , rd->divisor
                                               , h
                                               , st->units
                                               , t
                                );

                            if(unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s%s %s\n"
                                               , prefix
                                               , context
                                               , suffix
                                               , t
                                );

                            if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(wb
                                               , "%s_%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " COLLECTED_NUMBER_FORMAT " %llu\n"
                                               , prefix
                                               , context
                                               , suffix
                                               , chart
                                               , family
                                               , dimension
                                               , labels
                                               , rd->last_collected_value
                                               , timeval_msec(&rd->last_collected_time)
                                );
                            else
                                buffer_sprintf(wb
                                               , "%s_%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " COLLECTED_NUMBER_FORMAT "\n"
                                               , prefix
                                               , context
                                               , suffix
                                               , chart
                                               , family
                                               , dimension
                                               , labels
                                               , rd->last_collected_value
                                );
                        }
                        else {
                            // the dimensions of the chart, do not have the same algorithm, multiplier or divisor
                            // we create a metric per dimension

                            prometheus_name_copy(dimension, (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if(unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(wb
                                               , "# COMMENT %s_%s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n"
                                               , prefix
                                               , context
                                               , dimension
                                               , suffix
                                               , (output_options & PROMETHEUS_OUTPUT_NAMES && st->name) ? st->name : st->id
                                               , st->context
                                               , st->family
                                               , (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id
                                               , rd->multiplier
                                               , rd->divisor
                                               , h
                                               , st->units
                                               , t
                                );

                            if(unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s_%s%s %s\n"
                                               , prefix
                                               , context
                                               , dimension
                                               , suffix
                                               , t
                                );

                            if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(wb
                                               , "%s_%s_%s%s{chart=\"%s\",family=\"%s\"%s} " COLLECTED_NUMBER_FORMAT " %llu\n"
                                               , prefix
                                               , context
                                               , dimension
                                               , suffix
                                               , chart
                                               , family
                                               , labels
                                               , rd->last_collected_value
                                               , timeval_msec(&rd->last_collected_time)
                                );
                            else
                                buffer_sprintf(wb
                                               , "%s_%s_%s%s{chart=\"%s\",family=\"%s\"%s} " COLLECTED_NUMBER_FORMAT "\n"
                                               , prefix
                                               , context
                                               , dimension
                                               , suffix
                                               , chart
                                               , family
                                               , labels
                                               , rd->last_collected_value
                                );
                        }
                    }
                    else {
                        // we need average or sum of the data

                        time_t first_t = after, last_t = before;
                        calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

                        if(!isnan(value) && !isinf(value)) {

                            if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AVERAGE)
                                suffix = "_average";
                            else if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_SUM)
                                suffix = "_sum";

                            prometheus_label_copy(dimension, (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_HELP))
                                buffer_sprintf(wb, "# COMMENT %s_%s%s%s: dimension \"%s\", value is %s, gauge, dt %llu to %llu inclusive\n"
                                               , prefix
                                               , context
                                               , units
                                               , suffix
                                               , (output_options & PROMETHEUS_OUTPUT_NAMES && rd->name) ? rd->name : rd->id
                                               , st->units
                                               , (unsigned long long)first_t
                                               , (unsigned long long)last_t
                                );

                            if (unlikely(output_options & PROMETHEUS_OUTPUT_TYPES))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s%s%s gauge\n"
                                               , prefix
                                               , context
                                               , units
                                               , suffix
                                );

                            if(output_options & PROMETHEUS_OUTPUT_TIMESTAMPS)
                                buffer_sprintf(wb, "%s_%s%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " CALCULATED_NUMBER_FORMAT " %llu\n"
                                               , prefix
                                               , context
                                               , units
                                               , suffix
                                               , chart
                                               , family
                                               , dimension
                                               , labels
                                               , value
                                               , last_t * MSEC_PER_SEC
                                );
                            else
                                buffer_sprintf(wb, "%s_%s%s%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " CALCULATED_NUMBER_FORMAT "\n"
                                               , prefix
                                               , context
                                               , units
                                               , suffix
                                               , chart
                                               , family
                                               , dimension
                                               , labels
                                               , value
                                );
                        }
                    }
                }
            }

            rrdset_unlock(st);
        }
    }

    rrdhost_unlock(host);
}

#if ENABLE_PROMETHEUS_REMOTE_WRITE
inline static void remote_write_split_words(char *str, char **words, int max_words) {
    char *s = str;
    int i = 0;

    while(*s && i < max_words - 1) {
        while(*s && isspace(*s)) s++; // skip spaces to the begining of a tag name

        if(*s)
            words[i] = s;

        while(*s && !isspace(*s) && *s != '=') s++; // find the end of the tag name

        if(*s != '=') {
            words[i] = NULL;
            break;
        }
        *s = '\0';
        s++;
        i++;

        while(*s && isspace(*s)) s++; // skip spaces to the begining of a tag value

        if(*s && *s == '"') s++; // strip an opening quote
        if(*s)
            words[i] = s;

        while(*s && !isspace(*s) && *s != ',') s++; // find the end of the tag value

        if(*s && *s != ',') {
            words[i] = NULL;
            break;
        }
        if(s != words[i] && *(s - 1) == '"') *(s - 1) = '\0'; // strip a closing quote
        if(*s != '\0') {
            *s = '\0';
            s++;
            i++;
        }
    }
}

void rrd_stats_remote_write_allmetrics_prometheus(
        RRDHOST *host
        , const char *__hostname
        , const char *prefix
        , BACKEND_OPTIONS backend_options
        , time_t after
        , time_t before
        , size_t *count_charts
        , size_t *count_dims
        , size_t *count_dims_skipped
) {
    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, __hostname, PROMETHEUS_ELEMENT_MAX);

    add_host_info("netdata_info", hostname, host->program_name, host->program_version, now_realtime_usec() / USEC_PER_MS);

    if(host->tags && *(host->tags)) {
        char tags[PROMETHEUS_LABELS_MAX + 1];
        strncpy(tags, host->tags, PROMETHEUS_LABELS_MAX);
        char *words[PROMETHEUS_LABELS_MAX_NUMBER] = {NULL};
        int i;

        remote_write_split_words(tags, words, PROMETHEUS_LABELS_MAX_NUMBER);

        add_host_info("netdata_host_tags_info", hostname, NULL, NULL, now_realtime_usec() / USEC_PER_MS);

        for(i = 0; words[i] != NULL && words[i + 1] != NULL && (i + 1) < PROMETHEUS_LABELS_MAX_NUMBER; i += 2) {
            add_tag(words[i], words[i + 1]);
        }
    }

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        char chart[PROMETHEUS_ELEMENT_MAX + 1];
        char context[PROMETHEUS_ELEMENT_MAX + 1];
        char family[PROMETHEUS_ELEMENT_MAX + 1];
        char units[PROMETHEUS_ELEMENT_MAX + 1] = "";

        prometheus_label_copy(chart, (backend_options & BACKEND_OPTION_SEND_NAMES && st->name)?st->name:st->id, PROMETHEUS_ELEMENT_MAX);
        prometheus_label_copy(family, st->family, PROMETHEUS_ELEMENT_MAX);
        prometheus_name_copy(context, st->context, PROMETHEUS_ELEMENT_MAX);

        if(likely(backends_can_send_rrdset(backend_options, st))) {
            rrdset_rdlock(st);

            (*count_charts)++;

            int as_collected = (BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED);
            int homogeneous = 1;
            if(as_collected) {
                if(rrdset_flag_check(st, RRDSET_FLAG_HOMOGENEOUS_CHECK))
                    rrdset_update_heterogeneous_flag(st);

                if(rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
                    homogeneous = 0;
            }
            else {
                if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AVERAGE)
                    prometheus_units_copy(units, st->units, PROMETHEUS_ELEMENT_MAX, 0);
            }

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter && !rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                    char name[PROMETHEUS_LABELS_MAX + 1];
                    char dimension[PROMETHEUS_ELEMENT_MAX + 1];
                    char *suffix = "";

                    if (as_collected) {
                        // we need as-collected / raw data

                        if(unlikely(rd->last_collected_time.tv_sec < after)) {
                            debug(D_BACKEND, "BACKEND: not sending dimension '%s' of chart '%s' from host '%s', its last data collection (%lu) is not within our timeframe (%lu to %lu)", rd->id, st->id, __hostname, (unsigned long)rd->last_collected_time.tv_sec, (unsigned long)after, (unsigned long)before);
                            (*count_dims_skipped)++;
                            continue;
                        }

                        if(homogeneous) {
                            // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                            // we add all dimensions as labels

                            prometheus_label_copy(dimension, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);
                            snprintf(name, PROMETHEUS_LABELS_MAX, "%s_%s%s", prefix, context, suffix);

                            add_metric(name, chart, family, dimension, hostname, rd->last_collected_value, timeval_msec(&rd->last_collected_time));
                            (*count_dims)++;
                        }
                        else {
                            // the dimensions of the chart, do not have the same algorithm, multiplier or divisor
                            // we create a metric per dimension

                            prometheus_name_copy(dimension, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);
                            snprintf(name, PROMETHEUS_LABELS_MAX, "%s_%s_%s%s", prefix, context, dimension, suffix);

                            add_metric(name, chart, family, NULL, hostname, rd->last_collected_value, timeval_msec(&rd->last_collected_time));
                            (*count_dims)++;
                        }
                    }
                    else {
                        // we need average or sum of the data

                        time_t first_t = after, last_t = before;
                        calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

                        if(!isnan(value) && !isinf(value)) {

                            if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AVERAGE)
                                suffix = "_average";
                            else if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_SUM)
                                suffix = "_sum";

                            prometheus_label_copy(dimension, (backend_options & BACKEND_OPTION_SEND_NAMES && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);
                            snprintf(name, PROMETHEUS_LABELS_MAX, "%s_%s%s%s", prefix, context, units, suffix);

                            add_metric(name, chart, family, dimension, hostname, rd->last_collected_value, timeval_msec(&rd->last_collected_time));
                            (*count_dims)++;
                        }
                    }
                }
            }

            rrdset_unlock(st);
        }
    }
}
#endif /* ENABLE_PROMETHEUS_REMOTE_WRITE */

static inline time_t prometheus_preparation(RRDHOST *host, BUFFER *wb, BACKEND_OPTIONS backend_options, const char *server, time_t now, PROMETHEUS_OUTPUT_OPTIONS output_options) {
    if(!server || !*server) server = "default";

    time_t after  = prometheus_server_last_access(server, host, now);

    int first_seen = 0;
    if(!after) {
        after = now - global_backend_update_every;
        first_seen = 1;
    }

    if(after > now) {
        // oops! this should never happen
        after = now - global_backend_update_every;
    }

    if(output_options & PROMETHEUS_OUTPUT_HELP) {
        char *mode;
        if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AS_COLLECTED)
            mode = "as collected";
        else if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_AVERAGE)
            mode = "average";
        else if(BACKEND_OPTIONS_DATA_SOURCE(backend_options) == BACKEND_SOURCE_DATA_SUM)
            mode = "sum";
        else
            mode = "unknown";

        buffer_sprintf(wb, "# COMMENT netdata \"%s\" to %sprometheus \"%s\", source \"%s\", last seen %lu %s, time range %lu to %lu\n\n"
                , host->hostname
                , (first_seen)?"FIRST SEEN ":""
                , server
                , mode
                , (unsigned long)((first_seen)?0:(now - after))
                , (first_seen)?"never":"seconds ago"
                , (unsigned long)after, (unsigned long)now
        );
    }

    return after;
}

void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, BACKEND_OPTIONS backend_options, PROMETHEUS_OUTPUT_OPTIONS output_options) {
    time_t before = now_realtime_sec();

    // we start at the point we had stopped before
    time_t after = prometheus_preparation(host, wb, backend_options, server, before, output_options);

    rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, backend_options, after, before, 0, output_options);
}

void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, BACKEND_OPTIONS backend_options, PROMETHEUS_OUTPUT_OPTIONS output_options) {
    time_t before = now_realtime_sec();

    // we start at the point we had stopped before
    time_t after = prometheus_preparation(host, wb, backend_options, server, before, output_options);

    rrd_rdlock();
    rrdhost_foreach_read(host) {
        rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, backend_options, after, before, 1, output_options);
    }
    rrd_unlock();
}

#if ENABLE_PROMETHEUS_REMOTE_WRITE
int process_prometheus_remote_write_response(BUFFER *b) {
    if(unlikely(!b)) return 1;

    const char *s = buffer_tostring(b);
    int len = buffer_strlen(b);

    // do nothing with HTTP responses 200 or 204

    while(!isspace(*s) && len) {
        s++;
        len--;
    }
    s++;
    len--;

    if(likely(len > 4 && (!strncmp(s, "200 ", 4) || !strncmp(s, "204 ", 4))))
        return 0;
    else
        return discard_response(b, "prometheus remote write");
}
#endif
