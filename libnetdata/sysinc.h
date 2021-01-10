// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SYSINC_H
#define SYSINC_H 1

#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

#define OS_LINUX   1
#define OS_FREEBSD 2
#define OS_MACOS   3


// ----------------------------------------------------------------------------
// system include files for all netdata C programs

/* select the memory allocator, based on autoconf findings */
#if defined(ENABLE_JEMALLOC)

#if defined(HAVE_JEMALLOC_JEMALLOC_H)
#include <jemalloc/jemalloc.h>
#else // !defined(HAVE_JEMALLOC_JEMALLOC_H)
#include <malloc.h>
#endif // !defined(HAVE_JEMALLOC_JEMALLOC_H)

#elif defined(ENABLE_TCMALLOC)

#include <google/tcmalloc.h>

#else /* !defined(ENABLE_JEMALLOC) && !defined(ENABLE_TCMALLOC) */

#if !(defined(__FreeBSD__) || defined(__APPLE__))
#include <malloc.h>
#endif /* __FreeBSD__ || __APPLE__ */

#endif /* !defined(ENABLE_JEMALLOC) && !defined(ENABLE_TCMALLOC) */

// ----------------------------------------------------------------------------

#if defined(__FreeBSD__)
#include <pthread_np.h>
#define NETDATA_OS_TYPE "freebsd"
#elif defined(__APPLE__)
#define NETDATA_OS_TYPE "macos"
#else
#define NETDATA_OS_TYPE "linux"
#endif /* __FreeBSD__, __APPLE__*/

#include <pthread.h>
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <stdarg.h>
#include <stddef.h>
#include <ctype.h>
#include <string.h>
#include <strings.h>
#include <arpa/inet.h>
#include <netinet/tcp.h>
#include <sys/ioctl.h>
#include <libgen.h>
#include <dirent.h>
#include <fcntl.h>
#include <getopt.h>
#include <grp.h>
#include <pwd.h>
#include <limits.h>
#include <locale.h>
#include <net/if.h>
#include <poll.h>
#include <signal.h>
#include <syslog.h>
#include <sys/mman.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/un.h>
#include <time.h>
#include <unistd.h>
#include <uuid/uuid.h>
#include <spawn.h>
#include <uv.h>
#include <assert.h>

#ifdef HAVE_NETINET_IN_H
#include <netinet/in.h>
#endif

#ifdef HAVE_RESOLV_H
#include <resolv.h>
#endif

#ifdef HAVE_NETDB_H
#include <netdb.h>
#endif

#ifdef HAVE_SYS_PRCTL_H
#include <sys/prctl.h>
#endif

#ifdef HAVE_SYS_STAT_H
#include <sys/stat.h>
#endif

#ifdef HAVE_SYS_VFS_H
#include <sys/vfs.h>
#endif

#ifdef HAVE_SYS_STATFS_H
#include <sys/statfs.h>
#endif

#ifdef HAVE_SYS_MOUNT_H
#include <sys/mount.h>
#endif

#ifdef HAVE_SYS_STATVFS_H
#include <sys/statvfs.h>
#endif

// #1408
#ifdef MAJOR_IN_MKDEV
#include <sys/mkdev.h>
#endif
#ifdef MAJOR_IN_SYSMACROS
#include <sys/sysmacros.h>
#endif

#ifdef STORAGE_WITH_MATH
#include <math.h>
#include <float.h>
#endif

#if defined(HAVE_INTTYPES_H)
#include <inttypes.h>
#elif defined(HAVE_STDINT_H)
#include <stdint.h>
#endif

#ifdef NETDATA_WITH_ZLIB
#include <zlib.h>
#endif

#ifdef HAVE_CAPABILITY
#include <sys/capability.h>
#endif

#endif // SYSINC_H
