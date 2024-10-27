// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifdef OS_WINDOWS
void tinysleep(void) {
    // Improve the system timer resolution to 1 ms
    timeBeginPeriod(1);

    // Sleep for the desired duration
    Sleep(1);

    // Reset the system timer resolution
    timeEndPeriod(1);
}
#else
void tinysleep(void) {
    static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };
    nanosleep(&ns, NULL);
}
#endif
