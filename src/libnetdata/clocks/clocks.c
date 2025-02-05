// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// defaults are for compatibility
// call clocks_init() once, to optimize these default settings
static clockid_t clock_boottime_to_use = CLOCK_MONOTONIC;
static clockid_t clock_monotonic_to_use = CLOCK_MONOTONIC;

// the default clock resolution is 1ms
#define DEFAULT_CLOCK_RESOLUTION_UT ((usec_t)0 * USEC_PER_SEC + (usec_t)1 * USEC_PER_MS)

// the max clock resolution is 10ms
#define MAX_CLOCK_RESOLUTION_UT ((usec_t)0 * USEC_PER_SEC + (usec_t)10 * USEC_PER_MS)

usec_t clock_monotonic_resolution = DEFAULT_CLOCK_RESOLUTION_UT;
usec_t clock_realtime_resolution = DEFAULT_CLOCK_RESOLUTION_UT;

#ifndef HAVE_CLOCK_GETTIME
inline int clock_gettime(clockid_t clk_id __maybe_unused, struct timespec *ts) {
    struct timeval tv;
    if(unlikely(gettimeofday(&tv, NULL) == -1)) {
        netdata_log_error("gettimeofday() failed.");
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
    struct timespec ts = { 0 };

    if(clock_getres(clock, &ts) == 0) {
        usec_t ret = (usec_t)ts.tv_sec * USEC_PER_SEC + (usec_t)ts.tv_nsec / NSEC_PER_USEC;
        if(!ret && ts.tv_nsec > 0 && ts.tv_nsec < (long int)NSEC_PER_USEC)
            return (usec_t)1;

        else if(ret > MAX_CLOCK_RESOLUTION_UT) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "clock_getres(%d) returned %"PRIu64" usec is out of range, using defaults for clock resolution.", (int)clock, ret);
            return DEFAULT_CLOCK_RESOLUTION_UT;
        }

        return ret;
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_ERR, "clock_getres(%d) failed, using defaults for clock resolution.", (int)clock);
        return DEFAULT_CLOCK_RESOLUTION_UT;
    }
}

// perform any initializations required for clocks

static __attribute__((constructor)) void clocks_init(void) {
    os_get_system_HZ();

    // monotonic raw has to be tested before boottime
    test_clock_monotonic_raw();

    // boottime has to be tested after monotonic coarse
    test_clock_boottime();

    clock_monotonic_resolution = get_clock_resolution(clock_monotonic_to_use);
    clock_realtime_resolution = get_clock_resolution(CLOCK_REALTIME);

#if defined(OS_WINDOWS)
    timeBeginPeriod(1);
    clock_monotonic_resolution = 1 * USEC_PER_MS;
    clock_realtime_resolution = 1 * USEC_PER_MS;
#endif
}

static __attribute__((destructor)) void clocks_fin(void) {
#if defined(OS_WINDOWS)
    timeEndPeriod(1);
#endif
}

ALWAYS_INLINE time_t now_sec(clockid_t clk_id) {
    struct timespec ts;
    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        netdata_log_error("clock_gettime(%ld, &timespec) failed.", (long int)clk_id);
        return 0;
    }
    return ts.tv_sec;
}

ALWAYS_INLINE usec_t now_usec(clockid_t clk_id) {
    struct timespec ts;
    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        netdata_log_error("clock_gettime(%ld, &timespec) failed.", (long int)clk_id);
        return 0;
    }
    return (usec_t)ts.tv_sec * USEC_PER_SEC + (usec_t)(ts.tv_nsec % NSEC_PER_SEC) / NSEC_PER_USEC;
}

ALWAYS_INLINE int now_timeval(clockid_t clk_id, struct timeval *tv) {
    struct timespec ts;

    if(unlikely(clock_gettime(clk_id, &ts) == -1)) {
        netdata_log_error("clock_gettime(%ld, &timespec) failed.", (long int)clk_id);
        tv->tv_sec = 0;
        tv->tv_usec = 0;
        return -1;
    }

    tv->tv_sec = ts.tv_sec;
    tv->tv_usec = (suseconds_t)((ts.tv_nsec % NSEC_PER_SEC) / NSEC_PER_USEC);
    return 0;
}

ALWAYS_INLINE time_t now_realtime_sec(void) {
    return now_sec(CLOCK_REALTIME);
}

