// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "stacktrace.h"

// Structure to hold all test data
typedef struct {
    BUFFER *direct_trace;           // Buffer for direct stack trace
    BUFFER *indirect_trace;         // Buffer for indirect stack trace
    BUFFER *direct_root_cause;      // Root cause function from direct capture
    BUFFER *indirect_root_cause;    // Root cause function from indirect capture
    const char *never_inline_fn;    // Name of the never-inline function
    const char *always_inline_fn;   // Name of the always-inline function
} stacktrace_test_data_t;

// Function to analyze a stack trace
static bool analyze_stack_trace(
    const char *stack_trace,
    const char *never_inline_fn,
    const char *always_inline_fn,
    const char *unittest_fn,
    const char *root_cause)
{
    fprintf(stderr, "--------------------------------------------------------------------------------\n");
    fprintf(stderr, "%s\n", stack_trace);
    fprintf(stderr, "--------------------------------------------------------------------------------\n");

    if (!stack_trace || !*stack_trace) {
        fprintf(stderr, " - empty stack trace\n");
        return false;
    }
    
    // Report presence of each function
    bool never_inline_found = strstr(stack_trace, never_inline_fn) != NULL;
    bool always_inline_found = strstr(stack_trace, always_inline_fn) != NULL;
    bool unittest_found = strstr(stack_trace, unittest_fn) != NULL;
    
    fprintf(stderr, " - %50.50s: %s\n",
            never_inline_fn, never_inline_found ? "FOUND" : "NOT FOUND");
    fprintf(stderr, " - %50.50s: %s\n",
            always_inline_fn, always_inline_found ? "FOUND" : "NOT FOUND");
    fprintf(stderr, " - %50.50s: %s\n",
            unittest_fn, unittest_found ? "FOUND" : "NOT FOUND");
    fprintf(stderr, " - %50.50s: %s\n",
            "root cause function",
            root_cause && *root_cause ? root_cause : "NOT FOUND");
    
    // We only require the unittest function to be present for the test to pass
    return unittest_found;
}

// This function will never be inlined
NEVER_INLINE
static void never_inline_function_to_capture_stack_trace(stacktrace_test_data_t *test_data) {
    test_data->never_inline_fn = __FUNCTION__;

    BUFFER *wb = buffer_create(4096, NULL);
    
    // Test 1: Direct capture
    stacktrace_capture(wb);
    buffer_strcat(test_data->direct_trace, buffer_tostring(wb));
    
    // Get root cause (first time)
    buffer_flush(test_data->direct_root_cause);
    buffer_strcat(test_data->direct_root_cause, stacktrace_root_cause_function());

    // Test 2: Indirect capture (get + to_buffer)
    buffer_flush(wb);
    STACKTRACE trace = stacktrace_get();
    if (trace) {
        stacktrace_to_buffer(trace, wb);
        buffer_strcat(test_data->indirect_trace, buffer_tostring(wb));
        
        // Get root cause (second time)
        buffer_flush(test_data->indirect_root_cause);
        buffer_strcat(test_data->indirect_root_cause, stacktrace_root_cause_function());
    }
    
    buffer_free(wb);
}

// This function will be inlined in the caller
ALWAYS_INLINE
static void inline_function_to_capture_stack_trace(stacktrace_test_data_t *test_data) {
    test_data->always_inline_fn = __FUNCTION__;

    // Call the non-inlined function
    never_inline_function_to_capture_stack_trace(test_data);
}

// Run the stacktrace unittest
int stacktrace_unittest(void) {
    // Initialize stacktrace subsystem
    stacktrace_init();
    
    // Setup test data structure
    stacktrace_test_data_t test_data = {
        .direct_trace = buffer_create(4096, NULL),
        .indirect_trace = buffer_create(4096, NULL),
        .direct_root_cause = buffer_create(4096, NULL),
        .indirect_root_cause = buffer_create(4096, NULL),
        .never_inline_fn = NULL,
        .always_inline_fn = NULL,
    };
    
    // Run the test function to gather stack traces
    inline_function_to_capture_stack_trace(&test_data);
    
    // Print basic test information
    fprintf(stderr, "\nSTACKTRACE TEST: Backend: %s\n", stacktrace_backend());

    // Analyze both stack traces
    fprintf(stderr, "\nDIRECT STACK TRACE\n");
    bool direct_analysis = analyze_stack_trace(
        buffer_tostring(test_data.direct_trace),
        test_data.never_inline_fn,
        test_data.always_inline_fn,
        "stacktrace_unittest",
        buffer_tostring(test_data.direct_root_cause));
    
    fprintf(stderr, "\nINDIRECT STACK TRACE\n");
    bool indirect_analysis = analyze_stack_trace(
        buffer_tostring(test_data.indirect_trace),
        test_data.never_inline_fn,
        test_data.always_inline_fn,
        "stacktrace_unittest",
        buffer_tostring(test_data.indirect_root_cause));
    
    // Free resources
    buffer_free(test_data.direct_trace);
    buffer_free(test_data.indirect_trace);
    buffer_free(test_data.direct_root_cause);
    buffer_free(test_data.indirect_root_cause);
    
    // Report overall test status - success if both analyses succeed
    bool test_success = direct_analysis && indirect_analysis;
    fprintf(stderr, "\nSTACKTRACE TEST: Overall result: %s\n",
            test_success ? "SUCCESS" : "FAILURE");
    
    return test_success ? 0 : 1;
}