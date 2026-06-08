#include "netipc_service_common.h"

#include "netipc/netipc_protocol.h"

#include <stdlib.h>
#include <string.h>

#define NIPC_SERVICE_RESPONSE_BUF_DEFAULT 65536u

uint32_t nipc_service_common_next_power_of_2_u32(uint32_t n)
{
    if (n < 16)
        return 16;
    n--;
    n |= n >> 1;
    n |= n >> 2;
    n |= n >> 4;
    n |= n >> 8;
    n |= n >> 16;
    return n + 1;
}

bool nipc_service_common_header_payload_len(size_t payload_len,
                                            size_t *msg_len_out)
{
#if SIZE_MAX <= UINT32_MAX
    if (payload_len > SIZE_MAX - NIPC_HEADER_LEN)
        return false;
#endif

    *msg_len_out = NIPC_HEADER_LEN + payload_len;
    return true;
}

bool nipc_service_common_header_payload_len_u32(uint32_t payload_len,
                                                uint32_t *msg_len_out)
{
    if (payload_len > UINT32_MAX - NIPC_HEADER_LEN)
        return false;

    *msg_len_out = payload_len + NIPC_HEADER_LEN;
    return true;
}

bool nipc_service_common_mul_would_overflow(size_t count, size_t size)
{
    return size != 0 && count > SIZE_MAX / size;
}

uint32_t nipc_service_common_request_payload_default(void)
{
    return NIPC_MAX_PAYLOAD_DEFAULT;
}

uint32_t nipc_service_common_response_payload_default(void)
{
    return NIPC_SERVICE_RESPONSE_BUF_DEFAULT;
}

/* Typed services expose one batch-count knob; level 1 keeps request/response counts symmetric. */
uint32_t nipc_service_common_typed_response_batch_items(uint32_t max_request_batch_items)
{
    return max_request_batch_items;
}

void nipc_service_common_copy_cstr_field(char *dst, size_t dst_size, const char *src)
{
    if (!dst || dst_size == 0)
        return;

    dst[0] = '\0';
    if (!src)
        return;

    size_t len = 0;
    size_t max_copy = dst_size - 1;
    while (len < max_copy && src[len] != '\0')
        len++;

    memcpy(dst, src, len);
    dst[len] = '\0';
}

bool nipc_service_common_client_transport_fields(
    nipc_service_common_transport_fields_t *fields,
    const nipc_client_config_t *config)
{
    memset(fields, 0, sizeof(*fields));
    if (!config)
        return false;

    fields->supported_profiles = config->supported_profiles;
    fields->preferred_profiles = config->preferred_profiles;
    fields->max_request_batch_items = config->max_request_batch_items;
    fields->max_response_payload_bytes = config->max_response_payload_bytes;
    fields->max_response_batch_items =
        nipc_service_common_typed_response_batch_items(config->max_request_batch_items);
    fields->auth_token = config->auth_token;
    return true;
}

bool nipc_service_common_server_transport_fields(
    nipc_service_common_transport_fields_t *fields,
    const nipc_server_config_t *config)
{
    memset(fields, 0, sizeof(*fields));
    if (!config)
        return false;

    fields->supported_profiles = config->supported_profiles;
    fields->preferred_profiles = config->preferred_profiles;
    fields->max_request_batch_items = config->max_request_batch_items;
    fields->max_response_payload_bytes = config->max_response_payload_bytes;
    fields->max_response_batch_items =
        nipc_service_common_typed_response_batch_items(config->max_request_batch_items);
    fields->auth_token = config->auth_token;
    return true;
}

void nipc_service_common_client_init(nipc_client_ctx_t *ctx,
                                     const char *run_dir,
                                     const char *service_name)
{
    memset(ctx, 0, sizeof(*ctx));
    ctx->state = NIPC_CLIENT_DISCONNECTED;
    ctx->session_valid = false;
    ctx->shm = NULL;
    nipc_service_common_copy_cstr_field(ctx->run_dir, sizeof(ctx->run_dir), run_dir);
    nipc_service_common_copy_cstr_field(ctx->service_name, sizeof(ctx->service_name), service_name);
}

void nipc_service_common_client_status(const nipc_client_ctx_t *ctx,
                                       nipc_client_status_t *out)
{
    out->state = ctx->state;
    out->connect_count = ctx->connect_count;
    out->reconnect_count = ctx->reconnect_count;
    out->call_count = ctx->call_count;
    out->error_count = ctx->error_count;
}

