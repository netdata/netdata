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

// --------------------------------------------------------------------------------------------------------------------
// aclk time heatmap
// a similar history exists in dbengine cache.c

struct aclk_histogram_entry {
    usec_t upto;
    size_t count;
};

#define ACLK_TIME_HISTOGRAM_ENTRIES 19

static struct aclk_time_histogram {
    struct aclk_histogram_entry array[ACLK_TIME_HISTOGRAM_ENTRIES];
} aclk_time_heatmap;

void aclk_time_histogram_init(void) {
    struct aclk_time_histogram *h = &aclk_time_heatmap;

    // the histogram MUST be all-inclusive for the possible sizes,
    // so we start from 0, and the last value is UINT64_MAX.

    usec_t values[ACLK_TIME_HISTOGRAM_ENTRIES] = {
        // minimum
        0,

        // ms
        10 * USEC_PER_MS,
        50 * USEC_PER_MS, 100 * USEC_PER_MS, 200 * USEC_PER_MS, 350 * USEC_PER_MS,
        500 * USEC_PER_MS, 750 * USEC_PER_MS,

        // seconds
        1 * USEC_PER_SEC, 2 * USEC_PER_SEC, 4 * USEC_PER_SEC, 8 * USEC_PER_SEC,
        15 * USEC_PER_SEC, 30 * USEC_PER_SEC, 45 * USEC_PER_SEC,

        // minutes
        60 * USEC_PER_SEC, 120 * USEC_PER_SEC, 180 * USEC_PER_SEC,

        // maximum
        UINT64_MAX
    };

    usec_t last_value = 0;
    for(size_t i = 0; i < ACLK_TIME_HISTOGRAM_ENTRIES; i++) {
        if(i > 0 && values[i] == 0)
            fatal("only the first value in the array can be zero");

        if(i > 0 && values[i] <= last_value)
            fatal("the values need to be sorted");

        h->array[i].upto = values[i];
        last_value = values[i];
    }
}

static inline size_t aclk_time_histogram_slot(struct aclk_time_histogram *h, usec_t dt_ut) {
    if(dt_ut <= h->array[0].upto)
        return 0;

    if(dt_ut >= h->array[_countof(h->array) - 1].upto)
        return _countof(h->array) - 1;

    // binary search for the right size
    size_t low = 0, high = _countof(h->array) - 1;
    while (low < high) {
        size_t mid = low + (high - low) / 2;
        if (dt_ut < h->array[mid].upto)
            high = mid;
        else
            low = mid + 1;
    }
    return low - 1;
}

// Per-iteration PUBACK latency accumulators (microseconds)
static _Atomic uint64_t aclk_ack_count = 0;
static _Atomic uint64_t aclk_ack_sum_us = 0;
static _Atomic uint64_t aclk_ack_min_us = UINT64_MAX;
static _Atomic uint64_t aclk_ack_max_us = 0;

