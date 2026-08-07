// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage and netdata.conf loaders for the netdata_configured_*
// globals. Compile-time defaults come from build/config.h via the
// libnetdata.h chain.

#include "../libnetdata.h"

// =====================================================================================================================
// Windows: derive runtime install prefix from binary location

#if defined(OS_WINDOWS)
static char *windows_install_prefix_unittest_override = NULL;

char *nd_windows_install_prefix_from_executable_path(const char *executable_path) {
    if (!executable_path)
        return NULL;

    // Netdata installs as <prefix>/usr/bin/netdata.exe, so stripping
    // three path components gives <prefix> (e.g. C:\Program Files\Netdata).
    char *install_prefix = strdupz(executable_path);

    // Normalize backslashes to forward slashes.
    // UCRT64's CRT (no msys-2.0.dll) accepts C:/... everywhere that
    // it accepts C:\... — forward slashes are valid Windows path separators.
    for (char *p = install_prefix; *p; p++)
        if (*p == '\\') *p = '/';

    // Strip three components: netdata.exe, bin, usr.
    for (int i = 0; i < 3; i++) {
        char *sep = strrchr(install_prefix, '/');
        if (!sep) {
            freez(install_prefix);
            return NULL;
        }
        *sep = '\0';
    }
    return install_prefix;
}

static bool nd_windows_directory_exists(const char *path) {
    int wpath_length = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path, -1, NULL, 0);
    if (wpath_length <= 0)
        return false;

    wchar_t *wpath = mallocz((size_t)wpath_length * sizeof(*wpath));
    DWORD attributes = INVALID_FILE_ATTRIBUTES;
    if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, path, -1, wpath, wpath_length) > 0)
        attributes = GetFileAttributesW(wpath);

    bool exists = attributes != INVALID_FILE_ATTRIBUTES && (attributes & FILE_ATTRIBUTE_DIRECTORY) != 0;
    freez(wpath);
    return exists;
}

static char *nd_windows_detect_install_prefix_once(void) {
    CLEAN_CHAR_P *exe_path = os_get_process_path();
    CLEAN_CHAR_P *install_prefix = nd_windows_install_prefix_from_executable_path(exe_path);
    if (!install_prefix)
        return NULL;

    // Validate that the expected config directory exists under this prefix
    // before committing to the override.
    size_t test_path_size = strlen(install_prefix) + sizeof("/etc/netdata");
    CLEAN_CHAR_P *test_path = mallocz(test_path_size);
    snprintfz(test_path, test_path_size, "%s/etc/netdata", install_prefix);
    if (!nd_windows_directory_exists(test_path))
        return NULL;

    return strdupz(install_prefix);
}

