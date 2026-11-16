// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "cgroup_ebpfgo_rate.h"

#if defined(OS_LINUX)

static size_t cgroup_ebpfgo_rate_index(const cgroup_ebpfgo_rate_table_t *t, pid_t pid, const void *owner, bool *found)
{
    size_t left = 0;
    size_t right = t->count;
    while (left < right) {
        size_t mid = left + (right - left) / 2;
        if (t->items[mid].pid < pid ||
            (t->items[mid].pid == pid && (uintptr_t)t->items[mid].owner < (uintptr_t)owner))
            left = mid + 1;
        else
            right = mid;
    }
    *found = left < t->count && t->items[left].pid == pid && t->items[left].owner == owner;
    return left;
}

static cgroup_ebpfgo_rate_token_t *
cgroup_ebpfgo_rate_token(cgroup_ebpfgo_rate_table_t *t, pid_t pid, const void *owner, uint32_t update_every_s)
{
    bool found;
    size_t index = cgroup_ebpfgo_rate_index(t, pid, owner, &found);
    if (!found) {
        if (t->count == t->capacity) {
            size_t capacity = t->capacity ? t->capacity * 2 : 16;
            t->items = reallocz(t->items, capacity * sizeof(*t->items));
            t->capacity = capacity;
        }
        memmove(&t->items[index + 1], &t->items[index], (t->count - index) * sizeof(*t->items));
        t->items[index] = (cgroup_ebpfgo_rate_token_t){.pid = pid, .owner = owner};
        t->count++;
    }
    t->items[index].last_seen = t->generation;
    t->items[index].update_every_s = update_every_s;
    return &t->items[index];
}

void cgroup_ebpfgo_rate_begin_cycle(cgroup_ebpfgo_rate_table_t *t)
{
    t->generation++;
}

void cgroup_ebpfgo_rate_end_cycle(cgroup_ebpfgo_rate_table_t *t, uint32_t cgroup_update_every_s)
{
    size_t write = 0;
    for (size_t read = 0; read < t->count; read++) {
        if (cgroup_ebpfgo_rate_token_expired(
                t->items[read].last_seen, t->generation, t->items[read].update_every_s, cgroup_update_every_s))
            continue;
        t->items[write++] = t->items[read];
    }
    t->count = write;
}

void cgroup_ebpfgo_rate_add_pid(
    cgroup_ebpfgo_rate_table_t *t,
    pid_t pid,
    const void *owner,
    uint64_t ct,
    uint32_t update_every_s,
    uint32_t cgroup_update_every_s,
    const uint32_t *deltas,
    size_t n,
    long long *totals)
{
    if (n > CGROUP_EBPFGO_RATE_MAX_COUNTERS)
        n = CGROUP_EBPFGO_RATE_MAX_COUNTERS;
    if (!cgroup_update_every_s)
        cgroup_update_every_s = 1;
    if (!update_every_s)
        update_every_s = cgroup_update_every_s;

    // A row without a ct has never been published for this module; do not grow
    // the table for it unless a consumed row is still being held.
    if (!ct) {
        bool found;
        cgroup_ebpfgo_rate_index(t, pid, owner, &found);
        if (!found)
            return;
    }

    cgroup_ebpfgo_rate_token_t *token = cgroup_ebpfgo_rate_token(t, pid, owner, update_every_s);

    if (ct > token->ct) {
        for (size_t i = 0; i < n; i++)
            token->rate[i] = cgroup_ebpfgo_rate_normalize(deltas[i], update_every_s);

        uint64_t hold = ((uint64_t)update_every_s + cgroup_update_every_s - 1) / cgroup_update_every_s;
        token->rate_until = t->generation + (hold ? hold : 1) - 1;
        token->ct = ct;
    } else if (t->generation > token->rate_until)
        return;

    for (size_t i = 0; i < n; i++)
        totals[i] = cgroup_ebpfgo_rate_add(totals[i], token->rate[i]);
}

#endif
