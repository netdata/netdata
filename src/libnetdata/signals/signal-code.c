// SPDX-License-Identifier: GPL-3.0-or-later

#include "signal-code.h"
#include "libnetdata/libnetdata.h"

#define SIG_NOT_FOUND 0xFFFFFFF

// --------------------------------------------------------------------------------------------------------------------

typedef int SIGNAL_NUM;

ENUM_STR_MAP_DEFINE(SIGNAL_NUM) = {
    { SIGHUP, "SIGHUP" },       // 1 Hangup.
    { SIGINT, "SIGINT" },       // 2 Interactive attention signal.
    { SIGQUIT, "SIGQUIT" },     // 3 Quit.
    { SIGILL, "SIGILL" },       // 4 Illegal instruction.
    { SIGTRAP, "SIGTRAP" },     // 5 Trace/breakpoint trap.
    { SIGABRT, "SIGABRT" },     // 6 Abnormal termination.
    { SIGBUS, "SIGBUS" },       // 7 Bus error.
    { SIGFPE, "SIGFPE" },       // 8 Erroneous arithmetic operation.
    { SIGKILL, "SIGKILL" },     // 9 Killed.
    { SIGUSR1, "SIGUSR1" },     // 10 User-defined signal 1.
    { SIGSEGV, "SIGSEGV" },     // 11 Invalid access to storage.
    { SIGUSR2, "SIGUSR2" },     // 12 User-defined signal 2.
    { SIGPIPE, "SIGPIPE" },     // 13 Broken pipe.
    { SIGALRM, "SIGALRM" },     // 14 Alarm clock.
    { SIGTERM, "SIGTERM" },     // 15 Termination request.
#ifdef SIGSTKFLT
    { SIGSTKFLT, "SIGSTKFLT" }, // 16 Stack fault (obsolete).
#endif
    { SIGCHLD, "SIGCHLD" },     // 17 Child terminated or stopped.
    { SIGCONT, "SIGCONT" },     // 18 Continue.
    { SIGSTOP, "SIGSTOP" },     // 19 Stop, unblockable.
    { SIGTSTP, "SIGTSTP" },     // 20 Keyboard stop.
    { SIGTTIN, "SIGTTIN" },     // 21 Background read from control terminal.
    { SIGTTOU, "SIGTTOU" },     // 22 Background write to control terminal.
    { SIGURG, "SIGURG" },       // 23 Urgent data is available at a socket.
    { SIGXCPU, "SIGXCPU" },     // 24 CPU time limit exceeded.
    { SIGXFSZ, "SIGXFSZ" },     // 25 File size limit exceeded.
    { SIGVTALRM, "SIGVTALRM" }, // 26 Virtual timer expired.
    { SIGPROF, "SIGPROF" },     // 27 Profiling timer expired.
    { SIGWINCH, "SIGWINCH" },   // 28 Window size change (4.3 BSD, Sun).
#ifdef SIGPOLL
    { SIGPOLL, "SIGPOLL" },     // 29 Pollable event occurred (System V).
#endif
#ifdef SIGPWR
    { SIGPWR, "SIGPWR" },       // 30 Power failure imminent.
#endif
    { SIGSYS, "SIGSYS" },       // 31 Bad system call.

    // Terminator
    { 0, NULL },
};

// IMPORTANT: These function return invalid values when the signal is not found - always use a function wrapper
ENUM_STR_DEFINE_FUNCTIONS(SIGNAL_NUM, SIG_NOT_FOUND, NULL);

// --------------------------------------------------------------------------------------------------------------------

typedef int SI_CODE;

ENUM_STR_MAP_DEFINE(SI_CODE) = {
#ifdef SI_ASYNCNL
    { SI_ASYNCNL, "SI_ASYNCNL" },
#endif
#ifdef SI_DETHREAD
    { SI_DETHREAD, "SI_DETHREAD" },
#endif
#ifdef SI_TKILL
    { SI_TKILL, "SI_TKILL" },
#endif
#ifdef SI_SIGIO
    { SI_SIGIO, "SI_SIGIO" },
#endif
    { SI_ASYNCIO, "SI_ASYNCIO" },
    { SI_MESGQ, "SI_MESGQ" },
    { SI_TIMER, "SI_TIMER" },
    { SI_QUEUE, "SI_QUEUE" },
    { SI_USER, "SI_USER" },
#ifdef SI_KERNEL
    { SI_KERNEL, "SI_KERNEL" },
#endif

    // Terminator
    {0, NULL},
};

