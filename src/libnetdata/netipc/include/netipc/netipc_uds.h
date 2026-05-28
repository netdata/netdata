/*
 * netipc_uds.h - L1 POSIX UDS SEQPACKET transport.
 *
 * Connection lifecycle, handshake, send/receive with transparent chunking.
 * Uses the wire envelope from netipc_protocol.h for all framing.
 */

#ifndef NETIPC_UDS_H
#define NETIPC_UDS_H

#include "netipc_protocol.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Error codes (transport-level)                                      */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_UDS_OK = 0,
    NIPC_UDS_ERR_PATH_TOO_LONG,     /* socket path exceeds sun_path */
    NIPC_UDS_ERR_SOCKET,            /* socket()/bind()/listen() failed */
    NIPC_UDS_ERR_CONNECT,           /* connect() failed */
    NIPC_UDS_ERR_ACCEPT,            /* accept() failed */
    NIPC_UDS_ERR_SEND,              /* send() failed */
    NIPC_UDS_ERR_RECV,              /* recv() failed / peer disconnected */
    NIPC_UDS_ERR_HANDSHAKE,         /* handshake protocol error */
    NIPC_UDS_ERR_AUTH_FAILED,       /* auth token rejected */
    NIPC_UDS_ERR_NO_PROFILE,        /* no common profile */
    NIPC_UDS_ERR_INCOMPATIBLE,      /* protocol/layout version mismatch */
    NIPC_UDS_ERR_PROTOCOL,          /* wire protocol violation */
    NIPC_UDS_ERR_ADDR_IN_USE,       /* live server on the socket path */
    NIPC_UDS_ERR_CHUNK,             /* chunk header mismatch */
    NIPC_UDS_ERR_ALLOC,             /* memory allocation failed */
    NIPC_UDS_ERR_LIMIT_EXCEEDED,    /* payload/batch exceeds negotiated */
    NIPC_UDS_ERR_BAD_PARAM,         /* invalid argument */
    NIPC_UDS_ERR_DUPLICATE_MSG_ID,  /* message_id already in-flight */
    NIPC_UDS_ERR_UNKNOWN_MSG_ID,    /* response message_id not in-flight */
} nipc_uds_error_t;

/* ------------------------------------------------------------------ */
/*  Role                                                               */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_UDS_ROLE_CLIENT = 1,
    NIPC_UDS_ROLE_SERVER = 2,
} nipc_uds_role_t;

/* ------------------------------------------------------------------ */
/*  Client connect configuration                                       */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t supported_profiles;        /* bitmask */
    uint32_t preferred_profiles;        /* bitmask */
    uint32_t max_request_payload_bytes; /* 0 = use default */
    uint32_t max_request_batch_items;   /* 0 = use default (1) */
    uint32_t max_response_payload_bytes;/* 0 = use default */
    uint32_t max_response_batch_items;  /* 0 = use default (1) */
    uint64_t auth_token;
    uint32_t packet_size;              /* 0 = auto-detect from SO_SNDBUF */
} nipc_uds_client_config_t;

/* ------------------------------------------------------------------ */
/*  Server configuration (for listen + accept)                         */
/* ------------------------------------------------------------------ */

typedef struct {
    uint32_t supported_profiles;
    uint32_t preferred_profiles;
    uint32_t max_request_payload_bytes;  /* 0 = use default */
    uint32_t max_request_batch_items;    /* 0 = use default (1) */
    uint32_t max_response_payload_bytes; /* 0 = use default */
    uint32_t max_response_batch_items;   /* 0 = use default (1) */
    uint64_t auth_token;                 /* expected token from clients */
    uint32_t packet_size;               /* 0 = auto-detect from SO_SNDBUF */
    int      backlog;                   /* listen backlog, 0 = default (16) */
} nipc_uds_server_config_t;

/* ------------------------------------------------------------------ */
/*  Session                                                            */
/* ------------------------------------------------------------------ */

