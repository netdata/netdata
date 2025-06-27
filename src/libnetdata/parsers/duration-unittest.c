// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "duration.h"

typedef struct {
    const char *input;
    const char *default_unit;  // Default unit for parsing (NULL means "s")
    const char *output_unit;
    int64_t expected_value;
    bool should_succeed;
    const char *expected_reformat;  // Expected reformatted string (NULL to skip check)
    const char *description;
} duration_test_case_t;

static const duration_test_case_t test_cases[] = {
    // Basic abbreviated forms
    { "5m", NULL, "s", 300, true, "5m", "5 minutes to seconds" },
    { "2h", NULL, "s", 7200, true, "2h", "2 hours to seconds" },
    { "7d", NULL, "s", 604800, true, "7d", "7 days to seconds" },
    { "1w", NULL, "d", 7, true, "7d", "1 week to days" },
    { "30s", NULL, "s", 30, true, "30s", "30 seconds to seconds" },
    
    // Full unit names (lowercase)
    { "7 days", NULL, "s", 604800, true, "7d", "7 days (full) to seconds" },
    { "2 hours", NULL, "s", 7200, true, "2h", "2 hours (full) to seconds" },
    { "30 seconds", NULL, "s", 30, true, "30s", "30 seconds (full) to seconds" },
    { "5 minutes", NULL, "s", 300, true, "5m", "5 minutes (full) to seconds" },
    { "1 week", NULL, "d", 7, true, "7d", "1 week (full) to days" },
    { "2 months", NULL, "d", 60, true, "2mo", "2 months (full) to days" },
    { "1 year", NULL, "d", 365, true, "1y", "1 year (full) to days" },
    
    // Case variations
    { "7 DAYS", NULL, "s", 604800, true, "7d", "7 DAYS (uppercase) to seconds" },
    { "2 Hours", NULL, "s", 7200, true, "2h", "2 Hours (mixed case) to seconds" },
    { "30 SECONDS", NULL, "s", 30, true, "30s", "30 SECONDS (uppercase) to seconds" },
    { "5 Minutes", NULL, "s", 300, true, "5m", "5 Minutes (mixed case) to seconds" },
    
    // Without spaces
    { "7days", NULL, "s", 604800, true, "7d", "7days (no space) to seconds" },
    { "2hours", NULL, "s", 7200, true, "2h", "2hours (no space) to seconds" },
    { "30seconds", NULL, "s", 30, true, "30s", "30seconds (no space) to seconds" },
    { "5minutes", NULL, "s", 300, true, "5m", "5minutes (no space) to seconds" },
    
    // Singular forms
    { "1 day", NULL, "s", 86400, true, "1d", "1 day (singular) to seconds" },
    { "1 hour", NULL, "s", 3600, true, "1h", "1 hour (singular) to seconds" },
    { "1 second", NULL, "s", 1, true, "1s", "1 second (singular) to seconds" },
    { "1 minute", NULL, "s", 60, true, "1m", "1 minute (singular) to seconds" },
    { "1 week", NULL, "d", 7, true, "7d", "1 week (singular) to days" },
    { "1 month", NULL, "d", 30, true, "1mo", "1 month (singular) to days" },
    { "1 year", NULL, "d", 365, true, "1y", "1 year (singular) to days" },
    
    // Complex expressions with full names
    { "2 hours 30 minutes", NULL, "s", 9000, true, "2h30m", "2 hours 30 minutes to seconds" },
    { "1 day 12 hours", NULL, "s", 129600, true, "1d12h", "1 day 12 hours to seconds" },
    { "1 week 2 days", NULL, "d", 9, true, "9d", "1 week 2 days to days" },
    { "1 year 2 months 3 days", NULL, "d", 428, true, "1y2mo3d", "1 year 2 months 3 days to days" },
    
    // Mixed abbreviated and full names
    { "2h 30 minutes", NULL, "s", 9000, true, "2h30m", "2h 30 minutes (mixed) to seconds" },
    { "1d 12 hours", NULL, "s", 129600, true, "1d12h", "1d 12 hours (mixed) to seconds" },
    { "1 week 2d", NULL, "d", 9, true, "9d", "1 week 2d (mixed) to days" },
    
    // Other time units
    { "100 milliseconds", NULL, "ms", 100, true, "100ms", "100 milliseconds to ms" },
    { "50 microseconds", NULL, "us", 50, true, "50us", "50 microseconds to us" },
    { "25 nanoseconds", NULL, "ns", 25, true, "25ns", "25 nanoseconds to ns" },
    { "100 MILLISECONDS", NULL, "ms", 100, true, "100ms", "100 MILLISECONDS (uppercase) to ms" },
    { "50 Microseconds", NULL, "us", 50, true, "50us", "50 Microseconds (mixed case) to us" },
    
    // Fractional values with full names
    { "1.5 days", NULL, "h", 36, true, "1d12h", "1.5 days to hours" },
    { "2.5 hours", NULL, "m", 150, true, "2h30m", "2.5 hours to minutes" },
    { "0.5 minutes", NULL, "s", 30, true, "30s", "0.5 minutes to seconds" },
    
    // Alternative abbreviations
    { "30 sec", NULL, "s", 30, true, "30s", "30 sec to seconds" },
    { "30 secs", NULL, "s", 30, true, "30s", "30 secs to seconds" },
    { "2 hr", NULL, "m", 120, true, "2h", "2 hr to minutes" },
    { "2 hrs", NULL, "m", 120, true, "2h", "2 hrs to minutes" },
    
    // Special keywords (case-insensitive)
    { "never", NULL, "s", 0, true, "off", "never keyword" },
    { "NEVER", NULL, "s", 0, true, "off", "NEVER keyword (uppercase)" },
    { "Never", NULL, "s", 0, true, "off", "Never keyword (mixed case)" },
    { "off", NULL, "s", 0, true, "off", "off keyword" },
    { "OFF", NULL, "s", 0, true, "off", "OFF keyword (uppercase)" },
    { "Off", NULL, "s", 0, true, "off", "Off keyword (mixed case)" },
    
    // Negative durations
    { "-5 minutes", NULL, "s", -300, true, "-5m", "negative 5 minutes to seconds" },
    { "-2 hours", NULL, "s", -7200, true, "-2h", "negative 2 hours to seconds" },
    { "-1 day", NULL, "s", -86400, true, "-1d", "negative 1 day to seconds" },
    
    // "ago" suffix support (should negate the value)
    { "7 days ago", NULL, "s", -604800, true, "-7d", "7 days ago to negative seconds" },
    { "7d ago", NULL, "s", -604800, true, "-7d", "7d ago to negative seconds" },
    { "2 hours ago", NULL, "s", -7200, true, "-2h", "2 hours ago to negative seconds" },
    { "2h ago", NULL, "s", -7200, true, "-2h", "2h ago to negative seconds" },
    { "30 minutes ago", NULL, "s", -1800, true, "-30m", "30 minutes ago to negative seconds" },
    { "30m ago", NULL, "s", -1800, true, "-30m", "30m ago to negative seconds" },
    { "1 year ago", NULL, "d", -365, true, "-1y", "1 year ago to negative days" },
    { "1y ago", NULL, "d", -365, true, "-1y", "1y ago to negative days" },
    
    // Complex expressions with "ago"
    { "2 hours 30 minutes ago", NULL, "s", -9000, true, "-2h30m", "2 hours 30 minutes ago to negative seconds" },
    { "2h30m ago", NULL, "s", -9000, true, "-2h30m", "2h30m ago to negative seconds" },
    { "1 day 12 hours ago", NULL, "s", -129600, true, "-1d12h", "1 day 12 hours ago to negative seconds" },
    { "1d12h ago", NULL, "s", -129600, true, "-1d12h", "1d12h ago to negative seconds" },
    
    // Case variations with "ago"
    { "7 days AGO", NULL, "s", -604800, true, "-7d", "7 days AGO (uppercase) to negative seconds" },
    { "7 days Ago", NULL, "s", -604800, true, "-7d", "7 days Ago (mixed case) to negative seconds" },
    { "7daysago", NULL, "s", -604800, true, "-7d", "7daysago (no spaces) to negative seconds" },
    
    // Edge case: negative duration with "ago" - redundant but intent is clear, treat as negative
    { "-7 days ago", NULL, "s", -604800, true, "-7d", "negative duration with 'ago' stays negative" },
    { "-2h ago", NULL, "s", -7200, true, "-2h", "negative duration with 'ago' stays negative" },
    
    // Invalid cases that should fail
    { "invalid", NULL, "s", 0, false, NULL, "invalid unit should fail" },
    { "5 invalidunit", NULL, "s", 0, false, NULL, "invalid full unit name should fail" },
    { "abc days", NULL, "s", 0, false, NULL, "non-numeric value should fail" },
    { "", NULL, "s", 0, false, NULL, "empty string should fail" },
    { "7 days ago extra", NULL, "s", 0, false, NULL, "trailing text after 'ago' should fail" },
    { "7 days agooo", NULL, "s", 0, false, NULL, "misspelled 'ago' should fail" },
    { "ago", NULL, "s", 0, false, NULL, "'ago' without duration should fail" },
    { "7 ago days", NULL, "s", 0, false, NULL, "'ago' in wrong position should fail" },
    { "7 days ago 1 hour", NULL, "s", 0, false, NULL, "text after 'ago' should fail" },
    { "7d ago 1h", NULL, "s", 0, false, NULL, "duration after 'ago' should fail" },
    
    // Complex expressions with arithmetic
    { "-7d+1h", NULL, "s", -608400, true, "-7d1h", "negative days plus positive hours" },
    { "1d-12h", NULL, "s", 43200, true, "12h", "positive days minus hours" },
    { "2h-3h", NULL, "s", -3600, true, "-1h", "results in negative duration" },
    
    // Test various formatting edge cases
    { "3661s", NULL, "s", 3661, true, "1h1m1s", "many seconds to h/m/s" },
    { "90000s", NULL, "s", 90000, true, "1d1h", "many seconds to d/h" },
    { "31536000s", NULL, "s", 31536000, true, "1y", "seconds in a year" },
    { "366d", NULL, "d", 366, true, "1y1d", "more than a year in days" },
    { "100000000ns", NULL, "ns", 100000000, true, "100ms", "nanoseconds to milliseconds" },
    { "3600000ms", NULL, "ms", 3600000, true, "1h", "milliseconds to hours" },
    { "0.001s", NULL, "ms", 1, true, "1ms", "fractional seconds to ms" },
    { "1440m", NULL, "h", 24, true, "1d", "minutes to hours shows days" },
    { "10080m", NULL, "d", 7, true, "7d", "minutes to days" },
    
    // Zero value
    { "0s", NULL, "s", 0, true, "off", "zero seconds" },
    { "0d", NULL, "d", 0, true, "off", "zero days" },
    
    // Plain numbers (no unit) - should use default unit
    { "60", "s", "s", 60, true, "1m", "plain 60 with default seconds" },
    { "3600", "s", "s", 3600, true, "1h", "plain 3600 with default seconds" },
    { "86400", "s", "s", 86400, true, "1d", "plain 86400 with default seconds" },
    { "7", "d", "d", 7, true, "7d", "plain 7 with default days" },
    { "24", "h", "h", 24, true, "1d", "plain 24 with default hours" },
    { "60", "m", "m", 60, true, "1h", "plain 60 with default minutes" },
    { "1000", "ms", "ms", 1000, true, "1s", "plain 1000 with default milliseconds" },
    { "1000000", "us", "us", 1000000, true, "1s", "plain 1000000 with default microseconds" },
    { "1000000000", "ns", "ns", 1000000000, true, "1s", "plain 1000000000 with default nanoseconds" },
    
    // Negative plain numbers
    { "-60", "s", "s", -60, true, "-1m", "negative 60 with default seconds" },
    { "-3600", "s", "s", -3600, true, "-1h", "negative 3600 with default seconds" },
    { "-86400", "s", "s", -86400, true, "-1d", "negative 86400 with default seconds" },
    { "-7", "d", "d", -7, true, "-7d", "negative 7 with default days" },
    { "-24", "h", "h", -24, true, "-1d", "negative 24 with default hours" },
    { "-60", "m", "m", -60, true, "-1h", "negative 60 with default minutes" },
    
    // Fractional plain numbers
    { "1.5", "d", "d", 2, true, "2d", "fractional 1.5 with default days rounds to 2" },
    { "2.5", "h", "h", 3, true, "3h", "fractional 2.5 with default hours rounds to 3" },
    { "0.5", "m", "m", 1, true, "1m", "fractional 0.5 with default minutes rounds to 1" },
    { "1.5", "s", "s", 2, true, "2s", "fractional 1.5 with default seconds rounds to 2" },
    { "-1.5", "h", "h", -2, true, "-2h", "negative fractional with default hours rounds to -2" },
    
    // Edge cases with plain numbers
    { "0", "s", "s", 0, true, "off", "plain zero" },
    { "-0", "s", "s", 0, true, "off", "negative zero" },
    { "+60", "s", "s", 60, true, "1m", "explicit positive sign" },
    { " 60 ", "s", "s", 60, true, "1m", "spaces around number" },
    
    // Unix epoch timestamps (large numbers)
    { "1705318200", NULL, "s", 1705318200, true, NULL, "Unix timestamp: Mon Jan 15 2024 10:30:00 UTC" },
    { "1609459200", NULL, "s", 1609459200, true, NULL, "Unix timestamp: Fri Jan 01 2021 00:00:00 UTC" },
    { "946684800", NULL, "s", 946684800, true, NULL, "Unix timestamp: Sat Jan 01 2000 00:00:00 UTC" },
    { "0", NULL, "s", 0, true, "off", "Unix timestamp: epoch (Jan 01 1970)" },
    { "-86400", NULL, "s", -86400, true, "-1d", "Unix timestamp: negative (before epoch)" },
    
    // Very large numbers (future timestamps)
    { "2147483647", NULL, "s", 2147483647, true, NULL, "Unix timestamp: max 32-bit (Jan 19 2038)" },
    { "4102444800", NULL, "s", 4102444800, true, NULL, "Unix timestamp: Jan 01 2100" },
    
    // Timestamp-like numbers with units (should parse as duration)
    { "1705318200s", NULL, "s", 1705318200, true, NULL, "timestamp with 's' unit" },
    { "1705318200 seconds", NULL, "s", 1705318200, true, NULL, "timestamp with 'seconds' unit" },
    
    // End marker
    { NULL, NULL, NULL, 0, false, NULL, NULL }
};

