// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

unsigned int system_hz = 100;
void os_get_system_HZ(void) {
    long ticks;

    if ((ticks = sysconf(_SC_CLK_TCK)) <= 0) {
        netdata_log_error("Cannot get system clock ticks");
        ticks = 100;
    }

    system_hz = (unsigned int) ticks;
}

// =====================================================================================================================
// os_type

#if defined(OS_LINUX)
const char *os_type = "linux";
#endif

#if defined(OS_FREEBSD)
const char *os_type = "freebsd";
#endif

#if defined(OS_MACOS)
const char *os_type = "macos";
#endif

#if defined(OS_WINDOWS)
const char *os_type = "windows";

#define OS_WINDOWS_PATH_TRANSLATION_MAX 8191

char *os_translate_msys_to_windows_path(const char *src) {
    if (!src)
        return strdupz("");

    if (!*src)
        return strdupz("");

    if (src[0] == '/') {
        ssize_t converted_size = cygwin_conv_path(CCP_POSIX_TO_WIN_A, src, NULL, 0);
        if (converted_size > 0) {
            char *converted_path = mallocz((size_t)converted_size);
            if (cygwin_conv_path(CCP_POSIX_TO_WIN_A, src, converted_path, (size_t)converted_size) == 0)
                return converted_path;

            freez(converted_path);
        }
    }

    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    char *converted_path = mallocz(src_len + 3);
    size_t i = 0;
    size_t j = 0;

    if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        converted_path[j++] = (char)toupper((unsigned char)src[0]);
        converted_path[j++] = ':';
        i = 2;

        if ((src[i] == '\\' || src[i] == '/') && j < src_len + 2) {
            converted_path[j++] = '\\';
            i++;
        }
    }
    else if (src_len >= 3 && src[0] == '/' && isalpha((unsigned char)src[1]) && (src[2] == '/' || src[2] == '\0')) {
        converted_path[j++] = (char)toupper((unsigned char)src[1]);
        converted_path[j++] = ':';
        i = 2;

        if (src[i] == '/' && j < src_len + 2) {
            converted_path[j++] = '\\';
            i++;
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        converted_path[j++] = '\\';
        converted_path[j++] = '\\';
        i = 2;
    }

    for (; i < src_len && j < src_len + 2; i++)
        converted_path[j++] = (src[i] == '/') ? '\\' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}

char *os_translate_path(char *dst, const char *src, size_t dst_size) {
    if (!dst || !dst_size)
        return dst;

    if (!src) {
        dst[0] = '\0';
        return dst;
    }

    CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(src);
    snprintfz(dst, dst_size, "%s", translated);
    return dst;
}

const char *os_translate_windows_to_msys_path(const char *src) {
    if (!src)
        return strdupz("");

    // Keep already POSIX-style paths unchanged.
    if (src[0] == '/')
        return strdupz(src);

    ssize_t converted_size = cygwin_conv_path(CCP_WIN_A_TO_POSIX, src, NULL, 0);
    if (converted_size > 0) {
        char *converted_path = mallocz((size_t)converted_size);
        if (cygwin_conv_path(CCP_WIN_A_TO_POSIX, src, converted_path, (size_t)converted_size) == 0)
            return converted_path;

        freez(converted_path);
    }

    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    char *converted_path = mallocz(src_len + 3);
    size_t converted_size_fallback = src_len + 3;
    size_t i = 0;
    size_t j = 0;

    if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        converted_path[j++] = '/';
        if (j < converted_size_fallback - 1)
            converted_path[j++] = (char)tolower((unsigned char)src[0]);

        i = 2;
        if ((src[i] == '\\' || src[i] == '/') && j < converted_size_fallback - 1) {
            converted_path[j++] = '/';
            i++; // consume the separator so the loop below doesn't emit it again
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        converted_path[j++] = '/';
        if (j < converted_size_fallback - 1)
            converted_path[j++] = '/';
        i = 2;
    }

    for (; i < src_len && j < converted_size_fallback - 1; i++)
        converted_path[j++] = (src[i] == '\\') ? '/' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}
#endif
