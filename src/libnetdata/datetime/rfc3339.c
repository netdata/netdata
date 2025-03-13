// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#include "rfc3339.h"

// Helper functions for safe printing of date/time components
// These functions don't add a null terminator and return only the exact count of bytes written

static inline size_t print_4digit_year(char *buffer, size_t size, int year) {
    if (!buffer || size < 4) return 0; // Need at least 4 digits

    // Ensure year is in valid range (0-9999)
    year = year < 0 ? 0 : (year > 9999 ? 9999 : year);

    buffer[0] = '0' + (year / 1000) % 10;
    buffer[1] = '0' + (year / 100) % 10;
    buffer[2] = '0' + (year / 10) % 10;
    buffer[3] = '0' + year % 10;

    return 4;
}

static inline size_t print_2digit(char *buffer, size_t size, int value) {
    if (!buffer || size < 2) return 0; // Need at least 2 digits

    // Ensure value is in valid range (0-99)
    value = value < 0 ? 0 : (value > 99 ? 99 : value);

    buffer[0] = '0' + (value / 10) % 10;
    buffer[1] = '0' + value % 10;

    return 2;
}

static inline size_t print_fraction(char *buffer, size_t size, usec_t fraction, size_t digits) {
    if (!buffer || size < digits) return 0;

    // Validate and cap the number of digits
    digits = digits < 1 ? 1 : (digits > 9 ? 9 : digits);

    // Calculate divisor to get correct precision
    usec_t divisor = 1;
    for (size_t i = 0; i < 6 - digits; i++)
        divisor *= 10;

    // Calculate the fraction to print
    fraction = fraction / divisor;

    // Ensure fraction won't exceed the requested number of digits
    usec_t max_value = 1;
    for (size_t i = 0; i < digits; i++)
        max_value *= 10;
    max_value--;

    fraction = fraction > max_value ? max_value : fraction;

    // Print the fraction with leading zeros
    usec_t remaining = fraction;

    // Setup working backwards from least significant digit
    for (int i = digits - 1; i >= 0; i--) {
        buffer[i] = '0' + (remaining % 10);
        remaining /= 10;
    }

    return digits;
}

size_t rfc3339_datetime_ut(char *buffer, size_t len, usec_t now_ut, size_t fractional_digits, bool utc) {
    if (!buffer || len < 20) // Minimum size for YYYY-MM-DDThh:mm:ssZ
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

    size_t pos = 0;

    // Year (4 digits)
    if (len - pos < 4) goto finish;
    pos += print_4digit_year(&buffer[pos], len - pos, tmp->tm_year + 1900);

    // Month separator
    if (len - pos < 1) goto finish;
    buffer[pos++] = '-';

    // Month (2 digits)
    if (len - pos < 2) goto finish;
    pos += print_2digit(&buffer[pos], len - pos, tmp->tm_mon + 1);

    // Day separator
    if (len - pos < 1) goto finish;
    buffer[pos++] = '-';

    // Day (2 digits)
    if (len - pos < 2) goto finish;
    pos += print_2digit(&buffer[pos], len - pos, tmp->tm_mday);

    // T separator
    if (len - pos < 1) goto finish;
    buffer[pos++] = 'T';

    // Hour (2 digits)
    if (len - pos < 2) goto finish;
    pos += print_2digit(&buffer[pos], len - pos, tmp->tm_hour);

    // Minute separator
    if (len - pos < 1) goto finish;
    buffer[pos++] = ':';

    // Minute (2 digits)
    if (len - pos < 2) goto finish;
    pos += print_2digit(&buffer[pos], len - pos, tmp->tm_min);

    // Second separator
    if (len - pos < 1) goto finish;
    buffer[pos++] = ':';

    // Second (2 digits)
    if (len - pos < 2) goto finish;
    pos += print_2digit(&buffer[pos], len - pos, tmp->tm_sec);

    // Add fractional part if requested
    if (fractional_digits > 9) fractional_digits = 9;
    if (fractional_digits) {
        usec_t fractional_part = now_ut % USEC_PER_SEC;

        if (fractional_part > 0) {
            // Need space for decimal point and digits
            if (len - pos < fractional_digits + 1) goto finish;

            buffer[pos++] = '.';
            pos += print_fraction(&buffer[pos], len - pos, fractional_part, fractional_digits);
        }
    }

    // Add timezone information
    if (utc) {
        if (len - pos < 1) goto finish;
        buffer[pos++] = 'Z';
    }
    else {
        long offset = tmbuf.tm_gmtoff;
        int hours = (int)(offset / 3600);
        int minutes = abs((int)((offset % 3600) / 60));

        // Check if timezone is UTC
        if (hours == 0 && minutes == 0) {
            if (len - pos < 1) goto finish;
            buffer[pos++] = 'Z';
        }
        else {
            // Need space for sign, hours, colon, minutes (6 chars total)
            if (len - pos < 6) goto finish;

            // Add timezone offset
            buffer[pos++] = (hours >= 0) ? '+' : '-';
            hours = abs(hours);

            // Hours with leading zero
            pos += print_2digit(&buffer[pos], len - pos, hours);

            // Colon
            buffer[pos++] = ':';

            // Minutes with leading zero
            pos += print_2digit(&buffer[pos], len - pos, minutes);
        }
    }

finish:
    // Ensure null termination
    if (pos < len)
        buffer[pos] = '\0';
    else
        buffer[len - 1] = '\0';

    return pos;
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
