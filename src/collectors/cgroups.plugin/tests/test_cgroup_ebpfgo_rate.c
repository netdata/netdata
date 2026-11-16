// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "../cgroup_ebpfgo_rate.h"

static int expect_rate(const char *name, long long actual, long long expected)
{
    if (actual == expected)
        return 0;

    fprintf(stderr, "%s: got %lld, want %lld\n", name, actual, expected);
    return 1;
}

static long long refresh(cgroup_ebpfgo_rate_table_t *t, uint64_t ct, uint32_t delta)
{
    static const int owner;
    long long total = 0;
    const uint32_t deltas[] = {delta};

    cgroup_ebpfgo_rate_begin_cycle(t);
    cgroup_ebpfgo_rate_add_pid(t, 42, &owner, ct, 10, 1, deltas, 1, &total);
    cgroup_ebpfgo_rate_end_cycle(t, 1);
    return total;
}

int main(void)
{
    long long open = 0;
    long long close = 0;

    /* PID one reports 10 calls in one second; PID two reports 100 in ten. */
    open = cgroup_ebpfgo_rate_add(open, cgroup_ebpfgo_rate_normalize(10, 1));
    open = cgroup_ebpfgo_rate_add(open, cgroup_ebpfgo_rate_normalize(100, 10));
    close = cgroup_ebpfgo_rate_add(close, cgroup_ebpfgo_rate_normalize(10, 1));
    close = cgroup_ebpfgo_rate_add(close, cgroup_ebpfgo_rate_normalize(100, 10));

    int failed = 0;
    failed |= expect_rate("mixed-cadence open", open, 20 * CGROUP_EBPFGO_RATE_SCALE);
    failed |= expect_rate("mixed-cadence close", close, 20 * CGROUP_EBPFGO_RATE_SCALE);

    /* A ten-second row remains consumed across three one-second cgroup.procs
     * gaps, then expires after a complete producer interval. */
    failed |= expect_rate("token grace", cgroup_ebpfgo_rate_token_grace_generations(10, 1), 10);
    failed |= expect_rate("token survives short gap", cgroup_ebpfgo_rate_token_expired(1, 4, 10, 1), 0);
    failed |= expect_rate("token survives producer interval boundary", cgroup_ebpfgo_rate_token_expired(1, 11, 10, 1), 0);
    failed |= expect_rate("token expires after producer interval", cgroup_ebpfgo_rate_token_expired(1, 12, 10, 1), 1);

    /* A row of 100 calls over ten seconds is shown as 10/s for each of the ten
     * one-second refreshes it covers, never replayed, and then drops to 0. */
    cgroup_ebpfgo_rate_table_t t = {0};
    failed |= expect_rate("unpublished row adds nothing", refresh(&t, 0, 100), 0);
    failed |= expect_rate("unpublished row creates no token", (long long)t.count, 0);
    const long long ten_per_s = 10 * (long long)CGROUP_EBPFGO_RATE_SCALE;
    failed |= expect_rate("row consumed", refresh(&t, 5, 100), ten_per_s);
    for (int i = 1; i < 10; i++)
        failed |= expect_rate("rate held for the producer interval", refresh(&t, 5, 100), ten_per_s);
    failed |= expect_rate("rate expires after the interval", refresh(&t, 5, 100), 0);
    failed |= expect_rate("newer row consumed", refresh(&t, 6, 20), 2 * (long long)CGROUP_EBPFGO_RATE_SCALE);

    freez(t.items);
    return failed;
}
