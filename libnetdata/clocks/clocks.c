// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// defaults are for compatibility
// call clocks_init() once, to optimize these default settings
static clockid_t clock_boottime_to_use = CLOCK_MONOTONIC;
static clockid_t clock_monotonic_to_use = CLOCK_MONOTONIC;

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

// Similar to CLOCK_MONOTONIC, but provides access to a raw hardware-based time that is not subject to NTP adjustments
// or the incremental adjustments performed by adjtime(3).  This clock does not count time that the system is suspended

static void test_clock_monotonic_raw(void) {
#ifdef CLOCK_MONOTONIC_RAW
    struct timespec ts;
    if(clock_gettime(CLOCK_MONOTONIC_RAW, &ts) == -1 && errno == EINVAL)
        clock_monotonic_to_use = CLOCK_MONOTONIC;
    else
        clock_monotonic_to_use = CLOCK_MONOTONIC_RAW;
#else
    clock_monotonic_to_use = CLOCK_MONOTONIC;
#endif
}

// When running a binary with CLOCK_BOOTTIME defined on a system with a linux kernel older than Linux 2.6.39 the
// clock_gettime(2) system call fails with EINVAL. In that case it must fall-back to CLOCK_MONOTONIC.

static void test_clock_boottime(void) {
    struct timespec ts;
    if(clock_gettime(CLOCK_BOOTTIME, &ts) == -1 && errno == EINVAL)
        clock_boottime_to_use = clock_monotonic_to_use;
    else
        clock_boottime_to_use = CLOCK_BOOTTIME;
}

// perform any initializations required for clocks

void clocks_init(void) {
    // monotonic raw has to be tested before boottime
    test_clock_monotonic_raw();

    // boottime has to be tested after monotonic coarse
    test_clock_boottime();
}

inline time_t now_sec(clockid_t clk_id) {
    struct timespec ts;
    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        error("clock_gettime(%d, &timespec) failed.", clk_id);
        return 0;
    }
    return ts.tv_sec;
}

inline usec_t now_usec(clockid_t clk_id) {
    struct timespec ts;
    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        error("clock_gettime(%d, &timespec) failed.", clk_id);
        return 0;
    }
    return (usec_t)ts.tv_sec * USEC_PER_SEC + (ts.tv_nsec % NSEC_PER_SEC) / NSEC_PER_USEC;
}

inline int now_timeval(clockid_t clk_id, struct timeval *tv) {
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
    return now_sec(clock_monotonic_to_use);
}

inline usec_t now_monotonic_usec(void) {
    return now_usec(clock_monotonic_to_use);
}

inline int now_monotonic_timeval(struct timeval *tv) {
    return now_timeval(clock_monotonic_to_use, tv);
}

inline time_t now_monotonic_high_precision_sec(void) {
    return now_sec(CLOCK_MONOTONIC);
}

inline usec_t now_monotonic_high_precision_usec(void) {
    return now_usec(CLOCK_MONOTONIC);
}

inline int now_monotonic_high_precision_timeval(struct timeval *tv) {
    return now_timeval(CLOCK_MONOTONIC, tv);
}

inline time_t now_boottime_sec(void) {
    return now_sec(clock_boottime_to_use);
}

inline usec_t now_boottime_usec(void) {
    return now_usec(clock_boottime_to_use);
}

inline int now_boottime_timeval(struct timeval *tv) {
    return now_timeval(clock_boottime_to_use, tv);
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

inline void heartbeat_init(heartbeat_t *hb) {
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

static inline collected_number uptime_from_boottime(void) {
#ifdef CLOCK_BOOTTIME_IS_AVAILABLE
    return now_boottime_usec() / 1000;
#else
    error("uptime cannot be read from CLOCK_BOOTTIME on this system.");
    return 0;
#endif
}

static procfile *read_proc_uptime_ff = NULL;
static inline collected_number read_proc_uptime(char *filename) {
    if(unlikely(!read_proc_uptime_ff)) {
        read_proc_uptime_ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!read_proc_uptime_ff)) return 0;
    }

    read_proc_uptime_ff = procfile_readall(read_proc_uptime_ff);
    if(unlikely(!read_proc_uptime_ff)) return 0;

    if(unlikely(procfile_lines(read_proc_uptime_ff) < 1)) {
        error("/proc/uptime has no lines.");
        return 0;
    }
    if(unlikely(procfile_linewords(read_proc_uptime_ff, 0) < 1)) {
        error("/proc/uptime has less than 1 word in it.");
        return 0;
    }

    return (collected_number)(strtold(procfile_lineword(read_proc_uptime_ff, 0, 0), NULL) * 1000.0);
}

inline collected_number uptime_msec(char *filename){
    static int use_boottime = -1;

    if(unlikely(use_boottime == -1)) {
        collected_number uptime_boottime = uptime_from_boottime();
        collected_number uptime_proc     = read_proc_uptime(filename);

        long long delta = (long long)uptime_boottime - (long long)uptime_proc;
        if(delta < 0) delta = -delta;

        if(delta <= 1000 && uptime_boottime != 0) {
            procfile_close(read_proc_uptime_ff);
            info("Using now_boottime_usec() for uptime (dt is %lld ms)", delta);
            use_boottime = 1;
        }
        else if(uptime_proc != 0) {
            info("Using /proc/uptime for uptime (dt is %lld ms)", delta);
            use_boottime = 0;
        }
        else {
            error("Cannot find any way to read uptime on this system.");
            return 1;
        }
    }

    collected_number uptime;
    if(use_boottime)
        uptime = uptime_from_boottime();
    else
        uptime = read_proc_uptime(filename);

    return uptime;
}
