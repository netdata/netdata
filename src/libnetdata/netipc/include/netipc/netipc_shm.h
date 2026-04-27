/*
 * netipc_shm.h - L1 POSIX SHM transport (Linux only).
 *
 * Shared memory data plane with spin+futex synchronization.
 * The SHM region carries the same outer protocol envelope as the UDS
 * transport. Higher levels see no difference.
 *
 * Lifecycle:
 *   1. UDS handshake negotiates PROFILE_SHM_HYBRID.
 *   2. Server creates the SHM region; client attaches.
 *   3. Data plane switches to SHM; UDS socket stays open.
 */

#ifndef NETIPC_SHM_H
#define NETIPC_SHM_H

#include "netipc_protocol.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

#define NIPC_SHM_REGION_MAGIC     0x4e53484du  /* "NSHM" */
#define NIPC_SHM_REGION_VERSION   3u
#define NIPC_SHM_REGION_ALIGNMENT 64u
#define NIPC_SHM_HEADER_LEN       64u
#define NIPC_SHM_DEFAULT_SPIN     128u

/* ------------------------------------------------------------------ */
/*  Error codes                                                        */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_SHM_OK = 0,
    NIPC_SHM_ERR_PATH_TOO_LONG,     /* SHM path exceeds limit */
    NIPC_SHM_ERR_OPEN,              /* open/shm_open failed */
    NIPC_SHM_ERR_TRUNCATE,          /* ftruncate failed */
    NIPC_SHM_ERR_MMAP,              /* mmap failed */
    NIPC_SHM_ERR_BAD_MAGIC,         /* header magic mismatch */
    NIPC_SHM_ERR_BAD_VERSION,       /* header version mismatch */
    NIPC_SHM_ERR_BAD_HEADER,        /* header_len mismatch or corrupt */
    NIPC_SHM_ERR_BAD_SIZE,          /* file too small / capacity mismatch */
    NIPC_SHM_ERR_ADDR_IN_USE,       /* live server owns the region */
    NIPC_SHM_ERR_NOT_READY,         /* server hasn't finished setup (retry) */
    NIPC_SHM_ERR_MSG_TOO_LARGE,     /* message exceeds area capacity */
    NIPC_SHM_ERR_TIMEOUT,           /* futex wait timed out */
    NIPC_SHM_ERR_BAD_PARAM,         /* invalid argument */
    NIPC_SHM_ERR_PEER_DEAD,         /* owner process has exited */
} nipc_shm_error_t;

/* ------------------------------------------------------------------ */
/*  Role                                                               */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_SHM_ROLE_SERVER = 1,
    NIPC_SHM_ROLE_CLIENT = 2,
} nipc_shm_role_t;

/* ------------------------------------------------------------------ */
/*  Region header (64 bytes, mapped at offset 0)                       */
/* ------------------------------------------------------------------ */

/*
 * This struct is the on-disk/on-memory layout. Atomic fields are
 * accessed through __atomic builtins, not through direct struct reads.
 * The struct is declared solely for offset verification via
 * _Static_assert and for header initialization.
 */
typedef struct {
    uint32_t magic;              /*  0: NIPC_SHM_REGION_MAGIC */
    uint16_t version;            /*  4: NIPC_SHM_REGION_VERSION */
    uint16_t header_len;         /*  6: 64 */
    int32_t  owner_pid;          /*  8: server PID */
    uint32_t owner_generation;   /* 12: generation for PID reuse detection */
    uint32_t request_offset;     /* 16: byte offset to request area */
    uint32_t request_capacity;   /* 20: request area size */
    uint32_t response_offset;    /* 24: byte offset to response area */
    uint32_t response_capacity;  /* 28: response area size */
    uint64_t req_seq;            /* 32: request sequence (atomic) */
    uint64_t resp_seq;           /* 40: response sequence (atomic) */
    uint32_t req_len;            /* 48: current request msg length (atomic) */
    uint32_t resp_len;           /* 52: current response msg length (atomic) */
    uint32_t req_signal;         /* 56: request futex word (atomic) */
    uint32_t resp_signal;        /* 60: response futex word (atomic) */
} nipc_shm_region_header_t;

_Static_assert(sizeof(nipc_shm_region_header_t) == 64,
               "SHM region header must be exactly 64 bytes");

/* ------------------------------------------------------------------ */
/*  SHM context                                                        */
/* ------------------------------------------------------------------ */

