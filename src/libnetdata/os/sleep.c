// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifdef OS_WINDOWS
ALWAYS_INLINE void tinysleep(void) {
    Sleep(0);
    // SwitchToThread();
}
#else
ALWAYS_INLINE void tinysleep(void) {
    static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };
    nanosleep(&ns, NULL);
}
#endif

#ifdef OS_WINDOWS
ALWAYS_INLINE void yield_the_processor(void) {
    Sleep(0);
}
#else
ALWAYS_INLINE void yield_the_processor(void) {
    sched_yield();
}
#endif

#ifdef OS_WINDOWS
ALWAYS_INLINE void microsleep(usec_t ut) {
    size_t ms = ut / USEC_PER_MS + ((ut == 0 || (ut % USEC_PER_MS)) ? 1 : 0);
    Sleep(ms);
}
#else
ALWAYS_INLINE void microsleep(usec_t ut) {
    time_t secs = (time_t)(ut / USEC_PER_SEC);
    nsec_t nsec = (ut % USEC_PER_SEC) * NSEC_PER_USEC + ((ut == 0) ? 1 : 0);

    struct timespec remaining = {
        .tv_sec = secs,
        .tv_nsec = nsec,
    };

    errno_clear();
    while (nanosleep(&remaining, &remaining) == -1 && errno == EINTR && (remaining.tv_sec || remaining.tv_nsec)) {
        // Loop continues if interrupted by a signal
    }
}
#endif
