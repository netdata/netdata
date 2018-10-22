// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_DATA_QUERY_H
#define NETDATA_API_DATA_QUERY_H

typedef enum rrdr_grouping {
    RRDR_GROUPING_UNDEFINED         = 0,
    RRDR_GROUPING_AVERAGE           = 1,
    RRDR_GROUPING_MIN               = 2,
    RRDR_GROUPING_MAX               = 3,
    RRDR_GROUPING_SUM               = 4,
    RRDR_GROUPING_INCREMENTAL_SUM   = 5,
    RRDR_GROUPING_MEDIAN            = 6,
    RRDR_GROUPING_STDDEV            = 7,
    RRDR_GROUPING_SES               = 8,
} RRDR_GROUPING;

extern const char *group_method2string(RRDR_GROUPING group);
extern void web_client_api_v1_init_grouping(void);
extern RRDR_GROUPING web_client_api_request_v1_data_group(const char *name, RRDR_GROUPING def);

#endif //NETDATA_API_DATA_QUERY_H
