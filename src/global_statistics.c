#include "common.h"

volatile struct global_statistics global_statistics = {
        .connected_clients = 0,
        .web_requests = 0,
        .web_usec = 0,
        .bytes_received = 0,
        .bytes_sent = 0,
        .content_size = 0,
        .compressed_content_size = 0
};

netdata_mutex_t global_statistics_mutex = NETDATA_MUTEX_INITIALIZER;

inline void global_statistics_lock(void) {
    netdata_mutex_lock(&global_statistics_mutex);
}

inline void global_statistics_unlock(void) {
    netdata_mutex_unlock(&global_statistics_mutex);
}

void finished_web_request_statistics(uint64_t dt,
                                     uint64_t bytes_received,
                                     uint64_t bytes_sent,
                                     uint64_t content_size,
                                     uint64_t compressed_content_size) {
#if defined(HAVE_C___ATOMIC) && !defined(NETDATA_NO_ATOMIC_INSTRUCTIONS)
    uint64_t old_web_usec_max = global_statistics.web_usec_max;
    while(dt > old_web_usec_max)
        __atomic_compare_exchange(&global_statistics.web_usec_max, &old_web_usec_max, &dt, 1, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);

    __atomic_fetch_add(&global_statistics.web_requests, 1, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.web_usec, dt, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.bytes_received, bytes_received, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.bytes_sent, bytes_sent, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.content_size, content_size, __ATOMIC_SEQ_CST);
    __atomic_fetch_add(&global_statistics.compressed_content_size, compressed_content_size, __ATOMIC_SEQ_CST);
#else
#warning NOT using atomic operations - using locks for global statistics
    if (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED)
        global_statistics_lock();

    if (dt > global_statistics.web_usec_max)
        global_statistics.web_usec_max = dt;

    global_statistics.web_requests++;
    global_statistics.web_usec += dt;
    global_statistics.bytes_received += bytes_received;
    global_statistics.bytes_sent += bytes_sent;
    global_statistics.content_size += content_size;
    global_statistics.compressed_content_size += compressed_content_size;

    if (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED)
        global_statistics_unlock();
#endif
}

void web_client_connected(void) {
#if defined(HAVE_C___ATOMIC) && !defined(NETDATA_NO_ATOMIC_INSTRUCTIONS)
    __atomic_fetch_add(&global_statistics.connected_clients, 1, __ATOMIC_SEQ_CST);
#else
    if (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED)
        global_statistics_lock();

    global_statistics.connected_clients++;

    if (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED)
        global_statistics_unlock();
#endif
}

void web_client_disconnected(void) {
#if defined(HAVE_C___ATOMIC) && !defined(NETDATA_NO_ATOMIC_INSTRUCTIONS)
    __atomic_fetch_sub(&global_statistics.connected_clients, 1, __ATOMIC_SEQ_CST);
#else
    if (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED)
        global_statistics_lock();

    global_statistics.connected_clients--;

    if (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED)
        global_statistics_unlock();
#endif
}


inline void global_statistics_copy(struct global_statistics *gs, uint8_t options) {
#if defined(HAVE_C___ATOMIC) && !defined(NETDATA_NO_ATOMIC_INSTRUCTIONS)
    gs->connected_clients       = __atomic_fetch_add(&global_statistics.connected_clients, 0, __ATOMIC_SEQ_CST);
    gs->web_requests            = __atomic_fetch_add(&global_statistics.web_requests, 0, __ATOMIC_SEQ_CST);
    gs->web_usec                = __atomic_fetch_add(&global_statistics.web_usec, 0, __ATOMIC_SEQ_CST);
    gs->web_usec_max            = __atomic_fetch_add(&global_statistics.web_usec_max, 0, __ATOMIC_SEQ_CST);
    gs->bytes_received          = __atomic_fetch_add(&global_statistics.bytes_received, 0, __ATOMIC_SEQ_CST);
    gs->bytes_sent              = __atomic_fetch_add(&global_statistics.bytes_sent, 0, __ATOMIC_SEQ_CST);
    gs->content_size            = __atomic_fetch_add(&global_statistics.content_size, 0, __ATOMIC_SEQ_CST);
    gs->compressed_content_size = __atomic_fetch_add(&global_statistics.compressed_content_size, 0, __ATOMIC_SEQ_CST);

    if(options & GLOBAL_STATS_RESET_WEB_USEC_MAX) {
        uint64_t n = 0;
        __atomic_compare_exchange(&global_statistics.web_usec_max, &gs->web_usec_max, &n, 1, __ATOMIC_SEQ_CST,
                                  __ATOMIC_SEQ_CST);
    }
#else
    global_statistics_lock();

    memcpy(gs, (const void *)&global_statistics, sizeof(struct global_statistics));

    if (options & GLOBAL_STATS_RESET_WEB_USEC_MAX)
        global_statistics.web_usec_max = 0;

    global_statistics_unlock();
#endif
}