void nipc_service_common_client_close_buffers(nipc_client_ctx_t *ctx)
{
    free(ctx->response_buf);
    free(ctx->send_buf);
    ctx->response_buf = NULL;
    ctx->send_buf = NULL;
    ctx->response_buf_size = 0;
    ctx->send_buf_size = 0;
    ctx->state = NIPC_CLIENT_DISCONNECTED;
}

bool nipc_service_common_client_refresh(nipc_client_ctx_t *ctx,
                                        const nipc_service_common_client_ops_t *ops)
{
    nipc_client_state_t old_state = ctx->state;

    switch (ctx->state) {
    case NIPC_CLIENT_DISCONNECTED:
    case NIPC_CLIENT_NOT_FOUND:
        ctx->state = NIPC_CLIENT_CONNECTING;
        ctx->state = ops->try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            ctx->connect_count++;
        break;

    case NIPC_CLIENT_BROKEN:
        ops->disconnect(ctx);
        ctx->state = NIPC_CLIENT_CONNECTING;
        ctx->state = ops->try_connect(ctx);
        if (ctx->state == NIPC_CLIENT_READY)
            ctx->reconnect_count++;
        break;

    case NIPC_CLIENT_READY:
    case NIPC_CLIENT_CONNECTING:
    case NIPC_CLIENT_AUTH_FAILED:
    case NIPC_CLIENT_INCOMPATIBLE:
        break;
    }

    return ctx->state != old_state;
}

void nipc_service_common_client_note_request_capacity(nipc_client_ctx_t *ctx,
                                                      uint32_t payload_len)
{
    uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
    if (grown > NIPC_MAX_PAYLOAD_CAP)
        grown = NIPC_MAX_PAYLOAD_CAP;
    if (grown > ctx->transport_config.max_request_payload_bytes)
        ctx->transport_config.max_request_payload_bytes = grown;
}

void nipc_service_common_client_note_response_capacity(nipc_client_ctx_t *ctx,
                                                       uint32_t payload_len)
{
    uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
    if (grown > NIPC_MAX_PAYLOAD_CAP)
        grown = NIPC_MAX_PAYLOAD_CAP;
    if (grown > ctx->transport_config.max_response_payload_bytes)
        ctx->transport_config.max_response_payload_bytes = grown;
}

nipc_error_t nipc_service_common_response_status_to_error(nipc_client_ctx_t *ctx,
                                                          const nipc_header_t *resp_hdr)
{
    switch (resp_hdr->transport_status) {
    case NIPC_STATUS_OK:
        return NIPC_OK;
    case NIPC_STATUS_LIMIT_EXCEEDED:
        if (ctx->session.max_response_payload_bytes > 0) {
            uint32_t current = ctx->session.max_response_payload_bytes;
            nipc_service_common_client_note_response_capacity(
                ctx, current >= UINT32_MAX / 2u ? UINT32_MAX : current * 2u);
        }
        return NIPC_ERR_OVERFLOW;
    case NIPC_STATUS_UNSUPPORTED:
        return NIPC_ERR_BAD_LAYOUT;
    case NIPC_STATUS_BAD_ENVELOPE:
    case NIPC_STATUS_INTERNAL_ERROR:
    default:
        return NIPC_ERR_BAD_LAYOUT;
    }
}

nipc_error_t nipc_service_common_do_raw_call(
    nipc_client_ctx_t *ctx,
    uint16_t method_code,
    const void *request_payload,
    size_t request_len,
    const void **response_payload_out,
    size_t *response_len_out,
    nipc_service_common_transport_send_fn send_fn,
    nipc_service_common_transport_receive_fn receive_fn)
{
    nipc_header_t hdr = {0};
    hdr.kind             = NIPC_KIND_REQUEST;
    hdr.code             = method_code;
    hdr.flags            = 0;
    hdr.item_count       = 1;
    hdr.message_id       = (uint64_t)(ctx->call_count + 1);
    hdr.transport_status = NIPC_STATUS_OK;

    nipc_error_t err = send_fn(ctx, &hdr, request_payload, request_len);
    if (err != NIPC_OK)
        return err;

    nipc_header_t resp_hdr;
    err = receive_fn(ctx, ctx->response_buf, ctx->response_buf_size,
                     &resp_hdr, response_payload_out, response_len_out);
    if (err != NIPC_OK)
        return err;

    if (resp_hdr.kind != NIPC_KIND_RESPONSE)
        return NIPC_ERR_BAD_KIND;
    if (resp_hdr.code != method_code)
        return NIPC_ERR_BAD_LAYOUT;
    if (resp_hdr.message_id != hdr.message_id)
        return NIPC_ERR_BAD_LAYOUT;
    return nipc_service_common_response_status_to_error(ctx, &resp_hdr);
}

