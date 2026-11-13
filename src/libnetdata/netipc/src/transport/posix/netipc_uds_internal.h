#ifndef NETIPC_UDS_INTERNAL_H
#define NETIPC_UDS_INTERNAL_H

#include "netipc/netipc_protocol.h"
#include "netipc/netipc_uds.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <sys/socket.h>
#include <sys/types.h>

#define UDS_DEFAULT_BACKLOG 16
#define UDS_DEFAULT_BATCH_ITEMS 1

#define NIPC_UDS_PACKET_SIZE_FALLBACK 65536u
#define NIPC_UDS_PACKET_SIZE_MAX 262144u
/* Bytes Linux keeps from SO_SNDBUF for a SEQPACKET datagram: it accepts
 * SO_SNDBUF - 32. */
#define NIPC_UDS_SNDBUF_OVERHEAD 32

/* macOS has no AF_UNIX SOCK_SEQPACKET, and FreeBSD 15 SEQPACKET keeps record
 * boundaries only where the sender sets MSG_EOR and can write part of a
 * record. Both use a stream socket carrying the same packets, each delimited
 * by the length its header declares (docs/level1-posix-uds.md, "Stream
 * baseline"). */
#if defined(__APPLE__) || defined(__FreeBSD__)
#define NIPC_UDS_STREAM 1
#define NIPC_UDS_SOCK_TYPE SOCK_STREAM
#else
#define NIPC_UDS_STREAM 0
#define NIPC_UDS_SOCK_TYPE SOCK_SEQPACKET
#endif

#ifdef MSG_NOSIGNAL
#define NIPC_UDS_SEND_FLAGS MSG_NOSIGNAL
#else
#define NIPC_UDS_SEND_FLAGS 0
#endif

/* Profiles this platform can provide; unassigned bits are never offered or
 * accepted. POSIX SHM needs a futex-style wait (Linux futex, FreeBSD
 * _umtx_op), so other POSIX systems offer and accept the baseline profile
 * only. */
#if defined(__linux__) || defined(__FreeBSD__)
#define NIPC_UDS_PLATFORM_PROFILES                                            \
    (NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX)
#else
#define NIPC_UDS_PLATFORM_PROFILES NIPC_PROFILE_BASELINE
#endif

bool nipc_uds_header_payload_len(size_t payload_len, size_t *msg_len_out);

/* Packet size this side may propose: the configured size (0 = no
 * preference) capped by what the kernel accepts as one datagram. */
uint32_t nipc_uds_packet_size(int fd, uint32_t configured);

nipc_uds_error_t nipc_uds_raw_send(int fd, const void *data, size_t len);
nipc_uds_error_t nipc_uds_raw_send_iov(int fd, const void *hdr, size_t hdr_len,
                                       const void *payload, size_t payload_len);
ssize_t nipc_uds_raw_recv(int fd, void *buf, size_t buf_len);

/* Wait until fd is readable or hung up. timeout_ms 0 waits without a
 * deadline; abort_fd -1 disables abort. */
nipc_uds_error_t nipc_uds_wait_readable(int fd, uint32_t timeout_ms, int abort_fd);

/* One deadline shared by every wait of one receive. */
typedef struct {
    bool infinite;
    uint64_t deadline_ms;
    int abort_fd;
} nipc_uds_wait_t;

nipc_uds_wait_t nipc_uds_wait_init(uint32_t timeout_ms, int abort_fd);
/* Poll timeout left: -1 without a deadline, 0 once it passed. */
int nipc_uds_wait_timeout(const nipc_uds_wait_t *wait);
nipc_uds_error_t nipc_uds_wait_for_readable(int fd, const nipc_uds_wait_t *wait);

/* Stream-socket packet I/O (used by sessions on macOS and FreeBSD; built
 * on every POSIX system so its tests run everywhere). A packet is the
 * 32-byte header plus the bytes it declares, so the bytes on the wire match
 * SEQPACKET.
 *
 * nipc_uds_stream_read_exact() reads len bytes, waiting before each read
 * when wait is set (NULL blocks). EOF before the first byte is RECV (the
 * peer disconnected between packets); EOF inside a packet (any byte read,
 * or mid_packet) is PROTOCOL (the peer sent a truncated packet). When
 * got_out is set, it receives the bytes read, also on failure. */
