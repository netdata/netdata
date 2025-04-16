// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "eval-internal.h"
#include "ast/ast.h"

// Mock variable lookup function for testing
static bool test_variable_lookup(STRING *variable, void *data __maybe_unused, NETDATA_DOUBLE *result) {
    const char *var_name = string2str(variable);
    
    if (strcmp(var_name, "var1") == 0) {
        *result = 42.0;
        return true;
    }
    else if (strcmp(var_name, "var2") == 0) {
        *result = 24.0;
        return true;
    }
    else if (strcmp(var_name, "zero") == 0) {
        *result = 0.0;
        return true;
    }
    else if (strcmp(var_name, "negative") == 0) {
        *result = -10.0;
        return true;
    }
    else if (strcmp(var_name, "nan_var") == 0) {
        *result = NAN;
        return true;
    }
    else if (strcmp(var_name, "inf_var") == 0) {
        *result = INFINITY;
        return true;
    }
    else if (strcmp(var_name, "this variable") == 0) {
        *result = 100.0;
        return true;
    }
    else if (strcmp(var_name, "this") == 0) {
        *result = 50.0;
        return true;
    }
    else if (strcmp(var_name, "1var") == 0) {
        *result = 75.0;
        return true;
    }
    else if (strcmp(var_name, "_var") == 0) {
        *result = 76.0;
        return true;
    }
    else if (strcmp(var_name, "1.var") == 0) {
        *result = 77.0;
        return true;
    }
    else if (strcmp(var_name, "var.1") == 0) {
        *result = 78.0;
        return true;
    }
    else if (strcmp(var_name, "var-1") == 0) {
        *result = 79.0;
        return true;
    }

    return false; // Variable not found
}

typedef struct {
    const char *expression;
    NETDATA_DOUBLE expected_result;
    int expected_error;
    bool should_parse;
} TestCase;

typedef struct {
    const char *name;
    TestCase *test_cases;
    int test_count;
} TestGroup;

#define ARRAY_SIZE(x) (sizeof(x) / sizeof((x)[0]))

static void run_test_group(TestGroup *group) {
    printf("\n=== Running Test Group: %s ===\n", group->name);
    
    int passed = 0;
    int failed = 0;
    
    for (int i = 0; i < group->test_count; i++) {
        TestCase *tc = &group->test_cases[i];
        const char *failed_at = NULL;
        int error = 0;
        bool test_failed = false;
        char error_message[1024] = "";
        
        printf("Test %d: %s\n", i + 1, tc->expression);
        
        // Try to parse the expression
        EVAL_EXPRESSION *exp = expression_parse(tc->expression, &failed_at, &error);
        
        // Check if parsing succeeded as expected
        if (tc->should_parse && !exp) {
            snprintf(error_message, sizeof(error_message), 
                     "Expected parsing to succeed, but it failed with error %d (%s)",
                     error, expression_strerror(error));
            test_failed = true;
        }
        else if (!tc->should_parse && exp) {
            snprintf(error_message, sizeof(error_message),
                     "Expected parsing to fail, but it succeeded");
            test_failed = true;
        }
        
        // If the expression parsed successfully, evaluate it
        if (exp) {
            // Set up the variable lookup callback
            expression_set_variable_lookup_callback(exp, test_variable_lookup, NULL);
            
            // Evaluate the expression
            int eval_result = expression_evaluate(exp);
            
            // Check if there was an error during evaluation
            if (tc->expected_error != EVAL_ERROR_OK && exp->error == EVAL_ERROR_OK) {
                snprintf(error_message, sizeof(error_message),
                         "Expected evaluation error %d, but got no error",
                         tc->expected_error);
                test_failed = true;
            }
            else if (tc->expected_error == EVAL_ERROR_OK && exp->error != EVAL_ERROR_OK) {
                snprintf(error_message, sizeof(error_message),
                         "Expected no evaluation error, but got error %d (%s)",
                         exp->error, expression_strerror(exp->error));
                test_failed = true;
            }
            else if (tc->expected_error != EVAL_ERROR_OK && exp->error != tc->expected_error) {
                snprintf(error_message, sizeof(error_message),
                         "Expected evaluation error %d, but got error %d (%s)",
                         tc->expected_error, exp->error, expression_strerror(exp->error));
                test_failed = true;
            }
            
            // Check the evaluation result
            if (tc->expected_error == EVAL_ERROR_OK) {
                if (isnan(tc->expected_result) && !isnan(exp->result)) {
                    snprintf(error_message, sizeof(error_message),
                             "Expected NaN result, but got %f", exp->result);
                    test_failed = true;
                }
                else if (isinf(tc->expected_result) && !isinf(exp->result)) {
                    snprintf(error_message, sizeof(error_message),
                             "Expected Inf result, but got %f", exp->result);
                    test_failed = true;
                }
                else if (!isnan(tc->expected_result) && !isinf(tc->expected_result) &&
                         !isnan(exp->result) && !isinf(exp->result) &&
                         fabs(tc->expected_result - exp->result) > 0.000001) {
                    snprintf(error_message, sizeof(error_message),
                             "Expected result %f, but got %f", 
                             tc->expected_result, exp->result);
                    test_failed = true;
                }
            }
            
            // Print additional information for debugging
            printf("  Parsed as: %s\n", expression_parsed_as(exp));
            
            if (eval_result) {
                printf("  Evaluated to: %f\n", expression_result(exp));
            } else {
                printf("  Evaluation failed: %s\n", expression_error_msg(exp));
            }
            
            // Special handling for API function tests
            if (strcmp(group->name, "API Function Tests") == 0) {
                if (strstr(tc->expression, "hardcoded_var") != NULL) {
                    // Test the expression_hardcode_variable function
                    STRING *var = string_strdupz("hardcoded_var");
                    expression_hardcode_variable(exp, var, 123.456);
                    string_freez(var);
                    
                    // Re-evaluate the expression
                    expression_evaluate(exp);
                    
                    // Check if hardcoded variable works correctly
                    if (exp->error != EVAL_ERROR_OK || fabs(exp->result - 123.456) > 0.000001) {
                        snprintf(error_message, sizeof(error_message),
                                "expression_hardcode_variable failed: expected 123.456, got %f (error: %d)",
                                exp->result, exp->error);
                        test_failed = true;
                    } else {
                        printf("  expression_hardcode_variable test passed!\n");
                    }
                } 
                else if (strcmp(tc->expression, "1 + 2") == 0) {
                    // Test expression_source
                    const char *source = expression_source(exp);
                    if (strcmp(source, "1 + 2") != 0) {
                        snprintf(error_message, sizeof(error_message),
                                "expression_source failed: expected '1 + 2', got '%s'",
                                source);
                        test_failed = true;
                    } else {
                        printf("  expression_source test passed!\n");
                    }
                    
                    // Test expression_parsed_as
                    const char *parsed = expression_parsed_as(exp);
                    if (parsed == NULL || strlen(parsed) == 0) {
                        snprintf(error_message, sizeof(error_message),
                                "expression_parsed_as failed: got empty or NULL result");
                        test_failed = true;
                    } else {
                        printf("  expression_parsed_as test passed! Result: %s\n", parsed);
                    }
                    
                    // Test expression_result
                    NETDATA_DOUBLE result = expression_result(exp);
                    if (fabs(result - 3.0) > 0.000001) {
                        snprintf(error_message, sizeof(error_message),
                                "expression_result failed: expected 3.0, got %f",
                                result);
                        test_failed = true;
                    } else {
                        printf("  expression_result test passed!\n");
                    }
                }
                else if (strcmp(tc->expression, "bad/syntax") == 0) {
                    // This case is for testing expression_error_msg, but it is already tested
                    // in the main evaluation loop when errors occur.
                    printf("  expression_error_msg is tested during evaluation failures\n");
                }
            }
            
            // Clean up
            expression_free(exp);
        }
        else if (!tc->should_parse) {
            printf("  Parsing failed as expected at: %s\n", 
                   failed_at ? ((*failed_at) ? failed_at : "<END OF EXPRESSION>") : "<NONE>");
        }
        
        // Report test result
        if (test_failed) {
#ifdef USE_RE2C_LEMON_PARSER
            printf("  [RE2C_LEMON] FAILED: %s\n", error_message);
#else
            printf("  [RECURSIVE] FAILED: %s\n", error_message);
#endif
            failed++;
        } else {
            printf("  PASSED\n");
            passed++;
        }
    }
    
    printf("\nGroup Results: %d tests, %d passed, %d failed\n", 
           passed + failed, passed, failed);
}

