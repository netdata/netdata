/*
 * netipc_shm.c - L1 POSIX SHM transport (Linux only).
 *
 * Shared memory data plane with spin+futex synchronization.
 * Uses the same wire envelope as UDS -- higher levels are unaware
 * of the underlying transport.
 */

#include "netipc/netipc_shm.h"

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <inttypes.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/syscall.h>
#include <sys/types.h>

#include <linux/futex.h>

/* ------------------------------------------------------------------ */
/*  Internal helpers                                                   */
/* ------------------------------------------------------------------ */

/* Round up to 64-byte alignment. */
static inline uint32_t align64(uint32_t v)
{
    return (v + (NIPC_SHM_REGION_ALIGNMENT - 1)) & ~(uint32_t)(NIPC_SHM_REGION_ALIGNMENT - 1);
}

/* Validate service_name: only [a-zA-Z0-9._-], non-empty, not "." or "..". */
static int validate_service_name(const char *name)
{
    if (!name || name[0] == '\0')
        return -1;

    /* Reject "." and ".." */
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

/* Build per-session SHM file path: {run_dir}/{service_name}-{session_id:016x}.ipcshm */
static int build_shm_path(char *dst, size_t dst_len,
                           const char *run_dir, const char *service_name,
                           uint64_t session_id)
{
    if (validate_service_name(service_name) < 0)
        return -2; /* invalid service name */

    int n = snprintf(dst, dst_len, "%s/%s-%016" PRIx64 ".ipcshm",
                     run_dir, service_name, session_id);
    if (n < 0 || (size_t)n >= dst_len)
        return -1;
    return 0;
}

/* Thin wrapper around the futex syscall. */
static int futex_wake(uint32_t *addr, int count)
{
    return (int)syscall(SYS_futex, addr, FUTEX_WAKE, count, NULL, NULL, 0);
}

static int futex_wait(uint32_t *addr, uint32_t expected,
                      const struct timespec *timeout)
{
    return (int)syscall(SYS_futex, addr, FUTEX_WAIT, expected, timeout, NULL, 0);
}

/* CPU pause hint for spin loops. */
static inline void cpu_relax(void)
{
#if defined(__x86_64__) || defined(__i386__)
    __builtin_ia32_pause();
#elif defined(__aarch64__)
    __asm__ volatile("yield" ::: "memory");
#else
    /* generic: compiler barrier */
    __atomic_signal_fence(__ATOMIC_SEQ_CST);
#endif
}

/* Check if a PID is alive (without sending a signal). */
static bool pid_alive(pid_t pid)
{
    if (pid <= 0)
        return false;
    return kill(pid, 0) == 0 || errno == EPERM;
}

/* Pointer into the mapped region at a byte offset. */
static inline void *region_ptr(const nipc_shm_ctx_t *ctx, uint32_t offset)
{
    return (uint8_t *)ctx->base + offset;
}

/* Pointer to the header (always at offset 0). Used for non-atomic fields. */
static inline nipc_shm_region_header_t *region_hdr(const nipc_shm_ctx_t *ctx)
{
    return (nipc_shm_region_header_t *)ctx->base;
}

/*
 * Byte-offset accessors for atomic fields. These avoid taking the
 * address of a packed struct member, which GCC warns about.
 */
#define SHM_OFF_REQ_SEQ    32
#define SHM_OFF_RESP_SEQ   40
#define SHM_OFF_REQ_LEN    48
#define SHM_OFF_RESP_LEN   52
#define SHM_OFF_REQ_SIGNAL 56
#define SHM_OFF_RESP_SIGNAL 60

static inline uint64_t *shm_seq_ptr(void *base, int offset)
{
    return (uint64_t *)((uint8_t *)base + offset);
}

static inline uint32_t *shm_u32_ptr(void *base, int offset)
{
    return (uint32_t *)((uint8_t *)base + offset);
}

/* ------------------------------------------------------------------ */
/*  Stale region recovery                                              */
/* ------------------------------------------------------------------ */

/*
 * Returns:
 *   0  = stale (unlinked)
 *  +1  = live server
 *  -1  = doesn't exist
 *  -2  = exists but undersized / invalid (treated as stale, unlinked)
 */
static int unlink_same_file(const char *path, const struct stat *expected)
{
    struct stat current;
    if (lstat(path, &current) != 0)
        return (errno == ENOENT) ? 0 : -1;

    if (current.st_dev != expected->st_dev || current.st_ino != expected->st_ino)
        return -1;

    if (unlink(path) == 0 || errno == ENOENT)
        return 0;

    return -1;
}

static int check_shm_stale(const char *path)
{
#ifdef O_NOFOLLOW
    int fd = open(path, O_RDONLY | O_NOFOLLOW);
#else
    int fd = open(path, O_RDONLY);
#endif
    if (fd < 0) {
        if (errno == ENOENT)
            return -1;
        /* Symlinks, permission failures, and other ambiguous path states are
         * treated as live so stale recovery cannot remove an unsafe path. */
        return 1;
    }

    struct stat st;
    if (fstat(fd, &st) != 0) {
        close(fd);
        return 1;
    }

    if (!S_ISREG(st.st_mode)) {
        close(fd);
        return 1;
    }

    /* Must be at least header-sized to inspect. */
    if (st.st_size < (off_t)NIPC_SHM_HEADER_LEN) {
        close(fd);
        return (unlink_same_file(path, &st) == 0) ? -2 : 1;
    }

    void *map = mmap(NULL, NIPC_SHM_HEADER_LEN, PROT_READ, MAP_SHARED, fd, 0);
    close(fd);
    if (map == MAP_FAILED) {
        return (unlink_same_file(path, &st) == 0) ? -2 : 1;
    }

    const nipc_shm_region_header_t *hdr = (const nipc_shm_region_header_t *)map;

    /* Validate magic first. */
    if (hdr->magic != NIPC_SHM_REGION_MAGIC) {
        munmap(map, NIPC_SHM_HEADER_LEN);
        return (unlink_same_file(path, &st) == 0) ? -2 : 1;
    }

    int32_t owner = hdr->owner_pid;
    uint32_t gen   = hdr->owner_generation;
    munmap(map, NIPC_SHM_HEADER_LEN);

    if (pid_alive((pid_t)owner) && gen != 0) {
        return 1; /* live: PID is alive and generation is valid */
    }

    /* Dead owner or zero generation (uninitialized/legacy) — stale */
    return (unlink_same_file(path, &st) == 0) ? 0 : 1;
}

/* ------------------------------------------------------------------ */
/*  Server: create                                                     */
/* ------------------------------------------------------------------ */

nipc_shm_error_t nipc_shm_server_create(const char *run_dir,
                                          const char *service_name,
                                          uint64_t session_id,
                                          uint32_t req_capacity,
                                          uint32_t resp_capacity,
                                          nipc_shm_ctx_t *out)
{
    if (!run_dir || !service_name || !out)
        return NIPC_SHM_ERR_BAD_PARAM;

    memset(out, 0, sizeof(*out));
    out->fd = -1;

    /* Build per-session path (validates service_name) */
    char path[256];
    int path_rc = build_shm_path(path, sizeof(path), run_dir, service_name,
                                 session_id);
    if (path_rc == -2)
        return NIPC_SHM_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_SHM_ERR_PATH_TOO_LONG;

    /* Round capacities up to alignment. */
    req_capacity  = align64(req_capacity);
    resp_capacity = align64(resp_capacity);

    uint32_t req_off  = align64(NIPC_SHM_HEADER_LEN);
    /* Guard against uint32 overflow before computing resp_off */
    if (req_capacity > UINT32_MAX - req_off)
        return NIPC_SHM_ERR_BAD_PARAM;
    uint32_t resp_off = align64(req_off + req_capacity);
    if (resp_capacity > UINT32_MAX - resp_off ||
        (size_t)resp_off > SIZE_MAX - (size_t)resp_capacity)
        return NIPC_SHM_ERR_BAD_PARAM;
    size_t region_size = (size_t)resp_off + resp_capacity;

    /* Try O_EXCL create first (fast path, no stale check needed). */
    int fd = open(path, O_RDWR | O_CREAT | O_EXCL, 0600);

    /* If O_EXCL failed because file exists, do stale recovery and retry. */
    if (fd < 0 && errno == EEXIST) {
        int stale = check_shm_stale(path);
        if (stale == 1)
            return NIPC_SHM_ERR_ADDR_IN_USE;
        /* Stale file was unlinked, retry create */
        fd = open(path, O_RDWR | O_CREAT | O_EXCL, 0600);
    }
    if (fd < 0)
        return NIPC_SHM_ERR_OPEN;

    if (ftruncate(fd, (off_t)region_size) < 0) {
        close(fd);
        unlink(path);
        return NIPC_SHM_ERR_TRUNCATE;
    }

    void *map = mmap(NULL, region_size, PROT_READ | PROT_WRITE,
                     MAP_SHARED, fd, 0);
    if (map == MAP_FAILED) {
        close(fd);
        unlink(path);
        return NIPC_SHM_ERR_MMAP;
    }

    /* Zero the region first to init all atomics to 0. */
    memset(map, 0, region_size);

    /* Write header. */
    nipc_shm_region_header_t *hdr = (nipc_shm_region_header_t *)map;
    hdr->magic             = NIPC_SHM_REGION_MAGIC;
    hdr->version           = NIPC_SHM_REGION_VERSION;
    hdr->header_len        = NIPC_SHM_HEADER_LEN;
    hdr->owner_pid         = (int32_t)getpid();

    /* Use a time-based generation to detect PID reuse across restarts. */
    {
        struct timespec ts;
        clock_gettime(CLOCK_MONOTONIC, &ts);
        hdr->owner_generation = (uint32_t)(ts.tv_sec ^ (ts.tv_nsec >> 10));
    }
    hdr->request_offset    = req_off;
    hdr->request_capacity  = req_capacity;
    hdr->response_offset   = resp_off;
    hdr->response_capacity = resp_capacity;

    /* Ensure header writes are visible before clients read. */
    __atomic_thread_fence(__ATOMIC_RELEASE);

    /* Fill context. */
    out->role              = NIPC_SHM_ROLE_SERVER;
    out->fd                = fd;
    out->base              = map;
    out->region_size       = region_size;
    out->request_offset    = req_off;
    out->request_capacity  = req_capacity;
    out->response_offset   = resp_off;
    out->response_capacity = resp_capacity;
    out->local_req_seq     = 0;
    out->local_resp_seq    = 0;
    out->spin_tries        = NIPC_SHM_DEFAULT_SPIN;
    out->owner_generation  = hdr->owner_generation;
    strncpy(out->path, path, sizeof(out->path) - 1);
    out->path[sizeof(out->path) - 1] = '\0';

    return NIPC_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Server: destroy                                                    */
/* ------------------------------------------------------------------ */

void nipc_shm_destroy(nipc_shm_ctx_t *ctx)
{
    if (!ctx)
        return;

    if (ctx->base && ctx->base != MAP_FAILED) {
        munmap(ctx->base, ctx->region_size);
        ctx->base = NULL;
    }

    if (ctx->fd >= 0) {
        close(ctx->fd);
        ctx->fd = -1;
    }

    if (ctx->path[0]) {
        unlink(ctx->path);
        ctx->path[0] = '\0';
    }

    ctx->region_size = 0;
}

/* ------------------------------------------------------------------ */
/*  Client: attach                                                     */
/* ------------------------------------------------------------------ */

nipc_shm_error_t nipc_shm_client_attach(const char *run_dir,
                                          const char *service_name,
                                          uint64_t session_id,
                                          nipc_shm_ctx_t *out)
{
    if (!run_dir || !service_name || !out)
        return NIPC_SHM_ERR_BAD_PARAM;

    memset(out, 0, sizeof(*out));
    out->fd = -1;

    char path[256];
    int path_rc = build_shm_path(path, sizeof(path), run_dir, service_name,
                                 session_id);
    if (path_rc == -2)
        return NIPC_SHM_ERR_BAD_PARAM;
    if (path_rc < 0)
        return NIPC_SHM_ERR_PATH_TOO_LONG;

    /* Open the file. */
    int fd = open(path, O_RDWR);
    if (fd < 0)
        return NIPC_SHM_ERR_OPEN;

    /* Check file size. */
    struct stat st;
    if (fstat(fd, &st) < 0) {
        close(fd);
        return NIPC_SHM_ERR_OPEN;
    }

    if ((size_t)st.st_size < NIPC_SHM_HEADER_LEN) {
        close(fd);
        return NIPC_SHM_ERR_NOT_READY;
    }

    /* Map the region. */
    size_t file_size = (size_t)st.st_size;
    void *map = mmap(NULL, file_size, PROT_READ | PROT_WRITE,
                     MAP_SHARED, fd, 0);
    if (map == MAP_FAILED) {
        close(fd);
        return NIPC_SHM_ERR_MMAP;
    }

    /* Acquire fence so we see the server's header writes. */
    __atomic_thread_fence(__ATOMIC_ACQUIRE);

    const nipc_shm_region_header_t *hdr =
        (const nipc_shm_region_header_t *)map;

    /* Validate header. */
    if (hdr->magic != NIPC_SHM_REGION_MAGIC) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_BAD_MAGIC;
    }

    if (hdr->version != NIPC_SHM_REGION_VERSION) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_BAD_VERSION;
    }

    if (hdr->header_len != NIPC_SHM_HEADER_LEN) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_BAD_HEADER;
    }

    size_t header_end = align64((uint32_t)NIPC_SHM_HEADER_LEN);
    if (hdr->request_offset < header_end ||
        hdr->request_capacity == 0 ||
        hdr->response_offset < header_end ||
        hdr->response_capacity == 0) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_NOT_READY;
    }

    /* Guard against uint32 overflow in request_offset + request_capacity */
    if (hdr->request_offset > UINT32_MAX - hdr->request_capacity) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_BAD_SIZE;
    }

    if ((hdr->request_offset % NIPC_SHM_REGION_ALIGNMENT) != 0 ||
        (hdr->request_capacity % NIPC_SHM_REGION_ALIGNMENT) != 0 ||
        (hdr->response_offset % NIPC_SHM_REGION_ALIGNMENT) != 0 ||
        (hdr->response_capacity % NIPC_SHM_REGION_ALIGNMENT) != 0 ||
        hdr->response_offset < align64(hdr->request_offset + hdr->request_capacity)) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_BAD_SIZE;
    }

    /* Validate region is large enough for the declared areas. */
    size_t needed = 0;
    size_t req_end  = (size_t)hdr->request_offset + hdr->request_capacity;
    size_t resp_end = (size_t)hdr->response_offset + hdr->response_capacity;
    needed = req_end > resp_end ? req_end : resp_end;

    if (file_size < needed) {
        munmap(map, file_size);
        close(fd);
        return NIPC_SHM_ERR_BAD_SIZE;
    }

    /* Read the current sequence numbers so we don't see stale data. */
    uint64_t cur_req_seq  = __atomic_load_n(
        (uint64_t *)((uint8_t *)map + offsetof(nipc_shm_region_header_t, req_seq)),
        __ATOMIC_ACQUIRE);
    uint64_t cur_resp_seq = __atomic_load_n(
        (uint64_t *)((uint8_t *)map + offsetof(nipc_shm_region_header_t, resp_seq)),
        __ATOMIC_ACQUIRE);

    /* Fill context. */
    out->role              = NIPC_SHM_ROLE_CLIENT;
    out->fd                = fd;
    out->base              = map;
    out->region_size       = file_size;
    out->request_offset    = hdr->request_offset;
    out->request_capacity  = hdr->request_capacity;
    out->response_offset   = hdr->response_offset;
    out->response_capacity = hdr->response_capacity;
    out->local_req_seq     = cur_req_seq;
    out->local_resp_seq    = cur_resp_seq;
    out->spin_tries        = NIPC_SHM_DEFAULT_SPIN;
    out->owner_generation  = hdr->owner_generation;
    strncpy(out->path, path, sizeof(out->path) - 1);
    out->path[sizeof(out->path) - 1] = '\0';

    return NIPC_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Client: close (no unlink)                                          */
