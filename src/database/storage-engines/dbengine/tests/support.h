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
#include <vector>

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

// One engine with tier 0 up and ready on a scratch directory of its own, torn down in the order the contract
// requires. A case that needs a differently configured engine or tier overrides engine_config() or tier_config();
// one that has to bring the engine down and up again in its body (a restart on the same directory) calls
// take_down() and bring_up() itself, and TearDown() then finds nothing left to do or a fresh engine to take down.
//
// The teardown does not quiesce: the daemon does that before its shutdown, but nothing the engine promises depends
// on it, and a case that wants the quiesced shape performs it in its body where the order is visible.
class EngineFixture : public ::testing::Test {
protected:
    void SetUp() override {
        ASSERT_TRUE(scratch_.valid()) << "could not make a scratch directory";
        bring_up();
    }

    void TearDown() override { take_down(); }

    virtual struct dbengine_config engine_config() { return netdata_test_config(); }

    virtual struct dbengine_tier_config tier_config(size_t tier) {
        struct dbengine_tier_config tc = {};
        tc.tier = tier;
        tc.dbfiles_path = scratch_.c_str();
        tc.disk_space_mb = 0;
        tc.max_retention_s = 0;
        tc.page_type = DBENGINE_PAGE_TYPE_GORILLA_32BIT;
        tc.grouping = 1;
        return tc;
    }

    // Creates the engine and brings tier 0 up on it; the assertions end the calling case when it does not come up.
    void bring_up() {
        ASSERT_EQ(engine_, nullptr) << "bring_up() on a fixture that already holds an engine";

        const struct dbengine_config cfg = engine_config();

        engine_ = dbengine_create(&cfg);
        ASSERT_NE(engine_, nullptr) << "the engine did not come up";

        const struct dbengine_tier_config tc = tier_config(0);
        ASSERT_EQ(dbengine_tier_init(engine_, &tc), 0) << "the tier did not come up";

        tier_ = dbengine_tier(engine_, 0);
        ASSERT_NE(tier_, nullptr);

        // Nothing may be collected or queried until the tier's registry load is done.
        dbengine_readiness_wait(tier_);

        si_ = reinterpret_cast<STORAGE_INSTANCE *>(tier_);
    }

    // Every tier that is still up is exited, then the engine is stopped and destroyed. The destroy must find
    // nothing referenced: a case that leaks a metric or a page handle fails here, in its own name.
    void take_down() {
        if (!engine_)
            return;

        for (size_t tier = 0; tier < RRD_STORAGE_TIERS; tier++) {
            DBENGINE_TIER *t = dbengine_tier(engine_, tier);
            if (dbengine_tier_is_active(t))
                dbengine_tier_exit(t);
        }

        dbengine_shutdown(engine_);
        EXPECT_EQ(dbengine_destroy(engine_), 0u) << "metrics stayed referenced across the destroy";

        engine_ = nullptr;
        tier_ = nullptr;
        si_ = nullptr;
    }

    DBENGINE_ENGINE *engine_ = nullptr;
    DBENGINE_TIER *tier_ = nullptr;
    STORAGE_INSTANCE *si_ = nullptr;
    Scratch scratch_;
};

// A metric nobody else in the process shares: uuid map ids are unique for the map's lifetime and it is never
// reset, so a fresh uuid per case keeps the cases independent of each other.
inline UUIDMAP_ID make_metric_id() {
    nd_uuid_t uuid;
    uuid_generate(uuid);
    return uuidmap_create(uuid);
}

struct Point {
    time_t start_time_s;
    time_t end_time_s;
    NETDATA_DOUBLE value;
};

inline void store_point(STORAGE_COLLECT_HANDLE *sch, time_t end_time_s, NETDATA_DOUBLE value) {
    dbengine_store_next(sch, static_cast<usec_t>(end_time_s) * USEC_PER_SEC, value, value, value, 1, 0,
                        SN_DEFAULT_FLAGS);
}

// Reads the whole query and returns what it gave, rather than asserting inside the loop: a case that expected three
// points and got two should say so once, with both lists in hand.
//
// Bounded by max_points: a query that never reports itself finished is a failure, not a reason to spin. The default
// suits the cases that store a handful of points; a case that stores pages of them passes a cap above what it
// expects back, because a cap below it fails the case rather than truncating the answer.
inline std::vector<STORAGE_POINT> query_all(STORAGE_METRIC_HANDLE *smh, time_t start_time_s, time_t end_time_s,
                                            STORAGE_PRIORITY priority = STORAGE_PRIORITY_SYNCHRONOUS,
                                            size_t max_points = 64) {
    std::vector<STORAGE_POINT> points;

    struct storage_engine_query_handle seqh = {};
    dbengine_query_init(smh, &seqh, start_time_s, end_time_s, priority);

    while (points.size() < max_points && !dbengine_query_is_finished(&seqh))
        points.push_back(dbengine_query_next(&seqh));

    EXPECT_TRUE(dbengine_query_is_finished(&seqh)) << "the query did not finish within " << max_points << " points";

    dbengine_query_finalize(&seqh);
    return points;
}

inline void expect_point(const STORAGE_POINT &sp, const Point &expected) {
    SCOPED_TRACE(static_cast<long long>(expected.end_time_s));

    EXPECT_EQ(sp.start_time_s, expected.start_time_s);
    EXPECT_EQ(sp.end_time_s, expected.end_time_s);
    EXPECT_DOUBLE_EQ(sp.min, expected.value);
    EXPECT_DOUBLE_EQ(sp.max, expected.value);
    EXPECT_DOUBLE_EQ(sp.sum, expected.value);
    EXPECT_EQ(sp.count, 1u);
    EXPECT_EQ(sp.anomaly_count, 0u);
    EXPECT_EQ(sp.flags, SN_DEFAULT_FLAGS);
    EXPECT_FALSE(storage_point_is_gap(sp));
}

#endif // NETDATA_DBENGINE_TESTS_SUPPORT_H
