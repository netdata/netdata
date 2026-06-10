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
// winsock2.h must come before windows.h to avoid conflicts with the older
// winsock.h that windows.h pulls in, and to get WSAPoll / ws2tcpip types.
#include <winsock2.h>
#include <ws2tcpip.h>
#include <windows.h>
#include <wctype.h>
#include <wchar.h>
#include <tchar.h>
#include <guiddef.h>
#include <io.h>
#include <fcntl.h>
#include <process.h>
#include <tlhelp32.h>
// sys/cygwin.h only exists under the Cygwin/MSYS subsystems.
// UCRT64 is a native Windows toolchain with no Cygwin layer.
#if defined(__CYGWIN__) || defined(__MSYS__)
#include <sys/cygwin.h>
#endif
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

// -------------------------------------------------------------------------
// UCRT64 / MinGW-w64 POSIX compatibility shims
// These extensions are provided by the MSYS runtime but not by UCRT64.
// -------------------------------------------------------------------------

#include <direct.h>

// suseconds_t is POSIX-only, absent from UCRT64's sys/types.h
typedef long suseconds_t;

// UCRT64 mkdir() takes only one argument; redirect the POSIX 2-arg form.
#define mkdir(path, mode) _mkdir(path)

// strsep() is a BSD/glibc extension absent from UCRT64
static inline char *strsep(char **stringp, const char *delim) {
    char *s = *stringp;
    if (!s) return NULL;
    char *end = strpbrk(s, delim);
    if (end) { *end = '\0'; *stringp = end + 1; }
    else *stringp = NULL;
    return s;
}

// POSIX thread-safe time functions — UCRT64 provides _s variants with
// reversed parameter order and errno_t return value.
static inline struct tm *gmtime_r(const time_t *timep, struct tm *result) {
    return (gmtime_s(result, timep) == 0) ? result : NULL;
}
static inline struct tm *localtime_r(const time_t *timep, struct tm *result) {
    return (localtime_s(result, timep) == 0) ? result : NULL;
}

// Minimal strptime() — UCRT64 has no strptime.
// Handles %Y %m %d %H %M %S %n %t and literal characters.
// Sufficient for Netdata's RFC 3339 / ISO 8601 parsers.
static inline char *strptime(const char *s, const char *format, struct tm *t) {
    const char *sp = s;
    const char *fp = format;
    while (*fp) {
        if (*fp != '%') {
            if (*sp != *fp) return NULL;
            sp++; fp++;
            continue;
        }
        fp++;
        char *e;
        long v;
        switch (*fp++) {
        case 'Y': v = strtol(sp, &e, 10); if (e == sp) return NULL; t->tm_year = (int)(v - 1900); sp = e; break;
        case 'm': v = strtol(sp, &e, 10); if (e == sp) return NULL; t->tm_mon  = (int)(v - 1);    sp = e; break;
        case 'd': v = strtol(sp, &e, 10); if (e == sp) return NULL; t->tm_mday = (int)v;           sp = e; break;
        case 'H': v = strtol(sp, &e, 10); if (e == sp) return NULL; t->tm_hour = (int)v;           sp = e; break;
        case 'M': v = strtol(sp, &e, 10); if (e == sp) return NULL; t->tm_min  = (int)v;           sp = e; break;
        case 'S': v = strtol(sp, &e, 10); if (e == sp) return NULL; t->tm_sec  = (int)v;           sp = e; break;
        case 'n': case 't': while (isspace((unsigned char)*sp)) sp++; break;
        default: return NULL;
        }
    }
    return (char *)sp;
}

