#ifndef _NETDATA_VFS_EBPF_H_
# define _NETDATA_VFS_EBPF_H_ 1

# include <stdint.h>

#ifndef __FreeBSD__
#   include <linux/perf_event.h>
# endif
# include <stdint.h>
# include <errno.h>
# include <signal.h>
# include <stdio.h>
# include <stdint.h>
# include <stdlib.h>
# include <string.h>
# include <unistd.h>
# include <dlfcn.h>

# include <fcntl.h>
# include <ctype.h>
# include <dirent.h>

//From libnetdata.h
# include "../../libnetdata/threads/threads.h"
# include "../../libnetdata/locks/locks.h"
# include "../../libnetdata/avl/avl.h"
# include "../../libnetdata/clocks/clocks.h"
# include "../../libnetdata/config/appconfig.h"
# include "../../libnetdata/ebpf/ebpf.h"
# include "../../daemon/main.h"

typedef enum {
    MODE_RETURN = 0,    //This attaches kprobe when the function returns
    MODE_DEVMODE,       //This stores log given description about the errors raised
    MODE_ENTRY          //This attaches kprobe when the function is called
} netdata_run_mode_t;

typedef struct netdata_syscall_stat {
    unsigned long bytes;                //total number of bytes
    uint64_t call;                      //total number of calls
    uint64_t ecall;                     //number of calls that returned error
    struct netdata_syscall_stat  *next; //Link list
}netdata_syscall_stat_t;

typedef uint64_t netdata_idx_t;

typedef struct netdata_publish_syscall {
    char *dimension;
    char *name;
    unsigned long nbyte;
    unsigned long pbyte;
    uint64_t ncall;
    uint64_t pcall;
    uint64_t nerr;
    uint64_t perr;
    struct netdata_publish_syscall *next;
}netdata_publish_syscall_t;

typedef struct netdata_publish_vfs_common {
    long write;
    long read;

    long running;
    long zombie;
}netdata_publish_vfs_common_t;

typedef struct netdata_error_report {
    char comm[16];
    __u32 pid;

    int type;
    int err;
}netdata_error_report_t;

typedef struct ebpf_module {
    const char *thread_name;
    const char *config_name;
    int enabled;
    void *(*start_routine) (void *);
    int update_time;
    int global_charts;
    int apps_charts;
    netdata_run_mode_t mode;
    netdata_ebpf_events_t *probes;
    uint32_t thread_id;
} ebpf_module_t;

//Chart defintions
# define NETDATA_EBPF_FAMILY "ebpf"

//Log file
# define NETDATA_DEVELOPER_LOG_FILE "developer.log"

//Maximum number of processors monitored on perf events
# define NETDATA_MAX_PROCESSOR 512

//Kernel versions calculated with the formula:
//   R = MAJOR*65536 + MINOR*256 + PATCH
# define NETDATA_KERNEL_V5_3 328448
# define NETDATA_KERNEL_V4_15 265984


# define EBPF_MAX_MAPS 32


//Threads
extern void *ebpf_process_thread(void *ptr);
extern void *ebpf_socket_thread(void *ptr);

//Common variables
extern pthread_mutex_t lock;
extern int close_ebpf_plugin;
extern int ebpf_nprocs;
extern int running_on_kernel;
extern char *ebpf_plugin_dir;
extern char kernel_string[64];
extern netdata_ebpf_events_t process_probes[];
extern netdata_ebpf_events_t socket_probes[];

//Common functions
extern void ebpf_global_labels(netdata_syscall_stat_t *is,
                               netdata_publish_syscall_t *pio,
                               char **dim,
                               char **name,
                               int end);

extern void ebpf_write_chart_cmd(char *type
    , char *id
    , char *axis
    , char *web
    , int order);

extern void ebpf_write_global_dimension(char *n, char *d);

extern void ebpf_create_global_dimension(void *ptr, int end);

extern void ebpf_create_chart(char *family
    , char *name
    , char *axis
    , char *web
    , int order
    , void (*ncd)(void *, int)
    , void *move
    , int end);

extern void write_begin_chart(char *family, char *name);

extern void write_chart_dimension(char *dim, long long value);

extern void write_count_chart(char *name, char *family, netdata_publish_syscall_t *move, int end);

extern void write_err_chart(char *name, char *family, netdata_publish_syscall_t *move, int end);

void write_io_chart(char *chart, char *family, char *dwrite, char *dread, netdata_publish_vfs_common_t *pvc);

extern void fill_ebpf_functions(ebpf_functions_t *ef);

# define EBPF_GLOBAL_SECTION "global"
# define EBPF_PROGRAMS_SECTION "ebpf programs"

#endif
