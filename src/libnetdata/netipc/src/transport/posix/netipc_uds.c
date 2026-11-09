/*
 * netipc_uds.c - shared low-level helpers for the POSIX UDS transport.
 */

#include "netipc_uds_internal.h"

#include <errno.h>
#include <limits.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/uio.h>

bool nipc_uds_header_payload_len(size_t payload_len, size_t *msg_len_out)
{
    if (payload_len > SIZE_MAX - NIPC_HEADER_LEN)
        return false;

    size_t msg_len = NIPC_HEADER_LEN + payload_len;
    if (msg_len > UINT32_MAX)
        return false;

    *msg_len_out = msg_len;
    return true;
}

uint32_t nipc_uds_packet_size(int fd, uint32_t configured)
{
#if NIPC_UDS_STREAM
    /* A stream has no per-message size cap; the fixed cap keeps the first
     * packet's buffer bounded and the chunking code shared. */
    (void)fd;
    uint32_t limit = NIPC_UDS_PACKET_SIZE_MAX;
#else
    /* Linux rejects a datagram larger than SO_SNDBUF minus 32 bytes of
     * accounting overhead with EMSGSIZE; very large buffers also hit
     * ENOBUFS near 1 MiB, hence the fixed cap. */
    uint32_t limit = NIPC_UDS_PACKET_SIZE_FALLBACK;
    int val = 0;
    socklen_t len = sizeof(val);
    if (getsockopt(fd, SOL_SOCKET, SO_SNDBUF, &val, &len) == 0 &&
        val > NIPC_UDS_SNDBUF_OVERHEAD)
        limit = (uint32_t)val - NIPC_UDS_SNDBUF_OVERHEAD;
#endif

    if (limit > NIPC_UDS_PACKET_SIZE_MAX)
        limit = NIPC_UDS_PACKET_SIZE_MAX;
    if (configured != 0 && configured < limit)
        return configured;
    return limit;
}

/* SOCK_SEQPACKET sends and receives whole messages or nothing, so a call a
 * signal interrupted (EINTR) moved no data and is retried. */
nipc_uds_error_t nipc_uds_raw_send(int fd, const void *data, size_t len)
{
#if NIPC_UDS_STREAM
    return nipc_uds_stream_send_iov(fd, data, len, NULL, 0);
#else
    ssize_t n;
    do {
        n = send(fd, data, len, NIPC_UDS_SEND_FLAGS);
    } while (n < 0 && errno == EINTR);
    if (n < 0 || (size_t)n != len)
        return NIPC_UDS_ERR_SEND;
    return NIPC_UDS_OK;
#endif
}

nipc_uds_error_t nipc_uds_raw_send_iov(int fd, const void *hdr, size_t hdr_len,
                                       const void *payload, size_t payload_len)
{
#if NIPC_UDS_STREAM
    return nipc_uds_stream_send_iov(fd, hdr, hdr_len, payload, payload_len);
#else
    struct iovec iov[2];
    struct msghdr msg;
    int iovcnt = 0;

    memset(&msg, 0, sizeof(msg));

    iov[0].iov_base = (void *)hdr;
    iov[0].iov_len = hdr_len;
    iovcnt = 1;

    if (payload && payload_len > 0) {
        iov[1].iov_base = (void *)payload;
        iov[1].iov_len = payload_len;
        iovcnt = 2;
    }

    msg.msg_iov = iov;
    msg.msg_iovlen = iovcnt;

    size_t total = hdr_len + payload_len;
    ssize_t n;
    do {
        n = sendmsg(fd, &msg, NIPC_UDS_SEND_FLAGS);
    } while (n < 0 && errno == EINTR);
    if (n < 0 || (size_t)n != total)
        return NIPC_UDS_ERR_SEND;

    return NIPC_UDS_OK;
#endif
}

ssize_t nipc_uds_raw_recv(int fd, void *buf, size_t buf_len)
{
    ssize_t n;
    do {
        n = recv(fd, buf, buf_len, 0);
    } while (n < 0 && errno == EINTR);
    return n;
}

/* ------------------------------------------------------------------ */
/*  Stream-socket packet I/O                                           */
/* ------------------------------------------------------------------ */

