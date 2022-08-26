// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"

#define STATSD_CHART_PREFIX "statsd"

#define PLUGIN_STATSD_NAME "statsd.plugin"

#define STATSD_LISTEN_PORT 8125
#define STATSD_LISTEN_BACKLOG 4096

#define WORKER_JOB_TYPE_TCP_CONNECTED 0
#define WORKER_JOB_TYPE_TCP_DISCONNECTED 1
#define WORKER_JOB_TYPE_RCV_DATA 2
#define WORKER_JOB_TYPE_SND_DATA 3

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 4
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least 4
#endif

// --------------------------------------------------------------------------------------

// #define STATSD_MULTITHREADED 1

#ifdef STATSD_MULTITHREADED
// DO NOT ENABLE MULTITHREADING - IT IS NOT WELL TESTED
#define STATSD_DICTIONARY_OPTIONS DICTIONARY_FLAG_DONT_OVERWRITE_VALUE|DICTIONARY_FLAG_ADD_IN_FRONT
#else
#define STATSD_DICTIONARY_OPTIONS DICTIONARY_FLAG_DONT_OVERWRITE_VALUE|DICTIONARY_FLAG_ADD_IN_FRONT|DICTIONARY_FLAG_SINGLE_THREADED
#endif

#define STATSD_DECIMAL_DETAIL 1000 // floating point values get multiplied by this, with the same divisor

// --------------------------------------------------------------------------------------------------------------------
// data specific to each metric type

typedef struct statsd_metric_gauge {
    NETDATA_DOUBLE value;
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
    //RRDDIM *rd_sum;

    size_t size;
    size_t used;
    NETDATA_DOUBLE *values;   // dynamic array of values collected
} STATSD_METRIC_HISTOGRAM_EXTENSIONS;

typedef struct statsd_metric_histogram { // histogram and timer
    STATSD_METRIC_HISTOGRAM_EXTENSIONS *ext;
} STATSD_METRIC_HISTOGRAM;

typedef struct statsd_metric_set {
    DICTIONARY *dict;
    size_t unique;
} STATSD_METRIC_SET;

typedef struct statsd_metric_dictionary_item {
    size_t count;
    RRDDIM *rd;
} STATSD_METRIC_DICTIONARY_ITEM;

typedef struct statsd_metric_dictionary {
    DICTIONARY *dict;
    size_t unique;
} STATSD_METRIC_DICTIONARY;


// --------------------------------------------------------------------------------------------------------------------
// this is a metric - for all types of metrics

typedef enum statsd_metric_options {
    STATSD_METRIC_OPTION_NONE                         = 0x00000000, // no options set
    STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED = 0x00000001, // do not update the chart dimension, when this metric is not collected
    STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED        = 0x00000002, // render a private chart for this metric
    STATSD_METRIC_OPTION_PRIVATE_CHART_CHECKED        = 0x00000004, // the metric has been checked if it should get private chart or not
    STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT        = 0x00000008, // show the count of events for this private chart
    STATSD_METRIC_OPTION_CHECKED_IN_APPS              = 0x00000010, // set when this metric has been checked against apps
    STATSD_METRIC_OPTION_USED_IN_APPS                 = 0x00000020, // set when this metric is used in apps
    STATSD_METRIC_OPTION_CHECKED                      = 0x00000040, // set when the charting thread checks this metric for use in charts (its usefulness)
    STATSD_METRIC_OPTION_USEFUL                       = 0x00000080, // set when the charting thread finds the metric useful (i.e. used in a chart)
    STATSD_METRIC_OPTION_COLLECTION_FULL_LOGGED       = 0x00000100, // set when the collection is full for this metric
} STATS_METRIC_OPTIONS;

typedef enum statsd_metric_type {
    STATSD_METRIC_TYPE_GAUGE,
    STATSD_METRIC_TYPE_COUNTER,
    STATSD_METRIC_TYPE_METER,
    STATSD_METRIC_TYPE_TIMER,
    STATSD_METRIC_TYPE_HISTOGRAM,
    STATSD_METRIC_TYPE_SET,
    STATSD_METRIC_TYPE_DICTIONARY
} STATSD_METRIC_TYPE;


typedef struct statsd_metric {
    const char *name;               // the name of the metric - linked to dictionary name
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
        STATSD_METRIC_DICTIONARY dictionary;
    };

    char *units;
    char *dimname;
    char *family;

    // chart related members
    STATS_METRIC_OPTIONS options;   // STATSD_METRIC_OPTION_* (bitfield)
    char reset;                     // set to 1 by the charting thread to instruct the collector thread(s) to reset this metric
    collected_number last;          // the last value sent to netdata
    RRDSET *st;                     // the private chart of this metric
    RRDDIM *rd_value;               // the dimension of this metric value
    RRDDIM *rd_count;               // the dimension for the number of events received

    // linking, used for walking through all metrics
    struct statsd_metric *next_useful;
} STATSD_METRIC;


// --------------------------------------------------------------------------------------------------------------------
// each type of metric has its own index

typedef struct statsd_index {
    char *name;                     // the name of the index of metrics
    size_t events;                  // the number of events processed for this index
    size_t metrics;                 // the number of metrics in this index
    size_t useful;                  // the number of useful metrics in this index

    STATSD_METRIC_TYPE type;        // the type of index
    DICTIONARY *dict;

    STATSD_METRIC *first_useful;    // the linked list of useful metrics (new metrics are added in front)

    STATS_METRIC_OPTIONS default_options;  // default options for all metrics in this index
} STATSD_INDEX;

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
    const char *name;               // the name of this dimension
    const char *metric;             // the source metric name of this dimension
    uint32_t metric_hash;           // hash for fast string comparisons

    SIMPLE_PATTERN *metric_pattern; // set when the 'metric' is a simple pattern

    collected_number multiplier;    // the multiplier of the dimension
    collected_number divisor;       // the divisor of the dimension
    RRDDIM_FLAGS flags;             // the RRDDIM flags for this dimension

    STATSD_APP_CHART_DIM_VALUE_TYPE value_type; // which value to use of the source metric

    RRDDIM *rd;                     // a pointer to the RRDDIM that has been created for this dimension
    collected_number *value_ptr;    // a pointer to the source metric value
    RRD_ALGORITHM algorithm;        // the algorithm of this dimension

    struct statsd_app_chart_dimension *next; // the next dimension for this chart
} STATSD_APP_CHART_DIM;

typedef struct statsd_app_chart {
    const char *id;
    const char *name;
    const char *title;
    const char *family;
    const char *context;
    const char *units;
    const char *module;
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
    DICTIONARY *dict;
    long rrd_history_entries;

    const char *source;
    STATSD_APP_CHART *charts;
    struct statsd_app *next;
} STATSD_APP;

// --------------------------------------------------------------------------------------------------------------------
// global statsd data

struct collection_thread_status {
    int status;
    size_t max_sockets;

    netdata_thread_t thread;
};

static struct statsd {
    STATSD_INDEX gauges;
    STATSD_INDEX counters;
    STATSD_INDEX timers;
    STATSD_INDEX histograms;
    STATSD_INDEX meters;
    STATSD_INDEX sets;
    STATSD_INDEX dictionaries;

    size_t unknown_types;
    size_t socket_errors;
    size_t tcp_socket_connects;
    size_t tcp_socket_disconnects;
    size_t tcp_socket_connected;
    size_t tcp_socket_reads;
    size_t tcp_packets_received;
    size_t tcp_bytes_read;
    size_t udp_socket_reads;
    size_t udp_packets_received;
    size_t udp_bytes_read;

    int enabled;
    int update_every;
    SIMPLE_PATTERN *charts_for;

    size_t tcp_idle_timeout;
    collected_number decimal_detail;
    size_t private_charts;
    size_t max_private_charts_hard;
    long private_charts_rrd_history_entries;
    unsigned int private_charts_hidden:1;

    STATSD_APP *apps;
    size_t recvmmsg_size;
    size_t histogram_increase_step;
    double histogram_percentile;
    char *histogram_percentile_str;
    size_t dictionary_max_unique;

    int threads;
    struct collection_thread_status *collection_threads_status;

    LISTEN_SOCKETS sockets;
} statsd = {
        .enabled = 1,
        .max_private_charts_hard = 1000,
        .private_charts_hidden = 0,
        .recvmmsg_size = 10,
        .decimal_detail = STATSD_DECIMAL_DETAIL,

        .gauges     = {
                .name = "gauge",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_GAUGE,
                .default_options = STATSD_METRIC_OPTION_NONE
        },
        .counters   = {
                .name = "counter",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_COUNTER,
                .default_options = STATSD_METRIC_OPTION_NONE
        },
        .timers     = {
                .name = "timer",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_TIMER,
                .default_options = STATSD_METRIC_OPTION_NONE
        },
        .histograms = {
                .name = "histogram",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_HISTOGRAM,
                .default_options = STATSD_METRIC_OPTION_NONE
        },
        .meters     = {
                .name = "meter",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_METER,
                .default_options = STATSD_METRIC_OPTION_NONE
        },
        .sets       = {
                .name = "set",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_SET,
                .default_options = STATSD_METRIC_OPTION_NONE
        },
        .dictionaries = {
                .name = "dictionary",
                .events = 0,
                .metrics = 0,
                .dict = NULL,
                .type = STATSD_METRIC_TYPE_DICTIONARY,
                .default_options = STATSD_METRIC_OPTION_NONE
        },

        .tcp_idle_timeout = 600,

        .apps = NULL,
        .histogram_percentile = 95.0,
        .histogram_increase_step = 10,
        .dictionary_max_unique = 200,
        .threads = 0,
        .collection_threads_status = NULL,
        .sockets = {
                .config          = &netdata_config,
                .config_section  = CONFIG_SECTION_STATSD,
                .default_bind_to = "udp:localhost tcp:localhost",
                .default_port    = STATSD_LISTEN_PORT,
                .backlog         = STATSD_LISTEN_BACKLOG
        },
};


