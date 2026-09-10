// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>

#include "../cgroup_ebpfgo_fd.h"

static int expect_rate(const char *name, long long actual, long long expected)
{
    if (actual == expected)
        return 0;

    fprintf(stderr, "%s: got %lld, want %lld\n", name, actual, expected);
    return 1;
}

int main(void)
{
    long long open = 0;
    long long close = 0;

    /* PID one reports 10 calls in one second; PID two reports 100 in ten. */
    open = cgroup_ebpfgo_fd_add_rate(open, cgroup_ebpfgo_fd_normalize_rate(10, 1));
    open = cgroup_ebpfgo_fd_add_rate(open, cgroup_ebpfgo_fd_normalize_rate(100, 10));
    close = cgroup_ebpfgo_fd_add_rate(close, cgroup_ebpfgo_fd_normalize_rate(10, 1));
    close = cgroup_ebpfgo_fd_add_rate(close, cgroup_ebpfgo_fd_normalize_rate(100, 10));

    int failed = 0;
    failed |= expect_rate("mixed-cadence open", open, 20 * CGROUP_EBPFGO_FD_RATE_SCALE);
    failed |= expect_rate("mixed-cadence close", close, 20 * CGROUP_EBPFGO_FD_RATE_SCALE);

    /* A ten-second FD row remains consumed across three one-second cgroup.procs
     * gaps, then expires after a complete producer interval. */
    failed |= expect_rate("fd token grace", cgroup_ebpfgo_fd_token_grace_generations(10, 1), 10);
    failed |= expect_rate("fd token survives short gap", cgroup_ebpfgo_fd_token_expired(1, 4, 10, 1), 0);
    failed |= expect_rate("fd token survives producer interval boundary", cgroup_ebpfgo_fd_token_expired(1, 11, 10, 1), 0);
    failed |= expect_rate("fd token expires after producer interval", cgroup_ebpfgo_fd_token_expired(1, 12, 10, 1), 1);
    return failed;
}
