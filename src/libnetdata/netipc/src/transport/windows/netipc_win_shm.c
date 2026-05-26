/*
 * netipc_win_shm.c - L1 Windows SHM transport.
 *
 * Shared memory data plane with spin + kernel event synchronization.
 * Uses CreateFileMappingW / MapViewOfFile for the shared region and
 * auto-reset kernel events for signaling (SHM_HYBRID profile).
 *
 * Wire-compatible with all language implementations.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc/netipc_win_shm.h"
#include "netipc/netipc_named_pipe.h" /* nipc_fnv1a_64 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ------------------------------------------------------------------ */
/*  Internal helpers                                                   */
/* ------------------------------------------------------------------ */

typedef enum {
    NIPC_WIN_SHM_TEST_FAULT_NONE = 0,
    NIPC_WIN_SHM_TEST_FAULT_CREATE_MAPPING,
    NIPC_WIN_SHM_TEST_FAULT_OPEN_MAPPING,
    NIPC_WIN_SHM_TEST_FAULT_MAP_VIEW,
    NIPC_WIN_SHM_TEST_FAULT_CREATE_EVENT,
    NIPC_WIN_SHM_TEST_FAULT_OPEN_EVENT,
} nipc_win_shm_test_fault_site_t_internal;

typedef struct {
    nipc_win_shm_test_fault_site_t_internal site;
    DWORD error_code;
    uint32_t skip_matches;
} nipc_win_shm_test_fault_t;

static nipc_win_shm_test_fault_t g_win_shm_test_fault = {
    .site = NIPC_WIN_SHM_TEST_FAULT_NONE,
    .error_code = ERROR_SUCCESS,
    .skip_matches = 0,
};

void nipc_win_shm_test_fault_set(int site,
                                 DWORD error_code,
                                 uint32_t skip_matches)
{
    g_win_shm_test_fault.site = (nipc_win_shm_test_fault_site_t_internal)site;
    g_win_shm_test_fault.error_code = error_code;
    g_win_shm_test_fault.skip_matches = skip_matches;
}

void nipc_win_shm_test_fault_clear(void)
{
    g_win_shm_test_fault.site = NIPC_WIN_SHM_TEST_FAULT_NONE;
    g_win_shm_test_fault.error_code = ERROR_SUCCESS;
    g_win_shm_test_fault.skip_matches = 0;
}

static int should_inject_fault(nipc_win_shm_test_fault_site_t_internal site)
{
    if (g_win_shm_test_fault.site != site)
        return 0;

    if (g_win_shm_test_fault.skip_matches > 0) {
        g_win_shm_test_fault.skip_matches--;
        return 0;
    }

    SetLastError(g_win_shm_test_fault.error_code);
    g_win_shm_test_fault.site = NIPC_WIN_SHM_TEST_FAULT_NONE;
    return 1;
}

static HANDLE test_create_file_mapping(HANDLE file,
                                       LPSECURITY_ATTRIBUTES attrs,
                                       DWORD protect,
                                       DWORD size_high,
                                       DWORD size_low,
                                       LPCWSTR name)
{
    if (should_inject_fault(NIPC_WIN_SHM_TEST_FAULT_CREATE_MAPPING))
        return NULL;

    return CreateFileMappingW(file, attrs, protect, size_high, size_low, name);
}

static HANDLE test_open_file_mapping(DWORD access,
                                     BOOL inherit,
                                     LPCWSTR name)
{
    if (should_inject_fault(NIPC_WIN_SHM_TEST_FAULT_OPEN_MAPPING))
        return NULL;

    return OpenFileMappingW(access, inherit, name);
}

static void *test_map_view_of_file(HANDLE mapping,
                                   DWORD access,
                                   DWORD offset_high,
                                   DWORD offset_low,
                                   SIZE_T bytes)
{
    if (should_inject_fault(NIPC_WIN_SHM_TEST_FAULT_MAP_VIEW))
        return NULL;

    return MapViewOfFile(mapping, access, offset_high, offset_low, bytes);
}

static HANDLE test_create_event(LPSECURITY_ATTRIBUTES attrs,
                                BOOL manual_reset,
                                BOOL initial_state,
                                LPCWSTR name)
{
    if (should_inject_fault(NIPC_WIN_SHM_TEST_FAULT_CREATE_EVENT))
        return NULL;

    return CreateEventW(attrs, manual_reset, initial_state, name);
}