// --------------------------------------------------------------------------------------------------------------------
// statsd index management - add/find metrics

static void dictionary_metric_insert_callback(const char *name, void *value, void *data) {
    STATSD_INDEX *index = (STATSD_INDEX *)data;
    STATSD_METRIC *m = (STATSD_METRIC *)value;

    debug(D_STATSD, "Creating new %s metric '%s'", index->name, name);

    m->name = name;
    m->hash = simple_hash(name);
    m->type = index->type;
    m->options = index->default_options;

    if (m->type == STATSD_METRIC_TYPE_HISTOGRAM || m->type == STATSD_METRIC_TYPE_TIMER) {
        m->histogram.ext = callocz(1,sizeof(STATSD_METRIC_HISTOGRAM_EXTENSIONS));
        netdata_mutex_init(&m->histogram.ext->mutex);
    }

    __atomic_fetch_add(&index->metrics, 1, __ATOMIC_RELAXED);
}

static void dictionary_metric_delete_callback(const char *name, void *value, void *data) {
    (void)data; // STATSD_INDEX *index = (STATSD_INDEX *)data;
    (void)name;
    STATSD_METRIC *m = (STATSD_METRIC *)value;

    if(m->type == STATSD_METRIC_TYPE_HISTOGRAM || m->type == STATSD_METRIC_TYPE_TIMER) {
        freez(m->histogram.ext);
        m->histogram.ext = NULL;
    }

    freez(m->units);
    freez(m->family);
    freez(m->dimname);
}

static inline STATSD_METRIC *statsd_find_or_add_metric(STATSD_INDEX *index, const char *name) {
    debug(D_STATSD, "searching for metric '%s' under '%s'", name, index->name);

#ifdef STATSD_MULTITHREADED
    // avoid the write lock of dictionary_set() for existing metrics
    STATSD_METRIC *m = dictionary_get(index->dict, name);
    if(!m) m = dictionary_set(index->dict, name, NULL, sizeof(STATSD_METRIC));
#else
    // no locks here, go faster
    // this will call the dictionary_metric_insert_callback() if an item
    // is inserted, otherwise it will return the existing one.
    // We used the flag DICTIONARY_FLAG_DONT_OVERWRITE_VALUE to support this.
    STATSD_METRIC *m = dictionary_set(index->dict, name, NULL, sizeof(STATSD_METRIC));
#endif

    index->events++;
    return m;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd parsing numbers

static inline NETDATA_DOUBLE statsd_parse_float(const char *v, NETDATA_DOUBLE def) {
    NETDATA_DOUBLE value;

    if(likely(v && *v)) {
        char *e = NULL;
        value = str2ndd(v, &e);
        if(unlikely(e && *e))
            error("STATSD: excess data '%s' after value '%s'", e, v);
    }
    else
        value = def;

    return value;
}

static inline NETDATA_DOUBLE statsd_parse_sampling_rate(const char *v) {
    NETDATA_DOUBLE sampling_rate = statsd_parse_float(v, 1.0);
    if(unlikely(isless(sampling_rate, 0.001))) sampling_rate = 0.001;
    if(unlikely(isgreater(sampling_rate, 1.0))) sampling_rate = 1.0;
    return sampling_rate;
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

static inline int value_is_zinit(const char *value) {
    return (value && *value == 'z' && *++value == 'i' && *++value == 'n' && *++value == 'i' && *++value == 't' && *++value == '\0');
}

#define is_metric_checked(m) ((m)->options & STATSD_METRIC_OPTION_CHECKED)
#define is_metric_useful_for_collection(m) (!is_metric_checked(m) || ((m)->options & STATSD_METRIC_OPTION_USEFUL))

static inline void statsd_process_gauge(STATSD_METRIC *m, const char *value, const char *sampling) {
    if(!is_metric_useful_for_collection(m)) return;

    if(unlikely(!value || !*value)) {
        error("STATSD: metric '%s' of type gauge, with empty value is ignored.", m->name);
        return;
    }

    if(unlikely(m->reset)) {
        // no need to reset anything specific for gauges
        statsd_reset_metric(m);
    }

    if(unlikely(value_is_zinit(value))) {
        // magic loading of metric, without affecting anything
    }
    else {
        if (unlikely(*value == '+' || *value == '-'))
            m->gauge.value += statsd_parse_float(value, 1.0) / statsd_parse_sampling_rate(sampling);
        else
            m->gauge.value = statsd_parse_float(value, 1.0);

        m->events++;
        m->count++;
    }
}

static inline void statsd_process_counter_or_meter(STATSD_METRIC *m, const char *value, const char *sampling) {
    if(!is_metric_useful_for_collection(m)) return;

    // we accept empty values for counters

    if(unlikely(m->reset)) statsd_reset_metric(m);

    if(unlikely(value_is_zinit(value))) {
        // magic loading of metric, without affecting anything
    }
    else {
        m->counter.value += llrintndd((NETDATA_DOUBLE) statsd_parse_int(value, 1) / statsd_parse_sampling_rate(sampling));

        m->events++;
        m->count++;
    }
}

#define statsd_process_counter(m, value, sampling) statsd_process_counter_or_meter(m, value, sampling)
#define statsd_process_meter(m, value, sampling) statsd_process_counter_or_meter(m, value, sampling)

static inline void statsd_process_histogram_or_timer(STATSD_METRIC *m, const char *value, const char *sampling, const char *type) {
    if(!is_metric_useful_for_collection(m)) return;

    if(unlikely(!value || !*value)) {
        error("STATSD: metric of type %s, with empty value is ignored.", type);
        return;
    }

    if(unlikely(m->reset)) {
        m->histogram.ext->used = 0;
        statsd_reset_metric(m);
    }

    if(unlikely(value_is_zinit(value))) {
        // magic loading of metric, without affecting anything
    }
    else {
        NETDATA_DOUBLE v = statsd_parse_float(value, 1.0);
        NETDATA_DOUBLE sampling_rate = statsd_parse_sampling_rate(sampling);
        if(unlikely(isless(sampling_rate, 0.01))) sampling_rate = 0.01;
        if(unlikely(isgreater(sampling_rate, 1.0))) sampling_rate = 1.0;

        long long samples = llrintndd(1.0 / sampling_rate);
        while(samples-- > 0) {

            if(unlikely(m->histogram.ext->used == m->histogram.ext->size)) {
                netdata_mutex_lock(&m->histogram.ext->mutex);
                m->histogram.ext->size += statsd.histogram_increase_step;
                m->histogram.ext->values = reallocz(m->histogram.ext->values, sizeof(NETDATA_DOUBLE) * m->histogram.ext->size);
                netdata_mutex_unlock(&m->histogram.ext->mutex);
            }

            m->histogram.ext->values[m->histogram.ext->used++] = v;
        }

        m->events++;
        m->count++;
    }
}

#define statsd_process_timer(m, value, sampling) statsd_process_histogram_or_timer(m, value, sampling, "timer")
#define statsd_process_histogram(m, value, sampling) statsd_process_histogram_or_timer(m, value, sampling, "histogram")

static void dictionary_metric_set_value_insert_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)value;
    STATSD_METRIC *m = (STATSD_METRIC *)data;
    m->set.unique++;
}

static inline void statsd_process_set(STATSD_METRIC *m, const char *value) {
    if(!is_metric_useful_for_collection(m)) return;

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

    if (unlikely(!m->set.dict)) {
        m->set.dict   = dictionary_create(STATSD_DICTIONARY_OPTIONS);
        dictionary_register_insert_callback(m->set.dict, dictionary_metric_set_value_insert_callback, m);
        m->set.unique = 0;
    }

    if(unlikely(value_is_zinit(value))) {
        // magic loading of metric, without affecting anything
    }
    else {
#ifdef STATSD_MULTITHREADED
        // avoid the write lock to check if something is already there
        if(!dictionary_get(m->set.dict, value))
            dictionary_set(m->set.dict, value, NULL, 0);
#else
        dictionary_set(m->set.dict, value, NULL, 0);
#endif
        m->events++;
        m->count++;
    }
}

static void dictionary_metric_dict_value_insert_callback(const char *name, void *value, void *data) {
    (void)name;
    (void)value;
    STATSD_METRIC *m = (STATSD_METRIC *)data;
    m->dictionary.unique++;
}

static inline void statsd_process_dictionary(STATSD_METRIC *m, const char *value) {
    if(!is_metric_useful_for_collection(m)) return;

    if(unlikely(!value || !*value)) {
        error("STATSD: metric of type set, with empty value is ignored.");
        return;
    }

    if(unlikely(m->reset))
        statsd_reset_metric(m);

    if (unlikely(!m->dictionary.dict)) {
        m->dictionary.dict   = dictionary_create(STATSD_DICTIONARY_OPTIONS);
        dictionary_register_insert_callback(m->dictionary.dict, dictionary_metric_dict_value_insert_callback, m);
        m->dictionary.unique = 0;
    }

    if(unlikely(value_is_zinit(value))) {
        // magic loading of metric, without affecting anything
    }
    else {
        STATSD_METRIC_DICTIONARY_ITEM *t = (STATSD_METRIC_DICTIONARY_ITEM *)dictionary_get(m->dictionary.dict, value);

        if (unlikely(!t)) {
            if(!t && m->dictionary.unique >= statsd.dictionary_max_unique)
                value = "other";

            t = (STATSD_METRIC_DICTIONARY_ITEM *)dictionary_set(m->dictionary.dict, value, NULL, sizeof(STATSD_METRIC_DICTIONARY_ITEM));
        }

        t->count++;
        m->events++;
        m->count++;
    }
}


// --------------------------------------------------------------------------------------------------------------------
// statsd parsing

static inline const char *statsd_parse_skip_up_to(const char *s, char d1, char d2, char d3) {
    char c;

    for(c = *s; c && c != d1 && c != d2 && c != d3 && c != '\r' && c != '\n'; c = *++s) ;

    return s;
}

