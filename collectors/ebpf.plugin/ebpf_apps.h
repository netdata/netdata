// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_APPS_H
#define NETDATA_EBPF_APPS_H 1

#include "libnetdata/locks/locks.h"
#include "libnetdata/avl/avl.h"
#include "libnetdata/clocks/clocks.h"
#include "libnetdata/config/appconfig.h"
#include "libnetdata/ebpf/ebpf.h"

#define NETDATA_APPS_FAMILY "apps"
#define NETDATA_APPS_FILE_GROUP "file_access"
#define NETDATA_APPS_FILE_CGROUP_GROUP "file_access (eBPF)"
#define NETDATA_APPS_PROCESS_GROUP "process (eBPF)"
#define NETDATA_APPS_NET_GROUP "net"
#define NETDATA_APPS_IPC_SHM_GROUP "ipc shm (eBPF)"

#include "ebpf_process.h"
#include "ebpf_dcstat.h"
#include "ebpf_disk.h"
#include "ebpf_fd.h"
#include "ebpf_filesystem.h"
#include "ebpf_hardirq.h"
#include "ebpf_cachestat.h"
#include "ebpf_mdflush.h"
#include "ebpf_mount.h"
#include "ebpf_oomkill.h"
#include "ebpf_shm.h"
#include "ebpf_socket.h"
#include "ebpf_softirq.h"
#include "ebpf_sync.h"
#include "ebpf_swap.h"
#include "ebpf_vfs.h"

#define EBPF_MAX_COMPARE_NAME 100
#define EBPF_MAX_NAME 100

// ----------------------------------------------------------------------------
// pid_stat
//
struct ebpf_target {
    char compare[EBPF_MAX_COMPARE_NAME + 1];
    uint32_t comparehash;
    size_t comparelen;

    char id[EBPF_MAX_NAME + 1];
    uint32_t idhash;

    char name[EBPF_MAX_NAME + 1];

    // Changes made to simplify integration between apps and eBPF.
    netdata_publish_cachestat_t cachestat;
    netdata_publish_dcstat_t dcstat;
    netdata_publish_swap_t swap;
    netdata_publish_vfs_t vfs;
    netdata_fd_stat_t fd;
    netdata_publish_shm_t shm;

    kernel_uint_t starttime;
    kernel_uint_t collected_starttime;

    unsigned int processes; // how many processes have been merged to this
    int exposed;            // if set, we have sent this to netdata
    int hidden;             // if set, we set the hidden flag on the dimension
    int debug_enabled;
    int ends_with;
    int starts_with; // if set, the compare string matches only the
                     // beginning of the command

    struct ebpf_pid_on_target *root_pid; // list of aggregated pids for target debugging

    struct ebpf_target *target; // the one that will be reported to netdata
    struct ebpf_target *next;
};

extern struct ebpf_target *apps_groups_default_target;
extern struct ebpf_target *apps_groups_root_target;
extern struct ebpf_target *users_root_target;
extern struct ebpf_target *groups_root_target;

struct ebpf_pid_stat {
    int32_t pid;
    char comm[EBPF_MAX_COMPARE_NAME + 1];
    char *cmdline;

    uint32_t log_thrown;

    // char state;
    int32_t ppid;

    int children_count;              // number of processes directly referencing this
    unsigned char keep : 1;          // 1 when we need to keep this process in memory even after it exited
    int keeploops;                   // increases by 1 every time keep is 1 and updated 0
    unsigned char updated : 1;       // 1 when the process is currently running
    unsigned char updated_twice : 1; // 1 when the process was running in the previous iteration
    unsigned char merged : 1;        // 1 when it has been merged to its parent
    unsigned char read : 1;          // 1 when we have already read this process for this iteration

    int sortlist; // higher numbers = top on the process tree

    // each process gets a unique number

    struct ebpf_target *target;       // app_groups.conf targets
    struct ebpf_target *user_target;  // uid based targets
    struct ebpf_target *group_target; // gid based targets

    usec_t stat_collected_usec;
    usec_t last_stat_collected_usec;

    char *stat_filename;
    char *status_filename;
    char *io_filename;
    char *cmdline_filename;

    struct ebpf_pid_stat *parent;
    struct ebpf_pid_stat *prev;
    struct ebpf_pid_stat *next;
};

// ----------------------------------------------------------------------------
// target
//
// target is the structure that processes are aggregated to be reported
// to netdata.
//
// - Each entry in /etc/apps_groups.conf creates a target.
// - Each user and group used by a process in the system, creates a target.
struct ebpf_pid_on_target {
    int32_t pid;
    struct ebpf_pid_on_target *next;
};

// ----------------------------------------------------------------------------
// Structures used to read information from kernel ring
typedef struct ebpf_process_stat {
    uint64_t pid_tgid; // This cannot be removed, because it is used inside kernel ring.
    uint32_t pid;

    //Counter
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t create_process;
    uint32_t create_thread;

    //Counter
    uint32_t task_err;

    uint8_t removeme;
} ebpf_process_stat_t;

