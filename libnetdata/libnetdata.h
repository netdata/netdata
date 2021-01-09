// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_H
#define NETDATA_LIB_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "sysinc.h"

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

#define MIN(a,b) (((a)<(b))?(a):(b))
#define MAX(a,b) (((a)>(b))?(a):(b))

#define GUID_LEN 36

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
extern __thread size_t log_thread_memory_allocations;
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
#else // NETDATA_LOG_ALLOCATIONS
extern char *strdupz(const char *s) MALLOCLIKE NEVERNULL;
extern void *callocz(size_t nmemb, size_t size) MALLOCLIKE NEVERNULL;
extern void *mallocz(size_t size) MALLOCLIKE NEVERNULL;
extern void *reallocz(void *ptr, size_t size) MALLOCLIKE NEVERNULL;
extern void freez(void *ptr);
#endif // NETDATA_LOG_ALLOCATIONS

extern void json_escape_string(char *dst, const char *src, size_t size);
extern void json_fix_string(char *s);

extern void *mymmap(const char *filename, size_t size, int flags, int ksm);
extern int memory_file_save(const char *filename, void *mem, size_t size);

extern int fd_is_valid(int fd);

extern struct rlimit rlimit_nofile;

extern int enable_ksm;

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
extern char *read_by_filename(char *filename, long *file_size);

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#define BITS_IN_A_KILOBIT 1000

/* misc. */
#define UNUSED(x) (void)(x)
#define error_report(x, args...) do { errno = 0; error(x, ##args); } while(0)

extern void netdata_cleanup_and_exit(int ret) NORETURN;
extern void send_statistics(const char *action, const char *action_result, const char *action_data);
extern char *netdata_configured_host_prefix;
#include "os.h"
#include "storage_number/storage_number.h"
#include "threads/threads.h"
#include "buffer/buffer.h"
#include "locks/locks.h"
#include "circular_buffer/circular_buffer.h"
#include "avl/avl.h"
#include "inlined.h"
#include "clocks/clocks.h"
#include "popen/popen.h"
#include "simple_pattern/simple_pattern.h"
#ifdef ENABLE_HTTPS
# include "socket/security.h"
#endif
#include "socket/socket.h"
#include "config/appconfig.h"
#include "log/log.h"
#include "procfile/procfile.h"
#include "dictionary/dictionary.h"
#ifdef HAVE_LIBBPF
#include "ebpf/ebpf.h"
#endif
#include "eval/eval.h"
#include "statistical/statistical.h"
#include "adaptive_resortable_list/adaptive_resortable_list.h"
#include "url/url.h"
#include "json/json.h"
#include "health/health.h"
#include "string/utf8.h"

// BEWARE: Outside of the C code this also exists in alarm-notify.sh
#define DEFAULT_CLOUD_BASE_URL "https://app.netdata.cloud"

# ifdef __cplusplus
}
# endif

#endif // NETDATA_LIB_H