nipc_uds_error_t nipc_uds_stream_read_exact(int fd, void *dst, size_t len,
                                           const nipc_uds_wait_t *wait,
                                           bool mid_packet, size_t *got_out);
/* Bytes that follow a received 32-byte header: min(payload_len,
 * packet_size - 32) after an outer header, chunk_payload_len after a
 * continuation header, none after any other magic. packet_size 0
 * (handshake) caps an outer header at buf_len. Bytes beyond buf_len are
 * not counted, as a SEQPACKET recv into a short buffer drops them; the
 * caller's length checks then fail. */
size_t nipc_uds_stream_body_len(const void *hdr, size_t buf_len,
                                uint32_t packet_size);
/* Reads one packet (header, then its body); buf_len must be >= 32. */
nipc_uds_error_t nipc_uds_stream_recv_packet(int fd, void *buf, size_t buf_len,
                                             uint32_t packet_size,
                                             const nipc_uds_wait_t *wait,
                                             size_t *len_out);
/* Sends header + payload completely, resuming after partial writes. */
nipc_uds_error_t nipc_uds_stream_send_iov(int fd, const void *hdr,
                                          size_t hdr_len, const void *payload,
                                          size_t payload_len);

/* Session pending queue: inbound bytes a pipelined client send read while
 * it waited for buffer space. SEQPACKET stores each datagram as a 4-byte
 * length record followed by its bytes; the stream stores raw bytes. */
#if NIPC_UDS_STREAM
#define NIPC_UDS_PENDING_RECORD_LEN 0u
#else
#define NIPC_UDS_PENDING_RECORD_LEN 4u
#endif

static inline bool nipc_uds_pending_queued(const nipc_uds_session_t *s)
{
    return nipc_uds_session_has_pending(s);
}

/* Reads without blocking until the socket has nothing more to read, EOF,
 * or an error (both set *eof). Fails with PROTOCOL when the queued bytes
 * would exceed bound, or ALLOC. */
nipc_uds_error_t nipc_uds_pending_drain(nipc_uds_session_t *s, size_t bound,
                                        bool *eof);
/* SEQPACKET: pops one queued datagram into buf, truncating it to buf_len
 * like recv() does; returns the bytes copied. */
size_t nipc_uds_pending_pop_packet(nipc_uds_session_t *s, void *buf,
                                   size_t buf_len);
/* Stream: copies up to len queued bytes into dst; returns the bytes copied. */
size_t nipc_uds_pending_take(nipc_uds_session_t *s, void *dst, size_t len);
void nipc_uds_pending_free(nipc_uds_session_t *s);

int nipc_uds_build_socket_name(char *dst, size_t dst_len,
                               const char *service_name);
int nipc_uds_build_socket_path(char *dst, size_t dst_len,
                               const char *run_dir,
                               const char *service_name);
int nipc_uds_check_and_recover_stale(const char *run_dir,
                                     const char *socket_name,
                                     const char *path);

/* Waits at most timeout_ms for HELLO_ACK (0 = no deadline); abort_fd -1
 * disables abort. */
nipc_uds_error_t nipc_uds_client_handshake(int fd,
                                           const nipc_uds_client_config_t *cfg,
                                           uint32_t timeout_ms, int abort_fd,
                                           nipc_uds_session_t *session);
nipc_uds_error_t nipc_uds_server_handshake(int fd,
                                           const nipc_uds_server_config_t *cfg,
                                           uint64_t session_id,
                                           nipc_uds_profile_prepare_fn prepare,
                                           void *prepare_user,
                                           const nipc_uds_wait_t *wait,
                                           nipc_uds_session_t *session);

int nipc_uds_inflight_add(nipc_uds_session_t *s, uint64_t id);
int nipc_uds_inflight_remove(nipc_uds_session_t *s, uint64_t id);
void nipc_uds_inflight_fail_all(nipc_uds_session_t *s);

#endif /* NETIPC_UDS_INTERNAL_H */
