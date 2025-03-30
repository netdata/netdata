// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATUS_FILE_IO_H
#define NETDATA_STATUS_FILE_IO_H

#include "libnetdata/libnetdata.h"
#include "status-file.h"

void status_file_io_load(const char *filename, bool (*cb)(const char *, void *), void *data);
bool status_file_io_save(const char *filename, BUFFER *payload, bool log);

#endif //NETDATA_STATUS_FILE_IO_H
