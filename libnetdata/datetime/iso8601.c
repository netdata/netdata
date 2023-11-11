// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

size_t iso8601_datetime_utc_ut(char *buffer, size_t len, usec_t now_ut) {
    if(unlikely(!buffer || !len))
        return 0;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    // Use gmtime_r for UTC time conversion.
    tmp = gmtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return 0;
    }

    // Format the date and time according to the ISO 8601 format with a 'Z' designator for UTC.
    size_t ret = strftime(buffer, len, "%Y-%m-%dT%H:%M:%SZ", tmp);
    if (unlikely(ret == 0))
        buffer[0] = '\0';

    return ret;
}

size_t iso8601_datetime_usec_utc_ut(char *buffer, size_t len, usec_t now_ut) {
    if (unlikely(!buffer || !len))
        return 0;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    // Use gmtime_r for UTC time conversion.
    tmp = gmtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return 0;
    }

    // Format the date and time according to the ISO 8601 format with microseconds.
    size_t ret = strftime(buffer, len, "%Y-%m-%dT%H:%M:%S", tmp);
    if (unlikely(ret == 0)) {
        buffer[0] = '\0';
        return 0;
    }

    // Calculate the remaining microseconds
    int microseconds = (int)(now_ut % USEC_PER_SEC);

    // Add microseconds to the string.
    // Check that there is enough space in the buffer to add microseconds and the 'Z'.
    if (unlikely(len - ret < 8)) { // 6 digits for microseconds, 1 for 'Z', 1 for '\0'
        buffer[0] = '\0';
        return 0;
    }

    return ret + snprintfz(buffer + ret, len - ret, ".%06dZ", microseconds);
}

size_t iso8601_datetime_with_local_timezone_ut(char *buffer, size_t len, usec_t now_ut) {
    if(unlikely(!buffer || len == 0))
        return 0;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    // Use localtime_r for local time conversion.
    tmp = localtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return 0;
    }

    // Format the date and time according to the ISO 8601 format.
    size_t used_length = strftime(buffer, len, "%Y-%m-%dT%H:%M:%S", tmp);
    if (unlikely(used_length == 0)) {
        buffer[0] = '\0';
        return 0;
    }

    // Calculate the timezone offset in hours and minutes from UTC.
    long offset = tmbuf.tm_gmtoff;
    int hours = (int)(offset / 3600); // Convert offset seconds to hours.
    int minutes = (int)((offset % 3600) / 60); // Convert remainder to minutes (keep the sign for minutes).

    // Check if timezone is UTC.
    if (hours == 0 && minutes == 0) {
        // For UTC, append 'Z' to the timestamp.
        if (used_length + 1 < len) {
            buffer[used_length] = 'Z';
            buffer[used_length + 1] = '\0'; // null-terminate the string.
            used_length++;
        }
    }
    else {
        // For non-UTC, format the timezone offset. Omit minutes if they are zero.
        if (minutes == 0) {
            // Check enough space is available for the timezone offset string.
            if (used_length + 3 < len) // "+hh\0"
                used_length += snprintfz(buffer + used_length, len - used_length, "%+03d", hours);
        }
        else {
            // Check enough space is available for the timezone offset string.
            if (used_length + 6 < len) // "+hh:mm\0"
                used_length += snprintfz(buffer + used_length, len - used_length, "%+03d:%02d", hours, abs(minutes));
        }
    }

    return used_length;
}
