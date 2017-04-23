#include "common.h"

static struct statsd {
    size_t events;
    size_t events_gauge;
    size_t events_counter;
    size_t events_timer;
    size_t events_meter;
    size_t events_histogram;

    size_t metrics;
    size_t metrics_gauge;
    size_t metrics_counter;
    size_t metrics_timer;
    size_t metrics_meter;
    size_t metrics_histogram;
} statsd = {
        .events = 0,
        .metrics = 0
};

// --------------------------------------------------------------------------------------

static LISTEN_SOCKETS statsd_sockets = {
        .config_section  = CONFIG_SECTION_STATSD,
        .default_bind_to = "udp:* tcp:*",
        .default_port    = STATSD_LISTEN_PORT,
        .backlog         = STATSD_LISTEN_BACKLOG
};

int statsd_listen_sockets_setup(void) {
    return listen_sockets_setup(&statsd_sockets);
}

// --------------------------------------------------------------------------------------

typedef enum statsd_metric_type {
    STATSD_METRIC_TYPE_GAUGE        = 'g',
    STATSD_METRIC_TYPE_COUNTER      = 'c',
    STATSD_METRIC_TYPE_TIMER        = 't',
    STATSD_METRIC_TYPE_HISTOGRAM    = 'h',
    STATSD_METRIC_TYPE_METER        = 'm'
} STATSD_METRIC_TYPE;

typedef struct statsd_metric {
    avl avl;                        // indexing

    const char *name;
    uint32_t hash;                  // hash of the name

    STATSD_METRIC_TYPE type;

    usec_t last_collected_ut;       // the last time this metric was updated
    usec_t last_exposed_ut;         // the last time this metric was sent to netdata
    size_t events;                  // the number of times this metrics has been collected

    calculated_number count;        // number of events since the last exposure to netdata
    calculated_number last;         // the last value collected
    calculated_number value;        // the value of the metric
    calculated_number min;          // the min value collected since the last exposure to netdata
    calculated_number max;          // the max value collected since the last exposure to netdata

    RRDSET *st;
    RRDDIM *rd_min;
    RRDDIM *rd_max;
    RRDDIM *rd_avg;

    struct statsd_metric *next;
} STATSD_METRIC;


// --------------------------------------------------------------------------------------------------------------------
// statsd index

int statsd_compare(void* a, void* b) {
    if(((STATSD_METRIC *)a)->hash < ((STATSD_METRIC *)b)->hash) return -1;
    else if(((STATSD_METRIC *)a)->hash > ((STATSD_METRIC *)b)->hash) return 1;
    else return strcmp(((STATSD_METRIC *)a)->name, ((STATSD_METRIC *)b)->name);
}

static avl_tree statsd_index = {
        .compar = statsd_compare,
        .root = NULL
};

static inline STATSD_METRIC *stasd_metric_index_find(const char *name, uint32_t hash) {
    STATSD_METRIC tmp;
    tmp.name = name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (STATSD_METRIC *)avl_search(&statsd_index, (avl *)&tmp);
}


// --------------------------------------------------------------------------------------------------------------------
// statsd data collection

#define STATSD_GAUGE_COLLECTION_RELATIVE 0x00000001

