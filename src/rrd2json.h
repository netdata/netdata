#include <time.h>

#include "web_buffer.h"
#include "rrd.h"

#ifndef NETDATA_RRD2JSON_H
#define NETDATA_RRD2JSON_H 1

#define HOSTNAME_MAX 1024
extern char *hostname;

// type of JSON generations
#define DATASOURCE_INVALID -1
#define DATASOURCE_JSON 0
#define DATASOURCE_DATATABLE_JSON 1
#define DATASOURCE_DATATABLE_JSONP 2
#define DATASOURCE_SSV 3
#define DATASOURCE_CSV 4
#define DATASOURCE_JSONP 5
#define DATASOURCE_TSV 6
#define DATASOURCE_HTML 7
#define DATASOURCE_JS_ARRAY 8
#define DATASOURCE_SSV_COMMA 9
#define DATASOURCE_CSV_JSON_ARRAY 10

#define DATASOURCE_FORMAT_JSON "json"
#define DATASOURCE_FORMAT_DATATABLE_JSON "datatable"
#define DATASOURCE_FORMAT_DATATABLE_JSONP "datasource"
#define DATASOURCE_FORMAT_JSONP "jsonp"
#define DATASOURCE_FORMAT_SSV "ssv"
#define DATASOURCE_FORMAT_CSV "csv"
#define DATASOURCE_FORMAT_TSV "tsv"
#define DATASOURCE_FORMAT_HTML "html"
#define DATASOURCE_FORMAT_JS_ARRAY "array"
#define DATASOURCE_FORMAT_SSV_COMMA "ssvcomma"
#define DATASOURCE_FORMAT_CSV_JSON_ARRAY "csvjsonarray"

#define GROUP_AVERAGE	0
#define GROUP_MAX 		1
#define GROUP_SUM		2

#define RRDR_OPTION_NONZERO 		0x00000001 // don't output dimensions will just zero values
#define RRDR_OPTION_REVERSED		0x00000002 // output the rows in reverse order (oldest to newest)
#define RRDR_OPTION_ABSOLUTE 		0x00000004 // values positive, for DATASOURCE_SSV before summing
#define RRDR_OPTION_MIN2MAX	 		0x00000008 // for DATASOURCE_SSV, out max - min, instead of sum
#define RRDR_OPTION_SECONDS			0x00000010 // output seconds, instead of dates
#define RRDR_OPTION_MILLISECONDS	0x00000020 // output milliseconds, instead of dates
#define RRDR_OPTION_NULL2ZERO		0x00000040 // do not show nulls, convert them to zeros
#define RRDR_OPTION_OBJECTSROWS		0x00000080 // each row of values should be an object, not an array
#define RRDR_OPTION_GOOGLE_JSON		0x00000100 // comply with google JSON/JSONP specs
#define RRDR_OPTION_JSON_WRAP		0x00000200 // wrap the response in a JSON header with info about the result
#define RRDR_OPTION_LABEL_QUOTES 	0x00000400 // in CSV output, wrap header labels in double quotes
#define RRDR_OPTION_PERCENTAGE		0x00000800 // give values as percentage of total

extern void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb);
extern void rrd_stats_api_v1_charts(BUFFER *wb);

extern unsigned long rrd_stats_one_json(RRDSET *st, char *options, BUFFER *wb);

extern void rrd_stats_graph_json(RRDSET *st, char *options, BUFFER *wb);

extern void rrd_stats_all_json(BUFFER *wb);

extern time_t rrd_stats_json(int type, RRDSET *st, BUFFER *wb, long entries_to_show, long group, int group_method, time_t after, time_t before, int only_non_zero);

extern int rrd2format(RRDSET *st, BUFFER *out, BUFFER *dimensions, uint32_t format, long points, long long after, long long before, int group_method, uint32_t options, time_t *latest_timestamp);


#endif /* NETDATA_RRD2JSON_H */