ALWAYS_INLINE msec_t now_realtime_msec(void) {
    return now_usec(CLOCK_REALTIME) / USEC_PER_MS;
}

ALWAYS_INLINE usec_t now_realtime_usec(void) {
    return now_usec(CLOCK_REALTIME);
}

ALWAYS_INLINE int now_realtime_timeval(struct timeval *tv) {
    return now_timeval(CLOCK_REALTIME, tv);
}

ALWAYS_INLINE time_t now_monotonic_sec(void) {
    return now_sec(clock_monotonic_to_use);
}

ALWAYS_INLINE usec_t now_monotonic_usec(void) {
    return now_usec(clock_monotonic_to_use);
}

ALWAYS_INLINE int now_monotonic_timeval(struct timeval *tv) {
    return now_timeval(clock_monotonic_to_use, tv);
}

ALWAYS_INLINE time_t now_monotonic_high_precision_sec(void) {
    return now_sec(CLOCK_MONOTONIC);
}

ALWAYS_INLINE usec_t now_monotonic_high_precision_usec(void) {
    return now_usec(CLOCK_MONOTONIC);
}

ALWAYS_INLINE int now_monotonic_high_precision_timeval(struct timeval *tv) {
    return now_timeval(CLOCK_MONOTONIC, tv);
}

ALWAYS_INLINE time_t now_boottime_sec(void) {
    return now_sec(clock_boottime_to_use);
}

ALWAYS_INLINE usec_t now_boottime_usec(void) {
    return now_usec(clock_boottime_to_use);
}

ALWAYS_INLINE int now_boottime_timeval(struct timeval *tv) {
    return now_timeval(clock_boottime_to_use, tv);
}

ALWAYS_INLINE usec_t timeval_usec(struct timeval *tv) {
    return (usec_t)tv->tv_sec * USEC_PER_SEC + (tv->tv_usec % USEC_PER_SEC);
}

ALWAYS_INLINE msec_t timeval_msec(struct timeval *tv) {
    return (msec_t)tv->tv_sec * MSEC_PER_SEC + ((tv->tv_usec % USEC_PER_SEC) / MSEC_PER_SEC);
}

ALWAYS_INLINE susec_t dt_usec_signed(struct timeval *now, struct timeval *old) {
    usec_t ts1 = timeval_usec(now);
    usec_t ts2 = timeval_usec(old);

    if(likely(ts1 >= ts2)) return (susec_t)(ts1 - ts2);
    return -((susec_t)(ts2 - ts1));
}

ALWAYS_INLINE usec_t dt_usec(struct timeval *now, struct timeval *old) {
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

    errno = 0;
    int ret = 0;
    while( (ret = clock_nanosleep(clock, TIMER_ABSTIME, &req, NULL)) != 0 ) {
        if(ret == EINTR) {
            errno = 0;
            continue;
        }
        else {
            if (ret == EINVAL) {
                if (!einval_printed) {
                    einval_printed++;
                    netdata_log_error("Invalid time given to clock_nanosleep(): clockid = %d, tv_sec = %lld, tv_nsec = %ld",
                                      clock,
                                      (long long)req.tv_sec,
                                      req.tv_nsec);
                }
            } else if (ret == ENOTSUP) {
                if (!enotsup_printed) {
                    enotsup_printed++;
                    netdata_log_error("Invalid clock id given to clock_nanosleep(): clockid = %d, tv_sec = %lld, tv_nsec = %ld",
                                      clock,
                                      (long long)req.tv_sec,
                                      req.tv_nsec);
                }
            } else {
                if (!eunknown_printed) {
                    eunknown_printed++;
                    netdata_log_error("Unknown return value %d from clock_nanosleep(): clockid = %d, tv_sec = %lld, tv_nsec = %ld",
                                      ret,
                                      clock,
                                      (long long)req.tv_sec,
                                      req.tv_nsec);
                }
            }
            sleep_usec(usec);
        }
    }
}
#endif

#define HEARTBEAT_MIN_OFFSET_UT     (150 * USEC_PER_MS)
#define HEARTBEAT_RANDOM_OFFSET_UT  (350 * USEC_PER_MS)

#define HEARTBEAT_ALIGNMENT_STATISTICS_SIZE 20
static SPINLOCK heartbeat_alignment_spinlock = SPINLOCK_INITIALIZER;
static size_t heartbeat_alignment_id = 0;

