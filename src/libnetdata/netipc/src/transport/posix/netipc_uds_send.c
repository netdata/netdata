#include "netipc_uds_internal.h"

#include <assert.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/uio.h>

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

/* Wire bytes the in-flight responses can occupy: the queue bound. A bounded
 * send of an untracked message can run with nothing in flight; it still
 * gets one response's room, as the peer may be mid-response. */
static size_t pending_bound(const nipc_uds_session_t *session)
{
    size_t inflight = session->inflight_count ? session->inflight_count : 1;
    size_t payload = session->max_response_payload_bytes;
    size_t budget = session->packet_size > NIPC_HEADER_LEN
        ? (size_t)session->packet_size - NIPC_HEADER_LEN : 1;
    size_t packets = payload == 0 ? 1 : 1 + (payload - 1) / budget;
    size_t per_packet = NIPC_HEADER_LEN + NIPC_UDS_PENDING_RECORD_LEN;
    if (packets > (SIZE_MAX - payload) / per_packet)
        return SIZE_MAX;
    size_t per_response = payload + packets * per_packet;
    if (per_response > SIZE_MAX / inflight)
        return SIZE_MAX;
    return per_response * inflight;
}

/* A failure inside a progress send leaves a partial message on the wire or
 * inbound bytes dropped: the session cannot continue. */
static nipc_uds_error_t fail_session(nipc_uds_session_t *session,
                                     nipc_uds_error_t err)
{
    (void)shutdown(session->fd, SHUT_RDWR);
    nipc_uds_inflight_fail_all(session);
    return err;
}

/* Sends one packet without blocking; while the socket is full, reads what
 * the peer sent into the pending queue, so both peers keep making progress.
 * Without a wait it waits as long as a blocking send would; with one it ends
 * at the deadline or abort and breaks the session, since part of the message
 * may already be on the wire. */
static nipc_uds_error_t send_with_progress_loop(nipc_uds_session_t *session,
                                           const void *hdr, size_t hdr_len,
                                           const void *payload,
                                           size_t payload_len,
                                           const nipc_uds_wait_t *wait)
{
    struct iovec iov[2];
    int iovcnt = 1;
    iov[0].iov_base = (void *)hdr;
    iov[0].iov_len = hdr_len;
    if (payload && payload_len > 0) {
        iov[1].iov_base = (void *)payload;
        iov[1].iov_len = payload_len;
        iovcnt = 2;
    }
    struct iovec *cur = iov;
    size_t bound = pending_bound(session);
    /* After a failed stream receive the inbound bytes belong to an
     * abandoned message: never queue them. */
    bool eof = session->recv_failed;

    for (;;) {
        struct msghdr msg;
        memset(&msg, 0, sizeof(msg));
        msg.msg_iov = cur;
        msg.msg_iovlen = iovcnt;

        ssize_t n = sendmsg(session->fd, &msg,
                            NIPC_UDS_SEND_FLAGS | MSG_DONTWAIT);
        if (n >= 0) {
#if NIPC_UDS_STREAM
            if (n == 0)
                return NIPC_UDS_ERR_SEND;
            size_t left = (size_t)n;
            while (iovcnt > 0 && left >= cur->iov_len) {
                left -= cur->iov_len;
                cur++;
                iovcnt--;
            }
            if (iovcnt == 0)
                return NIPC_UDS_OK;
            cur->iov_base = (uint8_t *)cur->iov_base + left;
            cur->iov_len -= left;
            continue;
#else
            return (size_t)n == hdr_len + payload_len ? NIPC_UDS_OK
                                                      : NIPC_UDS_ERR_SEND;
#endif
        }
        if (errno == EINTR)
            continue;
        if (errno != EAGAIN && errno != EWOULDBLOCK)
            return NIPC_UDS_ERR_SEND;

        /* After EOF only writability matters; the retried send reports a
         * closed peer. */
        struct pollfd fds[2] = {
            {
                .fd = session->fd,
                .events = (short)(POLLOUT | (eof ? 0 : POLLIN)),
                .revents = 0,
            },
            {.fd = -1, .events = POLLIN, .revents = 0},
        };
        nfds_t nfds = 1;
        if (wait && wait->abort_fd >= 0) {
            fds[1].fd = wait->abort_fd;
            nfds = 2;
        }
        int timeout = wait ? nipc_uds_wait_timeout(wait) : -1;
        if (timeout == 0)
            return fail_session(session, NIPC_UDS_ERR_TIMEOUT);
        int rc = poll(fds, nfds, timeout);
        if (rc < 0) {
            if (errno == EINTR)
                continue;
            return NIPC_UDS_ERR_SEND;
        }
        if (rc == 0)
            continue; /* nipc_uds_wait_timeout() reports the deadline */
        if (nfds == 2 && (fds[1].revents & (POLLIN | POLLHUP | POLLERR)))
            return fail_session(session, NIPC_UDS_ERR_ABORTED);
        if (!eof && (fds[0].revents & (POLLIN | POLLHUP | POLLERR))) {
            nipc_uds_error_t err = nipc_uds_pending_drain(session, bound, &eof);
            if (err != NIPC_UDS_OK)
                return fail_session(session, err);
        }
    }
}

/* macOS ignores MSG_DONTWAIT on a stream send that lacks buffer space and
 * blocks, so on the stream baseline (macOS, FreeBSD) the descriptor is made
 * non-blocking for the send instead. */