const char *statsd_parse_skip_spaces(const char *s) {
    char c;

    for(c = *s; c && ( c == ' ' || c == '\t' || c == '\r' || c == '\n' ); c = *++s) ;

    return s;
}

static inline const char *statsd_parse_field_trim(const char *start, char *end) {
    if(unlikely(!start || !*start)) {
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

static void statsd_process_metric(const char *name, const char *value, const char *type, const char *sampling, const char *tags) {
    debug(D_STATSD, "STATSD: raw metric '%s', value '%s', type '%s', sampling '%s', tags '%s'", name?name:"(null)", value?value:"(null)", type?type:"(null)", sampling?sampling:"(null)", tags?tags:"(null)");

    if(unlikely(!name || !*name)) return;
    if(unlikely(!type || !*type)) type = "m";

    STATSD_METRIC *m = NULL;

    char t0 = type[0], t1 = type[1];
    if(unlikely(t0 == 'g' && t1 == '\0')) {
        statsd_process_gauge(
            m = statsd_find_or_add_metric(&statsd.gauges, name),
            value, sampling);
    }
    else if(unlikely((t0 == 'c' || t0 == 'C') && t1 == '\0')) {
        // etsy/statsd uses 'c'
        // brubeck     uses 'C'
        statsd_process_counter(
            m = statsd_find_or_add_metric(&statsd.counters, name),
            value, sampling);
    }
    else if(unlikely(t0 == 'm' && t1 == '\0')) {
        statsd_process_meter(
            m = statsd_find_or_add_metric(&statsd.meters, name),
            value, sampling);
    }
    else if(unlikely(t0 == 'h' && t1 == '\0')) {
        statsd_process_histogram(
            m = statsd_find_or_add_metric(&statsd.histograms, name),
            value, sampling);
    }
    else if(unlikely(t0 == 's' && t1 == '\0')) {
        statsd_process_set(
            m = statsd_find_or_add_metric(&statsd.sets, name),
            value);
    }
    else if(unlikely(t0 == 'd' && t1 == '\0')) {
        statsd_process_dictionary(
            m = statsd_find_or_add_metric(&statsd.dictionaries, name),
            value);
    }
    else if(unlikely(t0 == 'm' && t1 == 's' && type[2] == '\0')) {
        statsd_process_timer(
            m = statsd_find_or_add_metric(&statsd.timers, name),
            value, sampling);
    }
    else {
        statsd.unknown_types++;
        error("STATSD: metric '%s' with value '%s' is sent with unknown metric type '%s'", name, value?value:"", type);
    }

    if(m && tags && *tags) {
        const char *s = tags;
        while(*s) {
            const char *tagkey = NULL, *tagvalue = NULL;
            char *tagkey_end = NULL, *tagvalue_end = NULL;

            s = tagkey_end = (char *)statsd_parse_skip_up_to(tagkey = s, ':', '=', ',');
            if(tagkey == tagkey_end) {
                if (*s) {
                    s++;
                    s = statsd_parse_skip_spaces(s);
                }
                continue;
            }

            if(likely(*s == ':' || *s == '='))
                s = tagvalue_end = (char *) statsd_parse_skip_up_to(tagvalue = ++s, ',', '\0', '\0');

            if(*s == ',') s++;

            statsd_parse_field_trim(tagkey, tagkey_end);
            statsd_parse_field_trim(tagvalue, tagvalue_end);

            if(tagkey && *tagkey && tagvalue && *tagvalue) {
                if (!m->units && strcmp(tagkey, "units") == 0)
                    m->units = strdupz(tagvalue);

                if (!m->dimname && strcmp(tagkey, "name") == 0)
                    m->dimname = strdupz(tagvalue);

                if (!m->family && strcmp(tagkey, "family") == 0)
                    m->family = strdupz(tagvalue);
            }
        }
    }
}

static inline size_t statsd_process(char *buffer, size_t size, int require_newlines) {
    buffer[size] = '\0';
    debug(D_STATSD, "RECEIVED: %zu bytes: '%s'", size, buffer);

    const char *s = buffer;
    while(*s) {
        const char *name = NULL, *value = NULL, *type = NULL, *sampling = NULL, *tags = NULL;
        char *name_end = NULL, *value_end = NULL, *type_end = NULL, *sampling_end = NULL, *tags_end = NULL;

        s = name_end = (char *)statsd_parse_skip_up_to(name = s, ':', '=', '|');
        if(name == name_end) {
            if (*s) {
                s++;
                s = statsd_parse_skip_spaces(s);
            }
            continue;
        }

        if(likely(*s == ':' || *s == '='))
            s = value_end = (char *) statsd_parse_skip_up_to(value = ++s, '|', '@', '#');

        if(likely(*s == '|'))
            s = type_end = (char *) statsd_parse_skip_up_to(type = ++s, '|', '@', '#');

        while(*s == '|' || *s == '@' || *s == '#') {
            // parse all the fields that may be appended

            if ((*s == '|' && s[1] == '@') || *s == '@') {
                s = sampling_end = (char *)statsd_parse_skip_up_to(sampling = ++s, '|', '@', '#');
                if (*sampling == '@') sampling++;
            }
            else if ((*s == '|' && s[1] == '#') || *s == '#') {
                s = tags_end = (char *)statsd_parse_skip_up_to(tags = ++s, '|', '@', '#');
                if (*tags == '#') tags++;
            }
            else {
                // unknown field, skip it
                s = (char *)statsd_parse_skip_up_to(++s, '|', '@', '#');
            }
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
                , statsd_parse_field_trim(tags, tags_end)
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
    int *running;
    STATSD_SOCKET_DATA_TYPE type;
    size_t size;
    struct iovec *iovecs;
    struct mmsghdr *msgs;
};
#else
struct statsd_udp {
    int *running;
    STATSD_SOCKET_DATA_TYPE type;
    char buffer[STATSD_UDP_BUFFER_SIZE];
};
#endif

// new TCP client connected
static void *statsd_add_callback(POLLINFO *pi, short int *events, void *data) {
    (void)pi;
    (void)data;

    worker_is_busy(WORKER_JOB_TYPE_TCP_CONNECTED);
    *events = POLLIN;

    struct statsd_tcp *t = (struct statsd_tcp *)callocz(sizeof(struct statsd_tcp) + STATSD_TCP_BUFFER_SIZE, 1);
    t->type = STATSD_SOCKET_DATA_TYPE_TCP;
    t->size = STATSD_TCP_BUFFER_SIZE - 1;
    statsd.tcp_socket_connects++;
    statsd.tcp_socket_connected++;

    worker_is_idle();
    return t;
}

// TCP client disconnected
static void statsd_del_callback(POLLINFO *pi) {
    worker_is_busy(WORKER_JOB_TYPE_TCP_DISCONNECTED);

    struct statsd_tcp *t = pi->data;

    if(likely(t)) {
        if(t->type == STATSD_SOCKET_DATA_TYPE_TCP) {
            if(t->len != 0) {
                statsd.socket_errors++;
                error("STATSD: client is probably sending unterminated metrics. Closed socket left with '%s'. Trying to process it.", t->buffer);
                statsd_process(t->buffer, t->len, 0);
            }
            statsd.tcp_socket_disconnects++;
            statsd.tcp_socket_connected--;
        }
        else
            error("STATSD: internal error: received socket data type is %d, but expected %d", (int)t->type, (int)STATSD_SOCKET_DATA_TYPE_TCP);

        freez(t);
    }

    worker_is_idle();
}

// Receive data
static int statsd_rcv_callback(POLLINFO *pi, short int *events) {
    int retval = -1;
    worker_is_busy(WORKER_JOB_TYPE_RCV_DATA);

    *events = POLLIN;

    int fd = pi->fd;

    switch(pi->socktype) {
        case SOCK_STREAM: {
            struct statsd_tcp *d = (struct statsd_tcp *)pi->data;
            if(unlikely(!d)) {
                error("STATSD: internal error: expected TCP data pointer is NULL");
                statsd.socket_errors++;
                retval = -1;
                goto cleanup;
            }

#ifdef NETDATA_INTERNAL_CHECKS
            if(unlikely(d->type != STATSD_SOCKET_DATA_TYPE_TCP)) {
                error("STATSD: internal error: socket data type should be %d, but it is %d", (int)STATSD_SOCKET_DATA_TYPE_TCP, (int)d->type);
                statsd.socket_errors++;
                retval = -1;
                goto cleanup;
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

                if(unlikely(ret == -1)) {
                    retval = -1;
                    goto cleanup;
                }

            } while (rc != -1);
            break;
        }

        case SOCK_DGRAM: {
            struct statsd_udp *d = (struct statsd_udp *)pi->data;
            if(unlikely(!d)) {
                error("STATSD: internal error: expected UDP data pointer is NULL");
                statsd.socket_errors++;
                retval = -1;
                goto cleanup;
            }

#ifdef NETDATA_INTERNAL_CHECKS
            if(unlikely(d->type != STATSD_SOCKET_DATA_TYPE_UDP)) {
                error("STATSD: internal error: socket data should be %d, but it is %d", (int)d->type, (int)STATSD_SOCKET_DATA_TYPE_UDP);
                statsd.socket_errors++;
                retval = -1;
                goto cleanup;
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
                        retval = -1;
                        goto cleanup;
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
                        retval = -1;
                        goto cleanup;
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
            error("STATSD: internal error: unknown socktype %d on socket %d", pi->socktype, fd);
            statsd.socket_errors++;
            retval = -1;
            goto cleanup;
        }
    }

    retval = 0;
cleanup:
    worker_is_idle();
    return retval;
}

static int statsd_snd_callback(POLLINFO *pi, short int *events) {
    (void)pi;
    (void)events;

    worker_is_busy(WORKER_JOB_TYPE_SND_DATA);
    error("STATSD: snd_callback() called, but we never requested to send data to statsd clients.");
    worker_is_idle();

    return -1;
}

// --------------------------------------------------------------------------------------------------------------------
// statsd child thread to collect metrics from network

void statsd_collector_thread_cleanup(void *data) {
    struct statsd_udp *d = data;
    *d->running = 0;

    info("cleaning up...");

#ifdef HAVE_RECVMMSG
    size_t i;
    for (i = 0; i < d->size; i++)
        freez(d->iovecs[i].iov_base);

    freez(d->iovecs);
    freez(d->msgs);
#endif

    freez(d);
    worker_unregister();
}

void *statsd_collector_thread(void *ptr) {
    struct collection_thread_status *status = ptr;
    status->status = 1;

    worker_register("STATSD");
    worker_register_job_name(WORKER_JOB_TYPE_TCP_CONNECTED, "tcp connect");
    worker_register_job_name(WORKER_JOB_TYPE_TCP_DISCONNECTED, "tcp disconnect");
    worker_register_job_name(WORKER_JOB_TYPE_RCV_DATA, "receive");
    worker_register_job_name(WORKER_JOB_TYPE_SND_DATA, "send");

    info("STATSD collector thread started with taskid %d", gettid());

    struct statsd_udp *d = callocz(sizeof(struct statsd_udp), 1);
    d->running = &status->status;

    netdata_thread_cleanup_push(statsd_collector_thread_cleanup, d);

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
            , NULL                     // No access control pattern
            , 0                        // No dns lookups for access control pattern
            , (void *)d
            , 0                        // tcp request timeout, 0 = disabled
            , statsd.tcp_idle_timeout  // tcp idle timeout, 0 = disabled
            , statsd.update_every * 1000
            , ptr // timer_data
            , status->max_sockets
    );

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// --------------------------------------------------------------------------------------------------------------------
// statsd applications configuration files parsing

#define STATSD_CONF_LINE_MAX 8192

static STATSD_APP_CHART_DIM_VALUE_TYPE string2valuetype(const char *type, size_t line, const char *filename) {
    if(!type || !*type) type = "last";

    if(!strcmp(type, "events")) return STATSD_APP_CHART_DIM_VALUE_TYPE_EVENTS;
    else if(!strcmp(type, "last")) return STATSD_APP_CHART_DIM_VALUE_TYPE_LAST;
    else if(!strcmp(type, "min")) return STATSD_APP_CHART_DIM_VALUE_TYPE_MIN;
    else if(!strcmp(type, "max")) return STATSD_APP_CHART_DIM_VALUE_TYPE_MAX;
    else if(!strcmp(type, "sum")) return STATSD_APP_CHART_DIM_VALUE_TYPE_SUM;
    else if(!strcmp(type, "average")) return STATSD_APP_CHART_DIM_VALUE_TYPE_AVERAGE;
    else if(!strcmp(type, "median")) return STATSD_APP_CHART_DIM_VALUE_TYPE_MEDIAN;
    else if(!strcmp(type, "stddev")) return STATSD_APP_CHART_DIM_VALUE_TYPE_STDDEV;
    else if(!strcmp(type, "percentile")) return STATSD_APP_CHART_DIM_VALUE_TYPE_PERCENTILE;

    error("STATSD: invalid type '%s' at line %zu of file '%s'. Using 'last'.", type, line, filename);
    return STATSD_APP_CHART_DIM_VALUE_TYPE_LAST;
}

static const char *valuetype2string(STATSD_APP_CHART_DIM_VALUE_TYPE type) {
    switch(type) {
        case STATSD_APP_CHART_DIM_VALUE_TYPE_EVENTS: return "events";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_LAST: return "last";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_MIN: return "min";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_MAX: return "max";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_SUM: return "sum";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_AVERAGE: return "average";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_MEDIAN: return "median";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_STDDEV: return "stddev";
        case STATSD_APP_CHART_DIM_VALUE_TYPE_PERCENTILE: return "percentile";
    }

    return "unknown";
}

static STATSD_APP_CHART_DIM *add_dimension_to_app_chart(
        STATSD_APP *app __maybe_unused
        , STATSD_APP_CHART *chart
        , const char *metric_name
        , const char *dim_name
        , collected_number multiplier
        , collected_number divisor
        , RRDDIM_FLAGS flags
        , STATSD_APP_CHART_DIM_VALUE_TYPE value_type
) {
    STATSD_APP_CHART_DIM *dim = callocz(sizeof(STATSD_APP_CHART_DIM), 1);

    dim->metric = strdupz(metric_name);
    dim->metric_hash = simple_hash(dim->metric);

    dim->name = strdupz((dim_name)?dim_name:"");
    dim->multiplier = multiplier;
    dim->divisor = divisor;
    dim->value_type = value_type;
    dim->flags = flags;

    if(!dim->multiplier)
        dim->multiplier = 1;

    if(!dim->divisor)
        dim->divisor = 1;

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

    return dim;
}

static int statsd_readfile(const char *filename, STATSD_APP *app, STATSD_APP_CHART *chart, DICTIONARY *dict) {
    debug(D_STATSD, "STATSD configuration reading file '%s'", filename);

    char *buffer = mallocz(STATSD_CONF_LINE_MAX + 1);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        error("STATSD: cannot open file '%s'.", filename);
        freez(buffer);
        return -1;
    }

    size_t line = 0;
    char *s;
    while(fgets(buffer, STATSD_CONF_LINE_MAX, fp) != NULL) {
        buffer[STATSD_CONF_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if (!s || *s == '#') {
            debug(D_STATSD, "STATSD: ignoring line %zu of file '%s', it is empty.", line, filename);
            continue;
        }

        debug(D_STATSD, "STATSD: processing line %zu of file '%s': %s", line, filename, buffer);

        if(*s == 'i' && strncmp(s, "include", 7) == 0) {
            s = trim(&s[7]);
            if(s && *s) {
                char *tmp;
                if(*s == '/')
                    tmp = strdupz(s);
                else {
                    // the file to be included is relative to current file
                    // find the directory name from the file we already read
                    char *filename2 = strdupz(filename); // copy filename, since dirname() will change it
                    char *dir = dirname(filename2);      // find the directory part of the filename
                    tmp = strdupz_path_subpath(dir, s);  // compose the new filename to read;
                    freez(filename2);                    // free the filename we copied
                }
                statsd_readfile(tmp, app, chart, dict);
                freez(tmp);
            }
            else
                error("STATSD: ignoring line %zu of file '%s', include filename is empty", line, filename);

            continue;
        }

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
                dict = NULL;

                {
                    char lineandfile[FILENAME_MAX + 1];
                    snprintfz(lineandfile, FILENAME_MAX, "%zu@%s", line, filename);
                    app->source = strdupz(lineandfile);
                }
            }
            else if(app) {
                if(!strcmp(s, "dictionary")) {
                    if(!app->dict)
                        app->dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);

                    dict = app->dict;
                }
                else {
                    dict = NULL;

                    // a new chart
                    chart = callocz(sizeof(STATSD_APP_CHART), 1);
                    netdata_fix_chart_id(s);
                    chart->id         = strdupz(s);
                    chart->name       = strdupz(s);
                    chart->title      = strdupz("Statsd chart");
                    chart->context    = strdupz(s);
                    chart->family     = strdupz("overview");
                    chart->units      = strdupz("value");
                    chart->priority   = NETDATA_CHART_PRIO_STATSD_PRIVATE;
                    chart->chart_type = RRDSET_TYPE_LINE;

                    chart->next = app->charts;
                    app->charts = chart;

                    if (!strncmp(
                            filename,
                            netdata_configured_stock_config_dir,
                            strlen(netdata_configured_stock_config_dir))) {
                        char tmpfilename[FILENAME_MAX + 1];
                        strncpyz(tmpfilename, filename, FILENAME_MAX);
                        chart->module = strdupz(basename(tmpfilename));
                    } else {
                        chart->module = strdupz("synthetic_chart");
                    }
                }
            }
            else
                error("STATSD: ignoring line %zu ('%s') of file '%s', [app] is not defined.", line, s, filename);

            continue;
        }

        if(!app) {
            error("STATSD: ignoring line %zu ('%s') of file '%s', it is outside all sections.", line, s, filename);
            continue;
        }

        char *name = s;
        char *value = strchr(s, '=');
        if(!value) {
            error("STATSD: ignoring line %zu ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(!name || *name == '#') {
            error("STATSD: ignoring line %zu of file '%s', name is empty.", line, filename);
            continue;
        }
        if(!value) {
            debug(D_CONFIG, "STATSD: ignoring line %zu of file '%s', value is empty.", line, filename);
            continue;
        }

        if(unlikely(dict)) {
            // parse [dictionary] members

            dictionary_set(dict, name, value, strlen(value) + 1);
        }
        else if(!chart) {
            // parse [app] members

            if(!strcmp(name, "name")) {
                freez((void *)app->name);
                netdata_fix_chart_name(value);
                app->name = strdupz(value);
            }
            else if (!strcmp(name, "metrics")) {
                simple_pattern_free(app->metrics);
                app->metrics = simple_pattern_create(value, NULL, SIMPLE_PATTERN_EXACT);
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
                error("STATSD: ignoring line %zu ('%s') of file '%s'. Unknown keyword for the [app] section.", line, name, filename);
                continue;
            }
        }
        else {
            // parse [chart] members

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
                char *words[10];
                pluginsd_split_words(value, words, 10, NULL, NULL, 0);

                int pattern = 0;
                size_t i = 0;
                char *metric_name   = words[i++];

                if(strcmp(metric_name, "pattern") == 0) {
                    metric_name = words[i++];
                    pattern = 1;
                }

                char *dim_name      = words[i++];
                char *type          = words[i++];
                char *multiplier    = words[i++];
                char *divisor       = words[i++];
                char *options       = words[i++];

                RRDDIM_FLAGS flags = RRDDIM_FLAG_NONE;
                if(options && *options) {
                    if(strstr(options, "hidden") != NULL) flags |= RRDDIM_FLAG_HIDDEN;
                    if(strstr(options, "noreset") != NULL) flags |= RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS;
                    if(strstr(options, "nooverflow") != NULL) flags |= RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS;
                }

                if(!pattern) {
                    if(app->dict) {
                        if(dim_name && *dim_name) {
                            char *n = dictionary_get(app->dict, dim_name);
                            if(n) dim_name = n;
                        }
                        else {
                            dim_name = dictionary_get(app->dict, metric_name);
                        }
                    }

                    if(!dim_name || !*dim_name)
                        dim_name = metric_name;
                }

                STATSD_APP_CHART_DIM *dim = add_dimension_to_app_chart(
                        app
                        , chart
                        , metric_name
                        , dim_name
                        , (multiplier && *multiplier)?str2l(multiplier):1
                        , (divisor && *divisor)?str2l(divisor):1
                        , flags
                        , string2valuetype(type, line, filename)
                );

                if(pattern)
                    dim->metric_pattern = simple_pattern_create(dim->metric, NULL, SIMPLE_PATTERN_EXACT);
            }
            else {
                error("STATSD: ignoring line %zu ('%s') of file '%s'. Unknown keyword for the [%s] section.", line, name, filename, chart->id);
                continue;
            }
        }
    }

    freez(buffer);
    fclose(fp);
    return 0;
}

static int statsd_file_callback(const char *filename, void *data) {
    (void)data;
    return statsd_readfile(filename, NULL, NULL, NULL);
}

static inline void statsd_readdir(const char *user_path, const char *stock_path, const char *subpath) {
    recursive_config_double_dir_load(user_path, stock_path, subpath, statsd_file_callback, NULL, 0);
}

// --------------------------------------------------------------------------------------------------------------------
// send metrics to netdata - in private charts - called from the main thread

// extract chart type and chart id from metric name
static inline void statsd_get_metric_type_and_id(STATSD_METRIC *m, char *type, char *id, char *context, const char *metrictype, size_t len) {

    // The full chart type.id looks like this:
    // ${STATSD_CHART_PREFIX} + "_" + ${METRIC_NAME} + "_" + ${METRIC_TYPE}
    //
    // where:
    // STATSD_CHART_PREFIX = "statsd" as defined above
    // METRIC_NAME = whatever the user gave to statsd
    // METRIC_TYPE = "gauge", "counter", "meter", "timer", "histogram", "set", "dictionary"

    // for chart type, we want:
    // ${STATSD_CHART_PREFIX} + "_" + the first word of ${METRIC_NAME}

    // find the first word of ${METRIC_NAME}
    char firstword[len + 1], *s = "";
    strncpyz(firstword, m->name, len);
    for (s = firstword; *s ; s++) {
        if (unlikely(*s == '.' || *s == '_')) {
            *s = '\0';
            s++;
            break;
        }
    }
    // firstword has the first word of ${METRIC_NAME}
    // s has the remaining, if any

    // create the chart type:
    snprintfz(type, len, STATSD_CHART_PREFIX "_%s", firstword);

    // for chart id, we want:
    // the remaining of the words of ${METRIC_NAME} + "_" + ${METRIC_TYPE}
    // or the ${METRIC_NAME} has no remaining words, the ${METRIC_TYPE} alone
    if(*s)
        snprintfz(id, len, "%s_%s", s, metrictype);
    else
        snprintfz(id, len, "%s", metrictype);

    // for the context, we want the full of both the above, separated with a dot (type.id):
    snprintfz(context, RRD_ID_LENGTH_MAX, "%s.%s", type, id);

    // make sure they don't have illegal characters
    netdata_fix_chart_id(type);
    netdata_fix_chart_id(id);
    netdata_fix_chart_id(context);
}

static inline RRDSET *statsd_private_rrdset_create(
        STATSD_METRIC *m __maybe_unused
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
            , PLUGIN_STATSD_NAME // plugin
            , "private_chart" // module
            , priority        // priority
            , update_every    // update every
            , chart_type      // chart type
            , default_rrd_memory_mode     // memory mode
            , default_rrd_history_entries // history
    );
    rrdset_flag_set(st, RRDSET_FLAG_STORE_FIRST);

    if(statsd.private_charts_hidden)
        rrdset_flag_set(st, RRDSET_FLAG_HIDDEN);

    // rrdset_flag_set(st, RRDSET_FLAG_DEBUG);
    return st;
}

static inline void statsd_private_chart_gauge(STATSD_METRIC *m) {
    debug(D_STATSD, "updating private chart for gauge metric '%s'", m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, context, "gauge", RRD_ID_LENGTH_MAX);

        char title[RRD_ID_LENGTH_MAX + 1];
        snprintfz(title, RRD_ID_LENGTH_MAX, "statsd private chart for gauge %s", m->name);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , m->family?m->family:"gauges"      // family (submenu)
                , context       // context
                , title         // title
                , m->units?m->units:"value"       // units
                , NETDATA_CHART_PRIO_STATSD_PRIVATE
                , statsd.update_every
                , RRDSET_TYPE_LINE
        );

        m->rd_value = rrddim_add(m->st, "gauge",  m->dimname?m->dimname:NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);

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
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, context, dim, RRD_ID_LENGTH_MAX);

        char title[RRD_ID_LENGTH_MAX + 1];
        snprintfz(title, RRD_ID_LENGTH_MAX, "statsd private chart for %s %s", dim, m->name);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , m->family?m->family:family        // family (submenu)
                , context       // context
                , title         // title
                , m->units?m->units:"events/s"    // units
                , NETDATA_CHART_PRIO_STATSD_PRIVATE
                , statsd.update_every
                , RRDSET_TYPE_AREA
        );

        m->rd_value = rrddim_add(m->st, dim, m->dimname?m->dimname:NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

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
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, context, "set", RRD_ID_LENGTH_MAX);

        char title[RRD_ID_LENGTH_MAX + 1];
        snprintfz(title, RRD_ID_LENGTH_MAX, "statsd private chart for set %s", m->name);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , m->family?m->family:"sets"        // family (submenu)
                , context       // context
                , title         // title
                , m->units?m->units:"entries"     // units
                , NETDATA_CHART_PRIO_STATSD_PRIVATE
                , statsd.update_every
                , RRDSET_TYPE_LINE
        );

        m->rd_value = rrddim_add(m->st, "set", m->dimname?m->dimname:"unique", 1, 1, RRD_ALGORITHM_ABSOLUTE);

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    rrddim_set_by_pointer(m->st, m->rd_value, m->last);

    if(m->rd_count)
        rrddim_set_by_pointer(m->st, m->rd_count, m->events);

    rrdset_done(m->st);
}

