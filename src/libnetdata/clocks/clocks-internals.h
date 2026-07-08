// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLOCKS_INTERNALS_H
#define NETDATA_CLOCKS_INTERNALS_H 1

#include "clocks.h"

static inline bool sleep_usec_prepare_retry_after_eintr(
        usec_t usec,
        usec_t started_monotonic_ut,
        usec_t now_monotonic_ut,
        struct timespec *req) {
    usec_t elapsed_ut = now_monotonic_ut > started_monotonic_ut ? now_monotonic_ut - started_monotonic_ut : 0;
    if(elapsed_ut >= usec)
        return false;

    usec_t remaining_ut = (usec_t)req->tv_sec * USEC_PER_SEC + (usec_t)req->tv_nsec / NSEC_PER_USEC;
    usec_t check_ut = usec - elapsed_ut;
    if(remaining_ut > check_ut) {
        *req = (struct timespec) {
            .tv_sec = (time_t)(check_ut / USEC_PER_SEC),
            .tv_nsec = (long)((check_ut % USEC_PER_SEC) * NSEC_PER_USEC),
        };
    }

    return true;
}

#endif /* NETDATA_CLOCKS_INTERNALS_H */
