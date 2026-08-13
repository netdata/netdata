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
//
// this rule only covers the prefix. the files listed above read their template
// from netdata.conf (inicfg_get(), with the prefix-built path as the default),
// so a config value replaces the format string outright and never reaches this
// check - proc_diskstats.c:1371-1380, proc_stat.c:552-577, proc_mdstat.c:121.
//
// proc_net_dev.c used to be on this list; it now passes the prefix as a plain
// %s argument instead of baking it into a template, which is the shape to
// prefer for any new site.
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

static int close_keeping_errno(int fd) {
    int saved_errno = errno;
    close(fd);
    errno = saved_errno;
    return -1;
}

// Opens filename for writing and returns a file descriptor whose mode is 0600,
// or -1 with errno set. When the file exists, is accessible to group or other,
// and cannot be chmod()-ed, *replace_it is set: the caller may then remove the
// file and retry with O_EXCL.
static int fopen_secret_open_fd(const char *filename, const char *mode, int extra_flags, bool *replace_it) {
    *replace_it = false;

    // O_CREAT's mode applies only when the file is created, and umask can still
    // clear bits from it, so fchmod() after the open is what actually pins 0600
    // - it also fixes a file left behind with wider permissions by an older
    // agent, or by fopen() before this helper existed.
    //
    // Deliberately no O_TRUNC: we truncate only after the mode is confirmed, so
    // that a file we end up refusing keeps its previous content instead of being
    // replaced by an empty one.
    int flags = O_WRONLY | O_CREAT | O_CLOEXEC | extra_flags;

    // The directory holding these secrets is group-writable (cloud.d is 0770), so
    // the final path component is attacker-controllable: without protection we
    // would chmod and truncate whatever a planted symlink points at.
#ifdef O_NOFOLLOW
    flags |= O_NOFOLLOW;
#else
    // Platforms without O_NOFOLLOW (the Windows build) have to check the path
    // themselves: refuse a symlink up front, and refuse anything that is no
    // longer the object we inspected once the open() returns. Same approach as
    // daemon/status-file-io.c.
    struct stat before;
    bool have_before = lstat(filename, &before) == 0;
    if(have_before) {
#ifdef S_ISLNK
        if(S_ISLNK(before.st_mode)) {
            errno = ELOOP; // what O_NOFOLLOW would have returned
            return -1;
        }
#endif
        // a symlink also fails this check, so the refusal holds even where the
        // platform has no S_ISLNK - only the reported errno differs
        if(!S_ISREG(before.st_mode)) {
            errno = EINVAL;
            return -1;
        }
    }
    else {
        if(errno != ENOENT)
            return -1;

        // nothing is there, so require that we are the ones creating it: anything
        // planted between the lstat() and the open() then fails with EEXIST
        // instead of being followed
        flags |= O_EXCL;
    }
#endif

#ifdef O_NONBLOCK
    // if a FIFO was planted at the path, fail instead of blocking until a reader
    // appears. POSIX makes this a no-op for regular files, so the returned stream
    // behaves exactly as before for real secrets.
    flags |= O_NONBLOCK;
#endif

#ifdef O_BINARY
    // where text/binary modes exist, the descriptor decides newline translation,
    // so honour the caller's "b" the way the fopen() calls this replaced did
    if(mode && strchr(mode, 'b'))
        flags |= O_BINARY;
#endif

    int fd = open(filename, flags, 0600);
    if(fd == -1)
        return -1;

    struct stat st;
    if(fstat(fd, &st) != 0)
        return close_keeping_errno(fd);

    // never write a secret into a FIFO, device, or anything else a third party
    // could be reading from the other end of.
    if(!S_ISREG(st.st_mode)) {
        close(fd);
        errno = EINVAL;
        return -1;
    }

#ifndef O_NOFOLLOW
    // the path was swapped between the lstat() above and this open()
    if(have_before && (before.st_dev != st.st_dev || before.st_ino != st.st_ino)) {
        close(fd);
        errno = ELOOP;
        return -1;
    }
#endif

    if(fchmod(fd, 0600) != 0) {
        int saved_errno = errno;

        // fchmod() is gated on ownership, not on write access, so a secret left
        // behind by an agent that ran as root is writable through the netdata
        // group yet cannot be tightened. Some filesystems (CIFS, several FUSE and
        // bind mounts) also refuse chmod outright.
        if(st.st_mode & 0077) {
            // it is exposed and we cannot fix it in place - ask for a replacement
            close(fd);
            *replace_it = true;
            errno = saved_errno;
            return -1;
        }

        // nothing is exposed, so keep going rather than failing the write
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Cannot set mode 0600 on '%s' (%s), but it is already accessible only by its owner - continuing.",
               filename, strerror(saved_errno));
    }

    return fd;
}

