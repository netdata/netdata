// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-unicode.h"

inline void utf82unicode(wchar_t *dst, size_t dst_size, const char *src) {
    if (src) {
        // Convert from UTF-8 to wide char (UTF-16)
        if (MultiByteToWideChar(CP_UTF8, 0, src, -1, dst, (int)dst_size) == 0)
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
    utf82unicode(buffer, sizeof(buffer) / sizeof(buffer[0]), utf8str);
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

bool wevt_str_wchar_to_utf8(TXT_UTF8 *utf8, const wchar_t *src, int src_len_with_null) {
    if(!src || !src_len_with_null)
        goto cleanup;

    // make sure the input is null terminated at the exact point we need it
    // (otherwise, the output will not be null terminated either)
    assert(src_len_with_null == -1 || (src_len_with_null >= 1 && src[src_len_with_null - 1] == 0));

    // Try to convert using the existing buffer (if it exists, otherwise get the required buffer size)
    int size = WideCharToMultiByte(CP_UTF8, 0, src, src_len_with_null, utf8->data, (int)utf8->size, NULL, NULL);
    if(size <= 0 || !utf8->data) {
        // we have to set a buffer, or increase it

        if(utf8->data) {
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
        txt_utf8_resize(utf8, size);
        size = WideCharToMultiByte(CP_UTF8, 0, src, src_len_with_null, utf8->data, (int)utf8->size, NULL, NULL);
        if (size <= 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "WideCharToMultiByte() failed after resizing.");
            goto cleanup;
        }
    }

    // Make sure it is not zero padded at the end
    while(size >= 2 && utf8->data[size - 2] == 0)
        size--;

    utf8->used = (size_t)size;

    internal_fatal(strlen(utf8->data) + 1 != utf8->used,
                   "Wrong UTF8 string length");

    return true;

cleanup:
    txt_utf8_resize(utf8, 128);
    if(src)
        utf8->used = snprintfz(utf8->data, utf8->size, "[failed conv.]") + 1;
    else {
        utf8->data[0] = '\0';
        utf8->used = 1;
    }

    return false;
}

bool wevt_str_unicode_to_utf8(TXT_UTF8 *utf8, TXT_UNICODE *unicode) {
    assert(utf8 && ((utf8->data && utf8->size) || (!utf8->data && !utf8->size)));
    assert(unicode && ((unicode->data && unicode->size) || (!unicode->data && !unicode->size)));

    // pass the entire unicode size, including the null terminator
    // so that the resulting utf8 message will be null terminated too.
    return wevt_str_wchar_to_utf8(utf8, unicode->data, (int)unicode->used);
}

