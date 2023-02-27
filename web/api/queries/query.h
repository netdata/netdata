// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_DATA_QUERY_H
#define NETDATA_API_DATA_QUERY_H

#ifdef __cplusplus
extern "C" {
#endif

typedef enum rrdr_time_grouping {
    RRDR_GROUPING_UNDEFINED = 0,
    RRDR_GROUPING_AVERAGE,
    RRDR_GROUPING_MIN,
    RRDR_GROUPING_MAX,
    RRDR_GROUPING_SUM,
    RRDR_GROUPING_INCREMENTAL_SUM,
    RRDR_GROUPING_TRIMMED_MEAN1,
    RRDR_GROUPING_TRIMMED_MEAN2,
    RRDR_GROUPING_TRIMMED_MEAN3,
    RRDR_GROUPING_TRIMMED_MEAN5,
    RRDR_GROUPING_TRIMMED_MEAN10,
    RRDR_GROUPING_TRIMMED_MEAN15,
    RRDR_GROUPING_TRIMMED_MEAN20,
    RRDR_GROUPING_TRIMMED_MEAN25,
    RRDR_GROUPING_MEDIAN,
    RRDR_GROUPING_TRIMMED_MEDIAN1,
    RRDR_GROUPING_TRIMMED_MEDIAN2,
    RRDR_GROUPING_TRIMMED_MEDIAN3,
    RRDR_GROUPING_TRIMMED_MEDIAN5,
    RRDR_GROUPING_TRIMMED_MEDIAN10,
    RRDR_GROUPING_TRIMMED_MEDIAN15,
    RRDR_GROUPING_TRIMMED_MEDIAN20,
    RRDR_GROUPING_TRIMMED_MEDIAN25,
    RRDR_GROUPING_PERCENTILE25,
    RRDR_GROUPING_PERCENTILE50,
    RRDR_GROUPING_PERCENTILE75,
    RRDR_GROUPING_PERCENTILE80,
    RRDR_GROUPING_PERCENTILE90,
    RRDR_GROUPING_PERCENTILE95,
    RRDR_GROUPING_PERCENTILE97,
    RRDR_GROUPING_PERCENTILE98,
    RRDR_GROUPING_PERCENTILE99,
    RRDR_GROUPING_STDDEV,
    RRDR_GROUPING_CV,
    RRDR_GROUPING_SES,
    RRDR_GROUPING_DES,
    RRDR_GROUPING_COUNTIF,
} RRDR_TIME_GROUPING;

const char *time_grouping_method2string(RRDR_TIME_GROUPING group);
void time_grouping_init(void);
RRDR_TIME_GROUPING time_grouping_parse(const char *name, RRDR_TIME_GROUPING def);
const char *time_grouping_tostring(RRDR_TIME_GROUPING group);

typedef enum rrdr_group_by {
    RRDR_GROUP_BY_NONE      = 0,
    RRDR_GROUP_BY_DIMENSION = (1 << 0),
    RRDR_GROUP_BY_NODE      = (1 << 1),
    RRDR_GROUP_BY_INSTANCE  = (1 << 2),
    RRDR_GROUP_BY_LABEL     = (1 << 3),
} RRDR_GROUP_BY;

struct web_buffer;

RRDR_GROUP_BY group_by_parse(char *s);
void buffer_json_group_by_to_array(struct web_buffer *wb, RRDR_GROUP_BY group_by);

typedef enum rrdr_group_by_function {
    RRDR_GROUP_BY_FUNCTION_AVERAGE,
    RRDR_GROUP_BY_FUNCTION_MIN,
    RRDR_GROUP_BY_FUNCTION_MAX,
    RRDR_GROUP_BY_FUNCTION_SUM,
    RRDR_GROUP_BY_FUNCTION_SUM_COUNT,
} RRDR_GROUP_BY_FUNCTION;

RRDR_GROUP_BY_FUNCTION group_by_aggregate_function_parse(const char *s);
const char *group_by_aggregate_function_to_string(RRDR_GROUP_BY_FUNCTION group_by_function);

struct query_data_statistics;
void query_target_merge_data_statistics(struct query_data_statistics *d, struct query_data_statistics *s);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_API_DATA_QUERY_H