// IMPORTANT: These function return invalid values when the signal is not found - always use a function wrapper
ENUM_STR_DEFINE_FUNCTIONS(SI_CODE, SIG_NOT_FOUND, NULL);

// --------------------------------------------------------------------------------------------------------------------

// Helper macro to create a SIGNAL_CODE value
#define SIGNAL_CODE_CREATE(signo, si_code) (((uint64_t)(signo) << 32) | (uint32_t)(si_code))

// Define mapping from SIGNAL_CODE to string representation
ENUM_STR_MAP_DEFINE(SIGNAL_CODE) = {
    // SIGILL codes
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLOPC), "ILL_ILLOPC" },   // Illegal opcode
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLOPN), "ILL_ILLOPN" },   // Illegal operand
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLADR), "ILL_ILLADR" },   // Illegal addressing mode
    { SIGNAL_CODE_CREATE(SIGILL, ILL_ILLTRP), "ILL_ILLTRP" },   // Illegal trap
    { SIGNAL_CODE_CREATE(SIGILL, ILL_PRVOPC), "ILL_PRVOPC" },   // Privileged opcode
    { SIGNAL_CODE_CREATE(SIGILL, ILL_PRVREG), "ILL_PRVREG" },   // Privileged register
    { SIGNAL_CODE_CREATE(SIGILL, ILL_COPROC), "ILL_COPROC" },   // Coprocessor error
    { SIGNAL_CODE_CREATE(SIGILL, ILL_BADSTK), "ILL_BADSTK" },   // Internal stack error
#ifdef ILL_BADIADDR
    { SIGNAL_CODE_CREATE(SIGILL, ILL_BADIADDR), "ILL_BADIADDR" }, // Unimplemented instruction address
#endif

    // SIGFPE codes
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_INTDIV), "FPE_INTDIV" },   // Integer divide by zero
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_INTOVF), "FPE_INTOVF" },   // Integer overflow
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTDIV), "FPE_FLTDIV" },   // Floating point divide by zero
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTOVF), "FPE_FLTOVF" },   // Floating point overflow
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTUND), "FPE_FLTUND" },   // Floating point underflow
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTRES), "FPE_FLTRES" },   // Floating point inexact result
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTINV), "FPE_FLTINV" },   // Floating point invalid operation
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTSUB), "FPE_FLTSUB" },   // Subscript out of range
#ifdef FPE_FLTUNK
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_FLTUNK), "FPE_FLTUNK" },   // Undiagnosed floating-point exception
#endif
#ifdef FPE_CONDTRAP
    { SIGNAL_CODE_CREATE(SIGFPE, FPE_CONDTRAP), "FPE_CONDTRAP" }, // Trap on condition
#endif

    // SIGSEGV codes
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_MAPERR), "SEGV_MAPERR" }, // Address not mapped to object
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ACCERR), "SEGV_ACCERR" }, // Invalid permissions for mapped object
#ifdef SEGV_BNDERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_BNDERR), "SEGV_BNDERR" }, // Bounds checking failure
#endif
#ifdef SEGV_PKUERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_PKUERR), "SEGV_PKUERR" }, // Protection key checking failure
#endif
#ifdef SEGV_ACCADI
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ACCADI), "SEGV_ACCADI" }, // ADI not enabled for mapped object
#endif
#ifdef SEGV_ADIDERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ADIDERR), "SEGV_ADIDERR" }, // Disrupting MCD error
#endif
#ifdef SEGV_ADIPERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_ADIPERR), "SEGV_ADIPERR" }, // Precise MCD exception
#endif
#ifdef SEGV_MTEAERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_MTEAERR), "SEGV_MTEAERR" }, // Asynchronous ARM MTE error
#endif
#ifdef SEGV_MTESERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_MTESERR), "SEGV_MTESERR" }, // Synchronous ARM MTE exception
#endif
#ifdef SEGV_CPERR
    { SIGNAL_CODE_CREATE(SIGSEGV, SEGV_CPERR), "SEGV_CPERR" },  // Control protection fault
