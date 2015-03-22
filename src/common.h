#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#define abs(x) ((x < 0)? -x : x)
#define usecdiff(now, last) (((((now)->tv_sec * 1000000ULL) + (now)->tv_usec) - (((last)->tv_sec * 1000000ULL) + (last)->tv_usec)))

extern unsigned long simple_hash(const char *name);
extern void strreverse(char* begin, char* end);
extern char *mystrsep(char **ptr, char *s);
extern char *qstrsep(char **ptr);
extern char *trim(char *s);

extern void *mymmap(const char *filename, unsigned long size, int flags);
extern int savememory(const char *filename, void *mem, unsigned long size);

extern int fd_is_valid(int fd);

#endif /* NETDATA_COMMON_H */