static inline void statsd_private_chart_dictionary(STATSD_METRIC *m) {
    debug(D_STATSD, "updating private chart for dictionary metric '%s'", m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, context, "dictionary", RRD_ID_LENGTH_MAX);

        char title[RRD_ID_LENGTH_MAX + 1];
        snprintfz(title, RRD_ID_LENGTH_MAX, "statsd private chart for dictionary %s", m->name);

        m->st = statsd_private_rrdset_create(
            m
            , type
            , id
            , NULL          // name
            , m->family?m->family:"dictionaries" // family (submenu)
            , context       // context
            , title         // title
            , m->units?m->units:"events/s"     // units
            , NETDATA_CHART_PRIO_STATSD_PRIVATE
            , statsd.update_every
            , RRDSET_TYPE_STACKED
        );

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    STATSD_METRIC_DICTIONARY_ITEM *t;
    dfe_start_read(m->dictionary.dict, t) {
        if (!t->rd) t->rd = rrddim_add(m->st, t_name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rrddim_set_by_pointer(m->st, t->rd, (collected_number)t->count);
    }
    dfe_done(t);

    if(m->rd_count)
        rrddim_set_by_pointer(m->st, m->rd_count, m->events);

    rrdset_done(m->st);
}

static inline void statsd_private_chart_timer_or_histogram(STATSD_METRIC *m, const char *dim, const char *family, const char *units) {
    debug(D_STATSD, "updating private chart for %s metric '%s'", dim, m->name);

    if(unlikely(!m->st)) {
        char type[RRD_ID_LENGTH_MAX + 1], id[RRD_ID_LENGTH_MAX + 1], context[RRD_ID_LENGTH_MAX + 1];
        statsd_get_metric_type_and_id(m, type, id, context, dim, RRD_ID_LENGTH_MAX);

        char title[RRD_ID_LENGTH_MAX + 1];
        snprintfz(title, RRD_ID_LENGTH_MAX, "statsd private chart for %s %s", dim, m->name);

        m->st = statsd_private_rrdset_create(
                m
                , type
                , id
                , NULL          // name
                , m->family?m->family:family        // family (submenu)
                , context       // context
                , title         // title
                , m->units?m->units:units         // units
                , NETDATA_CHART_PRIO_STATSD_PRIVATE
                , statsd.update_every
                , RRDSET_TYPE_AREA
        );

        m->histogram.ext->rd_min = rrddim_add(m->st, "min", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_max = rrddim_add(m->st, "max", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->rd_value              = rrddim_add(m->st, "average", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_percentile = rrddim_add(m->st, statsd.histogram_percentile_str, NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_median = rrddim_add(m->st, "median", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        m->histogram.ext->rd_stddev = rrddim_add(m->st, "stddev", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);
        //m->histogram.ext->rd_sum = rrddim_add(m->st, "sum", NULL, 1, statsd.decimal_detail, RRD_ALGORITHM_ABSOLUTE);

        if(m->options & STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT)
            m->rd_count = rrddim_add(m->st, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else rrdset_next(m->st);

    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_min, m->histogram.ext->last_min);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_max, m->histogram.ext->last_max);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_percentile, m->histogram.ext->last_percentile);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_median, m->histogram.ext->last_median);
    rrddim_set_by_pointer(m->st, m->histogram.ext->rd_stddev, m->histogram.ext->last_stddev);
    //rrddim_set_by_pointer(m->st, m->histogram.ext->rd_sum, m->histogram.ext->last_sum);
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
    if(unlikely(!m->reset && m->count)) {
        m->last = (collected_number) (m->gauge.value * statsd.decimal_detail);

        m->reset = 1;
        updated = 1;
    }

    if(unlikely(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED))))
        statsd_private_chart_gauge(m);
}

static inline void statsd_flush_counter_or_meter(STATSD_METRIC *m, const char *dim, const char *family) {
    debug(D_STATSD, "flushing %s metric '%s'", dim, m->name);

    int updated = 0;
    if(unlikely(!m->reset && m->count)) {
        m->last = m->counter.value;

        m->reset = 1;
        updated = 1;
    }

    if(unlikely(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED))))
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
    if(unlikely(!m->reset && m->count)) {
        m->last = (collected_number)m->set.unique;

        m->reset = 1;
        updated = 1;
    }
    else {
        m->last = 0;
    }

    if(unlikely(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED))))
        statsd_private_chart_set(m);
}

