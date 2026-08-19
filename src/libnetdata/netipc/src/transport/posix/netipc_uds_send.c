#include "netipc_uds_internal.h"

#include <assert.h>
#include <string.h>

static size_t min_of_size_t(size_t a, size_t b)
{
    return a < b ? a : b;
}

static bool tracks_client_request(const nipc_uds_session_t *session,
                                  const nipc_header_t *hdr)
{
    return session->role == NIPC_UDS_ROLE_CLIENT &&
           hdr->kind == NIPC_KIND_REQUEST;
}

static void update_inflight_after_send(nipc_uds_session_t *session,
                                       const nipc_header_t *hdr,
                                       bool tracked,
                                       nipc_uds_error_t err)
{
    if (!tracked)
        return;

    if (err == NIPC_UDS_OK)
        return;

    if (err == NIPC_UDS_ERR_SEND)
        nipc_uds_inflight_fail_all(session);
    else
        nipc_uds_inflight_remove(session, hdr->message_id);
}

static nipc_uds_error_t track_outbound_request(nipc_uds_session_t *session,
                                               const nipc_header_t *hdr,
                                               bool tracked)
{
    if (!tracked)
        return NIPC_UDS_OK;

    int rc = nipc_uds_inflight_add(session, hdr->message_id);
    if (rc == -1)
        return NIPC_UDS_ERR_DUPLICATE_MSG_ID;
    if (rc == -2)
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;
    return NIPC_UDS_OK;
}

static void outbound_limits(const nipc_uds_session_t *session,
                            const nipc_header_t *hdr,
                            uint32_t *max_payload,
                            uint32_t *max_batch)
{
    *max_payload = 0;
    *max_batch = 0;

    if (session->role == NIPC_UDS_ROLE_CLIENT &&
        hdr->kind == NIPC_KIND_REQUEST) {
        *max_payload = session->max_request_payload_bytes;
        *max_batch = session->max_request_batch_items;
    } else if (session->role == NIPC_UDS_ROLE_SERVER &&
               hdr->kind == NIPC_KIND_RESPONSE) {
        *max_payload = session->max_response_payload_bytes;
        *max_batch = session->max_response_batch_items;
    }
}

static nipc_uds_error_t validate_outbound_limits(nipc_uds_session_t *session,
                                                 const nipc_header_t *hdr,
                                                 size_t payload_len,
                                                 bool tracked)
{
    uint32_t max_payload;
    uint32_t max_batch;
    outbound_limits(session, hdr, &max_payload, &max_batch);

    bool payload_fits_u32 = payload_len <= UINT32_MAX;
    bool payload_within_limit = max_payload == 0 || payload_len <= max_payload;
    bool batch_within_limit = max_batch == 0 || hdr->item_count <= max_batch;

    if (payload_fits_u32 && payload_within_limit && batch_within_limit)
        return NIPC_UDS_OK;

    if (tracked)
        nipc_uds_inflight_remove(session, hdr->message_id);
    return NIPC_UDS_ERR_LIMIT_EXCEEDED;
}

static void fill_envelope(nipc_header_t *hdr, size_t payload_len)
{
    hdr->magic = NIPC_MAGIC_MSG;
    hdr->version = NIPC_VERSION;
    hdr->header_len = NIPC_HEADER_LEN;
    /* validate_outbound_limits() rejects payloads that cannot fit the header. */
    assert(payload_len <= UINT32_MAX);
    hdr->payload_len = (uint32_t)payload_len;
}

static nipc_uds_error_t send_single_packet(nipc_uds_session_t *session,
                                           nipc_header_t *hdr,
                                           const void *payload,
                                           size_t payload_len,
                                           bool tracked)
{
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

    nipc_uds_error_t err = nipc_uds_raw_send_iov(
        session->fd, hdr_buf, NIPC_HEADER_LEN, payload, payload_len);
    update_inflight_after_send(session, hdr, tracked, err);
    return err;
}

static nipc_uds_error_t send_first_chunk(nipc_uds_session_t *session,
                                         nipc_header_t *hdr,
                                         const void *payload,
                                         size_t first_chunk_payload,
                                         bool tracked)
{
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

    nipc_uds_error_t err = nipc_uds_raw_send_iov(
        session->fd, hdr_buf, NIPC_HEADER_LEN, payload, first_chunk_payload);
    update_inflight_after_send(session, hdr, tracked, err);
    return err;
}

