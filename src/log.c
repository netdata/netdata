// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

int web_server_is_multithreaded = 1;

const char *program_name = "";
uint64_t debug_flags = DEBUG;

int access_log_syslog = 1;
int error_log_syslog = 1;
int output_log_syslog = 1;  // debug log

int stdaccess_fd = -1;
FILE *stdaccess = NULL;

const char *stdaccess_filename = NULL;
const char *stderr_filename = NULL;
const char *stdout_filename = NULL;

void syslog_init(void) {
    static int i = 0;

    if(!i) {
        openlog(program_name, LOG_PID, LOG_DAEMON);
        i = 1;
    }
}

#define LOG_DATE_LENGTH 26

static inline void log_date(char *buffer, size_t len) {
    if(unlikely(!buffer || !len))
        return;

    time_t t;
    struct tm *tmp, tmbuf;

    t = now_realtime_sec();
    tmp = localtime_r(&t, &tmbuf);

    if (tmp == NULL) {
        buffer[0] = '\0';
        return;
    }

    if (unlikely(strftime(buffer, len, "%Y-%m-%d %H:%M:%S", tmp) == 0))
        buffer[0] = '\0';

    buffer[len - 1] = '\0';
}

static netdata_mutex_t log_mutex = NETDATA_MUTEX_INITIALIZER;
static inline void log_lock() {
    netdata_mutex_lock(&log_mutex);
}
static inline void log_unlock() {
    netdata_mutex_unlock(&log_mutex);
}

static FILE *open_log_file(int fd, FILE *fp, const char *filename, int *enabled_syslog, int is_stdaccess, int *fd_ptr) {
    int f, devnull = 0;

    if(!filename || !*filename || !strcmp(filename, "none") ||  !strcmp(filename, "/dev/null")) {
        filename = "/dev/null";
        devnull = 1;
    }

    if(!strcmp(filename, "syslog")) {
        filename = "/dev/null";
        devnull = 1;
        syslog_init();
        if(enabled_syslog) *enabled_syslog = 1;
    }
    else if(enabled_syslog) *enabled_syslog = 0;

    // don't do anything if the user is willing
    // to have the standard one
    if(!strcmp(filename, "system")) {
        if(fd != -1 && !is_stdaccess) {
            if(fd_ptr) *fd_ptr = fd;
            return fp;
        }

        filename = "stderr";
    }

    if(!strcmp(filename, "stdout"))
        f = STDOUT_FILENO;

    else if(!strcmp(filename, "stderr"))
        f = STDERR_FILENO;

    else {
        f = open(filename, O_WRONLY | O_APPEND | O_CREAT, 0664);
        if(f == -1) {
            error("Cannot open file '%s'. Leaving %d to its default.", filename, fd);
            if(fd_ptr) *fd_ptr = fd;
            return fp;
        }
    }

    // if there is a level-2 file pointer
    // flush it before switching the level-1 fds
    if(fp)
        fflush(fp);

    if(devnull && is_stdaccess) {
        fd = -1;
        fp = NULL;
    }

    if(fd != f && fd != -1) {
        // it automatically closes
        int t = dup2(f, fd);
        if (t == -1) {
            error("Cannot dup2() new fd %d to old fd %d for '%s'", f, fd, filename);
            close(f);
            if(fd_ptr) *fd_ptr = fd;
            return fp;
        }
        // info("dup2() new fd %d to old fd %d for '%s'", f, fd, filename);
        close(f);
    }
    else fd = f;

    if(!fp) {
        fp = fdopen(fd, "a");
        if (!fp)
            error("Cannot fdopen() fd %d ('%s')", fd, filename);
        else {
            if (setvbuf(fp, NULL, _IOLBF, 0) != 0)
                error("Cannot set line buffering on fd %d ('%s')", fd, filename);
        }
    }

    if(fd_ptr) *fd_ptr = fd;
    return fp;
}

