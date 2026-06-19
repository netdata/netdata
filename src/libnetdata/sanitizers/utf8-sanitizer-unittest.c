// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ============================================================================
// TEST INFRASTRUCTURE
// ============================================================================

static int tests_run = 0;
static int tests_passed = 0;
static int tests_failed = 0;

// Identity char_map - everything passes through unchanged
static unsigned char identity_char_map[256];

// Test char_map similar to rrd_string_allowed_chars
static unsigned char test_rrd_char_map[256];

static void init_char_maps(void) {
    // Identity map
    for (int i = 0; i < 256; i++)
        identity_char_map[i] = (unsigned char)i;
    identity_char_map[0] = '\0';

    // RRD-like map
    for (int i = 0; i < 256; i++)
        test_rrd_char_map[i] = (unsigned char)i;

    // Control characters (0-31, 127) → space
    for (int i = 1; i < 32; i++)
        test_rrd_char_map[i] = ' ';
    test_rrd_char_map[127] = ' ';

    // High bytes (128-255) → space (fallback for orphan UTF-8 bytes)
    for (int i = 128; i < 256; i++)
        test_rrd_char_map[i] = ' ';

    test_rrd_char_map[0] = '\0';
    test_rrd_char_map['"'] = '\'';   // double quote → single quote
    test_rrd_char_map['\\'] = '/';   // backslash → forward slash
}

// Test result macro
#define TEST_ASSERT(name, condition, ...) do { \
    tests_run++; \
    if (condition) { \
        tests_passed++; \
    } else { \
        tests_failed++; \
        fprintf(stderr, "FAILED [%s]: ", name); \
        fprintf(stderr, __VA_ARGS__); \
        fprintf(stderr, "\n"); \
    } \
} while(0)

// Helper to run a single sanitize test with overflow detection
typedef struct {
    const char *name;
    const unsigned char *input;
    size_t dst_size;
    const unsigned char *char_map;
    bool utf;
    const char *empty;
    const char *expected_output;
    size_t expected_len;
    size_t expected_mblen;
} sanitize_test_t;

static void run_sanitize_test(const sanitize_test_t *t) {
    // Allocate with guard bytes
    size_t guard = 16;
    unsigned char *buffer = callocz(1, t->dst_size + guard * 2);
    unsigned char *dst = buffer + guard;

    // Fill guards
    memset(buffer, 0xAA, guard);
    memset(dst + t->dst_size, 0xBB, guard);
    memset(dst, 0xCC, t->dst_size);

    size_t mblen = 0;
    size_t len = text_sanitize(dst, t->input, t->dst_size, t->char_map, t->utf, t->empty, &mblen);

    // Check overflow
    bool overflow_before = false, overflow_after = false;
    for (size_t i = 0; i < guard; i++) {
        if (buffer[i] != 0xAA) overflow_before = true;
        if (dst[t->dst_size + i] != 0xBB) overflow_after = true;
    }

    TEST_ASSERT(t->name, !overflow_before && !overflow_after,
        "Buffer overflow! before=%d after=%d", overflow_before, overflow_after);

    TEST_ASSERT(t->name, len == t->expected_len,
        "Length mismatch: expected %zu, got %zu", t->expected_len, len);

    TEST_ASSERT(t->name, strcmp((char *)dst, t->expected_output) == 0,
        "Content mismatch: expected '%s', got '%s'", t->expected_output, dst);

    if (t->expected_mblen > 0) {
        TEST_ASSERT(t->name, mblen == t->expected_mblen,
            "Multibyte length mismatch: expected %zu, got %zu", t->expected_mblen, mblen);
    }

    // Verify null termination
    TEST_ASSERT(t->name, dst[len] == '\0',
        "Missing null terminator at position %zu", len);

    freez(buffer);
}

// ============================================================================
// TEST: VALID UTF-8 SEQUENCES
// ============================================================================

