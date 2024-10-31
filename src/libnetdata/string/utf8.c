// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
/*
 * Convert any CodePage to UTF16
 * Goals:
 *   1. Destination is always NULL terminated
 *   2. If the destination buffer is not enough, return as much as possible data (truncate)
 *   3. Always return the number of wide characters written, including the null terminator
 */

size_t any_to_utf16(uint32_t CodePage, wchar_t *dst, size_t dst_size, const char *src, int src_len, bool *truncated) {
    if(!src || src_len == 0) {
        // invalid input
        if(truncated)
            *truncated = true;

        if(dst && dst_size)
            *dst = L'\0';
        return 0;
    }

    if(!dst || !dst_size) {
        // the caller wants to know the buffer to allocate for the conversion

        if(truncated)
            *truncated = true;

        int required = MultiByteToWideChar(CodePage, 0, src, src_len, NULL, 0);
        if(required <= 0) return 0; // error in the conversion

        // Add 1 for null terminator only if src_len is not -1
        // so that the caller can call us again to get the entire string (not truncated)
        return (size_t)required + ((src_len != -1) ? 1 : 0);
    }

    // do the conversion directly to the destination buffer
    int rc = MultiByteToWideChar(CodePage, 0, src, src_len, dst, (int)dst_size);
    if(rc <= 0) {
        if(truncated)
            *truncated = true;

        // conversion failed, let's see why...
        DWORD status = GetLastError();
        if(status == ERROR_INSUFFICIENT_BUFFER) {
            // it cannot fit entirely, let's allocate a new buffer to convert it
            // and then truncate it to the destination buffer

            // clear errno and LastError to clear the error of the
            // MultiByteToWideChar() that failed
            errno_clear();

            // get the required size
            int required_size = MultiByteToWideChar(CodePage, 0, src, src_len, NULL, 0);

            // mallocz() never fails (exits the program on NULL)
            wchar_t *tmp = mallocz(required_size * sizeof(wchar_t));

            // convert it, now it should fit
            rc = MultiByteToWideChar(CodePage, 0, src, src_len, tmp, required_size);
            if (rc <= 0) {
                // it failed!
                *dst = L'\0';
                freez(tmp);
                return 0;
            }

            size_t len = rc;

            // copy as much as we can
            memcpy(dst, tmp, MIN(len, (dst_size - 1)) * sizeof(wchar_t));

            // null terminate it
            dst[MIN(len, (dst_size - 1))] = L'\0';

            // free the temporary buffer
            freez(tmp);

            // return the actual bytes written
            return MIN(len, dst_size);
        }

        // empty the destination
        *dst = L'\0';
        return 0;
    }

    size_t len = rc;

    if(truncated)
        *truncated = false;

    if(len >= dst_size) {
        if(dst[dst_size - 1] != L'\0') {
            if (truncated)
                *truncated = true;

            // Truncate it to fit the null terminator
            dst[dst_size - 1] = L'\0';
        }
        return dst_size;
    }

    if(dst[len - 1] != L'\0') {
        // the result is not null terminated
        // append the null
        dst[len] = L'\0';
        return len + 1;
    }

    // the result is already null terminated
    return len;
}

/*
 * Convert UTF16 (wide-character string) to UTF8
 * Goals:
 *   1. Destination is always NULL terminated
 *   2. If the destination buffer is not enough, return as much as possible data (truncate)
 *   3. Always return the number of bytes written, including the null terminator
 */

