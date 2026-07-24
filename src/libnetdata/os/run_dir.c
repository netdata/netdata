// SPDX-License-Identifier: GPL-3.0-or-later

#include "run_dir.h"
#include "libnetdata/libnetdata.h"

static char *cached_run_dir = NULL;
static bool cached_run_dir_available = false;
static bool cached_run_dir_published = false;
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

static inline bool is_dir_accessible(const char *dir, bool rw) {
    struct stat st;
    if (stat(dir, &st) == -1)
        return false;

    if (!S_ISDIR(st.st_mode))
        return false;

    // Directory access always requires search permission.
    if (access(dir, (rw ? W_OK : R_OK) | X_OK) == -1)
        return false;

    return true;
}

static bool make_dir_writable(const char *dir) {
    struct stat st;
    if(stat(dir, &st) == -1) {
        if(errno != ENOENT)
            return false;

        if(mkdir(dir, 0755) == -1 && errno != EEXIST)
            return false;

        if(stat(dir, &st) == -1)
            return false;
    }

    if(!S_ISDIR(st.st_mode)) {
        errno = ENOTDIR;
        return false;
    }

    return access(dir, W_OK | X_OK) == 0;
}

static inline bool netdata_dir_in_parent(const char *parent, char *out_path, size_t out_path_len, bool rw) {
    int ret = snprintf(out_path, out_path_len, "%s/netdata", parent);
    if (ret < 0 || (size_t)ret >= out_path_len)
        return false;

    if (is_dir_accessible(out_path, rw))
        return true;

    if (!is_dir_accessible(parent, rw))
        return false;

    if (mkdir(out_path, 0755) == -1 && errno != EEXIST)
        return false;

    return is_dir_accessible(out_path, rw);
}

static char *detect_run_dir(bool rw) {
    char path[FILENAME_MAX + 1];

    CLEAN_CHAR_P *env_dir = nd_environment_get_dup("NETDATA_RUN_DIR");
    if (env_dir && *env_dir) {
        if (is_dir_accessible(env_dir, rw))
            return strdupz(env_dir);
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
    return strdupz(path);
}

const char *os_run_dir(bool rw) {
    // Fast path - return cached directory if available
    if(__atomic_load_n(&cached_run_dir_available, __ATOMIC_ACQUIRE) &&
       (!rw || __atomic_load_n(&cached_run_dir_published, __ATOMIC_ACQUIRE)))
        return cached_run_dir;

    spinlock_lock(&spinlock);

    // Check again under lock in case another thread set it
    char *run_dir = cached_run_dir;
    if(!run_dir) {
        run_dir = detect_run_dir(rw);
        cached_run_dir = run_dir;

        if(run_dir)
            __atomic_store_n(&cached_run_dir_available, true, __ATOMIC_RELEASE);
    }

    if(run_dir && rw && !__atomic_load_n(&cached_run_dir_published, __ATOMIC_RELAXED)) {
        if(!make_dir_writable(run_dir))
            run_dir = NULL;
        else if(nd_environment_set("NETDATA_RUN_DIR", run_dir, true) == 0)
            __atomic_store_n(&cached_run_dir_published, true, __ATOMIC_RELEASE);
        else
            run_dir = NULL;
    }

    int saved_errno = errno;
    spinlock_unlock(&spinlock);

    if(!run_dir) {
        errno = saved_errno ? saved_errno : EACCES;
        return NULL;
    }

    errno_clear();
    return run_dir;
}
