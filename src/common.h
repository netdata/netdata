// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

// ----------------------------------------------------------------------------
// system include files for all netdata C programs

/* select the memory allocator, based on autoconf findings */
#if defined(ENABLE_JEMALLOC)

#if defined(HAVE_JEMALLOC_JEMALLOC_H)
#include <jemalloc/jemalloc.h>
#else
#include <malloc.h>
#endif

#elif defined(ENABLE_TCMALLOC)

#include <google/tcmalloc.h>

#else /* !defined(ENABLE_JEMALLOC) && !defined(ENABLE_TCMALLOC) */

#if !(defined(__FreeBSD__) || defined(__APPLE__))
#include <malloc.h>
#endif /* __FreeBSD__ || __APPLE__ */

#endif

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

#ifdef HAVE_NETINET_IN_H
#include <netinet/in.h>
#endif

#ifdef HAVE_RESOLV_H
#include <resolv.h>
#endif

#include <dirent.h>
#include <fcntl.h>
#include <getopt.h>
#include <grp.h>
#include <pwd.h>
#include <locale.h>

#ifdef HAVE_NETDB_H
#include <netdb.h>
#endif

#include <net/if.h>

#include <poll.h>
#include <signal.h>
#include <syslog.h>
#include <sys/mman.h>

#ifdef HAVE_SYS_PRCTL_H
#include <sys/prctl.h>
#endif

#include <sys/resource.h>
#include <sys/socket.h>

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

#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/un.h>
#include <time.h>
#include <unistd.h>
#include <uuid/uuid.h>

// #1408
#ifdef MAJOR_IN_MKDEV
#include <sys/mkdev.h>
#endif
#ifdef MAJOR_IN_SYSMACROS
#include <sys/sysmacros.h>
#endif

/*
#include <mntent.h>
*/

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
// netdata chart priorities

// This is a work in progress - to scope is to collect here all chart priorities.
// These should be based on the CONTEXT of the charts + the chart id when needed
// - for each SECTION +1000 (or +X000 for big sections)
// - for each FAMILY  +100
// - for each CHART   +10

#define NETDATA_CHART_PRIO_SYSTEM_IP               501
#define NETDATA_CHART_PRIO_SYSTEM_IPV6             502

// Memory Section - 1xxx
#define NETDATA_CHART_PRIO_MEM_SYSTEM              1000
#define NETDATA_CHART_PRIO_MEM_SYSTEM_AVAILABLE    1010
#define NETDATA_CHART_PRIO_MEM_SYSTEM_COMMITTED    1020
#define NETDATA_CHART_PRIO_MEM_SYSTEM_PGFAULTS     1030
#define NETDATA_CHART_PRIO_MEM_KERNEL              1100
#define NETDATA_CHART_PRIO_MEM_SLAB                1200
#define NETDATA_CHART_PRIO_MEM_HUGEPAGES           1250
#define NETDATA_CHART_PRIO_MEM_KSM                 1300
#define NETDATA_CHART_PRIO_MEM_NUMA                1400
#define NETDATA_CHART_PRIO_MEM_HW                  1500


// IP

#define NETDATA_CHART_PRIO_IP                      4000
#define NETDATA_CHART_PRIO_IP_ERRORS               4100
#define NETDATA_CHART_PRIO_IP_TCP                  4200
#define NETDATA_CHART_PRIO_IP_TCP_MEM              4290
#define NETDATA_CHART_PRIO_IP_BCAST                4500
#define NETDATA_CHART_PRIO_IP_MCAST                4600
#define NETDATA_CHART_PRIO_IP_ECN                  4700


// IPv4

#define NETDATA_CHART_PRIO_IPV4                    5100
#define NETDATA_CHART_PRIO_IPV4_SOCKETS            5100
#define NETDATA_CHART_PRIO_IPV4_PACKETS            5130
#define NETDATA_CHART_PRIO_IPV4_ERRORS             5150
#define NETDATA_CHART_PRIO_IPV4_ICMP               5170
#define NETDATA_CHART_PRIO_IPV4_TCP                5200
#define NETDATA_CHART_PRIO_IPV4_TCP_MEM            5290
#define NETDATA_CHART_PRIO_IPV4_UDP                5300
#define NETDATA_CHART_PRIO_IPV4_UDP_MEM            5390
#define NETDATA_CHART_PRIO_IPV4_UDPLITE            5400
#define NETDATA_CHART_PRIO_IPV4_RAW                5450
#define NETDATA_CHART_PRIO_IPV4_FRAGMENTS          5460
#define NETDATA_CHART_PRIO_IPV4_FRAGMENTS_MEM      5470

// IPv6

