#include "common.h"

// --------------------------------------------------------------------------------------

// #define STATSD_MULTITHREADED 1

#ifdef STATSD_MULTITHREADED
#define STATSD_AVL_TREE avl_tree_lock
#define STATSD_AVL_INSERT avl_insert_lock
#define STATSD_AVL_SEARCH avl_search_lock
#define STATSD_AVL_INDEX_INIT { .avl_tree = { NULL, statsd_metric_compare }, .rwlock = AVL_LOCK_INITIALIZER }
#define STATSD_FIRST_PTR_MUTEX netdata_mutex_t first_mutex
#define STATSD_FIRST_PTR_MUTEX_INIT .first_mutex = NETDATA_MUTEX_INITIALIZER
#define STATSD_FIRST_PTR_MUTEX_LOCK(index) netdata_mutex_lock(&((index)->first_mutex))
#define STATSD_FIRST_PTR_MUTEX_UNLOCK(index) netdata_mutex_unlock(&((index)->first_mutex))
#define STATSD_DICTIONARY_OPTIONS DICTIONARY_FLAG_DEFAULT
#else
#define STATSD_AVL_TREE avl_tree
#define STATSD_AVL_INSERT avl_insert
#define STATSD_AVL_SEARCH avl_search
#define STATSD_AVL_INDEX_INIT { .root = NULL, .compar = statsd_metric_compare }
#define STATSD_FIRST_PTR_MUTEX
#define STATSD_FIRST_PTR_MUTEX_INIT
#define STATSD_FIRST_PTR_MUTEX_LOCK(index)
#define STATSD_FIRST_PTR_MUTEX_UNLOCK(index)
#define STATSD_DICTIONARY_OPTIONS DICTIONARY_FLAG_SINGLE_THREADED
#endif

typedef struct statsd_metric_gauge {
    long double value;
} STATSD_METRIC_GAUGE;

typedef struct statsd_metric_counter {
    long long value;
} STATSD_METRIC_COUNTER;

typedef struct statsd_metric_histogram {
    size_t size;
    size_t used;
    long double *values;
} STATSD_METRIC_HISTOGRAM;

typedef struct statsd_metric_set {
    DICTIONARY *dict;
    unsigned long long unique;
} STATSD_METRIC_SET;

typedef struct statsd_metric_set_value {
    char *value;
    uint32_t hash;
} STATSD_METRIC_SET_VALUE;

typedef struct statsd_metric {
    avl avl;                        // indexing
    struct statsd_metric *next;

    const char *name;
    uint32_t hash;                  // hash of the name

    size_t count;                   // the number of times this metrics has been collected
    calculated_number sampling;     // the sampling rate of this metric

    char reset;                     // set to 1 to reset this metric to zero

    union {
        STATSD_METRIC_GAUGE gauge;
        STATSD_METRIC_COUNTER counter;
        STATSD_METRIC_HISTOGRAM histogram;
        STATSD_METRIC_SET set;
    };
} STATSD_METRIC;

typedef struct statsd_index {
    char *name;
    size_t events;
    size_t metrics;
    STATSD_METRIC *first;
    STATSD_AVL_TREE index;
    STATSD_FIRST_PTR_MUTEX;
} STATSD_INDEX;

static int statsd_metric_compare(void* a, void* b);

static struct statsd {
    STATSD_INDEX gauges;
    STATSD_INDEX counters;
    STATSD_INDEX timers;
    STATSD_INDEX histograms;
    STATSD_INDEX meters;
    STATSD_INDEX sets;

