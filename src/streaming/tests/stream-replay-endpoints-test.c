// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "streaming/stream-sender-internals.h"

typedef struct {
    const char *value;
    bool valid;
    time_t expected;
} REPLAY_ENDPOINT_TEST;

static bool check_endpoint(const REPLAY_ENDPOINT_TEST *test) {
    time_t endpoint = 12345;
    bool valid = stream_sender_parse_replay_endpoint(test->value, &endpoint);

    if(valid != test->valid) {
        fprintf(stderr, "replay endpoint '%s' was %s, expected %s\n",
                test->value ? test->value : "(null)",
                valid ? "accepted" : "rejected",
                test->valid ? "accepted" : "rejected");
        return false;
    }

    if(valid && endpoint != test->expected) {
        fprintf(stderr, "replay endpoint '%s' parsed as %" PRIdMAX ", expected %" PRIdMAX "\n",
                test->value, (intmax_t)endpoint, (intmax_t)test->expected);
        return false;
    }

    if(!valid && endpoint != 12345) {
        fprintf(stderr, "rejected replay endpoint '%s' changed the output to %" PRIdMAX "\n",
                test->value ? test->value : "(null)", (intmax_t)endpoint);
        return false;
    }

    return true;
}

int main(void) {
    uintmax_t maximum = (uintmax_t)nd_time_t_max();
    if(maximum > (uintmax_t)ULONG_MAX)
        maximum = (uintmax_t)ULONG_MAX;
    if(maximum > (uintmax_t)SIZE_MAX)
        maximum = (uintmax_t)SIZE_MAX;

    char maximum_string[64];
    char above_maximum_string[64];
    snprintf(maximum_string, sizeof(maximum_string), "%" PRIuMAX, maximum);
    snprintf(above_maximum_string, sizeof(above_maximum_string), "%" PRIuMAX, maximum + 1);

    const REPLAY_ENDPOINT_TEST tests[] = {
        { .value = "0", .valid = true, .expected = 0 },
        { .value = "1", .valid = true, .expected = 1 },
        { .value = "010", .valid = true, .expected = 10 },
        { .value = maximum_string, .valid = true, .expected = (time_t)maximum },

        { .value = NULL, .valid = false },
        { .value = "", .valid = false },
        { .value = " ", .valid = false },
        { .value = " 1", .valid = false },
        { .value = "1 ", .valid = false },
        { .value = "+1", .valid = false },
        { .value = "-1", .valid = false },
        { .value = "0x10", .valid = false },
        { .value = "12x", .valid = false },
        { .value = above_maximum_string, .valid = false },
        { .value = "18446744073709551616", .valid = false },
    };

    for(size_t i = 0; i < _countof(tests); i++) {
        if(!check_endpoint(&tests[i]))
            return 1;
    }

    if(stream_sender_parse_replay_endpoint("1", NULL)) {
        fprintf(stderr, "accepted a null replay endpoint output pointer\n");
        return 1;
    }

    errno = EDOM;
    time_t errno_endpoint;
    if(!stream_sender_parse_replay_endpoint("1", &errno_endpoint) || errno != EDOM) {
        fprintf(stderr, "valid replay endpoint parsing changed errno\n");
        return 1;
    }

    errno = E2BIG;
    if(stream_sender_parse_replay_endpoint("18446744073709551616", &errno_endpoint) || errno != E2BIG) {
        fprintf(stderr, "invalid replay endpoint parsing changed errno\n");
        return 1;
    }

    time_t after = 12345;
    time_t before = 12345;
    if(!stream_sender_parse_replay_endpoints("10", "20", &after, &before) || after != 10 || before != 20) {
        fprintf(stderr, "failed to preserve a valid replay endpoint pair\n");
        return 1;
    }

    if(stream_sender_parse_replay_endpoints("invalid", "20", &after, &before) || after != 0 || before != 0) {
        fprintf(stderr, "failed to normalize an invalid replay endpoint pair\n");
        return 1;
    }

    return 0;
}
