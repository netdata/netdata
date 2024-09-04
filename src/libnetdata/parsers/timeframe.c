// SPDX-License-Identifier: GPL-3.0-or-later

#include "timeframe.h"

// --------------------------------------------------------------------------------------------------------------------
// timeframe
/*
TIMEFRAME timeframe_parse(const char *txt) {
    if(!txt || !*txt)
        return TIMEFRAME_INVALID;

char buf[strlen(txt) + 1];
memcpy(buf, txt, strlen(txt) + 1);
char *s = trim_all(buf);
if(!s)
    return TIMEFRAME_INVALID;

while(isspace(*s)) s++;

if(strcasecmp(s, "this minute") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_THIS_MINUTE,
        .before = 0,
    };
}
if(strcasecmp(s, "this hour") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_THIS_HOUR,
        .before = 0,
    };
}
if(strcasecmp(s, "today") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_TODAY,
        .before = 0,
    };
}
if(strcasecmp(s, "this week") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_THIS_WEEK,
        .before = 0,
    };
}
if(strcasecmp(s, "this month") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_THIS_MONTH,
        .before = 0,
    };
}
if(strcasecmp(s, "this year") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_THIS_YEAR,
        .before = 0,
    };
}

if(strcasecmp(s, "last minute") == 0) {
    return (TIMEFRAME) {
        .after = -60,
        .before = API_RELATIVE_TIME_THIS_MINUTE,
    };
}
if(strcasecmp(s, "last hour") == 0) {
    return (TIMEFRAME) {
        .after = -3600,
        .before = API_RELATIVE_TIME_THIS_HOUR,
    };
}
if(strcasecmp(s, "yesterday") == 0) {
    return (TIMEFRAME) {
        .after = -86400,
        .before = API_RELATIVE_TIME_TODAY,
    };
}
if(strcasecmp(s, "this week") == 0) {
    return (TIMEFRAME) {
        .after = -86400 * 7,
        .before = API_RELATIVE_TIME_THIS_WEEK,
    };
}
if(strcasecmp(s, "this month") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_LAST_MONTH,
        .before = API_RELATIVE_TIME_THIS_MONTH,
    };
}
if(strcasecmp(s, "this year") == 0) {
    return (TIMEFRAME) {
        .after = API_RELATIVE_TIME_LAST_YEAR,
        .before = API_RELATIVE_TIME_THIS_YEAR,
    };
}

const char *end;
double after = strtondd(s, (char **)&end);

if(end == s)
    return TIMEFRAME_INVALID;

s = end;
while(isspace(*s)) s++;

time_t multiplier = 1;
if(!isdigit(*s) && *s != '-') {
    // after has units
    bool found = false;

    for (size_t i = 0; i < sizeof(units) / sizeof(units[0]); i++) {
        size_t len = strlen(units[i].unit);

        if (units[i].multiplier >= 1 * NSEC_PER_USEC &&
            strncmp(s, units[i].unit, len) == 0 &&
            (isspace(s[len]) || s[len] == '-')) {
            multiplier = units[i].multiplier / NSEC_PER_SEC;
            found = true;
            s += len;
        }
    }

    if(!found)
        return TIMEFRAME_INVALID;
}

const char *dash = strchr(s, '-');
if(!dash) return TIMEFRAME_INVALID;

}
*/
