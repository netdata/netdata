#include "common.h"

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

static inline char *prometheus_units_copy(char *d, const char *s, size_t usable) {
    const char *sorig = s;
    char *ret = d;
    size_t n;

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

static void rrd_stats_api_v1_charts_allmetrics_prometheus(RRDHOST *host, BUFFER *wb, const char *prefix, uint32_t options, time_t after, time_t before, int allhosts, int help, int types, int names, int timestamps) {
    rrdhost_rdlock(host);

    char hostname[PROMETHEUS_ELEMENT_MAX + 1];
    prometheus_label_copy(hostname, host->hostname, PROMETHEUS_ELEMENT_MAX);

    char labels[PROMETHEUS_LABELS_MAX + 1] = "";
    if(allhosts) {
        if(timestamps)
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1 %llu\n", hostname, host->program_name, host->program_version, now_realtime_usec() / USEC_PER_MS);
        else
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1\n", hostname, host->program_name, host->program_version);

        if(host->tags && *(host->tags)) {
            if(timestamps) {
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
        if(timestamps)
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1 %llu\n", hostname, host->program_name, host->program_version, now_realtime_usec() / USEC_PER_MS);
        else
            buffer_sprintf(wb, "netdata_info{instance=\"%s\",application=\"%s\",version=\"%s\"} 1\n", hostname, host->program_name, host->program_version);

        if(host->tags && *(host->tags)) {
            if(timestamps) {
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

    // for each chart
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        char chart[PROMETHEUS_ELEMENT_MAX + 1];
        char context[PROMETHEUS_ELEMENT_MAX + 1];
        char family[PROMETHEUS_ELEMENT_MAX + 1];
        char units[PROMETHEUS_ELEMENT_MAX + 1] = "";

        prometheus_label_copy(chart, (names && st->name)?st->name:st->id, PROMETHEUS_ELEMENT_MAX);
        prometheus_label_copy(family, st->family, PROMETHEUS_ELEMENT_MAX);
        prometheus_name_copy(context, st->context, PROMETHEUS_ELEMENT_MAX);

        if(likely(backends_can_send_rrdset(options, st))) {
            rrdset_rdlock(st);

            int as_collected = ((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AS_COLLECTED);
            int homogeneus = 1;
            if(as_collected) {
                if(rrdset_flag_check(st, RRDSET_FLAG_HOMEGENEOUS_CHECK))
                    rrdset_update_heterogeneous_flag(st);

                if(rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS))
                    homogeneus = 0;
            }
            else {
                if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AVERAGE)
                    prometheus_units_copy(units, st->units, PROMETHEUS_ELEMENT_MAX);
            }

            if(unlikely(help))
                buffer_sprintf(wb, "\n# COMMENT %s chart \"%s\", context \"%s\", family \"%s\", units \"%s\"\n"
                               , (homogeneus)?"homogeneus":"heterogeneous"
                               , (names && st->name) ? st->name : st->id
                               , st->context
                               , st->family
                               , st->units
                );

            // for each dimension
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if(rd->collections_counter) {
                    char dimension[PROMETHEUS_ELEMENT_MAX + 1];
                    char *suffix = "";

                    if (as_collected) {
                        // we need as-collected / raw data

                        const char *t = "gauge", *h = "gives";
                        if(rd->algorithm == RRD_ALGORITHM_INCREMENTAL ||
                           rd->algorithm == RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL) {
                            t = "counter";
                            h = "delta gives";
                            suffix = "_total";
                        }

                        if(homogeneus) {
                            // all the dimensions of the chart, has the same algorithm, multiplier and divisor
                            // we add all dimensions as labels

                            prometheus_label_copy(dimension, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if(unlikely(help))
                                buffer_sprintf(wb
                                               , "# COMMENT %s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n"
                                               , prefix
                                               , context
                                               , suffix
                                               , (names && st->name) ? st->name : st->id
                                               , st->context
                                               , st->family
                                               , (names && rd->name) ? rd->name : rd->id
                                               , rd->multiplier
                                               , rd->divisor
                                               , h
                                               , st->units
                                               , t
                                );

                            if(unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s%s %s\n"
                                               , prefix
                                               , context
                                               , suffix
                                               , t
                                );

                            if(timestamps)
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

                            prometheus_name_copy(dimension, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if(unlikely(help))
                                buffer_sprintf(wb
                                               , "# COMMENT %s_%s_%s%s: chart \"%s\", context \"%s\", family \"%s\", dimension \"%s\", value * " COLLECTED_NUMBER_FORMAT " / " COLLECTED_NUMBER_FORMAT " %s %s (%s)\n"
                                               , prefix
                                               , context
                                               , dimension
                                               , suffix
                                               , (names && st->name) ? st->name : st->id
                                               , st->context
                                               , st->family
                                               , (names && rd->name) ? rd->name : rd->id
                                               , rd->multiplier
                                               , rd->divisor
                                               , h
                                               , st->units
                                               , t
                                );

                            if(unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s_%s%s %s\n"
                                               , prefix
                                               , context
                                               , dimension
                                               , suffix
                                               , t
                                );

                            if(timestamps)
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
                        calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, options, &first_t, &last_t);

                        if(!isnan(value) && !isinf(value)) {

                            if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_AVERAGE)
                                suffix = "_average";
                            else if((options & BACKEND_SOURCE_BITS) == BACKEND_SOURCE_DATA_SUM)
                                suffix = "_sum";

                            prometheus_label_copy(dimension, (names && rd->name) ? rd->name : rd->id, PROMETHEUS_ELEMENT_MAX);

                            if (unlikely(help))
                                buffer_sprintf(wb, "# COMMENT %s_%s%s%s: dimension \"%s\", value is %s, gauge, dt %llu to %llu inclusive\n"
                                               , prefix
                                               , context
                                               , units
                                               , suffix
                                               , (names && rd->name) ? rd->name : rd->id
                                               , st->units
                                               , (unsigned long long)first_t
                                               , (unsigned long long)last_t
                                );

                            if (unlikely(types))
                                buffer_sprintf(wb, "# COMMENT TYPE %s_%s%s%s gauge\n"
                                               , prefix
                                               , context
                                               , units
                                               , suffix
                                );

                            if(timestamps)
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

static inline time_t prometheus_preparation(RRDHOST *host, BUFFER *wb, uint32_t options, const char *server, time_t now, int help) {
    if(!server || !*server) server = "default";

    time_t after  = prometheus_server_last_access(server, host, now);

    int first_seen = 0;
    if(!after) {
        after = now - backend_update_every;
        first_seen = 1;
    }

    if(after > now) {
        // oops! this should never happen
        after = now - backend_update_every;
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
            buffer_sprintf(wb, ", time range %lu to %lu", (unsigned long)after, (unsigned long)now);

        buffer_strcat(wb, "\n\n");
    }

    return after;
}

void rrd_stats_api_v1_charts_allmetrics_prometheus_single_host(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names, int timestamps) {
    time_t before = now_realtime_sec();

    // we start at the point we had stopped before
    time_t after = prometheus_preparation(host, wb, options, server, before, help);

    rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, options, after, before, 0, help, types, names, timestamps);
}

void rrd_stats_api_v1_charts_allmetrics_prometheus_all_hosts(RRDHOST *host, BUFFER *wb, const char *server, const char *prefix, uint32_t options, int help, int types, int names, int timestamps) {
    time_t before = now_realtime_sec();

    // we start at the point we had stopped before
    time_t after = prometheus_preparation(host, wb, options, server, before, help);

    rrd_rdlock();
    rrdhost_foreach_read(host) {
        rrd_stats_api_v1_charts_allmetrics_prometheus(host, wb, prefix, options, after, before, 1, help, types, names, timestamps);
    }
    rrd_unlock();
}
