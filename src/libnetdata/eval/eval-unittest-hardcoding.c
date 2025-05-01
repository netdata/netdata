// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "eval-internal.h"

// Test case structure for hardcode variable tests
typedef struct {
    const char *name;               // Name/description of the test case
    const char *expression;         // Initial expression to parse
    const char *variable;           // Variable name to hardcode (NULL for testing NULL variable)
    NETDATA_DOUBLE hardcode_value;  // Value to hardcode (NAN for testing NaN)
    const char *expected_source;    // Expected expression source after hardcoding
    NETDATA_DOUBLE expected_result; // Expected result after evaluation
    EVAL_ERROR expected_error;      // Expected error code (EVAL_ERROR_OK if no error)
} HardcodeTestCase;

int eval_hardcode_unittest(void) {
    printf("\n=== Running Tests for expression_hardcode_variable() ===\n");
    
    // Define all test cases as data
    HardcodeTestCase test_cases[] = {
        // Basic variable replacement
        {
            "Basic variable",
            "$test_var + 10",
            "test_var",
            42.0,
            "42 + 10",
            52.0,
            EVAL_ERROR_OK
        },
        // Variable with braces
        {
            "Variable with braces",
            "${test_var} * 2",
            "test_var",
            42.0,
            "42 * 2",
            84.0,
            EVAL_ERROR_OK
        },
        // Multiple occurrences of same variable
        {
            "Multiple occurrences",
            "$test_var + ${test_var} + $test_var + ${test_var} + $test_var + ${test_var}",
            "test_var",
            42.0,
            "42 + 42 + 42 + 42 + 42 + 42",
            252.0,
            EVAL_ERROR_OK
        },
        // Complex nested expression
        {
            "Complex expression",
            "($test_var > 30) ? (${test_var} * 2) : ($test_var / 2)",
            "test_var",
            42.0,
            "(42 > 30) ? (42 * 2) : (42 / 2)",
            84.0,
            EVAL_ERROR_OK
        },
        // Variable not in expression (should remain unchanged)
        {
            "Variable not in expression",
            "33 + 33",
            "test_var",
            42.0,
            "33 + 33",
            66.0,
            EVAL_ERROR_OK
        },
        // Hardcoding negative value
        {
            "Negative value",
            "$test_var * 10",
            "test_var",
            -5.0,
            "-5 * 10",
            -50.0,
            EVAL_ERROR_OK
        },
        // Hardcoding decimal value
        {
            "Decimal value",
            "$test_var / 10",
            "test_var",
            123.456,
            "123.456 / 10",
            12.3456,
            EVAL_ERROR_OK
        },
        // Function parameter
        {
            "Function parameter",
            "abs($test_var)",
            "test_var",
            -42.0,
            "abs(-42)",
            42.0,
            EVAL_ERROR_OK
        },
        // NaN value
        {
            "NaN value (ignored)",
            "$test_var + 10",
            "test_var",
            NAN,
            "nan + 10", // source should remain unchanged
            0.0, // Unresolved variable error
            EVAL_ERROR_VALUE_IS_NAN
        },
        // INFINITY value
        {
            "NaN value (ignored)",
            "$test_var + 10",
            "test_var",
            INFINITY,
            "inf + 10", // source should remain unchanged
            0.0, // Unresolved variable error
            EVAL_ERROR_VALUE_IS_INFINITE
        },
        // NULL expression (no crash test)
        {
            "NULL expression",
            NULL, // This isn't actually used - we'll handle it specially
            "test_var",
            42.0,
            NULL, // Expected source doesn't matter
            0.0,  // Result doesn't matter
            EVAL_ERROR_OK
        },
        // NULL variable (no crash test)
        {
            "NULL variable",
            "$test_var + 10",
            NULL, // NULL variable name
            42.0,
            "$test_var + 10", // source should remain unchanged
            0.0, // Unresolved variable error
            EVAL_ERROR_UNKNOWN_VARIABLE
        }
    };
    
    int passed = 0;
    int failed = 0;
    
    // Single loop to process all test cases
    for (size_t i = 0; i < sizeof(test_cases) / sizeof(test_cases[0]); i++) {
        HardcodeTestCase *tc = &test_cases[i];
        
        printf("Test %zu: %s\n", i + 1, tc->name);
        
        // Handle special case of NULL expression test (just don't crash)
        if (!tc->expression) {
            printf("  Testing NULL expression (shouldn't crash)...\n");
            STRING *var = tc->variable ? string_strdupz(tc->variable) : NULL;
            
            // This call shouldn't crash
            expression_hardcode_variable(NULL, var, tc->hardcode_value);
            
            printf("  PASSED: No crash with NULL expression\n");
            passed++;
            if (var) string_freez(var);
            continue;
        }
        
        // Parse the expression
        const char *failed_at = NULL;
        EVAL_ERROR error = EVAL_ERROR_OK;
        EVAL_EXPRESSION *exp = expression_parse(tc->expression, &failed_at, &error);
        
        if (!exp) {
            printf("  FAILED: Could not parse expression, error: %d (%s)\n", 
                   (int)error, expression_strerror(error));
            failed++;
            continue;
        }
        
        // Save the original source
        const char *original_source = expression_source(exp);
        printf("  Original source: %s\n", original_source);
        
        // Hardcode the variable
        STRING *var = tc->variable ? string_strdupz(tc->variable) : NULL;
        expression_hardcode_variable(exp, var, tc->hardcode_value);
        if (var) string_freez(var);
        
        // Get the modified source
        const char *modified_source = expression_source(exp);
        printf("  Modified source: %s\n", modified_source);
        
        // Check if source was modified as expected
        bool source_correct = true;
        if (tc->expected_source && 
            strcmp(modified_source, tc->expected_source) != 0) {
            printf("  FAILED: Source doesn't match expected.\n");
            printf("  Expected: %s\n", tc->expected_source);
            printf("  Actual:   %s\n", modified_source);
            source_correct = false;
        }
        
        // Evaluate the expression
        expression_evaluate(exp);
        
        // Check error code
        bool error_correct = (exp->error == tc->expected_error);
        if (!error_correct) {
            printf("  FAILED: Error code doesn't match expected.\n");
            printf("  Expected error: %u (%s)\n",
                  tc->expected_error, expression_strerror(tc->expected_error));
            printf("  Actual error:   %u (%s)\n",
                  exp->error, expression_strerror(exp->error));
        }
        
        // Check result if we expected no error
        bool result_correct = true;
        if (tc->expected_error == EVAL_ERROR_OK) {
            NETDATA_DOUBLE result = expression_result(exp);
            printf("  Result: %f\n", result);
            
            if (fabs(result - tc->expected_result) > 0.000001) {
                printf("  FAILED: Result doesn't match expected.\n");
                printf("  Expected: %f\n", tc->expected_result);
                printf("  Actual:   %f\n", result);
                result_correct = false;
            }
        }
        
        // Determine if test passed overall
        if (source_correct && error_correct && result_correct) {
            printf("  PASSED\n");
            passed++;
        } else {
            failed++;
        }
        
        // Clean up
        expression_free(exp);
    }
    
    // Report results
    printf("\nHardcode variable test results: %d passed, %d failed\n", passed, failed);
    return failed;
}