nipc_uds_error_t nipc_uds_stream_read_exact(int fd, void *dst, size_t len,
                                           const nipc_uds_wait_t *wait,
                                           bool mid_packet, size_t *got_out)
{
    uint8_t *p = dst;
    size_t got = 0;
    nipc_uds_error_t err = NIPC_UDS_OK;
    while (got < len) {
        if (wait) {
            err = nipc_uds_wait_for_readable(fd, wait);
            if (err != NIPC_UDS_OK)
                break;
        }
        ssize_t n = nipc_uds_raw_recv(fd, p + got, len - got);
        if (n == 0) {
            err = (mid_packet || got > 0) ? NIPC_UDS_ERR_PROTOCOL
                                          : NIPC_UDS_ERR_RECV;
            break;
        }
        if (n < 0) {
            err = NIPC_UDS_ERR_RECV;
            break;
        }
        got += (size_t)n;
    }
    if (got_out)
        *got_out = got;
    return err;
}

size_t nipc_uds_stream_body_len(const void *hdr, size_t buf_len,
                                uint32_t packet_size)
{
    /* The header fields are read raw; the receive path validates them. */
    size_t declared = 0;
    uint32_t magic;
    memcpy(&magic, hdr, sizeof(magic));
    if (magic == NIPC_MAGIC_MSG) {
        nipc_header_t h;
        memcpy(&h, hdr, NIPC_HEADER_LEN);
        size_t cap = packet_size > NIPC_HEADER_LEN
            ? (size_t)packet_size - NIPC_HEADER_LEN
            : buf_len - NIPC_HEADER_LEN;
        declared = h.payload_len < cap ? h.payload_len : cap;
    } else if (magic == NIPC_MAGIC_CHUNK) {
        nipc_chunk_header_t chk;
        memcpy(&chk, hdr, NIPC_HEADER_LEN);
        declared = chk.chunk_payload_len;
    }

    size_t room = buf_len - NIPC_HEADER_LEN;
    return declared < room ? declared : room;
}

nipc_uds_error_t nipc_uds_stream_recv_packet(int fd, void *buf, size_t buf_len,
                                             uint32_t packet_size,
                                             const nipc_uds_wait_t *wait,
                                             size_t *len_out)
{
    *len_out = 0;
    if (buf_len < NIPC_HEADER_LEN)
        return NIPC_UDS_ERR_BAD_PARAM;

    uint8_t *p = buf;
    nipc_uds_error_t err = nipc_uds_stream_read_exact(fd, p, NIPC_HEADER_LEN,
                                                      wait, false, NULL);
    if (err != NIPC_UDS_OK)
        return err;

    size_t body = nipc_uds_stream_body_len(p, buf_len, packet_size);
    err = nipc_uds_stream_read_exact(fd, p + NIPC_HEADER_LEN, body, wait, true,
                                     NULL);
    if (err != NIPC_UDS_OK)
        return err;

    *len_out = NIPC_HEADER_LEN + body;
    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_stream_send_iov(int fd, const void *hdr,
                                          size_t hdr_len, const void *payload,
                                          size_t payload_len)
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
    while (iovcnt > 0) {
        struct msghdr msg;
        memset(&msg, 0, sizeof(msg));
        msg.msg_iov = cur;
        msg.msg_iovlen = iovcnt;

        /* A stream write may move part of the data; EINTR means none moved. */
        ssize_t n = sendmsg(fd, &msg, NIPC_UDS_SEND_FLAGS);
        if (n < 0) {
            if (errno == EINTR)
                continue;
            return NIPC_UDS_ERR_SEND;
        }
        if (n == 0)
            return NIPC_UDS_ERR_SEND;

        size_t left = (size_t)n;
        while (iovcnt > 0 && left >= cur->iov_len) {
            left -= cur->iov_len;
            cur++;
            iovcnt--;
        }
        if (iovcnt > 0) {
            cur->iov_base = (uint8_t *)cur->iov_base + left;
            cur->iov_len -= left;
        }
    }
    return NIPC_UDS_OK;
}

/* ------------------------------------------------------------------ */
/*  Session pending queue                                              */
/* ------------------------------------------------------------------ */

