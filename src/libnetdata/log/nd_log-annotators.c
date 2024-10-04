// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

void timestamp_usec_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    usec_t ut = log_field_to_uint64(lf);

    if(!ut)
        return;

    char datetime[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(datetime, sizeof(datetime), ut, 3, false);

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=", 1);
    buffer_json_strcat(wb, datetime);
}

void errno_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    int64_t errnum = log_field_to_int64(lf);

    if(errnum == 0)
        return;

    char buf[1024];
    const char *s = errno2str((int)errnum, buf, sizeof(buf));

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=\"", 2);
    buffer_print_int64(wb, errnum);
    buffer_fast_strcat(wb, ", ", 2);
    buffer_json_strcat(wb, s);
    buffer_fast_strcat(wb, "\"", 1);
}

#if defined(OS_WINDOWS)
void winerror_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    DWORD errnum = log_field_to_uint64(lf);

    if (errnum == 0)
        return;

    char buf[1024];
    wchar_t wbuf[1024];
    DWORD size = FormatMessageW(
        FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS,
        NULL,
        errnum,
        MAKELANGID(LANG_NEUTRAL, SUBLANG_DEFAULT),
        wbuf,
        (DWORD)(sizeof(wbuf) / sizeof(wchar_t) - 1),
        NULL
    );

    if (size > 0) {
        // Remove \r\n at the end
        while (size > 0 && (wbuf[size - 1] == L'\r' || wbuf[size - 1] == L'\n'))
            wbuf[--size] = L'\0';

        // Convert wide string to UTF-8
        int utf8_size = WideCharToMultiByte(CP_UTF8, 0, wbuf, -1, buf, sizeof(buf), NULL, NULL);
        if (utf8_size == 0)
            snprintf(buf, sizeof(buf) - 1, "unknown error code");
        buf[sizeof(buf) - 1] = '\0';
    }
    else
        snprintf(buf, sizeof(buf) - 1, "unknown error code");

    if (buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=\"", 2);
    buffer_print_int64(wb, errnum);
    buffer_fast_strcat(wb, ", ", 2);
    buffer_json_strcat(wb, buf);
    buffer_fast_strcat(wb, "\"", 1);
}
#endif

void priority_annotator(BUFFER *wb, const char *key, struct log_field *lf) {
    uint64_t pri = log_field_to_uint64(lf);

    if(buffer_strlen(wb))
        buffer_fast_strcat(wb, " ", 1);

    buffer_strcat(wb, key);
    buffer_fast_strcat(wb, "=", 1);
    buffer_strcat(wb, nd_log_id2priority(pri));
}

