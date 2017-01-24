#ifndef NETDATA_RRD2JSON_H
#define NETDATA_RRD2JSON_H 1

/**
 * @file rrd2json.h
 * @brief API to serialize the round robin database.
 *
 * The functions provided by this API can be used to serialize the contet of
 * the round robin datababase into a `BUFFER`. The name of the file is
 * missleading because some of the methods provide other output formats.
 *
 * rrd_stats_api_v1_chart() and rrd_stats_api_v1_charts() serialize the
 * metadata of charts (`RRDSET`) they only support JSON.
 *
 * rrd2format() and rrd2format() serialize the collected data of a chart.
 * They support various output formats. Their string representations are
 * DATASOURCE_FORMAT_<name> and the corresponding integer representation
 * DATASOURCE_<name>.
 *
 * rrd_stats_api_v1_charts_allmetrics_shell() and 
 * rrd_stats_api_v1_charts_allmetrics_prometheus() output the current value
 * of each dimension in the database in a key-value based format readable by
 * shell or prometheus. The string and integer representation of these formats
 * are #ALLMETRICS_FORMAT_SHELL, #ALLMETRICS_FORMAT_PROMETHEUS and
 * #ALLMETRICS_SHELL, #ALLMETRICS_PROMETHEUS.
 */

#define HOSTNAME_MAX 1024 ///< Maximum hostanme length

#define API_RELATIVE_TIME_MAX (3 * 365 * 86400) ///< Last timestamp supported.

#define DATASOURCE_INVALID -1        ///< No valid format.
#define DATASOURCE_JSON 0            ///< Serialize to JSON. @see DATASOURCE_FORMAT_JSON
#define DATASOURCE_DATATABLE_JSON 1  ///< Serialize to datatable JSON. @see DATASOURCE_FORMAT_DATATABLE_JSON
#define DATASOURCE_DATATABLE_JSONP 2 ///< Serialize to datatable JSONP. @see DATASOURCE_FORMAT_DATATABLE_JSONP
#define DATASOURCE_SSV 3             ///< Serialize to ssv. @see DATASOURCE_FORMAT_SSV
#define DATASOURCE_CSV 4             ///< Serialize to csv. @see DATASOURCE_FORMAT_CSV
#define DATASOURCE_JSONP 5           ///< Serialize to JSONP. @see DATASOURCE_FORMAT_JSONP
#define DATASOURCE_TSV 6             ///< Serialize to TSV. @see DATASOURCE_FORMAT_TSV
#define DATASOURCE_HTML 7            ///< Serialize to HTML. @see DATASOURCE_FORMAT_HTML
#define DATASOURCE_JS_ARRAY 8        ///< Serialize to javascript array. @see DATASOURCE_FORMAT_JS_ARRAY
#define DATASOURCE_SSV_COMMA 9       ///< Serialize to comma seperated SSV. @see DATASOURCE_FORMAT_SSV_COMMA
#define DATASOURCE_CSV_JSON_ARRAY 10 ///< Serialize to CSV json array. @see DATASOURCE_CSV_JSON_ARRAY

#define DATASOURCE_FORMAT_JSON "json"                   ///< String representation of DATASOURCE_JSON
#define DATASOURCE_FORMAT_DATATABLE_JSON "datatable"    ///< String representation of DATASOURCE_DATATABLE_JSON
#define DATASOURCE_FORMAT_DATATABLE_JSONP "datasource"  ///< String representation of DATASOURCE_DATATABLE_JSONP
#define DATASOURCE_FORMAT_JSONP "jsonp"                 ///< String representation of DATASOURCE_JSONP
#define DATASOURCE_FORMAT_SSV "ssv"                     ///< String representation of DATASOURCE_SSV
#define DATASOURCE_FORMAT_CSV "csv"                     ///< String representation of DATASOURCE_CSV
#define DATASOURCE_FORMAT_TSV "tsv"                     ///< String representation of DATASOURCE_TSV
#define DATASOURCE_FORMAT_HTML "html"                   ///< String representation of DATASOURCE_HTML
#define DATASOURCE_FORMAT_JS_ARRAY "array"              ///< String representation of DATASOURCE_JS_ARRAY
#define DATASOURCE_FORMAT_SSV_COMMA "ssvcomma"          ///< String representation of DATASOURCE_SSV_COMMA
#define DATASOURCE_FORMAT_CSV_JSON_ARRAY "csvjsonarray" ///< String representation of DATASOURCE_CSV_JSON_ARRAY

