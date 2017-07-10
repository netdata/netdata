#include "common.h"

// ----------------------------------------------------------------------------
// PROMETHEUS
// /api/v1/allmetrics?format=prometheus

static struct prometheus_server {
    const char *server;
    uint32_t hash;
    time_t last_access;
    struct prometheus_server *next;
} *prometheus_server_root = NULL;

static inline time_t prometheus_server_last_access(const char *server, time_t now) {
    uint32_t hash = simple_hash(server);

    struct prometheus_server *ps;
    for(ps = prometheus_server_root; ps ;ps = ps->next) {
        if (hash == ps->hash && !strcmp(server, ps->server)) {
            time_t last = ps->last_access;
            ps->last_access = now;
            return last;
        }
    }

    ps = callocz(1, sizeof(struct prometheus_server));
    ps->server = strdupz(server);
    ps->hash = hash;
    ps->last_access = now;
    ps->next = prometheus_server_root;
    prometheus_server_root = ps;

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

#define PROMETHEUS_ELEMENT_MAX 256
#define PROMETHEUS_LABELS_MAX 1024

static void rrd_stats_api_v1_charts_allmetrics_prometheus(RRDHOST *host, BUFFER *wb, const char *prefix, uint32_t options, time_t after, time_t before, int allhosts, int help, int types, int names) {
    rrdhost_rdlock(host);

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, host->hostname, PROMETHEUS_ELEMENT_MAX);

    char labels[PROMETHEUS_LABELS_MAX + 1] = "";
    if(allhosts) {
        if(host->tags && *(host->tags))
            buffer_sprintf(wb, "netdata_host_tags{instance=\"%s\",%s} 1 %llu\n", hostname, host->tags, now_realtime_usec() / USEC_PER_MS);

        snprintfz(labels, PROMETHEUS_LABELS_MAX, ",instance=\"%s\"", hostname);
    }
    else {
        if(host->tags && *(host->tags))
            buffer_sprintf(wb, "netdata_host_tags{%s} 1 %llu\n", host->tags, now_realtime_usec() / USEC_PER_MS);
    }

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        char chart[PROMETHEUS_ELEMENT_MAX + 1];
        char context[PROMETHEUS_ELEMENT_MAX + 1];
        char family[PROMETHEUS_ELEMENT_MAX + 1];

        prometheus_label_copy(chart, (names && st->name)?st->name:st->id, PROMETHEUS_ELEMENT_MAX);
        prometheus_label_copy(family, st->family, PROMETHEUS_ELEMENT_MAX);
        prometheus_name_copy(context, st->context, PROMETHEUS_ELEMENT_MAX);

        if(likely(backends_can_send_rrdset(options, st))) {
            rrdset_rdlock(st);

            if(unlikely(help || types))
                buffer_strcat(wb, "\n");

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter) {
                    char dimension[PROMETHEUS_ELEMENT_MAX + 1];

                    if ((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED) {
                        // we need as-collected / raw data

                        prometheus_name_copy(dimension, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                        const char *t = "gauge", *h = "gives";
                        if (rd->algorithm == RRD_ALGORITHM_INCREMENTAL ||
                            rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL) {
                            t = "counter";
                            h = "delta gives";
                        }

                        if (unlikely(help))
                            buffer_sprintf(wb, "# COMMENT HELP %s_%s_%s netdata chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n",
                                           prefix, context, dimension,
                                           (names && st->name) ? st->name : st->id, st->context,
                                           st->family,
                                           (names && rd->name) ? rd->name : rd->id,
                                           rd->multiplier, rd->divisor,
                                           h, st->units, t
                        );

                        if (unlikely(types))
                            buffer_sprintf(wb, "# COMMENT TYPE %s_%s_%s %s\n", prefix, context, dimension, t);

                        buffer_sprintf(wb, "%s_%s_%s{chart=\"%s\",family=\"%s\"%s} " COLLECTED_NUMBER_FORMAT " %llu\n",
                                       prefix, context, dimension,
                                       chart, family, labels,
                                       rd->last_collected_value, timeval_msec(&rd->last_collected_time)
                        );
                    }
                    else {
                        // we need average or sum of the data

                        calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options);

                        if(!isnan(value) && !isinf(value)) {
                            prometheus_label_copy(dimension, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(help))
                                buffer_sprintf(wb, "# COMMENT HELP %s_%s netdata chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value gives %s (gauge)\n",
                                               prefix, context,
                                               (names && st->name) ? st->name : st->id, st->context,
                                               st->family,
                                               (names && rd->name) ? rd->name : rd->id,
                                               st->units
                                );

                            if (unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s gauge\n", prefix, context);

                            buffer_sprintf(wb, "%s_%s{chart=\"%s\",family=\"%s\",dimension=\"%s\"%s} " CALCULATED_NUMBER_FORMAT " %llu\n",
                                           prefix, context,
                                           chart, family, dimension, labels,
                                           value, timeval_msec(&rd->last_collected_time)
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

static inline time_t prometheus_preparation(RRDHOST *host, BUFFER *wb, uint32_t options, const char *server, time_t now, int help) {
    if(!server || !*server) server = "default";

    time_t after  = prometheus_server_last_access(server, now);

    int first_seen = 0;
    if(!after) {
        after = now - backend_update_every;
        first_seen = 1;
    }

    if(help) {
        int show_range = 1;
        char *mode;
        if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED) {
            mode = "as collected";
            show_range = 0;
        }
        else if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AVERAGE)
            mode = "average";
        else if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_SUM)
            mode = "sum";
        else
            mode = "unknown";

        buffer_sprintf(wb, "# COMMENT netdata \"%s\" to %sprometheus \"%s\", source \"%s\", last seen %lu %s"
                , host->hostname
                , (first_seen)?"FIRST SEEN ":""
                , server
                , mode
                , (unsigned long)((first_seen)?0:(now - after))
                , (first_seen)?"never":"seconds ago"
        );

        if(show_range)
            buffer_sprintf(wb, ", values for time range %lu to %lu", (unsigned long)after, (unsigned long)now);

        buffer_strcat(wb, "\n\n");
    }

    return after;
}

void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names) {
    time_t before = now_realtime_sec();
    time_t after = prometheus_preparation(host, wb, options, server, before, help);

    rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, options, after, before, 0, help, types, names);
}

void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names) {
    time_t before = now_realtime_sec();
    time_t after = prometheus_preparation(host, wb, options, server, before, help);

    rrd_rdlock();
    rrdhost_foreach_read(host) {
        rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, options, after, before, 1, help, types, names);
    }
    rrd_unlock();
}
