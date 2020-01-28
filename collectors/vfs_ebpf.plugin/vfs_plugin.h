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

# define NETDATA_GLOBAL_VECTOR 13
# define NETDATA_MAX_FILE_VECTOR 7
# define NETDATA_IN_START_BYTE 2
# define NETDATA_OUT_START_BYTE 3

# include <fcntl.h>
# include <ctype.h>
# include <dirent.h>

//From libnetdata.h
# include "../../libnetdata/threads/threads.h"
# include "../../libnetdata/locks/locks.h"
# include "../../libnetdata/avl/avl.h"
# include "../../libnetdata/clocks/clocks.h"

enum netdata_map_syscall {
    FILE_SYSCALL = 0
};

typedef struct netdata_error_report {
    uint32_t pid;

    int type;
    int error;
    int fd;

    char name[24];
} netdata_error_report_t;

typedef struct netdata_syscall_kern_stat {
    uint32_t pid;
    uint16_t sc_num;
    uint8_t idx;
    enum netdata_map_syscall type;
    unsigned long bytes;
    uint8_t error;
}netdata_syscall_kern_stat_t;

typedef struct netdata_syscall_stat {
    unsigned long bytes;                //total number of bytes
    uint64_t call;                      //total number of calls
    uint64_t ecall;                     //number of calls that returned error
    struct netdata_syscall_stat  *next; //Link list
}netdata_syscall_stat_t;

struct netdata_pid_stat_t {
    uint64_t pid_tgid;                  //Kernel unique identifier
    uint32_t pid;                       //process id

    uint32_t open_call;                 //Number of calls to do_sys_open
    uint32_t write_call;                //Number of calls to vfs_write
    uint32_t read_call;                 //Number of calls to vfs_read
    uint32_t unlink_call;               //Number of calls to unlink
    uint32_t exit_call;                 //Number of calls to do_exit
    uint32_t release_call;              //Number of calls to release_task
    uint32_t fork_call;                 //Number of calls to do_fork

    uint64_t write_bytes;               //Total of bytes successfully written to disk
    uint64_t read_bytes;                //Total of bytes successfully read from disk

    uint32_t open_err;                  //Number of times there was error with do_sys_open
    uint32_t write_err;                 //Number of times there was error with vfs_write
    uint32_t read_err;                  //Number of times there was error with vfs_read
    uint32_t unlink_err;                //Number of times there was error with unlink
};

typedef struct netdata_publish_process_syscall {
    uint32_t reset;  //process id

    char *dimension;

    uint64_t nopen_call;
    uint64_t popen_call;
    uint64_t nwrite_call;
    uint64_t pwrite_call;
    uint64_t nread_call;
    uint64_t pread_call;
    uint64_t nunlink_call;
    uint64_t punlink_call;
    uint64_t nexit_call;
    uint64_t pexit_call;
    uint64_t nrelease_call;
    uint64_t prelease_call;
    uint64_t nfork_call;
    uint64_t pfork_call;

    uint64_t nwrite_bytes;
    uint64_t pwrite_bytes;
    uint64_t nread_bytes;
    uint64_t pread_bytes;

    uint64_t nopen_err;
    uint64_t popen_err;
    uint64_t nwrite_err;
    uint64_t pwrite_err;
    uint64_t nread_err;
    uint64_t pread_err;
    uint64_t nunlink_err;
    uint64_t punlink_err;

    long long wopen;
    long long wwrite;
    long long wread;
    long long wunlink;
    long long wfork;
    long long wzombie;

    struct netdata_publish_process_syscall *next;
}netdata_publish_process_syscall_t ;

typedef struct netdata_publish_syscall {
    char *dimension;
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
}netdata_publish_vfs_common_t;

# define NETDATA_VFS_FAMILY "system"
# define NETDATA_APPS_FAMILY "apps"
# define NETDATA_WEB_GROUP "vfs"

# define NETDATA_VFS_FILE_OPEN_COUNT "open_files"
# define NETDATA_VFS_FILE_CLEAN_COUNT "delete_files"
# define NETDATA_VFS_FILE_WRITE_COUNT "write2files"
# define NETDATA_VFS_FILE_READ_COUNT "read2files"
# define NETDATA_VFS_FILE_ERR_COUNT "error_call"

# define NETDATA_EXIT_SYSCALL "exit_process"
# define NETDATA_PROCESS_SYSCALL "start_process"

# define NETDATA_VFS_IO_FILE_BYTES "file_IO_Bytes"
# define NETDATA_VFS_DIM_IN_FILE_BYTES "write"
# define NETDATA_VFS_DIM_OUT_FILE_BYTES "read"

# define NETDATA_DEVELOPER_LOG_FILE "developer.log"

# define NETDATA_MAX_PROCESSOR 128

# define NETDATA_VFS_THREAD (uint32_t)3

#endif
