//
// Created by costa on 07/10/2024.
//

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
#include <windows.h>
size_t any_to_utf16(uint32_t CodePage, wchar_t *utf16, size_t utf16_count, const char *src, int src_len) {
    if(!utf16 || !utf16_count)
        return MultiByteToWideChar(CodePage, 0, src, src_len, NULL, 0);

    int len = MultiByteToWideChar(CodePage, 0, src, src_len, utf16, (int)utf16_count);
    if(len <= 0) {
        DWORD status = GetLastError();
        if(status == ERROR_INSUFFICIENT_BUFFER) {
            // get as much as we can
            int required_size = MultiByteToWideChar(CodePage, 0, src, src_len, NULL, 0);
            wchar_t *tmp = mallocz(required_size * sizeof(wchar_t));
            len = MultiByteToWideChar(CodePage, 0, src, src_len, tmp, required_size);
            tmp[len - 1] = L'\0';
            memcpy(utf16, tmp, MIN(len, (int)utf16_count) * sizeof(wchar_t));
            utf16[MIN(len, (int)utf16_count) - 1] = L'\0';
            freez(tmp);
            errno_clear();
            return MIN(len, (int)utf16_count);
        }
        utf16[0] = L'\0';
        return 0;
    }

    if(len >= (int)utf16_count) {
        utf16[utf16_count - 1] = L'\0';
        return utf16_count;
    }

    utf16[len - 1] = L'\0';
    return len;
}
#endif
