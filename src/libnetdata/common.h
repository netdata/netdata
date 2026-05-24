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

#include "config.h"

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

#ifdef HAVE_LIBDATACHANNEL
#define ENABLE_WEBRTC 1
#endif

#define STRINGIFY(x) #x
#define TOSTRING(x) STRINGIFY(x)

// --------------------------------------------------------------------------------------------------------------------
// NETDATA_OS_TYPE

#if defined(__FreeBSD__)
#include <pthread_np.h>
#define NETDATA_OS_TYPE "freebsd"
#elif defined(__APPLE__)
#define NETDATA_OS_TYPE "macos"
#elif defined(OS_WINDOWS)
#define NETDATA_OS_TYPE "windows"
#else
#define NETDATA_OS_TYPE "linux"
#endif /* __FreeBSD__, __APPLE__*/

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
// Windows has no POSIX group database (NTFS / SIDs / ACLs instead).
// Provide a struct group + getgrgid/getgrnam/getgrgid_r stubs so
// cross-platform callers compile. All stubs report "not found" so
// the existing fallback paths (numeric gid formatting, default
// group, etc.) take over.
struct group {
    char  *gr_name;
    char  *gr_passwd;
    gid_t  gr_gid;
    char **gr_mem;
};
static inline struct group *getgrgid(gid_t gid) { (void)gid; return NULL; }
static inline struct group *getgrnam(const char *name) { (void)name; return NULL; }
static inline int getgrgid_r(gid_t gid, struct group *grp, char *buf, size_t buflen, struct group **result) {
    (void)gid; (void)grp; (void)buf; (void)buflen;
    if (result) *result = NULL;
    return ENOENT;
}
#endif

#ifdef HAVE_PWD_H
#include <pwd.h>
#else
typedef uint32_t uid_t;
// Windows has no POSIX user database. Same stub strategy as struct
// group above: provide just enough surface so call sites compile, and
// signal "no entry" at runtime so the existing pw == NULL / non-zero
// return fallbacks run.
struct passwd {
    char  *pw_name;
    char  *pw_passwd;
    uid_t  pw_uid;
    gid_t  pw_gid;
    char  *pw_gecos;
    char  *pw_dir;
    char  *pw_shell;
};
static inline struct passwd *getpwuid(uid_t uid) { (void)uid; return NULL; }
static inline struct passwd *getpwnam(const char *name) { (void)name; return NULL; }
static inline int getpwuid_r(uid_t uid, struct passwd *pwd, char *buf, size_t buflen, struct passwd **result) {
    (void)uid; (void)pwd; (void)buf; (void)buflen;
    if (result) *result = NULL;
    return ENOENT;
}
static inline int getpwnam_r(const char *name, struct passwd *pwd, char *buf, size_t buflen, struct passwd **result) {
    (void)name; (void)pwd; (void)buf; (void)buflen;
    if (result) *result = NULL;
    return ENOENT;
}
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

#if defined(OS_WINDOWS)
// UCRT64 ships no <sys/mman.h>; provide a POSIX-shaped shim built on
// top of CreateFileMappingW/MapViewOfFile and VirtualAlloc. See
// memory/mman-win32.h for the supported subset and the rationale.
#include "memory/mman-win32.h"
#elif defined(HAVE_SYS_MMAN_H)
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

#if defined(HAVE_INTTYPES_H)
#include <inttypes.h>
#elif defined(HAVE_STDINT_H)
#include <stdint.h>
#endif

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
// FUNCTION ATTRIBUTES

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

#if defined(HAVE_FUNC_ATTRIBUTE_FORMAT_GNU_PRINTF)
#define PRINTFLIKE(f, a) __attribute__ ((format(gnu_printf, f, a)))
#elif defined(HAVE_FUNC_ATTRIBUTE_FORMAT_PRINTF)
#define PRINTFLIKE(f, a) __attribute__ ((format(printf, f, a)))
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

#define UNUSED(x) (void)(x)