void reopen_all_log_files() {
    if(stdout_filename)
        open_log_file(STDOUT_FILENO, stdout, stdout_filename, &output_log_syslog, 0, NULL);

    if(stderr_filename)
        open_log_file(STDERR_FILENO, stderr, stderr_filename, &error_log_syslog, 0, NULL);

    if(stdaccess_filename)
         stdaccess = open_log_file(stdaccess_fd, stdaccess, stdaccess_filename, &access_log_syslog, 1, &stdaccess_fd);
}

void open_all_log_files() {
    // disable stdin
    open_log_file(STDIN_FILENO, stdin, "/dev/null", NULL, 0, NULL);

    open_log_file(STDOUT_FILENO, stdout, stdout_filename, &output_log_syslog, 0, NULL);
    open_log_file(STDERR_FILENO, stderr, stderr_filename, &error_log_syslog, 0, NULL);
    stdaccess = open_log_file(stdaccess_fd, stdaccess, stdaccess_filename, &access_log_syslog, 1, &stdaccess_fd);
}

// ----------------------------------------------------------------------------
// error log throttling

time_t error_log_throttle_period = 1200;
unsigned long error_log_errors_per_period = 200;
unsigned long error_log_errors_per_period_backup = 0;

int error_log_limit(int reset) {
    static time_t start = 0;
    static unsigned long counter = 0, prevented = 0;

    // fprintf(stderr, "FLOOD: counter=%lu, allowed=%lu, backup=%lu, period=%llu\n", counter, error_log_errors_per_period, error_log_errors_per_period_backup, (unsigned long long)error_log_throttle_period);

    // do not throttle if the period is 0
    if(error_log_throttle_period == 0)
        return 0;

    // prevent all logs if the errors per period is 0
    if(error_log_errors_per_period == 0)
#ifdef NETDATA_INTERNAL_CHECKS
        return 0;
#else
        return 1;
#endif

    time_t now = now_monotonic_sec();
    if(!start) start = now;

    if(reset) {
        if(prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH);
            fprintf(stderr, "%s: %s LOG FLOOD PROTECTION reset for process '%s' (prevented %lu logs in the last %ld seconds).\n"
                    , date
                    , program_name
                    , program_name
                    , prevented
                    , now - start
            );
        }

        start = now;
        counter = 0;
        prevented = 0;
    }

    // detect if we log too much
    counter++;

    if(now - start > error_log_throttle_period) {
        if(prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH);
            fprintf(stderr, "%s: %s LOG FLOOD PROTECTION resuming logging from process '%s' (prevented %lu logs in the last %ld seconds).\n"
                    , date
                    , program_name
                    , program_name
                    , prevented
                    , error_log_throttle_period
            );
        }

        // restart the period accounting
        start = now;
        counter = 1;
        prevented = 0;

        // log this error
        return 0;
    }

    if(counter > error_log_errors_per_period) {
        if(!prevented) {
            char date[LOG_DATE_LENGTH];
            log_date(date, LOG_DATE_LENGTH);
            fprintf(stderr, "%s: %s LOG FLOOD PROTECTION too many logs (%lu logs in %ld seconds, threshold is set to %lu logs in %ld seconds). Preventing more logs from process '%s' for %ld seconds.\n"
                    , date
                    , program_name
                    , counter
                    , now - start
                    , error_log_errors_per_period
                    , error_log_throttle_period
                    , program_name
                    , start + error_log_throttle_period - now);
        }

        prevented++;

        // prevent logging this error
#ifdef NETDATA_INTERNAL_CHECKS
        return 0;
#else
        return 1;
#endif
    }

    return 0;
}

// ----------------------------------------------------------------------------
// debug log

void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    va_list args;

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH);

    va_start( args, fmt );
    printf("%s: %s DEBUG : %s : (%04lu@%-10.10s:%-15.15s): ", date, program_name, netdata_thread_tag(), line, file, function);
    vprintf(fmt, args);
    va_end( args );
    putchar('\n');

    if(output_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_ERR,  fmt, args );
        va_end( args );
    }

    fflush(stdout);
}

// ----------------------------------------------------------------------------
// info log