size_t utf16_to_utf8(char *dst, size_t dst_size, const wchar_t *src, int src_len, bool *truncated) {
    if (!src || src_len == 0) {
        // invalid input
        if(truncated)
            *truncated = true;

        if(dst && dst_size)
            *dst = '\0';

        return 0;
    }

    if (!dst || dst_size == 0) {
        // The caller wants to know the buffer size required for the conversion

        if(truncated)
            *truncated = true;

        int required = WideCharToMultiByte(CP_UTF8, 0, src, src_len, NULL, 0, NULL, NULL);
        if (required <= 0) return 0; // error in the conversion

        // Add 1 for null terminator only if src_len is not -1
        return (size_t)required + ((src_len != -1) ? 1 : 0);
    }

    // Perform the conversion directly into the destination buffer
    int rc = WideCharToMultiByte(CP_UTF8, 0, src, src_len, dst, (int)dst_size, NULL, NULL);
    if (rc <= 0) {
        if(truncated)
            *truncated = true;

        // Conversion failed, let's see why...
        DWORD status = GetLastError();
        if (status == ERROR_INSUFFICIENT_BUFFER) {
            // It cannot fit entirely, let's allocate a new buffer to convert it
            // and then truncate it to the destination buffer

            // Clear errno and LastError to clear the error of the
            // WideCharToMultiByte() that failed
            errno_clear();

            // Get the required size
            int required_size = WideCharToMultiByte(CP_UTF8, 0, src, src_len, NULL, 0, NULL, NULL);

            // mallocz() never fails (exits the program on NULL)
            char *tmp = mallocz(required_size * sizeof(char));

            // Convert it, now it should fit
            rc = WideCharToMultiByte(CP_UTF8, 0, src, src_len, tmp, required_size, NULL, NULL);
            if (rc <= 0) {
                // Conversion failed
                *dst = '\0';
                freez(tmp);
                return 0;
            }

            size_t len = rc;

            // Copy as much as we can
            memcpy(dst, tmp, MIN(len, (dst_size - 1)) * sizeof(char));

            // Null-terminate it
            dst[MIN(len, (dst_size - 1))] = '\0';

            // Free the temporary buffer
            freez(tmp);

            // Return the actual bytes written
            return MIN(len, dst_size);
        }

        // Empty the destination
        *dst = '\0';
        return 0;
    }

    size_t len = rc;

    if(truncated)
        *truncated = false;

    if (len >= dst_size) {
        if(dst[dst_size - 1] != '\0') {
            if (truncated)
                *truncated = true;

            // Truncate it to fit the null terminator
            dst[dst_size - 1] = '\0';
        }
        return dst_size;
    }

    if (dst[len - 1] != '\0') {
        // The result is not null-terminated
        // Append the null terminator
        dst[len] = '\0';
        return len + 1;
    }

    // The result is already null-terminated
    return len;
}

// --------------------------------------------------------------------------------------------------------------------

size_t txt_compute_new_size(size_t old_size, size_t required_size) {
    size_t size = (required_size % 2048 == 0) ? required_size : required_size + 2048;
    size = (size / 2048) * 2048;

    if(size < old_size * 2)
        size = old_size * 2;

    return size;
}

// --------------------------------------------------------------------------------------------------------------------
// TXT_UTF8

void txt_utf8_cleanup(TXT_UTF8 *dst) {
    freez(dst->data);
    dst->data = NULL;
    dst->used = 0;
}

void txt_utf8_resize(TXT_UTF8 *dst, size_t required_size, bool keep) {
    if(required_size <= dst->size)
        return;

    size_t new_size = txt_compute_new_size(dst->size, required_size);

    if(keep && dst->data)
        dst->data = reallocz(dst->data, new_size);
    else {
        txt_utf8_cleanup(dst);
        dst->data = mallocz(new_size);
        dst->used = 0;
    }

    dst->size = new_size;
}

void txt_utf8_empty(TXT_UTF8 *dst) {
    txt_utf8_resize(dst, 1, false);
    dst->data[0] = '\0';
    dst->used = 1;
}

void txt_utf8_set(TXT_UTF8 *dst, const char *txt, size_t txt_len) {
    txt_utf8_resize(dst, txt_len + 1, false);
    memcpy(dst->data, txt, txt_len);
    dst->used = txt_len + 1;
    dst->data[dst->used - 1] = '\0';
}

void txt_utf8_append(TXT_UTF8 *dst, const char *txt, size_t txt_len) {
    if(dst->used <= 1) {
        // the destination is empty
        txt_utf8_set(dst, txt, txt_len);
    }
    else {
        // there is something already in the buffer
        txt_utf8_resize(dst, dst->used + txt_len, true);
        memcpy(&dst->data[dst->used - 1], txt, txt_len);
        dst->used += txt_len; // the null was already counted
        dst->data[dst->used - 1] = '\0';
    }
}

