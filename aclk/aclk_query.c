// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query.h"
#include "aclk_stats.h"
#include "aclk_tx_msgs.h"
#include "../../web/server/web_client_cache.h"

#define WEB_HDR_ACCEPT_ENC "Accept-Encoding:"
#define ACLK_MAX_WEB_RESPONSE_SIZE (30 * 1024 * 1024)

pthread_cond_t query_cond_wait = PTHREAD_COND_INITIALIZER;
pthread_mutex_t query_lock_wait = PTHREAD_MUTEX_INITIALIZER;
#define QUERY_THREAD_LOCK pthread_mutex_lock(&query_lock_wait)
#define QUERY_THREAD_UNLOCK pthread_mutex_unlock(&query_lock_wait)

struct pending_req_list {
    const char *msg_id;
    uint32_t hash;

    int canceled;

    struct pending_req_list *next;
};

static struct pending_req_list *pending_req_list_head = NULL;
static pthread_mutex_t pending_req_list_lock = PTHREAD_MUTEX_INITIALIZER;

static struct pending_req_list *pending_req_list_add(const char *msg_id)
{
    struct pending_req_list *new = callocz(1, sizeof(struct pending_req_list));
    new->msg_id = msg_id;
    new->hash = simple_hash(msg_id);

    pthread_mutex_lock(&pending_req_list_lock);
    new->next = pending_req_list_head;
    pending_req_list_head = new;
    pthread_mutex_unlock(&pending_req_list_lock);
    return new;
}

void pending_req_list_rm(const char *msg_id)
{
    uint32_t hash = simple_hash(msg_id);
    struct pending_req_list *prev = NULL;

    pthread_mutex_lock(&pending_req_list_lock);
    struct pending_req_list *curr = pending_req_list_head;

    while (curr) {
        if (curr->hash == hash && strcmp(curr->msg_id, msg_id) == 0) {
            if (prev)
                prev->next = curr->next;
            else
                pending_req_list_head = curr->next;

            freez(curr);
            break;
        }

        prev = curr;
        curr = curr->next;
    }
    pthread_mutex_unlock(&pending_req_list_lock);
}

int mark_pending_req_cancelled(const char *msg_id)
{
    uint32_t hash = simple_hash(msg_id);

    pthread_mutex_lock(&pending_req_list_lock);
    struct pending_req_list *curr = pending_req_list_head;

    while (curr) {
        if (curr->hash == hash && strcmp(curr->msg_id, msg_id) == 0) {
            curr->canceled = 1;
            pthread_mutex_unlock(&pending_req_list_lock);
            return 0;
        }

        curr = curr->next;
    }
    pthread_mutex_unlock(&pending_req_list_lock);
    return 1;
}

static bool aclk_web_client_interrupt_cb(struct web_client *w __maybe_unused, void *data)
{
    struct pending_req_list *req = (struct pending_req_list *)data;
    return req->canceled;
}