static nipc_uds_error_t send_continuation_chunk(nipc_uds_session_t *session,
                                                nipc_header_t *hdr,
                                                const uint8_t *src,
                                                size_t this_chunk,
                                                uint32_t chunk_index,
                                                uint32_t chunk_count,
                                                uint32_t total_msg,
                                                bool tracked)
{
    /* packet_size is negotiated as uint32_t, so continuation chunks fit u32. */
    assert(this_chunk <= UINT32_MAX);

    nipc_chunk_header_t chk = {
        .magic = NIPC_MAGIC_CHUNK,
        .version = NIPC_VERSION,
        .message_id = hdr->message_id,
        .total_message_len = total_msg,
        .chunk_index = chunk_index,
        .chunk_count = chunk_count,
        .chunk_payload_len = (uint32_t)this_chunk,
    };

    uint8_t chk_buf[NIPC_HEADER_LEN];
    nipc_chunk_header_encode(&chk, chk_buf, sizeof(chk_buf));

    nipc_uds_error_t err = nipc_uds_raw_send_iov(
        session->fd, chk_buf, NIPC_HEADER_LEN, src, this_chunk);
    update_inflight_after_send(session, hdr, tracked, err);
    return err;
}

static nipc_uds_error_t send_chunked(nipc_uds_session_t *session,
                                     nipc_header_t *hdr,
                                     const void *payload,
                                     size_t payload_len,
                                     size_t total_msg,
                                     bool tracked)
{
    if (session->packet_size <= NIPC_HEADER_LEN)
        return NIPC_UDS_ERR_BAD_PARAM;

    /* nipc_uds_header_payload_len() rejects totals wider than the wire field. */
    assert(total_msg <= UINT32_MAX);
    uint32_t total_msg_u32 = (uint32_t)total_msg;

    size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;
    size_t remaining = payload_len;
    size_t first_chunk_payload = min_of_size_t(remaining, chunk_payload_budget);
    remaining -= first_chunk_payload;

    uint32_t continuation_chunks = 0;
    if (remaining > 0) {
        continuation_chunks = (uint32_t)(1 + ((remaining - 1)
                                              / chunk_payload_budget));
    }
    uint32_t chunk_count = 1 + continuation_chunks;

    nipc_uds_error_t err = send_first_chunk(session, hdr, payload,
                                            first_chunk_payload, tracked);
    if (err != NIPC_UDS_OK)
        return err;

    const uint8_t *src = (const uint8_t *)payload + first_chunk_payload;
    remaining = payload_len - first_chunk_payload;

    for (uint32_t ci = 1; ci < chunk_count; ci++) {
        size_t this_chunk = min_of_size_t(remaining, chunk_payload_budget);

        err = send_continuation_chunk(session, hdr, src, this_chunk, ci,
                                      chunk_count, total_msg_u32,
                                      tracked);
        if (err != NIPC_UDS_OK)
            return err;

        src += this_chunk;
        remaining -= this_chunk;
    }

    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_send(nipc_uds_session_t *session,
                                nipc_header_t *hdr,
                                const void *payload,
                                size_t payload_len)
{
    if (!session || session->fd < 0)
        return NIPC_UDS_ERR_BAD_PARAM;

    bool tracked = tracks_client_request(session, hdr);
    nipc_uds_error_t err = track_outbound_request(session, hdr, tracked);
    if (err != NIPC_UDS_OK)
        return err;

    err = validate_outbound_limits(session, hdr, payload_len, tracked);
    if (err != NIPC_UDS_OK)
        return err;

    fill_envelope(hdr, payload_len);

    size_t total_msg;
    if (!nipc_uds_header_payload_len(payload_len, &total_msg)) {
        if (tracked)
            nipc_uds_inflight_remove(session, hdr->message_id);
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;
    }

    if (total_msg <= session->packet_size) {
        return send_single_packet(session, hdr, payload, payload_len,
                                  tracked);
    }

    return send_chunked(session, hdr, payload, payload_len, total_msg,
                        tracked);
}