typedef struct {
    int             fd;             /* connected socket (native wait object) */
    nipc_uds_role_t role;

    /* Negotiated limits */
    uint32_t max_request_payload_bytes;
    uint32_t max_request_batch_items;
    uint32_t max_response_payload_bytes;
    uint32_t max_response_batch_items;
    uint32_t packet_size;
    uint32_t selected_profile;

    /* Server-assigned session ID (from hello-ack) */
    uint64_t session_id;

    /* Internal receive buffer for chunked reassembly */
    uint8_t *recv_buf;
    size_t   recv_buf_size;

    /* In-flight message_id set (client-side only, dynamically grown) */
    uint64_t *inflight_ids;
    uint32_t  inflight_count;
    uint32_t  inflight_capacity;
} nipc_uds_session_t;

/* ------------------------------------------------------------------ */
/*  Listener                                                           */
/* ------------------------------------------------------------------ */

typedef struct {
    int                      fd;      /* listening socket */
    nipc_uds_server_config_t config;
    char                     path[108]; /* stored for cleanup */
} nipc_uds_listener_t;

/* ------------------------------------------------------------------ */
/*  Connection lifecycle                                               */
/* ------------------------------------------------------------------ */

/*
 * Create a listener on {run_dir}/{service_name}.sock.
 * Performs stale endpoint recovery: if the socket file exists and no
 * live server is connected, unlinks and recreates it.
 */
nipc_uds_error_t nipc_uds_listen(const char *run_dir,
                                  const char *service_name,
                                  const nipc_uds_server_config_t *config,
                                  nipc_uds_listener_t *out);

/*
 * Accept one client on a listener. Performs the full handshake.
 * session_id is placed into the hello-ack so the client can attach
 * to the correct per-session SHM region.
 * Blocks until a client connects and the handshake completes (or fails).
 */
nipc_uds_error_t nipc_uds_accept(nipc_uds_listener_t *listener,
                                  uint64_t session_id,
                                  nipc_uds_session_t *out);

/*
 * Connect to a server at {run_dir}/{service_name}.sock.
 * Performs the full handshake. Blocks until connected + handshake done.
 */
nipc_uds_error_t nipc_uds_connect(const char *run_dir,
                                   const char *service_name,
                                   const nipc_uds_client_config_t *config,
                                   nipc_uds_session_t *out);

/*
 * Close a session. Releases socket and internal buffers.
 * Safe to call on a zero-initialized session (no-op).
 */
void nipc_uds_close_session(nipc_uds_session_t *session);

/*
 * Close a listener. Stops accepting, closes the socket, unlinks
 * the socket file.
 */
void nipc_uds_close_listener(nipc_uds_listener_t *listener);

/* ------------------------------------------------------------------ */
/*  Message send / receive                                             */
/* ------------------------------------------------------------------ */

/*
 * Send one logical message. hdr is the 32-byte outer header (caller fills
 * kind, code, flags, payload_len, item_count, message_id; this function
 * sets magic/version/header_len). payload is the opaque payload bytes.
 *
 * If the total message (32 + payload_len) exceeds packet_size, the
 * message is chunked transparently.
 *
 * Blocks until all chunks are sent or an error occurs.
 */
nipc_uds_error_t nipc_uds_send(nipc_uds_session_t *session,
                                nipc_header_t *hdr,
                                const void *payload,
                                size_t payload_len);

/*
 * Receive one logical message. Blocks until a complete message arrives.
 *
 * On success:
 *   - hdr_out is filled with the decoded outer header.
 *   - *payload_out points to the payload bytes (inside session->recv_buf
 *     or buf). Valid until the next nipc_uds_receive call.
 *   - *payload_len_out is the payload length.
 *
 * buf/buf_size: caller-provided buffer. If the message fits, it is
 * placed here. If the message is chunked and exceeds buf_size, the
 * session's internal recv_buf is grown to fit (allocated via realloc).
 */
nipc_uds_error_t nipc_uds_receive(nipc_uds_session_t *session,
                                   void *buf, size_t buf_size,
                                   nipc_header_t *hdr_out,
                                   const void **payload_out,
                                   size_t *payload_len_out);

/* ------------------------------------------------------------------ */
/*  Utility                                                            */
/* ------------------------------------------------------------------ */

/* Get the fd for poll/epoll integration. */
static inline int nipc_uds_session_fd(const nipc_uds_session_t *s) {
    return s->fd;
}

static inline int nipc_uds_listener_fd(const nipc_uds_listener_t *l) {
    return l->fd;
}

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_UDS_H */
