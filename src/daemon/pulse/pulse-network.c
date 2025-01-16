// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-network.h"

#define PULSE_NETWORK_CHART_TITLE "Netdata Network Traffic"
#define PULSE_NETWORK_CHART_FAMILY "Network Traffic"
#define PULSE_NETWORK_CHART_CONTEXT "netdata.network"
#define PULSE_NETWORK_CHART_UNITS "kilobits/s"
#define PULSE_NETWORK_CHART_PRIORITY 130150

static struct network_statistics {
    bool extended;
    PAD64(uint64_t) api_bytes_received;
    PAD64(uint64_t) api_bytes_sent;
    PAD64(uint64_t) statsd_bytes_received;
    PAD64(uint64_t) statsd_bytes_sent;
    PAD64(uint64_t) stream_bytes_received;
    PAD64(uint64_t) stream_bytes_sent;
} live_stats = { 0 };

void pulse_web_server_received_bytes(size_t bytes) {
    __atomic_add_fetch(&live_stats.api_bytes_received, bytes, __ATOMIC_RELAXED);
}

void pulse_web_server_sent_bytes(size_t bytes) {
    __atomic_add_fetch(&live_stats.api_bytes_sent, bytes, __ATOMIC_RELAXED);
}

void pulse_statsd_received_bytes(size_t bytes) {
    __atomic_add_fetch(&live_stats.statsd_bytes_received, bytes, __ATOMIC_RELAXED);
}

void pulse_statsd_sent_bytes(size_t bytes) {
    __atomic_add_fetch(&live_stats.statsd_bytes_sent, bytes, __ATOMIC_RELAXED);
}

void pulse_stream_received_bytes(size_t bytes) {
    __atomic_add_fetch(&live_stats.stream_bytes_received, bytes, __ATOMIC_RELAXED);
}

void pulse_stream_sent_bytes(size_t bytes) {
    __atomic_add_fetch(&live_stats.stream_bytes_sent, bytes, __ATOMIC_RELAXED);
}

static inline void pulse_network_copy(struct network_statistics *gs) {
    gs->api_bytes_received = __atomic_load_n(&live_stats.api_bytes_received, __ATOMIC_RELAXED);
    gs->api_bytes_sent = __atomic_load_n(&live_stats.api_bytes_sent, __ATOMIC_RELAXED);
    
    gs->statsd_bytes_received = __atomic_load_n(&live_stats.statsd_bytes_received, __ATOMIC_RELAXED);
    gs->statsd_bytes_sent = __atomic_load_n(&live_stats.statsd_bytes_sent, __ATOMIC_RELAXED);

    gs->stream_bytes_received = __atomic_load_n(&live_stats.stream_bytes_received, __ATOMIC_RELAXED);
    gs->stream_bytes_sent = __atomic_load_n(&live_stats.stream_bytes_sent, __ATOMIC_RELAXED);
}

