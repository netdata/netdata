#ifndef NETDATA_LOG_H
#define NETDATA_LOG_H 1

/**
 * @file log.h
 * @brief Logging
 *
 * This file holds the API used for logging.
 *
 * We define five log level:
 * - __debug__: Fine grained information what the programm currently does. Useful for debugging. For example logging function calls.
 * - __info__: Information about the state of the application. For example start of plugins.
 * - __error__: Information about error events that might still allow the application to continue running.
 * - __fatal__: Information about errors that lead to application shutdown.
 * - __access__: Information about access from clients to the application.
 *
 * The debug output is split into different types. By default they are all disabled.
 *
 * We support three log destinations.
 * - __stdout__: This is used by log level _debug_. By default we use the systems stdout.
 *   Set `stdout_filename` and call open_all_log_files() to switch logging to `stdout_filename`.
 * - __stderr__: This is used by log level _info_, _error_ and _fatal. By default we use the systems stderr.
 *   Set `stderr_filename` and call open_all_log_files() to switch logging to `stderr_filename`.
 * - __stdaccess__: This is used by log leve _access_. By default we will not log this.
 *   Set `stdaccess_filename` and call open_all_log_files() to wwitch logging to `stdaccess_filename`.
 *
 * By default we additional log every destination to syslog.
 * To only log to syslog set `std*_filename` to "syslog" and call open_all_log_files.
 * To immediately disable logging for one location to syslog set access_log_syslog, error_log_syslog, output_log_syslog.
 * To change log destinations twice use reopen_all_log_files().
 */

#define D_WEB_BUFFER        0x00000001 ///< Debug web buffer.
#define D_WEB_CLIENT        0x00000002 ///< Debug web client.
#define D_LISTENER          0x00000004 ///< Debug web server listening to ip, port and socket.
#define D_WEB_DATA          0x00000008 ///< Debug web server data sent and recieved.
#define D_OPTIONS           0x00000010 ///< Debug command line and configuration file loading. @see `D_CONFIG`
#define D_PROCNETDEV_LOOP   0x00000020 ///< Debug proc or other system data collection loop.
#define D_RRD_STATS         0x00000040 ///< Debug round robin database statistics.
#define D_WEB_CLIENT_ACCESS 0x00000080 ///< Debug web client access.
#define D_TC_LOOP           0x00000100 ///< Debug `tc` plugin loop
#define D_DEFLATE           0x00000200 ///< Debug web client deflate.
#define D_CONFIG            0x00000400 ///< Debug applicaton configuration API.
#define D_PLUGINSD          0x00000800 ///< Debug external plugin daemon.
#define D_CHILDS            0x00001000 ///< Debug creation of child processes.
#define D_EXIT              0x00002000 ///< Debug application exit.
#define D_CHECKS            0x00004000 ///< \deprecated Currently not used. 
#define D_NFACCT_LOOP       0x00008000 ///< Debug NFACCT plugin loop.
#define D_PROCFILE          0x00010000 ///< Debug parsing of procfiles.
#define D_RRD_CALLS         0x00020000 ///< Debug round robin database API.
#define D_DICTIONARY        0x00040000 ///< Debug usage of dictionary API.
#define D_MEMORY            0x00080000 ///< \deprecated Currently not used.  
#define D_CGROUP            0x00100000 ///< Debug cgroups collection.
#define D_REGISTRY          0x00200000 ///< Debug Registry usage.
#define D_VARIABLES         0x00400000 ///< Debug health variables. @see D_HEALTH
#define D_HEALTH            0x00800000 ///< Debug health monitoring. @see D_VARIABLES
#define D_CONNECT_TO        0x01000000 ///< Debug socket connections.
#define D_SYSTEM            0x80000000 ///< Debug daemon and system.

//#define DEBUG (D_WEB_CLIENT_ACCESS|D_LISTENER|D_RRD_STATS)
//#define DEBUG 0xffffffff
#define DEBUG (0)  ///< Default debugging. Debug nothing.

extern unsigned long long debug_flags; ///< Global debugging setting (D_*).

extern const char *program_name; ///< Name of the program

extern int stdaccess_fd; ///< File descriptor of `stdaccess_filename`.
extern FILE *stdaccess;  ///< FILE `stdaccess_filename`.

