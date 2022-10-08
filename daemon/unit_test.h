// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_UNIT_TEST_H
#define NETDATA_UNIT_TEST_H

#ifdef ENABLE_DBENGINE
int test_dbengine(void);
void generate_dbengine_dataset(unsigned history_seconds);
void dbengine_stress_test(unsigned TEST_DURATION_SEC, unsigned DSET_CHARTS, unsigned QUERY_THREADS,
                                 unsigned RAMP_UP_SECONDS, unsigned PAGE_CACHE_MB, unsigned DISK_SPACE_MB);
#endif

#endif /* NETDATA_UNIT_TEST_H */
