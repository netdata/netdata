// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PLATFORM_FWD_H
#define LIBNETDATA_PLATFORM_FWD_H

#include "config.h"

#if !defined(OS_LINUX) && !defined(OS_FREEBSD) && !defined(OS_MACOS) && !defined(OS_WINDOWS)
#if defined(_WIN32)
#define OS_WINDOWS 1
#elif defined(__APPLE__)
#define OS_MACOS 1
#elif defined(__FreeBSD__)
#define OS_FREEBSD 1
#else
#define OS_LINUX 1
#endif
#endif

#if defined(OS_FREEBSD)
#include <pthread_np.h>
#define NETDATA_OS_TYPE "freebsd"
#elif defined(OS_MACOS)
#define NETDATA_OS_TYPE "macos"
#elif defined(OS_WINDOWS)
#define NETDATA_OS_TYPE "windows"
#else
#define NETDATA_OS_TYPE "linux"
#endif

#endif // LIBNETDATA_PLATFORM_FWD_H