static nipc_uds_error_t send_with_progress(nipc_uds_session_t *session,
                                           const void *hdr, size_t hdr_len,
                                           const void *payload,
                                           size_t payload_len,
                                           const nipc_uds_wait_t *wait)
{
#if NIPC_UDS_STREAM
    int flags = fcntl(session->fd, F_GETFL);
    if (flags < 0 || fcntl(session->fd, F_SETFL, flags | O_NONBLOCK) < 0)
        return NIPC_UDS_ERR_SEND;
    nipc_uds_error_t err = send_with_progress_loop(session, hdr, hdr_len,
                                                   payload, payload_len, wait);
    /* A descriptor left non-blocking would break later blocking calls. */
    if (fcntl(session->fd, F_SETFL, flags) < 0)
        return fail_session(session, NIPC_UDS_ERR_SEND);
    return err;
#else
    return send_with_progress_loop(session, hdr, hdr_len, payload,
                                   payload_len, wait);
#endif
}

/* A client send with a deadline or abort, or with another request in
 * flight (a pipelining client can deadlock with a server blocked sending),
 * waits in the progress loop; anything else keeps the plain blocking send. */
static nipc_uds_error_t session_send_packet(nipc_uds_session_t *session,
                                            const void *hdr, size_t hdr_len,
                                            const void *payload,
                                            size_t payload_len,
                                            const nipc_uds_wait_t *wait)
{
    if (session->role == NIPC_UDS_ROLE_CLIENT &&
        (wait || session->inflight_count > 1))
        return send_with_progress(session, hdr, hdr_len, payload,
                                  payload_len, wait);
    return nipc_uds_raw_send_iov(session->fd, hdr, hdr_len, payload,
                                 payload_len);
}

static nipc_uds_error_t send_single_packet(nipc_uds_session_t *session,
                                           nipc_header_t *hdr,
                                           const void *payload,
                                           size_t payload_len,
                                           bool tracked,
                                           const nipc_uds_wait_t *wait)
{
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

    nipc_uds_error_t err = session_send_packet(
        session, hdr_buf, NIPC_HEADER_LEN, payload, payload_len, wait);
    update_inflight_after_send(session, hdr, tracked, err);
    return err;
}

static nipc_uds_error_t send_first_chunk(nipc_uds_session_t *session,
                                         nipc_header_t *hdr,
                                         const void *payload,
                                         size_t first_chunk_payload,
                                         bool tracked,
                                         const nipc_uds_wait_t *wait)
{
    uint8_t hdr_buf[NIPC_HEADER_LEN];
    nipc_header_encode(hdr, hdr_buf, sizeof(hdr_buf));

    nipc_uds_error_t err = session_send_packet(
        session, hdr_buf, NIPC_HEADER_LEN, payload, first_chunk_payload,
        wait);
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
                                                bool tracked,
                                                const nipc_uds_wait_t *wait)
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

    nipc_uds_error_t err = session_send_packet(
        session, chk_buf, NIPC_HEADER_LEN, src, this_chunk, wait);
    update_inflight_after_send(session, hdr, tracked, err);
    return err;
}

static nipc_uds_error_t send_chunked(nipc_uds_session_t *session,
                                     nipc_header_t *hdr,
                                     const void *payload,
                                     size_t payload_len,
                                     size_t total_msg,
                                     bool tracked,
                                     const nipc_uds_wait_t *wait)
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
                                            first_chunk_payload, tracked,
                                            wait);
    if (err != NIPC_UDS_OK)
        return err;

    const uint8_t *src = (const uint8_t *)payload + first_chunk_payload;
    remaining = payload_len - first_chunk_payload;

    for (uint32_t ci = 1; ci < chunk_count; ci++) {
        size_t this_chunk = min_of_size_t(remaining, chunk_payload_budget);

        err = send_continuation_chunk(session, hdr, src, this_chunk, ci,
                                      chunk_count, total_msg_u32,
                                      tracked, wait);
        if (err != NIPC_UDS_OK)
            return err;

        src += this_chunk;
        remaining -= this_chunk;
    }

    return NIPC_UDS_OK;
}

static nipc_uds_error_t uds_send(nipc_uds_session_t *session,
                                 nipc_header_t *hdr,
                                 const void *payload,
                                 size_t payload_len,
                                 const nipc_uds_wait_t *wait)
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
                                  tracked, wait);
    }

    return send_chunked(session, hdr, payload, payload_len, total_msg,
                        tracked, wait);
}

nipc_uds_error_t nipc_uds_send(nipc_uds_session_t *session,
                                nipc_header_t *hdr,
                                const void *payload,
                                size_t payload_len)
{
    return uds_send(session, hdr, payload, payload_len, NULL);
}

nipc_uds_error_t nipc_uds_send_timeout(nipc_uds_session_t *session,
                                        nipc_header_t *hdr,
                                        const void *payload,
                                        size_t payload_len,
                                        uint32_t timeout_ms,
                                        int abort_fd)
{
    if (timeout_ms == 0 && abort_fd < 0)
        return uds_send(session, hdr, payload, payload_len, NULL);
    nipc_uds_wait_t wait = nipc_uds_wait_init(timeout_ms, abort_fd);
    return uds_send(session, hdr, payload, payload_len, &wait);
}
