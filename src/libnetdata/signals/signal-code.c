// SPDX-License-Identifier: GPL-3.0-or-later

#include "signal-code.h"
#include "libnetdata/libnetdata.h"

// Helper macro to create a SIGNAL_CODE value
#define SIGNAL_CODE_CREATE(signo, si_code) (((uint64_t)(signo) << 32) | (uint32_t)(si_code))

// Function to create a SIGNAL_CODE from signal number and signal code
SIGNAL_CODE signal_code(int signo, int si_code) {
    return SIGNAL_CODE_CREATE(signo, si_code);
}

// Define mapping from SIGNAL_CODE to string representation
ENUM_STR_MAP_DEFINE(SIGNAL_CODE) = {
    // SIGILL codes
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLOPC), "SIGILL/ILL_ILLOPC" },   // Illegal opcode
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLOPN), "SIGILL/ILL_ILLOPN" },   // Illegal operand
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLADR), "SIGILL/ILL_ILLADR" },   // Illegal addressing mode
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLTRP), "SIGILL/ILL_ILLTRP" },   // Illegal trap
    { SIGNAL_CODE_CREATE(SIGILL, ILL_PRVOPC), "SIGILL/ILL_PRVOPC" },   // Privileged opcode
    { SIGNAL_CODE_CREATE(SIGILL, ILL_PRVREG), "SIGILL/ILL_PRVREG" },   // Privileged register
    { SIGNAL_CODE_CREATE(SIGILL, ILL_COPROC), "SIGILL/ILL_COPROC" },   // Coprocessor error
    { SIGNAL_CODE_CREATE(SIGILL, ILL_BADSTK), "SIGILL/ILL_BADSTK" },   // Internal stack error
#ifdef ILL_BADIADDR
    { SIGNAL_CODE_CREATE(SIGILL, ILL_BADIADDR), "SIGILL/ILL_BADIADDR" }, // Unimplemented instruction address
#endif

    // SIGFPE codes
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_INTDIV), "SIGFPE/FPE_INTDIV" },   // Integer divide by zero
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_INTOVF), "SIGFPE/FPE_INTOVF" },   // Integer overflow
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTDIV), "SIGFPE/FPE_FLTDIV" },   // Floating point divide by zero
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTOVF), "SIGFPE/FPE_FLTOVF" },   // Floating point overflow
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTUND), "SIGFPE/FPE_FLTUND" },   // Floating point underflow
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTRES), "SIGFPE/FPE_FLTRES" },   // Floating point inexact result
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTINV), "SIGFPE/FPE_FLTINV" },   // Floating point invalid operation
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTSUB), "SIGFPE/FPE_FLTSUB" },   // Subscript out of range
#ifdef FPE_FLTUNK
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTUNK), "SIGFPE/FPE_FLTUNK" },   // Undiagnosed floating-point exception
#endif
#ifdef FPE_CONDTRAP
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_CONDTRAP), "SIGFPE/FPE_CONDTRAP" }, // Trap on condition
#endif

    // SIGSEGV codes
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_MAPERR), "SIGSEGV/SEGV_MAPERR" }, // Address not mapped to object
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ACCERR), "SIGSEGV/SEGV_ACCERR" }, // Invalid permissions for mapped object
#ifdef SEGV_BNDERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_BNDERR), "SIGSEGV/SEGV_BNDERR" }, // Bounds checking failure
#endif
#ifdef SEGV_PKUERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_PKUERR), "SIGSEGV/SEGV_PKUERR" }, // Protection key checking failure
#endif
#ifdef SEGV_ACCADI
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ACCADI), "SIGSEGV/SEGV_ACCADI" }, // ADI not enabled for mapped object
#endif
#ifdef SEGV_ADIDERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ADIDERR), "SIGSEGV/SEGV_ADIDERR" }, // Disrupting MCD error
#endif
#ifdef SEGV_ADIPERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ADIPERR), "SIGSEGV/SEGV_ADIPERR" }, // Precise MCD exception
#endif
#ifdef SEGV_MTEAERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_MTEAERR), "SIGSEGV/SEGV_MTEAERR" }, // Asynchronous ARM MTE error
#endif
#ifdef SEGV_MTESERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_MTESERR), "SIGSEGV/SEGV_MTESERR" }, // Synchronous ARM MTE exception
#endif
#ifdef SEGV_CPERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_CPERR), "SIGSEGV/SEGV_CPERR" },  // Control protection fault
#endif

    // SIGBUS codes
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_ADRALN), "SIGBUS/BUS_ADRALN" },   // Invalid address alignment
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_ADRERR), "SIGBUS/BUS_ADRERR" },   // Non-existent physical address
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_OBJERR), "SIGBUS/BUS_OBJERR" },   // Object specific hardware error
#ifdef BUS_MCEERR_AR
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_MCEERR_AR), "SIGBUS/BUS_MCEERR_AR" }, // Hardware memory error: action required
#endif
#ifdef BUS_MCEERR_AO
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_MCEERR_AO), "SIGBUS/BUS_MCEERR_AO" }, // Hardware memory error: action optional
#endif

    // SIGTRAP codes