void pulse_aclk_sent_message_acked(usec_t publish_latency, size_t len __maybe_unused) {
    if(!publish_latency) return;

//    usec_t usec = now_monotonic_usec() - publish_latency;

    size_t slot = aclk_time_histogram_slot(&aclk_time_heatmap, publish_latency);
    internal_fatal(slot >= _countof(aclk_time_heatmap.array), "hey!");

    __atomic_add_fetch(&aclk_time_heatmap.array[slot].count, 1, __ATOMIC_RELAXED);

    // Track per-iteration min/avg/max in microseconds using atomics
    __atomic_add_fetch(&aclk_ack_count, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&aclk_ack_sum_us, (uint64_t)publish_latency, __ATOMIC_RELAXED);

    // atomic min update
    uint64_t cur_min = __atomic_load_n(&aclk_ack_min_us, __ATOMIC_RELAXED);
    while (publish_latency < cur_min &&
           !__atomic_compare_exchange_n(&aclk_ack_min_us, &cur_min, (uint64_t)publish_latency, true, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
        ;
    }
    // atomic max update
    uint64_t cur_max = __atomic_load_n(&aclk_ack_max_us, __ATOMIC_RELAXED);
    while (publish_latency > cur_max &&
           !__atomic_compare_exchange_n(&aclk_ack_max_us, &cur_max, (uint64_t)publish_latency, true, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
        ;
    }
}

static void pulse_aclk_time_heatmap(void) {
    static RRDSET *st;
    static RRDDIM *rds[ACLK_TIME_HISTOGRAM_ENTRIES];

    if(!st) {
        st = rrdset_create_localhost(
            "netdata",
            "aclk_puback_latency",
            NULL,
            PULSE_NETWORK_CHART_FAMILY,
            "netdata.aclk_puback_latency",
            "Netdata ACLK PubACK Latency In Seconds",
            "messages",
            "netdata",
            "pulse",
            PULSE_NETWORK_CHART_PRIORITY + 1,
            localhost->rrd_update_every,
            RRDSET_TYPE_HEATMAP);

        rds[0] = rrddim_add(st, "instant", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        for(size_t i = 1; i < _countof(rds) - 1 ;i++) {
            char buf[350];
            snprintf(buf, sizeof(buf), "%.2fs", (double)aclk_time_heatmap.array[i].upto / (double)USEC_PER_SEC);
            // duration_snprintf(buf, sizeof(buf), aclk_time_heatmap.array[i].upto, "us", false);
            rds[i] = rrddim_add(st, buf, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        rds[_countof(rds) - 1] = rrddim_add(st, "+inf", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    for(size_t i = 0; i < _countof(rds) - 1 ;i++) {
        size_t old_value = 0, new_value = 0;
        __atomic_exchange(&aclk_time_heatmap.array[i].count, &new_value, &old_value, __ATOMIC_RELAXED);
        rrddim_set_by_pointer(st, rds[i], (collected_number)old_value);
    }

    rrdset_done(st);
}

// --------------------------------------------------------------------------------------------------------------------

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

        pulse_aclk_time_heatmap();

        if(extended) {
            static RRDSET *st_aclk_queue_size = NULL;
            static RRDDIM *rd_messages = NULL;
            static RRDDIM *rd_puback_wait = NULL;

            if (unlikely(!st_aclk_queue_size)) {
                st_aclk_queue_size = rrdset_create_localhost(
                    "netdata",
                    "network_aclk_send_queue",
                    NULL,
                    PULSE_NETWORK_CHART_FAMILY,
                    "netdata.network_aclk_send_queue",
                    "Netdata ACLK Send Queue Size",
                    "messages",
                    "netdata",
                    "pulse",
                    PULSE_NETWORK_CHART_PRIORITY + 2,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

                rrdlabels_add(st_aclk_queue_size->rrdlabels, "endpoint", "aclk", RRDLABEL_SRC_AUTO);

                rd_messages = rrddim_add(st_aclk_queue_size, "messages", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_puback_wait = rrddim_add(st_aclk_queue_size, "puback wait", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_aclk_queue_size, rd_messages, (collected_number)t.mqtt.tx_messages_queued);
            rrddim_set_by_pointer(st_aclk_queue_size, rd_puback_wait, (collected_number)t.mqtt.packets_waiting_puback);
            rrdset_done(st_aclk_queue_size);
        }

        if(extended) {
            static RRDSET *st_aclk_messages = NULL;
            static RRDDIM *rd_in = NULL, *rd_out = NULL;

            if (unlikely(!st_aclk_messages)) {
                st_aclk_messages = rrdset_create_localhost(
                    "netdata",
                    "network_aclk_messages",
                    NULL,
                    PULSE_NETWORK_CHART_FAMILY,
                    "netdata.network_aclk_messages",
                    "Netdata ACLK Messages",
                    "messages/s",
                    "netdata",
                    "pulse",
                    PULSE_NETWORK_CHART_PRIORITY + 3,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_AREA);

                rrdlabels_add(st_aclk_messages->rrdlabels, "endpoint", "aclk", RRDLABEL_SRC_AUTO);

                rd_in = rrddim_add(st_aclk_messages, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                rd_out = rrddim_add(st_aclk_messages, "queued", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }

            rrddim_set_by_pointer(st_aclk_messages, rd_in, (collected_number)t.mqtt.rx_messages_rcvd);
            rrddim_set_by_pointer(st_aclk_messages, rd_out, (collected_number)t.mqtt.tx_messages_sent);
            rrdset_done(st_aclk_messages);
        }

        if(extended) {
            // Bytes queued for send
            static RRDSET *st_aclk_bytes = NULL;
            static RRDDIM *rd_bytes = NULL;
            if (unlikely(!st_aclk_bytes)) {
                st_aclk_bytes = rrdset_create_localhost(
                    "netdata",
                    "network_aclk_send_queue_bytes",
                    NULL,
                    PULSE_NETWORK_CHART_FAMILY,
                    "netdata.network_aclk_send_queue_bytes",
                    "Netdata ACLK Send Queue Bytes",
                    "bytes",
                    "netdata",
                    "pulse",
                    PULSE_NETWORK_CHART_PRIORITY + 2,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);
                rrdlabels_add(st_aclk_bytes->rrdlabels, "endpoint", "aclk", RRDLABEL_SRC_AUTO);
                rd_bytes = rrddim_add(st_aclk_bytes, "bytes", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            rrddim_set_by_pointer(st_aclk_bytes, rd_bytes, (collected_number)t.mqtt.tx_bytes_queued);
            rrdset_done(st_aclk_bytes);
        }

        if(extended) {
            // Max wait times for PUBACK and send queue (convert us->s via divisor)
            static RRDSET *st_aclk_puback_wait = NULL;
            static RRDDIM *rd_puback_max = NULL;
            if (unlikely(!st_aclk_puback_wait)) {
                st_aclk_puback_wait = rrdset_create_localhost(
                    "netdata",
                    "network_aclk_puback_wait",
                    NULL,
                    PULSE_NETWORK_CHART_FAMILY,
                    "netdata.network_aclk_puback_wait",
                    "Netdata ACLK PUBACK Max Wait",
                    "seconds",
                    "netdata",
                    "pulse",
                    PULSE_NETWORK_CHART_PRIORITY + 3,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);
                rrdlabels_add(st_aclk_puback_wait->rrdlabels, "endpoint", "aclk", RRDLABEL_SRC_AUTO);
                rd_puback_max = rrddim_add(st_aclk_puback_wait, "max", NULL, 1, USEC_PER_SEC, RRD_ALGORITHM_ABSOLUTE);
            }
            rrddim_set_by_pointer(st_aclk_puback_wait, rd_puback_max, (collected_number)t.mqtt.max_puback_wait_us);
            rrdset_done(st_aclk_puback_wait);
        }

        if(extended) {
            static RRDSET *st_aclk_send_wait = NULL;
            static RRDDIM *rd_overall = NULL, *rd_unsent = NULL, *rd_partial = NULL;
            if (unlikely(!st_aclk_send_wait)) {
                st_aclk_send_wait = rrdset_create_localhost(
                    "netdata",
                    "network_aclk_send_queue_wait",
                    NULL,
                    PULSE_NETWORK_CHART_FAMILY,
                    "netdata.network_aclk_send_queue_wait",
                    "Netdata ACLK Send Queue Wait (Overall/Unsent/Partial)",
                    "seconds",
                    "netdata",
                    "pulse",
                    PULSE_NETWORK_CHART_PRIORITY + 3,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);
                rrdlabels_add(st_aclk_send_wait->rrdlabels, "endpoint", "aclk", RRDLABEL_SRC_AUTO);
                rd_overall = rrddim_add(st_aclk_send_wait, "overall", NULL, 1, USEC_PER_SEC, RRD_ALGORITHM_ABSOLUTE);
                rd_unsent  = rrddim_add(st_aclk_send_wait, "unsent",  NULL, 1, USEC_PER_SEC, RRD_ALGORITHM_ABSOLUTE);
                rd_partial = rrddim_add(st_aclk_send_wait, "partial", NULL, 1, USEC_PER_SEC, RRD_ALGORITHM_ABSOLUTE);
            }
            rrddim_set_by_pointer(st_aclk_send_wait, rd_overall, (collected_number)t.mqtt.max_send_queue_wait_us);
            rrddim_set_by_pointer(st_aclk_send_wait, rd_unsent,  (collected_number)t.mqtt.max_unsent_wait_us);
            rrddim_set_by_pointer(st_aclk_send_wait, rd_partial, (collected_number)t.mqtt.max_partial_wait_us);
            rrdset_done(st_aclk_send_wait);
        }
    }

    // Publish PUBACK latency min/avg/max per iteration
    {
        static RRDSET *st_stats = NULL;
        static RRDDIM *rd_min = NULL, *rd_avg = NULL, *rd_max = NULL;

        // pull and reset accumulators
        uint64_t count = __atomic_exchange_n(&aclk_ack_count, 0, __ATOMIC_RELAXED);
        uint64_t sum_us = __atomic_exchange_n(&aclk_ack_sum_us, 0, __ATOMIC_RELAXED);
        uint64_t min_us = __atomic_exchange_n(&aclk_ack_min_us, UINT64_MAX, __ATOMIC_RELAXED);
        uint64_t max_us = __atomic_exchange_n(&aclk_ack_max_us, 0, __ATOMIC_RELAXED);

        uint64_t avg_us = (count ? (sum_us / count) : 0);
        if (min_us == UINT64_MAX) min_us = 0;

        if (unlikely(!st_stats)) {
            st_stats = rrdset_create_localhost(
                "netdata",
                "aclk_puback_latency_stats",
                NULL,
                PULSE_NETWORK_CHART_FAMILY,
                "netdata.aclk_puback_latency_stats",
                "Netdata ACLK PubACK Latency (Min/Avg/Max)",
                "milliseconds",
                "netdata",
                "pulse",
                PULSE_NETWORK_CHART_PRIORITY + 1,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);
            rd_min = rrddim_add(st_stats, "min", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);
            rd_avg = rrddim_add(st_stats, "avg", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);
            rd_max = rrddim_add(st_stats, "max", NULL, 1, USEC_PER_MS, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_stats, rd_min, (collected_number)min_us);
        rrddim_set_by_pointer(st_stats, rd_avg, (collected_number)avg_us);
        rrddim_set_by_pointer(st_stats, rd_max, (collected_number)max_us);
        rrdset_done(st_stats);
    }
}
