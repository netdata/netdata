#include "common.h"

static int statsd_threads = 0;

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

    const char *key;                // "type|name" for indexing
    uint32_t hash;                  // hash of the key

    const char *name;
    STATSD_METRIC_TYPE type;

    usec_t last_collected_ut;       // the last time this metric was updated
    usec_t last_exposed_ut;         // the last time this metric was sent to netdata
    size_t events;                  // the number of times this metrics has been collected

    size_t count;                   // number of events since the last exposure to netdata
    calculated_number last;         // the last value collected
    calculated_number total;        // the sum of all values collected since the last exposure to netdata
    calculated_number min;          // the min value collected since the last exposure to netdata
    calculated_number max;          // the max value collected since the last exposure to netdata

    RRDSET *st;
    RRDDIM *rd_min;
    RRDDIM *rd_max;
    RRDDIM *rd_avg;

    netdata_mutex_t mutex;

    struct statsd_metric *next;
} STATSD_METRIC;

static inline void statsd_collected_value(STATSD_METRIC *mt, calculated_number value, const char *options) {
    (void)options;

    int lock = 0;
    if(unlikely(statsd_threads > 1)) {
        netdata_mutex_lock(&mt->mutex);
        lock = 1;
    }

    mt->last_collected_ut = now_realtime_usec();
    mt->events++;
    mt->count++;

    switch(mt->type) {
        case STATSD_METRIC_TYPE_HISTOGRAM:
            // FIXME: not implemented yet

        case STATSD_METRIC_TYPE_GAUGE:
        case STATSD_METRIC_TYPE_TIMER:
        case STATSD_METRIC_TYPE_METER:
            mt->last = value;
            mt->total += value;
            if(value < mt->min)
                mt->min = value;
            if(value > mt->max)
                mt->max = value;
            break;

        case STATSD_METRIC_TYPE_COUNTER:
            mt->total = mt->last = value;
            break;
    }

    if(unlikely(lock))
        netdata_mutex_unlock(&mt->mutex);
}

static void statsd_process(char *buffer, size_t size) {
    buffer[size] = '\0';
    debug(D_STATSD, "RECEIVED: '%s'", buffer);
}

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
