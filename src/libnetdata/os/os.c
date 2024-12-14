// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

unsigned int system_hz = 100;
void os_get_system_HZ(void) {
    long ticks;

    if ((ticks = sysconf(_SC_CLK_TCK)) == -1) {
        netdata_log_error("Cannot get system clock ticks");
        ticks = 100;
    }

    system_hz = (unsigned int) ticks;
}

// =====================================================================================================================
// os_type

#if defined(OS_LINUX)
const char *os_type = "linux";
#endif

#if defined(OS_FREEBSD)
const char *os_type = "freebsd";
#endif

#if defined(OS_MACOS)
const char *os_type = "macos";
#endif

#if defined(OS_WINDOWS)
const char *os_type = "windows";
#endif