static HANDLE test_open_event(DWORD access,
                              BOOL inherit,
                              LPCWSTR name)
{
    if (should_inject_fault(NIPC_WIN_SHM_TEST_FAULT_OPEN_EVENT))
        return NULL;

    return OpenEventW(access, inherit, name);
}

/* Round up to 64-byte cache-line alignment. */
static inline uint32_t align_cacheline(uint32_t v)
{
    return (v + (NIPC_WIN_SHM_CACHELINE - 1)) & ~(uint32_t)(NIPC_WIN_SHM_CACHELINE - 1);
}

/* Atomic reads without write contention.
 * InterlockedCompareExchange64(ptr, 0, 0) emits LOCK CMPXCHG8B which writes
 * the cache line on every call — catastrophic in spin loops. MemoryBarrier()
 * emits MFENCE which flushes the store buffer — also expensive.
 *
 * On x86-64, aligned 64-bit reads are naturally atomic and have acquire
 * semantics (loads are never reordered with loads). A volatile read with
 * a compiler barrier (no hardware barrier) is sufficient and matches what
 * Go's atomic.LoadInt64 compiles to (plain MOV). */
static inline LONG64 atomic_load_64(volatile LONG64 *ptr)
{
    LONG64 val = *ptr;
    __asm__ volatile("" ::: "memory"); /* compiler barrier only */
    return val;
}

static inline LONG atomic_load_32(volatile LONG *ptr)
{
    LONG val = *ptr;
    __asm__ volatile("" ::: "memory"); /* compiler barrier only */
    return val;
}

