// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_DATA_QUERY_H
#define NETDATA_API_DATA_QUERY_H

#ifdef __cplusplus
extern "C" {
#endif

typedef enum rrdr_grouping {
    RRDR_GROUPING_UNDEFINED = 0,
    RRDR_GROUPING_AVERAGE,
    RRDR_GROUPING_MIN,
    RRDR_GROUPING_MAX,
    RRDR_GROUPING_SUM,
    RRDR_GROUPING_INCREMENTAL_SUM,
    RRDR_GROUPING_MEDIAN,
    RRDR_GROUPING_STDDEV,
    RRDR_GROUPING_CV,
    RRDR_GROUPING_SES,
    RRDR_GROUPING_DES,
    RRDR_GROUPING_COUNTIF,
} RRDR_GROUPING;

extern const char *group_method2string(RRDR_GROUPING group);
extern void web_client_api_v1_init_grouping(void);
extern RRDR_GROUPING web_client_api_request_v1_data_group(const char *name, RRDR_GROUPING def);
extern const char *web_client_api_request_v1_data_group_to_string(RRDR_GROUPING group);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_API_DATA_QUERY_H