/* ------------------------------------------------------------------ */

void nipc_shm_close(nipc_shm_ctx_t *ctx)
{
    if (!ctx)
        return;

    if (ctx->base && ctx->base != MAP_FAILED) {
        munmap(ctx->base, ctx->region_size);
        ctx->base = NULL;
    }

    if (ctx->fd >= 0) {
        close(ctx->fd);
        ctx->fd = -1;
    }

    ctx->region_size = 0;
}

/* ------------------------------------------------------------------ */
/*  Data plane: send                                                   */
/* ------------------------------------------------------------------ */

nipc_shm_error_t nipc_shm_send(nipc_shm_ctx_t *ctx,
                                 const void *msg, size_t msg_len)
{
    if (!ctx || !ctx->base || !msg || msg_len == 0)
        return NIPC_SHM_ERR_BAD_PARAM;

    /*
     * Client writes to the request area; server writes to the response
     * area. The direction is determined by role.
     */
    uint32_t area_offset;
    uint32_t area_capacity;
    int seq_off, len_off, sig_off;

    if (ctx->role == NIPC_SHM_ROLE_CLIENT) {
        area_offset   = ctx->request_offset;
        area_capacity = ctx->request_capacity;
        seq_off       = SHM_OFF_REQ_SEQ;
        len_off       = SHM_OFF_REQ_LEN;
        sig_off       = SHM_OFF_REQ_SIGNAL;
    } else {
        area_offset   = ctx->response_offset;
        area_capacity = ctx->response_capacity;
        seq_off       = SHM_OFF_RESP_SEQ;
        len_off       = SHM_OFF_RESP_LEN;
        sig_off       = SHM_OFF_RESP_SIGNAL;
    }

    if (msg_len > area_capacity)
        return NIPC_SHM_ERR_MSG_TOO_LARGE;

    uint64_t *seq_ptr    = shm_seq_ptr(ctx->base, seq_off);
    uint32_t *len_ptr    = shm_u32_ptr(ctx->base, len_off);
    uint32_t *signal_ptr = shm_u32_ptr(ctx->base, sig_off);

    /* 1. Write message data into the area. */
    memcpy(region_ptr(ctx, area_offset), msg, msg_len);

    /* 2. Store message length (release). */
    __atomic_store_n(len_ptr, (uint32_t)msg_len, __ATOMIC_RELEASE);

    /* 3. Increment sequence number (release) to publish. */
    __atomic_add_fetch(seq_ptr, 1, __ATOMIC_RELEASE);

    /* 4. Wake the peer via futex. */
    __atomic_add_fetch(signal_ptr, 1, __ATOMIC_RELEASE);
    futex_wake(signal_ptr, 1);

    /* Track locally. */
    if (ctx->role == NIPC_SHM_ROLE_CLIENT)
        ctx->local_req_seq++;
    else
        ctx->local_resp_seq++;

    return NIPC_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Data plane: receive                                                */
/* ------------------------------------------------------------------ */

nipc_shm_error_t nipc_shm_receive(nipc_shm_ctx_t *ctx,
                                    void *buf,
                                    size_t buf_size,
                                    size_t *msg_len_out,
                                    uint32_t timeout_ms)
{
    if (!ctx || !ctx->base || !buf || !msg_len_out || buf_size == 0)
        return NIPC_SHM_ERR_BAD_PARAM;

    /*
     * Server reads from the request area; client reads from the
     * response area.
     */
    uint32_t area_offset;
    uint32_t area_capacity;
    int seq_off, len_off, sig_off;
    uint64_t expected_seq;

    if (ctx->role == NIPC_SHM_ROLE_SERVER) {
        area_offset   = ctx->request_offset;
        area_capacity = ctx->request_capacity;
        seq_off       = SHM_OFF_REQ_SEQ;
        len_off       = SHM_OFF_REQ_LEN;
        sig_off       = SHM_OFF_REQ_SIGNAL;
        expected_seq  = ctx->local_req_seq + 1;
    } else {
        area_offset   = ctx->response_offset;
        area_capacity = ctx->response_capacity;
        seq_off       = SHM_OFF_RESP_SEQ;
        len_off       = SHM_OFF_RESP_LEN;
        sig_off       = SHM_OFF_RESP_SIGNAL;
        expected_seq  = ctx->local_resp_seq + 1;
    }

    /* The copy ceiling is the smaller of the caller buffer and the
     * SHM area capacity. This prevents out-of-bounds reads even if
     * the peer writes a forged length value. */
    uint32_t max_copy = (buf_size < area_capacity) ? (uint32_t)buf_size : area_capacity;

    uint64_t *seq_ptr    = shm_seq_ptr(ctx->base, seq_off);
    uint32_t *len_ptr    = shm_u32_ptr(ctx->base, len_off);
    uint32_t *signal_ptr = shm_u32_ptr(ctx->base, sig_off);

    void *data_ptr = region_ptr(ctx, area_offset);

    /*
     * Phase 1: spin. On detecting the sequence advance, immediately
     * copy the message into the caller's buffer while still in the
     * same iteration. The shared area can be overwritten by the peer
     * within nanoseconds of the sequence advancing.
     */
    bool observed = false;
    uint32_t mlen = 0;
    for (uint32_t i = 0; i < ctx->spin_tries; i++) {
        uint64_t cur = __atomic_load_n(seq_ptr, __ATOMIC_ACQUIRE);
        if (cur >= expected_seq) {
            mlen = __atomic_load_n(len_ptr, __ATOMIC_ACQUIRE);
            if (mlen > 0 && mlen <= max_copy)
                memcpy(buf, data_ptr, mlen);
            observed = true;
            break;
        }
        cpu_relax();
    }

    /* Phase 2: futex wait (if spinning didn't observe the advance).
     *
     * The futex_wait loop handles spurious wakeups (EAGAIN when the
     * signal word changed between our read and the syscall, or EINTR
     * from signal delivery).  We compute a wall-clock deadline so the
     * total wait never exceeds timeout_ms regardless of retries. */
    if (!observed) {
        uint64_t deadline_ns = 0; /* 0 = no timeout */
        if (timeout_ms > 0) {
            struct timespec now_ts;
            clock_gettime(CLOCK_MONOTONIC, &now_ts);
            deadline_ns = (uint64_t)now_ts.tv_sec * 1000000000ull
                        + (uint64_t)now_ts.tv_nsec
                        + (uint64_t)timeout_ms * 1000000ull;
        }

        for (;;) {
            uint32_t sig_val = __atomic_load_n(signal_ptr, __ATOMIC_ACQUIRE);

            uint64_t cur = __atomic_load_n(seq_ptr, __ATOMIC_ACQUIRE);
            if (cur >= expected_seq)
                break; /* response arrived */

            /* Compute remaining timeout for this futex_wait call. */
            struct timespec ts;
            struct timespec *tsp = NULL;
            if (deadline_ns > 0) {
                struct timespec now_ts;
                clock_gettime(CLOCK_MONOTONIC, &now_ts);
                uint64_t now_val = (uint64_t)now_ts.tv_sec * 1000000000ull
                                 + (uint64_t)now_ts.tv_nsec;
                if (now_val >= deadline_ns)
                    return NIPC_SHM_ERR_TIMEOUT;

                uint64_t remain = deadline_ns - now_val;
                ts.tv_sec  = (time_t)(remain / 1000000000ull);
                ts.tv_nsec = (long)(remain % 1000000000ull);
                tsp = &ts;
            }

            int ret = futex_wait(signal_ptr, sig_val, tsp);
            if (ret < 0 && errno == ETIMEDOUT)
                return NIPC_SHM_ERR_TIMEOUT;

            /* EAGAIN (value changed) or EINTR (signal): re-check seq. */
        }

        /* Copy immediately after observing the sequence advance. */
        mlen = __atomic_load_n(len_ptr, __ATOMIC_ACQUIRE);
        if (mlen > 0 && mlen <= max_copy)
            memcpy(buf, data_ptr, mlen);
    }

    /* Message larger than caller buffer or area capacity */
    if (mlen > max_copy) {
        *msg_len_out = mlen;
        /* Still advance tracking -- message is consumed from SHM perspective */
        if (ctx->role == NIPC_SHM_ROLE_SERVER)
            ctx->local_req_seq = expected_seq;
        else
            ctx->local_resp_seq = expected_seq;
        return NIPC_SHM_ERR_MSG_TOO_LARGE;
    }

    /* mlen==0 after a sequence advance indicates corruption (send rejects 0-length) */
    if (mlen == 0) {
        if (ctx->role == NIPC_SHM_ROLE_SERVER)
            ctx->local_req_seq = expected_seq;
        else
            ctx->local_resp_seq = expected_seq;
        *msg_len_out = 0;
        return NIPC_SHM_ERR_BAD_HEADER;
    }

    *msg_len_out = mlen;

    /* Advance local tracking. */
    if (ctx->role == NIPC_SHM_ROLE_SERVER)
        ctx->local_req_seq = expected_seq;
    else
        ctx->local_resp_seq = expected_seq;

    return NIPC_SHM_OK;
}

/* ------------------------------------------------------------------ */
/*  Utility                                                            */
/* ------------------------------------------------------------------ */

bool nipc_shm_owner_alive(const nipc_shm_ctx_t *ctx)
{
    if (!ctx || !ctx->base)
        return false;

    const nipc_shm_region_header_t *hdr =
        (const nipc_shm_region_header_t *)ctx->base;

    if (!pid_alive((pid_t)hdr->owner_pid))
        return false;

    /* PID is alive; verify generation matches to detect PID reuse.
     * If owner_generation is 0, skip the check (legacy region). */
    if (ctx->owner_generation != 0 &&
        hdr->owner_generation != ctx->owner_generation)
        return false;

    return true;
}

/* ------------------------------------------------------------------ */
/*  Stale cleanup on server startup                                    */
/* ------------------------------------------------------------------ */

void nipc_shm_cleanup_stale(const char *run_dir, const char *service_name)
{
    if (!run_dir || !service_name)
        return;

    if (validate_service_name(service_name) < 0)
        return;

    /* Build the prefix to match: "{service_name}-" */
    char prefix[256];
    int pn = snprintf(prefix, sizeof(prefix), "%s-", service_name);
    if (pn < 0 || (size_t)pn >= sizeof(prefix))
        return;

    size_t prefix_len = (size_t)pn;
    const char *suffix = ".ipcshm";
    size_t suffix_len = strlen(suffix);

    DIR *dir = opendir(run_dir);
    if (!dir)
        return;

    struct dirent *ent;
    while ((ent = readdir(dir)) != NULL) {
        size_t nlen = strlen(ent->d_name);

        /* Must start with "{service_name}-" and end with ".ipcshm" */
        if (nlen <= prefix_len + suffix_len)
            continue;
        if (strncmp(ent->d_name, prefix, prefix_len) != 0)
            continue;
        if (strcmp(ent->d_name + nlen - suffix_len, suffix) != 0)
            continue;

        /* Build full path and check if stale */
        char path[512];
        int n = snprintf(path, sizeof(path), "%s/%s", run_dir, ent->d_name);
        if (n < 0 || (size_t)n >= sizeof(path))
            continue;

        /* check_shm_stale unlinks stale files and returns:
         *   0 = stale (unlinked), +1 = live, -1 = gone, -2 = invalid (unlinked) */
        check_shm_stale(path);
    }

    closedir(dir);
}