/* Validate service_name: only [a-zA-Z0-9._-], non-empty, not "." or "..". */
static int validate_service_name(const char *name)
{
    if (!name || name[0] == '\0')
        return -1;

    if (name[0] == '.' && (name[1] == '\0' || (name[1] == '.' && name[2] == '\0')))
        return -1;

    for (const char *p = name; *p; p++) {
        char c = *p;
        if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
            (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-')
            continue;
        return -1;
    }
    return 0;
}

/* CPU pause hint for spin loops. */
static inline void cpu_relax(void)
{
#if defined(__x86_64__) || defined(__i386__) || defined(_M_X64) || defined(_M_IX86)
    YieldProcessor();
#else
    /* generic fallback */
    MemoryBarrier();
#endif
}

/* Pointer into the mapped region at a byte offset. */
static inline void *region_ptr(const nipc_win_shm_ctx_t *ctx, uint32_t offset)
{
    return (uint8_t *)ctx->base + offset;
}

/* Pointer to the header. */
static inline nipc_win_shm_region_header_t *region_hdr(const nipc_win_shm_ctx_t *ctx)
{
    return (nipc_win_shm_region_header_t *)ctx->base;
}

/* ------------------------------------------------------------------ */
/*  Kernel object name derivation                                      */
/* ------------------------------------------------------------------ */

/*
 * Compute FNV-1a hash for SHM object names.
 * Input: run_dir + "\n" + service_name + "\n" + auth_token_decimal
 */
static uint64_t compute_shm_hash(const char *run_dir,
                                  const char *service_name,
                                  uint64_t auth_token)
{
    char buf[512];
    int n = snprintf(buf, sizeof(buf), "%s\n%s\n%llu",
                     run_dir, service_name,
                     (unsigned long long)auth_token);
    if (n < 0 || (size_t)n >= sizeof(buf))
        return 0;

    return nipc_fnv1a_64(buf, (size_t)n);
}

/*
 * Build a kernel object name.
 * Format: Local\netipc-{hash:016llx}-{service}-p{profile}-s{session_id:016llx}-{suffix}
 */
static int build_object_name(wchar_t *dst, size_t dst_chars,
                              uint64_t hash,
                              const char *service_name,
                              uint32_t profile,
                              uint64_t session_id,
                              const char *suffix)
{
    char narrow[NIPC_WIN_SHM_MAX_NAME];
    int n = snprintf(narrow, sizeof(narrow),
                     "Local\\netipc-%016llx-%s-p%u-s%016llx-%s",
                     (unsigned long long)hash, service_name,
                     (unsigned)profile,
                     (unsigned long long)session_id, suffix);
    if (n < 0 || (size_t)n >= sizeof(narrow))
        return -1;

    if ((size_t)(n + 1) > dst_chars)
        return -1;

    for (int i = 0; i <= n; i++)
        dst[i] = (wchar_t)(unsigned char)narrow[i];

    return 0;
}

/* ------------------------------------------------------------------ */
/*  Server: create                                                     */
/* ------------------------------------------------------------------ */

nipc_win_shm_error_t nipc_win_shm_server_create(
    const char *run_dir,
    const char *service_name,
    uint64_t auth_token,
    uint64_t session_id,
    uint32_t profile,
    uint32_t req_capacity,
    uint32_t resp_capacity,
    nipc_win_shm_ctx_t *ctx)
{
    if (!ctx)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    memset(ctx, 0, sizeof(*ctx));
    ctx->mapping    = NULL;
    ctx->req_event  = INVALID_HANDLE_VALUE;
    ctx->resp_event = INVALID_HANDLE_VALUE;

    if (!run_dir || !service_name)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    if (validate_service_name(service_name) < 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    if (profile != NIPC_WIN_SHM_PROFILE_HYBRID &&
        profile != NIPC_WIN_SHM_PROFILE_BUSYWAIT)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    /* Compute hash and build object names */
    uint64_t hash = compute_shm_hash(run_dir, service_name, auth_token);
    if (hash == 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    wchar_t mapping_name[NIPC_WIN_SHM_MAX_NAME];
    if (build_object_name(mapping_name, NIPC_WIN_SHM_MAX_NAME,
                           hash, service_name, profile, session_id,
                           "mapping") < 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    /* Compute aligned offsets */
    req_capacity  = align_cacheline(req_capacity);
    resp_capacity = align_cacheline(resp_capacity);

    uint32_t req_off  = align_cacheline(NIPC_WIN_SHM_HEADER_LEN);
    /* Guard against uint32 overflow before computing resp_off */
    if (req_capacity > UINT32_MAX - req_off)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;
    uint32_t resp_off = align_cacheline(req_off + req_capacity);
    if (resp_capacity > UINT32_MAX - resp_off ||
        (size_t)resp_off > SIZE_MAX - (size_t)resp_capacity)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;
    size_t region_size = (size_t)resp_off + resp_capacity;

    /* Create file mapping backed by page file */
    SetLastError(ERROR_SUCCESS);
    HANDLE mapping = test_create_file_mapping(
        INVALID_HANDLE_VALUE,   /* page file backed */
        NULL,                   /* default security */
        PAGE_READWRITE,
        (DWORD)(region_size >> 32),
        (DWORD)(region_size & 0xFFFFFFFF),
        mapping_name);
    if (!mapping)
        return NIPC_WIN_SHM_ERR_CREATE_MAPPING;
    if (GetLastError() == ERROR_ALREADY_EXISTS) {
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_ADDR_IN_USE;
    }

    /* Map the view */
    void *base = test_map_view_of_file(mapping, FILE_MAP_ALL_ACCESS, 0, 0, region_size);
    if (!base) {
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_MAP_VIEW;
    }

    /* Zero the region */
    memset(base, 0, region_size);

    /* Write header */
    nipc_win_shm_region_header_t *hdr = (nipc_win_shm_region_header_t *)base;
    hdr->magic             = NIPC_WIN_SHM_MAGIC;
    hdr->version           = NIPC_WIN_SHM_VERSION;
    hdr->header_len        = NIPC_WIN_SHM_HEADER_LEN;
    hdr->profile           = profile;
    hdr->request_offset    = req_off;
    hdr->request_capacity  = req_capacity;
    hdr->response_offset   = resp_off;
    hdr->response_capacity = resp_capacity;
    hdr->spin_tries        = NIPC_WIN_SHM_DEFAULT_SPIN;

    /* Memory barrier to ensure header is visible */
    MemoryBarrier();

    /* Create kernel events for HYBRID profile */
    HANDLE req_event  = INVALID_HANDLE_VALUE;
    HANDLE resp_event = INVALID_HANDLE_VALUE;

    if (profile == NIPC_WIN_SHM_PROFILE_HYBRID) {
        wchar_t req_event_name[NIPC_WIN_SHM_MAX_NAME];
        wchar_t resp_event_name[NIPC_WIN_SHM_MAX_NAME];

        if (build_object_name(req_event_name, NIPC_WIN_SHM_MAX_NAME,
                               hash, service_name, profile, session_id,
                               "req_event") < 0 ||
            build_object_name(resp_event_name, NIPC_WIN_SHM_MAX_NAME,
                               hash, service_name, profile, session_id,
                               "resp_event") < 0) {
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_BAD_PARAM;
        }

        /* Auto-reset events (bManualReset = FALSE) */
        SetLastError(ERROR_SUCCESS);
        req_event = test_create_event(NULL, FALSE, FALSE, req_event_name);
        if (!req_event) {
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_CREATE_EVENT;
        }
        if (GetLastError() == ERROR_ALREADY_EXISTS) {
            CloseHandle(req_event);
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_ADDR_IN_USE;
        }

        SetLastError(ERROR_SUCCESS);
        resp_event = test_create_event(NULL, FALSE, FALSE, resp_event_name);
        if (!resp_event) {
            CloseHandle(req_event);
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_CREATE_EVENT;
        }
        if (GetLastError() == ERROR_ALREADY_EXISTS) {
            CloseHandle(resp_event);
            CloseHandle(req_event);
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_ADDR_IN_USE;
        }
    }

    /* Fill context */
    ctx->role              = NIPC_WIN_SHM_ROLE_SERVER;
    ctx->mapping           = mapping;
    ctx->base              = base;
    ctx->region_size       = region_size;
    ctx->req_event         = req_event;
    ctx->resp_event        = resp_event;
    ctx->profile           = profile;
    ctx->request_offset    = req_off;
    ctx->request_capacity  = req_capacity;
    ctx->response_offset   = resp_off;
    ctx->response_capacity = resp_capacity;
    ctx->spin_tries        = NIPC_WIN_SHM_DEFAULT_SPIN;
    ctx->local_req_seq     = 0;
    ctx->local_resp_seq    = 0;

    return NIPC_WIN_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Server: destroy                                                    */
/* ------------------------------------------------------------------ */

void nipc_win_shm_destroy(nipc_win_shm_ctx_t *ctx)
{
    if (!ctx)
        return;

    /* Set close flag and signal waiting client */
    if (ctx->base) {
        nipc_win_shm_region_header_t *hdr = region_hdr(ctx);
        InterlockedExchange(&hdr->resp_server_closed, 1);
        MemoryBarrier();
    }

    if (ctx->profile == NIPC_WIN_SHM_PROFILE_HYBRID &&
        ctx->resp_event != INVALID_HANDLE_VALUE)
        SetEvent(ctx->resp_event);

    if (ctx->base) {
        UnmapViewOfFile(ctx->base);
        ctx->base = NULL;
    }

    if (ctx->mapping) {
        CloseHandle(ctx->mapping);
        ctx->mapping = NULL;
    }

    if (ctx->req_event != INVALID_HANDLE_VALUE) {
        CloseHandle(ctx->req_event);
        ctx->req_event = INVALID_HANDLE_VALUE;
    }

    if (ctx->resp_event != INVALID_HANDLE_VALUE) {
        CloseHandle(ctx->resp_event);
        ctx->resp_event = INVALID_HANDLE_VALUE;
    }

    ctx->region_size = 0;
}

/* ------------------------------------------------------------------ */
/*  Client: attach                                                     */
/* ------------------------------------------------------------------ */

nipc_win_shm_error_t nipc_win_shm_client_attach(
    const char *run_dir,
    const char *service_name,
    uint64_t auth_token,
    uint64_t session_id,
    uint32_t profile,
    nipc_win_shm_ctx_t *ctx)
{
    if (!ctx)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    memset(ctx, 0, sizeof(*ctx));
    ctx->mapping    = NULL;
    ctx->req_event  = INVALID_HANDLE_VALUE;
    ctx->resp_event = INVALID_HANDLE_VALUE;

    if (!run_dir || !service_name)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    if (validate_service_name(service_name) < 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    if (profile != NIPC_WIN_SHM_PROFILE_HYBRID &&
        profile != NIPC_WIN_SHM_PROFILE_BUSYWAIT)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    uint64_t hash = compute_shm_hash(run_dir, service_name, auth_token);
    if (hash == 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    wchar_t mapping_name[NIPC_WIN_SHM_MAX_NAME];
    if (build_object_name(mapping_name, NIPC_WIN_SHM_MAX_NAME,
                           hash, service_name, profile, session_id,
                           "mapping") < 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    /* Open existing file mapping */
    HANDLE mapping = test_open_file_mapping(FILE_MAP_ALL_ACCESS, FALSE, mapping_name);
    if (!mapping)
        return NIPC_WIN_SHM_ERR_OPEN_MAPPING;

    /* Map the view -- map header first to read region size */
    void *base = test_map_view_of_file(mapping, FILE_MAP_ALL_ACCESS, 0, 0, 0);
    if (!base) {
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_MAP_VIEW;
    }

    /* Acquire barrier to see server's header writes */
    MemoryBarrier();

    const nipc_win_shm_region_header_t *hdr =
        (const nipc_win_shm_region_header_t *)base;

    /* Validate header */
    if (hdr->magic != NIPC_WIN_SHM_MAGIC) {
        UnmapViewOfFile(base);
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_BAD_MAGIC;
    }

    if (hdr->version != NIPC_WIN_SHM_VERSION) {
        UnmapViewOfFile(base);
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_BAD_VERSION;
    }

    if (hdr->header_len != NIPC_WIN_SHM_HEADER_LEN) {
        UnmapViewOfFile(base);
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_BAD_HEADER;
    }

    if (hdr->profile != profile) {
        UnmapViewOfFile(base);
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_BAD_PROFILE;
    }

    /* Cache and validate header values */
    uint32_t req_off  = hdr->request_offset;
    uint32_t req_cap  = hdr->request_capacity;
    uint32_t resp_off = hdr->response_offset;
    uint32_t resp_cap = hdr->response_capacity;
    uint32_t spin     = hdr->spin_tries;

    /* Validate offsets and capacities from shared memory */
    if (req_off == 0 || req_cap == 0 || resp_off == 0 || resp_cap == 0 ||
        req_off % 64 != 0 || req_cap % 64 != 0 ||
        resp_off % 64 != 0 || resp_cap % 64 != 0 ||
        req_off > UINT32_MAX - req_cap ||
        resp_off < req_off + req_cap) {
        UnmapViewOfFile(base);
        CloseHandle(mapping);
        return NIPC_WIN_SHM_ERR_BAD_HEADER;
    }

    /* Read current sequence numbers */
    LONG64 cur_req_seq  = atomic_load_64(
        (volatile LONG64 *)&((nipc_win_shm_region_header_t *)base)->req_seq);
    LONG64 cur_resp_seq = atomic_load_64(
        (volatile LONG64 *)&((nipc_win_shm_region_header_t *)base)->resp_seq);

    size_t region_size = (size_t)resp_off + resp_cap;

    /* Open kernel events for HYBRID profile */
    HANDLE req_event  = INVALID_HANDLE_VALUE;
    HANDLE resp_event = INVALID_HANDLE_VALUE;

    if (profile == NIPC_WIN_SHM_PROFILE_HYBRID) {
        wchar_t req_event_name[NIPC_WIN_SHM_MAX_NAME];
        wchar_t resp_event_name[NIPC_WIN_SHM_MAX_NAME];

        if (build_object_name(req_event_name, NIPC_WIN_SHM_MAX_NAME,
                               hash, service_name, profile, session_id,
                               "req_event") < 0 ||
            build_object_name(resp_event_name, NIPC_WIN_SHM_MAX_NAME,
                               hash, service_name, profile, session_id,
                               "resp_event") < 0) {
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_BAD_PARAM;
        }

        req_event = test_open_event(EVENT_MODIFY_STATE | SYNCHRONIZE, FALSE, req_event_name);
        if (!req_event) {
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_OPEN_EVENT;
        }

        resp_event = test_open_event(EVENT_MODIFY_STATE | SYNCHRONIZE, FALSE, resp_event_name);
        if (!resp_event) {
            CloseHandle(req_event);
            UnmapViewOfFile(base);
            CloseHandle(mapping);
            return NIPC_WIN_SHM_ERR_OPEN_EVENT;
        }
    }

    /* Fill context */
    ctx->role              = NIPC_WIN_SHM_ROLE_CLIENT;
    ctx->mapping           = mapping;
    ctx->base              = base;
    ctx->region_size       = region_size;
    ctx->req_event         = req_event;
    ctx->resp_event        = resp_event;
    ctx->profile           = profile;
    ctx->request_offset    = req_off;
    ctx->request_capacity  = req_cap;
    ctx->response_offset   = resp_off;
    ctx->response_capacity = resp_cap;
    ctx->spin_tries        = spin;
    ctx->local_req_seq     = cur_req_seq;
    ctx->local_resp_seq    = cur_resp_seq;

    return NIPC_WIN_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Client: close                                                      */
/* ------------------------------------------------------------------ */

void nipc_win_shm_close(nipc_win_shm_ctx_t *ctx)
{
    if (!ctx)
        return;

    /* Set close flag and signal waiting server */
    if (ctx->base) {
        nipc_win_shm_region_header_t *hdr = region_hdr(ctx);
        InterlockedExchange(&hdr->req_client_closed, 1);
        MemoryBarrier();
    }

    if (ctx->profile == NIPC_WIN_SHM_PROFILE_HYBRID &&
        ctx->req_event != INVALID_HANDLE_VALUE)
        SetEvent(ctx->req_event);

    if (ctx->base) {
        UnmapViewOfFile(ctx->base);
        ctx->base = NULL;
    }

    if (ctx->mapping) {
        CloseHandle(ctx->mapping);
        ctx->mapping = NULL;
    }

    if (ctx->req_event != INVALID_HANDLE_VALUE) {
        CloseHandle(ctx->req_event);
        ctx->req_event = INVALID_HANDLE_VALUE;
    }

    if (ctx->resp_event != INVALID_HANDLE_VALUE) {
        CloseHandle(ctx->resp_event);
        ctx->resp_event = INVALID_HANDLE_VALUE;
    }

    ctx->region_size = 0;
}

/* ------------------------------------------------------------------ */
/*  Data plane: send                                                   */
/* ------------------------------------------------------------------ */

nipc_win_shm_error_t nipc_win_shm_send(
    nipc_win_shm_ctx_t *ctx,
    const void *msg,
    size_t msg_len)
{
    if (!ctx || !ctx->base || !msg || msg_len == 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    uint32_t area_offset;
    uint32_t area_capacity;
    volatile LONG *len_ptr;
    volatile LONG64 *seq_ptr;
    volatile LONG *peer_waiting_ptr;
    HANDLE peer_event;

    nipc_win_shm_region_header_t *hdr = region_hdr(ctx);

    if (ctx->role == NIPC_WIN_SHM_ROLE_CLIENT) {
        area_offset        = ctx->request_offset;
        area_capacity      = ctx->request_capacity;
        len_ptr            = &hdr->req_len;
        seq_ptr            = &hdr->req_seq;
        peer_waiting_ptr   = &hdr->req_server_waiting;
        peer_event         = ctx->req_event;
    } else {
        area_offset        = ctx->response_offset;
        area_capacity      = ctx->response_capacity;
        len_ptr            = &hdr->resp_len;
        seq_ptr            = &hdr->resp_seq;
        peer_waiting_ptr   = &hdr->resp_client_waiting;
        peer_event         = ctx->resp_event;
    }

    if (msg_len > area_capacity || msg_len > 0x7FFFFFFFu)
        return NIPC_WIN_SHM_ERR_MSG_TOO_LARGE;

    /* 1. Write message data into the area */
    memcpy(region_ptr(ctx, area_offset), msg, msg_len);

    /* 2. Store message length (interlocked exchange) */
    InterlockedExchange(len_ptr, (LONG)msg_len);

    /* 3. Increment sequence number (interlocked increment) */
    InterlockedIncrement64(seq_ptr);

    /* 4. If HYBRID and peer is waiting, signal the event */
    if (ctx->profile == NIPC_WIN_SHM_PROFILE_HYBRID) {
        if (atomic_load_32(peer_waiting_ptr) != 0)
            SetEvent(peer_event);
    }

    /* Track locally */
    if (ctx->role == NIPC_WIN_SHM_ROLE_CLIENT)
        ctx->local_req_seq++;
    else
        ctx->local_resp_seq++;

    return NIPC_WIN_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Data plane: receive                                                */
/* ------------------------------------------------------------------ */

nipc_win_shm_error_t nipc_win_shm_receive(
    nipc_win_shm_ctx_t *ctx,
    void *buf,
    size_t buf_size,
    size_t *msg_len_out,
    uint32_t timeout_ms)
{
    if (!ctx || !ctx->base || !buf || !msg_len_out || buf_size == 0)
        return NIPC_WIN_SHM_ERR_BAD_PARAM;

    uint32_t area_offset;
    uint32_t area_capacity;
    volatile LONG *len_ptr;
    volatile LONG64 *seq_ptr;
    volatile LONG *self_waiting_ptr;
    volatile LONG *peer_closed_ptr;
    HANDLE wait_event;
    LONG64 expected_seq;

    nipc_win_shm_region_header_t *hdr = region_hdr(ctx);

    if (ctx->role == NIPC_WIN_SHM_ROLE_SERVER) {
        area_offset      = ctx->request_offset;
        area_capacity    = ctx->request_capacity;
        len_ptr          = &hdr->req_len;
        seq_ptr          = &hdr->req_seq;
        self_waiting_ptr = &hdr->req_server_waiting;
        peer_closed_ptr  = &hdr->req_client_closed;
        wait_event       = ctx->req_event;
        expected_seq     = ctx->local_req_seq + 1;
    } else {
        area_offset      = ctx->response_offset;
        area_capacity    = ctx->response_capacity;
        len_ptr          = &hdr->resp_len;
        seq_ptr          = &hdr->resp_seq;
        self_waiting_ptr = &hdr->resp_client_waiting;
        peer_closed_ptr  = &hdr->resp_server_closed;
        wait_event       = ctx->resp_event;
        expected_seq     = ctx->local_resp_seq + 1;
    }

    /* The copy ceiling is the smaller of the caller buffer and the
     * SHM area capacity. This prevents out-of-bounds reads even if
     * the peer writes a forged length value. */
    uint32_t max_copy = (buf_size < area_capacity) ? (uint32_t)buf_size : area_capacity;

    void *data_ptr = region_ptr(ctx, area_offset);

    /*
     * Phase 1: spin. Copy immediately upon observing the sequence advance.
     */
    bool observed = false;
    LONG mlen = 0;
    for (uint32_t i = 0; i < ctx->spin_tries; i++) {
        LONG64 cur = atomic_load_64(seq_ptr);
        if (cur >= expected_seq) {
            mlen = atomic_load_32(len_ptr);
            if (mlen > 0 && (size_t)mlen <= max_copy)
                memcpy(buf, data_ptr, (size_t)mlen);
            observed = true;
            break;
        }
        cpu_relax();
    }

    /* Phase 2: kernel wait or busy-wait (deadline-based retry for
     * spurious wakes — same pattern as POSIX SHM Phase H6 fix). */
    if (!observed) {
        if (ctx->profile == NIPC_WIN_SHM_PROFILE_HYBRID) {
            DWORD deadline_ms = (timeout_ms == 0) ? INFINITE : timeout_ms;
            DWORD start_tick = GetTickCount64() & 0xFFFFFFFF;

            for (;;) {
                InterlockedExchange(self_waiting_ptr, 1);
                MemoryBarrier();

                /* Recheck after setting flag to avoid race */
                LONG64 cur = atomic_load_64(seq_ptr);
                if (cur >= expected_seq) {
                    InterlockedExchange(self_waiting_ptr, 0);
                    break; /* data available */
                }

                /* Compute remaining wait time */
                DWORD wait_ms;
                if (deadline_ms == INFINITE) {
                    wait_ms = INFINITE;
                } else {
                    DWORD elapsed = (GetTickCount64() & 0xFFFFFFFF) - start_tick;
                    if (elapsed >= deadline_ms) {
                        InterlockedExchange(self_waiting_ptr, 0);
                        return NIPC_WIN_SHM_ERR_TIMEOUT;
                    }
                    wait_ms = deadline_ms - elapsed;
                }

                DWORD ret = WaitForSingleObject(wait_event, wait_ms);
                InterlockedExchange(self_waiting_ptr, 0);

                /* Check sequence — data may have arrived */
                cur = atomic_load_64(seq_ptr);
                if (cur >= expected_seq)
                    break; /* data available */

                /* No data — check peer close */
                if (atomic_load_32(peer_closed_ptr) != 0) {
                    cur = atomic_load_64(seq_ptr);
                    if (cur >= expected_seq)
                        break;
                    if (ctx->role == NIPC_WIN_SHM_ROLE_SERVER)
                        ctx->local_req_seq = expected_seq;
                    else
                        ctx->local_resp_seq = expected_seq;
                    return NIPC_WIN_SHM_ERR_DISCONNECTED;
                }

                /* Actual timeout (not spurious) */
                if (ret == WAIT_TIMEOUT)
                    return NIPC_WIN_SHM_ERR_TIMEOUT;

                /* Spurious wake — retry with remaining deadline */
            }

            /* Copy immediately after waking */
            mlen = atomic_load_32(len_ptr);
            if (mlen > 0 && (size_t)mlen <= max_copy)
                memcpy(buf, data_ptr, (size_t)mlen);

        } else {
            /* SHM_BUSYWAIT: spin indefinitely with periodic deadline checks */
            ULONGLONG start = GetTickCount64();
            for (;;) {
                LONG64 cur = atomic_load_64(seq_ptr);
                if (cur >= expected_seq) {
                    mlen = atomic_load_32(len_ptr);
                    if (mlen > 0 && (size_t)mlen <= max_copy)
                        memcpy(buf, data_ptr, (size_t)mlen);
                    break;
                }

                /* Periodic timeout check */
                if (timeout_ms > 0) {
                    ULONGLONG elapsed = GetTickCount64() - start;
                    if (elapsed >= timeout_ms)
                        return NIPC_WIN_SHM_ERR_TIMEOUT;
                }

                /* Check peer close */
                if (atomic_load_32(peer_closed_ptr) != 0) {
                    cur = atomic_load_64(seq_ptr);
                    if (cur >= expected_seq) {
                        mlen = atomic_load_32(len_ptr);
                        if (mlen > 0 && (size_t)mlen <= max_copy)
                            memcpy(buf, data_ptr, (size_t)mlen);
                        break;
                    }
                    if (ctx->role == NIPC_WIN_SHM_ROLE_SERVER)
                        ctx->local_req_seq = expected_seq;
                    else
                        ctx->local_resp_seq = expected_seq;
                    return NIPC_WIN_SHM_ERR_DISCONNECTED;
                }

                cpu_relax();
            }
        }
    }

    /* Message larger than caller buffer or area capacity */
    if ((size_t)mlen > max_copy) {
        *msg_len_out = (size_t)mlen;
        /* Still advance tracking -- message is consumed from SHM perspective */
        if (ctx->role == NIPC_WIN_SHM_ROLE_SERVER)
            ctx->local_req_seq = expected_seq;
        else
            ctx->local_resp_seq = expected_seq;
        return NIPC_WIN_SHM_ERR_MSG_TOO_LARGE;
    }

    /* mlen==0 after sequence advance indicates SHM corruption (send rejects 0-length) */
    if (mlen == 0) {
        if (ctx->role == NIPC_WIN_SHM_ROLE_SERVER)
            ctx->local_req_seq = expected_seq;
        else
            ctx->local_resp_seq = expected_seq;
        *msg_len_out = 0;
        return NIPC_WIN_SHM_ERR_BAD_HEADER;
    }

    *msg_len_out = (size_t)mlen;

    /* Advance local tracking */
    if (ctx->role == NIPC_WIN_SHM_ROLE_SERVER)
        ctx->local_req_seq = expected_seq;
    else
        ctx->local_resp_seq = expected_seq;

    return NIPC_WIN_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Stale cleanup (no-op on Windows)                                   */
/* ------------------------------------------------------------------ */

void nipc_win_shm_cleanup_stale(const char *run_dir, const char *service_name)
{
    /* Windows kernel objects are reference-counted and auto-cleaned
     * when all handles close. No filesystem artifacts to scan. */
    (void)run_dir;
    (void)service_name;
}

#endif /* _WIN32 || __MSYS__ */
