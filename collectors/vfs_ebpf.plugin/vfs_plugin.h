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

# define NETDATA_MAX_FILE_VECTOR 4
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

typedef struct netdata_syscall_kern_stat {
    uint32_t pid;
    uint16_t sc_num;
    uint8_t idx;
    enum netdata_map_syscall type;
    unsigned long bytes;
    uint8_t error;
}netdata_syscall_kern_stat_t;

typedef struct netdata_syscall_stat {
    unsigned long bytes; //total bytes
    uint64_t call; //number of calls
    uint64_t ecall; //number of calls with error
    struct netdata_syscall_stat  *next;
}netdata_syscall_stat_t;

struct netdata_pid_stat_t {
    uint32_t pid;  //process id

    uint64_t open_call;
    uint64_t write_call;
    uint64_t read_call;
    uint64_t unlink_call;

    uint64_t write_bytes;
    uint64_t read_bytes;

    uint32_t open_err;
    uint32_t write_err;
    uint32_t read_err;
    uint32_t unlink_err;
};

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
# define NETDATA_WEB_GROUP "vfs"

# define NETDATA_VFS_FILE_OPEN_COUNT "Open_files"
# define NETDATA_VFS_FILE_CLEAN_COUNT "Clean_files"
# define NETDATA_VFS_FILE_WRITE_COUNT "Write2files"
# define NETDATA_VFS_FILE_READ_COUNT "Read2files"
# define NETDATA_VFS_FILE_ERR_COUNT "Error_call"

# define NETDATA_VFS_IN_FILE_BYTES "File_In_Bytes"
# define NETDATA_VFS_OUT_FILE_BYTES "File_Out_Bytes"

# define NETDATA_VFS_IO_FILE_BYTES "File_IO_Bytes"
# define NETDATA_VFS_DIM_IN_FILE_BYTES "Write"
# define NETDATA_VFS_DIM_OUT_FILE_BYTES "Read"

# define NETDATA_MAX_PROCESSOR 128

#endif