static int run_duration_test(const duration_test_case_t *test) {
    int64_t result = 0;
    const char *default_unit = test->default_unit ? test->default_unit : "s";
    bool success = duration_parse(test->input, &result, default_unit, test->output_unit);
    
    if (success != test->should_succeed) {
        fprintf(stderr, "FAILED: %s\n", test->description);
        fprintf(stderr, "  Input: '%s'\n", test->input);
        fprintf(stderr, "  Expected to %s but %s\n", 
                test->should_succeed ? "succeed" : "fail",
                success ? "succeeded" : "failed");
        return 1;
    }
    
    if (success) {
        // Check the parsed value
        if (result != test->expected_value) {
            fprintf(stderr, "FAILED: %s\n", test->description);
            fprintf(stderr, "  Input: '%s'\n", test->input);
            fprintf(stderr, "  Expected: %" PRId64 " %s\n", test->expected_value, test->output_unit);
            fprintf(stderr, "  Got: %" PRId64 " %s\n", result, test->output_unit);
            
            // Also show the reformatted version
            char buffer[256];
            ssize_t len = duration_snprintf(buffer, sizeof(buffer), result, test->output_unit, false);
            if (len > 0) {
                fprintf(stderr, "  Reformatted: '%s'\n", buffer);
            }
            return 1;
        }
        
        // Check the reformatted output if expected
        if (test->expected_reformat != NULL) {
            char buffer[256];
            ssize_t len = duration_snprintf(buffer, sizeof(buffer), result, test->output_unit, false);
            
            if (len < 0) {
                fprintf(stderr, "FAILED: %s (reformat failed)\n", test->description);
                fprintf(stderr, "  Input: '%s'\n", test->input);
                fprintf(stderr, "  Value: %" PRId64 " %s\n", result, test->output_unit);
                return 1;
            }
            
            if (strcmp(buffer, test->expected_reformat) != 0) {
                fprintf(stderr, "FAILED: %s (reformat mismatch)\n", test->description);
                fprintf(stderr, "  Input: '%s'\n", test->input);
                fprintf(stderr, "  Value: %" PRId64 " %s\n", result, test->output_unit);
                fprintf(stderr, "  Expected reformat: '%s'\n", test->expected_reformat);
                fprintf(stderr, "  Got reformat: '%s'\n", buffer);
                return 1;
            }
        }
    }
    
    return 0;
}