static int http_api_v2(struct aclk_query_thread *query_thr, aclk_query_t query) {
    int retval = 0;
    BUFFER *local_buffer = NULL;
    size_t size = 0;
    size_t sent = 0;

    int z_ret;
    BUFFER *z_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, &netdata_buffers_statistics.buffers_aclk);
    char *start, *end;

    struct web_client *w = web_client_get_from_cache();
    w->acl = WEB_CLIENT_ACL_ACLK;
    w->mode = WEB_CLIENT_MODE_GET;
    w->timings.tv_in = query->created_tv;

    w->interrupt.callback = aclk_web_client_interrupt_cb;
    w->interrupt.callback_data = pending_req_list_add(query->msg_id);

    usec_t t;
    web_client_timeout_checkpoint_set(w, query->timeout);
    if(web_client_timeout_checkpoint_and_check(w, &t)) {
        log_access("QUERY CANCELED: QUEUE TIME EXCEEDED %llu ms (LIMIT %d ms)", t / USEC_PER_MS, query->timeout);
        retval = 1;
        w->response.code = HTTP_RESP_BACKEND_FETCH_FAILED;
        aclk_http_msg_v2_err(query_thr->client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_SND_TIMEOUT, CLOUD_EMSG_SND_TIMEOUT, NULL, 0);
        goto cleanup;
    }

    web_client_decode_path_and_query_string(w, query->data.http_api_v2.query);
    char *path = (char *)buffer_tostring(w->url_path_decoded);

    if (aclk_stats_enabled) {
        char *url_path_endpoint = strrchr(path, '/');
        ACLK_STATS_LOCK;
        int stat_idx = aclk_cloud_req_http_type_to_idx(url_path_endpoint ? url_path_endpoint + 1 : "other");
        aclk_metrics_per_sample.cloud_req_http_by_type[stat_idx]++;
        ACLK_STATS_UNLOCK;
    }

    w->response.code = web_client_api_request_with_node_selection(localhost, w, path);
    web_client_timeout_checkpoint_response_ready(w, &t);

    if(buffer_strlen(w->response.data) > ACLK_MAX_WEB_RESPONSE_SIZE) {
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "response is too big");
        w->response.data->content_type = CT_TEXT_PLAIN;
        w->response.code = HTTP_RESP_CONTENT_TOO_LONG;
    }

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.cloud_q_process_total += t;
        aclk_metrics_per_sample.cloud_q_process_count++;
        if (aclk_metrics_per_sample.cloud_q_process_max < t)
            aclk_metrics_per_sample.cloud_q_process_max = t;
        ACLK_STATS_UNLOCK;
    }

    size = w->response.data->len;
    sent = size;

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
                w->response.zinitialized = true;
                w->response.zoutput = true;
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

    w->response.data->date = w->timings.tv_ready.tv_sec;
    web_client_build_http_header(w);
    local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, &netdata_buffers_statistics.buffers_aclk);
    local_buffer->content_type = CT_APPLICATION_JSON;

    buffer_strcat(local_buffer, w->response.header_output->buffer);

    if (w->response.data->len) {
        if (w->response.zinitialized) {
            buffer_need_bytes(local_buffer, w->response.data->len);
            memcpy(&local_buffer->buffer[local_buffer->len], w->response.data->buffer, w->response.data->len);
            local_buffer->len += w->response.data->len;
            sent = sent - size + w->response.data->len;
        } else {
            buffer_strcat(local_buffer, w->response.data->buffer);
        }
    }

    // send msg.
    w->response.code = aclk_http_msg_v2(query_thr->client, query->callback_topic, query->msg_id, t, query->created, w->response.code, local_buffer->buffer, local_buffer->len);

    struct timeval tv;

cleanup:
    now_monotonic_high_precision_timeval(&tv);
    log_access("%llu: %d '[ACLK]:%d' '%s' (sent/all = %zu/%zu bytes %0.0f%%, prep/sent/total = %0.2f/%0.2f/%0.2f ms) %d '%s'",
        w->id
        , gettid()
        , query_thr->idx
        , "DATA"
        , sent
        , size
        , size > sent ? -(((size - sent) / (double)size) * 100.0) : ((size > 0) ? (((sent - size ) / (double)size) * 100.0) : 0.0)
        , dt_usec(&w->timings.tv_ready, &w->timings.tv_in) / 1000.0
        , dt_usec(&tv, &w->timings.tv_ready) / 1000.0
        , dt_usec(&tv, &w->timings.tv_in) / 1000.0
        , w->response.code
        , strip_control_characters((char *)buffer_tostring(w->url_as_received))
    );

    web_client_release_to_cache(w);

    pending_req_list_rm(query->msg_id);

    buffer_free(z_buffer);
    buffer_free(local_buffer);
    return retval;
}

static int send_bin_msg(struct aclk_query_thread *query_thr, aclk_query_t query)
{
    // this will be simplified when legacy support is removed
    aclk_send_bin_message_subtopic_pid(query_thr->client, query->data.bin_payload.payload, query->data.bin_payload.size, query->data.bin_payload.topic, query->data.bin_payload.msg_name);
    return 0;
}

