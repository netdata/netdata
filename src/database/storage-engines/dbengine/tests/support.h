// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_SUPPORT_H
#define NETDATA_DBENGINE_TESTS_SUPPORT_H

// Shared by every test in this suite. It reaches the engine only through its public headers, the ones an embedder
// that is not the daemon would include; check-public-includes.sh enforces that for every file here whose name does
// not begin with internal_.

#include <gtest/gtest.h>

#include <dirent.h>
#include <sys/stat.h>
#include <unistd.h>

#include <cerrno>
#include <cstdlib>
#include <string>

#include "database/storage-engines/dbengine/include/dbengine/dbengine-api.h"

// The pool the engine's work is dispatched into is process-wide, sized once at its first use from
// UV_THREADPOOL_SIZE, and never resized. The engine does not size it and cannot read its size: it throttles itself
// against the figure the embedder hands it in libuv_worker_threads. If that figure is larger than the real pool,
// every pool thread can end up held by a parent waiting for a child that can never be scheduled, and that does not
// fail - the process hangs, silently and for good. So one constant feeds both: main() exports it before any test
// runs, and netdata_test_config() puts the same number in the configuration.
#define DBENGINE_TEST_UV_THREADS (16)

// The page allocator layer is process-wide and one-shot: the first engine in a process fixes its partitions and
// size classes, and a later engine asking for different ones is logged and ignored. A suite that let two
// configurations exist would get either a false red or a false green depending on the order its cases ran in. It is
// avoided by construction rather than by an ordering rule: every engine in this binary is created from this one
// function, and the two fields that feed the allocator layer are set explicitly rather than resolved from the
// machine, so the same configuration is used no matter which cases run or in what order.
inline struct dbengine_config netdata_test_config() {
    struct dbengine_config cfg = dbengine_config_defaults();

    cfg.cpus = 2;
    cfg.allocator.partitions = 2;
    cfg.libuv_worker_threads = DBENGINE_TEST_UV_THREADS;

    return cfg;
}

// Remove a directory and everything below it. Reports whether it managed to, because a cleanup that fails silently
// leaves litter nobody hears about - and every earlier version of this ignored what unlink() and rmdir() returned.
//
// It descends rather than assuming one flat level, and it skips only "." and "..", not every dotfile: a tier writes
// two plain files today, but a cleanup that quietly cannot cope with anything else is a cleanup that will one day
// quietly stop working.
inline bool netdata_test_remove_tree(const std::string &path) {
    DIR *dir = opendir(path.c_str());
    if (!dir)
        return rmdir(path.c_str()) == 0 || errno == ENOENT;

    bool removed_everything = true;

    while (const struct dirent *entry = readdir(dir)) {
        const std::string name = entry->d_name;
        if (name == "." || name == "..")
            continue;

        const std::string child = path + "/" + name;

        struct stat st = {};
        if (lstat(child.c_str(), &st) != 0) {
            removed_everything = false;
            continue;
        }

        if (S_ISDIR(st.st_mode))
            removed_everything = netdata_test_remove_tree(child) && removed_everything;
        else if (unlink(child.c_str()) != 0)
            removed_everything = false;
    }

    closedir(dir);

    if (rmdir(path.c_str()) != 0)
        removed_everything = false;

    return removed_everything;
}

// A directory of its own for a test that brings a tier up. Two tiers writing one directory corrupt it and nothing
// in the engine refuses the second, so no two tests may share one.
class Scratch {
public:
    Scratch() {
        // TMPDIR when the environment names one: a runner or a sandbox that redirects it usually cannot write to
        // /tmp at all, and a suite that ignores it fails there for a reason that looks like the engine's fault.
        const char *root = getenv("TMPDIR");
        if (!root || !*root)
            root = "/tmp";

        std::string tmpl = std::string(root) + "/dbengine-test-XXXXXX";
        if (mkdtemp(tmpl.data()))
            path_ = tmpl;
    }

    ~Scratch() {
        if (path_.empty())
            return;

        if (!netdata_test_remove_tree(path_))
            ADD_FAILURE() << "the scratch directory " << path_ << " could not be removed";
    }

    Scratch(const Scratch &) = delete;
    Scratch &operator=(const Scratch &) = delete;

    // A tier refuses an empty path by ending the process, so every case checks this before using the directory
    // rather than letting a full or read-only temporary directory take the whole binary down.
    bool valid() const { return !path_.empty(); }
    const char *c_str() const { return path_.c_str(); }

private:
    std::string path_;
};

#endif // NETDATA_DBENGINE_TESTS_SUPPORT_H