static inline void statsd_flush_dictionary(STATSD_METRIC *m) {
    debug(D_STATSD, "flushing dictionary metric '%s'", m->name);

    int updated = 0;
    if(unlikely(!m->reset && m->count)) {
        m->last = (collected_number)m->dictionary.unique;

        m->reset = 1;
        updated = 1;
    }
    else {
        m->last = 0;
    }

    if(unlikely(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED))))
        statsd_private_chart_dictionary(m);

    if(m->dictionary.unique >= statsd.dictionary_max_unique) {
        if(!(m->options & STATSD_METRIC_OPTION_COLLECTION_FULL_LOGGED)) {
            m->options |= STATSD_METRIC_OPTION_COLLECTION_FULL_LOGGED;
            info(
                "STATSD dictionary '%s' reach max of %zu items - try increasing 'dictionaries max unique dimensions' in netdata.conf",
                m->name,
                m->dictionary.unique);
        }
    }
}

static inline void statsd_flush_timer_or_histogram(STATSD_METRIC *m, const char *dim, const char *family, const char *units) {
    debug(D_STATSD, "flushing %s metric '%s'", dim, m->name);

    int updated = 0;
    if(unlikely(!m->reset && m->count && m->histogram.ext->used > 0)) {
        netdata_mutex_lock(&m->histogram.ext->mutex);

        size_t len = m->histogram.ext->used;
        NETDATA_DOUBLE *series = m->histogram.ext->values;
        sort_series(series, len);

        m->histogram.ext->last_min = (collected_number)roundndd(series[0] * statsd.decimal_detail);
        m->histogram.ext->last_max = (collected_number)roundndd(series[len - 1] * statsd.decimal_detail);
        m->last = (collected_number)roundndd(average(series, len) * statsd.decimal_detail);
        m->histogram.ext->last_median = (collected_number)roundndd(median_on_sorted_series(series, len) * statsd.decimal_detail);
        m->histogram.ext->last_stddev = (collected_number)roundndd(standard_deviation(series, len) * statsd.decimal_detail);
        m->histogram.ext->last_sum = (collected_number)roundndd(sum(series, len) * statsd.decimal_detail);

        size_t pct_len = (size_t)floor((double)len * statsd.histogram_percentile / 100.0);
        if(pct_len < 1)
            m->histogram.ext->last_percentile = (collected_number)(series[0] * statsd.decimal_detail);
        else
            m->histogram.ext->last_percentile = (collected_number)roundndd(series[pct_len - 1] * statsd.decimal_detail);

        netdata_mutex_unlock(&m->histogram.ext->mutex);

        debug(D_STATSD, "STATSD %s metric %s: min " COLLECTED_NUMBER_FORMAT ", max " COLLECTED_NUMBER_FORMAT ", last " COLLECTED_NUMBER_FORMAT ", pcent " COLLECTED_NUMBER_FORMAT ", median " COLLECTED_NUMBER_FORMAT ", stddev " COLLECTED_NUMBER_FORMAT ", sum " COLLECTED_NUMBER_FORMAT,
              dim, m->name, m->histogram.ext->last_min, m->histogram.ext->last_max, m->last, m->histogram.ext->last_percentile, m->histogram.ext->last_median, m->histogram.ext->last_stddev, m->histogram.ext->last_sum);

        m->histogram.ext->zeroed = 0;
        m->reset = 1;
        updated = 1;
    }
    else if(unlikely(!m->histogram.ext->zeroed)) {
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

    if(unlikely(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED && (updated || !(m->options & STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED))))
        statsd_private_chart_timer_or_histogram(m, dim, family, units);
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
        case STATSD_METRIC_TYPE_DICTIONARY:
            return RRD_ALGORITHM_INCREMENTAL;
    }
}