const char *aclk_query_get_name(aclk_query_type_t qt, int unknown_ok)
{
    switch (qt) {
        case HTTP_API_V2:               return "http_api_request_v2";
        case REGISTER_NODE:             return "register_node";
        case NODE_STATE_UPDATE:         return "node_state_update";
        case CHART_DIMS_UPDATE:         return "chart_and_dim_update";
        case CHART_CONFIG_UPDATED:      return "chart_config_updated";
        case CHART_RESET:               return "reset_chart_messages";
        case RETENTION_UPDATED:         return "update_retention_info";
        case UPDATE_NODE_INFO:          return "update_node_info";
        case ALARM_PROVIDE_CHECKPOINT:  return "alarm_checkpoint";
        case ALARM_PROVIDE_CFG:         return "provide_alarm_config";
        case ALARM_SNAPSHOT:            return "alarm_snapshot";
        case UPDATE_NODE_COLLECTORS:    return "update_node_collectors";
        case PROTO_BIN_MESSAGE:         return "generic_binary_proto_message";
        default:
            if (!unknown_ok)
                error_report("Unknown query type used %d", (int) qt);
            return "unknown";
    }
}

static void aclk_query_process_msg(struct aclk_query_thread *query_thr, aclk_query_t query)
{   
    if (query->type == UNKNOWN || query->type >= ACLK_QUERY_TYPE_COUNT) {
        error_report("Unknown query in query queue. %u", query->type);
        aclk_query_free(query);
        return;
    }

    worker_is_busy(query->type);
    if (query->type == HTTP_API_V2) {
        debug(D_ACLK, "Processing Queued Message of type: \"http_api_request_v2\"");
        http_api_v2(query_thr, query);
    } else {
        debug(D_ACLK, "Processing Queued Message of type: \"%s\"", query->data.bin_payload.msg_name);
        send_bin_msg(query_thr, query);
    }

    if (aclk_stats_enabled) {
        ACLK_STATS_LOCK;
        aclk_metrics_per_sample.queries_dispatched++;
        aclk_queries_per_thread[query_thr->idx]++;
        aclk_metrics_per_sample.queries_per_type[query->type]++;
        ACLK_STATS_UNLOCK;
    }

    aclk_query_free(query);

    worker_is_idle();
}

/* Processes messages from queue. Compete for work with other threads
 */
int aclk_query_process_msgs(struct aclk_query_thread *query_thr)
{
    aclk_query_t query;
    while ((query = aclk_queue_pop()))
        aclk_query_process_msg(query_thr, query);

    return 0;
}

static void worker_aclk_register(void) {
    worker_register("ACLKQUERY");
    for (int i = 1; i < ACLK_QUERY_TYPE_COUNT; i++) {
        worker_register_job_name(i, aclk_query_get_name(i, 0));
    }
}

static void aclk_query_request_cancel(void *data)
{
    pthread_cond_broadcast((pthread_cond_t *) data);
}

/**
 * Main query processing thread
 */
void *aclk_query_main_thread(void *ptr)
{
    worker_aclk_register();

    struct aclk_query_thread *query_thr = ptr;

    service_register(SERVICE_THREAD_TYPE_NETDATA, aclk_query_request_cancel, NULL, &query_cond_wait, false);

    while (service_running(SERVICE_ACLK | ABILITY_DATA_QUERIES)) {
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
        query_threads->thread_list[i].client = client;

        if(unlikely(snprintfz(thread_name, TASK_LEN_MAX, "ACLK_QRY[%d]", i) < 0))
            error("snprintf encoding error");
        netdata_thread_create(
            &query_threads->thread_list[i].thread, thread_name, NETDATA_THREAD_OPTION_JOINABLE, aclk_query_main_thread,
            &query_threads->thread_list[i]);
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