#if defined(__GNUC__) && !defined(FSANITIZE_ADDRESS)
#define UNUSED_FUNCTION(x) __attribute__((unused)) UNUSED_##x
#define ALWAYS_INLINE_ONLY __attribute__((always_inline))
#define ALWAYS_INLINE inline __attribute__((always_inline))             // Forces inlining
#define ALWAYS_INLINE_HOT inline __attribute__((hot, always_inline))    // Encourages optimization and forces inlining
#define ALWAYS_INLINE_HOT_FLATTEN inline __attribute__((hot, always_inline, flatten))    // Encourages optimization and forces inlining and flattening
#define NOT_INLINE_HOT __attribute__((hot))                             // Encourages optimization but doesn’t force inlining.
#define NEVER_INLINE __attribute__((noinline))
#else
#define UNUSED_FUNCTION(x) UNUSED_##x
#define ALWAYS_INLINE_ONLY
#define ALWAYS_INLINE inline
#define ALWAYS_INLINE_HOT inline
#define ALWAYS_INLINE_HOT_FLATTEN inline
#define NOT_INLINE_HOT
#define NEVER_INLINE
#endif

// --------------------------------------------------------------------------------------------------------------------
// fix for alpine linux

#if !defined(RUSAGE_THREAD) && defined(RUSAGE_CHILDREN)
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif

// --------------------------------------------------------------------------------------------------------------------
// HELPFUL MACROS

#define ABS(x) (((x) < 0)? (-(x)) : (x))
#define MIN(a,b) (((a)<(b))?(a):(b))
#define MAX(a,b) (((a)>(b))?(a):(b))
#define SWAP(a, b) do { \
    typeof(a) _tmp = b; \
    b = a;              \
    a = _tmp;           \
} while(0)

// returns the number of times the divider fits into the total
// if the divider is 0, it is treated as 1 (it returns total)
#define HOWMANY(total, divider) ({ \
    typeof(total) _t = (total);    \
    typeof(total) _d = (divider);  \
    _d = _d ? _d : 1;              \
    (_t + (_d - 1)) / _d;          \
})

#define FIT_IN_RANGE(value, min, max) ({ \
    typeof(value) _v = (value);     \
    typeof(min) _min = (min);       \
    typeof(max) _max = (max);       \
    (_v < _min) ? _min : ((_v > _max) ? _max : _v); \
})

// --------------------------------------------------------------------------------------------------------------------
// NETDATA CLOUD

// BEWARE: this exists in alarm-notify.sh
#define DEFAULT_CLOUD_BASE_URL "https://app.netdata.cloud"

// --------------------------------------------------------------------------------------------------------------------
// DBENGINE

#define RRD_STORAGE_TIERS 5

// --------------------------------------------------------------------------------------------------------------------
// PIPES

#define PIPE_READ 0
#define PIPE_WRITE 1

// --------------------------------------------------------------------------------------------------------------------
// UUIDs

#define GUID_LEN 36

// --------------------------------------------------------------------------------------------------------------------
// Macro-only includes

#include "libnetdata/linked_lists/linked_lists.h"

// --------------------------------------------------------------------------------------------------------------------

// Taken from linux kernel
#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

// --------------------------------------------------------------------------------------------------------------------

#define CONCAT_INDIRECT(a, b) a##b
#define CONCAT(a, b) CONCAT_INDIRECT(a, b)
#define PAD64(type) uint8_t CONCAT(padding, __COUNTER__)[64 - sizeof(type)]; type

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
#include <winevt.h>
// if_nametoindex() lives in <net/if.h> on POSIX. Windows has had it
// since Vista in <iphlpapi.h>; pull it in here so socket/connect-to.c
// and socket/listen-sockets.c get the declaration for free.
#include <iphlpapi.h>