nipc_error_t nipc_service_common_call_with_retry(
    nipc_client_ctx_t *ctx,
    nipc_service_common_attempt_fn attempt,
    void *state,
    const nipc_service_common_client_ops_t *ops)
{
    if (ctx->state != NIPC_CLIENT_READY) {
        ctx->error_count++;
        return NIPC_ERR_NOT_READY;
    }

    int overflow_retries = 0;
    for (;;) {
        uint32_t prev_req = ctx->session.max_request_payload_bytes;
        uint32_t prev_resp = ctx->session.max_response_payload_bytes;
        uint32_t prev_cfg_req = ctx->transport_config.max_request_payload_bytes;
        uint32_t prev_cfg_resp = ctx->transport_config.max_response_payload_bytes;

        nipc_error_t err = attempt(ctx, state);
        if (err == NIPC_OK) {
            ctx->call_count++;
            return NIPC_OK;
        }

        if (err != NIPC_ERR_OVERFLOW) {
            ops->disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            if (ops->reconnect_drain_ms > 0 && ops->sleep_ms)
                ops->sleep_ms(ops->reconnect_drain_ms);
            if (!ops->reconnect_for_call(ctx)) {
                ctx->error_count++;
                return err;
            }

            ctx->reconnect_count++;
            if (ops->sleep_ms && ops->reconnect_retry_interval_ms > 0)
                ops->sleep_ms(ops->reconnect_retry_interval_ms);
            err = attempt(ctx, state);
            if (err == NIPC_OK) {
                ctx->call_count++;
                return NIPC_OK;
            }

            ops->disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->error_count++;
            return err;
        }

        ops->disconnect(ctx);
        ctx->state = NIPC_CLIENT_BROKEN;
        if (!ops->reconnect_for_call(ctx)) {
            ctx->error_count++;
            return err;
        }
        ctx->reconnect_count++;

        if (ctx->session.max_request_payload_bytes <= prev_req &&
            ctx->session.max_response_payload_bytes <= prev_resp &&
            ctx->transport_config.max_request_payload_bytes <= prev_cfg_req &&
            ctx->transport_config.max_response_payload_bytes <= prev_cfg_resp) {
            ops->disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->error_count++;
            return err;
        }

        if (++overflow_retries >= 8) {
            ops->disconnect(ctx);
            ctx->state = NIPC_CLIENT_BROKEN;
            ctx->error_count++;
            return err;
        }
        if (ops->sleep_ms && ops->reconnect_retry_interval_ms > 0)
            ops->sleep_ms(ops->reconnect_retry_interval_ms);
    }
}

nipc_error_t nipc_service_common_server_init_base(
    nipc_managed_server_t *server,
    const char *run_dir,
    const char *service_name,
    int worker_count,
    uint16_t expected_method_code,
    nipc_server_handler_fn handler,
    void *user,
    uint32_t max_request_payload_bytes,
    uint32_t max_response_payload_bytes)
{
    if (!run_dir || !service_name || !handler)
        return NIPC_ERR_BAD_LAYOUT;

    if (worker_count < 1)
        worker_count = 1;

    nipc_service_common_copy_cstr_field(server->run_dir, sizeof(server->run_dir),
                                        run_dir);
    nipc_service_common_copy_cstr_field(server->service_name,
                                        sizeof(server->service_name),
                                        service_name);

    server->handler = handler;
    server->handler_user = user;
    server->worker_count = worker_count;
    server->expected_method_code = expected_method_code;
    server->learned_request_payload_bytes =
        max_request_payload_bytes > 0
            ? max_request_payload_bytes
            : NIPC_MAX_PAYLOAD_DEFAULT;
    server->learned_response_payload_bytes =
        max_response_payload_bytes > 0
            ? max_response_payload_bytes
            : NIPC_MAX_PAYLOAD_DEFAULT;
    server->session_capacity = worker_count * 2;
    if (server->session_capacity < 16)
        server->session_capacity = 16;
    server->session_count = 0;
    server->next_session_id = 1;
    return NIPC_OK;
}

nipc_error_t nipc_service_common_server_alloc_sessions(
    nipc_managed_server_t *server,
    nipc_service_common_calloc_fn calloc_fn,
    int fault_site)
{
    server->sessions = calloc_fn((size_t)server->session_capacity,
                                 sizeof(nipc_session_ctx_t *),
                                 fault_site);
    return server->sessions ? NIPC_OK : NIPC_ERR_OVERFLOW;
}

