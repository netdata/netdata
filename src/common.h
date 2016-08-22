#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#ifdef HAVE_CONFIG_H
#include <config.h>
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
#include <netinet/in.h>
#include <netinet/tcp.h>

#include <dirent.h>
#include <fcntl.h>
#include <getopt.h>
#include <grp.h>
#include <pwd.h>
#include <locale.h>
#include <malloc.h>
#include <netdb.h>
#include <poll.h>
#include <signal.h>
#include <syslog.h>
#include <sys/mman.h>
#include <sys/prctl.h>
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

#ifndef __ATOMIC_SEQ_CST
#define NETDATA_NO_ATOMIC_INSTRUCTIONS 1
#endif

#ifdef __GNUC__
#define GCC_VERSION (__GNUC__ * 10000 \
                               + __GNUC_MINOR__ * 100 \
                               + __GNUC_PATCHLEVEL__)

#if __x86_64__ || __ppc64__
#define ENVIRONMENT64
#else
#define ENVIRONMENT32
#endif

#else // !__GNUC__
#define NETDATA_NO_ATOMIC_INSTRUCTIONS 1
#define ENVIRONMENT32
#endif // __GNUC__

#include "avl.h"
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
#include "plugin_proc.h"
#include "plugin_tc.h"
#include "plugins_d.h"

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

#ifdef abs
#undef abs
#endif
#define abs(x) ((x < 0)? -x : x)

extern unsigned long long usec_dt(struct timeval *now, struct timeval *old);
extern unsigned long long timeval_usec(struct timeval *tv);

// #define usec_dt(now, last) (((((now)->tv_sec * 1000000ULL) + (now)->tv_usec) - (((last)->tv_sec * 1000000ULL) + (last)->tv_usec)))

extern void netdata_fix_chart_id(char *s);
extern void netdata_fix_chart_name(char *s);

extern uint32_t simple_hash(const char *name);
extern uint32_t simple_uhash(const char *name);

extern void strreverse(char* begin, char* end);
extern char *mystrsep(char **ptr, char *s);
extern char *trim(char *s);

extern char *strncpyz(char *dst, const char *src, size_t n);
extern int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
extern int  snprintfz(char *dst, size_t n, const char *fmt, ...) __attribute__ (( format (printf, 3, 4)));

// memory allocation functions that handle failures
extern char *strdupz(const char *s);
extern void *callocz(size_t nmemb, size_t size);
extern void *mallocz(size_t size);
extern void freez(void *ptr);
extern void *reallocz(void *ptr, size_t size);

extern void *mymmap(const char *filename, size_t size, int flags, int ksm);
extern int savememory(const char *filename, void *mem, size_t size);

extern int fd_is_valid(int fd);

extern char *global_host_prefix;
extern int enable_ksm;

/* Number of ticks per second */
extern unsigned int hz;
extern void get_HZ(void);

extern pid_t gettid(void);

extern unsigned long long time_usec(void);
extern int sleep_usec(unsigned long long usec);

extern char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#endif /* NETDATA_COMMON_H */
