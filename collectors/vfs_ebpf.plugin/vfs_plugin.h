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

# define NETDATA_GLOBAL_VECTOR 18
# define NETDATA_MAX_MONITOR_VECTOR 9
# define NETDATA_FILE_ERRORS 5
# define NETDATA_PROCESS_ERRORS 4

# define NETDATA_DEL_START 2
# define NETDATA_IN_START_BYTE 3
# define NETDATA_EXIT_START 5
# define NETDATA_PROCESS_START 7
# define NETDATA_PROCESS_RUNNING_COUNT 9

# define NETDATA_VFS_THREAD (uint32_t)2

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

typedef struct netdata_syscall_stat {
    unsigned long bytes;                //total number of bytes
    uint64_t call;                      //total number of calls
    uint64_t ecall;                     //number of calls that returned error
    struct netdata_syscall_stat  *next; //Link list
}netdata_syscall_stat_t;

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

# define NETDATA_EBPF_FAMILY "ebpf"
# define NETDATA_FILE_GROUP "file"
# define NETDATA_PROCESS_GROUP "process"

# define NETDATA_FILE_OPEN_CLOSE_COUNT "file_descriptor"
# define NETDATA_VFS_FILE_CLEAN_COUNT "delete_files"
# define NETDATA_VFS_FILE_IO_COUNT "io"
# define NETDATA_VFS_FILE_ERR_COUNT "io_error"

# define NETDATA_EXIT_SYSCALL "exit"
# define NETDATA_PROCESS_SYSCALL "process_thread"
# define NETDATA_PROCESS_ERROR_NAME "task_error"
# define NETDATA_PROCESS_RUNNING "running"

# define NETDATA_VFS_IO_FILE_BYTES "file_IO_Bytes"
# define NETDATA_VFS_DIM_IN_FILE_BYTES "write"
# define NETDATA_VFS_DIM_OUT_FILE_BYTES "read"

# define NETDATA_DEVELOPER_LOG_FILE "developer.log"

# define NETDATA_MAX_PROCESSOR 128

#endif