typedef struct ebpf_bandwidth {
    uint32_t pid;

    uint64_t first;              // First timestamp
    uint64_t ct;                 // Last timestamp
    uint64_t bytes_sent;         // Bytes sent
    uint64_t bytes_received;     // Bytes received
    uint64_t call_tcp_sent;      // Number of times tcp_sendmsg was called
    uint64_t call_tcp_received;  // Number of times tcp_cleanup_rbuf was called
    uint64_t retransmit;         // Number of times tcp_retransmit was called
    uint64_t call_udp_sent;      // Number of times udp_sendmsg was called
    uint64_t call_udp_received;  // Number of times udp_recvmsg was called
    uint64_t close;              // Number of times tcp_close was called
    uint64_t drop;               // THIS IS NOT USED FOR WHILE, we are in groom section
    uint32_t tcp_v4_connection;  // Number of times tcp_v4_connection was called.
    uint32_t tcp_v6_connection;  // Number of times tcp_v6_connection was called.
} ebpf_bandwidth_t;

/**
 * Internal function used to write debug messages.
 *
 * @param fmt   the format to create the message.
 * @param ...   the arguments to fill the format.
 */
static inline void debug_log_int(const char *fmt, ...)
{
    va_list args;

    fprintf(stderr, "apps.plugin: ");
    va_start(args, fmt);
    vfprintf(stderr, fmt, args);
    va_end(args);

    fputc('\n', stderr);
}

// ----------------------------------------------------------------------------
// Exported variabled and functions
//
extern struct ebpf_pid_stat **ebpf_all_pids;

int ebpf_read_apps_groups_conf(struct ebpf_target **apps_groups_default_target,
                               struct ebpf_target **apps_groups_root_target,
                               const char *path,
                               const char *file);

void clean_apps_groups_target(struct ebpf_target *apps_groups_root_target);

size_t zero_all_targets(struct ebpf_target *root);

int am_i_running_as_root();

void cleanup_exited_pids();

int ebpf_read_hash_table(void *ep, int fd, uint32_t pid);

int get_pid_comm(pid_t pid, size_t n, char *dest);

size_t read_processes_statistic_using_pid_on_target(ebpf_process_stat_t **ep,
                                                           int fd,
                                                           struct ebpf_pid_on_target *pids);

size_t read_bandwidth_statistic_using_pid_on_target(ebpf_bandwidth_t **ep, int fd, struct ebpf_pid_on_target *pids);

void collect_data_for_all_processes(int tbl_pid_stats_fd, int maps_per_core);
void ebpf_process_apps_accumulator(ebpf_process_stat_t *out, int maps_per_core);

extern ebpf_process_stat_t **global_process_stats;
extern netdata_publish_cachestat_t **cachestat_pid;
extern netdata_publish_dcstat_t **dcstat_pid;
extern netdata_publish_swap_t **swap_pid;
extern netdata_publish_vfs_t **vfs_pid;
extern netdata_fd_stat_t **fd_pid;
extern netdata_publish_shm_t **shm_pid;

// The default value is at least 32 times smaller than maximum number of PIDs allowed on system,
// this is only possible because we are using ARAL (https://github.com/netdata/netdata/tree/master/libnetdata/aral).
#ifndef NETDATA_EBPF_ALLOC_MAX_PID
# define NETDATA_EBPF_ALLOC_MAX_PID 1024
#endif
#define NETDATA_EBPF_ALLOC_MIN_ELEMENTS 256

// ARAL Sectiion
extern void ebpf_aral_init(void);

extern ebpf_process_stat_t *ebpf_process_stat_get(void);
extern void ebpf_process_stat_release(ebpf_process_stat_t *stat);
extern ebpf_process_stat_t *process_stat_vector;

extern ARAL *ebpf_aral_socket_pid;
void ebpf_socket_aral_init();
ebpf_socket_publish_apps_t *ebpf_socket_stat_get(void);
void ebpf_socket_release(ebpf_socket_publish_apps_t *stat);

extern ARAL *ebpf_aral_cachestat_pid;
void ebpf_cachestat_aral_init();
netdata_publish_cachestat_t *ebpf_publish_cachestat_get(void);
void ebpf_cachestat_release(netdata_publish_cachestat_t *stat);

extern ARAL *ebpf_aral_dcstat_pid;
void ebpf_dcstat_aral_init();
netdata_publish_dcstat_t *ebpf_publish_dcstat_get(void);
void ebpf_dcstat_release(netdata_publish_dcstat_t *stat);

extern ARAL *ebpf_aral_vfs_pid;
void ebpf_vfs_aral_init();
netdata_publish_vfs_t *ebpf_vfs_get(void);
void ebpf_vfs_release(netdata_publish_vfs_t *stat);

extern ARAL *ebpf_aral_fd_pid;
void ebpf_fd_aral_init();
netdata_fd_stat_t *ebpf_fd_stat_get(void);
void ebpf_fd_release(netdata_fd_stat_t *stat);

extern ARAL *ebpf_aral_shm_pid;
void ebpf_shm_aral_init();
netdata_publish_shm_t *ebpf_shm_stat_get(void);
void ebpf_shm_release(netdata_publish_shm_t *stat);

// ARAL Section end

// Threads integrated with apps
extern ebpf_socket_publish_apps_t **socket_bandwidth_curr;
// Threads integrated with apps

#include "libnetdata/threads/threads.h"

// ARAL variables
extern ARAL *ebpf_aral_apps_pid_stat;
extern ARAL *ebpf_aral_process_stat;
#define NETDATA_EBPF_PROC_ARAL_NAME "ebpf_proc_stat"

#endif /* NETDATA_EBPF_APPS_H */
