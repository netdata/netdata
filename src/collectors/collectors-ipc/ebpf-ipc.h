// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_IPC_H
#define NETDATA_EBPF_IPC_H 1

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

#include "libnetdata/libnetdata.h"
#include <fcntl.h>
#include <sys/stat.h>
#include <semaphore.h>

#ifdef __cplusplus
extern "C" {
#endif

#include <stdlib.h>
#include <stdio.h>
#include <stdint.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>
#ifdef LIBBPF_DEPRECATED
#include <bpf/btf.h>
#include <linux/btf.h>
#endif

typedef struct ebpf_user_mem_stat {
    uint32_t total;
    uint32_t current;
} ebpf_user_mem_stat_t;

// ----------------------------------------------------------------------------
// Enumeration used to identify threads with eBPF PIDs
enum ebpf_pids_index {
    NETDATA_EBPF_PIDS_PROCESS_IDX,
    NETDATA_EBPF_PIDS_SOCKET_IDX,
    NETDATA_EBPF_PIDS_CACHESTAT_IDX,
    NETDATA_EBPF_PIDS_DCSTAT_IDX,
    NETDATA_EBPF_PIDS_SWAP_IDX,
    NETDATA_EBPF_PIDS_VFS_IDX,
    NETDATA_EBPF_PIDS_FD_IDX,
    NETDATA_EBPF_PIDS_SHM_IDX,

    NETDATA_EBPF_PIDS_PROC_FILE,
    NETDATA_EBPF_PIDS_END_IDX
};

// ----------------------------------------------------------------------------
// Structures used to read data from kernel ring
typedef struct ebpf_process_stat {
    uint64_t ct;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    uint32_t tgid;
    uint32_t pid;

    //Counter
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t create_process;
    uint32_t create_thread;

    //Counter
    uint32_t task_err;
} ebpf_process_stat_t;

typedef struct ebpf_publish_process {
    uint64_t ct;

    //Counter
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t create_process;
    uint32_t create_thread;

    //Counter
    uint32_t task_err;
} ebpf_publish_process_t;

typedef struct ebpf_socket_publish_apps {
    // Data read
    uint64_t bytes_sent;             // Bytes sent
    uint64_t bytes_received;         // Bytes received
    uint64_t call_tcp_sent;          // Number of times tcp_sendmsg was called
    uint64_t call_tcp_received;      // Number of times tcp_cleanup_rbuf was called
    uint64_t retransmit;             // Number of times tcp_retransmit was called
    uint64_t call_udp_sent;          // Number of times udp_sendmsg was called
    uint64_t call_udp_received;      // Number of times udp_recvmsg was called
    uint64_t call_close;             // Number of times tcp_close was called
    uint64_t call_tcp_v4_connection; // Number of times tcp_v4_connect was called
    uint64_t call_tcp_v6_connection; // Number of times tcp_v6_connect was called
} ebpf_socket_publish_apps_t;

typedef struct netdata_socket {
    char name[TASK_COMM_LEN];

    // Timestamp
    uint64_t first_timestamp;
    uint64_t current_timestamp;
    // Socket additional info
    uint16_t protocol;
    uint16_t family;
    uint32_t external_origin;
    struct {
        uint32_t call_tcp_sent;
        uint32_t call_tcp_received;
        uint64_t tcp_bytes_sent;
        uint64_t tcp_bytes_received;
        uint32_t close;      //It is never used with UDP
        uint32_t retransmit; //It is never used with UDP
        uint32_t ipv4_connect;
        uint32_t ipv6_connect;
        uint32_t state; // We do not have charts for it, because we are using network viewer plugin
    } tcp;

    struct {
        uint32_t call_udp_sent;
        uint32_t call_udp_received;
        uint64_t udp_bytes_sent;
        uint64_t udp_bytes_received;
    } udp;
} netdata_socket_t;

typedef struct netdata_cachestat_pid {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    uint32_t add_to_page_cache_lru;
    uint32_t mark_page_accessed;
    uint32_t account_page_dirtied;
    uint32_t mark_buffer_dirty;
} netdata_cachestat_pid_t;

typedef struct netdata_cachestat {
    uint32_t add_to_page_cache_lru;
    uint32_t mark_page_accessed;
    uint32_t account_page_dirtied;
    uint32_t mark_buffer_dirty;
} netdata_cachestat_t;

typedef struct netdata_publish_cachestat {
    uint64_t ct;

    long long ratio;
    long long dirty;
    long long hit;
    long long miss;

    netdata_cachestat_t current;
    netdata_cachestat_t prev;
} netdata_publish_cachestat_t;

typedef struct netdata_publish_dcstat_pid {
    uint64_t cache_access;
    uint32_t file_system;
    uint32_t not_found;
} netdata_publish_dcstat_pid_t;

typedef struct netdata_publish_dcstat {
    uint64_t ct;

    long long ratio;
    long long cache_access;

    netdata_publish_dcstat_pid_t curr;
    netdata_publish_dcstat_pid_t prev;
} netdata_publish_dcstat_t;

typedef struct netdata_dcstat_pid {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    uint32_t cache_access;
    uint32_t file_system;
    uint32_t not_found;
} netdata_dcstat_pid_t;

typedef struct __attribute__((packed)) netdata_publish_swap {
    uint64_t ct;

    uint32_t read;
    uint32_t write;
} netdata_publish_swap_t;

typedef struct netdata_ebpf_swap {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    uint32_t read;
    uint32_t write;
} netdata_ebpf_swap_t;

typedef struct netdata_publish_vfs {
    uint64_t ct;

    //Counter
    uint32_t write_call;
    uint32_t writev_call;
    uint32_t read_call;
    uint32_t readv_call;
    uint32_t unlink_call;
    uint32_t fsync_call;
    uint32_t open_call;
    uint32_t create_call;

    //Accumulator
    uint64_t write_bytes;
    uint64_t writev_bytes;
    uint64_t readv_bytes;
    uint64_t read_bytes;

    //Counter
    uint32_t write_err;
    uint32_t writev_err;
    uint32_t read_err;
    uint32_t readv_err;
    uint32_t unlink_err;
    uint32_t fsync_err;
    uint32_t open_err;
    uint32_t create_err;
} netdata_publish_vfs_t;

typedef struct netdata_ebpf_vfs {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    //Counter
    uint32_t write_call;
    uint32_t writev_call;
    uint32_t read_call;
    uint32_t readv_call;
    uint32_t unlink_call;
    uint32_t fsync_call;
    uint32_t open_call;
    uint32_t create_call;

    //Accumulator
    uint64_t write_bytes;
    uint64_t writev_bytes;
    uint64_t readv_bytes;
    uint64_t read_bytes;

    //Counter
    uint32_t write_err;
    uint32_t writev_err;
    uint32_t read_err;
    uint32_t readv_err;
    uint32_t unlink_err;
    uint32_t fsync_err;
    uint32_t open_err;
    uint32_t create_err;
} netdata_ebpf_vfs_t;

typedef struct netdata_publish_fd_stat {
    uint64_t ct;

    uint32_t open_call;  // Open syscalls (open and openat)
    uint32_t close_call; // Close syscall (close)

    // Errors
    uint32_t open_err;
    uint32_t close_err;
} netdata_publish_fd_stat_t;

typedef struct netdata_fd_stat {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    uint32_t open_call;  // Open syscalls (open and openat)
    uint32_t close_call; // Close syscall (close)

    // Errors
    uint32_t open_err;
    uint32_t close_err;
} netdata_fd_stat_t;

typedef struct netdata_publish_shm {
    uint64_t ct;

    uint32_t get;
    uint32_t at;
    uint32_t dt;
    uint32_t ctl;
} netdata_publish_shm_t;

typedef struct netdata_ebpf_shm {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[TASK_COMM_LEN];

    uint32_t get;
    uint32_t at;
    uint32_t dt;
    uint32_t ctl;
} netdata_ebpf_shm_t;

typedef struct netdata_ebpf_pid_stats {
    uint32_t threads;
    uint32_t pid;

    ebpf_publish_process_t process;
    ebpf_socket_publish_apps_t socket;
    netdata_publish_cachestat_t cachestat;
    netdata_publish_dcstat_t directory_cache;
    netdata_publish_swap_t swap;
    netdata_publish_vfs_t vfs;
    netdata_publish_fd_stat_t fd;
    netdata_publish_shm_t shm;
} netdata_ebpf_pid_stats_t;

// ----------------------------------------------------------------------------
// Helpers used during integration

#define NETDATA_EBPF_INTEGRATION_NAME "netdata_shm_integration_ebpf"
#define NETDATA_EBPF_SHM_INTEGRATION_NAME "/netdata_sem_integration_ebpf"

int netdata_integration_initialize_shm(size_t pids);
void netdata_integration_cleanup_shm();
netdata_ebpf_pid_stats_t *netdata_ebpf_get_shm_pointer_unsafe(uint32_t pid, enum ebpf_pids_index idx);
bool netdata_ebpf_reset_shm_pointer_unsafe(int fd, uint32_t pid, enum ebpf_pids_index idx);
void netdata_integration_current_ipc_data(ebpf_user_mem_stat_t *values);

extern sem_t *shm_mutex_ebpf_integration;
extern netdata_ebpf_pid_stats_t *integration_shm;

#ifdef __cplusplus
}
#endif

#endif //NETDATA_EBPF_IPC_H
