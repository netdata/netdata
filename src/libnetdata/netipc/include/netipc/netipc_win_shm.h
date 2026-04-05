/*
 * netipc_win_shm.h - L1 Windows SHM transport.
 *
 * Shared memory data plane with spin + kernel event synchronization.
 * Uses CreateFileMappingW/MapViewOfFile for the region and
 * auto-reset kernel events for synchronization.
 *
 * Kernel object name derivation:
 *   Local\netipc-{FNV1a64(run_dir+"\n"+service_name+"\n"+auth_token):016llx}-{service}-p{profile}-s{session_id:016llx}-mapping
 *   Local\netipc-{hash}-{service}-p{profile}-s{session_id:016llx}-req_event
 *   Local\netipc-{hash}-{service}-p{profile}-s{session_id:016llx}-resp_event
 */

#ifndef NETIPC_WIN_SHM_H
#define NETIPC_WIN_SHM_H

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc_protocol.h"
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <windows.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */

/* Magic: "NSWH" as u32 LE */
#define NIPC_WIN_SHM_MAGIC          0x4e535748u
#define NIPC_WIN_SHM_VERSION        3u
#define NIPC_WIN_SHM_HEADER_LEN     128u
#define NIPC_WIN_SHM_CACHELINE      64u

/* Profile bits */
#define NIPC_WIN_SHM_PROFILE_HYBRID    0x02u
#define NIPC_WIN_SHM_PROFILE_BUSYWAIT  0x04u

/* Default spin count (higher than POSIX due to Windows kernel overhead) */
#define NIPC_WIN_SHM_DEFAULT_SPIN   1024u

/* Busy-wait deadline poll mask */
#define NIPC_WIN_SHM_BUSYWAIT_POLL_MASK 1023u

/* Max kernel object name length */
#define NIPC_WIN_SHM_MAX_NAME       256

/* ------------------------------------------------------------------ */
/*  Error codes                                                        */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_WIN_SHM_OK = 0,
    NIPC_WIN_SHM_ERR_BAD_PARAM,         /* invalid argument */
    NIPC_WIN_SHM_ERR_CREATE_MAPPING,     /* CreateFileMappingW failed */
    NIPC_WIN_SHM_ERR_OPEN_MAPPING,       /* OpenFileMappingW failed */
    NIPC_WIN_SHM_ERR_MAP_VIEW,           /* MapViewOfFile failed */
    NIPC_WIN_SHM_ERR_CREATE_EVENT,       /* CreateEventW failed */
    NIPC_WIN_SHM_ERR_OPEN_EVENT,         /* OpenEventW failed */
    NIPC_WIN_SHM_ERR_ADDR_IN_USE,        /* named mapping/event already exists */
    NIPC_WIN_SHM_ERR_BAD_MAGIC,          /* header magic mismatch */
    NIPC_WIN_SHM_ERR_BAD_VERSION,        /* header version mismatch */
    NIPC_WIN_SHM_ERR_BAD_HEADER,         /* header_len mismatch */
    NIPC_WIN_SHM_ERR_BAD_PROFILE,        /* profile mismatch */
    NIPC_WIN_SHM_ERR_MSG_TOO_LARGE,      /* message exceeds area capacity */
    NIPC_WIN_SHM_ERR_TIMEOUT,            /* wait timed out */
    NIPC_WIN_SHM_ERR_DISCONNECTED,       /* peer closed */
} nipc_win_shm_error_t;

/* ------------------------------------------------------------------ */
/*  Role                                                               */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_WIN_SHM_ROLE_SERVER = 1,
    NIPC_WIN_SHM_ROLE_CLIENT = 2,
} nipc_win_shm_role_t;

/* ------------------------------------------------------------------ */
/*  Region header (128 bytes, mapped at offset 0)                      */
/* ------------------------------------------------------------------ */

