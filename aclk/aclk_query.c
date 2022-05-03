// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query.h"
#include "aclk_stats.h"
#include "aclk_tx_msgs.h"

#define ACLK_QUERY_THREAD_NAME "ACLK_Query"

#define WEB_HDR_ACCEPT_ENC "Accept-Encoding:"

pthread_cond_t query_cond_wait = PTHREAD_COND_INITIALIZER;
pthread_mutex_t query_lock_wait = PTHREAD_MUTEX_INITIALIZER;
#define QUERY_THREAD_LOCK pthread_mutex_lock(&query_lock_wait)
#define QUERY_THREAD_UNLOCK pthread_mutex_unlock(&query_lock_wait)

typedef struct aclk_query_handler {
    aclk_query_type_t type;
    char *name; // for logging purposes
    int(*fnc)(struct aclk_query_thread *query_thr, aclk_query_t query);
} aclk_query_handler;

static int info_metadata(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    aclk_send_info_metadata(query_thr->client,
        !query->data.metadata_info.initial_on_connect,
        query->data.metadata_info.host);
    return 0;
}

static int alarms_metadata(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    aclk_send_alarm_metadata(query_thr->client,
        !query->data.metadata_info.initial_on_connect);
    return 0;
}

static usec_t aclk_web_api_v1_request(RRDHOST *host, struct web_client *w, char *url)
{
    usec_t t;

    t = now_monotonic_high_precision_usec();
    w->response.code = web_client_api_request_v1(host, w, url);
    t = now_monotonic_high_precision_usec() - t;

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.cloud_q_process_total += t;
        aclk_metrics_per_sample.cloud_q_process_count++;
        if (aclk_metrics_per_sample.cloud_q_process_max < t)
            aclk_metrics_per_sample.cloud_q_process_max = t;
        ACLK_STATS_UNLOCK;
    }

    return t;
}

static RRDHOST *node_id_2_rrdhost(const char *node_id)
{
    int res;
    uuid_t node_id_bin, host_id_bin;

    rrd_rdlock();
    RRDHOST *host = find_host_by_node_id((char *) node_id);
    rrd_unlock();
    if (host)
        return host;

    char host_id[UUID_STR_LEN];
    if (uuid_parse(node_id, node_id_bin)) {
        error("Couldn't parse UUID %s", node_id);
        return NULL;
    }
    if ((res = get_host_id(&node_id_bin, &host_id_bin))) {
        error("node not found rc=%d", res);
        return NULL;
    }
    uuid_unparse_lower(host_id_bin, host_id);
    return rrdhost_find_by_guid(host_id, 0);
}

