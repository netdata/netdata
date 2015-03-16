#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <strings.h>
#include <unistd.h>

#include "global_statistics.h"
#include "common.h"
#include "config.h"
#include "log.h"
#include "rrd.h"
#include "plugin_idlejitter.h"

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

void *cpuidlejitter_main(void *ptr)
{
	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	int sleep_ms = config_get_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
	if(sleep_ms <= 0) {
		config_set_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
		sleep_ms = CPU_IDLEJITTER_SLEEP_TIME_MS;
	}

	RRD_STATS *st = rrd_stats_find("system.idlejitter");
	if(!st) {
		st = rrd_stats_create("system", "idlejitter", NULL, "cpu", "CPU Idle Jitter", "microseconds lost/s", 9999, update_every, CHART_TYPE_LINE);
		rrd_stats_dimension_add(st, "jitter", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
	}

	struct timeval before, after;
	unsigned long long counter;
	for(counter = 0; 1 ;counter++) {
		unsigned long long usec = 0, susec = 0;

		while(susec < (update_every * 1000000ULL)) {

			gettimeofday(&before, NULL);
			usleep(sleep_ms * 1000);
			gettimeofday(&after, NULL);

			// calculate the time it took for a full loop
			usec = usecdiff(&after, &before);
			susec += usec;
		}
		usec -= (sleep_ms * 1000);

		if(counter) rrd_stats_next_usec(st, susec);
		rrd_stats_dimension_set(st, "jitter", usec);
		rrd_stats_done(st);
	}

	return NULL;
}

