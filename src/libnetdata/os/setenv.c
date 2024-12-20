// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

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
    os_setenv(name, value, overwite);
#endif
}
