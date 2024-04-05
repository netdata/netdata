// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_H
#define NETDATA_LIB_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "config.h"

#ifdef ENABLE_OPENSSL
#define ENABLE_HTTPS 1
#endif

#ifdef HAVE_LIBDATACHANNEL
#define ENABLE_WEBRTC 1
#endif

#define STRINGIFY(x) #x
#define TOSTRING(x) STRINGIFY(x)

#define JUDYHS_INDEX_SIZE_ESTIMATE(key_bytes) (((key_bytes) + sizeof(Word_t) - 1) / sizeof(Word_t) * 4)

#if defined(NETDATA_DEV_MODE) && !defined(NETDATA_INTERNAL_CHECKS)
#define NETDATA_INTERNAL_CHECKS 1
#endif

#ifndef SIZEOF_VOID_P
#error SIZEOF_VOID_P is not defined
#endif

#if SIZEOF_VOID_P == 4
#define ENV32BIT 1
#else
#define ENV64BIT 1
#endif

// NETDATA_TRACE_ALLOCATIONS does not work under musl libc, so don't enable it
//#if defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_TRACE_ALLOCATIONS)
//#define NETDATA_TRACE_ALLOCATIONS 1
//#endif

#define MALLOC_ALIGNMENT (sizeof(uintptr_t) * 2)
#define size_t_atomic_count(op, var, size) __atomic_## op ##_fetch(&(var), size, __ATOMIC_RELAXED)
#define size_t_atomic_bytes(op, var, size) __atomic_## op ##_fetch(&(var), ((size) % MALLOC_ALIGNMENT)?((size) + MALLOC_ALIGNMENT - ((size) % MALLOC_ALIGNMENT)):(size), __ATOMIC_RELAXED)

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
#include <stdbool.h>
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

// CentOS 7 has older version that doesn't define this
// same goes for MacOS
#ifndef UUID_STR_LEN
#define UUID_STR_LEN (37)
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

#if defined(HAVE_INTTYPES_H)
#include <inttypes.h>
#elif defined(HAVE_STDINT_H)
#include <stdint.h>
#endif

#include <zlib.h>

#ifdef HAVE_SYS_CAPABILITY_H
#include <sys/capability.h>
#endif


// ----------------------------------------------------------------------------
// netdata common definitions

#define _cleanup_(x) __attribute__((__cleanup__(x)))

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

void aral_judy_init(void);
size_t judy_aral_overhead(void);
size_t judy_aral_structures(void);

#define ABS(x) (((x) < 0)? (-(x)) : (x))
#define MIN(a,b) (((a)<(b))?(a):(b))
#define MAX(a,b) (((a)>(b))?(a):(b))
#define SWAP(a, b) do { \
    typeof(a) _tmp = b; \
    b = a;              \
    a = _tmp;           \
} while(0)

#define GUID_LEN 36

#include "linked-lists.h"
#include "storage-point.h"

void netdata_fix_chart_id(char *s);
void netdata_fix_chart_name(char *s);

int madvise_sequential(void *mem, size_t len);
int madvise_random(void *mem, size_t len);
int madvise_dontfork(void *mem, size_t len);
int madvise_willneed(void *mem, size_t len);
int madvise_dontneed(void *mem, size_t len);
int madvise_dontdump(void *mem, size_t len);
int madvise_mergeable(void *mem, size_t len);

int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
int  snprintfz(char *dst, size_t n, const char *fmt, ...) PRINTFLIKE(3, 4);

// memory allocation functions that handle failures
#ifdef NETDATA_TRACE_ALLOCATIONS
int malloc_trace_walkthrough(int (*callback)(void *item, void *data), void *data);

#define strdupz(s) strdupz_int(s, __FILE__, __FUNCTION__, __LINE__)
#define strndupz(s, len) strndupz_int(s, len, __FILE__, __FUNCTION__, __LINE__)
#define callocz(nmemb, size) callocz_int(nmemb, size, __FILE__, __FUNCTION__, __LINE__)
#define mallocz(size) mallocz_int(size, __FILE__, __FUNCTION__, __LINE__)
#define reallocz(ptr, size) reallocz_int(ptr, size, __FILE__, __FUNCTION__, __LINE__)
#define freez(ptr) freez_int(ptr, __FILE__, __FUNCTION__, __LINE__)
#define mallocz_usable_size(ptr) mallocz_usable_size_int(ptr, __FILE__, __FUNCTION__, __LINE__)