// posix_memalign() has no UCRT equivalent. UCRT exposes
// _aligned_malloc(size, alignment) -- args swapped vs POSIX, returns
// pointer-or-NULL instead of int-errno, and memory allocated this way
// MUST be released with _aligned_free, NOT free(). Provide a POSIX
// adapter so nd-mallocz.c's posix_memalignz() compiles unchanged.
// nd-mallocz.c's matching posix_memalign_freez() is patched separately
// to route through _aligned_free on Windows.
#include <malloc.h>
static inline int posix_memalign(void **memptr, size_t alignment, size_t size) {
    if (!memptr) return EINVAL;
    void *p = _aligned_malloc(size, alignment);
    if (!p) {
        *memptr = NULL;
        return ENOMEM;
    }
    *memptr = p;
    return 0;
}

// UCRT has the C-standard signal() / SIG_DFL / SIG_IGN / SIGINT etc.
// but none of POSIX's sigaction / siginfo_t / sigset_t / sigsetjmp
// API. signal-handler.c and protected-access.{h,c} use those types
// freely. Master compiled this code under MSYS-gcc only because the
// Cygwin runtime emulates POSIX signals.
//
// Provide the type and function surface as no-op stubs so the files
// compile. Signal handling on Windows degrades to "C-standard
// signal()" behaviour: SIGINT/SIGTERM still work for clean shutdown
// via the default handler, but no chained handlers, no
// SIGBUS/SIGSEGV mmap-fault recovery, no signal masking. This
// matches master's effective behaviour (the POSIX-shaped calls there
// were translated by Cygwin into best-effort SEH).
//
// Proper Windows signal handling -- SEH-based mmap fault recovery,
// CTRL+C handler via SetConsoleCtrlHandler, structured exception
// translation -- is a separate workstream.
typedef unsigned long sigset_t;

typedef struct {
    int   si_signo;
    int   si_errno;
    int   si_code;
    void *si_addr;
} siginfo_t;

struct sigaction {
    union {
        void (*sa_handler)(int);
        void (*sa_sigaction)(int, siginfo_t *, void *);
    };
    sigset_t sa_mask;
    int      sa_flags;
};

#ifndef SA_SIGINFO
#define SA_SIGINFO    0x0004
#endif
#ifndef SA_NODEFER
#define SA_NODEFER    0x0040
#endif
#ifndef SA_RESETHAND
#define SA_RESETHAND  0x0080
#endif
#ifndef SA_RESTART
#define SA_RESTART    0x0010
#endif
#ifndef SIG_BLOCK
#define SIG_BLOCK     0
#endif
#ifndef SIG_UNBLOCK
#define SIG_UNBLOCK   1
#endif
#ifndef SIG_SETMASK
#define SIG_SETMASK   2
#endif
#ifndef SI_USER
#define SI_USER       0
#endif
#ifndef SEGV_MAPERR
#define SEGV_MAPERR   1
#endif
#ifndef SEGV_ACCERR
#define SEGV_ACCERR   2
#endif
#ifndef BUS_ADRALN
#define BUS_ADRALN    1
#endif
#ifndef BUS_ADRERR
#define BUS_ADRERR    2
#endif
#ifndef BUS_OBJERR
#define BUS_OBJERR    3
#endif

static inline int sigemptyset(sigset_t *set)            { if (set) *set = 0; return 0; }
static inline int sigfillset(sigset_t *set)             { if (set) *set = (sigset_t)-1; return 0; }
static inline int sigaddset(sigset_t *set, int signo)   { (void)signo; if (set) *set |= 1u; return 0; }
static inline int sigdelset(sigset_t *set, int signo)   { (void)signo; (void)set; return 0; }
static inline int sigismember(const sigset_t *set, int signo) { (void)set; (void)signo; return 0; }
static inline int sigprocmask(int how, const sigset_t *set, sigset_t *oset) {
    (void)how; (void)set; (void)oset; return 0;
}
static inline int sigaction(int signum, const struct sigaction *act, struct sigaction *oldact) {
    (void)signum; (void)act;
    if (oldact) {
        oldact->sa_handler = (void (*)(int))0;
        oldact->sa_mask = 0;
        oldact->sa_flags = 0;
    }
    return 0;
}

