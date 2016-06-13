#include <stdarg.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <stdio.h>

#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#if defined(HAVE_INTTYPES_H)
#include <inttypes.h>
#elif defined(HAVE_STDINT_H)
#include <stdint.h>
#endif
#include <sys/types.h>
#include <unistd.h>

#define abs(x) ((x < 0)? -x : x)
#define usecdiff(now, last) (((((now)->tv_sec * 1000000ULL) + (now)->tv_usec) - (((last)->tv_sec * 1000000ULL) + (last)->tv_usec)))

extern void netdata_fix_chart_id(char *s);
extern void netdata_fix_chart_name(char *s);

extern uint32_t simple_hash(const char *name);
extern uint32_t simple_uhash(const char *name);

extern void strreverse(char* begin, char* end);
extern char *mystrsep(char **ptr, char *s);
extern char *trim(char *s);

extern char *strncpyz(char *dst, const char *src, size_t n);
extern int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
extern int  snprintfz(char *dst, size_t n, const char *fmt, ...);

extern void *mymmap(const char *filename, size_t size, int flags, int ksm);
extern int savememory(const char *filename, void *mem, size_t size);

extern int fd_is_valid(int fd);

extern char *global_host_prefix;
extern int enable_ksm;

/* Number of ticks per second */
extern unsigned int hz;
extern void get_HZ(void);

extern pid_t gettid(void);

extern unsigned long long timems(void);

extern char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

/* fix for alpine linux */
#ifndef RUSAGE_THREAD
#ifdef RUSAGE_CHILDREN
#define RUSAGE_THREAD RUSAGE_CHILDREN
#endif
#endif

#endif /* NETDATA_COMMON_H */
