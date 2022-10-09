// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_H
#define NETDATA_OS_H

#include "libnetdata.h"

// =====================================================================================================================
// FreeBSD

#if __FreeBSD__

#include <sys/sysctl.h>

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))
int getsysctl_by_name(const char *name, void *ptr, size_t len);

#define GETSYSCTL_MIB(name, mib) getsysctl_mib(name, mib, sizeof(mib)/sizeof(int))

int getsysctl_mib(const char *name, int *mib, size_t len);

#define GETSYSCTL_SIMPLE(name, mib, var) getsysctl_simple(name, mib, sizeof(mib)/sizeof(int), &(var), sizeof(var))
#define GETSYSCTL_WSIZE(name, mib, var, size) getsysctl_simple(name, mib, sizeof(mib)/sizeof(int), var, size)

int getsysctl_simple(const char *name, int *mib, size_t miblen, void *ptr, size_t len);

#define GETSYSCTL_SIZE(name, mib, size) getsysctl(name, mib, sizeof(mib)/sizeof(int), NULL, &(size))
#define GETSYSCTL(name, mib, var, size) getsysctl(name, mib, sizeof(mib)/sizeof(int), &(var), &(size))

int getsysctl(const char *name, int *mib, size_t miblen, void *ptr, size_t *len);

#endif

// =====================================================================================================================
// MacOS

#if __APPLE__

#include <sys/sysctl.h>

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))
int getsysctl_by_name(const char *name, void *ptr, size_t len);

#endif

// =====================================================================================================================
// common defs for Apple/FreeBSD/Linux

extern const char *os_type;

extern int processors;
long get_system_cpus(void);

extern pid_t pid_max;
pid_t get_system_pid_max(void);

extern unsigned int system_hz;
void get_system_HZ(void);

#include <sys/timex.h>
#if defined(__FreeBSD__) || defined(__APPLE__)
#define ADJUST_TIMEX(x) ntp_adjtime(x)
#else
#define ADJUST_TIMEX(x) adjtimex(x)
#endif

#endif //NETDATA_OS_H
