// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

// nd_win_trace() is declared in libnetdata/os/os.h (Windows) / macro no-op (non-Windows).

// Normalize a directory path for Win32/UCRT64 API use.
// Accepts both POSIX form (/c/...) and Windows-native form (C:\... or C:/...);
// output is always C:/... with forward slashes.  Non-Windows: straight copy.
void nd_env_normalize_dir_path(const char *src, char *dst, size_t dst_size) {
    if (!src || !dst || !dst_size) return;

#if defined(OS_WINDOWS)
    if (isalpha((unsigned char)src[0]) && src[1] == ':') {
        // Windows-native: upper-case drive, \ → /
        if (dst_size < 4) { dst[0] = '\0'; return; }
        dst[0] = (char)toupper((unsigned char)src[0]);
        dst[1] = ':';
        if (src[2] == '\0') {
            // Bare "C:" → "C:/" (drive-relative otherwise)
            dst[2] = '/'; dst[3] = '\0';
        } else {
            size_t i = 2, j = 2;
            for (; src[i] && j < dst_size - 1; i++, j++)
                dst[j] = (src[i] == '\\') ? '/' : src[i];
            dst[j] = '\0';
        }
    } else if (src[1] && isalpha((unsigned char)src[1]) && (src[2] == '/' || src[2] == '\0')) {
        // POSIX (/c/... or bare /c): extract drive letter, keep forward slashes.
        if (dst_size < 4) { dst[0] = '\0'; return; }
        dst[0] = (char)toupper((unsigned char)src[1]);
        dst[1] = ':';
        if (src[2] == '\0') {
            dst[2] = '/'; dst[3] = '\0';
        } else {
            strncpyz(dst + 2, src + 2, dst_size - 2);
        }
    } else {
        strncpyz(dst, src, dst_size);
    }
#else
    strncpyz(dst, src, dst_size);
#endif
}

#if defined(OS_WINDOWS)
// mkdir -p for Windows native paths (C:/foo/bar/baz).
// Walk every '/' separator and create each prefix, ignoring failures
// (the parent may already exist).  The drive root "X:" is skipped
// because mkdir on a bare drive letter always fails.
void mkdir_recursive(const char *native_path, int perms) {
    char tmp[FILENAME_MAX + 1];
    strncpyz(tmp, native_path, FILENAME_MAX);

    // Start after the drive root ("X:/") so we never call mkdir on "X:".
    char *walk_start = tmp;
    if (isalpha((unsigned char)tmp[0]) && tmp[1] == ':' && (tmp[2] == '/' || tmp[2] == '\0'))
        walk_start = tmp + 2;

    // If walk_start is already at '\0' (empty path or bare drive letter), there
    // are no intermediate components to create.  Skip the loop entirely so that
    // walk_start + 1 is never read — that byte is uninitialized for such inputs.
    if (!*walk_start)
        return;

    for (char *p = walk_start + 1; *p; p++) {
        if (*p == '/') {
            char saved = *p;
            *p = '\0';
            mkdir(tmp, perms);  // ignore failure — may already exist
            *p = saved;
        }
    }
}
#endif

void verify_required_directory(const char *env, const char *dir, bool create_it, int perms) {
    errno_clear();

#if defined(OS_WINDOWS)
    // Accept both POSIX form (/c/...) and Windows-native form (C:\... or C:/...).
    // A bare "C:" (no trailing separator) is drive-relative on Windows — treat it
    // as the root by requiring at least a separator or end-of-string after "X:".
    bool is_absolute_path =
        (dir && dir[0] == '/') ||
        (dir && isalpha((unsigned char)dir[0]) && dir[1] == ':' &&
         (dir[2] == '\\' || dir[2] == '/' || dir[2] == '\0'));
#else
    bool is_absolute_path = (dir && dir[0] == '/');
#endif
    if (!dir || !is_absolute_path)
        fatal("Invalid directory path (must be an absolute path): '%s' (%s)", dir, env?env:"");

#if defined(OS_WINDOWS)
    char win_dir[FILENAME_MAX + 1];
    nd_env_normalize_dir_path(dir, win_dir, sizeof(win_dir));
    const char *native_dir = win_dir;
#else
    const char *native_dir = dir;
#endif

    bool dir_ok = (chdir(native_dir) == 0);

    if (!dir_ok && create_it) {
#if defined(OS_WINDOWS)
        // On Windows, the WiX installer only creates static content directories;
        // runtime directories like var/log/ may have no parent.  Create all
        // intermediate components before attempting the final mkdir.
        mkdir_recursive(native_dir, perms);
#endif
        // Accept the case where mkdir_recursive already created the leaf.
        if (mkdir(native_dir, perms) == 0 || chdir(native_dir) == 0)
            dir_ok = true;
    }

    if (dir_ok) {
        if(env)
            nd_setenv(env, dir, 1);
        return;
    }

    char path[PATH_MAX];
    strncpyz(path, native_dir, sizeof(path) - 1);
    struct stat st;

    char *p = path;
    while (*p) {
        if (p != path && *p == '/') {
            *p = '\0';

            errno_clear();
            if (stat(path, &st) == -1) {
                nd_win_trace("verify_required_directory FATAL: missing component '%s' of '%s' (env=%s)",
                             path, dir, env?env:"");
                fatal("Required directory: '%s' (%s) - Missing or inaccessible component: '%s'",
                      dir, env?env:"", path);
            }

            if (!S_ISDIR(st.st_mode)) {
                nd_win_trace("verify_required_directory FATAL: component '%s' not a dir (env=%s)",
                             path, env?env:"");
                fatal("Required directory: '%s' (%s) - Component '%s' exists but is not a directory.",
                      dir, env?env:"", path);
            }

            *p = '/';
        }
        p++;
    }

    if (stat(native_dir, &st) == -1) {
        nd_win_trace("verify_required_directory FATAL: dir '%s' not found (env=%s, native='%s')",
                     dir, env?env:"", native_dir);
        fatal("Required directory: '%s' (%s) - Missing or inaccessible: '%s'",
              dir, env?env:"", native_dir);
    }

    if (!S_ISDIR(st.st_mode)) {
        nd_win_trace("verify_required_directory FATAL: '%s' not a dir (env=%s)",
                     native_dir, env?env:"");
        fatal("Required directory: '%s' (%s) - '%s' exists but is not a directory.",
              dir, env?env:"", native_dir);
    }

    if (access(native_dir, R_OK | X_OK) == -1) {
        nd_win_trace("verify_required_directory FATAL: '%s' no read/exec access (env=%s)",
                     native_dir, env?env:"");
        fatal("Required directory: '%s' (%s) - Insufficient permissions for: '%s'",
              dir, env?env:"", native_dir);
    }

    nd_win_trace("verify_required_directory FATAL: '%s' unknown failure (env=%s)",
                 native_dir, env?env:"");
    fatal("Required directory: '%s' (%s) - Failed",
          dir, env?env:"");
}

