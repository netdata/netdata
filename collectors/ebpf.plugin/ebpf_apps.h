#ifndef _NETDATA_EBPF_APPS_H
# define _NETDATA_EBPF_APPS_H 1

# include "../../libnetdata/threads/threads.h"
# include "../../libnetdata/locks/locks.h"
# include "../../libnetdata/avl/avl.h"
# include "../../libnetdata/clocks/clocks.h"
# include "../../libnetdata/config/appconfig.h"
# include "../../libnetdata/ebpf/ebpf.h"

# define MAX_COMPARE_NAME 100
# define MAX_NAME 100


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


struct target {
    char compare[MAX_COMPARE_NAME + 1];
    uint32_t comparehash;
    size_t comparelen;

    char id[MAX_NAME + 1];
    uint32_t idhash;

    char name[MAX_NAME + 1];

    uid_t uid;
    gid_t gid;

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

    kernel_uint_t starttime;
    kernel_uint_t collected_starttime;
    kernel_uint_t uptime_min;
    kernel_uint_t uptime_sum;
    kernel_uint_t uptime_max;

    unsigned int processes; // how many processes have been merged to this
    int exposed;            // if set, we have sent this to netdata
    int hidden;             // if set, we set the hidden flag on the dimension
    int debug_enabled;
    int ends_with;
    int starts_with;        // if set, the compare string matches only the
                            // beginning of the command

    struct pid_on_target *root_pid; // list of aggregated pids for target debugging

    struct target *target;  // the one that will be reported to netdata
    struct target *next;
};

extern int ebpf_read_apps_groups_conf(struct target **apps_groups_default_target,
                                      struct target **apps_groups_root_target, const char *path, const char *file);

extern void clean_apps_groups_target(struct target *apps_groups_root_target);

extern size_t zero_all_targets(struct target *root);

extern int am_i_running_as_root();

#endif
