#include "common.h"

#ifndef HAVE_CLOCK_GETTIME
inline int clock_gettime(clockid_t clk_id, struct timespec *ts) {
    struct timeval tv;
    if(unlikely(gettimeofday(&tv, NULL) == -1))
        return -1;
    ts->tv_sec = tv.tv_sec;
    ts->tv_nsec = tv.tv_usec * NSEC_PER_USEC;
    return 0;
}
#endif

inline time_t now_realtime_sec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_REALTIME, &ts) == -1))
        return 0;
    return ts.tv_sec;
}

inline int now_realtime_timeval(struct timeval *tv) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_REALTIME, &ts) == -1))
        return -1;
    tv->tv_sec = ts.tv_sec;
    tv->tv_usec = ts.tv_nsec / NSEC_PER_USEC;
    return 0;
}

inline usec_t now_realtime_usec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_REALTIME, &ts) == -1))
        return 0;
    return (usec_t)ts.tv_sec * USEC_PER_SEC + ts.tv_nsec / NSEC_PER_USEC;
}

inline time_t now_monotonic_sec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_MONOTONIC, &ts) == -1))
        return 0;
    return ts.tv_sec;
}

inline usec_t now_monotonic_usec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_MONOTONIC, &ts) == -1))
        return 0;
    return (usec_t)ts.tv_sec * USEC_PER_SEC + ts.tv_nsec / NSEC_PER_USEC;
}

inline time_t now_boottime_sec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_BOOTTIME, &ts) == -1))
        return 0;
    return ts.tv_sec;
}

inline usec_t now_boottime_usec(void) {
    struct timespec ts;
    if(unlikely(clock_gettime(CLOCK_BOOTTIME, &ts) == -1))
        return 0;
    return (usec_t)ts.tv_sec * USEC_PER_SEC + ts.tv_nsec / NSEC_PER_USEC;
}

inline usec_t timeval_usec(struct timeval *tv) {
    return (usec_t)tv->tv_sec * USEC_PER_SEC + tv->tv_usec;
}

inline usec_t dt_usec(struct timeval *now, struct timeval *old) {
    usec_t ts1 = timeval_usec(now);
    usec_t ts2 = timeval_usec(old);
    return (ts1 > ts2) ? (ts1 - ts2) : (ts2 - ts1);
}
