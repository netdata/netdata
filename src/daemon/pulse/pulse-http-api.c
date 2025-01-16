// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-http-api.h"

#define GLOBAL_STATS_RESET_WEB_USEC_MAX 0x01

static struct web_statistics {
    bool extended;

    PAD64(int32_t) connected_clients;

    PAD64(uint64_t) web_requests;
    PAD64(uint64_t) web_usec;
    PAD64(uint64_t) web_usec_max;

    PAD64(uint64_t) content_size_uncompressed;
    PAD64(uint64_t) content_size_compressed;
} live_stats = { 0 };

void pulse_web_client_connected(void) {
    __atomic_fetch_add(&live_stats.connected_clients, 1, __ATOMIC_RELAXED);
}

void pulse_web_client_disconnected(void) {
    __atomic_fetch_sub(&live_stats.connected_clients, 1, __ATOMIC_RELAXED);
}

void pulse_web_request_completed(uint64_t dt,
                                             uint64_t bytes_received __maybe_unused,
                                             uint64_t bytes_sent __maybe_unused,
                                             uint64_t content_size,
                                             uint64_t compressed_content_size) {
    uint64_t old_web_usec_max = live_stats.web_usec_max;
    while(dt > old_web_usec_max)
        __atomic_compare_exchange(&live_stats.web_usec_max, &old_web_usec_max, &dt, 1, __ATOMIC_RELAXED, __ATOMIC_RELAXED);

    __atomic_fetch_add(&live_stats.web_requests, 1, __ATOMIC_RELAXED);
    __atomic_fetch_add(&live_stats.web_usec, dt, __ATOMIC_RELAXED);
//    __atomic_fetch_add(&live_stats.bytes_received, bytes_received, __ATOMIC_RELAXED);
//    __atomic_fetch_add(&live_stats.bytes_sent, bytes_sent, __ATOMIC_RELAXED);
    __atomic_fetch_add(&live_stats.content_size_uncompressed, content_size, __ATOMIC_RELAXED);
    __atomic_fetch_add(&live_stats.content_size_compressed, compressed_content_size, __ATOMIC_RELAXED);
}

static inline void pulse_web_copy(struct web_statistics *gs, uint8_t options) {
    gs->connected_clients = __atomic_load_n(&live_stats.connected_clients, __ATOMIC_RELAXED);
    gs->web_requests = __atomic_load_n(&live_stats.web_requests, __ATOMIC_RELAXED);
    gs->web_usec = __atomic_load_n(&live_stats.web_usec, __ATOMIC_RELAXED);
    gs->web_usec_max = __atomic_load_n(&live_stats.web_usec_max, __ATOMIC_RELAXED);
//    gs->bytes_received = __atomic_load_n(&live_stats.bytes_received, __ATOMIC_RELAXED);
//    gs->bytes_sent = __atomic_load_n(&live_stats.bytes_sent, __ATOMIC_RELAXED);
    gs->content_size_uncompressed = __atomic_load_n(&live_stats.content_size_uncompressed, __ATOMIC_RELAXED);
    gs->content_size_compressed = __atomic_load_n(&live_stats.content_size_compressed, __ATOMIC_RELAXED);

    if(options & GLOBAL_STATS_RESET_WEB_USEC_MAX) {
        uint64_t n = 0;
        __atomic_compare_exchange(&live_stats.web_usec_max, (uint64_t *) &gs->web_usec_max, &n, 1, __ATOMIC_RELAXED, __ATOMIC_RELAXED);
    }
}