static inline void statsd_collected_value(STATSD_METRIC *m, calculated_number value, calculated_number sample_rate, uint32_t options) {
    debug(D_STATSD, "Updating metric '%s'", m->name);

    statsd.events++;

    m->last_collected_ut = now_realtime_usec();
    m->events++;
    m->count += sample_rate;

    m->last = value;
    if(value < m->min)
        m->min = value;
    if(value > m->max)
        m->max = value;

    switch(m->type) {
        case STATSD_METRIC_TYPE_HISTOGRAM:
            statsd.events_histogram++;
            // FIXME: not implemented yet
            m->value += value;
            break;

        case STATSD_METRIC_TYPE_METER:
            statsd.events_meter++;
            // we add to this metric
            m->value += value;
            break;

        case STATSD_METRIC_TYPE_TIMER:
            statsd.events_timer++;
            // we add time to this metric
            m->value += value;
            break;

        case STATSD_METRIC_TYPE_GAUGE:
            statsd.events_gauge++;
            if(unlikely(options & STATSD_GAUGE_COLLECTION_RELATIVE))
                // we add the collected value
                m->value += value;
            else
                // we replace the value of the metric
                m->value = value;
            break;

        case STATSD_METRIC_TYPE_COUNTER:
            statsd.events_counter++;
            // we add the collected value
            m->value += value;
            break;
    }

    debug(D_STATSD, "Updated metric '%s', type '%c', events %zu, count %0.5Lf, last_collected %llu, last value %0.5Lf, value %0.5Lf, min %0.5Lf, max %0.5Lf"
          , m->name
          , (char)m->type
          , m->events
          , m->count
          , m->last_collected_ut
          , m->last
          , m->value
          , m->min
          , m->max
    );
}

static inline void statsd_process_metric(const char *metric, STATSD_METRIC_TYPE type, calculated_number value, calculated_number sample_rate, uint32_t options) {
    debug(D_STATSD, "processing metric '%s', type '%c', value %0.5Lf, sample_rate %0.5Lf, options '0x%08x'", metric, (char)type, value, sample_rate, options);

    uint32_t hash = simple_hash(metric);

    STATSD_METRIC *m = stasd_metric_index_find(metric, hash);
    if(unlikely(!m)) {
        debug(D_STATSD, "Creating new metric '%s'", metric);

        m = (STATSD_METRIC *)callocz(sizeof(STATSD_METRIC), 1);
        m->name = strdupz(metric);
        m->hash = hash;
        m->type = type;
        m = (STATSD_METRIC *)avl_insert(&statsd_index, (avl *)m);

        statsd.metrics++;
        switch(type) {
            case STATSD_METRIC_TYPE_COUNTER:
                statsd.metrics_counter++;
                break;

            case STATSD_METRIC_TYPE_GAUGE:
                statsd.metrics_gauge++;
                break;

            case STATSD_METRIC_TYPE_METER:
                statsd.metrics_meter++;
                break;

            case STATSD_METRIC_TYPE_HISTOGRAM:
                statsd.metrics_histogram++;
                break;

            case STATSD_METRIC_TYPE_TIMER:
                statsd.metrics_timer++;
        }
    }

    statsd_collected_value(m, value, sample_rate, options);
}


// --------------------------------------------------------------------------------------------------------------------
// statsd parsing

static void statsd_process_metric_raw(char *m, char *v, char *t, char *r) {
    debug(D_STATSD, "STATSD: raw metric '%s', value '%s', type '%s', rate '%s'", m, v, t, r);

    if(unlikely(!m || !*m)) return;

    STATSD_METRIC_TYPE type = STATSD_METRIC_TYPE_METER;
    calculated_number value = 1.0;
    calculated_number sample_rate = 1.0;
    uint32_t options = 0;
    char *e;

    // collect the value
    if(likely(v && *v)) {
        e = NULL;
        value = strtold(v, &e);
        if(e && *e)
            error("STATSD: excess data '%s' after value, metric '%s', value '%s'", e, m, v);
    }

    if(likely(r && *r)) {
        e = NULL;
        sample_rate = strtold(r, &e);
        if(e && *e)
            error("STATSD: excess data '%s' after sampling rate, metric '%s', value '%s', sampling rate '%s'", e, m, v, r);
    }

    // we have the metric name, value and type
    if(likely(t && *t)) {
        switch (*t) {
            case 'g':
                type = STATSD_METRIC_TYPE_GAUGE;
                if (unlikely(*v == '-' || *v == '+'))
                    options |= STATSD_GAUGE_COLLECTION_RELATIVE;
                break;

            default:
            case 'c':
                type = STATSD_METRIC_TYPE_COUNTER;
                break;

            case 'm':
                if (t[1] == 's') type = STATSD_METRIC_TYPE_TIMER;
                else type = STATSD_METRIC_TYPE_METER;
                break;

            case 'h':
                type = STATSD_METRIC_TYPE_HISTOGRAM;
                break;
        }
    }

    statsd_process_metric(m, type, value, sample_rate, options);
}

