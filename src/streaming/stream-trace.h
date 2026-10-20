// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_TRACE_H
#define NETDATA_STREAM_TRACE_H 1

#include <limits.h>
#include <stdint.h>
#include <stdio.h>
#include <time.h>

#include "libnetdata/clocks/time_t_arithmetic.h"

#define STREAM_TRACE_PREFIX_SIZE 30

static inline uintmax_t stream_trace_positive_delta_seconds(time_t now, time_t started) {
    if(now <= started)
        return 0;

    if(started < 0 && now >= 0)
        return (uintmax_t)now + (uintmax_t)(-(started + 1)) + 1;

    return (uintmax_t)(now - started);
}

static inline void stream_trace_format_elapsed_prefix(
        char prefix[STREAM_TRACE_PREFIX_SIZE], struct timespec now, struct timespec started) {
    const uintmax_t max_elapsed_sec = (uintmax_t)UINT16_MAX * 86400U + 86399U;
    uintmax_t elapsed_sec = 0;
    long elapsed_nsec = 0;

    if(now.tv_sec > started.tv_sec) {
        elapsed_sec = stream_trace_positive_delta_seconds(now.tv_sec, started.tv_sec);

        if(now.tv_nsec < started.tv_nsec) {
            elapsed_sec--;
            elapsed_nsec = now.tv_nsec + (1000000000L - started.tv_nsec);
        }
        else
            elapsed_nsec = now.tv_nsec - started.tv_nsec;
    }
    else if(now.tv_sec == started.tv_sec && now.tv_nsec > started.tv_nsec)
        elapsed_nsec = now.tv_nsec - started.tv_nsec;

    if(elapsed_sec > max_elapsed_sec) {
        elapsed_sec = max_elapsed_sec;
        elapsed_nsec = 999999999L;
    }

    unsigned int days = (unsigned int)(elapsed_sec / 86400U);
    unsigned int hours = (unsigned int)((elapsed_sec % 86400U) / 3600U);
    unsigned int minutes = (unsigned int)((elapsed_sec % 3600U) / 60U);
    unsigned int seconds = (unsigned int)(elapsed_sec % 60U);
    unsigned int milliseconds = (unsigned int)(elapsed_nsec / 1000000L);

    (void)snprintf(prefix, STREAM_TRACE_PREFIX_SIZE, "%03ud.%02u:%02u:%02u.%03u ",
                   days, hours, minutes, seconds, milliseconds);
}

#endif // NETDATA_STREAM_TRACE_H
