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
#define NETDATA_APPS_SYSCALL_GROUP "ebpf syscall"
#define NETDATA_APPS_NET_GROUP "ebpf net"

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100

// ----------------------------------------------------------------------------
// process_pid_stat
//
// Fields read from the kernel ring for a specific PID
//
typedef struct process_pid_stat {
    uint64_t pid_tgid; // Unique identifier
    uint32_t pid;      // process id

    // Count number of calls done for specific function
    uint32_t open_call;
    uint32_t write_call;
    uint32_t writev_call;
    uint32_t read_call;
    uint32_t readv_call;
    uint32_t unlink_call;
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t fork_call;
    uint32_t clone_call;
    uint32_t close_call;

    // Count number of bytes written or read
    uint64_t write_bytes;
    uint64_t writev_bytes;
    uint64_t readv_bytes;
    uint64_t read_bytes;

    // Count number of errors for the specified function
    uint32_t open_err;
    uint32_t write_err;
    uint32_t writev_err;
    uint32_t read_err;
    uint32_t readv_err;
    uint32_t unlink_err;
    uint32_t fork_err;
    uint32_t clone_err;
    uint32_t close_err;
} process_pid_stat_t;

// ----------------------------------------------------------------------------
// socket_bandwidth
//
// Fields read from the kernel ring for a specific PID
//
typedef struct socket_bandwidth {
    uint64_t first;
    uint64_t ct;
    uint64_t sent;
    uint64_t received;
    unsigned char removed;
} socket_bandwidth_t;

// ----------------------------------------------------------------------------
// pid_stat
//
// structure to store data for each process running
// see: man proc for the description of the fields

struct pid_fd {
    int fd;

#ifndef __FreeBSD__
    ino_t inode;
    char *filename;
    uint32_t link_hash;
    size_t cache_iterations_counter;
    size_t cache_iterations_reset;
#endif
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

struct pid_stat {
    int32_t pid;
    char comm[MAX_COMPARE_NAME + 1];
    char *cmdline;

    uint32_t log_thrown;

    // char state;
    int32_t ppid;

    // int32_t pgrp;
    // int32_t session;
    // int32_t tty_nr;
    // int32_t tpgid;
    // uint64_t flags;

    /*
    // these are raw values collected
    kernel_uint_t minflt_raw;
    kernel_uint_t cminflt_raw;
    kernel_uint_t majflt_raw;
    kernel_uint_t cmajflt_raw;
    kernel_uint_t utime_raw;
    kernel_uint_t stime_raw;
    kernel_uint_t gtime_raw; // guest_time
    kernel_uint_t cutime_raw;
    kernel_uint_t cstime_raw;
    kernel_uint_t cgtime_raw; // cguest_time

    // these are rates
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

    // int64_t priority;
    // int64_t nice;
    int32_t num_threads;
    // int64_t itrealvalue;
    kernel_uint_t collected_starttime;
    // kernel_uint_t vsize;
    // kernel_uint_t rss;
    // kernel_uint_t rsslim;
    // kernel_uint_t starcode;
    // kernel_uint_t endcode;
    // kernel_uint_t startstack;
    // kernel_uint_t kstkesp;
    // kernel_uint_t kstkeip;
    // uint64_t signal;
    // uint64_t blocked;
    // uint64_t sigignore;
    // uint64_t sigcatch;
    // uint64_t wchan;
    // uint64_t nswap;
    // uint64_t cnswap;
    // int32_t exit_signal;
    // int32_t processor;
    // uint32_t rt_priority;
    // uint32_t policy;
    // kernel_uint_t delayacct_blkio_ticks;

    uid_t uid;
    gid_t gid;

    kernel_uint_t status_vmsize;
    kernel_uint_t status_vmrss;
    kernel_uint_t status_vmshared;
    kernel_uint_t status_rssfile;
    kernel_uint_t status_rssshmem;
    kernel_uint_t status_vmswap;
#ifndef __FreeBSD__
    ARL_BASE *status_arl;
#endif

    kernel_uint_t io_logical_bytes_read_raw;
    kernel_uint_t io_logical_bytes_written_raw;
    // kernel_uint_t io_read_calls_raw;
    // kernel_uint_t io_write_calls_raw;
    kernel_uint_t io_storage_bytes_read_raw;
    kernel_uint_t io_storage_bytes_written_raw;
    // kernel_uint_t io_cancelled_write_bytes_raw;

    kernel_uint_t io_logical_bytes_read;
    kernel_uint_t io_logical_bytes_written;
    // kernel_uint_t io_read_calls;
    // kernel_uint_t io_write_calls;
    kernel_uint_t io_storage_bytes_read;
    kernel_uint_t io_storage_bytes_written;
    // kernel_uint_t io_cancelled_write_bytes;
     */

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

    struct pid_stat *parent;
    struct pid_stat *prev;
    struct pid_stat *next;
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
    uint32_t open_call;
    uint32_t write_call;
    uint32_t writev_call;
    uint32_t read_call;
    uint32_t readv_call;
    uint32_t unlink_call;
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t fork_call;
    uint32_t clone_call;
    uint32_t close_call;

    //Accumulator
    uint64_t write_bytes;
    uint64_t writev_bytes;
    uint64_t readv_bytes;
    uint64_t read_bytes;

    //Counter
    uint32_t open_err;
    uint32_t write_err;
    uint32_t writev_err;
    uint32_t read_err;
    uint32_t readv_err;
    uint32_t unlink_err;
    uint32_t fork_err;
    uint32_t clone_err;
    uint32_t close_err;

    uint8_t removeme;
} ebpf_process_stat_t;

typedef struct ebpf_bandwidth {
    uint32_t pid;

    uint64_t first;        //First timestamp
    uint64_t ct;           //Last timestamp
    uint64_t sent;         //Bytes sent
    uint64_t received;     //Bytes received
    unsigned char removed; //Remove the PID from table
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
extern struct pid_stat **all_pids;

extern int ebpf_read_apps_groups_conf(struct target **apps_groups_default_target,
                                      struct target **apps_groups_root_target,
                                      const char *path,
                                      const char *file);

extern void clean_apps_groups_target(struct target *apps_groups_root_target);

extern size_t zero_all_targets(struct target *root);

extern int am_i_running_as_root();

extern void cleanup_exited_pids(ebpf_process_stat_t **out);

extern int ebpf_read_hash_table(void *ep, int fd, uint32_t pid);

extern size_t read_processes_statistic_using_pid_on_target(ebpf_process_stat_t **ep,
                                                           int fd,
                                                           struct pid_on_target *pids);

extern size_t read_bandwidth_statistic_using_pid_on_target(ebpf_bandwidth_t **ep, int fd, struct pid_on_target *pids);

extern void collect_data_for_all_processes(ebpf_process_stat_t **out, pid_t *index, int tbl_pid_stats_fd);

#endif /* NETDATA_EBPF_APPS_H */
