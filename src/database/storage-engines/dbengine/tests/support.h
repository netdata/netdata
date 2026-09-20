// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_SUPPORT_H
#define NETDATA_DBENGINE_TESTS_SUPPORT_H

// Shared by every test in this suite. It reaches the engine only through its public headers, the ones an embedder
// that is not the daemon would include; check-public-includes.sh enforces that for every file here whose name does
// not begin with internal_.

#include <gtest/gtest.h>

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

#endif // NETDATA_DBENGINE_TESTS_SUPPORT_H
