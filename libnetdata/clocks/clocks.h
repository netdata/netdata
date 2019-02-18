// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLOCKS_H
#define NETDATA_CLOCKS_H 1

#include "../libnetdata.h"

#ifndef HAVE_STRUCT_TIMESPEC
struct timespec {
    time_t tv_sec;  /* seconds */
    long   tv_nsec; /* nanoseconds */
};
#endif

#ifndef HAVE_CLOCKID_T
typedef int clockid_t;
#endif

typedef unsigned long long nsec_t;
typedef unsigned long long msec_t;
typedef unsigned long long usec_t;
typedef long long susec_t;

typedef struct heartbeat {
    usec_t monotonic;
    usec_t realtime;
} heartbeat_t;

/* Linux value is as good as any other */
#ifndef CLOCK_REALTIME
#define CLOCK_REALTIME  0
#endif

#ifndef CLOCK_MONOTONIC
/* fallback to CLOCK_REALTIME if not available */
#define CLOCK_MONOTONIC CLOCK_REALTIME
#endif

#ifndef CLOCK_BOOTTIME

#ifdef CLOCK_UPTIME
/* CLOCK_BOOTTIME falls back to CLOCK_UPTIME on FreeBSD */
#define CLOCK_BOOTTIME CLOCK_UPTIME
#else // CLOCK_UPTIME
/* CLOCK_BOOTTIME falls back to CLOCK_MONOTONIC */
#define CLOCK_BOOTTIME  CLOCK_MONOTONIC
#endif // CLOCK_UPTIME

#else // CLOCK_BOOTTIME

#ifdef HAVE_CLOCK_GETTIME
#define CLOCK_BOOTTIME_IS_AVAILABLE 1 // required for /proc/uptime
#endif // HAVE_CLOCK_GETTIME

#endif // CLOCK_BOOTTIME

#define NSEC_PER_MSEC   1000000ULL

#define NSEC_PER_SEC    1000000000ULL
#define NSEC_PER_USEC   1000ULL

#define USEC_PER_SEC    1000000ULL
#define MSEC_PER_SEC    1000ULL

#define USEC_PER_MS     1000ULL

#ifndef HAVE_CLOCK_GETTIME
/* Fallback function for POSIX.1-2001 clock_gettime() function.
 *
 * We use a realtime clock from gettimeofday(), this will
 * make systems without clock_gettime() support sensitive
 * to time jumps or hibernation/suspend side effects.
 */
extern int clock_gettime(clockid_t clk_id, struct timespec *ts);
#endif

/*
 * Three clocks are available (cf. man 3 clock_gettime):
 *
 * REALTIME clock (i.e. wall-clock):
 *  This clock is affected by discontinuous jumps in the system time
 *  (e.g., if the system administrator manually changes the clock), and by the incremental adjustments performed by adjtime(3) and NTP.
 *
 * MONOTONIC clock
 *  Clock that cannot be set and represents monotonic time since some unspecified starting point.
 *  This clock is not affected by discontinuous jumps in the system time
 *  (e.g., if the system administrator manually changes the clock), but is affected by the incremental adjustments performed by adjtime(3) and NTP.
 *  If not available on the system, this clock falls back to REALTIME clock.
 *
 * BOOTTIME clock
 *  Identical to  CLOCK_MONOTONIC, except it also includes any time that the system is suspended.
 *  This allows applications to get a suspend-aware monotonic clock without having to deal with the complications of CLOCK_REALTIME,
 *  which may have discontinuities if the time is changed using settimeofday(2).
 *  If not available on the system, this clock falls back to MONOTONIC clock.
 *
 * All now_*_timeval() functions fill the `struct timeval` with the time from the appropriate clock.
 * Those functions return 0 on success, -1 else with errno set appropriately.
 *
 * All now_*_sec() functions return the time in seconds from the approriate clock, or 0 on error.
 * All now_*_usec() functions return the time in microseconds from the approriate clock, or 0 on error.
 */
extern int now_realtime_timeval(struct timeval *tv);
extern time_t now_realtime_sec(void);
extern usec_t now_realtime_usec(void);

extern int now_monotonic_timeval(struct timeval *tv);
extern time_t now_monotonic_sec(void);
extern usec_t now_monotonic_usec(void);

extern int now_boottime_timeval(struct timeval *tv);
extern time_t now_boottime_sec(void);
extern usec_t now_boottime_usec(void);


extern usec_t timeval_usec(struct timeval *tv);
extern msec_t timeval_msec(struct timeval *tv);

extern usec_t dt_usec(struct timeval *now, struct timeval *old);
extern susec_t dt_usec_signed(struct timeval *now, struct timeval *old);

extern void heartbeat_init(heartbeat_t *hb);

/* Sleeps until next multiple of tick using monotonic clock.
 * Returns elapsed time in microseconds since previous heartbeat
 */
extern usec_t heartbeat_next(heartbeat_t *hb, usec_t tick);

/* Returns elapsed time in microseconds since last heartbeat */
extern usec_t heartbeat_monotonic_dt_to_now_usec(heartbeat_t *hb);

extern int sleep_usec(usec_t usec);

/*
 * When running a binary with CLOCK_BOOTTIME defined on a system with a linux kernel older than Linux 2.6.39 the
 * clock_gettime(2) system call fails with EINVAL. In that case it must fall-back to CLOCK_MONOTONIC.
 */
void test_clock_boottime(void);

#endif /* NETDATA_CLOCKS_H */
