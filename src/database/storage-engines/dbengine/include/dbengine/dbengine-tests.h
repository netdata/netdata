// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_H
#define NETDATA_DBENGINE_TESTS_H

#include "libnetdata/libnetdata.h"
#include "database/storage-engines/storage-engine-types.h"

// The engine's self-tests and benchmarks. Each runs to completion on the calling thread.
//
// Four are command-line modes for the embedder and return a process exit code. None of them needs a tier;
// dbengine_page_test(), dbengine_cache_unittest() and dbengine_metrics_registry_unittest() create a page cache,
// which reads the process-wide configuration, so dbengine_init() must have run before them;
// dbengine_metrics_registry_retention_benchmark() needs nothing.
//
// Two are for a test driver that adds their failed-check counts to its own: dbengine_cache_floor_unittest() runs
// before the engine is up (it checks what the caches fall back to without a main cache), and
// dbengine_zero_page_cadence_unittest() collects into and queries a tier the embedder brought up and hands it.

#ifdef __cplusplus
extern "C" {
#endif

int dbengine_page_test(int argc, char *argv[]);
int dbengine_cache_unittest(void);
int dbengine_metrics_registry_unittest(void);
int dbengine_metrics_registry_retention_benchmark(void);
int dbengine_cache_floor_unittest(void);
int dbengine_zero_page_cadence_unittest(STORAGE_INSTANCE *si);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_TESTS_H
