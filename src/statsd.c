#include "common.h"

#define STATSD_MAX_METRIC_LENGTH 200

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
            // FIXME: not implemented yet
            m->value += value;
            break;

        case STATSD_METRIC_TYPE_METER:
            // we add to this metric
            m->value += value;
            break;

        case STATSD_METRIC_TYPE_TIMER:
            // we add time to this metric
            m->value += value;
            break;

        case STATSD_METRIC_TYPE_GAUGE:
            if(unlikely(options & STATSD_GAUGE_COLLECTION_RELATIVE))
                // we add the collected value
                m->value += value;
            else
                // we replace the value of the metric
                m->value = value;
            break;

        case STATSD_METRIC_TYPE_COUNTER:
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
// statsd main thread

void *statsd_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

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

    poll_events(&statsd_sockets
            , statsd_add_callback
            , statsd_del_callback
            , statsd_rcv_callback
    );

cleanup:
    debug(D_WEB_CLIENT, "STATSD: exit!");
    listen_sockets_close(&statsd_sockets);

    pthread_exit(NULL);
    return NULL;
}
