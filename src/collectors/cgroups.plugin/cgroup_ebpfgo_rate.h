// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_EBPFGO_RATE_H
#define NETDATA_CGROUP_EBPFGO_RATE_H

#include <stdbool.h>
#include <stddef.h>
#include <limits.h>
#include <stdint.h>
#include <sys/types.h>

/* Per-cgroup rates from ebpf-go per-PID interval deltas.
 *
 * Some ebpf-go modules publish per-PID deltas over their own collection interval
 * (fd, process), while cgroups.plugin refreshes on its own, faster cadence.  A
 * PID's row is consumed once (when its ct token is newer than the one consumed
 * for that PID and cgroup), normalized to a per-second rate, and held for one
 * producer interval, so every cgroup refresh shows the rate instead of one burst
 * followed by zeros. */

/* Publish rates as nano-calls/s so per-PID intervals can be aggregated without
 * discarding fractional rates. */
#define CGROUP_EBPFGO_RATE_SCALE UINT64_C(1000000000)

#define CGROUP_EBPFGO_RATE_MAX_COUNTERS 5

static inline long long cgroup_ebpfgo_rate_normalize(uint32_t delta, uint32_t update_every_s)
{
    if (!update_every_s)
        update_every_s = 1;

    uint64_t rate = (uint64_t)delta * CGROUP_EBPFGO_RATE_SCALE / update_every_s;
    return rate > LLONG_MAX ? LLONG_MAX : (long long)rate;
}

/* Saturating accumulate.
 *
 * INVARIANT: rate MUST be >= 0.  `LLONG_MAX - rate` is what makes the overflow
 * test work, and with a negative rate that subtraction itself overflows, which
 * is undefined behaviour.  Every caller passes cgroup_ebpfgo_rate_normalize()
 * output, which is clamped into [0, LLONG_MAX]; the guard stays because this is
 * a header-visible helper. */
static inline long long cgroup_ebpfgo_rate_add(long long total, long long rate)
{
    if (rate <= 0)
        return total;

    return total > LLONG_MAX - rate ? LLONG_MAX : total + rate;
}

/* Keep consumed counter tokens for at least one producer interval, expressed
 * in cgroup refreshes, so a temporary cgroup.procs gap cannot replay a row. */
static inline uint64_t cgroup_ebpfgo_rate_token_grace_generations(
    uint32_t producer_update_every_s,
    uint32_t cgroup_update_every_s)
{
    if (!cgroup_update_every_s)
        cgroup_update_every_s = 1;
    if (!producer_update_every_s)
        producer_update_every_s = cgroup_update_every_s;

    uint64_t grace = ((uint64_t)producer_update_every_s + cgroup_update_every_s - 1) / cgroup_update_every_s;
    return grace < 2 ? 2 : grace;
}

static inline bool cgroup_ebpfgo_rate_token_expired(
    uint64_t last_seen,
    uint64_t generation,
    uint32_t producer_update_every_s,
    uint32_t cgroup_update_every_s)
{
    return generation > last_seen &&
           generation - last_seen > cgroup_ebpfgo_rate_token_grace_generations(
               producer_update_every_s, cgroup_update_every_s);
}

/* One consumed (PID, cgroup) row of one module.  `owner` is the cgroup; it is
 * only compared, never dereferenced. */
typedef struct {
    pid_t pid;
    const void *owner;
    uint64_t ct;
    uint64_t last_seen;
    uint32_t update_every_s;
    uint64_t rate_until;
    long long rate[CGROUP_EBPFGO_RATE_MAX_COUNTERS];
} cgroup_ebpfgo_rate_token_t;

/* One table per module: the ct tokens of different modules are unrelated. */
typedef struct {
    cgroup_ebpfgo_rate_token_t *items;
    size_t count;
    size_t capacity;
    uint64_t generation;
} cgroup_ebpfgo_rate_table_t;

/* Call once per cgroup refresh before the first cgroup_ebpfgo_rate_add_pid(). */
void cgroup_ebpfgo_rate_begin_cycle(cgroup_ebpfgo_rate_table_t *t);

/* Call once per cgroup refresh after the last cgroup_ebpfgo_rate_add_pid();
 * drops tokens unseen for longer than their grace period. */
void cgroup_ebpfgo_rate_end_cycle(cgroup_ebpfgo_rate_table_t *t, uint32_t cgroup_update_every_s);

/* Adds one PID's per-second rates (scaled by CGROUP_EBPFGO_RATE_SCALE) to
 * totals[0..n).  deltas are consumed only when ct is newer than the token held
 * for (pid, owner); otherwise the held rates are re-added until they expire.
 * update_every_s 0 means unknown and falls back to cgroup_update_every_s. */
void cgroup_ebpfgo_rate_add_pid(
    cgroup_ebpfgo_rate_table_t *t,
    pid_t pid,
    const void *owner,
    uint64_t ct,
    uint32_t update_every_s,
    uint32_t cgroup_update_every_s,
    const uint32_t *deltas,
    size_t n,
    long long *totals);

#endif
