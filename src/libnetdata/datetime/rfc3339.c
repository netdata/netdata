// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#include "rfc3339.h"

size_t rfc3339_datetime_ut(char *buffer, size_t len, usec_t now_ut, size_t fractional_digits, bool utc) {
    if (!buffer || len == 0)
        return 0;

    time_t t = (time_t)(now_ut / USEC_PER_SEC);
    struct tm *tmp, tmbuf;

    if (utc)
        tmp = gmtime_r(&t, &tmbuf);
    else
        tmp = localtime_r(&t, &tmbuf);

    if (!tmp) {
        buffer[0] = '\0';
        return 0;
    }

    size_t used_length = strftime(buffer, len, "%Y-%m-%dT%H:%M:%S", tmp);
    if (used_length == 0) {
        buffer[0] = '\0';
        return 0;
    }

    if (fractional_digits >= 1 && fractional_digits <= 9) {
        int fractional_part = (int)(now_ut % USEC_PER_SEC);
        if (fractional_part && len - used_length > fractional_digits + 1) {
            char format[] = ".%01d";
            format[3] = (char)('0' + fractional_digits);

            // Adjust fractional part
            fractional_part /= (int)pow(10, 6 - fractional_digits);

            used_length += snprintf(buffer + used_length, len - used_length,
                                    format, fractional_part);
        }
    }

    if (utc) {
        if (used_length + 1 < len) {
            buffer[used_length++] = 'Z';
            buffer[used_length] = '\0';
        }
    }
    else {
        long offset = tmbuf.tm_gmtoff;
        int hours = (int)(offset / 3600);
        int minutes = abs((int)((offset % 3600) / 60));

        if (used_length + 7 < len) { // Space for "+HH:MM\0"
            used_length += snprintf(buffer + used_length, len - used_length, "%+03d:%02d", hours, minutes);
        }
    }

    return used_length;
}

usec_t rfc3339_parse_ut(const char *rfc3339, char **endptr) {
    struct tm tm = { 0 };
    int tz_hours = 0, tz_mins = 0;
    char *s;
    usec_t timestamp, usec = 0;

    // Parse date and time (up to seconds)
    s = strptime(rfc3339, "%Y-%m-%dT%H:%M:%S", &tm);
    if (!s)
        return 0; // Parsing error

    // Parse fractional seconds if present
    if (*s == '.') {
        char *next;
        usec = strtoul(s + 1, &next, 10);
        int digits_parsed = (int)(next - (s + 1));

        if (digits_parsed < 1 || digits_parsed > 9)
            return 0; // Parsing error

        static const usec_t fix_usec[] = {
            1000000, // 0 digits (not used)
            100000,  // 1 digit
            10000,   // 2 digits
            1000,    // 3 digits
            100,     // 4 digits
            10,      // 5 digits
            1,       // 6 digits
            10,      // 7 digits
            100,     // 8 digits
            1000     // 9 digits
        };

        if (digits_parsed <= 6)
            usec = usec * fix_usec[digits_parsed];
        else
            usec = usec / fix_usec[digits_parsed];

        s = next;
    }

    // Parse timezone specification
    int tz_offset = 0;
    if (*s == '+' || *s == '-') {
        // Ensure format is correct: e.g. +02:00 or -05:30

        if (!isdigit((uint8_t)s[1]) || !isdigit((uint8_t)s[2]) || s[3] != ':' ||
            !isdigit((uint8_t)s[4]) || !isdigit((uint8_t)s[5]))
            return 0; // Parsing error

        char tz_sign = *s;
        tz_hours = (s[1] - '0') * 10 + (s[2] - '0');
        tz_mins  = (s[4] - '0') * 10 + (s[5] - '0');
        tz_offset = tz_hours * 3600 + tz_mins * 60;
        tz_offset *= (tz_sign == '+' ? 1 : -1);

        s += 6; // Move past the timezone part
    }
    else if (*s == 'Z')
        s++;
    else
        return 0; // Invalid RFC 3339 timezone specification

    // Convert struct tm to time_t in UTC
    time_t epoch_s;

#if defined(HAVE_TIMEGM)
    // If available, use timegm() which interprets tm as UTC.
    epoch_s = timegm(&tm);
#else
    // Use mktime(), which assumes tm is local time, then adjust.
    epoch_s = mktime(&tm);
    if (epoch_s == -1)
        return 0; // Error in time conversion

#  if defined(HAVE_TM_GMTOFF)
    // tm.tm_gmtoff is the offset (in seconds) of local time from UTC.
    epoch_s -= tm.tm_gmtoff;
#  else
    // Fallback: compute the difference between localtime and gmtime.
    {
        struct tm local_tm, utc_tm;
#if defined(_POSIX_THREAD_SAFE_FUNCTIONS) && !defined(__APPLE__)
        localtime_r(&epoch_s, &local_tm);
        gmtime_r(&epoch_s, &utc_tm);
#else
        // If thread-safe functions are not available, use localtime() and gmtime()
        struct tm *lt = localtime(&epoch_s);
        struct tm *gt = gmtime(&epoch_s);
        if (!lt || !gt)
            return 0;
        local_tm = *lt;
        utc_tm   = *gt;
#endif
        int local_offset = (local_tm.tm_hour - utc_tm.tm_hour) * 3600 +
                           (local_tm.tm_min  - utc_tm.tm_min)  * 60;
        int day_diff = local_tm.tm_yday - utc_tm.tm_yday;
        local_offset += day_diff * 86400;
        epoch_s -= local_offset;
    }
#  endif
#endif

    // Combine seconds with fractional microseconds, then adjust for the RFC 3339 timezone.
    timestamp = (usec_t)epoch_s * USEC_PER_SEC + usec;
    timestamp -= tz_offset * USEC_PER_SEC;

    if (endptr)
        *endptr = s;

    return timestamp;
}
