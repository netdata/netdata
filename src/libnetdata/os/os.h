// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_H
#define NETDATA_OS_H

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/syscall.h>
#endif

#include "setresuid.h"
#include "setresgid.h"
#include "getgrouplist.h"
#include "adjtimex.h"
#include "gettid.h"
#include "waitid.h"
#include "get_pid_max.h"
#include "get_system_cpus.h"
#include "tinysleep.h"
#include "uuid_generate.h"
#include "setenv.h"
#include "os-freebsd-wrappers.h"
#include "os-macos-wrappers.h"

// =====================================================================================================================
// common defs for Apple/FreeBSD/Linux

extern const char *os_type;

#define os_get_system_cpus() os_get_system_cpus_cached(true, false)
#define os_get_system_cpus_uncached() os_get_system_cpus_cached(false, false)
long os_get_system_cpus_cached(bool cache, bool for_netdata);
unsigned long os_read_cpuset_cpus(const char *filename, long system_cpus);

extern unsigned int system_hz;
void os_get_system_HZ(void);

#endif //NETDATA_OS_H
