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

#define NETDATA_EBPFGO_INTEGRATION_NAME "netdata_shm_integration_ebpfgo"
#define NETDATA_EBPFGO_SHM_INTEGRATION_NAME "/netdata_sem_integration_ebpfgo"

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

#endif