char *strdupz_int(const char *s, const char *file, const char *function, size_t line);
char *strndupz_int(const char *s, size_t len, const char *file, const char *function, size_t line);
void *callocz_int(size_t nmemb, size_t size, const char *file, const char *function, size_t line);
void *mallocz_int(size_t size, const char *file, const char *function, size_t line);
void *reallocz_int(void *ptr, size_t size, const char *file, const char *function, size_t line);
void freez_int(void *ptr, const char *file, const char *function, size_t line);
size_t mallocz_usable_size_int(void *ptr, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS
char *strdupz(const char *s) MALLOCLIKE NEVERNULL;
char *strndupz(const char *s, size_t len) MALLOCLIKE NEVERNULL;
void *callocz(size_t nmemb, size_t size) MALLOCLIKE NEVERNULL;
void *mallocz(size_t size) MALLOCLIKE NEVERNULL;
void *reallocz(void *ptr, size_t size) MALLOCLIKE NEVERNULL;
void freez(void *ptr);
#endif // NETDATA_TRACE_ALLOCATIONS

void posix_memfree(void *ptr);

void json_escape_string(char *dst, const char *src, size_t size);
void json_fix_string(char *s);

void *netdata_mmap(const char *filename, size_t size, int flags, int ksm, bool read_only, int *open_fd);
int netdata_munmap(void *ptr, size_t size);
int memory_file_save(const char *filename, void *mem, size_t size);

extern struct rlimit rlimit_nofile;

extern int enable_ksm;

char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

int verify_netdata_host_prefix(bool log_msg);

extern volatile sig_atomic_t netdata_exit;

extern const char *program_version;

char *strdupz_path_subpath(const char *path, const char *subpath);
int path_is_dir(const char *path, const char *subpath);
int path_is_file(const char *path, const char *subpath);
void recursive_config_double_dir_load(
        const char *user_path
        , const char *stock_path
        , const char *subpath
        , int (*callback)(const char *filename, void *data, bool stock_config)
        , void *data
        , size_t depth
);
char *read_by_filename(const char *filename, long *file_size);
char *find_and_replace(const char *src, const char *find, const char *replace, const char *where);

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#define BITS_IN_A_KILOBIT     1000
#define KILOBITS_IN_A_MEGABIT 1000

/* misc. */

#define UNUSED(x) (void)(x)

#ifdef __GNUC__
#define UNUSED_FUNCTION(x) __attribute__((unused)) UNUSED_##x
#else
#define UNUSED_FUNCTION(x) UNUSED_##x
#endif

#define error_report(x, args...) do { errno = 0; netdata_log_error(x, ##args); } while(0)

// Taken from linux kernel
#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

#include "bitmap64.h"

#define COMPRESSION_MAX_CHUNK 0x4000
#define COMPRESSION_MAX_OVERHEAD 128
#define COMPRESSION_MAX_MSG_SIZE (COMPRESSION_MAX_CHUNK - COMPRESSION_MAX_OVERHEAD - 1)
#define PLUGINSD_LINE_MAX (COMPRESSION_MAX_MSG_SIZE - 768)

bool run_command_and_copy_output_to_stdout(const char *command, int max_line_length);
struct web_buffer *run_command_and_get_output_to_buffer(const char *command, int max_line_length);

typedef enum {
    OPEN_FD_ACTION_CLOSE,
    OPEN_FD_ACTION_FD_CLOEXEC
} OPEN_FD_ACTION;
typedef enum {
    OPEN_FD_EXCLUDE_STDIN   = 0x01,
    OPEN_FD_EXCLUDE_STDOUT  = 0x02,
    OPEN_FD_EXCLUDE_STDERR  = 0x04
} OPEN_FD_EXCLUDE;
void for_each_open_fd(OPEN_FD_ACTION action, OPEN_FD_EXCLUDE excluded_fds);

void netdata_cleanup_and_exit(int ret, const char *action, const char *action_result, const char *action_data) NORETURN;
extern char *netdata_configured_host_prefix;

#define XXH_INLINE_ALL
#include "xxhash.h"

#include "uuid/uuid.h"
#include "template-enum.h"
#include "http/http_access.h"
#include "http/content_type.h"
#include "config/dyncfg.h"
#include "libjudy/src/Judy.h"
#include "july/july.h"
#include "os.h"
#include "threads/threads.h"
#include "buffer/buffer.h"
#include "locks/locks.h"
#include "circular_buffer/circular_buffer.h"
#include "avl/avl.h"
#include "inlined.h"
#include "line_splitter/line_splitter.h"
#include "clocks/clocks.h"
#include "datetime/iso8601.h"
#include "datetime/rfc3339.h"
#include "datetime/rfc7231.h"
#include "completion/completion.h"
#include "popen/popen.h"
#include "simple_pattern/simple_pattern.h"
#ifdef ENABLE_HTTPS
# include "socket/security.h"
#endif
#include "socket/socket.h"
#include "config/appconfig.h"
#include "log/journal.h"
#include "log/log.h"
#include "buffered_reader/buffered_reader.h"
#include "procfile/procfile.h"
#include "string/string.h"
#include "dictionary/dictionary.h"
#include "dictionary/thread-cache.h"
#if defined(HAVE_LIBBPF) && !defined(__cplusplus)
#include "ebpf/ebpf.h"
#endif
#include "eval/eval.h"
#include "statistical/statistical.h"
#include "adaptive_resortable_list/adaptive_resortable_list.h"
#include "url/url.h"
#include "json/json.h"
#include "json/json-c-parser-inline.h"
#include "string/utf8.h"
#include "libnetdata/aral/aral.h"
#include "onewayalloc/onewayalloc.h"
#include "worker_utilization/worker_utilization.h"
#include "yaml.h"
#include "http/http_defs.h"
#include "gorilla/gorilla.h"
#include "facets/facets.h"
#include "functions_evloop/functions_evloop.h"
#include "query_progress/progress.h"

// BEWARE: this exists in alarm-notify.sh
#define DEFAULT_CLOUD_BASE_URL "https://app.netdata.cloud"

#define RRD_STORAGE_TIERS 5

static inline size_t struct_natural_alignment(size_t size) __attribute__((const));

#define STRUCT_NATURAL_ALIGNMENT (sizeof(uintptr_t) * 2)
static inline size_t struct_natural_alignment(size_t size) {
    if(unlikely(size % STRUCT_NATURAL_ALIGNMENT))
        size = size + STRUCT_NATURAL_ALIGNMENT - (size % STRUCT_NATURAL_ALIGNMENT);

    return size;
}

#ifdef NETDATA_TRACE_ALLOCATIONS
struct malloc_trace {
    avl_t avl;

    const char *function;
    const char *file;
    size_t line;

    size_t malloc_calls;
    size_t calloc_calls;
    size_t realloc_calls;
    size_t strdup_calls;
    size_t free_calls;

    size_t mmap_calls;
    size_t munmap_calls;

    size_t allocations;
    size_t bytes;

    struct rrddim *rd_bytes;
    struct rrddim *rd_allocations;
    struct rrddim *rd_avg_alloc;
    struct rrddim *rd_ops;
};
#endif // NETDATA_TRACE_ALLOCATIONS

static inline PPvoid_t JudyLFirstThenNext(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JudyLFirst(PArray, PIndex, PJE0);
    }

    return JudyLNext(PArray, PIndex, PJE0);
}