static inline int is_statsd_space(char c) {
    if(c == ' ' || c == '\t')
        return 1;

    return 0;
}

static inline int is_statsd_newline(char c) {
    if(c == '\r' || c == '\n')
        return 1;

    return 0;
}

static inline int is_statsd_separator(char c) {
    if(c == ':' || c == '|' || c == '@')
        return 1;

    return 0;
}

static inline char *skip_to_next_separator(char *s) {
    while(*s && !is_statsd_separator(*s) && !is_statsd_newline(*s))
        s++;

    return s;
}

static char *statsd_get_field(char *s, char **field, int *line_break) {
    *line_break = 0;

    while(is_statsd_separator(*s) || is_statsd_space(*s))
        s++;

    if(unlikely(is_statsd_newline(*s))) {
        *field = NULL;
        *line_break = 1;

        while(is_statsd_newline(*s))
            *s++ = '\0';

        return s;
    }

    *field = s;
    s = skip_to_next_separator(s);

    if(unlikely(is_statsd_newline(*s))) {
        *line_break = 1;

        while(is_statsd_newline(*s))
            *s++ = '\0';
    }
    else if(likely(*s))
        *s++ = '\0';

    return s;
}

static void statsd_process(char *buffer, size_t size) {
    buffer[size] = '\0';
    debug(D_STATSD, "RECEIVED: '%s'", buffer);

    char *s = buffer, *m, *v, *t, *r;
    while(*s) {
        m = v = t = r = NULL;
        int line_break = 0;

        s = statsd_get_field(s, &m, &line_break);
        if(unlikely(line_break)) {
            statsd_process_metric_raw(m, v, t, r);
            continue;
        }

        s = statsd_get_field(s, &v, &line_break);
        if(unlikely(line_break)) {
            statsd_process_metric_raw(m, v, t, r);
            continue;
        }

        s = statsd_get_field(s, &t, &line_break);
        if(likely(line_break)) {
            statsd_process_metric_raw(m, v, t, r);
            continue;
        }

        s = statsd_get_field(s, &r, &line_break);
        if(likely(line_break)) {
            statsd_process_metric_raw(m, v, t, r);
            continue;
        }

        if(*s && *s != '\r' && *s != '\n') {
            error("STATSD: excess data '%s' at end of line, metric '%s', value '%s', sampling rate '%s'", s, m, v, r);

            // delete the excess data at the end if line
            while(*s && *s != '\r' && *s != '\n')
                s++;
        }

        statsd_process_metric_raw(m, v, t, r);
    }
}


// --------------------------------------------------------------------------------------------------------------------
// statsd pollfd interface

static char statsd_read_buffer[65536];

// new TCP client connected
static void *statsd_add_callback(int fd, short int *events) {
    (void)fd;
    *events = POLLIN;

    return NULL;
}


// TCP client disconnected
static void statsd_del_callback(int fd, void *data) {
    (void)fd;
    (void)data;

    return;
}

// Receive data
static int statsd_rcv_callback(int fd, int socktype, void *data, short int *events) {
    (void)data;

    switch(socktype) {
        case SOCK_STREAM: {
            ssize_t rc;
            do {
                rc = recv(fd, statsd_read_buffer, sizeof(statsd_read_buffer), MSG_DONTWAIT);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN) {
                        error("STATSD: recv() failed.");
                        return -1;
                    }
                } else if (!rc) {
                    // connection closed
                    error("STATSD: client disconnected.");
                    return -1;
                } else {
                    // data received
                    statsd_process(statsd_read_buffer, (size_t) rc);
                }
            } while (rc != -1);
            break;
        }

        case SOCK_DGRAM: {
            ssize_t rc;
            do {
                // FIXME: collect sender information
                rc = recvfrom(fd, statsd_read_buffer, sizeof(statsd_read_buffer), MSG_DONTWAIT, NULL, NULL);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN) {
                        error("STATSD: recvfrom() failed.");
                        return -1;
                    }
                } else if (rc) {
                    // data received
                    statsd_process(statsd_read_buffer, (size_t) rc);
                }
            } while (rc != -1);
            break;
        }

        default: {
            error("STATSD: unknown socktype %d on socket %d", socktype, fd);
            return -1;
        }
    }

    *events = POLLIN;
    return 0;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd child thread to update netdata

