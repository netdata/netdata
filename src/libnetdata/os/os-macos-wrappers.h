// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_MACOS_WRAPPERS_H
#define NETDATA_OS_MACOS_WRAPPERS_H

#if defined(COMPILED_FOR_MACOS)
#include <pthread.h>
#include <errno.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <stddef.h>
#include <ctype.h>
#include <string.h>
#include <strings.h>
#include <sys/types.h>
#include <unistd.h>

#include <sys/sysctl.h>
#include "byteorder.h"

#define GETSYSCTL_BY_NAME(name, var) getsysctl_by_name(name, &(var), sizeof(var))
int getsysctl_by_name(const char *name, void *ptr, size_t len);

#endif

#endif //NETDATA_OS_MACOS_WRAPPERS_H
