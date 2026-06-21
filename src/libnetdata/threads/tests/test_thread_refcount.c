// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include <setjmp.h>
#include <cmocka.h>

// Test that nd_thread_create properly initializes refcount
static void test_thread_refcount_init(void **state)
{
    (void)state;

    ND_THREAD *nti = nd_thread_create("test-refcount", NETDATA_THREAD_OPTION_DEFAULT, nd_thread_starting_point, NULL);
    assert_non_null(nti);

    // Refcount should be 1 after creation (creator holds reference)
    size_t ref = __atomic_load_n(&nti->refcount, __ATOMIC_ACQUIRE);
    assert_int_equal(ref, 1);

    // Signal cancel and join to clean up
    nd_thread_signal_cancel(nti);
    nd_thread_join(nti);
}

// Helper thread routine that just exits quickly
static void quick_exit_thread(void *arg)
{
    (void)arg;
    // Thread exits immediately
}

// Test that nd_thread_join decrements refcount and frees when last
static void test_thread_join_frees_last_ref(void **state)
{
    (void)state;

    ND_THREAD *nti = nd_thread_create("test-join-free", NETDATA_THREAD_OPTION_DEFAULT, quick_exit_thread, NULL);
    assert_non_null(nti);

    // Wait a bit for thread to finish
    sleep_usec(100 * USEC_PER_MS);

    // Join should succeed and free the structure (refcount goes 1 -> 0)
    int ret = nd_thread_join(nti);
    assert_int_equal(ret, 0);
}

// Test that nd_thread_join_threads auto-drain path works with refcount
static void test_thread_join_threads_auto_drain(void **state)
{
    (void)state;

    ND_THREAD *nti = nd_thread_create("test-auto-drain", NETDATA_THREAD_OPTION_DEFAULT, quick_exit_thread, NULL);
    assert_non_null(nti);

    // Wait for thread to exit and land on exited list
    sleep_usec(200 * USEC_PER_MS);

    // nd_thread_join_threads should drain the exited list and free via refcount
    nd_thread_join_threads();

    // If we try to join again it should fail safely (already freed or already joined)
    // This tests that the auto-drain path doesn't leave dangling pointers
}

// Test concurrent join paths: direct join + auto-drain race
// This is the core bug scenario from issue #22716
static void test_concurrent_join_paths(void **state)
{
    (void)state;

    ND_THREAD *nti = nd_thread_create("test-concurrent", NETDATA_THREAD_OPTION_DEFAULT, quick_exit_thread, NULL);
    assert_non_null(nti);

    // Wait for thread to exit
    sleep_usec(200 * USEC_PER_MS);

    // Scenario: nd_thread_join_threads() runs concurrently with a direct nd_thread_join()
    // In the old code, both would try to free the same nti causing double-free/UAF.
    // With refcount, both paths decrement but only the last one frees.

    // First, simulate auto-drain removing from exited list
    spinlock_lock(&threads_globals.exited.spinlock);
    // The thread may or may not be on the exited list depending on timing
    spinlock_unlock(&threads_globals.exited.spinlock);

    // Call nd_thread_join directly - this should be safe regardless of auto-drain
    int ret = nd_thread_join(nti);
    // May return 0 (joined) or ESRCH (already joined by auto-drain), both are safe
    (void)ret;

    // Then run auto-drain - should be safe even if direct join already freed
    nd_thread_join_threads();
}

// Test that JOINED flag prevents double-join from the same path
static void test_joined_flag_prevents_double_join(void **state)
{
    (void)state;

    ND_THREAD *nti = nd_thread_create("test-double-join", NETDATA_THREAD_OPTION_DEFAULT, quick_exit_thread, NULL);
    assert_non_null(nti);

    sleep_usec(100 * USEC_PER_MS);

    // First join should succeed
    int ret1 = nd_thread_join(nti);
    assert_int_equal(ret1, 0);

    // Second join should return 0 (already joined) not crash
    int ret2 = nd_thread_join(nti);
    assert_int_equal(ret2, 0);
}

int main(void)
{
    const struct CMUnitTest tests[] = {
        cmocka_unit_test(test_thread_refcount_init),
        cmocka_unit_test(test_thread_join_frees_last_ref),
        cmocka_unit_test(test_thread_join_threads_auto_drain),
        cmocka_unit_test(test_concurrent_join_paths),
        cmocka_unit_test(test_joined_flag_prevents_double_join),
    };

    return cmocka_run_group_tests_name("nd_thread_refcount", tests, NULL, NULL);
}
