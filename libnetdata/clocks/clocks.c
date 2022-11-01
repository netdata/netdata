// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// defaults are for compatibility
// call clocks_init() once, to optimize these default settings
static clockid_t clock_boottime_to_use = CLOCK_MONOTONIC;
static clockid_t clock_monotonic_to_use = CLOCK_MONOTONIC;

usec_t clock_monotonic_resolution = 1000;
usec_t clock_realtime_resolution = 1000;

#ifndef HAVE_CLOCK_GETTIME
inline int clock_gettime(clockid_t clk_id __maybe_unused, struct timespec *ts) {
    struct timeval tv;
    if(unlikely(gettimeofday(&tv, NULL) == -1)) {
        error("gettimeofday() failed.");
        return -1;
    }
    ts->tv_sec = tv.tv_sec;
    ts->tv_nsec = (long)((tv.tv_usec % USEC_PER_SEC) * NSEC_PER_USEC);
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

static usec_t get_clock_resolution(clockid_t clock) {
    struct timespec ts;
    clock_getres(clock, &ts);
    return ts.tv_sec * USEC_PER_SEC + ts.tv_nsec * NSEC_PER_USEC;
}

// perform any initializations required for clocks

void clocks_init(void) {
    // monotonic raw has to be tested before boottime
    test_clock_monotonic_raw();

    // boottime has to be tested after monotonic coarse
    test_clock_boottime();

    clock_monotonic_resolution = get_clock_resolution(clock_monotonic_to_use);
    clock_realtime_resolution = get_clock_resolution(CLOCK_REALTIME);

    // if for any reason these are zero, netdata will crash
    // since we use them as modulo to calculations
    if(!clock_realtime_resolution)
        clock_realtime_resolution = 1000;

    if(!clock_monotonic_resolution)
        clock_monotonic_resolution = 1000;
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

#ifdef __linux__
void sleep_to_absolute_time(usec_t usec) {
    static int einval_printed = 0, enotsup_printed = 0, eunknown_printed = 0;
    clockid_t clock = CLOCK_REALTIME;

    struct timespec req = {
        .tv_sec = (time_t)(usec / USEC_PER_SEC),
        .tv_nsec = (suseconds_t)((usec % USEC_PER_SEC) * NSEC_PER_USEC)
    };

    int ret = 0;
    while( (ret = clock_nanosleep(clock, TIMER_ABSTIME, &req, NULL)) != 0 ) {
        if(ret == EINTR) continue;
        else {
            if (ret == EINVAL) {
                if (!einval_printed) {
                    einval_printed++;
                    error(
                        "Invalid time given to clock_nanosleep(): clockid = %d, tv_sec = %lld, tv_nsec = %ld",
                        clock,
                        (long long)req.tv_sec,
                        req.tv_nsec);
                }
            } else if (ret == ENOTSUP) {
                if (!enotsup_printed) {
                    enotsup_printed++;
                    error(
                        "Invalid clock id given to clock_nanosleep(): clockid = %d, tv_sec = %lld, tv_nsec = %ld",
                        clock,
                        (long long)req.tv_sec,
                        req.tv_nsec);
                }
            } else {
                if (!eunknown_printed) {
                    eunknown_printed++;
                    error(
                        "Unknown return value %d from clock_nanosleep(): clockid = %d, tv_sec = %lld, tv_nsec = %ld",
                        ret,
                        clock,
                        (long long)req.tv_sec,
                        req.tv_nsec);
                }
            }
            sleep_usec(usec);
        }
    }
};
#endif

#define HEARTBEAT_ALIGNMENT_STATISTICS_SIZE 10
netdata_mutex_t heartbeat_alignment_mutex = NETDATA_MUTEX_INITIALIZER;
static size_t heartbeat_alignment_id = 0;

struct heartbeat_thread_statistics {
    size_t sequence;
    usec_t dt;
};
static struct heartbeat_thread_statistics heartbeat_alignment_values[HEARTBEAT_ALIGNMENT_STATISTICS_SIZE] = { 0 };

void heartbeat_statistics(usec_t *min_ptr, usec_t *max_ptr, usec_t *average_ptr, size_t *count_ptr) {
    struct heartbeat_thread_statistics current[HEARTBEAT_ALIGNMENT_STATISTICS_SIZE];
    static struct heartbeat_thread_statistics old[HEARTBEAT_ALIGNMENT_STATISTICS_SIZE] = { 0 };

    memcpy(current, heartbeat_alignment_values, sizeof(struct heartbeat_thread_statistics) * HEARTBEAT_ALIGNMENT_STATISTICS_SIZE);

    usec_t min = 0, max = 0, total = 0, average = 0;
    size_t i, count = 0;
    for(i = 0; i < HEARTBEAT_ALIGNMENT_STATISTICS_SIZE ;i++) {
        if(current[i].sequence == old[i].sequence) continue;
        usec_t value = current[i].dt - old[i].dt;

        if(!count) {
            min = max = total = value;
            count = 1;
        }
        else {
            total += value;
            if(value < min) min = value;
            if(value > max) max = value;
            count++;
        }
    }
    if(count)
        average = total / count;

    if(min_ptr) *min_ptr = min;
    if(max_ptr) *max_ptr = max;
    if(average_ptr) *average_ptr = average;
    if(count_ptr) *count_ptr = count;

    memcpy(old, current, sizeof(struct heartbeat_thread_statistics) * HEARTBEAT_ALIGNMENT_STATISTICS_SIZE);
}

inline void heartbeat_init(heartbeat_t *hb) {
    hb->realtime = 0ULL;
    hb->randomness = 250 * USEC_PER_MS + ((now_realtime_usec() * clock_realtime_resolution) % (250 * USEC_PER_MS));
    hb->randomness -= (hb->randomness % clock_realtime_resolution);

    netdata_mutex_lock(&heartbeat_alignment_mutex);
    hb->statistics_id = heartbeat_alignment_id;
    heartbeat_alignment_id++;
    netdata_mutex_unlock(&heartbeat_alignment_mutex);

    if(hb->statistics_id < HEARTBEAT_ALIGNMENT_STATISTICS_SIZE) {
        heartbeat_alignment_values[hb->statistics_id].dt = 0;
        heartbeat_alignment_values[hb->statistics_id].sequence = 0;
    }
}

// waits for the next heartbeat
// it waits using the monotonic clock
// it returns the dt using the realtime clock

usec_t heartbeat_next(heartbeat_t *hb, usec_t tick) {
    if(unlikely(hb->randomness > tick / 2)) {
        // TODO: The heartbeat tick should be specified at the heartbeat_init() function
        usec_t tmp = (now_realtime_usec() * clock_realtime_resolution) % (tick / 2);
        info("heartbeat randomness of %llu is too big for a tick of %llu - setting it to %llu", hb->randomness, tick, tmp);
        hb->randomness = tmp;
    }

    usec_t dt;
    usec_t now = now_realtime_usec();
    usec_t next = now - (now % tick) + tick + hb->randomness;

    // align the next time we want to the clock resolution
    if(next % clock_realtime_resolution)
        next = next - (next % clock_realtime_resolution) + clock_realtime_resolution;

    // sleep_usec() has a loop to guarantee we will sleep for at least the requested time.
    // According the specs, when we sleep for a relative time, clock adjustments should not affect the duration
    // we sleep.
    sleep_usec(next - now);
    now = now_realtime_usec();
    dt = now - hb->realtime;

    if(hb->statistics_id < HEARTBEAT_ALIGNMENT_STATISTICS_SIZE) {
        heartbeat_alignment_values[hb->statistics_id].dt += now - next;
        heartbeat_alignment_values[hb->statistics_id].sequence++;
    }

    if(unlikely(now < next)) {
        errno = 0;
        error("heartbeat clock: woke up %llu microseconds earlier than expected (can be due to the CLOCK_REALTIME set to the past).", next - now);
    }
    else if(unlikely(now - next >  tick / 2)) {
        errno = 0;
        error("heartbeat clock: woke up %llu microseconds later than expected (can be due to system load or the CLOCK_REALTIME set to the future).", now - next);
    }

    if(unlikely(!hb->realtime)) {
        // the first time return zero
        dt = 0;
    }

    hb->realtime = now;
    return dt;
}

void sleep_usec(usec_t usec) {
    // we expect microseconds (1.000.000 per second)
    // but timespec is nanoseconds (1.000.000.000 per second)
    struct timespec rem, req = {
            .tv_sec = (time_t) (usec / USEC_PER_SEC),
            .tv_nsec = (suseconds_t) ((usec % USEC_PER_SEC) * NSEC_PER_USEC)
    };

#ifdef __linux__
    while ((errno = clock_nanosleep(CLOCK_REALTIME, 0, &req, &rem)) != 0) {
#else
    while ((errno = nanosleep(&req, &rem)) != 0) {
#endif
        if (likely(errno == EINTR)) {
            req.tv_sec = rem.tv_sec;
            req.tv_nsec = rem.tv_nsec;
        } else {
#ifdef __linux__
            error("Cannot clock_nanosleep(CLOCK_REALTIME) for %llu microseconds.", usec);
#else
            error("Cannot nanosleep() for %llu microseconds.", usec);
#endif
            break;
        }
    }
}

static inline collected_number uptime_from_boottime(void) {
#ifdef CLOCK_BOOTTIME_IS_AVAILABLE
    return (collected_number)(now_boottime_usec() / USEC_PER_MS);
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

    return (collected_number)(strtondd(procfile_lineword(read_proc_uptime_ff, 0, 0), NULL) * 1000.0);
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
