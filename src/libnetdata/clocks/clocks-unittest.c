// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "clocks-internals.h"

#define CLOCKS_TEST(condition, msg) do {                                         \
        if(!(condition)) {                                                       \
            fprintf(stderr, "clocks unittest FAILED: %s (%s:%d)\n",             \
                    (msg), __FUNCTION__, __LINE__);                              \
            errors++;                                                            \
        }                                                                        \
    } while(0)

static int clocks_retry_keeps_valid_remaining_time(void) {
    int errors = 0;
    struct timespec req = {
        .tv_sec = 0,
        .tv_nsec = 123 * NSEC_PER_MSEC,
    };

    bool retry = sleep_usec_prepare_retry_after_eintr(
            1 * USEC_PER_SEC,
            100 * USEC_PER_MS,
            250 * USEC_PER_MS,
            &req);

    CLOCKS_TEST(retry, "retry continues while monotonic budget remains");
    CLOCKS_TEST(req.tv_sec == 0, "valid remaining seconds are preserved");
    CLOCKS_TEST(req.tv_nsec == (long)(123 * NSEC_PER_MSEC), "valid remaining nanoseconds are preserved");

    return errors;
}

static int clocks_retry_clamps_inflated_remaining_time(void) {
    int errors = 0;
    struct timespec req = {
        .tv_sec = 0,
        .tv_nsec = 300 * NSEC_PER_MSEC,
    };

    bool retry = sleep_usec_prepare_retry_after_eintr(
            200 * USEC_PER_MS,
            1 * USEC_PER_SEC,
            1 * USEC_PER_SEC + 150 * USEC_PER_MS,
            &req);

    CLOCKS_TEST(retry, "retry continues after clamp when budget remains");
    CLOCKS_TEST(req.tv_sec == 0, "clamped seconds stay below one second");
    CLOCKS_TEST(req.tv_nsec == (long)(50 * NSEC_PER_MSEC), "remaining time is clamped to monotonic budget");

    return errors;
}

static int clocks_retry_stops_when_budget_is_exhausted(void) {
    int errors = 0;
    struct timespec req = {
        .tv_sec = 0,
        .tv_nsec = 100 * NSEC_PER_MSEC,
    };

    bool retry = sleep_usec_prepare_retry_after_eintr(
            200 * USEC_PER_MS,
            1 * USEC_PER_SEC,
            1 * USEC_PER_SEC + 200 * USEC_PER_MS,
            &req);

    CLOCKS_TEST(!retry, "retry stops once monotonic budget is exhausted");

    return errors;
}

static int clocks_retry_treats_backward_monotonic_sample_as_no_elapsed_time(void) {
    int errors = 0;
    struct timespec req = {
        .tv_sec = 2,
        .tv_nsec = 0,
    };

    bool retry = sleep_usec_prepare_retry_after_eintr(
            1 * USEC_PER_SEC,
            2 * USEC_PER_SEC,
            1 * USEC_PER_SEC,
            &req);

    CLOCKS_TEST(retry, "retry continues when elapsed time is clamped to zero");
    CLOCKS_TEST(req.tv_sec == 1, "backward monotonic sample clamps to full budget seconds");
    CLOCKS_TEST(req.tv_nsec == 0, "backward monotonic sample clamps to full budget nanoseconds");

    return errors;
}

static int clocks_usec_delta_or_zero_saturates_backward_samples(void) {
    int errors = 0;

    CLOCKS_TEST(clocks_usec_delta_or_zero(250 * USEC_PER_MS, 100 * USEC_PER_MS) == 150 * USEC_PER_MS,
                "normal unsigned microsecond delta is preserved");
    CLOCKS_TEST(clocks_usec_delta_or_zero(100 * USEC_PER_MS, 100 * USEC_PER_MS) == 0,
                "equal unsigned microsecond samples produce zero delta");
    CLOCKS_TEST(clocks_usec_delta_or_zero(100 * USEC_PER_MS, 250 * USEC_PER_MS) == 0,
                "backward unsigned microsecond sample saturates to zero");

    return errors;
}

int clocks_unittest(void) {
    int errors = 0;

    fprintf(stderr, "\nrunning clocks unittest\n");

    errors += clocks_retry_keeps_valid_remaining_time();
    errors += clocks_retry_clamps_inflated_remaining_time();
    errors += clocks_retry_stops_when_budget_is_exhausted();
    errors += clocks_retry_treats_backward_monotonic_sample_as_no_elapsed_time();
    errors += clocks_usec_delta_or_zero_saturates_backward_samples();

    if(errors)
        fprintf(stderr, "clocks unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "clocks unittest: OK\n");

    return errors;
}
