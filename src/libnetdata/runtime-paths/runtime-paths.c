// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage and netdata.conf loaders for the netdata_configured_*
// globals. Compile-time defaults come from build/config.h via the
// libnetdata.h chain.

#include "../libnetdata.h"

// =====================================================================================================================
// Windows: derive runtime install prefix from binary location

#if defined(OS_WINDOWS)
void nd_windows_detect_prefix_and_override_paths(void) {
    // Derive the install prefix from the running executable path.
    // Netdata installs as <prefix>/usr/bin/netdata.exe, so stripping
    // three path components gives <prefix> (e.g. C:\Program Files\Netdata).
    char exe_path[MAX_PATH + 1];
    DWORD len = GetModuleFileNameA(NULL, exe_path, MAX_PATH);
    if (len == 0 || len >= MAX_PATH)
        return;

    // Normalize backslashes to forward slashes.
    // UCRT64's CRT (no msys-2.0.dll) accepts C:/... everywhere that
    // it accepts C:\... — forward slashes are valid Windows path separators.
    for (char *p = exe_path; *p; p++)
        if (*p == '\\') *p = '/';

    // Strip three components: netdata.exe, bin, usr.
    for (int i = 0; i < 3; i++) {
        char *sep = strrchr(exe_path, '/');
        if (!sep)
            return;
        *sep = '\0';
    }
    // exe_path is now: C:/Program Files/Netdata

    // Validate that the expected config directory exists under this prefix
    // before committing to the override.
    char test_path[FILENAME_MAX + 1];
    snprintfz(test_path, FILENAME_MAX, "%s/etc/netdata", exe_path);
    DWORD attrs = GetFileAttributesA(test_path);
    if (attrs == INVALID_FILE_ATTRIBUTES || !(attrs & FILE_ATTRIBUTE_DIRECTORY))
        return;

    // Convert the Windows prefix (C:/...) to POSIX/MSYS2 form (/c/...).
    // The runtime-paths globals must be in POSIX form because the config
    // system's reformat_path() passes them through os_translate_windows_to_msys_path(),
    // which leaves paths already starting with '/' unchanged.  Using /c/...
    // here means the globals survive reformat_path() intact and are correctly
    // converted to Windows form (C:\...) by os_translate_msys_to_windows_path()
    // at actual file-system call sites (e.g. chdir in daemon/main.c).
    CLEAN_CHAR_P *posix_prefix = os_translate_windows_to_msys_path(exe_path);

    // Override all runtime path globals.
    //
    // Small one-time startup leak: these strdupz() allocations are later
    // overwritten by nd_runtime_paths_load_directories_from_inicfg() which
    // stores the value in the config intern pool and returns a different
    // pointer.  The strdupz() pointers are orphaned but not freed.  The
    // total leaked memory is a few hundred bytes on every startup — acceptable
    // for a long-running daemon.
#define SET_PATH(var, suffix) \
    do { \
        char _buf[FILENAME_MAX + 1]; \
        snprintfz(_buf, FILENAME_MAX, "%s" suffix, posix_prefix); \
        (var) = strdupz(_buf); \
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
    char run_parent[FILENAME_MAX + 1];
    char run_dir[FILENAME_MAX + 1];
    snprintfz(run_parent, FILENAME_MAX, "%s/run",         exe_path);
    snprintfz(run_dir,    FILENAME_MAX, "%s/run/netdata", exe_path);
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