static int test_duration_generation(void) {
    int failed = 0;
    char buffer[256];
    
    // Test that generation still uses abbreviated forms
    struct {
        int64_t value;
        const char *unit;
        const char *expected;
    } gen_tests[] = {
        { 300, "s", "5m" },
        { 7200, "s", "2h" },
        { 86400, "s", "1d" },
        { 604800, "s", "7d" },
        { 2592000, "s", "1mo" },
        { 31536000, "s", "1y" },
        { 9000, "s", "2h30m" },
        { 129600, "s", "1d12h" },
        { 0, "s", "off" },
        { -300, "s", "-5m" },
        { 0, NULL, NULL }
    };
    
    for (int i = 0; gen_tests[i].unit != NULL; i++) {
        ssize_t len = duration_snprintf(buffer, sizeof(buffer), gen_tests[i].value, gen_tests[i].unit, false);
        
        if (len < 0) {
            fprintf(stderr, "FAILED: Generation test %d - snprintf failed\n", i);
            failed++;
            continue;
        }
        
        if (strcmp(buffer, gen_tests[i].expected) != 0) {
            fprintf(stderr, "FAILED: Generation test %d\n", i);
            fprintf(stderr, "  Value: %" PRId64 " %s\n", gen_tests[i].value, gen_tests[i].unit);
            fprintf(stderr, "  Expected: '%s'\n", gen_tests[i].expected);
            fprintf(stderr, "  Got: '%s'\n", buffer);
            failed++;
        }
    }
    
    return failed;
}

