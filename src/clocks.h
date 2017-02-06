#ifndef NETDATA_CLOCKS_H
#define NETDATA_CLOCKS_H 1

/**
 * @file clocks.h
 * @brief Get system time.
 *
 * Three clocks are available.
 *
 * - REALTIME clock (i.e. wall-clock):
 *
 *   This clock is affected by discontinuous jumps in the system time
 *   (e.g., if the system administrator manually changes the clock), and by the incremental adjustments performed by adjtime(3) and NTP.
 *
 * - MONOTONIC clock
 *
 *   Clock that cannot be set and represents monotonic time since some unspecified starting point.
 *   This clock is not affected by discontinuous jumps in the system time
 *   (e.g., if the system administrator manually changes the clock), but is affected by the incremental adjustments performed by adjtime(3) and NTP.
 *   If not available on the system, this clock falls back to REALTIME clock.
 *
 * - BOOTTIME clock
 *
 *   Identical to  CLOCK_MONOTONIC, except it also includes any time that the system is suspended.
 *   This allows applications to get a suspend-aware monotonic clock without having to deal with the complications of CLOCK_REALTIME,
 *   which may have discontinuities if the time is changed using settimeofday(2).
 *   If not available on the system, this clock falls back to MONOTONIC clock.
 *
 * @see man 3 clock_gettime
 *
 * All now_*_timeval() functions fill the `struct timeval` with the time from the appropriate clock.
 * Those functions return 0 on success, -1 else with errno set appropriately.
 *
 * All now_*_sec() functions return the time in seconds from the approriate clock, or 0 on error.
 * All now_*_usec() functions return the time in microseconds from the approriate clock, or 0 on error.
 *
 * heartbeat_init() heartbeat_next() and heartbeat_dt_usec() provide a API to periodically do something.
 * Use it as follows:
 * ```{.c}
 * heartbeat_t hb;
 * heartbeat_init(&hb);
 * for(;;) {
 *    usec_t hb_dt = heartbeat_next(&hb, step); // Sleep aligned to step
 *    // Do something...
 *    usec_t duration = heartbeat_dt_usec(&hb) // time since heartbeat_next
 *    // Do something...
 * }
 * ```
 *
 * @author rlefevre
 */

#ifndef HAVE_STRUCT_TIMESPEC
/** Fallback struct timespec. */
struct timespec {
    time_t tv_sec;  ///< Seconds.
    long   tv_nsec; ///< Nanoseconds.
};
#endif

#ifndef HAVE_CLOCKID_T
typedef int clockid_t; ///< Used for clock ID type in the clock and timer functions.
#endif

typedef unsigned long long usec_t; ///< Microsecond.

typedef usec_t heartbeat_t; ///< Data structure for use with heartbeat_* functions.

#ifndef HAVE_CLOCK_GETTIME
int clock_gettime(clockid_t clk_id, struct timespec *ts);
#endif

/** Linux value is as good as any other */
#ifndef CLOCK_REALTIME
#define CLOCK_REALTIME  0
#endif

#ifndef CLOCK_MONOTONIC
/** Fallback to CLOCK_REALTIME if not available. */
#define CLOCK_MONOTONIC CLOCK_REALTIME
#endif

#ifndef CLOCK_BOOTTIME
/** Fallback to CLOCK_MONOTONIC if not available. */
#define CLOCK_BOOTTIME  CLOCK_MONOTONIC
#else
#ifdef HAVE_CLOCK_GETTIME
#define CLOCK_BOOTTIME_IS_AVAILABLE 1 ///< Required for /proc/uptime.
#endif
#endif

#define NSEC_PER_SEC    1000000000ULL ///< Number of nanoseconds per second.
#define NSEC_PER_MSEC   1000000ULL    ///< Number of nanoseconds per millisecond.
#define NSEC_PER_USEC   1000ULL       ///< Number of nanoseconds per microsecond.
#define USEC_PER_SEC    1000000ULL    ///< Number of microseconds per second.

#ifndef HAVE_CLOCK_GETTIME
/**
 * Fallback function for POSIX.1-2001 clock_gettime() function.
 *
 * We use a realtime clock from `gettimeofday()`, this will
 * make systems without clock_gettime() support sensitive
 * to time jumps or hibernation/suspend side effects.
 *
 * errno is set by `gettimeofday()`.
 * 
 * @see man 2 gettimeofday
 *
 * @param clk_id Not used.
 * @param ts struct timespec to store time
 * @return 0 on succes, -1 on error with errno set appropriately
 */
extern int clock_gettime(clockid_t clk_id, struct timespec *ts);
#endif


