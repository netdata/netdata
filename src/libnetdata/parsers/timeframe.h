// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TIMEFRAME_H
#define NETDATA_TIMEFRAME_H

#include "parsers.h"

typedef struct {
    time_t after;
    time_t before;
} TIMEFRAME;

#define API_RELATIVE_TIME_MAX (3 * 365 * 86400)

#define API_RELATIVE_TIME_INVALID           (-1000000000)

#define API_RELATIVE_TIME_THIS_MINUTE       (API_RELATIVE_TIME_INVALID - 1) // this minute at 00 seconds
#define API_RELATIVE_TIME_THIS_HOUR         (API_RELATIVE_TIME_INVALID - 2) // this hour at 00 minutes, 00 seconds
#define API_RELATIVE_TIME_TODAY             (API_RELATIVE_TIME_INVALID - 3) // today at 00:00:00
#define API_RELATIVE_TIME_THIS_WEEK         (API_RELATIVE_TIME_INVALID - 4) // this Monday, 00:00:00
#define API_RELATIVE_TIME_THIS_MONTH        (API_RELATIVE_TIME_INVALID - 5) // this month's 1st at 00:00:00
#define API_RELATIVE_TIME_THIS_YEAR         (API_RELATIVE_TIME_INVALID - 6) // this year's Jan 1st, at 00:00:00
#define API_RELATIVE_TIME_LAST_MONTH        (API_RELATIVE_TIME_INVALID - 7) // last month's 1st, at 00:00:00
#define API_RELATIVE_TIME_LAST_YEAR         (API_RELATIVE_TIME_INVALID - 8) // last year's Jan 1st, at 00:00:00

#define TIMEFRAME_INVALID (TIMEFRAME){ .after = API_RELATIVE_TIME_INVALID, .before = API_RELATIVE_TIME_INVALID }

#endif //NETDATA_TIMEFRAME_H
