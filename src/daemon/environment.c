// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

static void set_required_environment(const char *name, const char *value) {
    if(nd_environment_set(name, value, true) != 0)
        fatal("Cannot publish required child environment variable '%s': %s", name, strerror(errno));
}

void verify_required_directory(const char *env, const char *dir, bool create_it, int perms) {
    errno_clear();

    if (!dir || *dir != '/')
        fatal("Invalid directory path (must be an absolute path): '%s' (%s)", dir, env?env:"");

    if (chdir(dir) == 0) {
        if(env)
            set_required_environment(env, dir);
        return;
    }

    if(create_it) {
        if(mkdir(dir, perms) == 0) {
            if(env)
                set_required_environment(env, dir);
            return;
        }
    }

    char path[PATH_MAX];
    strncpyz(path, dir, sizeof(path) - 1);
    struct stat st;

    char *p = path;
    while (*p) {
        if (p != path && *p == '/') {
            *p = '\0';

            errno_clear();
            if (stat(path, &st) == -1)
                fatal("Required directory: '%s' (%s) - Missing or inaccessible component: '%s'",
                      dir, env?env:"", path);

            if (!S_ISDIR(st.st_mode))
                fatal("Required directory: '%s' (%s) - Component '%s' exists but is not a directory.",
                      dir, env?env:"", path);

            *p = '/';
        }
        p++;
    }

    if (stat(dir, &st) == -1)
        fatal("Required directory: '%s' (%s) - Missing or inaccessible: '%s'",
              dir, env?env:"", dir);

    if (!S_ISDIR(st.st_mode))
        fatal("Required directory: '%s' (%s) - '%s' exists but is not a directory.",
              dir, env?env:"", dir);

    if (access(dir, R_OK | X_OK) == -1)
        fatal("Required directory: '%s' (%s) - Insufficient permissions for: '%s'",
              dir, env?env:"", dir);

    fatal("Required directory: '%s' (%s) - Failed",
          dir, env?env:"");
}

void set_environment_for_plugins_and_scripts(void) {
    {
        char b[16];
        snprintfz(b, sizeof(b) - 1, "%d", (int)nd_profile.update_every);
        set_required_environment("NETDATA_UPDATE_EVERY", b);
    }

    set_required_environment("NETDATA_VERSION", NETDATA_VERSION);
    set_required_environment("NETDATA_HOSTNAME", netdata_configured_hostname);
    set_required_environment("NETDATA_HOST_PREFIX", netdata_configured_host_prefix);

    verify_required_directory("NETDATA_CONFIG_DIR", netdata_configured_user_config_dir, false, 0);
    verify_required_directory("NETDATA_USER_CONFIG_DIR", netdata_configured_user_config_dir, false, 0);
    verify_required_directory("NETDATA_STOCK_CONFIG_DIR", netdata_configured_stock_config_dir, false, 0);
    verify_required_directory("NETDATA_STOCK_DATA_DIR", netdata_configured_stock_data_dir, false, 0);
    verify_required_directory("NETDATA_PLUGINS_DIR", netdata_configured_primary_plugins_dir, false, 0);
    verify_required_directory("NETDATA_WEB_DIR", netdata_configured_web_dir, false, 0);
    verify_required_directory("NETDATA_CACHE_DIR", netdata_configured_cache_dir, true, 0775);
    verify_required_directory("NETDATA_LIB_DIR", netdata_configured_varlib_dir, true, 0775);
    verify_required_directory("NETDATA_LOG_DIR", netdata_configured_log_dir, true, 0775);
    verify_required_directory("CLAIMING_DIR", netdata_configured_cloud_dir, true, 0770);

    {
        BUFFER *user_plugins_dirs = buffer_create(FILENAME_MAX, NULL);

        for (size_t i = 1; i < PLUGINSD_MAX_DIRECTORIES && plugin_directories[i]; i++) {
            if (i > 1)
                buffer_strcat(user_plugins_dirs, " ");
            buffer_strcat(user_plugins_dirs, plugin_directories[i]);
        }

        set_required_environment("NETDATA_USER_PLUGINS_DIRS", buffer_tostring(user_plugins_dirs));

        buffer_free(user_plugins_dirs);
    }

    const char *default_port = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "default port", NULL);
    int clean = 0;
    if (!default_port) {
        default_port = strdupz("19999");
        clean = 1;
    }

    set_required_environment("NETDATA_LISTEN_PORT", default_port);
    if (clean)
        freez((char *)default_port);

    // set the path we need
    char path[4096];
    CLEAN_CHAR_P *current_path = nd_environment_get_dup("PATH");
    snprintfz(path, sizeof(path), "%s:%s", current_path ? current_path : "/bin:/usr/bin",
              "/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin");
    set_required_environment(
        "PATH", inicfg_get_path_list(&netdata_config, CONFIG_SECTION_ENV_VARS, "PATH", path));

    // python options
    CLEAN_CHAR_P *python_path = nd_environment_get_dup("PYTHONPATH");
    set_required_environment(
        "PYTHONPATH",
        inicfg_get_path_list(&netdata_config, CONFIG_SECTION_ENV_VARS, "PYTHONPATH",
                             python_path ? python_path : ""));

    // disable buffering for python plugins
    set_required_environment("PYTHONUNBUFFERED", "1");

    // switch to standard locale for plugins
    set_required_environment("LC_ALL", "C");
}
