#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

/**
 * @file common.h
 * @brief This file is holding common includes, defines and functions.
 *
 * Every netdata C program should include this file.
 */

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
#include <sys/stat.h>
#include <sys/statvfs.h>
#include <sys/syscall.h>
#include <sys/time.h>
#include <sys/types.h>
#include <sys/wait.h>
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

/// returns_nonnull function attribute
#ifdef HAVE_FUNC_ATTRIBUTE_RETURNS_NONNULL
#define NEVERNULL __attribute__((returns_nonnull))
#else
#define NEVERNULL
#endif

/// malloc function attribute
#ifdef HAVE_FUNC_ATTRIBUTE_MALLOC
#define MALLOCLIKE __attribute__((malloc))
#else
#define MALLOCLIKE
#endif

/// format function attribute
#ifdef HAVE_FUNC_ATTRIBUTE_FORMAT
#define PRINTFLIKE(f, a) __attribute__ ((format(__printf__, f, a)))
#else
#define PRINTFLIKE(f, a)
#endif

/// noreturn function attribute
#ifdef HAVE_FUNC_ATTRIBUTE_NORETURN
#define NORETURN __attribute__ ((noreturn))
#else
#define NORETURN
#endif

/// warn_unused_reslult function attribute
#ifdef HAVE_FUNC_ATTRIBUTE_WARN_UNUSED_RESULT
#define WARNUNUSED __attribute__ ((warn_unused_result))
#else
#define WARNUNUSED
#endif

/// Absolute value
#ifdef abs
#undef abs
#endif
#define abs(x) ((x < 0)? -x : x)

/// Global User ID length
#define GUID_LEN 36

// ----------------------------------------------------------------------------
// netdata include files

#include "simple_pattern.h"
#include "avl.h"
#include "clocks.h"
#include "log.h"
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

#include "plugin_tc.h"
#include "plugins_d.h"
#include "socket.h"
#include "eval.h"
#include "health.h"
#include "rrd.h"
#include "rrd2json.h"
#include "web_client.h"
#include "web_server.h"
#include "registry.h"
#include "daemon.h"
#include "main.h"
#include "unit_test.h"
#include "ipc.h"
#include "backends.h"
#include "inlined.h"
#include "adaptive_resortable_list.h"

extern char *netdata_configured_config_dir;
extern char *netdata_configured_log_dir;
extern char *netdata_configured_plugins_dir;
extern char *netdata_configured_web_dir;
extern char *netdata_configured_cache_dir;
extern char *netdata_configured_varlib_dir;
extern char *netdata_configured_home_dir;
extern char *netdata_configured_host_prefix;

/**
 * Convert `s` to a valid chart id.
 *
 * Invalid characters are replaced.
 *
 * @param s string to convert
 */
extern void netdata_fix_chart_id(char *s);
/**
 * Convert `s` to a valid chart name.
 *
 * Invalid characters are replaced.
 *
 * @param s string to convert
 */
extern void netdata_fix_chart_name(char *s);

/** 
 * Reverse character order of a string.
 *
 * @param begin first character of string
 * @param end last character of string
 */
extern void strreverse(char* begin, char* end);
/**
 * `strsep() badjusting delimiters`
 *
 * `mystrsep` works like `strsep()` but it automatically skips adjusting delimiters
 * (so if the delimiter is a space, it will will skip all spaces).
 *
 * @see man 3 strsep
 *
 * @author Costa Tsaousis
 *
 * @param ptr
 * @param s
 * @return test
 */
extern char *mystrsep(char **ptr, char *s);
/**
 * Return `s` with leading and trainling whitespace omitted if not starting with '#'.
 *
 * Leading or trailing whitespace is not freed.
 * The first trailing whitespace gets replaced by '\0'.
 * Additional trailing whitespace is not freed.
 *
 * Lines starting with '#' after optional whitespace return NULL. They are treeted as comments.
 *
 * @param s string
 * @return the trimmed string or NULL
 */
extern char *trim(char *s);

/**
 * Write formatted output to `dst`.
 *
 * Write output accoruding to a format as described at manpage of `printf`.
 * `dst` is always propper terminated.
 * 
 * @see man 3 printf
 *
 * @param dst string to write to.
 * @param n write at most n-1 characters to `dst`
 * @param *fmt format string
 * @param args arguments to parse into format string
 * @return number of written characters without terminating '\0'
 */
extern int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
/**
 * Write formatted output to `dst`.
 *
 * Write output accoruding to a format as described at manpage of `printf`.
 * `dst` is always propper terminated.
 * 
 * @see man 3 printf
 *
 * @param dst string to write to.
 * @param n write at most n-1 characters to `dst`
 * @param *fmt format string
 * @return number of written characters without terminating '\0'
 */
extern int  snprintfz(char *dst, size_t n, const char *fmt, ...) PRINTFLIKE(3, 4);

// ----------------------------------------------------------------------------
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
/**
 * `strdup()` with failure handling.
 *
 * Exit netdata on failure.
 *
 * If NETDATA_LOG_ALLOCATIONS is set, this is logged.
 *
 * @see man 3 strdup
 *
 * @param s String to copy.
 * @return new string
 */