#define ALLMETRICS_FORMAT_SHELL "shell"           ///< String representation of ALLMETRICS_SHELL
#define ALLMETRICS_FORMAT_PROMETHEUS "prometheus" ///< String representation of ALLMETRICS_PROMETHEUS

#define ALLMETRICS_SHELL 1      ///< Serialize to shell environmen variable definitions. (`NETDATA_${chart_id^^}_${dimension_id^^}="${value}"`) @see ALLMETRICS_FORMAT_SHELL
#define ALLMETRICS_PROMETHEUS 2 ///< Serialize to a prometheus readable format. @see ALLMETRICS_FORMAT_PROMETHEUS

#define GROUP_UNDEFINED 0       ///< Grouping method is undefined.
#define GROUP_AVERAGE 1         ///< (a,b) => (a+b)/2
#define GROUP_MIN 2             ///< (a,b) => a>b?b:a
#define GROUP_MAX 3             ///< (a,b) => a>b?a:b
#define GROUP_SUM 4             ///< (a,b) => a+b
#define GROUP_INCREMENTAL_SUM 5 ///< ktsaou: Your help needed.

#define RRDR_OPTION_NONZERO 0x00000001      ///< don't output dimensions will just zero values
#define RRDR_OPTION_REVERSED 0x00000002     ///< output the rows in reverse order (oldest to newest)
#define RRDR_OPTION_ABSOLUTE 0x00000004     ///< values positive, for DATASOURCE_SSV before summing
#define RRDR_OPTION_MIN2MAX 0x00000008      ///< when adding dimensions, use max - min, instead of sum
#define RRDR_OPTION_SECONDS 0x00000010      ///< output seconds, instead of dates
#define RRDR_OPTION_MILLISECONDS 0x00000020 ///< output milliseconds, instead of dates
#define RRDR_OPTION_NULL2ZERO 0x00000040    ///< do not show nulls, convert them to zeros
#define RRDR_OPTION_OBJECTSROWS 0x00000080  ///< each row of values should be an object, not an array
#define RRDR_OPTION_GOOGLE_JSON 0x00000100  ///< comply with google JSON/JSONP specs
#define RRDR_OPTION_JSON_WRAP 0x00000200    ///< wrap the response in a JSON header with info about the result
#define RRDR_OPTION_LABEL_QUOTES 0x00000400 ///< in CSV output, wrap header labels in double quotes
#define RRDR_OPTION_PERCENTAGE 0x00000800   ///< give values as percentage of total
#define RRDR_OPTION_NOT_ALIGNED 0x00001000  ///< do not align charts for persistant timeframes

/**
 * Serialize RRDSET `st` into BUFFER `wb`
 *
 * Serialization fits this shema: 
 * \dontinclude netdata-swagger.yaml
 * \skip definitions:
 * \skip chart:
 * \until json_wrap:
 *
 * @param st RRDSET to seralize. 
 * @param wb Web buffer to store result in.
 */
extern void rrd_stats_api_v1_chart(RRDSET *st, BUFFER *wb);
/**
 * Serialiaze all charts including host information.
 *
 * Serialization fits this shema:
 * \dontinclude netdata-swagger.yaml
 * \skip definitions:
 * \skip chart_summary:
 * \until json_wrap:
 *
 * @param wb Web buffer to store result in.
 */
extern void rrd_stats_api_v1_charts(BUFFER *wb);

