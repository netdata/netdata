// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf.h"
#include "daemon/common.h"

// the files netdata_conf_load() reads, remembered for reloading sections later
static struct {
    char *primary;      // the -c file, or the user netdata.conf
    char *fallback;     // the stock netdata.conf, NULL when -c is given
    bool primary_loaded;
} netdata_conf_files = { 0 };

static bool netdata_conf_path_is_absolute(const char *path) {
#if defined(OS_WINDOWS)
    // "C:\x" is absolute, "C:x" is relative to the current directory of drive C
    if(*path == '\\' || (isalpha((uint8_t)path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/')))
        return true;
#endif
    return *path == '/';
}

// netdata changes into the user config dir later on, so a relative -c file has to be made absolute now
static char *netdata_conf_path_absolute_strdupz(const char *path) {
    if(netdata_conf_path_is_absolute(path))
        return strdupz(path);

    char cwd[FILENAME_MAX + 1];
    if(!getcwd(cwd, sizeof(cwd))) {
        netdata_log_error("CONFIG: cannot get the current directory, '%s' will be reloaded as given.", path);
        return strdupz(path);
    }

    return filename_from_path_entry_strdupz(cwd, path);
}

bool netdata_conf_load(char *filename, char overwrite_used, const char **user) {
    FUNCTION_RUN_ONCE_RET(false);

    errno_clear();

    int ret = 0;

    if(filename && *filename) {
        netdata_conf_files.primary = netdata_conf_path_absolute_strdupz(filename);

        ret = inicfg_load(&netdata_config, filename, overwrite_used, NULL);
        netdata_conf_files.primary_loaded = ret;
        if(!ret)
            netdata_log_error("CONFIG: cannot load config file '%s'.", filename);
    }
    else {
        netdata_conf_files.primary = filename_from_path_entry_strdupz(netdata_configured_user_config_dir, "netdata.conf");
        netdata_conf_files.fallback = filename_from_path_entry_strdupz(netdata_configured_stock_config_dir, "netdata.conf");

        ret = inicfg_load(&netdata_config, netdata_conf_files.primary, overwrite_used, NULL);
        netdata_conf_files.primary_loaded = ret;
        if(!ret) {
            netdata_log_info("CONFIG: cannot load user config '%s'. Will try the stock version.", netdata_conf_files.primary);

            ret = inicfg_load(&netdata_config, netdata_conf_files.fallback, overwrite_used, NULL);
            if(!ret)
                netdata_log_info("CONFIG: cannot load stock config '%s'. Running with internal defaults.", netdata_conf_files.fallback);
        }
    }

    netdata_conf_backwards_compatibility();
    netdata_conf_section_directories();
    netdata_conf_section_global_run_as_user(user);
    libuv_initialize();
    return ret;
}

bool netdata_conf_reload_section(const char *section) {
    if(inicfg_load(&netdata_config, netdata_conf_files.primary, 1, section)) {
        netdata_conf_files.primary_loaded = true;
        return true;
    }

    // the stock file stands in only for a user file that was never read: once it was, the stock
    // file (normally without the section) would wipe the values the user file gave
    bool try_fallback = netdata_conf_files.fallback && !netdata_conf_files.primary_loaded;
    if(try_fallback && inicfg_load(&netdata_config, netdata_conf_files.fallback, 1, section))
        return true;

    if(try_fallback)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "CONFIG: cannot reload section [%s] from '%s' or '%s', using the values in memory",
               section, netdata_conf_files.primary, netdata_conf_files.fallback);
    else
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "CONFIG: cannot reload section [%s] from '%s', using the values in memory",
               section, netdata_conf_files.primary);

    return false;
}
