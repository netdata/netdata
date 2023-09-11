// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UNIT_TEST_H
#define NETDATA_UNIT_TEST_H 1

#include "stdbool.h"

int unit_test_storage(void);
int unit_test(long delay, long shift);
int run_all_mockup_tests(void);
int unit_test_str2ld(void);
int unit_test_buffer(void);
int unit_test_static_threads(void);
int test_sqlite(void);
int unit_test_bitmaps(void);
#ifdef ENABLE_DBENGINE
int test_dbengine(void);
void generate_dbengine_dataset(unsigned history_seconds);
void dbengine_stress_test(unsigned TEST_DURATION_SEC, unsigned DSET_CHARTS, unsigned QUERY_THREADS,
                                 unsigned RAMP_UP_SECONDS, unsigned PAGE_CACHE_MB, unsigned DISK_SPACE_MB);

#endif

bool command_argument_sanitization_tests();

#endif /* NETDATA_UNIT_TEST_H */
