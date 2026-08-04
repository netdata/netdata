// SPDX-License-Identifier: GPL-3.0-or-later

#include "run_dir.h"
#include "libnetdata/libnetdata.h"

static char *cached_run_dir = NULL;
static bool cached_run_dir_available = false;
static bool cached_run_dir_writable = false; // the cached answer satisfied a writable request

// A read-only answer that has already been handed out, superseded by a writable
// one. Retained, not freed: callers are allowed to keep the pointer they got,
// which used to live for the whole process. At most one string per process.
static char *superseded_run_dir = NULL;

static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

static uid_t target_uid = (uid_t)-1;

void os_run_dir_set_target_uid(uid_t uid) {
    __atomic_store_n(&target_uid, uid, __ATOMIC_RELEASE);
}

// The accounts a run directory may belong to without it being somebody else's:
// root, whoever we are now, and the uid we are about to become. getuid() is
// checked as well as geteuid() because the setuid-root plugins (apps.plugin,
// network-viewer.plugin, ...) run with euid 0 and the netdata uid as their real
// uid, and must accept the same run dir the daemon created.
static inline bool run_dir_owner_is_ours(uid_t owner) {
    if (owner == 0 || owner == geteuid() || owner == getuid())
        return true;

    uid_t target = __atomic_load_n(&target_uid, __ATOMIC_ACQUIRE);
    return target != (uid_t)-1 && owner == target;
}

// Parent-directory check. Follows symlinks intentionally: parents such as
// /var/run are frequently a symlink to /run on modern systems, so rejecting a
// symlinked parent would break the /var/run branch everywhere.
static inline bool is_dir_accessible(const char *dir, bool rw) {
    struct stat st;
    if (stat(dir, &st) == -1)
        return false;

    if (!S_ISDIR(st.st_mode))
        return false;

    // Check if we can write to the directory
    if (access(dir, rw ? W_OK : R_OK) == -1)
        return false;

    return true;
}

