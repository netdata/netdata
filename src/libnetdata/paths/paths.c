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

// most consumers pass the host prefix as a %s argument, where a '%' would be
// harmless. these do not: they prepend it to a path template that is later
// used AS the format string, so a '%' in it becomes a conversion directive.
//
//   proc_diskstats.c  path_to_sys_block_device        "<prefix>/sys/block/%s"
//                     path_to_sys_block_device_bcache
//                     path_to_sys_devices_virtual_block_device
//                     path_to_sys_dev_block_major_minor_string
//   proc_stat.c       core_throttle_count_filename, package_throttle_count_filename,
//                     scaling_cur_freq_filename, time_in_state_filename,
//                     cpuidle_name_filename, cpuidle_time_filename
//   proc_net_dev.c    path_to_sys_devices_virtual_net, path_to_sys_class_net_speed,
//                     and the _duplex/_operstate/_carrier/_mtu variants
//   proc_mdstat.c     mismatch_cnt_filename
//
// each is consumed like snprintfz(buffer, FILENAME_MAX, path_to_sys_block_device, disk).
// a prefix carrying its own '%s' consumes that single argument early, leaving
// the template's real '%s' to dereference a vararg that was never passed;
// '%n' would be a write primitive. even a plain trailing '%' yields an
// undefined conversion ('%/'), which glibc and musl do not treat alike.
//
// a real host prefix never contains a '%'. supporting one would mean escaping
// '%' -> '%%' at every template site above, not relaxing this rule.
bool netdata_host_prefix_has_format_specifier(const char *prefix) {
    return prefix && strchr(prefix, '%') != NULL;
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

    // check the original, not the truncated copy in path[] - on success the
    // untruncated value is what stays in netdata_configured_host_prefix.
    if(netdata_host_prefix_has_format_specifier(netdata_configured_host_prefix)) {
        errno = EINVAL;
        reason = "contains '%'";
        goto failed;
    }

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
    bool is_dir = false;
    struct stat st;

    if(stat(filename, &st) == 0 && (st.st_mode & S_IFMT) == S_IFDIR)
        is_dir = true;

    if(!is_dir && create_it && mkdir(filename, 0750) == 0)
        is_dir = true;

    return is_dir;
}

bool path_entry_is_dir(const char *path, const char *entry, bool create_it) {
    char filename[FILENAME_MAX];
    filename_from_path_entry(filename, path, entry, NULL);
    return filename_is_dir(filename, create_it);
}

bool filename_is_file(const char *filename) {
    bool is_file = false;
    struct stat st;

    if(stat(filename, &st) == 0 && (st.st_mode & S_IFMT) == S_IFREG)
        is_file = true;

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

int paths_unittest(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

    // a prefix whose only '%' sits past the FILENAME_MAX truncation that
    // verify_netdata_host_prefix() applies internally. it must still be
    // rejected, because the check looks at the original string, not the copy.
    char long_prefix[FILENAME_MAX + 16];
    memset(long_prefix, 'a', sizeof(long_prefix));
    long_prefix[0] = '/';
    long_prefix[sizeof(long_prefix) - 3] = '%';
    long_prefix[sizeof(long_prefix) - 2] = 's';
    long_prefix[sizeof(long_prefix) - 1] = '\0';

    // "/" is accepted only where a real procfs and sysfs exist under it, which
    // minimal or chrooted environments may not have. probe with the same
    // predicates verify_netdata_host_prefix() uses, so the expectation is
    // computed rather than assumed and the case stays a real assertion in every
    // environment instead of testing host mount availability.
    bool host_mounts_available = is_procfs("/proc", NULL) == 0 && is_sysfs("/sys", NULL) == 0;

    // the host prefix ends up inside printf format strings, so a '%' in it must
    // never survive verification. "/" is here to prove the '%' check did not
    // start rejecting working prefixes.
    // expect_einval marks the cases that must be rejected by the '%' check
    // itself: none of those paths exist, so stat() would reject them anyway, and
    // without asserting the errno the whole table still passes with the '%'
    // check deleted.
    const struct {
        const char *name;
        const char *prefix;
        bool accept;
        bool expect_einval;
    } cases[] = {
        { "empty",            "",           true,                  false },
        { "root",             "/",          host_mounts_available, false },
        { "trailing %s",      "/host%s",    false,                 true  },
        { "bare %n",          "%n",         false,                 true  },
        { "embedded %",       "/a%b",       false,                 true  },
        { "escaped %%",       "/host%%",    false,                 true  },
        { "% past truncation", long_prefix, false,                 true  },
    };

    const char *saved = netdata_configured_host_prefix;
    int errors = 0;

    for(size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        netdata_configured_host_prefix = cases[i].prefix;
        errno_clear();
        int rc = verify_netdata_host_prefix(false);
        int err = errno;

        if((rc == 0) != cases[i].accept) {
            fprintf(stderr, "  FAILED %s: '%s' expected %s, got rc=%d\n",
                    cases[i].name, cases[i].prefix, cases[i].accept ? "accepted" : "rejected", rc);
            errors++;
            continue;
        }

        if(cases[i].accept) {
            // an accepted prefix must survive untouched
            if(strcmp(netdata_configured_host_prefix, cases[i].prefix) != 0) {
                fprintf(stderr, "  FAILED %s: '%s' accepted but prefix changed to '%s'\n",
                        cases[i].name, cases[i].prefix, netdata_configured_host_prefix);
                errors++;
            }
        }
        else {
            // a rejected prefix must be cleared, because runtime-paths.c
            // discards the return value and relies on this
            if(*netdata_configured_host_prefix) {
                fprintf(stderr, "  FAILED %s: '%s' rejected but prefix left as '%s'\n",
                        cases[i].name, cases[i].prefix, netdata_configured_host_prefix);
                errors++;
            }

            // assert the rejection came from the '%' check (EINVAL) and not
            // from stat() (ENOENT). "/" is exempt: when it lands here it was
            // rejected by the procfs/sysfs probes, which report their own errno.
            if(cases[i].expect_einval && err != EINVAL) {
                fprintf(stderr, "  FAILED %s: '%s' rejected by errno=%d, expected EINVAL (%d)\n",
                        cases[i].name, cases[i].prefix, err, EINVAL);
                errors++;
            }
        }
    }

    netdata_configured_host_prefix = saved;

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
}