/**
 * Serialize the current value of all metrics maintained.
 *
 * This returns key value paris that can be exposed as shell variables.
 * The format of these lines is `NETDATA_${chart_id^^}_${dimension_id^^}="${value}"`.
 * The value is rounded to the closest integer, since shell script cannot process decimal numbers.
 *
 * @param wb Web buffer to store result in.
 */
extern void rrd_stats_api_v1_charts_allmetrics_shell(BUFFER *wb);
/**
 * ktsaou: Your help needed
 *
 * @param wb Web buffer to store result in.
 */
extern void rrd_stats_api_v1_charts_allmetrics_prometheus(BUFFER *wb);

/**
 * Serialize RRDSET into `wb`.
 *
 * \deprecated Use rrd_stats_api_v1_chart() instead.
 *
 * @param st RRDSET to serialize.
 * @param options URL options to use
 * @param wb Web buffer to store result in.
 * @return bytes written to `wb`
 */
extern unsigned long rrd_stats_one_json(RRDSET *st, char *options, BUFFER *wb);

/**
 * Serialize RRDSET into `wb`.
 *
 * \deprecated Use rrd_stats_api_v1_chart() instead.
 *
 * @param st RRDSET to serialize.
 * @param options URL options to use
 * @param wb Web buffer to store result in.
 */
extern void rrd_stats_graph_json(RRDSET *st, char *options, BUFFER *wb);

/**
 * Serialize all charts.
 *
 * \deprecated Use rrd_stats_api_v1_charts() instead
 *
 * @param wb Web buffer to store result in.
 */
extern void rrd_stats_all_json(BUFFER *wb);

/**
 * Serialize data for one chart.
 *
 * \todo explain whats going on here exactly.
 *
 * @param type ktsaou: Your help needed.
 * @param st RRDSET to serialize.
 * @param wb Web buffer to store result in.
 * @param entries_to_show Number of entries to serialize.
 * @param group ktsaou: Your help needed.
 * @param group_method GROUP_*
 * @param after timestamp or absolute value to start output.
 * @param before timestamp or absolute value to end output.
 * @param only_non_zero ktsaou: Your help needed
 * @return ktsaou: Your help needed
 */
extern time_t rrd_stats_json(int type, RRDSET *st, BUFFER *wb, long entries_to_show, long group, int group_method, time_t after, time_t before, int only_non_zero);

/**
 * Serialize data for one chart.
 *
 * \todo explain whats going on here exactly.
 *
 * @param st RRDSET to serialize.
 * @param out Web buffer to store result in.
 * @param dimensions list of dimensions to take.
 * @param format DATASOURCE_*
 * @param points number of points to return
 * @param after timestamp or absolute value to start output.
 * @param before timestamp or absolute value to end output.
 * @param group_method GROUP_*
 * @param options RRDR_OPTION_*
 * @param latest_timestamp ktsaou: Your help needed.
 * @return HTTP response status code
 */
extern int rrd2format(RRDSET *st, BUFFER *out, BUFFER *dimensions, uint32_t format, long points, long long after, long long before, int group_method, uint32_t options, time_t *latest_timestamp);
/**
 * Serialize data of one chart.
 *
 * \todo explain how this can be used in detail.
 *
 * @param st RRDSET to serialize.
 * @param wb Web buffer to store result in.
 * @param n ktsaou: Your help needed.
 * @param dimensions String of RRDIM->name's sperated wit ','
 * @param points number of points to return
 * @param after timestamp or absolute value to start output.
 * @param before timestamp or absolute value to end output.
 * @param group_method GROUP_*
 * @param options RRDR_OPTION_*
 * @param db_before timestamp represented by `before`
 * @param db_after timestamp represented by `after`
 * @param value_is_null ktsaou: Your help needed
 * @return HTTP response status code
 */
extern int rrd2value(RRDSET *st, BUFFER *wb, calculated_number *n, const char *dimensions, long points, long long after, long long before, int group_method, uint32_t options, time_t *db_before, time_t *db_after, int *value_is_null);

#endif /* NETDATA_RRD2JSON_H */