// Run-directory (leaf) check. netdata binds its sockets here and chowns it while
// still running as root, so this directory must not be one another local user
// controls. Unlike the parent check above it uses lstat(),
// so intermediate symlinks (/var/run -> /run) still resolve, but a symlink at
// the final component is refused.
//
// rw distinguishes the two callers, because they are exposed to different things.
// With rw the agent is about to create, chown and bind here while still root, so
// the full rule applies. Without rw the caller only reads or connects to what is
// already there - netdatacli looking for netdata.pipe, a plugin looking for a
// socket - and one requirement is dropped for it: see the sticky case below.
bool os_run_dir_is_safe(const char *candidate, bool rw) {
    // Trimmed here, not just in the callers: with a trailing slash the kernel
    // resolves the final component as a directory, so lstat() would report a
    // planted symlink's target and every check below would pass.
    char dir[FILENAME_MAX + 1];
    if (!os_dir_path_trim(candidate, dir, sizeof(dir)))
        return false;

    struct stat st;
    if (lstat(dir, &st) == -1)
        // nothing is there (yet) - silent, the caller tries the next candidate or
        // creates it
        return false;

    // The check that stops a pre-created /tmp/netdata -> /etc from becoming our run
    // dir. Kept separate from the !S_ISDIR case below, which it implies, so each
    // refusal can say which one it was: a refusal here relocates the agent to the
    // next candidate (typically /tmp/netdata), and an operator who symlinked the run
    // directory on purpose would otherwise have nothing to go on.
    if (S_ISLNK(st.st_mode)) {
        netdata_log_error(
            "Refusing run directory '%s': it is a symbolic link. Point NETDATA_RUN_DIR at the "
            "directory itself, or bind-mount it there.", dir);
        return false;
    }

    if (!S_ISDIR(st.st_mode)) {
        netdata_log_error("Refusing run directory '%s': not a directory", dir);
        return false;
    }

    // silent: "we cannot use this one" is a normal outcome while probing the
    // built-in candidates, not a misconfiguration
    if (access(dir, rw ? W_OK : R_OK) == -1)
        return false;

#if defined(OS_WINDOWS)
    // Windows has no POSIX ownership: Cygwin synthesizes st_uid from the NTFS
    // ACL (there is no uid 0) and never reports a sticky bit unless one was set
    // explicitly, so the check below would refuse every candidate and leave the
    // agent with no run dir at all. There is also no privilege drop here, so
    // there is nothing for it to protect.
    return true;
#else
    // The leaf's own mode matters as much as its parent's: everything netdata
    // puts here (the spawn server sockets, netdata.pipe) is created and used
    // while still root, so a directory every local user can write into lets any
    // of them pre-plant or swap those names. This is what refuses
    // NETDATA_RUN_DIR=/tmp, which the parent check alone accepts (/tmp's parent
    // is root-owned and exclusive).
    // Group-write is deliberately still accepted: the shipped systemd unit
    // creates /run/netdata with RuntimeDirectoryMode=0775 and re-applies it on
    // every start, so refusing S_IWGRP would leave the default install with no
    // run directory at all.
    if (st.st_mode & S_IWOTH) {
        netdata_log_error(
            "Refusing run directory '%s': mode %04o lets any local user write into it",
            dir, (unsigned int)(st.st_mode & 07777));
        return false;
    }

    switch (os_dir_parent_trust(dir)) {
        case OS_DIR_PARENT_EXCLUSIVE:
            // Nobody else can create an entry here, so whoever owns the
            // directory, we are the ones who put it there. Its ownership is
            // therefore not our business - it legitimately differs per install:
            // tmpfiles.d creates /run/netdata owned by the netdata user, while
            // systemd's RuntimeDirectory= with User=root makes it root:netdata.
            return true;

        case OS_DIR_PARENT_STICKY:
            // The /tmp fallback. Anyone can create entries here, but sticky
            // means only the owner of an entry can rename or delete it, so
            // requiring the directory to be ours keeps every third account out.
            // "Ours" has to include the uid we will drop to, because the first
            // run creates this directory as root and then chowns it - and that
            // is also the limit of what this predicate can promise: a process
            // already running as that uid owns the entry, so it can still swap
            // the directory between here and the privileged use that follows
            // (the temporary spawn server binds its socket here long before
            // become_user()). What protects that use is performing it through a
            // descriptor - os_open_dir_privileged() and fchown() - not this
            // check.
            //
            // A read-only caller is exempt: it creates nothing and chowns nothing,
            // and it cannot answer the question anyway - netdatacli run by root
            // does not know which uid the agent runs as, so requiring the
            // directory to be "ours" only made it unable to find an agent that
            // uses this fallback. Everything else above still applies to it, so it
            // cannot be pointed elsewhere by a planted symlink, and the socket's
            // own mode is what decides whether it may actually talk to the agent.
            if (!rw || run_dir_owner_is_ours(st.st_uid))
                return true;

            netdata_log_error(
                "Refusing run directory '%s': owned by uid %u, inside a world-writable parent",
                dir, (unsigned int)st.st_uid);
            return false;

        case OS_DIR_PARENT_UNKNOWN:
            netdata_log_error("Refusing run directory '%s': cannot examine its parent directory", dir);
            return false;

        default:
            netdata_log_error(
                "Refusing run directory '%s': anyone can replace it, because its parent is neither "
                "ours nor sticky. Set NETDATA_RUN_DIR to a directory only netdata can write into.",
                dir);
            return false;
    }
#endif
}

static inline bool netdata_dir_in_parent(const char *parent, char *out_path, size_t out_path_len, bool rw) {
    int ret = snprintf(out_path, out_path_len, "%s/netdata", parent);
    if (ret < 0 || (size_t)ret >= out_path_len)
        return false;

    if (os_run_dir_is_safe(out_path, rw))
        return true;

    // a read-only resolution reports what exists; it does not create anything
    if (!rw || !is_dir_accessible(parent, rw))
        return false;

    if (mkdir(out_path, 0755) == -1 && errno != EEXIST)
        return false;

    // Re-validate after mkdir(): on EEXIST the directory is one we did not
    // create, so it could be a symlink or a directory planted by another user.
    return os_run_dir_is_safe(out_path, rw);
}

