// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_FREEBSD_WRAPPERS_H
#define NETDATA_OS_FREEBSD_WRAPPERS_H

#include "../libnetdata.h"

#if defined(OS_FREEBSD)
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

#endif //NETDATA_OS_FREEBSD_WRAPPERS_H
