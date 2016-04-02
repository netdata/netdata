#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <strings.h>
#include <unistd.h>

#include "common.h"
#include "appconfig.h"
#include "log.h"
#include "rrd.h"
#include "plugin_checks.h"

void *checks_main(void *ptr)
{
	if(ptr) { ; }

	info("CHECKS thread created with task id %d", gettid());

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	unsigned long long usec = 0, susec = rrd_update_every * 1000000ULL, loop_usec = 0, total_susec = 0;
	struct timeval now, last, loop;

	RRDSET *check1, *check2, *check3, *apps_cpu = NULL;

	check1 = rrdset_create("netdata", "check1", NULL, "netdata", NULL, "Caller gives microseconds", "a million !", 99999, rrd_update_every, RRDSET_TYPE_LINE);
	rrddim_add(check1, "absolute", NULL, -1, 1, RRDDIM_ABSOLUTE);
	rrddim_add(check1, "incremental", NULL, 1, 1, RRDDIM_INCREMENTAL);

	check2 = rrdset_create("netdata", "check2", NULL, "netdata", NULL, "Netdata calcs microseconds", "a million !", 99999, rrd_update_every, RRDSET_TYPE_LINE);
	rrddim_add(check2, "absolute", NULL, -1, 1, RRDDIM_ABSOLUTE);
	rrddim_add(check2, "incremental", NULL, 1, 1, RRDDIM_INCREMENTAL);

	check3 = rrdset_create("netdata", "checkdt", NULL, "netdata", NULL, "Clock difference", "microseconds diff", 99999, rrd_update_every, RRDSET_TYPE_LINE);
	rrddim_add(check3, "caller", NULL, 1, 1, RRDDIM_ABSOLUTE);
	rrddim_add(check3, "netdata", NULL, 1, 1, RRDDIM_ABSOLUTE);
	rrddim_add(check3, "apps.plugin", NULL, 1, 1, RRDDIM_ABSOLUTE);

	gettimeofday(&last, NULL);
	while(1) {
		usleep(susec);

		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&now, NULL);
		loop_usec = usecdiff(&now, &last);
		usec = loop_usec - susec;
		debug(D_PROCNETDEV_LOOP, "CHECK: last loop took %llu usec (worked for %llu, sleeped for %llu).", loop_usec, usec, susec);

		if(usec < (rrd_update_every * 1000000ULL / 2ULL)) susec = (rrd_update_every * 1000000ULL) - usec;
		else susec = rrd_update_every * 1000000ULL / 2ULL;

		// --------------------------------------------------------------------
		// Calculate loop time

		last.tv_sec = now.tv_sec;
		last.tv_usec = now.tv_usec;
		total_susec += loop_usec;

		// --------------------------------------------------------------------
		// check chart 1

		if(check1->counter_done) rrdset_next_usec(check1, loop_usec);
		rrddim_set(check1, "absolute", 1000000);
		rrddim_set(check1, "incremental", total_susec);
		rrdset_done(check1);

		// --------------------------------------------------------------------
		// check chart 2

		if(check2->counter_done) rrdset_next(check2);
		rrddim_set(check2, "absolute", 1000000);
		rrddim_set(check2, "incremental", total_susec);
		rrdset_done(check2);

		// --------------------------------------------------------------------
		// check chart 3

		if(!apps_cpu) apps_cpu = rrdset_find("apps.cpu");
		if(check3->counter_done) rrdset_next_usec(check3, loop_usec);
		gettimeofday(&loop, NULL);
		rrddim_set(check3, "caller", (long long)usecdiff(&loop, &check1->last_collected_time));
		rrddim_set(check3, "netdata", (long long)usecdiff(&loop, &check2->last_collected_time));
		if(apps_cpu) rrddim_set(check3, "apps.plugin", (long long)usecdiff(&loop, &apps_cpu->last_collected_time));
		rrdset_done(check3);
	}

	pthread_exit(NULL);
	return NULL;
}