// --------------------------------------------------------------------------------------------------------------------
// TXT_UTF16

void txt_utf16_cleanup(TXT_UTF16 *dst) {
    freez(dst->data);
}

void txt_utf16_resize(TXT_UTF16 *dst, size_t required_size, bool keep) {
    if(required_size <= dst->size)
        return;

    size_t new_size = txt_compute_new_size(dst->size, required_size);

    if (keep && dst->data) {
        dst->data = reallocz(dst->data, new_size * sizeof(wchar_t));
    } else {
        txt_utf16_cleanup(dst);
        dst->data = mallocz(new_size * sizeof(wchar_t));
        dst->used = 0;
    }

    dst->size = new_size;
}

void txt_utf16_set(TXT_UTF16 *dst, const wchar_t *txt, size_t txt_len) {
    txt_utf16_resize(dst, dst->used + txt_len + 1, true);
    memcpy(dst->data, txt, txt_len * sizeof(wchar_t));
    dst->used = txt_len + 1;
    dst->data[dst->used - 1] = '\0';
}

void txt_utf16_append(TXT_UTF16 *dst, const wchar_t *txt, size_t txt_len) {
    if(dst->used <= 1) {
        // the destination is empty
        txt_utf16_set(dst, txt, txt_len);
    }
    else {
        // there is something already in the buffer
        txt_utf16_resize(dst, dst->used + txt_len, true);
        memcpy(&dst->data[dst->used - 1], txt, txt_len * sizeof(wchar_t));
        dst->used += txt_len; // the null was already counted
        dst->data[dst->used - 1] = '\0';
    }
}

// --------------------------------------------------------------------------------------------------------------------

bool wchar_to_txt_utf8(TXT_UTF8 *dst, const wchar_t *src, int src_len) {
    if(!src || !src_len) {
        txt_utf8_empty(dst);
        return false;
    }

    if(!dst->data && !dst->size) {
        size_t size = utf16_to_utf8(NULL, 0, src, src_len, NULL);
        if(!size) {
            txt_utf8_empty(dst);
            return false;
        }

        // we +1 here to avoid entering the next condition below
        txt_utf8_resize(dst, size, false);
    }

    bool truncated = false;
    dst->used = utf16_to_utf8(dst->data, dst->size, src, src_len, &truncated);
    if(truncated) {
        // we need to resize
        size_t needed = utf16_to_utf8(NULL, 0, src, src_len, NULL); // find the size needed
        if(!needed) {
            txt_utf8_empty(dst);
            return false;
        }

        txt_utf8_resize(dst, needed, false);
        dst->used = utf16_to_utf8(dst->data, dst->size, src, src_len, NULL);
    }

    // Make sure it is not zero padded at the end
    while(dst->used >= 2 && dst->data[dst->used - 2] == 0)
        dst->used--;

    internal_fatal(strlen(dst->data) + 1 != dst->used,
                   "Wrong UTF8 string length");

    return true;
}

bool txt_utf16_to_utf8(TXT_UTF8 *utf8, TXT_UTF16 *utf16) {
    fatal_assert(utf8 && ((utf8->data && utf8->size) || (!utf8->data && !utf8->size)));
    fatal_assert(utf16 && ((utf16->data && utf16->size) || (!utf16->data && !utf16->size)));

    // pass the entire utf16 size, including the null terminator
    // so that the resulting utf8 message will be null terminated too.
    return wchar_to_txt_utf8(utf8, utf16->data, (int)utf16->used - 1);
}

char *utf16_to_utf8_strdupz(const wchar_t *src, size_t *dst_len) {
    size_t size = utf16_to_utf8(NULL, 0, src, -1, NULL);
    if (size) {
        char *dst = mallocz(size);

        size = utf16_to_utf8(dst, size, src, -1, NULL);
        if(dst_len)
            *dst_len = size - 1;

        return dst;
    }

    if(dst_len)
        *dst_len = 0;

    return NULL;
}

#endif