/*
 * On-disk/on-memory layout. Volatile fields are accessed via
 * InterlockedExchange / InterlockedCompareExchange, not direct reads.
 * Declared for offset verification and header initialization.
 */
#pragma pack(push, 1)
typedef struct {
    uint32_t magic;              /*  0: NIPC_WIN_SHM_MAGIC */
    uint32_t version;            /*  4: NIPC_WIN_SHM_VERSION */
    uint32_t header_len;         /*  8: 128 */
    uint32_t profile;            /* 12: selected profile */
    uint32_t request_offset;     /* 16: byte offset to request area */
    uint32_t request_capacity;   /* 20: request area size */
    uint32_t response_offset;    /* 24: byte offset to response area */
    uint32_t response_capacity;  /* 28: response area size */
    uint32_t spin_tries;         /* 32: spin iterations before kernel wait */
    volatile LONG req_len;       /* 36: current request message length */
    volatile LONG resp_len;      /* 40: current response message length */
    volatile LONG req_client_closed;  /* 44: client-side close flag */
    volatile LONG req_server_waiting; /* 48: server waiting for request */
    volatile LONG resp_server_closed; /* 52: server-side close flag */
    volatile LONG resp_client_waiting;/* 56: client waiting for response */
    uint32_t _padding;           /* 60: reserved */
    volatile LONG64 req_seq;     /* 64: request sequence number */
    volatile LONG64 resp_seq;    /* 72: response sequence number */
    uint8_t  _reserved[48];      /* 80: reserved for future use */
} nipc_win_shm_region_header_t;
#pragma pack(pop)

/* Compile-time assertions for header layout */
_Static_assert(sizeof(nipc_win_shm_region_header_t) == 128,
               "Windows SHM region header must be exactly 128 bytes");
_Static_assert(offsetof(nipc_win_shm_region_header_t, spin_tries) == 32,
               "spin_tries must be at offset 32");
_Static_assert(offsetof(nipc_win_shm_region_header_t, req_len) == 36,
               "req_len must be at offset 36");
_Static_assert(offsetof(nipc_win_shm_region_header_t, resp_len) == 40,
               "resp_len must be at offset 40");
_Static_assert(offsetof(nipc_win_shm_region_header_t, req_client_closed) == 44,
               "req_client_closed must be at offset 44");
_Static_assert(offsetof(nipc_win_shm_region_header_t, req_server_waiting) == 48,
               "req_server_waiting must be at offset 48");
_Static_assert(offsetof(nipc_win_shm_region_header_t, resp_server_closed) == 52,
               "resp_server_closed must be at offset 52");
_Static_assert(offsetof(nipc_win_shm_region_header_t, resp_client_waiting) == 56,
               "resp_client_waiting must be at offset 56");
_Static_assert(offsetof(nipc_win_shm_region_header_t, req_seq) == 64,
               "req_seq must be at offset 64");
_Static_assert(offsetof(nipc_win_shm_region_header_t, resp_seq) == 72,
               "resp_seq must be at offset 72");

/* ------------------------------------------------------------------ */
/*  SHM context                                                        */
/* ------------------------------------------------------------------ */

typedef struct {
    nipc_win_shm_role_t role;

    HANDLE mapping;              /* file mapping handle */
    void  *base;                 /* MapViewOfFile base pointer */
    size_t region_size;          /* total mapped size */

    /* Kernel events (SHM_HYBRID only; INVALID_HANDLE_VALUE for BUSYWAIT) */
    HANDLE req_event;
    HANDLE resp_event;

    /* Cached from header */
    uint32_t profile;
    uint32_t request_offset;
    uint32_t request_capacity;
    uint32_t response_offset;
    uint32_t response_capacity;
    uint32_t spin_tries;

    /* Sequence tracking */
    LONG64   local_req_seq;
    LONG64   local_resp_seq;

} nipc_win_shm_ctx_t;

/* ------------------------------------------------------------------ */
/*  Server API                                                         */
/* ------------------------------------------------------------------ */