struct heartbeat_thread_statistics {
    pid_t tid;
    size_t sequence;
    usec_t dt;
    usec_t randomness;
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

static XXH64_hash_t heartbeat_hash(usec_t step, size_t statistics_id) {
    struct {
        usec_t step;
        pid_t pid;
        pid_t tid;
        usec_t now_ut;
        size_t statistics_id;
        char tag[ND_THREAD_TAG_MAX + 1];
    } key = {
        .step = step,
        .pid = getpid(),
        .tid = os_gettid(),
        .now_ut = now_realtime_usec(),
        .statistics_id = statistics_id,
    };
    strncpyz(key.tag, nd_thread_tag(), sizeof(key.tag) - 1);
    return XXH3_64bits(&key, sizeof(key));
}

static usec_t heartbeat_randomness(XXH64_hash_t hash) {
    usec_t offset_ut = HEARTBEAT_MIN_OFFSET_UT + (hash % HEARTBEAT_RANDOM_OFFSET_UT);

    // Calculate the scheduler tick interval in microseconds
    usec_t scheduler_step_ut = USEC_PER_SEC / (usec_t)system_hz;
    if(scheduler_step_ut > 10 * USEC_PER_MS)
        scheduler_step_ut = 10 * USEC_PER_MS;

    // if the offset is close to the scheduler tick, move it away from it
    if(offset_ut % scheduler_step_ut < scheduler_step_ut / 4)
        offset_ut += scheduler_step_ut / 4;

    return offset_ut;
}

inline void heartbeat_init(heartbeat_t *hb, usec_t step) {
    if(!step) step = USEC_PER_SEC;

    spinlock_lock(&heartbeat_alignment_spinlock);
    hb->statistics_id = heartbeat_alignment_id;
    heartbeat_alignment_id++;
    spinlock_unlock(&heartbeat_alignment_spinlock);

    hb->step = step;
    hb->realtime = 0ULL;
    hb->hash = heartbeat_hash(hb->step, hb->statistics_id);
    hb->randomness = heartbeat_randomness(hb->hash);

    if(hb->statistics_id < HEARTBEAT_ALIGNMENT_STATISTICS_SIZE) {
        heartbeat_alignment_values[hb->statistics_id].dt = 0;
        heartbeat_alignment_values[hb->statistics_id].sequence = 0;
        heartbeat_alignment_values[hb->statistics_id].randomness = hb->randomness;
        heartbeat_alignment_values[hb->statistics_id].tid = os_gettid();
    }
}

// waits for the next heartbeat
// it waits using the monotonic clock
// it returns the dt using the realtime clock

usec_t heartbeat_next(heartbeat_t *hb) {
    usec_t tick = hb->step;

    usec_t dt;
    usec_t now = now_realtime_usec();
    usec_t next = now - (now % tick) + tick + hb->randomness;

    // align the next time we want to the clock resolution
    if(next % clock_realtime_resolution)
        next = next - (next % clock_realtime_resolution) + clock_realtime_resolution;

    // sleep_usec() has a loop to guarantee we will sleep for at least the requested time.
    // According to the specs, when we sleep for a relative time, clock adjustments should
    // not affect the duration we sleep.
    sleep_usec_with_now(next - now, now);
    spinlock_lock(&heartbeat_alignment_spinlock);
    now = now_realtime_usec();
    spinlock_unlock(&heartbeat_alignment_spinlock);

    dt = now - hb->realtime;

    if(hb->statistics_id < HEARTBEAT_ALIGNMENT_STATISTICS_SIZE) {
        heartbeat_alignment_values[hb->statistics_id].dt += now - next;
        heartbeat_alignment_values[hb->statistics_id].sequence++;
    }

    if(unlikely(now < next)) {
        errno_clear();
        nd_log_limit_static_global_var(erl, 10, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                     "heartbeat clock: woke up %"PRIu64" microseconds earlier than expected "
                     "(can be due to the CLOCK_REALTIME set to the past).",
                     next - now);
    }
    else if(unlikely(now - next >  tick / 2)) {
        errno_clear();
        nd_log_limit_static_global_var(erl, 10, 0);
        nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                     "heartbeat clock: woke up %"PRIu64" microseconds later than expected "
                     "(can be due to system load or the CLOCK_REALTIME set to the future).",
                     now - next);
    }

    if(unlikely(!hb->realtime)) {
        // the first time return zero
        dt = 0;
    }

    hb->realtime = now;
    return dt;
}

