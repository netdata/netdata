// SPDX-License-Identifier: GPL-3.0-or-later

#include "run_dir.h"
#include "libnetdata/libnetdata.h"

static char *cached_run_dir = NULL;
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

static inline bool is_dir_accessible(const char *dir, bool rw) {
    struct stat st;
    if (stat(dir, &st) == -1)
        return false;

    if (!S_ISDIR(st.st_mode))
        return false;

    // Check if we can write to the directory
    if (access(dir, rw ? W_OK : R_OK) == -1)
        return false;

    return true;
}

static inline bool netdata_dir_in_parent(const char *parent, char *out_path, size_t out_path_len, bool rw) {
    if (!is_dir_accessible(parent, rw))
        return false;

    snprintfz(out_path, out_path_len, "%s/netdata", parent);
    if (mkdir(out_path, 0755) == -1 && errno != EEXIST)
        return false;

    return is_dir_accessible(out_path, rw);
}

static char *detect_run_dir(bool rw) {
    char path[FILENAME_MAX + 1];

    if(!rw) {
        // First check for environment variable
        const char *env_dir = getenv("NETDATA_RUN_DIR");
        if (env_dir && *env_dir) {
            if (is_dir_accessible(env_dir, rw))
                return strdupz(env_dir);
        }
    }

#if defined(OS_LINUX)
    // First try /run/netdata
    if (netdata_dir_in_parent("/run", path, sizeof(path), rw))
        goto success;
#endif

#if defined(OS_MACOS)
    // macOS typically uses /private/var/run
    if (netdata_dir_in_parent("/private/var/run", path, sizeof(path), rw))
        goto success;
#endif

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
    // Then try /var/run/netdata
    if (netdata_dir_in_parent("/var/run", path, sizeof(path), rw))
        goto success;
#endif

//#if defined(OS_WINDOWS)
//    // On MSYS2/Cygwin get TEMP and convert it properly
//    WCHAR temp_pathW[MAX_PATH];
//    DWORD len = GetEnvironmentVariableW(L"TEMP", temp_pathW, MAX_PATH);
//    if (len > 0 && len < MAX_PATH) {
//        // Convert Windows wide path to UTF-8
//        int utf8_len = WideCharToMultiByte(CP_UTF8, 0, temp_pathW, -1, NULL, 0, NULL, NULL);
//        if (utf8_len > 0 && utf8_len < FILENAME_MAX) {
//            char win_path[FILENAME_MAX + 1];
//            if (WideCharToMultiByte(CP_UTF8, 0, temp_pathW, -1, win_path, sizeof(win_path), NULL, NULL)) {
//                // Convert Windows path to Unix path using Cygwin API
//                ssize_t unix_size = cygwin_conv_path(CCP_WIN_A_TO_POSIX, win_path, NULL, 0);
//                if (unix_size > 0) {
//                    char unix_path[FILENAME_MAX + 1];
//                    if (cygwin_conv_path(CCP_WIN_A_TO_POSIX, win_path, unix_path, sizeof(unix_path)) == 0) {
//                        if (is_dir_accessible(unix_path, rw)) {
//                            snprintfz(path, sizeof(path), "%s/netdata", unix_path);
//                            if (!rw)
//                                goto success;
//
//                            if (mkdir(path, 0755) == 0 || errno == EEXIST)
//                                goto success;
//                        }
//                    }
//                }
//            }
//        }
//    }
//#endif

    // Fallback to /tmp/netdata - force creation if needed
    if (!is_dir_accessible("/tmp", rw)) {
        // Try to create /tmp with standard permissions (including sticky bit)
        if (rw && mkdir("/tmp", 01777) == -1 && errno != EEXIST)
            return NULL;
    }

    snprintfz(path, sizeof(path), "/tmp/netdata");
    if (rw && mkdir(path, 0755) == -1 && errno != EEXIST)
        return NULL;

success:
    // Set the environment variable for child processes
    if(rw)
        nd_setenv("NETDATA_RUN_DIR", path, 1);

    return strdupz(path);
}

const char *os_run_dir(bool rw) {
    // Fast path - return cached directory if available
    if(cached_run_dir)
        return cached_run_dir;

    spinlock_lock(&spinlock);

    // Check again under lock in case another thread set it
    if(!cached_run_dir)
        cached_run_dir = detect_run_dir(rw);

    spinlock_unlock(&spinlock);

    return cached_run_dir;
}
