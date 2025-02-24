// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROCESS_PATH_H
#define NETDATA_PROCESS_PATH_H

#include "libnetdata/libnetdata.h"

// Get the full path of the current process executable
// Returns a malloced string that must be freed by the caller
// Returns NULL on error
char *os_get_process_path(void);

#endif //NETDATA_PROCESS_PATH_H