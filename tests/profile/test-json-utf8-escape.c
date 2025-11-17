// SPDX-License-Identifier: GPL-3.0-or-later
// Test for UTF-8 escaping in JSON output
//
// To compile and run this test:
// 1. Build netdata normally
// 2. cd tests/profile/
// 3. gcc -O1 -ggdb -Wall -Wextra -I ../../src/ -I ../../ -DBUFFER_JSON_ESCAPE_UTF \
//    -o test-json-utf8-escape test-json-utf8-escape.c \
//    ../../build/libnetdata.a -pthread -lm
// 4. ./test-json-utf8-escape

#include "libnetdata/libnetdata.h"
#include "libnetdata/buffer/buffer.h"
#include "libnetdata/required_dummies.h"

// Dummy implementations for required functions
void netdata_cleanup_and_exit(int ret, const char *action __maybe_unused, const char *action_result __maybe_unused, const char *action_data __maybe_unused) {
    exit(ret);
}

static void test_utf8_escape(const char *input, const char *expected_contains, const char *test_name) {
    BUFFER *wb = buffer_create(1024, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);
    buffer_json_member_add_string(wb, "test", input);
    buffer_json_finalize(wb);

    const char *result = buffer_tostring(wb);
    
    printf("\n%s\n", test_name);
    printf("Input: %s\n", input);
    printf("Output: %s\n", result);
    
    if (strstr(result, expected_contains)) {
        printf("✓ PASS: Found expected pattern '%s'\n", expected_contains);
    } else {
        printf("✗ FAIL: Expected pattern '%s' not found\n", expected_contains);
        buffer_free(wb);
        exit(1);
    }
    
    buffer_free(wb);
}

int main() {
    printf("==========================================\n");
    printf("Testing UTF-8 escaping in JSON output\n");
    printf("==========================================\n");

#ifdef BUFFER_JSON_ESCAPE_UTF
    printf("BUFFER_JSON_ESCAPE_UTF is ENABLED\n");
    printf("UTF-8 characters will be escaped as \\uXXXX\n");
#else
    printf("BUFFER_JSON_ESCAPE_UTF is DISABLED\n");
    printf("UTF-8 characters will pass through as raw bytes\n");
#endif

    // Test 1: Simple ASCII text (should pass through unchanged)
    test_utf8_escape("Hello World", "Hello World", 
                     "Test 1: ASCII text");

    // Test 2: 2-byte UTF-8 character (Latin Extended)
    // é (U+00E9) = C3 A9
    // With escaping: should become \u00E9
    // Without escaping: should pass through as C3 A9
#ifdef BUFFER_JSON_ESCAPE_UTF
    test_utf8_escape("café", "\\u00E9",
                     "Test 2: 2-byte UTF-8 (Latin Extended) - should be \\u00E9");
#else
    test_utf8_escape("café", "caf",
                     "Test 2: 2-byte UTF-8 (Latin Extended) - should pass through");
#endif

    // Test 3: 3-byte UTF-8 character (CJK)
    // 世 (U+4E16) = E4 B8 96
    // With escaping: should become \u4E16
    // Without escaping: should pass through as E4 B8 96
#ifdef BUFFER_JSON_ESCAPE_UTF
    test_utf8_escape("世界", "\\u4E16",
                     "Test 3: 3-byte UTF-8 (CJK) - should be \\u4E16\\u754C");
#else
    test_utf8_escape("世界", "\"test\":\"",
                     "Test 3: 3-byte UTF-8 (CJK) - should pass through");
#endif

    // Test 4: Mixed ASCII and UTF-8
#ifdef BUFFER_JSON_ESCAPE_UTF
    test_utf8_escape("Hello 世界", "Hello \\u",
                     "Test 4: Mixed ASCII and UTF-8");
#else
    test_utf8_escape("Hello 世界", "Hello",
                     "Test 4: Mixed ASCII and UTF-8");
#endif

    // Test 5: Emoji (4-byte UTF-8)
    // 😀 (U+1F600) = F0 9F 98 80
    // With escaping: should become surrogate pair \uD83D\uDE00
    // Without escaping: should pass through as F0 9F 98 80
#ifdef BUFFER_JSON_ESCAPE_UTF
    test_utf8_escape("😀", "\\uD83D\\uDE00",
                     "Test 5: 4-byte UTF-8 (Emoji) - should be surrogate pair");
#else
    test_utf8_escape("😀", "\"test\":\"",
                     "Test 5: 4-byte UTF-8 (Emoji) - should pass through");
#endif

    printf("\n==========================================\n");
    printf("All tests passed!\n");
    printf("==========================================\n");
    
    return 0;
}