// mmap/munmap/madvise constants — UCRT64 has no sys/mman.h.
// Implementations live in nd-mmap.c; these constants and declarations
// allow the rest of the tree to compile without per-call-site guards.
#ifndef MAP_SHARED
#define PROT_READ       0x01
#define PROT_WRITE      0x02
#define PROT_EXEC       0x04
#define PROT_NONE       0x00
#define MAP_SHARED      0x01
#define MAP_PRIVATE     0x02
#define MAP_ANONYMOUS   0x20
#define MAP_ANON        MAP_ANONYMOUS
#define MAP_FAILED      ((void *)-1)
#define MADV_SEQUENTIAL 0
#define MADV_RANDOM     0
#define MADV_WILLNEED   0
#define MADV_DONTNEED   0
#define MADV_DONTFORK   0
#define MADV_DONTDUMP   0
#define MADV_HUGEPAGE   0
#define MADV_MERGEABLE  0
void *mmap(void *addr, size_t len, int prot, int flags, int fd, off_t offset);
int   munmap(void *ptr, size_t len);
int   madvise(void *addr, size_t len, int advice);
#endif // MAP_SHARED

// ── POSIX resource limits ── UCRT64 has no sys/resource.h ──────────────────
#ifndef RLIM_INFINITY
typedef unsigned long rlim_t;
#define RLIM_INFINITY  ((rlim_t)~0UL)
struct rlimit {
    rlim_t rlim_cur;
    rlim_t rlim_max;
};
#define RLIMIT_NOFILE 7
static inline int getrlimit(int resource __maybe_unused, struct rlimit *rlim __maybe_unused) { return 0; }
static inline int setrlimit(int resource __maybe_unused, const struct rlimit *rlim __maybe_unused) { return 0; }
#endif // RLIM_INFINITY

// ── S_ISSOCK ── POSIX socket-type check absent from UCRT64 sys/stat.h ───────
#ifndef S_ISSOCK
#  ifdef S_IFSOCK
#    define S_ISSOCK(m) (((m) & S_IFMT) == S_IFSOCK)
#  else
#    define S_ISSOCK(m) (0)
#  endif
#endif

// ── Unix domain socket types ─────────────────────────────────────────────────
// Windows 10+ SDK provides afunix.h; fall back to a minimal stub otherwise.
#ifndef UNIX_PATH_MAX
#  if defined(__has_include) && __has_include(<afunix.h>)
#    include <afunix.h>
#  else
#    ifndef AF_UNIX
#      define AF_UNIX 1
#    endif
#    define UNIX_PATH_MAX 108
struct sockaddr_un {
    unsigned short sun_family;
    char           sun_path[UNIX_PATH_MAX];
};
#  endif
#endif // UNIX_PATH_MAX

// ── readlink() stub ── UCRT64 has no readlink; callers handle -1 gracefully ─
static inline ssize_t readlink(const char *path __maybe_unused,
                                char *buf __maybe_unused,
                                size_t bufsiz __maybe_unused) {
    errno = ENOSYS;
    return -1;
}

// ── sysconf() / _SC_CLK_TCK ── UCRT64 has no sysconf ─────────────────────────
#ifndef _SC_CLK_TCK
#  define _SC_CLK_TCK 2
static inline long sysconf(int name) {
    if (name == _SC_CLK_TCK) return 100;
    errno = EINVAL;
    return -1;
}
#endif

// ── strcasestr() ── GNU extension absent from UCRT64 ──────────────────────────
static inline char *strcasestr(const char *haystack, const char *needle) {
    if (!needle || !*needle) return (char *)haystack;
    size_t nlen = strlen(needle);
    for (; *haystack; haystack++)
        if (strncasecmp(haystack, needle, nlen) == 0)
            return (char *)haystack;
    return NULL;
}

// ── PTHREAD_STACK_MIN ── absent from MinGW-w64 UCRT64 winpthreads ─────────────
#ifndef PTHREAD_STACK_MIN
#  define PTHREAD_STACK_MIN (64 * 1024)
#endif

// ── nfds_t ── POSIX type absent from UCRT64 ────────────────────────────────────
#ifndef _NFDS_T
#  define _NFDS_T
typedef unsigned long nfds_t;
#endif