FILE *fopen_secret_write(const char *filename, const char *mode) {
    bool replace_it = false;
    int fd = fopen_secret_open_fd(filename, mode, 0, &replace_it);

    if(fd == -1 && replace_it) {
        // The file is group or other accessible and un-chmod-able, so writing the
        // secret into it would expose it. The secret directories are writable by
        // the netdata group, so replace the file instead of failing permanently -
        // this is a truncating open, so its old content is discarded either way.
        int chmod_errno = errno;

        if(unlink(filename) != 0) {
            // keep unlink()'s errno, it is the actionable one: EBUSY for a secret
            // that is a single-file bind mount, EROFS for a read-only directory
            int unlink_errno = errno;
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "Cannot make '%s' private (%s) and cannot replace it (%s) - "
                   "refusing to write a secret into a file readable by others.",
                   filename, strerror(chmod_errno), strerror(unlink_errno));
            errno = unlink_errno;
            return NULL;
        }

        // O_EXCL: if anything recreated the path in the meantime, fail instead of
        // writing the secret into someone else's file
        fd = fopen_secret_open_fd(filename, mode, O_EXCL, &replace_it);
    }

    if(fd == -1)
        return NULL;

    // fdopen() before ftruncate(): a failure here must leave the previous content
    // alone, the same way every refusal above does
    FILE *fp = fdopen(fd, mode);
    if(!fp) {
        close_keeping_errno(fd);
        return NULL;
    }

    // now that the mode is pinned, make it behave like fopen(filename, "w")
    if(ftruncate(fd, 0) != 0) {
        int saved_errno = errno;
        fclose(fp);
        errno = saved_errno;
        return NULL;
    }

    return fp;
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