static TestCase arithmetic_tests[] = {
    {"1 + 2", 3.0, EVAL_ERROR_OK, true},
    {"5 - 3", 2.0, EVAL_ERROR_OK, true},
    {"4 * 5", 20.0, EVAL_ERROR_OK, true},
    {"10 / 2", 5.0, EVAL_ERROR_OK, true},
    {"10 / 0", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true}, // Netdata reports error for division by zero
    {"-10", -10.0, EVAL_ERROR_OK, true},
    {"+5", 5.0, EVAL_ERROR_OK, true},
    {"5 + -3", 2.0, EVAL_ERROR_OK, true},
    {"5 * -3", -15.0, EVAL_ERROR_OK, true},
    {"1 + 2 * 3", 7.0, EVAL_ERROR_OK, true},
    {"(1 + 2) * 3", 9.0, EVAL_ERROR_OK, true},
    {"10.5 + 2.5", 13.0, EVAL_ERROR_OK, true},
    {"10.5 * 2", 21.0, EVAL_ERROR_OK, true},
    {"5.5 / 2", 2.75, EVAL_ERROR_OK, true},
    {"1.5e2 + 2", 152.0, EVAL_ERROR_OK, true},
    {"1+2*3+4", 11.0, EVAL_ERROR_OK, true},
};

// Test cases for comparison operations
static TestCase comparison_tests[] = {
    {"1 == 1", 1.0, EVAL_ERROR_OK, true},
    {"1 == 2", 0.0, EVAL_ERROR_OK, true},
    {"1 != 2", 1.0, EVAL_ERROR_OK, true},
    {"1 != 1", 0.0, EVAL_ERROR_OK, true},
    {"5 > 3", 1.0, EVAL_ERROR_OK, true},
    {"3 > 5", 0.0, EVAL_ERROR_OK, true},
    {"3 < 5", 1.0, EVAL_ERROR_OK, true},
    {"5 < 3", 0.0, EVAL_ERROR_OK, true},
    {"5 >= 5", 1.0, EVAL_ERROR_OK, true},
    {"5 >= 6", 0.0, EVAL_ERROR_OK, true},
    {"5 <= 5", 1.0, EVAL_ERROR_OK, true},
    {"5 <= 4", 0.0, EVAL_ERROR_OK, true},
    {"3 > 2 > 1", 0.0, EVAL_ERROR_OK, true}, // This is (3 > 2) > 1, which is 1 > 1, which is false
};

