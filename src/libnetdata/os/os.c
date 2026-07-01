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
    char *converted_path = mallocz(src_len + 3);
    size_t i = 0;
    size_t j = 0;

    if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
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

        if (src[i] == '/') {
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

// Append a timestamped diagnostic line to %TEMP%\netdata-trace.log.
// Used during early Windows startup before the nd_log subsystem is ready.
// FILE_FLAG_OPEN_REPARSE_POINT: open the reparse point itself, not its target,
// so a symlink/junction planted at this path cannot redirect writes to another file.
//
// The file is opened once and kept open across calls. Opening and closing on every
// call triggers a Windows Defender content scan per call (~8 s each), which causes
// multi-minute startup hangs when nd_win_trace is called in a tight loop.
//
// A spinlock serialises concurrent callers so that simultaneous writes from two
// threads (e.g. CLEANUP and EXIT_WATCHER starting at the same millisecond) are
// not interleaved, and the one-time file open races cleanly.
void nd_win_trace(const char *fmt, ...) {
    static volatile LONG nd_win_trace_lock = 0;
    while (InterlockedCompareExchange(&nd_win_trace_lock, 1, 0) != 0)
        YieldProcessor();

    static FILE *f = NULL;
    if (!f) {
        char path[MAX_PATH + 1];
        DWORD len = GetTempPathA(MAX_PATH, path);
        if (!len || len >= (DWORD)(MAX_PATH - 22)) {
            InterlockedExchange(&nd_win_trace_lock, 0);
            return;
        }
        snprintfz(path + len, sizeof(path) - len, "netdata-trace.log");
        HANDLE h = CreateFileA(path, FILE_APPEND_DATA,
                               FILE_SHARE_READ | FILE_SHARE_WRITE,
                               NULL, OPEN_ALWAYS,
                               FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OPEN_REPARSE_POINT,
                               NULL);
        if (h == INVALID_HANDLE_VALUE) {
            InterlockedExchange(&nd_win_trace_lock, 0);
            return;
        }
        int fd = _open_osfhandle((intptr_t)h, _O_APPEND | _O_WRONLY | _O_TEXT);
        if (fd < 0) {
            CloseHandle(h);
            InterlockedExchange(&nd_win_trace_lock, 0);
            return;
        }
        f = _fdopen(fd, "a");
        if (!f) {
            _close(fd);
            InterlockedExchange(&nd_win_trace_lock, 0);
            return;
        }
    }
    SYSTEMTIME t;
    GetSystemTime(&t);
    fprintf(f, "%02d:%02d:%02d.%03d - ", t.wHour, t.wMinute, t.wSecond, t.wMilliseconds);
    va_list args;
    va_start(args, fmt);
    vfprintf(f, fmt, args);
    va_end(args);
    fputc('\n', f);
    fflush(f);
    // File is kept open; no fclose() here.

    InterlockedExchange(&nd_win_trace_lock, 0);
}
#endif