#endif

    // SIGBUS codes
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_ADRALN), "BUS_ADRALN" },   // Invalid address alignment
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_ADRERR), "BUS_ADRERR" },   // Non-existent physical address
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_OBJERR), "BUS_OBJERR" },   // Object specific hardware error
#ifdef BUS_MCEERR_AR
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_MCEERR_AR), "BUS_MCEERR_AR" }, // Hardware memory error: action required
#endif
#ifdef BUS_MCEERR_AO
    { SIGNAL_CODE_CREATE(SIGBUS, BUS_MCEERR_AO), "BUS_MCEERR_AO" }, // Hardware memory error: action optional
#endif

    // SIGTRAP codes
#ifdef TRAP_BRKPT
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_BRKPT), "TRAP_BRKPT" },  // Process breakpoint
#endif
#ifdef TRAP_TRACE
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_TRACE), "TRAP_TRACE" },  // Process trace trap
#endif
#ifdef TRAP_BRANCH
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_BRANCH), "TRAP_BRANCH" }, // Process taken branch trap
#endif
#ifdef TRAP_HWBKPT
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_HWBKPT), "TRAP_HWBKPT" }, // Hardware breakpoint/watchpoint
#endif
#ifdef TRAP_UNK
    { SIGNAL_CODE_CREATE(SIGTRAP, TRAP_UNK), "TRAP_UNK" },      // Undiagnosed trap
#endif

    // Terminator
    { 0, NULL },
};

// IMPORTANT: These function return invalid values when the signal is not found - always use a function wrapper
ENUM_STR_DEFINE_FUNCTIONS(SIGNAL_CODE, SIG_NOT_FOUND, NULL);

// --------------------------------------------------------------------------------------------------------------------

void SIGNAL_CODE_2str_h(SIGNAL_CODE code, char *buf, size_t size) {
    if(!buf || !size) return;
    if(size < 3 || code == 0) {
        buf[0] = '\0';
        return;
    }

    char b1[UINT64_MAX_LENGTH];
    char b2[UINT64_MAX_LENGTH];

    int signo = SIGNAL_CODE_GET_SIGNO(code);
    int si_code = SIGNAL_CODE_GET_SI_CODE(code);

    // find the string for the signal
    const char *signo_str = SIGNAL_NUM_2str(signo);
    if(!signo_str || !*signo_str) {
        print_int64(b1, signo);
        signo_str = b1;
    }

    // find the string for the si_code
    const char *si_code_str = SIGNAL_CODE_2str(code);
    if(!si_code_str || !*si_code_str) {
        si_code_str = SI_CODE_2str(si_code);
        if(!si_code_str || !*si_code_str) {
            print_int64(b2, si_code);
            si_code_str = b2;
        }
    }

    // now we have to concatenate the two strings
    // with a slash in between

    strncpyz(buf, signo_str, size - 1);
    size_t len = strlen(buf);

    if(size - 1 > len)
        buf[len++] = '/';

    if(size -1 > len)
        strncpyz(&buf[len], si_code_str, size - 1 - len);

    buf[size - 1] = '\0';
}

SIGNAL_CODE SIGNAL_CODE_2id_h(const char *str) {
    if(!str || !*str) return 0;

    char buf[strlen(str) + 1];
    memcpy(buf, str, strlen(str) + 1);

    char *si_code_str = strchr(buf, '/');
    if(si_code_str) {
        *si_code_str = '\0';
        si_code_str++;
    }

    int signo = SIGNAL_NUM_2id(buf);
    if(signo == SIG_NOT_FOUND)
        signo = str2i(buf);

    SIGNAL_CODE code = si_code_str ? SIGNAL_CODE_2id(si_code_str) : SIG_NOT_FOUND;
    if(code != SIG_NOT_FOUND)
        // this has both signo and si_code in it
        return code;

    int si_code = si_code_str ? SI_CODE_2id(si_code_str) : SIG_NOT_FOUND;
    if(si_code == SIG_NOT_FOUND)
        si_code = si_code_str ? str2i(si_code_str) : 0;

    return SIGNAL_CODE_CREATE(signo, si_code);
}

// Function to create a SIGNAL_CODE from signal number and signal code
SIGNAL_CODE signal_code(int signo, int si_code) {
    return SIGNAL_CODE_CREATE(signo, si_code);
}
