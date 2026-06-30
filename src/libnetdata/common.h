// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * This file should include everything from the operating system needed to compile Netdata,
 * without any Netdata specific includes.
 *
 * It should be the baseline of includes (operating system and common libraries related).
 */

#ifndef LIBNETDATA_COMMON_H
#define LIBNETDATA_COMMON_H

# ifdef __cplusplus
extern "C" {
# endif

#include "libnetdata-base.h"
#include "libnetdata-types.h"
#include "libnetdata-platform-fwd.h"

// --------------------------------------------------------------------------------------------------------------------
// NETDATA_OS_TYPE

// --------------------------------------------------------------------------------------------------------------------
// memory allocators

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

// --------------------------------------------------------------------------------------------------------------------

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
#include <libgen.h>
#include <dirent.h>
#include <fcntl.h>
#include <getopt.h>
#include <limits.h>
#include <locale.h>
#include <signal.h>
#include <sys/time.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>
#include <uv.h>
#include <assert.h>

#ifdef HAVE_ARPA_INET_H
#include <arpa/inet.h>
#endif

#ifdef HAVE_NETINET_TCP_H
#include <netinet/tcp.h>
#endif

#ifdef HAVE_SYS_IOCTL_H
#include <sys/ioctl.h>
#endif

#ifdef HAVE_GRP_H
#include <grp.h>
#else
typedef uint32_t gid_t;
#endif

#ifdef HAVE_PWD_H
#include <pwd.h>
#else
typedef uint32_t uid_t;
#endif

#ifdef HAVE_NET_IF_H
#include <net/if.h>
#endif

#ifdef HAVE_POLL_H
#include <poll.h>
#endif

#ifdef HAVE_SYSLOG_H
#include <syslog.h>
#else
/* priorities */
#define	LOG_EMERG	0	/* system is unusable */
#define	LOG_ALERT	1	/* action must be taken immediately */
#define	LOG_CRIT	2	/* critical conditions */
#define	LOG_ERR		3	/* error conditions */
#define	LOG_WARNING	4	/* warning conditions */
#define	LOG_NOTICE	5	/* normal but significant condition */
#define	LOG_INFO	6	/* informational */
#define	LOG_DEBUG	7	/* debug-level messages */

/* facility codes */
#define	LOG_KERN	(0<<3)	/* kernel messages */
#define	LOG_USER	(1<<3)	/* random user-level messages */
#define	LOG_MAIL	(2<<3)	/* mail system */
#define	LOG_DAEMON	(3<<3)	/* system daemons */
#define	LOG_AUTH	(4<<3)	/* security/authorization messages */
#define	LOG_SYSLOG	(5<<3)	/* messages generated internally by syslogd */
#define	LOG_LPR		(6<<3)	/* line printer subsystem */
#define	LOG_NEWS	(7<<3)	/* network news subsystem */
#define	LOG_UUCP	(8<<3)	/* UUCP subsystem */
#define	LOG_CRON	(9<<3)	/* clock daemon */
#define	LOG_AUTHPRIV	(10<<3)	/* security/authorization messages (private) */
#define	LOG_FTP		(11<<3)	/* ftp daemon */

/* other codes through 15 reserved for system use */
#define	LOG_LOCAL0	(16<<3)	/* reserved for local use */
#define	LOG_LOCAL1	(17<<3)	/* reserved for local use */
#define	LOG_LOCAL2	(18<<3)	/* reserved for local use */
#define	LOG_LOCAL3	(19<<3)	/* reserved for local use */
#define	LOG_LOCAL4	(20<<3)	/* reserved for local use */
#define	LOG_LOCAL5	(21<<3)	/* reserved for local use */
#define	LOG_LOCAL6	(22<<3)	/* reserved for local use */
#define	LOG_LOCAL7	(23<<3)	/* reserved for local use */
#endif

#ifdef HAVE_SYS_MMAN_H
#include <sys/mman.h>
#endif

#ifdef HAVE_SYS_RESOURCE_H
#include <sys/resource.h>
#endif

#ifdef HAVE_SYS_SOCKET_H
#include <sys/socket.h>
#endif

#ifdef HAVE_SYS_WAIT_H
#include <sys/wait.h>
#endif

#ifdef HAVE_SYS_UN_H
#include <sys/un.h>
#endif

#ifdef HAVE_SPAWN_H
#include <spawn.h>
#endif

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

#ifdef HAVE_LINUX_MAGIC_H
#include <linux/magic.h>
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

#include <math.h>
#include <float.h>

#include <zlib.h>

#ifdef HAVE_CAPABILITY
#include <sys/capability.h>
#endif

#define XXH_INLINE_ALL
#include "libnetdata/xxHash/xxhash.h"

// --------------------------------------------------------------------------------------------------------------------
// OpenSSL

#define OPENSSL_VERSION_095 0x00905100L
#define OPENSSL_VERSION_097 0x0907000L
#define OPENSSL_VERSION_110 0x10100000L
#define OPENSSL_VERSION_111 0x10101000L
#define OPENSSL_VERSION_300 0x30000000L

#include <openssl/ssl.h>
#include <openssl/rand.h>
#include <openssl/err.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097) && (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
#include <openssl/conf.h>
#endif

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
#include <openssl/core_names.h>
#include <openssl/decoder.h>
#endif

// --------------------------------------------------------------------------------------------------------------------

#ifndef O_CLOEXEC
#define O_CLOEXEC (0)
#endif

// --------------------------------------------------------------------------------------------------------------------
// fix for alpine linux

#if !defined(RUSAGE_THREAD) && defined(RUSAGE_CHILDREN)
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif

// --------------------------------------------------------------------------------------------------------------------
// NETDATA CLOUD

// BEWARE: this exists in alarm-notify.sh
#define DEFAULT_CLOUD_BASE_URL "https://app.netdata.cloud"

// --------------------------------------------------------------------------------------------------------------------
// DBENGINE

#define RRD_STORAGE_TIERS 5

// --------------------------------------------------------------------------------------------------------------------
// UUIDs

#define GUID_LEN 36

// --------------------------------------------------------------------------------------------------------------------
// Macro-only includes

#include "libnetdata/linked_lists/linked_lists.h"

// --------------------------------------------------------------------------------------------------------------------

#define FUNCTION_RUN_ONCE() {                                           \
    static bool __run_once = false;                                     \
    if (!__sync_bool_compare_and_swap(&__run_once, false, true)) {      \
        return;                                                         \
    }                                                                   \
}

