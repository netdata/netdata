// SPDX-License-Identifier: GPL-3.0-or-later

#include "paths.h"

static int is_procfs(const char *path, char **reason) {
#if defined(__APPLE__) || defined(__FreeBSD__)
    (void)path;
    (void)reason;
#else
    struct statfs stat;

    if (statfs(path, &stat) == -1) {
        if (reason)
            *reason = "failed to statfs()";
        return -1;
    }

#if defined PROC_SUPER_MAGIC
    if (stat.f_type != PROC_SUPER_MAGIC) {
        if (reason)
            *reason = "type is not procfs";
        return -1;
    }
#endif

#endif

    return 0;
}

static int is_sysfs(const char *path, char **reason) {
#if defined(__APPLE__) || defined(__FreeBSD__)
    (void)path;
    (void)reason;
#else
    struct statfs stat;

    if (statfs(path, &stat) == -1) {
        if (reason)
            *reason = "failed to statfs()";
        return -1;
    }

#if defined SYSFS_MAGIC
    if (stat.f_type != SYSFS_MAGIC) {
        if (reason)
            *reason = "type is not sysfs";
        return -1;
    }
#endif

#endif

    return 0;
}

int verify_netdata_host_prefix(bool log_msg) {
    if(!netdata_configured_host_prefix)
        netdata_configured_host_prefix = "";

    if(!*netdata_configured_host_prefix)
        return 0;

    char path[FILENAME_MAX];
    char *reason = "unknown reason";
    errno_clear();

    strncpyz(path, netdata_configured_host_prefix, sizeof(path) - 1);

    struct stat sb;
    if (stat(path, &sb) == -1) {
        reason = "failed to stat()";
        goto failed;
    }

    if((sb.st_mode & S_IFMT) != S_IFDIR) {
        errno = EINVAL;
        reason = "is not a directory";
        goto failed;
    }

    snprintfz(path, sizeof(path), "%s/proc", netdata_configured_host_prefix);
    if(is_procfs(path, &reason) == -1)
        goto failed;

    snprintfz(path, sizeof(path), "%s/sys", netdata_configured_host_prefix);
    if(is_sysfs(path, &reason) == -1)
        goto failed;

    if (netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        if (log_msg)
            netdata_log_info("Using host prefix directory '%s'", netdata_configured_host_prefix);
    }

    return 0;

failed:
    if (log_msg)
        netdata_log_error("Ignoring host prefix '%s': path '%s' %s", netdata_configured_host_prefix, path, reason);

    netdata_configured_host_prefix = "";
    return -1;
}

size_t filename_from_path_entry(char out[FILENAME_MAX], const char *path, const char *entry, const char *extension) {
    if(unlikely(!path || !*path)) path = ".";
    if(unlikely(!entry)) entry = "";

    // skip trailing slashes in path
    size_t len = strlen(path);
    while(len > 0 && path[len - 1] == '/') len--;

    // skip leading slashes in subpath
    while(entry[0] == '/') entry++;

    // if the last character in path is / and (there is a subpath or path is now empty)
    // keep the trailing slash in path and remove the additional slash
    char *slash = "/";
    if(path[len] == '/' && (*entry || len == 0)) {
        slash = "";
        len++;
    }
    else if(!*entry) {
        // there is no entry
        // no need for trailing slash
        slash = "";
    }

    return snprintfz(out, FILENAME_MAX, "%.*s%s%s%s%s", (int)len, path, slash, entry,
                     extension && *extension ? "." : "",
                     extension && *extension ? extension : "");
}

char *filename_from_path_entry_strdupz(const char *path, const char *entry) {
    char filename[FILENAME_MAX];
    filename_from_path_entry(filename, path, entry, NULL);
    return strdupz(filename);
}

bool filename_is_dir(const char *filename, bool create_it) {
    CLEAN_CHAR_P *buffer = NULL;

    size_t max_links = 100;

    bool is_dir = false;
    struct stat st;
    while(max_links && stat(filename, &st) == 0) {
        if ((st.st_mode & S_IFMT) == S_IFDIR)
            is_dir = true;
        else if ((st.st_mode & S_IFMT) == S_IFLNK) {
            max_links--;

            if(!buffer)
                buffer = mallocz(FILENAME_MAX);

            char link_dst[FILENAME_MAX];
            ssize_t l = readlink(filename, link_dst, FILENAME_MAX - 1);
            if (l > 0) {
                link_dst[l] = '\0';
                strncpyz(buffer, link_dst, FILENAME_MAX - 1);
                filename = buffer;
                continue;
            }
        }

        break;
    }

    if(!is_dir && create_it && max_links == 100 && mkdir(filename, 0750) == 0)
        is_dir = true;

    return is_dir;
}

bool path_entry_is_dir(const char *path, const char *entry, bool create_it) {
    char filename[FILENAME_MAX];
    filename_from_path_entry(filename, path, entry, NULL);
    return filename_is_dir(filename, create_it);
}

