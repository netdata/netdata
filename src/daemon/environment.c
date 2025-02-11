// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include <curl/curl.h>

static const char *verify_required_directory(const char *dir)
{
    if (chdir(dir) == -1)
        fatal("Cannot change directory to '%s'", dir);

    DIR *d = opendir(dir);
    if (!d)
        fatal("Cannot examine the contents of directory '%s'", dir);
    closedir(d);

    return dir;
}

static const char *verify_or_create_required_directory(const char *dir) {
    errno_clear();

    if (mkdir(dir, 0755) != 0 && errno != EEXIST)
        fatal("Cannot create required directory '%s'", dir);

    return verify_required_directory(dir);
}

static const char *verify_or_create_required_private_directory(const char *dir) {
    errno_clear();

    if (mkdir(dir, 0770) != 0 && errno != EEXIST)
        fatal("Cannot create required directory '%s'", dir);

    return verify_required_directory(dir);
}

const char *detect_libcurl_default_ca() {
#if CURLVERSION_NOW >= CURLVERSION_SEVENTH
    curl_version_info_data *info = curl_version_info(CURLVERSION_NOW);
    if (info) {
        // Check built-in CA bundle
        if (info->cainfo && access(info->cainfo, R_OK) == 0)
            return info->cainfo;
    }
#endif
    return NULL;
}

static const char *detect_ca_path(void) {
    static const char *paths[] = {
        "/etc/ssl/certs/ca-certificates.crt",               // Debian, Ubuntu, Arch
        "/etc/pki/tls/certs/ca-bundle.crt",                 // RHEL, CentOS, Fedora
        "/etc/ssl/ca-bundle.pem",                           // OpenSUSE
        "/etc/ssl/cert.pem",                                // Alpine
        "/opt/netdata/etc/ssl/certs/ca-certificates.crt",   // Netdata static build
        "/opt/netdata/share/ssl/certs/ca-certificates.crt", // Netdata static build - fallback
        NULL
    };

    for (int i = 0; paths[i] != NULL; i++) {
        if (access(paths[i], R_OK) == 0)
            return paths[i];
    }

    return NULL;
}

void set_environment_for_plugins_and_scripts(void) {
    {
        char b[16];
        snprintfz(b, sizeof(b) - 1, "%d", (int)nd_profile.update_every);
        nd_setenv("NETDATA_UPDATE_EVERY", b, 1);
    }

    nd_setenv("NETDATA_VERSION", NETDATA_VERSION, 1);
    nd_setenv("NETDATA_HOSTNAME", netdata_configured_hostname, 1);
    nd_setenv("NETDATA_CONFIG_DIR", verify_required_directory(netdata_configured_user_config_dir), 1);
    nd_setenv("NETDATA_USER_CONFIG_DIR", verify_required_directory(netdata_configured_user_config_dir), 1);
    nd_setenv("NETDATA_STOCK_CONFIG_DIR", verify_required_directory(netdata_configured_stock_config_dir), 1);
    nd_setenv("NETDATA_PLUGINS_DIR", verify_required_directory(netdata_configured_primary_plugins_dir), 1);
    nd_setenv("NETDATA_WEB_DIR", verify_required_directory(netdata_configured_web_dir), 1);
    nd_setenv("NETDATA_CACHE_DIR", verify_or_create_required_directory(netdata_configured_cache_dir), 1);
    nd_setenv("NETDATA_LIB_DIR", verify_or_create_required_directory(netdata_configured_varlib_dir), 1);
    nd_setenv("NETDATA_LOCK_DIR", verify_or_create_required_directory(netdata_configured_lock_dir), 1);
    nd_setenv("NETDATA_LOG_DIR", verify_or_create_required_directory(netdata_configured_log_dir), 1);
    nd_setenv("NETDATA_HOST_PREFIX", netdata_configured_host_prefix, 1);

    nd_setenv("CLAIMING_DIR", verify_or_create_required_private_directory(netdata_configured_cloud_dir), 1);

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

    p = getenv("CURL_CA_BUNDLE");
    if(!p || !*p) p = detect_libcurl_default_ca();
    if(!p || !*p) p = detect_ca_path();
    setenv("CURL_CA_BUNDLE", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "CURL_CA_BUNDLE", p ? p : ""), 1);

    // switch to standard locale for plugins
    setenv("LC_ALL", "C", 1);
}
