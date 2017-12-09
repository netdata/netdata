#include "common.h"

#define STATSD_CHART_PREFIX "statsd"
#define STATSD_CHART_PRIORITY 90000

// --------------------------------------------------------------------------------------

// #define STATSD_MULTITHREADED 1

#ifdef STATSD_MULTITHREADED
// DO NOT ENABLE MULTITHREADING - IT IS NOT WELL TESTED
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

#define STATSD_DECIMAL_DETAIL 1000 // floating point values get multiplied by this, with the same divisor

// --------------------------------------------------------------------------------------------------------------------
// data specific to each metric type

typedef struct statsd_metric_gauge {
    long double value;
} STATSD_METRIC_GAUGE;

typedef struct statsd_metric_counter { // counter and meter
    long long value;
} STATSD_METRIC_COUNTER;

typedef struct statsd_histogram_extensions {
    netdata_mutex_t mutex;

    // average is stored in metric->last
    collected_number last_min;
    collected_number last_max;
    collected_number last_percentile;
    collected_number last_median;
    collected_number last_stddev;
    collected_number last_sum;

    int zeroed;

    RRDDIM *rd_min;
    RRDDIM *rd_max;
    RRDDIM *rd_percentile;
    RRDDIM *rd_median;
    RRDDIM *rd_stddev;
    RRDDIM *rd_sum;

    size_t size;
    size_t used;
    long double *values;   // dynamic array of values collected
} STATSD_METRIC_HISTOGRAM_EXTENSIONS;

typedef struct statsd_metric_histogram { // histogram and timer
    STATSD_METRIC_HISTOGRAM_EXTENSIONS *ext;
} STATSD_METRIC_HISTOGRAM;

typedef struct statsd_metric_set {
    DICTIONARY *dict;
    size_t unique;
} STATSD_METRIC_SET;


// --------------------------------------------------------------------------------------------------------------------
// this is a metric - for all types of metrics

typedef enum statsd_metric_options {
    STATSD_METRIC_OPTION_NONE                         = 0x00000000, // no options set
    STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED = 0x00000001, // do not update the chart dimension, when this metric is not collected
    STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED        = 0x00000002, // render a private chart for this metric
    STATSD_METRIC_OPTION_PRIVATE_CHART_CHECKED        = 0x00000004, // the metric has been checked if it should get private chart or not
    STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT        = 0x00000008, // show the count of events for this private chart
    STATSD_METRIC_OPTION_CHECKED_IN_APPS              = 0x00000010, // set when this metric has been checked agains apps
} STATS_METRIC_OPTIONS;

typedef enum statsd_metric_type {
    STATSD_METRIC_TYPE_GAUGE,
    STATSD_METRIC_TYPE_COUNTER,
    STATSD_METRIC_TYPE_METER,
    STATSD_METRIC_TYPE_TIMER,
    STATSD_METRIC_TYPE_HISTOGRAM,
    STATSD_METRIC_TYPE_SET
} STATSD_METRIC_TYPE;


typedef struct statsd_metric {
    avl avl;                        // indexing

    const char *name;               // the name of the metric
    uint32_t hash;                  // hash of the name

    STATSD_METRIC_TYPE type;

    // metadata about data collection
    collected_number events;        // the number of times this metric has been collected (never resets)
    size_t count;                   // the number of times this metric has been collected since the last flush

    // the actual collected data
    union {
        STATSD_METRIC_GAUGE gauge;
        STATSD_METRIC_COUNTER counter;
        STATSD_METRIC_HISTOGRAM histogram;
        STATSD_METRIC_SET set;
    };

    // chart related members
    STATS_METRIC_OPTIONS options;   // STATSD_METRIC_OPTION_* (bitfield)
    char reset;                     // set to 1 to reset this metric to zero
    collected_number last;          // the last value sent to netdata
    RRDSET *st;                     // the chart of this metric
    RRDDIM *rd_value;               // the dimension of this metric value
    RRDDIM *rd_count;               // the dimension for the number of events received

    // linking, used for walking through all metrics
    struct statsd_metric *next;
} STATSD_METRIC;


// --------------------------------------------------------------------------------------------------------------------
// each type of metric has its own index

typedef struct statsd_index {
    char *name;                     // the name of the index of metrics
    size_t events;                  // the number of events processed for this index
    size_t metrics;                 // the number of metrics in this index

    STATSD_AVL_TREE index;          // the AVL tree

    STATSD_METRIC *first;           // the linked list of metrics (new metrics are added in front)
    STATSD_FIRST_PTR_MUTEX;         // when mutli-threading is enabled, a lock to protect the linked list

    STATS_METRIC_OPTIONS default_options;  // default options for all metrics in this index
} STATSD_INDEX;

static int statsd_metric_compare(void* a, void* b);

// --------------------------------------------------------------------------------------------------------------------
// synthetic charts

typedef enum statsd_app_chart_dimension_value_type {
    STATSD_APP_CHART_DIM_VALUE_TYPE_EVENTS,
    STATSD_APP_CHART_DIM_VALUE_TYPE_LAST,
    STATSD_APP_CHART_DIM_VALUE_TYPE_AVERAGE,
    STATSD_APP_CHART_DIM_VALUE_TYPE_SUM,
    STATSD_APP_CHART_DIM_VALUE_TYPE_MIN,
    STATSD_APP_CHART_DIM_VALUE_TYPE_MAX,
    STATSD_APP_CHART_DIM_VALUE_TYPE_PERCENTILE,
    STATSD_APP_CHART_DIM_VALUE_TYPE_MEDIAN,
    STATSD_APP_CHART_DIM_VALUE_TYPE_STDDEV
} STATSD_APP_CHART_DIM_VALUE_TYPE;

typedef struct statsd_app_chart_dimension {
    const char *name;
    const char *metric;
    uint32_t metric_hash;
    collected_number multiplier;
    collected_number divisor;
    STATSD_APP_CHART_DIM_VALUE_TYPE value_type;

    RRDDIM *rd;
    collected_number *value_ptr;
    RRD_ALGORITHM algorithm;

    struct statsd_app_chart_dimension *next;
} STATSD_APP_CHART_DIM;

typedef struct statsd_app_chart {
    const char *source;
    const char *id;
    const char *name;
    const char *title;
    const char *family;
    const char *context;
    const char *units;
    long priority;
    RRDSET_TYPE chart_type;
    STATSD_APP_CHART_DIM *dimensions;
    size_t dimensions_count;
    size_t dimensions_linked_count;

    RRDSET *st;
    struct statsd_app_chart *next;
} STATSD_APP_CHART;

typedef struct statsd_app {
    const char *name;
    SIMPLE_PATTERN *metrics;
    STATS_METRIC_OPTIONS default_options;
    RRD_MEMORY_MODE rrd_memory_mode;
    long rrd_history_entries;

    const char *source;
    STATSD_APP_CHART *charts;
    struct statsd_app *next;
} STATSD_APP;

// --------------------------------------------------------------------------------------------------------------------
// global statsd data

static struct statsd {
    STATSD_INDEX gauges;
    STATSD_INDEX counters;
    STATSD_INDEX timers;
    STATSD_INDEX histograms;
    STATSD_INDEX meters;
    STATSD_INDEX sets;
    size_t unknown_types;
    size_t socket_errors;
    size_t tcp_socket_reads;
    size_t tcp_packets_received;
    size_t tcp_bytes_read;
    size_t udp_socket_reads;
    size_t udp_packets_received;
    size_t udp_bytes_read;

    int enabled;
    int update_every;
    SIMPLE_PATTERN *charts_for;

    size_t decimal_detail;
    size_t private_charts;
    size_t max_private_charts;
    size_t max_private_charts_hard;
    RRD_MEMORY_MODE private_charts_memory_mode;
    long private_charts_rrd_history_entries;