/*
 * Create a per-session Windows SHM region.
 *
 * The kernel objects are named using the FNV-1a hash of
 * run_dir + "\n" + service_name + "\n" + auth_token_decimal,
 * plus the session_id for per-session isolation.
 *
 * session_id: server-assigned session identifier (from hello-ack).
 * profile: NIPC_WIN_SHM_PROFILE_HYBRID or NIPC_WIN_SHM_PROFILE_BUSYWAIT.
 * req_capacity / resp_capacity: data area sizes in bytes.
 */
nipc_win_shm_error_t nipc_win_shm_server_create(
    const char *run_dir,
    const char *service_name,
    uint64_t auth_token,
    uint64_t session_id,
    uint32_t profile,
    uint32_t req_capacity,
    uint32_t resp_capacity,
    nipc_win_shm_ctx_t *ctx);

/*
 * Destroy a server SHM region.
 * Sets close flags, signals events, unmaps, closes handles.
 */
void nipc_win_shm_destroy(nipc_win_shm_ctx_t *ctx);

/* ------------------------------------------------------------------ */
/*  Client API                                                         */
/* ------------------------------------------------------------------ */

/*
 * Attach to an existing per-session Windows SHM region.
 * session_id is from the hello-ack received during handshake.
 * Validates header and opens event handles.
 */
nipc_win_shm_error_t nipc_win_shm_client_attach(
    const char *run_dir,
    const char *service_name,
    uint64_t auth_token,
    uint64_t session_id,
    uint32_t profile,
    nipc_win_shm_ctx_t *ctx);

/*
 * Cleanup stale Windows SHM kernel objects.
 * On Windows, kernel objects are reference-counted and auto-cleaned
 * when all handles close, so this is a no-op. Provided for API
 * symmetry with the POSIX SHM transport.
 */
void nipc_win_shm_cleanup_stale(const char *run_dir, const char *service_name);

/*
 * Close a client SHM context.
 * Sets close flags, signals events, unmaps, closes handles.
 */
void nipc_win_shm_close(nipc_win_shm_ctx_t *ctx);

/* ------------------------------------------------------------------ */
/*  Data plane                                                         */
/* ------------------------------------------------------------------ */

/*
 * Publish a message into the SHM region.
 * Client sends to request area; server sends to response area.
 * msg must include the 32-byte outer header + payload.
 */
nipc_win_shm_error_t nipc_win_shm_send(
    nipc_win_shm_ctx_t *ctx,
    const void *msg,
    size_t msg_len);

/*
 * Receive a message from the SHM region.
 * Server reads from request area; client reads from response area.
 * Copies into caller-provided buffer. Returns message length in *msg_len_out.
 */
nipc_win_shm_error_t nipc_win_shm_receive(
    nipc_win_shm_ctx_t *ctx,
    void *buf,
    size_t buf_size,
    size_t *msg_len_out,
    uint32_t timeout_ms);

#ifdef NIPC_INTERNAL_TESTING
typedef enum {
    NIPC_WIN_SHM_TEST_FAULT_NONE = 0,
    NIPC_WIN_SHM_TEST_FAULT_CREATE_MAPPING,
    NIPC_WIN_SHM_TEST_FAULT_OPEN_MAPPING,
    NIPC_WIN_SHM_TEST_FAULT_MAP_VIEW,
    NIPC_WIN_SHM_TEST_FAULT_CREATE_EVENT,
    NIPC_WIN_SHM_TEST_FAULT_OPEN_EVENT,
} nipc_win_shm_test_fault_site_t;

void nipc_win_shm_test_fault_set(nipc_win_shm_test_fault_site_t site,
                                 DWORD error_code,
                                 uint32_t skip_matches);
void nipc_win_shm_test_fault_clear(void);
#endif

#ifdef __cplusplus
}
#endif

#endif /* _WIN32 || __MSYS__ */

#endif /* NETIPC_WIN_SHM_H */
