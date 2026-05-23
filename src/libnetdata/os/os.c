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

// MSYS2 install root on Windows. The netdata MSI installer puts MSYS2 at
// C:\msys64 (matching the upstream MSYS2 default). When we encounter a
// root-relative MSYS path like /opt/netdata, we treat it as living under
// this prefix on disk -- the same way the Cygwin runtime did via its
// mount table, just without the table.
#define NETDATA_MSYS_INSTALL_ROOT "C:\\msys64"
#define NETDATA_MSYS_INSTALL_ROOT_LEN (sizeof(NETDATA_MSYS_INSTALL_ROOT) - 1)

char *os_translate_msys_to_windows_path(const char *src) {
    if (!src || !*src)
        return strdupz("");

    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);

    // Worst case: prepend full MSYS root + separator + src + NUL.
    size_t buf_size = src_len + NETDATA_MSYS_INSTALL_ROOT_LEN + 2;
    char *converted_path = mallocz(buf_size);
    size_t i = 0;
    size_t j = 0;

    if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        // "C:..." (already Windows-form) -> normalize the drive letter.
        converted_path[j++] = (char)toupper((unsigned char)src[0]);
        converted_path[j++] = ':';
        i = 2;

        if (i < src_len && (src[i] == '\\' || src[i] == '/')) {
            converted_path[j++] = '\\';
            i++;
        }
    }
    else if (src_len >= 2 && src[0] == '/' && isalpha((unsigned char)src[1]) && (src_len == 2 || src[2] == '/')) {
        // "/c[/...]" -> "C:[\...]"
        converted_path[j++] = (char)toupper((unsigned char)src[1]);
        converted_path[j++] = ':';
        i = 2;

        if (i < src_len && src[i] == '/') {
            converted_path[j++] = '\\';
            i++;
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        // UNC: "\\\\server\\share" or "//server/share" -> "\\\\server\\share"
        converted_path[j++] = '\\';
        converted_path[j++] = '\\';
        i = 2;
    }
    else if (src[0] == '/') {
        // Root-relative POSIX path. Replace the Cygwin mount-table lookup
        // with a fixed prefix at the MSYS install root.
        memcpy(converted_path, NETDATA_MSYS_INSTALL_ROOT, NETDATA_MSYS_INSTALL_ROOT_LEN);
        j = NETDATA_MSYS_INSTALL_ROOT_LEN;
        converted_path[j++] = '\\';
        i = 1;
    }
    // else: relative path -- just normalize separators in the loop below.

    for (; i < src_len && j + 1 < buf_size; i++)
        converted_path[j++] = (src[i] == '/') ? '\\' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}

wchar_t *os_translate_msys_to_windows_pathW(const char *src) {
    CLEAN_CHAR_P *win_path = os_translate_msys_to_windows_path(src);
    if (!win_path)
        return NULL;

    int n = MultiByteToWideChar(CP_UTF8, 0, win_path, -1, NULL, 0);
    if (n <= 0)
        return NULL;

    wchar_t *wpath = mallocz((size_t)n * sizeof(wchar_t));
    if (MultiByteToWideChar(CP_UTF8, 0, win_path, -1, wpath, n) <= 0) {
        freez(wpath);
        return NULL;
    }

    return wpath;
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

char *os_translate_windows_to_msys_path(const char *src) {
    if (!src)
        return strdupz("");

    // Keep already POSIX-style paths unchanged.
    if (src[0] == '/')
        return strdupz(src);

    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    char *converted_path = mallocz(src_len + 3);
    size_t converted_size_fallback = src_len + 3;
    size_t i = 0;
    size_t j = 0;

    if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        converted_path[j++] = '/';
        converted_path[j++] = (char)tolower((unsigned char)src[0]);

        i = 2;
        if (src[i] == '\\' || src[i] == '/') {
            converted_path[j++] = '/';
            i++; // consume the separator so the loop below doesn't emit it again
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        converted_path[j++] = '/';
        converted_path[j++] = '/';
        i = 2;
    }

    for (; i < src_len && j < converted_size_fallback - 1; i++)
        converted_path[j++] = (src[i] == '\\') ? '/' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}
#endif