void info_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
    va_list args;

    // prevent logging too much
    if(error_log_limit(0)) return;

    if(error_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_INFO,  fmt, args );
        va_end( args );
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH);

    log_lock();

    va_start( args, fmt );
    if(debug_flags) fprintf(stderr, "%s: %s INFO  : %s : (%04lu@%-10.10s:%-15.15s): ", date, program_name, netdata_thread_tag(), line, file, function);
    else            fprintf(stderr, "%s: %s INFO  : %s : ", date, program_name, netdata_thread_tag());
    vfprintf( stderr, fmt, args );
    va_end( args );

    fputc('\n', stderr);

    log_unlock();
}

// ----------------------------------------------------------------------------
// error log

#if defined(STRERROR_R_CHAR_P)
// GLIBC version of strerror_r
static const char *strerror_result(const char *a, const char *b) { (void)b; return a; }
#elif defined(HAVE_STRERROR_R)
// POSIX version of strerror_r
static const char *strerror_result(int a, const char *b) { (void)a; return b; }
#elif defined(HAVE_C__GENERIC)

// what a trick!
// http://stackoverflow.com/questions/479207/function-overloading-in-c
static const char *strerror_result_int(int a, const char *b) { (void)a; return b; }
static const char *strerror_result_string(const char *a, const char *b) { (void)b; return a; }

#define strerror_result(a, b) _Generic((a), \
    int: strerror_result_int, \
    char *: strerror_result_string \
    )(a, b)

#else
#error "cannot detect the format of function strerror_r()"
#endif

void error_int( const char *prefix, const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    // save a copy of errno - just in case this function generates a new error
    int __errno = errno;

    va_list args;

    // prevent logging too much
    if(error_log_limit(0)) return;

    if(error_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_ERR,  fmt, args );
        va_end( args );
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH);

    log_lock();

    va_start( args, fmt );
    if(debug_flags) fprintf(stderr, "%s: %s %-5.5s : %s : (%04lu@%-10.10s:%-15.15s): ", date, program_name, prefix, netdata_thread_tag(), line, file, function);
    else            fprintf(stderr, "%s: %s %-5.5s : %s : ", date, program_name, prefix, netdata_thread_tag());
    vfprintf( stderr, fmt, args );
    va_end( args );

    if(__errno) {
        char buf[1024];
        fprintf(stderr, " (errno %d, %s)\n", __errno, strerror_result(strerror_r(__errno, buf, 1023), buf));
        errno = 0;
    }
    else
        fputc('\n', stderr);

    log_unlock();
}

void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    va_list args;

    if(error_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_CRIT,  fmt, args );
        va_end( args );
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH);

    log_lock();

    va_start( args, fmt );
    if(debug_flags) fprintf(stderr, "%s: %s FATAL : %s : (%04lu@%-10.10s:%-15.15s): ", date, program_name, netdata_thread_tag(), line, file, function);
    else            fprintf(stderr, "%s: %s FATAL : %s :", date, program_name, netdata_thread_tag());
    vfprintf( stderr, fmt, args );
    va_end( args );

    perror(" # ");
    fputc('\n', stderr);

    log_unlock();

    netdata_cleanup_and_exit(1);
}

// ----------------------------------------------------------------------------
// access log

void log_access( const char *fmt, ... ) {
    va_list args;

    if(access_log_syslog) {
        va_start( args, fmt );
        vsyslog(LOG_INFO,  fmt, args );
        va_end( args );
    }

    if(stdaccess) {
        static netdata_mutex_t access_mutex = NETDATA_MUTEX_INITIALIZER;

        if(web_server_is_multithreaded)
            netdata_mutex_lock(&access_mutex);

        char date[LOG_DATE_LENGTH];
        log_date(date, LOG_DATE_LENGTH);
        fprintf(stdaccess, "%s: ", date);

        va_start( args, fmt );
        vfprintf( stdaccess, fmt, args );
        va_end( args );
        fputc('\n', stdaccess);

        if(web_server_is_multithreaded)
            netdata_mutex_unlock(&access_mutex);
    }
}
