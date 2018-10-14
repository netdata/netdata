// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_H
#define NETDATA_LIB_H 1

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


// ----------------------------------------------------------------------------
// netdata common definitions

#if (SIZEOF_VOID_P == 8)
#define ENVIRONMENT64
#elif (SIZEOF_VOID_P == 4)
#define ENVIRONMENT32
#else
#error "Cannot detect if this is a 32 or 64 bit CPU"
#endif

#ifdef __GNUC__
#define GCC_VERSION (__GNUC__ * 10000 + __GNUC_MINOR__ * 100 + __GNUC_PATCHLEVEL__)
#endif // __GNUC__

#ifdef HAVE_FUNC_ATTRIBUTE_RETURNS_NONNULL
#define NEVERNULL __attribute__((returns_nonnull))
#else
#define NEVERNULL
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_NOINLINE
#define NOINLINE __attribute__((noinline))
#else
#define NOINLINE
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_MALLOC
#define MALLOCLIKE __attribute__((malloc))
#else
#define MALLOCLIKE
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_FORMAT
#define PRINTFLIKE(f, a) __attribute__ ((format(__printf__, f, a)))
#else
#define PRINTFLIKE(f, a)
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_NORETURN
#define NORETURN __attribute__ ((noreturn))
#else
#define NORETURN
#endif

#ifdef HAVE_FUNC_ATTRIBUTE_WARN_UNUSED_RESULT
#define WARNUNUSED __attribute__ ((warn_unused_result))
#else
#define WARNUNUSED
#endif

#ifdef abs
#undef abs
#endif
#define abs(x) (((x) < 0)? (-(x)) : (x))

#define GUID_LEN 36

#include "os.h"
#include "storage_number.h"
#include "web_buffer.h"
#include "locks.h"
#include "avl.h"
#include "inlined.h"
#include "clocks.h"
#include "threads.h"
#include "popen.h"
#include "simple_pattern.h"
#include "socket.h"
#include "appconfig.h"
#include "log.h"
#include "procfile.h"
#include "dictionary.h"
#include "eval.h"
#include "statistical.h"
#include "adaptive_resortable_list.h"
#include "url.h"

extern void netdata_fix_chart_id(char *s);
extern void netdata_fix_chart_name(char *s);

extern void strreverse(char* begin, char* end);
extern char *mystrsep(char **ptr, char *s);
extern char *trim(char *s); // remove leading and trailing spaces; may return NULL
extern char *trim_all(char *buffer); // like trim(), but also remove duplicate spaces inside the string; may return NULL

extern int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
extern int  snprintfz(char *dst, size_t n, const char *fmt, ...) PRINTFLIKE(3, 4);

// memory allocation functions that handle failures
#ifdef NETDATA_LOG_ALLOCATIONS
#define strdupz(s) strdupz_int(__FILE__, __FUNCTION__, __LINE__, s)
#define callocz(nmemb, size) callocz_int(__FILE__, __FUNCTION__, __LINE__, nmemb, size)
#define mallocz(size) mallocz_int(__FILE__, __FUNCTION__, __LINE__, size)
#define reallocz(ptr, size) reallocz_int(__FILE__, __FUNCTION__, __LINE__, ptr, size)
#define freez(ptr) freez_int(__FILE__, __FUNCTION__, __LINE__, ptr)

extern char *strdupz_int(const char *file, const char *function, const unsigned long line, const char *s);
extern void *callocz_int(const char *file, const char *function, const unsigned long line, size_t nmemb, size_t size);
extern void *mallocz_int(const char *file, const char *function, const unsigned long line, size_t size);
extern void *reallocz_int(const char *file, const char *function, const unsigned long line, void *ptr, size_t size);
extern void freez_int(const char *file, const char *function, const unsigned long line, void *ptr);
#else
extern char *strdupz(const char *s) MALLOCLIKE NEVERNULL;
extern void *callocz(size_t nmemb, size_t size) MALLOCLIKE NEVERNULL;
extern void *mallocz(size_t size) MALLOCLIKE NEVERNULL;
extern void *reallocz(void *ptr, size_t size) MALLOCLIKE NEVERNULL;
extern void freez(void *ptr);
#endif

extern void json_escape_string(char *dst, const char *src, size_t size);
extern void json_fix_string(char *s);

extern void *mymmap(const char *filename, size_t size, int flags, int ksm);
extern int memory_file_save(const char *filename, void *mem, size_t size);

extern int fd_is_valid(int fd);

extern struct rlimit rlimit_nofile;

extern int enable_ksm;

extern int sleep_usec(usec_t usec);

extern char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

extern int verify_netdata_host_prefix();

extern int recursively_delete_dir(const char *path, const char *reason);

extern volatile sig_atomic_t netdata_exit;
extern const char *os_type;

extern const char *program_version;

extern char *strdupz_path_subpath(const char *path, const char *subpath);
extern int path_is_dir(const char *path, const char *subpath);
extern int path_is_file(const char *path, const char *subpath);
extern void recursive_config_double_dir_load(
        const char *user_path
        , const char *stock_path
        , const char *subpath
        , int (*callback)(const char *filename, void *data)
        , void *data
        , size_t depth
);

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#define BITS_IN_A_KILOBIT 1000


extern void netdata_cleanup_and_exit(int ret) NORETURN;
extern char *netdata_configured_host_prefix;

#endif // NETDATA_LIB_H
