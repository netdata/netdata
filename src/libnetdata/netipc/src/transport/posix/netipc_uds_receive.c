#include "netipc_uds_internal.h"

#include <stdlib.h>
#include <string.h>

static nipc_uds_error_t ensure_recv_buf(nipc_uds_session_t *session,
                                        size_t needed)
{
    if (session->recv_buf_size >= needed)
        return NIPC_UDS_OK;

    uint8_t *p = realloc(session->recv_buf, needed);
    if (!p)
        return NIPC_UDS_ERR_ALLOC;

    session->recv_buf = p;
    session->recv_buf_size = needed;
    return NIPC_UDS_OK;
}

static nipc_uds_error_t validate_batch(const nipc_header_t *hdr,
                                       const void *payload,
                                       size_t payload_len)
{
    if (!(hdr->flags & NIPC_FLAG_BATCH) || hdr->item_count <= 1)
        return NIPC_UDS_OK;

    uint32_t dir_bytes = hdr->item_count * 8;
    uint32_t dir_aligned = (uint32_t)nipc_align8(dir_bytes);
    if (payload_len < dir_aligned)
        return NIPC_UDS_ERR_PROTOCOL;

    uint32_t packed_area_len = (uint32_t)(payload_len - dir_aligned);
    nipc_error_t perr = nipc_batch_dir_validate(payload, dir_bytes,
                                                hdr->item_count,
                                                packed_area_len);
    return (perr == NIPC_OK) ? NIPC_UDS_OK : NIPC_UDS_ERR_PROTOCOL;
}

static nipc_uds_error_t validate_inbound_limits(
    const nipc_uds_session_t *session,
    const nipc_header_t *hdr)
{
    uint32_t max_payload = (session->role == NIPC_UDS_ROLE_SERVER)
        ? session->max_request_payload_bytes
        : session->max_response_payload_bytes;
    if (hdr->payload_len > max_payload)
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;

    uint32_t max_batch = (session->role == NIPC_UDS_ROLE_SERVER)
        ? session->max_request_batch_items
        : session->max_response_batch_items;
    if (hdr->item_count > max_batch)
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;

    return NIPC_UDS_OK;
}

static nipc_uds_error_t track_inbound_response(nipc_uds_session_t *session,
                                               const nipc_header_t *hdr)
{
    if (session->role != NIPC_UDS_ROLE_CLIENT ||
        hdr->kind != NIPC_KIND_RESPONSE)
        return NIPC_UDS_OK;

    if (nipc_uds_inflight_remove(session, hdr->message_id) < 0)
        return NIPC_UDS_ERR_UNKNOWN_MSG_ID;
    return NIPC_UDS_OK;
}

static nipc_uds_error_t receive_first_packet(nipc_uds_session_t *session,
                                             void *buf,
                                             size_t buf_size,
                                             nipc_header_t *hdr_out,
                                             ssize_t *received_out)
{
    ssize_t n = nipc_uds_raw_recv(session->fd, buf, buf_size);
    if (n <= 0) {
        nipc_uds_inflight_fail_all(session);
        return NIPC_UDS_ERR_RECV;
    }

    if ((size_t)n < NIPC_HEADER_LEN)
        return NIPC_UDS_ERR_PROTOCOL;

    nipc_error_t perr = nipc_header_decode(buf, (size_t)n, hdr_out);
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_PROTOCOL;

    *received_out = n;
    return NIPC_UDS_OK;
}

static nipc_uds_error_t return_complete_packet(void *buf,
                                               const nipc_header_t *hdr,
                                               const void **payload_out,
                                               size_t *payload_len_out)
{
    *payload_out = (const uint8_t *)buf + NIPC_HEADER_LEN;
    *payload_len_out = hdr->payload_len;
    return validate_batch(hdr, *payload_out, *payload_len_out);
}

static uint32_t expected_chunk_count(size_t payload_len,
                                     size_t first_payload_bytes,
                                     size_t chunk_payload_budget)
{
    size_t remaining_after_first = payload_len - first_payload_bytes;
    uint32_t expected_continuations = 0;
    if (remaining_after_first > 0 && chunk_payload_budget > 0) {
        expected_continuations = (uint32_t)(1 + ((remaining_after_first - 1)
                                            / chunk_payload_budget));
    }
    return 1 + expected_continuations;
}

static nipc_uds_error_t validate_chunk_header(const nipc_chunk_header_t *chk,
                                              const nipc_header_t *hdr,
                                              uint32_t chunk_index,
                                              uint32_t chunk_count,
                                              size_t total_msg)
{
    if (chk->message_id != hdr->message_id ||
        chk->chunk_index != chunk_index ||
        chk->chunk_count != chunk_count ||
        chk->total_message_len != (uint32_t)total_msg)
        return NIPC_UDS_ERR_CHUNK;
    return NIPC_UDS_OK;
}