/* Makes room for need more bytes after pending_tail. */
static nipc_uds_error_t pending_reserve(nipc_uds_session_t *s, size_t need)
{
    if (s->pending_head > 0) {
        size_t queued = s->pending_tail - s->pending_head;
        memmove(s->pending_buf, s->pending_buf + s->pending_head, queued);
        s->pending_head = 0;
        s->pending_tail = queued;
    }
    if (s->pending_cap - s->pending_tail >= need)
        return NIPC_UDS_OK;
    if (need > SIZE_MAX - s->pending_tail)
        return NIPC_UDS_ERR_ALLOC;

    size_t cap = s->pending_cap ? s->pending_cap : 4096;
    while (cap < s->pending_tail + need)
        cap = cap > SIZE_MAX / 2 ? s->pending_tail + need : cap * 2;

    uint8_t *p = realloc(s->pending_buf, cap);
    if (!p)
        return NIPC_UDS_ERR_ALLOC;
    s->pending_buf = p;
    s->pending_cap = cap;
    return NIPC_UDS_OK;
}

nipc_uds_error_t nipc_uds_pending_drain(nipc_uds_session_t *s, size_t bound,
                                        bool *eof)
{
    for (;;) {
        size_t read_len = s->packet_size;
#if !NIPC_UDS_STREAM
        /* Each datagram is stored whole, so its later receive sees what a
         * direct receive sees: one longer than packet_size, a protocol
         * violation, is rejected there instead of being stored cut short.
         * MSG_TRUNC reports the full length on Linux 3.4 and later; older
         * kernels report at most the probe length. */
        uint8_t probe;
        ssize_t full = recv(s->fd, &probe, sizeof(probe),
                            MSG_DONTWAIT | MSG_PEEK | MSG_TRUNC);
        if (full < 0 && errno == EINTR)
            continue;
        if (full < 0 && (errno == EAGAIN || errno == EWOULDBLOCK))
            return NIPC_UDS_OK;
        if (full <= 0) {
            /* EOF or a socket error: the retried send reports it. */
            *eof = true;
            return NIPC_UDS_OK;
        }
        if ((size_t)full > bound)
            return NIPC_UDS_ERR_PROTOCOL;
        if ((size_t)full > read_len)
            read_len = (size_t)full;
#endif
        nipc_uds_error_t err = pending_reserve(
            s, NIPC_UDS_PENDING_RECORD_LEN + read_len);
        if (err != NIPC_UDS_OK)
            return err;

        uint8_t *dst = s->pending_buf + s->pending_tail +
                       NIPC_UDS_PENDING_RECORD_LEN;
        ssize_t n = recv(s->fd, dst, read_len, MSG_DONTWAIT);
        if (n < 0 && errno == EINTR)
            continue;
        if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK))
            return NIPC_UDS_OK;
        if (n <= 0) {
            /* EOF or a socket error: the retried send reports it. */
            *eof = true;
            return NIPC_UDS_OK;
        }

#if !NIPC_UDS_STREAM
        uint32_t rec = (uint32_t)n;
        memcpy(s->pending_buf + s->pending_tail, &rec, sizeof(rec));
#endif
        s->pending_tail += NIPC_UDS_PENDING_RECORD_LEN + (size_t)n;
        if (s->pending_tail - s->pending_head > bound)
            return NIPC_UDS_ERR_PROTOCOL;
    }
}

static void pending_consumed(nipc_uds_session_t *s)
{
    if (s->pending_head == s->pending_tail)
        s->pending_head = s->pending_tail = 0;
}

size_t nipc_uds_pending_pop_packet(nipc_uds_session_t *s, void *buf,
                                   size_t buf_len)
{
    uint32_t rec;
    memcpy(&rec, s->pending_buf + s->pending_head, sizeof(rec));
    s->pending_head += sizeof(rec);

    size_t n = rec < buf_len ? rec : buf_len;
    memcpy(buf, s->pending_buf + s->pending_head, n);
    s->pending_head += rec;
    pending_consumed(s);
    return n;
}

size_t nipc_uds_pending_take(nipc_uds_session_t *s, void *dst, size_t len)
{
    size_t queued = s->pending_tail - s->pending_head;
    size_t n = len < queued ? len : queued;
    if (n > 0) {
        memcpy(dst, s->pending_buf + s->pending_head, n);
        s->pending_head += n;
        pending_consumed(s);
    }
    return n;
}

void nipc_uds_pending_free(nipc_uds_session_t *s)
{
    free(s->pending_buf);
    s->pending_buf = NULL;
    s->pending_cap = 0;
    s->pending_head = 0;
    s->pending_tail = 0;
}
