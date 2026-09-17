// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DBENGINE_TESTS_H
#define NETDATA_DBENGINE_TESTS_H

// The engine's self-tests and benchmarks, for an embedder that offers them as command-line modes. Each runs
// to completion on the calling thread and returns a process exit code. None needs a tier; pgd_test(),
// pgc_unittest() and mrg_unittest() create a page cache, which reads the process-wide configuration, so
// dbengine_init() must have run before them; mrg_retention_benchmark() needs nothing.
//
// rrdeng_zero_page_cadence_unittest() is the exception: it collects into and queries a tier the embedder brought
// up and hands it, and returns the number of failed checks, for a test driver that adds it to its own count.

#ifdef __cplusplus
extern "C" {
#endif

int pgd_test(int argc, char *argv[]);
int pgc_unittest(void);
int mrg_unittest(void);
int mrg_retention_benchmark(void);
int rrdeng_zero_page_cadence_unittest(STORAGE_INSTANCE *si);

#ifdef __cplusplus
}
#endif

#endif // NETDATA_DBENGINE_TESTS_H