static inline PPvoid_t JudyLLastThenPrev(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JudyLLast(PArray, PIndex, PJE0);
    }

    return JudyLPrev(PArray, PIndex, PJE0);
}

typedef enum {
    TIMING_STEP_INTERNAL = 0,

    TIMING_STEP_BEGIN2_PREPARE,
    TIMING_STEP_BEGIN2_FIND_CHART,
    TIMING_STEP_BEGIN2_PARSE,
    TIMING_STEP_BEGIN2_ML,
    TIMING_STEP_BEGIN2_PROPAGATE,
    TIMING_STEP_BEGIN2_STORE,

    TIMING_STEP_SET2_PREPARE,
    TIMING_STEP_SET2_LOOKUP_DIMENSION,
    TIMING_STEP_SET2_PARSE,
    TIMING_STEP_SET2_ML,
    TIMING_STEP_SET2_PROPAGATE,
    TIMING_STEP_RRDSET_STORE_METRIC,
    TIMING_STEP_DBENGINE_FIRST_CHECK,
    TIMING_STEP_DBENGINE_CHECK_DATA,
    TIMING_STEP_DBENGINE_PACK,
    TIMING_STEP_DBENGINE_PAGE_FIN,
    TIMING_STEP_DBENGINE_MRG_UPDATE,
    TIMING_STEP_DBENGINE_PAGE_ALLOC,
    TIMING_STEP_DBENGINE_CREATE_NEW_PAGE,
    TIMING_STEP_DBENGINE_FLUSH_PAGE,
    TIMING_STEP_SET2_STORE,

    TIMING_STEP_END2_PREPARE,
    TIMING_STEP_END2_PUSH_V1,
    TIMING_STEP_END2_ML,
    TIMING_STEP_END2_RRDSET,
    TIMING_STEP_END2_PROPAGATE,
    TIMING_STEP_END2_STORE,

    TIMING_STEP_FREEIPMI_CTX_CREATE,
    TIMING_STEP_FREEIPMI_DSR_CACHE_DIR,
    TIMING_STEP_FREEIPMI_SENSOR_CONFIG_FILE,
    TIMING_STEP_FREEIPMI_SENSOR_READINGS_BY_X,
    TIMING_STEP_FREEIPMI_READ_record_id,
    TIMING_STEP_FREEIPMI_READ_sensor_number,
    TIMING_STEP_FREEIPMI_READ_sensor_type,
    TIMING_STEP_FREEIPMI_READ_sensor_name,
    TIMING_STEP_FREEIPMI_READ_sensor_state,
    TIMING_STEP_FREEIPMI_READ_sensor_units,
    TIMING_STEP_FREEIPMI_READ_sensor_bitmask_type,
    TIMING_STEP_FREEIPMI_READ_sensor_bitmask,
    TIMING_STEP_FREEIPMI_READ_sensor_bitmask_strings,
    TIMING_STEP_FREEIPMI_READ_sensor_reading_type,
    TIMING_STEP_FREEIPMI_READ_sensor_reading,
    TIMING_STEP_FREEIPMI_READ_event_reading_type_code,
    TIMING_STEP_FREEIPMI_READ_record_type,
    TIMING_STEP_FREEIPMI_READ_record_type_class,
    TIMING_STEP_FREEIPMI_READ_sel_state,
    TIMING_STEP_FREEIPMI_READ_event_direction,
    TIMING_STEP_FREEIPMI_READ_event_type_code,
    TIMING_STEP_FREEIPMI_READ_event_offset_type,
    TIMING_STEP_FREEIPMI_READ_event_offset,
    TIMING_STEP_FREEIPMI_READ_event_offset_string,
    TIMING_STEP_FREEIPMI_READ_manufacturer_id,

    // terminator
    TIMING_STEP_MAX,
} TIMING_STEP;

