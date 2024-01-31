// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

inline size_t rfc7231_datetime(char *buffer, size_t len, time_t now_t) {
    if (unlikely(!buffer || !len))
        return 0;

    struct tm *tmp, tmbuf;

    // Use gmtime_r for UTC time conversion.
    tmp = gmtime_r(&now_t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return 0;
    }

    // Format the date and time according to the RFC 7231 format.
    size_t ret = strftime(buffer, len, "%a, %d %b %Y %H:%M:%S GMT", tmp);
    if (unlikely(ret == 0))
        buffer[0] = '\0';

    return ret;
}

size_t rfc7231_datetime_ut(char *buffer, size_t len, usec_t now_ut) {
    return rfc7231_datetime(buffer, len, (time_t) (now_ut / USEC_PER_SEC));
}
