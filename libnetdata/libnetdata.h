// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_H
#define NETDATA_LIB_H 1

# ifdef __cplusplus
extern "C" {
# endif

#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

#if defined(NETDATA_DEV_MODE) && !defined(NETDATA_INTERNAL_CHECKS)
#define NETDATA_INTERNAL_CHECKS 1
#endif

// NETDATA_TRACE_ALLOCATIONS does not work under musl libc, so don't enable it
//#if defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_TRACE_ALLOCATIONS)
//#define NETDATA_TRACE_ALLOCATIONS 1
//#endif

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

#define ABS(x) (((x) < 0)? (-(x)) : (x))
#define MIN(a,b) (((a)<(b))?(a):(b))
#define MAX(a,b) (((a)>(b))?(a):(b))

#define GUID_LEN 36

// ---------------------------------------------------------------------------------------------
// double linked list management

#define DOUBLE_LINKED_LIST_PREPEND_UNSAFE(head, item, prev, next)                              \
    do {                                                                                       \
        (item)->next = (head);                                                                 \
                                                                                               \
        if(likely(head)) {                                                                     \
            (item)->prev = (head)->prev;                                                       \
            (head)->prev = (item);                                                             \
        }                                                                                      \
        else                                                                                   \
            (item)->prev = (item);                                                             \
                                                                                               \
        (head) = (item);                                                                       \
    } while (0)

#define DOUBLE_LINKED_LIST_APPEND_UNSAFE(head, item, prev, next)                               \
    do {                                                                                       \
        if(likely(head)) {                                                                     \
            (item)->prev = (head)->prev;                                                       \
            (head)->prev->next = (item);                                                       \
            (head)->prev = (item);                                                             \
            (item)->next = NULL;                                                               \
        }                                                                                      \
        else {                                                                                 \
            (head) = (item);                                                                   \
            (head)->prev = (head);                                                             \
            (head)->next = NULL;                                                               \
        }                                                                                      \
                                                                                               \
    } while (0)

#define DOUBLE_LINKED_LIST_REMOVE_UNSAFE(head, item, prev, next)                               \
    do {                                                                                       \
        fatal_assert((head) != NULL);                                                          \
        fatal_assert((item)->prev != NULL);                                                    \
                                                                                               \
        if((item)->prev == (item)) {                                                           \
            /* it is the only item in the list */                                              \
            (head) = NULL;                                                                     \
        }                                                                                      \
        else if((item) == (head)) {                                                            \
            /* it is the first item */                                                         \
            (item)->next->prev = (item)->prev;                                                 \
            (head) = (item)->next;                                                             \
        }                                                                                      \
        else {                                                                                 \
            (item)->prev->next = (item)->next;                                                 \
            if ((item)->next) {                                                                \
                (item)->next->prev = (item)->prev;                                             \
            }                                                                                  \
            else {                                                                             \
                (head)->prev = (item)->prev;                                                   \
            }                                                                                  \
        }                                                                                      \
                                                                                               \
        (item)->next = NULL;                                                                   \
        (item)->prev = NULL;                                                                   \
    } while (0)

#define DOUBLE_LINKED_LIST_FOREACH_FORWARD(head, var, prev, next)                              \
    for ((var) = (head); (var) ; (var) = (var)->next)

#define DOUBLE_LINKED_LIST_FOREACH_BACKWARD(head, var, prev, next)                             \
    for ((var) = (head)?(head)->prev:NULL; (var) && (var) != (head)->prev ; (var) = (var)->prev)

// ---------------------------------------------------------------------------------------------


void netdata_fix_chart_id(char *s);
void netdata_fix_chart_name(char *s);

void strreverse(char* begin, char* end);
char *mystrsep(char **ptr, char *s);
char *trim(char *s); // remove leading and trailing spaces; may return NULL
char *trim_all(char *buffer); // like trim(), but also remove duplicate spaces inside the string; may return NULL

int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
int  snprintfz(char *dst, size_t n, const char *fmt, ...) PRINTFLIKE(3, 4);

// memory allocation functions that handle failures
#ifdef NETDATA_TRACE_ALLOCATIONS
int malloc_trace_walkthrough(int (*callback)(void *item, void *data), void *data);

#define strdupz(s) strdupz_int(s, __FILE__, __FUNCTION__, __LINE__)
#define callocz(nmemb, size) callocz_int(nmemb, size, __FILE__, __FUNCTION__, __LINE__)
#define mallocz(size) mallocz_int(size, __FILE__, __FUNCTION__, __LINE__)
#define reallocz(ptr, size) reallocz_int(ptr, size, __FILE__, __FUNCTION__, __LINE__)
#define freez(ptr) freez_int(ptr, __FILE__, __FUNCTION__, __LINE__)
#define mallocz_usable_size(ptr) mallocz_usable_size_int(ptr, __FILE__, __FUNCTION__, __LINE__)

char *strdupz_int(const char *s, const char *file, const char *function, size_t line);
void *callocz_int(size_t nmemb, size_t size, const char *file, const char *function, size_t line);
void *mallocz_int(size_t size, const char *file, const char *function, size_t line);
void *reallocz_int(void *ptr, size_t size, const char *file, const char *function, size_t line);
void freez_int(void *ptr, const char *file, const char *function, size_t line);
size_t mallocz_usable_size_int(void *ptr, const char *file, const char *function, size_t line);

#else // NETDATA_TRACE_ALLOCATIONS
char *strdupz(const char *s) MALLOCLIKE NEVERNULL;
void *callocz(size_t nmemb, size_t size) MALLOCLIKE NEVERNULL;
void *mallocz(size_t size) MALLOCLIKE NEVERNULL;
void *reallocz(void *ptr, size_t size) MALLOCLIKE NEVERNULL;
void freez(void *ptr);
#endif // NETDATA_TRACE_ALLOCATIONS

void posix_memfree(void *ptr);

void json_escape_string(char *dst, const char *src, size_t size);
void json_fix_string(char *s);

void *netdata_mmap(const char *filename, size_t size, int flags, int ksm, bool read_only);
int netdata_munmap(void *ptr, size_t size);
int memory_file_save(const char *filename, void *mem, size_t size);

int fd_is_valid(int fd);

extern struct rlimit rlimit_nofile;

extern int enable_ksm;

char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

int verify_netdata_host_prefix();

int recursively_delete_dir(const char *path, const char *reason);

extern volatile sig_atomic_t netdata_exit;

extern const char *program_version;

char *strdupz_path_subpath(const char *path, const char *subpath);
int path_is_dir(const char *path, const char *subpath);
int path_is_file(const char *path, const char *subpath);
void recursive_config_double_dir_load(
        const char *user_path
        , const char *stock_path
        , const char *subpath
        , int (*callback)(const char *filename, void *data)
        , void *data
        , size_t depth
);
char *read_by_filename(char *filename, long *file_size);
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

#define error_report(x, args...) do { errno = 0; error(x, ##args); } while(0)

// Taken from linux kernel
#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

typedef struct bitmap256 {
    uint64_t data[4];
} BITMAP256;

bool bitmap256_get_bit(BITMAP256 *ptr, uint8_t idx);
void bitmap256_set_bit(BITMAP256 *ptr, uint8_t idx, bool value);

#define COMPRESSION_MAX_MSG_SIZE 0x4000
#define PLUGINSD_LINE_MAX (COMPRESSION_MAX_MSG_SIZE - 1024)
int config_isspace(char c);
int pluginsd_space(char c);

size_t quoted_strings_splitter(char *str, char **words, size_t max_words, int (*custom_isspace)(char), char *recover_input, char **recover_location, int max_recover);
size_t pluginsd_split_words(char *str, char **words, size_t max_words, char *recover_string, char **recover_location, int max_recover);

static inline char *get_word(char **words, size_t num_words, size_t index) {
    if (index >= num_words)
        return NULL;

    return words[index];
}

bool run_command_and_copy_output_to_stdout(const char *command, int max_line_length);

void netdata_cleanup_and_exit(int ret) NORETURN;
void send_statistics(const char *action, const char *action_result, const char *action_data);
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
#include "completion/completion.h"
#include "popen/popen.h"
#include "simple_pattern/simple_pattern.h"
#ifdef ENABLE_HTTPS
# include "socket/security.h"
#endif
#include "socket/socket.h"
#include "config/appconfig.h"
#include "log/log.h"
#include "procfile/procfile.h"
#include "string/string.h"
#include "dictionary/dictionary.h"
#if defined(HAVE_LIBBPF) && !defined(__cplusplus)
#include "ebpf/ebpf.h"
#endif
#include "eval/eval.h"
#include "statistical/statistical.h"
#include "adaptive_resortable_list/adaptive_resortable_list.h"
#include "url/url.h"
#include "json/json.h"
#include "health/health.h"
#include "string/utf8.h"
#include "arrayalloc/arrayalloc.h"
#include "onewayalloc/onewayalloc.h"
#include "worker_utilization/worker_utilization.h"

// BEWARE: Outside of the C code this also exists in alarm-notify.sh
#define DEFAULT_CLOUD_BASE_URL "https://api.netdata.cloud"
#define DEFAULT_CLOUD_UI_URL "https://app.netdata.cloud"

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

# ifdef __cplusplus
}
# endif

#endif // NETDATA_LIB_H