void *statsd_child_thread(void *ptr) {
    info("STATSD thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");


    RRDSET *st_metrics = rrdset_create_localhost(
            "netdata"
            , "statsd_metrics"
            , NULL
            , "statsd"
            , NULL
            , "Metrics in the netdata statsd database"
            , "metrics"
            , 132000
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_metrics_gauge     = rrddim_add(st_metrics, "gauge", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_counter   = rrddim_add(st_metrics, "counter", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_timer     = rrddim_add(st_metrics, "timer", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_meter     = rrddim_add(st_metrics, "meter", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_histogram = rrddim_add(st_metrics, "histogram", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *st_events = rrdset_create_localhost(
            "netdata"
            , "statsd_events"
            , NULL
            , "statsd"
            , NULL
            , "Events processed by the netdata statsd server"
            , "events/s"
            , 132001
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_events_gauge     = rrddim_add(st_events, "gauge", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_counter   = rrddim_add(st_events, "counter", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_timer     = rrddim_add(st_events, "timer", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_meter     = rrddim_add(st_events, "meter", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_histogram = rrddim_add(st_events, "histogram", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(;;) {
        usec_t hb_dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit))
            break;

        if(hb_dt) {
            rrdset_next(st_metrics);
            rrdset_next(st_events);
        }

        rrddim_set_by_pointer(st_metrics, rd_metrics_gauge, (collected_number)statsd.metrics_gauge);
        rrddim_set_by_pointer(st_metrics, rd_metrics_counter, (collected_number)statsd.metrics_counter);
        rrddim_set_by_pointer(st_metrics, rd_metrics_timer, (collected_number)statsd.metrics_timer);
        rrddim_set_by_pointer(st_metrics, rd_metrics_meter, (collected_number)statsd.metrics_meter);
        rrddim_set_by_pointer(st_metrics, rd_metrics_histogram, (collected_number)statsd.metrics_histogram);

        rrddim_set_by_pointer(st_events, rd_events_gauge, (collected_number)statsd.events_gauge);
        rrddim_set_by_pointer(st_events, rd_events_counter, (collected_number)statsd.events_counter);
        rrddim_set_by_pointer(st_events, rd_events_timer, (collected_number)statsd.events_timer);
        rrddim_set_by_pointer(st_events, rd_events_meter, (collected_number)statsd.events_meter);
        rrddim_set_by_pointer(st_events, rd_events_histogram, (collected_number)statsd.events_histogram);

        rrdset_done(st_metrics);
        rrdset_done(st_events);
    }

    pthread_exit(NULL);
    return NULL;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd main thread

void *statsd_main(void *ptr) {
    info("STATSD thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    statsd_listen_sockets_setup();
    if(!statsd_sockets.opened) {
        error("STATSD: No statsd sockets to listen to.");
        goto cleanup;
    }

    pthread_t thread;

    if(pthread_create(&thread, NULL, statsd_child_thread, (void *)NULL))
        error("STATSD: failed to create child thread.");

    else if(pthread_detach(thread))
        error("STATSD: cannot request detach of child thread.");

    poll_events(&statsd_sockets
            , statsd_add_callback
            , statsd_del_callback
            , statsd_rcv_callback
    );

cleanup:
    pthread_cancel(thread);

    debug(D_WEB_CLIENT, "STATSD: exit!");
    listen_sockets_close(&statsd_sockets);

    pthread_exit(NULL);
    return NULL;
}