// sigsetjmp / siglongjmp degrade to plain setjmp / longjmp. The
// "savemask" argument is irrelevant -- signal masks aren't a thing
// here -- so we discard it. This is what mingw-w64's _setjmp/_longjmp
// already are under the hood.
#include <setjmp.h>
typedef jmp_buf sigjmp_buf;
#ifndef sigsetjmp
#define sigsetjmp(env, savemask) setjmp(env)
#endif
#ifndef siglongjmp
#define siglongjmp(env, val) longjmp((env), (val))
#endif

// ffs() (find-first-set, 1-based, returns 0 if no bits set) is a POSIX
// function from <strings.h>. UCRT64 doesn't have it. gcc has
// __builtin_ffs always available, and rrdenginelib.h calls ffs() in
// the cache-bitmap helpers. Forward via macro on Windows; the builtin
// emits the same single `bsf`/`tzcnt` instruction as glibc.
#ifndef ffs
#define ffs(x) __builtin_ffs(x)
#endif

// Winsock has no per-call MSG_DONTWAIT flag (POSIX recv/send/recvmsg/sendmsg
// flag for "non-blocking just this once"). Cygwin's fhandler_socket layer
// emulated it via internal events; under UCRT64 there is no emulation, so
// existing call sites must rely on the socket being in non-blocking mode
// (set via sock_setnonblock(fd, true) at creation). Defining the flag to 0
// here lets those call sites compile unchanged.
#ifndef MSG_DONTWAIT
#define MSG_DONTWAIT 0
#endif

// UCRT64's <sys/types.h> declares `useconds_t` but not POSIX's
// `suseconds_t`. struct timeval::tv_usec is plain `long` on this
// platform (mingw-w64's <sys/time.h>), so map the type accordingly --
// keeping all the (suseconds_t) casts in collectors and clocks code
// compiling unchanged.
typedef long suseconds_t;

// UCRT64 has Microsoft's gmtime_s / localtime_s (errno_t return,
// reversed argument order) but no POSIX gmtime_r / localtime_r.
// Provide thin inline adapters so call sites compile unchanged. Both
// shims populate *result and return it on success, NULL on failure --
// matching the POSIX contract exactly.
static inline struct tm *gmtime_r(const time_t *timep, struct tm *result) {
    return (gmtime_s(result, timep) == 0) ? result : NULL;
}
static inline struct tm *localtime_r(const time_t *timep, struct tm *result) {
    return (localtime_s(result, timep) == 0) ? result : NULL;
}

// UCRT64 has no POSIX fchown(). Windows uses ACLs/SIDs, not POSIX
// uid/gid ownership. nd_log-to-file.c uses fchown() to re-chown an
// already-opened log file after a privilege drop; that whole code
// path is meaningless on Windows. Stub to return 0 so the caller's
// "==-1" error check never trips.
static inline int fchown(int fd, uid_t owner, gid_t group) {
    (void)fd; (void)owner; (void)group;
    return 0;
}

// POSIX poll's "number of fds" type. mingw-w64 declares WSAPoll using
// ULONG; provide the POSIX-shaped typedef so log-forwarder.c's
// (nfds_t)-1 casts compile. ULONG is 32-bit under LLP64, matching
// glibc's nfds_t.
typedef ULONG nfds_t;

// POSIX pipe(int[2]) on UCRT64 is _pipe(int[2], unsigned size, int mode).
// Adapt with a macro: 64 KiB buffer (Cygwin's historical default) +
// binary mode (no CRLF translation) for log forwarder pipes and any
// other unblocked-pipe consumers.
#ifndef pipe
#define pipe(fds) _pipe((fds), 65536, _O_BINARY)
#endif

