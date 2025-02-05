// SPDX-License-Identifier: GPL-3.0-or-later

#include "aclk_query.h"
#include "aclk_tx_msgs.h"
#include "web/server/web_client_cache.h"

static HTTP_ACL default_aclk_http_acl = HTTP_ACL_ALL_FEATURES;

struct pending_req_list {
    const char *msg_id;
    uint32_t hash;

    int canceled;

    struct pending_req_list *next;
};

static struct pending_req_list *pending_req_list_head = NULL;
static SPINLOCK pending_req_list_lock = SPINLOCK_INITIALIZER;

void aclk_config_get_query_scope(void) {
    const char *s = inicfg_get(&netdata_config, CONFIG_SECTION_CLOUD, "scope", "full");
    if(strcmp(s, "license manager") == 0)
        default_aclk_http_acl = HTTP_ACL_ACLK_LICENSE_MANAGER;
}

bool aclk_query_scope_has(HTTP_ACL acl) {
    return (default_aclk_http_acl & acl) == acl;
}

static struct pending_req_list *pending_req_list_add(const char *msg_id)
{
    struct pending_req_list *new = callocz(1, sizeof(struct pending_req_list));
    new->msg_id = msg_id;
    new->hash = simple_hash(msg_id);

    spinlock_lock(&pending_req_list_lock);
    new->next = pending_req_list_head;
    pending_req_list_head = new;
    spinlock_unlock(&pending_req_list_lock);
    return new;
}

void pending_req_list_rm(const char *msg_id)
{
    uint32_t hash = simple_hash(msg_id);
    struct pending_req_list *prev = NULL;

    spinlock_lock(&pending_req_list_lock);
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
    spinlock_unlock(&pending_req_list_lock);
}

int mark_pending_req_cancelled(const char *msg_id)
{
    uint32_t hash = simple_hash(msg_id);

    spinlock_lock(&pending_req_list_lock);
    struct pending_req_list *curr = pending_req_list_head;

    while (curr) {
        if (curr->hash == hash && strcmp(curr->msg_id, msg_id) == 0) {
            curr->canceled = 1;
            spinlock_unlock(&pending_req_list_lock);
            return 0;
        }

        curr = curr->next;
    }
    spinlock_unlock(&pending_req_list_lock);
    return 1;
}

static bool aclk_web_client_interrupt_cb(struct web_client *w __maybe_unused, void *data)
{
    struct pending_req_list *req = (struct pending_req_list *)data;
    return req->canceled;
}

int http_api_v2(mqtt_wss_client client, aclk_query_t query)
{
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_TRANSPORT, "aclk"),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    int retval = 0;
    BUFFER *local_buffer = NULL;
    usec_t dt_ut = 0;

    int z_ret;
    BUFFER *z_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, &netdata_buffers_statistics.buffers_aclk);

    struct web_client *w = web_client_get_from_cache();
    web_client_set_conn_cloud(w);
    w->port_acl = HTTP_ACL_ACLK | default_aclk_http_acl;
    w->acl = w->port_acl;
    web_client_set_permissions(w, HTTP_ACCESS_MAP_OLD_MEMBER, HTTP_USER_ROLE_MEMBER, WEB_CLIENT_FLAG_AUTH_CLOUD);

    w->mode = HTTP_REQUEST_MODE_GET;
    w->timings.tv_in = query->created_tv;

    w->interrupt.callback = aclk_web_client_interrupt_cb;
    w->interrupt.callback_data = pending_req_list_add(query->msg_id);

    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, query->data.http_api_v2.payload);

    HTTP_VALIDATION validation = http_request_validate(w);
    if(validation != HTTP_VALIDATION_OK) {
        nd_log(NDLS_ACCESS, NDLP_ERR, "ACLK received request is not valid, code %d", validation);
        retval = 1;
        w->response.code = HTTP_RESP_BAD_REQUEST;
        w->response.code = (short)aclk_http_msg_v2(client, query->callback_topic, query->msg_id,
                                                   dt_ut, query->created, w->response.code,
                                                   NULL, 0);
        goto cleanup;
    }

    web_client_timeout_checkpoint_set(w, query->timeout);
    if(web_client_timeout_checkpoint_and_check(w, &dt_ut)) {
        nd_log(NDLS_ACCESS, NDLP_ERR,
               "QUERY CANCELED: QUEUE TIME EXCEEDED %llu ms (LIMIT %d ms)",
               dt_ut / USEC_PER_MS, query->timeout);
        retval = 1;
        w->response.code = HTTP_RESP_SERVICE_UNAVAILABLE;
        aclk_http_msg_v2_err(client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_SND_TIMEOUT, CLOUD_EMSG_SND_TIMEOUT, NULL, 0);
        goto cleanup;
    }

    char *path = (char *)buffer_tostring(w->url_path_decoded);

    w->response.code = (short)web_client_api_request_with_node_selection(localhost, w, path);
    web_client_timeout_checkpoint_response_ready(w, &dt_ut);

    if (w->response.data->len && w->response.zinitialized) {
        w->response.zstream.next_in = (Bytef *)w->response.data->buffer;
        w->response.zstream.avail_in = w->response.data->len;
        do {
            w->response.zstream.avail_out = NETDATA_WEB_RESPONSE_ZLIB_CHUNK_SIZE;
            w->response.zstream.next_out = w->response.zbuffer;
            z_ret = deflate(&w->response.zstream, Z_FINISH);
            if(z_ret < 0) {
                if(w->response.zstream.msg)
                    netdata_log_error("Error compressing body. ZLIB error: \"%s\"", w->response.zstream.msg);
                else
                    netdata_log_error("Unknown error during zlib compression.");
                retval = 1;
                w->response.code = 500;
                aclk_http_msg_v2_err(client, query->callback_topic, query->msg_id, w->response.code, CLOUD_EC_ZLIB_ERROR, CLOUD_EMSG_ZLIB_ERROR, NULL, 0);
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

    web_client_build_http_header(w);
    local_buffer = buffer_create(NETDATA_WEB_RESPONSE_INITIAL_SIZE, &netdata_buffers_statistics.buffers_aclk);
    local_buffer->content_type = CT_APPLICATION_JSON;

    buffer_strcat(local_buffer, w->response.header_output->buffer);

    if (w->response.data->len) {
        if (w->response.zinitialized) {
            buffer_need_bytes(local_buffer, w->response.data->len);
            memcpy(&local_buffer->buffer[local_buffer->len], w->response.data->buffer, w->response.data->len);
            local_buffer->len += w->response.data->len;
        } else
            buffer_strcat(local_buffer, w->response.data->buffer);
    }

    // send msg.
    w->response.code = (short)aclk_http_msg_v2(
        client,
        query->callback_topic,
        query->msg_id,
        dt_ut,
        query->created,
        w->response.code,
        local_buffer->buffer,
        local_buffer->len);

cleanup:
    web_client_log_completed_request(w, false);
    web_client_release_to_cache(w);

    pending_req_list_rm(query->msg_id);

    buffer_free(z_buffer);
    buffer_free(local_buffer);
    return retval;
}

int send_bin_msg(mqtt_wss_client client, aclk_query_t query)
{
    // this will be simplified when legacy support is removed
    aclk_send_bin_message_subtopic_pid(
        client,
        query->data.bin_payload.payload,
        query->data.bin_payload.size,
        query->data.bin_payload.topic,
        query->data.bin_payload.msg_name);
    return 0;
}
