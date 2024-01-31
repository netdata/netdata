// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

size_t iso8601_datetime_ut(char *buffer, size_t len, usec_t now_ut, ISO8601_OPTIONS options) {
    if(unlikely(!buffer || len == 0))
        return 0;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    if(options & ISO8601_UTC)
        // Use gmtime_r for UTC time conversion.
        tmp = gmtime_r(&t, &tmbuf);
    else
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

    if(options & ISO8601_MILLISECONDS) {
        // Calculate the remaining microseconds
        int milliseconds = (int) ((now_ut % USEC_PER_SEC) / USEC_PER_MS);
        if(milliseconds && len - used_length > 4)
            used_length += snprintfz(buffer + used_length, len - used_length, ".%03d", milliseconds);
    }
    else if(options & ISO8601_MICROSECONDS) {
        // Calculate the remaining microseconds
        int microseconds = (int) (now_ut % USEC_PER_SEC);
        if(microseconds && len - used_length > 7)
            used_length += snprintfz(buffer + used_length, len - used_length, ".%06d", microseconds);
    }

    if(options & ISO8601_UTC) {
        if(used_length + 1 < len) {
            buffer[used_length++] = 'Z';
            buffer[used_length] = '\0'; // null-terminate the string.
        }
    }
    else {
        // Calculate the timezone offset in hours and minutes from UTC.
        long offset = tmbuf.tm_gmtoff;
        int hours = (int) (offset / 3600); // Convert offset seconds to hours.
        int minutes = (int) ((offset % 3600) / 60); // Convert remainder to minutes (keep the sign for minutes).

        // Check if timezone is UTC.
        if(hours == 0 && minutes == 0) {
            // For UTC, append 'Z' to the timestamp.
            if(used_length + 1 < len) {
                buffer[used_length++] = 'Z';
                buffer[used_length] = '\0'; // null-terminate the string.
            }
        }
        else {
            // For non-UTC, format the timezone offset. Omit minutes if they are zero.
            if(minutes == 0) {
                // Check enough space is available for the timezone offset string.
                if(used_length + 3 < len) // "+hh\0"
                    used_length += snprintfz(buffer + used_length, len - used_length, "%+03d", hours);
            }
            else {
                // Check enough space is available for the timezone offset string.
                if(used_length + 6 < len) // "+hh:mm\0"
                    used_length += snprintfz(buffer + used_length, len - used_length,
                                             "%+03d:%02d", hours, abs(minutes));
            }
        }
    }

    return used_length;
}