static inline void link_metric_to_app_dimension(STATSD_APP *app, STATSD_METRIC *m, STATSD_APP_CHART *chart, STATSD_APP_CHART_DIM *dim) {
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
    m->options |= STATSD_METRIC_OPTION_USED_IN_APPS;
    debug(D_STATSD, "metric '%s' of type %u linked with app '%s', chart '%s', dimension '%s', algorithm '%s'", m->name, m->type, app->name, chart->id, dim->name, rrd_algorithm_name(dim->algorithm));
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
                    if(unlikely(dim->metric_pattern)) {
                        size_t dim_name_len = strlen(dim->name);
                        size_t wildcarded_len = dim_name_len + strlen(m->name) + 1;
                        char wildcarded[wildcarded_len];

                        strcpy(wildcarded, dim->name);
                        char *ws = &wildcarded[dim_name_len];

                        if(simple_pattern_matches_extract(dim->metric_pattern, m->name, ws, wildcarded_len - dim_name_len)) {

                            char *final_name = NULL;

                            if(app->dict) {
                                if(likely(*wildcarded)) {
                                    // use the name of the wildcarded string
                                    final_name = dictionary_get(app->dict, wildcarded);
                                }

                                if(unlikely(!final_name)) {
                                    // use the name of the metric
                                    final_name = dictionary_get(app->dict, m->name);
                                }
                            }

                            if(unlikely(!final_name))
                                final_name = wildcarded;

                            add_dimension_to_app_chart(
                                    app
                                    , chart
                                    , m->name
                                    , final_name
                                    , dim->multiplier
                                    , dim->divisor
                                    , dim->flags
                                    , dim->value_type
                            );

                            // the new dimension is appended to the list
                            // so, it will be matched and linked later too
                        }
                    }
                    else if(!dim->value_ptr && dim->metric_hash == m->hash && !strcmp(dim->metric, m->name)) {
                        // we have a match - this metric should be linked to this dimension
                        link_metric_to_app_dimension(app, m, chart, dim);
                    }
                }

            }
        }
    }
}