extern const char *stdaccess_filename; ///< Filename to log level _access_.
extern const char *stderr_filename;    ///< Filename to log level _info_, _error_ and _fatal_.
extern const char *stdout_filename;    ///< Filename to log level _debug_.

extern int access_log_syslog; ///< boolean enables additional logging level _access_ to syslog.
extern int error_log_syslog;  ///< boolean enables additional logging levels _info_, _error_ and fatal_ to syslog.
extern int output_log_syslog; ///< boolean enables additional logging level _debug_ to syslog.

extern time_t error_log_throttle_period;          ///< ktsaou: Your help needed.
extern time_t error_log_throttle_period_backup;   ///< ktsaou: Your help needed.
extern unsigned long error_log_errors_per_period; ///< ktsaou: Your help needed.
/**
 * ktsaou: Your help needed.
 *
 * @param reset boolean
 * @return ktsaou: Your help needed.
 */
extern int error_log_limit(int reset);

/**
 * Open log files.
 */
extern void open_all_log_files();
/**
 * Reopen all log files.
 */
extern void reopen_all_log_files();

/**
 * ktsaou: Your help needed.
 */
#define error_log_limit_reset() do { error_log_throttle_period = error_log_throttle_period_backup; error_log_limit(1); } while(0)
/**
 * ktsaou: Your help needed.
 */
#define error_log_limit_unlimited() do { error_log_throttle_period = 0; } while(0)

/**
 * Log with level debug.
 *
 * @param type Debug type (D_*).
 * @param args Format string and arguments.
 */
#define debug(type, args...) do { if(unlikely(debug_flags & type)) debug_int(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
/**
 * Log with level info.
 *
 * @param args Format string and arguments.
 */
#define info(args...)    info_int(__FILE__, __FUNCTION__, __LINE__, ##args)
/**
 * Log with level infoerr.
 *
 * \deprecated Never used.
 *
 * @param args Format string and arguments.
 */
#define infoerr(args...) error_int("INFO", __FILE__, __FUNCTION__, __LINE__, ##args)
/**
 * Log with level error.
 *
 * @param args Format string and arguments.
 */
#define error(args...)   error_int("ERROR", __FILE__, __FUNCTION__, __LINE__, ##args)
/**
 * Log with level fatal.
 *
 * This calls netdata_cleanup_and_exit(1).
 *
 * @param args Format string and arguments.
 */
#define fatal(args...)   fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args)

/**
 * Print current date and time to FILE `out`.
 *
 * The date is formatted %Y-%m-%d %H:%M:%S.
 *
 * \deprecated This will be if todo is fixed.
 * \todo this should print the date in a buffer the way it is now, logs from multiple threads may be multiplexed.
 *
 * @param out FILE to print date to.
 */
extern void log_date(FILE *out);
/**
 * Log with level debug.
 *
 * Use debug() instead.
 * @see debug()
 *
 * @param file from where we log
 * @param function from where we log
 * @param line number from where we log
 * @param fmt Format string.
 */
extern void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) PRINTFLIKE(4, 5);
/**
 * Log with level info.
 *
 * Use info() instead.
 * @see info()
 *
 * @param file from where we log
 * @param function from where we log
 * @param line number from where we log
 * @param fmt Format string.
 */
extern void info_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) PRINTFLIKE(4, 5);
/**
 * Log with level error.
 *
 * Use error() instead.
 * @see error()
 *
 * @param prefix of error level
 * @param file from where we log
 * @param function from where we log
 * @param line number from where we log
 * @param fmt Format string.
 */
extern void error_int( const char *prefix, const char *file, const char *function, const unsigned long line, const char *fmt, ... ) PRINTFLIKE(5, 6);
/**
 * Log with level fatal.
 *
 * Use fatal() instead.
 * @see fatal()
 *
 * @param file from where we log
 * @param function from where we log
 * @param line number from where we log
 * @param fmt Format string.
 */
extern void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) NORETURN PRINTFLIKE(4, 5);
/**
 * Log to access log file.
 *
 * @param fmt Format string.
 */
extern void log_access( const char *fmt, ... ) PRINTFLIKE(1, 2);

#endif /* NETDATA_LOG_H */
