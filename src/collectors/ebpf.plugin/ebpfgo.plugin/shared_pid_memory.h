// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPFGO_SHARED_PID_MEMORY_H
#define NETDATA_EBPFGO_SHARED_PID_MEMORY_H 1

#include <stddef.h>

#include "apps_ebpf_shared_pid_row.h"

struct shared_pid_memory;

/* shm_name / sem_name: POSIX names for the shared-memory object and its
 * semaphore (must start with '/').  Callers supply these so tests can use
 * unique non-production names and avoid touching a live agent's segment.
 *
 * update_every_s: the publisher's collection interval in seconds.  Written into
 * the SHM header so the reader can scale its stale-timeout window instead of
 * using a hardcoded constant that breaks when update_every > 10. */
struct shared_pid_memory *shared_pid_memory_open(const char *shm_name, const char *sem_name,
                                                  size_t total, uint32_t update_every_s);
/* flags: OR of EBPFGO_SHM_FLAG_* bits; written into the SHM header under the
 * semaphore along with last_publish_ut so consumers can tell which modules
 * produced valid data and whether the payload is still live. */
int shared_pid_memory_publish(struct shared_pid_memory *ctx, const struct ebpf_pid_stat *entries, size_t count, uint32_t flags);
void shared_pid_memory_close(struct shared_pid_memory *ctx);

#endif