// Test cases for logical operations
static TestCase logical_tests[] = {
    {"1 && 1", 1.0, EVAL_ERROR_OK, true},
    {"1 && 0", 0.0, EVAL_ERROR_OK, true},
    {"0 && 1", 0.0, EVAL_ERROR_OK, true},
    {"0 && 0", 0.0, EVAL_ERROR_OK, true},
    {"1 || 1", 1.0, EVAL_ERROR_OK, true},
    {"1 || 0", 1.0, EVAL_ERROR_OK, true},
    {"0 || 1", 1.0, EVAL_ERROR_OK, true},
    {"0 || 0", 0.0, EVAL_ERROR_OK, true},
    {"!1", 0.0, EVAL_ERROR_OK, true},
    {"!0", 1.0, EVAL_ERROR_OK, true},
    {"!(1 && 0)", 1.0, EVAL_ERROR_OK, true},
    {"1 && !0", 1.0, EVAL_ERROR_OK, true},
    {"0 || !(1 && 0)", 1.0, EVAL_ERROR_OK, true},
    
    // Tests for word operators (AND, OR)
    {"1 AND 1", 1.0, EVAL_ERROR_OK, true},
    {"1 AND 0", 0.0, EVAL_ERROR_OK, true},
    {"0 AND 1", 0.0, EVAL_ERROR_OK, true},
    {"0 AND 0", 0.0, EVAL_ERROR_OK, true},
    {"1 OR 1", 1.0, EVAL_ERROR_OK, true},
    {"1 OR 0", 1.0, EVAL_ERROR_OK, true},
    {"0 OR 1", 1.0, EVAL_ERROR_OK, true},
    {"0 OR 0", 0.0, EVAL_ERROR_OK, true},
    {"NOT 1", 0.0, EVAL_ERROR_OK, true},
    {"NOT 0", 1.0, EVAL_ERROR_OK, true},
    {"NOT(1 AND 0)", 1.0, EVAL_ERROR_OK, true},
    {"1 AND NOT 0", 1.0, EVAL_ERROR_OK, true},
    {"0 OR NOT(1 AND 0)", 1.0, EVAL_ERROR_OK, true},
    {"(1 AND 1) OR (0 AND 1)", 1.0, EVAL_ERROR_OK, true},
    
    // Mixed symbol and word operators
    {"1 AND (0 || 1)", 1.0, EVAL_ERROR_OK, true},
    {"(1 && 0) OR 1", 1.0, EVAL_ERROR_OK, true},
    {"NOT (1 && 0) OR (NOT 0 AND 1)", 1.0, EVAL_ERROR_OK, true},
    
    // Case-insensitive logical operators
    {"1 and 1", 1.0, EVAL_ERROR_OK, true},
    {"0 or 1", 1.0, EVAL_ERROR_OK, true},
    {"not 0", 1.0, EVAL_ERROR_OK, true},
    {"1 And 0", 0.0, EVAL_ERROR_OK, true},
    {"0 Or 1", 1.0, EVAL_ERROR_OK, true},
    {"Not 1", 0.0, EVAL_ERROR_OK, true},
};

// Test cases for variable usage
static TestCase variable_tests[] = {
    {"$var1", 42.0, EVAL_ERROR_OK, true},
    {"$var2", 24.0, EVAL_ERROR_OK, true},
    {"$var1 + $var2", 66.0, EVAL_ERROR_OK, true},
    {"$var1 * $var2", 1008.0, EVAL_ERROR_OK, true},
    {"$var1 > $var2", 1.0, EVAL_ERROR_OK, true},
    {"$var1 < $var2", 0.0, EVAL_ERROR_OK, true},
    {"$var1 && $var2", 1.0, EVAL_ERROR_OK, true},
    {"$zero && $var1", 0.0, EVAL_ERROR_OK, true},
    {"$var1", 42.0, EVAL_ERROR_OK, true}, // Test dollar sign prefix
    {"${var1}", 42.0, EVAL_ERROR_OK, true}, // Test with curly braces
    {"$unknown", 0.0, EVAL_ERROR_UNKNOWN_VARIABLE, true},
};

// Test cases for function calls
static TestCase function_tests[] = {
    {"abs(5)", 5.0, EVAL_ERROR_OK, true},
    {"abs(-5)", 5.0, EVAL_ERROR_OK, true},
    {"abs(0)", 0.0, EVAL_ERROR_OK, true},
    {"abs($var1)", 42.0, EVAL_ERROR_OK, true},
    {"abs($negative)", 10.0, EVAL_ERROR_OK, true},
    {"abs(1 + -3)", 2.0, EVAL_ERROR_OK, true},
    {"abs($var1 - $var2)", 18.0, EVAL_ERROR_OK, true},
    {"abs(abs(-5))", 5.0, EVAL_ERROR_OK, true}, // Nested function call
};

