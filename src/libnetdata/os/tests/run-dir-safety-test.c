// SPDX-License-Identifier: GPL-3.0-or-later
//
// netdata binds sockets in this directory and chowns it while still running as root, so
// it must refuse anything another account could have planted or could replace.
//
// Two rules are checked, because os_run_dir_is_safe() serves two callers: the agent
// itself (rw), which is about to create, chown and bind here as root, and a client
// (read-only) that only reads or connects to what is already there. Both refuse a
// directory they could be redirected away from - a symlink, a non-directory. Only
// the rw rule also asks who else could replace the directory, which is what the
// agent's privileged use of it needs - see the read-only section below.
//
// os_run_dir_is_safe() is exercised directly rather than through os_run_dir(),
// which caches its answer for the process lifetime and creates directories as a
// side effect. os_run_dir() itself is called only by the last case, which is
// about that cache, and only with NETDATA_RUN_DIR pointed inside this test's own
// temporary tree.

#include "libnetdata/libnetdata.h"

static const uid_t OTHER_UID = 65530; // some third account, not us
static const uid_t NETDATA_UID = 65531; // stands in for the configured "run as user"

static char root_dir[FILENAME_MAX + 1];
static int failures = 0;
static int skipped = 0;

static void check_mode(const char *what, const char *dir, bool rw, bool expected) {
    bool got = os_run_dir_is_safe(dir, rw);
    if (got == expected)
        fprintf(stderr, "  ok       %-52s (%s)\n", what, got ? "accepted" : "refused");
    else {
        fprintf(stderr, "  FAILED   %-52s got %s, expected %s\n",
                what, got ? "accepted" : "refused", expected ? "accepted" : "refused");
        failures++;
    }
}

// the run directory as the agent itself resolves it: the rule that has to hold
// while we are still root
static void check(const char *what, const char *dir, bool expected) {
    bool got = os_run_dir_is_safe(dir, true);
    if (got == expected)
        fprintf(stderr, "  ok       %-52s (%s)\n", what, got ? "accepted" : "refused");
    else {
        fprintf(stderr, "  FAILED   %-52s got %s, expected %s\n",
                what, got ? "accepted" : "refused", expected ? "accepted" : "refused");
        failures++;
    }
}

// os_open_dir_privileged() reports its trimming failures through errno, and the
// caller logs it: a path with nothing left to name must not be reported as if it
// were too long.
static void check_trim_errno(const char *what, const char *path, char *dst, size_t dst_size, int expected) {
    errno = 0;
    if (os_dir_path_trim(path, dst, dst_size)) {
        fprintf(stderr, "  FAILED   %-52s trimmed, expected failure\n", what);
        failures++;
        return;
    }

    if (errno == expected)
        fprintf(stderr, "  ok       %-52s (%s)\n", what, strerror(errno));
    else {
        fprintf(stderr, "  FAILED   %-52s errno %s, expected %s\n",
                what, strerror(errno), strerror(expected));
        failures++;
    }
}

// os_open_dir_privileged() decides whether to follow a symlink at the final
// component from the trust class of the directory holding it.
static void check_open(const char *what, const char *path, bool expected) {
    int fd = os_open_dir_privileged(path);
    bool got = (fd != -1);
    if (fd != -1) close(fd);

    if (got == expected)
        fprintf(stderr, "  ok       %-52s (%s)\n", what, got ? "opened" : "refused");
    else {
        fprintf(stderr, "  FAILED   %-52s got %s, expected %s\n",
                what, got ? "opened" : "refused", expected ? "opened" : "refused");
        failures++;
    }
}

// Build "<root_dir>/<name>" into buf.
static const char *under(char *buf, size_t size, const char *name) {
    snprintfz(buf, size, "%s/%s", root_dir, name);
    return buf;
}