#define NETDATA_CHART_PRIO_IPV6                    6200
#define NETDATA_CHART_PRIO_IPV6_PACKETS            6200
#define NETDATA_CHART_PRIO_IPV6_ERRORS             6300
#define NETDATA_CHART_PRIO_IPV6_FRAGMENTS          6400
#define NETDATA_CHART_PRIO_IPV6_TCP                6500
#define NETDATA_CHART_PRIO_IPV6_UDP                6600
#define NETDATA_CHART_PRIO_IPV6_UDP_ERRORS         6610
#define NETDATA_CHART_PRIO_IPV6_UDPLITE            6700
#define NETDATA_CHART_PRIO_IPV6_UDPLITE_ERRORS     6710
#define NETDATA_CHART_PRIO_IPV6_RAW                6800
#define NETDATA_CHART_PRIO_IPV6_BCAST              6840
#define NETDATA_CHART_PRIO_IPV6_MCAST              6850
#define NETDATA_CHART_PRIO_IPV6_ICMP               6900


// SCTP

#define NETDATA_CHART_PRIO_SCTP                    7000


// Netfilter

#define NETDATA_CHART_PRIO_NETFILTER               8700
#define NETDATA_CHART_PRIO_SYNPROXY                8750


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

// ----------------------------------------------------------------------------

typedef enum rrdcalc_status {
    RRDCALC_STATUS_REMOVED       = -2,
    RRDCALC_STATUS_UNDEFINED     = -1,
    RRDCALC_STATUS_UNINITIALIZED =  0,
    RRDCALC_STATUS_CLEAR         =  1,
    RRDCALC_STATUS_RAISED        =  2,
    RRDCALC_STATUS_WARNING       =  3,
    RRDCALC_STATUS_CRITICAL      =  4
} RRDCALC_STATUS;

// ----------------------------------------------------------------------------
// netdata include files

#include "clocks.h"
#include "log.h"
#include "threads.h"
#include "locks.h"
#include "simple_pattern.h"
#include "avl.h"
#include "global_statistics.h"
#include "storage_number.h"
#include "web_buffer.h"
#include "web_buffer_svg.h"
#include "url.h"
#include "popen.h"

#include "procfile.h"
#include "appconfig.h"
#include "dictionary.h"
#include "proc_self_mountinfo.h"
#include "plugin_checks.h"
#include "plugin_idlejitter.h"
#include "plugin_nfacct.h"

#if defined(__FreeBSD__)
#include <pthread_np.h>
#include "plugin_freebsd.h"
#define NETDATA_OS_TYPE "freebsd"
#elif defined(__APPLE__)
#include "plugin_macos.h"
#define NETDATA_OS_TYPE "macos"
#else
#include "plugin_proc.h"
#include "plugin_proc_diskspace.h"
#define NETDATA_OS_TYPE "linux"
#endif /* __FreeBSD__, __APPLE__*/

#include "eval.h"
#include "health.h"

#include "statistical.h"
#include "socket.h"
#include "rrd.h"
#include "plugin_tc.h"
#include "plugins_d.h"
#include "statsd.h"
#include "rrd2json.h"
#include "web_client.h"
#include "web_server.h"
#include "registry.h"
#include "signals.h"
#include "daemon.h"
#include "main.h"
#include "unit_test.h"
#include "ipc.h"
#include "backends.h"
#include "backend_prometheus.h"
#include "inlined.h"
#include "adaptive_resortable_list.h"
#include "rrdpush.h"
#include "web_api_v1.h"

extern char *netdata_configured_hostname;
extern char *netdata_configured_user_config_dir;
extern char *netdata_configured_stock_config_dir;
extern char *netdata_configured_log_dir;
extern char *netdata_configured_plugins_dir_base;
extern char *netdata_configured_plugins_dir;
extern char *netdata_configured_web_dir;
extern char *netdata_configured_cache_dir;
extern char *netdata_configured_varlib_dir;
extern char *netdata_configured_home_dir;
extern char *netdata_configured_host_prefix;
extern char *netdata_configured_timezone;

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

extern int processors;
extern long get_system_cpus(void);
extern int verify_netdata_host_prefix();

extern pid_t pid_max;
extern pid_t get_system_pid_max(void);

/* Number of ticks per second */
extern unsigned int system_hz;
extern void get_system_HZ(void);

extern int recursively_delete_dir(const char *path, const char *reason);

extern volatile sig_atomic_t netdata_exit;
extern const char *os_type;

extern const char *program_version;

extern char *strdupz_path_subpath(const char *path, const char *subpath);
extern int path_is_dir(const char *path, const char *subpath);
extern int path_is_file(const char *path, const char *subpath);
extern void recursive_config_double_dir_load(const char *user_path, const char *stock_path, const char *subpath
                                             , int (*callback)(const char *filename, void *data), void *data);

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#define BITS_IN_A_KILOBIT 1000

#endif /* NETDATA_COMMON_H */
