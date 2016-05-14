#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <time.h>
#include <syslog.h>
#include <errno.h>
#include <string.h>
#include <unistd.h>
#include <stdlib.h>

#include "log.h"
#include "common.h"


// ----------------------------------------------------------------------------
// LOG

const char *program_name = "";
unsigned long long debug_flags = DEBUG;

int silent = 0;

int access_fd = -1;
FILE *stdaccess = NULL;

int access_log_syslog = 1;
int error_log_syslog = 1;
int output_log_syslog = 1;	// debug log

time_t error_log_throttle_period = 1200;
unsigned long error_log_errors_per_period = 200;

int error_log_limit(int reset) {
	static time_t start = 0;
	static unsigned long counter = 0, prevented = 0;

	// do not throttle if the period is 0
	if(error_log_throttle_period == 0)
		return 0;

	// prevent all logs if the errors per period is 0
	if(error_log_errors_per_period == 0)
		return 1;

	time_t now = time(NULL);
	if(!start) start = now;

	if(reset) {
		if(prevented) {
			log_date(stderr);
			fprintf(stderr, "%s: Resetting logging for process '%s' (prevented %lu logs in the last %ld seconds).\n"
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
			log_date(stderr);
			fprintf(stderr, "%s: Resuming logging from process '%s' (prevented %lu logs in the last %ld seconds).\n"
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
			log_date(stderr);
			fprintf(stderr, "%s: Too many logs (%lu logs in %ld seconds, threshold is set to %lu logs in %ld seconds). Preventing more logs from process '%s' for %ld seconds.\n"
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
		return 1;
	}

	return 0;
}

void log_date(FILE *out)
{
		char outstr[200];
		time_t t;
		struct tm *tmp, tmbuf;

		t = time(NULL);
		tmp = localtime_r(&t, &tmbuf);

		if (tmp == NULL) return;
		if (strftime(outstr, sizeof(outstr), "%y-%m-%d %H:%M:%S", tmp) == 0) return;

		fprintf(out, "%s: ", outstr);
}

void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	va_list args;

	log_date(stdout);
	va_start( args, fmt );
	fprintf(stdout, "DEBUG (%04lu@%-10.10s:%-15.15s): %s: ", line, file, function, program_name);
	vfprintf( stdout, fmt, args );
	va_end( args );
	fprintf(stdout, "\n");
	// fflush( stdout );

	if(output_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_ERR,  fmt, args );
		va_end( args );
	}
}

void info_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	va_list args;

	// prevent logging too much
	if(error_log_limit(0)) return;

	log_date(stderr);

	va_start( args, fmt );
	if(debug_flags) fprintf(stderr, "INFO (%04lu@%-10.10s:%-15.15s): %s: ", line, file, function, program_name);
	else            fprintf(stderr, "INFO: %s: ", program_name);
	vfprintf( stderr, fmt, args );
	va_end( args );

	fprintf(stderr, "\n");

	if(error_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_INFO,  fmt, args );
		va_end( args );
	}
}

void error_int( const char *prefix, const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	va_list args;

	// prevent logging too much
	if(error_log_limit(0)) return;

	log_date(stderr);

	va_start( args, fmt );
	if(debug_flags) fprintf(stderr, "%s (%04lu@%-10.10s:%-15.15s): %s: ", prefix, line, file, function, program_name);
	else            fprintf(stderr, "%s: %s: ", prefix, program_name);
	vfprintf( stderr, fmt, args );
	va_end( args );

	if(errno) {
		char buf[200];
		char *s = strerror_r(errno, buf, 200);
		fprintf(stderr, " (errno %d, %s)\n", errno, s);
		errno = 0;
	}
	else fprintf(stderr, "\n");

	if(error_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_ERR,  fmt, args );
		va_end( args );
	}
}

void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	va_list args;

	log_date(stderr);

	va_start( args, fmt );
	if(debug_flags) fprintf(stderr, "FATAL (%04lu@%-10.10s:%-15.15s): %s: ", line, file, function, program_name);
	else            fprintf(stderr, "FATAL: %s: ", program_name);
	vfprintf( stderr, fmt, args );
	va_end( args );

	perror(" # ");
	fprintf(stderr, "\n");

	if(error_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_CRIT,  fmt, args );
		va_end( args );
	}

	exit(1);
}

void log_access( const char *fmt, ... )
{
	va_list args;

	if(stdaccess) {
		log_date(stdaccess);

		va_start( args, fmt );
		vfprintf( stdaccess, fmt, args );
		va_end( args );
		fprintf( stdaccess, "\n");
		// fflush( stdaccess );
	}

	if(access_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_INFO,  fmt, args );
		va_end( args );
	}
}

