#include <time.h>

#include "web_buffer.h"
#include "rrd.h"

#ifndef NETDATA_RRD2JSON_H
#define NETDATA_RRD2JSON_H 1

#define HOSTNAME_MAX 1024
extern char *hostname;

// type of JSON generations
#define DATASOURCE_JSON 0
#define DATASOURCE_GOOGLE_JSON 1
#define DATASOURCE_GOOGLE_JSONP 2

#define GROUP_AVERAGE	0
#define GROUP_MAX 		1
#define GROUP_SUM		2

extern unsigned long rrd_stats_one_json(RRD_STATS *st, char *options, struct web_buffer *wb);

extern void rrd_stats_graph_json(RRD_STATS *st, char *options, struct web_buffer *wb);

extern void rrd_stats_all_json(struct web_buffer *wb);

extern unsigned long rrd_stats_json(int type, RRD_STATS *st, struct web_buffer *wb, int entries_to_show, int group, int group_method, time_t after, time_t before, int only_non_zero);

#endif /* NETDATA_RRD2JSON_H */