void nipc_service_common_server_note_request_capacity(nipc_managed_server_t *server,
                                                      uint32_t payload_len)
{
    uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
#if defined(_WIN32) || defined(__MSYS__)
    uint32_t current = server->learned_request_payload_bytes;
    while (grown > current) {
        uint32_t previous = (uint32_t)InterlockedCompareExchange(
            (volatile LONG *)&server->learned_request_payload_bytes,
            (LONG)grown, (LONG)current);
        if (previous == current)
            break;
        current = previous;
    }
#else
    uint32_t current = __atomic_load_n(&server->learned_request_payload_bytes,
                                       __ATOMIC_RELAXED);
    while (grown > current &&
           !__atomic_compare_exchange_n(&server->learned_request_payload_bytes,
                                        &current, grown, false,
                                        __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
    }
#endif
}

void nipc_service_common_server_note_response_capacity(nipc_managed_server_t *server,
                                                       uint32_t payload_len)
{
    uint32_t grown = nipc_service_common_next_power_of_2_u32(payload_len);
#if defined(_WIN32) || defined(__MSYS__)
    uint32_t current = server->learned_response_payload_bytes;
    while (grown > current) {
        uint32_t previous = (uint32_t)InterlockedCompareExchange(
            (volatile LONG *)&server->learned_response_payload_bytes,
            (LONG)grown, (LONG)current);
        if (previous == current)
            break;
        current = previous;
    }
#else
    uint32_t current = __atomic_load_n(&server->learned_response_payload_bytes,
                                       __ATOMIC_RELAXED);
    while (grown > current &&
           !__atomic_compare_exchange_n(&server->learned_response_payload_bytes,
                                        &current, grown, false,
                                        __ATOMIC_RELEASE, __ATOMIC_RELAXED)) {
    }
#endif
}

void nipc_service_common_prepare_response_header(const nipc_header_t *request_hdr,
                                                 nipc_header_t *resp_hdr)
{
    memset(resp_hdr, 0, sizeof(*resp_hdr));
    resp_hdr->kind = NIPC_KIND_RESPONSE;
    resp_hdr->code = request_hdr->code;
    resp_hdr->message_id = request_hdr->message_id;
    if ((request_hdr->flags & NIPC_FLAG_BATCH) && request_hdr->item_count >= 1) {
        resp_hdr->item_count = request_hdr->item_count;
        resp_hdr->flags = NIPC_FLAG_BATCH;
    } else {
        resp_hdr->item_count = 1;
        resp_hdr->flags = 0;
    }
}

void nipc_service_common_apply_dispatch_result(nipc_managed_server_t *server,
                                               nipc_error_t dispatch_err,
                                               size_t resp_buf_size,
                                               uint32_t max_response_payload_bytes,
                                               bool check_header_room,
                                               nipc_header_t *resp_hdr,
                                               size_t *response_len,
                                               bool *close_after_response)
{
    *close_after_response = false;

    switch (dispatch_err) {
    case NIPC_OK:
        if (*response_len > resp_buf_size ||
            *response_len > max_response_payload_bytes ||
            (check_header_room && *response_len > SIZE_MAX - NIPC_HEADER_LEN)) {
            nipc_service_common_server_note_response_capacity(
                server, *response_len >= UINT32_MAX ? UINT32_MAX : (uint32_t)*response_len);
            resp_hdr->transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
            *close_after_response = true;
            *response_len = 0;
        } else {
            if (*response_len <= UINT32_MAX)
                nipc_service_common_server_note_response_capacity(
                    server, (uint32_t)*response_len);
            resp_hdr->transport_status = NIPC_STATUS_OK;
        }
        break;
    case NIPC_ERR_OVERFLOW:
        if (max_response_payload_bytes >= UINT32_MAX / 2u)
            nipc_service_common_server_note_response_capacity(server, UINT32_MAX);
        else
            nipc_service_common_server_note_response_capacity(
                server, max_response_payload_bytes * 2u);
        resp_hdr->transport_status = NIPC_STATUS_LIMIT_EXCEEDED;
        *close_after_response = true;
        *response_len = 0;
        break;
    case NIPC_ERR_TRUNCATED:
    case NIPC_ERR_BAD_LAYOUT:
    case NIPC_ERR_OUT_OF_BOUNDS:
    case NIPC_ERR_MISSING_NUL:
    case NIPC_ERR_BAD_ALIGNMENT:
    case NIPC_ERR_BAD_ITEM_COUNT:
        resp_hdr->transport_status = NIPC_STATUS_BAD_ENVELOPE;
        *close_after_response = true;
        *response_len = 0;
        break;
    case NIPC_ERR_HANDLER_FAILED:
    default:
        resp_hdr->transport_status = NIPC_STATUS_INTERNAL_ERROR;
        *close_after_response = true;
        *response_len = 0;
        break;
    }
}