bool filename_is_file(const char *filename) {
    CLEAN_CHAR_P *buffer = NULL;

    size_t max_links = 100;

    bool is_file = false;
    struct stat st;
    while(max_links && stat(filename, &st) == 0) {
        if((st.st_mode & S_IFMT) == S_IFREG)
            is_file = true;
        else if((st.st_mode & S_IFMT) == S_IFLNK) {
            max_links--;

            if(!buffer)
                buffer = mallocz(FILENAME_MAX);

            char link_dst[FILENAME_MAX];
            ssize_t l = readlink(filename, link_dst, FILENAME_MAX - 1);
            if(l > 0) {
                link_dst[l] = '\0';
                strncpyz(buffer, link_dst, FILENAME_MAX - 1);
                filename = buffer;
                continue;
            }
        }

        break;
    }

    return is_file;
}

bool path_entry_is_file(const char *path, const char *entry) {
    char filename[FILENAME_MAX];
    filename_from_path_entry(filename, path, entry, NULL);
    return filename_is_file(filename);
}

void recursive_config_double_dir_load(const char *user_path, const char *stock_path, const char *entry, int (*callback)(const char *filename, void *data, bool stock_config), void *data, size_t depth) {
    if(depth > 3) {
        netdata_log_error("CONFIG: Max directory depth reached while reading user path '%s', stock path '%s', subpath '%s'", user_path, stock_path,
                          entry);
        return;
    }

    if(!stock_path)
        stock_path = user_path;

    char *udir = filename_from_path_entry_strdupz(user_path, entry);
    char *sdir = filename_from_path_entry_strdupz(stock_path, entry);

    netdata_log_debug(D_HEALTH, "CONFIG traversing user-config directory '%s', stock config directory '%s'", udir, sdir);

    DIR *dir = opendir(udir);
    if (!dir) {
        netdata_log_error("CONFIG cannot open user-config directory '%s'.", udir);
    }
    else {
        struct dirent *de = NULL;
        while((de = readdir(dir))) {
            if(de->d_type == DT_DIR || de->d_type == DT_LNK) {
                if( !de->d_name[0] ||
                    (de->d_name[0] == '.' && de->d_name[1] == '\0') ||
                    (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                ) {
                    netdata_log_debug(D_HEALTH, "CONFIG ignoring user-config directory '%s/%s'", udir, de->d_name);
                    continue;
                }

                if(path_entry_is_dir(udir, de->d_name, false)) {
                    recursive_config_double_dir_load(udir, sdir, de->d_name, callback, data, depth + 1);
                    continue;
                }
            }

            if(de->d_type == DT_UNKNOWN || de->d_type == DT_REG || de->d_type == DT_LNK) {
                size_t len = strlen(de->d_name);
                if(path_entry_is_file(udir, de->d_name) &&
                    len > 5 && !strcmp(&de->d_name[len - 5], ".conf")) {
                    char *filename = filename_from_path_entry_strdupz(udir, de->d_name);
                    netdata_log_debug(D_HEALTH, "CONFIG calling callback for user file '%s'", filename);
                    callback(filename, data, false);
                    freez(filename);
                    continue;
                }
            }

            netdata_log_debug(D_HEALTH, "CONFIG ignoring user-config file '%s/%s' of type %d", udir, de->d_name, (int)de->d_type);
        }

        closedir(dir);
    }

    netdata_log_debug(D_HEALTH, "CONFIG traversing stock config directory '%s', user config directory '%s'", sdir, udir);

    dir = opendir(sdir);
    if (!dir) {
        netdata_log_error("CONFIG cannot open stock config directory '%s'.", sdir);
    }
    else {
        if (strcmp(udir, sdir)) {
            struct dirent *de = NULL;
            while((de = readdir(dir))) {
                if(de->d_type == DT_DIR || de->d_type == DT_LNK) {
                    if( !de->d_name[0] ||
                        (de->d_name[0] == '.' && de->d_name[1] == '\0') ||
                        (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
                    ) {
                        netdata_log_debug(D_HEALTH, "CONFIG ignoring stock config directory '%s/%s'", sdir, de->d_name);
                        continue;
                    }

                    if(path_entry_is_dir(sdir, de->d_name, false)) {
                        // we recurse in stock subdirectory, only when there is no corresponding
                        // user subdirectory - to avoid reading the files twice

                        if(!path_entry_is_dir(udir, de->d_name, false))
                            recursive_config_double_dir_load(udir, sdir, de->d_name, callback, data, depth + 1);

                        continue;
                    }
                }

                if(de->d_type == DT_UNKNOWN || de->d_type == DT_REG || de->d_type == DT_LNK) {
                    size_t len = strlen(de->d_name);
                    if(path_entry_is_file(sdir, de->d_name) && !path_entry_is_file(udir, de->d_name) &&
                        len > 5 && !strcmp(&de->d_name[len - 5], ".conf")) {
                        char *filename = filename_from_path_entry_strdupz(sdir, de->d_name);
                        netdata_log_debug(D_HEALTH, "CONFIG calling callback for stock file '%s'", filename);
                        callback(filename, data, true);
                        freez(filename);
                        continue;
                    }

                }

                netdata_log_debug(D_HEALTH, "CONFIG ignoring stock-config file '%s/%s' of type %d", udir, de->d_name, (int)de->d_type);
            }
        }
        closedir(dir);
    }

    netdata_log_debug(D_HEALTH, "CONFIG done traversing user-config directory '%s', stock config directory '%s'", udir, sdir);

    freez(udir);
    freez(sdir);
}
