// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_EBPFGO_FD_H
#define NETDATA_CGROUP_EBPFGO_FD_H

#include <stdbool.h>
#include <limits.h>
#include <stdint.h>

/* Publish rates as nano-calls/s so per-PID intervals can be aggregated without
 * discarding fractional rates. */
#define CGROUP_EBPFGO_FD_RATE_SCALE UINT64_C(1000000000)

static inline long long cgroup_ebpfgo_fd_normalize_rate(uint32_t delta, uint32_t update_every_s)
{
    if (!update_every_s)
        update_every_s = 1;

    uint64_t rate = (uint64_t)delta * CGROUP_EBPFGO_FD_RATE_SCALE / update_every_s;
    return rate > LLONG_MAX ? LLONG_MAX : (long long)rate;
}

/* Saturating accumulate.
 *
 * INVARIANT: rate MUST be >= 0.  `LLONG_MAX - rate` is what makes the overflow
 * test work, and with a negative rate that subtraction itself overflows, which
 * is undefined behaviour.  The invariant holds by construction today — every
 * caller passes cgroup_ebpfgo_fd_normalize_rate() output, which is a uint64
 * quotient clamped into [0, LLONG_MAX] — but this is a header-visible helper, so
 * the cheap guard is kept rather than relying on callers to stay disciplined. */
static inline long long cgroup_ebpfgo_fd_add_rate(long long total, long long rate)
{
    if (rate <= 0)
        return total;

    return total > LLONG_MAX - rate ? LLONG_MAX : total + rate;
}

/* Keep consumed counter tokens for at least one producer interval, expressed
 * in cgroup refreshes, so a temporary cgroup.procs gap cannot replay a row. */
static inline uint64_t cgroup_ebpfgo_fd_token_grace_generations(
    uint32_t fd_update_every_s,
    uint32_t cgroup_update_every_s)
{
    if (!cgroup_update_every_s)
        cgroup_update_every_s = 1;
    if (!fd_update_every_s)
        fd_update_every_s = cgroup_update_every_s;

    uint64_t grace = ((uint64_t)fd_update_every_s + cgroup_update_every_s - 1) / cgroup_update_every_s;
    return grace < 2 ? 2 : grace;
}

static inline bool cgroup_ebpfgo_fd_token_expired(
    uint64_t last_seen,
    uint64_t generation,
    uint32_t fd_update_every_s,
    uint32_t cgroup_update_every_s)
{
    return generation > last_seen &&
           generation - last_seen > cgroup_ebpfgo_fd_token_grace_generations(
               fd_update_every_s, cgroup_update_every_s);
}

#endif
