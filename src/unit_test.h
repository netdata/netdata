#ifndef NETDATA_UNIT_TEST_H
#define NETDATA_UNIT_TEST_H 1

/**
 * @file unit_test.h
 * @brief API to run tests.
 */

/**
 * Test storage_number.h
 *
 * @return 0 on success.
 */
extern int unit_test_storage(void);
/**
 * Run all unit tests.
 *
 * @param delay ktsaou: Your help needed
 * @param shift kstaou: Your help needed
 * @return 0 on success.
 */
extern int unit_test(long delay, long shift);
/**
 * Run all mockup tests.
 *
 * @return 0 on success
 */
extern int run_all_mockup_tests(void);

#endif /* NETDATA_UNIT_TEST_H */
