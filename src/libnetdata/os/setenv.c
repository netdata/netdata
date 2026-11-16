// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if !defined(HAVE_SECURE_GETENV) && defined(HAVE_GETAUXVAL)
#include <sys/auxv.h>
// AT_SECURE lives in <elf.h>, which the header above pulls in on the libcs we
// build against. Kept as a backstop for one that does not: the value is fixed
// kernel ABI, not a libc choice.
#if !defined(AT_SECURE)
#define AT_SECURE 23
#endif
#endif

#ifndef HAVE_SETENV
int os_setenv(const char *name, const char *value, int overwrite) {
    char *env_var;
    int result;

    if (!overwrite) {
        env_var = getenv(name);
        if (env_var) return 0; // Already set
    }

    size_t len = strlen(name) + strlen(value) + 2; // +2 for '=' and '\0'
    env_var = malloc(len);
    if (!env_var) return -1; // Allocation failure
    snprintf(env_var, len, "%s=%s", name, value);

    result = putenv(env_var);
    // free(env_var); // _putenv in Windows makes a copy of the string
    return result;
}

#endif

void nd_setenv(const char *name, const char *value, int overwrite) {
#if defined(OS_WINDOWS)
    if(overwrite)
        SetEnvironmentVariable(name, value);
    else {
        char buf[1024];
        if(GetEnvironmentVariable(name, buf, sizeof(buf)) == 0)
            SetEnvironmentVariable(name, value);
    }
#endif

#ifdef HAVE_SETENV
    setenv(name, value, overwrite);
#else
    os_setenv(name, value, overwrite);
#endif
}

#if !defined(HAVE_SECURE_GETENV)

// Did this process gain privileges at exec, i.e. is it more privileged than
// whoever started it? Each branch below is a way to answer that, in order of how
// completely it does so.
static bool nd_secure_execution(void) {
#if defined(OS_WINDOWS)
    // no setuid/setgid execution and no file capabilities to protect against
    return false;

#elif defined(HAVE_GETAUXVAL)
    // AT_SECURE is the kernel's own answer, set for every way a process can gain
    // privileges at exec: setuid, setgid, and file capabilities. Unlike reading
    // /proc/self/auxv, this reads the auxiliary vector already in our address
    // space, so it works no matter which uid we ended up with.
    return getauxval(AT_SECURE) != 0;

#elif defined(HAVE_ISSETUGID)
    // BSD and macOS, which have no file capabilities. Stays true for the life of
    // the process, so it is not defeated by a later uid change the way comparing
    // ids would be.
    return issetugid() != 0;

#else
    // Last resort. This catches setuid and setgid only: a platform that grants
    // privileges some other way (file capabilities) is NOT covered here, so keep
    // this branch unreachable on any such platform.
    return geteuid() != getuid() || getegid() != getgid();
#endif
}

#endif // !HAVE_SECURE_GETENV

char *nd_secure_getenv(const char *name) {
#if defined(HAVE_SECURE_GETENV)
    // libc answers this from AT_SECURE too; prefer it where it exists so we match
    // whatever else in the process is reading the environment through libc.
    return secure_getenv(name);
#else
    return nd_secure_execution() ? NULL : getenv(name);
#endif
}
