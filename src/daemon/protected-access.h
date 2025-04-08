// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef PROTECTED_ACCESS_H
#define PROTECTED_ACCESS_H

#include "libnetdata/libnetdata.h"
#include <setjmp.h>

typedef struct {
    const char *caller;
    sigjmp_buf      jump_buffer;          // Where to jump back to
    void           *protected_start_addr; // Start of the monitored memory range
    size_t          protected_size;       // Size of the monitored memory range
    // 0=inactive, 1=active (in protected block), 2=jump occurred
    volatile sig_atomic_t is_active;      // Must be sig_atomic_t for signal handler safety
} protected_access_t;

extern __thread protected_access_t protected_access_state;

#define PROTECTED_ACCESS_START(start, size) ({                                      \
    bool _rc = false;                                                               \
                                                                                    \
    if (protected_access_state.is_active == 1)                                      \
        fatal("PROTECTED ACCESS: nested PROTECTED_ACCESS_START attempted from "     \
                "function %s, while the active is from function %s!",               \
              __FUNCTION__, protected_access_state.caller);                         \
                                                                                    \
    if (start && size) {                                                            \
        protected_access_state.protected_start_addr = start;                        \
        protected_access_state.protected_size = size;                               \
        protected_access_state.is_active = 1;                                       \
        protected_access_state.caller = __FUNCTION__;                               \
        if (sigsetjmp(protected_access_state.jump_buffer, 1) == 0) {                \
            /* Initial call successful, sigsetjmp returns 0. */                     \
            _rc = true;                                                             \
        } else {                                                                    \
            /* Returned here via siglongjmp from the signal handler. */             \
            /* The handler should have set state->is_active = 2. */                 \
            /* Return false to indicate recovery path should be taken. */           \
            _rc = false;                                                            \
        }                                                                           \
    }                                                                               \
    _rc;                                                                            \
})

static inline void protected_access_end(volatile int *ptr __maybe_unused) {
    protected_access_state.is_active = 0;
    protected_access_state.protected_start_addr = NULL;
    protected_access_state.protected_size = 0;
    /* No need to clear jump_buffer explicitly */
}

#define PROTECTED_ACCESS_AUTO_CLEANUP() \
    volatile int _pa_dummy_cleanup_var __attribute__((cleanup(protected_access_end), unused)) = 0; \

#define PROTECTED_ACCESS_END() protected_access_end(NULL);

#define PROTECTED_ACCESS_SETUP(start, size)                                         \
    PROTECTED_ACCESS_AUTO_CLEANUP();                                                \
    bool no_signal_received = PROTECTED_ACCESS_START(start, size);                  \

void signal_protected_access_check(int sig, siginfo_t *si, void *context);

#endif // PROTECTED_ACCESS_H