extern char *strdupz(const char *s) MALLOCLIKE NEVERNULL;
/**
 * `calloc()` with failure handling.
 *
 * Exit netdata on failure.
 *
 * If NETDATA_LOG_ALLOCATIONS is set, this is logged.
 *
 * @see man 3 malloc
 *
 * @param nmemb number of objects that are `size` bytes to allocate memory for
 * @param size number of bytes of one object
 * @return Pointer to allocated memory
 */
extern void *callocz(size_t nmemb, size_t size) MALLOCLIKE NEVERNULL;
/**
 * `malloc()` with failure handling.
 *
 * Exit netdata on failure.
 *
 * If NETDATA_LOG_ALLOCATIONS is set, this is logged.
 *
 * @see man 3 malloc
 *
 * @param size Number of bytes to allocate.
 * @return Pointer to allocated memory
 */
extern void *mallocz(size_t size) MALLOCLIKE NEVERNULL;
/**
 * `realloc()` with failure handling.
 *
 * Exit netdata on failure.
 *
 * If NETDATA_LOG_ALLOCATIONS is set, this is logged.
 *
 * @see man 3 malloc
 *
 * @param ptr Memory to reallocate
 * @param size Number of bytes to reallocate.
 * @return Pointer to allocated memory.
 */
extern void *reallocz(void *ptr, size_t size) MALLOCLIKE NEVERNULL;
/**
 * Calls `free()`.
 *
 * If NETDATA_LOG_ALLOCATIONS is set, this is logged.
 *
 * @see man 3 malloc
 *
 * @param ptr Pointer to allocated memory.
 */
extern void freez(void *ptr);
#endif

/** 
 * It is like strncpy(), but it escapes the string while copying it.
 *
 * When a string is to be added to a JSON object, we need to escape it 
 * (i.e. escape double quotes and backslashes).
 *
 * @author Costa Tsaousis
 *
 * @param dst Destination.
 * @param src String to copy.
 * @param size Maximum number of character to write to destination.
 */
extern void json_escape_string(char *dst, const char *src, size_t size);

/**
 * Extended `nmap()``
 *
 * 1. it creates the file if does not exist (to the correct size).
 * 2. it truncates the file if it already exists and is bigger than expected.
 * 3. sets various memory management heuristics we know netdata memory databases use (like MADV_SEQUENTIAL)
 * 4. prevents this memory from being copied when netdata forks to execute its plugins.
 * 5. enables KSM for it.
 * 6. in memory mode ram, it loads the file into memory (in all other modes, loading it is on-demand by the kernel).
 *
 * @author Costa Tsaousis
 *
 * @param filename to create
 * @param size of filename
 * @param flags to pass to `nmap()`
 * @param ksm boolean
 */
extern void *mymmap(const char *filename, size_t size, int flags, int ksm);
/**
 * Write at most `size` bytes of `mem` into `filename`.
 *
 * Save `mem` at file `filename`.
 * For this a file \<filename\>.\<pid\>.tmp is temporary created and used while writing.
 * \<pid\> is gained with `getpid()`
 * After writing the data the file is renamed to `filename`
 *
 * @param filename to write `mem` to
 * @param mem Data to write.
 * @param size bytes to write.
 * @return 0 on success, -1 on error.
 */
extern int savememory(const char *filename, void *mem, size_t size);

/**
 * Check if `fd` is a valid open file descriptor.
 *
 * @param fd file descriptor
 * @return boolean
 */
extern int fd_is_valid(int fd);

extern int enable_ksm;

extern char *global_host_prefix; ///< A configurable host prefix
extern int enable_ksm; ///< boolean to enable Kernel Same-Page Merging

/**
 * Get the calling thread's unique ID.
 *
 * Use this for portability.
 *
 * @see man 3 pthread_threadid_np
 *
 * @return calling thread's unique ID
 */
extern pid_t gettid(void);

/**
 * Sleep for `usec` microseconds.
 *
 * @param usec Microseconds to sleep.
 * @return 0 on success, -1 on error
 */
extern int sleep_usec(usec_t usec);

/**
 * Get a line from `fp`.
 *
 * Get one line from `fp`, trim trailing white space and store it into `buf`.
 *
 * @see man 3 fgets
 *
 * @param buf Buffer to store line in.
 * @param buf_size Size of buffer in bytes.
 * @param fp Stream to read from.
 * @param len Length of written string.
 * @return `buf` on success, NULL on failure.
 */
extern char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

extern int processors; ///< Number of logical processors
/**
 * Get number of logical processors.
 *
 * Get the number of processors. If the lookup fails try to guess.
 * This sets `processors`
 *
 * @return Number of logical cpu's.
 */
extern long get_system_cpus(void);

extern pid_t pid_max; ///< maximum supported prozess id's
/**
 * Get maximum number of pids.
 *
 * Get the maximum number of processes the system can maintain.
 * If the lookup fails try to guess.
 * This sets `pid_max`.
 *
 * @return Maximum number of pids.
 */
extern pid_t get_system_pid_max(void);

/** Number of ticks per second */
extern unsigned int hz;
/** Set `hz`. */
extern void get_system_HZ(void);

extern volatile sig_atomic_t netdata_exit;
extern const char *os_type;

extern const char *program_version;

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#endif /* NETDATA_COMMON_H */
