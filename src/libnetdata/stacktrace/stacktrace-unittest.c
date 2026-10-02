// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "stacktrace.h"
#include "stacktrace-common.h"

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
    STACKTRACE trace = stacktrace_get(0);
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

// Recurse deep enough that the formatted stack exceeds STACKTRACE_MAX_TEXT_LENGTH, then capture it
NEVER_INLINE
static int deep_recursion_to_capture_stack_trace(int depth, BUFFER *wb) {
    if (depth == 0) {
        stacktrace_capture(wb);
        return 0;
    }

    // a volatile local read after the call gives every level its own frame (no tail call, no loop)
    volatile int frame_marker = depth;
    int rc = deep_recursion_to_capture_stack_trace(depth - 1, wb);
    return rc + frame_marker;
}

// libbacktrace captures from signal handlers must stay within STACKTRACE_MAX_TEXT_LENGTH and never grow the buffer
static bool stacktrace_max_length_unittest(void) {
    if (strncmp(stacktrace_backend(), "libbacktrace", strlen("libbacktrace")) != 0) {
        fprintf(stderr, "\nSTACKTRACE MAX LENGTH TEST: SKIPPED (backend %s)\n", stacktrace_backend());
        return true;
    }

    BUFFER *wb = buffer_create(STACKTRACE_CAPTURE_MIN_BUFFER_SIZE, NULL);
    uint32_t size_before = wb->size;

    deep_recursion_to_capture_stack_trace(300, wb);

    size_t len = buffer_strlen(wb);
    const char *marker = STACKTRACE_TRUNCATED_MARKER;
    size_t marker_len = sizeof(STACKTRACE_TRUNCATED_MARKER) - 1;
    bool fits = len < STACKTRACE_MAX_TEXT_LENGTH;
    bool not_grown = wb->size == size_before;
    bool marked = len >= marker_len && strcmp(buffer_tostring(wb) + len - marker_len, marker) == 0;

    fprintf(stderr, "\nSTACKTRACE MAX LENGTH TEST: length %zu (limit %d), buffer size %u -> %u, marker %s: %s\n",
            len, STACKTRACE_MAX_TEXT_LENGTH, size_before, wb->size, marked ? "present" : "missing",
            fits && not_grown && marked ? "SUCCESS" : "FAILURE");

    buffer_free(wb);
    return fits && not_grown && marked;
}

#if defined(USE_LIBBACKTRACE)
#define OVERSIZED_FRAMES 300

// Formats synthetic frames and checks the result stays within STACKTRACE_MAX_TEXT_LENGTH without growing the buffer.
// Returns the formatted text, or NULL on failure.
static const char *stacktrace_format_synthetic_frames(BUFFER *wb, const uintptr_t *pcs, const char *const *functions,
                                                      size_t count, const char *when) {
    uint32_t size_before = wb->size;
    stacktrace_capture_frames_unittest(wb, pcs, functions, count);

    if (buffer_strlen(wb) >= STACKTRACE_MAX_TEXT_LENGTH || wb->size != size_before) {
        fprintf(stderr, " - %s: length %zu (limit %d), buffer size %u -> %u\n",
                when, buffer_strlen(wb), STACKTRACE_MAX_TEXT_LENGTH, size_before, wb->size);
        return NULL;
    }
    return buffer_tostring(wb);
}

// A frame too long to fit in full keeps its number and address and costs no other frame; when even that does not
// fit, the rest are dropped and the trace says so - it never claims there were no frames.
static bool stacktrace_oversized_frame_unittest(void) {
    bool ok = true;

    char *huge = mallocz(STACKTRACE_MAX_TEXT_LENGTH * 2);
    memset(huge, 'x', STACKTRACE_MAX_TEXT_LENGTH * 2 - 1);
    huge[STACKTRACE_MAX_TEXT_LENGTH * 2 - 1] = '\0';

    BUFFER *wb = buffer_create(STACKTRACE_CAPTURE_MIN_BUFFER_SIZE, NULL);

    // an oversized first frame, then a normal one
    {
        const uintptr_t pcs[] = { 0x1234, 0x5678 };
        const char *const functions[] = { huge, "second_function" };
        const char *text = stacktrace_format_synthetic_frames(wb, pcs, functions, 2, "oversized first frame");
        if (!text || strcmp(text, "#0 <frame too long> [0x1234]\n#1 second_function [0x5678]") != 0) {
            fprintf(stderr, " - oversized first frame: unexpected trace:\n%s\n", text ? text : "");
            ok = false;
        }
    }

    // more frames than fit, every one oversized: the compact ones fill the budget in order, then the marker
    {
        buffer_flush(wb);
        uintptr_t pcs[OVERSIZED_FRAMES];
        const char *functions[OVERSIZED_FRAMES];
        for (size_t i = 0; i < OVERSIZED_FRAMES; i++) {
            pcs[i] = 0x1000 + i;
            functions[i] = huge;
        }
        const char *text = stacktrace_format_synthetic_frames(wb, pcs, functions, OVERSIZED_FRAMES, "oversized frames");

        // rebuild what the kept frames must look like: every frame up to the first one missing, then the marker
        BUFFER *expected = buffer_create(STACKTRACE_CAPTURE_MIN_BUFFER_SIZE * 2, NULL);
        size_t kept = 0;
        for (; text && kept < OVERSIZED_FRAMES; kept++) {
            char frame[64];
            snprintfz(frame, sizeof(frame), "%s#%zu <frame too long> [0x%zX]", kept ? "\n" : "", kept, (size_t)pcs[kept]);
            if (strncmp(text + buffer_strlen(expected), frame, strlen(frame)) != 0)
                break;
            buffer_strcat(expected, frame);
        }
        buffer_strcat(expected, STACKTRACE_TRUNCATED_MARKER);

        if (!text || kept == 0 || kept == OVERSIZED_FRAMES || strcmp(text, buffer_tostring(expected)) != 0) {
            fprintf(stderr, " - oversized frames: %zu frames kept, unexpected trace:\n%s\n", kept, text ? text : "");
            ok = false;
        }
        buffer_free(expected);
    }

    buffer_free(wb);
    freez(huge);

    fprintf(stderr, "\nSTACKTRACE OVERSIZED FRAME TEST: %s\n", ok ? "SUCCESS" : "FAILURE");
    return ok;
}
#else
static bool stacktrace_oversized_frame_unittest(void) {
    fprintf(stderr, "\nSTACKTRACE OVERSIZED FRAME TEST: SKIPPED (backend %s)\n", stacktrace_backend());
    return true;
}
#endif

// Run the stacktrace unittest
int stacktrace_unittest(void) {
    // Initialize stacktrace subsystem
    stacktrace_init();

    bool cache_collision_test = stacktrace_cache_unittest() == 0;
    fprintf(stderr, "\nSTACKTRACE CACHE COLLISION TEST: %s\n", cache_collision_test ? "SUCCESS" : "FAILURE");
    
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
    bool max_length_test = stacktrace_max_length_unittest();
    bool oversized_frame_test = stacktrace_oversized_frame_unittest();

    bool test_success = cache_collision_test && direct_analysis && indirect_analysis && max_length_test &&
                        oversized_frame_test;
    fprintf(stderr, "\nSTACKTRACE TEST: Overall result: %s\n",
            test_success ? "SUCCESS" : "FAILURE");
    
    return test_success ? 0 : 1;
}
