// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

void verify_required_directory(const char *env, const char *dir, bool create_it, int perms) {
    errno_clear();

    if (!dir || *dir != '/')
        fatal("Invalid directory path (must be an absolute path): '%s' (%s)", dir, env?env:"");

    if (chdir(dir) == 0) {
        if(env)
            nd_setenv(env, dir, 1);
        return;
    }

    if(create_it) {
        if(mkdir(dir, perms) == 0) {
            if(env)
                nd_setenv(env, dir, 1);
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
        nd_setenv("NETDATA_UPDATE_EVERY", b, 1);
    }

    nd_setenv("NETDATA_VERSION", NETDATA_VERSION, 1);
    nd_setenv("NETDATA_HOSTNAME", netdata_configured_hostname, 1);
    nd_setenv("NETDATA_HOST_PREFIX", netdata_configured_host_prefix, 1);

    verify_required_directory("NETDATA_CONFIG_DIR", netdata_configured_user_config_dir, false, 0);
    verify_required_directory("NETDATA_USER_CONFIG_DIR", netdata_configured_user_config_dir, false, 0);
    verify_required_directory("NETDATA_STOCK_CONFIG_DIR", netdata_configured_stock_config_dir, false, 0);
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
    setenv("PATH", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "PATH", path), 1);

    // python options
    p = getenv("PYTHONPATH");
    if (!p) p = "";
    setenv("PYTHONPATH", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "PYTHONPATH", p), 1);

    // disable buffering for python plugins
    setenv("PYTHONUNBUFFERED", "1", 1);

    // switch to standard locale for plugins
    setenv("LC_ALL", "C", 1);
}
