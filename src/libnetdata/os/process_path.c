// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)
#include <unistd.h>

static char *os_get_process_path_internal(void) {
    char path[PATH_MAX + 1] = "";
    ssize_t len = readlink("/proc/self/exe", path, PATH_MAX);

    if (len < 0) {
        // Error occurred; errno is set
        return NULL;
    }

    path[len] = '\0';  // readlink doesn't null terminate
    return strdupz(path);
}
#endif

#if defined(OS_FREEBSD)
#include <sys/sysctl.h>

static char *os_get_process_path_internal(void) {
    char path[PATH_MAX + 1] = "";
    int mib[4] = { CTL_KERN, KERN_PROC, KERN_PROC_PATHNAME, -1 };
    size_t len = sizeof(path);

    if (sysctl(mib, 4, path, &len, NULL, 0) == -1) {
        // Error occurred; errno is set
        return NULL;
    }

    return strdupz(path);
}
#endif

#if defined(OS_MACOS)
#include <mach-o/dyld.h>

static char *os_get_process_path_internal(void) {
    char path[PATH_MAX + 1] = "";
    uint32_t size = sizeof(path);

    if (_NSGetExecutablePath(path, &size) != 0) {
        // Buffer too small
        return NULL;
    }

    // Resolve any symlinks to get the real path
    char real_path[PATH_MAX + 1] = "";
    if (!realpath(path, real_path)) {
        // Error occurred; errno is set
        return NULL;
    }

    return strdupz(real_path);
}
#endif

#if defined(OS_WINDOWS)
#include <windows.h>

static char *os_get_process_path_internal(void) {
    wchar_t wpath[32768] = L"";  // Maximum path length in Windows
    DWORD length = GetModuleFileNameW(NULL, wpath, sizeof(wpath)/sizeof(wpath[0]));

    if (length == 0) {
        // GetModuleFileName failed
        return NULL;
    }

    // Convert wide string to UTF-8
    int utf8_len = WideCharToMultiByte(CP_UTF8, 0, wpath, -1, NULL, 0, NULL, NULL);
    if (utf8_len == 0) {
        // Conversion error
        return NULL;
    }

    char *path = mallocz(utf8_len);
    if (WideCharToMultiByte(CP_UTF8, 0, wpath, -1, path, utf8_len, NULL, NULL) == 0) {
        // Conversion error
        freez(path);
        return NULL;
    }

    return path;
}
#endif

char *os_get_process_path(void) {
    char b[FILENAME_MAX + 1];
    size_t b_size = sizeof(b) - 1;
    int ret = uv_exepath(b, &b_size);
    if(ret == 0)
        b[b_size] = '\0';

    if (ret != 0 || access(b, R_OK) != 0)
        return os_get_process_path_internal();

    b[b_size] = '\0';
    return strdupz(b);
}
