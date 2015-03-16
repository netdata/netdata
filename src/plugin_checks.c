#include <pthread.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <strings.h>
#include <unistd.h>

#include "common.h"
#include "config.h"
#include "log.h"
#include "rrd.h"
#include "plugin_checks.h"

void *checks_main(void *ptr)
{
	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	unsigned long long usec = 0, susec = update_every * 1000000ULL, loop_usec = 0, total_susec = 0;
	struct timeval now, last, loop;

	RRD_STATS *check1, *check2, *check3, *apps_cpu = NULL;

	check1 = rrd_stats_create("netdata", "check1", NULL, "netdata", "Caller gives microseconds", "a million !", 99999, update_every, CHART_TYPE_LINE);
	rrd_stats_dimension_add(check1, "absolute", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check1, "incremental", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);

	check2 = rrd_stats_create("netdata", "check2", NULL, "netdata", "Netdata calcs microseconds", "a million !", 99999, update_every, CHART_TYPE_LINE);
	rrd_stats_dimension_add(check2, "absolute", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check2, "incremental", NULL, 1, 1 * update_every, RRD_DIMENSION_INCREMENTAL);

	check3 = rrd_stats_create("netdata", "checkdt", NULL, "netdata", "Clock difference", "microseconds diff", 99999, update_every, CHART_TYPE_LINE);
	rrd_stats_dimension_add(check3, "caller", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check3, "netdata", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check3, "apps.plugin", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);

	gettimeofday(&last, NULL);
	while(1) {
		usleep(susec);

		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&now, NULL);
		loop_usec = usecdiff(&now, &last);
		usec = loop_usec - susec;
		debug(D_PROCNETDEV_LOOP, "CHECK: last loop took %llu usec (worked for %llu, sleeped for %llu).", loop_usec, usec, susec);
		
		if(usec < (update_every * 1000000ULL / 2ULL)) susec = (update_every * 1000000ULL) - usec;
		else susec = update_every * 1000000ULL / 2ULL;

		// --------------------------------------------------------------------
		// Calculate loop time

		last.tv_sec = now.tv_sec;
		last.tv_usec = now.tv_usec;
		total_susec += loop_usec;

		// --------------------------------------------------------------------
		// check chart 1

		if(check1->counter_done) rrd_stats_next_usec(check1, loop_usec);
		rrd_stats_dimension_set(check1, "absolute", 1000000);
		rrd_stats_dimension_set(check1, "incremental", total_susec);
		rrd_stats_done(check1);

		// --------------------------------------------------------------------
		// check chart 2

		if(check2->counter_done) rrd_stats_next(check2);
		rrd_stats_dimension_set(check2, "absolute", 1000000);
		rrd_stats_dimension_set(check2, "incremental", total_susec);
		rrd_stats_done(check2);

		// --------------------------------------------------------------------
		// check chart 3

		if(!apps_cpu) apps_cpu = rrd_stats_find("apps.cpu");
		if(check3->counter_done) rrd_stats_next_usec(check3, loop_usec);
		gettimeofday(&loop, NULL);
		rrd_stats_dimension_set(check3, "caller", (long long)usecdiff(&loop, &check1->last_collected_time));
		rrd_stats_dimension_set(check3, "netdata", (long long)usecdiff(&loop, &check2->last_collected_time));
		if(apps_cpu) rrd_stats_dimension_set(check3, "apps.plugin", (long long)usecdiff(&loop, &apps_cpu->last_collected_time));
		rrd_stats_done(check3);
	}

	return NULL;
}

