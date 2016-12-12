#ifndef NETDATA_CLOCKS_H
#define NETDATA_CLOCKS_H 1

#ifndef HAVE_STRUCT_TIMESPEC
struct timespec {
    time_t tv_sec;  /* seconds */
    long   tv_nsec; /* nanoseconds */
};
#endif

#ifndef HAVE_CLOCKID_T
typedef int clockid_t;
#endif

#ifndef HAVE_CLOCK_GETTIME
int clock_gettime(clockid_t clk_id, struct timespec *ts);
#endif

/* Linux value is as good as any other */
#ifndef CLOCK_REALTIME
#define CLOCK_REALTIME  0
#endif

#ifndef CLOCK_MONOTONIC
/* fallback to CLOCK_REALTIME if not available */
#define CLOCK_MONOTONIC CLOCK_REALTIME
#endif

#ifndef CLOCK_BOOTTIME
/* fallback to CLOCK_MONOTONIC if not available */
#define CLOCK_BOOTTIME  CLOCK_MONOTONIC
#endif

typedef unsigned long long usec_t;

#define NSEC_PER_SEC    1000000000ULL
#define NSEC_PER_MSEC   1000000ULL
#define NSEC_PER_USEC   1000ULL
#define USEC_PER_SEC    1000000ULL

#ifndef HAVE_CLOCK_GETTIME
/* Fallback function for POSIX.1-2001 clock_gettime() function.
 *
 * We use a realtime clock from gettimeofday(), this will
 * make systems without clock_gettime() support sensitive
 * to time jumps or hibernation/suspend side effects.
 */
extern int clock_gettime(clockid_t clk_id, struct timespec *ts);
#endif

/* Fills struct timeval with time since EPOCH from real-time clock (i.e. wall-clock).
 * - Hibernation/suspend time is included
 * - adjtime()/NTP adjustments affect this clock
 * Return 0 on succes, -1 else with errno set appropriately.
 */
extern int now_realtime_timeval(struct timeval *tv);

/* Returns time since EPOCH from real-time clock (i.e. wall-clock).
 * - Hibernation/suspend time is included
 * - adjtime()/NTP adjustments affect this clock
 */
extern time_t now_realtime_sec(void);
extern usec_t now_realtime_usec(void);

/* Returns time from monotonic clock if available, real-time clock else.
 * If monotonic clock is available:
 * - hibernation/suspend time is not included
 * - adjtime()/NTP adjusments affect this clock
 * If monotonic clock is not available, this fallbacks to now_realtime().
 */
extern time_t now_monotonic_sec(void);
extern usec_t now_monotonic_usec(void);

/* Returns time from boottime clock if available,
 * monotonic clock else if available, real-time clock else.
 * If boottime clock is available:
 * - hibernation/suspend time is included
 * - adjtime()/NTP adjusments affect this clock
 * If boottime clock is not available, this fallbacks to now_monotonic().
 * If monotonic clock is not available, this fallbacks to now_realtime().
 */
extern time_t now_boottime_sec(void);
extern usec_t now_boottime_usec(void);

extern usec_t timeval_usec(struct timeval *ts);
extern usec_t dt_usec(struct timeval *now, struct timeval *old);

#endif /* NETDATA_CLOCKS_H */
