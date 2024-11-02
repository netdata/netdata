// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifdef OS_WINDOWS
void tinysleep(void) {
    Sleep(1);
}
#else
void tinysleep(void) {
    static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };
    nanosleep(&ns, NULL);
}
#endif

#ifdef OS_WINDOWS
void microsleep(usec_t ut) {
    size_t ms = ut / USEC_PER_MS + ((ut == 0 || (ut % USEC_PER_MS)) ? 1 : 0);
    Sleep(ms);
}
#else
void microsleep(usec_t ut) {
    time_t secs = (time_t)(ut / USEC_PER_SEC);
    nsec_t nsec = (ut % USEC_PER_SEC) * NSEC_PER_USEC;

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