    STATSD_APP *apps;
    size_t recvmmsg_size;
    size_t histogram_increase_step;
    double histogram_percentile;
    char *histogram_percentile_str;
    int threads;
    LISTEN_SOCKETS sockets;
} statsd = {
        .enabled = 1,
        .max_private_charts = 200,
        .max_private_charts_hard = 1000,
        .recvmmsg_size = 10,
        .decimal_detail = STATSD_DECIMAL_DETAIL,

        .gauges     = {
                .name = "gauge",
                .events = 0,
                .metrics = 0,
                .index = STATSD_AVL_INDEX_INIT,
                .default_options = STATSD_METRIC_OPTION_NONE,
                .first = NULL,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .counters   = {
                .name = "counter",
                .events = 0,
                .metrics = 0,
                .index = STATSD_AVL_INDEX_INIT,
                .default_options = STATSD_METRIC_OPTION_NONE,
                .first = NULL,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .timers     = {
                .name = "timer",
                .events = 0,
                .metrics = 0,
                .index = STATSD_AVL_INDEX_INIT,
                .default_options = STATSD_METRIC_OPTION_NONE,
                .first = NULL,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .histograms = {
                .name = "histogram",
                .events = 0,
                .metrics = 0,
                .index = STATSD_AVL_INDEX_INIT,
                .default_options = STATSD_METRIC_OPTION_NONE,
                .first = NULL,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .meters     = {
                .name = "meter",
                .events = 0,
                .metrics = 0,
                .index = STATSD_AVL_INDEX_INIT,
                .default_options = STATSD_METRIC_OPTION_NONE,
                .first = NULL,
                STATSD_FIRST_PTR_MUTEX_INIT
        },
        .sets       = {
                .name = "set",
                .events = 0,
                .metrics = 0,
                .index = STATSD_AVL_INDEX_INIT,
                .default_options = STATSD_METRIC_OPTION_NONE,
                .first = NULL,
                STATSD_FIRST_PTR_MUTEX_INIT
        },

        .apps = NULL,
        .histogram_percentile = 95.0,
        .histogram_increase_step = 10,
        .threads = 0,
        .sockets = {
                .config_section  = CONFIG_SECTION_STATSD,
                .default_bind_to = "udp:localhost tcp:localhost",
                .default_port    = STATSD_LISTEN_PORT,
                .backlog         = STATSD_LISTEN_BACKLOG
        },
};


// --------------------------------------------------------------------------------------------------------------------
// statsd index management - add/find metrics

static int statsd_metric_compare(void* a, void* b) {
    if(((STATSD_METRIC *)a)->hash < ((STATSD_METRIC *)b)->hash) return -1;
    else if(((STATSD_METRIC *)a)->hash > ((STATSD_METRIC *)b)->hash) return 1;
    else return strcmp(((STATSD_METRIC *)a)->name, ((STATSD_METRIC *)b)->name);
}

static inline STATSD_METRIC *stasd_metric_index_find(STATSD_INDEX *index, const char *name, uint32_t hash) {
    STATSD_METRIC tmp;
    tmp.name = name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (STATSD_METRIC *)STATSD_AVL_SEARCH(&index->index, (avl *)&tmp);
}

static inline STATSD_METRIC *statsd_find_or_add_metric(STATSD_INDEX *index, const char *name, STATSD_METRIC_TYPE type) {
    debug(D_STATSD, "searching for metric '%s' under '%s'", name, index->name);

    uint32_t hash = simple_hash(name);

    STATSD_METRIC *m = stasd_metric_index_find(index, name, hash);
    if(unlikely(!m)) {
        debug(D_STATSD, "Creating new %s metric '%s'", index->name, name);

        m = (STATSD_METRIC *)callocz(sizeof(STATSD_METRIC), 1);
        m->name = strdupz(name);
        m->hash = hash;
        m->type = type;
        m->options = index->default_options;

        if(type == STATSD_METRIC_TYPE_HISTOGRAM || type == STATSD_METRIC_TYPE_TIMER) {
            m->histogram.ext = callocz(sizeof(STATSD_METRIC_HISTOGRAM_EXTENSIONS), 1);
            netdata_mutex_init(&m->histogram.ext->mutex);
        }
        STATSD_METRIC *n = (STATSD_METRIC *)STATSD_AVL_INSERT(&index->index, (avl *)m);
        if(unlikely(n != m)) {
            freez((void *)m->histogram.ext);
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
    long double value;

    if(likely(v && *v)) {
        char *e = NULL;
        value = str2ld(v, &e);
        if(unlikely(e && *e))
            error("STATSD: excess data '%s' after value '%s'", e, v);
    }
    else
        value = def;

    return value;
}

static inline long long statsd_parse_int(const char *v, long long def) {
    long long value;

    if(likely(v && *v)) {
        char *e = NULL;
        value = str2ll(v, &e);
        if(unlikely(e && *e))
            error("STATSD: excess data '%s' after value '%s'", e, v);
    }
    else
        value = def;

    return value;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd processors per metric type

static inline void statsd_reset_metric(STATSD_METRIC *m) {
    m->reset = 0;
    m->count = 0;
}

static inline void statsd_process_gauge(STATSD_METRIC *m, const char *value, const char *sampling) {
    if(unlikely(!value || !*value)) {
        error("STATSD: metric '%s' of type gauge, with empty value is ignored.", m->name);
        return;
    }

    if(unlikely(m->reset)) {
        // no need to reset anything specific for gauges
        statsd_reset_metric(m);
    }

    if(unlikely(*value == '+' || *value == '-'))
        m->gauge.value += statsd_parse_float(value, 1.0) / statsd_parse_float(sampling, 1.0);
    else
        m->gauge.value = statsd_parse_float(value, 1.0) / statsd_parse_float(sampling, 1.0);

    m->events++;
    m->count++;
}

static inline void statsd_process_counter(STATSD_METRIC *m, const char *value, const char *sampling) {
    // we accept empty values for counters

    if(unlikely(m->reset)) statsd_reset_metric(m);

    m->counter.value += llrintl((long double)statsd_parse_int(value, 1) / statsd_parse_float(sampling, 1.0));

    m->events++;
    m->count++;
}

static inline void statsd_process_meter(STATSD_METRIC *m, const char *value, const char *sampling) {
    // this is the same with the counter
    statsd_process_counter(m, value, sampling);
}

static inline void statsd_process_histogram(STATSD_METRIC *m, const char *value, const char *sampling) {
    if(unlikely(!value || !*value)) {
        error("STATSD: metric '%s' of type histogram, with empty value is ignored.", m->name);
        return;
    }

    if(unlikely(m->reset)) {
        m->histogram.ext->used = 0;
        statsd_reset_metric(m);
    }

    if(unlikely(m->histogram.ext->used == m->histogram.ext->size)) {
        netdata_mutex_lock(&m->histogram.ext->mutex);
        m->histogram.ext->size += statsd.histogram_increase_step;
        m->histogram.ext->values = reallocz(m->histogram.ext->values, sizeof(long double) * m->histogram.ext->size);
        netdata_mutex_unlock(&m->histogram.ext->mutex);
    }

    m->histogram.ext->values[m->histogram.ext->used++] = statsd_parse_float(value, 1.0) / statsd_parse_float(sampling, 1.0);

    m->events++;
    m->count++;
}

static inline void statsd_process_timer(STATSD_METRIC *m, const char *value, const char *sampling) {
    if(unlikely(!value || !*value)) {
        error("STATSD: metric of type timer, with empty value is ignored.");
        return;
    }

    // timers are a use case of histogram
    statsd_process_histogram(m, value, sampling);
}

static inline void statsd_process_set(STATSD_METRIC *m, const char *value) {
    if(unlikely(!value || !*value)) {
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

    void *t = dictionary_get(m->set.dict, value);
    if(unlikely(!t)) {
        dictionary_set(m->set.dict, value, NULL, 1);
        m->set.unique++;
    }

    m->events++;
    m->count++;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd parsing

static void statsd_process_metric(const char *name, const char *value, const char *type, const char *sampling) {
    debug(D_STATSD, "STATSD: raw metric '%s', value '%s', type '%s', rate '%s'", name?name:"(null)", value?value:"(null)", type?type:"(null)", sampling?sampling:"(null)");

    if(unlikely(!name || !*name)) return;
    if(unlikely(!type || !*type)) type = "m";

    char t0 = type[0], t1 = type[1];

    if(unlikely(t0 == 'g' && t1 == '\0')) {
        statsd_process_gauge(
                statsd_find_or_add_metric(&statsd.gauges, name, STATSD_METRIC_TYPE_GAUGE),
                value, sampling);
    }
    else if(unlikely((t0 == 'c' || t0 == 'C') && t1 == '\0')) {
        // etsy/statsd uses 'c'
        // brubeck     uses 'C'
        statsd_process_counter(
                statsd_find_or_add_metric(&statsd.counters, name, STATSD_METRIC_TYPE_COUNTER),
                value, sampling);
    }
    else if(unlikely(t0 == 'm' && t1 == '\0')) {
        statsd_process_meter(
                statsd_find_or_add_metric(&statsd.meters, name, STATSD_METRIC_TYPE_METER),
                value, sampling);
    }
    else if(unlikely(t0 == 'h' && t1 == '\0')) {
        statsd_process_histogram(
                statsd_find_or_add_metric(&statsd.histograms, name, STATSD_METRIC_TYPE_HISTOGRAM),
                value, sampling);
    }
    else if(unlikely(t0 == 's' && t1 == '\0')) {
        statsd_process_set(
                statsd_find_or_add_metric(&statsd.sets, name, STATSD_METRIC_TYPE_SET),
                value);
    }
    else if(unlikely(t0 == 'm' && t1 == 's' && type[2] == '\0')) {
        statsd_process_timer(
                statsd_find_or_add_metric(&statsd.timers, name, STATSD_METRIC_TYPE_TIMER),
                value, sampling);
    }
    else {
        statsd.unknown_types++;
        error("STATSD: metric '%s' with value '%s' is sent with unknown metric type '%s'", name, value?value:"", type);
    }
}

static inline const char *statsd_parse_skip_up_to(const char *s, char d1, char d2) {
    char c;

    for(c = *s; c && c != d1 && c != d2 && c != '\r' && c != '\n'; c = *++s) ;

    return s;
}

const char *statsd_parse_skip_spaces(const char *s) {
    char c;

    for(c = *s; c && ( c == ' ' || c == '\t' || c == '\r' || c == '\n' ); c = *++s) ;

    return s;
}

static inline const char *statsd_parse_field_trim(const char *start, char *end) {
    if(unlikely(!start)) {
        start = end;
        return start;
    }

    while(start <= end && (*start == ' ' || *start == '\t'))
        start++;

    *end = '\0';
    end--;
    while(end >= start && (*end == ' ' || *end == '\t'))
        *end-- = '\0';

    return start;
}

static inline size_t statsd_process(char *buffer, size_t size, int require_newlines) {
    buffer[size] = '\0';
    debug(D_STATSD, "RECEIVED: %zu bytes: '%s'", size, buffer);

    const char *s = buffer;
    while(*s) {
        const char *name = NULL, *value = NULL, *type = NULL, *sampling = NULL;
        char *name_end = NULL, *value_end = NULL, *type_end = NULL, *sampling_end = NULL;

        s = name_end = (char *)statsd_parse_skip_up_to(name = s, ':', '|');
        if(name == name_end) {
            s = statsd_parse_skip_spaces(s);
            continue;
        }

        if(likely(*s == ':'))
            s = value_end = (char *) statsd_parse_skip_up_to(value = ++s, '|', '|');

        if(likely(*s == '|'))
            s = type_end = (char *) statsd_parse_skip_up_to(type = ++s, '|', '@');

        if(likely(*s == '|' || *s == '@')) {
            s = sampling_end = (char *) statsd_parse_skip_up_to(sampling = ++s, '\r', '\n');
            if(*sampling == '@') sampling++;
        }

        // skip everything until the end of the line
        while(*s && *s != '\n') s++;

        if(unlikely(require_newlines && *s != '\n' && s > buffer)) {
            // move the remaining data to the beginning
            size -= (name - buffer);
            memmove(buffer, name, size);
            return size;
        }
        else
            s = statsd_parse_skip_spaces(s);

        statsd_process_metric(
                  statsd_parse_field_trim(name, name_end)
                , statsd_parse_field_trim(value, value_end)
                , statsd_parse_field_trim(type, type_end)
                , statsd_parse_field_trim(sampling, sampling_end)
        );
    }

    return 0;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd pollfd interface

#define STATSD_TCP_BUFFER_SIZE 65536 // minimize tcp reads
#define STATSD_UDP_BUFFER_SIZE 9000  // this should be up to MTU

typedef enum {
    STATSD_SOCKET_DATA_TYPE_TCP,
    STATSD_SOCKET_DATA_TYPE_UDP
} STATSD_SOCKET_DATA_TYPE;

struct statsd_tcp {
    STATSD_SOCKET_DATA_TYPE type;
    size_t size;
    size_t len;
    char buffer[];
};

#ifdef HAVE_RECVMMSG
struct statsd_udp {
    STATSD_SOCKET_DATA_TYPE type;
    size_t size;
    struct iovec *iovecs;
    struct mmsghdr *msgs;
};
#else
struct statsd_udp {
    STATSD_SOCKET_DATA_TYPE type;
    char buffer[STATSD_UDP_BUFFER_SIZE];
};
#endif

// new TCP client connected
static void *statsd_add_callback(int fd, int socktype, short int *events) {
    (void)fd;
    (void)socktype;
    *events = POLLIN;

    struct statsd_tcp *data = (struct statsd_tcp *)callocz(sizeof(struct statsd_tcp) + STATSD_TCP_BUFFER_SIZE, 1);
    data->type = STATSD_SOCKET_DATA_TYPE_TCP;
    data->size = STATSD_TCP_BUFFER_SIZE - 1;

    return data;
}

// TCP client disconnected
static void statsd_del_callback(int fd, int socktype, void *data) {
    (void)fd;
    (void)socktype;

    if(data) {
        struct statsd_tcp *t = data;
        if(t->type == STATSD_SOCKET_DATA_TYPE_TCP) {
            if(t->len != 0) {
                statsd.socket_errors++;
                error("STATSD: client is probably sending unterminated metrics. Closed socket left with '%s'. Trying to process it.", t->buffer);
                statsd_process(t->buffer, t->len, 0);
            }
        }
        else
            error("STATSD: internal error: received socket data type is %d, but expected %d", (int)t->type, (int)STATSD_SOCKET_DATA_TYPE_TCP);

        freez(data);
    }

    return;
}

// Receive data
static int statsd_rcv_callback(int fd, int socktype, void *data, short int *events) {
    *events = POLLIN;

    switch(socktype) {
        case SOCK_STREAM: {
            struct statsd_tcp *d = (struct statsd_tcp *)data;
            if(unlikely(!d)) {
                error("STATSD: internal error: expected TCP data pointer is NULL");
                statsd.socket_errors++;
                return -1;
            }

#ifdef NETDATA_INTERNAL_CHECKS
            if(unlikely(d->type != STATSD_SOCKET_DATA_TYPE_TCP)) {
                error("STATSD: internal error: socket data type should be %d, but it is %d", (int)STATSD_SOCKET_DATA_TYPE_TCP, (int)d->type);
                statsd.socket_errors++;
                return -1;
            }
#endif

            int ret = 0;
            ssize_t rc;
            do {
                rc = recv(fd, &d->buffer[d->len], d->size - d->len, MSG_DONTWAIT);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
                        error("STATSD: recv() on TCP socket %d failed.", fd);
                        statsd.socket_errors++;
                        ret = -1;
                    }
                }
                else if (!rc) {
                    // connection closed
                    debug(D_STATSD, "STATSD: client disconnected.");
                    ret = -1;
                }
                else {
                    // data received
                    d->len += rc;
                    statsd.tcp_socket_reads++;
                    statsd.tcp_bytes_read += rc;
                }

                if(likely(d->len > 0)) {
                    statsd.tcp_packets_received++;
                    d->len = statsd_process(d->buffer, d->len, 1);
                }

                if(unlikely(ret == -1))
                    return -1;

            } while (rc != -1);
            break;
        }

        case SOCK_DGRAM: {
            struct statsd_udp *d = (struct statsd_udp *)data;
            if(unlikely(!d)) {
                error("STATSD: internal error: expected UDP data pointer is NULL");
                statsd.socket_errors++;
                return -1;
            }

#ifdef NETDATA_INTERNAL_CHECKS
            if(unlikely(d->type != STATSD_SOCKET_DATA_TYPE_UDP)) {
                error("STATSD: internal error: socket data should be %d, but it is %d", (int)d->type, (int)STATSD_SOCKET_DATA_TYPE_UDP);
                statsd.socket_errors++;
                return -1;
            }
#endif

#ifdef HAVE_RECVMMSG
            ssize_t rc;
            do {
                rc = recvmmsg(fd, d->msgs, (unsigned int)d->size, MSG_DONTWAIT, NULL);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
                        error("STATSD: recvmmsg() on UDP socket %d failed.", fd);
                        statsd.socket_errors++;
                        return -1;
                    }
                } else if (rc) {
                    // data received
                    statsd.udp_socket_reads++;
                    statsd.udp_packets_received += rc;

                    size_t i;
                    for (i = 0; i < (size_t)rc; ++i) {
                        size_t len = (size_t)d->msgs[i].msg_len;
                        statsd.udp_bytes_read += len;
                        statsd_process(d->msgs[i].msg_hdr.msg_iov->iov_base, len, 0);
                    }
                }
            } while (rc != -1);

#else // !HAVE_RECVMMSG
            ssize_t rc;
            do {
                rc = recv(fd, d->buffer, STATSD_UDP_BUFFER_SIZE - 1, MSG_DONTWAIT);
                if (rc < 0) {
                    // read failed
                    if (errno != EWOULDBLOCK && errno != EAGAIN && errno != EINTR) {
                        error("STATSD: recv() on UDP socket %d failed.", fd);
                        statsd.socket_errors++;
                        return -1;
                    }
                } else if (rc) {
                    // data received
                    statsd.udp_socket_reads++;
                    statsd.udp_packets_received++;
                    statsd.udp_bytes_read += rc;
                    statsd_process(d->buffer, (size_t) rc, 0);
                }
            } while (rc != -1);
#endif

            break;
        }

        default: {
            error("STATSD: internal error: unknown socktype %d on socket %d", socktype, fd);
            statsd.socket_errors++;
            return -1;
        }
    }

    return 0;
}

static int statsd_snd_callback(int fd, int socktype, void *data, short int *events) {
    (void)fd;
    (void)socktype;
    (void)data;
    (void)events;

    error("STATSD: snd_callback() called, but we never requested to send data to statsd clients.");
    return -1;
}

// --------------------------------------------------------------------------------------------------------------------
// statsd child thread to collect metrics from network

void statsd_collector_thread_cleanup(void *data) {
    struct statsd_udp *d = data;

#ifdef HAVE_RECVMMSG
    size_t i;
    for (i = 0; i < d->size; i++)
        freez(d->iovecs[i].iov_base);

    freez(d->iovecs);
    freez(d->msgs);
#endif

    freez(d);
}

void *statsd_collector_thread(void *ptr) {
    int id = *((int *)ptr);

    info("STATSD collector thread No %d created with task id %d", id + 1, gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    struct statsd_udp *d = callocz(sizeof(struct statsd_udp), 1);
    pthread_cleanup_push(statsd_collector_thread_cleanup, d);

#ifdef HAVE_RECVMMSG
    d->type = STATSD_SOCKET_DATA_TYPE_UDP;
    d->size = statsd.recvmmsg_size;
    d->iovecs = callocz(sizeof(struct iovec), d->size);
    d->msgs = callocz(sizeof(struct mmsghdr), d->size);

    size_t i;
    for (i = 0; i < d->size; i++) {
        d->iovecs[i].iov_base = mallocz(STATSD_UDP_BUFFER_SIZE);
        d->iovecs[i].iov_len = STATSD_UDP_BUFFER_SIZE - 1;
        d->msgs[i].msg_hdr.msg_iov = &d->iovecs[i];
        d->msgs[i].msg_hdr.msg_iovlen = 1;
    }
#endif

    poll_events(&statsd.sockets
            , statsd_add_callback
            , statsd_del_callback
            , statsd_rcv_callback
            , statsd_snd_callback
            , NULL
            , (void *)d
    );

    pthread_cleanup_pop(1);

    debug(D_WEB_CLIENT, "STATSD: exit!");
    pthread_exit(NULL);
    return NULL;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd applications configuration files parsing

#define STATSD_CONF_LINE_MAX 8192

int statsd_readfile(const char *path, const char *filename) {
    debug(D_STATSD, "STATSD configuration reading file '%s/%s'", path, filename);

    char buffer[STATSD_CONF_LINE_MAX + 1];

    FILE *fp = NULL;
    snprintfz(buffer, STATSD_CONF_LINE_MAX, "%s/%s", path, filename);
    fp = fopen(buffer, "r");
    if(!fp) {
        error("STATSD: cannot open file '%s'.", buffer);
        return -1;
    }

    STATSD_APP *app = NULL;
    STATSD_APP_CHART *chart = NULL;

    size_t line = 0;
    char *s;
    while(fgets(buffer, STATSD_CONF_LINE_MAX, fp) != NULL) {
        buffer[STATSD_CONF_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if (!s || *s == '#') {
            debug(D_STATSD, "STATSD: ignoring line %zu of file '%s/%s', it is empty.", line, path, filename);
            continue;
        }
        debug(D_STATSD, "STATSD: processing line %zu of file '%s/%s': %s", line, path, filename, buffer);

        int len = (int) strlen(s);
        if (*s == '[' && s[len - 1] == ']') {
            // new section
            s[len - 1] = '\0';
            s++;

            if (!strcmp(s, "app")) {
                // a new app
                app = callocz(sizeof(STATSD_APP), 1);
                app->name = strdupz("unnamed");
                app->rrd_memory_mode = localhost->rrd_memory_mode;
                app->rrd_history_entries = localhost->rrd_history_entries;

                app->next = statsd.apps;
                statsd.apps = app;
                chart = NULL;
            }
            else if(app) {
                // a new chart
                chart = callocz(sizeof(STATSD_APP_CHART), 1);
                netdata_fix_chart_id(s);
                chart->id = strdupz(s);
                chart->name = strdupz(s);
                chart->title = strdupz("Statsd chart");
                chart->context = strdupz(s);
                chart->family = strdupz("overview");
                chart->units = strdupz("value");
                chart->priority = STATSD_CHART_PRIORITY;
                chart->chart_type = RRDSET_TYPE_LINE;

                chart->next = app->charts;
                app->charts = chart;
            }
            else
                error("STATSD: ignoring line %zu ('%s') of file '%s/%s', [app] is not defined.", line, s, path, filename);

            continue;
        }

        if(!app) {
            error("STATSD: ignoring line %zu ('%s') of file '%s/%s', it is outside all sections.", line, s, path, filename);
            continue;
        }

        char *name = s;
        char *value = strchr(s, '=');
        if(!value) {
            error("STATSD: ignoring line %zu ('%s') of file '%s/%s', there is no = in it.", line, s, path, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(!name || *name == '#') {
            error("STATSD: ignoring line %zu of file '%s/%s', name is empty.", line, path, filename);
            continue;
        }
        if(!value) {
            debug(D_CONFIG, "STATSD: ignoring line %zu of file '%s/%s', value is empty.", line, path, filename);
            continue;
        }

        if(!chart) {
            if(!strcmp(name, "name")) {
                freez((void *)app->name);
                netdata_fix_chart_name(value);
                app->name = strdupz(value);
            }
            else if (!strcmp(name, "metrics")) {
                simple_pattern_free(app->metrics);
                app->metrics = simple_pattern_create(value, SIMPLE_PATTERN_EXACT);
            }
            else if (!strcmp(name, "private charts")) {
                if (!strcmp(value, "yes") || !strcmp(value, "on"))
                    app->default_options |= STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;
                else
                    app->default_options &= ~STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;
            }
            else if (!strcmp(name, "gaps when not collected")) {
                if (!strcmp(value, "yes") || !strcmp(value, "on"))
                    app->default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;
            }
            else if (!strcmp(name, "memory mode")) {
                app->rrd_memory_mode = rrd_memory_mode_id(value);
            }
            else if (!strcmp(name, "history")) {
                app->rrd_history_entries = atol(value);
                if (app->rrd_history_entries < 5)
                    app->rrd_history_entries = 5;
            }
            else {
                error("STATSD: ignoring line %zu ('%s') of file '%s/%s'. Unknown keyword for the [app] section.", line, name, path, filename);
                continue;
            }
        }
        else {
            if(!strcmp(name, "name")) {
                freez((void *)chart->name);
                netdata_fix_chart_id(value);
                chart->name = strdupz(value);
            }
            else if(!strcmp(name, "title")) {
                freez((void *)chart->title);
                chart->title = strdupz(value);
            }
            else if (!strcmp(name, "family")) {
                freez((void *)chart->family);
                chart->family = strdupz(value);
            }
            else if (!strcmp(name, "context")) {
                freez((void *)chart->context);
                netdata_fix_chart_id(value);
                chart->context = strdupz(value);
            }
            else if (!strcmp(name, "units")) {
                freez((void *)chart->units);
                chart->units = strdupz(value);
            }
            else if (!strcmp(name, "priority")) {
                chart->priority = atol(value);
            }
            else if (!strcmp(name, "type")) {
                chart->chart_type = rrdset_type_id(value);
            }
            else if (!strcmp(name, "dimension")) {
                // metric [name [type [multiplier [divisor]]]]
                char *words[5];
                pluginsd_split_words(value, words, 5);

                char *metric_name = words[0];
                char *dim_name = words[1];
                char *type = words[2];
                char *multipler = words[3];
                char *divisor = words[4];

                STATSD_APP_CHART_DIM *dim = callocz(sizeof(STATSD_APP_CHART_DIM), 1);

                dim->metric = strdupz(metric_name);
                dim->metric_hash = simple_hash(dim->metric);

                dim->name = strdupz((dim_name && *dim_name)?dim_name:metric_name);
                dim->multiplier = (multipler && *multipler)?str2l(multipler):1;
                dim->divisor = (divisor && *divisor)?str2l(divisor):1;

                if(!type || !*type) type = "last";
                if(!strcmp(type, "events")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_EVENTS;
                else if(!strcmp(type, "last")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_LAST;
                else if(!strcmp(type, "min")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_MIN;
                else if(!strcmp(type, "max")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_MAX;
                else if(!strcmp(type, "sum")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_SUM;
                else if(!strcmp(type, "average")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_AVERAGE;
                else if(!strcmp(type, "median")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_MEDIAN;
                else if(!strcmp(type, "stddev")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_STDDEV;
                else if(!strcmp(type, "percentile")) dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_PERCENTILE;
                else {
                    error("STATSD: invalid type '%s' at line %zu of file '%s/%s'. Using 'last'.", type, line, path, filename);
                    dim->value_type = STATSD_APP_CHART_DIM_VALUE_TYPE_LAST;
                }

                if(!dim->multiplier) {
                    error("STATSD: invalid multiplier value '%s' at line %zu of file '%s/%s'. Using 1.", multipler, line, path, filename);
                    dim->multiplier = 1;
                }
                if(!dim->divisor) {
                    error("STATSD: invalid divisor value '%s' at line %zu of file '%s/%s'. Using 1.", divisor, line, path, filename);
                    dim->divisor = 1;
                }

                // append it to the list of dimension
                STATSD_APP_CHART_DIM *tdim;
                for(tdim = chart->dimensions; tdim && tdim->next ; tdim = tdim->next) ;
                if(!tdim) {
                    dim->next = chart->dimensions;
                    chart->dimensions = dim;
                }
                else {
                    dim->next = tdim->next;
                    tdim->next = dim;
                }
                chart->dimensions_count++;

                debug(D_STATSD, "Added dimension '%s' to chart '%s' of app '%s', for metric '%s', with type %u, multiplier " COLLECTED_NUMBER_FORMAT ", divisor " COLLECTED_NUMBER_FORMAT,
                    dim->name, chart->id, app->name, dim->metric, dim->value_type, dim->multiplier, dim->divisor);
            }
            else {
                error("STATSD: ignoring line %zu ('%s') of file '%s/%s'. Unknown keyword for the [%s] section.", line, name, path, filename, chart->id);
                continue;
            }
        }
    }

    fclose(fp);
    return 0;
}

static void statsd_readdir(const char *path) {
    size_t pathlen = strlen(path);

    debug(D_STATSD, "STATSD configuration reading directory '%s'", path);

    DIR *dir = opendir(path);
    if (!dir) {
        error("STATSD configuration cannot open directory '%s'.", path);
        return;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        size_t len = strlen(de->d_name);

        if(de->d_type == DT_DIR
           && (
                   (de->d_name[0] == '.' && de->d_name[1] == '\0')
                   || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
           )) {
            debug(D_STATSD, "STATSD: ignoring directory '%s'", de->d_name);
            continue;
        }

        else if(de->d_type == DT_DIR) {
            char *s = mallocz(pathlen + strlen(de->d_name) + 2);
            strcpy(s, path);
            strcat(s, "/");
            strcat(s, de->d_name);
            statsd_readdir(s);
            freez(s);
            continue;
        }

        else if((de->d_type == DT_LNK || de->d_type == DT_REG || de->d_type == DT_UNKNOWN) &&
                len > 5 && !strcmp(&de->d_name[len - 5], ".conf")) {
            statsd_readfile(path, de->d_name);
        }

        else debug(D_STATSD, "STATSD: ignoring file '%s'", de->d_name);
    }

    closedir(dir);
}

// --------------------------------------------------------------------------------------------------------------------
// send metrics to netdata - in private charts - called from the main thread

// extract chart type and chart id from metric name
static inline void statsd_get_metric_type_and_id(STATSD_METRIC *m, char *type, char *id, const char *defid, size_t len) {
    char *s;

    snprintfz(type, len, "%s_%s_%s", STATSD_CHART_PREFIX, defid, m->name);
    for(s = type; *s ;s++)
        if(unlikely(*s == '.')) break;

    if(*s == '.') {
        *s++ = '\0';
        strncpyz(id, s, len);
    }
    else {
        strncpyz(id, defid, len);
    }

    netdata_fix_chart_id(type);
    netdata_fix_chart_id(id);
}

static inline RRDSET *statsd_private_rrdset_create(
        STATSD_METRIC *m
        , const char *type
        , const char *id
        , const char *name
        , const char *family
        , const char *context
        , const char *title
        , const char *units
        , long priority
        , int update_every
        , RRDSET_TYPE chart_type
) {
    RRD_MEMORY_MODE memory_mode = statsd.private_charts_memory_mode;
    long history = statsd.private_charts_rrd_history_entries;

    if(unlikely(statsd.private_charts >= statsd.max_private_charts)) {
        debug(D_STATSD, "STATSD: metric '%s' will be charted with memory mode = none, because the maximum number of charts has been reached.", m->name);
        info("STATSD: metric '%s' will be charted with memory mode = none, because the maximum number of charts (%zu) has been reached. Increase the number of charts by editing netdata.conf, [statsd] section.", m->name, statsd.max_private_charts);
        memory_mode = RRD_MEMORY_MODE_NONE;
        history = 5;
    }

    statsd.private_charts++;
    RRDSET *st = rrdset_create_custom(
            localhost         // host
            , type            // type
            , id              // id
            , name            // name
            , family          // family
            , context         // context
            , title           // title
            , units           // units
            , "statsd"        // plugin
            , NULL            // module
            , priority        // priority
            , update_every    // update every
            , chart_type      // chart type
            , memory_mode     // memory mode
            , history         // history
    );
    rrdset_flag_set(st, RRDSET_FLAG_STORE_FIRST);
    // rrdset_flag_set(st, RRDSET_FLAG_DEBUG);
    return st;
}

static inline void statsd_private_chart_gauge(STATSD_METRIC *m) {
    debug(D_STATSD, "updating private chart for gauge metric '%s'", m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, "gauge", RRD_ID_LENGTH_MAX);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , "gauges"      // family (submenu)
                , m->name       // context
                , m->name       // title
                , "value"       // units
                , STATSD_CHART_PRIORITY
                , statsd.update_every
                , RRDSET_TYPE_LINE
        );

        m->rd_value = rrddim_add(m->st, "gauge",  NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1,    RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    rrddim_set_by_pointer(m->st, m->rd_value, m->last);

    if(m->rd_count)
        rrddim_set_by_pointer(m->st, m->rd_count, m->events);

    rrdset_done(m->st);
}

static inline void statsd_private_chart_counter_or_meter(STATSD_METRIC *m, const char *dim, const char *family) {
    debug(D_STATSD, "updating private chart for %s metric '%s'", dim, m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, dim, RRD_ID_LENGTH_MAX);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , family        // family (submenu)
                , m->name       // context
                , m->name       // title
                , "events/s"    // units
                , STATSD_CHART_PRIORITY
                , statsd.update_every
                , RRDSET_TYPE_AREA
        );

        m->rd_value = rrddim_add(m->st, dim, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    rrddim_set_by_pointer(m->st, m->rd_value, m->last);

    if(m->rd_count)
        rrddim_set_by_pointer(m->st, m->rd_count, m->events);

    rrdset_done(m->st);
}

static inline void statsd_private_chart_set(STATSD_METRIC *m) {
    debug(D_STATSD, "updating private chart for set metric '%s'", m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, "set", RRD_ID_LENGTH_MAX);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , "sets"        // family (submenu)
                , m->name       // context
                , m->name       // title
                , "entries"     // units
                , STATSD_CHART_PRIORITY
                , statsd.update_every
                , RRDSET_TYPE_LINE
        );

        m->rd_value = rrddim_add(m->st, "set", "set size", 1, 1, RRD_ALGORITHM_ABSOLUTE);

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    rrddim_set_by_pointer(m->st, m->rd_value, m->last);

    if(m->rd_count)
        rrddim_set_by_pointer(m->st, m->rd_count, m->events);

    rrdset_done(m->st);
}

static inline void statsd_private_chart_timer_or_histogram(STATSD_METRIC *m, const char *dim, const char *family, const char *units) {
    debug(D_STATSD, "updating private chart for %s metric '%s'", dim, m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, dim, RRD_ID_LENGTH_MAX);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , family        // family (submenu)
                , m->name       // context
                , m->name       // title
                , units         // units
                , STATSD_CHART_PRIORITY
                , statsd.update_every
                , RRDSET_TYPE_AREA
        );

        m->histogram.ext->rd_min = rrddim_add(m->st, "min", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_max = rrddim_add(m->st, "max", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->rd_value              = rrddim_add(m->st, "average", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_percentile = rrddim_add(m->st, statsd.histogram_percentile_str, NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_median = rrddim_add(m->st, "median", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_stddev = rrddim_add(m->st, "stddev", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_sum = rrddim_add(m->st, "sum", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_min, m->histogram.ext->last_min);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_max, m->histogram.ext->last_max);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_percentile, m->histogram.ext->last_percentile);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_median, m->histogram.ext->last_median);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_stddev, m->histogram.ext->last_stddev);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_sum, m->histogram.ext->last_sum);
    rrddim_set_by_pointer(m->st, m->rd_value, m->last);

    if(m->rd_count)
        rrddim_set_by_pointer(m->st, m->rd_count, m->events);

    rrdset_done(m->st);
}

// --------------------------------------------------------------------------------------------------------------------
// statsd flush metrics

static inline void statsd_flush_gauge(STATSD_METRIC *m) {
    debug(D_STATSD, "flushing gauge metric '%s'", m->name);

    int updated = 0;
    if(m->count && !m->reset) {
        m->last = (collected_number) (m->gauge.value * statsd.decimal_detail);

        m->reset = 1;
        updated = 1;
    }

    if(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED)))
        statsd_private_chart_gauge(m);
}

static inline void statsd_flush_counter_or_meter(STATSD_METRIC *m, const char *dim, const char *family) {
    debug(D_STATSD, "flushing %s metric '%s'", dim, m->name);

    int updated = 0;
    if(m->count && !m->reset) {
        m->last = m->counter.value;

        m->reset = 1;
        updated = 1;
    }

    if(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED)))
        statsd_private_chart_counter_or_meter(m, dim, family);
}

static inline void statsd_flush_counter(STATSD_METRIC *m) {
    statsd_flush_counter_or_meter(m, "counter", "counters");
}

static inline void statsd_flush_meter(STATSD_METRIC *m) {
    statsd_flush_counter_or_meter(m, "meter", "meters");
}

static inline void statsd_flush_set(STATSD_METRIC *m) {
    debug(D_STATSD, "flushing set metric '%s'", m->name);

    int updated = 0;
    if(m->count && !m->reset) {
        m->last = (collected_number)m->set.unique;

        m->reset = 1;
        updated = 1;
    }

    if(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED)))
        statsd_private_chart_set(m);
}

static inline void statsd_flush_timer_or_histogram(STATSD_METRIC *m, const char *dim, const char *family, const char *units) {
    debug(D_STATSD, "flushing %s metric '%s'", dim, m->name);

    netdata_mutex_lock(&m->histogram.ext->mutex);

    if(unlikely(!m->histogram.ext->zeroed)) {
        // reset the metrics
        // if we collected anything, they will be updated below
        // this ensures that we report zeros if nothing is collected

        m->histogram.ext->last_min = 0;
        m->histogram.ext->last_max = 0;
        m->last = 0;
        m->histogram.ext->last_median = 0;
        m->histogram.ext->last_stddev = 0;
        m->histogram.ext->last_sum = 0;
        m->histogram.ext->last_percentile = 0;

        m->histogram.ext->zeroed = 1;
    }

    int updated = 0;
    if(m->count && !m->reset && m->histogram.ext->used > 0) {
        size_t len = m->histogram.ext->used;
        long double *series = m->histogram.ext->values;
        sort_series(series, len);

        m->histogram.ext->last_min = (collected_number)roundl(series[0] * statsd.decimal_detail);
        m->histogram.ext->last_max = (collected_number)roundl(series[len - 1] * statsd.decimal_detail);
        m->last = (collected_number)roundl(average(series, len) * statsd.decimal_detail);
        m->histogram.ext->last_median = (collected_number)roundl(median_on_sorted_series(series, len) * statsd.decimal_detail);
        m->histogram.ext->last_stddev = (collected_number)roundl(standard_deviation(series, len) * statsd.decimal_detail);
        m->histogram.ext->last_sum = (collected_number)roundl(sum(series, len) * statsd.decimal_detail);

        size_t pct_len = (size_t)floor((double)len * statsd.histogram_percentile / 100.0);
        if(pct_len < 1)
            m->histogram.ext->last_percentile = (collected_number)(series[0] * statsd.decimal_detail);
        else
            m->histogram.ext->last_percentile = (collected_number)roundl(series[pct_len - 1] * statsd.decimal_detail);

        debug(D_STATSD, "STATSD %s metric %s: min " COLLECTED_NUMBER_FORMAT ", max " COLLECTED_NUMBER_FORMAT ", last " COLLECTED_NUMBER_FORMAT ", pcent " COLLECTED_NUMBER_FORMAT ", median " COLLECTED_NUMBER_FORMAT ", stddev " COLLECTED_NUMBER_FORMAT ", sum " COLLECTED_NUMBER_FORMAT,
              dim, m->name, m->histogram.ext->last_min, m->histogram.ext->last_max, m->last, m->histogram.ext->last_percentile, m->histogram.ext->last_median, m->histogram.ext->last_stddev, m->histogram.ext->last_sum);

        m->histogram.ext->zeroed = 0;
        m->reset = 1;
        updated = 1;
    }

    if(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED)))
        statsd_private_chart_timer_or_histogram(m, dim, family, units);

    netdata_mutex_unlock(&m->histogram.ext->mutex);
}

static inline void statsd_flush_timer(STATSD_METRIC *m) {
    statsd_flush_timer_or_histogram(m, "timer", "timers", "milliseconds");
}

static inline void statsd_flush_histogram(STATSD_METRIC *m) {
    statsd_flush_timer_or_histogram(m, "histogram", "histograms", "value");
}

static inline RRD_ALGORITHM statsd_algorithm_for_metric(STATSD_METRIC *m) {
    switch(m->type) {
        default:
        case STATSD_METRIC_TYPE_GAUGE:
        case STATSD_METRIC_TYPE_SET:
        case STATSD_METRIC_TYPE_TIMER:
        case STATSD_METRIC_TYPE_HISTOGRAM:
            return RRD_ALGORITHM_ABSOLUTE;

        case STATSD_METRIC_TYPE_METER:
        case STATSD_METRIC_TYPE_COUNTER:
            return RRD_ALGORITHM_INCREMENTAL;
    }
}

static inline void check_if_metric_is_for_app(STATSD_INDEX *index, STATSD_METRIC *m) {
    (void)index;

    STATSD_APP *app;
    for(app = statsd.apps; app ;app = app->next) {
        if(unlikely(simple_pattern_matches(app->metrics, m->name))) {
            debug(D_STATSD, "metric '%s' matches app '%s'", m->name, app->name);

            // the metric should get the options from the app

            if(app->default_options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED)
                m->options |= STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;
            else
                m->options &= ~STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;

            if(app->default_options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED)
                m->options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;
            else
                m->options &= ~STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

            m->options |= STATSD_METRIC_OPTION_PRIVATE_CHART_CHECKED;

            // check if there is a chart in this app, willing to get this metric
            STATSD_APP_CHART *chart;
            for(chart = app->charts; chart; chart = chart->next) {
                STATSD_APP_CHART_DIM *dim;
                for(dim = chart->dimensions; dim ; dim = dim->next) {
                    if(!dim->value_ptr && dim->metric_hash == m->hash && !strcmp(dim->metric, m->name)) {
                        // we have a match - this metric should be linked to this dimension

                        if(dim->value_type == STATSD_APP_CHART_DIM_VALUE_TYPE_EVENTS) {
                            dim->value_ptr = &m->events;
                            dim->algorithm = RRD_ALGORITHM_INCREMENTAL;
                        }
                        else if(m->type == STATSD_METRIC_TYPE_HISTOGRAM || m->type == STATSD_METRIC_TYPE_TIMER) {
                            dim->algorithm = RRD_ALGORITHM_ABSOLUTE;
                            dim->divisor *= statsd.decimal_detail;

                            switch(dim->value_type) {
                                case STATSD_APP_CHART_DIM_VALUE_TYPE_EVENTS:
                                    // will never match - added to avoid warning
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_LAST:
                                case STATSD_APP_CHART_DIM_VALUE_TYPE_AVERAGE:
                                    dim->value_ptr = &m->last;
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_SUM:
                                    dim->value_ptr = &m->histogram.ext->last_sum;
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_MIN:
                                    dim->value_ptr = &m->histogram.ext->last_min;
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_MAX:
                                    dim->value_ptr = &m->histogram.ext->last_max;
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_MEDIAN:
                                    dim->value_ptr = &m->histogram.ext->last_median;
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_PERCENTILE:
                                    dim->value_ptr = &m->histogram.ext->last_percentile;
                                    break;

                                case STATSD_APP_CHART_DIM_VALUE_TYPE_STDDEV:
                                    dim->value_ptr = &m->histogram.ext->last_stddev;
                                    break;
                            }
                        }
                        else {
                            if (dim->value_type != STATSD_APP_CHART_DIM_VALUE_TYPE_LAST)
                                error("STATSD: unsupported value type for dimension '%s' of chart '%s' of app '%s' on metric '%s'", dim->name, chart->id, app->name, m->name);

                            dim->value_ptr = &m->last;
                            dim->algorithm = statsd_algorithm_for_metric(m);

                            if(m->type == STATSD_METRIC_TYPE_GAUGE)
                                dim->divisor *= statsd.decimal_detail;
                        }

                        if(unlikely(chart->st && dim->rd)) {
                            rrddim_set_algorithm(chart->st, dim->rd, dim->algorithm);
                            rrddim_set_multiplier(chart->st, dim->rd, dim->multiplier);
                            rrddim_set_divisor(chart->st, dim->rd, dim->divisor);
                        }

                        chart->dimensions_linked_count++;
                        debug(D_STATSD, "metric '%s' of type %u linked with app '%s', chart '%s', dimension '%s', algorithm '%s'", m->name, m->type, app->name, chart->id, dim->name, rrd_algorithm_name(dim->algorithm));
                    }
                }
            }
        }
    }
}

static inline void statsd_update_app_chart(STATSD_APP *app, STATSD_APP_CHART *chart) {
    debug(D_STATSD, "updating chart '%s' for app '%s'", chart->id, app->name);

    if(!chart->st) {
        chart->st = rrdset_create_custom(
                localhost                   // host
                , app->name                 // type
                , chart->id                 // id
                , chart->name               // name
                , chart->family             // family
                , chart->context            // context
                , chart->title              // title
                , chart->units              // units
                , "statsd"                  // plugin
                , NULL                      // module
                , chart->priority           // priority
                , statsd.update_every       // update every
                , chart->chart_type         // chart type
                , app->rrd_memory_mode      // memory mode
                , app->rrd_history_entries  // history
        );

        rrdset_flag_set(chart->st, RRDSET_FLAG_STORE_FIRST);
        // rrdset_flag_set(chart->st, RRDSET_FLAG_DEBUG);
    }
    else rrdset_next(chart->st);

    STATSD_APP_CHART_DIM *dim;
    for(dim = chart->dimensions; dim ;dim = dim->next) {
        if(unlikely(!dim->rd))
            dim->rd = rrddim_add(chart->st, dim->metric, dim->name, dim->multiplier, dim->divisor, dim->algorithm);

        if(unlikely(dim->value_ptr)) {
            debug(D_STATSD, "updating dimension '%s' (%s) of chart '%s' (%s) for app '%s' with value " COLLECTED_NUMBER_FORMAT, dim->name, dim->rd->id, chart->id, chart->st->id, app->name, *dim->value_ptr);
            rrddim_set_by_pointer(chart->st, dim->rd, *dim->value_ptr);
        }
    }

    rrdset_done(chart->st);
    debug(D_STATSD, "completed update of chart '%s' for app '%s'", chart->id, app->name);
}

static inline void statsd_update_all_app_charts(void) {
    // debug(D_STATSD, "updating app charts");

    STATSD_APP *app;
    for(app = statsd.apps; app ;app = app->next) {
        // debug(D_STATSD, "updating charts for app '%s'", app->name);

        STATSD_APP_CHART *chart;
        for(chart = app->charts; chart ;chart = chart->next) {
            if(unlikely(chart->dimensions_linked_count)) {
                statsd_update_app_chart(app, chart);
            }
        }
    }

    // debug(D_STATSD, "completed update of app charts");
}

static inline void statsd_flush_index_metrics(STATSD_INDEX *index, void (*flush_metric)(STATSD_METRIC *)) {
    STATSD_METRIC *m;
    for(m = index->first; m ; m = m->next) {
        if(unlikely(!(m->options & STATSD_METRIC_OPTION_CHECKED_IN_APPS))) {
            check_if_metric_is_for_app(index, m);
            m->options |= STATSD_METRIC_OPTION_CHECKED_IN_APPS;
        }

        if(unlikely(!(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_CHECKED))) {
            if(statsd.private_charts >= statsd.max_private_charts_hard) {
                debug(D_STATSD, "STATSD: metric '%s' will not be charted, because the hard limit of the maximum number of charts has been reached.", m->name);
                info("STATSD: metric '%s' will not be charted, because the hard limit of the maximum number of charts (%zu) has been reached. Increase the number of charts by editing netdata.conf, [statsd] section.", m->name, statsd.max_private_charts);
                m->options &= ~STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;
            }
            else {
                if (simple_pattern_matches(statsd.charts_for, m->name)) {
                    debug(D_STATSD, "STATSD: metric '%s' will be charted.", m->name);
                    m->options |= STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;
                } else {
                    debug(D_STATSD, "STATSD: metric '%s' will not be charted.", m->name);
                    m->options &= ~STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED;
                }
            }

            m->options |= STATSD_METRIC_OPTION_PRIVATE_CHART_CHECKED;
        }

        flush_metric(m);
    }
}


// --------------------------------------------------------------------------------------
// statsd main thread

int statsd_listen_sockets_setup(void) {
    return listen_sockets_setup(&statsd.sockets);
}

void statsd_main_cleanup(void *data) {
    pthread_t *threads = data;

    int i;
    for(i = 0; i < statsd.threads ;i++)
        pthread_cancel(threads[i]);

    listen_sockets_close(&statsd.sockets);
}

void *statsd_main(void *ptr) {
    (void)ptr;

    info("STATSD main thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    // ----------------------------------------------------------------------------------------------------------------
    // statsd configuration

    statsd.enabled = config_get_boolean(CONFIG_SECTION_STATSD, "enabled", statsd.enabled);

    statsd.update_every = default_rrd_update_every;
    statsd.update_every = (int)config_get_number(CONFIG_SECTION_STATSD, "update every (flushInterval)", statsd.update_every);
    if(statsd.update_every < default_rrd_update_every) {
        error("STATSD: minimum flush interval %d given, but the minimum is the update every of netdata. Using %d", statsd.update_every, default_rrd_update_every);
        statsd.update_every = default_rrd_update_every;
    }

#ifdef HAVE_RECVMMSG
    statsd.recvmmsg_size = (size_t)config_get_number(CONFIG_SECTION_STATSD, "udp messages to process at once", (long long)statsd.recvmmsg_size);
#endif

    statsd.charts_for = simple_pattern_create(config_get(CONFIG_SECTION_STATSD, "create private charts for metrics matching", "*"), SIMPLE_PATTERN_EXACT);
    statsd.max_private_charts = (size_t)config_get_number(CONFIG_SECTION_STATSD, "max private charts allowed", (long long)statsd.max_private_charts);
    statsd.max_private_charts_hard = (size_t)config_get_number(CONFIG_SECTION_STATSD, "max private charts hard limit", (long long)statsd.max_private_charts * 5);
    statsd.private_charts_memory_mode = rrd_memory_mode_id(config_get(CONFIG_SECTION_STATSD, "private charts memory mode", rrd_memory_mode_name(default_rrd_memory_mode)));
    statsd.private_charts_rrd_history_entries = (int)config_get_number(CONFIG_SECTION_STATSD, "private charts history", default_rrd_history_entries);
    statsd.decimal_detail = (size_t)config_get_number(CONFIG_SECTION_STATSD, "decimal detail", (long long int)statsd.decimal_detail);

    statsd.histogram_percentile = (double)config_get_float(CONFIG_SECTION_STATSD, "histograms and timers percentile (percentThreshold)", statsd.histogram_percentile);
    if(isless(statsd.histogram_percentile, 0) || isgreater(statsd.histogram_percentile, 100)) {
        error("STATSD: invalid histograms and timers percentile %0.5f given", statsd.histogram_percentile);
        statsd.histogram_percentile = 95.0;
    }
    {
        char buffer[100 + 1];
        snprintf(buffer, 100, "%0.1f%%", statsd.histogram_percentile);
        statsd.histogram_percentile_str = strdupz(buffer);
    }

    if(config_get_boolean(CONFIG_SECTION_STATSD, "add dimension for number of events received", 1)) {
        statsd.gauges.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.counters.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.meters.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.sets.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.histograms.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.timers.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
    }

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on gauges (deleteGauges)", 0))
        statsd.gauges.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on counters (deleteCounters)", 0))
        statsd.counters.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on meters (deleteMeters)", 0))
        statsd.meters.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on sets (deleteSets)", 0))
        statsd.sets.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on histograms (deleteHistograms)", 0))
        statsd.histograms.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on timers (deleteTimers)", 0))
        statsd.timers.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

#ifdef STATSD_MULTITHREADED
    statsd.threads = (int)config_get_number(CONFIG_SECTION_STATSD, "threads", processors);
    if(statsd.threads < 1) {
        error("STATSD: Invalid number of threads %d, using %d", statsd.threads, processors);
        statsd.threads = processors;
        config_set_number(CONFIG_SECTION_STATSD, "collector threads", statsd.threads);
    }
#else
    statsd.threads = 1;
#endif

    // read custom application definitions
    {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/statsd.d", netdata_configured_config_dir);
        statsd_readdir(filename);
    }

    // ----------------------------------------------------------------------------------------------------------------
    // statsd setup

    if(!statsd.enabled) return NULL;

    statsd_listen_sockets_setup();
    if(!statsd.sockets.opened) {
        error("STATSD: No statsd sockets to listen to. statsd will be disabled.");
        pthread_exit(NULL);
    }

    pthread_t threads[statsd.threads];
    int i;

    for(i = 0; i < statsd.threads ;i++) {
        if(pthread_create(&threads[i], NULL, statsd_collector_thread, &i))
            error("STATSD: failed to create child thread.");

        else if(pthread_detach(threads[i]))
            error("STATSD: cannot request detach of child thread.");
    }

    pthread_cleanup_push(statsd_main_cleanup, &threads);

    // ----------------------------------------------------------------------------------------------------------------
    // statsd monitoring charts

    RRDSET *st_metrics = rrdset_create_localhost(
            "netdata"
            , "statsd_metrics"
            , NULL
            , "statsd"
            , NULL
            , "Metrics in the netdata statsd database"
            , "metrics"
            , "netdata"
            , "stats"
            , 132000
            , statsd.update_every
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
            , "netdata"
            , "stats"
            , 132001
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_events_gauge     = rrddim_add(st_events, "gauges", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_counter   = rrddim_add(st_events, "counters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_timer     = rrddim_add(st_events, "timers", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_meter     = rrddim_add(st_events, "meters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_histogram = rrddim_add(st_events, "histograms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_set       = rrddim_add(st_events, "sets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_unknown   = rrddim_add(st_events, "unknown", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_errors    = rrddim_add(st_events, "errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st_reads = rrdset_create_localhost(
            "netdata"
            , "statsd_reads"
            , NULL
            , "statsd"
            , NULL
            , "Read operations made by the netdata statsd server"
            , "reads/s"
            , "netdata"
            , "stats"
            , 132002
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_reads_tcp = rrddim_add(st_reads, "tcp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_reads_udp = rrddim_add(st_reads, "udp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st_bytes = rrdset_create_localhost(
            "netdata"
            , "statsd_bytes"
            , NULL
            , "statsd"
            , NULL
            , "Bytes read by the netdata statsd server"
            , "kilobits/s"
            , "netdata"
            , "stats"
            , 132003
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_bytes_tcp = rrddim_add(st_bytes, "tcp", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_bytes_udp = rrddim_add(st_bytes, "udp", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st_packets = rrdset_create_localhost(
            "netdata"
            , "statsd_packets"
            , NULL
            , "statsd"
            , NULL
            , "Network packets processed by the netdata statsd server"
            , "packets/s"
            , "netdata"
            , "stats"
            , 132004
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_packets_tcp = rrddim_add(st_packets, "tcp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_packets_udp = rrddim_add(st_packets, "udp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st_pcharts = rrdset_create_localhost(
            "netdata"
            , "private_charts"
            , NULL
            , "statsd"
            , NULL
            , "Private metric charts created by the netdata statsd server"
            , "charts"
            , "netdata"
            , "stats"
            , 132010
            , statsd.update_every
            , RRDSET_TYPE_AREA
    );
    RRDDIM *rd_pcharts = rrddim_add(st_pcharts, "charts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);


    // ----------------------------------------------------------------------------------------------------------------
    // statsd thread to turn metrics into charts

    usec_t step = statsd.update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(;;) {
        usec_t hb_dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit))
            break;

        statsd_flush_index_metrics(&statsd.gauges,     statsd_flush_gauge);
        statsd_flush_index_metrics(&statsd.counters,   statsd_flush_counter);
        statsd_flush_index_metrics(&statsd.meters,     statsd_flush_meter);
        statsd_flush_index_metrics(&statsd.timers,     statsd_flush_timer);
        statsd_flush_index_metrics(&statsd.histograms, statsd_flush_histogram);
        statsd_flush_index_metrics(&statsd.sets,       statsd_flush_set);

        statsd_update_all_app_charts();

        if(unlikely(netdata_exit))
            break;

        if(hb_dt) {
            rrdset_next(st_metrics);
            rrdset_next(st_events);
            rrdset_next(st_reads);
            rrdset_next(st_bytes);
            rrdset_next(st_packets);
            rrdset_next(st_pcharts);
        }

        rrddim_set_by_pointer(st_metrics, rd_metrics_gauge,     (collected_number)statsd.gauges.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_counter,   (collected_number)statsd.counters.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_timer,     (collected_number)statsd.timers.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_meter,     (collected_number)statsd.meters.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_histogram, (collected_number)statsd.histograms.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_set,       (collected_number)statsd.sets.metrics);

        rrddim_set_by_pointer(st_events,  rd_events_gauge,       (collected_number)statsd.gauges.events);
        rrddim_set_by_pointer(st_events,  rd_events_counter,     (collected_number)statsd.counters.events);
        rrddim_set_by_pointer(st_events,  rd_events_timer,       (collected_number)statsd.timers.events);
        rrddim_set_by_pointer(st_events,  rd_events_meter,       (collected_number)statsd.meters.events);
        rrddim_set_by_pointer(st_events,  rd_events_histogram,   (collected_number)statsd.histograms.events);
        rrddim_set_by_pointer(st_events,  rd_events_set,         (collected_number)statsd.sets.events);
        rrddim_set_by_pointer(st_events,  rd_events_unknown,     (collected_number)statsd.unknown_types);
        rrddim_set_by_pointer(st_events,  rd_events_errors,      (collected_number)statsd.socket_errors);

        rrddim_set_by_pointer(st_reads,   rd_reads_tcp,          (collected_number)statsd.tcp_socket_reads);
        rrddim_set_by_pointer(st_reads,   rd_reads_udp,          (collected_number)statsd.udp_socket_reads);

        rrddim_set_by_pointer(st_bytes,   rd_bytes_tcp,          (collected_number)statsd.tcp_bytes_read);
        rrddim_set_by_pointer(st_bytes,   rd_bytes_udp,          (collected_number)statsd.udp_bytes_read);

        rrddim_set_by_pointer(st_packets, rd_packets_tcp,        (collected_number)statsd.tcp_packets_received);
        rrddim_set_by_pointer(st_packets, rd_packets_udp,        (collected_number)statsd.udp_packets_received);

        rrddim_set_by_pointer(st_pcharts, rd_pcharts,           (collected_number)statsd.private_charts);

        if(unlikely(netdata_exit))
            break;

        rrdset_done(st_metrics);
        rrdset_done(st_events);
        rrdset_done(st_reads);
        rrdset_done(st_bytes);
        rrdset_done(st_packets);
        rrdset_done(st_pcharts);

        if(unlikely(netdata_exit))
            break;
    }

    pthread_cleanup_pop(1);

    pthread_exit(NULL);
    return NULL;
}
