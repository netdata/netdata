// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SETENV_H
#define NETDATA_SETENV_H

#include "config.h"

#ifndef HAVE_SETENV
int os_setenv(const char *name, const char *value, int overwrite);
#define setenv(name, value, overwrite) os_setenv(name, value, overwrite)
#endif

void nd_setenv(const char *name, const char *value, int overwrite);

// getenv() for a process that may be running with more privileges than the one
// that started it (setuid/setgid, or with file capabilities). In that case the
// environment was supplied by a less privileged caller, so anything read from it
// is caller controlled: this returns NULL and the caller uses its compiled-in
// default instead. When the process is not privileged this is plain getenv().
// Use it for every environment variable that names something the process then
// loads, reads, writes or executes.
char *nd_secure_getenv(const char *name);

#endif //NETDATA_SETENV_H