void global_statistics_charts(void) {
    static unsigned long long old_web_requests = 0,
                              old_web_usec = 0,
                              old_content_size = 0,
                              old_compressed_content_size = 0;

    static collected_number compression_ratio = -1,
                            average_response_time = -1;

    static RRDSET *stcpu = NULL,
                  *stcpu_thread = NULL,
                  *stclients = NULL,
                  *streqs = NULL,
                  *stbytes = NULL,
                  *stduration = NULL,
                  *stcompression = NULL;

    struct global_statistics gs;
    struct rusage me, thread;

    global_statistics_copy(&gs, GLOBAL_STATS_RESET_WEB_USEC_MAX);
    getrusage(RUSAGE_THREAD, &thread);
    getrusage(RUSAGE_SELF, &me);

#ifdef __FreeBSD__
    if (unlikely(!stcpu_thread)) {
        stcpu_thread = rrdset_create_localhost(
                "netdata"
                , "plugin_freebsd_cpu"
                , NULL
                , "freebsd"
                , NULL
                , "NetData FreeBSD Plugin CPU usage"
                , "milliseconds/s"
                , "netdata"
                , "stats"
                , 132000
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
        );
#else
    if (unlikely(!stcpu_thread)) {
        stcpu_thread = rrdset_create_localhost(
                "netdata"
                , "plugin_proc_cpu"
                , NULL
                , "proc"
                , NULL
                , "NetData Proc Plugin CPU usage"
                , "milliseconds/s"
                , "netdata"
                , "stats"
                , 132000
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
        );
#endif

        rrddim_add(stcpu_thread, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(stcpu_thread, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    }
    else
        rrdset_next(stcpu_thread);

    rrddim_set(stcpu_thread, "user", thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
    rrddim_set(stcpu_thread, "system", thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
    rrdset_done(stcpu_thread);

    // ----------------------------------------------------------------

    if (unlikely(!stcpu)) {
        stcpu = rrdset_create_localhost(
                "netdata"
                , "server_cpu"
                , NULL
                , "netdata"
                , NULL
                , "NetData CPU usage"
                , "milliseconds/s"
                , "netdata"
                , "stats"
                , 130000
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
        );

        rrddim_add(stcpu, "user", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(stcpu, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
    }
    else
        rrdset_next(stcpu);

    rrddim_set(stcpu, "user", me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec);
    rrddim_set(stcpu, "system", me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec);
    rrdset_done(stcpu);

    // ----------------------------------------------------------------

    if (unlikely(!stclients)) {
        stclients = rrdset_create_localhost(
                "netdata"
                , "clients"
                , NULL
                , "netdata"
                , NULL
                , "NetData Web Clients"
                , "connected clients"
                , "netdata"
                , "stats"
                , 130200
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
        );

        rrddim_add(stclients, "clients", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(stclients);

    rrddim_set(stclients, "clients", gs.connected_clients);
    rrdset_done(stclients);

    // ----------------------------------------------------------------

    if (unlikely(!streqs)) {
        streqs = rrdset_create_localhost(
                "netdata"
                , "requests"
                , NULL
                , "netdata"
                , NULL
                , "NetData Web Requests"
                , "requests/s"
                , "netdata"
                , "stats"
                , 130300
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
        );

        rrddim_add(streqs, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    else
        rrdset_next(streqs);

    rrddim_set(streqs, "requests", (collected_number) gs.web_requests);
    rrdset_done(streqs);

    // ----------------------------------------------------------------

    if (unlikely(!stbytes)) {
        stbytes = rrdset_create_localhost(
                "netdata"
                , "net"
                , NULL
                , "netdata"
                , NULL
                , "NetData Network Traffic"
                , "kilobits/s"
                , "netdata"
                , "stats"
                , 130000
                , localhost->rrd_update_every
                , RRDSET_TYPE_AREA
        );

        rrddim_add(stbytes, "in",  NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        rrddim_add(stbytes, "out", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
    }
    else
        rrdset_next(stbytes);

    rrddim_set(stbytes, "in", (collected_number) gs.bytes_received);
    rrddim_set(stbytes, "out", (collected_number) gs.bytes_sent);
    rrdset_done(stbytes);

    // ----------------------------------------------------------------

    if (unlikely(!stduration)) {
        stduration = rrdset_create_localhost(
                "netdata"
                , "response_time"
                , NULL
                , "netdata"
                , NULL
                , "NetData API Response Time"
                , "ms/request"
                , "netdata"
                , "stats"
                , 130400
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
        );

        rrddim_add(stduration, "average", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(stduration, "max", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(stduration);

    uint64_t gweb_usec = gs.web_usec;
    uint64_t gweb_requests = gs.web_requests;

    uint64_t web_usec = (gweb_usec >= old_web_usec) ? gweb_usec - old_web_usec : 0;
    uint64_t web_requests = (gweb_requests >= old_web_requests) ? gweb_requests - old_web_requests : 0;

    old_web_usec = gweb_usec;
    old_web_requests = gweb_requests;

    if (web_requests)
        average_response_time = (collected_number) (web_usec / web_requests);

    if (unlikely(average_response_time != -1))
        rrddim_set(stduration, "average", average_response_time);
    else
        rrddim_set(stduration, "average", 0);

    rrddim_set(stduration, "max", ((gs.web_usec_max)?(collected_number)gs.web_usec_max:average_response_time));
    rrdset_done(stduration);

    // ----------------------------------------------------------------

    if (unlikely(!stcompression)) {
        stcompression = rrdset_create_localhost(
                "netdata"
                , "compression_ratio"
                , NULL
                , "netdata"
                , NULL
                , "NetData API Responses Compression Savings Ratio"
                , "percentage"
                , "netdata"
                , "stats"
                , 130500
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
        );

        rrddim_add(stcompression, "savings", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }
    else
        rrdset_next(stcompression);

    // since we don't lock here to read the global statistics
    // read the smaller value first
    unsigned long long gcompressed_content_size = gs.compressed_content_size;
    unsigned long long gcontent_size = gs.content_size;

    unsigned long long compressed_content_size = gcompressed_content_size - old_compressed_content_size;
    unsigned long long content_size = gcontent_size - old_content_size;

    old_compressed_content_size = gcompressed_content_size;
    old_content_size = gcontent_size;

    if (content_size && content_size >= compressed_content_size)
        compression_ratio = ((content_size - compressed_content_size) * 100 * 1000) / content_size;

    if (compression_ratio != -1)
        rrddim_set(stcompression, "savings", compression_ratio);

    rrdset_done(stcompression);
}