#if defined(OS_WINDOWS)
void sleep_usec_with_now(usec_t usec, usec_t started_ut) {
    if (!started_ut)
        started_ut = now_realtime_usec();

    usec_t end_ut = started_ut + usec;
    usec_t remaining_ut = usec;

    while (remaining_ut >= clock_realtime_resolution) {
        DWORD sleep_ms = (DWORD) (remaining_ut / USEC_PER_MS);
        Sleep(sleep_ms);

        usec_t now_ut = now_realtime_usec();
        if (now_ut >= end_ut)
            break;

        remaining_ut = end_ut - now_ut;
    }
}
#else
void sleep_usec_with_now(usec_t usec, usec_t started_ut) {
    // we expect microseconds (1.000.000 per second)
    // but timespec is nanoseconds (1.000.000.000 per second)
    struct timespec rem = { 0, 0 }, req = {
            .tv_sec = (time_t) (usec / USEC_PER_SEC),
            .tv_nsec = (suseconds_t) ((usec % USEC_PER_SEC) * NSEC_PER_USEC)
    };

    // make sure errno is not EINTR
    errno_clear();

    if(!started_ut)
        started_ut = now_realtime_usec();

    usec_t end_ut = started_ut + usec;

    while (nanosleep(&req, &rem) != 0) {
        if (likely(errno == EINTR && (rem.tv_sec || rem.tv_nsec))) {
            req = rem;
            rem = (struct timespec){ 0, 0 };

            // break an infinite loop
            errno_clear();

            usec_t now_ut = now_realtime_usec();
            if(now_ut >= end_ut)
                break;

            usec_t remaining_ut = (usec_t)req.tv_sec * USEC_PER_SEC + (usec_t)req.tv_nsec * NSEC_PER_USEC > usec;
            usec_t check_ut = now_ut - started_ut;
            if(remaining_ut > check_ut) {
                req = (struct timespec){
                    .tv_sec = (time_t) ( check_ut / USEC_PER_SEC),
                    .tv_nsec = (suseconds_t) ((check_ut % USEC_PER_SEC) * NSEC_PER_USEC)
                };
            }
        }
        else {
            netdata_log_error("Cannot nanosleep() for %"PRIu64" microseconds.", usec);
            break;
        }
    }
}
#endif

static inline collected_number uptime_from_boottime(void) {
#ifdef CLOCK_BOOTTIME_IS_AVAILABLE
    return (collected_number)(now_boottime_usec() / USEC_PER_MS);
#else
    netdata_log_error("uptime cannot be read from CLOCK_BOOTTIME on this system.");
    return 0;
#endif
}

static procfile *read_proc_uptime_ff = NULL;
static inline collected_number read_proc_uptime(const char *filename) {
    if(unlikely(!read_proc_uptime_ff)) {
        read_proc_uptime_ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!read_proc_uptime_ff)) return 0;
    }

    read_proc_uptime_ff = procfile_readall(read_proc_uptime_ff);
    if(unlikely(!read_proc_uptime_ff)) return 0;

    if(unlikely(procfile_lines(read_proc_uptime_ff) < 1)) {
        netdata_log_error("/proc/uptime has no lines.");
        return 0;
    }
    if(unlikely(procfile_linewords(read_proc_uptime_ff, 0) < 1)) {
        netdata_log_error("/proc/uptime has less than 1 word in it.");
        return 0;
    }

    return (collected_number)(strtondd(procfile_lineword(read_proc_uptime_ff, 0, 0), NULL) * 1000.0);
}

inline collected_number uptime_msec(const char *filename){
    static int use_boottime = -1;

    if(unlikely(use_boottime == -1)) {
        collected_number uptime_boottime = uptime_from_boottime();
        collected_number uptime_proc     = read_proc_uptime(filename);

        long long delta = (long long)uptime_boottime - (long long)uptime_proc;
        if(delta < 0) delta = -delta;

        if(delta <= 1000 && uptime_boottime != 0) {
            procfile_close(read_proc_uptime_ff);
            netdata_log_info("Using now_boottime_usec() for uptime (dt is %lld ms)", delta);
            use_boottime = 1;
        }
        else if(uptime_proc != 0) {
            netdata_log_info("Using /proc/uptime for uptime (dt is %lld ms)", delta);
            use_boottime = 0;
        }
        else {
            netdata_log_error("Cannot find any way to read uptime on this system.");
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
