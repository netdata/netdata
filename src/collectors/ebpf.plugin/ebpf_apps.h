// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_APPS_H
#define NETDATA_EBPF_APPS_H 1

#include "libnetdata/libnetdata.h"
#include "collectors/collectors-ipc/collectors-ipc.h"
#include "libbpf_api/ebpf.h"

#define NETDATA_APPS_FAMILY "apps"
#define NETDATA_APP_FAMILY "app"
#define NETDATA_APPS_FILE_GROUP "file_access"
#define NETDATA_APPS_FILE_FDS "fds"
#define NETDATA_APPS_PROCESS_GROUP "process"
#define NETDATA_APPS_NET_GROUP "net"
#define NETDATA_APPS_IPC_SHM_GROUP "ipc shm"

#include "ebpf_process.h"
#include "ebpf_dcstat.h"
#include "ebpf_disk.h"
#include "ebpf_fd.h"
#include "ebpf_filesystem.h"
#include "ebpf_functions.h"
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

#define EBPF_MAX_COMPARE_NAME 95
#define EBPF_MAX_NAME 100

#define EBPF_CLEANUP_FACTOR 2

extern int pids_fd[NETDATA_EBPF_PIDS_END_IDX];

enum ebpf_main_index {
    EBPF_MODULE_PROCESS_IDX,
    EBPF_MODULE_SOCKET_IDX,
    EBPF_MODULE_CACHESTAT_IDX,
    EBPF_MODULE_SYNC_IDX,
    EBPF_MODULE_DCSTAT_IDX,
    EBPF_MODULE_SWAP_IDX,
    EBPF_MODULE_VFS_IDX,
    EBPF_MODULE_FILESYSTEM_IDX,
    EBPF_MODULE_DISK_IDX,
    EBPF_MODULE_MOUNT_IDX,
    EBPF_MODULE_FD_IDX,
    EBPF_MODULE_HARDIRQ_IDX,
    EBPF_MODULE_SOFTIRQ_IDX,
    EBPF_MODULE_OOMKILL_IDX,
    EBPF_MODULE_SHM_IDX,
    EBPF_MODULE_MDFLUSH_IDX,
    EBPF_MODULE_FUNCTION_IDX,
    /* THREADS MUST BE INCLUDED BEFORE THIS COMMENT */
    EBPF_OPTION_ALL_CHARTS,
    EBPF_OPTION_VERSION,
    EBPF_OPTION_HELP,
    EBPF_OPTION_GLOBAL_CHART,
    EBPF_OPTION_RETURN_MODE,
    EBPF_OPTION_LEGACY,
    EBPF_OPTION_CORE,
    EBPF_OPTION_UNITTEST
};

// ----------------------------------------------------------------------------
// Structures used to read information from kernel ring

typedef struct __attribute__((packed)) ebpf_publish_process {
    uint64_t ct;

    //Counter
    uint32_t exit_call;
    uint32_t release_call;
    uint32_t create_process;
    uint32_t create_thread;

    //Counter
    uint32_t task_err;
} ebpf_publish_process_t;

// ----------------------------------------------------------------------------
// pid_stat
//
struct ebpf_target {
    char compare[EBPF_MAX_COMPARE_NAME + 1];
    uint32_t comparehash;
    size_t comparelen;

    char id[EBPF_MAX_NAME + 1];
    uint32_t idhash;
    uint32_t charts_created;

    char name[EBPF_MAX_NAME + 1];
    char clean_name[EBPF_MAX_NAME + 1]; // sanitized name used in chart id (need to replace at least dots)

    // Changes made to simplify integration between apps and eBPF.
    netdata_publish_cachestat_t cachestat;
    netdata_publish_dcstat_t dcstat;
    netdata_publish_swap_t swap;
    netdata_publish_vfs_t vfs;
    netdata_fd_stat_t fd;
    netdata_publish_shm_t shm;
    ebpf_process_stat_t process;
    ebpf_socket_publish_apps_t socket;

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
extern uint64_t collect_pids;

// ebpf_pid_data
typedef struct __attribute__((packed)) ebpf_pid_data {
    uint32_t pid;
    uint32_t ppid;
    uint64_t thread_collecting;

    char comm[EBPF_MAX_COMPARE_NAME + 1];
    char *cmdline;

    uint32_t has_proc_file;
    uint32_t not_updated;
    int children_count; // number of processes directly referencing this
    int merged;
    int sortlist; // higher numbers = top on the process tree

    struct ebpf_target *target; // the one that will be reported to netdata
    struct ebpf_pid_data *parent;
    struct ebpf_pid_data *prev;
    struct ebpf_pid_data *next;

    netdata_publish_fd_stat_t *fd;
    netdata_publish_swap_t *swap;
    netdata_publish_shm_t *shm; // this has a leak issue
    netdata_publish_dcstat_t *dc;
    netdata_publish_vfs_t *vfs;
    netdata_publish_cachestat_t *cachestat;
    ebpf_publish_process_t *process;
    ebpf_socket_publish_apps_t *socket;

} ebpf_pid_data_t;

//extern ebpf_pid_data_t *ebpf_pids;
extern ebpf_pid_data_t *ebpf_pids_link_list;
extern size_t ebpf_all_pids_count;
extern size_t ebpf_hash_table_pids_count;
void ebpf_del_pid_entry(pid_t pid);

static inline void *ebpf_cachestat_allocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(netdata_publish_cachestat_t));
}