static void make_parent(const char *path, mode_t mode) {
    if (mkdir(path, 0755) == -1 && errno != EEXIST)
        fatal("test setup: cannot create '%s'", path);
    if (chmod(path, mode) == -1)
        fatal("test setup: cannot chmod '%s' to %04o", path, (unsigned)mode);
}

int main(void) {
    // the validator logs a reason for every refusal; most cases below expect one
    nd_log_limits_unlimited();

    snprintfz(root_dir, sizeof(root_dir), "/tmp/netdata-run-dir-safety-test-%d", (int)getpid());
    if (mkdir(root_dir, 0755) == -1)
        fatal("test setup: cannot create '%s'", root_dir);

    char exclusive[FILENAME_MAX + 1], sticky[FILENAME_MAX + 1], open_parent[FILENAME_MAX + 1];
    make_parent(under(exclusive, sizeof(exclusive), "exclusive"), 0755);   // ours, no group/other write
    make_parent(under(sticky, sizeof(sticky), "sticky"), 01777);           // ours, world-writable + sticky
    make_parent(under(open_parent, sizeof(open_parent), "open"), 0777);    // world-writable, NOT sticky

    char p[FILENAME_MAX + 1], q[FILENAME_MAX + 1];

    fprintf(stderr, "cases that need no privileges:\n");

    // The reported attack: an unprivileged user pre-creates the run dir as a
    // symlink, so the still-root daemon chowns the target instead.
    snprintfz(p, sizeof(p), "%s/to_etc", exclusive);
    if (symlink("/etc", p) == -1) fatal("test setup: symlink '%s'", p);
    check("symlink to /etc", p, false);

    // Same, pointing at a directory that really is ours - it is still a symlink.
    snprintfz(p, sizeof(p), "%s/real", exclusive);
    make_parent(p, 0755);
    snprintfz(q, sizeof(q), "%s/to_real", exclusive);
    if (symlink(p, q) == -1) fatal("test setup: symlink '%s'", q);
    check("symlink to a directory we own", q, false);

    // Anything that makes the kernel resolve the final component as a directory
    // defeats both O_NOFOLLOW and lstat()'s S_ISLNK, so the symlink above must
    // stay refused however the caller spells it. A trailing "." is the subtle
    // one: it also makes the parent-trust lookup stat() through the symlink and
    // read the trust class off the target.
    // The last two cases are ".." and "." themselves: they reach a directory
    // through the component before them, so ".." has no entry left to judge and
    // must be refused rather than validated against the wrong inode, while "."
    // on a real directory simply normalises away.
    static const struct {
        const char *suffix;
        const char *what;
        bool expected;
    } spellings[] = {
        { "to_real/",      "symlink, trailing slash",                   false },
        { "to_real/.",     "symlink, trailing dot",                     false },
        { "to_real/./",    "symlink, trailing dot-slash",               false },
        { "to_real/.//./", "symlink, repeated dots and slashes",        false },
        { "to_real//.",    "symlink, doubled slash then dot",           false },
        { "real/..",       "path ending in '..'",                       false },
        { "real/.",        "real directory, trailing dot (normalises)", true  },
    };

    for (size_t i = 0; i < _countof(spellings); i++) {
        snprintfz(p, sizeof(p), "%s/%s", exclusive, spellings[i].suffix);
        check(spellings[i].what, p, spellings[i].expected);
    }

    snprintfz(p, sizeof(p), "%s/afile", exclusive);
    int fd = open(p, O_WRONLY | O_CREAT | O_EXCL, 0644);
    if (fd == -1) fatal("test setup: create '%s'", p);
    close(fd);
    check("a regular file, not a directory", p, false);

    snprintfz(p, sizeof(p), "%s/missing", exclusive);
    check("a path that does not exist", p, false);

    // The legitimate shapes.
    snprintfz(p, sizeof(p), "%s/real", exclusive);
    check("real directory, parent writable only by us", p, true);

    snprintfz(p, sizeof(p), "%s/ours", sticky);
    make_parent(p, 0755);
    check("real directory we own, sticky parent (/tmp shape)", p, true);

    // Sticky is what stops another account renaming our directory away between
    // validation and use; without it a world-writable parent is not usable.
    snprintfz(p, sizeof(p), "%s/ours", open_parent);
    make_parent(p, 0755);
    check("world-writable parent without the sticky bit", p, false);

    // The leaf's own mode. Its parent being exclusive says nobody else can
    // replace the directory - it says nothing about who can create entries
    // inside it, which is where the sockets go.
    snprintfz(p, sizeof(p), "%s/other_writable", exclusive);
    make_parent(p, 0777);
    check("directory writable by everyone", p, false);

    snprintfz(p, sizeof(p), "%s/other_writable_sticky", exclusive);
    make_parent(p, 01777);
    check("directory writable by everyone, even sticky", p, false);

    // Must stay accepted: the shipped systemd unit creates /run/netdata with
    // RuntimeDirectoryMode=0775 and re-applies it on every start, so refusing
    // group-write would leave the default install without a run directory.
    snprintfz(p, sizeof(p), "%s/group_writable", exclusive);
    make_parent(p, 0775);
    check("directory writable by its group (systemd 0775 shape)", p, true);

    // The read-only resolution a client performs - netdatacli looking for
    // netdata.pipe, a plugin looking for a socket. It creates nothing, chowns
    // nothing and binds nothing, so none of the "who else could replace this
    // directory" rules apply to it: those exist to protect what we do here as
    // root. What still applies is that it must not be redirected to another
    // directory, so a symlink or a non-directory is refused for it too.
    fprintf(stderr, "read-only resolution, for a client rather than the agent:\n");
    snprintfz(p, sizeof(p), "%s/ours", sticky);
    check_mode("directory we own, sticky parent", p, false, true);
    snprintfz(p, sizeof(p), "%s/to_etc", exclusive);
    check_mode("symlink to /etc", p, false, false);
    snprintfz(p, sizeof(p), "%s/afile", exclusive);
    check_mode("a regular file, not a directory", p, false, false);

    // Accepted for a client, refused for the agent: the same directory, judged by
    // what the caller is about to do with it. A non-root install legitimately puts
    // its run directory under a parent the agent's own account owns, and a client
    // that only connects must still be able to find it.
    snprintfz(p, sizeof(p), "%s/ours", open_parent);
    check_mode("world-writable parent without the sticky bit", p, false, true);
    check_mode("world-writable parent without the sticky bit", p, true, false);
    snprintfz(p, sizeof(p), "%s/other_writable", exclusive);
    check_mode("directory writable by everyone", p, false, true);
    check_mode("directory writable by everyone", p, true, false);

    fprintf(stderr, "privileged open, symlink at the final component:\n");
    {
        // Exclusive parent: only we could have placed the link, so it is
        // followed - this is what keeps /var/cache/netdata -> another
        // filesystem working.
        snprintfz(p, sizeof(p), "%s/real", exclusive);
        snprintfz(q, sizeof(q), "%s/link_ok", exclusive);
        if (symlink(p, q) == -1) fatal("test setup: symlink '%s'", q);
        check_open("symlink in an exclusive parent (followed)", q, true);

        // Same link, but reached through a parent anyone can write into: now
        // somebody else could have planted it, so O_NOFOLLOW refuses it.
        snprintfz(p, sizeof(p), "%s/link_bad", open_parent);
        snprintfz(q, sizeof(q), "%s/real", exclusive);
        if (symlink(q, p) == -1) fatal("test setup: symlink '%s'", p);
        check_open("symlink in a world-writable parent (refused)", p, false);

        // A real directory opens either way.
        snprintfz(p, sizeof(p), "%s/real", exclusive);
        check_open("a real directory", p, true);
    }

    // The spawn server sets its socket's mode with
    // fchmodat(dir_fd, name, 0770, AT_SYMLINK_NOFOLLOW) while it may still be
    // root, in a directory the netdata account can write into. That is safe only
    // because AT_SYMLINK_NOFOLLOW refuses a symlink instead of following it, so
    // assert the platform really behaves that way rather than trusting it.
    fprintf(stderr, "the fchmodat() guarantee the spawn server relies on:\n");
    {
        snprintfz(p, sizeof(p), "%s/chmod_victim", exclusive);
        int vfd = open(p, O_WRONLY | O_CREAT | O_EXCL, 0600);
        if (vfd == -1) fatal("test setup: create '%s'", p);
        close(vfd);

        snprintfz(q, sizeof(q), "%s/chmod_link", exclusive);
        if (symlink(p, q) == -1) fatal("test setup: symlink '%s'", q);

        int dfd = open(exclusive, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
        if (dfd == -1) fatal("test setup: open '%s'", exclusive);

        errno = 0;
        bool refused = (fchmodat(dfd, "chmod_link", 0770, AT_SYMLINK_NOFOLLOW) == -1);
        int refused_errno = errno;

        struct st_check { struct stat st; } v;
        if (lstat(p, &v.st) == -1) fatal("test: lstat '%s'", p);
        bool victim_untouched = ((v.st.st_mode & 07777) == 0600);

        if (refused && victim_untouched)
            fprintf(stderr, "  ok       %-52s (%s)\n",
                    "symlink refused, its target left alone", strerror(refused_errno));
        else if (!refused && victim_untouched)
            // no platform is known to do this; it would mean the link itself was
            // chmoded, which is harmless here but worth noticing
            fprintf(stderr, "  ok       %-52s (link itself)\n", "symlink not followed");
        else {
            fprintf(stderr, "  FAILED   %-52s target became %04o - the spawn server "
                            "socket chmod can be redirected on this platform\n",
                    "symlink refused, its target left alone", (unsigned)(v.st.st_mode & 07777));
            failures++;
        }

        close(dfd);
    }

#if defined(OS_LINUX)
    // The spawn server binds its socket through "/proc/self/fd/<dir_fd>/<name>"
    // so that bind() resolves the run directory it already validated instead of
    // walking the path again. Prove that: rename the directory away and leave a
    // symlink to somewhere else in its place, then bind - the socket must appear
    // in the directory the descriptor refers to, not in the replacement.
    fprintf(stderr, "the bind() guarantee the spawn server relies on:\n");
    {
        char rundir[FILENAME_MAX + 1], attacker[FILENAME_MAX + 1], moved[FILENAME_MAX + 1];
        make_parent(under(rundir, sizeof(rundir), "bind_run"), 0755);
        make_parent(under(attacker, sizeof(attacker), "bind_attacker"), 0755);
        under(moved, sizeof(moved), "bind_run_moved");

        int dfd = open(rundir, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
        if (dfd == -1) fatal("test setup: open '%s'", rundir);

        // Same question the spawn server asks before it decides what a failed
        // bind means: is the descriptor route available here at all? Only an
        // unavailable route may skip - a route that is available and then fails
        // is the defect this case exists to catch, never a skip.
        char probe[FILENAME_MAX + 1];
        snprintfz(probe, sizeof(probe), "/proc/self/fd/%d", dfd);
        struct stat via_proc, via_fd;
        bool route_usable = (stat(probe, &via_proc) == 0 && fstat(dfd, &via_fd) == 0 &&
                             via_proc.st_dev == via_fd.st_dev && via_proc.st_ino == via_fd.st_ino);

        // The swap, exactly as a run-dir owner could perform it. Replacing a path
        // that was just examined is a TOCTOU race by construction - here it is the
        // fixture, not a defect: the race is precisely what the descriptor route
        // below has to survive.
        if (rename(rundir, moved) == -1) fatal("test setup: rename '%s'", rundir);
        if (symlink(attacker, rundir) == -1) fatal("test setup: symlink '%s'", rundir); // NOSONAR

        struct sockaddr_un addr = { .sun_family = AF_UNIX };
        snprintfz(addr.sun_path, sizeof(addr.sun_path) - 1, "/proc/self/fd/%d/s.sock", dfd);

        int sock = socket(AF_UNIX, SOCK_STREAM, 0);
        if (sock == -1) fatal("test setup: socket()");

        errno = 0;
        bool bound = (bind(sock, (struct sockaddr *)&addr, sizeof(addr)) == 0);
        int bind_errno = errno;

        char in_attacker[FILENAME_MAX + 1], in_real[FILENAME_MAX + 1];
        snprintfz(in_attacker, sizeof(in_attacker), "%s/s.sock", attacker);
        snprintfz(in_real, sizeof(in_real), "%s/s.sock", moved);

        struct stat st;
        bool landed_in_attacker = (lstat(in_attacker, &st) == 0);
        bool landed_in_real = (lstat(in_real, &st) == 0);

        if (!route_usable) {
            // No usable /proc (chroot, hardened sandbox, some containers). The
            // spawn server warns and binds the pathname there; that residual is
            // tracked, not asserted here.
            fprintf(stderr, "  SKIPPED  %-52s (no usable /proc/self/fd)\n",
                    "run dir swapped after open, before bind");
            skipped++;
        }
        else if (bound && landed_in_real && !landed_in_attacker)
            fprintf(stderr, "  ok       %-52s (followed the descriptor)\n",
                    "run dir swapped after open, before bind");
        else {
            // Either the bind failed although the route was available - in which
            // case the spawn server must refuse rather than retry on the pathname
            // - or it followed the swap.
            fprintf(stderr, "  FAILED   %-52s bound=%s(%s) real=%s attacker=%s\n",
                    "run dir swapped after open, before bind",
                    bound ? "yes" : "no", bound ? "-" : strerror(bind_errno),
                    landed_in_real ? "yes" : "no",
                    landed_in_attacker ? "YES - bind followed the swap" : "no");
            failures++;
        }

        close(sock);
        close(dfd);
        unlink(in_real);
        unlink(in_attacker);
        unlink(rundir);
    }
#endif

    fprintf(stderr, "trim failures, as reported to the caller:\n");
    {
        char small[8];
        check_trim_errno("NULL path", NULL, small, sizeof(small), EINVAL);
        check_trim_errno("empty path", "", small, sizeof(small), EINVAL);
        // "/x/." normalises to "/x"; these are the shapes with no entry left
        check_trim_errno("the cwd itself, '.'", ".", p, sizeof(p), EINVAL);
        check_trim_errno("path ending in '..'", "/var/lib/netdata/..", p, sizeof(p), EINVAL);
        check_trim_errno("path longer than the buffer", "/var/lib/netdata", small, sizeof(small), ENAMETOOLONG);
    }

    fprintf(stderr, "cases that need root:\n");
    if (geteuid() != 0) {
        fprintf(stderr, "  SKIPPED  third-account ownership, the restart case, and the"
                        " read-only client case (re-run as root to cover them)\n");
        skipped += 4;
    }
    else {
        snprintfz(p, sizeof(p), "%s/foreign", sticky);
        make_parent(p, 0755);
        if (chown(p, OTHER_UID, OTHER_UID) == -1)
            fatal("test setup: cannot chown '%s'", p);

        // Nobody declared this uid, so it is a third account: refuse it rather
        // than adopt a directory somebody else planted in /tmp.
        check("sticky parent, directory owned by a third account", p, false);

        // The restart case. The first run creates /tmp/netdata as root and then
        // chowns it to the "run as user" uid, so on the next start the directory
        // is owned by that uid and must still be recognised as ours.
        snprintfz(q, sizeof(q), "%s/netdata_owned", sticky);
        make_parent(q, 0755);
        if (chown(q, NETDATA_UID, NETDATA_UID) == -1)
            fatal("test setup: cannot chown '%s'", q);

        os_run_dir_set_target_uid(NETDATA_UID);
        check("sticky parent, directory owned by the configured uid", q, true);

        // Declaring a different uid must not launder a third account's directory.
        check("sticky parent, third account, a different uid declared", p, false);

        os_run_dir_set_target_uid((uid_t)-1);

        // The netdatacli case: root asking where an agent that runs as another uid
        // keeps its socket. The agent's own rule refuses this directory, and a
        // client asked to obey it could not reach such an agent at all.
        check_mode("read-only, sticky parent, a third account's directory", p, false, true);
    }

    // os_run_dir() caches its answer for the process lifetime, so this case runs
    // last: it is the one that populates that cache.
    // A read-only resolution is weaker than a writable one - it creates nothing
    // and only asks for R_OK - so it can settle on a directory we cannot write
    // into, and the first writable caller must re-run the detection instead of
    // inheriting that answer. Otherwise it binds its sockets in a directory
    // nobody checked it can write into, and fails.
    // NETDATA_RUN_DIR is repointed between the two calls purely as the probe: a
    // detection that really re-ran lands on the second directory, an inherited
    // answer is still the first one. Both directories are inside this test's own
    // tree, so nothing outside it is created or consulted.
    fprintf(stderr, "os_run_dir() caching, read-only answer then writable request:\n");
    {
        char ro_leaf[FILENAME_MAX + 1], rw_leaf[FILENAME_MAX + 1];
        make_parent(under(ro_leaf, sizeof(ro_leaf), "cached_readable"), 0500); // readable, not writable
        make_parent(under(rw_leaf, sizeof(rw_leaf), "cached_writable"), 0755);

        nd_setenv("NETDATA_RUN_DIR", ro_leaf, 1);
        const char *ro = os_run_dir(false);

        nd_setenv("NETDATA_RUN_DIR", rw_leaf, 1);
        const char *rw = os_run_dir(true);

        const char *what = "writable request after a read-only one";

        if(!ro || strcmp(ro, ro_leaf) != 0) {
            fprintf(stderr, "  FAILED   %-52s got '%s'\n", "read-only resolution", ro ? ro : "(null)");
            failures++;
        }
        else if(rw && strcmp(rw, rw_leaf) == 0)
            fprintf(stderr, "  ok       %-52s (re-detected)\n", what);
        else if(!rw) {
            // it re-detected, but found nothing: the failure is in the detection,
            // not in the cache
            fprintf(stderr, "  FAILED   %-52s returned NULL for a writable directory "
                            "that exists\n", what);
            failures++;
        }
        else if(strcmp(rw, ro_leaf) == 0) {
            fprintf(stderr, "  FAILED   %-52s got '%s' - the writable request reused the "
                            "read-only answer\n", what, rw);
            failures++;
        }
        else {
            fprintf(stderr, "  FAILED   %-52s got '%s', expected '%s'\n", what, rw, rw_leaf);
            failures++;
        }

        // the earlier read-only answer stays valid, and writable again so the
        // cleanup below can remove it. Only for us: nothing here is shared.
        if(chmod(ro_leaf, 0700) == -1)
            fprintf(stderr, "warning: could not chmod '%s' back\n", ro_leaf);
    }

    // best effort cleanup; the tree is under /tmp and named after our pid
    char cmd[FILENAME_MAX + 32];
    snprintfz(cmd, sizeof(cmd), "rm -rf '%s'", root_dir);
    if (system(cmd) == -1)
        fprintf(stderr, "warning: could not remove '%s'\n", root_dir);

    if (failures) {
        fprintf(stderr, "\n%d case(s) FAILED\n", failures);
        return 1;
    }

    fprintf(stderr, "\nall cases passed%s\n",
            skipped ? " (some skipped - not running as root)" : "");
    return 0;
}
