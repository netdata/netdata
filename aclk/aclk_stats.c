// SPDX-License-Identifier: GPL-3.0-or-later

#define MQTT_WSS_CPUSTATS

#include "aclk_stats.h"

#include "aclk_query.h"

netdata_mutex_t aclk_stats_mutex = NETDATA_MUTEX_INITIALIZER;

struct {
    int query_thread_count;
    unsigned int proto_hdl_cnt;
    uint32_t *aclk_proto_rx_msgs_sample;
    RRDDIM **rx_msg_dims;
} aclk_stats_cfg; // there is only 1 stats thread at a time

// data ACLK stats need per query thread
struct aclk_qt_data {
    RRDDIM *dim;
} *aclk_qt_data = NULL;

uint32_t *aclk_queries_per_thread = NULL;
uint32_t *aclk_queries_per_thread_sample = NULL;
uint32_t *aclk_proto_rx_msgs_sample = NULL;

struct aclk_metrics aclk_metrics = {
    .online = 0,
};

struct aclk_metrics_per_sample aclk_metrics_per_sample;

static void aclk_stats_collect(struct aclk_metrics_per_sample *per_sample, struct aclk_metrics *permanent)
{
    static RRDSET *st_aclkstats = NULL;
    static RRDDIM *rd_online_status = NULL;

    if (unlikely(!st_aclkstats)) {
        st_aclkstats = rrdset_create_localhost(
            "netdata", "aclk_status", NULL, "aclk", NULL, "ACLK/Cloud connection status",
            "connected", "netdata", "stats", 200000, localhost->rrd_update_every, RRDSET_TYPE_LINE);

        rd_online_status = rrddim_add(st_aclkstats, "online", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_aclkstats, rd_online_status, per_sample->offline_during_sample ? 0 : permanent->online);

    rrdset_done(st_aclkstats);
}

static void aclk_stats_query_queue(struct aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st_query_thread = NULL;
    static RRDDIM *rd_queued = NULL;
    static RRDDIM *rd_dispatched = NULL;

    if (unlikely(!st_query_thread)) {
        st_query_thread = rrdset_create_localhost(
            "netdata", "aclk_query_per_second", NULL, "aclk", NULL, "ACLK Queries per second", "queries/s",
            "netdata", "stats", 200001, localhost->rrd_update_every, RRDSET_TYPE_AREA);

        rd_queued = rrddim_add(st_query_thread, "added", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_dispatched = rrddim_add(st_query_thread, "dispatched", NULL, -1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_query_thread, rd_queued, per_sample->queries_queued);
    rrddim_set_by_pointer(st_query_thread, rd_dispatched, per_sample->queries_dispatched);

    rrdset_done(st_query_thread);
}

#ifdef NETDATA_INTERNAL_CHECKS
static void aclk_stats_latency(struct aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_avg = NULL;
    static RRDDIM *rd_max = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_latency_mqtt", NULL, "aclk", NULL, "ACLK Message Publish Latency", "ms",
            "netdata", "stats", 200002, localhost->rrd_update_every, RRDSET_TYPE_LINE);

        rd_avg = rrddim_add(st, "avg", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_max = rrddim_add(st, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    if(per_sample->latency_count)
        rrddim_set_by_pointer(st, rd_avg, roundf((float)per_sample->latency_total / per_sample->latency_count));
    else
        rrddim_set_by_pointer(st, rd_avg, 0);

    rrddim_set_by_pointer(st, rd_max, per_sample->latency_max);

    rrdset_done(st);
}
#endif

static void aclk_stats_cloud_req(struct aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_rq_rcvd = NULL;
    static RRDDIM *rd_rq_err = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_cloud_req", NULL, "aclk", NULL, "Requests received from cloud", "req/s",
            "netdata", "stats", 200005, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        rd_rq_rcvd = rrddim_add(st, "received", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_rq_err = rrddim_add(st, "malformed", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st, rd_rq_rcvd, per_sample->cloud_req_recvd - per_sample->cloud_req_err);
    rrddim_set_by_pointer(st, rd_rq_err, per_sample->cloud_req_err);

    rrdset_done(st);
}

static void aclk_stats_cloud_req_type(struct aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *dims[ACLK_QUERY_TYPE_COUNT];

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_processed_query_type", NULL, "aclk", NULL, "Query thread commands processed by their type", "cmd/s",
            "netdata", "stats", 200006, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        for (int i = 0; i < ACLK_QUERY_TYPE_COUNT; i++)
            dims[i] = rrddim_add(st, aclk_query_get_name(i, 1), NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);

    }

    for (int i = 0; i < ACLK_QUERY_TYPE_COUNT; i++)
        rrddim_set_by_pointer(st, dims[i], per_sample->queries_per_type[i]);

    rrdset_done(st);
}

static char *cloud_req_http_type_names[ACLK_STATS_CLOUD_HTTP_REQ_TYPE_CNT] = {
    "other",
    "info",
    "data",
    "alarms",
    "alarm_log",
    "chart",
    "charts",
    "function",
    "functions"
    // if you change then update `ACLK_STATS_CLOUD_HTTP_REQ_TYPE_CNT`.
};

int aclk_cloud_req_http_type_to_idx(const char *name)
{
    for (int i = 1; i < ACLK_STATS_CLOUD_HTTP_REQ_TYPE_CNT; i++)
        if (!strcmp(cloud_req_http_type_names[i], name))
            return i;
    return 0;
}

static void aclk_stats_cloud_req_http_type(struct aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_rq_types[ACLK_STATS_CLOUD_HTTP_REQ_TYPE_CNT];

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_cloud_req_http_type", NULL, "aclk", NULL, "Requests received from cloud via HTTP by their type", "req/s",
            "netdata", "stats", 200007, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        for (int i = 0; i < ACLK_STATS_CLOUD_HTTP_REQ_TYPE_CNT; i++)
            rd_rq_types[i] = rrddim_add(st, cloud_req_http_type_names[i], NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    }

    for (int i = 0; i < ACLK_STATS_CLOUD_HTTP_REQ_TYPE_CNT; i++)
        rrddim_set_by_pointer(st, rd_rq_types[i], per_sample->cloud_req_http_by_type[i]);

    rrdset_done(st);
}

#define MAX_DIM_NAME 22
static void aclk_stats_query_threads(uint32_t *queries_per_thread)
{
    static RRDSET *st = NULL;

    char dim_name[MAX_DIM_NAME];

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_query_threads", NULL, "aclk", NULL, "Queries Processed Per Thread", "req/s",
            "netdata", "stats", 200009, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        for (int i = 0; i < aclk_stats_cfg.query_thread_count; i++) {
            if (snprintfz(dim_name, MAX_DIM_NAME, "Query %d", i) < 0)
                error("snprintf encoding error");
            aclk_qt_data[i].dim = rrddim_add(st, dim_name, NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    for (int i = 0; i < aclk_stats_cfg.query_thread_count; i++) {
        rrddim_set_by_pointer(st, aclk_qt_data[i].dim, queries_per_thread[i]);
    }

    rrdset_done(st);
}

static void aclk_stats_query_time(struct aclk_metrics_per_sample *per_sample)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_rq_avg = NULL;
    static RRDDIM *rd_rq_max = NULL;
    static RRDDIM *rd_rq_total = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_query_time", NULL, "aclk", NULL, "Time it took to process cloud requested DB queries", "us",
            "netdata", "stats", 200008, localhost->rrd_update_every, RRDSET_TYPE_LINE);

        rd_rq_avg = rrddim_add(st, "avg", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_rq_max = rrddim_add(st, "max", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        rd_rq_total = rrddim_add(st, "total", NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
    }

    if(per_sample->cloud_q_process_count)
        rrddim_set_by_pointer(st, rd_rq_avg, roundf((float)per_sample->cloud_q_process_total / per_sample->cloud_q_process_count));
    else
        rrddim_set_by_pointer(st, rd_rq_avg, 0);
    rrddim_set_by_pointer(st, rd_rq_max, per_sample->cloud_q_process_max);
    rrddim_set_by_pointer(st, rd_rq_total, per_sample->cloud_q_process_total);

    rrdset_done(st);
}

const char *rx_handler_get_name(size_t i);
static void aclk_stats_newproto_rx(uint32_t *rx_msgs_sample)
{
    static RRDSET *st = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_protobuf_rx_types", NULL, "aclk", NULL, "Received new cloud architecture messages by their type.", "msg/s",
            "netdata", "stats", 200010, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        for (unsigned int i = 0; i < aclk_stats_cfg.proto_hdl_cnt; i++) {
            aclk_stats_cfg.rx_msg_dims[i] = rrddim_add(st, rx_handler_get_name(i), NULL, 1, localhost->rrd_update_every, RRD_ALGORITHM_ABSOLUTE);
        }
    }

    for (unsigned int i = 0; i < aclk_stats_cfg.proto_hdl_cnt; i++)
        rrddim_set_by_pointer(st, aclk_stats_cfg.rx_msg_dims[i], rx_msgs_sample[i]);

    rrdset_done(st);
}

static void aclk_stats_mqtt_wss(struct mqtt_wss_stats *stats)
{
    static RRDSET *st = NULL;
    static RRDDIM *rd_sent = NULL;
    static RRDDIM *rd_recvd = NULL;
    static uint64_t sent = 0;
    static uint64_t recvd = 0;

    static RRDSET *st_txbuf_perc = NULL;
    static RRDDIM *rd_txbuf_perc = NULL;

    static RRDSET *st_txbuf = NULL;
    static RRDDIM *rd_tx_buffer_usable = NULL;
    static RRDDIM *rd_tx_buffer_reclaimable = NULL;
    static RRDDIM *rd_tx_buffer_used = NULL;
    static RRDDIM *rd_tx_buffer_free = NULL;
    static RRDDIM *rd_tx_buffer_size = NULL;

    static RRDSET *st_timing = NULL;
    static RRDDIM *rd_keepalive = NULL;
    static RRDDIM *rd_read_socket = NULL;
    static RRDDIM *rd_write_socket = NULL;
    static RRDDIM *rd_process_websocket = NULL;
    static RRDDIM *rd_process_mqtt = NULL;

    sent += stats->bytes_tx;
    recvd += stats->bytes_rx;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "netdata", "aclk_openssl_bytes", NULL, "aclk", NULL, "Received and Sent bytes.", "B/s",
            "netdata", "stats", 200011, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        rd_sent  = rrddim_add(st, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_recvd = rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    if (unlikely(!st_txbuf_perc)) {
        st_txbuf_perc = rrdset_create_localhost(
            "netdata", "aclk_mqtt_tx_perc", NULL, "aclk", NULL, "Actively used percentage of MQTT Tx Buffer,", "%",
            "netdata", "stats", 200012, localhost->rrd_update_every, RRDSET_TYPE_LINE);
        
        rd_txbuf_perc = rrddim_add(st_txbuf_perc, "used", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);
    }

    if (unlikely(!st_txbuf)) {
        st_txbuf = rrdset_create_localhost(
            "netdata", "aclk_mqtt_tx_queue", NULL, "aclk", NULL, "State of transmit MQTT queue.", "B",
            "netdata", "stats", 200013, localhost->rrd_update_every, RRDSET_TYPE_LINE);

        rd_tx_buffer_usable = rrddim_add(st_txbuf, "usable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_tx_buffer_reclaimable = rrddim_add(st_txbuf, "reclaimable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_tx_buffer_used = rrddim_add(st_txbuf, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_tx_buffer_free = rrddim_add(st_txbuf, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_tx_buffer_size = rrddim_add(st_txbuf, "size", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    if (unlikely(!st_timing)) {
        st_timing = rrdset_create_localhost(
            "netdata", "aclk_mqtt_wss_time", NULL, "aclk", NULL, "Time spent handling MQTT, WSS, SSL and network communication.", "us",
            "netdata", "stats", 200014, localhost->rrd_update_every, RRDSET_TYPE_STACKED);

        rd_keepalive = rrddim_add(st_timing, "keep-alive", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_read_socket = rrddim_add(st_timing, "socket_read_ssl", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_write_socket = rrddim_add(st_timing, "socket_write_ssl", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_process_websocket = rrddim_add(st_timing, "process_websocket", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_process_mqtt = rrddim_add(st_timing, "process_mqtt", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st, rd_sent, sent);
    rrddim_set_by_pointer(st, rd_recvd, recvd);

    float usage = ((float)stats->mqtt.tx_buffer_free + stats->mqtt.tx_buffer_reclaimable) / stats->mqtt.tx_buffer_size;
    usage = (1 - usage) * 10000;
    rrddim_set_by_pointer(st_txbuf_perc, rd_txbuf_perc, usage);

    rrddim_set_by_pointer(st_txbuf, rd_tx_buffer_usable, stats->mqtt.tx_buffer_reclaimable + stats->mqtt.tx_buffer_free);
    rrddim_set_by_pointer(st_txbuf, rd_tx_buffer_reclaimable, stats->mqtt.tx_buffer_reclaimable);
    rrddim_set_by_pointer(st_txbuf, rd_tx_buffer_used, stats->mqtt.tx_buffer_used);
    rrddim_set_by_pointer(st_txbuf, rd_tx_buffer_free, stats->mqtt.tx_buffer_free);
    rrddim_set_by_pointer(st_txbuf, rd_tx_buffer_size, stats->mqtt.tx_buffer_size);

    rrddim_set_by_pointer(st_timing, rd_keepalive, stats->time_keepalive);
    rrddim_set_by_pointer(st_timing, rd_read_socket, stats->time_read_socket);
    rrddim_set_by_pointer(st_timing, rd_write_socket, stats->time_write_socket);
    rrddim_set_by_pointer(st_timing, rd_process_websocket, stats->time_process_websocket);
    rrddim_set_by_pointer(st_timing, rd_process_mqtt, stats->time_process_mqtt);

    rrdset_done(st);
    rrdset_done(st_txbuf_perc);
    rrdset_done(st_txbuf);
    rrdset_done(st_timing);
}

void aclk_stats_thread_prepare(int query_thread_count, unsigned int proto_hdl_cnt)
{
    aclk_qt_data = callocz(query_thread_count, sizeof(struct aclk_qt_data));
    aclk_queries_per_thread = callocz(query_thread_count, sizeof(uint32_t));
    aclk_queries_per_thread_sample = callocz(query_thread_count, sizeof(uint32_t));

    memset(&aclk_metrics_per_sample, 0, sizeof(struct aclk_metrics_per_sample));

    aclk_stats_cfg.proto_hdl_cnt = proto_hdl_cnt;
    aclk_stats_cfg.aclk_proto_rx_msgs_sample = callocz(proto_hdl_cnt, sizeof(*aclk_proto_rx_msgs_sample));
    aclk_proto_rx_msgs_sample = callocz(proto_hdl_cnt, sizeof(*aclk_proto_rx_msgs_sample));
    aclk_stats_cfg.rx_msg_dims = callocz(proto_hdl_cnt, sizeof(RRDDIM*));
}

void aclk_stats_thread_cleanup()
{
    freez(aclk_stats_cfg.rx_msg_dims);
    freez(aclk_proto_rx_msgs_sample);
    freez(aclk_stats_cfg.aclk_proto_rx_msgs_sample);
    freez(aclk_qt_data);
    freez(aclk_queries_per_thread);
    freez(aclk_queries_per_thread_sample);
}

void *aclk_stats_main_thread(void *ptr)
{
    struct aclk_stats_thread *args = ptr;

    aclk_stats_cfg.query_thread_count = args->query_thread_count;

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step_ut = localhost->rrd_update_every * USEC_PER_SEC;

    struct aclk_metrics_per_sample per_sample;
    struct aclk_metrics permanent;

    while (service_running(SERVICE_ACLK | SERVICE_COLLECTORS)) {
        netdata_thread_testcancel();
        // ------------------------------------------------------------------------
        // Wait for the next iteration point.

        heartbeat_next(&hb, step_ut);
        if (!service_running(SERVICE_ACLK | SERVICE_COLLECTORS)) break;

        ACLK_STATS_LOCK;
        // to not hold lock longer than necessary, especially not to hold it
        // during database rrd* operations
        memcpy(&per_sample, &aclk_metrics_per_sample, sizeof(struct aclk_metrics_per_sample));

        memcpy(aclk_stats_cfg.aclk_proto_rx_msgs_sample, aclk_proto_rx_msgs_sample, sizeof(*aclk_proto_rx_msgs_sample) * aclk_stats_cfg.proto_hdl_cnt);
        memset(aclk_proto_rx_msgs_sample, 0, sizeof(*aclk_proto_rx_msgs_sample) * aclk_stats_cfg.proto_hdl_cnt);

        memcpy(&permanent, &aclk_metrics, sizeof(struct aclk_metrics));
        memset(&aclk_metrics_per_sample, 0, sizeof(struct aclk_metrics_per_sample));

        memcpy(aclk_queries_per_thread_sample, aclk_queries_per_thread, sizeof(uint32_t) * aclk_stats_cfg.query_thread_count);
        memset(aclk_queries_per_thread, 0, sizeof(uint32_t) * aclk_stats_cfg.query_thread_count);
        ACLK_STATS_UNLOCK;

        aclk_stats_collect(&per_sample, &permanent);
        aclk_stats_query_queue(&per_sample);
#ifdef NETDATA_INTERNAL_CHECKS
        aclk_stats_latency(&per_sample);
#endif

        aclk_stats_cloud_req(&per_sample);
        aclk_stats_cloud_req_type(&per_sample);
        aclk_stats_cloud_req_http_type(&per_sample);

        aclk_stats_query_threads(aclk_queries_per_thread_sample);

        aclk_stats_query_time(&per_sample);

        struct mqtt_wss_stats mqtt_wss_stats = mqtt_wss_get_stats(args->client);
        aclk_stats_mqtt_wss(&mqtt_wss_stats);

        aclk_stats_newproto_rx(aclk_stats_cfg.aclk_proto_rx_msgs_sample);
    }

    return 0;
}

void aclk_stats_upd_online(int online) {
    if(!aclk_stats_enabled)
        return;

    ACLK_STATS_LOCK;
    aclk_metrics.online = online;

    if(!online)
        aclk_metrics_per_sample.offline_during_sample = 1;
    ACLK_STATS_UNLOCK;
}

#ifdef NETDATA_INTERNAL_CHECKS
static usec_t pub_time[UINT16_MAX + 1] = {0};
void aclk_stats_msg_published(uint16_t id)
{
    ACLK_STATS_LOCK;
    pub_time[id] = now_boottime_usec();
    ACLK_STATS_UNLOCK;
}

void aclk_stats_msg_puback(uint16_t id)
{
    ACLK_STATS_LOCK;
    usec_t t;

    if (!aclk_stats_enabled) {
        ACLK_STATS_UNLOCK;
        return;
    }

    if (unlikely(!pub_time[id])) {
        ACLK_STATS_UNLOCK;
        error("Received PUBACK for unknown message?!");
        return;
    }

    t = now_boottime_usec() - pub_time[id];
    t /= USEC_PER_MS;
    pub_time[id] = 0;
    if (aclk_metrics_per_sample.latency_max < t)
        aclk_metrics_per_sample.latency_max = t;

    aclk_metrics_per_sample.latency_total += t;
    aclk_metrics_per_sample.latency_count++;
    ACLK_STATS_UNLOCK;
}
#endif /* NETDATA_INTERNAL_CHECKS */