static inline RRDDIM *statsd_add_dim_to_app_chart(STATSD_APP *app, STATSD_APP_CHART *chart, STATSD_APP_CHART_DIM *dim) {
    (void)app;

    // allow the same statsd metric to be added multiple times to the same chart

    STATSD_APP_CHART_DIM *tdim;
    size_t count_same_metric = 0, count_same_metric_value_type = 0;
    size_t pos_same_metric_value_type = 0;

    for (tdim = chart->dimensions; tdim && tdim->next; tdim = tdim->next) {
        if (dim->metric_hash == tdim->metric_hash && !strcmp(dim->metric, tdim->metric)) {
            count_same_metric++;

            if(dim->value_type == tdim->value_type) {
                count_same_metric_value_type++;
                if (tdim == dim)
                    pos_same_metric_value_type = count_same_metric_value_type;
            }
        }
    }

    if(count_same_metric > 1) {
        // the same metric is found multiple times

        size_t len = strlen(dim->metric) + 100;
        char metric[ len + 1 ];

        if(count_same_metric_value_type > 1) {
            // the same metric, with the same value type, is added multiple times
            snprintfz(metric, len, "%s_%s%zu", dim->metric, valuetype2string(dim->value_type), pos_same_metric_value_type);
        }
        else {
            // the same metric, with different value type is added
            snprintfz(metric, len, "%s_%s", dim->metric, valuetype2string(dim->value_type));
        }

        dim->rd = rrddim_add(chart->st, metric, dim->name, dim->multiplier, dim->divisor, dim->algorithm);
        if(dim->flags != RRDDIM_FLAG_NONE) dim->rd->flags |= dim->flags;
        return dim->rd;
    }

    dim->rd = rrddim_add(chart->st, dim->metric, dim->name, dim->multiplier, dim->divisor, dim->algorithm);
    if(dim->flags != RRDDIM_FLAG_NONE) dim->rd->flags |= dim->flags;
    return dim->rd;
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
                , PLUGIN_STATSD_NAME        // plugin
                , chart->module             // module
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
        if(likely(!dim->metric_pattern)) {
            if (unlikely(!dim->rd))
                statsd_add_dim_to_app_chart(app, chart, dim);

            if (unlikely(dim->value_ptr)) {
                debug(D_STATSD, "updating dimension '%s' (%s) of chart '%s' (%s) for app '%s' with value " COLLECTED_NUMBER_FORMAT, dim->name, rrddim_id(dim->rd), chart->id, rrdset_id(chart->st), app->name, *dim->value_ptr);
                rrddim_set_by_pointer(chart->st, dim->rd, *dim->value_ptr);
            }
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

const char *statsd_metric_type_string(STATSD_METRIC_TYPE type) {
    switch(type) {
        case STATSD_METRIC_TYPE_COUNTER: return "counter";
        case STATSD_METRIC_TYPE_GAUGE: return "gauge";
        case STATSD_METRIC_TYPE_HISTOGRAM: return "histogram";
        case STATSD_METRIC_TYPE_METER: return "meter";
        case STATSD_METRIC_TYPE_SET: return "set";
        case STATSD_METRIC_TYPE_DICTIONARY: return "dictionary";
        case STATSD_METRIC_TYPE_TIMER: return "timer";
        default: return "unknown";
    }
}

static inline void statsd_flush_index_metrics(STATSD_INDEX *index, void (*flush_metric)(STATSD_METRIC *)) {
    STATSD_METRIC *m;

    // find the useful metrics (incremental = each time we are called, we check the new metrics only)
    dfe_start_read(index->dict, m) {
        // since we add new metrics at the beginning
        // check for useful charts, until the point we last checked
        if(unlikely(is_metric_checked(m))) break;

        if(unlikely(!(m->options & STATSD_METRIC_OPTION_CHECKED_IN_APPS))) {
            log_access("NEW STATSD METRIC '%s': '%s'", statsd_metric_type_string(m->type), m->name);
            check_if_metric_is_for_app(index, m);
            m->options |= STATSD_METRIC_OPTION_CHECKED_IN_APPS;
        }

        if(unlikely(!(m->options & STATSD_METRIC_OPTION_PRIVATE_CHART_CHECKED))) {
            if(unlikely(statsd.private_charts >= statsd.max_private_charts_hard)) {
                debug(D_STATSD, "STATSD: metric '%s' will not be charted, because the hard limit of the maximum number of charts has been reached.", m->name);
                info("STATSD: metric '%s' will not be charted, because the hard limit of the maximum number of charts (%zu) has been reached. Increase the number of charts by editing netdata.conf, [statsd] section.", m->name, statsd.max_private_charts_hard);
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

        // mark it as checked
        m->options |= STATSD_METRIC_OPTION_CHECKED;

        // check if it is used in charts
        if((m->options & (STATSD_METRIC_OPTION_PRIVATE_CHART_ENABLED|STATSD_METRIC_OPTION_USED_IN_APPS)) && !(m->options & STATSD_METRIC_OPTION_USEFUL)) {
            m->options |= STATSD_METRIC_OPTION_USEFUL;
            index->useful++;
            m->next_useful = index->first_useful;
            index->first_useful = m;
        }
    }
    dfe_done(m);

    // flush all the useful metrics
    for(m = index->first_useful; m ; m = m->next_useful) {
        flush_metric(m);
    }
}


// --------------------------------------------------------------------------------------
// statsd main thread

static int statsd_listen_sockets_setup(void) {
    return listen_sockets_setup(&statsd.sockets);
}

static void statsd_main_cleanup(void *data) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)data;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;
    info("cleaning up...");

    if (statsd.collection_threads_status) {
        int i;
        for (i = 0; i < statsd.threads; i++) {
            if(statsd.collection_threads_status[i].status) {
                info("STATSD: stopping data collection thread %d...", i + 1);
                netdata_thread_cancel(statsd.collection_threads_status[i].thread);
            }
            else {
                info("STATSD: data collection thread %d found stopped.", i + 1);
            }
        }
    }

    info("STATSD: closing sockets...");
    listen_sockets_close(&statsd.sockets);

    // destroy the dictionaries
    dictionary_destroy(statsd.gauges.dict);
    dictionary_destroy(statsd.meters.dict);
    dictionary_destroy(statsd.counters.dict);
    dictionary_destroy(statsd.histograms.dict);
    dictionary_destroy(statsd.dictionaries.dict);
    dictionary_destroy(statsd.sets.dict);
    dictionary_destroy(statsd.timers.dict);

    info("STATSD: cleanup completed.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;

    worker_unregister();
}

#define WORKER_STATSD_FLUSH_GAUGES 0
#define WORKER_STATSD_FLUSH_COUNTERS 1
#define WORKER_STATSD_FLUSH_METERS 2
#define WORKER_STATSD_FLUSH_TIMERS 3
#define WORKER_STATSD_FLUSH_HISTOGRAMS 4
#define WORKER_STATSD_FLUSH_SETS 5
#define WORKER_STATSD_FLUSH_DICTIONARIES 6
#define WORKER_STATSD_FLUSH_STATS 7

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 8
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 8
#endif

void *statsd_main(void *ptr) {
    worker_register("STATSDFLUSH");
    worker_register_job_name(WORKER_STATSD_FLUSH_GAUGES, "gauges");
    worker_register_job_name(WORKER_STATSD_FLUSH_COUNTERS, "counters");
    worker_register_job_name(WORKER_STATSD_FLUSH_METERS, "meters");
    worker_register_job_name(WORKER_STATSD_FLUSH_TIMERS, "timers");
    worker_register_job_name(WORKER_STATSD_FLUSH_HISTOGRAMS, "histograms");
    worker_register_job_name(WORKER_STATSD_FLUSH_SETS, "sets");
    worker_register_job_name(WORKER_STATSD_FLUSH_DICTIONARIES, "dictionaries");
    worker_register_job_name(WORKER_STATSD_FLUSH_STATS, "statistics");

    netdata_thread_cleanup_push(statsd_main_cleanup, ptr);

    statsd.gauges.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);
    statsd.meters.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);
    statsd.counters.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);
    statsd.histograms.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);
    statsd.dictionaries.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);
    statsd.sets.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);
    statsd.timers.dict = dictionary_create(STATSD_DICTIONARY_OPTIONS);

    dictionary_register_insert_callback(statsd.gauges.dict, dictionary_metric_insert_callback, &statsd.gauges);
    dictionary_register_insert_callback(statsd.meters.dict, dictionary_metric_insert_callback, &statsd.meters);
    dictionary_register_insert_callback(statsd.counters.dict, dictionary_metric_insert_callback, &statsd.counters);
    dictionary_register_insert_callback(statsd.histograms.dict, dictionary_metric_insert_callback, &statsd.histograms);
    dictionary_register_insert_callback(statsd.dictionaries.dict, dictionary_metric_insert_callback, &statsd.dictionaries);
    dictionary_register_insert_callback(statsd.sets.dict, dictionary_metric_insert_callback, &statsd.sets);
    dictionary_register_insert_callback(statsd.timers.dict, dictionary_metric_insert_callback, &statsd.timers);

    dictionary_register_delete_callback(statsd.gauges.dict, dictionary_metric_delete_callback, &statsd.gauges);
    dictionary_register_delete_callback(statsd.meters.dict, dictionary_metric_delete_callback, &statsd.meters);
    dictionary_register_delete_callback(statsd.counters.dict, dictionary_metric_delete_callback, &statsd.counters);
    dictionary_register_delete_callback(statsd.histograms.dict, dictionary_metric_delete_callback, &statsd.histograms);
    dictionary_register_delete_callback(statsd.dictionaries.dict, dictionary_metric_delete_callback, &statsd.dictionaries);
    dictionary_register_delete_callback(statsd.sets.dict, dictionary_metric_delete_callback, &statsd.sets);
    dictionary_register_delete_callback(statsd.timers.dict, dictionary_metric_delete_callback, &statsd.timers);

    // ----------------------------------------------------------------------------------------------------------------
    // statsd configuration

    statsd.enabled = config_get_boolean(CONFIG_SECTION_PLUGINS, "statsd", statsd.enabled);

    statsd.update_every = default_rrd_update_every;
    statsd.update_every = (int)config_get_number(CONFIG_SECTION_STATSD, "update every (flushInterval)", statsd.update_every);
    if(statsd.update_every < default_rrd_update_every) {
        error("STATSD: minimum flush interval %d given, but the minimum is the update every of netdata. Using %d", statsd.update_every, default_rrd_update_every);
        statsd.update_every = default_rrd_update_every;
    }

#ifdef HAVE_RECVMMSG
    statsd.recvmmsg_size = (size_t)config_get_number(CONFIG_SECTION_STATSD, "udp messages to process at once", (long long)statsd.recvmmsg_size);
