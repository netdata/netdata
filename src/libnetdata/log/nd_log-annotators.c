// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

const char *timestamp_usec_annotator(struct log_field *lf) {
    usec_t ut = log_field_to_uint64(lf);

    if(!ut)
        return NULL;

    static __thread char datetime[RFC3339_MAX_LENGTH];
    rfc3339_datetime_ut(datetime, sizeof(datetime), ut, 3, false);
    return datetime;
}

const char *errno_annotator(struct log_field *lf) {
    int64_t errnum = log_field_to_int64(lf);

    if(errnum == 0)
        return NULL;

    static __thread char buf[256];
    size_t len = print_uint64(buf, errnum);
    buf[len++] = ',';
    buf[len++] = ' ';

    char *msg_to = &buf[len];
    size_t msg_size = sizeof(buf) - len;

    const char *s = errno2str((int)errnum, msg_to, msg_size);
    if(s != msg_to)
        strncpyz(msg_to, s, msg_size - 1);

    return buf;
}

#if defined(OS_WINDOWS)
const char *winerror_annotator(struct log_field *lf) {
    DWORD errnum = log_field_to_uint64(lf);

    if (errnum == 0)
        return NULL;

    static __thread char buf[256];
    size_t len = print_uint64(buf, errnum);
    buf[len++] = ',';
    buf[len++] = ' ';

    char *msg_to = &buf[len];
    size_t msg_size = sizeof(buf) - len;

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
        int utf8_size = WideCharToMultiByte(CP_UTF8, 0, wbuf, -1, msg_to, (int)msg_size, NULL, NULL);
        if (utf8_size == 0)
            snprintf(msg_to, msg_size - 1, "unknown error code");
        msg_to[msg_size - 1] = '\0';
    }
    else
        snprintf(msg_to, msg_size - 1, "unknown error code");

    return buf;
}
#endif

const char *priority_annotator(struct log_field *lf) {
    uint64_t pri = log_field_to_uint64(lf);
    return nd_log_id2priority(pri);
}