    size_t histogram_increase_step;
    int threads;
    LISTEN_SOCKETS sockets;
} statsd = {
        .gauges     = {
                .name = "gauge",
                .events = 0,
                .metrics = 0,
                .first = NULL,
                .index = STATSD_AVL_INDEX_INIT,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .counters   = {
                .name = "counter",
                .events = 0,
                .metrics = 0,
                .first = NULL,
                .index = STATSD_AVL_INDEX_INIT,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .timers     = {
                .name = "timer",
                .events = 0,
                .metrics = 0,
                .first = NULL,
                .index = STATSD_AVL_INDEX_INIT,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .histograms = {
                .name = "histogram",
                .events = 0,
                .metrics = 0,
                .first = NULL,
                .index = STATSD_AVL_INDEX_INIT,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .meters     = {
                .name = "meter",
                .events = 0,
                .metrics = 0,
                .first = NULL,
                .index = STATSD_AVL_INDEX_INIT,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .sets       = {
                .name = "set",
                .events = 0,
                .metrics = 0,
                .first = NULL,
                .index = STATSD_AVL_INDEX_INIT,
                STATSD_FIRST_PTR_MUTEX_INIT
        },

        .histogram_increase_step = 10,
        .threads = 0,
        .sockets = {
                .config_section  = CONFIG_SECTION_STATSD,
                .default_bind_to = "udp:* tcp:*",
                .default_port    = STATSD_LISTEN_PORT,
                .backlog         = STATSD_LISTEN_BACKLOG
        },
};

// --------------------------------------------------------------------------------------

int statsd_listen_sockets_setup(void) {
    return listen_sockets_setup(&statsd.sockets);
}

// --------------------------------------------------------------------------------------------------------------------
// statsd index

static int statsd_metric_compare(void* a, void* b) {
    if(((STATSD_METRIC *)a)->hash < ((STATSD_METRIC *)b)->hash) return -1;
    else if(((STATSD_METRIC *)a)->hash > ((STATSD_METRIC *)b)->hash) return 1;
    else return strcmp(((STATSD_METRIC *)a)->name, ((STATSD_METRIC *)b)->name);
}

static int statsd_metric_set_compare(void* a, void* b) {
    if(((STATSD_METRIC_SET_VALUE *)a)->hash < ((STATSD_METRIC_SET_VALUE *)b)->hash) return -1;
    else if(((STATSD_METRIC_SET_VALUE *)a)->hash > ((STATSD_METRIC_SET_VALUE *)b)->hash) return 1;
    else return strcmp(((STATSD_METRIC_SET_VALUE *)a)->value, ((STATSD_METRIC_SET_VALUE *)b)->value);
}

static inline STATSD_METRIC *stasd_metric_index_find(STATSD_INDEX *index, const char *name, uint32_t hash) {
    STATSD_METRIC tmp;
    tmp.name = name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (STATSD_METRIC *)STATSD_AVL_SEARCH(&index->index, (avl *)&tmp);
}

static inline STATSD_METRIC *statsd_find_or_add_metric(STATSD_INDEX *index, char *metric) {
    debug(D_STATSD, "finding or adding metric '%s' under '%s'", metric, index->name);

    uint32_t hash = simple_hash(metric);

    STATSD_METRIC *m = stasd_metric_index_find(index, metric, hash);
    if(unlikely(!m)) {
        debug(D_STATSD, "Creating new %s metric '%s'", index->name, metric);

        m = (STATSD_METRIC *)callocz(sizeof(STATSD_METRIC), 1);
        m->name = strdupz(metric);
        m->hash = hash;
        STATSD_METRIC *n = (STATSD_METRIC *)STATSD_AVL_INSERT(&index->index, (avl *)m);
        if(unlikely(n != m)) {
            freez((void *)m->name);
            freez((void *)m);
            m = n;
        }
        else {
            STATSD_FIRST_PTR_MUTEX_LOCK(index);
            index->metrics++;
            m->next = index->first;
            index->first = m;
            STATSD_FIRST_PTR_MUTEX_UNLOCK(index);
        }
    }

    index->events++;
    return m;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd parsing numbers

static inline long double statsd_parse_float(const char *v, long double def) {
    long double value = def;

    if(likely(v && *v)) {
        char *e = NULL;
        value = strtold(v, &e);
        if(e && *e)
            error("STATSD: excess data '%s' after value '%s'", e, v);
    }

    return value;
}

static inline long long statsd_parse_int(const char *v, long long def) {
    long long value = def;

    if(likely(v && *v)) {
        char *e = NULL;
        value = strtoll(v, &e, 10);
        if(e && *e)
            error("STATSD: excess data '%s' after value '%s'", e, v);
    }

    return value;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd processors per metric type

static inline void statsd_reset_metric(STATSD_METRIC *m) {
    m->reset = 0;
    m->count = 0;
    m->sampling = 0.0;
}

static inline void statsd_process_gauge(STATSD_METRIC *m, char *v, char *r) {
    if(unlikely(!v || !*v)) {
        error("STATSD: metric '%s' of type gauge, with empty value is ignored.", m->name);
        return;
    }

    if(unlikely(m->reset)) statsd_reset_metric(m);
    m->count++;
    m->sampling += statsd_parse_float(r, 1.0);

    if(*v == '+' || *v == '-')
        m->gauge.value += statsd_parse_float(v, 1.0);
    else
        m->gauge.value = statsd_parse_float(v, 1.0);
}

static inline void statsd_process_counter(STATSD_METRIC *m, char *v, char *r) {
    // we accept empty values for counters

    if(unlikely(m->reset)) statsd_reset_metric(m);
    m->count++;
    m->sampling += statsd_parse_float(r, 1.0);

    m->counter.value += statsd_parse_int(v, 1);
}

static inline void statsd_process_meter(STATSD_METRIC *m, char *v, char *r) {
    // this is the same with the counter
    statsd_process_counter(m, v, r);
}

static inline void statsd_process_histogram(STATSD_METRIC *m, char *v, char *r) {
    if(unlikely(!v || !*v)) {
        error("STATSD: metric '%s' of type histogram, with empty value is ignored.", m->name);
        return;
    }

    if(unlikely(m->reset)) {
        m->histogram.used = 0;
        statsd_reset_metric(m);
    }

    m->count++;
    m->sampling += statsd_parse_float(r, 1.0);

    if(m->histogram.used == m->histogram.size) {
        m->histogram.size += statsd.histogram_increase_step;
        m->histogram.values = reallocz(m->histogram.values, sizeof(long double) * m->histogram.size);
    }

    m->histogram.values[m->histogram.used++] = statsd_parse_float(v, 1.0);
}

static inline void statsd_process_timer(STATSD_METRIC *m, char *v, char *r) {
    if(unlikely(!v || !*v)) {
        error("STATSD: metric of type set, with empty value is ignored.");
        return;
    }

    // timers are a use case of histogram
    statsd_process_histogram(m, v, r);
}

static inline void statsd_process_set(STATSD_METRIC *m, char *v, char *r) {
    if(unlikely(!v || !*v)) {
        error("STATSD: metric of type set, with empty value is ignored.");
        return;
    }

    if(unlikely(m->reset)) {
        if(likely(m->set.dict)) {
            dictionary_destroy(m->set.dict);
            m->set.dict = NULL;
        }
        statsd_reset_metric(m);
    }

    if(unlikely(!m->set.dict)) {
        m->set.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE);
        m->set.unique = 0;
    }

    void *t = dictionary_get(m->set.dict, v);
    if(unlikely(!t)) {
        dictionary_set(m->set.dict, v, v, 1);
        m->set.unique++;
    }
}


// --------------------------------------------------------------------------------------------------------------------
// statsd parsing

static void statsd_process_metric(char *m, char *v, char *t, char *r) {
    debug(D_STATSD, "STATSD: raw metric '%s', value '%s', type '%s', rate '%s'", m, v, t, r);

    if(unlikely(!m || !*m)) return;
    if(unlikely(!t || !*t)) t = "m";

    switch (*t) {
        case 'g':
            statsd_process_gauge(
                    statsd_find_or_add_metric(&statsd.gauges, m),
                    v, r);
            break;

        case 'c':
            statsd_process_counter(
                    statsd_find_or_add_metric(&statsd.counters, m),
                    v, r);
            break;

        case 'm':
            if (t[1] == 's')
                statsd_process_timer(
                        statsd_find_or_add_metric(&statsd.timers, m),
                        v, r);
            else
                statsd_process_meter(
                        statsd_find_or_add_metric(&statsd.meters, m),
                        v, r);
            break;

        case 'h':
            statsd_process_histogram(
                    statsd_find_or_add_metric(&statsd.histograms, m),
                    v, r);
            break;

        case 's':
            statsd_process_set(
                    statsd_find_or_add_metric(&statsd.sets, m),
                    v, r);
            break;

        default:
            error("STATSD: metric '%s' with value '%s' specifies an unknown type '%s'.", m, v?v:"<unset>", t);
            break;
    }
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

static inline char *statsd_get_field(char *s, char **field, int *line_break) {
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

static inline size_t statsd_process(char *buffer, size_t size, int require_newlines) {
    (void)require_newlines;
    // FIXME: respect require_newlines to support metrics split in multiple TCP packets

    buffer[size] = '\0';
    debug(D_STATSD, "RECEIVED: '%s'", buffer);

    char *s = buffer, *m, *v, *t, *r;
    while(*s) {
        m = v = t = r = NULL;
        int line_break = 0;

        s = statsd_get_field(s, &m, &line_break);
        if(unlikely(line_break)) {
            statsd_process_metric(m, v, t, r);
            continue;
        }

        s = statsd_get_field(s, &v, &line_break);
        if(unlikely(line_break)) {
            statsd_process_metric(m, v, t, r);
            continue;
        }

        s = statsd_get_field(s, &t, &line_break);
        if(likely(line_break)) {
            statsd_process_metric(m, v, t, r);
            continue;
        }

        s = statsd_get_field(s, &r, &line_break);
        if(likely(line_break)) {
            statsd_process_metric(m, v, t, r);
            continue;
        }

        if(*s && *s != '\r' && *s != '\n') {
            error("STATSD: excess data '%s' at end of line, metric '%s', value '%s', sampling rate '%s'", s, m, v, r);

            // delete the excess data at the end if line
            while(*s && *s != '\r' && *s != '\n')
                s++;
        }

        statsd_process_metric(m, v, t, r);
    }

    return 0;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd pollfd interface

#define STATSD_TCP_BUFFER_SIZE 16384 // minimize reads
#define STATSD_UDP_BUFFER_SIZE 9000  // this should be up to MTU

struct statsd_tcp {
    size_t size;
    size_t len;
    char buffer[];
};

// new TCP client connected
static void *statsd_add_callback(int fd, short int *events) {
    (void)fd;
    *events = POLLIN;

    struct statsd_tcp *data = (struct statsd_tcp *)callocz(sizeof(struct statsd_tcp) + STATSD_TCP_BUFFER_SIZE, 1);
    data->size = STATSD_TCP_BUFFER_SIZE - 1;

    return data;
}

// TCP client disconnected
static void statsd_del_callback(int fd, void *data) {
    (void)fd;

    freez(data);

    return;
}

// Receive data
static int statsd_rcv_callback(int fd, int socktype, void *data, short int *events) {
    (void)data;

    switch(socktype) {
        case SOCK_STREAM: {
            struct statsd_tcp *d = (struct statsd_tcp *)data;
            if(unlikely(!d)) {
                error("STATSD: internal error - tcp receive buffer is null");
                return -1;
            }

            int ret = 0;
            ssize_t rc;
            do {
                rc = recv(fd, &d->buffer[d->len], d->size - d->len, MSG_DONTWAIT);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN)
                        ret = -1;
                }
                else if (!rc) {
                    // connection closed
                    error("STATSD: client disconnected.");
                    ret = -1;
                }
                else {
                    // data received
                    d->len += rc;
                }

                if(likely(d->len > 0))
                    d->len = statsd_process(d->buffer, d->len, 1);

                if(unlikely(ret == -1))
                    return -1;

            } while (rc != -1);
            break;
        }

        case SOCK_DGRAM: {
            char buffer[STATSD_UDP_BUFFER_SIZE + 1];

            ssize_t rc;
            do {
                // FIXME: collect sender information
                rc = recvfrom(fd, buffer, STATSD_UDP_BUFFER_SIZE, MSG_DONTWAIT, NULL, NULL);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN) {
                        error("STATSD: recvfrom() failed.");
                        return -1;
                    }
                } else if (rc) {
                    // data received
                    statsd_process(buffer, (size_t) rc, 0);
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

void *statsd_collector_thread(void *ptr) {
    int id = *((int *)ptr);

    info("STATSD collector thread No %d created with task id %d", id + 1, gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    poll_events(&statsd.sockets
            , statsd_add_callback
            , statsd_del_callback
            , statsd_rcv_callback
    );

    debug(D_WEB_CLIENT, "STATSD: exit!");
    listen_sockets_close(&statsd.sockets);

    pthread_exit(NULL);
    return NULL;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd main thread

void *statsd_main(void *ptr) {
    (void)ptr;

    info("STATSD main thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    statsd_listen_sockets_setup();
    if(!statsd.sockets.opened) {
        error("STATSD: No statsd sockets to listen to.");
        pthread_exit(NULL);
    }

#ifdef STATSD_MULTITHREADED
    statsd.threads = (int)config_get_number(CONFIG_SECTION_STATSD, "collector threads", processors);
    if(statsd.threads < 1) {
        error("STATSD: Invalid number of threads %d, using %d", statsd.threads, processors);
        statsd.threads = processors;
        config_set_number(CONFIG_SECTION_STATSD, "collector threads", statsd.threads);
    }
#else
    statsd.threads = 1;
#endif

    pthread_t threads[statsd.threads];
    int i;

    for(i = 0; i < statsd.threads ;i++) {
        if(pthread_create(&threads[i], NULL, statsd_collector_thread, &i))
            error("STATSD: failed to create child thread.");

        else if(pthread_detach(threads[i]))
            error("STATSD: cannot request detach of child thread.");
    }

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
    RRDDIM *rd_metrics_gauge     = rrddim_add(st_metrics, "gauges", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_counter   = rrddim_add(st_metrics, "counters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_timer     = rrddim_add(st_metrics, "timers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_meter     = rrddim_add(st_metrics, "meters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_histogram = rrddim_add(st_metrics, "histograms", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_set       = rrddim_add(st_metrics, "sets", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

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
    RRDDIM *rd_events_gauge     = rrddim_add(st_events, "gauges", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_counter   = rrddim_add(st_events, "counters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_timer     = rrddim_add(st_events, "timers", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_meter     = rrddim_add(st_events, "meters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_histogram = rrddim_add(st_events, "histograms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_set       = rrddim_add(st_events, "sets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

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

        rrddim_set_by_pointer(st_metrics, rd_metrics_gauge,     (collected_number)statsd.gauges.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_counter,   (collected_number)statsd.counters.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_timer,     (collected_number)statsd.timers.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_meter,     (collected_number)statsd.meters.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_histogram, (collected_number)statsd.histograms.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_set,       (collected_number)statsd.sets.metrics);

        rrddim_set_by_pointer(st_events, rd_events_gauge,       (collected_number)statsd.gauges.events);
        rrddim_set_by_pointer(st_events, rd_events_counter,     (collected_number)statsd.counters.events);
        rrddim_set_by_pointer(st_events, rd_events_timer,       (collected_number)statsd.timers.events);
        rrddim_set_by_pointer(st_events, rd_events_meter,       (collected_number)statsd.meters.events);
        rrddim_set_by_pointer(st_events, rd_events_histogram,   (collected_number)statsd.histograms.events);
        rrddim_set_by_pointer(st_events, rd_events_set,         (collected_number)statsd.sets.events);

        if(unlikely(netdata_exit))
            break;

        rrdset_done(st_metrics);
        rrdset_done(st_events);

        if(unlikely(netdata_exit))
            break;

    }

    for(i = 0; i < statsd.threads ;i++)
        pthread_cancel(threads[i]);

    pthread_exit(NULL);
    return NULL;
}