#endif

    statsd.charts_for = simple_pattern_create(config_get(CONFIG_SECTION_STATSD, "create private charts for metrics matching", "*"), NULL, SIMPLE_PATTERN_EXACT);
    statsd.max_private_charts_hard = (size_t)config_get_number(CONFIG_SECTION_STATSD, "max private charts hard limit", (long long)statsd.max_private_charts_hard);
    statsd.private_charts_rrd_history_entries = (int)config_get_number(CONFIG_SECTION_STATSD, "private charts history", default_rrd_history_entries);
    statsd.decimal_detail = (collected_number)config_get_number(CONFIG_SECTION_STATSD, "decimal detail", (long long int)statsd.decimal_detail);
    statsd.tcp_idle_timeout = (size_t) config_get_number(CONFIG_SECTION_STATSD, "disconnect idle tcp clients after seconds", (long long int)statsd.tcp_idle_timeout);
    statsd.private_charts_hidden = (unsigned int)config_get_boolean(CONFIG_SECTION_STATSD, "private charts hidden", statsd.private_charts_hidden);

    statsd.histogram_percentile = (double)config_get_float(CONFIG_SECTION_STATSD, "histograms and timers percentile (percentThreshold)", statsd.histogram_percentile);
    if(isless(statsd.histogram_percentile, 0) || isgreater(statsd.histogram_percentile, 100)) {
        error("STATSD: invalid histograms and timers percentile %0.5f given", statsd.histogram_percentile);
        statsd.histogram_percentile = 95.0;
    }
    {
        char buffer[314 + 1];
        snprintfz(buffer, 314, "%0.1f%%", statsd.histogram_percentile);
        statsd.histogram_percentile_str = strdupz(buffer);
    }

    statsd.dictionary_max_unique = config_get_number(CONFIG_SECTION_STATSD, "dictionaries max unique dimensions", statsd.dictionary_max_unique);

    if(config_get_boolean(CONFIG_SECTION_STATSD, "add dimension for number of events received", 0)) {
        statsd.gauges.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.counters.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.meters.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.sets.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.histograms.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.timers.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
        statsd.dictionaries.default_options |= STATSD_METRIC_OPTION_CHART_DIMENSION_COUNT;
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

    if(config_get_boolean(CONFIG_SECTION_STATSD, "gaps on dictionaries (deleteDictionaries)", 0))
        statsd.dictionaries.default_options |= STATSD_METRIC_OPTION_SHOW_GAPS_WHEN_NOT_COLLECTED;

    size_t max_sockets = (size_t)config_get_number(CONFIG_SECTION_STATSD, "statsd server max TCP sockets", (long long int)(rlimit_nofile.rlim_cur / 4));

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
    statsd_readdir(netdata_configured_user_config_dir, netdata_configured_stock_config_dir, "statsd.d");

    // ----------------------------------------------------------------------------------------------------------------
    // statsd setup

    if(!statsd.enabled) goto cleanup;

    statsd_listen_sockets_setup();
    if(!statsd.sockets.opened) {
        error("STATSD: No statsd sockets to listen to. statsd will be disabled.");
        goto cleanup;
    }

    statsd.collection_threads_status = callocz((size_t)statsd.threads, sizeof(struct collection_thread_status));

    int i;
    for(i = 0; i < statsd.threads ;i++) {
        statsd.collection_threads_status[i].max_sockets = max_sockets / statsd.threads;
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "STATSD_COLLECTOR[%d]", i + 1);
        netdata_thread_create(&statsd.collection_threads_status[i].thread, tag, NETDATA_THREAD_OPTION_DEFAULT, statsd_collector_thread, &statsd.collection_threads_status[i]);
    }

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
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132010
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_metrics_gauge     = rrddim_add(st_metrics, "gauges", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_counter   = rrddim_add(st_metrics, "counters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_timer     = rrddim_add(st_metrics, "timers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_meter     = rrddim_add(st_metrics, "meters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_histogram = rrddim_add(st_metrics, "histograms", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_set       = rrddim_add(st_metrics, "sets", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_metrics_dictionary= rrddim_add(st_metrics, "dictionaries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *st_useful_metrics = rrdset_create_localhost(
            "netdata"
            , "statsd_useful_metrics"
            , NULL
            , "statsd"
            , NULL
            , "Useful metrics in the netdata statsd database"
            , "metrics"
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132010
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_useful_metrics_gauge     = rrddim_add(st_useful_metrics, "gauges", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_useful_metrics_counter   = rrddim_add(st_useful_metrics, "counters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_useful_metrics_timer     = rrddim_add(st_useful_metrics, "timers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_useful_metrics_meter     = rrddim_add(st_useful_metrics, "meters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_useful_metrics_histogram = rrddim_add(st_useful_metrics, "histograms", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_useful_metrics_set       = rrddim_add(st_useful_metrics, "sets", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    RRDDIM *rd_useful_metrics_dictionary= rrddim_add(st_useful_metrics, "dictionaries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *st_events = rrdset_create_localhost(
            "netdata"
            , "statsd_events"
            , NULL
            , "statsd"
            , NULL
            , "Events processed by the netdata statsd server"
            , "events/s"
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132011
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_events_gauge     = rrddim_add(st_events, "gauges", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_counter   = rrddim_add(st_events, "counters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_timer     = rrddim_add(st_events, "timers", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_meter     = rrddim_add(st_events, "meters", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_histogram = rrddim_add(st_events, "histograms", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_set       = rrddim_add(st_events, "sets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_events_dictionary= rrddim_add(st_events, "dictionaries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132012
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
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132013
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
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132014
            , statsd.update_every
            , RRDSET_TYPE_STACKED
    );
    RRDDIM *rd_packets_tcp = rrddim_add(st_packets, "tcp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_packets_udp = rrddim_add(st_packets, "udp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st_tcp_connects = rrdset_create_localhost(
            "netdata"
            , "tcp_connects"
            , NULL
            , "statsd"
            , NULL
            , "statsd server TCP connects and disconnects"
            , "events"
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132015
            , statsd.update_every
            , RRDSET_TYPE_LINE
    );
    RRDDIM *rd_tcp_connects = rrddim_add(st_tcp_connects, "connects", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    RRDDIM *rd_tcp_disconnects = rrddim_add(st_tcp_connects, "disconnects", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

    RRDSET *st_tcp_connected = rrdset_create_localhost(
            "netdata"
            , "tcp_connected"
            , NULL
            , "statsd"
            , NULL
            , "statsd server TCP connected sockets"
            , "sockets"
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132016
            , statsd.update_every
            , RRDSET_TYPE_LINE
    );
    RRDDIM *rd_tcp_connected = rrddim_add(st_tcp_connected, "connected", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    RRDSET *st_pcharts = rrdset_create_localhost(
            "netdata"
            , "private_charts"
            , NULL
            , "statsd"
            , NULL
            , "Private metric charts created by the netdata statsd server"
            , "charts"
            , PLUGIN_STATSD_NAME
            , "stats"
            , 132020
            , statsd.update_every
            , RRDSET_TYPE_AREA
    );
    RRDDIM *rd_pcharts = rrddim_add(st_pcharts, "charts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    // ----------------------------------------------------------------------------------------------------------------
    // statsd thread to turn metrics into charts

    usec_t step = statsd.update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!netdata_exit) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb, step);

        worker_is_busy(WORKER_STATSD_FLUSH_GAUGES);
        statsd_flush_index_metrics(&statsd.gauges,     statsd_flush_gauge);

        worker_is_busy(WORKER_STATSD_FLUSH_COUNTERS);
        statsd_flush_index_metrics(&statsd.counters,   statsd_flush_counter);

        worker_is_busy(WORKER_STATSD_FLUSH_METERS);
        statsd_flush_index_metrics(&statsd.meters,     statsd_flush_meter);

        worker_is_busy(WORKER_STATSD_FLUSH_TIMERS);
        statsd_flush_index_metrics(&statsd.timers,     statsd_flush_timer);

        worker_is_busy(WORKER_STATSD_FLUSH_HISTOGRAMS);
        statsd_flush_index_metrics(&statsd.histograms, statsd_flush_histogram);

        worker_is_busy(WORKER_STATSD_FLUSH_SETS);
        statsd_flush_index_metrics(&statsd.sets,       statsd_flush_set);

        worker_is_busy(WORKER_STATSD_FLUSH_DICTIONARIES);
        statsd_flush_index_metrics(&statsd.dictionaries,statsd_flush_dictionary);

        worker_is_busy(WORKER_STATSD_FLUSH_STATS);
        statsd_update_all_app_charts();

        if(unlikely(netdata_exit))
            break;

        if(likely(hb_dt)) {
            rrdset_next(st_metrics);
            rrdset_next(st_useful_metrics);
            rrdset_next(st_events);
            rrdset_next(st_reads);
            rrdset_next(st_bytes);
            rrdset_next(st_packets);
            rrdset_next(st_tcp_connects);
            rrdset_next(st_tcp_connected);
            rrdset_next(st_pcharts);
        }

        rrddim_set_by_pointer(st_metrics, rd_metrics_gauge,        (collected_number)statsd.gauges.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_counter,      (collected_number)statsd.counters.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_timer,        (collected_number)statsd.timers.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_meter,        (collected_number)statsd.meters.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_histogram,    (collected_number)statsd.histograms.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_set,          (collected_number)statsd.sets.metrics);
        rrddim_set_by_pointer(st_metrics, rd_metrics_dictionary,   (collected_number)statsd.dictionaries.metrics);
        rrdset_done(st_metrics);

        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_gauge,        (collected_number)statsd.gauges.useful);
        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_counter,      (collected_number)statsd.counters.useful);
        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_timer,        (collected_number)statsd.timers.useful);
        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_meter,        (collected_number)statsd.meters.useful);
        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_histogram,    (collected_number)statsd.histograms.useful);
        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_set,          (collected_number)statsd.sets.useful);
        rrddim_set_by_pointer(st_useful_metrics, rd_useful_metrics_dictionary,   (collected_number)statsd.dictionaries.useful);
        rrdset_done(st_useful_metrics);

        rrddim_set_by_pointer(st_events,  rd_events_gauge,         (collected_number)statsd.gauges.events);
        rrddim_set_by_pointer(st_events,  rd_events_counter,       (collected_number)statsd.counters.events);
        rrddim_set_by_pointer(st_events,  rd_events_timer,         (collected_number)statsd.timers.events);
        rrddim_set_by_pointer(st_events,  rd_events_meter,         (collected_number)statsd.meters.events);
        rrddim_set_by_pointer(st_events,  rd_events_histogram,     (collected_number)statsd.histograms.events);
        rrddim_set_by_pointer(st_events,  rd_events_set,           (collected_number)statsd.sets.events);
        rrddim_set_by_pointer(st_events,  rd_events_dictionary,    (collected_number)statsd.dictionaries.events);
        rrddim_set_by_pointer(st_events,  rd_events_unknown,       (collected_number)statsd.unknown_types);
        rrddim_set_by_pointer(st_events,  rd_events_errors,        (collected_number)statsd.socket_errors);
        rrdset_done(st_events);

        rrddim_set_by_pointer(st_reads,   rd_reads_tcp,            (collected_number)statsd.tcp_socket_reads);
        rrddim_set_by_pointer(st_reads,   rd_reads_udp,            (collected_number)statsd.udp_socket_reads);
        rrdset_done(st_reads);

        rrddim_set_by_pointer(st_bytes,   rd_bytes_tcp,            (collected_number)statsd.tcp_bytes_read);
        rrddim_set_by_pointer(st_bytes,   rd_bytes_udp,            (collected_number)statsd.udp_bytes_read);
        rrdset_done(st_bytes);

        rrddim_set_by_pointer(st_packets, rd_packets_tcp,          (collected_number)statsd.tcp_packets_received);
        rrddim_set_by_pointer(st_packets, rd_packets_udp,          (collected_number)statsd.udp_packets_received);
        rrdset_done(st_packets);

        rrddim_set_by_pointer(st_tcp_connects, rd_tcp_connects,    (collected_number)statsd.tcp_socket_connects);
        rrddim_set_by_pointer(st_tcp_connects, rd_tcp_disconnects, (collected_number)statsd.tcp_socket_disconnects);
        rrdset_done(st_tcp_connects);

        rrddim_set_by_pointer(st_tcp_connected, rd_tcp_connected,  (collected_number)statsd.tcp_socket_connected);
        rrdset_done(st_tcp_connected);

        rrddim_set_by_pointer(st_pcharts, rd_pcharts,              (collected_number)statsd.private_charts);
        rrdset_done(st_pcharts);
    }

cleanup: ; // added semi-colon to prevent older gcc error: label at end of compound statement
    netdata_thread_cleanup_pop(1);
    return NULL;
}
