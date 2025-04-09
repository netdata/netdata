// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "protected-access.h"

__thread protected_access_t protected_access_state = {0};

// Declare the thread-local state variable, initialized to zero/inactive.
// *** RELIES ON ASYNC-SIGNAL-SAFE ACCESS TO THIS VARIABLE ***

// Helper function to get diagnostic information from the last fault
const protected_access_frame_t *protected_access_get_last_fault(void) {
    if (protected_access_state.depth < 1)
        return NULL;
        
    protected_access_frame_t *frame = &protected_access_state.stack[protected_access_state.depth-1];
    if (frame->is_active != 2) // Not a frame with a fault
        return NULL;
        
    return frame;
}

// Format a string with diagnostic information about the last fault
void protected_access_format_error(char *buffer, size_t buffer_size) {
    const protected_access_frame_t *frame = protected_access_get_last_fault();
    if (!frame) {
        snprintf(buffer, buffer_size, "No protected access fault information available");
        return;
    }
    
    // Use the proper public API for signal code formatting
    char signal_code_buf[128];
    SIGNAL_CODE_2str_h(frame->signal_code, signal_code_buf, sizeof(signal_code_buf));
    
    snprintf(buffer, buffer_size, 
        "Protected access fault in %s: %s %s failed with signal %s\n"
        "  Fault address: %p (offset +%lu within protected region %p-%p)",
        frame->caller, 
        frame->operation, 
        frame->resource_name,
        signal_code_buf,
        frame->fault_address,
        (unsigned long)((char*)frame->fault_address - (char*)frame->protected_start_addr),
        frame->protected_start_addr,
        (void*)((char*)frame->protected_start_addr + frame->protected_size)
    );
}

// --- Public API Function (called by signal handler) ---
void signal_protected_access_check(int sig, siginfo_t *si, void *context __maybe_unused) {
    // --- ASYNC-SIGNAL-SAFETY WARNING ---
    // The following access to the thread-local 'protected_access'
    // variable MUST be async-signal-safe on your specific target platform.
    // This includes reading is_active, protected_start_addr, protected_size,
    // AND the subsequent call to siglongjmp referencing state->jump_buffer.
    // Standard C/POSIX do NOT guarantee safety for general TLS access here.
    // Use with extreme caution and verify thoroughly.
    // --- END WARNING ---

    protected_access_t *state = &protected_access_state;

    // Make sure we have active frames
    if (state->depth == 0)
        return; // No protection active, handler should ignore.

    // 2. Is it a signal we want to handle this way?
    // Typically SIGBUS or SIGSEGV for memory access errors.
    if (sig != SIGBUS && sig != SIGSEGV)
        return; // Not a signal we are designed to recover from.

    // 3. Did the fault occur within the registered protected range?
    // Ensure siginfo_t is valid (it should be if SA_SIGINFO was used)
    if (si == NULL)
        return; // This shouldn't happen if sigaction was set up correctly with SA_SIGINFO

    void *fault_addr = si->si_addr;

    // Start from the most recent frame and work backwards
    for (sig_atomic_t i = state->depth - 1; i >= 0; i--) {
        protected_access_frame_t *frame = &state->stack[i];
        
        // Skip inactive frames (shouldn't happen but check anyway)
        if (frame->is_active != 1)
            continue;
            
        void *start_addr = frame->protected_start_addr;
        
        // Check if start_addr is valid
        if (start_addr == NULL)
            continue;
            
        // Calculate end address (exclusive)
        void *end_addr = (unsigned char *)start_addr + frame->protected_size;

        if (fault_addr >= start_addr && fault_addr < end_addr) {
            // --- Conditions met! Perform recovery jump ---

            // Mark that recovery jump is occurring *before* jumping.
            // Set frame to '2'. This prevents handler re-entry if another signal occurs
            // immediately, and signals to start() that recovery happened.
            frame->is_active = 2;
            
            // Store diagnostic information about the fault
            frame->fault_address = fault_addr;
            frame->signal_code = signal_code(sig, si->si_code);

            // Update the depth to unwind all nested frames up to this one
            state->depth = i;

            // Jump back to the sigsetjmp point in PROTECTED_ACCESS_START
            // The '1' becomes the non-zero return value of sigsetjmp.
            siglongjmp(frame->jump_buffer, 1);

            // --- Execution should not reach here after siglongjmp ---
            // If it somehow did, something is fundamentally broken.
            fprintf(stderr, "FATAL: siglongjmp returned in signal handler!\n");
            abort();
            return; // Should be unreachable
        }
    }

    // Signal occurred while active, but fault address was outside all protected ranges.
    // Let the default handler deal with it.
}
