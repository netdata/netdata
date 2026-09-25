// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef APPS_NATIVE_H
#define APPS_NATIVE_H
#include <stddef.h>
#include <stdint.h>
enum {
    CPU_USER,
    CPU_SYSTEM,
    CPU_GUEST,
    CHILD_USER,
    CHILD_SYSTEM,
    CHILD_GUEST,
    MINFLT,
    MAJFLT,
    CHILD_MINFLT,
    CHILD_MAJFLT,
    VMEM,
    RSS,
    SHARED,
    SWAP,
    PSS,
    ESTIMATE,
    READ_BYTES,
    WRITE_BYTES,
    LOGICAL_READ,
    LOGICAL_WRITE,
    READ_CALLS,
    WRITE_CALLS,
    VOLUNTARY,
    INVOLUNTARY,
    THREADS,
    UPTIME,
    FD_PERCENT,
    PSS_AGE,
    METRIC_COUNT
};
enum { FD_FILE, FD_SOCKET, FD_PIPE, FD_INOTIFY, FD_EVENT, FD_TIMER, FD_SIGNAL, FD_EPOLL, FD_OTHER, FD_COUNT };
typedef struct {
    int32_t pid, ppid;
    uint64_t start;
    uint32_t uid, gid;
    char *comm, *cmdline;
    char state;
    double values[METRIC_COUNT];
    uint64_t valid, fds[FD_COUNT];
    int fd_valid;
} apps_row;
typedef struct {
    int32_t pid;
    uint64_t start;
    uint32_t groups[3];
} apps_assignment;
typedef struct {
    uint32_t id;
    uint64_t counts[FD_COUNT];
    int valid;
} apps_group_fd;
typedef struct {
    uint64_t generation;
    apps_row *rows;
    size_t count;
    double cpu[3];
    int cpu_valid, cpu_count;
    uint64_t file_reads, link_reads, read_errors;
} apps_snapshot;
typedef struct apps_scanner apps_scanner;
apps_scanner *apps_new(const char *root, int fds, int pss);
void apps_close(apps_scanner *s);
// now=0 uses CLOCK_MONOTONIC. A supplied timestamp is for deterministic fixtures.
int apps_scan(apps_scanner *s, double now, apps_snapshot **out);
int apps_finalize(apps_scanner *s, uint64_t generation, const apps_assignment *a, size_t count, apps_group_fd **groups,
                  size_t *group_count);
#endif
