// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_APPS_H
#define NETDATA_EBPF_APPS_H 1

#include "libnetdata/threads/threads.h"
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

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100

// ----------------------------------------------------------------------------
// pid_stat
//
// structure to store data for each process running
// see: man proc for the description of the fields

struct pid_fd {
    int fd;
};

struct target {
    char compare[MAX_COMPARE_NAME + 1];
    uint32_t comparehash;
    size_t comparelen;

    char id[MAX_NAME + 1];
    uint32_t idhash;

    char name[MAX_NAME + 1];

    uid_t uid;
    gid_t gid;

    // Changes made to simplify integration between apps and eBPF.
    netdata_publish_cachestat_t cachestat;
    netdata_publish_dcstat_t dcstat;
    netdata_publish_swap_t swap;
    netdata_publish_vfs_t vfs;
    netdata_fd_stat_t fd;
    netdata_publish_shm_t shm;

    /* These variables are not necessary for eBPF collector
    kernel_uint_t minflt;
    kernel_uint_t cminflt;
    kernel_uint_t majflt;
    kernel_uint_t cmajflt;
    kernel_uint_t utime;
    kernel_uint_t stime;
    kernel_uint_t gtime;
    kernel_uint_t cutime;
    kernel_uint_t cstime;
    kernel_uint_t cgtime;
    kernel_uint_t num_threads;
    // kernel_uint_t rss;

    kernel_uint_t status_vmsize;
    kernel_uint_t status_vmrss;
    kernel_uint_t status_vmshared;
    kernel_uint_t status_rssfile;
    kernel_uint_t status_rssshmem;
    kernel_uint_t status_vmswap;

    kernel_uint_t io_logical_bytes_read;
    kernel_uint_t io_logical_bytes_written;
    // kernel_uint_t io_read_calls;
    // kernel_uint_t io_write_calls;
    kernel_uint_t io_storage_bytes_read;
    kernel_uint_t io_storage_bytes_written;
    // kernel_uint_t io_cancelled_write_bytes;

    int *target_fds;
    int target_fds_size;

    kernel_uint_t openfiles;
    kernel_uint_t openpipes;
    kernel_uint_t opensockets;
    kernel_uint_t openinotifies;
    kernel_uint_t openeventfds;
    kernel_uint_t opentimerfds;
    kernel_uint_t opensignalfds;
    kernel_uint_t openeventpolls;
    kernel_uint_t openother;
    */

    kernel_uint_t starttime;
    kernel_uint_t collected_starttime;

    /*
    kernel_uint_t uptime_min;
    kernel_uint_t uptime_sum;
    kernel_uint_t uptime_max;
    */

    unsigned int processes; // how many processes have been merged to this
    int exposed;            // if set, we have sent this to netdata
    int hidden;             // if set, we set the hidden flag on the dimension
    int debug_enabled;
    int ends_with;
    int starts_with; // if set, the compare string matches only the
                     // beginning of the command

    struct pid_on_target *root_pid; // list of aggregated pids for target debugging

    struct target *target; // the one that will be reported to netdata
    struct target *next;
};

extern struct target *apps_groups_default_target;
extern struct target *apps_groups_root_target;
extern struct target *users_root_target;
extern struct target *groups_root_target;

struct ebpf_pid_stat {
    int32_t pid;
    char comm[MAX_COMPARE_NAME + 1];
    char *cmdline;

    uint32_t log_thrown;

    // char state;
    int32_t ppid;

    struct pid_fd *fds; // array of fds it uses
    size_t fds_size;    // the size of the fds array

    int children_count;              // number of processes directly referencing this
    unsigned char keep : 1;          // 1 when we need to keep this process in memory even after it exited
    int keeploops;                   // increases by 1 every time keep is 1 and updated 0
    unsigned char updated : 1;       // 1 when the process is currently running
    unsigned char updated_twice : 1; // 1 when the process was running in the previous iteration
    unsigned char merged : 1;        // 1 when it has been merged to its parent
    unsigned char read : 1;          // 1 when we have already read this process for this iteration

    int sortlist; // higher numbers = top on the process tree

    // each process gets a unique number

    struct target *target;       // app_groups.conf targets
    struct target *user_target;  // uid based targets
    struct target *group_target; // gid based targets

    usec_t stat_collected_usec;
    usec_t last_stat_collected_usec;

    usec_t io_collected_usec;
    usec_t last_io_collected_usec;

    kernel_uint_t uptime;

    char *fds_dirname; // the full directory name in /proc/PID/fd

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
struct pid_on_target {
    int32_t pid;
    struct pid_on_target *next;
};

// ----------------------------------------------------------------------------
// Structures used to read information from kernel ring
typedef struct ebpf_process_stat {
    uint64_t pid_tgid;
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
extern struct ebpf_pid_stat **all_pids;

int ebpf_read_apps_groups_conf(struct target **apps_groups_default_target,
                                      struct target **apps_groups_root_target,
                                      const char *path,
                                      const char *file);

void clean_apps_groups_target(struct target *apps_groups_root_target);

size_t zero_all_targets(struct target *root);

int am_i_running_as_root();

void cleanup_exited_pids();

int ebpf_read_hash_table(void *ep, int fd, uint32_t pid);

int get_pid_comm(pid_t pid, size_t n, char *dest);

size_t read_processes_statistic_using_pid_on_target(ebpf_process_stat_t **ep,
                                                           int fd,
                                                           struct pid_on_target *pids);

size_t read_bandwidth_statistic_using_pid_on_target(ebpf_bandwidth_t **ep, int fd, struct pid_on_target *pids);

void collect_data_for_all_processes(int tbl_pid_stats_fd);

extern ebpf_process_stat_t **global_process_stats;
extern ebpf_process_publish_apps_t **current_apps_data;
extern netdata_publish_cachestat_t **cachestat_pid;
extern netdata_publish_dcstat_t **dcstat_pid;

#endif /* NETDATA_EBPF_APPS_H */