static inline void ebpf_cachestat_release_publish(netdata_publish_cachestat_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_dcallocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(netdata_publish_dcstat_t));
}

static inline void ebpf_dc_release_publish(netdata_publish_dcstat_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_fd_allocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(netdata_publish_fd_stat_t));
}

static inline void ebpf_fd_release_publish(netdata_publish_fd_stat_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_shm_allocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(netdata_publish_shm_t));
}

static inline void ebpf_shm_release_publish(netdata_publish_shm_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_socket_allocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(ebpf_socket_publish_apps_t));
}

static inline void ebpf_socket_release_publish(ebpf_socket_publish_apps_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_swap_allocate_publish_swap()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(netdata_publish_swap_t));
}

static inline void ebpf_swap_release_publish(netdata_publish_swap_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_vfs_allocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(netdata_publish_vfs_t));
}

static inline void ebpf_vfs_release_publish(netdata_publish_vfs_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

static inline void *ebpf_process_allocate_publish()
{
    ebpf_hash_table_pids_count++;
    return callocz(1, sizeof(ebpf_publish_process_t));
}

static inline void ebpf_process_release_publish(ebpf_publish_process_t *ptr)
{
    ebpf_hash_table_pids_count--;
    freez(ptr);
}

ebpf_pid_data_t *ebpf_find_or_create_pid_data(pid_t pid);

static inline ebpf_pid_data_t *ebpf_get_pid_data(uint32_t pid, uint32_t tgid, char *name, uint32_t idx)
{
    //    ebpf_pid_data_t *ptr = &ebpf_pids[pid];
    ebpf_pid_data_t *ptr = ebpf_find_or_create_pid_data(pid);
    ptr->thread_collecting |= 1 << idx;
    // The caller is getting data to work.
    if (!name && idx != NETDATA_EBPF_PIDS_PROC_FILE)
        return ptr;

    if (ptr->pid == pid) {
        return ptr;
    }

    ptr->pid = pid;
    ptr->ppid = tgid;

    if (name)
        strncpyz(ptr->comm, name, EBPF_MAX_COMPARE_NAME);

    if (likely(ebpf_pids_link_list))
        ebpf_pids_link_list->prev = ptr;

    ptr->next = ebpf_pids_link_list;
    ebpf_pids_link_list = ptr;
    if (idx == NETDATA_EBPF_PIDS_PROC_FILE) {
        ebpf_all_pids_count++;
    }

    return ptr;
}

static inline void ebpf_release_pid_data(ebpf_pid_data_t *eps, int fd, uint32_t key, uint32_t idx)
{
    if (fd) {
        bpf_map_delete_elem(fd, &key);
    }
    eps->thread_collecting &= ~(1 << idx);
    if (!eps->thread_collecting && !eps->has_proc_file) {
        ebpf_del_pid_entry((pid_t)key);
    }
}

static inline void ebpf_reset_specific_pid_data(ebpf_pid_data_t *ptr)
{
    int idx;
    uint32_t pid = ptr->pid;
    for (idx = NETDATA_EBPF_PIDS_PROCESS_IDX; idx < NETDATA_EBPF_PIDS_PROC_FILE; idx++) {
        if (!(ptr->thread_collecting & (1 << idx))) {
            continue;
        }
        // Check if we still have the map loaded
        int fd = pids_fd[idx];
        if (fd <= STDERR_FILENO)
            continue;

        bpf_map_delete_elem(fd, &pid);
        ebpf_hash_table_pids_count--;
        void *clean;
        switch (idx) {
            case NETDATA_EBPF_PIDS_PROCESS_IDX:
                clean = ptr->process;
                break;
            case NETDATA_EBPF_PIDS_SOCKET_IDX:
                clean = ptr->socket;
                break;
            case NETDATA_EBPF_PIDS_CACHESTAT_IDX:
                clean = ptr->cachestat;
                break;
            case NETDATA_EBPF_PIDS_DCSTAT_IDX:
                clean = ptr->dc;
                break;
            case NETDATA_EBPF_PIDS_SWAP_IDX:
                clean = ptr->swap;
                break;
            case NETDATA_EBPF_PIDS_VFS_IDX:
                clean = ptr->vfs;
                break;
            case NETDATA_EBPF_PIDS_FD_IDX:
                clean = ptr->fd;
                break;
            case NETDATA_EBPF_PIDS_SHM_IDX:
                clean = ptr->shm;
                break;
            default:
                clean = NULL;
        }
        freez(clean);
    }

    ebpf_del_pid_entry(pid);
}

typedef struct ebpf_pid_stat {
    uint32_t pid;
    uint64_t thread_collecting;
    char comm[EBPF_MAX_COMPARE_NAME + 1];
    char *cmdline;

    uint32_t log_thrown;

    // char state;
    uint32_t ppid;

    int children_count;              // number of processes directly referencing this
    unsigned char keep : 1;          // 1 when we need to keep this process in memory even after it exited
    int keeploops;                   // increases by 1 every time keep is 1 and updated 0
    unsigned char updated : 1;       // 1 when the process is currently running
    unsigned char updated_twice : 1; // 1 when the process was running in the previous iteration
    unsigned char merged : 1;        // 1 when it has been merged to its parent
    unsigned char read : 1;          // 1 when we have already read this process for this iteration

    int sortlist; // higher numbers = top on the process tree

    // each process gets a unique number
    netdata_publish_cachestat_t cachestat;
    netdata_publish_dcstat_t dc;
    netdata_fd_stat_t fd;
    ebpf_process_stat_t process;
    netdata_publish_shm_t shm;
    netdata_publish_swap_t swap;
    ebpf_socket_publish_apps_t socket;
    netdata_publish_vfs_t vfs;

    int not_updated;

    struct ebpf_target *target;       // app_groups.conf targets
    struct ebpf_target *user_target;  // uid based targets
    struct ebpf_target *group_target; // gid based targets

    usec_t stat_collected_usec;
    usec_t last_stat_collected_usec;

    netdata_publish_cachestat_t cache;

    char *stat_filename;
    char *status_filename;
    char *io_filename;
    char *cmdline_filename;

    struct ebpf_pid_stat *parent;
    struct ebpf_pid_stat *prev;
    struct ebpf_pid_stat *next;
} ebpf_pid_stat_t;

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
int ebpf_read_apps_groups_conf(
    struct ebpf_target **apps_groups_default_target,
    struct ebpf_target **apps_groups_root_target,
    const char *path,
    const char *file);

void clean_apps_groups_target(struct ebpf_target *apps_groups_root_target);

size_t zero_all_targets(struct ebpf_target *root);

void cleanup_exited_pids();

int ebpf_read_hash_table(void *ep, int fd, uint32_t pid);

int get_pid_comm(pid_t pid, size_t n, char *dest);

void collect_data_for_all_processes(int tbl_pid_stats_fd, int maps_per_core);
void ebpf_process_apps_accumulator(ebpf_process_stat_t *out, int maps_per_core);

// The default value is at least 32 times smaller than maximum number of PIDs allowed on system,
// this is only possible because we are using ARAL (https://github.com/netdata/netdata/tree/master/src/libnetdata/aral).
#ifndef NETDATA_EBPF_ALLOC_MAX_PID
#define NETDATA_EBPF_ALLOC_MAX_PID 1024
#endif
#define NETDATA_EBPF_ALLOC_MIN_ELEMENTS 256

// ARAL Sectiion
void ebpf_aral_init(void);
extern ebpf_process_stat_t *process_stat_vector;

extern ARAL *ebpf_aral_vfs_pid;
void ebpf_vfs_aral_init();
netdata_publish_vfs_t *ebpf_vfs_get(void);
void ebpf_vfs_release(netdata_publish_vfs_t *stat);

extern ARAL *ebpf_aral_shm_pid;
void ebpf_shm_aral_init();
netdata_publish_shm_t *ebpf_shm_stat_get(void);
void ebpf_shm_release(netdata_publish_shm_t *stat);
void ebpf_parse_proc_files();

// ARAL Section end

// Threads integrated with apps
// Threads integrated with apps

#include "libnetdata/threads/threads.h"

#endif /* NETDATA_EBPF_APPS_H */
