// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_MACOS_WRAPPERS_H
#define NETDATA_OS_MACOS_WRAPPERS_H

#include "../libnetdata.h"

#if defined(OS_MACOS)
#include <sys/sysctl.h>
#include "byteorder.h"

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))
int getsysctl_by_name(const char *name, void *ptr, size_t len);

#endif

#endif //NETDATA_OS_MACOS_WRAPPERS_H
