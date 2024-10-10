// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
#include <windows.h>

/*
 * Convert any CodePage to UTF16
 * Goals:
 *   1. Destination is always NULL terminated
 *   2. If the destination buffer is not enough, return as much as possible data (truncate)
 *   3. Always return the number of wide characters written, including the null terminator
 */

size_t any_to_utf16(uint32_t CodePage, wchar_t *dst, size_t dst_size, const char *src, int src_len) {
    if(!src || src_len == 0) {
        // invalid input
        if(dst && dst_size)
            *dst = L'\0';
        return 0;
    }

    if(!dst || !dst_size) {
        // the caller wants to know the buffer to allocate for the conversion
        int required = MultiByteToWideChar(CodePage, 0, src, src_len, NULL, 0);
        if(required <= 0) return 0; // error in the conversion

        // Add 1 for null terminator only if src_len is not -1
        // so that the caller can call us again to get the entire string (not truncated)
        return (size_t)required + ((src_len != -1) ? 1 : 0);
    }

    // do the conversion directly to the destination buffer
    int rc = MultiByteToWideChar(CodePage, 0, src, src_len, dst, (int)dst_size);
    if(rc <= 0) {
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

    if(len >= dst_size) {
        // truncate it to fit the null
        dst[dst_size - 1] = L'\0';
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

size_t utf16_to_utf8(char *dst, size_t dst_size, const wchar_t *src, int src_len) {
    if (!src || src_len == 0) {
        // invalid input
        if(dst && dst_size)
            *dst = L'\0';
        return 0;
    }

    if (!dst || dst_size == 0) {
        // The caller wants to know the buffer size required for the conversion
        int required = WideCharToMultiByte(CP_UTF8, 0, src, src_len, NULL, 0, NULL, NULL);
        if (required <= 0) return 0; // error in the conversion

        // Add 1 for null terminator only if src_len is not -1
        return (size_t)required + ((src_len != -1) ? 1 : 0);
    }

    // Perform the conversion directly into the destination buffer
    int rc = WideCharToMultiByte(CP_UTF8, 0, src, src_len, dst, (int)dst_size, NULL, NULL);
    if (rc <= 0) {
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

    if (len >= dst_size) {
        // Truncate it to fit the null terminator
        dst[dst_size - 1] = '\0';
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
#endif
