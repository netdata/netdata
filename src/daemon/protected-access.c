// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "protected-access.h"

__thread protected_access_t protected_access_state = {0};

// Declare the thread-local state variable, initialized to zero/inactive.
// *** RELIES ON ASYNC-SIGNAL-SAFE ACCESS TO THIS VARIABLE ***

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

    // 1. Is protection currently active for *this thread*?
    // Check for state '1' specifically. Don't act if inactive ('0') or jump already happened ('2').
    if (state->is_active != 1)
        return; // Protection not active, handler should ignore.

    // 2. Is it a signal we want to handle this way?
    // Typically SIGBUS or SIGSEGV for memory access errors.
    if (sig != SIGBUS && sig != SIGSEGV)
        return; // Not a signal we are designed to recover from.

    // 3. Did the fault occur within the registered protected range?
    // Ensure siginfo_t is valid (it should be if SA_SIGINFO was used)
    if (si == NULL)
        return; // This shouldn't happen if sigaction was set up correctly with SA_SIGINFO

    void *fault_addr = si->si_addr;
    void *start_addr = state->protected_start_addr;
    // Perform boundary check carefully
    // Check if start_addr is valid before calculation
    if (start_addr == NULL) {
        // State inconsistency? Should not happen if is_active is 1.
        state->is_active = 0; // Attempt reset
        return;
    }

    // Calculate end address (exclusive)
    void *end_addr = (unsigned char *)start_addr + state->protected_size;

    if (fault_addr >= start_addr && fault_addr < end_addr) {
        // --- Conditions met! Perform recovery jump ---

        // Mark that recovery jump is occurring *before* jumping.
        // Set state to '2'. This prevents handler re-entry if another signal occurs
        // immediately, and signals to start() that recovery happened.
        state->is_active = 2;

        // Jump back to the sigsetjmp point in signal_protected_access_start()
        // The '1' becomes the non-zero return value of sigsetjmp.
        siglongjmp(state->jump_buffer, 1);

        // --- Execution should not reach here after siglongjmp ---
        // If it somehow did, something is fundamentally broken.
        fprintf(stderr, "FATAL: siglongjmp returned in signal handler!\n");
        abort();
        return; // Should be unreachable
    }

    // Signal occurred while active, but fault address was outside the protected range.
    // Let the default handler deal with it.
}