void pulse_web_do(bool extended) {
    static struct web_statistics gs;
    pulse_web_copy(&gs, GLOBAL_STATS_RESET_WEB_USEC_MAX);

    // ----------------------------------------------------------------

    {
        static RRDSET *st_clients = NULL;
        static RRDDIM *rd_clients = NULL;

        if (unlikely(!st_clients)) {
            st_clients = rrdset_create_localhost(
                "netdata"
                , "clients"
                , NULL
                , "HTTP API"
                , "netdata.http_api_clients"
                , "Netdata Web API Clients"
                , "connected clients"
                , "netdata"
                , "pulse"
                , 130200
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_clients = rrddim_add(st_clients, "clients", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_clients, rd_clients, gs.connected_clients);
        rrdset_done(st_clients);
    }

    // ----------------------------------------------------------------

    {
        static RRDSET *st_reqs = NULL;
        static RRDDIM *rd_requests = NULL;

        if (unlikely(!st_reqs)) {
            st_reqs = rrdset_create_localhost(
                "netdata"
                , "requests"
                , NULL
                , "HTTP API"
                , "netdata.http_api_requests"
                , "Netdata Web API Requests Received"
                , "requests/s"
                , "netdata"
                , "pulse"
                , 130300
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_requests = rrddim_add(st_reqs, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_reqs, rd_requests, (collected_number) gs.web_requests);
        rrdset_done(st_reqs);
    }

    // ----------------------------------------------------------------

    {
        static unsigned long long old_web_requests = 0, old_web_usec = 0;
        static collected_number average_response_time = -1;

        static RRDSET *st_duration = NULL;
        static RRDDIM *rd_average = NULL,
                      *rd_max     = NULL;

        if (unlikely(!st_duration)) {
            st_duration = rrdset_create_localhost(
                "netdata"
                , "response_time"
                , NULL
                , "HTTP API"
                , "netdata.http_api_response_time"
                , "Netdata Web API Response Time"
                , "milliseconds/request"
                , "netdata"
                , "pulse"
                , 130500
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_average = rrddim_add(st_duration, "average", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            rd_max = rrddim_add(st_duration, "max", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        uint64_t gweb_usec = gs.web_usec;
        uint64_t gweb_requests = gs.web_requests;

        uint64_t web_usec = (gweb_usec >= old_web_usec) ? gweb_usec - old_web_usec : 0;
        uint64_t web_requests = (gweb_requests >= old_web_requests) ? gweb_requests - old_web_requests : 0;

        old_web_usec = gweb_usec;
        old_web_requests = gweb_requests;

        if (web_requests)
            average_response_time = (collected_number) (web_usec / web_requests);

        if (unlikely(average_response_time != -1))
            rrddim_set_by_pointer(st_duration, rd_average, average_response_time);
        else
            rrddim_set_by_pointer(st_duration, rd_average, 0);

        rrddim_set_by_pointer(st_duration, rd_max, ((gs.web_usec_max)?(collected_number)gs.web_usec_max:average_response_time));
        rrdset_done(st_duration);
    }

    // ----------------------------------------------------------------

    if(!extended) return;

    // ----------------------------------------------------------------

    {
        static unsigned long long old_content_size = 0, old_compressed_content_size = 0;
        static collected_number compression_ratio = -1;

        static RRDSET *st_compression = NULL;
        static RRDDIM *rd_savings = NULL;

        if (unlikely(!st_compression)) {
            st_compression = rrdset_create_localhost(
                "netdata"
                , "compression_ratio"
                , NULL
                , "HTTP API"
                , "netdata.http_api_compression_ratio"
                , "Netdata Web API Responses Compression Savings Ratio"
                , "percentage"
                , "netdata"
                , "pulse"
                , 130600
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            rd_savings = rrddim_add(st_compression, "savings", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        // since we don't lock here to read the data
        // read the smaller value first
        unsigned long long gcompressed_content_size = gs.content_size_compressed;
        unsigned long long gcontent_size = gs.content_size_uncompressed;

        unsigned long long compressed_content_size = gcompressed_content_size - old_compressed_content_size;
        unsigned long long content_size = gcontent_size - old_content_size;

        old_compressed_content_size = gcompressed_content_size;
        old_content_size = gcontent_size;

        if (content_size && content_size >= compressed_content_size)
            compression_ratio = ((content_size - compressed_content_size) * 100 * 1000) / content_size;

        if (compression_ratio != -1)
            rrddim_set_by_pointer(st_compression, rd_savings, compression_ratio);

        rrdset_done(st_compression);
    }
}