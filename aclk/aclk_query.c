#include "aclk_query.h"
#include "aclk_stats.h"
#include "aclk_query_queue.h"
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
    int(*fnc)(mqtt_wss_client client, aclk_query_t query);
} aclk_query_handler;

static int info_metadata(mqtt_wss_client client, aclk_query_t query)
{
    aclk_send_info_metadata(client,
        !query->data.metadata_info.initial_on_connect,
        query->data.metadata_info.host);
    return 0;
}

static int alarms_metadata(mqtt_wss_client client, aclk_query_t query)
{
    aclk_send_alarm_metadata(client,
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

static int http_api_v2(mqtt_wss_client client, aclk_query_t query)
{
    int retval = 0;
    usec_t t;
    BUFFER *local_buffer = NULL;

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

    char *mysep = strchr(query->data.http_api_v2.query, '?');
    if (mysep) {
        url_decode_r(w->decoded_query_string, mysep, NETDATA_WEB_REQUEST_URL_SIZE + 1);
        *mysep = '\0';
    } else
        url_decode_r(w->decoded_query_string, query->data.http_api_v2.query, NETDATA_WEB_REQUEST_URL_SIZE + 1);

    mysep = strrchr(query->data.http_api_v2.query, '/');

    // execute the query
    t = aclk_web_api_v1_request(localhost, w, mysep ? mysep + 1 : "noop");

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
                goto cleanup;
            }
            int bytes_to_cpy = NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE - w->response.zstream.avail_out;
            buffer_need_bytes(z_buffer, bytes_to_cpy);
            memcpy(&z_buffer->buffer[z_buffer->len], w->response.zbuffer, bytes_to_cpy);
            z_buffer->len += bytes_to_cpy;
        } while(z_ret != Z_STREAM_END);
        // so that web_client_build_http_header
        // puts correct content lenght into header
        buffer_free(w->response.data);
        w->response.data = z_buffer;
        z_buffer = NULL;
    }
#endif

    now_realtime_timeval(&w->tv_ready);
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
        } else {
#endif
            buffer_strcat(local_buffer, w->response.data->buffer);
#ifdef NETDATA_WITH_ZLIB
        }
#endif
    }

    aclk_http_msg_v2(client, query->callback_topic, query->msg_id, t, query->created, w->response.code, local_buffer->buffer, local_buffer->len);

cleanup:
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
    return retval;
}

static int chart_query(mqtt_wss_client client, aclk_query_t query)
{
    aclk_chart_msg(client, query->data.chart_add_del.host, query->data.chart_add_del.chart_name);
    return 0;
}

static int alarm_state_update_query(mqtt_wss_client client, aclk_query_t query)
{
    aclk_alarm_state_msg(client, query->data.alarm_update);
    // aclk_alarm_state_msg frees the json object including the header it generates
    query->data.alarm_update = NULL;
    return 0;
}

aclk_query_handler aclk_query_handlers[] = {
    { .type = HTTP_API_V2,        .name = "http api request v2", .fnc = http_api_v2              },
    { .type = ALARM_STATE_UPDATE, .name = "alarm state update",  .fnc = alarm_state_update_query },
    { .type = METADATA_INFO,      .name = "info metadata",       .fnc = info_metadata            },
    { .type = METADATA_ALARMS,    .name = "alarms metadata",     .fnc = alarms_metadata          },
    { .type = CHART_NEW,          .name = "chart new",           .fnc = chart_query              },
    { .type = CHART_DEL,          .name = "chart delete",        .fnc = info_metadata            },
    { .type = UNKNOWN,            .name = NULL,                  .fnc = NULL                     }
};


static void aclk_query_process_msg(struct aclk_query_thread *info, aclk_query_t query)
{
    for (int i = 0; aclk_query_handlers[i].type != UNKNOWN; i++) {
        if (aclk_query_handlers[i].type == query->type) {
            debug(D_ACLK, "Processing Queued Message of type: \"%s\"", aclk_query_handlers[i].name);
            aclk_query_handlers[i].fnc(info->client, query);
            aclk_query_free(query);
            if (aclk_stats_enabled) {
                ACLK_STATS_LOCK;
                aclk_metrics_per_sample.queries_dispatched++;
                aclk_queries_per_thread[info->idx]++;
                ACLK_STATS_UNLOCK;
            }
            return;
        }
    }
    fatal("Unknown query in query queue. %u", query->type);
}

/* Processes messages from queue. Compete for work with other threads
 */
int aclk_query_process_msgs(struct aclk_query_thread *info)
{
    aclk_query_t query;
    while ((query = aclk_queue_pop()))
        aclk_query_process_msg(info, query);

    return 0;
}

/**
 * Main query processing thread
 */
void *aclk_query_main_thread(void *ptr)
{
    struct aclk_query_thread *info = ptr;
    while (!netdata_exit) {
        ACLK_SHARED_STATE_LOCK;
        if (unlikely(!aclk_shared_state.version_neg)) {
            if (!aclk_shared_state.version_neg_wait_till || aclk_shared_state.version_neg_wait_till > now_monotonic_usec()) {
                ACLK_SHARED_STATE_UNLOCK;
                info("Waiting for ACLK Version Negotiation message from Cloud");
                sleep(1);
                continue;
            }
            errno = 0;
            error("ACLK version negotiation failed. No reply to \"hello\" with \"version\" from cloud in time of %ds."
                " Reverting to default ACLK version of %d.", VERSION_NEG_TIMEOUT, ACLK_VERSION_MIN);
            aclk_shared_state.version_neg = ACLK_VERSION_MIN;
// When ACLK v3 is implemented you will need this
//            aclk_set_rx_handlers(aclk_shared_state.version_neg);
        }
        ACLK_SHARED_STATE_UNLOCK;

        aclk_query_process_msgs(info);

        QUERY_THREAD_LOCK;

        if (unlikely(pthread_cond_wait(&query_cond_wait, &query_lock_wait)))
            sleep_usec(USEC_PER_SEC * 1);

        QUERY_THREAD_UNLOCK;
    }
    return NULL;
}

#define TASK_LEN_MAX 16
void aclk_query_threads_start(struct aclk_query_threads *query_threads, mqtt_wss_client client)
{
    info("Starting %d query threads.", query_threads->count);

    char thread_name[TASK_LEN_MAX];
    query_threads->thread_list = callocz(query_threads->count, sizeof(struct aclk_query_thread));
    for (int i = 0; i < query_threads->count; i++) {
        query_threads->thread_list[i].idx = i; //thread needs to know its index for statistics

        if(unlikely(snprintf(thread_name, TASK_LEN_MAX, "%s_%d", ACLK_QUERY_THREAD_NAME, i) < 0))
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
