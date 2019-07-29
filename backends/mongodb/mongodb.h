// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_MONGODB_H
#define NETDATA_BACKEND_MONGODB_H

#include "backends/backends.h"

extern int mongodb_init(const char *uri_string, const char *database_string, const char *collection_string);

extern int mongodb_insert(char *data);

extern void mongodb_cleanup();

#endif //NETDATA_BACKEND_MONGODB_H
