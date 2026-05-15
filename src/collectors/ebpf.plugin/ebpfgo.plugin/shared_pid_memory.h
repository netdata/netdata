// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPFGO_SHARED_PID_MEMORY_H
#define NETDATA_EBPFGO_SHARED_PID_MEMORY_H 1

#include <stddef.h>

#include "apps_ebpf_shared_pid_row.h"

struct shared_pid_memory;

struct shared_pid_memory *shared_pid_memory_open(size_t total);
int shared_pid_memory_publish(struct shared_pid_memory *ctx, const struct ebpf_pid_stat *entries, size_t count);
void shared_pid_memory_close(struct shared_pid_memory *ctx);

#endif
