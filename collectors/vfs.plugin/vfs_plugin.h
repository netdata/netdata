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

# define NETDATA_MAX_FILE_VECTOR 8
# define NETDATA_IO_START_BYTE 4

# define NETDATA_MAX_DIR_VECTOR 5

# include <fcntl.h>
# include <ctype.h>

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
    unsigned long path;
    uint8_t error;
}netdata_syscall_kern_stat_t;

typedef struct netdata_syscall_stat {
    uint16_t sc_num;
    unsigned long bytes; //total bytes
    uint64_t call; //number of calls
    struct netdata_syscall_stat  *next;
}netdata_syscall_stat_t;

typedef struct netdata_publish_syscall {
    char *dimension;
    unsigned long nbyte;
    unsigned long pbyte;
    uint64_t ncall;
    uint64_t pcall;
    struct netdata_publish_syscall *next;
}netdata_publish_syscall_t;

# define NETDATA_VFS_FAMILY "Virtual_File_System"

# define NETDATA_VFS_FILE_OPEN_COUNT "Open_Files"
# define NETDATA_VFS_IO_FILE_BYTES "File_IO_Bytes"

# define NETDATA_MAX_PROCESSOR 128

#endif