// ── poll() ── UCRT64 has no poll(); delegate to WSAPoll via winsock2.h ──────────
// winsock2.h defines struct pollfd == WSAPOLLFD (same layout), so the cast is safe.
// Socket fds on UCRT64 are raw SOCKET/Win32 HANDLE values stored in int variables.
static inline int poll(struct pollfd *fds, nfds_t nfds, int timeout) {
    return WSAPoll((WSAPOLLFD *)fds, (ULONG)nfds, timeout);
}

// ── getsockopt / setsockopt ── Winsock declares char* for optval; POSIX void* ──
// Define wrappers before the macros so the wrapper bodies call raw Winsock.
static inline int nd_getsockopt(int s, int l, int o, void *v, socklen_t *vl) {
    int ilen = (int)*vl;
    int r = getsockopt(s, l, o, (char *)v, &ilen);
    *vl = (socklen_t)ilen;
    return r;
}
static inline int nd_setsockopt(int s, int l, int o, const void *v, socklen_t vl) {
    return setsockopt(s, l, o, (const char *)v, (int)vl);
}
#define getsockopt nd_getsockopt
#define setsockopt nd_setsockopt

// ── fcntl / F_GETFL / F_SETFL / F_GETFD / F_SETFD / O_NONBLOCK / FD_CLOEXEC ──
// UCRT64 has no POSIX fcntl; implement what socket.c needs via Winsock APIs.
// fd values are raw SOCKET / Win32 HANDLE values (not CRT fd indices).
#ifndef F_GETFL
#  define F_GETFL    3
#  define F_SETFL    4
#  define F_GETFD    1
#  define F_SETFD    2
#  define FD_CLOEXEC 1
#  ifndef O_NONBLOCK
#    define O_NONBLOCK 0x0004
#  endif
static inline int fcntl(int fd, int cmd, ...) {
    switch (cmd) {
    case F_GETFL:
    case F_GETFD:
        return 0;
    case F_SETFL: {
        va_list ap; va_start(ap, cmd);
        int flags = va_arg(ap, int); va_end(ap);
        u_long mode = (flags & O_NONBLOCK) ? 1 : 0;
        // zero-extend int → uintptr_t to avoid sign-extension of SOCKET values
        SOCKET s = (SOCKET)(uintptr_t)(unsigned int)fd;
        return (ioctlsocket(s, FIONBIO, &mode) == 0) ? 0 : -1;
    }
    case F_SETFD: {
        va_list ap; va_start(ap, cmd);
        int flags = va_arg(ap, int); va_end(ap);
        HANDLE h = (HANDLE)(uintptr_t)(unsigned int)fd;
        DWORD inherit = (flags & FD_CLOEXEC) ? 0 : HANDLE_FLAG_INHERIT;
        return SetHandleInformation(h, HANDLE_FLAG_INHERIT, inherit) ? 0 : -1;
    }
    default:
        errno = EINVAL;
        return -1;
    }
}
#endif // F_GETFL

// ── struct passwd / getpwuid_r ── pwd.h absent on UCRT64 ────────────────────
// Stub always reports "not found" so callers fall back to numeric UID string.
#ifndef _PASSWD_DEFINED
#define _PASSWD_DEFINED
struct passwd {
    char  *pw_name;
    uid_t  pw_uid;
    gid_t  pw_gid;
};
static inline int getpwuid_r(uid_t uid __maybe_unused,
                              struct passwd *pwd __maybe_unused,
                              char *buf __maybe_unused,
                              size_t buflen __maybe_unused,
                              struct passwd **result) {
    if (result) *result = NULL;
    return ENOENT;
}
#endif

// ── struct group / getgrgid_r ── grp.h absent on UCRT64 ─────────────────────
// Stub always reports "not found" so callers fall back to numeric GID string.
#ifndef _GROUP_DEFINED
#define _GROUP_DEFINED
struct group {
    char  *gr_name;
    gid_t  gr_gid;
};
static inline int getgrgid_r(gid_t gid __maybe_unused,
                              struct group *grp __maybe_unused,
                              char *buf __maybe_unused,
                              size_t buflen __maybe_unused,
                              struct group **result) {
    if (result) *result = NULL;
    return ENOENT;
}
#endif