typedef enum {
    TIMING_ACTION_INIT,
    TIMING_ACTION_STEP,
    TIMING_ACTION_FINISH,
} TIMING_ACTION;

#ifdef NETDATA_TIMING_REPORT
#define timing_init() timing_action(TIMING_ACTION_INIT, TIMING_STEP_INTERNAL)
#define timing_step(step) timing_action(TIMING_ACTION_STEP, step)
#define timing_report() timing_action(TIMING_ACTION_FINISH, TIMING_STEP_INTERNAL)
#else
#define timing_init() debug_dummy()
#define timing_step(step) debug_dummy()
#define timing_report() debug_dummy()
#endif
void timing_action(TIMING_ACTION action, TIMING_STEP step);

int hash256_string(const unsigned char *string, size_t size, char *hash);

extern bool unittest_running;
#define API_RELATIVE_TIME_MAX (3 * 365 * 86400)

bool rrdr_relative_window_to_absolute(time_t *after, time_t *before, time_t now);
bool rrdr_relative_window_to_absolute_query(time_t *after, time_t *before, time_t *now_ptr, bool unittest);

int netdata_base64_decode(const char *encoded, char *decoded, size_t decoded_size);

static inline void freez_charp(char **p) {
    freez(*p);
}

static inline void freez_const_charp(const char **p) {
    freez((void *)*p);
}

#define CLEAN_CONST_CHAR_P _cleanup_(freez_const_charp) const char
#define CLEAN_CHAR_P _cleanup_(freez_charp) char

# ifdef __cplusplus
}
# endif

#endif // NETDATA_LIB_H
