// SPDX-License-Identifier: GPL-3.0-or-later
//
// netdata binds sockets in this directory and chowns it while still running as root, so
// it must refuse anything another account could have planted or could replace.
//
// os_run_dir_is_safe() is exercised directly rather than through os_run_dir(),
// which caches its answer for the process lifetime and creates directories as a
// side effect.

#include "libnetdata/libnetdata.h"

static const uid_t OTHER_UID = 65530; // some third account, not us
static const uid_t NETDATA_UID = 65531; // stands in for the configured "run as user"

static char root_dir[FILENAME_MAX + 1];
static int failures = 0;
static int skipped = 0;

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
    snprintfz(p, sizeof(p), "%s/to_real/", exclusive);
    check("symlink, trailing slash", p, false);
    snprintfz(p, sizeof(p), "%s/to_real/.", exclusive);
    check("symlink, trailing dot", p, false);
    snprintfz(p, sizeof(p), "%s/to_real/./", exclusive);
    check("symlink, trailing dot-slash", p, false);
    snprintfz(p, sizeof(p), "%s/to_real/.//./", exclusive);
    check("symlink, repeated dots and slashes", p, false);
    snprintfz(p, sizeof(p), "%s/to_real//.", exclusive);
    check("symlink, doubled slash then dot", p, false);

    // "." and ".." reach a directory through the component before them, so there
    // is no entry left to judge - they must be refused rather than validated
    // against the wrong inode.
    snprintfz(p, sizeof(p), "%s/real/..", exclusive);
    check("path ending in '..'", p, false);
    snprintfz(p, sizeof(p), "%s/real/.", exclusive);
    check("real directory, trailing dot (normalises)", p, true);

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

    fprintf(stderr, "cases that need root:\n");
    if (geteuid() != 0) {
        fprintf(stderr, "  SKIPPED  third-account ownership and the restart case"
                        " (re-run as root to cover them)\n");
        skipped += 2;
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