typedef struct {
    nipc_shm_role_t role;

    int   fd;                    /* file descriptor for the SHM region */
    void *base;                  /* mmap base pointer */
    size_t region_size;          /* total mapped size */

    /* Cached from header (avoid repeated volatile reads) */
    uint32_t request_offset;
    uint32_t request_capacity;
    uint32_t response_offset;
    uint32_t response_capacity;

    /* Sequence tracking for send/receive */
    uint64_t local_req_seq;      /* last known req_seq */
    uint64_t local_resp_seq;     /* last known resp_seq */

    uint32_t spin_tries;         /* spin count before futex wait */
    uint32_t owner_generation;   /* cached for PID reuse detection */

    char     path[256];          /* stored for unlink on destroy */

} nipc_shm_ctx_t;

/* ------------------------------------------------------------------ */
/*  Server API                                                         */
/* ------------------------------------------------------------------ */

/*
 * Create a per-session SHM region at
 *   {run_dir}/{service_name}-{session_id:016x}.ipcshm
 *
 * session_id is the server-assigned session identifier (from hello-ack).
 * req_capacity / resp_capacity are the data area sizes in bytes.
 * They will be rounded up to NIPC_SHM_REGION_ALIGNMENT.
 */
nipc_shm_error_t nipc_shm_server_create(const char *run_dir,
                                          const char *service_name,
                                          uint64_t session_id,
                                          uint32_t req_capacity,
                                          uint32_t resp_capacity,
                                          nipc_shm_ctx_t *out);

/*
 * Destroy a server SHM region: munmap, close, unlink.
 */
void nipc_shm_destroy(nipc_shm_ctx_t *ctx);

/* ------------------------------------------------------------------ */
/*  Client API                                                         */
/* ------------------------------------------------------------------ */

/*
 * Attach to a per-session SHM region at
 *   {run_dir}/{service_name}-{session_id:016x}.ipcshm
 *
 * session_id is from the hello-ack received during handshake.
 * Validates the header (magic, version, sizes). If the file is
 * undersized (server not ready), returns NIPC_SHM_ERR_NOT_READY.
 */
nipc_shm_error_t nipc_shm_client_attach(const char *run_dir,
                                          const char *service_name,
                                          uint64_t session_id,
                                          nipc_shm_ctx_t *out);

/*
 * Detach a client from the SHM region: munmap, close (no unlink).
 */
void nipc_shm_close(nipc_shm_ctx_t *ctx);

/* ------------------------------------------------------------------ */
/*  Data plane                                                         */
/* ------------------------------------------------------------------ */

/*
 * Publish a message into the SHM region (client sends request,
 * server sends response). The role determines which area is written.
 *
 * The message must include the 32-byte outer header + payload, exactly
 * as would be sent over UDS.
 *
 * Returns NIPC_SHM_ERR_MSG_TOO_LARGE if msg_len exceeds the area capacity.
 */
nipc_shm_error_t nipc_shm_send(nipc_shm_ctx_t *ctx,
                                 const void *msg, size_t msg_len);

/*
 * Receive a message from the SHM region (server receives request,
 * client receives response).
 *
 * Spins for ctx->spin_tries iterations, then falls back to futex wait
 * with timeout_ms. Pass 0 for no timeout (infinite wait).
 *
 * The message is copied into the caller-provided buffer (buf, buf_size).
 * On success, *msg_len_out is the message length. Returns
 * NIPC_SHM_ERR_MSG_TOO_LARGE if the message exceeds buf_size.
 */
nipc_shm_error_t nipc_shm_receive(nipc_shm_ctx_t *ctx,
                                    void *buf,
                                    size_t buf_size,
                                    size_t *msg_len_out,
                                    uint32_t timeout_ms);

/* ------------------------------------------------------------------ */
/*  Utility                                                            */
/* ------------------------------------------------------------------ */

/* Check if the region's owner process is still alive. */
bool nipc_shm_owner_alive(const nipc_shm_ctx_t *ctx);

/*
 * Scan for and unlink stale per-session SHM files left by a crashed
 * server. Call once at server startup before accepting connections.
 * Files matching {run_dir}/{service_name}-*.ipcshm whose owner_pid
 * is dead (or whose generation mismatches) are unlinked.
 */
void nipc_shm_cleanup_stale(const char *run_dir, const char *service_name);

/* Get the file descriptor for external event integration. */
static inline int nipc_shm_fd(const nipc_shm_ctx_t *ctx) {
    return ctx->fd;
}

#ifdef __cplusplus
}
#endif

#endif /* NETIPC_SHM_H */
