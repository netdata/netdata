/** @file query_test.h
 *  @brief Header of query_test.c 
 *
 *  @author Dimitris Pantazis
 */

#ifndef QUERY_TEST_H_
#define QUERY_TEST_H_

#ifdef _WIN32
#define LOGS_MANAGEMENT_STRESS_TEST_PIPENAME "\\\\?\\pipe\\netdata-logs-stress-test"
#else
#define LOGS_MANAGEMENT_STRESS_TEST_PIPENAME "/tmp/netdata-logs-stress-test"
#endif  // _WIN32

void run_stress_test_queries_thread(void *args);
void test_execute_query_thread(void *args);

#endif  // QUERY_TEST_H_