static char *detect_run_dir(bool rw) {
    char path[FILENAME_MAX + 1];

    // An operator-supplied run dir (e.g. a mounted tmpfs in a hardened,
    // read-only-/run container) is honored, but gets the same validation as
    // every other branch, since it is chowned as root.
    const char *env_dir = getenv("NETDATA_RUN_DIR");
    if (env_dir && *env_dir) {
        // trim first: a trailing slash makes the kernel resolve the final
        // component as a directory, which would defeat the symlink checks
        if (os_dir_path_trim(env_dir, path, sizeof(path)) && os_run_dir_is_safe(path, rw))
            return strdupz(path);

        // an operator asked for this directory explicitly - never fall through
        // to the built-in branches without saying why
        netdata_log_error("Ignoring NETDATA_RUN_DIR='%s': not a usable run directory", env_dir);
    }

#if defined(OS_LINUX)
    // First try /run/netdata
    if (netdata_dir_in_parent("/run", path, sizeof(path), rw))
        goto success;
#endif

#if defined(OS_MACOS)
    // macOS typically uses /private/var/run
    if (netdata_dir_in_parent("/private/var/run", path, sizeof(path), rw))
        goto success;
#endif

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
    // Then try /var/run/netdata
    if (netdata_dir_in_parent("/var/run", path, sizeof(path), rw))
        goto success;
#endif

//#if defined(OS_WINDOWS)
//    // On MSYS2/Cygwin get TEMP and convert it properly
//    WCHAR temp_pathW[MAX_PATH];
//    DWORD len = GetEnvironmentVariableW(L"TEMP", temp_pathW, MAX_PATH);
//    if (len > 0 && len < MAX_PATH) {
//        // Convert Windows wide path to UTF-8
//        int utf8_len = WideCharToMultiByte(CP_UTF8, 0, temp_pathW, -1, NULL, 0, NULL, NULL);
//        if (utf8_len > 0 && utf8_len < FILENAME_MAX) {
//            char win_path[FILENAME_MAX + 1];
//            if (WideCharToMultiByte(CP_UTF8, 0, temp_pathW, -1, win_path, sizeof(win_path), NULL, NULL)) {
//                // Convert Windows path to Unix path using Cygwin API
//                ssize_t unix_size = cygwin_conv_path(CCP_WIN_A_TO_POSIX, win_path, NULL, 0);
//                if (unix_size > 0) {
//                    char unix_path[FILENAME_MAX + 1];
//                    if (cygwin_conv_path(CCP_WIN_A_TO_POSIX, win_path, unix_path, sizeof(unix_path)) == 0) {
//                        if (is_dir_accessible(unix_path, rw)) {
//                            snprintfz(path, sizeof(path), "%s/netdata", unix_path);
//                            if (!rw)
//                                goto success;
//
//                            if (mkdir(path, 0755) == 0 || errno == EEXIST)
//                                goto success;
//                        }
//                    }
//                }
//            }
//        }
//    }
//#endif

    // Fallback to /tmp/netdata - force creation if needed.
    // /tmp is world-writable, so this is the branch an unprivileged user can
    // plant. os_run_dir_is_safe() requires the directory to be ours and /tmp to be
    // sticky, so a planted one is refused instead of adopted.
    if (!is_dir_accessible("/tmp", rw)) {
        // Try to create /tmp with standard permissions (including sticky bit)
        if (rw && mkdir("/tmp", 01777) == -1 && errno != EEXIST)
            return NULL;
    }

    if (netdata_dir_in_parent("/tmp", path, sizeof(path), rw))
        goto success;

    return NULL;

success:
    // Set the environment variable for child processes
    if(rw)
        nd_setenv("NETDATA_RUN_DIR", path, 1);

    return strdupz(path);
}

const char *os_run_dir(bool rw) {
    // Fast path - reuse the cached directory only when it answers this request.
    // cached_run_dir_writable implies cached_run_dir_available, so the writable
    // flag is the whole test for rw callers.
    if(__atomic_load_n(rw ? &cached_run_dir_writable : &cached_run_dir_available, __ATOMIC_ACQUIRE))
        return __atomic_load_n(&cached_run_dir, __ATOMIC_ACQUIRE);

    spinlock_lock(&spinlock);

    // Check again under lock in case another thread set it
    char *run_dir = cached_run_dir;

    // A read-only resolution reports what already exists: it creates nothing and
    // only asks for R_OK. So it can settle on a directory we cannot write into
    // (a read-only /run bind mount, a root-owned /run/netdata when we are not
    // root), or on the /tmp fallback while /run/netdata simply does not exist
    // yet. Handing that answer to the first writable caller would make its
    // bind()/mkdir() fail on a directory nobody validated for writing - and it
    // would also skip the nd_setenv() that tells our children where the run dir
    // is. Detect again instead.
    if(!run_dir || (rw && !cached_run_dir_writable)) {
        char *found = detect_run_dir(rw);

        if(found && run_dir) {
            if(strcmp(run_dir, found) == 0) {
                // same directory, only its validation got stronger - keep the
                // pointer callers already have
                freez(found);
                found = run_dir;
            }
            else
                superseded_run_dir = run_dir;
        }

        if(found) {
            // published before the flags, so a reader that sees a flag sees this
            __atomic_store_n(&cached_run_dir, found, __ATOMIC_RELEASE);
            if(rw)
                __atomic_store_n(&cached_run_dir_writable, true, __ATOMIC_RELEASE);
            __atomic_store_n(&cached_run_dir_available, true, __ATOMIC_RELEASE);
        }

        // When no writable directory exists, the read-only answer stays cached
        // for the read-only callers, but this caller gets nothing: it asked for a
        // directory it can write into, and there is none.
        run_dir = found;
    }

    spinlock_unlock(&spinlock);

    errno_clear();
    return run_dir;
}