/**
 * Fills struct timeval with time since EPOCH from real-time clock. (i.e. wall-clock)
 *
 * - Hibernation/suspend time is included
 * - adjtime()/NTP adjustments affect this clock
 *
 * errno is set by `gettimeofday()`.
 * 
 * @see man 2 gettimeofday
 *
 * @param tv struct timeval gets updated.
 * @return 0 on success, -1 on error with errno set appropriately
 */
extern int now_realtime_timeval(struct timeval *tv);

/**
 * Returns time since EPOCH from real-time clock. (i.e. wall-clock)
 *
 * - Hibernation/suspend time is included
 * - adjtime()/NTP adjustments affect this clock
 *
 * @return time since EPOCH
 */
extern time_t now_realtime_sec(void);
/**
 * Returns time since EPOCH from real-time clock. (i.e. wall-clock)
 *
 * - Hibernation/suspend time is included
 * - adjtime()/NTP adjustments affect this clock
 *
 * @return time since EPOCH
 */
extern usec_t now_realtime_usec(void);

/**
 * Returns time from monotonic clock.
 *
 * If monotonic clock is available:
 * - hibernation/suspend time is not included
 * - adjtime()/NTP adjusments affect this clock
 * If monotonic clock is not available, this fallbacks to `now_realtime()`.
 *
 * @param tv struct timeval to update
 * @return time
 */
extern int now_monotonic_timeval(struct timeval *tv);
/**
 * Returns time from monotonic clock.
 *
 * If monotonic clock is available:
 * - hibernation/suspend time is not included
 * - adjtime()/NTP adjusments affect this clock
 * If monotonic clock is not available, this fallbacks to `now_realtime()`.
 *
 * @return time
 */
extern time_t now_monotonic_sec(void);
/**
 * Returns time from monotonic clock.
 *
 * If monotonic clock is available:
 * - hibernation/suspend time is not included
 * - adjtime()/NTP adjusments affect this clock
 * If monotonic clock is not available, this fallbacks to `now_realtime()`.
 *
 * @return time
 */
extern usec_t now_monotonic_usec(void);

/**
 * Returns time from boottime clock
 * 
 * If boottime clock is available:
 * - hibernation/suspend time is included
 * - adjtime()/NTP adjusments affect this clock
 * If boottime clock is not available, this fallbacks to now_monotonic().
 * If monotonic clock is not available, this fallbacks to `now_realtime()`.
 *
 * @param tv struct timeval to update.
 * @return time
 */
extern int now_boottime_timeval(struct timeval *tv);
/**
 * Returns time from boottime clock
 * 
 * If boottime clock is available:
 * - hibernation/suspend time is included
 * - adjtime()/NTP adjusments affect this clock
 * If boottime clock is not available, this fallbacks to now_monotonic().
 * If monotonic clock is not available, this fallbacks to `now_realtime()`.
 *
 * @return time
 */
extern time_t now_boottime_sec(void);
/**
 * Returns time from boottime clock
 * 
 * If boottime clock is available:
 * - hibernation/suspend time is included
 * - adjtime()/NTP adjusments affect this clock
 * If boottime clock is not available, this fallbacks to now_monotonic().
 * If monotonic clock is not available, this fallbacks to `now_realtime()`.
 *
 * @return time
 */
extern usec_t now_boottime_usec(void);

/**
 * Convert `struct timveal` to `usec_t`
 *
 * @param ts struct timeval
 * @return time in microseconds 
 */
extern usec_t timeval_usec(struct timeval *ts);
/**
 * Return microseconds between `now` and `old`.
 *
 * This returns
 * ~~~~~~~~~~~~~~{.c}
 * abs(now - old)
 * ~~~~~~~~~~~~~~
 *
 * @param now First struct timeval.
 * @param old Second struct timeval.
 * @return time in microseconds between `now` and `old`
 */
extern usec_t dt_usec(struct timeval *now, struct timeval *old);

/**
 * Initialize heartbeat_t `hb`.
 *
 * After that `hb` can be used with heartbeat_*() functions.
 *
 * @param hb heartbeat_t to initialize.
 */
extern void heartbeat_init(heartbeat_t *hb);

/**
 * Sleeps until next multiple of `tick` using monotonic clock.
 *
 * @param hb Previous heartbeat_t.
 * @param tick Multiple of monotonic clock to align heartbeat to.
 * @return elapsed time in microseconds since previous heartbeat
 */
extern usec_t heartbeat_next(heartbeat_t *hb, usec_t tick);

/**
 * Returns elapsed time in microseconds since last heartbeat.
 *
 * @param hb Heartbeat.
 * @return elapsed time in microseconds since last heartbeat
 */
extern usec_t heartbeat_dt_usec(heartbeat_t *hb);

#endif /* NETDATA_CLOCKS_H */