#define NODE_ID_QUERY "/node/"
// TODO this function should be quarantied and written nicely
// lots of skeletons from initial ACLK Legacy impl.
// quick and dirty from the start
static int http_api_v2(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    int retval = 0;
    usec_t t;
    BUFFER *local_buffer = NULL;
    BUFFER *log_buffer = buffer_create(NETDATA_WEB_REQUEST_URL_SIZE);
    RRDHOST *query_host = localhost;

#ifdef NETDATA_WITH_ZLIB
    int z_ret;
    BUFFER *z_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    char *start, *end;
#endif

    struct web_client *w = (struct web_client *)callocz(1, sizeof(struct web_client));
    w->response.data = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    w->response.header = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    w->response.header_output = buffer_create(NETDATA_WEB_RESPONSE_HEADER_SIZE);
    strcpy(w->origin, "*"); // Simulate web_client_create_on_fd()
    w->cookie1[0] = 0;      // Simulate web_client_create_on_fd()
    w->cookie2[0] = 0;      // Simulate web_client_create_on_fd()
    w->acl = 0x1f;

    buffer_strcat(log_buffer, query->data.http_api_v2.query);
    size_t size = 0;
    size_t sent = 0;
    w->tv_in = query->created_tv;
    now_realtime_timeval(&w->tv_ready);

    if (query->timeout && (dt_usec(&query->created_tv, &w->tv_ready) / 1000.0) > query->timeout) {
        log_access("QUERY CANCELED: QUEUE TIME EXCEEDED %0.2f ms (LIMIT %d ms)",
                   dt_usec(&query->created_tv, &w->tv_ready) / 1000.0, query->timeout);
        retval = 1;
        w->response.code = HTTP_RESP_BACKEND_FETCH_FAILED;
        aclk_http_msg_v2_err(query_thr->client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_SND_TIMEOUT, CLOUD_EMSG_SND_TIMEOUT, NULL, 0);
        goto cleanup;
    }

    RRDHOST *temp_host = NULL;
    if (!strncmp(query->data.http_api_v2.query, NODE_ID_QUERY, strlen(NODE_ID_QUERY))) {
        char *node_uuid = query->data.http_api_v2.query + strlen(NODE_ID_QUERY);
        char nodeid[UUID_STR_LEN];
        if (strlen(node_uuid) < (UUID_STR_LEN - 1)) {
            error_report(CLOUD_EMSG_MALFORMED_NODE_ID);
            retval = 1;
            w->response.code = 404;
            aclk_http_msg_v2_err(query_thr->client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_MALFORMED_NODE_ID, CLOUD_EMSG_MALFORMED_NODE_ID, NULL, 0);
            goto cleanup;
        }
        strncpyz(nodeid, node_uuid, UUID_STR_LEN - 1);

        query_host = node_id_2_rrdhost(nodeid);
        if (!query_host) {
            temp_host = sql_create_host_by_uuid(nodeid);
            if (!temp_host) {
                error_report("Host with node_id \"%s\" not found! Returning 404 to Cloud!", nodeid);
                retval = 1;
                w->response.code = 404;
                aclk_http_msg_v2_err(query_thr->client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_NODE_NOT_FOUND, CLOUD_EMSG_NODE_NOT_FOUND, NULL, 0);
                goto cleanup;
            }
        }
    }

    char *mysep = strchr(query->data.http_api_v2.query, '?');
    if (mysep) {
        url_decode_r(w->decoded_query_string, mysep, NETDATA_WEB_REQUEST_URL_SIZE + 1);
        *mysep = '\0';
    } else
        url_decode_r(w->decoded_query_string, query->data.http_api_v2.query, NETDATA_WEB_REQUEST_URL_SIZE + 1);

    mysep = strrchr(query->data.http_api_v2.query, '/');

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        int stat_idx = aclk_cloud_req_http_type_to_idx(mysep ? mysep + 1 : "other");
        aclk_metrics_per_sample.cloud_req_http_by_type[stat_idx]++;
        ACLK_STATS_UNLOCK;
    }

    // execute the query
    t = aclk_web_api_v1_request(query_host ? query_host : temp_host, w, mysep ? mysep + 1 : "noop");
    free_temporary_host(temp_host);
    size = (w->mode == WEB_CLIENT_MODE_FILECOPY) ? w->response.rlen : w->response.data->len;
    sent = size;

#ifdef NETDATA_WITH_ZLIB
    // check if gzip encoding can and should be used
    if ((start = strstr((char *)query->data.http_api_v2.payload, WEB_HDR_ACCEPT_ENC))) {
        start += strlen(WEB_HDR_ACCEPT_ENC);
        end = strstr(start, "\x0D\x0A");
        start = strstr(start, "gzip");

        if (start && start < end) {
            w->response.zstream.zalloc = Z_NULL;
            w->response.zstream.zfree = Z_NULL;
            w->response.zstream.opaque = Z_NULL;
            if(deflateInit2(&w->response.zstream, web_gzip_level, Z_DEFLATED, 15 + 16, 8, web_gzip_strategy) == Z_OK) {
                w->response.zinitialized = 1;
                w->response.zoutput = 1;
            } else
                error("Failed to initialize zlib. Proceeding without compression.");
        }
    }

    if (w->response.data->len && w->response.zinitialized) {
        w->response.zstream.next_in = (Bytef *)w->response.data->buffer;
        w->response.zstream.avail_in = w->response.data->len;
        do {
            w->response.zstream.avail_out = NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE;
            w->response.zstream.next_out = w->response.zbuffer;
            z_ret = deflate(&w->response.zstream, Z_FINISH);
            if(z_ret < 0) {
                if(w->response.zstream.msg)
                    error("Error compressing body. ZLIB error: \"%s\"", w->response.zstream.msg);
                else
                    error("Unknown error during zlib compression.");
                retval = 1;
                w->response.code = 500;
                aclk_http_msg_v2_err(query_thr->client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_ZLIB_ERROR, CLOUD_EMSG_ZLIB_ERROR, NULL, 0);
                goto cleanup;
            }
            int bytes_to_cpy = NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE - w->response.zstream.avail_out;
            buffer_need_bytes(z_buffer, bytes_to_cpy);
            memcpy(&z_buffer->buffer[z_buffer->len], w->response.zbuffer, bytes_to_cpy);
            z_buffer->len += bytes_to_cpy;
        } while(z_ret != Z_STREAM_END);
        // so that web_client_build_http_header
        // puts correct content length into header
        buffer_free(w->response.data);
        w->response.data = z_buffer;
        z_buffer = NULL;
    }