#define FUNCTION_RUN_ONCE_RET(ret) {                                    \
    static bool __run_once = false;                                     \
    if (!__sync_bool_compare_and_swap(&__run_once, false, true)) {      \
        return (ret);                                                   \
    }                                                                   \
}

// --------------------------------------------------------------------------------------------------------------------

#ifndef HOST_NAME_MAX
#define HOST_NAME_MAX 256
#endif

// --------------------------------------------------------------------------------------------------------------------

#if defined(OS_WINDOWS)
#include <windows.h>
#include <wctype.h>
#include <wchar.h>
#include <tchar.h>
#include <guiddef.h>
#include <io.h>
#include <fcntl.h>
#include <process.h>
#include <tlhelp32.h>
#include <sys/cygwin.h>
#include <winevt.h>
#include <evntprov.h>
#include <wbemidl.h>
#include <sddl.h>
// #include <winternl.h> // conflicts on STRING,

// wincrypt.h (included via windows.h) defines macros that conflict with OpenSSL
#ifdef X509_NAME
#undef X509_NAME
#endif
#ifdef X509_EXTENSIONS
#undef X509_EXTENSIONS
#endif
#ifdef PKCS7_SIGNER_INFO
#undef PKCS7_SIGNER_INFO
#endif
#ifdef OCSP_REQUEST
#undef OCSP_REQUEST
#endif
#ifdef OCSP_RESPONSE
#undef OCSP_RESPONSE
#endif
#endif

// --------------------------------------------------------------------------------------------------------------------

/* Define a portable way to access st_mtim across Unix variants */
#if defined(_POSIX_C_SOURCE) && _POSIX_C_SOURCE >= 200809L
/* POSIX.1-2008 compliant systems have st_mtim */
#define STAT_GET_MTIME_SEC(st)  ((st).st_mtim.tv_sec)
#define STAT_GET_MTIME_NSEC(st) ((st).st_mtim.tv_nsec)
#elif defined(__APPLE__) || defined(__darwin__) || defined(__MACH__)
/* macOS has st_mtimespec */
#define STAT_GET_MTIME_SEC(st)  ((st).st_mtimespec.tv_sec)
#define STAT_GET_MTIME_NSEC(st) ((st).st_mtimespec.tv_nsec)
#elif defined(__FreeBSD__) || defined(__DragonFly__) || defined(__NetBSD__) || defined(__OpenBSD__)
/* BSD systems typically have st_mtim or provide a compatibility layer */
#define STAT_GET_MTIME_SEC(st)  ((st).st_mtim.tv_sec)
#define STAT_GET_MTIME_NSEC(st) ((st).st_mtim.tv_nsec)
#else
/* Fallback for systems with only second precision */
#define STAT_GET_MTIME_SEC(st)  ((st).st_mtime)
#define STAT_GET_MTIME_NSEC(st) (0)
#endif

# ifdef __cplusplus
}
# endif

#endif //LIBNETDATA_COMMON_H
