// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef PROTECTED_ACCESS_H
#define PROTECTED_ACCESS_H

#include "libnetdata/libnetdata.h"
#include <setjmp.h>

// Maximum nesting depth for protected access regions
#define PROTECTED_ACCESS_MAX_NESTING 8

typedef struct {
    const char *caller;                   // Function that initiated the protected region
    sigjmp_buf      jump_buffer;          // Where to jump back to
    void           *protected_start_addr; // Start of the monitored memory range
    size_t          protected_size;       // Size of the monitored memory range
    // 0=inactive, 1=active (in protected block), 2=jump occurred
    volatile sig_atomic_t is_active;      // Must be sig_atomic_t for signal handler safety
    
    // Enhanced diagnostic information
    const char     *resource_name;        // Name/path of the resource being accessed (often a file path)
    const char     *operation;            // Operation being performed (e.g. "read", "write", "mmap")
    SIGNAL_CODE     signal_code;          // Signal and code combination (e.g. SIGSEGV/SEGV_MAPERR)
    void           *fault_address;        // The exact address that caused the fault
} protected_access_frame_t;

typedef struct {
    protected_access_frame_t stack[PROTECTED_ACCESS_MAX_NESTING];
    volatile sig_atomic_t depth;          // Current nesting depth (0 = no active protection)
} protected_access_t;

extern __thread protected_access_t protected_access_state;

#define PROTECTED_ACCESS_START(start, size, resource, op) ({                        \
    bool _rc = false;                                                               \
                                                                                    \
    if (protected_access_state.depth >= PROTECTED_ACCESS_MAX_NESTING)               \
        fatal("PROTECTED ACCESS: maximum nesting depth reached in function %s",     \
              __FUNCTION__);                                                        \
                                                                                    \
    if (start && size) {                                                            \
        /* Get the current frame on the stack */                                    \
        protected_access_frame_t *frame =                                           \
            &protected_access_state.stack[protected_access_state.depth];            \
                                                                                    \
        /* Initialize the frame */                                                  \
        frame->protected_start_addr = start;                                        \
        frame->protected_size = size;                                               \
        frame->is_active = 1;                                                       \
        frame->caller = __FUNCTION__;                                               \
        frame->resource_name = resource;                                            \
        frame->operation = op;                                                      \
        frame->signal_code = 0;                                                     \
        frame->fault_address = NULL;                                                \
                                                                                    \
        /* Increase the stack depth before setting up the jump */                   \
        protected_access_state.depth++;                                             \
                                                                                    \
        if (sigsetjmp(frame->jump_buffer, 1) == 0) {                                \
            /* Initial call successful, sigsetjmp returns 0. */                     \
            _rc = true;                                                             \
        } else {                                                                    \
            /* Returned here via siglongjmp from the signal handler. */             \
            /* The handler should have set frame->is_active = 2 */                  \
            /* and populated diagnostic information. */                             \
            /* Return false to indicate recovery path should be taken. */           \
            _rc = false;                                                            \
        }                                                                           \
    }                                                                               \
    _rc;                                                                            \
})

static inline void protected_access_end(volatile int *ptr __maybe_unused) {
    if (protected_access_state.depth > 0) {
        /* Decrease the stack depth */
        protected_access_state.depth--;
        
        /* Clear the frame at the current depth */
        protected_access_frame_t *frame = &protected_access_state.stack[protected_access_state.depth];
        frame->is_active = 0;
        frame->protected_start_addr = NULL;
        frame->protected_size = 0;
        /* No need to clear jump_buffer explicitly */
    }
}

#define PROTECTED_ACCESS_AUTO_CLEANUP()                                                             \
    volatile int _pa_dummy_cleanup_var __attribute__((cleanup(protected_access_end), unused)) = 0;

#define PROTECTED_ACCESS_END() protected_access_end(NULL);

#define PROTECTED_ACCESS_SETUP(start, size, resource, op)                           \
    PROTECTED_ACCESS_AUTO_CLEANUP();                                                \
    bool no_signal_received = PROTECTED_ACCESS_START(start, size, resource, op);    \
    if (!no_signal_received) {                                                      \
        char __pa_error_buf[1024];                                                  \
        protected_access_format_error(__pa_error_buf, sizeof(__pa_error_buf));      \
        nd_log(NDLS_DAEMON, NDLP_ERR, "%s", __pa_error_buf);                        \
    }

void signal_protected_access_check(int sig, siginfo_t *si, void *context);

// Function declarations for diagnostic functions
const protected_access_frame_t *protected_access_get_last_fault(void);
void protected_access_format_error(char *buffer, size_t buffer_size);

#endif // PROTECTED_ACCESS_H