static void test_valid_utf8_sequences(void) {
    fprintf(stderr, "\n=== Valid UTF-8 Sequences ===\n");

    // 2-byte: Latin characters with diacritics
    {
        sanitize_test_t t = {
            .name = "utf8_2byte_e_acute",
            .input = (unsigned char *)"caf\xC3\xA9",  // café
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "caf\xC3\xA9", .expected_len = 5, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // 2-byte: Superscript ² (U+00B2)
    {
        sanitize_test_t t = {
            .name = "utf8_2byte_superscript2",
            .input = (unsigned char *)"m/s\xC2\xB2",  // m/s²
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "m/s\xC2\xB2", .expected_len = 5, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // 2-byte: Degree symbol ° (U+00B0)
    {
        sanitize_test_t t = {
            .name = "utf8_2byte_degree",
            .input = (unsigned char *)"25\xC2\xB0""C",  // 25°C
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "25\xC2\xB0""C", .expected_len = 5, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // 2-byte: Micro sign µ (U+00B5)
    {
        sanitize_test_t t = {
            .name = "utf8_2byte_micro",
            .input = (unsigned char *)"\xC2\xB5s",  // µs
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC2\xB5s", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // 3-byte: Euro sign € (U+20AC)
    {
        sanitize_test_t t = {
            .name = "utf8_3byte_euro",
            .input = (unsigned char *)"100\xE2\x82\xAC",  // 100€
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "100\xE2\x82\xAC", .expected_len = 6, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // 3-byte: Japanese hiragana あ (U+3042)
    {
        sanitize_test_t t = {
            .name = "utf8_3byte_hiragana",
            .input = (unsigned char *)"\xE3\x81\x82",  // あ
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xE3\x81\x82", .expected_len = 3, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // 3-byte: Chinese character 中 (U+4E2D)
    {
        sanitize_test_t t = {
            .name = "utf8_3byte_chinese",
            .input = (unsigned char *)"\xE4\xB8\xAD\xE6\x96\x87",  // 中文
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xE4\xB8\xAD\xE6\x96\x87", .expected_len = 6, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // 4-byte: Emoji 😀 (U+1F600)
    {
        sanitize_test_t t = {
            .name = "utf8_4byte_emoji",
            .input = (unsigned char *)"hi\xF0\x9F\x98\x80",  // hi😀
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "hi\xF0\x9F\x98\x80", .expected_len = 6, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // 4-byte: Mathematical bold A 𝐀 (U+1D400)
    {
        sanitize_test_t t = {
            .name = "utf8_4byte_math",
            .input = (unsigned char *)"\xF0\x9D\x90\x80",  // 𝐀
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xF0\x9D\x90\x80", .expected_len = 4, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Mixed: ASCII + 2-byte + 3-byte + 4-byte
    {
        sanitize_test_t t = {
            .name = "utf8_mixed_all_types",
            .input = (unsigned char *)"A\xC2\xB0\xE2\x82\xAC\xF0\x9F\x98\x80",  // A°€😀
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "A\xC2\xB0\xE2\x82\xAC\xF0\x9F\x98\x80", .expected_len = 10, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // Multiple same-type UTF-8 characters
    {
        sanitize_test_t t = {
            .name = "utf8_multiple_2byte",
            .input = (unsigned char *)"\xC3\xA9\xC3\xA8\xC3\xA0",  // éèà
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC3\xA9\xC3\xA8\xC3\xA0", .expected_len = 6, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // UTF-8 at beginning of string
    {
        sanitize_test_t t = {
            .name = "utf8_at_beginning",
            .input = (unsigned char *)"\xC2\xB5sec",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC2\xB5sec", .expected_len = 5, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // UTF-8 in middle of string
    {
        sanitize_test_t t = {
            .name = "utf8_in_middle",
            .input = (unsigned char *)"pre\xC2\xB0post",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "pre\xC2\xB0post", .expected_len = 9, .expected_mblen = 8
        };
        run_sanitize_test(&t);
    }

    // UTF-8 at end of string
    {
        sanitize_test_t t = {
            .name = "utf8_at_end",
            .input = (unsigned char *)"temp\xC2\xB0",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "temp\xC2\xB0", .expected_len = 6, .expected_mblen = 5
        };
        run_sanitize_test(&t);
    }

    // Boundary: Minimum 2-byte (U+0080)
    {
        sanitize_test_t t = {
            .name = "utf8_2byte_min",
            .input = (unsigned char *)"\xC2\x80",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC2\x80", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Boundary: Maximum 2-byte (U+07FF)
    {
        sanitize_test_t t = {
            .name = "utf8_2byte_max",
            .input = (unsigned char *)"\xDF\xBF",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xDF\xBF", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Boundary: Minimum 3-byte (U+0800)
    {
        sanitize_test_t t = {
            .name = "utf8_3byte_min",
            .input = (unsigned char *)"\xE0\xA0\x80",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xE0\xA0\x80", .expected_len = 3, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Boundary: Maximum 3-byte (U+FFFF, excluding surrogates)
    {
        sanitize_test_t t = {
            .name = "utf8_3byte_max",
            .input = (unsigned char *)"\xEF\xBF\xBD",  // U+FFFD replacement char
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xEF\xBF\xBD", .expected_len = 3, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Boundary: Minimum 4-byte (U+10000)
    {
        sanitize_test_t t = {
            .name = "utf8_4byte_min",
            .input = (unsigned char *)"\xF0\x90\x80\x80",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xF0\x90\x80\x80", .expected_len = 4, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Boundary: Maximum valid 4-byte (U+10FFFF)
    {
        sanitize_test_t t = {
            .name = "utf8_4byte_max",
            .input = (unsigned char *)"\xF4\x8F\xBF\xBF",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xF4\x8F\xBF\xBF", .expected_len = 4, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: INVALID UTF-8 SEQUENCES
// ============================================================================

static void test_invalid_utf8_sequences(void) {
    fprintf(stderr, "\n=== Invalid UTF-8 Sequences ===\n");

    // Orphan continuation byte (0x80-0xBF without start byte)
    {
        sanitize_test_t t = {
            .name = "invalid_orphan_continuation",
            .input = (unsigned char *)"A\x80""B",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "A B", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Multiple orphan continuation bytes
    {
        sanitize_test_t t = {
            .name = "invalid_multiple_orphan",
            .input = (unsigned char *)"\x80\x81\x82",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "", .expected_len = 0, .expected_mblen = 0  // All become spaces, trimmed
        };
        run_sanitize_test(&t);
    }

    // Overlong 0xC0 (structurally valid 2-byte, semantically invalid)
    // NOTE: Function does structural validation only - passes through
    {
        sanitize_test_t t = {
            .name = "overlong_C0_structural_valid",
            .input = (unsigned char *)"X\xC0\x80Y",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "X\xC0\x80Y", .expected_len = 4, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Overlong 0xC1 (structurally valid 2-byte)
    {
        sanitize_test_t t = {
            .name = "overlong_C1_structural_valid",
            .input = (unsigned char *)"\xC1\xBF",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC1\xBF", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // 0xF5 with continuation bytes (structurally valid 4-byte, but beyond Unicode)
    {
        sanitize_test_t t = {
            .name = "out_of_range_F5_structural_valid",
            .input = (unsigned char *)"\xF5\x80\x80\x80",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xF5\x80\x80\x80", .expected_len = 4, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // 0xFF alone - not a valid start byte pattern, gets hex encoded
    {
        sanitize_test_t t = {
            .name = "invalid_FF_hex_encoded",
            .input = (unsigned char *)"A\xFF""B",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "AffB", .expected_len = 4, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Truncated 2-byte sequence at end - hex encoded
    {
        sanitize_test_t t = {
            .name = "truncated_2byte_hex",
            .input = (unsigned char *)"abc\xC2",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "abcc2", .expected_len = 5, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // Truncated 3-byte sequence (only 1 continuation) - hex encoded
    {
        sanitize_test_t t = {
            .name = "truncated_3byte_1cont_hex",
            .input = (unsigned char *)"X\xE2\x82",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "Xe282", .expected_len = 5, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Truncated 3-byte sequence (no continuation) - hex encoded
    {
        sanitize_test_t t = {
            .name = "truncated_3byte_0cont_hex",
            .input = (unsigned char *)"Y\xE2",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "Ye2", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Truncated 4-byte sequence - hex encoded
    {
        sanitize_test_t t = {
            .name = "truncated_4byte_hex",
            .input = (unsigned char *)"\xF0\x9F\x98",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "f09f98", .expected_len = 6, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Wrong continuation byte (ASCII instead of 0x80-0xBF) - hex encoded
    {
        sanitize_test_t t = {
            .name = "wrong_continuation_ascii_hex",
            .input = (unsigned char *)"\xC2X",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "c2X", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Wrong continuation byte (another start byte) - first hex encoded, second valid
    {
        sanitize_test_t t = {
            .name = "wrong_continuation_start_hex",
            .input = (unsigned char *)"\xC2\xC2\x80",  // Second C2 is wrong, should be 80-BF
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "c2\xC2\x80", .expected_len = 4, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Overlong NUL (structurally valid, security concern but passed through)
    {
        sanitize_test_t t = {
            .name = "overlong_nul_structural_valid",
            .input = (unsigned char *)"\xC0\x80",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC0\x80", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Overlong space (structurally valid 3-byte)
    {
        sanitize_test_t t = {
            .name = "overlong_space_structural_valid",
            .input = (unsigned char *)"\xE0\x80\xA0",  // Overlong space
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xE0\x80\xA0", .expected_len = 3, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // UTF-16 surrogate (invalid in UTF-8)
    {
        sanitize_test_t t = {
            .name = "invalid_surrogate_high",
            .input = (unsigned char *)"\xED\xA0\x80",  // U+D800 high surrogate
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xED\xA0\x80", .expected_len = 3, .expected_mblen = 1
            // Note: Current implementation doesn't reject surrogates (structural only)
        };
        run_sanitize_test(&t);
    }

    // Out of range (beyond U+10FFFF)
    {
        sanitize_test_t t = {
            .name = "invalid_out_of_range",
            .input = (unsigned char *)"\xF4\x90\x80\x80",  // U+110000 (invalid)
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xF4\x90\x80\x80", .expected_len = 4, .expected_mblen = 1
            // Note: Current implementation doesn't reject out of range (structural only)
        };
        run_sanitize_test(&t);
    }

    // Mixed valid UTF-8 and structurally valid overlong
    {
        sanitize_test_t t = {
            .name = "mixed_valid_and_overlong",
            .input = (unsigned char *)"A\xC2\xB0\xC0\x80\xE2\x82\xAC",  // A° + overlong + €
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            // All are structurally valid, so all pass through
            .expected_output = "A\xC2\xB0\xC0\x80\xE2\x82\xAC", .expected_len = 8, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: BUFFER BOUNDARY CONDITIONS
// ============================================================================

static void test_buffer_boundaries(void) {
    fprintf(stderr, "\n=== Buffer Boundary Conditions ===\n");

    // dst_size = 0
    {
        unsigned char dst[16] = {0xCC, 0xCC, 0xCC, 0xCC};
        size_t len = text_sanitize(dst, (unsigned char *)"hello", 0, identity_char_map, true, "", NULL);
        TEST_ASSERT("buffer_size_0", len == 0, "Expected 0, got %zu", len);
        TEST_ASSERT("buffer_size_0_unchanged", dst[0] == 0xCC, "Buffer was modified");
    }

    // dst_size = 1 (only null terminator fits)
    {
        sanitize_test_t t = {
            .name = "buffer_size_1",
            .input = (unsigned char *)"hello",
            .dst_size = 1, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "", .expected_len = 0, .expected_mblen = 0
        };
        run_sanitize_test(&t);
    }

    // dst_size = 2 (one char + null)
    {
        sanitize_test_t t = {
            .name = "buffer_size_2",
            .input = (unsigned char *)"hello",
            .dst_size = 2, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "h", .expected_len = 1, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Exact fit for ASCII
    {
        sanitize_test_t t = {
            .name = "buffer_exact_ascii",
            .input = (unsigned char *)"abc",
            .dst_size = 4, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "abc", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Off-by-one for ASCII (truncation)
    {
        sanitize_test_t t = {
            .name = "buffer_truncate_ascii",
            .input = (unsigned char *)"abcd",
            .dst_size = 4, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "abc", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Exact fit for 2-byte UTF-8
    {
        sanitize_test_t t = {
            .name = "buffer_exact_2byte",
            .input = (unsigned char *)"\xC2\xB0",  // ° (2 bytes)
            .dst_size = 3, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC2\xB0", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Off-by-one for 2-byte UTF-8 (can't fit, hex encode)
    {
        sanitize_test_t t = {
            .name = "buffer_truncate_2byte",
            .input = (unsigned char *)"\xC2\xB0",
            .dst_size = 2, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "", .expected_len = 0, .expected_mblen = 0  // Can't fit hex either
        };
        run_sanitize_test(&t);
    }

    // Overlong sequence (structurally valid) with exact fit
    {
        sanitize_test_t t = {
            .name = "buffer_overlong_exact_fit",
            .input = (unsigned char *)"\xC0\x80",  // Structurally valid 2-byte
            .dst_size = 3, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC0\x80", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // ASCII + UTF-8 boundary
    {
        sanitize_test_t t = {
            .name = "buffer_ascii_utf8_boundary",
            .input = (unsigned char *)"X\xC2\xB0",  // X° (3 bytes)
            .dst_size = 4, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "X\xC2\xB0", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // UTF-8 doesn't fit at buffer end - nothing written for UTF-8 but mblen still counts
    {
        sanitize_test_t t = {
            .name = "buffer_utf8_no_fit",
            .input = (unsigned char *)"XY\xC2\xB0",  // XY° (4 bytes, but UTF-8 needs 2)
            .dst_size = 4, .char_map = identity_char_map, .utf = true, .empty = "",
            // UTF-8 can't fit, hex can't fit either, nothing written for °
            // But mblen still increments (counts processed, not written)
            .expected_output = "XY", .expected_len = 2, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Overlong UTF-8 near buffer end (structurally valid, can't fit)
    {
        sanitize_test_t t = {
            .name = "buffer_overlong_no_fit",
            .input = (unsigned char *)"A\xC0\x80",  // A + overlong (structurally valid)
            .dst_size = 3, .char_map = identity_char_map, .utf = true, .empty = "",
            // Only A fits (1 byte), overlong needs 2 bytes but only 1 left
            // mblen counts 2 (A + attempted UTF-8)
            .expected_output = "A", .expected_len = 1, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Overlong 2-byte fits exactly, orphan continuation bytes follow
    {
        sanitize_test_t t = {
            .name = "buffer_overlong_with_orphans",
            .input = (unsigned char *)"X\xC0\x80\x80\x80",  // X + overlong + orphan continuations
            .dst_size = 4, .char_map = identity_char_map, .utf = true, .empty = "",
            // X (1) + overlong \xC0\x80 (2) = 3 bytes, fits in dst_size=4
            // Orphan bytes don't fit
            .expected_output = "X\xC0\x80", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Very long input (256 bytes)
    {
        unsigned char long_input[257];
        memset(long_input, 'A', 256);
        long_input[256] = '\0';

        unsigned char expected[101];
        memset(expected, 'A', 100);
        expected[100] = '\0';

        sanitize_test_t t = {
            .name = "buffer_long_input",
            .input = long_input,
            .dst_size = 101, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = (char *)expected, .expected_len = 100, .expected_mblen = 100
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: CHARACTER MAP TRANSFORMATIONS
// ============================================================================

static void test_char_map_transformations(void) {
    fprintf(stderr, "\n=== Character Map Transformations ===\n");

    // Double quote → single quote
    {
        sanitize_test_t t = {
            .name = "charmap_quote",
            .input = (unsigned char *)"say \"hello\"",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "say 'hello'", .expected_len = 11, .expected_mblen = 11
        };
        run_sanitize_test(&t);
    }

    // Backslash → forward slash
    {
        sanitize_test_t t = {
            .name = "charmap_backslash",
            .input = (unsigned char *)"C:\\path\\to\\file",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "C:/path/to/file", .expected_len = 15, .expected_mblen = 15
        };
        run_sanitize_test(&t);
    }

    // Tab → space
    {
        sanitize_test_t t = {
            .name = "charmap_tab",
            .input = (unsigned char *)"col1\tcol2",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "col1 col2", .expected_len = 9, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // Newline → space
    {
        sanitize_test_t t = {
            .name = "charmap_newline",
            .input = (unsigned char *)"line1\nline2",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "line1 line2", .expected_len = 11, .expected_mblen = 11
        };
        run_sanitize_test(&t);
    }

    // Carriage return → space
    {
        sanitize_test_t t = {
            .name = "charmap_cr",
            .input = (unsigned char *)"line1\rline2",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "line1 line2", .expected_len = 11, .expected_mblen = 11
        };
        run_sanitize_test(&t);
    }

    // CRLF → space (deduplicated)
    {
        sanitize_test_t t = {
            .name = "charmap_crlf",
            .input = (unsigned char *)"line1\r\nline2",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "line1 line2", .expected_len = 11, .expected_mblen = 11
        };
        run_sanitize_test(&t);
    }

    // Multiple control characters → single space
    {
        sanitize_test_t t = {
            .name = "charmap_multi_control",
            .input = (unsigned char *)"a\t\n\r\vb",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "a b", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // NUL character (should terminate)
    {
        unsigned char input[] = {'a', 'b', '\0', 'c', 'd', '\0'};
        sanitize_test_t t = {
            .name = "charmap_nul",
            .input = input,
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "ab", .expected_len = 2, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // DEL character (0x7F) → space
    {
        sanitize_test_t t = {
            .name = "charmap_del",
            .input = (unsigned char *)"a\x7F""b",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "a b", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // All printable ASCII preserved (30 characters)
    {
        sanitize_test_t t = {
            .name = "charmap_printable_ascii",
            .input = (unsigned char *)"!#$%&'()*+,-./:;<=>?@[]^_`{|}~",
            .dst_size = 64, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "!#$%&'()*+,-./:;<=>?@[]^_`{|}~", .expected_len = 30, .expected_mblen = 30
        };
        run_sanitize_test(&t);
    }

    // High bytes (0x80-0xBF) are continuation bytes → char_map (space)
    // 0xFF is a start byte but invalid pattern → hex encoded
    {
        sanitize_test_t t = {
            .name = "charmap_high_byte_mixed",
            .input = (unsigned char *)"a\x80\x90\xA0\xB0\xFF""b",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            // \x80-\xB0 are continuation bytes (10xxxxxx) → go through char_map → space
            // \xFF is start byte but invalid pattern → hex encoded as "ff"
            .expected_output = "a ffb", .expected_len = 5, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // Combined transformations
    {
        sanitize_test_t t = {
            .name = "charmap_combined",
            .input = (unsigned char *)"\"path\\to\\file\"\t(100%)",
            .dst_size = 64, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "'path/to/file' (100%)", .expected_len = 21, .expected_mblen = 21
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: SPACE HANDLING
// ============================================================================

static void test_space_handling(void) {
    fprintf(stderr, "\n=== Space Handling ===\n");

    // Leading spaces removed
    {
        sanitize_test_t t = {
            .name = "space_leading",
            .input = (unsigned char *)"   hello",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "hello", .expected_len = 5, .expected_mblen = 5
        };
        run_sanitize_test(&t);
    }

    // Trailing spaces removed
    {
        sanitize_test_t t = {
            .name = "space_trailing",
            .input = (unsigned char *)"hello   ",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "hello", .expected_len = 5, .expected_mblen = 5
        };
        run_sanitize_test(&t);
    }

    // Both leading and trailing
    {
        sanitize_test_t t = {
            .name = "space_both_ends",
            .input = (unsigned char *)"   hello   ",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "hello", .expected_len = 5, .expected_mblen = 5
        };
        run_sanitize_test(&t);
    }

    // Multiple consecutive spaces → single space
    {
        sanitize_test_t t = {
            .name = "space_consecutive",
            .input = (unsigned char *)"hello     world",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "hello world", .expected_len = 11, .expected_mblen = 11
        };
        run_sanitize_test(&t);
    }

    // Only spaces → empty
    {
        sanitize_test_t t = {
            .name = "space_only",
            .input = (unsigned char *)"     ",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "default",
            .expected_output = "default", .expected_len = 7, .expected_mblen = 7
        };
        run_sanitize_test(&t);
    }

    // Control chars becoming spaces and deduplicating
    {
        sanitize_test_t t = {
            .name = "space_from_control",
            .input = (unsigned char *)"a\t\t\t\nb",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "a b", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Space before UTF-8
    {
        sanitize_test_t t = {
            .name = "space_before_utf8",
            .input = (unsigned char *)"temp \xC2\xB0""C",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "temp \xC2\xB0""C", .expected_len = 8, .expected_mblen = 7
        };
        run_sanitize_test(&t);
    }

    // Space after UTF-8
    {
        sanitize_test_t t = {
            .name = "space_after_utf8",
            .input = (unsigned char *)"\xC2\xB0 Celsius",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC2\xB0 Celsius", .expected_len = 10, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // Tab-separated values
    {
        sanitize_test_t t = {
            .name = "space_tsv",
            .input = (unsigned char *)"col1\tcol2\tcol3",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "col1 col2 col3", .expected_len = 14, .expected_mblen = 14
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: EMPTY AND SPECIAL CASES
// ============================================================================

static void test_empty_and_special(void) {
    fprintf(stderr, "\n=== Empty and Special Cases ===\n");

    // NULL input
    {
        unsigned char dst[32];
        size_t len = text_sanitize(dst, NULL, sizeof(dst), identity_char_map, true, "null_val", NULL);
        TEST_ASSERT("null_input", strcmp((char *)dst, "null_val") == 0,
            "Expected 'null_val', got '%s'", dst);
        TEST_ASSERT("null_input_len", len == 8, "Expected 8, got %zu", len);
    }

    // NULL dst
    {
        size_t len = text_sanitize(NULL, (unsigned char *)"hello", 32, identity_char_map, true, "", NULL);
        TEST_ASSERT("null_dst", len == 0, "Expected 0, got %zu", len);
    }

    // Empty string input
    {
        sanitize_test_t t = {
            .name = "empty_input",
            .input = (unsigned char *)"",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "empty_val",
            .expected_output = "empty_val", .expected_len = 9, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // All underscores → empty (special rule)
    {
        sanitize_test_t t = {
            .name = "all_underscores",
            .input = (unsigned char *)"___",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "default",
            .expected_output = "default", .expected_len = 7, .expected_mblen = 7
        };
        run_sanitize_test(&t);
    }

    // Underscore followed by text (not empty)
    {
        sanitize_test_t t = {
            .name = "underscore_prefix",
            .input = (unsigned char *)"___abc",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "___abc", .expected_len = 6, .expected_mblen = 6
        };
        run_sanitize_test(&t);
    }

    // Only control characters → empty
    {
        sanitize_test_t t = {
            .name = "only_control_chars",
            .input = (unsigned char *)"\t\n\r\v",
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "ctrl_empty",
            .expected_output = "ctrl_empty", .expected_len = 10, .expected_mblen = 10
        };
        run_sanitize_test(&t);
    }

    // Invalid UTF-8 that becomes all underscores with utf=false
    {
        sanitize_test_t t = {
            .name = "utf8_to_underscores",
            .input = (unsigned char *)"\xC2\x80\xC2\x80",
            .dst_size = 32, .char_map = identity_char_map, .utf = false, .empty = "utf_empty",
            .expected_output = "utf_empty", .expected_len = 9, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // Empty string with empty default
    {
        sanitize_test_t t = {
            .name = "empty_with_empty_default",
            .input = (unsigned char *)"",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "", .expected_len = 0, .expected_mblen = 0
        };
        run_sanitize_test(&t);
    }

    // Single character
    {
        sanitize_test_t t = {
            .name = "single_char",
            .input = (unsigned char *)"X",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "X", .expected_len = 1, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Single UTF-8 character
    {
        sanitize_test_t t = {
            .name = "single_utf8_char",
            .input = (unsigned char *)"\xC2\xB0",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC2\xB0", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: UTF PARAMETER (true vs false)
// ============================================================================

static void test_utf_parameter(void) {
    fprintf(stderr, "\n=== UTF Parameter (true vs false) ===\n");

    // utf=true: valid UTF-8 preserved
    {
        sanitize_test_t t = {
            .name = "utf_true_valid",
            .input = (unsigned char *)"test\xC2\xB0""C",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "test\xC2\xB0""C", .expected_len = 7, .expected_mblen = 6
        };
        run_sanitize_test(&t);
    }

    // utf=false: valid UTF-8 → underscore
    {
        sanitize_test_t t = {
            .name = "utf_false_valid",
            .input = (unsigned char *)"test\xC2\xB0""C",
            .dst_size = 32, .char_map = identity_char_map, .utf = false, .empty = "",
            .expected_output = "test_C", .expected_len = 6, .expected_mblen = 6
        };
        run_sanitize_test(&t);
    }

    // utf=true: overlong (structurally valid) passes through
    {
        sanitize_test_t t = {
            .name = "utf_true_overlong",
            .input = (unsigned char *)"test\xC0\x80""X",
            .dst_size = 32, .char_map = identity_char_map, .utf = true, .empty = "",
            // \xC0\x80 is structurally valid (2-byte pattern), passes through
            .expected_output = "test\xC0\x80X", .expected_len = 7, .expected_mblen = 6
        };
        run_sanitize_test(&t);
    }

    // utf=false: invalid UTF-8 → underscore
    {
        sanitize_test_t t = {
            .name = "utf_false_invalid",
            .input = (unsigned char *)"test\xC0\x80""X",
            .dst_size = 32, .char_map = identity_char_map, .utf = false, .empty = "",
            .expected_output = "test_X", .expected_len = 6, .expected_mblen = 6
        };
        run_sanitize_test(&t);
    }

    // utf=false: 3-byte UTF-8 → single underscore
    {
        sanitize_test_t t = {
            .name = "utf_false_3byte",
            .input = (unsigned char *)"price\xE2\x82\xAC""100",  // price€100
            .dst_size = 32, .char_map = identity_char_map, .utf = false, .empty = "",
            .expected_output = "price_100", .expected_len = 9, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // utf=false: 4-byte UTF-8 → single underscore
    {
        sanitize_test_t t = {
            .name = "utf_false_4byte",
            .input = (unsigned char *)"hi\xF0\x9F\x98\x80""!",  // hi😀!
            .dst_size = 32, .char_map = identity_char_map, .utf = false, .empty = "",
            .expected_output = "hi_!", .expected_len = 4, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // utf=false: multiple UTF-8 → multiple underscores (but collapse doesn't happen)
    {
        sanitize_test_t t = {
            .name = "utf_false_multiple",
            .input = (unsigned char *)"\xC2\xB0\xC2\xB5",  // °µ
            .dst_size = 32, .char_map = identity_char_map, .utf = false, .empty = "x",
            .expected_output = "x", .expected_len = 1, .expected_mblen = 1
            // Two underscores collapse to empty due to all-underscore rule
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: MULTIBYTE LENGTH OUTPUT
// ============================================================================

static void test_multibyte_length(void) {
    fprintf(stderr, "\n=== Multibyte Length Output ===\n");

    // ASCII only: byte length == char count
    {
        unsigned char dst[32];
        size_t mblen = 0;
        size_t len = text_sanitize(dst, (unsigned char *)"hello", sizeof(dst),
                                   identity_char_map, true, "", &mblen);
        TEST_ASSERT("mblen_ascii", len == 5 && mblen == 5,
            "len=%zu mblen=%zu, expected both 5", len, mblen);
    }

    // Single 2-byte UTF-8: byte length > char count
    {
        unsigned char dst[32];
        size_t mblen = 0;
        size_t len = text_sanitize(dst, (unsigned char *)"\xC2\xB0", sizeof(dst),
                                   identity_char_map, true, "", &mblen);
        TEST_ASSERT("mblen_2byte", len == 2 && mblen == 1,
            "len=%zu mblen=%zu, expected len=2 mblen=1", len, mblen);
    }

    // Single 3-byte UTF-8
    {
        unsigned char dst[32];
        size_t mblen = 0;
        size_t len = text_sanitize(dst, (unsigned char *)"\xE2\x82\xAC", sizeof(dst),
                                   identity_char_map, true, "", &mblen);
        TEST_ASSERT("mblen_3byte", len == 3 && mblen == 1,
            "len=%zu mblen=%zu, expected len=3 mblen=1", len, mblen);
    }

    // Single 4-byte UTF-8
    {
        unsigned char dst[32];
        size_t mblen = 0;
        size_t len = text_sanitize(dst, (unsigned char *)"\xF0\x9F\x98\x80", sizeof(dst),
                                   identity_char_map, true, "", &mblen);
        TEST_ASSERT("mblen_4byte", len == 4 && mblen == 1,
            "len=%zu mblen=%zu, expected len=4 mblen=1", len, mblen);
    }

    // Mixed: ASCII + UTF-8
    {
        unsigned char dst[32];
        size_t mblen = 0;
        // "A°€😀" = 1 + 2 + 3 + 4 = 10 bytes, 4 chars
        size_t len = text_sanitize(dst, (unsigned char *)"A\xC2\xB0\xE2\x82\xAC\xF0\x9F\x98\x80",
                                   sizeof(dst), identity_char_map, true, "", &mblen);
        TEST_ASSERT("mblen_mixed", len == 10 && mblen == 4,
            "len=%zu mblen=%zu, expected len=10 mblen=4", len, mblen);
    }

    // NULL mblen pointer (shouldn't crash)
    {
        unsigned char dst[32];
        size_t len = text_sanitize(dst, (unsigned char *)"test", sizeof(dst),
                                   identity_char_map, true, "", NULL);
        TEST_ASSERT("mblen_null_ptr", len == 4, "len=%zu, expected 4", len);
    }
}

// ============================================================================
// TEST: RRD STRING ALLOWED CHARS SPECIFIC
// ============================================================================

static void test_rrd_string_allowed_chars(void) {
    fprintf(stderr, "\n=== RRD String Allowed Chars ===\n");

    // Use actual rrd_string_allowed_chars from the codebase
    extern unsigned char rrd_string_allowed_chars[256];

    // Basic ASCII passes through
    {
        sanitize_test_t t = {
            .name = "rrd_ascii",
            .input = (unsigned char *)"cpu.user",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "cpu.user", .expected_len = 8, .expected_mblen = 8
        };
        run_sanitize_test(&t);
    }

    // Double quote transformed
    {
        sanitize_test_t t = {
            .name = "rrd_double_quote",
            .input = (unsigned char *)"\"value\"",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "'value'", .expected_len = 7, .expected_mblen = 7
        };
        run_sanitize_test(&t);
    }

    // Backslash transformed
    {
        sanitize_test_t t = {
            .name = "rrd_backslash",
            .input = (unsigned char *)"path\\file",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "path/file", .expected_len = 9, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // UTF-8 units preserved
    {
        sanitize_test_t t = {
            .name = "rrd_utf8_units",
            .input = (unsigned char *)"requests/s\xC2\xB2",  // requests/s²
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "requests/s\xC2\xB2", .expected_len = 12, .expected_mblen = 11
        };
        run_sanitize_test(&t);
    }

    // Temperature with degree symbol
    {
        sanitize_test_t t = {
            .name = "rrd_temperature",
            .input = (unsigned char *)"Temperature (\xC2\xB0""C)",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "Temperature (\xC2\xB0""C)", .expected_len = 17, .expected_mblen = 16
        };
        run_sanitize_test(&t);
    }

    // Microseconds
    {
        sanitize_test_t t = {
            .name = "rrd_microseconds",
            .input = (unsigned char *)"\xC2\xB5s",  // µs
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "\xC2\xB5s", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Complex metric title
    {
        sanitize_test_t t = {
            .name = "rrd_complex_title",
            .input = (unsigned char *)"CPU \"usage\" on C:\\Windows (100%)",
            .dst_size = 64, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "CPU 'usage' on C:/Windows (100%)", .expected_len = 32, .expected_mblen = 32
        };
        run_sanitize_test(&t);
    }

    // Prometheus-style metric
    {
        sanitize_test_t t = {
            .name = "rrd_prometheus_style",
            .input = (unsigned char *)"http_requests_total{method=\"GET\"}",
            .dst_size = 64, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "http_requests_total{method='GET'}", .expected_len = 33, .expected_mblen = 33
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: SECURITY-FOCUSED CASES
// ============================================================================

static void test_security_cases(void) {
    fprintf(stderr, "\n=== Security-Focused Cases ===\n");

    // Path traversal attempt (should be handled safely)
    {
        sanitize_test_t t = {
            .name = "security_path_traversal",
            .input = (unsigned char *)"../../../etc/passwd",
            .dst_size = 64, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "../../../etc/passwd", .expected_len = 19, .expected_mblen = 19
        };
        run_sanitize_test(&t);
    }

    // Path traversal with backslash (Windows style, converted to /)
    {
        sanitize_test_t t = {
            .name = "security_path_traversal_win",
            .input = (unsigned char *)"..\\..\\..\\etc\\passwd",
            .dst_size = 64, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "../../../etc/passwd", .expected_len = 19, .expected_mblen = 19
        };
        run_sanitize_test(&t);
    }

    // Overlong NUL - structurally valid, passes through
    // NOTE: This is a security concern in some systems but this function
    // only does structural validation for sanitization purposes
    {
        sanitize_test_t t = {
            .name = "security_overlong_nul_passthrough",
            .input = (unsigned char *)"test\xC0\x80test",  // Overlong NUL
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "test\xC0\x80test", .expected_len = 10, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // Overlong slash - structurally valid, passes through
    {
        sanitize_test_t t = {
            .name = "security_overlong_slash_passthrough",
            .input = (unsigned char *)"\xC0\xAF",  // Overlong /
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xC0\xAF", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Overlong A (3 bytes) - structurally valid, passes through
    {
        sanitize_test_t t = {
            .name = "security_overlong_A_passthrough",
            .input = (unsigned char *)"\xE0\x81\x81",  // Overlong A
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xE0\x81\x81", .expected_len = 3, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // XSS attempt with angle brackets
    {
        sanitize_test_t t = {
            .name = "security_xss_tags",
            .input = (unsigned char *)"<script>alert(1)</script>",
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "<script>alert(1)</script>", .expected_len = 25, .expected_mblen = 25
        };
        run_sanitize_test(&t);
    }

    // SQL injection attempt (quotes transformed)
    {
        sanitize_test_t t = {
            .name = "security_sql_injection",
            .input = (unsigned char *)"test' OR '1'='1",
            .dst_size = 64, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "test' OR '1'='1", .expected_len = 15, .expected_mblen = 15
        };
        run_sanitize_test(&t);
    }

    // Null byte injection (string terminates at NUL)
    {
        unsigned char input[] = {'t', 'e', 's', 't', '\0', 'e', 'v', 'i', 'l', '\0'};
        sanitize_test_t t = {
            .name = "security_null_byte",
            .input = input,
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "test", .expected_len = 4, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // BOM (Byte Order Mark) at start - should be preserved as valid UTF-8
    {
        sanitize_test_t t = {
            .name = "security_bom",
            .input = (unsigned char *)"\xEF\xBB\xBFtext",  // UTF-8 BOM + text
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xEF\xBB\xBFtext", .expected_len = 7, .expected_mblen = 5
        };
        run_sanitize_test(&t);
    }

    // UTF-7 encoding attempt (should just pass through as ASCII)
    {
        sanitize_test_t t = {
            .name = "security_utf7",
            .input = (unsigned char *)"+ADw-script+AD4-",
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "+ADw-script+AD4-", .expected_len = 16, .expected_mblen = 16
        };
        run_sanitize_test(&t);
    }

    // Private Use Area character (valid UTF-8, possibly suspicious)
    {
        sanitize_test_t t = {
            .name = "security_private_use",
            .input = (unsigned char *)"\xEE\x80\x80",  // U+E000 (Private Use)
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "\xEE\x80\x80", .expected_len = 3, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: REGRESSION TESTS FOR FIXED BUGS
// ============================================================================

static void test_regression_fixed_bugs(void) {
    fprintf(stderr, "\n=== Regression Tests for Fixed Bugs ===\n");

    // REGRESSION: The original buffer overflow bug was in hex encoding path.
    // Test with TRULY invalid UTF-8 (truncated sequence) that triggers hex encoding
    {
        sanitize_test_t t = {
            .name = "regression_hex_buffer_overflow",
            .input = (unsigned char *)"\xC2",  // Truncated 2-byte sequence
            .dst_size = 3, .char_map = identity_char_map, .utf = true, .empty = "",
            // Hex needs 2 chars ("c2") + NUL = 3, exact fit
            .expected_output = "c2", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Test truncated sequence that would overflow if not properly bounded
    {
        sanitize_test_t t = {
            .name = "regression_hex_no_overflow",
            .input = (unsigned char *)"\xC2",  // Truncated
            .dst_size = 2, .char_map = identity_char_map, .utf = true, .empty = "",
            // Hex needs 2 chars but only 1 space (plus NUL) - nothing written
            // Note: mblen is 0 when loop doesn't process due to buffer constraints
            .expected_output = "", .expected_len = 0, .expected_mblen = 0
        };
        run_sanitize_test(&t);
    }

    // Overlong \xC0\x80 is structurally VALID - test it passes through
    {
        sanitize_test_t t = {
            .name = "regression_overlong_passthrough",
            .input = (unsigned char *)"X\xC0\x80Y",
            .dst_size = 16, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "X\xC0\x80Y", .expected_len = 4, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // REGRESSION: Memory read OOB (Issue: Loop didn't check for NUL before accessing src[i])
    {
        sanitize_test_t t = {
            .name = "regression_memory_oob_truncated",
            .input = (unsigned char *)"test\xE2\x82",  // Truncated 3-byte sequence
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "teste282", .expected_len = 8, .expected_mblen = 5
        };
        run_sanitize_test(&t);
    }

    // REGRESSION: Memory read OOB with 4-byte truncated at various points
    {
        sanitize_test_t t1 = {
            .name = "regression_oob_4byte_1cont",
            .input = (unsigned char *)"\xF0\x9F",  // Only 2 of 4 bytes
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "f09f", .expected_len = 4, .expected_mblen = 1
        };
        run_sanitize_test(&t1);

        sanitize_test_t t2 = {
            .name = "regression_oob_4byte_2cont",
            .input = (unsigned char *)"\xF0\x9F\x98",  // Only 3 of 4 bytes
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "f09f98", .expected_len = 6, .expected_mblen = 1
        };
        run_sanitize_test(&t2);
    }

    // Edge case: \xF5 is treated as 4-byte start, but alone it's truncated → hex
    // NOTE: \xF5 without continuation bytes triggers hex encoding
    {
        sanitize_test_t t = {
            .name = "regression_edge_F5_truncated",
            .input = (unsigned char *)"X\xF5",  // X + truncated F5 (no continuation)
            .dst_size = 5, .char_map = identity_char_map, .utf = true, .empty = "",
            // X (1) + "f5" (2) + NUL = 4, fits in 5
            .expected_output = "Xf5", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Edge case: Exactly 2 spaces for hex (dst_size=4, one char used)
    {
        sanitize_test_t t = {
            .name = "regression_edge_exact_hex_fit",
            .input = (unsigned char *)"A\xC0",  // A + truncated (missing continuation)
            .dst_size = 4, .char_map = identity_char_map, .utf = true, .empty = "",
            // A (1) + "c0" (2) + NUL = 4 (exact fit)
            .expected_output = "Ac0", .expected_len = 3, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Verify the original bug scenario from the PR: ms² being preserved
    {
        sanitize_test_t t = {
            .name = "regression_ms_squared",
            .input = (unsigned char *)"ms\xC2\xB2",  // ms²
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "ms\xC2\xB2", .expected_len = 4, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: ALL CONTROL CHARACTERS (0x00-0x1F, 0x7F)
// ============================================================================

static void test_all_control_characters(void) {
    fprintf(stderr, "\n=== All Control Characters ===\n");

    // Test each control character 0x01-0x1F individually
    for (unsigned int ctrl = 1; ctrl < 32; ctrl++) {
        unsigned char input[4] = {'A', (unsigned char)ctrl, 'B', '\0'};
        char expected[8];

        // With test_rrd_char_map, all control chars become space
        snprintf(expected, sizeof(expected), "A B");

        char name[32];
        snprintf(name, sizeof(name), "ctrl_0x%02X", ctrl);

        size_t guard = 16;
        unsigned char *buffer = callocz(1, 32 + guard * 2);
        unsigned char *dst = buffer + guard;
        memset(buffer, 0xAA, guard);
        memset(dst + 32, 0xBB, guard);
        memset(dst, 0xCC, 32);

        size_t mblen = 0;
        text_sanitize(dst, input, 32, test_rrd_char_map, true, "", &mblen);

        bool overflow = false;
        for (size_t i = 0; i < guard; i++) {
            if (buffer[i] != 0xAA || dst[32 + i] != 0xBB) {
                overflow = true;
                break;
            }
        }

        TEST_ASSERT(name, !overflow && strcmp((char *)dst, expected) == 0,
            "ctrl=0x%02X: overflow=%d, expected '%s', got '%s'", ctrl, overflow, expected, dst);

        freez(buffer);
    }

    // Test DEL (0x7F)
    {
        unsigned char input[] = {'A', 0x7F, 'B', '\0'};
        sanitize_test_t t = {
            .name = "ctrl_DEL_0x7F",
            .input = input,
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "A B", .expected_len = 3, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Multiple different control characters in sequence
    {
        unsigned char input[] = {'X', 0x01, 0x02, 0x03, 0x04, 0x05, 'Y', '\0'};
        sanitize_test_t t = {
            .name = "ctrl_multiple_sequence",
            .input = input,
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "X Y", .expected_len = 3, .expected_mblen = 3
            // All control chars become spaces, then deduplicated
        };
        run_sanitize_test(&t);
    }

    // Bell character (0x07) - common in terminal output
    {
        unsigned char input[] = {'b', 'e', 'l', 'l', 0x07, 't', 'e', 's', 't', '\0'};
        sanitize_test_t t = {
            .name = "ctrl_bell",
            .input = input,
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "bell test", .expected_len = 9, .expected_mblen = 9
        };
        run_sanitize_test(&t);
    }

    // Escape sequence (0x1B) - ANSI escape
    {
        unsigned char input[] = {0x1B, '[', '3', '1', 'm', 'r', 'e', 'd', 0x1B, '[', '0', 'm', '\0'};
        sanitize_test_t t = {
            .name = "ctrl_ansi_escape",
            .input = input,
            .dst_size = 32, .char_map = test_rrd_char_map, .utf = true, .empty = "",
            .expected_output = "[31mred [0m", .expected_len = 11, .expected_mblen = 11
            // 0x1B becomes space, which is leading/duplicated so gets handled
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: REAL-WORLD METRIC STRINGS
// ============================================================================

static void test_real_world_metrics(void) {
    fprintf(stderr, "\n=== Real-World Metric Strings ===\n");

    // Use actual rrd_string_allowed_chars
    extern unsigned char rrd_string_allowed_chars[256];

    // CPU metric title
    {
        sanitize_test_t t = {
            .name = "metric_cpu_title",
            .input = (unsigned char *)"CPU utilization (user, system, iowait, irq, softirq)",
            .dst_size = 64, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "CPU utilization (user, system, iowait, irq, softirq)", .expected_len = 52, .expected_mblen = 52
        };
        run_sanitize_test(&t);
    }

    // Memory with units
    {
        sanitize_test_t t = {
            .name = "metric_memory_unit",
            .input = (unsigned char *)"Memory (MiB)",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "Memory (MiB)", .expected_len = 12, .expected_mblen = 12
        };
        run_sanitize_test(&t);
    }

    // Network bandwidth with special chars
    {
        sanitize_test_t t = {
            .name = "metric_network_bandwidth",
            .input = (unsigned char *)"eth0: kilobits/s",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "eth0: kilobits/s", .expected_len = 16, .expected_mblen = 16
        };
        run_sanitize_test(&t);
    }

    // Disk I/O with latency units (microseconds)
    {
        sanitize_test_t t = {
            .name = "metric_disk_latency",
            .input = (unsigned char *)"Disk latency (\xC2\xB5s)",  // µs
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "Disk latency (\xC2\xB5s)", .expected_len = 18, .expected_mblen = 17
        };
        run_sanitize_test(&t);
    }

    // Temperature sensor
    {
        sanitize_test_t t = {
            .name = "metric_temperature",
            .input = (unsigned char *)"core_temp_0: \xC2\xB0""Celsius",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            // "core_temp_0: " (13) + "°" (2 bytes) + "Celsius" (7) = 22 bytes, 21 chars
            .expected_output = "core_temp_0: \xC2\xB0""Celsius", .expected_len = 22, .expected_mblen = 21
        };
        run_sanitize_test(&t);
    }

    // Docker container ID (common in Netdata)
    {
        sanitize_test_t t = {
            .name = "metric_docker_id",
            .input = (unsigned char *)"container_a1b2c3d4e5f6",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "container_a1b2c3d4e5f6", .expected_len = 22, .expected_mblen = 22
        };
        run_sanitize_test(&t);
    }

    // Kubernetes pod name
    {
        sanitize_test_t t = {
            .name = "metric_k8s_pod",
            .input = (unsigned char *)"nginx-deployment-5d8b7f9-xyz12",
            .dst_size = 64, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "nginx-deployment-5d8b7f9-xyz12", .expected_len = 30, .expected_mblen = 30
        };
        run_sanitize_test(&t);
    }

    // Windows path (backslash conversion)
    {
        sanitize_test_t t = {
            .name = "metric_windows_path",
            .input = (unsigned char *)"C:\\Program Files\\Application\\metric.exe",
            .dst_size = 64, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "C:/Program Files/Application/metric.exe", .expected_len = 39, .expected_mblen = 39
        };
        run_sanitize_test(&t);
    }

    // Prometheus metric with labels (quotes converted)
    {
        sanitize_test_t t = {
            .name = "metric_prometheus_labels",
            .input = (unsigned char *)"http_requests{method=\"POST\",status=\"200\"}",
            .dst_size = 64, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "http_requests{method='POST',status='200'}", .expected_len = 41, .expected_mblen = 41
        };
        run_sanitize_test(&t);
    }

    // Acceleration units (m/s²)
    {
        sanitize_test_t t = {
            .name = "metric_acceleration",
            .input = (unsigned char *)"Acceleration (m/s\xC2\xB2)",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "Acceleration (m/s\xC2\xB2)", .expected_len = 20, .expected_mblen = 19
        };
        run_sanitize_test(&t);
    }

    // Percentage with degree
    {
        sanitize_test_t t = {
            .name = "metric_angle_degree",
            .input = (unsigned char *)"Rotation angle: 90\xC2\xB0",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            // "Rotation angle: 90" (18) + "°" (2 bytes) = 20 bytes, 19 chars
            .expected_output = "Rotation angle: 90\xC2\xB0", .expected_len = 20, .expected_mblen = 19
        };
        run_sanitize_test(&t);
    }

    // IPv6 address in metric context
    {
        sanitize_test_t t = {
            .name = "metric_ipv6",
            .input = (unsigned char *)"host:2001:db8::1",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "host:2001:db8::1", .expected_len = 16, .expected_mblen = 16
        };
        run_sanitize_test(&t);
    }

    // Process name with parentheses and numbers
    {
        sanitize_test_t t = {
            .name = "metric_process_name",
            .input = (unsigned char *)"python3.11 (worker-1)",
            .dst_size = 32, .char_map = rrd_string_allowed_chars, .utf = true, .empty = "",
            .expected_output = "python3.11 (worker-1)", .expected_len = 21, .expected_mblen = 21
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: HEX ENCODING EDGE CASES
// ============================================================================

static void test_hex_encoding_edge_cases(void) {
    fprintf(stderr, "\n=== Hex Encoding Edge Cases ===\n");

    // Truncated 2-byte sequence - gets hex encoded
    // mblen counts the whole invalid sequence as 1 character
    {
        sanitize_test_t t = {
            .name = "hex_truncated_2byte",
            .input = (unsigned char *)"\xC2",  // Truncated (needs continuation)
            .dst_size = 3, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "c2", .expected_len = 2, .expected_mblen = 1
        };
        run_sanitize_test(&t);
    }

    // Not enough space for hex encoding
    {
        sanitize_test_t t = {
            .name = "hex_no_space",
            .input = (unsigned char *)"\xC2",  // Truncated
            .dst_size = 2, .char_map = identity_char_map, .utf = true, .empty = "",
            // Can't fit "c2" (needs 2 chars + NUL = 3) - nothing written, mblen=0
            .expected_output = "", .expected_len = 0, .expected_mblen = 0
        };
        run_sanitize_test(&t);
    }

    // Multiple truncated sequences - each counts as 1 mblen
    {
        sanitize_test_t t = {
            .name = "hex_multiple_truncated",
            .input = (unsigned char *)"\xC2\xC3",  // Two truncated 2-byte starts
            .dst_size = 5, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "c2c3", .expected_len = 4, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // ASCII + truncated hex
    {
        sanitize_test_t t = {
            .name = "hex_ascii_plus_truncated",
            .input = (unsigned char *)"AB\xC2",  // AB + truncated
            .dst_size = 5, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "ABc2", .expected_len = 4, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // 0xFE and 0xFF don't match valid UTF-8 start patterns - hex encoded
    {
        sanitize_test_t t = {
            .name = "hex_FE_FF",
            .input = (unsigned char *)"\xFE\xFF",
            .dst_size = 8, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = "feff", .expected_len = 4, .expected_mblen = 2
        };
        run_sanitize_test(&t);
    }

    // Structurally valid overlong + orphan continuation
    {
        sanitize_test_t t = {
            .name = "hex_valid_plus_orphan",
            .input = (unsigned char *)"X\xC0\x80\x80",  // X + valid 2-byte + orphan
            .dst_size = 8, .char_map = identity_char_map, .utf = true, .empty = "",
            // \xC0\x80 is structurally valid (passes through), \x80 is orphan (char_map)
            .expected_output = "X\xC0\x80\x80", .expected_len = 4, .expected_mblen = 3
        };
        run_sanitize_test(&t);
    }

    // Orphan continuation bytes go through char_map
    {
        sanitize_test_t t = {
            .name = "hex_orphan_continuations",
            .input = (unsigned char *)"\x80\x81\x82\x83",
            .dst_size = 16, .char_map = identity_char_map, .utf = true, .empty = "",
            // Orphan continuation bytes (10xxxxxx pattern) go through char_map
            .expected_output = "\x80\x81\x82\x83", .expected_len = 4, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }
}

// ============================================================================
// TEST: STRESS AND EDGE CASES
// ============================================================================

static void test_stress_and_edge_cases(void) {
    fprintf(stderr, "\n=== Stress and Edge Cases ===\n");

    // Very long UTF-8 string
    {
        // Create string with 100 2-byte UTF-8 characters (200 bytes)
        unsigned char input[201];
        for (int i = 0; i < 100; i++) {
            input[i*2] = 0xC2;
            input[i*2+1] = 0xB0;  // ° repeated 100 times
        }
        input[200] = '\0';

        unsigned char expected[201];
        memcpy(expected, input, 201);

        sanitize_test_t t = {
            .name = "stress_long_utf8",
            .input = input,
            .dst_size = 256, .char_map = identity_char_map, .utf = true, .empty = "",
            .expected_output = (char *)expected, .expected_len = 200, .expected_mblen = 100
        };
        run_sanitize_test(&t);
    }

    // Alternating valid UTF-8 and structurally valid overlong (both pass through)
    {
        sanitize_test_t t = {
            .name = "stress_alternating",
            .input = (unsigned char *)"\xC2\xB0\xC0\x80\xC2\xB0\xC0\x80",
            .dst_size = 64, .char_map = identity_char_map, .utf = true, .empty = "",
            // Both \xC2\xB0 and \xC0\x80 are structurally valid 2-byte sequences
            .expected_output = "\xC2\xB0\xC0\x80\xC2\xB0\xC0\x80", .expected_len = 8, .expected_mblen = 4
        };
        run_sanitize_test(&t);
    }

    // All 256 byte values (non-UTF-8 mode)
    {
        unsigned char input[256];
        for (int i = 1; i < 256; i++)  // Skip NUL
            input[i-1] = (unsigned char)i;
        input[255] = '\0';

        // With identity map, most pass through; control chars and high bytes
        // will be handled. This just tests no crash.
        unsigned char dst[512];
        size_t len = text_sanitize(dst, input, sizeof(dst), identity_char_map, false, "", NULL);
        TEST_ASSERT("stress_all_bytes", len > 0, "Expected non-zero length, got %zu", len);
    }

    // Rapid buffer size changes (fuzz-like)
    {
        const unsigned char *input = (unsigned char *)"test\xC2\xB0\xE2\x82\xAC\xF0\x9F\x98\x80";
        bool all_ok = true;

        for (size_t sz = 1; sz <= 20; sz++) {
            unsigned char *buffer = callocz(1, sz + 32);
            unsigned char *dst = buffer + 16;
            memset(buffer, 0xAA, 16);
            memset(dst + sz, 0xBB, 16);

            text_sanitize(dst, input, sz, identity_char_map, true, "", NULL);

            // Check for overflow
            for (size_t i = 0; i < 16; i++) {
                if (buffer[i] != 0xAA || dst[sz + i] != 0xBB) {
                    all_ok = false;
                    fprintf(stderr, "  Overflow at buffer size %zu\n", sz);
                    break;
                }
            }
            freez(buffer);
        }
        TEST_ASSERT("stress_buffer_sizes", all_ok, "Buffer overflow detected in size sweep");
    }

    // Repeated sanitization (idempotent for valid input)
    {
        unsigned char input[] = "test\xC2\xB0""C";
        unsigned char dst1[32], dst2[32];

        text_sanitize(dst1, input, sizeof(dst1), identity_char_map, true, "", NULL);
        text_sanitize(dst2, dst1, sizeof(dst2), identity_char_map, true, "", NULL);

        TEST_ASSERT("stress_idempotent", strcmp((char *)dst1, (char *)dst2) == 0,
            "Not idempotent: '%s' vs '%s'", dst1, dst2);
    }
}

// ============================================================================
// MAIN TEST RUNNER
// ============================================================================

int utf8_sanitizer_unittest(void) {
    fprintf(stderr, "\n");
    fprintf(stderr, "================================================================\n");
    fprintf(stderr, "UTF-8 Sanitizer Exhaustive Unit Tests\n");
    fprintf(stderr, "================================================================\n");

    init_char_maps();

    test_valid_utf8_sequences();
    test_invalid_utf8_sequences();
    test_buffer_boundaries();
    test_char_map_transformations();
    test_space_handling();
    test_empty_and_special();
    test_utf_parameter();
    test_multibyte_length();
    test_rrd_string_allowed_chars();
    test_security_cases();
    test_regression_fixed_bugs();
    test_all_control_characters();
    test_real_world_metrics();
    test_hex_encoding_edge_cases();
    test_stress_and_edge_cases();

    fprintf(stderr, "\n================================================================\n");
    fprintf(stderr, "Tests run: %d, Passed: %d, Failed: %d\n", tests_run, tests_passed, tests_failed);

    if (tests_failed == 0) {
        fprintf(stderr, "ALL TESTS PASSED\n");
    } else {
        fprintf(stderr, "SOME TESTS FAILED\n");
    }
    fprintf(stderr, "================================================================\n\n");

    return tests_failed;
}