static int test_parse_and_format_roundtrip(void) {
    int failed = 0;
    char buffer[256];
    
    // Test cases for parse -> format -> parse roundtrip
    struct {
        const char *input;
        const char *expected_format;  // What we expect after parsing and reformatting
        const char *description;
    } roundtrip_tests[] = {
        // Simple cases
        { "7d", "7d", "simple days" },
        { "7 days", "7d", "full days name" },
        { "2h30m", "2h30m", "hours and minutes" },
        { "2 hours 30 minutes", "2h30m", "full names" },
        
        // Arithmetic expressions
        { "1d-12h", "12h", "day minus hours" },
        { "2h-3h", "-1h", "negative result" },
        { "1d12h", "1d12h", "day plus hours" },
        
        // Negative durations
        { "-7d", "-7d", "negative days" },
        { "-2h30m", "-2h30m", "negative complex" },
        
        // With "ago" (should be formatted without "ago")
        { "7 days ago", "-7d", "days ago" },
        { "2h30m ago", "-2h30m", "complex ago" },
        { "-7 days ago", "-7d", "redundant negative ago" },
        
        // Edge cases
        { "0s", "off", "zero seconds" },
        { "never", "off", "never keyword" },
        { "off", "off", "off keyword" },
        
        { NULL, NULL, NULL }
    };
    
    for (int i = 0; roundtrip_tests[i].input != NULL; i++) {
        // Parse the input
        int64_t value;
        bool success = duration_parse(roundtrip_tests[i].input, &value, "s", "s");
        
        if (!success) {
            fprintf(stderr, "FAILED: Roundtrip parse failed for '%s' (%s)\n", 
                    roundtrip_tests[i].input, roundtrip_tests[i].description);
            failed++;
            continue;
        }
        
        // Format it back
        ssize_t len = duration_snprintf(buffer, sizeof(buffer), value, "s", false);
        if (len < 0) {
            fprintf(stderr, "FAILED: Roundtrip format failed for '%s' (%s)\n", 
                    roundtrip_tests[i].input, roundtrip_tests[i].description);
            failed++;
            continue;
        }
        
        // Check if it matches expected format
        if (strcmp(buffer, roundtrip_tests[i].expected_format) != 0) {
            fprintf(stderr, "FAILED: Roundtrip test '%s'\n", roundtrip_tests[i].description);
            fprintf(stderr, "  Input: '%s'\n", roundtrip_tests[i].input);
            fprintf(stderr, "  Expected format: '%s'\n", roundtrip_tests[i].expected_format);
            fprintf(stderr, "  Got format: '%s'\n", buffer);
            fprintf(stderr, "  Parsed value: %" PRId64 " seconds\n", value);
            failed++;
        }
        
        // Parse the formatted string again to verify it gives the same value
        int64_t value2;
        success = duration_parse(buffer, &value2, "s", "s");
        if (!success || value != value2) {
            fprintf(stderr, "FAILED: Roundtrip re-parse failed for '%s'\n", roundtrip_tests[i].description);
            fprintf(stderr, "  Original: '%s' -> %" PRId64 "\n", roundtrip_tests[i].input, value);
            fprintf(stderr, "  Formatted: '%s' -> %" PRId64 "\n", buffer, value2);
            failed++;
        }
    }
    
    return failed;
}

int duration_unittest(void) {
    int passed = 0;
    int failed = 0;
    
    printf("Starting duration parser unit tests with full unit name support\n");
    printf("===============================================================\n\n");
    
    // Run parsing tests
    printf("Running parsing tests...\n");
    for (const duration_test_case_t *test = test_cases; test->input != NULL; test++) {
        if (run_duration_test(test) == 0) {
            passed++;
        } else {
            failed++;
        }
    }
    
    printf("\nRunning generation tests...\n");
    int gen_failed = test_duration_generation();
    if (gen_failed == 0) {
        printf("All generation tests passed\n");
    } else {
        failed += gen_failed;
    }
    
    printf("\nRunning parse/format roundtrip tests...\n");
    int roundtrip_failed = test_parse_and_format_roundtrip();
    if (roundtrip_failed == 0) {
        printf("All roundtrip tests passed\n");
    } else {
        failed += roundtrip_failed;
    }
    
    printf("\n===============================================================\n");
    printf("Duration parser tests completed: %d passed, %d failed\n", passed, failed);
    
    return failed;
}