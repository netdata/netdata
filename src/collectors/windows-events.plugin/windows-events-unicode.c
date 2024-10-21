// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-unicode.h"

inline void utf82unicode(wchar_t *dst, size_t dst_size, const char *src) {
    if (src) {
        // Convert from UTF-8 to wide char (UTF-16)
        if (utf8_to_utf16(dst, dst_size, src, -1) == 0)
            wcsncpy(dst, L"[failed conv.]", dst_size - 1);
    }
    else
        wcsncpy(dst, L"[null]", dst_size - 1);
}

inline void unicode2utf8(char *dst, size_t dst_size, const wchar_t *src) {
    if (src) {
        if(WideCharToMultiByte(CP_UTF8, 0, src, -1, dst, (int)dst_size, NULL, NULL) == 0)
            strncpyz(dst, "[failed conv.]", dst_size - 1);
    }
    else
        strncpyz(dst, "[null]", dst_size - 1);
}

wchar_t *channel2unicode(const char *utf8str) {
    static __thread wchar_t buffer[1024];
    utf82unicode(buffer, _countof(buffer), utf8str);
    return buffer;
}

char *channel2utf8(const wchar_t *channel) {
    static __thread char buffer[1024];
    unicode2utf8(buffer, sizeof(buffer), channel);
    return buffer;
}

char *query2utf8(const wchar_t *query) {
    static __thread char buffer[16384];
    unicode2utf8(buffer, sizeof(buffer), query);
    return buffer;
}

char *provider2utf8(const wchar_t *provider) {
    static __thread char buffer[256];
    unicode2utf8(buffer, sizeof(buffer), provider);
    return buffer;
}
