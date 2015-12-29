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

extern uint32_t simple_hash(const char *name);
extern void strreverse(char* begin, char* end);
extern char *mystrsep(char **ptr, char *s);
extern char *trim(char *s);

extern void *mymmap(const char *filename, size_t size, int flags, int ksm);
extern int savememory(const char *filename, void *mem, unsigned long size);

extern int fd_is_valid(int fd);

extern char *global_host_prefix;
extern int enable_ksm;

/* Number of ticks per second */
#define HZ        myhz
extern unsigned int hz;
extern void get_HZ(void);

extern pid_t gettid(void);

#endif /* NETDATA_COMMON_H */