char *nd_windows_detect_install_prefix(void) {
    static volatile int initialized = 0;
    static char *install_prefix = NULL;

    if (windows_install_prefix_unittest_override)
        return strdupz(windows_install_prefix_unittest_override);

    if (__atomic_load_n(&initialized, __ATOMIC_ACQUIRE) != 2) {
        int expected = 0;
        if (__atomic_compare_exchange_n(&initialized, &expected, 1, false, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
            install_prefix = nd_windows_detect_install_prefix_once();
            __atomic_store_n(&initialized, 2, __ATOMIC_RELEASE);
        }
        else {
            while (__atomic_load_n(&initialized, __ATOMIC_ACQUIRE) != 2)
                ;
        }
    }

    return install_prefix ? strdupz(install_prefix) : NULL;
}

void nd_windows_set_install_prefix_for_unittest(const char *install_prefix) {
    freez(windows_install_prefix_unittest_override);
    windows_install_prefix_unittest_override = install_prefix ? strdupz(install_prefix) : NULL;
}

void nd_windows_detect_prefix_and_override_paths(void) {
    CLEAN_CHAR_P *install_prefix = nd_windows_detect_install_prefix();
    if (!install_prefix)
        return;

    // Override all runtime path globals with Windows-native form (C:/...).
    // reformat_path() in inicfg_api.c normalizes paths to this same form on every
    // inicfg_get_path() call, so the globals remain C:/... throughout the lifetime
    // of the process regardless of how the config file was authored.
    //
    // Small one-time startup leak: these allocations are later
    // overwritten by nd_runtime_paths_load_directories_from_inicfg() which
    // stores the value in the config intern pool and returns a different
    // pointer. The original pointers are orphaned but not freed. The
    // total leaked memory is a few hundred bytes on every startup — acceptable
    // for a long-running daemon.
#define SET_PATH(var, suffix) \
    do { \
        size_t _size = strlen(install_prefix) + sizeof(suffix); \
        char *_buf = mallocz(_size); \
        snprintfz(_buf, _size, "%s" suffix, install_prefix); \
        (var) = _buf; \
    } while(0)

    SET_PATH(netdata_configured_user_config_dir,     "/etc/netdata");
    SET_PATH(netdata_configured_stock_config_dir,    "/usr/lib/netdata/conf.d");
    SET_PATH(netdata_configured_stock_data_dir,      "/usr/share/netdata");
    SET_PATH(netdata_configured_log_dir,             "/var/log/netdata");
    SET_PATH(netdata_configured_primary_plugins_dir, "/usr/libexec/netdata/plugins.d");
    SET_PATH(netdata_configured_web_dir,             "/usr/share/netdata/web");
    SET_PATH(netdata_configured_cache_dir,           "/var/cache/netdata");
    SET_PATH(netdata_configured_varlib_dir,          "/var/lib/netdata");
    SET_PATH(netdata_configured_cloud_dir,           "/var/lib/netdata/cloud.d");
    SET_PATH(netdata_configured_home_dir,            "/var/lib/netdata");

#undef SET_PATH

    // Pre-create the run directory and advertise it via NETDATA_RUN_DIR
    // in the Windows-compatible C:/... form.  os_run_dir() calls stat()
    // directly without POSIX translation, so the path must be in a form
    // that UCRT64's CRT handles natively (C:/... works; /c/... does not).
    CLEAN_CHAR_P *run_parent = mallocz(strlen(install_prefix) + sizeof("/run"));
    CLEAN_CHAR_P *run_dir = mallocz(strlen(install_prefix) + sizeof("/run/netdata"));
    snprintfz(run_parent, strlen(install_prefix) + sizeof("/run"), "%s/run", install_prefix);
    snprintfz(run_dir, strlen(install_prefix) + sizeof("/run/netdata"), "%s/run/netdata", install_prefix);
    (void)mkdir(run_parent, 0755);
    (void)mkdir(run_dir,    0755);
    nd_setenv("NETDATA_RUN_DIR", run_dir, 1);
}
#endif

const char *netdata_configured_hostname            = NULL;
const char *netdata_configured_user_config_dir     = CONFIG_DIR;
const char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
const char *netdata_configured_stock_data_dir      = STOCK_DATA_DIR;
const char *netdata_configured_log_dir             = LOG_DIR;
const char *netdata_configured_primary_plugins_dir = PLUGINS_DIR;
const char *netdata_configured_web_dir             = WEB_DIR;
const char *netdata_configured_cache_dir           = CACHE_DIR;
const char *netdata_configured_varlib_dir          = VARLIB_DIR;
const char *netdata_configured_cloud_dir           = VARLIB_DIR "/cloud.d";
const char *netdata_configured_home_dir            = VARLIB_DIR;
const char *netdata_configured_host_prefix         = NULL;

// ----------------------------------------------------------------------------
// netdata.conf loaders

static const char *get_varlib_subdir_from_config(const char *prefix, const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/%s", prefix, dir);
    return inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, dir, filename);
}

void nd_runtime_paths_load_directories_from_inicfg(void) {
    FUNCTION_RUN_ONCE();

    netdata_configured_user_config_dir  = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "config",       netdata_configured_user_config_dir);
    netdata_configured_stock_config_dir = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "stock config", netdata_configured_stock_config_dir);
    netdata_configured_stock_data_dir   = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "stock data",   netdata_configured_stock_data_dir);
    netdata_configured_log_dir          = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "log",          netdata_configured_log_dir);
    netdata_configured_web_dir          = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "web",          netdata_configured_web_dir);
    netdata_configured_cache_dir        = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "cache",        netdata_configured_cache_dir);
    netdata_configured_varlib_dir       = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "lib",          netdata_configured_varlib_dir);
    netdata_configured_cloud_dir        = get_varlib_subdir_from_config(netdata_configured_varlib_dir, "cloud.d");
}

void nd_runtime_paths_load_hostname_from_inicfg(void) {
    FUNCTION_RUN_ONCE();

    netdata_configured_host_prefix = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "host access prefix", "");
    (void) verify_netdata_host_prefix(true);

    char buf[HOST_NAME_MAX * 4 + 1] = "";
    if (!os_hostname(buf, sizeof(buf), netdata_configured_host_prefix))
        netdata_log_error("Cannot get machine hostname.");

    netdata_configured_hostname = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "hostname", buf);
    netdata_log_debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);
}
