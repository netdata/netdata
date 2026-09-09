// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "streaming/stream-receiver-internals.h"

typedef struct {
    const char *value;
    bool valid;
    int16_t expected;
} HOPS_TEST;

static bool check_hops(const HOPS_TEST *test) {
    int16_t hops = 12345;
    bool valid = stream_receiver_parse_hops(test->value, &hops);

    if(valid != test->valid) {
        fprintf(stderr, "hops value '%s' was %s, expected %s\n",
                test->value ? test->value : "(null)",
                valid ? "accepted" : "rejected",
                test->valid ? "accepted" : "rejected");
        return false;
    }

    if(valid && hops != test->expected) {
        fprintf(stderr, "hops value '%s' parsed as %d, expected %d\n",
                test->value, hops, test->expected);
        return false;
    }

    if(!valid && hops != 12345) {
        fprintf(stderr, "rejected hops value '%s' changed the output to %d\n",
                test->value ? test->value : "(null)", hops);
        return false;
    }

    return true;
}

int main(void) {
    static const HOPS_TEST tests[] = {
        { .value = "1", .valid = true, .expected = 1 },
        { .value = "42", .valid = true, .expected = 42 },
        { .value = "32767", .valid = true, .expected = INT16_MAX },
        { .value = "+42", .valid = true, .expected = 42 },
        { .value = " 42", .valid = true, .expected = 42 },
        { .value = "0x2a", .valid = true, .expected = 42 },
        { .value = "052", .valid = true, .expected = 42 },

        { .value = NULL, .valid = false },
        { .value = "", .valid = false },
        { .value = " ", .valid = false },
        { .value = "+", .valid = false },
        { .value = "-", .valid = false },
        { .value = "0", .valid = false },
        { .value = "-0", .valid = false },
        { .value = "-1", .valid = false },
        { .value = "-32768", .valid = false },
        { .value = "-32769", .valid = false },
        { .value = "32768", .valid = false },
        { .value = "65535", .valid = false },
        { .value = "65536", .valid = false },
        { .value = "0x8000", .valid = false },
        { .value = "abc", .valid = false },
        { .value = "0x", .valid = false },
        { .value = "12abc", .valid = false },
        { .value = "1 ", .valid = false },
        { .value = "1\n", .valid = false },
        { .value = "9223372036854775807", .valid = false },
        { .value = "9223372036854775808", .valid = false },
        { .value = "-9223372036854775808", .valid = false },
        { .value = "-9223372036854775809", .valid = false },
    };

    for(size_t i = 0; i < _countof(tests); i++) {
        if(!check_hops(&tests[i]))
            return 1;
    }

    if(stream_receiver_parse_hops("1", NULL)) {
        fprintf(stderr, "accepted a null output pointer\n");
        return 1;
    }

    return 0;
}
