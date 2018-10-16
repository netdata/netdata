// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_H
#define NETDATA_OS_H

#include "libnetdata.h"

// =====================================================================================================================
// Linux

#if (TARGET_OS == OS_LINUX)


// =====================================================================================================================
// FreeBSD

#elif (TARGET_OS == OS_FREEBSD)

#include <sys/sysctl.h>

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))
extern int getsysctl_by_name(const char *name, void *ptr, size_t len);

#define GETSYSCTL_MIB(name, mib) getsysctl_mib(name, mib, sizeof(mib)/sizeof(int))

extern int getsysctl_mib(const char *name, int *mib, size_t len);

#define GETSYSCTL_SIMPLE(name, mib, var) getsysctl_simple(name, mib, sizeof(mib)/sizeof(int), &(var), sizeof(var))
#define GETSYSCTL_WSIZE(name, mib, var, size) getsysctl_simple(name, mib, sizeof(mib)/sizeof(int), var, size)

extern int getsysctl_simple(const char *name, int *mib, size_t miblen, void *ptr, size_t len);

#define GETSYSCTL_SIZE(name, mib, size) getsysctl(name, mib, sizeof(mib)/sizeof(int), NULL, &(size))
#define GETSYSCTL(name, mib, var, size) getsysctl(name, mib, sizeof(mib)/sizeof(int), &(var), &(size))

extern int getsysctl(const char *name, int *mib, size_t miblen, void *ptr, size_t *len);


// =====================================================================================================================
// MacOS

#elif (TARGET_OS == OS_MACOS)

#include <sys/sysctl.h>

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))
extern int getsysctl_by_name(const char *name, void *ptr, size_t len);


// =====================================================================================================================
// unknown O/S

#else
#error unsupported operating system
#endif


// =====================================================================================================================
// common for all O/S

extern int processors;
extern long get_system_cpus(void);

extern pid_t pid_max;
extern pid_t get_system_pid_max(void);

extern unsigned int system_hz;
extern void get_system_HZ(void);

#endif //NETDATA_OS_H