#endif

    w->response.data->date = w->tv_ready.tv_sec;
    web_client_build_http_header(w);
    local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE);
    local_buffer->contenttype = CT_APPLICATION_JSON;

    buffer_strcat(local_buffer, w->response.header_output->buffer);

    if (w->response.data->len) {
#ifdef NETDATA_WITH_ZLIB
        if (w->response.zinitialized) {
            buffer_need_bytes(local_buffer, w->response.data->len);
            memcpy(&local_buffer->buffer[local_buffer->len], w->response.data->buffer, w->response.data->len);
            local_buffer->len += w->response.data->len;
            sent = sent - size + w->response.data->len;
        } else {
#endif
            buffer_strcat(local_buffer, w->response.data->buffer);
#ifdef NETDATA_WITH_ZLIB
        }
#endif
    }

    // send msg.
    aclk_http_msg_v2(query_thr->client, query->callback_topic, query->msg_id, t, query->created, w->response.code, local_buffer->buffer, local_buffer->len);

    struct timeval tv;

cleanup:
    now_realtime_timeval(&tv);
    log_access("%llu: %d '[ACLK]:%d' '%s' (sent/all = %zu/%zu bytes %0.0f%%, prep/sent/total = %0.2f/%0.2f/%0.2f ms) %d '%s'",
        w->id
        , gettid()
        , query_thr->idx
        , "DATA"
        , sent
        , size
        , size > sent ? -(((size - sent) / (double)size) * 100.0) : ((size > 0) ? (((sent - size ) / (double)size) * 100.0) : 0.0)
        , dt_usec(&w->tv_ready, &w->tv_in) / 1000.0
        , dt_usec(&tv, &w->tv_ready) / 1000.0
        , dt_usec(&tv, &w->tv_in) / 1000.0
        , w->response.code
        , strip_control_characters((char *)buffer_tostring(log_buffer))
    );

#ifdef NETDATA_WITH_ZLIB
    if(w->response.zinitialized)
        deflateEnd(&w->response.zstream);
    buffer_free(z_buffer);
#endif
    buffer_free(w->response.data);
    buffer_free(w->response.header);
    buffer_free(w->response.header_output);
    freez(w);
    buffer_free(local_buffer);
    buffer_free(log_buffer);
    return retval;
}

static int chart_query(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    aclk_chart_msg(query_thr->client, query->data.chart_add_del.host, query->data.chart_add_del.chart_name);
    return 0;
}

static int alarm_state_update_query(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    aclk_alarm_state_msg(query_thr->client, query->data.alarm_update);
    // aclk_alarm_state_msg frees the json object including the header it generates
    query->data.alarm_update = NULL;
    return 0;
}

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
static int register_node(struct aclk_query_thread *query_thr, aclk_query_t query) {
    // TODO create a pending registrations list
    // with some timeouts to detect registration requests that
    // go unanswered from the cloud
    aclk_generate_node_registration(query_thr->client, &query->data.node_creation);
    return 0;
}

static int node_state_update(struct aclk_query_thread *query_thr, aclk_query_t query) {
    // TODO create a pending registrations list
    // with some timeouts to detect registration requests that
    // go unanswered from the cloud
    aclk_generate_node_state_update(query_thr->client, &query->data.node_update);
    return 0;
}

static int send_bin_msg(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    // this will be simplified when legacy support is removed
    aclk_send_bin_message_subtopic_pid(query_thr->client, query->data.bin_payload.payload, query->data.bin_payload.size, query->data.bin_payload.topic, query->data.bin_payload.msg_name);
    return 0;
}
#endif

