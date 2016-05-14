#include <stdio.h>
#include <stdarg.h>
#include <time.h>

#ifndef NETDATA_LOG_H
#define NETDATA_LOG_H 1

#define D_WEB_BUFFER 		0x00000001
#define D_WEB_CLIENT 		0x00000002
#define D_LISTENER   		0x00000004
#define D_WEB_DATA   		0x00000008
#define D_OPTIONS    		0x00000010
#define D_PROCNETDEV_LOOP   0x00000020
#define D_RRD_STATS 		0x00000040
#define D_WEB_CLIENT_ACCESS	0x00000080
#define D_TC_LOOP           0x00000100
#define D_DEFLATE           0x00000200
#define D_CONFIG            0x00000400
#define D_PLUGINSD          0x00000800
#define D_CHILDS            0x00001000
#define D_EXIT              0x00002000
#define D_CHECKS            0x00004000
#define D_NFACCT_LOOP		0x00008000
#define D_PROCFILE			0x00010000
#define D_RRD_CALLS			0x00020000
#define D_DICTIONARY		0x00040000
#define D_MEMORY			0x00080000
#define D_CGROUP            0x00100000
#define D_REGISTRY			0x00200000

//#define DEBUG (D_WEB_CLIENT_ACCESS|D_LISTENER|D_RRD_STATS)
//#define DEBUG 0xffffffff
#define DEBUG (0)

extern unsigned long long debug_flags;

extern const char *program_name;

extern int silent;

extern int access_fd;
extern FILE *stdaccess;

extern int access_log_syslog;
extern int error_log_syslog;
extern int output_log_syslog;

extern time_t error_log_throttle_period;
extern unsigned long error_log_errors_per_period;
extern int error_log_limit(int reset);

#define error_log_limit_reset() do { error_log_limit(1); } while(0)

#define debug(type, args...) do { if(unlikely(!silent && (debug_flags & type))) debug_int(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#define info(args...)    info_int(__FILE__, __FUNCTION__, __LINE__, ##args)
#define infoerr(args...) error_int("INFO", __FILE__, __FUNCTION__, __LINE__, ##args)
#define error(args...)   error_int("ERROR", __FILE__, __FUNCTION__, __LINE__, ##args)
#define fatal(args...)   fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args)

extern void log_date(FILE *out);
extern void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... );
extern void info_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... );
extern void error_int( const char *prefix, const char *file, const char *function, const unsigned long line, const char *fmt, ... );
extern void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) __attribute__ ((noreturn));
extern void log_access( const char *fmt, ... );

#endif /* NETDATA_LOG_H */