void set_environment_for_plugins_and_scripts(void) {
    {
        char b[16];
        snprintfz(b, sizeof(b) - 1, "%d", (int)nd_profile.update_every);
        nd_setenv("NETDATA_UPDATE_EVERY", b, 1);
    }

    nd_setenv("NETDATA_VERSION", NETDATA_VERSION, 1);
    nd_setenv("NETDATA_HOSTNAME", netdata_configured_hostname, 1);
    nd_setenv("NETDATA_HOST_PREFIX", netdata_configured_host_prefix, 1);

    nd_win_trace("verify NETDATA_CONFIG_DIR / NETDATA_USER_CONFIG_DIR: '%s'", netdata_configured_user_config_dir);
    verify_required_directory("NETDATA_CONFIG_DIR", netdata_configured_user_config_dir, false, 0);
    nd_setenv("NETDATA_USER_CONFIG_DIR", netdata_configured_user_config_dir, 1);
    nd_win_trace("verify NETDATA_STOCK_CONFIG_DIR: '%s'", netdata_configured_stock_config_dir);
    verify_required_directory("NETDATA_STOCK_CONFIG_DIR", netdata_configured_stock_config_dir, false, 0);
    nd_win_trace("verify NETDATA_STOCK_DATA_DIR: '%s'", netdata_configured_stock_data_dir);
    verify_required_directory("NETDATA_STOCK_DATA_DIR", netdata_configured_stock_data_dir, false, 0);
    nd_win_trace("verify NETDATA_PLUGINS_DIR: '%s'", netdata_configured_primary_plugins_dir);
    verify_required_directory("NETDATA_PLUGINS_DIR", netdata_configured_primary_plugins_dir, false, 0);
    nd_win_trace("verify NETDATA_WEB_DIR: '%s'", netdata_configured_web_dir);
    verify_required_directory("NETDATA_WEB_DIR", netdata_configured_web_dir, false, 0);
    nd_win_trace("verify NETDATA_CACHE_DIR: '%s'", netdata_configured_cache_dir);
    verify_required_directory("NETDATA_CACHE_DIR", netdata_configured_cache_dir, true, 0775);
    nd_win_trace("verify NETDATA_LIB_DIR: '%s'", netdata_configured_varlib_dir);
    verify_required_directory("NETDATA_LIB_DIR", netdata_configured_varlib_dir, true, 0775);
    nd_win_trace("verify NETDATA_LOG_DIR: '%s'", netdata_configured_log_dir);
    verify_required_directory("NETDATA_LOG_DIR", netdata_configured_log_dir, true, 0775);
    nd_win_trace("verify CLAIMING_DIR: '%s'", netdata_configured_cloud_dir);
    verify_required_directory("CLAIMING_DIR", netdata_configured_cloud_dir, true, 0770);
    nd_win_trace("all directory checks passed");

    {
        BUFFER *user_plugins_dirs = buffer_create(FILENAME_MAX, NULL);

        for (size_t i = 1; i < PLUGINSD_MAX_DIRECTORIES && plugin_directories[i]; i++) {
            if (i > 1)
                buffer_strcat(user_plugins_dirs, " ");
            buffer_strcat(user_plugins_dirs, plugin_directories[i]);
        }

        nd_setenv("NETDATA_USER_PLUGINS_DIRS", buffer_tostring(user_plugins_dirs), 1);

        buffer_free(user_plugins_dirs);
    }

    const char *default_port = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "default port", NULL);
    int clean = 0;
    if (!default_port) {
        default_port = strdupz("19999");
        clean = 1;
    }

    nd_setenv("NETDATA_LISTEN_PORT", default_port, 1);
    if (clean)
        freez((char *)default_port);

    // set the path we need
    char path[4096];
    const char *p = getenv("PATH");
    if (!p) p = "/bin:/usr/bin";
    snprintfz(path, sizeof(path), "%s:%s", p, "/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin");
    setenv("PATH", inicfg_get_path_list(&netdata_config, CONFIG_SECTION_ENV_VARS, "PATH", path), 1);

    // python options
    p = getenv("PYTHONPATH");
    if (!p) p = "";
    setenv("PYTHONPATH", inicfg_get_path_list(&netdata_config, CONFIG_SECTION_ENV_VARS, "PYTHONPATH", p), 1);

    // disable buffering for python plugins
    setenv("PYTHONUNBUFFERED", "1", 1);

    // switch to standard locale for plugins
    setenv("LC_ALL", "C", 1);
}
