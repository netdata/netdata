// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "streaming/stream-receiver-internals.h"
#include "web/server/web_client.h"

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

static bool check_metadata_bounds(void) {
    char maximum[STREAM_RECEIVER_METADATA_MAX_LENGTH + 1];
    memset(maximum, 'x', sizeof(maximum) - 1);
    maximum[sizeof(maximum) - 1] = '\0';

    char oversized[STREAM_RECEIVER_METADATA_MAX_LENGTH + 2];
    memset(oversized, 'x', sizeof(oversized) - 1);
    oversized[sizeof(oversized) - 1] = '\0';

    if(!stream_receiver_metadata_size_is_valid(maximum) || stream_receiver_metadata_size_is_valid(oversized)) {
        fprintf(stderr, "stream metadata size boundary is incorrect\n");
        return false;
    }

    if(stream_receiver_metadata_should_reject(
           true, STREAM_RECEIVER_METADATA_MAX_LENGTH, STREAM_RECEIVER_METADATA_MAX_LENGTH) ||
       !stream_receiver_metadata_should_reject(
           true, STREAM_RECEIVER_METADATA_MAX_LENGTH, STREAM_RECEIVER_METADATA_MAX_LENGTH + 1) ||
       stream_receiver_metadata_should_reject(
           false, STREAM_RECEIVER_METADATA_MAX_LENGTH, STREAM_RECEIVER_METADATA_MAX_LENGTH + 1) ||
       stream_receiver_metadata_should_reject(
           false, STREAM_RECEIVER_METADATA_MAX_LENGTH + 1, STREAM_RECEIVER_METADATA_MAX_LENGTH + 1)) {
        fprintf(stderr, "stream metadata recognition policy is incorrect\n");
        return false;
    }

    return true;
}

static bool check_unused_metadata_aggregation(void) {
    struct stream_receiver_unused_metadata unused = { 0 };
    size_t request_overhead = sizeof("STREAM ") - 1 + sizeof(" HTTP/1.1\r\n\r\n") - 1;
    size_t field_size = sizeof("x=x&") - 1;
    size_t fields = (NETDATA_WEB_REQUEST_MAX_SIZE - request_overhead) / field_size;

    for(size_t i = 0; i < fields; i++)
        stream_receiver_unused_metadata_add(&unused, 1, 1);

    if(unused.fields != fields || unused.name_bytes != fields || unused.value_bytes != fields) {
        fprintf(stderr, "stream unused metadata aggregation is incorrect\n");
        return false;
    }

    return true;
}

static bool check_user_agent(void) {
    char *program_name = NULL;
    char *program_version = NULL;

    stream_receiver_parse_user_agent("netdata/v1.2.3", &program_name, &program_version);
    bool valid = program_name && program_version && !strcmp(program_name, "netdata") &&
                 !strcmp(program_version, "v1.2.3");
    freez(program_name);
    freez(program_version);
    if(!valid) {
        fprintf(stderr, "normal streaming user agent was not split correctly\n");
        return false;
    }

    size_t oversized_length = STREAM_RECEIVER_METADATA_MAX_LENGTH + 100;
    char *oversized = mallocz(oversized_length * 2 + 2);
    memset(oversized, 'n', oversized_length);
    oversized[oversized_length] = '/';
    memset(oversized + oversized_length + 1, 'v', oversized_length);
    oversized[oversized_length * 2 + 1] = '\0';

    stream_receiver_parse_user_agent(oversized, &program_name, &program_version);
    valid = program_name && program_version && strlen(program_name) == STREAM_RECEIVER_METADATA_MAX_LENGTH &&
            strlen(program_version) == STREAM_RECEIVER_METADATA_MAX_LENGTH;
    freez(program_name);
    freez(program_version);
    freez(oversized);
    if(!valid) {
        fprintf(stderr, "oversized streaming user agent fields were not bounded\n");
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

    if(!check_metadata_bounds() || !check_unused_metadata_aggregation() || !check_user_agent())
        return 1;

    return 0;
}