// Test cases for special values
// In Netdata, NaN values cause VALUE_IS_UNSET errors and Infinity causes VALUE_IS_INFINITE errors
static TestCase special_value_tests[] = {
    // NaN tests - Netdata rejects them with VALUE_IS_UNSET error
    {"$nan_var", 0.0, EVAL_ERROR_VALUE_IS_NAN, true},

    // Comparison operators with NaN - these should work as they just check NaN status
    {"$nan_var == 5", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var != 5", 1.0, EVAL_ERROR_OK, true},
    {"$nan_var > 5", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var < 5", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var >= 5", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var <= 5", 0.0, EVAL_ERROR_OK, true},

    // NaN self-comparison (Netdata treats NaN == NaN as true, which is different from IEEE 754)
    {"$nan_var == $nan_var", 1.0, EVAL_ERROR_OK, true},
    {"$nan_var != $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var > $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var < $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var >= $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var <= $nan_var", 0.0, EVAL_ERROR_OK, true},

    // Logical operations with NaN
    {"$nan_var && 1", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var || 1", 1.0, EVAL_ERROR_OK, true},
    {"$nan_var && 0", 0.0, EVAL_ERROR_OK, true},
    {"$nan_var || 0", 0.0, EVAL_ERROR_OK, true},
    {"!$nan_var", 1.0, EVAL_ERROR_OK, true},

    // Ternary with NaN
    {"($nan_var) ? 1 : 2", 2.0, EVAL_ERROR_OK, true},

    // Infinity tests - Netdata rejects with VALUE_IS_INFINITE error
    {"$inf_var", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true},

    // Comparison operators with Infinity - these work
    {"$inf_var == 5", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var != 5", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var > 5", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var < 5", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var >= 5", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var <= 5", 0.0, EVAL_ERROR_OK, true},

    // Infinity self-comparison
    {"$inf_var == $inf_var", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var != $inf_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var > $inf_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var < $inf_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var >= $inf_var", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var <= $inf_var", 1.0, EVAL_ERROR_OK, true},

    // Logical operations with Infinity
    {"$inf_var && 1", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var || 1", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var && 0", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var || 0", 1.0, EVAL_ERROR_OK, true},
    {"!$inf_var", 0.0, EVAL_ERROR_OK, true},

    // Ternary with Infinity
    {"($inf_var) ? 1 : 2", 1.0, EVAL_ERROR_OK, true},

    // Zero division
    {"5 / 0", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true}, // Positive/zero gives infinity
    {"-5 / 0", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true}, // Negative/zero gives -infinity
    {"0 / 0", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true}, // In Netdata, this gives INFINITE error

    // NaN and Infinity comparison
    {"$inf_var == $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var != $nan_var", 1.0, EVAL_ERROR_OK, true},
    {"$inf_var > $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var < $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var >= $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var <= $nan_var", 0.0, EVAL_ERROR_OK, true},

    // Logical operations with mixed NaN and Infinity
    {"$inf_var && $nan_var", 0.0, EVAL_ERROR_OK, true},
    {"$inf_var || $nan_var", 1.0, EVAL_ERROR_OK, true},
    {"!$nan_var && $inf_var", 1.0, EVAL_ERROR_OK, true},
    {"!$nan_var || !$inf_var", 1.0, EVAL_ERROR_OK, true},

    // Special value operations with zero
    {"$zero * $inf_var", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true},
    {"$zero / $zero", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true}, // Netdata treats 0/0 as INFINITE
    {"($zero) ? 1 : 2", 2.0, EVAL_ERROR_OK, true},

    // Short-circuit evaluation with special values (these work because no evaluation happens)
    {"0 && $nan_var", 0.0, EVAL_ERROR_OK, true}, // Short-circuit should avoid NaN
    {"1 || $nan_var", 1.0, EVAL_ERROR_OK, true}, // Short-circuit should avoid NaN
    {"0 && $inf_var", 0.0, EVAL_ERROR_OK, true}, // Short-circuit should avoid Infinity
    {"1 || $inf_var", 1.0, EVAL_ERROR_OK, true}  // Short-circuit should avoid Infinity
};

// Complex expression tests
static TestCase complex_tests[] = {
    {"1 + 2 * 3 - 4 / 2", 5.0, EVAL_ERROR_OK, true},
    {"(1 + 2) * (3 - 4) / 2", -1.5, EVAL_ERROR_OK, true},
    {"1 > 0 && 2 > 1", 1.0, EVAL_ERROR_OK, true},
    {"1 > 0 || 0 > 1", 1.0, EVAL_ERROR_OK, true},
    {"(1 > 0) ? 10 : 20", 10.0, EVAL_ERROR_OK, true},
    {"(0 > 1) ? 10 : 20", 20.0, EVAL_ERROR_OK, true},
    {"((($var1 + $var2) / 2) > 30) ? ($var1 * $var2) : ($var1 + $var2)", 1008.0, EVAL_ERROR_OK, true},
    {"5 + (!($var1 > 50) * 10)", 15.0, EVAL_ERROR_OK, true},
    {"($var1 > $var2) ? ($var1 - $var2) : ($var2 - $var1)", 18.0, EVAL_ERROR_OK, true},
    {"(($zero > 0) ? $var1 : $var2) + (($zero < 0) ? $var1 : $var2)", 48.0, EVAL_ERROR_OK, true},
};

// Edge case and invalid expressions
static TestCase edge_case_tests[] = {
    {"", 0.0, EVAL_ERROR_OK, false},
    {" ", 0.0, EVAL_ERROR_MISSING_OPERAND, false},        // Netdata can't parse whitespace-only expressions
    {"\t\n", 0.0, EVAL_ERROR_MISSING_OPERAND, false},     // Netdata can't parse whitespace-only expressions
    {"    5    +    3    ", 8.0, EVAL_ERROR_OK, true},    // Whitespace between operands is fine
    {"$", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},      // Netdata's error is different for incomplete variables
    {"${", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},     // Netdata's error is different for incomplete variables
    {"$}", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},     // Netdata's error is different for incomplete variables
    {"${}", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},    // Netdata's error is different for incomplete variables
    {"5 + -3", 2.0, EVAL_ERROR_OK, true},                 // Netdata actually handles this correctly as a unary minus
    {"5 + 3", 8.0, EVAL_ERROR_OK, true},                  // Basic sanity check
};

// Operator precedence tests
static TestCase precedence_tests[] = {
    {"5 + 3 * 2", 11.0, EVAL_ERROR_OK, true},                 // * before +
    {"5 * 3 + 2", 17.0, EVAL_ERROR_OK, true},                 // * before +
    {"5 + 3 - 2", 6.0, EVAL_ERROR_OK, true},                  // + and - same precedence (left to right)
    {"5 - 3 + 2", 4.0, EVAL_ERROR_OK, true},                  // + and - same precedence (left to right)
    {"5 * 3 / 3", 5.0, EVAL_ERROR_OK, true},                  // * and / same precedence (left to right)
    {"5 / 5 * 3", 3.0, EVAL_ERROR_OK, true},                  // * and / same precedence (left to right)
    {"5 > 3 && 2 < 4 || 1 == 0", 1.0, EVAL_ERROR_OK, true},   // && before ||
    {"5 > 3 && (2 < 4 || 1 == 0)", 1.0, EVAL_ERROR_OK, true}, // same as above with explicit grouping
    {"5 > 3 || 2 < 4 && 1 == 0", 0.0, EVAL_ERROR_OK, true},   // In Netdata, || and && have same precedence (left to right)
    {"(5 > 3 || 2 < 4) && 1 == 0", 0.0, EVAL_ERROR_OK, true}, // explicit grouping with same result
    {"!5 > 3", 0.0, EVAL_ERROR_OK, true},                     // ! before >
    {"!(5 > 3)", 0.0, EVAL_ERROR_OK, true},                   // same as above with explicit grouping
    {"5 + 3 > 2 * 3", 1.0, EVAL_ERROR_OK, true},              // arithmetic before comparison
    {"5 + 3 > 2 * 4", 0.0, EVAL_ERROR_OK, true},              // arithmetic before comparison
    {"(5 > 3) ? (1 + 2) : (3 + 4)", 3.0, EVAL_ERROR_OK, true},      // ternary has low precedence
    {"($var1 + $var2 * 2 > 80) ? 100 : 200", 100.0, EVAL_ERROR_OK, true}, // complex precedence test (42 + 24*2 = 90, which is > 80)
};

// Parentheses tests - specifically testing how parentheses change operator precedence
static TestCase parentheses_tests[] = {
    {"5 + 3 * 2", 11.0, EVAL_ERROR_OK, true},            // Default: * has higher precedence
    {"(5 + 3) * 2", 16.0, EVAL_ERROR_OK, true},          // Parentheses change precedence
    {"5 * (3 + 2)", 25.0, EVAL_ERROR_OK, true},          // Parentheses change order of operations
    {"(5 + 3 * 2)", 11.0, EVAL_ERROR_OK, true},          // Redundant parentheses don't change anything
    {"((5 + 3) * 2)", 16.0, EVAL_ERROR_OK, true},        // Nested parentheses
    {"5 - (3 - 1)", 3.0, EVAL_ERROR_OK, true},           // Parentheses with subtraction
    {"5 - 3 - 1", 1.0, EVAL_ERROR_OK, true},             // Without parentheses (left-to-right)
    {"(5 - 3) - 1", 1.0, EVAL_ERROR_OK, true},           // Explicit grouping doesn't change result
    {"5 - (3 - 1)", 3.0, EVAL_ERROR_OK, true},           // Different grouping changes result
    {"5 / (2 * 2.5)", 1.0, EVAL_ERROR_OK, true},         // Division and multiplication with parentheses
    {"(5 / 2) * 2.5", 6.25, EVAL_ERROR_OK, true},        // Different grouping changes result
    {"$var1 * ($var2 + 6)", 1260.0, EVAL_ERROR_OK, true}, // Variables with parentheses
    {"($var1 * $var2) + 6", 1014.0, EVAL_ERROR_OK, true}, // Different grouping with variables
    {"!($var1 > $var2)", 0.0, EVAL_ERROR_OK, true},      // Logical NOT with parentheses
    {"!(0)", 1.0, EVAL_ERROR_OK, true},                  // Logical NOT with constant
    {"!0", 1.0, EVAL_ERROR_OK, true},                    // Same without parentheses
    {"5 > 3 && (2 < 1 || 3 > 1)", 1.0, EVAL_ERROR_OK, true}, // Complex logical expression with parentheses
    {"(5 > 3 && 2 < 1) || 3 > 1", 1.0, EVAL_ERROR_OK, true}, // Different grouping changes result
    {"5 > 3 && 2 < 1 || 3 > 1", 1.0, EVAL_ERROR_OK, true},   // Default precedence (&& before ||)
    {"(5 > 3) && ((2 < 1) || (3 > 1))", 1.0, EVAL_ERROR_OK, true}, // Excessive parentheses
    {"(((5))) + (((3)))", 8.0, EVAL_ERROR_OK, true},     // Multiple nested parentheses
    {"abs(-($var1 - $var2))", 18.0, EVAL_ERROR_OK, true}, // Function with parenthesized expression
    {"abs(-(($var1) - ($var2)))", 18.0, EVAL_ERROR_OK, true}, // Function with nested parentheses
    {"(5 > 3) ? ($var1 + $var2) : ($var1 - $var2)", 66.0, EVAL_ERROR_OK, true}, // Ternary with parentheses
    {"((5 > 3) ? $var1 : $var2) + 10", 52.0, EVAL_ERROR_OK, true}, // Parentheses around ternary
};

// Tests for API functions
static TestCase api_function_tests[] = {
    {"1 + 2", 3.0, EVAL_ERROR_OK, true},  // For testing expression_source and expression_parsed_as
    {"$var1", 42.0, EVAL_ERROR_OK, true},  // For testing expression_result and variable lookup
    {"bad/syntax", 0.0, EVAL_ERROR_UNKNOWN_OPERAND, false},  // For testing expression_error_msg
    {"$hardcoded_var", 0.0, EVAL_ERROR_UNKNOWN_VARIABLE, true},  // For testing expression_hardcode_variable
};

// Test cases for number overflow
static TestCase overflow_tests[] = {
    // Positive overflow - extreme large numbers
    {"1e308", 1e308, EVAL_ERROR_OK, true},  // Very large but valid number
    {"1e308 * 10", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Overflow to positive infinity
    {"1e308 + 1e308", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Addition causing overflow

    // Negative overflow - extreme large negative numbers
    {"-1e308", -1e308, EVAL_ERROR_OK, true},  // Very large negative but valid
    {"-1e308 * 10", -INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Overflow to negative infinity
    {"-1e308 - 1e308", -INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Subtraction causing negative overflow

    // Operations with infinity
    {"1e308 * 1e308", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Multiplication causing overflow
    {"-1e308 * -1e308", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Negative * negative = positive overflow
    {"1e308 / 1e-308", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Division causing overflow

    // Mixed operations
    {"1e308 - 1e308", 0.0, EVAL_ERROR_OK, true},  // This should properly cancel out
    {"(1e308 * 2) / 2", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},  // Overflow in intermediate calculation
};

// Test cases for combined complex expressions
static TestCase combined_tests[] = {
    // Complex arithmetic expressions combining multiple operations
    {"(5 + 3 * 2) / (1 + 1) * 4 - 10", 12.0, EVAL_ERROR_OK, true},
    {"((($var1 * 2) / 4) + (($var2 - 4) * 2)) / 10", 6.1, EVAL_ERROR_OK, true}, // Corrected expected result
    {"abs($negative) * 2 + $var1 / 2 - $var2", 17.0, EVAL_ERROR_OK, true}, // Corrected expected result

    // Complex boolean expressions with multiple conditions
    {"($var1 > 40 && $var2 < 30) || ($var1 - $var2 > 10)", 1.0, EVAL_ERROR_OK, true},
    {"!($var1 < 40) && ($var2 > 20 || $zero < 1) && !($var1 == $var2)", 1.0, EVAL_ERROR_OK, true},
    // Ternary expressions need proper parentheses in Netdata
    {"(($var1 > $var2) ? ($var1 - $var2) : ($var2 - $var1)) > 15", 1.0, EVAL_ERROR_OK, true},

    // Mix of arithmetic and boolean with precedence tests
    {"($var1 + $var2) / 2 > ($var1 > $var2 ? $var2 : $var1)", 1.0, EVAL_ERROR_OK, true},
    {"(($var1 > $var2 ? 1 : 0) * 10 + (($var1 - $var2) / 3)) > 15", 1.0, EVAL_ERROR_OK, true},

    // Complex expressions with potential overflows
    {"(1e308 - 1e308) * $var1 + $var2", 24.0, EVAL_ERROR_OK, true},
    {"($var1 > 0 ? 1e308 : -1e308) * ($var1 < 0 ? 1 : 0)", 0.0, EVAL_ERROR_OK, true},
    {"(1e308 + 1e308 > 0) ? $var1 : $var2", 42.0, EVAL_ERROR_OK, true},

    // Deeply nested expressions with mixed operations
    {"((((($var1 / 2) + ($var2 * 2)) - 10) * 2) / 4) + (($var1 > $var2) ? 5 : -5)", 34.5, EVAL_ERROR_OK, true}, // Corrected expected result
    // This tests the nested ternaries with proper parentheses
    {"(abs($negative) > 5) ? $var1 : $var2", 42.0, EVAL_ERROR_OK, true}, // Simplified to avoid nested ternary issue
    {"(($var1 + $var2) / 2 > 30) ? 10 : 5", 10.0, EVAL_ERROR_OK, true}, // Simplified second half of above test

    // Expressions with short-circuit evaluation
    {"$zero && (1 / $zero)", 0.0, EVAL_ERROR_OK, true},  // Short-circuit prevents division by zero
    {"1 || (1e308 * 1e308)", 1.0, EVAL_ERROR_OK, true},  // Short-circuit prevents overflow
    // Split this into two tests to avoid nested ternary issues
    {"($var1 < 0) ? (1 / $zero) : $var1", 42.0, EVAL_ERROR_OK, true}, // First part of previous test
    {"($var2 > 100) ? (1e308 * 1e308) : $var2", 24.0, EVAL_ERROR_OK, true}, // Second part of previous test
};

// Test cases that previously crashed Netdata
// Note: When using the re2c/lemon parser, nested ternary operators without parentheses
// are properly supported (unlike the original recursive descent parser)
static TestCase crash_tests[] = {
#ifdef USE_RE2C_LEMON_PARSER
    {"$var1 > 0 ? $var1 < 0 ? 1 : 2 : 3", 2.0, EVAL_ERROR_OK, true},
    {"$var1 > 0 ? ( $var1 < 0 ? 1 : 2 ) : 3", 2.0, EVAL_ERROR_OK, true},
    {"( $var1 > 0 ? $var1 < 0 ? 1 : 2 : 3 )", 2.0, EVAL_ERROR_OK, true},
#else
    // Original recursive descent parser can't handle nested ternaries without parentheses
    {"$var1 > 0 ? $var1 < 0 ? 1 : 2 : 3", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},
    {"$var1 > 0 ? ( $var1 < 0 ? 1 : 2 ) : 3", 1.0, EVAL_ERROR_OK, true},
    {"( $var1 > 0 ? $var1 < 0 ? 1 : 2 : 3 )", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},
#endif
    // Fully parenthesized works correctly in both parsers
    {"($var1 > 0) ? (($var1 < 0) ? 1 : 2) : 3", 2.0, EVAL_ERROR_OK, true},
    // Multiple nested parentheses are fine
    {"(($zero)) ? 0 : ((($var1)))", 42.0, EVAL_ERROR_OK, true},
    // Variable lookup errors are properly handled
    {"$nonexistent + $var1", 0.0, EVAL_ERROR_UNKNOWN_VARIABLE, true},
    // Division by (0-0) gives an INFINITE error in Netdata's implementation
    {"10 / ($zero - $zero)", 0.0, EVAL_ERROR_VALUE_IS_INFINITE, true},
    {"true", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},
    {"false", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},
};

// Test cases for variable names with spaces
static TestCase variable_space_tests[] = {
    // Testing $var syntax (can't have spaces)
    {"$this", 50.0, EVAL_ERROR_OK, true},
    {"$this variable", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false}, // This should fail to parse as "variable" is considered garbage
    {"$this + variable", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false}, // Invalid syntax

    // Testing ${var} syntax (can have spaces)
    {"${this}", 50.0, EVAL_ERROR_OK, true},
    {"${this variable}", 100.0, EVAL_ERROR_OK, true}, // This should parse as a single variable named 'this variable'

    // Testing more complex expressions with spaced variable names
    {"${this variable} * 2", 200.0, EVAL_ERROR_OK, true},
    {"${this variable} > ${this}", 1.0, EVAL_ERROR_OK, true},
    {"${this} + ${this variable}", 150.0, EVAL_ERROR_OK, true},

    // Edge cases with missing or incomplete braces
#ifdef USE_RE2C_LEMON_PARSER
    {"${this variable", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false}, // Missing closing brace is a syntax error
#else
    {"${this variable", 100.0, EVAL_ERROR_OK, true}, // Missing closing brace but parser accepts it
#endif
    {"${}", 0.0, EVAL_ERROR_REMAINING_GARBAGE, false},             // Empty brackets

    // Using ${var} inside complex expressions
    {"(${this variable} + ${this}) / 2", 75.0, EVAL_ERROR_OK, true},
    {"(${this} > 0) ? ${this variable} : 0", 100.0, EVAL_ERROR_OK, true}, // Fixed the ternary syntax
    {"$1var", 75.0, EVAL_ERROR_OK, true},
    {"${1var}", 75.0, EVAL_ERROR_OK, true},
    {"$_var", 76.0, EVAL_ERROR_OK, true},
    {"${_var}", 76.0, EVAL_ERROR_OK, true},
    {"$1.var", 77.0, EVAL_ERROR_OK, true},
    {"${1.var}", 77.0, EVAL_ERROR_OK, true},
    {"$var.1", 78.0, EVAL_ERROR_OK, true},
    {"${var.1}", 78.0, EVAL_ERROR_OK, true},
    {"$var-1", 0.0, EVAL_ERROR_UNKNOWN_VARIABLE, true},
    {"${var-1}", 79.0, EVAL_ERROR_OK, true},
};

// Test cases for nested unary operators
static TestCase nested_unary_tests[] = {
    // Nested minus operator
    {"-(-5)", 5.0, EVAL_ERROR_OK, true},
    {"-(-0)", 0.0, EVAL_ERROR_OK, true},
    {"-(-$negative)", -10.0, EVAL_ERROR_OK, true},
    {"-(-$nan_var)", NAN, EVAL_ERROR_VALUE_IS_NAN, true},
    {"-(-$inf_var)", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},
    
    // Nested plus operator
    {"+(-5)", -5.0, EVAL_ERROR_OK, true},
    {"+(-0)", 0.0, EVAL_ERROR_OK, true},
    {"+($negative)", -10.0, EVAL_ERROR_OK, true},
    {"+($nan_var)", NAN, EVAL_ERROR_VALUE_IS_NAN, true},
    {"+($inf_var)", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},
    {"+(+5)", 5.0, EVAL_ERROR_OK, true},
    
    // Nested not operator
    {"!(!0)", 0.0, EVAL_ERROR_OK, true},
    {"!(!1)", 1.0, EVAL_ERROR_OK, true},
    {"!(!$zero)", 0.0, EVAL_ERROR_OK, true},
    {"!(!$negative)", 1.0, EVAL_ERROR_OK, true},
    {"!(!$nan_var)", 0.0, EVAL_ERROR_OK, true},
    {"!(!$inf_var)", 1.0, EVAL_ERROR_OK, true},
    
    // Multiple nested unary operators
    {"-(-(-5))", -5.0, EVAL_ERROR_OK, true},
    {"+(-(-5))", 5.0, EVAL_ERROR_OK, true},
    {"-(-(-(-5)))", 5.0, EVAL_ERROR_OK, true},
    {"!(!(!0))", 1.0, EVAL_ERROR_OK, true},
    {"!(!(!1))", 0.0, EVAL_ERROR_OK, true},
    
    // Nested abs function
    {"abs(abs(-5))", 5.0, EVAL_ERROR_OK, true},
    {"abs(-abs(-5))", 5.0, EVAL_ERROR_OK, true}, // Now fixed
    {"abs(abs($negative))", 10.0, EVAL_ERROR_OK, true},
    {"abs(abs($nan_var))", NAN, EVAL_ERROR_VALUE_IS_NAN, true},
    {"abs(abs($inf_var))", INFINITY, EVAL_ERROR_VALUE_IS_INFINITE, true},
    
    // Mixed unary operators - fix the expected value for abs(!1)
    // For now, let's correct the test case for abs(!1)
    {"abs(-(-5))", 5.0, EVAL_ERROR_OK, true},
    {"abs(+(-5))", 5.0, EVAL_ERROR_OK, true},
    {"abs(!0)", 1.0, EVAL_ERROR_OK, true},
    {"abs(!1)", 0.0, EVAL_ERROR_OK, true}, // Changed from 1.0 to 0.0 because !1 is 0, abs(0) is 0
    {"-(!0)", -1.0, EVAL_ERROR_OK, true},
    {"-(!1)", 0.0, EVAL_ERROR_OK, true},
    {"+(!0)", 1.0, EVAL_ERROR_OK, true},
    {"+(!1)", 0.0, EVAL_ERROR_OK, true},
    
    // Complex nested expressions
    {"-(5 + -3)", -2.0, EVAL_ERROR_OK, true},
    {"+(5 + -3)", 2.0, EVAL_ERROR_OK, true},
    {"!(5 > 3)", 0.0, EVAL_ERROR_OK, true},
    {"!!(5 > 3)", 1.0, EVAL_ERROR_OK, true},
    {"abs(-(5 - 10))", 5.0, EVAL_ERROR_OK, true},
    {"-abs(-(5 - 10))", -5.0, EVAL_ERROR_OK, true}, // Now fixed
};

// Define the test groups
static TestGroup test_groups[] = {
    {"Arithmetic Tests", arithmetic_tests, ARRAY_SIZE(arithmetic_tests)},
    {"Comparison Tests", comparison_tests, ARRAY_SIZE(comparison_tests)},
    {"Logical Tests", logical_tests, ARRAY_SIZE(logical_tests)},
    {"Variable Tests", variable_tests, ARRAY_SIZE(variable_tests)},
    {"Variable Space Tests", variable_space_tests, ARRAY_SIZE(variable_space_tests)},
    {"Function Tests", function_tests, ARRAY_SIZE(function_tests)},
    {"Special Value Tests", special_value_tests, ARRAY_SIZE(special_value_tests)},
    {"Complex Expression Tests", complex_tests, ARRAY_SIZE(complex_tests)},
    {"Edge Case Tests", edge_case_tests, ARRAY_SIZE(edge_case_tests)},
    {"Operator Precedence Tests", precedence_tests, ARRAY_SIZE(precedence_tests)},
    {"Parentheses Tests", parentheses_tests, ARRAY_SIZE(parentheses_tests)},
    {"Nested Unary Tests", nested_unary_tests, ARRAY_SIZE(nested_unary_tests)},
    {"API Function Tests", api_function_tests, ARRAY_SIZE(api_function_tests)},
    {"Number Overflow Tests", overflow_tests, ARRAY_SIZE(overflow_tests)},
    {"Combined Complex Expressions", combined_tests, ARRAY_SIZE(combined_tests)},
    {"Crash Tests", crash_tests, ARRAY_SIZE(crash_tests)},
};

void eval_unittest_ast(void) {
    for (size_t i = 0; i < ARRAY_SIZE(test_groups); i++) {
        TestGroup *group = &test_groups[i];

        printf("=== Running Test Group: %s ===\n", group->name);

        for (int j = 0; j < group->test_count; j++) {
            TestCase *tc = &group->test_cases[j];
            printf("Test %d: %s\n", j + 1, tc->expression);
            ASTNode *ast = parse_expression_ast(tc->expression);

            if (ast != NULL) {
                printf("AST Structure:\n");
                print_ast(ast, 2);
                eval_ast_node_free(ast);
            } else {
                printf("Failed to parse expression\n");
            }

            printf("\n");
        }
    }
}

int eval_unittest(void) {
    // Test cases for basic arithmetic operations

    // the ast experiment is commented out
    // eval_unittest_ast();

    // Run all test groups
    int total_passed = 0;
    int total_failed = 0;
    int total_tests = 0;
    
#ifdef USE_RE2C_LEMON_PARSER
    printf("Starting comprehensive evaluation tests using RE2C/LEMON PARSER\n");
#else
    printf("Starting comprehensive evaluation tests using RECURSIVE DESCENT PARSER\n");
#endif
    
    for (size_t i = 0; i < ARRAY_SIZE(test_groups); i++) {
        run_test_group(&test_groups[i]);
        
        int group_tests = test_groups[i].test_count;
        int group_passed = 0;
        int group_failed = 0;
        
        for (int j = 0; j < group_tests; j++) {
            TestCase *tc = &test_groups[i].test_cases[j];
            const char *failed_at = NULL;
            int error = 0;
            
            // Try to parse the expression
            EVAL_EXPRESSION *exp = expression_parse(tc->expression, &failed_at, &error);
            
            // Check if parsing succeeded as expected
            bool test_failed = false;
            
            if (tc->should_parse && !exp) {
                test_failed = true;
            }
            else if (!tc->should_parse && exp) {
                test_failed = true;
            }
            
            // If the expression parsed successfully, evaluate it
            if (exp) {
                // Set up the variable lookup callback
                expression_set_variable_lookup_callback(exp, test_variable_lookup, NULL);
                
                // Evaluate the expression
                expression_evaluate(exp);
                
                // Check if there was an error during evaluation
                if (tc->expected_error != EVAL_ERROR_OK && exp->error == EVAL_ERROR_OK) {
                    test_failed = true;
                }
                else if (tc->expected_error == EVAL_ERROR_OK && exp->error != EVAL_ERROR_OK) {
                    test_failed = true;
                }
                else if (tc->expected_error != EVAL_ERROR_OK && exp->error != tc->expected_error) {
                    test_failed = true;
                }
                
                // Check the evaluation result
                if (tc->expected_error == EVAL_ERROR_OK) {
                    if (isnan(tc->expected_result) && !isnan(exp->result)) {
                        test_failed = true;
                    }
                    else if (isinf(tc->expected_result) && !isinf(exp->result)) {
                        test_failed = true;
                    }
                    else if (!isnan(tc->expected_result) && !isinf(tc->expected_result) &&
                             !isnan(exp->result) && !isinf(exp->result) &&
                             fabs(tc->expected_result - exp->result) > 0.000001) {
                        test_failed = true;
                    }
                }
                
                // Clean up
                expression_free(exp);
            }
            
            if (test_failed) {
                group_failed++;
            } else {
                group_passed++;
            }
        }
        
        total_passed += group_passed;
        total_failed += group_failed;
        total_tests += group_tests;
    }
    
    printf("\n========== OVERALL TEST SUMMARY ==========\n");
    printf("Total tests: %d\n", total_tests);
    printf("Passed: %d (%.1f%%)\n", total_passed, (float)total_passed / total_tests * 100);
    printf("Failed: %d (%.1f%%)\n", total_failed, (float)total_failed / total_tests * 100);
    
    return total_failed > 0 ? 1 : 0;
}
