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
#if defined(__CYGWIN__) || defined(__MSYS__)
        ssize_t converted_size = cygwin_conv_path(CCP_POSIX_TO_WIN_A, src, NULL, 0);
        if (converted_size > 0) {
            char *converted_path = mallocz((size_t)converted_size);
            if (cygwin_conv_path(CCP_POSIX_TO_WIN_A, src, converted_path, (size_t)converted_size) == 0)
                return converted_path;

            freez(converted_path);
        }
#endif
    }

    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    bool package_relative_posix_path = false;
#if !defined(__CYGWIN__) && !defined(__MSYS__)
    package_relative_posix_path = src[0] == '/' &&
        !(src_len >= 2 && isalpha((unsigned char)src[1]) && (src_len == 2 || src[2] == '/')) &&
        !(src_len >= 2 && src[1] == '/');
#endif
    size_t prefix_len = package_relative_posix_path ? strlen(NETDATA_WINDOWS_PATH_PREFIX) : 0;
    size_t converted_size = prefix_len + src_len + 3;
    char *converted_path = mallocz(converted_size);
    size_t i = 0;
    size_t j = 0;

    if (package_relative_posix_path) {
        // UCRT64 has no POSIX mount table. Packaged paths such as /usr/share/netdata/web
        // are relative to the installation prefix, not the current drive root.
        for (; j < prefix_len; j++)
            converted_path[j] = (NETDATA_WINDOWS_PATH_PREFIX[j] == '/') ? '\\' : NETDATA_WINDOWS_PATH_PREFIX[j];
    }
    else if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        converted_path[j++] = (char)toupper((unsigned char)src[0]);
        converted_path[j++] = ':';
        i = 2;

        if (src[i] == '\\' || src[i] == '/') {
            converted_path[j++] = '\\';
            i++;
        }
    }
    else if (src_len >= 2 && src[0] == '/' && isalpha((unsigned char)src[1]) && (src_len == 2 || src[2] == '/')) {
        converted_path[j++] = (char)toupper((unsigned char)src[1]);
        converted_path[j++] = ':';
        i = 2;

        if (src_len == 2 || src[i] == '/') {
            converted_path[j++] = '\\';
            if (src[i] == '/')
                i++;
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        converted_path[j++] = '\\';
        converted_path[j++] = '\\';
        i = 2;
    }

    for (; i < src_len && j < converted_size - 1; i++)
        converted_path[j++] = (src[i] == '/') ? '\\' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}

wchar_t *os_translate_msys_to_windows_pathW(const char *src) {
    if (!src)
        return NULL;

#if defined(__CYGWIN__) || defined(__MSYS__)
    ssize_t cygwin_size = cygwin_conv_path(CCP_POSIX_TO_WIN_W, src, NULL, 0);
    if (cygwin_size > 0) {
        wchar_t *converted_path = mallocz((size_t)cygwin_size);
        if (cygwin_conv_path(CCP_POSIX_TO_WIN_W, src, converted_path, (size_t)cygwin_size) == 0)
            return converted_path;

        freez(converted_path);
    }

    // Absolute POSIX paths require the MSYS/Cygwin runtime's mount translation.
    if (src[0] == '/')
        return NULL;
#endif

    CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(src);
    int converted_size = MultiByteToWideChar(CP_UTF8, 0, translated, -1, NULL, 0);
    if (converted_size <= 0)
        return NULL;

    wchar_t *converted_path = mallocz((size_t)converted_size * sizeof(*converted_path));
    if (MultiByteToWideChar(CP_UTF8, 0, translated, -1, converted_path, converted_size) <= 0) {
        freez(converted_path);
        return NULL;
    }

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

int os_windows_path_translation_unittest(void) {
#if !defined(__CYGWIN__) && !defined(__MSYS__)
    static const struct {
        const char *input;
        const char *expected_suffix;
        bool use_package_prefix;
    } cases[] = {
        { "/usr/share/netdata/web", "\\usr\\share\\netdata\\web", true },
        { "/etc/netdata", "\\etc\\netdata", true },
        { "/c", "C:\\", false },
        { "/c/custom", "C:\\custom", false },
        { "C:/custom", "C:\\custom", false },
        { "//server/share", "\\\\server\\share", false },
    };

    int errors = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(cases[i].input);
        char expected[FILENAME_MAX + 1];
        if (cases[i].use_package_prefix)
            snprintfz(expected, sizeof(expected), "%s%s", NETDATA_WINDOWS_PATH_PREFIX, cases[i].expected_suffix);
        else
            snprintfz(expected, sizeof(expected), "%s", cases[i].expected_suffix);

        for (char *p = expected; *p; p++)
            if (*p == '/') *p = '\\';

        if (strcmp(translated, expected) != 0) {
            fprintf(stderr, "  FAILED path translation for '%s': expected '%s', got '%s'\n",
                    cases[i].input, expected, translated);
            errors++;
        }
    }

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
#else
    return 0;
#endif
}

char *os_translate_windows_to_msys_path(const char *src) {
    if (!src)
        return strdupz("");

    // Keep already POSIX-style paths unchanged.
    if (src[0] == '/')
        return strdupz(src);

#if defined(__CYGWIN__) || defined(__MSYS__)
    ssize_t converted_size = cygwin_conv_path(CCP_WIN_A_TO_POSIX, src, NULL, 0);
    if (converted_size > 0) {
        char *converted_path = mallocz((size_t)converted_size);
        if (cygwin_conv_path(CCP_WIN_A_TO_POSIX, src, converted_path, (size_t)converted_size) == 0)
            return converted_path;

        freez(converted_path);
    }
#endif

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
