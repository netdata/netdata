// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RUN_DIR_H
#define NETDATA_RUN_DIR_H

#include "libnetdata/libnetdata.h"

/**
 * Initialize and get the runtime directory for Netdata
 * This function gets or creates the runtime directory based on environment or system defaults
 *
 * @param rw When true, create the directory if it doesn't exist
 * @return const char* The runtime directory path
 */
const char *os_run_dir(bool rw);

#endif //NETDATA_RUN_DIR_H