#ifdef TRAP_BRKPT
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_BRKPT), "SIGTRAP/TRAP_BRKPT" },  // Process breakpoint
#endif
#ifdef TRAP_TRACE
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_TRACE), "SIGTRAP/TRAP_TRACE" },  // Process trace trap
#endif
#ifdef TRAP_BRANCH
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_BRANCH), "SIGTRAP/TRAP_BRANCH" }, // Process taken branch trap
#endif
#ifdef TRAP_HWBKPT
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_HWBKPT), "SIGTRAP/TRAP_HWBKPT" }, // Hardware breakpoint/watchpoint
#endif
#ifdef TRAP_UNK
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_UNK), "SIGTRAP/TRAP_UNK" },      // Undiagnosed trap
#endif

    // Add entries for standard signals
    { SIGNAL_CODE_CREATE(SIGINT, 0), "SIGINT" },
    { SIGNAL_CODE_CREATE(SIGILL, 0), "SIGILL" },
    { SIGNAL_CODE_CREATE(SIGABRT, 0), "SIGABRT" },
    { SIGNAL_CODE_CREATE(SIGFPE, 0), "SIGFPE" },
    { SIGNAL_CODE_CREATE(SIGSEGV, 0), "SIGSEGV" },
    { SIGNAL_CODE_CREATE(SIGTERM, 0), "SIGTERM" },
    { SIGNAL_CODE_CREATE(SIGHUP, 0), "SIGHUP" },
    { SIGNAL_CODE_CREATE(SIGQUIT, 0), "SIGQUIT" },
    { SIGNAL_CODE_CREATE(SIGTRAP, 0), "SIGTRAP" },
    { SIGNAL_CODE_CREATE(SIGKILL, 0), "SIGKILL" },
    { SIGNAL_CODE_CREATE(SIGPIPE, 0), "SIGPIPE" },
    { SIGNAL_CODE_CREATE(SIGALRM, 0), "SIGALRM" },
    { SIGNAL_CODE_CREATE(SIGCHLD, 0), "SIGCHLD" },

    { SIGNAL_CODE_CREATE(SIGUSR1, 0), "SIGUSR1" },
    { SIGNAL_CODE_CREATE(SIGUSR2, 0), "SIGUSR2" },

    // Terminator
    { 0, NULL },
};

// Define string conversion functions for SIGNAL_CODE type
ENUM_STR_DEFINE_FUNCTIONS(SIGNAL_CODE, 0, "");

void SIGNAL_CODE_2str_h(SIGNAL_CODE code, char *buf, size_t len) {
    if(!buf || !len) return;
    if(len < 3) {
        buf[0] = '\0';
        return;
    }

    const char *s = SIGNAL_CODE_2str(code);
    if (!s || !*s) {
        char b[UINT64_MAX_LENGTH + 2];
        strcpy(b, "0x");
        print_uint64_hex(&b[2], code);
        strncpyz(buf, b, len - 1);
    }
    else
        strncpyz(buf, s, len - 1);
}

SIGNAL_CODE SIGNAL_CODE_2id_h(const char *str) {
    if(strncmp(str, "0x", 2) == 0)
        return strtoull(str + 2, NULL, 16);

    return SIGNAL_CODE_2id(str);
}