aclk_query_handler aclk_query_handlers[] = {
    { .type = HTTP_API_V2,          .name = "http_api_request_v2",      .fnc = http_api_v2              },
    { .type = ALARM_STATE_UPDATE,   .name = "alarm_state_update",       .fnc = alarm_state_update_query },
    { .type = METADATA_INFO,        .name = "info_metadata",            .fnc = info_metadata            },
    { .type = METADATA_ALARMS,      .name = "alarms_metadata",          .fnc = alarms_metadata          },
    { .type = CHART_NEW,            .name = "chart_new",                .fnc = chart_query              },
    { .type = CHART_DEL,            .name = "chart_delete",             .fnc = info_metadata            },
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    { .type = REGISTER_NODE,        .name = "register_node",            .fnc = register_node            },
    { .type = NODE_STATE_UPDATE,    .name = "node_state_update",        .fnc = node_state_update        },
    { .type = CHART_DIMS_UPDATE,    .name = "chart_and_dim_update",     .fnc = send_bin_msg             },
    { .type = CHART_CONFIG_UPDATED, .name = "chart_config_updated",     .fnc = send_bin_msg             },
    { .type = CHART_RESET,          .name = "reset_chart_messages",     .fnc = send_bin_msg             },
    { .type = RETENTION_UPDATED,    .name = "update_retention_info",    .fnc = send_bin_msg             },
    { .type = UPDATE_NODE_INFO,     .name = "update_node_info",         .fnc = send_bin_msg             },
    { .type = ALARM_LOG_HEALTH,     .name = "alarm_log_health",         .fnc = send_bin_msg             },
    { .type = ALARM_PROVIDE_CFG,    .name = "provide_alarm_config",     .fnc = send_bin_msg             },
    { .type = ALARM_SNAPSHOT,       .name = "alarm_snapshot",           .fnc = send_bin_msg             },
#endif
    { .type = UNKNOWN,              .name = NULL,                       .fnc = NULL                     }
};

const char *aclk_query_get_name(aclk_query_type_t qt)
{
    aclk_query_handler *ptr = aclk_query_handlers;
    while (ptr->type != UNKNOWN) {
        if (ptr->type == qt)
            return ptr->name;
        ptr++;
    }
    return "unknown";
}

static void aclk_query_process_msg(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    for (int i = 0; aclk_query_handlers[i].type != UNKNOWN; i++) {
        if (aclk_query_handlers[i].type == query->type) {
            debug(D_ACLK, "Processing Queued Message of type: \"%s\"", aclk_query_handlers[i].name);
            aclk_query_handlers[i].fnc(query_thr, query);
            if (aclk_stats_enabled) {
                ACLK_STATS_LOCK;
                aclk_metrics_per_sample.queries_dispatched++;
                aclk_queries_per_thread[query_thr->idx]++;
                aclk_metrics_per_sample.queries_per_type[query->type]++;
                ACLK_STATS_UNLOCK;
            }
            aclk_query_free(query);
            return;
        }
    }
    fatal("Unknown query in query queue. %u", query->type);
}

/* Processes messages from queue. Compete for work with other threads
 */
int aclk_query_process_msgs(struct aclk_query_thread *query_thr)
{
    aclk_query_t query;
    //while ((query = aclk_queue_pop()))
    query = aclk_queue_pop();
    if (query)
        aclk_query_process_msg(query_thr, query);

    return 0;
}

/**
 * Main query processing thread
 */
void *aclk_query_main_thread(void *ptr)
{
    worker_register("ACLKQUERY");

    struct aclk_query_thread *query_thr = ptr;

    while (!netdata_exit) {
        worker_is_busy();
        aclk_query_process_msgs(query_thr);
        worker_is_idle();

        QUERY_THREAD_LOCK;
        if (unlikely(pthread_cond_wait(&query_cond_wait, &query_lock_wait)))
            sleep_usec(USEC_PER_SEC * 1);
        QUERY_THREAD_UNLOCK;
    }

    worker_unregister();
    return NULL;
}

#define TASK_LEN_MAX 22
void aclk_query_threads_start(struct aclk_query_threads *query_threads, mqtt_wss_client client)
{
    info("Starting %d query threads.", query_threads->count);

    char thread_name[TASK_LEN_MAX];
    query_threads->thread_list = callocz(query_threads->count, sizeof(struct aclk_query_thread));
    for (int i = 0; i < query_threads->count; i++) {
        query_threads->thread_list[i].idx = i; //thread needs to know its index for statistics

        if(unlikely(snprintfz(thread_name, TASK_LEN_MAX, "%s_%d", ACLK_QUERY_THREAD_NAME, i) < 0))
            error("snprintf encoding error");
        netdata_thread_create(
            &query_threads->thread_list[i].thread, thread_name, NETDATA_THREAD_OPTION_JOINABLE, aclk_query_main_thread,
            &query_threads->thread_list[i]);

        query_threads->thread_list[i].client = client;
    }
}

void aclk_query_threads_cleanup(struct aclk_query_threads *query_threads)
{
    if (query_threads && query_threads->thread_list) {
        for (int i = 0; i < query_threads->count; i++) {
            netdata_thread_join(query_threads->thread_list[i].thread, NULL);
        }
        freez(query_threads->thread_list);
    }
    aclk_queue_lock();
    aclk_queue_flush();
}
