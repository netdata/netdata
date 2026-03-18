// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_PLATFORM_FWD_H
#define LIBNETDATA_PLATFORM_FWD_H

#if defined(__FreeBSD__)
#include <pthread_np.h>
#define NETDATA_OS_TYPE "freebsd"
#elif defined(__APPLE__)
#define NETDATA_OS_TYPE "macos"
#elif defined(OS_WINDOWS)
#define NETDATA_OS_TYPE "windows"
#else
#define NETDATA_OS_TYPE "linux"
#endif

#endif // LIBNETDATA_PLATFORM_FWD_H
