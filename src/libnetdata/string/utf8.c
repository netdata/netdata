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

size_t any_to_utf16(uint32_t CodePage, wchar_t *utf16, size_t utf16_count, const char *src, int src_len) {
    if(!src || src_len == 0)
        return 0; // invalid input

    if(!utf16 || !utf16_count) {
        // the caller wants to know the buffer to allocate for the conversion
        int required = MultiByteToWideChar(CodePage, 0, src, src_len, NULL, 0);
        if(required <= 0) return 0; // error in the conversion

        // Add 1 for null terminator only if src_len is not -1
        // so that the caller can call us again to get the entire string (not truncated)
        return (size_t)required + ((src_len != -1) ? 1 : 0);
    }

    // do the conversion directly to the destination buffer
    int rc = MultiByteToWideChar(CodePage, 0, src, src_len, utf16, (int)utf16_count);
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
                utf16[0] = L'\0';
                freez(tmp);
                return 0;
            }

            size_t len = rc;

            // copy as much as we can
            memcpy(utf16, tmp, MIN(len, (utf16_count - 1)) * sizeof(wchar_t));

            // null terminate it
            utf16[MIN(len, (utf16_count - 1))] = L'\0';

            // free the temporary buffer
            freez(tmp);

            // return the actual bytes written
            return MIN(len, utf16_count);
        }

        // empty the destination
        utf16[0] = L'\0';
        return 0;
    }

    size_t len = rc;

    if(len >= utf16_count) {
        // truncate it to fit the null
        utf16[utf16_count - 1] = L'\0';
        return utf16_count;
    }

    if(utf16[len - 1] != L'\0') {
        // the result is not null terminated
        // append the null
        utf16[len] = L'\0';
        return len + 1;
    }

    // the result is already null terminated
    return len;
}
#endif
