// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_H
#define NETDATA_DBENGINE_TESTS_H

// The engine's self-tests and benchmarks, for an embedder that offers them as command-line modes. Each runs
// to completion on the calling thread and returns a process exit code; none needs an initialised engine.

#ifdef __cplusplus
extern "C" {
#endif

int pgd_test(int argc, char *argv[]);
int pgc_unittest(void);
int mrg_unittest(void);
int mrg_retention_benchmark(void);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_TESTS_H
