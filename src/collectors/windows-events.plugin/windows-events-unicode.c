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

char *unicode2utf8_strdupz(const wchar_t *src, size_t *utf8_len) {
    int size = WideCharToMultiByte(CP_UTF8, 0, src, -1, NULL, 0, NULL, NULL);
    if (size > 0) {
        char *dst = mallocz(size);
        WideCharToMultiByte(CP_UTF8, 0, src, -1, dst, size, NULL, NULL);

        if(utf8_len)
            *utf8_len = size - 1;

        return dst;
    }

    if(utf8_len)
        *utf8_len = 0;

    return NULL;
}

wchar_t *channel2unicode(const char *utf8str) {
    static __thread wchar_t buffer[1024];
    utf82unicode(buffer, sizeof(buffer) / sizeof(wchar_t), utf8str);
    return buffer;
}

char *channel2utf8(const wchar_t *channel) {
    static __thread char buffer[1024];
    unicode2utf8(buffer, sizeof(buffer), channel);
    return buffer;
}

char *account2utf8(const wchar_t *user) {
    static __thread char buffer[1024];
    unicode2utf8(buffer, sizeof(buffer), user);
    return buffer;
}

char *domain2utf8(const wchar_t *domain) {
    static __thread char buffer[1024];
    unicode2utf8(buffer, sizeof(buffer), domain);
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

bool wevt_str_wchar_to_utf8(TXT_UTF8 *dst, const wchar_t *src, int src_len_with_null) {
    if(!src || !src_len_with_null)
        goto cleanup;

    // make sure the input is null terminated at the exact point we need it
    // (otherwise, the output will not be null terminated either)
    fatal_assert(src_len_with_null == -1 || (src_len_with_null >= 1 && src[src_len_with_null - 1] == 0));

    // Try to convert using the existing buffer (if it exists, otherwise get the required buffer size)
    int size = WideCharToMultiByte(CP_UTF8, 0, src, src_len_with_null, dst->data, (int)dst->size, NULL, NULL);
    if(size <= 0 || !dst->data) {
        // we have to set a buffer, or increase it

        if(dst->data) {
            // we need to increase it the buffer size

            if(GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "WideCharToMultiByte() failed.");
                goto cleanup;
            }

            // we have to find the required buffer size
            size = WideCharToMultiByte(CP_UTF8, 0, src, src_len_with_null, NULL, 0, NULL, NULL);
            if(size <= 0)
                goto cleanup;
        }

        // Retry conversion with the new buffer
        txt_utf8_resize(dst, size, false);
        size = WideCharToMultiByte(CP_UTF8, 0, src, src_len_with_null, dst->data, (int)dst->size, NULL, NULL);
        if (size <= 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "WideCharToMultiByte() failed after resizing.");
            goto cleanup;
        }
    }

    // Make sure it is not zero padded at the end
    while(size >= 2 && dst->data[size - 2] == 0)
        size--;

    dst->used = (size_t)size;

    internal_fatal(strlen(dst->data) + 1 != dst->used,
                   "Wrong UTF8 string length");

    return true;

cleanup:
    txt_utf8_resize(dst, 128, false);
    if(src)
        dst->used = snprintfz(dst->data, dst->size, "[failed conv.]") + 1;
    else {
        dst->data[0] = '\0';
        dst->used = 1;
    }

    return false;
}

bool wevt_str_unicode_to_utf8(TXT_UTF8 *dst, TXT_UNICODE *unicode) {
    fatal_assert(dst && ((dst->data && dst->size) || (!dst->data && !dst->size)));
    fatal_assert(unicode && ((unicode->data && unicode->size) || (!unicode->data && !unicode->size)));

    // pass the entire unicode size, including the null terminator
    // so that the resulting utf8 message will be null terminated too.
    return wevt_str_wchar_to_utf8(dst, unicode->data, (int)unicode->used);
}
