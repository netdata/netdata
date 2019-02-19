// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static int clock_boottime_valid = 1;

#ifndef HAVE_CLOCK_GETTIME
inline int clock_gettime(clockid_t clk_id, struct timespec *ts) {
    struct timeval tv;
    if(unlikely(gettimeofday(&tv, NULL) == -1)) {
        error("gettimeofday() failed.");
        return -1;
    }
    ts->tv_sec = tv.tv_sec;
    ts->tv_nsec = (tv.tv_usec % USEC_PER_SEC) * NSEC_PER_USEC;
    return 0;
}
#endif

void test_clock_boottime(void) {
    struct timespec ts;
    if(clock_gettime(CLOCK_BOOTTIME, &ts) == -1 && errno == EINVAL)
        clock_boottime_valid = 0;
}

static inline time_t now_sec(clockid_t clk_id) {
    struct timespec ts;
    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        error("clock_gettime(%d, &timespec) failed.", clk_id);
        return 0;
    }
    return ts.tv_sec;
}

static inline usec_t now_usec(clockid_t clk_id) {
    struct timespec ts;
    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        error("clock_gettime(%d, &timespec) failed.", clk_id);
        return 0;
    }
    return (usec_t)ts.tv_sec * USEC_PER_SEC + (ts.tv_nsec % NSEC_PER_SEC) / NSEC_PER_USEC;
}

static inline int now_timeval(clockid_t clk_id, struct timeval *tv) {
    struct timespec ts;

    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        error("clock_gettime(%d, &timespec) failed.", clk_id);
        tv->tv_sec = 0;
        tv->tv_usec = 0;
        return -1;
    }

    tv->tv_sec = ts.tv_sec;
    tv->tv_usec = (suseconds_t)((ts.tv_nsec % NSEC_PER_SEC) / NSEC_PER_USEC);
    return 0;
}

inline time_t now_realtime_sec(void) {
    return now_sec(CLOCK_REALTIME);
}

inline usec_t now_realtime_usec(void) {
    return now_usec(CLOCK_REALTIME);
}

inline int now_realtime_timeval(struct timeval *tv) {
    return now_timeval(CLOCK_REALTIME, tv);
}

inline time_t now_monotonic_sec(void) {
    return now_sec(CLOCK_MONOTONIC);
}

inline usec_t now_monotonic_usec(void) {
    return now_usec(CLOCK_MONOTONIC);
}

inline int now_monotonic_timeval(struct timeval *tv) {
    return now_timeval(CLOCK_MONOTONIC, tv);
}

inline time_t now_boottime_sec(void) {
    return now_sec(likely(clock_boottime_valid) ? CLOCK_BOOTTIME : CLOCK_MONOTONIC);
}

inline usec_t now_boottime_usec(void) {
    return now_usec(likely(clock_boottime_valid) ? CLOCK_BOOTTIME : CLOCK_MONOTONIC);
}

inline int now_boottime_timeval(struct timeval *tv) {
    return now_timeval(likely(clock_boottime_valid) ? CLOCK_BOOTTIME : CLOCK_MONOTONIC, tv);
}

inline usec_t timeval_usec(struct timeval *tv) {
    return (usec_t)tv->tv_sec * USEC_PER_SEC + (tv->tv_usec % USEC_PER_SEC);
}

inline msec_t timeval_msec(struct timeval *tv) {
    return (msec_t)tv->tv_sec * MSEC_PER_SEC + ((tv->tv_usec % USEC_PER_SEC) / MSEC_PER_SEC);
}

inline susec_t dt_usec_signed(struct timeval *now, struct timeval *old) {
    usec_t ts1 = timeval_usec(now);
    usec_t ts2 = timeval_usec(old);

    if(likely(ts1 >= ts2)) return (susec_t)(ts1 - ts2);
    return -((susec_t)(ts2 - ts1));
}

inline usec_t dt_usec(struct timeval *now, struct timeval *old) {
    usec_t ts1 = timeval_usec(now);
    usec_t ts2 = timeval_usec(old);
    return (ts1 > ts2) ? (ts1 - ts2) : (ts2 - ts1);
}

inline void heartbeat_init(heartbeat_t *hb)
{
    hb->monotonic = hb->realtime = 0ULL;
}

// waits for the next heartbeat
// it waits using the monotonic clock
// it returns the dt using the realtime clock

usec_t heartbeat_next(heartbeat_t *hb, usec_t tick) {
    heartbeat_t now;
    now.monotonic = now_monotonic_usec();
    now.realtime  = now_realtime_usec();

    usec_t next_monotonic = now.monotonic - (now.monotonic % tick) + tick;

    while(now.monotonic < next_monotonic) {
        sleep_usec(next_monotonic - now.monotonic);
        now.monotonic = now_monotonic_usec();
        now.realtime  = now_realtime_usec();
    }

    if(likely(hb->realtime != 0ULL)) {
        usec_t dt_monotonic = now.monotonic - hb->monotonic;
        usec_t dt_realtime  = now.realtime - hb->realtime;

        hb->monotonic = now.monotonic;
        hb->realtime  = now.realtime;

        if(unlikely(dt_monotonic >= tick + tick / 2)) {
            errno = 0;
            error("heartbeat missed %llu monotonic microseconds", dt_monotonic - tick);
        }

        return dt_realtime;
    }
    else {
        hb->monotonic = now.monotonic;
        hb->realtime  = now.realtime;
        return 0ULL;
    }
}

// returned the elapsed time, since the last heartbeat
// using the monotonic clock

inline usec_t heartbeat_monotonic_dt_to_now_usec(heartbeat_t *hb) {
    if(!hb || !hb->monotonic) return 0ULL;
    return now_monotonic_usec() - hb->monotonic;
}

int sleep_usec(usec_t usec) {

#ifndef NETDATA_WITH_USLEEP
    // we expect microseconds (1.000.000 per second)
    // but timespec is nanoseconds (1.000.000.000 per second)
    struct timespec rem, req = {
            .tv_sec = (time_t) (usec / 1000000),
            .tv_nsec = (suseconds_t) ((usec % 1000000) * 1000)
    };

    while (nanosleep(&req, &rem) == -1) {
        if (likely(errno == EINTR)) {
            debug(D_SYSTEM, "nanosleep() interrupted (while sleeping for %llu microseconds).", usec);
            req.tv_sec = rem.tv_sec;
            req.tv_nsec = rem.tv_nsec;
        } else {
            error("Cannot nanosleep() for %llu microseconds.", usec);
            break;
        }
    }

    return 0;
#else
    int ret = usleep(usec);
    if(unlikely(ret == -1 && errno == EINVAL)) {
        // on certain systems, usec has to be up to 999999
        if(usec > 999999) {
            int counter = usec / 999999;
            while(counter--)
                usleep(999999);

            usleep(usec % 999999);
        }
        else {
            error("Cannot usleep() for %llu microseconds.", usec);
            return ret;
        }
    }

    if(ret != 0)
        error("usleep() failed for %llu microseconds.", usec);

    return ret;
#endif
}
