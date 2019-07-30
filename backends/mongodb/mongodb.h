// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_MONGODB_H
#define NETDATA_BACKEND_MONGODB_H

#include "backends/backends.h"

extern int mongodb_init(const char *uri_string, const char *database_string, const char *collection_string, const int32_t socket_timeout);

extern int mongodb_insert(char *data, size_t n_metrics);

extern void mongodb_cleanup();

extern int read_mongodb_conf(const char *path, char **uri_p, char **database_p, char **collection_p);

#endif //NETDATA_BACKEND_MONGODB_H