// Checks for fopen_secret_write(). Lives here, next to the implementation, so
// `netdata -W unittest` runs it in CI - the standalone target only wraps it.
//
// Every check runs under umask(0022) on purpose: plain fopen() would then produce
// 0644, so an assertion of 0600 only passes if the helper's own fchmod() did the
// work. A permissive umask is what makes these able to tell the helper apart from
// fopen(). The process umask is restored before returning.
//
// Not covered here, because an unprivileged test process cannot construct the
// precondition (a file it does not own, so that fchmod() fails): the
// unlink-and-recreate path and the already-owner-only tolerance path. Those are
// exercised out of tree - see .agents/sow/q/ for the docker recipe.
int fopen_secret_write_unittest(void) {
    fprintf(stderr, "%s() running...\n", __FUNCTION__);

#if defined(OS_WINDOWS)
    // POSIX mode bits, symlinks and FIFOs do not carry their intended meaning here
    fprintf(stderr, "%s() skipped on windows\n", __FUNCTION__);
    return 0;
#else
    int errors = 0;
    mode_t saved_umask = umask(0022);

    char dir[] = "/tmp/nd-fopen-secret-write-XXXXXX";
    if(!mkdtemp(dir)) {
        fprintf(stderr, "  FAILED cannot create temp directory: %s\n", strerror(errno));
        umask(saved_umask);
        return 1;
    }

    char created[FILENAME_MAX + 1], control[FILENAME_MAX + 1], preexisting[FILENAME_MAX + 1];
    char victim[FILENAME_MAX + 1], link[FILENAME_MAX + 1], fifo[FILENAME_MAX + 1], missing[FILENAME_MAX + 1];
    snprintfz(created, sizeof(created), "%s/created", dir);
    snprintfz(control, sizeof(control), "%s/control", dir);
    snprintfz(preexisting, sizeof(preexisting), "%s/preexisting", dir);
    snprintfz(victim, sizeof(victim), "%s/victim", dir);
    snprintfz(link, sizeof(link), "%s/link", dir);
    snprintfz(fifo, sizeof(fifo), "%s/fifo", dir);
    snprintfz(missing, sizeof(missing), "%s/no-such-directory/file", dir);

    struct stat st;
    FILE *fp;

    // a new file must be created 0600 even though umask(0022) would allow 0644
    fp = fopen_secret_write(created, "w");
    if(!fp) {
        fprintf(stderr, "  FAILED cannot create '%s': %s\n", created, strerror(errno));
        errors++;
    }
    else {
        fprintf(fp, "secret");
        if(fclose(fp) != 0) {
            fprintf(stderr, "  FAILED cannot write '%s'\n", created);
            errors++;
        }

        if(stat(created, &st) != 0 || (st.st_mode & 0777) != 0600) {
            fprintf(stderr, "  FAILED new secret is mode %o, expected 600\n",
                    stat(created, &st) == 0 ? (unsigned)(st.st_mode & 0777) : 0);
            errors++;
        }
    }

    // control: plain fopen() under the same umask must NOT be 0600 - this is what
    // proves the assertion above tests the helper and not the umask
    fp = fopen(control, "w");
    if(!fp) {
        fprintf(stderr, "  FAILED control: cannot create '%s'\n", control);
        errors++;
    }
    else {
        fclose(fp);
        if(stat(control, &st) != 0 || (st.st_mode & 0777) != 0644) {
            fprintf(stderr, "  FAILED control: plain fopen() is mode %o, expected 644 - "
                            "these checks would prove nothing\n",
                    stat(control, &st) == 0 ? (unsigned)(st.st_mode & 0777) : 0);
            errors++;
        }
    }

    // a file an older agent left at 0660 must be tightened, and a shorter secret
    // must not leave a tail of the longer one behind
    fp = fopen(preexisting, "w");
    if(!fp || fprintf(fp, "a-long-previous-secret") < 0 || fclose(fp) != 0 || chmod(preexisting, 0660) != 0) {
        fprintf(stderr, "  FAILED cannot set up the pre-existing 0660 secret\n");
        errors++;
    }
    else {
        fp = fopen_secret_write(preexisting, "w");
        if(!fp) {
            fprintf(stderr, "  FAILED cannot reopen '%s': %s\n", preexisting, strerror(errno));
            errors++;
        }
        else {
            fprintf(fp, "short");
            fclose(fp);

            char buf[128] = "";
            if(stat(preexisting, &st) != 0 || (st.st_mode & 0777) != 0600) {
                fprintf(stderr, "  FAILED pre-existing 0660 secret was not repaired to 600\n");
                errors++;
            }
            if(read_txt_file(preexisting, buf, sizeof(buf)) != 0 || strcmp(buf, "short") != 0) {
                fprintf(stderr, "  FAILED reopening did not truncate like fopen(.., \"w\")\n");
                errors++;
            }
        }
    }

    // the secret directories are group-writable, so a symlink planted at the path
    // must be refused - neither the target's mode nor its content may change
    fp = fopen(victim, "w");
    if(!fp || fprintf(fp, "victim") < 0 || fclose(fp) != 0 || symlink(victim, link) != 0) {
        fprintf(stderr, "  FAILED cannot set up the symlink case\n");
        errors++;
    }
    else {
        fp = fopen_secret_write(link, "w");
        if(fp) {
            fprintf(stderr, "  FAILED a symlinked secret path was accepted\n");
            fclose(fp);
            errors++;
        }

        if(stat(victim, &st) != 0 || (st.st_mode & 0777) != 0644 || st.st_size != 6) {
            fprintf(stderr, "  FAILED refusing a symlink changed its target\n");
            errors++;
        }
    }

    // a planted FIFO must be refused, not written into and not blocked on
    if(mkfifo(fifo, 0600) != 0) {
        fprintf(stderr, "  FAILED cannot set up the fifo case\n");
        errors++;
    }
    else {
        fp = fopen_secret_write(fifo, "w");
        if(fp) {
            fprintf(stderr, "  FAILED a non-regular secret path was accepted\n");
            fclose(fp);
            errors++;
        }
    }

    // failure must be reported as NULL with errno set
    errno = 0;
    fp = fopen_secret_write(missing, "w");
    if(fp) {
        fprintf(stderr, "  FAILED a secret in a missing directory was accepted\n");
        fclose(fp);
        errors++;
    }
    else if(errno != ENOENT) {
        fprintf(stderr, "  FAILED errno is %d, expected ENOENT (%d)\n", errno, ENOENT);
        errors++;
    }

    // best effort cleanup
    DIR *d = opendir(dir);
    if(d) {
        struct dirent *de;
        char buf[FILENAME_MAX + 1];
        while((de = readdir(d))) {
            if(!strcmp(de->d_name, ".") || !strcmp(de->d_name, ".."))
                continue;
            snprintfz(buf, sizeof(buf), "%s/%s", dir, de->d_name);
            unlink(buf);
        }
        closedir(d);
    }
    rmdir(dir);

    umask(saved_umask);

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
#endif
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

    errors += fopen_secret_write_unittest();

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
}