// UCRT64 has no <endian.h> -- no htole16/32/64, htobe16/32/64,
// le16toh/le32toh/le64toh, be16toh/be32toh/be64toh. Windows x86_64
// is always little-endian (LLP64 host), so htole*() is identity and
// htobe*()/be*toh() reduce to a byte-swap via UCRT's builtin
// _byteswap_*() functions (single-instruction `bswap` on x86).
#ifndef htole16
#define htole16(x) ((uint16_t)(x))
#endif
#ifndef htole32
#define htole32(x) ((uint32_t)(x))
#endif
#ifndef htole64
#define htole64(x) ((uint64_t)(x))
#endif
#ifndef le16toh
#define le16toh(x) ((uint16_t)(x))
#endif
#ifndef le32toh
#define le32toh(x) ((uint32_t)(x))
#endif
#ifndef le64toh
#define le64toh(x) ((uint64_t)(x))
#endif
#ifndef htobe16
#define htobe16(x) _byteswap_ushort((uint16_t)(x))
#endif
#ifndef htobe32
#define htobe32(x) _byteswap_ulong((uint32_t)(x))
#endif
#ifndef htobe64
#define htobe64(x) _byteswap_uint64((uint64_t)(x))
#endif
#ifndef be16toh
#define be16toh(x) _byteswap_ushort((uint16_t)(x))
#endif
#ifndef be32toh
#define be32toh(x) _byteswap_ulong((uint32_t)(x))
#endif
#ifndef be64toh
#define be64toh(x) _byteswap_uint64((uint64_t)(x))
#endif

// strerror_r() has the same shape: UCRT exposes strerror_s() with
// reversed argument order (buf first, errnum last). nd_log-internals.c
// auto-detects via _Generic on the return type; the XSI POSIX form
// returns int, which matches strerror_s and what we forward here.
static inline int strerror_r(int errnum, char *buf, size_t buflen) {
    return (int)strerror_s(buf, buflen, errnum);
}

// UCRT64 has no <sys/resource.h>: no struct rlimit, no getrlimit /
// setrlimit, no RLIMIT_NOFILE / RLIMIT_CORE / RLIM_INFINITY. Provide a
// minimal POSIX-shaped shim covering the two resources netdata actually
// touches:
//
//   * RLIMIT_NOFILE -> map to the Windows CRT's per-process stdio cache
//     limit via _getmaxstdio() / _setmaxstdio(). UCRT documents a
//     hard ceiling of 8192 on the latter. POSIX RLIMIT_NOFILE governs
//     a different concept (kernel file-descriptor table) but on
//     Windows the CRT's cache is the operative limit for code that
//     uses fopen / open / etc., so the mapping is semantically the
//     right one for callers that size buffers off rlim_cur / N.
//   * RLIMIT_CORE -> no-op. Windows produces minidumps through
//     SetUnhandledExceptionFilter / WER, not POSIX core dumps; there
//     is nothing rlimit-shaped to set.
typedef unsigned long rlim_t;

#ifndef RLIM_INFINITY
#define RLIM_INFINITY ((rlim_t)-1)
#endif
#ifndef RLIMIT_CORE
#define RLIMIT_CORE   4
#endif
#ifndef RLIMIT_NOFILE
#define RLIMIT_NOFILE 7
#endif

struct rlimit {
    rlim_t rlim_cur;
    rlim_t rlim_max;
};

static inline int getrlimit(int resource, struct rlimit *rl) {
    if (!rl) return -1;
    switch (resource) {
        case RLIMIT_NOFILE: {
            int n = _getmaxstdio();
            rl->rlim_cur = (rlim_t)((n > 0) ? n : 512);
            rl->rlim_max = 8192; // UCRT's documented ceiling for _setmaxstdio
            return 0;
        }
        case RLIMIT_CORE:
            rl->rlim_cur = 0;
            rl->rlim_max = RLIM_INFINITY;
            return 0;
        default:
            return -1;
    }
}