// ── WIFEXITED / WEXITSTATUS / WIFSIGNALED / WTERMSIG ── sys/wait.h absent ───
// On Windows, processes exit normally (no POSIX signal killing).
// _pclose / waitpid return the exit code directly.
#ifndef WIFEXITED
#define WIFEXITED(status)    (1)
#define WEXITSTATUS(status)  ((status) & 0xFF)
#define WIFSIGNALED(status)  (0)
#define WTERMSIG(status)     (0)
#endif

// ── SIGPIPE / SIGTRAP ── absent from UCRT64 signal.h ─────────────────────────
#ifndef SIGPIPE
#define SIGPIPE  13
#endif
#ifndef SIGTRAP
#define SIGTRAP   5
#endif

// ── pipe() ── POSIX 2-arg version; UCRT64 only has _pipe(fds, size, mode) ───
#ifndef pipe
#define pipe(fds) _pipe((fds), 65536, _O_BINARY)
#endif

// ── kill() stub ── POSIX, absent from UCRT64 ─────────────────────────────────
// Callers in spawn_server_windows.c already follow up with TerminateProcess().
static inline int kill(pid_t pid __maybe_unused, int sig __maybe_unused) {
    errno = ESRCH;
    return -1;
}

// ── struct statfs / statfs() ── absent from UCRT64 ───────────────────────────
// paths.c uses statfs() only to detect procfs/sysfs (Linux-only fs types).
// On Windows those filesystems never exist; returning -1 makes callers skip
// procfs/sysfs-specific logic, which is the correct behaviour.
#ifndef _STATFS_DEFINED
#define _STATFS_DEFINED
struct statfs { long f_type; };
static inline int statfs(const char *path __maybe_unused, struct statfs *buf __maybe_unused) {
    errno = ENOSYS;
    return -1;
}
#endif

// ── d_type / DT_* ── MinGW UCRT64 struct dirent has no d_type member ─────────
// Windows has no inode numbers, so d_ino is always 0.  Mapping d_type to d_ino
// means every entry reports DT_UNKNOWN (0), and callers fall through to the
// stat()-based type check (the DT_UNKNOWN branch is always guarded in paths.c).
#ifndef DT_UNKNOWN
#define DT_UNKNOWN  0
#define DT_DIR      4
#define DT_REG      8
#define DT_LNK     10
#define d_type      d_ino
#endif

// ── strerror_r() ── POSIX XSI variant, absent from UCRT64 ────────────────────
// Windows provides strerror_s() with reversed argument order and returns errno.
static inline int strerror_r(int errnum, char *buf, size_t buflen) {
    return (int)strerror_s(buf, buflen, errnum);
}

// ── syslog stubs ── openlog/closelog/syslog/LOG_PID absent from UCRT64 ────────
// nd_log-to-syslog.c is compiled unconditionally; these become no-ops on Windows
// because the Windows log sink (EventLog / OutputDebugString) is used instead.
#ifndef LOG_PID
#define LOG_PID    0x01
#define LOG_ODELAY 0x04
#define LOG_NDELAY 0x08
static inline void openlog(const char *ident __maybe_unused, int opt __maybe_unused, int fac __maybe_unused) {}
static inline void closelog(void) {}
static inline void syslog(int pri __maybe_unused, const char *fmt __maybe_unused, ...) {}
#endif

// ── htole64 / le64toh / htole32 / le32toh ── absent from UCRT64 (no endian.h) ─
// x86_64 Windows is always little-endian, so host and LE byte-order are identical.
#ifndef htole64
#define htole64(x) (x)
#define le64toh(x) (x)
#define htole32(x) (x)
#define le32toh(x) (x)
#endif

#endif // OS_WINDOWS

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
