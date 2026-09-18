// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_H
#define NETDATA_DBENGINE_TESTS_H

#include "libnetdata/libnetdata.h"
#include "database/storage-engines/storage-engine-types.h"
#include "dbengine-config.h"

// The engine's self-tests and benchmarks. Each runs to completion on the calling thread.
//
// Four are command-line modes for the embedder and return a process exit code. None of them needs a tier or the
// engine; dbengine_page_test(), dbengine_cache_unittest() and dbengine_metrics_registry_unittest() create a page
// cache, which reads the process-wide configuration, so they take it from the embedder and set it themselves;
// dbengine_metrics_registry_retention_benchmark() needs nothing.
//
// Two are for a test driver that adds their failed-check counts to its own: dbengine_cache_floor_unittest() runs
// before the engine is up (it checks what the caches fall back to without a main cache), and
// dbengine_zero_page_cadence_unittest() collects into and queries a tier the embedder brought up and hands it.

#ifdef __cplusplus
extern "C" {
#endif

int dbengine_page_test(const struct dbengine_config *cfg, int argc, char *argv[]);
int dbengine_cache_unittest(const struct dbengine_config *cfg);
int dbengine_metrics_registry_unittest(const struct dbengine_config *cfg);
int dbengine_metrics_registry_retention_benchmark(void);
int dbengine_cache_floor_unittest(void);
int dbengine_zero_page_cadence_unittest(STORAGE_INSTANCE *si);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_TESTS_H