void pulse_network_do(bool extended __maybe_unused) {
    static struct network_statistics gs;
    pulse_network_copy(&gs);

    if(gs.api_bytes_received || gs.api_bytes_sent) {
        static RRDSET *st_bytes = NULL;
        static RRDDIM *rd_in = NULL,
                      *rd_out = NULL;

        if (unlikely(!st_bytes)) {
            st_bytes = rrdset_create_localhost(
                "netdata"
                , "network_api"
                , NULL
                , PULSE_NETWORK_CHART_FAMILY
                , PULSE_NETWORK_CHART_CONTEXT
                , PULSE_NETWORK_CHART_TITLE
                , PULSE_NETWORK_CHART_UNITS
                , "netdata"
                , "pulse"
                , PULSE_NETWORK_CHART_PRIORITY
                , localhost->rrd_update_every
                , RRDSET_TYPE_AREA
            );

            rrdlabels_add(st_bytes->rrdlabels, "endpoint", "web-server", RRDLABEL_SRC_AUTO);
            
            rd_in  = rrddim_add(st_bytes, "in",  NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_bytes, "out", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_bytes, rd_in, (collected_number) gs.api_bytes_received);
        rrddim_set_by_pointer(st_bytes, rd_out, (collected_number) gs.api_bytes_sent);
        rrdset_done(st_bytes);
    }

    if(gs.statsd_bytes_received || gs.statsd_bytes_sent) {
        static RRDSET *st_bytes = NULL;
        static RRDDIM *rd_in = NULL,
                      *rd_out = NULL;

        if (unlikely(!st_bytes)) {
            st_bytes = rrdset_create_localhost(
                "netdata"
                , "network_statsd"
                , NULL
                , PULSE_NETWORK_CHART_FAMILY
                , PULSE_NETWORK_CHART_CONTEXT
                , PULSE_NETWORK_CHART_TITLE
                , PULSE_NETWORK_CHART_UNITS
                , "netdata"
                , "pulse"
                , PULSE_NETWORK_CHART_PRIORITY
                , localhost->rrd_update_every
                , RRDSET_TYPE_AREA
            );

            rrdlabels_add(st_bytes->rrdlabels, "endpoint", "statsd", RRDLABEL_SRC_AUTO);

            rd_in  = rrddim_add(st_bytes, "in",  NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_bytes, "out", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_bytes, rd_in, (collected_number) gs.statsd_bytes_received);
        rrddim_set_by_pointer(st_bytes, rd_out, (collected_number) gs.statsd_bytes_sent);
        rrdset_done(st_bytes);
    }

    if(gs.stream_bytes_received || gs.stream_bytes_sent) {
        static RRDSET *st_bytes = NULL;
        static RRDDIM *rd_in = NULL,
                      *rd_out = NULL;

        if (unlikely(!st_bytes)) {
            st_bytes = rrdset_create_localhost(
                "netdata"
                , "network_streaming"
                , NULL
                , PULSE_NETWORK_CHART_FAMILY
                , PULSE_NETWORK_CHART_CONTEXT
                , PULSE_NETWORK_CHART_TITLE
                , PULSE_NETWORK_CHART_UNITS
                , "netdata"
                , "pulse"
                , PULSE_NETWORK_CHART_PRIORITY
                , localhost->rrd_update_every
                , RRDSET_TYPE_AREA
            );

            rrdlabels_add(st_bytes->rrdlabels, "endpoint", "streaming", RRDLABEL_SRC_AUTO);

            rd_in  = rrddim_add(st_bytes, "in",  NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_bytes, "out", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_bytes, rd_in, (collected_number) gs.stream_bytes_received);
        rrddim_set_by_pointer(st_bytes, rd_out, (collected_number) gs.stream_bytes_sent);
        rrdset_done(st_bytes);
    }

    if(aclk_online()) {
        struct mqtt_wss_stats t = aclk_statistics();
        if (t.bytes_rx || t.bytes_tx) {
            static RRDSET *st_bytes = NULL;
            static RRDDIM *rd_in = NULL, *rd_out = NULL;

            if (unlikely(!st_bytes)) {
                st_bytes = rrdset_create_localhost(
                    "netdata",
                    "network_aclk",
                    NULL,
                    PULSE_NETWORK_CHART_FAMILY,
                    PULSE_NETWORK_CHART_CONTEXT,
                    PULSE_NETWORK_CHART_TITLE,
                    PULSE_NETWORK_CHART_UNITS,
                    "netdata",
                    "pulse",
                    PULSE_NETWORK_CHART_PRIORITY,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

                rrdlabels_add(st_bytes->rrdlabels, "endpoint", "aclk", RRDLABEL_SRC_AUTO);

                rd_in = rrddim_add(st_bytes, "in", NULL, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                rd_out = rrddim_add(st_bytes, "out", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(st_bytes, rd_in, (collected_number)t.bytes_rx);
            rrddim_set_by_pointer(st_bytes, rd_out, (collected_number)t.bytes_tx);
            rrdset_done(st_bytes);
        }
    }
}