static nipc_uds_error_t receive_one_chunk(nipc_uds_session_t *session,
                                          uint8_t *pkt_buf,
                                          size_t pkt_buf_size,
                                          const nipc_header_t *hdr,
                                          uint32_t chunk_index,
                                          uint32_t chunk_count,
                                          size_t total_msg,
                                          size_t *assembled)
{
    ssize_t cn = nipc_uds_raw_recv(session->fd, pkt_buf, pkt_buf_size);
    if (cn <= 0) {
        nipc_uds_inflight_fail_all(session);
        return NIPC_UDS_ERR_RECV;
    }

    if ((size_t)cn < NIPC_HEADER_LEN)
        return NIPC_UDS_ERR_CHUNK;

    nipc_chunk_header_t chk;
    nipc_error_t perr = nipc_chunk_header_decode(pkt_buf, (size_t)cn, &chk);
    if (perr != NIPC_OK)
        return NIPC_UDS_ERR_CHUNK;

    nipc_uds_error_t err = validate_chunk_header(
        &chk, hdr, chunk_index, chunk_count, total_msg);
    if (err != NIPC_UDS_OK)
        return err;

    size_t chunk_data = (size_t)cn - NIPC_HEADER_LEN;
    if (chunk_data != chk.chunk_payload_len)
        return NIPC_UDS_ERR_CHUNK;

    if (*assembled + chunk_data > hdr->payload_len)
        return NIPC_UDS_ERR_CHUNK;

    memcpy(session->recv_buf + *assembled, pkt_buf + NIPC_HEADER_LEN,
           chunk_data);
    *assembled += chunk_data;
    return NIPC_UDS_OK;
}

static nipc_uds_error_t receive_chunked_payload(nipc_uds_session_t *session,
                                                void *buf,
                                                ssize_t first_packet_len,
                                                const nipc_header_t *hdr,
                                                size_t total_msg,
                                                const void **payload_out,
                                                size_t *payload_len_out)
{
    size_t first_payload_bytes = (size_t)first_packet_len - NIPC_HEADER_LEN;
    nipc_uds_error_t err = ensure_recv_buf(session, hdr->payload_len);
    if (err != NIPC_UDS_OK)
        return err;

    memcpy(session->recv_buf, (uint8_t *)buf + NIPC_HEADER_LEN,
           first_payload_bytes);

    size_t assembled = first_payload_bytes;
    size_t chunk_payload_budget = session->packet_size - NIPC_HEADER_LEN;
    uint32_t chunk_count = expected_chunk_count(
        hdr->payload_len, first_payload_bytes, chunk_payload_budget);

    size_t pkt_buf_size = session->packet_size;
    uint8_t *pkt_buf = malloc(pkt_buf_size);
    if (!pkt_buf)
        return NIPC_UDS_ERR_ALLOC;

    for (uint32_t ci = 1; assembled < hdr->payload_len; ci++) {
        err = receive_one_chunk(session, pkt_buf, pkt_buf_size, hdr, ci,
                                chunk_count, total_msg, &assembled);
        if (err != NIPC_UDS_OK) {
            free(pkt_buf);
            return err;
        }
    }

    free(pkt_buf);

    *payload_out = session->recv_buf;
    *payload_len_out = hdr->payload_len;
    return validate_batch(hdr, *payload_out, *payload_len_out);
}

nipc_uds_error_t nipc_uds_receive(nipc_uds_session_t *session,
                                   void *buf, size_t buf_size,
                                   nipc_header_t *hdr_out,
                                   const void **payload_out,
                                   size_t *payload_len_out)
{
    if (!session || session->fd < 0)
        return NIPC_UDS_ERR_BAD_PARAM;

    ssize_t n;
    nipc_uds_error_t err = receive_first_packet(session, buf, buf_size,
                                                hdr_out, &n);
    if (err != NIPC_UDS_OK)
        return err;

    err = validate_inbound_limits(session, hdr_out);
    if (err != NIPC_UDS_OK)
        return err;

    err = track_inbound_response(session, hdr_out);
    if (err != NIPC_UDS_OK)
        return err;

    size_t total_msg;
    if (!nipc_uds_header_payload_len(hdr_out->payload_len, &total_msg))
        return NIPC_UDS_ERR_LIMIT_EXCEEDED;

    if ((size_t)n >= total_msg) {
        return return_complete_packet(buf, hdr_out, payload_out,
                                      payload_len_out);
    }

    return receive_chunked_payload(session, buf, n, hdr_out, total_msg,
                                   payload_out, payload_len_out);
}