static inline int setrlimit(int resource, const struct rlimit *rl) {
    if (!rl) return -1;
    switch (resource) {
        case RLIMIT_NOFILE: {
            rlim_t want = rl->rlim_cur;
            if (want == RLIM_INFINITY || want > 8192) want = 8192;
            return (_setmaxstdio((int)want) == (int)want) ? 0 : -1;
        }
        case RLIMIT_CORE:
            return 0; // no-op; see getrlimit() comment above
        default:
            return -1;
    }
}

// UCRT64 has no <unistd.h> POSIX sysconf() interface. Provide a shim
// that handles the _SC_* keys netdata actually queries. Each mapping
// uses the closest Windows-native primitive:
//
//   * _SC_CLK_TCK -- Linux exposes kernel-jiffy rate (typically 100 or
//     1000). netdata only consumes this through system_hz for /proc CPU
//     accounting, which Windows builds never touch. Returning 100 keeps
//     the global initialized to its compile-time default.
//   * _SC_PAGESIZE / _SC_PAGE_SIZE -> GetSystemInfo().dwPageSize.
//   * _SC_NPROCESSORS_ONLN / _SC_NPROCESSORS_CONF
//       -> GetSystemInfo().dwNumberOfProcessors.
//   * _SC_NGROUPS_MAX -- Windows uses SIDs / ACLs, not POSIX
//     supplementary groups; the value just sizes a buffer in
//     become_user(). Return the Linux default of 32 so the buffer
//     allocates correctly on the unlikely path it is ever entered.
//   * _SC_OPEN_MAX -> 8192 (matches the rlimit shim cap).
#ifndef _SC_CLK_TCK
#define _SC_CLK_TCK             1
#endif
#ifndef _SC_PAGESIZE
#define _SC_PAGESIZE            2
#endif
#ifndef _SC_PAGE_SIZE
#define _SC_PAGE_SIZE           _SC_PAGESIZE
#endif
#ifndef _SC_NPROCESSORS_ONLN
#define _SC_NPROCESSORS_ONLN    3
#endif
#ifndef _SC_NPROCESSORS_CONF
#define _SC_NPROCESSORS_CONF    4
#endif
#ifndef _SC_NGROUPS_MAX
#define _SC_NGROUPS_MAX         5
#endif
#ifndef _SC_OPEN_MAX
#define _SC_OPEN_MAX            6
#endif

// UCRT64 has no <sys/wait.h>: no WIFEXITED / WEXITSTATUS / WIFSIGNALED
// / WTERMSIG, and no SIGPIPE. spawn_popen.c needs these to decode the
// status word that spawn_server_exec_wait() returns. The Windows
// spawn-server side already encodes results in POSIX format -- normal
// exits as ((exit_code & 0xFF) << 8), abnormal exits as POSIX signal
// numbers -- so the standard glibc-shaped macros DTRT here.
#ifndef WIFEXITED
#define WIFEXITED(status)    (((status) & 0x7F) == 0)
#endif
#ifndef WEXITSTATUS
#define WEXITSTATUS(status)  (((status) >> 8) & 0xFF)
#endif
#ifndef WIFSIGNALED
#define WIFSIGNALED(status)  (((status) & 0x7F) != 0 && ((status) & 0x7F) != 0x7F)
#endif
#ifndef WTERMSIG
#define WTERMSIG(status)     ((status) & 0x7F)
#endif
#ifndef SIGPIPE
// UCRT has no SIGPIPE -- there is no POSIX "write to closed pipe"
// signal on Windows; the underlying write returns EPIPE / ERROR_BROKEN_PIPE
// instead. Define the macro so switch cases over signal numbers
// compile; it never matches a real spawned-process termination.
#define SIGPIPE 13
#endif
#ifndef SIGTRAP
// UCRT has no SIGTRAP. spawn_server_windows.c uses it as the target
// of STATUS_BREAKPOINT / STATUS_SINGLE_STEP in its NTSTATUS-to-signal
// mapper. Use the POSIX value (5) so the case compiles and any
// consumer reading the resulting status can recognise it.
#define SIGTRAP 5
#endif

