#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <strings.h>
#include <unistd.h>

#include "global_statistics.h"
#include "common.h"
#include "appconfig.h"
#include "log.h"
#include "rrd.h"
#include "plugin_idlejitter.h"

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

void *cpuidlejitter_main(void *ptr)
{
	if(ptr) { ; }

	info("CPU Idle Jitter thread created with task id %d", gettid());

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	int sleep_ms = (int) config_get_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
	if(sleep_ms <= 0) {
		config_set_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
		sleep_ms = CPU_IDLEJITTER_SLEEP_TIME_MS;
	}

	RRDSET *st = rrdset_find("system.idlejitter");
	if(!st) {
		st = rrdset_create("system", "idlejitter", NULL, "processes", NULL, "CPU Idle Jitter", "microseconds lost/s", 9999, rrd_update_every, RRDSET_TYPE_LINE);
		rrddim_add(st, "jitter", NULL, 1, 1, RRDDIM_ABSOLUTE);
	}

	struct timeval before, after;
	unsigned long long counter;
	for(counter = 0; 1 ;counter++) {
		unsigned long long usec = 0, susec = 0;

		while(susec < (rrd_update_every * 1000000ULL)) {

			gettimeofday(&before, NULL);
			usleep(sleep_ms * 1000);
			gettimeofday(&after, NULL);

			// calculate the time it took for a full loop
			usec = usecdiff(&after, &before);
			susec += usec;
		}
		usec -= (sleep_ms * 1000);

		if(counter) rrdset_next(st);
		rrddim_set(st, "jitter", usec);
		rrdset_done(st);
	}

	pthread_exit(NULL);
	return NULL;
}

