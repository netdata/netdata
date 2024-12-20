// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SETENV_H
#define NETDATA_SETENV_H

#include "config.h"

#ifndef HAVE_SETENV
int os_setenv(const char *name, const char *value, int overwrite);
#define setenv(name, value, overwrite) os_setenv(name, value, overwrite)
#endif

void nd_setenv(const char *name, const char *value, int overwrite);

#endif //NETDATA_SETENV_H