// Winsock's WSAPoll() is the documented Windows equivalent of POSIX
// poll() since Vista, with identical signature for struct pollfd and
// the POLLIN/POLLOUT/POLLHUP/POLLERR constants. Macro-rename so call
// sites compile unchanged. The macro is self-referencing -- C99 6.10.3
// guarantees the inner identifier resolves to the function, not the
// macro -- so no infinite expansion.
#ifndef poll
#define poll(fds, nfds, timeout) WSAPoll((fds), (nfds), (timeout))
#endif

// Winsock declares getsockopt() and setsockopt() with `char *` as the
// option-value argument, while POSIX uses `void *`. Existing call
// sites pass typed buffers (int *, struct timeval *, struct linger *,
// ...) -- compilation succeeds on Linux/macOS/FreeBSD via the
// implicit void * conversion and fails on Windows with
// -Wincompatible-pointer-types.
//
// Wrap with self-referencing macros that cast the option-value
// argument to `char *`. C99 6.10.3 guarantees no infinite expansion:
// the inner getsockopt/setsockopt resolve to the Winsock functions.
// The double cast (`void *` then `char *`) is intentional -- it strips
// `const` qualifiers callers might pass into setsockopt without
// triggering -Wcast-qual.
#ifndef getsockopt
#define getsockopt(s, level, optname, optval, optlen) \
    getsockopt((s), (level), (optname), (char *)(void *)(optval), (optlen))
#endif
#ifndef setsockopt
#define setsockopt(s, level, optname, optval, optlen) \
    setsockopt((s), (level), (optname), (const char *)(const void *)(optval), (optlen))
#endif

// strcasestr() is a GNU/BSD extension, not POSIX, and UCRT64 omits it.
// Provide a portable inline implementation. Behaviour matches glibc:
// returns a pointer into haystack at the first case-insensitive match
// of needle, NULL if not found, haystack itself if needle is empty.
static inline char *strcasestr(const char *haystack, const char *needle) {
    if (!*needle)
        return (char *)haystack;
    size_t nlen = strlen(needle);
    for (; *haystack; haystack++) {
        if (strncasecmp(haystack, needle, nlen) == 0)
            return (char *)haystack;
    }
    return NULL;
}

// UCRT64 has no readlink() (POSIX) and no S_IFLNK in <sys/stat.h>:
// Windows reparse points are reachable through GetFinalPathNameByHandle,
// not through readlink semantics. Two Windows-built call sites
// dereference these (paths/paths.c symlink-chain resolver, procfile
// filename-for-error-message getter); both already handle "no symlink
// here" gracefully -- paths.c gates the readlink call on the
// S_IFLNK == st_mode & S_IFMT check (false on Windows since stat never
// returns this bit), procfile.c falls through to "unknown filename for
// fd %d" when readlink returns -1.
//
// Use the Linux value for S_IFLNK so the macro is non-zero (callers
// that fold it into bitmasks don't silently match dirs/regular files)
// and provide a readlink() that returns -1 / EINVAL.
#ifndef S_IFLNK
#define S_IFLNK 0xA000
#endif

static inline ssize_t readlink(const char *path, char *buf, size_t bufsiz) {
    (void)path;
    (void)buf;
    (void)bufsiz;
    errno = EINVAL;
    return -1;
}

static inline long sysconf(int name) {
    SYSTEM_INFO si;
    switch (name) {
        case _SC_CLK_TCK:
            return 100;
        case _SC_PAGESIZE:
            GetSystemInfo(&si);
            return (long)si.dwPageSize;
        case _SC_NPROCESSORS_ONLN:
        case _SC_NPROCESSORS_CONF:
            GetSystemInfo(&si);
            return (long)si.dwNumberOfProcessors;
        case _SC_NGROUPS_MAX:
            return 32;
        case _SC_OPEN_MAX:
            return 8192;
        default:
            return -1;
    }
}

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
