// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UNIT_TEST_H
#define NETDATA_UNIT_TEST_H 1

extern int unit_test_storage(void);
extern int unit_test(long delay, long shift);
extern int run_all_mockup_tests(void);
extern int unit_test_str2ld(void);
extern int unit_test_buffer(void);
#ifdef ENABLE_DBENGINE
extern int test_dbengine(void);
extern void generate_dbengine_dataset(unsigned history_seconds);
extern void dbengine_stress_test(unsigned TEST_DURATION_SEC, unsigned DSET_CHARTS, unsigned QUERY_THREADS,
                                 unsigned RAMP_UP_SECONDS, unsigned PAGE_CACHE_MB, unsigned DISK_SPACE_MB);

#endif

#endif /* NETDATA_UNIT_TEST_H */
