// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_EBPF_SHARED_PID_ROW_H
#define NETDATA_APPS_EBPF_SHARED_PID_ROW_H 1

#include <stdint.h>

#ifndef EBPF_MAX_COMPARE_NAME
#define EBPF_MAX_COMPARE_NAME 95
#endif

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

/* v3: live_count added at offset 16; header is now 24 bytes; entries start
 *     at offset 24 (still 8-byte aligned for the uint64_t fields in ebpf_pid_stat).
 * v2: update_every_s replaced _pad; header grew from 8 to 16 bytes.
 * Version suffix ensures old consumers never map the new layout at the wrong offset. */
#define NETDATA_EBPFGO_INTEGRATION_NAME "/netdata_shm_integration_ebpfgo_v3"
#define NETDATA_EBPFGO_SHM_INTEGRATION_NAME "/netdata_sem_integration_ebpfgo_v3"

/* SHM header written at byte-offset 0; the ebpf_pid_stat[] array follows
 * immediately.  sizeof == 24 so entries start on an 8-byte boundary, which
 * satisfies the alignment of the uint64_t fields inside ebpf_pid_stat.
 * Producers set flags, update_every_s, live_count, and last_publish_ut before
 * releasing the semaphore; consumers use them to determine which modules
 * contributed data this cycle, how many entries to copy (live_count), and
 * whether the payload is still live. */
struct ebpfgo_shm_header {
    uint32_t flags;           /* EBPFGO_SHM_FLAG_* bits set by the active publisher(s) */
    uint32_t update_every_s;  /* publish interval in seconds; 0 = unknown (old writer) */
    uint64_t last_publish_ut; /* CLOCK_MONOTONIC, usec; 0 means no live producer */
    uint32_t live_count;      /* entries written this cycle; reader copies only this many */
    uint32_t _reserved;       /* reserved for future use */
};

#define EBPFGO_SHM_FLAG_CACHESTAT 0x01u /* cachestat per-PID fields are valid */
#define EBPFGO_SHM_FLAG_SOCKET    0x02u /* socket per-PID fields are valid */

struct ebpf_cachestat {
    uint32_t add_to_page_cache_lru;
    uint32_t mark_page_accessed;
    uint32_t account_page_dirtied;
    uint32_t mark_buffer_dirty;
};

struct ebpf_publish_cachestat {
    uint64_t ct;
    int64_t ratio;
    int64_t dirty;
    int64_t hit;
    int64_t miss;
    struct ebpf_cachestat current;
    struct ebpf_cachestat prev;
};

struct ebpf_publish_dcstat_pid {
    uint64_t cache_access;
    uint64_t file_system;
    uint64_t not_found;
};

struct ebpf_publish_dcstat {
    uint64_t ct;
    int64_t ratio;
    int64_t cache_access;
    struct ebpf_publish_dcstat_pid curr;
    struct ebpf_publish_dcstat_pid prev;
};

struct ebpf_publish_fd_stat {
    uint64_t ct;
    uint32_t open_call;
    uint32_t close_call;
    uint32_t open_err;
    uint32_t close_err;
};

struct ebpf_process_stat {
    uint64_t ct;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];
    uint32_t tgid;
    uint32_t pid;
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t create_process;
    uint32_t create_thread;
    uint32_t task_err;
};

struct ebpf_publish_shm {
    uint64_t ct;
    uint32_t get;
    uint32_t at;
    uint32_t dt;
    uint32_t ctl;
};

struct ebpf_publish_swap {
    uint64_t ct;
    uint32_t read;
    uint32_t write;
};

struct ebpf_socket_publish_apps {
    uint64_t bytes_sent;
    uint64_t bytes_received;
    uint64_t call_tcp_sent;
    uint64_t call_tcp_received;
    uint64_t retransmit;
    uint64_t call_udp_sent;
    uint64_t call_udp_received;
    uint64_t call_close;
    uint64_t call_tcp_v4_connection;
    uint64_t call_tcp_v6_connection;
};

struct ebpf_publish_vfs {
    uint64_t ct;
    uint32_t write_call;
    uint32_t writev_call;
    uint32_t read_call;
    uint32_t readv_call;
    uint32_t unlink_call;
    uint32_t fsync_call;
    uint32_t open_call;
    uint32_t create_call;
    uint64_t write_bytes;
    uint64_t writev_bytes;
    uint64_t readv_bytes;
    uint64_t read_bytes;
    uint32_t write_err;
    uint32_t writev_err;
    uint32_t read_err;
    uint32_t readv_err;
    uint32_t unlink_err;
    uint32_t fsync_err;
    uint32_t open_err;
    uint32_t create_err;
};

struct ebpf_pid_stat {
    uint32_t pid;
    char comm[EBPF_MAX_COMPARE_NAME + 1];
    uint32_t ppid;
    struct ebpf_publish_cachestat cachestat;
    struct ebpf_publish_dcstat dc;
    struct ebpf_publish_fd_stat fd;
    struct ebpf_process_stat process;
    struct ebpf_publish_shm shm;
    struct ebpf_publish_swap swap;
    struct ebpf_socket_publish_apps socket;
    struct ebpf_publish_vfs vfs;
};

#ifdef __linux__
#include <errno.h>
#include <semaphore.h>
#include <stdbool.h>
#include <time.h>

/* Returns the current CLOCK_MONOTONIC timestamp in microseconds.
 * All SHM writers and readers MUST use this so that last_publish_ut
 * comparisons stay on the same clock.  Do NOT use now_monotonic_usec()
 * here: libnetdata resolves that to CLOCK_MONOTONIC_RAW, which drifts
 * relative to CLOCK_MONOTONIC on NTP-disciplined hosts. */
static inline uint64_t ebpfgo_shm_now_monotonic_usec(void)
{
    struct timespec ts;
    if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0)
        return 0;
    return (uint64_t)ts.tv_sec * 1000000ULL + (uint64_t)ts.tv_nsec / 1000ULL;
}

/* Timed semaphore acquire: 200 ms deadline, CLOCK_MONOTONIC.
 *
 * sem_timedwait requires a CLOCK_REALTIME absolute deadline; an NTP forward
 * step can push that deadline into the past, causing immediate ETIMEDOUT and
 * a spurious replace_generation.  We avoid that by measuring the deadline
 * with CLOCK_MONOTONIC and polling with sem_trywait + clock_nanosleep. */
static inline bool ebpfgo_shm_sem_wait(sem_t *sem)
{
    if (!sem || sem == SEM_FAILED) {
        errno = EINVAL;
        return false;
    }

    struct timespec deadline;
    if (clock_gettime(CLOCK_MONOTONIC, &deadline) == -1)
        return false;

    deadline.tv_nsec += 200 * 1000 * 1000;  /* +200 ms */
    if (deadline.tv_nsec >= 1000000000L) {
        deadline.tv_sec  += deadline.tv_nsec / 1000000000L;
        deadline.tv_nsec %= 1000000000L;
    }

    while (1) {
        if (sem_trywait(sem) == 0)
            return true;

        if (errno != EAGAIN)
            return false;

        struct timespec now;
        if (clock_gettime(CLOCK_MONOTONIC, &now) == -1)
            return false;

        if (now.tv_sec > deadline.tv_sec ||
            (now.tv_sec == deadline.tv_sec && now.tv_nsec >= deadline.tv_nsec))
            return false;  /* timed out */

        struct timespec slp = { .tv_sec = 0, .tv_nsec = 1000000L };  /* 1 ms */
        clock_nanosleep(CLOCK_MONOTONIC, 0, &slp, NULL);
    }
}
#endif /* __linux__ */

#endif
