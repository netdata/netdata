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

static int clocks_time_t_arithmetic_preserves_order(void) {
    int errors = 0;
    const time_t maximum = nd_time_t_max();
    const time_t minimum = nd_time_t_min();

    CLOCKS_TEST(nd_time_t_add_saturating(100, 23) == 123,
                "representable positive time_t addition is preserved");
    CLOCKS_TEST(nd_time_t_add_saturating(100, -23) == 77,
                "representable negative time_t addition is preserved");
    CLOCKS_TEST(nd_time_t_add_saturating(maximum, 1) == maximum,
                "positive time_t overflow saturates at the maximum");
    CLOCKS_TEST(nd_time_t_add_saturating(minimum, -1) == minimum,
                "negative time_t overflow saturates at the minimum");

    CLOCKS_TEST(nd_time_t_add_compare(maximum, 1, maximum) > 0,
                "an unrepresentable future sum remains after every time_t");
    CLOCKS_TEST(nd_time_t_add_compare(minimum, -1, minimum) < 0,
                "an unrepresentable past sum remains before every time_t");
    CLOCKS_TEST(nd_time_t_add_compare(maximum, -1, maximum) < 0,
                "a representable boundary subtraction preserves ordering");
    CLOCKS_TEST(nd_time_t_add_compare(minimum, 1, minimum) > 0,
                "a representable boundary addition preserves ordering");
    CLOCKS_TEST(nd_time_t_add_compare(100, 23, 123) == 0,
                "representable time_t addition preserves equality");

    intmax_t combined_offset = (intmax_t)INT_MAX + (intmax_t)INT_MAX - (intmax_t)INT_MAX;
    CLOCKS_TEST(nd_time_t_add_compare(maximum - INT_MAX, combined_offset, maximum) == 0,
                "combined offsets are compared after mathematical cancellation");

    if(sizeof(time_t) < sizeof(intmax_t)) {
        intmax_t beyond_time_t = (intmax_t)maximum + 1;
        CLOCKS_TEST(nd_time_t_add_saturating(0, beyond_time_t) == maximum,
                    "wide positive offset saturates on narrower time_t");
        CLOCKS_TEST(nd_time_t_add_compare(0, beyond_time_t, maximum) > 0,
                    "wide positive offset compares beyond narrower time_t");
    }

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
    errors += clocks_time_t_arithmetic_preserves_order();

    if(errors)
        fprintf(stderr, "clocks unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "clocks unittest: OK\n");

    return errors;
}
